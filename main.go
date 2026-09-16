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

DIR is the directory to read, and defaults to the current one. It does not
have to be a git repository. One directory is read per run; to cover several,
loop over them in the shell.

Declarations are read from the runtime version files (.python-version,
.nvmrc, .node-version, .ruby-version, the go directive in go.mod), the tool
lists (mise.toml, .tool-versions), the FROM lines of any Dockerfile, the
image: of any Compose service, and the runs-on labels and setup-* versions
in .github/workflows, then matched against endoflife.date. DIR is searched
to the bottom, skipping directories that hold somebody else's code:
node_modules, vendor, .venv and the like.

Every expired declaration is printed, oldest first, together with the ones
still ahead. A line that names no version — lts/hydrogen, ubuntu-latest, a
digest-pinned FROM, an image outside the Docker official library — is
reported on stderr rather than guessed at, once per distinct complaint.

Options:
  --within DUR    only show what expires within DUR (1d, 36h, 2w); what has
                  already expired is always shown
  --json          print JSON instead of the table, with the unreadable lines
                  in the document
  --verbose       also say which declarations name software endoflife.date
                  does not track, which is most of what a tool list holds
  -h, --help      show this help
  -v, --version   show the version
  --instructions  print the paragraph for an agent's instruction file

Output:
  Each row is the days until support ends — 0 on the day it ends, negative
  after — then the date, the software and its release cycle, and the file it
  was declared in.
  Only rows go to stdout; everything else is an ` + "`eolwhen:`" + ` line on stderr.
  The exit code answers for the rows that were printed, so --within narrows
  what it covers as well as what is shown.

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
const instructionsText = "To find out whether the runtimes and base images a directory declares are still supported, use `eolwhen` instead of reading version files and checking dates by hand: `eolwhen` for the current directory, or `eolwhen DIR` for another one. It reads the version declarations in that one directory, matches them against endoflife.date, and prints one row per declaration with the days until support ends, 0 on the day it ends and negative after. Add `--within 90d` to hide what expires further out than that; what has already expired is always shown, and the exit code then answers only for the rows that were printed. Exit 1 means something is already out of support and exit 2 means something will be, so both are answers and neither is a failure; exit 3 is a usage error and exit 4 means endoflife.date could not be read, which is worth one retry. Only rows go to stdout, so awk can read the columns; lines that name no version, and anything else the answer needs said in words, are `eolwhen:` lines on stderr.\n"

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
// were read but could not be placed on the timeline, why an empty table is
// empty, and what a window hid. They go to stderr so stdout carries rows
// alone.
func notes(dir string, r resolve.Result, shown []timeline.Finding, a cliArgs) []string {
	var out []string
	for _, u := range collapse(r.Unreadable) {
		// A file nothing could be read from has no text to quote, and the
		// path is the whole of what there is to say about it.
		line := fmt.Sprintf("%s: %s", u.Source, u.Reason)
		if what := u.What(); what != "" {
			line = fmt.Sprintf("%s: %s %s", u.Source, what, u.Reason)
		}
		if u.more > 0 {
			line += fmt.Sprintf(" (and %d more)", u.more)
		}
		out = append(out, line)
	}
	if a.verbose {
		for _, d := range r.Untracked {
			out = append(out, fmt.Sprintf("%s: %s %s is not tracked by endoflife.date", d.Source, d.Product, d.Version))
		}
	}
	switch {
	// Declarations were read — run returns earlier when there are none — so
	// an empty timeline with nothing to say about it means every one of them
	// named software endoflife.date does not track.
	case len(r.Findings) == 0 && len(r.Unreadable) == 0:
		out = append(out, "nothing endoflife.date tracks was declared in "+dir)
	case len(r.Findings) == 0:
		out = append(out, "nothing could be placed on the timeline")
	case len(shown) < len(r.Findings):
		out = append(out, fmt.Sprintf("%d more expire further out than %s; drop --within to see them",
			len(r.Findings)-len(shown), a.withinText))
	}
	return out
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

	ds, us, err := scan.Dir(a.dir)
	if err != nil {
		fmt.Fprintln(stderr, "eolwhen:", err)
		return exitUsage
	}
	if len(ds) == 0 && len(us) == 0 {
		fmt.Fprintf(stderr, "eolwhen: no version declarations in %s\n", a.dir)
		if a.asJSON {
			fmt.Fprint(stdout, timeline.JSON(timeline.Report{Directory: a.dir}, time.Now()))
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
	return report(a, c, ds, us, time.Now(), stdout, stderr)
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
func report(a cliArgs, c *catalog.Catalog, ds []decl.Decl, us []decl.Unreadable, now time.Time, stdout, stderr io.Writer) int {
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
		}
		if a.verbose {
			doc.Untracked = r.Untracked
		}
		fmt.Fprint(stdout, timeline.JSON(doc, now))
		return exitCode(shown, now)
	}
	for _, n := range notes(a.dir, r, shown, a) {
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
