package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Delivery Order module (warehouse order management) ----------

func (s *Server) registerDeliveryOrderRoutes() {
	s.mux.HandleFunc("GET /delivery_order/list", s.deliveryOrderList)
	// Las rutas /data se registran SIN método: las tablas DataTables de las
	// vistas hacen sus peticiones ajax con type: 'POST', y el ServeMux
	// rechazaría con 405 todo verbo no declarado (así lo hacen los listados
	// que sí cargan: customer, invoice, product, purchase, return).
	s.mux.HandleFunc("/delivery_order/data", s.deliveryOrderData)
	s.mux.HandleFunc("GET /delivery_order/add", s.deliveryOrderForm)
	s.mux.HandleFunc("GET /delivery_order/edit/{id}", s.deliveryOrderForm)
	s.mux.HandleFunc("POST /delivery_order/save", s.deliveryOrderSave)
	s.mux.HandleFunc("POST /delivery_order/delete/{id}", s.deliveryOrderDelete)
	s.mux.HandleFunc("POST /delivery_order/update-status/{id}", s.deliveryOrderUpdateStatus)
}

func (s *Server) deliveryOrderList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "delivery_order_list.html", map[string]interface{}{"Title": "Órdenes de Entrega"})
}

func (s *Server) deliveryOrderData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE CONCAT('DO-', do.id) LIKE ? OR c.customer_name LIKE ? OR do.status LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM delivery_order").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM delivery_order do JOIN customer_information c ON c.customer_id = do.customer_id"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT do.id, do.customer_id, IFNULL(c.customer_name, ''), do.status,
		       do.date, do.warehouse_id, do.notes, do.created_at
		FROM delivery_order do
		JOIN customer_information c ON c.customer_id = do.customer_id
		`+where+`
		ORDER BY do.id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id, customerID int
		var customerName, status, date, warehouse, notes, createdAt string
		if err := rows.Scan(&id, &customerID, &customerName, &status, &date, &warehouse, &notes, &createdAt); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":          id,
			"customer_id": customerID,
			"customer":    customerName,
			"status":      status,
			"date":        date,
			"warehouse":   warehouse,
			"notes":       notes,
			"created_at":  createdAt,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

func (s *Server) deliveryOrderForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":    "Nueva Orden de Entrega",
		"Statuses": deliveryOrderStatuses(),
	}

	if id := r.PathValue("id"); id != "" {
		var customerID int
		var warehouse, status, date, notes, createdAt, deliveredAt string
		err := s.dbFor(r).QueryRow(`
			SELECT customer_id, warehouse_id, status, date, notes, delivered_at, created_at
			FROM delivery_order WHERE id = ?`, id).
			Scan(&customerID, &warehouse, &status, &date, &notes, &deliveredAt, &createdAt)
		if err != nil {
			http.Error(w, "Orden de entrega no encontrada", 404)
			return
		}

		data["Title"] = "Editar Orden de Entrega #" + id
		data["EditID"] = id
		data["CustomerID"] = customerID
		data["Warehouse"] = warehouse
		data["Status"] = status
		data["Date"] = date
		data["Notes"] = notes
		data["DeliveredAt"] = deliveredAt

		details := []map[string]interface{}{}
		rows, _ := s.dbFor(r).Query(`SELECT id, product_id, quantity FROM delivery_order_detail WHERE delivery_order_id = ?`, id)
		if rows != nil {
			for rows.Next() {
				var detailID, productID int
				var quantity float64
				_ = rows.Scan(&detailID, &productID, &quantity)
				details = append(details, map[string]interface{}{
					"id":         detailID,
					"product_id": productID,
					"quantity":   quantity,
				})
			}
			rows.Close()
		}
		data["Details"] = details
	}

	s.render(w, r, "delivery_order.html", data)
}

func (s *Server) deliveryOrderSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	r.ParseMultipartForm(32 << 20)

	customerID, _ := strconv.Atoi(r.FormValue("customer_id"))
	if customerID == 0 {
		jsonError(w, "Cliente es obligatorio", 400)
		return
	}

	warehouse := strings.TrimSpace(r.FormValue("warehouse_id"))
	if warehouse == "" {
		warehouse = s.almacenPorDefecto(r, AlmacenTipoVenta)
	}
	date := today()
	if d := r.FormValue("date"); d != "" {
		date = d
	}
	notes := strings.TrimSpace(r.FormValue("notes"))

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var doID int
	if idStr := r.FormValue("id"); idStr != "" && idStr != "0" {
		doID, _ = strconv.Atoi(idStr)
		_, err := tx.Exec(`
			UPDATE delivery_order SET customer_id=?, warehouse_id=?, date=?, notes=?
			WHERE id=?`,
			customerID, warehouse, date, notes, doID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if _, err := tx.Exec("DELETE FROM delivery_order_detail WHERE delivery_order_id = ?", doID); err != nil {
			_ = tx.Rollback()
			jsonError(w, "no se pudo limpiar las líneas de la orden de despacho", 500)
			return
		}
	} else {
		res, err := tx.Exec(`
			INSERT INTO delivery_order (customer_id, warehouse_id, status, date, notes, created_at)
			VALUES (?, ?, 'draft', ?, ?, NOW())`,
			customerID, warehouse, date, notes)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		id, _ := res.LastInsertId()
		doID = int(id)
	}

	// Insert detail rows from arrays product_id[] and quantity[]
	pids := r.Form["product_id"]
	qtys := r.Form["quantity"]
	for i, pidStr := range pids {
		pid, _ := strconv.Atoi(pidStr)
		if pid == 0 {
			continue
		}
		qty := 0.0
		if i < len(qtys) {
			qty, _ = strconv.ParseFloat(qtys[i], 64)
		}
		if qty <= 0 {
			continue
		}
		_, err := tx.Exec(`
			INSERT INTO delivery_order_detail (delivery_order_id, product_id, quantity)
			VALUES (?, ?, ?)`, doID, pid, qty)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Orden de entrega guardada", "id": doID})
}

func (s *Server) deliveryOrderDelete(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	var status string
	_ = s.dbFor(r).QueryRow("SELECT status FROM delivery_order WHERE id = ?", id).Scan(&status)
	if status != "draft" && status != "" {
		jsonError(w, "Solo se pueden eliminar órdenes en estado borrador", 400)
		return
	}

	if _, err := s.dbFor(r).Exec("DELETE FROM delivery_order_detail WHERE delivery_order_id = ?", id); err != nil {
		jsonError(w, "no se pudo eliminar las líneas de la orden de despacho", 500)
		return
	}
	_, err := s.dbFor(r).Exec("DELETE FROM delivery_order WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Orden de entrega eliminada"})
}

func (s *Server) deliveryOrderUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	newStatus := strings.TrimSpace(r.FormValue("status"))
	if newStatus == "" {
		jsonError(w, "Estado requerido", 400)
		return
	}

	var currentStatus string
	err := s.dbFor(r).QueryRow("SELECT status FROM delivery_order WHERE id = ?", id).Scan(&currentStatus)
	if err != nil {
		jsonError(w, "Orden no encontrada", 404)
		return
	}

	if !isValidStatusTransition(currentStatus, newStatus) {
		jsonError(w, "Transición de estado no permitida: "+currentStatus+" → "+newStatus, 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// delivered_at se asigna con NOW() al marcar entregado; si no, se limpia a NULL.
	// No viene del usuario, así que la columna va literal en la sentencia (no es
	// parametizable porque NULL y NOW() son expresiones, no valores).
	deliveredAtExpr := "NULL"
	if newStatus == "delivered" {
		deliveredAtExpr = "NOW()"
	}
	_, err = tx.Exec("UPDATE delivery_order SET status=?, delivered_at="+deliveredAtExpr+" WHERE id=?", newStatus, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// When transitioning to shipped or delivered, create stock_movement records
	if newStatus == "shipped" || newStatus == "delivered" {
		// El almacén es el del despacho, no un código fijo; y un servicio no
		// genera movimiento porque no tiene existencias.
		var almacenDespacho string
		_ = tx.QueryRow(`SELECT IFNULL(warehouse_id,'') FROM delivery_order WHERE id = ?`, id).Scan(&almacenDespacho)
		if almacenDespacho == "" {
			almacenDespacho = s.almacenPorDefecto(r, AlmacenTipoVenta)
		}
		rows, err := tx.Query(`
			SELECT d.product_id, d.quantity, IFNULL(p.item_type,'venta')
			FROM delivery_order_detail d
			LEFT JOIN product_information p ON p.product_id = d.product_id
			WHERE d.delivery_order_id = ?`, id)
		if err == nil {
			for rows.Next() {
				var productID int
				var qty float64
				var tipo string
				rows.Scan(&productID, &qty, &tipo)
				if !itemMueveStock(tipo) {
					continue
				}
				if _, err := tx.Exec(`
					INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost, reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
					VALUES ('delivery', ?, ?, ?, 0, 'delivery_order', ?, CONCAT('DO-',?), CURDATE(), 1, NOW(), NOW())`,
					productID, almacenDespacho, qty, id, id); err != nil {
					_ = tx.Rollback()
					jsonError(w, fmt.Sprintf("movimiento de inventario: %v", err), 500)
					return
				}
			}
			rows.Close()
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Estado actualizado a " + newStatus})
}

// deliveryOrderStatuses returns valid status values.
func deliveryOrderStatuses() []string {
	return []string{"draft", "confirmed", "picking", "packed", "shipped", "delivered", "cancelled"}
}

// isValidStatusTransition validates forward-only transitions with no skipping.
func isValidStatusTransition(current, next string) bool {
	transitions := map[string][]string{
		"draft":     {"confirmed", "cancelled"},
		"confirmed": {"picking", "cancelled"},
		"picking":   {"packed", "cancelled"},
		"packed":    {"shipped", "cancelled"},
		"shipped":   {"delivered", "cancelled"},
		"delivered": {},
		"cancelled": {},
	}
	for _, allowed := range transitions[current] {
		if allowed == next {
			return true
		}
	}
	return false
}
