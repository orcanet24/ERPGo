package web

// inventory_stock.go — Cálculo centralizado del stock disponible.
//
// QUÉ CAMBIÓ Y POR QUÉ
//
// Antes el stock se calculaba como "lo comprado menos lo facturado" leyendo
// directamente `product_purchase_details` e `invoice_details`. Ese cálculo no
// veía almacenes, ni ajustes, ni devoluciones, ni consumo interno; por eso el
// papel de la oficina entraba al inventario de venta y no había forma de sacarlo
// sin cargarlo al costo de ventas.
//
// Ahora la fuente es `stock_movement`: cada entrada y cada salida queda con su
// almacén y su tipo, y el stock es la suma con signo de esos movimientos. El
// stock se puede pedir total (toda la empresa) o por almacén, que es lo que
// necesita una empresa con varias locaciones.
//
// Lo que NO cambia es la configuración de flujo: el modo `invoice` sigue
// descontando al facturar y los modos `delivery`/`shipped` al despachar. Lo que
// cambia es dónde se escribe el movimiento (el almacén elegido, no "WH-MAIN") y
// que un servicio no escribe ninguno.

import (
	"database/sql"
	"net/http"
)

// tipoItemProducto devuelve el tipo del producto del catálogo.
//
// Tolera catálogos anteriores a la migración (sin `item_type`) y devuelve
// "venta", que es el valor histórico de todo lo que ya existía.
func (s *Server) tipoItemProducto(r *http.Request, productID int) string {
	var tipo sql.NullString
	// COINCIDENCIA DOBLE (D-4, pruebas de ajuste/reversión): los llamadores
	// llegan con dos mundos distintos metidos en el mismo int —el CÓDIGO
	// numérico del producto (product_id, varchar) en los formularios viejos,
	// y el id interno en quien ya lo resolvió con idInternoProducto—. Con
	// `WHERE product_id = ?` a secas, un código alfanumérico ya resuelto a id
	// interno no encontraba su fila y todo producto era «venta» por defecto:
	// los servicios descontaban stock y los suministros se colaban en las
	// ventas. Comparar por código O por id cubre ambos sin tocar llamadores.
	err := s.dbFor(r).QueryRow(
		`SELECT IFNULL(NULLIF(item_type,''),'venta') FROM product_information
		 WHERE product_id = CAST(? AS CHAR) OR id = ? LIMIT 1`,
		productID, productID).Scan(&tipo)
	if err != nil {
		return ItemVenta
	}
	return normalizarTipoItem(tipo.String)
}

// productType se conserva para los llamadores que ya existían. Devuelve el tipo
// del ítem (venta/consumo/servicio).
func (s *Server) productType(r *http.Request, productID int) string {
	return s.tipoItemProducto(r, productID)
}

// requiresStockMovement dice si el producto vive en un almacén.
func (s *Server) requiresStockMovement(r *http.Request, productID int) bool {
	return itemMueveStock(s.tipoItemProducto(r, productID))
}

// inventoryFlowConfig es la configuración de flujo de la empresa.
type inventoryFlowConfig struct {
	DeductionMode     string
	EnableReservation bool
}

func (s *Server) getInventoryFlowConfig(r *http.Request) inventoryFlowConfig {
	var mode string
	var enableReservation int
	err := s.dbFor(r).QueryRow(
		"SELECT COALESCE(deduction_mode, 'invoice'), COALESCE(enable_reservation, 0) FROM inventory_flow_config WHERE tenant_id = ?",
		"default").Scan(&mode, &enableReservation)
	if err != nil || mode == "" {
		// Sin tabla o sin fila: el comportamiento histórico es descontar al
		// facturar. No se inventa otro modo.
		mode = "invoice"
	}
	return inventoryFlowConfig{DeductionMode: mode, EnableReservation: enableReservation == 1}
}

// inventoryDeductionMode devuelve el modo de descuento configurado.
func (s *Server) inventoryDeductionMode(r *http.Request) string {
	return s.getInventoryFlowConfig(r).DeductionMode
}

// stockForProduct devuelve el stock total del producto en todos los almacenes.
// Un servicio devuelve 0: no tiene existencias.
func (s *Server) stockForProduct(r *http.Request, productID int) float64 {
	if !s.requiresStockMovement(r, productID) {
		return 0
	}
	stock := stockPorMovimientos(s.dbFor(r), productID, "")
	fc := s.getInventoryFlowConfig(r)
	if fc.EnableReservation && normalizarTipoItem(s.tipoItemProducto(r, productID)) == ItemVenta {
		stock -= s.stockReserved(r, productID)
	}
	if stock < 0 {
		return 0
	}
	return stock
}

// stockForProductEnAlmacen devuelve el stock del producto en un almacén.
// Es la lectura que necesita el multi-almacén: "cuánto hay en Lara" y "cuánto
// hay en Caracas" no son la misma pregunta.
func (s *Server) stockForProductEnAlmacen(r *http.Request, productID int, almacen string) float64 {
	if !s.requiresStockMovement(r, productID) {
		return 0
	}
	stock := stockPorMovimientos(s.dbFor(r), productID, almacen)
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
