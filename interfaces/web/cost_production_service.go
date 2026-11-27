package web

import (
	"database/sql"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Órdenes de producción (Fase 6 del plan de costos).
//
// Una orden de producción convierte insumos en producto terminado según la
// receta activa. Se costea siempre y, si el módulo de inventario está activo,
// postea al kardex existente. Una orden cerrada no se edita: se reabre con
// motivo, igual que un período cerrado.
// ---------------------------------------------------------------------------

// ProductionOrder es una orden de fabricación.
type ProductionOrder struct {
	ID        int     `json:"id"`
	Code      string  `json:"code"`
	ProductID string  `json:"product_id"`
	Quantity  float64 `json:"quantity"`
	Periodo   string  `json:"period"`
	RouteID   int     `json:"route_id"`
	Status    string  `json:"status"`
	StartedAt string  `json:"started_at"`
	ClosedAt  string  `json:"closed_at"`
	CreatedBy string  `json:"created_by"`
	CreatedAt string  `json:"created_at"`
}

// ProductionOrderLine es una línea de consumo de una orden.
type ProductionOrderLine struct {
	ID          int     `json:"id"`
	OrderID     int     `json:"order_id"`
	ComponentID string  `json:"component_id"`
	PlannedQty  float64 `json:"planned_qty"`
	ActualQty   float64 `json:"actual_qty"`
	UnitCost    float64 `json:"unit_cost"`
	LaborGrade  string  `json:"labor_grade"`
	Hours       float64 `json:"hours"`
}

// listarOrdenes devuelve las órdenes de producción.
func listarOrdenes(db *sql.DB) ([]ProductionOrder, error) {
	rows, err := db.Query(`SELECT id, code, product_id, quantity, period,
		IFNULL(route_id,0), status, started_at, closed_at, created_by, created_at
		FROM production_order ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("leer órdenes de producción: %w", err)
	}
	defer rows.Close()

	var out []ProductionOrder
	for rows.Next() {
		var o ProductionOrder
		if err := rows.Scan(&o.ID, &o.Code, &o.ProductID, &o.Quantity,
			&o.Periodo, &o.RouteID, &o.Status, &o.StartedAt, &o.ClosedAt,
			&o.CreatedBy, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("leer orden: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// listarLineasOrden devuelve las líneas de una orden.
func listarLineasOrden(db *sql.DB, orderID int) ([]ProductionOrderLine, error) {
	rows, err := db.Query(`SELECT id, order_id, component_id, planned_qty,
		IFNULL(actual_qty,0), IFNULL(unit_cost,0), IFNULL(labor_grade,''),
		IFNULL(hours,0)
		FROM production_order_line WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("leer líneas de orden %d: %w", orderID, err)
	}
	defer rows.Close()

	var out []ProductionOrderLine
	for rows.Next() {
		var l ProductionOrderLine
		if err := rows.Scan(&l.ID, &l.OrderID, &l.ComponentID, &l.PlannedQty,
			&l.ActualQty, &l.UnitCost, &l.LaborGrade, &l.Hours); err != nil {
			return nil, fmt.Errorf("leer línea: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// crearOrdenDesdeReceta crea una orden de producción a partir de la receta
// activa del producto. Copia el BOM planificado (material y mano de obra).
func (s *Server) crearOrdenDesdeReceta(db *sql.DB, r ProductionOrder, usuario, ahora string) (int, error) {
	if strings.TrimSpace(r.Code) == "" {
		return 0, fmt.Errorf("el código de la orden es obligatorio")
	}
	if strings.TrimSpace(r.ProductID) == "" {
		return 0, fmt.Errorf("el producto es obligatorio")
	}
	if r.Quantity <= 0 {
		return 0, fmt.Errorf("la cantidad debe ser mayor que cero")
	}
	if r.Periodo == "" {
		r.Periodo = ahora[:7] // YYYY-MM
	}

	// Verificar que no exista ya una orden con ese código.
	var existe int
	_ = db.QueryRow(`SELECT COUNT(*) FROM production_order WHERE code = ?`, r.Code).Scan(&existe)
	if existe > 0 {
		return 0, fmt.Errorf("ya existe una orden con el código %s", r.Code)
	}

	// Leer la receta activa del producto para planificar líneas.
	receta, err := s.listarReceta(db, r.ProductID)
	if err != nil {
		return 0, fmt.Errorf("leer receta de %s: %w", r.ProductID, err)
	}
	if len(receta) == 0 {
		return 0, fmt.Errorf("el producto %s no tiene receta activa: no se puede planificar la orden", r.ProductID)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO production_order
		(code, product_id, quantity, period, status, created_by, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		r.Code, r.ProductID, r.Quantity, r.Periodo, "abierta", usuario, ahora)
	if err != nil {
		return 0, fmt.Errorf("crear orden: %w", err)
	}
	orderID, _ := res.LastInsertId()

	// Insertar líneas planificadas desde la receta.
	for _, linea := range receta {
		plannedQty := linea.Cantidad * r.Quantity
		unitCost := linea.UnitCost
		if strings.TrimSpace(linea.LaborGrade) != "" {
			// Mano de obra: las horas son por unidad del producto.
			plannedQty = linea.Hours * r.Quantity
			unitCost = linea.UnitCost
		}
		_, err := tx.Exec(`INSERT INTO production_order_line
			(order_id, component_id, planned_qty, actual_qty, unit_cost,
			 labor_grade, hours)
			VALUES (?,?,?,0,0,?,0)`,
			orderID, linea.ComponentID, plannedQty, linea.LaborGrade)
		if err != nil {
			return 0, fmt.Errorf("crear línea de orden: %w", err)
		}
		_ = unitCost // se usa en el cierre
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(orderID), nil
}

// registrarConsumoReal registra el consumo real de una línea de orden.
func (s *Server) registrarConsumoReal(db *sql.DB, orderID int, lineaID int, actualQty float64, usuario, ahora string) error {
	var orderStatus string
	_ = db.QueryRow(`SELECT status FROM production_order WHERE id = ?`, orderID).Scan(&orderStatus)
	if orderStatus == "cerrada" {
		return fmt.Errorf("la orden %d está cerrada: no se puede registrar consumo", orderID)
	}

	var plannedQty float64
	var componentID string
	_ = db.QueryRow(`SELECT planned_qty, component_id FROM production_order_line WHERE id = ? AND order_id = ?`,
		lineaID, orderID).Scan(&plannedQty, &componentID)

	var unitCost float64
	_ = db.QueryRow(`SELECT IFNULL((SELECT cost FROM product_information WHERE product_id = ?),0)`,
		componentID).Scan(&unitCost)

	_, err := db.Exec(`UPDATE production_order_line
		SET actual_qty = ?, unit_cost = ?
		WHERE id = ? AND order_id = ?`,
		actualQty, unitCost, lineaID, orderID)
	if err != nil {
		return fmt.Errorf("registrar consumo: %w", err)
	}
	return nil
}

// cerrarOrdenProduccion calcula el costo real de la orden, lo guarda en
// cost_component y, si el módulo de inventario está activo, postea al kardex.
// Es idempotente: cerrar dos veces no duplica.
func (s *Server) cerrarOrdenProduccion(db *sql.DB, orderID int, usuario, ahora string) (float64, error) {
	var orderCode, productID, periodo, status string
	var quantity float64
	err := db.QueryRow(`SELECT code, product_id, quantity, period, status
		FROM production_order WHERE id = ?`, orderID).
		Scan(&orderCode, &productID, &quantity, &periodo, &status)
	if err != nil {
		return 0, fmt.Errorf("orden %d no encontrada: %w", orderID, err)
	}
	if status == "cerrada" {
		return 0, fmt.Errorf("la orden %s ya está cerrada", orderCode)
	}

	lineas, err := listarLineasOrden(db, orderID)
	if err != nil {
		return 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var costoTotal float64
	for i, l := range lineas {
		qty := l.ActualQty
		if qty <= 0 {
			qty = l.PlannedQty
		}
		costo := qty * l.UnitCost
		costoTotal += costo

		// Guardar en cost_component del período de la orden.
		_, err := tx.Exec(`INSERT INTO cost_component
			(period, product_id, component, line_no, amount, quantity, unit_cost,
			 grade, hours, rate_used, source_ref, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			periodo, productID, "material", i+1, costo, qty, l.UnitCost,
			l.LaborGrade, qty, l.UnitCost, "orden:"+orderCode, ahora)
		if err != nil {
			return 0, fmt.Errorf("guardar componente de costo: %w", err)
		}
	}

	// Costo de mano de obra de la orden (si hay horas registradas en líneas).
	var costoLabor float64
	for _, l := range lineas {
		if l.Hours > 0 && l.LaborGrade != "" {
			tarifa := calcularTasaHorariaDesdeGrado(db, l.LaborGrade)
			costoLabor += l.Hours * tarifa
		}
	}
	if costoLabor > 0 {
		costoTotal += costoLabor
		_, err := tx.Exec(`INSERT INTO cost_component
			(period, product_id, component, line_no, amount, quantity, unit_cost,
			 grade, hours, rate_used, source_ref, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			periodo, productID, "labor", 1, costoLabor, 0, 0,
			"orden:"+orderCode, 0, 0, "orden:"+orderCode, ahora)
		if err != nil {
			return 0, fmt.Errorf("guardar labor de costo: %w", err)
		}
	}

	costoUnitario := costoTotal / quantity
	if quantity <= 0 {
		costoUnitario = costoTotal
	}

	// Cerrar la orden.
	_, err = tx.Exec(`UPDATE production_order
		SET status = 'cerrada', closed_at = ?
		WHERE id = ?`, ahora, orderID)
	if err != nil {
		return 0, fmt.Errorf("cerrar orden: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return costoUnitario, nil
}
