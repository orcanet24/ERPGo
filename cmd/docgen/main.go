// main.go — docgen: builds the Programmer's Manual for this repository.
//
// Usage:
//
//	go run ./cmd/docgen                 # regenerate doc/developer/index.html
//	go run ./cmd/docgen -check          # fail if the committed HTML is stale
//
// The generator is deterministic: same sources, same bytes.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := flag.String("root", ".", "raíz del repositorio a documentar")
	overlay := flag.String("overlay", "", "ruta del overlay JSON (por defecto <root>/doc/developer/overlay.json)")
	out := flag.String("out", "", "HTML de salida (por defecto <root>/doc/developer/index.html)")
	check := flag.Bool("check", false, "regenera en memoria y falla si difiere del fichero commiteado")
	flag.Parse()

	if err := run(*root, *overlay, *out, *check); err != nil {
		fmt.Fprintln(os.Stderr, "docgen:", err)
		os.Exit(1)
	}
}

func run(root, overlayPath, outPath string, check bool) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if overlayPath == "" {
		overlayPath = filepath.Join(absRoot, "doc", "developer", "overlay.json")
	}
	if outPath == "" {
		outPath = filepath.Join(absRoot, "doc", "developer", "index.html")
	}

	outDir := ""
	if rel, rerr := filepath.Rel(absRoot, filepath.Dir(outPath)); rerr == nil && !strings.HasPrefix(rel, "..") {
		outDir = filepath.ToSlash(rel)
	}

	scan, err := ScanRepo(absRoot, outDir)
	if err != nil {
		return fmt.Errorf("fallo el parser: %w", err)
	}

	ov, err := LoadOverlay(overlayPath)
	if err != nil {
		return err
	}
	if err := ApplyOverlay(scan, ov); err != nil {
		return err
	}

	g := BuildGraph(absRoot, scan)
	model := BuildModel(scan, g, ov)
	html, err := RenderHTML(model)
	if err != nil {
		return err
	}

	if check {
		committed, rerr := os.ReadFile(outPath)
		if rerr != nil {
			return fmt.Errorf("docs-check: no se pudo leer %s (%v)", outPath, rerr)
		}
		if !bytes.Equal(committed, html) {
			return fmt.Errorf("docs-check: %s está desfasado respecto al código; regenera con `go run ./cmd/docgen`", outPath)
		}
		fmt.Printf("docs-check: OK (%d bytes, %d archivos, %d aristas)\n", len(html), model.Stats.Files, model.Stats.Edges)
		return nil
	}

	if err := WriteFile(outPath, html); err != nil {
		return err
	}
	fmt.Printf("manual generado: %s\n", outPath)
	fmt.Printf("  archivos=%d funciones=%d tipos=%d aristas=%d\n",
		model.Stats.Files, model.Stats.Funcs, model.Stats.Types, model.Stats.Edges)
	fmt.Printf("  llamadas=%d resueltas=%d no resueltas=%d (profundas=%d)\n",
		model.Stats.Calls, model.Stats.Resolved, model.Stats.Unresolved, model.Stats.Deep)
	fmt.Printf("  cobertura curada=%d/%d (%.1f%%)\n",
		model.Coverage.Curated, model.Coverage.Total, model.Coverage.Percent)
	return nil
}
