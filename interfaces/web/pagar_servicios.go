package web

// pagar_servicios.go — Cuentas por pagar: servicios, gastos operativos y el pago
// de una compra a crédito.
//
// EL HUECO QUE TAPA ESTE ARCHIVO.
//
// Hasta ahora solo una compra podía crear una deuda: `product_purchase` con su
// `due_amount`. Pero el comercio también debe el agua, la luz, el teléfono, el
// internet y el arriendo, y nada de eso pasa por el almacén ni por una factura de
// mercancía. No había dónde cargarlo: el gasto aparecía —si aparecía— ya pagado,
// sin deuda registrada antes, y Cuentas por Pagar no lo mostraba por ningún lado.
//
// Aquí se registra la deuda («debemos 1.200 de electricidad de marzo») con su
// contrapartida contable: DÉBITO a la cuenta de servicios —el gasto— y CRÉDITO a
// la cuenta del proveedor —lo que se debe—. El pago se hace después por el camino
// normal, que debita al proveedor y baja la deuda.
//
// POR QUÉ EXIGE PROVEEDOR. Una deuda sin contraparte no tiene cuenta a la que
// acreditarse, y el asiento quedaría contra una cuenta al azar. El proveedor es a
// quién se le debe, aunque sea el dueño del local: cargarlo es un dato, no un
// trámite, y es lo que permite después pagarle y ver su libro.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// pagarServicioSave registra la deuda de un servicio o gasto operativo.
func (s *Server) pagarServicioSave(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "create", "supplier_payment") {
		return
	}
	r.ParseMultipartForm(32 << 20)

	supplierID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("supplier_id")), 10, 64)
	concepto := strings.TrimSpace(r.FormValue("concept"))
	monto := parseMoney(r.FormValue("amount"))
	referencia := strings.TrimSpace(r.FormValue("reference"))
	notas := strings.TrimSpace(r.FormValue("notes"))
	fecha := orDefault(r.FormValue("date"), today())

	switch {
	case supplierID <= 0:
		jsonError(w, "Indique a qué proveedor se le debe el servicio. El agua, la luz, el teléfono y el arriendo los presta alguien: sin esa contraparte la deuda no tiene cuenta a la que acreditarse.", http.StatusBadRequest)
		return
	case concepto == "":
		jsonError(w, "Describa el servicio («Electricidad marzo», «Arriendo local 2», «Internet»): es el texto con el que se verá en Cuentas por Pagar.", http.StatusBadRequest)
		return
	case monto <= 0:
		jsonError(w, "El monto de la deuda tiene que ser mayor que cero.", http.StatusBadRequest)
		return
	}

	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)
	ip := clientIP(r)

	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// La cuenta del proveedor: es la que se acredita (lo que se debe).
	head, sub, err := s.cuentaProveedorTx(tx, supplierID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res, err := tx.Exec(`INSERT INTO accounts_payable (supplier_id, purchase_id, concept, origin, amount,
		paid_amount, due_amount, payment_method_id, payment_date, reference, notes, created_by, created_at)
		VALUES (?, NULL, ?, 'servicio', ?, 0, ?, 0, NULL, ?, ?, ?, ?)`,
		supplierID, concepto, monto, monto, referencia, notas, createBy, now())
	if err != nil {
		jsonError(w, "No se pudo registrar la deuda: "+err.Error(), http.StatusInternalServerError)
		return
	}
	apID, _ := res.LastInsertId()

	// EL ASIENTO: el gasto al débito, la deuda al crédito. Sin esto la deuda
	// existiría solo en esta lista y el libro no la vería, que es exactamente lo
	// que pasaba cuando se pagaba un servicio sin registrarlo antes.
	vno := nextVoucherNoTx(tx, "JV")
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("SERV-%d", apID),
		VDate: fecha, COAID: pre.ServiceCode, RevCodde: head,
		Narration: "Servicio: " + concepto, Comment: referencia,
		Debit: monto, SubType: 4, SubCode: sub,
	}, createBy); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Aprobar es lo que mueve el libro. La aprobación usa su propia transacción:
	// si el período está cerrado, la deuda queda registrada y se dice con el
	// número de asiento, en vez de dar por asentado lo que no lo está.
	msg := "Deuda registrada. El asiento " + vno + " se aprueba desde Comprobantes (la aprobación automática está desactivada)."
	if st := s.settingsFor(r); st.Web.IsAutoapprove == 1 {
		if err := s.approveVoucher(r, vno, createBy); err != nil {
			msg = "Deuda registrada. El asiento " + vno + " quedó pendiente: " + err.Error()
		} else {
			msg = "Deuda registrada y asentada"
		}
	}

	s.appendAudit(r, createBy, ip, "create", "Cuentas por pagar", "accounts_payable",
		strconv.FormatInt(apID, 10), nil, map[string]interface{}{
			"supplier_id": supplierID, "concept": concepto, "amount": monto, "voucher": vno,
		})

	writeJSON(w, map[string]interface{}{
		"status": 1, "saved": true, "id": apID, "voucher": vno, "msg": msg,
	})
}

// pagarCompraPay paga el saldo de una compra a crédito.
//
// POR QUÉ NO BASTA CON accounts_payable. La deuda de una compra vive en
// `product_purchase.due_amount`, que es el número que ven el listado de compras,
// el mayor del proveedor y las cuentas por pagar. Pagarla tiene que bajar ESE
// número; si solo se anotara un pago en otra tabla, la compra seguiría apareciendo
// debiendo para siempre. Aquí se hace todo junto: baja el saldo de la compra, se
// asienta el pago y el pago queda anotado en el registro de cuentas por pagar.
func (s *Server) pagarCompraPay(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "update", "supplier_payment") {
		return
	}
	r.ParseMultipartForm(32 << 20)

	purchaseID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("purchase_id")), 10, 64)
	monto := parseMoney(r.FormValue("amount"))
	pmID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("payment_method_id")), 10, 64)
	referencia := strings.TrimSpace(r.FormValue("reference"))
	notas := strings.TrimSpace(r.FormValue("notes"))
	if purchaseID <= 0 || monto <= 0 {
		jsonError(w, "Indique la compra y un monto mayor que cero.", http.StatusBadRequest)
		return
	}

	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)
	db := s.dbFor(r)

	var supplierID int64
	var pagado, debe float64
	var chalan sql.NullString
	if err := db.QueryRow(`SELECT supplier_id, paid_amount, due_amount, IFNULL(chalan_no,'')
		FROM product_purchase WHERE purchase_id = ?`, purchaseID).
		Scan(&supplierID, &pagado, &debe, &chalan); err != nil {
		jsonError(w, "La compra no existe", http.StatusNotFound)
		return
	}
	if monto > round2(debe)+0.01 {
		jsonError(w, fmt.Sprintf("A la compra #%d solo le quedan %.2f por pagar; no se puede pagar %.2f.", purchaseID, debe, monto), http.StatusBadRequest)
		return
	}

	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	medioCOA := int64(0)
	if pmID > 0 {
		_ = db.QueryRow(`SELECT coa_id FROM payment_method WHERE id = ? AND is_active = 1`, pmID).Scan(&medioCOA)
	}
	if medioCOA == 0 {
		medioCOA = pre.CashCode
	}

	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	nuevoPagado := round2(pagado + monto)
	nuevaDeuda := round2(debe - monto)
	if nuevaDeuda < 0 {
		nuevaDeuda = 0
	}
	if _, err := tx.Exec(`UPDATE product_purchase SET paid_amount = ?, due_amount = ? WHERE purchase_id = ?`,
		nuevoPagado, nuevaDeuda, purchaseID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	head, sub, err := s.cuentaProveedorTx(tx, supplierID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// El pago queda en el registro de cuentas por pagar como fila ya saldada: es
	// lo que permite reconstruir «quién pagó esto, con qué referencia y cuándo».
	if _, err := tx.Exec(`INSERT INTO accounts_payable (supplier_id, purchase_id, concept, origin, amount,
		paid_amount, due_amount, payment_method_id, payment_date, reference, notes, created_by, created_at)
		VALUES (?, ?, ?, 'pago_compra', ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		supplierID, purchaseID, fmt.Sprintf("Pago de compra #%d", purchaseID), monto, monto,
		pmID, today(), referencia, notas, createBy, now()); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// DÉBITO al proveedor —baja el pasivo— y CRÉDITO a la cuenta del medio de
	// pago —sale el dinero—, por el mismo camino que el resto del sistema.
	vno := nextVoucherNoTx(tx, "Payment")
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "Payment", ReferenceNo: fmt.Sprintf("%d", purchaseID),
		COAID: head, RevCodde: medioCOA,
		Narration: "Pago de compra #" + strconv.FormatInt(purchaseID, 10), Comment: referencia,
		Debit: monto, SubType: 4, SubCode: sub,
	}, createBy); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg := "Pago registrado. El asiento " + vno + " se aprueba desde Comprobantes."
	if st := s.settingsFor(r); st.Web.IsAutoapprove == 1 {
		if err := s.approveVoucher(r, vno, createBy); err != nil {
			msg = "Pago registrado. El asiento " + vno + " quedó pendiente: " + err.Error()
		} else {
			msg = "Pago registrado y asentado"
		}
	}

	s.appendAudit(r, createBy, clientIP(r), "update", "Cuentas por pagar", "product_purchase",
		strconv.FormatInt(purchaseID, 10), map[string]interface{}{"due_before": debe},
		map[string]interface{}{"amount": monto, "due_after": nuevaDeuda, "voucher": vno})

	writeJSON(w, map[string]interface{}{
		"status": 1, "paid": true, "voucher": vno, "msg": msg,
		"paid_amount": money(nuevoPagado), "due_amount": money(nuevaDeuda),
	})
}

// pagarPorPagar devuelve TODO lo que el comercio debe hoy, separado por origen.
//
// DOS LISTAS Y NO UNA, PORQUE SON DOS COSAS DISTINTAS. «Documentos por pagar» son
// las compras cuyo saldo quedó abierto: nacen de un documento con número. «Servicios
// y gastos» son las deudas cargadas a mano —agua, luz, arriendo—, que no tienen
// factura de mercancía pero sí una contraparte y una fecha. Sumarlas es lo que
// responde la única pregunta que importa aquí: cuánto hay que pagar.
func (s *Server) pagarPorPagar(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_payment") {
		return
	}
	db := s.dbFor(r)

	documentos := []map[string]interface{}{}
	totalDoc := 0.0
	rows, err := db.Query(`SELECT p.purchase_id, IFNULL(CAST(p.purchase_date AS CHAR),''),
		IFNULL(s.supplier_name,''), IFNULL(p.chalan_no,''), IFNULL(p.control_no,''),
		IFNULL(p.document_kind,'factura'), p.grand_total_amount, p.paid_amount, p.due_amount
		FROM product_purchase p
		LEFT JOIN supplier_information s ON s.supplier_id = p.supplier_id
		WHERE p.due_amount > 0
		ORDER BY p.purchase_date ASC, p.id ASC LIMIT 500`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var pid int64
		var fecha, prov, chalan, control, kind sql.NullString
		var total, pagado, debe float64
		if err := rows.Scan(&pid, &fecha, &prov, &chalan, &control, &kind, &total, &pagado, &debe); err != nil {
			continue
		}
		documentos = append(documentos, map[string]interface{}{
			"purchase_id": pid, "date": fecha.String, "supplier": prov.String,
			"chalan_no": chalan.String, "control_no": control.String,
			"document_kind": CompraKind(kind.String), "kind_label": EtiquetaCompra(kind.String),
			"total": money(total), "paid": money(pagado), "due": money(debe),
		})
		totalDoc += debe
	}
	rows.Close()

	servicios := []map[string]interface{}{}
	totalServ := 0.0
	rows2, err := db.Query(`SELECT ap.id, IFNULL(ap.concept,''), IFNULL(s.supplier_name,''),
		IFNULL(ap.reference,''), IFNULL(CAST(ap.created_at AS CHAR),''), ap.amount, ap.paid_amount, ap.due_amount
		FROM accounts_payable ap
		LEFT JOIN supplier_information s ON s.supplier_id = ap.supplier_id
		WHERE ap.due_amount > 0 AND ap.origin = 'servicio'
		ORDER BY ap.id DESC LIMIT 500`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows2.Next() {
		var id int64
		var concepto, prov, ref, fecha sql.NullString
		var monto, pagado, debe float64
		if err := rows2.Scan(&id, &concepto, &prov, &ref, &fecha, &monto, &pagado, &debe); err != nil {
			continue
		}
		servicios = append(servicios, map[string]interface{}{
			"id": id, "concept": concepto.String, "supplier": prov.String,
			"reference": ref.String, "date": fecha.String,
			"amount": money(monto), "paid": money(pagado), "due": money(debe),
		})
		totalServ += debe
	}
	rows2.Close()

	// Registros manuales de compra: NO se suman, porque son la misma deuda de una
	// compra que ya está contada arriba. Se cuentan para que quien mire sepa que
	// existen y no los busque sumando.
	var manuales int
	_ = db.QueryRow(`SELECT COUNT(*) FROM accounts_payable WHERE due_amount > 0 AND IFNULL(origin,'compra') <> 'servicio'`).Scan(&manuales)

	writeJSON(w, map[string]interface{}{
		"documentos":         documentos,
		"servicios":          servicios,
		"total_documentos":   money(totalDoc),
		"total_servicios":    money(totalServ),
		"total_por_pagar":    money(totalDoc + totalServ),
		"registros_manuales": manuales,
		"nota": "El total por pagar son los documentos de compra con saldo más los servicios y gastos registrados. " +
			"Las cuentas por pagar anotadas a mano sobre una compra no se suman aquí: son la misma deuda, ya contada en su documento.",
	})
}

// ---------- Listas para los selectores ----------

// pbListaProveedores alimenta el selector del gasto de servicio. Se arma en el
// servidor y no con una llamada del navegador: el formulario tiene que estar
// completo desde el primer render, sin un paso que pueda fallar.
func (s *Server) pbListaProveedores(r *http.Request) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := s.dbFor(r).Query(`SELECT supplier_id, IFNULL(supplier_name,'') FROM supplier_information ORDER BY supplier_name LIMIT 1000`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{"id": id, "name": name})
	}
	return out
}

// pbListaMetodos son los medios de pago REALES (payment_method), no las cuentas de
// caja. Es la diferencia que importaba: accounts_payable y debt_collection guardan
// payment_method.id, y ofrecer el HeadCode de acc_coa mandaba el id equivocado.
func (s *Server) pbListaMetodos(r *http.Request) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := s.dbFor(r).Query(`SELECT id, IFNULL(name,'') FROM payment_method WHERE is_active = 1 ORDER BY name`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{"id": id, "name": name})
	}
	return out
}

// pbListaClientes alimenta los selectores de cuentas por cobrar.
func (s *Server) pbListaClientes(r *http.Request) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := s.dbFor(r).Query(`SELECT customer_id, IFNULL(customer_name,'') FROM customer_information ORDER BY customer_name LIMIT 2000`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{"id": id, "name": name})
	}
	return out
}
