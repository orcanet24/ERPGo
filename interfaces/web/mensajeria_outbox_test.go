package web

// Pruebas de la fase 2A: la cola, el ritmo y el opt-out.
//
// El reloj se pasa como parámetro (`ahora`), así que no hay esperas ni pruebas
// que fallen los sábados. Lo que se certifica es lo que puede costar caro:
// idempotencia, prioridad, horario, cupos, caída del daemon, fallo permanente,
// cortacircuito y opt-out.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// gowaFalso cuenta envíos y permite forzar un fallo.
type gowaFalso struct {
	mu     sync.Mutex
	envios int
	fallar int // código HTTP; 0 = éxito
	ultimo string
}

func (g *gowaFalso) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/send/message":
			var m map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&m)
			g.mu.Lock()
			if g.fallar != 0 {
				codigo := g.fallar
				g.mu.Unlock()
				w.WriteHeader(codigo)
				_, _ = w.Write([]byte(`{"code":` + itoa(codigo) + `,"message":"fallo de prueba"}`))
				return
			}
			g.envios++
			g.ultimo, _ = m["message"].(string)
			n := g.envios
			g.mu.Unlock()
			_, _ = w.Write([]byte(`{"code":200,"results":{"message_id":"M-` + itoa(n) + `","status":"sent"}}`))
		case "/send/chat-presence":
			_, _ = w.Write([]byte(`{"code":200}`))
		default:
			_, _ = w.Write([]byte(`{"code":200}`))
		}
	}))
}

func (g *gowaFalso) enviados() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.envios
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// fecha fija el reloj de las pruebas de la cola. Sin esto, una prueba de
// horario fallaría los sábados y pasaría los miércoles.
func fecha(t *testing.T, valor string) time.Time {
	t.Helper()
	f, err := time.ParseInLocation("2006-01-02 15:04", valor, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func baseCola(t *testing.T) (*Server, *sql.DB, *gowaFalso) {
	t.Helper()
	s, db := baseDispositivos(t)
	ddl := []string{
		`CREATE TABLE msg_outbox (id INTEGER PRIMARY KEY AUTOINCREMENT, tenant TEXT NOT NULL DEFAULT '',
			provider TEXT DEFAULT '', device_id TEXT NOT NULL DEFAULT '', priority TEXT DEFAULT 'transaccional',
			kind TEXT DEFAULT 'aviso', to_msisdn TEXT DEFAULT '', body TEXT, media_path TEXT DEFAULT '',
			media_type TEXT DEFAULT '', template_code TEXT DEFAULT '', ref_type TEXT DEFAULT '',
			ref_id TEXT DEFAULT '', idem_key TEXT NOT NULL UNIQUE,
			status TEXT DEFAULT 'en_cola', attempts INTEGER DEFAULT 0, send_after TEXT DEFAULT '',
			claimed_at TEXT DEFAULT '', provider_ref TEXT DEFAULT '', error TEXT DEFAULT '',
			created_by TEXT DEFAULT '', created_at TEXT DEFAULT '', updated_at TEXT DEFAULT '')`,
		`CREATE TABLE msg_optout (id INTEGER PRIMARY KEY AUTOINCREMENT, msisdn TEXT NOT NULL UNIQUE,
			reason TEXT DEFAULT '', source TEXT DEFAULT '', created_by TEXT DEFAULT '', created_at TEXT DEFAULT '')`,
		`CREATE TABLE msg_contact_channel (id INTEGER PRIMARY KEY AUTOINCREMENT, msisdn TEXT NOT NULL,
			provider TEXT NOT NULL, address TEXT DEFAULT '', verified_at TEXT DEFAULT '', source TEXT DEFAULT '',
			status TEXT DEFAULT 'verificado', preference INTEGER DEFAULT 0, notes TEXT DEFAULT '',
			UNIQUE (msisdn, provider))`,
		`INSERT INTO msg_device (device_id, tenant, area, state, daily_cap, hourly_cap, min_delay_s, max_delay_s, created_at)
			VALUES ('default.cxc', 'default', 'cxc', 'connected', 120, 25, 20, 20, '2026-10-01 08:00:00')`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar cola: %v\n%s", err, q)
		}
	}

	falso := &gowaFalso{}
	srv := falso.servidor(t)
	t.Cleanup(srv.Close)
	configurarServicio(t, s, db, srv.URL)
	return s, db, falso
}

func encolarDePrueba(t *testing.T, s *Server, ctl *sql.DB, cuerpo string, prioridad string) int64 {
	t.Helper()
	id, nuevo, err := s.msgEncolar(ctl, msgOutbox{
		Tenant: "default", DeviceID: "default.cxc", ToMSISDN: "584141234567",
		Body: cuerpo, Kind: "recordatorio", Priority: prioridad, RefType: "prueba", RefID: cuerpo,
		// Listo desde el principio: las pruebas fijan un «ahora» de 2026 y el
		// reloj real es posterior. Sin esto, la fila nunca estaría «vencida».
		SendAfter: "2026-01-01 00:00:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !nuevo {
		t.Fatalf("el mensaje %q no se encoló: ya existía", cuerpo)
	}
	return id
}

func estadoDeLaCola(t *testing.T, db *sql.DB, id int64) (string, int, string, string) {
	t.Helper()
	var estado, motivo, sendAfter string
	var intentos int
	if err := db.QueryRow(`SELECT status, attempts, error, send_after FROM msg_outbox WHERE id=?`, id).
		Scan(&estado, &intentos, &motivo, &sendAfter); err != nil {
		t.Fatal(err)
	}
	return estado, intentos, motivo, sendAfter
}

// ---------- Idempotencia y prioridad ----------

func TestEncolarElMismoAvisoDosVecesNoDuplica(t *testing.T) {
	s, db, _ := baseCola(t)
	id, nuevo, err := s.msgEncolar(db, msgOutbox{
		Tenant: "default", DeviceID: "default.cxc", ToMSISDN: "584141234567",
		Body: "Su factura venció", Kind: "recordatorio", RefType: "factura", RefID: "10",
	})
	if err != nil || !nuevo || id == 0 {
		t.Fatalf("primer encolado: %d %v %v", id, nuevo, err)
	}
	if _, nuevo, err := s.msgEncolar(db, msgOutbox{
		Tenant: "default", DeviceID: "default.cxc", ToMSISDN: "584141234567",
		Body: "Su factura venció", Kind: "recordatorio", RefType: "factura", RefID: "10",
	}); err != nil || nuevo {
		t.Fatalf("el segundo encolado debía ser idempotente: nuevo=%v err=%v", nuevo, err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_outbox`).Scan(&n)
	if n != 1 {
		t.Fatalf("filas en la cola = %d; debe ser 1", n)
	}
}

func TestLaColaRespetaLaPrioridad(t *testing.T) {
	s, db, _ := baseCola(t)
	encolarDePrueba(t, s, db, "recordatorio", "recordatorio")
	encolarDePrueba(t, s, db, "codigo", "otp")
	encolarDePrueba(t, s, db, "factura", "transaccional")

	reclamados, err := s.msgReclamar(db, fecha(t, "2026-10-28 10:00").Format("2006-01-02 15:04:05"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclamados) != 3 {
		t.Fatalf("se reclamaron %d; debían ser 3", len(reclamados))
	}
	if reclamados[0].Priority != "otp" || reclamados[1].Priority != "transaccional" || reclamados[2].Priority != "recordatorio" {
		t.Fatalf("orden inesperado: %s, %s, %s", reclamados[0].Priority, reclamados[1].Priority, reclamados[2].Priority)
	}
}

// ---------- Envío ----------

func TestEnvioExitosoGuardaElMensajeYProgramaLaPausa(t *testing.T) {
	s, db, falso := baseCola(t)
	id := encolarDePrueba(t, s, db, "Hola", "transaccional")
	ahora := fecha(t, "2026-10-28 10:00")

	s.msgDespacharUnaVez(t.Context(), ahora)

	estado, _, motivo, _ := estadoDeLaCola(t, db, id)
	if estado != "sent" && estado != msgAceptado {
		t.Fatalf("estado tras enviar: %q (%s)", estado, motivo)
	}
	if falso.enviados() != 1 {
		t.Fatalf("envíos = %d; debía ser 1", falso.enviados())
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_message WHERE direction='out'`).Scan(&n)
	if n != 1 {
		t.Fatalf("el historial no registró el envío: %d filas", n)
	}
	var siguiente string
	_ = db.QueryRow(`SELECT next_send_at FROM msg_device WHERE device_id='default.cxc'`).Scan(&siguiente)
	if msgFechaDe(siguiente).Before(ahora.Add(19 * time.Second)) {
		t.Fatalf("la pausa siguiente no se programó: %q", siguiente)
	}
}

func TestOptOutBloqueaElAvisoYNoLlamaAlDaemon(t *testing.T) {
	s, db, falso := baseCola(t)
	if _, err := db.Exec(`INSERT INTO msg_optout (msisdn, reason) VALUES ('584141234567', 'BAJA')`); err != nil {
		t.Fatal(err)
	}
	id := encolarDePrueba(t, s, db, "Recordatorio", "recordatorio")

	s.msgDespacharUnaVez(t.Context(), fecha(t, "2026-10-28 10:00"))

	estado, _, motivo, _ := estadoDeLaCola(t, db, id)
	if estado != msgBloqueado {
		t.Fatalf("debía quedar bloqueado; quedó %q", estado)
	}
	if !strings.Contains(motivo, "opt-out") {
		t.Fatalf("el motivo no explica el opt-out: %q", motivo)
	}
	if falso.enviados() != 0 {
		t.Fatal("se llamó al daemon con un contacto en opt-out")
	}
}

func TestFueraDeHorarioSeReprogramaYNoSePierde(t *testing.T) {
	s, db, falso := baseCola(t)
	id := encolarDePrueba(t, s, db, "Hola", "transaccional")

	// Sábado a las 10:00.
	s.msgDespacharUnaVez(t.Context(), fecha(t, "2026-10-31 10:00"))

	estado, _, motivo, sendAfter := estadoDeLaCola(t, db, id)
	if estado != msgEnCola {
		t.Fatalf("fuera de horario debía volver a la cola; quedó %q (%s)", estado, motivo)
	}
	quiero := fecha(t, "2026-11-02 08:00")
	if !msgFechaDe(sendAfter).Equal(quiero) {
		t.Fatalf("reprogramado para %q; se esperaba %s", sendAfter, quiero)
	}
	if falso.enviados() != 0 {
		t.Fatal("se envió fuera del horario permitido")
	}
}

func TestCupoDiarioAgotadoBloqueaConMotivo(t *testing.T) {
	s, db, falso := baseCola(t)
	if _, err := db.Exec(`UPDATE msg_device SET daily_cap = 1 WHERE device_id='default.cxc'`); err != nil {
		t.Fatal(err)
	}
	// Un envío ya hecho hoy.
	if _, err := db.Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		to_msisdn, body, status, is_from_me, created_at) VALUES ('default.cxc','whatsapp_gowa','x','out','ya-1',
		'584141234567','antes','sent',1,'2026-10-28 09:00:00')`); err != nil {
		t.Fatal(err)
	}
	id := encolarDePrueba(t, s, db, "Otro", "transaccional")

	s.msgDespacharUnaVez(t.Context(), fecha(t, "2026-10-28 10:00"))

	estado, _, motivo, _ := estadoDeLaCola(t, db, id)
	if estado != msgBloqueado || !strings.Contains(motivo, "cupo diario") {
		t.Fatalf("debía bloquearse por cupo: %q / %q", estado, motivo)
	}
	if falso.enviados() != 0 {
		t.Fatal("se envió con el cupo agotado")
	}
}

func TestDaemonCaidoMantieneElMensajeEnLaCola(t *testing.T) {
	s, db, falso := baseCola(t)
	falso.mu.Lock()
	falso.fallar = http.StatusInternalServerError
	falso.mu.Unlock()
	id := encolarDePrueba(t, s, db, "Hola", "transaccional")
	ahora := fecha(t, "2026-10-28 10:00")

	s.msgDespacharUnaVez(t.Context(), ahora)

	estado, intentos, motivo, sendAfter := estadoDeLaCola(t, db, id)
	if estado != msgEnCola {
		t.Fatalf("un 500 debe reintentarse; quedó %q", estado)
	}
	if intentos != 1 {
		t.Fatalf("intentos = %d; debía ser 1", intentos)
	}
	if !strings.Contains(motivo, "fallo de prueba") {
		t.Fatalf("no se guardó el motivo: %q", motivo)
	}
	if !msgFechaDe(sendAfter).After(ahora) {
		t.Fatalf("no se programó el reintento: %q", sendAfter)
	}
}

func TestUnFalloPermanenteNoSeReintentaYPausaElNumero(t *testing.T) {
	s, db, falso := baseCola(t)
	falso.mu.Lock()
	falso.fallar = http.StatusUnauthorized
	falso.mu.Unlock()
	id := encolarDePrueba(t, s, db, "Hola", "transaccional")
	ahora := fecha(t, "2026-10-28 10:00")

	s.msgDespacharUnaVez(t.Context(), ahora)

	estado, _, _, _ := estadoDeLaCola(t, db, id)
	if estado != msgFallido {
		t.Fatalf("un 401 no se reintenta; quedó %q", estado)
	}
	var pausado string
	_ = db.QueryRow(`SELECT paused_until FROM msg_device WHERE device_id='default.cxc'`).Scan(&pausado)
	if !msgFechaDe(pausado).After(ahora) {
		t.Fatalf("el número debía quedar en pausa: %q", pausado)
	}
}

func TestCincoFallosAbrenElCircuito(t *testing.T) {
	s, db, falso := baseCola(t)
	falso.mu.Lock()
	falso.fallar = http.StatusInternalServerError
	falso.mu.Unlock()
	for i := 0; i < 5; i++ {
		encolarDePrueba(t, s, db, "Mensaje "+itoa(i), "transaccional")
	}
	ahora := fecha(t, "2026-10-28 10:00")

	s.msgDespacharUnaVez(t.Context(), ahora)

	var pausado string
	_ = db.QueryRow(`SELECT paused_until FROM msg_device WHERE device_id='default.cxc'`).Scan(&pausado)
	if !msgFechaDe(pausado).After(ahora) {
		t.Fatalf("tras cinco fallos el número debía pausarse; paused_until=%q", pausado)
	}
}

// ---------- Opt-out y resolución de canal ----------

func TestOptOutDeTextoNoConfundeUnaFraseConUnaBaja(t *testing.T) {
	bajas := []string{"BAJA", " stop ", "No molestar", "DARME DE BAJA", "cancelar."}
	for _, b := range bajas {
		if !msgOptOutDeTexto(b) {
			t.Errorf("%q debía reconocerse como baja", b)
		}
	}
	noBajas := []string{"La factura está baja", "Baja prioridad, gracias", "STOPPING", ""}
	for _, b := range noBajas {
		if msgOptOutDeTexto(b) {
			t.Errorf("%q NO es una baja y se marcó como tal", b)
		}
	}
}

func TestResolverCanalRespetaPreferenciaYOrden(t *testing.T) {
	s, db, _ := baseCola(t)
	if _, _, motivo := s.msgResolverCanal(db, "584141234567", nil); motivo == "" {
		t.Fatal("sin canales verificados debía explicar el motivo")
	}

	_ = s.msgVerificarCanal(db, "584141234567", "whatsapp_gowa", "584141234567", "manual")
	_ = s.msgVerificarCanal(db, "584141234567", "telegram_user", "584141234567", "manual")
	prov, _, motivo := s.msgResolverCanal(db, "584141234567", nil)
	if motivo != "" || prov != "whatsapp_gowa" {
		t.Fatalf("el orden por defecto debía elegir WhatsApp: %q %q", prov, motivo)
	}

	if _, err := db.Exec(`UPDATE msg_contact_channel SET preference=1 WHERE provider='telegram_user'`); err != nil {
		t.Fatal(err)
	}
	prov, _, _ = s.msgResolverCanal(db, "584141234567", nil)
	if prov != "telegram_user" {
		t.Fatalf("la preferencia explícita debía mandar; eligió %q", prov)
	}
}

func TestElPrimerMensajeEntranteVerificaElCanalYRegistraLaBaja(t *testing.T) {
	s, db := baseDispositivos(t)
	if _, err := db.Exec(`CREATE TABLE msg_optout (id INTEGER PRIMARY KEY AUTOINCREMENT, msisdn TEXT NOT NULL UNIQUE,
		reason TEXT DEFAULT '', source TEXT DEFAULT '', created_by TEXT DEFAULT '', created_at TEXT DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE msg_contact_channel (id INTEGER PRIMARY KEY AUTOINCREMENT, msisdn TEXT NOT NULL,
		provider TEXT NOT NULL, address TEXT DEFAULT '', verified_at TEXT DEFAULT '', source TEXT DEFAULT '',
		status TEXT DEFAULT 'verificado', preference INTEGER DEFAULT 0, notes TEXT DEFAULT '',
		UNIQUE (msisdn, provider))`); err != nil {
		t.Fatal(err)
	}
	dispositivoDePrueba(t, s, db, "default", "default.cxc", "secreto")

	cuerpo := `{"event":"message","payload":{"id":"M1","from":"584141234567@s.whatsapp.net","body":"BAJA"}}`
	if res := enviarWebhook(s, "default.cxc", cuerpo, firmarWebhook(cuerpo, "secreto")); res.Code != http.StatusOK {
		t.Fatalf("webhook: %d %s", res.Code, res.Body.String())
	}

	if !msgEstaEnOptOut(db, "584141234567") {
		t.Fatal("la baja escrita por WhatsApp no se registró")
	}
	canales := s.msgCanalesDeContacto(db, "584141234567")
	if len(canales) != 1 || canales[0].Source != "mensaje_entrante" {
		t.Fatalf("el primer mensaje no verificó el canal: %+v", canales)
	}
}
