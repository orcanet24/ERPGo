// graph.go — Phase 2: resolve relations and build both directions of every
// index. Nothing here is guessed: an edge is only emitted when one of the
// documented rules proves it, and the confidence of that proof travels with
// the edge.
package main

import (
	"sort"
	"strings"
)

// Confidence vocabulary (REGLA DE ORO, point 2).
const (
	ConfDirect    = "directa"    // same file/package: certainty
	ConfImport    = "import"     // resolved through an explicit import
	ConfStructure = "estructura" // name/receiver match: may be a false positive
)

// Edge is one proven relation.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"`  // call | import | implement | test
	Conf  string `json:"conf"`  // directa | import | estructura
	Scope string `json:"scope"` // file | func | type
}

// Graph is the resolved relation set plus its counters.
type Graph struct {
	Edges      []Edge
	Unresolved int
	Deep       int // selector depth >= 2, deliberately not resolved
	Resolved   int
	CallsTotal int
}

// pkgOf returns the import path recorded by the scanner for this file.
func pkgOf(f *File) string { return f.PkgPath }

// BuildGraph resolves every call site recorded by the scanner.
func BuildGraph(root string, s *Scan) *Graph {
	g := &Graph{}

	// Local identifier -> import path, for internal imports only.
	localImport := map[string]map[string]string{} // file id -> ident -> pkg path
	// Package -> type -> method names, and package -> interface -> methods.
	methodsOf := map[string]map[string][]string{}
	ifaceOf := map[string]map[string][]string{}
	pkgFiles := map[string][]string{}

	for i := range s.Files {
		f := &s.Files[i]
		pp := pkgOf(f)
		pkgFiles[pp] = append(pkgFiles[pp], f.ID)

		m := map[string]string{}
		for _, im := range f.Imports {
			if !strings.HasPrefix(im.Path, modulePathOf(root)) {
				continue
			}
			ident := im.Alias
			if ident == "" {
				ident = s.PkgOfPath[im.Path]
			}
			if ident == "" || ident == "." {
				continue
			}
			m[ident] = im.Path
		}
		localImport[f.ID] = m

		if methodsOf[pp] == nil {
			methodsOf[pp] = map[string][]string{}
			ifaceOf[pp] = map[string][]string{}
		}
		for _, fn := range f.Funcs {
			if fn.Recv == "" {
				continue
			}
			methodsOf[pp][fn.Recv] = append(methodsOf[pp][fn.Recv], fn.Name)
		}
		for _, ty := range f.Types {
			if ty.Kind == "interface" {
				ifaceOf[pp][ty.Name] = ty.IfaceMethods
			}
		}
	}
	for pp := range methodsOf {
		for k := range methodsOf[pp] {
			methodsOf[pp][k] = uniqueSorted(methodsOf[pp][k])
		}
	}

	// Index of method names across a package, to apply rule 4 (unique name).
	uniqueMethod := map[string]map[string]string{} // pkg -> method name -> type

	// Resolve call sites.
	fileOfFunc := map[string]string{}
	for i := range s.Files {
		f := &s.Files[i]
		for j := range f.Funcs {
			fileOfFunc[f.Funcs[j].ID] = f.ID
		}
	}

	for i := range s.Files {
		f := &s.Files[i]
		pp := pkgOf(f)
		for j := range f.Funcs {
			fn := &f.Funcs[j]
			for _, c := range fn.Calls {
				g.CallsTotal++
				target, conf, status := resolveCall(root, s, f, pp, fn, c, methodsOf)
				switch status {
				case "resolved":
					g.Resolved++
					g.Edges = append(g.Edges, Edge{From: fn.ID, To: target, Kind: "call", Conf: conf, Scope: "func"})
					if tf := findFunc(s, target); tf != nil {
						tf.CalledBy = append(tf.CalledBy, fn.ID)
					}
				case "deep":
					g.Deep++
					g.Unresolved++
				default:
					g.Unresolved++
				}
			}
		}
	}

	// Rule 4 support: method names unique inside their package.
	for i := range s.Files {
		f := &s.Files[i]
		pp := pkgOf(f)
		if uniqueMethod[pp] == nil {
			uniqueMethod[pp] = map[string]string{}
		}
		for _, fn := range f.Funcs {
			if fn.Recv == "" {
				continue
			}
			if _, seen := uniqueMethod[pp][fn.Name]; seen {
				uniqueMethod[pp][fn.Name] = "" // ambiguous
			} else {
				uniqueMethod[pp][fn.Name] = fn.Recv
			}
		}
	}

	// Second pass for rule 4 (needs the uniqueMethod index built above).
	for i := range s.Files {
		f := &s.Files[i]
		pp := pkgOf(f)
		for j := range f.Funcs {
			fn := &f.Funcs[j]
			for _, c := range fn.Calls {
				if c.Depth != 1 || c.Left == "" || c.Self {
					continue
				}
				if alreadyResolved(g, fn.ID, c.Name) {
					continue
				}
				if _, isImport := localImport[f.ID][c.Left]; isImport {
					continue
				}
				if recv, ok := uniqueMethod[pp][c.Name]; ok && recv != "" {
					// The name exists exactly once as a method in this package.
					if id := methodID(s, pp, recv, c.Name); id != "" {
						g.Resolved++
						g.Edges = append(g.Edges, Edge{From: fn.ID, To: id, Kind: "call", Conf: ConfStructure, Scope: "func"})
						if tf := findFunc(s, id); tf != nil {
							tf.CalledBy = append(tf.CalledBy, fn.ID)
						}
					}
				}
			}
		}
	}

	// Import edges: file -> every file of the imported package.
	for i := range s.Files {
		f := &s.Files[i]
		for _, im := range f.Imports {
			targets := pkgFiles[im.Path]
			if len(targets) == 0 {
				continue
			}
			for _, t := range targets {
				if t == f.ID {
					continue
				}
				g.Edges = append(g.Edges, Edge{From: f.ID, To: t, Kind: "import", Conf: ConfImport, Scope: "file"})
			}
		}
	}

	// File-level call edges (cross file only) for the mind map.
	seenPair := map[string]bool{}
	for _, e := range g.Edges {
		if e.Scope != "func" || e.Kind != "call" {
			continue
		}
		a, b := fileOfFunc[e.From], fileOfFunc[e.To]
		if a == "" || b == "" || a == b {
			continue
		}
		k := a + "\x00" + b
		if seenPair[k] {
			continue
		}
		seenPair[k] = true
		g.Edges = append(g.Edges, Edge{From: a, To: b, Kind: "call", Conf: e.Conf, Scope: "file"})
	}

	// Interface implementation, with ordered evidence.
	g.Edges = append(g.Edges, resolveImplements(root, s, methodsOf, ifaceOf)...)

	// Test coverage: a test covers a function it actually calls.
	for i := range s.Files {
		f := &s.Files[i]
		if !f.IsTestFile() {
			continue
		}
		for _, fn := range f.Funcs {
			if !fn.IsTest {
				continue
			}
			for _, e := range g.Edges {
				if e.Kind != "call" || e.Scope != "func" || e.From != fn.ID {
					continue
				}
				if tf := findFunc(s, e.To); tf != nil {
					tf.Covers = append(tf.Covers, fn.ID)
					g.Edges = append(g.Edges, Edge{From: fn.ID, To: tf.ID, Kind: "test", Conf: ConfDirect, Scope: "func"})
				}
			}
		}
	}

	// Reverse indexes.
	for i := range s.Files {
		f := &s.Files[i]
		for j := range f.Funcs {
			f.Funcs[j].CalledBy = uniqueSorted(f.Funcs[j].CalledBy)
			f.Funcs[j].Covers = uniqueSorted(f.Funcs[j].Covers)
		}
	}
	importedBy := map[string][]string{}
	for _, e := range g.Edges {
		if e.Kind == "import" {
			importedBy[e.To] = append(importedBy[e.To], e.From)
		}
	}
	for i := range s.Files {
		s.Files[i].ImportedBy = uniqueSorted(importedBy[s.Files[i].ID])
	}

	// Determinism: every slice sorted, edges totally ordered.
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Conf < b.Conf
	})
	g.Edges = dedupeEdges(g.Edges)
	return g
}

func dedupeEdges(in []Edge) []Edge {
	seen := map[string]bool{}
	var out []Edge
	for _, e := range in {
		k := e.Scope + "\x00" + e.Kind + "\x00" + e.From + "\x00" + e.To
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

func alreadyResolved(g *Graph, from, name string) bool {
	for _, e := range g.Edges {
		if e.Scope == "func" && e.Kind == "call" && e.From == from {
			if t := findEdgeTargetName(g, e.To); t == name {
				return true
			}
		}
	}
	return false
}

func findEdgeTargetName(g *Graph, id string) string {
	i := strings.LastIndex(id, "#")
	if i < 0 {
		return ""
	}
	return id[i+1:]
}

func findFunc(s *Scan, id string) *Func {
	for i := range s.Files {
		for j := range s.Files[i].Funcs {
			if s.Files[i].Funcs[j].ID == id {
				return &s.Files[i].Funcs[j]
			}
		}
	}
	return nil
}

func methodID(s *Scan, pkgPath, recv, name string) string {
	for i := range s.Files {
		f := &s.Files[i]
		if f.PkgPath != pkgPath {
			continue
		}
		for _, fn := range f.Funcs {
			if fn.Recv == recv && fn.Name == name {
				return fn.ID
			}
		}
	}
	return ""
}

func filePkgPathOf(s *Scan, f *File) string { return f.PkgPath }

// resolveCall applies the five documented rules in order.
func resolveCall(root string, s *Scan, f *File, pkgPath string, fn *Func, c CallSite, methodsOf map[string]map[string][]string) (target, conf, status string) {
	if c.Name == "" {
		return "", "", "unresolved"
	}

	// Rule 1: unqualified call resolved inside the same package.
	if c.Depth == 0 {
		for i := range s.Files {
			other := &s.Files[i]
			if other.PkgPath != pkgPath {
				continue
			}
			for _, of := range other.Funcs {
				if of.Recv == "" && of.Name == c.Name {
					return of.ID, ConfDirect, "resolved"
				}
			}
		}
		return "", "", "unresolved"
	}

	if c.Depth >= 2 {
		return "", "", "deep"
	}

	// Rule 2: left identifier is an internal import and the symbol exists.
	if im, ok := importPathOf(f, c.Left); ok {
		if id, ok := s.Symbols[im][c.Name]; ok {
			return id, ConfImport, "resolved"
		}
		return "", "", "unresolved"
	}

	// Rule 3: method on the receiver of the enclosing function.
	if c.Self && fn.Recv != "" {
		for i := range s.Files {
			other := &s.Files[i]
			if other.PkgPath != pkgPath {
				continue
			}
			for _, of := range other.Funcs {
				if of.Recv == fn.Recv && of.Name == c.Name {
					return of.ID, ConfStructure, "resolved"
				}
			}
		}
	}

	return "", "", "unresolved"
}

func importPathOf(f *File, ident string) (string, bool) {
	for _, im := range f.Imports {
		if !strings.HasPrefix(im.Path, f.Module) {
			continue
		}
		local := im.Alias
		if local == "" {
			local = im.Name
		}
		if local == ident {
			return im.Path, true
		}
	}
	return "", false
}

// resolveImplements orders the evidence: compile-time assert first, then a
// matching method set inside the same package. Co-import is never enough.
func resolveImplements(root string, s *Scan, methodsOf map[string]map[string][]string, ifaceOf map[string]map[string][]string) []Edge {
	var out []Edge
	typeID := map[string]map[string]string{} // pkg -> type name -> id
	for i := range s.Files {
		f := &s.Files[i]
		pp := pkgOf(f)
		if typeID[pp] == nil {
			typeID[pp] = map[string]string{}
		}
		for _, ty := range f.Types {
			typeID[pp][ty.Name] = ty.ID
		}
	}

	for i := range s.Files {
		f := &s.Files[i]
		pp := pkgOf(f)
		for j := range f.Types {
			ty := &f.Types[j]
			// Evidence 1: explicit compile-time assert.
			for _, iface := range ty.Asserts {
				if _, ok := ifaceOf[pp][iface]; !ok {
					continue
				}
				ty.Implements = append(ty.Implements, iface)
				out = append(out, Edge{From: ty.ID, To: typeID[pp][iface], Kind: "implement", Conf: ConfDirect, Scope: "type"})
			}
			if ty.Kind == "interface" {
				continue
			}
			// Evidence 2: method set matches, same package.
			have := map[string]bool{}
			for _, m := range methodsOf[pp][ty.Name] {
				have[m] = true
			}
			if len(have) == 0 {
				continue
			}
			for iface, want := range ifaceOf[pp] {
				if len(want) == 0 {
					continue
				}
				ok := true
				for _, w := range want {
					if !have[w] {
						ok = false
						break
					}
				}
				if !ok || contains(ty.Implements, iface) {
					continue
				}
				ty.Implements = append(ty.Implements, iface)
				out = append(out, Edge{From: ty.ID, To: typeID[pp][iface], Kind: "implement", Conf: ConfStructure, Scope: "type"})
			}
		}
	}
	for i := range s.Files {
		for j := range s.Files[i].Types {
			s.Files[i].Types[j].Implements = uniqueSorted(s.Files[i].Types[j].Implements)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
