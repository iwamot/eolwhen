// Package pomxml reads the dependencies a Maven pom.xml declares: the parent
// it builds on, and each dependency it asks for by name and version.
//
// What a package name reaches is settled elsewhere. This package says only
// that the line named a Maven artifact, which is its group and its artifact
// together, and the catalog answers through the purls upstream publishes:
// org.springframework.boot/spring-boot-starter-web is Spring Boot because
// endoflife.date says so, while the rest of a manifest is software it does
// not track.
//
// A pom.xml does not close over itself the way the other manifests do. A
// version may be a property, and a property may be set in a parent POM this
// file does not hold; a dependency may carry no version at all, the parent
// deciding it. What is read is what the file settles on its own: a literal
// version, and a property the project's own properties set. The rest names
// no version here, and Maven is the one that resolves it.
package pomxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Ecosystem is the purl type a Maven artifact belongs to.
const Ecosystem = "maven"

// Matches reports whether a file is a Maven POM.
func Matches(name string) bool { return name == "pom.xml" }

// inherited is what is said about a dependency with no version of its own,
// which is the usual way a POM inheriting from a parent is written.
const inherited = "names no version here, so the POM it inherits from decides"

// Extract reads the parent and every dependency a POM declares.
//
// A file that does not parse is skipped in silence, as a composer.json that
// is not JSON yet is: the tool that owns it reports that better than this
// one can, and a file mid-edit is not a declaration that could not be read.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	coords, props := parse(data)
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, c := range coords {
		if c.group == "" || c.artifact == "" {
			// Maven needs both to name an artifact, and so does a purl.
			continue
		}
		src := decl.Source{File: file, Line: c.line}
		name := c.group + "/" + c.artifact
		v, reason := resolve(c.version, props)
		if reason != "" {
			// Each of these leaves the version to Maven on purpose, so
			// there is nothing here to place and nothing to go and change.
			us = append(us, decl.Unreadable{
				Source:    src,
				Ecosystem: Ecosystem,
				Product:   name,
				Text:      c.version,
				Reason:    reason,
				Moving:    true,
			})
			continue
		}
		ds = append(ds, decl.Decl{Ecosystem: Ecosystem, Product: name, Version: v, Source: src})
	}
	return ds, us
}

// resolve reads the version a dependency asks for, standing a property in
// for its value where this file sets one. reason is empty when the file
// settles a version, and otherwise says which way it does not.
//
// There are four ways, and they are not the same thing to read. A dependency
// may carry no version, the parent deciding it. It may be written in terms
// of a property this file does not settle, which a parent POM sets and this
// does not hold. It may name a range, or ask for whatever is newest. Each
// leaves the choice to Maven, which is a resolver this tool does not run.
func resolve(version string, props map[string]string) (v, reason string) {
	v = strings.TrimSpace(version)
	if name, ok := property(v); ok {
		if set, found := props[name]; found {
			v = strings.TrimSpace(set)
		}
	}
	switch {
	case v == "":
		return "", inherited
	case strings.Contains(v, "${"):
		return "", "takes its version from a property this file does not settle"
	case v == "LATEST", v == "RELEASE":
		return "", "asks for whatever is newest, not a version"
	// A range admits a stretch of versions rather than one, and which of
	// them is built with is settled when the build runs.
	case strings.ContainsAny(v, "[](),"):
		return "", "names a range of versions rather than one, so the build decides which one"
	}
	return v, ""
}

// property reads a version written as a reference to one, which Maven
// spells ${name}. A version carrying more than the reference is not one
// this reads: what it works out to is Maven's to say.
func property(v string) (string, bool) {
	name, ok := strings.CutPrefix(v, "${")
	if !ok {
		return "", false
	}
	name, rest, closed := strings.Cut(name, "}")
	if !closed || rest != "" {
		return "", false
	}
	return name, true
}

// coord is one artifact a POM names: its group, its artifact, the version as
// written, and the line to send a reader to.
type coord struct {
	group    string
	artifact string
	version  string
	line     int
}

// parse walks the document, gathering the artifacts it names and the
// properties it sets. Both are needed before either can be read, a property
// being allowed to sit after the dependency that uses it.
func parse(data []byte) ([]coord, map[string]string) {
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	var path []string
	var coords []coord
	props := map[string]string{}
	for {
		t, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return coords, props
			}
			return nil, nil
		}
		switch t := t.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			switch {
			case names(path):
				c, ok := artifact(dec, data)
				if !ok {
					return nil, nil
				}
				coords = append(coords, c)
				path = path[:len(path)-1]
			case sets(path):
				v, ok := text(dec)
				if !ok {
					return nil, nil
				}
				props[t.Name.Local] = v
				path = path[:len(path)-1]
			}
		case xml.EndElement:
			path = path[:len(path)-1]
		}
	}
}

// names reports whether the element just entered names an artifact: the
// parent a POM builds on, or one of its dependencies. A dependency is read
// wherever it is declared — under a profile, or in the dependencyManagement
// that decides versions for the modules below — because each of those is the
// project saying which version it builds with.
func names(path []string) bool {
	if len(path) < 2 {
		return false
	}
	switch last, up := path[len(path)-1], path[len(path)-2]; {
	case last == "parent" && up == "project":
		return true
	case last == "dependency" && up == "dependencies":
		return true
	}
	return false
}

// sets reports whether the element just entered sets a property, which is a
// child of the project's own properties and of nothing else.
//
// A profile may set its own, and two profiles may set the same one to
// different values; which of them applies is settled when the build runs, so
// none of them is read. A profile's dependencies are read all the same: each
// names a version the project builds with somewhere, and a version is what a
// row is about.
func sets(path []string) bool {
	return len(path) == 3 && path[0] == "project" && path[1] == "properties"
}

// artifact reads the group, artifact and version of the element just
// entered, up to its end tag. The line kept is the version's, that being the
// line to go and change; an artifact with no version of its own keeps the
// line it was named on.
func artifact(dec *xml.Decoder, data []byte) (coord, bool) {
	c := coord{line: lineAt(data, dec.InputOffset())}
	for {
		t, err := dec.Token()
		if err != nil {
			return coord{}, false
		}
		switch t := t.(type) {
		case xml.StartElement:
			line := lineAt(data, dec.InputOffset())
			// text reads to this element's own end tag, so what sits deeper
			// is passed over: the groupId of an exclusion is not the
			// dependency's own.
			v, ok := text(dec)
			if !ok {
				return coord{}, false
			}
			switch t.Name.Local {
			case "groupId":
				c.group = v
			case "artifactId":
				c.artifact = v
			case "version":
				c.version, c.line = v, line
			}
		case xml.EndElement:
			return c, true
		}
	}
}

// text reads the character data of the element the decoder has just entered,
// up to its end tag. Markup inside it is passed over: a version is the text,
// and an element carrying anything else carries no more of one for it.
func text(dec *xml.Decoder) (string, bool) {
	var b strings.Builder
	for {
		t, err := dec.Token()
		if err != nil {
			return "", false
		}
		switch t := t.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			if err := dec.Skip(); err != nil {
				return "", false
			}
		case xml.EndElement:
			return strings.TrimSpace(b.String()), true
		}
	}
}

// lineAt is the line an offset falls on, counted from 1.
func lineAt(data []byte, offset int64) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
