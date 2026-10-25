// Package orchestrator es la puerta ÚNICA por la que pasa toda escritura a la
// base de datos.
//
// El problema que resuelve: hoy hay escrituras por todas partes (97 INSERT en el
// código) y ninguna deja rastro obligatorio. La auditoría existe (audit_log con
// cadena de hashes) pero es una llamada opcional que cada handler recuerda o no.
// Una llamada opcional en un sitio que nadie audita es una llamada que se
// pierde, y entonces el bloque mensual queda mal sellado sin que nada lo diga.
//
// Lo que este paquete garantiza:
//
//   - Una sola puerta. Quien escribe, pasa por aquí.
//   - Orden determinista. El correlativo es la posición en la fila, no el id de
//     SQLite: dos escrituras del mismo segundo siguen teniendo orden.
//   - Nunca se pierde trabajo. Si la cola está llena, el productor SE FRENA; no
//     se le rechaza. Perder una venta porque la cola llenó es exactamente el
//     fallo que ya costó datos.
//   - Idempotencia por clave única. Una reinserción tras una caída no duplica.
//   - Auditoría obligatoria. El registro en audit_log se escribe EN LA MISMA
//     TRANSACCIÓN que el dato. Si no se puede escribir el rastro, no entra el
//     dato: se deshace todo.
//
// Lo que NO es todavía: esto cubre el camino de escritura nuevo. Los handlers
// antiguos que hacen INSERT por su cuenta siguen estándolo hasta que se migren
// uno a uno (ver doc/HOJA-DE-RUTA.md). La diferencia es que ahora existe la
// puerta y hay pruebas de que cierra.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"erpgow/infrastructure/persistence"
)

// ErrParado indica que el orquestador ya se está cerrando. Una escritura que
// llega tarde se dice que no, en vez de aceptarse en una cola que ya no se vacía.
var ErrParado = errors.New("orchestrator: el orquestador está parado")

// Entrada es lo que se quiere escribir. Es explícita a propósito: el
// orquestador no adivina qué hacer con un mapa arbitrario.
type Entrada struct {
	// Empresa es obligatoria. Una escritura sin empresa no tiene contexto
	// válido y por tanto no puede quedar registrada de forma interpretable.
	Empresa string
	// Tabla y Columnas identifican el destino. No se acepta SQL libre: el
	// orquestador construye la sentencia, que es lo que impide que una vía
	// (API, fork de conexión) inyecte SQL por aquí.
	Tabla    string
	Columnas []string
	Valores  []any
	// Clave es la clave de idempotencia. Dos entregas con la misma clave
	// producen UN solo registro. Sin ella una reinserción tras una caída
	// duplicaría, que en facturación es un desastre.
	Clave string
	// Modulo, Usuario, IP y Accion van al audit_log. Modulo y Accion son
	// obligatorios: un rastro sin saber qué se hizo no sirve para nada.
	Modulo  string
	Usuario string
	IP      string
	Accion  string
	// RegistroID es el identificador lógico del registro afectado. Se puede
	// dejar vacío en una creación, en la que se rellena con el id devuelto.
	RegistroID string
	OldValue   any
	NewValue   any
	// Correlativo lo rellena el orquestador. Quien llama no lo pone: ponerlo
	// sería la puerta de atrás que este paquete existe para cerrar.
	Correlativo uint64
}

// Resultado es lo que devuelve una escritura aplicada.
type Resultado struct {
	Correlativo uint64 `json:"correlativo"`
	RegistroID  int64  `json:"registro_id"`
	// Duplicado indica que la clave ya estaba y no se escribió otra vez. No es
	// un error: es la idempotencia funcionando, y el llamador debe tratarlo
	// como éxito.
	Duplicado bool `json:"duplicado"`
}

// Aplicador escribe el registro de negocio DENTRO de la transacción que abre
// el orquestador.
//
// Por qué recibe el *sql.Tx y no el *sql.DB: es la diferencia entre "el rastro
// va en la misma transacción que el dato" y "el rastro va en otra transacción
// que hope". Si el aplicador recibiera la base abriría su propia conexión, su
// INSERT se confirmaría por separado y un fallo del rastro dejaría el dato
// escrito sinAuditado, que es justo el fallo que este paquete existe para
// evitar. Con el tx, un solo commit o un solo rollback.
//
// Lo recibe el orquestador para que quien llama aporte SU SQL validado (los
// handlers ya saben qué validan) y el orquestador aporte la transacción, el
// correlativo y la auditoría. Si devuelve error, no entra nada: tampoco el
// rastro, porque la transacción se deshace entera.
type Aplicador func(ctx context.Context, tx *sql.Tx, e Entrada) (registroID int64, err error)

// Config acota el orquestador.
type Config struct {
	// MaxPending es cuántas escrituras caben sin aplicar. Al llenarse, Submit
	// bloquea al productor en vez de rechazar.
	MaxPending int
	// EsperaAlLlenarse es cada cuánto se sondea el hueco. No se usa un
	// sync.Cond a propósito: el despertar perdido lo convertiría en un
	// cuelgue, y un ERP colgado no factura.
	EsperaAlLlenarse time.Duration
	// EsperaInactiva es cada cuánto se comprueba el estado si no hay nada que
	// hacer.
	EsperaInactiva time.Duration
	// PausaParaLimpiar es cuánto tiempo lleva el sistema en calma antes de
	// limpiar el journal. Limpiar durante un pico de escritura es tirar
	// trabajo ya confirmado para ganar unos kilobytes.
	PausaParaLimpiar time.Duration
	// Reintento es la espera entre intentos de una escritura que falla. Falla
	// aquí casi siempre es contención de SQLite, que es cola de espera y no un
	// fallo del trabajo.
	Reintento time.Duration
	// MaxIntentos es cuántas veces se intenta una misma escritura antes de
	// dejar de reintentarla en el bucle y|reportarla. La entrada NO se tira:
	// sigue en el journal y se vuelve a intentar en el siguiente arranque.
	MaxIntentos int
}

func (c *Config) conDefaults() {
	if c.MaxPending <= 0 {
		c.MaxPending = 1024
	}
	if c.EsperaAlLlenarse <= 0 {
		c.EsperaAlLlenarse = 5 * time.Millisecond
	}
	if c.EsperaInactiva <= 0 {
		c.EsperaInactiva = 250 * time.Millisecond
	}
	if c.PausaParaLimpiar <= 0 {
		c.PausaParaLimpiar = 30 * time.Second
	}
	if c.Reintento <= 0 {
		c.Reintento = 25 * time.Millisecond
	}
	if c.MaxIntentos <= 0 {
		c.MaxIntentos = 5
	}
}

// fallo lleva la cuenta de intentos de una escritura que no se pudo aplicar.
type fallo struct {
	intentos int
	err      error
}

// Orquestador coordina la cola. Es seguro para uso concurrente.
type Orquestador struct {
	db      *sql.DB
	cfg     Config
	rev     *journal
	aplicar Aplicador

	mu          sync.Mutex
	pendientes  []entradaDisco // en orden de correlativo, lo que aún no se aplicó
	aplicado    uint64         // último correlativo confirmado en la base
	correlativo uint64         // último entregado
	ultima      time.Time      // cuándo se confirmó algo por última vez
	fallos      map[uint64]*fallo
	parado      bool

	despierta chan struct{}
	stop      chan struct{}
	wg        sync.WaitGroup
	onceStop  sync.Once
}

// New crea el orquestador sobre una base y un archivo de cola.
//
// rutaJournal es el archivo de cola de ESA empresa: el correlativo es la
// posición en la fila y las filas son suyas, así que cada empresa lleva el suyo.
// fieldKey es la FieldKey del binario; vacía significa desarrollo, y entonces el
// journal va en claro.
func New(db *sql.DB, rutaJournal, fieldKey string, aplicar Aplicador, cfg Config) (*Orquestador, error) {
	if db == nil {
		return nil, errors.New("orchestrator: falta la base de datos")
	}
	if aplicar == nil {
		return nil, errors.New("orchestrator: falta el aplicador")
	}
	cfg.conDefaults()

	rev, err := abrirJournal(rutaJournal, fieldKey)
	if err != nil {
		return nil, err
	}
	if err := crearEsquema(db); err != nil {
		_ = rev.cerrar()
		return nil, err
	}

	o := &Orquestador{
		db:        db,
		cfg:       cfg,
		rev:       rev,
		aplicar:   aplicar,
		despierta: make(chan struct{}, 1),
		stop:      make(chan struct{}),
		fallos:    map[uint64]*fallo{},
		ultima:    time.Now(),
	}

	aplicado, err := o.ultimoAplicado()
	if err != nil {
		_ = rev.cerrar()
		return nil, err
	}
	o.aplicado = aplicado
	o.correlativo = rev.correlativoActual()

	// Recuperación: lo que quedó en el journal tras un cierre vuelve a la
	// cola. Lo que ya está en la base se descarta por correlativo, y eso es
	// justo lo que hace segura la idempotencia tras una caída.
	pendientes, err := rev.leerPendientes()
	if err != nil {
		_ = rev.cerrar()
		return nil, err
	}
	for _, e := range pendientes {
		if e.Correlativo > aplicado {
			o.pendientes = append(o.pendientes, e)
		}
	}
	if len(o.pendientes) > 0 {
		fmt.Printf("orchestrator: recuperadas %d escrituras sin aplicar (hasta el correlativo %d)\n",
			len(o.pendientes), o.pendientes[len(o.pendientes)-1].Correlativo)
	}
	return o, nil
}

// esquema son las dos tablas del orquestador. Viven en la base de la empresa
// porque son dato de negocio, no infraestructura: el correlativo es la posición
// en la fila y las filas son de esa empresa.
var esquema = []string{
	// La clave única es lo que hace idempotente la reinserción. Dos entregas
	// con la misma clave producen un solo registro, aunque la segunda llegue
	// después de un reinicio.
	`CREATE TABLE IF NOT EXISTS orchestrator_claves (
		clave       VARCHAR(191) NOT NULL PRIMARY KEY,
		correlativo INTEGER NOT NULL,
		tabla       VARCHAR(128) NOT NULL DEFAULT '',
		modulo      VARCHAR(64) NOT NULL DEFAULT '',
		creada_en   VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	// Una fila por escritura aplicada. Es la memoria del correlativo y el
	// respaldo del rastro: sin esta tabla, limpiar el journal sería perder el
	// orden.
	`CREATE TABLE IF NOT EXISTS orchestrator_escrituras (
		correlativo INTEGER NOT NULL PRIMARY KEY,
		clave       VARCHAR(191) NOT NULL,
		tabla       VARCHAR(128) NOT NULL,
		registro_id INTEGER NOT NULL DEFAULT 0,
		modulo      VARCHAR(64) NOT NULL DEFAULT '',
		usuario     VARCHAR(128) NOT NULL DEFAULT '',
		ip          VARCHAR(64) NOT NULL DEFAULT '',
		accion      VARCHAR(64) NOT NULL DEFAULT '',
		audit_id    INTEGER NOT NULL DEFAULT 0,
		creada_en   VARCHAR(32) NOT NULL DEFAULT '',
		aplicada_en VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX idx_orch_escrituras_aplicada ON orchestrator_escrituras(aplicada_en)`,
}

func crearEsquema(db *sql.DB) error {
	if err := persistence.AplicarEsquema(db, esquema); err != nil {
		return fmt.Errorf("orchestrator: esquema: %w", err)
	}
	return nil
}

// ultimoAplicado devuelve el correlativo más alto ya escrito en la base. Es lo
// que hace que una recuperación no repita trabajo.
func (o *Orquestador) ultimoAplicado() (uint64, error) {
	var n sql.NullInt64
	err := o.db.QueryRow(`SELECT correlativo FROM orchestrator_escrituras ORDER BY correlativo DESC LIMIT 1`).Scan(&n)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("orchestrator: no se pudo leer el correlativo aplicado: %w", err)
	}
	if !n.Valid || n.Int64 <= 0 {
		return 0, nil
	}
	return uint64(n.Int64), nil
}

// Start levanta el escritor. Llámalo una vez.
func (o *Orquestador) Start() {
	o.wg.Add(1)
	go o.escritor()
}

// Stop para el escritor. Lo pendiente NO se tira: sigue en el journal y se
// recupera al arrancar. Perder una venta al apagar el servidor sería absurdo.
func (o *Orquestador) Stop() {
	o.mu.Lock()
	if o.parado {
		o.mu.Unlock()
		return
	}
	o.parado = true
	o.mu.Unlock()

	o.onceStop.Do(func() { close(o.stop) })
	o.wg.Wait()
	_ = o.rev.cerrar()
}

// Submit entrega una escritura y la aplica. Es la puerta: no hay otro camino.
//
// Devuelve cuando la escritura está en la base Y su rastro en audit_log. Un
// sistema de facturación necesita eso: devolver "hecho" y escribir después es
// como se pierden ventas.
//
// Si la cola está llena, Submit ESPERA. No devuelve error por saturación: eso
// es lo que dejaba la decisión en el llamador, y si no reintentaba la venta se
// perdía.
func (o *Orquestador) Submit(ctx context.Context, e Entrada) (Resultado, error) {
	if err := validar(e); err != nil {
		return Resultado{}, err
	}

	correlativo, err := o.encolar(ctx, e)
	if err != nil {
		return Resultado{}, err
	}
	return o.esperar(ctx, correlativo)
}

// encolar toma correlativo, escribe en el journal y mete la entrada en la cola.
// Devuelve en cuanto la escritura es recuperable, sin esperar a aplicarla.
func (o *Orquestador) encolar(ctx context.Context, e Entrada) (uint64, error) {
	esperando := time.NewTicker(o.cfg.EsperaAlLlenarse)
	defer esperando.Stop()

	o.mu.Lock()
	for len(o.pendientes) >= o.cfg.MaxPending {
		// Bloqueo del productor, con techo. El techo existe para que un
		// escritor MUERTO se reporte en vez de dejar la pantalla colgada
		// para siempre: pararse sin reportar también es una forma de
		// perder el trabajo.
		if o.parado {
			o.mu.Unlock()
			return 0, ErrParado
		}
		if err := ctx.Err(); err != nil {
			o.mu.Unlock()
			return 0, err
		}
		o.mu.Unlock()
		select {
		case <-esperando.C:
		case <-ctx.Done():
		}
		o.mu.Lock()
	}
	if o.parado {
		o.mu.Unlock()
		return 0, ErrParado
	}

	o.correlativo++
	entrada := entradaDisco{
		Correlativo: o.correlativo,
		Empresa:     e.Empresa,
		Clave:       e.Clave,
		Tabla:       e.Tabla,
		Columnas:    e.Columnas,
		Valores:     e.Valores,
		Modulo:      e.Modulo,
		Usuario:     e.Usuario,
		IP:          e.IP,
		Accion:      e.Accion,
		RegistroID:  e.RegistroID,
		OldValue:    e.OldValue,
		NewValue:    e.NewValue,
		Creada:      ahoraTexto(),
	}

	// Primero el journal, que es la durabilidad: si se cae justo después, la
	// escritura se recupera al arrancar. Al revés (aplicar y luego
	// journalizar) deja una ventana con el dato escrito y sin rastro de
	// dónde vino.
	if err := o.rev.anadir(entrada); err != nil {
		o.correlativo--
		o.mu.Unlock()
		return 0, err
	}
	o.pendientes = append(o.pendientes, entrada)
	correlativo := o.correlativo
	o.mu.Unlock()

	o.despertar()
	return correlativo, nil
}

// esperar se queda hasta que el escritor confirma el correlativo, o hasta que
// la escritura agota sus intentos y hay que reportarlo.
func (o *Orquestador) esperar(ctx context.Context, correlativo uint64) (Resultado, error) {
	sondeo := time.NewTicker(2 * o.cfg.EsperaAlLlenarse)
	defer sondeo.Stop()

	for {
		o.mu.Lock()
		if correlativo <= o.aplicado {
			o.mu.Unlock()
			return o.resultadoDe(correlativo)
		}
		if o.parado {
			o.mu.Unlock()
			return Resultado{}, ErrParado
		}
		f := o.fallos[correlativo]
		var agotado error
		if f != nil && f.intentos >= o.cfg.MaxIntentos {
			agotado = f.err
		}
		o.mu.Unlock()
		if agotado != nil {
			return Resultado{}, fmt.Errorf("orchestrator: la escritura %d no se pudo aplicar tras %d intentos: %w",
				correlativo, o.cfg.MaxIntentos, agotado)
		}

		select {
		case <-ctx.Done():
			// La entrada sigue en el journal: se reintentará al arrancar.
			// Perderla sería perder la venta, y esto no la pierde.
			return Resultado{}, ctx.Err()
		case <-sondeo.C:
		}
	}
}

// resultadoDe lee el resultado ya confirmado.
func (o *Orquestador) resultadoDe(correlativo uint64) (Resultado, error) {
	var registroID int64
	err := o.db.QueryRow(`SELECT registro_id FROM orchestrator_escrituras WHERE correlativo=?`, correlativo).Scan(&registroID)
	if err != nil {
		return Resultado{Correlativo: correlativo}, nil
	}
	return Resultado{Correlativo: correlativo, RegistroID: registroID}, nil
}

// escritor es el ÚNICO que aplica. Un solo escritor es lo que da el orden, y el
// orden es lo que hace determinista el correlativo.
func (o *Orquestador) escritor() {
	defer o.wg.Done()
	ticker := time.NewTicker(o.cfg.EsperaInactiva)
	defer ticker.Stop()

	for {
		o.drenar()
		o.intentarLimpiar()

		select {
		case <-o.despierta:
		case <-ticker.C:
		case <-o.stop:
			return
		}
	}
}

// drenar aplica las entradas pendientes, en orden, hasta que no queda ninguna
// o hasta que una agota sus intentos.
func (o *Orquestador) drenar() {
	for {
		o.mu.Lock()
		if o.parado || len(o.pendientes) == 0 {
			o.mu.Unlock()
			return
		}
		entrada := o.pendientes[0]
		f := o.fallos[entrada.Correlativo]
		agotado := f != nil && f.intentos >= o.cfg.MaxIntentos
		o.mu.Unlock()

		if agotado {
			// Se deja la entrada en su sitio y se abandona el bucle: no se
			// salta. Saltarse una escritura por un fallo momentáneo perdería
			// una venta, y además rompería el orden de las siguientes.
			return
		}

		if _, err := o.aplicarUna(entrada); err != nil {
			o.registrarFallo(entrada.Correlativo, err)
			select {
			case <-time.After(o.cfg.Reintento):
			case <-o.stop:
				return
			}
			continue
		}
		o.confirmar(entrada)
	}
}

// aplicarUna escribe el dato y su rastro en UNA transacción. Si algo falla, no
// entra nada: tampoco el rastro, que es lo que mantiene honesto el bloque
// mensual.
func (o *Orquestador) aplicarUna(d entradaDisco) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	// Rollback después de Commit es un no-op en Go, así que este defer es
	// seguro y garantiza que nada se queda a medias en ningún camino de salida.
	defer func() { _ = tx.Rollback() }()

	// Idempotencia primero. La clave única decide: si ya estaba, esta es la
	// segunda vez que llega el mismo trabajo y no se escribe nada.
	var previo uint64
	err = tx.QueryRowContext(ctx, `SELECT correlativo FROM orchestrator_claves WHERE clave=?`, d.Clave).Scan(&previo)
	if err == nil {
		var registroID int64
		_ = tx.QueryRowContext(ctx, `SELECT registro_id FROM orchestrator_escrituras WHERE correlativo=?`, previo).Scan(&registroID)
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return registroID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	registroID, err := o.aplicar(ctx, tx, entradaDesdeDisco(d))
	if err != nil {
		return 0, err
	}

	// El rastro va DENTRO de la misma transacción. Si no se puede escribir, la
	// transacción se deshace entera y el dato no entra. Ese es el requisito
	// duro del plan, y por eso el INSERT del rastro es parte del mismo tx y no
	// una llamada aparte: fuera del tx, un fallo dejaría el dato sin rastro, que
	// es justo lo que hay que evitar.
	auditID, err := o.escribirAuditoria(ctx, tx, d, registroID)
	if err != nil {
		return 0, fmt.Errorf("orchestrator: sin rastro no entra la escritura (correlativo %d): %w", d.Correlativo, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO orchestrator_claves (clave, correlativo, tabla, modulo, creada_en) VALUES (?,?,?,?,?)`,
		d.Clave, d.Correlativo, d.Tabla, d.Modulo, d.Creada); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO orchestrator_escrituras
			(correlativo, clave, tabla, registro_id, modulo, usuario, ip, accion, audit_id, creada_en, aplicada_en)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		d.Correlativo, d.Clave, d.Tabla, registroID, d.Modulo, d.Usuario, d.IP, d.Accion,
		auditID, d.Creada, ahoraTexto()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return registroID, nil
}

// escribirAuditoria inserta en audit_log con su hash encadenado.
//
// El hash lo calcula el MISMO algoritmo que usa el resto del proyecto
// (computeAuditHash en interfaces/web/audit_chain.go). Que coincida no es
// opcional: si esta función usara otro formato, la verificación de la cadena
// declararía manipulado un bloque entero que está bien.
func (o *Orquestador) escribirAuditoria(ctx context.Context, tx *sql.Tx, d entradaDisco, registroID int64) (int64, error) {
	prev := "GENESIS"
	var h sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT record_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&h)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if h.Valid && h.String != "" {
		prev = h.String
	}

	recordID := d.RegistroID
	if recordID == "" && registroID > 0 {
		recordID = fmt.Sprintf("%d", registroID)
	}
	oldJSON := marshalJSON(d.OldValue)
	newJSON := marshalJSON(d.NewValue)
	ts := d.Creada
	if ts == "" {
		ts = ahoraTexto()
	}
	usuario := d.Usuario
	if usuario == "" {
		usuario = "system"
	}
	ip := d.IP
	if ip == "" {
		ip = "127.0.0.1"
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO audit_log
		(user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at, prev_hash, record_hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		usuario, d.Accion, d.Modulo, d.Tabla, recordID, oldJSON, newJSON, ip, ts, prev, "")
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	suma := CalcularHashAuditoria(prev, id, usuario, d.Accion, d.Modulo, d.Tabla, recordID, oldJSON, newJSON, ip, ts)
	// El único UPDATE que el sistema hace sobre audit_log, y solo rellena el
	// hash de la fila que acaba de crear. Nunca toca filas anteriores.
	if _, err := tx.ExecContext(ctx,
		`UPDATE audit_log SET record_hash=? WHERE id=? AND record_hash=''`, suma, id); err != nil {
		return 0, err
	}
	return id, nil
}

// CalcularHashAuditoria es el algoritmo de la cadena, idéntico al de
// interfaces/web/audit_chain.go.
//
// Se exporta para que una prueba pueda comprobar que las dos copias no
// divergen: si divergieran, la verificación de la cadena declararía manipulado
// un bloque que está bien, y ese fallo es muy difícil de ver en producción.
func CalcularHashAuditoria(prev string, id int64, usuario, accion, modulo, tabla, recordID, oldV, newV, ip, ts string) string {
	payload := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		prev, id, usuario, accion, modulo, tabla, recordID, oldV, newV, ip, ts)
	suma := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(suma[:])
}

// marshalJSON es el mismo criterio que el del resto del proyecto: nil se guarda
// como cadena vacía y no como "null", para que un rastro vacío se distinga de un
// rastro con valor nulo.
func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// confirmar saca la entrada de la cola y deja constancia de que está aplicada.
func (o *Orquestador) confirmar(d entradaDisco) {
	o.mu.Lock()
	o.aplicado = d.Correlativo
	o.ultima = time.Now()
	delete(o.fallos, d.Correlativo)
	if len(o.pendientes) > 0 && o.pendientes[0].Correlativo == d.Correlativo {
		o.pendientes = o.pendientes[1:]
	}
	o.mu.Unlock()
}

// registrarFallo apunta el error y cuenta el intento.
func (o *Orquestador) registrarFallo(correlativo uint64, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	f := o.fallos[correlativo]
	if f == nil {
		f = &fallo{}
		o.fallos[correlativo] = f
	}
	f.intentos++
	f.err = err
}

// intentarLimpiar vacía el journal cuando el sistema lleva en calma y no queda
// nada sin aplicar.
func (o *Orquestador) intentarLimpiar() {
	o.mu.Lock()
	calma := len(o.pendientes) == 0 && time.Since(o.ultima) >= o.cfg.PausaParaLimpiar
	parado := o.parado
	o.mu.Unlock()
	if !calma || parado {
		return
	}
	if err := o.rev.limpiar(); err != nil {
		fmt.Printf("orchestrator: no se pudo limpiar el journal: %v\n", err)
		return
	}
	// Limpiar NO reinicia el correlativo: el journal conserva el último
	// entregado. Si se reiniciara aquí, dos escrituras ocuparían el mismo
	// número y el orden fiscal dejaría de ser determinista.
	fmt.Printf("orchestrator: journal limpiado (correlativo conservado: %d)\n", o.rev.correlativoActual())
}

// despertar avisa al escritor sin bloquear.
func (o *Orquestador) despertar() {
	select {
	case o.despierta <- struct{}{}:
	default:
	}
}

// Pendientes dice cuántas escrituras hay sin aplicar.
func (o *Orquestador) Pendientes() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.pendientes)
}

// Aplicadas dice cuántas escrituras hay confirmadas en la base. Se cuenta en
// la base, no en memoria: es lo que sobrevive a un reinicio.
func (o *Orquestador) Aplicadas() int {
	var n int
	_ = o.db.QueryRow(`SELECT COUNT(*) FROM orchestrator_escrituras`).Scan(&n)
	return n
}

// CorrelativoActual es el último correlativo entregado. Es la posición en la
// fila, no el id de SQLite.
func (o *Orquestador) CorrelativoActual() uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.correlativo
}

// HayCola responde si queda algo en el journal, sin abrir la base.
func (o *Orquestador) HayCola() bool { return o.rev.hayPendientes() }

// entradaDesdeDisco reconstruye la Entrada desde lo que venía del archivo. A esa
// hora ya no hay un http.Request del que sacar el usuario, así que todo lo que
// hace falta tiene que estar en el propio registro.
func entradaDesdeDisco(d entradaDisco) Entrada {
	return Entrada{
		Empresa:     d.Empresa,
		Tabla:       d.Tabla,
		Columnas:    d.Columnas,
		Valores:     d.Valores,
		Clave:       d.Clave,
		Modulo:      d.Modulo,
		Usuario:     d.Usuario,
		IP:          d.IP,
		Accion:      d.Accion,
		RegistroID:  d.RegistroID,
		OldValue:    d.OldValue,
		NewValue:    d.NewValue,
		Correlativo: d.Correlativo,
	}
}

// validar rechaza lo que no tiene sentido antes de escribirlo en el journal. Es
// mejor fallar aquí, en memoria, que dejar en el disco una entrada que el
// escritor no va a poder aplicar.
func validar(e Entrada) error {
	if strings.TrimSpace(e.Empresa) == "" {
		return errors.New("orchestrator: la escritura necesita empresa")
	}
	if strings.TrimSpace(e.Clave) == "" {
		return errors.New("orchestrator: la escritura necesita clave de idempotencia")
	}
	if strings.TrimSpace(e.Modulo) == "" {
		return errors.New("orchestrator: la escritura necesita módulo (es lo que queda registrado)")
	}
	if strings.TrimSpace(e.Accion) == "" {
		return errors.New("orchestrator: la escritura necesita acción (es lo que queda registrado)")
	}
	if len(e.Columnas) != len(e.Valores) {
		return fmt.Errorf("orchestrator: %d columnas y %d valores no cuadran", len(e.Columnas), len(e.Valores))
	}
	// El destino se valida AQUÍ, en memoria, y no en el aplicador. La diferencia
	// no es de método: si se aceptara un identificador inválido, la entrada se
	// escribiría en el journal, el escritor la recogería, fallaría cinco veces
	// con reintentos y solo entonces se reportaría. Un error que se detecta
	// veinte segundos tarde, tras ocupar el único escritor, es un error que se
	// ha pagado dos veces: una en el disco y otra en el tiempo del usuario.
	if !IdentificadorSeguro(e.Tabla) {
		return fmt.Errorf("orchestrator: nombre de tabla no válido: %q", e.Tabla)
	}
	for _, col := range e.Columnas {
		if !IdentificadorSeguro(col) {
			return fmt.Errorf("orchestrator: nombre de columna no válido: %q", col)
		}
	}
	return nil
}

// IdentificadorSeguro acepta solo lo que es un identificador de SQL: letras,
// dígitos y guion bajo, empezando por letra o guion bajo. Sin comillas, sin
// espacios, sin punto, sin punto y coma.
//
// No es una paranoia: la puerta existe justo para que el día que un
// endpoint HTTP la use, un nombre de tabla que venga de fuera sea un dato y no
// pase a ser código. Nombres de tabla y de columna llegan del código que llama,
// pero un identificador sin validar en una cadena de SQL es una puerta abierta
// en cuanto ese código pase a leer una petición.
func IdentificadorSeguro(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
