// Package runtimefile reads the small files that name a runtime version:
// .python-version, .nvmrc, .node-version, .ruby-version, .php-version,
// .go-version, .terraform-version, and the go directive in go.mod.
//
// The file name decides the product. .nvmrc can only be about Node.js, so
// nothing here has to guess a product from the text it reads, and a version
// that turns out to be a word rather than a number is reported rather than
// guessed at.
//
// That is what keeps .java-version out. A version manager writes 17 in it
// and nothing else, and endoflife.date tracks nine builds of Java, each with
// a calendar of its own, so the file settles which software no more than the
// bare number does.
package runtimefile

import (
	"path/filepath"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
)

// File is one of the files this package reads, and what reading it takes.
//
// Prefixes are the decorations that file's own writers put in front of the
// version, and only that file's: stripping ruby- inside an .nvmrc would turn
// a line that makes no sense there into a Node.js version.
//
// Settings says the file's syntax lets a line be a setting rather than a
// version. An .nvmrc may carry key=value pairs beside the one bare line that
// names the version, and nvm keeps the two apart.
type File struct {
	Name     string
	Product  string
	Prefixes []string
	Settings bool
}

// Files lists each supported file, in the order they are looked for.
var Files = []File{
	{Name: ".python-version", Product: "python", Prefixes: []string{"python-"}},
	{Name: ".nvmrc", Product: "nodejs", Prefixes: []string{"node-", "nodejs-"}, Settings: true},
	{Name: ".node-version", Product: "nodejs", Prefixes: []string{"node-", "nodejs-"}},
	{Name: ".ruby-version", Product: "ruby", Prefixes: []string{"ruby-"}},
	{Name: ".php-version", Product: "php"},
	{Name: ".go-version", Product: "go"},
	{Name: ".terraform-version", Product: "terraform"},
	{Name: "go.mod", Product: "go"},
}

// lookup finds the entry for a file name.
func lookup(file string) (int, bool) {
	for i, f := range Files {
		if f.Name == file {
			return i, true
		}
	}
	return 0, false
}

// Product returns the product a supported file declares.
func Product(file string) (string, bool) {
	i, ok := lookup(file)
	if !ok {
		return "", false
	}
	return Files[i].Product, true
}

// Extract reads one supported file. path is reported as written, so a file
// found below the directory keeps the path it was found at; the product
// comes from the base name, which is the part that decides it.
func Extract(path string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	name := filepath.Base(path)
	i, ok := lookup(name)
	if !ok {
		return nil, nil
	}
	if name == "go.mod" {
		return goMod(path, data)
	}
	return versionLines(path, Files[i], data)
}

// versionLines reads a file whose lines are versions. .python-version may
// name several at once, which pyenv uses to make more than one interpreter
// available, so every line is kept, and a line that names a setting instead
// is passed over where the file's syntax has them.
func versionLines(path string, f File, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for i, raw := range strings.Split(string(data), "\n") {
		line := content(raw)
		if line == "" || (f.Settings && setting(line)) {
			continue
		}
		// pyenv, nodenv and rbenv read the first word of a line and ignore
		// whatever follows it, so a second word falls away here as a
		// trailing comment already has.
		text := strings.Fields(line)[0]
		src := decl.Source{File: path, Line: i + 1}
		v := normalize(text, f.Prefixes)
		// A codename is only read where a file's own syntax marks one:
		// nvm's lts/hydrogen names Node.js 18, while a bare word in a
		// .python-version is an interpreter's name and not a release.
		if !looksLikeVersion(v) && !(strings.HasPrefix(text, "lts/") && codename(v)) {
			r, moving := reason(text)
			us = append(us, decl.Unreadable{Source: src, Product: f.Product, Text: text, Reason: r, Moving: moving})
			continue
		}
		ds = append(ds, decl.Decl{Product: f.Product, Version: v, Source: src})
	}
	return ds, us
}

// goMod reads the go directive. The toolchain directive is left alone: it
// names the toolchain used to build, while the go directive is the language
// version the module promises to work with, which is the one whose support
// window matters.
func goMod(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	for i, raw := range strings.Split(string(data), "\n") {
		// The directive is a word and its arguments, separated by any run of
		// spaces or tabs, so the version is the second word rather than
		// whatever follows a single space. What comes after it is a comment.
		fields := strings.Fields(raw)
		if len(fields) < 2 || fields[0] != "go" {
			continue
		}
		v := fields[1]
		src := decl.Source{File: file, Line: i + 1}
		if !looksLikeVersion(v) {
			r, moving := reason(v)
			return nil, []decl.Unreadable{{Source: src, Product: "go", Text: v, Reason: r, Moving: moving}}
		}
		return []decl.Decl{{Product: "go", Version: v, Source: src}}, nil
	}
	return nil, nil
}

// content is what a line says once its comment is off. nvm reads an .nvmrc
// that way — a # starts a comment wherever it appears — while pyenv, nodenv
// and rbenv read the first word of a line and ignore the rest of it, so a
// trailing comment falls away under either reading. An empty result is a
// line that says nothing, comment or not.
func content(line string) string {
	text, _, _ := strings.Cut(line, "#")
	return strings.TrimSpace(text)
}

// setting reports whether a line is one of the key=value pairs an .nvmrc may
// carry beside its version. A pair names something other than a version — the
// version is the bare line — so there is nothing in it to look up. A line
// with nothing before the = is not a pair, and nvm reads it as the bare line.
func setting(line string) bool {
	return strings.Index(line, "=") > 0
}

// normalize strips the decorations these files carry around a version: the
// product name that rvm and friends write in front of it, the lts/ that nvm
// writes in front of a codename, and a leading v.
func normalize(line string, prefixes []string) string {
	v := line
	for _, prefix := range prefixes {
		if rest, ok := strings.CutPrefix(v, prefix); ok {
			v = rest
			break
		}
	}
	v = strings.TrimPrefix(v, "lts/")
	return strings.TrimPrefix(v, "v")
}

// codename reports whether what nvm's lts/ was written in front of is one
// word, which is how nvm names a release line it does not number:
// lts/hydrogen is Node.js 18, and the codename endoflife.date publishes for
// that cycle says so. Which words are codenames is the catalog's answer, so
// a word that is none of them comes back as a version no cycle covers, which
// is what it is.
//
// lts/* and lts/latest are not read this way: they are the newest of them,
// which moves.
func codename(s string) bool {
	if s == "" || s == "latest" {
		return false
	}
	for i := range len(s) {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// looksLikeVersion reports whether s starts with a digit, which is the whole
// test. Whether the version reaches a real cycle is the catalog's answer,
// not this package's.
func looksLikeVersion(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// reason says why a line could not be used, in words that name the next step
// where there is one, and whether the line names no fixed version by design.
// A moving target, a version left to whatever is installed, and one left to
// the configuration beside it are all the latter: they were never going to
// have a date, so there is nothing to go and look at.
func reason(text string) (string, bool) {
	switch {
	case strings.HasPrefix(text, "lts/"), text == "lts", text == "node", text == "latest", text == "stable":
		return "names a moving target, not a version", true
	// tfenv writes the newest release matching a pattern, which is a rule
	// for following one rather than a version, and reads min-required out
	// of the configuration beside it, which is where the version then is.
	case strings.HasPrefix(text, "latest:"):
		return "names the newest release matching a pattern, not a version", true
	case text == "min-required":
		return "defers to what the configuration requires", true
	case text == "system":
		return "defers to whatever is installed", true
	default:
		return "is not a version", false
	}
}
