package pos

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AuditFunc define la firma para logging de auditoría del sync server.
type AuditFunc func(action, module, table, recordID string, oldVal, newVal interface{})

// SyncServer maneja la sincronización de CATÁLOGOS y estado con los
// terminales POS.
//
// LA VENTA YA NO VIVE AQUÍ (F-2/F-3/F-12/F-14, auditoría atómica
// 2026-11-22): registrar una venta necesita el contador fiscal con bloqueo,
// el plan de cuentas de LA EMPRESA, el libro mayor y el régimen IGTF, y todo
// dentro de una transacción. Ese motor está en interfaces/web/venta_terminal.go
// (registrarVentaTerminal), único para HTTP-sync y gRPC. El SyncServer de
// antes hacía MAX(invoice)+1 sin transacción, sin asiento y sin pagos: la
// venta quedaba fuera del libro mayor y, si un inserto intermedio fallaba,
// la factura nacía coja. Lo que queda aquí es lo que sí le corresponde:
// delivery de productos/clientes y validación de keys.
type SyncServer struct {
	db          *sql.DB
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
// Request / Response structs (JSON over HTTP)
// ─────────────────────────────────────────────────────────────────────────────

// SyncSaleRequest representa una venta enviada desde el terminal POS.
//
// currency/exchange_rate/igtf_amount llegaron con la reparación del terminal
// (fase 4): la caja conoce la tasa y el IGTF con los que COBRÓ; el ERP los
// guarda en vez de inventar 1.0/'VES'. payments ya existía en el payload y
// se tiraba a la basura: es el desglose que hace posible el arqueo central.
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
	Currency     string          `json:"currency"`
	ExchangeRate float64         `json:"exchange_rate"`
	IgtfAmount   float64         `json:"igtf_amount"`
}

// SyncSaleResponse es la respuesta al sincronizar una venta.
//
// Advertencias: el registro offline NUNCA se rechaza por huecos de
// configuración (el dinero ya se cobró); lo que no se pudo hacer perfecto
// viaja aquí, visible, para que el panel lo repare.
type SyncSaleResponse struct {
	Success      bool     `json:"success"`
	RemoteID     string   `json:"remote_id"`
	Status       string   `json:"status"`
	FiscalNumber string   `json:"fiscal_number"`
	Error        string   `json:"error,omitempty"`
	Advertencias []string `json:"advertencias,omitempty"`
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
// Endpoints de catálogo (llamados por los HTTP handlers de interfaces/web)
// ─────────────────────────────────────────────────────────────────────────────

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

	var pending int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM pos_sale_sync WHERE status <> 'synced'`).Scan(&pending)

	return &SyncStatusResponse{
		TerminalID:    terminalID,
		LastSync:      time.Now(),
		TotalSales:    totalSales,
		PendingSales:  pending,
		LastInvoiceID: lastInvoiceID,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Referencia de contingencia
// ─────────────────────────────────────────────────────────────────────────────

// ContingenciaRef construye la referencia del documento que la caja emitió
// sin red, y devuelve también el correlativo local de la caja. Exportada
// porque quien registra la venta (interfaces/web/venta_terminal.go) la
// necesita, y la regla no puede estar escrita dos veces.
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
func ContingenciaRef(terminalID, fiscalNumber, saleLocalID string) (control string, offlineNo int64) {
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
	// El correlativo local puede venir como UUID (terminal reparado, fase 4
	// ULID/sale_local_id): se derivan deterministicamente los últimos 8
	// alfanuméricos, que es lo que cabe en el varchar y lo que el ticket
	// mostraba por cola.
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
