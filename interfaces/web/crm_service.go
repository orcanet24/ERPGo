package web

// crm_service.go — Lógica del CRM.
//
// El CRM trata con la relación comercial ANTES de que exista una venta: quién
// es el contacto, en qué etapa está la oportunidad, qué se habló la última vez.
// Un cliente del ERP es, por definición, alguien a quien ya le facturamos.
//
// La decisión estructural es que el contacto y el cliente se enlazan en vez de
// duplicarse: `crm_contact.customer_id` se rellena al convertir la oportunidad.
// Duplicar el contacto y el cliente obliga a sincronizar dos registros que
// divergen, y la primera vez que divergen el sistema pierde la trazabilidad de
// qué contacto originó qué factura.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---------- Embudo ----------

// CRMStage es una etapa del embudo.
type CRMStage struct {
	ID          int64
	Name        string
	Position    int
	Probability int
	IsWon       bool
	IsLost      bool
}

// crmDefaultStages son las etapas que se siembran en una empresa nueva.
//
// Se siembran como DATOS y no como constantes en código porque el embudo de
// cada empresa es distinto: una que vende por catálogo necesita "Enviado" y una
// que vende el proyecto necesita "Licitación". Cambiar el embudo no debe requerir
// recompilar.
var crmDefaultStages = []struct {
	Name        string
	Probability int
	IsWon       bool
	IsLost      bool
}{
	{"Nuevo", 10, false, false},
	{"Contactado", 25, false, false},
	{"Cotización enviada", 50, false, false},
	{"Negociación", 75, false, false},
	{"Ganado", 100, true, false},
	{"Perdido", 0, false, true},
}

// SeedCRM crea las etapas base si la tabla está vacía.
func SeedCRM(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM crm_stage`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for i, s := range crmDefaultStages {
		if _, err := db.Exec(`INSERT INTO crm_stage (name, position, probability, is_won, is_lost, status)
			VALUES (?,?,?,?,?,1)`, s.Name, i, s.Probability, boolInt(s.IsWon), boolInt(s.IsLost)); err != nil {
			return fmt.Errorf("etapa CRM %s: %w", s.Name, err)
		}
	}
	return nil
}

// crmStages devuelve las etapas ordenadas como se muestran en el embudo.
func (s *Server) crmStages(r *http.Request) ([]CRMStage, error) {
	rows, err := s.dbFor(r).Query(`SELECT id, name, position, probability, is_won, is_lost
		FROM crm_stage WHERE status = 1 ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CRMStage
	for rows.Next() {
		var st CRMStage
		var won, lost int
		if rows.Scan(&st.ID, &st.Name, &st.Position, &st.Probability, &won, &lost) == nil {
			st.IsWon, st.IsLost = won == 1, lost == 1
			out = append(out, st)
		}
	}
	return out, rows.Err()
}

// crmStageByID devuelve una etapa concreta.
func (s *Server) crmStageByID(r *http.Request, id int64) (CRMStage, error) {
	var st CRMStage
	var won, lost int
	err := s.dbFor(r).QueryRow(`SELECT id, name, position, probability, is_won, is_lost
		FROM crm_stage WHERE id = ? AND status = 1`, id).
		Scan(&st.ID, &st.Name, &st.Position, &st.Probability, &won, &lost)
	if err != nil {
		return st, fmt.Errorf("la etapa no existe")
	}
	st.IsWon, st.IsLost = won == 1, lost == 1
	return st, nil
}

// ---------- Contactos ----------

// CRMContact es la ficha del contacto comercial.
type CRMContact struct {
	ID            int64
	FullName      string
	Email         string
	Phone         string
	CompanyName   string
	PositionTitle string
	CustomerID    sql.NullInt64
	SupplierID    sql.NullInt64
	Source        string
	OwnerID       int64
	Status        int
	Notes         string
}

// SaveContact crea o actualiza un contacto.
//
// Devuelve el id resultante. La unicidad del correo NO se impone: el mismo
// correo puede ser la persona de compras de dos clientes distintos, y forzar
// unicidad sería peor que permitir el duplicado visible.
func (s *Server) SaveContact(r *http.Request, c *CRMContact) (int64, error) {
	c.FullName = strings.TrimSpace(c.FullName)
	if c.FullName == "" {
		return 0, fmt.Errorf("el nombre del contacto es obligatorio")
	}
	if c.Email != "" && !strings.Contains(c.Email, "@") {
		return 0, fmt.Errorf("el correo no tiene un formato válido")
	}
	if c.CustomerID.Valid && c.CustomerID.Int64 <= 0 {
		c.CustomerID = sql.NullInt64{}
	}
	if c.SupplierID.Valid && c.SupplierID.Int64 <= 0 {
		c.SupplierID = sql.NullInt64{}
	}
	if c.Status == 0 {
		c.Status = 1
	}
	if c.Source == "" {
		c.Source = "otro"
	}

	if c.ID > 0 {
		_, err := s.dbFor(r).Exec(`UPDATE crm_contact SET
			full_name=?, email=?, phone=?, company_name=?, position_title=?,
			customer_id=?, supplier_id=?, source=?, owner_id=?, status=?, notes=?, updated_at=?
			WHERE id = ?`,
			c.FullName, c.Email, c.Phone, c.CompanyName, c.PositionTitle,
			nullInt(c.CustomerID), nullInt(c.SupplierID), c.Source, c.OwnerID,
			c.Status, c.Notes, now(), c.ID)
		return c.ID, err
	}
	res, err := s.dbFor(r).Exec(`INSERT INTO crm_contact
		(full_name, email, phone, company_name, position_title, customer_id, supplier_id,
		 source, owner_id, status, notes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.FullName, c.Email, c.Phone, c.CompanyName, c.PositionTitle,
		nullInt(c.CustomerID), nullInt(c.SupplierID), c.Source, c.OwnerID,
		c.Status, c.Notes, now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nullInt(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}

// LoadContact recupera un contacto con su historial de oportunidades.
func (s *Server) LoadContact(r *http.Request, id int64) (*CRMContact, error) {
	var c CRMContact
	err := s.dbFor(r).QueryRow(`SELECT id, full_name, IFNULL(email,''), IFNULL(phone,''),
		IFNULL(company_name,''), IFNULL(position_title,''), customer_id, supplier_id,
		IFNULL(source,''), IFNULL(owner_id,0), status, IFNULL(notes,'')
		FROM crm_contact WHERE id = ?`, id).
		Scan(&c.ID, &c.FullName, &c.Email, &c.Phone, &c.CompanyName, &c.PositionTitle,
			&c.CustomerID, &c.SupplierID, &c.Source, &c.OwnerID, &c.Status, &c.Notes)
	if err != nil {
		return nil, fmt.Errorf("el contacto no existe")
	}
	return &c, nil
}

// ---------- Oportunidades ----------

// CRMOpportunity es una oportunidad comercial con su valor y su etapa.
type CRMOpportunity struct {
	ID            int64
	Title         string
	ContactID     int64
	ContactName   string
	StageID       int64
	StageName     string
	Amount        float64
	ExpectedClose string
	OwnerID       int64
	Description   string
	LostReason    string
	ClosedAt      string
}

// SaveOpportunity crea o actualiza una oportunidad, con sus validaciones.
func (s *Server) SaveOpportunity(r *http.Request, o *CRMOpportunity) (int64, error) {
	o.Title = strings.TrimSpace(o.Title)
	if o.Title == "" {
		return 0, fmt.Errorf("el título de la oportunidad es obligatorio")
	}
	if o.ContactID <= 0 {
		return 0, fmt.Errorf("la oportunidad debe tener un contacto")
	}
	if o.ExpectedClose != "" && !aiValidDate(o.ExpectedClose) {
		return 0, fmt.Errorf("la fecha de cierre esperado debe tener el formato AAAA-MM-DD")
	}
	if o.Amount < 0 {
		return 0, fmt.Errorf("el monto no puede ser negativo")
	}

	// La etapa debe existir: sin esta comprobación, una oportunidad puede quedar
	// en una etapa que nadie puede ver en el embudo, que es una venta perdida
	// sin que nadie lo sepa.
	if o.StageID > 0 {
		if _, err := s.crmStageByID(r, o.StageID); err != nil {
			return 0, err
		}
	}

	// Cerrar deja constancia de cuándo y por qué. Una oportunidad perdida sin
	// motivo no teaches nada y en seis meses nadie sabe qué se dejó de intentar.
	closedAt := o.ClosedAt
	if o.StageID > 0 {
		if st, err := s.crmStageByID(r, o.StageID); err == nil && (st.IsWon || st.IsLost) {
			if closedAt == "" {
				closedAt = now()
			}
			if st.IsLost && strings.TrimSpace(o.LostReason) == "" {
				return 0, fmt.Errorf("una oportunidad perdida necesita un motivo")
			}
		} else {
			closedAt = ""
		}
	}

	if o.ID > 0 {
		_, err := s.dbFor(r).Exec(`UPDATE crm_opportunity SET
			title=?, contact_id=?, stage_id=?, amount=?, expected_close=?, owner_id=?,
			description=?, lost_reason=?, closed_at=?, updated_at=?
			WHERE id = ?`,
			o.Title, o.ContactID, o.StageID, o.Amount, o.ExpectedClose, o.OwnerID,
			o.Description, o.LostReason, closedAt, now(), o.ID)
		return o.ID, err
	}
	res, err := s.dbFor(r).Exec(`INSERT INTO crm_opportunity
		(title, contact_id, stage_id, amount, expected_close, owner_id,
		 description, lost_reason, closed_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		o.Title, o.ContactID, o.StageID, o.Amount, o.ExpectedClose, o.OwnerID,
		o.Description, o.LostReason, closedAt, now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MoveOpportunity cambia la etapa de una oportunidad.
//
// Mover a "Ganado" es el momento en que la venta se vuelve real: es donde el
// contacto deja de ser una promesa y pasa a poder facturarse.
func (s *Server) MoveOpportunity(r *http.Request, id, stageID int64, lostReason string) error {
	st, err := s.crmStageByID(r, stageID)
	if err != nil {
		return err
	}
	if st.IsLost && strings.TrimSpace(lostReason) == "" {
		return fmt.Errorf("una oportunidad perdida necesita un motivo")
	}
	closed := ""
	if st.IsWon || st.IsLost {
		closed = now()
	}
	_, err = s.dbFor(r).Exec(`UPDATE crm_opportunity SET stage_id=?, lost_reason=?, closed_at=?, updated_at=?
		WHERE id = ?`, stageID, lostReason, closed, now(), id)
	return err
}

// crmPipeline agrupa las oportunidades por etapa y calcula el valor ponderado.
//
// El valor ponderado (importe × probabilidad) es lo que convierte al embudo en
// una previsión de ingresos. Sin él, el embudo es un tablero bonito que no
// sirve para decidir.
type CRMPipelineStage struct {
	CRMStage
	Count    int
	Amount   float64
	Weighted float64
}

func (s *Server) crmPipeline(r *http.Request) ([]CRMPipelineStage, error) {
	stages, err := s.crmStages(r)
	if err != nil {
		return nil, err
	}
	type agg struct {
		count  int
		amount float64
	}
	porStage := map[int64]agg{}
	rows, err := s.dbFor(r).Query(`SELECT stage_id, COUNT(*), IFNULL(SUM(amount),0)
		FROM crm_opportunity WHERE status = 1 GROUP BY stage_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var a agg
		if rows.Scan(&id, &a.count, &a.amount) == nil {
			porStage[id] = a
		}
	}

	out := make([]CRMPipelineStage, 0, len(stages))
	for _, st := range stages {
		a := porStage[st.ID]
		p := CRMPipelineStage{CRMStage: st, Count: a.count, Amount: round2(a.amount)}
		p.Weighted = round2(a.amount * float64(st.Probability) / 100)
		out = append(out, p)
	}
	return out, nil
}

// ---------- Actividades ----------

// LogActivity registra una interacción y, si la oportunidad está en la etapa
// "Ganado", enlaza además el contacto con el cliente del ERP.
//
// Ese enlace automático es la razón de ser del módulo: convierte un contacto
// suelto en algo facturable sin que nadie tenga que acordarse de hacerlo.
func (s *Server) LogActivity(r *http.Request, contactID, oppID int64, tipo, subject, body, dueDate string, createdBy int64) (int64, error) {
	if contactID <= 0 && oppID <= 0 {
		return 0, fmt.Errorf("la actividad debe vincularse a un contacto o una oportunidad")
	}
	if oppID > 0 && contactID == 0 {
		if err := s.dbFor(r).QueryRow(`SELECT contact_id FROM crm_opportunity WHERE id = ?`, oppID).Scan(&contactID); err != nil {
			return 0, fmt.Errorf("la oportunidad no existe")
		}
	}
	if dueDate != "" && !aiValidDate(dueDate) {
		return 0, fmt.Errorf("la fecha debe tener el formato AAAA-MM-DD")
	}
	// body se pasa tal cual: la columna es NULLable, y envolverla con IFNULL
	// dentro de la lista de columnas del INSERT no es SQL válido (IFNULL es
	// una expresión de valor, no un nombre de columna).
	res, err := s.dbFor(r).Exec(`INSERT INTO crm_activity
		(contact_id, opportunity_id, activity_type, subject, body, due_date, done, created_by, created_at)
		VALUES (?,?,?,?,?,?,0,?,?)`,
		contactID, oppID, tipo, subject, body, dueDate, createdBy, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CompleteActivity marca una actividad como hecha con su fecha de cierre.
func (s *Server) CompleteActivity(r *http.Request, id int64, done bool) error {
	completed := ""
	if done {
		completed = now()
	}
	_, err := s.dbFor(r).Exec(`UPDATE crm_activity SET done = ?, completed_at = ? WHERE id = ?`,
		boolInt(done), completed, id)
	return err
}

// LinkContactToCustomer enlaza un contacto ya ganado con su ficha de cliente,
// para que el historial comercial y las facturas queden unidos.
func (s *Server) LinkContactToCustomer(r *http.Request, contactID, customerID int64) error {
	if customerID <= 0 {
		return fmt.Errorf("debe indicar el cliente")
	}
	var existe int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM customer_information WHERE customer_id = ?`, customerID).Scan(&existe); err != nil {
		return err
	}
	if existe == 0 {
		return fmt.Errorf("el cliente indicado no existe")
	}
	_, err := s.dbFor(r).Exec(`UPDATE crm_contact SET customer_id = ?, updated_at = ? WHERE id = ?`,
		customerID, now(), contactID)
	return err
}

// crmVencidas son las actividades pendientes cuya fecha ya pasó.
//
// Usa la fecha de HOY de la empresa, no la del servidor: el cierre de caja en
// Venezuela puede ir un día o más detrás del reloj del equipo, y marcar como
// vencida una tarea que el usuario aún no ha visto sería una molestia diaria.
func (s *Server) crmOverdue(r *http.Request, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.dbFor(r).Query(`
		SELECT a.id, IFNULL(a.subject,''), a.activity_type, a.due_date,
		       IFNULL(c.full_name,''), IFNULL(o.title,'')
		FROM crm_activity a
		LEFT JOIN crm_contact c ON c.id = a.contact_id
		LEFT JOIN crm_opportunity o ON o.id = a.opportunity_id
		WHERE a.done = 0 AND a.due_date IS NOT NULL AND a.due_date < ?
		ORDER BY a.due_date LIMIT ?`, today(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var subject, tipo, due, contact, opp string
		if rows.Scan(&id, &subject, &tipo, &due, &contact, &opp) == nil {
			out = append(out, map[string]interface{}{
				"id": id, "subject": subject, "type": tipo, "due_date": due,
				"contact": contact, "opportunity": opp,
			})
		}
	}
	return out, rows.Err()
}

// crmContactoStats resume la actividad de un contacto en los últimos días.
func (s *Server) crmContactoStats(r *http.Request, contactID int64, days int) (map[string]int, error) {
	if days <= 0 {
		days = 30
	}
	desde := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	var total, llamadas, reuniones, correos int
	err := s.dbFor(r).QueryRow(`
		SELECT COUNT(*),
		       SUM(CASE WHEN activity_type='llamada' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN activity_type='reunion' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN activity_type='email' THEN 1 ELSE 0 END)
		FROM crm_activity
		WHERE contact_id = ? AND COALESCE(NULLIF(CAST(due_date AS CHAR), '0000-00-00'), substr(created_at,1,10)) >= ?`,
		contactID, desde).Scan(&total, &llamadas, &reuniones, &correos)
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"total": total, "llamadas": llamadas,
		"reuniones": reuniones, "correos": correos, "dias": days,
	}, nil
}
