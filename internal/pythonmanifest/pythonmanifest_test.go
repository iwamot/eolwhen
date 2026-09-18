package pythonmanifest

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"pyproject.toml", true},
		{"src/pyproject.toml", true},
		{"requirements.txt", true},
		{"requirements-dev.txt", true},
		{"requirements_test.txt", true},
		// A directory of them is the other convention, which pip-tools
		// projects keep beside a pyproject.toml.
		{"requirements/base.txt", true},
		{"requirements/dev.txt", true},
		{"setup.py", false},
		{"poetry.lock", false},
		{"constraints.txt", false},
		{"docs/notes.txt", false},
		{"requirements.in", false},
		{"", false},
	} {
		if got := Matches(tt.path); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}

type want struct {
	product     string
	version     string
	from, below string
	line        int
}

func check(t *testing.T, path, body string, ws ...want) {
	t.Helper()
	ds, _ := Extract(path, []byte(body))
	if len(ds) != len(ws) {
		t.Fatalf("declarations = %+v; want %d", ds, len(ws))
	}
	for i, w := range ws {
		got := ds[i]
		if got.Ecosystem != Ecosystem {
			t.Errorf("[%d] ecosystem = %q; want %q", i, got.Ecosystem, Ecosystem)
		}
		if got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %s %q; want %s %q", i, got.Product, got.Version, w.product, w.version)
		}
		if got.Allows.From != w.from || got.Allows.Below != w.below {
			t.Errorf("[%d] range = %q..%q; want %q..%q", i, got.Allows.From, got.Allows.Below, w.from, w.below)
		}
		if got.Source != (decl.Source{File: path, Line: w.line}) {
			t.Errorf("[%d] source = %s; want %s:%d", i, got.Source, path, w.line)
		}
	}
}

const requirements = `# what it runs on
Django==2.2.0
numpy ~= 1.14.0
wagtail==4.1.*

-r requirements/dev.txt
-e .
--index-url https://example.com/simple
django-extensions==3.2.3 \
    --hash=sha256:deadbeef
`

func TestExtractReadsARequirementsFile(t *testing.T) {
	check(t, "requirements.txt", requirements,
		want{"Django", "==2.2.0", "2.2.0", "2.2.1", 2},
		want{"numpy", "~= 1.14.0", "1.14.0", "1.15", 3},
		want{"wagtail", "==4.1.*", "4.1", "4.2", 4},
		// Options after a requirement are options, and a line continued is
		// continued into them.
		want{"django-extensions", "==3.2.3", "3.2.3", "3.2.4", 9})
}

const project = `[project]
name = "demo"
requires-python = ">=3.9"
dependencies = [
    "django>=4.2,<5",
    "requests",
]

[project.optional-dependencies]
web = ["wagtail~=5.2.0"]

[dependency-groups]
dev = ["pytest>=8", "apache-airflow==2.7.3"]
`

func TestExtractReadsAPyproject(t *testing.T) {
	check(t, "pyproject.toml", project,
		want{"django", ">=4.2,<5", "4.2", "5", 5},
		// An extra and a group are read after what the project itself
		// needs, each in the order the file wrote them.
		want{"wagtail", "~=5.2.0", "5.2.0", "5.3", 10},
		// Two requirements may sit on one line, and both are sent there.
		want{"apache-airflow", "==2.7.3", "2.7.3", "2.7.4", 13})
}

// requires-python states what the project accepts rather than what it runs
// on, so it is no declaration of a version in use.
func TestExtractReadsNoRequiresPython(t *testing.T) {
	ds, us := Extract("pyproject.toml", []byte("[project]\nrequires-python = \">=3.9\"\n"))
	if len(ds) != 0 || len(us) != 0 {
		t.Errorf("= %+v, %+v; want neither", ds, us)
	}
}

func TestExtractSetsAsideWhatNamesNoVersion(t *testing.T) {
	ds, us := Extract("requirements.txt", []byte("requests>=2.0\nansible\n"))
	if len(ds) != 0 {
		t.Fatalf("declarations = %+v; want none", ds)
	}
	want := []decl.Unreadable{
		{Source: decl.Source{File: "requirements.txt", Line: 1}, Ecosystem: Ecosystem, Product: "requests", Text: ">=2.0",
			Reason: "names no single version here, so what gets installed decides which one", Moving: true},
		{Source: decl.Source{File: "requirements.txt", Line: 2}, Ecosystem: Ecosystem, Product: "ansible", Text: "",
			Reason: "names no single version here, so what gets installed decides which one", Moving: true},
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

func TestExtractReadsNothingElse(t *testing.T) {
	for _, tt := range []struct{ name, path, body string }{
		{"an empty requirements file", "requirements.txt", ""},
		{"comments and options alone", "requirements.txt", "# nothing\n-r other.txt\n--no-deps\n"},
		// A line that names no package is no requirement, and pip would
		// refuse it before this tool had anything to say about it.
		{"a line naming no package", "requirements.txt", "==4.2.0\n"},
		{"a pyproject that is not TOML yet", "pyproject.toml", "[project\n"},
		{"a pyproject declaring nothing", "pyproject.toml", "[project]\nname = \"demo\"\n"},
		{"an empty pyproject", "pyproject.toml", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(tt.path, []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v, %+v; want neither", ds, us)
			}
		})
	}
}

// Options may follow a requirement on its own line, which the dash they
// start with ends.
func TestExtractReadsOptionsBesideARequirement(t *testing.T) {
	check(t, "requirements.txt", "django==4.2.0 --hash=sha256:deadbeef\n",
		want{"django", "==4.2.0", "4.2.0", "4.2.1", 1})
}

// A requirement the scan cannot place keeps the file without a line, which
// is the worst it can do: what was read is never changed by where it was
// found.
func TestExtractKeepsARequirementItCannotPlace(t *testing.T) {
	// TOML writes a multi-line string, so the requirement is on no line of
	// the file as the reader sees it.
	ds, _ := Extract("pyproject.toml", []byte("[project]\ndependencies = [\n    \"\"\"dja\\\nngo==4.2.0\"\"\",\n]\n"))
	if len(ds) != 1 || ds[0].Product != "django" || ds[0].Source.Line != 0 {
		t.Errorf("= %+v; want django with no line", ds)
	}
}
