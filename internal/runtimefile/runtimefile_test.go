package runtimefile

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestProduct(t *testing.T) {
	for _, tt := range []struct {
		file string
		want string
		ok   bool
	}{
		{".python-version", "python", true},
		{".nvmrc", "nodejs", true},
		{".node-version", "nodejs", true},
		{".ruby-version", "ruby", true},
		{".php-version", "php", true},
		{".go-version", "go", true},
		{".terraform-version", "terraform", true},
		{"go.mod", "go", true},
		{"package.json", "", false},
		// A version manager writes 17 in this one and nothing else, and
		// endoflife.date tracks nine builds of Java. The file settles which
		// software no more than the bare number does.
		{".java-version", "", false},
		{"", "", false},
	} {
		got, ok := Product(tt.file)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Product(%q) = %q, %v; want %q, %v", tt.file, got, ok, tt.want, tt.ok)
		}
	}
}

func TestExtractCaught(t *testing.T) {
	tests := []struct {
		name string
		file string
		data string
		want []decl.Decl
	}{
		{"bare version", ".python-version", "3.9.10\n",
			[]decl.Decl{{Product: "python", Version: "3.9.10", Source: decl.Source{File: ".python-version", Line: 1}}}},
		{"the tool a version manager pins", ".terraform-version", "1.5.7\n",
			[]decl.Decl{{Product: "terraform", Version: "1.5.7", Source: decl.Source{File: ".terraform-version", Line: 1}}}},
		{"a language a version manager pins", ".php-version", "8.1.2\n",
			[]decl.Decl{{Product: "php", Version: "8.1.2", Source: decl.Source{File: ".php-version", Line: 1}}}},
		// The same product a go.mod directive names, from the file goenv
		// keeps it in.
		{"go from its own file", ".go-version", "1.21.5\n",
			[]decl.Decl{{Product: "go", Version: "1.21.5", Source: decl.Source{File: ".go-version", Line: 1}}}},
		{"leading v", ".nvmrc", "v14.19.0\n",
			[]decl.Decl{{Product: "nodejs", Version: "14.19.0", Source: decl.Source{File: ".nvmrc", Line: 1}}}},
		{"rvm prefix", ".ruby-version", "ruby-2.6.2\n",
			[]decl.Decl{{Product: "ruby", Version: "2.6.2", Source: decl.Source{File: ".ruby-version", Line: 1}}}},
		// Another implementation of Ruby is its own software.
		{"jruby", ".ruby-version", "jruby-9.4.8.0\n",
			[]decl.Decl{{Product: "jruby", Version: "9.4.8.0", Source: decl.Source{File: ".ruby-version", Line: 1}}}},
		{"truffleruby", ".ruby-version", "truffleruby-24.1.0\n",
			[]decl.Decl{{Product: "truffleruby", Version: "24.1.0", Source: decl.Source{File: ".ruby-version", Line: 1}}}},
		{"node-version prefix", ".node-version", "node-18.0.0\n",
			[]decl.Decl{{Product: "nodejs", Version: "18.0.0", Source: decl.Source{File: ".node-version", Line: 1}}}},
		{"go directive", "go.mod", "module example.com/x\n\ngo 1.21.5\n\nrequire (\n)\n",
			[]decl.Decl{{Product: "go", Version: "1.21.5", Source: decl.Source{File: "go.mod", Line: 3}}}},
		{"go directive with a comment", "go.mod", "go 1.26 // the oldest supported\n",
			[]decl.Decl{{Product: "go", Version: "1.26", Source: decl.Source{File: "go.mod", Line: 1}}}},
		// go.mod separates a directive from its arguments with any run of
		// spaces or tabs, and go itself reads all of them the same way.
		{"go directive after a tab", "go.mod", "module example.com/x\n\ngo\t1.16\n",
			[]decl.Decl{{Product: "go", Version: "1.16", Source: decl.Source{File: "go.mod", Line: 3}}}},
		{"go directive after several spaces", "go.mod", "go  1.16\n",
			[]decl.Decl{{Product: "go", Version: "1.16", Source: decl.Source{File: "go.mod", Line: 1}}}},
		// A comment after the version is nvm's own syntax, and pyenv,
		// nodenv and rbenv read the first word of a line and ignore the
		// rest, so the version is what comes first either way.
		{"trailing comment", ".nvmrc", "18 # legacy runtime\n",
			[]decl.Decl{{Product: "nodejs", Version: "18", Source: decl.Source{File: ".nvmrc", Line: 1}}}},
		{"trailing comment with no space", ".nvmrc", "18# legacy runtime\n",
			[]decl.Decl{{Product: "nodejs", Version: "18", Source: decl.Source{File: ".nvmrc", Line: 1}}}},
		{"trailing comment on a patch version", ".python-version", "3.9.10 # pinned for the C extension\n",
			[]decl.Decl{{Product: "python", Version: "3.9.10", Source: decl.Source{File: ".python-version", Line: 1}}}},
		// An .nvmrc may carry key=value pairs beside the one bare line that
		// names the version. nvm reads the bare line and nothing else.
		{"a setting beside the version", ".nvmrc", "flavor=iojs\n18\n",
			[]decl.Decl{{Product: "nodejs", Version: "18", Source: decl.Source{File: ".nvmrc", Line: 2}}}},
		{"a setting written with spaces", ".nvmrc", "flavor = iojs\n18\n",
			[]decl.Decl{{Product: "nodejs", Version: "18", Source: decl.Source{File: ".nvmrc", Line: 2}}}},
		{"pyenv names several", ".python-version", "3.9.10\n# a comment\n\n3.11.2\n",
			[]decl.Decl{
				{Product: "python", Version: "3.9.10", Source: decl.Source{File: ".python-version", Line: 1}},
				{Product: "python", Version: "3.11.2", Source: decl.Source{File: ".python-version", Line: 4}},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(tt.file, []byte(tt.data))
			if len(us) != 0 {
				t.Fatalf("unreadable = %+v; want none", us)
			}
			if len(ds) != len(tt.want) {
				t.Fatalf("Extract = %+v; want %+v", ds, tt.want)
			}
			for i := range ds {
				if ds[i] != tt.want[i] {
					t.Errorf("Extract[%d] = %+v; want %+v", i, ds[i], tt.want[i])
				}
			}
		})
	}
}

// TestExtractReported covers the lines that name something other than a
// version. Guessing at these is what would put a wrong date on the timeline,
// so each is handed back with a reason instead.
//
// nvm's lts/<codename> is not among them: see TestExtractCodename.
func TestExtractReported(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		data   string
		reason string
	}{
		{"bare lts", ".nvmrc", "lts\n", "names a moving target, not a version"},
		{"the newest lts", ".nvmrc", "lts/*\n", "names a moving target, not a version"},
		{"lts/latest", ".nvmrc", "lts/latest\n", "names a moving target, not a version"},
		{"node means newest", ".nvmrc", "node\n", "names a moving target, not a version"},
		{"latest", ".node-version", "latest\n", "names a moving target, not a version"},
		{"stable", ".ruby-version", "stable\n", "names a moving target, not a version"},
		{"system", ".python-version", "system\n", "defers to whatever is installed"},
		// tfenv writes three things that are not versions, and each of them
		// follows something rather than naming one.
		{"the newest terraform", ".terraform-version", "latest\n", "names a moving target, not a version"},
		{"the newest matching one", ".terraform-version", "latest:^1.5\n", "names the newest release matching a pattern, not a version"},
		{"whatever the configuration requires", ".terraform-version", "min-required\n", "defers to what the configuration requires"},
		{"anything else", ".python-version", "anaconda3-2021.05\n", "is not a version"},
		{"go directive is a word", "go.mod", "go tip\n", "is not a version"},
		// The comment falls away before the line is read, so what is left
		// is what gets the reason.
		{"moving target with a comment", ".nvmrc", "lts # the one we run\n",
			"names a moving target, not a version"},
		// A prefix from another file's conventions is not stripped, so this
		// stays unreadable rather than becoming Node.js 2.6.
		{"foreign prefix", ".nvmrc", "ruby-2.6\n", "is not a version"},
		// The key=value syntax is nvm's own, so the same line elsewhere is
		// a version that cannot be read rather than a setting.
		{"a pair in a file that has no settings", ".node-version", "flavor=iojs\n", "is not a version"},
		// Nothing before the = makes it the bare line rather than a pair,
		// which is how nvm reads it too.
		{"a pair with no key", ".nvmrc", "=iojs\n", "is not a version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(tt.file, []byte(tt.data))
			if len(ds) != 0 {
				t.Fatalf("Extract = %+v; want nothing on the timeline", ds)
			}
			if len(us) != 1 || us[0].Reason != tt.reason {
				t.Fatalf("unreadable = %+v; want one with reason %q", us, tt.reason)
			}
			// The file still says what the line is about, so the product
			// travels with the complaint.
			product, _ := Product(tt.file)
			if us[0].Product != product {
				t.Errorf("Product = %q; want %q", us[0].Product, product)
			}
		})
	}
}

func TestExtractNothing(t *testing.T) {
	for _, tt := range []struct{ name, file, data string }{
		{"unsupported file", "package.json", `{"engines":{"node":"14"}}`},
		{"empty", ".nvmrc", ""},
		{"comments only", ".python-version", "# nothing here\n\n"},
		{"an indented comment", ".nvmrc", "   # nothing here either\n"},
		{"settings and no version", ".nvmrc", "flavor=iojs\n"},
		{"go.mod without the directive", "go.mod", "module example.com/x\n"},
		// "going" starts with go but is not the directive, which needs the
		// space.
		{"go.mod near miss", "go.mod", "going 1.21\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(tt.file, []byte(tt.data))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

// TestExtractCodename: lts/hydrogen names Node.js 18 as surely as 18 does,
// because nvm writes there the codename endoflife.date publishes for that
// cycle. The word is handed on for the catalog to match, like the codename
// an image tag carries; lts/* and lts/latest are the newest of them and stay
// moving targets.
func TestExtractCodename(t *testing.T) {
	for _, tt := range []struct{ name, data, version string }{
		{"an lts codename", "lts/hydrogen\n", "hydrogen"},
		{"with a comment after it", "lts/iron # the one we run\n", "iron"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(".nvmrc", []byte(tt.data))
			if len(us) != 0 {
				t.Fatalf("unreadable = %+v; want none", us)
			}
			if len(ds) != 1 || ds[0].Product != "nodejs" || ds[0].Version != tt.version {
				t.Fatalf("Extract = %+v; want nodejs %s", ds, tt.version)
			}
		})
	}
}

// TestGoModReadsTheDirectiveAndNotTheToolchain: a go.mod may carry both, and
// only the go directive is a declaration this tool reads. The two are not a
// contradiction to resolve either — the directive is the floor the module
// supports and the toolchain names what to build with — so nothing here
// reconciles them.
func TestGoModReadsTheDirectiveAndNotTheToolchain(t *testing.T) {
	ds, us := Extract("go.mod", []byte("module example.com/x\n\ngo 1.16\n\ntoolchain go1.25.1\n"))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	want := []decl.Decl{{Product: "go", Version: "1.16", Source: decl.Source{File: "go.mod", Line: 3}}}
	if len(ds) != len(want) || ds[0] != want[0] {
		t.Fatalf("declarations = %+v; want %+v", ds, want)
	}
}

// TestExtractRubyDevelopment: a development build follows its branch on
// purpose, and says which implementation it is a build of.
func TestExtractRubyDevelopment(t *testing.T) {
	for _, tt := range []struct{ data, product, text string }{
		{"ruby-head\n", "ruby", "head"},
		{"jruby-head\n", "jruby", "head"},
	} {
		ds, us := Extract(".ruby-version", []byte(tt.data))
		want := decl.Unreadable{Source: decl.Source{File: ".ruby-version", Line: 1}, Product: tt.product, Text: tt.text,
			Reason: "names a development build, not a version", Moving: true}
		if len(ds) != 0 || len(us) != 1 || us[0] != want {
			t.Errorf("Extract(%q) = %+v, %+v; want %+v", tt.data, ds, us, want)
		}
	}
}
