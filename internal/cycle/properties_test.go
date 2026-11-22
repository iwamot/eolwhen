package cycle

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/iwamot/eolwhen/internal/span"
)

// The properties below check every version in a small grid against every
// cycle list in a handful, so the whole domain is exercised on each run.
// The segments 1 and 10, and 9 and 10, are there because text and numbers
// disagree on them: "10" starts with "1", and "10" sorts before "9".
var gridSegments = []int{0, 1, 9, 10}

// grid is every version of one up to depth segments drawn from
// gridSegments.
func grid(depth int) [][]int {
	var out [][]int
	var walk func(prefix []int)
	walk = func(prefix []int) {
		if len(prefix) > 0 {
			out = append(out, slices.Clone(prefix))
		}
		if len(prefix) == depth {
			return
		}
		for _, n := range gridSegments {
			walk(append(prefix, n))
		}
	}
	walk(nil)
	return out
}

func spell(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

func spellAll(vs [][]int) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = spell(v)
	}
	return out
}

// cycleLists are shaped like the catalogs a product has: flat majors,
// major.minor lines, lines of mixed depth, cycles nested under one another,
// and names that are words. One list starts at 9 and 10 alone, so its
// oldest is not also first as text.
var cycleLists = [][]string{
	{"10", "9", "1", "0"},
	{"10.1", "10.0", "9.10", "9.9", "9.1", "1.10", "1.9", "1.1"},
	{"10.1", "10.0", "9.10", "9.9"},
	{"1", "1.10", "1.10.1", "9", "9.1"},
	{"10", "9.10", "9.1", "1"},
	{"1.10.1", "1.10.0", "1.9", "1.1"},
	{"10.0", "9.x", "next", "1.0"},
	nil,
}

// probes are versions the grid does not produce: nothing, a leading dot, a
// word, and a word after a number.
var probes = []string{"", ".9", ".10", "latest", "9.x", "9.x.1", "next"}

func versions() []string { return append(spellAll(grid(3)), probes...) }

// numeric reads a cycle as numbers, or reports that a word is in it.
func numeric(s string) ([]int, bool) {
	if s == "" {
		return nil, false
	}
	var out []int
	for part := range strings.SplitSeq(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// Under a cycle means being the cycle or one of the versions it names: the
// cycle's text, followed by nothing or by a dot.
func under(v, c string) bool {
	return c != "" && (v == c || strings.HasPrefix(v, c+"."))
}

func TestMatchIsTheDeepestCycleAVersionIsUnder(t *testing.T) {
	for _, cycles := range cycleLists {
		for _, v := range versions() {
			want := ""
			for _, c := range cycles {
				if under(v, c) && len(c) > len(want) {
					want = c
				}
			}
			got, ok := Match(v, cycles)
			if got != want || ok != (want != "") {
				t.Errorf("Match(%q, %v) = %q, %v; want %q", v, cycles, got, ok, want)
			}
		}
	}
}

func TestCoveringCountsTheCyclesUnderAVersion(t *testing.T) {
	for _, cycles := range cycleLists {
		for _, v := range versions() {
			want := 0
			for _, c := range cycles {
				if c != v && under(c, v) {
					want++
				}
			}
			if got := Covering(v, cycles); got != want {
				t.Errorf("Covering(%q, %v) = %d; want %d", v, cycles, got, want)
			}
		}
	}
}

func TestOldestIsTheLowestNumberedCycle(t *testing.T) {
	for _, cycles := range cycleLists {
		// Nested cycles — 8 and 8.1 — have no lowest by number, so a list
		// with them is left out.
		if slices.ContainsFunc(cycles, func(a string) bool {
			return slices.ContainsFunc(cycles, func(b string) bool { return a != b && under(b, a) })
		}) {
			continue
		}
		// Every order a catalog could list them in gives the same answer.
		reversed := slices.Clone(cycles)
		slices.Reverse(reversed)
		for _, order := range [][]string{cycles, reversed} {
			want, best := "", []int(nil)
			for _, c := range order {
				if n, ok := numeric(c); ok && (best == nil || slices.Compare(n, best) < 0) {
					want, best = c, n
				}
			}
			got, ok := Oldest(order)
			if got != want || ok != (want != "") {
				t.Errorf("Oldest(%v) = %q, %v; want %q", order, got, ok, want)
			}
		}
	}
}

func TestBeforeComparesTheSegmentsBothVersionsHave(t *testing.T) {
	vs := versions()
	for _, a := range vs {
		for _, b := range vs {
			x, okA := numeric(a)
			y, okB := numeric(b)
			// A version that runs out where the other goes on covers it, so
			// only the segments both have are compared.
			n := min(len(x), len(y))
			want := okA && okB && slices.Compare(x[:n], y[:n]) < 0
			if got := Before(a, b); got != want {
				t.Errorf("Before(%q, %q) = %v; want %v", a, b, got, want)
			}
		}
	}
}

// within reads a span the way a requirement means it: from its floor,
// inclusive, up to its ceiling, exclusive, with each bound padded with zeros
// and an unset floor reaching down to nothing.
func within(v []int, s span.Span) bool {
	pad := func(x []int, n int) []int {
		out := slices.Clone(x)
		for len(out) < n {
			out = append(out, 0)
		}
		return out
	}
	below, _ := numeric(s.Below)
	n := max(len(v), len(below))
	if slices.Compare(pad(v, n), pad(below, n)) >= 0 {
		return false
	}
	if s.From == "" {
		return true
	}
	from, _ := numeric(s.From)
	n = max(len(v), len(from))
	return slices.Compare(pad(v, n), pad(from, n)) >= 0
}

func TestSoleIsTheOneCycleAnAllowedVersionIsUnder(t *testing.T) {
	bounds := spellAll(grid(2))
	candidates := spellAll(grid(4))
	for _, cycles := range cycleLists {
		for _, from := range append(bounds, "") {
			for _, below := range bounds {
				s := span.Span{From: from, Below: below}
				var reached []string
				for _, c := range cycles {
					if slices.ContainsFunc(candidates, func(v string) bool {
						n, _ := numeric(v)
						return under(v, c) && within(n, s)
					}) {
						reached = append(reached, c)
					}
				}
				want := ""
				if len(reached) == 1 {
					want = reached[0]
				}
				got, ok := Sole(s, cycles)
				if got != want || ok != (want != "") {
					t.Errorf("Sole(%+v, %v) = %q, %v; want %q (reached %v)", s, cycles, got, ok, want, reached)
				}
			}
		}
	}
}
