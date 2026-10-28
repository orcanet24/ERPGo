package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Purchase module (replicates modules/purchase) ----------

func (s *Server) registerPurchaseRoutes() {
	s.mux.HandleFunc("GET /purchase/list", s.purchaseList)
	s.mux.HandleFunc("/purchase/list/data", s.purchaseListData)
	s.mux.HandleFunc("GET /purchase/add", s.purchaseForm)
	s.mux.HandleFunc("GET /purchase/edit/{id}", s.purchaseEdit)
	s.mux.HandleFunc("POST /purchase/save", s.purchaseSave)
	// Comprobación previa del candado: si esa compra ya está, se dice ANTES de
	// escribir nada (y el guardado la bloquea igual, por el índice único).
	s.mux.HandleFunc("POST /purchase/dup-check", s.purchaseDupCheck)
	s.mux.HandleFunc("POST /purchase/delete/{id}", s.purchaseDelete)
	s.mux.HandleFunc("GET /purchase/details/{id}", s.purchaseDetails)
}

func (s *Server) purchaseList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "purchase_list.html", map[string]interface{}{"Title": "Gestionar Compras"})
}

func (s *Server) purchaseListData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE a.chalan_no LIKE ? OR s.supplier_name LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_purchase").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_purchase a JOIN supplier_information s ON s.supplier_id=a.supplier_id"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.purchase_id, a.chalan_no, a.supplier_id, s.supplier_name,
			a.grand_total_amount, a.paid_amount, a.due_amount, a.purchase_date, a.status,
			IFNULL(a.document_kind,'factura') AS document_kind
		FROM product_purchase a
		JOIN supplier_information s ON s.supplier_id = a.supplier_id
		`+where+`
		ORDER BY a.id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var pid int64
		var chalan, sname, pdate, dkind sql.NullString
		var sid int64
		var total, paid, due float64
		var status int
		if err := rows.Scan(&pid, &chalan, &sid, &sname, &total, &paid, &due, &pdate, &status, &dkind); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": pid, "chalan": chalan.String, "supplier_id": sid, "supplier": sname.String,
			"total": money(total), "paid": money(paid), "due": money(due),
			"date": pdate.String, "status": status,
			"document_kind": CompraKind(dkind.String), "kind_label": EtiquetaCompra(dkind.String),
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// purchaseForm shows the purchase entry page.
func (s *Server) purchaseForm(w http.ResponseWriter, r *http.Request) {
	// payment methods: accounts under Cash / Cash at Bank (replicates pmethod_dropdown)
	methods := []map[string]interface{}{}
	rows, _ := s.dbFor(r).Query(`SELECT HeadCode, HeadName FROM acc_coa WHERE PHeadName='Cash' OR PHeadName='Cash at Bank' ORDER BY HeadName`)
	if rows != nil {
		for rows.Next() {
			var hc int64
			var hn string
			_ = rows.Scan(&hc, &hn)
			methods = append(methods, map[string]interface{}{"id": hc, "name": hn})
		}
		rows.Close()
	}
	data := map[string]interface{}{
		"Title":        "Nueva Compra",
		"ControlNo":    "",
		"DocumentKind": CompraFactura,
		"Methods":      methods,
	}
	// ¿Se viene de la bandeja con un documento? La propuesta se carga AQUÍ, en
	// el mismo formulario de siempre: nada de atajos que salten el candado ni el
	// renglón a renglón. Si el documento no existe o ya se descartó, el
	// formulario se abre vacío y sin ruido.
	s.inboxParaFormulario(r, data)
	s.render(w, r, "purchase_form.html", data)
}

// purchaseEdit loads a purchase for editing.
func (s *Server) purchaseEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var pid int64
	var chalan, pdate, pdetails sql.NullString
	var sid int64
	var total, paid, due, discount float64
	var dkind sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT purchase_id, chalan_no, supplier_id, grand_total_amount, paid_amount,
		due_amount, total_discount, purchase_date, purchase_details, IFNULL(document_kind,'factura')
		FROM product_purchase WHERE purchase_id = ?`, id).
		Scan(&pid, &chalan, &sid, &total, &paid, &due, &discount, &pdate, &pdetails, &dkind)
	if err != nil {
		http.Error(w, "Compra no encontrada", 404)
		return
	}

	type drow struct {
		ProductID, ProductName, Model string
		Qty, Rate, Total              float64
		Batch, Expiry                 string
	}
	details := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), d.quantity, d.rate,
			d.total_amount, IFNULL(d.batch_id,''), IFNULL(CAST(d.expiry_date AS CHAR),'')
		FROM product_purchase_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.purchase_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var d drow
			var batch, exp sql.NullString
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Qty, &d.Rate, &d.Total, &batch, &exp)
			d.Batch, d.Expiry = batch.String, exp.String
			details = append(details, d)
		}
		rows.Close()
	}

	methods := []map[string]interface{}{}
	mrows, _ := s.dbFor(r).Query(`SELECT HeadCode, HeadName FROM acc_coa WHERE PHeadName='Cash' OR PHeadName='Cash at Bank' ORDER BY HeadName`)
	if mrows != nil {
		for mrows.Next() {
			var hc int64
			var hn string
			_ = mrows.Scan(&hc, &hn)
			methods = append(methods, map[string]interface{}{"id": hc, "name": hn})
		}
		mrows.Close()
	}

	var controlNo sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(control_no,'') FROM product_purchase WHERE purchase_id = ?`, pid).Scan(&controlNo)

	s.render(w, r, "purchase_form.html", map[string]interface{}{
		"Title":        "Editar Compra",
		"EditID":       pid,
		"Chalan":       chalan.String,
		"ControlNo":    controlNo.String,
		"DocumentKind": CompraKind(dkind.String),
		"Supplier":     sid,
		"PDate":        pdate.String,
		"PDetails":     pdetails.String,
		"Grand":        total,
		"Details":      details,
		"Methods":      methods,
	})
}

// purchaseSave replicates insert_purchase(): header + details + vouchers +
// autoapprove when enabled.
func (s *Server) purchaseSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	supplierID, _ := strconv.ParseInt(r.FormValue("supplier_id"), 10, 64)
	// Documento de la bandeja que propuso esta compra: se amarra DESPUÉS del
	// commit (inboxVincular), nunca antes: si el guardado falla, el documento
	// sigue disponible para otro intento.
	inboxID, _ := strconv.ParseInt(r.FormValue("inbox_id"), 10, 64)
	if supplierID == 0 {
		jsonError(w, "Proveedor inválido", 400)
		return
	}

	productIDs := r.Form["product_id"]
	quantities := r.Form["product_quantity"]
	rates := r.Form["product_rate"]
	totals := r.Form["total_price"]
	batches := r.Form["batch_no"]
	expiries := r.Form["expiry_date"]
	discounts := r.Form["discountvalue"]
	discPercents := r.Form["discount_per"]
	vatPercents := r.Form["vatpercent"]
	vatValues := r.Form["vatvalue"]

	if len(productIDs) == 0 {
		jsonError(w, "Agregue al menos un producto", 400)
		return
	}

	// payment (multipay like PHP: pamount_by_method[] + multipaytype[])
	payAmounts := r.Form["pamount_by_method"]
	payTypes := r.Form["multipaytype"]
	grandTotal := parseMoney(r.FormValue("grand_total_price"))
	paidAmount := parseMoney(r.FormValue("paid_amount"))
	dueAmount := parseMoney(r.FormValue("due_amount"))

	// ---------------------------------------------------------------
	// CLASIFICACIÓN DEL DOCUMENTO Y CANDADO CONTRA DUPLICADOS.
	// ---------------------------------------------------------------
	//
	// UNA FACTURA Y UNA ORDEN NO SON LO MISMO. La factura del proveedor es un
	// documento fiscal —da crédito de IVA y obliga a retenciones—; una nota de
	// entrega o una orden meten mercancía y generan deuda, pero no dan crédito
	// fiscal. Hasta ahora las dos se guardaban igual y el Libro de Compras las
	// tomaba a todas, así que una nota de entrega aparecía como IVA inexistente.
	//
	// Una factura CON IVA sin número de factura no se acepta: es un crédito que el
	// SENIAT no reconoce y una retención sin documento que la respalde. Se pide el
	// número o que se clasifique como lo que es; no se bloquea nada más.
	docKind := CompraKind(r.FormValue("document_kind"))
	controlNo := strings.TrimSpace(r.FormValue("control_no"))
	chalanNo := strings.TrimSpace(r.FormValue("chalan_no"))
	if docKind == CompraFactura && chalanNo == "" && parseMoney(r.FormValue("total_vat_amnt")) > 0 {
		jsonError(w, "Esta compra va clasificada como FACTURA y trae IVA, pero no tiene número de factura del proveedor: sin el número no hay crédito fiscal que declarar ni retención que respaldar. Escríbalo, o clasifíquela como nota de entrega / orden de compra si todavía no llegó la factura.", 400)
		return
	}

	// EL HASH: la identidad de la compra (proveedor + tipo + números + renglones).
	hashCompra := hashDeCompra(supplierID, r.FormValue("supplier_rif"), docKind, controlNo, chalanNo,
		compraLineasDeFormulario(r), grandTotal)

	// EL CANDADO, ANTES DE ESCRIBIR NADA. Si esa compra ya está registrada, no se
	// crea otra: se responde cuál es y desde cuándo. El intento queda en la pista
	// de auditoría, porque un duplicado a medias es lo que descuadra el stock y las
	// cuentas por pagar, y saber quién lo intentó es parte del arreglo.
	if existente, dup := s.compraExistente(r, hashCompra, supplierID, controlNo, chalanNo); dup {
		s.appendAudit(r, createBy, clientIP(r), "duplicate_blocked", "Compras", "product_purchase",
			fmt.Sprint(existente["purchase_id"]), nil, map[string]interface{}{
				"supplier_id": supplierID, "chalan_no": chalanNo, "control_no": controlNo,
				"document_kind": docKind, "total": grandTotal,
			})
		writeJSON(w, compraDuplicadaRespuesta(existente))
		return
	}

	var sumPay float64
	for _, a := range payAmounts {
		sumPay += parseMoney(a)
	}
	if len(payAmounts) == 0 {
		// single paid_amount
		sumPay = paidAmount
	}
	if len(payTypes) == 0 {
		payTypes = []string{"0"} // credit purchase default
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// purchase id = MAX(purchase_id)+1 (number_generator)
	var purchaseID int64
	err = tx.QueryRow(`SELECT IFNULL(MAX(purchase_id),0)+1 FROM product_purchase`).Scan(&purchaseID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	isCredit := payTypes[0] == "0"
	// payment_type: 1=cash, 2=card, 3=credit sale, 4=multi-pay (like PHP)
	paymentType := 1
	if len(payTypes) > 0 && payTypes[0] != "0" {
		if t, err := strconv.Atoi(payTypes[0]); err == nil && t > 0 {
			paymentType = t
		}
	}

	// Multi-monedia: tasa BCV del día
	currencyCode := orDefault(r.FormValue("currency_code"), "VES")
	tasaBCV := 1.0
	totalAmountUSD := 0.0
	if currencyCode == "VES" {
		tasaVal, _, err := s.ObtenerTasaHoy(tx, orDefault(r.FormValue("purchase_date"), today()), "USD")
		if err == nil && tasaVal > 0 {
			tasaBCV = tasaVal
		}
		if grandTotal > 0 && tasaBCV > 0 {
			totalAmountUSD = round2(grandTotal / tasaBCV)
		}
	} else if currencyCode == "USD" {
		tasaVal, _, err := s.ObtenerTasaHoy(tx, orDefault(r.FormValue("purchase_date"), today()), "USD")
		if err == nil && tasaVal > 0 {
			tasaBCV = tasaVal
		}
		totalAmountUSD = grandTotal
	}

	// header
	if _, err := tx.Exec(`
		INSERT INTO product_purchase (purchase_id, chalan_no, supplier_id, grand_total_amount, paid_amount,
			due_amount, total_discount, invoice_discount, total_vat_amnt, purchase_date, purchase_details,
			status, payment_type, is_credit, control_no, supplier_rif, warehouse_id,
			currency_code, exchange_rate, total_amount_usd, document_kind, purchase_hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?,?,?,?,?,?)`,
		purchaseID, chalanNo, supplierID, grandTotal, paidAmount, dueAmount,
		parseMoney(r.FormValue("discount")), parseMoney(r.FormValue("total_discount")),
		parseMoney(r.FormValue("total_vat_amnt")),
		orDefault(r.FormValue("purchase_date"), today()),
		r.FormValue("purchase_details"), paymentType, isCredit,
		controlNo,
		NormalizeRIF(r.FormValue("supplier_rif")),
		orDefault(r.FormValue("warehouse_id"), "WH-MAIN"),
		currencyCode, tasaBCV, totalAmountUSD, docKind, hashCompra); err != nil {
		// CARRERA: dos cargas de la misma compra a la vez, o un reenvío que se coló
		// entre la comprobación de arriba y esta escritura. El índice único decidió,
		// así que se responde con la compra que quedó en vez de con un error de base
		// que no dice nada al operador ni al agente.
		if compraEsDuplicadoDeClave(err) {
			_ = tx.Rollback()
			if existente, dup := s.compraExistente(r, hashCompra, supplierID, controlNo, chalanNo); dup {
				writeJSON(w, compraDuplicadaRespuesta(existente))
				return
			}
		}
		jsonError(w, err.Error(), 500)
		return
	}

	// accounting vouchers (replicates insert_purchase_debitvoucher logic)
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	narration := "Purchase Voucher"
	comment := "Purchase Voucher for supplier"
	purchaseCOA := pre.PurchaseCode

	// LA CUENTA DEL PROVEEDOR, NO LA GENERAL.
	//
	// El mayor del proveedor (/supplier/ledger) y la columna de saldo del listado
	// leen la subcuenta del proveedor en acc_coa (la que crea su alta bajo 21110).
	// Los asientos se hacían contra la cuenta GENERAL de proveedores, con el
	// detalle en subType/subCode: cada proveedor tenía su cuenta en cero y su libro
	// decía «Sin movimientos» para siempre.
	//
	// Se resuelve una vez para todos los renglones, y se crea si le falta —caso de
	// los proveedores cargados desde el PHP original—, para que la deuda tenga
	// dónde asentarse.
	headSupplier, subSupplier, err := s.cuentaProveedorTx(tx, supplierID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if len(payAmounts) > 0 && len(payTypes) > 0 {
		if payTypes[0] == "0" {
			// COMPRA A CRÉDITO: DÉBITO a la cuenta de compras, CRÉDITO a la del
			// proveedor. El signo estaba invertido (la fila se armaba con `Credit:`
			// sobre la cuenta de compras y la doble partida de approveVoucher la
			// convertía en un DÉBITO al proveedor), así que la deuda del proveedor
			// nacía RESTANDO en su libro. La venta a crédito lo hace bien y sirve de
			// espejo: «Debit: pre.CustomerCode» (invoice.go) — la deuda del tercero
			// va en el lado que le corresponde.
			vno := nextVoucherNoTx(tx, "JV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: purchaseCOA, RevCodde: headSupplier,
				Narration: narration, Comment: comment,
				Debit: grandTotal, SubType: 4, SubCode: subSupplier,
			}, createBy)
		} else {
			// El select del formulario manda el ID del método de pago
			// (payment_method.id), no una cuenta contable: con el ID crudo el
			// asiento se iba a acc_coa 1, 2, 3… en vez de a la cuenta del
			// método (efectivo=101, banco=10205…). Se resuelve coa_id aquí.
			for i, pt := range payTypes {
				mid, _ := strconv.ParseInt(pt, 10, 64)
				var coaSQL sql.NullInt64
				_ = tx.QueryRow(`SELECT coa_id FROM payment_method WHERE id = ? AND is_active = 1`, mid).Scan(&coaSQL)
				coaid := coaSQL.Int64
				if coaid == 0 {
					coaid = pre.CashCode
				}
				amount := parseMoney(payAmounts[i])
				vno := nextVoucherNoTx(tx, "DV")
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: "DV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
					COAID: purchaseCOA, RevCodde: coaid,
					Narration: narration, Comment: comment,
					Debit: amount,
				}, createBy)
			}
			if dueAmount > 0 {
				// Lo que queda a crédito, igual que arriba: débito a compras y
				// crédito al proveedor (antes iba al revés y contra la cuenta general).
				vno := nextVoucherNoTx(tx, "JV")
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
					COAID: purchaseCOA, RevCodde: headSupplier,
					Narration: narration, Comment: comment,
					Debit: dueAmount, SubType: 4, SubCode: subSupplier,
				}, createBy)
			}
		}
	} else if grandTotal > 0 {
		// POR QUÉ ESTA RAMA EXISTE: el MISMO bug que en la venta, y por el
		// mismo motivo. La sección de pagos del formulario de compra solo se
		// envía en multi-pago, así que una compra corriente —pagada o a crédito—
		// no llegaba a `acc_vaucher` en absoluto. La mercadería entraba al
		// almacén y el costo no aparecía en ninguna parte: el inventario era un
		// número que solo existía en los lotes.
		// Los dos renglones iban con `Credit:` sobre la cuenta de compras, que en
		// la doble partida deja DÉBITO en la cuenta contraria: la compra se
		// acreditaba (bajando el costo) y salían débitos en caja y en el proveedor.
		// Aquí se asienta como en la rama de multi-pago, que ya lo hacía bien: la
		// cuenta de compras al DÉBITO y el dinero que sale al crédito.
		if sumPay > 0 {
			vno := nextVoucherNoTx(tx, "DV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "DV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: purchaseCOA, RevCodde: pre.CashCode,
				Narration: narration, Comment: comment,
				Debit: sumPay,
			}, createBy)
		}
		if dueAmount > 0 {
			vno := nextVoucherNoTx(tx, "JV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: purchaseCOA, RevCodde: headSupplier,
				Narration: narration, Comment: comment,
				Debit: dueAmount, SubType: 4, SubCode: subSupplier,
			}, createBy)
		}
	}

	// details + supplier_product price update
	for i, pid := range productIDs {
		qty := float64(1)
		rate, total := 0.0, 0.0
		disc, discP, vatP, vatV := 0.0, 0.0, 0.0, 0.0
		batch, expiry := "", ""
		if i < len(quantities) {
			qty = parseMoney(quantities[i])
		}
		if i < len(rates) {
			rate = parseMoney(rates[i])
		}
		if i < len(totals) {
			total = parseMoney(totals[i])
		}
		if i < len(discounts) {
			disc = parseMoney(discounts[i])
		}
		if i < len(discPercents) {
			discP = parseMoney(discPercents[i])
		}
		if i < len(vatPercents) {
			vatP = parseMoney(vatPercents[i])
		}
		if i < len(vatValues) {
			vatV = parseMoney(vatValues[i])
		}
		if i < len(batches) {
			batch = batches[i]
		}
		if i < len(expiries) {
			expiry = expiries[i]
		}
		expiryVal := interface{}(nil)
		if expiry != "" {
			expiryVal = expiry
		}

		if _, err := tx.Exec(`
			INSERT INTO product_purchase_details (purchase_detail_id, purchase_id, product_id, quantity, rate,
				batch_id, expiry_date, total_amount, discount, discount_amnt, vat_amnt_per, vat_amnt, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1)`,
			generator(15), purchaseID, pid, qty, rate, batch, expiryVal, total, discP, disc, vatP, vatV); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// Actualizar stock_movement (rastreo)
		pidInt, _ := strconv.Atoi(pid)
		purchaseDate := orDefault(r.FormValue("purchase_date"), today())
		if err := insertStockMovement(tx, "purchase", pidInt, "WH-MAIN", qty, rate, "purchase", int(purchaseID), fmt.Sprintf("PUR-%d", purchaseID), purchaseDate); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}

		// update supplier price (like PHP)
		_, _ = tx.Exec(`UPDATE supplier_product SET supplier_price=? WHERE product_id=? AND supplier_id=?`,
			fmt.Sprintf("%.2f", rate), pid, supplierID)
		// ensure supplier_product relation exists
		var relID int
		_ = tx.QueryRow(`SELECT supplier_pr_id FROM supplier_product WHERE product_id=? AND supplier_id=?`, pid, supplierID).Scan(&relID)
		if relID == 0 {
			_, _ = tx.Exec(`INSERT INTO supplier_product (product_id, products_model, supplier_id, supplier_price)
				VALUES (?,?,?,?)`, pid, "", supplierID, fmt.Sprintf("%.2f", rate))
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// El documento de la bandeja queda cargado: la compra ya existe, así que
	// solo se registra el enlace y su rastro de auditoría. Si esto falla, la
	// compra sigue siendo válida —lo que se pierde es la puesta enlazada—.
	if inboxID > 0 {
		s.inboxVincular(r, inboxID, purchaseID)
	}

	// autoapprove if web_setting.is_autoapprove_v == 1 (like PHP)
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		vouchers, _ := s.dbFor(r).Query(`SELECT VNo FROM acc_vaucher WHERE referenceNo = ? AND status = 0`, fmt.Sprintf("%d", purchaseID))
		var vnos []string
		if vouchers != nil {
			for vouchers.Next() {
				var v string
				_ = vouchers.Scan(&v)
				vnos = append(vnos, v)
			}
			vouchers.Close()
		}
		for _, v := range vnos {
			_ = s.approveVoucher(r, v, createBy)
		}
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Compra registrada", "purchase_id": purchaseID,
	})
}

func (s *Server) purchaseDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// LA REGLA DE NO BORRADO. Una compra registrada movió mercancía, publicó
	// asiento y declaró IVA; nada de eso se deshace borrando la fila, y lo que
	// hacía este handler era exactamente eso: borrar los asientos publicados sin
	// reverso. Si ya tiene movimiento, se anula, no se elimina.
	if msg := s.guardarBorrado(r, id, "la compra", consejoCompra, referenciasCompra); msg != "" {
		jsonError(w, msg, 400)
		return
	}
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// remove vouchers + transactions tied to this purchase
	_, _ = tx.Exec(`DELETE FROM acc_transaction WHERE referenceNo = ?`, id)
	_, _ = tx.Exec(`DELETE FROM acc_vaucher WHERE referenceNo = ?`, id)
	_, _ = tx.Exec(`DELETE FROM product_purchase_details WHERE purchase_id = ?`, id)
	res, err := tx.Exec(`DELETE FROM product_purchase WHERE purchase_id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Compra no encontrada", 404)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminada", "redirect": saveRedirect(r, "/purchase/list")})
}

// purchaseDetails shows purchase view.
func (s *Server) purchaseDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var pid int64
	var chalan, pdate, pdetails sql.NullString
	var sid int64
	var total, paid, due float64
	err := s.dbFor(r).QueryRow(`SELECT purchase_id, chalan_no, supplier_id, grand_total_amount, paid_amount,
		due_amount, purchase_date, purchase_details FROM product_purchase WHERE purchase_id = ?`, id).
		Scan(&pid, &chalan, &sid, &total, &paid, &due, &pdate, &pdetails)
	if err != nil {
		http.Error(w, "Compra no encontrada", 404)
		return
	}
	var sname sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT supplier_name FROM supplier_information WHERE supplier_id=?`, sid).Scan(&sname)

	type drow struct {
		ProductID, ProductName, Model, Unit string
		Qty, Rate, Total                    float64
	}
	details := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), IFNULL(p.unit,''),
			d.quantity, d.rate, d.total_amount
		FROM product_purchase_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.purchase_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var d drow
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Unit, &d.Qty, &d.Rate, &d.Total)
			details = append(details, d)
		}
		rows.Close()
	}

	st := s.settingsFor(r)
	s.render(w, r, "purchase_details.html", map[string]interface{}{
		"Title":    "Detalle de Compra #" + id,
		"PID":      pid,
		"Chalan":   chalan.String,
		"PDate":    pdate.String,
		"Supplier": sname.String,
		"Grand":    total,
		"Paid":     paid,
		"Due":      due,
		"Details":  details,
		"Currency": st.Web.Currency,
	})
}

// orDefault returns s if non-empty else def.
func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// nextVoucherNoTx: next voucher number inside a transaction.
func nextVoucherNoTx(tx *sql.Tx, vtype string) string {
	var vno sql.NullString
	_ = tx.QueryRow(`SELECT VNo FROM acc_vaucher WHERE Vtype = ? ORDER BY id DESC LIMIT 1`, vtype).Scan(&vno)
	n := int64(0)
	if vno.Valid && vno.String != "" {
		parts := strings.SplitN(vno.String, "-", 2)
		if len(parts) == 2 {
			fmt.Sscanf(parts[1], "%d", &n)
		}
	}
	return fmt.Sprintf("%s-%d", vtype, n+1)
}

// fechaDelDocumento: la fecha del documento que originó el asiento.
//
// insertVoucherRowTx usaba today() cuando el llamador no pasaba VDate, y
// factura/compra/cobro/banco NO lo pasaban: todos los asientos de una
// operación de 2024 se grababan con la fecha de HOY. Los libros anuales
// (YEAR(VDate)) salían vacíos, el balance por período no cuadraba con las
// facturas y no había forma de cerrar un mes histórico. El asiento lleva la
// fecha del documento porque es la fecha de la OPERACIÓN, no la del clic.
//
// Se leen los campos de fecha que los formularios realmente mandan; si el
// request no trae ninguno (JSON, API interna), hoy sigue siendo lo correcto.
func fechaDelDocumento(r *http.Request) string {
	for _, k := range []string{"invoice_date", "purchase_date", "payment_date",
		"transaction_date", "date", "period_from", "collection_date"} {
		if v := strings.TrimSpace(r.FormValue(k)); v != "" {
			return v
		}
	}
	return today()
}

// insertVoucherRowTx: voucher insert inside a transaction.
func (s *Server) insertVoucherRowTx(r *http.Request, tx *sql.Tx, row VoucherRow, createBy string) error {
	if row.VDate == "" {
		row.VDate = fechaDelDocumento(r)
	}
	// Período cerrado: no se crea contabilidad en un mes ya firmado. Es el
	// mismo punto de paso que usan venta, compra, cobro, banco y la IA, así que
	// la regla no depende de que cada handler se acuerde de comprobarla.
	if err := s.exigirPeriodoAbiertoTx(tx, row.VDate); err != nil {
		return err
	}
	chequeDate := interface{}(nil)
	if row.ChequeDate != "" {
		chequeDate = row.ChequeDate
	}
	_, err := tx.Exec(`
		INSERT INTO acc_vaucher
			(fyear, VNo, Vtype, referenceNo, VDate, COAID, Narration, ledgerComment,
			 Debit, Credit, RevCodde, subType, subCode, chequeNo, chequeDate,
			 isApproved, CreateBy, CreateDate, status)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,0)`,
		financialYear(s.dbFor(r)), row.VNo, row.Vtype, row.ReferenceNo, row.VDate, row.COAID,
		row.Narration, row.Comment, row.Debit, row.Credit, row.RevCodde,
		row.SubType, row.SubCode, row.ChequeNo, chequeDate, voucherCreateBy(createBy), now())
	return err
}

// voucherCreateBy traduce el usuario que hace la operación al entero que
// espera la columna.
//
// `acc_vaucher.CreateBy` y `approvedBy` son INT (columnas heredadas del PHP
// original), pero el ERP pasa por todas partes el NOMBRE de usuario, que es
// texto. Con MySQL en modo estricto eso es Error 1366 "Incorrect integer value
// for column 'CreateBy'" y el asiento NO se escribe.
//
// El fallo era invisible porque las veinticinco llamadas del proyecto a
// `insertVoucherRowTx` descartan el error con `_ =`. Resultado: una compra o una
// factura se guardaban sin su asiento, sin mensaje y sin rastro, y el descuadre
// aparecía semanas después al cerrar el mes.
//
// Se traduce a entero cuando el usuario es numérico —que es lo que guarda
// `users.id`— y a 0 cuando es un nombre. El nombre completo queda en
// `acc_transaction.CreateBy`, que sí es VARCHAR.
func voucherCreateBy(usuario string) int {
	n, err := strconv.Atoi(strings.TrimSpace(usuario))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
