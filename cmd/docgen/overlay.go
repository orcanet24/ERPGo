// overlay.go — Phase 3: load and validate the curated narrative.
//
// The overlay is the only place where prose lives. It is validated against
// the parsed facts: a key that points at something which no longer exists is
// a hard failure, because narrative drift is an error, not a silent loss.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Meta is the header of the manual.
type Meta struct {
	Title   string `json:"title"`
	Product string `json:"product"`
	Tagline string `json:"tagline"`
}

// Segment is one tab of the manual.
type Segment struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Tagline          string   `json:"tagline"`
	Summary          string   `json:"summary"`
	Responsibilities []string `json:"responsibilities"`
	Invariants       []string `json:"invariants"`
	Paths            []string `json:"paths"`
	Files            []string `json:"files"` // resolved file ids, sorted
}

// Rule is an immutable project rule.
type Rule struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// TourStep is one stop of a guided tour.
type TourStep struct {
	N      int    `json:"n"`
	Anchor string `json:"anchor"`
	Title  string `json:"title"`
	Note   string `json:"note"`
}

// Tour is a guided walk through the main flow.
type Tour struct {
	ID    string     `json:"id"`
	Title string     `json:"title"`
	Intro string     `json:"intro"`
	Steps []TourStep `json:"steps"`
}

// OverlayFile is the narrative for one file.
type OverlayFile struct {
	Role      string `json:"role"`
	How       string `json:"how"`
	Relations string `json:"relations"`
	Watchouts string `json:"watchouts"`
}

// OverlayFunc is the narrative for one function.
type OverlayFunc struct {
	What      string   `json:"what"`
	How       []string `json:"how"`
	Relations string   `json:"relations"`
}

// OverlayType is the narrative for one type.
type OverlayType struct {
	How       string `json:"how"`
	Relations string `json:"relations"`
}

// Overlay is the raw JSON document.
type Overlay struct {
	Meta     Meta                   `json:"meta"`
	Segments []Segment              `json:"segments"`
	Files    map[string]OverlayFile `json:"files"`
	Funcs    map[string]OverlayFunc `json:"funcs"`
	Types    map[string]OverlayType `json:"types"`
	Rules    []Rule                 `json:"rules"`
	Tours    []Tour                 `json:"tours"`
}

// LoadOverlay reads and decodes the overlay file.
func LoadOverlay(path string) (*Overlay, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el overlay: %w", err)
	}
	var o Overlay
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return nil, fmt.Errorf("el overlay no es JSON válido: %w", err)
	}
	if o.Files == nil {
		o.Files = map[string]OverlayFile{}
	}
	if o.Funcs == nil {
		o.Funcs = map[string]OverlayFunc{}
	}
	if o.Types == nil {
		o.Types = map[string]OverlayType{}
	}
	return &o, nil
}

// reservedKey reports whether a key is a pending note ("_..."), which is
// allowed to dangle without breaking the gate.
func reservedKey(k string) bool { return strings.HasPrefix(k, "_") }

// ApplyOverlay validates the narrative against the facts, assigns every file
// to exactly one segment and copies the curated prose onto the model.
func ApplyOverlay(s *Scan, o *Overlay) error {
	var errs []string

	if strings.TrimSpace(o.Meta.Title) == "" {
		errs = append(errs, "meta.title es obligatorio")
	}

	// --- segments -------------------------------------------------------
	segOf := map[string]string{} // file id -> segment id
	exact := map[string][]string{}
	for i := range o.Segments {
		seg := &o.Segments[i]
		if strings.TrimSpace(seg.ID) == "" || strings.TrimSpace(seg.Title) == "" {
			errs = append(errs, fmt.Sprintf("segmento en posición %d sin id/título", i))
			continue
		}
		if seg.Responsibilities == nil {
			seg.Responsibilities = []string{}
		}
		if seg.Invariants == nil {
			seg.Invariants = []string{}
		}
		if seg.Paths == nil {
			seg.Paths = []string{}
		}
		exact[seg.ID] = append([]string(nil), seg.Files...)
		seg.Files = []string{}
	}

	// Pass 1: exact file ids win over path prefixes.
	for i := range o.Segments {
		seg := &o.Segments[i]
		for _, id := range exact[seg.ID] {
			if !fileExists(s, id) {
				errs = append(errs, fmt.Sprintf("segmento %q lista un archivo inexistente: %s", seg.ID, id))
				continue
			}
			if prev, ok := segOf[id]; ok && prev != seg.ID {
				errs = append(errs, fmt.Sprintf("archivo %s asignado a dos segmentos: %s y %s", id, prev, seg.ID))
				continue
			}
			segOf[id] = seg.ID
		}
	}
	// Pass 2: path prefixes.
	for i := range o.Segments {
		seg := &o.Segments[i]
		for _, f := range s.Files {
			if _, done := segOf[f.ID]; done {
				continue
			}
			for _, p := range seg.Paths {
				if matchPrefix(f.ID, p) {
					segOf[f.ID] = seg.ID
					break
				}
			}
		}
	}
	// Every scanned file must land somewhere.
	var missing []string
	for _, f := range s.Files {
		if _, ok := segOf[f.ID]; !ok {
			missing = append(missing, f.ID)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		errs = append(errs, fmt.Sprintf("%d archivo(s) sin segmento: %s",
			len(missing), strings.Join(limit(missing, 12), ", ")))
	}
	for i := range s.Files {
		if id, ok := segOf[s.Files[i].ID]; ok {
			s.Files[i].Segment = id
		}
	}
	for i := range o.Segments {
		seg := &o.Segments[i]
		for _, f := range s.Files {
			if f.Segment == seg.ID {
				seg.Files = append(seg.Files, f.ID)
			}
		}
		sort.Strings(seg.Files)
		if len(seg.Files) == 0 {
			errs = append(errs, fmt.Sprintf("segmento %q no engancha ningún archivo", seg.ID))
		}
	}

	// --- files ----------------------------------------------------------
	for k, v := range o.Files {
		if reservedKey(k) {
			continue
		}
		idx := indexOfFile(s, k)
		if idx < 0 {
			errs = append(errs, fmt.Sprintf("overlay.files apunta a un archivo inexistente: %s", k))
			continue
		}
		f := &s.Files[idx]
		f.Curated = true
		f.Role, f.How, f.Relations, f.Watchouts = v.Role, v.How, v.Relations, v.Watchouts
	}

	// --- functions ------------------------------------------------------
	for k, v := range o.Funcs {
		if reservedKey(k) {
			continue
		}
		fn, err := resolveFunc(s, k)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		fn.Curated = true
		fn.What, fn.How, fn.Relations = v.What, v.How, v.Relations
		if fn.How == nil {
			fn.How = []string{}
		}
	}

	// --- types ----------------------------------------------------------
	for k, v := range o.Types {
		if reservedKey(k) {
			continue
		}
		ty, err := resolveType(s, k)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		ty.Curated = true
		ty.How, ty.Relations = v.How, v.Relations
	}

	// --- rules and tours ------------------------------------------------
	for i, r := range o.Rules {
		if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Title) == "" {
			errs = append(errs, fmt.Sprintf("regla en posición %d sin id/título", i))
		}
	}
	for i, t := range o.Tours {
		if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Title) == "" {
			errs = append(errs, fmt.Sprintf("tour en posición %d sin id/título", i))
			continue
		}
		if len(t.Steps) == 0 {
			errs = append(errs, fmt.Sprintf("tour %q sin pasos", t.ID))
		}
		for j, st := range t.Steps {
			if strings.TrimSpace(st.Anchor) == "" {
				errs = append(errs, fmt.Sprintf("tour %q paso %d sin anchor", t.ID, j+1))
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("el overlay ha derivado del código (%d problema(s)):\n  - %s",
			len(errs), strings.Join(errs, "\n  - "))
	}
	return nil
}

func matchPrefix(id, prefix string) bool {
	p := strings.TrimSuffix(prefix, "/")
	if id == p {
		return true
	}
	return strings.HasPrefix(id, p+"/")
}

func fileExists(s *Scan, id string) bool { return indexOfFile(s, id) >= 0 }

func indexOfFile(s *Scan, id string) int {
	for i := range s.Files {
		if s.Files[i].ID == id {
			return i
		}
	}
	return -1
}

// resolveFunc accepts both "path#Name" and "path#Recv.Name". An ambiguous
// name is an error: the overlay must be precise.
func resolveFunc(s *Scan, key string) (*Func, error) {
	i := strings.Index(key, "#")
	if i < 0 {
		return nil, fmt.Errorf("clave de función mal formada (falta #): %s", key)
	}
	path, name := key[:i], key[i+1:]
	idx := indexOfFile(s, path)
	if idx < 0 {
		return nil, fmt.Errorf("overlay.funcs apunta a un archivo inexistente: %s", path)
	}
	var found *Func
	n := 0
	for j := range s.Files[idx].Funcs {
		fn := &s.Files[idx].Funcs[j]
		if fn.ID == key || fn.Name == name {
			found = fn
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("overlay.funcs apunta a una función inexistente: %s", key)
	}
	if n > 1 {
		return nil, fmt.Errorf("clave de función ambigua (usa path#Recv.Name): %s", key)
	}
	return found, nil
}

func resolveType(s *Scan, key string) (*Type, error) {
	i := strings.Index(key, "#")
	if i < 0 {
		return nil, fmt.Errorf("clave de tipo mal formada (falta #): %s", key)
	}
	path, name := key[:i], key[i+1:]
	idx := indexOfFile(s, path)
	if idx < 0 {
		return nil, fmt.Errorf("overlay.types apunta a un archivo inexistente: %s", path)
	}
	var found *Type
	n := 0
	for j := range s.Files[idx].Types {
		ty := &s.Files[idx].Types[j]
		if ty.ID == key || ty.Name == name {
			found = ty
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("overlay.types apunta a un tipo inexistente: %s", key)
	}
	if n > 1 {
		return nil, fmt.Errorf("clave de tipo ambigua: %s", key)
	}
	return found, nil
}

func limit(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return append(in[:n:n], fmt.Sprintf("… y %d más", len(in)-n))
}
