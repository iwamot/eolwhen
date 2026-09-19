package jsonfile

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

const doc = `{
    "name": "demo",
    "packageManager": "pnpm@10.18.0",
    "dependencies": {
        "next": "~13.4.1",
        "React": "18.x"
    },
    "scripts": {"build": "next build"},
    "workspaces": ["packages/*"],
    "devDependencies": {
        "eslint": "~8.57.0"
    }
}`

func TestRead(t *testing.T) {
	got, skipped := Read([]byte(doc), []string{"dependencies", "devDependencies"}, []string{"packageManager"})
	if skipped != "" {
		t.Fatalf("skipped = %q; want the document to be read", skipped)
	}
	want := []Member{
		// A member holding a string of its own carries its name in both,
		// and the line it sits on is its own.
		{In: "packageManager", Name: "packageManager", Value: "pnpm@10.18.0", Line: 3},
		// A key is the document's own spelling, whatever a registry would
		// make of it.
		{In: "dependencies", Name: "next", Value: "~13.4.1", Line: 5},
		{In: "dependencies", Name: "React", Value: "18.x", Line: 6},
		{In: "devDependencies", Name: "eslint", Value: "~8.57.0", Line: 11},
	}
	if len(got) != len(want) {
		t.Fatalf("Read = %+v; want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %+v; want %+v", i, got[i], w)
		}
	}
}

// A member nobody asked for is stepped over whole, however much it holds.
func TestReadStepsOverTheRest(t *testing.T) {
	got, skipped := Read([]byte(doc), nil, nil)
	if got != nil || skipped != "" {
		t.Errorf("Read = %+v, %q; want nothing and no reason", got, skipped)
	}
}

// Nothing is read from any of these, and they are not one answer. A file
// that is not JSON, and one that is JSON and does not hold what it is read
// for, are each short of a manifest in a way the reader can go and see; a
// manifest that simply declares nothing is complete, and saying so is the
// whole point of telling them apart.
func TestReadsNothing(t *testing.T) {
	for _, tt := range []struct{ name, body, skipped string }{
		{"empty", ``, decl.NotJSON},
		{"not JSON yet", `{"dependencies": {`, decl.NotJSON},
		{"not an object at the top", `["dependencies"]`, decl.Shape},
		{"an object where a string was wanted", `{"packageManager": {"name": "pnpm"}}`, decl.Shape},
		{"a string where an object was wanted", `{"dependencies": "next"}`, decl.Shape},
		{"a list where an object was wanted", `{"dependencies": ["next"]}`, decl.Shape},
		{"a value that is not a string", `{"dependencies": {"next": 13}}`, decl.Shape},
		// The document is whole and holds what it is read for; one entry of
		// it has no name to look up, which is that entry's business and not
		// the file's.
		{"an unnamed key", `{"dependencies": {"": "^1.0.0"}}`, ""},
		{"nothing wanted is there", `{"name": "demo"}`, ""},
		// Half a document is not half a set of members, so what was read
		// before it ran out is given up on with the rest.
		{"cut short inside a key", `{"na`, decl.NotJSON},
		{"cut short after a key", `{"name"`, decl.NotJSON},
		{"cut short before a value", `{"name": "demo", "dependencies"`, decl.NotJSON},
		{"cut short inside a wanted object", `{"dependencies": {"next"`, decl.NotJSON},
		{"cut short before a wanted object closes", `{"dependencies": {"next": "13"`, decl.NotJSON},
		{"cut short after a wanted object closed", `{"dependencies": {"next": "13"}, "name"`, decl.NotJSON},
		{"cut short inside a member nobody wanted", `{"scripts": {"build"`, decl.NotJSON},
		// A file holding a document and then something else is not a
		// document, whatever the part that parses holds.
		{"something after the document", `{"dependencies": {"next": "13"}} garbage`, decl.NotJSON},
		{"a second document after the first", `{"dependencies": {"next": "13"}} {}`, decl.NotJSON},
		{"a trailing comma", `{"dependencies": {"next": "13"},}`, decl.NotJSON},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, skipped := Read([]byte(tt.body), []string{"dependencies"}, []string{"packageManager"})
			if skipped != tt.skipped {
				t.Errorf("skipped = %q; want %q", skipped, tt.skipped)
			}
			if len(got) != 0 {
				t.Errorf("Read = %+v; want nothing", got)
			}
		})
	}
}
