package composerjson

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"composer.json", true},
		{"composer.lock", false},
		{"package.json", false},
		{"Composer.json", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

const manifest = `{
    "name": "acme/site",
    "require": {
        "php": "^7.4",
        "ext-json": "*",
        "Laravel/Framework": "^8.0",
        "monolog/monolog": "^2.0"
    },
    "autoload": {"psr-4": {"App\\": "src/"}},
    "require-dev": {
        "behat/behat": "^3.0"
    }
}`

func TestExtract(t *testing.T) {
	ds, us := Extract("composer.json", []byte(manifest))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	want := []struct {
		ecosystem, product, version string
		line                        int
	}{
		// Composer reserves php for the version of the language, which is a
		// runtime declaration and not a package: the catalog answers it by
		// name.
		{"", "php", "^7.4", 4},
		// A package name is lowercased, as Composer requires it to be
		// written and as a purl spells it.
		{"composer", "laravel/framework", "^8.0", 6},
		{"composer", "monolog/monolog", "^2.0", 7},
		// What a project needs to develop goes out of support the same way.
		{"composer", "behat/behat", "^3.0", 11},
	}
	if len(ds) != len(want) {
		t.Fatalf("declarations = %+v; want %d", ds, len(want))
	}
	for i, w := range want {
		got := ds[i]
		if got.Ecosystem != w.ecosystem || got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %q %q %q; want %q %q %q",
				i, got.Ecosystem, got.Product, got.Version, w.ecosystem, w.product, w.version)
		}
		if got.Source != (decl.Source{File: "composer.json", Line: w.line}) {
			t.Errorf("[%d] source = %s; want composer.json:%d", i, got.Source, w.line)
		}
	}
}

func TestExtractLeavesTheVersionToTheLockfile(t *testing.T) {
	ds, us := Extract("composer.json", []byte(`{"require": {"laravel/framework": "^7.4 || ^8.0"}}`))
	if len(ds) != 0 {
		t.Fatalf("declarations = %+v; want none", ds)
	}
	if len(us) != 1 {
		t.Fatalf("unreadable = %+v; want one", us)
	}
	u := us[0]
	if u.Product != "laravel/framework" || u.Ecosystem != Ecosystem || u.Text != "^7.4 || ^8.0" {
		t.Errorf("= %q %q %q", u.Ecosystem, u.Product, u.Text)
	}
	if !u.Moving {
		t.Error("moving = false; want true, since the lockfile holds the version")
	}
}

func TestExtractPassesOverWhatIsNotAPackage(t *testing.T) {
	for _, name := range []string{"ext-json", "lib-openssl", "composer-runtime-api", "composer-plugin-api", "hhvm", "php-64bit"} {
		t.Run(name, func(t *testing.T) {
			ds, us := Extract("composer.json", []byte(`{"require": {"`+name+`": "^1.0"}}`))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v %+v; want neither", ds, us)
			}
		})
	}
}

// TestExtractReadsAVendorNamedLikeAReservedOne guards the one place the
// reserved names could take a package with them: composer/semver is a
// package, and the slash is what says so.
func TestExtractReadsAVendorNamedLikeAReservedOne(t *testing.T) {
	for _, name := range []string{"composer/semver", "php-http/client-common", "ext-lib/thing"} {
		t.Run(name, func(t *testing.T) {
			ds, _ := Extract("composer.json", []byte(`{"require": {"`+name+`": "^1.0"}}`))
			if len(ds) != 1 || ds[0].Product != name || ds[0].Ecosystem != Ecosystem {
				t.Errorf("= %+v; want the package %q", ds, name)
			}
		})
	}
}

func TestExtractSkipsWhatItCannotRead(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"not JSON yet", `{"require": {`},
		{"not an object at the top", `["require"]`},
		{"require is not an object", `{"require": "laravel/framework"}`},
		{"require is a list", `{"require": ["laravel/framework"]}`},
		// Half a document is not half a set of declarations, so what was
		// read before it ran out is given up on with the rest.
		{"cut short after require closed", `{"require": {"laravel/framework": "^8.0"}, "name"`},
		{"a constraint that is not a string", `{"require": {"laravel/framework": 8}}`},
		{"nothing required", `{"name": "acme/site"}`},
		{"empty", ``},
		{"an unnamed package", `{"require": {"": "^1.0"}}`},
		// A file cut short anywhere gives up what it has read so far, which
		// here is nothing.
		{"cut short inside a key", `{"na`},
		{"cut short after a key", `{"name"`},
		{"cut short before a value", `{"name": "acme/site", "require"`},
		{"cut short inside a package name", `{"require": {"lara`},
		{"cut short inside require", `{"require": {"laravel/framework"`},
		{"cut short before require closes", `{"require": {"laravel/framework": "^8.0"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("composer.json", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v %+v; want neither", ds, us)
			}
		})
	}
}
