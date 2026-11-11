package queue

import (
	"database/sql"
	"testing"

	"erpgo/infrastructure/persistence"
)

// TestAplicarEsquema verifica que la tabla de la cola se crea y que la
// aplicación es idempotente (el generador del esquema canónico la aplica sobre
// una base que ya puede tenerla).
func TestAplicarEsquema(t *testing.T) {
	db, err := persistence.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir SQLite: %v", err)
	}
	defer db.Close()

	if err := AplicarEsquema(db); err != nil {
		t.Fatalf("AplicarEsquema: %v", err)
	}
	if err := AplicarEsquema(db); err != nil {
		t.Fatalf("segunda aplicación falló: %v", err)
	}

	var nombre string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='queue_tasks'`).Scan(&nombre)
	if err == sql.ErrNoRows {
		t.Fatal("la tabla queue_tasks no se creó")
	} else if err != nil {
		t.Fatalf("consultando queue_tasks: %v", err)
	}
}
