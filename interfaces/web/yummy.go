package web

// yummy.go — Integración con la pasarela YummyProxy2 (orcanet24/YummyProxy2).
//
// La pasarela de envíos NO vive dentro de este ERP: es un servicio aparte (su
// propio binario/Docker) que guarda la API key real de Yummy y expone un API
// simple por clave `X-Gateway-Key`. En lugar de tejer Yummy en la fuente del
// ERP —el mismo error que costó el SDK antiguo—, este archivo es el CLIENTE
// de esa pasarela y el receptor de los webhooks que ella entrega, nada más.
//
// Reparto de responsabilidades, y por qué es así:
//
//   - La clave de Yummy (`mk_live_…`) vive SOLO en la pasarela, cifrada allí.
//     El ERP solo conoce su clave de gateway (`gw_live_…`) y el secreto con
//     el que la pasarela firma las entregas. Un ERP que se filtra no filtra
//     la llave del vendedor de Yummy.
//   - La pasarela ya trae cola con reintentos hasta 24 h: si el ERP está
//     caído, la pasarela reintenta. Por eso este receptor es prácticamente
//     sin estado: verifica la firma, guarda el evento y responde rápido.
//     Nunca se procesa la carga dentro de la entrega: los eventos quedan
//     durables y el que los consume decide después.
//   - El receptor NO pide sesión de ERP: es máquina a máquina con la
//     pasarela. Su autenticación es la firma HMAC del evento; con el secreto
//     vacío o el módulo apagado responde 403 (fail-closed).
//
// Variables de entorno (configs/config.go):
//   YUMMY_ENABLED          1 activa el módulo (default 0, como ENABLE_OCR)
//   YUMMY_GATEWAY_URL      base de la pasarela, ej. http://gw.empresa:8080
//   YUMMY_GATEWAY_KEY      la clave gw_live_… que emitió el admin del gateway
//   YUMMY_CALLBACK_SECRET  el callback_secret del alta del tenant: con él se
//                          compone `HMAC-SHA256(seg, "<t>.<body>")` que firma
//                          la cabecera `X-Gateway-Signature: t=<unix>,v1=<hex>`.

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

// ---------- Esquema de los eventos recibidos ----------

// yummySchema define la bitácora de los webhooks que la pasarela entregó.
//
// Vive en la BASE DE CONTROL, en la clase de base donde ya viven otros ledgers
// de integración (digital_invoice_log): un evento de la pasarela no pertenece
// a la empresa activa del momento, sino a la instalación. La tabla se crea
// desde aquí y NO pasa por el conversor de migraciones, por la misma razón que
// ai_legal_rule: este DDL se ejecuta directo desde Go.
//
// Dialecto MySQL (la base de control de producción es MySQL): AUTO_INCREMENT,
// sin backticks de esmero no crítico, sin COMMENT. En las pruebas SQLite la
// tabla se crea ANTES (yummy_test.go) con dialecto SQLite: llegó con
// `IF NOT EXISTS`, que ambas bases toleran, y nunca se vuelve a
// ejecutar, así el dialecto del DDL solo importa la primera vez, que es
// siempre en MySQL real.
//
// event_id es ÚNICO por diseño de la pasarela (X-Gateway-Event-Id es estable
// entre reintentos): es la llave de idempotencia. Si el mismo evento llega
// dos veces —paso normal en el protocolo hasta confirmar la entrega— solo
// sobrevive una fila.
var yummySchema = []string{
	`CREATE TABLE IF NOT EXISTS yummy_webhook_evento (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		event_id VARCHAR(64) NOT NULL,
		event_type VARCHAR(120) NOT NULL DEFAULT '',
		attempt INT NOT NULL DEFAULT 0,
		payload TEXT NOT NULL,
		estado VARCHAR(20) NOT NULL DEFAULT 'recibido',
		recibido_at VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	// MySQL no admite IF NOT EXISTS en CREATE INDEX: AplicarEsquema descarta
	// el error de "ya estaba", que es la misma convención del resto del ERP.
	`CREATE UNIQUE INDEX idx_yummy_evento_unico ON yummy_webhook_evento(event_id)`,
}

// EnsureYummySchema crea la bitácora de eventos si falta. Idempotente.
func EnsureYummySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("esquema yummy: sin base de datos")
	}
	if err := persistence.AplicarEsquema(db, yummySchema); err != nil {
		return fmt.Errorf("esquema yummy: %w", err)
	}
	return nil
}

// yummyEvento es una fila de la bitácora, para el /yummy/status.
type yummyEvento struct {
	EventID    string
	EventType  string
	Attempt    int
	PayloadTxt string
	RecibidoAt string
}

// ---------- Cliente de la pasarela ----------

// YummyStockItem es un renglón de sincronización de stock, en el dialecto
// del API de la pasarela (POST /v1/stock/sync).
type YummyStockItem struct {
	SKU              string  `json:"sku"`
	BranchExternalID string  `json:"branchExternalId,omitempty"`
	Stock            float64 `json:"stock"`
	Operation        string  `json:"operation"`
}

// yummyPasarela es el cliente HTTP hacia el gateway. Nada del API de Yummy
// aparece aquí: las rutas que soporta son las del gateway documentadas en
// su README (stock/sync, orders, chat y la pasarela genérica /v1/yummy/{r}).
type yummyPasarela struct {
	base  string
	clave string
	cli   *http.Client
}

func nuevaPasarela(cfg *configs.Config) *yummyPasarela {
	return &yummyPasarela{
		base:  strings.TrimRight(cfg.YummyGatewayURL, "/"),
		clave: cfg.YummyGatewayKey,
		// 30s = timeout del gateway hacia Yummy por defecto; aquí se nos va
		// la franja del ERP esperando al gateway, no a Yummy, así que lo
		// duplicamos para que un gateway lento no tumbe una venta.
		cli: &http.Client{Timeout: 30 * time.Second},
	}
}

// yummyOperacion ejecuta UNA llamada al gateway y devuelve el estatus y el
// cuerpo tal como la pasarela los contestó (los errores JSON del gateway
// legible van al front tal cual: ya vienen con code/message del gateway). El error de red
// (gateway caído) se traduce a un mensaje operable.
func (y *yummyPasarela) yummyOperacion(metodo, ruta, cuerpo string) (int, []byte, error) {
	url := y.base + ruta
	var lector io.Reader
	if cuerpo != "" {
		lector = strings.NewReader(cuerpo)
	}
	req, err := http.NewRequest(metodo, url, lector)
	if err != nil {
		return 0, nil, fmt.Errorf("solicitud al gateway inválida: %w", err)
	}
	req.Header.Set("X-Gateway-Key", y.clave)
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := y.cli.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("la pasarela Yummy no respondió (%s): %w", url, err)
	}
	defer resp.Body.Close()
	grano, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("no se pudo leer la respuesta del gateway: %w", err)
	}
	return resp.StatusCode, grano, nil
}

// SincronizarStock manda un lote de items al gateway (permiso stock:write).
func (y *yummyPasarela) SincronizarStock(items []YummyStockItem) (int, []byte, error) {
	cuerpo, err := json.Marshal(map[string]interface{}{"items": items})
	if err != nil {
		return 0, nil, fmt.Errorf("no se pudo armar el lote de stock: %w", err)
	}
	return y.yummyOperacion(http.MethodPost, "/v1/stock/sync", string(cuerpo))
}

// Pedidos consulta las órdenes de Yummy por la pasarela (permiso orders:read).
func (y *yummyPasarela) Pedidos(query string) (int, []byte, error) {
	ruta := "/v1/orders"
	if query != "" {
		ruta += "?" + query
	}
	return y.yummyOperacion(http.MethodGet, ruta, "")
}

// ---------- Receptor de webhooks ----------

// yummyVerificarFirma valida la cabecera `X-Gateway-Signature:
// t=<unix>,v1=<hex>` de la entrega contra `HMAC-SHA256(secreto, t+"."+body)`,
// el formato que la pasarela firma en sus envíos (ver README de YummyProxy2).
// Con el timestamp dentro de la ventana evitamos replays colectados sin definir otra cosa: la pasarela ya reintenta, un replay a los
// seis minutos no repite el negocio del mensaje.
const yummyVentanaFrescura = 5 * time.Minute

func yummyVerificarFirma(secreto string, cuerpo []byte, header string, ahora time.Time) error {
	if strings.TrimSpace(header) == "" {
		return fmt.Errorf("falta la cabecera de firma (X-Gateway-Signature)")
	}
	partes := map[string]string{}
	for _, trozo := range strings.Split(header, ",") {
		trozo = strings.TrimSpace(trozo)
		if i := strings.Index(trozo, "="); i > 0 {
			partes[strings.TrimSpace(trozo[:i])] = strings.TrimSpace(trozo[i+1:])
		}
	}
	tsraw, ok1 := partes["t"]
	v1, ok2 := partes["v1"]
	if !ok1 || !ok2 {
		return fmt.Errorf("la firma no trae el formato t=...,v1=...")
	}
	ts, err := strconv.ParseInt(tsraw, 10, 64)
	if err != nil {
		return fmt.Errorf("el timestamp de la firma no es numérico")
	}
	if d := ahora.Sub(time.Unix(ts, 0)); d > yummyVentanaFrescura || d < -yummyVentanaFrescura {
		return fmt.Errorf("el evento está fuera de la ventana de frescura (%s)", yummyVentanaFrescura)
	}
	mac := hmac.New(sha256.New, []byte(secreto))
	mac.Write([]byte(fmt.Sprintf("%d.", ts)))
	mac.Write(cuerpo)
	if !hmac.Equal([]byte(v1), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return fmt.Errorf("la firma no corresponde con el secreto del tenant")
	}
	return nil
}

// registerYummyRoutes publica los 4 endpoints del módulo:
//
//	POST /webhooks/yummy      receptor machine-to-machine (sin sesión)
//	GET  /yummy/status        diagnóstico de la integración (con sesión)
//	POST /yummy/stock/sync    envía el stock de la empresa a la pasarela
//	GET  /yummy/orders        consulta de órdenes de Yummy por la pasarela
func (s *Server) registerYummyRoutes() {
	// --- Receptor de webhooks. Autenticado por FIRMA, no por cookie; por
	// eso authMiddleware lo deja pasar por el prefix /webhooks/ (ver
	// server.go). Con YUMMY_ENABLED=0 o sin secreto: fail-closed 403; ninguna
	// de sus verificaciones falla silenciosamente.
	s.mux.HandleFunc("POST /webhooks/yummy", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		if cfg == nil || !cfg.YummyEnabled || strings.TrimSpace(cfg.YummyCallbackSecret) == "" {
			jsonError(w, "Yummy desactivado: el superusuario debe fijar YUMMY_ENABLED=1 y YUMMY_CALLBACK_SECRET", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			jsonError(w, "Cuerpo del webhook demasiado grande o ilegible", http.StatusBadRequest)
			return
		}
		if err := yummyVerificarFirma(cfg.YummyCallbackSecret, body, r.Header.Get("X-Gateway-Signature"), time.Now()); err != nil {
			log.Printf("yummy webhook rechazado: %v", err)
			jsonError(w, "Evento rechazado: "+err.Error(), http.StatusUnauthorized)
			return
		}
		eventID := strings.TrimSpace(r.Header.Get("X-Gateway-Event-Id"))
		if eventID == "" {
			eventID = strings.TrimSpace(r.Header.Get("X-Gateway-Delivery-Id"))
		}
		if eventID == "" {
			jsonError(w, "El evento no trae X-Gateway-Event-Id: no se puede deduplicar", http.StatusBadRequest)
			return
		}
		db := s.authDB(r)
		// Idempotencia: la pasarela reintenta hasta confirmar, por lo que el
		// mismo event_id puede llegar más de una vez. Se consulta PRIMERO y
		// solo si la tabla NO existe (primera entrega de esta instalación)
		// se crea: planear el DDL antes de cada entrega obliga a todo el
		// dialecto de la sentencia en cada llamada, y.mysql —la base de
		// control real— espera MySQL, mientras las pruebas montan su tabla
		// SQLite. Con el Ensure POR DEMANDA, el DDL solo corre la primera
		// vez, que es la única vez que hace falta.
		var ya int
		if err := db.QueryRow(`SELECT COUNT(*) FROM yummy_webhook_evento WHERE event_id = ?`, eventID).Scan(&ya); err != nil {
			if !esErrorNoExisteTabla(err) {
				log.Printf("yummy webhook: consulta de idempotencia falló: %v", err)
				jsonError(w, "No se pudo registrar el evento", http.StatusInternalServerError)
				return
			}
			if errC := EnsureYummySchema(db); errC != nil {
				log.Printf("yummy webhook: %v", errC)
				jsonError(w, "No se pudo preparar la bitácora de eventos de Yummy", http.StatusInternalServerError)
				return
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM yummy_webhook_evento WHERE event_id = ?`, eventID).Scan(&ya); err != nil {
				log.Printf("yummy webhook: consulta de idempotencia persiste fallando tras crear la tabla: %v", err)
				jsonError(w, "No se pudo registrar el evento", http.StatusInternalServerError)
				return
			}
		}
		if ya > 0 {
			writeJSON(w, map[string]interface{}{"status": 1, "dedup": true})
			return
		}
		ahora := time.Now().UTC().Format("2006-01-02 15:04:05")
		intent, _ := strconv.Atoi(r.Header.Get("X-Gateway-Attempt"))
		if _, err := db.Exec(`INSERT INTO yummy_webhook_evento
			(event_id, event_type, attempt, payload, estado, recibido_at)
			VALUES (?, ?, ?, ?, 'recibido', ?)`,
			eventID, r.Header.Get("X-Gateway-Event-Type"), intent, string(body), ahora); err != nil {
			// Carrera de dos entregas simultáneas: si el índice único dice
			// que ya estaba, es dedupe y es una respuesta de éxito, no error.
			if persistence.EsErrorYaExiste(err) {
				writeJSON(w, map[string]interface{}{"status": 1, "dedup": true})
				return
			}
			log.Printf("yummy webhook: insert falló: %v", err)
			jsonError(w, "No se pudo guardar el evento", http.StatusInternalServerError)
			return
		}
		log.Printf("yummy webhook recibido: %s tipo=%s intento=%d", eventID, r.Header.Get("X-Gateway-Event-Type"), intent)
		writeJSON(w, map[string]interface{}{"status": 1})
	})

	// --- Estado de la integración.
	s.mux.HandleFunc("GET /yummy/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		res := map[string]interface{}{
			"enabled":             cfg != nil && cfg.YummyEnabled,
			"gateway_url_set":     cfg != nil && strings.TrimSpace(cfg.YummyGatewayURL) != "",
			"gateway_key_set":     cfg != nil && strings.TrimSpace(cfg.YummyGatewayKey) != "",
			"callback_secret_set": cfg != nil && strings.TrimSpace(cfg.YummyCallbackSecret) != "",
		}
		var eventos []map[string]interface{}
		if cfg != nil && cfg.YummyEnabled {
			eventos = yumUltimosEventos(s.authDB(r))
		}
		if eventos == nil {
			eventos = []map[string]interface{}{}
		}
		res["eventos"] = eventos
		writeJSON(w, res)
	})

	// --- Sincronización de stock. Igual que el resto del ERP atraviesa
	// authMiddleware: solo con sesión viva se dispara un pedido al gateway.
	s.mux.HandleFunc("POST /yummy/stock/sync", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		if cfg == nil || !cfg.YummyEnabled || strings.TrimSpace(cfg.YummyGatewayURL) == "" || strings.TrimSpace(cfg.YummyGatewayKey) == "" {
			jsonError(w, "Yummy no está configurado: falte YUMMY_ENABLED, YUMMY_GATEWAY_URL y YUMMY_GATEWAY_KEY", http.StatusBadRequest)
			return
		}
		items, err := s.yummyStockDeEmpresa(r)
		if err != nil {
			jsonError(w, "No se pudo leer el stock de la empresa: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if len(items) == 0 {
			jsonError(w, "No hay productos activos con stock que sincronizar", http.StatusBadRequest)
			return
		}
		pg := nuevaPasarela(cfg)
		est, cuerpo, err := pg.SincronizarStock(items)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		if est >= 400 {
			log.Printf("yummy stock/sync: gateway respondió %d: %s", est, string(cuerpo))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(est)
		writeJSON(w, map[string]interface{}{
			"upstream_status": est,
			"items_enviados":  len(items),
			"upstream":        json.RawMessage(cuerpoComoJSON(cuerpo)),
		})
	})

	// --- Órdenes de Yummy por la pasarela (permiso orders:read).
	s.mux.HandleFunc("GET /yummy/orders", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.config
		if cfg == nil || !cfg.YummyEnabled || strings.TrimSpace(cfg.YummyGatewayURL) == "" || strings.TrimSpace(cfg.YummyGatewayKey) == "" {
			jsonError(w, "Yummy no está configurado: faltan YUMMY_ENABLED, YUMMY_GATEWAY_URL y YUMMY_GATEWAY_KEY", http.StatusBadRequest)
			return
		}
		pg := nuevaPasarela(cfg)
		est, cuerpo, err := pg.Pedidos(r.URL.RawQuery)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(est)
		writeJSON(w, map[string]interface{}{
			"upstream_status": est,
			"upstream":        json.RawMessage(cuerpoComoJSON(cuerpo)),
		})
	})
}

// yummyStockDeEmpresa arma el lote de sincronización desde el inventario real
// de la empresa: productos activos con el stock DISPONIBLE por lotes activos
// (product_lot.quantity_available), que es como el ERP cuenta lo que hay.
// El código visible product_information.product_id es lo que viaja como sku;
// branchExternalId queda vacío a propósito: la pasarela se lo asigna cuando
// el cliente la configure (campo opcional del API).
func (s *Server) yummyStockDeEmpresa(r *http.Request) ([]YummyStockItem, error) {
	db := s.dbFor(r)
	rows, err := db.Query(`SELECT p.product_id, COALESCE(SUM(l.quantity_available), 0)
		FROM product_information p
		LEFT JOIN product_lot l ON l.product_id = p.id AND l.status = 'active'
		WHERE p.status = 1
		GROUP BY p.id, p.product_id
		ORDER BY p.product_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]YummyStockItem, 0, 32)
	for rows.Next() {
		var sku string
		var stock float64
		if err := rows.Scan(&sku, &stock); err != nil {
			return nil, err
		}
		if strings.TrimSpace(sku) == "" {
			continue // productos sin código: no se pueden referenciar en Yummy
		}
		items = append(items, YummyStockItem{SKU: sku, Stock: stock, Operation: "set"})
	}
	return items, rows.Err()
}

// esErrorNoExisteTabla dice si el error es "la tabla aún no existe" en las dos
// bases que este ERP usa: MySQL contesta con el error 1146 ("doesn't exist") y
// SQLite con "no such table". Es lo que decide si se crea la tabla de eventos
// AHORA (primera entrega de la instalación) o si el fallo es real y hay que
// devolverlo: crearla por detrás de un error que no es ese escondería un daño.
func esErrorNoExisteTabla(err error) bool {
	if err == nil {
		return false
	}
	texto := strings.ToLower(err.Error())
	return strings.Contains(texto, "no such table") || strings.Contains(texto, "doesn't exist")
}

// yumUltimosEventos lista los últimos 10 eventos recibidos para el
// /yummy/status. Si la tabla aún no existe (ninguna entrega jamás), por
// demanda se crea y se vuelve a leer; un fallo después de eso se reporta
// (parainformación) y la respuesta sigue completa: el diagnóstico del
// status no debe morir por la bitácora.
func yumUltimosEventos(db *sql.DB) []map[string]interface{} {
	listar := func() ([]map[string]interface{}, error) {
		rows, err := db.Query(`SELECT event_id, event_type, attempt, recibido_at
			FROM yummy_webhook_evento ORDER BY id DESC LIMIT 10`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		eventos := []map[string]interface{}{}
		for rows.Next() {
			var id, tipo, cuandoTxt string
			var intento int
			if err := rows.Scan(&id, &tipo, &intento, &cuandoTxt); err == nil {
				eventos = append(eventos, map[string]interface{}{
					"event_id":    id,
					"event_type":  tipo,
					"attempt":     intento,
					"recibido_at": cuandoTxt,
				})
			}
		}
		return eventos, rows.Err()
	}
	eventos, err := listar()
	if err != nil && esErrorNoExisteTabla(err) {
		if errC := EnsureYummySchema(db); errC != nil {
			log.Printf("yummy status: %v", errC)
			return []map[string]interface{}{}
		}
		eventos, err = listar()
	}
	if err != nil {
		log.Printf("yummy status: el listado de eventos falló: %v", err)
		return []map[string]interface{}{}
	}
	return eventos
}

// cuerpoComoJSON devuelve el cuerpo como JSON válido para incrustar en una
// respuesta: si la pasarela contestó algo que no es JSON (página de error de
// un proxy intermedio, por ejemplo), se guarda como string escapado en vez de
// romper la respuesta del ERP.
func cuerpoComoJSON(cuerpo []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(cuerpo, &v); err == nil {
		return cuerpo
	}
	escapado, err := json.Marshal(string(cuerpo))
	if err != nil {
		return []byte("null")
	}
	return escapado
}
