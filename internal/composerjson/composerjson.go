// Package composerjson reads the packages a composer.json requires.
//
// What a package name reaches is settled elsewhere. This package says only
// that the line named a Composer package, and the catalog answers through
// the purls upstream publishes: `laravel/framework` is Laravel because
// endoflife.date says pkg:composer/laravel/framework, while the rest of a
// manifest is software it does not track.
//
// One name is not a package at all. Composer reserves `php` for the version
// of PHP the project runs on, and that is a runtime declaration like the one
// in a .python-version: the key can only mean the language, because Composer
// says so rather than because the name reads that way. Its other reserved
// names — ext-, lib-, hhvm, composer itself — say which extensions and
// which Composer a project needs, none of which is software with a release
// calendar of its own, so they are passed over.
package composerjson

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/jsonfile"
)

// Ecosystem is the purl type a Composer package name belongs to.
const Ecosystem = "composer"

// Matches reports whether a file is a composer.json.
func Matches(name string) bool { return name == "composer.json" }

// required are the members whose object holds the packages a project
// declares. What it needs to develop is declared as plainly as what it needs
// to run, and a development framework goes out of support the same way.
var required = []string{"require", "require-dev"}

// Extract reads every package a composer.json requires.
//
// A file that does not parse is skipped in silence, as a Compose file that
// is not YAML yet is: the tool that owns it reports that better than this
// one can, and a file mid-edit is not a declaration that could not be read.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, m := range jsonfile.Read(data, required, nil) {
		// Composer requires a package to be named in lower case, and a purl
		// spells it the same way, so that is the name to look up.
		name := strings.ToLower(m.Name)
		if platform(name) {
			continue
		}
		src := decl.Source{File: file, Line: m.Line}
		// The one name Composer reserves that does name software is the
		// language itself, and the catalog answers to that by name rather
		// than through a purl, as it does for any other runtime.
		ecosystem := Ecosystem
		if name == "php" {
			ecosystem = ""
		}
		allowed, ok := allows(m.Value)
		if !ok {
			us = append(us, decl.Unreadable{
				Source:    src,
				Ecosystem: ecosystem,
				Product:   name,
				Text:      m.Value,
				Reason:    reason(ecosystem == ""),
				Moving:    true,
			})
			continue
		}
		ds = append(ds, decl.Decl{
			Ecosystem: ecosystem,
			Product:   name,
			Version:   m.Value,
			Allows:    allowed,
			Source:    src,
		})
	}
	return ds, us
}

// reason says why a requirement naming no one version was set aside, which
// differs for the one that is not on a package. Which PHP a project runs on
// is whatever the machine has, not what a composer.lock resolved, and a
// range there is the project saying what it will put up with.
func reason(runtime bool) string {
	if runtime {
		return "names no single version here, so whatever the host has decides which one"
	}
	return "names no single version here, so the lockfile decides which one"
}

// platform reports whether a name is one of Composer's reserved names for
// something other than a package: an extension, a library the runtime was
// built against, Composer itself. `php` is reserved too and is not among
// these, being the one that names software with an end-of-life date.
func platform(name string) bool {
	// A package is written vendor/name and a reserved name never is, which
	// is what keeps composer/semver — a package like any other — apart from
	// composer, the tool.
	if strings.Contains(name, "/") {
		return false
	}
	switch {
	case strings.HasPrefix(name, "ext-"), strings.HasPrefix(name, "lib-"):
		return true
	case strings.HasPrefix(name, "composer"):
		return true
	case name == "hhvm":
		return true
	// php-64bit and its kind ask for how PHP was built rather than for a
	// version of it, and the version they carry is the same one php already
	// declares.
	case strings.HasPrefix(name, "php-"):
		return true
	}
	return false
}
