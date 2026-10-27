package web

// order.go — Gestión de pedidos (órdenes de venta).
// Los pedidos reservan stock pero no descuentan hasta que se confirman o facturan.

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerOrderRoutes() {
	s.mux.HandleFunc("GET /order/list", s.orderList)
	s.mux.HandleFunc("GET /order/data", s.orderData)
	s.mux.HandleFunc("GET /order/add", s.orderForm)
	s.mux.HandleFunc("GET /order/edit/{id}", s.orderForm)
	s.mux.HandleFunc("POST /order/save", s.orderSave)
	s.mux.HandleFunc("POST /order/confirm/{id}", s.orderConfirm)
	s.mux.HandleFunc("POST /order/cancel/{id}", s.orderCancel)
	s.mux.HandleFunc("GET /order/details/{id}", s.orderDetails)
	s.mux.HandleFunc("POST /order/to-delivery/{id}", s.orderToDelivery)
}

// orderToDelivery convierte un pedido (sale_order) en una nota de salida (delivery_order).
// Cancela la reserva de stock del pedido para no duplicar el descuento.
func (s *Server) orderToDelivery(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "order", "confirm") {
		jsonError(w, "Sin permisos para convertir pedido a entrega", 403)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Verificar que el pedido existe y está confirmado
	var customerID int
	var orderDate string
	err = tx.QueryRow("SELECT customer_id, date FROM sale_order WHERE id = ? AND status = 'confirmed'", id).Scan(&customerID, &orderDate)
	if err != nil {
		jsonError(w, "Pedido no encontrado o no está confirmado", 404)
		return
	}

	// Obtener líneas del pedido
	rows, err := tx.Query("SELECT product_id, quantity FROM sale_order_detail WHERE sale_order_id = ?", id)
	if err != nil {
		jsonError(w, "Error leyendo líneas del pedido", 500)
		return
	}

	type detail struct {
		ProductID int
		Qty       float64
	}
	var details []detail
	for rows.Next() {
		var d detail
		rows.Scan(&d.ProductID, &d.Qty)
		details = append(details, d)
	}
	rows.Close()

	if len(details) == 0 {
		jsonError(w, "El pedido no tiene líneas", 400)
		return
	}

	// Crear delivery_order a partir del pedido
	res, err := tx.Exec(`
		INSERT INTO delivery_order (customer_id, date, status, notes, created_at)
		VALUES (?, ?, 'draft', ?, NOW())`,
		customerID, orderDate, "Generado desde pedido #"+id)
	if err != nil {
		jsonError(w, "Error creando nota de entrega: "+err.Error(), 500)
		return
	}
	deliveryOrderID, _ := res.LastInsertId()

	// Insertar líneas en delivery_order_detail
	for _, d := range details {
		_, err := tx.Exec(`
			INSERT INTO delivery_order_detail (delivery_order_id, product_id, quantity, status)
			VALUES (?, ?, ?, 'pending')`,
			int(deliveryOrderID), d.ProductID, d.Qty)
		if err != nil {
			jsonError(w, "Error insertando línea de entrega: "+err.Error(), 500)
			return
		}
	}

	// Cancelar reservas de stock del pedido (evitar doble descuento)
	_, _ = tx.Exec("UPDATE stock_reservation SET status = 'cancelled', cancelled_at = NOW() WHERE document_type = 'order' AND document_id = ?", id)

	// Actualizar estado del pedido
	_, _ = tx.Exec("UPDATE sale_order SET status = 'delivered' WHERE id = ?", id)

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":            1,
		"msg":               "Pedido convertido a nota de entrega",
		"delivery_order_id": deliveryOrderID,
	})
}

func (s *Server) orderList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "order_list.html", map[string]interface{}{"Title": "Pedidos"})
}

func (s *Server) orderData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT o.id, o.order_no, o.customer_id, COALESCE(ci.customer_name, '') as customer_name,
		       o.status, o.date, o.notes, o.created_at
		FROM sale_order o
		LEFT JOIN customer_information ci ON ci.customer_id = o.customer_id
		ORDER BY o.id DESC`)
	if err != nil {
		// Tabla no existe aún
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id, customerID int
		var orderNo, customerName, status, date, notes, createdAt string
		rows.Scan(&id, &orderNo, &customerID, &customerName, &status, &date, &notes, &createdAt)
		list = append(list, map[string]interface{}{
			"id": id, "order_no": orderNo, "customer_id": customerID,
			"customer_name": customerName, "status": status, "date": date,
			"notes": notes, "created_at": createdAt,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

func (s *Server) orderForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Nuevo Pedido"}
	if id := r.PathValue("id"); id != "" {
		var customerID int
		var orderNo, status, date, notes string
		err := s.dbFor(r).QueryRow(`
			SELECT customer_id, order_no, status, date, notes
			FROM sale_order WHERE id = ?`, id).Scan(&customerID, &orderNo, &status, &date, &notes)
		if err != nil {
			http.Error(w, "Pedido no encontrado", 404)
			return
		}
		data["Title"] = "Editar Pedido #" + id
		data["EditID"] = id
		data["CustomerID"] = customerID
		data["Status"] = status
		data["Date"] = date
		data["Notes"] = notes
	}
	s.render(w, r, "order.html", data)
}

func (s *Server) orderSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	customerID, _ := strconv.Atoi(r.FormValue("customer_id"))
	if customerID == 0 {
		jsonError(w, "Cliente requerido", 400)
		return
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

	// Crear pedido
	res, err := tx.Exec(`
		INSERT INTO sale_order (customer_id, status, date, notes, created_at)
		VALUES (?, 'draft', ?, ?, NOW())`,
		customerID, date, notes)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	orderID, _ := res.LastInsertId()

	// Actualizar order_no
	tx.Exec("UPDATE sale_order SET order_no = CONCAT('ORD-', LPAD(id, 6, '0')) WHERE id = ?", orderID)

	// Insertar líneas
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
			INSERT INTO sale_order_detail (sale_order_id, product_id, quantity)
			VALUES (?, ?, ?)`, int(orderID), pid, qty)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Pedido guardado", "id": orderID})
}

func (s *Server) orderConfirm(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	// Verificar permisos - solo roles autorizados
	if !s.hasPermission(r, "order", "confirm") {
		jsonError(w, "Sin permisos para confirmar pedidos", 403)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Actualizar estado
	_, err = tx.Exec("UPDATE sale_order SET status = 'confirmed' WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Crear reservas de stock
	rows, err := tx.Query("SELECT product_id, quantity FROM sale_order_detail WHERE sale_order_id = ?", id)
	if err == nil {
		for rows.Next() {
			var productID int
			var qty float64
			rows.Scan(&productID, &qty)
			_, _ = tx.Exec(`
				INSERT INTO stock_reservation (document_type, document_id, product_id, quantity, status, created_at)
				VALUES ('order', ?, ?, ?, 'pending', NOW())`,
				id, productID, qty)
		}
		rows.Close()
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Pedido confirmado"})
}

func (s *Server) orderCancel(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	// Verificar permisos
	if !s.hasPermission(r, "order", "cancel") {
		jsonError(w, "Sin permisos para cancelar pedidos", 403)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Cancelar pedido
	_, err = tx.Exec("UPDATE sale_order SET status = 'cancelled' WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Cancelar reservas
	_, err = tx.Exec("UPDATE stock_reservation SET status = 'cancelled', cancelled_at = NOW() WHERE document_type = 'order' AND document_id = ?", id)

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Pedido cancelado"})
}

func (s *Server) orderDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}

	var customerID, orderNo, status, date, notes string
	err := s.dbFor(r).QueryRow(`
		SELECT order_no, customer_id, status, date, notes
		FROM sale_order WHERE id = ?`, id).Scan(&orderNo, &customerID, &status, &date, &notes)
	if err != nil {
		http.Error(w, "Pedido no encontrado", 404)
		return
	}

	// Obtener líneas
	rows, _ := s.dbFor(r).Query(`
		SELECT sod.product_id, pi.product_name, sod.quantity
		FROM sale_order_detail sod
		LEFT JOIN product_information pi ON pi.product_id = sod.product_id
		WHERE sod.sale_order_id = ?`, id)

	details := []map[string]interface{}{}
	if rows != nil {
		for rows.Next() {
			var productID int
			var productName string
			var qty float64
			rows.Scan(&productID, &productName, &qty)
			details = append(details, map[string]interface{}{
				"product_id": productID, "product_name": productName, "quantity": qty,
			})
		}
		rows.Close()
	}

	writeJSON(w, map[string]interface{}{
		"id": id, "order_no": orderNo, "customer_id": customerID,
		"status": status, "date": date, "notes": notes, "details": details,
	})
}
