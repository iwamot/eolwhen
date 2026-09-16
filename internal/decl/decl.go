// Package decl holds the record every extractor produces.
//
// An extractor reads one file and says what software it declares, which
// version string was written, and where. Nothing here knows about
// endoflife.date, so the records an extractor driven by configuration would
// produce are the same records the built-in extractors produce.
package decl

import "fmt"

// Decl is one declaration read from a file. Product is the name as the file
// spelled it, which the catalog resolves through its own names and aliases;
// Version is the version string verbatim, before any cycle is matched.
type Decl struct {
	Product string
	Version string
	Source  Source
}

// Source locates a declaration. Line is 0 when the file has no meaningful
// line to point at, as with .nvmrc, whose whole content is the declaration.
type Source struct {
	File string
	Line int
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
// it, instead of being reported because its version was a word.
type Unreadable struct {
	Source  Source
	Product string
	Text    string
	Reason  string
}

// What is the line as the reader sees it: the software, when it is named
// apart from the text, and then the text.
func (u Unreadable) What() string {
	if u.Product == "" {
		return u.Text
	}
	return u.Product + " " + u.Text
}
