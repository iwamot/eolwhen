package resolve

import (
	"testing"
	"time"

	"github.com/iwamot/eolwhen/internal/catalog"
	"github.com/iwamot/eolwhen/internal/decl"
)

const doc = `{"result":[
  {"name":"python","aliases":[],"releases":[
    {"name":"3.10","eolFrom":"2026-10-04"},
    {"name":"3.1","eolFrom":"2012-04-10"},
    {"name":"2.7","eolFrom":"2020-01-01"}
  ]},
  {"name":"redis","aliases":[],"releases":[
    {"name":"8.0","eolFrom":null},
    {"name":"7.4","eolFrom":"2027-01-01"},
    {"name":"7.2","eolFrom":"2026-01-01"}
  ]},
  {"name":"runners","aliases":[],"releases":[
    {"name":"macos-15","eolFrom":"2028-01-01"},
    {"name":"windows-2025","eolFrom":"2029-01-01"}
  ]},
  {"name":"nodejs","aliases":["node"],"releases":[
    {"name":"26","eolFrom":null},
    {"name":"14","eolFrom":"2023-04-30"}
  ]}
]}`

func load(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Decode([]byte(doc))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return c
}

func src(file string) decl.Source { return decl.Source{File: file} }

func TestAllPlacesOnTheTimeline(t *testing.T) {
	c := load(t)
	r := All(c, []decl.Decl{
		{Product: "python", Version: "2.7.18", Source: src(".python-version")},
		{Product: "node", Version: "14.19.0", Source: src(".nvmrc")}, // reached by alias
	}, nil)
	if len(r.Unreadable) != 0 {
		t.Fatalf("Unreadable = %+v; want none", r.Unreadable)
	}
	if len(r.Findings) != 2 {
		t.Fatalf("Findings = %+v; want 2", r.Findings)
	}
	if r.Findings[0].Product != "python" || r.Findings[0].Cycle != "2.7" {
		t.Errorf("Findings[0] = %+v; want python 2.7", r.Findings[0])
	}
	// The alias resolves to the catalog's own name, which is what gets
	// printed.
	if r.Findings[1].Product != "nodejs" || r.Findings[1].Cycle != "14" {
		t.Errorf("Findings[1] = %+v; want nodejs 14", r.Findings[1])
	}
	if got := r.Findings[0].EOL.Format(time.DateOnly); got != "2020-01-01" {
		t.Errorf("EOL = %s; want 2020-01-01", got)
	}
}

// TestAllSegmentMatch is the case a string prefix gets wrong: 3.10 must not
// land in the 3.1 cycle, which expired in 2012.
func TestAllSegmentMatch(t *testing.T) {
	r := All(load(t), []decl.Decl{{Product: "python", Version: "3.10.2", Source: src(".python-version")}}, nil)
	if len(r.Findings) != 1 || r.Findings[0].Cycle != "3.10" {
		t.Fatalf("Findings = %+v; want cycle 3.10", r.Findings)
	}
}

func TestAllReports(t *testing.T) {
	for _, tt := range []struct {
		name   string
		d      decl.Decl
		reason string
	}{
		{"a version the catalog never had", decl.Decl{Product: "python", Version: "4.0.1", Source: src("x")},
			"no release cycle covers this version"},
		// A label of a product that names its cycles with words is not a
		// variant; it is a runner that is simply not offered any more.
		{"a retired runner label", decl.Decl{Product: "runners", Version: "windows-2019", Source: src("x")},
			"no release cycle covers this version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := All(load(t), []decl.Decl{tt.d}, nil)
			if len(r.Findings) != 0 {
				t.Fatalf("Findings = %+v; want none", r.Findings)
			}
			if len(r.Unreadable) != 1 || r.Unreadable[0].Reason != tt.reason {
				t.Fatalf("Unreadable = %+v; want reason %q", r.Unreadable, tt.reason)
			}
		})
	}
}

// TestAllSetsAsideMoving covers the lines that name no fixed version by
// design. A major line is a rule for following the newest cycle under it and
// a variant is a build of whatever the current release is, so neither has a
// date of its own and neither is a line to go and change. Reporting them is
// what filled the output with complaints nobody could act on, so they are
// set aside for --verbose like the rest.
func TestAllSetsAsideMoving(t *testing.T) {
	for _, tt := range []struct {
		name   string
		d      decl.Decl
		reason string
	}{
		{"a major line over several cycles", decl.Decl{Product: "redis", Version: "7", Source: src("x")},
			"names a major line rather than a release cycle, so it follows the newest in that line"},
		{"a major line over exactly one", decl.Decl{Product: "redis", Version: "8", Source: src("x")},
			"names a major line rather than a release cycle, so it follows the newest in that line"},
		{"a major line over many", decl.Decl{Product: "python", Version: "3", Source: src("x")},
			"names a major line rather than a release cycle, so it follows the newest in that line"},
		{"a variant", decl.Decl{Product: "redis", Version: "alpine", Source: src("x")},
			"names a variant or an alias, not a version"},
		{"an alias", decl.Decl{Product: "python", Version: "latest-slim", Source: src("x")},
			"names a variant or an alias, not a version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := All(load(t), []decl.Decl{tt.d}, nil)
			if len(r.Findings) != 0 || len(r.Unreadable) != 0 {
				t.Fatalf("All = %+v; want nothing placed and nothing reported", r)
			}
			if len(r.Moving) != 1 || r.Moving[0].Reason != tt.reason {
				t.Fatalf("Moving = %+v; want reason %q", r.Moving, tt.reason)
			}
		})
	}
}

// TestAllKeepsMovingApart: a line an extractor already knew was moving —
// :latest, ubuntu-latest, a tool pinned to stable — is set aside on the same
// grounds, and only the ones about software the catalog does not track go on
// being counted as untracked.
func TestAllKeepsMovingApart(t *testing.T) {
	label := decl.Unreadable{Source: src("ci.yml"), Text: "ubuntu-latest", Reason: "names latest, not a version", Moving: true}
	tool := decl.Unreadable{Source: src("mise.toml"), Product: "node", Text: "latest", Reason: "names a moving target, not a version", Moving: true}
	untracked := decl.Unreadable{Source: src("mise.toml"), Product: "jq", Text: "latest", Reason: "names a moving target, not a version", Moving: true}
	r := All(load(t), nil, []decl.Unreadable{label, tool, untracked})
	if len(r.Unreadable) != 0 {
		t.Fatalf("Unreadable = %+v; want none", r.Unreadable)
	}
	if len(r.Moving) != 2 || r.Moving[0] != label || r.Moving[1] != tool {
		t.Errorf("Moving = %+v; want the label and the tool", r.Moving)
	}
	if len(r.Untracked) != 1 || r.Untracked[0].Product != "jq" {
		t.Errorf("Untracked = %+v; want jq alone", r.Untracked)
	}
}

// TestAllSetsAsideUndated covers the cycle endoflife.date has not given an
// end date: the line is right and the cycle is there, so there is nothing to
// report and nothing to do. It is kept so --verbose can account for it.
func TestAllSetsAsideUndated(t *testing.T) {
	r := All(load(t), []decl.Decl{{Product: "nodejs", Version: "26.1.0", Source: src("x")}}, nil)
	if len(r.Findings) != 0 || len(r.Unreadable) != 0 || len(r.Untracked) != 0 {
		t.Fatalf("All = %+v; want nothing placed and nothing reported", r)
	}
	if len(r.Undated) != 1 || r.Undated[0].Product != "nodejs" || r.Undated[0].Cycle != "26" {
		t.Errorf("Undated = %+v; want nodejs 26", r.Undated)
	}
}

func TestAllNothing(t *testing.T) {
	r := All(load(t), nil, nil)
	if len(r.Findings) != 0 || len(r.Unreadable) != 0 {
		t.Errorf("All(nil) = %+v; want nothing", r)
	}
}

// TestAllSetsAsideUntracked covers the names a tool list carries that have
// no end-of-life policy to look up. They are not reported, because there was
// never a date to find; only the ones the catalog knows reach the output.
// They are kept, though, so --verbose can account for them.
func TestAllSetsAsideUntracked(t *testing.T) {
	r := All(load(t), []decl.Decl{
		{Product: "biome", Version: "2.5.13", Source: src("mise.toml")},
		{Product: "python", Version: "2.7.18", Source: src("mise.toml")},
	}, nil)
	if len(r.Unreadable) != 0 {
		t.Fatalf("Unreadable = %+v; want none", r.Unreadable)
	}
	if len(r.Findings) != 1 || r.Findings[0].Product != "python" {
		t.Errorf("Findings = %+v; want python alone", r.Findings)
	}
	if len(r.Untracked) != 1 || r.Untracked[0].Product != "biome" {
		t.Errorf("Untracked = %+v; want biome alone", r.Untracked)
	}
}

// TestAllByCodename covers `FROM debian:bookworm`, where the tag names a
// cycle without giving a number.
func TestAllByCodename(t *testing.T) {
	c, err := catalog.Decode([]byte(`{"result":[
	  {"name":"debian","aliases":[],"releases":[
	    {"name":"10","codename":"Buster","eolFrom":"2024-06-30"}
	  ]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	r := All(c, []decl.Decl{{Product: "debian", Version: "buster", Source: src("Dockerfile")}}, nil)
	if len(r.Unreadable) != 0 {
		t.Fatalf("Unreadable = %+v; want none", r.Unreadable)
	}
	if len(r.Findings) != 1 || r.Findings[0].Cycle != "10" {
		t.Fatalf("Findings = %+v; want cycle 10", r.Findings)
	}
}

// TestAllBaseCodename covers the half of an image tag that names its
// distribution: a declaration with no product, whose version is the word
// itself. The catalog says which words those are, and a word it has no
// codename for — the slim in python:3.12-slim — was a build variant and
// never a declaration, so it is dropped without a word of its own.
func TestAllBaseCodename(t *testing.T) {
	c, err := catalog.Decode([]byte(`{"result":[
	  {"name":"debian","aliases":[],"releases":[
	    {"name":"11","codename":"Bullseye","eolFrom":"2026-08-31"}
	  ]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	r := All(c, []decl.Decl{
		{Version: "bullseye", Source: src("Dockerfile")},
		{Version: "slim", Source: src("Dockerfile")},
	}, nil)
	if len(r.Unreadable) != 0 || len(r.Moving) != 0 || len(r.Untracked) != 0 {
		t.Fatalf("All = %+v; want nothing reported and nothing set aside", r)
	}
	if len(r.Findings) != 1 || r.Findings[0].Product != "debian" || r.Findings[0].Cycle != "11" {
		t.Fatalf("Findings = %+v; want debian 11", r.Findings)
	}
}

// TestAllSortsUnreadable: a line an extractor could not read is still a line
// about some software, and the same rule applies to it. One about software
// the catalog does not track is set aside, whatever its version looked like;
// one about tracked software is carried through ahead of anything resolution
// had to say; one that names no software apart from its text is carried
// through as it is.
func TestAllSortsUnreadable(t *testing.T) {
	untracked := decl.Unreadable{Source: src("mise.toml"), Product: "jq", Text: "latest", Reason: "names a moving target, not a version"}
	tracked := decl.Unreadable{Source: src("mise.toml"), Product: "node", Text: "latest", Reason: "names a moving target, not a version"}
	label := decl.Unreadable{Source: src("ci.yml"), Text: "ubuntu-latest", Reason: "names latest, not a version"}
	r := All(load(t),
		[]decl.Decl{{Product: "python", Version: "4.0.1", Source: src("x")}},
		[]decl.Unreadable{untracked, tracked, label})
	if len(r.Unreadable) != 3 || r.Unreadable[0] != tracked || r.Unreadable[1] != label || r.Unreadable[2].Product != "python" {
		t.Errorf("Unreadable = %+v; want node latest, ubuntu-latest, then python 4.0.1", r.Unreadable)
	}
	if len(r.Untracked) != 1 || r.Untracked[0] != (decl.Decl{Product: "jq", Version: "latest", Source: src("mise.toml")}) {
		t.Errorf("Untracked = %+v; want jq latest alone", r.Untracked)
	}
	// With nothing to look up, the catalog is never touched.
	r = All(nil, nil, []decl.Unreadable{label})
	if len(r.Unreadable) != 1 || len(r.Untracked) != 0 {
		t.Errorf("All(nil) = %+v; want the label alone", r)
	}
}

// TestAllByImage covers a name outside the official library. It reaches a
// product only when endoflife.date publishes that image for one, and a name
// nobody published is a declaration of software the catalog does not track:
// there was never a date to find, so it is set aside rather than reported.
func TestAllByImage(t *testing.T) {
	c, err := catalog.Decode([]byte(`{"result":[
	  {"name":"opensearch","aliases":[],"identifiers":[
	    {"type":"purl","id":"pkg:docker/opensearchproject/opensearch"}
	  ],"releases":[{"name":"1.3","eolFrom":"2023-03-17"}]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	r := All(c, []decl.Decl{
		{Product: "opensearchproject/opensearch", Version: "1.3.0", Source: src("compose.yml")},
		{Product: "acme/sandbox", Version: "0.2.10", Source: src("compose.yml")},
	}, nil)
	if len(r.Unreadable) != 0 {
		t.Fatalf("Unreadable = %+v; want none", r.Unreadable)
	}
	// The row names the product, not the image the file happened to write.
	if len(r.Findings) != 1 || r.Findings[0].Product != "opensearch" || r.Findings[0].Cycle != "1.3" {
		t.Fatalf("Findings = %+v; want opensearch 1.3", r.Findings)
	}
	if len(r.Untracked) != 1 || r.Untracked[0].Product != "acme/sandbox" {
		t.Errorf("Untracked = %+v; want the unpublished image alone", r.Untracked)
	}
}

// TestAllPredating covers the version older than anything endoflife.date
// tracks. A redis 3.2 is the most neglected line in a directory, and reading
// it as a version nobody has heard of is the one answer that is certainly
// wrong. It gets a row, carrying the day the oldest tracked cycle ended,
// which support for anything older had run out by.
func TestAllPredating(t *testing.T) {
	r := All(load(t), []decl.Decl{{Product: "redis", Version: "3.2", Source: src("compose.yml")}}, nil)
	if len(r.Unreadable) != 0 || len(r.Moving) != 0 {
		t.Fatalf("All = %+v; want nothing reported", r)
	}
	if len(r.Findings) != 1 || r.Findings[0].Cycle != "<7.2" {
		t.Fatalf("Findings = %+v; want cycle <7.2", r.Findings)
	}
	if got := r.Findings[0].EOL.Format(time.DateOnly); got != "2026-01-01" {
		t.Errorf("EOL = %s; want the oldest cycle's own date", got)
	}
}

// TestAllPredatingNeedsADate: a product whose cycles are words has no
// ordering of this kind, and one whose oldest cycle has no end date
// announced has no day to carry over. Both are reported as they were.
func TestAllPredatingNeedsADate(t *testing.T) {
	c, err := catalog.Decode([]byte(`{"result":[
	  {"name":"runners","aliases":[],"releases":[{"name":"macos-15","eolFrom":"2028-01-01"}]},
	  {"name":"new","aliases":[],"releases":[{"name":"2.0","eolFrom":null}]}
	]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	r := All(c, []decl.Decl{
		{Product: "runners", Version: "macos-11", Source: src("ci.yml")},
		{Product: "new", Version: "1.0", Source: src("x")},
	}, nil)
	if len(r.Findings) != 0 {
		t.Fatalf("Findings = %+v; want none", r.Findings)
	}
	if len(r.Unreadable) != 2 {
		t.Fatalf("Unreadable = %+v; want both reported", r.Unreadable)
	}
}
