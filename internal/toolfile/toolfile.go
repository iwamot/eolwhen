// Package toolfile reads the files that list a directory's tools and the
// version pinned for each: mise.toml and .tool-versions.
//
// These differ from the single-runtime files in one way that matters. A
// .nvmrc can only be about Node.js, but a tool list names whatever the
// project uses — biome, hugo, gh, jq — and most of those publish no
// end-of-life policy at all. The names are read as written and handed on;
// deciding which of them anything is known about is the catalog's job.
//
// A key may carry a backend — aqua:, go:, npm:, pipx: — and then it names a
// package rather than a tool. The backend is what fixes the registry, which
// is what lets the name be answered the way a Gemfile's is: through the
// purls upstream publishes for that registry and through nothing else. A
// backend that names no registry — an asdf plugin, a download from a URL —
// is passed over, there being no table to answer its names from.
package toolfile

import (
	"cmp"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Matches reports whether a file name is a tool list.
func Matches(name string) bool {
	return name == "mise.toml" || name == ".mise.toml" || name == ".tool-versions"
}

// Extract reads one tool list.
func Extract(path string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	if filepath.Base(path) == ".tool-versions" {
		return toolVersions(path, data)
	}
	return miseToml(path, data)
}

// miseToml reads the [tools] table.
//
// The file is parsed for what it says and then scanned for where it said it.
// A version that cannot be located keeps the file without a line, which is
// the worst the scan can do; it never changes what was read.
func miseToml(path string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var doc struct {
		Tools map[string]toml.Primitive `toml:"tools"`
	}
	md, err := toml.Decode(string(data), &doc)
	if err != nil {
		// mise reports a broken config better than this tool can.
		return nil, nil
	}
	lines := lineOf(data)
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, key := range inFileOrder(doc.Tools, lines) {
		ecosystem, name, ok := backend(key)
		if !ok {
			continue
		}
		src := decl.Source{File: path, Line: lines[key]}
		for _, v := range versionsOf(md, doc.Tools[key]) {
			d, u, ok := read(ecosystem, name, v, src)
			if ok {
				ds = append(ds, d)
			} else {
				us = append(us, u)
			}
		}
	}
	return ds, us
}

// versionsOf reads the shapes a mise tool version takes: one string, a list
// of them, or a table with the version among its options.
func versionsOf(md toml.MetaData, p toml.Primitive) []string {
	var one string
	if err := md.PrimitiveDecode(p, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := md.PrimitiveDecode(p, &many); err == nil {
		return many
	}
	var table struct {
		Version string `toml:"version"`
	}
	if err := md.PrimitiveDecode(p, &table); err == nil && table.Version != "" {
		return []string{table.Version}
	}
	return nil
}

// toolVersions reads asdf's format: one tool and its versions per line.
func toolVersions(path string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for i, raw := range strings.Split(string(data), "\n") {
		line, _, _ := strings.Cut(raw, "#")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		if strings.Contains(name, ":") {
			// asdf reads a plugin name and nothing else, so a colon here is
			// not a backend and names nothing this can look up.
			continue
		}
		src := decl.Source{File: path, Line: i + 1}
		for _, v := range fields[1:] {
			d, u, ok := read("", name, v, src)
			if ok {
				ds = append(ds, d)
			} else {
				us = append(us, u)
			}
		}
	}
	return ds, us
}

// backends are the mise backends whose key names a package, and the purl
// type that package belongs to. aqua, github and ubi install from a GitHub
// release and name the repository it comes from, which is what
// endoflife.date publishes as a github purl; the rest name a registry of
// their own.
//
// The backends left out are the ones with no registry to look a name up in:
// an asdf or vfox plugin, a download from a URL or a bucket, a forge no purl
// type covers.
var backends = map[string]string{
	"aqua":   "github",
	"github": "github",
	"ubi":    "github",
	"cargo":  "cargo",
	"conda":  "conda",
	"dotnet": "nuget",
	"gem":    "gem",
	"go":     "golang",
	"npm":    "npm",
	"pipx":   "pypi",
	"spm":    "swift",
}

// backend reads a tool list key: the registry its name belongs to, and the
// name. A key with no backend names a tool, whose identity the name settles
// on its own. ok is false for a backend with no registry to read from, and
// for one with no name after it.
func backend(key string) (ecosystem, name string, ok bool) {
	prefix, rest, found := strings.Cut(key, ":")
	if !found {
		return "", key, true
	}
	if rest == "" {
		return "", "", false
	}
	// mise's core tools are the ones a bare name already reaches, so the
	// backend says nothing about the name: core:node is node.
	if prefix == "core" {
		return "", rest, true
	}
	ecosystem, known := backends[prefix]
	if !known {
		return "", "", false
	}
	return ecosystem, rest, true
}

// read turns one tool and one version into a declaration, or into the reason
// it is not one.
func read(ecosystem, name, v string, src decl.Source) (decl.Decl, decl.Unreadable, bool) {
	version := strings.TrimPrefix(v, "v")
	if strings.Contains(version, ":") {
		// ref:main, prefix:1.27, sub-2:lts — each defers the choice to
		// whatever mise resolves it to at install time.
		version = ""
	}
	if version == "" || version[0] < '0' || version[0] > '9' {
		// The tool is named apart from the version, so that whether the
		// catalog tracks it can still decide if this line is worth a word.
		r, moving := reason(v)
		return decl.Decl{}, decl.Unreadable{Source: src, Ecosystem: ecosystem, Product: name, Text: v, Reason: r, Moving: moving}, false
	}
	return decl.Decl{Ecosystem: ecosystem, Product: name, Version: version, Source: src}, decl.Unreadable{}, true
}

// reason says why a version could not be used, and whether the line names no
// fixed version by design: each of these but the last leaves the choice to
// something other than the file, so there was never a date to place.
func reason(v string) (string, bool) {
	switch {
	case v == "latest", v == "lts", v == "stable", strings.HasPrefix(v, "lts-"):
		return "names a moving target, not a version", true
	case v == "system":
		return "defers to whatever is installed", true
	case strings.Contains(v, ":"):
		return "leaves the version for mise to resolve", true
	default:
		return "is not a version", false
	}
}

// lineOf finds the line each tool name was written on, by looking inside the
// [tools] table for a key with that name. It decorates the answer and never
// decides it: a name it cannot find keeps line 0, which prints as the file
// alone.
func lineOf(data []byte) map[string]int {
	out := map[string]int{}
	inTools := false
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") {
			inTools = line == "[tools]"
			continue
		}
		if !inTools {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		if key != "" {
			out[key] = i + 1
		}
	}
	return out
}

// inFileOrder orders the tool names the way the file wrote them, which is
// the order every other file is read in, so that what is said about a tool
// list reads down the file like the rest. A map has no order of its own, and
// a name whose line could not be found comes after the ones that could, by
// name, so the output never moves between runs.
func inFileOrder(m map[string]toml.Primitive, lines map[string]int) []string {
	at := func(name string) int {
		if l := lines[name]; l != 0 {
			return l
		}
		return math.MaxInt
	}
	out := slices.Collect(maps.Keys(m))
	slices.SortFunc(out, func(a, b string) int {
		return cmp.Or(cmp.Compare(at(a), at(b)), strings.Compare(a, b))
	})
	return out
}
