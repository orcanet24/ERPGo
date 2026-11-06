package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Account module (replicates modules/account) ----------

func (s *Server) registerAccountRoutes() {
	// COA
	s.mux.HandleFunc("GET /account/coa", s.coaTree)
	s.mux.HandleFunc("GET /account/coa/data", s.coaData)
	s.mux.HandleFunc("POST /account/coa/save", s.coaSave)
	s.mux.HandleFunc("POST /account/coa/delete/{id}", s.coaDelete)
	// Vouchers
	s.mux.HandleFunc("GET /account/vouchers", s.voucherList)
	s.mux.HandleFunc("/account/vouchers/data", s.voucherListData)
	s.mux.HandleFunc("GET /account/voucher/new", s.voucherForm)
	s.mux.HandleFunc("POST /account/voucher/save", s.voucherSave)
	s.mux.HandleFunc("POST /account/voucher/approve/{vno}", s.voucherApprove)
	s.mux.HandleFunc("POST /account/voucher/reverse/{vno}", s.voucherReverse)
	s.mux.HandleFunc("POST /account/voucher/delete/{vno}", s.voucherDelete)
	// Ledger & reports
	s.mux.HandleFunc("GET /account/ledger", s.generalLedger)
	s.mux.HandleFunc("GET /account/ledger/data", s.generalLedgerData)
	s.mux.HandleFunc("GET /account/trial-balance", s.trialBalance)
	s.mux.HandleFunc("GET /account/profit-loss", s.profitLoss)
	s.mux.HandleFunc("GET /account/balance-sheet", s.balanceSheet)
	s.mux.HandleFunc("GET /account/income-statement", s.incomeStatement)
	// payment heads dropdown
	s.mux.HandleFunc("GET /account/heads", s.headsDropdown)
	s.mux.HandleFunc("GET /account/heads-transactions", s.headsTransactions)
}

// ---------- COA ----------

func (s *Server) coaTree(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "coa.html", map[string]interface{}{"Title": "Plan de Cuentas"})
}

// coaData returns the full tree as nested JSON (jsTree compatible).
//
// El JSON va SOLO con `children` anidados y sin `parent`. jsTree decide entre
// sus dos parseadores mirando exclusivamente el primer elemento del array: si
// tiene `parent`, trata todo el envío como lista plana y exige que cada padre
// exista entre los nodos. Las cuentas raíz (PHeadCode 0 o NULL) no tienen ese
// nodo, y jsTree 3.3.7 revienta dentro de su Web Worker con «Cannot read
// properties of undefined (reading 'core')» —un fallo que además deja el
// árbol en blanco sin decir nada—. Sin `parent`, jsTree usa el parseador de
// árbol anidado, que es el que corresponde a la jerarquía real de acc_coa.
func (s *Server) coaData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT id, HeadCode, HeadName, PHeadName, PHeadCode, HeadLevel, HeadType,
		IsActive, IsTransaction, IsGL, isCashNature, isBankNature FROM acc_coa ORDER BY HeadCode`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type node struct {
		ID       string  `json:"id"`
		Text     string  `json:"text"`
		HeadCode int64   `json:"headcode"`
		Level    int     `json:"level"`
		Type     string  `json:"type"`
		Children []*node `json:"children,omitempty"`
	}
	// build flat map
	byCode := map[int64]*node{}
	// parentCode guarda el PHeadCode declarado por cada cuenta. Es la fuente de
	// verdad de la jerarquía: recortar dígitos del HeadCode (parentOf) acierta
	// casi siempre, pero se equivoca en cuentas como 40301 (Purchase Account),
	// cuyo PHeadCode es 402 y no el 403 que saldría del recorte.
	parentCode := map[int64]int64{}
	var order []int64
	for rows.Next() {
		var id, hc, level, active, istrans, isgl, cash, bank int
		var hname, pname, pcode, htype sql.NullString
		_ = rows.Scan(&id, &hc, &hname, &pname, &pcode, &level, &htype, &active, &istrans, &isgl, &cash, &bank)
		var pcodeI int64
		if pcode.Valid && pcode.String != "" && pcode.String != "0" {
			fmt.Sscanf(pcode.String, "%d", &pcodeI)
		}
		n := &node{
			ID:       fmt.Sprintf("%d", hc),
			Text:     fmt.Sprintf("%d - %s", hc, hname.String),
			HeadCode: int64(hc),
			Level:    level,
			Type:     htype.String,
		}
		byCode[int64(hc)] = n
		parentCode[int64(hc)] = pcodeI
		order = append(order, int64(hc))
	}
	// attach children
	var roots []*node
	for _, code := range order {
		n := byCode[code]
		parent, ok := byCode[parentCode[code]]
		if !ok || parent == n {
			// Sin PHeadCode o colgando de una cuenta que no existe: se prueba el
			// recorte del HeadCode, que era la jerarquía histórica del COA.
			parent, ok = byCode[parentOf(code)]
		}
		if ok && parent != n {
			parent.Children = append(parent.Children, n)
		} else {
			roots = append(roots, n)
		}
	}
	writeJSON(w, roots)
}

// parentOf derives the parent HeadCode by trimming trailing pairs of digits
// (HeadCode hierarchy: 1 -> 102 -> 10201 -> 1020101 ...).
func parentOf(code int64) int64 {
	s := fmt.Sprintf("%d", code)
	for l := len(s) - 2; l >= 1; l -= 2 {
		var p int64
		fmt.Sscanf(s[:l], "%d", &p)
		if p > 0 {
			return p
		}
	}
	return 0
}

// coaSave creates/updates a COA account.
func (s *Server) coaSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	name := strings.TrimSpace(r.FormValue("HeadName"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	id := r.FormValue("id")

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if id == "" {
		// new account under a parent
		parentCode := r.FormValue("PHeadCode")
		var pcode int64
		fmt.Sscanf(parentCode, "%d", &pcode)
		var plevel int
		var ptype, pname sql.NullString
		if err := tx.QueryRow(`SELECT HeadLevel, HeadType, HeadName FROM acc_coa WHERE HeadCode=?`, pcode).
			Scan(&plevel, &ptype, &pname); err != nil {
			jsonError(w, "Cuenta padre no encontrada", 400)
			return
		}
		// next child code: MAX(HeadCode)+1 among children of this parent level
		var maxChild sql.NullInt64
		prefixLen := len(fmt.Sprintf("%d", pcode)) + 2
		_ = tx.QueryRow(`SELECT MAX(HeadCode) FROM acc_coa WHERE CHAR_LENGTH(HeadCode) = ? AND HeadCode LIKE ?`,
			prefixLen, fmt.Sprintf("%d%%", pcode)).Scan(&maxChild)
		newCode := pcode*100 + 1
		if maxChild.Valid && maxChild.Int64 >= newCode {
			newCode = maxChild.Int64 + 1
		}

		if _, err := tx.Exec(`
			INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, PHeadCode, HeadLevel, IsActive, IsTransaction,
				IsGL, HeadType, IsBudget, IsDepreciation, DepreciationRate, CreateBy, CreateDate, UpdateBy, UpdateDate, isSubType, subType)
			VALUES (?,?,?,?,?,1,?,?,?,0,0,0,?,?,?,?,0,1)`,
			newCode, name, pname.String, parentCode, plevel+1,
			boolInt(r.FormValue("IsTransaction") == "1"), boolInt(r.FormValue("IsGL") == "1"),
			ptype.String, sessUserID(sess), now(), sessUserID(sess), now()); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		if _, err := tx.Exec(`UPDATE acc_coa SET HeadName=?, IsActive=? WHERE id=?`,
			name, boolInt(r.FormValue("IsActive") == "1"), id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cuenta guardada", "redirect": saveRedirect(r, "/account/coa")})
}

func (s *Server) coaDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// only allow deleting transaction-less leaf accounts
	var used int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_transaction WHERE COAID = ?`, id).Scan(&used)
	if used > 0 {
		jsonError(w, "No se puede eliminar: la cuenta tiene movimientos", 400)
		return
	}
	var children int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE PHeadCode = ?`, id).Scan(&children)
	if children > 0 {
		jsonError(w, "No se puede eliminar: la cuenta tiene subcuentas", 400)
		return
	}
	if _, err := s.dbFor(r).Exec(`DELETE FROM acc_coa WHERE HeadCode = ? AND IsTransaction = 1`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cuenta eliminada", "redirect": saveRedirect(r, "/account/coa")})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------- Vouchers ----------

func (s *Server) voucherList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "voucher_list.html", map[string]interface{}{"Title": "Comprobantes"})
}

func (s *Server) voucherListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}
	vtype := strings.TrimSpace(r.FormValue("vtype"))
	search := strings.TrimSpace(r.FormValue("search[value]"))

	where := " WHERE 1=1 "
	args := []interface{}{}
	if vtype != "" {
		where += " AND a.Vtype = ? "
		args = append(args, vtype)
	}
	if search != "" {
		where += " AND (a.VNo LIKE ? OR a.referenceNo LIKE ?) "
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(DISTINCT VNo) FROM acc_vaucher").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(DISTINCT a.VNo) FROM acc_vaucher a"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.VNo, a.Vtype, a.VDate, a.referenceNo, a.Narration,
			SUM(a.Debit) AS debit, SUM(a.Credit) AS credit, MAX(a.isApproved) AS approved
		FROM acc_vaucher a `+where+`
		GROUP BY a.VNo, a.Vtype, a.VDate, a.referenceNo, a.Narration
		ORDER BY MAX(a.id) DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var vno, vtype, vdate, ref, narr sql.NullString
		var debit, credit float64
		var approved int
		if err := rows.Scan(&vno, &vtype, &vdate, &ref, &narr, &debit, &credit, &approved); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"vno": vno.String, "vtype": vtype.String, "date": vdate.String,
			"ref": ref.String, "narration": narr.String,
			"debit": money(debit), "credit": money(credit), "approved": approved,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// voucherForm: manual voucher entry (DV/CV/CONTRA/JV).
func (s *Server) voucherForm(w http.ResponseWriter, r *http.Request) {
	vtype := r.FormValue("type")
	if vtype == "" {
		vtype = "DV"
	}
	titles := map[string]string{"DV": "Comprobante de Débito", "CV": "Comprobante de Crédito",
		"CONTRA": "Comprobante Contra", "JV": "Comprobante de Diario"}
	s.render(w, r, "voucher_form.html", map[string]interface{}{
		"Title": titles[vtype],
		"VType": vtype,
		"Heads": s.transactionalHeads(r),
	})
}

// transactionalHeads: accounts that accept transactions.
func (s *Server) transactionalHeads(r *http.Request) []map[string]interface{} {
	heads := []map[string]interface{}{}
	rows, _ := s.dbFor(r).Query(`SELECT HeadCode, HeadName, HeadType FROM acc_coa WHERE IsTransaction=1 AND IsActive=1 ORDER BY HeadCode`)
	if rows != nil {
		for rows.Next() {
			var hc int64
			var hn, ht string
			_ = rows.Scan(&hc, &hn, &ht)
			heads = append(heads, map[string]interface{}{"id": hc, "name": hn, "type": ht})
		}
		rows.Close()
	}
	return heads
}

// voucherSave: manual voucher with debit/credit rows.
func (s *Server) voucherSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)
	vtype := r.FormValue("Vtype")
	if vtype == "" {
		vtype = "JV"
	}
	vdate := orDefault(r.FormValue("VDate"), today())
	reference := r.FormValue("referenceNo")
	narration := r.FormValue("Narration")

	coaIDs := r.Form["COAID"]
	debits := r.Form["Debit"]
	credits := r.Form["Credit"]
	revCodes := r.Form["RevCodde"]
	remarks := r.Form["remark"]

	if len(coaIDs) < 2 {
		jsonError(w, "Un comprobante requiere al menos 2 líneas", 400)
		return
	}
	var sumD, sumC float64
	for i := range coaIDs {
		d, c := 0.0, 0.0
		if i < len(debits) {
			d = parseMoney(debits[i])
		}
		if i < len(credits) {
			c = parseMoney(credits[i])
		}
		sumD += d
		sumC += c
	}
	if sumD != sumC {
		jsonError(w, fmt.Sprintf("Débito (%.2f) y Crédito (%.2f) no cuadran", sumD, sumC), 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	vno := nextVoucherNoTx(tx, vtype)
	for i, coaStr := range coaIDs {
		coaid, _ := strconv.ParseInt(coaStr, 10, 64)
		d, c := 0.0, 0.0
		if i < len(debits) {
			d = parseMoney(debits[i])
		}
		if i < len(credits) {
			c = parseMoney(credits[i])
		}
		if d == 0 && c == 0 {
			continue
		}
		rev := int64(0)
		if i < len(revCodes) {
			rev, _ = strconv.ParseInt(revCodes[i], 10, 64)
		}
		remark := narration
		if i < len(remarks) && remarks[i] != "" {
			remark = remarks[i]
		}
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: vtype, ReferenceNo: reference, VDate: vdate,
			COAID: coaid, RevCodde: rev, Narration: remark,
			Debit: d, Credit: c,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// autoapprove?
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, createBy)
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Comprobante guardado", "vno": vno})
}

func (s *Server) voucherApprove(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	vno := r.PathValue("vno")
	if err := s.approveVoucher(r, vno, sessUserID(sess)); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Comprobante aprobado", "redirect": saveRedirect(r, "/account/vouchers")})
}

func (s *Server) voucherReverse(w http.ResponseWriter, r *http.Request) {
	vno := r.PathValue("vno")
	if err := s.reverseVoucher(r, vno); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Comprobante revertido", "redirect": saveRedirect(r, "/account/vouchers")})
}

func (s *Server) voucherDelete(w http.ResponseWriter, r *http.Request) {
	vno := r.PathValue("vno")
	if err := s.deleteVoucher(r, vno); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Comprobante eliminado", "redirect": saveRedirect(r, "/account/vouchers")})
}

// ---------- Ledger & reports ----------

func (s *Server) generalLedger(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "general_ledger.html", map[string]interface{}{
		"Title": "Libro Mayor",
		"Heads": s.transactionalHeads(r),
	})
}

func (s *Server) generalLedgerData(w http.ResponseWriter, r *http.Request) {
	headcode := r.FormValue("headcode")
	from := orDefault(r.FormValue("from"), "2000-01-01")
	to := orDefault(r.FormValue("to"), today())
	if headcode == "" {
		jsonError(w, "Seleccione una cuenta", 400)
		return
	}

	// opening balance
	var headType string
	var openD, openC float64
	_ = s.dbFor(r).QueryRow(`SELECT HeadType FROM acc_coa WHERE HeadCode=?`, headcode).Scan(&headType)
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0) FROM acc_transaction
		WHERE COAID=? AND IsAppove=1 AND VDate < ?`, headcode, from).Scan(&openD, &openC)
	var opening float64
	if headType == "A" || headType == "E" {
		opening = openD - openC
	} else {
		opening = openC - openD
	}

	rows, err := s.dbFor(r).Query(`
		SELECT VDate, VNo, Vtype, Narration, Debit, Credit
		FROM acc_transaction
		WHERE COAID = ? AND IsAppove = 1 AND VDate BETWEEN ? AND ?
		ORDER BY VDate ASC, ID ASC`, headcode, from, to)
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
	balance := opening
	var totalD, totalC float64
	for rows.Next() {
		var d, vno, vtype, narr sql.NullString
		var dr, cr float64
		if err := rows.Scan(&d, &vno, &vtype, &narr, &dr, &cr); err != nil {
			continue
		}
		if headType == "A" || headType == "E" {
			balance += dr - cr
		} else {
			balance += cr - dr
		}
		list = append(list, lrow{d.String, vno.String, vtype.String, narr.String, dr, cr, balance})
		totalD += dr
		totalC += cr
	}
	writeJSON(w, map[string]interface{}{
		"headcode":     headcode,
		"opening":      money(opening),
		"data":         list,
		"total_debit":  money(totalD),
		"total_credit": money(totalC),
		"closing":      money(balance),
	})
}

// trialBalance: balances of every transactional head.
func (s *Server) trialBalance(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), "2000-01-01")
	to := orDefault(r.FormValue("to"), today())

	rows, err := s.dbFor(r).Query(`
		SELECT c.HeadCode, c.HeadName, c.HeadType,
			IFNULL(SUM(t.Debit),0) AS debit, IFNULL(SUM(t.Credit),0) AS credit
		FROM acc_coa c
		LEFT JOIN acc_transaction t ON t.COAID = c.HeadCode AND t.IsAppove = 1 AND t.VDate BETWEEN ? AND ?
		WHERE c.IsTransaction = 1 AND c.IsActive = 1
		GROUP BY c.HeadCode, c.HeadName, c.HeadType
		HAVING debit > 0 OR credit > 0
		ORDER BY c.HeadCode`, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type brow struct {
		HeadCode int64
		HeadName string
		Debit    float64
		Credit   float64
	}
	list := []brow{}
	var totalD, totalC float64
	for rows.Next() {
		var b brow
		var ht string
		if err := rows.Scan(&b.HeadCode, &b.HeadName, &ht, &b.Debit, &b.Credit); err != nil {
			continue
		}
		list = append(list, b)
		totalD += b.Debit
		totalC += b.Credit
	}
	s.render(w, r, "trial_balance.html", map[string]interface{}{
		"Title":       "Balance de Comprobación",
		"From":        from,
		"To":          to,
		"Rows":        list,
		"TotalDebit":  totalD,
		"TotalCredit": totalC,
		"Balanced":    totalD == totalC,
	})
}

// profitLoss: income vs expenses.
func (s *Server) profitLoss(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), today()[:4]+"-01-01")
	to := orDefault(r.FormValue("to"), today())

	type row struct {
		HeadCode int64
		HeadName string
		Amount   float64
	}
	income := []row{}
	expense := []row{}

	rows, err := s.dbFor(r).Query(`
		SELECT c.HeadCode, c.HeadName, IFNULL(SUM(t.Credit)-SUM(t.Debit),0) AS amount
		FROM acc_coa c
		JOIN acc_transaction t ON t.COAID = c.HeadCode AND t.IsAppove = 1 AND t.VDate BETWEEN ? AND ?
		WHERE c.HeadType = 'I' AND c.IsTransaction = 1
		GROUP BY c.HeadCode, c.HeadName ORDER BY c.HeadCode`, from, to)
	if err == nil {
		for rows.Next() {
			var rw row
			_ = rows.Scan(&rw.HeadCode, &rw.HeadName, &rw.Amount)
			income = append(income, rw)
		}
		rows.Close()
	}
	rows, err = s.dbFor(r).Query(`
		SELECT c.HeadCode, c.HeadName, IFNULL(SUM(t.Debit)-SUM(t.Credit),0) AS amount
		FROM acc_coa c
		JOIN acc_transaction t ON t.COAID = c.HeadCode AND t.IsAppove = 1 AND t.VDate BETWEEN ? AND ?
		WHERE c.HeadType = 'E' AND c.IsTransaction = 1
		GROUP BY c.HeadCode, c.HeadName ORDER BY c.HeadCode`, from, to)
	if err == nil {
		for rows.Next() {
			var rw row
			_ = rows.Scan(&rw.HeadCode, &rw.HeadName, &rw.Amount)
			expense = append(expense, rw)
		}
		rows.Close()
	}

	var totalIncome, totalExpense float64
	for _, i := range income {
		totalIncome += i.Amount
	}
	for _, e := range expense {
		totalExpense += e.Amount
	}

	s.render(w, r, "profit_loss.html", map[string]interface{}{
		"Title":        "Estado de Resultados",
		"From":         from,
		"To":           to,
		"Income":       income,
		"Expense":      expense,
		"TotalIncome":  totalIncome,
		"TotalExpense": totalExpense,
		"Profit":       totalIncome - totalExpense,
	})
}

// balanceSheet: assets vs liabilities+equity.
func (s *Server) balanceSheet(w http.ResponseWriter, r *http.Request) {
	asOf := orDefault(r.FormValue("as_of"), today())

	assets := []bsRow{}
	liabilities := []bsRow{}
	equity := []bsRow{}

	load := func(ht, sign string) []bsRow {
		out := []bsRow{}
		rows, err := s.dbFor(r).Query(`
			SELECT c.HeadCode, c.HeadName, IFNULL(SUM(t.`+sign+`),0) - IFNULL(SUM(t.`+map[bool]string{true: "Debit", false: "Credit"}[sign == "Debit"]+`),0) AS amount
			FROM acc_coa c
			JOIN acc_transaction t ON t.COAID = c.HeadCode AND t.IsAppove = 1 AND t.VDate <= ?
			WHERE c.HeadType = ? AND c.IsTransaction = 1
			GROUP BY c.HeadCode, c.HeadName ORDER BY c.HeadCode`, asOf, ht)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var rw bsRow
			_ = rows.Scan(&rw.HeadCode, &rw.HeadName, &rw.Amount)
			out = append(out, rw)
		}
		return out
	}
	assets = load("A", "Debit")
	liabilities = load("L", "Credit")
	equity = loadWithEquity(asOf, s.dbFor(r))

	var totalA, totalL, totalE float64
	for _, a := range assets {
		totalA += a.Amount
	}
	for _, l := range liabilities {
		totalL += l.Amount
	}
	for _, e := range equity {
		totalE += e.Amount
	}

	s.render(w, r, "balance_sheet.html", map[string]interface{}{
		"Title":       "Balance General",
		"AsOf":        asOf,
		"Assets":      assets,
		"Liabilities": liabilities,
		"Equity":      equity,
		"TotalAssets": totalA,
		"TotalLiabEq": totalL + totalE,
	})
}

// bsRow is a balance-sheet line.
type bsRow struct {
	HeadCode int64
	HeadName string
	Amount   float64
}

// loadWithEquity: equity + current-year profit folded into retained earnings.
func loadWithEquity(asOf string, db *sql.DB) []bsRow {
	out := []bsRow{}
	rows, err := db.Query(`
		SELECT c.HeadCode, c.HeadName, IFNULL(SUM(t.Credit)-SUM(t.Debit),0) AS amount
		FROM acc_coa c
		JOIN acc_transaction t ON t.COAID = c.HeadCode AND t.IsAppove = 1 AND t.VDate <= ?
		WHERE c.HeadType = 'L' AND c.IsTransaction = 1 AND c.HeadCode BETWEEN 201000000 AND 201999999
		GROUP BY c.HeadCode, c.HeadName ORDER BY c.HeadCode`, asOf)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rw bsRow
		_ = rows.Scan(&rw.HeadCode, &rw.HeadName, &rw.Amount)
		out = append(out, rw)
	}
	return out
}

// incomeStatement is an alias view of profit & loss.
func (s *Server) incomeStatement(w http.ResponseWriter, r *http.Request) {
	s.profitLoss(w, r)
}

// headsDropdown: transactional heads as JSON.
func (s *Server) headsDropdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.transactionalHeads(r))
}

// headsTransactions: recent transactions of a head (AJAX preview).
func (s *Server) headsTransactions(w http.ResponseWriter, r *http.Request) {
	headcode := r.FormValue("headcode")
	rows, err := s.dbFor(r).Query(`
		SELECT VDate, VNo, Vtype, Narration, Debit, Credit
		FROM acc_transaction WHERE COAID = ? AND IsAppove = 1
		ORDER BY ID DESC LIMIT 20`, headcode)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var d, vno, vtype, narr sql.NullString
		var dr, cr float64
		_ = rows.Scan(&d, &vno, &vtype, &narr, &dr, &cr)
		list = append(list, map[string]interface{}{
			"date": d.String, "vno": vno.String, "vtype": vtype.String,
			"narration": narr.String, "debit": dr, "credit": cr,
		})
	}
	writeJSON(w, list)
}
