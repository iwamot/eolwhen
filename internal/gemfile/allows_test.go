package gemfile

import "testing"

func TestAllows(t *testing.T) {
	tests := []struct {
		name   string
		reqs   []string
		lo, hi string
		ok     bool
	}{
		// Caught: the requirements close at both ends, so there is a range
		// to hand on.
		{"pessimistic to the patch", []string{"~> 6.1.0"}, "6.1.0", "6.2", true},
		{"pessimistic to the minor", []string{"~> 6.1"}, "6.1", "7", true},
		{"pessimistic to the major", []string{"~> 6"}, "6", "7", true},
		{"a bare version", []string{"6.1.7.6"}, "6.1.7.6", "6.1.7.7", true},
		{"an equals", []string{"= 6.1.7"}, "6.1.7", "6.1.8", true},
		{"a floor and a ceiling", []string{">= 6.0", "< 7"}, "6.0", "7", true},
		{"a ceiling that admits itself", []string{">= 6.0", "<= 6.1"}, "6.0", "6.2", true},
		{"an exclusive floor", []string{"> 6.0", "< 7"}, "6.0", "7", true},
		{"a floor inside a pessimistic bound", []string{"~> 6.0", ">= 6.0.3"}, "6.0.3", "7", true},
		{"the narrower of two ceilings", []string{"< 8", "< 7"}, "", "7", true},
		{"a wider ceiling does not widen it", []string{"< 7", "< 8"}, "", "7", true},
		{"the higher of two floors", []string{">= 6.0", ">= 6.5", "< 7"}, "6.5", "7", true},
		{"a lower floor does not lower it", []string{">= 6.5", ">= 6.0", "< 7"}, "6.5", "7", true},
		{"no space after the operator", []string{"~>6.1.0"}, "6.1.0", "6.2", true},

		// Not caught: nothing closes the upper end, so the version that got
		// installed is in the lockfile rather than here.
		{"no requirements at all", nil, "", "", false},
		{"a floor alone", []string{">= 6.0"}, "", "", false},
		{"a floor and an exclusion", []string{">= 6.0", "!= 6.1"}, "", "", false},

		// Undecidable: the version is not one this can order.
		{"an exclusion", []string{"!= 6.1"}, "", "", false},
		{"a pre-release", []string{"~> 7.1.0.rc1"}, "", "", false},
		{"a word", []string{"latest"}, "", "", false},
		{"an empty requirement", []string{""}, "", "", false},
		{"an empty segment", []string{"~> 6..1"}, "", "", false},
		{"a segment too large to be a number", []string{"= 99999999999999999999"}, "", "", false},
		{"a negative segment", []string{"= 6.-1"}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := allows(tt.reqs)
			if got.From != tt.lo || got.Below != tt.hi || ok != tt.ok {
				t.Errorf("allows(%q) = %q, %q, %v; want %q, %q, %v", tt.reqs, got.From, got.Below, ok, tt.lo, tt.hi, tt.ok)
			}
		})
	}
}
