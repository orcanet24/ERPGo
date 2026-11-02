package web

// Pruebas de GOti: la guía demo no debe dar 500 al cargar.

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	// AplicarEsquema tolera que las tablas ya existan.
	for _, q := range guiaDDLForTest() {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// La caché de tenantEsDemo y el marcador guiaTablaOk son GLOBALES al
	// proceso de pruebas: otro test del paquete (o el test GOti anterior)
	// pudo haber grabado default→no-demo, o default→tablas-ya-preparadas con
	// SU base; entonces el test actual recibiría {"active":false} o una base
	// sin sembrar. Cada test de GOti arranca con ambos registros limpios.
	guiaDemoCacheMu.Lock()
	delete(guiaDemoCache, "default")
	guiaDemoCacheMu.Unlock()
	guiaTabla_mu.Lock()
	delete(guiaTablaOk, "default")
	guiaTabla_mu.Unlock()

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

// TestGuiaProgressExigeTokenCSRF: /guia/ ya NO está exento del CSRF. El
// POST /guia/progress cambia estado (guia_progreso) y se pide igual que
// cualquier mutación de la app; goti.js lo manda correctamente vía
// ERPGo.csrfFetch, así que la exención histórica solo quitaba protección.
func TestGuiaProgressExigeTokenCSRF(t *testing.T) {
	s := &Server{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Con sesión y token en ella, POST /guia/progress SIN la cabecera → 403.
	req := httptest.NewRequest(http.MethodPost, "/guia/progress", nil)
	res := httptest.NewRecorder()
	s.enforceCSRF(res, req, next, &SessionData{CSRFToken: "token-de-sesion"})
	if res.Code != http.StatusForbidden {
		t.Fatalf("/guia/progress sin token debe dar 403; dio %d: %s", res.Code, res.Body.String())
	}

	// Con la cabecera correcta pasa: es el camino de goti.js + csrfFetch.
	reqOK := httptest.NewRequest(http.MethodPost, "/guia/progress", nil)
	reqOK.Header.Set(csrfHeader, "token-de-sesion")
	resOK := httptest.NewRecorder()
	s.enforceCSRF(resOK, reqOK, next, &SessionData{CSRFToken: "token-de-sesion"})
	if resOK.Code != http.StatusOK {
		t.Fatalf("/guia/progress con el token de la sesión debe pasar; dio %d: %s", resOK.Code, resOK.Body.String())
	}

	// Y un token INCORRECTO también cae: la comparación es contra el de la sesión.
	reqMal := httptest.NewRequest(http.MethodPost, "/guia/progress", nil)
	reqMal.Header.Set(csrfHeader, "otro-valor")
	resMal := httptest.NewRecorder()
	s.enforceCSRF(resMal, reqMal, next, &SessionData{CSRFToken: "token-de-sesion"})
	if resMal.Code != http.StatusForbidden {
		t.Fatalf("/guia/progress con token ajeno debe dar 403; dio %d", resMal.Code)
	}
}

// TestGuiaProgresoSuperviveReinicio: el ciclo completo de progreso de un
// usuario del tour, con el RE-SEMBRADO en medio (lo que cada reinicio del
// ERP hace contra la misma base). El defecto que originó este test: el
// DELETE+INSERT de la siembra re-creaba los pasos con ids nuevos y el
// progreso — que va por paso_id — quedaba huérfano: el usuario arrancaba de
// cero en cada reinicio del servidor.
func TestGuiaProgresoSuperviveReinicio(t *testing.T) {
	s, db := baseGuia(t)
	sess := &SessionData{UserID: "7", Username: "usuario", UserLevel: "admin"}

	// (0) GET inicial: el tour arranca en el paso 1.
	idsDe := func(pasos []map[string]interface{}) map[int]int64 {
		m := map[int]int64{}
		for _, p := range pasos {
			m[int(toInt(p["orden"]))] = toInt(p["id"])
		}
		return m
	}
	resp := func() (map[string]interface{}, map[int]int64) {
		req := httptest.NewRequest(http.MethodGet, "/guia/active", nil)
		req = withScope(req, &scope{tenant: "default", db: db, sess: sess})
		res := httptest.NewRecorder()
		s.guiaActive(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("guia/active respondió %d: %s", res.Code, res.Body.String())
		}
		raw := map[string]interface{}{}
		if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if raw["active"] != true {
			t.Fatalf("guia/active no activo: %s", res.Body.String())
		}
		pasos, _ := raw["pasos"].([]interface{})
		normalizada := []map[string]interface{}{}
		for _, p := range pasos {
			normalizada = append(normalizada, p.(map[string]interface{}))
		}
		return raw, idsDe(normalizada)
	}
	progresa := func(pasoID int64, accion string) {
		t.Helper()
		cuerpo := fmt.Sprintf(`{"paso_id":%d,"action":"%s"}`, pasoID, accion)
		req := httptest.NewRequest(http.MethodPost, "/guia/progress", strings.NewReader(cuerpo))
		req = withScope(req, &scope{tenant: "default", db: db, sess: sess})
		res := httptest.NewRecorder()
		s.guiaProgress(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("guia/progress %s del paso %d dio %d: %s", accion, pasoID, res.Code, res.Body.String())
		}
	}

	_, ids1 := resp()
	if len(ids1) != 6 {
		t.Fatalf("el tour sembrado debe tener 6 pasos; tiene %d", len(ids1))
	}
	// (1) El usuario marca skip el paso 1, oculta el 2 y hecho el 3.
	progresa(ids1[1], "skip")
	progresa(ids1[2], "hide")
	progresa(ids1[3], "done")
	// El actual ya es el paso 4: skip del 1, hidden del 2 (¡también avanza!) y
	// done del 3. Este es el primer punto de verificación del ciclo.
	if raw, _ := resp(); toInt(raw["actual_id"]) != ids1[4] {
		t.Fatalf("antes del re-sembrado el actual debe ser el paso 4 (%d); dio %v", ids1[4], raw["actual_id"])
	}
	// (2) Re-sembrado: es exactamente lo que hace el arranque del proceso con
	// la misma base. El cache del proceso se limpia para forzar el camino.
	guiaTabla_mu.Lock()
	delete(guiaTablaOk, "default")
	guiaTabla_mu.Unlock()
	if err := ensureGuiaTables(db); err != nil {
		t.Fatalf("re-sembrado: %v", err)
	}

	// (3) Los ids SE CONSERVAN: el progreso quedó referido a los mismos pasos.
	_, ids2 := resp()
	for orden, id1 := range ids1 {
		if ids2[orden] != id1 {
			t.Fatalf("el paso de orden %d cambió de id tras el re-sembrado: %d → %d", orden, id1, ids2[orden])
		}
	}

	// (4) Y el estado sobrevive: el actual ahora es el paso 4 (el 1 va skip, el
	// 2 hidden — ¡también avanza! — y el 3 done).
	raw, _ := resp()
	if actual := toInt(raw["actual_id"]); actual != ids2[4] {
		t.Fatalf("después de skip+hide+done el actual debe ser el paso de orden 4 (%d); dio %d", ids2[4], actual)
	}

	// (5) El paso con check (orden 5) refusa done sin la factura de respaldo: la
	// verificación determinista corre y sin datos no pasa, así que no se marca.
	progresa(ids2[5], "done")
	raw, _ = resp()
	if actual := toInt(raw["actual_id"]); actual != ids2[4] {
		t.Fatalf("el done sin check pasado no debe avanzar el paso (actual sigue en el 4); dio %d", actual)
	}
}
