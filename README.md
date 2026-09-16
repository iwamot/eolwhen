# eolwhen

[![release](https://img.shields.io/github/v/release/iwamot/eolwhen)](https://github.com/iwamot/eolwhen/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/iwamot/eolwhen)](https://pkg.go.dev/github.com/iwamot/eolwhen)

Say which of a directory's declared versions are out of support.

```
$ eolwhen
eolwhen: Dockerfile:5: ghcr.io/acme/base:1.2 is not a Docker official image, so its contents are not known here
-2450d  2020-01-01  python 2.7         .python-version:1
-1964d  2021-05-01  alpine-linux 3.10  docker/Dockerfile.ci:2
-1177d  2023-06-27  python 3.7         Dockerfile:1
 -808d  2024-06-30  debian 10          Dockerfile:3
 +258d  2027-06-01  ubuntu 22.04       Dockerfile:4
```

One directory in, one timeline out. Each row is the number of days until support ends — 0 on the day it ends, negative after — the date, the software and its release cycle, and the file that declared it. Dates come from [endoflife.date](https://endoflife.date).

## Why

A repository nobody maintains still says what it runs on. `.python-version` says 2.7, `go.mod` says 1.16, `.ruby-version` says 2.6. Those lines are still true, and every one of them names something that stopped getting security fixes years ago.

Nothing tells you. Renovate and Dependabot would, but a neglected repository is exactly the one where they were never turned on, or where their pull requests have been piling up unread since 2022. There is no alert to miss, because there was never an alert.

`eolwhen` answers the question from the outside, in one command, with no setup and no credentials: point it at a checkout and it reads what is written there.

It reads declarations, which is the gap. Scanners that resolve dependency manifests — [xeol](https://github.com/xeol-io/xeol), [uzomuzo](https://github.com/future-architect/uzomuzo) — answer a neighbouring question well, and they are worth running alongside this. What they do not read is the line that says which Ruby the thing runs on.

## Setup

```bash
brew install iwamot/tap/eolwhen
```

Or with Go:

```bash
go install github.com/iwamot/eolwhen@latest
```

Or download a prebuilt binary from the [Releases page](https://github.com/iwamot/eolwhen/releases).

## What it reads

| File | Declares |
|---|---|
| `.python-version` | Python |
| `.nvmrc`, `.node-version` | Node.js |
| `.ruby-version` | Ruby |
| `go.mod` (the `go` directive) | Go |
| `mise.toml`, `.mise.toml`, `.tool-versions` | each tool listed under a bare name |
| `Dockerfile`, `Dockerfile.*` | whatever each `FROM` names |
| `compose*.yml`, `docker-compose*.yml` (and `.yaml`) | whatever each service's `image:` names, unless the service has a `build:` and the image is what it builds |
| `.github/workflows/*.yml` (and `.yaml`) | the `runs-on:` runner images, and the versions given to `actions/setup-node`, `-python`, `-go`, `-dotnet`, `ruby/setup-ruby` and `shivammathur/setup-php` |

A tool list is the one place where finding the file does not promise there is anything to look up. `.nvmrc` is Node.js and Node.js has an end-of-life policy; a tool list holds whatever the project uses, and `biome`, `hugo` and `jq` have none at all. Those are set aside without a word, and `--verbose` accounts for them. A key carrying a backend — `aqua:`, `go:`, `npm:` — names a package in a registry anyone can publish to, so it is not read.

The directory is searched to the bottom, so a monorepo's `packages/web/.nvmrc` and a `docker/Dockerfile` are both found. Directories holding somebody else's code — `node_modules`, `vendor`, `third_party`, `.venv`, `.git` and the like — are skipped, because a `.nvmrc` inside a dependency is its author's declaration and not this directory's.

### How a line becomes a row

The place a version is written is what decides the software, so nothing is guessed from the text. A file name can only mean one thing: `.nvmrc` is Node.js. An official image name can only mean one thing too, because that namespace is a short curated list — `library/postgres` is PostgreSQL.

That is also why `FROM ghcr.io/acme/python:3.7` is reported rather than read: outside the official library anyone can name an image anything, so its contents are not knowable from the line. `FROM python@sha256:...` with no tag is reported for the same reason — a digest does not say which version it is.

A Compose file is read service by service. What sits under `services:` is a service and nothing else is, because a file keeps its templates in extension fields — `x-defaults` and the like — and a service pulls one in with a merge key. Merges are applied the way YAML applies them, so a service that sets its own image over an inherited one declares the image it set.

A version written once and referred to elsewhere is still read: a workflow that keeps it in `env:` under an anchor and writes `python-version: *python` in the step declares it at the step, which is the line to go and change. An anchor may be defined more than once, and an alias means the definition above it, as YAML says; one added further down does not reach back and change what an earlier reference meant.

Versions are read as they are written, quoted or not. A workflow that says `python-version: 3.10` means the 3.10 line, and reading it as a number would make it 3.1, which is a different Python that went out of support in 2012.

A runner label is the same idea from the other end: endoflife.date's runner-image product names its release cycles `macos-13` and `ubuntu-22.04`, which is exactly what `runs-on:` says, so no translation is needed at all. A label is only read when it is spelled the way one of GitHub's images is: a family, then a version or the word latest. A custom pool is free to start with the same word — `ubuntu-x64-small` and `ubuntu-slim` are real ones — so `self-hosted` and its kind are left alone, because they name a machine and not a version. GitHub writes `ubuntu-24.04-arm` where the catalog writes `ubuntu-24.04-arm64`, so a row spells it the catalog's way.

A tag that names only a major line — `redis:7`, or `python-version: 3` — is not a version but a rule for following one: the image moves to the next 7.x the day it exists. It gets no row, and the reason says so, because a date that changes on its own is not a date this tool can put on a timeline.

A version reaches its release cycle by dot-separated segments, not by string prefix: `3.10.2` reaches Python's 3.10 cycle and never its 3.1 cycle, which expired in 2012. A tag that names a codename instead of a number is resolved through the codename endoflife.date publishes, so `FROM debian:buster` reads as Debian 10 and `FROM ubuntu:jammy` as Ubuntu 22.04.

## Using it

Both halves of the timeline matter, so both are printed. What has expired needs work now; what is coming needs a date in the calendar.

```
$ eolwhen ../some-fresh-checkout
 +592d  2028-04-30  nodejs 24    .nvmrc:1
+1141d  2029-10-31  python 3.13  .python-version:1
```

`--within` narrows what is ahead. What has already expired is always shown, and the count that was hidden is said on stderr:

```
$ eolwhen --within 2w ../some-fresh-checkout
eolwhen: 2 more expire further out than 2w; drop --within to see them
```

Only rows go to stdout. Anything the answer needs a sentence for — a line that names no version, a directory that declares nothing, a window that hid something — is an `eolwhen:` line on stderr, so `awk` over stdout reads rows and nothing else.

A directory that declares nothing says so and exits 0. It is not an error, and endoflife.date is not read at all on that path:

```
$ eolwhen /tmp/empty
eolwhen: no version declarations in /tmp/empty
```

The exit code is the answer, so a check can be one line. This one fails the
job when something has already expired, passes when only future dates were
found, and fails when the run could not answer at all — a usage error or an
unreachable endoflife.date:

```bash
eolwhen || [ $? = 2 ]
```

The same answer as JSON. Everything the table needs said in words on stderr is a field here instead: `hidden` is how many findings a `--within` window kept out, and `untracked` is filled when `--verbose` asks for the declarations naming software endoflife.date has no policy for.

```
$ eolwhen --json
{
  "directory": ".",
  "findings": [
    {
      "product": "python",
      "cycle": "2.7",
      "eol": "2020-01-01",
      "days": -2450,
      "past": true,
      "source": ".python-version:1"
    }
  ],
  "unreadable": [
    {
      "source": ".nvmrc:1",
      "product": "nodejs",
      "text": "lts/hydrogen",
      "reason": "names a moving target, not a version"
    }
  ],
  "hidden": 0,
  "untracked": []
}
```

To cover several directories, loop over them in the shell. `eolwhen` reads one.

## For coding agents

`eolwhen --instructions` prints the paragraph to drop into `CLAUDE.md`, `AGENTS.md`, or whichever file your agent reads:

```markdown
To find out whether the runtimes and base images a directory declares are still supported, use `eolwhen` instead of reading version files and checking dates by hand: `eolwhen` for the current directory, or `eolwhen DIR` for another one. It reads the version declarations in that one directory, matches them against endoflife.date, and prints one row per declaration with the days until support ends, 0 on the day it ends and negative after. Add `--within 90d` to hide what expires further out than that; what has already expired is always shown, and the exit code then answers only for the rows that were printed. Exit 1 means something is already out of support and exit 2 means something will be, so both are answers and neither is a failure; exit 3 is a usage error and exit 4 means endoflife.date could not be read, which is worth one retry. Only rows go to stdout, so awk can read the columns; lines that name no version, and anything else the answer needs said in words, are `eolwhen:` lines on stderr.
```

## Reference

```
$ eolwhen --help
eolwhen — say which of a directory's declared versions are out of support.

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
  Only rows go to stdout; everything else is an `eolwhen:` line on stderr.
  The exit code answers for the rows that were printed, so --within narrows
  what it covers as well as what is shown.

Exit codes:
  0  nothing to report, including a directory with no declarations
  1  something is already out of support
  2  nothing has expired, but something will
  3  usage error: fix the flags or the directory
  4  endoflife.date could not be read: check the network, then retry
```

- A release cycle's end-of-life date is the first day without support, so something due today reads as `0d` and counts as expired. The sign says which side of today a date falls on, and the day it lands on takes none. Days are counted between calendar days, and today is today where you are: endoflife.date publishes a date rather than a moment, so counting from anyone else's calendar would put a date and a number that disagree on the same row.
- Rows are ordered by the date itself, oldest first, so the timeline runs in one direction and the most overdue reads at the top.
- `.python-version` may name several versions, as pyenv allows; each gets its own row.
- A cycle with no announced end-of-life date is reported on stderr rather than printed, because there is no day to place it on. Current releases are often in this state.
- Software endoflife.date does not track is passed over without a word, whatever version it was given. Plenty of tools publish no end-of-life policy at all, and there was never a date to find for them; a `jq = "latest"` is a line about jq before it is a line about latest.
- The runner-image catalog holds the images GitHub offers and the ones it retired most recently, so a label retired longer ago — `ubuntu-18.04`, `windows-2019` — is reported as covered by no release cycle. A workflow still asking for one has already stopped running.
- A file the walk offers but cannot open is reported by path and the rest of the tree is still read. One unreadable corner is not a reason to refuse an answer, though the directory named on the command line has to be readable, being the question itself.
- A directory reached through a symlink is that directory. Links met further down the tree are left alone, which is what keeps a loop from being possible.
- The exit code answers for the rows that were printed, so `--within 90d` turns the run into a check for "is anything expiring in the next 90 days", and what the window hid is not counted.
- The same complaint from several places is said once, with a count. Nearly every workflow in a healthy repository says `runs-on: ubuntu-latest`, and knowing that once is enough; `grep` finds the rest.
- A file that does not parse — a workflow or Compose file that is not YAML yet, a `mise.toml` that is not TOML yet — is skipped in silence. The tool that owns it reports that better than this one can, and a file mid-edit is not a declaration that could not be read.
- The whole catalog — every product endoflife.date knows, with every cycle — is fetched from endoflife.date when a run needs it. There is no prebuilt database in the binary and nothing to update.
- The endoflife.date API is beta and says breaking changes can happen, so only the fields this tool needs are read and the rest is ignored.
- While the version is 0.x, the exit codes and the shape of the output can still change between releases; from 1.0 they only gain cases.

## Out of scope

- **Package and library versions.** Reaching them means matching a package name against a product name, and in a registry anyone can publish to, `mongodb` is as likely to be the driver as the server. That layer belongs to [xeol](https://github.com/xeol-io/xeol) and [uzomuzo](https://github.com/future-architect/uzomuzo). It can be reconsidered for the pairs endoflife.date names explicitly in its purl identifiers, where there is nothing to guess.
- **Vulnerabilities.** They carry no date, so they do not belong on a timeline, and `osv-scanner`, `trivy`, and `grype` already read a directory for them. The two answers meet in one place worth saying out loud: once a runtime is past its end of life, the vulnerabilities found from then on are never fixed.
- **Deprecated GitHub Actions.** `actions/checkout@v2` has no machine-readable source to track, and unlike an expired base image it does not fail quietly — the workflow says so the next time it runs.
- **Terraform and cloud service versions.** The most valuable layer by far, but the one where the value is usually `var.eks_version` rather than a literal, which only Terraform itself can resolve. Later.
- **Resolving `${{ matrix.* }}`.** The biggest single source of lines that cannot be read, and the place a project often declares which versions it supports. It needs the job's `strategy.matrix` read alongside the step. Later.
- **`requires-python` in `pyproject.toml`.** It states a range — `>=3.9` — which says what the project accepts rather than what it runs on, and a range has no single cycle to date. The version actually in use is in `.python-version`, the Dockerfile, or the workflow.
- **Scanning many directories.** One run reads one directory; a shell loop covers the rest.

## Credits

End-of-life dates come from [endoflife.date](https://endoflife.date), which is MIT licensed.

## License

MIT
