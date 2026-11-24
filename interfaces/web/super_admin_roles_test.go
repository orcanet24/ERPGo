package web

// super_admin_roles_test.go — El dueño de la instalación no puede quedar con
// dos roles, uno de ellos de menor nivel.
//
// El fallo real: una instalación sembrada por el asistente anterior dejó al
// usuario 1 con el master (id 1) y un manager (id 3). El login resolvía el
// nivel con un LIMIT 1 y podía devolver el manager: el dueño entraba sin
// matriz de permisos y solo veía Dashboard + Gestion Inventario. El arranque
// debe dejar SOLO el master y corregir la sesión ya abierta.

import (
	"net/http"
	"testing"

	"erpgo/infrastructure/persistence"

	"golang.org/x/crypto/bcrypt"
)

func TestNormalizarSuperAdminRetiraRolesAjenos(t *testing.T) {
	db, err := persistence.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("abrir SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, ddl := range []string{
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY, type TEXT, level TEXT,
			description TEXT, is_system INTEGER DEFAULT 0)`,
		`CREATE TABLE sec_userrole (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT,
			roleid INTEGER, createby TEXT, createdate TEXT)`,
		`CREATE TABLE user_login (user_id TEXT, username TEXT)`,
		`CREATE TABLE web_sessions (id TEXT PRIMARY KEY, user_id TEXT, user_level TEXT)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla: %v", err)
		}
	}

	// El estado roto: el dueño quedó atado al manager (id 3) y al master (id 1).
	if _, err := db.Exec(`INSERT INTO sec_role (id, type, level) VALUES
		(1, 'Super Admin', 'master'), (3, 'Manager', 'manager')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_login (user_id, username) VALUES ('1','admin'), ('2','otro')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES
		('1', 3), ('1', 1), ('2', 3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO web_sessions (id, user_id, user_level) VALUES ('s1','1','manager')`); err != nil {
		t.Fatal(err)
	}

	if err := normalizarSuperAdmin(db); err != nil {
		t.Fatalf("normalizando: %v", err)
	}

	var filas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE user_id = '1'`).Scan(&filas); err != nil {
		t.Fatal(err)
	}
	if filas != 1 {
		t.Errorf("el superadministrador debe quedar con un solo rol, tiene %d", filas)
	}
	var nivel string
	if err := db.QueryRow(`SELECT r.level FROM sec_userrole ur JOIN sec_role r ON r.id = ur.roleid
		WHERE ur.user_id = '1'`).Scan(&nivel); err != nil {
		t.Fatalf("el superadministrador quedó sin rol: %v", err)
	}
	if nivel != nivelSuperAdmin {
		t.Errorf("el rol que queda debe ser master, es %q", nivel)
	}

	// La sesión ya abierta se corrige: sin esto el arreglo no se vería hasta
	// que caducara.
	var nivelSesion string
	if err := db.QueryRow(`SELECT user_level FROM web_sessions WHERE id = 's1'`).Scan(&nivelSesion); err != nil {
		t.Fatal(err)
	}
	if nivelSesion != nivelSuperAdmin {
		t.Errorf("la sesión del dueño debe pasar a master, quedó %q", nivelSesion)
	}

	// Un usuario normal con su rol de manager no se toca: la limpieza es del
	// superadministrador, no de todos.
	var rolOtro int
	if err := db.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = '2'`).Scan(&rolOtro); err != nil {
		t.Fatalf("se perdió el rol de un usuario normal: %v", err)
	}
	if rolOtro != 3 {
		t.Errorf("el rol del usuario normal cambió a %d", rolOtro)
	}
}

// TestLoginNoDejaQueUnRolDeEmpresaTapeElMaster reproduce el síntoma tal como lo
// vivía el dueño: la empresa le devolvía un manager (LIMIT 1 sobre dos filas) y
// el login se quedaba con ese nivel aunque el catálogo de control le concediera
// el master. El master es GLOBAL: ninguna asignación de empresa puede rebajarlo.
func TestLoginNoDejaQueUnRolDeEmpresaTapeElMaster(t *testing.T) {
	db, err := persistence.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("abrir SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, ddl := range []string{
		`CREATE TABLE users (user_id INTEGER PRIMARY KEY, first_name TEXT,
			last_name TEXT, logo TEXT)`,
		`CREATE TABLE user_login (user_id TEXT, username TEXT, user_type INTEGER,
			password TEXT, status INTEGER DEFAULT 1)`,
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY, type TEXT, level TEXT,
			description TEXT, is_system INTEGER DEFAULT 0)`,
		`CREATE TABLE sec_userrole (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT,
			roleid INTEGER, createby TEXT, createdate TEXT)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla: %v", err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("clave"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generar hash: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, first_name, last_name) VALUES (7, 'Dueño', 'Sistema')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_login (user_id, username, user_type, password, status)
		VALUES ('7', 'jefe', 2, ?, 1)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sec_role (id, type, level) VALUES
		(1, 'Super Admin', 'master'), (3, 'Manager', 'manager')`); err != nil {
		t.Fatal(err)
	}
	// El manager entra PRIMERO a propósito: es la fila que el LIMIT 1 devolvía
	// en la instalación rota, y la que antes tapaba al master.
	if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES ('7', 3), ('7', 1)`); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: db, controlDB: db, controlListo: true}
	sess, err := s.authenticateUser(&http.Request{}, "jefe", "clave")
	if err != nil {
		t.Fatalf("el dueño no pudo entrar: %v", err)
	}
	if sess.UserLevel != nivelSuperAdmin {
		t.Errorf("el login devolvió %q; el master de control no puede quedar tapado por un rol de empresa", sess.UserLevel)
	}

	// Control negativo: sin la fila de control, el mismo fixture resuelve el
	// manager de la empresa. Así se demuestra que la prueba no pasa por azar.
	if _, err := db.Exec(`DELETE FROM sec_userrole WHERE roleid = 1`); err != nil {
		t.Fatal(err)
	}
	sess, err = s.authenticateUser(&http.Request{}, "jefe", "clave")
	if err != nil {
		t.Fatalf("el usuario sin master debe seguir entrando: %v", err)
	}
	if sess.UserLevel != "manager" {
		t.Errorf("sin master el nivel debe ser el de la empresa (manager), es %q", sess.UserLevel)
	}
}
