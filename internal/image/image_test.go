package image

import (
	"reflect"
	"testing"
)

func TestSkip(t *testing.T) {
	for _, tt := range []struct {
		ref  string
		want bool
	}{
		{"scratch", true},
		{"scratch:latest", true},
		{"python:3.7", false},
		{"acme/scratch", false},
		{"", false},
	} {
		if got := Skip(tt.ref); got != tt.want {
			t.Errorf("Skip(%q) = %v; want %v", tt.ref, got, tt.want)
		}
	}
}

// TestReadCaught is the half that reaches the timeline: an official image
// with a tag that names a version. The first declaration is the software the
// image is; TestReadBase covers the ones its variant adds.
func TestReadCaught(t *testing.T) {
	for _, tt := range []struct{ name, ref, product, version string }{
		{"plain", "python:3.7", "python", "3.7"},
		{"a variant after the dash", "python:3.9.10-slim-buster", "python", "3.9.10"},
		{"a two-part version", "ubuntu:18.04", "ubuntu", "18.04"},
		{"a bare major", "centos:7", "centos", "7"},
		{"the library form spelled out", "library/node:20", "node", "20"},
		{"with Docker Hub's own host", "docker.io/library/redis:6.2", "redis", "6.2"},
		{"the full registry name", "index.docker.io/library/redis:6.2", "redis", "6.2"},
		{"the registry's own hostname", "registry-1.docker.io/library/redis:6.2", "redis", "6.2"},
		// Docker Hub fills in the library namespace when a name is written
		// without one, so these name the official image too.
		{"Docker Hub's host and no namespace", "docker.io/python:2.7", "python", "2.7"},
		{"the full registry name and no namespace", "index.docker.io/python:2.7", "python", "2.7"},
		{"the registry's own hostname and no namespace", "registry-1.docker.io/python:2.7", "python", "2.7"},
		{"a mirror of the official library", "public.ecr.aws/docker/library/node:24.21.0-trixie-slim", "node", "24.21.0"},
		// A tag next to a digest is still the version the author wrote.
		{"a tag pinned by digest", "python:3.7-slim@sha256:00", "python", "3.7"},
		// A codename is left whole for the catalog to recognize.
		// Some images tag a v where the catalog names the cycle without one.
		{"a v-prefixed tag", "traefik:v2.1", "traefik", "2.1"},
		{"a v-prefixed tag with a variant", "node:v20-alpine", "node", "20"},
		{"a codename", "debian:bookworm-slim", "debian", "bookworm"},
		{"a codename with a date", "ubuntu:jammy-20230624", "ubuntu", "jammy"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ns, u, ok := Read(tt.ref)
			if !ok {
				t.Fatalf("Read(%q) reason = %q; want none", tt.ref, u.Reason)
			}
			if len(ns) == 0 {
				t.Fatalf("Read(%q) named nothing", tt.ref)
			}
			if ns[0].Product != tt.product || ns[0].Version != tt.version {
				t.Errorf("Read(%q) = %s %s; want %s %s", tt.ref, ns[0].Product, ns[0].Version, tt.product, tt.version)
			}
		})
	}
}

// TestReadReported is the half that is read and deliberately not matched.
// Each names no version this tool can place, so guessing is what would put
// a wrong date on the timeline. The software is still named wherever the
// name says it, so that the catalog can set aside what it does not track.
func TestReadReported(t *testing.T) {
	for _, tt := range []struct {
		name, ref string
		want      Unread
	}{
		{"nothing at all", "", Unread{Reason: "names no image"}},
		{"pinned by digest", "python@sha256:00", Unread{Product: "python", Text: "sha256:00",
			Reason: "is pinned by digest, which does not say which version it is"}},
		// A reference that follows the newest release was never going to
		// have a date, so it is moving rather than a line to go and read.
		{"no tag", "python", Unread{Product: "python", Reason: "names no tag, so it follows latest", Moving: true}},
		{"latest", "python:latest", Unread{Product: "python", Text: "latest", Reason: "names latest, not a version", Moving: true}},
		{"a variable in the tag", "python:${PYTHON_VERSION}",
			Unread{Product: "python", Text: "${PYTHON_VERSION}", Reason: "takes its version from a variable"}},
		{"a variable in the name", "${REGISTRY}python:3.11-bullseye", Unread{Text: "${REGISTRY}python:3.11-bullseye",
			Reason: "takes its image from a variable, so its contents are not known here"}},
		// Outside the official library the name is the product as written,
		// for the catalog to know or not.
		{"another publisher's image with no tag", "sonatype/nexus",
			Unread{Product: "sonatype/nexus", Reason: "names no tag, so it follows latest", Moving: true}},
		{"another publisher's image by digest", "ghcr.io/acme/app@sha256:00", Unread{Product: "ghcr.io/acme/app", Text: "sha256:00",
			Reason: "is pinned by digest, which does not say which version it is"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ns, u, ok := Read(tt.ref)
			if ok || len(ns) != 0 {
				t.Fatalf("Read(%q) = %+v; want nothing", tt.ref, ns)
			}
			if u != tt.want {
				t.Errorf("Read(%q) = %+v; want %+v", tt.ref, u, tt.want)
			}
		})
	}
}

// TestReadBase covers the half of a tag that usually expires first: the
// distribution the image was built on, which the variant after the dash
// names. A segment that is not one carries no declaration, and the ones that
// name a codename leave the product for the catalog to fill in.
func TestReadBase(t *testing.T) {
	for _, tt := range []struct {
		name, ref string
		want      []Named
	}{
		{"a codename variant", "python:3.12.4-bookworm", []Named{
			{Product: "python", Version: "3.12.4"},
			{Version: "bookworm"},
		}},
		{"a codename behind another word", "python:3.12-slim-bookworm", []Named{
			{Product: "python", Version: "3.12"},
			{Version: "slim"},
			{Version: "bookworm"},
		}},
		{"an alpine release", "node:20-alpine3.19", []Named{
			{Product: "node", Version: "20"},
			{Product: "alpine", Version: "3.19"},
		}},
		{"alpine with no release named", "php:8.2-fpm-alpine", []Named{
			{Product: "php", Version: "8.2"},
			{Version: "fpm"},
			{Version: "alpine"},
		}},
		{"a segment carrying digits is no codename", "python:3.12-windowsservercore-ltsc2022", []Named{
			{Product: "python", Version: "3.12"},
			{Version: "windowsservercore"},
		}},
		// A codename is written in lowercase and is a word; neither a
		// shouted variant nor the nothing between two dashes is one.
		{"a variant in capitals", "python:3.12-SLIM", []Named{
			{Product: "python", Version: "3.12"},
		}},
		{"an empty segment", "python:3.12--slim", []Named{
			{Product: "python", Version: "3.12"},
			{Version: "slim"},
		}},
		{"a tag with no variant at all", "postgres:16", []Named{
			{Product: "postgres", Version: "16"},
		}},
		{"the distribution's own image names it once", "debian:bookworm-slim", []Named{
			{Product: "debian", Version: "bookworm"},
			{Version: "slim"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ns, u, ok := Read(tt.ref)
			if !ok {
				t.Fatalf("Read(%q) reason = %q; want none", tt.ref, u.Reason)
			}
			if !reflect.DeepEqual(ns, tt.want) {
				t.Errorf("Read(%q) = %+v; want %+v", tt.ref, ns, tt.want)
			}
		})
	}
}

// TestReadOutsideTheLibrary covers the names anyone may publish. None of
// them says what it holds, so each is handed on as the repository it is,
// for the catalog to recognize among the images its products publish or
// not. Nothing is guessed: acme/python is a name, not Python.
func TestReadOutsideTheLibrary(t *testing.T) {
	for _, tt := range []struct{ name, ref, product, version string }{
		{"a Docker Hub user's image", "acme/python:3.7", "acme/python", "3.7"},
		{"another registry", "ghcr.io/acme/python:3.7", "ghcr.io/acme/python", "3.7"},
		{"a private registry with a port", "registry.corp:5000/base:1.2", "registry.corp:5000/base", "1.2"},
		{"a mirror of something else entirely", "public.ecr.aws/acme/python:3.7", "public.ecr.aws/acme/python", "3.7"},
		// Filling in the library namespace is Docker Hub's rule and nobody
		// else's, so a namespace on Docker Hub is still somebody else's and
		// a mirror is only the library at the exact path it copies it to.
		// Docker Hub's host comes off, because a purl spells the repository
		// without it.
		{"another namespace on Docker Hub", "docker.io/acme/python:3.7", "acme/python", "3.7"},
		{"a mirror without the library path", "public.ecr.aws/docker/python:3.7", "public.ecr.aws/docker/python", "3.7"},
		{"one endoflife.date publishes a purl for", "opensearchproject/opensearch:1.3.0", "opensearchproject/opensearch", "1.3.0"},
		{"a namespace under a mirror's library path", "public.ecr.aws/docker/library/acme/python:3.7", "public.ecr.aws/docker/library/acme/python", "3.7"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ns, u, ok := Read(tt.ref)
			if !ok {
				t.Fatalf("Read(%q) reason = %q; want none", tt.ref, u.Reason)
			}
			want := []Named{{Product: tt.product, Version: tt.version}}
			if !reflect.DeepEqual(ns, want) {
				t.Errorf("Read(%q) = %+v; want %+v", tt.ref, ns, want)
			}
		})
	}
}

// TestReadVariantIsTheLibrarys: the variant convention is the official
// library's own, so another publisher's tag ending in a word is left as the
// word it is rather than read as the distribution of that name.
func TestReadVariantIsTheLibrarys(t *testing.T) {
	ns, _, _ := Read("acme/app:1.2-bookworm")
	want := []Named{{Product: "acme/app", Version: "1.2"}}
	if !reflect.DeepEqual(ns, want) {
		t.Errorf("Read = %+v; want %+v", ns, want)
	}
}
