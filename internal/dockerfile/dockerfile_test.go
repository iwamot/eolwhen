package dockerfile

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"Dockerfile", true},
		{"Dockerfile.dev", true},
		{"Dockerfile.ci", true},
		{"dockerfile", false},
		{"Dockerfile-dev", false},
		{"docker-compose.yml", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

func one(t *testing.T, body string) (decl.Decl, bool) {
	t.Helper()
	ds, us := Extract("Dockerfile", []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != 1 {
		t.Fatalf("declarations = %+v; want 1", ds)
	}
	return ds[0], true
}

// TestExtractCaught is the half that reaches the timeline: an official image
// with a tag that names a version.
func TestExtractCaught(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		product string
		version string
	}{
		{"plain", "FROM python:3.7\n", "python", "3.7"},
		{"a variant after the dash", "FROM python:3.9.10-slim-buster\n", "python", "3.9.10"},
		{"a two-part version", "FROM ubuntu:18.04\n", "ubuntu", "18.04"},
		{"a bare major", "FROM centos:7\n", "centos", "7"},
		{"the library form spelled out", "FROM library/node:20\n", "node", "20"},
		{"with Docker Hub's own host", "FROM docker.io/library/redis:6.2\n", "redis", "6.2"},
		{"a mirror of the official library", "FROM public.ecr.aws/docker/library/node:24.21.0-trixie-slim\n", "node", "24.21.0"},
		// A tag next to a digest is still the version the author wrote, and
		// the one an update would change.
		{"a tag pinned by digest", "FROM python:3.7-slim@sha256:0000000000000000000000000000000000000000000000000000000000000000\n", "python", "3.7"},
		{"a platform flag", "FROM --platform=linux/amd64 alpine:3.10\n", "alpine", "3.10"},
		{"a stage name after it", "FROM golang:1.16 AS builder\n", "golang", "1.16"},
		{"lowercase from", "from postgres:11\n", "postgres", "11"},
		// Comments above the first instruction are read for the parser
		// directives they may carry, and are otherwise nothing.
		{"a comment above it", "# the runtime we ship on\nFROM postgres:11\n", "postgres", "11"},
		{"a setting that is not a directive above it", "# owner=platform\nFROM postgres:11\n", "postgres", "11"},
		// A codename is left whole for the catalog to recognize.
		{"a codename", "FROM debian:bookworm-slim\n", "debian", "bookworm"},
		{"a codename with a date", "FROM ubuntu:jammy-20230624\n", "ubuntu", "jammy"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := one(t, tt.body)
			if d.Product != tt.product || d.Version != tt.version {
				t.Errorf("Extract = %s %s; want %s %s", d.Product, d.Version, tt.product, tt.version)
			}
		})
	}
}

// TestExtractReported is the half that is read but deliberately not matched.
// Each of these is a line somebody wrote about software this tool cannot
// identify from the text, so guessing is what would put a wrong date on the
// timeline.
func TestExtractReported(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   string
		reason string
	}{
		{"someone else's namespace", "FROM ghcr.io/acme/python:3.7\n",
			"is not a Docker official image, so its contents are not known here"},
		{"a Docker Hub user's image", "FROM acme/python:3.7\n",
			"is not a Docker official image, so its contents are not known here"},
		{"a private registry with a port", "FROM registry.corp:5000/base:1.2\n",
			"is not a Docker official image, so its contents are not known here"},
		{"pinned by digest", "FROM python@sha256:0000000000000000000000000000000000000000000000000000000000000000\n",
			"is pinned by digest, which does not say which version it is"},
		{"no tag", "FROM python\n", "names no tag, so it follows latest"},
		{"latest", "FROM python:latest\n", "names latest, not a version"},
		{"a build argument", "FROM python:${PYTHON_VERSION}\n", "takes its version from a variable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("Dockerfile", []byte(tt.body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			if len(us) != 1 || us[0].Reason != tt.reason {
				t.Fatalf("unreadable = %+v; want one with reason %q", us, tt.reason)
			}
		})
	}
}

// TestExtractStages: a later stage building on an earlier one names no
// image, so there is nothing to look up and nothing to report either.
func TestExtractStages(t *testing.T) {
	body := "FROM golang:1.16 AS builder\n" +
		"RUN go build ./...\n" +
		"FROM builder AS test\n" +
		"FROM alpine:3.10\n" +
		"COPY --from=builder /app /app\n"
	ds, us := Extract("Dockerfile", []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != 2 || ds[0].Product != "golang" || ds[1].Product != "alpine" {
		t.Fatalf("declarations = %+v; want golang then alpine", ds)
	}
	if ds[1].Source != (decl.Source{File: "Dockerfile", Line: 4}) {
		t.Errorf("Source = %v; want Dockerfile:4", ds[1].Source)
	}
}

func TestExtractNothing(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"empty", ""},
		{"no FROM", "RUN echo hi\nCOPY . .\n"},
		// scratch is the empty image, not a piece of software.
		{"scratch", "FROM scratch\n"},
		{"scratch with a stage", "FROM scratch AS base\n"},
		{"FROM with nothing after it", "FROM\n"},
		{"only flags after FROM", "FROM --platform=linux/amd64\n"},
		// A word starting with FROM is not the instruction.
		{"a near miss", "FROMAGE python:3.7\n"},
		{"directives and nothing else", "# syntax=docker/dockerfile:1\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("Dockerfile", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

// TestExtractContinuation: an instruction may be written across several
// lines, and the FROM in one of them is still a FROM. The line reported is
// where the instruction starts, which is where a reader looks for it.
func TestExtractContinuation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		product string
		version string
		line    int
	}{
		{"a flag before the break", "FROM --platform=linux/amd64 \\\n  python:2.7\n", "python", "2.7", 1},
		{"the reference on its own line", "FROM \\\n  node:20\n", "node", "20", 1},
		{"a stage name after the break", "FROM golang:1.16 \\\n  AS builder\n", "golang", "1.16", 1},
		// A comment or an empty line inside a continuation is dropped, and
		// the instruction goes on past it.
		{"a comment inside it", "FROM \\\n# which one\n  node:20\n", "node", "20", 1},
		{"an empty line inside it", "FROM \\\n\n  node:20\n", "node", "20", 1},
		// Nothing is inserted where the lines join, so a reference broken
		// mid-word comes back whole.
		{"a word broken in two", "FROM pyth\\\non:3.7\n", "python", "3.7", 1},
		{"after an instruction that was continued", "RUN echo one \\\n  && echo two\nFROM alpine:3.10\n", "alpine", "3.10", 3},
		// A Dockerfile may name a different escape character at the top,
		// which Windows builds do so that paths do not read as escapes.
		{"a backtick escape", "# escape=`\nFROM --platform=windows/amd64 `\n  python:2.7\n", "python", "2.7", 2},
		{"a directive above the escape one", "# syntax=docker/dockerfile:1\n# escape=`\nFROM `\n  python:2.7\n", "python", "2.7", 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := one(t, tt.body)
			if d.Product != tt.product || d.Version != tt.version {
				t.Errorf("Extract = %s %s; want %s %s", d.Product, d.Version, tt.product, tt.version)
			}
			if d.Source.Line != tt.line {
				t.Errorf("Source.Line = %d; want %d", d.Source.Line, tt.line)
			}
		})
	}
}

// TestExtractHeredoc: the body of a heredoc is a file being written or a
// script being run, so a line in it that reads like a FROM is a string. The
// image that is really there is the only one found.
func TestExtractHeredoc(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"a quoted word", "FROM python:3.13\nRUN cat > /tmp/example.txt <<'EOF'\nFROM python:2.7\nEOF\n"},
		{"a bare word", "FROM python:3.13\nRUN cat > /tmp/example.txt <<EOF\nFROM python:2.7\nEOF\n"},
		{"tabs stripped from the body", "FROM python:3.13\nRUN cat > /tmp/example.txt <<-EOF\n\tFROM python:2.7\n\tEOF\n"},
		{"a file written by COPY", "FROM python:3.13\nCOPY <<EOF /tmp/example.txt\nFROM python:2.7\nEOF\n"},
		{"two of them in one instruction", "FROM python:3.13\nRUN cat <<ONE > /a && cat <<TWO > /b\nFROM python:2.7\nONE\nFROM python:2.6\nTWO\n"},
		{"opened by an instruction that was continued", "FROM python:3.13\nRUN cat \\\n  > /tmp/example.txt <<EOF\nFROM python:2.7\nEOF\n"},
		{"carried by ONBUILD", "FROM python:3.13\nONBUILD RUN cat > /tmp/example.txt <<EOF\nFROM python:2.7\nEOF\n"},
		// A heredoc with no terminator takes the rest of the file, which is
		// what the builder makes of it too.
		{"never terminated", "FROM python:3.13\nRUN cat > /tmp/example.txt <<EOF\nFROM python:2.7\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := one(t, tt.body)
			if d.Product != "python" || d.Version != "3.13" {
				t.Errorf("Extract = %s %s; want python 3.13", d.Product, d.Version)
			}
		})
	}
}

// TestExtractHeredocOnlyWhereOneCanBe: only RUN, COPY and ADD take a
// heredoc. Elsewhere a << is part of the command being written, and the
// lines that follow are instructions like any other.
func TestExtractHeredocOnlyWhereOneCanBe(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"a << inside a quoted argument", "FROM python:3.13\nRUN echo '<<'\nFROM alpine:3.10\n"},
		{"an instruction that takes no heredoc", "FROM python:3.13\nLABEL note=<<EOF\nFROM alpine:3.10\n"},
		{"a << with no word after it", "FROM python:3.13\nRUN cat <<\nFROM alpine:3.10\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("Dockerfile", []byte(tt.body))
			if len(us) != 0 {
				t.Fatalf("unreadable = %+v; want none", us)
			}
			if len(ds) != 2 || ds[0].Product != "python" || ds[1].Product != "alpine" {
				t.Fatalf("declarations = %+v; want python then alpine", ds)
			}
		})
	}
}
