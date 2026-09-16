package catalog

import (
	"testing"
	"time"
)

const doc = `{
  "schema_version": "1.2.0",
  "result": [
    {"name":"python","label":"Python","aliases":[],"releases":[
      {"name":"3.13","eolFrom":"2029-10-31"},
      {"name":"2.7","eolFrom":"2020-01-01"}
    ]},
    {"name":"nodejs","label":"Node.js","aliases":["node"],"releases":[
      {"name":"24","eolFrom":"2028-04-30"},
      {"name":"26","eolFrom":null}
    ]},
    {"name":"alpine-linux","label":"Alpine Linux","aliases":["alpine","alpinelinux"],"releases":[
      {"name":"3.10","eolFrom":"2021-05-01"}
    ]}
  ]
}`

func decode(t *testing.T) *Catalog {
	t.Helper()
	c, err := Decode([]byte(doc))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return c
}

func TestDecodeAndLen(t *testing.T) {
	if got := decode(t).Len(); got != 3 {
		t.Errorf("Len = %d; want 3", got)
	}
}

func TestDecodeErrors(t *testing.T) {
	for _, tt := range []struct{ name, data, want string }{
		{"not json", "{", "decoding"},
		{"no products", `{"result":[]}`, "no products"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decode([]byte(tt.data)); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestLookup(t *testing.T) {
	c := decode(t)
	for _, tt := range []struct {
		query string
		want  string
		ok    bool
	}{
		{"python", "python", true},
		{"nodejs", "nodejs", true},
		{"node", "nodejs", true},         // an alias
		{"alpine", "alpine-linux", true}, // the alias that fills the gap
		{"ALPINE", "alpine-linux", true}, // case does not matter
		{"Node", "nodejs", true},
		{"mongo", "", false}, // not in this catalog
		{"", "", false},
	} {
		got, ok := c.Lookup(tt.query)
		if ok != tt.ok || got.Name != tt.want {
			t.Errorf("Lookup(%q) = %q, %v; want %q, %v", tt.query, got.Name, ok, tt.want, tt.ok)
		}
	}
}

// TestLookupPrefersName pins the two-pass indexing: were a product's alias to
// collide with another product's real name, the real name answers.
func TestLookupPrefersName(t *testing.T) {
	c, err := Decode([]byte(`{"result":[
	  {"name":"one","aliases":["two"],"releases":[]},
	  {"name":"two","aliases":[],"releases":[]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, _ := c.Lookup("two"); got.Name != "two" {
		t.Errorf("Lookup(two) = %q; want two", got.Name)
	}
}

func TestCyclesAndRelease(t *testing.T) {
	c := decode(t)
	p, _ := c.Lookup("python")
	want := []string{"3.13", "2.7"}
	got := p.Cycles()
	if len(got) != len(want) {
		t.Fatalf("Cycles = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Cycles[%d] = %q; want %q", i, got[i], want[i])
		}
	}
	if r := p.Release("2.7"); r.EOLFrom != "2020-01-01" {
		t.Errorf("Release(2.7).EOLFrom = %q; want 2020-01-01", r.EOLFrom)
	}
	// A cycle the product does not have comes back as the zero Release,
	// which announces no date rather than crashing.
	if _, ok := p.Release("4.0").EOL(); ok {
		t.Error("Release(4.0) should announce no date")
	}
}

func TestReleaseEOL(t *testing.T) {
	for _, tt := range []struct {
		name string
		from string
		want string
		ok   bool
	}{
		{"a date", "2020-01-01", "2020-01-01", true},
		{"none announced", "", "", false},
		{"unparseable is treated as none", "soon", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Release{Name: "x", EOLFrom: tt.from}.EOL()
			if ok != tt.ok {
				t.Fatalf("EOL() ok = %v; want %v", ok, tt.ok)
			}
			if ok && got.Format(time.DateOnly) != tt.want {
				t.Errorf("EOL() = %s; want %s", got.Format(time.DateOnly), tt.want)
			}
		})
	}
}

func TestByCodename(t *testing.T) {
	c, err := Decode([]byte(`{"result":[
	  {"name":"ubuntu","aliases":[],"releases":[
	    {"name":"24.04","codename":"Noble Numbat","eolFrom":"2029-05-31"},
	    {"name":"18.04","codename":"Bionic Beaver","eolFrom":"2023-05-31"}
	  ]},
	  {"name":"debian","aliases":[],"releases":[
	    {"name":"12","codename":"Bookworm","eolFrom":"2028-06-30"}
	  ]},
	  {"name":"python","aliases":[],"releases":[
	    {"name":"3.13","eolFrom":"2029-10-31"}
	  ]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for _, tt := range []struct {
		product  string
		codename string
		want     string
		ok       bool
	}{
		// Ubuntu names a cycle with two words and tags it with the first.
		{"ubuntu", "noble", "24.04", true},
		{"ubuntu", "bionic", "18.04", true},
		{"ubuntu", "Bionic", "18.04", true},
		// Debian's is one word to begin with.
		{"debian", "bookworm", "12", true},
		// The second word is not the tag.
		{"ubuntu", "numbat", "", false},
		// A product with no codenames matches nothing, which is what keeps
		// this pass free for every other file.
		{"python", "3.13", "", false},
		{"ubuntu", "", "", false},
		{"ubuntu", "trixie", "", false},
	} {
		p, _ := c.Lookup(tt.product)
		got, ok := p.ByCodename(tt.codename)
		if ok != tt.ok || got.Name != tt.want {
			t.Errorf("%s.ByCodename(%q) = %q, %v; want %q, %v", tt.product, tt.codename, got.Name, ok, tt.want, tt.ok)
		}
	}
}

// TestNumbered separates the products that name their cycles with numbers
// from the runner images, which name theirs with words. What a declaration
// starting with a letter can have meant depends on which kind it is.
func TestNumbered(t *testing.T) {
	c, err := Decode([]byte(`{"result":[
	  {"name":"nginx","aliases":[],"releases":[{"name":"1.29"},{"name":"1.28"}]},
	  {"name":"runners","aliases":[],"releases":[{"name":"macos-15"},{"name":"windows-2025"}]},
	  {"name":"nothing","aliases":[],"releases":[]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for _, tt := range []struct {
		product string
		want    bool
	}{
		{"nginx", true},
		{"runners", false},
		{"nothing", false},
	} {
		p, _ := c.Lookup(tt.product)
		if got := p.Numbered(); got != tt.want {
			t.Errorf("%s.Numbered() = %v; want %v", tt.product, got, tt.want)
		}
	}
}
