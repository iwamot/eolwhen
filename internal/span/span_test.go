package span

import "testing"

func TestNarrowing(t *testing.T) {
	tests := []struct {
		name       string
		s          Span
		from, next string
		closed     bool
	}{
		{"a floor and a ceiling", Span{}.AtLeast("6.0").Under("7"), "6.0", "7", true},
		{"the highest floor holds", Span{}.AtLeast("6.0").AtLeast("6.5").Under("7"), "6.5", "7", true},
		{"a lower floor does not lower it", Span{}.AtLeast("6.5").AtLeast("6.0").Under("7"), "6.5", "7", true},
		{"the lowest ceiling holds", Span{}.Under("8").Under("7"), "", "7", true},
		{"a wider ceiling does not widen it", Span{}.Under("7").Under("8"), "", "7", true},

		// An end nothing has set is no bound, and a span with an open one
		// is not closed.
		{"nothing narrowed at all", Span{}, "", "", false},
		{"only a floor", Span{}.AtLeast("6.0"), "6.0", "", false},
		{"only a ceiling", Span{}.Under("7"), "", "7", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.s.From != tt.from || tt.s.Below != tt.next || tt.s.Closed() != tt.closed {
				t.Errorf("= %q..%q, closed %v; want %q..%q, closed %v",
					tt.s.From, tt.s.Below, tt.s.Closed(), tt.from, tt.next, tt.closed)
			}
		})
	}
}

func TestEnds(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) (string, bool)
		v    string
		want string
		ok   bool
	}{
		// Next: the version immediately after an exact one.
		{"after a patch version", Next, "6.1.7.6", "6.1.7.7", true},
		{"after a minor version", Next, "6.1", "6.2", true},
		{"after a major version", Next, "6", "7", true},

		// AfterLine: the last segment given is free, the rest is held.
		{"a line to the patch", AfterLine, "6.1.0", "6.2", true},
		{"a line to the minor", AfterLine, "6.1", "7", true},
		{"a line of one segment holds nothing", AfterLine, "6", "7", true},

		// AfterMinor: the minor is held where there is one, which is where
		// npm's tilde parts from Composer's.
		{"a tilde to the patch", AfterMinor, "1.2.3", "1.3", true},
		{"a tilde to the minor", AfterMinor, "1.2", "1.3", true},
		{"a tilde with no minor to hold", AfterMinor, "1", "2", true},

		// AfterCompatible: everything from the first meaningful segment is
		// free, and below 1.0 that segment is not the first.
		{"a caret on a major", AfterCompatible, "8.1", "9", true},
		{"a caret on a patch", AfterCompatible, "8.1.3", "9", true},
		{"a caret below 1.0", AfterCompatible, "0.3.0", "0.4", true},
		{"a caret below 0.1", AfterCompatible, "0.0.3", "0.0.4", true},
		{"a caret on nothing but zeros", AfterCompatible, "0.0", "0.1", true},

		// Undecidable: not a version to do arithmetic on.
		{"a pre-release", Next, "7.1.0.rc1", "", false},
		{"a word", AfterLine, "latest", "", false},
		{"a dist-tag", AfterMinor, "next", "", false},
		{"empty", AfterCompatible, "", "", false},
		{"an empty segment", Next, "6..1", "", false},
		{"a negative segment", AfterLine, "6.-1", "", false},
		{"a segment too large to be a number", Next, "99999999999999999999", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.fn(tt.v)
			if got != tt.want || ok != tt.ok {
				t.Errorf("(%q) = %q, %v; want %q, %v", tt.v, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNumeric(t *testing.T) {
	for _, tt := range []struct {
		v    string
		want bool
	}{
		{"6", true},
		{"6.1.7.6", true},
		{"7.1.0.rc1", false},
		{"latest", false},
		{"", false},
	} {
		if got := Numeric(tt.v); got != tt.want {
			t.Errorf("Numeric(%q) = %v; want %v", tt.v, got, tt.want)
		}
	}
}

func TestLower(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		// Caught: the shorter bound is read as if padded with zeros, which
		// is what a bound means.
		{"lower major", "6.1", "7.0", true},
		{"lower minor", "6.1", "6.2", true},
		{"shorter against its own line", "6.1", "6.1.1", true},
		{"6.10 is above 6.9", "6.9", "6.10", true},

		// Not caught: equal bounds, and a bound that merely runs out.
		{"the same", "6.1", "6.1", false},
		{"the same padded", "6.1", "6.1.0", false},
		{"higher", "7.0", "6.1", false},

		// Undecidable: a bound that is not a version is below nothing.
		{"a word", "latest", "6.1", false},
		{"a word on the right", "6.1", "latest", false},
		{"empty", "", "6.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Lower(tt.a, tt.b); got != tt.want {
				t.Errorf("Lower(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNarrow(t *testing.T) {
	tests := []struct {
		name       string
		op, v      string
		from, next string
		ok         bool
	}{
		// One version, written with the operator or without it.
		{"a bare version", "", "6.1.7", "6.1.7", "6.1.8", true},
		{"an equals", "=", "6.1.7", "6.1.7", "6.1.8", true},

		// One end. A bound that excludes its own version reads as one that
		// admits it, the two differing by a version no cycle turns on.
		{"a floor", ">=", "6.1", "6.1", "", true},
		{"an exclusive floor", ">", "6.1", "6.1", "", true},
		{"a ceiling", "<", "7.0", "", "7.0", true},
		{"a ceiling that admits itself", "<=", "6.1", "", "6.2", true},

		// Not one of the shared comparisons, or not a version to compare.
		{"an operator of its own", "~>", "6.1", "", "", false},
		{"an exclusion", "!=", "6.1", "", "", false},
		{"a word", "", "latest", "", "", false},
		{"a word under a ceiling", "<=", "latest", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Span{}.Narrow(tt.op, tt.v)
			if ok != tt.ok {
				t.Fatalf("Narrow(%q, %q) ok = %v; want %v", tt.op, tt.v, ok, tt.ok)
			}
			if ok && (got.From != tt.from || got.Below != tt.next) {
				t.Errorf("Narrow(%q, %q) = %q, %q; want %q, %q", tt.op, tt.v, got.From, got.Below, tt.from, tt.next)
			}
		})
	}
}

func TestMeets(t *testing.T) {
	tests := []struct {
		name      string
		s         Span
		lo, below string
		want      bool
	}{
		// Two ranges overlap when each starts before the other ends.
		{"inside", Span{From: "6.1.0", Below: "6.2"}, "6.1", "6.2", true},
		{"overlapping at the top", Span{From: "6.1", Below: "7"}, "6.0", "6.2", true},
		{"the whole of one inside the other", Span{From: "6.1.1", Below: "6.1.2"}, "6.1", "6.2", true},
		{"ends meeting is not overlapping", Span{From: "6.1", Below: "7"}, "6.0", "6.1", false},
		{"below", Span{From: "3.0", Below: "3.1"}, "6.1", "6.2", false},
		{"above", Span{From: "9.0", Below: "10"}, "6.1", "6.2", false},

		// An end nothing has set is no bound, so it stops nothing. A
		// requirement of `< 7.0` alone must not read as one that starts
		// below every version there is.
		{"no floor, reaching down", Span{Below: "5.3"}, "5.2", "5.3", true},
		{"no floor, stopped by the ceiling", Span{Below: "5.3"}, "6.1", "6.2", false},
		{"no ceiling, reaching up", Span{From: "6.1"}, "9.0", "9.1", true},
		{"no ceiling, stopped by the floor", Span{From: "6.1"}, "5.2", "5.3", false},
		{"neither end bound", Span{}, "6.1", "6.2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Meets(tt.lo, tt.below); got != tt.want {
				t.Errorf("%+v.Meets(%q, %q) = %v; want %v", tt.s, tt.lo, tt.below, got, tt.want)
			}
		})
	}
}
