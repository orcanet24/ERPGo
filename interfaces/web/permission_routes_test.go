package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"erpgo/configs"
)

func TestRutasPosAdministrativasNoAceptanSesionesAusentes(t *testing.T) {
	s := &Server{config: &configs.Config{AppEnv: "test"}, mux: http.NewServeMux()}
	s.registerPOSAdminRoutes()
	handler := s.permissionMiddleware(s.mux)

	casos := []struct {
		ruta   string
		metodo string
	}{
		{"/admin/pos/branches", http.MethodGet},
		{"/admin/pos/branch/create", http.MethodPost},
		{"/admin/pos/registers", http.MethodGet},
		{"/admin/pos/apikeys", http.MethodGet},
		{"/admin/pos/apikey/create", http.MethodPost},
		{"/pos/api/dashboard", http.MethodGet},
		{"/pos/api/terminals", http.MethodGet},
		{"/config/menu", http.MethodGet},
		{"/config/menu/move", http.MethodPost},
	}
	for _, caso := range casos {
		t.Run(caso.metodo+" "+caso.ruta, func(t *testing.T) {
			req := httptest.NewRequest(caso.metodo, caso.ruta, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusFound {
				t.Fatalf("sin sesión, %s respondió %d; debe redirigir al inicio de sesión", caso.ruta, res.Code)
			}
		})
	}
}

func TestRutasPosAdministrativasExigenRolMaster(t *testing.T) {
	s, db := basePrueba(t)
	if _, err := db.Exec(`INSERT INTO user_login (username, password) VALUES ('usuario', 'hash')`); err != nil {
		t.Fatal(err)
	}
	s.sessions = NewSessionStore()
	sess := sessPrueba("manager")
	sess.Username = "usuario"
	// Sin ExpiresAt la sesión se considera vencida al instante (Get la borra) y
	// el middleware respondería 302 al login en vez de evaluar el rol.
	sess.ExpiresAt = time.Now().Add(time.Hour)
	id := s.sessions.Create(sess)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /pos/api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]int{"ok": 1})
	})
	handler := s.authMiddleware(s.permissionMiddleware(mux))
	req := httptest.NewRequest(http.MethodGet, "/pos/api/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "erpgow_session", Value: id})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("un gerente sin rol master obtuvo HTTP %d; debe quedar bloqueado, respuesta: %s", res.Code, res.Body.String())
	}
}
