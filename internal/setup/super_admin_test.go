package setup

// super_admin_test.go — Recuperar la credencial del super administrador sin
// repartir el rol.
//
// El caso que se protege: el panel muestra al super administrador pero no lo
// toca; la salida es el binario en modo --setup. Ese camino tiene que dejar UNA
// sola identidad con el rol reservado, restablecer SU clave y no convertir a
// cualquiera en dueño del sistema por escribir su nombre en la consola.

import (
	"database/sql"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	_ "github.com/glebarez/go-sqlite"
)

func baseDeSuperAdmin(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	for _, ddl := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, first_name TEXT,
			last_name TEXT, company_name TEXT, phone TEXT, status INTEGER)`,
		`CREATE TABLE user_login (user_id TEXT PRIMARY KEY, username TEXT UNIQUE, password TEXT,
			user_type INTEGER, status INTEGER)`,
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT, level TEXT,
			description TEXT, is_system INTEGER DEFAULT 0)`,
		`CREATE TABLE sec_userrole (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, roleid INTEGER,
			createby TEXT, createdate TEXT)`,
		`CREATE TABLE tenant_users (tenant_code TEXT, username TEXT, role_id INTEGER, is_owner INTEGER,
			status INTEGER, created_at TEXT, PRIMARY KEY (tenant_code, username))`,
	} {
		if _, err := db.Exec(strings.Join(strings.Fields(ddl), " ")); err != nil {
			t.Fatalf("crear tabla: %v", err)
		}
	}
	return db
}

func TestRestablecerCreaElSuperAdminCuandoNoHay(t *testing.T) {
	db := baseDeSuperAdmin(t)
	nombre, err := RestablecerCredencialSuperAdmin(db, "default", "jefe", "clave-fuerte")
	if err != nil {
		t.Fatalf("restableciendo: %v", err)
	}
	if nombre != "jefe" {
		t.Fatalf("usuario devuelto = %q, se esperaba jefe", nombre)
	}

	var userID, hash string
	if err := db.QueryRow(`SELECT user_id, password FROM user_login WHERE username = 'jefe'`).
		Scan(&userID, &hash); err != nil {
		t.Fatalf("no se creó la identidad: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("clave-fuerte")) != nil {
		t.Error("la contraseña guardada no es la elegida")
	}
	var nivel string
	if err := db.QueryRow(`SELECT r.level FROM sec_userrole ur JOIN sec_role r ON r.id = ur.roleid
		WHERE ur.user_id = ?`, userID).Scan(&nivel); err != nil {
		t.Fatalf("no se asignó el rol: %v", err)
	}
	if nivel != nivelSuperAdmin {
		t.Errorf("el rol asignado debe ser master: %q", nivel)
	}
	var miembro int
	_ = db.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code = 'default' AND username = 'jefe'`).Scan(&miembro)
	if miembro != 1 {
		t.Error("el super administrador debe quedar como miembro de la empresa maestra")
	}
}

func TestRestablecerNoReparteElRolAOtroUsuario(t *testing.T) {
	db := baseDeSuperAdmin(t)
	if _, err := db.Exec(`INSERT INTO sec_role (id, type, level) VALUES (1, 'Super Admin', 'master')`); err != nil {
		t.Fatal(err)
	}
	for _, u := range [][2]string{{"1", "admin"}, {"2", "otro"}} {
		if _, err := db.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES (?, 'X', 'Y', 1)`, u[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES (?, ?, 'vieja', 2, 1)`, u[0], u[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES ('1', 1)`); err != nil {
		t.Fatal(err)
	}

	// Se pide por «otro»: aun así debe restablecerse el super administrador que ya
	// existe (admin), no promover a otro.
	nombre, err := RestablecerCredencialSuperAdmin(db, "default", "otro", "nueva-clave")
	if err != nil {
		t.Fatalf("restableciendo: %v", err)
	}
	if nombre != "admin" {
		t.Errorf("se restableció %q, se esperaba al super administrador existente (admin)", nombre)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE user_id = '2'`).Scan(&n)
	if n != 0 {
		t.Error("el otro usuario no debe recibir el rol reservado")
	}
	var hash string
	_ = db.QueryRow(`SELECT password FROM user_login WHERE username = 'admin'`).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("nueva-clave")) != nil {
		t.Error("la clave del super administrador no se restableció")
	}
}

func TestRestablecerDejaUnSoloSuperAdmin(t *testing.T) {
	db := baseDeSuperAdmin(t)
	if _, err := db.Exec(`INSERT INTO sec_role (id, type, level) VALUES (1, 'Super Admin', 'master')`); err != nil {
		t.Fatal(err)
	}
	for _, u := range [][2]string{{"1", "primero"}, {"2", "segundo"}} {
		if _, err := db.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES (?, 'X', 'Y', 1)`, u[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status) VALUES (?, ?, 'vieja', 1, 1)`, u[0], u[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES (?, 1)`, u[0]); err != nil {
			t.Fatal(err)
		}
	}

	nombre, err := RestablecerCredencialSuperAdmin(db, "default", "", "clave-nueva")
	if err != nil {
		t.Fatalf("restableciendo: %v", err)
	}
	if nombre != "primero" {
		t.Errorf("se conserva el más antiguo: %q", nombre)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE roleid = 1`).Scan(&n)
	if n != 1 {
		t.Fatalf("debe quedar una sola asignación: %d", n)
	}
}

func TestRestablecerRechazaClaveVacia(t *testing.T) {
	db := baseDeSuperAdmin(t)
	if _, err := RestablecerCredencialSuperAdmin(db, "default", "jefe", "   "); err == nil {
		t.Error("una contraseña vacía no puede aceptarse")
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_login`).Scan(&n)
	if n != 0 {
		t.Error("un intento rechazado no debe dejar identidades")
	}
}
