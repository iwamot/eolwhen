// Package runtimefile reads the small files that name a runtime version:
// .python-version, .nvmrc, .node-version, .ruby-version, and the go
// directive in go.mod.
//
// The file name decides the product. .nvmrc can only be about Node.js, so
// nothing here has to guess a product from the text it reads, and a version
// that turns out to be a word rather than a number is reported rather than
// guessed at.
package runtimefile

import (
	"path/filepath"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Files maps each supported file name to the product it declares, in the
// order they are looked for.
// Prefixes are the decorations that file's own writers put in front of the
// version, and only that file's: stripping ruby- inside an .nvmrc would turn
// a line that makes no sense there into a Node.js version.
var Files = []struct {
	Name     string
	Product  string
	Prefixes []string
}{
	{".python-version", "python", []string{"python-"}},
	{".nvmrc", "nodejs", []string{"node-", "nodejs-"}},
	{".node-version", "nodejs", []string{"node-", "nodejs-"}},
	{".ruby-version", "ruby", []string{"ruby-"}},
	{"go.mod", "go", nil},
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
	return versionLines(path, Files[i].Product, Files[i].Prefixes, data)
}

// versionLines reads a file whose every meaningful line is a version.
// .python-version may name several versions at once, which pyenv uses to
// make more than one interpreter available, so every line is kept.
func versionLines(file, product string, prefixes []string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		src := decl.Source{File: file, Line: i + 1}
		v := normalize(line, prefixes)
		if !looksLikeVersion(v) {
			us = append(us, decl.Unreadable{Source: src, Product: product, Text: line, Reason: reason(line)})
			continue
		}
		ds = append(ds, decl.Decl{Product: product, Version: v, Source: src})
	}
	return ds, us
}

// goMod reads the go directive. The toolchain directive is left alone: it
// names the toolchain used to build, while the go directive is the language
// version the module promises to work with, which is the one whose support
// window matters.
func goMod(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		rest, ok := strings.CutPrefix(line, "go ")
		if !ok {
			continue
		}
		v := strings.TrimSpace(rest)
		if idx := strings.Index(v, "//"); idx >= 0 {
			v = strings.TrimSpace(v[:idx])
		}
		src := decl.Source{File: file, Line: i + 1}
		if !looksLikeVersion(v) {
			return nil, []decl.Unreadable{{Source: src, Product: "go", Text: line, Reason: reason(v)}}
		}
		return []decl.Decl{{Product: "go", Version: v, Source: src}}, nil
	}
	return nil, nil
}

// normalize strips the decorations these files carry around a version: the
// product name that rvm and friends write in front of it, and a leading v.
func normalize(line string, prefixes []string) string {
	v := line
	for _, prefix := range prefixes {
		if rest, ok := strings.CutPrefix(v, prefix); ok {
			v = rest
			break
		}
	}
	return strings.TrimPrefix(v, "v")
}

// looksLikeVersion reports whether s starts with a digit, which is the whole
// test. Whether the version reaches a real cycle is the catalog's answer,
// not this package's.
func looksLikeVersion(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// reason says why a line could not be used, in words that name the next step
// where there is one.
func reason(text string) string {
	switch {
	case strings.HasPrefix(text, "lts/"), text == "lts", text == "node", text == "latest", text == "stable":
		return "names a moving target, not a version"
	case text == "system":
		return "defers to whatever is installed"
	default:
		return "is not a version"
	}
}
