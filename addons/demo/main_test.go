package main

// main_test.go — Prueba de la pantalla del módulo demo.
//
// POR QUÉ EXISTE. La página se arma con fmt.Fprintf: un argumento fuera de
// orden no rompe la compilación, solo imprime `%!s(MISSING)` en mitad del
// formulario — el error aparece cuando el usuario ya está mirando la pantalla.
// Esta prueba pinta la página de verdad y mira que no sobre ni falte nada,
// además de que el formulario traiga el token CSRF sin el cual el núcleo
// rechaza el POST con 403 (enforceCSRF corre ANTES del proxy).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLaPantallaDelModuloDemoSePintaSinHuecosDeFormato(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/demo", nil)
	r.Header.Set("X-ERP-User", "7")
	r.Header.Set("X-ERP-Tenant", "default")
	r.Header.Set("X-ERP-DB", "erpgo_default")
	r.Header.Set("X-ERP-Auth-DB", "erpgo")
	r.Header.Set("X-ERP-Perms", "demo_ver,demo_publicar")
	r.Header.Set("X-ERP-CSRF", "token-de-prueba")

	w := httptest.NewRecorder()
	// Sin ERP_DB_READ_DSN las lecturas fallan a propósito: la pantalla tiene
	// que explicar el motivo en vez de quedarse a medias.
	renderizar(w, r, "fallo de prueba", "todo bien")
	cuerpo := w.Body.String()

	if strings.Contains(cuerpo, "%!") {
		t.Fatalf("la plantilla se quedó con argumentos de Fprintf sin cubrir:\n%s", cuerpo)
	}
	esperado := []string{
		"7", "default", "erpgo_default", "demo_ver,demo_publicar",
		"fallo de prueba", "todo bien",
		`name="csrf_token"`, "token-de-prueba",
		"ERP_DB_READ_DSN", // la lectura granular explica su fallo, no lo calla
	}
	for _, texto := range esperado {
		if !strings.Contains(cuerpo, texto) {
			t.Errorf("la pantalla no muestra %q", texto)
		}
	}
}

// TestElFormularioDelModuloDemoApuntaASuRuta: el formulario tiene que subir a
// la ruta que el manifiesto declara, no a otra.
func TestElFormularioDelModuloDemoApuntaASuRuta(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/demo", nil)
	w := httptest.NewRecorder()
	renderizar(w, r, "", "")
	if !strings.Contains(w.Body.String(), `action="/demo/aviso"`) {
		t.Fatal("el formulario del demo no apunta a /demo/aviso")
	}
}
