package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Quotation module (replicates quotation) ----------

func (s *Server) registerQuotationRoutes() {
	s.mux.HandleFunc("GET /quotation/list", s.quotationList)
	s.mux.HandleFunc("GET /quotation/list/data", s.quotationListData)
	s.mux.HandleFunc("GET /quotation/new", s.quotationForm)
	s.mux.HandleFunc("GET /quotation/edit/{id}", s.quotationForm)
	s.mux.HandleFunc("POST /quotation/save", s.quotationSave)
	s.mux.HandleFunc("POST /quotation/delete/{id}", s.quotationDelete)
	s.mux.HandleFunc("GET /quotation/details/{id}", s.quotationDetails)
	s.mux.HandleFunc("GET /quotation/print/{id}", s.quotationPrint)
}

func (s *Server) quotationList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "quotation_list.html", map[string]interface{}{"Title": "Cotizaciones"})
}

func (s *Server) quotationListData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE q.quot_no LIKE ? OR c.customer_name LIKE ?"
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM quotation").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM quotation q LEFT JOIN customer_information c ON c.customer_id = q.customer_id"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT q.quotation_id, q.quot_no, q.customer_id, IFNULL(c.customer_name,'') AS customer,
			q.quotdate, q.expire_date, q.item_total_amount, q.item_total_dicount,
			q.item_total_vat, q.status, q.cust_show
		FROM quotation q
		LEFT JOIN customer_information c ON c.customer_id = q.customer_id
		`+where+`
		ORDER BY q.id DESC LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var qid, qno, custID, custName, quotdate, expire sql.NullString
		var amount, discount, vat float64
		var status, show int
		_ = rows.Scan(&qid, &qno, &custID, &custName, &quotdate, &expire, &amount, &discount, &vat, &status, &show)
		list = append(list, map[string]interface{}{
			"id": qid.String, "quot_no": qno.String, "customer": custName.String,
			"date": quotdate.String, "expire": expire.String,
			"amount": amount, "discount": discount, "vat": vat,
			"status": status, "show": show,
		})
	}
	writeJSON(w, map[string]interface{}{"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list})
}

func (s *Server) quotationForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Nueva Cotización"}

	// Customers
	customers := []map[string]interface{}{}
	rows, _ := s.dbFor(r).Query(`SELECT customer_id, customer_name FROM customer_information WHERE status=1 ORDER BY customer_name`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var name string
			_ = rows.Scan(&id, &name)
			customers = append(customers, map[string]interface{}{"id": id, "name": name})
		}
		rows.Close()
	}
	data["Customers"] = customers

	// Products
	products := []map[string]interface{}{}
	rows, _ = s.dbFor(r).Query(`SELECT product_id, product_name, product_model, price, unit FROM product_information WHERE status=1 ORDER BY product_name`)
	if rows != nil {
		for rows.Next() {
			var pid, name, model, price, unit sql.NullString
			_ = rows.Scan(&pid, &name, &model, &price, &unit)
			products = append(products, map[string]interface{}{"id": pid.String, "name": name.String, "model": model.String, "price": price.String, "unit": unit.String})
		}
		rows.Close()
	}
	data["Products"] = products

	// Services
	services := []map[string]interface{}{}
	rows, _ = s.dbFor(r).Query(`SELECT service_id, service_name, charge FROM product_service ORDER BY service_name`)
	if rows != nil {
		for rows.Next() {
			var sid int64
			var name, charge string
			_ = rows.Scan(&sid, &name, &charge)
			services = append(services, map[string]interface{}{"id": sid, "name": name, "charge": charge})
		}
		rows.Close()
	}
	data["Services"] = services

	if id := r.PathValue("id"); id != "" {
		// Load existing quotation
		var qid, qno, desc, custID, quotdate, expire sql.NullString
		var itemAmt, itemDisc, itemVat, itemTax, servAmt, servDisc, servVat, servTax, quotDiscItem, quotDiscServ float64
		var isFixed, isDynamic, status, show int
		err := s.dbFor(r).QueryRow(`SELECT quotation_id, quot_no, quot_description, customer_id, quotdate, expire_date,
			item_total_amount, item_total_dicount, item_total_vat, item_total_tax,
			service_total_amount, service_total_discount, service_total_vat, service_total_tax,
			quot_dis_item, quot_dis_service, is_fixed, is_dynamic, status, cust_show
			FROM quotation WHERE quotation_id = ?`, id).
			Scan(&qid, &qno, &desc, &custID, &quotdate, &expire,
				&itemAmt, &itemDisc, &itemVat, &itemTax, &servAmt, &servDisc, &servVat, &servTax,
				&quotDiscItem, &quotDiscServ, &isFixed, &isDynamic, &status, &show)
		if err != nil {
			http.Error(w, "Cotización no encontrada", 404)
			return
		}
		data["Title"] = "Editar Cotización #" + id
		data["Q"] = map[string]interface{}{
			"id": qid.String, "quot_no": qno.String, "description": desc.String,
			"customer_id": custID.String, "date": quotdate.String, "expire": expire.String,
			"item_total": itemAmt, "item_discount": itemDisc, "item_vat": itemVat, "item_tax": itemTax,
			"service_total": servAmt, "service_discount": servDisc, "service_vat": servVat, "service_tax": servTax,
			"status": status, "is_fixed": isFixed, "is_dynamic": isDynamic,
		}

		// Products used
		var pused []map[string]interface{}
		rows, _ := s.dbFor(r).Query(`
			SELECT IFNULL(p.product_name,'') AS product_name, IFNULL(p.product_model,'') AS model,
				qu.used_qty, qu.rate, qu.total_price, qu.discount, qu.vat_amnt, qu.tax
			FROM quot_products_used qu
			LEFT JOIN product_information p ON p.product_id = qu.product_id
			WHERE qu.quot_id = ?`, id)
		if rows != nil {
			for rows.Next() {
				var pname, model sql.NullString
				var qty, rate, total, disc, vat, tax float64
				_ = rows.Scan(&pname, &model, &qty, &rate, &total, &disc, &vat, &tax)
				pused = append(pused, map[string]interface{}{
					"product": pname.String, "model": model.String,
					"qty": qty, "rate": rate, "total": total, "discount": disc, "vat": vat, "tax": tax,
				})
			}
			rows.Close()
		}
		data["ProductsUsed"] = pused
	}

	s.render(w, r, "quotation_form.html", data)
}

func (s *Server) quotationSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	quotationID := r.FormValue("quotation_id")
	customerID := r.FormValue("customer_id")
	quotdate := orDefault(r.FormValue("quotdate"), today())
	expire := r.FormValue("expire_date")
	desc := r.FormValue("quot_description")

	productIDs := r.Form["product_id"]
	qty := r.Form["qty"]
	rates := r.Form["rate"]
	discounts := r.Form["discount"]

	serviceIDs := r.Form["service_id"]
	sQty := r.Form["service_qty"]
	sCharges := r.Form["service_charge"]

	isFixed := boolInt(r.FormValue("is_fixed") == "1")
	isDynamic := boolInt(r.FormValue("is_dynamic") == "1")
	status := 1 // active

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var itemTotal, itemDisc, itemVat, itemTax float64
	for i, pid := range productIDs {
		q := 1.0
		rate := 0.0
		disc := 0.0
		if i < len(qty) {
			q = parseMoney(qty[i])
		}
		if i < len(rates) {
			rate = parseMoney(rates[i])
		}
		if i < len(discounts) {
			disc = parseMoney(discounts[i])
		}
		total := q * rate
		vat := total * 0.16
		itemTotal += total
		itemDisc += disc
		itemVat += vat
		itemTax += 0

		if pid != "" && q > 0 {
			_, _ = tx.Exec(`INSERT INTO quot_products_used (quot_id, product_id, description, used_qty, rate, total_price, discount, vat_amnt, tax) VALUES (?,?,?,?,?,?,?,?,?)`,
				quotationID, pid, "", q, rate, total, disc, vat, 0)
		}
	}

	var servTotal, servDisc, servVat, servTax float64
	for i, sid := range serviceIDs {
		q := 1.0
		charge := 0.0
		if i < len(sQty) {
			q = parseMoney(sQty[i])
		}
		if i < len(sCharges) {
			charge = parseMoney(sCharges[i])
		}
		total := q * charge
		vat := total * 0.16
		servTotal += total
		servVat += vat

		if sid != "" && q > 0 {
			_, _ = tx.Exec(`INSERT INTO quotation_service_used (quot_id, service_id, qty, charge, vat_amnt, total) VALUES (?,?,?,?,?,?)`,
				quotationID, sid, q, charge, vat, total)
		}
	}

	quotNo := r.FormValue("quot_no")
	if quotNo == "" {
		var maxQuot sql.NullString
		_ = tx.QueryRow(`SELECT quot_no FROM quotation ORDER BY id DESC LIMIT 1`).Scan(&maxQuot)
		n := 0
		if maxQuot.Valid && maxQuot.String != "" {
			fmt.Sscanf(maxQuot.String, "QUOT-%d", &n)
		}
		quotNo = fmt.Sprintf("QUOT-%d", n+1)
	}

	if quotationID == "" {
		// Generate quotation_id
		quotationID = generator(12)
		_, err = tx.Exec(`INSERT INTO quotation (quotation_id, quot_no, quot_description, customer_id, quotdate, expire_date,
			item_total_amount, item_total_dicount, item_total_vat, item_total_tax,
			service_total_amount, service_total_discount, service_total_vat, service_total_tax,
			quot_dis_item, quot_dis_service, is_fixed, is_dynamic, create_by, create_date, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			quotationID, quotNo, desc, customerID, quotdate, expire,
			itemTotal, itemDisc, itemVat, itemTax, servTotal, servDisc, servVat, servTax,
			0, 0, isFixed, isDynamic, createBy, today(), status)
	} else {
		_, err = tx.Exec(`UPDATE quotation SET quot_description=?, customer_id=?, quotdate=?, expire_date=?,
			item_total_amount=?, item_total_dicount=?, item_total_vat=?, item_total_tax=?,
			service_total_amount=?, service_total_discount=?, service_total_vat=?, service_total_tax=?,
			is_fixed=?, is_dynamic=?, update_by=?, update_date=?
			WHERE quotation_id=?`,
			desc, customerID, quotdate, expire,
			itemTotal, itemDisc, itemVat, itemTax, servTotal, servDisc, servVat, servTax,
			isFixed, isDynamic, createBy, today(), quotationID)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// redirect es lo que permite al navegador honrar «Guardar y Salir» sin caer
	// en una pantalla intermedia: saveRedirect devuelve la URL de listado y,
	// cuando la acción es «Guardar» a secas, la página de origen —que es donde
	// se queda el formulario. (Antes esta respuesta no llevaba destino y el
	// formulario, que se enviaba solo, dejaba el JSON en pantalla.)
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "Cotización guardada",
		"id":       quotationID,
		"redirect": saveRedirect(r, "/quotation/list"),
	})
}

func (s *Server) quotationDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := s.dbFor(r).Exec(`DELETE FROM quotation WHERE quotation_id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Cotización no encontrada", 404)
		return
	}
	_, _ = s.dbFor(r).Exec(`DELETE FROM quot_products_used WHERE quot_id = ?`, id)
	_, _ = s.dbFor(r).Exec(`DELETE FROM quotation_service_used WHERE quot_id = ?`, id)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminada", "redirect": saveRedirect(r, "/quotation/list")})
}

func (s *Server) quotationDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var qno, desc, custName, date, expire sql.NullString
	var itemAmt, itemDisc, itemVat, itemTax, servAmt, servDisc, servVat, servTax float64
	var status, show int
	err := s.dbFor(r).QueryRow(`
		SELECT q.quot_no, q.quot_description, IFNULL(c.customer_name,''),
			q.quotdate, q.expire_date,
			q.item_total_amount, q.item_total_dicount, q.item_total_vat, q.item_total_tax,
			q.service_total_amount, q.service_total_discount, q.service_total_vat, q.service_total_tax,
			q.status, q.cust_show
		FROM quotation q LEFT JOIN customer_information c ON c.customer_id = q.customer_id
		WHERE q.quotation_id = ?`, id).
		Scan(&qno, &desc, &custName, &date, &expire,
			&itemAmt, &itemDisc, &itemVat, &itemTax, &servAmt, &servDisc, &servVat, &servTax,
			&status, &show)
	if err != nil {
		http.Error(w, "Cotización no encontrada", 404)
		return
	}

	// Products used
	pused := []map[string]interface{}{}
	rows, _ := s.dbFor(r).Query(`SELECT IFNULL(p.product_name,'') AS name, IFNULL(p.product_model,'') AS model, qu.used_qty, qu.rate, qu.total_price, qu.discount, qu.vat_amnt, qu.tax FROM quot_products_used qu LEFT JOIN product_information p ON p.product_id = qu.product_id WHERE qu.quot_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var name, model sql.NullString
			var qty, rate, total, disc, vat, tax float64
			_ = rows.Scan(&name, &model, &qty, &rate, &total, &disc, &vat, &tax)
			pused = append(pused, map[string]interface{}{
				"product": name.String, "model": model.String,
				"qty": qty, "rate": rate, "total": total, "discount": disc, "vat": vat, "tax": tax,
			})
		}
		rows.Close()
	}

	// Services used
	sused := []map[string]interface{}{}
	rows, _ = s.dbFor(r).Query(`SELECT IFNULL(ps.service_name,'') AS name, qs.qty, qs.charge, qs.vat_amnt, qs.total FROM quotation_service_used qs LEFT JOIN product_service ps ON ps.service_id = qs.service_id WHERE qs.quot_id = ?`, id)
	if rows != nil {
		for rows.Next() {
			var name sql.NullString
			var qty, charge, vat, total float64
			_ = rows.Scan(&name, &qty, &charge, &vat, &total)
			sused = append(sused, map[string]interface{}{
				"service": name.String, "qty": qty, "charge": charge, "vat": vat, "total": total,
			})
		}
		rows.Close()
	}

	st := s.settingsFor(r)
	s.render(w, r, "quotation_details.html", map[string]interface{}{
		"Title":       "Cotización #" + id,
		"QuotationID": id, "QNo": qno.String, "Description": desc.String,
		"Customer": custName.String, "Date": date.String, "Expire": expire.String,
		"ItemTotal": itemAmt, "ItemDiscount": itemDisc, "ItemVat": itemVat, "ItemTax": itemTax,
		"ServTotal": servAmt, "ServDiscount": servDisc, "ServVat": servVat, "ServTax": servTax,
		"Status": status, "Show": show,
		"ProductsUsed": pused, "ServicesUsed": sused,
		"Currency": st.Web.Currency,
	})
}

func (s *Server) quotationPrint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Reuse quotation_details data
	var qno, desc, custName, date, expire sql.NullString
	var itemAmt, itemDisc, itemVat, itemTax, servAmt, servDisc, servVat, servTax float64
	err := s.dbFor(r).QueryRow(`
		SELECT q.quot_no, q.quot_description, IFNULL(c.customer_name,''),
			q.quotdate, q.expire_date,
			q.item_total_amount, q.item_total_dicount, q.item_total_vat, q.item_total_tax,
			q.service_total_amount, q.service_total_discount, q.service_total_vat, q.service_total_tax
		FROM quotation q LEFT JOIN customer_information c ON c.customer_id = q.customer_id
		WHERE q.quotation_id = ?`, id).
		Scan(&qno, &desc, &custName, &date, &expire,
			&itemAmt, &itemDisc, &itemVat, &itemTax, &servAmt, &servDisc, &servVat, &servTax)
	if err != nil {
		http.Error(w, "Cotización no encontrada", 404)
		return
	}

	s.renderStandalone(r, w, "quotation_print.html", map[string]interface{}{
		"Title": "Cotización #" + id,
		"QNo":   qno.String, "Description": desc.String,
		"Customer": custName.String, "Date": date.String, "Expire": expire.String,
		"ItemTotal": itemAmt, "ItemDiscount": itemDisc, "ItemVat": itemVat, "ItemTax": itemTax,
		"ServTotal": servAmt, "ServDiscount": servDisc, "ServVat": servVat, "ServTax": servTax,
	})
}
