// Package dockerfile reads the FROM instructions of a Dockerfile. What an
// image reference names is the image package's answer; this one finds the
// references and keeps track of the stages.
package dockerfile

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/image"
)

// Matches reports whether a file name is a Dockerfile. Both the plain name
// and the suffixed variants that hold a second build — Dockerfile.dev,
// Dockerfile.ci — are read.
func Matches(name string) bool {
	return name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.")
}

// Extract reads every FROM in the file.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	// A later stage may build on an earlier one by name. Those names are not
	// images, so they are collected as they are declared and skipped when
	// they come back around.
	stages := map[string]bool{}
	// The build arguments a FROM may be written in terms of, which are the
	// ones declared above the first of them.
	args := map[string]string{}
	built := false
	for _, in := range instructions(data) {
		ref, stage, ok := from(in.text)
		if !ok {
			if !built {
				declareArgs(in.text, args)
			}
			continue
		}
		// An expansion that leaves nothing at all is a reference the file
		// does not have: the argument carrying the whole image is given on
		// the command line. The reference as written says that better than
		// the empty string it works out to, so it is what gets reported.
		if expanded := expand(ref, args); expanded != "" {
			ref = expanded
		}
		built = true
		if stage != "" {
			stages[strings.ToLower(stage)] = true
		}
		if stages[strings.ToLower(ref)] || image.Skip(ref) {
			continue
		}
		src := decl.Source{File: file, Line: in.line}
		ns, reason, moving := image.Read(ref)
		if reason != "" {
			us = append(us, decl.Unreadable{Source: src, Text: ref, Reason: reason, Moving: moving})
			continue
		}
		for _, n := range ns {
			ds = append(ds, decl.Decl{Product: n.Product, Version: n.Version, Source: src})
		}
	}
	return ds, us
}

// declareArgs records the build arguments an ARG instruction declares, with
// the default each one was given.
//
// An ARG with no default is recorded as empty, because empty is what a
// `docker build` with no --build-arg puts there: `FROM ${REGISTRY}python:3.12`
// under a bare `ARG REGISTRY` builds from the official python image, and
// that is the build the file describes. A name no ARG declares is a value
// that only comes from outside, so it is left as it was written and the
// reference says it could not be read.
func declareArgs(text string, into map[string]string) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "ARG") {
		return
	}
	for _, field := range fields[1:] {
		name, value, _ := strings.Cut(field, "=")
		if name == "" {
			continue
		}
		// An argument may be written in terms of the ones above it.
		into[name] = expand(unquote(value), into)
	}
}

// unquote takes off the quotes a default may be written in. They are the
// shell's punctuation rather than part of the value.
func unquote(s string) string {
	for _, q := range []string{`"`, `'`} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// expand substitutes the build arguments a reference is written in terms of,
// in the two spellings a Dockerfile uses: $NAME and ${NAME}. A name that was
// never declared is left as it was, so that what the reference could not say
// still reads as a variable rather than as a reference somebody wrote.
func expand(ref string, args map[string]string) string {
	var b strings.Builder
	for i := 0; i < len(ref); i++ {
		if ref[i] != '$' {
			b.WriteByte(ref[i])
			continue
		}
		name, end, ok := argRef(ref, i)
		if !ok {
			b.WriteByte(ref[i])
			continue
		}
		value, declared := args[name]
		if !declared {
			b.WriteString(ref[i:end])
		} else {
			b.WriteString(value)
		}
		i = end - 1
	}
	return b.String()
}

// argRef reads the reference to a build argument that starts at i, and
// returns the name and the index just past it. A ${...} holding anything but
// a name — a ${NAME:-default}, a modifier — is not one this reads, so it
// comes back as it was.
func argRef(ref string, i int) (name string, end int, ok bool) {
	rest := ref[i+1:]
	if strings.HasPrefix(rest, "{") {
		j := strings.Index(rest, "}")
		if j < 0 {
			return "", 0, false
		}
		name = rest[1:j]
		end = i + 1 + j + 1
	} else {
		n := 0
		for n < len(rest) && (rest[n] == '_' || isAlnum(rest[n])) {
			n++
		}
		name, end = rest[:n], i+1+n
	}
	if !argName(name) {
		return "", 0, false
	}
	return name, end, true
}

// argName reports whether s is spelled the way a build argument is: a letter
// or an underscore, then letters, digits and underscores.
func argName(s string) bool {
	if s == "" || (!isAlpha(s[0]) && s[0] != '_') {
		return false
	}
	for i := range len(s) {
		if !isAlnum(s[i]) && s[i] != '_' {
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isAlnum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}

// from picks the image reference and the stage name out of a FROM
// instruction, or reports that the instruction is not one. The --platform
// flag is dropped: it says where the image runs, not which image it is.
func from(text string) (ref, stage string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
		return "", "", false
	}
	fields = fields[1:]
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return "", "", false
	}
	ref = fields[0]
	if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
		stage = fields[2]
	}
	return ref, stage, true
}
