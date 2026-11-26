package web

// stock_movement_helper.go — Funciones auxiliares para crear stock_movement.

import (
	"database/sql"
	"fmt"
)

// insertStockMovement crea un registro en stock_movement.
// Es la ÚNICA forma de crear movimientos de stock en el sistema.
func insertStockMovement(tx *sql.Tx, movementType string, productID int, warehouseID string,
	quantity, unitCost float64, referenceType string, referenceID int, referenceNo string, date string) error {
	_, err := tx.Exec(`
		INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
			reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		movementType, productID, warehouseID, quantity, unitCost,
		referenceType, referenceID, referenceNo, date)
	return err
}

// insertarMovimientoCompra registra la entrada de un renglón de compra.
//
// El renglón trae el CÓDIGO del producto (`product_id`, la clave natural que
// escribe el formulario), pero `stock_movement.product_id` es el id numérico
// interno. Convertir el código con strconv.Atoi devolvía 0 para cualquier
// código alfanumérico —el que genera el alta de productos—, y el movimiento no
// se insertaba: la compra quedaba guardada y el inventario seguía en cero. La
// conversión pasa por idInternoProducto, la misma que ya usaba la factura.
func insertarMovimientoCompra(tx *sql.Tx, productID, warehouseID string, quantity, unitCost float64,
	purchaseID int, date string) error {
	pid := idInternoProducto(tx, productID)
	if pid == 0 {
		return fmt.Errorf("el producto %q no existe en el catálogo: no se puede cargar su entrada al inventario", productID)
	}
	return insertStockMovement(tx, "purchase", pid, warehouseID, quantity, unitCost,
		"purchase", purchaseID, fmt.Sprintf("PUR-%d", purchaseID), date)
}

// insertStockMovementEntrada crea un movimiento de entrada (cantidad positiva).
func insertStockMovementEntrada(tx *sql.Tx, productID int, warehouseID string,
	quantity, unitCost float64, referenceType string, referenceID int, referenceNo string, date string) error {
	return insertStockMovement(tx, movementTypeEntrada(referenceType), productID, warehouseID, quantity, unitCost, referenceType, referenceID, referenceNo, date)
}

// movementTypeEntrada devuelve el tipo de movimiento de entrada según el referenceType.
func movementTypeEntrada(referenceType string) string {
	switch referenceType {
	case "purchase":
		return "purchase"
	case "return":
		return "return_customer"
	case "adjustment":
		return "adjustment_add"
	default:
		return "purchase"
	}
}

// movementTypeSalida devuelve el tipo de movimiento de salida según el referenceType.
func movementTypeSalida(referenceType string) string {
	switch referenceType {
	case "invoice":
		return "sale"
	case "delivery_order":
		return "delivery"
	case "return":
		return "return_supplier"
	case "adjustment":
		return "adjustment_remove"
	default:
		return "sale"
	}
}
