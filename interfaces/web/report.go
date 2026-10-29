package web

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Report module (replicates modules/report) ----------

func (s *Server) registerReportRoutes() {
	s.mux.HandleFunc("GET /reports", s.reportsHome)
	s.mux.HandleFunc("GET /reports/sales", s.reportSales)
	s.mux.HandleFunc("/reports/sales/data", s.reportSalesData)
	s.mux.HandleFunc("GET /reports/purchases", s.reportPurchases)
	s.mux.HandleFunc("/reports/purchases/data", s.reportPurchasesData)
	s.mux.HandleFunc("GET /reports/stock", s.reportStock)
	s.mux.HandleFunc("/reports/stock/data", s.reportStockData)
	s.mux.HandleFunc("GET /reports/daily-closing", s.reportDailyClosing)
	s.mux.HandleFunc("POST /reports/daily-closing", s.reportDailyClosing)
	s.mux.HandleFunc("GET /reports/tax", s.reportTax)
	s.mux.HandleFunc("GET /api/reports/currency-position", s.reportCurrencyPosition)
	s.mux.HandleFunc("GET /api/reports/currency-exposure", s.reportCurrencyExposure)
}

func (s *Server) reportsHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "reports_home.html", map[string]interface{}{"Title": "Reportes"})
}

// reportSales: sales report with date/user filters.
func (s *Server) reportSales(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "report_sales.html", map[string]interface{}{
		"Title": "Reporte de Ventas",
		"From":  orDefault(r.FormValue("from"), today()),
		"To":    orDefault(r.FormValue("to"), today()),
	})
}

func (s *Server) reportSalesData(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), "2000-01-01")
	to := orDefault(r.FormValue("to"), today())

	rows, err := s.dbFor(r).Query(`
		SELECT i.date, i.invoice_id, IFNULL(c.customer_name,''), i.total_amount, i.paid_amount, i.due_amount
		FROM invoice i
		LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.date BETWEEN ? AND ? AND i.is_draft = 'no'
		ORDER BY i.date DESC, i.id DESC`, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	var total, paid, due float64
	for rows.Next() {
		var d, cname sql.NullString
		var iid int64
		var t, p, du float64
		_ = rows.Scan(&d, &iid, &cname, &t, &p, &du)
		list = append(list, map[string]interface{}{
			"date": d.String, "invoice": iid, "customer": cname.String,
			"total": t, "paid": p, "due": du,
		})
		total += t
		paid += p
		due += du
	}
	writeJSON(w, map[string]interface{}{
		"data": list, "total": total, "paid": paid, "due": due,
	})
}

func (s *Server) reportPurchases(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "report_purchases.html", map[string]interface{}{
		"Title": "Reporte de Compras",
		"From":  orDefault(r.FormValue("from"), today()),
		"To":    orDefault(r.FormValue("to"), today()),
	})
}

func (s *Server) reportPurchasesData(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), "2000-01-01")
	to := orDefault(r.FormValue("to"), today())

	rows, err := s.dbFor(r).Query(`
		SELECT p.purchase_date, p.purchase_id, IFNULL(s.supplier_name,''), p.grand_total_amount, p.paid_amount, p.due_amount
		FROM product_purchase p
		LEFT JOIN supplier_information s ON s.supplier_id = p.supplier_id
		WHERE p.purchase_date BETWEEN ? AND ?
		ORDER BY p.purchase_date DESC, p.id DESC`, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	var total, paid, due float64
	for rows.Next() {
		var d, sname sql.NullString
		var pid int64
		var t, p, du float64
		_ = rows.Scan(&d, &pid, &sname, &t, &p, &du)
		list = append(list, map[string]interface{}{
			"date": d.String, "purchase": pid, "supplier": sname.String,
			"total": t, "paid": p, "due": du,
		})
		total += t
		paid += p
		due += du
	}
	writeJSON(w, map[string]interface{}{
		"data": list, "total": total, "paid": paid, "due": due,
	})
}

func (s *Server) reportStock(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(c.category_name,''),
			IFNULL(a.unit,''), a.price
		FROM product_information a
		LEFT JOIN product_category c ON c.category_id = a.category_id
		WHERE a.status = 1
		ORDER BY a.product_name`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type StockItem struct {
		ID, Name, Model, Category, Unit, Price string
		Stock                                  float64
	}
	items := []StockItem{}
	for rows.Next() {
		var it StockItem
		_ = rows.Scan(&it.ID, &it.Name, &it.Model, &it.Category, &it.Unit, &it.Price)
		pidInt, _ := strconv.Atoi(it.ID)
		it.Stock = s.stockForProduct(r, pidInt)
		items = append(items, it)
	}

	s.render(w, r, "report_stock.html", map[string]interface{}{
		"Title": "Reporte de Inventario",
		"Items": items,
	})
}

func (s *Server) reportStockData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(c.category_name,''),
			IFNULL(a.unit,''), a.price
		FROM product_information a
		LEFT JOIN product_category c ON c.category_id = a.category_id
		WHERE a.status = 1
		ORDER BY a.product_name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var pid, name, model, cat, unit, price sql.NullString
		_ = rows.Scan(&pid, &name, &model, &cat, &unit, &price)
		pidInt, _ := strconv.Atoi(pid.String)
		stock := s.stockForProduct(r, pidInt)
		list = append(list, map[string]interface{}{
			"id": pid.String, "name": name.String, "model": model.String,
			"category": cat.String, "unit": unit.String, "price": price.String, "stock": stock,
		})
	}
	writeJSON(w, list)
}

// reportDailyClosing replicates daily_closing: cash in/out snapshot.
func (s *Server) reportDailyClosing(w http.ResponseWriter, r *http.Request) {
	st := s.settingsFor(r)
	pre, err := s.predefineAccount(r)
	if err != nil {
		http.Error(w, "Cuentas predefinidas no configuradas", 500)
		return
	}

	date := orDefault(r.FormValue("date"), today())

	if r.Method == "POST" {
		// save closing snapshot (like PHP daily_closing insert)
		var lastClose float64
		_ = s.dbFor(r).QueryRow(`SELECT amount FROM daily_closing WHERE date < ? ORDER BY closing_id DESC LIMIT 1`, date).Scan(&lastClose)

		var cashIn, cashOut float64
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Debit),0) FROM acc_transaction WHERE COAID=? AND IsAppove=1 AND VDate=?`, pre.CashCode, date).Scan(&cashIn)
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Credit),0) FROM acc_transaction WHERE COAID=? AND IsAppove=1 AND VDate=?`, pre.CashCode, date).Scan(&cashOut)

		adjustment := parseMoney(r.FormValue("adjustment"))
		amount := lastClose + cashIn - cashOut + adjustment
		if _, err := s.dbFor(r).Exec(`INSERT INTO daily_closing (last_day_closing, cash_in, cash_out, date, amount, adjustment, status)
			VALUES (?,?,?,?,?,?,1)`, lastClose, cashIn, cashOut, date, amount, adjustment); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cierre guardado", "amount": amount,
			"redirect": saveRedirect(r, "/reports/daily-closing")})
		return
	}

	// GET: show form + today's numbers
	var lastClose float64
	_ = s.dbFor(r).QueryRow(`SELECT amount FROM daily_closing WHERE date < ? ORDER BY closing_id DESC LIMIT 1`, date).Scan(&lastClose)
	var cashIn, cashOut float64
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Debit),0) FROM acc_transaction WHERE COAID=? AND IsAppove=1 AND VDate=?`, pre.CashCode, date).Scan(&cashIn)
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Credit),0) FROM acc_transaction WHERE COAID=? AND IsAppove=1 AND VDate=?`, pre.CashCode, date).Scan(&cashOut)

	// recent closings
	type closing struct {
		Date    string
		LastDay float64
		CashIn  float64
		CashOut float64
		Amount  float64
	}
	closings := []closing{}
	rows, _ := s.dbFor(r).Query(`SELECT date, last_day_closing, cash_in, cash_out, amount FROM daily_closing ORDER BY closing_id DESC LIMIT 30`)
	if rows != nil {
		for rows.Next() {
			var c closing
			_ = rows.Scan(&c.Date, &c.LastDay, &c.CashIn, &c.CashOut, &c.Amount)
			closings = append(closings, c)
		}
		rows.Close()
	}

	s.render(w, r, "daily_closing.html", map[string]interface{}{
		"Title":     "Cierre Diario",
		"Date":      date,
		"LastClose": lastClose,
		"CashIn":    cashIn,
		"CashOut":   cashOut,
		"Expected":  lastClose + cashIn - cashOut,
		"Closings":  closings,
		"Currency":  st.Web.Currency,
	})
}

// reportTax: tax collection summary.
func (s *Server) reportTax(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), today()[:4]+"-01-01")
	to := orDefault(r.FormValue("to"), today())

	type trow struct {
		Date     string
		Invoice  int64
		Customer string
		Vat      float64
		Tax      float64
	}
	rowsData := []trow{}
	rows, err := s.dbFor(r).Query(`
		SELECT i.date, i.invoice_id, IFNULL(c.customer_name,''), i.total_vat_amnt, IFNULL(i.total_tax,0)
		FROM invoice i LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.date BETWEEN ? AND ? AND i.is_draft='no'
		ORDER BY i.date DESC`, from, to)
	if err == nil && rows != nil {
		for rows.Next() {
			var t trow
			_ = rows.Scan(&t.Date, &t.Invoice, &t.Customer, &t.Vat, &t.Tax)
			rowsData = append(rowsData, t)
		}
		rows.Close()
	}
	var totalVat, totalTax float64
	for _, t := range rowsData {
		totalVat += t.Vat
		totalTax += t.Tax
	}

	s.render(w, r, "report_tax.html", map[string]interface{}{
		"Title":    "Reporte de Impuestos",
		"From":     from,
		"To":       to,
		"Rows":     rowsData,
		"TotalVat": totalVat,
		"TotalTax": totalTax,
	})
}

var (
	_ = strconv.Itoa
	_ = strings.TrimSpace
	_ = time.Now
)
