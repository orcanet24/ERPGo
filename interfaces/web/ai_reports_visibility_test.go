package web

// Tests de visibilidad y guardado de reportes creados por IA.
//
// La tabla report_template_acl no existe en instalaciones sin aplicar la
// migración; estos tests la crean explícitamente sobre una base SQLite de
// prueba. La visibilidad se valida con reportCanView y loadReportTemplates.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"erpgo/configs"
)

// baseReportes extiende basePrueba con report_template + report_template_acl y
// una semilla de roles para probar visibilidad.
func baseReportes(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := basePrueba(t)
	s.config = &configs.Config{AppEnv: "test", AppName: "ERPGo"}
	ddl := `
	CREATE TABLE IF NOT EXISTS report_template (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(120) NOT NULL,
		description VARCHAR(250) NOT NULL DEFAULT '',
		definition MEDIUMTEXT NOT NULL,
		created_by VARCHAR(60) NOT NULL DEFAULT '',
		status TINYINT NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS report_template_acl (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		report_template_id INT NOT NULL,
		role_id INT NULL,
		user_id INT NULL
	);`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("create report tables: %v", err)
	}
	for _, r := range []struct {
		id    int
		tipo  string
		level string
	}{{1, "manager", "manager"}, {2, "employee", "employee"}} {
		if _, err := db.Exec(`INSERT INTO sec_role (id, type, level) VALUES (?,?,?)`, r.id, r.tipo, r.level); err != nil {
			t.Fatalf("insert sec_role: %v", err)
		}
	}
	// sec_userrole: "7" → role 1 (manager), "99" → role 2 (employee). Sin esto
	// sessionRoleID devuelve 0 y la visibilidad no se filtra por rol.
	for _, u := range []struct {
		uid  string
		role int
	}{{"7", 1}, {"99", 2}} {
		if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES (?,?)`, u.uid, u.role); err != nil {
			t.Fatalf("insert sec_userrole: %v", err)
		}
	}
	return s, db
}

// insertTestReport inserta un reporte de prueba y devuelve su id.
func insertTestReport(t *testing.T, s *Server, name, definition string) int64 {
	t.Helper()
	res, err := s.db.Exec(`INSERT INTO report_template (name, description, definition, created_by, status, created_at, updated_at)
		VALUES (?,?,?,1,1,'2026-01-01','2026-01-01')`, name, "test", definition)
	if err != nil {
		t.Fatalf("insert report_template: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestReportACLVisibilidad verifica la regla:
//   - Sin filas en report_template_acl → todos ven.
//   - Con filas → solo los roles/usuarios que coinciden ven.
func TestReportACLVisibilidad(t *testing.T) {
	s, _ := baseReportes(t)
	def := `{"title":"test","sql":"SELECT 1 AS x","columns":[{"key":"x","label":"X"}],"totals":[]}`
	idAll := insertTestReport(t, s, "Publico", def)
	idRole1 := insertTestReport(t, s, "Solo managers", def)

	// Asignar ACL: role_id=1 (managers) ve idRole1
	if _, err := s.db.Exec(`INSERT INTO report_template_acl (report_template_id, role_id, user_id) VALUES (?,1,NULL)`, idRole1); err != nil {
		t.Fatal(err)
	}

	// Usuario con role_id=1 (manager) → ve ambos.
	mgr := &SessionData{UserID: "7", UserLevel: "manager"}
	if !s.reportCanView(mgr, idAll) {
		t.Error("el manager debe ver el reporte público (sin ACL)")
	}
	if !s.reportCanView(mgr, idRole1) {
		t.Error("el manager (role 1) debe ver el reporte restringido a managers")
	}

	// Usuario con role_id=2 (employee) → ve idAll pero NO idRole1.
	emp := &SessionData{UserID: "99", UserLevel: "employee"}
	if !s.reportCanView(emp, idAll) {
		t.Error("el employee debe ver el reporte público")
	}
	if s.reportCanView(emp, idRole1) {
		t.Error("el employee NO debe ver el reporte restringido a managers")
	}

	// loadReportTemplates filtra: el employee solo ve "Publico".
	empList := s.loadReportTemplates(emp)
	if len(empList) != 1 || empList[0].Name != "Publico" {
		t.Errorf("employee debe ver 1 reporte ('Publico'); vio %d:", len(empList))
		for _, r := range empList {
			t.Logf("  - %s", r.Name)
		}
	}

	// El manager ve ambos.
	mgrList := s.loadReportTemplates(mgr)
	if len(mgrList) != 2 {
		t.Errorf("manager debe ver 2 reportes; vio %d", len(mgrList))
	}
}

// TestReportTemplateRolesDevuelveRoles: GET /reports/custom/roles.
func TestReportTemplateRolesDevuelveRoles(t *testing.T) {
	s, _ := baseReportes(t)
	req := httptest.NewRequest(http.MethodGet, "/reports/custom/roles", nil)
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("manager")})
	res := httptest.NewRecorder()
	s.reportTemplateRoles(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("esperaba 200; llegó %d: %s", res.Code, res.Body.String())
	}
	var roles []map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &roles); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("esperaba 2 roles; llegaron %d", len(roles))
	}
}

// TestReportSaveAceptaDefinitionValida: ai-save guarda con definición válida.
// La definición usa SELECT 1 (sin tabla) para ser independiente del esquema.
func TestReportSaveAceptaDefinitionValida(t *testing.T) {
	s, _ := baseReportes(t)
	def := `{"title":"Simple","sql":"SELECT 1 AS x","columns":[{"key":"x","label":"X"}],"totals":[]}`
	body, _ := json.Marshal(map[string]interface{}{
		"name": "Informe simple", "description": "test", "definition": def, "role_ids": []int64{},
	})
	req := httptest.NewRequest(http.MethodPost, "/reports/custom/ai-save", io.NopCloser(bytes.NewReader(body)))
	req = withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("admin")})
	res := httptest.NewRecorder()
	s.reportTemplateAISave(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("esperaba 200; llegó %d: %s", res.Code, res.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(res.Body.Bytes(), &resp)
	if resp["status"] != float64(1) {
		t.Errorf("esperaba status=1; salió: %+v", resp)
	}
}
