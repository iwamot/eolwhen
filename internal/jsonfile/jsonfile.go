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
func Read(data []byte, objects, texts []string) []Member {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out []Member
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil
		}
		// A JSON object's key is a string by construction.
		name, _ := key.(string)
		isObject := slices.Contains(objects, name)
		if !isObject && !slices.Contains(texts, name) {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil
			}
			continue
		}
		read, ok := member(dec, data, name, isObject)
		if !ok {
			return nil
		}
		out = append(out, read...)
	}
	return out
}

// member reads one wanted top-level member. ok is false when the document
// ran out, and when the member did not hold what the caller said it would.
func member(dec *json.Decoder, data []byte, in string, isObject bool) (out []Member, ok bool) {
	// Taken before the member is read, so that an object spanning many
	// lines does not put its own name at the end of itself.
	start := lineAt(data, dec.InputOffset())
	t, err := dec.Token()
	if err != nil {
		return nil, false
	}
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
		key, err := dec.Token()
		if err != nil {
			return nil, false
		}
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
	// nothing else: a document that ran out mid-object has already been
	// given up on above. Stepping over it is what leaves the decoder on the
	// next member of the document.
	_, _ = dec.Token()
	return out, true
}

// lineAt is the line an offset falls on, counted from 1.
func lineAt(data []byte, offset int64) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
