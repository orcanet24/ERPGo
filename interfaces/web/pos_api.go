package web

// pos_api.go — API POS con autenticación por middleware.
// Punto crítico: todas las rutas POS (excepto auth) requieren token válido.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

func (s *Server) registerPOSAPIRoutes() {
	// Rutas públicas (no requieren token)
	s.mux.HandleFunc("POST /api/pos/auth/login", s.handlePOSLogin)
	s.mux.HandleFunc("POST /api/pos/auth/validate", s.handlePOSValidate)
	s.mux.HandleFunc("POST /api/pos/auth/logout", s.handlePOSLogout)
	s.mux.HandleFunc("POST /api/pos/auth/supervisor", s.handlePOSAutorizaSupervisor)

	// Rutas protegidas (requieren token POS válido)
	s.mux.HandleFunc("POST /api/pos/checkout", s.posAPIMiddleware(s.posAPICheckout))
	s.mux.HandleFunc("GET /api/pos/catalog", s.posAPIMiddleware(s.posAPIProducts))
	s.mux.HandleFunc("POST /api/pos/shift/open", s.posAPIMiddleware(s.posAPIShiftOpen))
	s.mux.HandleFunc("POST /api/pos/shift/close", s.posAPIMiddleware(s.posAPIShiftClose))
	s.mux.HandleFunc("GET /api/pos/shift/status", s.posAPIMiddleware(s.posAPIShiftStatus))
	s.mux.HandleFunc("GET /api/pos/receipt/{invoice_id}", s.posAPIMiddleware(s.posAPIRecipt))
	s.mux.HandleFunc("GET /api/pos/invoice/{invoice_id}", s.posAPIMiddleware(s.posAPIInvoice))
}

type posUserIDContextKey struct{}
type posTenantContextKey struct{}
type posTerminalContextKey struct{}
type posBranchContextKey struct{}
type posSessionDBContextKey struct{}
type posSessionTokenContextKey struct{}

func (s *Server) posAuthenticatedDB(r *http.Request) *sql.DB {
	if db, ok := r.Context().Value(posSessionDBContextKey{}).(*sql.DB); ok && db != nil {
		return db
	}
	if tenant, ok := r.Context().Value(posTenantContextKey{}).(string); ok && tenant != "" {
		return s.dbForTenant(tenant)
	}
	return nil
}

func posIdentityFromContext(r *http.Request) (int, string, int, bool) {
	userID, ok := r.Context().Value(posUserIDContextKey{}).(int)
	if !ok || userID <= 0 {
		return 0, "", 0, false
	}
	terminalID, ok := r.Context().Value(posTerminalContextKey{}).(string)
	if !ok || strings.TrimSpace(terminalID) == "" {
		return 0, "", 0, false
	}
	branchID, _ := r.Context().Value(posBranchContextKey{}).(int)
	return userID, terminalID, branchID, true
}

func posBranchWarehouse(r *http.Request) string {
	_, terminalID, _, ok := posIdentityFromContext(r)
	if !ok {
		return ""
	}
	// cash_session.warehouse_id uses the terminal identifier on POS routes;
	// keep the invoice linked to that same shift key.
	return terminalID
}

func posSaleWasRegistered(q interface {
	QueryRow(string, ...interface{}) *sql.Row
}, terminalID, offlineID string) (int64, string, error) {
	var invoiceNo int64
	var controlNo string
	err := q.QueryRow(`SELECT invoice_no, control_no FROM pos_sale_idempotency
		WHERE terminal_id = ? AND offline_id = ?`, terminalID, offlineID).Scan(&invoiceNo, &controlNo)
	if err == nil {
		return invoiceNo, controlNo, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return 0, "", err
}

func posIsDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	message := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(message, "unique constraint failed") || strings.Contains(message, "duplicate key")
}

func posWaitForRegisteredSale(ctx context.Context, db *sql.DB, terminalID, offlineID string) (int64, string, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		invoiceNo, controlNo, err := posSaleWasRegistered(db, terminalID, offlineID)
		if err != nil {
			return 0, "", err
		}
		if invoiceNo > 0 {
			return invoiceNo, controlNo, nil
		}
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		if time.Now().After(deadline) {
			return 0, "", fmt.Errorf("venta POS %q aún se está registrando", offlineID)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// posAPIMiddleware verifica el token POS antes de ejecutar el handler.
func (s *Server) posAPIMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-POS-Token")
		if token == "" {
			jsonError(w, "Token POS requerido", http.StatusUnauthorized)
			return
		}

		tenantDB := s.posDBFor(r)
		if tenantDB == nil {
			jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
			return
		}
		var userID int
		var terminalID sql.NullString
		var branchID sql.NullInt64
		var expiresAt string
		if err := tenantDB.QueryRow(`
			SELECT user_id, terminal_id, branch_id, expires_at FROM pos_sessions
			WHERE token = ? AND expires_at > NOW() AND is_active = 1 AND IFNULL(revoked, 0) = 0
			LIMIT 1`, token).Scan(&userID, &terminalID, &branchID, &expiresAt); err != nil {
			jsonError(w, "Token inválido o expirado", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), posUserIDContextKey{}, userID)
		ctx = context.WithValue(ctx, posTenantContextKey{}, s.posTenantCode(r))
		ctx = context.WithValue(ctx, posTerminalContextKey{}, terminalID.String)
		ctx = context.WithValue(ctx, posBranchContextKey{}, int(branchID.Int64))
		ctx = context.WithValue(ctx, posSessionDBContextKey{}, tenantDB)
		ctx = context.WithValue(ctx, posSessionTokenContextKey{}, token)

		// Límite de VOLUMEN por terminal (token): una caja en mal estado o un
		// token robado no pueden inundar el servidor con syncs/checkout. Generoso
		// para no apretar la operación real; solo corta abuso. Ver pos_ratelimit.go.
		//
		// S-2 (auditoría 2026-11-22): el ESTADO del cajero se comprueba en la
		// base de control, la misma que el panel usa para dar de baja. Con la
		// copia de la empresa, un despido no cerraba la caja.
		var activeUser bool
		if ctl, errCtl := s.controlStore(); errCtl == nil {
			if err := ctl.QueryRow(`SELECT EXISTS(SELECT 1 FROM users u JOIN user_login ul ON ul.user_id = u.user_id
				WHERE u.user_id = ? AND u.status = 1 AND ul.status = 1)`, userID).Scan(&activeUser); err != nil {
				jsonError(w, "No se pudo validar el usuario POS", http.StatusInternalServerError)
				return
			}
		} else {
			// Sin identidad no hay permiso: fail-closed (el checkout no se
			// inventa un cajero activo).
			jsonError(w, "Servicio de identidad no disponible", http.StatusServiceUnavailable)
			return
		}
		if !activeUser {
			_, _ = tenantDB.Exec(`UPDATE pos_sessions SET is_active = 0, revoked = 1 WHERE token = ?`, token)
			jsonError(w, "Usuario POS inactivo", http.StatusUnauthorized)
			return
		}
		if !posVolumeLimiter.allow("tok:" + token) {
			jsonError(w, "Demasiadas peticiones, reduzca la frecuencia", http.StatusTooManyRequests)
			return
		}

		next(w, r.WithContext(ctx))
	}
}

func (s *Server) posAPICheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	controlDB := s.posAuthenticatedDB(r)
	if controlDB == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}

	var req struct {
		Items []struct {
			ProductID string  `json:"product_id"`
			Code      string  `json:"code"`
			Quantity  float64 `json:"quantity"`
		} `json:"items"`
		Payments []struct {
			Method string  `json:"method"`
			Amount float64 `json:"amount"`
		} `json:"payments"`
		TerminalID     string `json:"terminal_id"`
		OfflineID      string `json:"offline_id"`
		UseCashSession bool   `json:"use_cash_session"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	if len(req.Items) == 0 || len(req.OfflineID) > 64 {
		jsonError(w, "Carrito vacío o offline_id inválido", http.StatusBadRequest)
		return
	}
	userID, terminalID, _, ok := posIdentityFromContext(r)
	if !ok {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	if req.TerminalID != "" && req.TerminalID != terminalID {
		jsonError(w, "La caja no corresponde a la sesión POS", http.StatusForbidden)
		return
	}
	if req.OfflineID != strings.TrimSpace(req.OfflineID) {
		jsonError(w, "offline_id inválido", http.StatusBadRequest)
		return
	}
	// El OCR vive únicamente en /ocr/* (ocr_addon.go): la venta POS no
	// procesa facturas de proveedor, así que el checkout no depende del
	// estado del addon. Un addon caído no retrasa ni bloquea la caja.

	// La transacción se abre DESPUÉS de la política externa. Un addon que se
	// cae no puede quedarse con la base abierta ni retrasar la caja.
	type saleItem struct {
		ProductID string
		Name      string
		Price     float64
		Qty       float64
		TaxRate   float64
		VATRate   float64
	}

	var items []saleItem
	var subtotal, totalTax, totalVAT, total float64

	for _, it := range req.Items {
		pid := strings.TrimSpace(it.ProductID)
		if pid == "" {
			pid = strings.TrimSpace(it.Code)
		}
		if pid == "" {
			jsonError(w, "Producto sin identificador", http.StatusBadRequest)
			return
		}
		qty := it.Quantity
		if math.IsNaN(qty) || math.IsInf(qty, 0) || qty <= 0 {
			jsonError(w, "Cantidad de producto inválida", http.StatusBadRequest)
			return
		}

		var name string
		var price float64
		var taxRate float64
		var vatRate float64

		err := controlDB.QueryRow(`
			SELECT pi.product_name,
				COALESCE(ppl.price, CAST(pi.price AS DECIMAL(16,4))),
				COALESCE(pi.tax, 0),
				COALESCE(pi.product_vat, 0)
			FROM product_information pi
			LEFT JOIN product_price_list ppl ON ppl.product_id = pi.product_id AND ppl.price_list_id = 1
			WHERE (pi.product_id = ? OR pi.barcode = ?) AND pi.status = 1
			LIMIT 1`, pid, pid).Scan(&name, &price, &taxRate, &vatRate)
		if err != nil {
			jsonError(w, "Producto: "+pid, http.StatusBadRequest)
			return
		}

		if !finiteNonNegative(price) || !finiteNonNegative(vatRate) || !finiteNonNegative(taxRate) {
			jsonError(w, "Precio o impuesto de producto inválido", http.StatusInternalServerError)
			return
		}
		itemSubtotal := price * qty
		itemVAT := itemSubtotal * (vatRate / 100)
		itemTax := itemSubtotal * (taxRate / 100)
		if math.IsInf(itemSubtotal, 0) || math.IsInf(itemVAT, 0) || math.IsInf(itemTax, 0) {
			jsonError(w, "Importe del producto fuera de rango", http.StatusBadRequest)
			return
		}
		subtotal += itemSubtotal
		totalVAT += itemVAT
		totalTax += itemTax
		total += itemSubtotal + itemVAT + itemTax
		items = append(items, saleItem{pid, name, price, qty, taxRate, vatRate})
	}

	if len(items) == 0 {
		jsonError(w, "No se procesaron productos", http.StatusBadRequest)
		return
	}

	var paidTotal, cashAmount, cardAmount float64
	for _, p := range req.Payments {
		if strings.TrimSpace(p.Method) == "" || math.IsNaN(p.Amount) || math.IsInf(p.Amount, 0) || p.Amount <= 0 {
			jsonError(w, "Monto de pago inválido", http.StatusBadRequest)
			return
		}
		paidTotal += p.Amount
		method := strings.ToLower(strings.TrimSpace(p.Method))
		switch method {
		case "cash", "cashhea":
			cashAmount += p.Amount
		case "card", "punto":
			cardAmount += p.Amount
		case "transferencia", "transfer", "pago_movil", "zelle", "cripto":
			cardAmount += p.Amount
		default:
			jsonError(w, "Método de pago no admitido", http.StatusBadRequest)
			return
		}
	}

	if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(paidTotal) || math.IsInf(paidTotal, 0) || len(req.Payments) == 0 || paidTotal+0.001 < total {
		jsonError(w, "La suma de pagos no cubre el total", http.StatusBadRequest)
		return
	}
	if total <= 0 {
		jsonError(w, "Total de venta inválido", http.StatusBadRequest)
		return
	}
	change := math.Round((paidTotal-total)*100) / 100
	if change > cashAmount+0.001 {
		jsonError(w, "El vuelto debe salir del pago en efectivo", http.StatusBadRequest)
		return
	}
	// Store/account for the amount retained by the business, not cash handed
	// over before change is returned.
	cashAmount = math.Round((cashAmount-change)*100) / 100
	paidTotal = total

	// Multi-monedia: tasa BCV del día
	tasaBCV := 1.0
	totalUSD := 0.0
	tasaFuente := "BCV"
	tasaVal, fuente, err := s.ObtenerTasaHoy(controlDB, today(), "USD")
	if err == nil && tasaVal > 0 {
		tasaBCV = tasaVal
		tasaFuente = fuente
	}
	if total > 0 && tasaBCV > 0 {
		totalUSD = math.Round((total/tasaBCV)*100) / 100
	}

	// Política de venta: solo el núcleo puede vetar (stock, numeración, asiento).

	tx, err := controlDB.Begin()
	if err != nil {
		jsonError(w, "Error iniciando transacción", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if req.OfflineID != "" {
		// The UNIQUE key arbitrates concurrent requests. The loser blocks until
		// the winner commits or rolls back, then either returns its document or
		// acquires the key itself if the first attempt failed.
		if _, err := tx.Exec(`INSERT INTO pos_sale_idempotency (terminal_id, offline_id, created_at) VALUES (?, ?, NOW())`, terminalID, req.OfflineID); err != nil {
			_ = tx.Rollback()
			if posIsDuplicateKey(err) {
				invoiceNo, controlNo, waitErr := posWaitForRegisteredSale(r.Context(), controlDB, terminalID, req.OfflineID)
				if waitErr == nil {
					writeJSON(w, map[string]interface{}{"invoice_id": invoiceNo, "control_no": controlNo, "status": "paid", "already_registered": true})
					return
				}
				jsonError(w, "La venta está en proceso; reintente con el mismo offline_id", http.StatusConflict)
				return
			}
			jsonError(w, "No se pudo registrar la clave de venta POS", http.StatusInternalServerError)
			return
		}
	}

	invoiceID, err := s.reservarNumeroFacturaTx(tx)
	if err != nil {
		jsonError(w, "No se pudo reservar el número de factura", http.StatusInternalServerError)
		return
	}
	doc, err := s.nextFiscalDoc(tx, "FACT")
	if err != nil {
		jsonError(w, "No se pudo asignar número de control fiscal", http.StatusInternalServerError)
		return
	}
	userIDStr := fmt.Sprintf("%d", userID)
	var cashSessionID sql.NullInt64
	if req.UseCashSession {
		var sessionID int64
		if err := tx.QueryRow(`SELECT id FROM cash_session WHERE cashier = ? AND warehouse_id = ? AND status = 1 ORDER BY id DESC LIMIT 1`, userIDStr, terminalID).Scan(&sessionID); err != nil {
			jsonError(w, "No hay turno abierto para esta sesión", http.StatusBadRequest)
			return
		}
		cashSessionID = sql.NullInt64{Int64: sessionID, Valid: true}
	}
	pre, err := predefineAccountTx(tx)
	if err != nil || pre == nil {
		jsonError(w, "No está configurado el plan contable de la empresa", http.StatusInternalServerError)
		return
	}
	res, err := tx.Exec(`
		INSERT INTO invoice (		invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
		total_vat_amnt, total_tax, invoice, cash_amount, card_amount, invoice_details,
		sales_by, is_credit, is_draft, status, payment_type, control_no, series,
		currency_code, exchange_rate, total_amount_usd, exchange_rate_source,
		cash_session_id, warehouse_id
	) VALUES (?, 1, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, 0, 'no', 1, 0, ?, ?, 'VES', ?, ?, ?, ?, ?)`,
		invoiceID, today(), total, paidTotal, totalVAT, totalTax, invoiceID, cashAmount, cardAmount,
		"POS Terminal", userIDStr, doc.ControlNo, doc.Series, tasaBCV, totalUSD, tasaFuente, cashSessionID, posBranchWarehouse(r),
	)
	if err != nil {
		jsonError(w, "Factura: "+err.Error(), http.StatusInternalServerError)
		return
	}
	internalInvoiceID, err := res.LastInsertId()
	if err != nil {
		jsonError(w, "No se pudo identificar la factura", http.StatusInternalServerError)
		return
	}

	for i, it := range items {
		detailID := fmt.Sprintf("DET-%d-%d", i+1, invoiceID)
		_, err = tx.Exec(`
			INSERT INTO invoice_details (
				invoice_details_id, invoice_id, product_id, description,
				quantity, rate, price_list_id, discount_pct, discount_amount, base_price,
				total_price, vat_amnt, tax, paid_amount, due_amount, status
			) VALUES (?, ?, ?, ?, ?, ?, 1, '0.00', 0, ?, ?, ?, ?, ?, 0, 1)`,
			detailID, internalInvoiceID, it.ProductID, it.Name, it.Qty, it.Price,
			it.Price,
			it.Price*it.Qty, it.Price*it.Qty*(it.VATRate/100),
			it.Price*it.Qty*(it.TaxRate/100), paidTotal,
		)
		if err != nil {
			jsonError(w, "Detalle: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Publicar el asiento desde las cuentas configuradas, enlazando cada fila
	// por número de factura con el mecanismo heredado del libro mayor. En este
	// camino remoto no debe asumirse el ámbito activo de otra empresa.
	voucherNo := generateVoucherNo()
	todayStr := today()
	ref := fmt.Sprintf("%d", invoiceID)
	fyear := financialYear(controlDB)
	createBy := voucherCreateBy(userIDStr)
	for _, entry := range []struct {
		coa, reverse  int64
		debit, credit float64
	}{{pre.CashCode, pre.SalesCode, total, 0}, {pre.SalesCode, pre.CashCode, 0, total}} {
		if _, err := tx.Exec(`
			INSERT INTO acc_transaction (vid, fyear, VNo, Vtype, referenceNo, VDate, COAID, Narration, Debit, Credit, CreateBy, CreateDate, StoreID, IsPosted, IsAppove, RevCodde)
			VALUES (0, ?, ?, 'Invoice', ?, ?, ?, ?, ?, ?, ?, NOW(), 0, 'yes', 'Approved', ?)`,
			fyear, voucherNo, ref, todayStr, entry.coa, "Venta POS "+ref,
			entry.debit, entry.credit, createBy, entry.reverse); err != nil {
			jsonError(w, "Asiento: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if req.OfflineID != "" {
		res, err := tx.Exec(`UPDATE pos_sale_idempotency SET invoice_no = ?, control_no = ? WHERE terminal_id = ? AND offline_id = ? AND invoice_no = 0`,
			invoiceID, doc.ControlNo, terminalID, req.OfflineID)
		if err != nil {
			jsonError(w, "No se pudo guardar el resultado de idempotencia", http.StatusInternalServerError)
			return
		}
		if affected, err := res.RowsAffected(); err != nil || affected != 1 {
			jsonError(w, "No se pudo confirmar la clave idempotente", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, "Commit: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go s.aiAudit(nil, "POS-SYSTEM", "pos_sale", "invoice", "invoice", ref,
		fmt.Sprintf("total:%.2f,items:%d", total, len(items)), nil)

	// El checkout en línea pasa por la MISMA cola de facturación digital que
	// las ventas offline: una sola fila digital_invoice_log y un runner que
	// emite al proveedor activo de la empresa, con reintentos.
	go s.diEnqueueVenta(controlDB, s.posTenantCode(r), ref)

	writeJSON(w, map[string]interface{}{
		"invoice_id": invoiceID,
		"control_no": doc.ControlNo,
		"total":      total,
		"total_usd":  totalUSD,
		"subtotal":   subtotal,
		"tax":        totalTax,
		"vat":        totalVAT,
		"change":     change,
		"status":     "paid",
	})
}

func (s *Server) posAPIProducts(w http.ResponseWriter, r *http.Request) {
	db := s.posAuthenticatedDB(r)
	if db == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, []interface{}{})
		return
	}

	// Buscar por nombre (LIKE) o ID (igualdad exacta) o código de barras
	like := "%" + q + "%"
	rows, err := db.Query(`
		SELECT pi.product_id, pi.product_name, pi.barcode,
			COALESCE(ppl.price, CAST(pi.price AS DECIMAL(16,4))),
			COALESCE(pi.product_vat, 0),
			COALESCE(pi.tax, 0)
		FROM product_information pi
		LEFT JOIN product_price_list ppl ON ppl.product_id = pi.product_id AND ppl.price_list_id = 1
		WHERE pi.status = 1
		AND (pi.product_name LIKE ? OR pi.product_id = ? OR pi.barcode LIKE ?)
		ORDER BY pi.product_name
		LIMIT 50`, like, q, like)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var products []map[string]interface{}
	for rows.Next() {
		var id, name string
		var barcode sql.NullString
		var price, vatRate, taxRate float64
		if err := rows.Scan(&id, &name, &barcode, &price, &vatRate, &taxRate); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		products = append(products, map[string]interface{}{
			"product_id": id, "name": name, "barcode": barcode.String,
			"price": price, "vat_rate": vatRate, "tax_rate": taxRate,
		})
	}
	if err := rows.Err(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, products)
}

func (s *Server) posAPIShiftStatus(w http.ResponseWriter, r *http.Request) {
	db := s.posAuthenticatedDB(r)
	if db == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	userID, terminalID, _, ok := posIdentityFromContext(r)
	if !ok {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	var sessionID int
	var openingVES, openingUSD float64
	var openedAt time.Time
	err := db.QueryRow(`
		SELECT id, opening_amount, opening_amount_usd, opened_at
		FROM cash_session WHERE cashier = ? AND warehouse_id = ? AND status = 1
		ORDER BY opened_at DESC LIMIT 1`, fmt.Sprintf("%d", userID), terminalID).
		Scan(&sessionID, &openingVES, &openingUSD, &openedAt)
	if err == sql.ErrNoRows {
		writeJSON(w, map[string]interface{}{"has_open_shift": false, "terminal_id": terminalID})
		return
	}
	if err != nil {
		jsonError(w, "No se pudo consultar el turno", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"has_open_shift": true, "terminal_id": terminalID, "session_id": sessionID,
		"opening_balance": openingVES, "opening_balance_usd": openingUSD,
		"opened_at": openedAt.Format(time.RFC3339),
	})
}

func (s *Server) posAPIShiftOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.posAuthenticatedDB(r)
	if db == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	userID, terminalID, branchID, ok := posIdentityFromContext(r)
	if !ok {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	var req struct {
		TerminalID        string  `json:"terminal_id"`
		OpeningBalance    float64 `json:"opening_balance"`
		OpeningBalanceUSD float64 `json:"opening_balance_usd"`
		TillPurpose       string  `json:"till_purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}
	if req.TerminalID != "" && req.TerminalID != terminalID {
		jsonError(w, "La caja no corresponde a la sesión POS", http.StatusForbidden)
		return
	}
	if !finiteNonNegative(req.OpeningBalance) || !finiteNonNegative(req.OpeningBalanceUSD) {
		jsonError(w, "Fondo de apertura inválido", http.StatusBadRequest)
		return
	}
	userIDStr := strconv.Itoa(userID)
	token := posSessionToken(r)
	var activeTerminal string
	err := db.QueryRow(`SELECT terminal_id FROM pos_sessions WHERE token = ? AND is_active = 1 AND IFNULL(revoked, 0) = 0`, token).Scan(&activeTerminal)
	if err != nil || activeTerminal != terminalID {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, "No se pudo iniciar la apertura del turno", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize opens for a cashier before checking the absence of a shift;
	// locking only an empty cash_session range is not reliable across schemas.
	var lockedUserID int
	if err := tx.QueryRow(`SELECT user_id FROM users WHERE user_id = ? FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		jsonError(w, "No se pudo validar el cajero", http.StatusInternalServerError)
		return
	}
	var lockedSessionID int
	if err := tx.QueryRow(`SELECT id FROM cash_session WHERE cashier = ? AND warehouse_id = ? AND status = 1 ORDER BY id DESC LIMIT 1 FOR UPDATE`, userIDStr, terminalID).Scan(&lockedSessionID); err != nil && err != sql.ErrNoRows {
		jsonError(w, "No se pudo comprobar el turno existente", http.StatusInternalServerError)
		return
	}
	var openCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cash_session WHERE cashier = ? AND warehouse_id = ? AND status = 1`, userIDStr, terminalID).Scan(&openCount); err != nil {
		jsonError(w, "No se pudo comprobar el turno existente", http.StatusInternalServerError)
		return
	}
	if openCount > 0 {
		jsonError(w, "Ya tiene un turno abierto", http.StatusConflict)
		return
	}
	openingVES, openingUSD := req.OpeningBalance, req.OpeningBalanceUSD
	if req.TillPurpose == "continuacion" && openingVES == 0 && openingUSD == 0 {
		_ = tx.QueryRow(`SELECT COALESCE(closing_amount, 0), COALESCE(closing_amount_usd, 0)
			FROM cash_session WHERE cashier = ? AND warehouse_id = ? AND status = 0 ORDER BY closed_at DESC LIMIT 1`,
			userIDStr, terminalID).Scan(&openingVES, &openingUSD)
	}
	res, err := tx.Exec(`INSERT INTO cash_session (cashier, opening_amount, opening_amount_usd, status, opened_at, warehouse_id)
		VALUES (?, ?, ?, 1, NOW(), ?)`, userIDStr, openingVES, openingUSD, terminalID)
	if err != nil {
		jsonError(w, "Turno: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sessionID, err := res.LastInsertId()
	if err != nil {
		jsonError(w, "No se pudo identificar el turno", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`INSERT INTO pos_turn_events (terminal_id, branch_id, user_id, event_type, event_time, details)
		VALUES (?, ?, ?, 'open_turn', NOW(), ?)`, terminalID, branchID, userID,
		fmt.Sprintf("Apertura VES: %.2f, USD: %.2f", openingVES, openingUSD)); err != nil {
		jsonError(w, "No se pudo registrar la apertura del turno", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, "No se pudo confirmar la apertura del turno", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"opened": true, "session_id": sessionID, "terminal_id": terminalID,
		"opening_balance": openingVES, "opening_balance_usd": openingUSD})
}

func (s *Server) posAPIShiftClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := s.posAuthenticatedDB(r)
	if db == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	userID, terminalID, branchID, ok := posIdentityFromContext(r)
	if !ok {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	var req struct {
		TerminalID         string  `json:"terminal_id"`
		ExpectedBalance    float64 `json:"expected_balance"`
		ExpectedBalanceUSD float64 `json:"expected_balance_usd"`
		CloseNote          string  `json:"close_note"`
		ClosureType        string  `json:"closure_type"`
		SupervisorID       int     `json:"supervisor_id"`
		SupervisorCode     string  `json:"supervisor_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}
	if req.TerminalID != "" && req.TerminalID != terminalID {
		jsonError(w, "La caja no corresponde a la sesión POS", http.StatusForbidden)
		return
	}
	if !finiteNonNegative(req.ExpectedBalance) || !finiteNonNegative(req.ExpectedBalanceUSD) {
		jsonError(w, "Saldo de cierre inválido", http.StatusBadRequest)
		return
	}
	if req.ClosureType == "" {
		req.ClosureType = "X"
	}

	userIDStr := strconv.Itoa(userID)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, "No se pudo iniciar el cierre del turno", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var sessionID int
	var openingVES, openingUSD float64
	err = tx.QueryRow(`SELECT id, opening_amount, opening_amount_usd FROM cash_session
		WHERE cashier = ? AND warehouse_id = ? AND status = 1 ORDER BY opened_at DESC LIMIT 1 FOR UPDATE`,
		userIDStr, terminalID).Scan(&sessionID, &openingVES, &openingUSD)
	if err != nil {
		jsonError(w, "No hay turno abierto para esta sesión", http.StatusBadRequest)
		return
	}

	diffVES := req.ExpectedBalance - openingVES
	diffUSD := req.ExpectedBalanceUSD - openingUSD

	// Supervisor verification (approves close)
	supervisorApproved := 0
	if req.SupervisorID > 0 && req.SupervisorCode != "" {
		// Validate supervisor credentials
		var count int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ? AND password = SHA2(?, 256) AND role_id IN (SELECT id FROM role WHERE name IN ('supervisor','admin'))`,
			req.SupervisorID, req.SupervisorCode).Scan(&count)
		if count > 0 {
			supervisorApproved = 1
		}
	}

	integrityHash := fmt.Sprintf("session=%d;terminal=%s;cashier=%d;opening=%.2f;closing=%.2f;diff=%.2f;type=%s;supervisor=%d;closed_at=%s",
		sessionID, terminalID, userID, openingVES, req.ExpectedBalance, diffVES, req.ClosureType, req.SupervisorID, time.Now().Format("2006-01-02T15:04:05"))

	if _, err := tx.Exec(`UPDATE cash_session SET status = 0, closed_at = NOW(),
		closing_amount = ?, closing_amount_usd = ?, expected_cash = ?, expected_cash_usd = ?,
		difference = ?, difference_usd = ?, close_note = ?, closure_type = ?,
		supervisor_id = ?, approved_at = CASE WHEN ? = 1 THEN NOW() ELSE approved_at END,
		integrity_hash = ?
		WHERE id = ? AND status = 1`,
		req.ExpectedBalance, req.ExpectedBalanceUSD, req.ExpectedBalance, req.ExpectedBalanceUSD,
		diffVES, diffUSD, req.CloseNote, req.ClosureType,
		req.SupervisorID, supervisorApproved,
		integrityHash, sessionID); err != nil {
		jsonError(w, "Error cerrando turno: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`INSERT INTO pos_turn_events (terminal_id, branch_id, user_id, event_type, event_time, details)
		VALUES (?, ?, ?, 'close_turn', NOW(), ?)`, terminalID, branchID, userID,
		fmt.Sprintf("Cierre %s VES: %.2f (diff: %.2f), USD: %.2f (diff: %.2f), supervisor: %d",
			req.ClosureType, req.ExpectedBalance, diffVES, req.ExpectedBalanceUSD, diffUSD, req.SupervisorID)); err != nil {
		jsonError(w, "No se pudo registrar el cierre del turno", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, "No se pudo confirmar el cierre del turno", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"closed": true, "terminal_id": terminalID,
		"opening_balance": openingVES, "opening_balance_usd": openingUSD,
		"expected_balance": req.ExpectedBalance, "expected_balance_usd": req.ExpectedBalanceUSD,
		"difference": diffVES, "difference_usd": diffUSD,
		"closure_type": req.ClosureType, "supervisor_approved": supervisorApproved,
		"integrity_hash": integrityHash})
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func posSessionToken(r *http.Request) string {
	token, _ := r.Context().Value(posSessionTokenContextKey{}).(string)
	return token
}

func generateVoucherNo() string {
	return fmt.Sprintf("VCH-%s-%06d", time.Now().Format("20060102"), time.Now().UnixNano()%1000000)
}

func (s *Server) posAPIRecipt(w http.ResponseWriter, r *http.Request) {
	db := s.posAuthenticatedDB(r)
	if db == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	id := posParseInt(strings.TrimPrefix(r.URL.Path, "/api/pos/receipt/"))
	if id <= 0 {
		jsonError(w, "Factura inválida", http.StatusBadRequest)
		return
	}
	var inv struct {
		InvoiceID, CustomerID                                               int
		Date                                                                string
		TotalAmount, TotalVAT, TotalTax, PaidAmount, CashAmount, CardAmount float64
	}
	var internalInvoiceID int64
	if err := db.QueryRow(`SELECT id, invoice_id, customer_id, date, total_amount, total_vat_amnt, total_tax, paid_amount, cash_amount, card_amount FROM invoice WHERE invoice_id = ?`, id).Scan(
		&internalInvoiceID, &inv.InvoiceID, &inv.CustomerID, &inv.Date, &inv.TotalAmount, &inv.TotalVAT, &inv.TotalTax, &inv.PaidAmount, &inv.CashAmount, &inv.CardAmount); err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, "Factura no encontrada", http.StatusNotFound)
		} else {
			jsonError(w, "No se pudo consultar la factura", http.StatusInternalServerError)
		}
		return
	}
	rows, err := db.Query(`SELECT description, quantity, rate, total_price, vat_amnt, tax FROM invoice_details WHERE invoice_id = ?`, internalInvoiceID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var items []map[string]interface{}
	for rows.Next() {
		var desc string
		var qty, rate, totalPrice, vat, tax float64
		if err := rows.Scan(&desc, &qty, &rate, &totalPrice, &vat, &tax); err != nil {
			jsonError(w, "No se pudo leer el detalle de la factura", http.StatusInternalServerError)
			return
		}
		items = append(items, map[string]interface{}{"description": desc, "quantity": qty, "rate": rate, "total_price": totalPrice, "vat": vat, "tax": tax})
	}
	if err := rows.Err(); err != nil {
		jsonError(w, "No se pudo leer el detalle de la factura", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"invoice": inv, "items": items})
}

func (s *Server) posAPIInvoice(w http.ResponseWriter, r *http.Request) {
	db := s.posAuthenticatedDB(r)
	if db == nil {
		jsonError(w, "Sesión POS inválida", http.StatusUnauthorized)
		return
	}
	id := posParseInt(strings.TrimPrefix(r.URL.Path, "/api/pos/invoice/"))
	if id <= 0 {
		jsonError(w, "Factura inválida", http.StatusBadRequest)
		return
	}
	var inv struct {
		InvoiceID   int
		Date        string
		TotalAmount float64
		Status      int
	}
	if err := db.QueryRow(`SELECT invoice_id, date, total_amount, status FROM invoice WHERE invoice_id = ?`, id).Scan(&inv.InvoiceID, &inv.Date, &inv.TotalAmount, &inv.Status); err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, "Factura no encontrada", http.StatusNotFound)
		} else {
			jsonError(w, "No se pudo consultar la factura", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, inv)
}

func posParseInt(s string) int { i, _ := strconv.Atoi(s); return i }
