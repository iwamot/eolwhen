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
		{"go.mod", "go", true},
		{"package.json", "", false},
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
		{"leading v", ".nvmrc", "v14.19.0\n",
			[]decl.Decl{{Product: "nodejs", Version: "14.19.0", Source: decl.Source{File: ".nvmrc", Line: 1}}}},
		{"rvm prefix", ".ruby-version", "ruby-2.6.2\n",
			[]decl.Decl{{Product: "ruby", Version: "2.6.2", Source: decl.Source{File: ".ruby-version", Line: 1}}}},
		{"node-version prefix", ".node-version", "node-18.0.0\n",
			[]decl.Decl{{Product: "nodejs", Version: "18.0.0", Source: decl.Source{File: ".node-version", Line: 1}}}},
		{"go directive", "go.mod", "module example.com/x\n\ngo 1.21.5\n\nrequire (\n)\n",
			[]decl.Decl{{Product: "go", Version: "1.21.5", Source: decl.Source{File: "go.mod", Line: 3}}}},
		{"go directive with a comment", "go.mod", "go 1.26 // the oldest supported\n",
			[]decl.Decl{{Product: "go", Version: "1.26", Source: decl.Source{File: "go.mod", Line: 1}}}},
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
func TestExtractReported(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		data   string
		reason string
	}{
		{"moving target", ".nvmrc", "lts/hydrogen\n", "names a moving target, not a version"},
		{"bare lts", ".nvmrc", "lts\n", "names a moving target, not a version"},
		{"node means newest", ".nvmrc", "node\n", "names a moving target, not a version"},
		{"latest", ".node-version", "latest\n", "names a moving target, not a version"},
		{"stable", ".ruby-version", "stable\n", "names a moving target, not a version"},
		{"system", ".python-version", "system\n", "defers to whatever is installed"},
		{"anything else", ".python-version", "anaconda3-2021.05\n", "is not a version"},
		{"go directive is a word", "go.mod", "go tip\n", "is not a version"},
		// A prefix from another file's conventions is not stripped, so this
		// stays unreadable rather than becoming Node.js 2.6.
		{"foreign prefix", ".nvmrc", "ruby-2.6\n", "is not a version"},
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
