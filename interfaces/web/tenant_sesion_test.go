package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCrearEmpresaDejaLaSesionEnLaEmpresaNueva.
//
// setTenant() abre la base de la empresa nueva, pero quien decide en qué base
// escribe cada petición es la empresa de la SESIÓN. Sin este cambio, el super
// administrador que creaba una empresa veía el panel de la nueva y guardaba el
// almacén, el usuario y el plan de cuentas en la empresa anterior.
func TestCrearEmpresaDejaLaSesionEnLaEmpresaNueva(t *testing.T) {
	s := &Server{sessions: NewSessionStore()}
	sess := &SessionData{
		UserID: "7", Username: "admin", UserLevel: "master",
		TenantCode: defaultTenantCode,
	}
	r := httptest.NewRequest("POST", "/setup/company", nil)
	r = withScope(r, &scope{tenant: defaultTenantCode, sess: sess})
	r.AddCookie(&http.Cookie{Name: "erpgow_session", Value: "sesion-vieja"})
	w := httptest.NewRecorder()

	if !s.sesionEnEmpresa(w, r, "panadera_el_trigal_ca") {
		t.Fatal("con sesión viva debía activar la empresa nueva")
	}
	if sess.TenantCode != "panadera_el_trigal_ca" {
		t.Fatalf("la sesión quedó en %q y debía quedar en la empresa nueva", sess.TenantCode)
	}

	// El id de sesión se rota: el contexto de datos cambió, y una cookie vieja
	// no puede seguir sirviendo para la empresa anterior.
	rotada := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "erpgow_session" && c.Value != "" && c.Value != "sesion-vieja" {
			rotada = true
		}
	}
	if !rotada {
		t.Fatal("no se emitió una cookie de sesión nueva al cambiar de empresa")
	}

	// Sin sesión (primera instalación) no hay nada que cambiar: el asistente
	// manda al login y no debe inventar una empresa activa.
	sinSesion := withScope(httptest.NewRequest("POST", "/setup/company", nil), &scope{})
	if s.sesionEnEmpresa(httptest.NewRecorder(), sinSesion, "otra_empresa") {
		t.Fatal("sin sesión no debía activar ninguna empresa")
	}
}
