package timeline

import (
	"strings"
	"testing"
	"time"

	"github.com/iwamot/eolwhen/internal/decl"
)

var now = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

func at(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func find(product, cycle, eol, file string) Finding {
	return Finding{Product: product, Cycle: cycle, EOL: at(eol), Source: decl.Source{File: file}}
}

func TestWhat(t *testing.T) {
	if got := find("python", "2.7", "2020-01-01", "x").What(); got != "python 2.7" {
		t.Errorf("What = %q; want python 2.7", got)
	}
}

func TestDaysAndPast(t *testing.T) {
	for _, tt := range []struct {
		name string
		eol  string
		days int
		past bool
	}{
		{"long gone", "2020-01-01", -2450, true},
		{"yesterday", "2026-09-15", -1, true},
		// The end-of-life date is the first day without support, so a
		// finding due today has already passed.
		{"today", "2026-09-16", 0, true},
		{"tomorrow", "2026-09-17", 1, false},
		{"far ahead", "2027-04-17", 213, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := find("x", "1", tt.eol, "f")
			if got := Days(now, f); got != tt.days {
				t.Errorf("Days = %d; want %d", got, tt.days)
			}
			if got := Past(now, f); got != tt.past {
				t.Errorf("Past = %v; want %v", got, tt.past)
			}
		})
	}
}

// TestDaysIgnoresTheClock pins that the answer is a count of days, not of
// 24-hour spans: asking at one minute to midnight gives the same number as
// asking at noon.
func TestDaysIgnoresTheClock(t *testing.T) {
	late := time.Date(2026, 9, 16, 23, 59, 0, 0, time.UTC)
	f := find("x", "1", "2026-09-17", "f")
	if a, b := Days(now, f), Days(late, f); a != b || a != 1 {
		t.Errorf("Days = %d and %d; want 1 and 1", a, b)
	}
}

func TestSort(t *testing.T) {
	fs := []Finding{
		find("nodejs", "24", "2028-04-30", "b"),
		find("python", "2.7", "2020-01-01", "a"),
		find("ruby", "2.6", "2022-03-31", "c"),
		// Same day: ordered by what, then by where.
		find("zsh", "1", "2022-03-31", "a"),
	}
	Sort(fs)
	want := []string{"python 2.7", "ruby 2.6", "zsh 1", "nodejs 24"}
	for i, w := range want {
		if fs[i].What() != w {
			t.Errorf("Sort[%d] = %q; want %q", i, fs[i].What(), w)
		}
	}
}

func TestSortSameDaySameName(t *testing.T) {
	fs := []Finding{
		{Product: "go", Cycle: "1", EOL: at("2020-01-01"), Source: decl.Source{File: "z"}},
		{Product: "go", Cycle: "1", EOL: at("2020-01-01"), Source: decl.Source{File: "a"}},
	}
	Sort(fs)
	if fs[0].Source.File != "a" {
		t.Errorf("Sort put %q first; want a", fs[0].Source.File)
	}
}

// TestSortSameFileByLine: a Dockerfile naming the same image in four stages
// reads down the file, so line 8 comes before line 62 rather than after it.
func TestSortSameFileByLine(t *testing.T) {
	var fs []Finding
	for _, line := range []int{62, 8, 50, 41} {
		fs = append(fs, Finding{Product: "nodejs", Cycle: "24", EOL: at("2028-04-30"), Source: decl.Source{File: "Dockerfile", Line: line}})
	}
	Sort(fs)
	for i, want := range []int{8, 41, 50, 62} {
		if fs[i].Source.Line != want {
			t.Errorf("Sort[%d] = line %d; want %d", i, fs[i].Source.Line, want)
		}
	}
}

func TestWithin(t *testing.T) {
	fs := []Finding{
		find("python", "2.7", "2020-01-01", "a"),
		find("nodejs", "22", "2026-10-01", "b"),
		find("nodejs", "24", "2028-04-30", "c"),
	}
	// The window narrows what is coming and never what has expired.
	got := Within(fs, now, 90*24*time.Hour)
	if len(got) != 2 || got[0].What() != "python 2.7" || got[1].What() != "nodejs 22" {
		t.Fatalf("Within = %+v; want the past one and the near one", got)
	}
	if got := Within(fs, now, 0); len(got) != 1 || got[0].What() != "python 2.7" {
		t.Errorf("Within(0) = %+v; want the past one alone", got)
	}
}

func TestCounts(t *testing.T) {
	fs := []Finding{
		find("python", "2.7", "2020-01-01", "a"),
		find("ruby", "2.6", "2022-03-31", "b"),
		find("nodejs", "24", "2028-04-30", "c"),
	}
	past, future := Counts(fs, now)
	if past != 2 || future != 1 {
		t.Errorf("Counts = %d, %d; want 2, 1", past, future)
	}
	if past, future := Counts(nil, now); past != 0 || future != 0 {
		t.Errorf("Counts(nil) = %d, %d; want 0, 0", past, future)
	}
}

func TestTable(t *testing.T) {
	fs := []Finding{
		{Product: "python", Cycle: "2.7", EOL: at("2020-01-01"), Source: decl.Source{File: ".python-version", Line: 1}},
		{Product: "nodejs", Cycle: "24", EOL: at("2028-04-30"), Source: decl.Source{File: ".nvmrc"}},
	}
	want := "-2450d  2020-01-01  python 2.7  .python-version:1\n" +
		" +592d  2028-04-30  nodejs 24   .nvmrc\n"
	if got := Table(fs, now); got != want {
		t.Errorf("Table =\n%q\nwant\n%q", got, want)
	}
	if got := Table(nil, now); got != "" {
		t.Errorf("Table(nil) = %q; want empty", got)
	}
}

// TestTableFolds: one row is one release cycle, however many lines declared
// it. A multi-stage Dockerfile naming the same base three times is one thing
// to deal with and not three, and the places follow as the list of what to
// go and change — a file named once, with its lines behind it.
func TestTableFolds(t *testing.T) {
	at2 := func(file string, line int) decl.Source { return decl.Source{File: file, Line: line} }
	fs := []Finding{
		{Product: "debian", Cycle: "11", EOL: at("2026-08-31"), Source: at2("Dockerfile", 2)},
		{Product: "debian", Cycle: "11", EOL: at("2026-08-31"), Source: at2("Dockerfile", 22)},
		{Product: "debian", Cycle: "11", EOL: at("2026-08-31"), Source: at2("Dockerfile", 34)},
		{Product: "python", Cycle: "3.11", EOL: at("2027-10-31"), Source: at2(".github/workflows/lint.yml", 14)},
		{Product: "python", Cycle: "3.11", EOL: at("2027-10-31"), Source: at2("Dockerfile", 2)},
		{Product: "python", Cycle: "3.11", EOL: at("2027-10-31"), Source: at2("Dockerfile", 22)},
	}
	want := " -16d  2026-08-31  debian 11    Dockerfile:2,22,34\n" +
		"+410d  2027-10-31  python 3.11  .github/workflows/lint.yml:14, Dockerfile:2,22\n"
	if got := Table(fs, now); got != want {
		t.Errorf("Table =\n%q\nwant\n%q", got, want)
	}
}

// TestTableFoldsRepeats: the same cycle declared twice at one line is still
// the one place, and a file with no line to point at is named alone.
func TestTableFoldsRepeats(t *testing.T) {
	fs := []Finding{
		{Product: "nodejs", Cycle: "14", EOL: at("2023-04-30"), Source: decl.Source{File: ".nvmrc"}},
		{Product: "nodejs", Cycle: "14", EOL: at("2023-04-30"), Source: decl.Source{File: "mise.toml", Line: 2}},
		{Product: "nodejs", Cycle: "14", EOL: at("2023-04-30"), Source: decl.Source{File: "mise.toml", Line: 2}},
	}
	want := "-1235d  2023-04-30  nodejs 14  .nvmrc, mise.toml:2\n"
	if got := Table(fs, now); got != want {
		t.Errorf("Table = %q; want %q", got, want)
	}
}

// TestDistance: the sign says which side of today a date falls on, so the
// day it lands on carries none. +0d would read as still to come for
// something that has already arrived.
func TestDistance(t *testing.T) {
	for _, tt := range []struct {
		name string
		eol  string
		want string
	}{
		{"long gone", "2020-01-01", "-2450d"},
		{"yesterday", "2026-09-15", "-1d"},
		{"today", "2026-09-16", "0d"},
		{"tomorrow", "2026-09-17", "+1d"},
		{"far ahead", "2027-04-17", "+213d"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Distance(now, find("x", "1", tt.eol, "f")); got != tt.want {
				t.Errorf("Distance = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestTableShowsTheBoundaryWithoutASign keeps the column and the rule
// together: a row for the day itself is the one the reader most needs to
// read correctly.
func TestTableShowsTheBoundaryWithoutASign(t *testing.T) {
	fs := []Finding{{Product: "gitlab-runner", Cycle: "19.1", EOL: at("2026-09-16"), Source: decl.Source{File: "mise.toml", Line: 2}}}
	want := "0d  2026-09-16  gitlab-runner 19.1  mise.toml:2\n"
	if got := Table(fs, now); got != want {
		t.Errorf("Table = %q; want %q", got, want)
	}
}

func TestJSON(t *testing.T) {
	fs := []Finding{{
		Product: "python", Cycle: "2.7", Version: "2.7.18",
		Matched: ByName, Dated: DatedByCycle, Page: "https://endoflife.date/python",
		EOL: at("2020-01-01"), Source: decl.Source{File: ".python-version", Line: 1},
	}}
	us := []decl.Unreadable{{Source: decl.Source{File: ".nvmrc", Line: 1}, Product: "nodejs", Text: "lts/hydrogen", Reason: "names a moving target, not a version"}}
	moving := []decl.Unreadable{{Source: decl.Source{File: "compose.yml", Line: 2}, Text: "postgres:latest", Reason: "names latest, not a version"}}
	got := JSON(Report{Directory: "some/dir", Findings: fs, Unreadable: us, Hidden: 2, Moving: moving,
		Untracked: []decl.Decl{{Product: "biome", Version: "2.5.13", Source: decl.Source{File: "mise.toml", Line: 5}}},
		Undated:   []Undated{{Product: "go", Cycle: "1.26", Source: decl.Source{File: "go.mod", Line: 9}}},
		Ended:     []Undated{{Product: "metabase", Cycle: "0.46", Source: decl.Source{File: "compose.yml", Line: 4}}},
		Skipped:   []decl.Skipped{{File: "package.json", Reason: decl.NotJSON}}}, now)
	for _, want := range []string{
		`"directory": "some/dir"`,
		`"product": "python"`,
		`"cycle": "2.7"`,
		`"eol": "2020-01-01"`,
		`"days": -2450`,
		`"past": true`,
		`"source": ".python-version:1"`,
		// What the row was worked out from: the version the file wrote,
		// which the cycle no longer shows, how the name was answered, and
		// that the day is the cycle's own rather than one carried over.
		`"version": "2.7.18"`,
		`"matched": "name"`,
		`"dated": "cycle"`,
		`"link": "https://endoflife.date/python"`,
		`"product": "nodejs"`,
		`"text": "lts/hydrogen"`,
		// What a window hid, and what --verbose asked about, are fields here
		// rather than sentences on stderr.
		`"hidden": 2`,
		`"product": "biome"`,
		`"source": "go.mod:9"`,
		`"cycle": "1.26"`,
		// A cycle upstream calls over without dating it is its own list, so
		// a caller reading the document is never left to infer which kind
		// of undated a cycle was.
		`"source": "compose.yml:4"`,
		`"product": "metabase"`,
		// A line following the newest release is a field here too, so that
		// --json owes a caller nothing --verbose would have said.
		`"text": "postgres:latest"`,
		// A file that was read nothing from names itself and the closed-set
		// reason, and carries no line: the whole of it was set aside.
		`"source": "package.json"`,
		`"reason": "not JSON"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON is missing %s:\n%s", want, got)
		}
	}
	// Every list is always an array, so a caller can index without a nil
	// check, and the lists --verbose fills are empty unless they were asked
	// for.
	empty := JSON(Report{Directory: "."}, now)
	for _, want := range []string{`"findings": []`, `"unreadable": []`, `"moving": []`, `"untracked": []`, `"undated": []`, `"ended": []`, `"hidden": 0`} {
		if !strings.Contains(empty, want) {
			t.Errorf("JSON with nothing is missing %s:\n%s", want, empty)
		}
	}
}

// TestDaysUsesTheAskersCalendar: the count and the date on the same row have
// to agree for whoever is reading them. Nine hours east of Greenwich, a cycle
// ending on the day the row shows must read as today and not as tomorrow.
func TestDaysUsesTheAskersCalendar(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	// 00:30 on the 17th in Tokyo, which is still the 16th in UTC.
	morning := time.Date(2026, 9, 17, 0, 30, 0, 0, jst)
	f := find("gitlab-runner", "19.1", "2026-09-17", "mise.toml")
	if got := Days(morning, f); got != 0 {
		t.Errorf("Days = %d; want 0, the day the row shows", got)
	}
	if !Past(morning, f) {
		t.Error("a cycle ending today has already ended")
	}
	// West of Greenwich the same holds: late on the 16th in Los Angeles is
	// the 17th in UTC, and the row still reads a day ahead.
	pst := time.FixedZone("PST", -8*60*60)
	evening := time.Date(2026, 9, 16, 20, 0, 0, 0, pst)
	if got := Days(evening, f); got != 1 {
		t.Errorf("Days = %d; want 1, the day after where the reader is", got)
	}
}

// The two sides of one convention: a version below everything tracked is
// named after the oldest cycle it sits under, and the name reads back to
// that cycle so a sentence can say which day it was.
func TestPredatingCycle(t *testing.T) {
	name := PredatingCycle("4.0")
	if name != "<4.0" {
		t.Errorf("PredatingCycle = %q; want %q", name, "<4.0")
	}
	if got := OldestCycle(name); got != "4.0" {
		t.Errorf("OldestCycle(%q) = %q; want %q", name, got, "4.0")
	}
}
