package web

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------- Pista de Auditoría (SENIAT) ----------
// Diseño: cadena de hash SHA-256 encadenada estilo blockchain.
// Cada registro incluye record_hash = SHA256(prev_hash + id + user + action +
// module + table + record_id + old + new + ip + timestamp).
// Alterar o eliminar cualquier registro rompe TODA la cadena posterior:
// la vista de auditoría recalcula la cadena y marca el primer registro roto.
// La tabla es append-only en la práctica: la app nunca hace UPDATE/DELETE.
//
// La cadena se SEGMENTA en bloques por período (mes) en vez de ser una sola
// cadena infinita. Cada bloque sigue enlazado con el anterior, así que la
// cadena lógica continúa siendo continua e inalterable de extremo a extremo,
// pero la verificación ya no necesita recorrer el histórico: para auditar un
// mes se recorre ese mes. Ver doc/bitacora-sqlite-mysql.md §7.
//
//   audit_log            → verdad criptográfica (append-only, nunca se altera)
//   audit_chain_blocks   → índice + SELLO de cada bloque ya cerrado
//
// El sello (block_hash) se escribe UNA vez por período y nunca se sobrescribe:
// recalcularlo y obtener otro valor es, en sí mismo, la prueba de manipulación.
// Por eso el sello no sustituye a la cadena: ambos se verifican.

// auditChainMu serializa la obtención del último hash para mantener
// la cadena consistente bajo concurrencia.
var auditChainMu sync.Mutex

// lastAuditHash returns the record_hash of the last audit row ("genesis" if none).
func (s *Server) lastAuditHash(r *http.Request) string {
	var h sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT record_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&h)
	if !h.Valid || h.String == "" {
		return "GENESIS"
	}
	return h.String
}

// ---------- Bloques por período ----------

// auditPeriod es la clave de bloque de una auditoría: "YYYY-MM".
func auditPeriod(t time.Time) string { return t.Format("2006-01") }

// auditPeriodBounds devuelve el rango [from, to] de created_at del período.
func auditPeriodBounds(period string) (from, to string) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		t = time.Now()
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	return start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05")
}

// computeBlockHash sella un bloque: cubre su cadena Y el sello del anterior,
// así que borrar un mes entero también rompe el enlace.
func computeBlockHash(prevBlock, startHash, endHash string, firstID, lastID int64, count int) string {
	payload := fmt.Sprintf("BLOCK|%s|%s|%s|%d|%d|%d", prevBlock, startHash, endHash, firstID, lastID, count)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// lastAuditHashBefore devuelve el hash del último registro anterior a `from`:
// es el ancla con la que arranca la cadena de un período, sin necesidad de
// recorrer los registros previos. Usa idx_audit_created.
func (s *Server) lastAuditHashBefore(r *http.Request, from string) string {
	var h sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT record_hash FROM audit_log WHERE created_at < ? ORDER BY id DESC LIMIT 1`, from).Scan(&h)
	if !h.Valid || h.String == "" {
		return "GENESIS"
	}
	return h.String
}

// prevAuditBlockHash devuelve el sello del bloque inmediatamente anterior.
// El orden lexicográfico de "YYYY-MM" coincide con el cronológico.
func (s *Server) prevAuditBlockHash(r *http.Request, period string) string {
	var h sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT block_hash FROM audit_chain_blocks WHERE period < ? ORDER BY period DESC LIMIT 1`, period).Scan(&h)
	if !h.Valid || h.String == "" {
		return "GENESIS"
	}
	return h.String
}

// auditChainResult es el resultado de recorrer y validar un tramo de cadena.
type auditChainResult struct {
	FirstID   int64
	LastID    int64
	Count     int
	StartHash string
	EndHash   string
	BrokenAt  int64
	Reason    string
}

// auditChainRange recorre la cadena validando enlace y contenido. from/to
// vacíos = toda la tabla (uso explícito y opcional); con ambos informados el
// recorrido se limita a ese rango, que es lo que hace escalable la auditoría.
// Recorre con rows.Next() (streaming): no carga el tramo en memoria.
func (s *Server) auditChainRange(r *http.Request, from, to string) (auditChainResult, error) {
	var res auditChainResult

	// El ancla del rango es el hash final de lo anterior: el último registro
	// previo al período (o GENESIS si el período abre la cadena). Es lo que
	// hace que un bloque dependa del anterior sin recorrer el histórico.
	// Se resuelve ANTES de abrir el cursor: anidarla dejaría dos cursores
	// abiertos a la vez sobre el mismo pool.
	expectedPrev := "GENESIS"
	if from != "" {
		expectedPrev = s.lastAuditHashBefore(r, from)
	}

	q := `SELECT id, user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at, prev_hash, record_hash FROM audit_log`
	args := []interface{}{}
	if from != "" && to != "" {
		q += ` WHERE created_at >= ? AND created_at <= ?`
		args = append(args, from, to)
	}
	q += ` ORDER BY id ASC`

	rows, err := s.dbFor(r).Query(q, args...)
	if err != nil {
		return res, err
	}
	defer rows.Close()

	var id int64
	var uid, action, module, table, recordID, oldVal, newVal, ip, createdAt, prevH, hash sql.NullString
	for rows.Next() {
		_ = rows.Scan(&id, &uid, &action, &module, &table, &recordID, &oldVal, &newVal, &ip, &createdAt, &prevH, &hash)
		if prevH.String != expectedPrev {
			res.FirstID = id
			res.StartHash = prevH.String
			res.BrokenAt = id
			if res.Count == 0 {
				res.Reason = fmt.Sprintf("el bloque no arranca en el hash final del bloque anterior (registro %d): posible eliminación o alteración del período previo", id)
			} else {
				res.Reason = fmt.Sprintf("enlace roto en registro %d: prev_hash no coincide con el hash del registro anterior (posible eliminación o alteración)", id)
			}
			return res, nil
		}
		if res.Count == 0 {
			res.FirstID = id
			res.StartHash = prevH.String
		}
		// created_at escanea como time.Time (parseTime=true) y stringifica como
		// RFC3339; se normaliza al formato usado en el INSERT.
		ts := createdAt.String
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			ts = t.Format("2006-01-02 15:04:05")
		}
		// Verificación en dos formatos (S-10): la fila pudo escribirse antes
		// de la clave (sha pelado) o con ella ("h2:" HMAC simple). Basta con
		// que coincida UNA de las dos recalculaciones; una fila que no
		// coincide con ninguna está alterada, venga de la época que venga.
		clave := s.claveCadenaAuditoria()
		want := computeAuditHash(expectedPrev, id, uid.String, action.String, module.String, table.String, recordID.String, oldVal.String, newVal.String, ip.String, ts, clave)
		wantVieja := want
		if clave != "" {
			wantVieja = computeAuditHash(expectedPrev, id, uid.String, action.String, module.String, table.String, recordID.String, oldVal.String, newVal.String, ip.String, ts, "")
		}
		if hash.String != want && hash.String != wantVieja {
			res.BrokenAt = id
			res.Reason = fmt.Sprintf("contenido alterado en registro %d: el hash recalculado no coincide con el almacenado", id)
			return res, nil
		}
		expectedPrev = hash.String
		res.LastID = id
		res.Count++
	}
	res.EndHash = expectedPrev
	return res, rows.Err()
}

// sealClosedAuditBlocks sella los períodos ya cerrados. Solo se ejecuta cuando
// cambia el período en curso (una vez al mes), no en cada auditoría: el costo
// por escritura sigue siendo O(1). Sin estado en memoria se auto-repara tras
// un reinicio, porque vuelve a barrer los períodos cerrados sin sello.
// Errores aquí NUNCA deben tumbar una auditoría: son best-effort.
func (s *Server) sealClosedAuditBlocks(r *http.Request, now time.Time) {
	period := auditPeriod(now)
	if s.auditLastPeriod == period {
		return
	}
	s.auditLastPeriod = period

	from, _ := auditPeriodBounds(period)
	rows, err := s.dbFor(r).Query(`SELECT DISTINCT substr(created_at,1,7) FROM audit_log WHERE created_at < ? ORDER BY 1`, from)
	if err != nil {
		return
	}
	var closed []string
	for rows.Next() {
		var p sql.NullString
		if err := rows.Scan(&p); err == nil && p.Valid && p.String != "" && p.String != period {
			closed = append(closed, p.String)
		}
	}
	rows.Close()
	for _, p := range closed {
		_ = s.sealAuditPeriod(r, p)
	}
}

// sealAuditPeriod escribe el sello de un período cerrado, una sola vez.
func (s *Server) sealAuditPeriod(r *http.Request, period string) error {
	var exists int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM audit_chain_blocks WHERE period = ?`, period).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		// Ya sellado. El sello original es la evidencia: nunca se sobrescribe,
		// ni siquiera si el recálculo difiere (esa diferencia es el hallazgo).
		return nil
	}
	from, to := auditPeriodBounds(period)
	res, err := s.auditChainRange(r, from, to)
	if err != nil || res.Count == 0 || res.BrokenAt > 0 {
		// No se sella una cadena rota ni un período vacío: queda pendiente y la
		// verificación lo reporta en vez de dejarlo pasar por bueno.
		return err
	}
	prevBlock := s.prevAuditBlockHash(r, period)
	blockHash := computeBlockHash(prevBlock, res.StartHash, res.EndHash, res.FirstID, res.LastID, res.Count)
	_, err = s.dbFor(r).Exec(`INSERT INTO audit_chain_blocks
		(period, prev_block_hash, start_hash, end_hash, first_id, last_id, record_count, block_hash, sealed_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		period, prevBlock, res.StartHash, res.EndHash, res.FirstID, res.LastID, res.Count, blockHash,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

// computeAuditHash builds the chained hash for one audit entry.
//
// S-10 (auditoría 2026-11-22): la versión anterior era SHA-256 público —
// quien pudiera escribir en la base podía recalcular TODA la cadena y
// reescribir la historia sin dejar rastro, que es justo lo que la cadena
// promete impedir ante el SENIAT—. Con clave maestra (MasterKey del .env,
// que no vive en la base) la huella se marca "h2:" y usa HMAC simple
// sha256(payload|clave): sin la clave no hay forma de fabricar huellas
// válidas. Sin clave (instalaciones antiguas sin JWT_SECRET configurado)
// se mantiene el formato antiguo para no romper cadenas existentes; el
// verificador acepta ambos formatos y la traza de verificación avisa de
// qué versión lleva cada fila.
func computeAuditHash(prev string, id int64, userID, action, module, table, recordID, oldV, newV, ip, ts, clave string) string {
	payload := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		prev, id, userID, action, module, table, recordID, oldV, newV, ip, ts)
	if strings.TrimSpace(clave) == "" {
		sum := sha256.Sum256([]byte(payload))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256([]byte(payload + "|" + clave))
	return "h2:" + hex.EncodeToString(sum[:])
}

// claveCadenaAuditoria devuelve la MasterKey con la que se sellan las huellas
// nuevas. Config nil (pruebas) o clave vacía = formato clásico.
func (s *Server) claveCadenaAuditoria() string {
	if s == nil || s.config == nil {
		return ""
	}
	return strings.TrimSpace(s.config.MasterKey)
}

// audit appends a tamper-evident record to the audit chain.
// userID/ip are resolved from the request when available.
func (s *Server) audit(r *http.Request, action, module, table, recordID string, oldVal, newVal interface{}) {
	userID := "system"
	ip := "127.0.0.1"
	if r != nil {
		if sess := s.sessionFromRequest(r); sess != nil {
			userID = sess.Username
		}
		ip = clientIP(r)
	}
	s.appendAudit(r, userID, ip, action, module, table, recordID, oldVal, newVal)
}

// appendAudit writes the audit row with its chained hash.
func (s *Server) appendAudit(r *http.Request, userID, ip, action, module, table, recordID string, oldVal, newVal interface{}) {
	s.appendAuditAt(r, userID, ip, action, module, table, recordID, oldVal, newVal, time.Now())
}

// appendAuditAt es appendAudit con instante explícito. Lo necesitan las
// auditorías programadas o atrasadas, cuyo created_at no es "ahora", y los
// tests, que deben poder colocar registros en un período concreto.
//
// S-10 (auditoría 2026-11-22): todo el registro —INSERT, huella y enlace—
// vive en UNA transacción. La versión anterior insertaba con record_hash ''
// y lo sellaba con un UPDATE suelto: un corte de luz entre los dos dejaba
// una fila con huella vacía que el verificador marcaba como «contenido
// alterado» PARA SIEMPRE, acusando de manipulación al cliente honesto.
// Ahora: si el sellado falla, el registro entero se deshace (no queda fila
// coja) y se avisa; si el commit falla, tampoco. La lectura del hash
// anterior ocurre dentro de la misma transacción, bajo auditChainMu, para
// que el enlace no pueda calcularse contra un mundo que ya cambió.
func (s *Server) appendAuditAt(r *http.Request, userID, ip, action, module, table, recordID string, oldVal, newVal interface{}, at time.Time) {
	oldJSON := marshalJSON(oldVal)
	newJSON := marshalJSON(newVal)
	ts := at.Format("2006-01-02 15:04:05")

	auditChainMu.Lock()
	defer auditChainMu.Unlock()

	db := s.dbFor(r)
	if db == nil {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		log.Printf("auditoría: no se pudo abrir la transacción del registro (%s/%s): %v", module, action, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var h sql.NullString
	_ = tx.QueryRow(`SELECT record_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&h)
	prev := "GENESIS"
	if h.Valid && h.String != "" {
		prev = h.String
	}

	res, err := tx.Exec(`INSERT INTO audit_log
		(user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at, prev_hash, record_hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		userID, action, module, table, recordID, oldJSON, newJSON, ip, ts, prev, "")
	if err != nil {
		return
	}
	id, _ := res.LastInsertId()
	hash := computeAuditHash(prev, id, userID, action, module, table, recordID, oldJSON, newJSON, ip, ts, s.claveCadenaAuditoria())
	if _, err := tx.Exec(`UPDATE audit_log SET record_hash=? WHERE id=? AND record_hash=''`, hash, id); err != nil {
		// El rollback deshace el INSERT: nunca queda una fila sin huella.
		log.Printf("auditoría: no se pudo escribir la huella del registro %d; el registro se deshizo: %v", id, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("auditoría: no se pudo confirmar el registro %s/%s: %v", module, action, err)
		return
	}

	// Sella los períodos cerrados. Barato: solo corre al cambiar de mes.
	s.sealClosedAuditBlocks(r, at)
}

// clientIP devuelve la IP que se puede AUDITAR (R8, auditoría 2026-11-22).
//
// Antes bastaba con enviar X-Forwarded-For para ser quien hiciera falta:
// con eso se saltaba el límite de fuerza bruta del POS (la clave del
// supervisor se podía probar cambiando una cabecera) y la auditoría
// quedaba con un ip_address inventado. La cabecera solo se cree cuando la
// conexión TCP viene de un proxy declarado por el operador
// (PROXY_TRUSTED_IPS); en cualquier otro caso se usa el origen real.
func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil && esProxyConfiado(ip) {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			if primera := strings.TrimSpace(strings.Split(xf, ",")[0]); primera != "" {
				return primera
			}
		}
	}
	return strings.TrimSpace(host)
}

// sufijosSoloLectura son rutas que se sirven por POST pero NO cambian nada: los
// datos de los listados (DataTables serverSide), la comprobación de duplicados
// de una compra, los escáneres de CxC/CxP, la verificación de tasas y la
// comprobación de integridad de la base.
//
// POR QUÉ SE DEJAN DE AUDITAR. auditMiddleware registraba TODA petición
// POST/PUT/DELETE, y cada registro son tres sentencias (SELECT del hash
// anterior, INSERT, UPDATE del hash nuevo) bajo auditChainMu, el candado que
// serializa las escrituras de auditoría. Como los listados piden sus datos por
// POST, cada tecla tecleada en el buscador de una lista escribía una fila de
// ruido en audit_log y esperaba por ese candado: la pista se llenaba de
// «create» de búsquedas y las escrituras de verdad contendían con las lecturas.
//
// La cadena NO pierde nada: sigue encadenando exactamente igual cada
// MUTACIÓN, que es lo que la ley de auditoría pide. Ver la prueba
// TestAuditoriaCadenaSigueEncadenadaTrasDejarDeAuditarLecturas.
//
// Regla para añadir aquí: sólo rutas que sólo lean. Si un endpoint acaba
// escribiendo, se le cambia el nombre o se le quita de esta lista — ese es
// precisamente el tipo de fallo que esta lista debe hacer visible.
var sufijosSoloLectura = []string{
	"/data",      // datos de listados y reportes (DataTables serverSide)
	"/dup-check", // pregunta si la compra ya existe, sin escribir
	"/escanear",  // escaneo de CxC y CxP
	"/verify",    // verificación de tasas
	"/check",     // comprobación de integridad de la base
}

func esLecturaPorPost(path string) bool {
	for _, suf := range sufijosSoloLectura {
		if strings.HasSuffix(path, suf) {
			return true
		}
	}
	return false
}

// auditMiddleware logs every state-changing request (POST/PUT/DELETE)
// automatically, giving full coverage even for handlers without explicit calls.
func (s *Server) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			path := r.URL.Path
			// skip login noise, the audit endpoints themselves, and pure reads
			if path != "/login" && !strings.HasPrefix(path, "/reports/audit-log") && !esLecturaPorPost(path) {
				action := map[string]string{
					http.MethodPost:   "create",
					http.MethodPut:    "update",
					http.MethodDelete: "delete",
				}[r.Method]
				if action == "" {
					action = strings.ToLower(r.Method)
				}
				s.audit(r, action, moduleFromPath(path), path, "", nil, nil)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// moduleFromPath derives a human module name from the URL path.
func moduleFromPath(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	names := map[string]string{
		"invoice": "Facturación", "customer": "Clientes", "supplier": "Proveedores",
		"product": "Productos", "purchase": "Compras", "account": "Contabilidad",
		"bank": "Bancos", "service": "Servicios", "quotation": "Cotizaciones",
		"return": "Devoluciones", "adjustment": "Ajustes", "user": "Usuarios",
		"role": "Roles", "settings": "Configuración", "reports": "Reportes",
	}
	if len(segs) > 0 {
		if n, ok := names[segs[0]]; ok {
			return n
		}
		return segs[0]
	}
	return "sistema"
}

// ---------- Audit viewer ----------

func (s *Server) registerAuditRoutes() {
	s.mux.HandleFunc("GET /reports/audit-log", s.auditLog)
	s.mux.HandleFunc("GET /reports/audit-log/data", s.auditLogData)
	s.mux.HandleFunc("GET /reports/audit-log/verify", s.auditLogVerify)
	s.mux.HandleFunc("GET /reports/audit-log/blocks", s.auditLogBlocks)
	s.mux.HandleFunc("GET /reports/audit-log/export", s.auditLogExport)
}

func (s *Server) auditLog(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), time.Now().AddDate(0, -1, 0).Format("2006-01-02"))
	to := orDefault(r.FormValue("to"), time.Now().Format("2006-01-02"))
	s.render(w, r, "audit_log.html", map[string]interface{}{
		"Title":     "Pista de Auditoría",
		"From":      from,
		"To":        to,
		"Periods":   s.auditPeriodList(r),
		"PeriodNow": auditPeriod(time.Now()),
	})
}

// auditBlockRow es un bloque ya cerrado, para el selector del auditor.
type auditBlockRow struct {
	Period      string `json:"period"`
	RecordCount int    `json:"record_count"`
	FirstID     int64  `json:"first_id"`
	LastID      int64  `json:"last_id"`
	BlockHash   string `json:"block_hash"`
	SealedAt    string `json:"sealed_at"`
}

// auditPeriodList devuelve los períodos existentes: los sellados más el que
// está en curso, para que el verificador pueda elegir un bloque acotado.
func (s *Server) auditPeriodList(r *http.Request) []string {
	seen := map[string]bool{}
	var out []string
	rows, err := s.dbFor(r).Query(`SELECT DISTINCT substr(created_at,1,7) FROM audit_log ORDER BY 1 DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p sql.NullString
			if rows.Scan(&p) == nil && p.Valid && p.String != "" && !seen[p.String] {
				seen[p.String] = true
				out = append(out, p.String)
			}
		}
	}
	if now := auditPeriod(time.Now()); !seen[now] {
		out = append(out, now)
	}
	return out
}

// auditLogBlocks lista los bloques sellados (integridad por período).
func (s *Server) auditLogBlocks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT period, record_count, first_id, last_id, block_hash, sealed_at
		FROM audit_chain_blocks ORDER BY period DESC`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var list []auditBlockRow
	for rows.Next() {
		var b auditBlockRow
		var firstID, lastID, sealedAt sql.NullString
		if rows.Scan(&b.Period, &b.RecordCount, &firstID, &lastID, &b.BlockHash, &sealedAt) != nil {
			continue
		}
		b.FirstID, b.LastID = parseNullInt64(firstID), parseNullInt64(lastID)
		b.SealedAt = sealedAt.String
		list = append(list, b)
	}
	writeJSON(w, map[string]interface{}{"data": list, "current": auditPeriod(time.Now())})
}

// parseNullInt64 convierte un entero escaneado como NullString.
func parseNullInt64(s sql.NullString) int64 {
	var n int64
	if s.Valid {
		_, _ = fmt.Sscanf(s.String, "%d", &n)
	}
	return n
}

func (s *Server) auditLogData(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), time.Now().AddDate(0, -1, 0).Format("2006-01-02"))
	to := orDefault(r.FormValue("to"), time.Now().Format("2006-01-02"))
	rows, err := s.dbFor(r).Query(`SELECT id, user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at, prev_hash, record_hash
		FROM audit_log WHERE created_at BETWEEN ? AND ? ORDER BY id DESC LIMIT 1000`, from, to+" 23:59:59")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type logRow struct {
		ID         int64  `json:"id"`
		UserID     string `json:"user_id"`
		Action     string `json:"action"`
		Module     string `json:"module"`
		Table      string `json:"table_name"`
		RecordID   string `json:"record_id"`
		OldValue   string `json:"old_value"`
		NewValue   string `json:"new_value"`
		IP         string `json:"ip_address"`
		CreatedAt  string `json:"created_at"`
		RecordHash string `json:"record_hash"`
	}
	var list []logRow
	for rows.Next() {
		var id int64
		var uid, action, module, table, recordID, oldVal, newVal, ip, createdAt, prevH, hash sql.NullString
		_ = rows.Scan(&id, &uid, &action, &module, &table, &recordID, &oldVal, &newVal, &ip, &createdAt, &prevH, &hash)
		list = append(list, logRow{id, uid.String, action.String, module.String, table.String, recordID.String, oldVal.String, newVal.String, ip.String, createdAt.String, hash.String})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

// auditLogVerify valida UN bloque (período) por defecto, no toda la tabla:
// recorrer el histórico completo con millones de registros colapsaba. El
// alcance completo sigue disponible, pero de forma explícita (?all=1), que es
// lo que puede pedir un auditor con un rango acotado.
func (s *Server) auditLogVerify(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("all") == "1" {
		res, err := s.auditChainRange(r, "", "")
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if res.BrokenAt > 0 {
			writeJSON(w, map[string]interface{}{
				"valid": false, "total": res.Count, "broken_at": res.BrokenAt,
				"scope": "all", "period": "", "sealed": false,
				"reason": res.Reason,
			})
			return
		}
		writeJSON(w, map[string]interface{}{
			"valid": true, "total": res.Count, "broken_at": nil,
			"scope": "all", "period": "", "sealed": false,
			"reason": fmt.Sprintf("cadena íntegra: %d registros verificados, hash final %s", res.Count, shortHash(res.EndHash)),
		})
		return
	}

	period := r.FormValue("period")
	if period == "" {
		period = auditPeriod(time.Now())
	}
	if _, err := time.Parse("2006-01", period); err != nil {
		jsonError(w, "período inválido: use el formato YYYY-MM", 400)
		return
	}
	from, to := auditPeriodBounds(period)
	res, err := s.auditChainRange(r, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if res.BrokenAt > 0 {
		writeJSON(w, map[string]interface{}{
			"valid": false, "total": res.Count, "broken_at": res.BrokenAt,
			"scope": "period", "period": period, "sealed": false,
			"reason": res.Reason,
		})
		return
	}

	// El sello del bloque cerrado, si existe, se contrasta con el recálculo.
	var bStart, bEnd, bHash, bPrev sql.NullString
	sealed := s.dbFor(r).QueryRow(`SELECT start_hash, end_hash, block_hash, prev_block_hash
		FROM audit_chain_blocks WHERE period = ?`, period).Scan(&bStart, &bEnd, &bHash, &bPrev) == nil
	if sealed {
		if bEnd.String != res.EndHash {
			writeJSON(w, map[string]interface{}{
				"valid": false, "total": res.Count, "broken_at": nil,
				"scope": "period", "period": period, "sealed": true,
				"reason": fmt.Sprintf("el sello del bloque %s no coincide con la cadena recalculada (%s vs %s): el período fue alterado después de cerrarse", period, shortHash(bEnd.String), shortHash(res.EndHash)),
			})
			return
		}
		if computeBlockHash(bPrev.String, bStart.String, bEnd.String, res.FirstID, res.LastID, res.Count) != bHash.String {
			writeJSON(w, map[string]interface{}{
				"valid": false, "total": res.Count, "broken_at": nil,
				"scope": "period", "period": period, "sealed": true,
				"reason": fmt.Sprintf("el sello del bloque %s fue modificado: block_hash no corresponde a sus propios datos", period),
			})
			return
		}
		var prevEnd sql.NullString
		if err := s.dbFor(r).QueryRow(`SELECT end_hash FROM audit_chain_blocks WHERE period < ? ORDER BY period DESC LIMIT 1`, period).Scan(&prevEnd); err == nil {
			if prevEnd.String != bStart.String {
				writeJSON(w, map[string]interface{}{
					"valid": false, "total": res.Count, "broken_at": nil,
					"scope": "period", "period": period, "sealed": true,
					"reason": fmt.Sprintf("enlace de bloques roto en %s: no arranca en el hash final del bloque anterior (posible eliminación de un mes completo)", period),
				})
				return
			}
		}
		writeJSON(w, map[string]interface{}{
			"valid": true, "total": res.Count, "broken_at": nil,
			"scope": "period", "period": period, "sealed": true,
			"block_hash": bHash.String,
			"reason":     fmt.Sprintf("bloque %s íntegro y sellado: %d registros verificados, hash de bloque %s", period, res.Count, shortHash(bHash.String)),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"valid": true, "total": res.Count, "broken_at": nil,
		"scope": "period", "period": period, "sealed": false,
		"reason": fmt.Sprintf("cadena íntegra del período en curso %s: %d registros verificados, hash final %s (se sella al cerrar el mes)", period, res.Count, shortHash(res.EndHash)),
	})
}

func shortHash(h string) string {
	if len(h) > 16 {
		return h[:16] + "…"
	}
	return h
}

// auditLogExport streams the audit log as CSV for external auditors.
func (s *Server) auditLogExport(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), "2000-01-01")
	to := orDefault(r.FormValue("to"), time.Now().Format("2006-01-02"))
	rows, err := s.dbFor(r).Query(`SELECT id, user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at, prev_hash, record_hash
		FROM audit_log WHERE created_at BETWEEN ? AND ? ORDER BY id ASC`, from, to+" 23:59:59")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="auditoria_erpgow_%s_%s.csv"`, from, to))
	// BOM so Excel opens it UTF-8 correctly
	w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID", "Usuario", "Acción", "Módulo", "Ruta/Tabla", "Registro", "Valor Anterior", "Valor Nuevo", "IP", "Fecha", "Hash Previo", "Hash Registro"})
	var id int64
	var uid, action, module, table, recordID, oldVal, newVal, ip, createdAt, prevH, hash sql.NullString
	for rows.Next() {
		_ = rows.Scan(&id, &uid, &action, &module, &table, &recordID, &oldVal, &newVal, &ip, &createdAt, &prevH, &hash)
		_ = cw.Write([]string{fmt.Sprintf("%d", id), uid.String, action.String, module.String, table.String, recordID.String, oldVal.String, newVal.String, ip.String, createdAt.String, prevH.String, hash.String})
	}
	cw.Flush()
}

// marshalJSON is a nil-safe JSON helper.
func marshalJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	return mustJSON(v)
}
