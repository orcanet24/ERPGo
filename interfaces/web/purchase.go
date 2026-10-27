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
			a.grand_total_amount, a.paid_amount, a.due_amount, a.purchase_date, a.status
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
		var chalan, sname, pdate sql.NullString
		var sid int64
		var total, paid, due float64
		var status int
		if err := rows.Scan(&pid, &chalan, &sid, &sname, &total, &paid, &due, &pdate, &status); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": pid, "chalan": chalan.String, "supplier_id": sid, "supplier": sname.String,
			"total": money(total), "paid": money(paid), "due": money(due),
			"date": pdate.String, "status": status,
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
	s.render(w, r, "purchase_form.html", map[string]interface{}{
		"Title":     "Nueva Compra",
		"ControlNo": "",
		"Methods":   methods,
	})
}

// purchaseEdit loads a purchase for editing.
func (s *Server) purchaseEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var pid int64
	var chalan, pdate, pdetails sql.NullString
	var sid int64
	var total, paid, due, discount float64
	err := s.dbFor(r).QueryRow(`SELECT purchase_id, chalan_no, supplier_id, grand_total_amount, paid_amount,
		due_amount, total_discount, purchase_date, purchase_details FROM product_purchase WHERE purchase_id = ?`, id).
		Scan(&pid, &chalan, &sid, &total, &paid, &due, &discount, &pdate, &pdetails)
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
		"Title":     "Editar Compra",
		"EditID":    pid,
		"Chalan":    chalan.String,
		"ControlNo": controlNo.String,
		"Supplier":  sid,
		"PDate":     pdate.String,
		"PDetails":  pdetails.String,
		"Grand":     total,
		"Details":   details,
		"Methods":   methods,
	})
}

// purchaseSave replicates insert_purchase(): header + details + vouchers +
// autoapprove when enabled.
func (s *Server) purchaseSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	supplierID, _ := strconv.ParseInt(r.FormValue("supplier_id"), 10, 64)
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
			currency_code, exchange_rate, total_amount_usd)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?,?,?,?)`,
		purchaseID, r.FormValue("chalan_no"), supplierID, grandTotal, paidAmount, dueAmount,
		parseMoney(r.FormValue("discount")), parseMoney(r.FormValue("total_discount")),
		parseMoney(r.FormValue("total_vat_amnt")),
		orDefault(r.FormValue("purchase_date"), today()),
		r.FormValue("purchase_details"), paymentType, isCredit,
		strings.TrimSpace(r.FormValue("control_no")),
		NormalizeRIF(r.FormValue("supplier_rif")),
		orDefault(r.FormValue("warehouse_id"), "WH-MAIN"),
		currencyCode, tasaBCV, totalAmountUSD); err != nil {
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

	if len(payAmounts) > 0 && len(payTypes) > 0 {
		if payTypes[0] == "0" {
			// full credit: Credit purchaseCode, RevCodde=supplierCode
			sub := s.subcodeID(r, supplierID, 4)
			vno := nextVoucherNoTx(tx, "JV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: purchaseCOA, RevCodde: pre.SupplierCode,
				Narration: narration, Comment: comment,
				Credit: grandTotal, SubType: 4, SubCode: sub,
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
				sub := s.subcodeID(r, supplierID, 4)
				vno := nextVoucherNoTx(tx, "JV")
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
					COAID: purchaseCOA, RevCodde: pre.SupplierCode,
					Narration: narration, Comment: comment,
					Credit: dueAmount, SubType: 4, SubCode: sub,
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
		if sumPay > 0 {
			vno := nextVoucherNoTx(tx, "DV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "DV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: purchaseCOA, RevCodde: pre.CashCode,
				Narration: narration, Comment: comment,
				Credit: sumPay,
			}, createBy)
		}
		if dueAmount > 0 {
			sub := s.subcodeID(r, supplierID, 4)
			vno := nextVoucherNoTx(tx, "JV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", purchaseID),
				COAID: purchaseCOA, RevCodde: pre.SupplierCode,
				Narration: narration, Comment: comment,
				Credit: dueAmount, SubType: 4, SubCode: sub,
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
