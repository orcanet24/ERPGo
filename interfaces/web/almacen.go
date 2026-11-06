package web

// almacen.go — Gestión de almacenes (multi-almacén real).
//
// POR QUÉ EXISTÍA A MEDIAS
//
// La tabla `warehouse` existía desde el principio y `stock_movement` guardaba
// `warehouse_id`, pero no había ninguna pantalla para administrarlos y el código
// escribía siempre "WH-MAIN" a mano. Resultado: el multi-almacén era decorativo.
//
// Aquí se administran de verdad, separados por tipo:
//
//   - venta:   mercancía para la venta (puede haber varias: una por local).
//   - consumo: suministros de uso propio (papelería, limpieza).
//
// El tipo del almacén es lo que impide que una compra de limpieza entre al
// almacén de venta. Un almacén con movimientos no se elimina: se desactiva, para
// que el histórico siga cuadrando.

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerAlmacenRoutes() {
	s.mux.HandleFunc("GET /warehouse/list", s.almacenList)
	s.mux.HandleFunc("GET /warehouse/data", s.almacenListData)
	s.mux.HandleFunc("GET /warehouse/new", s.almacenForm)
	s.mux.HandleFunc("GET /warehouse/edit/{id}", s.almacenForm)
	s.mux.HandleFunc("POST /warehouse/save", s.almacenSave)
	s.mux.HandleFunc("POST /warehouse/delete/{id}", s.almacenDelete)
}

func (s *Server) almacenList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "warehouse_list.html", map[string]interface{}{"Title": "Almacenes"})
}

func (s *Server) almacenListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`
		SELECT id, code, name, IFNULL(address,''), IFNULL(kind,'venta'),
			IFNULL(is_default,0), IFNULL(is_active,1)
		FROM warehouse
		ORDER BY IFNULL(kind,'venta'), IFNULL(is_default,0) DESC, name`)
	if err != nil {
		// Base anterior a la migración: sin tipo de almacén.
		rows, err = s.dbFor(r).Query(`
			SELECT id, code, name, IFNULL(address,''), 'venta', IFNULL(is_default,0), IFNULL(is_active,1)
			FROM warehouse ORDER BY IFNULL(is_default,0) DESC, name`)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var code, name, addr, kind string
		var def, act int
		if err := rows.Scan(&id, &code, &name, &addr, &kind, &def, &act); err != nil {
			continue
		}
		kind = normalizarTipoAlmacen(kind)
		list = append(list, map[string]interface{}{
			"id": id, "code": code, "name": name, "address": addr,
			"kind": kind, "kind_label": etiquetaTipoAlmacen(kind),
			"is_default": def == 1, "is_active": act == 1,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

func (s *Server) almacenForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Nuevo Almacén", "Kind": AlmacenTipoVenta}
	if id := r.PathValue("id"); id != "" {
		var a struct {
			ID                  int64
			Code, Name, Address string
			Kind                string
			IsDefault, IsActive int
		}
		err := s.dbFor(r).QueryRow(`
			SELECT id, code, name, IFNULL(address,''), IFNULL(kind,'venta'),
				IFNULL(is_default,0), IFNULL(is_active,1)
			FROM warehouse WHERE id = ?`, id).
			Scan(&a.ID, &a.Code, &a.Name, &a.Address, &a.Kind, &a.IsDefault, &a.IsActive)
		if err != nil {
			http.Error(w, "Almacén no encontrado", 404)
			return
		}
		data["Title"] = "Editar Almacén"
		data["A"] = a
		data["Kind"] = normalizarTipoAlmacen(a.Kind)
	}
	s.render(w, r, "warehouse_form.html", data)
}

func (s *Server) almacenSave(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		jsonError(w, "El código y el nombre son obligatorios", 400)
		return
	}
	if strings.ContainsAny(code, " \t/\\") {
		jsonError(w, "El código no puede llevar espacios ni barras", 400)
		return
	}
	kind := normalizarTipoAlmacen(r.FormValue("kind"))
	address := strings.TrimSpace(r.FormValue("address"))
	isDefault := 0
	if r.FormValue("is_default") == "1" {
		isDefault = 1
	}
	isActive := 1
	if r.FormValue("is_active") == "0" {
		isActive = 0
	}

	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	var existente int64
	_ = s.dbFor(r).QueryRow(`SELECT id FROM warehouse WHERE code = ?`, code).Scan(&existente)
	if existente != 0 && existente != id {
		jsonError(w, "Ya existe un almacén con ese código", 400)
		return
	}

	if id == 0 {
		res, err := s.dbFor(r).Exec(`
			INSERT INTO warehouse (code, name, address, is_default, is_active, kind, created_at)
			VALUES (?,?,?,?,?,?, CURRENT_TIMESTAMP)`, code, name, address, isDefault, isActive, kind)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		id, _ = res.LastInsertId()
	} else {
		if _, err := s.dbFor(r).Exec(`
			UPDATE warehouse SET code=?, name=?, address=?, is_default=?, is_active=?, kind=?
			WHERE id=?`, code, name, address, isDefault, isActive, kind, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// Un solo almacén por defecto POR TIPO: si este lo es, los demás del mismo
	// tipo dejan de serlo. Dos "por defecto" harían que el sistema eligiera uno
	// al azar según el orden de la consulta.
	if isDefault == 1 {
		if _, err := s.dbFor(r).Exec(`
			UPDATE warehouse SET is_default = 0
			WHERE id <> ? AND IFNULL(kind,'venta') = ?`, id, kind); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Almacén guardado", "id": id,
		"redirect": saveRedirect(r, "/warehouse/list"),
	})
}

func (s *Server) almacenDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var code, kind string
	err := s.dbFor(r).QueryRow(`SELECT code, IFNULL(kind,'venta') FROM warehouse WHERE id = ?`, id).Scan(&code, &kind)
	if err != nil {
		jsonError(w, "Almacén no encontrado", 404)
		return
	}
	kind = normalizarTipoAlmacen(kind)
	if code == AlmacenVentaPorDefecto || code == AlmacenConsumoPorDefecto {
		jsonError(w, "Los almacenes principales del sistema no se eliminan: puede desactivarlos", 400)
		return
	}
	var conMovimientos int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM stock_movement WHERE warehouse_id = ?`, code).Scan(&conMovimientos)
	if conMovimientos > 0 {
		jsonError(w, "No se puede eliminar: el almacén tiene movimientos. Desactívelo para conservar el histórico", 400)
		return
	}
	var activos int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM warehouse WHERE IFNULL(kind,'venta') = ? AND IFNULL(is_active,1) = 1`, kind).Scan(&activos)
	if activos <= 1 {
		jsonError(w, "No se puede eliminar el único almacén activo de su tipo", 400)
		return
	}
	if _, err := s.dbFor(r).Exec(`DELETE FROM warehouse WHERE id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Almacén eliminado", "redirect": saveRedirect(r, "/warehouse/list")})
}

// almacenesParaFormulario devuelve los almacenes en la forma que esperan las
// plantillas: separados por tipo, con su etiqueta y su marca de "por defecto".
func (s *Server) almacenesParaFormulario(r *http.Request) map[string]interface{} {
	venta := []map[string]interface{}{}
	consumo := []map[string]interface{}{}
	for _, a := range s.almacenesDe(r) {
		item := map[string]interface{}{
			"code": a.Code, "name": a.Name, "is_default": a.IsDefault,
			"kind_label": etiquetaTipoAlmacen(a.Kind),
		}
		if a.Kind == AlmacenTipoConsumo {
			consumo = append(consumo, item)
		} else {
			venta = append(venta, item)
		}
	}
	return map[string]interface{}{
		"AlmacenesVenta":    venta,
		"AlmacenesConsumo":  consumo,
		"AlmacenVentaDef":   s.almacenPorDefecto(r, AlmacenTipoVenta),
		"AlmacenConsumoDef": s.almacenPorDefecto(r, AlmacenTipoConsumo),
	}
}

// cuentasDeGasto devuelve las cuentas de gasto activas, para que el renglón de
// servicio o el consumo interno elijan dónde se asienta el gasto.
func (s *Server) cuentasDeGasto(r *http.Request) []map[string]interface{} {
	rows, err := s.dbFor(r).Query(`
		SELECT HeadCode, HeadName FROM acc_coa
		WHERE HeadType = 'E' AND IsActive = 1 AND IsTransaction = 1
		ORDER BY HeadName`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	lista := []map[string]interface{}{}
	for rows.Next() {
		var code int64
		var name string
		if err := rows.Scan(&code, &name); err != nil {
			continue
		}
		lista = append(lista, map[string]interface{}{"code": code, "name": name})
	}
	return lista
}

// nombreCuenta devuelve "código — nombre" de una cuenta, para las pantallas.
func (s *Server) nombreCuenta(r *http.Request, coa int64) string {
	var name sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT HeadName FROM acc_coa WHERE HeadCode = ?`, coa).Scan(&name)
	if !name.Valid {
		return ""
	}
	return strconv.FormatInt(coa, 10) + " — " + name.String
}
