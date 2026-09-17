package composerjson

import "testing"

func TestAllows(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		from, next string
		ok         bool
	}{
		// Caught: the constraint closes at both ends, so there is a range
		// to hand on.
		{"a caret", "^8.0", "8.0", "9", true},
		{"a caret below 1.0 holds the minor", "^0.3.0", "0.3.0", "0.4", true},
		{"a tilde to the patch", "~10.4.0", "10.4.0", "10.5", true},
		{"a tilde to the minor", "~10.4", "10.4", "11", true},
		{"a wildcard on the patch", "8.1.*", "8.1", "8.2", true},
		{"a wildcard on the minor", "8.*", "8", "9", true},
		{"an exact version", "1.2.3", "1.2.3", "1.2.4", true},
		{"a v in front", "v1.2.3", "1.2.3", "1.2.4", true},
		{"an equals", "=8.1.0", "8.1.0", "8.1.1", true},
		{"two ends separated by a space", ">=8.0 <9.0", "8.0", "9.0", true},
		{"two ends separated by a comma", ">=8.0,<9.0", "8.0", "9.0", true},
		{"operators held away from their versions", ">= 8.0 < 9.0", "8.0", "9.0", true},
		{"a caret narrowed further", "^8.0 <8.5", "8.0", "8.5", true},
		{"a ceiling that admits itself", "<=8.1", "", "8.2", true},

		// Not caught: more than one range is left behind, or none is closed.
		{"a union", "^7.4 || ^8.0", "", "", false},
		{"a single pipe is a union too", "^7.4|^8.0", "", "", false},
		{"every version there is", "*", "", "", false},
		{"a floor alone", ">=8.0", "", "", false},
		{"a hyphen range", "1.0 - 2.0", "", "", false},
		{"a stability flag", "^8.0@dev", "", "", false},
		{"a branch", "dev-main", "", "", false},
		{"an exclusion", "!=8.0", "", "", false},
		{"nothing at all", "", "", "", false},

		// Undecidable: not a version to do arithmetic on.
		{"a word", "^latest", "", "", false},
		{"a wildcard with an operator", "^8.1.*", "", "", false},
		{"a wildcard over a word", "x.*", "", "", false},
		{"a pre-release", "8.1.0rc1", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := allows(tt.constraint)
			if got.From != tt.from || got.Below != tt.next || ok != tt.ok {
				t.Errorf("allows(%q) = %q, %q, %v; want %q, %q, %v",
					tt.constraint, got.From, got.Below, ok, tt.from, tt.next, tt.ok)
			}
		})
	}
}
