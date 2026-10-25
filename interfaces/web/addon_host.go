package web

// addon_host.go — El canal de negocio entre ERPGo y los addons.
//
// QUÉ ES ESTO Y QUÉ NO ES.
//
// El addon corre en su propio proceso: si se cae, el ERP sigue vendiendo, y el
// supervisor lo reinicia. Eso es lo que protege al binario principal. Pero un
// addon encerrado en su proceso no puede hacer nada útil: no ve un producto, no
// sabe si un lote venció, no puede avisar a un cliente.
//
// Este fichero abre ese canal sin darle la base de datos:
//
//	Query    lee recursos declarados en el manifiesto, con columnas fijas.
//	Command  pide una acción de dominio; el núcleo la valida y la audita.
//	Event    se le avisa lo que ya pasó, después del commit.
//	Guard    opina sobre una venta ANTES de facturar. Veta, no escribe.
//
// POR QUÉ EL GUARD NO COMPARTE LA TRANSACCIÓN.
//
// El stock, la factura y el asiento siguen siendo del núcleo. Si el addon
// escribiera en la misma transacción, un fallo suyo dejaría la caja a medias.
// Aquí el addon solo puede decir "no vendas esto" con un motivo, y el núcleo
// decide qué hacer con ese veto. Un addon lento se corta a los 400 ms; si su
// manifiesto pide fail_closed, la venta se bloquea con ADDON_UNAVAILABLE. Esa
// decisión es del negocio (una farmacia no quiere vender a ciegas), no del
// programador del addon.
//
// POR QUÉ EL GRANT ES EFÍMERO Y ESTÁ ATADO A UNA BASE.
//
// El token del proceso identifica al addon, pero no dice de qué empresa es la
// venta. El grant se emite en el momento del guard, dura un minuto y lleva el
// *sql.DB de ESA empresa. Así el addon puede preguntar por los lotes del
// producto que está vendiendo sin poder cambiarse de empresa: no hay ningún
// campo "tenant" que pueda manipular.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"erpgow/internal/pos"

	"github.com/orcanet/ergoaddon"
)

const (
	addonQueryPath   = "/__addon/query"
	addonCommandPath = "/__addon/command"

	// Un minuto es suficiente para una venta y corto para que un grant robado
	// no sirva de nada. No se reutiliza entre ventas.
	addonGrantTTL = time.Minute

	// El guard tiene que responder dentro del turno de caja. 400 ms es el
	// presupuesto total por addon; sumar más addons obliga a subir el tiempo
	// de la venta, y eso se nota en la fila.
	addonGuardTimeout = 400 * time.Millisecond
)

// Recursos que un addon puede leer. La lista es cerrada a propósito: cada
// recurso es una consulta escrita aquí, con sus columnas y sus filtros. Un
// addon no envía SQL ni elige columnas.
const (
	addonResProduct  = "inventory.product.read"
	addonResLot      = "inventory.lot.read"
	addonResCustomer = "customer.contact.read"
	addonResInvoice  = "invoice.summary.read"
)

// Comandos que un addon puede pedir. También cerrados y auditados.
const (
	addonCmdCRMActivity = "crm.activity.create"
)

// Temas de eventos.
const (
	addonEvtInvoiceIssued = "invoice.issued"
	addonEvtPaymentRecv   = "payment.received"
)

// addonGrant es un permiso de lectura/acción para una llamada concreta.
type addonGrant struct {
	Code    string
	DB      *sql.DB
	Caps    ergoaddon.Capabilities
	Expires time.Time
}

var addonGrantStore = struct {
	sync.Mutex
	byToken map[string]addonGrant
}{byToken: map[string]addonGrant{}}

// issueAddonGrant crea el permiso efímero que viaja al addon.
func issueAddonGrant(code string, db *sql.DB, caps ergoaddon.Capabilities) string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	token := hex.EncodeToString(raw)
	addonGrantStore.Lock()
	defer addonGrantStore.Unlock()
	now := time.Now()
	// Poda perezosa: no hay goroutine de limpieza y el mapa no crece sin fin.
	for k, g := range addonGrantStore.byToken {
		if now.After(g.Expires) {
			delete(addonGrantStore.byToken, k)
		}
	}
	addonGrantStore.byToken[token] = addonGrant{
		Code:    code,
		DB:      db,
		Caps:    caps,
		Expires: now.Add(addonGrantTTL),
	}
	return token
}

// addonGrantFromRequest resuelve el grant de una llamada del addon.
//
// Solo se acepta desde loopback: el addon corre en la misma máquina, y una
// petición de la red no tiene por qué llegar a este canal aunque adivine la
// ruta y el token.
func addonGrantFromRequest(r *http.Request) (addonGrant, bool) {
	if !addonRequestIsLoopback(r) {
		return addonGrant{}, false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer"))
	if token == "" || token == auth {
		return addonGrant{}, false
	}
	addonGrantStore.Lock()
	defer addonGrantStore.Unlock()
	g, ok := addonGrantStore.byToken[token]
	if !ok || time.Now().After(g.Expires) {
		delete(addonGrantStore.byToken, token)
		return addonGrant{}, false
	}
	return g, true
}

func addonRequestIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// addonHostURL es la dirección que el addon usa para volver a llamar. Es
// loopback siempre: el addon no necesita la IP pública del ERP.
func (s *Server) addonHostURL() string {
	port := "8080"
	if s.config != nil && strings.TrimSpace(s.config.ServerPort) != "" {
		port = strings.TrimSpace(s.config.ServerPort)
	}
	return "http://127.0.0.1:" + port
}

// registerAddonHostRoutes monta el canal que usan los addons para consultar y
// actuar. La autenticación es el grant, no la cookie de sesión: un addon no
// tiene sesión de usuario y no debe tenerla.
func (s *Server) registerAddonHostRoutes() {
	s.mux.HandleFunc("POST "+addonQueryPath, s.addonQuery)
	s.mux.HandleFunc("POST "+addonCommandPath, s.addonCommand)
}

// writeJSONStatus escribe una respuesta JSON con un código HTTP distinto de
// 200. writeJSON siempre responde 200; un bloqueo de venta no es un éxito.
func writeJSONStatus(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// installAddonSaleGuard conecta el camino gRPC del POS con el evaluador de
// addons. El paquete pos no puede importar web (ciclo), así que el enlace se
// hace aquí, una sola vez al construir el servidor.
func (s *Server) installAddonSaleGuard() {
	pos.SaleGuard = func(ctx context.Context, db *sql.DB, terminalID string, items []pos.GuardItem) []pos.GuardBlock {
		guards := make([]ergoaddon.GuardItem, 0, len(items))
		for _, it := range items {
			guards = append(guards, ergoaddon.GuardItem{
				ProductID: it.ProductID,
				Barcode:   it.Barcode,
				Quantity:  it.Quantity,
			})
		}
		blocks := s.addonSaleBlocks(ctx, db, "", terminalID, guards)
		out := make([]pos.GuardBlock, 0, len(blocks))
		for _, b := range blocks {
			out = append(out, pos.GuardBlock{Code: b.Code, ProductID: b.ProductID, Message: b.Message})
		}
		return out
	}
}

// --- Guard de venta ---------------------------------------------------------

// addonSaleBlocks consulta a los addons que declararon sale_guard y devuelve
// todos los motivos para no vender. Si no hay addons activos con guard, no
// hace ninguna llamada de red: la venta normal no paga este canal.
func (s *Server) addonSaleBlocks(ctx context.Context, db *sql.DB, tenant, terminalID string, items []ergoaddon.GuardItem) []ergoaddon.SaleBlock {
	if db == nil || len(items) == 0 {
		return nil
	}
	guards := s.activeAddonGuards()
	if len(guards) == 0 {
		return nil
	}

	var blocks []ergoaddon.SaleBlock
	for _, guard := range guards {
		addonMgr.mu.RLock()
		proc := addonMgr.process[guard.code]
		addonMgr.mu.RUnlock()

		if proc == nil {
			if guard.caps.SaleGuard != nil && guard.caps.SaleGuard.FailClosed {
				blocks = append(blocks, ergoaddon.SaleBlock{
					Code:    "ADDON_UNAVAILABLE",
					Message: "El addon " + guard.code + " no está disponible y exige bloquear la venta.",
				})
			}
			continue
		}

		grant := issueAddonGrant(guard.code, db, guard.caps)
		resp, err := callAddonSaleGuard(ctx, proc, grant, s.addonHostURL(), tenant, terminalID, items)
		if err != nil {
			if guard.caps.SaleGuard != nil && guard.caps.SaleGuard.FailClosed {
				blocks = append(blocks, ergoaddon.SaleBlock{
					Code:    "ADDON_UNAVAILABLE",
					Message: "No se pudo validar la venta con " + guard.code + ": " + err.Error(),
				})
			}
			continue
		}
		if resp == nil || resp.Allow {
			continue
		}
		if len(resp.Blocks) == 0 {
			blocks = append(blocks, ergoaddon.SaleBlock{
				Code:    "SALE_BLOCKED",
				Message: "El addon " + guard.code + " bloqueó la venta sin indicar el motivo.",
			})
			continue
		}
		blocks = append(blocks, resp.Blocks...)
	}
	return dedupeSaleBlocks(blocks)
}

type activeAddonGuard struct {
	code string
	caps ergoaddon.Capabilities
}

// activeAddonGuards lee los addons activos desde la base de control, que es
// donde vive el catálogo de addons, no la base de la empresa.
func (s *Server) activeAddonGuards() []activeAddonGuard {
	db := s.addonCatalogDB(nil)
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT id, manifest_json FROM addons WHERE is_active = 1`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []activeAddonGuard
	for rows.Next() {
		var code, manifestJSON string
		if rows.Scan(&code, &manifestJSON) != nil {
			continue
		}
		var manifest ergoaddon.Manifest
		if json.Unmarshal([]byte(manifestJSON), &manifest) != nil {
			continue
		}
		// El manifiesto DICE lo que el addon pide; la tabla de aprobaciones dice
		// lo que la empresa concede. Un guard que nadie aprobó no se consulta:
		// vetar una venta es de las cosas que más poder dan sobre el negocio.
		caps := addonEffectiveCapabilitiesFor(db, code, &manifest)
		if caps == nil || caps.SaleGuard == nil {
			continue
		}
		out = append(out, activeAddonGuard{code: code, caps: *caps})
	}
	return out
}

// addonCatalogDB devuelve la base donde están las filas de addons. En una
// instalación normal es la base de control; en pruebas de una sola base, la
// propia base del tenant. Se intenta control primero para no depender de que la
// empresa tenga la tabla.
func (s *Server) addonCatalogDB(fallback *sql.DB) *sql.DB {
	// controlDB es el handle que fijó NewServer; usarlo evita abrir una segunda
	// conexión y funciona igual en producción y en las pruebas de una sola base.
	for _, db := range []*sql.DB{s.controlDB, fallback} {
		if db == nil {
			continue
		}
		var n int
		if db.QueryRow(`SELECT COUNT(*) FROM addons`).Scan(&n) == nil {
			return db
		}
	}
	if ctl, err := s.controlStore(); err == nil && ctl != nil {
		var n int
		if ctl.QueryRow(`SELECT COUNT(*) FROM addons`).Scan(&n) == nil {
			return ctl
		}
	}
	return nil
}

func callAddonSaleGuard(ctx context.Context, proc *addonProcess, grant, hostURL, tenant, terminalID string, items []ergoaddon.GuardItem) (*ergoaddon.SaleGuardResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, addonGuardTimeout)
	defer cancel()

	body, err := json.Marshal(ergoaddon.SaleGuardRequest{
		Tenant:     tenant,
		TerminalID: terminalID,
		HostURL:    hostURL,
		Items:      items,
	})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", proc.HTTPPort, ergoaddon.HookSaleGuardPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ergoaddon.AddonTokenHeader, proc.Token)
	req.Header.Set("Authorization", "Bearer "+grant)

	resp, err := (&http.Client{Timeout: addonGuardTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hook respondió HTTP %d", resp.StatusCode)
	}
	var out ergoaddon.SaleGuardResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("respuesta inválida: %w", err)
	}
	return &out, nil
}

func dedupeSaleBlocks(in []ergoaddon.SaleBlock) []ergoaddon.SaleBlock {
	seen := map[string]bool{}
	out := in[:0]
	for _, b := range in {
		key := b.Code + "|" + b.ProductID + "|" + b.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, b)
	}
	return out
}

// saleBlockedResponse es el cuerpo que recibe el POS. Es el mismo en HTTP y en
// gRPC (donde va como texto del error) para que el cajero lea lo mismo por las
// dos vías.
type saleBlockedResponse struct {
	Code   string                `json:"code"`
	Msg    string                `json:"msg"`
	Blocks []ergoaddon.SaleBlock `json:"blocks"`
}

func writeSaleBlocked(w http.ResponseWriter, blocks []ergoaddon.SaleBlock) {
	msg := "La venta no se puede completar"
	if len(blocks) > 0 {
		msg = blocks[0].Message
	}
	writeJSONStatus(w, http.StatusConflict, saleBlockedResponse{
		Code:   "SALE_BLOCKED",
		Msg:    msg,
		Blocks: blocks,
	})
}

// formatSaleBlocks convierte los bloqueos en un texto para el error de gRPC,
// que no transporta JSON cómodamente en el mensaje.
func formatSaleBlocks(blocks []ergoaddon.SaleBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.ProductID != "" {
			parts = append(parts, b.Code+": "+b.ProductID+": "+b.Message)
		} else {
			parts = append(parts, b.Code+": "+b.Message)
		}
	}
	return strings.Join(parts, " | ")
}

// --- Query ------------------------------------------------------------------

func (s *Server) addonQuery(w http.ResponseWriter, r *http.Request) {
	grant, ok := addonGrantFromRequest(r)
	if !ok {
		jsonError(w, "grant inválido o vencido", http.StatusForbidden)
		return
	}
	var req ergoaddon.QueryRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		jsonError(w, "consulta inválida", http.StatusBadRequest)
		return
	}
	if !addonCapsAllow(grant.Caps.Resources, req.Resource) {
		jsonError(w, "el addon no declaró el recurso "+req.Resource, http.StatusForbidden)
		return
	}
	rows, err := addonQueryResource(grant.DB, req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, ergoaddon.QueryResponse{Rows: rows})
}

func addonCapsAllow(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// addonQueryResource ejecuta la consulta del recurso. Cada recurso tiene sus
// columnas y sus filtros permitidos; cualquier otro filtro se ignora en vez de
// convertirse en SQL.
func addonQueryResource(db *sql.DB, req ergoaddon.QueryRequest) ([]map[string]interface{}, error) {
	if db == nil {
		return nil, fmt.Errorf("sin base de datos para la consulta")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	filter := func(key string) string { return strings.TrimSpace(req.Filter[key]) }

	var (
		query string
		args  []interface{}
	)
	switch req.Resource {
	case addonResProduct:
		query = `SELECT product_id, product_name, COALESCE(barcode,''), COALESCE(status,0)
		         FROM product_information WHERE 1=1`
		if v := filter("product_id"); v != "" {
			query += " AND product_id = ?"
			args = append(args, v)
		}
		if v := filter("barcode"); v != "" {
			query += " AND barcode = ?"
			args = append(args, v)
		}
		query += " ORDER BY product_name LIMIT ?"
		args = append(args, limit)

	case addonResLot:
		// product_lot.product_id es el ID numérico de product_information, no el
		// código que usa el POS. Si se filtrara por `l.product_id`, un addon que
		// pide "ACETAMOL-500" nunca encontraría el lote: la consulta devolvería
		// cero filas y un vencido pasaría como si no tuviera lotes. Se une por el
		// ID y se filtra por el código del producto.
		query = `SELECT p.product_id AS product_id, p.product_name AS product_name,
		                l.lot_code, COALESCE(CAST(l.expiry_date AS CHAR), '') AS expiry_date,
		                l.quantity_available, l.status
		         FROM product_lot l
		         JOIN product_information p ON p.id = l.product_id
		         WHERE l.quantity_available > 0`
		if v := filter("product_id"); v != "" {
			query += " AND p.product_id = ?"
			args = append(args, v)
		}
		if v := filter("barcode"); v != "" {
			query += " AND p.barcode = ?"
			args = append(args, v)
		}
		query += " ORDER BY l.expiry_date IS NULL, l.expiry_date ASC LIMIT ?"
		args = append(args, limit)

	case addonResCustomer:
		query = `SELECT customer_id, COALESCE(customer_name,''),
		                COALESCE(customer_mobile,''), COALESCE(customer_email,'')
		         FROM customer_information WHERE 1=1`
		if v := filter("customer_id"); v != "" {
			query += " AND customer_id = ?"
			args = append(args, v)
		}
		if v := filter("mobile"); v != "" {
			query += " AND customer_mobile = ?"
			args = append(args, v)
		}
		query += " ORDER BY customer_name LIMIT ?"
		args = append(args, limit)

	case addonResInvoice:
		query = `SELECT invoice_id, customer_id, total_amount, due_amount, status
		         FROM invoice WHERE 1=1`
		if v := filter("invoice_id"); v != "" {
			query += " AND invoice_id = ?"
			args = append(args, v)
		}
		if v := filter("customer_id"); v != "" {
			query += " AND customer_id = ?"
			args = append(args, v)
		}
		query += " ORDER BY invoice_id DESC LIMIT ?"
		args = append(args, limit)

	default:
		return nil, fmt.Errorf("recurso no soportado: %s", req.Resource)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("consulta del recurso %s: %w", req.Resource, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, _ := rows.ColumnTypes()
	out := []map[string]interface{}{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		pointers := make([]interface{}, len(cols))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			var ct *sql.ColumnType
			if i < len(types) {
				ct = types[i]
			}
			row[col] = addonJSONValue(values[i], ct)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// addonJSONValue normaliza lo que devuelve el driver. Un []byte se serializa
// como base64 si se deja tal cual, y una fecha como time.Time rompe la
// comparación que hace el addon; ambos se convierten a texto.
func addonJSONValue(v interface{}, ct *sql.ColumnType) interface{} {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		return t.Format("2006-01-02")
	case []byte:
		// El driver de MySQL devuelve TODO como []byte si no se escanea a un
		// tipo. Si se serializara tal cual, una cantidad de lote llegaría como
		// texto y un addon que compara `qty > 0` vería cero: el producto con
		// stock válido se bloquearía igual que uno vencido. Se usa el tipo de
		// columna para decidir, no una heurística sobre el contenido.
		if ct != nil {
			switch strings.ToUpper(ct.DatabaseTypeName()) {
			case "INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT":
				if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
					return n
				}
			case "DOUBLE", "FLOAT", "REAL", "DECIMAL", "NEWDECIMAL", "NUMERIC":
				if f, err := strconv.ParseFloat(string(t), 64); err == nil {
					return f
				}
			}
		}
		return string(t)
	default:
		return t
	}
}

// --- Command ----------------------------------------------------------------

func (s *Server) addonCommand(w http.ResponseWriter, r *http.Request) {
	grant, ok := addonGrantFromRequest(r)
	if !ok {
		jsonError(w, "grant inválido o vencido", http.StatusForbidden)
		return
	}
	var req ergoaddon.CommandRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		jsonError(w, "comando inválido", http.StatusBadRequest)
		return
	}
	if !addonCapsAllow(grant.Caps.Commands, req.Name) {
		jsonError(w, "el addon no declaró el comando "+req.Name, http.StatusForbidden)
		return
	}
	result, err := addonRunCommand(grant.DB, req)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, ergoaddon.CommandResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, ergoaddon.CommandResponse{OK: true, Result: result})
}

// addonRunCommand es la lista cerrada de acciones que un addon puede pedir.
// Cada una es una operación del ERP, no SQL libre; si mañana hace falta otra,
// se escribe aquí con su validación y su auditoría.
func addonRunCommand(db *sql.DB, req ergoaddon.CommandRequest) (map[string]interface{}, error) {
	switch req.Name {
	case addonCmdCRMActivity:
		contactID, _ := strconv.ParseInt(fmt.Sprint(req.Payload["contact_id"]), 10, 64)
		subject := strings.TrimSpace(fmt.Sprint(req.Payload["subject"]))
		body := strings.TrimSpace(fmt.Sprint(req.Payload["body"]))
		activityType := strings.TrimSpace(fmt.Sprint(req.Payload["activity_type"]))
		switch activityType {
		case "llamada", "email", "reunion", "visita", "tarea", "nota", "whatsapp":
		default:
			activityType = "nota"
		}
		if contactID <= 0 || subject == "" {
			return nil, fmt.Errorf("contact_id y subject son obligatorios")
		}
		res, err := db.Exec(`INSERT INTO crm_activity
			(contact_id, opportunity_id, activity_type, subject, body, done, created_by, created_at)
			VALUES (?, 0, ?, ?, ?, 1, 0, ?)`,
			contactID, activityType, subject, body, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			return nil, fmt.Errorf("no se pudo registrar la actividad: %w", err)
		}
		id, _ := res.LastInsertId()
		return map[string]interface{}{"activity_id": id}, nil
	}
	return nil, fmt.Errorf("comando no soportado: %s", req.Name)
}

// --- Eventos ----------------------------------------------------------------

// dispatchAddonEvent avisa a los addons suscritos lo que ya ocurrió. Se llama
// después del commit y en segundo plano: un addon caído no puede retrasar ni
// tumbar una venta ya confirmada.
func (s *Server) dispatchAddonEvent(db *sql.DB, tenant, topic string, payload map[string]interface{}) {
	if db == nil {
		return
	}
	addonMgr.mu.RLock()
	defer addonMgr.mu.RUnlock()

	catalog := s.addonCatalogDB(db)
	if catalog == nil {
		return
	}
	rows, err := catalog.Query(`SELECT id, manifest_json FROM addons WHERE is_active = 1`)
	if err != nil {
		return
	}
	defer rows.Close()

	// host_url se fija UNA vez, antes de lanzar las goroutines: mutar el mapa
	// compartido desde cada envío sería una carrera en cuanto haya dos addons
	// suscritos al mismo evento.
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["host_url"] = s.addonHostURL()
	event := ergoaddon.Event{
		ID:         newAddonEventID(),
		Topic:      topic,
		Tenant:     tenant,
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}
	for rows.Next() {
		var code, manifestJSON string
		if rows.Scan(&code, &manifestJSON) != nil {
			continue
		}
		var manifest ergoaddon.Manifest
		if json.Unmarshal([]byte(manifestJSON), &manifest) != nil || manifest.Capabilities == nil {
			continue
		}
		caps := addonEffectiveCapabilitiesFor(catalog, code, &manifest)
		if caps == nil || !addonCapsAllow(caps.Events, topic) {
			continue
		}
		proc := addonMgr.process[code]
		if proc == nil {
			continue
		}
		// El grant lleva las capabilities EFECTIVAS: si el administrador revocó
		// un recurso, el addon no puede seguir consultándolo con un evento.
		grant := issueAddonGrant(code, db, *caps)
		go postAddonEvent(proc, grant, event)
	}
}

func postAddonEvent(proc *addonProcess, grant string, event ergoaddon.Event) {
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d%s", proc.HTTPPort, ergoaddon.HookEventsPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ergoaddon.AddonTokenHeader, proc.Token)
	req.Header.Set("Authorization", "Bearer "+grant)
	req.Header.Set(ergoaddon.EventIDHeader, event.ID)
	req.Header.Set(ergoaddon.EventSignatureHeader, signAddonBody(proc.Token, body))
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func signAddonBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newAddonEventID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(raw)
}
