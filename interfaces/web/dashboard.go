package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// ---------- Dashboard (replicates modules/dashboard Home) ----------

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/home" {
		http.NotFound(w, r)
		return
	}

	stats := map[string]interface{}{}
	var customers, products, suppliers, invoices int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM customer_information`).Scan(&customers)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM product_information`).Scan(&products)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM supplier_information`).Scan(&suppliers)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM invoice`).Scan(&invoices)
	stats["customers"] = customers
	stats["products"] = products
	stats["suppliers"] = suppliers
	stats["invoices"] = invoices

	// today's sales/purchases
	var todaySales, todayPurchases, todayDue sql.NullFloat64
	_ = s.dbFor(r).QueryRow(`SELECT SUM(total_amount) FROM invoice WHERE date = ? AND is_draft='no'`, today()).Scan(&todaySales)
	_ = s.dbFor(r).QueryRow(`SELECT SUM(grand_total_amount) FROM product_purchase WHERE purchase_date = ?`, today()).Scan(&todayPurchases)
	_ = s.dbFor(r).QueryRow(`SELECT SUM(due_amount) FROM invoice WHERE date = ? AND is_draft='no'`, today()).Scan(&todayDue)
	stats["today_sales"] = todaySales.Float64
	stats["today_purchases"] = todayPurchases.Float64
	stats["today_due"] = todayDue.Float64

	// Cobrado del día, en porcentaje. Es el dato que convierte «vendimos X» en
	// una frase con final: sin él, la ficha de ventas y la de por cobrar son dos
	// cifras sueltas y hay que restarlas de cabeza. Si no hubo ventas la ficha lo
	// dice con palabras, no con un «0 %» que parece una caída.
	collected := 0.0
	if todaySales.Float64 > 0 {
		collected = (todaySales.Float64 - todayDue.Float64) / todaySales.Float64 * 100
		if collected < 0 {
			collected = 0
		}
	}

	// Los conteos que acompañan a cada ficha de dinero. Son tres COUNT sobre
	// índices de fecha y evitan que la línea de contexto tenga que decir algo
	// vacío: «3 órdenes de compra» informa; «compras del día» solo repite el
	// rótulo.
	var invoicesToday, purchasesToday, pendingToday int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM invoice WHERE date = ? AND is_draft='no'`, today()).Scan(&invoicesToday)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM product_purchase WHERE purchase_date = ?`, today()).Scan(&purchasesToday)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM invoice WHERE date = ? AND is_draft='no' AND due_amount > 0`, today()).Scan(&pendingToday)

	// Las frases se arman aquí y no en la plantilla: el plural en español y el
	// caso «no hubo nada» son lógica, no presentación. La plantilla solo decide
	// si pinta la píldora y escribe el texto.
	stats["sales_note"] = "Sin ventas registradas"
	if todaySales.Float64 > 0 {
		stats["sales_note"] = fmt.Sprintf("%.0f%% cobrado", collected)
	}

	stats["purchases_note"] = "Sin compras registradas"
	if purchasesToday == 1 {
		stats["purchases_note"] = "1 orden de compra"
	} else if purchasesToday > 1 {
		stats["purchases_note"] = fmt.Sprintf("%d órdenes de compra", purchasesToday)
	}

	stats["pending_note"] = "Nada pendiente de cobro"
	stats["has_pending"] = pendingToday > 0
	if pendingToday == 1 {
		stats["pending_note"] = "En 1 factura por cobrar"
	} else if pendingToday > 1 {
		stats["pending_note"] = fmt.Sprintf("En %d facturas por cobrar", pendingToday)
	}

	stats["has_invoices_today"] = invoicesToday > 0
	stats["invoices_today_note"] = fmt.Sprintf("%d hoy", invoicesToday)

	// low stock products (like dashboard widget)
	type lowStock struct {
		ID    string
		Name  string
		Stock float64
	}
	low := []lowStock{}
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name
		FROM product_information a`)
	if err == nil && rows != nil {
		for rows.Next() {
			var l lowStock
			_ = rows.Scan(&l.ID, &l.Name)
			pidInt, _ := strconv.Atoi(l.ID)
			l.Stock = s.stockForProduct(r, pidInt)
			if l.Stock <= 5 {
				low = append(low, l)
			}
		}
		rows.Close()
		sort.Slice(low, func(i, j int) bool {
			return low[i].Stock < low[j].Stock
		})
	}
	// Se cuentan todos antes de recortar: la ficha de productos avisa cuántos
	// hay en alerta, y con el recorte de la tabla ese número mentiría en cuanto
	// pasara de diez. La tabla sí se queda en diez, que es lo que cabe.
	stats["has_low_stock"] = len(low) > 0
	stats["low_stock_note"] = ""
	if len(low) == 1 {
		stats["low_stock_note"] = "1 bajo"
	} else if len(low) > 1 {
		stats["low_stock_note"] = fmt.Sprintf("%d bajos", len(low))
	}
	if len(low) > 10 {
		low = low[:10]
	}

	// today's invoices table
	type tinv struct {
		ID       int64
		Customer string
		Total    float64
		Paid     float64
	}
	todays := []tinv{}
	rows, err = s.dbFor(r).Query(`
		SELECT i.invoice_id, IFNULL(c.customer_name,''), i.total_amount, i.paid_amount
		FROM invoice i LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.date = ? AND i.is_draft='no' ORDER BY i.id DESC LIMIT 10`, today())
	if err == nil && rows != nil {
		for rows.Next() {
			var t tinv
			_ = rows.Scan(&t.ID, &t.Customer, &t.Total, &t.Paid)
			todays = append(todays, t)
		}
		rows.Close()
	}

	s.render(w, r, "dashboard.html", map[string]interface{}{
		"Title":         "Dashboard",
		"Stats":         stats,
		"LowStock":      low,
		"TodayInvoices": todays,
	})
}

// handleDashboardChartData: 12-month sales/purchases + best sellers (Chart.js).
func (s *Server) handleDashboardChartData(w http.ResponseWriter, r *http.Request) {
	year := time.Now().Year()
	if y := r.FormValue("year"); y != "" {
		if parsed, err := time.Parse("2006", y); err == nil {
			year = parsed.Year()
		}
	}

	sales := make([]float64, 12)
	purchases := make([]float64, 12)

	rows, err := s.dbFor(r).Query(`SELECT date, total_amount FROM invoice WHERE is_draft='no'`)
	if err == nil {
		for rows.Next() {
			var d sql.NullString
			var amt float64
			_ = rows.Scan(&d, &amt)
			if len(d.String) >= 7 {
				var y int
				var m int
				if n, _ := time.Parse("2006-01", d.String[:7]); n.Year() > 0 {
					y = n.Year()
					m = int(n.Month())
					if y == year {
						sales[m-1] += amt
					}
				}
			}
		}
		rows.Close()
	}
	rows, err = s.dbFor(r).Query(`SELECT purchase_date, grand_total_amount FROM product_purchase`)
	if err == nil {
		for rows.Next() {
			var d sql.NullString
			var amt float64
			_ = rows.Scan(&d, &amt)
			if len(d.String) >= 7 {
				if n, _ := time.Parse("2006-01", d.String[:7]); n.Year() == year {
					purchases[int(n.Month())-1] += amt
				}
			}
		}
		rows.Close()
	}

	// best sellers
	type best struct {
		Name  string  `json:"name"`
		Qty   float64 `json:"qty"`
		Total float64 `json:"total"`
	}
	bestSellers := []best{}
	rows, err = s.dbFor(r).Query(`
		SELECT IFNULL(p.product_name,''), SUM(d.quantity), SUM(d.total_price)
		FROM invoice_details d LEFT JOIN product_information p ON p.product_id = d.product_id
		GROUP BY d.product_id, p.product_name ORDER BY SUM(d.quantity) DESC LIMIT 10`)
	if err == nil && rows != nil {
		for rows.Next() {
			var b best
			_ = rows.Scan(&b.Name, &b.Qty, &b.Total)
			bestSellers = append(bestSellers, b)
		}
		rows.Close()
	}

	writeJSON(w, map[string]interface{}{
		"labels":    []string{"Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"},
		"sales":     sales,
		"purchases": purchases,
		"best":      bestSellers,
	})
}
