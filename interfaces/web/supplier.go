package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Supplier module (replicates modules/supplier) ----------

func (s *Server) registerSupplierRoutes() {
	s.mux.HandleFunc("GET /supplier/list", s.supplierList)
	s.mux.HandleFunc("/supplier/list/data", s.supplierListData)
	s.mux.HandleFunc("GET /supplier/add", s.supplierForm)
	s.mux.HandleFunc("GET /supplier/edit/{id}", s.supplierForm)
	s.mux.HandleFunc("POST /supplier/save", s.supplierSave)
	s.mux.HandleFunc("POST /supplier/delete/{id}", s.supplierDelete)
	s.mux.HandleFunc("GET /supplier/ledger", s.supplierLedgerList)
	s.mux.HandleFunc("GET /supplier/ledger/{id}", s.supplierLedger)
	s.mux.HandleFunc("GET /supplier/ledger/{id}/data", s.supplierLedgerData)
	s.mux.HandleFunc("POST /supplier/previous-balance/{id}", s.supplierPreviousBalance)
	s.mux.HandleFunc("GET /supplier/search", s.supplierSearch)
	s.mux.HandleFunc("GET /supplier/dropdown", s.supplierDropdown)
}

func (s *Server) supplierList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "supplier_list.html", map[string]interface{}{"Title": "Gestionar Proveedores"})
}

func (s *Server) supplierListData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE a.supplier_name LIKE ? OR a.mobile LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM supplier_information").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM supplier_information a"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.supplier_id, a.supplier_name, IFNULL(a.mobile,''), IFNULL(a.address,''), IFNULL(a.city,''),
			IFNULL(b.HeadCode,0),
			IFNULL((SELECT SUM(t.Credit)-SUM(t.Debit) FROM acc_transaction t WHERE t.COAID=b.HeadCode AND t.IsAppove=1),0) AS balance
		FROM supplier_information a
		LEFT JOIN acc_coa b ON b.supplier_id = a.supplier_id
		`+where+`
		ORDER BY a.supplier_id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, mobile, address, city sql.NullString
		var hc int64
		var balance float64
		if err := rows.Scan(&id, &name, &mobile, &address, &city, &hc, &balance); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":      id,
			"name":    name.String,
			"mobile":  mobile.String,
			"address": address.String,
			"city":    city.String,
			"balance": money(balance),
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

func (s *Server) supplierForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Añadir Proveedor"}
	if id := r.PathValue("id"); id != "" {
		var sup struct {
			ID, Name, Mobile, Email, Address, Address2, City, State, Zip, Country, Details, RIF sql.NullString
		}
		err := s.dbFor(r).QueryRow(`SELECT supplier_id, supplier_name, mobile, email_address, address, address2,
			city, state, zip, country, details, IFNULL(rif,'') FROM supplier_information WHERE supplier_id = ?`, id).
			Scan(&sup.ID, &sup.Name, &sup.Mobile, &sup.Email, &sup.Address, &sup.Address2, &sup.City, &sup.State, &sup.Zip, &sup.Country, &sup.Details, &sup.RIF)
		if err != nil {
			http.Error(w, "Proveedor no encontrado", 404)
			return
		}
		data["Title"] = "Editar Proveedor"
		data["S"] = sup
	}
	s.render(w, r, "supplier_form.html", data)
}

func (s *Server) supplierSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	name := strings.TrimSpace(r.FormValue("supplier_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	idStr := r.FormValue("supplier_id")

	// FISCAL: RIF validado (obligatorio para libro de compras con crédito fiscal)
	rif := NormalizeRIF(r.FormValue("rif"))
	if rif != "" {
		if ok, msg := ValidateRIF(rif); !ok {
			jsonError(w, "RIF inválido: "+msg, 400)
			return
		}
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if idStr == "" {
		res, err := tx.Exec(`
			INSERT INTO supplier_information (supplier_name, address, address2, mobile, email_address,
				city, state, zip, country, details, rif, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,1)`,
			name, r.FormValue("address"), r.FormValue("address2"), r.FormValue("mobile"),
			r.FormValue("email_address"), r.FormValue("city"), r.FormValue("state"),
			r.FormValue("zip"), r.FormValue("country"), r.FormValue("details"), rif)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		supplierID, _ := res.LastInsertId()
		// HeadCode under 21110% (Suppliers), seed 21110000001
		headcode, err := nextHeadCodeTx(tx, "21110", "2111000001")
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		_, _ = tx.Exec(`
			INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, HeadLevel, IsActive, IsTransaction, IsGL,
				HeadType, IsBudget, IsDepreciation, DepreciationRate, supplier_id, CreateBy, CreateDate, UpdateBy, UpdateDate, isSubType, subType)
			VALUES (?,?,?,?,1,1,0,'L',0,0,0,?,?,?,?,?,1,4)`,
			headcode, fmt.Sprintf("%d-%s", supplierID, name), "Suppliers", 4, supplierID, sessUserID(sess), now(), sessUserID(sess), now())
		_, _ = tx.Exec(`INSERT INTO acc_subcode (subTypeId, name, referenceNo, status, created_by, created_date, updated_by)
			VALUES (4, ?, ?, 1, ?, ?, ?)`, name, supplierID, sessUserID(sess), today(), sessUserID(sess))
	} else {
		_, err = tx.Exec(`
			UPDATE supplier_information SET supplier_name=?, address=?, address2=?, mobile=?, email_address=?,
				city=?, state=?, zip=?, country=?, details=?, rif=? WHERE supplier_id=?`,
			name, r.FormValue("address"), r.FormValue("address2"), r.FormValue("mobile"),
			r.FormValue("email_address"), r.FormValue("city"), r.FormValue("state"),
			r.FormValue("zip"), r.FormValue("country"), r.FormValue("details"), rif, idStr)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Guardado correctamente", "redirect": saveRedirect(r, "/supplier/list")})
}

func (s *Server) supplierDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM acc_subcode WHERE referenceNo = ? AND subTypeId = 4`, id)
	_, _ = tx.Exec(`DELETE FROM acc_coa WHERE supplier_id = ?`, id)
	res, err := tx.Exec(`DELETE FROM supplier_information WHERE supplier_id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Proveedor no encontrado", 404)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/supplier/list")})
}

func (s *Server) supplierLedgerList(w http.ResponseWriter, r *http.Request) {
	type sup struct {
		ID   int64
		Name string
	}
	list := []sup{}
	rows, err := s.dbFor(r).Query(`SELECT supplier_id, supplier_name FROM supplier_information ORDER BY supplier_name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c sup
			_ = rows.Scan(&c.ID, &c.Name)
			list = append(list, c)
		}
	}
	s.render(w, r, "ledger_picker.html", map[string]interface{}{
		"Title":     "Libro Mayor de Proveedores",
		"Kind":      "supplier",
		"Suppliers": list,
	})
}

func (s *Server) supplierLedger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name, mobile, address sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT supplier_name, mobile, address FROM supplier_information WHERE supplier_id=?`, id).
		Scan(&name, &mobile, &address)
	if err != nil {
		http.Error(w, "Proveedor no encontrado", 404)
		return
	}
	s.render(w, r, "supplier_ledger.html", map[string]interface{}{
		"Title":   "Libro Mayor: " + name.String,
		"SID":     id,
		"SName":   name.String,
		"SMobile": mobile.String,
		"SAddr":   address.String,
	})
}

func (s *Server) supplierLedgerData(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	from := r.FormValue("from")
	to := r.FormValue("to")

	var headcode int64
	err := s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE supplier_id = ? LIMIT 1`, id).Scan(&headcode)
	if err != nil {
		jsonError(w, "El proveedor no tiene cuenta contable", 400)
		return
	}

	q := `SELECT VDate, VNo, Vtype, Narration, Debit, Credit,
		IFNULL((SELECT SUM(x.Credit)-SUM(x.Debit) FROM acc_transaction x
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
		var d, vno, vtype, narr sql.NullString
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
		"balance":      money(totalC - totalD),
	})
}

// supplierPreviousBalance replicates supplier previous_balance_add:
// debit supplier account, credit Cash In Hand (1020101).
func (s *Server) supplierPreviousBalance(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	balance := parseMoney(r.FormValue("balance"))
	if balance == 0 {
		jsonError(w, "Monto inválido", 400)
		return
	}
	supplierID, _ := strconv.ParseInt(idStr, 10, 64)
	sess := s.sessionFromRequest(r)

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	var headcode int64
	if err := tx.QueryRow(`SELECT supplier_name FROM supplier_information WHERE supplier_id=?`, supplierID).Scan(&name); err != nil {
		jsonError(w, "Proveedor no encontrado", 404)
		return
	}
	if err := tx.QueryRow(`SELECT HeadCode FROM acc_coa WHERE supplier_id=? LIMIT 1`, supplierID).Scan(&headcode); err != nil {
		jsonError(w, "El proveedor no tiene cuenta contable", 400)
		return
	}

	tranID := generator(10)
	if _, err := tx.Exec(`INSERT INTO acc_transaction (VNo, Vtype, VDate, COAID, Narration, Debit, Credit, IsPosted, is_opening, CreateBy, CreateDate, IsAppove, vid, fyear, RevCodde)
		VALUES (?,?,?,?,?,?,0,1,1,?,?,1,0,0,0)`,
		tranID, "PR Balance", today(), headcode, "Supplier debit For "+name, balance, sessUserID(sess), now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// cash credit (PHP uses 111000001 -> our COA: Cash In Hand 1020101)
	if _, err := tx.Exec(`INSERT INTO acc_transaction (VNo, Vtype, VDate, COAID, Narration, Debit, Credit, IsPosted, is_opening, CreateBy, CreateDate, IsAppove, vid, fyear, RevCodde)
		VALUES (?,?,?,?,?,?,?,1,1,?,?,1,0,0,0)`,
		tranID, "PR Balance", today(), "1020101", "Cash credit for supplier previous balance "+name, 0, balance, sessUserID(sess), now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Balance anterior aplicado", "redirect": saveRedirect(r, "/supplier/list")})
}

func (s *Server) supplierSearch(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.FormValue("term"))
	if term == "" {
		writeJSON(w, []interface{}{})
		return
	}
	rows, err := s.dbFor(r).Query(`SELECT supplier_id, supplier_name, IFNULL(mobile,''), IFNULL(address,'')
		FROM supplier_information WHERE supplier_name LIKE ? OR mobile LIKE ? LIMIT 20`, "%"+term+"%", "%"+term+"%")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, mobile, addr sql.NullString
		_ = rows.Scan(&id, &name, &mobile, &addr)
		out = append(out, map[string]interface{}{
			"id": id, "label": name.String, "value": name.String,
			"mobile": mobile.String, "address": addr.String,
		})
	}
	writeJSON(w, out)
}

// supplierDropdown returns active suppliers as JSON (for purchase form).
func (s *Server) supplierDropdown(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT supplier_id, supplier_name FROM supplier_information WHERE status=1 ORDER BY supplier_name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		_ = rows.Scan(&id, &name)
		out = append(out, map[string]interface{}{"id": id, "text": name})
	}
	writeJSON(w, out)
}
