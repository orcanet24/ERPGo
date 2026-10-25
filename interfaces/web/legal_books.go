package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Legal Books module (Venezuelan statutory books) ----------

func (s *Server) registerLegalBooksRoutes() {
	s.mux.HandleFunc("GET /reports/book-purchases", s.bookPurchases)
	s.mux.HandleFunc("GET /reports/book-purchases/data", s.bookPurchasesData)
	s.mux.HandleFunc("GET /reports/book-purchases/print", s.bookPurchasesPrint)
	s.mux.HandleFunc("GET /reports/book-sales", s.bookSales)
	s.mux.HandleFunc("GET /reports/book-sales/data", s.bookSalesData)
	s.mux.HandleFunc("GET /reports/book-sales/print", s.bookSalesPrint)
	s.mux.HandleFunc("GET /reports/inventory-control", s.inventoryControl)
	s.mux.HandleFunc("GET /reports/inventory-control/data", s.inventoryControlData)
	s.mux.HandleFunc("GET /reports/inventory-control/print", s.inventoryControlPrint)
	s.mux.HandleFunc("GET /reports/vat-retention", s.vatRetention)
	s.mux.HandleFunc("GET /reports/vat-retention/data", s.vatRetentionData)
	s.mux.HandleFunc("POST /reports/vat-retention/save", s.vatRetentionSave)
	s.mux.HandleFunc("GET /reports/vat-retention/txt", s.vatRetentionTxt)
	s.mux.HandleFunc("GET /reports/islr-retention", s.islrRetention)
	s.mux.HandleFunc("GET /reports/islr-retention/data", s.islrRetentionData)
	s.mux.HandleFunc("POST /reports/islr-retention/save", s.islrRetentionSave)
	s.mux.HandleFunc("GET /reports/islr-retention/txt", s.islrRetentionTxt)
}

// ---------- Book Purchases (Libro de Compras IVA) ----------

func (s *Server) bookPurchases(w http.ResponseWriter, r *http.Request) {
	st := s.settingsFor(r)
	s.render(w, r, "book_purchases.html", map[string]interface{}{
		"Title":    "Libro de Compras (IVA)",
		"Month":    orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month()))),
		"Year":     orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year())),
		"Currency": st.Web.Currency,
	})
}

func (s *Server) bookPurchasesData(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT p.purchase_date, IFNULL(p.supplier_rif, IFNULL(s.rif,'')) AS rif, IFNULL(s.supplier_name,'') AS name,
			IFNULL(p.chalan_no,'') AS invoice_no, IFNULL(p.control_no,'') AS control_no,
			p.grand_total_amount - IFNULL(p.total_vat_amnt,0) AS base_amount,
			IFNULL(p.total_vat_amnt,0) AS vat_amount, 0 AS exempt, '' AS retention_no
		FROM product_purchase p
		LEFT JOIN supplier_information s ON s.supplier_id = p.supplier_id
		WHERE p.purchase_date BETWEEN ? AND ?
		ORDER BY p.purchase_date ASC, p.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	var totalBase, totalVat, totalExempt float64
	for rows.Next() {
		var d, rif, name, inv, ctrl, ret sql.NullString
		var base, vat, exempt float64
		_ = rows.Scan(&d, &rif, &name, &inv, &ctrl, &base, &vat, &exempt, &ret)
		list = append(list, map[string]interface{}{
			"date": d.String, "rif": rif.String, "name": name.String,
			"invoice_no": inv.String, "control_no": ctrl.String,
			"base_amount": base, "vat_amount": vat, "exempt": exempt,
			"retention_no": ret.String,
		})
		totalBase += base
		totalVat += vat
		totalExempt += exempt
	}
	writeJSON(w, map[string]interface{}{
		"data": list, "total_base": totalBase, "total_vat": totalVat,
		"total_exempt": totalExempt,
	})
}

func (s *Server) bookPurchasesPrint(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT p.purchase_date, IFNULL(p.supplier_rif, IFNULL(s.rif,'')) AS rif, IFNULL(s.supplier_name,'') AS name,
			IFNULL(p.chalan_no,'') AS invoice_no, IFNULL(p.control_no,'') AS control_no,
			p.grand_total_amount - IFNULL(p.total_vat_amnt,0) AS base_amount,
			IFNULL(p.total_vat_amnt,0) AS vat_amount, 0 AS exempt, '' AS retention_no
		FROM product_purchase p
		LEFT JOIN supplier_information s ON s.supplier_id = p.supplier_id
		WHERE p.purchase_date BETWEEN ? AND ?
		ORDER BY p.purchase_date ASC, p.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type prow struct {
		Date, Rif, Name, Invoice, Control, Retention string
		Base, Vat, Exempt                            float64
	}
	rowsData := []prow{}
	var totalBase, totalVat, totalExempt float64
	for rows.Next() {
		var d, rif, name, inv, ctrl, ret sql.NullString
		var base, vat, exempt float64
		_ = rows.Scan(&d, &rif, &name, &inv, &ctrl, &base, &vat, &exempt, &ret)
		rowsData = append(rowsData, prow{d.String, rif.String, name.String, inv.String, ctrl.String, ret.String, base, vat, exempt})
		totalBase += base
		totalVat += vat
		totalExempt += exempt
	}
	s.renderStandalone(r, w, "book_purchases_print.html", map[string]interface{}{
		"Title":       "Libro de Compras (IVA) - Imprimible",
		"Month":       month,
		"Year":        year,
		"Rows":        rowsData,
		"TotalBase":   totalBase,
		"TotalVat":    totalVat,
		"TotalExempt": totalExempt,
		"Currency":    s.settingsFor(r).Web.Currency,
	})
}

// ---------- Book Sales (Libro de Ventas IVA) ----------

func (s *Server) bookSales(w http.ResponseWriter, r *http.Request) {
	st := s.settingsFor(r)
	s.render(w, r, "book_sales.html", map[string]interface{}{
		"Title":    "Libro de Ventas (IVA)",
		"Month":    orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month()))),
		"Year":     orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year())),
		"Currency": st.Web.Currency,
	})
}

func (s *Server) bookSalesData(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT i.date, IFNULL(i.customer_rif, IFNULL(c.rif,'')) AS rif, IFNULL(c.customer_name,'') AS name,
			i.invoice AS invoice_no, IFNULL(i.control_no,'') AS control_no,
			i.total_amount - IFNULL(i.total_vat_amnt,0) - IFNULL(i.igtf_amount,0) AS base_amount,
			IFNULL(i.total_vat_amnt,0) AS vat_amount, 0 AS exempt, '' AS retention_no
		FROM invoice i
		LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.date BETWEEN ? AND ? AND i.is_draft = 'no' AND i.is_anulled = 0
		ORDER BY i.date ASC, i.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	var totalBase, totalVat, totalExempt float64
	for rows.Next() {
		var d, rif, name, inv, ctrl, ret sql.NullString
		var base, vat, exempt float64
		_ = rows.Scan(&d, &rif, &name, &inv, &ctrl, &base, &vat, &exempt, &ret)
		list = append(list, map[string]interface{}{
			"date": d.String, "rif": rif.String, "name": name.String,
			"invoice_no": inv.String, "control_no": ctrl.String,
			"base_amount": base, "vat_amount": vat, "exempt": exempt,
			"retention_no": ret.String,
		})
		totalBase += base
		totalVat += vat
		totalExempt += exempt
	}
	// FISCAL: notas de crédito del período (documentan la Col. L)
	notes := []map[string]interface{}{}
	var totalNC float64
	nrows, err := s.dbFor(r).Query(`
		SELECT `+s.sqlDateYMD("cn.date")+` AS cndate, cn.note_no, cn.control_no, IFNULL(cn.customer_rif,''),
			IFNULL(c.customer_name,''), cn.total_amount, cn.vat_amount, cn.reason
		FROM credit_note cn
		LEFT JOIN customer_information c ON c.customer_id = cn.customer_id
		WHERE cn.date BETWEEN ? AND ?
		ORDER BY cn.date ASC, cn.id ASC`, startDate, endDate)
	if err == nil {
		defer nrows.Close()
		for nrows.Next() {
			var d, ctrl, rif, name, reason sql.NullString
			var noteNo int64
			var total, vat float64
			if e := nrows.Scan(&d, &noteNo, &ctrl, &rif, &name, &total, &vat, &reason); e != nil {
				continue
			}
			notes = append(notes, map[string]interface{}{
				"date": d.String, "doc_type": "NC",
				"doc_no": fmt.Sprintf("NC-%08d", noteNo), "control_no": ctrl.String,
				"rif": rif.String, "name": name.String,
				"total": total, "vat": vat, "reason": reason.String,
			})
			totalNC += total
		}
	}
	writeJSON(w, map[string]interface{}{
		"data": list, "total_base": totalBase, "total_vat": totalVat,
		"total_exempt": totalExempt,
		"credit_notes": notes, "total_nc": totalNC,
	})
}

func (s *Server) bookSalesPrint(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT i.date, IFNULL(i.customer_rif, IFNULL(c.rif,'')) AS rif, IFNULL(c.customer_name,'') AS name,
			i.invoice AS invoice_no, IFNULL(i.control_no,'') AS control_no,
			i.total_amount - IFNULL(i.total_vat_amnt,0) - IFNULL(i.igtf_amount,0) AS base_amount,
			IFNULL(i.total_vat_amnt,0) AS vat_amount, 0 AS exempt, '' AS retention_no
		FROM invoice i
		LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.date BETWEEN ? AND ? AND i.is_draft = 'no' AND i.is_anulled = 0
		ORDER BY i.date ASC, i.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type srow struct {
		Date, Rif, Name, Invoice, Control, Retention string
		Base, Vat, Exempt                            float64
	}
	rowsData := []srow{}
	var totalBase, totalVat, totalExempt float64
	for rows.Next() {
		var d, rif, name, inv, ctrl, ret sql.NullString
		var base, vat, exempt float64
		_ = rows.Scan(&d, &rif, &name, &inv, &ctrl, &base, &vat, &exempt, &ret)
		rowsData = append(rowsData, srow{d.String, rif.String, name.String, inv.String, ctrl.String, ret.String, base, vat, exempt})
		totalBase += base
		totalVat += vat
		totalExempt += exempt
	}
	s.renderStandalone(r, w, "book_sales_print.html", map[string]interface{}{
		"Title":       "Libro de Ventas (IVA) - Imprimible",
		"Month":       month,
		"Year":        year,
		"Rows":        rowsData,
		"TotalBase":   totalBase,
		"TotalVat":    totalVat,
		"TotalExempt": totalExempt,
		"Currency":    s.settingsFor(r).Web.Currency,
	})
}

// ---------- Inventory Control (Kardex) ----------

func (s *Server) inventoryControl(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "inventory_control.html", map[string]interface{}{
		"Title":    "Control de Inventario (Kardex)",
		"Month":    orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month()))),
		"Year":     orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year())),
		"Currency": s.settingsFor(r).Web.Currency,
	})
}

func (s *Server) inventoryControlData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,'') AS model,
			IFNULL(a.unit,'') AS unit, a.price,
			IFNULL((SELECT SUM(total_amount) FROM product_purchase_details WHERE product_id = a.product_id),0) AS total_cost
		FROM product_information a
		WHERE a.status = 1
		ORDER BY a.product_name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var pid, name, model, unit, price sql.NullString
		var totalCost float64
		_ = rows.Scan(&pid, &name, &model, &unit, &price, &totalCost)
		pidInt, _ := strconv.Atoi(pid.String)
		stock := s.stockForProduct(r, pidInt)
		avgCost := 0.0
		if stock > 0 {
			avgCost = totalCost / stock
		}
		var entries, exits, adjustments float64
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(quantity),0) FROM product_purchase_details WHERE product_id = ?`, pid.String).Scan(&entries)
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(quantity),0) FROM invoice_details WHERE product_id = ?`, pid.String).Scan(&exits)
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(CASE WHEN type='add' THEN quantity ELSE -quantity END),0) FROM store_adjustment_details WHERE product_id = ?`, pid.String).Scan(&adjustments)
		list = append(list, map[string]interface{}{
			"product_id": pid.String, "name": name.String, "model": model.String,
			"unit": unit.String, "price": price.String,
			"entries": entries, "exits": exits, "adjustments": adjustments,
			"stock": stock, "avg_cost": avgCost,
		})
	}
	writeJSON(w, list)
}

func (s *Server) inventoryControlPrint(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,'') AS model,
			IFNULL(a.unit,'') AS unit, a.price,
			IFNULL((SELECT SUM(total_amount) FROM product_purchase_details WHERE product_id = a.product_id),0) AS total_cost
		FROM product_information a
		WHERE a.status = 1
		ORDER BY a.product_name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type krow struct {
		PID, Name, Model, Unit, Price string
		Entries, Exits, Adjustments   float64
		Stock, AvgCost                float64
	}
	rowsData := []krow{}
	for rows.Next() {
		var pid, name, model, unit, price sql.NullString
		var totalCost float64
		_ = rows.Scan(&pid, &name, &model, &unit, &price, &totalCost)
		pidInt, _ := strconv.Atoi(pid.String)
		stock := s.stockForProduct(r, pidInt)
		avgCost := 0.0
		if stock > 0 {
			avgCost = totalCost / stock
		}
		var entries, exits, adjustments float64
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(quantity),0) FROM product_purchase_details WHERE product_id = ?`, pid.String).Scan(&entries)
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(quantity),0) FROM invoice_details WHERE product_id = ?`, pid.String).Scan(&exits)
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(CASE WHEN type='add' THEN quantity ELSE -quantity END),0) FROM store_adjustment_details WHERE product_id = ?`, pid.String).Scan(&adjustments)
		rowsData = append(rowsData, krow{pid.String, name.String, model.String, unit.String, price.String, entries, exits, adjustments, stock, avgCost})
	}
	s.renderStandalone(r, w, "inventory_control_print.html", map[string]interface{}{
		"Title":    "Control de Inventario (Kardex) - Imprimible",
		"Rows":     rowsData,
		"Currency": s.settingsFor(r).Web.Currency,
	})
}

// ---------- VAT Retention ----------

func (s *Server) vatRetention(w http.ResponseWriter, r *http.Request) {
	st := s.settingsFor(r)
	supplierRows, _ := s.dbFor(r).Query(`SELECT supplier_id, supplier_id, supplier_name FROM supplier_information WHERE status=1 ORDER BY supplier_name`)
	var suppliers []map[string]interface{}
	if supplierRows != nil {
		defer supplierRows.Close()
		for supplierRows.Next() {
			var id, rif, name sql.NullString
			_ = supplierRows.Scan(&id, &rif, &name)
			suppliers = append(suppliers, map[string]interface{}{"id": id.String, "rif": rif.String, "name": name.String})
		}
	}
	purchaseRows, _ := s.dbFor(r).Query(`SELECT id, purchase_id, chalan_no, grand_total_amount, total_vat_amnt FROM product_purchase ORDER BY purchase_date DESC LIMIT 100`)
	var purchases []map[string]interface{}
	if purchaseRows != nil {
		defer purchaseRows.Close()
		for purchaseRows.Next() {
			var id int64
			var pid, chalan sql.NullString
			var total, vat float64
			_ = purchaseRows.Scan(&id, &pid, &chalan, &total, &vat)
			purchases = append(purchases, map[string]interface{}{"id": id, "purchase_id": pid.String, "chalan_no": chalan.String, "total": total, "vat": vat})
		}
	}
	s.render(w, r, "vat_retention.html", map[string]interface{}{
		"Title":     "Retenciones de IVA",
		"Month":     orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month()))),
		"Year":      orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year())),
		"Suppliers": suppliers,
		"Purchases": purchases,
		"Currency":  st.Web.Currency,
	})
}

func (s *Server) vatRetentionData(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT v.id, v.retention_no, v.date, IFNULL(s.supplier_name,'') AS supplier,
			v.invoice_no, v.base_amount, v.vat_amount, v.retention_percent, v.retention_amount, v.created_at
		FROM vat_retention v
		LEFT JOIN supplier_information s ON s.supplier_id = v.supplier_id
		WHERE v.date BETWEEN ? AND ?
		ORDER BY v.date ASC, v.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	var totalRetention float64
	for rows.Next() {
		var id int64
		var retNo, supplier, invoice, createdAt sql.NullString
		var d sql.NullString
		var base, vat, pct, amount float64
		_ = rows.Scan(&id, &retNo, &d, &supplier, &invoice, &base, &vat, &pct, &amount, &createdAt)
		list = append(list, map[string]interface{}{
			"id": id, "retention_no": retNo.String, "date": d.String,
			"supplier": supplier.String, "invoice_no": invoice.String,
			"base_amount": base, "vat_amount": vat,
			"retention_percent": pct, "retention_amount": amount,
			"created_at": createdAt.String,
		})
		totalRetention += amount
	}
	writeJSON(w, map[string]interface{}{"data": list, "total_retention": totalRetention})
}

func (s *Server) vatRetentionSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "No autorizado", 401)
		return
	}

	retentionNo := r.FormValue("retention_no")
	date := r.FormValue("date")
	purchaseID := parseInt64(r.FormValue("purchase_id"))
	supplierID := parseInt64(r.FormValue("supplier_id"))
	invoiceNo := r.FormValue("invoice_no")
	baseAmount := parseMoney(r.FormValue("base_amount"))
	vatAmount := parseMoney(r.FormValue("vat_amount"))
	retPercent := parseMoney(r.FormValue("retention_percent"))
	retAmount := parseMoney(r.FormValue("retention_amount"))

	if retAmount == 0 && vatAmount > 0 && retPercent > 0 {
		retAmount = vatAmount * retPercent / 100
	}

	_, err := s.dbFor(r).Exec(`INSERT INTO vat_retention 
		(retention_no, date, purchase_id, supplier_id, invoice_no, base_amount, vat_amount, retention_percent, retention_amount, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		retentionNo, date, purchaseID, supplierID, invoiceNo, baseAmount, vatAmount, retPercent, retAmount, sess.Username, now())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Retención IVA guardada", "redirect": saveRedirect(r, "/legal_books")})
}

func (s *Server) vatRetentionPrint(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT v.id, v.retention_no, v.date, IFNULL(s.supplier_name,'') AS supplier,
			v.invoice_no, v.base_amount, v.vat_amount, v.retention_percent, v.retention_amount, v.created_at
		FROM vat_retention v
		LEFT JOIN supplier_information s ON s.supplier_id = v.supplier_id
		WHERE v.date BETWEEN ? AND ?
		ORDER BY v.date ASC, v.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type vrow struct {
		ID, RetNo, Date, Supplier, Invoice string
		Base, Vat, Pct, Amount             float64
	}
	rowsData := []vrow{}
	var totalRetention float64
	for rows.Next() {
		var id int64
		var retNo, supplier, invoice, createdAt sql.NullString
		var d sql.NullString
		var base, vat, pct, amount float64
		_ = rows.Scan(&id, &retNo, &d, &supplier, &invoice, &base, &vat, &pct, &amount, &createdAt)
		rowsData = append(rowsData, vrow{strconv.FormatInt(id, 10), retNo.String, d.String, supplier.String, invoice.String, base, vat, pct, amount})
		totalRetention += amount
	}
	s.renderStandalone(r, w, "vat_retention_print.html", map[string]interface{}{
		"Title":          "Retenciones de IVA - Imprimible",
		"Month":          month,
		"Year":           year,
		"Rows":           rowsData,
		"TotalRetention": totalRetention,
		"Currency":       s.settingsFor(r).Web.Currency,
	})
}

// ---------- VAT Retention TXT (SENIAT Format) ----------
// GET /reports/vat-retention/txt?periodo=YYYYMM
// Generates tab-delimited TXT file with 16 columns per SENIAT specification:
// RIF_Agente | Periodo | Fecha_Factura | Tipo_Op | Tipo_Doc | RIF_Prov | Nro_Doc | Nro_Control |
// Monto_Total | Base_Imponible | IVA_Retenido | Doc_Afectado | Nro_Comprobante | Monto_Exento | Alícuota | Expediente

type txtCol = string // represents a TXT column value

func (s *Server) vatRetentionTxt(w http.ResponseWriter, r *http.Request) {
	periodo := orDefault(r.FormValue("periodo"), time.Now().Format("200601"))
	if len(periodo) != 6 {
		http.Error(w, "Período inválido. Use formato YYYYMM (ej: 202609)", http.StatusBadRequest)
		return
	}
	year := periodo[:4]
	month := periodo[4:6]

	// Get company RIF from company_information
	var companyRIF sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT IFNULL(vat_no,'') FROM company_information WHERE status=1 ORDER BY company_id LIMIT 1`).Scan(&companyRIF)
	if err != nil || !companyRIF.Valid || companyRIF.String == "" {
		companyRIF.String = "J000000000" // fallback
	}
	agentRIF := normalizeRIF(companyRIF.String)

	// Get purchases with retentions for the period
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT v.date, v.invoice_no, v.base_amount, v.vat_amount, v.retention_percent,
			v.retention_amount, v.retention_no, IFNULL(s.supplier_id,'') AS rif_prov,
			p.grand_total_amount AS total_amount, '' AS doc_afectado
		FROM vat_retention v
		LEFT JOIN supplier_information s ON s.supplier_id = v.supplier_id
		LEFT JOIN product_purchase p ON p.id = v.purchase_id
		WHERE v.date BETWEEN ? AND ?
		ORDER BY v.date ASC, v.id ASC`, startDate, endDate)
	if err != nil {
		http.Error(w, "Error consultando retenciones: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type retentionRecord struct {
		InvoiceNo   string
		Date        string
		Base        float64
		Vat         float64
		Percent     float64
		Retained    float64
		RetNo       string
		SupplierRIF string
		Total       float64
		DocAfectado string
	}
	var records []retentionRecord
	for rows.Next() {
		var rec retentionRecord
		var d, inv, retNo, rifProv, docAfect sql.NullString
		var base, vat, pct, retained, total float64
		_ = rows.Scan(&d, &inv, &base, &vat, &pct, &retained, &retNo, &rifProv, &total, &docAfect)
		rec = retentionRecord{
			InvoiceNo:   inv.String,
			Date:        d.String,
			Base:        base,
			Vat:         vat,
			Percent:     pct,
			Retained:    retained,
			RetNo:       retNo.String,
			SupplierRIF: rifProv.String,
			Total:       total,
			DocAfectado: docAfect.String,
		}
		records = append(records, rec)
	}

	// Build TXT content
	var sb strings.Builder
	if len(records) == 0 {
		// Generate single row with zeros (SENIAT requires at least one line)
		line := buildTxtRow(agentRIF, periodo, "01/01/"+year, "C", "01",
			"V000000000", "000000000000", "", "0.00", "0.00", "0.00", "", "00000000000000", "0.00", "0.00", "")
		sb.WriteString(line)
		sb.WriteString("\r\n")
	} else {
		for _, rec := range records {
			supplierRIF := normalizeRIF(rec.SupplierRIF)
			if supplierRIF == "" {
				supplierRIF = "V000000000"
			}
			docAfectado := rec.InvoiceNo
			if docAfectado == "" {
				docAfectado = "000000000000"
			}
			controlNo := ""
			expediente := ""
			alicuota := "12.00" // default 12% VAT
			if rec.Percent > 0 {
				alicuota = fmt.Sprintf("%.2f", rec.Percent)
			}

			line := buildTxtRow(agentRIF, periodo, rec.Date, "C", "01",
				supplierRIF, rec.InvoiceNo, controlNo,
				fmt.Sprintf("%.2f", rec.Total), fmt.Sprintf("%.2f", rec.Base),
				fmt.Sprintf("%.2f", rec.Retained), docAfectado, rec.RetNo,
				"0.00", alicuota, expediente)
			sb.WriteString(line)
			sb.WriteString("\r\n")
		}
	}

	// Set headers for file download
	filename := "retencion_iva_" + periodo + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(sb.Len()))
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, sb.String())
}

// buildTxtRow builds a single tab-delimited row for the SENIAT TXT format.
func buildTxtRow(agentRIF, periodo, fecha, tipoOp, tipoDoc, rifProv, nroDoc, nroCtrl,
	montoTotal, baseImponible, ivaRetenido, docAfectado, nroComprobante, montoExento,
	alcuota, expediente string) string {
	cols := []string{agentRIF, periodo, fecha, tipoOp, tipoDoc, rifProv, nroDoc, nroCtrl,
		montoTotal, baseImponible, ivaRetenido, docAfectado, nroComprobante, montoExento,
		alcuota, expediente}
	return strings.Join(cols, "\t")
}

// normalizeRIF normalizes a RIF to SENIAT format: letter + 9 digits, no dashes.
func normalizeRIF(rif string) string {
	rif = strings.TrimSpace(rif)
	rif = strings.ReplaceAll(rif, "-", "")
	rif = strings.ReplaceAll(rif, " ", "")
	rif = strings.ToUpper(rif)
	// Extract letter + digits
	var sb strings.Builder
	hasLetter := false
	for _, c := range rif {
		if !hasLetter && (c >= 'A' && c <= 'Z') {
			sb.WriteRune(c)
			hasLetter = true
		} else if c >= '0' && c <= '9' {
			sb.WriteRune(c)
		}
	}
	result := sb.String()
	if len(result) < 2 {
		return "V000000000"
	}
	// Ensure letter + 9 digits
	if !hasLetter {
		result = "V" + result
	}
	digits := ""
	for _, c := range result {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}
	if len(digits) < 9 {
		digits = digits + strings.Repeat("0", 9-len(digits))
	} else if len(digits) > 9 {
		digits = digits[:9]
	}
	return string(result[0]) + digits
}

// ---------- ISLR Retention ----------

func (s *Server) islrRetention(w http.ResponseWriter, r *http.Request) {
	st := s.settingsFor(r)
	supplierRows, _ := s.dbFor(r).Query(`SELECT supplier_id, supplier_id, supplier_name FROM supplier_information WHERE status=1 ORDER BY supplier_name`)
	var suppliers []map[string]interface{}
	if supplierRows != nil {
		defer supplierRows.Close()
		for supplierRows.Next() {
			var id, rif, name sql.NullString
			_ = supplierRows.Scan(&id, &rif, &name)
			suppliers = append(suppliers, map[string]interface{}{"id": id.String, "rif": rif.String, "name": name.String})
		}
	}
	s.render(w, r, "islr_retention.html", map[string]interface{}{
		"Title":     "Retenciones de ISLR",
		"Month":     orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month()))),
		"Year":      orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year())),
		"Suppliers": suppliers,
		"Currency":  st.Web.Currency,
	})
}

func (s *Server) islrRetentionData(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT i.id, i.retention_no, i.date, IFNULL(s.supplier_name,'') AS supplier,
			i.concept, i.base_amount, i.retention_percent, i.retention_amount, i.created_at
		FROM islr_retention i
		LEFT JOIN supplier_information s ON s.supplier_id = i.supplier_id
		WHERE i.date BETWEEN ? AND ?
		ORDER BY i.date ASC, i.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	var totalRetention float64
	for rows.Next() {
		var id int64
		var retNo, supplier, concept, createdAt sql.NullString
		var d sql.NullString
		var base, pct, amount float64
		_ = rows.Scan(&id, &retNo, &d, &supplier, &concept, &base, &pct, &amount, &createdAt)
		list = append(list, map[string]interface{}{
			"id": id, "retention_no": retNo.String, "date": d.String,
			"supplier": supplier.String, "concept": concept.String,
			"base_amount": base, "retention_percent": pct,
			"retention_amount": amount, "created_at": createdAt.String,
		})
		totalRetention += amount
	}
	writeJSON(w, map[string]interface{}{"data": list, "total_retention": totalRetention})
}

func (s *Server) islrRetentionSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "No autorizado", 401)
		return
	}

	retentionNo := r.FormValue("retention_no")
	date := r.FormValue("date")
	supplierID := parseInt64(r.FormValue("supplier_id"))
	concept := r.FormValue("concept")
	baseAmount := parseMoney(r.FormValue("base_amount"))
	retPercent := parseMoney(r.FormValue("retention_percent"))
	retAmount := parseMoney(r.FormValue("retention_amount"))

	if retAmount == 0 && baseAmount > 0 && retPercent > 0 {
		retAmount = baseAmount * retPercent / 100
	}

	_, err := s.dbFor(r).Exec(`INSERT INTO islr_retention 
		(retention_no, date, supplier_id, concept, base_amount, retention_percent, retention_amount, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		retentionNo, date, supplierID, concept, baseAmount, retPercent, retAmount, sess.Username, now())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Retención ISLR guardada", "redirect": saveRedirect(r, "/legal_books")})
}

func (s *Server) islrRetentionPrint(w http.ResponseWriter, r *http.Request) {
	month := orDefault(r.FormValue("month"), strconv.Itoa(int(time.Now().Month())))
	year := orDefault(r.FormValue("year"), strconv.Itoa(time.Now().Year()))
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT i.id, i.retention_no, i.date, IFNULL(s.supplier_name,'') AS supplier,
			i.concept, i.base_amount, i.retention_percent, i.retention_amount, i.created_at
		FROM islr_retention i
		LEFT JOIN supplier_information s ON s.supplier_id = i.supplier_id
		WHERE i.date BETWEEN ? AND ?
		ORDER BY i.date ASC, i.id ASC`, startDate, endDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type irow struct {
		ID, RetNo, Date, Supplier, Concept string
		Base, Pct, Amount                  float64
	}
	rowsData := []irow{}
	var totalRetention float64
	for rows.Next() {
		var id int64
		var retNo, supplier, concept, createdAt sql.NullString
		var d sql.NullString
		var base, pct, amount float64
		_ = rows.Scan(&id, &retNo, &d, &supplier, &concept, &base, &pct, &amount, &createdAt)
		rowsData = append(rowsData, irow{strconv.FormatInt(id, 10), retNo.String, d.String, supplier.String, concept.String, base, pct, amount})
		totalRetention += amount
	}
	s.renderStandalone(r, w, "islr_retention_print.html", map[string]interface{}{
		"Title":          "Retenciones ISLR - Imprimible",
		"Month":          month,
		"Year":           year,
		"Rows":           rowsData,
		"TotalRetention": totalRetention,
		"Currency":       s.settingsFor(r).Web.Currency,
	})
}

// ---------- ISLR Retention TXT (XML SENIAT Format) ----------
// GET /reports/islr-retention/txt?periodo=YYYYMM
// Generates XML file per SENIAT ISLR specification.

func (s *Server) islrRetentionTxt(w http.ResponseWriter, r *http.Request) {
	periodo := orDefault(r.FormValue("periodo"), time.Now().Format("200601"))
	if len(periodo) != 6 {
		s.render(w, r, "islr_retention_txt.html", map[string]interface{}{
			"Title":   "Error - Retención ISLR TXT",
			"Error":   "Período inválido. Use formato YYYYMM (ej: 202609)",
			"Periodo": periodo,
		})
		return
	}
	year := periodo[:4]
	month := periodo[4:6]

	// Get company RIF from company_information
	var companyRIF sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT IFNULL(vat_no,'') FROM company_information WHERE status=1 ORDER BY company_id LIMIT 1`).Scan(&companyRIF)
	if err != nil || !companyRIF.Valid || companyRIF.String == "" {
		companyRIF.String = "J000000000"
	}
	agentRIF := normalizeRIF(companyRIF.String)

	// Get ISLR retentions for the period, joining with islr_concept for code
	startDate := year + "-" + month + "-01"
	endDate := year + "-" + month + "-31"

	rows, err := s.dbFor(r).Query(`
		SELECT i.retention_no, i.date, IFNULL(s.supplier_id,'') AS rif_ret,
			i.concept, i.base_amount, i.retention_percent, i.retention_amount,
			IFNULL(ic.code,'000') AS concept_code
		FROM islr_retention i
		LEFT JOIN supplier_information s ON s.supplier_id = i.supplier_id
		LEFT JOIN islr_concept ic ON `+s.sqlCastText("ic.id")+` = i.concept OR ic.description = i.concept
		WHERE i.date BETWEEN ? AND ?
		ORDER BY i.date ASC, i.id ASC`, startDate, endDate)
	if err != nil {
		s.render(w, r, "islr_retention_txt.html", map[string]interface{}{
			"Title":   "Error - Retención ISLR TXT",
			"Error":   "Error consultando retenciones: " + err.Error(),
			"Periodo": periodo,
		})
		return
	}
	defer rows.Close()

	type islrRecord struct {
		RetNo       string
		Date        string
		RifRet      string
		Concept     string
		Base        float64
		Percent     float64
		Retained    float64
		ConceptCode string
	}
	var records []islrRecord
	for rows.Next() {
		var rec islrRecord
		var d, retNo, rifRet, concept, conceptCode sql.NullString
		var base, pct, retained float64
		_ = rows.Scan(&retNo, &d, &rifRet, &concept, &base, &pct, &retained, &conceptCode)
		rec = islrRecord{
			RetNo:       retNo.String,
			Date:        d.String,
			RifRet:      rifRet.String,
			Concept:     concept.String,
			Base:        base,
			Percent:     pct,
			Retained:    retained,
			ConceptCode: conceptCode.String,
		}
		records = append(records, rec)
	}

	// Build XML content
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n")
	sb.WriteString("<RelacionRetencionesISLR AgenteRetencion=\"" + agentRIF + "\" Periodo=\"" + periodo + "\">\r\n")

	if len(records) == 0 {
		// Empty record with zeros (SENIAT requires at least one DetalleRetenido)
		sb.WriteString("  <DetalleRetenido>\r\n")
		sb.WriteString("    <RifRetenido>V000000000</RifRetenido>\r\n")
		sb.WriteString("    <NumeroFactura>000000000000</NumeroFactura>\r\n")
		sb.WriteString("    <NumeroControl>000000000000</NumeroControl>\r\n")
		sb.WriteString("    <FechaOperacion>01/" + month + "/" + year + "</FechaOperacion>\r\n")
		sb.WriteString("    <CodigoConcepto>000</CodigoConcepto>\r\n")
		sb.WriteString("    <BaseImponible>0.00</BaseImponible>\r\n")
		sb.WriteString("    <PorcentajeRetencion>0.00</PorcentajeRetencion>\r\n")
		sb.WriteString("    <MontoRetenido>0.00</MontoRetenido>\r\n")
		sb.WriteString("  </DetalleRetenido>\r\n")
	} else {
		for _, rec := range records {
			rifRetenido := normalizeRIF(rec.RifRet)
			if rifRetenido == "" {
				rifRetenido = "V000000000"
			}
			invoiceNo := rec.RetNo
			if invoiceNo == "" {
				invoiceNo = "000000000000"
			}
			controlNo := ""
			fechaOp := formatDateDDMMYYYY(rec.Date)
			conceptCode := rec.ConceptCode
			if conceptCode == "" {
				conceptCode = "000"
			}
			// Calculate actual retention percentage
			percent := rec.Percent
			if percent == 0 && rec.Base > 0 && rec.Retained > 0 {
				percent = (rec.Retained / rec.Base) * 100
			}

			sb.WriteString("  <DetalleRetenido>\r\n")
			sb.WriteString("    <RifRetenido>" + rifRetenido + "</RifRetenido>\r\n")
			sb.WriteString("    <NumeroFactura>" + invoiceNo + "</NumeroFactura>\r\n")
			sb.WriteString("    <NumeroControl>" + controlNo + "</NumeroControl>\r\n")
			sb.WriteString("    <FechaOperacion>" + fechaOp + "</FechaOperacion>\r\n")
			sb.WriteString("    <CodigoConcepto>" + conceptCode + "</CodigoConcepto>\r\n")
			sb.WriteString("    <BaseImponible>" + fmt.Sprintf("%.2f", rec.Base) + "</BaseImponible>\r\n")
			sb.WriteString("    <PorcentajeRetencion>" + fmt.Sprintf("%.2f", percent) + "</PorcentajeRetencion>\r\n")
			sb.WriteString("    <MontoRetenido>" + fmt.Sprintf("%.2f", rec.Retained) + "</MontoRetenido>\r\n")
			sb.WriteString("  </DetalleRetenido>\r\n")
		}
	}
	sb.WriteString("</RelacionRetencionesISLR>\r\n")

	// Set headers for file download
	filename := "retencion_islr_" + periodo + ".xml"
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(sb.Len()))
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, sb.String())
}

// formatDateDDMMYYYY converts YYYY-MM-DD to DD/MM/YYYY.
func formatDateDDMMYYYY(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)
	parts := strings.Split(dateStr, "-")
	if len(parts) != 3 {
		return dateStr
	}
	return parts[2] + "/" + parts[1] + "/" + parts[0]
}

// parseInt64 parses a string to int64, returns 0 on error.
func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
