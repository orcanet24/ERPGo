package web

// mensajeria_webhook.go — La puerta de entrada de WhatsApp.
//
// ESTA RUTA NO TIENE SESIÓN y no puede tenerla: quien llama es el daemon, no un
// navegador. Por eso su autenticación es un HMAC-SHA256 del cuerpo con el
// secreto DEL DISPOSITIVO, en `X-Hub-Signature-256`. Sin firma válida no se
// escribe nada.
//
// LA EMPRESA SALE DEL DISPOSITIVO, NUNCA DEL CUERPO. El payload trae el JID de
// WhatsApp; si se creyera ese dato, cualquiera con una firma de la empresa A
// podría escribir en el historial de la B. El `device_id` de la URL se resuelve
// contra `msg_device` (base de control) y de ahí sale el tenant.
//
// IDEMPOTENCIA: GOWA reintenta hasta cinco veces. La clave única
// `(device_id, message_id)` hace que un reintento no duplique el historial. Si
// el payload no trae id, se usa el hash del cuerpo: un reintento idéntico
// tampoco se duplica.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// msgWebhookEvento es la forma observada del evento de GOWA.
type msgWebhookEvento struct {
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id"`
	Event     string `json:"event"`
	Payload   struct {
		ID        string `json:"id"`
		From      string `json:"from"`
		ChatID    string `json:"chat_id"`
		FromName  string `json:"from_name"`
		IsFromMe  bool   `json:"is_from_me"`
		Body      string `json:"body"`
		Ack       int    `json:"ack"`
		Timestamp int64  `json:"timestamp"`
		MediaType string `json:"media_type"`
		Type      string `json:"type"`
		Mimetype  string `json:"mimetype"`
		Media     string `json:"media"`
		Filename  string `json:"filename"`
	} `json:"payload"`
}

func (s *Server) mensajeriaWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("device")
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		http.Error(w, "sin base de control", http.StatusServiceUnavailable)
		return
	}
	dispositivo, err := s.msgDeviceDe(ctl, id)
	// Un dispositivo desconocido o retirado responde 404 y no revela nada más.
	if err != nil || dispositivo.State == "retirado" {
		http.NotFound(w, r)
		return
	}

	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "cuerpo ilegible", http.StatusBadRequest)
		return
	}
	secreto, err := s.msgDeviceSecreto(dispositivo)
	if err != nil {
		http.Error(w, "dispositivo sin secreto", http.StatusInternalServerError)
		return
	}
	if !firmaWebhookValida(cuerpo, secreto, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "firma inválida", http.StatusUnauthorized)
		return
	}

	var ev msgWebhookEvento
	if err := json.Unmarshal(cuerpo, &ev); err != nil {
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}

	db, err := s.tenantDB(dispositivo.Tenant)
	if err != nil || db == nil {
		// NUNCA se cae a la empresa activa: escribir el mensaje de la empresa A
		// en la base de la B sería peor que perderlo. Si su base no está
		// disponible, se responde 503 y el daemon lo reintenta.
		http.Error(w, "empresa no disponible", http.StatusServiceUnavailable)
		return
	}

	switch strings.ToLower(strings.TrimSpace(ev.Event)) {
	case "message":
		tipo := msgTipoEntrante(primeroNoVacio(ev.Payload.MediaType, ev.Payload.Type, ev.Payload.Mimetype))
		if err := s.msgGuardarEntrante(db, dispositivo, ev, cuerpo, tipo); err != nil {
			http.Error(w, "no se pudo guardar el mensaje", http.StatusInternalServerError)
			return
		}
		if !ev.Payload.IsFromMe {
			numero := msisdnDeJID(ev.Payload.From)
			// El primer mensaje entrante es la prueba de que el número tiene
			// WhatsApp: se guarda verificado sin llamar a /user/check.
			_ = s.msgVerificarCanal(db, numero, dispositivo.Provider, numero, "mensaje_entrante")
			// Y si la persona pidió la baja, se registra en el acto. Esperar a
			// que un humano lo lea es cómo se acaba insistiendo a quien dijo
			// «BAJA».
			if msgOptOutDeTexto(ev.Payload.Body) {
				_ = s.msgRegistrarOptOut(db, numero, "pidió la baja por WhatsApp", "whatsapp", "system")
			}
			// El archivo se descarga después de guardar el mensaje: si la
			// descarga falla, el mensaje y su tipo se conservan.
			if tipo != "" {
				s.msgBajarMediaEntrante(r.Context(), ctl, db, dispositivo, ev, cuerpo, tipo)
			}
		}
	case "message.ack":
		s.msgAplicarAcuse(db, dispositivo.DeviceID, ev.Payload.ID, ev.Payload.Ack)
	default:
		// Un evento que no conocemos se ignora con 200: responder error haría
		// que GOWA reintentara cinco veces algo que nunca vamos a procesar.
		writeJSON(w, map[string]interface{}{"ok": true, "ignored": true})
		return
	}

	_, _ = ctl.Exec(`UPDATE msg_device SET last_seen_at=? WHERE device_id=?`, now(), dispositivo.DeviceID)
	writeJSON(w, map[string]interface{}{"ok": true})
}

// msgGuardarEntrante escribe el mensaje en la base de la empresa. Un duplicado
// no es un error: es el reintento de GOWA y se responde 200.
func (s *Server) msgGuardarEntrante(db *sql.DB, dispositivo msgDevice, ev msgWebhookEvento, cuerpo []byte, tipo string) error {
	chat := primeroNoVacio(ev.Payload.ChatID, ev.Payload.From)
	from, to := msisdnDeJID(ev.Payload.From), ""
	direccion, estado := "in", "recibido"
	if ev.Payload.IsFromMe {
		direccion, estado = "out", "aceptado"
		from, to = msisdnDeJID(chat), msisdnDeJID(chat)
	}

	msgID := msgIDDeEvento(ev, cuerpo)
	cuando := now()
	if ev.Payload.Timestamp > 0 {
		cuando = time.Unix(ev.Payload.Timestamp, 0).Format("2006-01-02 15:04:05")
	}
	crudo := string(cuerpo)
	if len(crudo) > 8192 {
		crudo = crudo[:8192]
	}

	_, err := db.Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		from_msisdn, to_msisdn, body, media_type, status, is_from_me, sent_at, received_at, raw_json, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		dispositivo.DeviceID, dispositivo.Provider, chat, direccion, msgID,
		from, to, ev.Payload.Body, tipo, estado, boolAInt(ev.Payload.IsFromMe),
		cuando, cuando, crudo, now())
	if err != nil && aiEsClaveDuplicada(err) {
		return nil
	}
	return err
}

// msgIDDeEvento usa el id del proveedor; si no viene, el hash del cuerpo, que
// hace idempotente un reintento idéntico.
func msgIDDeEvento(ev msgWebhookEvento, cuerpo []byte) string {
	if id := strings.TrimSpace(ev.Payload.ID); id != "" {
		return id
	}
	sum := sha256.Sum256(cuerpo)
	return "auto-" + hex.EncodeToString(sum[:16])
}

// msgBajarMediaEntrante descarga el archivo del mensaje y lo deja enlazado. Si
// falla, no se toca la fila: el mensaje ya está guardado con su tipo.
func (s *Server) msgBajarMediaEntrante(ctx context.Context, ctl, db *sql.DB, dispositivo msgDevice, ev msgWebhookEvento, cuerpo []byte, tipo string) {
	cliente, err := s.msgClienteSinRequest(ctl)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	msgID := msgIDDeEvento(ev, cuerpo)
	datos, _, err := cliente.DescargarMedia(ctx, dispositivo.DeviceID, msgID)
	if err != nil {
		return
	}
	ext := ".bin"
	switch tipo {
	case "image":
		ext = ".jpg"
	case "audio":
		ext = ".ogg"
	case "video":
		ext = ".mp4"
	case "document":
		ext = ".pdf"
	}
	guardado, err := s.msgGuardarMedia(dispositivo.Tenant, "entrante"+ext, datos)
	if err != nil {
		return
	}
	_, _ = db.Exec(`UPDATE msg_message SET media_path=? WHERE device_id=? AND message_id=?`,
		guardado, dispositivo.DeviceID, msgID)
}

// firmaWebhookValida compara la firma en tiempo constante.
func firmaWebhookValida(cuerpo []byte, secreto, cabecera string) bool {
	cabecera = strings.TrimSpace(cabecera)
	if cabecera == "" || secreto == "" {
		return false
	}
	// Se acepta con y sin el prefijo «sha256=».
	if i := strings.IndexByte(cabecera, '='); i >= 0 {
		cabecera = cabecera[i+1:]
	}
	recibida, err := hex.DecodeString(strings.TrimSpace(cabecera))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secreto))
	mac.Write(cuerpo)
	return hmac.Equal(recibida, mac.Sum(nil))
}

// msgAplicarAcuse traduce el acuse de WhatsApp a un estado del historial.
//
// «Aceptado» es lo que dijo el daemon al enviar; «entregado» y «leído» solo se
// escriben cuando llega este evento. Sin esta distinción el ERP diría que un
// mensaje se entregó cuando el teléfono estaba apagado.
func (s *Server) msgAplicarAcuse(db *sql.DB, deviceID, messageID string, ack int) {
	if strings.TrimSpace(messageID) == "" {
		return
	}
	estado := map[int]string{
		0: "error",
		1: "aceptado",
		2: "entregado",
		3: "leido",
		4: "reproducido",
	}[ack]
	if estado == "" {
		return
	}
	_, _ = db.Exec(`UPDATE msg_message SET status=? WHERE device_id=? AND message_id=?`,
		estado, deviceID, messageID)
}

// msisdnDeJID quita el sufijo de WhatsApp (`@s.whatsapp.net`, `@c.us`) y el
// dispositivo del JID (`58414…:12`). Lo que queda es el número.
func msisdnDeJID(jid string) string {
	jid = strings.TrimSpace(jid)
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		jid = jid[:i]
	}
	if i := strings.IndexByte(jid, ':'); i >= 0 {
		jid = jid[:i]
	}
	var b strings.Builder
	for _, r := range jid {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func primeroNoVacio(valores ...string) string {
	for _, v := range valores {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
