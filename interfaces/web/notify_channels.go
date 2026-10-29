package web

// notify_channels.go — La pantalla donde se configuran las salidas.
//
// VIVE BAJO /settings A PROPÓSITO. Un canal de correo guarda la clave SMTP de la
// empresa; en este ERP las rutas /settings exigen master, y esta no es la
// excepción: quien configura por dónde sale la información del negocio es el
// dueño del sistema, no cualquier usuario con sesión.
//
// LA CLAVE NUNCA VUELVE AL NAVEGADOR. La lista que se pinta trae id, tipo,
// nombre, si está activo y si los agentes pueden usarlo. Al editar, el campo de
// la clave se deja vacío y significa «no la cambies»: si se devolviera el valor
// guardado, la credencial estaría en el HTML de cualquiera que abra la pantalla o
// mire el código fuente de la página.

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerNotifyChannelRoutes() {
	s.mux.HandleFunc("GET /settings/notify", s.notifyChannelsForm)
	s.mux.HandleFunc("POST /settings/notify/save", s.notifyChannelSave)
	s.mux.HandleFunc("POST /settings/notify/delete/{id}", s.notifyChannelDelete)
	s.mux.HandleFunc("POST /settings/notify/test", s.notifyChannelTest)
}

func (s *Server) notifyChannelsForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "notify_channels.html", map[string]interface{}{
		"Title":    "Canales de aviso",
		"Canales":  s.notifyCanales(s.dbFor(r), false),
		"Mensajes": s.finMensajesUltimos(s.dbFor(r), "", 10),
		"Nota": "Un canal sin configurar no envía nada: los avisos quedan en Mensajes enviados con el motivo del fallo. " +
			"«Permitir agentes» decide si además de una persona, un agente del sistema puede usar ese canal; " +
			"mientras esté apagado, ninguna herramienta de IA podrá mandar nada por él. " +
			"El enfriamiento (días) impide repetir el mismo aviso al mismo destinatario por la misma alerta.",
	})
}

// notifyChannelSave crea o actualiza un canal.
func (s *Server) notifyChannelSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(32 << 20)
	db := s.dbFor(r)

	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	kind := strings.ToLower(strings.TrimSpace(r.FormValue("kind")))
	nombre := strings.TrimSpace(r.FormValue("name"))
	if kind != "email" && kind != "sms" && kind != "webhook" && kind != "wapp" {
		jsonError(w, "Elija el tipo de canal: correo, SMS, WhatsApp o webhook.", http.StatusBadRequest)
		return
	}
	if nombre == "" {
		jsonError(w, "Póngale un nombre al canal («Correo de cobranza», «SMS de la pasarela»): es como lo verá el usuario al elegir por dónde avisar.", http.StatusBadRequest)
		return
	}

	activo := 0
	if r.FormValue("is_active") == "1" {
		activo = 1
	}
	aiAllowed := 0
	if r.FormValue("ai_allowed") == "1" {
		aiAllowed = 1
	}
	cooldown, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("cooldown_days")))
	if cooldown < 0 {
		cooldown = 0
	}
	port, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	if port <= 0 {
		port = 587
	}
	seguridad := strings.ToLower(strings.TrimSpace(r.FormValue("security")))
	if seguridad != "tls" && seguridad != "starttls" && seguridad != "none" {
		seguridad = "starttls"
	}
	metodo := strings.ToUpper(strings.TrimSpace(r.FormValue("http_method")))
	if metodo == "" {
		metodo = http.MethodPost
	}

	campos := map[string]interface{}{
		"kind": kind, "name": nombre, "is_active": activo, "ai_allowed": aiAllowed,
		"cooldown_days": cooldown,
		"host":          strings.TrimSpace(r.FormValue("host")), "port": port,
		"from_address": strings.TrimSpace(r.FormValue("from_address")),
		"from_name":    strings.TrimSpace(r.FormValue("from_name")),
		"security":     seguridad,
		"url":          strings.TrimSpace(r.FormValue("url")),
		"http_method":  metodo,
		"auth_header":  strings.TrimSpace(r.FormValue("auth_header")),
		"notes":        strings.TrimSpace(r.FormValue("notes")),
	}

	// Lo que cada tipo necesita para poder funcionar de verdad. Se exige AL
	// GUARDAR, no al enviar: un canal a medias se descubre a la hora de mandar el
	// aviso urgente, que es el peor momento para descubrirlo.
	switch kind {
	case "email":
		if campos["host"] == "" || campos["from_address"] == "" {
			jsonError(w, "Un canal de correo necesita servidor (host) y dirección del remitente.", http.StatusBadRequest)
			return
		}
	case "sms", "wapp", "webhook":
		if campos["url"] == "" {
			jsonError(w, "Un canal de "+kind+" necesita la URL a la que se entrega el mensaje.", http.StatusBadRequest)
			return
		}
	}

	if id <= 0 {
		usuario := strings.TrimSpace(r.FormValue("username"))
		clave := r.FormValue("password")
		token := r.FormValue("auth_token")
		firma := r.FormValue("sign_secret")
		res, err := db.Exec(`INSERT INTO notify_channel (kind, name, is_active, ai_allowed, cooldown_days,
			host, port, username, password, from_address, from_name, security, url, http_method,
			auth_header, auth_token, sign_secret, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			kind, nombre, activo, aiAllowed, cooldown,
			campos["host"], port, usuario, clave, campos["from_address"], campos["from_name"], seguridad,
			campos["url"], metodo, campos["auth_header"], token, firma, campos["notes"], now(), now())
		if err != nil {
			if aiEsClaveDuplicada(err) {
				jsonError(w, "Ya existe un canal con el nombre «"+nombre+"». Use otro nombre.", http.StatusBadRequest)
				return
			}
			jsonError(w, "No se pudo guardar el canal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		id, _ = res.LastInsertId()
	} else {
		// Los secretos solo se pisan si el formulario trae un valor nuevo. Un campo
		// vacío significa «déjala como está», y esa es la única forma de que la clave
		// no tenga que volver al navegador para poder editar el resto.
		sets := []string{"kind=?", "name=?", "is_active=?", "ai_allowed=?", "cooldown_days=?",
			"host=?", "port=?", "from_address=?", "from_name=?", "security=?", "url=?", "http_method=?",
			"auth_header=?", "notes=?", "updated_at=?"}
		args := []interface{}{kind, nombre, activo, aiAllowed, cooldown,
			campos["host"], port, campos["from_address"], campos["from_name"], seguridad,
			campos["url"], metodo, campos["auth_header"], campos["notes"], now()}
		for _, campo := range []string{"username", "password", "auth_token", "sign_secret"} {
			if v, ok := r.Form[campo]; ok && strings.TrimSpace(v[0]) != "" {
				sets = append(sets, campo+"=?")
				args = append(args, v[0])
			}
		}
		args = append(args, id)
		if _, err := db.Exec(`UPDATE notify_channel SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			if aiEsClaveDuplicada(err) {
				jsonError(w, "Ya existe otro canal con el nombre «"+nombre+"».", http.StatusBadRequest)
				return
			}
			jsonError(w, "No se pudo guardar el canal: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// La auditoría NO guarda el token ni la clave: deja constancia de qué canal
	// cambió, quién y cuándo, no de sus credenciales.
	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), clientIP(r), "update", "Configuración",
		"notify_channel", strconv.FormatInt(id, 10), nil, map[string]interface{}{
			"name": nombre, "kind": kind, "is_active": activo, "ai_allowed": aiAllowed,
			"cooldown_days": cooldown,
		})
	writeJSON(w, map[string]interface{}{"status": 1, "id": id, "msg": "Canal guardado."})
}

func (s *Server) notifyChannelDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		jsonError(w, "Canal inválido.", http.StatusBadRequest)
		return
	}
	// Los mensajes ya enviados NO se borran: son el historial de lo que salió del
	// sistema. El canal desaparece y el mensaje conserva su nombre.
	if _, err := s.dbFor(r).Exec(`DELETE FROM notify_channel WHERE id = ?`, id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), clientIP(r), "delete", "Configuración",
		"notify_channel", strconv.FormatInt(id, 10), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Canal eliminado. Los mensajes ya enviados se conservan en el historial."})
}

// notifyChannelTest manda un aviso de prueba por un canal.
//
// POR QUÉ HACE FALTA UNA PRUEBA. Un canal mal configurado se descubre al intentar
// avisar al cliente moroso, y allí ya hay una persona esperando. La prueba
// resuelve la duda en el momento de configurar, que es cuando importa. Se puede
// apuntar a cualquier destino porque lo pide un administrador a propósito; el
// resto de la ruta de avisos no admite esa libertad.
func (s *Server) notifyChannelTest(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(32 << 20)
	db := s.dbFor(r)
	canalID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("channel_id")), 10, 64)
	para := strings.TrimSpace(r.FormValue("to"))
	if canalID <= 0 || para == "" {
		jsonError(w, "Indique el canal y a dónde mandar la prueba.", http.StatusBadRequest)
		return
	}
	canal, err := s.notifyCanalPorID(db, canalID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	empresa := s.finEmpresa(db)
	if empresa == "" {
		empresa = "su empresa"
	}
	asunto := "Prueba de canal de avisos — " + canal.Name
	cuerpo := "Este es un mensaje de prueba enviado desde ERPGo a nombre de " + empresa + ".\n\n" +
		"Si lo está leyendo, el canal «" + canal.Name + "» quedó bien configurado y ya se pueden enviar " +
		"avisos de cobranza y comprobantes de retención por esta vía."

	clave := aiHashKey("prueba|" + strconv.FormatInt(canal.ID, 10) + "|" + strings.ToLower(para) + "|" + now())
	res, err := db.Exec(`INSERT INTO finance_message (channel_id, channel, channel_kind, recipient, subject, body,
		ref_type, ref_id, status, idem_key, created_by, created_at)
		VALUES (?,?,?,?,?,?, 'prueba', '', 'enviando', ?, ?, ?)`,
		canal.ID, canal.Name, canal.Kind, para, asunto, cuerpo, clave, sessUserID(s.sessionFromRequest(r)), now())
	if err != nil {
		jsonError(w, "No se pudo registrar la prueba: "+err.Error(), http.StatusInternalServerError)
		return
	}
	msgID, _ := res.LastInsertId()

	var prov string
	switch canal.Kind {
	case "email":
		prov, err = s.envioEmail(canal, para, asunto, cuerpo)
	case "sms":
		if strings.TrimSpace(canal.URL) == "" {
			err = errSinURL
		} else {
			prov, err = s.envioHTTP(canal, "sms", map[string]interface{}{"to": para, "message": asunto + "\n" + cuerpo, "subject": asunto})
		}
	case "webhook":
		if strings.TrimSpace(canal.URL) == "" {
			err = errSinURL
		} else {
			prov, err = s.envioHTTP(canal, "finance.prueba", map[string]interface{}{
				"test": true, "channel": canal.Name, "subject": asunto, "body": cuerpo, "recipient": para,
			})
		}
	default:
		err = errSinURL
	}

	if err != nil {
		_, _ = db.Exec(`UPDATE finance_message SET status = 'error', error = ? WHERE id = ?`, err.Error(), msgID)
		jsonError(w, "La prueba falló: "+err.Error(), http.StatusBadGateway)
		return
	}
	_, _ = db.Exec(`UPDATE finance_message SET status = 'enviado', sent_at = ?, provider_ref = ? WHERE id = ?`, now(), prov, msgID)
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Prueba enviada a " + para + " por «" + canal.Name + "». Queda en Mensajes enviados.",
	})
}

// errSinURL se usa cuando el canal existe pero le falta la dirección de entrega.
var errSinURL = errorString("el canal no tiene URL de entrega configurada")

// errorString es un error de una línea, para mensajes que no envuelven otro.
type errorString string

func (e errorString) Error() string { return string(e) }
