// Package projectfile reads the target framework an MSBuild project file
// declares: the <TargetFramework> of a .csproj, .fsproj or .vbproj, and the
// <TargetFrameworkVersion> a project written before the SDK-style project
// existed spelled the same thing as.
//
// A target framework moniker is a runtime declaration and not a package
// one. net6.0 says the project runs on .NET 6 the way a .python-version
// says 2.7, so there is no purl to go through and no name to guess at: the
// monikers are a vocabulary .NET defines, and the word in front of the
// digits says which .NET. net6.0 is Microsoft .NET and net472 is the .NET
// Framework, which are different products with different calendars, and the
// dot is what tells them apart — the .0 in net5.0 was added precisely so
// that the two could never be read as each other.
//
// A moniker naming something else — netstandard, a Xamarin or UWP target —
// is read as the declaration it is and looked up like any other. netstandard
// is an API contract rather than a thing that runs, and nothing publishes an
// end-of-life date for one, so it is set aside like a tool the catalog does
// not track.
package projectfile

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
)

// extensions are the ones a project file carries. Which of them it is says
// the language the project is written in rather than what it declares: all
// three hold the same property, spelled the same way.
var extensions = []string{".csproj", ".fsproj", ".vbproj"}

// Matches reports whether a file is an MSBuild project file. The extension
// is compared without case, as MSBuild and the platform it grew up on
// compare it.
func Matches(name string) bool {
	return slices.Contains(extensions, strings.ToLower(filepath.Ext(name)))
}

// Extract reads every target framework a project file declares.
//
// A file that does not parse is skipped in silence, as a composer.json that
// is not JSON yet is: the tool that owns it reports that better than this
// one can, and a file mid-edit is not a declaration that could not be read.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, p := range properties(data) {
		src := decl.Source{File: file, Line: p.line}
		// TargetFrameworks holds several at once, semicolon-separated,
		// which is how one project builds for more than one runtime. Each
		// is a declaration of its own, as each line of a .python-version is.
		for text := range strings.SplitSeq(p.value, ";") {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			product, version, reason, moving := target(p.name, text)
			if reason != "" {
				us = append(us, decl.Unreadable{Source: src, Text: text, Reason: reason, Moving: moving})
				continue
			}
			ds = append(ds, decl.Decl{Product: product, Version: version, Source: src})
		}
	}
	return ds, us
}

// legacy is the property an older project used to say the same thing, its
// value written as v4.7.2. It only ever named .NET Framework versions, the
// SDK-style project having arrived with .NET Core.
const legacy = "TargetFrameworkVersion"

// wanted are the properties that carry a target framework. Which of them a
// file uses says how old it is rather than what it targets.
var wanted = []string{"TargetFramework", "TargetFrameworks", legacy}

// target says what one target framework value declares: the product and the
// version of it, or the reason there is neither.
//
// The version an MSBuild property stands in for is not in this file. It is
// in a Directory.Build.props or on the command line, which is the same kind
// of answer a manifest gives when it leaves a version to the lockfile: the
// line names no version of its own on purpose, so there is nothing here to
// place and nothing to go and change.
func target(property, text string) (product, version, reason string, moving bool) {
	if strings.Contains(text, "$(") {
		return "", "", "is written as an MSBuild property, whose value is set outside this file", true
	}
	if property == legacy {
		v, ok := strings.CutPrefix(strings.ToLower(text), "v")
		if !ok || !numeric(v) {
			return "", "", "is not a .NET Framework version", false
		}
		return "dotnetfx", framework(v), "", false
	}
	product, version, reason = moniker(text)
	return product, version, reason, false
}

// moniker reads a target framework moniker: the framework it names, and the
// version of it.
//
// The platform a moniker may end in — the -windows of net8.0-windows, the
// profile of a portable-net45+win8 — says which APIs the target adds and not
// which version of it is being targeted, so it is cut away before anything
// else.
func moniker(text string) (product, version, reason string) {
	name, _, _ := strings.Cut(strings.ToLower(text), "-")
	// The identifier is the letters in front and the version is the digits
	// behind, which is the whole of a moniker's shape.
	i := strings.IndexFunc(name, func(r rune) bool { return r >= '0' && r <= '9' })
	if name == "" || i == 0 {
		// No moniker starts with a digit, and a value that is all platform
		// and no framework is not one either.
		return "", "", notAMoniker
	}
	identifier, digits := name, ""
	if i > 0 {
		identifier, digits = name[:i], name[i:]
	}
	switch {
	case identifier != "net" && identifier != "netcoreapp":
		// Anything else is looked up under the name it gave. A platform
		// target may carry no version at all, which is still a declaration
		// of the platform.
		if digits != "" && !numeric(digits) {
			return "", "", notAMoniker
		}
		return identifier, digits, ""
	case !numeric(digits):
		return "", "", notAMoniker
	// A dot is Microsoft .NET: net5.0 and above are spelled with one so that
	// they can never be read as a .NET Framework, and netcoreapp is the same
	// product under the name it had before. Bare digits are a .NET
	// Framework, which stopped at 4.8.1 and will not gain another.
	case identifier == "netcoreapp", strings.Contains(digits, "."):
		return "dotnet", digits, ""
	default:
		return "dotnetfx", framework(spread(digits)), ""
	}
}

// notAMoniker is what is said about a value that is not a target framework
// at all, which is one complaint however many ways there are to write one.
const notAMoniker = "is not a target framework moniker"

// spread reads the digits of a .NET Framework moniker as the version they
// stand for, one digit per segment: net48 is 4.8 and net472 is 4.7.2.
func spread(digits string) string {
	var b strings.Builder
	for i, d := range digits {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// framework answers with the cycle endoflife.date tracks a .NET Framework
// version under, which is the version itself everywhere but one.
//
// 3.5 is tracked as 3.5-sp1, the service pack being the only 3.5 still
// installable and the only one still supported — for years yet, and longer
// than several of the 4.x versions above it. Leaving it as 3.5 would reach
// no cycle at all and fall below the oldest one tracked, which would date a
// target that is still supported by the day 4.0 went out of support.
func framework(v string) string {
	if v == "3.5" {
		return "3.5-sp1"
	}
	return v
}

// numeric reports whether a string is a version's worth of digits and dots.
func numeric(s string) bool {
	if s == "" {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool {
		return (r < '0' || r > '9') && r != '.'
	}) < 0
}

// property is one property the file set, and the line it sits on.
type property struct {
	name  string
	value string
	line  int
}

// properties reads the target framework properties a project sets, keeping
// the line each sits on. The document is walked as tokens rather than
// unmarshalled into a struct, because a struct has no line numbers in it and
// the line is what a reader is sent to.
//
// A property is a child of a PropertyGroup and nothing else is, which is
// what keeps the same name apart from the metadata of a project reference,
// where it says which build of another project to use rather than what this
// one targets.
func properties(data []byte) []property {
	// A file Visual Studio wrote may start with a byte order mark, which is
	// not markup and leaves the decoder with nothing it can read.
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	var out []property
	var stack []string
	for {
		t, err := dec.Token()
		if err != nil {
			// The end of the document, or markup that does not parse. A
			// half-read file is not half a set of declarations, so what was
			// gathered so far is kept only when the document ended.
			if errors.Is(err, io.EOF) {
				return out
			}
			return nil
		}
		switch t := t.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			if len(stack) < 2 || stack[len(stack)-2] != "PropertyGroup" {
				continue
			}
			i := slices.IndexFunc(wanted, func(w string) bool { return strings.EqualFold(w, t.Name.Local) })
			if i < 0 {
				continue
			}
			// The line is taken before the element is read, so that a
			// value written across several lines still sends the reader to
			// the property rather than to the end of it.
			line := lineAt(data, dec.InputOffset())
			value, ok := text(dec)
			if !ok {
				return nil
			}
			stack = stack[:len(stack)-1]
			out = append(out, property{name: wanted[i], value: value, line: line})
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
}

// text reads the character data of the element the decoder has just entered,
// up to its end tag. Markup inside it — a comment, an element — is passed
// over: a version is the text, and a property carrying anything else carries
// no more of one for it.
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
