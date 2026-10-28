// render.go — Phase 4: assemble the single self-contained HTML file.
//
// The generator never writes HTML by hand. It serialises one typed model to
// JSON, injects it as a <script type="application/json"> island, and lets the
// browser JS render everything from that island. Zero network requests.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/manual.html.tmpl
var tmplSource string

//go:embed assets/manual.css
var cssSource string

//go:embed assets/manual.js
var jsSource string

// Stats are the counters the manual shows about itself.
type Stats struct {
	Files      int            `json:"files"`
	GoFiles    int            `json:"goFiles"`
	Funcs      int            `json:"funcs"`
	Types      int            `json:"types"`
	Lines      int            `json:"lines"`
	Edges      int            `json:"edges"`
	Calls      int            `json:"calls"`
	Resolved   int            `json:"resolved"`
	Unresolved int            `json:"unresolved"`
	Deep       int            `json:"deep"`
	ByConf     map[string]int `json:"byConf"`
	Excluded   int            `json:"excluded"`
	OnDisk     int            `json:"onDisk"`
}

// Coverage is the honesty counter: curated vs total.
type Coverage struct {
	Curated int     `json:"curated"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// Model is the single JSON object the browser consumes.
type Model struct {
	Meta     Meta      `json:"meta"`
	Segments []Segment `json:"segments"`
	Files    []File    `json:"files"`
	Edges    []Edge    `json:"edges"`
	Rules    []Rule    `json:"rules"`
	Tours    []Tour    `json:"tours"`
	Stats    Stats     `json:"stats"`
	Coverage Coverage  `json:"coverage"`
}

// BuildModel turns the parsed facts plus the overlay into the render model.
func BuildModel(s *Scan, g *Graph, o *Overlay) Model {
	m := Model{
		Meta:     o.Meta,
		Segments: o.Segments,
		Files:    s.Files,
		Edges:    g.Edges,
		Rules:    o.Rules,
		Tours:    o.Tours,
	}
	if m.Rules == nil {
		m.Rules = []Rule{}
	}
	if m.Tours == nil {
		m.Tours = []Tour{}
	}

	st := Stats{ByConf: map[string]int{}, Excluded: s.Excluded, OnDisk: s.Total}
	for _, f := range s.Files {
		st.Files++
		st.Lines += f.Lines
		if strings.HasSuffix(f.ID, ".go") {
			st.GoFiles++
		}
		st.Funcs += len(f.Funcs)
		st.Types += len(f.Types)
	}
	for _, e := range g.Edges {
		st.Edges++
		st.ByConf[e.Conf]++
	}
	st.Calls = g.CallsTotal
	st.Resolved = g.Resolved
	st.Unresolved = g.Unresolved
	st.Deep = g.Deep
	m.Stats = st

	curated := 0
	for _, f := range s.Files {
		if f.Curated {
			curated++
		}
	}
	m.Coverage = Coverage{Curated: curated, Total: len(s.Files)}
	if len(s.Files) > 0 {
		m.Coverage.Percent = float64(int(float64(curated)/float64(len(s.Files))*1000)) / 10
	}
	return m
}

// RenderHTML produces the final document. It is deterministic: same inputs,
// same bytes.
func RenderHTML(m Model) ([]byte, error) {
	payload, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	t, err := template.New("manual").Parse(tmplSource)
	if err != nil {
		return nil, fmt.Errorf("plantilla inválida: %w", err)
	}
	data := struct {
		Title string
		JSON  string
		CSS   template.CSS
		JS    string
	}{
		Title: m.Meta.Title,
		JSON:  sanitizeJSONIsland(payload),
		// template.CSS y no string: html/template RECHAZA cualquier string
		// plano dentro de <style> y lo sustituye entero por el marcador
		// «ZgotmplZ». Ocurrió: el manual se publicó meses sin una sola regla
		// de estilo y nadie lo notó, porque un HTML sin CSS se ve roto pero
		// legible. El tipo es la declaración de que esta hoja es nuestra y
		// está bien formada (no lleva «</style» ni «</», comprobado).
		CSS: template.CSS(cssSource),
		JS:  jsSource,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("no se pudo ensamblar el HTML: %w", err)
	}
	return buf.Bytes(), nil
}

// sanitizeJSONIsland keeps "</script>" inside the JSON from closing the tag.
func sanitizeJSONIsland(b []byte) string {
	s := string(b)
	s = strings.ReplaceAll(s, "</", "<\\/")
	return s
}

// WriteFile writes bytes to path, creating parent directories.
func WriteFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
