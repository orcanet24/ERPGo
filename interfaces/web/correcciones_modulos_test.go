package web

// Pruebas de las correcciones del sistema de módulos y de la salida de avisos.
//
// Ninguna es cosmética. La primera evita que un aviso por WhatsApp (pasarela)
// se registre contra un correo; la segunda, que un gerente apague o encienda
// módulos; la tercera, que el interruptor parezca roto durante 30 segundos; la
// cuarta, que un módulo apagado siga en la barra lateral. Y la cuarta tenía una
// trampa: el `LEFT JOIN` mal escrito borraba TODO el menú, porque
// `menu.module_id` vale 'core' y no hay fila 'core' en `modules`.
//
// Los módulos que aquí sirven de datos son uno inventado: estas conductas son
// del núcleo, no de ningún módulo en particular.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- 1. El destinatario de un canal telefónico es el móvil ----------

func TestDestinatarioDeCanalTelefonicoEsElMovilYNoElCorreo(t *testing.T) {
	db := abrirBasePrueba(t)
	if _, err := db.Exec(`CREATE TABLE customer_information (customer_id INTEGER PRIMARY KEY,
		customer_name TEXT, customer_email TEXT, email_address TEXT, customer_mobile TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name, customer_email, customer_mobile)
		VALUES (1, 'Cliente', 'cliente@ejemplo.com', '0414-1234567')`); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: db}
	alerta := map[string]interface{}{"party_type": "customer", "party_id": int64(1)}

	destino, motivo := s.finDestinatario(db, notifyChannel{Kind: "wapp"}, alerta)
	if motivo != "" {
		t.Fatalf("no debía fallar, dijo: %s", motivo)
	}
	if destino != "0414-1234567" {
		t.Fatalf("el destino fue %q; debe ser el móvil de la ficha, no el correo", destino)
	}

	// El correo sigue siendo el correo: la corrección no puede cambiar el otro
	// camino.
	destino, _ = s.finDestinatario(db, notifyChannel{Kind: "email"}, alerta)
	if destino != "cliente@ejemplo.com" {
		t.Fatalf("el canal de correo cambió de destinatario: %q", destino)
	}
}

// ---------- 2. /module es del super administrador ----------

func TestActivarModulosExigeMasterAunqueElRolTengaConfiguracion(t *testing.T) {
	s, db := basePrueba(t)
	s.sessions = NewSessionStore()

	ddl := []string{
		`CREATE TABLE user_company_access (user_id TEXT, tenant_code TEXT, role_id INTEGER)`,
		`CREATE TABLE modules (id TEXT PRIMARY KEY, is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 0)`,
		`INSERT INTO sec_role (id, type, level) VALUES (3, 'gerente', 'manager')`,
		`INSERT INTO sec_userrole (user_id, roleid) VALUES ('usuario', 3)`,
		`INSERT INTO sub_module (id, directory, status) VALUES (99, 'setting', 1)`,
		`INSERT INTO role_permission (role_id, fk_module_id, "create", "read", "update", "delete") VALUES (3, 99, 1, 1, 1, 1)`,
		`INSERT INTO user_company_access (user_id, tenant_code, role_id) VALUES ('usuario', 'default', 3)`,
		`INSERT INTO user_login (username, password) VALUES ('usuario', 'hash')`,
		`INSERT INTO modules (id, is_core, is_active) VALUES ('demo', 0, 0)`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar base: %v\n%s", err, q)
		}
	}

	sess := sessPrueba("manager")
	sess.UserID = "usuario"
	sess.Username = "usuario"
	sess.TenantCode = defaultTenantCode
	sess.ExpiresAt = time.Now().Add(time.Hour)
	sess.CSRFToken = "token-de-prueba"
	id := s.sessions.Create(sess)

	mux := http.NewServeMux()
	s.mux = mux
	s.registerModuleRoutes()
	handler := s.authMiddleware(s.permissionMiddleware(mux))

	req := httptest.NewRequest(http.MethodPost, "/module/demo/toggle", nil)
	req.AddCookie(&http.Cookie{Name: "erpgow_session", Value: id})
	req.Header.Set(csrfHeader, sess.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("un gerente con permiso de configuración obtuvo HTTP %d; /module debe ser solo del master", res.Code)
	}
	if !strings.Contains(res.Body.String(), "super admin") {
		t.Fatalf("el rechazo no dice que hace falta el super administrador: %s", res.Body.String())
	}

	var activo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = 'demo'`).Scan(&activo); err != nil {
		t.Fatal(err)
	}
	if activo != 0 {
		t.Fatal("el módulo cambió de estado pese al 403: la comprobación llegó tarde")
	}
}

// ---------- 3 y 4. El menú y su caché ----------

func baseDeMenuConModulo(t *testing.T) (*Server, func(string, int)) {
	t.Helper()
	db := abrirBasePrueba(t)
	ddl := []string{
		`ALTER TABLE menu ADD COLUMN module_id TEXT DEFAULT 'core'`,
		`CREATE TABLE modules (id TEXT PRIMARY KEY, is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 1)`,
		`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id)
		 VALUES (1, NULL, 'Dashboard', '/home', NULL, NULL, NULL, 10, 1, 'core')`,
		`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id)
		 VALUES (2, NULL, 'Demo', NULL, NULL, NULL, NULL, 20, 1, 'demo')`,
		`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id)
		 VALUES (3, 2, 'Pantalla', '/demo', NULL, NULL, '/demo', 10, 1, 'demo')`,
		`INSERT INTO modules (id, is_core, is_active) VALUES ('demo', 0, 0)`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar menú: %v\n%s", err, q)
		}
	}
	s := &Server{db: db, controlDB: db, controlListo: true}
	cambiar := func(estado string, activo int) {
		if _, err := db.Exec(`UPDATE modules SET is_active = ? WHERE id = ?`, activo, estado); err != nil {
			t.Fatal(err)
		}
	}
	return s, cambiar
}

func TestMenuOcultaUnModuloApagadoYConservaElMenuCore(t *testing.T) {
	limpiarCachesDeRender()
	s, cambiar := baseDeMenuConModulo(t)
	r := reqPrueba(s, sessPrueba("master"), "/home")

	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 1 || menu[0].Label != "Dashboard" {
		t.Fatalf("con el módulo apagado el menú debía quedar solo con Dashboard; quedó: %+v", menu)
	}

	cambiar("demo", 1)
	limpiarCachesDeRender()

	menu = s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 2 {
		t.Fatalf("con el módulo encendido debían verse Dashboard y Demo; quedó: %+v", menu)
	}
	if menu[1].Label != "Demo" || len(menu[1].Children) != 1 {
		t.Fatalf("el grupo Demo no trajo su entrada hija: %+v", menu[1])
	}
}

func TestMenuCoreSinFilaEnModulosSigueVisible(t *testing.T) {
	// Este es el caso que rompía el JOIN ingenuo: todo el menú core tiene
	// module_id 'core' y no existe ninguna fila con ese id.
	limpiarCachesDeRender()
	s, _ := baseDeMenuConModulo(t)
	r := reqPrueba(s, sessPrueba("master"), "/home")
	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) == 0 {
		t.Fatal("el menú core desapareció: el filtro por módulo está tratando la ausencia de fila como apagado")
	}
}

func TestMenuSigueCompletoSiLaBaseNoTieneTablaDeModulos(t *testing.T) {
	// Instalaciones antiguas: `modules` aún no existe. La consulta con JOIN
	// falla y el menú NO puede quedarse vacío; se reintenta sin el JOIN.
	limpiarCachesDeRender()
	db := abrirBasePrueba(t)
	if _, err := db.Exec(`INSERT INTO menu (id, parent_id, label, url, sort_order, status)
		VALUES (1, NULL, 'Dashboard', '/home', 10, 1)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, controlDB: db, controlListo: true}
	r := reqPrueba(s, sessPrueba("master"), "/home")

	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 1 || menu[0].Label != "Dashboard" {
		t.Fatalf("sin tabla modules el menú debía seguir completo; quedó: %+v", menu)
	}
}

func TestToggleDeModuloInvalidaLaCacheDelMenu(t *testing.T) {
	limpiarCachesDeRender()
	s, _ := baseDeMenuConModulo(t)
	r := reqPrueba(s, sessPrueba("master"), "/home")

	// Se pinta una vez: eso llena la caché de 30 s con el módulo apagado.
	if menu := s.menuTree(r, map[string]bool{}, "/home"); len(menu) != 1 {
		t.Fatalf("estado inicial inesperado: %+v", menu)
	}

	req := reqPrueba(s, sessPrueba("master"), "/module/demo/toggle")
	req.Method = http.MethodPost
	req.SetPathValue("id", "demo")
	res := httptest.NewRecorder()
	s.moduleToggle(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("el master no pudo activar el módulo: %d %s", res.Code, res.Body.String())
	}

	// Sin limpiar la caché a mano: el toggle tiene que haberla invalidado.
	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 2 {
		t.Fatalf("el menú siguió mostrando el estado viejo tras el toggle: %+v", menu)
	}
}
