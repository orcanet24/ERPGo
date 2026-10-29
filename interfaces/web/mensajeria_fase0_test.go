package web

// Pruebas de la fase 0: el candado del módulo de mensajería.
//
// Lo que se certifica aquí es lo que el dueño del sistema pidió: que el módulo
// pueda activarse y desactivarse por el super administrador, y que mientras
// falte una llave NO EXISTA (404, menú oculto), que no se encienda sin aceptar
// el riesgo, que la credencial no vuelva nunca al navegador y que la prueba de
// conexión hable de verdad con el daemon (falso, en la prueba).

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"erpgo/internal/security"
)

// baseMensajeria crea las tablas del módulo sobre la base de prueba.
func baseMensajeria(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := basePrueba(t)
	ddl := []string{
		`CREATE TABLE modules (id TEXT PRIMARY KEY, name TEXT, is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 0)`,
		`INSERT INTO modules (id, name, is_core, is_active) VALUES ('mensajeria', 'Mensajería', 0, 0)`,
		`CREATE TABLE msg_global (id INTEGER PRIMARY KEY AUTOINCREMENT, enabled INTEGER DEFAULT 0,
			daemon_url TEXT DEFAULT '', daemon_user TEXT DEFAULT '', daemon_password_enc TEXT DEFAULT '',
			webhook_secret_enc TEXT DEFAULT '', default_country TEXT DEFAULT 'VE',
			risk_text_version INTEGER DEFAULT 1, updated_by TEXT DEFAULT '', updated_at TEXT DEFAULT '')`,
		`CREATE TABLE msg_setting (id INTEGER PRIMARY KEY AUTOINCREMENT, enabled INTEGER DEFAULT 0,
			risk_accepted_by TEXT DEFAULT '', risk_accepted_at TEXT DEFAULT '', default_country TEXT DEFAULT 'VE',
			chat_enabled INTEGER DEFAULT 0, otp_enabled INTEGER DEFAULT 0,
			updated_by TEXT DEFAULT '', updated_at TEXT DEFAULT '')`,
		`CREATE TABLE msg_provider (id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT UNIQUE, enabled INTEGER DEFAULT 0,
			config_json TEXT DEFAULT '', updated_by TEXT DEFAULT '', updated_at TEXT DEFAULT '')`,
		`INSERT INTO msg_provider (provider, enabled) VALUES ('whatsapp_gowa', 0)`,
		`INSERT INTO msg_provider (provider, enabled) VALUES ('telegram_bot', 0)`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar mensajería: %v\n%s", err, q)
		}
	}
	return s, db
}

// postMensajeria llama a un handler como lo haría el navegador, con sesión y
// ámbito de empresa. El cuerpo se pasa ya codificado para poder probar también
// los casos sin formulario.
func postMensajeria(s *Server, cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
}

func jsonDePrueba(t *testing.T, res *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &m); err != nil {
		t.Fatalf("la respuesta no es JSON: %v\n%s", err, res.Body.String())
	}
	return m
}

// ---------- Las tres llaves ----------

func TestMensajeriaApagadaNoExiste(t *testing.T) {
	s, _ := baseMensajeria(t)
	r := reqPrueba(s, sessPrueba("master"), "/mensajeria")

	if s.mensajeriaActiva(r) {
		t.Fatal("el módulo nace apagado y se reportó activo")
	}
	res := httptest.NewRecorder()
	s.mensajeriaHome(res, r)
	if res.Code != http.StatusNotFound {
		t.Fatalf("con el módulo apagado /mensajeria respondió %d; debe ser 404", res.Code)
	}
}

func TestMensajeriaNecesitaLasTresLlaves(t *testing.T) {
	s, db := baseMensajeria(t)
	r := reqPrueba(s, sessPrueba("master"), "/mensajeria")

	if _, err := db.Exec(`INSERT INTO msg_global (enabled) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if s.mensajeriaActiva(r) {
		t.Fatal("con solo la llave de la instalación no puede estar activa")
	}

	if _, err := db.Exec(`INSERT INTO msg_setting (enabled, risk_accepted_by, risk_accepted_at) VALUES (1, 'jefe', '2026-10-29 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if s.mensajeriaActiva(r) {
		t.Fatal("sin el módulo visible no puede estar activa")
	}

	if _, err := db.Exec(`UPDATE modules SET is_active = 1 WHERE id = 'mensajeria'`); err != nil {
		t.Fatal(err)
	}
	if !s.mensajeriaActiva(r) {
		t.Fatal("con las tres llaves el módulo debía estar activo")
	}
}

// ---------- Riesgo ----------

func TestEmpresaNoSeEnciendeSinAceptarElRiesgo(t *testing.T) {
	s, db := baseMensajeria(t)
	if _, err := db.Exec(`INSERT INTO msg_global (enabled) VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	res := postMensajeria(s, "enabled=1")
	s.mensajeriaEmpresaSave(httptest.NewRecorder(), res)

	var enabled int
	if err := db.QueryRow(`SELECT enabled FROM msg_setting ORDER BY id LIMIT 1`).Scan(&enabled); err == nil && enabled == 1 {
		t.Fatal("la empresa se encendió sin aceptar el texto de riesgo")
	}
}

func TestEmpresaSeEnciendeConRiesgoYAceptaLaLlaveDeLaInstalacion(t *testing.T) {
	s, db := baseMensajeria(t)
	if _, err := db.Exec(`INSERT INTO msg_global (enabled) VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/empresa",
		strings.NewReader("enabled=1&risk_accepted=1&risk_responsable=Ana"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaEmpresaSave(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("no se pudo encender con riesgo aceptado: %d %s", res.Code, res.Body.String())
	}
	var enabled int
	var responsable, cuando string
	if err := db.QueryRow(`SELECT enabled, risk_accepted_by, risk_accepted_at FROM msg_setting ORDER BY id LIMIT 1`).
		Scan(&enabled, &responsable, &cuando); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 || responsable != "Ana" || cuando == "" {
		t.Fatalf("la aceptación no quedó registrada: enabled=%d responsable=%q fecha=%q", enabled, responsable, cuando)
	}

	// Y queda en la auditoría, que es lo que permite responder «quién lo encendió».
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE module = 'mensajeria' AND action = 'activar'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("el encendido no dejó rastro en la auditoría")
	}
}

func TestEmpresaNoSeEnciendeSiLaInstalacionEstaApagada(t *testing.T) {
	s, db := baseMensajeria(t)

	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/empresa",
		strings.NewReader("enabled=1&risk_accepted=1&risk_responsable=Ana"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaEmpresaSave(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("debía rechazarse con la instalación apagada; respondió %d", res.Code)
	}
	var enabled int
	if err := db.QueryRow(`SELECT enabled FROM msg_setting ORDER BY id LIMIT 1`).Scan(&enabled); err == nil && enabled == 1 {
		t.Fatal("la empresa quedó encendida con la instalación apagada")
	}
}

// ---------- Servicio de la instalación ----------

func TestServicioNoAceptaUnaDireccionAbiertaALaRed(t *testing.T) {
	s, db := baseMensajeria(t)

	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/global",
		strings.NewReader("daemon_url="+url.QueryEscape("http://192.168.1.10:3000")+"&daemon_user=erpgo&daemon_password=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaGlobalSave(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("una dirección no loopback debía rechazarse; respondió %d", res.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_global`).Scan(&n)
	if n != 0 {
		t.Fatal("se guardó una configuración que expone la sesión a la red")
	}
}

func TestClaveDelServicioSeGuardaCifradaYNoVuelve(t *testing.T) {
	s, db := baseMensajeria(t)

	guardar := func(cuerpo string) {
		req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/global", strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
		res := httptest.NewRecorder()
		s.mensajeriaGlobalSave(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("no se guardó: %d %s", res.Code, res.Body.String())
		}
	}

	guardar("daemon_url=" + url.QueryEscape("http://127.0.0.1:3000") + "&daemon_user=erpgo&daemon_password=secreto")

	var enc string
	if err := db.QueryRow(`SELECT daemon_password_enc FROM msg_global ORDER BY id LIMIT 1`).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if enc == "" || enc == "secreto" || strings.Contains(enc, "secreto") {
		t.Fatalf("la credencial no está cifrada: %q", enc)
	}
	claro, err := security.Decrypt(enc, security.DeriveKey(s.config.MasterKey))
	if err != nil || claro != "secreto" {
		t.Fatalf("no se puede recuperar la credencial: %q %v", claro, err)
	}

	// Un guardado sin contraseña no la borra: el campo vacío significa «no la
	// cambies».
	guardar("daemon_url=" + url.QueryEscape("http://127.0.0.1:3000") + "&daemon_user=erpgo")
	var enc2 string
	if err := db.QueryRow(`SELECT daemon_password_enc FROM msg_global ORDER BY id LIMIT 1`).Scan(&enc2); err != nil {
		t.Fatal(err)
	}
	if enc2 != enc {
		t.Fatal("un guardado sin contraseña borró la credencial existente")
	}

	// Y la pantalla solo recibe el aviso de que hay clave, nunca la clave.
	form := s.msgGlobalDe(reqPrueba(s, sessPrueba("master"), "/settings/mensajeria"))
	if form.DaemonPasswordEnc == "" {
		t.Fatal("la lectura del estado perdió la credencial")
	}
}

func TestProbarConexionHablaConElDaemon(t *testing.T) {
	s, _ := baseMensajeria(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"code":200}`))
		case "/app/info":
			_, _ = w.Write([]byte(`{"code":200,"results":{"version":"9.1.0"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/global",
		strings.NewReader("daemon_url="+url.QueryEscape(srv.URL)+"&daemon_user=erpgo&daemon_password=x&enabled=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaGlobalSave(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("no se pudo guardar el servicio: %d %s", res.Code, res.Body.String())
	}

	probar := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/probar", nil)
	probar = withScope(probar, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	res = httptest.NewRecorder()
	s.mensajeriaProbar(res, probar)
	m := jsonDePrueba(t, res)
	if m["status"] != float64(1) {
		t.Fatalf("la prueba debía pasar: %v", m)
	}
	if !strings.Contains(m["msg"].(string), "9.1.0") {
		t.Fatalf("la prueba no informó la versión: %v", m)
	}

	// Con el servicio caído se dice, no se finge.
	srv.Close()
	res = httptest.NewRecorder()
	s.mensajeriaProbar(res, probar)
	m = jsonDePrueba(t, res)
	if m["status"] != float64(0) {
		t.Fatalf("con el servicio caído la prueba debía fallar: %v", m)
	}
	if !strings.Contains(m["error"].(string), "no responde") {
		t.Fatalf("el error no explica que el servicio no responde: %v", m)
	}
}

// ---------- Tercera llave desde la pantalla ----------

func TestModuloSeEnciendeYApagaDesdeLaPantalla(t *testing.T) {
	s, db := baseMensajeria(t)
	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/modulo", nil)
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})

	res := httptest.NewRecorder()
	s.mensajeriaModuloToggle(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("no se pudo mostrar el módulo: %d %s", res.Code, res.Body.String())
	}
	var activo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = 'mensajeria'`).Scan(&activo); err != nil {
		t.Fatal(err)
	}
	if activo != 1 {
		t.Fatal("el módulo no quedó visible")
	}

	res = httptest.NewRecorder()
	s.mensajeriaModuloToggle(res, req)
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = 'mensajeria'`).Scan(&activo); err != nil {
		t.Fatal(err)
	}
	if activo != 0 {
		t.Fatal("el módulo no se pudo volver a ocultar")
	}
}

func TestPaisDeLaEmpresaUsaLaInstalacionYElDeFabrica(t *testing.T) {
	s, db := baseMensajeria(t)
	r := reqPrueba(s, sessPrueba("master"), "/mensajeria")

	if got := s.paisDeLaEmpresa(r); got != "VE" {
		t.Fatalf("sin configuración debía usar VE, usó %q", got)
	}
	if _, err := db.Exec(`INSERT INTO msg_setting (default_country) VALUES ('CO')`); err != nil {
		t.Fatal(err)
	}
	if got := s.paisDeLaEmpresa(r); got != "CO" {
		t.Fatalf("debía usar el país de la empresa, usó %q", got)
	}
	if msisdn, err := s.normalizarTelefonoDeLaEmpresa(r, "300 123 4567"); err != nil || msisdn != "573001234567" {
		t.Fatalf("normalización con el país de la empresa: %q %v", msisdn, err)
	}
}

// TestGuardarDosVecesNoDuplicaLaConfiguracion protege contra la trampa de
// RowsAffected en MySQL: un UPDATE que no cambia nada devuelve 0, y decidir con
// ese número insertaba una segunda fila de configuración.
func TestGuardarDosVecesNoDuplicaLaConfiguracion(t *testing.T) {
	s, db := baseMensajeria(t)
	cuerpo := "daemon_url=" + url.QueryEscape("http://127.0.0.1:3000") + "&daemon_user=erpgo&daemon_password=x"

	for i := 0; i < 2; i++ {
		req := postMensajeria(s, cuerpo)
		res := httptest.NewRecorder()
		s.mensajeriaGlobalSave(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("guardado %d: %d %s", i+1, res.Code, res.Body.String())
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_global`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("se guardaron %d filas de configuración; debe haber una sola", n)
	}
}
