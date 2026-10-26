package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Customer module (replicates modules/customer) ----------

func (s *Server) registerCustomerRoutes() {
	s.mux.HandleFunc("GET /customer/list", s.customerList)
	s.mux.HandleFunc("/customer/list/data", s.customerListData)
	s.mux.HandleFunc("GET /customer/add", s.customerForm)
	s.mux.HandleFunc("GET /customer/edit/{id}", s.customerForm)
	s.mux.HandleFunc("POST /customer/save", s.customerSave)
	s.mux.HandleFunc("POST /customer/delete/{id}", s.customerDelete)
	s.mux.HandleFunc("GET /customer/credit", s.customerCreditList)
	s.mux.HandleFunc("GET /customer/paid", s.customerPaidList)
	s.mux.HandleFunc("GET /customer/ledger", s.customerLedgerList)
	s.mux.HandleFunc("GET /customer/ledger/{id}", s.customerLedger)
	s.mux.HandleFunc("GET /customer/ledger/{id}/data", s.customerLedgerData)
	s.mux.HandleFunc("POST /customer/previous-balance/{id}", s.customerPreviousBalance)
	s.mux.HandleFunc("GET /customer/search", s.customerSearch)
	s.mux.HandleFunc("POST /customer/quick", s.customerQuickCreate)
}

// customerRow is a list row with balance.
type customerRow struct {
	ID       int64
	Name     string
	Mobile   string
	Email    string
	Address  string
	City     string
	Country  string
	Balance  float64
	Status   int
	HeadCode int64
}

func (s *Server) customerList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "customer_list.html", map[string]interface{}{
		"Title": "Gestionar Clientes",
	})
}

func (s *Server) customerListData(w http.ResponseWriter, r *http.Request) {
	// DataTables server-side protocol (like getCustomerList)
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
		where = " WHERE a.customer_name LIKE ? OR a.customer_mobile LIKE ? OR a.customer_email LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM customer_information").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM customer_information a"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.customer_id, a.customer_name, IFNULL(a.customer_mobile,''), IFNULL(a.customer_email,''),
			IFNULL(a.customer_address,''), IFNULL(a.city,''), IFNULL(a.country,''),
			IFNULL(b.HeadCode,0),
			IFNULL((SELECT SUM(t.Debit)-SUM(t.Credit) FROM acc_transaction t WHERE t.COAID=b.HeadCode AND t.IsAppove=1),0) AS balance
		FROM customer_information a
		LEFT JOIN acc_coa b ON b.customer_id = a.customer_id
		`+where+`
		ORDER BY a.customer_id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var c customerRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Mobile, &c.Email, &c.Address, &c.City, &c.Country, &c.HeadCode, &c.Balance); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":      c.ID,
			"name":    c.Name,
			"mobile":  c.Mobile,
			"email":   c.Email,
			"address": c.Address,
			"city":    c.City,
			"balance": money(c.Balance),
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

func (s *Server) customerForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Añadir Cliente"}
	if id := r.PathValue("id"); id != "" {
		var c struct {
			ID, Name, Mobile, Email, Email2, Address, Address2, City, State, Zip, Country, Contact, Phone, Fax, RIF sql.NullString
		}
		err := s.dbFor(r).QueryRow(`SELECT customer_id, customer_name, customer_mobile, customer_email,
			email_address, customer_address, address2, city, state, zip, country, contact, phone, fax, IFNULL(rif,'')
			FROM customer_information WHERE customer_id = ?`, id).
			Scan(&c.ID, &c.Name, &c.Mobile, &c.Email, &c.Email2, &c.Address, &c.Address2, &c.City, &c.State, &c.Zip, &c.Country, &c.Contact, &c.Phone, &c.Fax, &c.RIF)
		if err != nil {
			http.Error(w, "Cliente no encontrado", 404)
			return
		}
		data["Title"] = "Editar Cliente"
		data["C"] = c
	}
	s.render(w, r, "customer_form.html", data)
}

func (s *Server) customerSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	name := strings.TrimSpace(r.FormValue("customer_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	// FISCAL: RIF o cédula, según el selector de la pantalla (la letra J/G
	// manda: jurídica siempre RIF completo).
	rif := NormalizeRIF(r.FormValue("rif"))
	if rif != "" {
		if ok, msg := ValidateDocumento(rif, r.FormValue("rif_tipo")); !ok {
			jsonError(w, msg, 400)
			return
		}
	}
	idStr := r.FormValue("customer_id")

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if idStr == "" {
		// CREATE (replicates Customer_model->create)
		res, err := tx.Exec(`
			INSERT INTO customer_information
				(customer_name, customer_mobile, customer_email, email_address, contact, phone, fax,
				 city, state, zip, country, customer_address, address2, rif, status, create_by)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?)`,
			name, r.FormValue("customer_mobile"), r.FormValue("customer_email"), r.FormValue("email_address"),
			r.FormValue("contact"), r.FormValue("phone"), r.FormValue("fax"),
			r.FormValue("city"), r.FormValue("state"), r.FormValue("zip"), r.FormValue("country"),
			r.FormValue("customer_address"), r.FormValue("address2"), rif, createBy)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		customerID, _ := res.LastInsertId()

		// HeadCode: MAX(HeadCode)+1 under 113100% (Customer Receivable), start 1131000001
		headcode, err := nextHeadCodeTx(tx, "113100", "1131000001")
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		headName := fmt.Sprintf("%d-%s", customerID, name)
		if _, err := tx.Exec(`
			INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, HeadLevel, IsActive, IsTransaction, IsGL,
				HeadType, IsBudget, IsDepreciation, DepreciationRate, customer_id, CreateBy, CreateDate, UpdateBy, UpdateDate, isSubType, subType)
			VALUES (?,?,?,?,1,1,0,'A',0,0,0,?,?,?,?,?,0,1)`,
			headcode, headName, "Customer Receivable", 4, customerID, createBy, now(), createBy, now()); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if _, err := tx.Exec(`
			INSERT INTO acc_subcode (subTypeId, name, referenceNo, status, created_by, created_date, updated_by)
			VALUES (3, ?, ?, 1, ?, ?, ?)`, name, customerID, createBy, today(), createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		_, err = tx.Exec(`
			UPDATE customer_information SET customer_name=?, customer_mobile=?, customer_email=?, email_address=?,
				contact=?, phone=?, fax=?, city=?, state=?, zip=?, country=?, customer_address=?, address2=?, rif=?
			WHERE customer_id=?`,
			name, r.FormValue("customer_mobile"), r.FormValue("customer_email"), r.FormValue("email_address"),
			r.FormValue("contact"), r.FormValue("phone"), r.FormValue("fax"),
			r.FormValue("city"), r.FormValue("state"), r.FormValue("zip"), r.FormValue("country"),
			r.FormValue("customer_address"), r.FormValue("address2"), rif, idStr)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "Guardado correctamente",
		"redirect": saveRedirect(r, "/customer/list"),
	})
}

// customerDelete replicates Customer_model->delete: removes subcode + COA + customer.
func (s *Server) customerDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.Exec(`DELETE FROM acc_subcode WHERE referenceNo = ? AND subTypeId = 3`, id)
	_, _ = tx.Exec(`DELETE FROM acc_coa WHERE customer_id = ?`, id)
	res, err := tx.Exec(`DELETE FROM customer_information WHERE customer_id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Cliente no encontrado", 404)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/customer/list")})
}

func (s *Server) customerCreditList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "customer_credit.html", map[string]interface{}{
		"Title": "Clientes con Crédito",
		"Mode":  "credit",
	})
}

func (s *Server) customerPaidList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "customer_credit.html", map[string]interface{}{
		"Title": "Clientes Pagados",
		"Mode":  "paid",
	})
}

func (s *Server) customerLedgerList(w http.ResponseWriter, r *http.Request) {
	type cust struct {
		ID   int64
		Name string
	}
	list := []cust{}
	rows, err := s.dbFor(r).Query(`SELECT customer_id, customer_name FROM customer_information ORDER BY customer_name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c cust
			_ = rows.Scan(&c.ID, &c.Name)
			list = append(list, c)
		}
	}
	s.render(w, r, "ledger_picker.html", map[string]interface{}{
		"Title":     "Libro Mayor de Clientes",
		"Kind":      "customer",
		"Customers": list,
	})
}

// customerLedger shows the ledger page of one customer.
func (s *Server) customerLedger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name, mobile, address sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT customer_name, customer_mobile, customer_address FROM customer_information WHERE customer_id=?`, id).
		Scan(&name, &mobile, &address)
	if err != nil {
		http.Error(w, "Cliente no encontrado", 404)
		return
	}
	s.render(w, r, "customer_ledger.html", map[string]interface{}{
		"Title":   "Libro Mayor: " + name.String,
		"CID":     id,
		"CName":   name.String,
		"CMobile": mobile.String,
		"CAddr":   address.String,
	})
}

// customerLedgerData returns ledger rows (AJAX, filterable by date).
func (s *Server) customerLedgerData(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	from := r.FormValue("from")
	to := r.FormValue("to")

	var headcode int64
	err := s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE customer_id = ? LIMIT 1`, id).Scan(&headcode)
	if err != nil {
		jsonError(w, "El cliente no tiene cuenta contable", 400)
		return
	}

	q := `SELECT VDate, VNo, Vtype, Narration, Debit, Credit,
		IFNULL((SELECT SUM(x.Debit)-SUM(x.Credit) FROM acc_transaction x
			WHERE x.COAID = t.COAID AND x.IsAppove=1 AND (x.VDate < t.VDate OR (x.VDate = t.VDate AND x.ID <= t.ID))),0) AS balance
		FROM acc_transaction t
		WHERE t.COAID = ? AND t.IsAppove = 1`
	args := []interface{}{headcode}
	if from != "" {
		q += " AND t.VDate >= ?"
		args = append(args, from)
	}
	if to != "" {
		q += " AND t.VDate <= ?"
		args = append(args, to)
	}
	q += " ORDER BY t.VDate ASC, t.ID ASC"

	rows, err := s.dbFor(r).Query(q, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type lrow struct {
		Date      string  `json:"date"`
		VNo       string  `json:"vno"`
		VType     string  `json:"vtype"`
		Narration string  `json:"narration"`
		Debit     float64 `json:"debit"`
		Credit    float64 `json:"credit"`
		Balance   float64 `json:"balance"`
	}
	list := []lrow{}
	var totalD, totalC float64
	for rows.Next() {
		var d sql.NullString
		var vno, vtype, narr sql.NullString
		var dr, cr, bal float64
		if err := rows.Scan(&d, &vno, &vtype, &narr, &dr, &cr, &bal); err != nil {
			continue
		}
		list = append(list, lrow{d.String, vno.String, vtype.String, narr.String, dr, cr, bal})
		totalD += dr
		totalC += cr
	}
	writeJSON(w, map[string]interface{}{
		"data":         list,
		"total_debit":  money(totalD),
		"total_credit": money(totalC),
		"balance":      money(totalD - totalC),
	})
}

// customerPreviousBalance replicates previous_balance_add: opening debit to
// customer account, credit to Inventory (1141).
func (s *Server) customerPreviousBalance(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	balance := parseMoney(r.FormValue("balance"))
	if balance == 0 {
		jsonError(w, "Monto inválido", 400)
		return
	}
	customerID, _ := strconv.ParseInt(idStr, 10, 64)
	sess := s.sessionFromRequest(r)

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	var headcode int64
	err = tx.QueryRow(`SELECT customer_name FROM customer_information WHERE customer_id=?`, customerID).Scan(&name)
	if err != nil {
		jsonError(w, "Cliente no encontrado", 404)
		return
	}
	err = tx.QueryRow(`SELECT HeadCode FROM acc_coa WHERE customer_id=? LIMIT 1`, customerID).Scan(&headcode)
	if err != nil {
		jsonError(w, "El cliente no tiene cuenta contable", 400)
		return
	}

	tranID := generator(10)
	// debit customer (vid/fyear/RevCodde are NOT NULL without defaults in the
	// live schema; is_opening marks this as an opening entry)
	if _, err := tx.Exec(`INSERT INTO acc_transaction (VNo, Vtype, VDate, COAID, Narration, Debit, Credit, IsPosted, is_opening, CreateBy, CreateDate, IsAppove, vid, fyear, RevCodde)
		VALUES (?,?,?,?,?, ?,0,1,1,?,?,1,0,0,0)`,
		tranID, "PR Balance", today(), headcode, "Customer debit For "+name, balance, sessUserID(sess), now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// credit inventory 1141
	if _, err := tx.Exec(`INSERT INTO acc_transaction (VNo, Vtype, VDate, COAID, Narration, Debit, Credit, IsPosted, is_opening, CreateBy, CreateDate, IsAppove, vid, fyear, RevCodde)
		VALUES (?,?,?,?,?,?,?,1,1,?,?,1,0,0,0)`,
		tranID, "PR Balance", today(), "1141", "Inventory credit For Old sale For "+name, 0, balance, sessUserID(sess), now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Balance anterior aplicado", "redirect": saveRedirect(r, "/customer/list")})
}

// customerSearch: autocomplete by name/mobile (used by invoice forms).
func (s *Server) customerSearch(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.FormValue("term"))
	if term == "" {
		writeJSON(w, []interface{}{})
		return
	}
	rows, err := s.dbFor(r).Query(`SELECT customer_id, customer_name, IFNULL(customer_mobile,''), IFNULL(customer_address,''), IFNULL(rif,'')
		FROM customer_information
		WHERE customer_name LIKE ? OR customer_mobile LIKE ? OR rif LIKE ? LIMIT 20`, "%"+term+"%", "%"+term+"%", "%"+term+"%")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, mobile, addr, rif sql.NullString
		_ = rows.Scan(&id, &name, &mobile, &addr, &rif)
		out = append(out, map[string]interface{}{
			"id": id, "label": name.String, "value": name.String,
			"mobile": mobile.String, "address": addr.String, "rif": rif.String,
		})
	}
	writeJSON(w, out)
}

// customerQuickCreate: instant customer creation from POS modal.
func (s *Server) customerQuickCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("customer_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	// FISCAL: RIF opcional en cliente contado; si viene, se valida según el
	// selector (RIF completo o cédula suelta).
	rif := NormalizeRIF(r.FormValue("customer_rif"))
	if rif != "" {
		if ok, msg := ValidateDocumento(rif, r.FormValue("rif_tipo")); !ok {
			jsonError(w, msg, 400)
			return
		}
	}
	sess := s.sessionFromRequest(r)
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO customer_information (customer_name, customer_mobile, customer_address, address2, rif, status, create_by)
	VALUES (?,?,?,?,?,1,?)`, name, r.FormValue("customer_mobile"), r.FormValue("customer_address"), r.FormValue("customer_address"), rif, sessUserID(sess))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	customerID, _ := res.LastInsertId()
	headcode, err := nextHeadCodeTx(tx, "113100", "1131000001")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	_, _ = tx.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, HeadLevel, IsActive, IsTransaction, IsGL,
		HeadType, IsBudget, IsDepreciation, DepreciationRate, customer_id, CreateBy, CreateDate, isSubType, subType)
		VALUES (?,?,?,?,1,1,0,'A',0,0,0,?,?,?,0,1)`,
		headcode, fmt.Sprintf("%d-%s", customerID, name), "Customer Receivable", 4, customerID, sessUserID(sess), now())
	_, _ = tx.Exec(`INSERT INTO acc_subcode (subTypeId, name, referenceNo, status, created_by, created_date)
		VALUES (3, ?, ?, 1, ?, ?)`, name, customerID, sessUserID(sess), today())

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "customer_id": customerID, "name": name, "rif": rif})
}

// customerSearch ya existe; asegurar que devuelve RIF para POS/lookup

// ---------- shared helpers ----------

// sessUserID returns the session user id or "1".
func sessUserID(sess *SessionData) string {
	if sess == nil || sess.UserID == "" {
		return "1"
	}
	return sess.UserID
}

// nextHeadCodeTx replicates headcode(): MAX(HeadCode)+1 for a prefix, or the
// default seed when empty.
func nextHeadCodeTx(tx *sql.Tx, prefix, seed string) (int64, error) {
	var max sql.NullInt64
	err := tx.QueryRow(`SELECT MAX(HeadCode) FROM acc_coa WHERE HeadLevel=4 AND HeadCode LIKE ?`, prefix+"%").Scan(&max)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if !max.Valid || max.Int64 == 0 {
		var seedI int64
		fmt.Sscanf(seed, "%d", &seedI)
		return seedI, nil
	}
	return max.Int64 + 1, nil
}
