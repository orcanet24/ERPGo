package web

// guia_goti.go — GOti, el instructor de la empresa DEMO (Fase 1).
//
// Qué es y qué NO es:
//   - NO es la IA. Es un instructor determinista: pasos sembrados como datos,
//     verificaciones SQL read-only declaradas en el paso, cero modelo de lenguaje.
//   - SOLO opera si la empresa activa está marcada como demo (tenants.is_demo=1
//     en la base de control). En una empresa real no responde nada, y el JS no
//     se carga (GuiDemo=false en el render).
//   - No bloquea: los `depends_on` son contexto con botón "Llévame", no candados.
// stuffing en guia_progreso hidden/skipped.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// guiaToursDemo son los tours sembrados. Con INSERT ... ON DUPLICATE KEY UPDATE
// el ensamblado se re-sembraria idempotente, actualizando el contenido sin
// tocar el progreso de los usuarios (guia_progreso va por paso_id, no por orden).
type guiaTourSemilla struct {
	Code, Label, Rol string
	Pasos            []guiaPasoSemilla
}

type guiaPasoSemilla struct {
	Orden    int
	Url      string
	Selector string
	Titulo   string
	Mensaje  string
	// DependsOnOrden: orden del paso previo dentro del mismo tour (0 = sin prerrequisito).
	DependsOnOrden int
	// CheckSQL es read-only y opcional: si el resultado no da ok, el paso no se marca hecho.
	CheckSQL string
	CheckMsg string
}

// guiaSemillasPrimerTour: "Facturar una venta manual". URLs reales del menú del ERP;
// los selectors existen en las plantillas correspondientes o son tolerantes.
var guiToursDemo = []guiaTourSemilla{
	{
		Code: "facturar_manual", Label: "Facturar una venta manual", Rol: "*",
		Pasos: []guiaPasoSemilla{
			{
				Orden: 1, Url: "/", Selector: "",
				Titulo:  "Bienvenido a ERPGo. Empecemos por una venta completa.",
				Mensaje: "Vamos a crear un cliente, agregarle productos y facturar. Cuando estés en el panel, sigue el globo: iremos paso a paso.",
			},
			{
				Orden: 2, Url: "/customer/add", Selector: "#content",
				DependsOnOrden: 1,
				Titulo:         "Primer paso: dar de alta un Cliente",
				Mensaje:        "Un cliente necesita, al menos, RIF y nombre. Sin cliente no hay factura: ERPGo exige RIF por exigencia del SENIAT.",
			},
			{
				Orden: 3, Url: "/product/add", Selector: "#content",
				DependsOnOrden: 2,
				Titulo:         "Segundo paso: crear un Producto",
				Mensaje:        "Con nombre, precio y el impuesto (IVA o exento). El mantenedor ya trae los porcentajes de impuestos que usas en español.",
			},
			{
				Orden: 4, Url: "/invoice/new", Selector: "#content",
				DependsOnOrden: 3,
				Titulo:         "Tercer paso: la Factura (carrito)",
				Mensaje:        "Busca el cliente que creaste, agrégale el producto, ajusta la cantidad y OBSERVA el total. Puedes también revisar el final de la mecanografía: el sistema calcula la tasa de cambio del día si es en USD.",
			},
			{
				Orden: 5, Url: "/invoice/new", Selector: "#btnCharge",
				DependsOnOrden: 4,
				Titulo:         "Cobrar la factura",
				Mensaje:        "Cuando el total cuadra, presiona Cobrar. Puedes ver que el recibo aparece con número de factura y número de control fiscal: eso es tu respaldo ante el SENIAT.",
				// Verificación: después de cobrar existe la última factura con detalle.
				CheckSQL: "SELECT COUNT(*) > 0 FROM invoice_details WHERE invoice_id = (SELECT MAX(invoice_id) FROM invoice)",
				CheckMsg: "¿Ya cobraste una factura? No vemos ningún renglón en la última factura.",
			},
			{
				Orden: 6, Url: "/invoice/list", Selector: "#dataTables_wrapper",
				DependsOnOrden: 5,
				Titulo:         "¡La factura quedó registrada!",
				Mensaje:        "Ya tienes una venta operada de punta a punta. El siguiente módulo que te recomendamos es el POS (Cobrar con caja) o la rutina del cierre del mes.",
			},
		},
	},
}

// guiaRoutes registra las rutas del instructor.
func (s *Server) registerGuiaRoutes() {
	s.mux.HandleFunc("GET /guia/active", s.guiaActive)
	s.mux.HandleFunc("POST /guia/progress", s.guiaProgress)
	s.mux.HandleFunc("GET /guia/demo-estado", s.guiaDemoEstado)  // admin
	s.mux.HandleFunc("POST /guia/demo-marcar", s.guiaDemoMarcar) // admin
}

// ----------
// is_demo del tenant (base de CONTROL)
// ----------

var (
	guiaDemoCacheMu sync.Mutex
	guiaDemoCache   = map[string]bool{} // code -> es demo
)

// ensureDemoColumn crea la columna tenants.is_demo una sola vez.
//
// SQLite (el motor embebido de las pruebas y de la instalación ligera) no
// soporta `ADD COLUMN IF NOT EXISTS`. La función es mejor esfuerzo: si la
// columna ya existe o el motor no deja añadirla, simplemente sigue. El SELECT de
// tenantEsDemo usa IFNULL(is_demo,0), así que una columna que no existe tampoco
// deja la guía colgada: responde no-demo.
func (s *Server) ensureDemoColumn() {
	ctl, err := s.controlStore()
	if err != nil {
		return
	}
	_, _ = ctl.Exec(`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS is_demo TINYINT(1) NOT NULL DEFAULT 0`)
}

// tenantEsDemo consulta (con caché de 60 s) si una empresa es la demo.
func (s *Server) tenantEsDemo(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	guiaDemoCacheMu.Lock()
	if v, ok := guiaDemoCache[code]; ok {
		guiaDemoCacheMu.Unlock()
		return v
	}
	guiaDemoCacheMu.Unlock()

	s.ensureDemoColumn()
	ctl, err := s.controlStore()
	if err != nil {
		return false
	}
	var demo int
	err = ctl.QueryRow(`SELECT IFNULL(is_demo,0) FROM tenants WHERE code = ? LIMIT 1`, code).Scan(&demo)
	esDemo := err == nil && demo == 1

	guiaDemoCacheMu.Lock()
	guiaDemoCache[code] = esDemo
	guiaDemoCacheMu.Unlock()
	// Caché de 60 s: marcar/desmarcar demo por admin se refleja sin reiniciar.
	go func(c string) {
		time.Sleep(time.Minute)
		guiaDemoCacheMu.Lock()
		delete(guiaDemoCache, c)
		guiaDemoCacheMu.Unlock()
	}(code)
	return esDemo
}

// guiaDemoEstado (admin): lista de empresas y cuáles están marcadas como demo.
func (s *Server) guiaDemoEstado(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil || (sess.UserLevel != "master" && sess.UserLevel != "admin") {
		jsonError(w, "Solo el administrador puede consultar el estado demo", http.StatusForbidden)
		return
	}
	s.ensureDemoColumn()
	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "Sin acceso a la base de control", 500)
		return
	}
	rows, err := ctl.Query(`SELECT code, IFNULL(is_demo,0) FROM tenants WHERE status = 1 ORDER BY code`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var code string
		var demo int
		if rows.Scan(&code, &demo) == nil {
			list = append(list, map[string]interface{}{"code": code, "demo": demo == 1})
		}
	}
	writeJSON(w, map[string]interface{}{"empresas": list})
}

// guiaDemoMarcar (admin) marca o desmarca una empresa como DEMO.
func (s *Server) guiaDemoMarcar(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil || (sess.UserLevel != "master" && sess.UserLevel != "admin") {
		jsonError(w, "Solo el administrador puede marcar empresas demo", http.StatusForbidden)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	demo := r.FormValue("demo") == "1"
	if code == "" {
		jsonError(w, "Código de empresa requerido", 400)
		return
	}
	s.ensureDemoColumn()
	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "Sin acceso a la base de control", 500)
		return
	}
	if _, err := ctl.Exec(`UPDATE tenants SET is_demo = ? WHERE code = ?`, boolToInt(demo), code); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	guiaDemoCacheMu.Lock()
	delete(guiaDemoCache, code)
	guiaDemoCacheMu.Unlock()
	msg := "La empresa " + code + " YA NO es demo: es de producción real."
	if demo {
		msg = "HomePage demo marcada: GOti disponible en " + code + " para todos los usuarios con sesión."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg})
}

// ----------
// Tablas guia_* (SOLO bajo demanda del tenant demo)
// ----------

var (
	guiaTabla_mu sync.Mutex
	guiaTablaOk  = map[string]bool{}
)

// ensureGuiaTables crea las 3 tablas y siembra el tour si el tenant es demo y
// no están. Barato en escondido: por tenant memorizado en memoria del proceso.
func ensureGuiaTables(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("sin base")
	}
	sentencias := []string{
		`CREATE TABLE IF NOT EXISTS guia_tour (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  code VARCHAR(64) NOT NULL UNIQUE,
		  label VARCHAR(160) NOT NULL,
		  rol VARCHAR(30) NOT NULL DEFAULT '*',
		  status TINYINT NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS guia_paso (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  tour_id INT NOT NULL,
		  orden INT NOT NULL,
		  url VARCHAR(255) NOT NULL,
		  selector VARCHAR(255) NOT NULL DEFAULT '',
		  titulo VARCHAR(200) NOT NULL,
		  mensaje TEXT NOT NULL,
		  depends_on INT NULL,
		  check_sql TEXT NULL,
		  check_msg VARCHAR(255) NULL
		)`,
		`CREATE TABLE IF NOT EXISTS guia_progreso (
		  user_id INT NOT NULL,
		  paso_id INT NOT NULL,
		  done TINYINT NOT NULL DEFAULT 0,
		  skipped TINYINT NOT NULL DEFAULT 0,
		  hidden TINYINT NOT NULL DEFAULT 0,
		  updated_at VARCHAR(32) NOT NULL DEFAULT '',
		  PRIMARY KEY (user_id, paso_id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_tour_orden ON guia_paso (tour_id, orden)`,
	}
	for _, q := range sentencias {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	// Siembra idempotente del contenido. Se borra lo sembrado de cada tour antes
	// de volver a insertar, para que el texto se pueda actualizar sin romper el
	// progreso (guia_progreso va por paso_id y no se toca aquí). DELETE+INSERT
	// es portable en MySQL y en SQLite, que no entiende ON DUPLICATE KEY.
	for _, t := range guiToursDemo {
		var tourID int64
		err := db.QueryRow(`SELECT id FROM guia_tour WHERE code = ?`, t.Code).Scan(&tourID)
		if err == nil {
			_, _ = db.Exec(`DELETE FROM guia_paso WHERE tour_id = ?`, tourID)
			_, _ = db.Exec(`DELETE FROM guia_tour WHERE code = ?`, t.Code)
		}
		res, err := db.Exec(`INSERT INTO guia_tour (code, label, rol, status)
			VALUES (?, ?, ?, 1)`, t.Code, t.Label, t.Rol)
		if err != nil {
			return err
		}
		tourID, _ = res.LastInsertId()

		for _, p := range t.Pasos {
			depends := sql.NullInt64{}
			if p.DependsOnOrden > 0 {
				var depID int64
				if e := db.QueryRow(`SELECT id FROM guia_paso WHERE tour_id = ? AND orden = ?`, tourID, p.DependsOnOrden).Scan(&depID); e == nil {
					depends = sql.NullInt64{Int64: depID, Valid: true}
				}
			}
			check := sql.NullString{}
			if p.CheckSQL != "" {
				check = sql.NullString{String: p.CheckSQL, Valid: true}
			}
			checkMsg := sql.NullString{}
			if p.CheckMsg != "" {
				checkMsg = sql.NullString{String: p.CheckMsg, Valid: true}
			}
			if _, err := db.Exec(`INSERT INTO guia_paso
				(tour_id, orden, url, selector, titulo, mensaje, depends_on, check_sql, check_msg)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				tourID, p.Orden, p.Url, p.Selector, p.Titulo, p.Mensaje, depends, check, checkMsg); err != nil {
				return err
			}
		}
	}
	return nil
}

// guiaProgresoOF es el estado de un paso para un usuario.
type guiaProgresoOF struct {
	Done    bool
	Skipped bool
	Hidden  bool
}

// ----------
// Endpoints
// ----------

// guiaActive devuelve el tour activo (si la empresa es demo y hay sesión).
// En producción (no demo) responde {"active":false} sin error para que el
// frontend ni cargue el overlay.
func (s *Server) guiaActive(w http.ResponseWriter, r *http.Request) {
	tenant := s.tenantCodeOrDefault()
	if !s.tenantEsDemo(tenant) {
		writeJSON(w, map[string]interface{}{"active": false})
		return
	}
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión requerida", http.StatusUnauthorized)
		return
	}
	db := s.dbFor(r)
	if db == nil {
		jsonError(w, "Base no disponible", 500)
		return
	}
	guiaTabla_mu.Lock()
	ok := guiaTablaOk[tenant]
	guiaTabla_mu.Unlock()
	if !ok {
		if err := ensureGuiaTables(db); err != nil {
			jsonError(w, "No se pudo preparar GOti: "+err.Error(), 500)
			return
		}
		guiaTabla_mu.Lock()
		guiaTablaOk[tenant] = true
		guiaTabla_mu.Unlock()
	}

	// Tours activos según rol del usuario.
	rolQuery := "*"
	rows, err := db.Query(`SELECT id, code, label FROM guia_tour WHERE status = 1 AND rol IN ('*', ?) ORDER BY id`, rolQuery)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var tourID int64
	var tourCode, tourLabel string
	for rows.Next() {
		if err := rows.Scan(&tourID, &tourCode, &tourLabel); err != nil {
			continue
		}
		break // v1: un tour a la vez (el primero sembrado)
	}
	rows.Close()
	if tourID == 0 {
		writeJSON(w, map[string]interface{}{"active": false})
		return
	}

	// Pasos del tour con el estado de ESTE usuario.
	pasos, err := s.guiaPasosDelTour(db, tourID, sess.UserID)

	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	paso := s.guiaPasoActual(pasos)
	writeJSON(w, map[string]interface{}{
		"active":    true,
		"code":      tourCode,
		"label":     tourLabel,
		"tenant":    tenant,
		"pasos":     pasos,
		"actual_id": paso,
	})
}

// guiaPasosDelTour devuelve los pasos con su estado para el usuario.
func (s *Server) guiaPasosDelTour(db *sql.DB, tourID int64, userID string) ([]map[string]interface{}, error) {
	rows, err := db.Query(`SELECT p.id, p.orden, p.url, p.selector, p.titulo, p.mensaje,
		IFNULL(p.depends_on,0), IFNULL(p.check_msg,''),
		IFNULL(pr.done,0), IFNULL(pr.skipped,0), IFNULL(pr.hidden,0)
		FROM guia_paso p
		LEFT JOIN guia_progreso pr ON pr.paso_id = p.id AND pr.user_id = ?
		WHERE p.tour_id = ?
		ORDER BY p.orden`, userID, tourID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pasos := []map[string]interface{}{}
	for rows.Next() {
		var id, orden, depends int64
		var url, titulo, mensaje string
		var selector, checkMsg sql.NullString
		var done, skipped, hidden int
		if err := rows.Scan(&id, &orden, &url, &selector, &titulo, &mensaje, &depends, &checkMsg, &done, &skipped, &hidden); err != nil {
			continue
		}
		pasos = append(pasos, map[string]interface{}{
			"id": id, "orden": orden, "url": url, "selector": selector.String,
			"titulo": titulo, "mensaje": mensaje, "depends_on": depends,
			"check_msg": checkMsg.String,
			"done":      done == 1, "skipped": skipped == 1, "hidden": hidden == 1,
		})
	}
	return pasos, rows.Err()
}

// guiaPasoActual: el primer paso no done/skip (o ” si el tour terminó).
func (s *Server) guiaPasoActual(pasos []map[string]interface{}) int64 {
	for _, p := range pasos {
		if b, _ := p["done"].(bool); b {
			continue
		}
		if b, _ := p["skipped"].(bool); b {
			continue
		}
		return toInt(p["id"])
	}
	return 0
}

func toInt(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// guiaProgress marca done/skip/hide de un paso del usuario EN SU empresa.
// Si hay check_sql: 'done' solo se acepta cuando la verificación pasa.
func (s *Server) guiaProgress(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión requerida", http.StatusUnauthorized)
		return
	}
	tenant := s.tenantCodeOrDefault()
	if !s.tenantEsDemo(tenant) {
		jsonError(w, "GOti solo opera en la empresa demo", http.StatusForbidden)
		return
	}
	db := s.dbFor(r)
	if db == nil {
		jsonError(w, "Base no disponible", 500)
		return
	}

	var req struct {
		PasoID int64  `json:"paso_id"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "JSON inválido", 400)
		return
	}

	// El paso pertenece a la base de ESTA empresa (GOTI no cruza empresas).
	var checkSQL sql.NullString
	var dependsOn sql.NullInt64
	err := db.QueryRow(`SELECT IFNULL(check_sql,''), IFNULL(depends_on,0) FROM guia_paso WHERE id = ?`, req.PasoID).Scan(&checkSQL, &dependsOn)
	if err == sql.ErrNoRows {
		jsonError(w, "Paso inexistente en esta empresa", 404)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	switch req.Action {
	case "done":
		// Verificación determinista del RESULTADO (read-only y exactamente el
		// check del paso, nada más).
		if checkSQL.Valid && strings.TrimSpace(checkSQL.String) != "" {
			ok, msg := s.guiaEjecutarCheck(db, checkSQL.String)
			if !ok {
				resp := map[string]interface{}{"status": 0, "error": "verificacion"}
				if msg != "" {
					resp["msg"] = msg
				}
				writeJSON(w, resp)
				return
			}
		}
		if err := s.guardarProgreso(db, sess.UserID, req.PasoID, "done"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	case "skip":
		if err := s.guardarProgreso(db, sess.UserID, req.PasoID, "skip"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	case "hide":
		if err := s.guardarProgreso(db, sess.UserID, req.PasoID, "hide"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	default:
		jsonError(w, "Acción inválida", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1})
}

// guardarProgreso inserta o actualiza el estado de un paso. DELETE+INSERT es
// portable en MySQL y en SQLite, que no entiende ON DUPLICATE KEY.
func (s *Server) guardarProgreso(db *sql.DB, userID string, pasoID int64, accion string) error {
	_, _ = db.Exec(`DELETE FROM guia_progreso WHERE user_id = ? AND paso_id = ?`, userID, pasoID)
	switch accion {
	case "done":
		_, err := db.Exec(`INSERT INTO guia_progreso (user_id, paso_id, done, skipped, hidden, updated_at)
			VALUES (?, ?, 1, 0, 0, ?)`, userID, pasoID, now())
		return err
	case "skip":
		_, err := db.Exec(`INSERT INTO guia_progreso (user_id, paso_id, done, skipped, hidden, updated_at)
			VALUES (?, ?, 0, 1, 0, ?)`, userID, pasoID, now())
		return err
	case "hide":
		_, err := db.Exec(`INSERT INTO guia_progreso (user_id, paso_id, done, skipped, hidden, updated_at)
			VALUES (?, ?, 0, 0, 1, ?)`, userID, pasoID, now())
		return err
	}
	return nil
}

// guiaEjecutarCheck corre el SQL del paso restringido a lectura.
func (s *Server) guiaEjecutarCheck(db *sql.DB, checkSQL string) (bool, string) {
	sqlLimpio := strings.TrimSpace(checkSQL)
	up := strings.ToUpper(sqlLimpio)
	if !strings.HasPrefix(up, "SELECT") {
		return false, "El check de GOti está mal configurado: no es SELECT"
	}
	var valor int
	if err := db.QueryRow(sqlLimpio).Scan(&valor); err != nil {
		return false, "El check de GOti esta fallo: consulta inválida"
	}
	if valor > 0 {
		return true, ""
	}
	return false, ""
}

// GuiDemo es el flag que la capa de render entrega a las plantillas: si es
// false, el JS de GOti ni se descarga. En producción el coste es cero.
func (s *Server) guiDemoFlag(r *http.Request) bool {
	// El tour lo ve el usuario en SU empresa activa; si la activa es demo, GOti.
	t := s.tenantCodeOrDefault() // la empresa activa del panel web
	return s.tenantEsDemo(t)
}
