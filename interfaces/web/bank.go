package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Bank module (bank_information + bank_transaction) ----------
// Cada cuenta bancaria crea su COA bajo 'Cash at Bank' (prefijo 10205,
// HeadCode automático 1020501+ con isBankNature=1 y bank_id vinculado).
// Las transacciones (deposit/withdraw/transfer) generan comprobante contable:
//   deposit  -> DV:     Débito Banco, Crédito Caja u otra cuenta
//   withdraw -> CV:     Débito Gasto u otra cuenta, Crédito Banco
//   transfer -> CONTRA: Débito Banco destino, Crédito Banco origen

func (s *Server) registerBankRoutes() {
	s.mux.HandleFunc("GET /bank/list", s.bankList)
	s.mux.HandleFunc("/bank/list/data", s.bankListData)
	s.mux.HandleFunc("GET /bank/add", s.bankForm)
	s.mux.HandleFunc("GET /bank/edit/{id}", s.bankForm)
	s.mux.HandleFunc("POST /bank/save", s.bankSave)
	s.mux.HandleFunc("POST /bank/delete/{id}", s.bankDelete)
	s.mux.HandleFunc("GET /bank/ledger", s.bankLedger)
	s.mux.HandleFunc("/bank/ledger/data", s.bankLedgerData)
	s.mux.HandleFunc("GET /bank/transaction", s.bankTransactionForm)
	s.mux.HandleFunc("POST /bank/transaction/save", s.bankTransactionSave)
}

// bankList renders the bank accounts page.
func (s *Server) bankList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "bank_list.html", map[string]interface{}{"Title": "Cuentas Bancarias"})
}

// bankListData: DataTables serverSide con saldo actual
// (opening_balance + movimientos contables aprobados del COA vinculado).
func (s *Server) bankListData(w http.ResponseWriter, r *http.Request) {
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
		where = " WHERE a.bank_name LIKE ? OR a.bank_id LIKE ? OR IFNULL(a.ac_name,'') LIKE ? OR IFNULL(a.ac_number,'') LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM bank_information").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM bank_information a"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.id, a.bank_id, a.bank_name, IFNULL(a.ac_name,''), IFNULL(a.ac_number,''), IFNULL(a.branch,''),
			a.opening_balance, a.status
		FROM bank_information a
		`+where+`
		ORDER BY a.id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var bankID, name, acName, acNumber, branch sql.NullString
		var opening float64
		var status int64
		if err := rows.Scan(&id, &bankID, &name, &acName, &acNumber, &branch, &opening, &status); err != nil {
			continue
		}
		// COA + saldo: consultas parametrizadas (evita mezcla de collations
		// entre acc_coa legacy y bank_information al comparar columnas).
		var headcode int64
		_ = s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE bank_id = ? LIMIT 1`, bankID.String).Scan(&headcode)
		balance := opening
		if headcode > 0 {
			var movD, movC float64
			_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0)
				FROM acc_transaction WHERE COAID = ? AND IsAppove = 1`, headcode).Scan(&movD, &movC)
			balance = opening + movD - movC
		}
		list = append(list, map[string]interface{}{
			"id": id, "bank_id": bankID.String, "name": name.String,
			"ac_name": acName.String, "ac_number": acNumber.String,
			"branch": branch.String, "opening": money(opening),
			"balance": money(balance), "status": status, "headcode": headcode,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

// bankForm: formulario añadir/editar cuenta bancaria.
func (s *Server) bankForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Añadir Cuenta Bancaria"}
	if id := r.PathValue("id"); id != "" {
		var bankID, bankName, acName, acNumber, branch, swift, ifsc sql.NullString
		var balance float64
		var status int
		err := s.dbFor(r).QueryRow(`SELECT bank_id, bank_name, ac_name, ac_number, branch, swift_code, ifsc_code, opening_balance, status
			FROM bank_information WHERE id = ?`, id).
			Scan(&bankID, &bankName, &acName, &acNumber, &branch, &swift, &ifsc, &balance, &status)
		if err != nil {
			http.Error(w, "Cuenta no encontrada", 404)
			return
		}
		var headcode int64
		_ = s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE bank_id = ? LIMIT 1`, bankID.String).Scan(&headcode)
		data["Title"] = "Editar Cuenta Bancaria"
		data["B"] = map[string]interface{}{
			"id": id, "bank_id": bankID.String, "name": bankName.String,
			"ac_name": acName.String, "ac_number": acNumber.String,
			"branch": branch.String, "swift": swift.String, "ifsc": ifsc.String,
			"balance": balance, "status": status, "headcode": headcode,
		}
	}
	s.render(w, r, "bank_form.html", data)
}

// bankHeadName: nombre del COA del banco (nombre + número de cuenta).
func bankHeadName(name, acNumber string) string {
	if acNumber != "" {
		return fmt.Sprintf("%s - %s", name, acNumber)
	}
	return name
}

// bankSave: inserta/actualiza bank_information y crea el COA bajo
// 'Cash at Bank' (HeadCode automático) al dar de alta una cuenta.
func (s *Server) bankSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	name := strings.TrimSpace(r.FormValue("bank_name"))
	if name == "" {
		jsonError(w, "El nombre del banco es obligatorio", 400)
		return
	}
	id := r.FormValue("id")
	bankID := strings.TrimSpace(r.FormValue("bank_id"))
	acName := r.FormValue("ac_name")
	acNumber := r.FormValue("ac_number")
	opening := parseMoney(r.FormValue("opening_balance"))
	status := 1
	if r.FormValue("status") == "0" {
		status = 0
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if id == "" {
		// alta: bank_id automático si no se indica
		if bankID == "" {
			bankID = fmt.Sprintf("BANK-%s", generator(6))
		}
		var dup int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM bank_information WHERE bank_id = ?`, bankID).Scan(&dup); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if dup > 0 {
			jsonError(w, "El ID de banco ya existe", 400)
			return
		}
		if _, err := tx.Exec(`
			INSERT INTO bank_information (bank_id, bank_name, ac_name, ac_number, branch, swift_code, ifsc_code, opening_balance, status)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			bankID, name, acName, acNumber, r.FormValue("branch"), r.FormValue("swift_code"),
			r.FormValue("ifsc_code"), opening, status); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// COA bajo 'Cash at Bank' (1020501+), isBankNature=1, vinculado por bank_id
		headcode, err := nextHeadCodeTx(tx, "10205", "1020501")
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if _, err := tx.Exec(`
			INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, PHeadCode, HeadLevel, IsActive, IsTransaction,
				IsGL, isCashNature, isBankNature, HeadType, IsBudget, IsDepreciation, DepreciationRate,
				bank_id, CreateBy, CreateDate, UpdateBy, UpdateDate, isSubType, subType)
			VALUES (?,?,?,?,?,1,1,0,0,1,'A',0,0,0,?,?,?,?,?,0,1)`,
			headcode, bankHeadName(name, acNumber), "Cash at Bank", "10205", 4, bankID,
			sessUserID(sess), now(), sessUserID(sess), now()); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		res, err := tx.Exec(`UPDATE bank_information SET bank_name=?, ac_name=?, ac_number=?, branch=?,
			swift_code=?, ifsc_code=?, opening_balance=?, status=? WHERE id=?`,
			name, acName, acNumber, r.FormValue("branch"), r.FormValue("swift_code"),
			r.FormValue("ifsc_code"), opening, status, id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "Cuenta no encontrada", 404)
			return
		}
		// mantener el nombre del COA sincronizado (por pasos: evita comparar
		// columnas de tablas con collations distintas)
		var oldBankID string
		if err := tx.QueryRow(`SELECT bank_id FROM bank_information WHERE id = ?`, id).Scan(&oldBankID); err == nil {
			if _, err := tx.Exec(`UPDATE acc_coa SET HeadName=?, UpdateBy=?, UpdateDate=? WHERE bank_id = ?`,
				bankHeadName(name, acNumber), sessUserID(sess), now(), oldBankID); err != nil {
				jsonError(w, fmt.Sprintf("actualizar cuenta contable del banco: %v", err), 500)
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cuenta guardada", "redirect": saveRedirect(r, "/bank/list")})
}

// bankDelete: elimina la cuenta si no tiene transacciones bancarias
// ni movimientos contables; borra también su COA vinculado.
func (s *Server) bankDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var bankID string
	if err := s.dbFor(r).QueryRow(`SELECT bank_id FROM bank_information WHERE id = ?`, id).Scan(&bankID); err != nil {
		jsonError(w, "Cuenta no encontrada", 404)
		return
	}

	var txCount int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM bank_transaction WHERE bank_id = ?`, bankID).Scan(&txCount)
	if txCount > 0 {
		jsonError(w, "No se puede eliminar: la cuenta tiene transacciones bancarias", 400)
		return
	}

	var headcode int64
	_ = s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE bank_id = ? LIMIT 1`, bankID).Scan(&headcode)
	if headcode > 0 {
		var used int
		_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_transaction WHERE COAID = ?`, headcode).Scan(&used)
		if used > 0 {
			jsonError(w, "No se puede eliminar: la cuenta contable tiene movimientos", 400)
			return
		}
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM acc_coa WHERE bank_id = ?`, bankID); err != nil {
		jsonError(w, fmt.Sprintf("limpiar cuenta contable del banco: %v", err), 500)
		return
	}
	res, err := tx.Exec(`DELETE FROM bank_information WHERE id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Cuenta no encontrada", 404)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cuenta eliminada", "redirect": saveRedirect(r, "/bank/list")})
}

// bankOptions: bancos activos para dropdowns.
func (s *Server) bankOptions(r *http.Request) []map[string]interface{} {
	banks := []map[string]interface{}{}
	rows, _ := s.dbFor(r).Query(`SELECT id, bank_id, bank_name, IFNULL(ac_number,'')
		FROM bank_information WHERE status=1 ORDER BY bank_name`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var bankID, name, ac sql.NullString
			_ = rows.Scan(&id, &bankID, &name, &ac)
			banks = append(banks, map[string]interface{}{
				"id": id, "bank_id": bankID.String, "name": name.String, "ac": ac.String,
			})
		}
		rows.Close()
	}
	return banks
}

// bankLedger: página del libro bancario (selector de banco + filtros).
func (s *Server) bankLedger(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "bank_ledger.html", map[string]interface{}{
		"Title": "Libro Bancario",
		"Banks": s.bankOptions(r),
	})
}

// bankLedgerData: movimientos de bank_transaction con saldo corrido.
// Apertura = opening_balance + movimientos anteriores al rango.
func (s *Server) bankLedgerData(w http.ResponseWriter, r *http.Request) {
	key := r.FormValue("bank_id")
	from := orDefault(r.FormValue("from"), today()[:4]+"-01-01")
	to := orDefault(r.FormValue("to"), today())
	if key == "" {
		jsonError(w, "Seleccione una cuenta bancaria", 400)
		return
	}

	var bankID, name, acNumber sql.NullString
	var opening0 float64
	err := s.dbFor(r).QueryRow(`SELECT bank_id, bank_name, IFNULL(ac_number,''), opening_balance
		FROM bank_information WHERE id = ? OR bank_id = ? LIMIT 1`, key, key).
		Scan(&bankID, &name, &acNumber, &opening0)
	if err != nil {
		jsonError(w, "Cuenta bancaria no encontrada", 404)
		return
	}

	// movimientos anteriores al rango (deposit entra, withdraw/transfer sale)
	var inBefore, outBefore float64
	_ = s.dbFor(r).QueryRow(`
		SELECT IFNULL(SUM(CASE WHEN transaction_type='deposit' THEN amount ELSE 0 END),0),
			IFNULL(SUM(CASE WHEN transaction_type IN ('withdraw','transfer') THEN amount ELSE 0 END),0)
		FROM bank_transaction WHERE bank_id = ? AND date < ?`, bankID.String, from).Scan(&inBefore, &outBefore)
	balance := opening0 + inBefore - outBefore
	opening := balance

	rows, err := s.dbFor(r).Query(`
		SELECT date, transaction_type, IFNULL(reference,''), IFNULL(description,''), amount
		FROM bank_transaction
		WHERE bank_id = ? AND date BETWEEN ? AND ?
		ORDER BY date ASC, id ASC`, bankID.String, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type lrow struct {
		Date        string  `json:"date"`
		Type        string  `json:"type"`
		Ref         string  `json:"ref"`
		Description string  `json:"description"`
		Debit       float64 `json:"debit"`
		Credit      float64 `json:"credit"`
		Balance     float64 `json:"balance"`
	}
	list := []lrow{}
	var totalD, totalC float64
	for rows.Next() {
		var date, ttype, ref, desc sql.NullString
		var amount float64
		if err := rows.Scan(&date, &ttype, &ref, &desc, &amount); err != nil {
			continue
		}
		debit, credit := 0.0, 0.0
		switch ttype.String {
		case "deposit":
			debit = amount
			balance += amount
		case "withdraw", "transfer":
			credit = amount
			balance -= amount
		}
		list = append(list, lrow{date.String, ttype.String, ref.String, desc.String, debit, credit, balance})
		totalD += debit
		totalC += credit
	}
	writeJSON(w, map[string]interface{}{
		"bank":         name.String,
		"ac_number":    acNumber.String,
		"opening":      money(opening),
		"data":         list,
		"total_debit":  money(totalD),
		"total_credit": money(totalC),
		"closing":      money(balance),
	})
}

// bankTransactionForm: formulario de movimiento bancario.
func (s *Server) bankTransactionForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "Transacción Bancaria",
		"Banks": s.bankOptions(r),
		"Heads": s.transactionalHeads(r),
	}
	if pre, err := s.predefineAccount(r); err == nil {
		data["CashCode"] = pre.CashCode
	}
	s.render(w, r, "bank_transaction_form.html", data)
}

// headCodeByBankTx: HeadCode del COA vinculado a un bank_id (dentro de tx).
func headCodeByBankTx(tx *sql.Tx, bankID string) (int64, error) {
	var hc int64
	err := tx.QueryRow(`SELECT HeadCode FROM acc_coa WHERE bank_id = ? LIMIT 1`, bankID).Scan(&hc)
	return hc, err
}

// bankTransactionSave: registra el movimiento en bank_transaction y genera
// el asiento contable (DV/CV/CONTRA) con la doble partida del motor.
func (s *Server) bankTransactionSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	bankKey := strings.TrimSpace(r.FormValue("bank_id"))
	txType := r.FormValue("transaction_type")
	amount := parseMoney(r.FormValue("amount"))
	ref := strings.TrimSpace(r.FormValue("reference"))
	desc := strings.TrimSpace(r.FormValue("description"))
	date := orDefault(r.FormValue("date"), today())

	switch txType {
	case "deposit", "withdraw", "transfer":
	default:
		jsonError(w, "Tipo de transacción inválido", 400)
		return
	}
	if amount <= 0 {
		jsonError(w, "El monto debe ser mayor a cero", 400)
		return
	}
	if bankKey == "" {
		jsonError(w, "Seleccione la cuenta bancaria", 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// banco origen -> bank_id + COA
	var bankID string
	if err := tx.QueryRow(`SELECT bank_id FROM bank_information WHERE id = ? OR bank_id = ? LIMIT 1`, bankKey, bankKey).Scan(&bankID); err != nil {
		jsonError(w, "Cuenta bancaria no encontrada", 404)
		return
	}
	bankCOA, err := headCodeByBankTx(tx, bankID)
	if err != nil {
		jsonError(w, "La cuenta bancaria no tiene cuenta contable", 400)
		return
	}

	var vno string
	switch txType {
	case "deposit":
		// Débito Banco, Crédito Caja u otra cuenta
		contra, _ := strconv.ParseInt(r.FormValue("contra_account"), 10, 64)
		if contra == 0 {
			jsonError(w, "Seleccione la cuenta de origen de los fondos", 400)
			return
		}
		vno = nextVoucherNoTx(tx, "DV")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "DV", ReferenceNo: ref, VDate: date,
			COAID: bankCOA, RevCodde: contra,
			Narration: "Depósito bancario " + bankID, Comment: desc,
			Debit: amount,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	case "withdraw":
		// Débito Gasto u otra cuenta, Crédito Banco
		contra, _ := strconv.ParseInt(r.FormValue("contra_account"), 10, 64)
		if contra == 0 {
			jsonError(w, "Seleccione la cuenta de destino de los fondos", 400)
			return
		}
		vno = nextVoucherNoTx(tx, "CV")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "CV", ReferenceNo: ref, VDate: date,
			COAID: contra, RevCodde: bankCOA,
			Narration: "Retiro bancario " + bankID, Comment: desc,
			Debit: amount,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	case "transfer":
		// Débito Banco destino, Crédito Banco origen
		destKey := strings.TrimSpace(r.FormValue("dest_bank_id"))
		if destKey == "" {
			jsonError(w, "Seleccione el banco destino", 400)
			return
		}
		var destID string
		if err := tx.QueryRow(`SELECT bank_id FROM bank_information WHERE id = ? OR bank_id = ? LIMIT 1`, destKey, destKey).Scan(&destID); err != nil {
			jsonError(w, "Banco destino no encontrado", 404)
			return
		}
		if destID == bankID {
			jsonError(w, "El banco destino debe ser distinto al de origen", 400)
			return
		}
		destCOA, err := headCodeByBankTx(tx, destID)
		if err != nil {
			jsonError(w, "El banco destino no tiene cuenta contable", 400)
			return
		}
		vno = nextVoucherNoTx(tx, "CONTRA")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "CONTRA", ReferenceNo: ref, VDate: date,
			COAID: destCOA, RevCodde: bankCOA,
			Narration: "Transferencia bancaria " + bankID + " -> " + destID, Comment: desc,
			Debit: amount,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO bank_transaction (bank_id, date, transaction_type, amount, reference, description, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		bankID, date, txType, amount, ref, desc, createBy, now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// autoaprobación del comprobante (mismo patrón que el resto del ERP)
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, createBy)
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Transacción registrada", "vno": vno})
}

// REGISTRO: s.registerBankRoutes() ya está registrado en server.go (NewServer).
