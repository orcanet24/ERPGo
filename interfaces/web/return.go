package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Return module (replicates product_return / sales_return) ----------

func (s *Server) registerReturnRoutes() {
	s.mux.HandleFunc("GET /return/list", s.returnList)
	s.mux.HandleFunc("/return/list/data", s.returnListData)
	s.mux.HandleFunc("GET /return/new", s.returnForm)
	s.mux.HandleFunc("POST /return/customer/save", s.returnCustomerSave)
	s.mux.HandleFunc("POST /return/supplier/save", s.returnSupplierSave)
}

// returnCustomerSave devolución de cliente - reversa stock_movement de venta.
func (s *Server) returnCustomerSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "return", "customer") {
		jsonError(w, "Sin permisos para devoluciones de cliente", 403)
		return
	}

	invoiceID := r.FormValue("invoice_id")
	productID, _ := strconv.Atoi(r.FormValue("product_id"))
	qty, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	reason := strings.TrimSpace(r.FormValue("reason"))

	if productID == 0 || qty <= 0 {
		jsonError(w, "Producto y cantidad requeridos", 400)
		return
	}
	if !s.requiresStockMovement(r, productID) {
		jsonError(w, "Producto no físico", 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var originalMovementID int
	var originalQty float64
	err = tx.QueryRow(`
		SELECT id, quantity FROM stock_movement 
		WHERE product_id = ? AND movement_type IN ('sale', 'delivery') AND reference_id = ?
		ORDER BY id DESC LIMIT 1`, productID, invoiceID).Scan(&originalMovementID, &originalQty)
	if err != nil {
		jsonError(w, "Movimiento original no encontrado", 404)
		return
	}
	if qty > originalQty {
		jsonError(w, fmt.Sprintf("Excede cantidad vendida (%.2f > %.2f)", qty, originalQty), 400)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO stock_return (return_type, reference_type, reference_id, product_id, quantity, reason, status, created_at)
		VALUES ('customer', 'invoice', ?, ?, ?, ?, 'confirmed', NOW())`,
		invoiceID, productID, qty, reason)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
			reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
		VALUES ('return_customer', ?, 'WH-MAIN', ?, 0, 'return', ?, CONCAT('RET-C-',?), CURDATE(), 1, NOW(), NOW())`,
		productID, qty, originalMovementID, originalMovementID)
	if err != nil {
		jsonError(w, "Error registrando movimiento: "+err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Devolución registrada"})
}

// returnSupplierSave devolución a proveedor - salida de stock.
func (s *Server) returnSupplierSave(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	if !s.hasPermission(r, "return", "supplier") {
		jsonError(w, "Sin permisos para devoluciones a proveedor", 403)
		return
	}

	purchaseID := r.FormValue("purchase_id")
	productID, _ := strconv.Atoi(r.FormValue("product_id"))
	qty, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	reason := strings.TrimSpace(r.FormValue("reason"))

	if productID == 0 || qty <= 0 {
		jsonError(w, "Producto y cantidad requeridos", 400)
		return
	}
	if !s.requiresStockMovement(r, productID) {
		jsonError(w, "Producto no físico", 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var originalMovementID int
	var originalQty float64
	err = tx.QueryRow(`
		SELECT id, quantity FROM stock_movement 
		WHERE product_id = ? AND movement_type = 'purchase' AND reference_id = ?
		ORDER BY id DESC LIMIT 1`, productID, purchaseID).Scan(&originalMovementID, &originalQty)
	if err != nil {
		jsonError(w, "Movimiento de compra no encontrado", 404)
		return
	}
	if qty > originalQty {
		jsonError(w, fmt.Sprintf("Excede cantidad comprada (%.2f > %.2f)", qty, originalQty), 400)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO stock_return (return_type, reference_type, reference_id, product_id, quantity, reason, status, created_at)
		VALUES ('supplier', 'purchase', ?, ?, ?, ?, 'confirmed', NOW())`,
		purchaseID, productID, qty, reason)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
			reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
		VALUES ('return_supplier', ?, 'WH-MAIN', ?, 0, 'return', ?, CONCAT('RET-S-',?), CURDATE(), 1, NOW(), NOW())`,
		productID, -qty, originalMovementID, originalMovementID)
	if err != nil {
		jsonError(w, "Error registrando movimiento: "+err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Devolución a proveedor registrada"})
}

func (s *Server) returnList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "return_list.html", map[string]interface{}{"Title": "Devoluciones"})
}

func (s *Server) returnListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	search := strings.TrimSpace(r.FormValue("search[value]"))

	where := ""
	args := []interface{}{}
	if search != "" {
		where = " WHERE r.return_id LIKE ? OR p.product_name LIKE ? OR c.customer_name LIKE ? OR s.supplier_name LIKE ?"
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_return").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_return r LEFT JOIN product_information p ON p.product_id = r.product_id LEFT JOIN customer_information c ON c.customer_id = r.customer_id LEFT JOIN supplier_information s ON s.supplier_id = r.supplier_id"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT r.return_id, r.product_id, IFNULL(p.product_name,'') AS product_name,
			r.invoice_id, r.purchase_id, r.date_purchase, r.date_return,
			r.byy_qty, r.ret_qty, r.product_rate, r.deduction, r.total_deduct,
			r.total_tax, r.total_ret_amount, r.net_total_amount, r.reason, r.usablity,
			IFNULL(c.customer_name,'') AS customer, IFNULL(s.supplier_name,'') AS supplier
		FROM product_return r
		LEFT JOIN product_information p ON p.product_id = r.product_id
		LEFT JOIN customer_information c ON c.customer_id = r.customer_id
		LEFT JOIN supplier_information s ON s.supplier_id = r.supplier_id
		`+where+`
		ORDER BY r.date_return DESC LIMIT 100`, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var rid, pid, invID, purID, datePur, dateRet, reason sql.NullString
		var byyQty, retQty, rate, deduct, totalDeduct, tax, totalRet, netAmount sql.NullFloat64
		var usablity int
		var customer, supplier sql.NullString
		_ = rows.Scan(&rid, &pid, &product_name, &invID, &purID, &datePur, &dateRet,
			&byyQty, &retQty, &rate, &deduct, &totalDeduct, &tax, &totalRet, &netAmount,
			&reason, &usablity, &customer, &supplier)
		list = append(list, map[string]interface{}{
			"return_id": rid.String, "product_id": pid.String, "product_name": product_name.String,
			"invoice_id": invID.String, "purchase_id": purID.String,
			"date_purchase": datePur.String, "date_return": dateRet.String,
			"byy_qty": byyQty.Float64, "ret_qty": retQty.Float64, "rate": rate.Float64,
			"deduction": deduct.Float64, "total_deduct": totalDeduct.Float64,
			"total_tax": tax.Float64, "total_ret_amount": totalRet.Float64,
			"net_total_amount": netAmount.Float64, "reason": reason.String,
			"usablity": usablity, "customer": customer.String, "supplier": supplier.String,
		})
	}
	writeJSON(w, map[string]interface{}{"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list})
}

func (s *Server) returnForm(w http.ResponseWriter, r *http.Request) {
	// Get type parameter (sale or purchase)
	rtype := r.FormValue("type")
	if rtype == "" {
		rtype = "sale"
	}

	data := map[string]interface{}{"Title": "Nueva Devolución", "ReturnType": rtype}

	// Load invoices or purchases for selection
	if rtype == "purchase" {
		rows, err := s.dbFor(r).Query(`SELECT purchase_id, chalan_no, purchase_date, grand_total_amount FROM product_purchase ORDER BY id DESC LIMIT 100`)
		if err == nil {
			defer rows.Close()
			var purchases []map[string]interface{}
			for rows.Next() {
				var pid, chalan, date sql.NullString
				var total float64
				_ = rows.Scan(&pid, &chalan, &date, &total)
				purchases = append(purchases, map[string]interface{}{"id": pid.String, "chalan": chalan.String, "date": date.String, "total": total})
			}
			data["Purchases"] = purchases
		}
	} else {
		rows, err := s.dbFor(r).Query(`SELECT invoice_id, date, total_amount FROM invoice WHERE is_draft='no' ORDER BY id DESC LIMIT 100`)
		if err == nil {
			defer rows.Close()
			var invoices []map[string]interface{}
			for rows.Next() {
				var iid, date sql.NullString
				var total float64
				_ = rows.Scan(&iid, &date, &total)
				invoices = append(invoices, map[string]interface{}{"id": iid.String, "date": date.String, "total": total})
			}
			data["Invoices"] = invoices
		}
	}

	// Load products
	rows, _ := s.dbFor(r).Query(`SELECT product_id, product_name, product_model, price FROM product_information WHERE status=1 ORDER BY product_name`)
	if rows != nil {
		defer rows.Close()
		var products []map[string]interface{}
		for rows.Next() {
			var pid, name, model, price sql.NullString
			_ = rows.Scan(&pid, &name, &model, &price)
			products = append(products, map[string]interface{}{"id": pid.String, "name": name.String, "model": model.String, "price": price.String})
		}
		data["Products"] = products
	}

	s.render(w, r, "return_form.html", data)
}

func (s *Server) returnDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.dbFor(r).Query(`
		SELECT r.return_id, r.product_id, IFNULL(p.product_name,'') AS product_name,
			r.invoice_id, r.purchase_id, r.date_purchase, r.date_return,
			r.byy_qty, r.ret_qty, r.product_rate, r.deduction, r.total_deduct,
			r.total_tax, r.total_ret_amount, r.net_total_amount, r.reason, r.usablity,
			IFNULL(c.customer_name,'') AS customer, IFNULL(s.supplier_name,'') AS supplier
		FROM product_return r
		LEFT JOIN product_information p ON p.product_id = r.product_id
		LEFT JOIN customer_information c ON c.customer_id = r.customer_id
		LEFT JOIN supplier_information s ON s.supplier_id = r.supplier_id
		WHERE r.return_id = ?`, id)
	if err != nil {
		http.Error(w, "Devolución no encontrada", 404)
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var rid, pid, invID, purID, datePur, dateRet, reason sql.NullString
		var byyQty, retQty, rate, deduct, totalDeduct, tax, totalRet, netAmount sql.NullFloat64
		var usablity int
		var customer, supplier sql.NullString
		_ = rows.Scan(&rid, &pid, &product_name, &invID, &purID, &datePur, &dateRet,
			&byyQty, &retQty, &rate, &deduct, &totalDeduct, &tax, &totalRet, &netAmount,
			&reason, &usablity, &customer, &supplier)
		items = append(items, map[string]interface{}{
			"return_id": rid.String, "product_id": pid.String, "product_name": product_name.String,
			"invoice_id": invID.String, "purchase_id": purID.String,
			"date_purchase": datePur.String, "date_return": dateRet.String,
			"byy_qty": byyQty.Float64, "ret_qty": retQty.Float64, "rate": rate.Float64,
			"deduction": deduct.Float64, "total_deduct": totalDeduct.Float64,
			"total_tax": tax.Float64, "total_ret_amount": totalRet.Float64,
			"net_total_amount": netAmount.Float64, "reason": reason.String,
			"usablity": usablity, "customer": customer.String, "supplier": supplier.String,
		})
	}

	s.render(w, r, "return_details.html", map[string]interface{}{
		"Title":    "Devolución #" + id,
		"Items":    items,
		"ReturnID": id,
	})
}

var product_name sql.NullString
