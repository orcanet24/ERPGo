package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Service module (replicates modules/service) ----------

func (s *Server) registerServiceRoutes() {
	s.mux.HandleFunc("GET /service/list", s.serviceList)
	s.mux.HandleFunc("GET /service/list/data", s.serviceListData)
	s.mux.HandleFunc("GET /service/add", s.serviceForm)
	s.mux.HandleFunc("GET /service/edit/{id}", s.serviceForm)
	s.mux.HandleFunc("POST /service/save", s.serviceSave)
	s.mux.HandleFunc("POST /service/delete/{id}", s.serviceDelete)
	s.mux.HandleFunc("GET /service/invoice", s.serviceInvoiceList)
	s.mux.HandleFunc("GET /service/invoice/data", s.serviceInvoiceListData)
	s.mux.HandleFunc("GET /service/invoice/new", s.serviceInvoiceForm)
	s.mux.HandleFunc("POST /service/invoice/save", s.serviceInvoiceSave)
}

func (s *Server) serviceList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "service_list.html", map[string]interface{}{"Title": "Gestionar Servicios"})
}

func (s *Server) serviceListData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE service_name LIKE ?"
		args = append(args, "%"+search+"%")
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_service").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_service"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`SELECT service_id, service_name, description, charge, service_vat, is_fixed, is_dynamic FROM product_service`+where+` ORDER BY service_id DESC LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, desc, charge, vat sql.NullString
		var isFixed, isDynamic int
		_ = rows.Scan(&id, &name, &desc, &charge, &vat, &isFixed, &isDynamic)
		list = append(list, map[string]interface{}{
			"id": id, "name": name.String, "description": desc.String,
			"charge": charge.String, "vat": vat.String,
			"is_fixed": isFixed, "is_dynamic": isDynamic,
		})
	}
	writeJSON(w, map[string]interface{}{"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list})
}

func (s *Server) serviceForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Añadir Servicio"}
	if id := r.PathValue("id"); id != "" {
		var name, desc, charge, vat sql.NullString
		var isFixed, isDynamic int
		err := s.dbFor(r).QueryRow(`SELECT service_name, description, charge, service_vat, is_fixed, is_dynamic FROM product_service WHERE service_id = ?`, id).
			Scan(&name, &desc, &charge, &vat, &isFixed, &isDynamic)
		if err != nil {
			http.Error(w, "Servicio no encontrado", 404)
			return
		}
		data["Title"] = "Editar Servicio"
		data["S"] = map[string]interface{}{
			"id": id, "name": name.String, "description": desc.String,
			"charge": charge.String, "vat": vat.String,
			"is_fixed": isFixed, "is_dynamic": isDynamic,
		}
	}
	s.render(w, r, "service_form.html", data)
}

func (s *Server) serviceSave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("service_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	id := r.FormValue("service_id")
	if id == "" {
		_, err := s.dbFor(r).Exec(`INSERT INTO product_service (service_name, description, charge, service_vat, is_fixed, is_dynamic) VALUES (?,?,?,?,?,?)`,
			name, r.FormValue("description"), parseMoney(r.FormValue("charge")),
			parseMoney(r.FormValue("service_vat")), boolInt(r.FormValue("is_fixed") == "1"), boolInt(r.FormValue("is_dynamic") == "1"))
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		_, err := s.dbFor(r).Exec(`UPDATE product_service SET service_name=?, description=?, charge=?, service_vat=?, is_fixed=?, is_dynamic=? WHERE service_id=?`,
			name, r.FormValue("description"), parseMoney(r.FormValue("charge")),
			parseMoney(r.FormValue("service_vat")), boolInt(r.FormValue("is_fixed") == "1"), boolInt(r.FormValue("is_dynamic") == "1"), id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Servicio guardado", "redirect": saveRedirect(r, "/service/list")})
}

func (s *Server) serviceDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := s.dbFor(r).Exec(`DELETE FROM product_service WHERE service_id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Servicio no encontrado", 404)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/service/list")})
}

// ---------- Service Invoice ----------

func (s *Server) serviceInvoiceList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "service_invoice_list.html", map[string]interface{}{"Title": "Facturas de Servicio"})
}

func (s *Server) serviceInvoiceListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT voucher_no, date, IFNULL(c.customer_name,'') AS customer, total_amount, paid_amount, due_amount, is_credit FROM service_invoice s JOIN customer_information c ON c.customer_id=s.customer_id ORDER BY s.id DESC LIMIT 100`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var vno, date, customer sql.NullString
		var total, paid, due float64
		var isCredit int
		_ = rows.Scan(&vno, &date, &customer, &total, &paid, &due, &isCredit)
		list = append(list, map[string]interface{}{
			"voucher": vno.String, "date": date.String, "customer": customer.String,
			"total": total, "paid": paid, "due": due, "is_credit": isCredit,
		})
	}
	writeJSON(w, list)
}

func (s *Server) serviceInvoiceForm(w http.ResponseWriter, r *http.Request) {
	// customers dropdown
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
	// services dropdown
	services := []map[string]interface{}{}
	rows, _ = s.dbFor(r).Query(`SELECT service_id, service_name, charge FROM product_service ORDER BY service_name`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var name, charge string
			_ = rows.Scan(&id, &name, &charge)
			services = append(services, map[string]interface{}{"id": id, "name": name, "charge": charge})
		}
		rows.Close()
	}
	s.render(w, r, "service_invoice_form.html", map[string]interface{}{
		"Title":     "Nueva Factura de Servicio",
		"Customers": customers,
		"Services":  services,
	})
}

func (s *Server) serviceInvoiceSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	customerID, _ := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	if customerID == 0 {
		jsonError(w, "Cliente inválido", 400)
		return
	}

	serviceIDs := r.Form["service_id"]
	qty := r.Form["qty"]
	charges := r.Form["charge"]

	if len(serviceIDs) == 0 {
		jsonError(w, "Agregue al menos un servicio", 400)
		return
	}

	grandTotal := parseMoney(r.FormValue("total_amount"))
	totalDiscount := parseMoney(r.FormValue("total_discount"))
	invoiceDiscount := parseMoney(r.FormValue("invoice_discount"))
	totalVat := parseMoney(r.FormValue("total_vat_amnt"))
	totalTax := parseMoney(r.FormValue("total_tax"))
	paidAmount := parseMoney(r.FormValue("paid_amount"))
	dueAmount := grandTotal - paidAmount

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// next voucher_no
	var nextVno string
	var vno sql.NullString
	_ = tx.QueryRow(`SELECT voucher_no FROM service_invoice ORDER BY id DESC LIMIT 1`).Scan(&vno)
	n := 0
	if vno.Valid && vno.String != "" {
		fmt.Sscanf(vno.String, "SINV-%d", &n)
	}
	nextVno = fmt.Sprintf("SINV-%d", n+1)

	isCredit := 1
	if paidAmount >= grandTotal {
		isCredit = 0
	}

	date := orDefault(r.FormValue("date"), today())

	res, err := tx.Exec(`INSERT INTO service_invoice (voucher_no, date, customer_id, total_amount, total_discount, invoice_discount, total_vat_amnt, total_tax, paid_amount, due_amount, shipping_cost, previous, details, sales_by, is_fixed, is_dynamic, is_credit)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,0,?)`,
		nextVno, date, customerID, grandTotal, totalDiscount, invoiceDiscount, totalVat, totalTax,
		paidAmount, dueAmount, parseMoney(r.FormValue("shipping_cost")),
		parseMoney(r.FormValue("previous")), r.FormValue("details"), createBy, isCredit)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	invInsertID, _ := res.LastInsertId()

	// details
	for i, sid := range serviceIDs {
		q := 1.0
		c := 0.0
		if i < len(qty) {
			q = parseMoney(qty[i])
		}
		if i < len(charges) {
			c = parseMoney(charges[i])
		}
		_, _ = tx.Exec(`INSERT INTO service_invoice_details (service_id, service_inv_id, qty, charge, discount, discount_amount, vat, vat_amnt, total)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			sid, fmt.Sprintf("%d", invInsertID), q, c, 0, 0, 0, 0, c*q)
	}

	// accounting vouchers (service sale)
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// credit voucher: debit customer, credit service account
	sub := s.subcodeID(r, customerID, 3)
	vnoCv := nextVoucherNoTx(tx, "CV")
	_ = s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vnoCv, Vtype: "CV", ReferenceNo: nextVno,
		COAID: pre.CustomerCode, RevCodde: pre.ServiceCode,
		Narration: "Service Sales Voucher", Comment: "Service sales for customer",
		Debit: grandTotal, SubType: 3, SubCode: sub,
	}, createBy)

	// payment
	if paidAmount > 0 {
		vnoJv := nextVoucherNoTx(tx, "JV")
		_ = s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vnoJv, Vtype: "JV", ReferenceNo: nextVno,
			COAID: pre.CashCode, RevCodde: pre.CustomerCode,
			Narration: "Service payment received", Comment: "Payment for service invoice",
			Debit: paidAmount,
		}, createBy)
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// autoapprove
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		vouchers, _ := s.dbFor(r).Query(`SELECT VNo FROM acc_vaucher WHERE referenceNo = ? AND status = 0`, nextVno)
		var vnos []string
		if vouchers != nil {
			for vouchers.Next() {
				var v string
				_ = vouchers.Scan(&v)
				vnos = append(vnos, v)
			}
			vouchers.Close()
		}
		for _, v := range vnos {
			_ = s.approveVoucher(r, v, createBy)
		}
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Factura de servicio registrada", "voucher": nextVno})
}

// res is used inside serviceInvoiceSave; declared to avoid compilation error
var res sql.Result
