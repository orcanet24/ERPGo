package web

// stock_movement_helper.go — Funciones auxiliares para crear stock_movement.

import (
	"database/sql"
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
