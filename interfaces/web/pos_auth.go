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

// VerifyPOSCredentials comprueba las credenciales de un cajero y devuelve su
// id y su nivel.
//
// Existe porque el servidor gRPC del POS necesita autenticar usuario y
// contraseña (Login) sin reimplementar la comprobación: los formatos de
// contraseña guardada (bcrypt, md5 heredado de PHP, texto plano heredado) y su
// reescritura a bcrypt viven en verificarPassword, y una segunda copia de esa
// regla es la copia que se queda atrás.
//
// S-2 (auditoría 2026-11-22): la IDENTIDAD se lee en la base de control —la
// misma del panel y del login POS por HTTP—; tenantDB (la empresa de la API
// key, que llega del gRPC) aporta el departamento de punto de venta. Las
// reglas son idénticas a handlePOSLogin: si un camino exige menos que el
// otro, la caja se abre por la puerta que quedó floja.
func (s *Server) VerifyPOSCredentials(tenantDB *sql.DB, username, password string) (int, string, error) {
	username = strings.TrimSpace(username)
	if tenantDB == nil || username == "" || password == "" {
		return 0, "", fmt.Errorf("credenciales incompletas")
	}
	ctl, err := s.controlStore()
	if err != nil {
		return 0, "", fmt.Errorf("servicio de identidad no disponible: %w", err)
	}

	var (
		userID       int
		passwordHash string
	)
	err = ctl.QueryRow(`
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
		// para subir la fila heredada a bcrypt —en CONTROL, la fila que se
		// lee arriba—.
		if err := reescribirPassword(ctl, userID, username, password); err != nil {
			log.Printf("AVISO: no se pudo actualizar la contraseña POS de %s a bcrypt: %v", username, err)
		}
	}

	var deptOK bool
	if err := tenantDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_department WHERE user_id = ? AND department_id = 4)`, userID).Scan(&deptOK); err != nil {
		return 0, "", err
	}
	if !deptOK {
		return 0, "", fmt.Errorf("sin acceso a punto de venta")
	}

	return userID, s.nivelParaPOS(ctl, userID), nil
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
	//
	// S-2 (auditoría 2026-11-22): LA IDENTIDAD VIVE EN LA BASE DE CONTROL, la
	// misma que lee el login del panel. Antes el POS leía la COPIA de
	// user_login de la empresa, y el panel (alta, cambio de clave, baja)
	// escribe la de control: despedir a un cajero o cambiarle la clave no le
	// cerraba la caja, y un empleado creado después de aprovisionar la
	// empresa no tenía fila en la copia. api_keys, departamentos de punto de
	// venta y sesiones POS siguen siendo por empresa —lo que se resuelve por
	// empresa es el ACCESO, no quién eres—.
	tenant := s.posTenantCode(r)
	if req.Tenant != "" {
		tenant = slugifyTenant(req.Tenant)
	}
	if headerTenant := strings.TrimSpace(r.Header.Get("X-POS-Tenant")); headerTenant != "" && slugifyTenant(headerTenant) != tenant {
		jsonError(w, "La empresa declarada no coincide con la empresa de la petición", http.StatusForbidden)
		return
	}
	tenantDB := s.dbForTenant(tenant)
	if tenantDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "Servicio de identidad no disponible", http.StatusServiceUnavailable)
		return
	}

	var branchID int
	var branchName sql.NullString
	err = tenantDB.QueryRow(`
		SELECT b.id, b.name FROM api_keys ak
		JOIN branches b ON b.id = ak.branch_id
		WHERE ak.api_key = ? AND ak.is_active = 1 LIMIT 1`, req.APIKey).Scan(&branchID, &branchName)
	if err != nil {
		jsonError(w, "API key inválida", http.StatusUnauthorized)
		return
	}
	var currentBranch int
	if err := tenantDB.QueryRow(`SELECT branch_id FROM api_keys WHERE api_key = ? AND is_active = 1 LIMIT 1`, req.APIKey).Scan(&currentBranch); err != nil || currentBranch != branchID {
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
	err = ctl.QueryRow(`
		SELECT u.user_id, ul.username, ul.password, u.first_name, u.last_name
		FROM user_login ul
		JOIN users u ON u.user_id = ul.user_id
		WHERE ul.username = ? AND ul.status = 1 AND u.status = 1 LIMIT 1`, req.Username).Scan(&userID, &username, &passwordHash, &firstName, &lastName)
	if err != nil {
		jsonError(w, "Credenciales inválidas", http.StatusUnauthorized)
		return
	}

	// Pertenencia a la empresa del API key: la misma lista que valida el
	// selector de empresa del panel (tenant_users en control). Sin esto, una
	// clave de la empresa B más un usuario de la A abrirían caja en B.
	if !s.CanAccessTenant(username, tenant) {
		jsonError(w, "El usuario no pertenece a esta empresa", http.StatusForbidden)
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
		// Se reescribe en CONTROL: es la fila que el panel cambia y la que
		// este login lee. La copia de la empresa queda desactualizada a
		// propósito —ya nadie autentica contra ella— y la migración de
		// identidad la retirará.
		if err := reescribirPassword(ctl, userID, username, req.Password); err != nil {
			log.Printf("AVISO: no se pudo actualizar la contraseña POS de %s a bcrypt: %v", username, err)
		}
	}

	// Credenciales correctas: se limpia el contador de intentos fallidos.
	loginLimiter.resetAttempts(ipKey)
	loginLimiter.resetAttempts(userKey)

	var deptOK bool
	if err := tenantDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_department WHERE user_id = ? AND department_id = 4)`, userID).Scan(&deptOK); err != nil {
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

	res, err := tenantDB.Exec(`
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

	_, _ = tenantDB.Exec(`
		INSERT INTO pos_turn_events (terminal_id, branch_id, user_id, event_type, event_time, details)
		VALUES (?, ?, ?, 'login', NOW(), ?)`,
		req.TerminalID, branchID, userID, "Login POS: "+username)

	// El nivel sale de la identidad de control (sec_userrole global), con el
	// mismo fallback por user_type que el panel: un administrador de empresa
	// que abre caja no puede ser «employee» solo porque la copia local no
	// tenga su rol.
	level := s.nivelParaPOS(ctl, userID)

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

// nivelParaPOS resuelve el nivel de un usuario de POS desde la base de
// control, compartiendo las reglas del panel (loadUserRole + user_type):
// master si es super, admin por tipo, employee por defecto.
func (s *Server) nivelParaPOS(ctl *sql.DB, userID int) string {
	uid := strconv.Itoa(userID)
	if level, err := loadUserRole(ctl, uid); err == nil && level != "" {
		return level
	}
	if level := levelDeTipoDeUsuario(ctl, uid); level != "" {
		return level
	}
	return "employee"
}

func (s *Server) handlePOSValidate(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-POS-Token")
	if token == "" {
		jsonError(w, "Token requerido en X-POS-Token", http.StatusUnauthorized)
		return
	}

	// La empresa del terminal viene en X-POS-Tenant: las SESIONES POS y las
	// sucursales viven en la base de ESA empresa. La IDENTIDAD se comprueba
	// contra la base de control (S-2): si el usuario fue dado de baja o
	// desactivado en el panel, la sesión del POS cae en la siguiente
	// validación —antes quedaba viva hasta las 12 horas porque el POS leía
	// la copia de la empresa, que el panel no actualiza—.
	tenantDB := s.posDBFor(r)
	if tenantDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	var err error

	var userID string
	var terminalID sql.NullString
	var branchID int
	var expiresAt time.Time

	err = tenantDB.QueryRow(`
		SELECT user_id, terminal_id, branch_id, expires_at FROM pos_sessions
		WHERE token = ? AND expires_at > NOW() AND is_active = 1 AND IFNULL(revoked, 0) = 0
		LIMIT 1`, token).Scan(&userID, &terminalID, &branchID, &expiresAt)
	if err != nil {
		jsonError(w, "Token inválido o expirado", http.StatusUnauthorized)
		return
	}

	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "Servicio de identidad no disponible", http.StatusServiceUnavailable)
		return
	}
	var firstName, lastName sql.NullString
	var active bool
	if err := ctl.QueryRow(`SELECT EXISTS(SELECT 1 FROM users u JOIN user_login ul ON ul.user_id = u.user_id
		WHERE u.user_id = ? AND u.status = 1 AND ul.status = 1)`, userID).Scan(&active); err != nil {
		jsonError(w, "No se pudo comprobar el usuario POS", http.StatusInternalServerError)
		return
	}
	if !active {
		// Revocación efectiva: la sesión muere con el usuario, no con su
		// expiración. Se marca aquí para que el logout dejen de aparecer
		// activas en el panel.
		_, _ = tenantDB.Exec(`UPDATE pos_sessions SET is_active = 0, revoked = 1 WHERE token = ?`, token)
		jsonError(w, "Usuario POS inactivo", http.StatusUnauthorized)
		return
	}
	_ = ctl.QueryRow(`SELECT first_name, last_name FROM users WHERE user_id = ? LIMIT 1`, userID).
		Scan(&firstName, &lastName)
	level := s.nivelParaPOS(ctl, mustAtoi(userID))
	writeJSON(w, map[string]interface{}{
		"valid":       true,
		"user_id":     userID,
		"first_name":  firstName.String,
		"last_name":   lastName.String,
		"level":       level,
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
	tenantDB := s.posDBFor(r)
	if tenantDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	var err error

	var userID, terminalID sql.NullString
	err = tenantDB.QueryRow(`
		SELECT user_id, terminal_id FROM pos_sessions WHERE token = ? AND is_active = 1 AND IFNULL(revoked, 0) = 0`,
		token).Scan(&userID, &terminalID)

	if err == nil {
		if _, err := tenantDB.Exec(`UPDATE pos_sessions SET is_active = 0, revoked = 1, logout_time = NOW() WHERE token = ?`, token); err != nil {
			jsonError(w, "No se pudo cerrar la sesión POS", http.StatusInternalServerError)
			return
		}
		_, _ = tenantDB.Exec(`
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

	// La empresa del terminal aporta la sesión y el registro del evento; la
	// IDENTIDAD y el nivel del supervisor se comprueban en control (S-2) y
	// se limitan por barcode además de por IP (R8, auditoría: la IP era
	// fabricable con X-Forwarded-For y sin límite por carnet, la clave que
	// destraba devoluciones y descuentos se podía probar a discreción).
	tenantDB := s.posDBFor(r)
	if tenantDB == nil {
		jsonError(w, "Empresa POS no disponible", http.StatusServiceUnavailable)
		return
	}
	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "Servicio de identidad no disponible", http.StatusServiceUnavailable)
		return
	}
	supUserKey := "pos:sup:code:" + strings.ToLower(strings.TrimSpace(req.Barcode))
	if loginLimiter.checkRateLimit(supUserKey) {
		jsonError(w, "Demasiados intentos con este carnet. Espere unos minutos.", http.StatusTooManyRequests)
		return
	}

	// Buscar supervisor por barcode/employee_code y verificar que tenga rol
	// master/manager (o admin de empresa, que gestiona su compañía entera).
	var userID int
	var passwordHash string
	var supervisorName string
	var supervisorUser string
	err = ctl.QueryRow(`
		SELECT u.user_id, ul.username, ul.password, CONCAT(u.first_name, ' ', u.last_name)
		FROM users u
		JOIN user_login ul ON ul.user_id = u.user_id
		WHERE (u.employee_code = ? OR u.barcode = ?)
		AND u.status = 1 AND ul.status = 1
		LIMIT 1`, req.Barcode, req.Barcode).Scan(&userID, &supervisorUser, &passwordHash, &supervisorName)
	if err != nil {
		loginLimiter.recordFailedAttempt(supIPKey)
		loginLimiter.recordFailedAttempt(supUserKey)
		jsonError(w, "Código de supervisor no válido", http.StatusUnauthorized)
		return
	}
	switch nivel := s.nivelParaPOS(ctl, userID); nivel {
	case "master", "manager", "admin":
		// nivel autorizado
	default:
		loginLimiter.recordFailedAttempt(supIPKey)
		loginLimiter.recordFailedAttempt(supUserKey)
		jsonError(w, "El código no pertenece a un supervisor", http.StatusUnauthorized)
		return
	}

	// Verificar contraseña
	valid, necesitaRehash := verificarPassword(passwordHash, req.Password)
	if !valid {
		loginLimiter.recordFailedAttempt(supIPKey)
		loginLimiter.recordFailedAttempt(supUserKey)
		jsonError(w, "Contraseña de supervisor incorrecta", http.StatusUnauthorized)
		return
	}
	if necesitaRehash {
		if err := reescribirPassword(ctl, userID, supervisorUser, req.Password); err != nil {
			log.Printf("AVISO: no se pudo subir a bcrypt la clave del supervisor %d: %v", userID, err)
		}
	}

	// Autorización correcta: limpiar el contador de intentos fallidos.
	loginLimiter.resetAttempts(supIPKey)
	loginLimiter.resetAttempts(supUserKey)

	// Registrar autorización
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = req.AuthType
	}

	_, _ = tenantDB.Exec(`
		INSERT INTO pos_supervisor_auth (session_id, supervisor_id, auth_type, auth_time, reason, barcode)
		VALUES (0, ?, ?, NOW(), ?, ?)`,
		userID, req.AuthType, reason, req.Barcode)

	writeJSON(w, map[string]interface{}{
		"authorized":      true,
		"supervisor_id":   userID,
		"supervisor_name": supervisorName,
	})
}

// mustAtoi convierte el id de usuario (varchar en las sesiones, int en
// control) sin abortar: un id no numérico devuelve 0 y nivelParaPOS lo
// resolverá como employee, que es el piso seguro.
func mustAtoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func generatePOSToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return "pos_" + hex.EncodeToString(b)
}
