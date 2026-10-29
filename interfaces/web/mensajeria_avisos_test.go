package web

// Pruebas de la integración de la fase 2: el canal «gowa» de los avisos.
//
// Lo que se certifica: un aviso de cartera con canal GOWA NO llama al daemon
// desde el handler; entra en la cola con el teléfono NORMALIZADO y el historial
// dice «en cola», no «enviado». Y un teléfono ambiguo se rechaza con motivo en
// vez de encolarse para fallar en silencio.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func baseAvisos(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db, _ := baseCola(t)
	ddl := []string{
		`CREATE TABLE customer_information (customer_id INTEGER PRIMARY KEY, customer_name TEXT,
			customer_email TEXT, email_address TEXT, customer_mobile TEXT)`,
		`INSERT INTO customer_information (customer_id, customer_name, customer_email, customer_mobile)
			VALUES (1, 'Cliente', 'cliente@ejemplo.com', '0414-1234567')`,
		`CREATE TABLE notify_channel (id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL DEFAULT 'email',
			name TEXT NOT NULL UNIQUE, is_active INTEGER DEFAULT 1, ai_allowed INTEGER DEFAULT 0,
			cooldown_days INTEGER DEFAULT 3, host TEXT, port INTEGER DEFAULT 587, username TEXT, password TEXT,
			from_address TEXT, from_name TEXT, security TEXT DEFAULT 'starttls', url TEXT,
			http_method TEXT DEFAULT 'POST', auth_header TEXT, auth_token TEXT, sign_secret TEXT,
			msg_device_id TEXT DEFAULT '', msg_template_code TEXT DEFAULT '', notes TEXT, created_at TEXT DEFAULT '', updated_at TEXT DEFAULT '')`,
		`CREATE TABLE finance_message (id INTEGER PRIMARY KEY AUTOINCREMENT, channel_id INTEGER DEFAULT 0,
			channel TEXT DEFAULT '', channel_kind TEXT DEFAULT '', recipient TEXT DEFAULT '', subject TEXT DEFAULT '',
			body TEXT, ref_type TEXT DEFAULT '', ref_id TEXT DEFAULT '', status TEXT DEFAULT 'enviando', error TEXT,
			provider_ref TEXT, idem_key TEXT NOT NULL UNIQUE, created_by TEXT, created_at TEXT NOT NULL, sent_at TEXT)`,
		`INSERT INTO notify_channel (kind, name, is_active, ai_allowed, cooldown_days, msg_device_id, created_at, updated_at)
			VALUES ('gowa', 'WhatsApp cobranza', 1, 0, 0, 'default.cxc', '', '')`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar avisos: %v\n%s", err, q)
		}
	}
	return s, db
}

func TestUnAvisoConCanalGowaEntraEnLaColaYNoLlamaAlDaemon(t *testing.T) {
	s, db := baseAvisos(t)
	canal, err := s.notifyCanalPorID(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, sessPrueba("master"), "/finance/mensajes")
	alerta := map[string]interface{}{"party_type": "customer", "party_id": int64(1)}

	res, err := s.financeEnviarAviso(r, db, canal, alerta, "Factura vencida", "Saldo pendiente",
		"alerta", "1", false)
	if err != nil {
		t.Fatal(err)
	}
	if res["enviado"] != true || res["encolado"] != true {
		t.Fatalf("la respuesta debía decir que quedó en cola: %v", res)
	}

	var msisdn, estado string
	if err := db.QueryRow(`SELECT to_msisdn, status FROM msg_outbox ORDER BY id LIMIT 1`).Scan(&msisdn, &estado); err != nil {
		t.Fatal(err)
	}
	if msisdn != "584141234567" {
		t.Fatalf("el teléfono se encoló sin normalizar: %q", msisdn)
	}
	if estado != msgEnCola {
		t.Fatalf("estado en la cola: %q", estado)
	}

	var estadoFin string
	if err := db.QueryRow(`SELECT status FROM finance_message ORDER BY id LIMIT 1`).Scan(&estadoFin); err != nil {
		t.Fatal(err)
	}
	if estadoFin != "en_cola" {
		t.Fatalf("el historial dice %q; un aviso encolado no está enviado", estadoFin)
	}
}

func TestUnTelefonoAmbiguoSeRechazaYQuedaRegistradoComoError(t *testing.T) {
	s, db := baseAvisos(t)
	if _, err := db.Exec(`UPDATE customer_information SET customer_mobile = '1234567' WHERE customer_id = 1`); err != nil {
		t.Fatal(err)
	}
	canal, _ := s.notifyCanalPorID(db, 1)
	r := reqPrueba(s, sessPrueba("master"), "/finance/mensajes")
	alerta := map[string]interface{}{"party_type": "customer", "party_id": int64(1)}

	res, err := s.financeEnviarAviso(r, db, canal, alerta, "Factura", "Cuerpo", "alerta", "1", false)
	if err != nil {
		t.Fatal(err)
	}
	if res["enviado"] == true {
		t.Fatal("un teléfono ambiguo no puede darse por enviado")
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_outbox`).Scan(&n)
	if n != 0 {
		t.Fatal("se encoló un aviso con un teléfono ambiguo")
	}
	var estado, motivo string
	if err := db.QueryRow(`SELECT status, IFNULL(error,'') FROM finance_message ORDER BY id LIMIT 1`).Scan(&estado, &motivo); err != nil {
		t.Fatal(err)
	}
	if estado != "error" || motivo == "" {
		t.Fatalf("el fallo debía quedar registrado con motivo: %q %q", estado, motivo)
	}
}

func TestElCanalGowaSeGuardaConSuDispositivo(t *testing.T) {
	s, db := baseAvisos(t)
	form := url.Values{
		"kind":          {"gowa"},
		"name":          {"WA RRHH"},
		"is_active":     {"1"},
		"cooldown_days": {"0"},
		"msg_device_id": {"default.rrhh"},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/notify/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.notifyChannelSave(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("no se guardó el canal: %d %s", res.Code, res.Body.String())
	}
	var dispositivo string
	if err := db.QueryRow(`SELECT msg_device_id FROM notify_channel WHERE name = 'WA RRHH'`).Scan(&dispositivo); err != nil {
		t.Fatal(err)
	}
	if dispositivo != "default.rrhh" {
		t.Fatalf("dispositivo guardado: %q", dispositivo)
	}
}
