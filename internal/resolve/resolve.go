// Package resolve places declarations on the timeline by looking each one up
// in the catalog. It is pure: the catalog is handed in already decoded.
package resolve

import (
	"sort"

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
	// Moving are the lines that name no fixed version by design: a :latest
	// tag, an ubuntu-latest runner, a tag naming only a major line. They
	// follow the newest release on purpose, so there is no date to place and
	// nothing to go and change, and --verbose accounts for them like the
	// rest.
	Moving []decl.Unreadable
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
// A line that follows the newest release on purpose is the third: a :latest
// tag, an ubuntu-latest runner, a tag naming a major line. Nothing is wrong
// with it either, and there is no fixed cycle behind it to date, so it is
// set aside with the other two.
//
// What is reported is the line resolution could not follow at all: no
// release cycle covers the version. There the software is known and the
// version is not one it ever had, so the line itself is what to go and look
// at.
//
// Everything set aside is ordered by where it was found, so --verbose reads
// down the directory rather than in the order the two passes happened to
// gather it. With no declarations and no unreadable line that names a
// product, the catalog is never consulted, so it may be nil.
func All(c *catalog.Catalog, ds []decl.Decl, us []decl.Unreadable) Result {
	var r Result
	for _, u := range us {
		if u.Product != "" && !tracked(c, u.Ecosystem, u.Product) {
			r.Untracked = append(r.Untracked, decl.Decl{
				Ecosystem: u.Ecosystem,
				Product:   u.Product,
				Version:   u.Text,
				Source:    u.Source,
			})
			continue
		}
		r.add(u)
	}
	for _, d := range ds {
		if d.Product == "" {
			r.codename(c, d)
			continue
		}
		p, ok := product(c, d.Ecosystem, d.Product)
		if !ok {
			r.Untracked = append(r.Untracked, d)
			continue
		}
		name, ok := reach(p, d)
		if !ok {
			if cycle, release, older := predating(p, lowest(d)); older {
				r.place(p.Name, cycle, release, d.Source)
				continue
			}
			reason, moving := unmatched(p, d)
			r.add(decl.Unreadable{
				Source:    d.Source,
				Ecosystem: d.Ecosystem,
				Product:   p.Name,
				Text:      d.Version,
				Reason:    reason,
				Moving:    moving,
			})
			continue
		}
		r.place(p.Name, name, p.Release(name), d.Source)
	}
	r.order()
	return r
}

// add files a line that reached no cycle: as one to go and look at, or, when
// it was following the newest release on purpose, as one with no date to
// place and nothing to do about it.
func (r *Result) add(u decl.Unreadable) {
	if u.Moving {
		r.Moving = append(r.Moving, u)
		return
	}
	r.Unreadable = append(r.Unreadable, u)
}

// order puts the lines with no row on them in the order they sit in the
// directory. They are gathered in two passes — what an extractor could not
// read, and then what resolution had an opinion about — so a Gemfile's line
// 4 would otherwise be printed before its line 3. A reader following
// --verbose down a file should not have to jump back. Findings are ordered
// by their date instead, which the timeline does.
func (r *Result) order() {
	bySource(r.Unreadable, func(u decl.Unreadable) decl.Source { return u.Source })
	bySource(r.Moving, func(u decl.Unreadable) decl.Source { return u.Source })
	bySource(r.Untracked, func(d decl.Decl) decl.Source { return d.Source })
	bySource(r.Undated, func(u timeline.Undated) decl.Source { return u.Source })
}

// bySource orders a list of set-aside lines by where each one was found.
// Each kind keeps that in a field of its own, so the caller says which.
func bySource[T any](xs []T, at func(T) decl.Source) {
	sort.SliceStable(xs, func(i, j int) bool { return at(xs[i]).Before(at(xs[j])) })
}

func tracked(c *catalog.Catalog, ecosystem, name string) bool {
	_, ok := product(c, ecosystem, name)
	return ok
}

// product finds what a declaration is about.
//
// A package name reaches a product through the purls upstream publishes for
// that ecosystem and through nothing else. Names and aliases are the wrong
// table for it: `mongo` is an alias of MongoDB and `pg` of PostgreSQL, and
// both are also the names of the drivers a manifest depends on, which are
// different software with different versions. A registry anyone can publish
// to is not a namespace to read names out of, so only upstream's own answer
// counts.
//
// Anything else is a name the catalog answers to, or the Docker Hub
// repository a product publishes under. The second is what reaches software
// outside the official library — opensearchproject/opensearch is OpenSearch
// because endoflife.date says so — and it costs nothing elsewhere, since no
// other kind of name is written with a slash.
func product(c *catalog.Catalog, ecosystem, name string) (catalog.Product, bool) {
	if ecosystem != "" {
		return c.ByPackage(ecosystem, name)
	}
	if p, ok := c.Lookup(name); ok {
		return p, true
	}
	return c.ByImage(name)
}

// predating places a version older than every cycle the catalog tracks.
//
// A redis 3.2 in a Compose file is the most neglected line in the
// directory, and reading it as a version nobody has heard of is the one
// answer that is certainly wrong: endoflife.date starts at 4.0 because
// everything below it stopped being a going concern long ago. So it gets a
// row, and the row carries the day the oldest tracked cycle ended, which
// support for anything older had already run out by. The cycle reads <4.0,
// so that the row says what it knows — out of support by this date — and
// not a day it cannot know.
//
// A product whose cycles are words has no ordering of this kind, and one
// whose oldest cycle has no end date announced yet has no day to carry
// over; both fall through to being reported.
func predating(p catalog.Product, v string) (string, catalog.Release, bool) {
	oldest, ok := cycle.Oldest(p.Cycles())
	if !ok || !cycle.Before(v, oldest) {
		return "", catalog.Release{}, false
	}
	release := p.Release(oldest)
	if _, dated := release.EOL(); !dated {
		return "", catalog.Release{}, false
	}
	return "<" + oldest, release, true
}

// codename places a declaration whose version names its software as well:
// the -bookworm an image tag carries, which says Debian and 12 in one word.
//
// A word the catalog has no codename for is dropped without any word of its
// own. It was a build variant — slim, fpm, jre — and not a declaration read
// wrong, so there is nothing to account for and nothing to report: the tag
// segments an image carries are not all versions, and only the catalog can
// say which of them are.
func (r *Result) codename(c *catalog.Catalog, d decl.Decl) {
	p, release, ok := c.ByCodename(d.Version)
	if !ok {
		return
	}
	r.place(p.Name, release.Name, release, d.Source)
}

// place files a declaration that reached a cycle: as a finding when the
// cycle has an end date, and as undated when it has none.
func (r *Result) place(product, cycle string, release catalog.Release, src decl.Source) {
	eol, ok := release.EOL()
	if !ok {
		r.Undated = append(r.Undated, timeline.Undated{
			Product: product,
			Cycle:   cycle,
			Source:  src,
		})
		return
	}
	r.Findings = append(r.Findings, timeline.Finding{
		Product: product,
		Cycle:   cycle,
		EOL:     eol,
		Source:  src,
	})
}

// unmatched says why a version reached no cycle, and whether the line was
// following the newest release rather than naming one. Four things are true
// at this point and each leaves the reader somewhere different.
//
// A range that sits inside no one cycle names no version to date. Which
// version a manifest's range became is a resolver's answer, written in the
// lockfile beside it, so the line follows whatever was resolved and there is
// nothing here to place.
//
// A version that starts with a letter, of a product that numbers its cycles,
// named a variant or an alias of some version rather than a version: alpine
// and slim are builds of whatever the current release is, so the line is
// moving by design. Codenames are already gone by here, and the runner
// images name their own cycles with letters, so neither is caught by this.
//
// A version that covers cycles without being one named a major line, which
// is a rule for following the newest of them rather than a version. It gets
// no date, because the date it would get moves on its own, and it is moving
// for the same reason.
//
// Anything else is a version the catalog has never had, and that line is one
// to go and look at. A version older than every cycle it tracks is not
// among them: predating has already placed that one on the timeline.
func unmatched(p catalog.Product, d decl.Decl) (reason string, moving bool) {
	v := d.Version
	switch {
	case d.Below != "":
		return "names a range of versions rather than one, so the lockfile decides which one", true
	case p.Numbered() && (v == "" || v[0] < '0' || v[0] > '9'):
		return "names a variant or an alias, not a version", true
	case cycle.Covering(v, p.Cycles()) > 0:
		return "names a major line rather than a release cycle, so it follows the newest in that line", true
	default:
		return "no release cycle covers this version", false
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

// reach finds the cycle a declaration lands in: the one its version belongs
// to, or, when the file pinned a range instead, the one cycle the whole
// range sits inside. A range spanning two cycles reaches neither, because
// which of them gets installed is a resolver's answer and not this one's.
func reach(p catalog.Product, d decl.Decl) (string, bool) {
	if d.Below != "" {
		return cycle.Sole(d.From, d.Below, p.Cycles())
	}
	return match(p, d.Version)
}

// lowest is the version a declaration starts at, which is the whole of it
// unless it named a range.
func lowest(d decl.Decl) string {
	if d.From != "" {
		return d.From
	}
	return d.Version
}
