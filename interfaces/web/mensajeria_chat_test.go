package web

// Pruebas de la fase 3A: bandeja, hilo y envío de texto.
//
// Lo que se certifica: que la bandeja agrupa por conversación y no mezcla
// empresas (la base ya es la de la empresa), que el hilo solo trae su
// conversación, que el envío normaliza el teléfono y pasa por la cola, que un
// usuario sin el dispositivo asignado no ve nada, y que un mensaje de chat no
// espera la pausa de los avisos.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func insertarMensaje(t *testing.T, db *sql.DB, device, jid, direccion, cuerpo, cuando string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		from_msisdn, to_msisdn, body, status, is_from_me, sent_at, received_at, raw_json, created_at)
		VALUES (?, 'whatsapp_gowa', ?, ?, ?, '', ?, ?, 'recibido', ?, ?, '', '', ?)`,
		device, jid, direccion, "m-"+cuerpo+"-"+cuando, jid, cuerpo, boolAInt(direccion == "out"), cuando, cuando); err != nil {
		t.Fatal(err)
	}
}

func TestLaBandejaAgrupaPorConversacion(t *testing.T) {
	s, db := baseAreas(t)
	insertarMensaje(t, db, "default.cxc", "584141111111@s.whatsapp.net", "in", "Hola", "2026-10-28 09:00:00")
	insertarMensaje(t, db, "default.cxc", "584141111111@s.whatsapp.net", "out", "Buenos días", "2026-10-28 09:05:00")
	insertarMensaje(t, db, "default.cxc", "584142222222@s.whatsapp.net", "in", "Otra", "2026-10-28 10:00:00")

	req := reqPrueba(s, sessPrueba("master"), "/mensajeria/conversaciones")
	res := httptest.NewRecorder()
	s.mensajeriaConversacionesJSON(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("bandeja: %d %s", res.Code, res.Body.String())
	}
	var r struct {
		Conversaciones []msgConversacion `json:"conversaciones"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Conversaciones) != 2 {
		t.Fatalf("conversaciones = %d; deben ser 2", len(r.Conversaciones))
	}
	// La más reciente va primero.
	if r.Conversaciones[0].JID != "584142222222@s.whatsapp.net" {
		t.Fatalf("orden inesperado: %+v", r.Conversaciones)
	}
	if r.Conversaciones[1].Ultimo != "Buenos días" || r.Conversaciones[1].Direccion != "out" {
		t.Fatalf("último mensaje inesperado: %+v", r.Conversaciones[1])
	}
}

func TestElHiloSoloTraeSuConversacion(t *testing.T) {
	s, db := baseAreas(t)
	insertarMensaje(t, db, "default.cxc", "584141111111@s.whatsapp.net", "in", "Hola", "2026-10-28 09:00:00")
	insertarMensaje(t, db, "default.cxc", "584142222222@s.whatsapp.net", "in", "No debe salir", "2026-10-28 09:01:00")

	req := reqPrueba(s, sessPrueba("master"), "/mensajeria/chat/584141111111@s.whatsapp.net/mensajes")
	req.SetPathValue("jid", "584141111111@s.whatsapp.net")
	req.URL.RawQuery = "device=default.cxc"
	res := httptest.NewRecorder()
	s.mensajeriaMensajesJSON(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("hilo: %d %s", res.Code, res.Body.String())
	}
	var r struct {
		Mensajes []map[string]interface{} `json:"mensajes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Mensajes) != 1 || r.Mensajes[0]["body"] != "Hola" {
		t.Fatalf("el hilo trajo mensajes de otra conversación: %+v", r.Mensajes)
	}
}

func TestEnviarTextoEncolaYNormaliza(t *testing.T) {
	s, db := baseAreas(t)
	form := url.Values{
		"device_id": {"default.cxc"},
		"jid":       {"0414-1234567@s.whatsapp.net"},
		"texto":     {"Hola, ¿cómo está?"},
	}
	req := httptest.NewRequest(http.MethodPost, "/mensajeria/enviar", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaEnviar(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("envío: %d %s", res.Code, res.Body.String())
	}

	var prioridad, kind, destino, cuerpo string
	if err := db.QueryRow(`SELECT priority, kind, to_msisdn, body FROM msg_outbox ORDER BY id LIMIT 1`).
		Scan(&prioridad, &kind, &destino, &cuerpo); err != nil {
		t.Fatal(err)
	}
	if prioridad != "chat" || kind != "chat" {
		t.Fatalf("prioridad/tipo inesperados: %q %q", prioridad, kind)
	}
	if destino != "584141234567" {
		t.Fatalf("el destino no se normalizó: %q", destino)
	}
	if cuerpo != "Hola, ¿cómo está?" {
		t.Fatalf("cuerpo inesperado: %q", cuerpo)
	}

	// La fila del historial se escribe al encolar para que el usuario la vea.
	var estado string
	if err := db.QueryRow(`SELECT status FROM msg_message WHERE direction='out'`).Scan(&estado); err != nil {
		t.Fatal(err)
	}
	if estado != "en_cola" {
		t.Fatalf("el mensaje debía quedar en cola, no %q", estado)
	}
}

func TestUnUsuarioSinDispositivoNoVeLaConversacion(t *testing.T) {
	s, db := baseAreas(t)
	insertarMensaje(t, db, "default.cxc", "584141111111@s.whatsapp.net", "in", "Privado", "2026-10-28 09:00:00")

	sess := sessPrueba("employee")
	sess.UserID = "7"
	req := reqPrueba(s, sess, "/mensajeria/chat/584141111111@s.whatsapp.net/mensajes")
	req.SetPathValue("jid", "584141111111@s.whatsapp.net")
	req.URL.RawQuery = "device=default.cxc"
	res := httptest.NewRecorder()
	s.mensajeriaMensajesJSON(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("un empleado sin dispositivo obtuvo %d; debía ser 403", res.Code)
	}
}

func TestEnviarSinPermisoDeEnvioSeRechaza(t *testing.T) {
	s, db := baseAreas(t)
	sess := sessPrueba("employee")
	sess.UserID = "7"
	req := httptest.NewRequest(http.MethodPost, "/mensajeria/enviar",
		strings.NewReader("device_id=default.cxc&jid=584141234567@s.whatsapp.net&texto=Hola"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: db, sess: sess})
	res := httptest.NewRecorder()
	s.mensajeriaEnviar(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("un empleado pudo enviar: %d %s", res.Code, res.Body.String())
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_outbox`).Scan(&n)
	if n != 0 {
		t.Fatal("se encoló un mensaje sin permiso de envío")
	}
}

func TestElChatNoEsperaLaPausaDeLosAvisos(t *testing.T) {
	s, db, falso := baseCola(t)
	// El dispositivo no puede mandar avisos hasta dentro de una hora.
	if _, err := db.Exec(`UPDATE msg_device SET next_send_at = ? WHERE device_id='default.cxc'`,
		"2026-10-28 11:00:00"); err != nil {
		t.Fatal(err)
	}
	// Un chat y un aviso, listos los dos.
	ahora := fecha(t, "2026-10-28 10:00")
	if _, _, err := s.msgEncolar(db, msgOutbox{
		Tenant: "default", DeviceID: "default.cxc", ToMSISDN: "584141234567",
		Body: "Hola", Kind: "chat", Priority: "chat", RefType: "chat", RefID: "x",
		IdemKey: "chat-x", SendAfter: "2026-01-01 00:00:00",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.msgEncolar(db, msgOutbox{
		Tenant: "default", DeviceID: "default.cxc", ToMSISDN: "584141234567",
		Body: "Aviso", Kind: "aviso", Priority: "transaccional", RefType: "prueba", RefID: "a",
		SendAfter: "2026-01-01 00:00:00",
	}); err != nil {
		t.Fatal(err)
	}

	s.msgDespacharUnaVez(t.Context(), ahora)

	if falso.enviados() != 1 {
		t.Fatalf("envíos = %d; solo el chat debía salir", falso.enviados())
	}
	var estadoChat, estadoAviso string
	_ = db.QueryRow(`SELECT status FROM msg_outbox WHERE kind='chat'`).Scan(&estadoChat)
	_ = db.QueryRow(`SELECT status FROM msg_outbox WHERE kind='aviso'`).Scan(&estadoAviso)
	if estadoChat != "sent" && estadoChat != msgAceptado {
		t.Fatalf("el chat no salió: %q", estadoChat)
	}
	if estadoAviso != msgEnCola {
		t.Fatalf("el aviso debía esperar la pausa: %q", estadoAviso)
	}
}
