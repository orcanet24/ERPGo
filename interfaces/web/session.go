package web

// session.go — Sesiones persistentes.
//
// Antes vivían en un mapa de RAM: reiniciar el servidor cerraba la sesión de
// todo el mundo y, en multi-empresa, el TenantCode se iba con ella. Ahora se
// guardan en la BD de control (erpgo.db), de modo que sobreviven a un reinicio.
//
// El diseño evita las dos trampas de un almacén persistente:
//
//   - NO se consulta la base en cada request. Un SELECT por cada petición de la
//     aplicación es exactamente lo que hace lento a un ERP. Hay una caché en memoria
//     que se lee primero y se rellena al fallar; la escritura va a la base y a
//     la caché a la vez.
//   - NO se pierde la invalidación. Borrar sesión borra en ambos, y una sesión
//     expirada se detecta por su ExpiresAt en la caché, no confiando en que
//     alguien limpió la tabla.
//
// La caché no es una segunda fuente de verdad: la fila sigue siendo la fuente.
// Si el proceso corre en varias instancias (no es el caso de SQLite con un solo
// archivo), la caché haría falta invalidación entre procesos; aquí, un solo
// proceso escribe, así que basta.

import (
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SessionData holds the logged-in user info (mirrors PHP session userdata).
type SessionData struct {
	UserID    string
	Username  string
	Fullname  string
	UserLevel string // "admin" | "employee" | "user"
	Image     string
	// TenantCode es la empresa activa (código = nombre del archivo SQLite).
	// Vacío = empresa implícita "default" (PyME de una sola empresa).
	// En Enterprise, cambiar este valor cambia de archivo de BD, así que va
	// en la sesión, no en una cookie de cliente manipulable.
	TenantCode string
	// IsAgent marca la sesión como la de un agente IA. Un agente no es una
	// persona: sus acciones se auditan como "agente", nunca como el humano
	// que las disparó, y quedan bloqueadas por las reglas anti-fraude del
	// nivel 1 (ver ai_rules.go).
	IsAgent bool
	// CSRFToken acredita que la petición que modifica datos salió del propio
	// navegador y no de un formulario inyectado desde otro sitio. Viaja en la
	// sesión (no en una cookie aparte) porque leer la cookie requiere estar en el
	// mismo origen, mientras que enviarla es automático: ese es justo el
	// atributo que hace falta.
	CSRFToken string
	ExpiresAt time.Time
}

// SessionStore guarda las sesiones en la BD de control con una caché de lectura.
//
// db puede ser nil (en tests y durante el arranque): en ese caso degrada a solo
// memoria, que es el comportamiento antiguo y suficiente para una vida corta.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SessionData

	muDB sync.Mutex
	db   *sql.DB
}

// NewSessionStore crea el almacén en memoria. Las sesiones se persisten en
// cuanto se conecta la BD de control (SetDB).
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*SessionData)}
}

// SetDB conecta el almacén a la BD de control. A partir de ahí cada alta,
// cambio y baja se persiste. Se llama una vez, al arrancar.
func (st *SessionStore) SetDB(db *sql.DB) {
	st.muDB.Lock()
	st.db = db
	st.muDB.Unlock()
}

// Create registra una sesión y devuelve su identificador (la cookie).
func (st *SessionStore) Create(data *SessionData) string {
	id := randomHex(32)

	st.mu.Lock()
	st.sessions[id] = data
	st.mu.Unlock()

	st.persist(id, data)
	return id
}

// Get devuelve la sesión no expirada, o nil.
func (st *SessionStore) Get(id string) *SessionData {
	if id == "" {
		return nil
	}
	st.mu.RLock()
	s := st.sessions[id]
	st.mu.RUnlock()

	if s == nil {
		// No está en caché: puede ser una sesión creada antes de un reinicio.
		// Este es el camino que justifica persistir.
		if s = st.load(id); s == nil {
			return nil
		}
		st.mu.Lock()
		st.sessions[id] = s
		st.mu.Unlock()
	}

	if s.ExpiresAt.Before(time.Now()) {
		st.Delete(id)
		return nil
	}
	return s
}

// Update reemplaza los datos de una sesión (p. ej. al cambiar de empresa) y
// actualiza tanto la caché como la base.
func (st *SessionStore) Update(id string, data *SessionData) {
	st.mu.Lock()
	st.sessions[id] = data
	st.mu.Unlock()
	st.persist(id, data)
}

// Touch extiende la vida de una sesión, para que un usuario que trabaja no
// expires mientras escribe su informe.
func (st *SessionStore) Touch(id string, expires time.Time) {
	st.mu.Lock()
	s := st.sessions[id]
	if s != nil {
		cp := *s
		cp.ExpiresAt = expires
		st.sessions[id] = &cp
		s = &cp
	}
	st.mu.Unlock()
	if s != nil {
		st.persist(id, s)
	}
}

// Delete cierra la sesión en memoria y en la base.
func (st *SessionStore) Delete(id string) {
	st.mu.Lock()
	delete(st.sessions, id)
	st.mu.Unlock()

	if db := st.database(); db != nil {
		_, _ = db.Exec(`DELETE FROM web_sessions WHERE id = ?`, id)
	}
}

// StartJanitor purga las sesiones expiradas.
//
// Se ejecuta periódicamente para que la tabla no crezca sin límite y para que
// una sesión caducada no siga ocupando espacio tras miles de inicios de sesión.
func (st *SessionStore) StartJanitor() {
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			st.purgeExpired()
		}
	}()
}

func (st *SessionStore) purgeExpired() {
	cutoff := time.Now().Format("2006-01-02 15:04:05")

	st.mu.Lock()
	for k, v := range st.sessions {
		if v.ExpiresAt.Before(time.Now()) {
			delete(st.sessions, k)
		}
	}
	st.mu.Unlock()

	if db := st.database(); db != nil {
		_, _ = db.Exec(`DELETE FROM web_sessions WHERE expires_at <> '' AND expires_at < ?`, cutoff)
	}
}

// database devuelve el handle de la BD de control bajo su propio lock: el
// puntero se escribe una vez al arrancar, pero se lee desde cada request.
func (st *SessionStore) database() *sql.DB {
	st.muDB.Lock()
	defer st.muDB.Unlock()
	return st.db
}

// persist escribe la sesión en la BD de control. Los errores se ignoran a
// propósito: una caída puntual de la base no debe tumbar la respuesta que el
// usuario ya tiene en pantalla, y la caché en memoria sigue sirviendo.
func (st *SessionStore) persist(id string, d *SessionData) {
	db := st.database()
	if db == nil || d == nil {
		return
	}
	exp := ""
	if !d.ExpiresAt.IsZero() {
		exp = d.ExpiresAt.Format("2006-01-02 15:04:05")
	}
	isAgent := 0
	if d.IsAgent {
		isAgent = 1
	}
	_, _ = db.Exec(`
		INSERT INTO web_sessions
			(id, user_id, username, fullname, user_level, image, tenant_code, is_agent, csrf_token, expires_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), username=VALUES(username), fullname=VALUES(fullname),
			user_level=VALUES(user_level), image=VALUES(image), tenant_code=VALUES(tenant_code),
			is_agent=VALUES(is_agent), csrf_token=VALUES(csrf_token), expires_at=VALUES(expires_at)`,
		id, d.UserID, d.Username, d.Fullname, d.UserLevel, d.Image, d.TenantCode, isAgent, d.CSRFToken, exp, now())
}

// load recupera una sesión de la BD de control.
func (st *SessionStore) load(id string) *SessionData {
	db := st.database()
	if db == nil {
		return nil
	}
	var (
		d          SessionData
		tenantCode string
		isAgent    int
		expires, _ string
	)
	err := db.QueryRow(`
		SELECT user_id, username, fullname, user_level, image, tenant_code, is_agent, csrf_token, expires_at
		FROM web_sessions WHERE id = ?`, id).
		Scan(&d.UserID, &d.Username, &d.Fullname, &d.UserLevel, &d.Image, &tenantCode, &isAgent, &d.CSRFToken, &expires)
	if err != nil {
		return nil
	}
	d.TenantCode = tenantCode
	d.IsAgent = isAgent == 1
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(expires), time.Local); err == nil {
		d.ExpiresAt = t
	} else {
		// Sin fecha utilizable la sesión se considera inválida en vez de
		// Eternal: una fila corrupta no debe conceder acceso.
		return nil
	}
	return &d
}

// sessionFromRequest extracts the current session (nil if not logged in).
func (s *Server) sessionFromRequest(r *http.Request) *SessionData {
	// `r == nil` llega desde las llamadas internas y desde las pruebas, que
	// llaman a las funciones de negocio sin pasar por HTTP. `r.Cookie` sobre un
	// valor nulo revienta con un panic, no con un error: un nil en la base del
	// panic es mucho más difícil de encontrar que un "no hay sesión".
	if r == nil {
		return nil
	}
	// Una petición interna (la IA, el semillero de una empresa de prueba) no
	// trae cookie: su identidad viaja en el ámbito de la petición. Sin esto,
	// una compra cargada por un agente se registraba como del usuario "1" y su
	// rol nunca se consultaba.
	if sc := scopeOf(r); sc != nil && sc.sess != nil {
		return sc.sess
	}
	cookie, err := r.Cookie("erpgow_session")
	if err != nil {
		return nil
	}
	return s.sessions.Get(cookie.Value)
}
