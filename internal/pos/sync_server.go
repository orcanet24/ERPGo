package pos

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AuditFunc define la firma para logging de auditoría del sync server.
type AuditFunc func(action, module, table, recordID string, oldVal, newVal interface{})

// SyncServer maneja la sincronización de ventas desde terminales POS.
// Recibe ventas de las cajas, las guarda en MySQL central y responde con
// el ID remoto de la factura creada.
type SyncServer struct {
	db          *sql.DB
	mu          sync.Mutex
	auditLogger AuditFunc
}

// NewSyncServer crea una nueva instancia del servidor de sincronización.
func NewSyncServer(db *sql.DB, audit AuditFunc) *SyncServer {
	return &SyncServer{
		db:          db,
		auditLogger: audit,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / Response structs (JSON over HTTP ahora, gRPC proto después)
// ─────────────────────────────────────────────────────────────────────────────

// SyncSaleRequest representa una venta enviada desde el terminal POS.
type SyncSaleRequest struct {
	TerminalID   string          `json:"terminal_id"`
	BranchID     int             `json:"branch_id"`
	SaleLocalID  string          `json:"sale_local_id"`
	Items        []SaleItem      `json:"items"`
	Payments     []PaymentMethod `json:"payments"`
	CustomerRIF  string          `json:"customer_rif"`
	CustomerName string          `json:"customer_name"`
	Subtotal     float64         `json:"subtotal"`
	Tax          float64         `json:"tax"`
	Total        float64         `json:"total"`
	FiscalNumber string          `json:"fiscal_number"`
}

// SyncSaleResponse es la respuesta al sincronizar una venta.
type SyncSaleResponse struct {
	Success      bool   `json:"success"`
	RemoteID     string `json:"remote_id"`
	Status       string `json:"status"`
	FiscalNumber string `json:"fiscal_number"`
	Error        string `json:"error,omitempty"`
}

// SaleItem y PaymentMethod están definidos en server.go para evitar duplicados.

// ProductSyncResponse representa un producto para sincronización POS.
type ProductSyncResponse struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Price   float64 `json:"price"`
	TaxRate float64 `json:"tax_rate"`
	Stock   float64 `json:"stock"`
}

// CustomerSyncResponse representa un cliente para sincronización POS.
type CustomerSyncResponse struct {
	RIF     string `json:"rif"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
}

// ValidateAPIKeyResponse respuesta de validación de API key.
type ValidateAPIKeyResponse struct {
	Valid      bool   `json:"valid"`
	BranchID   int    `json:"branch_id"`
	BranchName string `json:"branch_name"`
	Error      string `json:"error,omitempty"`
}

// SyncStatusResponse respuesta de estado de sincronización.
type SyncStatusResponse struct {
	TerminalID    string    `json:"terminal_id"`
	LastSync      time.Time `json:"last_sync"`
	TotalSales    int       `json:"total_sales"`
	PendingSales  int       `json:"pending_sales"`
	LastInvoiceID string    `json:"last_invoice_id"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Endpoints de negocio (llamados por los HTTP handlers en interfaces/web/)
// ─────────────────────────────────────────────────────────────────────────────

// SyncSale procesa una venta desde el terminal POS y la guarda en MySQL.
// Idempotente: si el número fiscal ya existe, retorna la factura existente.
func (s *SyncServer) SyncSale(req *SyncSaleRequest) (*SyncSaleResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// La venta llega SIN RED: el número de control fiscal todavía no existe (lo
	// asigna la imprenta digital), así que el documento se registra como
	// contingencia con referencia propia por caja. Ver contingenciaRef.
	controlNo, offlineNo := contingenciaRef(req.TerminalID, req.FiscalNumber, req.SaleLocalID)

	// 1. Verificar si el documento ya existe (idempotencia).
	if req.FiscalNumber != "" {
		var existingID int64
		err := s.db.QueryRow(`
			SELECT invoice_id FROM invoice
			WHERE control_no = ? AND series = 'CONT'`,
			controlNo).Scan(&existingID)
		if err == nil {
			return &SyncSaleResponse{
				Success:      true,
				RemoteID:     fmt.Sprintf("%d", existingID),
				Status:       "already_exists",
				FiscalNumber: req.FiscalNumber,
			}, nil
		}
	}

	// 2. Buscar o crear cliente.
	customerID, err := s.findOrCreateCustomer(req.CustomerRIF, req.CustomerName)
	if err != nil {
		return &SyncSaleResponse{
			Status: "error",
			Error:  fmt.Sprintf("error con cliente: %v", err),
		}, err
	}

	// 3. Generar número de factura.
	var invoiceNo int64
	err = s.db.QueryRow(`SELECT IFNULL(MAX(invoice),999)+1 FROM invoice`).Scan(&invoiceNo)
	if err != nil {
		return &SyncSaleResponse{
			Status: "error",
			Error:  fmt.Sprintf("error generando número de factura: %v", err),
		}, err
	}

	// 4. Insertar cabecera de factura.
	now := time.Now().Format("2006-01-02")

	_, err = s.db.Exec(`
		INSERT INTO invoice (invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
			invoice, invoice_details, status, payment_type, control_no, customer_rif, series,
			total_vat_amnt, warehouse_id, is_online, sales_by, currency_code, exchange_rate, total_amount_usd, exchange_rate_source,
			offline_invoice_no)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'VES',?,?,'BCV',?)`,
		invoiceNo, customerID, now, req.Total, req.Total, 0,
		invoiceNo, "Venta POS sincronizada (contingencia)", 1, 1, controlNo, req.CustomerRIF, "CONT",
		req.Tax, "WH-MAIN", 2, "POS-SYSTEM",
		1.0, 0.0, offlineNo)
	if err != nil {
		return &SyncSaleResponse{
			Status: "error",
			Error:  fmt.Sprintf("error insertando factura: %v", err),
		}, err
	}

	// 5. Insertar detalles de factura + stock_movement.
	for _, item := range req.Items {
		productID := item.ProductCode
		if productID == "" {
			productID = fmt.Sprintf("POS-AUTO-%d", time.Now().UnixNano())
		}

		if _, err := s.findOrCreateProduct(productID, item.Description, item.UnitPrice, item.TaxRate); err != nil {
			continue
		}

		itemTotal := item.UnitPrice * item.Quantity
		itemTax := itemTotal * item.TaxRate / 100

		_, _ = s.db.Exec(`
			INSERT INTO invoice_details (invoice_details_id, invoice_id, product_id, description,
				quantity, rate, total_price, vat_amnt, status)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("POS-%d-%s", invoiceNo, productID),
			fmt.Sprintf("%d", invoiceNo), productID, item.Description,
			item.Quantity, item.UnitPrice, itemTotal, itemTax, 1)

		// Stock movement: POS siempre descuenta al facturar (modo invoice)
		// Servicios no mueven stock
		var productType string
		_ = s.db.QueryRow(`SELECT COALESCE(pic.product_type, 'physical')
			FROM product_information pi
			LEFT JOIN product_inventory_config pic ON pic.product_id = pi.product_id
			WHERE pi.product_id = ?`, productID).Scan(&productType)

		if productType != "service" {
			_, _ = s.db.Exec(`
				INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
					reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
				VALUES ('sale', ?, 'WH-MAIN', ?, ?, 'invoice', ?, ?, ?, 1, NOW(), NOW())`,
				productID, item.Quantity, item.UnitPrice, invoiceNo, fmt.Sprintf("INV-%d", invoiceNo), now)
		}
	}

	// 6. Auditoría.
	if s.auditLogger != nil {
		s.auditLogger("pos_sync_sale", "pos", "invoice", fmt.Sprintf("%d", invoiceNo), nil, req)
	}

	return &SyncSaleResponse{
		Success:      true,
		RemoteID:     fmt.Sprintf("%d", invoiceNo),
		Status:       "synced",
		FiscalNumber: req.FiscalNumber,
	}, nil
}

// GetProducts retorna todos los productos activos para sincronización POS.
func (s *SyncServer) GetProducts() ([]ProductSyncResponse, error) {
	rows, err := s.db.Query(`
		SELECT product_id, product_name,
			CAST(price AS DECIMAL(12,2)) as price,
			COALESCE(tax, 0) as tax_rate
		FROM product_information
		WHERE status = 1
		ORDER BY product_name
		LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []ProductSyncResponse
	for rows.Next() {
		var p ProductSyncResponse
		if err := rows.Scan(&p.Code, &p.Name, &p.Price, &p.TaxRate); err != nil {
			continue
		}
		products = append(products, p)
	}
	return products, nil
}

// GetCustomers retorna clientes activos para sincronización POS.
func (s *SyncServer) GetCustomers() ([]CustomerSyncResponse, error) {
	rows, err := s.db.Query(`
		SELECT rif, customer_name,
			COALESCE(customer_address, '') as address,
			COALESCE(customer_mobile, '') as phone
		FROM customer_information
		WHERE status = 1
		ORDER BY customer_name
		LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customers []CustomerSyncResponse
	for rows.Next() {
		var c CustomerSyncResponse
		var rif sql.NullString
		if err := rows.Scan(&rif, &c.Name, &c.Address, &c.Phone); err != nil {
			continue
		}
		c.RIF = rif.String
		customers = append(customers, c)
	}
	return customers, nil
}

// ValidateAPIKey valida una API key y retorna información de la sucursal.
func (s *SyncServer) ValidateAPIKey(apiKey string) (*ValidateAPIKeyResponse, error) {
	var branchID int
	var branchName string
	err := s.db.QueryRow(`
		SELECT b.id, b.name
		FROM api_keys ak
		JOIN branches b ON b.id = ak.branch_id
		WHERE ak.api_key = ? AND ak.is_active = 1`,
		apiKey).Scan(&branchID, &branchName)
	if err != nil {
		return &ValidateAPIKeyResponse{
			Valid: false,
			Error: "API key inválida",
		}, nil
	}

	// Actualizar last_used_at.
	_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = NOW() WHERE api_key = ?`, apiKey)

	return &ValidateAPIKeyResponse{
		Valid:      true,
		BranchID:   branchID,
		BranchName: branchName,
	}, nil
}

// GetSyncStatus retorna estadísticas de sincronización para un terminal.
func (s *SyncServer) GetSyncStatus(terminalID string) (*SyncStatusResponse, error) {
	var totalSales int
	var lastInvoiceID string

	_ = s.db.QueryRow(`
		SELECT COUNT(*) FROM invoice
		WHERE series IN ('POS','CONT') AND invoice_details LIKE ?`,
		"%"+terminalID+"%").Scan(&totalSales)

	var lastID sql.NullInt64
	_ = s.db.QueryRow(`
		SELECT MAX(invoice_id) FROM invoice WHERE series IN ('POS','CONT')`).Scan(&lastID)
	if lastID.Valid {
		lastInvoiceID = fmt.Sprintf("%d", lastID.Int64)
	}

	return &SyncStatusResponse{
		TerminalID:    terminalID,
		LastSync:      time.Now(),
		TotalSales:    totalSales,
		PendingSales:  0,
		LastInvoiceID: lastInvoiceID,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de resolución de conflictos
// ─────────────────────────────────────────────────────────────────────────────

// findOrCreateCustomer busca un cliente por RIF o crea uno mínimo.
func (s *SyncServer) findOrCreateCustomer(rif, name string) (int64, error) {
	if rif == "" {
		rif = "V000000000"
	}
	if name == "" {
		name = "Cliente POS"
	}

	// Buscar por RIF.
	var existingID int64
	err := s.db.QueryRow(`SELECT customer_id FROM customer_information WHERE rif = ?`, rif).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}

	// Crear cliente mínimo.
	res, err := s.db.Exec(`
		INSERT INTO customer_information (customer_name, rif, address2, status, customer_type)
		VALUES (?, ?, '', 1, 1)`,
		name, rif)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// findOrCreateProduct busca un producto por código o crea un placeholder.
func (s *SyncServer) findOrCreateProduct(code, name string, price, taxRate float64) (string, error) {
	if code == "" {
		code = fmt.Sprintf("POS-AUTO-%d", time.Now().UnixNano())
	}

	// Verificar si existe.
	var existing string
	err := s.db.QueryRow(`SELECT product_id FROM product_information WHERE product_id = ?`, code).Scan(&existing)
	if err == nil {
		return existing, nil
	}

	// Crear producto placeholder.
	priceStr := fmt.Sprintf("%.2f", price)
	_, err = s.db.Exec(`
		INSERT INTO product_information (product_id, product_name, price, tax, status, cost)
		VALUES (?, ?, ?, ?, 1, 0)`,
		code, name, priceStr, taxRate)
	if err != nil {
		return code, err
	}
	return code, nil
}

// contingenciaRef construye la referencia del documento que la caja emitió sin
// red, y devuelve también el correlativo local de la caja.
//
// POR QUÉ NO SE INVENTA UN NÚMERO DE CONTROL.
//
// Antes aquí se hacía `controlNo := "POS-" + req.FiscalNumber`. Eso no es un
// número de control fiscal: el control lo asigna la imprenta digital autorizada
// y es consecutivo y único por emisor (Providencia SNAT/2024/000102, art. 30).
// Un "POS-7" se repite en cada caja, no existe en ningún libro del SENIAT y
// además no cabe en las consultas que cruzan control con serie.
//
// QUÉ ES ESTE DOCUMENTO MIENTRAS TANTO.
//
// Un documento de CONTINGENCIA (art. 16 de la misma providencia): la caja
// vendió sin conexión, entregó su ticket con numeración propia y la factura
// digital se emite al reconectar. Por eso la serie es 'CONT' y la referencia
// lleva el terminal y el correlativo local: identifica el papel que tiene el
// cliente y permite armar la relación que se notifica después.
//
// El formato cabe en control_no (varchar 20): "CONT-" + 6 del terminal + "-" +
// correlativo de hasta 8 dígitos.
func contingenciaRef(terminalID, fiscalNumber, saleLocalID string) (control string, offlineNo int64) {
	term := soloAlfanumerico(terminalID)
	if len(term) > 6 {
		term = term[len(term)-6:]
	}
	if term == "" {
		term = "POS"
	}
	n := strings.TrimSpace(fiscalNumber)
	if n == "" {
		n = strings.TrimSpace(saleLocalID)
	}
	n = soloAlfanumerico(n)
	if n == "" {
		n = "0"
	}
	if len(n) > 8 {
		n = n[len(n)-8:]
	}
	control = "CONT-" + term + "-" + n
	if len(control) > 20 {
		control = control[:20]
	}
	offlineNo, _ = strconv.ParseInt(soloDigitos(fiscalNumber), 10, 64)
	return control, offlineNo
}

// soloAlfanumerico deja solo A-Z y 0-9 en mayúscula.
func soloAlfanumerico(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// soloDigitos deja solo los dígitos de un texto ("7" de "7", "" de "A-7").
func soloDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
