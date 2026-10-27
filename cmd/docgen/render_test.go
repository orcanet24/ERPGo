package main

// Test del ensamblado del manual. Los assets del docgen (template, css y js)
// se perdieron una vez del repositorio y el binario dejó de compilar, porque
// los patrones go:embed fueron embedidos sin archivo que casar. Este test
// verifica que los tres existen, parsean y ensamblan un HTML en el que el
// modelo y el código JS llegan como literales válidos.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderHTMLEnsamblaDocumentoCompleto(t *testing.T) {
	m := Model{
		Meta:     Meta{Title: "Manual de prueba", Product: "prueba", Tagline: "lema de prueba"},
		Segments: []Segment{},
		Files:    []File{},
		Rules:    []Rule{},
		Tours:    []Tour{},
		Stats:    Stats{ByConf: map[string]int{}},
	}
	out, err := RenderHTML(m)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "<title>Manual de prueba</title>") {
		t.Errorf("el título no llegó a la cabecera; salida: %.200s", s)
	}
	if !strings.Contains(s, "new Function") {
		t.Errorf("el documento no monta el JS con new Function; salida: %.200s", s)
	}

	// El argumento de JSON.parse es un literal de cadena JS (jsValEscaper):
	// NO es el JSON directo, sino un string que lo contiene. El navegador
	// evalúa el literal —recuperando el texto JSON— y JSON.parse lo levanta.
	// Lo mismo a dos niveles: primera unmarshal (del literal de cadena) y
	// segunda unmarshal (del JSON dentro).
	marca := "JSON.parse("
	at := strings.Index(s, marca)
	if at < 0 {
		t.Fatalf("la plantilla no incluye la isla del modelo")
	}
	resto := s[at+len(marca):]
	fin := strings.Index(resto, ");")
	if fin < 0 {
		t.Fatalf("no hay cierre del JSON.parse; salida: %.200s", s[at:])
	}
	var textoJSON string
	if err := json.Unmarshal([]byte(resto[:fin]), &textoJSON); err != nil {
		t.Errorf("el literal embebido no es una cadena JSON válida: %v", err)
	} else {
		var deVuelta map[string]interface{}
		if err := json.Unmarshal([]byte(textoJSON), &deVuelta); err != nil {
			t.Errorf("el JSON dentro de la isla no decodifica: %v", err)
		} else if meta, ok := deVuelta["meta"].(map[string]interface{}); !ok || meta["title"] != "Manual de prueba" {
			t.Errorf("el modelo en la isla no viaja completo: %v", deVuelta["meta"])
		}
	}

	// El JS va por la misma vía: literal de cadena que new Function compila.
	marcaJS := "new Function(\"model\", \"doc\", "
	atJS := strings.Index(s, marcaJS)
	if atJS < 0 {
		t.Fatalf("la plantilla no pasa el JS a new Function")
	}
	// El literal termina justo antes de "))(model, document);" — un ")")
	// suelto dentro del código cortaría antes y abortaría el test, así que el
	// corte se ancla al cierre que solo puede estar al final.
	cierre := "))(model, document);"
	actura := strings.Index(s[atJS:], cierre)
	if actura < 0 {
		t.Fatalf("el new Function no cierra como espera la plantilla")
	}
	var fuenteJS string
	if err := json.Unmarshal([]byte(s[atJS+len(marcaJS):atJS+actura]), &fuenteJS); err != nil {
		t.Errorf("el JS embebido no es una cadena válida: %v", err)
	} else if !strings.Contains(fuenteJS, "function render") {
		t.Errorf("el JS embebido no trae el renderizador")
	}
}
