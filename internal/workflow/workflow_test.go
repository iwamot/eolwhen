package workflow

import (
	"strings"
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		path string
		want bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/workflows/ci.yaml", true},
		{".github/workflows/nested/ci.yml", true},
		// A YAML file is a workflow because of where it sits, so these are
		// not workflows however they are named.
		{"workflows/ci.yml", false},
		{".github/actions/build/action.yml", false},
		{".github/dependabot.yml", false},
		{".github/workflows/README.md", false},
		{"", false},
	} {
		if got := Matches(tt.path); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}

type want struct {
	product string
	version string
	line    int
}

func check(t *testing.T, body string, ws ...want) {
	t.Helper()
	ds, us := Extract(".github/workflows/ci.yml", []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != len(ws) {
		t.Fatalf("declarations = %+v; want %d", ds, len(ws))
	}
	for i, w := range ws {
		got := ds[i]
		if got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %s %s; want %s %s", i, got.Product, got.Version, w.product, w.version)
		}
		if w.line != 0 && got.Source.Line != w.line {
			t.Errorf("[%d] line = %d; want %d", i, got.Source.Line, w.line)
		}
	}
}

func TestRunsOn(t *testing.T) {
	t.Run("one label", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: macos-13\n", want{Runners, "macos-13", 3})
	})
	t.Run("a two-part label", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: ubuntu-22.04\n", want{Runners, "ubuntu-22.04", 0})
	})
	t.Run("an arm variant is its own cycle", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: ubuntu-22.04-arm64\n", want{Runners, "ubuntu-22.04-arm64", 0})
	})
	t.Run("a list, with the machine names in it left alone", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: [self-hosted, linux, macos-13]\n", want{Runners, "macos-13", 0})
	})
	t.Run("a group with its labels", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on:\n      group: big\n      labels: [macos-14]\n", want{Runners, "macos-14", 0})
	})
	t.Run("a mapping of one entry, which the parser shapes differently", func(t *testing.T) {
		check(t, "runs-on: macos-13\n", want{Runners, "macos-13", 1})
	})
	t.Run("labels alone, with no group beside them", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on:\n      labels: [macos-13]\n", want{Runners, "macos-13", 0})
	})
	t.Run("an unquoted label is still a label", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: windows-2022\n", want{Runners, "windows-2022", 0})
	})
	// GitHub writes the architecture short and the catalog writes it in
	// full, so the label is spelled the catalog's way before it is matched.
	t.Run("an arm label spelled GitHub's way", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: ubuntu-24.04-arm\n", want{Runners, "ubuntu-24.04-arm64", 0})
	})
	t.Run("a windows arm label", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: windows-11-arm\n", want{Runners, "windows-11-arm64", 0})
	})
	t.Run("several jobs", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: macos-13\n  b:\n    runs-on: windows-2022\n",
			want{Runners, "macos-13", 3}, want{Runners, "windows-2022", 5})
	})
}

func TestSetup(t *testing.T) {
	step := func(uses, input, value string) string {
		return "jobs:\n  a:\n    steps:\n      - uses: " + uses + "\n        with:\n          " + input + ": " + value + "\n"
	}
	t.Run("node", func(t *testing.T) {
		check(t, step("actions/setup-node@v4", "node-version", "'20'"), want{"node", "20", 6})
	})
	t.Run("python", func(t *testing.T) {
		check(t, step("actions/setup-python@v5", "python-version", "'3.9.10'"), want{"python", "3.9.10", 0})
	})
	t.Run("a wildcard patch", func(t *testing.T) {
		check(t, step("actions/setup-go@v5", "go-version", "1.21.x"), want{"go", "1.21", 0})
	})
	t.Run("a bare wildcard", func(t *testing.T) {
		check(t, step("actions/setup-node@v4", "node-version", "20.x"), want{"node", "20", 0})
	})
	t.Run("a leading v", func(t *testing.T) {
		check(t, step("actions/setup-node@v4", "node-version", "v20.1.0"), want{"node", "20.1.0", 0})
	})
	t.Run("ruby, whose action is not under actions/", func(t *testing.T) {
		check(t, step("ruby/setup-ruby@v1", "ruby-version", "'3.4'"), want{"ruby", "3.4", 0})
	})
	t.Run("dotnet", func(t *testing.T) {
		check(t, step("actions/setup-dotnet@v4", "dotnet-version", "'6.0'"), want{"dotnet", "6.0", 0})
	})
	t.Run("php", func(t *testing.T) {
		check(t, step("shivammathur/setup-php@v2", "php-version", "'8.1'"), want{"php", "8.1", 0})
	})
	// A version written without quotes is a number to the parser and text to
	// everyone else. 3.10 must stay 3.10 and not become 3.1.
	t.Run("unquoted", func(t *testing.T) {
		check(t, step("actions/setup-node@v4", "node-version", "22"), want{"node", "22", 0})
	})
	t.Run("unquoted with a minor", func(t *testing.T) {
		check(t, step("actions/setup-go@v5", "go-version", "1.22"), want{"go", "1.22", 0})
	})
	t.Run("unquoted, where reading it as a number would change it", func(t *testing.T) {
		check(t, step("actions/setup-python@v5", "python-version", "3.10"), want{"python", "3.10", 0})
	})
	t.Run("several at once", func(t *testing.T) {
		body := "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: |\n            18\n            20\n"
		check(t, body, want{"node", "18", 0}, want{"node", "20", 0})
	})
}

func TestReported(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   string
		text   string
		reason string
	}{
		{"a moving runner", "jobs:\n  a:\n    runs-on: ubuntu-latest\n",
			"ubuntu-latest", "names latest, not a version"},
		{"a runner from the matrix", "jobs:\n  a:\n    runs-on: ${{ matrix.os }}\n",
			"${{ matrix.os }}", "takes its runner from an expression"},
		{"a version from the matrix", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ matrix.python }}\n",
			"${{ matrix.python }}", "takes its version from an expression"},
		// An expression may run over several lines. A complaint is one line,
		// so that every line of stderr starts with the program's name, and
		// folding rather than truncating keeps it readable: an expression
		// often opens with nothing but ${{.
		{"an expression over several lines", "jobs:\n  a:\n    runs-on: >-\n      ${{ github.repository == 'x/y'\n          && 'big'\n          || 'ubuntu-22.04' }}\n",
			"${{ github.repository == 'x/y' && 'big' || 'ubuntu-22.04' }}", "takes its runner from an expression"},
		{"a word where a version goes", "jobs:\n  a:\n    steps:\n      - uses: ruby/setup-ruby@v1\n        with:\n          ruby-version: ruby\n",
			"ruby", "is not a version"},
		{"a range", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: '^1.21'\n",
			"^1.21", "is not a version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			if len(us) != 1 || us[0].Text != tt.text || us[0].Reason != tt.reason {
				t.Fatalf("unreadable = %+v; want %q %q", us, tt.text, tt.reason)
			}
		})
	}
}

// TestNothing covers what is read and deliberately says nothing. A label
// naming somebody's own machine is not a version and never was, and an action
// this tool has not been told about could install anything.
func TestNothing(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"self-hosted", "jobs:\n  a:\n    runs-on: self-hosted\n"},
		{"a custom label", "jobs:\n  a:\n    runs-on: buildjet-8vcpu\n"},
		// A custom pool is free to begin with a family's name, so the family
		// alone does not make a label a version. These are real ones.
		{"a custom pool under a family name", "jobs:\n  a:\n    runs-on: ubuntu-x64-small\n"},
		{"a custom variant under a family name", "jobs:\n  a:\n    runs-on: ubuntu-slim\n"},
		{"a family name with nothing after it", "jobs:\n  a:\n    runs-on: ubuntu-\n"},
		{"a list of machine names", "jobs:\n  a:\n    runs-on: [self-hosted, linux, x64]\n"},
		{"an action that installs nothing", "jobs:\n  a:\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          fetch-depth: 0\n"},
		{"an action nobody vouched for", "jobs:\n  a:\n    steps:\n      - uses: acme/setup-node@v1\n        with:\n          node-version: '20'\n"},
		{"a setup step with no with", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n"},
		{"a setup step without its version input", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          cache: npm\n"},
		{"a with that is not a mapping", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with: nonsense\n"},
		{"uses that is not a scalar", "jobs:\n  a:\n    steps:\n      - uses: [a, b]\n        with:\n          node-version: '20'\n"},
		{"a runner that is not a scalar", "jobs:\n  a:\n    runs-on:\n      group: big\n"},
		// A label is a name, so neither a bare number nor no value at all is
		// one.
		{"a runner written as a number", "jobs:\n  a:\n    runs-on: 2024\n"},
		{"a runner with no value", "jobs:\n  a:\n    runs-on:\n"},
		{"a version input that is a list", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: [18, 20]\n"},
		{"no workflow in it at all", "name: nothing\n"},
		{"empty", ""},
		{"not YAML", "\tthis: is: not: yaml\n  - [\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

func TestSourceIsTheWorkflow(t *testing.T) {
	ds, _ := Extract(".github/workflows/release.yml", []byte("jobs:\n  a:\n    runs-on: macos-13\n"))
	if len(ds) != 1 || ds[0].Source != (decl.Source{File: ".github/workflows/release.yml", Line: 3}) {
		t.Errorf("Source = %+v; want release.yml:3", ds)
	}
}

// TestLongExpressionIsCut: folding keeps a complaint readable, and cutting
// keeps it beside its reason. Both together keep it on one line.
func TestLongExpressionIsCut(t *testing.T) {
	long := "${{ github.repository_owner == 'discourse' && 'cdck-linux-16-core-ubuntu-22' || 'ubuntu-latest' }}"
	ds, us := Extract(".github/workflows/ci.yml", []byte("jobs:\n  a:\n    runs-on: \""+long+"\"\n"))
	if len(ds) != 0 || len(us) != 1 {
		t.Fatalf("Extract = %+v, %+v", ds, us)
	}
	if strings.Contains(us[0].Text, "\n") {
		t.Errorf("Text spans lines: %q", us[0].Text)
	}
	if !strings.HasSuffix(us[0].Text, " …") {
		t.Errorf("Text = %q; want it cut", us[0].Text)
	}
	if n := len([]rune(us[0].Text)); n > oneLineLimit+2 {
		t.Errorf("Text is %d runes; want at most %d", n, oneLineLimit+2)
	}
}

// TestAliasedVersion: a workflow may hold the version in env: under an
// anchor and refer to it from the step that installs it. The anchor says
// nothing about what the version is for; the place it is referred from does.
func TestAliasedVersion(t *testing.T) {
	body := "on: push\n" +
		"env:\n" +
		"  PYTHON_VERSION: &python '2.7'\n" +
		"jobs:\n" +
		"  test:\n" +
		"    runs-on: ubuntu-22.04\n" +
		"    steps:\n" +
		"      - uses: actions/setup-python@v5\n" +
		"        with:\n" +
		"          python-version: *python\n"
	ds, _ := Extract(".github/workflows/ci.yml", []byte(body))
	var python []decl.Decl
	for _, d := range ds {
		if d.Product == "python" {
			python = append(python, d)
		}
	}
	if len(python) != 1 || python[0].Version != "2.7" {
		t.Fatalf("python declarations = %+v; want one at 2.7", python)
	}
	// The source is where the version was referred from, not where the
	// anchor was written: that is the line to go and change.
	if python[0].Source.Line != 10 {
		t.Errorf("line = %d; want 10, the alias", python[0].Source.Line)
	}
}

// TestSelfReferringRunsOn: a runs-on naming itself holds no label, in either
// shape it can take. That reading it ends at all is checked by running the
// binary, in e2e.
func TestSelfReferringRunsOn(t *testing.T) {
	for _, body := range []string{
		"jobs:\n  test:\n    runs-on: &runner [*runner]\n",
		"jobs:\n  test:\n    runs-on: &runner {labels: *runner}\n",
	} {
		ds, us := Extract(".github/workflows/ci.yml", []byte(body))
		if len(ds) != 0 || len(us) != 0 {
			t.Errorf("Extract(%q) = %+v, %+v; want nothing", body, ds, us)
		}
	}
}
