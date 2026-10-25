package web

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ---------- Kárdex — Libro de Inventario (Entradas y Salidas) ----------
//
// Registro de inventario permanente exigido por el Reglamento de la Ley de
// ISLR, Artículo 177, valorizado por Costo Promedio Ponderado (CPP).
//
// El libro NO tiene tabla propia y no se carga a mano: se arma leyendo los
// movimientos reales del sistema, que es como lo pide la norma. El libro debe
// reflejar la mercancía que entró y salió; un registro paralelo que se llene
// aparte se desincroniza y deja de ser prueba.
//
//	ENTRADAS: compras, devoluciones de venta (el cliente devuelve mercancía),
//	          ajustes de inventario que suman.
//	SALIDAS:  ventas, devoluciones de compra (se devuelve al proveedor),
//	          ajustes de inventario que restan.
//
// CPP: al entrar mercancía el costo promedio se recalcula
// (saldo anterior + entrada) / (cantidad anterior + cantidad entrada). Al salir
// NO se recalcula: la salida se valoriza al CPP vigente, que es justamente lo
// que hace del promedio ponderado un método estable.

func (s *Server) registerKardexRoutes() {
	s.mux.HandleFunc("GET /reports/kardex", s.kardex)
	s.mux.HandleFunc("GET /reports/kardex/data", s.kardexData)
	s.mux.HandleFunc("GET /reports/kardex/print", s.kardexPrint)
	s.mux.HandleFunc("GET /reports/kardex/products", s.productosKardex)
	// Alias corto: en Venezuela el libro se pide por su nombre ("entradas y
	// salidas"), no por el nombre de la tabla de la que sale.
	s.mux.HandleFunc("GET /kardex", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/reports/kardex", http.StatusMovedPermanently)
	})
}

// kardexMovement es un renglón del libro, ya valorizado por CPP.
type kardexMovement struct {
	Date         string  `json:"date"`
	Concept      string  `json:"concept"`
	DocType      string  `json:"doc_type"`
	DocNumber    string  `json:"doc_number"`
	QtyIn        float64 `json:"qty_in"`
	CostIn       float64 `json:"cost_in"`
	TotalIn      float64 `json:"total_in"`
	QtyOut       float64 `json:"qty_out"`
	CostOut      float64 `json:"cost_out"`
	TotalOut     float64 `json:"total_out"`
	QtyBalance   float64 `json:"qty_balance"`
	AvgCost      float64 `json:"avg_cost"`
	TotalBalance float64 `json:"total_balance"`

	// valorarAlCPP marca las entradas que no traen costo propio (los ajustes):
	// entran valorizadas al promedio vigente en ese momento, no a cero. Es
	// campo interno, no se serializa.
	valorarAlCPP bool
}

// esEntrada dice si el renglón suma o resta mercancía.
func (m kardexMovement) esEntrada() bool { return m.QtyIn > 0 }

// kardexRango devuelve el período pedido, con el mes en curso como valor por
// defecto. El último día se calcula de verdad (time.Date(y, mes+1, 0)) en vez de
// asumir 31: `store_adjustment.date` es DATE y MySQL 8 rechaza '2026-02-31'.
func kardexRango(r *http.Request) (from, to string) {
	now := time.Now()
	inicio := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	fin := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location())
	from = orDefault(r.FormValue("from"), inicio.Format("2006-01-02"))
	to = orDefault(r.FormValue("to"), fin.Format("2006-01-02"))
	return from, to
}

// kardexMovements arma el libro de un producto: junta los movimientos de todas
// las fuentes, los ordena cronológicamente y calcula saldos y CPP.
//
// Las tres vistas (pantalla, JSON e impresión) llaman aquí. Antes cada una
// repetía su propia copia del cálculo y las tres diferían; un libro fiscal no
// puede tener tres respuestas distintas para la misma pregunta.
func (s *Server) kardexMovements(r *http.Request, pid, from, to string) ([]kardexMovement, error) {
	db := s.dbFor(r)
	var moves []kardexMovement

	// ---------- ENTRADAS ----------

	// Compras.
	rows, err := db.Query(`
		SELECT pp.purchase_date, ppd.quantity, ppd.rate, ppd.total_amount, pp.chalan_no
		FROM product_purchase_details ppd
		JOIN product_purchase pp ON pp.purchase_id = ppd.purchase_id
		WHERE ppd.product_id = ? AND pp.purchase_date BETWEEN ? AND ?
		ORDER BY pp.purchase_date ASC, pp.id ASC`, pid, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d, doc sql.NullString
		var qty, rate, total float64
		_ = rows.Scan(&d, &qty, &rate, &total, &doc)
		moves = append(moves, kardexMovement{
			Date: d.String, Concept: "Compra", DocType: "FACTURA_COMPRA",
			DocNumber: doc.String, QtyIn: qty, CostIn: rate, TotalIn: total,
		})
	}
	rows.Close()

	// Devoluciones de venta: el cliente devuelve mercancía, vuelve al almacén.
	// `product_return` guarda la devolución de venta con invoice_id y la de
	// compra con purchase_id; es lo único que las distingue.
	rows, err = db.Query(`
		SELECT pr.date_return, pr.ret_qty, pr.product_rate, pr.total_ret_amount, pr.return_id
		FROM product_return pr
		WHERE pr.product_id = ? AND pr.date_return BETWEEN ? AND ?
		  AND pr.invoice_id IS NOT NULL AND pr.invoice_id <> ''
		ORDER BY pr.date_return ASC`, pid, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d, doc sql.NullString
		var qty, rate, total float64
		_ = rows.Scan(&d, &qty, &rate, &total, &doc)
		moves = append(moves, kardexMovement{
			Date: d.String, Concept: "Devolución venta", DocType: "DEVOLUCION_VENTA",
			DocNumber: doc.String, QtyIn: qty, CostIn: rate, TotalIn: total,
		})
	}
	rows.Close()

	// Ajustes que suman (sobrantes, correcciones al alza).
	rows, err = db.Query(`
		SELECT a.date, d.quantity, a.adjustment_no, IFNULL(a.reference,'')
		FROM store_adjustment_details d
		JOIN store_adjustment a ON a.adjustment_no = d.adjustment_no
		WHERE d.product_id = ? AND d.type = 'add' AND a.date BETWEEN ? AND ?
		ORDER BY a.date ASC, d.id ASC`, pid, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d sql.NullTime
		var doc, ref sql.NullString
		var qty float64
		_ = rows.Scan(&d, &qty, &doc, &ref)
		moves = append(moves, kardexMovement{
			Date: d.Time.Format("2006-01-02"), Concept: "Ajuste (+)", DocType: "AJUSTE_SOBRANTE",
			DocNumber: doc.String, QtyIn: qty, valorarAlCPP: true,
		})
	}
	rows.Close()

	// ---------- SALIDAS ----------

	// Ventas. Se excluyen borradores y anuladas: una factura anulada no sacó
	// mercancía del almacén y no puede figurar en el libro. Con la nueva
	// arquitectura de inventario, las salidas se leen de stock_movement.
	rows, err = db.Query(`
		SELECT sm.date, sm.quantity, sm.unit_cost, sm.quantity * sm.unit_cost, sm.reference_no
		FROM stock_movement sm
		WHERE sm.product_id = ? AND sm.movement_type = 'sale' AND sm.date BETWEEN ? AND ?
		ORDER BY sm.date ASC, sm.id ASC`, pid, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d, doc sql.NullString
		var qty, rate, total float64
		_ = rows.Scan(&d, &qty, &rate, &total, &doc)
		moves = append(moves, kardexMovement{
			Date: d.String, Concept: "Venta", DocType: "FACTURA_VENTA",
			DocNumber: doc.String, QtyOut: qty, TotalOut: total,
		})
	}
	rows.Close()

	// Servicios: No mueven stock, pero generan ingreso contable.
	// Se muestran en el kardex como "Venta Servicio" para reflejar
	// balances contables armónicos (ingreso sin salida de inventario).
	rows, err = db.Query(`
		SELECT i.date, i.total_amount, i.invoice
		FROM invoice i
		JOIN invoice_details id ON id.invoice_id = i.invoice_id
		JOIN product_information pi ON pi.product_id = id.product_id
		LEFT JOIN product_inventory_config pic ON pic.product_id = pi.product_id
		WHERE i.date BETWEEN ? AND ?
		  AND (pic.product_type = 'service' OR pic.product_id IS NULL AND pi.product_type = 'service')
		GROUP BY i.id
		ORDER BY i.date ASC, i.id ASC`, from, to)
	if err == nil {
		for rows.Next() {
			var d sql.NullString
			var total float64
			var invNo sql.NullString
			_ = rows.Scan(&d, &total, &invNo)
			moves = append(moves, kardexMovement{
				Date: d.String, Concept: "Venta Servicio", DocType: "FACTURA_SERVICIO",
				DocNumber: invNo.String, QtyIn: 0, TotalIn: total,
			})
		}
		rows.Close()
	}

	// Devoluciones de compra: se devuelve mercancía al proveedor.
	rows, err = db.Query(`
		SELECT pr.date_return, pr.ret_qty, pr.product_rate, pr.total_ret_amount, pr.return_id
		FROM product_return pr
		WHERE pr.product_id = ? AND pr.date_return BETWEEN ? AND ?
		  AND (pr.invoice_id IS NULL OR pr.invoice_id = '')
		  AND pr.purchase_id IS NOT NULL AND pr.purchase_id <> ''
		ORDER BY pr.date_return ASC`, pid, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d, doc sql.NullString
		var qty, rate, total float64
		_ = rows.Scan(&d, &qty, &rate, &total, &doc)
		moves = append(moves, kardexMovement{
			Date: d.String, Concept: "Devolución compra", DocType: "DEVOLUCION_COMPRA",
			DocNumber: doc.String, QtyOut: qty, TotalOut: total,
		})
	}
	rows.Close()

	// Ajustes que restan (faltantes, mermas, daños).
	rows, err = db.Query(`
		SELECT a.date, d.quantity, a.adjustment_no, IFNULL(a.reference,'')
		FROM store_adjustment_details d
		JOIN store_adjustment a ON a.adjustment_no = d.adjustment_no
		WHERE d.product_id = ? AND d.type = 'remove' AND a.date BETWEEN ? AND ?
		ORDER BY a.date ASC, d.id ASC`, pid, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d sql.NullTime
		var doc, ref sql.NullString
		var qty float64
		_ = rows.Scan(&d, &qty, &doc, &ref)
		moves = append(moves, kardexMovement{
			Date: d.Time.Format("2006-01-02"), Concept: "Ajuste (-)", DocType: "AJUSTE_FALTANTE",
			DocNumber: doc.String, QtyOut: qty,
		})
	}
	rows.Close()

	// ---------- Orden cronológico ----------
	//
	// Las consultas vienen ordenadas cada una por su cuenta, pero el libro es
	// la mezcla de todas. Sin ordenar, las compras quedaban todas antes que las
	// ventas y el saldo corrido salía mal (llegaba a negativo sin motivo).
	//
	// A igual fecha, las entradas van primero: es el orden en que la mercancía
	// se movió de verdad y evita que una salida consuma stock que aún no había
	// entrado ese mismo día.
	sort.SliceStable(moves, func(i, j int) bool {
		if moves[i].Date != moves[j].Date {
			return moves[i].Date < moves[j].Date
		}
		return moves[i].esEntrada() && !moves[j].esEntrada()
	})

	// ---------- Saldos y Costo Promedio Ponderado ----------
	var qty, total float64
	for i := range moves {
		m := &moves[i]

		if m.QtyIn > 0 {
			// Ajuste sin costo propio: entra valorizado al promedio vigente.
			if m.valorarAlCPP {
				if qty > 0 {
					m.CostIn = total / qty
				} else {
					m.CostIn = 0
				}
				m.TotalIn = m.QtyIn * m.CostIn
			}
			qty += m.QtyIn
			total += m.TotalIn
		} else if m.QtyOut > 0 {
			// La salida se valoriza al CPP vigente (no al precio de venta): el
			// libro es de costo, no de ingreso.
			if qty > 0 {
				m.CostOut = total / qty
			} else {
				m.CostOut = 0
			}
			m.TotalOut = m.QtyOut * m.CostOut
			qty -= m.QtyOut
			total -= m.TotalOut
		}

		m.QtyBalance = qty
		m.TotalBalance = total
		// El "Costo Prom." de la columna SALDO es el CPP vigente después del
		// movimiento. Antes se asignaba la CANTIDAD a este campo en la rama de
		// salida (m.AvgCost = qtyBalance), así que el libro mostraba como costo
		// promedio el número de unidades.
		if qty > 0 {
			m.AvgCost = total / qty
		} else {
			m.AvgCost = 0
		}
	}

	return moves, nil
}

// kardexTotales suma las columnas de entrada y salida del período.
func kardexTotales(moves []kardexMovement) (qtyEntradas, totalEntradas, qtySalidas, totalSalidas float64) {
	for _, m := range moves {
		qtyEntradas += m.QtyIn
		totalEntradas += m.TotalIn
		qtySalidas += m.QtyOut
		totalSalidas += m.TotalOut
	}
	return
}

// kardex muestra la vista del libro con filtros.
func (s *Server) kardex(w http.ResponseWriter, r *http.Request) {
	pid := strings.TrimSpace(r.FormValue("product_id"))
	from, to := kardexRango(r)

	// Sin producto seleccionado, el selector.
	if pid == "" {
		s.render(w, r, "kardex_select.html", map[string]interface{}{
			"Title": "Libro de Inventario — Entradas y Salidas (Art. 177 ISLR)",
			"From":  from,
			"To":    to,
		})
		return
	}

	s.kardexShow(w, r, pid, from, to)
}

// kardexShow renderiza el libro completo de un producto.
func (s *Server) kardexShow(w http.ResponseWriter, r *http.Request, pid, from, to string) {
	var pname, pmodel, punit sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT product_name, IFNULL(product_model,''), IFNULL(unit,'') FROM product_information WHERE product_id = ?`, pid).
		Scan(&pname, &pmodel, &punit)

	moves, err := s.kardexMovements(r, pid, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	qtyEntradas, totalEntradas, qtySalidas, totalSalidas := kardexTotales(moves)

	var finalQty, finalTotal, cpp float64
	if len(moves) > 0 {
		last := moves[len(moves)-1]
		finalQty, finalTotal, cpp = last.QtyBalance, last.TotalBalance, last.AvgCost
	}

	s.render(w, r, "kardex.html", map[string]interface{}{
		"Title":         "Libro de Inventario — Entradas y Salidas (Art. 177 ISLR)",
		"ProductID":     pid,
		"ProductName":   pname.String,
		"ProductModel":  pmodel.String,
		"ProductUnit":   punit.String,
		"From":          from,
		"To":            to,
		"Rows":          moves,
		"QtyEntradas":   qtyEntradas,
		"TotalEntradas": totalEntradas,
		"QtySalidas":    qtySalidas,
		"TotalSalidas":  totalSalidas,
		"FinalQty":      finalQty,
		"FinalTotal":    finalTotal,
		"AvgCost":       cpp,
		"CostoVentas":   totalSalidas,
		"Currency":      s.settingsFor(r).Web.Currency,
	})
}

// kardexData devuelve el libro en JSON (DataTables y consumo programático).
func (s *Server) kardexData(w http.ResponseWriter, r *http.Request) {
	pid := strings.TrimSpace(r.FormValue("product_id"))
	from, to := kardexRango(r)

	if pid == "" {
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
		return
	}

	var pname sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT product_name FROM product_information WHERE product_id = ?`, pid).Scan(&pname)

	moves, err := s.kardexMovements(r, pid, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var finalQty, finalTotal float64
	if len(moves) > 0 {
		last := moves[len(moves)-1]
		finalQty, finalTotal = last.QtyBalance, last.TotalBalance
	}
	if moves == nil {
		moves = []kardexMovement{}
	}

	writeJSON(w, map[string]interface{}{
		"data":        moves,
		"product":     pname.String,
		"final_qty":   finalQty,
		"final_total": finalTotal,
	})
}

// kardexPrint muestra la versión imprimible del libro.
func (s *Server) kardexPrint(w http.ResponseWriter, r *http.Request) {
	pid := strings.TrimSpace(r.FormValue("product_id"))
	from, to := kardexRango(r)

	if pid == "" {
		http.Error(w, "Producto requerido", 400)
		return
	}

	var pname, pmodel, punit sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT product_name, IFNULL(product_model,''), IFNULL(unit,'') FROM product_information WHERE product_id = ?`, pid).
		Scan(&pname, &pmodel, &punit)

	moves, err := s.kardexMovements(r, pid, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	qtyEntradas, totalEntradas, qtySalidas, totalSalidas := kardexTotales(moves)

	var finalQty, finalTotal float64
	if len(moves) > 0 {
		last := moves[len(moves)-1]
		finalQty, finalTotal = last.QtyBalance, last.TotalBalance
	}

	s.renderStandalone(r, w, "kardex_print.html", map[string]interface{}{
		"Title":         "Libro de Inventario — Entradas y Salidas (Art. 177 ISLR)",
		"ProductID":     pid,
		"Product":       pname.String,
		"Model":         pmodel.String,
		"Unit":          punit.String,
		"From":          from,
		"To":            to,
		"Rows":          moves,
		"QtyEntradas":   qtyEntradas,
		"TotalEntradas": totalEntradas,
		"QtySalidas":    qtySalidas,
		"TotalSalidas":  totalSalidas,
		"FinalQty":      finalQty,
		"FinalTotal":    finalTotal,
	})
}

// productosKardex devuelve la lista de productos para el selector.
func (s *Server) productosKardex(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.FormValue("term"))
	rows, err := s.dbFor(r).Query(`SELECT product_id, product_name, IFNULL(product_model,'') FROM product_information WHERE status=1 AND (product_name LIKE ? OR product_id LIKE ?) ORDER BY product_name LIMIT 50`, "%"+term+"%", "%"+term+"%")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type prod struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Name  string `json:"name"`
	}
	var list []prod
	for rows.Next() {
		var id, name, model sql.NullString
		_ = rows.Scan(&id, &name, &model)
		list = append(list, prod{ID: id.String, Label: name.String + " (" + model.String + ")", Name: name.String})
	}
	if list == nil {
		list = []prod{}
	}
	writeJSON(w, list)
}

// currentCPP devuelve el costo promedio ponderado de un producto con todo su
// histórico: (costo total comprado) / (cantidad total comprada).
func (s *Server) currentCPP(r *http.Request, productID string) float64 {
	var totalQty, totalValue float64
	_ = s.dbFor(r).QueryRow(`
		SELECT IFNULL(SUM(quantity),0), IFNULL(SUM(total_amount),0)
		FROM product_purchase_details WHERE product_id = ?`, productID).Scan(&totalQty, &totalValue)
	if totalQty > 0 {
		return totalValue / totalQty
	}
	return 0
}
