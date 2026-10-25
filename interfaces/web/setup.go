package web

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"erpgow/configs"
	"erpgow/infrastructure/persistence"
)

// nombreTenantMaestro devuelve el código de la primera empresa configurada.
func nombreTenantMaestro(cfg *configs.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.DBMasterTenantCode) == "" {
		return defaultTenantCode
	}
	return strings.TrimSpace(cfg.DBMasterTenantCode)
}

// PrepararEmpresaMaestra aplica el esquema a DB_NAME, deja listas las tablas de
// negocio e identidad y registra el tenant que apunta a esa misma base. No crea
// una segunda base para "default".
func PrepararEmpresaMaestra(cfg *configs.Config, db *sql.DB) error {
	if cfg == nil || db == nil {
		return fmt.Errorf("configuración o conexión maestra nula")
	}
	if err := persistence.RunMigrationsForMySQL(db, persistence.ConfigMySQL{
		Host: cfg.DBHost, Port: cfg.DBPort, Usuario: cfg.DBUser,
		Clave: cfg.DBPassword, Nombre: cfg.DBName,
	}); err != nil {
		return fmt.Errorf("migrando DB_NAME (base maestra): %w", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("comprobando identidad maestra: %w", err)
	}
	if count == 0 {
		if err := persistence.InitializeAdminUser(db, "mysql", "", ""); err != nil {
			return fmt.Errorf("sembrando identidad maestra: %w", err)
		}
	}

	var legalName, rif string
	_ = db.QueryRow(`SELECT company_name, IFNULL(vat_no,'') FROM company_information WHERE status=1 ORDER BY company_id LIMIT 1`).Scan(&legalName, &rif)
	if strings.TrimSpace(legalName) == "" {
		legalName = "Mi Empresa"
	}
	code := nombreTenantMaestro(cfg)
	if _, err := db.Exec(`INSERT INTO tenants (code, legal_name, rif, status, created_at) VALUES (?, ?, ?, 1, NOW())
		ON DUPLICATE KEY UPDATE status=1`, code, legalName, rif); err != nil {
		return fmt.Errorf("registrando empresa maestra %q: %w", code, err)
	}
	rows, err := db.Query(`SELECT username FROM user_login WHERE user_type=1 AND status=1`)
	if err != nil {
		return fmt.Errorf("listando administradores de empresa maestra: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return err
		}
		if strings.TrimSpace(username) == "" {
			continue
		}
		if _, err := db.Exec(`INSERT IGNORE INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at)
			VALUES (?, ?, 0, 1, 1, NOW())`, code, username); err != nil {
			return fmt.Errorf("registrando dueño %q de empresa maestra: %w", username, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("empresa maestra %q lista en DB_NAME=%q", code, cfg.DBName)
	return nil
}
