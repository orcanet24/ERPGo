package web

// mensajeria_contactos.go — A quién se le puede escribir y por dónde.
//
// ESTE ARCHIVO EXISTE PARA NO ADIVINAR. «Este cliente tiene WhatsApp» no se
// deduce de que el campo teléfono no esté vacío: se sabe porque hay una fila
// verificada en `msg_contact_channel`, con fecha y origen. Y «este cliente tiene
// Telegram» no se deduce del teléfono si el proveedor es un bot: un bot solo
// puede escribir a quien ya inició el bot, y eso se guarda como `chat_id`.
//
// LA VERIFICACIÓN NO SE HACE AL ENVIAR. Cada consulta al daemon es tráfico y
// levanta sospecha; además, un aviso urgente no puede depender de una llamada
// externa que puede tardar o fallar. Se verifica a mano (o con el primer
// mensaje entrante) y se guarda con caducidad.

import (
	"database/sql"
	"strings"
	"time"
)

// msgOrdenDeCanales es el orden por defecto cuando la empresa no ha elegido
// otro. WhatsApp primero porque es donde está la relación previa; Telegram
// después; SMS y correo como red de seguridad.
var msgOrdenDeCanales = []string{"whatsapp_gowa", "telegram_user", "telegram_bot", "sms", "email"}

// msgCanalVerificado es una fila de `msg_contact_channel`.
type msgCanalVerificado struct {
	MSISDN     string
	Provider   string
	Address    string
	VerifiedAt string
	Source     string
	Status     string
	Preference int
	Notes      string
}

// msgCanalesDeContacto lista los canales de un número.
func (s *Server) msgCanalesDeContacto(db *sql.DB, msisdn string) []msgCanalVerificado {
	out := []msgCanalVerificado{}
	rows, err := db.Query(`SELECT msisdn, provider, IFNULL(address,''), IFNULL(verified_at,''),
		IFNULL(source,''), IFNULL(status,'verificado'), IFNULL(preference,0), IFNULL(notes,'')
		FROM msg_contact_channel WHERE msisdn = ? ORDER BY preference DESC, provider`, msisdn)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c msgCanalVerificado
		if err := rows.Scan(&c.MSISDN, &c.Provider, &c.Address, &c.VerifiedAt, &c.Source,
			&c.Status, &c.Preference, &c.Notes); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// msgVerificarCanal guarda un canal comprobado. Si ya existía, lo actualiza: la
// verificación nueva manda sobre la vieja.
func (s *Server) msgVerificarCanal(db *sql.DB, msisdn, provider, address, source string) error {
	msisdn = strings.TrimSpace(msisdn)
	provider = strings.TrimSpace(strings.ToLower(provider))
	if msisdn == "" || provider == "" {
		return nil
	}
	address = strings.TrimSpace(address)
	if address == "" {
		address = msisdn
	}
	var hay int
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_contact_channel WHERE msisdn=? AND provider=?`,
		msisdn, provider).Scan(&hay); err != nil {
		return err
	}
	if hay > 0 {
		_, err := db.Exec(`UPDATE msg_contact_channel SET address=?, verified_at=?, source=?, status='verificado'
			WHERE msisdn=? AND provider=?`, address, now(), source, msisdn, provider)
		return err
	}
	_, err := db.Exec(`INSERT INTO msg_contact_channel (msisdn, provider, address, verified_at, source, status, preference, notes)
		VALUES (?,?,?,?,?,'verificado',0,'')`, msisdn, provider, address, now(), source)
	return err
}

// msgResolverCanal decide por dónde sale un aviso, en orden y sin adivinar.
//
//  1. La preferencia explícita del contacto manda, si ese canal está verificado.
//  2. Si no, se recorre el orden y se devuelve el primero verificado.
//  3. Si no hay ninguno, se devuelve el motivo. El llamador bloquea el aviso con
//     ese motivo; NUNCA cae a otro canal ni a otro destinatario.
func (s *Server) msgResolverCanal(db *sql.DB, msisdn string, orden []string) (string, string, string) {
	msisdn = strings.TrimSpace(msisdn)
	if msisdn == "" {
		return "", "", "la ficha no tiene un teléfono normalizado"
	}
	if len(orden) == 0 {
		orden = msgOrdenDeCanales
	}
	canales := s.msgCanalesDeContacto(db, msisdn)
	if len(canales) == 0 {
		return "", "", "el contacto no tiene ningún canal verificado; verifique su número antes de avisarle"
	}

	// La preferencia explícita solo vale si está verificada.
	for _, c := range canales {
		if c.Preference == 1 && c.Status == "verificado" {
			for _, p := range orden {
				if p == c.Provider {
					return c.Provider, c.Address, ""
				}
			}
		}
	}
	for _, p := range orden {
		for _, c := range canales {
			if c.Provider == p && c.Status == "verificado" {
				return c.Provider, c.Address, ""
			}
		}
	}
	return "", "", "el contacto no tiene un canal verificado del tipo que este aviso necesita"
}

// ---------- Opt-out ----------

// msgEstaEnOptOut dice si el número pidió no recibir más mensajes.
func msgEstaEnOptOut(db *sql.DB, msisdn string) bool {
	msisdn = strings.TrimSpace(msisdn)
	if msisdn == "" {
		return false
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_optout WHERE msisdn = ?`, msisdn).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// msgRegistrarOptOut deja constancia de la baja. Es idempotente: si ya estaba,
// no se duplica ni se pisa el motivo original.
func (s *Server) msgRegistrarOptOut(db *sql.DB, msisdn, motivo, origen, usuario string) error {
	msisdn = strings.TrimSpace(msisdn)
	if msisdn == "" {
		return nil
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_optout WHERE msisdn = ?`, msisdn).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := db.Exec(`INSERT INTO msg_optout (msisdn, reason, source, created_by, created_at)
		VALUES (?,?,?,?,?)`, msisdn, motivo, origen, usuario, now())
	return err
}

// msgOptOutDeTexto reconoce una baja escrita por la persona.
//
// Se exige la palabra SOLA (o con espacios alrededor). Buscar «baja» dentro de
// cualquier frase marcaría como baja a quien escribe «la factura está baja»,
// que es justo el tipo de falso positivo que deja a un cliente sin sus avisos.
func msgOptOutDeTexto(cuerpo string) bool {
	t := strings.ToUpper(strings.TrimSpace(cuerpo))
	t = strings.Trim(t, " .,;:!¡¿?")
	switch t {
	case "BAJA", "STOP", "NO MOLESTAR", "UNSUBSCRIBE", "CANCELAR", "DARME DE BAJA":
		return true
	}
	return false
}

// msgCanalCaducado dice si una verificación es demasiado vieja para confiar en
// ella. No se borra: se vuelve a verificar y se actualiza.
func msgCanalCaducado(verificado string, dias int) bool {
	if strings.TrimSpace(verificado) == "" {
		return true
	}
	t := msgFechaDe(verificado)
	if t.IsZero() {
		return true
	}
	return time.Since(t) > time.Duration(dias)*24*time.Hour
}
