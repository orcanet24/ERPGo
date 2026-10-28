package web

// cartera_cobros.go — Cuentas por cobrar: el saldo por cliente y el cobro
// repartido entre facturas.
//
// LA IDEA QUE ORDENA ESTE ARCHIVO: una factura no se cobra sola.
//
// Un cliente paga 1.000 y esa plata puede cubrir completa la factura 12 y en
// parte la 15. El registro viejo ataba cada cobro a UNA factura, así que ese pago
// había que partirlo en dos cobros con dos asientos, y la relación de aportes
// —cuánto de este pago fue a qué factura— se perdía. Aquí un cobro es un hecho con
// su monto, su fecha y su referencia, y el reparto por factura queda escrito fila
// por fila en `debt_collection`: es lo que permite responder «esta factura se pagó
// con estos tres pagos» y «este pago se fue en estas dos facturas».
//
// LO QUE SE COMPRUEBA ANTES DE ESCRIBIR, porque un cobro mal repartido descuadra
// la cartera Y el asiento: que cada factura sea del cliente que paga, que no se le
// aplique más de lo que debe, y que el reparto sume exactamente lo cobrado.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// carteraPorCobrar es el panel del módulo: saldo por cliente, facturas abiertas y
// últimos cobros, en una sola respuesta.
func (s *Server) carteraPorCobrar(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "customer_ledger") {
		return
	}
	db := s.dbFor(r)

	clientes := []map[string]interface{}{}
	totalFacturado, totalCobrado, totalPorCobrar := 0.0, 0.0, 0.0
	rows, err := db.Query(`SELECT c.customer_id, IFNULL(c.customer_name,''), COUNT(i.id),
		IFNULL(SUM(i.total_amount),0), IFNULL(SUM(i.paid_amount),0), IFNULL(SUM(i.due_amount),0)
		FROM invoice i
		JOIN customer_information c ON c.customer_id = i.customer_id
		GROUP BY c.customer_id, c.customer_name
		ORDER BY IFNULL(SUM(i.due_amount),0) DESC, c.customer_name ASC
		LIMIT 500`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var id int64
		var nombre sql.NullString
		var nfac int
		var facturado, cobrado, debe float64
		if err := rows.Scan(&id, &nombre, &nfac, &facturado, &cobrado, &debe); err != nil {
			continue
		}
		clientes = append(clientes, map[string]interface{}{
			"customer_id": id, "customer": nombre.String, "invoices": nfac,
			"invoiced": money(facturado), "collected": money(cobrado), "due": money(debe),
			"has_due": debe > 0.005,
		})
		totalFacturado += facturado
		totalCobrado += cobrado
		totalPorCobrar += debe
	}
	rows.Close()

	facturas := []map[string]interface{}{}
	rows2, err := db.Query(`SELECT i.invoice_id, IFNULL(i.invoice,0), IFNULL(CAST(i.date AS CHAR),''),
		i.total_amount, i.paid_amount, i.due_amount, i.customer_id, IFNULL(c.customer_name,'')
		FROM invoice i
		LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.due_amount > 0
		ORDER BY i.date ASC, i.id ASC LIMIT 500`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows2.Next() {
		var invID, invNo, custID int64
		var fecha, nombre sql.NullString
		var total, pagado, debe float64
		if err := rows2.Scan(&invID, &invNo, &fecha, &total, &pagado, &debe, &custID, &nombre); err != nil {
			continue
		}
		facturas = append(facturas, map[string]interface{}{
			"invoice_id": invID, "invoice_no": invNo, "date": fecha.String,
			"customer_id": custID, "customer": nombre.String,
			"total": money(total), "paid": money(pagado), "due": money(debe),
		})
	}
	rows2.Close()

	cobros := []map[string]interface{}{}
	rows3, err := db.Query(`SELECT dc.id, dc.customer_id, IFNULL(c.customer_name,''), dc.invoice_id, dc.amount,
		IFNULL(pm.name,''), IFNULL(CAST(dc.collection_date AS CHAR),''), IFNULL(dc.reference,''), IFNULL(dc.created_by,'')
		FROM debt_collection dc
		LEFT JOIN customer_information c ON c.customer_id = dc.customer_id
		LEFT JOIN payment_method pm ON pm.id = dc.payment_method_id
		ORDER BY dc.id DESC LIMIT 50`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows3.Next() {
		var id, custID, invID int64
		var nombre, metodo, fecha, ref, quien sql.NullString
		var monto float64
		if err := rows3.Scan(&id, &custID, &nombre, &invID, &monto, &metodo, &fecha, &ref, &quien); err != nil {
			continue
		}
		cobros = append(cobros, map[string]interface{}{
			"id": id, "customer_id": custID, "customer": nombre.String, "invoice_id": invID,
			"amount": money(monto), "method": metodo.String, "date": fecha.String,
			"reference": ref.String, "created_by": quien.String,
		})
	}
	rows3.Close()

	writeJSON(w, map[string]interface{}{
		"status":   1,
		"clientes": clientes,
		"facturas": facturas,
		"cobros":   cobros,
		"resumen": map[string]interface{}{
			"facturado":      money(totalFacturado),
			"cobrado":        money(totalCobrado),
			"por_cobrar":     money(totalPorCobrar),
			"clientes_deben": len(clientes),
		},
		"nota": "Este módulo es la cartera, no la facturación: aquí vive el saldo por cliente y el reparto de cada cobro entre facturas. " +
			"Las notas de entrega y los pedidos que hace Facturación comprometen inventario y no recargan valor fiscal, así que no aparecen como deuda.",
	})
}

// carteraFacturasCliente devuelve las facturas con saldo de UN cliente, para el
// reparto del cobro. Los montos van como número además de formateados: el navegador
// tiene que poder sumar el reparto y avisar si no cuadra, sin re-parsear un texto.
func (s *Server) carteraFacturasCliente(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "customer_ledger") {
		return
	}
	cid, _ := strconv.ParseInt(r.PathValue("customer_id"), 10, 64)
	if cid <= 0 {
		jsonError(w, "Cliente inválido", http.StatusBadRequest)
		return
	}
	rows, err := s.dbFor(r).Query(`SELECT invoice_id, IFNULL(invoice,0), IFNULL(CAST(date AS CHAR),''),
		total_amount, paid_amount, due_amount
		FROM invoice WHERE customer_id = ? AND due_amount > 0
		ORDER BY date ASC, id ASC LIMIT 300`, cid)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []map[string]interface{}{}
	for rows.Next() {
		var invID, invNo int64
		var fecha sql.NullString
		var total, pagado, debe float64
		if err := rows.Scan(&invID, &invNo, &fecha, &total, &pagado, &debe); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"invoice_id": invID, "invoice_no": invNo, "date": fecha.String,
			"total": money(total), "paid": money(pagado),
			"due": debe, "due_fmt": money(debe),
		})
	}

	// El documento fiscal digital de CADA factura pendiente, en lote: es lo que
	// el cobrador quiere mandar al cliente sin salir del modal. Mismo camino y
	// mismas reglas que la lista de facturación (ver digitalinvoice_doc.go).
	numeros := make([]int64, 0, len(out))
	for _, f := range out {
		if n, ok := f["invoice_no"].(int64); ok {
			numeros = append(numeros, n)
		}
	}
	docs := s.diDocsDeFacturasDeLaPetición(r, numeros)
	for _, f := range out {
		n, _ := f["invoice_no"].(int64)
		if d, ok := docs[n]; ok {
			pk, _ := f["invoice_id"].(int64)
			f["doc"] = s.diJSONDoc(pk, d)
		}
	}

	writeJSON(w, map[string]interface{}{"status": 1, "customer_id": cid, "invoices": out})
}

// carteraCobrar registra UN cobro repartido entre una o varias facturas.
func (s *Server) carteraCobrar(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "create", "customer_ledger") {
		return
	}
	r.ParseMultipartForm(32 << 20)

	customerID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("customer_id")), 10, 64)
	pmID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("payment_method_id")), 10, 64)
	monto := parseMoney(r.FormValue("amount"))
	ref := strings.TrimSpace(r.FormValue("reference"))
	notas := strings.TrimSpace(r.FormValue("notes"))
	fecha := orDefault(r.FormValue("collection_date"), today())

	if customerID <= 0 || monto <= 0 {
		jsonError(w, "Cliente y monto son obligatorios (el monto tiene que ser mayor que cero).", http.StatusBadRequest)
		return
	}

	// El reparto: apply_invoice[] y apply_amount[] en paralelo, como los manda el
	// formulario. Un renglón sin monto o en cero no es una aplicación.
	type aplicacion struct {
		invoice  int64
		monto    float64
		pagado   float64
		debe     float64
		nuevoSal float64
	}
	invIDs := r.Form["apply_invoice"]
	invAmts := r.Form["apply_amount"]
	aplicaciones := []aplicacion{}
	for i, raw := range invIDs {
		invID, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if invID <= 0 {
			continue
		}
		var m float64
		if i < len(invAmts) {
			m = parseMoney(invAmts[i])
		}
		if m <= 0 {
			continue
		}
		aplicaciones = append(aplicaciones, aplicacion{invoice: invID, monto: round2(m)})
	}
	if len(aplicaciones) == 0 {
		// Cobro contra UNA factura, sin reparto: es la forma de siempre, y sigue
		// funcionando para quien cobra una factura y nada más.
		unica, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("invoice_id")), 10, 64)
		if unica <= 0 {
			jsonError(w, "Diga a qué factura se aplica el cobro. Sin factura el dinero entra pero la deuda del cliente no baja, y la cartera queda mintiendo.", http.StatusBadRequest)
			return
		}
		aplicaciones = append(aplicaciones, aplicacion{invoice: unica, monto: round2(monto)})
	}

	suma := 0.0
	for _, a := range aplicaciones {
		suma += a.monto
	}
	suma = round2(suma)
	if dif := round2(suma - monto); dif > 0.01 || dif < -0.01 {
		jsonError(w, fmt.Sprintf("El reparto por factura suma %.2f y el cobro es de %.2f. Ajústelo antes de registrar: un cobro que no cuadra descuadra la cartera y el asiento.", suma, monto), http.StatusBadRequest)
		return
	}

	sess := s.sessionFromRequest(r)
	createBy := sessUserID(sess)

	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	paymentCOA := pre.CashCode
	if pmID > 0 {
		var coa sql.NullInt64
		_ = s.dbFor(r).QueryRow(`SELECT coa_id FROM payment_method WHERE id = ? AND is_active = 1`, pmID).Scan(&coa)
		if coa.Valid && coa.Int64 > 0 {
			paymentCOA = coa.Int64
		}
	}

	currencyCode := orDefault(r.FormValue("currency_code"), "VES")
	exchangeRate := 1.0
	amountUSD := 0.0
	if tasaVal, _, terr := s.ObtenerTasaHoy(s.dbFor(r), fecha, "USD"); terr == nil && tasaVal > 0 {
		exchangeRate = tasaVal
		if currencyCode == "USD" {
			amountUSD = monto
		} else {
			amountUSD = round2(monto / tasaVal)
		}
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// La tabla del cobro, por si esta instalación nunca cobró: el DDL de cartera.go
	// no traía las columnas de multi-moneda y el INSERT sí las usa.
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS debt_collection (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		customer_id INT NOT NULL,
		invoice_id INT DEFAULT 0,
		amount DECIMAL(12,2) NOT NULL DEFAULT 0,
		amount_usd DECIMAL(18,2) DEFAULT 0,
		exchange_rate DECIMAL(18,4) DEFAULT 1,
		currency_code VARCHAR(5) DEFAULT 'VES',
		payment_method_id INT DEFAULT 0,
		collection_date DATE,
		reference VARCHAR(255),
		notes TEXT,
		created_by VARCHAR(50),
		created_at DATETIME,
		INDEX idx_customer (customer_id),
		INDEX idx_invoice (invoice_id),
		INDEX idx_date (collection_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`); err != nil {
		jsonError(w, "debt_collection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for i := range aplicaciones {
		a := &aplicaciones[i]
		var interno int64
		// El filtro por customer_id es la comprobación de pertenencia: sin él, un
		// cobro podría aplicarse a la factura de OTRO cliente.
		if err := tx.QueryRow(`SELECT id, paid_amount, due_amount FROM invoice
			WHERE invoice_id = ? AND customer_id = ?`, a.invoice, customerID).
			Scan(&interno, &a.pagado, &a.debe); err != nil {
			jsonError(w, fmt.Sprintf("La factura #%d no existe o no es de ese cliente.", a.invoice), http.StatusBadRequest)
			return
		}
		if a.monto > round2(a.debe)+0.01 {
			jsonError(w, fmt.Sprintf("A la factura #%d solo le quedan %.2f por cobrar; no se le pueden aplicar %.2f.", a.invoice, a.debe, a.monto), http.StatusBadRequest)
			return
		}
		nuevoPagado := round2(a.pagado + a.monto)
		nuevoSaldo := round2(a.debe - a.monto)
		if nuevoSaldo < 0 {
			nuevoSaldo = 0
		}
		a.nuevoSal = nuevoSaldo
		if _, err := tx.Exec(`UPDATE invoice SET paid_amount = ?, due_amount = ? WHERE id = ?`,
			nuevoPagado, nuevoSaldo, interno); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := tx.Exec(`INSERT INTO debt_collection (customer_id, invoice_id, amount, amount_usd,
			exchange_rate, currency_code, payment_method_id, collection_date, reference, notes, created_by, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			customerID, a.invoice, a.monto, amountUSD, exchangeRate, currencyCode,
			pmID, fecha, ref, notas, createBy, now()); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// UN ASIENTO PARA UN COBRO, aunque cubra varias facturas: el dinero entró una
	// sola vez. Repartirlo en varios asientos por factura multiplicaba el trabajo
	// de cuadrar la caja contra el banco.
	sub := s.subcodeID(r, customerID, 3)
	vno := nextVoucherNoTx(tx, "CV")
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "CV", ReferenceNo: ref,
		VDate: fecha, COAID: paymentCOA, RevCodde: pre.CustomerCode,
		Narration: "Cobro a cliente " + strconv.FormatInt(customerID, 10),
		Comment:   notas, Debit: monto, SubType: 3, SubCode: sub,
	}, createBy); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg := "Cobro registrado. El asiento " + vno + " se aprueba desde Comprobantes."
	if st := s.settingsFor(r); st.Web.IsAutoapprove == 1 {
		if err := s.approveVoucher(r, vno, createBy); err != nil {
			msg = "Cobro registrado. El asiento " + vno + " quedó pendiente: " + err.Error()
		} else {
			msg = "Cobro registrado y asentado"
		}
	}

	detalle := []map[string]interface{}{}
	for _, a := range aplicaciones {
		detalle = append(detalle, map[string]interface{}{
			"invoice_id": a.invoice, "amount": money(a.monto), "due_after": money(a.nuevoSal),
		})
	}
	s.appendAudit(r, createBy, clientIP(r), "create", "Cartera", "debt_collection",
		strconv.FormatInt(customerID, 10), nil, map[string]interface{}{
			"amount": monto, "aplicaciones": detalle, "voucher": vno, "reference": ref,
		})

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": msg, "voucher": vno,
		"applied": len(aplicaciones), "applications": detalle,
	})
}
