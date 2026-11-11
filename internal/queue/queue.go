// Package queue implementa el motor de trabajo en segundo plano de ERPGo.
//
// El objetivo es explícito: ejecutar tareas (IA, reportes, cierres) SIN que la
// aplicación se quede sin memoria ni bloquee al usuario. MySQL/InnoDB permite
// múltiples escritores, pero la cola usa un pool fijo de workers para mantener
// la concurrencia predecible y no sobrecargar la base de datos:
//
//   - Pool FIJO de workers. Nunca un goroutine por tarea.
//   - Backpressure por FRENADO: si la cola está llena, Enqueue espera a que
//     haya hueco en vez de rechazar.
//   - Spill a disco: las tareas pendientes viven en la tabla queue_tasks, no en
//     un canal de Go. Un reinicio no pierde trabajo y la RAM no crece con la cola.
//   - Cotas por tarea: un handler recibe un contexto con tiempo y un presupuesto
//     de filas. Una tarea que se desvía se corta, no se come el proceso.
//
// La cola es infraestructura, no dato de negocio: por eso vive en la BD de
// control y cada tarea lleva su empresa (tenant). Así una tarea de la empresa A
// nunca se ejecuta contra los datos de la B.
//
// REQUISITO del store: debe ser una conexión MySQL/InnoDB. Los workers leen y
// escriben la tabla queue_tasks a la vez.
package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"erpgo/infrastructure/persistence"
)

// ErrFull indica que la cola lleva lleno más de EnqueueTimeout. Con el
// freno por defecto ya no se devuelve por saturación normal, solo cuando el
// escritor está parado, que es un problema que hay que reportar.
// El productor debe decidir si
// reintentar más tarde o descartar. Es backpressure deliberado, no un fallo.
var ErrFull = errors.New("queue: cola de tareas llena")

// ErrNoHandler indica que nadie sabe procesar ese tipo de tarea.
var ErrNoHandler = errors.New("queue: no hay handler registrado para el tipo")

// Status es el estado de una tarea en su ciclo de vida.
type Status string

const (
	StatusPending Status = "pending" // en espera
	StatusRunning Status = "running" // un worker la tomó
	StatusDone    Status = "done"    // terminó bien
	StatusFailed  Status = "failed"  // agotó los reintentos
)

// Task es una unidad de trabajo. Tenant es obligatorio: una tarea sin empresa
// no se ejecuta, porque no hay contexto válido donde correrla.
type Task struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Tenant    string          `json:"tenant"`
	Payload   json.RawMessage `json:"payload"`
	Attempts  int             `json:"attempts"`
	NotBefore time.Time       `json:"not_before"` // retrasa sin.timer en RAM
	CreatedAt time.Time       `json:"created_at"`
}

// Handler procesa una tarea. El contexto trae el tenant y lleva timeout.
type Handler func(ctx context.Context, t Task) error

// Config acota el motor. Los defaults son conservadores a propósito: una app
// de escritorio debe funcionar en una máquina modesta.
type Config struct {
	// Workers es el pool fijo. 2 basta para el ERP; subirlo aumenta el
	// consumo sin mejorar el throughput de la BD.
	Workers int
	// MaxPending acota la cola. Al llenarse, Enqueue devuelve ErrFull.
	MaxPending int
	// MaxAttempts es el número de intentos antes de marcar la tarea fallida.
	MaxAttempts int
	// PollInterval es cada cuánto se busca trabajo pendiente. Como el trabajo
	// vive en disco, un reinicio no pierde nada: solo se retrasa hasta el poll.
	PollInterval time.Duration
	// TaskTimeout corta una tarea que se desvía.
	TaskTimeout time.Duration
	// RetryBackoff es la espera base entre reintentos. El crecimiento es
	// exponencial (1x, 2x, 4x) y está acotado por MaxAttempts, así que un
	// fallo persistente no genera una avalancha de reintentos.
	RetryBackoff time.Duration
	// MaxPayloadBytes acota el tamaño del payload para que una tarea no pueda
	// arrastrar un payload enorme a memoria.
	MaxPayloadBytes int
	// EnqueueTimeout acota cuánto espera un productor cuando la cola está
	// llena. Pasado ese tiempo Enqueue devuelve ErrFull. Con cero, espera
	// hasta que haya hueco.
	EnqueueTimeout time.Duration
}

func (c *Config) withDefaults() {
	if c.Workers <= 0 {
		c.Workers = 2
	}
	if c.MaxPending <= 0 {
		c.MaxPending = 256
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.TaskTimeout <= 0 {
		c.TaskTimeout = 5 * time.Minute
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = 30 * time.Second
	}
	if c.EnqueueTimeout <= 0 {
		// Treinta segundos. Suficiente para que un pico de carga se
		// absorba solo, y lo bastante corto para que un escritor parado
		// se reporte en lugar de colgar al usuario.
		c.EnqueueTimeout = 30 * time.Second
	}
	if c.MaxPayloadBytes <= 0 {
		c.MaxPayloadBytes = 256 * 1024
	}
}

// Queue es el motor. Es seguro para uso concurrente.
type Queue struct {
	store    *sql.DB
	cfg      Config
	handlers map[string]Handler

	wake chan struct{} // señal de "hay trabajo": tamaño 1, nunca bloquea

	mu      sync.Mutex
	pending int // tamaño actual de la cola, para el backpressure
	// lleno avisa a los productores que están esperando hueco. Es un
	// Cond y no un canal porque el aviso vapaired al mutex: así no hay
	// carrera entre "comprobar que está lleno" y "quedarse sin hueco"
	// (el despertar perdido, que sería la forma clásica de colgarse).
	lleno *sync.Cond

	wg   sync.WaitGroup
	stop chan struct{}
	once sync.Once
	seq  uint64 // contador atómico de IDs (ver nextID)
}

// New crea el motor y aplica el esquema de su tabla de tareas. El store debe
// ser la BD de control (no una base de empresa): la cola es infraestructura.
func New(store *sql.DB, cfg Config) (*Queue, error) {
	cfg.withDefaults()
	if store == nil {
		return nil, errors.New("queue: store nil")
	}
	if err := persistence.AplicarEsquema(store, schema); err != nil {
		return nil, fmt.Errorf("queue: esquema: %w", err)
	}
	q := &Queue{
		store:    store,
		cfg:      cfg,
		handlers: make(map[string]Handler),
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}
	// El Cond comparte el mutex de la cola: la espera por hueco y el
	// contador que lo regula son el mismo estado sincronizado.
	q.lleno = sync.NewCond(&q.mu)
	// Recuperación: una tarea "running" al arrancar quedó a medias por un
	// cierre duro. Vuelve a pendiente para reintentarse.
	if _, err := store.Exec(`UPDATE queue_tasks SET status='pending' WHERE status='running'`); err != nil {
		return nil, fmt.Errorf("queue: recuperación: %w", err)
	}
	q.syncPending()
	return q, nil
}

// schema de la cola. Una sola tabla: pending/running/done/failed con un índice
// por (status, not_before) para que el poll tome trabajo maduro en orden.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS queue_tasks (
		id VARCHAR(64) PRIMARY KEY,
		kind VARCHAR(64) NOT NULL,
		tenant VARCHAR(64) NOT NULL,
		payload LONGTEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		not_before VARCHAR(32) NOT NULL DEFAULT '',
		status VARCHAR(32) NOT NULL DEFAULT 'pending',
		last_error TEXT NULL,
		created_at VARCHAR(32) NOT NULL DEFAULT '',
		updated_at VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX idx_queue_poll ON queue_tasks(status, not_before)`,
}

// AplicarEsquema crea la tabla de la cola fuera de New, para quien necesite
// solo el esquema (el generador del esquema canónico, las pruebas).
//
// POR QUÉ EXPORTADA. La cola es de la BD de control, no de cada empresa; el
// generador del canónico necesita crearla sin levantar el motor de workers.
func AplicarEsquema(store *sql.DB) error {
	return persistence.AplicarEsquema(store, schema)
}

// Handle registra el handler de un tipo de tarea.
func (q *Queue) Handle(kind string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[kind] = h
}

// Start levanta los workers. Idempotente en la práctica: llámalo una vez.
func (q *Queue) Start() {
	for i := 0; i < q.cfg.Workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
}

// Stop detiene los workers y espera a que terminen la tarea en curso.
func (q *Queue) Stop() {
	q.once.Do(func() {
		close(q.stop)
		q.wg.Wait()
	})
}

// Enqueue agrega una tarea. Devuelve ErrFull si la cola está a su capacidad
// (backpressure) o ErrNoHandler si nadie puede procesar ese tipo.
func (q *Queue) Enqueue(kind, tenant string, payload any) (string, error) {
	if tenant == "" {
		return "", errors.New("queue: la tarea necesita empresa")
	}
	q.mu.Lock()
	_, known := q.handlers[kind]
	q.mu.Unlock()
	if !known {
		return "", fmt.Errorf("%w: %s", ErrNoHandler, kind)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(raw) > q.cfg.MaxPayloadBytes {
		return "", fmt.Errorf("queue: payload de %d bytes excede el máximo (%d)", len(raw), q.cfg.MaxPayloadBytes)
	}

	// El contador se actualiza aquí, antes del INSERT, y se revierte si el
	// INSERT falla: así el backpressure nunca queda inflado por un error.
	//
	// Backpressure por ESPERA, no por rechazo. Antes, al llegar al tope,
	// Enqueue devolvía ErrFull y el llamador decidía qué hacer: si no
	// reintentaba, la venta se perdía. Eso es lo que hacía fallar
	// TestEncoladoRAM_vs_Directo, y por eso el modo DIRECTO, que no usa
	// la cola, perdía también (allí la pérdida es del llamador, no de la
	// cola). Un sistema de facturación no puede perder una venta por
	// estar lleno: se frena.
	//
	// La espera está acotada por EnqueueTimeout. Pasado ese tiempo sí se
	// devuelve ErrFull, porque entonces el problema no es la cola sino
	// que el escritor está parado, y eso hay que reportarlo.
	q.mu.Lock()
	limite := time.Now().Add(q.cfg.EnqueueTimeout)
	esperando := time.NewTicker(5 * time.Millisecond)
	defer esperando.Stop()
	for q.pending >= q.cfg.MaxPending && time.Now().Before(limite) {
		// sync.Cond no sabe de tiempo. Se sondea el contador con un
		// ticker corto en vez de confiar en un aviso: si el aviso se
		// perdiera (porque llega antes de que el productor se duerma),
		// el productor se quedaría colgado hasta el final del proceso.
		// Sondear cuesta un poco más pero no se cuelga nunca, que es
		// justo lo que se le pide a esta espera.
		q.mu.Unlock()
		<-esperando.C
		q.mu.Lock()
	}
	lleno := q.pending >= q.cfg.MaxPending
	if !lleno {
		q.pending++
	}
	q.mu.Unlock()
	if lleno {
		return "", ErrFull
	}

	id := q.nextID()
	// `last_error` se manda explícitamente aunque esté vacía, y en el esquema es
	// una TEXT sin NOT NULL. MySQL no admite DEFAULT en columnas TEXT ("BLOB,
	// TEXT, GEOMETRY or JSON column can't have a default value") y tampoco las
	// deja NOT NULL sin valor por defecto (Error 1364). Con las dos cosas, el
	// INSERT entero fallaba y la tarea NUNCA entraba en la cola: el planificador
	// de tareas de IA y el encolador de escrituras quedaban mudos, sin error
	// visible. El driver MySQL es más estricto con los tipos que SQLite.
	// El fallo llevaba tanto tiempo escondido.
	_, err = q.execRetry(`INSERT INTO queue_tasks (id, kind, tenant, payload, attempts, not_before, status, last_error, created_at, updated_at)
		VALUES (?,?,?,?,0,?, 'pending', '', ?, ?)`,
		id, kind, tenant, string(raw), nowStr(), nowStr(), nowStr())
	if err != nil {
		q.mu.Lock()
		q.pending--
		q.mu.Unlock()
		return "", err
	}
	q.signal()
	return id, nil
}

// nextID genera un identificador único y monotónico.
//
// La unicidad NO puede depender del reloj: con varios productores encolando a
// la vez, time.Now() puede devolver el mismo valor y dos tareas acabarían con
// el mismo id, con lo que una se perdería en el INSERT. Por eso el id combina
// un contador atómico con un sufijo aleatorio.
func (q *Queue) nextID() string {
	n := atomic.AddUint64(&q.seq, 1)
	return fmt.Sprintf("%d-%s", n, randomSuffix())
}

// randomSuffix devuelve 8 caracteres hexadecimales aleatorios.
func randomSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand no debería fallar; si falla, el contador sigue garantizando
		// unicidad dentro del proceso.
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

// execRetry ejecuta una sentencia reintentando ante contención transitoria.
// MySQL/InnoDB puede tener deadlocks o timeouts de lock con múltiples
// escritores concurrentes. Los reintentos son pocos y acotados: si el bloqueo
// persiste, es un problema real y se devuelve el error.
func (q *Queue) execRetry(query string, args ...any) (sql.Result, error) {
	const attempts = 5
	var lastErr error
	for i := 0; i < attempts; i++ {
		res, err := q.store.Exec(query, args...)
		if err == nil {
			return res, nil
		}
		if !isBusy(err) {
			return nil, err
		}
		lastErr = err
		time.Sleep(time.Duration(5*(i+1)) * time.Millisecond)
	}
	return nil, lastErr
}

// isBusy detecta la contención transitoria de MySQL o SQLite.
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// MySQL: deadlock, lock wait timeout, too many connections
	// SQLite: database is locked (por si se usa en tests)
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "busy") ||
		strings.Contains(msg, "deadlock") ||
		strings.Contains(msg, "lock wait timeout") ||
		strings.Contains(msg, "too many connections")
}

// Stats devuelve el estado actual de la cola (para el panel y las pruebas).
func (q *Queue) Stats() map[string]int {
	out := map[string]int{}
	rows, err := q.store.Query(`SELECT status, COUNT(*) FROM queue_tasks GROUP BY status`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var n int
		if rows.Scan(&s, &n) == nil {
			out[s] = n
		}
	}
	return out
}

// ---------- internos ----------

// signal despierta a un worker sin bloquear: el canal tiene tamaño 1.
func (q *Queue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// syncPending recalcula el tamaño de la cola desde disco, para que el
// backpressure sea correcto incluso tras un reinicio.
func (q *Queue) syncPending() {
	var n int
	_ = q.store.QueryRow(`SELECT COUNT(*) FROM queue_tasks WHERE status='pending'`).Scan(&n)
	q.mu.Lock()
	q.pending = n
	q.mu.Unlock()
}

// worker es un consumidor: espera una señal (o el poll) y toma una tarea.
func (q *Queue) worker(id int) {
	defer q.wg.Done()
	ticker := time.NewTicker(q.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-q.stop:
			return
		case <-q.wake:
		case <-ticker.C:
		}
		for {
			t, ok := q.take()
			if !ok {
				break
			}
			q.run(t)
		}
	}
}

// take reclama la siguiente tarea madura. La comparación de not_before se hace
// en Go (no en SQL) para no depender del formato de fecha del backend.
func (q *Queue) take() (Task, bool) {
	rows, err := q.store.Query(`SELECT id, kind, tenant, payload, attempts, not_before
		FROM queue_tasks WHERE status='pending' ORDER BY not_before, id LIMIT 20`)
	if err != nil {
		return Task{}, false
	}
	defer rows.Close()

	now := nowStr()
	for rows.Next() {
		var t Task
		var payload, notBefore string
		// El payload se escanea a string y se convierte después: escanearlo
		// directo a json.RawMessage falla con el driver SQLite y, si el error
		// se ignorara, la tarea se descartaría en silencio para siempre.
		if err := rows.Scan(&t.ID, &t.Kind, &t.Tenant, &payload, &t.Attempts, &notBefore); err != nil {
			fmt.Printf("queue: tarea ilegible, se omite: %v\n", err)
			continue
		}
		t.Payload = json.RawMessage(payload)
		// Todavía no maduró (reintento con espera): se salta.
		if notBefore != "" && notBefore > now {
			continue
		}
		// Reclamar atómicamente: el UPDATE con la condición status='pending'
		// hace que solo un worker gane la carrera.
		res, err := q.store.Exec(`UPDATE queue_tasks SET status='running', attempts=attempts+1, updated_at=?
			WHERE id=? AND status='pending'`, nowStr(), t.ID)
		if err != nil {
			fmt.Printf("queue: no se pudo reclamar %s: %v\n", t.ID, err)
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // otro worker se adelantado
		}
		t.Attempts++
		q.mu.Lock()
		if q.pending > 0 {
			q.pending--
		}
		q.mu.Unlock()
		return t, true
	}
	return Task{}, false
}

// run ejecuta una tarea con timeout y registra el resultado.
func (q *Queue) run(t Task) {
	q.mu.Lock()
	h := q.handlers[t.Kind]
	q.mu.Unlock()

	var err error
	if h == nil {
		err = ErrNoHandler
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), q.cfg.TaskTimeout)
		defer cancel()
		ctx = WithTenant(ctx, t.Tenant)
		err = h(ctx, t)
	}

	now := nowStr()
	if err == nil {
		_, _ = q.store.Exec(`UPDATE queue_tasks SET status='done', last_error='', updated_at=? WHERE id=?`, now, t.ID)
		return
	}

	// Reintento con espera exponencial acotada; al agotar, queda failed para
	// que un humano la revise. Nunca se reintenta infinitamente.
	if t.Attempts < q.cfg.MaxAttempts {
		backoff := time.Duration(1<<(t.Attempts-1)) * q.cfg.RetryBackoff
		retryAt := time.Now().Add(backoff).Format(nowLayout)
		_, _ = q.store.Exec(`UPDATE queue_tasks SET status='pending', last_error=?, not_before=?, updated_at=? WHERE id=?`,
			err.Error(), retryAt, now, t.ID)
		q.mu.Lock()
		q.pending++
		q.mu.Unlock()
		q.signal()
		return
	}
	_, _ = q.store.Exec(`UPDATE queue_tasks SET status='failed', last_error=?, updated_at=? WHERE id=?`,
		err.Error(), now, t.ID)
}

func nowStr() string { return time.Now().Format(nowLayout) }

const nowLayout = "2006-01-02 15:04:05"
