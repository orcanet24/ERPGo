package web

// crm_remarketing_service.go — Lógica de remarketing dentro del CRM.
//
// El remarketing es la extensión natural del CRM: usa contactos, clientes,
// oportunidades y canales de aviso (notify_channel) que ya existen. No duplica
// datos; los referencia por ID.
//
// Flujo:
//   1. Admin crea un segmento (regla: "no compran hace 60 días", "oportunidad
//      perdida", etc.)
//   2. Admin crea una plantilla de mensaje con variables (%nombre%, %empresa%)
//   3. Admin crea una campaña: elige segmento + plantilla + canal (email/SMS/WA)
//   4. Al lanzar la campaña, el sistema resuelve los contactos del segmento,
//      les aplica la plantilla y despacha por el canal configurado.
//
// La idempotencia se garantiza por (campaign_id + contact_id): un contacto
// solo recibe una campaña una vez. El envío por canal reutiliza outbound.go
// que ya tiene su propia capa de idempotencia.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Segmentos ----------

// CRMSegment es un grupo de contactos definido por una regla.
type CRMSegment struct {
	ID          int64
	Name        string
	Description string
	RuleType    string
	RuleDays    int
	Status      int
	CreatedAt   string
	UpdatedAt   string
}

// Segmentos predefinidos que se siembran en una empresa nueva.
// Se siembran como DATOS porque cada empresa puede necesitar reglas distintas.
var crmDefaultRemarketingSegments = []struct {
	Name     string
	RuleType string
	RuleDays int
}{
	{"No compran hace 60 días", "no_purchase_60d", 60},
	{"Oportunidades perdidas", "opportunity_lost", 0},
	{"Cotizaciones enviadas sin cerrar", "quote_sent_open", 0},
}

// SeedRemarketingSegments crea los segmentos base si la tabla está vacía.
func SeedRemarketingSegments(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM crm_remarketing_segment`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, s := range crmDefaultRemarketingSegments {
		if _, err := db.Exec(`INSERT INTO crm_remarketing_segment (name, rule_type, rule_days, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?)`, s.Name, s.RuleType, s.RuleDays, 1, now(), now()); err != nil {
			return fmt.Errorf("segmento remarketing %s: %w", s.Name, err)
		}
	}
	return nil
}

// SaveSegment crea o actualiza un segmento.
func (s *Server) SaveSegment(r *http.Request, seg *CRMSegment) (int64, error) {
	seg.Name = strings.TrimSpace(seg.Name)
	if seg.Name == "" {
		return 0, fmt.Errorf("el nombre del segmento es obligatorio")
	}
	if seg.RuleType == "" {
		seg.RuleType = "no_purchase_60d"
	}
	if seg.Status == 0 && seg.ID == 0 {
		seg.Status = 1
	}

	if seg.ID > 0 {
		_, err := s.dbFor(r).Exec(`UPDATE crm_remarketing_segment SET
			name=?, description=?, rule_type=?, rule_days=?, status=?, updated_at=?
			WHERE id = ?`,
			seg.Name, seg.Description, seg.RuleType, seg.RuleDays, seg.Status, now(), seg.ID)
		return seg.ID, err
	}
	res, err := s.dbFor(r).Exec(`INSERT INTO crm_remarketing_segment
		(name, description, rule_type, rule_days, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`,
		seg.Name, seg.Description, seg.RuleType, seg.RuleDays, seg.Status, now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LoadSegment recupera un segmento por id.
func (s *Server) LoadSegment(r *http.Request, id int64) (*CRMSegment, error) {
	var seg CRMSegment
	err := s.dbFor(r).QueryRow(`SELECT id, IFNULL(name,''), IFNULL(description,''),
		IFNULL(rule_type,'no_purchase_60d'), IFNULL(rule_days,0), status,
		IFNULL(created_at,''), IFNULL(updated_at,'')
		FROM crm_remarketing_segment WHERE id = ?`, id).
		Scan(&seg.ID, &seg.Name, &seg.Description, &seg.RuleType, &seg.RuleDays,
			&seg.Status, &seg.CreatedAt, &seg.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("el segmento no existe")
	}
	return &seg, nil
}

// crmSegmentContacts devuelve los contactos que cumplen la regla del segmento.
//
// Cada regla se traduce a una consulta SQL contra las tablas CRM y de negocio
// existentes. Si se añade una nueva regla, se añade aquí su rama.
func (s *Server) crmSegmentContacts(r *http.Request, seg *CRMSegment) ([]map[string]interface{}, error) {
	db := s.dbFor(r)
	hace := time.Now().AddDate(0, 0, -seg.RuleDays).Format("2006-01-02")

	switch seg.RuleType {
	case "no_purchase_60d", "no_purchase_30d", "no_purchase_90d", "no_activity_Nd":
		// Clientes cuya última factura (o creación) es anterior a la fecha límite,
		// o que nunca han comprado.
		rows, err := db.Query(`
			SELECT c.id, IFNULL(c.full_name,''), IFNULL(c.email,''), IFNULL(c.phone,''),
			       IFNULL(c.company_name,''), IFNULL(c.customer_id,0)
			FROM crm_contact c
			WHERE c.status = 1
			  AND (
			    SELECT COALESCE(MAX(i.date), '')
			    FROM invoice i
			    WHERE i.customer_id = c.customer_id
			  ) < ?
			ORDER BY c.full_name LIMIT 500`, hace)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanContactRows(rows)

	case "opportunity_lost":
		rows, err := db.Query(`
			SELECT DISTINCT c.id, IFNULL(c.full_name,''), IFNULL(c.email,''),
			       IFNULL(c.phone,''), IFNULL(c.company_name,''), IFNULL(c.customer_id,0)
			FROM crm_opportunity o
			JOIN crm_contact c ON c.id = o.contact_id
			JOIN crm_stage st ON st.id = o.stage_id AND st.is_lost = 1
			WHERE c.status = 1
			ORDER BY c.full_name LIMIT 500`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanContactRows(rows)

	case "quote_sent_open":
		// Contactos con oportunidades en etapa "Cotización enviada" que siguen abiertas.
		rows, err := db.Query(`
			SELECT DISTINCT c.id, IFNULL(c.full_name,''), IFNULL(c.email,''),
			       IFNULL(c.phone,''), IFNULL(c.company_name,''), IFNULL(c.customer_id,0)
			FROM crm_opportunity o
			JOIN crm_contact c ON c.id = o.contact_id
			JOIN crm_stage st ON st.id = o.stage_id AND st.probability BETWEEN 40 AND 60
			WHERE c.status = 1 AND o.status = 1
			ORDER BY c.full_name LIMIT 500`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanContactRows(rows)

	case "customer_all":
		rows, err := db.Query(`
			SELECT id, IFNULL(full_name,''), IFNULL(email,''), IFNULL(phone,''),
			       IFNULL(company_name,''), IFNULL(customer_id,0)
			FROM crm_contact
			WHERE status = 1 AND customer_id IS NOT NULL
			ORDER BY full_name LIMIT 500`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanContactRows(rows)

	case "contact_all":
		rows, err := db.Query(`
			SELECT id, IFNULL(full_name,''), IFNULL(email,''), IFNULL(phone,''),
			       IFNULL(company_name,''), IFNULL(customer_id,0)
			FROM crm_contact
			WHERE status = 1
			ORDER BY full_name LIMIT 500`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanContactRows(rows)

	default:
		return nil, fmt.Errorf("tipo de regla no reconocido: %s", seg.RuleType)
	}
}

// scanContactRows lee las filas de contactos en un formato uniforme.
func scanContactRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id, customerID int64
		var nombre, email, telefono, empresa string
		if rows.Scan(&id, &nombre, &email, &telefono, &empresa, &customerID) == nil {
			list = append(list, map[string]interface{}{
				"id": id, "name": nombre, "email": email, "phone": telefono,
				"company": empresa, "customer_id": customerID,
			})
		}
	}
	return list, rows.Err()
}

// ---------- Plantillas ----------

// CRMTemplate es un mensaje reutilizable con variables.
type CRMTemplate struct {
	ID        int64
	Name      string
	Subject   string
	Body      string
	Status    int
	CreatedAt string
	UpdatedAt string
}

// SaveTemplate crea o actualiza una plantilla.
func (s *Server) SaveTemplate(r *http.Request, tpl *CRMTemplate) (int64, error) {
	tpl.Name = strings.TrimSpace(tpl.Name)
	if tpl.Name == "" {
		return 0, fmt.Errorf("el nombre de la plantilla es obligatorio")
	}
	tpl.Body = strings.TrimSpace(tpl.Body)
	if tpl.Body == "" {
		return 0, fmt.Errorf("el cuerpo del mensaje es obligatorio")
	}

	if tpl.ID > 0 {
		_, err := s.dbFor(r).Exec(`UPDATE crm_remarketing_template SET
			name=?, subject=?, body=?, status=?, updated_at=? WHERE id=?`,
			tpl.Name, tpl.Subject, tpl.Body, tpl.Status, now(), tpl.ID)
		return tpl.ID, err
	}
	res, err := s.dbFor(r).Exec(`INSERT INTO crm_remarketing_template
		(name, subject, body, status, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		tpl.Name, tpl.Subject, tpl.Body, 1, now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LoadTemplate recupera una plantilla por id.
func (s *Server) LoadTemplate(r *http.Request, id int64) (*CRMTemplate, error) {
	var tpl CRMTemplate
	err := s.dbFor(r).QueryRow(`SELECT id, IFNULL(name,''), IFNULL(subject,''), IFNULL(body,''),
		status, IFNULL(created_at,''), IFNULL(updated_at,'')
		FROM crm_remarketing_template WHERE id = ?`, id).
		Scan(&tpl.ID, &tpl.Name, &tpl.Subject, &tpl.Body, &tpl.Status, &tpl.CreatedAt, &tpl.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("la plantilla no existe")
	}
	return &tpl, nil
}

// crmApplyTemplate reemplaza las variables del cuerpo con los datos del contacto.
func crmApplyTemplate(body string, contact map[string]interface{}) string {
	replacer := strings.NewReplacer(
		"%nombre%", fmt.Sprintf("%v", contact["name"]),
		"%empresa%", fmt.Sprintf("%v", contact["company"]),
		"%telefono%", fmt.Sprintf("%v", contact["phone"]),
		"%email%", fmt.Sprintf("%v", contact["email"]),
	)
	return replacer.Replace(body)
}

// ---------- Campañas ----------

// CRMCampaign es un envío de remarketing a un segmento.
type CRMCampaign struct {
	ID              int64
	Name            string
	SegmentID       int64
	TemplateID      int64
	ChannelID       int64
	Status          int
	TotalRecipients int
	SentCount       int
	ErrorCount      int
	ScheduledAt     string
	CreatedBy       int64
	CreatedAt       string
	UpdatedAt       string
}

// SaveCampaign crea o actualiza una campaña.
func (s *Server) SaveCampaign(r *http.Request, camp *CRMCampaign) (int64, error) {
	camp.Name = strings.TrimSpace(camp.Name)
	if camp.Name == "" {
		return 0, fmt.Errorf("el nombre de la campaña es obligatorio")
	}
	if camp.SegmentID <= 0 {
		return 0, fmt.Errorf("debe elegir un segmento")
	}
	if camp.TemplateID <= 0 {
		return 0, fmt.Errorf("debe elegir una plantilla")
	}
	if camp.ChannelID <= 0 {
		return 0, fmt.Errorf("debe elegir un canal")
	}

	if camp.ID > 0 {
		_, err := s.dbFor(r).Exec(`UPDATE crm_remarketing_campaign SET
			name=?, segment_id=?, template_id=?, channel_id=?, updated_at=? WHERE id=?`,
			camp.Name, camp.SegmentID, camp.TemplateID, camp.ChannelID, now(), camp.ID)
		return camp.ID, err
	}
	res, err := s.dbFor(r).Exec(`INSERT INTO crm_remarketing_campaign
		(name, segment_id, template_id, channel_id, status, created_by, created_at, updated_at)
		VALUES (?,?,?,?,0,?,?,?)`,
		camp.Name, camp.SegmentID, camp.TemplateID, camp.ChannelID, sessUserID(s.sessionFromRequest(r)), now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// lanzaCampaña resuelve los contactos del segmento, genera los destinatarios
// y envía por el canal. Actualiza contadores en la campaña.
//
// Es deliberadamente síncrona y secuencial: una campaña de remarketing PYME no
// supera los cientos de contactos, y hacerlo uno a uno permite registrar cada
// resultado y frenar ante error sin perder el rastro.
func (s *Server) lanzaCampaña(r *http.Request, campaignID int64) error {
	db := s.dbFor(r)

	camp, err := s.loadCampaign(db, campaignID)
	if err != nil {
		return err
	}
	seg, err := s.LoadSegment(r, camp.SegmentID)
	if err != nil {
		return err
	}
	tpl, err := s.LoadTemplate(r, camp.TemplateID)
	if err != nil {
		return err
	}
	canal, err := s.notifyCanalPorID(db, camp.ChannelID)
	if err != nil {
		return fmt.Errorf("canal de aviso: %w", err)
	}

	contactos, err := s.crmSegmentContacts(r, seg)
	if err != nil {
		return err
	}

	if _, err := db.Exec(`UPDATE crm_remarketing_campaign SET status=1, total_recipients=?, sent_count=0, error_count=0, updated_at=? WHERE id=?`,
		len(contactos), now(), campaignID); err != nil {
		return err
	}

	for _, c := range contactos {
		contactID := c["id"].(int64)
		destino := s.crmDestinoContacto(canal, c)
		if destino == "" {
			// Sin destino para este canal (sin email/sin teléfono): se salta
			// pero deja rastro como error para que el admin lo vea.
			s.crmLogRecipient(db, campaignID, contactID, c["customer_id"].(int64), "", "error", "contacto sin "+s.crmDestinoTipo(canal))
			continue
		}

		idem := aiHashKey("remkt|" + strconv.FormatInt(campaignID, 10) + "|" + strconv.FormatInt(contactID, 10))
		if err := s.crmLogRecipient(db, campaignID, contactID, c["customer_id"].(int64), destino, "pendiente", ""); err != nil {
			continue
		}

		cuerpo := crmApplyTemplate(tpl.Body, c)
		asunto := crmApplyTemplate(tpl.Subject, c)

		if _, err := db.Exec(`UPDATE crm_remarketing_recipient SET status='enviando', idem_key=? WHERE campaign_id=? AND contact_id=?`,
			idem, campaignID, contactID); err != nil {
			continue
		}

		prov, envioErr := s.crmDespachar(r, db, canal, destino, asunto, cuerpo)
		if envioErr != nil {
			db.Exec(`UPDATE crm_remarketing_recipient SET status='error', error_msg=?, sent_at=? WHERE campaign_id=? AND contact_id=?`,
				envioErr.Error(), now(), campaignID, contactID)
			db.Exec(`UPDATE crm_remarketing_campaign SET error_count=error_count+1, updated_at=? WHERE id=?`, now(), campaignID)
		} else {
			db.Exec(`UPDATE crm_remarketing_recipient SET status='enviado', sent_at=?, provider_ref=? WHERE campaign_id=? AND contact_id=?`,
				now(), prov, campaignID, contactID)
			db.Exec(`UPDATE crm_remarketing_campaign SET sent_count=sent_count+1, updated_at=? WHERE id=?`, now(), campaignID)
		}
	}

	db.Exec(`UPDATE crm_remarketing_campaign SET status=2, updated_at=? WHERE id=?`, now(), campaignID)
	return nil
}

// crmDestinoContacto resuelve a dónde le llega el mensaje a un contacto,
// según el tipo de canal.
func (s *Server) crmDestinoContacto(canal notifyChannel, c map[string]interface{}) string {
	switch canal.Kind {
	case "email":
		email := strings.TrimSpace(fmt.Sprintf("%v", c["email"]))
		if email == "" || !strings.Contains(email, "@") {
			return ""
		}
		return email
	case "sms", "wapp":
		telefono := strings.TrimSpace(fmt.Sprintf("%v", c["phone"]))
		if telefono == "" {
			return ""
		}
		return telefono
	default:
		return ""
	}
}

// crmDestinoTipo describe qué campo necesita un canal, para mensajes de error.
func (s *Server) crmDestinoTipo(canal notifyChannel) string {
	switch canal.Kind {
	case "email":
		return "correo"
	case "sms":
		return "teléfono"
	case "wapp":
		return "teléfono"
	default:
		return "destino"
	}
}

// crmLogRecipient escribe una fila de destinatario (para idempotencia y rastro).
func (s *Server) crmLogRecipient(db *sql.DB, campaignID, contactID, customerID int64, destino, status, errMsg string) error {
	_, err := db.Exec(`INSERT INTO crm_remarketing_recipient
		(campaign_id, contact_id, customer_id, recipient, status, error_msg, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		campaignID, contactID, customerID, destino, status, errMsg, now())
	return err
}

// crmDespachar envía un mensaje por el canal usando la infraestructura de
// outbound.go. Reutiliza financeEnviarAviso con una alerta sintética.
func (s *Server) crmDespachar(r *http.Request, db *sql.DB, canal notifyChannel, destino, asunto, cuerpo string) (string, error) {
	switch canal.Kind {
	case "email":
		return s.envioEmail(canal, destino, asunto, cuerpo)
	case "sms":
		return s.envioHTTP(canal, "sms", map[string]interface{}{
			"to": destino, "message": asunto + "\n" + cuerpo, "subject": asunto,
		})
	case "wapp":
		return s.envioHTTP(canal, "wapp.message", map[string]interface{}{
			"to": destino, "message": "*" + asunto + "*\n\n" + cuerpo, "subject": asunto,
		})
	default:
		return "", fmt.Errorf("tipo de canal no soportado para remarketing: %s", canal.Kind)
	}
}

// loadCampaign recupera una campaña.
func (s *Server) loadCampaign(db *sql.DB, id int64) (*CRMCampaign, error) {
	var camp CRMCampaign
	err := db.QueryRow(`SELECT id, IFNULL(name,''), segment_id, template_id, channel_id,
		status, IFNULL(total_recipients,0), IFNULL(sent_count,0), IFNULL(error_count,0),
		IFNULL(scheduled_at,''), IFNULL(created_by,0), IFNULL(created_at,''), IFNULL(updated_at,'')
		FROM crm_remarketing_campaign WHERE id = ?`, id).
		Scan(&camp.ID, &camp.Name, &camp.SegmentID, &camp.TemplateID, &camp.ChannelID,
			&camp.Status, &camp.TotalRecipients, &camp.SentCount, &camp.ErrorCount,
			&camp.ScheduledAt, &camp.CreatedBy, &camp.CreatedAt, &camp.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("la campaña no existe")
	}
	return &camp, nil
}
