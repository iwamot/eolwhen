// Package packagejson reads the packages a package.json depends on, the
// package manager it says the project is run with, and what it asks of the
// host it runs on.
//
// What a package name reaches is settled elsewhere. This package says only
// that the line named an npm package, and the catalog answers through the
// purls upstream publishes: `next` is Next.js because endoflife.date says
// pkg:npm/next, while the rest of a manifest is software it does not track.
// A registry anyone can publish to is not a namespace to read names out of,
// and upstream's own answer is the whole of what is known about a name.
//
// Two members are not dependencies at all. packageManager names the package
// manager the project is run with and the version of it, which corepack
// installs and uses; engines names what the project asks of its host. Both
// declare software the catalog answers to by name, the way a mise.toml entry
// and a .nvmrc do, rather than something the project imports.
//
// An engines requirement is usually >=18 or the like, which names a floor
// and no ceiling and so names no release cycle. One written closed — ^18, or
// 18.x — names one, and that is the Node.js the project runs on as squarely
// as a .nvmrc would say it.
package packagejson

import (
	"slices"
	"strings"

	"github.com/iwamot/eolwhen/internal/decl"
	"github.com/iwamot/eolwhen/internal/jsonfile"
	"github.com/iwamot/eolwhen/internal/span"
)

// Ecosystem is the purl type an npm package name belongs to.
const Ecosystem = "npm"

// Matches reports whether a file is a package.json.
func Matches(name string) bool { return name == "package.json" }

// depends are the members whose object holds the packages a project
// declares. What it needs to develop is declared as plainly as what it needs
// to run, and a development framework goes out of support the same way. A
// peer dependency names the version of a framework the project is built
// against and an optional one a package it uses where it is there; both name
// software with a calendar like any other.
var depends = []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"}

// engines is the member naming what a project asks of its host: the Node.js
// it runs on, and the package manager it is installed with. What it names is
// a runtime rather than a package, so the catalog answers it by name as it
// does a .nvmrc, and no purl is involved.
const engines = "engines"

// objects are the members read as a mapping of names to requirements.
var objects = slices.Concat(depends, []string{engines})

// hostDecides is what is said about a requirement on the host that names no
// one version. Not the lockfile: which Node.js a project runs on is whatever
// the machine has, and a range is the project saying what it will put up
// with rather than what it met.
const hostDecides = "names no single version here, so whatever the host has decides which one"

// manager is the member naming the package manager the project is run with,
// and notAManager what is said about a value that does not name one.
const (
	manager     = "packageManager"
	notAManager = "does not name a package manager and a version of it"
)

// Extract reads every package a package.json depends on, and its package
// manager.
func Extract(file string, data []byte) ([]decl.Decl, []decl.Unreadable, string) {
	members, skipped := jsonfile.Read(data, objects, []string{manager})
	if skipped != "" {
		return nil, nil, skipped
	}
	var ds []decl.Decl
	var us []decl.Unreadable
	for _, m := range members {
		src := decl.Source{File: file, Line: m.Line}
		switch m.In {
		case engines:
			// A runtime, which the catalog answers to by name as it does
			// for a .nvmrc. No purl is involved.
			allowed, ok := allows(m.Value)
			if !ok {
				us = append(us, decl.Unreadable{Source: src, Product: m.Name, Text: m.Value, Reason: hostDecides, Moving: true})
				continue
			}
			ds = append(ds, decl.Decl{Product: m.Name, Version: m.Value, Allows: allowed, Source: src})
		case manager:
			// A tool, answered by name for the same reason, and pinned
			// exactly by the field's own rule.
			name, version, ok := packageManager(m.Value)
			if !ok {
				us = append(us, decl.Unreadable{Source: src, Text: m.Value, Reason: notAManager})
				continue
			}
			ds = append(ds, decl.Decl{Product: name, Version: version, Source: src})
		default:
			allowed, ok := allows(m.Value)
			if !ok {
				us = append(us, decl.Unreadable{
					Source:    src,
					Ecosystem: Ecosystem,
					Product:   m.Name,
					Text:      m.Value,
					Reason:    "names no single version here, so the lockfile decides which one",
					Moving:    true,
				})
				continue
			}
			ds = append(ds, decl.Decl{
				Ecosystem: Ecosystem,
				Product:   m.Name,
				Version:   m.Value,
				Allows:    allowed,
				Source:    src,
			})
		}
	}
	return ds, us, ""
}

// packageManager reads the field, which writes the tool and its version as
// one string: pnpm@10.18.0. The name is left as it was written, the catalog
// answering to a name whatever case it is in.
//
// The version is exact by the field's own rule — corepack installs that one
// and no other — so there is no range to read here, and a value that is not
// a version is a line to go and look at rather than one a resolver settles.
// A hash may follow it, which says which download was meant and not which
// version.
func packageManager(text string) (name, version string, ok bool) {
	name, version, found := strings.Cut(text, "@")
	version, _, _ = strings.Cut(version, "+")
	if !found || name == "" || !span.Numeric(version) {
		return "", "", false
	}
	return name, version, true
}
