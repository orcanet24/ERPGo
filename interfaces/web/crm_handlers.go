package web

// crm_handlers.go — Pantallas del CRM.
//
// Los handlers son delgados: validan, delegan en crm_service.go y traducen a
// JSON o plantilla. Toda la regla de negocio vive en el servicio, que es donde
// se puede probar sin levantar un servidor HTTP.

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerCRMRoutes() {
	s.mux.HandleFunc("GET /crm", s.crmHome)
	s.mux.HandleFunc("GET /crm/contact/list", s.crmContactList)
	s.mux.HandleFunc("GET /crm/contact/list/data", s.crmContactListData)
	s.mux.HandleFunc("GET /crm/contact/add", s.crmContactForm)
	s.mux.HandleFunc("GET /crm/contact/view/{id}", s.crmContactView)
	s.mux.HandleFunc("POST /crm/contact/save", s.crmContactSave)
	s.mux.HandleFunc("GET /crm/contact/data/{id}", s.crmContactData)
	s.mux.HandleFunc("GET /crm/pipeline", s.crmPipelineView)
	s.mux.HandleFunc("POST /crm/opportunity/save", s.crmOpportunitySave)
	s.mux.HandleFunc("POST /crm/opportunity/move", s.crmOpportunityMove)
	s.mux.HandleFunc("GET /crm/activity/overdue", s.crmActivityOverdue)
	s.mux.HandleFunc("POST /crm/activity/save", s.crmActivitySave)
	s.mux.HandleFunc("POST /crm/activity/done/{id}", s.crmActivityDone)
	s.mux.HandleFunc("POST /crm/contact/link-customer", s.crmLinkCustomer)
}

// ---------- Panel ----------

func (s *Server) crmHome(w http.ResponseWriter, r *http.Request) {
	pipeline, err := s.crmPipeline(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var contactos, oportunidades int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM crm_contact WHERE status = 1`).Scan(&contactos)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM crm_opportunity WHERE status = 1`).Scan(&oportunidades)

	vencidas, _ := s.crmOverdue(r, 20)
	s.render(w, r, "crm_home.html", map[string]interface{}{
		"Title":         "CRM",
		"Pipeline":      pipeline,
		"Contactos":     contactos,
		"Oportunidades": oportunidades,
		"Vencidas":      vencidas,
	})
}

// ---------- Contactos ----------

func (s *Server) crmContactList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "crm_contact_list.html", map[string]interface{}{
		"Title": "Contactos",
	})
}

func (s *Server) crmContactListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}
	search := strings.TrimSpace(r.FormValue("search[value]"))

	where := ""
	args := []interface{}{}
	if search != "" {
		where = ` WHERE (full_name LIKE ? OR email LIKE ? OR phone LIKE ? OR company_name LIKE ?)`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM crm_contact`).Scan(&total)
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM crm_contact`+where, args...).Scan(&filtered); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	rows, err := s.dbFor(r).Query(`
		SELECT c.id, c.full_name, IFNULL(c.email,''), IFNULL(c.phone,''),
		       IFNULL(c.company_name,''), IFNULL(c.source,''), c.customer_id,
		       (SELECT COUNT(*) FROM crm_opportunity o WHERE o.contact_id = c.id AND o.status = 1),
		       (SELECT COUNT(*) FROM crm_activity a WHERE a.contact_id = c.id)
		FROM crm_contact c`+where+`
		ORDER BY c.full_name LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var (
			id                                       int64
			nombre, email, telefono, empresa, origen string
			customerID                               sql.NullInt64
			opps, acts                               int
		)
		if err := rows.Scan(&id, &nombre, &email, &telefono, &empresa, &origen,
			&customerID, &opps, &acts); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "name": nombre, "email": email, "phone": telefono,
			"company": empresa, "source": origen,
			"opportunities": opps, "activities": acts,
			// Enlazado o no: la interfaz muestra si este contacto ya se puede
			// facturar. Un contacto sin cliente es una venta aún por hacer.
			"is_customer": customerID.Valid,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

func (s *Server) crmContactForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Nuevo Contacto"}
	if id := r.PathValue("id"); id != "" {
		c, err := s.LoadContact(r, parseID(id))
		if err != nil {
			http.Error(w, "Contacto no encontrado", 404)
			return
		}
		data["Title"] = "Editar Contacto"
		data["C"] = c
	}
	s.render(w, r, "crm_contact_form.html", data)
}

func (s *Server) crmContactSave(w http.ResponseWriter, r *http.Request) {
	c := &CRMContact{
		ID:            parseID(r.FormValue("id")),
		FullName:      r.FormValue("full_name"),
		Email:         r.FormValue("email"),
		Phone:         r.FormValue("phone"),
		CompanyName:   r.FormValue("company_name"),
		PositionTitle: r.FormValue("position_title"),
		Source:        r.FormValue("source"),
		Notes:         r.FormValue("notes"),
	}
	c.OwnerID = parseID(r.FormValue("owner_id"))
	c.Status = 1
	if cid := strings.TrimSpace(r.FormValue("customer_id")); cid != "" {
		c.CustomerID = sql.NullInt64{Int64: parseID(cid), Valid: true}
	}

	id, err := s.SaveContact(r, c)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.crmAudit(r, sessUserID(s.sessionFromRequest(r)), "save", "crm_contact", strconv.FormatInt(id, 10),
		nil, map[string]interface{}{"name": c.FullName})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Guardado correctamente", "id": id})
}

// crmContactView es la vista 360° del contacto: historial de interacciones y
// oportunidades, enlazado a sus facturas si ya es cliente.
func (s *Server) crmContactView(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	c, err := s.LoadContact(r, id)
	if err != nil {
		http.Error(w, "Contacto no encontrado", 404)
		return
	}

	rows, err := s.dbFor(r).Query(`
		SELECT o.id, o.title, IFNULL(st.name,''), IFNULL(o.amount,0),
		       IFNULL(CAST(o.expected_close AS CHAR),''), IFNULL(o.closed_at,'')
		FROM crm_opportunity o
		LEFT JOIN crm_stage st ON st.id = o.stage_id
		WHERE o.contact_id = ? AND o.status = 1
		ORDER BY o.updated_at DESC`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type oppRow struct {
		ID                     int64
		Title, Stage, Expected string
		Amount                 float64
		Closed                 string
	}
	opps := []oppRow{}
	for rows.Next() {
		var o oppRow
		if rows.Scan(&o.ID, &o.Title, &o.Stage, &o.Amount, &o.Expected, &o.Closed) == nil {
			opps = append(opps, o)
		}
	}

	arows, err := s.dbFor(r).Query(`
		SELECT a.id, a.activity_type, IFNULL(a.subject,''), IFNULL(a.body,''),
		       IFNULL(CAST(a.due_date AS CHAR),''), a.done, IFNULL(a.created_at,'')
		FROM crm_activity a WHERE a.contact_id = ?
		ORDER BY a.created_at DESC LIMIT 100`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer arows.Close()
	type actRow struct {
		ID                             int64
		Type, Subject, Body, Due, Date string
		Done                           bool
	}
	acts := []actRow{}
	for arows.Next() {
		var a actRow
		var done int
		if arows.Scan(&a.ID, &a.Type, &a.Subject, &a.Body, &a.Due, &done, &a.Date) == nil {
			a.Done = done == 1
			acts = append(acts, a)
		}
	}

	stats, _ := s.crmContactoStats(r, id, 30)
	stages, _ := s.crmStages(r)

	// Si el contacto está enlazado a un cliente, sus facturas entran en la vista
	// 360: es el puente entre la conversación y el dinero.
	var facturas []map[string]interface{}
	var saldo float64
	if c.CustomerID.Valid {
		frows, err := s.dbFor(r).Query(`
			SELECT invoice, date, IFNULL(total_amount,0), IFNULL(due_amount,0)
			FROM invoice WHERE customer_id = ? ORDER BY id DESC LIMIT 25`, c.CustomerID.Int64)
		if err == nil {
			defer frows.Close()
			for frows.Next() {
				var no int64
				var fecha string
				var total, due float64
				if frows.Scan(&no, &fecha, &total, &due) == nil {
					facturas = append(facturas, map[string]interface{}{
						"invoice": no, "date": fecha,
						"total": money(total), "due": money(due),
					})
					saldo += due
				}
			}
		}
	}

	s.render(w, r, "crm_contact_view.html", map[string]interface{}{
		"Title":    c.FullName,
		"C":        c,
		"Opps":     opps,
		"Acts":     acts,
		"Stats":    stats,
		"Stages":   stages,
		"Facturas": facturas,
		"Saldo":    money(saldo),
	})
}

// crmContactData alimenta las tablas del panel lateral (DataTables).
func (s *Server) crmContactData(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	tabla := r.FormValue("table")

	// Solo se permiten tres tablas, y se eligen por un nombre corto en lugar de
	// interpolar el parámetro: es la diferencia entre un filtro y una inyección.
	var (
		query string
		args  []interface{}
	)
	switch tabla {
	case "activities":
		query = `SELECT id, activity_type, IFNULL(subject,''), IFNULL(CAST(due_date AS CHAR),''), done
			FROM crm_activity WHERE contact_id = ? ORDER BY created_at DESC LIMIT 200`
		args = []interface{}{id}
	case "opportunities":
		query = `SELECT id, title, stage_id, IFNULL(amount,0), IFNULL(expected_close,'')
			FROM crm_opportunity WHERE contact_id = ? ORDER BY updated_at DESC LIMIT 200`
		args = []interface{}{id}
	case "invoices":
		query = `SELECT invoice, date, total_amount, due_amount FROM invoice
			WHERE customer_id = (SELECT customer_id FROM crm_contact WHERE id = ?)
			ORDER BY id DESC LIMIT 200`
		args = []interface{}{id}
	default:
		jsonError(w, "tabla no permitida", 400)
		return
	}

	rows, err := s.dbFor(r).Query(query, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	data := []map[string]interface{}{}
	cols, _ := rows.Columns()
	for rows.Next() {
		// Se escanea sobre un slice y no sobre un map porque database/sql
		// necesita direcciones: el índice de un map no es direccionable en Go.
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		row := make(map[string]interface{}, len(cols))
		for i, name := range cols {
			if b, ok := raw[i].([]byte); ok {
				row[name] = string(b)
			} else {
				row[name] = raw[i]
			}
		}
		data = append(data, row)
	}
	writeJSON(w, map[string]interface{}{"draw": draw, "data": data})
}

// ---------- Pipeline ----------

func (s *Server) crmPipelineView(w http.ResponseWriter, r *http.Request) {
	pipeline, err := s.crmPipeline(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	stages, _ := s.crmStages(r)

	// Una consulta por etapa, no una por tarjeta: el número de etapas es de 5 o
	// 6, así que el costo es fijo y predecible.
	cols := make([]map[string]interface{}, 0, len(stages))
	for _, st := range stages {
		rows, err := s.dbFor(r).Query(`
			SELECT o.id, o.title, IFNULL(o.amount,0), o.contact_id,
			       IFNULL(c.full_name,''), IFNULL(c.company_name,''), IFNULL(CAST(o.expected_close AS CHAR),'')
			FROM crm_opportunity o
			LEFT JOIN crm_contact c ON c.id = o.contact_id
			WHERE o.stage_id = ? AND o.status = 1
			ORDER BY o.updated_at DESC`, st.ID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		cards := []map[string]interface{}{}
		for rows.Next() {
			var id, contactID int64
			var title, contact, empresa, cierra string
			var amount float64
			if rows.Scan(&id, &title, &amount, &contactID, &contact, &empresa, &cierra) == nil {
				cards = append(cards, map[string]interface{}{
					"id": id, "title": title, "amount": money(amount),
					"contact_id": contactID, "contact": contact,
					"company": empresa, "expected_close": cierra,
				})
			}
		}
		rows.Close()
		cols = append(cols, map[string]interface{}{"Stage": st, "Cards": cards})
	}

	s.render(w, r, "crm_pipeline.html", map[string]interface{}{
		"Title":    "Embudo comercial",
		"Pipeline": pipeline,
		"Columns":  cols,
	})
}

func (s *Server) crmOpportunitySave(w http.ResponseWriter, r *http.Request) {
	o := &CRMOpportunity{
		ID:            parseID(r.FormValue("id")),
		Title:         r.FormValue("title"),
		ContactID:     parseID(r.FormValue("contact_id")),
		StageID:       parseID(r.FormValue("stage_id")),
		ExpectedClose: strings.TrimSpace(r.FormValue("expected_close")),
		Description:   r.FormValue("description"),
		LostReason:    r.FormValue("lost_reason"),
	}
	o.Amount, _ = strconv.ParseFloat(strings.TrimSpace(r.FormValue("amount")), 64)
	o.OwnerID = parseID(r.FormValue("owner_id"))

	id, err := s.SaveOpportunity(r, o)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.crmAudit(r, sessUserID(s.sessionFromRequest(r)), "save", "crm_opportunity",
		strconv.FormatInt(id, 10), nil, map[string]interface{}{"title": o.Title, "amount": o.Amount})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Oportunidad guardada", "id": id})
}

func (s *Server) crmOpportunityMove(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.FormValue("id"))
	stageID := parseID(r.FormValue("stage_id"))
	if err := s.MoveOpportunity(r, id, stageID, r.FormValue("lost_reason")); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.crmAudit(r, sessUserID(s.sessionFromRequest(r)), "move_stage", "crm_opportunity",
		strconv.FormatInt(id, 10), nil, map[string]interface{}{"stage_id": stageID})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Oportunidad movida", "redirect": saveRedirect(r, "/crm/list")})
}

// ---------- Actividades ----------

func (s *Server) crmActivityOverdue(w http.ResponseWriter, r *http.Request) {
	items, err := s.crmOverdue(r, 50)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"data": items})
}

func (s *Server) crmActivitySave(w http.ResponseWriter, r *http.Request) {
	tipo := strings.TrimSpace(r.FormValue("activity_type"))
	if tipo == "" {
		tipo = "nota"
	}
	var ownerID int64
	if v := sessUserID(s.sessionFromRequest(r)); v != "" {
		ownerID = parseID(v)
	}
	id, err := s.LogActivity(r,
		parseID(r.FormValue("contact_id")),
		parseID(r.FormValue("opportunity_id")),
		tipo,
		strings.TrimSpace(r.FormValue("subject")),
		r.FormValue("body"),
		strings.TrimSpace(r.FormValue("due_date")),
		ownerID,
	)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Actividad registrada", "id": id})
}

func (s *Server) crmActivityDone(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	done := r.FormValue("done") != "0"
	if err := s.CompleteActivity(r, id, done); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1})
}

func (s *Server) crmLinkCustomer(w http.ResponseWriter, r *http.Request) {
	err := s.LinkContactToCustomer(r, parseID(r.FormValue("contact_id")), parseID(r.FormValue("customer_id")))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.crmAudit(r, sessUserID(s.sessionFromRequest(r)), "link_customer", "crm_contact",
		r.FormValue("contact_id"), nil,
		map[string]interface{}{"customer_id": r.FormValue("customer_id")})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Contacto enlazado con el cliente", "redirect": saveRedirect(r, "/crm/list")})
}

// ---------- Utilidades ----------

// parseID convierte un parámetro de ruta o formulario a id.
//
// Ante un valor no numérico devuelve 0, que en todos los servicios significa
// "no hay id". Preferimos un mensaje claro de validación a un error de SQL.
func parseID(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// crmAudit registra un movimiento del CRM en la cadena de auditoría.
func (s *Server) crmAudit(r *http.Request, userID, action, table, recordID string, oldV, newV interface{}) {
	s.appendAudit(r, userID, "", action, "crm", table, recordID, oldV, newV)
}
