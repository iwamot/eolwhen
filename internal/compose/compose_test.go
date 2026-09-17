package compose

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"compose.yml", true},
		{"compose.yaml", true},
		{"docker-compose.yml", true},
		{"docker-compose.yaml", true},
		// An override file carries a profile between the stem and the
		// extension.
		{"compose.override.yml", true},
		{"docker-compose.prod.yaml", true},
		{"composer.yml", false},
		{"my-compose.yml", false},
		{"compose", false},
		{"compose.yml.bak", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

func TestExtract(t *testing.T) {
	body := "services:\n" +
		"  db:\n" +
		"    image: postgres:11-alpine\n" +
		"  cache:\n" +
		"    image: redis:5\n" +
		"  web:\n" +
		"    build: .\n" +
		"  legacy:\n" +
		"    image: ghcr.io/acme/web:1.0\n"
	ds, us := Extract("compose.yml", []byte(body))
	// The alpine between them is the variant the postgres tag carries, which
	// is declared at the same line as the image itself, and the last is the
	// private image, handed on as the repository it is.
	if len(ds) != 4 {
		t.Fatalf("declarations = %+v; want 4", ds)
	}
	if ds[0].Product != "postgres" || ds[0].Version != "11" {
		t.Errorf("[0] = %+v; want postgres 11", ds[0])
	}
	if ds[0].Source != (decl.Source{File: "compose.yml", Line: 3}) {
		t.Errorf("[0] source = %v; want compose.yml:3", ds[0].Source)
	}
	if ds[1].Product != "" || ds[1].Version != "alpine" {
		t.Errorf("[1] = %+v; want the alpine variant", ds[1])
	}
	if ds[2].Product != "redis" || ds[2].Version != "5" {
		t.Errorf("[2] = %+v; want redis 5", ds[2])
	}
	if ds[3].Product != "ghcr.io/acme/web" || ds[3].Version != "1.0" {
		t.Errorf("[3] = %+v; want ghcr.io/acme/web 1.0", ds[3])
	}
	if len(us) != 0 {
		t.Errorf("unreadable = %+v; want none", us)
	}
}

func TestExtractNothing(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"empty", ""},
		{"only builds", "services:\n  web:\n    build: .\n"},
		// A service may be defined by what it connects to and nothing else.
		{"neither builds nor names an image", "services:\n  web:\n    ports: [\"80:80\"]\n    depends_on: [db]\n"},
		// image: next to build: names what the service builds, not what it
		// stands on; the base is in the Dockerfile the build points at.
		{"a name for what it builds", "services:\n  web:\n    build: .\n    image: myapp\n"},
		{"the same with a build block", "services:\n  web:\n    build:\n      context: .\n    image: ghcr.io/me/app:1.0\n"},
		{"scratch", "services:\n  web:\n    image: scratch\n"},
		{"an image that is not a scalar", "services:\n  web:\n    image: [a, b]\n"},
		{"not YAML", "\tthis: is: not: yaml\n  - [\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("compose.yml", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

// TestExtractAnchored: a value carrying an anchor is read like any other,
// and a template nobody applies is not a service.
func TestExtractAnchored(t *testing.T) {
	body := "x-db: &db\n  image: postgres:11\nservices:\n  cache:\n    image: &img redis:6\n"
	ds, us := Extract("compose.yml", []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != 1 || ds[0].Product != "redis" {
		t.Fatalf("declarations = %+v; want redis alone", ds)
	}
}

// TestExtractVariable: Compose interpolates the environment, so a version
// written as a variable is not in the file.
func TestExtractVariable(t *testing.T) {
	ds, us := Extract("compose.yml", []byte("services:\n  db:\n    image: postgres:${PG_TAG}\n"))
	if len(ds) != 0 || len(us) != 1 || us[0].Reason != "takes its version from a variable" {
		t.Errorf("Extract = %+v, %+v", ds, us)
	}
}

// TestExtractServicesOnly: a Compose file keeps templates in extension
// fields and a service pulls one in with a merge key. Reading every mapping
// that has an image: would report a template the file never applies, and
// would miss that the service applying it overrode the image.
func TestExtractServicesOnly(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want []string
	}{
		{"an override wins over the template",
			"x-defaults: &d\n  image: python:2.7\nservices:\n  app:\n    <<: *d\n    image: python:3.13\n",
			[]string{"python 3.13"}},
		{"a service with no image of its own inherits one",
			"x-defaults: &d\n  image: python:2.7\nservices:\n  app:\n    <<: *d\n",
			[]string{"python 2.7"}},
		{"a template nobody applies is not a service",
			"x-unused: &u\n  image: python:2.7\nservices:\n  app:\n    image: python:3.13\n",
			[]string{"python 3.13"}},
		{"a key called image inside a service is not the service's image",
			"services:\n  app:\n    image: python:3.13\n    environment:\n      image: python:2.7\n",
			[]string{"python 3.13"}},
		{"a build inherited through a merge still excludes the image",
			"x-b: &b\n  build: .\nservices:\n  app:\n    <<: *b\n    image: myapp:1.0\n",
			nil},
		{"an image outside services entirely", "x-only: &o\n  image: python:2.7\n", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("compose.yml", []byte(tt.body))
			if len(us) != 0 {
				t.Fatalf("unreadable = %+v; want none", us)
			}
			if len(ds) != len(tt.want) {
				t.Fatalf("declarations = %+v; want %v", ds, tt.want)
			}
			for i, w := range tt.want {
				if got := ds[i].Product + " " + ds[i].Version; got != w {
					t.Errorf("[%d] = %q; want %q", i, got, w)
				}
			}
		})
	}
}

// TestExtractOddShapes: services, and the services block itself, written as
// something other than a mapping. Nothing is read and nothing is claimed.
func TestExtractOddShapes(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"services is a list", "services: [a, b]\n"},
		{"a service is a scalar", "services:\n  app: hello\n"},
		{"services is empty", "services:\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("compose.yml", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

// TestExtractAnchorRedefinedLater: a name may be defined again further down
// the file, and YAML says that does not reach back and change what an
// earlier alias meant. The service refers to the definition above it.
func TestExtractAnchorRedefinedLater(t *testing.T) {
	body := "x-old: &img python:2.7\n" +
		"services:\n" +
		"  app:\n" +
		"    image: *img\n" +
		"x-new: &img python:3.13\n"
	ds, us := Extract("compose.yml", []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != 1 || ds[0].Product != "python" || ds[0].Version != "2.7" {
		t.Fatalf("declarations = %+v; want python 2.7", ds)
	}
	// The row points at where the version was referred from.
	if ds[0].Source != (decl.Source{File: "compose.yml", Line: 4}) {
		t.Errorf("Source = %v; want compose.yml:4", ds[0].Source)
	}
}

// TestExtractSelfReferringMerge: a template holding itself is read for what
// it does say. That it ends at all is checked by running the binary, in e2e,
// a stack overflow being fatal to whatever process meets it.
func TestExtractSelfReferringMerge(t *testing.T) {
	body := "x-loop: &loop [*loop]\nservices:\n  app:\n    <<: *loop\n    image: python:2.7\n"
	ds, _ := Extract("compose.yml", []byte(body))
	if len(ds) != 1 || ds[0].Version != "2.7" {
		t.Fatalf("declarations = %+v; want the image the service states itself", ds)
	}
}
