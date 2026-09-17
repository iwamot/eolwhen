package gemfile

import (
	"strconv"
	"strings"

	"github.com/iwamot/eolwhen/internal/cycle"
)

// span reads what a gem's requirements allow: the lowest version they admit,
// and the first version past them.
//
// ok is false when the requirements leave the upper end open, or name
// something this does not read. There is no range to place then, and no
// version either: a gem allowed to be anything from 6.0 upwards will be
// whatever the resolver picked, which is written in the lockfile and not
// here. An exclusion — != 6.1 — is the same answer from the other side, as
// it takes a version out of the middle and narrows neither end. A
// pre-release — 7.1.0.rc1 — reads as none of these, because its ordering
// against a release is Bundler's rule and not a number's.
func span(reqs []string) (lo, hi string, ok bool) {
	if len(reqs) == 0 {
		return "", "", false
	}
	lo = "0"
	for _, req := range reqs {
		op, text := split(req)
		v, read := numbers(text)
		if !read {
			return "", "", false
		}
		switch op {
		case "", "=":
			lo, hi = higher(lo, text), lower(hi, join(bump(v)))
		// A bound that excludes its own version and one that admits it
		// differ by a single version, which is never the version that
		// decides a cycle.
		case ">=", ">":
			lo = higher(lo, text)
		case "<":
			hi = lower(hi, text)
		case "<=":
			hi = lower(hi, join(bump(v)))
		case "~>":
			lo, hi = higher(lo, text), lower(hi, join(pessimistic(v)))
		default:
			return "", "", false
		}
	}
	if hi == "" {
		return "", "", false
	}
	return lo, hi, true
}

// split reads a requirement as its operator and its version. A requirement
// with no operator is an exact version, which is what a Gemfile written from
// a lockfile looks like.
func split(req string) (op, v string) {
	req = strings.TrimSpace(req)
	for _, o := range []string{"~>", ">=", "<=", "!=", ">", "<", "="} {
		if rest, found := strings.CutPrefix(req, o); found {
			return o, strings.TrimSpace(rest)
		}
	}
	return "", req
}

// numbers reads a version as the numbers it is made of, which is the only
// shape the arithmetic and the ordering here can work on. A segment that is
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

// join writes a version back out after the arithmetic above has moved it.
func join(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// higher is the greater of two lower bounds, which is the one that holds
// when several requirements each set a floor.
func higher(a, b string) string {
	if cycle.Lower(a, b) {
		return b
	}
	return a
}

// lower is the lesser of two upper bounds, where an empty bound is no bound
// at all and so loses to any other.
func lower(a, b string) string {
	if a == "" || cycle.Lower(b, a) {
		return b
	}
	return a
}

// bump is the first version after one exact version: 6.1.7.6 is followed by
// 6.1.7.7. It turns a bound that admits its version into one that does not.
func bump(v []int) []int {
	out := append([]int(nil), v...)
	out[len(out)-1]++
	return out
}

// pessimistic is where `~> v` stops: the operator lets the last segment
// given move and holds everything before it, so `~> 6.1.0` stops at 6.2 and
// `~> 6.1` at 7. A single segment has nothing before it to hold, and `~> 6`
// stops at 7 for the same reason a major line does.
func pessimistic(v []int) []int {
	if len(v) > 1 {
		v = v[:len(v)-1]
	}
	return bump(v)
}
