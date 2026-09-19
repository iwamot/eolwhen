// Command eolwhen reads the version declarations in a directory and says
// which of them have gone out of support, and which are about to.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/iwamot/eolwhen/internal/catalog"
	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/period"
	"github.com/iwamot/eolwhen/internal/resolve"
	"github.com/iwamot/eolwhen/internal/scan"
	"github.com/iwamot/eolwhen/internal/timeline"
)

// Exit codes. 1 and 2 are both answers, not errors, and they are separate
// because the next step differs: something has already expired and needs
// work now, or nothing has and there is only a date to put in the calendar.
const (
	exitNone    = 0
	exitPast    = 1
	exitFuture  = 2
	exitUsage   = 3
	exitCatalog = 4
)

const devVersion = "0.0.0-dev"

var version = devVersion

const helpText = `eolwhen — say which of a directory's declared versions are out of support.

Usage:
  eolwhen [options] [DIR]

Examples:
  eolwhen
  eolwhen ../some-neglected-checkout
  eolwhen --within 90d

DIR defaults to the current directory. It does not have to be a git
repository. It is searched to the bottom, skipping directories that hold
somebody else's code (node_modules, vendor, .venv and the like), and one
directory is read per run.

Declarations are read from the runtime version files (.python-version,
.nvmrc, .ruby-version, go.mod, ...), the tool lists (mise.toml,
.tool-versions), the FROM lines of Dockerfiles and the images of Compose
services, the frameworks named in Gemfile, composer.json, package.json,
pyproject.toml, requirements.txt and pom.xml, the target framework of .NET
project files, and the runners and setup-* versions of GitHub workflows,
then matched against endoflife.date. How each kind of line is read:
https://github.com/iwamot/eolwhen/blob/main/docs/reading.md

Options:
  --within DUR    only show what expires within DUR (1d, 36h, 2w); what has
                  already expired is always shown
  --json          print JSON instead of the table, one entry per declaration
  --verbose       also say how each printed row was reached, and name what
                  got no row: software endoflife.date does not track, cycles
                  it has not dated yet, lines that follow the newest
                  release, and files that were read nothing from
  -h, --help      show this help
  -v, --version   show the version
  --instructions  print the paragraph for an agent's instruction file

Output:
  One row per release cycle: the days until support ends (0 on the day it
  ends, negative after), the date, the software and its release cycle, and
  every place it was declared, as Dockerfile:2,22,34. Rows go to stdout;
  everything else is an ` + "`eolwhen:`" + ` line on stderr. The exit code answers for
  the rows that were printed, so --within narrows what it covers too.

Exit codes:
  0  nothing to report, including a directory with no declarations
  1  something is already out of support
  2  nothing has expired, but something will
  3  usage error: fix the flags or the directory
  4  endoflife.date could not be read: check the network, then retry
`

// instructionsText is the paragraph an agent needs in order to use eolwhen:
// what it answers, what the argument is, and what each exit code means for
// what to do next. README.md quotes it verbatim.
const instructionsText = "To find out whether the runtimes, base images and frameworks a directory declares are still supported, use `eolwhen` instead of reading version files and checking dates by hand: `eolwhen` for the current directory, or `eolwhen DIR` for another one. It reads the version declarations in that one directory, matches them against endoflife.date, and prints one row per release cycle with the days until support ends, 0 on the day it ends and negative after, followed by every place that cycle was declared — a file is named once with its lines behind it, as `Dockerfile:2,22,34`, and `--json` has one entry per declaration instead. Add `--within 90d` to hide what expires further out than that; what has already expired is always shown, and the exit code then answers only for the rows that were printed. Exit 1 means something is already out of support and exit 2 means something will be, so both are answers and neither is a failure; exit 0 means no row was printed, and the single `eolwhen:` line says why — most often that nothing declared has an end-of-life date yet, which is nothing to do; exit 3 is a usage error and exit 4 means endoflife.date could not be read, which is worth one retry. Only rows go to stdout, so awk can read the first three columns; lines it could not read, and anything else the answer needs said in words, are `eolwhen:` lines on stderr; a line that follows the newest release on purpose, such as `ubuntu-latest`, is not one of them and only `--verbose` names it. A recognized file that does not parse is read as nothing at all, whole rather than in part; when there is no row the `eolwhen:` line counts those files, `--verbose` names each one with a short reason and `--json` carries them in `skipped`, so an empty answer always says whether it is a complete one. A row says what to change and not how it was reached, so `--verbose` explains each printed row — the version the file wrote, how the name was answered, whether the date is that cycle's own, and the product's endoflife.date page — and `--json` carries the same as the `version`, `matched`, `dated` and `link` fields of each entry; `dated` is `predates` where the declared version is older than every cycle endoflife.date tracks, the date then being the day the oldest one ended rather than a day published for what was declared. Every row is about a version a file declares rather than about what is running: a `go 1.16` in a go.mod is the oldest Go that module promises to work with, and the build may fetch a newer toolchain.\n"

type cliArgs struct {
	showHelp         bool
	showVersion      bool
	showInstructions bool
	dir              string
	within           time.Duration
	withinSet        bool
	withinText       string
	asJSON           bool
	verbose          bool
}

func parseArgs(argv []string) (cliArgs, error) {
	a := cliArgs{dir: "."}
	haveDir := false
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			// Everything after "--" is the directory, even one that starts
			// with "-".
			for _, arg := range argv[i+1:] {
				if haveDir {
					return cliArgs{}, fmt.Errorf("one directory per run, got %q and %q; loop in the shell to cover several", a.dir, arg)
				}
				a.dir, haveDir = arg, true
			}
			break
		}
		switch arg {
		case "-h", "--help":
			a.showHelp = true
		case "-v", "--version":
			a.showVersion = true
		case "--instructions":
			a.showInstructions = true
		case "--json":
			a.asJSON = true
		case "--verbose":
			a.verbose = true
		case "--within":
			if i+1 >= len(argv) {
				return cliArgs{}, fmt.Errorf("--within needs a value")
			}
			i++
			d, err := period.Parse(argv[i])
			if err != nil {
				return cliArgs{}, fmt.Errorf("--within: %w", err)
			}
			a.within, a.withinSet, a.withinText = d, true, argv[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return cliArgs{}, fmt.Errorf("unknown flag %q; run `eolwhen --help` for the options", arg)
			}
			if haveDir {
				return cliArgs{}, fmt.Errorf("one directory per run, got %q and %q; loop in the shell to cover several", a.dir, arg)
			}
			a.dir, haveDir = arg, true
		}
	}
	return a, nil
}

// resolveVersion picks the most authoritative version string available.
//
// Priority:
//  1. injected (set via `-ldflags '-X main.version=...'` during a GoReleaser
//     build) when it differs from devVersion.
//  2. info.Main.Version when present and not "(devel)" or "" — this is what
//     `go install module@vX.Y.Z` records, even though ldflags don't apply.
//  3. injected (devVersion) as the final fallback.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != devVersion {
		return injected
	}
	if info != nil {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return injected
}

// exitCode turns the answer into the code. Anything expired outranks
// anything merely coming, because that is the half that needs work now.
func exitCode(fs []timeline.Finding, now time.Time) int {
	past, future := timeline.Counts(fs, now)
	switch {
	case past > 0:
		return exitPast
	case future > 0:
		return exitFuture
	default:
		return exitNone
	}
}

// notes lists what the answer still owes the caller in words: the lines that
// could not be read, the files nothing could be read from, why an empty
// table is empty, and what a window hid. They go to stderr so stdout carries
// rows alone.
func notes(dir string, r resolve.Result, shown []timeline.Finding, sk []decl.Skipped, a cliArgs) []string {
	var out []string
	for _, u := range collapse(r.Unreadable) {
		out = append(out, complaint(u))
	}
	// A cycle upstream calls over is out of support whether or not the
	// reader asked for detail, which is the question they ran the tool
	// with, so it is said like a line that could not be read rather than
	// kept back for --verbose.
	for _, u := range collapse(ended(r.Ended)) {
		out = append(out, complaint(u))
	}
	if a.verbose {
		// A line that follows the newest release is folded the same way: a
		// repository saying ubuntu-latest in nine workflows is saying one
		// thing, whether or not the reader asked to hear it.
		for _, u := range collapse(r.Moving) {
			out = append(out, complaint(u))
		}
		for _, u := range r.Undated {
			out = append(out, fmt.Sprintf("%s: %s %s has no end-of-life date yet", u.Source, u.Product, u.Cycle))
		}
		for _, d := range r.Untracked {
			out = append(out, fmt.Sprintf("%s: %s is not tracked by endoflife.date", d.Source, d.What()))
		}
		out = append(out, unreadNotes(sk, true)...)
		// Last, so that the rows a reader is about to see are explained
		// next to them rather than above everything else the run set
		// aside. Only the rows that were printed: a window hides what it
		// hides, and explaining a row nobody can see is noise.
		for _, f := range shown {
			out = append(out, evidence(f))
		}
	}
	// An empty table is worth one line saying why, and the reader wants that
	// line to answer the question they ran the tool with. Declarations that
	// have no date to place mean there is nothing to do; a line that could
	// not be read means the answer is short of what the directory declares,
	// and the complaints above say which lines to go and look at.
	//
	// Declarations were read on every path here, because run returns earlier
	// when there are none.
	switch {
	case len(r.Findings) > 0:
		if len(shown) < len(r.Findings) {
			out = append(out, hidden(len(r.Findings)-len(shown), a.withinText))
		}
	case len(r.Unreadable) > 0:
		out = append(out, "nothing in "+dir+" could be placed on the timeline")
	case len(r.Ended) > 0:
		out = append(out, "nothing declared in "+dir+" has a date to put on the timeline")
	case len(r.Undated) > 0:
		out = append(out, "nothing declared in "+dir+" has an end-of-life date yet")
	case len(r.Moving) > 0:
		out = append(out, "everything declared in "+dir+" follows the newest release, so there is no date to place")
	default:
		out = append(out, "nothing declared in "+dir+" is tracked by endoflife.date")
	}
	// A file that was read nothing from is the one thing an empty answer
	// cannot account for on its own: the directory may declare plenty this
	// run never saw, and the line above would say none of it. Only when
	// there is no row at all, because a run with rows has its answer and
	// the default output is where that answer has to stay short. --verbose
	// has already named each file above.
	if len(r.Findings) == 0 && !a.verbose {
		out = append(out, unreadNotes(sk, false)...)
	}
	return out
}

// unreadNotes words the files that were recognized and read nothing from:
// each of them when detail was asked for, and otherwise the count alone.
//
// The count is what says an answer may be short of what the directory
// declares, which is a different thing from the answer being empty. Which
// files they were is detail, and detail is what --verbose is.
func unreadNotes(sk []decl.Skipped, verbose bool) []string {
	if len(sk) == 0 {
		return nil
	}
	if !verbose {
		if len(sk) == 1 {
			return []string{"1 file was recognized and read nothing from; --verbose names it"}
		}
		return []string{fmt.Sprintf("%d files were recognized and read nothing from; --verbose names them", len(sk))}
	}
	out := make([]string, 0, len(sk))
	for _, s := range sk {
		out = append(out, fmt.Sprintf("%s: %s, so nothing in it was read", s.File, s.Reason))
	}
	return out
}

// hidden words what a window kept out. It counts declarations and not rows,
// because a row is a release cycle and the same cycle may be declared in
// several places; what was held back is those places.
func hidden(n int, within string) string {
	if n == 1 {
		return fmt.Sprintf("1 more declaration expires further out than %s; drop --within to see it", within)
	}
	return fmt.Sprintf("%d more declarations expire further out than %s; drop --within to see them", n, within)
}

// matchedBy words how a declaration's name reached its product. The set is
// the one resolve fills, and a product reached through a registry is worth
// saying out loud: a Gemfile naming rails is Ruby on Rails because upstream
// publishes pkg:gem/rails for it, which is a different kind of answer from
// a file that names python.
var matchedBy = map[string]string{
	timeline.ByName:     "matched by name",
	timeline.ByAlias:    "matched by an alias endoflife.date lists",
	timeline.ByPackage:  "matched by the package name upstream publishes",
	timeline.ByImage:    "matched by the image name upstream publishes",
	timeline.ByCodename: "matched by the codename of the release",
}

// evidence words how one row was arrived at: what the file wrote, the cycle
// it reached, how its name was answered, and where the day came from. A row
// is a claim about somebody else's software, worked out through a name
// table, a purl, a codename or an ordering of cycles, and this is what lets
// a reader take the claim apart instead of taking it on trust. The page at
// the end is where to check it.
func evidence(f timeline.Finding) string {
	line := fmt.Sprintf("%s: %s is %s, %s; %s",
		f.Source, f.Version, f.What(), matchedBy[f.Matched], dating(f))
	if f.Page != "" {
		line += " — " + f.Page
	}
	return line
}

// dating words where a row's date came from. A cycle endoflife.date has
// dated answers for itself. A version below every cycle it tracks has no
// date of its own, and the day on the row is the oldest tracked cycle's,
// which support for anything older had already run out by — an upper bound
// worked out from an ordering, and not a day anyone published for the
// version that was declared.
func dating(f timeline.Finding) string {
	if f.Dated == timeline.DatedByPredating {
		return fmt.Sprintf("no cycle covers it, so the date is the day %s ended, which support for anything older had run out by",
			timeline.OldestCycle(f.Cycle))
	}
	return "the date is that cycle's own"
}

// complaint words one line that could not be placed, with the count of the
// further places that said exactly the same thing. A file nothing could be
// read from has no text to quote, and the path is the whole of what there is
// to say about it.
func complaint(u repeated) string {
	line := fmt.Sprintf("%s: %s", u.Source, u.Reason)
	if what := u.What(); what != "" {
		line = fmt.Sprintf("%s: %s %s", u.Source, what, u.Reason)
	}
	if u.more > 0 {
		line += fmt.Sprintf(" (and %d more)", u.more)
	}
	return line
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	a, err := parseArgs(argv)
	if err != nil {
		fmt.Fprintln(stderr, "eolwhen:", err)
		return exitUsage
	}
	switch {
	case a.showHelp:
		fmt.Fprint(stdout, helpText)
		return exitNone
	case a.showVersion:
		info, _ := debug.ReadBuildInfo()
		fmt.Fprintln(stdout, resolveVersion(version, info))
		return exitNone
	case a.showInstructions:
		fmt.Fprint(stdout, instructionsText)
		return exitNone
	}

	info, err := os.Stat(a.dir)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "eolwhen: %s does not exist; give a directory to read\n", a.dir)
		return exitUsage
	}
	if err != nil {
		fmt.Fprintf(stderr, "eolwhen: %v\n", err)
		return exitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "eolwhen: %s is not a directory; eolwhen reads a directory, not a file\n", a.dir)
		return exitUsage
	}

	ds, us, sk, err := scan.Dir(a.dir)
	if err != nil {
		fmt.Fprintln(stderr, "eolwhen:", err)
		return exitUsage
	}
	if len(ds) == 0 && len(us) == 0 {
		// Nothing was declared, which is an answer of its own. The files
		// that were read nothing from are what says whether it is the whole
		// of one, so they are said here as they are said anywhere else.
		fmt.Fprintf(stderr, "eolwhen: no version declarations in %s\n", a.dir)
		for _, n := range unreadNotes(sk, a.verbose) {
			fmt.Fprintln(stderr, "eolwhen:", n)
		}
		if a.asJSON {
			fmt.Fprint(stdout, timeline.JSON(timeline.Report{Directory: a.dir, Skipped: sk}, time.Now()))
		}
		return exitNone
	}
	// The catalog is read only when the answer needs it. A directory that
	// declares nothing, or one whose every line was unreadable before any
	// software was named — runs-on: ubuntu-latest in every workflow and not
	// much else — has its answer already, and asking would only make it
	// depend on the network.
	var c *catalog.Catalog
	if needsCatalog(ds, us) {
		c, err = catalog.Fetch()
		if err != nil {
			fmt.Fprintln(stderr, "eolwhen:", err)
			return exitCatalog
		}
	}
	return report(a, c, ds, us, sk, time.Now(), stdout, stderr)
}

// needsCatalog reports whether anything read has to be looked up: a
// declaration, or an unreadable line whose software is named and might turn
// out to be nothing endoflife.date tracks.
func needsCatalog(ds []decl.Decl, us []decl.Unreadable) bool {
	if len(ds) > 0 {
		return true
	}
	for _, u := range us {
		if u.Product != "" {
			return true
		}
	}
	return false
}

// report turns what was read into the answer and writes it out. Everything
// that decides what the run says lives here, with the catalog handed in, so
// the whole path from a declaration to a row and an exit code is exercised
// without a request. The catalog may be nil when needsCatalog said nothing
// has to be looked up.
func report(a cliArgs, c *catalog.Catalog, ds []decl.Decl, us []decl.Unreadable, sk []decl.Skipped, now time.Time, stdout, stderr io.Writer) int {
	r := resolve.All(c, ds, us)
	timeline.Sort(r.Findings)
	shown := r.Findings
	if a.withinSet {
		shown = timeline.Within(r.Findings, now, a.within)
	}

	if a.asJSON {
		doc := timeline.Report{
			Directory:  a.dir,
			Findings:   shown,
			Unreadable: r.Unreadable,
			Hidden:     len(r.Findings) - len(shown),
			// Said on stderr without being asked for, so carried here
			// without being asked for: a caller reading the document is
			// owed everything one reading the words would have been told.
			Ended:   r.Ended,
			Skipped: sk,
		}
		if a.verbose {
			doc.Moving, doc.Untracked, doc.Undated = r.Moving, r.Untracked, r.Undated
		}
		fmt.Fprint(stdout, timeline.JSON(doc, now))
		return exitCode(shown, now)
	}
	for _, n := range notes(a.dir, r, shown, sk, a) {
		fmt.Fprintln(stderr, "eolwhen:", n)
	}
	fmt.Fprint(stdout, timeline.Table(shown, now))
	return exitCode(shown, now)
}

// repeated is one unreadable line together with how many further places said
// exactly the same thing.
type repeated struct {
	decl.Unreadable
	more int
}

// ended words a cycle upstream calls over as the complaint it is. It has no
// date and so no row, which is what a line that could not be read has in
// common with it, and saying both the same way is what lets one folding rule
// serve them both.
func ended(us []timeline.Undated) []decl.Unreadable {
	out := make([]decl.Unreadable, 0, len(us))
	for _, u := range us {
		out = append(out, decl.Unreadable{
			Source:  u.Source,
			Product: u.Product,
			Text:    u.Cycle,
			Reason:  "is out of support, with no date published",
		})
	}
	return out
}

// collapse folds identical complaints into one line each, keeping the place
// they were first seen. Nearly every workflow in a healthy repository says
// runs-on: ubuntu-latest, so a project with nine jobs would otherwise spend
// nine lines saying one thing; the reader wants to know it once, and grep
// finds the rest.
func collapse(us []decl.Unreadable) []repeated {
	var out []repeated
	at := map[string]int{}
	for _, u := range us {
		k := u.Text + "\x00" + u.Reason
		if i, seen := at[k]; seen {
			out[i].more++
			continue
		}
		at[k] = len(out)
		out = append(out, repeated{Unreadable: u})
	}
	return out
}
