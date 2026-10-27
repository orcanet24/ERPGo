package web

// ============================================================================
// ERPGo — Notas de débito fiscales (SENIAT)
// ============================================================================
// Una nota de débito AUMENTA la deuda del cliente (gastos bancarios, cargos
// por mora, diferencias de precio posteriores a la factura, etc.). Es el
// inverso de una nota de crédito: en vez de reducir el saldo, lo incrementa.
// Genera asientos contables automáticos (cargo a Clientes).
// ============================================================================

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerDebitNoteRoutes() {
	s.mux.HandleFunc("GET /debitnote/list", s.debitNoteList)
	s.mux.HandleFunc("GET /debitnote/list/data", s.debitNoteListData)
	s.mux.HandleFunc("GET /debitnote/new", s.debitNoteForm)
	s.mux.HandleFunc("POST /debitnote/save", s.debitNoteSave)
	s.mux.HandleFunc("GET /debitnote/print/{id}", s.debitNotePrint)
}

// debitNoteList muestra el listado.
func (s *Server) debitNoteList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "debitnote_list.html", map[string]interface{}{"Title": "Notas de Débito"})
}

// debitNoteListData — DataTables server-side.
func (s *Server) debitNoteListData(w http.ResponseWriter, r *http.Request) {
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
		where = ` WHERE dn.note_no LIKE ? OR dn.control_no LIKE ? OR c.customer_name LIKE ? OR dn.reason LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM debit_note`).Scan(&total)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM debit_note dn
		LEFT JOIN customer_information c ON c.customer_id = dn.customer_id`+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT dn.id, dn.note_no, dn.control_no, `+s.sqlDateYMD("dn.date")+` AS dndate, dn.total_amount, dn.vat_amount,
			dn.reason, IFNULL(c.customer_name,''), IFNULL(dn.customer_rif,''),
			(SELECT invoice FROM invoice i WHERE i.id = dn.invoice_id LIMIT 1) AS inv_no
		FROM debit_note dn
		LEFT JOIN customer_information c ON c.customer_id = dn.customer_id
		`+where+`
		ORDER BY dn.date DESC, dn.id DESC LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var noteNo int64
		var totalAmt, vatAmt float64
		var ctrl, date, reason, cust, rif sql.NullString
		var invNo sql.NullInt64
		if err := rows.Scan(&id, &noteNo, &ctrl, &date, &totalAmt, &vatAmt, &reason, &cust, &rif, &invNo); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":         id,
			"note_no":    fmt.Sprintf("ND-%08d", noteNo),
			"control_no": ctrl.String,
			"date":       date.String,
			"total":      money(totalAmt),
			"vat":        money(vatAmt),
			"reason":     reason.String,
			"customer":   cust.String,
			"rif":        rif.String,
			"invoice":    invNo.Int64,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// debitNoteForm: formulario para crear una nota de débito (cargo a cliente).
func (s *Server) debitNoteForm(w http.ResponseWriter, r *http.Request) {
	// Clientes para el selector
	rows, err := s.dbFor(r).Query(`
		SELECT c.customer_id, c.customer_name, c.customer_rif,
			IFNULL((SELECT SUM(t.Debit)-SUM(t.Credit) FROM acc_transaction t WHERE t.COAID=b.HeadCode AND t.IsAppove=1),0) AS balance
		FROM customer_information c
		LEFT JOIN acc_coa b ON b.customer_id = c.customer_id
		WHERE c.status = 1
		ORDER BY c.customer_name`)
	type custOpt struct {
		ID      int64
		Name    string
		RIF     string
		Balance float64
	}
	opts := []custOpt{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var o custOpt
			var rif sql.NullString
			if err := rows.Scan(&o.ID, &o.Name, &rif, &o.Balance); err == nil {
				o.RIF = rif.String
				opts = append(opts, o)
			}
		}
	}

	// Facturas recientes para el selector (opcional, ND no requiere factura de origen)
	invRows, err := s.dbFor(r).Query(`
		SELECT i.id, i.invoice, i.date, i.total_amount, IFNULL(c.customer_name,'')
		FROM invoice i LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.is_draft = 'no' AND i.is_anulled = 0
		ORDER BY i.id DESC LIMIT 200`)
	type invOpt struct {
		ID       int64
		Invoice  int64
		Date     string
		Total    string
		Customer string
	}
	invOpts := []invOpt{}
	if err == nil {
		defer invRows.Close()
		for invRows.Next() {
			var o invOpt
			var total float64
			var cust sql.NullString
			if err := invRows.Scan(&o.ID, &o.Invoice, &o.Date, &total, &cust); err == nil {
				o.Total = money(total)
				o.Customer = cust.String
				invOpts = append(invOpts, o)
			}
		}
	}

	s.render(w, r, "debitnote_form.html", map[string]interface{}{
		"Title":     "Nueva Nota de Débito",
		"Customers": opts,
		"Invoices":  invOpts,
	})
}

// debitNoteSave emite la nota: serie fiscal + asientos + cargo a cliente.
func (s *Server) debitNoteSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	customerID, _ := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	reason := strings.TrimSpace(r.FormValue("reason"))
	invoiceID, _ := strconv.ParseInt(r.FormValue("invoice_id"), 10, 64)
	if customerID == 0 || reason == "" {
		jsonError(w, "Cliente y motivo son obligatorios", 400)
		return
	}

	// líneas del desglose
	descs := r.Form["item_desc"]
	qtys := r.Form["item_qty"]
	rates := r.Form["item_rate"]
	vats := r.Form["item_vat"]
	if len(descs) == 0 {
		jsonError(w, "Agregue al menos un renglón a la nota", 400)
		return
	}

	// Cliente (para RIF)
	var custRIF sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT customer_rif FROM customer_information WHERE customer_id = ?`, customerID).Scan(&custRIF)

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Período cerrado: la nota lleva la fecha de HOY, así que se comprueba el mes
	// en curso antes de consumir la serie ND (mismo criterio que la nota de
	// crédito).
	if err := s.exigirPeriodoAbiertoTx(tx, today()); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	// Serie fiscal ND
	doc, err := s.nextFiscalDoc(tx, "ND")
	if err != nil {
		jsonError(w, "No se pudo asignar la serie fiscal: "+err.Error(), 500)
		return
	}

	// Totales: base + IVA de los renglones
	var total, vatTotal float64
	type line struct {
		desc                     string
		qty, rate, vatPct, total float64
	}
	lines := []line{}
	for i := range descs {
		qty := parseMoney(qtys[i])
		rate := parseMoney(rates[i])
		vatPct := 0.0
		if i < len(vats) {
			vatPct = parseMoney(vats[i])
		}
		base := round2(qty * rate)
		vat := round2(base * vatPct / 100.0)
		total += base
		vatTotal += vat
		lines = append(lines, line{desc: strings.TrimSpace(descs[i]), qty: qty, rate: rate, vatPct: vatPct, total: base})
	}
	grand := round2(total + vatTotal)

	// Insertar nota
	res, err := tx.Exec(`INSERT INTO debit_note
		(note_no, control_no, series, invoice_id, customer_id, customer_rif, date,
		 total_amount, vat_amount, reason, doc_type, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		doc.Number, doc.ControlNo, "ND", invoiceID, customerID, custRIF, today(),
		grand, vatTotal, reason, "02", createBy, now())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	dnID, _ := res.LastInsertId()

	for _, ln := range lines {
		if _, err := tx.Exec(`INSERT INTO debit_note_details
			(debit_note_id, description, quantity, rate, vat_percent, vat_amount, total_price)
			VALUES (?,?,?,?,?,?,?)`,
			dnID, ln.desc, ln.qty, ln.rate, ln.vatPct, round2(ln.total*ln.vatPct/100.0), ln.total); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// ---- Asientos contables automáticos ----
	// Una nota de débito AUMENTA la deuda del cliente:
	//   Debit: cuenta del gasto/ingreso (según motivo) o directamente Clientes
	//   Credit: Clientes (aumenta el saldo por cobrar)
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	ref := fmt.Sprintf("ND-%08d", doc.Number)
	vno := nextVoucherNoTx(tx, "JV")
	_ = s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: ref,
		COAID: pre.CustomerCode, RevCodde: pre.SalesCode,
		Narration: "Nota de débito " + ref, Comment: reason,
		Debit: grand, SubType: 3, SubCode: s.subcodeID(r, customerID, 3),
	}, createBy)

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Aprobar voucher si autoaprobado
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, createBy)
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Nota de débito emitida",
		"note_no": doc.DocNo, "control_no": doc.ControlNo, "id": dnID,
	})
}

// debitNotePrint — versión imprimible de la nota (formato fiscal).
func (s *Server) debitNotePrint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var (
		noteNo                              int64
		total, vat                          float64
		ctrl, date, reason, series, custRIF sql.NullString
		custID, invoiceID                   int64
	)
	err := s.dbFor(r).QueryRow(`SELECT note_no, control_no, series, date, total_amount, vat_amount,
		reason, customer_id, customer_rif, invoice_id
		FROM debit_note WHERE id = ?`, id).
		Scan(&noteNo, &ctrl, &series, &date, &total, &vat, &reason, &custID, &custRIF, &invoiceID)
	if err != nil {
		http.Error(w, "Nota no encontrada", 404)
		return
	}
	var custName string
	_ = s.dbFor(r).QueryRow(`SELECT customer_name FROM customer_information WHERE customer_id = ?`, custID).Scan(&custName)
	var invNo int64
	if invoiceID > 0 {
		_ = s.dbFor(r).QueryRow(`SELECT invoice FROM invoice WHERE id = ?`, invoiceID).Scan(&invNo)
	}

	st := s.settingsFor(r)
	fs := s.loadFiscalSettings(r)
	s.renderStandalone(r, w, "debitnote_print.html", map[string]interface{}{
		"Title":    "Nota de Débito",
		"No":       fmt.Sprintf("ND-%08d", noteNo),
		"Control":  ctrl.String,
		"Series":   series.String,
		"Date":     date.String,
		"Total":    money(total),
		"VAT":      money(vat),
		"Reason":   reason.String,
		"Customer": custName,
		"RIF":      custRIF.String,
		"Invoice":  invNo,
		"Company":  st.Company,
		"Footnote": fs.FiscalFootnote,
	})
}
