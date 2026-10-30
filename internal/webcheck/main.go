// Comando de verificación: parsea cada plantilla con EXACTAMENTE el mismo
// conjunto de funciones que registra la aplicación. Un chequeo hecho con otro
// conjunto da falsos positivos (una plantilla puede usar `money` sin problema y
// fallar en el chequeo), y un falso positivo en el chequeo es la forma
// segura de ignorar la herramienta entera.
//
// Hace dos comprobaciones:
//  1. Sintaxis + render con datos de muestra: detecta {{template "x"}} indefinido,
//     campos inexistentes, etc.
//  2. Enlaces vivos y verbos: un href/action/JS que apunte a una ruta que nadie
//     registró en s.mux → 404 en producción; y un POST sobre una ruta registrada
//     solo GET → 405. Antes de esta fase, el "sin fin de 404/405" solo se veía
//     navegando.
//
// Vive en internal/webcheck y no en un script suelto porque forma parte de la
// garantía de calidad: se ejecuta igual que los tests.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir := flag.String("dir", "templates", "directorio de plantillas")
	gocode := flag.String("gocode-dir", "interfaces/web", "directorio .go donde se registran las rutas (s.mux.HandleFunc/Handle)")
	flag.Parse()

	// ---------- Fase 1: sintaxis + render ----------
	entries, err := os.ReadDir(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo leer %s: %v\n", *dir, err)
		os.Exit(2)
	}

	layout, err := os.ReadFile(filepath.Join(*dir, "layout.html"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo leer layout.html: %v\n", err)
		os.Exit(2)
	}

	var fallidos, revisados, conLayout, conDatos int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		path := filepath.Join(*dir, e.Name())
		revisados++
		if _, err := template.New(e.Name()).Funcs(templateFuncs()).ParseFiles(path); err != nil {
			fmt.Printf("PARSE ERROR %s: %v\n", e.Name(), err)
			fallidos++
			continue
		}
		if e.Name() == "layout.html" {
			continue
		}

		page, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Hay dos familias de plantillas y se distinguen por si definen "content".
		// Las que lo hacen se pintan DENTRO del layout; las otras son páginas
		// completas (login, asistente, selector de empresa) que se renderizan
		// solas y no deben someterse al layout (no definen content → 404 de
		// template al render).
		if !strings.Contains(string(page), `{{define "content"}}`) {
			continue
		}
		conLayout++

		// Igual que buildTemplateSet en interfaces/web: la página se parsea con
		// un "pagejs" POR DEFECTO definido ANTES que layout y página. Sin esto
		// el comprobador grita «no such template pagejs» en toda página que no
		// declare el suyo, mientras que la aplicación la pinta sin problema —
		// un falso positivo, que es exactamente lo que esta herramienta existe
		// para no producir.
		t := template.New(e.Name()).Funcs(templateFuncs())
		if _, err := t.Parse(`{{define "pagejs"}}{{end}}`); err != nil {
			fmt.Printf("PAGEJS ERROR %s: %v\n", e.Name(), err)
			fallidos++
			continue
		}
		if t, err = t.Parse(string(layout)); err != nil {
			fmt.Printf("LAYOUT PARSE ERROR %s: %v\n", e.Name(), err)
			fallidos++
			continue
		}
		if t, err = t.Parse(string(page)); err != nil {
			fmt.Printf("PAGE PARSE ERROR %s: %v\n", e.Name(), err)
			fallidos++
			continue
		}
		if err := t.ExecuteTemplate(io.Discard, "layout", sampleData()); err != nil {
			// Solo los errores ESTRUCTURALES son un fallo de la plantilla: un
			// {{template "x"}} sin definir rompe igual con cualquier dato. Los
			// de tipo ("expected float64") dependen del dato que se le pase, y el
			// de muestra no los conoce todos: se listan aparte para no gritar en
			// falso y hacer que el comprobador se deje de usar.
			if isStructural(err) {
				fmt.Printf("RENDER ERROR %s: %v\n", e.Name(), err)
				fallidos++
				continue
			}
			conDatos++
		}
	}
	fmt.Printf("revisadas %d plantillas (%d con layout), %d con error estructural", revisados, conLayout, fallidos)
	if conDatos > 0 {
		// Informativo, no falla: son páginas que necesitan datos que el
		// comprobador no puede inventar.
		fmt.Printf(" (%d necesitan datos reales para evaluarse por completo)\n", conDatos)
	} else {
		fmt.Println()
	}

	// ---------- Fase 2: enlaces vivos y verbos ----------
	if rutas, rerr := inventarioRutas(*gocode); rerr != nil {
		fmt.Fprintf(os.Stderr, "aviso: no se pudieron inventariar las rutas de %s: %v\n", *gocode, rerr)
	} else {
		usos, uerr := inventarioUsos(*dir)
		if uerr != nil {
			fmt.Fprintf(os.Stderr, "aviso: no se pudieron inventariar los usos de %s: %v\n", *dir, uerr)
		} else {
			errazados := 0
			for _, u := range usos {
				encontro, metodoOK := casaYMetodo(u.URL, u.Metodo, rutas)
				switch {
				case !encontro:
					fmt.Printf("ENLACE MUERTO %s:%d  %s  no registrada en el router (→ 404)\n", u.Archivo, u.Linea, u.URL)
					errazados++
				case u.Metodo != "" && !metodoOK:
					fmt.Printf("VERBO INCORRECTO %s:%d  %s %s  la ruta existe pero no acepta ese verbo (→ 405)\n", u.Archivo, u.Linea, u.Metodo, u.URL)
					errazados++
				}
			}
			if errazados == 0 {
				fmt.Printf("enlaces vivos: 0 rotos entre %d referencias revisadas\n", len(usos))
			} else {
				fmt.Printf("enlaces vivos: %d rotos entre %d referencias revisadas\n", errazados, len(usos))
			}
			if errazados > 0 {
				fallidos += errazados
			}
		}
	}

	if fallidos > 0 {
		os.Exit(1)
	}
}

// templateFuncs replica interfaces/web.templateFuncs. Se mantiene aquí por
// diseño: el comprobador es una herramienta independiente y no debe importar
// el paquete web (que arrastra el servidor entero solo para leer una lista).
// Si algún día divergen, el fallo aparece como "function not defined", que es
// exactamente la señal que se busca.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
		"money": func(v float64) string { return fmt.Sprintf("%.2f", v) },
		"divf":  func(a, b float64) float64 { return a / b },
		"query": func(string) string { return "" },
		"today": func() string { return "" },
		"now":   func() string { return "" },
		// assetVer es la huella POR ARCHIVO de los recursos estáticos (el ?v= de
		// cada <link>/<script> del layout). Se replica aquí con una constante no
		// vacía: el chequeo sólo comprueba sintaxis y estructura, y un valor
		// estable es lo que permite ejecutar el layout entero sin fingir un
		// cambio de versión.
		"assetVer": func(ruta string) string { return "deadbeef" },
		// Debe replicarse aquí también: si el comprobador no conoce una función
		// que la aplicación sí, da "function not defined" en una plantilla que
		// funciona, y ese falso positivo es la forma segura de que la herramienta
		// deje de usarse.
		"catalogLabel": func(catalog string) string {
			if catalog == "conasev" {
				return "Catálogo CONASEV (SENIAT)"
			}
			return "Codificación propia de la empresa"
		},
		// kindLabel: la clase del documento de compra en la bandeja de
		// recepción. Los literales son los de los valores de compra_hash.go
		// (factura, nota_entrega, orden, sin_documento).
		"kindLabel": func(kind string) string {
			switch kind {
			case "factura":
				return "Factura"
			case "nota_entrega":
				return "Nota de entrega"
			case "orden":
				return "Orden de compra"
			case "sin_documento":
				return "Sin documento"
			}
			return "Sin clasificar"
		},
	}
}

// isStructural distingue un error de plantilla de un error del dato de prueba.
// Solo los primeros hacen fallar la comprobación.
func isStructural(err error) bool {
	return strings.Contains(err.Error(), "no such template") ||
		strings.Contains(err.Error(), "can't evaluate field") ||
		strings.Contains(err.Error(), "undefined variable")
}

// casaYMetodo dice, para un uso concreto, si alguna ruta registrada lo cubre y si
// al menos una de esas rutas acepta el verbo del uso.
func casaYMetodo(uso, metodo string, rutas []Ruta) (encontro, metodoOK bool) {
	for _, r := range rutas {
		if !casaRuta(uso, r) {
			continue
		}
		encontro = true
		// Una ruta sin prefijo de verbo acepta cualquier cosa (aceptaMetodo("")).
		if r.aceptaMetodo(metodo) {
			metodoOK = true
		}
	}
	return encontro, metodoOK
}

// sampleData es el contexto mínimo para ejecutar el layout. No busca replicar
// los datos reales: solo tiene que ser una estructura válida con las claves que
// el layout lee sin condición, porque un nil inesperado haría que el fallo
// reportado fuera del dato de prueba y no de la plantilla.
func sampleData() map[string]interface{} {
	zero := 0
	return map[string]interface{}{
		"AppName":     "ERPGo",
		"Company":     CompanyInfo{},
		"Setting":     WebSetting{Currency: "$"},
		"Currency":    "$",
		"BaseURL":     "",
		"CompanyName": "",
		"Path1":       "",
		"Path2":       "",
		"CurrentURL":  "/",
		"Module":      "",
		"OutOfStock":  0,
		"UserID":      "",
		"Username":    "",
		"Fullname":    "",
		"UserLevel":   "",
		"IsAdmin":     true,
		"UserImage":   "",
		"Perm":        map[string]bool{},
		"Zero":        &zero,
		// Coinciden con los tipos de interfaces/web: un dato de prueba con la
		// forma equivocada hace fallar el render por el dato, no por la
		// plantilla, y el error que se lee no sirve.
		"Catalog":        "propio",
		"Source":         "",
		"AppliedAt":      "",
		"Notes":          "",
		"AccountCount":   0,
		"CollisionCount": 0,
		"Collisions":     []string{},
		"OrderMissing":   []string{},
		"IsCONASEV":      false,
	}
}

// Los tipos deben coincidir con los de interfaces/web o el render falla por el
// dato de prueba y no por la plantilla.
type CompanyInfo struct {
	CompanyName string
	Email       string
	Address     string
	Mobile      string
	Website     string
	VatNo       string
	CrNo        string
}

type WebSetting struct {
	Logo          string
	InvoiceLogo   string
	Favicon       string
	Currency      string
	Timezone      string
	FooterText    string
	Language      string
	Rtr           string
	IsQr          int
	IsAutoapprove int
	DiscountType  int
}
