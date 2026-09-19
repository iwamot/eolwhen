// Package jsonfile reads the members of a JSON manifest, keeping the line
// each one sits on.
//
// A manifest is read as tokens rather than decoded into a map, because a map
// has no line numbers in it and the line is what a reader is sent to. What
// each member means is the caller's to say: this only finds them.
package jsonfile

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/iwamot/eolwhen/internal/decl"
)

// Member is one member of the document: which top-level member it came from,
// the name it was given, the string it holds, and the line it sits on.
//
// In is what a caller tells one kind of member from another. A member read
// out of an object carries that object's name in In and its own key in
// Name; one that is a string of its own carries its own name in both.
//
// A name is the document's, spelled as the document spelled it. What a
// registry makes of the spelling is the caller's to know.
type Member struct {
	In    string
	Name  string
	Value string
	Line  int
}

// Read walks the document once. objects names the top-level members to read
// as a mapping of names to strings, one Member per entry; texts names the
// ones to read as a string of their own, one Member each.
//
// A member that is not the shape its list says leaves the whole document
// unreadable, as one that does not parse does. Reading nothing at all is the
// same answer an unparsable Compose file or mise.toml gets: half a file is
// not half a set of declarations, and the tool that owns it says what is
// wrong with it better than this one can.
//
// skipped is why nothing was read, and empty when the document was read. It
// is what keeps a manifest that is not JSON yet apart from one that is JSON
// and declares nothing, which are the same empty list and not the same
// answer.
func Read(data []byte, objects, texts []string) (members []Member, skipped string) {
	// The walk below stops where the top-level object closes and never
	// looks past it, so what follows a document goes unseen, and a
	// document that runs out mid-object leaves it to the decoder whether
	// anything says so. Reading the whole of it first is what makes a
	// file that is not one document unreadable here rather than wherever
	// a particular Go release happens to notice. It is also why nothing
	// below reads an error from the decoder: every token is there, and
	// what is left to find is the shape a member holds.
	if !json.Valid(data) {
		return nil, decl.NotJSON
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	// A document that is whole and is not an object is JSON and is still
	// not a manifest, which is the other way to hold nothing this reads
	// for.
	if t, _ := dec.Token(); t != json.Delim('{') {
		return nil, decl.Shape
	}
	var out []Member
	for dec.More() {
		key, _ := dec.Token()
		// A JSON object's key is a string by construction.
		name, _ := key.(string)
		isObject := slices.Contains(objects, name)
		if !isObject && !slices.Contains(texts, name) {
			var skip json.RawMessage
			_ = dec.Decode(&skip)
			continue
		}
		read, ok := member(dec, data, name, isObject)
		if !ok {
			return nil, decl.Shape
		}
		out = append(out, read...)
	}
	return out, ""
}

// member reads one wanted top-level member. ok is false when the member did
// not hold what the caller said it would, the document itself being whole
// by the time anything here reads it.
func member(dec *json.Decoder, data []byte, in string, isObject bool) (out []Member, ok bool) {
	// Taken before the member is read, so that an object spanning many
	// lines does not put its own name at the end of itself.
	start := lineAt(data, dec.InputOffset())
	t, _ := dec.Token()
	if !isObject {
		s, isText := t.(string)
		if !isText {
			return nil, false
		}
		return []Member{{In: in, Name: in, Value: s, Line: start}}, true
	}
	if t != json.Delim('{') {
		return nil, false
	}
	for dec.More() {
		key, _ := dec.Token()
		name, _ := key.(string)
		line := lineAt(data, dec.InputOffset())
		var value string
		if err := dec.Decode(&value); err != nil {
			return nil, false
		}
		// A name is what has to be there to be looked up, and an empty one
		// is not that.
		if name != "" {
			out = append(out, Member{In: in, Name: name, Value: value, Line: line})
		}
	}
	// The loop ends at the object's closing brace, and by then it can be
	// nothing else: the document was whole before any of it was read.
	// Stepping over it is what leaves the decoder on the next member of
	// the document.
	_, _ = dec.Token()
	return out, true
}

// lineAt is the line an offset falls on, counted from 1.
func lineAt(data []byte, offset int64) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
