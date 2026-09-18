package pythonmanifest

import "testing"

func TestRequirement(t *testing.T) {
	for _, tt := range []struct {
		line      string
		name      string
		specifier string
	}{
		{"django==4.2.0", "django", "==4.2.0"},
		{"Django == 4.2.0", "Django", "== 4.2.0"},
		// PyPI treats the three separators alike, and so does the catalog,
		// so the name is handed on as the file spelled it.
		{"typing_extensions>=4", "typing_extensions", ">=4"},
		{"zope.interface>=5", "zope.interface", ">=5"},
		// Extras name parts of the same package, and a marker says when the
		// requirement applies rather than to what.
		{"django[argon2]>=4.2", "django", ">=4.2"},
		{"django [argon2] >= 4.2", "django", ">= 4.2"},
		{`django>=4.2 ; python_version >= "3.9"`, "django", ">=4.2"},
		{"django[argon2,bcrypt]==4.2.0; sys_platform == 'linux'", "django", "==4.2.0"},
		// A package with no specifier at all is still a package.
		{"ansible", "ansible", ""},
		{"ansible ", "ansible", ""},
		// A direct reference says where a version came from rather than
		// which one it is, so the package is named and no version is.
		{"django @ https://example.com/django.whl", "django", ""},
	} {
		t.Run(tt.line, func(t *testing.T) {
			name, specifier, ok := requirement(tt.line)
			if !ok || name != tt.name || specifier != tt.specifier {
				t.Errorf("requirement(%q) = %q, %q, %v; want %q, %q", tt.line, name, specifier, ok, tt.name, tt.specifier)
			}
		})
	}
}

func TestRequirementReadsNothing(t *testing.T) {
	for _, line := range []string{
		"",
		"   ",
		"@ https://example.com/x.whl",
		// A line that is not a name to look up.
		"==4.2.0",
		"; python_version >= '3.9'",
		"django[argon2",
	} {
		t.Run(line, func(t *testing.T) {
			if name, specifier, ok := requirement(line); ok {
				t.Errorf("requirement(%q) = %q, %q, true; want nothing", line, name, specifier)
			}
		})
	}
}

func TestAllows(t *testing.T) {
	for _, tt := range []struct {
		specifier   string
		from, below string
	}{
		// A pin is one version and the next one is not.
		{"==4.2.0", "4.2.0", "4.2.1"},
		{"== 4.2.0", "4.2.0", "4.2.1"},
		{"==v4.2.0", "4.2.0", "4.2.1"},
		// A wildcard names the line above it, which is how a requirements
		// file asks for a release cycle by name.
		{"==4.2.*", "4.2", "4.3"},
		// A compatible release holds everything but the last segment given.
		{"~=4.2.0", "4.2.0", "4.3"},
		{"~=4.2", "4.2", "5"},
		// Comparisons are separated by commas and every one of them holds.
		{">=4.2,<5", "4.2", "5"},
		{">=4.2, < 5", "4.2", "5"},
		{">4.2,<=4.2.9", "4.2", "4.2.10"},
		{"<5", "", "5"},
	} {
		t.Run(tt.specifier, func(t *testing.T) {
			s, ok := allows(tt.specifier)
			if !ok || s.From != tt.from || s.Below != tt.below {
				t.Errorf("allows(%q) = %q..%q, %v; want %q..%q", tt.specifier, s.From, s.Below, ok, tt.from, tt.below)
			}
		})
	}
}

func TestAllowsReadsNothing(t *testing.T) {
	for _, specifier := range []string{
		// Nothing closes the upper end, so which version was installed is
		// the lockfile's answer.
		">=4.2",
		">4.2",
		"",
		"   ",
		// An exclusion leaves a range with a hole in it, and an arbitrary
		// equality is not ordered against anything.
		"!=1.26.0",
		">=4.2,<5,!=4.2.1",
		"===4.2.0",
		// A wildcard belongs to equality alone, and names a line only where
		// what is left of it is a version.
		">=4.2.*",
		"~=4.2.*",
		"==x.*",
		// Not a version to do arithmetic on.
		"==4.2.0b1",
		"==4.2.0+local",
		// PEP 440 gives every clause an operator.
		"4.2.0",
	} {
		t.Run(specifier, func(t *testing.T) {
			if s, ok := allows(specifier); ok {
				t.Errorf("allows(%q) = %q..%q, true; want nothing", specifier, s.From, s.Below)
			}
		})
	}
}
