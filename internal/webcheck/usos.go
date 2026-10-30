package main

// usos.go — Mitad "plantilla" del cruce: qué URLs usan las plantillas y con qué
// verbo. Se cruza contra inventarioRutas (rutas.go) para detectar enlaces
// muertos (ruta no registrada) y desajustes de método (el JS hace POST contra
// una ruta registrada solo GET → 405).
//
// El verificador antiguo no miraba esto: validaba sintaxis y render, y un href a
// una ruta que nadie registró pasaba a producción como 404. Aquí se cierra ese
// hueco.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Uso es una URL referenciada desde una plantilla, con el verbo que se infiere
// del contexto (href → GET, $.post → POST, etcétera).
type Uso struct {
	URL     string // ruta normalizada (sin query ni fragmento)
	Metodo  string // "" = no se infiere verbo; el matcher lo ignora
	Archivo string
	Linea   int
	Origen  string // href | action | ajax | post | get | fetch
}

// filtrables son prefijos/casos que no se cruzan contra el router de la app.
var filtrables = []string{"#", "javascript:", "mailto:", "http://", "https://", "/static/"}

func esFiltro(u string) bool {
	if u == "" {
		return true
	}
	if strings.Contains(u, "{{") || strings.Contains(u, "}}") {
		// la URL es dinámica: el router no puede conocerla de estar.
		return true
	}
	for _, f := range filtrables {
		if strings.HasPrefix(u, f) {
			return true
		}
	}
	return false
}

// normalizarUso quita query/fragmento y normaliza para casar contra patrones.
func normalizarUso(u string) string {
	if i := strings.Index(u, "?"); i >= 0 {
		u = u[:i]
	}
	if i := strings.Index(u, "#"); i >= 0 {
		u = u[:i]
	}
	return u
}

// reHrefAction captura href="..." / href='...' y action="..." / action='...'.
// Go usa RE2 (sin backreferences), así que se alternan las dos comillas: el
// contenido está en el grupo 1 (comillas dobles) o en el 2 (simples).
var reHrefAction = regexp.MustCompile(`(?i)(?:href|action)\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// reAjax captura `url: '/x'` dentro de ajax: { ... }, con el type que le corresponde.
var reAjax = regexp.MustCompile(`url\s*:\s*['"]([^'"]+)['"]`)

// reAjaxType captura el type junto a un ajax url cuando aparece cerca.
var reAjaxType = regexp.MustCompile(`type\s*:\s*['"]([A-Za-z]+)['"]`)

// rePostGet captura $.post('url', ...), $.get('url' ...) y fetch('url').
var rePost = regexp.MustCompile(`\$\.(post|get)\s*\(\s*['"]([^'"]+)['"]`)
var reFetch = regexp.MustCompile(`fetch\s*\(\s*['"]([^'"]+)['"]`)

// inventarioUsos recorre las plantillas de dir y devuelve los usos de URLs.
func inventarioUsos(dir string) ([]Uso, error) {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Uso
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		lineas := strings.Split(string(b), "\n")
		for i, linea := range lineas {
			out = append(out, usosDeLinea(linea, e.Name(), i+1)...)
		}
	}
	return out, nil
}

// usosDeLinea extrae los usos de una línea de plantilla.
func usosDeLinea(linea, archivo string, lineaNum int) []Uso {
	var out []Uso
	// href / action: navegación GET salvo <form method="POST"> en la misma línea.
	for _, m := range reHrefAction.FindAllStringSubmatchIndex(linea, -1) {
		attr := ""
		if m[2] >= 0 {
			attr = linea[m[2]:m[3]]
		} else if m[4] >= 0 {
			attr = linea[m[4]:m[5]]
		}
		if esFiltro(attr) {
			continue
		}
		metodo := "GET"
		// El form action que postea lo lleva el método del <form>, no del action.
		if strings.Contains(linea, "action") && strings.Contains(strings.ToUpper(linea), `METHOD="POST"`) {
			metodo = "POST"
		}
		out = append(out, Uso{URL: normalizarUso(attr), Metodo: metodo, Archivo: archivo, Linea: lineaNum, Origen: "href"})
	}
	// ajax: { url: '...', type: 'POST' }
	for _, m := range reAjax.FindAllStringSubmatchIndex(linea, -1) {
		u := linea[m[2]:m[3]]
		if esFiltro(u) {
			continue
		}
		metodo := ""
		if mt := reAjaxType.FindStringSubmatch(linea); mt != nil {
			metodo = strings.ToUpper(mt[1])
		}
		out = append(out, Uso{URL: normalizarUso(u), Metodo: metodo, Archivo: archivo, Linea: lineaNum, Origen: "ajax"})
	}
	// $.post / $.get
	for _, m := range rePost.FindAllStringSubmatchIndex(linea, -1) {
		metodo := strings.ToUpper(linea[m[2]:m[3]])
		u := linea[m[4]:m[5]]
		if esFiltro(u) {
			continue
		}
		origen := "get"
		if metodo == "POST" {
			origen = "post"
		}
		out = append(out, Uso{URL: normalizarUso(u), Metodo: metodo, Archivo: archivo, Linea: lineaNum, Origen: origen})
	}
	// fetch
	for _, m := range reFetch.FindAllStringSubmatchIndex(linea, -1) {
		u := linea[m[2]:m[3]]
		if esFiltro(u) {
			continue
		}
		out = append(out, Uso{URL: normalizarUso(u), Metodo: "GET", Archivo: archivo, Linea: lineaNum, Origen: "fetch"})
	}
	return out
}
