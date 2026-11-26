package web

// consumo_interno.go — Consumo de suministros propios (papelería, limpieza).
//
// EL HUECO QUE CIERRA
//
// Con la separación de almacenes, la papelería entra al inventario de suministros
// cuando se compra. Pero un inventario de oficina que nunca baja no sirve: el
// gasto hay que reconocerlo cuando el producto se consume. Antes la única salida
// era un ajuste de inventario, que asienta contra el costo de ventas y ensucia el
// margen comercial.
//
// Aquí se registra el consumo: sale del almacén de consumo al costo promedio y se
// debita la cuenta de gasto elegida, acreditando el inventario de suministros.
// Nunca toca el costo de ventas ni la mercancía para la venta.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerConsumoInternoRoutes() {
	s.mux.HandleFunc("GET /consumption/list", s.consumoList)
	s.mux.HandleFunc("GET /consumption/data", s.consumoListData)
	s.mux.HandleFunc("GET /consumption/new", s.consumoForm)
	s.mux.HandleFunc("POST /consumption/save", s.consumoSave)
	s.mux.HandleFunc("GET /consumption/details/{no}", s.consumoDetails)
	s.mux.HandleFunc("POST /consumption/delete/{id}", s.consumoDelete)
}

func (s *Server) consumoList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "consumption_list.html", map[string]interface{}{"Title": "Consumo Interno"})
}

func (s *Server) consumoListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT c.id, c.consumption_no, c.date, c.warehouse_id, c.coa_id,
			IFNULL(c.concept,''), c.total, IFNULL(c.created_by,''), c.created_at,
			(SELECT COUNT(*) FROM internal_consumption_details d WHERE d.consumption_no = c.consumption_no) AS items
		FROM internal_consumption c
		ORDER BY c.id DESC`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var no, fecha, almacen, concepto, creadoPor, creado string
		var coa int64
		var total float64
		var items int
		if err := rows.Scan(&id, &no, &fecha, &almacen, &coa, &concepto, &total, &creadoPor, &creado, &items); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "consumption_no": no, "date": fecha, "warehouse": almacen,
			"account": s.nombreCuenta(r, coa), "concept": concepto,
			"total": money(total), "created_by": creadoPor, "created_at": creado,
			"items": items,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

func (s *Server) consumoForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":        "Nuevo Consumo Interno",
		"Almacen":      s.almacenPorDefecto(r, AlmacenTipoConsumo),
		"CuentasGasto": s.cuentasDeGasto(r),
		"Almacenes":    s.almacenesPorTipo(r, AlmacenTipoConsumo),
	}
	pre, err := s.predefineAccount(r)
	if err == nil {
		data["CuentaDefecto"] = pre.CuentaGastoSuministros()
	}
	s.render(w, r, "consumption_form.html", data)
}

// consumoSave registra la salida de suministros y su gasto.
func (s *Server) consumoSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión requerida", 401)
		return
	}
	createBy := sessUserID(sess)
	_ = r.ParseMultipartForm(32 << 20)

	productIDs := r.Form["product_id"]
	quantities := r.Form["quantity"]
	if len(productIDs) == 0 {
		jsonError(w, "Agregue al menos un producto de consumo interno", 400)
		return
	}

	concepto := strings.TrimSpace(r.FormValue("concept"))
	if concepto == "" {
		jsonError(w, "El concepto es obligatorio: para qué se consumió (papelería de oficina, limpieza del local, etc.)", 400)
		return
	}
	almacen := strings.TrimSpace(r.FormValue("warehouse_id"))
	if almacen == "" {
		almacen = s.almacenPorDefecto(r, AlmacenTipoConsumo)
	}
	if err := s.almacenValidoPara(r, almacen, AlmacenTipoConsumo); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	coa, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("coa_id")), 10, 64)
	if coa == 0 {
		coa = pre.CuentaGastoSuministros()
	}
	if coa == 0 {
		jsonError(w, "No hay cuenta de gasto por consumo de suministros asignada. Elíjala en el documento o asígnela en Configuración → Cuentas por defecto (papel «Gasto por consumo de suministros»).", 400)
		return
	}
	if err := s.cuentaDeGastoValida(r, coa); err != nil {
		jsonError(w, "Cuenta de gasto del consumo: "+err.Error(), 400)
		return
	}
	// El consumo acredita el inventario de suministros: si ese papel no está
	// atado al plan de la empresa, no se puede valorizar la salida.
	if pre.CuentaInventarioSuministros() == 0 {
		jsonError(w, "No hay cuenta de inventario de suministros asignada. Asígnela en Configuración → Cuentas por defecto (papel «Inventario de suministros y consumibles»).", 400)
		return
	}

	date := orDefault(r.FormValue("date"), today())

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var nextID int64
	if err := tx.QueryRow(`SELECT IFNULL(MAX(id),0)+1 FROM internal_consumption`).Scan(&nextID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	no := fmt.Sprintf("CONS-%d", nextID)

	type linea struct {
		ProductID string
		Qty       float64
		Costo     float64
		Total     float64
		Nombre    string
	}
	lineas := []linea{}
	var total float64
	for i, pid := range productIDs {
		pid = strings.TrimSpace(pid)
		qty := 0.0
		if i < len(quantities) {
			qty = parseMoney(quantities[i])
		}
		if pid == "" || qty <= 0 {
			continue
		}
		pidInt := idInternoProducto(tx, pid)
		var nombre, tipo sql.NullString
		if err := tx.QueryRow(`SELECT product_name, IFNULL(item_type,'venta') FROM product_information WHERE product_id = ?`, pid).
			Scan(&nombre, &tipo); err != nil {
			jsonError(w, fmt.Sprintf("El producto %s no existe", pid), 400)
			return
		}
		if normalizarTipoItem(tipo.String) != ItemConsumo {
			jsonError(w, fmt.Sprintf("%s no está marcado como consumo interno: cámbiele el tipo en el catálogo o regístrelo como compra/venta según corresponda", nombre.String), 400)
			return
		}
		// El costo es el promedio de compras: el gasto debe ser el costo real
		// del suministro, no el precio de venta ni un cero.
		costo := costoPromedioCompra(tx, pidInt)
		if costo <= 0 {
			costo = ultimoCostoCompra(tx, pidInt)
		}
		if costo <= 0 {
			jsonError(w, fmt.Sprintf("%s no tiene costo de compra registrado: no se puede valorizar el consumo", nombre.String), 400)
			return
		}
		disponible := stockPorMovimientos(tx, pidInt, almacen)
		if disponible < qty {
			jsonError(w, fmt.Sprintf("Stock insuficiente de %s en %s: hay %.2f y se piden %.2f", nombre.String, almacen, disponible, qty), 400)
			return
		}
		monto := round2(qty * costo)
		total += monto
		lineas = append(lineas, linea{ProductID: pid, Qty: qty, Costo: costo, Total: monto, Nombre: nombre.String})
	}
	if len(lineas) == 0 {
		jsonError(w, "No hay renglones con cantidad para consumir", 400)
		return
	}
	total = round2(total)

	if _, err := tx.Exec(`
		INSERT INTO internal_consumption (consumption_no, date, warehouse_id, coa_id, concept, total, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, no, date, almacen, coa, concepto, total, createBy, now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	for _, l := range lineas {
		if _, err := tx.Exec(`
			INSERT INTO internal_consumption_details (consumption_no, product_id, quantity, unit_cost, total)
			VALUES (?,?,?,?,?)`, no, l.ProductID, l.Qty, l.Costo, l.Total); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		pidInt := idInternoProducto(tx, l.ProductID)
		if pidInt == 0 {
			jsonError(w, fmt.Sprintf("el producto %q no existe en el catálogo", l.ProductID), 400)
			return
		}
		if err := insertStockMovement(tx, "consumo", pidInt, almacen, l.Qty, l.Costo,
			"consumption", int(nextID), no, date); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// Gasto del período contra inventario de suministros: el consumo propio no
	// es costo de ventas y no puede tocar la mercancía para la venta.
	vno := nextVoucherNoTx(tx, "JV")
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: no, VDate: date,
		COAID: coa, RevCodde: pre.CuentaInventarioSuministros(),
		Narration: "Internal supplies consumption", Comment: concepto,
		Debit: total,
	}, createBy); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if st := s.settingsFor(r); st.Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, createBy)
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Consumo registrado", "consumption_no": no,
		"redirect": saveRedirect(r, "/consumption/list"),
	})
}

func (s *Server) consumoDetails(w http.ResponseWriter, r *http.Request) {
	no := r.PathValue("no")
	var id int64
	var fecha, almacen, concepto, creadoPor string
	var coa int64
	var total float64
	err := s.dbFor(r).QueryRow(`
		SELECT id, date, warehouse_id, coa_id, IFNULL(concept,''), total, IFNULL(created_by,'')
		FROM internal_consumption WHERE consumption_no = ?`, no).
		Scan(&id, &fecha, &almacen, &coa, &concepto, &total, &creadoPor)
	if err != nil {
		http.Error(w, "Consumo no encontrado", 404)
		return
	}
	type fila struct {
		ProductID, Nombre, Unidad string
		Qty, Costo, Total         float64
	}
	filas := []fila{}
	rows, err := s.dbFor(r).Query(`
		SELECT d.product_id, IFNULL(p.product_name,''), IFNULL(p.unit,''), d.quantity, d.unit_cost, d.total
		FROM internal_consumption_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.consumption_no = ?`, no)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var f fila
			_ = rows.Scan(&f.ProductID, &f.Nombre, &f.Unidad, &f.Qty, &f.Costo, &f.Total)
			filas = append(filas, f)
		}
	}
	s.render(w, r, "consumption_details.html", map[string]interface{}{
		"Title": "Consumo Interno " + no, "No": no, "Date": fecha, "Warehouse": almacen,
		"Concept": concepto, "Total": total, "Cuenta": s.nombreCuenta(r, coa),
		"CreatedBy": creadoPor, "Rows": filas, "ID": id,
	})
}

// consumoDelete elimina un consumo y deshace su rastro.
//
// POR QUÉ SE BORRA EN VEZ DE "ANULAR": un consumo cargado por error —el doble,
// con la cantidad equivocada— tiene que devolver el suministro al almacén y
// quitar el gasto. Se borran sus movimientos, su asiento y sus renglones dentro
// de la misma transacción, igual que hace el ajuste de inventario. El consumo
// no está aprobado contra un período cerrado: si lo estuviera, el período lo
// rechaza al borrar el asiento (exigirPeriodoAbierto).
func (s *Server) consumoDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Consumo inválido", 400)
		return
	}
	var no string
	if err := s.dbFor(r).QueryRow(`SELECT consumption_no FROM internal_consumption WHERE id = ?`, id).Scan(&no); err != nil {
		jsonError(w, "Consumo no encontrado", 404)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for _, q := range []string{
		`DELETE FROM acc_transaction WHERE referenceNo = ?`,
		`DELETE FROM acc_vaucher WHERE referenceNo = ?`,
		`DELETE FROM internal_consumption_details WHERE consumption_no = ?`,
	} {
		if _, err := tx.Exec(q, no); err != nil {
			jsonError(w, fmt.Sprintf("limpiando el consumo: %v", err), 500)
			return
		}
	}
	// El movimiento se identifica por el id del consumo, no por su número.
	if _, err := tx.Exec(`DELETE FROM stock_movement WHERE reference_type = 'consumption' AND reference_id = ?`, id); err != nil {
		jsonError(w, fmt.Sprintf("devolviendo el stock del consumo: %v", err), 500)
		return
	}
	if _, err := tx.Exec(`DELETE FROM internal_consumption WHERE id = ?`, id); err != nil {
		jsonError(w, fmt.Sprintf("eliminando el consumo: %v", err), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Consumo eliminado y stock devuelto"})
}
