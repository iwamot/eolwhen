package pythonmanifest

import (
	"maps"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/span"
)

// poetryDecl is one dependency a Poetry table names. constraint is empty
// when the entry names no version of its own — a dependency taken from a
// repository or a path, or one written as a version per interpreter.
type poetryDecl struct {
	name       string
	constraint string
	line       int
}

// poetryDoc is the shape of the tables Poetry keeps its dependencies in. A
// group holds what a project needs to develop, which goes out of support the
// same way.
type poetryDoc struct {
	Tool struct {
		Poetry struct {
			Dependencies map[string]toml.Primitive `toml:"dependencies"`
			Group        map[string]struct {
				Dependencies map[string]toml.Primitive `toml:"dependencies"`
			} `toml:"group"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

// readPoetry reads the dependencies a pyproject.toml declares Poetry's way.
//
// Poetry writes a dependency as a key rather than as a PEP 508 string, and
// asks for versions in operators of its own: a caret and a tilde that mean
// what npm's mean, beside the comparisons every manifest shares.
func readPoetry(path string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, p := range poetry(data) {
		src := decl.Source{File: path, Line: p.line}
		allowed, ok := poetryAllows(p.constraint)
		if !ok {
			us = append(us, decl.Unreadable{
				Source:    src,
				Ecosystem: Ecosystem,
				Product:   p.name,
				Text:      p.constraint,
				Reason:    unsettled,
				Moving:    true,
			})
			continue
		}
		ds = append(ds, decl.Decl{
			Ecosystem: Ecosystem,
			Product:   p.name,
			Version:   p.constraint,
			Allows:    allowed,
			Source:    src,
		})
	}
	return ds, us
}

// poetry reads every dependency of every Poetry table. The file is parsed
// for what it says and then scanned for where it said it, the TOML reader
// having no line numbers in it.
//
// The python key is left alone. Poetry reserves it for the interpreters the
// project accepts, which is requires-python under another name and says what
// the project accepts rather than what it runs on.
func poetry(data []byte) []poetryDecl {
	var doc poetryDoc
	md, err := toml.Decode(string(data), &doc)
	if err != nil {
		return nil
	}
	lines := poetryLines(data)
	var out []poetryDecl
	for _, t := range poetryDependencies(doc) {
		for _, key := range slices.Sorted(maps.Keys(t.deps)) {
			if key == "python" {
				continue
			}
			out = append(out, poetryDecl{
				name:       key,
				constraint: poetryConstraint(md, t.deps[key]),
				line:       lines[t.name+"."+key],
			})
		}
	}
	return out
}

// poetryTable is one dependency table: the name it is written under, which
// is what finds its lines, and what it holds.
type poetryTable struct {
	name string
	deps map[string]toml.Primitive
}

// poetryDependencies lists the dependency tables a file holds: the project's
// own, then each group's. A mapping has no order of its own, so the groups
// are sorted and a file read twice is read the same way round.
func poetryDependencies(doc poetryDoc) []poetryTable {
	p := doc.Tool.Poetry
	out := []poetryTable{{name: "tool.poetry.dependencies", deps: p.Dependencies}}
	for _, group := range slices.Sorted(maps.Keys(p.Group)) {
		out = append(out, poetryTable{
			name: "tool.poetry.group." + group + ".dependencies",
			deps: p.Group[group].Dependencies,
		})
	}
	return out
}

// poetryConstraint reads the version a dependency asks for, which Poetry
// writes as a string or as the version member of a table.
//
// Anything else names no one version: a dependency taken from a repository,
// a path or a URL, and one written as a list of constraints, each for a
// different interpreter. Reading part of those would be answering a question
// only an install can.
func poetryConstraint(md toml.MetaData, p toml.Primitive) string {
	var one string
	if err := md.PrimitiveDecode(p, &one); err == nil {
		return one
	}
	var t struct {
		Version string `toml:"version"`
	}
	if err := md.PrimitiveDecode(p, &t); err == nil {
		return t.Version
	}
	return ""
}

// poetryLines finds the line each dependency key sits on, keyed by the table
// it sits in, so that the same package asked for in two groups is sent to
// the line each group wrote.
func poetryLines(data []byte) map[string]int {
	out := map[string]int{}
	table := ""
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if header, ok := strings.CutPrefix(line, "["); ok {
			table, _, _ = strings.Cut(header, "]")
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		if key != "" {
			out[table+"."+key] = i + 1
		}
	}
	return out
}

// poetryAllows reads what a Poetry constraint lets a package be: the lowest
// version it admits, and the first version past it.
//
// ok is false when the constraint leaves the upper end open, or is a union,
// which leaves two ranges behind rather than one. A stability marker after
// an at sign asks for something a number does not order, and a constraint
// that is no version at all — a bare *, or the empty one an entry with no
// version of its own reaches here with — names nothing to place.
func poetryAllows(constraint string) (span.Span, bool) {
	if strings.Contains(constraint, "||") || strings.Contains(constraint, "@") {
		return span.Span{}, false
	}
	// A comma separates one clause from the next, and every one of them has
	// to hold at once.
	fields := strings.FieldsFunc(constraint, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return span.Span{}, false
	}
	var s span.Span
	for _, field := range fields {
		op, v := poetrySplit(field)
		if line, wild := strings.CutSuffix(v, ".*"); wild {
			// A wildcard segment names a line of versions rather than a
			// version, which is a range already: 1.2.* runs from 1.2 to
			// 1.3. An operator in front of one is not Poetry.
			if op != "" || !span.Numeric(line) {
				return span.Span{}, false
			}
			ceiling, _ := span.Next(line)
			s = s.AtLeast(line).Under(ceiling)
			continue
		}
		if !span.Numeric(v) {
			return span.Span{}, false
		}
		switch op {
		// Poetry's caret and tilde are npm's: a caret holds the first
		// segment that says anything about compatibility, and a tilde holds
		// the minor where one is given.
		case "^":
			ceiling, _ := span.AfterCompatible(v)
			s = s.AtLeast(v).Under(ceiling)
		case "~":
			ceiling, _ := span.AfterMinor(v)
			s = s.AtLeast(v).Under(ceiling)
		default:
			narrowed, ok := s.Narrow(comparison(op), v)
			if !ok {
				return span.Span{}, false
			}
			s = narrowed
		}
	}
	if !s.Closed() {
		return span.Span{}, false
	}
	return s, true
}

// poetrySplit reads one clause as its operator and its version. A clause
// with no operator is a version on its own, which Poetry reads as that one
// version.
func poetrySplit(field string) (op, v string) {
	for _, o := range []string{"==", "!=", ">=", "<=", "^", "~", ">", "<"} {
		if rest, found := strings.CutPrefix(field, o); found {
			return o, rest
		}
	}
	return "", field
}
