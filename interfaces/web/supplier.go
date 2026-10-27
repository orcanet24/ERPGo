package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Supplier module (replicates modules/supplier) ----------

// supplierTotalesSQL: el debe y el haber del proveedor POR DOCUMENTOS.
//
// Compras (lo comprado), pagado (lo pagado al momento de la compra) y por pagar
// (lo que quedó a crédito) salen de `product_purchase`, que es donde vive el
// documento. Así el listado dice algo verdadero aunque el asiento esté pendiente
// de aprobar o el proveedor se haya cargado desde el PHP original sin cuenta
// contable propia. El alias `a` es supplier_information y el MISMO texto alimenta
// la prueba de base de datos: si alguien lo rompe, la prueba cae.
const supplierTotalesSQL = `
			IFNULL((SELECT SUM(p.grand_total_amount) FROM product_purchase p WHERE p.supplier_id = a.supplier_id),0),
			IFNULL((SELECT SUM(p.paid_amount) FROM product_purchase p WHERE p.supplier_id = a.supplier_id),0),
			IFNULL((SELECT SUM(p.due_amount) FROM product_purchase p WHERE p.supplier_id = a.supplier_id),0)`

// supplierSaldoSQL: el saldo CONTABLE del proveedor, el mismo número con el que
// cierra su libro mayor. Crédito menos débito porque su cuenta es un pasivo
// ('L' en acc_coa): lo que se le debe nace al crédito y los pagos lo bajan al
// débito. El alias `b` es su fila de acc_coa; sin cuenta, el saldo es 0.
const supplierSaldoSQL = `IFNULL((SELECT SUM(t.Credit)-SUM(t.Debit) FROM acc_transaction t WHERE t.COAID = b.HeadCode AND t.IsAppove=1),0)`

// supplierDocumentTotalsSQL: los tres totales del listado, para UN proveedor.
const supplierDocumentTotalsSQL = `SELECT IFNULL(SUM(grand_total_amount),0), IFNULL(SUM(paid_amount),0), IFNULL(SUM(due_amount),0)
		FROM product_purchase WHERE supplier_id = ?`

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
	s.mux.HandleFunc("POST /supplier/quick", s.supplierQuickCreate)
	s.mux.HandleFunc("GET /supplier/search", s.supplierSearch)
	s.mux.HandleFunc("GET /supplier/dropdown", s.supplierDropdown)
}

func (s *Server) supplierList(w http.ResponseWriter, r *http.Request) {
	// La lista la abre tanto quien gestiona proveedores como quien solo los da de
	// alta (necesita encontrar el que acaba de crear). El listado de datos devuelve
	// la deuda, así que va con el mismo permiso que la pantalla.
	if !s.exigirPermiso(w, r, "read", "manage_supplier", "add_supplier") {
		return
	}
	s.render(w, r, "supplier_list.html", map[string]interface{}{"Title": "Gestionar Proveedores"})
}

func (s *Server) supplierListData(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "manage_supplier", "add_supplier") {
		return
	}
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
			IFNULL(b.HeadCode,0),`+supplierTotalesSQL+`,
			`+supplierSaldoSQL+` AS balance
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
		var compras, pagado, porPagar, saldo float64
		if err := rows.Scan(&id, &name, &mobile, &address, &city, &hc,
			&compras, &pagado, &porPagar, &saldo); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":        id,
			"name":      name.String,
			"mobile":    mobile.String,
			"address":   address.String,
			"city":      city.String,
			"compras":   money(compras),
			"pagado":    money(pagado),
			"por_pagar": money(porPagar),
			"balance":   money(saldo),
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
	if !s.exigirPermiso(w, r, "read", "add_supplier", "manage_supplier") {
		return
	}
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
	// LA ACCIÓN ES LA DE VERDAD, no la del método HTTP. `/supplier/save` inserta o
	// edita según venga el id, y la matriz los separa («Añadir Proveedor» crear /
	// «Gestionar Proveedores» editar). Con la acción deducida del método, las dos
	// caían en «create» y un rol con permiso de edición podía además crear.
	if idStr == "" {
		if !s.exigirPermiso(w, r, "create", "add_supplier") {
			return
		}
	} else if !s.exigirPermiso(w, r, "update", "manage_supplier") {
		return
	}
	// El id se DEVUELVE en la respuesta: quien crea un proveedor desde el
	// formulario de compra —o la IA al cargar una compra de un proveedor
	// nuevo— necesita usarlo en la misma operación, y sin él habría que
	// adivinar cuál se acaba de crear.
	var supplierID int64

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
		supplierID, _ = res.LastInsertId()
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
		supplierID, _ = strconv.ParseInt(idStr, 10, 64)
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
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Guardado correctamente", "supplier_id": supplierID,
		"redirect": saveRedirect(r, "/supplier/list")})
}

func (s *Server) supplierDelete(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "delete", "manage_supplier") {
		return
	}
	id := r.PathValue("id")
	// LA REGLA DE NO BORRADO. Un proveedor con compras o con retenciones es
	// contraparte de documentos fiscales: el libro de compras y las retenciones
	// emitidas tienen que conservar su nombre y su RIF.
	if msg := s.guardarBorrado(r, id, "el proveedor", consejoProveedor, referenciasProveedor); msg != "" {
		jsonError(w, msg, 400)
		return
	}
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
	if !s.exigirPermiso(w, r, "read", "supplier_ledger") {
		return
	}
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
	if !s.exigirPermiso(w, r, "read", "supplier_ledger") {
		return
	}
	id := r.PathValue("id")
	supplierID, _ := strconv.ParseInt(id, 10, 64)
	var name, mobile, address sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT supplier_name, mobile, address FROM supplier_information WHERE supplier_id=?`, id).
		Scan(&name, &mobile, &address)
	if err != nil {
		http.Error(w, "Proveedor no encontrado", 404)
		return
	}
	// Resumen del documento, para que la pantalla diga de entrada cuánto se le
	// debe. Es el mismo número que ve el listado y no depende de que el asiento
	// esté aprobado: si la compra existe, la deuda se ve.
	docs := s.supplierDocumentTotals(r, supplierID)
	var headcode int64
	conCuenta := s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE supplier_id = ? ORDER BY HeadCode LIMIT 1`, supplierID).Scan(&headcode) == nil
	s.render(w, r, "supplier_ledger.html", map[string]interface{}{
		"Title":      "Libro Mayor: " + name.String,
		"SID":        id,
		"SName":      name.String,
		"SMobile":    mobile.String,
		"SAddr":      address.String,
		"Documentos": docs,
		"ConCuenta":  conCuenta,
	})
}

// supplierDocumentTotals devuelve compras, pagado y por pagar de un proveedor a
// partir de sus documentos.
func (s *Server) supplierDocumentTotals(r *http.Request, supplierID int64) map[string]interface{} {
	var compras, pagado, porPagar float64
	_ = s.dbFor(r).QueryRow(supplierDocumentTotalsSQL, supplierID).Scan(&compras, &pagado, &porPagar)
	return map[string]interface{}{
		"Compras":  money(compras),
		"Pagado":   money(pagado),
		"PorPagar": money(porPagar),
	}
}

// supplierLedgerRow es un movimiento del mayor del proveedor.
type supplierLedgerRow struct {
	Date      string  `json:"date"`
	VNo       string  `json:"vno"`
	VType     string  `json:"vtype"`
	Narration string  `json:"narration"`
	Debit     float64 `json:"debit"`
	Credit    float64 `json:"credit"`
	Balance   float64 `json:"balance"`
}

// consultadorFilas es el mínimo que necesita el mayor para consultar filas: sirve
// un *sql.DB y un *sql.Tx. Existe para que la prueba de base de datos ejecute LA
// CONSULTA REAL en vez de una copia que puede desviarse.
type consultadorFilas interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// supplierLedgerMovements lee los movimientos de la cuenta de un proveedor con su
// saldo corrido. Crédito menos débito: la cuenta del proveedor es un pasivo, así
// que la deuda nace al crédito y el pago la baja al débito.
//
// LA FECHA SE LEE COMO interface{} Y SE CONVIERTE CON fechaBD. `VDate` es DATE y
// el DSN va con parseTime=true: con MySQL vuelve como time.Time y escanearla a
// string falla con «unsupported Scan, storing driver.Value type time.Time into
// type *string». Ese error se descartaba con `continue`, así que se caían TODAS
// las filas y el libro decía «Sin movimientos» aunque hubiera asientos. Era el
// segundo motivo por el que este mayor parecía vacío.
func supplierLedgerMovements(q consultadorFilas, headcode int64, from, to string) ([]supplierLedgerRow, float64, float64, error) {
	query := `SELECT t.VDate, IFNULL(t.VNo,''), IFNULL(t.Vtype,''), IFNULL(t.Narration,''),
		t.Debit, t.Credit,
		IFNULL((SELECT SUM(x.Credit)-SUM(x.Debit) FROM acc_transaction x
			WHERE x.COAID = t.COAID AND x.IsAppove=1 AND (x.VDate < t.VDate OR (x.VDate = t.VDate AND x.ID <= t.ID))),0) AS balance
		FROM acc_transaction t
		WHERE t.COAID = ? AND t.IsAppove = 1`
	args := []interface{}{headcode}
	if from != "" {
		query += " AND t.VDate >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND t.VDate <= ?"
		args = append(args, to)
	}
	query += " ORDER BY t.VDate ASC, t.ID ASC"

	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	list := []supplierLedgerRow{}
	var totalD, totalC float64
	for rows.Next() {
		var fecha interface{}
		var vno, vtype, narr sql.NullString
		var dr, cr, bal float64
		if err := rows.Scan(&fecha, &vno, &vtype, &narr, &dr, &cr, &bal); err != nil {
			return nil, 0, 0, err
		}
		list = append(list, supplierLedgerRow{fechaBD(fecha), vno.String, vtype.String, narr.String, dr, cr, bal})
		totalD += dr
		totalC += cr
	}
	return list, totalD, totalC, rows.Err()
}

func (s *Server) supplierLedgerData(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_ledger") {
		return
	}
	id := r.PathValue("id")
	from := r.FormValue("from")
	to := r.FormValue("to")
	supplierID, _ := strconv.ParseInt(id, 10, 64)

	var headcode int64
	err := s.dbFor(r).QueryRow(`SELECT HeadCode FROM acc_coa WHERE supplier_id = ? ORDER BY HeadCode LIMIT 1`, supplierID).Scan(&headcode)
	if err == sql.ErrNoRows {
		// Un proveedor sin cuenta contable no es un error del usuario: se le
		// devuelve el mayor vacío y su deuda por documentos, en vez del 400 que
		// dejaba la pantalla sin nada. Pasa con los proveedores cargados desde el
		// PHP original, porque la cuenta la crea el alta hecha en Go.
		writeJSON(w, map[string]interface{}{
			"data": []supplierLedgerRow{}, "total_debit": "0.00", "total_credit": "0.00",
			"balance": "0.00", "sin_cuenta": true,
			"documentos": s.supplierDocumentTotals(r, supplierID),
		})
		return
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	list, totalD, totalC, err := supplierLedgerMovements(s.dbFor(r), headcode, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"data":         list,
		"cuenta":       headcode,
		"total_debit":  money(totalD),
		"total_credit": money(totalC),
		"balance":      money(totalC - totalD),
		"documentos":   s.supplierDocumentTotals(r, supplierID),
	})
}

// cuentaProveedorTx devuelve la cuenta contable PROPIA del proveedor y su código
// de detalle en acc_subcode, creando la cuenta si no la tiene.
//
// POR QUÉ LA CUENTA PROPIA Y NO LA GENERAL. El alta de proveedor crea una
// subcuenta bajo 21110 (Suppliers) y TANTO el libro mayor como la columna de
// saldo del listado leen ESA cuenta. Los documentos, en cambio, se asentaban
// contra la cuenta general de proveedores
// (acc_predefine_account.supplierCode) con el detalle en subType/subCode: cada
// proveedor tenía su cuenta en cero y su mayor decía «Sin movimientos» para
// siempre. Aquí se resuelve la cuenta propia en los caminos que ESCRIBEN; si
// falta —proveedores cargados desde el PHP original, o cuenta borrada a mano— se
// crea con el mismo código que usa el alta, para que sus documentos tengan dónde
// asentarse y su deuda se vea.
func (s *Server) cuentaProveedorTx(tx *sql.Tx, supplierID int64) (int64, sql.NullInt64, error) {
	var headcode int64
	err := tx.QueryRow(`SELECT HeadCode FROM acc_coa WHERE supplier_id = ? ORDER BY HeadCode LIMIT 1`, supplierID).Scan(&headcode)
	if err == nil {
		return headcode, s.subcodeProveedorTx(tx, supplierID), nil
	}
	if err != sql.ErrNoRows {
		return 0, sql.NullInt64{}, err
	}

	var name string
	if err := tx.QueryRow(`SELECT supplier_name FROM supplier_information WHERE supplier_id = ?`, supplierID).Scan(&name); err != nil {
		return 0, sql.NullInt64{}, err
	}
	hc, err := nextHeadCodeTx(tx, "21110", "2111000001")
	if err != nil {
		return 0, sql.NullInt64{}, err
	}
	if _, err := tx.Exec(`
		INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, HeadLevel, IsActive, IsTransaction, IsGL,
			HeadType, IsBudget, IsDepreciation, DepreciationRate, supplier_id, CreateBy, CreateDate, isSubType, subType)
		VALUES (?,?,?,?,1,1,0,'L',0,0,0,?,?,?,1,4)`,
		hc, fmt.Sprintf("%d-%s", supplierID, name), "Suppliers", 4, supplierID, "sistema", now()); err != nil {
		return 0, sql.NullInt64{}, err
	}
	return hc, s.subcodeProveedorTx(tx, supplierID), nil
}

// subcodeProveedorTx devuelve el id de acc_subcode del proveedor (subTypeId 4) y
// lo crea si falta: es el detalle con el que se marca cada asiento del proveedor
// y el que usa el alta para dejarlo creado.
func (s *Server) subcodeProveedorTx(tx *sql.Tx, supplierID int64) sql.NullInt64 {
	var id int64
	if err := tx.QueryRow(`SELECT id FROM acc_subcode WHERE referenceNo = ? AND subTypeId = 4 LIMIT 1`, supplierID).Scan(&id); err == nil {
		return sql.NullInt64{Int64: id, Valid: true}
	}
	var name string
	_ = tx.QueryRow(`SELECT supplier_name FROM supplier_information WHERE supplier_id = ?`, supplierID).Scan(&name)
	res, err := tx.Exec(`INSERT INTO acc_subcode (subTypeId, name, referenceNo, status, created_by, created_date, updated_by)
		VALUES (4, ?, ?, 1, ?, ?, ?)`, name, supplierID, "sistema", today(), "sistema")
	if err != nil {
		return sql.NullInt64{}
	}
	id, _ = res.LastInsertId()
	return sql.NullInt64{Int64: id, Valid: id > 0}
}

// supplierPreviousBalance replicates supplier previous_balance_add:
// debit supplier account, credit Cash In Hand (1020101).
func (s *Server) supplierPreviousBalance(w http.ResponseWriter, r *http.Request) {
	// Cargar un saldo anterior es una operación sobre la deuda del proveedor:
	// mismo permiso que registrar un pago.
	if !s.exigirPermiso(w, r, "create", "supplier_payment") {
		return
	}
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
	// EL SALDO ANTERIOR ES UNA DEUDA, Y UNA DEUDA VA AL CRÉDITO de la cuenta del
	// proveedor. Estaba asentado al débito (el mismo lado que un pago), así que
	// cargar lo que se le debía desde antes RESTABA en su libro.
	if _, err := tx.Exec(`INSERT INTO acc_transaction (VNo, Vtype, VDate, COAID, Narration, Debit, Credit, IsPosted, is_opening, CreateBy, CreateDate, IsAppove, vid, fyear, RevCodde)
		VALUES (?,?,?,?,?,0,?,1,1,?,?,1,0,0,0)`,
		tranID, "PR Balance", today(), headcode, "Supplier credit for previous balance "+name, balance, sessUserID(sess), now()); err != nil {
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
	// Lo usan los formularios de compra además de la propia pantalla de
	// proveedores: se admite cualquier permiso de LECTURA de esos módulos, porque
	// el buscador solo devuelve nombre, teléfono y dirección.
	if !s.exigirPermiso(w, r, "read", "manage_supplier", "add_supplier",
		"add_purchase", "manage_purchase", "supplier_ledger") {
		return
	}
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
	// Alimenta el select del formulario de compra, así que vale con permiso de
	// lectura de proveedores o de compras.
	if !s.exigirPermiso(w, r, "read", "manage_supplier", "add_supplier",
		"add_purchase", "manage_purchase") {
		return
	}
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
