// Package gemfile reads the gems a Gemfile declares.
//
// A Gemfile is Ruby, and this reads it as text rather than running it: a
// `gem` line with a literal name is the form nearly every Gemfile uses, and
// a name built at runtime is not read at all. A declaration spread over
// several lines is read as far as its first line goes, which names the gem
// and no version. The tool that owns the file reports a Gemfile that does
// not parse better than this one can.
//
// What a gem name reaches is settled elsewhere. This package says only that
// the line named a gem, and the catalog answers through the purls upstream
// publishes: `rails` is Ruby on Rails because endoflife.date says
// pkg:gem/rails, while `pg` reaches nothing, being the PostgreSQL driver and
// not PostgreSQL.
package gemfile

import (
	"regexp"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Ecosystem is the purl type a gem name belongs to.
const Ecosystem = "gem"

// Matches reports whether a file is a Gemfile. Bundler reads gems.rb as
// the same file under another name.
func Matches(name string) bool {
	return name == "Gemfile" || name == "gems.rb"
}

// gemLine is a `gem` call with a literal name: the name, and whatever the
// line carries after it.
var gemLine = regexp.MustCompile(`^\s*gem\s+['"]([^'"]+)['"]\s*(.*)$`)

// quoted is a requirement string following a comma, which is how a gem line
// carries more than one: gem "rails", ">= 6.0", "< 7".
var quoted = regexp.MustCompile(`^,\s*['"]([^'"]*)['"]\s*`)

// Extract reads every gem a Gemfile declares.
//
// A line whose requirements pin a range of versions is a declaration like
// any other: the range is carried through, and a cycle comes of it when the
// whole range sits inside one. A line with no requirements, or with ones
// this cannot read, names no version here — the lockfile holds the answer —
// so it is set aside rather than reported.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := gemLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		src := decl.Source{File: file, Line: i + 1}
		name, reqs := m[1], requirements(m[2])
		text := strings.Join(reqs, ", ")
		lo, hi, ok := span(reqs)
		if !ok {
			us = append(us, decl.Unreadable{
				Source:    src,
				Ecosystem: Ecosystem,
				Product:   name,
				Text:      text,
				Reason:    "names no single version here, so the lockfile decides which one",
				Moving:    true,
			})
			continue
		}
		ds = append(ds, decl.Decl{
			Ecosystem: Ecosystem,
			Product:   name,
			Version:   text,
			From:      lo,
			Below:     hi,
			Source:    src,
		})
	}
	return ds, us
}

// requirements reads the quoted strings that follow the gem's name, and
// stops at the first thing that is not one. What follows a gem line's
// requirements is keyword arguments — require:, group:, git: — and a
// comment, none of which say anything about the version.
func requirements(rest string) []string {
	var out []string
	for {
		m := quoted.FindStringSubmatch(rest)
		if m == nil {
			return out
		}
		out = append(out, strings.TrimSpace(m[1]))
		rest = rest[len(m[0]):]
	}
}
