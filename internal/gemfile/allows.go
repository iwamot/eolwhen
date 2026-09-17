package gemfile

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/span"
)

// allows reads what a gem's requirements let it be: the lowest version they
// admit, and the first version past them.
//
// ok is false when the requirements leave the upper end open, or name
// something this does not read. There is no range to place then, and no
// version either: a gem allowed to be anything from 6.0 upwards will be
// whatever the resolver picked, which is written in the lockfile and not
// here. An exclusion — != 6.1 — is the same answer from the other side, as
// it takes a version out of the middle and narrows neither end.
func allows(reqs []string) (span.Span, bool) {
	if len(reqs) == 0 {
		return span.Span{}, false
	}
	var s span.Span
	for _, req := range reqs {
		op, v := split(req)
		// Everything below is arithmetic on a version, so a requirement
		// that is not one is given up on here rather than in each branch.
		if !span.Numeric(v) {
			return span.Span{}, false
		}
		switch op {
		case "~>":
			ceiling, _ := span.AfterLine(v)
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

// split reads a requirement as its operator and its version. A requirement
// with no operator is an exact version, which is what a Gemfile written from
// a lockfile looks like.
func split(req string) (op, v string) {
	req = strings.TrimSpace(req)
	for _, o := range []string{"~>", ">=", "<=", "!=", ">", "<", "="} {
		if rest, found := strings.CutPrefix(req, o); found {
			return o, strings.TrimSpace(rest)
		}
	}
	return "", req
}
