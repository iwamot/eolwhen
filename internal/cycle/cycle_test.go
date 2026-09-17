package cycle

import "testing"

func TestMatch(t *testing.T) {
	python := []string{"3.14", "3.13", "3.12", "3.11", "3.10", "3.9", "3.1", "2.7"}
	node := []string{"26", "24", "22", "20", "14"}
	ubuntu := []string{"26.04", "24.04", "22.04", "18.04", "16.04"}
	golang := []string{"1.25", "1.24", "1.21", "1.16", "1.13", "1.12"}

	tests := []struct {
		name   string
		v      string
		cycles []string
		want   string
		ok     bool
	}{
		// Caught: a patch version reaches the cycle it belongs to.
		{"python patch", "3.9.10", python, "3.9", true},
		{"node patch", "14.19.0", node, "14", true},
		{"ubuntu point", "18.04.6", ubuntu, "18.04", true},
		{"go patch", "1.21.5", golang, "1.21", true},
		{"exact cycle", "2.7", python, "2.7", true},
		{"node bare major", "22", node, "22", true},

		// Not caught: the segment split is what keeps these apart. A string
		// prefix would file 3.10.2 under the 3.1 cycle.
		{"3.10 is not 3.1", "3.10.2", python, "3.10", true},
		{"3.1 stays 3.1", "3.1.4", python, "3.1", true},
		{"unknown major", "4.0.1", python, "", false},
		{"unknown runner-style label", "16.04", node, "", false},

		// Undecidable: nothing to match, so the caller reports the line
		// rather than guessing.
		{"empty", "", python, "", false},
		{"leading dot", ".9", python, "", false},
		{"word", "latest", python, "", false},
		{"shorter than any cycle", "3", ubuntu, "", false},
		{"no cycles", "3.9", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Match(tt.v, tt.cycles)
			if got != tt.want || ok != tt.ok {
				t.Errorf("Match(%q) = %q, %v; want %q, %v", tt.v, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestMatchPrefersLongest pins the tie-break directly: 1.21 and 1 both fit
// 1.21.5, and the deeper cycle is the answer.
func TestMatchPrefersLongest(t *testing.T) {
	got, ok := Match("1.21.5", []string{"1", "1.21"})
	if got != "1.21" || !ok {
		t.Errorf("Match = %q, %v; want 1.21, true", got, ok)
	}
}

// TestCovering counts what a version would reach if it were a prefix rather
// than a cycle, which is how a major line is told apart from a version the
// catalog never had.
func TestCovering(t *testing.T) {
	python := []string{"3.14", "3.13", "3.12", "3.1", "2.7"}
	redis := []string{"8.0", "7.4", "7.2", "7.0"}
	node := []string{"24", "22", "20"}
	runners := []string{"macos-15", "ubuntu-24.04", "windows-2025"}

	for _, tt := range []struct {
		name   string
		v      string
		cycles []string
		want   int
	}{
		// A major line: several cycles fall under it.
		{"a major with several cycles", "7", redis, 3},
		{"a major with many", "3", python, 4},
		// One is still a line rather than a version, and the wording has to
		// hold for it: a real product has exactly one cycle under a major.
		{"a major with exactly one", "8", redis, 1},
		// A cycle is not under itself, so an exact match counts nothing and
		// Match has already handled it.
		{"an exact cycle", "3.12", python, 0},
		{"a flat cycle list", "20", node, 0},
		// Nothing to reach.
		{"a version the catalog never had", "2.5", python, 0},
		{"a label", "windows-2019", runners, 0},
		{"empty", "", python, 0},
		{"no cycles", "3", nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Covering(tt.v, tt.cycles); got != tt.want {
				t.Errorf("Covering(%q) = %d; want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestOldest(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cycles []string
		want   string
	}{
		// Compared as numbers: 2.7 is older than 3.1, which is older than
		// 3.10, whatever the text says.
		{"python", []string{"3.13", "3.1", "2.7"}, "2.7"},
		{"a two-part version", []string{"22.04", "14.10", "14.04"}, "14.04"},
		{"cycles named with words have no oldest", []string{"macos-15", "windows-2025"}, ""},
		// A product that numbers some of its cycles is ordered by those.
		{"a mixture", []string{"next", "8.0", "7.4"}, "7.4"},
		{"nothing at all", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Oldest(tt.cycles)
			if ok != (tt.want != "") || got != tt.want {
				t.Errorf("Oldest(%v) = %q, %v; want %q", tt.cycles, got, ok, tt.want)
			}
		})
	}
}

func TestBefore(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"3.2", "4.0", true},
		{"3.2.1", "4.0", true},
		{"2.7", "3.1", true},
		{"3.1", "3.10", true},
		{"3.10", "3.1", false},
		{"4.0", "4.0", false},
		// A version that runs out where the cycle goes on covers it rather
		// than predating it.
		{"1.2", "1.2.3", false},
		{"14.04", "14.10", true},
		// Nothing spelled with a word is compared at all.
		{"bookworm", "4.0", false},
		{"4.0", "macos-15", false},
		{"", "4.0", false},
	} {
		if got := Before(tt.a, tt.b); got != tt.want {
			t.Errorf("Before(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSole(t *testing.T) {
	rails := []string{"8.1", "8.0", "7.2", "7.1", "7.0", "6.1", "6.0", "5.2"}
	django := []string{"5.2", "5.1", "5.0", "4.2", "4.1", "4.0"}
	angular := []string{"22", "21", "20", "19", "18", "17"}
	runners := []string{"ubuntu-24.04", "macos-15"}

	tests := []struct {
		name   string
		lo, hi string
		cycles []string
		want   string
		ok     bool
	}{
		// Caught: the whole range sits inside one cycle, so the cycle is
		// known even though the version is not.
		{"pessimistic to the patch", "6.1.0", "6.2", rails, "6.1", true},
		{"pessimistic to the minor", "6.1", "7", rails, "6.1", true},
		{"one version", "6.1.7.6", "6.1.7.7", rails, "6.1", true},
		{"caret on major cycles", "17.0.0", "18", angular, "17", true},
		{"compatible release on minor cycles", "4.2", "4.3", django, "4.2", true},
		{"lower bound inside the cycle", "6.1.4", "6.2", rails, "6.1", true},

		// Not caught: the range spans more than one cycle, and which one
		// gets installed is a resolver's answer rather than this one's.
		{"a whole major line", "6", "7", rails, "", false},
		{"caret over minor cycles", "5.0.0", "6", django, "", false},
		{"every cycle", "0", "99", angular, "", false},

		// Undecidable: no cycle is reached at all, or the bounds are not
		// versions to compare.
		{"below every cycle", "3.0", "3.1", rails, "", false},
		{"above every cycle", "9.0", "10", rails, "", false},
		{"cycles named with words", "15", "16", runners, "", false},
		{"no cycles", "6.1.0", "6.2", nil, "", false},
		{"empty lower bound", "", "6.2", rails, "", false},
		{"empty upper bound", "6.1.0", "", rails, "", false},
		{"bound is a word", "latest", "6.2", rails, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Sole(tt.lo, tt.hi, tt.cycles)
			if got != tt.want || ok != tt.ok {
				t.Errorf("Sole(%q, %q) = %q, %v; want %q, %v", tt.lo, tt.hi, got, ok, tt.want, tt.ok)
			}
		})
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
