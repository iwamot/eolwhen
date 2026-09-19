// Package resolve places declarations on the timeline by looking each one up
// in the catalog. It is pure: the catalog is handed in already decoded.
package resolve

import (
	"sort"
	"strings"

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
	// Ended are the declarations whose cycle upstream says is out of support
	// without saying when. There is no date to place either, but there is
	// plenty for the reader to do, so they are kept apart from Undated
	// rather than reported as a cycle that is merely too new to have a date.
	Ended []timeline.Undated
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
		p, matched, ok := product(c, d.Ecosystem, d.Product)
		if !ok {
			r.Untracked = append(r.Untracked, d)
			continue
		}
		name, ok := reach(p, d)
		if !ok {
			if cycle, release, older := predating(p, d); older {
				r.place(found{p, cycle, d, matched, timeline.DatedByPredating}, release)
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
		r.place(found{p, name, d, matched, timeline.DatedByCycle}, p.Release(name))
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
	bySource(r.Ended, func(u timeline.Undated) decl.Source { return u.Source })
}

// bySource orders a list of set-aside lines by where each one was found.
// Each kind keeps that in a field of its own, so the caller says which.
func bySource[T any](xs []T, at func(T) decl.Source) {
	sort.SliceStable(xs, func(i, j int) bool { return at(xs[i]).Before(at(xs[j])) })
}

func tracked(c *catalog.Catalog, ecosystem, name string) bool {
	_, _, ok := product(c, ecosystem, name)
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
func product(c *catalog.Catalog, ecosystem, name string) (catalog.Product, string, bool) {
	if ecosystem != "" {
		p, ok := c.ByPackage(ecosystem, name)
		return p, timeline.ByPackage, ok
	}
	if p, ok := c.Lookup(name); ok {
		return p, named(p, name), true
	}
	p, ok := c.ByImage(name)
	return p, timeline.ByImage, ok
}

// named tells the catalog's own name for a product from one of the other
// names it answers to. The two share a table, a lookup having no reason to
// care which it was; a reader checking a row does, an alias being upstream's
// word for the software rather than the one this file wrote.
func named(p catalog.Product, name string) string {
	if strings.EqualFold(p.Name, name) {
		return timeline.ByName
	}
	return timeline.ByAlias
}

// predating places a declaration older than every cycle the catalog tracks.
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
// A range answers with the whole of its span rather than with the version it
// starts at. `>= 3.0, < 8.0` begins below the oldest cycle Rails has and
// allows every cycle after it as well, so the day the oldest one ended says
// nothing about it; what a resolver picked is in the lockfile instead.
//
// A product whose cycles are words has no ordering of this kind, and one
// whose oldest cycle has no end date announced yet has no day to carry
// over; both fall through to being reported.
func predating(p catalog.Product, d decl.Decl) (string, catalog.Release, bool) {
	oldest, ok := cycle.Oldest(p.Cycles())
	if !ok || !below(d, oldest) {
		return "", catalog.Release{}, false
	}
	release := p.Release(oldest)
	if _, dated := release.EOL(); !dated {
		return "", catalog.Release{}, false
	}
	return timeline.PredatingCycle(oldest), release, true
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
	r.place(found{p, release.Name, d, timeline.ByCodename, timeline.DatedByCycle}, release)
}

// found is a declaration that reached a release cycle, with what it took to
// get there. It is what lets place say why a row is there as well as that it
// is: the same day, reached through a purl or through an ordering of cycles,
// is not the same claim.
type found struct {
	product catalog.Product
	cycle   string
	from    decl.Decl
	matched string
	dated   string
}

// place files a declaration that reached a cycle: as a finding when the
// cycle has an end date, and, when it has none, by what upstream says about
// the cycle instead.
//
// A cycle with no date is not one situation but two. Upstream may have said
// nothing yet, which is what a current release looks like, or it may have
// said support is over and published no day for it. Both leave nothing to
// put on a timeline and they are opposite in what they ask of the reader, so
// the date alone cannot decide which was meant.
func (r *Result) place(f found, release catalog.Release) {
	eol, ok := release.EOL()
	if !ok {
		reached := timeline.Undated{Product: f.product.Name, Cycle: f.cycle, Source: f.from.Source}
		if release.IsEOL {
			r.Ended = append(r.Ended, reached)
			return
		}
		r.Undated = append(r.Undated, reached)
		return
	}
	r.Findings = append(r.Findings, timeline.Finding{
		Product: f.product.Name,
		Cycle:   f.cycle,
		Version: f.from.Version,
		Matched: f.matched,
		Dated:   f.dated,
		Page:    f.product.Page(),
		EOL:     eol,
		Source:  f.from.Source,
	})
}

// unmatched says why a version reached no cycle, and whether the line was
// following the newest release rather than naming one. Five things are true
// at this point and each leaves the reader somewhere different.
//
// A range whose floor sits at or above its ceiling holds no version at all,
// so nothing was ever going to install and no date belongs to it. That is a
// mistake in the file rather than a line following the newest release, and
// it is answered first: every question below it is about a range something
// can be in.
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
	case d.Allows.Empty():
		return "names a range no version can be in", false
	case d.Allows.Closed():
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

// below reports whether every version a declaration allows is older than the
// oldest cycle the catalog tracks: a range answers with the whole of its
// span, a version with itself.
func below(d decl.Decl, oldest string) bool {
	if d.Allows.Closed() {
		return d.Allows.Precedes(oldest)
	}
	return cycle.Before(d.Version, oldest)
}

// reach finds the cycle a declaration lands in: the one its version belongs
// to, or, when the file pinned a range instead, the one cycle the whole
// range sits inside. A range spanning two cycles reaches neither, because
// which of them gets installed is a resolver's answer and not this one's.
func reach(p catalog.Product, d decl.Decl) (string, bool) {
	if d.Allows.Closed() {
		return cycle.Sole(d.Allows, p.Cycles())
	}
	return match(p, d.Version)
}
