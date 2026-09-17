package composerjson

import (
	"regexp"
	"strings"

	"github.com/iwamot/eolwhen/internal/span"
)

// unread are the shapes of constraint this does not read. A union leaves
// two ranges behind rather than one; a range written as two ends with a
// dash is Composer's own spelling and not one of the operators below; and a
// stability flag or a branch admits versions that are not ordered by their
// numbers at all.
var unread = []string{"|", " - ", "@", "dev-"}

// operatorSpace is an operator with the version pushed away from it, which
// Composer allows and which would otherwise read as two requirements.
var operatorSpace = regexp.MustCompile(`(\^|~|>=|<=|!=|>|<|=)\s+`)

// allows reads what a constraint lets a package be: the lowest version it
// admits, and the first version past it.
//
// ok is false when the constraint leaves the upper end open, or is one of
// the shapes above. There is no range to place then, and no version either:
// a package allowed to be anything from 8.0 upwards will be whatever the
// resolver picked, which is written in the composer.lock and not here.
func allows(constraint string) (span.Span, bool) {
	for _, u := range unread {
		if strings.Contains(constraint, u) {
			return span.Span{}, false
		}
	}
	// An operator may be written away from its version, which would
	// otherwise read as a requirement of its own. What separates one
	// requirement from the next is a space or a comma, both meaning and.
	joined := operatorSpace.ReplaceAllString(strings.Join(strings.Fields(constraint), " "), "$1")
	fields := strings.FieldsFunc(joined, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return span.Span{}, false
	}
	var s span.Span
	for _, field := range fields {
		op, v := split(field)
		if line, wild := strings.CutSuffix(v, ".*"); wild {
			// A wildcard names a line of versions rather than a version,
			// which is a range already: 8.1.* runs from 8.1 to 8.2. An
			// operator in front of one is not Composer, so it is not read,
			// and a bare * is every version there is and narrows nothing.
			if op != "" || !span.Numeric(line) {
				return span.Span{}, false
			}
			ceiling, _ := span.Next(line)
			s = s.AtLeast(line).Under(ceiling)
			continue
		}
		// Everything below is arithmetic on a version, so a requirement
		// that is not one is given up on here rather than in each branch.
		if !span.Numeric(v) {
			return span.Span{}, false
		}
		switch op {
		case "~":
			ceiling, _ := span.AfterLine(v)
			s = s.AtLeast(v).Under(ceiling)
		case "^":
			ceiling, _ := span.AfterCompatible(v)
			s = s.AtLeast(v).Under(ceiling)
		default:
			narrowed, ok := s.Narrow(op, v)
			if !ok {
				return span.Span{}, false
			}
			s = narrowed
		}
	}
	if !s.Closed() {
		return span.Span{}, false
	}
	return s, true
}

// split reads one requirement as its operator and its version. A version may
// carry the v Composer allows in front of it, which is not part of the
// number.
func split(field string) (op, v string) {
	for _, o := range []string{">=", "<=", "!=", "^", "~", ">", "<", "="} {
		if rest, found := strings.CutPrefix(field, o); found {
			return o, strings.TrimPrefix(rest, "v")
		}
	}
	return "", strings.TrimPrefix(field, "v")
}
