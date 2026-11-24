package persistence

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// randomPassword genera la contraseña de respaldo cuando nadie la proveyó.
//
// Se usa crypto/rand y no math/rand: esta clave abre un ERP con datos fiscales
// y predecirla es abrir la empresa. La generación es solo para el arranque sin
// asistente (siembra automática, migración); el flujo normal siempre recibe la
// contraseña que escribió el usuario en el asistente.
func randomPassword() (string, error) {
	const alfabeto = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const largo = 16
	out := make([]byte, largo)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alfabeto))))
		if err != nil {
			return "", err
		}
		out[i] = alfabeto[n.Int64()]
	}
	return string(out), nil
}

// insertIgnoreSQL returns the dialect-appropriate idempotent INSERT prefix.
//
// SQLite no conoce INSERT IGNORE (su forma equivalente es INSERT OR IGNORE):
// devolver el mismo string para los dos motores era otra de las copias de la
// mentira «SQLite único» (D-10, auditoría 2026-11-22) — sobre SQLite estas
// siembras habrían muerto en error de sintaxis.
func insertIgnoreSQL(dialect string) string {
	if dialect == "sqlite" {
		return "INSERT OR IGNORE INTO "
	}
	return "INSERT IGNORE INTO "
}

// InitializeAdminUser creates the default data if the DB is empty.
// The onboarding wizard MUST provide credentials — there is no default password.
// The system generates a strong random one only as a last resort (e.g. automated testing).
func InitializeAdminUser(db *sql.DB, dialect, username, password string) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check users: %w", err)
	}
	if count > 0 {
		return nil
	}

	// Un administrador sin nombre de usuario no es un administrador: es una
	// fila que nadie puede usar para entrar. Y como `users` queda con un
	// registro, esta función nunca vuelve a entrar (count > 0), así que el
	// sistema se queda sin forma de acceder para siempre. Pasó: el arranque
	// siembra la empresa «default» con PrepareCompanyAs(db, ..., "", "") y
	// creaba un admin con username vacío.
	//
	// Quien llame sin credenciales (siembra automática, migración) tiene que
	// recibir un admin usable: se generan unas y se imprimen UNA vez.
	if strings.TrimSpace(username) == "" {
		username = "admin"
	}
	if strings.TrimSpace(password) == "" {
		generada, err := randomPassword()
		if err != nil {
			return fmt.Errorf("no se pudo generar la contraseña del administrador: %w", err)
		}
		password = generada
		fmt.Printf("=== ADMIN USER CREATED ===\nUsername: %s\nPassword: %s\n"+
			"=============================\n"+
			"IMPORTANT: change this password after first login.\n", username, password)
	}

	fmt.Printf("No users found. Creating admin user %q...\n", username)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// users + user_login (user_id=1, admin)
	if _, err := tx.Exec(`INSERT INTO users (user_id, first_name, last_name, company_name, phone, status)
		VALUES ('1', 'System', 'Admin', 'ERPGo', '0000000000', 1)`); err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
		VALUES ('1', ?, ?, 1, 1)`, username, string(hashedPassword)); err != nil {
		return fmt.Errorf("failed to insert login: %w", err)
	}

	// company
	if _, err := tx.Exec(`INSERT INTO company_information (company_id, company_name, email, address, mobile, website, status)
		VALUES ('1', 'Mi Empresa', 'info@empresa.local', 'Dirección principal', '+0000000000', '', 1)`); err != nil {
		return fmt.Errorf("failed to insert company: %w", err)
	}

	// web settings
	if _, err := tx.Exec(`INSERT INTO web_setting (setting_id, logo, invoice_logo, favicon, currency, timezone,
		currency_position, footer_text, language, rtr, captcha, is_qr, is_autoapprove_v, discount_type)
		VALUES (1, '', '', '', '$', 'America/Caracas', '0', 'ERPGo', 'spanish', '0', 1, 0, 1, 1)`); err != nil {
		return fmt.Errorf("failed to insert web_setting: %w", err)
	}

	// financial year (current)
	fy := time.Now().Year()
	if _, err := tx.Exec(`INSERT INTO financial_year (yearName, startDate, endDate, isCloseReq, status, created_by)
		VALUES (?, ?, ?, 0, 1, 1)`, fy, fmt.Sprintf("%d-01-01", fy), fmt.Sprintf("%d-12-31", fy)); err != nil {
		return fmt.Errorf("failed to insert financial_year: %w", err)
	}

	// vat tax setting: fixed
	if _, err := tx.Exec(`INSERT INTO vat_tax_setting (dynamic_tax, fixed_tax) VALUES (0, 1)`); err != nil {
		return fmt.Errorf("failed to insert vat_tax_setting: %w", err)
	}

	// print setting
	if _, err := tx.Exec(`INSERT INTO print_setting (header, footer) VALUES (200, 100)`); err != nil {
		return fmt.Errorf("failed to insert print_setting: %w", err)
	}

	// currency
	if _, err := tx.Exec(`INSERT INTO currency_tbl (currency_name, icon) VALUES ('USD', '$')`); err != nil {
		return fmt.Errorf("failed to insert currency: %w", err)
	}

	// modules (mirror PHP install)
	modules := []string{"invoice", "customer", "product", "supplier", "purchase", "stock", "return",
		"report", "accounts", "bank", "tax", "hrm_management", "service", "setting", "quotation"}
	for i, m := range modules {
		if _, err := tx.Exec(insertIgnoreSQL(dialect)+"module (id, name, status) VALUES (?,?,1)", i+1, m); err != nil {
			return fmt.Errorf("failed to insert module %s: %w", m, err)
		}
	}

	// sub modules (directory keys used by permissionMap)
	subModules := [][3]string{ // {mid, name, directory}
		{"1", "new_invoice", "new_invoice"},
		{"1", "manage_invoice", "manage_invoice"},
		{"1", "pos_invoice", "gui_pos"},
		{"2", "add_customer", "add_customer"},
		{"2", "manage_customer", "manage_customer"},
		{"2", "credit_customer", "credit_customer"},
		{"2", "paid_customer", "paid_customer"},
		{"2", "customer_ledger", "customer_ledger"},
		{"2", "customer_advance", "customer_advance"},
		{"3", "category", "category"},
		{"3", "unit", "unit"},
		{"3", "add_product", "create_product"},
		{"3", "manage_product", "manage_product"},
		{"4", "add_supplier", "add_supplier"},
		{"4", "manage_supplier", "manage_supplier"},
		{"4", "supplier_ledger", "supplier_ledger"},
		{"5", "add_purchase", "add_purchase"},
		{"5", "manage_purchase", "manage_purchase"},
		{"6", "stock_report", "stock_report"},
		{"9", "coa", "coa"},
		{"9", "vouchers", "vouchers"},
		{"9", "ledger", "ledger"},
		{"8", "sales_report", "sales_report"},
		{"8", "purchase_report", "purchase_report"},
		{"14", "setting", "setting"},
		{"14", "users", "users"},
		{"14", "roles", "roles"},
		{"14", "backup", "backup"},
		// New modules (ids 29-45 must match migrations/schema_menu_permissions.sql,
		// which inserts the same rows with INSERT IGNORE when upgrading a database
		// migrated from PHP; mid values must match the module names above).
		{"13", "create_service", "create_service"},
		{"13", "manage_service", "manage_service"},
		{"7", "manage_return", "manage_return"},
		{"15", "create_quotation", "create_quotation"},
		{"15", "manage_quotation", "manage_quotation"},
		{"10", "bank", "bank"},
		{"10", "bank_ledger", "bank_ledger"},
		{"6", "adjustment", "adjustment"},
		{"8", "legal_books", "legal_books"},
		{"8", "kardex", "kardex"},
		{"14", "audit_log", "audit_log"},
		{"14", "exchange_rate", "exchange_rate"},
		// RRHH (mid 12 = hrm_management). "publicar nomina" va separada de
		// "ver nomina": publicar paga dinero real, y el permiso alto es lo
		// único que separa mirar una corrida de liquidarla.
		{"12", "hr_employee", "hr_employee"},
		{"12", "hr_payroll", "hr_payroll"},
		{"12", "hr_payroll_post", "hr_payroll_post"},
		{"12", "hr_tax_rates", "hr_tax_rates"},
		{"12", "crm", "crm"},
	}
	for i, sm := range subModules {
		mid := sm[0]
		if mid == "0" {
			mid = "2"
		}
		if _, err := tx.Exec(insertIgnoreSQL(dialect)+"sub_module (id, mid, name, directory, status) VALUES (?,?,?,?,1)",
			i+1, mid, sm[1], sm[2]); err != nil {
			return fmt.Errorf("failed to insert sub_module %s: %w", sm[2], err)
		}
	}

	// ROLES CANÓNICOS, ANTES DE ATAR AL ADMINISTRADOR.
	//
	// El id es parte del contrato del catálogo: 1 master, 2 admin, 3 manager,
	// 4 employee. La siembra los deja en esa forma aunque la migración de
	// arranque (schema_z_admin_master.sql) ya haya creado un rol master con
	// otro id: en ese momento NO hay usuarios a los que atar, así que
	// reescribirlos no degrada a nadie.
	//
	// POR QUÉ NO SE INSERTAN A CIEGAS. Antes esta función hacía
	// `INSERT INTO sec_role (id,type) VALUES (1,'admin'),(2,'usuario')` y
	// añadía un master nuevo sin comprobar si ya existía. En una instalación
	// limpia el master de la migración nacía con id 3 (AUTO_INCREMENT=3 de
	// schema.sql), y el asistente reutilizaba después ese id como «Manager»
	// —con el administrador ya atado a él—: el dueño entraba como manager,
	// sin matriz de permisos, y solo veía el menú público (Dashboard y Gestion
	// Inventario). Sembrar aquí, idempotente y en el mismo orden que el resto
	// del catálogo, elimina esa carrera entre sembradores.
	rolesBase := []struct {
		id    int
		tipo  string
		nivel string
		desc  string
	}{
		{1, "Super Admin", "master", "Acceso total al sistema"},
		{2, "Admin", "admin", "Administrador de empresa"},
		{3, "Manager", "manager", "Gerente"},
		{4, "Employee", "employee", "Empleado"},
	}
	for _, r := range rolesBase {
		var existe int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sec_role WHERE id = ?`, r.id).Scan(&existe); err != nil {
			return fmt.Errorf("comprobando el rol %q: %w", r.tipo, err)
		}
		if existe == 0 {
			if _, err := tx.Exec(`INSERT INTO sec_role (id, type, level, description, is_system)
				VALUES (?, ?, ?, ?, 1)`, r.id, r.tipo, r.nivel, r.desc); err != nil {
				return fmt.Errorf("sembrando el rol %q: %w", r.tipo, err)
			}
			continue
		}
		if _, err := tx.Exec(`UPDATE sec_role SET type = ?, level = ?, description = ?, is_system = 1
			WHERE id = ?`, r.tipo, r.nivel, r.desc, r.id); err != nil {
			return fmt.Errorf("normalizando el rol %q: %w", r.tipo, err)
		}
	}

	// EL ADMIN INICIAL NACE CON PODER DE VERDAD (bootstrap, auditoría
	// 2026-11-23): las rutas de sistema (/ai, /users, /tenants) exigen nivel
	// 'master'. Se ata al rol canónico 1 en la MISMA transacción: semilla y
	// autoridad no pueden quedar a mitades.
	const masterRoleID = 1
	if _, err := tx.Exec(`INSERT INTO sec_userrole (user_id, roleid, createby, createdate)
		VALUES ('1', ?, 'bootstrap', ?)`, masterRoleID, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		return fmt.Errorf("no se pudo atar el rol maestro al administrador: %w", err)
	}

	// walking customer
	res, err := tx.Exec(`INSERT INTO customer_information (customer_name, customer_mobile, status, create_by)
		VALUES ('Walking Customer', '', 1, '1')`)
	if err == nil {
		cid, _ := res.LastInsertId()
		var maxHC sql.NullInt64
		_ = tx.QueryRow(`SELECT MAX(HeadCode) FROM acc_coa WHERE HeadLevel=4 AND HeadCode LIKE '113100%'`).Scan(&maxHC)
		headcode := int64(113100000001)
		if maxHC.Valid && maxHC.Int64 > 0 {
			headcode = maxHC.Int64 + 1
		}
		// UpdateBy/UpdateDate y updated_by NO son opcionales: el DDL canónico los
		// declara NOT NULL sin DEFAULT (Error 1364). En la creación valen lo mismo
		// que su contraparte CreateBy/created_by (usuario 1: el administrador).
		if _, err := tx.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, PHeadName, HeadLevel, IsActive, IsTransaction,
			IsGL, HeadType, IsBudget, IsDepreciation, DepreciationRate, customer_id, CreateBy, CreateDate, UpdateBy, UpdateDate, isSubType, subType)
			VALUES (?,?,?,?,1,1,0,'A',0,0,0,?,?,?,?,?,0,1)`,
			headcode, fmt.Sprintf("%d-Walking Customer", cid), "Customer Receivable", 4, cid, "1", now(), "1", now()); err != nil {
			log.Printf("admin: no se pudo crear la cuenta contable del cliente genérico: %v", err)
		}
		if _, err := tx.Exec(`INSERT INTO acc_subcode (subTypeId, name, referenceNo, status, created_by, created_date, updated_by)
			VALUES (3, 'Walking Customer', ?, 1, 1, ?, 1)`, cid, time.Now().Format("2006-01-02")); err != nil {
			log.Printf("admin: no se pudo crear la subcuenta del cliente genérico: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Println("=== ADMIN USER CREATED ===")
	fmt.Println("Username: " + username)
	fmt.Println("=============================")
	fmt.Println("IMPORTANT: change this password after first login.")
	return nil
}

func now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
