package web

// candados_s3_s10_test.go — Pruebas de los candados de la fase 3 (seguridad
// de sesión/identidad/transporte, auditoría atómica 2026-11-22). Cada una
// fija la regla POR LA QUE SE CAMBIÓ el código: si alguien la revierte, el
// test la delata.

import (
	"crypto/tls"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"erpgo/configs"
)

func TestSesionActivaNoVenceFija(t *testing.T) {
	// U-6/S-3 (addendum): la sesión con actividad se desliza; sin actividad
	// vence. TouchSliding solo escribe cuando queda menos de MEDIA vida, así
	// que renovar no es escribir en cada petición.
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS web_sessions (
		id TEXT PRIMARY KEY, user_id TEXT, username TEXT, fullname TEXT, user_level TEXT,
		image TEXT, tenant_code TEXT, is_agent INTEGER, csrf_token TEXT, expires_at TEXT, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	s.sessions = NewSessionStore()
	s.sessions.SetDB(db)

	reciente := s.sessions.Create(&SessionData{UserID: "9", Username: "caja", ExpiresAt: time.Now().Add(23 * time.Hour)})
	vieja := s.sessions.Create(&SessionData{UserID: "9", Username: "caja", ExpiresAt: time.Now().Add(2 * time.Hour)})

	s.sessions.TouchSliding(reciente, duracionSesion)
	if got := s.sessions.Get(reciente); !got.ExpiresAt.After(time.Now().Add(23*time.Hour - time.Second)) {
		t.Error("una sesión con casi toda la vida por delante NO debía renovarse todavía")
	}
	s.sessions.TouchSliding(vieja, duracionSesion)
	if got := s.sessions.Get(vieja); time.Until(got.ExpiresAt) < 23*time.Hour {
		t.Errorf("sesión a punto de vencer con actividad: expira en %v, se quería ~24 h", time.Until(got.ExpiresAt))
	}
	// Y la base sigue al día (es Get tras reinicio lo que la lee):
	var exp string
	if err := db.QueryRow(`SELECT expires_at FROM web_sessions WHERE id = ?`, vieja).Scan(&exp); err == nil {
		tt, err2 := time.Parse("2006-01-02 15:04:05", exp)
		if err2 == nil && time.Until(tt) < 23*time.Hour {
			t.Errorf("la base quedó con vencimiento viejo: %s", exp)
		}
	}
}

func TestDeleteByUserCierraCacheYBase(t *testing.T) {
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS web_sessions (
		id TEXT PRIMARY KEY, user_id TEXT, username TEXT, fullname TEXT, user_level TEXT,
		image TEXT, tenant_code TEXT, is_agent INTEGER, csrf_token TEXT, expires_at TEXT, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	s.sessions = NewSessionStore()
	s.sessions.SetDB(db)

	s1 := s.sessions.Create(&SessionData{UserID: "42", Username: "cajero", ExpiresAt: time.Now().Add(time.Hour)})
	s2 := s.sessions.Create(&SessionData{UserID: "42", Username: "cajero", ExpiresAt: time.Now().Add(time.Hour)})
	otra := s.sessions.Create(&SessionData{UserID: "43", Username: "otro", ExpiresAt: time.Now().Add(time.Hour)})

	if n := s.sessions.DeleteByUser("42"); n != 2 {
		t.Fatalf("DeleteByUser cerró %d sesiones, se querían 2", n)
	}
	if s.sessions.Get(s1) != nil || s.sessions.Get(s2) != nil {
		t.Error("las sesiones revocadas siguen vivas en la caché")
	}
	if s.sessions.Get(otra) == nil {
		t.Error("DeleteByUser cerró la sesión de OTRO usuario: revoca por identidad, no global")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM web_sessions WHERE user_id = '42'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("quedan %d filas de sesión en la base tras revocar", n)
	}
}

func TestClientIPNoCreeXFFSinProxyDeConfianza(t *testing.T) {
	configurarProxyConfianza("") // estado por defecto: nadie es proxy

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.9:52233"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := clientIP(req); got != "203.0.113.9" {
		t.Errorf("sin proxy declarado, clientIP creyó la cabecera: %q (se quería la IP TCP)", got)
	}

	t.Cleanup(func() { configurarProxyConfianza("") })
	configurarProxyConfianza("127.0.0.1, 10.0.0.0/8")
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.RemoteAddr = "127.0.0.1:41111"
	req2.Header.Set("X-Forwarded-For", "8.8.8.8, 9.9.9.9")
	if got := clientIP(req2); got != "8.8.8.8" {
		t.Errorf("desde proxy de confianza se esperaba la primera XFF (8.8.8.8), llegó %q", got)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req3.RemoteAddr = "10.1.2.3:40000"
	req3.Header.Set("X-Forwarded-For", "7.7.7.7")
	if got := clientIP(req3); got != "7.7.7.7" {
		t.Errorf("la red CIDR declarada no se respetó: %q", got)
	}
	req4 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req4.RemoteAddr = "172.16.0.9:40000" // fuera de la lista
	req4.Header.Set("X-Forwarded-For", "6.6.6.6")
	if got := clientIP(req4); got != "172.16.0.9" {
		t.Errorf("una IP fuera de la lista de confianza no debe habilitar la cabecera: %q", got)
	}
}

func TestGalletaSesionPoneSecureCuandoHayTLS(t *testing.T) {
	s, _ := basePrueba(t)

	// HTTP plano (despliegue LAN normal): SIN Secure — una cookie Secure en
	// http es una cookie que el navegador tira.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	s.escribirGalletaSesion(rec, r, "id-de-prueba", 86400)
	c := galletasDe(rec)[0]
	if c.Secure {
		t.Error("Secure puesto sobre http plano: la cookie quedaría inutilizable en LAN")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("flags básicos rotos: HttpOnly=%v SameSite=%v", c.HttpOnly, c.SameSite)
	}

	// HTTPS real: Secure exigido (S-5).
	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.TLS = &tls.ConnectionState{}
	s.escribirGalletaSesion(rec2, r2, "id-segura", 86400)
	if c2 := galletasDe(rec2)[0]; !c2.Secure {
		t.Error("sirviendo por https la cookie de sesión sigue sin Secure")
	}

	// TLS configurado aunque la petición actual llegue por http (proxy que
	// termina TLS): el operador declaró https, la cookie se emite Secure.
	rec3 := httptest.NewRecorder()
	sTLS := &Server{config: &configs.Config{AppEnv: "test", TLSCertFile: "server.crt"}}
	sTLS.escribirGalletaSesion(rec3, httptest.NewRequest(http.MethodGet, "/", nil), "id", 1)
	if c3 := galletasDe(rec3)[0]; !c3.Secure {
		t.Error("con TLS configurado la cookie debería marcarse Secure")
	}
}

func TestHSTSConfiguradoYNoForzadoEnHTTP(t *testing.T) {
	s, _ := basePrueba(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := s.corsMiddleware(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/home", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS enviado sobre http: fijaría en el navegador la prohibición de volver al único acceso que funciona")
	}

	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/home", nil)
	r2.TLS = &tls.ConnectionState{}
	h.ServeHTTP(rec2, r2)
	if hsts := rec2.Header().Get("Strict-Transport-Security"); !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("bajo https falta HSTS (llegó %q)", hsts)
	}
}

func TestDbForTenantEmpresaInaccesibleDevuelveNil(t *testing.T) {
	s, _ := basePrueba(t)
	// D-7/S-9: la empresa que no abre NO devuelve la base activa. Antes era
	// el fallback silencioso: una petición de B leía y escribía los datos de
	// A. Con nil, el llamador responde 503 y nadie escribe en empresa ajena.
	if db := s.dbForTenant("empresa-que-no-existe"); db != nil {
		t.Error("dbForTenant devolvió una base para una empresa inexistente: el fallback silencioso sigue vivo")
	}
}

func TestCadenaAuditoriaConClaveVerificaAmbosFormatos(t *testing.T) {
	s, db := basePrueba(t)
	s.config = &configs.Config{AppEnv: "test", MasterKey: "clave-maestra-de-prueba"}

	r := httptest.NewRequest(http.MethodGet, "/audit/log", nil)
	ts := time.Now().Format("2006-01-02 15:04:05")

	// Primera fila: formato CLÁSICO (sha pelado), como las escribió el ERP
	// antes de la clave. Enlazada desde GENESIS.
	res, err := db.Exec(`INSERT INTO audit_log (user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at, prev_hash, record_hash)
		VALUES ('antiguo','prueba-vieja','mod','tabla','1','{}','{}','127.0.0.1', ?, 'GENESIS', '')`, ts)
	if err != nil {
		t.Fatal(err)
	}
	idVieja, _ := res.LastInsertId()
	huellaVieja := computeAuditHash("GENESIS", idVieja, "antiguo", "prueba-vieja", "mod", "tabla", "1", "{}", "{}", "127.0.0.1", ts, "")
	if _, err := db.Exec(`UPDATE audit_log SET record_hash = ? WHERE id = ?`, huellaVieja, idVieja); err != nil {
		t.Fatal(err)
	}

	// Segunda fila: la escribe el código nuevo → "h2:" (HMAC simple con la
	// MasterKey), enlazada a la clásica. La cadena mixta debe verificar.
	s.appendAuditAt(r, "dueño", "127.0.0.1", "prueba", "mod", "tabla", "2", nil, map[string]string{"a": "1"}, time.Now())
	var huellaNueva string
	var prevNuevo string
	var idNueva int64
	if err := db.QueryRow(`SELECT id, prev_hash, record_hash FROM audit_log WHERE action = 'prueba'`).Scan(&idNueva, &prevNuevo, &huellaNueva); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(huellaNueva, "h2:") {
		t.Fatalf("con MasterKey configurado la huella nueva debía ser h2:, llegó %q", huellaNueva)
	}

	res2, err := s.auditChainRange(r, "1970-01", "2099-12")
	if err != nil {
		t.Fatalf("verificando cadena mixta: %v", err)
	}
	if res2.BrokenAt != 0 {
		t.Errorf("la cadena mixta (clásica + h2) se declaró rota en %d: %s", res2.BrokenAt, res2.Reason)
	}

	// Y la alteración SÍ se detecta: cambiar una fila sin la clave no puede
	// producir una huella válida.
	if _, err := db.Exec(`UPDATE audit_log SET new_value = '{"a":"MAL"}' WHERE id = ?`, idNueva); err != nil {
		t.Fatal(err)
	}
	res3, err := s.auditChainRange(r, "1970-01", "2099-12")
	if err != nil {
		t.Fatal(err)
	}
	if res3.BrokenAt == 0 {
		t.Error("un contenido alterado pasó la verificación de la cadena")
	}
}

func TestReescrituraDePasswordVaALaBaseCorrecta(t *testing.T) {
	// El primitivo que usan login web y POS: reescribe bcrypt SOLO en la base
	// que se le pasa (control). Una segunda copia de la regla (antes el POS
	// reescribía la copia de la empresa) es la copia que se queda vieja.
	db, err := persistenceOpenSQLiteForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE user_login (user_id INTEGER, username TEXT, password TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_login (user_id, username, password) VALUES (77, 'cajera', 'plano-legado')`); err != nil {
		t.Fatal(err)
	}
	if err := reescribirPassword(db, 77, "cajera", "secreto-nuevo"); err != nil {
		t.Fatalf("reescribirPassword: %v", err)
	}
	var guardado string
	if err := db.QueryRow(`SELECT password FROM user_login WHERE user_id = 77`).Scan(&guardado); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(guardado, "$2") {
		t.Errorf("tras la reescritura no quedó bcrypt: %q", guardado)
	}
	if ok, _ := verificarPassword(guardado, "secreto-nuevo"); !ok {
		t.Error("la clave reescrita no verifica")
	}
}

// persistenceOpenSQLiteForTest abre una base SQLite de pruebas en el
// directorio temporal del test (para pruebas que necesitan tablas fuera del
// juego mínimo de crearTablasPrueba).
func persistenceOpenSQLiteForTest(t *testing.T) (*sql.DB, error) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "candado.sqlite3"))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, nil
}

// galletasDe lee las cookies de la respuesta. Result().Cookies es método
// desde Go 1.26; envolverlo aquí deja claro por qué no se indexa el campo.
func galletasDe(rec *httptest.ResponseRecorder) []*http.Cookie {
	return rec.Result().Cookies()
}
