// Package resolve places declarations on the timeline by looking each one up
// in the catalog. It is pure: the catalog is handed in already decoded.
package resolve

import (
	"github.com/iwamot/eolwhen/internal/catalog"
	"github.com/iwamot/eolwhen/internal/cycle"
	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/timeline"
)

// Result is the answer for one directory: what landed on the timeline, and
// what was read but could not be placed on it.
type Result struct {
	Findings   []timeline.Finding
	Unreadable []decl.Unreadable
	// Untracked are the declarations naming software endoflife.date has no
	// product for. They are kept rather than forgotten so --verbose can
	// account for them, and left out of the default output because there was
	// never a date to find.
	Untracked []decl.Decl
	// Undated are the declarations whose cycle was found but has no end date
	// announced. They are kept and left out for the same reason: there is no
	// date to place, and nothing for the reader to do about it.
	Undated []timeline.Undated
}

// All resolves every declaration, and sorts the lines the extractors could
// not read by the same rule.
//
// A declaration with no date to place is set aside rather than reported, and
// there are two ways to have none. Software the catalog does not track is
// one: a file that lists tools — mise.toml, .tool-versions — names plenty
// that have no published end-of-life policy at all, and saying so for each
// would bury the answer under things that were never going to have a date.
// That holds whatever the version looked like: a `jq = "latest"` is a line
// about jq before it is a line about latest, and the extractor that met it
// could not know whether jq is tracked. A cycle that has no end date
// announced is the other: the line is right, the cycle is there, and support
// has not been dated yet, which is what a current version looks like.
// --verbose asks for both anyway.
//
// What is reported is the line resolution could not follow at all: no
// release cycle covers the version. There the software is known and the
// version is not one it ever had, so the line itself is what to go and look
// at.
//
// The lines the extractors could not read come first in the result: they
// were found earlier in the file than anything resolution had an opinion
// about. With no declarations and no unreadable line that names a product,
// the catalog is never consulted, so it may be nil.
func All(c *catalog.Catalog, ds []decl.Decl, us []decl.Unreadable) Result {
	var r Result
	for _, u := range us {
		if u.Product != "" {
			if _, ok := c.Lookup(u.Product); !ok {
				r.Untracked = append(r.Untracked, decl.Decl{Product: u.Product, Version: u.Text, Source: u.Source})
				continue
			}
		}
		r.Unreadable = append(r.Unreadable, u)
	}
	for _, d := range ds {
		p, ok := c.Lookup(d.Product)
		if !ok {
			r.Untracked = append(r.Untracked, d)
			continue
		}
		name, ok := match(p, d.Version)
		if !ok {
			r.Unreadable = append(r.Unreadable, decl.Unreadable{
				Source:  d.Source,
				Product: p.Name,
				Text:    d.Version,
				Reason:  unmatched(p, d.Version),
			})
			continue
		}
		eol, ok := p.Release(name).EOL()
		if !ok {
			r.Undated = append(r.Undated, timeline.Undated{
				Product: p.Name,
				Cycle:   name,
				Source:  d.Source,
			})
			continue
		}
		r.Findings = append(r.Findings, timeline.Finding{
			Product: p.Name,
			Cycle:   name,
			EOL:     eol,
			Source:  d.Source,
		})
	}
	return r
}

// unmatched says why a version reached no cycle. Three things are true at
// this point and each leaves the reader somewhere different.
//
// A version that starts with a letter, of a product that numbers its cycles,
// named a variant or an alias of some version rather than a version: alpine
// and slim are builds of whatever the current release is. Codenames are
// already gone by here, and the runner images name their own cycles with
// letters, so neither is caught by this.
//
// A version that covers cycles without being one named a major line, which
// is a rule for following the newest of them rather than a version. It gets
// no date, because the date it would get moves on its own.
//
// Anything else is a version the catalog has never had.
func unmatched(p catalog.Product, v string) string {
	switch {
	case p.Numbered() && (v == "" || v[0] < '0' || v[0] > '9'):
		return "names a variant or an alias, not a version"
	case cycle.Covering(v, p.Cycles()) > 0:
		return "names a major line rather than a release cycle, so it follows the newest in that line"
	default:
		return "no release cycle covers this version"
	}
}

// match finds the cycle a declared version belongs to, by number and then by
// codename. The codename pass is what reads `FROM debian:bookworm`, where
// the tag names a cycle without giving a number; it costs nothing elsewhere,
// since a version number matches no codename.
func match(p catalog.Product, v string) (string, bool) {
	if name, ok := cycle.Match(v, p.Cycles()); ok {
		return name, true
	}
	if r, ok := p.ByCodename(v); ok {
		return r.Name, true
	}
	return "", false
}
