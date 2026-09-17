package image

import "testing"

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
// with a tag that names a version.
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
			product, version, reason := Read(tt.ref)
			if reason != "" {
				t.Fatalf("Read(%q) reason = %q; want none", tt.ref, reason)
			}
			if product != tt.product || version != tt.version {
				t.Errorf("Read(%q) = %s %s; want %s %s", tt.ref, product, version, tt.product, tt.version)
			}
		})
	}
}

// TestReadReported is the half that is read and deliberately not matched.
// Each names software this tool cannot identify from the line, so guessing
// is what would put a wrong date on the timeline.
func TestReadReported(t *testing.T) {
	for _, tt := range []struct{ name, ref, reason string }{
		{"someone else's namespace", "ghcr.io/acme/python:3.7",
			"is not a Docker official image, so its contents are not known here"},
		{"a Docker Hub user's image", "acme/python:3.7",
			"is not a Docker official image, so its contents are not known here"},
		{"a private registry with a port", "registry.corp:5000/base:1.2",
			"is not a Docker official image, so its contents are not known here"},
		{"a mirror of something else entirely", "public.ecr.aws/acme/python:3.7",
			"is not a Docker official image, so its contents are not known here"},
		// Filling in the library namespace is Docker Hub's rule and nobody
		// else's, so a namespace on Docker Hub is still somebody else's and
		// a mirror is only the library at the exact path it copies it to.
		{"another namespace on Docker Hub", "docker.io/acme/python:3.7",
			"is not a Docker official image, so its contents are not known here"},
		{"a mirror without the library path", "public.ecr.aws/docker/python:3.7",
			"is not a Docker official image, so its contents are not known here"},
		{"nothing at all", "",
			"is not a Docker official image, so its contents are not known here"},
		{"pinned by digest", "python@sha256:00",
			"is pinned by digest, which does not say which version it is"},
		{"no tag", "python", "names no tag, so it follows latest"},
		{"latest", "python:latest", "names latest, not a version"},
		{"a variable", "python:${PYTHON_VERSION}", "takes its version from a variable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			product, version, reason := Read(tt.ref)
			if reason != tt.reason {
				t.Fatalf("Read(%q) reason = %q; want %q", tt.ref, reason, tt.reason)
			}
			if product != "" || version != "" {
				t.Errorf("Read(%q) = %s %s; want nothing", tt.ref, product, version)
			}
		})
	}
}
