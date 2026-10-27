package web

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter controla intentos de login por IP y usuario.
type rateLimiter struct {
	mu       sync.RWMutex
	attempts map[string]*loginAttempts
}

type loginAttempts struct {
	count    int
	lastFail time.Time
	locked   bool
}

var loginLimiter = &rateLimiter{
	attempts: make(map[string]*loginAttempts),
}

const maxAttempts = 5
const lockoutDuration = 15 * time.Minute

func (rl *rateLimiter) checkRateLimit(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for k, v := range rl.attempts {
		if now.Sub(v.lastFail) > lockoutDuration {
			delete(rl.attempts, k)
		}
	}

	att, exists := rl.attempts[key]
	if !exists {
		return false
	}

	if att.locked && now.Sub(att.lastFail) > lockoutDuration {
		att.locked = false
		att.count = 0
		return false
	}

	return att.locked
}

func (rl *rateLimiter) recordFailedAttempt(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	att, exists := rl.attempts[key]
	if !exists {
		att = &loginAttempts{}
		rl.attempts[key] = att
	}

	att.count++
	att.lastFail = time.Now()

	if att.count >= maxAttempts {
		att.locked = true
		log.Printf("⚠️ Rate limit: %s bloqueado por %d intentos fallidos", key, att.count)
	}
}

func (rl *rateLimiter) resetAttempts(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

// handleLogin shows the login page and processes POST login.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		clientIP := r.RemoteAddr
		if host, _, found := strings.Cut(clientIP, ":"); found {
			clientIP = host
		}

		if loginLimiter.checkRateLimit(clientIP) {
			s.renderStandalone(r, w, "login.html", map[string]interface{}{
				"Error": "Demasiados intentos. Espera 15 minutos.",
			})
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))

		if loginLimiter.checkRateLimit("user:" + username) {
			s.renderStandalone(r, w, "login.html", map[string]interface{}{
				"Error": "Demasiados intentos. Espera 15 minutos.",
			})
			return
		}

		password := r.FormValue("password")

		sess, err := s.authenticateUser(r, username, password)
		if err != nil {
			loginLimiter.recordFailedAttempt(clientIP)
			loginLimiter.recordFailedAttempt("user:" + username)

			s.renderStandalone(r, w, "login.html", map[string]interface{}{
				"Error": "Credenciales inválidas",
			})
			return
		}

		loginLimiter.resetAttempts(clientIP)
		loginLimiter.resetAttempts("user:" + username)

		tenants, _ := s.ListTenantsForUser(sess.Username)
		if len(tenants) > 1 {
			sess.TenantCode = ""
			id := s.sessions.Create(sess)
			http.SetCookie(w, &http.Cookie{
				Name: "erpgow_session", Value: id, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400,
			})
			http.Redirect(w, r, "/switch-tenant", http.StatusFound)
			return
		}
		if len(tenants) == 1 {
			if err := s.setTenant(tenants[0].Code); err == nil {
				sess.TenantCode = tenants[0].Code
			}
		}

		id := s.sessions.Create(sess)
		http.SetCookie(w, &http.Cookie{
			Name:     "erpgow_session",
			Value:    id,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if s.sessionFromRequest(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if s.needsTenantSetup() {
		http.Redirect(w, r, "/setup/company", http.StatusFound)
		return
	}
	s.renderStandalone(r, w, "login.html", nil)
}

// handleLogout destroys the session.
//
// NO SE CIERRA EN SILENCIO SI HAY UN TURNO DE CAJA ABIERTO. La sesión se va, el
// turno se queda: su tiempo sigue corriendo, la caja sigue abierta en el panel
// del POS y nadie se enteró. Cerrar el turno es un arqueo y no se puede hacer
// por el usuario —falsearía la prueba—, así que se le dice cuál es, desde cuándo
// y cuánto lleva, y elige: volver a cerrarlo o salir dejándolo abierto. En ese
// segundo caso la salida queda en la cadena de auditoría, con los turnos que
// quedaron abiertos, para que el turno huérfano tenga explicación.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)

	var turnos []turnoAbierto
	if sess != nil {
		turnos = s.turnosAbiertosDe(sess)
	}
	if len(turnos) > 0 && r.URL.Query().Get("forzar") != "1" {
		s.renderStandalone(r, w, "logout_turno_abierto.html", map[string]interface{}{
			"Title":  "Turno de caja abierto",
			"Turnos": turnos,
			"Texto":  textoDeTurnosAbiertos(turnos),
		})
		return
	}
	if len(turnos) > 0 {
		s.appendAudit(r, sess.UserID, clientIP(r), "logout", "Sesión", "web_sessions", "",
			map[string]interface{}{"salida_con_turno_abierto": turnos}, nil)
	}

	if cookie, err := r.Cookie("erpgow_session"); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "erpgow_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// handleAPILogin is the JSON login endpoint (mobile/API).
func (s *Server) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := r.RemoteAddr
	if host, _, found := strings.Cut(clientIP, ":"); found {
		clientIP = host
	}

	if loginLimiter.checkRateLimit("api:" + clientIP) {
		jsonError(w, "Demasiados intentos. Espera 15 minutos.", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil {
		jsonError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(req.Username)

	if loginLimiter.checkRateLimit("api:user:" + username) {
		jsonError(w, "Demasiados intentos. Espera 15 minutos.", http.StatusTooManyRequests)
		return
	}

	sess, err := s.authenticateUser(r, username, req.Password)
	if err != nil {
		loginLimiter.recordFailedAttempt("api:" + clientIP)
		loginLimiter.recordFailedAttempt("api:user:" + username)
		jsonError(w, "Credenciales inválidas", http.StatusUnauthorized)
		return
	}

	loginLimiter.resetAttempts("api:" + clientIP)
	loginLimiter.resetAttempts("api:user:" + username)

	id := s.sessions.Create(sess)
	writeJSON(w, map[string]interface{}{
		"session":  id,
		"user_id":  sess.UserID,
		"username": sess.Username,
		"level":    sess.UserLevel,
		"fullname": sess.Fullname,
	})
}

// authenticateUser validates credentials and returns session data.
func (s *Server) authenticateUser(r *http.Request, username, password string) (*SessionData, error) {
	var (
		userID       int
		userType     sql.NullInt64
		passwordHash sql.NullString
		first, last  sql.NullString
		logo         sql.NullString
	)
	err := s.authDB(r).QueryRow(`
		SELECT u.user_id, ul.user_type, ul.password, u.first_name, u.last_name, u.logo
		FROM user_login ul
		JOIN users u ON u.user_id = ul.user_id
		WHERE ul.username = ? AND ul.status = 1
		LIMIT 1`, username).
		Scan(&userID, &userType, &passwordHash, &first, &last, &logo)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}

	hash := passwordHash.String
	valid, necesitaRehash := verificarPassword(hash, password)
	if !valid {
		return nil, fmt.Errorf("invalid password")
	}

	if necesitaRehash {
		if err := reescribirPassword(s.authDB(r), userID, username, password); err != nil {
			log.Printf("AVISO: no se pudo actualizar la contraseña de %s a bcrypt: %v", username, err)
		}
	}

	// El rol por empresa (sec_userrole) manda si existe; si está vacía —empresa
	// recién creada— se cae al catálogo GLOBAL de la BD de control, que es la
	// misma fuente que usa el middleware de permisos. Sin este fallback el admin
	// de una empresa nueva entraba con nivel "employee": el menú lateral se
	// quedaba en Dashboard+Caja y no aparecían Personal ni Usuarios.
	uid := fmt.Sprintf("%d", userID)
	level := ""
	err = s.dbFor(r).QueryRow(`
		SELECT r.level FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid
		WHERE ur.user_id = ? LIMIT 1`, uid).Scan(&level)
	if err != nil || level == "" {
		level = ""
		if ctl, cerr := s.controlStore(); cerr == nil {
			_ = ctl.QueryRow(`
				SELECT r.level FROM sec_userrole ur
				JOIN sec_role r ON r.id = ur.roleid
				WHERE ur.user_id = ? LIMIT 1`, uid).Scan(&level)
		}
	}
	if level == "" {
		// Sin rol en ninguna base: user_type=1 es el admin de la tabla users
		// (el mismo mapeo que usa la pantalla de usuarios).
		if userType.Valid && userType.Int64 == 1 {
			level = "admin"
		} else {
			level = "employee"
		}
	}

	fullname := strings.TrimSpace(first.String + " " + last.String)
	if fullname == "" {
		fullname = username
	}
	return &SessionData{
		UserID:    fmt.Sprintf("%d", userID),
		Username:  username,
		Fullname:  fullname,
		UserLevel: level,
		Image:     logo.String,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil
}
