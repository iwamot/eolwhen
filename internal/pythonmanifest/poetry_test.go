package pythonmanifest

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestPoetryAllows(t *testing.T) {
	for _, tt := range []struct {
		constraint  string
		from, below string
	}{
		// A caret holds the first segment that says anything about
		// compatibility, and a tilde the minor where one is given. Both are
		// npm's rules rather than PEP 440's.
		{"^4.2", "4.2", "5"},
		{"^0.3.0", "0.3.0", "0.4"},
		{"^0.0.3", "0.0.3", "0.0.4"},
		{"~1.14.0", "1.14.0", "1.15"},
		{"~1.14", "1.14", "1.15"},
		{"~1", "1", "2"},
		// A version on its own is that version, which is Poetry's own rule
		// and not PEP 440's, where every clause carries an operator.
		{"4.2.0", "4.2.0", "4.2.1"},
		{"==4.2.0", "4.2.0", "4.2.1"},
		// A wildcard names the line above it.
		{"4.1.*", "4.1", "4.2"},
		{"1.*", "1", "2"},
		// Clauses are separated by commas and every one of them holds.
		{">=2.28,<3", "2.28", "3"},
		{">=2.28, <3", "2.28", "3"},
		{">4.2,<=4.2.9", "4.2", "4.2.10"},
	} {
		t.Run(tt.constraint, func(t *testing.T) {
			s, ok := poetryAllows(tt.constraint)
			if !ok || s.From != tt.from || s.Below != tt.below {
				t.Errorf("poetryAllows(%q) = %q..%q, %v; want %q..%q", tt.constraint, s.From, s.Below, ok, tt.from, tt.below)
			}
		})
	}
}

func TestPoetryAllowsReadsNothing(t *testing.T) {
	for _, constraint := range []string{
		// Every version there is narrows nothing, and an entry naming no
		// version at all reaches here as an empty constraint.
		"*",
		"",
		// Nothing closes the upper end.
		">=4.2",
		"^",
		// A union leaves two ranges behind rather than one, and a stability
		// marker asks for something a number does not order.
		">=1.0 || <2.0",
		"^1.0@beta",
		// An exclusion leaves a range with a hole in it.
		"!=1.26.0",
		">=4.2,<5,!=4.2.1",
		// An operator in front of a wildcard is not Poetry, and a wildcard
		// names a line only where what is left of it is a version.
		"^1.2.*",
		"x.*",
		// Not a version to do arithmetic on.
		"^1.0.0b1",
		"latest",
	} {
		t.Run(constraint, func(t *testing.T) {
			if s, ok := poetryAllows(constraint); ok {
				t.Errorf("poetryAllows(%q) = %q..%q, true; want nothing", constraint, s.From, s.Below)
			}
		})
	}
}

const poetryProject = `[tool.poetry]
name = "demo"

[tool.poetry.dependencies]
django = "^4.2"
numpy = "~1.14.0"
apache-airflow = {version = "2.7.3", optional = true}

[tool.poetry.group.dev.dependencies]
django = "^3.2"
`

func TestExtractReadsAPoetryTable(t *testing.T) {
	check(t, "pyproject.toml", poetryProject,
		// Within a table the keys are sorted, and the project's own table
		// is read before its groups.
		want{"apache-airflow", "2.7.3", "2.7.3", "2.7.4", 7},
		want{"django", "^4.2", "4.2", "5", 5},
		want{"numpy", "~1.14.0", "1.14.0", "1.15", 6},
		// The same package asked for in two tables is sent to the line each
		// table wrote.
		want{"django", "^3.2", "3.2", "4", 10})
}

// An entry naming no version of its own is set aside like any requirement
// that leaves the version to an install.
func TestExtractSetsAsidePoetryEntriesWithNoVersion(t *testing.T) {
	ds, us, _ := Extract("pyproject.toml", []byte(`[tool.poetry.dependencies]
mylib = {git = "https://example.com/mylib.git"}
six = [{version = "^1.0", python = "<3.10"}, {version = "^2.0", python = ">=3.10"}]
`))
	if len(ds) != 0 {
		t.Fatalf("declarations = %+v; want none", ds)
	}
	want := []decl.Unreadable{
		{Source: decl.Source{File: "pyproject.toml", Line: 2}, Ecosystem: Ecosystem, Product: "mylib", Reason: unsettled, Moving: true},
		{Source: decl.Source{File: "pyproject.toml", Line: 3}, Ecosystem: Ecosystem, Product: "six", Reason: unsettled, Moving: true},
	}
	if len(us) != len(want) {
		t.Fatalf("unreadable = %+v; want %+v", us, want)
	}
	for i, w := range want {
		if us[i] != w {
			t.Errorf("[%d] = %+v; want %+v", i, us[i], w)
		}
	}
}

// A file may declare its dependencies both ways, which a project moving from
// one to the other does for a while.
func TestExtractReadsBothHalvesOfAPyproject(t *testing.T) {
	check(t, "pyproject.toml", "[project]\ndependencies = [\"wagtail~=5.2.0\"]\n\n[tool.poetry.dependencies]\ndjango = \"^4.2\"\n",
		want{"wagtail", "~=5.2.0", "5.2.0", "5.3", 2},
		want{"django", "^4.2", "4.2", "5", 5})
}

// A key outside a Poetry table is not a dependency, and a quoted one is the
// name it quotes.
func TestPoetryLines(t *testing.T) {
	got := poetryLines([]byte("[project]\nname = \"demo\"\n\n[tool.poetry.dependencies]\n\"django\" = \"^4.2\"\n"))
	if got["tool.poetry.dependencies.django"] != 5 || got["project.name"] != 2 {
		t.Errorf("poetryLines = %v", got)
	}
}
