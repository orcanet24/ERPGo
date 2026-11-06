package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- Inventory Adjustment module (replicates modules/store) ----------

func (s *Server) registerAdjustmentRoutes() {
	s.mux.HandleFunc("GET /adjustment/list", s.adjustmentList)
	s.mux.HandleFunc("/adjustment/list/data", s.adjustmentListData)
	s.mux.HandleFunc("GET /adjustment/new", s.adjustmentForm)
	s.mux.HandleFunc("POST /adjustment/save", s.adjustmentSave)
	s.mux.HandleFunc("GET /adjustment/details/{id}", s.adjustmentDetails)
	s.mux.HandleFunc("POST /adjustment/delete/{id}", s.adjustmentDelete)
}

func (s *Server) adjustmentList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "adjustment_list.html", map[string]interface{}{"Title": "Ajustes de Inventario"})
}

func (s *Server) adjustmentListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}
	search := strings.TrimSpace(r.FormValue("search[value]"))

	where := " WHERE a.status = 1 "
	args := []interface{}{}
	if search != "" {
		where += " AND (a.adjustment_no LIKE ? OR a.reference LIKE ? OR a.note LIKE ?) "
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM store_adjustment WHERE status=1").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM store_adjustment a"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT a.id, a.adjustment_no, a.date, a.warehouse_id, a.reference,
			a.note, a.created_by, a.created_at,
			(SELECT COUNT(*) FROM store_adjustment_details WHERE adjustment_no = a.adjustment_no) AS items
		FROM store_adjustment a
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
		var adjNo, date, warehouse, ref, note, createdBy, createdAt sql.NullString
		var items int
		if err := rows.Scan(&id, &adjNo, &date, &warehouse, &ref, &note, &createdBy, &createdAt, &items); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":            id,
			"adjustment_no": adjNo.String,
			"date":          date.String,
			"warehouse":     warehouse.String,
			"reference":     ref.String,
			"note":          note.String,
			"created_by":    createdBy.String,
			"created_at":    createdAt.String,
			"items":         items,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

func (s *Server) adjustmentForm(w http.ResponseWriter, r *http.Request) {
	type product struct {
		ID, Name, Model, Unit string
		Stock                 float64
	}
	products := []product{}
	// Un servicio no se ajusta: no tiene existencias.
	rows, _ := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(a.unit,''), IFNULL(a.item_type,'venta')
		FROM product_information a WHERE a.status = 1 AND IFNULL(a.item_type,'venta') <> 'servicio'
		ORDER BY a.product_name LIMIT 500`)
	if rows != nil {
		for rows.Next() {
			var p product
			var tipo string
			_ = rows.Scan(&p.ID, &p.Name, &p.Model, &p.Unit, &tipo)
			pidInt, _ := strconv.Atoi(p.ID)
			p.Stock = s.stockForProduct(r, pidInt)
			products = append(products, p)
		}
		rows.Close()
	}

	data := map[string]interface{}{
		"Title":    "Nuevo Ajuste de Inventario",
		"Products": products,
	}
	for k, v := range s.almacenesParaFormulario(r) {
		data[k] = v
	}
	s.render(w, r, "adjustment_form.html", data)
}

// adjustmentSave creates an inventory adjustment and adjusts stock.
//   - ADD: inserts a positive row in product_purchase_details (batch_id='ADJ-ADD-{n}')
//   - REMOVE: inserts a negative row in product_purchase_details (batch_id='ADJ-REM-{n}')
//     Also generates accounting vouchers (adjustment expense/income vs inventory).
func (s *Server) adjustmentSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	createBy := sessUserID(sess)

	r.ParseMultipartForm(32 << 20)

	productIDs := r.Form["product_id"]
	types := r.Form["adjust_type"]
	quantities := r.Form["quantity"]
	batchIDs := r.Form["batch_id"]
	expiryDates := r.Form["expiry_date"]
	reasons := r.Form["reason"]

	if len(productIDs) == 0 {
		jsonError(w, "Agregue al menos un producto", 400)
		return
	}

	reference := strings.TrimSpace(r.FormValue("reference"))
	note := strings.TrimSpace(r.FormValue("note"))
	warehouse := strings.TrimSpace(r.FormValue("warehouse_id"))
	date := orDefault(r.FormValue("adjustment_date"), today())

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Adjustment number: MAX(id)+1 formatted as ADJ-{n}
	var nextID int64
	if err := tx.QueryRow(`SELECT IFNULL(MAX(id),0)+1 FROM store_adjustment`).Scan(&nextID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	adjNo := fmt.Sprintf("ADJ-%d", nextID)

	// Insert adjustment header
	if _, err := tx.Exec(`
		INSERT INTO store_adjustment (adjustment_no, date, warehouse_id, reference, note, created_by, created_at, status)
		VALUES (?,?,?,?,?,?,?,1)`,
		adjNo, date, warehouse, reference, note, createBy, now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Process each line
	type adjLine struct {
		ProductID string
		Type      string // add or remove
		Quantity  float64
		Tipo      string // venta | consumo
		Costo     float64
		Almacen   string
		BatchID   string
		Reason    string
	}
	var lines []adjLine
	for i, pid := range productIDs {
		qty := 0.0
		adjType := "add"
		if i < len(quantities) {
			qty = parseMoney(quantities[i])
		}
		if i < len(types) {
			adjType = types[i]
		}
		if qty <= 0 {
			continue
		}
		batch := ""
		if i < len(batchIDs) {
			batch = batchIDs[i]
		}
		expiry := interface{}(nil)
		if i < len(expiryDates) && expiryDates[i] != "" {
			expiry = expiryDates[i]
		}
		reason := ""
		if i < len(reasons) {
			reason = reasons[i]
		}

		// Store adjustment detail
		if _, err := tx.Exec(`
			INSERT INTO store_adjustment_details (adjustment_no, product_id, batch_id, expiry_date, quantity, type)
			VALUES (?,?,?,?,?,?)`,
			adjNo, pid, batch, expiry, qty, adjType); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}

		// Un ajuste NO es una compra. Antes se escribía una fila en
		// `product_purchase_details` —con cantidad negativa cuando se restaba—
		// y eso contaminaba el costo promedio y el historial de compras. Ahora
		// se escribe en `stock_movement`, que es el libro real del inventario,
		// con el almacén y el costo del producto.
		pidInt, _ := strconv.Atoi(pid)
		tipo := s.tipoItemProducto(r, pidInt)
		if !itemMueveStock(tipo) {
			continue // un servicio no tiene existencias que ajustar
		}
		kind := tipoAlmacenDeItem(tipo)
		alm := warehouse
		if err := s.almacenValidoPara(r, alm, kind); err != nil {
			// Un ajuste puede mezclar mercancía y suministros: el almacén del
			// documento aplica al tipo que le corresponde y el otro renglón va
			// al almacén por defecto de su tipo, en vez de obligar a dos ajustes.
			alm = s.almacenPorDefecto(r, kind)
		}
		costo := costoPromedioCompra(tx, pidInt)
		if costo <= 0 {
			costo = ultimoCostoCompra(tx, pidInt)
		}
		mov := "adjustment_add"
		if adjType == "remove" {
			mov = "adjustment_remove"
		}
		if err := insertStockMovement(tx, mov, pidInt, alm, qty, costo, "adjustment", int(nextID), adjNo, date); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}

		lines = append(lines, adjLine{ProductID: pid, Type: adjType, Quantity: qty, Tipo: tipo,
			Costo: costo, Almacen: alm, BatchID: batch, Reason: reason})
	}

	// Accounting vouchers
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Los importes se valorizan al costo: antes se sumaba la CANTIDAD como si
	// fuera dinero, así que ajustar 3 unidades asentaba 3,00 bolívares.
	var prodAdd, prodRemove, consAdd, consRemove float64
	for _, l := range lines {
		monto := round2(l.Quantity * l.Costo)
		switch {
		case normalizarTipoItem(l.Tipo) == ItemConsumo && l.Type == "remove":
			consRemove += monto
		case normalizarTipoItem(l.Tipo) == ItemConsumo:
			consAdd += monto
		case l.Type == "remove":
			prodRemove += monto
		default:
			prodAdd += monto
		}
	}

	// Mercancía de venta: el sobrante vuelve del costo de ventas y el faltante
	// se carga al costo de ventas (es la merma del inventario comercial).
	if prodAdd > 0 {
		vno := nextVoucherNoTx(tx, "JV")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: adjNo, VDate: date,
			COAID: pre.InventoryCode, RevCodde: pre.COGSCode,
			Narration: "Inventory Adjustment (Add)", Comment: fmt.Sprintf("Stock increase %s", adjNo),
			Debit: prodAdd,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	if prodRemove > 0 {
		vno := nextVoucherNoTx(tx, "JV")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: adjNo, VDate: date,
			COAID: pre.COGSCode, RevCodde: pre.InventoryCode,
			Narration: "Inventory Adjustment (Remove)", Comment: fmt.Sprintf("Stock decrease %s", adjNo),
			Debit: prodRemove,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	// Consumo interno: su inventario es un activo aparte y su gasto no es costo
	// de ventas. Mezclarlos era justo lo que distorsionaba el margen.
	if consAdd > 0 {
		vno := nextVoucherNoTx(tx, "JV")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: adjNo, VDate: date,
			COAID: pre.CuentaInventarioSuministros(), RevCodde: pre.CuentaGastoSuministros(),
			Narration: "Supplies Adjustment (Add)", Comment: fmt.Sprintf("Supplies increase %s", adjNo),
			Debit: consAdd,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	if consRemove > 0 {
		vno := nextVoucherNoTx(tx, "JV")
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: adjNo, VDate: date,
			COAID: pre.CuentaGastoSuministros(), RevCodde: pre.CuentaInventarioSuministros(),
			Narration: "Supplies Adjustment (Remove)", Comment: fmt.Sprintf("Supplies decrease %s", adjNo),
			Debit: consRemove,
		}, createBy); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Autoapprove vouchers
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove == 1 {
		rows, _ := s.dbFor(r).Query(`SELECT VNo FROM acc_vaucher WHERE referenceNo = ? AND status = 0`, adjNo)
		var vnos []string
		if rows != nil {
			for rows.Next() {
				var v string
				_ = rows.Scan(&v)
				vnos = append(vnos, v)
			}
			rows.Close()
		}
		for _, v := range vnos {
			_ = s.approveVoucher(r, v, createBy)
		}
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Ajuste registrado", "adjustment_no": adjNo,
	})
}

func (s *Server) adjustmentDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var adjNo, date, warehouse, ref, note, createdBy, createdAt sql.NullString
	var status int
	err := s.dbFor(r).QueryRow(`SELECT adjustment_no, date, warehouse_id, reference, note, created_by, created_at, status
		FROM store_adjustment WHERE id = ?`, id).
		Scan(&adjNo, &date, &warehouse, &ref, &note, &createdBy, &createdAt, &status)
	if err != nil {
		http.Error(w, "Ajuste no encontrado", 404)
		return
	}

	type drow struct {
		ProductID, ProductName, Model, Unit, BatchID, Expiry, Type string
		Quantity                                                   float64
	}
	details := []drow{}
	rows, _ := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.product_model,''), IFNULL(p.unit,''),
			IFNULL(d.batch_id,''), IFNULL(CAST(d.expiry_date AS CHAR),''), d.type, d.quantity
		FROM store_adjustment_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.adjustment_no = ? ORDER BY d.id`, adjNo.String)
	if rows != nil {
		for rows.Next() {
			var d drow
			_ = rows.Scan(&d.ProductID, &d.ProductName, &d.Model, &d.Unit, &d.BatchID, &d.Expiry, &d.Type, &d.Quantity)
			details = append(details, d)
		}
		rows.Close()
	}

	st := s.settingsFor(r)
	s.render(w, r, "adjustment_details.html", map[string]interface{}{
		"Title":       "Ajuste #" + adjNo.String,
		"AdjNo":       adjNo.String,
		"Date":        date.String,
		"Warehouse":   warehouse.String,
		"Reference":   ref.String,
		"Note":        note.String,
		"CreatedBy":   createdBy.String,
		"CreatedAt":   createdAt.String,
		"Status":      status,
		"Details":     details,
		"Currency":    st.Web.Currency,
		"CompanyName": st.Company.CompanyName,
	})
}

func (s *Server) adjustmentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var adjNo sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT adjustment_no FROM store_adjustment WHERE id = ? AND status = 1`, id).Scan(&adjNo)
	if err != nil {
		jsonError(w, "Ajuste no encontrado", 404)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Remove stock adjustments. Los movimientos viven en stock_movement desde
	// que el stock se calcula de ahí: sin borrarlos, el ajuste desaparecería del
	// listado pero seguiría sumando (o restando) en el inventario.
	for _, q := range []string{
		`DELETE FROM acc_transaction WHERE referenceNo = ?`,
		`DELETE FROM acc_vaucher WHERE referenceNo = ?`,
		`DELETE FROM product_purchase_details WHERE purchase_id = ?`,
		`DELETE FROM store_adjustment_details WHERE adjustment_no = ?`,
	} {
		if _, err := tx.Exec(q, adjNo.String); err != nil {
			jsonError(w, fmt.Sprintf("limpiar ajuste: %v", err), 500)
			return
		}
	}
	// Los movimientos de stock se identifican por el id del ajuste, no por su
	// número: el número es texto y reference_id es numérico.
	if _, err := tx.Exec(`DELETE FROM stock_movement WHERE reference_type = 'adjustment' AND reference_id = ?`, id); err != nil {
		jsonError(w, fmt.Sprintf("limpiar movimientos del ajuste: %v", err), 500)
		return
	}

	// Soft delete header
	res, err := tx.Exec(`UPDATE store_adjustment SET status=0 WHERE id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Ajuste no encontrado", 404)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Ajuste eliminado", "redirect": saveRedirect(r, "/adjustment/list")})
}
