package yamlfile

import (
	"testing"

	"github.com/goccy/go-yaml/ast"
)

func collect(data string) map[string][]string {
	got := map[string][]string{}
	Parse([]byte(data)).Walk(func(entries []Entry) {
		for _, e := range entries {
			if s, ok := e.Scalar(); ok {
				got[e.Key] = append(got[e.Key], s)
			}
		}
	})
	return got
}

func TestWalk(t *testing.T) {
	body := "a: one\n" +
		"b:\n" +
		"  c: two\n" +
		"list:\n" +
		"  - d: three\n" +
		"  - d: four\n" +
		"block: |\n" +
		"  five\n" +
		"  six\n"
	got := collect(body)
	if len(got["a"]) != 1 || got["a"][0] != "one" {
		t.Errorf("a = %v; want [one]", got["a"])
	}
	if len(got["c"]) != 1 || got["c"][0] != "two" {
		t.Errorf("c = %v; want [two]", got["c"])
	}
	// Mappings inside a sequence are reached too, which is how a workflow's
	// steps are read.
	if len(got["d"]) != 2 {
		t.Errorf("d = %v; want two of them", got["d"])
	}
	// A block scalar is a separate kind of node holding the same string.
	if len(got["block"]) != 1 || got["block"][0] != "five\nsix\n" {
		t.Errorf("block = %q; want the two lines", got["block"])
	}
	// Keys are lowercased, so a file that shouts still matches.
	if len(collect("KEY: value\n")["key"]) != 1 {
		t.Error("a shouted key should still be found")
	}
}

// TestUnquotedNumbers: a version written without quotes is a number to the
// parser and text to everyone else. 3.10 must come back as 3.10, because
// reading it as a number makes it 3.1 — a Python that went out of support in
// 2012.
func TestUnquotedNumbers(t *testing.T) {
	got := collect("a: 22\nb: 1.22\nc: 3.10\nd: 3.0\ne: '3.10'\n")
	for key, want := range map[string]string{"a": "22", "b": "1.22", "c": "3.10", "d": "3.0", "e": "3.10"} {
		if len(got[key]) != 1 || got[key][0] != want {
			t.Errorf("%s = %v; want [%s]", key, got[key], want)
		}
	}
}

// TestAnchors: an anchor wraps the node that carries the value, both when it
// is a mapping to be walked into and when it is the value itself.
func TestAnchors(t *testing.T) {
	got := collect("x: &tpl\n  image: postgres:11\ny:\n  image: &img redis:5\n")
	if len(got["image"]) != 2 {
		t.Fatalf("image = %v; want both of them", got["image"])
	}
	if got["image"][0] != "postgres:11" || got["image"][1] != "redis:5" {
		t.Errorf("image = %v; want postgres:11 and redis:5", got["image"])
	}
}

// TestWalkNotYAML: the tool that owns the file reports a broken one better
// than this one can, so nothing comes back and nothing is said.
func TestWalkNotYAML(t *testing.T) {
	for _, body := range []string{"\tthis: is: not: yaml\n  - [\n", ""} {
		n := 0
		Parse([]byte(body)).Walk(func([]Entry) { n++ })
		if n != 0 {
			t.Errorf("Walk(%q) visited %d mappings; want none", body, n)
		}
	}
}

func TestLineAndScalar(t *testing.T) {
	Parse([]byte("a: one\nb: two\n")).Walk(func(entries []Entry) {
		e, ok := Find(entries, "b")
		if !ok {
			t.Fatal("b not found")
		}
		if e.Line() != 2 {
			t.Errorf("Line = %d; want 2", e.Line())
		}
		if _, ok := Find(entries, "missing"); ok {
			t.Error("Find should not invent an entry")
		}
	})
}

// TestScalarOfAList: a value that is not a string is left to the caller,
// which is how an image: or a runs-on: written as a list is noticed.
func TestScalarOfAList(t *testing.T) {
	Parse([]byte("a: [1, 2]\n")).Walk(func(entries []Entry) {
		if _, ok := entries[0].Scalar(); ok {
			t.Error("a list should not read as a scalar")
		}
	})
}

// TestAliasToAnAnchorElsewhere: an anchor may sit under a key that says
// nothing about what it holds — a workflow's env: block — and the alias that
// refers to it is where the declaration is. Following it is what keeps the
// value from being dropped without a word.
func TestAliasToAnAnchorElsewhere(t *testing.T) {
	got := collect("env:\n  PYTHON_VERSION: &python '2.7'\nwith:\n  python-version: *python\n")
	if len(got["python-version"]) != 1 || got["python-version"][0] != "2.7" {
		t.Errorf("python-version = %v; want [2.7]", got["python-version"])
	}
}

// TestUnresolvedAlias: an alias naming an anchor the document does not have
// reads as nothing rather than as something invented.
func TestUnresolvedAlias(t *testing.T) {
	if v := collect("a: *missing\n")["a"]; len(v) != 0 {
		t.Errorf("a = %v; want nothing", v)
	}
}

// TestMerge applies a merge key the way YAML does: what the mapping says
// itself wins over what the merge brought in.
func TestMerge(t *testing.T) {
	f := Parse([]byte("x-d: &d\n  image: python:2.7\n  restart: always\nsvc:\n  <<: *d\n  image: python:3.13\n"))
	roots := f.Roots()
	if len(roots) != 1 {
		t.Fatalf("Roots = %d; want 1", len(roots))
	}
	svc, ok := Find(roots[0], "svc")
	if !ok {
		t.Fatal("svc not found")
	}
	entries, ok := svc.Mapping()
	if !ok {
		t.Fatal("svc is not a mapping")
	}
	img, _ := Find(entries, "image")
	if v, _ := img.Scalar(); v != "python:3.13" {
		t.Errorf("image = %q; want python:3.13, the value the mapping states itself", v)
	}
	// What only the merge brought in is there too.
	restart, ok := Find(entries, "restart")
	if !ok {
		t.Fatal("restart did not come through the merge")
	}
	if v, _ := restart.Scalar(); v != "always" {
		t.Errorf("restart = %q; want always", v)
	}
}

// TestMergeSequence: several templates merged at once, the first winning.
func TestMergeSequence(t *testing.T) {
	f := Parse([]byte("a: &a\n  image: python:2.7\nb: &b\n  image: python:3.13\nsvc:\n  <<: [*a, *b]\n"))
	svc, _ := Find(f.Roots()[0], "svc")
	entries, _ := svc.Mapping()
	img, _ := Find(entries, "image")
	if v, _ := img.Scalar(); v != "python:2.7" {
		t.Errorf("image = %q; want python:2.7, the first merge", v)
	}
}

// TestCyclesStillRead: a document that refers to itself is read for what it
// does say, rather than followed round. Whether it finishes at all is not
// something a test in this process can decide — a stack overflow is fatal
// and no timeout here would survive it — so that is checked by running the
// binary, in e2e.
func TestCyclesStillRead(t *testing.T) {
	for _, tt := range []struct{ name, body, key, want string }{
		{"a template merging itself",
			"a: &x\n  <<: *x\nsvc:\n  <<: *x\n  image: python:2.7\n", "image", "python:2.7"},
		// Nothing comes of merging a name defined below: an alias means a
		// definition above it, which is also why two templates cannot merge
		// each other and only a template merging itself can come round.
		{"a merge naming a template defined below it",
			"a: &x\n  <<: *y\nb: &y\n  <<: *x\nsvc:\n  <<: *x\n  image: python:2.7\n", "image", "python:2.7"},
		{"a list holding itself, merged",
			"a: &x [*x]\nsvc:\n  <<: *x\n  image: python:2.7\n", "image", "python:2.7"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse([]byte(tt.body))
			svc, ok := Find(f.Roots()[0], "svc")
			if !ok {
				t.Fatal("svc not found")
			}
			entries, _ := svc.Mapping()
			e, ok := Find(entries, tt.key)
			if !ok {
				t.Fatalf("%s not found in %+v", tt.key, entries)
			}
			if v, _ := e.Scalar(); v != tt.want {
				t.Errorf("%s = %q; want %q", tt.key, v, tt.want)
			}
		})
	}
}

// TestSequence: a list comes back as entries under the key it was listed
// under, and a single value is not a list, which is how a caller reading
// runs-on tells the two shapes apart.
func TestSequence(t *testing.T) {
	f := Parse([]byte("runs-on: [self-hosted, macos-13]\none: macos-13\n"))
	root := f.Roots()[0]
	list, _ := Find(root, "runs-on")
	items, ok := list.Sequence()
	if !ok || len(items) != 2 {
		t.Fatalf("Sequence = %v, %v; want two items", items, ok)
	}
	if v, _ := items[1].Scalar(); v != "macos-13" {
		t.Errorf("items[1] = %q; want macos-13", v)
	}
	if items[0].Key != "runs-on" {
		t.Errorf("Key = %q; want the key the list was under", items[0].Key)
	}
	// A single value is not a list, which is how the caller tells them apart.
	single, _ := Find(root, "one")
	if _, ok := single.Sequence(); ok {
		t.Error("a scalar should not read as a list")
	}
	if _, ok := single.Mapping(); ok {
		t.Error("a scalar should not read as a mapping")
	}
}

// TestAliasChainEnds: an alias that leads back to itself is given up on
// rather than followed.
//
// The parser this is built on refuses to anchor a bare alias — `a: &x *y` is
// a syntax error — so no document can put resolution in a loop today. The
// bound is what makes that a property of this package rather than of the
// parser's current behaviour, so the loop is built here by hand, out of a
// real alias node, and the guard is exercised on it.
func TestAliasChainEnds(t *testing.T) {
	f := Parse([]byte("anchor: &x 1\nuse: *x\n"))
	use, ok := Find(f.Roots()[0], "use")
	if !ok {
		t.Fatal("use not found")
	}
	alias, ok := use.Value.(*ast.AliasNode)
	if !ok {
		t.Fatalf("use is %T; want an alias", use.Value)
	}
	// Point the anchor the alias names back at the alias itself, defined
	// before the alias so that it is the one found.
	f.docs[0].anchors["x"] = []anchor{{at: 0, node: alias}}

	// Without the bound this never returns, and the test times out rather
	// than failing; with it, the loop is given up on and reads as nothing.
	if v, ok := use.Scalar(); ok {
		t.Errorf("a loop read as %q; want nothing", v)
	}
}

// TestAnchorRedefinition: YAML lets a name be defined more than once, and an
// alias means the last definition written before it. A definition added
// further down does not reach back and change what an earlier alias meant.
func TestAnchorRedefinition(t *testing.T) {
	body := "old: &img python:2.7\n" +
		"before: *img\n" +
		"new: &img python:3.13\n" +
		"after: *img\n"
	got := collect(body)
	if len(got["before"]) != 1 || got["before"][0] != "python:2.7" {
		t.Errorf("before = %v; want [python:2.7], the definition above it", got["before"])
	}
	if len(got["after"]) != 1 || got["after"][0] != "python:3.13" {
		t.Errorf("after = %v; want [python:3.13], the definition above it", got["after"])
	}
}

// TestForwardReference: an alias with no definition before it has none. The
// one further down is not reached back for.
func TestForwardReference(t *testing.T) {
	got := collect("early: *img\nlate: &img python:3.13\n")
	if len(got["early"]) != 0 {
		t.Errorf("early = %v; want nothing", got["early"])
	}
	if len(got["late"]) != 1 || got["late"][0] != "python:3.13" {
		t.Errorf("late = %v; want [python:3.13]", got["late"])
	}
}

// TestAnchorsDoNotCrossDocuments: a name defined in one document says
// nothing about the same name in another.
func TestAnchorsDoNotCrossDocuments(t *testing.T) {
	f := Parse([]byte("---\none: &img python:2.7\nuse: *img\n---\ntwo: *img\n"))
	roots := f.Roots()
	if len(roots) != 2 {
		t.Fatalf("Roots = %d; want 2", len(roots))
	}
	first, _ := Find(roots[0], "use")
	if v, _ := first.Scalar(); v != "python:2.7" {
		t.Errorf("use = %q; want python:2.7", v)
	}
	second, _ := Find(roots[1], "two")
	if v, ok := second.Scalar(); ok {
		t.Errorf("two = %q; want nothing, the name being another document's", v)
	}
}

// TestRedefinitionKeepsTheReferenceLine: the row points at where the version
// was referred from, which is the line to go and change.
func TestRedefinitionKeepsTheReferenceLine(t *testing.T) {
	f := Parse([]byte("old: &img python:2.7\nuse: *img\nnew: &img python:3.13\n"))
	use, _ := Find(f.Roots()[0], "use")
	if use.Line() != 2 {
		t.Errorf("Line = %d; want 2, the alias", use.Line())
	}
}

// TestWalkStopsAtANodeItIsInside: a mapping holding an alias to itself is
// walked once. The alias points backwards, as every alias does, so without
// the guard the descent would come round to where it started.
func TestWalkStopsAtANodeItIsInside(t *testing.T) {
	n := 0
	Parse([]byte("a: &x\n  b: *x\n  image: python:2.7\n")).Walk(func(entries []Entry) {
		n++
		if n > 100 {
			t.Fatal("the walk came round again")
		}
	})
	// The one mapping under a:, and the document's own.
	if n != 2 {
		t.Errorf("visited %d mappings; want 2", n)
	}
}
