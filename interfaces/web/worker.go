package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"erpgo/internal/queue"
)

// ---------- Puente entre la cola y el servidor ----------
//
// La cola vive en la BD de CONTROL (no en la base de una empresa): es
// infraestructura, no dato contable. Cada tarea lleva su empresa, y el worker
// abre esa base antes de ejecutar. Así una tarea de la empresa A jamás corre
// contra los datos de la B, aunque el usuario haya cambiado de empresa en el
// navegador mientras la tarea esperaba.

// queueAIKind es el tipo de tarea que ejecuta un agente IA.
const queueAIKind = "ai_task"

// aiTaskPayload es el cuerpo de una tarea IA encolada.
type aiTaskPayload struct {
	TaskID  int64 `json:"task_id"`
	AgentID int64 `json:"agent_id"`
	// Slot es la ranura de programación calculada al ENCOLAR, no al ejecutar.
	//
	// Tiene que viajar en el payload: entre el tick de las 6:55 y la ejecución
	// pueden pasar minutos, y recalcularla al ejecutar daría otra ranura (u otro
	// día) y abriría la puerta a la corrida duplicada que esto cierra.
	Slot string `json:"slot"`
}

// startQueue levanta el motor de tareas sobre la BD de control y registra el
// handler de IA. Se llama una vez desde NewServer.
func (s *Server) startQueue() error {
	ctl, err := s.controlStore()
	if err != nil {
		return err
	}
	q, err := queue.New(ctl, queue.Config{
		// Un solo escritor en SQLite: dos workers bastan. Más workers solo
		// agregan contención sobre el mismo lock de escritura.
		Workers:         2,
		MaxPending:      256,
		MaxAttempts:     3,
		PollInterval:    5 * time.Second,
		TaskTimeout:     10 * time.Minute,
		MaxPayloadBytes: 256 * 1024,
		// Espera corta a propósito. Esta cola lleva tareas de IA, no ventas:
		// si se llena, interesa que el scheduler siga su ciclo y lo intente
		// en la vuelta siguiente, como dice el comentario de enqueueDueAITasks,
		// en vez de quedarse parado. El freno largo (30 s) es para las
		// escrituras de negocio, que no pueden perder trabajo.
		EnqueueTimeout: 2 * time.Second,
	})
	if err != nil {
		return err
	}
	q.Handle(queueAIKind, s.runQueuedAITask)
	s.queue = q
	q.Start()
	return nil
}

// runQueuedAITask ejecuta una tarea IA en segundo plano.
//
// El contexto trae la empresa y el presupuesto de filas. El presupuesto se
// aplica AQUÍ, en Go, y no como una instrucción en el prompt: una instrucción
// en el prompt es una sugerencia para el modelo, mientras que un límite en Go
// es una garantía de memoria.
func (s *Server) runQueuedAITask(ctx context.Context, t queue.Task) error {
	var p aiTaskPayload
	if err := json.Unmarshal(t.Payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	tenant := queue.TenantFromContext(ctx)
	if tenant == "" {
		tenant = t.Tenant
	}

	db, err := s.openTenantDB(tenant)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la base de %s: %w", tenant, err)
	}

	// Los helpers de agente usan la conexión global agentDB; se apunta a la
	// base de la tarea y se restaura al terminar.
	prev := agentDB
	agentDB = db
	defer func() { agentDB = prev }()

	// La tarea viene de la cola, no de una petición: la empresa la trae el
	// contexto de la tarea. Se construye la sesión mínima que necesitan los
	// helpers para resolver la empresa correcta.
	sess := &SessionData{TenantCode: tenant}

	task, ok := s.loadAITaskByID(sess, p.TaskID)
	if !ok {
		return fmt.Errorf("tarea %d no encontrada en la empresa %s", p.TaskID, tenant)
	}

	ctx = queue.WithRowBudget(ctx, aiAgentRowBudget(p.AgentID))
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := s.aiExecuteTask(sess, task, p.Slot); err != nil {
		// Ranura tomada: la tarea ya corrió en este período. NO es un fallo y no
		// debe reintentarse: devolver el error haría que la cola la reintentara
		// tres veces para volver a chocar con la misma ranura.
		if errors.Is(err, errAIRunSlotTaken) {
			return nil
		}
		return err
	}
	return nil
}

// aiAgentRowBudget devuelve el presupuesto de filas del agente, acotado a un
// rango seguro. El tope superior es deliberado: un agente con max_rows
// configurado a 100.000 traería la tabla entera a memoria.
func aiAgentRowBudget(agentID int64) int {
	var maxRows sql.NullInt64
	_ = agentDB.QueryRow(`SELECT max_rows FROM ai_agent WHERE id = ?`, agentID).Scan(&maxRows)
	budget := 500
	if maxRows.Valid && maxRows.Int64 > 0 {
		budget = int(maxRows.Int64)
	}
	if budget > 5000 {
		return 5000
	}
	if budget < 1 {
		return 500
	}
	return budget
}

// ---------- Reemplazo del scheduler en memoria ----------

// aiTaskRunner ahora solo PLANIFICA: cada 5 minutos encola las tareas IA que
// vencen, y la cola decide cuándo, con qué límite y en qué orden se ejecutan.
// Antes este bucle las ejecutaba una a una en el mismo goroutine: sin cola, sin
// backpressure, sin recuperación tras un reinicio y sin techo de memoria.
func (s *Server) aiTaskRunner() {
	if s.queue == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.enqueueDueAITasks()
		}
	}()
}

// enqueueDueAITasks encola (no ejecuta) las tareas IA cuya programación venció.
//
// Recorre TODAS las empresas registradas, no solo la activa: el planificador es
// un servicio del proceso, no de quien esté usando la aplicación en este momento.
// Antes miraba solo la empresa activa, así que un agente programmed en una
// segunda empresa se quedaba sin ejecutar hasta que alguien entraba a ella.
func (s *Server) enqueueDueAITasks() {
	if s.queue == nil {
		return
	}
	nowT := aiAgentClock()
	for _, tenant := range s.TenantCodes() {
		db := s.tenantDBMust(tenant)
		if db == nil {
			continue
		}
		s.enqueueDueAITasksOf(tenant, db, nowT)
	}
}

// enqueueDueAITasksOf encola las tareas vencidas de una empresa concreta.
func (s *Server) enqueueDueAITasksOf(tenant string, db *sql.DB, nowT time.Time) {
	tasks := []aiTask{}
	rows, err := db.Query(`SELECT id, agent_id, name, schedule_type, schedule_value, enabled
		FROM ai_task WHERE enabled = 1 AND schedule_type != 'manual'`)
	if err != nil {
		return
	}
	for rows.Next() {
		var t aiTask
		if rows.Scan(&t.ID, &t.AgentID, &t.Name, &t.ScheduleType, &t.ScheduleValue, &t.Enabled) != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	rows.Close()

	for _, t := range tasks {
		if !aiScheduleDue(t, nowT) {
			continue
		}
		// No encolar si la corrida anterior sigue activa.
		var status string
		_ = db.QueryRow(`SELECT status FROM ai_task_run WHERE task_id = ? AND started_at >= ?
			ORDER BY id DESC LIMIT 1`,
			t.ID, nowT.Add(-6*time.Minute).Format("2006-01-02 15:04:05")).Scan(&status)
		if status == "running" {
			continue
		}
		// La ranura se calcula aquí, en el momento de encolar, y viaja con la
		// tarea. Es la clave única que impide que esta misma tarea se encole otra
		// vez en el mismo período.
		slot := aiRunSlot(t, nowT)
		if slot == "" {
			continue
		}
		// Backpressure: si la cola está llena, esta tarea espera al siguiente
		// ciclo. No es un motivo para tumbar el scheduler ni para descartar trabajo.
		_, err := s.queue.Enqueue(queueAIKind, tenant, aiTaskPayload{
			TaskID: t.ID, AgentID: t.AgentID, Slot: slot,
		})
		// El error se imprime y no se descarta. Una tarea de IA que no se
		// encola es trabajo que no se hace, y con el descarte el síntoma es
		// "el agente nunca corre" sin ninguna pista de por qué.
		if err != nil {
			fmt.Printf("  IA: la tarea %d de %s no se encoló: %v\n", t.ID, tenant, err)
		}
	}
}

