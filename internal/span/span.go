// Package span holds the ends of the range of versions a requirement allows.
//
// A manifest usually pins a range rather than a version, and each writes
// its ranges in its own operators — a Gemfile's `~> 6.1.0`, a
// composer.json's `^8.1`. What they have in common is the arithmetic
// underneath: a floor, a ceiling, and the rule for where the line a version
// names runs out. An operator a file invents for itself stays with that
// file; the comparisons they all write the same way are here, along with
// what every operator reduces to.
package span

import (
	"strconv"
	"strings"
)

// Span is the versions a requirement allows: From is the lowest it admits,
// and Below the first it does not. Either is empty until something sets it,
// and an end nothing has set is no bound at all: a requirement with only a
// floor leaves Below empty, and one with only a ceiling leaves From empty.
// The zero Span is therefore the one nothing has narrowed, which is where
// reading a requirement starts.
type Span struct {
	From  string
	Below string
}

// Closed reports whether anything has closed the upper end. An open one
// names no single version and no single cycle either, because a package
// allowed to be anything from 6.0 upwards will be whatever the resolver
// picked.
func (s Span) Closed() bool { return s.Below != "" }

// AtLeast raises the floor to v, and leaves it alone when it already sits
// higher: several requirements each setting a floor leave the highest.
func (s Span) AtLeast(v string) Span {
	if s.From == "" || Lower(s.From, v) {
		s.From = v
	}
	return s
}

// Under lowers the ceiling to v, and leaves it alone when it already sits
// lower. No ceiling at all loses to any other.
func (s Span) Under(v string) Span {
	if s.Below == "" || Lower(v, s.Below) {
		s.Below = v
	}
	return s
}

// Narrow applies one of the comparisons every manifest writes the same way:
// a bare version or `=` for one version, and `>=`, `>`, `<` and `<=` for an
// end. ok is false for anything else, each manifest's own operators being
// its own to read.
//
// A bound that excludes its own version and one that admits it differ by a
// single version, which is never the version that decides a cycle, so `>`
// is read as `>=`.
func (s Span) Narrow(op, v string) (Span, bool) {
	switch op {
	case "", "=":
		next, ok := Next(v)
		if !ok {
			return s, false
		}
		return s.AtLeast(v).Under(next), true
	case ">=", ">":
		return s.AtLeast(v), true
	case "<":
		return s.Under(v), true
	case "<=":
		next, ok := Next(v)
		if !ok {
			return s, false
		}
		return s.Under(next), true
	}
	return s, false
}

// Meets reports whether this span and the versions from lo up to below have
// any in common, which is true when each range starts before the other ends.
// An end nothing has set stops nothing.
func (s Span) Meets(lo, below string) bool {
	if s.From != "" && !Lower(s.From, below) {
		return false
	}
	if s.Below != "" && !Lower(lo, s.Below) {
		return false
	}
	return true
}

// Precedes reports whether every version this span allows is below v, which
// is what a span older than anything a catalog tracks looks like. An upper
// end nothing has closed reaches past any version and precedes nothing, and
// the floor says nothing about it: a span may start below v and carry on
// well past it.
func (s Span) Precedes(v string) bool {
	return s.Below != "" && !Lower(v, s.Below)
}

// Next is the version after v: 6.1.7.6 is followed by 6.1.7.7. It turns a
// bound that admits its own version into one that does not, which is what
// an exact requirement and a `<=` both need.
func Next(v string) (string, bool) {
	parts, ok := numbers(v)
	if !ok {
		return "", false
	}
	return bump(parts, len(parts)-1), true
}

// AfterLine is where the line v names runs out when its last segment is
// free to move, which is what a tilde means: Ruby's `~> 6.1.0` stops at 6.2
// and Composer's `~8.1.0` at 8.2, while `~> 6.1` stops at 7 and `~8.1` at 9.
// A version of one segment has nothing in front to hold, so `~> 6` stops at
// 7.
func AfterLine(v string) (string, bool) {
	parts, ok := numbers(v)
	if !ok {
		return "", false
	}
	return bump(parts, max(len(parts)-2, 0)), true
}

// AfterMinor is where the line v names runs out when the minor is the last
// segment held, which is what npm's tilde means: ~1.2.3 and ~1.2 both stop
// at 1.3, while ~1 has no minor to hold and stops at 2.
//
// It parts from AfterLine at two segments and nowhere else. Composer reads
// ~1.2 as holding the major alone and stops at 2, npm as holding the minor
// and stops at 1.3, and the same three characters mean different things in
// the two files.
func AfterMinor(v string) (string, bool) {
	parts, ok := numbers(v)
	if !ok {
		return "", false
	}
	return bump(parts, min(1, len(parts)-1)), true
}

// AfterCompatible is where the line v names runs out when everything after
// its first meaningful segment is free to move, which is what a caret
// means: `^8.1` stops at 9. A leading zero says nothing about
// compatibility, so the first segment that is not zero is the one held, and
// `^0.3.0` stops at 0.4 rather than at 1.
func AfterCompatible(v string) (string, bool) {
	parts, ok := numbers(v)
	if !ok {
		return "", false
	}
	at := 0
	for at < len(parts)-1 && parts[at] == 0 {
		at++
	}
	return bump(parts, at), true
}

// Numeric reports whether v is made of numbers alone, which is the only
// shape the arithmetic here can work on. A pre-release — 7.1.0.rc1 — is
// not, because where it falls against a release is the package manager's
// rule rather than a number's.
func Numeric(v string) bool {
	_, ok := numbers(v)
	return ok
}

// Lower compares two bounds, reading the shorter as if it were padded with
// zeros: 6.1 as a bound is 6.1.0, so it is neither above nor below it. That
// is what a bound means, and it is not how two versions are ordered when one
// merely runs out where the other carries on.
func Lower(a, b string) bool {
	x, okA := numbers(a)
	y, okB := numbers(b)
	if !okA || !okB {
		return false
	}
	for i := range max(len(x), len(y)) {
		p, q := segment(x, i), segment(y, i)
		if p != q {
			return p < q
		}
	}
	return false
}

// bump raises parts[at] by one and drops whatever followed it, so the
// result is the first version past the line those segments name.
func bump(parts []int, at int) string {
	out := make([]string, at+1)
	for i := range at {
		out[i] = strconv.Itoa(parts[i])
	}
	out[at] = strconv.Itoa(parts[at] + 1)
	return strings.Join(out, ".")
}

// numbers reads a version as the numbers it is made of. A segment that is
// not a number, or is too large to be one, makes the whole of it unreadable.
func numbers(v string) ([]int, bool) {
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// segment is the number at index i, reading a version as if it carried
// zeros past its end, which is what comparing two bounds of unequal length
// needs.
func segment(s []int, i int) int {
	if i < len(s) {
		return s[i]
	}
	return 0
}
