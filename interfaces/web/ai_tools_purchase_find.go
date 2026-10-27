package web

// ai_tools_purchase_find.go — La herramienta de lectura que deja al agente VER lo
// que ya está cargado antes de cargar otra vez.
//
// POR QUÉ ES LA MITAD DEL CANDADO. El índice único impide el duplicado; esta
// herramienta es la que evita que el agente lo INTENTE. Un agente que recibe un
// listado de compras y no puede consultar qué compras existen ya solo tiene dos
// caminos: cargar todo y chocar con el candado una por una, o adivinar. Con
// purchase.find consulta por proveedor, número de factura, número de control o
// rango de fechas y ve, para cada compra, su tipo de documento y sus saldos —que es
// la misma información que ve el operador en Gestionar Compras.

import (
	"database/sql"
	"fmt"
	"strings"
)

func (s *Server) aiToolPurchaseFind(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)

	limit := int(aiNumParam(args, "limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	where := []string{}
	params := []interface{}{}

	if sid := int64(aiNumParam(args, "supplier_id")); sid > 0 {
		where = append(where, "p.supplier_id = ?")
		params = append(params, sid)
	}
	if nombre := strings.TrimSpace(aiStrParam(args, "supplier_name")); nombre != "" {
		where = append(where, "s.supplier_name LIKE ?")
		params = append(params, "%"+nombre+"%")
	}
	if chalan := strings.TrimSpace(aiStrParam(args, "chalan_no")); chalan != "" {
		where = append(where, "REPLACE(REPLACE(UPPER(IFNULL(p.chalan_no,'')),'-',''),' ','') = ?")
		params = append(params, normalizaDoc(chalan))
	}
	if control := strings.TrimSpace(aiStrParam(args, "control_no")); control != "" {
		where = append(where, "REPLACE(REPLACE(UPPER(IFNULL(p.control_no,'')),'-',''),' ','') = ?")
		params = append(params, normalizaDoc(control))
	}
	desde := strings.TrimSpace(aiStrParam(args, "date_from"))
	hasta := strings.TrimSpace(aiStrParam(args, "date_to"))
	if aiValidDate(desde) {
		where = append(where, "p.purchase_date >= ?")
		params = append(params, desde)
	}
	if aiValidDate(hasta) {
		where = append(where, "p.purchase_date <= ?")
		params = append(params, hasta)
	}

	condicion := ""
	if len(where) > 0 {
		condicion = " WHERE " + strings.Join(where, " AND ")
	}

	rows, err := db.Query(`SELECT p.purchase_id, IFNULL(CAST(p.purchase_date AS CHAR),''),
		p.supplier_id, IFNULL(s.supplier_name,''), IFNULL(p.chalan_no,''), IFNULL(p.control_no,''),
		IFNULL(p.document_kind,'factura'), p.grand_total_amount, p.paid_amount, p.due_amount
		FROM product_purchase p
		LEFT JOIN supplier_information s ON s.supplier_id = p.supplier_id`+condicion+`
		ORDER BY p.id DESC LIMIT `+fmt.Sprint(limit), params...)
	if err != nil {
		return string(mustJSON(aiToolError{"no se pudo consultar las compras: " + err.Error()})), nil
	}
	defer rows.Close()

	compras := []map[string]interface{}{}
	for rows.Next() {
		var pid, sid int64
		var fecha, prov, chalan, control, kind sql.NullString
		var total, pagado, debe float64
		if err := rows.Scan(&pid, &fecha, &sid, &prov, &chalan, &control, &kind, &total, &pagado, &debe); err != nil {
			continue
		}
		compras = append(compras, map[string]interface{}{
			"purchase_id": pid, "date": fecha.String, "supplier_id": sid, "supplier": prov.String,
			"chalan_no": chalan.String, "control_no": control.String,
			"document_kind": CompraKind(kind.String), "kind_label": EtiquetaCompra(kind.String),
			"total": money(total), "paid": money(pagado), "due": money(debe),
		})
	}

	resp := map[string]interface{}{
		"status":  "ok",
		"count":   len(compras),
		"compras": compras,
		"nota": "Esta es la lista de lo que YA está registrado. Si la compra que va a cargar aparece aquí, NO la cargue otra vez: " +
			"el sistema la rechazará. Mismo proveedor, mismos renglones y los mismos números de factura y control son la misma compra; " +
			"con otro número de factura u orden es otra compra distinta.",
	}
	if len(compras) == 0 {
		resp["note"] = "No hay compras registradas con esos datos: no debería haber duplicado al cargarla, si la clasifica igual."
	}
	_ = agent
	return string(mustJSON(resp)), nil
}
