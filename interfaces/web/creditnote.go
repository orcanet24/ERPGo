package web

// ============================================================================
// ERPGo — Notas de crédito fiscales (SENIAT)
// ============================================================================
// Las facturas NUNCA se borran ni se editan retroactivamente: toda
// corrección se hace con una Nota de Crédito fiscal que referencia la
// factura afectada (requisito del libro de ventas, columna "doc afectado").
// Genera asientos contables automáticos (patrón del sistema).
// ============================================================================

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerCreditNoteRoutes() {
	s.mux.HandleFunc("GET /creditnote/list", s.creditNoteList)
	s.mux.HandleFunc("GET /creditnote/list/data", s.creditNoteListData)
	s.mux.HandleFunc("GET /creditnote/new", s.creditNoteForm)
	s.mux.HandleFunc("POST /creditnote/save", s.creditNoteSave)
	s.mux.HandleFunc("GET /creditnote/print/{id}", s.creditNotePrint)
}

// creditNoteList muestra el listado.
func (s *Server) creditNoteList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "creditnote_list.html", map[string]interface{}{"Title": "Notas de Crédito"})
}

// creditNoteListData — DataTables server-side.
func (s *Server) creditNoteListData(w http.ResponseWriter, r *http.Request) {
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
		where = ` WHERE cn.note_no LIKE ? OR cn.control_no LIKE ? OR c.customer_name LIKE ? OR cn.reason LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM credit_note`).Scan(&total)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM credit_note cn
		LEFT JOIN customer_information c ON c.customer_id = cn.customer_id`+where, args...).Scan(&filtered)

	// Negocio: nota_no formateado; facturas mostradas por número de negocio

	rows, err := s.dbFor(r).Query(`
		SELECT cn.id, cn.note_no, cn.control_no, `+s.sqlDateYMD("cn.date")+` AS cndate, cn.total_amount, cn.vat_amount,
			cn.reason, IFNULL(c.customer_name,''), IFNULL(cn.customer_rif,''),
			(SELECT invoice FROM invoice i WHERE i.id = cn.invoice_id LIMIT 1) AS inv_no
		FROM credit_note cn
		LEFT JOIN customer_information c ON c.customer_id = cn.customer_id
		`+where+`
		ORDER BY cn.date DESC, cn.id DESC LIMIT ? OFFSET ?`, append(args, length, start)...)
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
			"note_no":    fmt.Sprintf("NC-%08d", noteNo),
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

// creditNoteForm: formulario con selector de factura y desglose.
func (s *Server) creditNoteForm(w http.ResponseWriter, r *http.Request) {
	invoiceIDStr := r.URL.Query().Get("invoice_id")
	var opts []invOpt
	var linesJSON []byte = nil
	var custName, custRIF, invTotal string

	if invoiceIDStr != "" {
		// Cargar detalles de la factura seleccionada para pre-popular el formulario
		invID, err := strconv.ParseInt(invoiceIDStr, 10, 64)
		if err == nil {
			rows, err := s.dbFor(r).Query(`
			SELECT pi.product_id, pi.product_name, COALESCE(pi.price,0), pi.tax
			FROM invoice_details id
			JOIN product_information pi ON pi.product_id = id.product_id
			WHERE id.invoice_id = ?`, invID).
			if err == nil {
				var desc, pid string
				var price, taxRate float64
				for rows.Next() {
					var id sql.NullString
					_ = rows.Scan(&pid, &desc, &price, &taxRate)
					linesJSON = append(linesJSON, fmt.Sprintf(`<tr data-id="%s" data-desc="%s" data-price="%f" data-tax="%f"><td>%s</td><td><input type="text" class="form-control cn-qty" value="1"></td><td><input type="text" class="form-control text-right cn-rate" value="%f"></td><td><input type="text" class="form-control text-right cn-vat" value="%f"></td><td class="text-right cn-total" style="font-size:12px">0.00</td></tr>`, pid, desc, price, price, taxRate, taxRate))
				}
			}
		}
		// También obtener datos de la factura para el info box
		var inv struct {
			Invoice int64
			Date    string
			Total   float64
			Customer string
			RIF      string
		}
		err = s.dbFor(r).QueryRow(`SELECT invoice_id, date, total_amount, customer_name, customer_rif FROM invoice WHERE id = ?`, invID).
			Scan(&inv.Invoice, &inv.Date, &inv.Total, &custName, &custRIF)
		}
	}

	// Facturas recientes para el selector (vivas, no anuladas)
	rows, err := s.dbFor(r).Query(`
		SELECT i.id, i.invoice, i.date, i.total_amount, IFNULL(c.customer_name,''), IFNULL(i.customer_rif,'')
		FROM invoice i LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.is_draft = 'no' AND i.is_anulled = 0
		ORDER BY i.id DESC LIMIT 200`)
	type invOpt struct {
		ID       int64
		Invoice  int64
		Date     string
		Total    string
		Customer string
		RIF      string
	}
	opts := []invOpt{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var o invOpt
			var total float64
			var cust, rif sql.NullString
			_ = rows.Scan(&o.ID, &o.Invoice, &o.Date, &total, &cust, &rif)
			o.Total = money(total)
			o.Customer = cust.String
			o.RIF = rif.String
			opts = append(opts, o)
		}
	}
	s.render(w, r, "creditnote_form.html", map[string]interface{}{
		"Title":       "Nueva Nota de Crédito",
		"Invoices":    opts,
		"LinesJSON":   linesJSON,
		"CustName":    custName,
		"CustRIF":     custRIF,
		"InvTotal":    invTotal,
	})
	// Set LinesJSON as data attribute in form for JavaScript access
	formHTML := s.render(w, r, "creditnote_form_html_partial", map[string]interface{}{
		"LinesJSON": linesJSON,
	})}
	// Return just the form partial with LinesJSON as data attribute if invoice_id was provided
	if invoiceIDStr != "" {
		// Render partial and output only the form with data attribute
		partialOpts := map[string]interface{}{
			"LinesJSON": linesJSON,
			"Title":     "Nueva Nota de Crédito",
		}
		partialHTML := s.render(w, r, "creditnote_form.html", partialOpts)
		// Extract the form element with data attribute
		// We need to add data-linesjson attribute to the form
		formStart := strings.Index(partialHTML, "<form id=\"cnForm\"")
		if formStart >= 0 {
			attrEnd := strings.Index(partialHTML[formStart:], ">")
			if attrEnd >= 0 {
				partialHTML = partialHTML[:formStart+attrEnd+1] + " data-linesjson=\"" + string(linesJSON) + "\"" + partialHTML[formStart+attrEnd+1:]
			}
		}
		w.Write([]byte(partialHTML))
		return
	}
}

// creditNoteSave emite la nota: serie fiscal + asientos + reversión de stock.
func (s *Server) creditNoteSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	invoiceID, _ := strconv.ParseInt(r.FormValue("invoice_id"), 10, 64)
	reason := strings.TrimSpace(r.FormValue("reason"))
	if invoiceID == 0 || reason == "" {
		jsonError(w, "Factura y motivo son obligatorios", 400)
		return
	}

	// líneas del desglose
	ids := r.Form["item_id"]
	qtys := r.Form["item_qty"]
	rates := r.Form["item_rate"]
	vats := r.Form["item_vat"]
	if len(ids) == 0 {
		jsonError(w, "Agregue al menos un renglón a la nota", 400)
		return
	}

	fs := s.loadFiscalSettings(r)
	_ = fs

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Factura origen, con BLOQUEO de fila (FOR UPDATE).
	//
	// El bloqueo no es decorativo: el tope acumulado de la nota se calcula
	// contra las notas ya emitidas para esta factura, y dos notas simultáneas
	// que leyeran el acumulado sin bloquear podrían acreditar juntas más que lo
	// facturado. El bloqueo de la fila de origen las ordena: la segunda espera y
	// lee lo que la primera dejó escrito.
	var (
		customerID   int64
		custRIF      sql.NullString
		invoiceTotal float64
		invoiceDraft sql.NullString
	)
	err = tx.QueryRow(`SELECT customer_id, customer_rif, IFNULL(total_amount,0), IFNULL(is_draft,'no')
		FROM invoice WHERE id = ? FOR UPDATE`, invoiceID).
		Scan(&customerID, &custRIF, &invoiceTotal, &invoiceDraft)
	if err == sql.ErrNoRows {
		jsonError(w, "Factura no encontrada", 404)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if strings.EqualFold(strings.TrimSpace(invoiceDraft.String), "yes") {
		jsonError(w, "La factura es un borrador: no se acredita lo que nunca se emitio", http.StatusConflict)
		return
	}

	// Período cerrado: la nota se emite con la fecha de HOY —es el documento que
	// corrige ahora, no una edición del pasado—, así que se comprueba el mes en
	// curso antes de consumir la serie NC. La corrección de una factura de un mes
	// cerrado se hace en el mes abierto, que es lo que deja el rastro coherente.
	if err := s.exigirPeriodoAbiertoTx(tx, today()); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	// Serie fiscal NC
	doc, err := s.nextFiscalDoc(tx, "NC")
	if err != nil {
		jsonError(w, "No se pudo asignar la serie fiscal: "+err.Error(), 500)
		return
	}

	// Totales: base + IVA de los renglones
	var total, vatTotal float64
	type line struct {
		productID                sql.NullString
		desc                     string
		qty, rate, vatPct, total float64
	}
	lines := []line{}
	for i, idv := range ids {
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
		lines = append(lines, line{desc: "", qty: qty, rate: rate, vatPct: vatPct, total: base})
		_ = idv
	}
	grand := round2(total + vatTotal)

	// Tope acumulado contra el documento origen.
	//
	// Una nota de crédito no es un documento suelto: es la corrección de UNA
	// factura, y su razón de ser es que lo acreditado no supere lo facturado.
	// Sin este tope se podía acreditar de más —se pedía una nota por el total
	// de la factura varias veces— y la factura quedaba con saldo negativo:
	// contabilidad que no cuadra y un libro de ventas que no se sostiene ante
	// una revisión. El acumulado se lee DENTRO de la transacción y con la fila
	// de origen bloqueada, así que la comprobación no puede colarse por una
	// carrera entre dos notas.
	var yaAcreditado float64
	if err := tx.QueryRow(`SELECT IFNULL(SUM(total_amount),0) FROM credit_note WHERE invoice_id = ?`, invoiceID).
		Scan(&yaAcreditado); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if disponible := round2(invoiceTotal - yaAcreditado); round2(disponible-grand) < -0.005 {
		jsonError(w, fmt.Sprintf("La nota excede lo facturado: de %.2f facturados quedan %.2f por acreditar",
			invoiceTotal, disponible), http.StatusConflict)
		return
	}

	// Insertar nota
	res, err := tx.Exec(`INSERT INTO credit_note
		(note_no, control_no, series, invoice_id, customer_id, customer_rif, date,
		 total_amount, vat_amount, reason, doc_type, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		doc.Number, doc.ControlNo, "NC", invoiceID, customerID, custRIF, today(),
		grand, vatTotal, reason, "03", createBy, now())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	cnID, _ := res.LastInsertId()

	for _, ln := range lines {
		if _, err := tx.Exec(`INSERT INTO credit_note_details
			(credit_note_id, product_id, description, quantity, rate, vat_percent, vat_amount, total_price)
			VALUES (?,?,?,?,?,?,?,?)`,
			cnID, ln.productID, ln.desc, ln.qty, ln.rate, ln.vatPct, round2(ln.total*ln.vatPct/100.0), ln.total); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// ---- Asientos contables automáticos ----
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	ref := fmt.Sprintf("NC-%08d", doc.Number)
	var vno2 string
	// 1) Reversión de ingreso: Debit Sales, Credit Customer por el total
	vno := nextVoucherNoTx(tx, "JV")
	_ = s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: ref,
		COAID: pre.SalesCode, RevCodde: pre.CustomerCode,
		Narration: "Nota de crédito " + ref, Comment: reason,
		Debit: grand, SubType: 3, SubCode: s.subcodeID(r, customerID, 3),
	}, createBy)
	// 2) Reversión de IVA: Debit Tax (provisión), Credit Customer
	if vatTotal > 0 {
		vno2 = nextVoucherNoTx(tx, "JV")
		_ = s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno2, Vtype: "JV", ReferenceNo: ref,
			COAID: pre.TaxCode, RevCodde: pre.CustomerCode,
			Narration: "IVA nota de crédito " + ref, Comment: reason,
			Debit: vatTotal, SubType: 0,
		}, createBy)
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Aprobar vouchers si autoaprobado
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, createBy)
		if vatTotal > 0 {
			_ = s.approveVoucher(r, vno2, createBy)
		}
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Nota de crédito emitida",
		"note_no": doc.DocNo, "control_no": doc.ControlNo, "id": cnID,
	})
}

// creditNotePrint — versión imprimible de la nota (formato fiscal).
func (s *Server) creditNotePrint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var (
		noteNo                              int64
		total, vat                          float64
		ctrl, date, reason, series, custRIF sql.NullString
		custID, invoiceID                   int64
	)
	err := s.dbFor(r).QueryRow(`SELECT note_no, control_no, series, date, total_amount, vat_amount,
		reason, customer_id, customer_rif, invoice_id
		FROM credit_note WHERE id = ?`, id).
		Scan(&noteNo, &ctrl, &series, &date, &total, &vat, &reason, &custID, &custRIF, &invoiceID)
	if err != nil {
		http.Error(w, "Nota no encontrada", 404)
		return
	}
	var custName string
	_ = s.dbFor(r).QueryRow(`SELECT customer_name FROM customer_information WHERE customer_id = ?`, custID).Scan(&custName)
	var invNo int64
	_ = s.dbFor(r).QueryRow(`SELECT invoice FROM invoice WHERE id = ?`, invoiceID).Scan(&invNo)

	st := s.settingsFor(r)
	fs := s.loadFiscalSettings(r)
	s.renderStandalone(r, w, "creditnote_print.html", map[string]interface{}{
		"Title":    "Nota de Crédito",
		"No":       fmt.Sprintf("NC-%08d", noteNo),
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
