package web

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// VerifyPOSCredentials comprueba las credenciales de un cajero contra la base de
// SU empresa y devuelve su id y su nivel.
//
// Existe porque el servidor gRPC del POS necesita autenticar usuario y
// contraseña (Login) sin reimplementar la comprobación: los formatos de
// contraseña guardada (bcrypt, md5 heredado de PHP, texto plano heredado) y su
// reescritura a bcrypt viven en verificarPassword, y una segunda copia de esa
// regla es la copia que se queda atrás.
//
// Las reglas son las MISMAS que el login POS por HTTP: usuario activo del
// catálogo de la empresa, contraseña válida y pertenencia al departamento de
// punto de venta (4). Si el POS por HTTP lo exige, el POS por gRPC no puede
// exigir menos: sería entrar por la puerta de atrás a la misma caja.
func (s *Server) VerifyPOSCredentials(db *sql.DB, username, password string) (int, string, error) {
	username = strings.TrimSpace(username)
	if db == nil || username == "" || password == "" {
		return 0, "", fmt.Errorf("credenciales incompletas")
	}

	var (
		userID       int
		passwordHash string
	)
	err := db.QueryRow(`
		SELECT u.user_id, ul.password
		FROM user_login ul
		JOIN users u ON u.user_id = ul.user_id
		WHERE ul.username = ? AND ul.status = 1 AND u.status = 1 LIMIT 1`, username).
		Scan(&userID, &passwordHash)
	if err == sql.ErrNoRows {
		return 0, "", fmt.Errorf("usuario no encontrado")
	}
	if err != nil {
		return 0, "", err
	}

	valid, necesitaRehash := verificarPassword(passwordHash, password)
	if !valid {
		return 0, "", fmt.Errorf("contraseña incorrecta")
	}
	if necesitaRehash {
		// Única ocasión en la que se tiene la contraseña en claro: se aprovecha
		// para subir la fila heredada a bcrypt.
		if err := reescribirPassword(db, userID, username, password); err != nil {
			log.Printf("AVISO: no se pudo actualizar la contraseña POS de %s a bcrypt: %v", username, err)
		}
	}

	var deptOK bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_department WHERE user_id = ? AND department_id = 4)`, userID).Scan(&deptOK); err != nil {
		return 0, "", err
	}
	if !deptOK {
		return 0, "", fmt.Errorf("sin acceso a punto de venta")
	}

	level := "employee"
	_ = db.QueryRow(`SELECT COALESCE(r.level,'employee') FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid WHERE ur.user_id = ? LIMIT 1`, userID).Scan(&level)
	if strings.TrimSpace(level) == "" {
		level = "employee"
	}
	return userID, level, nil
}

func (s *Server) handlePOSLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		TerminalID string `json:"terminal_id"`
		APIKey     string `json:"api_key"`
		Tenant     string `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	// Fuerza bruta: se limita por IP de la caja y por usuario. Un atacante
	// probando contraseñas sobre el endpoint POS se bloquea a los pocos
	// intentos, igual que el login del panel web (mismo loginLimiter).
	ipKey := "pos:ip:" + clientIP(r)
	userKey := "pos:user:" + strings.ToLower(strings.TrimSpace(req.Username))
	if loginLimiter.checkRateLimit(ipKey) || loginLimiter.checkRateLimit(userKey) {
		jsonError(w, "Demasiados intentos. Espere unos minutos.", http.StatusTooManyRequests)
		return
	}

	// La empresa del terminal viene en X-POS-Tenant (peticiones posteriores) o
	// en el campo "tenant" del body (este login, que manda el binario POS).
	// Usuarios, api_keys, sesiones POS y roles viven en la base de ESA empresa.
	tenant := s.posTenantCode(r)
	if req.Tenant != "" {
		tenant = slugifyTenant(req.Tenant)
	}
	if headerTenant := strings.TrimSpace(r.Header.Get("X-POS-Tenant")); headerTenant != "" && slugifyTenant(headerTenant) != tenant {
		jsonError(w, "La empresa declarada no coincide con la empresa de la petición", http.StatusForbidden)
		return
	}
	controlDB := s.dbForTenant(tenant)
	if controlDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	var err error

	var branchID int
	var branchName sql.NullString
	err = controlDB.QueryRow(`
		SELECT b.id, b.name FROM api_keys ak
		JOIN branches b ON b.id = ak.branch_id
		WHERE ak.api_key = ? AND ak.is_active = 1 LIMIT 1`, req.APIKey).Scan(&branchID, &branchName)
	if err != nil {
		jsonError(w, "API key inválida", http.StatusUnauthorized)
		return
	}
	var currentBranch int
	if err := controlDB.QueryRow(`SELECT branch_id FROM api_keys WHERE api_key = ? AND is_active = 1 LIMIT 1`, req.APIKey).Scan(&currentBranch); err != nil || currentBranch != branchID {
		jsonError(w, "API key inválida", http.StatusUnauthorized)
		return
	}

	if strings.TrimSpace(req.TerminalID) == "" {
		jsonError(w, "terminal_id requerido", http.StatusBadRequest)
		return
	}
	req.TerminalID = strings.TrimSpace(req.TerminalID)

	var userID int
	var username string
	var passwordHash string
	var firstName, lastName sql.NullString
	err = controlDB.QueryRow(`
		SELECT u.user_id, ul.username, ul.password, u.first_name, u.last_name
		FROM user_login ul
		JOIN users u ON u.user_id = ul.user_id
		WHERE ul.username = ? AND ul.status = 1 AND u.status = 1 LIMIT 1`, req.Username).Scan(&userID, &username, &passwordHash, &firstName, &lastName)
	if err != nil {
		jsonError(w, "Credenciales inválidas", http.StatusUnauthorized)
		return
	}

	valid, necesitaRehash := verificarPassword(passwordHash, req.Password)
	if !valid {
		loginLimiter.recordFailedAttempt(ipKey)
		loginLimiter.recordFailedAttempt(userKey)
		jsonError(w, "Credenciales inválidas", http.StatusUnauthorized)
		return
	}
	if necesitaRehash {
		if err := reescribirPassword(controlDB, userID, username, req.Password); err != nil {
			log.Printf("AVISO: no se pudo actualizar la contraseña POS de %s a bcrypt: %v", username, err)
		}
	}

	// Credenciales correctas: se limpia el contador de intentos fallidos.
	loginLimiter.resetAttempts(ipKey)
	loginLimiter.resetAttempts(userKey)

	var deptOK bool
	if err := controlDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_department WHERE user_id = ? AND department_id = 4)`, userID).Scan(&deptOK); err != nil {
		jsonError(w, "No se pudo comprobar el acceso al punto de venta", http.StatusInternalServerError)
		return
	}
	if !deptOK {
		jsonError(w, "Sin acceso a punto de venta", http.StatusForbidden)
		return
	}

	token := generatePOSToken()
	if token == "" {
		jsonError(w, "No se pudo generar una sesión segura", http.StatusInternalServerError)
		return
	}
	expiry := time.Now().Add(12 * time.Hour)

	res, err := controlDB.Exec(`
		INSERT INTO pos_sessions (token, user_id, terminal_id, branch_id, login_time, expires_at, is_active, revoked, created_at)
		SELECT ?, ?, ?, ?, NOW(), ?, 1, 0, NOW()
		FROM DUAL
		WHERE EXISTS (SELECT 1 FROM api_keys WHERE api_key = ? AND branch_id = ? AND is_active = 1)`,
		token, userID, req.TerminalID, branchID, expiry.Format("2006-01-02 15:04:05"), req.APIKey, branchID)
	if err != nil {
		jsonError(w, "Error creando sesión: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if res == nil {
		jsonError(w, "No se pudo crear la sesión POS", http.StatusInternalServerError)
		return
	}
	if affected, rowsErr := res.RowsAffected(); rowsErr != nil || affected != 1 {
		jsonError(w, "API key revocada durante el inicio de sesión", http.StatusUnauthorized)
		return
	}

	_, _ = controlDB.Exec(`
		INSERT INTO pos_turn_events (terminal_id, branch_id, user_id, event_type, event_time, details)
		VALUES (?, ?, ?, 'login', NOW(), ?)`,
		req.TerminalID, branchID, userID, "Login POS: "+username)

	var level string
	_ = controlDB.QueryRow(`SELECT COALESCE(r.level,'employee') FROM sec_userrole ur JOIN sec_role r ON r.id = ur.roleid WHERE ur.user_id = ? LIMIT 1`, userID).Scan(&level)

	// fullname es el nombre que la caja muestra en cabecera. Se calcula aquí y
	// se manda además de first_name/last_name para que el POS no tenga que
	// recomponerlo (y para no romper a quien ya lee las partes).
	fullname := strings.TrimSpace(strings.TrimSpace(firstName.String) + " " + strings.TrimSpace(lastName.String))
	if fullname == "" {
		fullname = username
	}

	writeJSON(w, map[string]interface{}{
		"token":       token,
		"expires_at":  expiry.Format(time.RFC3339),
		"user_id":     userID,
		"username":    username,
		"fullname":    fullname,
		"first_name":  firstName.String,
		"last_name":   lastName.String,
		"level":       level,
		"branch_id":   branchID,
		"branch_name": branchName.String,
		"tenant":      tenant,
	})
}

func (s *Server) handlePOSValidate(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-POS-Token")
	if token == "" {
		jsonError(w, "Token requerido en X-POS-Token", http.StatusUnauthorized)
		return
	}

	// La empresa del terminal viene en X-POS-Tenant: usuarios, api_keys,
	// sesiones POS y roles viven en la base de ESA empresa, no en la de control.
	controlDB := s.posDBFor(r)
	if controlDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	var err error

	var userID string
	var terminalID sql.NullString
	var branchID int
	var expiresAt time.Time

	err = controlDB.QueryRow(`
		SELECT user_id, terminal_id, branch_id, expires_at FROM pos_sessions
		WHERE token = ? AND expires_at > NOW() AND is_active = 1 AND IFNULL(revoked, 0) = 0
		LIMIT 1`, token).Scan(&userID, &terminalID, &branchID, &expiresAt)
	if err != nil {
		jsonError(w, "Token inválido o expirado", http.StatusUnauthorized)
		return
	}

	var firstName, lastName, level sql.NullString
	var active bool
	if err := controlDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE user_id = ? AND status = 1)`, userID).Scan(&active); err != nil {
		jsonError(w, "No se pudo comprobar el usuario POS", http.StatusInternalServerError)
		return
	}
	if !active {
		jsonError(w, "Usuario POS inactivo", http.StatusUnauthorized)
		return
	}
	_ = controlDB.QueryRow(`
		SELECT u.first_name, u.last_name, r.level
		FROM users u
		LEFT JOIN sec_userrole ur ON ur.user_id = u.user_id
		LEFT JOIN sec_role r ON r.id = ur.roleid
		WHERE u.user_id = ? LIMIT 1`, userID).Scan(&firstName, &lastName, &level)

	writeJSON(w, map[string]interface{}{
		"valid":       true,
		"user_id":     userID,
		"first_name":  firstName.String,
		"last_name":   lastName.String,
		"level":       level.String,
		"terminal_id": terminalID.String,
		"branch_id":   branchID,
		"expires_at":  expiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handlePOSLogout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-POS-Token")
	if token == "" {
		jsonError(w, "Token requerido", http.StatusUnauthorized)
		return
	}

	// La empresa del terminal viene en X-POS-Tenant: usuarios, api_keys,
	// sesiones POS y roles viven en la base de ESA empresa, no en la de control.
	controlDB := s.posDBFor(r)
	if controlDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	var err error

	var userID, terminalID sql.NullString
	err = controlDB.QueryRow(`
		SELECT user_id, terminal_id FROM pos_sessions WHERE token = ? AND is_active = 1 AND IFNULL(revoked, 0) = 0`,
		token).Scan(&userID, &terminalID)

	if err == nil {
		if _, err := controlDB.Exec(`UPDATE pos_sessions SET is_active = 0, revoked = 1, logout_time = NOW() WHERE token = ?`, token); err != nil {
			jsonError(w, "No se pudo cerrar la sesión POS", http.StatusInternalServerError)
			return
		}
		_, _ = controlDB.Exec(`
			INSERT INTO pos_turn_events (terminal_id, user_id, event_type, event_time, details)
			VALUES (?, ?, 'logout', NOW(), 'Logout POS')`,
			terminalID.String, userID.String)
	}

	writeJSON(w, map[string]interface{}{"status": "logged_out"})
}

// handlePOSAutorizaSupervisor requiere barcode + contraseña (doble factor).
// Punto de seguridad: no basta con escanear el carnet, se necesita la clave.
func (s *Server) handlePOSAutorizaSupervisor(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Barcode  string `json:"barcode"`
		Password string `json:"password"`
		Reason   string `json:"reason"`
		AuthType string `json:"auth_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	// Fuerza bruta sobre la autorización de supervisor: limita por IP. Se usa
	// para destrabar devoluciones, así que adivinar la contraseña del supervisor
	// es un vector real. Se limita con el mismo loginLimiter que el login.
	supIPKey := "pos:sup:ip:" + clientIP(r)
	if loginLimiter.checkRateLimit(supIPKey) {
		jsonError(w, "Demasiados intentos. Espere unos minutos.", http.StatusTooManyRequests)
		return
	}

	// Validar que vengan ambos factores
	if req.Barcode == "" {
		jsonError(w, "Código de barra del supervisor requerido", http.StatusUnauthorized)
		return
	}
	if req.Password == "" {
		jsonError(w, "Contraseña del supervisor requerida", http.StatusUnauthorized)
		return
	}

	// La empresa del terminal viene en X-POS-Tenant: usuarios, api_keys,
	// sesiones POS y roles viven en la base de ESA empresa, no en la de control.
	controlDB := s.posDBFor(r)
	if controlDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	var err error

	// Buscar supervisor por barcode/employee_code y verificar que tenga rol master/manager
	var userID int
	var passwordHash string
	var supervisorName string
	err = controlDB.QueryRow(`
		SELECT u.user_id, ul.password, CONCAT(u.first_name, ' ', u.last_name)
		FROM users u
		JOIN user_login ul ON ul.user_id = u.user_id
		JOIN sec_userrole ur ON ur.user_id = u.user_id
		JOIN sec_role r ON r.id = ur.roleid
		WHERE (u.employee_code = ? OR u.barcode = ?)
		AND r.level IN ('master', 'manager')
		AND u.status = 1
		LIMIT 1`, req.Barcode, req.Barcode).Scan(&userID, &passwordHash, &supervisorName)
	if err != nil {
		loginLimiter.recordFailedAttempt(supIPKey)
		jsonError(w, "Código de supervisor no válido", http.StatusUnauthorized)
		return
	}

	// Verificar contraseña
	valid, _ := verificarPassword(passwordHash, req.Password)
	if !valid {
		loginLimiter.recordFailedAttempt(supIPKey)
		jsonError(w, "Contraseña de supervisor incorrecta", http.StatusUnauthorized)
		return
	}

	// Autorización correcta: limpiar el contador de intentos fallidos.
	loginLimiter.resetAttempts(supIPKey)

	// Registrar autorización
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = req.AuthType
	}

	_, _ = controlDB.Exec(`
		INSERT INTO pos_supervisor_auth (session_id, supervisor_id, auth_type, auth_time, reason, barcode)
		VALUES (0, ?, ?, NOW(), ?, ?)`,
		userID, req.AuthType, reason, req.Barcode)

	writeJSON(w, map[string]interface{}{
		"authorized":      true,
		"supervisor_id":   userID,
		"supervisor_name": supervisorName,
	})
}

func generatePOSToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return "pos_" + hex.EncodeToString(b)
}
