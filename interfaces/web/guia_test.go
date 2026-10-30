package web

// Pruebas de GOti: la guía demo no debe dar 500 al cargar.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"erpgo/configs"
)

func baseGuia(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := basePrueba(t)
	s.config = &configs.Config{AppEnv: "test", AppName: "ERPGo"}
	s.activeTenant = "default"
	// La empresa demo vive en control (= db en las pruebas).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tenants (code VARCHAR(64) PRIMARY KEY, is_demo INTEGER DEFAULT 0, status INTEGER DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tenants (code, is_demo, status) VALUES ('default',1,1)
		ON DUPLICATE KEY UPDATE is_demo=1`); err != nil {
		// SQLite no entiende ON DUPLICATE: INSERT OR REPLACE.
		if _, err2 := db.Exec(`INSERT OR REPLACE INTO tenants (code, is_demo, status) VALUES ('default',1,1)`); err2 != nil {
			t.Fatal(err2)
		}
	}
	return s, db
}

func TestGuiaActiveNoDa500EnLaEmpresaDemo(t *testing.T) {
	s, _ := baseGuia(t)
	sess := &SessionData{UserID: "7", Username: "usuario", UserLevel: "admin"}
	req := httptest.NewRequest(http.MethodGet, "/guia/active", nil)
	req = withScope(req, &scope{tenant: "default", db: s.db, sess: sess})
	res := httptest.NewRecorder()
	s.guiaActive(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("guia/active respondió %d: %s", res.Code, res.Body.String())
	}
	var r struct {
		Active bool   `json:"active"`
		Code   string `json:"code"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if !r.Active || r.Code != "facturar_manual" {
		t.Fatalf("GOti no devolvió el tour activo: %+v", r)
	}
}

func TestGuiaActiveRespondeFalseEnProduccion(t *testing.T) {
	s, db := basePrueba(t)
	s.config = &configs.Config{AppEnv: "test"}
	s.activeTenant = "prod"
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tenants (code VARCHAR(64) PRIMARY KEY, is_demo INTEGER DEFAULT 0, status INTEGER DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO tenants (code, is_demo, status) VALUES ('prod',0,1)`); err != nil {
		t.Fatal(err)
	}
	sess := &SessionData{UserID: "7", UserLevel: "admin"}
	req := httptest.NewRequest(http.MethodGet, "/guia/active", nil)
	req = withScope(req, &scope{tenant: "prod", db: db, sess: sess})
	res := httptest.NewRecorder()
	s.guiaActive(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("esperaba 200 con active=false; dio %d: %s", res.Code, res.Body.String())
	}
	var r struct {
		Active bool `json:"active"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &r)
	if r.Active {
		t.Fatal("una empresa de producción no debe activar GOti")
	}
}
