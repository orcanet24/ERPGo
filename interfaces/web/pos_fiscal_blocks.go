package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// Bloques de numeración fiscal offline por caja
// ---------------------------------------------------------------------------
// Cada caja recibe bloques exclusivos de números fiscales del servidor.
// Esto evita duplicados cuando múltiples cajas operan offline simultáneamente.

const (
	defaultBlockSize   = 100
	maxBlocksize       = 1000
	blockReserveExpiry = 24 * time.Hour
)

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

	// Verificar si la caja ya tiene un bloque activo
	var existingBlockID, existingUsed, existingStart, existingEnd int64
	err = db.QueryRow(
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

	// Reservar nuevo bloque: encontrar el último número usado en la serie
	var lastFiscalNum int64
	err = db.QueryRow(
		`SELECT COALESCE(MAX(fiscal_number), 0) FROM pos_fiscal_block_usage WHERE series = ?`,
		req.Series,
	).Scan(&lastFiscalNum)
	if err != nil {
		lastFiscalNum = 0
	}

	// También revisar la serie fiscal central
	var centralNext int64
	err = db.QueryRow(
		`SELECT COALESCE(control_next, 1) FROM fiscal_series WHERE code = ?`,
		req.Series,
	).Scan(&centralNext)
	if err == nil && centralNext > lastFiscalNum+1 {
		lastFiscalNum = centralNext - 1
	}

	blockStart := lastFiscalNum + 1
	blockEnd := blockStart + int64(blockSize) - 1

	// Insertar el bloque
	result, err := db.Exec(
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

	// Sincronizar la serie fiscal central para que no se solapen futuros bloques.
	// Si falla, el bloque reservado se solaparía con futuros → error fiscal (SENIAT).
	if _, err := db.Exec(
		`UPDATE fiscal_series SET control_next = ? WHERE code = ? AND control_next <= ?`,
		blockEnd+1, req.Series, blockEnd+1,
	); err != nil {
		log.Printf("fiscal: no se pudo sincronizar el contador de la serie %s tras reservar bloque %d: %v", req.Series, blockID, err)
		writeJSON(w, ReserveBlockResponse{Success: false, Message: fmt.Sprintf("fiscal sync: %v", err)})
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

	// Buscar el bloque activo de la caja
	var blockID, blockStart, blockEnd, lastUsed int64
	err = db.QueryRow(
		`SELECT id, block_start, block_end, last_used FROM pos_fiscal_blocks 
		 WHERE terminal_id = ? AND status = 'active' 
		 AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY id DESC LIMIT 1`,
		req.TerminalID,
	).Scan(&blockID, &blockStart, &blockEnd, &lastUsed)

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
		// Marcar bloque como agotado. Si falla, el bloque sigue "activo" y los
		// números se reutilizan → error fiscal (SENIAT).
		if _, err := db.Exec(`UPDATE pos_fiscal_blocks SET status = 'exhausted' WHERE id = ?`, blockID); err != nil {
			log.Printf("fiscal: no se pudo marcar el bloque %d como agotado: %v", blockID, err)
			writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("fiscal close: %v", err)})
			return
		}
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: "block exhausted, request new block"})
		return
	}

	// Registrar el uso del número
	controlPrefix := "00"
	var ctrlPrefix sql.NullString
	_ = db.QueryRow(`SELECT control_prefix FROM fiscal_series WHERE code = 'FACT'`).Scan(&ctrlPrefix)
	if ctrlPrefix.Valid && ctrlPrefix.String != "" {
		controlPrefix = ctrlPrefix.String
	}
	controlNo := fmt.Sprintf("%s-%08d", controlPrefix, nextNum)

	_, err = db.Exec(
		`INSERT INTO pos_fiscal_block_usage (block_id, terminal_id, fiscal_number, sale_local_id, used_at) 
		 VALUES (?, ?, ?, ?, ?)`,
		blockID, req.TerminalID, nextNum, req.SaleLocalID, time.Now(),
	)
	if err != nil {
		writeJSON(w, ConsumeFiscalNumberResponse{Success: false, Message: fmt.Sprintf("recording usage: %v", err)})
		return
	}

	// Actualizar el contador del bloque
	_, _ = db.Exec(
		`UPDATE pos_fiscal_blocks SET used_count = used_count + 1, last_used = ? WHERE id = ?`,
		nextNum, blockID,
	)

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
