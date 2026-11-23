package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Bloques de numeración fiscal offline por caja
// ---------------------------------------------------------------------------
// Cada caja recibe bloques exclusivos de números fiscales del servidor.
// Esto evita duplicados cuando múltiples cajas operan offline simultáneamente.
//
// F-3 (auditoría 2026-11-22): la reserva y el consumo vivían sin transacción
// —dos cajas que pedían bloque a la vez leían el mismo contador y recibían el
// MISMO rango; un consumo concurrente repetía número; y si el UPDATE de
// sincronización fallaba después de insertar el bloque, el rango quedaba
// activo mientras el contador seguía abajo (reutilización de controles
// fiscales: el peor de los fallos SENIAT)—. Ahora cada operación es UNA
// transacción sobre el contador (UPDATE-primero, igual que nextFiscalDoc)
// bajo un mutex de proceso, y el consumo es IDEMPOTENTE por venta: el mismo
// sale_local_id devuelve el mismo número en vez de quemar otro.

const (
	defaultBlockSize   = 100
	maxBlocksize       = 1000
	blockReserveExpiry = 24 * time.Hour
)

// fiscalBloquesMu serializa reserva y consumo dentro del proceso. El
// ERPGo es un solo proceso por servidor (decisión del producto), así que el
// mutex cierra la ventana entre leer el contador y escribir el bloque, que
// ni el aislamiento de la transacción cierra en todos los motores. La fila
// del contador da la protección extra para cualquier despliegue multi-proceso.
var fiscalBloquesMu sync.Mutex

// ReserveBlockRequest es la petición de reserva de un bloque fiscal.
type ReserveBlockRequest struct {
	TerminalID string `json:"terminal_id"`
	BranchID   int    `json:"branch_id"`
	Series     string `json:"series"`
	BlockSize  int    `json:"block_size"`
}

// ReserveBlockResponse es la respuesta con el bloque asignado.
type ReserveBlockResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	BlockID    int64  `json:"block_id"`
	BlockStart int64  `json:"block_start"`
	BlockEnd   int64  `json:"block_end"`
	UsedCount  int    `json:"used_count"`
}

// ConsumeFiscalNumberRequest consume un número fiscal del bloque.
type ConsumeFiscalNumberRequest struct {
	TerminalID  string `json:"terminal_id"`
	BlockID     int64  `json:"block_id"`
	SaleLocalID int64  `json:"sale_local_id"`
}

// ConsumeFiscalNumberResponse devuelve el número fiscal consumido.
type ConsumeFiscalNumberResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	FiscalNumber int64  `json:"fiscal_number"`
	ControlNo    string `json:"control_no"`
	BlockID      int64  `json:"block_id"`
}

// reserveFiscalBlockHandler reserva un bloque de números fiscales para una caja.
func (s *Server) reserveFiscalBlockHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ReserveBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: "invalid request body"})
		return
	}

	if req.TerminalID == "" {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: "terminal_id required"})
		return
	}

	if req.Series == "" {
		req.Series = "FACT"
	}

	blockSize := req.BlockSize
	if blockSize <= 0 {
		blockSize = defaultBlockSize
	}
	if blockSize > maxBlocksize {
		blockSize = maxBlocksize
	}

	db, err := s.posSyncDB(r)
	if err != nil || db == nil {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: "database unavailable"})
		return
	}

	// El mutex ordena las reservas del proceso; el UPDATE-primero sobre la
	// fila del contador es la protección real contra DOS CAJAS a la vez
	// (y contra cualquier segundo proceso que algún día exista).
	fiscalBloquesMu.Lock()
	defer fiscalBloquesMu.Unlock()

	tx, err := abrirTxReal(db)
	if err != nil {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("abrir transacción: %v", err)})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Verificar si la caja ya tiene un bloque activo —DENTRO de la tx:
	// comprobar fuera y decidir dentro era la ventana que permitía a dos
	// reservas hermanas de la misma caja quedarse cada una con su rango.
	var existingBlockID, existingUsed, existingStart, existingEnd int64
	err = tx.QueryRow(
		`SELECT id, used_count, block_start, block_end FROM pos_fiscal_blocks
		 WHERE terminal_id = ? AND series = ? AND status = 'active'
		 AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY id DESC LIMIT 1`,
		req.TerminalID, req.Series,
	).Scan(&existingBlockID, &existingUsed, &existingStart, &existingEnd)

	if err == nil && existingBlockID > 0 {
		// Devolver el bloque existente
		writeJSON(w, ReserveBlockResponse{
			Success:    true,
			Message:    "existing block returned",
			BlockID:    existingBlockID,
			BlockStart: existingStart,
			BlockEnd:   existingEnd,
			UsedCount:  int(existingUsed),
		})
		return
	}

	// Consumir blockSize del contador de la serie con UPDATE-primero
	// (mecanismo probado de nextFiscalDoc): el bloqueo exclusivo de la fila
	// dura hasta el commit, así que dos reservas simultáneas se ordenan en
	// vez de leer el mismo suelo.
	if _, err := tx.Exec(`UPDATE fiscal_series SET control_next = control_next + ? WHERE code = ?`,
		int64(blockSize), req.Series); err != nil {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("contador de serie: %v", err)})
		return
	}
	var nuevo int64
	if err := tx.QueryRow(`SELECT control_next FROM fiscal_series WHERE code = ?`, req.Series).Scan(&nuevo); err != nil {
		// La serie no existe todavía: se crea con el mismo primitivo que usa
		// la numeración central y se reintenta UNA vez (si siguiera sin
		// fila, es un fallo de base que el operador debe ver, no un rango
		// inventado).
		if e := s.ensureFiscalSeries(tx, req.Series); e != nil {
			writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("crear serie %s: %v", req.Series, e)})
			return
		}
		if _, err := tx.Exec(`UPDATE fiscal_series SET control_next = control_next + ? WHERE code = ?`,
			int64(blockSize), req.Series); err != nil {
			writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("contador de serie: %v", err)})
			return
		}
		if err := tx.QueryRow(`SELECT control_next FROM fiscal_series WHERE code = ?`, req.Series).Scan(&nuevo); err != nil {
			writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("leyendo serie tras crearla: %v", err)})
			return
		}
	}
	blockEnd := nuevo - 1
	blockStart := blockEnd - int64(blockSize) + 1

	// Piso de honestidad: ningún número del bloque puede estar por debajo de
	// un número YA usado (bases antiguas donde el contador quedó rezagado).
	// Si el rango calculado chocara, se desplaza y se refija el contador —
	// dentro de la misma tx y bajo el mismo bloqueo de fila.
	var maxUso int64
	_ = tx.QueryRow(`SELECT COALESCE(MAX(fiscal_number), 0) FROM pos_fiscal_block_usage WHERE series = ?`,
		req.Series).Scan(&maxUso)
	if maxUso >= blockStart {
		blockStart = maxUso + 1
		blockEnd = blockStart + int64(blockSize) - 1
		if _, err := tx.Exec(`UPDATE fiscal_series SET control_next = ? WHERE code = ?`,
			blockEnd+1, req.Series); err != nil {
			writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("refijar contador tras el piso: %v", err)})
			return
		}
	}

	// Insertar el bloque — en la MISMA transacción que el contador: si el
	// INSERT fallara, el rango consumido se devuelve con el rollback y no
	// queda hueco ni solape (antes quedaba el bloque activo con el
	// contador abajo: la receta del control duplicado).
	result, err := tx.Exec(
		`INSERT INTO pos_fiscal_blocks 
		 (terminal_id, branch_id, series, block_start, block_end, block_size, reserved_at, expires_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.TerminalID, req.BranchID, req.Series, blockStart, blockEnd, blockSize,
		time.Now(), time.Now().Add(blockReserveExpiry),
	)
	if err != nil {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("reserving block: %v", err)})
		return
	}

	blockID, _ := result.LastInsertId()

	if err := tx.Commit(); err != nil {
		writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("confirmando reserva: %v", err)})
		return
	}

	writeJSON(w, ReserveBlockResponse{
		Success:    true,
		Message:    "block reserved",
		BlockID:    blockID,
		BlockStart: blockStart,
		BlockEnd:   blockEnd,
		UsedCount:  0,
	})
}

// controlNoDesde lee el prefijo de control de la serie FACT (dentro de la tx
// que numera) y forma el número de control canónico del número dado.
func controlNoDesde(tx *sql.Tx, numero int64) string {
	controlPrefix := "00"
	var ctrlPrefix sql.NullString
	_ = tx.QueryRow(`SELECT control_prefix FROM fiscal_series WHERE code = 'FACT'`).Scan(&ctrlPrefix)
	if ctrlPrefix.Valid && strings.TrimSpace(ctrlPrefix.String) != "" {
		controlPrefix = strings.TrimSpace(ctrlPrefix.String)
	}
	return fmt.Sprintf("%s-%08d", controlPrefix, numero)
}

// consumeFiscalNumberHandler consume un número fiscal del bloque de la caja.
func (s *Server) consumeFiscalNumberHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ConsumeFiscalNumberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "invalid request body"})
		return
	}

	if req.TerminalID == "" {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "terminal_id required"})
		return
	}

	db, err := s.posSyncDB(r)
	if err != nil || db == nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "database unavailable"})
		return
	}

	fiscalBloquesMu.Lock()
	defer fiscalBloquesMu.Unlock()

	tx, err := abrirTxReal(db)
	if err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("abrir transacción: %v", err)})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// IDEMPOTENCIA POR VENTA (F-3, auditoría): el mismo (terminal,
	// sale_local_id) devuelve el número YA consumido en vez de quemar otro.
	// El terminal reintenta porque perdió la respuesta, no porque quiera dos
	// tickets; sin esta rama cada reintento desplazaba last_used y dejaba
	// números fantasma en el rango.
	if req.SaleLocalID != 0 {
		var ya int64
		if err := tx.QueryRow(`SELECT fiscal_number FROM pos_fiscal_block_usage
			WHERE terminal_id = ? AND sale_local_id = ? ORDER BY id DESC LIMIT 1`,
			req.TerminalID, req.SaleLocalID).Scan(&ya); err == nil && ya > 0 {
			var bloqueYa int64
			_ = tx.QueryRow(`SELECT block_id FROM pos_fiscal_block_usage
				WHERE terminal_id = ? AND sale_local_id = ? ORDER BY id DESC LIMIT 1`,
				req.TerminalID, req.SaleLocalID).Scan(&bloqueYa)
			writeJSON(w, ConsumeFiscalNumberResponse{
				Success:      true,
				Message:      "already consumed",
				FiscalNumber: ya,
				ControlNo:    controlNoDesde(tx, ya),
				BlockID:      bloqueYa,
			})
			return
		}
	}

	// Buscar el bloque activo de la caja —dentro de la tx y, si el terminal
	// trajo block_id, exactamente ese bloque: consumir "el último activo"
	// cuando la caja pidió uno específico era otra forma de repartir el mismo
	// número entre dos ventas.
	var blockID, blockStart, blockEnd, lastUsed int64
	if req.BlockID > 0 {
		err = tx.QueryRow(
			`SELECT id, block_start, block_end, last_used FROM pos_fiscal_blocks 
			 WHERE id = ? AND terminal_id = ? AND status = 'active'`,
			req.BlockID, req.TerminalID,
		).Scan(&blockID, &blockStart, &blockEnd, &lastUsed)
	} else {
		err = tx.QueryRow(
			`SELECT id, block_start, block_end, last_used FROM pos_fiscal_blocks 
			 WHERE terminal_id = ? AND status = 'active' 
			 AND (expires_at IS NULL OR expires_at > NOW())
			 ORDER BY id DESC LIMIT 1`,
			req.TerminalID,
		).Scan(&blockID, &blockStart, &blockEnd, &lastUsed)
	}

	if err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "no active block for terminal"})
		return
	}

	// Calcular el siguiente número disponible
	nextNum := lastUsed + 1
	if nextNum < blockStart {
		nextNum = blockStart
	}
	if nextNum > blockEnd {
		// Marcar bloque como agotado DENTRO de la tx: si el UPDATE fallara,
		// el rollback deja el bloque activo… y el error se reporta; nunca
		// queda "activo" con el contador arriba a ciegas.
		if _, err := tx.Exec(`UPDATE pos_fiscal_blocks SET status = 'exhausted' WHERE id = ?`, blockID); err != nil {
			log.Printf("fiscal: no se pudo marcar el bloque %d como agotado: %v", blockID, err)
			writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("fiscal close: %v", err)})
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("confirmando agotamiento: %v", err)})
			return
		}
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "block exhausted, request new block"})
		return
	}

	// Registrar el uso del número
	controlNo := controlNoDesde(tx, nextNum)

	if _, err := tx.Exec(
		`INSERT INTO pos_fiscal_block_usage (block_id, terminal_id, fiscal_number, sale_local_id, used_at) 
		 VALUES (?, ?, ?, ?, ?)`,
		blockID, req.TerminalID, nextNum, req.SaleLocalID, time.Now(),
	); err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("recording usage: %v", err)})
		return
	}

	// Avanzar el contador del bloque CON la condición de que siga donde lo
	// leímos (last_used = valor viejo). Filas tocadas != 1 significa que
	// alguien se cruzó: se deshace TODO y el terminal reintenta —prefiero
	// un reintento honesto a dos ventas con el mismo número fiscal—.
	res, err := tx.Exec(
		`UPDATE pos_fiscal_blocks SET used_count = used_count + 1, last_used = ? WHERE id = ? AND last_used = ?`,
		nextNum, blockID, lastUsed,
	)
	if err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("actualizando bloque: %v", err)})
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "el bloque cambió durante el consumo; reintente"})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("confirmando consumo: %v", err)})
		return
	}

	writeJSON(w, ConsumeFiscalNumberResponse{
		Success:      true,
		Message:      "number consumed",
		FiscalNumber: nextNum,
		ControlNo:    controlNo,
		BlockID:      blockID,
	})
}

// getFiscalBlockStatusHandler devuelve el estado del bloque de una caja.
func (s *Server) getFiscalBlockStatusHandler(w http.ResponseWriter, r *http.Request) {
	terminalID := r.URL.Query().Get("terminal_id")
	if terminalID == "" {
		http.Error(w, "terminal_id required", http.StatusBadRequest)
		return
	}

	db, err := s.posSyncDB(r)
	if err != nil || db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	var blockID, blockStart, blockEnd, usedCount, lastUsed int64
	var status string
	err = db.QueryRow(
		`SELECT id, block_start, block_end, used_count, last_used, status 
		 FROM pos_fiscal_blocks 
		 WHERE terminal_id = ? AND status = 'active'
		 ORDER BY id DESC LIMIT 1`,
		terminalID,
	).Scan(&blockID, &blockStart, &blockEnd, &usedCount, &lastUsed, &status)

	if err == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"block_id":    blockID,
			"block_start": blockStart,
			"block_end":   blockEnd,
			"used_count":  usedCount,
			"last_used":   lastUsed,
			"status":      status,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": "no active block",
	})
}

// parseBlockSize convierte un string a tamaño de bloque con límites.
func parseBlockSize(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultBlockSize
	}
	if n > maxBlocksize {
		return maxBlocksize
	}
	return n
}
