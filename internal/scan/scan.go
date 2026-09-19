// Package scan walks one directory and hands each file it recognizes to the
// extractor that reads it. It is the only place that touches the filesystem.
package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/iwamot/eolwhen/internal/compose"
	"github.com/iwamot/eolwhen/internal/composerjson"
	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/dockerfile"
	"github.com/iwamot/eolwhen/internal/gemfile"
	"github.com/iwamot/eolwhen/internal/packagejson"
	"github.com/iwamot/eolwhen/internal/pomxml"
	"github.com/iwamot/eolwhen/internal/projectfile"
	"github.com/iwamot/eolwhen/internal/pythonmanifest"
	"github.com/iwamot/eolwhen/internal/runtimefile"
	"github.com/iwamot/eolwhen/internal/toolfile"
	"github.com/iwamot/eolwhen/internal/workflow"
)

// skipped are directories whose contents belong to something else: another
// project's dependencies, a virtual environment, a build tree. A .nvmrc
// inside node_modules is a package author's declaration, not this
// directory's, and reading it would answer a question nobody asked.
var skipped = map[string]bool{
	".git":         true,
	".terraform":   true,
	".tox":         true,
	".venv":        true,
	"__pycache__":  true,
	"node_modules": true,
	"third-party":  true,
	"third_party":  true,
	"vendor":       true,
	"venv":         true,
}

// Dir reads every file in dir, and below it, that an extractor recognizes.
// Paths are reported relative to dir, so a row reads the same whether the
// directory was named absolutely or as a dot.
//
// The third list is the files that were recognized and read nothing from,
// because they do not parse or do not hold what they are read for. They are
// what keeps a directory that declares nothing apart from one whose
// declarations this run could not see, which are the same empty answer and
// not the same thing to do about it.
func Dir(dir string) ([]decl.Decl, []decl.Unreadable, []decl.Skipped, error) {
	var ds []decl.Decl
	var us []decl.Unreadable
	var sk []decl.Skipped
	// The directory a caller names may be a symlink, and WalkDir does not
	// follow one, so it is resolved here: a checkout reached through a link
	// is the checkout. Links met further down are left alone, which is what
	// keeps a loop from being possible.
	root, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return nil, nil, nil, err
	}
	// Walking a cleaned root means every path below it is that root plus a
	// separator plus the rest, so the part to print is a prefix away and
	// there is no second way for the two to relate.
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// The directory that was asked about has to be readable; a
			// corner of the tree that is not only costs whatever it held,
			// and saying so beats refusing to answer at all.
			if path == root {
				return err
			}
			us = append(us, unreadableAt(root, path, err))
			return nil
		}
		if d.IsDir() {
			// The skip list never applies to the directory that was asked
			// about: `eolwhen vendor` reads that vendor directory.
			if path != root && skipped[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		// Only the directory's own files are read. A link names a file that
		// sits somewhere else, whose declarations are that directory's
		// rather than this one's, and a pipe or a device named like a
		// manifest holds no manifest at all. Both are passed over as
		// quietly as a skipped directory is: nothing here was offered as a
		// file of this directory in the first place.
		if !d.Type().IsRegular() {
			return nil
		}
		// Normalized here and nowhere else: which extractor reads a file
		// depends on the directory it sits in, and a path that still
		// carries the platform's separator would hide .github/workflows
		// from the one that reads it.
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		extract, ok := extractorFor(rel)
		if !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			us = append(us, unreadableAt(root, path, err))
			return nil
		}
		newDs, newUs, reason := extract(rel, data)
		if reason != "" {
			sk = append(sk, decl.Skipped{File: rel, Reason: reason})
			return nil
		}
		ds = append(ds, newDs...)
		us = append(us, newUs...)
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return ds, us, sk, nil
}

// unreadableAt words a path the walk offered but could not open, so that a
// file missing from the answer is never missing without a word.
func unreadableAt(root, path string, err error) decl.Unreadable {
	rel := strings.TrimPrefix(path, root+string(filepath.Separator))
	reason := "could not be read"
	if errors.Is(err, fs.ErrPermission) {
		reason = "could not be read: permission denied"
	}
	// Nothing was read, so there is no text to quote; the path is the whole
	// of what there is to say.
	return decl.Unreadable{Source: decl.Source{File: filepath.ToSlash(rel)}, Reason: reason}
}

// extractor reads one file. The last value is why it read nothing from the
// file, empty when it read it, and a file it read nothing from yields no
// declarations at all: half a file is not half a set of declarations.
type extractor func(file string, data []byte) ([]decl.Decl, []decl.Unreadable, string)

// whole adapts an extractor that reads a file a line at a time. A Dockerfile,
// a Gemfile and a .nvmrc have no document to parse, so there is no state the
// file could be in that leaves the whole of it unread, and nothing for one of
// them to set aside.
func whole(f func(string, []byte) ([]decl.Decl, []decl.Unreadable)) extractor {
	return func(file string, data []byte) ([]decl.Decl, []decl.Unreadable, string) {
		ds, us := f(file, data)
		return ds, us, ""
	}
}

// extractorFor picks the extractor for a path. Which extractor reads a file
// is settled by where it sits and what it is called, which is what lets each
// one know the software it is reading about without guessing. A workflow is
// the one that needs the directory too: a YAML file is only a workflow
// because of where it lives.
func extractorFor(path string) (extractor, bool) {
	if _, ok := runtimefile.Product(filepath.Base(path)); ok {
		return whole(runtimefile.Extract), true
	}
	if dockerfile.Matches(filepath.Base(path)) {
		return whole(dockerfile.Extract), true
	}
	if compose.Matches(filepath.Base(path)) {
		return compose.Extract, true
	}
	if toolfile.Matches(filepath.Base(path)) {
		return toolfile.Extract, true
	}
	if gemfile.Matches(filepath.Base(path)) {
		return whole(gemfile.Extract), true
	}
	if composerjson.Matches(filepath.Base(path)) {
		return composerjson.Extract, true
	}
	if packagejson.Matches(filepath.Base(path)) {
		return packagejson.Extract, true
	}
	if pythonmanifest.Matches(path) {
		return pythonmanifest.Extract, true
	}
	if projectfile.Matches(filepath.Base(path)) {
		return projectfile.Extract, true
	}
	if pomxml.Matches(filepath.Base(path)) {
		return pomxml.Extract, true
	}
	if workflow.Matches(path) {
		return workflow.Extract, true
	}
	return nil, false
}
