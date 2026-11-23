package web

// price_list.go — Gestión de listas de precios y descuentos por cliente.

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// registerPriceListRoutes registra las rutas del módulo de precios.
func (s *Server) registerPriceListRoutes() {
	s.mux.HandleFunc("GET /price_list/list", s.priceListList)
	// Las rutas /data se registran SIN método: las tablas DataTables de las
	// vistas hacen sus peticiones ajax con type: 'POST', y el ServeMux
	// rechazaría con 405 todo verbo no declarado (así lo hacen los listados
	// que sí cargan: customer, invoice, product, purchase, return).
	s.mux.HandleFunc("/price_list/data", s.priceListData)
	s.mux.HandleFunc("POST /price_list/save", s.priceListSave)
	// POST (no GET): activa una lista con dos UPDATE (S-10 de la auditoría) —
	// una navegación GET de nivel superior con SameSite=Lax enviaba la cookie
	// y un enlace preparado cambiaba la lista de precios de la empresa.
	s.mux.HandleFunc("POST /price_list/activate/{id}", s.priceListActivate)
	s.mux.HandleFunc("POST /price_list/delete/{id}", s.priceListDelete)
	s.mux.HandleFunc("GET /product_price/list", s.productPriceList)
	s.mux.HandleFunc("/product_price/data", s.productPriceData)
	s.mux.HandleFunc("POST /product_price/save", s.productPriceSave)
	s.mux.HandleFunc("GET /product_price/discount/list", s.discountList)
	s.mux.HandleFunc("GET /product_price/discount/form", s.discountForm)
	s.mux.HandleFunc("/product_price/discount/data", s.discountData)
	s.mux.HandleFunc("POST /product_price/discount/save", s.discountSave)
	s.mux.HandleFunc("POST /product_price/discount/delete/{id}", s.discountDelete)
}

// ---- LISTAS DE PRECIO ----

func (s *Server) priceListList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "price_list_list.html", map[string]interface{}{"Title": "Listas de Precio"})
}

func (s *Server) priceListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query("SELECT id, code, name, sort_order, is_default, currency_code, status FROM price_list ORDER BY sort_order")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id, sortOrder, isDefault int
		var code, name, currency, status string
		rows.Scan(&id, &code, &name, &sortOrder, &isDefault, &currency, &status)
		list = append(list, map[string]interface{}{
			"id": id, "code": code, "name": name,
			"sort_order": sortOrder, "is_default": isDefault,
			"currency_code": currency, "status": status,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

func (s *Server) priceListSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "price_list", "save") {
		jsonError(w, "Sin permisos para modificar listas de precio", 403)
		return
	}

	idStr := r.FormValue("id")
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	currency := strings.TrimSpace(r.FormValue("currency_code"))
	if currency == "" {
		currency = "VES"
	}
	isDefault, _ := strconv.Atoi(r.FormValue("is_default"))
	status := orDefault(r.FormValue("status"), "active")

	if code == "" || name == "" {
		jsonError(w, "Código y nombre son obligatorios", 400)
		return
	}

	if idStr == "" {
		_, err := s.dbFor(r).Exec(
			"INSERT INTO price_list (code, name, sort_order, is_default, currency_code, status) VALUES (?, ?, ?, ?, ?, ?)",
			code, name, sortOrder, isDefault, currency, status)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		id, _ := strconv.Atoi(idStr)
		_, err := s.dbFor(r).Exec(
			"UPDATE price_list SET code=?, name=?, sort_order=?, is_default=?, currency_code=?, status=? WHERE id=?",
			code, name, sortOrder, isDefault, currency, status, id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Lista de precio guardada"})
}

func (s *Server) priceListActivate(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "price_list", "activate") {
		jsonError(w, "Sin permisos para activar listas de precio", 403)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}
	_, _ = s.dbFor(r).Exec("UPDATE price_list SET is_default = 0")
	_, err := s.dbFor(r).Exec("UPDATE price_list SET is_default = 1, status = 'active' WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Lista activada"})
}

func (s *Server) priceListDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}
	_, err := s.dbFor(r).Exec("DELETE FROM price_list WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Lista eliminada"})
}

// ---- PRECIOS DE PRODUCTOS ----

func (s *Server) productPriceList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "product_price_list.html", map[string]interface{}{"Title": "Precios de Productos"})
}

func (s *Server) productPriceData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT pi.product_id, pi.product_name, pi.price, pi.price_usd, pi.currency_code,
		       COALESCE(ppl1.price, 0) as price_detalle,
		       COALESCE(ppl2.price, 0) as price_mayor,
		       COALESCE(ppl3.price, 0) as price_gran_mayor
		FROM product_information pi
		LEFT JOIN product_price_list ppl1 ON ppl1.product_id = pi.product_id AND ppl1.price_list_id = 1
		LEFT JOIN product_price_list ppl2 ON ppl2.product_id = pi.product_id AND ppl2.price_list_id = 2
		LEFT JOIN product_price_list ppl3 ON ppl3.product_id = pi.product_id AND ppl3.price_list_id = 3
		ORDER BY pi.product_name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var productID int
		var productName, currencyCode string
		var price, priceUSD, priceDetalle, priceMayor, priceGranMayor float64
		rows.Scan(&productID, &productName, &price, &priceUSD, &currencyCode, &priceDetalle, &priceMayor, &priceGranMayor)
		list = append(list, map[string]interface{}{
			"product_id": productID, "product_name": productName,
			"price": price, "price_usd": priceUSD, "currency_code": currencyCode,
			"price_detalle": priceDetalle, "price_mayor": priceMayor, "price_gran_mayor": priceGranMayor,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

func (s *Server) productPriceSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}

	// Recibir arrays: product_id[], price_list_id[], price[]
	products := r.Form["product_id"]
	prices := r.Form["price"]

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for i, pidStr := range products {
		pid, _ := strconv.Atoi(pidStr)
		if pid == 0 {
			continue
		}
		price := 0.0
		if i < len(prices) {
			price, _ = strconv.ParseFloat(prices[i], 64)
		}

		// Determinar lista por defecto (detalle=1, mayor=2, gran_mayor=3)
		// Aquí se asume que se está guardando el precio de la lista detalle
		priceListID := 1
		_ = priceListID

		_, _ = tx.Exec(`INSERT INTO product_price_list (product_id, price_list_id, price, updated_at)
			VALUES (?, ?, ?, NOW()) ON DUPLICATE KEY UPDATE price = VALUES(price), updated_at = NOW()`,
			pid, 1, price)
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Precios guardados"})
}

// ---- DESCUENTOS ----

func (s *Server) discountList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "discount_list.html", map[string]interface{}{"Title": "Descuentos por Cliente"})
}

// discountForm sirve la pantalla de alta/edición de descuentos. Antes era una
// plantilla huérfana: los enlaces de discount_list.html apuntaban a
// /product_price/discount/form, que no estaba registrada y caía en 404, así que
// "Nuevo descuento" no se podía abrir.
func (s *Server) discountForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":         "Nuevo Descuento",
		"DiscountID":    "",
		"DiscountPct":   "0.00",
		"DiscountMinQty": "0",
	}
	if idStr := r.FormValue("id"); idStr != "" {
		id, _ := strconv.Atoi(idStr)
		if id > 0 {
			// Carga el registro paraEditar; si no existe, se crea en blanco
			// (mejor UX que un 404: el técnico vino a escribir, no a leer).
			var prodID, custID, priceListID, minQty int
			var custClass, vFrom, vUntil, reason, status string
			var discPct float64
			err := s.dbFor(r).QueryRow(`SELECT product_id, customer_id, customer_class, price_list_id,
				discount_pct, min_qty, valid_from, valid_until, reason, status
				FROM product_price_discount WHERE id = ?`, id).
				Scan(&prodID, &custID, &custClass, &priceListID, &discPct, &minQty, &vFrom, &vUntil, &reason, &status)
			if err == nil {
				data["Title"] = "Editar Descuento"
				data["DiscountID"] = id
				data["DiscountProductID"] = prodID
				data["DiscountCustomerID"] = custID
				data["DiscountClass"] = custClass
				data["DiscountPriceList"] = priceListID
				data["DiscountPct"] = strconv.FormatFloat(discPct, 'f', 2, 64)
				data["DiscountMinQty"] = strconv.Itoa(minQty)
				data["ValidFrom"] = vFrom
				data["ValidUntil"] = vUntil
				data["Reason"] = reason
				data["DiscountStatus"] = status
			}
		}
	}
	// Catálogos para los <select>: productos activos y clientes. Si la tabla no
	// existe la query tira error y los listados quedan vacíos (no se rompe la
	// página).
	if rows, err := s.dbFor(r).Query(`SELECT product_id, product_name FROM product_information WHERE status=1 ORDER BY product_name`); err == nil {
		var prods []map[string]interface{}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err == nil {
				prods = append(prods, map[string]interface{}{"id": id, "name": name})
			}
		}
		_ = rows.Close()
		data["Products"] = prods
	}
	if rows, err := s.dbFor(r).Query(`SELECT customer_id, customer_name FROM customer_information ORDER BY customer_name`); err == nil {
		var custs []map[string]interface{}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err == nil {
				custs = append(custs, map[string]interface{}{"id": id, "name": name})
			}
		}
		_ = rows.Close()
		data["Customers"] = custs
	}
	s.render(w, r, "discount.html", data)
}

func (s *Server) discountData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT ppd.id, ppd.product_id, pi.product_name, ppd.customer_id,
		       COALESCE(ci.customer_name, 'TODOS') as customer_name,
		       ppd.customer_class, ppd.price_list_id, ppd.discount_pct,
		       ppd.min_qty, ppd.valid_from, ppd.valid_until, ppd.reason, ppd.status
		FROM product_price_discount ppd
		LEFT JOIN product_information pi ON pi.product_id = ppd.product_id
		LEFT JOIN customer_information ci ON ci.customer_id = ppd.customer_id
		ORDER BY ppd.id DESC`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id, productID, customerID, minQty int
		var productName, customerName, customerClass, reason, status string
		var discountPct float64
		var validFrom, validUntil string
		var priceListID int
		rows.Scan(&id, &productID, &productName, &customerID, &customerName, &customerClass, &priceListID, &discountPct, &minQty, &validFrom, &validUntil, &reason, &status)
		list = append(list, map[string]interface{}{
			"id": id, "product_id": productID, "product_name": productName,
			"customer_id": customerID, "customer_name": customerName,
			"customer_class": customerClass, "price_list_id": priceListID,
			"discount_pct": discountPct, "min_qty": minQty,
			"valid_from": validFrom, "valid_until": validUntil,
			"reason": reason, "status": status,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

func (s *Server) discountSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "discount", "save") {
		jsonError(w, "Sin permisos para gestionar descuentos", 403)
		return
	}

	productID, _ := strconv.Atoi(r.FormValue("product_id"))
	if productID == 0 {
		jsonError(w, "Producto requerido", 400)
		return
	}

	customerID, _ := strconv.Atoi(r.FormValue("customer_id"))
	customerClass := strings.TrimSpace(r.FormValue("customer_class"))
	priceListID, _ := strconv.Atoi(r.FormValue("price_list_id"))
	discountPct, _ := strconv.ParseFloat(r.FormValue("discount_pct"), 64)
	minQty, _ := strconv.ParseFloat(r.FormValue("min_qty"), 64)
	validFrom := r.FormValue("valid_from")
	validUntil := r.FormValue("valid_until")
	reason := strings.TrimSpace(r.FormValue("reason"))
	status := orDefault(r.FormValue("status"), "active")

	if discountPct < 0 || discountPct > 100 {
		jsonError(w, "Descuento debe ser entre 0 y 100", 400)
		return
	}

	_, err := s.dbFor(r).Exec(`
		INSERT INTO product_price_discount (product_id, customer_id, customer_class, price_list_id, discount_pct, min_qty, valid_from, valid_until, reason, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
		productID, sql.NullInt64{Int64: int64(customerID), Valid: customerID > 0},
		sql.NullString{String: customerClass, Valid: customerClass != ""},
		sql.NullInt64{Int64: int64(priceListID), Valid: priceListID > 0},
		discountPct, minQty,
		sql.NullString{String: validFrom, Valid: validFrom != ""},
		sql.NullString{String: validUntil, Valid: validUntil != ""},
		reason, status)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Descuento creado"})
}

func (s *Server) discountDelete(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "discount", "delete") {
		jsonError(w, "Sin permisos para eliminar descuentos", 403)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "ID requerido", 400)
		return
	}
	_, err := s.dbFor(r).Exec("DELETE FROM product_price_discount WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Descuento eliminado"})
}

// ---- FUNCIONES DE CÁLCULO DE PRECIO ----

// precioDeLista devuelve el precio de un producto en una lista dada.
// Si no hay precio en la lista, usa el precio default de product_information.
func (s *Server) precioDeLista(r *http.Request, productID int, priceListID int) (float64, error) {
	var price float64
	err := s.dbFor(r).QueryRow(`
		SELECT COALESCE(
			(SELECT price FROM product_price_list WHERE product_id = ? AND price_list_id = ?),
			(SELECT CAST(price AS DECIMAL(16,4)) FROM product_information WHERE product_id = ?)
		)`, productID, priceListID, productID).Scan(&price)
	return price, err
}

// descuentoActivo devuelve el descuento aplicable a un producto para un cliente y lista.
// Busca por prioridad: cliente+producto+lista → clase+producto+lista → producto+lista.
func (s *Server) descuentoActivo(r *http.Request, productID int, customerID int, priceListID int) float64 {
	var discount float64
	today := time.Now().Format("2006-01-02")
	minQty := 0.0 // TODO: pasar qty como parámetro

	// 1. Buscar por cliente específico
	_ = s.dbFor(r).QueryRow(`
		SELECT discount_pct FROM product_price_discount
		WHERE product_id = ? AND customer_id = ? AND price_list_id = ?
		  AND status = 'active' AND (valid_from IS NULL OR valid_from <= ?) AND (valid_until IS NULL OR valid_until >= ?)
		  AND min_qty <= ?`,
		productID, customerID, priceListID, today, today, minQty).Scan(&discount)
	if discount > 0 {
		return discount
	}

	// 2. Buscar por clase de cliente
	var customerClass string
	_ = s.dbFor(r).QueryRow("SELECT customer_class FROM customer_information WHERE customer_id = ?", customerID).Scan(&customerClass)
	if customerClass != "" {
		_ = s.dbFor(r).QueryRow(`
			SELECT discount_pct FROM product_price_discount
			WHERE product_id = ? AND customer_class = ? AND price_list_id = ?
			  AND status = 'active' AND (valid_from IS NULL OR valid_from <= ?) AND (valid_until IS NULL OR valid_until >= ?)
			  AND min_qty <= ?`,
			productID, customerClass, priceListID, today, today, minQty).Scan(&discount)
		if discount > 0 {
			return discount
		}
	}

	// 3. Buscar por producto+lista (cualquier cliente)
	_ = s.dbFor(r).QueryRow(`
		SELECT discount_pct FROM product_price_discount
		WHERE product_id = ? AND price_list_id = ? AND customer_id IS NULL AND customer_class IS NULL
		  AND status = 'active' AND (valid_from IS NULL OR valid_from <= ?) AND (valid_until IS NULL OR valid_until >= ?)
		  AND min_qty <= ?`,
		productID, priceListID, today, today, minQty).Scan(&discount)
	return discount
}

// precioFinal calcula el precio final de un producto para un cliente y cantidad.
func (s *Server) precioFinal(r *http.Request, productID int, customerID int, priceListID int, qty float64) (precio float64, descuento float64, final float64, err error) {
	// 1. Precio de lista o default
	precio, err = s.precioDeLista(r, productID, priceListID)
	if err != nil {
		return 0, 0, 0, err
	}

	// 2. Descuento (usando qty para verificar min_qty)
	_ = qty // TODO: usar qty en descuentoActivo
	descuento = s.descuentoActivo(r, productID, customerID, priceListID)

	// 3. Aplicar descuento
	final = precio * (1 - descuento/100)
	return precio, descuento, final, nil
}
