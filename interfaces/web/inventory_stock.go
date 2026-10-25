package web

// inventory_stock.go — Cálculo centralizado de stock disponible.
//
// El sistema soporta múltiples formas de trabajo configurables:
//
// 1. Por producto: físico vs servicio (tabla product_inventory_config)
//    - physical: requiere pasa por almacén, mueve stock, va a kardex
//    - service: no mueve stock, no va a kardex, solo asiento contable
//
// 2. Por empresa: modo de descuento (tabla inventory_flow_config)
//    - "invoice" (default): descuenta al facturar
//    - "delivery": descuenta al despachar (delivery order shipped)
//    - "shipped": descuenta al enviar (delivery order shipped)
//
// 3. Reservas opcionales (stock_reservation)
//    - Pedidos y cotizaciones pueden comprometer stock
//    - No descuentan, solo reservan (no disponible para venta)
//
// Toda lectura de stock en los 17+ puntos del codebase pasa por
// stockForProduct(), que delega en el cálculo según configuración.

import (
	"database/sql"
	"net/http"
)

// productType devuelve el tipo de producto (physical o service).
// Lee de product_inventory_config. Default: physical.
func (s *Server) productType(r *http.Request, productID int) string {
	var pType string
	err := s.dbFor(r).QueryRow(
		"SELECT product_type FROM product_inventory_config WHERE product_id = ?",
		productID).Scan(&pType)
	if err != nil || pType == "" {
		return "physical" // default
	}
	return pType
}

// requiresStockMovement devuelve si el producto mueve stock.
func (s *Server) requiresStockMovement(r *http.Request, productID int) bool {
	return s.productType(r, productID) == "physical"
}

// inventoryFlowConfig devuelve la configuración de flujo de la empresa.
type inventoryFlowConfig struct {
	DeductionMode     string
	EnableReservation bool
}

func (s *Server) getInventoryFlowConfig(r *http.Request) inventoryFlowConfig {
	var mode string
	var enableReservation int
	_ = s.dbFor(r).QueryRow(
		"SELECT COALESCE(deduction_mode, 'invoice'), COALESCE(enable_reservation, 0) FROM inventory_flow_config WHERE tenant_id = ?",
		"default").Scan(&mode, &enableReservation)
	return inventoryFlowConfig{
		DeductionMode:     mode,
		EnableReservation: enableReservation == 1,
	}
}

// inventoryDeductionMode devuelve el modo de descuento configurado.
func (s *Server) inventoryDeductionMode(r *http.Request) string {
	return s.getInventoryFlowConfig(r).DeductionMode
}

// stockForProduct devuelve el stock disponible de un producto.
// Si el producto es servicio, retorna 0 (no aplica).
// Si es físico, calcula según el modo de descuento y reservas.
func (s *Server) stockForProduct(r *http.Request, productID int) float64 {
	// Si es servicio, no aplica inventario
	if !s.requiresStockMovement(r, productID) {
		return 0
	}

	fc := s.getInventoryFlowConfig(r)
	switch fc.DeductionMode {
	case "delivery", "shipped":
		stock := s.stockFromMovement(s.dbFor(r), productID)
		if fc.EnableReservation {
			stock -= s.stockReserved(r, productID)
		}
		if stock < 0 {
			return 0
		}
		return stock
	default: // "invoice"
		return s.stockForProductWithoutMovement(s.dbFor(r), productID)
	}
}

// stockForProductWithoutMovement cálculo legado: compras - facturado.
// Modo "invoice": descuenta al facturar.
func (s *Server) stockForProductWithoutMovement(db *sql.DB, productID int) float64 {
	var stock float64
	_ = db.QueryRow(`
		SELECT
			COALESCE((SELECT SUM(quantity) FROM product_purchase_details WHERE product_id = ?), 0)
		  - COALESCE((SELECT SUM(quantity) FROM invoice_details WHERE product_id = ?), 0)`,
		productID, productID).Scan(&stock)
	if stock < 0 {
		return 0
	}
	return stock
}

// stockForProductWithoutMovementTx versión con *sql.Tx para usar en invoiceSave.
func stockForProductWithoutMovementTx(tx *sql.Tx, productID int) float64 {
	var stock float64
	_ = tx.QueryRow(`
		SELECT
			COALESCE((SELECT SUM(quantity) FROM product_purchase_details WHERE product_id = ?), 0)
		  - COALESCE((SELECT SUM(quantity) FROM invoice_details WHERE product_id = ?), 0)`,
		productID, productID).Scan(&stock)
	if stock < 0 {
		return 0
	}
	return stock
}

// stockFromMovement calcula stock basado en movimientos físicos.
// entradas (compras) - salidas (stock_movement).
// Usado en modos "delivery" y "shipped".
func (s *Server) stockFromMovement(db *sql.DB, productID int) float64 {
	purchased := purchasedQty(db, productID)
	movementOut := movementOutQty(db, productID)

	stock := purchased - movementOut
	if stock < 0 {
		return 0
	}
	return stock
}

// stockReserved devuelve la cantidad reservada en stock_reservation activas.
func (s *Server) stockReserved(r *http.Request, productID int) float64 {
	var reserved float64
	_ = s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(quantity), 0)
		FROM stock_reservation
		WHERE product_id = ? AND status = 'pending'`,
		productID).Scan(&reserved)
	if reserved < 0 {
		return 0
	}
	return reserved
}

// stockCommitted devuelve cantidad comprometida en delivery orders activas.
func (s *Server) stockCommitted(db *sql.DB, productID int) float64 {
	var committed float64
	_ = db.QueryRow(`
		SELECT COALESCE(SUM(dod.quantity), 0)
		FROM delivery_order_detail dod
		JOIN delivery_order dod2 ON dod2.id = dod.delivery_order_id
		WHERE dod.product_id = ?
		  AND dod2.status IN ('confirmed', 'picking', 'packed', 'shipped')`,
		productID).Scan(&committed)
	if committed < 0 {
		return 0
	}
	return committed
}

// purchasedQty devuelve el total comprado de un producto.
func purchasedQty(db *sql.DB, productID int) float64 {
	var qty float64
	_ = db.QueryRow(
		"SELECT COALESCE(SUM(quantity), 0) FROM product_purchase_details WHERE product_id = ?",
		productID).Scan(&qty)
	if qty < 0 {
		return 0
	}
	return qty
}

// movementOutQty devuelve el total de salidas en stock_movement.
func movementOutQty(db *sql.DB, productID int) float64 {
	var qty float64
	_ = db.QueryRow(
		"SELECT COALESCE(SUM(quantity), 0) FROM stock_movement WHERE product_id = ?",
		productID).Scan(&qty)
	if qty < 0 {
		return 0
	}
	return qty
}
