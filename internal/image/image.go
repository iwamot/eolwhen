// Package image reads a container image reference.
//
// An official image name is the software itself — library/python is Python,
// library/postgres is PostgreSQL — because that namespace is a short curated
// list rather than something anyone can publish into. Any other name is
// handed on as written, for the catalog to recognize or not: endoflife.date
// publishes the Docker Hub repository each product ships under, so
// opensearchproject/opensearch is OpenSearch on upstream's word rather than
// on the strength of how the name reads. Nothing here guesses, and
// ghcr.io/acme/python is still not Python.
package image

import "strings"

// hubHosts are the registry hosts that serve Docker Hub. A name written
// under one of them follows Docker Hub's own rule that a name with no
// namespace means the official library, so docker.io/python is
// docker.io/library/python.
var hubHosts = []string{
	"docker.io/",
	"index.docker.io/",
	"registry-1.docker.io/",
}

// mirrorPrefixes are the registries that carry a copy of the official
// library under a path of their own. Each is listed because it was checked,
// not because it matched a pattern; a registry not on this list may put
// anything under any name, and only this exact path is the library.
var mirrorPrefixes = []string{
	"public.ecr.aws/docker/library/",
}

// Skip reports whether a reference is one there is nothing to look up for.
// scratch is the empty image rather than a piece of software, so no version
// was ever missing from it.
func Skip(ref string) bool {
	name, _, _ := split(ref)
	return name == "scratch"
}

// Named is one piece of software a reference names, with the version it
// names for it. An empty Product means the version names the software as
// well: a distribution codename does, and the catalog resolves it to both.
type Named struct {
	Product string
	Version string
}

// Unread is a reference that could not be read as a version, and why not.
//
// Product is the software the name says, when it says one, and Text is what
// was written where the version goes: python:latest is python and latest,
// and an untagged sonatype/nexus is that name and nothing. Keeping the two
// apart is what lets the catalog decide whether the software is tracked at
// all, since a line about software nobody publishes a calendar for is set
// aside whatever its tag says. A name that could not be read — a variable,
// or nothing — leaves Product empty and the reference whole in Text.
//
// Moving says the reference asks for the newest release rather than a
// version, which is a reason there was never a date rather than a line to
// go and look at.
type Unread struct {
	Product string
	Text    string
	Reason  string
	Moving  bool
}

// Read turns a reference into what it declares, or says why it declares no
// version when ok is false.
//
// The product of a name outside the official library is that name, written
// the way Docker Hub means it, for the catalog to look up among the images
// its products publish. Whether anything is known about it is the catalog's
// answer and not this package's.
func Read(ref string) (ns []Named, u Unread, ok bool) {
	name, tag, digest := split(ref)
	// Which half a variable is in is what the reader has to go and look at.
	// A variable in the name leaves no software to speak of, while one in
	// the tag still names software the catalog may or may not track.
	if strings.Contains(name, "$") {
		return nil, Unread{Text: ref, Reason: "takes its image from a variable, so its contents are not known here"}, false
	}
	product, official := officialName(name)
	switch {
	case product == "":
		return nil, Unread{Text: ref, Reason: "names no image"}, false
	case strings.Contains(tag, "$"):
		return nil, Unread{Product: product, Text: tag, Reason: "takes its version from a variable"}, false
	case digest != "" && tag == "":
		return nil, Unread{Product: product, Text: digest, Reason: "is pinned by digest, which does not say which version it is"}, false
	case tag == "":
		return nil, Unread{Product: product, Reason: "names no tag, so it follows latest", Moving: true}, false
	case tag == "latest":
		return nil, Unread{Product: product, Text: tag, Reason: "names latest, not a version", Moving: true}, false
	}
	ns = []Named{{Product: product, Version: versionOf(tag)}}
	if !official {
		// The variant convention is the official library's own. Another
		// publisher's tag may end in any word at all, and reading one as a
		// distribution would be the guess this package does not make.
		return ns, Unread{}, true
	}
	return append(ns, base(tag)...), Unread{}, true
}

// base reads the operating system an official image's tag says it was built
// on, which is the half of the tag that usually expires first: a
// python:3.11-bullseye run in 2026 is a supported Python on a Debian that
// stopped getting security fixes.
//
// The variant is spelled after a dash — 3.12-slim-bookworm, 20-alpine3.19 —
// and its segments say which build this is. Two kinds carry a version. A
// segment spelled alpine3.19 names Alpine 3.19 outright. A segment that is
// one word may be a distribution codename, which names the release and the
// distribution at once; the catalog settles which words those are, so a
// segment it has no codename for was slim, fpm or jre and never a
// declaration at all.
func base(tag string) []Named {
	_, variant, found := strings.Cut(tag, "-")
	if !found {
		return nil
	}
	var out []Named
	for seg := range strings.SplitSeq(variant, "-") {
		if v, ok := strings.CutPrefix(seg, "alpine"); ok && v != "" && v[0] >= '0' && v[0] <= '9' {
			out = append(out, Named{Product: "alpine", Version: v})
			continue
		}
		if word(seg) {
			out = append(out, Named{Version: seg})
		}
	}
	return out
}

// word reports whether a tag segment is one lowercase word, which is how a
// codename is written. Anything carrying a digit — ltsc2022, jre17 — names a
// build of something rather than a release of a distribution.
func word(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// split breaks a reference into its name, tag, and digest. The tag is cut at
// the last colon that comes after the last slash, so a registry written with
// a port keeps it.
func split(ref string) (name, tag, digest string) {
	name = ref
	if i := strings.Index(name, "@"); i >= 0 {
		name, digest = name[:i], name[i+1:]
	}
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name, tag = name[:i], name[i+1:]
	}
	return name, tag, digest
}

// officialName reads an image name: the software it names when it is a
// Docker official image, and otherwise the name itself, with Docker Hub's
// own host taken off so that the repository is spelled the way a purl
// spells it. The host and the library namespace, both of which a reference
// may leave out, are taken off in turn; anything left carrying a slash sits
// in a namespace somebody else controls.
func officialName(name string) (string, bool) {
	for _, prefix := range mirrorPrefixes {
		if rest, ok := strings.CutPrefix(name, prefix); ok {
			if s, ok := single(rest); ok {
				return s, true
			}
			return name, false
		}
	}
	for _, host := range hubHosts {
		if rest, ok := strings.CutPrefix(name, host); ok {
			name = rest
			break
		}
	}
	// The namespace is written out in library/python and left out in
	// python; both name the same image.
	if rest, ok := strings.CutPrefix(name, "library/"); ok {
		name = rest
	}
	if s, ok := single(name); ok {
		return s, true
	}
	return name, false
}

// single reports whether what is left of a name is one segment, which is
// what an official image name is once its namespace is off.
func single(name string) (string, bool) {
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

// versionOf is the part of a tag that names a version. Official images spell
// their variants after a dash — 3.9.10-slim-buster, 20-bookworm — so the
// first segment is the version and the rest is which build of it. A tag with
// no dash is left whole, which is what carries a codename such as bookworm
// through to the catalog.
//
// A leading v is dropped, as it is everywhere else a version is read: some
// images tag v2.1 where the catalog names the cycle 2.1. No codename starts
// with one.
func versionOf(tag string) string {
	v, _, _ := strings.Cut(tag, "-")
	return strings.TrimPrefix(v, "v")
}
