package web

import (
	"database/sql"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Unidades de medida y conversiones (Fase 4 del plan de costos).
//
// product_information.unit es texto libre. Para que una receta en gramos sobre
// un costo por kilo no salga 1.000 veces mal, se crea un catálogo de UoM y
// una tabla de conversiones por producto.
// ---------------------------------------------------------------------------

// UnitOfMeasure es una unidad de medida del catálogo.
type UnitOfMeasure struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	AllowsDecimal int    `json:"allows_decimal"`
}

// listarUoM devuelve todas las unidades de medida definidas.
func listarUoM(db *sql.DB) ([]UnitOfMeasure, error) {
	rows, err := db.Query(`SELECT code, name, allows_decimal
		FROM unit_of_measure ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("leer unidades de medida: %w", err)
	}
	defer rows.Close()

	var out []UnitOfMeasure
	for rows.Next() {
		var u UnitOfMeasure
		if err := rows.Scan(&u.Code, &u.Name, &u.AllowsDecimal); err != nil {
			return nil, fmt.Errorf("leer UoM: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// guardarUoM crea o actualiza una unidad de medida.
func guardarUoM(db *sql.DB, u UnitOfMeasure) error {
	if strings.TrimSpace(u.Code) == "" {
		return fmt.Errorf("el código de la UoM es obligatorio")
	}
	if strings.TrimSpace(u.Name) == "" {
		return fmt.Errorf("el nombre de la UoM es obligatorio")
	}
	_, err := db.Exec(`INSERT INTO unit_of_measure (code, name, allows_decimal)
		VALUES (?,?,?)
		ON DUPLICATE KEY UPDATE name=VALUES(name), allows_decimal=VALUES(allows_decimal)`,
		u.Code, u.Name, u.AllowsDecimal)
	if err != nil {
		return fmt.Errorf("guardar UoM %s: %w", u.Code, err)
	}
	return nil
}

// ProductUOMConversion es una conversión de unidad para un producto.
type ProductUOMConversion struct {
	ID        int     `json:"id"`
	ProductID string  `json:"product_id"`
	FromUOM   string  `json:"from_uom"`
	ToUOM     string  `json:"to_uom"`
	Factor    float64 `json:"factor"`
}

// listarConversionUoM devuelve las conversiones de un producto.
func listarConversionUoM(db *sql.DB, productID string) ([]ProductUOMConversion, error) {
	rows, err := db.Query(`SELECT id, product_id, from_uom, to_uom, factor
		FROM product_uom_conversion WHERE product_id = ?
		ORDER BY from_uom, to_uom`, productID)
	if err != nil {
		return nil, fmt.Errorf("leer conversiones de UoM de %s: %w", productID, err)
	}
	defer rows.Close()

	var out []ProductUOMConversion
	for rows.Next() {
		var c ProductUOMConversion
		if err := rows.Scan(&c.ID, &c.ProductID, &c.FromUOM, &c.ToUOM, &c.Factor); err != nil {
			return nil, fmt.Errorf("leer conversión: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// guardarConversionUoM crea o actualiza una conversión de UoM.
func guardarConversionUoM(db *sql.DB, c ProductUOMConversion) error {
	if strings.TrimSpace(c.ProductID) == "" {
		return fmt.Errorf("el producto es obligatorio")
	}
	if strings.TrimSpace(c.FromUOM) == "" || strings.TrimSpace(c.ToUOM) == "" {
		return fmt.Errorf("las unidades de origen y destino son obligatorias")
	}
	if c.FromUOM == c.ToUOM {
		return fmt.Errorf("la unidad de origen y destino son iguales")
	}
	if c.Factor <= 0 {
		return fmt.Errorf("el factor debe ser mayor que cero")
	}
	_, err := db.Exec(`INSERT INTO product_uom_conversion
		(product_id, from_uom, to_uom, factor)
		VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE factor=VALUES(factor)`,
		c.ProductID, c.FromUOM, c.ToUOM, c.Factor)
	if err != nil {
		return fmt.Errorf("guardar conversión de UoM: %w", err)
	}
	return nil
}

// borrarConversionUoM quita una conversión.
func borrarConversionUoM(db *sql.DB, id int) error {
	if id <= 0 {
		return fmt.Errorf("el identificador de conversión no es válido")
	}
	_, err := db.Exec(`DELETE FROM product_uom_conversion WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("borrar conversión de UoM: %w", err)
	}
	return nil
}

// convertirUoM convierte una cantidad de fromUom a toUom para un producto.
// Si fromUom == toUom, devuelve la cantidad sin cambios. Si hay una conversión
// directa registrada, la aplica. Si no, devuelve la cantidad sin convertir y
// un error que el llamante puede ignorar (o no).
func convertirUoM(db *sql.DB, productID, fromUom, toUom string, cantidad float64) (float64, error) {
	if strings.TrimSpace(fromUom) == "" || strings.TrimSpace(toUom) == "" {
		return cantidad, nil
	}
	if fromUom == toUom {
		return cantidad, nil
	}
	var factor float64
	err := db.QueryRow(`SELECT factor FROM product_uom_conversion
		WHERE product_id = ? AND from_uom = ? AND to_uom = ?`,
		productID, fromUom, toUom).Scan(&factor)
	if err == sql.ErrNoRows {
		return cantidad, fmt.Errorf("no hay conversión de %s a %s para el producto %s", fromUom, toUom, productID)
	}
	if err != nil {
		return 0, err
	}
	return cantidad * factor, nil
}

// unidadBaseProducto devuelve la unidad base del producto desde
// product_information.unit.
func unidadBaseProducto(db *sql.DB, productID string) string {
	var uom string
	_ = db.QueryRow(`SELECT IFNULL(unit,'') FROM product_information WHERE product_id = ?`, productID).Scan(&uom)
	return uom
}

// cantidadEnUoMBase convierte una cantidad a la unidad base del producto.
// Si no hay conversión definida, devuelve la cantidad original (asumiendo que
// ya está en la unidad base).
func cantidadEnUoMBase(db *sql.DB, productID, fromUom string, cantidad float64) float64 {
	base := unidadBaseProducto(db, productID)
	if base == "" || fromUom == base {
		return cantidad
	}
	convertido, err := convertirUoM(db, productID, fromUom, base, cantidad)
	if err != nil {
		return cantidad
	}
	return convertido
}
