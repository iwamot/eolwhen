package packagejson

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"package.json", true},
		{"package-lock.json", false},
		{"composer.json", false},
		{"Package.json", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

const manifest = `{
    "name": "demo",
    "packageManager": "pnpm@9.15.4+sha512.abc",
    "dependencies": {
        "next": "~13.4.1",
        "@angular/core": "^12.0.0"
    },
    "scripts": {"build": "next build"},
    "devDependencies": {
        "eslint": "~8.57.0"
    },
    "peerDependencies": {
        "react": "18.x"
    },
    "optionalDependencies": {
        "fsevents": "^2.3.0"
    }
}`

func TestExtract(t *testing.T) {
	ds, us := Extract("package.json", []byte(manifest))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	want := []struct {
		ecosystem, product, version string
		line                        int
	}{
		// The package manager names a tool and an exact version of it, not
		// a package, so the catalog answers it by name.
		{"", "pnpm", "9.15.4", 3},
		{Ecosystem, "next", "~13.4.1", 5},
		// A scoped name is written in a purl with its at sign escaped, and
		// the catalog decodes it back to what the manifest wrote.
		{Ecosystem, "@angular/core", "^12.0.0", 6},
		// What a project needs to develop goes out of support the same way,
		// and so do the versions it asks its host and its extras for.
		{Ecosystem, "eslint", "~8.57.0", 10},
		{Ecosystem, "react", "18.x", 13},
		{Ecosystem, "fsevents", "^2.3.0", 16},
	}
	if len(ds) != len(want) {
		t.Fatalf("declarations = %+v; want %d", ds, len(want))
	}
	for i, w := range want {
		got := ds[i]
		if got.Ecosystem != w.ecosystem || got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %q %s %q; want %q %s %q", i, got.Ecosystem, got.Product, got.Version, w.ecosystem, w.product, w.version)
		}
		if got.Source != (decl.Source{File: "package.json", Line: w.line}) {
			t.Errorf("[%d] source = %s; want package.json:%d", i, got.Source, w.line)
		}
	}
}

// A name is kept as the manifest spelled it, which is what a reader greps
// for. npm has required a lower-case name for years, but the packages from
// before it did are still there and still depended on.
func TestExtractKeepsAName(t *testing.T) {
	ds, _ := Extract("package.json", []byte(`{"dependencies": {"JSONStream": "1.3.5"}}`))
	if len(ds) != 1 || ds[0].Product != "JSONStream" {
		t.Errorf("= %+v; want the name as written", ds)
	}
}

// A requirement reaching no single cycle is not a line to go and look at:
// the version it became is in the lockfile beside it.
func TestExtractSetsAsideARequirementWithNoOneVersion(t *testing.T) {
	ds, us := Extract("package.json", []byte(`{"dependencies": {"vue": ">=2.6"}}`))
	want := decl.Unreadable{
		Source:    decl.Source{File: "package.json", Line: 1},
		Ecosystem: Ecosystem,
		Product:   "vue",
		Text:      ">=2.6",
		Reason:    "names no single version here, so the lockfile decides which one",
		Moving:    true,
	}
	if len(ds) != 0 || len(us) != 1 || us[0] != want {
		t.Errorf("= %+v, %+v; want the one set-aside line", ds, us)
	}
}

func TestExtractReadsThePackageManager(t *testing.T) {
	for _, tt := range []struct{ value, product, version string }{
		{"pnpm@10.18.0", "pnpm", "10.18.0"},
		// A hash says which download was meant, not which version.
		{"pnpm@9.15.4+sha512.abc", "pnpm", "9.15.4"},
		{"yarn@4.9.1", "yarn", "4.9.1"},
		{"npm@10.8.2", "npm", "10.8.2"},
		{"Bun@1.2.0", "Bun", "1.2.0"},
	} {
		t.Run(tt.value, func(t *testing.T) {
			ds, us := Extract("package.json", []byte(`{"packageManager": "`+tt.value+`"}`))
			if len(us) != 0 {
				t.Fatalf("unreadable = %+v; want none", us)
			}
			if len(ds) != 1 || ds[0].Ecosystem != "" || ds[0].Product != tt.product || ds[0].Version != tt.version {
				t.Errorf("= %+v; want %s %s with no ecosystem", ds, tt.product, tt.version)
			}
		})
	}
}

// The field's own rule is an exact version — corepack installs that one and
// no other — so a value that is not one is a line to go and look at rather
// than one a resolver settles.
func TestExtractReportsAPackageManagerThatIsNotOne(t *testing.T) {
	for _, value := range []string{"pnpm", "pnpm@latest", "pnpm@^10", "@10.18.0", "pnpm@"} {
		t.Run(value, func(t *testing.T) {
			ds, us := Extract("package.json", []byte(`{"packageManager": "`+value+`"}`))
			want := decl.Unreadable{
				Source: decl.Source{File: "package.json", Line: 1},
				Text:   value,
				Reason: "does not name a package manager and a version of it",
			}
			if len(ds) != 0 || len(us) != 1 || us[0] != want {
				t.Errorf("= %+v, %+v; want the one complaint", ds, us)
			}
		})
	}
}

func TestExtractReadsNothingElse(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"not JSON yet", `{"dependencies": {`},
		{"nothing depended on", `{"name": "demo"}`},
		{"empty", ``},
		{"dependencies is not an object", `{"dependencies": "next"}`},
		{"the package manager is not a string", `{"packageManager": {"name": "pnpm"}}`},
		{"an unnamed package", `{"dependencies": {"": "^1.0.0"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("package.json", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v %+v; want neither", ds, us)
			}
		})
	}
}
