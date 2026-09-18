# eolwhen

[![release](https://img.shields.io/github/v/release/iwamot/eolwhen)](https://github.com/iwamot/eolwhen/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/iwamot/eolwhen)](https://pkg.go.dev/github.com/iwamot/eolwhen)

Say which of a directory's declared versions are out of support.

```
$ eolwhen
eolwhen: Dockerfile:12: python@sha256:3f1a2b is pinned by digest, which does not say which version it is
-2450d  2020-01-01  python 2.7         .python-version:1
-1964d  2021-05-01  alpine-linux 3.10  docker/Dockerfile.ci:2
-1177d  2023-06-27  python 3.7         Dockerfile:1,9
 -808d  2024-06-30  debian 10          Dockerfile:1,9
 +258d  2027-06-01  ubuntu 22.04       Dockerfile:4
```

One directory in, one timeline out. Each row is the number of days until support ends — 0 on the day it ends, negative after — the date, the software and its release cycle, and every place that declared it. Dates come from [endoflife.date](https://endoflife.date).

## Why

A repository nobody maintains still says what it runs on. `.python-version` says 2.7, `go.mod` says 1.16, `.ruby-version` says 2.6. Those lines are still true, and every one of them names something that stopped getting security fixes years ago.

Nothing tells you. Renovate and Dependabot would, but a neglected repository is exactly the one where they were never turned on, or where their pull requests have been piling up unread since 2022. There is no alert to miss, because there was never an alert.

`eolwhen` answers the question from the outside, in one command, with no setup and no credentials: point it at a checkout and it reads what is written there.

It reads declarations, which is the gap. Scanners that resolve dependency manifests — [xeol](https://github.com/xeol-io/xeol), [uzomuzo](https://github.com/future-architect/uzomuzo) — answer a neighbouring question well, and they are worth running alongside this. What they do not read is the line that says which Ruby the thing runs on. A manifest is read here for the same kind of line and no other: the framework the application sits on, not the libraries around it.

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
| `Dockerfile`, `Dockerfile.*` | whatever each `FROM` names — an official image, or any image endoflife.date publishes a purl for — and the distribution its tag was built on |
| `compose*.yml`, `docker-compose*.yml` (and `.yaml`) | the same, for each service's `image:`, unless the service has a `build:` and the image is what it builds |
| `Gemfile`, `gems.rb` | each `gem` whose name endoflife.date publishes as a gem — the framework the application sits on, not the libraries around it |
| `composer.json` | each package in `require` and `require-dev` whose name endoflife.date publishes, and the `php` the project runs on |
| `*.csproj`, `*.fsproj`, `*.vbproj` | the target framework the project runs on — Microsoft .NET, or the .NET Framework |
| `.github/workflows/*.yml` (and `.yaml`) | the `runs-on:` runner images, and the versions given to `actions/setup-node`, `-python`, `-go`, `-dotnet`, `ruby/setup-ruby` and `shivammathur/setup-php`, including the ones a job's `strategy.matrix` lists |

A tool list is the one place where finding the file does not promise there is anything to look up. `.nvmrc` is Node.js and Node.js has an end-of-life policy; a tool list holds whatever the project uses, and `biome`, `hugo` and `jq` have none at all. Those are set aside without a word, and `--verbose` accounts for them. A key carrying a backend — `aqua:`, `go:`, `npm:` — names a package rather than a tool, and a package is read where the file it sits in fixes the registry, as a `Gemfile` and a `composer.json` do. A backend written as a prefix on a key is not read that way yet.

The directory is searched to the bottom, so a monorepo's `packages/web/.nvmrc` and a `docker/Dockerfile` are both found. Directories holding somebody else's code — `node_modules`, `vendor`, `third_party`, `.venv`, `.git` and the like — are skipped, because a `.nvmrc` inside a dependency is its author's declaration and not this directory's.

### How a line reaches the timeline

The place a version is written is what decides the software, so nothing is guessed from the text. A file name can only mean one thing: `.nvmrc` is Node.js. An official image name can only mean one thing too, because that namespace is a short curated list — `library/postgres` is PostgreSQL.

Outside that library anyone can name an image anything, so the name alone settles nothing — but endoflife.date publishes the Docker Hub repository each of its products ships under, as a purl. `opensearchproject/opensearch` is OpenSearch because upstream says so, and `FROM ghcr.io/acme/python:3.7` is still not Python, because nobody said so. An image nobody published a purl for is a declaration of software the catalog does not track, like `jq` in a tool list: there was never a date to find, so it is set aside without a word and `--verbose` accounts for it.

`FROM python@sha256:...` with no tag is reported, because a digest does not say which version it is.

A Compose file is read service by service. What sits under `services:` is a service and nothing else is, because a file keeps its templates in extension fields — `x-defaults` and the like — and a service pulls one in with a merge key. Merges are applied the way YAML applies them, so a service that sets its own image over an inherited one declares the image it set.

A version written once and referred to elsewhere is still read: a workflow that keeps it in `env:` under an anchor and writes `python-version: *python` in the step declares it at the step, which is the line to go and change. An anchor may be defined more than once, and an alias means the definition above it, as YAML says; one added further down does not reach back and change what an earlier reference meant.

Versions are read as they are written, quoted or not. A workflow that says `python-version: 3.10` means the 3.10 line, and reading it as a number would make it 3.1, which is a different Python that went out of support in 2012.

A runner label is the same idea from the other end: endoflife.date's runner-image product names its release cycles `macos-13` and `ubuntu-22.04`, which is exactly what `runs-on:` says, so no translation is needed at all. A label is only read when it is spelled the way one of GitHub's images is: a family, then a version or the word latest. A custom pool is free to start with the same word — `ubuntu-x64-small` and `ubuntu-slim` are real ones — so `self-hosted` and its kind are left alone, because they name a machine and not a version. GitHub writes `ubuntu-24.04-arm` where the catalog writes `ubuntu-24.04-arm64`, so a row spells it the catalog's way.

A tag that names only a major line — `redis:7`, or `python-version: 3` — is not a version but a rule for following one: the image moves to the next 7.x the day it exists. It gets no row, because a date that changes on its own is not a date this tool can put on a timeline. Neither does `:latest`, `ubuntu-latest`, or a tool pinned to `stable`: those lines follow the newest release on purpose, so there is nothing wrong with them and nothing to go and change. They are set aside without a word and `--verbose` accounts for them, because a complaint nobody can act on is what buries the rows that matter.

A version reaches its release cycle by dot-separated segments, not by string prefix: `3.10.2` reaches Python's 3.10 cycle and never its 3.1 cycle, which expired in 2012. A tag that names a codename instead of a number is resolved through the codename endoflife.date publishes, so `FROM debian:buster` reads as Debian 10 and `FROM ubuntu:jammy` as Ubuntu 22.04.

A version older than every cycle endoflife.date tracks gets a row of its own. A `redis:3.2` in a Compose file is the most neglected line in the directory, and reading it as a version nobody has heard of is the one answer that is certainly wrong — the catalog starts at 4.0 because everything below it stopped being a going concern long ago. The row carries the day that oldest cycle ended, which support for anything older had already run out by, and names the cycle `<4.0` so that it says what it knows rather than a day it cannot know:

```
$ eolwhen
-2270d  2020-07-01  redis <4.0  compose.yml:12
```

A cycle reached that way still gets no row when endoflife.date has given it no end date. Nothing is wrong with the line and there is nothing to go and change — support has not been dated yet, which is what a recent release looks like — so it is set aside without a word, like the software the catalog does not track, and `--verbose` accounts for both.

### The other half of an image tag

`FROM python:3.11-bullseye` declares two things. The Python is one, and the Debian 11 it is built on is the other — and the Debian is usually the half that expires first, because a base image outlives the runtime's own support window. So the variant after the dash is read too: a segment spelled `alpine3.19` names Alpine 3.19 outright, and a segment that is one word may be a distribution codename, which names the release and the distribution at once. Which words those are is endoflife.date's answer and not a list kept here, so `bookworm` reads as Debian 12 while `slim`, `fpm` and `jre` are builds of something rather than releases of anything, and say nothing.

A codename two products share is read as neither, since which one a tag meant would be a guess — and a codename is worth reading precisely because it needs none. The same word is what nvm writes in `.nvmrc` as `lts/hydrogen`, which is Node.js 18.

A `FROM` written in terms of a build argument is read as `docker build` with no `--build-arg` resolves it, which is the build the file describes: the argument's default, or nothing at all when it was declared without one. That is what makes `FROM ${REGISTRY}python:3.11-bullseye` under a bare `ARG REGISTRY` read as the official image it falls back to. Only the arguments above the first `FROM` are in scope for one, which is Docker's own rule, and a name no `ARG` declares comes from outside the file, so the line is reported instead.

### What a matrix declares

A workflow that writes `python-version: ${{ matrix.python-version }}` is not hiding the versions it runs on — they are listed a few lines above it, in the same job's `strategy.matrix`, which is where a project says which versions it supports. Each is read there, and reported at the line it was written on, which is the line to go and change:

```
$ eolwhen
-320d  2025-10-31  python 3.9  .github/workflows/ci.yml:8
```

A matrix belongs to its job, so one job's `os:` says nothing about another's. `include:` holds whole combinations and may name a version the list does not, so what it says under a key is read as another value of it; `exclude:` takes combinations away and declares nothing. A matrix built by an expression — `fromJSON` of another job's output — lists nothing to read, and an expression doing more than naming one key is left alone: what it works out to is the workflow's to decide at run time.

### Which packages are a declaration

A manifest is mostly libraries, and a library rarely has an end of life. What it also holds is the framework the application sits on, and a framework publishes a support calendar for the same reason a distribution does — Rails 6.1 stopped getting security fixes on 2024-10-01, and an application still asking for it is in the same position as one still asking for Debian 10.

Which of the two a line names is upstream's answer rather than a guess made here. endoflife.date publishes the package name for the products that have one, so `rails` is Ruby on Rails and `laravel/framework` is Laravel on its word, and every other package is software it does not track and is passed over in silence. That is what keeps `gem "pg"` from reading as PostgreSQL: the database answers to `pg` as an alias, but the gem is the driver, which is different software on a different calendar, and a registry anyone can publish to is not a namespace to read names out of.

A `composer.json` holds one line that is not a package at all. Composer reserves `php` for the version of the language the project runs on, so that key can only mean PHP — not because the name reads that way, but because Composer says so, which is the same kind of answer a `.python-version` gives. Its other reserved names are passed over: `ext-` and `lib-` ask for extensions, and `composer` and `hhvm` for the tooling, none of which is software with a release calendar of its own.

```
$ eolwhen
-1390d  2022-11-28  php 7.4    composer.json:4
-1333d  2023-01-24  laravel 8  composer.json:6
 -717d  2024-10-01  rails 6.1  Gemfile:3
```

A requirement usually pins a range rather than a version, and a range is still an answer when the whole of it sits inside one cycle, because a row is about a cycle. Every version `~> 6.1.0` allows is Rails 6.1 and every version `^8.0` allows is Laravel 8, so those lines are dated. `~> 6` is not: it admits both 6.0 and 6.1, and which one was installed is written in the lockfile, which this does not read. Neither is one that leaves the upper end open, as `>= 6.0` does, nor one that leaves two ranges behind, as the union `^7.4 || ^8.0` does, nor a package given no requirement at all. Those are set aside without a word, and `--verbose` names them.

A version with a letter in it — `7.1.0.rc1` — is passed over too. Where a pre-release falls against a release is the package manager's rule rather than a number's, and getting it wrong would date a line by a cycle it is not in.

### Which .NET a target framework names

A project file says which runtime the project runs on, and nothing is guessed from the text there either: the target framework monikers are a vocabulary .NET defines. The word in front of the digits says which .NET it is. `net6.0` is Microsoft .NET and `net472` is the .NET Framework, which are different products on different calendars, and the dot is what tells them apart — the `.0` in `net5.0` was added so that the two could never be read as each other. A moniker of bare digits is one digit per segment, so `net481` is 4.8.1. `net35` reads as the 3.5 service pack, which is the only 3.5 still installable and the cycle endoflife.date tracks; reading it as a plain 3.5 would reach no cycle at all and date a target that is still supported by the day the 4.0 above it expired.

A platform on the end — the `-windows` of `net8.0-windows` — says which APIs the target adds rather than which version of it is meant, so it is cut away. `<TargetFrameworks>` holds several at once, and each is a declaration of its own, as each line of a `.python-version` is. A project written before the SDK-style project existed spells the same declaration `<TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion>`, which only ever named a .NET Framework.

```
$ eolwhen
-1606d  2022-04-26  dotnetfx 4.5.2  src/Legacy/Legacy.csproj:4
 -675d  2024-11-12  dotnet 6        src/Api/Api.csproj:3
```

A moniker naming something that is not a runtime is looked up under the name it gave, and nothing answers to it: `netstandard2.0` is an API contract rather than a thing that runs, and nobody publishes an end-of-life date for one, so it is set aside like a tool the catalog does not track. A target framework written as `$(DefaultTargetFramework)` names no version of its own — the value is in a `Directory.Build.props` or on the command line, neither of which is read — so there is nothing to place and nothing to go and change. `--verbose` accounts for both.

## Using it

Both halves of the timeline matter, so both are printed. What has expired needs work now; what is coming needs a date in the calendar.

One row is one release cycle, however many lines declared it. What there is to deal with is that Debian 11 stops getting security fixes, and a multi-stage build naming the same base three times is one thing to deal with and not three; the places follow as the list of what to go and change, with a file named once and its lines behind it.

```
$ eolwhen ../some-fresh-checkout
 +592d  2028-04-30  nodejs 24    .nvmrc:1
+1141d  2029-10-31  python 3.13  .python-version:1
```

`--within` narrows what is ahead. What has already expired is always shown, and the count that was hidden is said on stderr:

```
$ eolwhen --within 2w ../some-fresh-checkout
eolwhen: 2 more declarations expire further out than 2w; drop --within to see them
```

Only rows go to stdout. Anything the answer needs a sentence for — a line that could not be read, a directory that declares nothing, a window that hid something — is an `eolwhen:` line on stderr, so `awk` over stdout reads rows and nothing else.

An empty table is one line on stderr, and that line is the answer. A directory can come out empty without anything being wrong, and the line says which one it is: one running current versions, where no cycle it declares has been given an end date yet; one that declares nothing at all, which is answered without reading endoflife.date; one whose every declaration names software endoflife.date has no policy for; and one whose every line follows the newest release. All of them exit 0, because none of them is an error.

```
$ eolwhen ../a-repo-kept-up-to-date
eolwhen: nothing declared in ../a-repo-kept-up-to-date has an end-of-life date yet

$ eolwhen /tmp/empty
eolwhen: no version declarations in /tmp/empty

$ eolwhen ../a-repo-of-tools
eolwhen: nothing declared in ../a-repo-of-tools is tracked by endoflife.date
```

A directory whose every line could not be read comes out empty too, and there the complaints are the answer: they are printed one per distinct complaint, and the closing line says only that nothing in the directory could be placed on the timeline.

`--verbose` names the declarations that had no date to place — software endoflife.date does not track, cycles it has not dated yet, and the lines that follow the newest release. They are quiet by default because a repository doing everything right would otherwise spend a line on every file it keeps up to date, which is noise on the run that has an answer.

The exit code is the answer, so a check can be one line. This one fails the
job when something has already expired, passes when only future dates were
found, and fails when the run could not answer at all — a usage error or an
unreachable endoflife.date:

```bash
eolwhen || [ $? = 2 ]
```

The same answer as JSON, with one entry per declaration rather than one per cycle, and `cycle` reading `<4.0` where the version predates everything endoflife.date tracks: the table folds them for reading, and a caller reading the document wants each place as its own record. Everything the table needs said in words on stderr is a field here instead: `hidden` is how many findings a `--within` window kept out, and `moving`, `untracked` and `undated` are filled when `--verbose` asks for the declarations that had no date to place — lines that follow the newest release, software endoflife.date has no policy for, and cycles it has not dated yet.

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
      "source": "Dockerfile:5",
      "product": "",
      "text": "ghcr.io/acme/base:1.2",
      "reason": "is not a Docker official image, so its contents are not known here"
    }
  ],
  "hidden": 0,
  "moving": [],
  "untracked": [],
  "undated": []
}
```

To cover several directories, loop over them in the shell. `eolwhen` reads one.

## For coding agents

`eolwhen --instructions` prints the paragraph to drop into `CLAUDE.md`, `AGENTS.md`, or whichever file your agent reads:

```markdown
To find out whether the runtimes, base images and frameworks a directory declares are still supported, use `eolwhen` instead of reading version files and checking dates by hand: `eolwhen` for the current directory, or `eolwhen DIR` for another one. It reads the version declarations in that one directory, matches them against endoflife.date, and prints one row per release cycle with the days until support ends, 0 on the day it ends and negative after, followed by every place that cycle was declared — a file is named once with its lines behind it, as `Dockerfile:2,22,34`, and `--json` has one entry per declaration instead. Add `--within 90d` to hide what expires further out than that; what has already expired is always shown, and the exit code then answers only for the rows that were printed. Exit 1 means something is already out of support and exit 2 means something will be, so both are answers and neither is a failure; exit 0 means no row was printed, and the single `eolwhen:` line says why — most often that nothing declared has an end-of-life date yet, which is nothing to do; exit 3 is a usage error and exit 4 means endoflife.date could not be read, which is worth one retry. Only rows go to stdout, so awk can read the first three columns; lines it could not read, and anything else the answer needs said in words, are `eolwhen:` lines on stderr; a line that follows the newest release on purpose, such as `ubuntu-latest`, is not one of them and only `--verbose` names it.
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
image: of any Compose service, the gem lines of any Gemfile, the require of
any composer.json, the target framework of any .csproj, .fsproj or .vbproj,
and the runs-on labels and setup-* versions in .github/workflows, then
matched against endoflife.date. DIR is searched to the bottom, skipping
directories that hold somebody else's code: node_modules, vendor, .venv and
the like.

A package reaches a product only through the package names endoflife.date
publishes, so rails is Ruby on Rails on upstream's word while pg is the
PostgreSQL driver and reaches nothing. A composer.json's php is the one
exception, Composer having reserved that name for the language, so it
declares a runtime the way a .python-version does. A requirement that pins
a range rather than a version still names a cycle when the whole range sits
inside one: ~> 6.1.0 is Rails 6.1 and ^8.0 is Laravel 8. One that does not,
or a package left to the lockfile, has no one version to date and is set
aside.

A target framework moniker names the runtime a project runs on, and the dot
says which .NET it is: net6.0 is Microsoft .NET, while net472 is the .NET
Framework, a different product on a calendar of its own. A moniker naming
something that is not a runtime, such as netstandard2.0, is set aside like
software the catalog does not track. So is one written as an MSBuild
property, the value being in a file this does not read.

An image outside the Docker official library is read through the Docker Hub
repository endoflife.date publishes for each product, so
opensearchproject/opensearch is OpenSearch on upstream's word; a name nobody
published is software the catalog does not track, like a tool in a tool
list. An official image's tag that names the distribution it was built on
declares that too: python:3.11-bullseye is a Python and a Debian 11, and the
Debian is usually the half that expires first. A FROM written in terms of a
build argument is read as a docker build with no --build-arg resolves it,
and a version given as ${{ matrix.* }} is read from the job's own
strategy.matrix, which is where a project says which versions it supports.

Every expired declaration is printed, oldest first, together with the ones
still ahead. A version older than every cycle endoflife.date tracks gets a
row of its own, reading <4.0 and carrying the day that oldest cycle ended,
which support for anything older had run out by. A line this tool could not
read — a digest-pinned FROM, an expression it cannot work out, a version no
release cycle covers — is reported on stderr rather than guessed at, once
per distinct complaint.
A declaration with no date to place is set aside without a word: software
endoflife.date does not track, a cycle it has not dated yet, and a line that
follows the newest release on purpose, such as ubuntu-latest, a :latest tag
or a tool pinned to stable. An empty table is one line on stderr saying
which of these the directory is.

Options:
  --within DUR    only show what expires within DUR (1d, 36h, 2w); what has
                  already expired is always shown
  --json          print JSON instead of the table: one entry per declaration,
                  with the unreadable lines in the document
  --verbose       also say which declarations have no date to place: software
                  endoflife.date does not track, cycles it has not dated yet,
                  and lines that follow the newest release on purpose
  -h, --help      show this help
  -v, --version   show the version
  --instructions  print the paragraph for an agent's instruction file

Output:
  Each row is the days until support ends — 0 on the day it ends, negative
  after — then the date, the software and its release cycle, and every place
  it was declared. One row is one release cycle, however many lines declared
  it: a multi-stage build naming the same base three times is one thing to
  deal with, and a file is named once with its lines behind it, as
  Dockerfile:2,22,34.
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
- A file that does not parse — a workflow or Compose file that is not YAML yet, a `mise.toml` that is not TOML yet, a `composer.json` that is not JSON yet — is skipped in silence, and whole: half a file is not half a set of declarations. The tool that owns it reports that better than this one can, and a file mid-edit is not a declaration that could not be read.
- The whole catalog — every product endoflife.date knows, with every cycle — is fetched from endoflife.date when a run needs it. There is no prebuilt database in the binary and nothing to update.
- The endoflife.date API is beta and says breaking changes can happen, so only the fields this tool needs are read and the rest is ignored.
- While the version is 0.x, the exit codes and the shape of the output can still change between releases; from 1.0 they only gain cases.

## Out of scope

- **Libraries.** A manifest is mostly libraries, and a library rarely has an end of life: it is released until it is not, and "old" is not "unsupported" when nobody promised support in the first place. Whether a library has been abandoned is a real question with no date behind it, and it belongs to [uzomuzo](https://github.com/future-architect/uzomuzo) and [xeol](https://github.com/xeol-io/xeol). What is read here is the other half of a manifest — the framework the application sits on, which publishes a support calendar for the same reason a distribution does. The two are told apart by the package names endoflife.date publishes and by nothing else, so the line is drawn by upstream rather than guessed at here.
- **Manifests other than a `Gemfile` and a `composer.json`.** `package.json` and `pyproject.toml` declare frameworks the same way, and each needs its own reading of how a requirement pins a version. Later.
- **Lockfiles.** A manifest that pins no single version has its answer in the lockfile beside it, which is not read: `Gemfile.lock` says which Rails was resolved where the `Gemfile` only said `>= 6.0`, and `composer.lock` the same for `^7.4 || ^8.0`. Those lines are set aside rather than guessed at, and `--verbose` names them.
- **Vulnerabilities.** They carry no date, so they do not belong on a timeline, and `osv-scanner`, `trivy`, and `grype` already read a directory for them. The two answers meet in one place worth saying out loud: once a runtime is past its end of life, the vulnerabilities found from then on are never fixed.
- **Deprecated GitHub Actions.** `actions/checkout@v2` has no machine-readable source to track, and unlike an expired base image it does not fail quietly — the workflow says so the next time it runs.
- **Terraform and cloud service versions.** The most valuable layer by far, but the one where the value is usually `var.eks_version` rather than a literal, which only Terraform itself can resolve. Later.
- **`requires-python` in `pyproject.toml`.** It states a range — `>=3.9` — which says what the project accepts rather than what it runs on, and a range has no single cycle to date. The version actually in use is in `.python-version`, the Dockerfile, or the workflow.
- **Scanning many directories.** One run reads one directory; a shell loop covers the rest.

## Credits

End-of-life dates come from [endoflife.date](https://endoflife.date), which is MIT licensed.

## License

MIT
