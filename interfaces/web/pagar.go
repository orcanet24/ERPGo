package web

// pagar.go — Cuentas por Pagar (Proveedores)

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerPagarRoutes() {
	s.mux.HandleFunc("GET /pagar/list", s.pagarList)
	s.mux.HandleFunc("/pagar/list/data", s.pagarListData)
	s.mux.HandleFunc("POST /pagar/save", s.pagarSave)
	s.mux.HandleFunc("POST /pagar/delete/{id}", s.pagarDelete)
	s.mux.HandleFunc("POST /pagar/pay", s.pagarPay)
	s.mux.HandleFunc("GET /pagar/statement/{supplier_id}", s.pagarStatement)
}

func (s *Server) pagarList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "pagar_list.html", map[string]interface{}{
		"Title":  "Cuentas por Pagar",
		"Module": "pagar",
	})
}

func (s *Server) pagarListData(w http.ResponseWriter, r *http.Request) {
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
		where = ` WHERE ap.reference LIKE ? OR ap.notes LIKE ? OR s.supplier_name LIKE ? OR ap.purchase_id LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	countQuery := "SELECT COUNT(*) FROM accounts_payable ap" + where
	var total int
	_ = s.dbFor(r).QueryRow(countQuery, args...).Scan(&total)

	dataQuery := fmt.Sprintf(`SELECT ap.id, ap.purchase_id, s.supplier_name, ap.amount, ap.paid_amount, ap.due_amount,
		ap.payment_method_id, ap.payment_date, ap.reference, ap.notes, ap.created_by, ap.created_at
		FROM accounts_payable ap
		LEFT JOIN supplier_information s ON s.supplier_id = ap.supplier_id
		%s ORDER BY ap.created_at DESC LIMIT %d OFFSET %d`, where, length, start)

	rows, err := s.dbFor(r).Query(dataQuery, args...)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id int
		var amount, paidAmount, dueAmount float64
		var purchaseID, supplierName, reference, notes, createdBy, createdAt sql.NullString
		var paymentMethodID sql.NullInt64
		var paymentDate sql.NullString
		_ = rows.Scan(&id, &purchaseID, &supplierName, &amount, &paidAmount, &dueAmount,
			&paymentMethodID, &paymentDate, &reference, &notes, &createdBy, &createdAt)
		items = append(items, map[string]interface{}{
			"id":               id,
			"purchase_id":      purchaseID.String,
			"supplier_name":    supplierName.String,
			"amount":           amount,
			"paid_amount":      paidAmount,
			"due_amount":       dueAmount,
			"payment_method_id": paymentMethodID.Int64,
			"payment_date":     paymentDate.String,
			"reference":        reference.String,
			"notes":            notes.String,
			"created_by":       createdBy.String,
			"created_at":       createdAt.String,
		})
	}

	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": total,
		"data":            items,
	})
}

func (s *Server) pagarSave(w http.ResponseWriter, r *http.Request) {
	supplierID, _ := strconv.Atoi(r.FormValue("supplier_id"))
	purchaseID := r.FormValue("purchase_id")
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	paymentMethodID, _ := strconv.Atoi(r.FormValue("payment_method_id"))
	reference := r.FormValue("reference")
	notes := r.FormValue("notes")

	if supplierID <= 0 || amount <= 0 {
		jsonError(w, "missing fields", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`INSERT INTO accounts_payable (supplier_id, purchase_id, amount, paid_amount, due_amount,
		payment_method_id, payment_date, reference, notes, created_by, created_at)
		VALUES (?, ?, ?, 0, ?, ?, NOW(), ?, ?, ?, NOW())`,
		supplierID, purchaseID, amount, amount, paymentMethodID, reference, notes, s.sessionUserID(r))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"saved": true})
}

func (s *Server) pagarDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/pagar/delete/")
	if id == "" {
		jsonError(w, "missing id", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)
	_, err := db.Exec("DELETE FROM accounts_payable WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"deleted": true})
}

func (s *Server) pagarPay(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.FormValue("id"))
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	paymentMethodID, _ := strconv.Atoi(r.FormValue("payment_method_id"))
	reference := r.FormValue("reference")
	notes := r.FormValue("notes")

	if id <= 0 || amount <= 0 {
		jsonError(w, "missing fields", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`UPDATE accounts_payable SET paid_amount = paid_amount + ?, due_amount = due_amount - ?,
		payment_method_id = ?, reference = ?, notes = ?, payment_date = NOW() WHERE id = ?`,
		amount, amount, paymentMethodID, reference, notes, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`INSERT INTO acc_transaction (VNo, Vtype, COAID, Debit, Credit, Narration, referenceNo, VDate, CreateBy, CreateDate)
		VALUES (?, 'Payment', ?, 0, ?, ?, ?, CURDATE(), ?, NOW())`,
		reference, paymentMethodID, amount, notes, reference, s.sessionUserID(r))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"paid": true})
}

func (s *Server) pagarStatement(w http.ResponseWriter, r *http.Request) {
	supplierID := strings.TrimPrefix(r.URL.Path, "/pagar/statement/")
	if supplierID == "" {
		jsonError(w, "missing supplier id", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)

	var supplierName string
	_ = db.QueryRow("SELECT supplier_name FROM supplier_information WHERE supplier_id = ?", supplierID).Scan(&supplierName)

	rows, err := db.Query(`SELECT id, purchase_id, amount, paid_amount, due_amount, payment_date, reference, created_at
		FROM accounts_payable WHERE supplier_id = ? ORDER BY created_at DESC`, supplierID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id int
		var amount, paidAmount, dueAmount float64
		var purchaseID, reference, paymentDate, createdAt sql.NullString
		_ = rows.Scan(&id, &purchaseID, &amount, &paidAmount, &dueAmount, &paymentDate, &reference, &createdAt)
		items = append(items, map[string]interface{}{
			"id":          id,
			"purchase_id": purchaseID.String,
			"amount":      amount,
			"paid_amount": paidAmount,
			"due_amount":  dueAmount,
			"payment_date": paymentDate.String,
			"reference":   reference.String,
			"created_at":  createdAt.String,
		})
	}

	writeJSON(w, map[string]interface{}{
		"supplier_id":   supplierID,
		"supplier_name": supplierName,
		"items":         items,
	})
}
