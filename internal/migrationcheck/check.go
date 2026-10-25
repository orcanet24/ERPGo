// migrationcheck verifica al inicio que las tablas críticas existan.
// Si faltan, se avisa por log y se activa un badge discreto para superadmin.
package migrationcheck

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

var (
	muCheck   sync.RWMutex
	estaListo = true
	faltantes []string
)

// TablasCriticas son las tablas que el sistema necesita para funcionar.
var TablasCriticas = []string{
	"users",
	"user_login",
	"sec_role",
	"sec_userrole",
	"sub_module",
	"module",
	"role_permission",
	"company_information",
	"invoice",
	"invoice_details",
	"product_information",
	"customer_information",
	"supplier_information",
	"purchase",
	"product_purchase",
	"product_purchase_details",
	"stock_movement",
	"menu",
	"fiscal_series",
	"fiscal_setting",
	"tax_setting",
	"acc_transaction",
	"acc_vaucher",
	"audit_log",
}

// CheckMissingTables verifica si las tablas existen en la base de datos.
// Retorna las que faltan y un booleano indicando si está todo completo.
func CheckMissingTables(db *sql.DB) ([]string, bool) {
	if db == nil {
		return nil, true
	}

	var missing []string
	for _, tabla := range TablasCriticas {
		var nombre string
		err := db.QueryRow(fmt.Sprintf("SHOW TABLES LIKE '%s'", tabla)).Scan(&nombre)
		if err == sql.ErrNoRows {
			missing = append(missing, tabla)
		} else if err != nil {
			log.Printf("migrationcheck: error verificando %s: %v", tabla, err)
			missing = append(missing, tabla)
		}
	}

	muCheck.Lock()
	faltantes = missing
	estaListo = len(missing) == 0
	muCheck.Unlock()

	return missing, len(missing) == 0
}

// EstaListo retorna true si todas las tablas existen.
func EstaListo() bool {
	muCheck.RLock()
	defer muCheck.RUnlock()
	return estaListo
}

// Faltantes retorna la lista de tablas que faltan.
func Faltantes() []string {
	muCheck.RLock()
	defer muCheck.RUnlock()
	return append([]string{}, faltantes...)
}

// MensajeAdmin retiene el aviso que debe mostrarse al superadmin.
func MensajeAdmin() string {
	if EstaListo() {
		return ""
	}
	f := Faltantes()
	if len(f) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"⚠️ El sistema tiene %d tablas pendientes. Ejecute ./erpgow.exe -migrate para activar las nuevas funcionalidades. Tablas faltantes: %s",
		len(f), strings.Join(f, ", "),
	)
}

// LoguearResultado escribe en el log el estado de las migraciones.
func LoguearResultado() {
	if EstaListo() {
		log.Println("✓ Migración: todas las tablas críticas presentes.")
		return
	}
	f := Faltantes()
	log.Printf("⚠ MIGRACIÓN PENDIENTE: faltan %d tablas: %s", len(f), strings.Join(f, ", "))
	log.Printf("⚠ Ejecute: %s -migrate", os.Args[0])
}
