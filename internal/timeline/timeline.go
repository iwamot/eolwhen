// Package timeline turns matched declarations into the one ordered list that
// is this tool's answer, and renders it. Nothing here does I/O.
package timeline

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Finding is one declaration placed on the timeline: what it names, which
// release cycle it falls in, and the day that cycle goes out of support.
type Finding struct {
	Product string
	Cycle   string
	EOL     time.Time
	Source  decl.Source
}

// What names the software and cycle, as one column.
func (f Finding) What() string { return f.Product + " " + f.Cycle }

// Undated is a declaration that reached a release cycle endoflife.date has
// given no end date. The line was read and the cycle was found; only the
// date is missing.
//
// There are two ways to be here and they are opposite. Support has not been
// dated yet, which is what a directory running current versions looks like
// and is nothing to do. Or upstream says support has ended and published no
// date for it, which is everything to do and still has no day to put on a
// timeline. The two are kept in separate lists for that reason, and
// --verbose says which lines were which.
type Undated struct {
	Product string
	Cycle   string
	Source  decl.Source
}

// Days is how many days away the end of support is, as of now: negative
// once it has passed, positive while it is ahead.
//
// Both are reduced to a calendar day first, each in its own reckoning: the
// end-of-life date carries no zone because endoflife.date publishes a date
// and not a moment, and now is counted on the calendar of whoever is asking.
// Anything else puts a date and a number that disagree on the same row —
// east of Greenwich, a cycle ending on the day shown would read as a day
// away for the first hours of it.
func Days(now time.Time, f Finding) int {
	return int(day(f.EOL).Sub(day(now)) / (24 * time.Hour))
}

// day drops everything below the date, keeping the year, month, and day as
// they read in whatever zone t carries.
func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Past reports whether support has already ended. The end-of-life date is
// the first day without support, so a finding due today is already past.
func Past(now time.Time, f Finding) bool { return Days(now, f) <= 0 }

// Sort orders the list by the day itself, oldest first, so the most overdue
// reads at the top and the timeline runs in one direction. Findings sharing
// a day are ordered by what they name, then by where they were found, so the
// output does not move between runs — and so the places a row lists read
// down the directory in one direction too.
//
// Where they were found is compared as a path and then as a number, because
// a Dockerfile that names the same image in four stages would otherwise put
// line 8 after line 62.
func Sort(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if !fs[i].EOL.Equal(fs[j].EOL) {
			return fs[i].EOL.Before(fs[j].EOL)
		}
		if fs[i].What() != fs[j].What() {
			return fs[i].What() < fs[j].What()
		}
		return fs[i].Source.Before(fs[j].Source)
	})
}

// Within drops findings further ahead than d. Past findings are always kept:
// the window narrows what is coming, never what has already expired.
func Within(fs []Finding, now time.Time, d time.Duration) []Finding {
	limit := int(d / (24 * time.Hour))
	var out []Finding
	for _, f := range fs {
		if Past(now, f) || Days(now, f) <= limit {
			out = append(out, f)
		}
	}
	return out
}

// Counts reports how many findings are behind and ahead, which is what the
// exit code is made of.
func Counts(fs []Finding, now time.Time) (past, future int) {
	for _, f := range fs {
		if Past(now, f) {
			past++
		} else {
			future++
		}
	}
	return past, future
}

// Distance renders how far off the end of support is, for the first column.
//
// The sign says which side of today a date falls on, so the day it lands on
// carries none: +0d reads as still to come, for something that has already
// arrived.
func Distance(now time.Time, f Finding) string {
	d := Days(now, f)
	if d == 0 {
		return "0d"
	}
	return fmt.Sprintf("%+dd", d)
}

// Table renders the findings as aligned columns: the signed distance in
// days, the day itself, what it names, and every place it was declared.
// There is no header, so every line of stdout is a finding and awk can read
// the columns without skipping one.
//
// One row is one release cycle, however many lines declared it. What the
// reader has to deal with is that Debian 11 stops getting security fixes,
// and a multi-stage Dockerfile naming the same base three times is one thing
// to deal with and not three; the places follow as the list of what to go
// and change.
func Table(fs []Finding, now time.Time) string {
	rows := make([][3]string, 0, len(fs))
	var where []string
	for _, group := range fold(fs) {
		f := group[0]
		rows = append(rows, [3]string{
			Distance(now, f),
			f.EOL.Format(time.DateOnly),
			f.What(),
		})
		where = append(where, sources(group))
	}
	var width [3]int
	for _, r := range rows {
		for i := range r {
			width[i] = max(width[i], len(r[i]))
		}
	}
	var b strings.Builder
	for i, r := range rows {
		fmt.Fprintf(&b, "%*s  %-*s  %-*s  %s\n",
			width[0], r[0], width[1], r[1], width[2], r[2], where[i])
	}
	return b.String()
}

// fold groups the findings that name the same release cycle, keeping the
// order they were sorted into: the first of a cycle is where its row goes.
// A cycle settles its own end date, so everything a group holds shares a
// day as well as a name.
func fold(fs []Finding) [][]Finding {
	var out [][]Finding
	at := map[string]int{}
	for _, f := range fs {
		k := f.What()
		if i, seen := at[k]; seen {
			out[i] = append(out[i], f)
			continue
		}
		at[k] = len(out)
		out = append(out, []Finding{f})
	}
	return out
}

// sources renders where a cycle was declared: every place, in the order they
// were sorted into, with a file named once however many of its lines
// declared it. A multi-stage Dockerfile reads as Dockerfile:2,22,34 rather
// than three times over, which is what keeps the list short enough to be
// read where it matters most — the repository declaring one base image
// everywhere.
func sources(fs []Finding) string {
	var files []string
	lines := map[string][]string{}
	for _, f := range fs {
		file := f.Source.File
		if _, seen := lines[file]; !seen {
			files = append(files, file)
		}
		line := strconv.Itoa(f.Source.Line)
		// A file with no meaningful line to point at is named alone, and a
		// line declaring the same cycle twice is still the one place.
		if f.Source.Line == 0 || slices.Contains(lines[file], line) {
			continue
		}
		lines[file] = append(lines[file], line)
	}
	var out []string
	for _, file := range files {
		if len(lines[file]) == 0 {
			out = append(out, file)
			continue
		}
		out = append(out, file+":"+strings.Join(lines[file], ","))
	}
	return strings.Join(out, ", ")
}

// Report is one run's whole answer, as the document renders it.
type Report struct {
	Directory  string
	Findings   []Finding
	Unreadable []decl.Unreadable
	// Hidden is how many findings a window kept out of Findings.
	Hidden int
	// Moving are the lines that follow the newest release by design. Like
	// Untracked and Undated, they are filled only when they were asked for.
	Moving []decl.Unreadable
	// Untracked and Undated are filled only when they were asked for. Both
	// are declarations with no date to place — software endoflife.date has
	// no policy for, and cycles it has not dated yet — and a tool list holds
	// enough of the first to bury the answer.
	Untracked []decl.Decl
	Undated   []Undated
	// Ended are the cycles upstream says are out of support without saying
	// when. They earn no row, having no date, but they are not news the
	// reader has to ask for, so unlike the three above they are always
	// filled.
	Ended []Undated
	// Skipped are the files that were recognized and read nothing from.
	// Every other list here holds declarations; this one holds the files
	// whose declarations were never reached, which is what says an empty
	// answer is short rather than complete. Like Ended it is always filled,
	// because the count of it is said in words whether or not it was asked
	// for.
	Skipped []decl.Skipped
}

type document struct {
	Directory  string       `json:"directory"`
	Findings   []entry      `json:"findings"`
	Unreadable []unreadable `json:"unreadable"`
	Hidden     int          `json:"hidden"`
	Moving     []unreadable `json:"moving"`
	Untracked  []untracked  `json:"untracked"`
	Undated    []undated    `json:"undated"`
	Ended      []undated    `json:"ended"`
	Skipped    []skipped    `json:"skipped"`
}

// skipped names a file that was read nothing from, and why. It carries no
// line: the whole of the file was set aside, and the line a parser stopped
// on is not where the reader has to go. The reasons are the closed set decl
// declares, so a caller tells one from another without reading the prose.
type skipped struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type untracked struct {
	Source  string `json:"source"`
	Product string `json:"product"`
	Version string `json:"version"`
}

// undated names the cycle the version reached, which is the line to watch
// for a date, rather than the version as it was written.
type undated struct {
	Source  string `json:"source"`
	Product string `json:"product"`
	Cycle   string `json:"cycle"`
}

func cycleOf(u Undated) undated {
	return undated{
		Source:  u.Source.String(),
		Product: u.Product,
		Cycle:   u.Cycle,
	}
}

type entry struct {
	Product string `json:"product"`
	Cycle   string `json:"cycle"`
	EOL     string `json:"eol"`
	Days    int    `json:"days"`
	Past    bool   `json:"past"`
	Source  string `json:"source"`
}

// unreadable carries the product apart from the text when the file named
// them apart, and an empty product when the text is the whole of what was
// written, as with an image reference.
type unreadable struct {
	Source  string `json:"source"`
	Product string `json:"product"`
	Text    string `json:"text"`
	Reason  string `json:"reason"`
}

func lineOf(u decl.Unreadable) unreadable {
	return unreadable{
		Source:  u.Source.String(),
		Product: u.Product,
		Text:    u.Text,
		Reason:  u.Reason,
	}
}

// JSON renders the same answer as one document. Everything the table needs
// said in words on stderr is a field here instead, so a caller reading the
// document is owed nothing it cannot see.
func JSON(r Report, now time.Time) string {
	doc := document{
		Directory:  r.Directory,
		Findings:   []entry{},
		Unreadable: []unreadable{},
		Hidden:     r.Hidden,
		Moving:     []unreadable{},
		Untracked:  []untracked{},
		Undated:    []undated{},
		Ended:      []undated{},
		Skipped:    []skipped{},
	}
	for _, f := range r.Findings {
		doc.Findings = append(doc.Findings, entry{
			Product: f.Product,
			Cycle:   f.Cycle,
			EOL:     f.EOL.Format(time.DateOnly),
			Days:    Days(now, f),
			Past:    Past(now, f),
			Source:  f.Source.String(),
		})
	}
	for _, u := range r.Unreadable {
		doc.Unreadable = append(doc.Unreadable, lineOf(u))
	}
	for _, u := range r.Moving {
		doc.Moving = append(doc.Moving, lineOf(u))
	}
	for _, d := range r.Untracked {
		doc.Untracked = append(doc.Untracked, untracked{
			Source:  d.Source.String(),
			Product: d.Product,
			Version: d.Version,
		})
	}
	for _, u := range r.Undated {
		doc.Undated = append(doc.Undated, cycleOf(u))
	}
	for _, u := range r.Ended {
		doc.Ended = append(doc.Ended, cycleOf(u))
	}
	for _, s := range r.Skipped {
		doc.Skipped = append(doc.Skipped, skipped{Source: s.File, Reason: s.Reason})
	}
	// The document holds only strings, numbers, and booleans, so Marshal
	// cannot fail.
	b, _ := json.MarshalIndent(doc, "", "  ")
	return string(b) + "\n"
}
