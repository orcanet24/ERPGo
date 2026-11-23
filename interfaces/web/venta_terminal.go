package web

// venta_terminal.go — El UNICO camino de registro de una venta cobrada en caja
// terminal (sync HTTP de POSGo2 y, vía el registrar inyectado, el gRPC).
//
// POR QUÉ EXISTE (F-2, F-3, F-12, F-14 y D-6 de la auditoría atómica
// 2026-11-22). Antes, la venta sincronizada corría por
// internal/pos/sync_server.go: sin transacción (cabecera, renglones e
// inventario en ejecuciones sueltas — una falla a mitad dejaba una factura
// fiscal sin renglones—), numerada con `SELECT MAX(invoice)+1` (dos cajas al
// unísono podían quedarse con el mismo número; el mutex del SyncServer no
// protegía nada porque el servidor se construye por petición), sin ASIENTO
// CONTABLE (el libro mayor no veía las cajas), sin pagos (el arqueo central
// era imposible) y con el tipo de cambio fijado a ciegas en 1.0/'VES'.
//
// LA REGLA DE ESTE CAMINO: una venta offline YA se cobró; registrarla nunca
// se rechaza por un hueco de configuración de la empresa. Lo que falte para
// la contabilidad perfecta queda como AVISO visible en la respuesta, no como
// daño silencioso.
//
// EL CONTRATO CON EL TERMINAL (coordinado con la reparación de POSGo2, fase
// 4): payments=[{method, amount, reference}], currency, exchange_rate,
// igtf_amount, sale_local_id (UUID persistente por venta). Los nombres JSON
// son los que manda el terminal; cambiarlos rompe las cajas instaladas.

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"erpgo/internal/pos"
)

// registrarVentaTerminal inserta la venta del terminal en la base de SU
// empresa, atómicamente, con numeración central, asiento doble publicado,
// pagos, inventario e idempotencia por (terminal, sale_local_id).
func (s *Server) registrarVentaTerminal(db *sql.DB, req *pos.SyncSaleRequest) (*pos.SyncSaleResponse, error) {
	if db == nil {
		return nil, fmt.Errorf("sin base de empresa para la venta del terminal")
	}
	fecha := today()
	// Fecha del COBRO, no de la sincronización: el terminal manda su hora
	// local en created_at (RFC3339 con offset, acordado en su fase 4). Se
	// conserva el reloj de pared del ticket; si falta o no parsea, se usa el
	// hoy del servidor y la venta no se pierde por un dato cosmético.
	if raw := strings.TrimSpace(req.CreatedAt); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			fecha = t.Format("2006-01-02 15:04:05")
		} else if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
			fecha = t.Format("2006-01-02 15:04:05")
		}
	}

	// REFERENCIA DE CONTINGENCIA: la caja sin imprenta entregó un ticket con
	// numeración propia; el documento ERP es de contingencia (Providencia
	// SNAT/2024/000102 art. 16/30). Dos columnas conviven con papeles
	// distintos y ambos honestos: control_no guarda el NÚMERO DE CONTROL
	// FISCAL que vio el cliente (el del bloque impreso, o el central si no
	// lo había), y offline_invoice_no guarda el correlativo local del papel
	// (abajo, offlineNo). Mezclarlos fue la raíz de que «el ticket dice una
	// cosa y el ERP otra» (F-3).
	_, offlineNo := pos.ContingenciaRef(req.TerminalID, req.FiscalNumber, req.SaleLocalID)

	tx, err := abrirTxReal(db)
	if err != nil {
		return nil, fmt.Errorf("abrir transacción de venta: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. IDEMPOTENCIA dentro de la transacción. El libro pos_sale_sync tiene
	// UNIQUE(terminal_id, sale_local_id): se reclama primero (INSERT
	// IGNORE); si la fila ya existía, la venta ya corrió o está corriendo en
	// otro reintento y NO se duplica. Con sale_local_id = UUID persistente
	// del terminal (fase 4 POSGo2), esto sobrevive a restauraciones de la
	// base local — el autoincrement antiguo volvía a empezar y el ERP
	// respondía «already_exists» a una venta nunca vista (F-14)—.
	claim, err := reclamarVentaTerminal(tx, req)
	if err != nil {
		return nil, err
	}
	switch claim.modo {
	case reclamoYaHecho:
		return &pos.SyncSaleResponse{Success: true, RemoteID: strconv.FormatInt(claim.invoiceID, 10),
			Status: "already_exists", FiscalNumber: claim.fiscal}, nil
	case reclamoEnCurso:
		// Otra petición la está registrando AHORA (reclamo sin factura aún;
		// la tx de esa petición no ha hecho commit): el reintento de la cola
		// la recogerá convertida.
		return &pos.SyncSaleResponse{Success: false, Status: "in_progress",
			Error: "la venta está siendo procesada por otra sincronización"}, nil
	case reclamoSinClave:
		if claim.invoiceID > 0 {
			return &pos.SyncSaleResponse{Success: true, RemoteID: strconv.FormatInt(claim.invoiceID, 10),
				Status: "already_exists", FiscalNumber: claim.fiscal}, nil
		}
	}

	// 2. Cliente y productos (dentro de la misma tx: si un renglón falla, la
	// factura no queda huérfana).
	customerID, err := findOrCreateCustomerTx(tx, req.CustomerRIF, req.CustomerName)
	if err != nil {
		return nil, fmt.Errorf("cliente de la venta: %w", err)
	}

	// 3. Número de factura y control fiscal — con los MISMOS primitivos que
	// usa invoiceSave (F-3): el número del contador compartido
	// (reservarNumeroFacturaTx, con suelo MAX(invoice,999) para no reutilizar
	// nunca un número heredado) y el control de la serie FACT. Dos caminos
	// de numeración distintos para la misma columna era la receta del
	// duplicado: caja y panel pisándose el correlativo.
	numero, err := s.reservarNumeroFacturaTx(tx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo reservar el número de factura: %w", err)
	}
	invoiceNo := numero

	// CONTROL FISCAL: dos orígenes legítimos y registrar SIEMPRE el que vio
	// el cliente (F-3, auditoría):
	//   - venta offline con ticket: el número de control que la caja imprimió
	//     de su bloque. El ERP lo registra TAL CUAL (prefijo de serie + 8
	//     dígitos) y NO consume la serie central: consumir otro control
	//     además era dejar un hueco en la serie y un libro que no coincide
	//     con el papel del cliente.
	//   - sin control impreso (sincronización antigua o panel): se consume
	//     la serie FACT central, con bloqueo de fila, igual que invoiceSave.
	controlNo := ""
	if impreso := digitosDeNumero(req.FiscalNumber); impreso > 0 {
		controlNo = controlNoDesde(tx, impreso)
	} else {
		doc, err := s.nextFiscalDoc(tx, "FACT")
		if err != nil {
			return nil, fmt.Errorf("número de control fiscal: %w", err)
		}
		controlNo = doc.ControlNo
	}

	// 4. Moneda, tasa e IGTF: los manda el terminal; se adoptan con sentido
	// común (tasa <=0 → 1 con fuente marcada; moneda vacía → VES). Guardar
	// la tasa que la caja CONOCÍA es lo que hace auditable la venta en
	// divisas (F-12: antes se fijaba 1.0/'BCV' sin datos).
	moneda := strings.ToUpper(strings.TrimSpace(req.Currency))
	if moneda == "" {
		moneda = "VES"
	}
	tasa := req.ExchangeRate
	fuente := "TERMINAL"
	if tasa <= 0 {
		tasa = 1.0
		fuente = "SIN-TASA"
	}
	igtf := req.IgtfAmount
	if igtf < 0 {
		igtf = 0
	}
	baseUSD := round2((req.Total - igtf) / tasa)

	var avisos []string
	if fuente == "SIN-TASA" && req.Total > 0 && len(req.Payments) > 0 && hayPagosDivisa(req.Payments) {
		avisos = append(avisos, "la venta tiene pagos en divisa y el terminal no envió tasa BCV: se registró con tasa 1.0 —verifíquela en el listado de facturas—")
	}

	// 5. Cabecera.
	var invInternal int64
	res, err := tx.Exec(`
		INSERT INTO invoice (invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
			invoice, invoice_details, status, payment_type, control_no, customer_rif, series,
			total_vat_amnt, warehouse_id, is_online, sales_by, currency_code, exchange_rate, total_amount_usd, exchange_rate_source,
			igtf_amount, offline_invoice_no)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		invoiceNo, customerID, fecha, req.Total, req.Total, 0,
		invoiceNo, "Venta POS sincronizada (contingencia)", 1, 1, controlNo, req.CustomerRIF, "CONT",
		req.Tax, "WH-MAIN", 2, "POS-SYSTEM",
		moneda, tasa, baseUSD, fuente,
		igtf, offlineNo)
	if err != nil {
		return nil, fmt.Errorf("cabecera de factura: %w", err)
	}
	if id64, e := res.LastInsertId(); e == nil {
		invInternal = id64
	}

	// 6. Plan de cuentas y régimen fiscal DE LA EMPRESA DEL TERMINAL (la base
	// es la del API key; leer las cuentas del panel activo sería «asentar la
	// venta de B con las cuentas de A», el bug que predefineAccountTx ya
	// cerró para el otro camino).
	pre, err := predefineAccountTx(tx)
	if err != nil {
		return nil, fmt.Errorf("cuentas predefinidas de la empresa: %w", err)
	}
	fs := cargarAjustesFiscalesTx(tx)

	// 7. Renglones + inventario. El terminal ya descontó su stock local; el
	// ERP refleja la realidad aunque quede negativo (avisando): rechazar la
	// venta sincronizada sería perder dinero cobrado al cliente.
	ivaSuma := 0.0
	for _, item := range req.Items {
		productID := firstNonEmpty(item.ProductCode, item.SKU)
		if productID == "" {
			avisos = append(avisos, "un renglón llegó sin código de producto y se registró con código automático")
		}
		pid, err := ensureProductTx(tx, productID, item.Description, item.UnitPrice, item.TaxRate)
		if err != nil {
			return nil, fmt.Errorf("producto %q: %w", productID, err)
		}
		linea := round2(item.UnitPrice * item.Quantity)
		descuento := round2(linea * item.DiscountPct / 100)
		baseLinea := round2(linea - descuento)
		tasaIVA := item.TaxRate
		if tasaIVA <= 0 {
			// El producto existe con su tasa: esa manda (fs no define una
			// tasa IVA general; no se inventa).
			var t sql.NullFloat64
			_ = tx.QueryRow(`SELECT IFNULL(tax,0) FROM product_information WHERE product_id = ?`, pid).Scan(&t)
			tasaIVA = t.Float64
		}
		ivaLinea := round2(baseLinea * tasaIVA / 100)
		ivaSuma += ivaLinea

		if _, err := tx.Exec(`
			INSERT INTO invoice_details (invoice_details_id, invoice_id, product_id, description,
				quantity, rate, discount_pct, discount_amount, base_price,
				total_price, vat_amnt, tax, paid_amount, due_amount, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,1)`,
			fmt.Sprintf("POS-%d-%s", invoiceNo, pid), strconv.FormatInt(invoiceNo, 10), pid, item.Description,
			item.Quantity, item.UnitPrice, item.DiscountPct, descuento, baseLinea,
			baseLinea, ivaLinea, 0, req.Total); err != nil {
			return nil, fmt.Errorf("renglón %s: %w", pid, err)
		}

		// Inventario: mismo criterio que el panel —servicios y consumo no
		// descuentan—, leído del campo real item_type.
		//
		// EL KARDEX SE REGISTRA CON EL id INTERO de producto, no con el
		// código: stock_movement.product_id es int (la columna interna de
		// product_information), y tanto el panel como el checkout en línea
		// usan ese id. El sync viejo insertaba el código del terminal a
		// ciegas: funcionaba solo si el código era numérico, y con códigos
		// alfabéticos el kardex moría (o peor: quedaba en el producto 0).
		var tipoItem string
		_ = tx.QueryRow(`SELECT COALESCE(NULLIF(item_type,''), 'venta') FROM product_information WHERE product_id = ?`, pid).Scan(&tipoItem)
		if tipoItem != "servicio" && tipoItem != "consumo" {
			var pidInt int64
			if err := tx.QueryRow(`SELECT id FROM product_information WHERE product_id = ?`, pid).Scan(&pidInt); err != nil || pidInt == 0 {
				return nil, fmt.Errorf("el producto %s no tiene id interno para el kardex (id=%d, err=%v)", pid, pidInt, err)
			}
			if _, err := tx.Exec(`
				INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
					reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
				VALUES ('sale', ?, 'WH-MAIN', ?, ?, 'invoice', ?, ?, ?, 1, ?, ?)`,
				pidInt, item.Quantity, item.UnitPrice, invoiceNo, fmt.Sprintf("INV-%d", invoiceNo), fecha, now(), now()); err != nil {
				return nil, fmt.Errorf("inventario de %s: %w", pid, err)
			}
			var restante sql.NullFloat64
			_ = tx.QueryRow(`SELECT SUM(CASE WHEN movement_type IN ('sale','delivery','consumo','ajuste-') THEN -quantity ELSE quantity END)
				FROM stock_movement WHERE product_id = ? AND IFNULL(is_posted,1) = 1`, pidInt).Scan(&restante)
			if restante.Valid && restante.Float64 < 0 {
				avisos = append(avisos, fmt.Sprintf("stock de %s quedó en %.2f tras sincronizar la venta offline", pid, restante.Float64))
			}
		}
	}
	// Si los renglones traen su propio IVA, el asiento se construye con esa
	// suma (más fiel al detalle que el total declarado por el terminal).
	ivaAsiento := round2(req.Tax)
	if ivaSuma > 0 {
		ivaAsiento = round2(ivaSuma)
	}

	// 8. Pagos: el desglose que el terminal cobró. Sin esto el arqueo central
	// es ciego (F-12). Si el payload no trae pagos, la venta se asume
	// efectivo (el ticket lo era).
	pagos := req.Payments
	if len(pagos) == 0 {
		pagos = []pos.PaymentMethod{{Method: "efectivo", Amount: req.Total}}
	}
	foreignSum := 0.0
	for _, p := range pagos {
		metodo := strings.ToLower(strings.TrimSpace(p.Method))
		if esMetodoDivisa(metodo) {
			foreignSum += p.Amount
		}
		var pmID sql.NullInt64
		_ = tx.QueryRow(`SELECT id FROM payment_method WHERE LOWER(name) = ? AND is_active = 1 LIMIT 1`, metodo).Scan(&pmID)
		if !pmID.Valid && metodo != "" && metodo != "efectivo" && metodo != "cash" {
			avisos = append(avisos, fmt.Sprintf("el método de pago %q no existe en la empresa: el asiento lo registró como equivalente de caja/banco", p.Method))
		}
		if invInternal > 0 && pmID.Valid {
			usd := round2(p.Amount / tasa)
			if _, err := tx.Exec(`INSERT INTO invoice_payment_summary (invoice_id, cash_session_id, method_id, amount, amount_usd, exchange_rate, currency_code)
				VALUES (?,?,?,?,?,?,?)`, invInternal, nil, pmID.Int64, p.Amount, usd, tasa, moneda); err != nil {
				return nil, fmt.Errorf("resumen de pago: %w", err)
			}
		}
	}

	// 9. IGTF: manda el terminal (ya lo calculó al cobrar); si no vino pero
	// hubo pagos en divisa y la empresa lo tiene activo, se calcula aquí con
	// la misma regla del panel (IGTF% sobre lo pagado en divisa).
	if igtf == 0 && fs.IGTFEnabled && foreignSum > 0 {
		igtf = round2(foreignSum * fs.IGTFRate / 100.0)
		if igtf > 0 {
			avisos = append(avisos, fmt.Sprintf("IGTF recalculado en el servidor: %.2f (el terminal no lo envió)", igtf))
		}
	}

	// 10. Asiento doble PUBLICADO (patrón del checkout en línea, que la
	// auditoría señaló como el camino bueno): Clientes debita el total,
	// Ventas acredita la base, IVA por pagar y IGTF por pagar son pasivos.
	// Con cuenta de IVA sin asignar (empresa antigua a medio migrar), el IVA
	// se queda dentro de Ventas y se AVISA —la venta cobrada no se pierde
	// nunca por un hueco de configuración; el aviso obliga a cerrarlo—.
	if pre.VatCode == 0 && ivaAsiento > 0 {
		avisos = append(avisos, "la empresa no tiene cuenta «IVA por pagar» asignada; el IVA quedó dentro de Ventas hasta corregirlo en Configuración → Cuentas por defecto")
	}
	ventaNeto := round2(req.Total - igtf - ivaAsiento)
	if pre.VatCode == 0 {
		ventaNeto = round2(req.Total - igtf)
	}
	if ventaNeto < 0 {
		ventaNeto = 0
	}

	fyear := financialYear(db)
	voucherNo := generateVoucherNo()
	ref := strconv.FormatInt(invoiceNo, 10)
	createBy := voucherCreateBy("POS-SYSTEM")
	sub := subcodeIDTx(tx, customerID, 3)
	narr := "Venta POS " + ref
	publicar := func(coa, rev int64, debe, haber float64, subTipo int, subCod sql.NullInt64, com string) error {
		if debe == 0 && haber == 0 {
			return nil
		}
		_, err := tx.Exec(`
			INSERT INTO acc_transaction (vid, fyear, VNo, Vtype, referenceNo, VDate, COAID, Narration, Debit, Credit, CreateBy, CreateDate, StoreID, IsPosted, IsAppove, RevCodde, subType, subCode)
			VALUES (0, ?, ?, 'Invoice', ?, ?, ?, ?, ?, ?, ?, ?, 0, 'yes', 'Approved', ?, ?, ?)`,
			fyear, voucherNo, ref, fecha, coa, narr, debe, haber, createBy, now(), rev, subTipo, subCod)
		if err != nil {
			return fmt.Errorf("asiento (%s): %w", com, err)
		}
		return nil
	}
	if err := publicar(pre.CustomerCode, pre.SalesCode, req.Total, 0, 3, sub, "cobro al cliente"); err != nil {
		return nil, err
	}
	if err := publicar(pre.SalesCode, pre.CustomerCode, 0, ventaNeto, 0, sql.NullInt64{}, "ingreso por venta"); err != nil {
		return nil, err
	}
	if pre.VatCode > 0 && ivaAsiento > 0 {
		if err := publicar(pre.VatCode, pre.CustomerCode, 0, ivaAsiento, 0, sql.NullInt64{}, "IVA repercutido"); err != nil {
			return nil, err
		}
	}
	if igtf > 0 {
		if pre.CuentaIgtfPasiva() > 0 {
			if err := publicar(pre.IgtfCode, pre.CustomerCode, 0, igtf, 0, sql.NullInt64{}, "IGTF percibido"); err != nil {
				return nil, err
			}
		} else {
			avisos = append(avisos, "la empresa no tiene cuenta «IGTF por Pagar» asignada; el IGTF cobrado no quedó registrado como pasivo")
		}
	}

	// 11. Cerrar el reclamo de idempotencia con la factura ya puesta.
	if err := confirmarVentaTerminal(tx, req, invoiceNo, controlNo, controlNo); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit de la venta: %w", err)
	}

	return &pos.SyncSaleResponse{
		Success:      true,
		RemoteID:     strconv.FormatInt(invoiceNo, 10),
		Status:       "synced",
		FiscalNumber: controlNo,
		Advertencias: avisos,
	}, nil
}

// RegistrarVentaTerminal es la puerta exportada del motor, para que
// cmd/server lo inyecte en el gRPC sin crear un ciclo de importación
// (mismo patrón que NextFiscalDoc).
func (s *Server) RegistrarVentaTerminal(db *sql.DB, req *pos.SyncSaleRequest) (*pos.SyncSaleResponse, error) {
	return s.registrarVentaTerminal(db, req)
}

// ── Reclamo idempotente ─────────────────────────────────────────────────────

type modoReclamo int

const (
	reclamoNuevo    modoReclamo = iota
	reclamoYaHecho              // la venta ya terminó antes (respuesta already_exists)
	reclamoEnCurso              // otra petición la tiene reclamada sin factura aún
	reclamoSinClave             // payload sin (terminal, sale_local_id): se deja pasar
)

type resultadoReclamo struct {
	modo      modoReclamo
	invoiceID int64
	fiscal    string
}

// reclamarVentaTerminal aplica la clave idempotente ANTES del efecto, como el
// checkout en línea (pos_api.go): el que inserta el reclamo sigue; el que se
// lo encuentra hecho lee el resultado. Dentro de la MISMA tx del registro,
// para que dos reintentos concurrentes del mismo UUID se ordenen por el
// bloqueo del INSERT y nadie duplique.
func reclamarVentaTerminal(tx *sql.Tx, req *pos.SyncSaleRequest) (resultadoReclamo, error) {
	terminalID := strings.TrimSpace(req.TerminalID)
	saleLocalID := strings.TrimSpace(req.SaleLocalID)
	controlNo, _ := pos.ContingenciaRef(terminalID, req.FiscalNumber, saleLocalID)

	legajo := func() (int64, bool) {
		var existing int64
		if err := tx.QueryRow(`SELECT invoice_id FROM invoice WHERE control_no = ? AND series = 'CONT' LIMIT 1`,
			controlNo).Scan(&existing); err == nil && existing > 0 {
			return existing, true
		}
		// Segunda llave legacy: el terminal pudo imprimir ya un control de
		// bloque (las filas nuevas se guardan CON ese control en control_no,
		// no con la referencia CONT-). Sin esta comprobación, una caja de
		// firmware viejo sin sale_local_id duplicaría la venta tras el
		// cambio de semántica (F-3).
		if impreso := digitosDeNumero(req.FiscalNumber); impreso > 0 {
			if err := tx.QueryRow(`SELECT invoice_id FROM invoice WHERE control_no = ? LIMIT 1`,
				controlNoDesde(tx, impreso)).Scan(&existing); err == nil && existing > 0 {
				return existing, true
			}
		}
		return 0, false
	}

	if terminalID == "" || saleLocalID == "" {
		if id, existe := legajo(); existe {
			return resultadoReclamo{modo: reclamoSinClave, invoiceID: id}, nil
		}
		return resultadoReclamo{modo: reclamoSinClave}, nil
	}

	// INSERT pelado, no INSERT IGNORE: el reclamo debe funcionar igual en
	// MySQL (campo) y SQLite (pruebas), que no comparten la sintaxis
	// idempotente. El duplicado se reconoce por el error —como hace el
	// checkout en línea (pos_api.go)— y significa "la clave ya está
	// reclamada", que es exactamente la lectura del ganador abajo.
	res, err := tx.Exec(`INSERT INTO pos_sale_sync (terminal_id, sale_local_id, invoice_id, invoice_no, control_no, fiscal_number, status)
		VALUES (?,?,0,0,'',?,'claimed')`, terminalID, saleLocalID, req.FiscalNumber)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "unique") && !strings.Contains(msg, "constraint") {
			return resultadoReclamo{}, fmt.Errorf("reclamo idempotente: %w", err)
		}
	} else if n, _ := res.RowsAffected(); n == 1 {
		return resultadoReclamo{modo: reclamoNuevo}, nil
	}

	var inv, no int64
	var ctrl, fis, estado sql.NullString
	if e := tx.QueryRow(`SELECT invoice_id, invoice_no, control_no, fiscal_number, status FROM pos_sale_sync
		WHERE terminal_id = ? AND sale_local_id = ? LIMIT 1`, terminalID, saleLocalID).
		Scan(&inv, &no, &ctrl, &fis, &estado); e != nil {
		return resultadoReclamo{}, fmt.Errorf("leer reclamo de venta: %w", e)
	}
	if inv > 0 {
		return resultadoReclamo{modo: reclamoYaHecho, invoiceID: inv, fiscal: fis.String}, nil
	}
	if id, existe := legajo(); existe {
		// Fila huérfana (invoice_id 0) pero el documento existe por control
		// legado: se cosecha la fila y se responde hecho.
		_, _ = tx.Exec(`UPDATE pos_sale_sync SET invoice_id=?, invoice_no=?, control_no=?, status='synced'
			WHERE terminal_id=? AND sale_local_id=? AND invoice_id=0`, id, id, ctrl.String, terminalID, saleLocalID)
		return resultadoReclamo{modo: reclamoYaHecho, invoiceID: id, fiscal: fis.String}, nil
	}
	if strings.EqualFold(estado.String, "synced") {
		// Coincidencia exacta que perdió la factura por el camino (respuesta
		// cortada tras commit de otra petición que luego falló): el UNIQUE ya
		// impide duplicado de clave; se reclaman el derecho a re-registrar.
		if _, e := tx.Exec(`UPDATE pos_sale_sync SET status='claimed' WHERE terminal_id=? AND sale_local_id=? AND invoice_id=0`,
			terminalID, saleLocalID); e == nil {
			return resultadoReclamo{modo: reclamoNuevo}, nil
		}
	}
	return resultadoReclamo{modo: reclamoEnCurso}, nil
}

// confirmarVentaTerminal pega la factura al reclamo. `WHERE invoice_id = 0`
// hace la escritura condicional: dos finalizaciones concurrentes no pueden
// asignar dos facturas a la misma clave.
func confirmarVentaTerminal(tx *sql.Tx, req *pos.SyncSaleRequest, invoiceNo int64, controlNo, fiscalControl string) error {
	terminalID := strings.TrimSpace(req.TerminalID)
	saleLocalID := strings.TrimSpace(req.SaleLocalID)
	if terminalID == "" || saleLocalID == "" {
		return nil
	}
	res, err := tx.Exec(`UPDATE pos_sale_sync SET invoice_id=?, invoice_no=?, control_no=?, fiscal_number=?, status='synced'
		WHERE terminal_id=? AND sale_local_id=? AND invoice_id=0`,
		invoiceNo, invoiceNo, controlNo, fiscalControl, terminalID, saleLocalID)
	if err != nil {
		return fmt.Errorf("confirmar venta sincronizada: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("confirmar venta sincronizada: el reclamo se perdió (filas tocadas %d)", n)
	}
	return nil
}
