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
	// Almacenes (venta y consumo) y cuentas de gasto: el renglón se carga con
	// lo que existe de verdad en esta empresa, no con códigos escritos a mano.
	for k, v := range s.almacenesParaFormulario(r) {
		data[k] = v
	}
	data["CuentasGasto"] = s.cuentasDeGasto(r)
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
		Tipo, Concepto, Almacen       string
	}
	details := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), d.quantity, d.rate,
			d.total_amount, IFNULL(d.batch_id,''), IFNULL(CAST(d.expiry_date AS CHAR),''),
			IFNULL(d.item_type,'venta'), IFNULL(d.concept,''), IFNULL(d.warehouse_id,'')
		FROM product_purchase_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.purchase_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var d drow
			var batch, exp, tipo, concepto, almacen sql.NullString
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Qty, &d.Rate, &d.Total, &batch, &exp,
				&tipo, &concepto, &almacen)
			d.Batch, d.Expiry = batch.String, exp.String
			d.Tipo, d.Concepto, d.Almacen = normalizarTipoItem(tipo.String), concepto.String, almacen.String
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

	data := map[string]interface{}{
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
	}
	for k, v := range s.almacenesParaFormulario(r) {
		data[k] = v
	}
	data["CuentasGasto"] = s.cuentasDeGasto(r)
	s.render(w, r, "purchase_form.html", data)
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

	// -----------------------------------------------------------------
	// CADA RENGLÓN SE INTERPRETA ANTES DE ESCRIBIR NADA.
	// -----------------------------------------------------------------
	//
	// Tipo de ítem (venta / consumo interno / servicio), almacén y cuenta
	// contable se resuelven aquí. Si un renglón no cuadra —un servicio sin
	// concepto, un producto de consumo enviado al almacén de venta, una cuenta
	// de ingreso para un gasto— la compra NO se guarda y el motivo se explica.
	// Antes nada de esto se preguntaba: se metía todo al inventario y se
	// asentaba contra una sola cuenta.
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	renglones, err := s.renglonesDeCompra(r, productIDs, quantities, rates, totals,
		discounts, discPercents, vatPercents, vatValues, batches, expiries)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := s.resolverDestinosCompra(r, pre, renglones); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

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
	narration := "Purchase Voucher"
	comment := "Purchase Voucher for supplier"

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
	headSupplier, subSupplier, err := s.cuentaProveedorTx(tx, supplierID, createBy)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// -----------------------------------------------------------------
	// ASIENTOS: UNA CUENTA POR DESTINO, NO UNA SOLA PARA TODO.
	// -----------------------------------------------------------------
	//
	// El documento se parte en dos: la base (mercancía, suministros o gasto) y
	// el IVA. La base se reparte entre los renglones según su importe y cada
	// renglón va a SU cuenta:
	//
	//   mercancía  -> inventario de mercancía      (activo)
	//   consumo    -> inventario de suministros    (activo)
	//   servicio   -> cuenta de gasto del renglón  (resultado)
	//   IVA        -> IVA crédito fiscal           (activo)
	//
	// El crédito sigue siendo el mismo de antes: caja/banco si se pagó, o el
	// proveedor si quedó a crédito. Cada pata del crédito se reparte entre los
	// mismos destinos para que la partida doble cierre al centavo.
	ivaTotal := parseMoney(r.FormValue("total_vat_amnt"))
	// El IVA va a su propia cuenta y esa cuenta la decide la empresa. Si no está
	// configurada no se asienta «donde caiga»: se detiene y se dice dónde
	// arreglarlo.
	if ivaTotal > 0 && pre.CuentaIvaCompras() == 0 {
		jsonError(w, "La compra tiene IVA y no hay cuenta de IVA crédito fiscal asignada. Asígnela en Configuración → Cuentas por defecto (papel «IVA crédito fiscal (compras)»).", 400)
		return
	}
	baseTotal := round2(grandTotal - ivaTotal)
	if baseTotal < 0 {
		baseTotal = 0
	}

	pesos := make([]float64, len(renglones))
	for i, l := range renglones {
		pesos[i] = l.Total
	}
	importes := repartirImporte(baseTotal, pesos)

	type destinoAsiento struct {
		Cuenta  int64
		Importe float64
	}
	destinosAsiento := []destinoAsiento{}
	for i := range renglones {
		if importes[i] > 0 {
			destinosAsiento = append(destinosAsiento, destinoAsiento{Cuenta: renglones[i].Destino.Cuenta, Importe: importes[i]})
		}
	}
	if ivaTotal > 0 {
		destinosAsiento = append(destinosAsiento, destinoAsiento{Cuenta: pre.CuentaIvaCompras(), Importe: ivaTotal})
	}

	// Patas del crédito: de dónde salió el dinero o a quién se le debe.
	type pataCredito struct {
		Cuenta  int64
		Importe float64
		Vtype   string
		SubType int
		SubCode sql.NullInt64
	}
	patas := []pataCredito{}
	if len(payAmounts) > 0 && len(payTypes) > 0 {
		if payTypes[0] == "0" {
			// Compra a crédito: toda la deuda al proveedor.
			patas = append(patas, pataCredito{Cuenta: headSupplier, Importe: grandTotal, Vtype: "JV", SubType: 4, SubCode: subSupplier})
		} else {
			// Multipago: cada método a su cuenta.
			for i, pt := range payTypes {
				mid, _ := strconv.ParseInt(pt, 10, 64)
				var coaSQL sql.NullInt64
				_ = tx.QueryRow(`SELECT coa_id FROM payment_method WHERE id = ? AND is_active = 1`, mid).Scan(&coaSQL)
				coaid := coaSQL.Int64
				if coaid == 0 {
					coaid = pre.CashCode
				}
				patas = append(patas, pataCredito{Cuenta: coaid, Importe: parseMoney(payAmounts[i]), Vtype: "DV"})
			}
			if dueAmount > 0 {
				patas = append(patas, pataCredito{Cuenta: headSupplier, Importe: dueAmount, Vtype: "JV", SubType: 4, SubCode: subSupplier})
			}
		}
	} else if grandTotal > 0 {
		// Compra corriente que no manda filas de pago: lo pagado sale de caja y
		// lo pendiente queda con el proveedor.
		if sumPay > 0 {
			patas = append(patas, pataCredito{Cuenta: pre.CashCode, Importe: sumPay, Vtype: "DV"})
		}
		if dueAmount > 0 {
			patas = append(patas, pataCredito{Cuenta: headSupplier, Importe: dueAmount, Vtype: "JV", SubType: 4, SubCode: subSupplier})
		}
	}

	pesosDestino := make([]float64, len(destinosAsiento))
	for i, d := range destinosAsiento {
		pesosDestino[i] = d.Importe
	}
	for _, pata := range patas {
		if pata.Importe <= 0 || len(destinosAsiento) == 0 {
			continue
		}
		partes := repartirImporte(pata.Importe, pesosDestino)
		// UNA sola VNo por pata: todas sus cuentas de débito se aprueban juntas,
		// como un asiento con varios renglones y no como asientos sueltos.
		vno := nextVoucherNoTx(tx, pata.Vtype)
		for i, d := range destinosAsiento {
			if partes[i] == 0 {
				continue
			}
			if err := s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: pata.Vtype, ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: d.Cuenta, RevCodde: pata.Cuenta,
				Narration: narration, Comment: comment,
				Debit: partes[i], SubType: pata.SubType, SubCode: pata.SubCode,
			}, createBy); err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
		}
	}

	// details + supplier_product price update
	//
	// El detalle guarda AHORA el destino del renglón: su tipo, su almacén y —si
	// es servicio— la cuenta de gasto. Sin eso, el kárdex, los reportes y una
	// revisión posterior no pueden saber qué era cada renglón; con eso, la
	// compra se puede auditar renglón por renglón.
	purchaseDate := orDefault(r.FormValue("purchase_date"), today())
	for _, l := range renglones {
		expiryVal := interface{}(nil)
		if l.Expiry != "" {
			expiryVal = l.Expiry
		}
		var coaVal any
		if l.Tipo == ItemServicio && l.Destino.Cuenta > 0 {
			coaVal = l.Destino.Cuenta
		}
		// La columna es NOT NULL con '' por defecto: un servicio no tiene
		// almacén y se guarda vacío, no NULL (MySQL en modo estricto rechaza el
		// NULL y la compra del servicio no se podría guardar).
		almacenVal := any("")
		if l.Destino.MueveStock {
			almacenVal = l.Almacen
		}
		if _, err := tx.Exec(`
			INSERT INTO product_purchase_details (purchase_detail_id, purchase_id, product_id, quantity, rate,
				batch_id, expiry_date, total_amount, discount, discount_amnt, vat_amnt_per, vat_amnt, status,
				item_type, coa_id, warehouse_id, concept)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?)`,
			generator(15), purchaseID, l.ProductID, l.Qty, l.Rate, l.Batch, expiryVal, l.Total,
			l.DiscP, l.Disc, l.VatP, l.VatV, l.Tipo, coaVal, almacenVal, l.Concepto); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}

		// Stock: SOLO si el renglón vive en un almacén. Un servicio no entra, no
		// sale y no se cuenta; por eso aquí no hay movimiento que crear.
		if l.Destino.MueveStock {
			pidInt, _ := strconv.Atoi(l.ProductID)
			if pidInt > 0 {
				if err := insertStockMovement(tx, "purchase", pidInt, l.Almacen, l.Qty, l.Rate,
					"purchase", int(purchaseID), fmt.Sprintf("PUR-%d", purchaseID), purchaseDate); err != nil {
					jsonError(w, err.Error(), 500)
					return
				}
			}
		}

		// Precio del proveedor: solo para mercancía y suministros. Un servicio no
		// tiene precio de catálogo que actualizar.
		if l.ProductID == "" || l.Tipo == ItemServicio {
			continue
		}
		_, _ = tx.Exec(`UPDATE supplier_product SET supplier_price=? WHERE product_id=? AND supplier_id=?`,
			fmt.Sprintf("%.2f", l.Rate), l.ProductID, supplierID)
		var relID int
		_ = tx.QueryRow(`SELECT supplier_pr_id FROM supplier_product WHERE product_id=? AND supplier_id=?`, l.ProductID, supplierID).Scan(&relID)
		if relID == 0 {
			_, _ = tx.Exec(`INSERT INTO supplier_product (product_id, products_model, supplier_id, supplier_price)
				VALUES (?,?,?,?)`, l.ProductID, "", supplierID, fmt.Sprintf("%.2f", l.Rate))
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

	// remove vouchers + transactions tied to this purchase. Si cualquiera falla,
	// la compra queda sin borrar: la transacción se revierte al final del handler.
	// El stock ahora se lee de stock_movement: si al borrar la compra se dejan
	// sus movimientos, la mercancía sigue "existiendo" en el almacén aunque la
	// compra ya no esté. Se borran en la misma transacción.
	for _, q := range []string{
		`DELETE FROM acc_transaction WHERE referenceNo = ?`,
		`DELETE FROM acc_vaucher WHERE referenceNo = ?`,
		`DELETE FROM stock_movement WHERE reference_type = 'purchase' AND reference_id = ?`,
		`DELETE FROM product_purchase_details WHERE purchase_id = ?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			jsonError(w, fmt.Sprintf("limpiar compra: %v", err), 500)
			return
		}
	}
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
		Tipo, EtiquetaTipo, Concepto        string
		Almacen, Cuenta                     string
	}
	details := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), IFNULL(p.unit,''),
			d.quantity, d.rate, d.total_amount,
			IFNULL(d.item_type,'venta'), IFNULL(d.concept,''), IFNULL(d.warehouse_id,''), IFNULL(d.coa_id,0)
		FROM product_purchase_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.purchase_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var d drow
			var tipo, concepto, almacen sql.NullString
			var coa int64
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Unit, &d.Qty, &d.Rate, &d.Total,
				&tipo, &concepto, &almacen, &coa)
			d.Tipo = normalizarTipoItem(tipo.String)
			d.EtiquetaTipo, d.Concepto, d.Almacen = etiquetaTipoItem(d.Tipo), concepto.String, almacen.String
			if coa > 0 {
				d.Cuenta = s.nombreCuenta(r, coa)
			}
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
