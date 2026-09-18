package packagejson

import "testing"

func TestAllows(t *testing.T) {
	for _, tt := range []struct {
		requirement string
		from, below string
	}{
		// A caret holds the first segment that says anything about
		// compatibility, which is npm's rule and Composer's alike.
		{"^4.18.2", "4.18.2", "5"},
		{"^0.3.0", "0.3.0", "0.4"},
		{"^12", "12", "13"},
		// A tilde holds the minor where there is one. This is the one
		// operator npm and Composer spell the same and mean differently:
		// Composer reads ~1.2 as every 1.x.
		{"~13.4.1", "13.4.1", "13.5"},
		{"~1.2", "1.2", "1.3"},
		{"~1", "1", "2"},
		// A version on its own is that version and the next one is not.
		{"1.3.0", "1.3.0", "1.3.1"},
		{"=1.3.0", "1.3.0", "1.3.1"},
		{"v1.3.0", "1.3.0", "1.3.1"},
		// Comparators are separated by space and every one of them holds.
		{">=4 <6", "4", "6"},
		{">=4.0.0 <5.0.0", "4.0.0", "5.0.0"},
		{"<=3.4", "", "3.5"},
		// An operator may be written away from its version.
		{">= 4 < 6", "4", "6"},
		{"^ 12", "12", "13"},
		// A wildcard segment names the line above it, however many of them
		// there are.
		{"18.x", "18", "19"},
		{"3.4.x", "3.4", "3.5"},
		{"1.X", "1", "2"},
		{"1.2.*", "1.2", "1.3"},
		{"1.x.x", "1", "2"},
	} {
		t.Run(tt.requirement, func(t *testing.T) {
			s, ok := allows(tt.requirement)
			if !ok || s.From != tt.from || s.Below != tt.below {
				t.Errorf("allows(%q) = %q..%q, %v; want %q..%q", tt.requirement, s.From, s.Below, ok, tt.from, tt.below)
			}
		})
	}
}

func TestAllowsReadsNothing(t *testing.T) {
	for _, requirement := range []string{
		// A union leaves two ranges behind rather than one, and a hyphen
		// range is npm's own spelling rather than an operator.
		"^1 || ^2",
		"1.0.0 - 2.0.0",
		// Nothing closes the upper end, so which version was installed is
		// the lockfile's answer.
		">=2.6",
		">4",
		"",
		// Not a version range at all.
		"npm:lodash-es@^4",
		"workspace:*",
		"file:../shared",
		"github:owner/repo",
		"git+https://example.com/x.git#v1",
		"https://example.com/x.tgz",
		"owner/repo",
		// A dist-tag names whatever is newest, and a pre-release falls
		// where the package manager says rather than where a number does.
		"latest",
		"next",
		"^1.0.0-beta.1",
		// Every version there is narrows nothing.
		"*",
		"x",
		// An operator in front of a wildcard is not npm.
		"^16.x",
		">=1.x",
		// Not a version on either side.
		"one.two",
		"x.2",
	} {
		t.Run(requirement, func(t *testing.T) {
			if s, ok := allows(requirement); ok {
				t.Errorf("allows(%q) = %q..%q, true; want nothing", requirement, s.From, s.Below)
			}
		})
	}
}
