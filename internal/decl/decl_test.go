package decl

import "testing"

func TestSourceString(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  Source
		want string
	}{
		// A file whose every line matters points at the line.
		{"a line", Source{File: "go.mod", Line: 3}, "go.mod:3"},
		// A file whose whole content is the declaration does not.
		{"the whole file", Source{File: ".nvmrc"}, ".nvmrc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.src.String(); got != tt.want {
				t.Errorf("String = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestUnreadableWhat(t *testing.T) {
	for _, tt := range []struct {
		name string
		u    Unreadable
		want string
	}{
		// A tool list names the tool apart from the version it gave it.
		{"a tool and its version", Unreadable{Product: "jq", Text: "latest"}, "jq latest"},
		// An image reference or a runner label is the whole of what was
		// written.
		{"text that names the software itself", Unreadable{Text: "ubuntu-latest"}, "ubuntu-latest"},
		// A manifest line that leaves the version to a lockfile named the
		// software and nothing else.
		{"a package and no version", Unreadable{Product: "sidekiq"}, "sidekiq"},
		// A file nothing could be read from has nothing to quote.
		{"nothing", Unreadable{}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.What(); got != tt.want {
				t.Errorf("What = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestDeclWhat(t *testing.T) {
	for _, tt := range []struct {
		name string
		d    Decl
		want string
	}{
		{"software and its version", Decl{Product: "python", Version: "3.7"}, "python 3.7"},
		// A gem line with no requirements leaves the version to the
		// lockfile, so there is none to show.
		{"a package and no version", Decl{Product: "sidekiq"}, "sidekiq"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.What(); got != tt.want {
				t.Errorf("What = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestSourceBefore(t *testing.T) {
	for _, tt := range []struct {
		name string
		a, b Source
		want bool
	}{
		// Files sort as paths, so a directory's contents stay together.
		{"an earlier file", Source{File: "Dockerfile"}, Source{File: "compose.yml"}, true},
		{"a later file", Source{File: "compose.yml"}, Source{File: "Dockerfile"}, false},
		// Lines sort as numbers: line 8 comes before line 62, which reading
		// them as text would get backwards.
		{"an earlier line", Source{File: "Dockerfile", Line: 8}, Source{File: "Dockerfile", Line: 62}, true},
		{"a later line", Source{File: "Dockerfile", Line: 62}, Source{File: "Dockerfile", Line: 8}, false},
		// A file whose whole content is the declaration has no line, and is
		// before nothing in it.
		{"the same place", Source{File: ".nvmrc"}, Source{File: ".nvmrc"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Before(tt.b); got != tt.want {
				t.Errorf("%s.Before(%s) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
