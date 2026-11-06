package web

// Pruebas del vínculo entre los papeles contables y el plan de cuentas de la
// empresa.
//
// Lo que se protege: una cuenta de ingreso no puede ocupar el papel de gasto,
// una cuenta de grupo no puede recibir asientos, y al vincular con el plan
// estándar solo se toman cuentas que existan y sirvan. Una cuenta configurada
// que dejó de existir no se conserva «por si acaso»: se pone en cero y se
// informa, para que la pantalla la muestre.

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

// baseCuentas crea las dos tablas que intervienen y una fila de configuración
// con todos los papeles en cero, para que cada prueba arme su escenario.
func baseCuentas(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE acc_coa (
		HeadCode INTEGER PRIMARY KEY,
		HeadName TEXT NOT NULL DEFAULT '',
		HeadType TEXT NOT NULL DEFAULT 'A',
		IsActive INTEGER NOT NULL DEFAULT 1,
		IsTransaction INTEGER NOT NULL DEFAULT 1
	)`); err != nil {
		t.Fatalf("no se pudo crear acc_coa: %v", err)
	}
	columnas := append(append([]string{}, cuentasColumnasLegacy...), cuentaColumnasNuevas...)
	defs := make([]string, 0, len(columnas))
	for _, c := range columnas {
		defs = append(defs, c+" INTEGER NOT NULL DEFAULT 0")
	}
	if _, err := db.Exec(`CREATE TABLE acc_predefine_account (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		` + strings.Join(defs, ",\n\t\t") + `
	)`); err != nil {
		t.Fatalf("no se pudo crear acc_predefine_account: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO acc_predefine_account (id) VALUES (1)`); err != nil {
		t.Fatalf("no se pudo sembrar la configuración: %v", err)
	}
	return db
}

func cuenta(t *testing.T, db *sql.DB, codigo int64, nombre, tipo string, activa, transaccional int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, HeadType, IsActive, IsTransaction)
		VALUES (?,?,?,?,?)`, codigo, nombre, tipo, activa, transaccional); err != nil {
		t.Fatalf("insertando cuenta %d: %v", codigo, err)
	}
}

func fijarCuenta(t *testing.T, db *sql.DB, columna string, codigo int64) {
	t.Helper()
	if _, err := db.Exec(`UPDATE acc_predefine_account SET `+columna+` = ? WHERE id = 1`, codigo); err != nil {
		t.Fatalf("fijando %s: %v", columna, err)
	}
}

func TestRevisarRolCuentaExigeNaturalezaYMovimiento(t *testing.T) {
	db := baseCuentas(t)
	cuenta(t, db, 3010401, "Ingresos por servicios", "I", 1, 1)
	cuenta(t, db, 4021401, "Gastos de servicios", "E", 1, 1)
	cuenta(t, db, 10201, "Caja (grupo)", "A", 1, 0)
	cuenta(t, db, 4021402, "Gastos de suministros", "E", 0, 1)

	rol := rolesPorClave()["service_expense_code"]

	if err := revisarRolCuenta(db, rol, 4021401); err != nil {
		t.Fatalf("una cuenta de gasto debe servir para gasto de servicios: %v", err)
	}
	err := revisarRolCuenta(db, rol, 3010401)
	if err == nil {
		t.Fatal("una cuenta de INGRESO no puede ocupar el papel de gasto")
	}
	if !strings.Contains(err.Error(), "ingreso") {
		t.Errorf("el error debe decir que la cuenta es de ingreso: %v", err)
	}
	if err := revisarRolCuenta(db, rol, 10201); err == nil || !strings.Contains(err.Error(), "no admite movimientos") {
		t.Errorf("una cuenta de grupo debe rechazarse con su motivo: %v", err)
	}
	if err := revisarRolCuenta(db, rol, 4021402); err == nil || !strings.Contains(err.Error(), "inactiva") {
		t.Errorf("una cuenta inactiva debe rechazarse: %v", err)
	}
	if err := revisarRolCuenta(db, rol, 999999); err == nil || !strings.Contains(err.Error(), "no existe") {
		t.Errorf("una cuenta inexistente debe rechazarse: %v", err)
	}
}

func TestVincularEstandarRespetaLoValidoYCompletaLoQueFalta(t *testing.T) {
	db := baseCuentas(t)
	// Plan estándar disponible.
	cuenta(t, db, 1020101, "Caja", "A", 1, 1)
	cuenta(t, db, 1020501, "Banco", "A", 1, 1)
	cuenta(t, db, 1020801, "Clientes", "A", 1, 1)
	cuenta(t, db, 5020201, "Proveedores", "L", 1, 1)
	cuenta(t, db, 3010301, "Ventas", "I", 1, 1)
	cuenta(t, db, 3010401, "Ingresos por servicios", "I", 1, 1)
	cuenta(t, db, 1020401, "Inventario de mercancía", "A", 1, 1)
	cuenta(t, db, 4010101, "Costo de ventas", "E", 1, 1)
	cuenta(t, db, 1020403, "Inventario de suministros", "A", 1, 1)
	cuenta(t, db, 4021402, "Gasto de suministros", "E", 1, 1)
	cuenta(t, db, 4021401, "Gasto de servicios", "E", 1, 1)
	cuenta(t, db, 1020901, "IVA crédito fiscal", "A", 1, 1)
	cuenta(t, db, 4021101, "Impuestos", "E", 1, 1)
	cuenta(t, db, 4020501, "Sueldos", "E", 1, 1)
	cuenta(t, db, 5020101, "Impuesto estatal", "L", 1, 1)

	// La empresa ya había atado el gasto de servicios a una cuenta propia
	// válida: eso se respeta aunque el estándar diga otra cosa.
	cuenta(t, db, 4021499, "Gastos de servicios (propia)", "E", 1, 1)
	fijarCuenta(t, db, "service_expense_code", 4021499)
	// Y el IVA de compras apuntaba a una cuenta que ya no existe.
	fijarCuenta(t, db, "purchase_vat_code", 888888)

	asignadas, pendientes, err := vincularCuentasEstandar(db)
	if err != nil {
		t.Fatalf("vinculando: %v", err)
	}
	valores, err := leerCuentasRol(db)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if valores["service_expense_code"] != 4021499 {
		t.Errorf("la cuenta propia válida se pisó: %d", valores["service_expense_code"])
	}
	if valores["supplies_inventory_code"] != 1020403 {
		t.Errorf("inventario de suministros = %d; se esperaba 1020403", valores["supplies_inventory_code"])
	}
	if valores["supplies_expense_code"] != 4021402 {
		t.Errorf("gasto de suministros = %d; se esperaba 4021402", valores["supplies_expense_code"])
	}
	if valores["purchase_vat_code"] != 1020901 {
		t.Errorf("IVA de compras = %d; se esperaba 1020901", valores["purchase_vat_code"])
	}
	if len(asignadas) == 0 {
		t.Error("debe informar qué papeles vinculó")
	}
	// Los papeles generales no se tocan ni se reportan si nunca estuvieron
	// configurados: no es una decisión del sistema.
	for _, p := range pendientes {
		if strings.Contains(p, "Compras") || strings.Contains(p, "Sueldos") {
			t.Errorf("un papel general sin configurar no debe reportarse como pendiente: %v", pendientes)
		}
	}
}

func TestVincularEstandarLimpiaCuentaInexistente(t *testing.T) {
	db := baseCuentas(t)
	// El papel de inventario de suministros apunta a una cuenta que ya no está
	// y el plan no tiene el código estándar: debe quedar en cero e informarse.
	fijarCuenta(t, db, "supplies_inventory_code", 777777)
	_, pendientes, err := vincularCuentasEstandar(db)
	if err != nil {
		t.Fatalf("vinculando: %v", err)
	}
	valores, _ := leerCuentasRol(db)
	if valores["supplies_inventory_code"] != 0 {
		t.Errorf("una cuenta inexistente no puede quedar configurada: %d", valores["supplies_inventory_code"])
	}
	informado := false
	for _, p := range pendientes {
		if strings.Contains(p, "Inventario de suministros") {
			informado = true
		}
	}
	if !informado {
		t.Errorf("el papel sin cuenta debe informarse: %v", pendientes)
	}
}

func TestVincularEstandarNoAtaPorCodigoSiElNombreNoCorresponde(t *testing.T) {
	db := baseCuentas(t)
	// El código estándar del IVA de compras existe, es activo y transaccional,
	// pero en este plan significa otra cosa. Atarlo por código asentaría el IVA
	// en una cuenta equivocada: debe quedar pendiente.
	cuenta(t, db, 1020901, "Caja chica", "A", 1, 1)
	_, pendientes, err := vincularCuentasEstandar(db)
	if err != nil {
		t.Fatalf("vinculando: %v", err)
	}
	valores, _ := leerCuentasRol(db)
	if valores["purchase_vat_code"] != 0 {
		t.Errorf("no debe atarse una cuenta que significa otra cosa: %d", valores["purchase_vat_code"])
	}
	informado := false
	for _, p := range pendientes {
		if strings.Contains(p, "IVA crédito fiscal") {
			informado = true
		}
	}
	if !informado {
		t.Errorf("el papel no vinculado debe informarse: %v", pendientes)
	}
}

func TestEstadoCuentasRolDistingueSinAsignarInvalidaYCorrecta(t *testing.T) {
	db := baseCuentas(t)
	cuenta(t, db, 1020401, "Inventario de mercancía", "A", 1, 1)
	fijarCuenta(t, db, "inventoryCode", 1020401)
	fijarCuenta(t, db, "costs_of_good_solds", 555555)
	estados, err := estadoCuentasRol(db)
	if err != nil {
		t.Fatalf("estado: %v", err)
	}
	porClave := map[string]estadoRolCuenta{}
	for _, e := range estados {
		porClave[e.Rol.Clave] = e
	}
	if e := porClave["inventoryCode"]; e.Estado != "ok" {
		t.Errorf("inventario configurado y válido debe estar ok: %+v", e)
	}
	if e := porClave["costs_of_good_solds"]; e.Estado != "invalida" {
		t.Errorf("costo de ventas apuntando a una cuenta inexistente debe estar inválido: %+v", e)
	}
	if e := porClave["supplies_expense_code"]; e.Estado != "sin_asignar" {
		t.Errorf("un papel en cero debe estar sin asignar: %+v", e)
	}
}

func TestVincularEstandarNoCambiaPapelesGeneralesInvalidos(t *testing.T) {
	db := baseCuentas(t)
	fijarCuenta(t, db, "cashCode", 555555) // caja apuntando a una cuenta inexistente
	_, pendientes, err := vincularCuentasEstandar(db)
	if err != nil {
		t.Fatalf("vinculando: %v", err)
	}
	valores, _ := leerCuentasRol(db)
	if valores["cashCode"] != 555555 {
		t.Errorf("un papel general no se reemplaza a ciegas: %d", valores["cashCode"])
	}
	informado := false
	for _, p := range pendientes {
		if p == "Caja" {
			informado = true
		}
	}
	if !informado {
		t.Errorf("la cuenta general inválida debe informarse: %v", pendientes)
	}
}
