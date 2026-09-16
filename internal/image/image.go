// Package image reads a container image reference.
//
// Only Docker official images are read. An official image name is the
// software itself — library/python is Python, library/postgres is PostgreSQL
// — because that namespace is a short curated list rather than something
// anyone can publish into. An image under any other namespace may be named
// anything at all, so ghcr.io/acme/python is reported rather than read as
// Python.
package image

import "strings"

// officialPrefixes are the ways of writing that an image comes from the
// Docker official library: the bare shorthand, the library/ form it stands
// for, and the registries that mirror that library under a path of their
// own. Each is listed because it was checked, not because it matched a
// pattern; a registry not on this list may put anything under any name.
var officialPrefixes = []string{
	"library/",
	"docker.io/library/",
	"index.docker.io/library/",
	"registry-1.docker.io/library/",
	"public.ecr.aws/docker/library/",
}

// Skip reports whether a reference is one there is nothing to look up for.
// scratch is the empty image rather than a piece of software, so no version
// was ever missing from it.
func Skip(ref string) bool {
	name, _, _ := split(ref)
	return name == "scratch"
}

// Read turns a reference into the software and version it names. reason is
// empty when it could be read, and otherwise says why not, in words that
// name what is missing.
func Read(ref string) (product, version, reason string) {
	if strings.Contains(ref, "$") {
		return "", "", "takes its version from a variable"
	}
	name, tag, digest := split(ref)
	product, official := officialName(name)
	switch {
	case !official:
		return "", "", "is not a Docker official image, so its contents are not known here"
	case digest != "" && tag == "":
		return "", "", "is pinned by digest, which does not say which version it is"
	case tag == "":
		return "", "", "names no tag, so it follows latest"
	case tag == "latest":
		return "", "", "names latest, not a version"
	}
	return product, versionOf(tag), ""
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

// officialName reports whether an image name is a Docker official image, and
// returns the software it names. Anything left carrying a slash sits in a
// namespace somebody else controls.
func officialName(name string) (string, bool) {
	for _, prefix := range officialPrefixes {
		if rest, ok := strings.CutPrefix(name, prefix); ok {
			name = rest
			break
		}
	}
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
