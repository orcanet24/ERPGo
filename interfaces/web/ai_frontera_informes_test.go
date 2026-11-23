package web

// ai_frontera_informes_test.go — La frontera única de SQL de informes y la
// aprobación previa de lo crítico.
//
// Cada prueba fija uno de los agujeros que la auditoría 2026-11-22 midió en el
// subsistema IA/informes, escrita para VOLVER A FALLAR si el agujero reaparece:
//
//   - F2/S-4: `SELECT api_key FROM ai_providers` se guardaba y se ejecutaba,
//     porque la función estricta con whitelist de tablas estaba muerta y el
//     camino de informes solo miraba VERBOS. Aquí se rechaza en la frontera única
//     y también al ejecutar una plantilla ya guardada (los existentes quedan NO
//     EJECUTABLES, no se migran ni se borran).
//   - F4: el alcance departamental solo corría en db.query. Aquí se comprueba en
//     report.run_preview con el departamento de QUIEN EJECUTA.
//   - M-3: STRAIGHT_JOIN no estaba en el escáner de tablas y los comentarios
//     ejecutables `/*! ... */` se validaban fuera y se ejecutaban dentro. Aquí se
//     rechazan los dos.
//   - F5: la aprobación humana era POSTERIOR al efecto. Aquí el dispatcher de una
//     acción crítica no la ejecuta: deja propuesta pendiente, y un humano con el
//     permiso real sí puede aplicarla; sin el permiso, no.
//   - F9: no había corrida auditable de informe. Aquí se comprueba que el run
//     inserta report_run con su huella de definición y su usuario.
//
// Patrón del paquete: basePrueba/abrirBasePrueba (SQLite tolerante, con IFNULL),
// y las tablas que la migración aporta se crean a mano, como hacen
// informes_ia_crea_test.go y ai_reports_visibility_test.go.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// ---------- andamio ----------

// crearTablasInformePrueba crea las tablas que el cierre necesita y que el
// andamio de cachas_test.go no incluye: report_template (la lista de informes),
// report_run (corrida auditable, F9) y una tabla de negocio para que el SELECT
// tenga dónde correr.
func crearTablasInformePrueba(t *testing.T, db *sql.DB) {
	t.Helper()
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS report_template (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, description TEXT,
			definition TEXT, created_by TEXT, status INTEGER DEFAULT 1,
			created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS report_run (
			id INTEGER PRIMARY KEY AUTOINCREMENT, report_id INTEGER,
			definition_hash TEXT, sql TEXT, params TEXT, rows_count INTEGER,
			run_by TEXT, agent_id INTEGER NULL, tenant_code TEXT,
			created_at TEXT, exported_at TEXT NULL)`,
		`CREATE TABLE IF NOT EXISTS invoice (
			id INTEGER PRIMARY KEY AUTOINCREMENT, invoice_id INTEGER, customer_id INTEGER,
			date TEXT, total_amount REAL, invoice INTEGER, is_draft TEXT DEFAULT 'no',
			paid_amount REAL DEFAULT 0, due_amount REAL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS hr_salary_assignment (
			id INTEGER PRIMARY KEY AUTOINCREMENT, employee_id INTEGER, base_salary REAL)`,
	}
	for _, ddl := range ddls {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla de prueba: %v\n%s", err, ddl)
		}
	}
	if _, err := db.Exec(`INSERT INTO invoice (invoice_id, customer_id, date, total_amount, invoice)
		VALUES (1, 10, '2026-11-05', 100, 1), (2, 11, '2026-11-06', 250, 2)`); err != nil {
		t.Fatalf("sembrar invoice: %v", err)
	}
}

// crearTablasPropuestaPrueba crea ai_tool_proposal (migración
// schema_z_ai_report_run.sql) y la ai_task_run mínima que toca el botón de
// revisión.
func crearTablasPropuestaPrueba(t *testing.T, db *sql.DB) {
	t.Helper()
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS ai_tool_proposal (
			id INTEGER PRIMARY KEY AUTOINCREMENT, idem_key TEXT UNIQUE, tool TEXT,
			args_json TEXT, run_id INTEGER DEFAULT 0, task_id INTEGER DEFAULT 0,
			agent_id INTEGER DEFAULT 0, status TEXT DEFAULT 'pending', reason TEXT,
			result_json TEXT, requested_by TEXT DEFAULT '', applied_by TEXT DEFAULT '',
			applied_at TEXT NULL, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS ai_task_run (
			id INTEGER PRIMARY KEY AUTOINCREMENT, task_id INTEGER, agent_id INTEGER,
			status TEXT DEFAULT 'running', started_at TEXT, finished_at TEXT,
			steps TEXT, final_report TEXT, undo_journal TEXT,
			needs_human INTEGER DEFAULT 0, reviewed_by TEXT DEFAULT '', reviewed_at TEXT NULL)`,
		`CREATE TABLE IF NOT EXISTS ai_agent (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, user_id TEXT,
			provider_id INTEGER, model_override TEXT, role_id INTEGER,
			operator_enabled INTEGER DEFAULT 0, max_steps INTEGER, max_rows INTEGER,
			enabled INTEGER DEFAULT 1, created_by TEXT, created_at TEXT,
			department TEXT, config_prompt TEXT, config_prompt_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS manage_invoice_dummy (x INTEGER)`,
	}
	for _, ddl := range ddls {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla de propuesta: %v\n%s", err, ddl)
		}
	}
}

// sembrarPermisoDeAlta deja concedido el permiso de ALTA (create) sobre
// manage_invoice para el rol del usuario, que es lo que aiPermisoDenegado y
// exigirPermiso comprueban en el camino de la propuesta.
func sembrarPermisoDeAlta(t *testing.T, db *sql.DB, userID string, roleID int, concederAlta bool) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR REPLACE INTO sub_module (id, directory, status) VALUES (?,?,1)`,
		roleID*100, "manage_invoice"); err != nil {
		t.Fatalf("sembrar sub_module: %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO sec_role (id, type, level) VALUES (?,?,?)`,
		roleID, "cobrador", "user"); err != nil {
		t.Fatalf("sembrar sec_role: %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO sec_userrole (user_id, roleid) VALUES (?,?)`,
		userID, roleID); err != nil {
		t.Fatalf("sembrar sec_userrole: %v", err)
	}
	c, rd, up, dl := 0, 1, 0, 0
	if concederAlta {
		c, up = 1, 1
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO role_permission (role_id, fk_module_id, "create", "read", "update", "delete")
		VALUES (?,?,?,?,?,?)`, roleID, roleID*100, c, rd, up, dl); err != nil {
		t.Fatalf("sembrar role_permission: %v", err)
	}
}

// ---------- F2 / S-4: lista negra por la frontera única ----------

// TestSQLInformesBloqueaTablasSensibles: el agujero medido era que un rol con
// «crear informe» guardaba `SELECT api_key FROM ai_providers` (o credenciales de
// user_login) y lo exportaba, porque el SQL de informes solo pasaba por un
// guardián de VERBOS. Aquí se rechaza en los tres puntos: la función canónica, el
// guardián de definiciones (no se puede crear) y la ejecución de una plantilla ya
// guardada (los existentes quedan no ejecutables, sin borrarlos).
func TestSQLInformesBloqueaTablasSensibles(t *testing.T) {
	casos := []struct {
		nombre string
		sql    string
		trampa string // tabla que debe aparecer nombrada en el motivo
	}{
		{"claves del proveedor", "SELECT api_key FROM ai_providers", "ai_providers"},
		{"credenciales de acceso", "SELECT username, password FROM user_login", "user_login"},
		{"usuarios de control", "SELECT * FROM users", "users"},
		{"claves del POS", "SELECT * FROM api_keys", "api_keys"},
		{"sesiones activas", "SELECT id FROM web_sessions", "web_sessions"},
		{"matriz de permisos", "SELECT * FROM role_permission", "role_permission"},
		{"ledger de migraciones", "SELECT checksum FROM schema_migrations", "schema_migrations"},
		{"cuentas predefinidas", "SELECT cashCode FROM acc_predefine_account", "acc_predefine_account"},
		{"auditoría", "SELECT * FROM audit_log", "audit_log"},
		// El join de una tabla inocente con una prohibida tampoco pasa: el
		// agujero histórico era exactamente que el regex solo veía la primera.
		{"prohibida en join", "SELECT i.id FROM invoice i JOIN user_login u ON u.username = 'x'", "user_login"},
		// Lista negra también para la configuración fiscal: norma que ya vivía
		// en el guardián de informes y que ahora aplica por todos los caminos.
		{"configuración fiscal", "SELECT * FROM fiscal_setting", "fiscal_setting"},
	}
	for _, c := range casos {
		for _, dept := range []string{DeptIT, DeptVentas, DeptCXC} {
			_, err := validarSQLInformes(c.sql, dept, aiHardMaxRows)
			if err == nil {
				t.Errorf("%s: la frontera dejó pasar %q con dept=%s", c.nombre, c.sql, dept)
				continue
			}
			if !strings.Contains(strings.ToLower(err.Error()), c.trampa) {
				t.Errorf("%s: el motivo debe nombrar la tabla (%s); salió: %v", c.nombre, c.trampa, err)
			}
		}
	}

	// Una tabla de negocio legítima sigue pasando: el candado no puede ser un
	// «nada se puede ejecutar» disimulado.
	if _, err := validarSQLInformes("SELECT total_amount FROM invoice", DeptVentas, aiHardMaxRows); err != nil {
		t.Errorf("un informe legítimo de facturación fue rechazado: %v", err)
	}

	// 1. No se puede CREAR con esa definición.
	s, db := basePrueba(t)
	crearTablasInformePrueba(t, db)
	sess := sessPrueba("admin")
	def := `{"title":"Fuga","sql":"SELECT api_key FROM ai_providers","columns":[{"key":"api_key","label":"Clave"}],"totals":[]}`
	if err := s.validateReportDefinition(def); err == nil {
		t.Error("validateReportDefinition debía rechazar una definición con tabla de la lista negra")
	} else if !strings.Contains(err.Error(), "ai_providers") {
		t.Errorf("el motivo debe decir qué tabla: %v", err)
	}

	// 2. Un informe YA GUARDADO con esa definición queda NO EJECUTABLE: se
	//    rehúsa en el run, sin borrar la fila (es trabajo del usuario; lo que se
	//    quita es el acceso, no el registro).
	if _, err := db.Exec(`INSERT INTO report_template (name, description, definition, created_by, status, created_at, updated_at)
		VALUES ('Fuga legado','',?,'user:7',1,'2026-01-01','2026-01-01')`, def); err != nil {
		t.Fatalf("insertar plantilla legada: %v", err)
	}
	_, err := s.executeReportDefinition(sess, def, "2026-11-01", "2026-11-30", 1)
	if err == nil {
		t.Fatal("el run de una plantilla legada con tabla prohibida debía rechazarse")
	}
	if !strings.Contains(err.Error(), "lista negra") {
		t.Errorf("el rechazo debe decir que es la lista negra; salió: %v", err)
	}
	var siguen int
	if err := db.QueryRow(`SELECT COUNT(*) FROM report_template WHERE name = 'Fuga legado'`).Scan(&siguen); err != nil || siguen != 1 {
		t.Errorf("la plantilla no se borra: sigue habiendo %d fila(s), err=%v", siguen, err)
	}
	// Y el mensaje de la pantalla de run explica que NO se borró nada.
	if m := aiErrorInformeNoEjecutable("Fuga legado", err); !strings.Contains(m.Error(), "NO se ha borrado") {
		t.Errorf("el mensaje al usuario debe decir que la plantilla no se borró: %v", m)
	}

	// 3. db.query comparte la MISMA lista (antes tenía norma propia).
	if _, err := aiValidarSQLConScope("SELECT api_key FROM ai_providers", DeptIT, 100); err == nil {
		t.Error("db.query debe usar la misma lista negra que los informes")
	}
}

// ---------- F4: el alcance en el preview ----------

// TestScopeSeAplicaEnRunPreview: report.run_preview era uno de los caminos que no
// aplicaba alcance. Se comprueba con el departamento de QUIEN EJECUTA (el agente),
// y que el mismo SQL sí pasa para un agente con alcance amplio: si no, la prueba
// estaría comprobando un fallo de sintaxis y no el candado.
func TestScopeSeAplicaEnRunPreview(t *testing.T) {
	s, db := basePrueba(t)
	crearTablasInformePrueba(t, db)
	sess := sessPrueba("admin")

	def := `{"title":"Sueldos","sql":"SELECT base_salary FROM hr_salary_assignment","columns":[{"key":"base_salary","label":"Salario"}],"totals":["base_salary"]}`
	agenteCartera := aiAgent{ID: 3, Name: "Agente CXP", UserID: "cxp", Department: DeptCXP, RoleName: "cuentas por pagar", Enabled: 1}
	salida, err := s.aiToolRunPreview(sess, agenteCartera, map[string]interface{}{"definition": def})
	if err != nil {
		t.Fatalf("la herramienta debe responder con JSON, no con error de transporte: %v", err)
	}
	if !strings.Contains(salida, "error") {
		t.Fatalf("run_preview debía rechazar la tabla de otro departamento; respondió: %s", salida)
	}
	if !strings.Contains(salida, "hr_salary_assignment") {
		t.Errorf("el rechazo debe nombrar la tabla a la que no se tiene alcance: %s", salida)
	}
	if strings.Contains(salida, "hr_salary_assignment") && !strings.Contains(strings.ToLower(salida), "acceso") {
		t.Errorf("el rechazo debe decir que es un problema de acceso, no de SQL: %s", salida)
	}

	// Control: el mismo SQL pasa para un agente cuyo alcance sí cubre la tabla.
	agenteRRHH := aiAgent{ID: 4, Name: "Agente RRHH", UserID: "rrhh", Department: DeptRRHH, RoleName: "recursos humanos", Enabled: 1}
	salidaRRHH, _ := s.aiToolRunPreview(sess, agenteRRHH, map[string]interface{}{"definition": def})
	if strings.Contains(salidaRRHH, "\"error\"") {
		t.Errorf("un agente de RRHH sí debe poder leer la tabla de salarios: %s", salidaRRHH)
	}

	// Y en la ejecución de plantilla guardada el alcance es el del que ejecuta,
	// no el del que la creó: la plantilla la hizo IT y la abre un rol de cartera.
	guardada := `{"title":"Sueldos guardado","sql":"SELECT base_salary FROM hr_salary_assignment","columns":[{"key":"base_salary","label":"Salario"}],"totals":[]}`
	if _, err := s.executeReportDefinition(sess, guardada, "", ""); err != nil {
		t.Fatalf("el administrador (IT) debe poder ejecutarla: %v", err)
	}
	empleado := &SessionData{UserID: "99", Username: "cobrador", UserLevel: "user"}
	if _, err := db.Exec(`INSERT OR REPLACE INTO sec_userrole (user_id, roleid) VALUES ('99', 9)`); err != nil {
		t.Fatalf("sembrar rol del empleado: %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO sec_role (id, type, level) VALUES (9, 'cuentas por pagar', 'user')`); err != nil {
		t.Fatalf("sembrar sec_role del empleado: %v", err)
	}
	if _, err := s.executeReportDefinition(empleado, guardada, "", ""); err == nil {
		t.Error("quien ejecuta es de cartera: la plantilla de nómina debe rechazarse en el run, no solo en el guardado")
	}
}

// ---------- M-3: STRAIGHT_JOIN y comentarios ejecutables ----------

// TestStraigthJoinYComentarioEjecutableNoPasable: dos elusiones medidas en la
// auditoría. STRAIGHT_JOIN no estaba en el mapa de apertores de tabla (el escáner
// declaraba `invoice` y no veía la segunda tabla) y `/*! ... */` se ejecutaba en
// MySQL mientras se validaba el texto sin él.
func TestStraigthJoinYComentarioEjecutableNoPasable(t *testing.T) {
	// 1. El escáner ya ve la tabla que trae STRAIGHT_JOIN (defensa: aunque la
	//    frontera decidiera tolerar el operador, la tabla no queda invisible).
	tablas := aiTablasEnSQL("SELECT * FROM invoice STRAIGHT_JOIN hr_salary_assignment")
	if len(tablas) != 2 || tablas[0] != "invoice" || tablas[1] != "hr_salary_assignment" {
		t.Errorf("el escáner perdió una tabla de un STRAIGHT_JOIN: %v", tablas)
	}
	if t2 := aiTablasEnSQL("SELECT * FROM invoice CROSS JOIN user_login"); len(t2) != 2 {
		t.Errorf("el escáner perdió una tabla de un CROSS JOIN: %v", t2)
	}

	// 2. STRAIGHT_JOIN se rechaza, no se tolera: el orden de tablas nunca es
	//    razón para leer una tabla fuera del alcance.
	if _, err := validarSQLInformes("SELECT * FROM invoice STRAIGHT_JOIN hr_salary_assignment", DeptIT, aiHardMaxRows); err == nil {
		t.Error("STRAIGHT_JOIN debe rechazarse en SQL de informes")
	} else if !strings.Contains(strings.ToUpper(err.Error()), "STRAIGHT_JOIN") {
		t.Errorf("el motivo debe nombrar el operador rechazado; salió: %v", err)
	}

	// 3. Comentarios ejecutables: se rechazan ANTES de limpiar, y la cadena que
	//    se ejecutaría no es la que se validó.
	casos := []string{
		"SELECT 1 /*!FROM ai_providers*/",
		"/*!40101 SET SESSION sql_mode='' */ SELECT 1 FROM invoice",
		"SELECT /*!50000 * */ FROM invoice",
		"SELECT /*+ BKA(t1) */ * FROM invoice",
	}
	for _, sqlCaso := range casos {
		_, err := aiNormalizarSQLInforme(sqlCaso)
		if err == nil {
			t.Errorf("comentario ejecutable aceptado en la normalización: %q", sqlCaso)
			continue
		}
		if !strings.Contains(err.Error(), "ejecutables") {
			t.Errorf("el rechazo debe decir que es un comentario ejecutable; salió: %v (%q)", err, sqlCaso)
		}
		if _, err := validarSQLInformes(sqlCaso, DeptIT, aiHardMaxRows); err == nil {
			t.Errorf("la frontera aceptó %q", sqlCaso)
		}
	}

	// 4. Se valida y se ejecuta la MISMA cadena: la normalización quita los
	//    comentarios normales y el texto devuelto no contiene ni uno.
	aprobado, err := validarSQLInformes("SELECT i.total_amount -- total\nFROM invoice i /* nota */", DeptVentas, aiHardMaxRows)
	if err != nil {
		t.Fatalf("un comentario normal no puede rechazar el informe: %v", err)
	}
	if strings.Contains(aprobado, "--") || strings.Contains(aprobado, "/*") {
		t.Errorf("la cadena ejecutada sigue carrying comentarios: %q", aprobado)
	}
	if !strings.Contains(aprobado, "LIMIT") {
		t.Errorf("la frontera debe devolver la cadena con su tope de filas: %q", aprobado)
	}

	// 5. Literales sin cerrar: no se puede saber qué ejecuta el servidor.
	if _, err := aiNormalizarSQLInforme("SELECT 'sin cerrar FROM ai_providers"); err == nil {
		t.Error("un literal sin cerrar debe rechazarse")
	}
	// Un '--' dentro de un literal es dato, no comentario.
	if x, err := aiNormalizarSQLInforme("SELECT 'a--b' AS c FROM invoice"); err != nil || !strings.Contains(x, "'a--b'") {
		t.Errorf("el literal con -- debe conservarse tal cual: %q err=%v", x, err)
	}
}

// ---------- F5: la aprobación va antes del efecto ----------

// herramientaCriticaDePrueba sustituye el Run de una herramienta crítica REAL por
// uno que cuenta cuántas veces lo llamaron. Se restaura al terminar: el registro
// es un var de paquete y dejarlo tocado contaminaría al resto de las pruebas.
func herramientaCriticaDePrueba(t *testing.T, ejecutadas *int) {
	t.Helper()
	nombre := "payment.record"
	original, existia := aiToolByName[nombre]
	aiToolByName[nombre] = aiToolDef{
		Name:       nombre,
		Risk:       RiskCritical,
		Permission: aiToolPermission{Directory: "manage_invoice", Action: "create"},
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			*ejecutadas++
			return string(mustJSON(map[string]interface{}{"status": "ok", "invoice_id": 1})), nil
		},
	}
	t.Cleanup(func() {
		if existia {
			aiToolByName[nombre] = original
			return
		}
		delete(aiToolByName, nombre)
	})
}

// TestCriticoQuedaPendiente: el dispatcher de una acción RiskCritical NO la
// ejecuta: deja propuesta con applied=false y avisa al modelo. Después, un humano
// con el permiso real la aplica, y uno sin el permiso no puede.
func TestCriticoQuedaPendiente(t *testing.T) {
	s, db := basePrueba(t)
	crearTablasPropuestaPrueba(t, db)
	crearTablasInformePrueba(t, db)

	ejecutadas := 0
	herramientaCriticaDePrueba(t, &ejecutadas)

	// El rol del AGENTE tiene el permiso (Nivel 2): sin él el dispatcher bloquea
	// antes de llegar a la aprobación, y la prueba no estaría midiendo F5.
	sembrarPermisoDeAlta(t, db, "cxp", 31, true)
	sessAgente := &SessionData{UserID: "cxp", Username: "agente.cartera", UserLevel: "agent"}
	agente := aiAgent{ID: 3, Name: "Agente CXP", UserID: "cxp", Department: DeptCXP, RoleName: "cuentas por pagar"}
	gate := gateDePrueba("user", map[string]string{"manage_invoice": "create,read,update"})
	journal := &aiRunJournal{runID: 12, taskID: 5, agentID: 3, scope: "task:5:day:2026-11-22"}

	args := map[string]interface{}{"invoice_id": float64(1), "amount": float64(50)}
	salida, err := s.aiDispatchTool(sessAgente, agente, aiToolPayload{Name: "payment.record", Args: args}, journal, gate)
	if err != nil {
		t.Fatalf("despachar una crítica no es un error de transporte: %v", err)
	}
	if ejecutadas != 0 {
		t.Fatalf("EL dispatcher ejecutó la acción crítica (%d vez/veces): F5 sigue abierto", ejecutadas)
	}
	var cuerpo map[string]interface{}
	if err := json.Unmarshal([]byte(salida), &cuerpo); err != nil {
		t.Fatalf("la respuesta no es JSON: %v — %s", err, salida)
	}
	if cuerpo["applied"] != false {
		t.Errorf("la respuesta debe traer applied=false: %s", salida)
	}
	if !strings.Contains(fmt.Sprintf("%v", cuerpo["note"]), "aprobación") {
		t.Errorf("la respuesta debe decir que falta aprobación humana: %s", salida)
	}
	if !strings.Contains(fmt.Sprintf("%v", cuerpo["status"]), "pendiente") {
		t.Errorf("la respuesta debe traer estado pendiente: %s", salida)
	}

	// La propuesta quedó registrada, pendiente y con sus argumentos exactos.
	var estado, argsGuardados string
	var idem string
	if err := db.QueryRow(`SELECT status, IFNULL(args_json,''), idem_key FROM ai_tool_proposal WHERE run_id = 12`).
		Scan(&estado, &argsGuardados, &idem); err != nil {
		t.Fatalf("la propuesta no quedó registrada: %v", err)
	}
	if estado != aiEstadoPropuestaPendiente {
		t.Errorf("la propuesta debe quedar pendiente; está en %q", estado)
	}
	if !strings.Contains(argsGuardados, "invoice_id") {
		t.Errorf("la propuesta debe guardar los argumentos del modelo: %q", argsGuardados)
	}
	if idem == "" {
		t.Error("la propuesta necesita su clave de idempotencia")
	}

	// Idempotencia: el mismo modelo pidiendo lo mismo no crea segunda propuesta.
	_, _ = s.aiDispatchTool(sessAgente, agente, aiToolPayload{Name: "payment.record", Args: args}, journal, gate)
	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM ai_tool_proposal`).Scan(&total)
	if total != 1 {
		t.Errorf("la misma llamada debió dar UNA propuesta; hay %d", total)
	}

	// La corrida queda marcada para revisión humana (es lo que pone el botón).
	if journal.NeedsHuman != true {
		t.Error("una corrida con acción crítica pendiente debe quedar marcada needs_human")
	}

	// --- el humano con permiso real SÍ puede aplicarla ---
	// No basta con que sea administrador de pantalla: la propuesta se aplica con
	// el permiso DECLARADO por la herramienta (manage_invoice/create), así que se
	// le concede por la misma matriz que usa la interfaz. Si la prueba dependiera
	// del bypass de admin, no estaría comprobando la regla de F5.
	sembrarPermisoDeAlta(t, db, "7", 71, true)
	sessHumana := sessPrueba("admin")
	if _, err := db.Exec(`INSERT INTO ai_task_run (id, task_id, agent_id, status, started_at, needs_human)
		VALUES (12, 5, 3, 'done', '2026-11-22 08:00:00', 1)`); err != nil {
		t.Fatalf("sembrar corrida: %v", err)
	}
	w := httptest.NewRecorder()
	r := reqPrueba(s, sessHumana, "/ai/tasks/runs/12/review")
	r.SetPathValue("id", "12")
	s.aiTaskRunReview(w, r)
	if w.Code != 200 {
		t.Fatalf("el administrador con permiso debe poder aplicar: %d %s", w.Code, w.Body.String())
	}
	if ejecutadas != 1 {
		t.Errorf("la propuesta debía ejecutarse UNA vez al aprobarla; se ejecutó %d", ejecutadas)
	}
	var estadoFinal, aplicoPor string
	_ = db.QueryRow(`SELECT status, applied_by FROM ai_tool_proposal WHERE run_id = 12`).Scan(&estadoFinal, &aplicoPor)
	if estadoFinal != aiEstadoPropuestaAplicada {
		t.Errorf("tras aprobar, la propuesta queda aplicada; quedó en %q", estadoFinal)
	}
	if aplicoPor != "7" {
		t.Errorf("applied_by debe ser el usuario humano que aprobó; es %q", aplicoPor)
	}

	// Second click: no se vuelve a ejecutar (claim antes del efecto).
	w2 := httptest.NewRecorder()
	r2 := reqPrueba(s, sessHumana, "/ai/tasks/runs/12/review")
	r2.SetPathValue("id", "12")
	s.aiTaskRunReview(w2, r2)
	if ejecutadas != 1 {
		t.Errorf("aprobar dos veces la misma propuesta no puede ejecutar dos veces; van %d", ejecutadas)
	}

	// --- el humano SIN el permiso real NO puede aplicarla ---
	if _, err := db.Exec(`INSERT INTO ai_tool_proposal
		(idem_key, tool, args_json, run_id, task_id, agent_id, status, reason, requested_by, created_at, updated_at)
		VALUES ('otra-clave','payment.record','{}',12,5,3,'pending','prueba','agente.cartera','2026-11-22 08:00:00','2026-11-22 08:00:00')`); err != nil {
		t.Fatalf("sembrar segunda propuesta: %v", err)
	}
	sinPermiso := &SessionData{UserID: "99", Username: "solo_lectura", UserLevel: "employee"}
	w3 := httptest.NewRecorder()
	r3 := reqPrueba(s, sinPermiso, "/ai/tasks/runs/12/review")
	r3.SetPathValue("id", "12")
	s.aiTaskRunReview(w3, r3)
	if w3.Code != 403 {
		t.Errorf("un rol sin el permiso de alta no puede aplicar la propuesta; respondió %d: %s", w3.Code, w3.Body.String())
	}
	if ejecutadas != 1 {
		t.Errorf("la propuesta sin permiso no debió ejecutarse; van %d ejecuciones", ejecutadas)
	}
	var siguePendiente string
	_ = db.QueryRow(`SELECT status FROM ai_tool_proposal WHERE idem_key='otra-clave'`).Scan(&siguePendiente)
	if siguePendiente != aiEstadoPropuestaPendiente {
		t.Errorf("la propuesta rechazada queda pendiente, no aplicando; está en %q", siguePendiente)
	}

	// --- retirar una propuesta la cierra SIN ejecutarla ---
	// Sin este camino, una propuesta que nadie aprueba se queda pendiente para
	// siempre y cualquiera con el permiso podría aplicarla mañana.
	var idRechazo int64
	if err := db.QueryRow(`SELECT id FROM ai_tool_proposal WHERE idem_key = 'otra-clave'`).Scan(&idRechazo); err != nil {
		t.Fatalf("leer el id de la propuesta a retirar: %v", err)
	}
	reqRechazo := reqPrueba(s, sessHumana, "/ai/tasks/runs/12/review")
	reqRechazo.Form = url.Values{"rechazar": {strconv.FormatInt(idRechazo, 10)}}
	reqRechazo.SetPathValue("id", "12")
	w4 := httptest.NewRecorder()
	s.aiTaskRunReview(w4, reqRechazo)
	if w4.Code != 200 {
		t.Fatalf("retirar la propuesta debió responder 200; llegó %d: %s", w4.Code, w4.Body.String())
	}
	var despues string
	_ = db.QueryRow(`SELECT status FROM ai_tool_proposal WHERE idem_key='otra-clave'`).Scan(&despues)
	if despues != aiEstadoPropuestaRechazada {
		t.Errorf("la propuesta retirada queda 'rejected'; quedó en %q", despues)
	}
	if ejecutadas != 1 {
		t.Errorf("retirar una propuesta no puede ejecutar nada; van %d ejecuciones", ejecutadas)
	}
	// Y ya no es reaplicable: retirar no es «pausar».
	w5 := httptest.NewRecorder()
	r5 := reqPrueba(s, sessHumana, "/ai/tasks/runs/12/review")
	r5.SetPathValue("id", "12")
	s.aiTaskRunReview(w5, r5)
	if ejecutadas != 1 {
		t.Errorf("una propuesta retirada no se puede aplicar después; van %d ejecuciones", ejecutadas)
	}
}

// ---------- F9: corrida auditable ----------

// TestCorridaInformeRegistra: cada ejecución de un informe deja su corrida con la
// huella de la definición y el usuario que la pidió. Sin eso, la pregunta «qué
// cifras vio esta persona en este mes» no tiene respuesta y un informe editado no
// se puede reproducir.
func TestCorridaInformeRegistra(t *testing.T) {
	s, db := basePrueba(t)
	crearTablasInformePrueba(t, db)
	sess := sessPrueba("admin")

	def := `{"title":"Ventas","sql":"SELECT total_amount AS total FROM invoice","columns":[{"key":"total","label":"Total"}],"totals":["total"]}`
	res, err := s.executeReportDefinition(sess, def, "2026-11-01", "2026-11-30", 42)
	if err != nil {
		t.Fatalf("el informe legítimo debía ejecutarse: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("el informe trajo %d filas; se esperaban 2", len(res.Rows))
	}
	if res.runID <= 0 {
		t.Error("la ejecución no devolvió id de corrida: la exportación no podría sellarla")
	}

	var hash, sqlCorrido, params, runBy string
	var filas, reportID int
	if err := db.QueryRow(`SELECT definition_hash, IFNULL(sql,''), IFNULL(params,''), run_by, rows_count, report_id
		FROM report_run WHERE id = ?`, res.runID).
		Scan(&hash, &sqlCorrido, &params, &runBy, &filas, &reportID); err != nil {
		t.Fatalf("no se registró la corrida del informe: %v", err)
	}
	if hash != aiHashKey(def) {
		t.Errorf("la huella de definición no es la de la definición ejecutada: %q", hash)
	}
	if runBy != sess.UserID {
		t.Errorf("run_by debe ser el usuario que ejecutó (%q); es %q", sess.UserID, runBy)
	}
	if filas != 2 {
		t.Errorf("rows_count debía ser 2; es %d", filas)
	}
	if reportID != 42 {
		t.Errorf("report_id debía ser 42; es %d", reportID)
	}
	if !strings.Contains(sqlCorrido, "invoice") || !strings.Contains(sqlCorrido, "LIMIT") {
		t.Errorf("el SQL registrado debe ser el que corrió (normalizado y con tope): %q", sqlCorrido)
	}
	if !strings.Contains(params, "2026-11-01") {
		t.Errorf("los parámetros deben quedar registrados: %q", params)
	}

	// El preview de una definición sin guardar no inventa un report_id: registra 0
	// y no falla, porque todavía no es un informe de la lista.
	if _, err := s.executeReportDefinition(sess, def, "", ""); err != nil {
		t.Fatalf("el preview sin id debía ejecutar: %v", err)
	}
	var ceros int
	_ = db.QueryRow(`SELECT COUNT(*) FROM report_run WHERE report_id = 0 OR report_id IS NULL`).Scan(&ceros)
	if ceros == 0 {
		t.Error("el preview también debe quedar anotado (report_id 0): es una corrida, no un informe guardado")
	}

	// La exportación sella exported_at sobre la corrida que exportó.
	s.marcarCorridaExportada(db, res.runID)
	var exportado sql.NullString
	if err := db.QueryRow(`SELECT exported_at FROM report_run WHERE id = ?`, res.runID).Scan(&exportado); err != nil || !exportado.Valid {
		t.Errorf("exported_at debe quedar sellado tras exportar; válido=%v err=%v", exportado.Valid, err)
	}
}

// ---------- la norma no se duplica ----------

// TestListaNegraUnica: la lista negra vive en UN solo lugar y la consumen los dos
// caminos. Si alguien añade una tabla a la lista y se olvida de aplicarla en uno de
// los dos, esta prueba lo delata —que es la clase de fallo que la auditoría encontró.
func TestListaNegraUnica(t *testing.T) {
	if len(aiTablasProhibidasEnSQL) < 20 {
		t.Fatalf("la lista negra se quedó corta (%d): la frontera se apaga sola", len(aiTablasProhibidasEnSQL))
	}
	// Las fiscales que declara el guardián de informes están dentro.
	for _, fiscal := range aiTablasFiscalesDeInforme() {
		if !aiTablaProhibida(fiscal) {
			t.Errorf("la tabla fiscal %q no entró en la lista negra común", fiscal)
		}
	}
	for _, sensible := range []string{"ai_providers", "ai_setting", "user_login", "users", "sec_userrole",
		"sec_role", "role_permission", "web_sessions", "audit_log", "audit_chain_blocks",
		"schema_migrations", "acc_predefine_account", "api_keys"} {
		if !aiTablaProhibida(sensible) {
			t.Errorf("la tabla sensible %q falta de la lista negra", sensible)
		}
	}
	// La lista ordenada es estable (mensajes de error reproducibles).
	a, b := aiListaNegraOrdenada(), aiListaNegraOrdenada()
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Error("el orden de la lista negra no es determinista: los motivos de rechazo cambiarían entre ejecuciones")
	}
}
