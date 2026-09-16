package toolfile

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"mise.toml", true},
		{".mise.toml", true},
		{".tool-versions", true},
		{"pyproject.toml", false},
		{"mise.lock", false},
		{"tool-versions", false},
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
	line    int
}

func check(t *testing.T, file, body string, ws ...want) {
	t.Helper()
	ds, us := Extract(file, []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != len(ws) {
		t.Fatalf("declarations = %+v; want %d", ds, len(ws))
	}
	for i, w := range ws {
		if ds[i].Product != w.product || ds[i].Version != w.version {
			t.Errorf("[%d] = %s %s; want %s %s", i, ds[i].Product, ds[i].Version, w.product, w.version)
		}
		if ds[i].Source.Line != w.line {
			t.Errorf("[%d] line = %d; want %d", i, ds[i].Source.Line, w.line)
		}
	}
}

func TestMiseToml(t *testing.T) {
	t.Run("a bare tool name", func(t *testing.T) {
		check(t, "mise.toml", "[tools]\ngo = \"1.27.1\"\n", want{"go", "1.27.1", 2})
	})
	t.Run("a leading v", func(t *testing.T) {
		check(t, "mise.toml", "[tools]\ngo = \"v1.27.1\"\n", want{"go", "1.27.1", 2})
	})
	t.Run("several versions of one tool", func(t *testing.T) {
		check(t, "mise.toml", "[tools]\nnode = [\"20\", \"22\"]\n", want{"node", "20", 2}, want{"node", "22", 2})
	})
	t.Run("a table with options beside the version", func(t *testing.T) {
		check(t, "mise.toml", "[tools]\npython = { version = \"3.13\", virtualenv = \".venv\" }\n", want{"python", "3.13", 2})
	})
	t.Run("in file order, not the map's", func(t *testing.T) {
		body := "[tools]\nzig = \"0.13\"\nbun = \"1.2\"\n"
		check(t, "mise.toml", body, want{"zig", "0.13", 2}, want{"bun", "1.2", 3})
	})
	// A tool written as its own table sits outside the [tools] lines, so its
	// line is not found; it comes after the ones that were, by name.
	t.Run("a tool whose line is not found comes last", func(t *testing.T) {
		body := "[tools.node]\nversion = \"20\"\n[tools.bun]\nversion = \"1.2\"\n[tools]\nzig = \"0.13\"\n"
		check(t, "mise.toml", body, want{"zig", "0.13", 6}, want{"bun", "1.2", 0}, want{"node", "20", 0})
	})
	t.Run("only the tools table is read for lines", func(t *testing.T) {
		body := "[settings]\ngo = \"decoy\"\n\n[tools]\ngo = \"1.27.1\"\n"
		check(t, "mise.toml", body, want{"go", "1.27.1", 5})
	})
	t.Run("a quoted key", func(t *testing.T) {
		check(t, "mise.toml", "[tools]\n\"go\" = \"1.27.1\"\n", want{"go", "1.27.1", 2})
	})
}

// TestBackendKeysAreSkipped: a key carrying a backend names a package in a
// registry anyone can publish to, where the name is not a promise about what
// the software is.
func TestBackendKeysAreSkipped(t *testing.T) {
	body := "[tools]\n" +
		"\"aqua:goreleaser/goreleaser\" = \"2.18.1\"\n" +
		"\"go:golang.org/x/vuln/cmd/govulncheck\" = \"v1.8.0\"\n" +
		"\"npm:typescript\" = \"5.9.0\"\n" +
		"node = \"24.21.0\"\n"
	check(t, "mise.toml", body, want{"node", "24.21.0", 5})
}

func TestToolVersions(t *testing.T) {
	t.Run("one per line", func(t *testing.T) {
		check(t, ".tool-versions", "python 3.9.10\nnodejs 14.19.0\n",
			want{"python", "3.9.10", 1}, want{"nodejs", "14.19.0", 2})
	})
	t.Run("several on one line, as asdf allows", func(t *testing.T) {
		check(t, ".tool-versions", "nodejs 14.19.0 18.0.0\n",
			want{"nodejs", "14.19.0", 1}, want{"nodejs", "18.0.0", 1})
	})
	t.Run("comments", func(t *testing.T) {
		check(t, ".tool-versions", "# the runtimes\nruby 2.6.2 # old\n", want{"ruby", "2.6.2", 2})
	})
	t.Run("below the directory", func(t *testing.T) {
		check(t, "sub/.tool-versions", "golang 1.16\n", want{"golang", "1.16", 1})
	})
}

// TestReported: the tool is carried apart from the version it was given, so
// that whether the catalog tracks the tool can still decide if the line is
// worth a word — a `jq = "latest"` is about jq before it is about latest.
func TestReported(t *testing.T) {
	for _, tt := range []struct{ name, file, body, product, text, reason string }{
		{"latest", "mise.toml", "[tools]\nnode = \"latest\"\n", "node", "latest", "names a moving target, not a version"},
		{"lts", "mise.toml", "[tools]\nnode = \"lts\"\n", "node", "lts", "names a moving target, not a version"},
		{"an lts codename", "mise.toml", "[tools]\nnode = \"lts-jod\"\n", "node", "lts-jod", "names a moving target, not a version"},
		{"system", ".tool-versions", "ruby system\n", "ruby", "system", "defers to whatever is installed"},
		{"a ref", "mise.toml", "[tools]\nnode = \"ref:main\"\n", "node", "ref:main", "leaves the version for mise to resolve"},
		{"a prefix", "mise.toml", "[tools]\ngo = \"prefix:1.27\"\n", "go", "prefix:1.27", "leaves the version for mise to resolve"},
		{"anything else", ".tool-versions", "python anaconda3-2021.05\n", "python", "anaconda3-2021.05", "is not a version"},
		{"a tool nobody dates", "mise.toml", "[tools]\njq = \"latest\"\n", "jq", "latest", "names a moving target, not a version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(tt.file, []byte(tt.body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			if len(us) != 1 || us[0].Product != tt.product || us[0].Text != tt.text || us[0].Reason != tt.reason {
				t.Fatalf("unreadable = %+v; want %s %s %q", us, tt.product, tt.text, tt.reason)
			}
		})
	}
}

func TestNothing(t *testing.T) {
	for _, tt := range []struct{ name, file, body string }{
		{"empty toml", "mise.toml", ""},
		{"no tools table", "mise.toml", "min_version = \"2026.9.5\"\n"},
		{"an empty tools table", "mise.toml", "[tools]\n"},
		// mise reports a broken config better than this tool can.
		{"not TOML", "mise.toml", "[tools\ngo = \n"},
		{"a version of a shape mise does not use", "mise.toml", "[tools]\ngo = 127\n"},
		{"a table with no version in it", "mise.toml", "[tools]\ngo = { virtualenv = \".venv\" }\n"},
		{"empty tool-versions", ".tool-versions", ""},
		{"comments only", ".tool-versions", "# nothing here\n\n"},
		{"a name with no version", ".tool-versions", "python\n"},
		{"a backend in tool-versions", ".tool-versions", "npm:typescript 5.9.0\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us := Extract(tt.file, []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

func TestSourceIsThePath(t *testing.T) {
	ds, _ := Extract("tools/mise.toml", []byte("[tools]\ngo = \"1.27.1\"\n"))
	if len(ds) != 1 || ds[0].Source != (decl.Source{File: "tools/mise.toml", Line: 2}) {
		t.Errorf("Source = %+v; want tools/mise.toml:2", ds)
	}
}
