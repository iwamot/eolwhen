package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/iwamot/eolwhen/internal/catalog"
	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/resolve"
	"github.com/iwamot/eolwhen/internal/timeline"
)

var now = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

func at(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name  string
		argv  []string
		check func(*testing.T, cliArgs)
	}{
		{"no arguments reads here", nil, func(t *testing.T, a cliArgs) {
			if a.dir != "." {
				t.Errorf("dir = %q; want .", a.dir)
			}
		}},
		{"a directory", []string{"../x"}, func(t *testing.T, a cliArgs) {
			if a.dir != "../x" {
				t.Errorf("dir = %q; want ../x", a.dir)
			}
		}},
		{"within", []string{"--within", "2w"}, func(t *testing.T, a cliArgs) {
			if !a.withinSet || a.within != 14*24*time.Hour || a.withinText != "2w" {
				t.Errorf("within = %v, %v, %q", a.within, a.withinSet, a.withinText)
			}
		}},
		{"json", []string{"--json"}, func(t *testing.T, a cliArgs) {
			if !a.asJSON {
				t.Error("asJSON = false")
			}
		}},
		{"verbose", []string{"--verbose"}, func(t *testing.T, a cliArgs) {
			if !a.verbose {
				t.Error("verbose = false")
			}
		}},
		{"help", []string{"-h"}, func(t *testing.T, a cliArgs) {
			if !a.showHelp {
				t.Error("showHelp = false")
			}
		}},
		{"version", []string{"-v"}, func(t *testing.T, a cliArgs) {
			if !a.showVersion {
				t.Error("showVersion = false")
			}
		}},
		{"instructions", []string{"--instructions"}, func(t *testing.T, a cliArgs) {
			if !a.showInstructions {
				t.Error("showInstructions = false")
			}
		}},
		{"flags and a directory together", []string{"--within", "1d", "dir", "--json"}, func(t *testing.T, a cliArgs) {
			if a.dir != "dir" || !a.asJSON || !a.withinSet {
				t.Errorf("args = %+v", a)
			}
		}},
		{"dash-dash before a directory that starts with a dash", []string{"--json", "--", "-x"}, func(t *testing.T, a cliArgs) {
			if a.dir != "-x" || !a.asJSON {
				t.Errorf("args = %+v", a)
			}
		}},
		{"dash-dash after the directory", []string{"dir", "--"}, func(t *testing.T, a cliArgs) {
			if a.dir != "dir" {
				t.Errorf("dir = %q; want dir", a.dir)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := parseArgs(tt.argv)
			if err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
			tt.check(t, a)
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, tt := range []struct {
		name, want string
		argv       []string
	}{
		{"unknown flag", "unknown flag", []string{"--nope"}},
		{"within needs a value", "needs a value", []string{"--within"}},
		{"within needs a duration", "--within:", []string{"--within", "soon"}},
		{"ambiguous unit", "minutes or months", []string{"--within", "1m"}},
		{"two directories", "one directory per run", []string{"a", "b"}},
		{"dash-dash then two directories", "one directory per run", []string{"--", "a", "b"}},
		{"a directory on each side of dash-dash", "one directory per run", []string{"a", "--", "b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(tt.argv)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parseArgs(%v) = %v; want an error containing %q", tt.argv, err, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	past := timeline.Finding{Product: "python", Cycle: "2.7", EOL: at("2020-01-01")}
	future := timeline.Finding{Product: "nodejs", Cycle: "24", EOL: at("2028-04-30")}
	for _, tt := range []struct {
		name string
		fs   []timeline.Finding
		want int
	}{
		{"nothing", nil, exitNone},
		{"only ahead", []timeline.Finding{future}, exitFuture},
		{"something expired", []timeline.Finding{past}, exitPast},
		{"expired outranks ahead", []timeline.Finding{future, past}, exitPast},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.fs, now); got != tt.want {
				t.Errorf("exitCode = %d; want %d", got, tt.want)
			}
		})
	}
}

func TestNotes(t *testing.T) {
	f := timeline.Finding{Product: "python", Cycle: "2.7", EOL: at("2020-01-01")}
	ahead := timeline.Finding{Product: "nodejs", Cycle: "24", EOL: at("2028-04-30")}
	u := decl.Unreadable{Source: decl.Source{File: ".nvmrc", Line: 1}, Text: "lts/hydrogen", Reason: "names a moving target, not a version"}

	tests := []struct {
		name  string
		r     resolve.Result
		shown []timeline.Finding
		a     cliArgs
		want  []string
	}{
		{"everything named software with no policy", resolve.Result{}, nil, cliArgs{},
			[]string{"nothing endoflife.date tracks was declared in dir"}},
		{"everything was unreadable", resolve.Result{Unreadable: []decl.Unreadable{u}}, nil, cliArgs{},
			[]string{".nvmrc:1: lts/hydrogen names a moving target, not a version", "nothing could be placed on the timeline"}},
		{"the same complaint from several places", resolve.Result{Unreadable: []decl.Unreadable{u, {Source: decl.Source{File: "b", Line: 2}, Text: u.Text, Reason: u.Reason}}}, nil, cliArgs{},
			[]string{".nvmrc:1: lts/hydrogen names a moving target, not a version (and 1 more)", "nothing could be placed on the timeline"}},
		{"a window hid some", resolve.Result{Findings: []timeline.Finding{f, ahead}}, []timeline.Finding{f},
			cliArgs{withinSet: true, withinText: "90d"},
			[]string{"1 more expire further out than 90d; drop --within to see them"}},
		{"nothing owed", resolve.Result{Findings: []timeline.Finding{f}}, []timeline.Finding{f}, cliArgs{}, nil},
		// What a tool list holds is mostly software with no end-of-life
		// policy, so it is only worth a line when asked for.
		{"untracked stays quiet", resolve.Result{Findings: []timeline.Finding{f}, Untracked: []decl.Decl{{Product: "biome", Version: "2.5.13", Source: decl.Source{File: "mise.toml", Line: 5}}}}, []timeline.Finding{f}, cliArgs{}, nil},
		{"untracked when asked for", resolve.Result{Findings: []timeline.Finding{f}, Untracked: []decl.Decl{{Product: "biome", Version: "2.5.13", Source: decl.Source{File: "mise.toml", Line: 5}}}}, []timeline.Finding{f}, cliArgs{verbose: true},
			[]string{"mise.toml:5: biome 2.5.13 is not tracked by endoflife.date"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notes("dir", tt.r, tt.shown, tt.a)
			if len(got) != len(tt.want) {
				t.Fatalf("notes = %q; want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("notes[%d] = %q; want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	for _, tt := range []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		want     string
	}{
		{"ldflags win", "1.2.3", &debug.BuildInfo{Main: debug.Module{Version: "v9"}}, "1.2.3"},
		{"go install records the tag", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}}, "v1.0.0"},
		{"a local build", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, devVersion},
		{"no build info", devVersion, nil, devVersion},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.injected, tt.info); got != tt.want {
				t.Errorf("resolveVersion = %q; want %q", got, tt.want)
			}
		})
	}
}

// runArgs exercises run() on the paths that never reach the network.
func runArgs(t *testing.T, argv ...string) (string, string, int) {
	t.Helper()
	var so, se bytes.Buffer
	code := run(argv, &so, &se)
	return so.String(), se.String(), code
}

func TestRunPrints(t *testing.T) {
	for _, tt := range []struct{ name, flag, prefix string }{
		{"help", "--help", "eolwhen — "},
		{"instructions", "--instructions", "To find out whether"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			so, se, code := runArgs(t, tt.flag)
			if code != exitNone || se != "" || !strings.HasPrefix(so, tt.prefix) {
				t.Errorf("%s = %q, %q, %d", tt.flag, so, se, code)
			}
		})
	}
	if so, se, code := runArgs(t, "--version"); code != exitNone || se != "" || strings.TrimSpace(so) == "" {
		t.Errorf("--version = %q, %q, %d", so, se, code)
	}
}

func TestRunUsageErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, want string
		argv       []string
	}{
		{"unknown flag", "unknown flag", []string{"--nope"}},
		{"missing directory", "does not exist", []string{filepath.Join(t.TempDir(), "nope")}},
		{"a file, not a directory", "is not a directory", []string{file}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			so, se, code := runArgs(t, tt.argv...)
			if code != exitUsage || so != "" || !strings.Contains(se, tt.want) {
				t.Errorf("%v = %q, %q, %d", tt.argv, so, se, code)
			}
		})
	}
}

// TestRunNoDeclarations is the half of the corpus that declares nothing. It
// must not reach the network, so that a directory with nothing to check
// answers the same way offline.
func TestRunNoDeclarations(t *testing.T) {
	dir := t.TempDir()
	so, se, code := runArgs(t, dir)
	if code != exitNone || so != "" || !strings.Contains(se, "no version declarations in") {
		t.Errorf("run(%q) = %q, %q, %d", dir, so, se, code)
	}
	so, _, code = runArgs(t, "--json", dir)
	if code != exitNone || !strings.Contains(so, `"findings": []`) {
		t.Errorf("run --json = %q, %d", so, code)
	}
}

const catalogDoc = `{"result":[
  {"name":"python","aliases":[],"releases":[
    {"name":"3.13","eolFrom":"2029-10-31"},
    {"name":"2.7","eolFrom":"2020-01-01"}
  ]},
  {"name":"nodejs","aliases":["node"],"releases":[
    {"name":"26","eolFrom":null},
    {"name":"22","eolFrom":"2027-04-30"}
  ]}
]}`

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Decode([]byte(catalogDoc))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return c
}

func at2(file string, line int) decl.Source { return decl.Source{File: file, Line: line} }

// TestReport walks the whole path from declarations to rows and an exit
// code, with the catalog handed in rather than fetched.
func TestReport(t *testing.T) {
	expired := decl.Decl{Product: "python", Version: "2.7.18", Source: at2(".python-version", 1)}
	ahead := decl.Decl{Product: "node", Version: "22.1.0", Source: at2(".nvmrc", 1)}
	unread := decl.Unreadable{Source: at2(".node-version", 1), Text: "lts/iron", Reason: "names a moving target, not a version"}

	tests := []struct {
		name     string
		a        cliArgs
		ds       []decl.Decl
		us       []decl.Unreadable
		wantOut  string
		wantErr  []string
		wantCode int
	}{
		{
			name: "expired and ahead, oldest first",
			a:    cliArgs{dir: "."},
			ds:   []decl.Decl{ahead, expired},
			wantOut: "-2450d  2020-01-01  python 2.7  .python-version:1\n" +
				" +226d  2027-04-30  nodejs 22   .nvmrc:1\n",
			wantCode: exitPast,
		},
		{
			name:     "only ahead",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{ahead},
			wantOut:  "+226d  2027-04-30  nodejs 22  .nvmrc:1\n",
			wantCode: exitFuture,
		},
		{
			name:     "a window hides what is far ahead, never what expired",
			a:        cliArgs{dir: ".", withinSet: true, within: 90 * 24 * time.Hour, withinText: "90d"},
			ds:       []decl.Decl{ahead, expired},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantErr:  []string{"1 more expire further out than 90d"},
			wantCode: exitPast,
		},
		{
			name:     "an extractor's unreadable line is carried through",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{expired},
			us:       []decl.Unreadable{unread},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantErr:  []string{".node-version:1: lts/iron names a moving target"},
			wantCode: exitPast,
		},
		{
			name:     "a cycle with no announced date is said, not printed",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{{Product: "nodejs", Version: "26.0.1", Source: at2(".nvmrc", 1)}},
			wantErr:  []string{"has no announced end-of-life date", "nothing could be placed on the timeline"},
			wantCode: exitNone,
		},
		// A tool list's line about software endoflife.date does not track is
		// set aside whatever its version looked like; only --verbose says so.
		{
			name:     "an unreadable line about untracked software stays quiet",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{expired},
			us:       []decl.Unreadable{{Source: at2("mise.toml", 2), Product: "jq", Text: "latest", Reason: "names a moving target, not a version"}},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantErr:  nil,
			wantCode: exitPast,
		},
		{
			name:     "and is accounted for when asked",
			a:        cliArgs{dir: ".", verbose: true},
			ds:       []decl.Decl{expired},
			us:       []decl.Unreadable{{Source: at2("mise.toml", 2), Product: "jq", Text: "latest", Reason: "names a moving target, not a version"}},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantErr:  []string{"mise.toml:2: jq latest is not tracked by endoflife.date"},
			wantCode: exitPast,
		},
		{
			name:     "the same line about tracked software is said",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{expired},
			us:       []decl.Unreadable{{Source: at2("mise.toml", 2), Product: "node", Text: "latest", Reason: "names a moving target, not a version"}},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantErr:  []string{"mise.toml:2: node latest names a moving target, not a version"},
			wantCode: exitPast,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var so, se bytes.Buffer
			code := report(tt.a, testCatalog(t), tt.ds, tt.us, now, &so, &se)
			if code != tt.wantCode {
				t.Errorf("code = %d; want %d", code, tt.wantCode)
			}
			if so.String() != tt.wantOut {
				t.Errorf("stdout =\n%q\nwant\n%q", so.String(), tt.wantOut)
			}
			if tt.wantErr == nil && se.String() != "" {
				t.Errorf("stderr = %q; want nothing", se.String())
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(se.String(), want) {
					t.Errorf("stderr =\n%q\nwant it to contain %q", se.String(), want)
				}
			}
		})
	}
}

// TestReportWithoutCatalog is the run whose every line was unreadable before
// any software was named: the answer is complete without a lookup, so no
// catalog is handed in.
func TestReportWithoutCatalog(t *testing.T) {
	us := []decl.Unreadable{{Source: at2(".github/workflows/ci.yml", 5), Text: "ubuntu-latest", Reason: "names latest, not a version"}}
	var so, se bytes.Buffer
	code := report(cliArgs{dir: "."}, nil, nil, us, now, &so, &se)
	if code != exitNone || so.String() != "" {
		t.Errorf("report = %q, %d; want no rows and exit %d", so.String(), code, exitNone)
	}
	for _, want := range []string{"ubuntu-latest names latest", "nothing could be placed on the timeline"} {
		if !strings.Contains(se.String(), want) {
			t.Errorf("stderr = %q; want it to contain %q", se.String(), want)
		}
	}
}

func TestNeedsCatalog(t *testing.T) {
	d := decl.Decl{Product: "python", Version: "2.7", Source: at2(".python-version", 1)}
	label := decl.Unreadable{Source: at2("ci.yml", 1), Text: "ubuntu-latest", Reason: "names latest, not a version"}
	tool := decl.Unreadable{Source: at2("mise.toml", 2), Product: "jq", Text: "latest", Reason: "names a moving target, not a version"}
	for _, tt := range []struct {
		name string
		ds   []decl.Decl
		us   []decl.Unreadable
		want bool
	}{
		{"a declaration", []decl.Decl{d}, nil, true},
		{"a label that names no software", nil, []decl.Unreadable{label}, false},
		{"a tool that may be untracked", nil, []decl.Unreadable{label, tool}, true},
		{"nothing", nil, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsCatalog(tt.ds, tt.us); got != tt.want {
				t.Errorf("needsCatalog = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestRunWithoutCatalog runs a directory whose only declaration is a runner
// label that names no version. Nothing there has to be looked up, so the run
// answers without the network, the way a directory with no declarations
// does.
func TestRunWithoutCatalog(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "ci.yml"), []byte("jobs:\n  test:\n    runs-on: ubuntu-latest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	so, se, code := runArgs(t, dir)
	if code != exitNone || so != "" || !strings.Contains(se, "ubuntu-latest names latest") {
		t.Errorf("run(%q) = %q, %q, %d", dir, so, se, code)
	}
	so, _, code = runArgs(t, "--json", dir)
	if code != exitNone || !strings.Contains(so, `"text": "ubuntu-latest"`) {
		t.Errorf("run --json = %q, %d", so, code)
	}
}

func TestReportJSON(t *testing.T) {
	var so, se bytes.Buffer
	ds := []decl.Decl{{Product: "python", Version: "2.7.18", Source: at2(".python-version", 1)}}
	us := []decl.Unreadable{{Source: at2(".nvmrc", 1), Text: "lts/iron", Reason: "names a moving target, not a version"}}
	code := report(cliArgs{dir: "some/dir", asJSON: true}, testCatalog(t), ds, us, now, &so, &se)
	if code != exitPast {
		t.Errorf("code = %d; want %d", code, exitPast)
	}
	// With --json the words go into the document, so stderr stays empty.
	if se.String() != "" {
		t.Errorf("stderr = %q; want empty", se.String())
	}
	for _, want := range []string{`"directory": "some/dir"`, `"cycle": "2.7"`, `"text": "lts/iron"`} {
		if !strings.Contains(so.String(), want) {
			t.Errorf("stdout is missing %s:\n%s", want, so.String())
		}
	}
}

// TestCollapse: nearly every workflow in a healthy repository says
// runs-on: ubuntu-latest, so the same complaint arrives many times and is
// worth one line, not nine.
func TestCollapse(t *testing.T) {
	at := func(f string, l int) decl.Source { return decl.Source{File: f, Line: l} }
	us := []decl.Unreadable{
		{Source: at("a.yml", 1), Text: "ubuntu-latest", Reason: "names latest, not a version"},
		{Source: at("b.yml", 2), Text: "ubuntu-latest", Reason: "names latest, not a version"},
		{Source: at("c.yml", 3), Text: "${{ matrix.os }}", Reason: "takes its runner from an expression"},
		{Source: at("d.yml", 4), Text: "ubuntu-latest", Reason: "names latest, not a version"},
		// The same text for a different reason stays its own line.
		{Source: at("e.yml", 5), Text: "ubuntu-latest", Reason: "is not a version"},
	}
	got := collapse(us)
	if len(got) != 3 {
		t.Fatalf("collapse = %+v; want 3 lines", got)
	}
	// The place kept is the first one seen, so the list reads in the order
	// the files were walked.
	if got[0].Source != at("a.yml", 1) || got[0].more != 2 {
		t.Errorf("[0] = %+v; want a.yml:1 and 2 more", got[0])
	}
	if got[1].Source != at("c.yml", 3) || got[1].more != 0 {
		t.Errorf("[1] = %+v; want c.yml:3 alone", got[1])
	}
	if got[2].Reason != "is not a version" || got[2].more != 0 {
		t.Errorf("[2] = %+v; want the other reason alone", got[2])
	}
	if len(collapse(nil)) != 0 {
		t.Error("collapse(nil) should be empty")
	}
}

// TestReadmeQuotesTheReference guards the two blocks README copies out of
// this file. They drifted once: the reference still described a tool that
// read only the runtime version files, several releases after it had stopped
// being one. A reader following a stale reference is worse served than one
// with none.
func TestReadmeQuotesTheReference(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	for _, tt := range []struct{ name, text string }{
		{"the help", helpText},
		{"the instructions paragraph", instructionsText},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(string(readme), strings.TrimSuffix(tt.text, "\n")) {
				t.Errorf("README.md does not quote %s verbatim; paste the current output in", tt.name)
			}
		})
	}
}
