package web

// Pruebas de base de datos del mayor del proveedor.
//
// Ejecutan las SENTENCIAS REALES —supplierLedgerMovements y los fragmentos
// supplierTotalesSQL / supplierSaldoSQL que arma el listado— contra una base en
// memoria. Lo que se comprueba aquí no se ve en una prueba de funciones puras:
//
//  1. Que una compra a crédito deje la deuda SUMANDO (crédito a la cuenta del
//     proveedor) y que un pago la reste (débito). Ese es el par de signos con el
//     que el libro mayor del proveedor tiene sentido: su cuenta es un pasivo.
//  2. Que las fechas del mayor se lean aunque el motor las devuelva como
//     time.Time (MySQL con parseTime=true). Era el motivo por el que el libro
//     decía «Sin movimientos» teniendo asientos cargados.
//  3. Que el listado saque compras, pagado y por pagar del propio documento.

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

func nuevaBaseProveedor(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	// Una sola conexión: con varias, cada una tendría su propia base :memory: y la
	// prueba pasaría o fallaría según el azar del pool.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := []string{
		`CREATE TABLE supplier_information (
			supplier_id INTEGER PRIMARY KEY,
			supplier_name TEXT NOT NULL DEFAULT '',
			mobile TEXT, address TEXT)`,
		`CREATE TABLE acc_coa (
			HeadCode INTEGER PRIMARY KEY,
			HeadName TEXT NOT NULL DEFAULT '',
			supplier_id INTEGER)`,
		`CREATE TABLE acc_subcode (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subTypeId INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT '',
			referenceNo INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE acc_transaction (
			ID INTEGER PRIMARY KEY AUTOINCREMENT,
			COAID INTEGER NOT NULL DEFAULT 0,
			VNo TEXT NOT NULL DEFAULT '',
			Vtype TEXT NOT NULL DEFAULT '',
			VDate TEXT,
			Narration TEXT NOT NULL DEFAULT '',
			Debit REAL NOT NULL DEFAULT 0,
			Credit REAL NOT NULL DEFAULT 0,
			IsAppove INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE product_purchase (
			purchase_id INTEGER PRIMARY KEY,
			supplier_id INTEGER NOT NULL DEFAULT 0,
			grand_total_amount REAL NOT NULL DEFAULT 0,
			paid_amount REAL NOT NULL DEFAULT 0,
			due_amount REAL NOT NULL DEFAULT 0)`,
	}
	for _, d := range ddl {
		if _, err := db.Exec(d); err != nil {
			t.Fatalf("no se pudo crear la tabla: %v", err)
		}
	}
	// Proveedor 1 con su cuenta propia, la que crea el alta de proveedor.
	if _, err := db.Exec(`INSERT INTO supplier_information (supplier_id, supplier_name) VALUES (1, 'Harinas del Sur')`); err != nil {
		t.Fatalf("proveedor: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, supplier_id) VALUES (2111000002, '1-Harinas del Sur', 1)`); err != nil {
		t.Fatalf("cuenta: %v", err)
	}
	return db
}

// TestMayorDelProveedorSumaLaDeudaYRestaelPago es la prueba del arreglo central:
// la deuda nace al crédito y el pago la baja al débito.
func TestMayorDelProveedorSumaLaDeudaYRestaelPago(t *testing.T) {
	db := nuevaBaseProveedor(t)

	// Compra a crédito de 100: crédito a la cuenta del proveedor, como la asienta
	// ahora purchaseSave (antes iba al débito y contra la cuenta general).
	if _, err := db.Exec(`INSERT INTO acc_transaction (COAID, VNo, Vtype, VDate, Narration, Debit, Credit, IsAppove)
		VALUES (2111000002, 'JV-1', 'JV', '2026-10-01', 'Purchase Voucher', 0, 100, 1)`); err != nil {
		t.Fatalf("compra: %v", err)
	}
	// Pago de 40: débito al proveedor.
	if _, err := db.Exec(`INSERT INTO acc_transaction (COAID, VNo, Vtype, VDate, Narration, Debit, Credit, IsAppove)
		VALUES (2111000002, 'Payment-1', 'Payment', '2026-10-05', 'Pago a proveedor', 40, 0, 1)`); err != nil {
		t.Fatalf("pago: %v", err)
	}
	// Un asiento SIN aprobar no entra al mayor.
	if _, err := db.Exec(`INSERT INTO acc_transaction (COAID, VNo, Vtype, VDate, Narration, Debit, Credit, IsAppove)
		VALUES (2111000002, 'JV-9', 'JV', '2026-10-06', 'Pendiente de aprobar', 0, 500, 0)`); err != nil {
		t.Fatalf("pendiente: %v", err)
	}
	// Y el asiento de OTRO proveedor tampoco.
	if _, err := db.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, supplier_id) VALUES (2111000003, '2-Otro', 2)`); err != nil {
		t.Fatalf("cuenta del otro: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO acc_transaction (COAID, VNo, Vtype, VDate, Narration, Debit, Credit, IsAppove)
		VALUES (2111000003, 'JV-2', 'JV', '2026-10-02', 'Compra de otro', 0, 999, 1)`); err != nil {
		t.Fatalf("otro proveedor: %v", err)
	}

	movs, totalD, totalC, err := supplierLedgerMovements(db, 2111000002, "", "")
	if err != nil {
		t.Fatalf("el mayor devolvió error: %v", err)
	}
	if len(movs) != 2 {
		t.Fatalf("se esperaban 2 movimientos (el pendiente y el de otro proveedor no cuentan), llegaron %d", len(movs))
	}
	if movs[0].Credit != 100 || movs[0].Debit != 0 {
		t.Errorf("la compra debía quedar al crédito: %+v", movs[0])
	}
	if movs[1].Debit != 40 || movs[1].Credit != 0 {
		t.Errorf("el pago debía quedar al débito: %+v", movs[1])
	}
	if movs[1].Balance != 60 {
		t.Errorf("saldo corrido: se esperaba 60 (100 de deuda menos 40 pagados), llegó %v", movs[1].Balance)
	}
	if totalD != 40 || totalC != 100 {
		t.Errorf("totales: débito %v crédito %v", totalD, totalC)
	}
	if movs[0].Date != "2026-10-01" || movs[1].Date != "2026-10-05" {
		t.Errorf("las fechas se debían leer del motor: %q y %q", movs[0].Date, movs[1].Date)
	}
}

// TestMayorDelProveedorLeeFechasComoTime cubre el tipo de valor que entrega el
// driver de MySQL con parseTime=true, que es lo que hacía caer todas las filas.
func TestMayorDelProveedorLeeFechasComoTime(t *testing.T) {
	if got := fechaBD(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); got != "2026-10-01" {
		t.Fatalf("fechaBD(time.Time) = %q, se esperaba 2026-10-01", got)
	}
	if got := fechaBD([]byte("2026-10-01")); got != "2026-10-01" {
		t.Fatalf("fechaBD([]byte) = %q, se esperaba 2026-10-01", got)
	}
	if got := fechaBD(nil); got != "" {
		t.Fatalf("fechaBD(nil) = %q, se esperaba vacío", got)
	}
}

// TestListadoDelProveedorConDeudaYPagos ejecuta los fragmentos SQL del listado
// (supplierTotalesSQL y supplierSaldoSQL) tal como los arma el handler.
func TestListadoDelProveedorConDeudaYPagos(t *testing.T) {
	db := nuevaBaseProveedor(t)

	compras := [][3]float64{
		{100, 40, 60}, // pagada a medias
		{50, 50, 0},   // pagada completa
	}
	for _, c := range compras {
		if _, err := db.Exec(`INSERT INTO product_purchase (supplier_id, grand_total_amount, paid_amount, due_amount)
			VALUES (1,?,?,?)`, c[0], c[1], c[2]); err != nil {
			t.Fatalf("compra: %v", err)
		}
	}
	// Saldo contable: 60 de deuda asentada y 20 ya pagados.
	if _, err := db.Exec(`INSERT INTO acc_transaction (COAID, VNo, Vtype, VDate, Narration, Debit, Credit, IsAppove)
		VALUES (2111000002, 'JV-1', 'JV', '2026-10-01', 'Purchase Voucher', 0, 60, 1),
		       (2111000002, 'Payment-1', 'Payment', '2026-10-02', 'Pago a proveedor', 20, 0, 1)`); err != nil {
		t.Fatalf("asientos: %v", err)
	}

	var sid int64
	var comprado, pagado, porPagar, saldo float64
	err := db.QueryRow(`SELECT a.supplier_id,`+supplierTotalesSQL+`,`+supplierSaldoSQL+` AS balance
		FROM supplier_information a
		LEFT JOIN acc_coa b ON b.supplier_id = a.supplier_id
		WHERE a.supplier_id = 1`).Scan(&sid, &comprado, &pagado, &porPagar, &saldo)
	if err != nil {
		t.Fatalf("la consulta del listado falló: %v", err)
	}
	if comprado != 150 || pagado != 90 || porPagar != 60 {
		t.Errorf("documentos: comprado %v, pagado %v, por pagar %v (se esperaban 150, 90 y 60)",
			comprado, pagado, porPagar)
	}
	if saldo != 40 {
		t.Errorf("saldo contable: se esperaban 40 (60 de deuda menos 20 pagados), llegó %v", saldo)
	}

	// Y los mismos totales por proveedor, que es lo que usa el mayor para su
	// resumen cuando no hay cuenta contable.
	err = db.QueryRow(supplierDocumentTotalsSQL, 1).Scan(&comprado, &pagado, &porPagar)
	if err != nil {
		t.Fatalf("los totales por documento fallaron: %v", err)
	}
	if comprado != 150 || pagado != 90 || porPagar != 60 {
		t.Errorf("totales por documento: %v / %v / %v", comprado, pagado, porPagar)
	}
}
