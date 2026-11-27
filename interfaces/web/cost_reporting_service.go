package web

import (
	"database/sql"
	"fmt"
)

// ---------------------------------------------------------------------------
// Informes e integración contable (Fase 7 del plan de costos).
//
// 1. Informe de costo vs. precio vs. margen por producto y por lista.
// 2. Costo de ventas del período desde cost_snapshot (no desde el costo
//    actual: un mes cerrado no se reescribe).
// 3. Conciliación explícita: suma de cost_snapshot vs. salidas del kardex.
// ---------------------------------------------------------------------------

// CostoMargen es el informe de costo vs precio vs margen de un producto.
type CostoMargen struct {
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Costo       float64 `json:"costo"`
	Precio      float64 `json:"precio"`
	Margen      float64 `json:"margen"`
	MargenPct   float64 `json:"margen_pct"`
}

// listarCostoMargen devuelve el informe de costo vs precio para todos los
// productos, opcionalmente filtrado por una lista de precio.
func listarCostoMargen(db *sql.DB, listaID int) ([]CostoMargen, error) {
	var q string
	var args []interface{}
	if listaID > 0 {
		q = `SELECT pi.product_id, IFNULL(pi.product_name,''), IFNULL(pi.cost,0),
			IFNULL(pl.price,0)
			FROM product_information pi
			LEFT JOIN product_price_list pl ON pl.product_id = pi.product_id
				AND pl.price_list_id = ?
			WHERE pi.status = 1
			ORDER BY pi.product_id`
		args = append(args, listaID)
	} else {
		q = `SELECT product_id, IFNULL(product_name,''), IFNULL(cost,0), 0
			FROM product_information
			WHERE status = 1
			ORDER BY product_id`
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("leer informe de costo vs precio: %w", err)
	}
	defer rows.Close()

	var out []CostoMargen
	for rows.Next() {
		var c CostoMargen
		var precio float64
		if err := rows.Scan(&c.ProductID, &c.ProductName, &c.Costo, &precio); err != nil {
			return nil, fmt.Errorf("leer renglón de informe: %w", err)
		}
		c.Precio = precio
		if c.Precio > 0 {
			c.Margen = c.Precio - c.Costo
			c.MargenPct = c.Margen / c.Precio * 100
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CostoVentasPeriodo es el costo de ventas de un período desde el snapshot.
type CostoVentasPeriodo struct {
	Periodo       string  `json:"periodo"`
	Productos     int     `json:"productos"`
	CostoTotal    float64 `json:"costo_total"`
	CostoMaterial float64 `json:"costo_material"`
	CostoLabor    float64 `json:"costo_labor"`
	CostoDepr     float64 `json:"costo_depreciacion"`
	CostoIndirect float64 `json:"costo_indirecto"`
	CostoAduana   float64 `json:"costo_aduana"`
}

// costoVentasDesdeSnapshot calcula el costo de ventas del período desde
// cost_snapshot (los costos congelados al cerrar).
func costoVentasDesdeSnapshot(db *sql.DB, periodo string) (*CostoVentasPeriodo, error) {
	r := &CostoVentasPeriodo{Periodo: periodo}
	err := db.QueryRow(`SELECT COUNT(*), IFNULL(SUM(final_cost),0),
		IFNULL(SUM(material),0), IFNULL(SUM(labor),0),
		IFNULL(SUM(depreciation),0), IFNULL(SUM(overhead),0),
		IFNULL(SUM(customs),0)
		FROM cost_snapshot WHERE period = ?`, periodo).
		Scan(&r.Productos, &r.CostoTotal, &r.CostoMaterial, &r.CostoLabor,
			&r.CostoDepr, &r.CostoIndirect, &r.CostoAduana)
	if err != nil {
		return nil, fmt.Errorf("leer costo de ventas de %s: %w", periodo, err)
	}
	return r, nil
}

// SalidaKardexPeriodo es el valor de las salidas del kardex en un período.
type SalidaKardexPeriodo struct {
	Periodo     string  `json:"periodo"`
	ValorTotal  float64 `json:"valor_total"`
	Movimientos int     `json:"movimientos"`
}

// salidasKardexPeriodo lee el valor de las salidas del kardex en un período.
// Nota: el nombre exacto de la tabla de kardex puede variar según la
// instalación; se intenta la más común (stock_movement o kardex).
func salidasKardexPeriodo(db *sql.DB, periodo string) (*SalidaKardexPeriodo, error) {
	r := &SalidaKardexPeriodo{Periodo: periodo}

	// Intentar tabla stock_movement primero, luego kardex.
	var q string
	var tablaExiste int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'stock_movement'`).Scan(&tablaExiste)
	if tablaExiste > 0 {
		q = `SELECT COUNT(*), IFNULL(SUM(unit_cost * quantity),0)
			FROM stock_movement
			WHERE type = 'salida' AND date LIKE ?`
	} else {
		q = `SELECT COUNT(*), IFNULL(SUM(unit_cost * quantity),0)
			FROM kardex
			WHERE type = 'salida' AND date LIKE ?`
	}
	var err error
	err = db.QueryRow(q, periodo+"%").Scan(&r.Movimientos, &r.ValorTotal)
	if err != nil {
		return nil, fmt.Errorf("leer salidas de kardex de %s: %w", periodo, err)
	}
	return r, nil
}

// ConciliacionCostoKardex compara el costo de ventas del snapshot con las
// salidas del kardex y reporta la diferencia.
type ConciliacionCostoKardex struct {
	Periodo           string  `json:"periodo"`
	CostoSnapshot     float64 `json:"costo_snapshot"`
	ValorKardex       float64 `json:"valor_kardex"`
	Diferencia        float64 `json:"diferencia"`
	Cuadrada          bool    `json:"cuadrada"`
	ProductoDiferente string  `json:"producto_diferente"`
}

// conciliarCostoKardex compara cost_snapshot con salidas del kardex por
// producto y reporta si cuadran o no.
func conciliarCostoKardex(db *sql.DB, periodo string) (*ConciliacionCostoKardex, error) {
	c := &ConciliacionCostoKardex{Periodo: periodo}

	snap, err := costoVentasDesdeSnapshot(db, periodo)
	if err != nil {
		return nil, err
	}
	c.CostoSnapshot = snap.CostoTotal

	kardex, err := salidasKardexPeriodo(db, periodo)
	if err != nil {
		// Si no hay tabla de kardex, no se puede conciliar; se informa.
		c.ValorKardex = 0
		c.Diferencia = c.CostoSnapshot
		c.Cuadrada = false
		return c, nil
	}
	c.ValorKardex = kardex.ValorTotal
	c.Diferencia = c.CostoSnapshot - c.ValorKardex

	// Considerar cuadrada si la diferencia es menor a 0.01 (redondeo).
	if c.Diferencia < 0 {
		c.Diferencia = -c.Diferencia
	}
	c.Cuadrada = c.Diferencia < 0.01

	// Buscar el producto con mayor diferencia (para el aviso).
	rows, err := db.Query(`SELECT cs.product_id,
		IFNULL(cs.final_cost,0) - IFNULL(km.valor,0) AS diff
		FROM cost_snapshot cs
		LEFT JOIN (
			SELECT product_id, IFNULL(SUM(unit_cost * quantity),0) AS valor
			FROM stock_movement
			WHERE type = 'salida' AND date LIKE ?
			GROUP BY product_id
		) km ON km.product_id = cs.product_id
		WHERE cs.period = ?
		ORDER BY ABS(diff) DESC LIMIT 1`, periodo+"%", periodo)
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			var pid string
			var diff float64
			if err := rows.Scan(&pid, &diff); err == nil {
				if diff > 0.01 || diff < -0.01 {
					c.ProductoDiferente = pid
				}
			}
		}
	}
	return c, nil
}
