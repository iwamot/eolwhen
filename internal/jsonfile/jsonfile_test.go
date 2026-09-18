package jsonfile

import "testing"

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
	got := Read([]byte(doc), []string{"dependencies", "devDependencies"}, []string{"packageManager"})
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
	if got := Read([]byte(doc), nil, nil); got != nil {
		t.Errorf("Read = %+v; want nothing", got)
	}
}

func TestReadsNothing(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"empty", ``},
		{"not JSON yet", `{"dependencies": {`},
		{"not an object at the top", `["dependencies"]`},
		{"an object where a string was wanted", `{"packageManager": {"name": "pnpm"}}`},
		{"a string where an object was wanted", `{"dependencies": "next"}`},
		{"a list where an object was wanted", `{"dependencies": ["next"]}`},
		{"a value that is not a string", `{"dependencies": {"next": 13}}`},
		{"an unnamed key", `{"dependencies": {"": "^1.0.0"}}`},
		{"nothing wanted is there", `{"name": "demo"}`},
		// Half a document is not half a set of members, so what was read
		// before it ran out is given up on with the rest.
		{"cut short inside a key", `{"na`},
		{"cut short after a key", `{"name"`},
		{"cut short before a value", `{"name": "demo", "dependencies"`},
		{"cut short inside a wanted object", `{"dependencies": {"next"`},
		{"cut short before a wanted object closes", `{"dependencies": {"next": "13"`},
		{"cut short after a wanted object closed", `{"dependencies": {"next": "13"}, "name"`},
		{"cut short inside a member nobody wanted", `{"scripts": {"build"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Read([]byte(tt.body), []string{"dependencies"}, []string{"packageManager"})
			if len(got) != 0 {
				t.Errorf("Read = %+v; want nothing", got)
			}
		})
	}
}
