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
