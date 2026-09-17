package gemfile

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"Gemfile", true},
		{"gems.rb", true},
		{"Gemfile.lock", false},
		{"gemfile", false},
		{"rails.gemspec", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

type want struct {
	product string
	version string
	from    string
	below   string
	line    int
}

func check(t *testing.T, body string, ws ...want) {
	t.Helper()
	ds, _ := Extract("Gemfile", []byte(body))
	if len(ds) != len(ws) {
		t.Fatalf("declarations = %+v; want %d", ds, len(ws))
	}
	for i, w := range ws {
		got := ds[i]
		if got.Ecosystem != Ecosystem {
			t.Errorf("[%d] ecosystem = %q; want %q", i, got.Ecosystem, Ecosystem)
		}
		if got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %s %q; want %s %q", i, got.Product, got.Version, w.product, w.version)
		}
		if got.From != w.from || got.Below != w.below {
			t.Errorf("[%d] range = %q..%q; want %q..%q", i, got.From, got.Below, w.from, w.below)
		}
		if got.Source != (decl.Source{File: "Gemfile", Line: w.line}) {
			t.Errorf("[%d] source = %s; want Gemfile:%d", i, got.Source, w.line)
		}
	}
}

func TestExtract(t *testing.T) {
	t.Run("a gem with a pessimistic requirement", func(t *testing.T) {
		check(t, "gem \"rails\", \"~> 6.1.0\"\n",
			want{"rails", "~> 6.1.0", "6.1.0", "6.2", 1})
	})
	t.Run("single quotes read the same", func(t *testing.T) {
		check(t, "gem 'rails', '~> 6.1.0'\n",
			want{"rails", "~> 6.1.0", "6.1.0", "6.2", 1})
	})
	t.Run("two requirements are both read", func(t *testing.T) {
		check(t, "gem \"rails\", \">= 6.0\", \"< 7\"\n",
			want{"rails", ">= 6.0, < 7", "6.0", "7", 1})
	})
	t.Run("keyword arguments end the requirements", func(t *testing.T) {
		check(t, "gem \"rails\", \"~> 6.1.0\", require: false\n",
			want{"rails", "~> 6.1.0", "6.1.0", "6.2", 1})
	})
	t.Run("a trailing comment is not a requirement", func(t *testing.T) {
		check(t, "gem \"rails\", \"~> 6.1.0\" # the framework\n",
			want{"rails", "~> 6.1.0", "6.1.0", "6.2", 1})
	})
	t.Run("a gem inside a group keeps its line", func(t *testing.T) {
		check(t, "group :production do\n  gem \"rails\", \"~> 6.1.0\"\nend\n",
			want{"rails", "~> 6.1.0", "6.1.0", "6.2", 2})
	})
	t.Run("a commented-out gem is not a declaration", func(t *testing.T) {
		check(t, "# gem \"rails\", \"~> 6.1.0\"\n")
	})
	t.Run("what is not a gem line is passed over", func(t *testing.T) {
		check(t, "source \"https://rubygems.org\"\nruby \"3.1.4\"\ngemspec\n")
	})
	t.Run("a name built at runtime is not a literal", func(t *testing.T) {
		check(t, "gem name, \"~> 6.1.0\"\n")
	})
}

func TestExtractLeavesTheVersionToTheLockfile(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		text string
	}{
		{"no requirement at all", "gem \"sidekiq\"\n", ""},
		{"a floor alone", "gem \"sidekiq\", \">= 6.0\"\n", ">= 6.0"},
		{"a source instead of a version", "gem \"sidekiq\", github: \"sidekiq/sidekiq\"\n", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract("Gemfile", []byte(tt.body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			if len(us) != 1 {
				t.Fatalf("unreadable = %+v; want one", us)
			}
			u := us[0]
			if u.Product != "sidekiq" || u.Text != tt.text {
				t.Errorf("= %q %q; want %q %q", u.Product, u.Text, "sidekiq", tt.text)
			}
			if u.Ecosystem != Ecosystem {
				t.Errorf("ecosystem = %q; want %q", u.Ecosystem, Ecosystem)
			}
			if !u.Moving {
				t.Error("moving = false; want true, since the lockfile holds the version")
			}
		})
	}
}
