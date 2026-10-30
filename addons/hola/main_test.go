package main

// main_test.go — Prueba de la pantalla del addon demo hola.
//
// Mismo motivo que en addons/demo: la página se arma con fmt.Fprintf y un
// argumento de más solo se ve en pantalla como `%!s(EXTRA)`. Aquí se pintan
// TODOS los estados (sin aviso, con aviso y con resultado) porque es donde
// suelen descuadrar los argumentos.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLaPantallaDeHolaSePintaSinHuecosDeFormato(t *testing.T) {
	casos := []struct {
		nombre    string
		aviso     string
		resultado string
	}{
		{"sin nada", "", ""},
		{"con aviso", "algo falló", ""},
		{"con resultado", "", "guardado por el orquestador"},
	}
	for _, c := range casos {
		r := httptest.NewRequest(http.MethodGet, "/hola", nil)
		r.Header.Set("X-ERP-User", "7")
		r.Header.Set("X-ERP-Tenant", "default")
		r.Header.Set("X-ERP-Perms", "hola_ver")
		r.Header.Set("X-ERP-CSRF", "token-de-prueba")
		w := httptest.NewRecorder()
		renderizarPantalla(w, r, c.aviso, c.resultado)
		cuerpo := w.Body.String()
		if strings.Contains(cuerpo, "%!") {
			t.Fatalf("%s: la plantilla se quedó con argumentos de Fprintf sin cubrir:\n%s", c.nombre, cuerpo)
		}
		for _, e := range []string{"7", "default", "hola_ver", "token-de-prueba", `name="csrf_token"`} {
			if !strings.Contains(cuerpo, e) {
				t.Errorf("%s: la pantalla no muestra %q", c.nombre, e)
			}
		}
		if c.aviso != "" && !strings.Contains(cuerpo, c.aviso) {
			t.Errorf("%s: no se ve el aviso", c.nombre)
		}
		if c.resultado != "" && !strings.Contains(cuerpo, c.resultado) {
			t.Errorf("%s: no se ve el resultado", c.nombre)
		}
	}
}
