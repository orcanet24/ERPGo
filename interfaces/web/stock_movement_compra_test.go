package web

// La entrada de una compra al inventario se registra por el id interno del
// producto, no por el código que escribe el formulario.
//
// El caso real: el alta de productos genera códigos alfanuméricos (U6PQK62Z),
// el renglón de la compra los guarda tal cual y `stock_movement.product_id` es
// numérico. Convertir el código con strconv.Atoi daba 0, el movimiento no se
// insertaba y el inventario se quedaba en cero con la compra ya guardada. Esta
// prueba fija el comportamiento: con un código alfanumérico el stock sube, y un
// código que no existe en el catálogo da error en vez de perderse.

import (
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func nuevaBaseMovimientos(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := []string{
		`CREATE TABLE product_information (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id TEXT NOT NULL,
			product_name TEXT NOT NULL DEFAULT '',
			item_type TEXT NOT NULL DEFAULT 'venta')`,
		`CREATE TABLE stock_movement (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			movement_type TEXT NOT NULL,
			product_id INTEGER NOT NULL,
			warehouse_id TEXT NOT NULL DEFAULT '',
			quantity REAL NOT NULL DEFAULT 0,
			unit_cost REAL NOT NULL DEFAULT 0,
			reference_type TEXT NOT NULL DEFAULT '',
			reference_id INTEGER NOT NULL DEFAULT 0,
			reference_no TEXT NOT NULL DEFAULT '',
			date TEXT NOT NULL DEFAULT '',
			is_posted INTEGER NOT NULL DEFAULT 1,
			posted_at TEXT NULL,
			created_at TEXT NULL)`,
	}
	for _, d := range ddl {
		if _, err := db.Exec(d); err != nil {
			t.Fatalf("no se pudo crear el esquema: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, product_name)
		VALUES ('U6PQK62Z', 'Harina de Maíz Precocida 1 kg')`); err != nil {
		t.Fatalf("no se pudo sembrar el producto: %v", err)
	}
	return db
}

// TestLaCompraCargaStockConCodigoAlfanumerico es el defecto que dejaba el
// inventario en cero: el renglón guarda el código, el movimiento se inserta con
// el id interno y el stock resultante es la cantidad comprada.
func TestLaCompraCargaStockConCodigoAlfanumerico(t *testing.T) {
	db := nuevaBaseMovimientos(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("no se pudo abrir la transacción: %v", err)
	}
	if err := insertarMovimientoCompra(tx, "U6PQK62Z", "WH-MAIN", 100, 8.5, 1, "2026-11-26"); err != nil {
		t.Fatalf("la compra debía cargar el movimiento: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("no se pudo confirmar: %v", err)
	}

	if stock := stockPorMovimientos(db, 1, "WH-MAIN"); stock != 100 {
		t.Errorf("stock en WH-MAIN = %v, se esperaba 100", stock)
	}
	var tipo string
	var referencia string
	if err := db.QueryRow(`SELECT movement_type, reference_no FROM stock_movement`).Scan(&tipo, &referencia); err != nil {
		t.Fatalf("no se pudo leer el movimiento: %v", err)
	}
	if tipo != "purchase" || referencia != "PUR-1" {
		t.Errorf("movimiento = %q/%q, se esperaba purchase/PUR-1", tipo, referencia)
	}
}

// TestLaCompraDeUnProductoInexistenteFalla cubre la otra mitad de la regla: si
// el código no está en el catálogo, la compra no se guarda a medias. Antes se
// guardaba sin movimiento y el inventario quedaba descuadrado en silencio.
func TestLaCompraDeUnProductoInexistenteFalla(t *testing.T) {
	db := nuevaBaseMovimientos(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("no se pudo abrir la transacción: %v", err)
	}
	defer tx.Rollback()

	if err := insertarMovimientoCompra(tx, "NO-EXISTE", "WH-MAIN", 5, 1, 2, "2026-11-26"); err == nil {
		t.Fatal("un producto que no existe debía devolver error")
	}
}
