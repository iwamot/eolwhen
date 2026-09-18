package pythonmanifest

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/span"
)

// requirement is one line of a manifest read apart: the package it names and
// what it asks of it. ok is false when the line is not a requirement on a
// named package at all.
//
// The shape is PEP 508, which writes a name, then extras, then a version
// specifier, then a marker. Only the first and the third say anything about
// which version is running. Extras name parts of the same package; a marker
// says when the requirement applies and not to what.
func requirement(line string) (name, specifier string, ok bool) {
	// A marker follows a semicolon, and a direct reference an at sign. The
	// reference names a URL to install from, which says where a version
	// came from and not which one it is.
	line, _, _ = strings.Cut(line, ";")
	if before, _, found := strings.Cut(line, "@"); found {
		line = before
	}
	line = strings.TrimSpace(line)
	// The name runs up to whatever is not part of one, and extras sit in
	// brackets between the name and the specifier.
	i := strings.IndexFunc(line, func(r rune) bool { return !nameRune(r) })
	if line == "" || i == 0 {
		return "", "", false
	}
	if i < 0 {
		return line, "", true
	}
	name, rest := line[:i], strings.TrimSpace(line[i:])
	if after, found := strings.CutPrefix(rest, "["); found {
		// Extras name parts of the same package rather than a version of
		// it, and a bracket nothing closes is not a requirement to read.
		_, tail, closed := strings.Cut(after, "]")
		if !closed {
			return "", "", false
		}
		rest = strings.TrimSpace(tail)
	}
	return name, rest, true
}

// nameRune reports whether a rune may sit in a package name, which PEP 508
// writes with letters, digits and the three separators PyPI treats alike.
func nameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '-', r == '_', r == '.':
		return true
	}
	return false
}

// allows reads what a version specifier lets a package be: the lowest
// version it admits, and the first version past it.
//
// ok is false when the specifier leaves the upper end open, names no version
// at all, or uses an operator that does not narrow a range to one stretch.
// There is no range to place then, and no version either: a package allowed
// to be anything from 4.2 upwards will be whatever the resolver picked,
// which is written in the lockfile and not here.
func allows(specifier string) (span.Span, bool) {
	var s span.Span
	for clause := range strings.SplitSeq(specifier, ",") {
		op, v := split(strings.TrimSpace(clause))
		// PEP 440 gives every clause an operator, so a version standing on
		// its own is not a specifier this should be reading.
		if op == "" {
			return span.Span{}, false
		}
		if line, wild := strings.CutSuffix(v, ".*"); wild {
			// A wildcard names a line of versions rather than a version,
			// which is a range already: ==4.2.* runs from 4.2 to 4.3. Only
			// equality may carry one, which is PEP 440's own rule.
			if op != "==" || !span.Numeric(line) {
				return span.Span{}, false
			}
			ceiling, _ := span.Next(line)
			s = s.AtLeast(line).Under(ceiling)
			continue
		}
		// Everything below is arithmetic on a version, so a specifier that
		// is not one — a pre-release, a local version — is given up on here
		// rather than in each branch.
		if !span.Numeric(v) {
			return span.Span{}, false
		}
		if op == "~=" {
			// A compatible release holds everything but the last segment
			// given: ~=4.2.0 is every 4.2.x and ~=4.2 every 4.x.
			ceiling, _ := span.AfterLine(v)
			s = s.AtLeast(v).Under(ceiling)
			continue
		}
		narrowed, ok := s.Narrow(comparison(op), v)
		if !ok {
			return span.Span{}, false
		}
		s = narrowed
	}
	if !s.Closed() {
		return span.Span{}, false
	}
	return s, true
}

// comparison spells an operator the way Narrow reads it, which differs for
// equality alone: both of Python's manifests write == where the comparisons
// every manifest shares write =. The rest keep their spelling, and the ones
// that name no one stretch of versions — an exclusion, an arbitrary equality
// — are refused there.
func comparison(op string) string {
	if op == "==" {
		return "="
	}
	return op
}

// split reads one clause as its operator and its version. A version may
// carry the v PEP 440 allows in front of it, which is not part of the
// number.
func split(clause string) (op, v string) {
	for _, o := range []string{"===", "==", "!=", ">=", "<=", "~=", ">", "<"} {
		if rest, found := strings.CutPrefix(clause, o); found {
			return o, strings.TrimPrefix(strings.TrimSpace(rest), "v")
		}
	}
	return "", strings.TrimPrefix(clause, "v")
}
