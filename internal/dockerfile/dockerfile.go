// Package dockerfile reads the FROM lines of a Dockerfile. What an image
// reference names is the image package's answer; this one finds the
// references and keeps track of the stages.
package dockerfile

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/image"
)

// Matches reports whether a file name is a Dockerfile. Both the plain name
// and the suffixed variants that hold a second build — Dockerfile.dev,
// Dockerfile.ci — are read.
func Matches(name string) bool {
	return name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.")
}

// Extract reads every FROM in the file.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable) {
	var ds []decl.Decl
	var us []decl.Unreadable
	// A later stage may build on an earlier one by name. Those names are not
	// images, so they are collected as they are declared and skipped when
	// they come back around.
	stages := map[string]bool{}
	for i, raw := range strings.Split(string(data), "\n") {
		ref, stage, ok := from(raw)
		if !ok {
			continue
		}
		if stage != "" {
			stages[strings.ToLower(stage)] = true
		}
		if stages[strings.ToLower(ref)] || image.Skip(ref) {
			continue
		}
		src := decl.Source{File: file, Line: i + 1}
		product, version, reason := image.Read(ref)
		if reason != "" {
			us = append(us, decl.Unreadable{Source: src, Text: ref, Reason: reason})
			continue
		}
		ds = append(ds, decl.Decl{Product: product, Version: version, Source: src})
	}
	return ds, us
}

// from picks the image reference and the stage name out of a FROM line, or
// reports that the line is not one. The --platform flag is dropped: it says
// where the image runs, not which image it is.
func from(line string) (ref, stage string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
		return "", "", false
	}
	fields = fields[1:]
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return "", "", false
	}
	ref = fields[0]
	if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
		stage = fields[2]
	}
	return ref, stage, true
}
