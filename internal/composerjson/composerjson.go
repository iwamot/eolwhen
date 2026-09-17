// Package composerjson reads the packages a composer.json requires.
//
// What a package name reaches is settled elsewhere. This package says only
// that the line named a Composer package, and the catalog answers through
// the purls upstream publishes: `laravel/framework` is Laravel because
// endoflife.date says pkg:composer/laravel/framework, while the rest of a
// manifest is software it does not track.
//
// One name is not a package at all. Composer reserves `php` for the version
// of PHP the project runs on, and that is a runtime declaration like the one
// in a .python-version: the key can only mean the language, because Composer
// says so rather than because the name reads that way. Its other reserved
// names — ext-, lib-, hhvm, composer itself — say which extensions and
// which Composer a project needs, none of which is software with a release
// calendar of its own, so they are passed over.
package composerjson

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Ecosystem is the purl type a Composer package name belongs to.
const Ecosystem = "composer"

// Matches reports whether a file is a composer.json.
func Matches(name string) bool { return name == "composer.json" }

// required are the members whose object holds the packages a project
// declares. What it needs to develop is declared as plainly as what it needs
// to run, and a development framework goes out of support the same way.
var required = []string{"require", "require-dev"}

// Extract reads every package a composer.json requires.
//
// A file that does not parse is skipped in silence, as a Compose file that
// is not YAML yet is: the tool that owns it reports that better than this
// one can, and a file mid-edit is not a declaration that could not be read.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, m := range members(data) {
		if platform(m.name) {
			continue
		}
		src := decl.Source{File: file, Line: m.line}
		// The one name Composer reserves that does name software is the
		// language itself, and the catalog answers to that by name rather
		// than through a purl, as it does for any other runtime.
		ecosystem := Ecosystem
		if m.name == "php" {
			ecosystem = ""
		}
		allowed, ok := allows(m.constraint)
		if !ok {
			us = append(us, decl.Unreadable{
				Source:    src,
				Ecosystem: ecosystem,
				Product:   m.name,
				Text:      m.constraint,
				Reason:    "names no single version here, so the lockfile decides which one",
				Moving:    true,
			})
			continue
		}
		ds = append(ds, decl.Decl{
			Ecosystem: ecosystem,
			Product:   m.name,
			Version:   m.constraint,
			Allows:    allowed,
			Source:    src,
		})
	}
	return ds, us
}

// platform reports whether a name is one of Composer's reserved names for
// something other than a package: an extension, a library the runtime was
// built against, Composer itself. `php` is reserved too and is not among
// these, being the one that names software with an end-of-life date.
func platform(name string) bool {
	// A package is written vendor/name and a reserved name never is, which
	// is what keeps composer/semver — a package like any other — apart from
	// composer, the tool.
	if strings.Contains(name, "/") {
		return false
	}
	switch {
	case strings.HasPrefix(name, "ext-"), strings.HasPrefix(name, "lib-"):
		return true
	case strings.HasPrefix(name, "composer"):
		return true
	case name == "hhvm":
		return true
	// php-64bit and its kind ask for how PHP was built rather than for a
	// version of it, and the version they carry is the same one php already
	// declares.
	case strings.HasPrefix(name, "php-"):
		return true
	}
	return false
}

// member is one entry of a require object: the package, what it asked for,
// and the line it was written on.
type member struct {
	name       string
	constraint string
	line       int
}

// members reads the require objects, keeping the line each entry sits on.
// The document is walked as tokens rather than decoded into a map, because
// a map has no line numbers in it and the line is what a reader is sent to.
//
// A document that does not parse is read as nothing at all, the same answer
// an unparsable Compose file or mise.toml gets: half a file is not half a
// set of declarations, and the tool that owns it says what is wrong with it
// better than this one can.
func members(data []byte) []member {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out []member
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil
		}
		// A JSON object's key is a string by construction.
		name, _ := key.(string)
		if !slices.Contains(required, name) {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil
			}
			continue
		}
		read, ok := object(dec, data)
		if !ok {
			return nil
		}
		out = append(out, read...)
	}
	return out
}

// object reads one require object, which is a mapping of package names to
// the constraint each was given. ok is false when the document ran out, and
// when what require held was not an object at all.
func object(dec *json.Decoder, data []byte) (out []member, ok bool) {
	t, err := dec.Token()
	if err != nil || t != json.Delim('{') {
		return nil, false
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, false
		}
		name, _ := key.(string)
		line := lineAt(data, dec.InputOffset())
		var constraint string
		if err := dec.Decode(&constraint); err != nil {
			return nil, false
		}
		// A package is what a name has to be to be looked up, and an empty
		// one is not that.
		if name != "" {
			out = append(out, member{name: strings.ToLower(name), constraint: constraint, line: line})
		}
	}
	// The loop ends at the object's closing brace, and by then it can be
	// nothing else: a document that ran out mid-object has already been
	// given up on above. Stepping over it is what leaves the decoder on the
	// next member of the document.
	_, _ = dec.Token()
	return out, true
}

// lineAt is the line an offset falls on, counted from 1.
func lineAt(data []byte, offset int64) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
