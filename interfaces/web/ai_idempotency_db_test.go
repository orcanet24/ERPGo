package web

// Pruebas de base de datos del libro de idempotencia y de la ranura de período.
//
// Las del archivo ai_governance_test.go son funciones puras; estas ejecutan las
// SENTENCIAS REALES del código contra una base en memoria. Es la única forma de
// comprobar lo que de verdad importa aquí: que la clave única impide la segunda
// escritura, que el estado 'undone' vuelve a liberarla y que la ranura de un
// período se reclama una sola vez. Una prueba de funciones puras no distingue un
// candado puesto de un candado que nunca se cerró.
//
// El motor es SQLite en memoria (el driver ya está en go.mod), así que corren sin
// servidor. La forma de INSERT y la detección de clave duplicada se eligen para
// funcionar en los dos motores: ver aiEsClaveDuplicada.

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

// nuevaBaseDeIdempotencia crea las dos tablas del libro y la ranura.
func nuevaBaseDeIdempotencia(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	// Una sola conexión: con varias, cada una tendría su propia base :memory: y la
	// prueba pasaría o fallaría según el azar del pool.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	sentencias := []string{
		`CREATE TABLE ai_tool_execution (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			idem_key TEXT NOT NULL UNIQUE,
			tool TEXT NOT NULL DEFAULT '',
			agent_id INTEGER NOT NULL DEFAULT 0,
			run_id INTEGER NOT NULL DEFAULT 0,
			task_id INTEGER NOT NULL DEFAULT 0,
			external_ref TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'claimed',
			result_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL)`,
		`CREATE TABLE ai_run_slot (
			slot_key TEXT NOT NULL PRIMARY KEY,
			task_id INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL)`,
	}
	for _, ddl := range sentencias {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("no se pudo crear la tabla: %v", err)
		}
	}
	return db
}

func contarFilas(t *testing.T, db *sql.DB, tabla string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tabla).Scan(&n); err != nil {
		t.Fatalf("no se pudo contar %s: %v", tabla, err)
	}
	return n
}

// TestLibroNoRepiteLaAccion es la regresión del hueco 3 en la base.
func TestLibroNoRepiteLaAccion(t *testing.T) {
	db := nuevaBaseDeIdempotencia(t)
	s := &Server{}
	args := map[string]interface{}{"invoice_id": float64(4), "amount": float64(50)}
	journal := &aiRunJournal{runID: 11, taskID: 2, agentID: 3, scope: "task:2:day:2026-10-27"}

	primera := s.aiClaimTool(db, journal, "payment.record", "", args)
	if primera.Skip || primera.Key == "" {
		t.Fatalf("la primera llamada debe reclamar la clave; skip=%v key=%q", primera.Skip, primera.Key)
	}
	s.aiFinishTool(db, primera, `{"status":"ok","voucher":"CV-7"}`, nil)

	// El reintento de la cola vuelve a llamar con lo mismo: no puede ejecutar.
	segunda := s.aiClaimTool(db, journal, "payment.record", "", args)
	if !segunda.Skip {
		t.Fatal("la segunda llamada con los mismos datos ejecutó de nuevo: el documento se duplicaría")
	}
	if !strings.Contains(segunda.Result, "CV-7") {
		t.Errorf("debe devolver el documento original para el informe final; salió: %s", segunda.Result)
	}
	if !strings.Contains(segunda.Result, "duplicate") {
		t.Errorf("debe marcar la respuesta como duplicada: %s", segunda.Result)
	}
	if n := contarFilas(t, db, "ai_tool_execution"); n != 1 {
		t.Errorf("el reintento creó otra fila en el libro (hay %d)", n)
	}

	// Otro día es otro documento: la clave no es un candado permanente.
	otroDia := &aiRunJournal{runID: 12, taskID: 2, agentID: 3, scope: "task:2:day:2026-10-28"}
	if c := s.aiClaimTool(db, otroDia, "payment.record", "", args); c.Skip {
		t.Error("el día siguiente debe poder registrar su propio cobro")
	}
}

// TestClaveLiberadaTrasRevertir: revertir la corrida y volver a lanzarla tiene que
// poder crear el documento otra vez. Su reverso —bloquear para siempre— sería el
// fallo opuesto.
func TestClaveLiberadaTrasRevertir(t *testing.T) {
	db := nuevaBaseDeIdempotencia(t)
	s := &Server{}
	args := map[string]interface{}{"vtype": "JV", "note": "ajuste"}
	journal := &aiRunJournal{runID: 20, taskID: 4, agentID: 5, scope: "task:4:day:2026-10-27"}

	claim := s.aiClaimTool(db, journal, "voucher.create", "", args)
	if claim.Skip {
		t.Fatal("la primera ejecución debe reclamar la clave")
	}
	s.aiFinishTool(db, claim, `{"status":"ok","vno":"JV-3"}`, nil)

	// Lo que hace aiUndoLastRun al revertir la corrida.
	if _, err := db.Exec(`UPDATE ai_tool_execution SET status='undone' WHERE run_id = ?`, int64(20)); err != nil {
		t.Fatalf("no se pudo marcar el libro como revertido: %v", err)
	}

	revuelta := s.aiClaimTool(db, journal, "voucher.create", "", args)
	if revuelta.Skip {
		t.Fatal("tras revertir, la misma acción debe poder volver a ejecutarse")
	}
	// Y vuelve a quedar reclamada, no libre por accidente.
	if !s.aiClaimTool(db, journal, "voucher.create", "", args).Skip {
		t.Error("la clave reutilizada debe quedar reclamada otra vez")
	}
}

// TestReferenciaExternaDeduplicaEntreCorridas: el caso central del informe —una
// tarea que carga documentos de un listado, corrida dos veces—.
func TestReferenciaExternaDeduplicaEntreCorridas(t *testing.T) {
	db := nuevaBaseDeIdempotencia(t)
	s := &Server{}
	args := map[string]interface{}{"customer_id": float64(1), "items": []interface{}{}}

	// La corrida de hoy carga la línea 3 del listado 77.
	hoy := &aiRunJournal{runID: 30, taskID: 6, agentID: 7, scope: "task:6:day:2026-10-27"}
	primera := s.aiClaimTool(db, hoy, "invoice.create_draft", "LISTADO-77/linea-3", args)
	if primera.Skip {
		t.Fatal("la primera corrida debe crear el borrador")
	}
	s.aiFinishTool(db, primera, `{"status":"ok","draft_id":1201}`, nil)

	// Pasan los días y la tarea se vuelve a lanzar sobre el mismo listado.
	despues := &aiRunJournal{runID: 44, taskID: 6, agentID: 7, scope: "task:6:day:2026-11-03"}
	segunda := s.aiClaimTool(db, despues, "invoice.create_draft", "LISTADO-77/linea-3", args)
	if !segunda.Skip {
		t.Fatal("la misma referencia externa debe devolver el documento ya creado, no crear otro")
	}
	if !strings.Contains(segunda.Result, "1201") {
		t.Errorf("debe devolver el borrador original; salió: %s", segunda.Result)
	}

	// Y la referencia queda guardada: es el rastro que antes no existía.
	var ref, tool string
	if err := db.QueryRow(`SELECT external_ref, tool FROM ai_tool_execution LIMIT 1`).Scan(&ref, &tool); err != nil {
		t.Fatalf("no se pudo leer el rastro: %v", err)
	}
	if ref != "LISTADO-77/linea-3" || tool != "invoice.create_draft" {
		t.Errorf("rastro incompleto: tool=%q ref=%q", tool, ref)
	}
}

// TestRanuraDePeriodoSeReclamaUnaVez es la regresión de la duplicación del ticker.
func TestRanuraDePeriodoSeReclamaUnaVez(t *testing.T) {
	db := nuevaBaseDeIdempotencia(t)
	s := &Server{}
	slot := "task:1:daily:2026-10-27"

	ok, err := s.aiClaimRunSlot(db, slot, 1)
	if err != nil || !ok {
		t.Fatalf("la primera vuelta del ticker debe reclamar la ranura (ok=%v err=%v)", ok, err)
	}
	// Las otras once vueltas de esa hora caen en la misma ranura.
	for i := 0; i < 11; i++ {
		ok, err := s.aiClaimRunSlot(db, slot, 1)
		if err != nil {
			t.Fatalf("reclamar una ranura ya tomada no es un error: %v", err)
		}
		if ok {
			t.Fatal("la ranura del período se reclamó dos veces: la tarea diaria volvería a duplicar")
		}
	}

	// Un fallo libera la ranura para que el reintento pueda correr.
	s.aiReleaseRunSlot(db, slot)
	if ok, err := s.aiClaimRunSlot(db, slot, 1); err != nil || !ok {
		t.Fatalf("tras un fallo la ranura debe quedar libre (ok=%v err=%v)", ok, err)
	}

	// Otro período es otra ranura.
	if ok, _ := s.aiClaimRunSlot(db, "task:1:daily:2026-10-28", 1); !ok {
		t.Error("el día siguiente necesita su propia ranura")
	}

	// Sin ranura (disparo manual) nunca bloquea.
	if ok, _ := s.aiClaimRunSlot(db, "", 1); !ok {
		t.Error("un disparo manual no tiene período que reclamar")
	}
}

// TestBookSinJournalNoBloquea: sin journal no hay ámbito del que derivar una
// clave, y el código debe ejecutar como antes en vez de inventarse una clave
// compartida que bloquearía todo el sistema.
func TestBookSinJournalNoBloquea(t *testing.T) {
	db := nuevaBaseDeIdempotencia(t)
	s := &Server{}
	claim := s.aiClaimTool(db, nil, "payment.record", "", map[string]interface{}{"amount": float64(1)})
	if claim.Skip {
		t.Fatal("sin journal no se puede deduplicar: debe ejecutarse, no bloquearse")
	}
	if claim.Key != "" {
		t.Errorf("sin journal no debe haber clave; salió %q", claim.Key)
	}
	if n := contarFilas(t, db, "ai_tool_execution"); n != 0 {
		t.Errorf("no debe dejar rastro sin ámbito; hay %d filas", n)
	}
}
