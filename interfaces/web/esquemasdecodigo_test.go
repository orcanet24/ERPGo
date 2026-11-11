package web

import (
	"database/sql"
	"testing"

	"erpgo/infrastructure/persistence"
)

// TestAplicarEsquemasDeCodigo verifica que el esquema que el código crea fuera
// de migrations/ se aplica entero: la BD de control y la columna is_demo.
//
// POR QUÉ ESTA PRUEBA. El generador del esquema canónico (tools/esquemacanonico)
// se apoya en esta función; si alguna tabla de control deja de crearse, el
// canónico se queda incompleto y la verificación contra la base real lo delata
// — pero solo si esta prueba no pasó primero por aquí.
//
// Corre contra MySQL, no contra SQLite: controlSchema es DDL de MySQL
// (AUTO_INCREMENT) y probarlo contra otro motor no diría nada del real. Si no
// hay MySQL, se salta: un go test en una máquina sin servidor no debe ponerse
// rojo por algo que no está probado.
func TestAplicarEsquemasDeCodigo(t *testing.T) {
	db, limpiar, err := persistence.NuevaBaseVaciaDePrueba("esqcod")
	if err != nil {
		t.Skipf("no hay MySQL para esta prueba: %v", err)
	}
	defer limpiar()

	// La BD de control de verdad nace de migrations/schema.sql, que ya crea
	// `tenants` con estas 10 columnas; aquí se imita ese punto de partida para
	// que la prueba mida el ALTER de is_demo sobre una tabla preexistente.
	if _, err := db.Exec(`CREATE TABLE tenants (
		code VARCHAR(64) PRIMARY KEY,
		legal_name VARCHAR(255) NOT NULL DEFAULT '',
		rif VARCHAR(32) NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 1,
		created_at VARCHAR(32) NOT NULL DEFAULT '',
		db_user VARCHAR(128) NOT NULL DEFAULT '',
		db_password_enc VARBINARY(512) DEFAULT '',
		max_users INTEGER NOT NULL DEFAULT 0,
		created_by VARCHAR(128) NOT NULL DEFAULT '',
		db_size_mb INTEGER NOT NULL DEFAULT 0
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		t.Fatalf("tenants inicial: %v", err)
	}

	if err := AplicarEsquemasDeCodigo(db); err != nil {
		t.Fatalf("AplicarEsquemasDeCodigo: %v", err)
	}

	for _, tabla := range []string{"tenants", "tenant_users", "tenant_provision_log", "web_sessions", "addon_hook_out"} {
		var nombre string
		err := db.QueryRow("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", tabla).Scan(&nombre)
		if err == sql.ErrNoRows {
			t.Errorf("la tabla %s no se creó", tabla)
		} else if err != nil {
			t.Fatalf("consultando %s: %v", tabla, err)
		}
	}

	// is_demo debe existir aunque la tabla ya existiera sin ella.
	var columna string
	err = db.QueryRow("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tenants' AND COLUMN_NAME = 'is_demo'").Scan(&columna)
	if err == sql.ErrNoRows {
		t.Error("la columna tenants.is_demo no se agregó")
	} else if err != nil {
		t.Fatalf("consultando is_demo: %v", err)
	}

	// Idempotencia: la segunda aplicación no debe fallar (el ALTER repetido
	// se descarta a propósito).
	if err := AplicarEsquemasDeCodigo(db); err != nil {
		t.Fatalf("segunda aplicación falló: %v", err)
	}
}
