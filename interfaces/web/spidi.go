package web

// spidi.go — Integración con el agente de pagos SpidiAgent
// (orcanet24/SpidiAgent).
//
// La pasarela de pagos SPIDI tampoco vive dentro de este ERP: es un servicio
// externo (binario propio, SQLite de auditoría) al que los proyectos entran
// con una clave `X-Tenant-Key`. Este archivo es el CLIENTE del agente y la
// bitácora local de sesiones de pago; nada de SPIDI se teje en la fuente.
//
// Reparto de responsabilidades:
//
//   - Las credenciales reales de SPIDI viven y se cifran dentro del agente
//     (AES-256-GCM). El ERP solo lleva la clave de su tenant y su tenant_id:
//     un ERP vulnerado no entrega las llaves del banco.
//   - El agente NO envía webhooks hacia el ERP (su /webhook solo recibe a
//     SPIDI): por eso el ERP CONSULTA el estado del pago. El evento de
//     pago automático queda para cuando el agente exponga callbacks hacia
//     nosotros.
//   - La fila de spidi_pago es el registro de la SESIÓN de cobro, no el
//     asiento contable: reflejar en contabilidad un pago confirmado es una
//     decisión con factura y libro en medio, y en esta fase no se hace
//     implícitamente. El estado que aquí se guarda es el que el agente dice.
//
// Variables de entorno (configs/config.go):
//   SPIDI_ENABLED       1 activa el módulo (default 0)
//   SPIDI_AGENT_URL     base del agente, ej. http://127.0.0.1:8484
//   SPIDI_TENANT_ID     el tenant del ERP en el agente (ej. "erpgo")
//   SPIDI_TENANT_KEY    la X-Tenant-Key que le asignó el agente

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erpgo/configs"
	"erpgo/infrastructure/persistence"
)

// ---------- Esquema local de sesiones de pago ----------

// spidiSchema define la bitácora de sesiones de pago de la EMPRESA: es el
// registro de la sesión (quién paga, cuánto, con qué link), no de contabilidad.
// Vive en la base de cada empresa (dbFor), junto a sus facturas: su cobro es
// suyo y dos empresas no comparten sesiones de pago.
//
// Dialecto MySQL (la base de empresa de producción es MySQL), con
// CREATE TABLE IF NOT EXISTS: en las pruebas SQLite la tabla se crea ANTES
// con dialecto SQLite (spidi_test.go), y con IF NOT EXISTS el DDL no se
// vuelve a ejecutar: el dialecto del DDL solo importa la primera vez, que
// siempre es MySQL real. El índice va sin IF NOT EXISTS porque MySQL no lo
// admite: AplicarEsquema descarta el error de "ya estaba", como el resto
// del ERP.
var spidiSchema = []string{
	`CREATE TABLE IF NOT EXISTS spidi_pago (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		session_id VARCHAR(64) NOT NULL,
		invoice_id BIGINT NOT NULL,
		monto DECIMAL(18,2) NOT NULL DEFAULT 0,
		moneda VARCHAR(5) NOT NULL DEFAULT '',
		orden VARCHAR(100) NOT NULL DEFAULT '',
		estado VARCHAR(24) NOT NULL DEFAULT 'pending',
		payment_url TEXT NOT NULL DEFAULT '',
		creado_at VARCHAR(32) NOT NULL DEFAULT '',
		actualizado_at VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	`CREATE UNIQUE INDEX idx_spidi_pago_sesion ON spidi_pago(session_id)`,
}

// EnsureSpidiSchema crea la bitácora de pagos si falta. Idempotente.
func EnsureSpidiSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("esquema spidi: sin base de datos")
	}
	if err := persistence.AplicarEsquema(db, spidiSchema); err != nil {
		return fmt.Errorf("esquema spidi: %w", err)
	}
	return nil
}

// ---------- Cliente del agente ----------

// spidiSesion es la respuesta del agente al crear y al consultar un pago
// (contrato del agente: {"success":true,"data":{"session_id":...,
// "payment_url":...,"status":...}}).
type spidiSesion struct {
	SessionID  string
	PaymentURL string
	Status     string
}

// spidiAgento es el cliente HTTP hacia el agente de pagos.
type spidiAgento struct {
	base        string
	tenantID    string
	claveTenant string
	cli         *http.Client
}

func nuevoAgento(cfg *configs.Config) *spidiAgento {
	return &spidiAgento{
		base:        strings.TrimRight(cfg.SpidiAgentURL, "/"),
		tenantID:    cfg.SpidiTenantID,
		claveTenant: cfg.SpidiTenantKey,
		// 30s: crear un pago no debe tumbar la pantalla de ventas si el
		// agente amanece lento.
		cli: &http.Client{Timeout: 30 * time.Second},
	}
}

// pedir ejecuta UNA llamada al agente y la desenrolla en la forma de su API:
// {"success":bool,"data":{...},"error"?"message"?}. Un success=false del
// agente se convierte en error con su mensaje (los mensajes del agente ya
// explican el por qué: beneficiarse de la trazabilidad es no tragarlos).
func (a *spidiAgento) pedir(metodo, ruta, cuerpo string) (*spidiSesion, error) {
	var lector io.Reader
	if cuerpo != "" {
		lector = strings.NewReader(cuerpo)
	}
	req, err := http.NewRequest(metodo, a.base+ruta, lector)
	if err != nil {
		return nil, fmt.Errorf("solicitud al agente inválida: %w", err)
	}
	req.Header.Set("X-Tenant-Key", a.claveTenant)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("el agente de pagos no respondió (%s): %w", a.base, err)
	}
	defer resp.Body.Close()
	grano, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta del agente: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("el agente respondió %d: %s", resp.StatusCode, string(grano))
	}
	var envuelto struct {
		Success bool `json:"success"`
		Data    struct {
			SessionID  string `json:"session_id"`
			PaymentURL string `json:"payment_url"`
			Status     string `json:"status"`
		} `json:"data"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(grano, &envuelto); err != nil {
		return nil, fmt.Errorf("la respuesta del agente no es JSON del contrato: %s", string(grano))
	}
	if !envuelto.Success {
		if envuelto.Error != "" {
			return nil, fmt.Errorf("el agente rechazó la operación: %s", envuelto.Error)
		}
		if envuelto.Message != "" {
			return nil, fmt.Errorf("el agente rechazó la operación: %s", envuelto.Message)
		}
		return nil, fmt.Errorf("el agente rechazó la operación sin explicar el por qué")
	}
	return &spidiSesion{
		SessionID:  envuelto.Data.SessionID,
		PaymentURL: envuelto.Data.PaymentURL,
		Status:     envuelto.Data.Status,
	}, nil
}

// CrearPago abre una sesión de cobro en el agente (POST /pagos). La orden de
// referencia la pone el que la llama (aquí: la factura).
func (a *spidiAgento) CrearPago(monto float64, moneda, orden string) (*spidiSesion, error) {
	cuerpo, err := json.Marshal(map[string]interface{}{
		"tenant_id": a.tenantID,
		"monto":     monto,
		"moneda":    moneda,
		"orden":     orden,
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo armar el pedido de pago: %w", err)
	}
	return a.pedir(http.MethodPost, "/pagos", string(cuerpo))
}

// EstadoPago consulta el estado de UNA sesión (GET /pagos/:id).
func (a *spidiAgento) EstadoPago(sessionID string) (*spidiSesion, error) {
	return a.pedir(http.MethodGet, "/pagos/"+sessionID, "")
}

// ---------- Rutas del ERP ----------

// registerSpidiRoutes publica los 3 endpoints del módulo. Todos atraviesan
// authMiddleware (sesión de ERP): un actor sin sesión no crea ni consulta
// pagos. El agente no entra aquí: no hay receptor, el ERP consulta.
//
//	POST /spidi/pagos/cobro-factura   abre una sesión de cobro por una factura
//	GET  /spidi/pagos/status          consulta sesiones (por factura o sesión)
//	GET  /spidi/status                diagnóstico de la integración
func (s *Server) registerSpidiRoutes() {
	// POST /spidi/pagos/cobro-factura — body {"invoice_id":N,"monto":opcional}.
	// Por defecto se cobra el SALDO DEUDOR de la factura (due_amount), que es
	// la pregunta natural de "quiero cobrar esta factura"; con "monto" se
	// admite un anticipo o cobro parcial explícito. Facturas anuladas no se
	// cobran por ninguna vía.
	s.mux.HandleFunc("POST /spidi/pagos/cobro-factura", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		if cfg == nil || !cfg.SpidiEnabled || strings.TrimSpace(cfg.SpidiAgentURL) == "" || strings.TrimSpace(cfg.SpidiTenantKey) == "" {
			jsonError(w, "SPIDI no está configurado: faltan SPIDI_ENABLED, SPIDI_AGENT_URL y SPIDI_TENANT_KEY", http.StatusBadRequest)
			return
		}
		var pedido struct {
			InvoiceID int64    `json:"invoice_id"`
			Monto     *float64 `json:"monto"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&pedido); err != nil {
			jsonError(w, "Cuerpo inválido: se espera {\"invoice_id\": <número>}", http.StatusBadRequest)
			return
		}
		if pedido.InvoiceID <= 0 {
			jsonError(w, "Falta invoice_id de la factura a cobrar", http.StatusBadRequest)
			return
		}

		db := s.dbFor(r)

		// La lectura de la factura NO depende de la bitácora: se hace primero
		// (fallar el cobro por un DDL sería reordenar el problema). El Ensure
		// de la bitácora solo ocurre si al INSERTAR resulta que la tabla aún
		// no existe (primera cobranza de la instalación) — ver abajo.
		var saldo float64
		var moneda string
		var anulada int
		err := db.QueryRow(`SELECT due_amount, currency_code, is_anulled FROM invoice WHERE invoice_id = ?`,
			pedido.InvoiceID).Scan(&saldo, &moneda, &anulada)
		if err == sql.ErrNoRows {
			jsonError(w, "Esa factura no existe en esta empresa", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("spidi cobro-factura: consulta de factura falló: %v", err)
			jsonError(w, "No se pudo leer la factura", http.StatusInternalServerError)
			return
		}
		if anulada == 1 {
			jsonError(w, "La factura está anulada: no se abre cobro contra una factura anulada", http.StatusBadRequest)
			return
		}
		monto := saldo
		if pedido.Monto != nil {
			monto = *pedido.Monto
		}
		if monto <= 0 {
			jsonError(w, "No hay saldo por cobrar en esa factura (envía \"monto\" para un cobro parcial explícito)", http.StatusBadRequest)
			return
		}
		if moneda == "" {
			moneda = "USD"
		}

		ses, err := nuevoAgento(cfg).CrearPago(monto, moneda, fmt.Sprintf("FACT-%d", pedido.InvoiceID))
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		if strings.TrimSpace(ses.SessionID) == "" {
			jsonError(w, "El agente no devolvió session_id: no se puede registrar la sesión", http.StatusBadGateway)
			return
		}
		ahora := time.Now().UTC().Format("2006-01-02 15:04:05")
		respuesta := map[string]interface{}{
			"session_id":  ses.SessionID,
			"payment_url": ses.PaymentURL,
			"estado":      estadoOpendiente(ses.Status),
			"monto":       monto,
			"moneda":      moneda,
		}
		// La bitácora se garantiza EN EL MOMENTO de escribir: si la tabla aún
		// no existe (primera cobranza de esta instalación), se crea y se
		// reintenta el mismo INSERT una sola vez. Correr el DDL antes de cada
		// cobro obligaría a todo el dialecto de la sentencia en cada llamada,
		// que es justo lo que no hace el resto del ERP (ver yummy.go).
		agregar := func() (sql.Result, error) {
			return db.Exec(`INSERT INTO spidi_pago
				(session_id, invoice_id, monto, moneda, orden, estado, payment_url, creado_at, actualizado_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ses.SessionID, pedido.InvoiceID, monto, moneda, fmt.Sprintf("FACT-%d", pedido.InvoiceID),
				estadoOpendiente(ses.Status), ses.PaymentURL, ahora, ahora)
		}
		_, err = agregar()
		if err != nil && esErrorNoExisteTabla(err) {
			if errC := EnsureSpidiSchema(db); errC != nil {
				log.Printf("spidi cobro-factura: %v", errC)
				jsonError(w, "No se pudo preparar la bitácora de pagos SPIDI", http.StatusInternalServerError)
				return
			}
			_, err = agregar()
		}
		if err != nil {
			if persistence.EsErrorYaExiste(err) {
				// La misma sesión dos veces: no es error, es la segunda
				// confirmación de la misma operación. Se responde con la URL
				// de la sesión que ya en la bitácora.
				writeJSON(w, respuesta)
				return
			}
			log.Printf("spidi cobro-factura: insert falló: %v", err)
			jsonError(w, "No se pudo registrar la sesión de pago", http.StatusInternalServerError)
			return
		}
		writeJSON(w, respuesta)
	})

	// GET /spidi/pagos/status — ?invoice_id=N (todas las sesiones de la
	// factura) o ?session_id=abc (una). Consulta al agente y actualiza el
	// estado local: idempotente, y NO toca contabilidad (ver cabecera del
	// archivo).
	s.mux.HandleFunc("GET /spidi/pagos/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		if cfg == nil || !cfg.SpidiEnabled || strings.TrimSpace(cfg.SpidiAgentURL) == "" || strings.TrimSpace(cfg.SpidiTenantKey) == "" {
			jsonError(w, "SPIDI no está configurado: faltan SPIDI_ENABLED, SPIDI_AGENT_URL y SPIDI_TENANT_KEY", http.StatusBadRequest)
			return
		}
		db := s.dbFor(r)
		filas, err := filasDeSesion(db, r.URL.Query().Get("invoice_id"), r.URL.Query().Get("session_id"))
		if err != nil {
			jsonError(w, "No se pudo leer la bitácora de pagos: "+err.Error(), http.StatusInternalServerError)
			return
		}
		agento := nuevoAgento(cfg)
		hoy := time.Now().UTC().Format("2006-01-02 15:04:05")
		for _, fila := range filas {
			if ses, err := agento.EstadoPago(fila.SessionID); err == nil && ses.Status != "" && ses.Status != fila.Estado {
				if _, err := db.Exec(`UPDATE spidi_pago SET estado = ?, actualizado_at = ? WHERE session_id = ?`,
					ses.Status, hoy, fila.SessionID); err == nil {
					fila.Estado = ses.Status
				}
			}
		}
		pagos := []map[string]interface{}{}
		for _, fila := range filas {
			pagos = append(pagos, map[string]interface{}{
				"session_id":  fila.SessionID,
				"invoice_id":  fila.InvoiceID,
				"monto":       fila.Monto,
				"moneda":      fila.Moneda,
				"estado":      fila.Estado,
				"payment_url": fila.PaymentURL,
			})
		}
		writeJSON(w, map[string]interface{}{"pagos": pagos})
	})

	// GET /spidi/status — diagnóstico: config y últimas sesiones (sin
	// consultar al agente, para poder abrirlo aunque SPIDI esté caído).
	s.mux.HandleFunc("GET /spidi/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		res := map[string]interface{}{
			"enabled":        cfg != nil && cfg.SpidiEnabled,
			"agent_url_set":  cfg != nil && strings.TrimSpace(cfg.SpidiAgentURL) != "",
			"tenant_key_set": cfg != nil && strings.TrimSpace(cfg.SpidiTenantKey) != "",
			"tenant_id_set":  cfg != nil && strings.TrimSpace(cfg.SpidiTenantID) != "",
		}
		pagos := []map[string]interface{}{}
		if cfg != nil && cfg.SpidiEnabled {
			if filas, err := filasDeSesion(s.dbFor(r), "", ""); err == nil {
				for _, fila := range filas {
					pagos = append(pagos, map[string]interface{}{
						"session_id": fila.SessionID,
						"invoice_id": fila.InvoiceID,
						"monto":      fila.Monto,
						"moneda":     fila.Moneda,
						"estado":     fila.Estado,
						"creado_at":  fila.CreadoAt,
					})
				}
			}
		}
		res["pagos"] = pagos
		writeJSON(w, res)
	})
}

// spidiFila es una sesión de pago en la bitácora local.
type spidiFila struct {
	SessionID  string
	InvoiceID  int64
	Monto      float64
	Moneda     string
	Estado     string
	PaymentURL string
	CreadoAt   string
}

// filasDeSesion lee de la bitácora local, opcionalmente filtrado. Sin
// filtros trae las últimas 20: el diagnóstico no debe arrastrar meses de
// cobros de una empresa con volumen.
func filasDeSesion(db *sql.DB, invoiceID, sessionID string) ([]*spidiFila, error) {
	consulta := `SELECT session_id, invoice_id, monto, moneda, estado, payment_url, creado_at FROM spidi_pago`
	var condiciones []string
	var params []interface{}
	if idTxt := strings.TrimSpace(invoiceID); idTxt != "" {
		id, err := strconv.ParseInt(idTxt, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invoice_id no es numérico")
		}
		condiciones = append(condiciones, "invoice_id = ?")
		params = append(params, id)
	}
	if sid := strings.TrimSpace(sessionID); sid != "" {
		condiciones = append(condiciones, "session_id = ?")
		params = append(params, sid)
	}
	if len(condiciones) > 0 {
		consulta += " WHERE " + strings.Join(condiciones, " AND ")
	}
	consulta += " ORDER BY id DESC LIMIT 20"
	rows, err := db.Query(consulta, params...)
	if err != nil {
		if !esErrorNoExisteTabla(err) {
			return nil, err
		}
		// Primera consulta de instalación: se crea la bitácora y se intenta
		// de una vez (el Ensure en producción solo corre aquí, no en cada
		// llamada).
		if errC := EnsureSpidiSchema(db); errC != nil {
			return nil, errC
		}
		rows, err = db.Query(consulta, params...)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	filas := []*spidiFila{}
	for rows.Next() {
		var f spidiFila
		if err := rows.Scan(&f.SessionID, &f.InvoiceID, &f.Monto, &f.Moneda, &f.Estado, &f.PaymentURL, &f.CreadoAt); err == nil {
			filas = append(filas, &f)
		}
	}
	return filas, rows.Err()
}

// estadoOpendiente devuelve el estado del agente o "pending" si viniera
// vacío: la bitácora local nunca queda sin estado legible por una respuesta
// corta del agente.
func estadoOpendiente(estado string) string {
	if strings.TrimSpace(estado) == "" {
		return "pending"
	}
	return estado
}
