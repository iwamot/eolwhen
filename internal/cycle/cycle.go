// Package cycle matches a declared version against the release cycles a
// product has.
package cycle

import "strings"

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
