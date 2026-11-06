package web

// Pruebas de la separación venta / consumo interno / servicio.
//
// Lo que se protege aquí es la regla que antes no existía: un servicio no mueve
// stock ni toca inventario; un suministro entra a su propio inventario; la
// mercancía de venta va al inventario comercial. Y los importes de un documento
// con varios destinos tienen que cerrar al centavo.

import (
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestNormalizarTipoItem(t *testing.T) {
	casos := map[string]string{
		"":            ItemVenta,
		"venta":       ItemVenta,
		"physical":    ItemVenta,
		"consumo":     ItemConsumo,
		"suministro":  ItemConsumo,
		"autoconsumo": ItemConsumo,
		"servicio":    ItemServicio,
		"service":     ItemServicio,
		"gasto":       ItemServicio,
	}
	for entrada, esperado := range casos {
		if got := normalizarTipoItem(entrada); got != esperado {
			t.Errorf("normalizarTipoItem(%q) = %q; se esperaba %q", entrada, got, esperado)
		}
	}
}

func TestSoloVentaYConsumoMuevenStock(t *testing.T) {
	if !itemMueveStock(ItemVenta) || !itemMueveStock(ItemConsumo) {
		t.Fatal("la mercancía y los suministros deben moverse en almacén")
	}
	if itemMueveStock(ItemServicio) {
		t.Fatal("un servicio no puede mover stock: no tiene existencias")
	}
	if tipoAlmacenDeItem(ItemServicio) != "" {
		t.Fatal("un servicio no tiene almacén")
	}
	if tipoAlmacenDeItem(ItemConsumo) != AlmacenTipoConsumo {
		t.Fatal("el consumo interno debe vivir en el almacén de consumo")
	}
}

func predefinidaDePrueba() *PredefineAccount {
	return &PredefineAccount{
		InventoryCode:         1020401,
		COGSCode:              4010101,
		VatCode:               1020801,
		ServiceCode:           3010401, // ingreso por servicios
		SuppliesInventoryCode: 1020403,
		SuppliesExpenseCode:   4021402,
		ServiceExpenseCode:    4021401,
		PurchaseVatCode:       1020901,
	}
}

func TestDestinoDeRenglonCompra(t *testing.T) {
	pre := predefinidaDePrueba()

	// Servicio: gasto, sin almacén y sin stock.
	d, err := destinoDeRenglonCompra(pre, ItemServicio, AlmacenVentaPorDefecto, 0)
	if err != nil {
		t.Fatalf("un servicio con cuenta predefinida no puede fallar: %v", err)
	}
	if d.MueveStock {
		t.Error("el servicio no puede mover stock")
	}
	if d.Almacen != "" {
		t.Errorf("el servicio no debe quedar en un almacén, quedó en %q", d.Almacen)
	}
	if d.Cuenta != pre.ServiceExpenseCode {
		t.Errorf("cuenta del servicio = %d; se esperaba la de gasto %d", d.Cuenta, pre.ServiceExpenseCode)
	}
	if d.Cuenta == pre.ServiceCode {
		t.Error("el gasto de un servicio no puede asentarse contra la cuenta de INGRESO por servicios")
	}

	// El usuario puede elegir otra cuenta de gasto para el renglón.
	d, err = destinoDeRenglonCompra(pre, ItemServicio, "", 4021402)
	if err != nil || d.Cuenta != 4021402 {
		t.Errorf("la cuenta elegida en el renglón debe respetarse: cuenta=%d err=%v", d.Cuenta, err)
	}

	// Suministros: inventario de suministros y almacén de consumo.
	d, err = destinoDeRenglonCompra(pre, ItemConsumo, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !d.MueveStock || d.Almacen != AlmacenConsumoPorDefecto {
		t.Errorf("el suministro debe entrar al almacén de consumo, quedó %+v", d)
	}
	if d.Cuenta != pre.SuppliesInventoryCode {
		t.Errorf("cuenta de suministros = %d; se esperaba %d", d.Cuenta, pre.SuppliesInventoryCode)
	}

	// Mercancía de venta: inventario comercial.
	d, err = destinoDeRenglonCompra(pre, ItemVenta, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.Cuenta != pre.InventoryCode || d.Almacen != AlmacenVentaPorDefecto {
		t.Errorf("la mercancía debe ir al inventario comercial, quedó %+v", d)
	}
}

func TestServicioSinCuentaDeGastoNoSeAsienta(t *testing.T) {
	pre := &PredefineAccount{InventoryCode: 1020401}
	if _, err := destinoDeRenglonCompra(pre, ItemServicio, "", 0); err == nil {
		t.Fatal("sin cuenta de gasto el servicio debe rechazarse, no caer a la cuenta de ingreso")
	}
}

func TestCuentasNuevasSinRespaldoSilencioso(t *testing.T) {
	// Papel sin cuenta asignada: se devuelve 0 para que la operación se detenga
	// con un mensaje que manda a Configuración → Cuentas por defecto. Caer al
	// inventario general, al costo de ventas o al IVA por pagar reintroduciría
	// exactamente la mezcla que esta separación corrige.
	pre := &PredefineAccount{InventoryCode: 1020401, COGSCode: 4010101, VatCode: 1020801, ServiceCode: 3010401}
	if got := pre.CuentaInventarioSuministros(); got != 0 {
		t.Errorf("suministros no puede caer al inventario de mercancía: %d", got)
	}
	if got := pre.CuentaGastoSuministros(); got != 0 {
		t.Errorf("el gasto de suministros no puede caer al costo de ventas: %d", got)
	}
	if got := pre.CuentaIvaCompras(); got != 0 {
		t.Errorf("el IVA de compras no puede caer al IVA por pagar: %d", got)
	}
	if got := pre.CuentaGastoServicio(); got != 0 {
		t.Errorf("un gasto de servicio NUNCA debe caer a la cuenta de ingreso; se obtuvo %d", got)
	}
}

func TestRepartirImporteCierraAlCentavo(t *testing.T) {
	partes := repartirImporte(100, []float64{1, 1, 1})
	if len(partes) != 3 {
		t.Fatalf("se esperaban 3 partes, hay %d", len(partes))
	}
	var suma float64
	for _, p := range partes {
		suma = round2(suma + p)
	}
	if suma != 100 {
		t.Fatalf("las partes suman %.2f y deben sumar 100.00 (%v)", suma, partes)
	}

	// Reparto con pesos distintos y centavos que no dividen exacto.
	partes = repartirImporte(0.10, []float64{1, 1, 1})
	suma = 0
	for _, p := range partes {
		suma = round2(suma + p)
	}
	if suma != 0.10 {
		t.Fatalf("las partes suman %.2f y deben sumar 0.10 (%v)", suma, partes)
	}
}

func nuevaBaseStock(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE stock_movement (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		movement_type TEXT NOT NULL,
		product_id INTEGER NOT NULL,
		warehouse_id TEXT NOT NULL DEFAULT 'WH-MAIN',
		quantity REAL NOT NULL,
		is_posted INTEGER NOT NULL DEFAULT 1
	)`); err != nil {
		t.Fatalf("no se pudo crear stock_movement: %v", err)
	}
	return db
}

func movimiento(t *testing.T, db *sql.DB, tipo string, producto int, almacen string, qty float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity)
		VALUES (?,?,?,?)`, tipo, producto, almacen, qty); err != nil {
		t.Fatalf("insertando movimiento: %v", err)
	}
}

func TestStockPorMovimientosSeparaAlmacenesYConsumo(t *testing.T) {
	db := nuevaBaseStock(t)
	// Entran 10 al almacén de venta y 6 al de consumo.
	movimiento(t, db, "purchase", 1, "WH-MAIN", 10)
	movimiento(t, db, "purchase", 7, "WH-CONSUMO", 6)
	// Salen 3 por venta del almacén de venta.
	movimiento(t, db, "sale", 1, "WH-MAIN", 3)
	// Y 2 de papel por consumo interno del almacén de consumo.
	movimiento(t, db, "consumo", 7, "WH-CONSUMO", 2)
	// Un movimiento no aprobado no cuenta.
	if _, err := db.Exec(`INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, is_posted)
		VALUES ('sale', 1, 'WH-MAIN', 99, 0)`); err != nil {
		t.Fatal(err)
	}

	if got := stockPorMovimientos(db, 1, ""); got != 7 {
		t.Errorf("stock total del producto de venta = %.2f; se esperaba 7", got)
	}
	if got := stockPorMovimientos(db, 1, "WH-MAIN"); got != 7 {
		t.Errorf("stock del producto de venta en su almacén = %.2f; se esperaba 7", got)
	}
	if got := stockPorMovimientos(db, 7, ""); got != 4 {
		t.Errorf("stock total del suministro = %.2f; se esperaba 4 (6 comprados - 2 consumidos)", got)
	}
	if got := stockPorMovimientos(db, 7, "WH-CONSUMO"); got != 4 {
		t.Errorf("stock del suministro en consumo = %.2f; se esperaba 4", got)
	}
	// El suministro nunca aparece en el almacén de venta.
	if got := stockPorMovimientos(db, 7, "WH-MAIN"); got != 0 {
		t.Errorf("el suministro no puede tener stock en el almacén de venta: %.2f", got)
	}
}

func TestDevolucionAlProveedorRestaUnaSolaVez(t *testing.T) {
	db := nuevaBaseStock(t)
	movimiento(t, db, "purchase", 3, "WH-MAIN", 10)
	// La devolución se guarda en positivo; el signo lo da el tipo de movimiento.
	movimiento(t, db, "return_supplier", 3, "WH-MAIN", 4)
	if got := stockPorMovimientos(db, 3, ""); got != 6 {
		t.Fatalf("stock tras comprar 10 y devolver 4 = %.2f; se esperaba 6", got)
	}
}
