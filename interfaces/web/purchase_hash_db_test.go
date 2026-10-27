package web

// Pruebas del candado contra compras duplicadas.
//
// La identidad de la compra se calcula en Go, pero el candado que de verdad
// impide duplicar es el ÍNDICE ÚNICO de la base: por eso la prueba levanta el
// esquema real de la columna (`purchase_hash` única, admitiendo varios NULL para
// las compras anteriores al candado) e intenta la segunda inserción.
//
// Lo demás que se comprueba aquí es la regla del negocio: mismo proveedor, mismos
// productos y los mismos números es un duplicado; con otro número de factura u
// orden es otra compra.

import (
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func nuevaBaseCompras(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// El mismo esquema que agrega la migración: la columna admite NULL (compras
	// anteriores) y el índice único es lo que cierra la puerta.
	ddl := []string{
		`CREATE TABLE product_purchase (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			purchase_id INTEGER NOT NULL,
			chalan_no TEXT NOT NULL DEFAULT '',
			supplier_id INTEGER NOT NULL DEFAULT 0,
			control_no TEXT,
			supplier_rif TEXT,
			document_kind TEXT NOT NULL DEFAULT 'factura',
			purchase_hash TEXT NULL,
			grand_total_amount REAL NOT NULL DEFAULT 0)`,
		`CREATE UNIQUE INDEX uq_purchase_hash ON product_purchase (purchase_hash)`,
	}
	for _, d := range ddl {
		if _, err := db.Exec(d); err != nil {
			t.Fatalf("no se pudo crear la tabla: %v", err)
		}
	}
	return db
}

// TestElCandadoRechazaLaSegundaCompra prueba el índice único con la misma
// identidad y comprueba que las compras anteriores (hash NULL) no se estorban
// entre ellas.
func TestElCandadoRechazaLaSegundaCompra(t *testing.T) {
	db := nuevaBaseCompras(t)

	// Dos compras viejas, sin hash: el índice único tiene que admitir varios NULL.
	for _, id := range []int{1, 2} {
		if _, err := db.Exec(`INSERT INTO product_purchase (purchase_id, supplier_id, purchase_hash)
			VALUES (?, 7, NULL)`, id); err != nil {
			t.Fatalf("una compra sin hash debía entrar (fila %d): %v", id, err)
		}
	}

	lineas := []compraLineaHash{
		{ProductID: "P-1", Qty: 10, Rate: 5.5},
		{ProductID: "P-2", Qty: 2, Rate: 100},
	}
	h := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000123", "000456", lineas, 255)

	if _, err := db.Exec(`INSERT INTO product_purchase (purchase_id, supplier_id, document_kind, purchase_hash)
		VALUES (3, 7, 'factura', ?)`, h); err != nil {
		t.Fatalf("la primera compra con hash debía entrar: %v", err)
	}
	_, err := db.Exec(`INSERT INTO product_purchase (purchase_id, supplier_id, document_kind, purchase_hash)
		VALUES (4, 7, 'factura', ?)`, h)
	if err == nil {
		t.Fatal("el candado no bloqueó la segunda compra con el mismo hash")
	}
	if !compraEsDuplicadoDeClave(err) {
		t.Fatalf("el error no se reconoce como clave duplicada: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM product_purchase WHERE purchase_hash = ?`, h).Scan(&n); err != nil {
		t.Fatalf("contando: %v", err)
	}
	if n != 1 {
		t.Fatalf("quedaron %d compras con la misma identidad; se esperaba 1", n)
	}
}

// TestLaIdentidadDeLaCompra es la regla del negocio escrita como prueba.
func TestLaIdentidadDeLaCompra(t *testing.T) {
	lineas := []compraLineaHash{
		{ProductID: "P-1", Qty: 10, Rate: 5.5},
		{ProductID: "P-2", Qty: 2, Rate: 100},
	}
	base := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000123", "000456", lineas, 255)

	// El mismo contenido en otro orden es la misma compra.
	revueltas := []compraLineaHash{lineas[1], lineas[0]}
	if otro := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000123", "000456", revueltas, 255); otro != base {
		t.Error("el orden de los renglones cambió la identidad de la compra")
	}
	// Escrito de otra manera, el mismo número de control es el mismo documento.
	if otro := hashDeCompra(7, "J-30123456-7", CompraFactura, "0000000123", "000456", lineas, 255); otro != base {
		t.Error("el mismo número de control escrito sin guiones dio otra identidad")
	}
	// Cantidades y precios escritos con otro formato son los mismos importes.
	formato := []compraLineaHash{{ProductID: "P-1", Qty: 10.00, Rate: 5.50}, {ProductID: "P-2", Qty: 2, Rate: 100.00}}
	if otro := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000123", "000456", formato, 255); otro != base {
		t.Error("1 y 1.00 dieron identidades distintas")
	}

	// OTRA compra: otro número de factura u orden.
	if otro := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000123", "000457", lineas, 255); otro == base {
		t.Error("otra factura del mismo proveedor y los mismos productos no puede ser la misma compra")
	}
	if otro := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000124", "000456", lineas, 255); otro == base {
		t.Error("otro número de control no puede dar la misma identidad")
	}
	if otro := hashDeCompra(7, "J-30123456-7", CompraOrden, "00-00000123", "000456", lineas, 255); otro == base {
		t.Error("una orden no puede tener la identidad de una factura")
	}
	// Otro proveedor.
	if otro := hashDeCompra(8, "J-30123456-8", CompraFactura, "00-00000123", "000456", lineas, 255); otro == base {
		t.Error("otro proveedor no puede dar la misma identidad")
	}
	// Otros productos o cantidades.
	if otro := hashDeCompra(7, "J-30123456-7", CompraFactura, "00-00000123", "000456",
		[]compraLineaHash{{ProductID: "P-1", Qty: 11, Rate: 5.5}}, 255); otro == base {
		t.Error("otra cantidad no puede dar la misma identidad")
	}
}

// TestLaClasificacionDecideElEfectoFiscal fija qué tipo de documento da crédito
// fiscal: solo la factura del proveedor.
func TestLaClasificacionDecideElEfectoFiscal(t *testing.T) {
	casos := []struct {
		entrada string
		kind    string
		fiscal  bool
	}{
		{"factura", CompraFactura, true},
		{"Factura", CompraFactura, true},
		{"", CompraFactura, true}, // las compras ya registradas migran como factura
		{"nota_entrega", CompraNotaEntrega, false},
		{"Nota de entrega", CompraNotaEntrega, false},
		{"orden", CompraOrden, false},
		{"OC", CompraOrden, false},
		{"sin_documento", CompraSinDoc, false},
	}
	for _, c := range casos {
		if got := CompraKind(c.entrada); got != c.kind {
			t.Errorf("CompraKind(%q) = %q; se esperaba %q", c.entrada, got, c.kind)
		}
		if got := EsCompraFiscal(c.entrada); got != c.fiscal {
			t.Errorf("EsCompraFiscal(%q) = %v; se esperaba %v", c.entrada, got, c.fiscal)
		}
	}
}
