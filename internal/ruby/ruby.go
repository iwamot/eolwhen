// Package ruby reads a Ruby version the way rbenv's .ruby-version and
// ruby/setup-ruby write one: the implementation's name, a dash, and its
// version, with the name left out for the reference implementation. 3.3 and
// ruby-3.3 are the same Ruby, and jruby-9.4 is not Ruby 9.4 but JRuby, which
// has a calendar of its own.
package ruby

import (
	"slices"
	"strings"
)

// engines are the implementations setup-ruby installs, spelled the way it
// and ruby-build spell them, each with the words that name a build of its
// development branch rather than a release. The list is the action's own
// closed vocabulary, which is what makes reading a name off the front of a
// version a reading rather than a guess; a word outside it is left for the
// caller to report.
//
// Each name is handed to the catalog as written. endoflife.date tracks JRuby
// and not TruffleRuby, and whether it tracks one is its answer to give;
// truffleruby+graalvm in particular is a build of TruffleRuby, not the
// GraalVM the catalog has a product for.
var engines = map[string][]string{
	"ruby":                {"head", "debug", "asan", "mingw", "mswin", "ucrt"},
	"jruby":               {"head"},
	"truffleruby":         {"head"},
	"truffleruby+graalvm": {"head"},
}

// Read splits a version into the implementation it is of and what was
// written for that implementation's version. reason is empty when the
// version is left for the caller to read, and otherwise says why there is
// no version to read: a development build and an implementation named on
// its own both follow the newest there is on purpose.
func Read(s string) (engine, version, reason string) {
	if slices.Contains(engines["ruby"], s) {
		return "ruby", s, "names a development build, not a version"
	}
	if _, ok := engines[s]; ok {
		return s, "", "names the newest stable release, not a version"
	}
	for name := range engines {
		rest, ok := strings.CutPrefix(s, name+"-")
		if !ok {
			continue
		}
		if slices.Contains(engines[name], rest) {
			return name, rest, "names a development build, not a version"
		}
		return name, rest, ""
	}
	return "ruby", s, ""
}
