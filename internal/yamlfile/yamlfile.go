// Package yamlfile reads the mappings of a YAML document.
//
// Both the files this tool reads as YAML — workflows and Compose files —
// want the same thing: the mappings in the tree, with the line each value
// was written on, and with anchors, aliases and merge keys resolved so that
// a version written once and referred to elsewhere is still read. Keeping
// that in one place also keeps the parser's own types out of the extractors.
package yamlfile

import (
	"slices"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// aliasDepth is how far a chain of aliases is followed before it is given up
// on. The parser refuses to anchor a bare alias, so no document can build a
// chain that long, let alone one that loops; the bound is what makes ending
// a property of this package rather than of what the parser happens to
// accept.
const aliasDepth = 32

// anchor is one definition of a name, and where in the file it was written.
// A name may be defined more than once, and an alias means the last
// definition before it, so where each one sits is part of what it is.
type anchor struct {
	at   int
	node ast.Node
}

// File is what one file held: the documents in it, each with the anchors it
// defines, since a name defined in one says nothing about the same name in
// another.
type File struct {
	docs []document
}

type document struct {
	body    ast.Node
	anchors map[string][]anchor
}

// Parse reads data. A document that does not parse yields a File with
// nothing in it: the tool that owns the file reports a broken one better
// than this one can, and a file mid-edit is not a declaration that could not
// be read.
func Parse(data []byte) *File {
	f := &File{}
	parsed, err := parser.ParseBytes(data, 0)
	if err != nil {
		return f
	}
	for _, src := range parsed.Docs {
		if src.Body == nil {
			continue
		}
		doc := document{body: src.Body, anchors: map[string][]anchor{}}
		collectAnchors(src.Body, doc.anchors)
		// In order, so that the last definition before an alias is the last
		// one anchorFor looks at.
		for _, defs := range doc.anchors {
			slices.SortFunc(defs, func(a, b anchor) int { return a.at - b.at })
		}
		f.docs = append(f.docs, doc)
	}
	return f
}

// collectAnchors records every anchor in the tree, with where it was
// written. An anchor is recorded wherever it sits, including under a key
// that says nothing about what it holds, so that the place it is referred
// from is what gives it meaning.
func collectAnchors(n ast.Node, into map[string][]anchor) {
	switch n := n.(type) {
	case *ast.AnchorNode:
		if name := n.Name.GetToken().Value; name != "" {
			into[name] = append(into[name], anchor{at: n.GetToken().Position.Offset, node: n.Value})
		}
		collectAnchors(n.Value, into)
	case *ast.MappingNode:
		for _, v := range n.Values {
			collectAnchors(v.Value, into)
		}
	case *ast.SequenceNode:
		for _, item := range n.Values {
			collectAnchors(item, into)
		}
	}
}

// Entry is one key and value of a mapping.
type Entry struct {
	Key   string
	Value ast.Node
	doc   *document
}

// Line is the line the value was written on. For a value written as an
// alias, that is the line the alias was written on, which is where the
// declaration is.
func (e Entry) Line() int { return e.Value.GetToken().Position.Line }

// Scalar reads the value as a string.
//
// A version is often written without quotes — node-version: 22, go-version:
// 1.22 — and the parser then calls it a number. What it holds is still the
// text somebody typed, so that text is what comes back: reading 3.10 as a
// number would make it 3.1, which is a different Python that went out of
// support in 2012.
func (e Entry) Scalar() (string, bool) { return scalar(e.doc.resolve(e.Value)) }

func scalar(n ast.Node) (string, bool) {
	switch n := n.(type) {
	case *ast.StringNode:
		return n.Value, true
	case *ast.LiteralNode:
		return n.Value.Value, true
	case *ast.IntegerNode, *ast.FloatNode:
		return n.GetToken().Value, true
	default:
		return "", false
	}
}

// Mapping reads the value as a mapping, with merge keys applied: the keys a
// `<<` brings in are there, and any the mapping states itself win over them,
// which is what YAML means by a merge.
func (e Entry) Mapping() ([]Entry, bool) {
	return e.doc.mapping(e.Value, map[ast.Node]bool{})
}

// Sequence reads the value as a list, following aliases to get there. Each
// item comes back as an entry under the same key it was listed under.
func (e Entry) Sequence() ([]Entry, bool) {
	seq, ok := e.doc.resolve(e.Value).(*ast.SequenceNode)
	if !ok {
		return nil, false
	}
	out := make([]Entry, 0, len(seq.Values))
	for _, item := range seq.Values {
		out = append(out, Entry{Key: e.Key, Value: item, doc: e.doc})
	}
	return out, true
}

// Roots returns the top-level mapping of each document.
func (f *File) Roots() [][]Entry {
	var out [][]Entry
	for i := range f.docs {
		doc := &f.docs[i]
		if entries, ok := doc.mapping(doc.body, map[ast.Node]bool{}); ok {
			out = append(out, entries)
		}
	}
	return out
}

// resolve steps through anchors and aliases to the node carrying the value.
// An alias naming an anchor the document does not have, or a chain that
// loops, comes back as it was and reads as nothing.
func (d *document) resolve(n ast.Node) ast.Node {
	for range aliasDepth {
		switch v := n.(type) {
		case *ast.AnchorNode:
			n = v.Value
		case *ast.AliasNode:
			target, ok := d.anchorFor(v)
			if !ok {
				return n
			}
			n = target
		default:
			return n
		}
	}
	return n
}

// anchorFor finds the definition an alias means: the last one of that name
// written before the alias itself. A name may be defined again further down,
// and YAML says that later definition does not reach back and change what an
// earlier alias meant. An alias with no definition before it has none.
func (d *document) anchorFor(a *ast.AliasNode) (ast.Node, bool) {
	defs := d.anchors[a.Value.GetToken().Value]
	at := a.GetToken().Position.Offset
	for _, def := range slices.Backward(defs) {
		if def.at < at {
			return def.node, true
		}
	}
	return nil, false
}

// mapping turns a node into its entries, following aliases to get there and
// applying any merge keys it holds. open holds the mappings a merge is
// already inside; see merged.
func (d *document) mapping(n ast.Node, open map[ast.Node]bool) ([]Entry, bool) {
	m, ok := d.resolve(n).(*ast.MappingNode)
	if !ok {
		return nil, false
	}
	var own []Entry
	var merged []Entry
	for _, v := range m.Values {
		if v.Key.IsMergeKey() {
			merged = append(merged, d.merged(v.Value, open, m)...)
			continue
		}
		own = append(own, Entry{Key: key(v), Value: v.Value, doc: d})
	}
	if len(merged) == 0 {
		return own, true
	}
	// What the mapping says itself wins over what a merge brought in, and
	// among several merges the first one wins.
	out := own
	seen := map[string]bool{}
	for _, e := range own {
		seen[e.Key] = true
	}
	for _, e := range merged {
		if !seen[e.Key] {
			seen[e.Key] = true
			out = append(out, e)
		}
	}
	return out, true
}

// merged reads what a `<<` in the mapping from brings in.
//
// from is held while its sources are read, so that a template merging
// itself is read once and then let go of rather than followed until the
// stack runs out. Since an alias means a definition above it, a template
// can only reach back to one already begun, which is what makes holding
// the one mapping enough. The guard is on the mapping being merged into and
// not on the sources, because one `<<` may name several and each is its own
// source to read.
func (d *document) merged(n ast.Node, open map[ast.Node]bool, from ast.Node) []Entry {
	if open[from] {
		return nil
	}
	open[from] = true
	defer delete(open, from)
	return d.mergeSources(n, open)
}

// mergeSources reads one mapping, or a list of them, in the order written:
// among several, the first to name a key is the one that brought it in.
//
// What a list holds is mappings, and only mappings, which is also what keeps
// a list holding itself from being followed round for ever.
func (d *document) mergeSources(n ast.Node, open map[ast.Node]bool) []Entry {
	if seq, ok := d.resolve(n).(*ast.SequenceNode); ok {
		var out []Entry
		for _, item := range seq.Values {
			entries, _ := d.mapping(item, open)
			out = append(out, entries...)
		}
		return out
	}
	entries, _ := d.mapping(n, open)
	return entries
}

// Walk hands every mapping in the tree to fn, as a list of entries.
func (f *File) Walk(fn func([]Entry)) {
	for i := range f.docs {
		doc := &f.docs[i]
		doc.walk(doc.body, fn, map[ast.Node]bool{})
	}
}

// walk descends the tree, keeping the nodes it is currently inside so that
// an alias pointing back at one of them ends the descent instead of
// repeating it forever.
func (d *document) walk(n ast.Node, fn func([]Entry), open map[ast.Node]bool) {
	resolved := d.resolve(n)
	if open[resolved] {
		return
	}
	open[resolved] = true
	defer delete(open, resolved)
	switch node := resolved.(type) {
	case *ast.MappingNode:
		entries, _ := d.mapping(node, map[ast.Node]bool{})
		fn(entries)
		for _, v := range node.Values {
			d.walk(v.Value, fn, open)
		}
	case *ast.SequenceNode:
		for _, item := range node.Values {
			d.walk(item, fn, open)
		}
	}
}

func key(v *ast.MappingValueNode) string {
	return strings.ToLower(strings.TrimSpace(v.Key.GetToken().Value))
}

// Find returns the entry with the given key.
func Find(entries []Entry, k string) (Entry, bool) {
	for _, e := range entries {
		if e.Key == k {
			return e, true
		}
	}
	return Entry{}, false
}
