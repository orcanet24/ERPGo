// Validación y migración del schema.
package setup

import (
	"database/sql"
	"fmt"
	"strings"
)

// SchemaValidator compara el schema esperado con el real y genera ALTERs.
type SchemaValidator struct {
	db *sql.DB
}

// NewSchemaValidator crea un validador para la BD dada.
func NewSchemaValidator(db *sql.DB) *SchemaValidator {
	return &SchemaValidator{db: db}
}

// ColumnInfo describe una columna de una tabla.
type ColumnInfo struct {
	Name    string
	Type    string
	Null    string
	Key     string
	Default string
	Extra   string
}

// GetColumns devuelve las columnas de una tabla.
func (v *SchemaValidator) GetColumns(table string) ([]ColumnInfo, error) {
	rows, err := v.db.Query(fmt.Sprintf("SHOW COLUMNS FROM `%s`", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		if err := rows.Scan(&col.Name, &col.Type, &col.Null, &col.Key, &col.Default, &col.Extra); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// TableExists verifica si una tabla existe.
func (v *SchemaValidator) TableExists(table string) (bool, error) {
	var name string
	err := v.db.QueryRow(
		"SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MigrateResult describe lo que se necesita migrar.
type MigrateResult struct {
	Table      string
	Column     string
	Current    string // "" si no existe
	Expected   string
	Action     string // "add", "modify", "none"
	SQL        string
}

// ValidateTable compara una tabla con su definición esperada.
// expected es un mapa de nombre_columna -> tipo SQL.
func (v *SchemaValidator) ValidateTable(table string, expected map[string]string) ([]MigrateResult, error) {
	exists, err := v.TableExists(table)
	if err != nil {
		return nil, fmt.Errorf("verificando %s: %w", table, err)
	}

	var results []MigrateResult

	if !exists {
		// Tabla no existe: se crea con CREATE TABLE
		var cols []string
		for name, typ := range expected {
			cols = append(cols, fmt.Sprintf("`%s` %s", name, typ))
		}
		results = append(results, MigrateResult{
			Table:    table,
			Action:   "create",
			SQL:      fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (%s)", table, strings.Join(cols, ", ")),
		})
		return results, nil
	}

	// Tabla existe: verificar columnas
	cols, err := v.GetColumns(table)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]bool)
	for _, c := range cols {
		existing[strings.ToLower(c.Name)] = true
	}

	for name, typ := range expected {
		if !existing[strings.ToLower(name)] {
			results = append(results, MigrateResult{
				Table:    table,
				Column:   name,
				Expected: typ,
				Action:   "add",
				SQL:      fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN IF NOT EXISTS `%s` %s", table, name, typ),
			})
		}
	}

	return results, nil
}

// ValidateControlSchema valida el schema de control y devuelve migraciones necesarias.
func (v *SchemaValidator) ValidateControlSchema() ([]MigrateResult, error) {
	expected := map[string]string{
		"code":             "VARCHAR(64) PRIMARY KEY",
		"legal_name":       "VARCHAR(255) NOT NULL DEFAULT ''",
		"rif":              "VARCHAR(32) NOT NULL DEFAULT ''",
		"status":           "INTEGER NOT NULL DEFAULT 1",
		"created_at":       "VARCHAR(32) NOT NULL DEFAULT ''",
		"db_user":          "VARCHAR(128) NOT NULL DEFAULT ''",
		"db_password_enc":  "VARBINARY(512) DEFAULT ''",
		"max_users":        "INTEGER NOT NULL DEFAULT 0",
		"created_by":       "VARCHAR(128) NOT NULL DEFAULT ''",
		"db_size_mb":       "INTEGER NOT NULL DEFAULT 0",
	}
	return v.ValidateTable("tenants", expected)
}

// ValidateAll valida el schema completo del sistema.
func (v *SchemaValidator) ValidateAll() ([]MigrateResult, error) {
	return v.ValidateControlSchema()
}

// ExecuteMigration ejecuta una migración.
func (v *SchemaValidator) ExecuteMigration(m MigrateResult) error {
	_, err := v.db.Exec(m.SQL)
	return err
}
