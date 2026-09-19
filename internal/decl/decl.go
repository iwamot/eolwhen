// Package decl holds the record every extractor produces.
//
// An extractor reads one file and says what software it declares, which
// version string was written, and where. Nothing here knows about
// endoflife.date, so the records an extractor driven by configuration would
// produce are the same records the built-in extractors produce.
package decl

import (
	"fmt"

	"github.com/iwamot/eolwhen/internal/span"
)

// Decl is one declaration read from a file. Product is the name as the file
// spelled it and Version is the version string verbatim, both before the
// catalog has been asked anything.
//
// Ecosystem is what decides how the name is answered. It is empty when the
// file named a piece of software, which the catalog resolves through its own
// names and aliases, and it is set when the file named a package instead:
// the gem in a Gemfile. A package reaches a product through the purls
// upstream publishes for that registry and through nothing else, which is
// the whole difference between the two kinds of line — `pg` in a Gemfile is
// the PostgreSQL driver and reaches no product, while `postgres` in a
// Compose file is the database.
type Decl struct {
	Ecosystem string
	Product   string
	Version   string
	// Allows is the range of versions the declaration admits, set when the
	// file pinned a range rather than a version, as a manifest usually
	// does, and set only when both of its ends are known.
	// Version stays the text as written, which is what a reader is shown.
	// A range still reaches a cycle when the whole of it sits inside one.
	Allows span.Span
	Source Source
}

// What is the declaration as the reader sees it: the software and the
// version it was given, or the software alone when the line named none, as
// a manifest line that leaves the version to a lockfile does.
func (d Decl) What() string { return what(d.Product, d.Version) }

// Source locates a declaration. Line is 0 when the file has no meaningful
// line to point at, as with .nvmrc, whose whole content is the declaration.
type Source struct {
	File string
	Line int
}

// Before orders one place in the directory against another: as a path, and
// then as a number rather than as text, so a Dockerfile's line 8 comes
// before its line 62 instead of after it. It is what lets a list of places
// read down the directory in one direction.
func (s Source) Before(o Source) bool {
	if s.File != o.File {
		return s.File < o.File
	}
	return s.Line < o.Line
}

func (s Source) String() string {
	if s.Line == 0 {
		return s.File
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// Unreadable is a declaration that was found but could not be placed on the
// timeline: the version is not a version (`lts/hydrogen`, `ubuntu-latest`),
// or no cycle covers it. Reporting these is what keeps "nothing is expiring"
// apart from "nothing could be read".
//
// Product is the software the line is about, when the file names it apart
// from the version: a tool list's key, a runtime file's name. It is empty
// when the text itself names the software, as an image reference or a
// runner label does. Carrying it is what lets a line about software
// endoflife.date does not track be set aside like any other declaration of
// it, instead of being reported because its version was a word. Ecosystem
// goes with it and means what it means on Decl: a package name, answered by
// the purls upstream publishes and by nothing else.
//
// Moving says the line names no fixed version of its own: `:latest`,
// `ubuntu-latest`, a tag naming only a major line, a tool pinned to
// `stable`, a manifest requirement that leaves the version to a lockfile.
// Those are not lines to go and look at — the version is settled somewhere
// else on purpose, and the reason says where — so they are set aside like
// software the catalog does not track, and --verbose accounts for them.
// Reporting them was what filled the output with lines nobody could act on.
type Unreadable struct {
	Source    Source
	Ecosystem string
	Product   string
	Text      string
	Reason    string
	Moving    bool
}

// What is the line as the reader sees it: the software, when it is named
// apart from the text, and then the text.
func (u Unreadable) What() string { return what(u.Product, u.Text) }

// what joins the software and what was written about it, leaving out
// whichever of the two the line did not have: a runner label names its own
// software, and a manifest line may name no version.
func what(product, text string) string {
	switch {
	case product == "":
		return text
	case text == "":
		return product
	}
	return product + " " + text
}

// Skipped is a file an extractor recognized and read nothing from: the file
// does not parse, or a member it is read for does not hold what it is read
// for. It names a file and not a line, because the whole of it is set aside
// — half a file is not half a set of declarations — and because the line a
// parser stopped on is not where a reader has to go to fix it.
//
// It is kept apart from Unreadable, which is a line to go and look at.
// Nothing is wrong with a file mid-edit, and the tool that owns it says what
// is wrong with it better than this one can, so a skipped file earns no
// complaint. What it costs is that the directory declares more than the run
// could see, and that is worth being able to find out, which is what this
// is for.
type Skipped struct {
	File   string
	Reason string
}

// The reasons a file is set aside whole. They are a closed set, so a caller
// reading the document tells one from another without reading prose, and
// none of them repeats anything the file said. A parser's own message quotes
// the line it stopped on, and a file this tool skips may hold anything at
// all — a token, a password in a Compose file that is not YAML yet — so the
// message is the one thing not to pass on.
//
// Each of them is written to follow the path it is about, so that a line of
// stderr reads as a sentence and still begins with the file, which is what
// keeps stderr greppable.
const (
	NotJSON = "not JSON"
	NotYAML = "not YAML"
	NotTOML = "not TOML"
	NotXML  = "not XML"
	// Shape is a document that parses and then does not hold what it is
	// read for: a package.json whose dependencies is a list, an engines
	// whose value is a number. The file is its format and is still not a
	// manifest, which is a different thing to go and look at.
	Shape = "not the shape it is read for"
)
