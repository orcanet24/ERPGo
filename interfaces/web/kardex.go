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
	Almacen      string  `json:"warehouse"`
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
	// UNA SOLA FUENTE: `stock_movement`.
	//
	// Antes el libro mezclaba compras leídas de `product_purchase_details`,
	// ventas de `stock_movement`, devoluciones de `product_return` y ajustes de
	// `store_adjustment`: cuatro fuentes que podían no coincidir entre sí. Y la
	// consulta de "venta de servicio" leía una columna que no existía
	// (`product_information.product_type`), fallaba y se saltaba en silencio.
	//
	// Ahora todo movimiento de inventario se escribe en stock_movement con su
	// almacén. El libro se arma de ahí: si algo no está en el libro, es porque
	// no movió inventario. Un servicio, por definición, no aparece.
	// La pantalla recibe el CÓDIGO del producto y `stock_movement.product_id`
	// es el id interno numérico: comparar el código contra la columna lo
	// convertía en 0 y el libro salía «Sin datos» aunque la compra y la venta
	// estuvieran registradas. La conversión es la misma que usa el resto del
	// inventario (idInternoProducto).
	pidInt := idInternoProducto(s.dbFor(r), pid)
	rows, err := s.dbFor(r).Query(`
		SELECT date, movement_type, IFNULL(reference_no,''), quantity, IFNULL(unit_cost,0),
			IFNULL(warehouse_id,'')
		FROM stock_movement
		WHERE product_id = ? AND date BETWEEN ? AND ? AND IFNULL(is_posted,1) = 1
		ORDER BY date ASC, id ASC`, pidInt, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moves []kardexMovement
	for rows.Next() {
		var fecha, tipo, doc, almacen sql.NullString
		var qty, costo float64
		if err := rows.Scan(&fecha, &tipo, &doc, &qty, &costo, &almacen); err != nil {
			continue
		}
		m := kardexMovement{Date: fecha.String, DocNumber: doc.String, Almacen: almacen.String}
		switch tipo.String {
		case "purchase":
			m.Concept, m.DocType = "Compra", "FACTURA_COMPRA"
			m.QtyIn, m.CostIn = qty, costo
			m.TotalIn = round2(qty * costo)
		case "return_customer":
			m.Concept, m.DocType = "Devolución venta", "DEVOLUCION_VENTA"
			m.QtyIn, m.CostIn = qty, costo
			m.TotalIn = round2(qty * costo)
		case "adjustment_add", "transfer_in":
			if tipo.String == "transfer_in" {
				m.Concept, m.DocType = "Transferencia entrada", "TRANSFERENCIA"
			} else {
				m.Concept, m.DocType = "Ajuste (+)", "AJUSTE_SOBRANTE"
			}
			m.QtyIn = qty
			// Un ajuste que suma no trae costo propio: entra al promedio vigente.
			m.valorarAlCPP = true
		case "sale":
			m.Concept, m.DocType = "Venta", "FACTURA_VENTA"
			m.QtyOut = qty
		case "delivery":
			m.Concept, m.DocType = "Despacho", "DESPACHO"
			m.QtyOut = qty
		case "adjustment_remove":
			m.Concept, m.DocType = "Ajuste (-)", "AJUSTE_FALTANTE"
			m.QtyOut = qty
		case "return_supplier":
			m.Concept, m.DocType = "Devolución compra", "DEVOLUCION_COMPRA"
			m.QtyOut = qty
		case "consumo":
			m.Concept, m.DocType = "Consumo interno", "CONSUMO_INTERNO"
			m.QtyOut = qty
		case "transfer_out":
			m.Concept, m.DocType = "Transferencia salida", "TRANSFERENCIA"
			m.QtyOut = qty
		default:
			continue
		}
		moves = append(moves, m)
	}

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
	var pname, pmodel, punit, itemType sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT product_name, IFNULL(product_model,''), IFNULL(unit,''), IFNULL(item_type,'venta') FROM product_information WHERE product_id = ?`, pid).
		Scan(&pname, &pmodel, &punit, &itemType)

	moves, err := s.kardexMovements(r, pid, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Un servicio no tiene kárdex. Se dice, en vez de mostrar un libro vacío
	// que parecería un error de carga.
	aviso := ""
	if normalizarTipoItem(itemType.String) == ItemServicio {
		aviso = "Este artículo es un servicio o gasto: no tiene existencias ni movimientos de kárdex. Su costo se registra en la cuenta de gasto que se elige al comprarlo."
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
		"Aviso":         aviso,
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
