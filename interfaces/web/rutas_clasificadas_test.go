package web

// rutas_clasificadas_test.go — Ninguna ruta del ERP puede existir sin frontera.
//
// LA REGLA QUE PROTEGE (auditoría 2026-11-22, hallazgo S-1): el middleware de
// permisos decidía «¿está el prefijo en routeDepartment?» y, si no estaba,
// dejaba pasar a CUALQUIER usuario con sesión. product_price/save se reescribía
// así el precio de venta de toda la empresa desde el rol de un cajero. El
// arreglo de raíz no es «recordarse de clasificar»: es que el olvido falle en
// el test. Este test lee los fuentes del propio paquete, extrae TODAS las
// rutas registradas en el mux y exige que cada una caiga en una frontera
// nombrada: pública (con autenticación propia en el handler), de sesión (con
// guarda de propiedad en el handler), master, addon o módulo de la matriz.
//
// Una ruta nueva sin clasificar no puede mergearse. Y la defensa en
// profundidad vive en permissionMiddleware: «sin-clasificar» es 403 en
// producción, no un 200 silencioso, aunque alguien corra el binario sin la
// batería de pruebas.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reRegistroRuta = regexp.MustCompile(`mux\.Handle(?:Func)?\(\s*"([^"]+)"`)

// patronARuta quita el método HTTP del patrón de Go 1.22 ("POST /x/save" → "/x/save").
func patronARuta(patron string) string {
	if i := strings.LastIndexByte(patron, ' '); i >= 0 {
		patron = patron[i+1:]
	}
	return patron
}

func TestRutasRegistradasTodasClasificadas(t *testing.T) {
	archivos, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("leyendo el paquete: %v", err)
	}
	total := 0
	sinClasificar := []string{}
	for _, e := range archivos {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		contenido, err := os.ReadFile(filepath.Clean(e.Name()))
		if err != nil {
			t.Fatalf("leyendo %s: %v", e.Name(), err)
		}
		for _, m := range reRegistroRuta.FindAllStringSubmatch(string(contenido), -1) {
			patron := m[1]
			// Los patrones con comillas embebidas o formato (%s) son
			// registro dinámico: no se puede prometer una frontera estática
			// para ellos, y el scaneo los nombra para revisión explícita.
			if strings.Contains(patron, "%") || strings.Contains(patron, `\"`) {
				t.Errorf("registro de ruta dinámico en %s: %q —clásifícalo con un caso de este test o fíjalo como patrón literal", e.Name(), patron)
				continue
			}
			ruta := patronARuta(patron)
			if !strings.HasPrefix(ruta, "/") {
				continue
			}
			total++
			clase, _ := clasificarRuta(ruta)
			if clase == "sin-clasificar" {
				sinClasificar = append(sinClasificar, e.Name()+": "+ruta)
			}
		}
	}
	if len(sinClasificar) > 0 {
		t.Errorf("%d ruta(s) registrada(s) sin frontera de permisos —añádelas a routeDepartment, sessionOnlyRoutes, publicRoutes, masterOnlyRoutes o routeDirectory con su POR QUÉ:\n  %s",
			len(sinClasificar), strings.Join(sinClasificar, "\n  "))
	}
	// Que el scaneo no se haya vuelto loco: el paquete registra cientos de
	// rutas; si este número cae en picada, el regex dejó de ver los registros
	// y el candado es decorativo.
	if total < 300 {
		t.Errorf("el escaneo solo encontró %d rutas: el regex no está leyendo el registro real del mux", total)
	}
}

// TestClasificarRutaCasosFijos fija las decisiones que el test de arriba no
// puede deducir solas: que una ruta de módulo NO sea pública, que una pública
// siga estándolo pese a sus sub-rutas, y que el prefijo más largo gane.
func TestClasificarRutaCasosFijos(t *testing.T) {
	casos := []struct {
		ruta  string
		clase string
		mod   int
	}{
		{"/product_price/save", "modulo", 3},
		{"/price_list/activate/7", "modulo", 3},
		{"/order/save", "modulo", 15},
		{"/delivery_order/list", "modulo", 15},
		{"/warehouse/save", "modulo", 6},
		{"/consumption/new", "modulo", 6},
		{"/finance/mensajes", "modulo", 9},
		{"/spidi/pagos/cobro-factura", "modulo", 9},
		{"/yummy/status", "modulo", 14},
		{"/ocr/process", "modulo", 5},
		{"/api/reports/currency-position", "modulo", 8},
		{"/settings/fiscal", "modulo", 14},
		{"/settings/company", "modulo", 14},
		{"/settings/backup", "master", 0},
		{"/data/backup/resp.sql", "master", 0},
		{"/user/profile", "sesion", 0},
		{"/user/change-password", "sesion", 0},
		{"/user/view-pref/save", "sesion", 0},
		{"/user/list", "master", 0},
		{"/role/save", "master", 0},
		{"/logout", "sesion", 0},
		{"/home", "sesion", 0},
		{"/", "sesion", 0},
		{"/dashboard/chart-data", "sesion", 0},
		{"/switch-tenant", "sesion", 0},
		{"/appearance/set", "sesion", 0},
		{"/guia/tour", "sesion", 0},
		{"/invoice/list", "modulo", 1},
		{"/ai/chat", "master", 0},
		{"/login", "publico", 0},
		{"/api/pos/sync/sale", "publico", 0},
		{"/webhooks/yummy", "publico", 0},
		{"/static/css/style.css", "publico", 0},
		{"/uploads/EMP123/doc.pdf", "publico", 0},
		{"/__addon/nota.crear", "publico", 0},
		{"/mod/memo/nota", "addon", 0},
		{"/reports/stock", "modulo", 8},
	}
	for _, c := range casos {
		clase, mod := clasificarRuta(c.ruta)
		if clase != c.clase || mod != c.mod {
			t.Errorf("clasificarRuta(%q) = (%q,%d), quiere (%q,%d)", c.ruta, clase, mod, c.clase, c.mod)
		}
	}
}
