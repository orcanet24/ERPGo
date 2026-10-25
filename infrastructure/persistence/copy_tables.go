package persistence

// copy_tables.go — copia las tablas de negocio de una base a otra en el MISMO
// servidor MySQL.
//
// Existe para separar la base de CONTROL (catálogo de empresas, miembros y
// sesiones) de la base de una EMPRESA, en instalaciones antiguas donde el
// esquema de negocio vivía mezclado en la base de control. Se ejecuta UNA vez:
// la siguiente vez, la base de la empresa ya tiene datos y no se vuelve a copiar.
//
// Nada se borra ni se modifica en el origen. La copia es repetible: las tablas
// destino se crean con LIKE y los INSERT son IGNORE, así que un arranque
// interrumpido simplemente continúa en el siguiente.
//
// El transporte es server-side (SELECT ... INTO la otra base), así que no pasa
// ni una fila por el cliente: una base de 2 GB se copia en segundos.

import (
	"database/sql"
	"fmt"
	"strings"
)

// TablasDeControl son las tablas que SIEMPRE viven en la base de control y no
// se copian a la de la empresa. schema_migrations va en la lista porque cada
// base lleva su propio registro de migraciones.
//
// Es exportada porque el verificador de primer arranque (cmd/verificar_default)
// debe usar EXACTAMENTE esta lista para comprobar la separación: si el
// copiador y el verificador discreparan, el verificador aprobaría una
// instalación que el copiador trata distinto.
var TablasDeControl = map[string]bool{
	"tenants":              true,
	"tenant_users":         true,
	"tenant_provision_log": true,
	"web_sessions":         true,
	"schema_migrations":    true,
}

// CopiarTablasEmpresariales copia todas las tablas de negocio de la base de
// control (origen) a la base de la empresa (destino), omitiendo las tablas de
// control. Devuelve cuántas tablas se copiaron.
func CopiarTablasEmpresariales(ctl, empresa *sql.DB) (int, error) {
	var ctlName, empName string
	if err := ctl.QueryRow(`SELECT DATABASE()`).Scan(&ctlName); err != nil {
		return 0, fmt.Errorf("nombre de la base origen: %w", err)
	}
	if err := empresa.QueryRow(`SELECT DATABASE()`).Scan(&empName); err != nil {
		return 0, fmt.Errorf("nombre de la base destino: %w", err)
	}

	rows, err := ctl.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`)
	if err != nil {
		return 0, fmt.Errorf("listando tablas de %s: %w", ctlName, err)
	}
	defer rows.Close()

	var tablas []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tablas = append(tablas, t)
		}
	}
	rows.Close()

	copiadas := 0
	var fallos []string
	for _, t := range tablas {
		if TablasDeControl[strings.ToLower(t)] {
			continue
		}
		qt := quoteIdent(t)
		qOrigen := quoteIdent(ctlName) + "." + qt
		qDestino := quoteIdent(empName) + "." + qt

		if _, err := empresa.Exec("CREATE TABLE IF NOT EXISTS " + qDestino + " LIKE " + qOrigen); err != nil {
			fallos = append(fallos, fmt.Sprintf("%s (crear): %v", t, err))
			continue
		}
		if _, err := empresa.Exec("INSERT IGNORE INTO " + qDestino + " SELECT * FROM " + qOrigen); err != nil {
			fallos = append(fallos, fmt.Sprintf("%s (copiar): %v", t, err))
			continue
		}
		copiadas++
	}

	for _, f := range fallos {
		fmt.Printf("  WARN: tabla no copiada de la base de control: %s\n", f)
	}
	return copiadas, nil
}

// quoteIdent entrecomilla un identificador de MySQL de forma segura.
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
