package main

// main_test.go — Prueba de la pantalla del módulo Memo.
//
// La página se arma con fmt.Fprintf: un argumento de más o de menos no rompe
// la compilación, solo imprime `%!s(MISSING)` en mitad del formulario. Aquí se
// pinta de verdad y se mira que no sobre nada, que el formulario lleva el
// token CSRF (sin él el núcleo responde 403 antes del proxy) y que el muro
// explica sus fallos de lectura en vez de quedarse mudo.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLaPantallaDeMemoSePintaSinHuecosDeFormato(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/memo", nil)
	r.Header.Set("X-ERP-User", "7")
	r.Header.Set("X-ERP-Tenant", "default")
	r.Header.Set("X-ERP-DB", "erpgo_default")
	r.Header.Set("X-ERP-Auth-DB", "erpgo")
	r.Header.Set("X-ERP-Perms", "memo_ver,memo_publicar")
	r.Header.Set("X-ERP-CSRF", "token-de-prueba")

	w := httptest.NewRecorder()
	// Sin ERP_DB_READ_DSN las dos lecturas fallan a propósito: la pantalla
	// tiene que decirlo, no quedarse a medias.
	muroConAviso(w, r, "fallo de prueba", "todo bien")
	cuerpo := w.Body.String()

	if strings.Contains(cuerpo, "%!") {
		t.Fatalf("la plantilla se quedó con argumentos de Fprintf sin cubrir:\n%s", cuerpo)
	}
	esperado := []string{
		"7", "default", "erpgo_default", "erpgo",
		"memo_ver,memo_publicar",
		"fallo de prueba", "todo bien",
		`name="csrf_token"`, "token-de-prueba",
		`action="/memo/nota"`,
		`name="visibilidad"`, `name="destinatarios"`,
		// Fase B: la nota se puede repartir por grupo y los grupos se crean y
		// se rellenan desde esta misma pantalla. Los tres selectores van SIEMPRE,
		// aunque la lectura falle: un campo que desaparece deja el formulario a
		// medias y sin decir qué falló.
		`value="grupo"`, `name="grupos"`,
		`action="/memo/grupo"`, `name="nombre"`, `name="miembros"`,
		// (el formulario de EDITAR miembros solo aparece cuando hay grupos;
		// sin DSN esta prueba no los carga, y ese caso lo cubre la lista real)
		"Grupos de trabajo",
		"ERP_DB_READ_DSN",
	}
	for _, texto := range esperado {
		if !strings.Contains(cuerpo, texto) {
			t.Errorf("la pantalla no muestra %q", texto)
		}
	}
}
