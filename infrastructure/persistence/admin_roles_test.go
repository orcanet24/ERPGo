package persistence

// admin_roles_test.go — La siembra del administrador no puede degradar al
// dueño de la instalación.
//
// El fallo que se protege, medido en una instalación limpia: la migración de
// arranque creaba el rol master con el id 3 (schema.sql deja AUTO_INCREMENT=3)
// y el asistente reutilizaba ese id como «Manager» después de que la siembra
// hubiera atado al administrador a él. El dueño entraba como manager, sin
// matriz de permisos, y el menú se quedaba en Dashboard + Gestion Inventario.
//
// Estas pruebas fijan el contrato que lo evita: pase lo que pase con los ids
// previos, tras sembrar al administrador hay UN solo rol master, es el id 1, y
// el usuario 1 está atado a él.

import (
	"database/sql"
	"strings"
	"testing"
)

func baseSiembraAdmin(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("abrir SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ddl := []string{
		`CREATE TABLE users (user_id TEXT PRIMARY KEY, first_name TEXT, last_name TEXT,
			company_name TEXT, phone TEXT, status INTEGER)`,
		`CREATE TABLE user_login (user_id TEXT, username TEXT, password TEXT,
			user_type INTEGER, status INTEGER)`,
		`CREATE TABLE company_information (company_id TEXT, company_name TEXT, email TEXT,
			address TEXT, mobile TEXT, website TEXT, status INTEGER)`,
		`CREATE TABLE web_setting (setting_id INTEGER, logo TEXT, invoice_logo TEXT,
			favicon TEXT, currency TEXT, timezone TEXT, currency_position TEXT,
			footer_text TEXT, language TEXT, rtr TEXT, captcha INTEGER, is_qr INTEGER,
			is_autoapprove_v INTEGER, discount_type INTEGER)`,
		`CREATE TABLE financial_year (yearName INTEGER, startDate TEXT, endDate TEXT,
			isCloseReq INTEGER, status INTEGER, created_by INTEGER)`,
		`CREATE TABLE vat_tax_setting (dynamic_tax INTEGER, fixed_tax INTEGER)`,
		`CREATE TABLE print_setting (header INTEGER, footer INTEGER)`,
		`CREATE TABLE currency_tbl (currency_name TEXT, icon TEXT)`,
		`CREATE TABLE module (id INTEGER PRIMARY KEY, name TEXT, status INTEGER)`,
		`CREATE TABLE sub_module (id INTEGER PRIMARY KEY, mid TEXT, name TEXT,
			directory TEXT, status INTEGER)`,
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY, type TEXT, level TEXT,
			description TEXT, is_system INTEGER)`,
		`CREATE TABLE sec_userrole (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT,
			roleid INTEGER, createby TEXT, createdate TEXT)`,
		`CREATE TABLE customer_information (customer_id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT, customer_mobile TEXT, status INTEGER, create_by TEXT)`,
		`CREATE TABLE acc_coa (HeadCode INTEGER, HeadName TEXT, PHeadName TEXT,
			HeadLevel INTEGER, IsActive INTEGER, IsTransaction INTEGER, IsGL INTEGER,
			HeadType TEXT, IsBudget INTEGER, IsDepreciation INTEGER, DepreciationRate REAL,
			customer_id INTEGER, CreateBy TEXT, CreateDate TEXT, UpdateBy TEXT,
			UpdateDate TEXT, isSubType INTEGER, subType INTEGER)`,
		`CREATE TABLE acc_subcode (subTypeId INTEGER, name TEXT, referenceNo TEXT,
			status INTEGER, created_by INTEGER, created_date TEXT, updated_by INTEGER)`,
	}
	for _, s := range ddl {
		if _, err := db.Exec(strings.Join(strings.Fields(s), " ")); err != nil {
			t.Fatalf("crear tabla: %v\n%s", err, s)
		}
	}
	return db
}

// exigirUnSoloMaster comprueba el estado final: un único master, el canónico,
// y el administrador atado a él.
func exigirUnSoloMaster(t *testing.T, db *sql.DB) {
	t.Helper()

	var masters int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sec_role WHERE level = 'master'`).Scan(&masters); err != nil {
		t.Fatal(err)
	}
	if masters != 1 {
		t.Fatalf("debe quedar un solo rol master, hay %d", masters)
	}

	var masterID int64
	if err := db.QueryRow(`SELECT id FROM sec_role WHERE level = 'master'`).Scan(&masterID); err != nil {
		t.Fatal(err)
	}
	if masterID != 1 {
		t.Errorf("el master canónico es el id 1, quedó el %d", masterID)
	}

	var rolAdmin int64
	if err := db.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = '1'`).Scan(&rolAdmin); err != nil {
		t.Fatalf("el administrador no quedó atado a ningún rol: %v", err)
	}
	if rolAdmin != 1 {
		t.Errorf("el administrador debe quedar atado al master 1, quedó al %d", rolAdmin)
	}

	// Los otros roles canónicos conservan su nivel: nadie los degradó por error.
	for _, r := range []struct {
		id    int
		nivel string
	}{{2, "admin"}, {3, "manager"}, {4, "employee"}} {
		var nivel string
		if err := db.QueryRow(`SELECT level FROM sec_role WHERE id = ?`, r.id).Scan(&nivel); err != nil {
			t.Fatalf("falta el rol canónico %d: %v", r.id, err)
		}
		if nivel != r.nivel {
			t.Errorf("rol %d: nivel %q, se esperaba %q", r.id, nivel, r.nivel)
		}
	}
}

// TestSembrarAdminNormalizaUnMasterHeredado cubre el estado que deja la
// migración en MySQL: el rol master nació con id 3 y no hay usuarios todavía.
func TestSembrarAdminNormalizaUnMasterHeredado(t *testing.T) {
	db := baseSiembraAdmin(t)
	if _, err := db.Exec(`INSERT INTO sec_role (id, type, level, description, is_system)
		VALUES (3, 'super', 'master', 'Super Administrador del sistema', 1)`); err != nil {
		t.Fatal(err)
	}

	if err := InitializeAdminUser(db, "sqlite", "admin", "clave-de-prueba"); err != nil {
		t.Fatalf("la siembra falló sobre un master heredado: %v", err)
	}
	exigirUnSoloMaster(t, db)
}

// TestSembrarAdminRespetaLosRolesCanonicos cubre el orden nuevo del asistente:
// los roles 1..4 ya están sembrados antes de atar al administrador. La siembra
// no debe fallar por el id 1 ocupado ni crear un segundo master.
func TestSembrarAdminRespetaLosRolesCanonicos(t *testing.T) {
	db := baseSiembraAdmin(t)
	for _, r := range []struct {
		id    int
		tipo  string
		nivel string
	}{
		{1, "Super Admin", "master"},
		{2, "Admin", "admin"},
		{3, "Manager", "manager"},
		{4, "Employee", "employee"},
	} {
		if _, err := db.Exec(`INSERT INTO sec_role (id, type, level, description, is_system)
			VALUES (?, ?, ?, '', 1)`, r.id, r.tipo, r.nivel); err != nil {
			t.Fatal(err)
		}
	}

	if err := InitializeAdminUser(db, "sqlite", "admin", "clave-de-prueba"); err != nil {
		t.Fatalf("la siembra falló con los roles canónicos ya presentes: %v", err)
	}
	exigirUnSoloMaster(t, db)
}
