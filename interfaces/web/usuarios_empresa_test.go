package web

// usuarios_empresa_test.go — De qué empresa es cada usuario y quién es el super
// administrador.
//
// POR QUÉ ESTA PRUEBA EXISTE.
//
// El alta de usuarios inscribía a la identidad en el PRIMER tenant activo
// (`SELECT code FROM tenants WHERE status = 1 ORDER BY id LIMIT 1`), así que todo
// usuario creado desde una empresa terminaba en la maestra o en la demo. Y la
// lista de Usuarios no filtraba: cada empresa veía las identidades de todas, con
// lo que un usuario «saltaba» de una a otra. Además el desplegable de rol ofrecía
// el del super administrador, y cualquier administrador podía fabricar un segundo
// dueño del sistema.
//
// Las pruebas trabajan con una base de control como la de una instalación con
// varias empresas: `default` (la primera por id, la que elegía el alta anterior)
// y `acme` (la que tiene la sesión). Si el alta vuelve a mirar la primera, el
// usuario aparece en `default` y la prueba se rompe.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"erpgo/configs"

	_ "github.com/glebarez/go-sqlite"
)

// baseDeUsuarios crea la base de control mínima: identidad, roles, pertenencia y
// el catálogo de empresas. No se aplica el esquema completo porque esta prueba es
// del reparto de identidades, no del esquema.
func baseDeUsuarios(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base de control: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	tablas := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, first_name TEXT,
			last_name TEXT, phone TEXT, company_name TEXT, status INTEGER)`,
		`CREATE TABLE user_login (user_id TEXT PRIMARY KEY, username TEXT UNIQUE, password TEXT,
			user_type INTEGER, status INTEGER)`,
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT, level TEXT,
			description TEXT, is_system INTEGER DEFAULT 0)`,
		`CREATE TABLE sec_userrole (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, roleid INTEGER,
			createby TEXT, createdate TEXT)`,
		`CREATE TABLE tenant_users (tenant_code TEXT, username TEXT, role_id INTEGER, is_owner INTEGER,
			status INTEGER, created_at TEXT, PRIMARY KEY (tenant_code, username))`,
		`CREATE TABLE user_company_access (user_id TEXT, tenant_code TEXT, role_id INTEGER)`,
		`CREATE TABLE tenants (id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT, legal_name TEXT,
			status INTEGER)`,
	}
	for _, ddl := range tablas {
		if _, err := db.Exec(strings.Join(strings.Fields(ddl), " ")); err != nil {
			t.Fatalf("crear tabla de control: %v", err)
		}
	}
	// `default` primero por id: es la empresa que elegía el alta anterior.
	for _, ddl := range []string{
		`INSERT INTO tenants (id, code, legal_name, status) VALUES (1, 'default', 'Empresa maestra', 1)`,
		`INSERT INTO tenants (id, code, legal_name, status) VALUES (2, 'acme', 'ACME C.A.', 1)`,
		`INSERT INTO sec_role (id, type, level) VALUES (1, 'Super Admin', 'master')`,
		`INSERT INTO sec_role (id, type, level) VALUES (2, 'Admin', 'admin')`,
		`INSERT INTO sec_role (id, type, level) VALUES (3, 'Manager', 'manager')`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("sembrando la base de control: %v", err)
		}
	}
	return db
}

// servidorDeUsuarios monta el Server con esa base como control y como empresa
// activa, igual que servidorDoble: `controlListo` evita que se intente aplicar el
// esquema MySQL sobre SQLite.
func servidorDeUsuarios(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	ctl := baseDeUsuarios(t)
	s := &Server{
		config:       &configs.Config{AppEnv: "test", AppName: "ERPGo"},
		db:           ctl,
		controlDB:    ctl,
		controlListo: true,
		sessions:     NewSessionStore(),
		activeTenant: "acme",
	}
	return s, ctl
}

// peticionDeUsuario envía el formulario con la sesión de la empresa indicada.
func peticionDeUsuario(s *Server, empresa string, form url.Values) *http.Request {
	r := httptest.NewRequest("POST", "/user/save", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	sess := &SessionData{UserID: "7", Username: "admin", Fullname: "Admin", UserLevel: "admin", TenantCode: empresa}
	return withScope(r, &scope{tenant: empresa, db: s.db, sess: sess})
}

func TestAltaDeUsuarioPerteneceALaEmpresaDeLaSesion(t *testing.T) {
	s, ctl := servidorDeUsuarios(t)
	form := url.Values{
		"first_name": {"Ana"}, "last_name": {"Pérez"}, "username": {"ana"},
		"password": {"clave-segura"}, "user_type": {"2"}, "role_id": {"3"},
	}
	w := httptest.NewRecorder()
	s.userSave(w, peticionDeUsuario(s, "acme", form))
	if w.Code != 200 {
		t.Fatalf("el alta respondió %d: %s", w.Code, w.Body.String())
	}

	var userID string
	if err := ctl.QueryRow(`SELECT user_id FROM user_login WHERE username = 'ana'`).Scan(&userID); err != nil {
		t.Fatalf("el usuario no se creó: %v", err)
	}

	// La pertenencia es la de la sesión, no la del primer tenant por id.
	var enAcme, enDefault int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code = 'acme' AND username = 'ana'`).Scan(&enAcme)
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code = 'default' AND username = 'ana'`).Scan(&enDefault)
	if enAcme != 1 {
		t.Errorf("el usuario debe pertenecer a la empresa de la sesión (acme): %d", enAcme)
	}
	if enDefault != 0 {
		t.Errorf("el usuario no debe caer en la empresa maestra/demo: %d", enDefault)
	}
	var accesos int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM user_company_access WHERE user_id = ? AND tenant_code = 'acme'`, userID).Scan(&accesos)
	if accesos != 1 {
		t.Errorf("falta el acceso del usuario a la empresa de la sesión: %d", accesos)
	}

	// Y el rol que se eligió se guarda: antes se llamaba con user_id vacío por una
	// declaración corta que tapaba la variable y el usuario nacía sin rol.
	var roleID int
	if err := ctl.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = ?`, userID).Scan(&roleID); err != nil {
		t.Fatalf("el usuario debe tener el rol elegido: %v", err)
	}
	if roleID != 3 {
		t.Errorf("rol guardado = %d, se esperaba 3", roleID)
	}
}

func TestNoSePuedeAsignarElRolDelSuperAdministrador(t *testing.T) {
	s, ctl := servidorDeUsuarios(t)
	form := url.Values{
		"first_name": {"Mallory"}, "username": {"mallory"},
		"password": {"clave-segura"}, "user_type": {"2"}, "role_id": {"1"},
	}
	w := httptest.NewRecorder()
	s.userSave(w, peticionDeUsuario(s, "acme", form))
	if w.Code == 200 {
		t.Fatalf("el alta debía rechazar el rol de super administrador: %s", w.Body.String())
	}
	var n int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM user_login WHERE username = 'mallory'`).Scan(&n)
	if n != 0 {
		t.Error("un alta rechazada no debe dejar la identidad creada")
	}
}

func TestNoSePuedeEditarUnUsuarioDeOtraEmpresa(t *testing.T) {
	s, ctl := servidorDeUsuarios(t)
	// Usuario que solo pertenece a la maestra.
	if _, err := ctl.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES ('9','Otro','Usuario',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES ('9','ajeno','x',2,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO tenant_users (tenant_code, username, is_owner, status) VALUES ('default','ajeno',0,1)`); err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"user_id": {"9"}, "first_name": {"Cambiado"}, "username": {"ajeno"},
		"user_type": {"2"},
	}
	w := httptest.NewRecorder()
	s.userSave(w, peticionDeUsuario(s, "acme", form))
	if w.Code != http.StatusForbidden {
		t.Fatalf("editar un usuario de otra empresa debía dar 403, dio %d: %s", w.Code, w.Body.String())
	}
	var nombre string
	_ = ctl.QueryRow(`SELECT first_name FROM users WHERE user_id = '9'`).Scan(&nombre)
	if nombre != "Otro" {
		t.Errorf("el usuario de otra empresa no debe cambiar: %q", nombre)
	}
}

func TestBorrarUsuarioDeDosEmpresasSoloQuitaEsaEmpresa(t *testing.T) {
	s, ctl := servidorDeUsuarios(t)
	if _, err := ctl.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES ('11','Doble','Empresa',2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES ('11','doble','x',2,1)`); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"acme", "default"} {
		if _, err := ctl.Exec(`INSERT INTO tenant_users (tenant_code, username, is_owner, status) VALUES (?, 'doble', 0, 1)`, code); err != nil {
			t.Fatal(err)
		}
	}

	r := httptest.NewRequest("POST", "/user/delete/11", nil)
	// PathValue solo se rellena al pasar por el mux: al llamar al handler directo
	// hay que fijarlo, o el handler cree que no le dieron id.
	r.SetPathValue("id", "11")
	sess := &SessionData{UserID: "7", Username: "admin", UserLevel: "admin", TenantCode: "acme"}
	r = withScope(r, &scope{tenant: "acme", db: ctl, sess: sess})
	w := httptest.NewRecorder()
	s.userDelete(w, r)
	if w.Code != 200 {
		t.Fatalf("quitar el acceso debía responder 200, dio %d: %s", w.Code, w.Body.String())
	}

	var n int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE username = 'doble' AND tenant_code = 'acme'`).Scan(&n)
	if n != 0 {
		t.Error("debe quedar sin acceso a la empresa donde se lo quitaron")
	}
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE username = 'doble' AND tenant_code = 'default'`).Scan(&n)
	if n != 1 {
		t.Error("no debe perder el acceso a la otra empresa")
	}
	var existe int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM user_login WHERE username = 'doble'`).Scan(&existe)
	if existe != 1 {
		t.Error("la identidad debe seguir existiendo mientras tenga otra empresa")
	}
}

func TestNoSeBorraNiSeEditaAlSuperAdministrador(t *testing.T) {
	s, ctl := servidorDeUsuarios(t)
	if _, err := ctl.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES ('1','Súper','Admin',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES ('1','admin','x',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES ('1', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO tenant_users (tenant_code, username, is_owner, status) VALUES ('default','admin',1,1)`); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("POST", "/user/delete/1", nil)
	r.SetPathValue("id", "1")
	sess := &SessionData{UserID: "7", Username: "admin2", UserLevel: "admin", TenantCode: "default"}
	r = withScope(r, &scope{tenant: "default", db: ctl, sess: sess})
	w := httptest.NewRecorder()
	s.userDelete(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("borrar al super administrador debía dar 403, dio %d", w.Code)
	}

	form := url.Values{"user_id": {"1"}, "first_name": {"Otro"}, "username": {"admin"}, "user_type": {"1"}}
	w = httptest.NewRecorder()
	s.userSave(w, peticionDeUsuario(s, "default", form))
	if w.Code != http.StatusForbidden {
		t.Fatalf("editar al super administrador debía dar 403, dio %d", w.Code)
	}
	var nombre string
	_ = ctl.QueryRow(`SELECT first_name FROM users WHERE user_id = '1'`).Scan(&nombre)
	if nombre != "Súper" {
		t.Errorf("el super administrador no debe cambiar desde el panel: %q", nombre)
	}
}

func TestNormalizarSuperAdminDejaUnoSolo(t *testing.T) {
	db := baseDeUsuarios(t)
	for _, fila := range [][2]string{{"1", "primero"}, {"2", "segundo"}} {
		if _, err := db.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES (?, 'X', 'Y', 1)`, fila[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES (?, ?, 'x', 1, 1)`, fila[0], fila[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES (?, 1)`, fila[0]); err != nil {
			t.Fatal(err)
		}
	}
	if err := normalizarSuperAdmin(db); err != nil {
		t.Fatalf("normalizando: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE roleid = 1`).Scan(&n)
	if n != 1 {
		t.Fatalf("debe quedar una sola asignación del rol: %d", n)
	}
	var queda string
	_ = db.QueryRow(`SELECT user_id FROM sec_userrole WHERE roleid = 1`).Scan(&queda)
	if queda != "1" {
		t.Errorf("debe quedarse la asignación más antigua (user 1), quedó %q", queda)
	}
}

func TestElRolReservadoNoSeReasignaNiSeCreaMaster(t *testing.T) {
	db := baseDeUsuarios(t)
	if _, err := db.Exec(`INSERT INTO sec_role (type, level) VALUES ('Ventas', 'master')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rolElegidoParaUsuario(db, "1"); err == nil {
		t.Error("el rol reservado no debe poder elegirse")
	}
	var idMaster int
	_ = db.QueryRow(`SELECT id FROM sec_role WHERE type = 'Ventas'`).Scan(&idMaster)
	if _, err := rolElegidoParaUsuario(db, strconv.Itoa(idMaster)); err == nil {
		t.Error("un rol nuevo con nivel master tampoco debe poder asignarse")
	}
	if id, err := rolElegidoParaUsuario(db, "3"); err != nil || id != 3 {
		t.Errorf("un rol normal debe aceptarse: id=%d err=%v", id, err)
	}
	if id, err := rolElegidoParaUsuario(db, ""); err != nil || id != 0 {
		t.Errorf("sin rol debe devolver 0 sin error: id=%d err=%v", id, err)
	}
}

func TestRepararPertenenciasHuerfanasDevuelveElAccesoALaEmpresa(t *testing.T) {
	db := baseDeUsuarios(t)
	// El alta anterior dejaba SOLO user_company_access: sin tenant_users el login
	// no reconoce la pertenencia y el usuario no puede entrar a la empresa.
	if _, err := db.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES ('21','Viejo','Alta',2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES ('21','viejo','x',2,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_company_access (user_id, tenant_code, role_id) VALUES ('21','acme',3)`); err != nil {
		t.Fatal(err)
	}

	n, err := repararPertenenciasHuerfanas(db)
	if err != nil {
		t.Fatalf("reparando: %v", err)
	}
	if n != 1 {
		t.Fatalf("debe reparar una pertenencia: %d", n)
	}
	var rol int
	if err := db.QueryRow(`SELECT role_id FROM tenant_users WHERE tenant_code='acme' AND username='viejo'`).Scan(&rol); err != nil {
		t.Fatalf("no se creó la pertenencia: %v", err)
	}
	if rol != 3 {
		t.Errorf("el rol de la pertenencia debe conservarse: %d", rol)
	}

	// Idempotente: la segunda pasada no duplica ni vuelve a contar.
	n, err = repararPertenenciasHuerfanas(db)
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if n != 0 {
		t.Errorf("la reparación debe ser idempotente, reparó %d", n)
	}
	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code='acme' AND username='viejo'`).Scan(&total)
	if total != 1 {
		t.Errorf("no debe duplicar la pertenencia: %d", total)
	}
}
