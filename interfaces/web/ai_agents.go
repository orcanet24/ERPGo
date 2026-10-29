package web

// ai_agents.go — Agentes IA: un agente es un usuario real del sistema con un
// rol RBAC normal y un proveedor de la whitelist compilada. El admin los crea
// como N operadores; sus permisos son exactamente los de su rol.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// aiAudit escribe auditoría de acciones de agentes (sin *http.Request: el
// runner actúa server-side; la "ip" se marca como ia).
func (s *Server) aiAudit(sess *SessionData, userID, action, module, table, recordID string, oldV, newV interface{}) {
	s.appendAudit(s.requestForSession(sess), userID, "ia", action, module, table, recordID, oldV, newV)
}

// aiAgent espeja una fila de ai_agent.
type aiAgent struct {
	ID              int64
	Name            string
	UserID          string
	ProviderID      int64
	ModelOverride   string
	RoleID          int64
	RoleName        string
	OperatorEnabled int
	MaxSteps        int
	MaxRows         int
	Enabled         int
	CreatedBy       string
	CreatedAt       string

	// Department es lo que decide qué tablas ve el agente y sobre cuáles puede
	// consultar (ai_schema_introspect.go, ai_sql_scope.go). Antes se deducía del
	// NOMBRE del rol, y un rol con nombre propio ("Analista de cobro") caía en
	// IT, que lo ve todo: un agente de cobranza acababa accessing la nómina.
	//
	// Vacío significa "deducir del nombre del rol", que es lo que hacen los
	// agentes creados antes de que existiera la columna. Cualquier otro valor
	// manda sobre el rol, incluido IT explícito.
	Department string

	// ConfigPrompt es el prompt de comportamiento que escribe el
	// administrador. Se antepone al del departamento y al del rol, y va DESPUÉS
	// de las reglas ocultas, que no son editables.
	ConfigPrompt string
	// ConfigPromptAt vale "" si el administrador no ha escrito ninguno.
	ConfigPromptAt string
}

// loadAIAgents lista todos los agentes (con nombre de rol y proveedor).
func (s *Server) loadAIAgents(sess *SessionData) []aiAgent {
	rows, err := s.dbForSession(sess).Query(`SELECT a.id, a.name, a.user_id, a.provider_id, IFNULL(a.model_override,''),
		a.role_id, IFNULL(sr.type,''), IFNULL(a.operator_enabled,0), a.max_steps, a.max_rows, a.enabled,
		IFNULL(a.created_by,''), IFNULL(CAST(a.created_at AS CHAR),''),
		IFNULL(a.department,''), IFNULL(a.config_prompt,''), IFNULL(a.config_prompt_at,'')
		FROM ai_agent a LEFT JOIN sec_role sr ON sr.id = a.role_id
		ORDER BY a.id DESC`)
	if err != nil {
		return []aiAgent{}
	}
	defer rows.Close()
	list := []aiAgent{}
	for rows.Next() {
		var g aiAgent
		var role, created, configPrompt, configAt sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.UserID, &g.ProviderID, &g.ModelOverride, &g.RoleID,
			&role, &g.OperatorEnabled, &g.MaxSteps, &g.MaxRows, &g.Enabled, &g.CreatedBy, &created,
			&g.Department, &configPrompt, &configAt); err != nil {
			continue
		}
		g.RoleName = role.String
		g.CreatedAt = created.String
		g.ConfigPrompt = configPrompt.String
		g.ConfigPromptAt = configAt.String
		list = append(list, g)
	}
	return list
}

// aiDeptDelAgente es el departamento EFFECTIVO de un agente.
//
// Dos fuentes, y la columna manda:
//
//  1. `ai_agent.department`, si tiene un valor válido. Es lo que elige el
//     administrador al crear o editar el agente.
//  2. El nombre del rol, cuando la columna está vacía.
//
// La 2 es compatibilidad pura: una base migrada tiene todos los agentes con la
// columna vacía, y sus roles sí dicen su departamento ("cuentas por cobrar").
// Sin ella, un agente de cobranza existente caería en IT —que lo ve todo— y
// ganaría acceso al plan de cuentas y a la nómina. La columna se deja vacía a
// propósito, no con 'it' por defecto, justamente para que esos dos casos se
// puedan distinguir: si el default fuera 'it', un administrador que quisiera IT
// explícito y un agente viejo sin tocar serían lo mismo.
func aiDeptDelAgente(a aiAgent) string {
	if dep := strings.ToLower(strings.TrimSpace(a.Department)); aiDeptEsValido(dep) {
		return dep
	}
	return aiDeptDeRolNombre(a.RoleName)
}

// loadAIAgentByID carga un agente específico.
func (s *Server) loadAIAgentByID(sess *SessionData, id int64) (aiAgent, bool) {
	for _, g := range s.loadAIAgents(sess) {
		if g.ID == id {
			return g, true
		}
	}
	return aiAgent{}, false
}

// loadAIAgentByUser resuelve el agente a partir del user_id.
func (s *Server) loadAIAgentByUser(sess *SessionData, userID string) (aiAgent, bool) {
	for _, g := range s.loadAIAgents(sess) {
		if g.UserID == userID {
			return g, true
		}
	}
	return aiAgent{}, false
}

// aiAgentUsername es la identidad de auditoría: username real del usuario
// agente (creado por agentSave como "agente.<slug>").
func aiAgentUsername(sess *SessionData, agent aiAgent) string {
	if sess != nil && sess.UserID == agent.UserID {
		return sess.Username
	}
	var u sql.NullString
	if agentDB != nil {
		_ = agentDB.QueryRow(`SELECT username FROM user_login WHERE user_id = ?`, agent.UserID).Scan(&u)
	}
	if u.Valid && u.String != "" {
		return u.String
	}
	return "agente." + agent.Name
}

// agentDB es la conexión usada por los helpers de agente (asignada en
// NewServer y en tests; nil-checked en aiAgentUsername vía dbAgentLookup).
var agentDB *sql.DB

// aiAgentCreator marca el creador de plantillas (user:ID | agente:ID).
func aiAgentCreator(sess *SessionData, agent aiAgent) string {
	if sess != nil && sess.UserID == agent.UserID {
		return "user:" + sess.UserID
	}
	return "agent:" + strconv.FormatInt(agent.ID, 10)
}

// aiAppendAuditForAgent: firma de auditoría referenciada por las herramientas.

// ---------- Handlers ----------

func (s *Server) registerAIAgentRoutes() {
	s.mux.HandleFunc("GET /ai/agents", s.agentList)
	s.mux.HandleFunc("GET /ai/agents/data", s.agentListData)
	s.mux.HandleFunc("GET /ai/agents/add", s.agentForm)
	s.mux.HandleFunc("GET /ai/agents/edit/{id}", s.agentForm)
	s.mux.HandleFunc("POST /ai/agents/save", s.agentSave)
	s.mux.HandleFunc("POST /ai/agents/delete/{id}", s.agentDelete)
	s.mux.HandleFunc("GET /ai/catalog", s.aiCatalogView)
	s.mux.HandleFunc("GET /ai/catalog/data", s.aiCatalogData)
}

func (s *Server) agentList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "agent_list.html", map[string]interface{}{"Title": "Agentes IA"})
}

func (s *Server) agentListData(w http.ResponseWriter, r *http.Request) {
	list := s.loadAIAgents(s.sessionFromRequest(r))
	out := make([]map[string]interface{}, 0, len(list))
	// El proveedor que muestra la lista es el que de verdad usan los agentes:
	// el activo de Configuración IA. La fila vieja de cada agente apunta a lo
	// que sea que hubiera guardado antes y la columna mentiría.
	vendorLabel := "—"
	if p, ok := s.loadActiveProviderRow(s.sessionFromRequest(r)); ok && p.VendorLabel != "" {
		vendorLabel = p.VendorLabel
	}
	for _, g := range list {
		var username sql.NullString
		_ = s.dbFor(r).QueryRow(`SELECT username FROM user_login WHERE user_id = ?`, g.UserID).Scan(&username)
		out = append(out, map[string]interface{}{
			"id": g.ID, "name": g.Name, "username": username.String, "role": g.RoleName,
			"vendor": vendorLabel, "model": g.ModelOverride,
			"operator": g.OperatorEnabled == 1, "enabled": g.Enabled == 1,
			"max_steps": g.MaxSteps, "max_rows": g.MaxRows,
		})
	}
	// DataTables busca las filas en «data» por defecto (sAjaxDataProp). Con una
	// matriz suelta en la raíz no encuentra nada, le llega undefined y revienta
	// dentro de su propio callback de éxito con «Cannot read properties of
	// undefined (reading 'length')», dejando la tabla vacía. Misma envoltura que
	// /user/list/data, que es como se alimenta el resto de los listados.
	writeJSON(w, map[string]interface{}{"data": out})
}

// aiProveedorActivoParaAgentes devuelve el ÚNICO proveedor que un agente puede
// usar —el que esté activo en Configuración IA— y la etiqueta para enseñárselo
// a quien crea el agente.
//
// No se le da a elegir: solo uno de los tres puede estar activo y con ese
// trabajan todas las pantallas. Un selector con las filas heredadas de la base
// era la puerta para que un agente quedara amarrado a un proveedor que nadie
// usaba ya.
func (s *Server) aiProveedorActivoParaAgentes(sess *SessionData) (aiProvider, string, bool) {
	p, ok := s.loadActiveProviderRow(sess)
	if !ok {
		return aiProvider{}, "", false
	}
	etiqueta := p.VendorLabel
	if n := strings.TrimSpace(p.Name); n != "" && !strings.EqualFold(n, p.VendorLabel) {
		etiqueta = n + " · " + p.VendorLabel
	}
	if m := strings.TrimSpace(p.Model); m != "" {
		etiqueta += " · " + m
	}
	return p, etiqueta, true
}

// agentForm muestra el formulario (add/edit) con roles y el proveedor activo.
func (s *Server) agentForm(w http.ResponseWriter, r *http.Request) {
	type option struct {
		ID   int64
		Name string
	}
	roles := []option{}
	rows, err := s.dbFor(r).Query(`SELECT id, type FROM sec_role ORDER BY type`)
	if err == nil {
		for rows.Next() {
			var o option
			_ = rows.Scan(&o.ID, &o.Name)
			roles = append(roles, o)
		}
		rows.Close()
	}
	proveedor, etiqueta, hayProveedor := s.aiProveedorActivoParaAgentes(s.sessionFromRequest(r))
	data := map[string]interface{}{
		"Title":             "Añadir Agente IA",
		"Roles":             roles,
		"ProveedorOK":       hayProveedor,
		"ProveedorID":       proveedor.ID,
		"ProveedorEtiqueta": etiqueta,
	}
	if idStr := r.PathValue("id"); idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if g, ok := s.loadAIAgentByID(s.sessionFromRequest(r), id); ok {
			data["Title"] = "Editar Agente IA"
			data["Agent"] = g
		}
	}
	s.render(w, r, "agent_form.html", data)
}

// agentSave crea/actualiza el agente y SU USUARIO (identidad real con rol).
func (s *Server) agentSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, "Formulario inválido", 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	roleID, _ := strconv.ParseInt(r.FormValue("role_id"), 10, 64)
	if roleID <= 0 {
		jsonError(w, "El rol es obligatorio", 400)
		return
	}
	sess := s.sessionFromRequest(r)
	// El agente trabaja SIEMPRE con el proveedor activo de Configuración IA. El
	// valor que traiga el formulario se ignora a propósito: un formulario
	// abierto hace una semana no puede enlazar al agente con un proveedor que
	// alguien ya desactivó. Y sin proveedor activo no se crean agentes —de otro
	// modo quedaría un agente que no puede trabajar y nadie se entera—.
	proveedor, _, hayProveedor := s.aiProveedorActivoParaAgentes(sess)
	if !hayProveedor {
		jsonError(w, "No hay proveedor de IA activo: actívelo en Configuración IA antes de crear agentes.", 400)
		return
	}
	providerID := proveedor.ID
	var roleExists int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM sec_role WHERE id = ?`, roleID).Scan(&roleExists)
	if roleExists == 0 {
		jsonError(w, "Rol no encontrado", 404)
		return
	}
	modelOverride := strings.TrimSpace(r.FormValue("model_override"))
	operator := boolInt(r.FormValue("operator_enabled") == "1")
	enabled := boolInt(r.FormValue("enabled") != "0")
	maxSteps, stepsOK := aiAtoiSafe(r.FormValue("max_steps"))
	if !stepsOK || maxSteps <= 0 || maxSteps > 40 {
		maxSteps = 8
	}
	maxRows, rowsOK := aiAtoiSafe(r.FormValue("max_rows"))
	if !rowsOK || maxRows <= 0 || maxRows > aiHardMaxRows {
		maxRows = 200
	}

	// Departamento: lo elige el administrador. Vacío = "deducir del rol", que es
	// lo que hacen los agentes antiguos (ver aiDeptDelAgente).
	department := strings.ToLower(strings.TrimSpace(r.FormValue("department")))
	if department != "" && !aiDeptEsValido(department) {
		jsonError(w, "Departamento desconocido: "+department, 400)
		return
	}
	configPrompt := strings.TrimSpace(r.FormValue("config_prompt"))
	creator := sessUserID(sess)

	idStr := r.FormValue("id")
	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		agent, found := s.loadAIAgentByID(s.sessionFromRequest(r), id)
		if !found {
			jsonError(w, "Agente no encontrado", 404)
			return
		}
		if _, err := s.dbFor(r).Exec(`UPDATE ai_agent SET name=?, provider_id=?, model_override=?, role_id=?,
			operator_enabled=?, max_steps=?, max_rows=?, enabled=?, department=?, config_prompt=?,
			config_prompt_at=? WHERE id=?`,
			name, providerID, modelOverride, roleID, operator, maxSteps, maxRows, enabled,
			department, nullableTexto(configPrompt), now(), id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// El rol del usuario agente sigue SIEMPRE al rol del agente.
		_, _ = s.dbFor(r).Exec(`UPDATE sec_userrole SET roleid=? WHERE user_id=?`, roleID, agent.UserID)
		s.aiAudit(s.sessionFromRequest(r), creator, "update", "ai_agents", "ai_agent", fmt.Sprintf("%d", id), nil, map[string]interface{}{"name": name, "role_id": roleID, "department": department, "operator": operator == 1})
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Agente actualizado", "id": id})
		return
	}

	// Crear identidad del agente: user_login + users + sec_userrole.
	username := "agente." + aiSlug(name)
	var dup int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM user_login WHERE username = ?`, username).Scan(&dup)
	for i := 2; dup > 0 && i < 100; i++ {
		username = "agente." + aiSlug(name) + "." + strconv.Itoa(i)
		_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM user_login WHERE username = ?`, username).Scan(&dup)
	}
	hash, err := aiRandomSecret(24)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// La tabla traducida no hereda AUTO_INCREMENT en sec_userrole.id (solo
	// UNIQUE), así que el id se calcula ANTES de abrir la tx (leer dentro de la
	// tx de escritura devuelve 0 filas en SQLite).
	var nextRoleRow int64
	_ = s.dbFor(r).QueryRow(`SELECT COALESCE(MAX(id),0)+1 FROM sec_userrole`).Scan(&nextRoleRow)
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`INSERT INTO users (user_id, first_name, last_name, status) VALUES (?,?, ' (Agente IA)', 1)`,
		generator(6), name)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	newUserID, _ := res.LastInsertId()
	uid := fmt.Sprintf("%d", newUserID)
	// Mismo invariante que userSave: users.user_id = user_login.user_id.
	_, _ = tx.Exec(`UPDATE users SET user_id = ? WHERE id = ?`, uid, newUserID)
	// Contraseña aleatoria de 43 chars: nadie la conoce ni la usa (los
	// agentes actúan a través del runner del servidor, no inician sesión).
	if _, err := tx.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
		VALUES (?,?,?,2,0)`, uid, username, hash); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if _, err := tx.Exec(`INSERT INTO sec_userrole (id, user_id, roleid, createby, createdate) VALUES (?,?,?,?,?)`,
		nextRoleRow, uid, roleID, creator, now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if _, err := tx.Exec(`INSERT INTO ai_agent (name, user_id, provider_id, model_override, role_id,
		operator_enabled, max_steps, max_rows, enabled, created_by, department, config_prompt, config_prompt_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		name, uid, providerID, modelOverride, roleID, operator, maxSteps, maxRows, enabled, creator,
		department, nullableTexto(configPrompt), now()); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.aiAudit(s.sessionFromRequest(r), creator, "create", "ai_agents", "ai_agent", username, nil, map[string]interface{}{"name": name, "role_id": roleID, "provider_id": providerID})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Agente creado", "username": username})
}

// agentDelete elimina el agente y su identidad de usuario.
func (s *Server) agentDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	agent, found := s.loadAIAgentByID(s.sessionFromRequest(r), id)
	if !found {
		jsonError(w, "Agente no encontrado", 404)
		return
	}
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()
	tx.Exec(`DELETE FROM sec_userrole WHERE user_id = ?`, agent.UserID)
	tx.Exec(`DELETE FROM user_login WHERE user_id = ?`, agent.UserID)
	if _, err := tx.Exec(`DELETE FROM users WHERE user_id = ?`, agent.UserID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if _, err := tx.Exec(`DELETE FROM ai_agent WHERE id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	sess := s.sessionFromRequest(r)
	s.aiAudit(s.sessionFromRequest(r), sessUserID(sess), "delete", "ai_agents", "ai_agent", fmt.Sprintf("%d", id), map[string]interface{}{"name": agent.Name}, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Agente eliminado", "redirect": saveRedirect(r, "/ai_agents/list")})
}

// ---------- Catálogo (mapa de BD) ----------

func (s *Server) aiCatalogView(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "ai_catalog.html", map[string]interface{}{"Title": "Mapa de Base de Datos"})
}

func (s *Server) aiCatalogData(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"map": s.aiSchemaMap(s.sessionFromRequest(r))})
}

// ---------- small helpers ----------

// aiSlug normaliza un nombre a slug para el username del agente.
func aiSlug(name string) string {
	var sb strings.Builder
	lastDash := true
	for _, c := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			sb.WriteRune(c)
			lastDash = false
		default:
			if !lastDash {
				sb.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(sb.String(), "-")
	if out == "" {
		out = "ia"
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

// aiRandomSecret genera un secreto aleatorio hex para la contraseña del
// usuario agente (inutilizable: status=0 y nadie la conoce).
func aiRandomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// aiAgentClock reloj inyectable (referenciado por el runner).
var aiAgentClock = func() time.Time { return time.Now() }
