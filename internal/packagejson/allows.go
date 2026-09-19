package packagejson

import (
	"regexp"
	"slices"
	"strings"

	"github.com/iwamot/eolwhen/internal/span"
)

// unread are the shapes of requirement this does not read. A union leaves
// two ranges behind rather than one, and a range written as two ends with a
// dash is npm's own spelling rather than one of the operators below. A colon
// or a slash means the requirement is not a version range at all: an alias,
// a workspace, a file, a repository, a tarball.
var unread = []string{"||", " - ", ":", "/"}

// operatorSpace is an operator with the version pushed away from it, which
// npm allows and which would otherwise read as two comparators.
var operatorSpace = regexp.MustCompile(`(\^|~|>=|<=|>|<|=)\s+`)

// wildcards are the ways npm writes a segment that is free to be anything.
var wildcards = []string{"x", "X", "*"}

// allows reads what a requirement lets a package be: the lowest version it
// admits, and the first version past it.
//
// ok is false when the requirement leaves the upper end open, or is one of
// the shapes above, or is a dist-tag rather than a version at all. There is
// no range to place then, and no version either: a package allowed to be
// anything from 16 upwards will be whatever the resolver picked, which is
// written in the lockfile and not here.
func allows(requirement string) (span.Span, bool) {
	for _, u := range unread {
		if strings.Contains(requirement, u) {
			return span.Span{}, false
		}
	}
	// Space is what separates one comparator from the next, and every one of
	// them has to hold at once — except where it only pushes an operator
	// away from its version, which would otherwise read as a comparator of
	// its own.
	joined := operatorSpace.ReplaceAllString(strings.Join(strings.Fields(requirement), " "), "$1")
	fields := strings.Fields(joined)
	if len(fields) == 0 {
		return span.Span{}, false
	}
	var s span.Span
	for _, field := range fields {
		op, v := split(field)
		if line, wild := trimWildcard(v); wild {
			// A wildcard segment names a line of versions rather than a
			// version, which is a range already: 16.x runs from 16 to 17.
			// An operator in front of one is not npm, and a requirement
			// that is nothing but a wildcard is every version there is and
			// narrows nothing.
			if op != "" || !span.Numeric(line) {
				return span.Span{}, false
			}
			ceiling, _ := span.Next(line)
			s = s.AtLeast(line).Under(ceiling)
			continue
		}
		// Everything below is arithmetic on a version, so a requirement
		// that is not one — a dist-tag, a pre-release — is given up on here
		// rather than in each branch.
		if !span.Numeric(v) {
			return span.Span{}, false
		}
		switch op {
		case "~":
			ceiling, _ := span.AfterMinor(v)
			s = s.AtLeast(v).Under(ceiling)
		case "^":
			ceiling, _ := span.AfterCompatible(v)
			s = s.AtLeast(v).Under(ceiling)
		case ">":
			// A version npm leaves unfinished names a line rather than a
			// version, and a `>` in front of one excludes the whole line:
			// >18 admits nothing below 19, and >18.1 nothing below 18.2.
			// Written out in full it excludes one version, which is the
			// single version Narrow reads as no difference at all.
			floor := v
			if strings.Count(v, ".") < 2 {
				floor, _ = span.Next(v)
			}
			s = s.AtLeast(floor)
		default:
			// Every operator split leaves here is one Narrow reads, and the
			// version it is given has already been found to be numbers, so
			// there is no way for it to refuse.
			s, _ = s.Narrow(op, v)
		}
	}
	if !s.Closed() {
		return span.Span{}, false
	}
	return s, true
}

// trimWildcard takes the wildcard segments off the end of a version: 1.x.x
// names the same line as 1.x, and both are every 1. wild is false when the
// version ends in a number, which is the usual case and no line at all.
func trimWildcard(v string) (line string, wild bool) {
	for {
		i := strings.LastIndex(v, ".")
		if i < 0 {
			break
		}
		if !slices.Contains(wildcards, v[i+1:]) {
			break
		}
		v, wild = v[:i], true
	}
	if slices.Contains(wildcards, v) {
		// Nothing but a wildcard: there is no line left to name.
		return "", true
	}
	return v, wild
}

// split reads one comparator as its operator and its version. A version may
// carry the v npm allows in front of it, which is not part of the number.
func split(field string) (op, v string) {
	for _, o := range []string{">=", "<=", "^", "~", ">", "<", "="} {
		if rest, found := strings.CutPrefix(field, o); found {
			return o, strings.TrimPrefix(rest, "v")
		}
	}
	return "", strings.TrimPrefix(field, "v")
}
