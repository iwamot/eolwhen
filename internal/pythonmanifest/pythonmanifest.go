// Package pythonmanifest reads the packages a Python project declares: the
// dependencies of a pyproject.toml and the lines of a requirements.txt.
//
// What a package name reaches is settled elsewhere. This package says only
// that the line named a PyPI package, and the catalog answers through the
// purls upstream publishes: `django` is Django because endoflife.date says
// pkg:pypi/django, while the rest of a manifest is software it does not
// track. PyPI compares a name with its dashes, underscores and dots reduced
// to one, so a manifest may spell it any of those ways and the catalog
// answers to all of them.
//
// A requirement usually names a range rather than a version, and Python's
// are narrow: == names one version outright, and ~=4.2.0 and ==4.2.* each
// name one release cycle, which is how a requirements.txt is usually
// written.
//
// requires-python is read the same way, being the same kind of statement
// about the interpreter instead of a package. It is almost always written
// >=3.9 or the like, which names a floor and no ceiling and so names no
// cycle, and it is then set aside like any other requirement no one version
// answers.
package pythonmanifest

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Ecosystem is the purl type a Python package name belongs to.
const Ecosystem = "pypi"

// unsettled is what is said about a requirement naming no one version, and
// hostDecides the same for the one that is not on a package.
//
// Not the lockfile the other manifests are told to blame: a requirements.txt
// is often the whole of what a project pins, with nothing beside it to hold
// the answer. And which interpreter a project runs on is whatever the
// machine has, which is not an install's answer either.
const (
	unsettled   = "names no single version here, so what gets installed decides which one"
	hostDecides = "names no single version here, so whatever the host has decides which one"
)

// reason says which of the two a requirement gets.
func reason(runtime bool) string {
	if runtime {
		return hostDecides
	}
	return unsettled
}

// ecosystemOf says where a name is looked up: among the purls of a registry,
// or among the catalog's own names, which is where a runtime is answered.
func ecosystemOf(runtime bool) string {
	if runtime {
		return ""
	}
	return Ecosystem
}

// Matches reports whether a file is one of the manifests this reads.
//
// A requirements file is named by convention rather than by a rule, and
// there are two conventions: a name beginning with requirements, and a
// requirements directory of them, which pip-tools projects keep beside a
// pyproject.toml.
func Matches(path string) bool {
	name := filepath.Base(path)
	if name == "pyproject.toml" {
		return true
	}
	if !strings.HasSuffix(name, ".txt") {
		return false
	}
	return strings.HasPrefix(name, "requirements") ||
		filepath.Base(filepath.Dir(path)) == "requirements"
}

// Extract reads one manifest.
func Extract(path string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	if filepath.Base(path) == "pyproject.toml" {
		// A pyproject.toml may declare its dependencies either way, and a
		// project moving from one to the other has both for a while.
		ds, us := read(path, pyproject(data))
		poetryDs, poetryUs := readPoetry(path, data)
		return append(ds, poetryDs...), append(us, poetryUs...)
	}
	return read(path, requirementsTxt(data))
}

// entry is one requirement the file wrote, and the line it sits on.
//
// runtime says the requirement is on the interpreter rather than on a
// package, which is what requires-python is. The catalog answers a runtime
// by name, as it does for a .python-version, and no purl is involved.
type entry struct {
	text    string
	line    int
	runtime bool
}

// read turns the requirements a file wrote into declarations. Which file
// they came from no longer matters here: PEP 508 is the same grammar in a
// pyproject.toml and a requirements.txt.
func read(path string, entries []entry) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, e := range entries {
		name, specifier, ok := requirement(e.text)
		if !ok {
			continue
		}
		ecosystem := ecosystemOf(e.runtime)
		src := decl.Source{File: path, Line: e.line}
		text := strings.TrimSpace(specifier)
		allowed, ok := allows(specifier)
		if !ok {
			us = append(us, decl.Unreadable{
				Source:    src,
				Ecosystem: ecosystem,
				Product:   name,
				Text:      text,
				Reason:    reason(e.runtime),
				Moving:    true,
			})
			continue
		}
		ds = append(ds, decl.Decl{
			Ecosystem: ecosystem,
			Product:   name,
			Version:   text,
			Allows:    allowed,
			Source:    src,
		})
	}
	return ds, us
}

// requirementsTxt reads the requirements a pip requirements file lists.
//
// A line may be an option rather than a requirement — another file to
// include with -r, an editable install with -e, a hash to check — and pip
// tells them apart by the dash they start with. A requirement may carry
// options after it on the same line, which the same dash ends.
func requirementsTxt(data []byte) []entry {
	var out []entry
	for i, raw := range strings.Split(string(data), "\n") {
		line, _, _ := strings.Cut(raw, "#")
		// A backslash continues a line, and what it carries onto the next
		// one is the requirement's options in every file that writes them —
		// a hash to check — rather than more of the requirement.
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "\\"))
		if before, _, found := strings.Cut(line, " --"); found {
			line = strings.TrimSpace(before)
		}
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		out = append(out, entry{text: line, line: i + 1})
	}
	return out
}

// tables are the members of a pyproject.toml that hold requirements written
// as PEP 508 strings: what the project needs, what its extras need, and what
// its dependency groups need. A group holds what a project needs to develop,
// which goes out of support the same way.
type tables struct {
	Project struct {
		Dependencies []string            `toml:"dependencies"`
		Optional     map[string][]string `toml:"optional-dependencies"`
		Python       string              `toml:"requires-python"`
	} `toml:"project"`
	Groups map[string][]string `toml:"dependency-groups"`
}

// pyproject reads the requirements a pyproject.toml declares.
//
// The file is parsed for what it says and then scanned for where it said it,
// the TOML reader having no line numbers in it. A requirement that cannot be
// located keeps the file without a line, which is the worst the scan can do;
// it never changes what was read.
func pyproject(data []byte) []entry {
	var doc tables
	if _, err := toml.Decode(string(data), &doc); err != nil {
		// The tool that owns the file reports a broken one better than this
		// one can, and half a file is not half a set of declarations.
		return nil
	}
	lines := strings.Split(string(data), "\n")
	from := map[string]int{}
	var out []entry
	if spec := strings.TrimSpace(doc.Project.Python); spec != "" {
		// requires-python writes the specifier on its own, and what it
		// specifies is the interpreter, so it is read as the requirement on
		// python that it means.
		out = append(out, entry{
			text:    "python " + spec,
			line:    lineOf(lines, "requires-python", from),
			runtime: true,
		})
	}
	for _, list := range lists(doc) {
		for _, text := range list {
			out = append(out, entry{text: text, line: lineOf(lines, text, from)})
		}
	}
	return out
}

// lists gathers the requirement lists: what the project needs, then what its
// extras need, then its groups. A mapping has no order of its own, so its
// keys are sorted and a file read twice is read the same way round. Where
// each requirement sits is carried by the line found for it, which is what
// the reader is sent to.
func lists(doc tables) [][]string {
	out := [][]string{doc.Project.Dependencies}
	for _, m := range []map[string][]string{doc.Project.Optional, doc.Groups} {
		for _, key := range slices.Sorted(maps.Keys(m)) {
			out = append(out, m[key])
		}
	}
	return out
}

// lineOf finds the line a requirement was written on, and remembers where to
// carry on looking for the next one written the same way.
//
// Two lists may ask for the same package, and each asks on a line of its
// own, so the second search starts past the first answer. Two requirements
// may equally share a line, an array being allowed to sit on one, so the
// search is kept per requirement rather than per line.
func lineOf(lines []string, text string, from map[string]int) int {
	for i := from[text]; i < len(lines); i++ {
		if strings.Contains(lines[i], text) {
			from[text] = i + 1
			return i + 1
		}
	}
	return 0
}
