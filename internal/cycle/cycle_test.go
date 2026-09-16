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
