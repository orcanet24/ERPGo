package web

// period_close_test.go — La prueba que faltaba (F-1, auditoría 2026-11-22).
//
// El cierre de período NO tenía ninguna prueba, y por eso el bug vivió: la
// validación comparaba SUM(Debit)−SUM(Credit) sobre acc_vaucher, donde los
// asientos automáticos se guardan como PATAS SUELTAS con la contra-cuenta
// declarada en RevCodde (la doble entrada se materializa en el libro
// acc_transaction al aprobar). Con un solo asiento automático el cierre
// declaraba descuadrado el mes y ningún período con facturas podía firmarse.
// Estas pruebas fijan el modelo correcto: el cuadre vive en el LIBRO, los
// pendientes se exigen cerrados, y la forma de cada fila se comprueba según
// las dos morfologías legítimas (pata única con RevCodde, o fila manual
// equilibrada d == c).

import (
	"database/sql"
	"strings"
	"testing"
)

func nuevaBaseCierre(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := basePrueba(t)
	ddl := []string{
		`CREATE TABLE acc_vaucher (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			VNo TEXT NOT NULL DEFAULT '', Vtype TEXT, ReferenceNo TEXT, VDate TEXT,
			COAID INTEGER DEFAULT 0, RevCodde INTEGER DEFAULT 0,
			Debit REAL DEFAULT 0, Credit REAL DEFAULT 0,
			isApproved INTEGER DEFAULT 0, status INTEGER DEFAULT 0,
			Narration TEXT, ledgerComment TEXT, subType INTEGER DEFAULT 0, subCode INTEGER)`,
		`CREATE TABLE acc_transaction (
			ID INTEGER PRIMARY KEY AUTOINCREMENT,
			vid INTEGER DEFAULT 0, fyear TEXT, VNo TEXT, Vtype TEXT, ReferenceNo TEXT, VDate TEXT,
			COAID INTEGER DEFAULT 0, Debit REAL DEFAULT 0, Credit REAL DEFAULT 0,
			IsPosted INTEGER DEFAULT 1, IsAppove INTEGER DEFAULT 1, Narration TEXT)`,
		`CREATE TABLE acc_coa (id INTEGER PRIMARY KEY AUTOINCREMENT, HeadCode INTEGER UNIQUE, HeadName TEXT, HeadType TEXT)`,
		`CREATE TABLE invoice (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice INTEGER DEFAULT 0, invoice_id TEXT, date TEXT,
			total_amount REAL DEFAULT 0, total_vat_amnt REAL DEFAULT 0, total_tax REAL DEFAULT 0,
			is_draft TEXT DEFAULT 'no', is_credit INTEGER DEFAULT 0, payment_type INTEGER DEFAULT 1)`,
		`CREATE TABLE acc_period_close (
			period TEXT PRIMARY KEY, status TEXT, closed_at TEXT, closed_by TEXT,
			reopened_at TEXT, reopened_by TEXT, reopen_reason TEXT)`,
	}
	for _, d := range ddl {
		if _, err := db.Exec(d); err != nil {
			t.Fatalf("creando esquema de cierre: %v", err)
		}
	}
	return s, db
}

// validationBlocking devuelve el problema de la validación con el código dado
// ("" si pasó) y si era bloqueante.
func validationBlocking(res *PeriodCheckResult, code string) (string, bool) {
	for _, v := range res.Validations {
		if v.Code == code {
			return v.Problem, v.Blocking && v.Problem != ""
		}
	}
	return "", false
}

// TestCierreCuadraConAsientoAutomaticoDePataUnica: el caso F-1 puro. Un
// asiento de venta guardado como pata única (Debit 116, Credit 0, aprobado)
// con sus dos mitades publicadas en el libro debe dejar cerrar el mes.
// El código antiguo lo declaraba descuadrada siempre.
func TestCierreCuadraConAsientoAutomaticoDePataUnica(t *testing.T) {
	s, db := nuevaBaseCierre(t)

	if _, err := db.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, HeadType) VALUES (1020801, 'Customer Receivable', 'A')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, HeadType) VALUES (3010301, 'Sales Account', 'I')`); err != nil {
		t.Fatal(err)
	}
	// La pata en acc_vaucher, tal y como la escribe invoiceSave: débito con
	// contra declarada, crédito en cero.
	if _, err := db.Exec(`INSERT INTO acc_vaucher (VNo, Vtype, ReferenceNo, VDate, COAID, RevCodde, Debit, Credit, isApproved, status, Narration)
		VALUES ('CV-1', 'CV', '2026', '2026-11-05', 1020801, 3010301, 116, 0, 1, 1, 'Sales Voucher')`); err != nil {
		t.Fatal(err)
	}
	// El libro, como lo materializa approveVoucher: las dos mitades.
	if _, err := db.Exec(`INSERT INTO acc_transaction (VNo, Vtype, ReferenceNo, VDate, COAID, Debit, Credit, IsPosted)
		VALUES ('CV-1', 'CV', '2026', '2026-11-05', 1020801, 116, 0, 1),
		       ('CV-1', 'CV', '2026', '2026-11-05', 3010301, 0, 116, 1)`); err != nil {
		t.Fatal(err)
	}
	// La factura con su asiento puesto (checkInvoicesPosted une por
	// invoice.invoice y acc_vaucher.ReferenceNo).
	if _, err := db.Exec(`INSERT INTO invoice (invoice, invoice_id, date, total_amount, is_draft)
		VALUES (2026, '2026', '2026-11-05', 116, 'no')`); err != nil {
		t.Fatal(err)
	}

	r := reqPrueba(s, sessPrueba("master"), "/period/check?period=202611")
	res, err := s.CheckPeriod(r, "202611")
	if err != nil {
		t.Fatalf("CheckPeriod: %v", err)
	}
	if problema, bloquea := validationBlocking(res, "balance"); bloquea {
		t.Fatalf("F-1 sigue vivo: el cierre de un mes con asiento automático se declara descuadrado (%s)", problema)
	}
	if problema, bloquea := validationBlocking(res, "voucher_balance"); bloquea {
		t.Fatalf("la pata única legítima fue marcada como mal formada: %s", problema)
	}
	if problema, bloquea := validationBlocking(res, "invoice_asiento"); bloquea {
		t.Fatalf("la factura con su asiento publicada sigue apareciendo sin asiento: %s", problema)
	}
	if !res.CanClose {
		t.Fatalf("el período debería poder cerrarse; validaciones: %+v", res.Validations)
	}
	if err := s.ClosePeriod(r, "202611", "master"); err != nil {
		t.Fatalf("ClosePeriod: %v", err)
	}
	var estado string
	if err := db.QueryRow(`SELECT status FROM acc_period_close WHERE period='202611'`).Scan(&estado); err != nil {
		t.Fatalf("el cierre no quedó registrado: %v", err)
	}
	if estado != "closed" {
		t.Errorf("estado %q, quería closed", estado)
	}
}

// TestNoCierraConAsientosPendientes: el libro no puede firmarse con work en
// curso (la doble entrada del pendiente aún no está publicada).
func TestNoCierraConAsientosPendientes(t *testing.T) {
	s, db := nuevaBaseCierre(t)

	if _, err := db.Exec(`INSERT INTO acc_vaucher (VNo, Vtype, ReferenceNo, VDate, COAID, RevCodde, Debit, Credit, isApproved, status)
		VALUES ('CV-9', 'CV', '2027', '2026-11-06', 1020801, 3010301, 50, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	// Y su espejo aprobado para que el libro cuadre pese al pendiente.
	if _, err := db.Exec(`INSERT INTO acc_transaction (VNo, Vtype, ReferenceNo, VDate, COAID, Debit, Credit, IsPosted)
		VALUES ('CV-1', 'CV', '1', '2026-11-01', 1, 10, 0, 1),
		       ('CV-1', 'CV', '1', '2026-11-01', 2, 0, 10, 1)`); err != nil {
		t.Fatal(err)
	}

	r := reqPrueba(s, sessPrueba("master"), "/period/check?period=202611")
	res, err := s.CheckPeriod(r, "202611")
	if err != nil {
		t.Fatal(err)
	}
	if problema, bloquea := validationBlocking(res, "pendientes"); !bloquea || !strings.Contains(problema, "1 asiento") {
		t.Fatalf("el pendiente no bloqueó el cierre: %q", problema)
	}
	if err := s.ClosePeriod(r, "202611", "master"); err == nil || !strings.Contains(err.Error(), "pendiente") {
		t.Fatalf("ClosePeriod debería rechazar pendientes, obtuvo: %v", err)
	}
}

// TestCierraSoloLibroDescuadrado: un libro partido a medias (una sola mitad
// publicada) ES el descuadre real que la comprobación debe atrapar.
func TestCierraSoloLibroDescuadrado(t *testing.T) {
	s, db := nuevaBaseCierre(t)

	if _, err := db.Exec(`INSERT INTO acc_transaction (VNo, Vtype, ReferenceNo, VDate, COAID, Debit, Credit, IsPosted)
		VALUES ('JV-7', 'JV', '7', '2026-11-03', 5, 100, 0, 1)`); err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, sessPrueba("master"), "/period/check?period=202611")
	res, err := s.CheckPeriod(r, "202611")
	if err != nil {
		t.Fatal(err)
	}
	if problema, bloquea := validationBlocking(res, "balance"); !bloquea {
		t.Fatalf("un libro con solo un débito de 100 debería bloquear; problema=%q", problema)
	}
}

// TestFilaMixtaDescuadradaSaleEnVoucherBalance: la fila mal formada (débito y
// crédito a la vez desiguales) se reporta; la manual equilibrada no.
func TestFilaMixtaDescuadradaSaleEnVoucherBalance(t *testing.T) {
	s, db := nuevaBaseCierre(t)

	if _, err := db.Exec(`INSERT INTO acc_vaucher (VNo, Vtype, ReferenceNo, VDate, COAID, RevCodde, Debit, Credit, isApproved, status)
		VALUES ('JV-8', 'JV', '8', '2026-11-04', 1, 2, 70, 50, 1, 1),
		       ('JV-8', 'JV', '8', '2026-11-04', 3, 4, 20, 20, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	// Libro equilibrado (120/120) para que la validación 1 no opaque la 2.
	if _, err := db.Exec(`INSERT INTO acc_transaction (VNo, Vtype, ReferenceNo, VDate, COAID, Debit, Credit, IsPosted)
		VALUES ('JV-8', 'JV', '8', '2026-11-04', 1, 70, 0, 1),
		       ('JV-8', 'JV', '8', '2026-11-04', 2, 0, 50, 1),
		       ('JV-8', 'JV', '8', '2026-11-04', 3, 20, 0, 1),
		       ('JV-8', 'JV', '8', '2026-11-04', 4, 0, 20, 1)`); err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, sessPrueba("master"), "/period/check?period=202611")
	res, err := s.CheckPeriod(r, "202611")
	if err != nil {
		t.Fatal(err)
	}
	if problema, bloquea := validationBlocking(res, "voucher_balance"); !bloquea || !strings.Contains(problema, "JV-8") {
		t.Fatalf("la fila mixta no fue reportada: %q", problema)
	}
}
