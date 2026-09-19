// Package compose reads the image: of each Compose service.
//
// A service's image is the same kind of declaration as a Dockerfile's FROM —
// this is what will be running — so it is read the same way, and the same
// rule about official images applies.
package compose

import (
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/image"
	"github.com/iwamot/eolwhen/internal/yamlfile"
)

// Matches reports whether a file name is a Compose file, in either the
// current spelling or the older one, and with or without the profile suffix
// an override file carries.
func Matches(name string) bool {
	for _, ext := range []string{".yml", ".yaml"} {
		base, ok := strings.CutSuffix(name, ext)
		if !ok {
			continue
		}
		stem, _, _ := strings.Cut(base, ".")
		if stem == "compose" || stem == "docker-compose" {
			return true
		}
	}
	return false
}

// Extract reads the image of every service.
//
// Only what sits under services: is read. A Compose file keeps templates in
// extension fields — x-defaults and the like — and a service pulls one in
// with a merge key, so reading every mapping that has an image: would report
// a template the file never applies, and would miss that the service which
// does apply it overrode the image.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable, string) {
	parsed, skipped := yamlfile.Parse(data)
	if skipped != "" {
		return nil, nil, skipped
	}
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, root := range parsed.Roots() {
		services, ok := yamlfile.Find(root, "services")
		if !ok {
			continue
		}
		byName, ok := services.Mapping()
		if !ok {
			continue
		}
		for _, svc := range byName {
			d, u := service(file, svc)
			ds = append(ds, d...)
			us = append(us, u...)
		}
	}
	return ds, us, ""
}

// service reads one service's image. A service that declares nothing this
// tool can speak about comes back with neither a declaration nor a
// complaint, and one whose tag names the distribution it was built on comes
// back with a declaration for each.
func service(file string, svc yamlfile.Entry) ([]decl.Decl, []decl.Unreadable) {
	entries, ok := svc.Mapping()
	if !ok {
		return nil, nil
	}
	// A service with a build: names, in image:, what to call what it builds.
	// That is the output, not the base it stands on, and the base is in the
	// Dockerfile the build points at.
	if _, builds := yamlfile.Find(entries, "build"); builds {
		return nil, nil
	}
	e, ok := yamlfile.Find(entries, "image")
	if !ok {
		return nil, nil
	}
	ref, ok := e.Scalar()
	if !ok || image.Skip(ref) {
		return nil, nil
	}
	src := decl.Source{File: file, Line: e.Line()}
	ns, reason, moving := image.Read(ref)
	if reason != "" {
		return nil, []decl.Unreadable{{Source: src, Text: ref, Reason: reason, Moving: moving}}
	}
	ds := make([]decl.Decl, 0, len(ns))
	for _, n := range ns {
		ds = append(ds, decl.Decl{Product: n.Product, Version: n.Version, Source: src})
	}
	return ds, nil
}
