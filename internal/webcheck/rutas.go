// Inventario de rutas del lado Go.
//
// La app registra TODO el enrutado por código —s.mux.HandleFunc("GET /x", ...)—
// repartido en decenas de archivos de interfaces/web. El verificador antiguo
// sólo miraba plantillas, así que un href apuntando a una ruta que nadie
// registró pasaba是用 silencio y el defecto aparecía en producción como 404.
// Este archivo construye la mitad "servidor" del cruce: la lista de patrones
// REALES que el servidor atendería, extraída del código fuente.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Ruta es un patrón registrado en el ServeMux del servidor.
//
// Metodo vacío no significa "sin método": significa "cualquier verbo", que es
// lo que Go 1.22 hace con un patrón sin prefijo. Ambas cosas existen en el
// código (los endpoints de datos van con "GET "/"POST " explícito; /login, no)
// y el cruce las trata distinto: la primera rechaza verbos extraños con 405,
// la segunda acepta todo.
type Ruta struct {
	Patron  string // literal completo tal como figura en el fuente: "GET /x/{id}"
	Metodo  string // "" = cualquier verbo
	Path    string // patrón sin prefijo de método
	Archivo string
	Linea   int
	// Subarbol marca los patrones de cola "/x/" (Sin los corchetes de Go 1.22):
	// el ServeMux los casa por prefijo con todo lo que cuelga de ahí.
	Subarbol bool
}

// reRegistro captura las dos familias de registro del enrutador. Busca en TODO
// el texto del archivo para no fallar con el registro partido en dos líneas.
// NO entra a resolver variables (`s.mux.HandleFunc(ruta, ...)`, addons.go): la
// ruta que lleva esa variable es la frontera /__addon/, que el cruce filtra
// por diseño.
var reRegistro = regexp.MustCompile(`(?s)\.(?:HandleFunc|Handle)\(\s*"([^"]*)"`)

// reMetodo separa el prefijo de verbo del patrón, igual que lo hace el parser
// de net/http en Go 1.22.
var reMetodo = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS) +(.*)$`)

// inventarioRutas recorre los .go de dir (excluye _test.go: el enrutado que
// viste un test monta su propio ServeMux y no es el que atiende al navegador)
// y devuelve las rutas registradas en orden de aparición.
func inventarioRutas(dir string) ([]Ruta, error) {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", dir, err)
	}
	var rutas []Ruta
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		fonte, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		texto := string(fonte)
		for _, m := range reRegistro.FindAllStringSubmatchIndex(texto, -1) {
			patron := texto[m[2]:m[3]]
			ok := strings.Contains(patron, "/")
			// Un literal sin "/" cerca de un HandleFunc es raro pero posible
			// (una ruta raíz "/"); el método necesita espacio tras él, así que
			// cualquier pieza que no separa con el split se descarta.
			var metodo, path string
			if mm := reMetodo.FindStringSubmatch(patron); mm != nil {
				metodo, path = mm[1], mm[2]
			} else {
				path = patron
			}
			if path == "" || strings.ContainsAny(path, " \t\n") {
				_ = ok
				continue
			}
			rutas = append(rutas, Ruta{
				Patron:   patron,
				Metodo:   metodo,
				Path:     path,
				Archivo:  e.Name(),
				Linea:    1 + strings.Count(texto[:m[2]], "\n"),
				Subarbol: strings.HasSuffix(path, "/") && !strings.Contains(path, "{"),
			})
		}
	}
	return rutas, nil
}

// segmento es un trozo de un patrón o de un uso: literal encajado, o parámetro
// (los {id} de Go 1.22 del lado servidor; los {*} que produce el cruce en el
// lado plantilla cuando la URL se arma por concatenación o con {{.X}}).
type segmento struct {
	texto   string
	esParam bool
}

// segmentos parte una ruta en segmentos. Los {x} y los {x...} son parámetros;
// el resto son literales que deben coincidir letra a letra.
func segmentos(ruta string) []segmento {
	var out []segmento
	for _, trozo := range strings.Split(ruta, "/") {
		if trozo == "" {
			continue
		}
		esParam := strings.HasPrefix(trozo, "{") && strings.HasSuffix(trozo, "}")
		out = append(out, segmento{texto: trozo, esParam: esParam})
	}
	return out
}

// casaRuta dice si un uso (con su URL ya normalizada) caería en esta ruta
// registrada. Esto replica la semántica del ServeMux de Go 1.22 en lo que el
// cruce necesita: segmentos literales iguales, {param} casando con un segmento
// cualquiera, colas {x...} con todo lo sobrante, patrones de cola por prefijo
// y "/" casando con todo.
func casaRuta(usado string, r Ruta) bool {
	if r.Path == "/" {
		return true
	}
	if r.Subarbol {
		return strings.HasPrefix(usado, r.Path)
	}
	sSu := segmentos(usado)
	sRu := segmentos(r.Path)
	// Cola {x...}: los segmentos anteriores deben casar y el resto es libre.
	if len(sRu) > 0 && strings.HasSuffix(sRu[len(sRu)-1].texto, "...}") {
		if len(sSu) < len(sRu)-1 {
			return false
		}
		sSu = sSu[:len(sRu)-1]
		sRu = sRu[:len(sRu)-1]
	}
	if len(sSu) != len(sRu) {
		return false
	}
	for i := range sSu {
		if sRu[i].esParam || sSu[i].esParam {
			continue
		}
		if sRu[i].texto != sSu[i].texto {
			return false
		}
	}
	return true
}

// aceptaMetodo: una ruta con prefijo de verbo sólo atiende ése; sin prefijo
// acepta cualquiera.
func (r Ruta) aceptaMetodo(metodo string) bool {
	return r.Metodo == "" || r.Metodo == metodo
}
