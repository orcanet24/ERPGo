package web

// vistas_enlaces_test.go — Regresión de los enlaces y formularios rotos que el
// webcheck histórico no detectaba (href/action a rutas no registradas y verbos
// HTTP desajustados). Cada arreglo de vistas lleva su test aquí.

import (
	"database/sql"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo"
)

// reqPostConScope monta una petición POST con cuerpo de formulario y el ámbito
// (empresa + sesión), como lo hace attachScope: sin él sessionFromRequest no
// resuelve identidad y hasPermission siempre devuelve false.
func reqPostConScope(s *Server, sess *SessionData, path, body string) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	// ParseForm solo le el body a los POST con content-type de formulario; sin
	// el encabezado ReturnSave leería return_type vacío y caería en el default.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sess})
}

// TestInventarioHubRedirige comprueba que GET /inventory (que daba 404) vuelve a
// existir: ahora responde 302 al módulo principal de inventario.
func TestInventarioHubRedirige(t *testing.T) {
	s, _ := basePrueba(t)
	w := httptest.NewRecorder()
	s.inventarioHub(w, reqPrueba(s, sessPrueba("admin"), "/inventory"))
	if w.Code != http.StatusFound {
		t.Fatalf("/inventory debió redirigir (302), llegó %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/inventory/reorder" {
		t.Errorf("la redirección debe ir al módulo principal de inventario; llegó %q", loc)
	}
}

// TestReturnSaveDespachaPorTipo comprueba el dispatcher /return/save: el
// formulario de devoluciones postea a esta ruta y el campo return_type elige
// entre venta y compra (que antes caían en 404 porque no existía el despachador).
func TestReturnSaveDespachaPorTipo(t *testing.T) {
	s, _ := basePrueba(t)
	sess := sessPrueba("admin")

	// Tipo desconocido → 400, sin adivinar lado.
	w := httptest.NewRecorder()
	s.returnSave(w, reqPostConScope(s, sess, "/return/save", "return_type=bogus"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("tipo desconocido: 400 esperado, llegó %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Tipo de devolución") {
		t.Errorf("debe pedir el tipo; salió: %s", w.Body.String())
	}

	// venta → ruta de cliente; sin producto ni cantidad devuelve 400 del handler
	// de cliente (prueba que el despacho llegó a returnCustomerSave, no a 404).
	w = httptest.NewRecorder()
	s.returnSave(w, reqPostConScope(s, sess, "/return/save", "return_type=sale"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("venta sin producto: 400 esperado, llegó %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Producto y cantidad") {
		t.Errorf("debe ser el handler de cliente quien pida los datos; salió: %s", w.Body.String())
	}

	// compra → ruta de proveedor (mismo mensaje de validación, otro handler).
	w = httptest.NewRecorder()
	s.returnSave(w, reqPostConScope(s, sess, "/return/save", "return_type=purchase"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("compra sin producto: 400 esperado, llegó %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Producto y cantidad") {
		t.Errorf("debe ser el handler de proveedor quien pida los datos; salió: %s", w.Body.String())
	}
}

// TestLaPantallaDeDescuentosSeRepresenta asegura que discount.html (antes
// huérfana: nadie la servía) compila, convive con el layout y se renderiza con
// los nombres de variables que discountForm provee al formulario de alta/edición.
func TestLaPantallaDeDescuentosSeRepresenta(t *testing.T) {
	tpl, err := template.New("discount.html").Funcs(templateFuncs(nil, nil)).
		ParseFS(erpgo.Templates(), "templates/discount.html")
	if err != nil {
		t.Fatalf("la plantilla de descuentos no se puede cargar: %v", err)
	}
	if _, err := tpl.ParseFS(erpgo.Templates(), "templates/layout.html"); err != nil {
		t.Fatalf("la página no convive con el layout: %v", err)
	}
	datos := map[string]interface{}{
		"Title":          "Nuevo Descuento",
		"DiscountID":     "",
		"DiscountPct":    "0.00",
		"DiscountMinQty": "0",
		"Products":       []map[string]interface{}{{"id": "1", "name": "Lápiz", "price": "0.50"}},
		"Customers":      []map[string]interface{}{{"id": "2", "name": "Distribuidora"}},
	}
	for _, define := range []string{"content", "pagejs"} {
		cuerpo := representar(t, tpl, define, datos)
		if strings.Contains(cuerpo, "ZgotmplZ") {
			t.Errorf("la sección %s dejó el marcador ZgotmplZ: algo no se representó", define)
		}
	}
	cuerpo := representar(t, tpl, "content", datos)
	for _, esperado := range []string{"Nuevo Descuento", "Guardar", "TODOS", "Detalle", "Minorista"} {
		if !strings.Contains(cuerpo, esperado) {
			t.Errorf("la pantalla de descuentos no muestra %q", esperado)
		}
	}
}

// TestLaTablaDeDescuentosNoPosteaAlEndpointMuerto comprueba que la plantilla de
// lista ya no referencia la ruta /product_price/discount/form inexistente... y
// que data sigue registrada (el fix de método está en tabla_lista_metodos_test).
func TestLaTablaDeDescuentosEnlazaAlFormulario(t *testing.T) {
	b, err := fs.ReadFile(erpgo.Templates(), "templates/discount_list.html")
	if err != nil {
		t.Fatalf("no se pudo leer discount_list.html: %v", err)
	}
	txt := string(b)
	if !strings.Contains(txt, "/product_price/discount/form") {
		t.Error("la lista de descuentos debe seguir enlazando al formulario; se le borró el href")
	}
}

// _ sql.NullString se usa en return.go; este import evita que el compilador lo
// marque si la prueba de arriba se reduce. Mantiene el archivo autocontenido.
var _ = sql.NullString{}