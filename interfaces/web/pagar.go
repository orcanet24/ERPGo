package web

// pagar.go — Cuentas por Pagar (Proveedores)

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerPagarRoutes() {
	s.mux.HandleFunc("GET /pagar/list", s.pagarList)
	s.mux.HandleFunc("/pagar/list/data", s.pagarListData)
	s.mux.HandleFunc("POST /pagar/save", s.pagarSave)
	s.mux.HandleFunc("POST /pagar/delete/{id}", s.pagarDelete)
	s.mux.HandleFunc("POST /pagar/pay", s.pagarPay)
	s.mux.HandleFunc("GET /pagar/statement/{supplier_id}", s.pagarStatement)
	// Lo que se debe y no viene de una factura de mercancía: agua, luz, teléfono,
	// internet, arriendo. Y el pago de una compra a crédito, que es la otra mitad
	// del módulo: registrar la deuda sin poder pagarla no sirve de nada.
	s.mux.HandleFunc("POST /pagar/servicio/save", s.pagarServicioSave)
	s.mux.HandleFunc("POST /pagar/compra/pay", s.pagarCompraPay)
	s.mux.HandleFunc("GET /pagar/por-pagar", s.pagarPorPagar)
}

func (s *Server) pagarList(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_payment") {
		return
	}
	s.render(w, r, "pagar_list.html", map[string]interface{}{
		"Title":       "Cuentas por Pagar",
		"Module":      "pagar",
		"Suppliers":   s.pbListaProveedores(r),
		"Methods":     s.pbListaMetodos(r),
		"PuedePagar":  s.hasPermission(r, "supplier_payment", "update"),
		"PuedeBorrar": s.hasPermission(r, "supplier_payment", "delete"),
		// El aviso al proveedor —su comprobante de retención, o el aviso de pago— va
		// con el mismo permiso que registrar un pago.
		"PuedeAvisar": s.hasPermission(r, "supplier_payment", "create"),
		"Canales":     s.notifyOpciones(s.dbFor(r)),
	})
}

func (s *Server) pagarListData(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_payment") {
		return
	}
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
		where = ` WHERE ap.reference LIKE ? OR ap.notes LIKE ? OR s.supplier_name LIKE ? OR ap.purchase_id LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	// El contar lleva el MISMO JOIN que la consulta: el filtro de búsqueda nombra
	// s.supplier_name, y sin el join la búsqueda devolvía error 1054 —el listado se
	// quedaba vacío justo cuando alguien buscaba algo—.
	countQuery := `SELECT COUNT(*) FROM accounts_payable ap
		LEFT JOIN supplier_information s ON s.supplier_id = ap.supplier_id` + where
	var total int
	_ = s.dbFor(r).QueryRow(countQuery, args...).Scan(&total)

	dataQuery := fmt.Sprintf(`SELECT ap.id, ap.purchase_id, s.supplier_name, ap.amount, ap.paid_amount, ap.due_amount,
		ap.payment_method_id, ap.payment_date, ap.reference, ap.notes, ap.created_by, ap.created_at,
		IFNULL(ap.concept,'') AS concept, IFNULL(ap.origin,'compra') AS origin
		FROM accounts_payable ap
		LEFT JOIN supplier_information s ON s.supplier_id = ap.supplier_id
		%s ORDER BY ap.created_at DESC LIMIT %d OFFSET %d`, where, length, start)

	rows, err := s.dbFor(r).Query(dataQuery, args...)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id int
		var amount, paidAmount, dueAmount float64
		var purchaseID, supplierName, reference, notes, createdBy, createdAt sql.NullString
		var paymentMethodID sql.NullInt64
		var paymentDate, concept, origin sql.NullString
		_ = rows.Scan(&id, &purchaseID, &supplierName, &amount, &paidAmount, &dueAmount,
			&paymentMethodID, &paymentDate, &reference, &notes, &createdBy, &createdAt, &concept, &origin)
		etiqueta := "Compra"
		if origin.String == "servicio" {
			etiqueta = "Servicio"
		} else if origin.String == "pago_compra" {
			etiqueta = "Pago"
		}
		items = append(items, map[string]interface{}{
			"id":                id,
			"concept":           concept.String,
			"origin":            origin.String,
			"origin_label":      etiqueta,
			"purchase_id":       purchaseID.String,
			"supplier_name":     supplierName.String,
			"amount":            amount,
			"paid_amount":       paidAmount,
			"due_amount":        dueAmount,
			"payment_method_id": paymentMethodID.Int64,
			"payment_date":      paymentDate.String,
			"reference":         reference.String,
			"notes":             notes.String,
			"created_by":        createdBy.String,
			"created_at":        createdAt.String,
		})
	}

	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": total,
		"data":            items,
	})
}

func (s *Server) pagarSave(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "create", "supplier_payment") {
		return
	}
	supplierID, _ := strconv.Atoi(r.FormValue("supplier_id"))
	purchaseID := r.FormValue("purchase_id")
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	paymentMethodID, _ := strconv.Atoi(r.FormValue("payment_method_id"))
	reference := r.FormValue("reference")
	notes := r.FormValue("notes")

	if supplierID <= 0 || amount <= 0 {
		jsonError(w, "missing fields", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// El concepto: sin él, en la lista solo se ve un monto y una referencia, y
	// «por qué se debe esto» hay que ir a buscarlo a otro módulo.
	concepto := strings.TrimSpace(r.FormValue("concept"))
	if concepto == "" {
		if purchaseID != "" {
			concepto = "Compra #" + purchaseID
		} else {
			concepto = "Cuenta por pagar"
		}
	}

	_, err = tx.Exec(`INSERT INTO accounts_payable (supplier_id, purchase_id, concept, origin, amount, paid_amount, due_amount,
		payment_method_id, payment_date, reference, notes, created_by, created_at)
		VALUES (?, ?, ?, 'compra', ?, 0, ?, ?, NOW(), ?, ?, ?, NOW())`,
		supplierID, purchaseID, concepto, amount, amount, paymentMethodID, reference, notes, s.sessionUserID(r))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// SIN ASIENTO A PROPÓSITO: lo que se asienta es la compra, que ya debita la
	// cuenta de compras y acredita la del proveedor. Si este registro asentara
	// también, una compra con su cuenta por pagar duplicaría la deuda en el libro.
	// Aquí solo se anota el documento; lo que mueve el libro es el pago.
	writeJSON(w, map[string]interface{}{"status": 1, "saved": true, "msg": "Cuenta por pagar registrada"})
}

func (s *Server) pagarDelete(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "delete", "supplier_payment") {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/pagar/delete/")
	if id == "" {
		jsonError(w, "missing id", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)
	_, err := db.Exec("DELETE FROM accounts_payable WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "deleted": true, "msg": "Eliminado"})
}

func (s *Server) pagarPay(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "update", "supplier_payment") {
		return
	}
	id, _ := strconv.Atoi(r.FormValue("id"))
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	paymentMethodID, _ := strconv.Atoi(r.FormValue("payment_method_id"))
	reference := r.FormValue("reference")
	notes := r.FormValue("notes")

	if id <= 0 || amount <= 0 {
		jsonError(w, "missing fields", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	createBy := s.sessionUserID(r)

	// La cuenta por pagar dice A QUÉ PROVEEDOR se le está pagando: sin eso no hay
	// cuenta contable que debitar.
	var supplierID int64
	var purchaseRef sql.NullString
	if err := db.QueryRow(`SELECT supplier_id, IFNULL(purchase_id,'') FROM accounts_payable WHERE id = ?`, id).
		Scan(&supplierID, &purchaseRef); err != nil {
		jsonError(w, "La cuenta por pagar no existe", http.StatusNotFound)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`UPDATE accounts_payable SET paid_amount = paid_amount + ?, due_amount = due_amount - ?,
		payment_method_id = ?, reference = ?, notes = ?, payment_date = NOW() WHERE id = ?`,
		amount, amount, paymentMethodID, reference, notes, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// EL ASIENTO DEL PAGO, POR EL CAMINO DEL SISTEMA.
	//
	// Antes se insertaba una fila suelta en acc_transaction con
	// COAID = payment_method_id —el id del MÉTODO de pago, que no es una cuenta:
	// caía en acc_coa 1, 2, 3…—, sin IsAppove ni IsPosted, y sin tocar la cuenta
	// del proveedor. El resultado: el pago no bajaba la deuda en su libro y el
	// asiento quedaba en una cuenta al azar (además invisible, porque el mayor
	// filtra IsAppove=1). Ahora va por el mismo camino que el resto del sistema
	// (acc_vaucher + aprobación, con el período abierto comprobado) y con la doble
	// partida que corresponde: DÉBITO al proveedor —baja el pasivo— y CRÉDITO a la
	// cuenta del método de pago —sale el dinero—.
	medioCOA := int64(0)
	_ = tx.QueryRow(`SELECT coa_id FROM payment_method WHERE id = ? AND is_active = 1`, paymentMethodID).Scan(&medioCOA)
	if medioCOA == 0 {
		pre, perr := s.predefineAccount(r)
		if perr != nil {
			jsonError(w, perr.Error(), http.StatusInternalServerError)
			return
		}
		medioCOA = pre.CashCode
	}
	headcode, sub, err := s.cuentaProveedorTx(tx, supplierID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reference == "" {
		reference = purchaseRef.String
	}
	vno := nextVoucherNoTx(tx, "Payment")
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "Payment", ReferenceNo: fmt.Sprintf("%d", id),
		COAID: headcode, RevCodde: medioCOA,
		Narration: "Pago a proveedor", Comment: notes,
		Debit: amount, SubType: 4, SubCode: sub,
	}, createBy); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Aprobar es lo que mueve el libro (y usa su propia transacción). Si falla
	// —período cerrado, por ejemplo—, el pago quedó registrado pero el asiento
	// está pendiente: se dice tal cual y con el número, para aprobarlo desde
	// Comprobantes, en lugar de dar el pago por asentado sin estarlo.
	if err := s.approveVoucher(r, vno, createBy); err != nil {
		writeJSON(w, map[string]interface{}{"status": 1, "paid": true, "voucher": vno,
			"msg": "Pago registrado. El asiento " + vno + " quedó pendiente: " + err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "paid": true, "voucher": vno, "msg": "Pago registrado y asentado"})
}

func (s *Server) pagarStatement(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_payment") {
		return
	}
	supplierID := strings.TrimPrefix(r.URL.Path, "/pagar/statement/")
	if supplierID == "" {
		jsonError(w, "missing supplier id", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)

	var supplierName string
	_ = db.QueryRow("SELECT supplier_name FROM supplier_information WHERE supplier_id = ?", supplierID).Scan(&supplierName)

	rows, err := db.Query(`SELECT id, purchase_id, amount, paid_amount, due_amount, payment_date, reference, created_at
		FROM accounts_payable WHERE supplier_id = ? ORDER BY created_at DESC`, supplierID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id int
		var amount, paidAmount, dueAmount float64
		var purchaseID, reference, paymentDate, createdAt sql.NullString
		_ = rows.Scan(&id, &purchaseID, &amount, &paidAmount, &dueAmount, &paymentDate, &reference, &createdAt)
		items = append(items, map[string]interface{}{
			"id":           id,
			"purchase_id":  purchaseID.String,
			"amount":       amount,
			"paid_amount":  paidAmount,
			"due_amount":   dueAmount,
			"payment_date": paymentDate.String,
			"reference":    reference.String,
			"created_at":   createdAt.String,
		})
	}

	writeJSON(w, map[string]interface{}{
		"supplier_id":   supplierID,
		"supplier_name": supplierName,
		"items":         items,
	})
}
