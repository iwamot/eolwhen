// Package cycle matches a declared version against the release cycles a
// product has.
package cycle

import (
	"strconv"
	"strings"

	"github.com/iwamot/eolwhen/internal/span"
)

// Match returns the longest cycle whose dot-separated segments are a prefix
// of v's.
//
// Comparing segments rather than the string is the whole point. Python has
// both a 3.1 and a 3.10 cycle, so strings.HasPrefix("3.10.2", "3.1") is true
// and would file a 3.10 declaration under a cycle that went end-of-life in
// 2012. Split first and the second segment, "10" against "1", settles it.
//
// Longest wins because a product can have cycles at more than one depth: Go
// declares 1.21 and 1.22 while Node.js declares 20 and 22, and a version like
// 1.21.5 must reach 1.21 rather than stopping at any shorter cycle that also
// fits.
func Match(v string, cycles []string) (string, bool) {
	want := segments(v)
	if len(want) == 0 {
		return "", false
	}
	best := ""
	bestLen := 0
	for _, c := range cycles {
		got := segments(c)
		if len(got) == 0 || len(got) > len(want) {
			continue
		}
		if !equal(want[:len(got)], got) {
			continue
		}
		if len(got) > bestLen {
			best, bestLen = c, len(got)
		}
	}
	return best, bestLen > 0
}

// segments splits a version on dots. An empty string, or one that splits to
// an empty first segment, has no segments, so it matches nothing.
func segments(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	if parts[0] == "" {
		return nil
	}
	return parts
}

func equal(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Covering counts the cycles that fall under v, which is the other direction
// from Match: v's segments are a prefix of the cycle's rather than the other
// way round.
//
// A tag naming only a major line — redis:7, python-version: 3 — reaches no
// cycle, because it is not a version but a rule for following one: the image
// moves to 7.6 the day it exists. Counting what it would cover is how that is
// told apart from a version the catalog has simply never heard of.
func Covering(v string, cycles []string) int {
	want := segments(v)
	if len(want) == 0 {
		return 0
	}
	n := 0
	for _, c := range cycles {
		got := segments(c)
		if len(got) <= len(want) {
			continue
		}
		if equal(got[:len(want)], want) {
			n++
		}
	}
	return n
}

// Oldest is the lowest cycle a product numbers, compared as numbers rather
// than as text so that 2.7 comes before 3.10. A product that names its
// cycles with words — the runner images name theirs macos-15 — has no oldest
// of this kind, and neither has one with no cycles at all.
func Oldest(cycles []string) (string, bool) {
	oldest, best := "", []int(nil)
	for _, c := range cycles {
		got, ok := numbers(c)
		if !ok {
			continue
		}
		if best == nil || less(got, best) {
			oldest, best = c, got
		}
	}
	return oldest, best != nil
}

// Before reports whether a is a lower version than b, segment by segment and
// as numbers. Anything either side spells with a word is not compared at
// all, and a version that only runs out — 1.2 against 1.2.3 — is not before
// it: it covers it.
func Before(a, b string) bool {
	x, okA := numbers(a)
	y, okB := numbers(b)
	if !okA || !okB {
		return false
	}
	return less(x, y)
}

func less(x, y []int) bool {
	for i := range min(len(x), len(y)) {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return false
}

// numbers reads a version as the numbers it is made of. A segment that is
// not a number makes the whole of it unreadable this way, which is what
// keeps a codename or a runner label out of an ordering that would mean
// nothing for it.
func numbers(s string) ([]int, bool) {
	parts := segments(s)
	if len(parts) == 0 {
		return nil, false
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// Sole returns the one cycle that every version a requirement allows falls
// into, when the range reaches exactly one.
//
// A manifest pins a range more often than a version: `gem "rails", "~> 6.1.0"`
// allows every 6.1.x, and which of them is installed is decided by a
// resolver this tool does not run. A range is still an answer when the whole
// of it sits inside one cycle, because a row is about a cycle and not about
// a version — every version `~> 6.1.0` allows is Rails 6.1, and Rails 6.1
// has an end-of-life date. A range that reaches two cycles has no single
// date behind it and gets no row.
func Sole(s span.Span, cycles []string) (string, bool) {
	found, n := "", 0
	for _, c := range cycles {
		// A cycle holds a line of versions rather than one version: 6.1
		// covers every 6.1.x, so it runs from 6.1 up to where 6.2 begins.
		// The two ranges meet when each starts before the other ends.
		end, ok := span.Next(c)
		if !ok {
			continue
		}
		if s.Meets(c, end) {
			found, n = c, n+1
		}
	}
	if n != 1 {
		return "", false
	}
	return found, true
}
