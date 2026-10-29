package web

// outbound.go — La única puerta por la que el ERP habla hacia afuera.
//
// POR QUÉ ES UN ARCHIVO APARTE Y NO UN `mail()` SUELTO.
//
// Un aviso al cliente sobre su mora y un comprobante de retención al proveedor
// tienen las mismas exigencias, y ninguna es opcional:
//
//   - Sale UNA vez. El ticker de un agente, un reintento de la cola y un
//     «Ejecutar ahora» pueden pedir el mismo aviso tres veces; un cliente que
//     recibe tres correos idénticos deja de leer el cuarto. La clave de
//     idempotencia (`idem_key`) y el enfriamiento por canal son las dos redes.
//   - Sale a quien dice el REGISTRO. El destinatario se resuelve desde la ficha
//     del cliente o del proveedor; ni el formulario ni el modelo eligen a dónde
//     va. Eso es lo que hace imposible que un prompt inyectado use el ERP como
//     mensajero hacia una dirección que él elija.
//   - Queda ESCRITO. Canal, destinatario, asunto, cuerpo, quién lo mandó y si
//     salió o falló. Sin ese registro, «¿ya se le avisó a este cliente?» solo lo
//     puede contestar la persona que lo mandó, y esa persona se va de vacaciones.
//   - Un canal sin configurar NO se finge: el mensaje queda en `error` con el
//     motivo escrito. Nada peor que un aviso que el sistema cree enviado y que
//     nunca salió.
//
// LAS CREDENCIALES VIVEN EN LA BASE DE LA EMPRESA (`notify_channel`) y no viajan
// nunca al navegador: la pantalla de configuración solo recibe id, nombre y tipo.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// notifyChannel es la configuración de un canal de salida.
type notifyChannel struct {
	ID           int64
	Kind         string
	Name         string
	IsActive     int
	AIAllowed    int
	CooldownDays int

	// Correo
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	Security    string // tls | starttls | none

	// SMS / webhook
	URL        string
	Method     string
	AuthHeader string
	AuthToken  string
	SignSecret string

	// Mensajería (canal «gowa»): qué dispositivo de la empresa manda el aviso.
	MsgDeviceID string

	Notes string
}

// notifyCanalesSQL es una sola sentencia para que el lector y el probador no se
// separen de las columnas.
const notifyCanalesSQL = `SELECT id, IFNULL(kind,''), IFNULL(name,''), is_active, ai_allowed, cooldown_days,
	IFNULL(host,''), port, IFNULL(username,''), IFNULL(password,''), IFNULL(from_address,''), IFNULL(from_name,''),
	IFNULL(security,'starttls'), IFNULL(url,''), IFNULL(http_method,'POST'), IFNULL(auth_header,''),
	IFNULL(auth_token,''), IFNULL(sign_secret,''), IFNULL(msg_device_id,''), IFNULL(notes,'')
	FROM notify_channel`

// notifyCanalEscanear llena una fila del canal.
func notifyCanalEscanear(s interface {
	Scan(dest ...interface{}) error
}) (notifyChannel, error) {
	var c notifyChannel
	err := s.Scan(&c.ID, &c.Kind, &c.Name, &c.IsActive, &c.AIAllowed, &c.CooldownDays,
		&c.Host, &c.Port, &c.Username, &c.Password, &c.FromAddress, &c.FromName,
		&c.Security, &c.URL, &c.Method, &c.AuthHeader, &c.AuthToken, &c.SignSecret,
		&c.MsgDeviceID, &c.Notes)
	return c, err
}

func (s *Server) notifyCanalPorID(db *sql.DB, id int64) (notifyChannel, error) {
	c, err := notifyCanalEscanear(db.QueryRow(notifyCanalesSQL+` WHERE id = ?`, id))
	if err != nil {
		return c, fmt.Errorf("el canal %d no existe", id)
	}
	return c, nil
}

// notifyCanales lista los canales configurados.
func (s *Server) notifyCanales(db *sql.DB, soloActivos bool) []notifyChannel {
	out := []notifyChannel{}
	q := notifyCanalesSQL
	if soloActivos {
		q += ` WHERE is_active = 1`
	}
	q += ` ORDER BY kind, name`
	rows, err := db.Query(q)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		c, err := notifyCanalEscanear(rows)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// notifyCanalParaAgente elige el canal que un agente puede usar.
//
// La regla es deliberadamente estrecha: el canal tiene que estar activo, abierto
// a la IA por el administrador, del tipo pedido y —si se nombró uno— llamarse
// así. Un agente nunca inventa un destino: elige entre los canales que un humano
// ya autorizó.
func (s *Server) notifyCanalParaAgente(db *sql.DB, tipo, nombre string) (notifyChannel, error) {
	tipo = strings.TrimSpace(strings.ToLower(tipo))
	nombre = strings.TrimSpace(nombre)
	for _, c := range s.notifyCanales(db, true) {
		if c.AIAllowed != 1 {
			continue
		}
		if tipo != "" && c.Kind != tipo {
			continue
		}
		if nombre != "" && !strings.EqualFold(c.Name, nombre) {
			continue
		}
		return c, nil
	}
	abiertos := []string{}
	for _, c := range s.notifyCanales(db, true) {
		if c.AIAllowed == 1 {
			abiertos = append(abiertos, c.Name+" ("+c.Kind+")")
		}
	}
	if len(abiertos) == 0 {
		return notifyChannel{}, fmt.Errorf("no hay ningún canal de aviso habilitado para agentes. " +
			"Configuración > Canales de aviso: el administrador debe crear el canal y marcar «permitir agentes»; " +
			"sin eso el sistema no deja que un agente mande nada hacia afuera")
	}
	return notifyChannel{}, fmt.Errorf("no hay canal de tipo %q habilitado para agentes. Disponibles: %s",
		nombreODefecto(tipo, "cualquiera"), strings.Join(abiertos, ", "))
}

// notifyOpciones es lo que ve el navegador: id, nombre y tipo. Nunca las
// credenciales.
func (s *Server) notifyOpciones(db *sql.DB) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, c := range s.notifyCanales(db, true) {
		out = append(out, map[string]interface{}{
			"id": c.ID, "name": c.Name, "kind": c.Kind, "ai_allowed": c.AIAllowed,
		})
	}
	return out
}

// finDestinatario resuelve a dónde va el aviso, según el canal y la contraparte.
func (s *Server) finDestinatario(db *sql.DB, canal notifyChannel, alerta map[string]interface{}) (string, string) {
	partyType := fmt.Sprintf("%v", alerta["party_type"])
	var id int64
	switch v := alerta["party_id"].(type) {
	case int64:
		id = v
	case float64:
		id = int64(v)
	}
	if id <= 0 {
		return "", "La alerta no identifica la contraparte, así que no hay a quién avisarle."
	}

	var nombre, email, movil string
	if partyType == "supplier" {
		nombre, email, movil = s.finContactoProveedor(db, id)
	} else {
		nombre, email, movil = s.finContactoCliente(db, id)
	}
	_ = nombre

	switch canal.Kind {
	case "sms":
		if strings.TrimSpace(movil) == "" {
			return "", "La ficha no tiene teléfono. Cárguelo en " + etiquetaContraparte(partyType) + " y vuelva a intentarlo: el destinatario del aviso sale siempre del registro, nunca de la pantalla."
		}
		return movil, ""
	case "webhook":
		return "webhook://" + canal.Name, ""
	default:
		if canalUsaMovil(canal.Kind) {
			if strings.TrimSpace(movil) == "" {
				return "", "La ficha no tiene teléfono. Cárguelo en " + etiquetaContraparte(partyType) + " y vuelva a intentarlo: el destinatario del aviso sale siempre del registro, nunca de la pantalla."
			}
			return movil, ""
		}
		if strings.TrimSpace(email) == "" {
			return "", "La ficha no tiene correo. Cárguelo en " + etiquetaContraparte(partyType) + " y vuelva a intentarlo: el destinatario del aviso sale siempre del registro, nunca de la pantalla."
		}
		return email, ""
	}
}

func etiquetaContraparte(partyType string) string {
	if partyType == "supplier" {
		return "Proveedores"
	}
	return "Clientes"
}

// canalUsaMovil dice si el destinatario de este canal es un teléfono y no un
// correo.
//
// EXISTE PORQUE EL `default` DE finDestinatario ERA EL CORREO. Un canal `wapp`
// caía ahí y el ERP le entregaba a la pasarela de WhatsApp el email de la ficha
// como «to»: el aviso salía (o fallaba) contra una dirección que no es un
// teléfono. Los tipos nuevos de mensajería (gowa, telegram) tienen el mismo
// problema, así que la decisión se toma en un solo sitio y no en cada `switch`.
func canalUsaMovil(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "wapp", "gowa", "telegram":
		return true
	}
	return false
}

// finUltimoAviso dice cuándo se le mandó a esta referencia por este canal, para
// poder explicar el bloqueo por enfriamiento con una fecha y no con un «no se
// puede».
func (s *Server) finUltimoAviso(db *sql.DB, refType, refID string, canalID int64) (string, bool) {
	var cuando sql.NullString
	err := db.QueryRow(`SELECT IFNULL(CAST(sent_at AS CHAR),'') FROM finance_message
		WHERE ref_type = ? AND ref_id = ? AND channel_id = ? AND status = 'enviado'
		ORDER BY id DESC LIMIT 1`, refType, refID, canalID).Scan(&cuando)
	if err != nil || strings.TrimSpace(cuando.String) == "" {
		return "", false
	}
	return cuando.String, true
}

// financeEnviarAviso es el envío en sí: reclama la clave, comprueba el
// enfriamiento, registra el mensaje, despacha y cierra con el resultado.
//
// El orden importa. La fila del mensaje se escribe ANTES de intentar el envío
// porque es el reclamo: si dos procesos piden lo mismo a la vez, la clave única
// deja pasar a uno solo. Si el envío falla después, el mensaje queda en `error`
// con el motivo, que es información, no basura.
func (s *Server) financeEnviarAviso(r *http.Request, db *sql.DB, canal notifyChannel, alerta map[string]interface{},
	asunto, cuerpo, refType, refID string, forzar bool) (map[string]interface{}, error) {

	destino, motivo := s.finDestinatario(db, canal, alerta)
	if motivo != "" {
		return map[string]interface{}{
			"enviado": false, "msg": motivo, "destino": "",
		}, nil
	}

	// Enfriamiento: el mismo aviso, por el mismo canal, no se repite antes de
	// tiempo. Un agente que corre cada cinco minutos no debe convertirse en una
	// máquina de mandar recordatorios.
	cooldown := canal.CooldownDays
	if cooldown > 0 && !forzar {
		if cuando, hay := s.finUltimoAviso(db, refType, refID, canal.ID); hay {
			if finDentroDe(cuando, time.Duration(cooldown)*24*time.Hour) {
				return map[string]interface{}{
					"enviado": false, "cooldown": true, "destino": destino,
					"msg": fmt.Sprintf("Ya se le avisó por «%s» el %s. El canal tiene un enfriamiento de %d día(s) para no repetir el mismo aviso. "+
						"Si de todas formas hay que insistir ahora, vuelva a enviarlo dando «forzar».", canal.Name, cuando, cooldown),
				}, nil
			}
		}
	}

	usuario := ""
	if sess := s.sessionFromRequest(r); sess != nil {
		usuario = sessUserID(sess)
	}

	// La clave de idempotencia lleva la referencia, el canal y el DÍA: el mismo
	// aviso dos veces el mismo día es el mismo aviso; al día siguiente, insistir es
	// una decisión nueva y legítima.
	clave := aiHashKey("aviso|" + refType + "|" + refID + "|" + strconv.FormatInt(canal.ID, 10) + "|" +
		strings.ToLower(strings.TrimSpace(destino)) + "|" + finFechaCorta(now()))

	res, err := db.Exec(`INSERT INTO finance_message (channel_id, channel, channel_kind, recipient, subject, body,
		ref_type, ref_id, status, idem_key, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?, 'enviando', ?, ?, ?)`,
		canal.ID, canal.Name, canal.Kind, destino, asunto, cuerpo, refType, refID, clave, usuario, now())
	if err != nil {
		if aiEsClaveDuplicada(err) {
			return map[string]interface{}{
				"enviado": false, "duplicado": true, "destino": destino,
				"msg": "Este mismo aviso ya se registró hoy por «" + canal.Name + "» a " + destino +
					". No se manda otra vez: está en Mensajes enviados con su resultado.",
			}, nil
		}
		return nil, err
	}
	msgID, _ := res.LastInsertId()

	// Un canal sin los datos mínimos no se finge: se registra el fallo y se dice
	// exactamente qué falta.
	var prov string
	var envioErr error
	switch canal.Kind {
	case "email":
		if strings.TrimSpace(canal.Host) == "" || strings.TrimSpace(canal.FromAddress) == "" {
			envioErr = fmt.Errorf("el canal de correo «%s» no tiene servidor o remitente configurado", canal.Name)
		} else {
			prov, envioErr = s.envioEmail(canal, destino, asunto, cuerpo)
		}
	case "sms":
		if strings.TrimSpace(canal.URL) == "" {
			envioErr = fmt.Errorf("el canal de SMS «%s» no tiene dirección de la pasarela configurada", canal.Name)
		} else {
			prov, envioErr = s.envioHTTP(canal, "sms", map[string]interface{}{
				"to": destino, "message": asunto + "\n" + cuerpo, "subject": asunto,
			})
		}
	case "wapp":
		if strings.TrimSpace(canal.URL) == "" {
			envioErr = fmt.Errorf("el canal de WhatsApp «%s» no tiene dirección de la pasarela configurada", canal.Name)
		} else {
			prov, envioErr = s.envioHTTP(canal, "wapp.message", map[string]interface{}{
				"to": destino, "message": "*" + asunto + "*\n\n" + cuerpo,
				"subject": asunto,
			})
		}
	case "gowa":
		// El aviso no se manda aquí: se encola. El despachador decide cuándo,
		// con el ritmo y el cupo del número. «Enviado» en este punto sería una
		// mentira: solo está aceptado por la cola.
		if strings.TrimSpace(canal.MsgDeviceID) == "" {
			envioErr = fmt.Errorf("el canal de WhatsApp «%s» no tiene un dispositivo asignado", canal.Name)
		} else {
			prov, envioErr = s.msgEncolarAviso(r, canal, destino, asunto, cuerpo, refType, refID, usuario)
		}
	case "webhook":
		if strings.TrimSpace(canal.URL) == "" {
			envioErr = fmt.Errorf("el canal webhook «%s» no tiene URL configurada", canal.Name)
		} else {
			prov, envioErr = s.envioHTTP(canal, "finance.aviso", map[string]interface{}{
				"ref_type": refType, "ref_id": refID, "subject": asunto, "body": cuerpo,
				"recipient": destino, "alert": finJSONMap(mustJSON(alerta["metricas"])),
				"kind": alerta["kind"], "party": alerta["party"],
			})
		}
	default:
		envioErr = fmt.Errorf("tipo de canal desconocido: %q", canal.Kind)
	}

	if envioErr != nil {
		_, _ = db.Exec(`UPDATE finance_message SET status = 'error', error = ? WHERE id = ?`, envioErr.Error(), msgID)
		return map[string]interface{}{
			"enviado": false, "id": msgID, "destino": destino, "canal": canal.Name,
			"msg": "No se pudo enviar por «" + canal.Name + "»: " + envioErr.Error(),
		}, nil
	}

	_, _ = db.Exec(`UPDATE finance_message SET status = 'enviado', sent_at = ?, provider_ref = ? WHERE id = ?`,
		now(), prov, msgID)
	if canal.Kind == "gowa" {
		// La cola es la verdad: el aviso está aceptado, no entregado. El
		// historial lo dice así y el estado real llegará con el acuse.
		_, _ = db.Exec(`UPDATE finance_message SET status = 'en_cola' WHERE id = ?`, msgID)
		return map[string]interface{}{
			"enviado": true, "encolado": true, "id": msgID, "destino": destino, "canal": canal.Name, "kind": canal.Kind,
			"msg": "Aviso en cola para " + destino + " por " + canal.Name + ". Se enviará con el ritmo del número; el historial dirá «en cola» hasta que salga.",
		}, nil
	}
	return map[string]interface{}{
		"enviado": true, "id": msgID, "destino": destino, "canal": canal.Name, "kind": canal.Kind,
		"msg": "Aviso enviado a " + destino + " por " + canal.Name + ".",
	}, nil
}

// ---------- Entrega: correo ----------

// envioEmail manda por SMTP con TLS implícito (465), STARTTLS (587) o sin cifrar
// (25, solo para un servidor de la propia red).
//
// NO se usa smtp.SendMail porque ese atajo no deja elegir TLS ni poner tiempos de
// espera: contra un servidor que no responde, el handler se quedaría colgado
// hasta que el cliente se cansara, con la pantalla esperando.
func (s *Server) envioEmail(canal notifyChannel, para, asunto, cuerpo string) (string, error) {
	host := strings.TrimSpace(canal.Host)
	port := canal.Port
	if port <= 0 {
		port = 587
	}
	dir := net.JoinHostPort(host, strconv.Itoa(port))
	desde := strings.TrimSpace(canal.FromAddress)
	nombre := strings.TrimSpace(canal.FromName)
	if nombre == "" {
		nombre = desde
	}

	var auth smtp.Auth
	if strings.TrimSpace(canal.Username) != "" {
		auth = smtp.PlainAuth("", canal.Username, canal.Password, host)
	}

	mensaje := construirMensaje(nombre, desde, para, asunto, cuerpo)

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if canal.Security == "tls" {
		conn, err := tls.DialWithDialer(dialer, "tcp", dir, &tls.Config{ServerName: host})
		if err != nil {
			return "", fmt.Errorf("no se pudo abrir la conexión cifrada con %s: %w", dir, err)
		}
		cliente, err := smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return "", fmt.Errorf("saludo SMTP fallido: %w", err)
		}
		defer func() { _ = cliente.Close() }()
		return entregarSMTP(cliente, auth, desde, para, mensaje)
	}

	conn, err := dialer.Dial("tcp", dir)
	if err != nil {
		return "", fmt.Errorf("no se pudo conectar con %s: %w", dir, err)
	}
	cliente, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return "", fmt.Errorf("saludo SMTP fallido: %w", err)
	}
	defer func() { _ = cliente.Close() }()

	if canal.Security != "none" {
		if ok, _ := cliente.Extension("STARTTLS"); ok {
			if err := cliente.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return "", fmt.Errorf("STARTTLS rechazado por %s: %w", host, err)
			}
		}
	}
	return entregarSMTP(cliente, auth, desde, para, mensaje)
}

func entregarSMTP(cliente *smtp.Client, auth smtp.Auth, desde, para, mensaje string) (string, error) {
	if auth != nil {
		if ok, _ := cliente.Extension("AUTH"); ok {
			if err := cliente.Auth(auth); err != nil {
				return "", fmt.Errorf("autenticación SMTP rechazada: %w", err)
			}
		}
	}
	if err := cliente.Mail(desde); err != nil {
		return "", fmt.Errorf("el servidor rechazó el remitente %s: %w", desde, err)
	}
	if err := cliente.Rcpt(para); err != nil {
		return "", fmt.Errorf("el servidor rechazó el destinatario %s: %w", para, err)
	}
	w, err := cliente.Data()
	if err != nil {
		return "", fmt.Errorf("el servidor no aceptó el mensaje: %w", err)
	}
	if _, err := w.Write([]byte(mensaje)); err != nil {
		_ = w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	_ = cliente.Quit()
	return "", nil
}

// construirMensaje arma el mensaje RFC 5322.
//
// El asunto y el remitente van codificados porque el texto es español y lleva
// tildes: sin codificar, un asunto como «Retención de IVA» llega roto o se
// descarta. El cuerpo va en UTF-8 declarado, que es lo que entienden todos los
// clientes actuales.
func construirMensaje(nombre, desde, para, asunto, cuerpo string) string {
	var b strings.Builder
	b.WriteString("From: " + codificarCabecera(nombre) + " <" + desde + ">\r\n")
	b.WriteString("To: <" + para + ">\r\n")
	b.WriteString("Subject: " + codificarCabecera(asunto) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(cuerpo, "\n", "\r\n"))
	return b.String()
}

func codificarCabecera(v string) string {
	v = strings.TrimSpace(v)
	for _, r := range v {
		if r > 127 {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(v)) + "?="
		}
	}
	return v
}

// ---------- Entrega: SMS y webhook ----------

// envioHTTP entrega por HTTP y devuelve la respuesta del proveedor (recortada),
// que es lo que hace diagnosticable un fallo.
//
// EL CONTRATO DE SMS ES DELIBERADAMENTE GENÉRICO: un POST con `to` y `message`.
// Así funciona con una pasarela propia, con un relé interno o con el webhook de
// un proveedor de mensajería, sin que este archivo tenga que conocer a ninguno.
// El webhook, además, va firmado: HMAC-SHA256 del cuerpo con `sign_secret`, en la
// cabecera X-ERPGo-Signature. Sin firma, cualquier cosa que llegue a esa URL
// parece legítima.
func (s *Server) envioHTTP(canal notifyChannel, evento string, payload map[string]interface{}) (string, error) {
	cuerpo, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	metodo := strings.ToUpper(strings.TrimSpace(canal.Method))
	if metodo == "" {
		metodo = http.MethodPost
	}
	req, err := http.NewRequest(metodo, canal.URL, bytes.NewReader(cuerpo))
	if err != nil {
		return "", fmt.Errorf("la URL del canal no es válida: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERPGo-Event", evento)
	req.Header.Set("User-Agent", "ERPGo-notify/1")
	if strings.TrimSpace(canal.AuthHeader) != "" && strings.TrimSpace(canal.AuthToken) != "" {
		valor := canal.AuthToken
		if !strings.HasPrefix(strings.ToLower(valor), "bearer ") && !strings.HasPrefix(strings.ToLower(valor), "basic ") {
			// Si el administrador puso solo el token, se asume Bearer, que es lo que
			// usan casi todos los servicios.
			valor = "Bearer " + valor
		}
		req.Header.Set(strings.TrimSpace(canal.AuthHeader), valor)
	}
	if strings.TrimSpace(canal.SignSecret) != "" {
		mac := hmac.New(sha256.New, []byte(canal.SignSecret))
		mac.Write(cuerpo)
		req.Header.Set("X-ERPGo-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	cliente := &http.Client{Timeout: 15 * time.Second}
	resp, err := cliente.Do(req)
	if err != nil {
		return "", fmt.Errorf("no se pudo entregar a %s: %w", hostDe(canal.URL), err)
	}
	defer resp.Body.Close()
	trozo, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
	texto := strings.TrimSpace(string(trozo))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if texto == "" {
			texto = resp.Status
		}
		return "", fmt.Errorf("el destino respondió %d: %s", resp.StatusCode, texto)
	}
	return texto, nil
}

// hostDe resume una URL a su host, para que un mensaje de error no escupa una URL
// con credenciales embebidas.
func hostDe(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return u
}
