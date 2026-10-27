package web

// pos_shift_sync.go — cola de TURNOS del POS (apertura/cierre) y latido.
//
// POR QUÉ NO SE REUTILIZA /api/pos/shift/open.
//
// Aquellas rutas viven detrás de posAPIMiddleware, que exige un X-POS-Token de
// sesión. Ese token caduca (y el cajero puede cerrar sesión) mientras la cola
// offline del POS puede tardar horas o días en reconectar. Una caja que estuvo
// sin red toda la tarde no puede perder sus turnos porque el token expiró.
//
// Estas rutas usan la misma autenticación que el resto de la sincronización:
// API key de terminal (posSyncKeyMiddleware), que no caduca. La empresa sale de
// la key, nunca de lo que declare el cliente.
//
// IDEMPOTENCIA. La caja reintenta hasta confirmar. Cada apertura/cierre lleva
// (terminal_id, local_turn_id) y se registra en pos_shift_sync. Un reintento
// devuelve el mismo remote_id y no duplica el evento de RRHH.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// posSyncAPIVersion permite que la caja detecte un servidor incompatible antes
// de vaciar su cola contra él.
const posSyncAPIVersion = 1

// posPingResponse certifica, en una sola llamada barata, que la caja habla con
// el servidor correcto: empresa, sucursal y terminal. Sustituye a descargar el
// catálogo entero cada 30 s solo para saber si hay red.
type posPingResponse struct {
	Success       bool   `json:"success"`
	APIVersion    int    `json:"api_version"`
	Tenant        string `json:"tenant"`
	BranchID      int    `json:"branch_id"`
	BranchName    string `json:"branch_name"`
	TerminalID    string `json:"terminal_id"`
	ServerTime    string `json:"server_time"`
	ServerUnix    int64  `json:"server_unix"`
	ServerVersion string `json:"server_version"`
}

// handlePOSPing responde al latido del terminal. Exige API key (middleware) y
// devuelve la identidad que el servidor reconoce para esa key.
func (s *Server) handlePOSPing(w http.ResponseWriter, r *http.Request) {
	db, err := s.posSyncDB(r)
	if err != nil {
		jsonError(w, "Base no resuelta", http.StatusInternalServerError)
		return
	}

	var branchID int
	var branchName sql.NullString
	err = db.QueryRow(`
		SELECT b.id, b.name FROM api_keys ak
		JOIN branches b ON b.id = ak.branch_id
		WHERE ak.api_key = ? AND ak.is_active = 1 LIMIT 1`,
		extractPOSAPIKey(r)).Scan(&branchID, &branchName)
	if err == sql.ErrNoRows {
		jsonError(w, "API key inválida", http.StatusUnauthorized)
		return
	}
	if err != nil {
		jsonError(w, "No se pudo resolver la sucursal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tenant, _ := r.Context().Value(posSyncTenantKey{}).(string)
	now := time.Now()
	writeJSON(w, posPingResponse{
		Success:       true,
		APIVersion:    posSyncAPIVersion,
		Tenant:        tenant,
		BranchID:      branchID,
		BranchName:    branchName.String,
		TerminalID:    strings.TrimSpace(r.Header.Get("X-Terminal-ID")),
		ServerTime:    now.Format(time.RFC3339),
		ServerUnix:    now.Unix(),
		ServerVersion: "erpgo-pos-sync/" + fmt.Sprint(posSyncAPIVersion),
	})
}

// posShiftOpenRequest es la apertura tal como la registró la caja, sin red.
type posShiftOpenRequest struct {
	TerminalID     string  `json:"terminal_id"`
	LocalTurnID    int64   `json:"local_turn_id"`
	BranchID       int     `json:"branch_id"`
	UserID         string  `json:"user_id"`
	UserName       string  `json:"user_name"`
	SupervisorID   string  `json:"supervisor_id"`
	SupervisorName string  `json:"supervisor_name"`
	OpeningVES     float64 `json:"opening_ves"`
	OpeningUSD     float64 `json:"opening_usd"`
	OpenedAt       string  `json:"opened_at"`
	TillPurpose    string  `json:"till_purpose"`
	Notes          string  `json:"notes"`
}

// posShiftCloseRequest es el cierre tal como la caja lo calculó.
type posShiftCloseRequest struct {
	TerminalID     string  `json:"terminal_id"`
	LocalTurnID    int64   `json:"local_turn_id"`
	BranchID       int     `json:"branch_id"`
	UserID         string  `json:"user_id"`
	UserName       string  `json:"user_name"`
	SupervisorID   string  `json:"supervisor_id"`
	SupervisorName string  `json:"supervisor_name"`
	OpeningVES     float64 `json:"opening_ves"`
	OpeningUSD     float64 `json:"opening_usd"`
	DeclaredVES    float64 `json:"declared_ves"`
	DeclaredUSD    float64 `json:"declared_usd"`
	SystemVES      float64 `json:"system_ves"`
	SystemUSD      float64 `json:"system_usd"`
	DifferenceVES  float64 `json:"difference_ves"`
	DifferenceUSD  float64 `json:"difference_usd"`
	TotalSales     float64 `json:"total_sales"`
	TotalRefunds   float64 `json:"total_refunds"`
	SalesCount     int     `json:"sales_count"`
	ClosureType    string  `json:"closure_type"`
	Notes          string  `json:"notes"`
	ClosedAt       string  `json:"closed_at"`
	RemoteSession  int     `json:"remote_session_id"`
}

// posShiftSyncResponse es la confirmación que la caja necesita para borrar el
// turno de su cola. success=true y remote_id estable en cada reintento.
type posShiftSyncResponse struct {
	Success       bool   `json:"success"`
	RemoteID      string `json:"remote_id"`
	SessionID     int    `json:"session_id,omitempty"`
	AlreadyExists bool   `json:"already_exists"`
	Message       string `json:"message,omitempty"`
}

// handlePOSShiftOpenSync recibe la apertura de un turno desde la cola offline.
func (s *Server) handlePOSShiftOpenSync(w http.ResponseWriter, r *http.Request) {
	db, err := s.posSyncDB(r)
	if err != nil {
		jsonError(w, "Base no resuelta", http.StatusInternalServerError)
		return
	}

	var req posShiftOpenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "JSON inválido: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.TerminalID = strings.TrimSpace(req.TerminalID)
	if req.TerminalID == "" {
		req.TerminalID = strings.TrimSpace(r.Header.Get("X-Terminal-ID"))
	}
	if req.TerminalID == "" || req.LocalTurnID <= 0 {
		jsonError(w, "terminal_id y local_turn_id son obligatorios", http.StatusBadRequest)
		return
	}

	openedAt := posParseTime(req.OpenedAt)
	if !openedAt.Valid {
		openedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	// 1. Guardar el turno portable (fuente de verdad para el ERP y RRHH).
	shiftID, err := posUpsertShiftOpen(db, req, openedAt.Time)
	if err != nil {
		jsonError(w, "No se pudo guardar el turno: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. ¿Primera vez que se ve esta apertura?
	first, err := posMarkShiftSync(db, req.TerminalID, req.LocalTurnID, "open", shiftID)
	if err != nil {
		jsonError(w, "No se pudo registrar la sincronización: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := 0
	if first {
		// 3. Espejo en cash_session para que el resto del ERP lo vea como caja
		//    abierta. Es best-effort: pos_shifts ya tiene el dato completo.
		sessionID = posMirrorShiftOpen(db, req, openedAt.Time)
		posLogShiftEvent(db, req.TerminalID, req.BranchID, req.UserID, "open_turn", req.SupervisorID,
			fmt.Sprintf("Apertura turno local #%d — VES %.2f / USD %.2f%s",
				req.LocalTurnID, req.OpeningVES, req.OpeningUSD, posSupervisorSuffix(req.SupervisorName)))
	}

	writeJSON(w, posShiftSyncResponse{
		Success:       true,
		RemoteID:      fmt.Sprintf("%d", shiftID),
		SessionID:     sessionID,
		AlreadyExists: !first,
	})
}

// handlePOSShiftCloseSync recibe el cierre de un turno desde la cola offline.
func (s *Server) handlePOSShiftCloseSync(w http.ResponseWriter, r *http.Request) {
	db, err := s.posSyncDB(r)
	if err != nil {
		jsonError(w, "Base no resuelta", http.StatusInternalServerError)
		return
	}

	var req posShiftCloseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "JSON inválido: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.TerminalID = strings.TrimSpace(req.TerminalID)
	if req.TerminalID == "" {
		req.TerminalID = strings.TrimSpace(r.Header.Get("X-Terminal-ID"))
	}
	if req.TerminalID == "" || req.LocalTurnID <= 0 {
		jsonError(w, "terminal_id y local_turn_id son obligatorios", http.StatusBadRequest)
		return
	}

	closedAt := posParseTime(req.ClosedAt)
	if !closedAt.Valid {
		closedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	shiftID, err := posUpsertShiftClose(db, req, closedAt.Time)
	if err != nil {
		jsonError(w, "No se pudo guardar el cierre: "+err.Error(), http.StatusInternalServerError)
		return
	}

	first, err := posMarkShiftSync(db, req.TerminalID, req.LocalTurnID, "close", shiftID)
	if err != nil {
		jsonError(w, "No se pudo registrar la sincronización: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if first {
		posMirrorShiftClose(db, req, closedAt.Time)
		posLogShiftEvent(db, req.TerminalID, req.BranchID, req.UserID, "close_turn", req.SupervisorID,
			fmt.Sprintf("Cierre %s turno local #%d — declarado VES %.2f / USD %.2f; sistema VES %.2f / USD %.2f; diferencia VES %.2f / USD %.2f; %d ventas (%.2f)%s",
				req.ClosureType, req.LocalTurnID, req.DeclaredVES, req.DeclaredUSD,
				req.SystemVES, req.SystemUSD, req.DifferenceVES, req.DifferenceUSD,
				req.SalesCount, req.TotalSales, posSupervisorSuffix(req.SupervisorName)))
	}

	writeJSON(w, posShiftSyncResponse{
		Success:       true,
		RemoteID:      fmt.Sprintf("%d", shiftID),
		AlreadyExists: !first,
	})
}

// handlePOSShiftStatusSync devuelve el turno tal como lo tiene el servidor.
// La caja lo usa para reconciliar tras una reconexión larga.
func (s *Server) handlePOSShiftStatusSync(w http.ResponseWriter, r *http.Request) {
	db, err := s.posSyncDB(r)
	if err != nil {
		jsonError(w, "Base no resuelta", http.StatusInternalServerError)
		return
	}
	terminalID := strings.TrimSpace(r.URL.Query().Get("terminal_id"))
	if terminalID == "" {
		terminalID = strings.TrimSpace(r.Header.Get("X-Terminal-ID"))
	}
	if terminalID == "" {
		jsonError(w, "terminal_id requerido", http.StatusBadRequest)
		return
	}

	query := `SELECT id, local_turn_id, terminal_id, branch_id, user_id, user_name,
	                 supervisor_id, supervisor_name, opened_at, closed_at,
	                 opening_ves, opening_usd, declared_ves, declared_usd,
	                 system_ves, system_usd, difference_ves, difference_usd,
	                 total_sales, total_refunds, sales_count, status, closure_type, notes
	          FROM pos_shifts WHERE terminal_id = ?`
	args := []interface{}{terminalID}
	if raw := strings.TrimSpace(r.URL.Query().Get("local_turn_id")); raw != "" {
		var id int64
		if _, err := fmt.Sscan(raw, &id); err == nil {
			query += " AND local_turn_id = ?"
			args = append(args, id)
		}
	}
	query += " ORDER BY opened_at DESC LIMIT 20"

	rows, err := db.Query(query, args...)
	if err != nil {
		jsonError(w, "No se pudo consultar el turno: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	shifts := []map[string]interface{}{}
	for rows.Next() {
		var (
			id, localTurnID, branchID, salesCount             int64
			terminal, userID, userName, supervisorID, supName sql.NullString
			status, closureType, notes                        sql.NullString
			openedAt, closedAt                                sql.NullTime
			openingVES, openingUSD, declaredVES, declaredUSD  float64
			systemVES, systemUSD, diffVES, diffUSD            float64
			totalSales, totalRefunds                          float64
		)
		if err := rows.Scan(&id, &localTurnID, &terminal, &branchID, &userID, &userName,
			&supervisorID, &supName, &openedAt, &closedAt,
			&openingVES, &openingUSD, &declaredVES, &declaredUSD,
			&systemVES, &systemUSD, &diffVES, &diffUSD,
			&totalSales, &totalRefunds, &salesCount, &status, &closureType, &notes); err != nil {
			jsonError(w, "No se pudo leer el turno: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shifts = append(shifts, map[string]interface{}{
			"remote_id":       id,
			"local_turn_id":   localTurnID,
			"terminal_id":     terminal.String,
			"branch_id":       branchID,
			"user_id":         userID.String,
			"user_name":       userName.String,
			"supervisor_id":   supervisorID.String,
			"supervisor_name": supName.String,
			"opened_at":       posFormatTime(openedAt),
			"closed_at":       posFormatTime(closedAt),
			"opening_ves":     openingVES,
			"opening_usd":     openingUSD,
			"declared_ves":    declaredVES,
			"declared_usd":    declaredUSD,
			"system_ves":      systemVES,
			"system_usd":      systemUSD,
			"difference_ves":  diffVES,
			"difference_usd":  diffUSD,
			"total_sales":     totalSales,
			"total_refunds":   totalRefunds,
			"sales_count":     salesCount,
			"status":          status.String,
			"closure_type":    closureType.String,
			"notes":           notes.String,
		})
	}
	if err := rows.Err(); err != nil {
		jsonError(w, "No se pudo leer el turno: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"success": true, "shifts": shifts})
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// posUpsertShiftOpen guarda/actualiza la apertura y devuelve el id portable.
func posUpsertShiftOpen(db *sql.DB, req posShiftOpenRequest, openedAt time.Time) (int64, error) {
	_, err := db.Exec(`
		INSERT INTO pos_shifts
			(terminal_id, local_turn_id, branch_id, user_id, user_name,
			 supervisor_id, supervisor_name, opened_at, opening_ves, opening_usd,
			 status, till_purpose, notes, synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?, 'open', ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			branch_id = VALUES(branch_id),
			user_id = VALUES(user_id),
			user_name = VALUES(user_name),
			supervisor_id = VALUES(supervisor_id),
			supervisor_name = VALUES(supervisor_name),
			opened_at = VALUES(opened_at),
			opening_ves = VALUES(opening_ves),
			opening_usd = VALUES(opening_usd),
			till_purpose = VALUES(till_purpose),
			notes = VALUES(notes),
			synced_at = NOW()`,
		req.TerminalID, req.LocalTurnID, req.BranchID, req.UserID, req.UserName,
		req.SupervisorID, req.SupervisorName, openedAt, req.OpeningVES, req.OpeningUSD,
		req.TillPurpose, req.Notes)
	if err != nil {
		return 0, err
	}
	return posShiftID(db, req.TerminalID, req.LocalTurnID)
}

// posUpsertShiftClose guarda/actualiza el cierre y devuelve el id portable.
// Si el servidor nunca vio la apertura (por ejemplo, se perdió la respuesta),
// crea la fila con lo que el cierre trae.
func posUpsertShiftClose(db *sql.DB, req posShiftCloseRequest, closedAt time.Time) (int64, error) {
	_, err := db.Exec(`
		INSERT INTO pos_shifts
			(terminal_id, local_turn_id, branch_id, user_id, user_name,
			 supervisor_id, supervisor_name, opened_at, closed_at,
			 opening_ves, opening_usd,
			 declared_ves, declared_usd, system_ves, system_usd,
			 difference_ves, difference_usd, total_sales, total_refunds, sales_count,
			 status, closure_type, notes, remote_session_id, synced_at)
		VALUES (?,?,?,?,?,?,?, NULL, ?,?,?,?,?,?,?,?,?,?,?,?,?, 'closed', ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			branch_id = VALUES(branch_id),
			user_id = VALUES(user_id),
			user_name = VALUES(user_name),
			supervisor_id = VALUES(supervisor_id),
			supervisor_name = VALUES(supervisor_name),
			closed_at = VALUES(closed_at),
			opening_ves = IF(opening_ves = 0, VALUES(opening_ves), opening_ves),
			opening_usd = IF(opening_usd = 0, VALUES(opening_usd), opening_usd),
			declared_ves = VALUES(declared_ves),
			declared_usd = VALUES(declared_usd),
			system_ves = VALUES(system_ves),
			system_usd = VALUES(system_usd),
			difference_ves = VALUES(difference_ves),
			difference_usd = VALUES(difference_usd),
			total_sales = VALUES(total_sales),
			total_refunds = VALUES(total_refunds),
			sales_count = VALUES(sales_count),
			status = 'closed',
			closure_type = VALUES(closure_type),
			notes = VALUES(notes),
			remote_session_id = COALESCE(VALUES(remote_session_id), remote_session_id),
			synced_at = NOW()`,
		req.TerminalID, req.LocalTurnID, req.BranchID, req.UserID, req.UserName,
		req.SupervisorID, req.SupervisorName, closedAt,
		req.OpeningVES, req.OpeningUSD,
		req.DeclaredVES, req.DeclaredUSD, req.SystemVES, req.SystemUSD,
		req.DifferenceVES, req.DifferenceUSD, req.TotalSales, req.TotalRefunds, req.SalesCount,
		req.ClosureType, req.Notes, posNullInt(req.RemoteSession))
	if err != nil {
		return 0, err
	}
	return posShiftID(db, req.TerminalID, req.LocalTurnID)
}

func posShiftID(db *sql.DB, terminalID string, localTurnID int64) (int64, error) {
	var id int64
	if err := db.QueryRow(`SELECT id FROM pos_shifts WHERE terminal_id = ? AND local_turn_id = ?`,
		terminalID, localTurnID).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// posMarkShiftSync registra (terminal, turno, tipo) y dice si es la primera vez.
// INSERT IGNORE + RowsAffected es la forma atómica de saberlo sin carrera entre
// dos reintentos simultáneos del mismo terminal.
func posMarkShiftSync(db *sql.DB, terminalID string, localTurnID int64, kind string, remoteID int64) (bool, error) {
	res, err := db.Exec(`
		INSERT IGNORE INTO pos_shift_sync (terminal_id, local_turn_id, kind, remote_id, status)
		VALUES (?,?,?,?, 'ok')`,
		terminalID, localTurnID, kind, fmt.Sprintf("%d", remoteID))
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// posMirrorShiftOpen abre (o reutiliza) la sesión de caja del ERP.
func posMirrorShiftOpen(db *sql.DB, req posShiftOpenRequest, openedAt time.Time) int {
	cashier := posTrunc(req.UserID, 30)
	if cashier == "" {
		cashier = "POS-SYSTEM"
	}
	warehouse := posCashWarehouse(req.TerminalID)

	var sessionID int
	err := db.QueryRow(`
		SELECT id FROM cash_session
		WHERE cashier = ? AND warehouse_id = ? AND status = 1
		ORDER BY opened_at DESC LIMIT 1`, cashier, warehouse).Scan(&sessionID)
	if err == nil {
		return sessionID
	}
	if err != sql.ErrNoRows {
		log.Printf("POS shift sync: no se pudo consultar cash_session: %v", err)
		return 0
	}

	res, err := db.Exec(`
		INSERT INTO cash_session
			(cashier, warehouse_id, opening_amount, opening_amount_usd, status, currency_,
			 opened_at, open_note)
		VALUES (?,?,?,?, 1, 'VES', ?, ?)`,
		cashier, warehouse, req.OpeningVES, req.OpeningUSD, openedAt,
		strings.TrimSpace(req.TillPurpose+" "+req.Notes))
	if err != nil {
		log.Printf("POS shift sync: no se pudo crear cash_session: %v", err)
		return 0
	}
	if id, err := res.LastInsertId(); err == nil {
		sessionID = int(id)
	}
	return sessionID
}

// posMirrorShiftClose cierra la sesión de caja del ERP.
func posMirrorShiftClose(db *sql.DB, req posShiftCloseRequest, closedAt time.Time) {
	cashier := posTrunc(req.UserID, 30)
	if cashier == "" {
		cashier = "POS-SYSTEM"
	}
	warehouse := posCashWarehouse(req.TerminalID)

	closureType := strings.TrimSpace(req.ClosureType)
	if closureType == "" {
		closureType = "X"
	}
	supervisor := 0
	if n, err := parseIntOrZero(req.SupervisorID); err == nil {
		supervisor = n
	}
	integrity := fmt.Sprintf("pos=%d;terminal=%s;user=%s;declared=%.2f/%.2f;system=%.2f/%.2f;diff=%.2f/%.2f;type=%s;closed_at=%s",
		req.LocalTurnID, req.TerminalID, cashier, req.DeclaredVES, req.DeclaredUSD,
		req.SystemVES, req.SystemUSD, req.DifferenceVES, req.DifferenceUSD,
		closureType, closedAt.Format("2006-01-02T15:04:05"))

	var err error
	if req.RemoteSession > 0 {
		_, err = db.Exec(`
			UPDATE cash_session SET status = 0, closed_at = ?,
				closing_amount = ?, closing_amount_usd = ?,
				expected_cash = ?, expected_cash_usd = ?,
				difference = ?, difference_usd = ?, close_note = ?,
				closure_type = ?, supervisor_id = ?,
				approved_at = CASE WHEN ? > 0 THEN NOW() ELSE approved_at END,
				integrity_hash = ?
			WHERE id = ?`,
			closedAt, req.DeclaredVES, req.DeclaredUSD, req.SystemVES, req.SystemUSD,
			req.DifferenceVES, req.DifferenceUSD, req.Notes, closureType, supervisor,
			supervisor, integrity, req.RemoteSession)
	} else {
		_, err = db.Exec(`
			UPDATE cash_session SET status = 0, closed_at = ?,
				closing_amount = ?, closing_amount_usd = ?,
				expected_cash = ?, expected_cash_usd = ?,
				difference = ?, difference_usd = ?, close_note = ?,
				closure_type = ?, supervisor_id = ?,
				approved_at = CASE WHEN ? > 0 THEN NOW() ELSE approved_at END,
				integrity_hash = ?
			WHERE cashier = ? AND warehouse_id = ? AND status = 1
			ORDER BY opened_at DESC LIMIT 1`,
			closedAt, req.DeclaredVES, req.DeclaredUSD, req.SystemVES, req.SystemUSD,
			req.DifferenceVES, req.DifferenceUSD, req.Notes, closureType, supervisor,
			supervisor, integrity, cashier, warehouse)
	}
	if err != nil {
		log.Printf("POS shift sync: no se pudo cerrar cash_session: %v", err)
	}
}

// posLogShiftEvent escribe el evento de RRHH. Es best-effort: si falla, el
// turno ya está en pos_shifts y la caja no debe reintentar por esto.
func posLogShiftEvent(db *sql.DB, terminalID string, branchID int, userID, eventType, supervisorID, details string) {
	if strings.TrimSpace(userID) == "" {
		userID = "POS-SYSTEM"
	}
	_, err := db.Exec(`
		INSERT INTO pos_turn_events (terminal_id, branch_id, user_id, event_type, event_time, details, supervisor_id)
		VALUES (?, ?, ?, ?, NOW(), ?, NULLIF(?, ''))`,
		posTrunc(terminalID, 50), branchID, posTrunc(userID, 50), eventType, details,
		posTrunc(supervisorID, 50))
	if err != nil {
		log.Printf("POS shift sync: no se pudo registrar evento %s: %v", eventType, err)
	}
}

func posParseTime(raw string) sql.NullTime {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sql.NullTime{}
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	}
	return sql.NullTime{}
}

func posFormatTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

func posSupervisorSuffix(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return " — supervisor: " + strings.TrimSpace(name)
}

// posTrunc recorta al tamaño de la columna destino. Las columnas de
// cash_session/pos_turn_events son más cortas que las de pos_shifts; sin este
// recorte, MySQL en modo estricto rechaza el espejo y el turno queda solo en
// pos_shifts (no se pierde, pero el resto del ERP no lo ve).
func posTrunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// posCashWarehouse deriva un warehouse_id estable y único para el terminal.
//
// cash_session.warehouse_id es varchar(30) y el ID generado por el POS puede
// tener 32 caracteres. En vez de truncar a ciegas (dos cajas con el mismo
// prefijo chocarían), si no cabe se conserva el prefijo y se añade un hash de
// 6 hex del ID completo.
func posCashWarehouse(terminalID string) string {
	t := strings.TrimSpace(terminalID)
	if t == "" {
		return "POS"
	}
	if len(t) <= 30 {
		return t
	}
	sum := sha256.Sum256([]byte(t))
	suffix := hex.EncodeToString(sum[:3])
	keep := 30 - len(suffix) - 1
	if keep < 1 {
		keep = 1
	}
	return t[:keep] + "-" + suffix
}

func posNullInt(v int) interface{} {
	if v <= 0 {
		return nil
	}
	return v
}

func parseIntOrZero(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var n int
	_, err := fmt.Sscan(s, &n)
	return n, err
}
