package web

import (
	"database/sql"

	"erpgo/infrastructure/persistence"
)

// AplicarEsquemasDeCodigo aplica las tablas que el código crea fuera de
// migrations/*.sql y que TODA instalación tiene: la BD de control (tenants,
// tenant_users, tenant_provision_log, web_sessions, addon_hook_out, ...) y la
// columna tenants.is_demo.
//
// POR QUÉ EXPORTADA. El generador del esquema canónico (tools/esquemacanonico)
// y las pruebas necesitan el mismo esquema que arranca la aplicación. Si cada
// uno copiara las sentencias por su lado, el canónico quedaría distinto de la
// base que produce el arranque real — que es exactamente el defecto que el
// plan de esquema canónico viene a cerrar.
//
// LO QUE NO ESTÁ AQUÍ, A PROPÓSITO. Las tablas condicionales (guia_* solo
// para tenants demo, spidi_pago y yummy_webhook_evento solo con la integración
// activa) siguen creándolas cada módulo cuando se activa; meterlas aquí las
// pondría en instalaciones que no las usan.
func AplicarEsquemasDeCodigo(db *sql.DB) error {
	if err := persistence.AplicarEsquema(db, controlSchema); err != nil {
		return err
	}
	// is_demo: ALTER directo, tolerante a "la columna ya existe" (el mismo
	// patrón que ensureDemoColumn en guia_goti.go; MySQL no admite
	// ADD COLUMN IF NOT EXISTS y con el error descartado la columna nunca
	// nacía en instalaciones MySQL).
	_, _ = db.Exec(`ALTER TABLE tenants ADD COLUMN is_demo TINYINT(1) NOT NULL DEFAULT 0`)
	return nil
}
