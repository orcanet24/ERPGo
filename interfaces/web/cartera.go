package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ============================================================================
// ERPGo — Cartera / Cuentas por Cobrar
// ============================================================================
// Módulo de cobros: registra pagos de clientes, actualiza facturas
// (paid_amount/due_amount), genera asientos contables (débito caja/banco,
// crédito cliente) y emite estado de cuenta por cliente.
// ============================================================================

func (s *Server) registerCarteraRoutes() {
	s.mux.HandleFunc("GET /cartera/list", s.carteraList)
	s.mux.HandleFunc("/cartera/list/data", s.carteraListData)
	s.mux.HandleFunc("POST /cartera/save", s.carteraSave)
	s.mux.HandleFunc("POST /cartera/delete/{id}", s.carteraDelete)
	s.mux.HandleFunc("POST /cartera/collect", s.carteraCollect)
	s.mux.HandleFunc("GET /cartera/statement/{customer_id}", s.carteraStatement)
	// El saldo por cliente, el reparto de un cobro entre varias facturas y el
	// listado de facturas abiertas de un cliente. Es lo que convierte la lista de
	// cobros en un módulo de cartera: ver cuánto debe cada quién y en qué se fue
	// cada pago.
	s.mux.HandleFunc("GET /cartera/por-cobrar", s.carteraPorCobrar)
	s.mux.HandleFunc("GET /cartera/facturas/{customer_id}", s.carteraFacturasCliente)
	s.mux.HandleFunc("POST /cartera/cobrar", s.carteraCobrar)
}

// carteraList renderiza la página de cuentas por cobrar.
func (s *Server) carteraList(w http.ResponseWriter, r *http.Request) {
	// La cartera maneja plata que se espera cobrar y crea cobros con su asiento: se
	// entra con el permiso del libro de clientes, igual que el resto del sistema.
	if !s.exigirPermiso(w, r, "read", "customer_ledger") {
		return
	}
	s.render(w, r, "cartera_list.html", map[string]interface{}{
		"Title":         "Cuentas por Cobrar",
		"Module":        "cartera",
		"Customers":     s.pbListaClientes(r),
		"Methods":       s.pbListaMetodos(r),
		"PuedeCobrar":   s.hasPermission(r, "customer_ledger", "create"),
		"PuedeBorrar":   s.hasPermission(r, "customer_ledger", "delete"),
		"PuedeClientes": s.hasPermission(r, "add_customer", "create"),
		// El aviso al cliente sale con el mismo permiso que registrar un cobro: quien
		// puede cobrar puede reclamar. La pantalla recibe de los canales SOLO id,
		// nombre y tipo; las credenciales nunca viajan al navegador.
		"PuedeAvisar": s.hasPermission(r, "customer_ledger", "create"),
		"Canales":     s.notifyOpciones(s.dbFor(r)),
	})
}

// carteraListData: DataTables server-side con JOIN a customer_information e invoice.
func (s *Server) carteraListData(w http.ResponseWriter, r *http.Request) {
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
		where = ` WHERE dc.reference LIKE ? OR dc.notes LIKE ? OR c.customer_name LIKE ? OR dc.invoice_id LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM debt_collection dc`).Scan(&total)
	_ = s.dbFor(r).QueryRow(`
		SELECT COUNT(*) FROM debt_collection dc
		LEFT JOIN customer_information c ON c.customer_id = dc.customer_id
		LEFT JOIN invoice i ON i.invoice_id = dc.invoice_id`+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT dc.id, dc.customer_id, IFNULL(c.customer_name,''),
		       dc.invoice_id, dc.amount, dc.payment_method_id,
		       IFNULL(pm.name,''), dc.collection_date,
		       IFNULL(dc.reference,''), IFNULL(dc.notes,''),
		       dc.created_by, dc.created_at
		FROM debt_collection dc
		LEFT JOIN customer_information c ON c.customer_id = dc.customer_id
		LEFT JOIN payment_method pm ON pm.id = dc.payment_method_id
		`+where+`
		ORDER BY dc.id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id, custID, invID, pmID int64
		var cname, pmName, ref, notes, createdBy, createdAt sql.NullString
		var amount float64
		var colDate sql.NullString
		if err := rows.Scan(&id, &custID, &cname, &invID, &amount, &pmID, &pmName, &colDate, &ref, &notes, &createdBy, &createdAt); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "customer_id": custID, "customer": cname.String,
			"invoice_id": invID, "amount": money(amount),
			"payment_method_id": pmID, "payment_method": pmName.String,
			"collection_date": colDate.String, "reference": ref.String,
			"notes": notes.String, "created_by": createdBy.String,
			"created_at": createdAt.String,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// carteraSave: registra un cobro desde el formulario, actualiza factura,
// genera asiento contable y audita.
func (s *Server) carteraSave(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	customerID, _ := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	invoiceID, _ := strconv.ParseInt(r.FormValue("invoice_id"), 10, 64)
	pmID, _ := strconv.ParseInt(r.FormValue("payment_method_id"), 10, 64)
	amount := parseMoney(r.FormValue("amount"))
	ref := strings.TrimSpace(r.FormValue("reference"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	date := orDefault(r.FormValue("collection_date"), today())

	if customerID == 0 || amount <= 0 {
		jsonError(w, "Cliente y monto son obligatorios", 400)
		return
	}

	if err := s.processCollection(w, r, customerID, invoiceID, pmID, amount, ref, notes, date); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
}

// carteraCollect: endpoint directo de cobro (recibe payment_method_id, amount,
// reference, notes, customer_id, invoice_id). Misma lógica que save.
func (s *Server) carteraCollect(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	customerID, _ := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	invoiceID, _ := strconv.ParseInt(r.FormValue("invoice_id"), 10, 64)
	pmID, _ := strconv.ParseInt(r.FormValue("payment_method_id"), 10, 64)
	amount := parseMoney(r.FormValue("amount"))
	ref := strings.TrimSpace(r.FormValue("reference"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	date := orDefault(r.FormValue("collection_date"), today())

	if customerID == 0 || amount <= 0 {
		jsonError(w, "Cliente y monto son obligatorios", 400)
		return
	}

	if err := s.processCollection(w, r, customerID, invoiceID, pmID, amount, ref, notes, date); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
}

// processCollection: lógica transaccional de cobro. Registra debt_collection,
// actualiza invoice, genera asiento contable y audita.
func (s *Server) processCollection(w http.ResponseWriter, r *http.Request,
	customerID, invoiceID, pmID int64, amount float64, ref, notes, date string) error {

	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)
	ip := clientIP(r)

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1) Crear tabla si no existe (idempotente)
	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS debt_collection (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		customer_id INT NOT NULL,
		invoice_id INT DEFAULT 0,
		amount DECIMAL(12,2) NOT NULL DEFAULT 0,
		payment_method_id INT DEFAULT 0,
		collection_date DATE,
		reference VARCHAR(255),
		notes TEXT,
		created_by VARCHAR(50),
		created_at DATETIME,
		INDEX idx_customer (customer_id),
		INDEX idx_invoice (invoice_id),
		INDEX idx_date (collection_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`)
	if err != nil {
		return fmt.Errorf("debt_collection DDL: %w", err)
	}

	// 2) Obtener cuenta contable del método de pago
	var coaID sql.NullInt64
	if pmID > 0 {
		_ = tx.QueryRow(`SELECT coa_id FROM payment_method WHERE id = ?`, pmID).Scan(&coaID)
	}

	pre, err := s.predefineAccount(r)
	if err != nil {
		return fmt.Errorf("predefine: %w", err)
	}

	// Si el método de pago no tiene COA, usar Caja por defecto
	paymentCOA := pre.CashCode
	if coaID.Valid && coaID.Int64 > 0 {
		paymentCOA = coaID.Int64
	}

	// 3) Registrar cobro en debt_collection con multi-monedia
	currencyCode := orDefault(r.FormValue("currency_code"), "VES")
	exchangeRate := 1.0
	amountUSD := 0.0
	if currencyCode == "USD" {
		amountUSD = amount
		tasaVal, _, err := s.ObtenerTasaHoy(tx, date, "USD")
		if err == nil && tasaVal > 0 {
			exchangeRate = tasaVal
		}
	} else {
		// Cobro en VES, calcular equivalente USD
		tasaVal, _, err := s.ObtenerTasaHoy(tx, date, "USD")
		if err == nil && tasaVal > 0 {
			exchangeRate = tasaVal
			amountUSD = round2(amount / tasaVal)
		}
	}

	res, err := tx.Exec(`
		INSERT INTO debt_collection (customer_id, invoice_id, amount, amount_usd, exchange_rate, currency_code,
			payment_method_id, collection_date, reference, notes, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		customerID, invoiceID, amount, amountUSD, exchangeRate, currencyCode,
		pmID, date, ref, notes, createBy, now())
	if err != nil {
		return fmt.Errorf("insert cobro: %w", err)
	}
	collectionID, _ := res.LastInsertId()

	// 4) Actualizar factura si hay invoice_id
	if invoiceID > 0 {
		var invInternalID int64
		var currentPaid, currentDue, totalAmount float64
		err := tx.QueryRow(`
			SELECT id, paid_amount, due_amount, total_amount
			FROM invoice WHERE invoice_id = ?`, invoiceID).Scan(&invInternalID, &currentPaid, &currentDue, &totalAmount)
		if err == nil {
			newPaid := round2(currentPaid + amount)
			newDue := round2(currentDue - amount)
			if newDue < 0 {
				newDue = 0
			}
			// Defensivo: no cobrar más del total
			if newPaid > totalAmount {
				newPaid = totalAmount
				newDue = 0
			}
			_, err = tx.Exec(`UPDATE invoice SET paid_amount = ?, due_amount = ? WHERE id = ?`,
				newPaid, newDue, invInternalID)
			if err != nil {
				return fmt.Errorf("update invoice: %w", err)
			}
		}
	}

	// 5) Generar asiento contable (acc_vaucher): Débito caja/banco, Crédito cliente.
	// El motor (approveVoucher) genera automáticamente la partida doble en acc_transaction:
	//   Fila 1: paymentCOA Debit amount (entra caja/banco)
	//   Reversa: CustomerCode Credit amount (cliente deja de deber)
	sub := s.subcodeID(r, customerID, 3)
	vno := nextVoucherNoTx(tx, "CV")
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "CV", ReferenceNo: fmt.Sprintf("COL-%d", collectionID),
		VDate: date, COAID: paymentCOA, RevCodde: pre.CustomerCode,
		Narration: "Cobro a cliente " + strconv.FormatInt(customerID, 10),
		Comment:   ref, Debit: amount, SubType: 3, SubCode: sub,
	}, createBy); err != nil {
		return fmt.Errorf("voucher: %w", err)
	}

	// 6) Commit
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// 7) Autoaprobar si está configurado
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, createBy)
	}

	// 8) Auditoría
	oldVal := map[string]interface{}{"invoice_id": invoiceID, "paid_before": 0, "due_before": 0}
	newVal := map[string]interface{}{
		"collection_id": collectionID, "amount": amount,
		"payment_method_id": pmID, "voucher": vno,
	}
	s.appendAudit(r, createBy, ip, "create", "Cartera", "debt_collection",
		fmt.Sprintf("%d", collectionID), oldVal, newVal)

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Cobro registrado",
		"collection_id": collectionID, "voucher": vno,
	})
	return nil
}

// carteraDelete: elimina un registro de cobro y reversa el asiento.
func (s *Server) carteraDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)
	ip := clientIP(r)

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Obtener datos del cobro antes de eliminar
	var customerID, invoiceID, pmID int64
	var amount float64
	var ref, vno sql.NullString
	err = tx.QueryRow(`
		SELECT customer_id, invoice_id, amount, payment_method_id,
		       reference, IFNULL('','') FROM debt_collection WHERE id = ?`, id).
		Scan(&customerID, &invoiceID, &amount, &pmID, &ref, &vno)
	if err != nil {
		jsonError(w, "Cobro no encontrado", 404)
		return
	}

	// Revertir factura
	if invoiceID > 0 {
		var invInternalID int64
		var currentPaid, currentDue float64
		err := tx.QueryRow(`SELECT id, paid_amount, due_amount FROM invoice WHERE invoice_id = ?`, invoiceID).
			Scan(&invInternalID, &currentPaid, &currentDue)
		if err == nil {
			newPaid := round2(currentPaid - amount)
			newDue := round2(currentDue + amount)
			if newPaid < 0 {
				newPaid = 0
			}
			if _, err := tx.Exec(`UPDATE invoice SET paid_amount = ?, due_amount = ? WHERE id = ?`,
				newPaid, newDue, invInternalID); err != nil {
				_ = tx.Rollback()
				jsonError(w, fmt.Sprintf("revertir factura: %v", err), 500)
				return
			}
		}
	}

	// Eliminar cobro
	_, err = tx.Exec(`DELETE FROM debt_collection WHERE id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Auditoría
	s.appendAudit(r, createBy, ip, "delete", "Cartera", "debt_collection", id,
		map[string]interface{}{"amount": amount, "customer_id": customerID, "invoice_id": invoiceID}, nil)

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cobro eliminado", "redirect": saveRedirect(r, "/cartera/list")})
}

// carteraStatement: estado de cuenta de un cliente (facturas, pagos, saldo).
func (s *Server) carteraStatement(w http.ResponseWriter, r *http.Request) {
	customerID := r.PathValue("customer_id")
	if customerID == "" {
		jsonError(w, "customer_id requerido", 400)
		return
	}

	cid, _ := strconv.ParseInt(customerID, 10, 64)
	if cid == 0 {
		jsonError(w, "customer_id inválido", 400)
		return
	}

	// Datos del cliente
	var cname, cmobile, caddr sql.NullString
	_ = s.dbFor(r).QueryRow(`
		SELECT customer_name, IFNULL(customer_mobile,''), IFNULL(customer_address,'')
		FROM customer_information WHERE customer_id = ?`, cid).Scan(&cname, &cmobile, &caddr)

	// Facturas del cliente
	invRows, err := s.dbFor(r).Query(`
		SELECT invoice_id, invoice, date, total_amount, paid_amount, due_amount, status
		FROM invoice WHERE customer_id = ?
		ORDER BY date DESC, id DESC LIMIT 500`, cid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer invRows.Close()

	type invLine struct {
		InvoiceID, InvoiceNo int64
		Date                 string
		Total, Paid, Due     float64
		Status               int
	}
	var invoices []invLine
	var sumTotal, sumPaid, sumDue float64
	for invRows.Next() {
		var l invLine
		var dt sql.NullString
		if err := invRows.Scan(&l.InvoiceID, &l.InvoiceNo, &dt, &l.Total, &l.Paid, &l.Due, &l.Status); err != nil {
			continue
		}
		l.Date = dt.String
		invoices = append(invoices, l)
		sumTotal += l.Total
		sumPaid += l.Paid
		sumDue += l.Due
	}

	// Pagos del cliente
	payRows, err := s.dbFor(r).Query(`
		SELECT dc.id, dc.invoice_id, dc.amount, IFNULL(pm.name,''),
		       dc.collection_date, IFNULL(dc.reference,''), IFNULL(dc.notes,'')
		FROM debt_collection dc
		LEFT JOIN payment_method pm ON pm.id = dc.payment_method_id
		WHERE dc.customer_id = ?
		ORDER BY dc.collection_date DESC, dc.id DESC LIMIT 500`, cid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer payRows.Close()

	type payLine struct {
		ID                int64
		InvoiceID, PMName string
		Amount            float64
		Date, Ref, Notes  string
	}
	var payments []payLine
	var sumPayments float64
	for payRows.Next() {
		var p payLine
		var dt, ref, notes sql.NullString
		var pmName sql.NullString
		var invID int64
		if err := payRows.Scan(&p.ID, &invID, &p.Amount, &pmName, &dt, &ref, &notes); err != nil {
			continue
		}
		p.InvoiceID = strconv.FormatInt(invID, 10)
		p.PMName = pmName.String
		p.Date = dt.String
		p.Ref = ref.String
		p.Notes = notes.String
		payments = append(payments, p)
		sumPayments += p.Amount
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"customer": map[string]interface{}{
			"id": cid, "name": cname.String, "mobile": cmobile.String, "address": caddr.String,
		},
		"invoices": invoices,
		"payments": payments,
		"summary": map[string]interface{}{
			"total_invoiced":  money(sumTotal),
			"total_paid_inv":  money(sumPaid),
			"total_due":       money(sumDue),
			"total_collected": money(sumPayments),
			"balance":         money(sumTotal - sumPayments),
		},
	})
}
