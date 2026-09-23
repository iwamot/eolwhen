# eolwhen

[![release](https://img.shields.io/github/v/release/iwamot/eolwhen)](https://github.com/iwamot/eolwhen/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/iwamot/eolwhen)](https://pkg.go.dev/github.com/iwamot/eolwhen)

Say which of a directory's declared versions are out of support.

```
$ eolwhen
eolwhen: Dockerfile:12: python sha256:3f1a2b is pinned by digest, which does not say which version it is
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
| `.ruby-version` | Ruby, or the JRuby or TruffleRuby it names |
| `.php-version` | PHP |
| `.go-version`, `go.mod` (the `go` directive) | Go |
| `.terraform-version` | Terraform |
| `mise.toml`, `.mise.toml`, `.tool-versions` | each tool listed under a bare name, and each package a `mise.toml` key names through a backend |
| `Dockerfile`, `Dockerfile.*` | whatever each `FROM` names — an official image, or any image endoflife.date publishes a purl for — and the distribution its tag was built on |
| `compose*.yml`, `docker-compose*.yml` (and `.yaml`) | the same, for each service's `image:`, unless the service has a `build:` and the image is what it builds |
| `Gemfile`, `gems.rb` | each `gem` whose name endoflife.date publishes as a gem — the framework the application sits on, not the libraries around it |
| `composer.json` | each package in `require` and `require-dev` whose name endoflife.date publishes, and the `php` the project runs on |
| `package.json` | each package in `dependencies`, `devDependencies`, `peerDependencies` and `optionalDependencies` whose name endoflife.date publishes, the `packageManager` the project is run with, and what `engines` asks of the host |
| `pyproject.toml` | each requirement in `dependencies`, `optional-dependencies` and `dependency-groups` whose name endoflife.date publishes, each dependency of a Poetry table, and the `requires-python` either of them writes |
| `requirements*.txt`, and any `.txt` in a `requirements/` directory | the same, one requirement per line |
| `*.csproj`, `*.fsproj`, `*.vbproj` | the target framework the project runs on — Microsoft .NET, or the .NET Framework |
| `pom.xml` | the `<parent>` it builds on and each `<dependency>` whose group and artifact endoflife.date publishes, where this file settles the version |
| `.github/workflows/*.yml` (and `.yaml`) | the `runs-on:` runner images, and the versions given to `actions/setup-node`, `-python`, `-go`, `-dotnet`, `-java`, `ruby/setup-ruby` and `shivammathur/setup-php`, including the ones a job's `strategy.matrix` lists and the ones an `env:` sets |

The directory is searched to the bottom, so a monorepo's `packages/web/.nvmrc` and a `docker/Dockerfile` are both found. Directories holding somebody else's code — `node_modules`, `vendor`, `third_party`, `.venv`, `.git` and the like — are skipped, because a `.nvmrc` inside a dependency is its author's declaration and not this directory's.

The place a version is written is what decides the software, so nothing is guessed from the text: `.nvmrc` is Node.js, `library/postgres` is PostgreSQL, and a gem or an npm package reaches a product only through the package names endoflife.date publishes for it. A line with no date to find — software endoflife.date does not track, a cycle it has not dated yet, `ubuntu-latest` or `redis:7` following the newest release on purpose — is set aside without a word, and `--verbose` accounts for it. A line that could not be read — a digest-pinned `FROM`, a version no release cycle covers — is reported on stderr rather than guessed at. [docs/reading.md](https://github.com/iwamot/eolwhen/blob/main/docs/reading.md) says how each kind of line is read, and why a line gets a row or none.

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

An empty table is one line on stderr, and that line is the answer. A directory can come out empty without anything being wrong — one running current versions, where no cycle it declares has been given an end date yet, is the usual case — and the line says which one it is:

```
$ eolwhen ../a-repo-kept-up-to-date
eolwhen: nothing declared in ../a-repo-kept-up-to-date has an end-of-life date yet
```

`--verbose` names the declarations that had no date to place — software endoflife.date does not track, cycles it has not dated yet, and the lines that follow the newest release. They are quiet by default because a repository doing everything right would otherwise spend a line on every file it keeps up to date, which is noise on the run that has an answer.

It also names the files that were recognized and read nothing from. A `compose.yml` that is not YAML yet, or a `package.json` whose `dependencies` is a list, is set aside whole — half a file is not half a set of declarations — and the tool that owns it says what is wrong with it better than this one can. That leaves a directory whose files all failed to parse looking exactly like one that declares nothing, so whenever there is no row the `eolwhen:` line counts them, and `--verbose` says which files and which of a closed set of reasons: `not JSON`, `not YAML`, `not TOML`, `not XML`, or `not the shape it is read for`. What a file held is never repeated back, a file mid-edit being able to hold anything at all:

```
$ eolwhen ../a-repo-mid-edit
eolwhen: no version declarations in ../a-repo-mid-edit
eolwhen: 1 file was recognized and read nothing from; --verbose names it

$ eolwhen --verbose ../a-repo-mid-edit
eolwhen: no version declarations in ../a-repo-mid-edit
eolwhen: compose.yml: not YAML, so nothing in it was read
```

Rows say what to change; they do not say how the tool got there. A name may be the catalog's own, an alias it lists, a package name upstream publishes for a registry, a Docker Hub repository, or a codename that names the software and the version at once — and a date may be the cycle's own or one carried over. `--verbose` says which, for every row it printed, and ends each line with the product's page as endoflife.date publishes it, read from the catalog rather than built out of the name:

```
$ eolwhen --verbose
eolwhen: compose.yml:9: 3.2 is redis <7.2, matched by name; no cycle covers it, so the date is the day 7.2 ended, which support for anything older had run out by — https://endoflife.date/redis
eolwhen: .nvmrc:1: 22.11.0 is nodejs 22, matched by an alias endoflife.date lists; the date is that cycle's own — https://endoflife.date/nodejs
-258d  2026-01-01  redis <7.2  compose.yml:9
+226d  2027-04-30  nodejs 22   .nvmrc:1
```

Every row is about a version a file declares, which is not what a directory runs on. A `go.mod` saying `go 1.16` is the oldest Go that module promises to work with: since Go 1.21 the go command fetches a newer toolchain when the one to hand is older, and a `toolchain` directive in the same file may name one outright. That directive is not read, and the two are not reconciled — which toolchain a build picks up is settled where the build runs.

```
$ eolwhen
-1650d  2022-03-15  go 1.16  go.mod:3
```

A module still promising to work with a Go that lost support in 2022 is worth going and changing. It is not a claim that anything is being built with Go 1.16.

A file set aside says the answer may be short of what the directory declares; it says nothing about whether anything there is out of support. An `eolwhen` that printed no row and counted two unread files has found no expiry and has not looked everywhere either, which are two different things to do next.

The exit code is the answer, so a check can be one line. This one fails the job when something has already expired, passes when only future dates were found, and fails when the run could not answer at all — a usage error or an unreachable endoflife.date:

```bash
eolwhen || [ $? = 2 ]
```

The exit code answers for the rows, so a job that also wants to know the rows were the whole story reads `skipped` beside it:

```bash
eolwhen --json > eol.json || [ $? = 2 ]
jq -e '.skipped | length == 0' eol.json
```

The same answer as JSON, with one entry per declaration rather than one per cycle, and `cycle` reading `<4.0` where the version predates everything endoflife.date tracks: the table folds them for reading, and a caller reading the document wants each place as its own record. Each entry also carries what its row was worked out from: `version` is the version read from the line, which the cycle no longer shows; `matched` is `name`, `alias`, `package`, `image` or `codename`; `dated` is `cycle` when the day is that cycle's own and `predates` when it was carried over from the oldest cycle tracked; `link` is the product's page. Read `dated` rather than the `<` the cycle name carries — Go writes that character escaped, so a caller looking for it in the document finds `\u003c` or nothing at all. Everything the table needs said in words on stderr is a field here instead: `hidden` is how many findings a `--within` window kept out, `ended` holds the cycles upstream calls out of support without publishing a day, `skipped` holds the files that were recognized and read nothing from, and `moving`, `untracked` and `undated` are filled when `--verbose` asks for the declarations that had no date to place — lines that follow the newest release, software endoflife.date has no policy for, and cycles it has not dated yet.

```
$ eolwhen --json
{
  "directory": ".",
  "findings": [
    {
      "product": "python",
      "cycle": "2.7",
      "version": "2.7.18",
      "eol": "2020-01-01",
      "days": -2450,
      "past": true,
      "matched": "name",
      "dated": "cycle",
      "link": "https://endoflife.date/python",
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
  "undated": [],
  "ended": [],
  "skipped": [
    {
      "source": "compose.yml",
      "reason": "not YAML"
    }
  ]
}
```

To cover several directories, loop over them in the shell. `eolwhen` reads one.

## For coding agents

`eolwhen --instructions` prints the paragraph to drop into `CLAUDE.md`, `AGENTS.md`, or whichever file your agent reads:

```markdown
To find out whether the runtimes, base images and frameworks a directory declares are still supported, use `eolwhen` instead of reading version files and checking dates by hand: `eolwhen` for the current directory, or `eolwhen DIR` for another one. It reads the version declarations in that one directory, matches them against endoflife.date, and prints one row per release cycle with the days until support ends, 0 on the day it ends and negative after, followed by every place that cycle was declared — a file is named once with its lines behind it, as `Dockerfile:2,22,34`, and `--json` has one entry per declaration instead. Add `--within 90d` to hide what expires further out than that; what has already expired is always shown, and the exit code then answers only for the rows that were printed. Exit 1 means something is already out of support and exit 2 means something will be, so both are answers and neither is a failure; exit 0 means no row was printed, and the single `eolwhen:` line says why — most often that nothing declared has an end-of-life date yet, which is nothing to do; exit 3 is a usage error and exit 4 means endoflife.date could not be read, which is worth one retry. Only rows go to stdout, so awk can read the first three columns; lines it could not read, and anything else the answer needs said in words, are `eolwhen:` lines on stderr; a line that follows the newest release on purpose, such as `ubuntu-latest`, is not one of them and only `--verbose` names it. A recognized file that does not parse is read as nothing at all, whole rather than in part; when there is no row the `eolwhen:` line counts those files, `--verbose` names each one with a short reason and `--json` carries them in `skipped`, so an empty answer always says whether it is a complete one. A row says what to change and not how it was reached, so `--verbose` explains each printed row — the version read from the line, how the name was answered, whether the date is that cycle's own, and the product's endoflife.date page — and `--json` carries the same as the `version`, `matched`, `dated` and `link` fields of each entry; `dated` is `predates` where the declared version is older than every cycle endoflife.date tracks, the date then being the day the oldest one ended rather than a day published for what was declared. Every row is about a version a file declares rather than about what is running: a `go 1.16` in a go.mod is the oldest Go that module promises to work with, and the build may fetch a newer toolchain.
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
  everything else is an `eolwhen:` line on stderr. The exit code answers for
  the rows that were printed, so --within narrows what it covers too.

Exit codes:
  0  nothing to report, including a directory with no declarations
  1  something is already out of support
  2  nothing has expired, but something will
  3  usage error: fix the flags or the directory
  4  endoflife.date could not be read: check the network, then retry
```

- A release cycle's end-of-life date is the first day without support, so something due today reads as `0d` and counts as expired. The sign says which side of today a date falls on, and the day it lands on takes none. Days are counted between calendar days, and today is today where you are: endoflife.date publishes a date rather than a moment, so counting from anyone else's calendar would put a date and a number that disagree on the same row.
- Rows are ordered by the date itself, oldest first, so the timeline runs in one direction and the most overdue reads at the top.
- The exit code answers for the rows that were printed, so `--within 90d` turns the run into a check for "is anything expiring in the next 90 days", and what the window hid is not counted. A cycle that is out of support with no date has no row either, and neither has a line that could not be read, so neither of those changes it.
- The same complaint from several places is said once, with a count. Nearly every workflow in a healthy repository says `runs-on: ubuntu-latest`, and knowing that once is enough; `grep` finds the rest.
- The whole catalog — every product endoflife.date knows, with every cycle — is fetched from endoflife.date when a run needs it. There is no prebuilt database in the binary and nothing to update.
- The endoflife.date API is beta and says breaking changes can happen, so only the fields this tool needs are read and the rest is ignored.
- While the version is 0.x, the exit codes and the shape of the output can still change between releases; from 1.0 they only gain cases.

## Out of scope

- **Libraries.** A manifest is mostly libraries, and a library rarely has an end of life: it is released until it is not, and "old" is not "unsupported" when nobody promised support in the first place. Whether a library has been abandoned is a real question with no date behind it, and it belongs to [uzomuzo](https://github.com/future-architect/uzomuzo) and [xeol](https://github.com/xeol-io/xeol). What is read here is the other half of a manifest — the framework the application sits on, which publishes a support calendar for the same reason a distribution does. The two are told apart by the package names endoflife.date publishes and by nothing else, so the line is drawn by upstream rather than guessed at here.
- **A POM's parent, and the properties it sets.** A `<version>` written as `${spring.version}` is read where the same file sets that property and left alone where a parent POM does, because following it would mean resolving a POM this directory may not hold. Maven itself is the tool for that.
- **Lockfiles.** A manifest that pins no single version has its answer in the lockfile beside it, which is not read: `Gemfile.lock` says which Rails was resolved where the `Gemfile` only said `>= 6.0`, and `composer.lock` the same for `^7.4 || ^8.0`. Those lines are set aside rather than guessed at, and `--verbose` names them.
- **Vulnerabilities.** They carry no date, so they do not belong on a timeline, and `osv-scanner`, `trivy`, and `grype` already read a directory for them. The two answers meet in one place worth saying out loud: once a runtime is past its end of life, the vulnerabilities found from then on are never fixed.
- **Deprecated GitHub Actions.** `actions/checkout@v2` has no machine-readable source to track, and unlike an expired base image it does not fail quietly — the workflow says so the next time it runs.
- **Versions inside `.tf` files.** The most valuable layer by far, but the one where the value is usually `var.eks_version` rather than a literal, which only Terraform itself can resolve. Later. A `.terraform-version` is a different thing and is read: it names the Terraform itself, as a literal.
- **Scanning many directories.** One run reads one directory; a shell loop covers the rest.

## Credits

End-of-life dates come from [endoflife.date](https://endoflife.date), which is MIT licensed.

## License

MIT
