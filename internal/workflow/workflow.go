// Package workflow reads GitHub Actions workflows: the runner each job asks
// for, and the language versions the setup-* actions are told to install.
//
// Both are places where the declaration decides the software without anything
// having to be guessed from the text. A runner label is a cycle name in
// endoflife.date's runner-image product, and an action name says which
// runtime its version input is about.
package workflow

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/yamlfile"
)

// Runners is the product whose release cycles are the runner labels
// themselves, so `runs-on: macos-13` needs no translation at all.
const Runners = "github-actions-runner-images"

// setups are the actions that install a runtime, with the input that carries
// its version. Each is listed because it was checked: an action named
// setup-something is not on its own a promise about what it installs.
var setups = map[string]struct {
	Input   string
	Product string
}{
	"actions/setup-node":     {"node-version", "node"},
	"actions/setup-python":   {"python-version", "python"},
	"actions/setup-go":       {"go-version", "go"},
	"actions/setup-dotnet":   {"dotnet-version", "dotnet"},
	"ruby/setup-ruby":        {"ruby-version", "ruby"},
	"shivammathur/setup-php": {"php-version", "php"},
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

// Extract reads one workflow.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	r := &reader{file: file, yaml: yamlfile.Parse(data)}
	r.yaml.Walk(r.mapping)
	return r.ds, r.us
}

type reader struct {
	file string
	yaml *yamlfile.File
	ds   []decl.Decl
	us   []decl.Unreadable
}

func (r *reader) at(e yamlfile.Entry) decl.Source {
	return decl.Source{File: r.file, Line: e.Line()}
}

func (r *reader) report(e yamlfile.Entry, text, reason string) {
	r.us = append(r.us, decl.Unreadable{Source: r.at(e), Text: text, Reason: reason})
}

// mapping is handed every mapping in the document. A job names its runner and
// a step names its action, and both are just keys in a mapping, so there is
// no need to know which level of the workflow this one is.
func (r *reader) mapping(entries []yamlfile.Entry) {
	if e, ok := yamlfile.Find(entries, "runs-on"); ok {
		r.runsOn(e)
	}
	uses, hasUses := yamlfile.Find(entries, "uses")
	with, hasWith := yamlfile.Find(entries, "with")
	if hasUses && hasWith {
		r.step(uses, with)
	}
}

// runsOn reads the runner labels a job asks for. The field takes one label,
// a list of them, or a group with its labels spelled out, and each shape is
// read to the depth it has: a group holds labels, a list holds labels, and
// there it ends. A field that names itself is then a list whose one item is
// not a label, rather than a descent with nothing to stop it.
func (r *reader) runsOn(e yamlfile.Entry) {
	if entries, ok := e.Mapping(); ok {
		if labels, ok := yamlfile.Find(entries, "labels"); ok {
			r.labels(labels)
		}
		return
	}
	r.labels(e)
}

// labels reads either one label or a list of them. What a list holds is
// labels, and nothing deeper.
func (r *reader) labels(e yamlfile.Entry) {
	if items, ok := e.Sequence(); ok {
		for _, item := range items {
			r.label(item)
		}
		return
	}
	r.label(e)
}

func (r *reader) label(e yamlfile.Entry) {
	label, ok := e.Scalar()
	if !ok {
		return
	}
	switch {
	case strings.Contains(label, "${{"):
		r.report(e, oneLine(label), "takes its runner from an expression")
	case !hosted(label):
		// A self-hosted or custom label names a machine, not a version, so
		// there was never a date to look for.
	case strings.HasSuffix(label, "-latest"):
		r.report(e, label, "names latest, not a version")
	default:
		r.ds = append(r.ds, decl.Decl{Product: Runners, Version: catalogLabel(label), Source: r.at(e)})
	}
}

// step reads the version a setup-* action is told to install. The action name
// decides the runtime; anything else with a with: block is left alone.
func (r *reader) step(usesEntry, withEntry yamlfile.Entry) {
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
	for _, e := range with {
		if e.Key != setup.Input {
			continue
		}
		raw, ok := e.Scalar()
		if !ok {
			continue
		}
		// A version input may be a block of them, one per line, which is how
		// a workflow installs several at once.
		for line := range strings.SplitSeq(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if v, reason := version(line); reason != "" {
				r.report(e, oneLine(line), reason)
			} else {
				r.ds = append(r.ds, decl.Decl{Product: setup.Product, Version: v, Source: r.at(e)})
			}
		}
	}
}

// version turns a setup-* version input into a version, or says why it is
// not one. A trailing wildcard segment is dropped, since 20.x and 1.21.x name
// the cycle and leave the patch to the action.
func version(s string) (v, reason string) {
	if strings.Contains(s, "${{") {
		return "", "takes its version from an expression"
	}
	v = strings.TrimPrefix(s, "v")
	for {
		base, last, found := cutLast(v)
		if !found || (last != "x" && last != "*") {
			break
		}
		v = base
	}
	if v == "" || v[0] < '0' || v[0] > '9' {
		return "", "is not a version"
	}
	return v, ""
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
