// Package catalog reads the endoflife.date product catalog.
//
// The API is beta and says breaking changes can happen, so only the fields
// this tool needs are decoded and everything else is ignored. Dates are kept
// as the strings the document carried and parsed when a release is actually
// looked up, which keeps a malformed date on a product nobody asked about
// from affecting the run.
package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// URL is the whole catalog in one document: every product, its names, and
// every release cycle with its end-of-life date.
const URL = "https://endoflife.date/api/v1/products/full"

// Release is one release cycle of a product. EOLFrom is the date support
// ends, as written, and is empty when none has been announced. Codename is
// the name the cycle also goes by, which the Debian family uses as its image
// tag and everyone else leaves empty.
type Release struct {
	Name     string `json:"name"`
	EOLFrom  string `json:"eolFrom"`
	Codename string `json:"codename"`
}

// tag is the codename as an image tag: lowercased, and cut to its first
// word, because Ubuntu names a cycle "Noble Numbat" and tags it noble while
// Debian names one "Trixie" and tags it trixie.
func (r Release) tag() string {
	name, _, _ := strings.Cut(strings.ToLower(r.Codename), " ")
	return name
}

// EOL parses EOLFrom. ok is false when no date was announced, and also when
// the date does not parse, because either way there is no day to place on
// the timeline.
func (r Release) EOL() (t time.Time, ok bool) {
	if r.EOLFrom == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.DateOnly, r.EOLFrom)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// Product is one piece of software with its release cycles. Aliases are the
// other names upstream accepts for it, which is how `node` reaches nodejs
// and `alpine` reaches alpine-linux without a table of our own.
type Product struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Aliases  []string  `json:"aliases"`
	Releases []Release `json:"releases"`
}

// Cycles lists the cycle names, for matching a declared version.
func (p Product) Cycles() []string {
	out := make([]string, 0, len(p.Releases))
	for _, r := range p.Releases {
		out = append(out, r.Name)
	}
	return out
}

// ByCodename finds the cycle whose codename is written as s, which is how
// `FROM debian:bookworm` and `FROM ubuntu:jammy` name a version without
// giving a number. A product with no codenames matches nothing.
func (p Product) ByCodename(s string) (Release, bool) {
	s = strings.ToLower(s)
	if s == "" {
		return Release{}, false
	}
	for _, r := range p.Releases {
		if r.Codename != "" && r.tag() == s {
			return r, true
		}
	}
	return Release{}, false
}

// Release returns the cycle by name, or the zero Release when there is none.
// The zero value announces no end-of-life date, which is how a caller that
// hands back a name Cycles did not produce is answered rather than crashed.
func (p Product) Release(cycle string) Release {
	for _, r := range p.Releases {
		if r.Name == cycle {
			return r
		}
	}
	return Release{}
}

// Catalog is the decoded document, indexed by every name a product answers
// to.
type Catalog struct {
	products []Product
	byName   map[string]int
}

type document struct {
	Result []Product `json:"result"`
}

// Decode reads the catalog document. A product is indexed under its name and
// each of its aliases, lowercased, so a lookup is case-insensitive.
func Decode(data []byte) (*Catalog, error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decoding the endoflife.date catalog: %w", err)
	}
	if len(doc.Result) == 0 {
		return nil, fmt.Errorf("the endoflife.date catalog came back with no products")
	}
	c := &Catalog{products: doc.Result, byName: make(map[string]int, len(doc.Result)*2)}
	// Aliases go in first and names second, so a product's own name always
	// wins over another product's alias for the same string.
	for i, p := range doc.Result {
		for _, a := range p.Aliases {
			if a != "" {
				c.byName[strings.ToLower(a)] = i
			}
		}
	}
	for i, p := range doc.Result {
		if p.Name != "" {
			c.byName[strings.ToLower(p.Name)] = i
		}
	}
	return c, nil
}

// Lookup finds a product by its name or any alias, ignoring case.
func (c *Catalog) Lookup(name string) (Product, bool) {
	i, ok := c.byName[strings.ToLower(name)]
	if !ok {
		return Product{}, false
	}
	return c.products[i], true
}

// Len is the number of products, for the note that says how many were read.
func (c *Catalog) Len() int { return len(c.products) }

// Numbered reports whether this product names its cycles with numbers, as
// nearly every product does. The runner images are the exception — their
// cycles are macos-15 and windows-2025 — and knowing which kind a product is
// settles what a declaration that starts with a letter can have meant.
func (p Product) Numbered() bool {
	for _, r := range p.Releases {
		if r.Name != "" && r.Name[0] >= '0' && r.Name[0] <= '9' {
			return true
		}
	}
	return false
}
