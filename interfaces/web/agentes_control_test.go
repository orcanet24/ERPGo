package web

// agentes_control_test.go — La identidad de los agentes vive en la BD de
// control, aunque la petición venga de una empresa cualquiera.
//
// POR QUÉ ESTA PRUEBA EXISTE.
//
// agentForm, agentSave y loadAIAgents consultaban sec_role, user_login y
// ai_agent con s.dbFor(r) (la base de la EMPRESA). En la empresa maestra ambas
// bases coinciden y todo parecía bien; en cualquier otra empresa el
// desplegable de roles salía VACÍO («no aparecen los roles»), el alta
// respondía «Rol no encontrado» aunque los roles estuvieran, y el listado
// quedaba en blanco. El fallo se escondió desde el mega-commit 6f94394 hasta
// que se creó la segunda empresa.
//
// El escenario de abajo es doble-BD a propósito: la base de empresa NO tiene
// ninguna tabla de identidad ni de agentes —si el código vuelve a preguntarle,
// la prueba se rompe, que es exactamente lo que debe pasar—.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"erpgo/configs"
	"erpgo/infrastructure/persistence"

	_ "github.com/glebarez/go-sqlite"
)

// abrirControlDeAgentes crea la BD de CONTROL de la prueba: solo lo que vive
// ahí (roles, identidad, filas de agente) más lo que controlSchema le añade.
func abrirControlDeAgentes(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base de control: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	tablas := []string{
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY, type TEXT, level TEXT)`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, first_name TEXT, last_name TEXT, status INTEGER)`,
		`CREATE TABLE user_login (user_id TEXT PRIMARY KEY, username TEXT UNIQUE, password TEXT, user_type INTEGER, status INTEGER)`,
		`CREATE TABLE sec_userrole (id INTEGER PRIMARY KEY, user_id TEXT, roleid INTEGER, createby TEXT, createdate TEXT)`,
		`CREATE TABLE ai_agent (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, user_id TEXT, provider_id INTEGER,
			model_override TEXT, role_id INTEGER, operator_enabled INTEGER, max_steps INTEGER, max_rows INTEGER,
			enabled INTEGER, created_by TEXT, created_at TEXT, department TEXT, config_prompt TEXT, config_prompt_at TEXT)`,
	}
	for _, ddl := range tablas {
		if _, err := db.Exec(strings.Join(strings.Fields(ddl), " ")); err != nil {
			t.Fatalf("crear tabla de control: %v", err)
		}
	}
	return db
}

// abrirEmpresaSinIdentidad crea la BD de la EMPRESA de la prueba: negocio pero
// NADA de identidad —ni sec_role ni users ni user_login ni ai_agent—. Que el
// código vuelva a preguntarle por roles o agentes es el error que esto detecta.
func abrirEmpresaSinIdentidad(t *testing.T) *sql.DB {
	t.Helper()
	db, err := persistence.OpenSQLite(filepath.Join(t.TempDir(), "empresa.sqlite3"))
	if err != nil {
		t.Fatalf("no se pudo abrir la base de la empresa: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tablas := []string{
		`CREATE TABLE ai_providers (id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL DEFAULT '', vendor TEXT, base_url TEXT NOT NULL DEFAULT '',
			api_key TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
			temperature REAL NOT NULL DEFAULT 0.7, max_tokens INTEGER NOT NULL DEFAULT 400,
			prompt_template TEXT NOT NULL DEFAULT '', is_active INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, action TEXT, module TEXT,
			table_name TEXT, record_id TEXT, old_value TEXT, new_value TEXT, ip_address TEXT, created_at TEXT,
			prev_hash TEXT, record_hash TEXT)`,
		`CREATE TABLE audit_chain_blocks (period TEXT PRIMARY KEY, prev_block_hash TEXT, start_hash TEXT,
			end_hash TEXT, first_id INTEGER, last_id INTEGER, record_count INTEGER, block_hash TEXT, sealed_at TEXT)`,
	}
	for _, ddl := range tablas {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla de empresa: %v", err)
		}
	}
	return db
}

// servidorDoble monta el Server igual que el arranque: base de empresa activa,
// base de control aparte. Es el retrato de una instalación con una empresa
// NO maestra.
func servidorDoble(t *testing.T) (*Server, *sql.DB, *sql.DB) {
	t.Helper()
	control := abrirControlDeAgentes(t)
	empresa := abrirEmpresaSinIdentidad(t)
	s := &Server{
		config:      &configs.Config{AppEnv: "test", AppName: "ERPGo"},
		db:          empresa,
		controlDB:   control,
		controlListo: true,
		sessions:    NewSessionStore(),
		activeTenant: defaultTenantCode,
	}
	return s, empresa, control
}

// pagarConAmbito pide una petición con el ámbito que attachScope llevaría:
// la base de la EMPRESA en el contexto, y la identidad en la sesión.
func pagarConAmbito(s *Server, empresaDB *sql.DB, sess *SessionData, metodo, path string, form url.Values) *http.Request {
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(metodo, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return withScope(r, &scope{tenant: defaultTenantCode, db: empresaDB, sess: sess})
}

// TestLosRolesDelFormularioDeAgentesVienenDeControl es la regresión literal
// del bug reportado: «los roles no cargan en /ai/agents/add». El desplegable
// debe leerlos en la BD de control aunque la empresa de la petición ni
// siquiera tenga la tabla sec_role.
func TestLosRolesDelFormularioDeAgentesVienenDeControl(t *testing.T) {
	s, empresa, control := servidorDoble(t)
	for _, nombre := range []string{"Contador", "Vendedor"} {
		if _, err := control.Exec(`INSERT INTO sec_role (type) VALUES (?)`, nombre); err != nil {
			t.Fatalf("no se pudo sembrar el rol %s: %v", nombre, err)
		}
	}

	r := pagarConAmbito(s, empresa, sessPrueba("admin"), "GET", "/ai/agents/add", nil)
	roles := s.rolesDeControl(r)
	if len(roles) != 2 {
		t.Fatalf("el formulario debe ofrecer 2 roles de control, ofreció %d", len(roles))
	}
	if roles[0].Name != "Contador" || roles[1].Name != "Vendedor" {
		t.Errorf("los roles deben salir ordenados de sec_role, salieron %q y %q", roles[0].Name, roles[1].Name)
	}
}

// TestElAltaDeAgenteVerificaElRolYLaIdentidadEnControl comprueba el ciclo
// completo del bug: alta con rol de control y —la parte que nadie ve hasta que
// dos usernames chocan— unicidad del username resuelta TAMBIÉN en control.
func TestElAltaDeAgenteVerificaElRolYLaIdentidadEnControl(t *testing.T) {
	s, empresa, control := servidorDoble(t)
	if _, err := control.Exec(`INSERT INTO sec_role (id, type) VALUES (1, 'Contador')`); err != nil {
		t.Fatalf("no se pudo sembrar el rol: %v", err)
	}
	// Un agente anterior ya tomó «agente.cobranza» en control: el nuevo que
	// quiera el mismo nombre debe salir con sufijo, no chocar.
	if _, err := control.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
		VALUES ('35', 'agente.cobranza', 'x', 2, 0)`); err != nil {
		t.Fatalf("no se pudo sembrar el username previo: %v", err)
	}
	if _, err := empresa.Exec(`INSERT INTO ai_providers (id, name, vendor, is_active)
		VALUES (2, 'ORC24', 'orc24', 1)`); err != nil {
		t.Fatalf("no se pudo sembrar el proveedor activo: %v", err)
	}

	// 1) El rol se verifica en control: un id inexistente ahí se rechaza.
	form := url.Values{"name": {"Cobranza"}, "role_id": {"99"}}
	w := httptest.NewRecorder()
	s.agentSave(w, pagarConAmbito(s, empresa, sessPrueba("admin"), "POST", "/ai/agents/save", form))
	if w.Code != 404 || !strings.Contains(w.Body.String(), "Rol no encontrado") {
		t.Fatalf("el rol 99 no existe y debió rechazarse (404); salió %d: %s", w.Code, w.Body.String())
	}

	// 2) El alta correcta: identidad y fila de agente se escriben en CONTROL.
	form = url.Values{"name": {"Cobranza"}, "role_id": {"1"}}
	w = httptest.NewRecorder()
	s.agentSave(w, pagarConAmbito(s, empresa, sessPrueba("admin"), "POST", "/ai/agents/save", form))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":1`) {
		t.Fatalf("el alta debió crear el agente con rol de control; salió %d: %s", w.Code, w.Body.String())
	}

	var username, userIDEnLogin string
	if err := control.QueryRow(`SELECT user_id, username FROM user_login WHERE username LIKE 'agente.cobranza.%' ORDER BY username LIMIT 1`).
		Scan(&userIDEnLogin, &username); err != nil {
		t.Fatalf("la identidad del agente no quedó en la BD de control: %v", err)
	}
	if username != "agente.cobranza.2" {
		t.Errorf("el username debe desplazarse al siguiente libre de CONTROL, salió %q", username)
	}
	var rolUser int64
	if err := control.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = ?`, userIDEnLogin).Scan(&rolUser); err != nil || rolUser != 1 {
		t.Errorf("el rol del usuario agente debió escribirse en control (sec_userrole); salió rol=%d err=%v", rolUser, err)
	}
	var providerID int64
	if err := control.QueryRow(`SELECT provider_id FROM ai_agent WHERE user_id = ?`, userIDEnLogin).Scan(&providerID); err != nil {
		t.Fatalf("la fila del agente no quedó en control: %v", err)
	}
	if providerID != 2 {
		t.Errorf("el agente debe quedar amarrado al proveedor activo (2), quedó con %d", providerID)
	}
	var primerNombre, apellido string
	if err := control.QueryRow(`SELECT first_name, last_name FROM users WHERE user_id = ?`, userIDEnLogin).
		Scan(&primerNombre, &apellido); err != nil {
		t.Errorf("el usuario del agente no quedó en users de control: %v", err)
	}
	if primerNombre != "Cobranza" || apellido != " (Agente IA)" {
		t.Errorf("el nombre del usuario agente debió guardarse separado en control, salió %q %q", primerNombre, apellido)
	}

	// 3) Un segundo agente con el mismo nombre toma el siguiente libre, y la
	// unicidad se comprueba contra CONTROL (donde viven los logins), no contra
	// la base de la empresa.
	form = url.Values{"name": {"Cobranza"}, "role_id": {"1"}}
	w = httptest.NewRecorder()
	s.agentSave(w, pagarConAmbito(s, empresa, sessPrueba("admin"), "POST", "/ai/agents/save", form))
	if !strings.Contains(w.Body.String(), `"status":1`) {
		t.Fatalf("el segundo alta debió salir bien; salió %d: %s", w.Code, w.Body.String())
	}
	var tercer int
	if err := control.QueryRow(`SELECT COUNT(*) FROM user_login WHERE username = 'agente.cobranza.3'`).Scan(&tercer); err != nil || tercer != 1 {
		t.Errorf("el tercer username esperado (agente.cobranza.3) no quedó en control: %d err=%v", tercer, err)
	}
	if n := contarFilas(t, control, "users"); n != 2 {
		t.Errorf("deben quedar 2 usuarios agente en control, quedaron %d", n)
	}
}

// TestElListadoDeAgentesLeeControlNoLaEmpresa cubre la otra cara del puerto
// equivocado: el listado que quedaba en blanco en toda empresa no maestra.
func TestElListadoDeAgentesLeeControlNoLaEmpresa(t *testing.T) {
	s, _, control := servidorDoble(t)
	if _, err := control.Exec(`INSERT INTO sec_role (id, type) VALUES (1, 'Contador')`); err != nil {
		t.Fatalf("no se pudo sembrar el rol: %v", err)
	}
	if _, err := control.Exec(`INSERT INTO ai_agent (id, name, user_id, provider_id, role_id, enabled, max_steps, max_rows)
		VALUES (5, 'Cobranza', '44', 2, 1, 1, 8, 200)`); err != nil {
		t.Fatalf("no se pudo sembrar el agente: %v", err)
	}

	lista := s.loadAIAgents(sessPrueba("admin"))
	if len(lista) != 1 {
		t.Fatalf("el listado debe leer los agentes de CONTROL (hay 1), devolvió %d", len(lista))
	}
	g := lista[0]
	if g.ID != 5 || g.UserID != "44" {
		t.Errorf("el agente leído no es el sembrado: %+v", g)
	}
	if g.RoleName != "Contador" {
		t.Errorf("el nombre del rol debe venir del JOIN de control, salió %q", g.RoleName)
	}
}
