package web

// Pruebas de la fase 1: dispositivos, QR y webhook.
//
// Lo que se certifica aquí es la parte que puede filtrar datos o duplicar
// historial: que el alta hable con el daemon y guarde el secreto cifrado, que un
// identificador retirado no se recicle, que el webhook rechace una firma
// inválida, que resuelva la empresa por el DISPOSITIVO y no por el cuerpo, y que
// un reintento de GOWA no duplique el mensaje.

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo/configs"
	"erpgo/internal/security"
)

func baseDispositivos(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := baseMensajeria(t)
	s.config.AppURL = "http://localhost:8080"
	s.tenantDBs = map[string]*sql.DB{}

	ddl := []string{
		`CREATE TABLE msg_device (id INTEGER PRIMARY KEY AUTOINCREMENT, device_id TEXT UNIQUE NOT NULL,
			tenant TEXT NOT NULL, area TEXT NOT NULL DEFAULT 'general', label TEXT DEFAULT '',
			provider TEXT DEFAULT 'whatsapp_gowa', msisdn TEXT UNIQUE, webhook_secret_enc TEXT DEFAULT '',
			state TEXT DEFAULT 'nuevo', jid TEXT DEFAULT '', daily_cap INTEGER DEFAULT 120,
			hourly_cap INTEGER DEFAULT 25, min_delay_s INTEGER DEFAULT 45, max_delay_s INTEGER DEFAULT 180,
			warmup_day INTEGER DEFAULT 0, next_send_at TEXT DEFAULT '', paused_until TEXT DEFAULT '',
			last_seen_at TEXT DEFAULT '', created_by TEXT DEFAULT '', created_at TEXT DEFAULT '')`,
		`CREATE TABLE msg_message (id INTEGER PRIMARY KEY AUTOINCREMENT, device_id TEXT NOT NULL,
			provider TEXT DEFAULT '', chat_jid TEXT DEFAULT '', direction TEXT DEFAULT 'out',
			message_id TEXT NOT NULL, from_msisdn TEXT DEFAULT '', to_msisdn TEXT DEFAULT '', body TEXT,
			media_type TEXT DEFAULT '', media_path TEXT DEFAULT '', status TEXT DEFAULT 'aceptado',
			is_from_me INTEGER DEFAULT 0, sent_at TEXT DEFAULT '', received_at TEXT DEFAULT '',
			raw_json TEXT, created_at TEXT DEFAULT '', UNIQUE (device_id, message_id))`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar dispositivos: %v\n%s", err, q)
		}
	}
	return s, db
}

// servidorGowaDeDispositivos imita lo justo del daemon para el alta y el QR.
func servidorGowaDeDispositivos(t *testing.T, creado *map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/devices" && r.Method == http.MethodPost:
			var m map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&m)
			if creado != nil {
				*creado = m
			}
			_, _ = w.Write([]byte(`{"code":200}`))
		case strings.HasSuffix(r.URL.Path, "/login"):
			_, _ = w.Write([]byte(`{"code":200,"results":{"qr_link":"data:image/png;base64,iVBORw0KGgo=","qr_duration":30}}`))
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = w.Write([]byte(`{"code":200,"results":{"state":"connected","jid":"584141234567@s.whatsapp.net","msisdn":"584141234567"}}`))
		case strings.HasSuffix(r.URL.Path, "/reconnect"):
			_, _ = w.Write([]byte(`{"code":200}`))
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"code":200}`))
		default:
			_, _ = w.Write([]byte(`{"code":200}`))
		}
	}))
}

func configurarServicio(t *testing.T, s *Server, db *sql.DB, urlServicio string) {
	t.Helper()
	enc, err := security.Encrypt("x", security.DeriveKey(s.config.MasterKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.msgGuardarGlobal(db, msgGlobal{
		Enabled: true, DaemonURL: urlServicio, DaemonUser: "erpgo", DaemonPasswordEnc: enc,
		DefaultCountry: "VE", RiskTextVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func peticionDeDispositivo(s *Server, cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/dispositivos/crear", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
}

// ---------- Alta ----------

func TestCrearDispositivoLlamaAlDaemonYGuardaElSecretoCifrado(t *testing.T) {
	s, db := baseDispositivos(t)
	var creado map[string]interface{}
	srv := servidorGowaDeDispositivos(t, &creado)
	defer srv.Close()
	configurarServicio(t, s, db, srv.URL)

	req := peticionDeDispositivo(s, "area=cxc&label=Cobranza")
	res := httptest.NewRecorder()
	s.mensajeriaDeviceCreate(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("no se creó: %d %s", res.Code, res.Body.String())
	}

	var deviceID, tenant, area, label, enc string
	if err := db.QueryRow(`SELECT device_id, tenant, area, label, webhook_secret_enc FROM msg_device`).
		Scan(&deviceID, &tenant, &area, &label, &enc); err != nil {
		t.Fatal(err)
	}
	if deviceID != "default.cxc" || tenant != "default" || area != "cxc" || label != "Cobranza" {
		t.Fatalf("fila inesperada: %q %q %q %q", deviceID, tenant, area, label)
	}
	if enc == "" || strings.Contains(enc, creado["webhook_secret"].(string)) {
		t.Fatal("el secreto del webhook no está cifrado")
	}
	claro, err := security.Decrypt(enc, security.DeriveKey(s.config.MasterKey))
	if err != nil || claro != creado["webhook_secret"] {
		t.Fatalf("el secreto guardado no es el que recibió el daemon: %v", err)
	}
	if !strings.HasSuffix(creado["webhook_url"].(string), "/webhooks/mensajeria/default.cxc") {
		t.Fatalf("URL de webhook inesperada: %v", creado["webhook_url"])
	}
}

func TestUnIdentificadorRetiradoNoSeReutiliza(t *testing.T) {
	s, db := baseDispositivos(t)
	srv := servidorGowaDeDispositivos(t, nil)
	defer srv.Close()
	configurarServicio(t, s, db, srv.URL)

	if _, err := db.Exec(`INSERT INTO msg_device (device_id, tenant, area, state, webhook_secret_enc)
		VALUES ('default.cxc', 'default', 'cxc', 'retirado', 'x')`); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	s.mensajeriaDeviceCreate(res, peticionDeDispositivo(s, "area=cxc"))
	if res.Code != http.StatusConflict {
		t.Fatalf("un identificador retirado debía rechazarse con 409; respondió %d %s", res.Code, res.Body.String())
	}
}

func TestElQRSeSirveComoImagenYNoComoEnlaceAlDaemon(t *testing.T) {
	s, db := baseDispositivos(t)
	srv := servidorGowaDeDispositivos(t, nil)
	defer srv.Close()
	configurarServicio(t, s, db, srv.URL)

	enc, _ := security.Encrypt("secreto", security.DeriveKey(s.config.MasterKey))
	if _, err := db.Exec(`INSERT INTO msg_device (device_id, tenant, area, state, webhook_secret_enc)
		VALUES ('default.cxc', 'default', 'cxc', 'nuevo', ?)`, enc); err != nil {
		t.Fatal(err)
	}

	req := reqPrueba(s, sessPrueba("master"), "/settings/mensajeria/dispositivos/default.cxc/qr")
	req.SetPathValue("id", "default.cxc")
	res := httptest.NewRecorder()
	s.mensajeriaDeviceQR(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("el QR debía servirse: %d %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("tipo de contenido inesperado: %q", got)
	}
	if res.Body.Len() == 0 {
		t.Fatal("la imagen del QR llegó vacía")
	}
	if strings.Contains(res.Body.String(), "http") {
		t.Fatal("se está devolviendo el enlace al daemon en vez de la imagen")
	}
}

// ---------- Webhook ----------

func firmarWebhook(cuerpo, secreto string) string {
	mac := hmac.New(sha256.New, []byte(secreto))
	mac.Write([]byte(cuerpo))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func dispositivoDePrueba(t *testing.T, s *Server, db *sql.DB, tenant, deviceID, secreto string) {
	t.Helper()
	enc, err := security.Encrypt(secreto, security.DeriveKey(s.config.MasterKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO msg_device (device_id, tenant, area, state, webhook_secret_enc)
		VALUES (?, ?, 'general', 'connected', ?)`, deviceID, tenant, enc); err != nil {
		t.Fatal(err)
	}
}

func enviarWebhook(s *Server, deviceID, cuerpo, firma string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mensajeria/"+deviceID, strings.NewReader(cuerpo))
	req.SetPathValue("device", deviceID)
	if firma != "" {
		req.Header.Set("X-Hub-Signature-256", firma)
	}
	res := httptest.NewRecorder()
	s.mensajeriaWebhook(res, req)
	return res
}

func TestWebhookRechazaUnaFirmaInvalida(t *testing.T) {
	s, db := baseDispositivos(t)
	dispositivoDePrueba(t, s, db, "default", "default.cxc", "secreto")

	cuerpo := `{"event":"message","payload":{"id":"M1","from":"584141234567@s.whatsapp.net","body":"Hola"}}`
	res := enviarWebhook(s, "default.cxc", cuerpo, "sha256=0000000000000000000000000000000000000000000000000000000000000000")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("una firma falsa debía dar 401; dio %d", res.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_message`).Scan(&n)
	if n != 0 {
		t.Fatal("se escribió un mensaje con firma inválida")
	}

	// Sin cabecera, tampoco.
	if res := enviarWebhook(s, "default.cxc", cuerpo, ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("sin firma debía dar 401; dio %d", res.Code)
	}
}

func TestWebhookGuardaUnaVezAunqueGowaReintente(t *testing.T) {
	s, db := baseDispositivos(t)
	dispositivoDePrueba(t, s, db, "default", "default.cxc", "secreto")

	cuerpo := `{"event":"message","payload":{"id":"M1","from":"584141234567@s.whatsapp.net","chat_id":"584141234567@s.whatsapp.net","body":"Hola","is_from_me":false}}`
	firma := firmarWebhook(cuerpo, "secreto")

	for i := 0; i < 3; i++ {
		res := enviarWebhook(s, "default.cxc", cuerpo, firma)
		if res.Code != http.StatusOK {
			t.Fatalf("reintento %d respondió %d: %s", i+1, res.Code, res.Body.String())
		}
	}

	var n int
	var chat, cuerpoGuardado, estado string
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_message`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("el reintento duplicó el historial: %d filas", n)
	}
	if err := db.QueryRow(`SELECT chat_jid, body, status FROM msg_message`).Scan(&chat, &cuerpoGuardado, &estado); err != nil {
		t.Fatal(err)
	}
	if chat != "584141234567@s.whatsapp.net" || cuerpoGuardado != "Hola" || estado != "recibido" {
		t.Fatalf("mensaje guardado inesperado: chat=%q body=%q estado=%q", chat, cuerpoGuardado, estado)
	}
}

func TestWebhookDeUnDispositivoDesconocidoEs404(t *testing.T) {
	s, _ := baseDispositivos(t)
	res := enviarWebhook(s, "no.existe", `{"event":"message"}`, "sha256=00")
	if res.Code != http.StatusNotFound {
		t.Fatalf("un dispositivo desconocido debía dar 404; dio %d", res.Code)
	}
}

func TestWebhookEscribeEnLaEmpresaDelDispositivoYNoEnLaActiva(t *testing.T) {
	s, db := baseDispositivos(t)
	otra := abrirBasePrueba(t)
	if _, err := otra.Exec(`CREATE TABLE msg_message (id INTEGER PRIMARY KEY AUTOINCREMENT, device_id TEXT NOT NULL,
		provider TEXT DEFAULT '', chat_jid TEXT DEFAULT '', direction TEXT DEFAULT 'out',
		message_id TEXT NOT NULL, from_msisdn TEXT DEFAULT '', to_msisdn TEXT DEFAULT '', body TEXT,
		media_type TEXT DEFAULT '', media_path TEXT DEFAULT '', status TEXT DEFAULT 'aceptado',
		is_from_me INTEGER DEFAULT 0, sent_at TEXT DEFAULT '', received_at TEXT DEFAULT '',
		raw_json TEXT, created_at TEXT DEFAULT '', UNIQUE (device_id, message_id))`); err != nil {
		t.Fatal(err)
	}
	s.tenantDBs["otra"] = otra
	dispositivoDePrueba(t, s, db, "otra", "otra.cxc", "secreto-otra")

	cuerpo := `{"event":"message","payload":{"id":"M9","from":"584141234567@s.whatsapp.net","body":"De otra empresa"}}`
	res := enviarWebhook(s, "otra.cxc", cuerpo, firmarWebhook(cuerpo, "secreto-otra"))
	if res.Code != http.StatusOK {
		t.Fatalf("el webhook debía aceptarse: %d %s", res.Code, res.Body.String())
	}

	var enOtra, enActiva int
	_ = otra.QueryRow(`SELECT COUNT(*) FROM msg_message`).Scan(&enOtra)
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_message`).Scan(&enActiva)
	if enOtra != 1 {
		t.Fatalf("el mensaje no llegó a la empresa del dispositivo (%d)", enOtra)
	}
	if enActiva != 0 {
		t.Fatal("el mensaje de otra empresa se escribió en la base activa: fuga entre empresas")
	}
}

func TestWebhookAplicaElAcuseSinInventarLaEntrega(t *testing.T) {
	s, db := baseDispositivos(t)
	dispositivoDePrueba(t, s, db, "default", "default.cxc", "secreto")

	entrada := `{"event":"message","payload":{"id":"M1","from":"584141234567@s.whatsapp.net","body":"Hola"}}`
	if res := enviarWebhook(s, "default.cxc", entrada, firmarWebhook(entrada, "secreto")); res.Code != http.StatusOK {
		t.Fatalf("entrada: %d", res.Code)
	}
	var estado string
	_ = db.QueryRow(`SELECT status FROM msg_message WHERE message_id = 'M1'`).Scan(&estado)
	if estado != "recibido" {
		t.Fatalf("sin acuse el estado debía ser recibido; es %q", estado)
	}

	acuse := `{"event":"message.ack","payload":{"id":"M1","ack":2}}`
	if res := enviarWebhook(s, "default.cxc", acuse, firmarWebhook(acuse, "secreto")); res.Code != http.StatusOK {
		t.Fatalf("acuse: %d", res.Code)
	}
	_ = db.QueryRow(`SELECT status FROM msg_message WHERE message_id = 'M1'`).Scan(&estado)
	if estado != "entregado" {
		t.Fatalf("el acuse 2 debía dejar el mensaje en entregado; quedó %q", estado)
	}
}

func TestElWebhookEsPublicoPeroNoAceptaOtroMetodo(t *testing.T) {
	// El middleware deja pasar /webhooks/ sin sesión (lo autentica el HMAC) y
	// el mux solo registra POST: un GET no puede escribir nada.
	s := &Server{config: s2Config(), mux: http.NewServeMux()}
	s.registerMensajeriaDispositivoRoutes()
	handler := s.permissionMiddleware(s.mux)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/mensajeria/default.cxc", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code == http.StatusFound {
		t.Fatalf("el webhook no puede redirigir al login: el daemon no tiene sesión (dio %d)", res.Code)
	}
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("un GET al webhook debía dar 405; dio %d", res.Code)
	}
}

// s2Config evita depender de basePrueba para la prueba del middleware.
func s2Config() *configs.Config {
	return &configs.Config{AppEnv: "test", AppName: "ERPGo"}
}
