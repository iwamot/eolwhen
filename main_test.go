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
	"github.com/iwamot/eolwhen/internal/scan"
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
	f := timeline.Finding{
		Product: "python", Cycle: "2.7", Version: "2.7.18",
		Matched: timeline.ByName, Dated: timeline.DatedByCycle,
		Page: "https://endoflife.date/python",
		EOL:  at("2020-01-01"), Source: at2(".python-version", 1),
	}
	evidenceOfF := ".python-version:1: 2.7.18 is python 2.7, matched by name; the date is that cycle's own — https://endoflife.date/python"
	ahead := timeline.Finding{Product: "nodejs", Cycle: "24", EOL: at("2028-04-30")}
	u := decl.Unreadable{Source: decl.Source{File: ".nvmrc", Line: 1}, Text: "lts/hydrogen", Reason: "names a moving target, not a version"}
	undated := timeline.Undated{Product: "go", Cycle: "1.26", Source: decl.Source{File: "go.mod", Line: 9}}
	ended := timeline.Undated{Product: "metabase", Cycle: "0.46", Source: decl.Source{File: "compose.yml", Line: 4}}
	m := decl.Unreadable{Source: decl.Source{File: ".github/workflows/ci.yml", Line: 3}, Text: "ubuntu-latest", Reason: "follows the newest release", Moving: true}
	skipped := []decl.Skipped{{File: "compose.yml", Reason: decl.NotYAML}}

	tests := []struct {
		name  string
		r     resolve.Result
		shown []timeline.Finding
		a     cliArgs
		sk    []decl.Skipped
		want  []string
	}{
		{"everything named software with no policy", resolve.Result{}, nil, cliArgs{}, nil,
			[]string{"nothing declared in dir is tracked by endoflife.date"}},
		// Nothing is wrong with the directory and nothing is owed, so the
		// one line says so rather than complaining once per declaration.
		{"nothing has a date yet", resolve.Result{Undated: []timeline.Undated{undated}}, nil, cliArgs{}, nil,
			[]string{"nothing declared in dir has an end-of-life date yet"}},
		{"and is accounted for when asked", resolve.Result{Undated: []timeline.Undated{undated}}, nil, cliArgs{verbose: true}, nil,
			[]string{"go.mod:9: go 1.26 has no end-of-life date yet", "nothing declared in dir has an end-of-life date yet"}},
		// A cycle upstream calls over without dating it has no row either,
		// and the line says the opposite of the one above: there is
		// something to do and no day to put it on. It is said whether or
		// not the reader asked for detail, that being the question they ran
		// the tool with.
		{"something is out of support with no date", resolve.Result{Ended: []timeline.Undated{ended}}, nil, cliArgs{}, nil,
			[]string{
				"compose.yml:4: metabase 0.46 is out of support, with no date published",
				"nothing declared in dir has a date to put on the timeline",
			}},
		// The same cycle declared in several places is one thing to know,
		// folded the way a repeated complaint is.
		{"the same ended cycle from several places",
			resolve.Result{Ended: []timeline.Undated{ended, {Product: "metabase", Cycle: "0.46", Source: decl.Source{File: "compose.yml", Line: 9}}}}, nil, cliArgs{}, nil,
			[]string{
				"compose.yml:4: metabase 0.46 is out of support, with no date published (and 1 more)",
				"nothing declared in dir has a date to put on the timeline",
			}},
		// What --verbose adds is the lines that ask for nothing; the ended
		// cycle above them was printed without being asked for.
		{"which lines those were, when asked", resolve.Result{Ended: []timeline.Undated{ended}, Moving: []decl.Unreadable{m}, Undated: []timeline.Undated{undated}}, nil, cliArgs{verbose: true}, nil,
			[]string{
				"compose.yml:4: metabase 0.46 is out of support, with no date published",
				".github/workflows/ci.yml:3: ubuntu-latest follows the newest release",
				"go.mod:9: go 1.26 has no end-of-life date yet",
				"nothing declared in dir has a date to put on the timeline",
			}},
		// A line that could not be read leaves the answer short of what the
		// directory declares, which outranks anything that had no date.
		{"everything was unreadable", resolve.Result{Unreadable: []decl.Unreadable{u}, Undated: []timeline.Undated{undated}}, nil, cliArgs{}, nil,
			[]string{".nvmrc:1: lts/hydrogen names a moving target, not a version", "nothing in dir could be placed on the timeline"}},
		{"the same complaint from several places", resolve.Result{Unreadable: []decl.Unreadable{u, {Source: decl.Source{File: "b", Line: 2}, Text: u.Text, Reason: u.Reason}}}, nil, cliArgs{}, nil,
			[]string{".nvmrc:1: lts/hydrogen names a moving target, not a version (and 1 more)", "nothing in dir could be placed on the timeline"}},
		{"a window hid some", resolve.Result{Findings: []timeline.Finding{f, ahead}}, []timeline.Finding{f},
			cliArgs{withinSet: true, withinText: "90d"}, nil,
			[]string{"1 more declaration expires further out than 90d; drop --within to see it"}},
		{"a window hid several", resolve.Result{Findings: []timeline.Finding{f, ahead, ahead}}, []timeline.Finding{f},
			cliArgs{withinSet: true, withinText: "90d"}, nil,
			[]string{"2 more declarations expire further out than 90d; drop --within to see them"}},
		{"nothing owed", resolve.Result{Findings: []timeline.Finding{f}}, []timeline.Finding{f}, cliArgs{}, nil, nil},
		// What a tool list holds is mostly software with no end-of-life
		// policy, so it is only worth a line when asked for.
		{"untracked stays quiet", resolve.Result{Findings: []timeline.Finding{f}, Untracked: []decl.Decl{{Product: "biome", Version: "2.5.13", Source: decl.Source{File: "mise.toml", Line: 5}}}}, []timeline.Finding{f}, cliArgs{}, nil, nil},
		{"untracked when asked for", resolve.Result{Findings: []timeline.Finding{f}, Untracked: []decl.Decl{{Product: "biome", Version: "2.5.13", Source: decl.Source{File: "mise.toml", Line: 5}}}}, []timeline.Finding{f}, cliArgs{verbose: true}, nil,
			[]string{"mise.toml:5: biome 2.5.13 is not tracked by endoflife.date", evidenceOfF}},
		// A dated declaration alongside an undated one is the whole answer,
		// so the undated one says nothing.
		{"undated stays quiet beside a row", resolve.Result{Findings: []timeline.Finding{f}, Undated: []timeline.Undated{undated}}, []timeline.Finding{f}, cliArgs{}, nil, nil},
		// A file nobody could parse earns no complaint: nothing is wrong
		// with a file mid-edit. What it costs is that the answer may be
		// short of what the directory declares, and that is worth a line
		// only when there is no answer for it to be short of.
		{"a skipped file stays quiet beside a row", resolve.Result{Findings: []timeline.Finding{f}}, []timeline.Finding{f}, cliArgs{}, skipped, nil},
		{"and is named when asked", resolve.Result{Findings: []timeline.Finding{f}}, []timeline.Finding{f}, cliArgs{verbose: true}, skipped,
			[]string{"compose.yml: not YAML, so nothing in it was read", evidenceOfF}},
		{"and is counted when there is no row", resolve.Result{}, nil, cliArgs{}, skipped,
			[]string{
				"nothing declared in dir is tracked by endoflife.date",
				"1 file was recognized and read nothing from; --verbose names it",
			}},
		{"several are counted together", resolve.Result{}, nil, cliArgs{},
			[]decl.Skipped{skipped[0], {File: "package.json", Reason: decl.NotJSON}},
			[]string{
				"nothing declared in dir is tracked by endoflife.date",
				"2 files were recognized and read nothing from; --verbose names them",
			}},
		// Naming them is what --verbose is, so the count is not said twice.
		{"named rather than counted when asked", resolve.Result{}, nil, cliArgs{verbose: true}, skipped,
			[]string{
				"compose.yml: not YAML, so nothing in it was read",
				"nothing declared in dir is tracked by endoflife.date",
			}},
		// A line with no date to place is not a file that could not be
		// read, and neither is a line following the newest release.
		{"quiet lines are not skipped files",
			resolve.Result{Moving: []decl.Unreadable{m}, Untracked: []decl.Decl{{Product: "biome", Source: decl.Source{File: "mise.toml", Line: 5}}}}, nil, cliArgs{}, nil,
			[]string{"everything declared in dir follows the newest release, so there is no date to place"}},
		// A window hid the rows rather than there being none, so the answer
		// is whole and the line about it is the window's.
		{"a window is not a short answer", resolve.Result{Findings: []timeline.Finding{f, ahead}}, []timeline.Finding{f},
			cliArgs{withinSet: true, withinText: "90d"}, skipped,
			[]string{"1 more declaration expires further out than 90d; drop --within to see it"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notes("dir", tt.r, tt.shown, tt.sk, tt.a)
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
		{"go install records the tag, without the v", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}}, "1.0.0"},
		{"only the leading v is dropped", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0-dev"}}, "1.0.0-dev"},
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
		sk       []decl.Skipped
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
			wantErr:  []string{"1 more declaration expires further out than 90d"},
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
			name:     "a file nobody could parse leaves the rows beside it alone",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{expired},
			sk:       []decl.Skipped{{File: "compose.yml", Reason: decl.NotYAML}},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantCode: exitPast,
		},
		{
			name:     "and is named beside them when asked",
			a:        cliArgs{dir: ".", verbose: true},
			ds:       []decl.Decl{expired},
			sk:       []decl.Skipped{{File: "compose.yml", Reason: decl.NotYAML}},
			wantOut:  "-2450d  2020-01-01  python 2.7  .python-version:1\n",
			wantErr:  []string{"compose.yml: not YAML, so nothing in it was read"},
			wantCode: exitPast,
		},
		{
			name:     "a cycle with no announced date is one line, not a complaint",
			a:        cliArgs{dir: "."},
			ds:       []decl.Decl{{Product: "nodejs", Version: "26.0.1", Source: at2(".nvmrc", 1)}},
			wantErr:  []string{"eolwhen: nothing declared in . has an end-of-life date yet\n"},
			wantCode: exitNone,
		},
		{
			name:     "and the line it was is named when asked",
			a:        cliArgs{dir: ".", verbose: true},
			ds:       []decl.Decl{{Product: "nodejs", Version: "26.0.1", Source: at2(".nvmrc", 1)}},
			wantErr:  []string{".nvmrc:1: nodejs 26 has no end-of-life date yet"},
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
			code := report(tt.a, testCatalog(t), tt.ds, tt.us, tt.sk, now, &so, &se)
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
	code := report(cliArgs{dir: "."}, nil, nil, us, nil, now, &so, &se)
	if code != exitNone || so.String() != "" {
		t.Errorf("report = %q, %d; want no rows and exit %d", so.String(), code, exitNone)
	}
	for _, want := range []string{"ubuntu-latest names latest", "nothing in . could be placed on the timeline"} {
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
//
// ubuntu-latest is also the line nobody can act on: it follows the newest
// runner image on purpose. The default answer is the one line saying so, and
// --verbose is what names the label.
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
	if code != exitNone || so != "" || !strings.Contains(se, "follows the newest release") {
		t.Errorf("run(%q) = %q, %q, %d", dir, so, se, code)
	}
	if strings.Contains(se, "ubuntu-latest") {
		t.Errorf("stderr = %q; want the label left to --verbose", se)
	}
	_, se, code = runArgs(t, "--verbose", dir)
	if code != exitNone || !strings.Contains(se, "ubuntu-latest names latest") {
		t.Errorf("run --verbose = %q, %d", se, code)
	}
	so, _, code = runArgs(t, "--json", "--verbose", dir)
	if code != exitNone || !strings.Contains(so, `"text": "ubuntu-latest"`) {
		t.Errorf("run --json = %q, %d", so, code)
	}
}

func TestReportJSON(t *testing.T) {
	var so, se bytes.Buffer
	ds := []decl.Decl{{Product: "python", Version: "2.7.18", Source: at2(".python-version", 1)}}
	us := []decl.Unreadable{{Source: at2(".nvmrc", 1), Text: "lts/iron", Reason: "names a moving target, not a version"}}
	code := report(cliArgs{dir: "some/dir", asJSON: true}, testCatalog(t), ds, us, nil, now, &so, &se)
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
	// The declarations with no date to place are the document's two
	// --verbose lists, the same ones stderr keeps quiet about.
	so.Reset()
	ds = append(ds,
		decl.Decl{Product: "nodejs", Version: "26.0.1", Source: at2(".nvmrc", 1)},
		decl.Decl{Product: "jq", Version: "1.8.1", Source: at2("mise.toml", 3)})
	report(cliArgs{dir: "some/dir", asJSON: true, verbose: true}, testCatalog(t), ds, us, nil, now, &so, &se)
	for _, want := range []string{`"cycle": "26"`, `"source": "mise.toml:3"`} {
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

// corpusDoc is a catalog for the directory below: a Python still supported,
// the Debian its image was built on, and the runner images.
const corpusDoc = `{"result":[
  {"name":"python","aliases":[],"releases":[
    {"name":"3.11","eolFrom":"2027-10-31"},
    {"name":"3.9","eolFrom":"2025-10-31"}
  ]},
  {"name":"debian","aliases":[],"releases":[
    {"name":"11","codename":"Bullseye","eolFrom":"2026-08-31"}
  ]},
  {"name":"github-actions-runner-images","aliases":[],"releases":[
    {"name":"ubuntu-22.04","eolFrom":"2027-04-01"}
  ]},
  {"name":"redis","aliases":[],"releases":[
    {"name":"8.0","eolFrom":null},
    {"name":"7.2","eolFrom":"2026-01-01"}
  ]},
  {"name":"opensearch","aliases":[],"identifiers":[
    {"type":"purl","id":"pkg:docker/opensearchproject/opensearch"}
  ],"releases":[
    {"name":"1.3","eolFrom":"2023-03-17"}
  ]},
  {"name":"rails","aliases":["ruby-on-rails"],"identifiers":[
    {"type":"purl","id":"pkg:gem/rails"}
  ],"releases":[
    {"name":"6.1","eolFrom":"2024-10-01"}
  ]},
  {"name":"postgresql","aliases":["pg"],"releases":[
    {"name":"13","eolFrom":"2025-11-13"}
  ]},
  {"name":"php","aliases":[],"releases":[
    {"name":"8.1","eolFrom":"2025-12-31"},
    {"name":"7.4","eolFrom":"2022-11-28"}
  ]},
  {"name":"laravel","aliases":[],"identifiers":[
    {"type":"purl","id":"pkg:composer/laravel/framework"}
  ],"releases":[
    {"name":"8","eolFrom":"2023-01-24"}
  ]}
]}`

// TestRunCorpus reads a directory of the shape a real project has, and is
// the test for what the answer leaves out as much as for what it says.
//
// Three of the four lines here used to be complaints on stderr that nobody
// could act on, and the one thing the directory most needed saying — the
// image is built on a Debian that stops getting security fixes first — was
// not said at all, because the version the tag carries was read and the
// distribution beside it was dropped.
func TestRunCorpus(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A multi-stage build names the same base twice, which is one thing to
	// deal with and not two.
	write("Dockerfile", "ARG REGISTRY=\n"+
		"FROM ${REGISTRY}python:3.11-bullseye AS builder\n"+
		"RUN pip install .\n"+
		"FROM ${REGISTRY}python:3.11-bullseye\n")
	// Third-party images: one endoflife.date publishes a purl for, one
	// nobody has, and a version older than anything it tracks.
	write("docker-compose.yml", "services:\n"+
		"  search:\n    image: opensearchproject/opensearch:1.3.0\n"+
		"  sandbox:\n    image: acme/sandbox:0.2.10\n"+
		"  cache:\n    image: redis:3.2\n")
	// A manifest: the framework the application sits on, and two gems
	// endoflife.date publishes no name for — one of which the catalog
	// answers to as an alias of something else entirely.
	write("Gemfile", "source \"https://rubygems.org\"\n"+
		"gem \"rails\", \"~> 6.1.0\"\n"+
		"gem \"pg\", \">= 1.1\"\n"+
		"gem \"puma\"\n")
	// A manifest whose framework is a declaration, whose extension
	// requirement is not a package at all, and whose php is the runtime the
	// whole thing sits on.
	write("composer.json", "{\n"+
		"  \"require\": {\n"+
		"    \"php\": \"^7.4\",\n"+
		"    \"ext-json\": \"*\",\n"+
		"    \"laravel/framework\": \"^8.0\",\n"+
		"    \"monolog/monolog\": \"^2.0\"\n"+
		"  }\n"+
		"}\n")
	write(filepath.Join(".github", "workflows", "ci.yml"), "jobs:\n"+
		"  lint:\n"+
		"    runs-on: ubuntu-latest\n"+
		"  test:\n"+
		"    strategy:\n"+
		"      matrix:\n"+
		"        os: [ubuntu-22.04]\n"+
		"        python-version: ['3.9']\n"+
		"    runs-on: ${{ matrix.os }}\n"+
		"    steps:\n"+
		"      - uses: actions/setup-python@v5\n"+
		"        with:\n"+
		"          python-version: ${{ matrix.python-version }}\n")

	ds, us, sk, err := scan.Dir(dir)
	if err != nil {
		t.Fatalf("scan.Dir: %v", err)
	}
	c, err := catalog.Decode([]byte(corpusDoc))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var so, se bytes.Buffer
	code := report(cliArgs{dir: dir}, c, ds, us, sk, now, &so, &se)
	want := "" +
		"-1388d  2022-11-28  php 7.4                                    composer.json:3\n" +
		"-1331d  2023-01-24  laravel 8                                  composer.json:5\n" +
		"-1279d  2023-03-17  opensearch 1.3                             docker-compose.yml:3\n" +
		" -715d  2024-10-01  rails 6.1                                  Gemfile:2\n" +
		" -320d  2025-10-31  python 3.9                                 .github/workflows/ci.yml:8\n" +
		" -258d  2026-01-01  redis <7.2                                 docker-compose.yml:7\n" +
		"  -16d  2026-08-31  debian 11                                  Dockerfile:2,4\n" +
		" +197d  2027-04-01  github-actions-runner-images ubuntu-22.04  .github/workflows/ci.yml:7\n" +
		" +410d  2027-10-31  python 3.11                                Dockerfile:2,4\n"
	if so.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", so.String(), want)
	}
	// What is left over is ubuntu-latest, which follows the newest runner
	// image on purpose; acme/sandbox, which endoflife.date publishes no
	// image for; the gems and packages it publishes no name for, pg among
	// them, which the catalog would otherwise have answered as PostgreSQL;
	// and ext-json, which is not software with a calendar of its own. None
	// is a line anyone can act on, so the default answer says nothing at
	// all.
	if se.String() != "" {
		t.Errorf("stderr = %q; want empty", se.String())
	}
	if code != exitPast {
		t.Errorf("code = %d; want %d", code, exitPast)
	}
}

// A directory whose recognized files all failed to parse reads exactly like
// one that declares nothing, which is what this tells apart. Nothing here
// has to be looked up, so the catalog is never asked for and the test does
// not touch the network.
func TestRunSetsAsideWhatItCannotParse(t *testing.T) {
	dir := t.TempDir()
	// A file mid-edit may hold anything at all, and what it holds is the
	// one thing a diagnostic must not pass on.
	body := "\tthis: is: not: yaml\n  POSTGRES_PASSWORD: hunter2\n  - [\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	_, quiet, code := runArgs(t, dir)
	if code != exitNone {
		t.Errorf("code = %d; want %d", code, exitNone)
	}
	for _, want := range []string{"no version declarations in", "1 file was recognized and read nothing from"} {
		if !strings.Contains(quiet, want) {
			t.Errorf("stderr = %q; want %q in it", quiet, want)
		}
	}
	if strings.Contains(quiet, "compose.yml") {
		t.Errorf("stderr = %q; want which file left to --verbose", quiet)
	}

	_, loud, _ := runArgs(t, "--verbose", dir)
	if want := "compose.yml: not YAML, so nothing in it was read"; !strings.Contains(loud, want) {
		t.Errorf("stderr = %q; want %q in it", loud, want)
	}

	// The words on stderr and the fields in the document say the same
	// thing, which is what the document promises.
	doc, _, _ := runArgs(t, "--json", dir)
	for _, want := range []string{`"source": "compose.yml"`, `"reason": "not YAML"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("stdout = %q; want %q in it", doc, want)
		}
	}
	for _, out := range []string{quiet, loud, doc} {
		if strings.Contains(out, "hunter2") {
			t.Errorf("output = %q; want nothing the file itself held", out)
		}
	}
}

// A directory that declares nothing and hid nothing says so and stops there.
func TestRunSaysNothingWasSetAsideWhenNothingWas(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name": "demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	so, se, code := runArgs(t, "--json", dir)
	if code != exitNone {
		t.Errorf("code = %d; want %d", code, exitNone)
	}
	if strings.Contains(se, "read nothing from") {
		t.Errorf("stderr = %q; want no count for a directory that hid nothing", se)
	}
	if !strings.Contains(so, `"skipped": []`) {
		t.Errorf("stdout = %q; want an empty skipped list", so)
	}
}

// TestReportExplainsHowARowWasReached: a row is the same three columns
// however it was arrived at, so the table stays as it was and --verbose is
// where the difference is said. The day carried over from the oldest cycle
// endoflife.date tracks is the one that most needs saying, printing like a
// published date without being one.
func TestReportExplainsHowARowWasReached(t *testing.T) {
	c, err := catalog.Decode([]byte(`{"result":[
	  {"name":"redis","aliases":[],"links":{"html":"https://endoflife.date/redis"},
	   "releases":[{"name":"7.2","eolFrom":"2026-01-01"}]},
	  {"name":"nodejs","aliases":["node"],"links":{"html":"https://endoflife.date/nodejs"},
	   "releases":[{"name":"22","eolFrom":"2027-04-30"}]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	ds := []decl.Decl{
		{Product: "redis", Version: "3.2", Source: at2("compose.yml", 9)},
		{Product: "node", Version: "22.11.0", Source: at2(".nvmrc", 1)},
	}

	// The rows and the exit code are what they were before any of this was
	// carried, and the default run says nothing extra.
	var so, se bytes.Buffer
	if code := report(cliArgs{dir: "."}, c, ds, nil, nil, now, &so, &se); code != exitPast {
		t.Errorf("code = %d; want %d", code, exitPast)
	}
	want := "-258d  2026-01-01  redis <7.2  compose.yml:9\n" +
		"+226d  2027-04-30  nodejs 22   .nvmrc:1\n"
	if so.String() != want {
		t.Errorf("stdout =\n%q\nwant\n%q", so.String(), want)
	}
	if se.String() != "" {
		t.Errorf("stderr = %q; want the rows to stay as they were", se.String())
	}

	// Asked for detail, each row says what it was worked out from, in the
	// order the rows are printed in.
	so.Reset()
	se.Reset()
	report(cliArgs{dir: ".", verbose: true}, c, ds, nil, nil, now, &so, &se)
	wantErr := "eolwhen: compose.yml:9: 3.2 is redis <7.2, matched by name; " +
		"no cycle covers it, so the date is the day 7.2 ended, which support for anything older had run out by" +
		" — https://endoflife.date/redis\n" +
		"eolwhen: .nvmrc:1: 22.11.0 is nodejs 22, matched by an alias endoflife.date lists; " +
		"the date is that cycle's own — https://endoflife.date/nodejs\n"
	if se.String() != wantErr {
		t.Errorf("stderr =\n%q\nwant\n%q", se.String(), wantErr)
	}

	// In the document the same difference is a field rather than a
	// sentence, so a caller never has to read the prose to find it — and
	// never has to read the cycle name either, which is the reason the
	// field is there rather than the < the table shows: Go writes that
	// character escaped, so a caller looking for one in the document finds
	// \u003c or nothing at all.
	so.Reset()
	se.Reset()
	report(cliArgs{dir: ".", asJSON: true}, c, ds, nil, nil, now, &so, &se)
	for _, w := range []string{
		`"version": "3.2"`, `"cycle": "\u003c7.2"`, `"dated": "predates"`, `"matched": "name"`,
		`"link": "https://endoflife.date/redis"`,
		`"version": "22.11.0"`, `"dated": "cycle"`, `"matched": "alias"`,
	} {
		if !strings.Contains(so.String(), w) {
			t.Errorf("stdout is missing %s:\n%s", w, so.String())
		}
	}
}
