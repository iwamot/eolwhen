package workflow

import (
	"slices"
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
	ds, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
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
	// A document whose root holds one key is shaped differently by the
	// parser, and jobs: alone is the shape nearly every workflow has.
	t.Run("a root of one entry, which the parser shapes differently", func(t *testing.T) {
		check(t, "jobs:\n  a:\n    runs-on: macos-13\n", want{Runners, "macos-13", 3})
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
	// Java is the one that installs no single software: the distribution
	// says which build of it, and the catalog dates the build.
	t.Run("java", func(t *testing.T) {
		for _, tt := range []struct{ distribution, product string }{
			// Most are names the catalog already answers to, by upstream's
			// own aliases, so they are handed on as written.
			{"temurin", "temurin"},
			{"corretto", "corretto"},
			{"zulu", "zulu"},
			{"Temurin", "temurin"},
			// The two it has no alias for are the ones named here.
			{"oracle", "oracle-jdk"},
			{"microsoft", "microsoft-build-of-openjdk"},
			// One it knows nothing about is handed on all the same, and
			// what is known about it is the catalog's to say.
			{"jetbrains", "jetbrains"},
		} {
			t.Run(tt.distribution, func(t *testing.T) {
				body := "jobs:\n  a:\n    steps:\n      - uses: actions/setup-java@v4\n        with:\n" +
					"          distribution: " + tt.distribution + "\n          java-version: '17'\n"
				check(t, body, want{tt.product, "17", 7})
			})
		}
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

// A step that does not say which build of Java it installs names no software
// with a calendar of its own. setup-java requires the input, so a step
// missing it does not run, and there is nothing to report about one that
// never was.
func TestJavaWithoutADistribution(t *testing.T) {
	for _, tt := range []struct{ name, with string }{
		{"none at all", "          java-version: '17'\n"},
		{"one that is not a scalar", "          distribution:\n            - temurin\n          java-version: '17'\n"},
		{"an empty one", "          distribution: ''\n          java-version: '17'\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := "jobs:\n  a:\n    steps:\n      - uses: actions/setup-java@v4\n        with:\n" + tt.with
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v, %+v; want neither", ds, us)
			}
		})
	}
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
		// A matrix key the job's matrix does not list is a value this file
		// cannot produce; the ones it does list are TestMatrix.
		{"a runner from a matrix that lists none", "jobs:\n  a:\n    runs-on: ${{ matrix.os }}\n",
			"${{ matrix.os }}", "takes its runner from an expression"},
		{"a version from a matrix that lists none", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ matrix.python }}\n",
			"${{ matrix.python }}", "takes its version from an expression"},
		// A matrix and an env: in scope are followed; the ones that set
		// the name are TestEnv. An env variable nothing in the file sets
		// comes from somewhere this file cannot see.
		{"a version from an env variable nothing sets", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ env.PYTHON_VERSION }}\n",
			"${{ env.PYTHON_VERSION }}", "takes its version from an expression"},
		{"a version from any other context", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ vars.PYTHON_VERSION }}\n",
			"${{ vars.PYTHON_VERSION }}", "takes its version from an expression"},
		// A matrix built at run time lists nothing to read either.
		{"a matrix built by an expression", "jobs:\n  a:\n    strategy:\n      matrix: ${{ fromJSON(needs.setup.outputs.m) }}\n    runs-on: ${{ matrix.os }}\n",
			"${{ matrix.os }}", "takes its runner from an expression"},
		// An expression that does more than name a key works itself out at
		// run time, whatever the matrix lists.
		{"a label built out of a matrix value", "jobs:\n  a:\n    strategy:\n      matrix:\n        arch: [arm]\n    runs-on: ubuntu-24.04-${{ matrix.arch }}\n",
			"ubuntu-24.04-${{ matrix.arch }}", "takes its runner from an expression"},
		// An expression may run over several lines. A complaint is one line,
		// so that every line of stderr starts with the program's name, and
		// folding rather than truncating keeps it readable: an expression
		// often opens with nothing but ${{.
		{"an expression over several lines", "jobs:\n  a:\n    runs-on: >-\n      ${{ github.repository == 'x/y'\n          && 'big'\n          || 'ubuntu-22.04' }}\n",
			"${{ github.repository == 'x/y' && 'big' || 'ubuntu-22.04' }}", "takes its runner from an expression"},
		// An implementation setup-ruby does not install is no version of
		// the one it does.
		{"a word where a version goes", "jobs:\n  a:\n    steps:\n      - uses: ruby/setup-ruby@v1\n        with:\n          ruby-version: mruby-3.2\n",
			"mruby-3.2", "is not a version"},
		// Which build of Java a matrix lists would have to be paired with
		// the versions beside it, which is a run's answer and not this one's.
		{"a distribution from an expression", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-java@v4\n        with:\n          distribution: ${{ matrix.dist }}\n          java-version: '17'\n",
			"${{ matrix.dist }}", "takes its distribution from an expression"},
		{"a range", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: '^1.21'\n",
			"^1.21", "is not a version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(tt.body))
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
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

func TestSourceIsTheWorkflow(t *testing.T) {
	ds, _, _ := Extract(".github/workflows/release.yml", []byte("jobs:\n  a:\n    runs-on: macos-13\n"))
	if len(ds) != 1 || ds[0].Source != (decl.Source{File: ".github/workflows/release.yml", Line: 3}) {
		t.Errorf("Source = %+v; want release.yml:3", ds)
	}
}

// TestLongExpressionIsCut: folding keeps a complaint readable, and cutting
// keeps it beside its reason. Both together keep it on one line.
func TestLongExpressionIsCut(t *testing.T) {
	long := "${{ github.repository_owner == 'discourse' && 'cdck-linux-16-core-ubuntu-22' || 'ubuntu-latest' }}"
	ds, us, _ := Extract(".github/workflows/ci.yml", []byte("jobs:\n  a:\n    runs-on: \""+long+"\"\n"))
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
	ds, _, _ := Extract(".github/workflows/ci.yml", []byte(body))
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
		ds, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
		if len(ds) != 0 || len(us) != 0 {
			t.Errorf("Extract(%q) = %+v, %+v; want nothing", body, ds, us)
		}
	}
}

// TestMatrix covers the versions a job runs over. A matrix is where a project
// says which versions it supports, and `${{ matrix.python-version }}` was the
// single biggest source of lines this tool could not read — with the answer
// written out a few lines above it in the same job.
//
// Each version is reported at the line of the matrix, which is the line to go
// and change.
func TestMatrix(t *testing.T) {
	t.Run("runners", func(t *testing.T) {
		body := "jobs:\n" +
			"  a:\n" +
			"    strategy:\n" +
			"      matrix:\n" +
			"        os: [ubuntu-22.04, macos-13]\n" +
			"    runs-on: ${{ matrix.os }}\n"
		check(t, body, want{Runners, "ubuntu-22.04", 5}, want{Runners, "macos-13", 5})
	})
	t.Run("versions", func(t *testing.T) {
		body := "jobs:\n" +
			"  a:\n" +
			"    strategy:\n" +
			"      matrix:\n" +
			"        python-version:\n" +
			"          - '3.9'\n" +
			"          - '3.13'\n" +
			"    steps:\n" +
			"      - uses: actions/setup-python@v5\n" +
			"        with:\n" +
			"          python-version: ${{ matrix.python-version }}\n"
		check(t, body, want{"python", "3.9", 6}, want{"python", "3.13", 7})
	})
	// A matrix lists what a job runs over, and a list may hold anything YAML
	// allows. What is not a version to read is passed over, the rest of the
	// list still being the versions the project supports.
	t.Run("a value that is not a version to read", func(t *testing.T) {
		body := "jobs:\n" +
			"  a:\n" +
			"    strategy:\n" +
			"      matrix:\n" +
			"        python-version:\n" +
			"          - '3.9'\n" +
			"          - { version: '3.13' }\n" +
			"    steps:\n" +
			"      - uses: actions/setup-python@v5\n" +
			"        with:\n" +
			"          python-version: ${{ matrix.python-version }}\n"
		check(t, body, want{"python", "3.9", 6})
	})
	// A version written without quotes is a number to the parser and text to
	// everyone else, in a matrix as anywhere else.
	t.Run("an unquoted version", func(t *testing.T) {
		body := "jobs:\n  a:\n    strategy:\n      matrix:\n        v: [3.10]\n" +
			"    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ matrix.v }}\n"
		check(t, body, want{"python", "3.10", 5})
	})
	// include: holds whole combinations rather than values, and one of them
	// may name a version the list does not.
	t.Run("a version only include names", func(t *testing.T) {
		body := "jobs:\n  a:\n    strategy:\n      matrix:\n        go: ['1.22']\n        include:\n          - go: '1.21'\n" +
			"    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: ${{ matrix.go }}\n"
		check(t, body, want{"go", "1.22", 5}, want{"go", "1.21", 7})
	})
	// Two steps reading the same key point at the same lines, which is one
	// declaration each and not two.
	t.Run("read twice, declared once", func(t *testing.T) {
		body := "jobs:\n  a:\n    strategy:\n      matrix:\n        node: ['20']\n" +
			"    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: ${{ matrix.node }}\n" +
			"      - uses: actions/setup-node@v4\n        with:\n          node-version: ${{ matrix.node }}\n"
		check(t, body, want{"node", "20", 5})
	})
	// A matrix belongs to its job, so one job's key says nothing about the
	// same key in another.
	t.Run("a matrix is the one job's", func(t *testing.T) {
		body := "jobs:\n" +
			"  a:\n    strategy:\n      matrix:\n        v: ['20']\n" +
			"    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: ${{ matrix.v }}\n" +
			"  b:\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ matrix.v }}\n"
		ds, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
		if len(ds) != 1 || ds[0].Product != "node" || ds[0].Version != "20" {
			t.Fatalf("declarations = %+v; want node 20 alone", ds)
		}
		if len(us) != 1 || us[0].Reason != "takes its version from an expression" {
			t.Fatalf("unreadable = %+v; want the second job's expression", us)
		}
	})
	// A moving value in a matrix is moving wherever it is written.
	t.Run("a moving value", func(t *testing.T) {
		body := "jobs:\n  a:\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n    runs-on: ${{ matrix.os }}\n"
		ds, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
		if len(ds) != 0 {
			t.Fatalf("declarations = %+v; want none", ds)
		}
		if len(us) != 1 || us[0].Source.Line != 5 || !us[0].Moving {
			t.Fatalf("unreadable = %+v; want the matrix line, moving", us)
		}
	})
}

// TestMovingReported: a line that follows the newest release on purpose is
// still read and still handed back, with moving set so that the answer can
// leave it out of what it reports.
func TestMovingReported(t *testing.T) {
	for _, tt := range []struct{ name, body, text string }{
		{"a latest runner", "jobs:\n  a:\n    runs-on: ubuntu-latest\n", "ubuntu-latest"},
		{"a version input asking for the newest", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: latest\n", "latest"},
		{"a bare wildcard input", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: '*'\n", "*"},
		{"nvm's newest lts", "jobs:\n  a:\n    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: lts/*\n", "lts/*"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			if len(us) != 1 || us[0].Text != tt.text || !us[0].Moving {
				t.Fatalf("unreadable = %+v; want %q, moving", us, tt.text)
			}
		})
	}
}

// TestMatrixShapes: a workflow may be mid-edit or built at run time, and
// every shape that is not a job with a matrix of listed values reads as no
// matrix at all rather than as something to guess at.
func TestMatrixShapes(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"jobs that is not a mapping", "jobs: [a, b]\n"},
		{"a job that is not a mapping", "jobs:\n  a: ci.yml\n"},
		{"steps that is not a list", "jobs:\n  a:\n    steps: none\n"},
		{"a step that is not a mapping", "jobs:\n  a:\n    steps:\n      - run\n"},
		{"strategy that is not a mapping", "jobs:\n  a:\n    strategy: fail-fast\n    runs-on: macos-13\n"},
		{"a strategy with no matrix", "jobs:\n  a:\n    strategy:\n      fail-fast: false\n    runs-on: macos-13\n"},
		{"a matrix key that is not a list", "jobs:\n  a:\n    strategy:\n      matrix:\n        os: ubuntu-22.04\n    runs-on: macos-13\n"},
		{"an include entry that is not a mapping", "jobs:\n  a:\n    strategy:\n      matrix:\n        include: [x]\n    runs-on: macos-13\n"},
		{"an exclude, which declares nothing", "jobs:\n  a:\n    strategy:\n      matrix:\n        os: [macos-13]\n        exclude:\n          - os: macos-13\n    runs-on: macos-13\n"},
		{"a document that is not a workflow", "on: push\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(tt.body))
			for _, d := range ds {
				if d.Version != "macos-13" {
					t.Errorf("declarations = %+v; want nothing but the job's own runner", ds)
				}
			}
			if len(us) != 0 {
				t.Errorf("unreadable = %+v; want none", us)
			}
		})
	}
}

// A workflow that is not YAML is set aside whole, and one that is YAML and
// holds no job is not.
func TestExtractSetsAsideAFileItCannotParse(t *testing.T) {
	for _, tt := range []struct{ name, body, skipped string }{
		{"not yaml", "\tthis: is: not: yaml\n  - [\n", decl.NotYAML},
		{"no jobs in it", "name: nothing\n", ""},
		{"empty", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, skipped := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if skipped != tt.skipped {
				t.Errorf("skipped = %q; want %q", skipped, tt.skipped)
			}
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

// TestRubyEngines: setup-ruby names the implementation in the version, so
// each line reaches the software it installs. A development build or an
// implementation named on its own follows the newest there is, and says
// which software it is so that one the catalog does not track is set aside.
func TestRubyEngines(t *testing.T) {
	body := "jobs:\n  a:\n    strategy:\n      matrix:\n        ruby: ['3.3', ruby-2.6.5, jruby-9.4, truffleruby-24.1, head, jruby-head, truffleruby]\n" +
		"    steps:\n      - uses: ruby/setup-ruby@v1\n        with:\n          ruby-version: ${{ matrix.ruby }}\n"
	ds, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
	var got []string
	for _, d := range ds {
		got = append(got, d.Product+" "+d.Version)
	}
	if want := []string{"ruby 3.3", "ruby 2.6.5", "jruby 9.4", "truffleruby 24.1"}; !slices.Equal(got, want) {
		t.Errorf("declarations = %q; want %q", got, want)
	}
	got = nil
	for _, u := range us {
		if !u.Moving {
			t.Errorf("%+v is not moving", u)
		}
		got = append(got, u.What()+": "+u.Reason)
	}
	want := []string{
		"ruby head: names a development build, not a version",
		"jruby head: names a development build, not a version",
		"truffleruby: names the newest stable release, not a version",
	}
	if !slices.Equal(got, want) {
		t.Errorf("unreadable = %q; want %q", got, want)
	}
}

// TestReportedNamesTheSetup: a setup-* step names its software apart from
// the version, so a line it could not read still says which software it is
// about, for the catalog to set aside one it does not track. A runner label
// names its own.
func TestReportedNamesTheSetup(t *testing.T) {
	body := "jobs:\n  a:\n    runs-on: ${{ inputs.runner }}\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: ${{ env.PYTHON_VERSION }}\n"
	_, us, _ := Extract(".github/workflows/ci.yml", []byte(body))
	var got []string
	for _, u := range us {
		got = append(got, u.Product)
	}
	if want := []string{"", "python"}; !slices.Equal(got, want) {
		t.Errorf("products = %q; want %q", got, want)
	}
}

// TestEnv: a version kept in an env: block and given to a setup-* action as
// ${{ env.NAME }} is declared where the env: sets it, which is the line to
// go and change. The nearest env: in scope wins, as it does when the
// workflow runs.
func TestEnv(t *testing.T) {
	type line struct {
		product, version string
		line             int
	}
	const setup = "      - uses: actions/setup-node@v4\n        with:\n          node-version: ${{ env.NODE }}\n"
	for _, tt := range []struct {
		name string
		body string
		want []line
	}{
		{"set for the workflow", "env:\n  NODE: 20.18.1\njobs:\n  a:\n    steps:\n" + setup,
			[]line{{"node", "20.18.1", 2}}},
		{"a trailing wildcard, as setup-dotnet takes one", "env:\n  DOTNET: '10.0.x'\njobs:\n  a:\n    steps:\n      - uses: actions/setup-dotnet@v4\n        with:\n          dotnet-version: ${{ env.DOTNET }}\n",
			[]line{{"dotnet", "10.0", 2}}},
		{"the job's over the workflow's", "env:\n  NODE: '18'\njobs:\n  a:\n    env:\n      NODE: '20'\n    steps:\n" + setup,
			[]line{{"node", "20", 6}}},
		{"the step's over the job's", "jobs:\n  a:\n    env:\n      NODE: '18'\n    steps:\n      - env:\n          NODE: '22'\n        uses: actions/setup-node@v4\n        with:\n          node-version: ${{ env.NODE }}\n",
			[]line{{"node", "22", 7}}},
		// A job sees the workflow's env: and its own, not another job's.
		{"one job's for each job", "env:\n  NODE: '18'\njobs:\n  a:\n    env:\n      NODE: '20'\n    steps:\n" + setup + "  b:\n    steps:\n" + setup,
			[]line{{"node", "20", 6}, {"node", "18", 2}}},
		// Two steps reading the same line declare it once.
		{"read twice", "env:\n  NODE: '20'\njobs:\n  a:\n    steps:\n" + setup + setup,
			[]line{{"node", "20", 2}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if len(us) != 0 {
				t.Fatalf("unreadable = %+v; want none", us)
			}
			var got []line
			for _, d := range ds {
				got = append(got, line{d.Product, d.Version, d.Source.Line})
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("declarations = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestEnvUnread: an env: that says nothing this file can read is reported
// where the reader has to go and look.
func TestEnvUnread(t *testing.T) {
	const setup = "    steps:\n      - uses: actions/setup-node@v4\n        with:\n          node-version: ${{ env.NODE }}\n"
	for _, tt := range []struct {
		name, body, text string
		line             int
	}{
		// The variable is set, to something only a run can work out, and
		// that line is the one to look at.
		{"set by an expression", "env:\n  NODE: ${{ vars.NODE }}\njobs:\n  a:\n" + setup, "${{ vars.NODE }}", 2},
		// An env: written as an expression may set the name or replace the
		// workflow's, so neither is read.
		{"under an env: built by an expression", "env:\n  NODE: '20'\njobs:\n  a:\n    env: ${{ fromJSON(inputs.env) }}\n" + setup, "${{ env.NODE }}", 9},
		// Another job's env: is not in scope.
		{"set only for another job", "jobs:\n  a:\n    env:\n      NODE: '20'\n    steps: []\n  b:\n" + setup, "${{ env.NODE }}", 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract(".github/workflows/ci.yml", []byte(tt.body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			want := decl.Unreadable{Source: decl.Source{File: ".github/workflows/ci.yml", Line: tt.line}, Product: "node", Text: tt.text, Reason: "takes its version from an expression"}
			if len(us) != 1 || us[0] != want {
				t.Fatalf("unreadable = %+v; want %+v", us, want)
			}
		})
	}
}
