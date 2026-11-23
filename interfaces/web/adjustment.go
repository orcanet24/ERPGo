package web

import (
	"database/sql"
	"fmt"
	"log"
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
	// POST /adjustment/revert/{id} reemplaza a /adjustment/delete/{id}
	// (D-4, auditoría atómica 2026-11-22): el borrado físico destruía el
	// kardex y el mayor de un ajuste —aunque el período estuviera cerrado—
	// y mentía contra la política del propio README ("los registros
	// históricos nunca se modifican"). Un ajuste mal se CORRIGE con su
	// reverso, que deja las dos mitades visibles.
	s.mux.HandleFunc("POST /adjustment/revert/{id}", s.adjustmentRevert)
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

// ajusteRenglón es una línea de ajuste, compartida por la ALTA (que la saca
// del formulario) y la REVERSIÓN (que la saca del detalle original con el
// tipo invertido — D-4, auditoría atómica 2026-11-22—).
type ajusteRenglón struct {
	PID    string
	Type   string // add | remove
	Qty    float64
	Batch  string
	Expiry string
	Reason string
}

// ajustar escribe el ajuste —cabecera, detalles, kardex y asientos— dentro
// de la transacción que recibe el llamador. Alta y reversión comparten así
// LA MISMA regla de escritura: duplicar el bloque era la segunda copia que
// se queda atrás (patrón que el propio repo persigue desde la fase fiscal).
// Si un paso falla responde aquí y devuelve false; el defer del llamador
// deshace todo, y no queda ni kardex sin cabecera ni asiento sin kardex.
func (s *Server) ajustar(w http.ResponseWriter, r *http.Request, tx *sql.Tx, createBy string,
	nextID int64, adjNo, date, warehouse, reference, note string, renglones []ajusteRenglón) bool {

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

	// Insert adjustment header
	if _, err := tx.Exec(`
		INSERT INTO store_adjustment (adjustment_no, date, warehouse_id, reference, note, created_by, created_at, status)
		VALUES (?,?,?,?,?,?,?,1)`,
		adjNo, date, warehouse, reference, note, createBy, now()); err != nil {
		jsonError(w, err.Error(), 500)
		return false
	}

	for _, rg := range renglones {
		pid := rg.PID
		qty := rg.Qty
		adjType := rg.Type
		if adjType == "" {
			adjType = "add"
		}
		if qty <= 0 {
			continue
		}
		batch := rg.Batch
		expiry := interface{}(nil)
		if rg.Expiry != "" {
			expiry = rg.Expiry
		}
		reason := rg.Reason

		// Store adjustment detail
		if _, err := tx.Exec(`
			INSERT INTO store_adjustment_details (adjustment_no, product_id, batch_id, expiry_date, quantity, type)
			VALUES (?,?,?,?,?,?)`,
			adjNo, pid, batch, expiry, qty, adjType); err != nil {
			jsonError(w, err.Error(), 500)
			return false
		}

		// Un ajuste NO es una compra. Antes se escribía una fila en
		// `product_purchase_details` —con cantidad negativa cuando se restaba—
		// y eso contaminaba el costo promedio y el historial de compras. Ahora
		// se escribe en `stock_movement`, que es el libro real del inventario,
		// con el almacén y el costo del producto.
		//
		// El identificador del formulario puede ser el código (varchar) o el
		// id interno en texto: stock_movement.product_id es ENTERO, así que
		// el viejo `strconv.Atoi(pid)` mandaba los códigos alfanuméricos al
		// producto 0 —el ajuste se registraba "bien" y el stock no se movía
		// (lo destapó la prueba de reversión D-4)—. Se resuelve el id real;
		// sin resolución no hay kardex posible y se dice, no se finge.
		pidInt := idInternoProducto(tx, pid)
		if pidInt == 0 {
			jsonError(w, fmt.Sprintf("el producto %q no existe en el catálogo: el ajuste no puede tocar el inventario", pid), http.StatusBadRequest)
			return false
		}
		var tipoRaw sql.NullString
		_ = tx.QueryRow(`SELECT IFNULL(NULLIF(item_type,''), 'venta') FROM product_information WHERE id = ?`, pidInt).Scan(&tipoRaw)
		tipo := normalizarTipoItem(tipoRaw.String)
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
			return false
		}

		lines = append(lines, adjLine{ProductID: pid, Type: adjType, Quantity: qty, Tipo: tipo,
			Costo: costo, Almacen: alm, BatchID: batch, Reason: reason})
	}

	// Accounting vouchers
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return false
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

	// Cuentas del asiento: salen del mapa de papeles de la empresa. Un ajuste no
	// puede asentarse contra una cuenta que no esté asignada en su plan.
	type cuentaRequerida struct {
		Codigo int64
		Papel  string
	}
	requeridas := []cuentaRequerida{}
	if prodAdd > 0 || prodRemove > 0 {
		requeridas = append(requeridas,
			cuentaRequerida{pre.CuentaInventarioMercancia(), "Inventario de mercancía para la venta"},
			cuentaRequerida{pre.COGSCode, "Costo de ventas"})
	}
	if consAdd > 0 || consRemove > 0 {
		requeridas = append(requeridas,
			cuentaRequerida{pre.CuentaInventarioSuministros(), "Inventario de suministros y consumibles"},
			cuentaRequerida{pre.CuentaGastoSuministros(), "Gasto por consumo de suministros"})
	}
	for _, c := range requeridas {
		if c.Codigo == 0 {
			jsonError(w, fmt.Sprintf("El ajuste necesita la cuenta «%s». Asígnela en Configuración → Cuentas por defecto.", c.Papel), 400)
			return false
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
			return false
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
			return false
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
			return false
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
			return false
		}
	}

	return true
}

// reservarAjusteNoTx reserva el correlativo de ajuste con el contador con
// bloqueo (F-3): MAX(id)+1 sin reserva dejaba a dos ajustes simultáneos con
// el mismo ADJ-, y el detalle huérfano se atribuía al primero. El suelo es
// el MAX(id) actual, para no volver a emitir nunca un número heredado.
func reservarAjusteNoTx(tx *sql.Tx) (int64, string, error) {
	var piso int64
	if err := tx.QueryRow(`SELECT IFNULL(MAX(id),0) FROM store_adjustment`).Scan(&piso); err != nil {
		return 0, "", err
	}
	valor, err := reservarConsecutivoTx(tx, "store_adjustment", piso)
	if err != nil {
		return 0, "", err
	}
	return valor, fmt.Sprintf("ADJ-%d", valor), nil
}

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

	renglones := make([]ajusteRenglón, 0, len(productIDs))
	for i, pid := range productIDs {
		rg := ajusteRenglón{PID: pid, Type: "add"}
		if i < len(quantities) {
			rg.Qty = parseMoney(quantities[i])
		}
		if i < len(types) && types[i] != "" {
			rg.Type = types[i]
		}
		if i < len(batchIDs) {
			rg.Batch = batchIDs[i]
		}
		if i < len(expiryDates) {
			rg.Expiry = expiryDates[i]
		}
		if i < len(reasons) {
			rg.Reason = reasons[i]
		}
		renglones = append(renglones, rg)
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

	nextID, adjNo, err := reservarAjusteNoTx(tx)
	if err != nil {
		jsonError(w, "no se pudo reservar el número de ajuste: "+err.Error(), 500)
		return
	}

	if !s.ajustar(w, r, tx, createBy, nextID, adjNo, date, warehouse, reference, note, renglones) {
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Autoapprove vouchers
	s.aprobarVouchersDeAjuste(r, adjNo, createBy)

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Ajuste registrado", "adjustment_no": adjNo,
	})
}

// adjustmentRevert corrige un ajuste con su REVERSO (D-4, auditoría
// atómica 2026-11-22). El /adjustment/delete que existía hacía DELETEs en
// acc_transaction, acc_vaucher, stock_movement y detalles —destruyendo
// histórico contable, aunque el período estuviera cerrado, y contradiciendo
// la política del README—; además arrastraba un DELETE muerto a
// product_purchase_details con un número de ajuste en la columna de compra.
// Ahora: el ajuste original queda intacto y se registra un ajuste nuevo,
// en la fecha de HOY (el mes cerrado no se toca), con los tipos invertidos
// y su asiento, enlazado por reference = REVERTIR:<adjNo original>, que es
// además la guarda anti-doble-reversión.
func (s *Server) adjustmentRevert(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	createBy := sessUserID(sess)

	id := r.PathValue("id")

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var adjOrig, warehouseOrig sql.NullString
	if err := tx.QueryRow(`SELECT adjustment_no, IFNULL(warehouse_id,'') FROM store_adjustment WHERE id = ? AND status = 1`, id).
		Scan(&adjOrig, &warehouseOrig); err != nil {
		jsonError(w, "Ajuste no encontrado (o ya está inactivo)", http.StatusNotFound)
		return
	}

	// Guarda anti-doble-reversión: si ya existe el reverso activo de este
	// ajuste, no se crea un segundo.
	var yaRevertido int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM store_adjustment WHERE status = 1 AND reference = CONCAT('REVERTIR:', ?)`,
		adjOrig.String).Scan(&yaRevertido); err != nil {
		jsonError(w, "no se pudo comprobar la reversión previa: "+err.Error(), 500)
		return
	}
	if yaRevertido > 0 {
		jsonError(w, "El ajuste "+adjOrig.String+" ya fue revertido", http.StatusConflict)
		return
	}

	rows, err := tx.Query(`SELECT product_id, IFNULL(batch_id,''), IFNULL(expiry_date,''), quantity, type
		FROM store_adjustment_details WHERE adjustment_no = ?`, adjOrig.String)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var renglones []ajusteRenglón
	for rows.Next() {
		var pid, batch, expiry, tipo string
		var qty float64
		if err := rows.Scan(&pid, &batch, &expiry, &qty, &tipo); err != nil {
			continue
		}
		invertido := "remove"
		if tipo == "remove" {
			invertido = "add"
		}
		renglones = append(renglones, ajusteRenglón{
			PID: pid, Type: invertido, Qty: qty, Batch: batch, Expiry: expiry,
			Reason: "Reversión de " + adjOrig.String,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		jsonError(w, "leyendo el detalle original: "+err.Error(), 500)
		return
	}
	if len(renglones) == 0 {
		jsonError(w, "El ajuste "+adjOrig.String+" no tiene renglones que revertir", http.StatusConflict)
		return
	}

	nextID, adjNo, err := reservarAjusteNoTx(tx)
	if err != nil {
		jsonError(w, "no se pudo reservar el número del reverso: "+err.Error(), 500)
		return
	}

	note := "Reversión del ajuste " + adjOrig.String
	if !s.ajustar(w, r, tx, createBy, nextID, adjNo, today(), warehouseOrig.String,
		"REVERTIR:"+adjOrig.String, note, renglones) {
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.aprobarVouchersDeAjuste(r, adjNo, createBy)

	// El original NO se marca inactivo: dos documentos visibles (ajuste y
	// reverso) es lo que deja el rastro contable íntegro; "ocultar" el
	// original era la mitad del borrado viejo que quedaba disfrazada.
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Ajuste revertido con " + adjNo, "adjustment_no": adjNo,
		"redirect": saveRedirect(r, "/adjustment/list"),
	})
}

// aprobarVouchersDeAjuste autoaprueba los vouchers recién creados cuando la
// empresa tiene autoaprobación. Compartido por alta y reversión; el error de
// cada aprobación se loguea (un asiento que no se aprobó es visible en el
// cierre como pendiente, no invisible).
func (s *Server) aprobarVouchersDeAjuste(r *http.Request, adjNo, createBy string) {
	st := s.settingsFor(r)
	if st.Web.IsAutoapprove != 1 {
		return
	}
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
		if err := s.approveVoucher(r, v, createBy); err != nil {
			log.Printf("ajuste %s: voucher %s no se pudo autoaprobar: %v", adjNo, v, err)
		}
	}
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
	// (ajustmentDelete fue eliminado de este archivo: ver adjustmentRevert y el
	// comentario de la ruta en registerAdjustmentRoutes — D-4, auditoría 2026-11-22.)
}
