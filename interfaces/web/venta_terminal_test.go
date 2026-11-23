package web

// venta_terminal_test.go — El motor único de ventas terminal sobre el
// esquema REAL de migraciones (no tablas a mano): si la cadena SQLite no
// sobrevive a schema_z_iva_credito_fiscal.sql o mi INSERT de cabecera pide
// una columna que el esquema no tiene, esta prueba lo dice aquí y no en una
// caja a las 8:00. (F-2/F-3/F-12/F-14, auditoría atómica 2026-11-22.)

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"erpgo/infrastructure/persistence"
	"erpgo/internal/pos"
)

func baseTerminalPrueba(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db, err := persistence.OpenSQLite(filepath.Join(t.TempDir(), "terminal.sqlite3"))
	if err != nil {
		t.Fatalf("abrir base: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := persistence.RunMigrationsFor(db); err != nil {
		t.Fatalf("migraciones sqlite: %v", err)
	}
	// El plan de cuentas con IVA/IGTF por pagar tiene que venir de la cadena:
	// esta prueba FALLA si schema_z_iva_credito_fiscal.sql no sobrevivió al
	// traductor SQLite — que es exactamente el riesgo que corrige.
	var vat int64
	if err := db.QueryRow(`SELECT vat FROM acc_predefine_account ORDER BY id LIMIT 1`).Scan(&vat); err != nil {
		t.Fatalf("leyendo predefine: %v", err)
	}
	if vat == 0 {
		t.Fatal("la migración del IVA por pagar no corrió en SQLite: las cajas asentarían el IVA dentro de Ventas")
	}
	// Catálogo como lo tendría una caja sincronizada: el segundo artículo es
	// SERVICIO y no debe descontar stock (esa era una de las afirmaciones
	// del motor que había que probar).
	if _, err := db.Exec(`INSERT INTO product_information (product_id, product_name, price, tax, status, cost, item_type)
		VALUES ('P-001','Harina PAN',50,16,1,20,'venta'), ('S-001','Corte de pelo',16,0,1,0,'servicio')`); err != nil {
		t.Fatalf("sembrando catálogo: %v", err)
	}
	s, _ := basePrueba(t)
	s.db = db
	s.controlDB = db
	return s, db
}

func ventaTerminalBasica() *pos.SyncSaleRequest {
	return &pos.SyncSaleRequest{
		TerminalID:  "CAJA-1",
		BranchID:    1,
		SaleLocalID: "01J9TESTUUID0000000000AAAA",
		Items: []pos.SaleItem{
			{ProductCode: "P-001", Description: "Harina PAN", Quantity: 2, UnitPrice: 50, TaxRate: 16},
			{ProductCode: "S-001", Description: "Corte de pelo", Quantity: 1, UnitPrice: 16, TaxRate: 0},
		},
		Payments: []pos.PaymentMethod{
			{Method: "efectivo", Amount: 60},
			{Method: "zelle", Amount: 56},
		},
		CustomerRIF:  "V12345678A",
		CustomerName: "Doña Prueba",
		Tax:          16,
		Total:        116,
	}
}

// TestRegistrarVentaTerminalAsientaBalanceaYDescuenta: la venta del terminal
// entra con asiento DOBLE BALANCEADO, IVA e IGTF como pasivos, inventario,
// resumen de pago y el reclamo idempotente en 'synced'.
func TestRegistrarVentaTerminalAsientaBalanceaYDescuenta(t *testing.T) {
	s, db := baseTerminalPrueba(t)

	resp, err := s.registrarVentaTerminal(db, ventaTerminalBasica())
	if err != nil {
		t.Fatalf("registro: %v (resp=%+v)", err, resp)
	}
	if resp == nil || !resp.Success || resp.Status != "synced" {
		t.Fatalf("respuesta inesperada: %+v", resp)
	}
	invoiceNo, err := strconv.ParseInt(resp.RemoteID, 10, 64)
	if err != nil || invoiceNo <= 999 {
		t.Fatalf("número de factura ilegible o del rango viejo: %q", resp.RemoteID)
	}

	// Cabecera con moneda y serie de contingencia.
	var (
		series, moneda, fuente, control string
		total, tasa                      float64
	)
	if err := db.QueryRow(`SELECT series, currency_code, exchange_rate_source, control_no, total_amount, exchange_rate
		FROM invoice WHERE invoice_id = ?`, invoiceNo).Scan(&series, &moneda, &fuente, &control, &total, &tasa); err != nil {
		t.Fatalf("leyendo cabecera: %v", err)
	}
	if series != "CONT" || control == "" || total != 116 {
		t.Errorf("cabecera: series=%q control=%q total=%v", series, control, total)
	}

	// El libro, por referencia de factura: débitos == créditos, y el IVA y el
	// IGTF viven como PASIVOS (no como gasto ni dentro de Ventas).
	var debe, haber float64
	if err := db.QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0) FROM acc_transaction WHERE referenceNo = ?`,
		strconv.FormatInt(invoiceNo, 10)).Scan(&debe, &haber); err != nil {
		t.Fatalf("leyendo libro: %v", err)
	}
	if round2(debe) != round2(haber) || debe == 0 {
		t.Fatalf("el asiento del terminal no balancea: débitos %.2f vs créditos %.2f", debe, haber)
	}
	var vat, igtf int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_transaction t JOIN acc_coa c ON c.HeadCode = t.COAID
		WHERE t.referenceNo = ? AND c.HeadCode = 5020701`, strconv.FormatInt(invoiceNo, 10)).Scan(&vat); err != nil {
		t.Fatalf("leyendo IVA: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_transaction t JOIN acc_coa c ON c.HeadCode = t.COAID
		WHERE t.referenceNo = ? AND c.HeadCode = 5020702`, strconv.FormatInt(invoiceNo, 10)).Scan(&igtf); err != nil {
		t.Fatalf("leyendo IGTF: %v", err)
	}
	if vat == 0 {
		t.Error("no hubo pata contra la cuenta de IVA por pagar (5020701)")
	}
	if igtf == 0 {
		t.Error("no hubo pata contra la cuenta de IGTF por pagar (5020702) habiendo un pago zelle")
	}

	// Inventario: solo el producto físico descuenta (S-001 es servicio).
	var movimientos int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stock_movement WHERE reference_id = ?`, invoiceNo).Scan(&movimientos); err != nil {
		t.Fatalf("leyendo inventario: %v", err)
	}
	if movimientos != 1 {
		t.Errorf("movimientos de stock = %d, se quería 1 (el servicio no descuenta)", movimientos)
	}

	// El reclamo de idempotencia quedó confirmado.
	var estado string
	var invID int64
	if err := db.QueryRow(`SELECT status, invoice_id FROM pos_sale_sync WHERE terminal_id='CAJA-1' AND sale_local_id=?`,
		ventaTerminalBasica().SaleLocalID).Scan(&estado, &invID); err != nil {
		t.Fatalf("leyendo reclamo: %v", err)
	}
	if estado != "synced" || invID != invoiceNo {
		t.Errorf("reclamo: status=%q invoice_id=%d, se quería synced/%d", estado, invID, invoiceNo)
	}

	// Y el período que contiene la venta se puede cerrar con ella dentro:
	// ese era el examen que F-1 y F-2 suspendían (asiento publicado directo
	// en el libro, sin fila en acc_vaucher).
	periodo := time.Now().Format("200601")
	res, err := s.CheckPeriod(reqPrueba(s, sessPrueba("admin"), "/period/check?period="+periodo), periodo)
	if err != nil {
		t.Fatalf("CheckPeriod: %v", err)
	}
	if problema, bloquea := validationBlocking(res, "invoice_asiento"); bloquea {
		t.Errorf("la venta POS publicada aparece como «sin asiento»: %s", problema)
	}
	if problema, bloquea := validationBlocking(res, "vacio"); bloquea {
		t.Errorf("un mes con ventas POS no está vacío: %s", problema)
	}
	if problema, bloquea := validationBlocking(res, "balance"); bloquea {
		t.Errorf("el libro de una venta POS balanceada descuadra: %s", problema)
	}
}

// TestRegistrarVentaTerminalIdempotencia: el reintento de la misma cola no
// duplica factura, ni número, ni libro. Es la defensa contra la respuesta
// perdida del terminal (la caja cobra una, el sync manda dos).
func TestRegistrarVentaTerminalIdempotencia(t *testing.T) {
	s, db := baseTerminalPrueba(t)

	primera, err := s.registrarVentaTerminal(db, ventaTerminalBasica())
	if err != nil {
		t.Fatalf("primer registro: %v", err)
	}
	var facturas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM invoice WHERE series='CONT'`).Scan(&facturas); err != nil {
		t.Fatal(err)
	}
	if facturas != 1 {
		t.Fatalf("se quería 1 factura, hay %d", facturas)
	}

	segunda, err := s.registrarVentaTerminal(db, ventaTerminalBasica())
	if err != nil {
		t.Fatalf("segundo registro (reintento): %v", err)
	}
	if segunda.Status != "already_exists" {
		t.Fatalf("el reintento devolvió %q, se quería already_exists", segunda.Status)
	}
	if segunda.RemoteID != primera.RemoteID {
		t.Errorf("el reintento cambió el documento: %q vs %q", segunda.RemoteID, primera.RemoteID)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM invoice WHERE series='CONT'`).Scan(&facturas); err != nil {
		t.Fatal(err)
	}
	if facturas != 1 {
		t.Errorf("el reintento duplicó la factura: %d", facturas)
	}
	var asientoFilas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_transaction WHERE referenceNo = ?`, primera.RemoteID).Scan(&asientoFilas); err != nil {
		t.Fatal(err)
	}
	// El asiento del reintento no se duplica (no hay segunda tx de libro):
	var asientoTotal int
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_transaction`).Scan(&asientoTotal); err != nil {
		t.Fatal(err)
	}
	if asientoTotal != asientoFilas {
		t.Errorf("el reintento añadió %d filas de libro duplicadas", asientoTotal-asientoFilas)
	}
}

// TestRegistrarVentaTerminalSinClaveDeducePorControl: una cola vieja (o un
// terminal sin sale_local_id) no duplica por el camino legacy: la referencia
// CONT-<terminal>-<numero> es la segunda defensa.
func TestRegistrarVentaTerminalSinClaveDeducePorControl(t *testing.T) {
	s, db := baseTerminalPrueba(t)

	req := ventaTerminalBasica()
	req.SaleLocalID = ""
	req.FiscalNumber = "7"

	primera, err := s.registrarVentaTerminal(db, req)
	if err != nil || primera.Status != "synced" {
		t.Fatalf("primer registro sin clave: %v %+v", err, primera)
	}
	control, _ := pos.ContingenciaRef(req.TerminalID, req.FiscalNumber, "")
	var got string
	if err := db.QueryRow(`SELECT control_no FROM invoice WHERE invoice_id = ?`, primera.RemoteID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got, control) {
		t.Errorf("control %q, se esperaba la referencia %q", got, control)
	}

	segunda, err := s.registrarVentaTerminal(db, req)
	if err != nil {
		t.Fatalf("segundo registro sin clave: %v", err)
	}
	if segunda.Status != "already_exists" {
		t.Fatalf("sin clave y con la misma referencia, el reintento devolvió %q", segunda.Status)
	}
}
