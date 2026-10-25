package web

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// Estructuras de datos
// ============================================================

type Branch struct {
	ID        int       `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Phone     string    `json:"phone"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type CashRegister struct {
	ID            int    `json:"id"`
	BranchID      int    `json:"branch_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	PrinterPort   string `json:"printer_port"`
	PrinterBrand  string `json:"printer_brand"`
	PrinterModel  string `json:"printer_model"`
	SerialPort    string `json:"serial_port"`
	BaudRate      int    `json:"baud_rate"`
	FiscalEnabled bool   `json:"fiscal_enabled"`
	Status        string `json:"status"`
}

type APIKey struct {
	ID          int        `json:"id"`
	BranchID    int        `json:"branch_id"`
	BranchName  string     `json:"branch_name,omitempty"`
	APIKey      string     `json:"api_key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	IsActive    bool       `json:"is_active"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ============================================================
// Generador de API Keys
// ============================================================

func generateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "erpgo-" + hex.EncodeToString(b)
}

// ============================================================
// Rutas de administración POS (sucursales, cajas, API keys)
// ============================================================

// registerPOSAdminRoutes registra la INTEGRACIÓN con el POS, no el POS.
//
// AQUÍ SE QUITÓ EL PANEL WEB DEL POS (2026-10-18). Lo que había era: /pos/admin
// pintaba `templates/pos/admin.html`, un archivo que no existe en el proyecto
// —así que la pantalla era un 500 fijo—, y /pos/admin/{terminales,turnos,ventas,
// config} devolvían literalmente «En construcción», más /pos/dashboard, que
// pintaba el panel de caja del propio ERP. El POS es OTRO PROYECTO: se comunica
// con el ERP por API/gRPC, pero el ERP no factura tipo caja registradora ni
// administra sus pantallas. Dejarlas era ofrecer menús que no llevan a ninguna
// parte.
//
// LO QUE **NO** SE TOCA, y es lo que mantiene la comunicación: los endpoints
// JSON de sucursales, cajas y API keys (/admin/pos/*, /pos/api/*) y todo
// /api/pos/* con su autenticación. Sin ellos, un terminal POS nuevo no se podría
// dar de alta ni sincronizar.
func (s *Server) registerPOSAdminRoutes() {
	s.mux.HandleFunc("GET /pos/api/dashboard", s.posAPIDashboard)
	s.mux.HandleFunc("GET /pos/api/terminals", s.posAPITerminals)
	s.mux.HandleFunc("GET /pos/api/turns", s.posAPITurns)
	s.mux.HandleFunc("POST /pos/api/terminal/{id}/logout", s.posAPITerminalLogout)
	s.mux.HandleFunc("POST /pos/api/turn/{id}/close", s.posAPITurnClose)
	s.mux.HandleFunc("POST /pos/api/terminal/create", s.posAPICreateTerminal)

	// Sucursales
	s.mux.HandleFunc("/admin/pos/branches", s.handleBranchList)
	s.mux.HandleFunc("/admin/pos/branch/create", s.handleBranchCreate)
	s.mux.HandleFunc("/admin/pos/branch/edit", s.handleBranchEdit)
	s.mux.HandleFunc("/admin/pos/branch/delete", s.handleBranchDelete)

	// Cajas
	s.mux.HandleFunc("/admin/pos/registers", s.handleRegisterList)
	s.mux.HandleFunc("/admin/pos/register/create", s.handleRegisterCreate)
	s.mux.HandleFunc("/admin/pos/register/edit", s.handleRegisterEdit)
	s.mux.HandleFunc("/admin/pos/register/delete", s.handleRegisterDelete)

	// API Keys
	s.mux.HandleFunc("/admin/pos/apikeys", s.handleAPIKeyList)
	s.mux.HandleFunc("/admin/pos/apikey/create", s.handleAPIKeyCreate)
	s.mux.HandleFunc("/admin/pos/apikey/regenerate", s.handleAPIKeyRegenerate)
	s.mux.HandleFunc("/admin/pos/apikey/revoke", s.handleAPIKeyRevoke)
	s.mux.HandleFunc("/admin/pos/apikey/delete", s.handleAPIKeyDelete)

	// Dashboard POS
	s.mux.HandleFunc("/admin/pos/dashboard", s.handlePOSDashboard)
}

// ============================================================
// Handler: Dashboard POS Admin
// ============================================================

// posDashboardSincronizacion devuelve el estado de sincronización POS que antes
// pintaba el panel retirado. Se conserva como JSON porque es el dato que un
// administrador quiere seguir viendo (¿están sincronizando los terminales?) sin
// devolverle al ERP la pantalla de caja.
func (s *Server) posDashboardSincronizacion(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)

	var stats struct {
		ActiveTerminals int
		OpenTurns       int
		TodaySales      float64
		TodayTurns      int
	}

	db.QueryRow("SELECT COUNT(*) FROM pos_terminal_sessions WHERE status='active'").Scan(&stats.ActiveTerminals)
	db.QueryRow("SELECT COUNT(*) FROM pos_turns WHERE status='open'").Scan(&stats.OpenTurns)
	db.QueryRow("SELECT COALESCE(SUM(total_sales),0) FROM pos_turns WHERE DATE(opened_at)=CURDATE()").Scan(&stats.TodaySales)
	db.QueryRow("SELECT COUNT(*) FROM pos_turns WHERE DATE(opened_at)=CURDATE()").Scan(&stats.TodayTurns)

	// Terminales activas
	type Terminal struct {
		TerminalID  string
		Status      string
		LastSeen    string
		IPAddress   string
	}
	var terminals []Terminal
	rows, _ := db.Query("SELECT terminal_id, status, COALESCE(last_seen, ''), COALESCE(ip_address, '') FROM pos_terminal_sessions WHERE status='active' ORDER BY last_seen DESC LIMIT 20")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var t Terminal
			if err := rows.Scan(&t.TerminalID, &t.Status, &t.LastSeen, &t.IPAddress); err == nil {
				terminals = append(terminals, t)
			}
		}
	}

	// Turnos abiertos
	type Turn struct {
		TurnID      int
		TerminalID  string
		OpenedAt    string
		OpenedBy    string
		TotalSales  float64
	}
	var turns []Turn
	// pos_turns no tiene columna `opened_by`: el que abre el turno es
	// t.user_id (numérico) y t.user_name (texto). El JOIN va por user_id y el
	// respaldo de la consulta es user_name, no una columna inexistente.
	rows2, _ := db.Query(`SELECT t.id, t.terminal_id, t.opened_at, COALESCE(u.first_name, t.user_name), t.total_sales 
		FROM pos_turns t LEFT JOIN users u ON u.user_id = t.user_id 
		WHERE t.status='open' ORDER BY t.opened_at DESC`)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var t Turn
			if err := rows2.Scan(&t.TurnID, &t.TerminalID, &t.OpenedAt, &t.OpenedBy, &t.TotalSales); err == nil {
				turns = append(turns, t)
			}
		}
	}

	data := map[string]interface{}{
		"Stats":     stats,
		"Terminals": terminals,
		"Turns":     turns,
	}

	writeJSON(w, data)
}

func (s *Server) posAPIDashboard(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	var stats struct {
		TotalBranches  int
		TotalRegisters int
		ActiveSessions int
		OpenTurns      int
		PendingOffline int
	}
	db.QueryRow("SELECT COUNT(*) FROM branches WHERE is_active=1").Scan(&stats.TotalBranches)
	db.QueryRow("SELECT COUNT(*) FROM cash_registers WHERE status='active'").Scan(&stats.TotalRegisters)
	db.QueryRow("SELECT COUNT(*) FROM pos_terminal_sessions WHERE status='active'").Scan(&stats.ActiveSessions)
	db.QueryRow("SELECT COUNT(*) FROM pos_turns WHERE status='open'").Scan(&stats.OpenTurns)
	db.QueryRow("SELECT COUNT(*) FROM pos_offline_queue WHERE status='pending'").Scan(&stats.PendingOffline)
	writeJSON(w, stats)
}

func (s *Server) posAPITerminals(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	rows, err := db.Query("SELECT id, terminal_id, status, last_activity FROM pos_terminal_sessions WHERE status='active'")
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var terminals []map[string]interface{}
	for rows.Next() {
		var id int
		var terminalID, status string
		var lastActivity time.Time
		rows.Scan(&id, &terminalID, &status, &lastActivity)
		terminals = append(terminals, map[string]interface{}{
			"id": id, "terminal_id": terminalID, "status": status, "last_activity": lastActivity,
		})
	}
	writeJSON(w, terminals)
}

func (s *Server) posAPITurns(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	rows, err := db.Query("SELECT id, terminal_id, user_name, opened_at, status, total_sales FROM pos_turns WHERE status='open'")
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var turns []map[string]interface{}
	for rows.Next() {
		var id int
		var terminalID, userName, status string
		var openedAt time.Time
		var totalSales float64
		rows.Scan(&id, &terminalID, &userName, &openedAt, &status, &totalSales)
		turns = append(turns, map[string]interface{}{
			"id": id, "terminal_id": terminalID, "user_name": userName,
			"opened_at": openedAt, "status": status, "total_sales": totalSales,
		})
	}
	writeJSON(w, turns)
}

func (s *Server) posAPITerminalLogout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "logged_out"})
}

func (s *Server) posAPITurnClose(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "closed"})
}

func (s *Server) posAPICreateTerminal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TerminalID string `json:"terminal_id"`
		Name       string `json:"name"`
		BranchID   int    `json:"branch_id"`
		RegisterID int    `json:"register_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid data", http.StatusBadRequest)
		return
	}
	apiKey := generateAPIKey()
	writeJSON(w, map[string]interface{}{
		"status": "created", "terminal_id": req.TerminalID, "api_key": apiKey,
	})
}

// ============================================================
// Handler: Lista de sucursales
// ============================================================

func (s *Server) handleBranchList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	db := s.dbFor(r)
	rows, err := db.Query(`
		SELECT b.id, b.code, b.name, b.address, b.phone, b.is_active, b.created_at,
		       COUNT(DISTINCT cr.id) as register_count,
		       COUNT(DISTINCT ak.id) as apikey_count
		FROM branches b
		LEFT JOIN cash_registers cr ON cr.branch_id = b.id
		LEFT JOIN api_keys ak ON ak.branch_id = b.id
		GROUP BY b.id
		ORDER BY b.name`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var branches []map[string]interface{}
	for rows.Next() {
		var b Branch
		var addr, phone sql.NullString
		var regCount, keyCount int
		err := rows.Scan(&b.ID, &b.Code, &b.Name, &addr, &phone, &b.IsActive, &b.CreatedAt, &regCount, &keyCount)
		if err != nil {
			continue
		}
		branches = append(branches, map[string]interface{}{
			"id":             b.ID,
			"code":           b.Code,
			"name":           b.Name,
			"address":        addr.String,
			"phone":          phone.String,
			"is_active":      b.IsActive,
			"created_at":     b.CreatedAt,
			"register_count": regCount,
			"apikey_count":   keyCount,
		})
	}
	writeJSON(w, branches)
}

// ============================================================
// Handler: Crear sucursal
// ============================================================

func (s *Server) handleBranchCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Code    string `json:"code"`
		Name    string `json:"name"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid data", http.StatusBadRequest)
		return
	}

	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)

	if req.Code == "" || req.Name == "" {
		jsonError(w, "code and name required", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	_, err := db.Exec(`
		INSERT INTO branches (code, name, address, phone) VALUES (?, ?, ?, ?)`,
		req.Code, req.Name, req.Address, req.Phone)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			jsonError(w, "branch code already exists", http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Sucursal creada: %s - %s", req.Code, req.Name)
	writeJSON(w, map[string]interface{}{"status": "created", "code": req.Code})
}

// ============================================================
// Handler: Editar sucursal
// ============================================================

func (s *Server) handleBranchEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Address  string `json:"address"`
		Phone    string `json:"phone"`
		IsActive *bool  `json:"is_active,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid data", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	_, err := db.Exec(`
		UPDATE branches SET name=?, address=?, phone=?, is_active=? WHERE id=?`,
		req.Name, req.Address, req.Phone, req.IsActive != nil && *req.IsActive, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"status": "updated"})
}

// ============================================================
// Handler: Eliminar sucursal
// ============================================================

func (s *Server) handleBranchDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	_, err := db.Exec("DELETE FROM branches WHERE id=?", id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"status": "deleted"})
}

// ============================================================
// Handler: Lista de cajas por sucursal
// ============================================================

func (s *Server) handleRegisterList(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	branchID, _ := strconv.Atoi(r.URL.Query().Get("branch_id"))

	query := `
		SELECT cr.id, cr.branch_id, cr.code, cr.name, cr.description,
		       cr.printer_port, cr.printer_brand, cr.printer_model,
		       cr.serial_port, cr.baud_rate, cr.fiscal_enabled, cr.status,
		       b.name as branch_name
		FROM cash_registers cr
		JOIN branches b ON b.id = cr.branch_id`
	args := []interface{}{}
	if branchID > 0 {
		query += " WHERE cr.branch_id = ?"
		args = append(args, branchID)
	}
	query += " ORDER BY b.name, cr.code"

	rows, err := db.Query(query, args...)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var registers []map[string]interface{}
	for rows.Next() {
		var cr CashRegister
		var desc, printerPort, printerBrand, printerModel, serialPort, status, branchName string
		var baudRate int
		err := rows.Scan(&cr.ID, &cr.BranchID, &cr.Code, &cr.Name, &desc,
			&printerPort, &printerBrand, &printerModel, &serialPort, &baudRate,
			&cr.FiscalEnabled, &status, &branchName)
		if err != nil {
			continue
		}
		registers = append(registers, map[string]interface{}{
			"id":            cr.ID,
			"branch_id":     cr.BranchID,
			"branch_name":   branchName,
			"code":          cr.Code,
			"name":          cr.Name,
			"description":   desc,
			"printer_port":  printerPort,
			"printer_brand": printerBrand,
			"printer_model": printerModel,
			"serial_port":   serialPort,
			"baud_rate":     baudRate,
			"fiscal_enabled": cr.FiscalEnabled,
			"status":        status,
		})
	}
	writeJSON(w, registers)
}

// ============================================================
// Handler: Crear caja
// ============================================================

func (s *Server) handleRegisterCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BranchID      int    `json:"branch_id"`
		Code          string `json:"code"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		PrinterPort   string `json:"printer_port"`
		PrinterBrand  string `json:"printer_brand"`
		PrinterModel  string `json:"printer_model"`
		SerialPort    string `json:"serial_port"`
		BaudRate      int    `json:"baud_rate"`
		FiscalEnabled bool   `json:"fiscal_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid data", http.StatusBadRequest)
		return
	}

	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" || req.BranchID == 0 {
		jsonError(w, "branch_id, code and name required", http.StatusBadRequest)
		return
	}

	if req.PrinterPort == "" {
		req.PrinterPort = "COM1"
	}
	if req.BaudRate == 0 {
		req.BaudRate = 9600
	}

	db := s.dbFor(r)
	_, err := db.Exec(`
		INSERT INTO cash_registers 
		(branch_id, code, name, description, printer_port, printer_brand, printer_model, serial_port, baud_rate, fiscal_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.BranchID, req.Code, req.Name, req.Description,
		req.PrinterPort, req.PrinterBrand, req.PrinterModel, req.SerialPort, req.BaudRate, req.FiscalEnabled)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			jsonError(w, "register code already exists in this branch", http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Caja creada: %s en sucursal %d", req.Code, req.BranchID)
	writeJSON(w, map[string]interface{}{"status": "created", "code": req.Code})
}

// ============================================================
// Handler: Editar caja
// ============================================================

func (s *Server) handleRegisterEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		PrinterPort   string `json:"printer_port"`
		PrinterBrand  string `json:"printer_brand"`
		PrinterModel  string `json:"printer_model"`
		SerialPort    string `json:"serial_port"`
		BaudRate      int    `json:"baud_rate"`
		FiscalEnabled *bool  `json:"fiscal_enabled,omitempty"`
		Status        string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid data", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	fiscal := req.FiscalEnabled != nil && *req.FiscalEnabled
	_, err := db.Exec(`
		UPDATE cash_registers 
		SET name=?, description=?, printer_port=?, printer_brand=?, printer_model=?,
		    serial_port=?, baud_rate=?, fiscal_enabled=?, status=?
		WHERE id=?`,
		req.Name, req.Description, req.PrinterPort, req.PrinterBrand, req.PrinterModel,
		req.SerialPort, req.BaudRate, fiscal, req.Status, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"status": "updated"})
}

// ============================================================
// Handler: Eliminar caja
// ============================================================

func (s *Server) handleRegisterDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	_, err := db.Exec("DELETE FROM cash_registers WHERE id=?", id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"status": "deleted"})
}

// ============================================================
// Handler: Lista de API Keys
// ============================================================

func (s *Server) handleAPIKeyList(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	rows, err := db.Query(`
		SELECT ak.id, ak.branch_id, b.name as branch_name, ak.api_key, ak.name, 
		       ak.description, ak.is_active, ak.last_used_at, ak.created_at
		FROM api_keys ak
		JOIN branches b ON b.id = ak.branch_id
		ORDER BY b.name, ak.name`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var keys []map[string]interface{}
	for rows.Next() {
		var ak APIKey
		var desc sql.NullString
		var lastUsed sql.NullTime
		err := rows.Scan(&ak.ID, &ak.BranchID, &ak.BranchName, &ak.APIKey, &ak.Name,
			&desc, &ak.IsActive, &lastUsed, &ak.CreatedAt)
		if err != nil {
			continue
		}
		keys = append(keys, map[string]interface{}{
			"id":          ak.ID,
			"branch_id":   ak.BranchID,
			"branch_name": ak.BranchName,
			"api_key":     ak.APIKey,
			"name":        ak.Name,
			"description": desc.String,
			"is_active":   ak.IsActive,
			"last_used_at": lastUsed.Time,
			"created_at":  ak.CreatedAt,
		})
	}
	writeJSON(w, keys)
}

// ============================================================
// Handler: Crear API Key
// ============================================================

func (s *Server) handleAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BranchID    int    `json:"branch_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		APIKey      string `json:"api_key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid data", http.StatusBadRequest)
		return
	}

	if req.BranchID == 0 || req.Name == "" {
		jsonError(w, "branch_id and name required", http.StatusBadRequest)
		return
	}

	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = generateAPIKey()
	}

	db := s.dbFor(r)
	_, err := db.Exec(`
		INSERT INTO api_keys (branch_id, api_key, name, description) VALUES (?, ?, ?, ?)`,
		req.BranchID, apiKey, req.Name, req.Description)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			jsonError(w, "api key already exists", http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("API Key creada para sucursal %d: %s", req.BranchID, apiKey)
	writeJSON(w, map[string]interface{}{"status": "created", "api_key": apiKey})
}

// ============================================================
// Handler: Regenerar API Key
// ============================================================

func (s *Server) handleAPIKeyRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	newKey := generateAPIKey()
	db := s.dbFor(r)
	_, err := db.Exec("UPDATE api_keys SET api_key=? WHERE id=?", newKey, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// La key vieja deja de valer ya: purgar su caché de resolución.
	// (No sabemos el valor viejo aquí, así que limpiamos toda la caché.)
	InvalidatePOSKeyCache("")

	writeJSON(w, map[string]interface{}{"status": "regenerated", "api_key": newKey})
}

// ============================================================
// Handler: Revocar API Key (soft: is_active=0, sin borrar la fila)
// ============================================================
//
// Revocar es distinto de borrar: la key queda registrada (para auditoría y
// para saber qué pasó) pero deja de autenticar. La caja que la usa empieza a
// recibir 401 en el siguiente intento. La caché de resolución se purga aquí
// para que el efecto sea inmediato, no al TTL de un minuto.
func (s *Server) handleAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	// Permitir revocar (0) o reactivar (1).
	active := 0
	if v := r.URL.Query().Get("active"); v == "1" || v == "true" {
		active = 1
	}

	db := s.dbFor(r)
	var key string
	_ = db.QueryRow("SELECT api_key FROM api_keys WHERE id=?", id).Scan(&key)

	res, err := db.Exec("UPDATE api_keys SET is_active=? WHERE id=?", active, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "api key not found", http.StatusNotFound)
		return
	}

	// Efecto inmediato: la key revocada ya no resuelve empresa.
	InvalidatePOSKeyCache(key)

	status := "revoked"
	if active == 1 {
		status = "restored"
	}
	log.Printf("API Key %s (id=%d): %s", status, id, key)
	writeJSON(w, map[string]interface{}{"status": status, "id": id})
}

// ============================================================
// Handler: Eliminar API Key
// ============================================================

func (s *Server) handleAPIKeyDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		jsonError(w, "id required", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	var key string
	_ = db.QueryRow("SELECT api_key FROM api_keys WHERE id=?", id).Scan(&key)
	_, err := db.Exec("DELETE FROM api_keys WHERE id=?", id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Borrada: purgar su caché de resolución para que deje de valer ya.
	InvalidatePOSKeyCache(key)

	writeJSON(w, map[string]interface{}{"status": "deleted"})
}

// ============================================================
// Handler: Dashboard POS
// ============================================================

func (s *Server) handlePOSDashboard(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)

	var stats struct {
		TotalBranches   int
		TotalRegisters  int
		ActiveSessions  int
		OpenTurns       int
		PendingOffline  int
		TotalSalesToday float64
	}

	db.QueryRow("SELECT COUNT(*) FROM branches WHERE is_active=1").Scan(&stats.TotalBranches)
	db.QueryRow("SELECT COUNT(*) FROM cash_registers WHERE status='active'").Scan(&stats.TotalRegisters)
	db.QueryRow("SELECT COUNT(*) FROM pos_terminal_sessions WHERE status='active'").Scan(&stats.ActiveSessions)
	db.QueryRow("SELECT COUNT(*) FROM pos_turns WHERE status='open'").Scan(&stats.OpenTurns)
	db.QueryRow("SELECT COUNT(*) FROM pos_offline_queue WHERE status='pending'").Scan(&stats.PendingOffline)
	db.QueryRow("SELECT COALESCE(SUM(total_sales),0) FROM pos_turns WHERE DATE(opened_at)=CURDATE()").Scan(&stats.TotalSalesToday)

	writeJSON(w, map[string]interface{}{
		"stats":     stats,
		"branches":  "/admin/pos/branches",
		"registers": "/admin/pos/registers",
		"apikeys":   "/admin/pos/apikeys",
	})
}

// ============================================================
// Helper: Validar API Key y obtener sucursal/caja
// ============================================================

func validatePOSAPIKey(db *sql.DB, apiKey string) (branchID int, registerID int, err error) {
	if apiKey == "" {
		return 0, 0, fmt.Errorf("api key required")
	}

	err = db.QueryRow(`
		SELECT ak.branch_id, cr.id
		FROM api_keys ak
		JOIN cash_registers cr ON cr.branch_id = ak.branch_id
		WHERE ak.api_key = ? AND ak.is_active = 1
		LIMIT 1`, apiKey).Scan(&branchID, &registerID)

	if err == sql.ErrNoRows {
		return 0, 0, fmt.Errorf("invalid api key")
	}
	if err != nil {
		return 0, 0, err
	}
	return branchID, registerID, nil
}
