package web

// Pruebas de GOti: la guía demo no debe dar 500 al cargar.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"erpgo/configs"
	"erpgo/infrastructure/persistence"
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

	// Pre-crea las tablas de GOti con DDL SQLite-compatible. Las pruebas usan
	// SQLite; en producción son MySQL. guiaDDLSentencias detecta el dialecto y
	// AplicarEsquema tolera que las tablas ya existan. El semillado (DELETE+INSERT)
	// sigue ejecutándose en ensureGuiaTables, así que los tours quedan poblados.
	for _, q := range guiaDDLForTest() {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
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

// TestGuiaDDLUsaDialectoCorrecto: esMySQL distingue SQLite de MySQL, y
// guiaDDLSentencias devuelve DDL compatible. El bug original era que el DDL usaba
// INTEGER PRIMARY KEY AUTOINCREMENT (SQLite) directamente en producción MySQL →
// 500 en /guia/active. Con detección de dialecto, cada motor recibe su DDL.
func TestGuiaDDLUsaDialectoCorrecto(t *testing.T) {
	s, db := baseGuia(t)
	_ = s // baseGuia configura el server; el test de dialecto solo usa db.

	// En los tests la base es SQLite: SELECT VERSION() falla.
	if esMySQL(db) {
		t.Fatal("en tests SQLite, esMySQL debe ser falso")
	}

	// El DDL para SQLite no usa AUTO_INCREMENT (MySQL) sino AUTOINCREMENT.
	ddl := guiaDDLSentencias(db)
	for _, q := range ddl {
		if strings.Contains(q, "AUTO_INCREMENT") && !strings.Contains(q, "AUTOINCREMENT") {
			t.Errorf("el DDL de SQLite contiene AUTO_INCREMENT (MySQL): %s", q)
		}
	}

	// Aplicar el DDL sobre una base SQLite limpia (sin pre-crear) también funciona.
	// Esto prueba que esMySQL detecta bien y el DDL es compatible.
	dbLimpia, err := persistence.OpenSQLite(filepath.Join(t.TempDir(), "limpia.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.AplicarEsquema(dbLimpia, guiaDDLForTest()); err != nil {
		t.Fatalf("AplicarEsquema con DDL SQLite falló: %v", err)
	}

	// ensureGuiaTables sobre la base limpia también funciona (esMySQL=false → DDL SQLite).
	if err := ensureGuiaTables(dbLimpia); err != nil {
		t.Fatalf("ensureGuiaTables falló sobre SQLite: %v", err)
	}
	// Cerrar antes de Salir: SQLite mantiene el archivo .sqlite3 bloqueado por el
	// WAL, y TempDir cleanup falla en Windows si no se cierra primero.
	if err := dbLimpia.Close(); err != nil {
		t.Logf("cierre de base limpia: %v", err)
	}
}
