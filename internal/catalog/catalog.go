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

// Identifier is a name upstream publishes for a product in somebody else's
// namespace: a purl, a CPE, a repology name. The purls of type docker are
// the ones this tool reads, because they say which image on Docker Hub is
// which software — upstream's own answer to the question an image name
// outside the official library cannot answer on its own.
type Identifier struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Product is one piece of software with its release cycles. Aliases are the
// other names upstream accepts for it, which is how `node` reaches nodejs
// and `alpine` reaches alpine-linux without a table of our own.
type Product struct {
	Name        string       `json:"name"`
	Label       string       `json:"label"`
	Aliases     []string     `json:"aliases"`
	Identifiers []Identifier `json:"identifiers"`
	Releases    []Release    `json:"releases"`
}

// images lists the Docker Hub repositories this product publishes, as its
// purls of type docker name them. A purl may carry a version or qualifiers
// after the name, and neither is part of the repository.
func (p Product) images() []string {
	var out []string
	for _, id := range p.Identifiers {
		if id.Type != "purl" {
			continue
		}
		name, ok := strings.CutPrefix(id.ID, "pkg:docker/")
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(name, "@")
		name, _, _ = strings.Cut(name, "?")
		name, _, _ = strings.Cut(name, "#")
		if name != "" {
			out = append(out, strings.ToLower(name))
		}
	}
	return out
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
// to and by every codename its cycles carry.
type Catalog struct {
	products   []Product
	byName     map[string]int
	byCodename map[string]codename
	byImage    map[string]int
}

// codename is where a cycle's codename leads: the product and the release,
// and whether the word belongs to one of them alone.
type codename struct {
	product int
	release int
	sole    bool
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
	c := &Catalog{
		products:   doc.Result,
		byName:     make(map[string]int, len(doc.Result)*2),
		byCodename: map[string]codename{},
		byImage:    map[string]int{},
	}
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
	// An image is indexed under the repository each of its purls names. The
	// first product to claim a repository keeps it: upstream owns these
	// strings, so two products naming one image is its own bug and not an
	// ambiguity to resolve here.
	for i, p := range doc.Result {
		for _, image := range p.images() {
			if _, seen := c.byImage[image]; !seen {
				c.byImage[image] = i
			}
		}
	}
	// A codename is indexed across the whole catalog, and a word two
	// products both use is marked as belonging to neither.
	for i, p := range doc.Result {
		for j, r := range p.Releases {
			tag := r.tag()
			if tag == "" {
				continue
			}
			was, seen := c.byCodename[tag]
			switch {
			case !seen:
				c.byCodename[tag] = codename{product: i, release: j, sole: true}
			case was.sole && was.product != i:
				c.byCodename[tag] = codename{}
			}
		}
	}
	return c, nil
}

// ByCodename finds the product and cycle a codename names, across the whole
// catalog. It reads the variant an image tag carries — the -bookworm in
// python:3.12-bookworm — where the word names the software as well as the
// version, because only Debian calls a release bookworm.
//
// A word two products share answers neither: which one a tag meant would be
// a guess, and a codename is worth reading precisely because it needs none.
func (c *Catalog) ByCodename(s string) (Product, Release, bool) {
	at, ok := c.byCodename[strings.ToLower(s)]
	if !ok || !at.sole {
		return Product{}, Release{}, false
	}
	p := c.products[at.product]
	return p, p.Releases[at.release], true
}

// Lookup finds a product by its name or any alias, ignoring case.
func (c *Catalog) Lookup(name string) (Product, bool) {
	i, ok := c.byName[strings.ToLower(name)]
	if !ok {
		return Product{}, false
	}
	return c.products[i], true
}

// ByImage finds the product a Docker Hub repository holds, as endoflife.date
// itself names it: opensearchproject/opensearch is OpenSearch because
// upstream publishes pkg:docker/opensearchproject/opensearch for it, not
// because the name reads that way.
//
// This is the whole of what is known about a name outside the official
// library. An image nobody published a purl for is one whose contents are
// not knowable from the line, and that is a declaration of software the
// catalog does not track rather than a line to go and look at.
func (c *Catalog) ByImage(name string) (Product, bool) {
	i, ok := c.byImage[strings.ToLower(name)]
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
