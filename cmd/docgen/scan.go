// scan.go — Phase 1: extract facts from the repository.
//
// The scanner never interprets intent. It records what the syntax says:
// files, packages, imports, functions, types, call sites. Everything that
// requires judgement lives in the overlay (overlay.go) instead.
package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Import records one import clause of a file.
type Import struct {
	Path  string `json:"path"`  // import path as written
	Name  string `json:"name"`  // package name as declared by the target package
	Alias string `json:"alias"` // local identifier used in this file ("" = package name)
}

// CallSite is one CallExpr found inside a function body, with enough
// information to be resolved later by graph.go.
type CallSite struct {
	Name  string `json:"name"`  // final selector: M in a.b.M()
	Left  string `json:"left"`  // leftmost identifier: a in a.b.M()
	Depth int    `json:"depth"` // selector chain length (0 = unqualified)
	Self  bool   `json:"self"`  // left == receiver of the enclosing function
}

// Func is a function or method declaration.
type Func struct {
	ID         string     `json:"id"`
	File       string     `json:"file"`
	Name       string     `json:"name"`
	Recv       string     `json:"recv"` // "" for plain functions
	Sig        string     `json:"sig"`
	Start      int        `json:"start"`
	End        int        `json:"end"`
	Exported   bool       `json:"exported"`
	Doc        string     `json:"doc"`
	Branches   int        `json:"branches"`
	IsTest     bool       `json:"isTest"`
	Calls      []CallSite `json:"calls"`
	CalledBy   []string   `json:"calledBy"`
	Implements []string   `json:"implements"`
	Covers     []string   `json:"covers"` // test funcs that call this one
	Curated    bool       `json:"curated"`
	What       string     `json:"what"`
	How        []string   `json:"how"`
	Relations  string     `json:"relations"`
}

// Type is a declared named type.
type Type struct {
	ID           string   `json:"id"`
	File         string   `json:"file"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // struct | interface | alias | other
	Doc          string   `json:"doc"`
	Methods      []string `json:"methods"` // func ids declared on this receiver
	Asserts      []string `json:"asserts"` // `var _ Iface = (*T)(nil)` evidence
	Implements   []string `json:"implements"`
	Implementors []string `json:"implementors"`
	IfaceMethods []string `json:"ifaceMethods"` // method names declared by an interface
	Curated      bool     `json:"curated"`
	How          string   `json:"how"`
	Relations    string   `json:"relations"`
}

// File is one scanned file and everything declared in it.
type File struct {
	ID         string   `json:"id"` // repo-relative path, slash separated
	Path       string   `json:"path"`
	Segment    string   `json:"segment"`
	Pkg        string   `json:"pkg"`
	PkgPath    string   `json:"pkgPath"` // full import path ("" for non-Go)
	Module     string   `json:"module"`
	Kind       string   `json:"kind"` // code | test | migración | plantilla | doc | script | config
	Lines      int      `json:"lines"`
	CodeLines  int      `json:"codeLines"`
	Doc        string   `json:"doc"`
	Imports    []Import `json:"imports"`
	Funcs      []Func   `json:"funcs"`
	Types      []Type   `json:"types"`
	Rules      []string `json:"rules"`
	ImportedBy []string `json:"importedBy"`
	Curated    bool     `json:"curated"`
	Role       string   `json:"role"`
	How        string   `json:"how"`
	Relations  string   `json:"relations"`
	Watchouts  string   `json:"watchouts"`
}

// IsTestFile reports whether this file is a Go test.
func (f *File) IsTestFile() bool { return f.Kind == "test" }

// Scan holds the raw output of the parser plus the indexes graph.go needs.
type Scan struct {
	Files       []File
	PkgOfPath   map[string]string            // import path -> package name (as declared)
	Symbols     map[string]map[string]string // import path -> symbol name -> func id
	TypesByPath map[string]map[string]string // import path -> type name -> type id
	Total       int                          // files seen on disk
	Excluded    int                          // files deliberately not scanned
}

var includeExt = map[string]string{
	".go":   "code",
	".sql":  "migración",
	".html": "plantilla",
	".md":   "doc",
	".bat":  "script",
	".mod":  "config",
	".sum":  "config",
}

// skipDirs are directories that never carry source we document. Their file
// count is reported as "excluded" so the manual stays honest about coverage.
var skipDirs = map[string]bool{
	".git":         true,
	"backup":       true,
	"uploads":      true,
	"node_modules": true,
	".freebuff":    true,
	"static":       true, // legacy PHP-era assets, served verbatim
	"php_original": true, // reference copy of the old PHP tree
	"data":         true, // runtime database directory
	"developer":    true, // this manual's own output directory
}

var ruleRe = regexp.MustCompile(`\b(?:REG|RULE)-\d{1,2}\b`)

// ScanRepo walks root and parses every included file.
func ScanRepo(root, outDir string) (*Scan, error) {
	s := &Scan{
		PkgOfPath:   map[string]string{},
		Symbols:     map[string]map[string]string{},
		TypesByPath: map[string]map[string]string{},
	}

	var paths []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] || (outDir != "" && strings.HasPrefix(rel+"/", outDir+"/")) {
				// Out of scope, but counted: hiding what we do not document
				// would make the coverage number a lie.
				n := countFiles(p)
				s.Excluded += n
				s.Total += n
				return filepath.SkipDir
			}
			return nil
		}
		s.Total++
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if _, ok := includeExt[ext]; !ok {
			s.Excluded++
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	// Pass A: package clauses, so imports can be resolved by package NAME
	// (never by the last element of the import path).
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	pkgPathOf := map[string]string{}
	for _, rel := range paths {
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		f, perr := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil,
			parser.ParseComments|parser.SkipObjectResolution)
		if perr != nil {
			// A file that does not parse is still listed, just without symbols.
			continue
		}
		parsed[rel] = f
		impPath := modulePathOf(root) + "/" + dirOf(rel)
		if dirOf(rel) == "." {
			impPath = modulePathOf(root)
		}
		pkgPathOf[rel] = impPath
		s.PkgOfPath[impPath] = f.Name.Name
	}

	// Pass B: full extraction.
	for _, rel := range paths {
		fe := File{ID: rel, Path: rel, Kind: includeExt[strings.ToLower(filepath.Ext(rel))],
			Module: modulePathOf(root), PkgPath: pkgPathOf[rel]}
		if strings.HasSuffix(rel, "_test.go") {
			fe.Kind = "test"
		}
		content, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if rerr != nil {
			return nil, rerr
		}
		text := string(content)
		fe.Lines = strings.Count(text, "\n") + 1
		if strings.HasSuffix(text, "\n") {
			fe.Lines--
		}
		fe.CodeLines = countCodeLines(text)
		fe.Rules = uniqueSorted(ruleRe.FindAllString(text, -1))

		if af, ok := parsed[rel]; ok {
			fe.Pkg = af.Name.Name
			if af.Doc != nil {
				fe.Doc = cleanDoc(af.Doc.Text())
			}
			fe.Imports = extractImports(af)
			for k := range fe.Imports {
				fe.Imports[k].Name = s.PkgOfPath[fe.Imports[k].Path]
			}
			fe.Funcs, fe.Types = extractDecls(af, fset, rel)
		}
		s.Files = append(s.Files, fe)
	}

	// Indexes: import path -> symbol.
	for i := range s.Files {
		fe := &s.Files[i]
		if fe.Pkg == "" {
			continue
		}
		impPath := fe.PkgPath
		if s.Symbols[impPath] == nil {
			s.Symbols[impPath] = map[string]string{}
			s.TypesByPath[impPath] = map[string]string{}
		}
		for _, fn := range fe.Funcs {
			if fn.Recv != "" {
				continue // methods are resolved by rules 3 and 4, not by import
			}
			s.Symbols[impPath][fn.Name] = fn.ID
		}
		for _, ty := range fe.Types {
			s.TypesByPath[impPath][ty.Name] = ty.ID
		}
	}
	return s, nil
}

// countFiles returns the number of regular files under dir, used to report
// how much of the repository the manual deliberately leaves out.
func countFiles(dir string) int {
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func dirOf(rel string) string {
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return "."
	}
	return rel[:i]
}

var moduleCache = map[string]string{}

func modulePathOf(root string) string {
	if v, ok := moduleCache[root]; ok {
		return v
	}
	v := "module"
	if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "module ") {
				v = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}
	moduleCache[root] = v
	return v
}

func extractImports(f *ast.File) []Import {
	var out []Import
	for _, im := range f.Imports {
		p := strings.Trim(im.Path.Value, `"`)
		imp := Import{Path: p}
		if im.Name != nil {
			imp.Alias = im.Name.Name
		}
		out = append(out, imp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func extractDecls(f *ast.File, fset *token.FileSet, rel string) ([]Func, []Type) {
	var funcs []Func
	var types []Type

	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			fn := Func{
				File:     rel,
				Name:     decl.Name.Name,
				Exported: ast.IsExported(decl.Name.Name),
				Start:    fset.Position(decl.Pos()).Line,
				End:      fset.Position(decl.End()).Line,
			}
			if decl.Doc != nil {
				fn.Doc = cleanDoc(decl.Doc.Text())
			}
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				fn.Recv = recvName(decl.Recv.List[0].Type)
			}
			fn.ID = funcID(rel, fn.Recv, fn.Name)
			fn.Sig = funcSig(decl, fset)
			fn.IsTest = strings.HasSuffix(rel, "_test.go") && strings.HasPrefix(fn.Name, "Test")
			fn.Branches = countBranches(decl.Body)
			fn.Calls = collectCalls(decl)
			funcs = append(funcs, fn)

		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				continue
			}
			for _, spec := range decl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				ty := Type{
					ID:   rel + "#" + ts.Name.Name,
					File: rel,
					Name: ts.Name.Name,
					Kind: typeKind(ts.Type),
				}
				if ts.Doc != nil {
					ty.Doc = cleanDoc(ts.Doc.Text())
				} else if decl.Doc != nil {
					ty.Doc = cleanDoc(decl.Doc.Text())
				}
				if it, ok := ts.Type.(*ast.InterfaceType); ok && it.Methods != nil {
					for _, m := range it.Methods.List {
						for _, n := range m.Names {
							ty.IfaceMethods = append(ty.IfaceMethods, n.Name)
						}
					}
					ty.IfaceMethods = uniqueSorted(ty.IfaceMethods)
				}
				types = append(types, ty)
			}
		}
	}

	// Contract asserts: `var _ Iface = (*T)(nil)` and equivalents.
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || vs.Names[0].Name != "_" || len(vs.Values) == 0 {
				continue
			}
			iface := exprName(vs.Type)
			if iface == "" {
				continue
			}
			recv := recvNameFromValue(vs.Values[0])
			for i := range types {
				if recv != "" && types[i].Name == recv {
					types[i].Asserts = append(types[i].Asserts, iface)
				}
			}
		}
	}

	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].Start != funcs[j].Start {
			return funcs[i].Start < funcs[j].Start
		}
		return funcs[i].ID < funcs[j].ID
	})
	sort.Slice(types, func(i, j int) bool { return types[i].ID < types[j].ID })
	return funcs, types
}

func funcID(rel, recv, name string) string {
	if recv == "" {
		return rel + "#" + name
	}
	return rel + "#" + recv + "." + name
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

func recvNameFromValue(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.CallExpr:
		// (*T)(nil): the receiver type is the callee, not an argument.
		return recvNameFromValue(t.Fun)
	case *ast.UnaryExpr:
		return recvNameFromValue(t.X)
	case *ast.ParenExpr:
		return recvNameFromValue(t.X)
	case *ast.StarExpr:
		return recvName(t.X)
	}
	return recvName(e)
}

func exprName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

func typeKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.Ident, *ast.SelectorExpr:
		return "alias"
	}
	return "other"
}

// funcSig renders "Recv Name(params) results" without the func keyword.
func funcSig(d *ast.FuncDecl, fset *token.FileSet) string {
	var b strings.Builder
	if d.Recv != nil {
		b.WriteString("(")
		writeFields(&b, d.Recv, fset)
		b.WriteString(") ")
	}
	b.WriteString(d.Name.Name)
	b.WriteString("(")
	writeFields(&b, d.Type.Params, fset)
	b.WriteString(")")
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		b.WriteString(" ")
		if len(d.Type.Results.List) == 1 && len(d.Type.Results.List[0].Names) == 0 {
			writeExpr(&b, d.Type.Results.List[0].Type, fset)
		} else {
			b.WriteString("(")
			writeFields(&b, d.Type.Results, fset)
			b.WriteString(")")
		}
	}
	return b.String()
}

func writeFields(b *strings.Builder, fl *ast.FieldList, fset *token.FileSet) {
	if fl == nil {
		return
	}
	for i, f := range fl.List {
		if i > 0 {
			b.WriteString(", ")
		}
		for j, n := range f.Names {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString(n.Name)
		}
		if len(f.Names) > 0 {
			b.WriteString(" ")
		}
		writeExpr(b, f.Type, fset)
	}
}

func writeExpr(b *strings.Builder, e ast.Expr, fset *token.FileSet) {
	if e == nil {
		return
	}
	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, e); err == nil {
		b.WriteString(strings.ReplaceAll(buf.String(), "\n", " "))
	}
}

func countBranches(body *ast.BlockStmt) int {
	if body == nil {
		return 0
	}
	n := 0
	ast.Inspect(body, func(x ast.Node) bool {
		switch t := x.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			n++
		case *ast.CaseClause:
			if t.List != nil {
				n++
			}
		case *ast.BinaryExpr:
			if t.Op == token.LAND || t.Op == token.LOR {
				n++
			}
		}
		return true
	})
	return n
}

// collectCalls records every CallExpr in the function body with the data the
// graph phase needs. Nested function literals are attributed to the outer
// function: they are still code that runs inside it.
func collectCalls(d *ast.FuncDecl) []CallSite {
	self := ""
	if d.Recv != nil && len(d.Recv.List) > 0 {
		r := d.Recv.List[0]
		if len(r.Names) > 0 {
			self = r.Names[0].Name
		} else {
			self = recvName(r.Type)
		}
	}
	var out []CallSite
	if d.Body == nil {
		return out
	}
	ast.Inspect(d.Body, func(x ast.Node) bool {
		ce, ok := x.(*ast.CallExpr)
		if !ok {
			return true
		}
		site, ok := callSiteOf(ce.Fun, self)
		if ok {
			out = append(out, site)
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Left < out[j].Left
	})
	return dedupeCalls(out)
}

func callSiteOf(fun ast.Expr, self string) (CallSite, bool) {
	switch t := fun.(type) {
	case *ast.Ident:
		if t.Name == "nil" || t.Name == "true" || t.Name == "false" {
			return CallSite{}, false
		}
		return CallSite{Name: t.Name, Depth: 0, Self: t.Name == self}, true
	case *ast.SelectorExpr:
		left, depth := selectorChain(t)
		if depth >= 2 {
			// Rule 5: a.b.M() is not resolved to a function; the import edge
			// already carries the dependency. Recorded so it counts as
			// unresolved instead of silently disappearing.
			return CallSite{Name: t.Sel.Name, Left: left, Depth: depth}, true
		}
		return CallSite{Name: t.Sel.Name, Left: left, Depth: 1, Self: left == self}, true
	}
	return CallSite{}, false
}

// selectorChain returns the leftmost identifier and the length of the
// selector chain. A CallExpr in the chain (f().M) yields depth >= 2.
func selectorChain(e *ast.SelectorExpr) (string, int) {
	depth := 1
	cur := e.X
	for {
		switch t := cur.(type) {
		case *ast.Ident:
			return t.Name, depth
		case *ast.SelectorExpr:
			depth++
			cur = t.X
		case *ast.CallExpr:
			return "", depth + 1
		case *ast.IndexExpr, *ast.IndexListExpr:
			return "", depth + 1
		default:
			return "", depth + 1
		}
	}
}

func dedupeCalls(in []CallSite) []CallSite {
	seen := map[string]bool{}
	var out []CallSite
	for _, c := range in {
		k := c.Name + "\x00" + c.Left + "\x00" + string(rune(c.Depth))
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}

func cleanDoc(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func countCodeLines(text string) int {
	n := 0
	inBlock := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if inBlock {
			if strings.Contains(line, "*/") {
				inBlock = false
			}
			continue
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "/*") {
			inBlock = !strings.Contains(line, "*/")
			continue
		}
		n++
	}
	return n
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
