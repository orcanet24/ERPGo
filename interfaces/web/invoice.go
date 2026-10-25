package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Invoice module (replicates modules/invoice) ----------

func (s *Server) registerInvoiceRoutes() {
	s.mux.HandleFunc("GET /invoice/list", s.invoiceList)
	s.mux.HandleFunc("/invoice/list/data", s.invoiceListData)
	s.mux.HandleFunc("GET /invoice/new", s.invoiceForm)
	s.mux.HandleFunc("GET /invoice/edit/{id}", s.invoiceEdit)
	s.mux.HandleFunc("POST /invoice/save", s.invoiceSave)
	s.mux.HandleFunc("POST /invoice/delete/{id}", s.invoiceDelete)
	s.mux.HandleFunc("GET /invoice/details/{id}", s.invoiceDetails)
	// POS web eliminado — el POS ahora es un binario independiente.
	// Las rutas que quedaban aquí (/invoice/pos, /pos/admin) eran del POS web
	// viejo. El binario POS se comunica vía API (/api/pos/*) y gRPC.
	s.mux.HandleFunc("GET /invoice/next-number", s.invoiceNextNumber)
	s.mux.HandleFunc("GET /invoice/walking-customer", s.walkingCustomer)
}

func (s *Server) invoiceList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "invoice_list.html", map[string]interface{}{"Title": "Gestionar Facturas"})
}

func (s *Server) invoiceListData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE a.invoice_id LIKE ? OR c.customer_name LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM invoice").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM invoice a JOIN customer_information c ON c.customer_id=a.customer_id"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.invoice_id, a.invoice, a.customer_id, IFNULL(c.customer_name,''),
			a.total_amount, a.paid_amount, a.due_amount, a.date, a.is_draft, a.status
		FROM invoice a
		JOIN customer_information c ON c.customer_id = a.customer_id
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
		var iid, custID int64
		var cname, date, draft sql.NullString
		var total, paid, due float64
		var status int
		var invoiceNo int64
		if err := rows.Scan(&iid, &invoiceNo, &custID, &cname, &total, &paid, &due, &date, &draft, &status); err != nil {
			_ = err
		}
		list = append(list, map[string]interface{}{
			"id": iid, "invoice_no": invoiceNo, "customer_id": custID, "customer": cname.String,
			"total": money(total), "paid": money(paid), "due": money(due),
			"date": date.String, "draft": draft.String, "status": status,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// iid2 is a no-op placeholder (kept for scan clarity).
func iid2(p *int64) *int64 { return p }

func (s *Server) invoiceForm(w http.ResponseWriter, r *http.Request) {
	methods := s.paymentMethods(r)
	canCreate := s.hasPermission(r, "add_customer", "create") || s.hasPermission(r, "manage_customer", "create")
	s.render(w, r, "invoice_form.html", map[string]interface{}{
		"Title":            "Nueva Factura",
		"Methods":          methods,
		"CanCreateCustomer": canCreate,
	})
}

// paymentMethods: cash/bank accounts (like pmethod_dropdown).
func (s *Server) paymentMethods(r *http.Request) []map[string]interface{} {
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
	return methods
}

func (s *Server) invoiceEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var iid, custID int64
	var date, details, draft sql.NullString
	var total, paid, due, discount, vat, tax, shipping float64
	var paymentType int
	err := s.dbFor(r).QueryRow(`SELECT invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
		invoice_discount, total_vat_amnt, total_tax, shipping_cost, invoice_details, is_draft, payment_type
		FROM invoice WHERE invoice_id = ?`, id).
		Scan(&iid, &custID, &date, &total, &paid, &due, &discount, &vat, &tax, &shipping, &details, &draft, &paymentType)
	if err != nil {
		http.Error(w, "Factura no encontrada", 404)
		return
	}
	var cname sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT customer_name FROM customer_information WHERE customer_id=?`, custID).Scan(&cname)

	type drow struct {
		ProductID, ProductName, Model string
		Qty, Rate, Total              float64
	}
	drows := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), d.quantity, d.rate, d.total_price
		FROM invoice_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.invoice_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var d drow
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Qty, &d.Rate, &d.Total)
			drows = append(drows, d)
		}
		rows.Close()
	}

	s.render(w, r, "invoice_form.html", map[string]interface{}{
		"Title":    "Editar Factura #" + id,
		"EditID":   iid,
		"Customer": custID,
		"CName":    cname.String,
		"IDate":    date.String,
		"Grand":    total,
		"Paid":     paid,
		"Due":      due,
		"Discount": discount,
		"Vat":      vat,
		"Tax":      tax,
		"Shipping": shipping,
		"IDetails": details.String,
		"Details":  drows,
		"Methods":  s.paymentMethods(r),
	})
}

// invoiceSave replicates invoice_entry(): invoice no, header, vouchers
// (credit + COGS + tax), details, product price update, autoapprove.
func (s *Server) invoiceSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	customerID, _ := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	if customerID == 0 {
		jsonError(w, "Cliente inválido", 400)
		return
	}

	productIDs := r.Form["product_id"]
	quantities := r.Form["product_quantity"]
	rates := r.Form["product_rate"]
	totals := r.Form["total_price"]
	descriptions := r.Form["desc"]
	discounts := r.Form["discountvalue"]
	discPercents := r.Form["discount"]
	vatValues := r.Form["vatvalue"]
	vatPercents := r.Form["vatpercent"]

	if len(productIDs) == 0 {
		jsonError(w, "Agregue al menos un producto", 400)
		return
	}

	// ---- FISCAL: RIF del cliente (obligatorio para factura con crédito fiscal)
	rif := NormalizeRIF(r.FormValue("customer_rif"))
	if rif == "" && customerID > 0 {
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(rif,'') FROM customer_information WHERE customer_id = ?`, customerID).Scan(&rif)
	}
	if rif != "" {
		if ok, msg := ValidateRIF(rif); !ok {
			jsonError(w, "RIF inválido: "+msg, 400)
			return
		}
	}

	// ---- MÉTODOS DE PAGO VE + IGTF
	pmList := s.paymentMethodsActive(r)
	pmCodes := r.Form["pay_method"]
	payAmountsVE := r.Form["pamount_by_method"]
	var foreignTotal float64
	type pmPay struct {
		pm  *paymentMethodRow
		amt float64
	}
	var pmPays []pmPay
	for i, code := range pmCodes {
		m := methodByCode(pmList, code)
		if m == nil {
			continue
		}
		amt := 0.0
		if i < len(payAmountsVE) {
			amt = parseMoney(payAmountsVE[i])
		}
		if amt > 0 {
			pmPays = append(pmPays, pmPay{m, amt})
			if m.IsForeign {
				foreignTotal += amt
			}
		}
	}
	var openSession int64
	// NOTA: El módulo web de caja (cashsession.go) fue eliminado el 2026-10-20.
	// Las facturas web (invoice_form.html) ya NO se vinculan a turnos de caja.
	// El POS sigue asignando cash_session_id vía /api/pos/checkout (pos_api.go).

	saleType := r.FormValue("sale_type")
	// grand total: explicit form value, or computed from line totals
	grandTotal := parseMoney(r.FormValue("grand_total_price"))
	if grandTotal == 0 {
		grandTotal = parseMoney(r.FormValue("grand_total"))
	}
	if grandTotal == 0 {
		for _, tv := range r.Form["total_price"] {
			grandTotal += parseMoney(tv)
		}
	}
	paidAmount := parseMoney(r.FormValue("paid_amount"))
	dueAmount := parseMoney(r.FormValue("due_amount"))
	shipping := parseMoney(r.FormValue("shipping_cost"))
	totalDiscount := parseMoney(r.FormValue("total_discount"))
	invoiceDiscount := parseMoney(r.FormValue("invoice_discount"))
	totalVat := parseMoney(r.FormValue("total_vat_amnt"))
	totalTax := parseMoney(r.FormValue("total_tax"))

	payAmounts := r.Form["pamount_by_method"]
	payTypes := r.Form["multipaytype"]

	// payment type mapping (like PHP)
	paymentType := 1
	status := 1
	isDraft := "no"
	switch saleType {
	case "cash":
		paymentType = 1
		paidAmount = grandTotal
		dueAmount = 0
	case "card":
		paymentType = 2
		paidAmount = grandTotal
		dueAmount = 0
	case "credit_sale":
		paymentType = 3
		paidAmount = 0
		dueAmount = grandTotal
	case "multi-pay":
		paymentType = 4
		var sum float64
		for _, a := range payAmounts {
			sum += parseMoney(a)
		}
		paidAmount = sum
		dueAmount = 0
	case "draft":
		paymentType = 5
		paidAmount = 0
		dueAmount = 0
		isDraft = "yes"
		status = 0
	}

	if len(payTypes) == 0 {
		payTypes = []string{"0"}
	}
	isCredit := payTypes[0] == "0"

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// La fecha del documento se resuelve AQUÍ y no más abajo porque decide dos
	// cosas que van antes de numerar: si el período está abierto y con qué fecha
	// se asienta.
	date := orDefault(r.FormValue("invoice_date"), today())

	// Período cerrado: un mes ya firmado no admite documentos nuevos.
	//
	// La comprobación va ANTES de numerar, porque el número fiscal que se
	// consume no se devuelve: si se rechazara después, la serie quedaría con un
	// hueco por cada intento en un mes cerrado.
	if err := s.exigirPeriodoAbiertoTx(tx, date); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	// Número de factura: se RESERVA con bloqueo en el contador compartido.
	//
	// ANTES: `SELECT IFNULL(MAX(invoice),999)+1` sin bloqueo. Que la
	// transacción fuera correcta no bastaba: el SELECT no bloquea, así que dos
	// ventas simultáneas leían el mismo máximo y las dos se quedaban con el
	// mismo número de factura.
	var invoiceNo int64
	invoiceNo, err = s.reservarNumeroFacturaTx(tx)
	if err != nil {
		jsonError(w, "No se pudo reservar el número de factura: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// fixed vs dynamic tax
	isFixed, isDynamic := 0, 0
	paidTax := totalTax
	var fixedTax, dynamicTax int
	_ = s.dbFor(r).QueryRow(`SELECT fixed_tax, dynamic_tax FROM vat_tax_setting ORDER BY id LIMIT 1`).Scan(&fixedTax, &dynamicTax)
	if fixedTax == 1 {
		isFixed, isDynamic = 1, 0
		paidTax = totalVat
	} else if dynamicTax == 1 {
		isFixed, isDynamic = 0, 1
		paidTax = totalTax
	}

	// (la fecha del documento se resolvió arriba, antes de numerar)

	// ---- MULTI-MONEDIA: Tasa BCV del día
	currencyCode := orDefault(r.FormValue("currency_code"), "VES")
	tasaBCV := 1.0
	totalUSD := 0.0
	tasaFuente := "BCV"
	if currencyCode == "VES" {
		tasaVal, fuente, err := s.ObtenerTasaHoy(tx, date, "USD")
		if err == nil && tasaVal > 0 {
			tasaBCV = tasaVal
			tasaFuente = fuente
		}
		// Calcular total en USD
		if grandTotal > 0 {
			usd, _, _ := s.ConvertirVESaUSD(tx, grandTotal, date)
			totalUSD = usd
		}
	} else if currencyCode == "USD" {
		tasaVal, fuente, err := s.ObtenerTasaHoy(tx, date, "USD")
		if err == nil && tasaVal > 0 {
			tasaBCV = tasaVal
			tasaFuente = fuente
		}
		totalUSD = grandTotal
	}

	// ---- FISCAL: control fiscal + IGTF + serie
	fs := s.loadFiscalSettings(r)
	var igtfAmount float64
	if fs.IGTFEnabled && foreignTotal > 0 {
		igtfAmount = round2(foreignTotal * fs.IGTFRate / 100.0)
	}
	if igtfAmount > 0 {
		grandTotal = round2(grandTotal + igtfAmount)
		if saleType == "cash" || saleType == "card" {
			paidAmount = grandTotal
			dueAmount = 0
		}
		// Recalcular USD si aplica
		if currencyCode == "VES" {
			usd, _, _ := s.ConvertirVESaUSD(tx, grandTotal, date)
			totalUSD = usd
		}
	}

	// Balance defensivo: en multi-pay lo cobrado + por cobrar debe cubrir el total
	if saleType == "multi-pay" && round2(paidAmount+dueAmount) != grandTotal {
		dueAmount = round2(grandTotal - paidAmount)
		if dueAmount < 0 {
			dueAmount = 0
		}
	}
	doc, err := s.nextFiscalDoc(tx, "FACT")
	if err != nil {
		jsonError(w, "No se pudo asignar número de control fiscal: "+err.Error(), 500)
		return
	}

	// header
	res, err := tx.Exec(`
		INSERT INTO invoice (invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
			prevous_due, shipping_cost, invoice, invoice_discount, total_discount, total_vat_amnt,
			total_tax, invoice_details, is_credit, is_fixed, is_draft, is_dynamic, status, payment_type, sales_by,
			control_no, customer_rif, series, igtf_amount, igtf_rate, cash_session_id, warehouse_id,
			currency_code, exchange_rate, total_amount_usd, exchange_rate_source)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		invoiceNo, customerID, date, grandTotal, paidAmount, dueAmount,
		parseMoney(r.FormValue("previous")), shipping, invoiceNo, invoiceDiscount, totalDiscount,
		totalVat, totalTax, "ERP Go", isCredit, isFixed, isDraft, isDynamic, status, paymentType, createBy,
		doc.ControlNo, rif, doc.Series, igtfAmount, fs.IGTFRate, openSession, parseMoney(r.FormValue("warehouse_id")),
		currencyCode, tasaBCV, totalUSD, tasaFuente)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	invInsertID, _ := res.LastInsertId()

	// Auditoría de tasa aplicada
	if currencyCode == "VES" && tasaBCV > 0 {
		s.RegistrarAuditoriaTasa(tx, date, "USD", tasaBCV, tasaFuente, createBy, int(invInsertID))
	}

	// purchase average value for COGS (like PHP: Avg(rate) * qty)
	//
	// POR QUÉ HAY QUE MIRAR TRES FUENTES Y NO UNA.
	//
	// El promedio de `product_purchase_details` es el costo de lo COMPRADO. Es
	// correcto para lo que entra por la puerta del proveedor, y es 0 para todo
	// lo que la empresa FABRICA. Una carnicería no compra filete: compra una
	// canal y la despieza. El corte no tiene renglón de compra —tiene lote, que
	// es otra cosa— así que el promedio daba 0 y el asiento de costo de ventas
	// salía a 0,00: el libro "cuadraba" con 840 asientos a cero y ningún margen
	// por medir. El costo de un corte existe, está en `product_lot.unit_cost`,
	// y es el número que el motor de costos ya calcula al despiezar.
	//
	// El orden importa y es de mayor a menor confianza:
	//
	//	1. `product_lot.unit_cost` — lo que costó la mercadería que SALE. Es el
	//	   correcto para un despacho por FEFO, que es lo que hace una venta.
	//	2. `product_purchase_details.rate` — promedio histórico de compras.
	//	3. `product_information.cost` — lo que se escribió a mano al catalogar.
	//
	// Los tres pueden dar 0 legitimately: un producto regalado, o una factura
	// de cortesía. En ese caso el asiento va a 0, que es lo correcto, y no un
	// costo inventado.
	var purchaseValue float64
	for i, pid := range productIDs {
		qty := 1.0
		if i < len(quantities) {
			qty = parseMoney(quantities[i])
		}
		purchaseValue += costoVentaDeProducto(tx, pid) * qty
	}

	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	narration := "Sales Voucher"
	comment := "Sales Voucher for customer"
	revSales := pre.SalesCode

	// 1) credit voucher: debit customer (or payment methods)
	if len(payAmounts) > 0 && len(payTypes) > 0 {
		if payTypes[0] == "0" {
			// credit sale: Debit customerCode
			sub := s.subcodeID(r, customerID, 3)
			vno := nextVoucherNoTx(tx, isCreditVType(isCredit))
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: isCreditVType(isCredit), ReferenceNo: fmt.Sprintf("%d", invoiceNo),
				COAID: pre.CustomerCode, RevCodde: revSales,
				Narration: narration, Comment: comment,
				Debit: grandTotal, SubType: 3, SubCode: sub,
			}, createBy)
		} else if len(pmPays) > 0 {
			// FISCAL VE: métodos de pago venezolanos (payment_method.coa_id)
			for _, pp := range pmPays {
				coaid := pp.pm.COAID
				if !coaid.Valid {
					coaid = sql.NullInt64{Int64: pre.CashCode, Valid: true}
				}
				vno := nextVoucherNoTx(tx, isCreditVType(isCredit))
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: isCreditVType(isCredit), ReferenceNo: fmt.Sprintf("%d", invoiceNo),
					COAID: coaid.Int64, RevCodde: revSales,
					Narration: narration, Comment: pp.pm.Name,
					Debit: pp.amt,
				}, createBy)
			}
			// El cobro real al cliente incluye el IGTF percibido
			if igtfAmount > 0 {
				sub := s.subcodeID(r, customerID, 3)
				vno := nextVoucherNoTx(tx, "JV")
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
					COAID: pre.CustomerCode, RevCodde: pre.TaxCode,
					Narration: "IGTF factura " + fmt.Sprintf("%d", invoiceNo), Comment: "Percepción IGTF",
					Debit: igtfAmount, SubType: 3, SubCode: sub,
				}, createBy)
			}
			if dueAmount > 0 {
				sub := s.subcodeID(r, customerID, 3)
				vno := nextVoucherNoTx(tx, "JV")
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
					COAID: pre.CustomerCode, RevCodde: revSales,
					Narration: narration, Comment: comment,
					Debit: dueAmount, SubType: 3, SubCode: sub,
				}, createBy)
			}
		} else {
			// El select del formulario manda el ID del método de pago
			// (payment_method.id): sin traducirlo, el débito caía en
			// acc_coa 1, 2, 3… en vez de en la cuenta real del método
			// (efectivo=101, punto/transferencia=10205…).
			for i, pt := range payTypes {
				mid, _ := strconv.ParseInt(pt, 10, 64)
				var coaSQL sql.NullInt64
				_ = tx.QueryRow(`SELECT coa_id FROM payment_method WHERE id = ? AND is_active = 1`, mid).Scan(&coaSQL)
				coaid := coaSQL.Int64
				if coaid == 0 {
					coaid = pre.CashCode
				}
				amount := parseMoney(payAmounts[i])
				vno := nextVoucherNoTx(tx, isCreditVType(isCredit))
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: isCreditVType(isCredit), ReferenceNo: fmt.Sprintf("%d", invoiceNo),
					COAID: coaid, RevCodde: revSales,
					Narration: narration, Comment: comment,
					Debit: amount,
				}, createBy)
			}
			if dueAmount > 0 {
				sub := s.subcodeID(r, customerID, 3)
				vno := nextVoucherNoTx(tx, "JV")
				_ = s.insertVoucherRowTx(r, tx, VoucherRow{
					VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
					COAID: pre.CustomerCode, RevCodde: revSales,
					Narration: narration, Comment: comment,
					Debit: dueAmount, SubType: 3, SubCode: sub,
				}, createBy)
			}
		}
	} else if grandTotal > 0 {
		// POR QUÉ ESTA RAMA EXISTE.
		//
		// Todo el bloque de arriba está condicionado a que lleguen filas de
		// método de pago (`pamount_by_method`). Esas filas solo las manda el
		// multi-pago: en `invoice_form.html` la sección de pagos se DESACTIVA
		// para "contado" y para "venta a crédito", para que una fila vieja no
		// se cuele pegada al tipo de venta. Consecuencia: una venta de contado o
		// a crédito no llegaba NINGÚN asiento de ingreso. Solo se contabilizaba
		// el costo de ventas, y como el costo salía a cero, la factura entera
		// no dejaba ni un solo número en el libro.
		//
		// O sea: el ERP no solo fallaba en cuadrar, es que una venta normal no
		// se veía en la contabilidad. El estado de resultados nacía vacío y el
		// informe de ingresos no tenía de dónde sacar nada.
		//
		// Aquí se asienta lo que la propia factura dice: lo cobrado va a caja y
		// lo pendiente, al cliente. Sin inventar nada: los importes son los
		// mismos `paid_amount` y `due_amount` que se guardaron en la cabecera.
		vtype := isCreditVType(isCredit)
		if paidAmount > 0 {
			vno := nextVoucherNoTx(tx, vtype)
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: vtype, ReferenceNo: fmt.Sprintf("%d", invoiceNo),
				COAID: pre.CashCode, RevCodde: revSales,
				Narration: narration, Comment: comment,
				Debit: paidAmount,
			}, createBy)
		}
		if dueAmount > 0 {
			sub := s.subcodeID(r, customerID, 3)
			vno := nextVoucherNoTx(tx, "JV")
			_ = s.insertVoucherRowTx(r, tx, VoucherRow{
				VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
				COAID: pre.CustomerCode, RevCodde: revSales,
				Narration: narration, Comment: comment,
				Debit: dueAmount, SubType: 3, SubCode: sub,
			}, createBy)
		}
	}

	// 2) COGS voucher: Debit COGS, RevCodde=inventory
	vnoGoods := nextVoucherNoTx(tx, "JV")
	_ = s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vnoGoods, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
		COAID: pre.COGSCode, RevCodde: pre.InventoryCode,
		Narration: "Sales cost of goods Voucher", Comment: "Sales cost of goods Voucher for customer",
		Debit: purchaseValue,
	}, createBy)

	// 3) tax voucher
	if paidTax > 0 {
		vnoTax := nextVoucherNoTx(tx, "JV")
		_ = s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vnoTax, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
			COAID: pre.TaxCode, RevCodde: pre.ProvStateTaxCode,
			Narration: "Tax for Sales Voucher", Comment: "Tax for Sales Voucher for customer",
			Debit: paidTax,
		}, createBy)
	}

	// 3.c) Registro de pagos por método VE (para caja X/Z y reportes)
	// invoice_payment_summary.invoice_id referencia invoice.id (PK interna)
	var sessionID sql.NullInt64
	if openSession > 0 {
		sessionID = sql.NullInt64{Int64: openSession, Valid: true}
	}
	for _, pp := range pmPays {
		amountUSD := 0.0
		if currencyCode == "VES" && tasaBCV > 0 {
			amountUSD = round2(pp.amt / tasaBCV)
		} else if currencyCode == "USD" {
			amountUSD = pp.amt
		}
		if _, err := tx.Exec(`INSERT INTO invoice_payment_summary (invoice_id, cash_session_id, method_id, amount, amount_usd, exchange_rate, currency_code)
			VALUES (?,?,?,?,?,?,?)`, invInsertID, sessionID, pp.pm.ID, pp.amt, amountUSD, tasaBCV, currencyCode); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// Configuración de flujo de inventario
	fc := s.getInventoryFlowConfig(r)

	// details + product price update
	for i, pid := range productIDs {
		qty, rate, total := 1.0, 0.0, 0.0
		disc, discP, vatV, vatP := 0.0, 0.0, 0.0, 0.0
		desc := ""
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
		if i < len(vatValues) {
			vatV = parseMoney(vatValues[i])
		}
		if i < len(vatPercents) {
			vatP = parseMoney(vatPercents[i])
		}
		if i < len(descriptions) {
			desc = descriptions[i]
		}

		pidInt, _ := strconv.Atoi(pid)

		// Validar stock si el producto es físico y el modo es invoice
		if fc.DeductionMode == "invoice" && s.requiresStockMovement(r, pidInt) {
			stock := stockForProductWithoutMovementTx(tx, pidInt)
			if stock < qty {
				jsonError(w, fmt.Sprintf("Stock insuficiente para producto %s: disponible %.2f, solicitado %.2f", pid, stock, qty), 400)
				return
			}
		}

		if _, err := tx.Exec(`
			INSERT INTO invoice_details (invoice_details_id, invoice_id, product_id, serial_no, description,
				quantity, rate, discount, discount_per, price_list_id, discount_pct, discount_amount, base_price,
				vat_amnt, vat_amnt_per, tax, paid_amount, due_amount,
				total_price, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?)`,
			generator(15), invInsertID, pid, "", desc, qty, rate, disc, fmt.Sprintf("%.2f", discP),
			1, fmt.Sprintf("%.2f", discP), disc, rate,
			vatV, fmt.Sprintf("%.2f", vatP), paidAmount, dueAmount, total, status); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}

		// Crear stock_movement si el producto es físico y el modo descuenta al facturar
		if fc.DeductionMode == "invoice" && s.requiresStockMovement(r, pidInt) {
			_, err := tx.Exec(`
				INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
					reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
				VALUES ('sale', ?, 'WH-MAIN', ?, ?, 'invoice', ?, CONCAT('INV-',?), ?, 1, NOW(), NOW())`,
				pidInt, qty, rate, invInsertID, invInsertID, date)
			if err != nil {
				jsonError(w, "Error registrando movimiento de stock: "+err.Error(), 500)
				return
			}
		}
		// FISCAL: redondeo del renglón a 2 decimales (norma SENIAT)
		if len(totals) > i {
			_ = round2(parseMoney(totals[i]))
		}
		// POR QUÉ YA NO SE PISA EL PRECIO DEL CATÁLOGO.
		//
		// Aquí había un `UPDATE product_information SET price = <precio de la
		// factura>` en cada venta. Su intención era "recordar el último precio",
		// pero `product_information.price` es el PRECIO DE MOSTRADOR, y el ERP
		// vende también al mayor, más barato. Una venta al mayor escribía el
		// precio mayor sobre el de mostrador, y a partir de ahí TODAS las
		// ventas siguientes —también de mostrador— salían al precio rebajado.
		//
		// El efecto era una escalera hacia abajo sin fondo: cada mes el precio
		// caía otro 18% y el margen se volvía negativo sin que nada lo
		// anunciara. En la carnicería Lara, el corte de res empezó en 16,90
		// Bs/kg y terminó en 13,86, que es exactamente 16,90 × 0,82: el precio
		// al mayor, escribir encima del de mostrador.
		//
		// El precio de venta lo decide `product_price_list` (listas) o el
		// catálogo; una factura no lo cambia. Solo se rellena si el producto
		// aún no tiene precio, para que un producto nuevo facture algo y no
		// cero por un campo vacío.
		_, _ = tx.Exec(`UPDATE product_information SET price = ? WHERE product_id = ? AND (price IS NULL OR price = 0)`,
			fmt.Sprintf("%.2f", rate), pid)
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// autoapprove vouchers
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		rows, _ := s.dbFor(r).Query(`SELECT VNo FROM acc_vaucher WHERE referenceNo = ? AND status = 0`, fmt.Sprintf("%d", invoiceNo))
		var vnos []string
		if rows != nil {
			for rows.Next() {
				var v string
				_ = rows.Scan(&v)
				vnos = append(vnos, v)
			}
			rows.Close()
		}
		for _, v := range vnos {
			_ = s.approveVoucher(r, v, createBy)
		}
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Factura registrada", "invoice_id": invoiceNo,
		"control_no": doc.ControlNo, "igtf": igtfAmount,
	})
}

// posBarcodeLookup: busca producto por código de barras (exacto) o nombre.
func (s *Server) posBarcodeLookup(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.FormValue("code"))
	if code == "" {
		jsonError(w, "Código vacío", 400)
		return
	}
	var pid, name, price string
	var stock float64
	err := s.dbFor(r).QueryRow(`
		SELECT a.product_id, a.product_name, a.price
		FROM product_information a
		WHERE a.barcode = ? OR a.product_name = ?
		LIMIT 1`, code, code).Scan(&pid, &name, &price)
	if err != nil {
		jsonError(w, "Producto no encontrado", 404)
		return
	}
	pidInt, _ := strconv.Atoi(pid)
	stock = s.stockForProduct(r, pidInt)
	writeJSON(w, map[string]interface{}{
		"id": pid, "name": name, "price": price, "stock": stock,
	})
}

// isCreditVType maps isCredit flag to voucher type (like PHP).
func isCreditVType(isCredit bool) string {
	if isCredit {
		return "JV"
	}
	return "CV"
}

// costoVentaDeProducto devuelve lo que le cuesta al ERP una unidad del producto,
// para el asiento de costo de ventas.
//
// Hace falta porque el costo de un producto no vive en un solo sitio, y el lugar
// "natural" (`product_purchase_details.rate`) solo conoce lo que se COMPRÓ. Lo
// que la empresa produce —el corte de una canal, el pan de una panadería, el
// ensamble de un taller— no pasa por una compra y sí sale del almacén con un
// costo que alguien calculó: el del lote.
//
// La primera fuente es el LOTE y no el promedio de compras porque una venta
// consume lotes por FEFO, y el costo que sale del almacén es el de ese lote. El
// promedio histórico es la segunda mejor respuesta, y `product_information.cost`
// es la última: lo que se escribió a mano al catalogar.
//
// Los tres pueden dar cero de verdad, y entonces el asiento va a cero, que es
// lo correcto. Devolver 0 es mejor que inventar un costo.
func costoVentaDeProducto(tx *sql.Tx, productID string) float64 {
	var costo float64

	// 1) el costo del lote: lo que costó la mercadería que realmente sale
	_ = tx.QueryRow(`SELECT IFNULL(AVG(unit_cost),0) FROM product_lot
		WHERE product_id = ? AND quantity_received > 0`, productID).Scan(&costo)
	if costo > 0 {
		return costo
	}

	// 2) el promedio histórico de lo comprado
	_ = tx.QueryRow(`SELECT IFNULL(AVG(rate),0) FROM product_purchase_details
		WHERE product_id = ?`, productID).Scan(&costo)
	if costo > 0 {
		return costo
	}

	// 3) el costo del catálogo
	_ = tx.QueryRow(`SELECT IFNULL(cost,0) FROM product_information
		WHERE product_id = ?`, productID).Scan(&costo)
	return costo
}

// invoiceDelete elimina un BORRADOR. Nada más.
//
// ANTES: `DELETE FROM invoice WHERE invoice_id = ?` sin mirar el estado, más
// `DELETE` de sus renglones y de sus asientos. Un documento fiscal emitido
// desaparecía con su número de control: la serie quedaba con un hueco, el libro
// perdía el rastro y la corrección no dejaba huella —los tres indicios que una
// revisión fiscal lee como documento suprimido—. El propio módulo de notas de
// crédito dice "las facturas NUNCA se borran ni se editan retroactivamente".
//
// El camino correcto para dejar sin efecto una factura emitida es una NOTA DE
// CRÉDITO: lleva su propio número de control, referencia el documento que
// corrige y se asienta sola. Aquí se responde con eso, no con un borrado.
//
// El asiento del borrador se retira SOLO si está pendiente (isApproved = 0).
// Un asiento publicado no se borra nunca: eso rompería el libro, y si un
// borrador llegara a tenerlo, el borrado se rechaza y se pide la reversión con
// asiento.
func (s *Server) invoiceDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var draft, control sql.NullString
	err = tx.QueryRow(`SELECT IFNULL(is_draft,'no'), IFNULL(control_no,'') FROM invoice WHERE invoice_id = ?`, id).
		Scan(&draft, &control)
	if err == sql.ErrNoRows {
		jsonError(w, "Factura no encontrada", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if !strings.EqualFold(strings.TrimSpace(draft.String), "yes") {
		jsonError(w, "Una factura emitida no se elimina. Use una nota de credito (con su numero de control) para dejarla sin efecto", http.StatusConflict)
		return
	}

	// Un borrador no debería tener asientos publicados. Si los tiene, no se
	// borra nada: primero se revierten con asiento, que es lo que deja rastro.
	var publicados int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM acc_transaction WHERE referenceNo = ?`, id).Scan(&publicados); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if publicados > 0 {
		jsonError(w, "El borrador tiene asientos publicados: anulelos con un asiento de reversion antes de eliminarlo", http.StatusConflict)
		return
	}

	// La condición va EN el DELETE, no solo en el if de arriba: entre la lectura
	// y el borrado nadie lo convierte en documento emitido sin que nos enteremos.
	res, err := tx.Exec(`DELETE FROM invoice WHERE invoice_id = ? AND is_draft = 'yes'`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n == 0 {
		jsonError(w, "La factura dejo de ser un borrador: no se elimina", http.StatusConflict)
		return
	}

	// Renglones y asientos PENDIENTES del borrador. Nunca los publicados (ya se
	// comprobó arriba que no hay).
	if _, err := tx.Exec(`DELETE FROM invoice_details WHERE invoice_id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if _, err := tx.Exec(`DELETE FROM acc_vaucher WHERE referenceNo = ? AND isApproved = 0`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), clientIP(r), "delete", "invoice", "invoice", id,
		map[string]interface{}{"is_draft": "yes", "control_no": control.String}, nil)

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Borrador eliminado", "redirect": saveRedirect(r, "/invoice/list")})
}

func (s *Server) invoiceDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var iid, custID int64
	var date, details, ctrl, custRIF, series sql.NullString
	var igtf float64
	var total, paid, due, discount, vat, tax, shipping float64
	var paymentType int
	err := s.dbFor(r).QueryRow(`SELECT invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
		invoice_discount, total_vat_amnt, total_tax, shipping_cost, invoice_details, payment_type,
		IFNULL(control_no,''), IFNULL(customer_rif,''), series, IFNULL(igtf_amount,0)
		FROM invoice WHERE invoice_id = ?`, id).
		Scan(&iid, &custID, &date, &total, &paid, &due, &discount, &vat, &tax, &shipping, &details, &paymentType,
			&ctrl, &custRIF, &series, &igtf)
	if err != nil {
		http.Error(w, "Factura no encontrada", 404)
		return
	}
	var cname, cmobile, caddr sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT customer_name, IFNULL(customer_mobile,''), IFNULL(customer_address,'')
		FROM customer_information WHERE customer_id=?`, custID).Scan(&cname, &cmobile, &caddr)

	type drow struct {
		ProductID, ProductName, Model, Unit string
		Qty, Rate, Total                    float64
	}
	drows := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), IFNULL(p.unit,''),
			d.quantity, d.rate, d.total_price
		FROM invoice_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.invoice_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var d drow
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Unit, &d.Qty, &d.Rate, &d.Total)
			drows = append(drows, d)
		}
		rows.Close()
	}

	st := s.settingsFor(r)
	comp := st.Company
	s.render(w, r, "invoice_details.html", map[string]interface{}{
		"Title":     "Factura #" + id,
		"IID":       iid,
		"CName":     cname.String,
		"CMobile":   cmobile.String,
		"CAddr":     caddr.String,
		"CRIF":      custRIF.String,
		"IDate":     date.String,
		"Grand":     total,
		"Paid":      paid,
		"Due":       due,
		"Discount":  discount,
		"Vat":       vat,
		"Tax":       tax,
		"Shipping":  shipping,
		"IGTF":      igtf,
		"ControlNo": ctrl.String,
		"Series":    series.String,
		"IDetails":  details.String,
		"Details":   drows,
		"Currency":  st.Web.Currency,
		"Company":   comp,
	})
}

// ---------- POS ----------

func (s *Server) posForm(w http.ResponseWriter, r *http.Request) {
	// categories for grid
	cats := []map[string]interface{}{}
	rows, _ := s.dbFor(r).Query(`SELECT category_id, category_name FROM product_category WHERE status=1 ORDER BY category_name`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var name string
			_ = rows.Scan(&id, &name)
			cats = append(cats, map[string]interface{}{"id": id, "name": name})
		}
		rows.Close()
	}
	// métodos de pago VE con IGTF para el JS
	type pmJSON struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		IsForeign bool   `json:"is_foreign"`
	}
	pms := []pmJSON{}
	for _, m := range s.paymentMethodsActive(r) {
		pms = append(pms, pmJSON{Code: m.Code, Name: m.Name, IsForeign: m.IsForeign})
	}
	pmBytes, _ := json.Marshal(pms)
	fs := s.loadFiscalSettings(r)
	s.render(w, r, "pos.html", map[string]interface{}{
		"Title":       "POS",
		"Categories":  cats,
		"MethodsJSON": string(pmBytes),
		"IGTFRate":    fs.IGTFRate,
		"IGTFEnabled": fs.IGTFEnabled,
	})
}

// posProducts returns products (optionally by category) for the POS grid.
func (s *Server) posProducts(w http.ResponseWriter, r *http.Request) {
	cat := r.FormValue("category_id")
	term := strings.TrimSpace(r.FormValue("term"))

	q := `
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), a.price, IFNULL(a.unit,''),
			IFNULL(a.image,'')
		FROM product_information a WHERE a.status = 1`
	args := []interface{}{}
	if cat != "" {
		q += " AND a.category_id = ?"
		args = append(args, cat)
	}
	if term != "" {
		q += " AND (a.product_name LIKE ? OR a.product_model LIKE ?)"
		args = append(args, "%"+term+"%", "%"+term+"%")
	}
	q += " ORDER BY a.product_name LIMIT 100"

	rows, err := s.dbFor(r).Query(q, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var pid, name, model, price, unit, image sql.NullString
		_ = rows.Scan(&pid, &name, &model, &price, &unit, &image)
		img := image.String
		if img == "" {
			img = "/static/image/product.png"
		}
		pidInt, _ := strconv.Atoi(pid.String)
		stock := s.stockForProduct(r, pidInt)
		out = append(out, map[string]interface{}{
			"id": pid.String, "name": name.String, "model": model.String,
			"price": price.String, "unit": unit.String, "image": img, "stock": stock,
		})
	}
	writeJSON(w, out)
}

// invoiceNextNumber: next invoice number for the form (AJAX).
func (s *Server) invoiceNextNumber(w http.ResponseWriter, r *http.Request) {
	var n int64
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(MAX(invoice),999)+1 FROM invoice`).Scan(&n)
	writeJSON(w, map[string]interface{}{"next": n})
}

// walkingCustomer: the default walk-in customer (like walking_customer()).
func (s *Server) walkingCustomer(w http.ResponseWriter, r *http.Request) {
	var id int64
	var name sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT customer_id, customer_name FROM customer_information
		WHERE customer_name = 'Walking Customer' OR customer_name = 'Cliente Contado' LIMIT 1`).Scan(&id, &name)
	if err != nil {
		// create it
		tx, _ := s.dbFor(r).Begin()
		res, err2 := tx.Exec(`INSERT INTO customer_information (customer_name, customer_mobile, status, create_by)
			VALUES ('Walking Customer','',1,'1')`)
		if err2 == nil {
			cid, _ := res.LastInsertId()
			hc, _ := nextHeadCodeTx(tx, "113100", "1131000001")
			_, _ = tx.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, HeadLevel, IsActive, IsTransaction, IsGL,
				HeadType, IsBudget, IsDepreciation, DepreciationRate, customer_id, CreateBy, CreateDate, UpdateBy, UpdateDate, isSubType, subType)
				VALUES (?,?,?,?,1,1,0,'A',0,0,0,?,?,?,?,?,0,1)`, hc, fmt.Sprintf("%d-Walking Customer", cid), "Customer Receivable", 4, cid, "1", now(), "1", now())
			_ = tx.Commit()
			id = cid
			name = sql.NullString{String: "Walking Customer", Valid: true}
		}
	}
	writeJSON(w, map[string]interface{}{"id": id, "name": name.String})
}
