package web

// mensajeria_chat.go — La bandeja y el hilo de conversaciones.
//
// LEE DEL HISTORIAL, NO DEL DAEMON. Cada pantalla consulta `msg_message`, que es
// lo que el webhook ya guardó. Si se consultara GOWA en cada carga, una bandeja
// abierta sería una consulta cada cinco segundos al número, y el daemon se
// convertiría en una dependencia de la UI. El daemon solo se usa para ENVIAR.
//
// EL ENVÍO PASA POR LA COLA, COMO TODO. Un chat no puede saltarse el ritmo ni
// el cupo, pero tampoco puede esperar tres minutos: los mensajes de chat tienen
// prioridad propia y no esperan la pausa entre avisos. La fila se escribe en el
// historial al encolar (estado «en_cola») para que el usuario vea su mensaje en
// el acto, y el despachador la actualiza cuando sale.
//
// PERMISOS. `mensajeria_chat` para leer y `mensajeria_send` para escribir. Un
// usuario que no tiene el dispositivo asignado en `msg_user_access` no ve esa
// conversación: el master y el admin ven todas las de su empresa.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"erpgo/internal/mensajeria"
)

// msgConversacion es una fila de la bandeja.
type msgConversacion struct {
	DeviceID  string `json:"device_id"`
	JID       string `json:"jid"`
	Ultimo    string `json:"ultimo"`
	Cuando    string `json:"cuando"`
	Estado    string `json:"estado"`
	Direccion string `json:"direccion"`
	Media     string `json:"media"`
}

func (s *Server) registerMensajeriaChatRoutes() {
	s.mux.HandleFunc("GET /mensajeria/conversaciones", s.mensajeriaConversacionesJSON)
	s.mux.HandleFunc("GET /mensajeria/chat/{jid}/mensajes", s.mensajeriaMensajesJSON)
	s.mux.HandleFunc("POST /mensajeria/enviar", s.mensajeriaEnviar)
}

// msgDispositivosVisibles devuelve los dispositivos que este usuario puede leer.
// Master y admin ven todos los de su empresa; el resto, solo los asignados.
func (s *Server) msgDispositivosVisibles(r *http.Request) []string {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		return nil
	}
	if sess.UserLevel == "admin" || sess.UserLevel == "master" {
		ctl, err := s.controlStore()
		if err != nil || ctl == nil {
			return nil
		}
		rows, err := ctl.Query(`SELECT device_id FROM msg_device
			WHERE tenant = ? AND state <> 'retirado' ORDER BY device_id`, s.tenantOf(r))
		if err != nil {
			return nil
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		return ids
	}
	rows, err := s.dbFor(r).Query(`SELECT device_id FROM msg_user_access
		WHERE user_id = ? AND can_read = 1 ORDER BY device_id`, sess.UserID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func msgContiene(lista []string, valor string) bool {
	for _, v := range lista {
		if v == valor {
			return true
		}
	}
	return false
}

// msgInbox lista las conversaciones de los dispositivos visibles.
func (s *Server) msgInbox(db *sql.DB, dispositivos []string) []msgConversacion {
	out := []msgConversacion{}
	if len(dispositivos) == 0 {
		return out
	}
	marcas := strings.TrimSuffix(strings.Repeat("?,", len(dispositivos)), ",")
	args := make([]interface{}, len(dispositivos))
	for i, d := range dispositivos {
		args[i] = d
	}

	rows, err := db.Query(`SELECT device_id, chat_jid, MAX(created_at) AS cuando
		FROM msg_message WHERE device_id IN (`+marcas+`)
		GROUP BY device_id, chat_jid ORDER BY cuando DESC LIMIT 100`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c msgConversacion
		if err := rows.Scan(&c.DeviceID, &c.JID, &c.Cuando); err != nil {
			continue
		}
		_ = db.QueryRow(`SELECT IFNULL(body,''), direction, IFNULL(status,''), IFNULL(media_type,'')
			FROM msg_message WHERE device_id=? AND chat_jid=? ORDER BY id DESC LIMIT 1`,
			c.DeviceID, c.JID).Scan(&c.Ultimo, &c.Direccion, &c.Estado, &c.Media)
		out = append(out, c)
	}
	return out
}

func (s *Server) mensajeriaConversacionesJSON(w http.ResponseWriter, r *http.Request) {
	if !s.mensajeriaActiva(r) {
		notFoundOrJSON(w, r, "mensajería no disponible")
		return
	}
	writeJSON(w, map[string]interface{}{
		"conversaciones": s.msgInbox(s.dbFor(r), s.msgDispositivosVisibles(r)),
	})
}

func (s *Server) mensajeriaMensajesJSON(w http.ResponseWriter, r *http.Request) {
	if !s.mensajeriaActiva(r) {
		notFoundOrJSON(w, r, "mensajería no disponible")
		return
	}
	jid := r.PathValue("jid")
	device := strings.TrimSpace(r.URL.Query().Get("device"))
	if !msgContiene(s.msgDispositivosVisibles(r), device) {
		jsonError(w, "No tiene acceso a ese dispositivo.", http.StatusForbidden)
		return
	}

	rows, err := s.dbFor(r).Query(`SELECT id, direction, IFNULL(body,''), IFNULL(status,''),
		IFNULL(media_type,''), IFNULL(media_path,''), IFNULL(created_at,'') FROM msg_message
		WHERE device_id = ? AND chat_jid = ? ORDER BY id ASC LIMIT 300`, device, jid)
	if err != nil {
		jsonError(w, "No se pudo leer el historial.", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	mensajes := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var direccion, cuerpo, estado, media, mediaPath, cuando string
		if err := rows.Scan(&id, &direccion, &cuerpo, &estado, &media, &mediaPath, &cuando); err != nil {
			continue
		}
		mensajes = append(mensajes, map[string]interface{}{
			"id": id, "direction": direccion, "body": cuerpo, "status": estado,
			"media_type": media, "has_media": mediaPath != "", "created_at": cuando,
		})
	}
	writeJSON(w, map[string]interface{}{"mensajes": mensajes})
}

// mensajeriaEnviar encola un texto escrito por una persona.
func (s *Server) mensajeriaEnviar(w http.ResponseWriter, r *http.Request) {
	if !s.mensajeriaActiva(r) {
		notFoundOrJSON(w, r, "mensajería no disponible")
		return
	}
	if !s.exigirPermiso(w, r, "create", "mensajeria_send") {
		return
	}
	_ = r.ParseForm()
	device := strings.TrimSpace(r.FormValue("device_id"))
	jid := strings.TrimSpace(r.FormValue("jid"))
	texto := strings.TrimSpace(r.FormValue("texto"))
	if texto == "" {
		jsonError(w, "Escriba el mensaje antes de enviarlo.", http.StatusBadRequest)
		return
	}
	if len([]rune(texto)) > 4000 {
		jsonError(w, "El mensaje es demasiado largo; divídalo en dos.", http.StatusBadRequest)
		return
	}
	if !msgContiene(s.msgDispositivosVisibles(r), device) {
		jsonError(w, "No tiene acceso a ese dispositivo.", http.StatusForbidden)
		return
	}
	if strings.Contains(jid, "@g.us") {
		jsonError(w, "Los grupos no están disponibles en esta fase.", http.StatusBadRequest)
		return
	}
	msisdn, err := s.normalizarTelefonoDeLaEmpresa(r, msisdnDeJID(jid))
	if err != nil {
		jsonError(w, "El teléfono de la conversación no se pudo normalizar: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "No hay base de control disponible.", http.StatusServiceUnavailable)
		return
	}
	tenant := s.tenantOf(r)
	// Clave única por mensaje: dos «ok» seguidos son dos mensajes, no un
	// duplicado. La idempotencia del aviso no aplica al chat.
	clave := "chat|" + tenant + "|" + jid + "|" + randomHex(8)
	id, _, err := s.msgEncolar(ctl, msgOutbox{
		Tenant: tenant, Provider: "whatsapp_gowa", DeviceID: device,
		Priority: "chat", Kind: "chat", ToMSISDN: msisdn, Body: texto,
		RefType: "chat", RefID: jid, IdemKey: clave, CreatedBy: s.sessionUserID(r),
	})
	if err != nil {
		jsonError(w, "No se pudo encolar el mensaje: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Se escribe en el historial con el estado «en_cola» para que el usuario vea
	// su mensaje en el acto. El despachador lo actualizará al enviarlo.
	_, _ = s.dbFor(r).Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		from_msisdn, to_msisdn, body, status, is_from_me, sent_at, received_at, raw_json, created_at)
		VALUES (?, 'whatsapp_gowa', ?, 'out', ?, '', ?, ?, 'en_cola', 1, ?, '', '', ?)`,
		device, jid, clave, msisdn, texto, now(), now())

	s.audit(r, "create", "mensajeria", "msg_outbox", fmt.Sprintf("%d", id), nil,
		map[string]interface{}{"device": device, "jid": jid, "tipo": "texto"})

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Mensaje en cola.",
		"outbox_id": id, "clave": clave})
}

// msgEsChatIndica si la fila es un mensaje escrito en la bandeja.
func msgEsChat(kind string) bool { return strings.EqualFold(strings.TrimSpace(kind), "chat") }

// msgNormalizarJID se usa en las pruebas y en la bandeja para mostrar el número.
func msgNormalizarJID(jid string) string {
	if m, err := mensajeria.Normalizar(msisdnDeJID(jid), mensajeria.PaisPorDefecto); err == nil {
		return m
	}
	return msisdnDeJID(jid)
}
