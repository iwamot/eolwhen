// Package workflow reads GitHub Actions workflows: the runner each job asks
// for, and the language versions the setup-* actions are told to install.
//
// Both are places where the declaration decides the software without anything
// having to be guessed from the text. A runner label is a cycle name in
// endoflife.date's runner-image product, and an action name says which
// runtime its version input is about.
package workflow

import (
	"regexp"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/ruby"
	"github.com/iwamot/eolwhen/internal/yamlfile"
)

// Runners is the product whose release cycles are the runner labels
// themselves, so `runs-on: macos-13` needs no translation at all.
const Runners = "github-actions-runner-images"

// setups are the actions that install a runtime, with the input that carries
// its version. Each is listed because it was checked: an action named
// setup-something is not on its own a promise about what it installs.
//
// One of them names no software by itself. There is no such thing as a
// version of Java on its own — endoflife.date tracks nine builds of it, each
// with a calendar of its own — so setup-java says which in an input beside
// the version, and Names carries where to read the software rather than
// Product carrying it.
//
// Another names its software in the version itself. setup-ruby installs
// JRuby and TruffleRuby as readily as Ruby, written jruby-9.4 where Ruby is
// 3.3, so Engine reads which implementation a version is of before the
// version is read.
var setups = map[string]struct {
	Input   string
	Product string
	Names   string
	Engine  func(string) (engine, version, reason string)
}{
	"actions/setup-node":     {Input: "node-version", Product: "node"},
	"actions/setup-python":   {Input: "python-version", Product: "python"},
	"actions/setup-go":       {Input: "go-version", Product: "go"},
	"actions/setup-dotnet":   {Input: "dotnet-version", Product: "dotnet"},
	"actions/setup-java":     {Input: "java-version", Names: "distribution"},
	"ruby/setup-ruby":        {Input: "ruby-version", Product: "ruby", Engine: ruby.Read},
	"shivammathur/setup-php": {Input: "php-version", Product: "php"},
}

// distributions are the builds of Java the catalog answers to under another
// name than the one setup-java writes. It answers to most of them already —
// temurin is eclipse-temurin and corretto is amazon-corretto by upstream's
// own aliases — so what is here is the two it has no alias for. The value is
// a closed vocabulary the action defines, which is what makes translating it
// a reading rather than a guess.
//
// A distribution this does not name is handed on as it was written, and
// whether anything is known about it is the catalog's to say.
var distributions = map[string]string{
	"microsoft": "microsoft-build-of-openjdk",
	"oracle":    "oracle-jdk",
}

// families are the runner images GitHub hosts. A label outside them names
// somebody's own machine, whatever they liked to call it.
var families = []string{"ubuntu", "macos", "windows"}

// hosted reports whether a label is spelled the way one of GitHub's images
// is: a family, then either a version or the word latest. A custom pool is
// free to start with the same word — ubuntu-x64-small, ubuntu-slim are real
// ones — so the family alone does not settle it, and reading those as
// versions is what filled the output with labels that were never versions.
func hosted(label string) bool {
	for _, f := range families {
		rest, ok := strings.CutPrefix(label, f+"-")
		if !ok {
			continue
		}
		return rest == "latest" || (rest != "" && rest[0] >= '0' && rest[0] <= '9')
	}
	return false
}

// catalogLabel spells a label the way endoflife.date names the cycle. GitHub
// writes ubuntu-24.04-arm and windows-11-arm; the catalog writes them with
// the architecture in full.
func catalogLabel(label string) string {
	if rest, ok := strings.CutSuffix(label, "-arm"); ok {
		return rest + "-arm64"
	}
	return label
}

// Matches reports whether a path is a workflow. The directory is part of the
// answer: a YAML file is only a workflow because of where it sits.
func Matches(path string) bool {
	if !strings.HasPrefix(path, ".github/workflows/") {
		return false
	}
	return strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")
}

// Extract reads one workflow, job by job.
//
// Reading it as jobs rather than as every mapping in the file is what lets a
// `${{ matrix.python-version }}` be answered: the versions it stands for are
// in the same job's strategy.matrix, and a matrix is only the one job's.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable, string) {
	parsed, skipped := yamlfile.Parse(data)
	if skipped != "" {
		return nil, nil, skipped
	}
	r := &reader{
		file:     file,
		declared: map[decl.Decl]bool{},
		reported: map[decl.Unreadable]bool{},
	}
	for _, root := range parsed.Roots() {
		jobs, ok := yamlfile.Find(root, "jobs")
		if !ok {
			continue
		}
		byID, ok := jobs.Mapping()
		if !ok {
			continue
		}
		for _, job := range byID {
			r.job(job)
		}
	}
	return r.ds, r.us, ""
}

type reader struct {
	file string
	ds   []decl.Decl
	us   []decl.Unreadable
	// declared and reported hold what has already been recorded, so that two
	// steps reading the same matrix key record the versions it lists once.
	// Both point at the same lines of the matrix, and one line written once
	// is one declaration however many steps read it.
	declared map[decl.Decl]bool
	reported map[decl.Unreadable]bool
}

func (r *reader) at(e yamlfile.Entry) decl.Source {
	return decl.Source{File: r.file, Line: e.Line()}
}

func (r *reader) declare(e yamlfile.Entry, product, version string) {
	d := decl.Decl{Product: product, Version: version, Source: r.at(e)}
	if !r.declared[d] {
		r.declared[d] = true
		r.ds = append(r.ds, d)
	}
}

// report files a line that could not be read. product is the software the
// line is about when the step names it apart from the version, as a setup-*
// action does, so that a line about software the catalog does not track is
// set aside like any other declaration of it; a runner label names its own.
func (r *reader) report(e yamlfile.Entry, product, text, reason string, moving bool) {
	u := decl.Unreadable{Source: r.at(e), Product: product, Text: text, Reason: reason, Moving: moving}
	if !r.reported[u] {
		r.reported[u] = true
		r.us = append(r.us, u)
	}
}

// job reads one job: the runner it asks for, and the steps it runs, both
// with the job's matrix in hand.
func (r *reader) job(job yamlfile.Entry) {
	entries, ok := job.Mapping()
	if !ok {
		return
	}
	m := matrixOf(entries)
	if e, ok := yamlfile.Find(entries, "runs-on"); ok {
		r.runsOn(e, m)
	}
	steps, ok := yamlfile.Find(entries, "steps")
	if !ok {
		return
	}
	items, ok := steps.Sequence()
	if !ok {
		return
	}
	for _, item := range items {
		step, ok := item.Mapping()
		if !ok {
			continue
		}
		uses, hasUses := yamlfile.Find(step, "uses")
		with, hasWith := yamlfile.Find(step, "with")
		if hasUses && hasWith {
			r.step(uses, with, m)
		}
	}
}

// matrix is the values a job's strategy.matrix lists under each key, as the
// entries they were written as, so that a version read through one is
// reported at the line it was written on. That line is the one to go and
// change: a matrix is where a project says which versions it supports.
type matrix map[string][]yamlfile.Entry

// matrixOf reads a job's strategy.matrix.
//
// A key holds the list of values the job runs over. include: holds whole
// combinations rather than values, and each of them may name a version the
// list does not, so what it says under a key is read as another value of it.
// exclude: takes combinations away; a version it names is still one the file
// declares elsewhere, so nothing is read from it.
func matrixOf(job []yamlfile.Entry) matrix {
	strategy, ok := yamlfile.Find(job, "strategy")
	if !ok {
		return nil
	}
	entries, ok := strategy.Mapping()
	if !ok {
		return nil
	}
	e, ok := yamlfile.Find(entries, "matrix")
	if !ok {
		return nil
	}
	keys, ok := e.Mapping()
	if !ok {
		// A matrix built by an expression — fromJSON of a job's output —
		// lists nothing here to read.
		return nil
	}
	m := matrix{}
	for _, key := range keys {
		if key.Key == "exclude" {
			continue
		}
		values, ok := key.Sequence()
		if !ok {
			continue
		}
		if key.Key != "include" {
			m[key.Key] = append(m[key.Key], values...)
			continue
		}
		for _, combination := range values {
			under, ok := combination.Mapping()
			if !ok {
				continue
			}
			for _, v := range under {
				m[v.Key] = append(m[v.Key], v)
			}
		}
	}
	return m
}

// reMatrixValue matches a value that is one matrix key and nothing else,
// which is the shape a version or a runner label is written in when the
// matrix carries it. An expression doing anything more than that — joining
// a label together, choosing between two — is left to be reported: what it
// works out to is the workflow's to decide at run time.
var reMatrixValue = regexp.MustCompile(`^\$\{\{\s*matrix\.([A-Za-z_][A-Za-z0-9_-]*)\s*\}\}$`)

// lookup reads a field that may stand for a matrix key. named says the field
// is one and nothing else; listed says the matrix has values under it, which
// a matrix built by an expression, or one belonging to another job, does not.
func (m matrix) lookup(text string) (vs []yamlfile.Entry, named, listed bool) {
	key := reMatrixValue.FindStringSubmatch(text)
	if key == nil {
		return nil, false, false
	}
	vs, listed = m[key[1]]
	return vs, true, listed
}

// runsOn reads the runner labels a job asks for. The field takes one label,
// a list of them, or a group with its labels spelled out, and each shape is
// read to the depth it has: a group holds labels, a list holds labels, and
// there it ends. A field that names itself is then a list whose one item is
// not a label, rather than a descent with nothing to stop it.
func (r *reader) runsOn(e yamlfile.Entry, m matrix) {
	if entries, ok := e.Mapping(); ok {
		if labels, ok := yamlfile.Find(entries, "labels"); ok {
			r.labels(labels, m)
		}
		return
	}
	r.labels(e, m)
}

// labels reads either one label or a list of them. What a list holds is
// labels, and nothing deeper.
func (r *reader) labels(e yamlfile.Entry, m matrix) {
	if items, ok := e.Sequence(); ok {
		for _, item := range items {
			r.label(item, m)
		}
		return
	}
	r.label(e, m)
}

func (r *reader) label(e yamlfile.Entry, m matrix) {
	label, ok := e.Scalar()
	if !ok {
		return
	}
	if vs, named, listed := m.lookup(label); named {
		if !listed {
			r.report(e, "", oneLine(label), "takes its runner from an expression", false)
			return
		}
		// The matrix lists the labels this field stands for, and each is
		// read where it was written. What a matrix holds is a label itself,
		// so the matrix is done with.
		for _, v := range vs {
			r.label(v, nil)
		}
		return
	}
	switch {
	case strings.Contains(label, "${{"):
		r.report(e, "", oneLine(label), "takes its runner from an expression", false)
	case !hosted(label):
		// A self-hosted or custom label names a machine, not a version, so
		// there was never a date to look for.
	case strings.HasSuffix(label, "-latest"):
		r.report(e, "", label, "names latest, not a version", true)
	default:
		r.declare(e, Runners, catalogLabel(label))
	}
}

// step reads the version a setup-* action is told to install. The action name
// decides the runtime, and where the action installs more than one
// implementation of it, the version says which; anything else with a with:
// block is left alone.
func (r *reader) step(usesEntry, withEntry yamlfile.Entry, m matrix) {
	uses, ok := usesEntry.Scalar()
	if !ok {
		return
	}
	name, _, _ := strings.Cut(uses, "@")
	setup, ok := setups[name]
	if !ok {
		return
	}
	with, ok := withEntry.Mapping()
	if !ok {
		return
	}
	product := setup.Product
	if setup.Names != "" {
		named, ok := r.installs(with, setup.Names)
		if !ok {
			return
		}
		product = named
	}
	for _, e := range with {
		if e.Key != setup.Input {
			continue
		}
		raw, ok := e.Scalar()
		if !ok {
			continue
		}
		vs, named, listed := m.lookup(strings.TrimSpace(raw))
		if named && !listed {
			r.report(e, product, oneLine(raw), "takes its version from an expression", false)
			continue
		}
		if !named {
			// The field carries the versions itself.
			vs = []yamlfile.Entry{e}
		}
		for _, v := range vs {
			text, ok := v.Scalar()
			if !ok {
				continue
			}
			// A version input may be a block of them, one per line, which is
			// how a workflow installs several at once.
			for line := range strings.SplitSeq(text, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				software := product
				if setup.Engine != nil {
					engine, rest, reason := setup.Engine(line)
					if reason != "" {
						r.report(v, engine, oneLine(rest), reason, true)
						continue
					}
					software, line = engine, rest
				}
				got, reason, moving := version(line)
				if reason != "" {
					r.report(v, software, oneLine(line), reason, moving)
					continue
				}
				r.declare(v, software, got)
			}
		}
	}
}

// installs reads the input that says which software a step installs, which
// is setup-java's distribution and nothing else so far. A build of Java is
// what has a calendar; which one it is is not a thing to work out from the
// version beside it.
//
// A step that does not say is left without a word. setup-java requires the
// input, so a step missing it does not run, and there is nothing to report
// about a step that never was. An expression is reported like any other this
// file cannot work out.
func (r *reader) installs(with []yamlfile.Entry, input string) (string, bool) {
	e, found := yamlfile.Find(with, input)
	if !found {
		return "", false
	}
	name, ok := e.Scalar()
	if !ok {
		return "", false
	}
	if strings.Contains(name, "${{") {
		r.report(e, "", oneLine(name), "takes its distribution from an expression", false)
		return "", false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", false
	}
	if product, ok := distributions[name]; ok {
		return product, true
	}
	return name, true
}

// version turns a setup-* version input into a version, or says why it is
// not one. A trailing wildcard segment is dropped, since 20.x and 1.21.x name
// the cycle and leave the patch to the action. An input that is nothing but
// a wildcard, or one of the words for the newest release, is a moving target
// rather than a version left unread.
func version(s string) (v, reason string, moving bool) {
	if strings.Contains(s, "${{") {
		return "", "takes its version from an expression", false
	}
	v = strings.TrimPrefix(s, "v")
	for {
		base, last, found := cutLast(v)
		if !found || (last != "x" && last != "*") {
			break
		}
		v = base
	}
	switch {
	case v == "latest", v == "lts", v == "lts/*", v == "stable", v == "*":
		return "", "names a moving target, not a version", true
	case v == "" || v[0] < '0' || v[0] > '9':
		return "", "is not a version", false
	}
	return v, "", false
}

// oneLine folds a value onto one line, cutting it if it is long.
//
// An expression may run over several lines — a runs-on: choosing between
// runners does — and a complaint spanning lines breaks the promise that
// every line of stderr starts with the program's name. Folding rather than
// taking the first line keeps something readable: an expression often opens
// with nothing but ${{. The whole of it is still in the file it came from.
func oneLine(s string) string {
	folded := strings.Join(strings.Fields(s), " ")
	if len([]rune(folded)) <= oneLineLimit {
		return folded
	}
	return string([]rune(folded)[:oneLineLimit]) + " …"
}

// oneLineLimit is where a folded value is cut. Wide enough for an expression
// to say which inputs it reads, short enough to leave room for the reason
// beside it.
const oneLineLimit = 60

func cutLast(s string) (base, last string, found bool) {
	i := strings.LastIndex(s, ".")
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}
