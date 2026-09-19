# How a line is read

The place a version is written is what decides the software, so nothing is guessed from the text. A file name can only mean one thing: `.nvmrc` is Node.js. An official image name can only mean one thing too, because that namespace is a short curated list — `library/postgres` is PostgreSQL. Where a name could mean anything — a Docker Hub repository, a gem, an npm package — it is read through the names endoflife.date publishes for its products, and through nothing else.

Every declaration ends up in one of three places:

- **A row**, when the version reaches a release cycle that has an end-of-life date.
- **Set aside without a word**, when there is no date to find: software endoflife.date does not track, a cycle it has not dated yet, or a line that follows the newest release on purpose. Nothing is wrong with the line and there is nothing to go and change, so it is quiet by default and `--verbose` accounts for it.
- **Reported on stderr**, when the line could not be read — a digest-pinned `FROM`, an expression that cannot be worked out, a version no release cycle covers — or when endoflife.date calls the cycle out of support without publishing the day. The same complaint from several places is said once, with a count.

## The walk

The directory is searched to the bottom, so a monorepo's `packages/web/.nvmrc` and a `docker/Dockerfile` are both found. Directories holding somebody else's code — `node_modules`, `vendor`, `third_party`, `.venv`, `.git` and the like — are skipped, because a `.nvmrc` inside a dependency is its author's declaration and not this directory's.

A directory reached through a symlink is that directory. Links met further down the tree are left alone, in silence: a link names something that sits somewhere else, whose declarations belong where it sits rather than here, and leaving links alone is also what keeps a loop from being possible. Only plain files are read, so a pipe or a device named like a manifest is passed over as well.

A file the walk offers but cannot open is reported by path and the rest of the tree is still read. One unreadable corner is not a reason to refuse an answer, though the directory named on the command line has to be readable, being the question itself.

A file that does not parse — a workflow or Compose file that is not YAML yet, a `mise.toml` that is not TOML yet, a `composer.json` that is not JSON yet — is set aside whole, and without a complaint: half a file is not half a set of declarations. The tool that owns it reports that better than this one can, and a file mid-edit is not a declaration that could not be read.

Setting it aside quietly is not forgetting it. A directory whose recognized files all failed to parse reads exactly like one that declares nothing, so whenever there is no row the `eolwhen:` line counts them — `2 files were recognized and read nothing from` — and `--verbose` names each one. The reason is one of a closed set, so a caller tells them apart without reading prose: `not JSON`, `not YAML`, `not TOML`, `not XML`, and `not the shape it is read for` for a document that parses and then does not hold what it is read for, such as a `package.json` whose `dependencies` is a list rather than an object. `--json` carries the same pairs in `skipped`.

Nothing the file itself held is passed on. A parser's own message quotes the line it stopped on, and a file mid-edit may hold anything — a token, a password in a Compose file that is not YAML yet — so the path and the reason are the whole of what is said about it.

## From a version to a row

A version reaches its release cycle by dot-separated segments, not by string prefix: `3.10.2` reaches Python's 3.10 cycle and never its 3.1 cycle, which expired in 2012. Versions are read as they are written, quoted or not. A workflow that says `python-version: 3.10` means the 3.10 line, and reading it as a number would make it 3.1, which is a different Python.

A tag that names a codename instead of a number is resolved through the codename endoflife.date publishes, so `FROM debian:buster` reads as Debian 10 and `FROM ubuntu:jammy` as Ubuntu 22.04. A codename two products share is read as neither, since which one a tag meant would be a guess — and a codename is worth reading precisely because it needs none. The same word is what nvm writes in `.nvmrc` as `lts/hydrogen`, which is Node.js 18.

A tag that names only a major line — `redis:7`, or `python-version: 3` — is not a version but a rule for following one: the image moves to the next 7.x the day it exists. It gets no row, because a date that changes on its own is not a date this tool can put on a timeline. Neither does `:latest`, `ubuntu-latest`, or a tool pinned to `stable`: those lines follow the newest release on purpose, so there is nothing wrong with them and nothing to go and change. They are set aside without a word, because a complaint nobody can act on is what buries the rows that matter.

A version older than every cycle endoflife.date tracks gets a row of its own. A `redis:3.2` in a Compose file is the most neglected line in the directory, and reading it as a version nobody has heard of is the one answer that is certainly wrong — the catalog starts at 4.0 because everything below it stopped being a going concern long ago. The row carries the day that oldest cycle ended, which support for anything older had already run out by, and names the cycle `<4.0` so that it says what it knows rather than a day it cannot know:

```
$ eolwhen
-2270d  2020-07-01  redis <4.0  compose.yml:12
```

A version that reaches a cycle still gets no row when endoflife.date has given that cycle no end date. Support has not been dated yet, which is what a recent release looks like, so it is set aside without a word. Upstream sometimes says the opposite: that a cycle is out of support, without publishing the day it happened. There is still no date to put on a timeline, so there is still no row — but a dead release is not a current one, and it is what the reader ran the tool to find out. Those cycles are reported on stderr like a line that could not be read, whether or not `--verbose` was asked for.

Software endoflife.date does not track is passed over without a word, whatever version it was given. Plenty of tools publish no end-of-life policy at all, and there was never a date to find for them; a `jq = "latest"` is a line about jq before it is a line about latest.

## Runtime version files

`.python-version`, `.nvmrc`, `.node-version`, `.ruby-version`, `.php-version`, `.go-version` and `.terraform-version` each name one runtime by their file name, and the `go` directive of a `go.mod` names Go the same way. `.python-version` may name several versions, as pyenv allows; each gets its own row.

## Tool lists

A tool list is the one place where finding the file does not promise there is anything to look up. `.nvmrc` is Node.js and Node.js has an end-of-life policy; a `mise.toml` or `.tool-versions` holds whatever the project uses, and `biome`, `hugo` and `jq` have none at all. Those are set aside without a word.

A `mise.toml` key may carry a backend — `aqua:`, `go:`, `npm:` — and then it names a package rather than a tool. The backend is what fixes the registry, so the name is answered as a `Gemfile`'s is: through the purls endoflife.date publishes for that registry and through nothing else. `aqua:`, `github:` and `ubi:` install from a GitHub release and name the repository, which is a github purl; `cargo:`, `conda:`, `dotnet:`, `gem:`, `go:`, `npm:`, `pipx:` and `spm:` name their own registries; `core:` names one of mise's own tools, which a bare name reaches anyway. A backend with no registry to look a name up in — an asdf or vfox plugin, a download from a URL or a bucket — is passed over. A `.tool-versions` has no backends: asdf reads a plugin name and nothing else.

## Dockerfiles

Each `FROM` is read. An official image name can only mean one thing, because the library namespace is a short curated list. Outside that library anyone can name an image anything, so the name alone settles nothing — but endoflife.date publishes the Docker Hub repository each of its products ships under, as a purl. `opensearchproject/opensearch` is OpenSearch because upstream says so, and `FROM ghcr.io/acme/python:3.7` is still not Python, because nobody said so. An image nobody published a purl for is a declaration of software the catalog does not track, like `jq` in a tool list, and is set aside without a word.

`FROM python@sha256:...` with no tag is reported, because a digest does not say which version it is.

A `FROM` written in terms of a build argument is read as `docker build` with no `--build-arg` resolves it, which is the build the file describes: the argument's default, or nothing at all when it was declared without one. That is what makes `FROM ${REGISTRY}python:3.11-bullseye` under a bare `ARG REGISTRY` read as the official image it falls back to. Only the arguments above the first `FROM` are in scope for one, which is Docker's own rule, and a name no `ARG` declares comes from outside the file, so the line is reported instead.

### The other half of an image tag

`FROM python:3.11-bullseye` declares two things. The Python is one, and the Debian 11 it is built on is the other — and the Debian is usually the half that expires first, because a base image outlives the runtime's own support window. So the variant after the dash is read too: a segment spelled `alpine3.19` names Alpine 3.19 outright, and a segment that is one word may be a distribution codename, which names the release and the distribution at once. Which words those are is endoflife.date's answer and not a list kept here, so `bookworm` reads as Debian 12 while `slim`, `fpm` and `jre` are builds of something rather than releases of anything, and say nothing.

## Compose files

A Compose file is read service by service, and each service's `image:` is read as a `FROM` is. What sits under `services:` is a service and nothing else is, because a file keeps its templates in extension fields — `x-defaults` and the like — and a service pulls one in with a merge key. Merges are applied the way YAML applies them, so a service that sets its own image over an inherited one declares the image it set. A service with a `build:` declares nothing through its `image:`, which names what to call what it builds: that is the output, and the base it stands on is in the Dockerfile the build points at, which is read on its own.

## Manifests

A manifest is mostly libraries, and a library rarely has an end of life. What it also holds is the framework the application sits on, and a framework publishes a support calendar for the same reason a distribution does — Rails 6.1 stopped getting security fixes on 2024-10-01, and an application still asking for it is in the same position as one still asking for Debian 10.

Which of the two a line names is upstream's answer rather than a guess made here. endoflife.date publishes the package name for the products that have one, so `rails` is Ruby on Rails and `laravel/framework` is Laravel on its word, and every other package is software it does not track and is passed over in silence. That is what keeps `gem "pg"` from reading as PostgreSQL: the database answers to `pg` as an alias, but the gem is the driver, which is different software on a different calendar, and a registry anyone can publish to is not a namespace to read names out of.

```
$ eolwhen
-1390d  2022-11-28  php 7.4    composer.json:4
-1333d  2023-01-24  laravel 8  composer.json:6
 -717d  2024-10-01  rails 6.1  Gemfile:3
```

### Ranges

A requirement usually pins a range rather than a version, and a range is still an answer when one release cycle is the only one it can be, because a row is about a cycle. Every version `~> 6.1.0` allows is Rails 6.1 and every version `^8.0` allows is Laravel 8, so those lines are dated. `~> 6` is not: it admits both 6.0 and 6.1, and which one was installed is written in the lockfile, which this does not read. Neither is one that leaves the upper end open, as `>= 6.0` does, nor one that leaves two ranges behind, as the union `^7.4 || ^8.0` does, nor a package given no requirement at all. Those are set aside without a word, and `--verbose` names them.

A version with a letter in it — `7.1.0.rc1` — is passed over too. Where a pre-release falls against a release is the package manager's rule rather than a number's, and getting it wrong would date a line by a cycle it is not in.

Each manifest writes its ranges in its own operators, and the same three characters can mean different things in two files. `~1.2` in a `composer.json` is every 1.x, while `~1.2` in a `package.json` is every 1.2.x, so each file is read by its own arithmetic rather than by a shared guess.

### What a manifest asks of the host

A manifest also says what it asks of the host it runs on, and that is read as the same kind of requirement: a `composer.json`'s `php`, a `package.json`'s `engines`, a `pyproject.toml`'s `requires-python` and the `python` key of a Poetry table. Written open — `>=3.9`, `>=22` — it names a floor and no ceiling and so names no release cycle, which is how one is usually written and why it usually says nothing. Written closed — `^7.4`, `^18`, `==3.11.*` — it names one, and that is the runtime the project runs on as squarely as a `.python-version` would say it. Which of the two it is settles the answer; there is no line drawn between accepting a version and running on one, because a manifest does not draw one.

### Gemfile and gems.rb

Each `gem` line is read, and reaches a product only through the gem names endoflife.date publishes.

### composer.json

Each package in `require` and `require-dev` is read. One line is not a package at all: Composer reserves `php` for the version of the language the project runs on, so that key can only mean PHP — not because the name reads that way, but because Composer says so, which is the same kind of answer a `.python-version` gives. Its other reserved names are passed over: `ext-` and `lib-` ask for extensions, and `composer` and `hhvm` for the tooling, none of which is software with a release calendar of its own.

### package.json

Each package in `dependencies`, `devDependencies`, `peerDependencies` and `optionalDependencies` is read. A `package.json` also writes a line of versions as `16.x` or `3.4.*`, which names a cycle as squarely as a caret does. What it leaves for the lockfile is read as that: a union (`^7 || ^8`), a hyphen range, a floor with no ceiling, an alias (`npm:lodash-es@^4`), a `workspace:` or `file:` or repository requirement, and a dist-tag such as `latest`.

`packageManager` names the package manager the project is run with and the exact version of it, which is what corepack installs, so it declares a tool the way a `mise.toml` entry does. The field's rule is one exact version, so a range or a `latest` there is a line to go and look at rather than one a resolver settles.

### pyproject.toml and requirements.txt

Python is the ecosystem where the ranges usually do decide. A `requirements.txt` is written with `==`, which names one version, and `~=4.2.0` and `==4.2.*` each name one release cycle outright — so a neglected Python project says which Django it is running rather than leaving it to a resolver. Any `requirements*.txt`, and any `.txt` in a `requirements/` directory, is read one requirement per line. A `pyproject.toml` writes the same requirements in `dependencies`, in an extra under `optional-dependencies`, or in a dependency group, and all of them are read. What is not read is the rest of the line: the extras in `django[argon2]`, which name parts of the same package, and the marker after a semicolon, which says when a requirement applies rather than to what. An exclusion (`!=`) leaves a range with a hole in it, and sets the whole requirement aside.

Poetry writes the same dependencies its own way, as a key with a constraint rather than as a PEP 508 string, and a `pyproject.toml` is read both ways — a project moving from one to the other has both for a while. Poetry's operators are npm's rather than PEP 440's: `^4.2` is every 4.x from 4.2, `~1.14.0` every 1.14.x, and a version on its own is that version. An entry that names no version of its own — a dependency taken from a repository or a path, or one written as a constraint per interpreter — is set aside like any requirement an install settles. Its `python` key is left alone, being `requires-python` under another name.

PyPI treats a name with dashes, underscores and dots as one name — `typing_extensions` and `typing-extensions` are the same package — so a manifest may spell it any of those ways and still reach the product. That rule is PyPI's alone, and no other registry here gets it.

### pom.xml

A `pom.xml` is the one manifest that does not close over itself. A version may be written as `${spring.version}`, and the property may be set in a parent POM this file does not hold; a dependency may carry no version at all, the parent deciding it, which is how a Spring Boot project is usually written. What is read is what the file settles on its own: a literal `<version>`, and a property the project's own `<properties>` sets. Everything else names no version here, and Maven is the one that resolves it. The `<parent>` it builds on and each `<dependency>` whose group and artifact endoflife.date publishes are read wherever they are declared — under `<dependencyManagement>`, or in a `<profile>` — because each of those is the project saying which version it builds with. A profile's own properties are not read: two profiles may set the same one differently, and which of them applies is settled when the build runs.

## .NET project files

A `.csproj`, `.fsproj` or `.vbproj` says which runtime the project runs on, and nothing is guessed from the text there either: the target framework monikers are a vocabulary .NET defines. The word in front of the digits says which .NET it is. `net6.0` is Microsoft .NET and `net472` is the .NET Framework, which are different products on different calendars, and the dot is what tells them apart — the `.0` in `net5.0` was added so that the two could never be read as each other. A moniker of bare digits is one digit per segment, so `net481` is 4.8.1. `net35` reads as the 3.5 service pack, which is the only 3.5 still installable and the cycle endoflife.date tracks; reading it as a plain 3.5 would reach no cycle at all and date a target that is still supported by the day the 4.0 above it expired.

A platform on the end — the `-windows` of `net8.0-windows` — says which APIs the target adds rather than which version of it is meant, so it is cut away. `<TargetFrameworks>` holds several at once, and each is a declaration of its own, as each line of a `.python-version` is. A project written before the SDK-style project existed spells the same declaration `<TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion>`, which only ever named a .NET Framework.

```
$ eolwhen
-1606d  2022-04-26  dotnetfx 4.5.2  src/Legacy/Legacy.csproj:4
 -675d  2024-11-12  dotnet 6        src/Api/Api.csproj:3
```

A moniker naming something that is not a runtime is looked up under the name it gave, and nothing answers to it: `netstandard2.0` is an API contract rather than a thing that runs, and nobody publishes an end-of-life date for one, so it is set aside like a tool the catalog does not track. A target framework written as `$(DefaultTargetFramework)` names no version of its own — the value is in a `Directory.Build.props` or on the command line, neither of which is read — so there is nothing to place and nothing to go and change. `--verbose` accounts for both.

## GitHub workflows

Every `.yml` and `.yaml` under `.github/workflows` is read for its `runs-on:` labels and for the versions given to `actions/setup-node`, `-python`, `-go`, `-dotnet`, `-java`, `ruby/setup-ruby` and `shivammathur/setup-php`.

### Runner labels

A runner label needs no translation at all: endoflife.date's runner-image product names its release cycles `macos-13` and `ubuntu-22.04`, which is exactly what `runs-on:` says. A label is only read when it is spelled the way one of GitHub's images is: a family, then a version or the word latest. A custom pool is free to start with the same word — `ubuntu-x64-small` and `ubuntu-slim` are real ones — so `self-hosted` and its kind are left alone, because they name a machine and not a version. GitHub writes `ubuntu-24.04-arm` where the catalog writes `ubuntu-24.04-arm64`, so a row spells it the catalog's way.

The runner-image catalog holds the images GitHub offers and the ones it retired most recently, so a label retired longer ago — `ubuntu-18.04`, `windows-2019` — is reported as covered by no release cycle. A workflow still asking for one has already stopped running.

### Versions written once and read elsewhere

A version written once and referred to elsewhere is still read: a workflow that keeps it in `env:` under an anchor and writes `python-version: *python` in the step declares it at the step, which is the line to go and change. An anchor may be defined more than once, and an alias means the definition above it, as YAML says; one added further down does not reach back and change what an earlier reference meant.

### What a matrix declares

A workflow that writes `python-version: ${{ matrix.python-version }}` is not hiding the versions it runs on — they are listed a few lines above it, in the same job's `strategy.matrix`, which is where a project says which versions it supports. Each is read there, and reported at the line it was written on, which is the line to go and change:

```
$ eolwhen
-320d  2025-10-31  python 3.9  .github/workflows/ci.yml:8
```

A matrix belongs to its job, so one job's `os:` says nothing about another's. `include:` holds whole combinations and may name a version the list does not, so what it says under a key is read as another value of it; `exclude:` takes combinations away and declares nothing. A matrix built by an expression — `fromJSON` of another job's output — lists nothing to read, and an expression doing more than naming one key is left alone: what it works out to is the workflow's to decide at run time.

### Which build of Java

There is no such thing as a version of Java on its own. endoflife.date tracks nine builds of it — Temurin, Zulu, Corretto, Oracle's own and the rest — each with a calendar of its own, so `17` says nothing until something says whose 17 it is. That is why a `<maven.compiler.source>` in a `pom.xml` and a `.java-version` are not read: neither names a build.

`actions/setup-java` does, in an input of its own:

```yaml
- uses: actions/setup-java@v4
  with:
    distribution: temurin
    java-version: '8'
```

The value is a closed vocabulary the action defines, and the catalog already answers to most of it — `temurin` is Eclipse Temurin and `corretto` is Amazon Corretto by upstream's own aliases — so the word is handed on as it was written. Only `oracle` and `microsoft`, which the catalog has no alias for, are spelled its way instead. A distribution it knows nothing about is passed over like any other software it does not track, and a step naming none installs nothing this can date.

## When nothing is printed

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
