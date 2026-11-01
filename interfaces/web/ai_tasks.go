package web

// ai_tasks.go — Tareas de agente: instrucciones claras + programación. El
// runner ejecuta el bucle modelo-herramientas con límites estrictos y deja
// bitácora paso a paso, informe final y journal de reversión.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// aiTask espeja una fila de ai_task.
type aiTask struct {
	ID            int64
	AgentID       int64
	AgentName     string
	Name          string
	Instructions  string
	ScheduleType  string
	ScheduleValue string
	Enabled       int
	CreatedBy     string
	CreatedAt     string

	// RespuestaJSON fuerza una salida JSON en el modelo en vez de Markdown: se
	// pide al runner que el «informe final» SEA el objeto que las instrucciones
	// solicitan (para informes creados por IA) y que no haya texto extra que el
	// extractor tenga que adivinar. Las tareas normales del scheduler dejanlo
	// en false: su salida es para leer un humano, no para parsearla.
	RespuestaJSON bool

	// HerramientasExcluidas quita herramientas del catálogo que esa corrida no
	// debe usar. Se usa donde el SERVIDOR, no el agente, hace el guardado al
	// final (p.ej. crear el informe con la IA): si el agente pudiera usar
	// report.save_template, dejaría la fila y el servidor volvería a insertarla
	// al recibir la respuesta → duplicados. Las herramientas que sí se usan
	// para validar (report.run_preview) no se excluyen: el agente necesita
	// consultar antes de proponer.
	HerramientasExcluidas []string
}

// loadAITasks lista todas las tareas.
//
// La tarea es de la empresa (ai_task vive en su base) pero el AGENTE vive en
// control: el JOIN cross-db no existe, así que el nombre se resuelve aparte
// con la lista de agentes (una sola consulta en lugar de una por tarea).
func (s *Server) loadAITasks(sess *SessionData) []aiTask {
	nombres := map[int64]string{}
	for _, a := range s.loadAIAgents(sess) {
		nombres[a.ID] = a.Name
	}
	rows, err := s.dbForSession(sess).Query(`SELECT t.id, t.agent_id, t.name, t.instructions,
		t.schedule_type, IFNULL(t.schedule_value,''), t.enabled, IFNULL(t.created_by,''), IFNULL(CAST(t.created_at AS CHAR),'')
		FROM ai_task t
		ORDER BY t.id DESC`)
	if err != nil {
		return []aiTask{}
	}
	defer rows.Close()
	list := []aiTask{}
	for rows.Next() {
		var t aiTask
		var created sql.NullString
		if err := rows.Scan(&t.ID, &t.AgentID, &t.Name, &t.Instructions,
			&t.ScheduleType, &t.ScheduleValue, &t.Enabled, &t.CreatedBy, &created); err != nil {
			continue
		}
		t.AgentName = nombres[t.AgentID]
		t.CreatedAt = created.String
		list = append(list, t)
	}
	return list
}

// loadAITaskByID carga una tarea.
func (s *Server) loadAITaskByID(sess *SessionData, id int64) (aiTask, bool) {
	for _, t := range s.loadAITasks(sess) {
		if t.ID == id {
			return t, true
		}
	}
	return aiTask{}, false
}

// aiTaskRun es la vista de una corrida para la UI.
type aiTaskRun struct {
	ID          int64
	TaskID      int64
	TaskName    string
	AgentID     int64
	AgentName   string
	Status      string
	StartedAt   string
	FinishedAt  string
	Steps       []aiStep
	FinalReport string
	CanUndo     bool
	// NeedsHuman llega de la BASE (ai_task_run.needs_human), no de una variable
	// del runner: es lo que hace que la marca sobreviva al proceso y al reinicio.
	NeedsHuman bool
	ReviewedBy string
	ReviewedAt string
}

type aiStep struct {
	N        int    `json:"n"`
	Type     string `json:"type"` // "model" | "tool" | "error"
	Name     string `json:"name,omitempty"`
	Input    string `json:"input,omitempty"`
	Output   string `json:"output,omitempty"`
	At       string `json:"at"`
	Truncate bool   `json:"-"`
}

// aiStepList decodifica la bitácora persistida.
func aiStepList(raw sql.NullString) []aiStep {
	steps := []aiStep{}
	if raw.Valid && strings.TrimSpace(raw.String) != "" {
		_ = json.Unmarshal([]byte(raw.String), &steps)
	}
	return steps
}

func (s *Server) registerAITaskRoutes() {
	s.mux.HandleFunc("GET /ai/tasks", s.aiTaskList)
	s.mux.HandleFunc("GET /ai/tasks/data", s.aiTaskListData)
	s.mux.HandleFunc("GET /ai/tasks/add", s.aiTaskForm)
	s.mux.HandleFunc("GET /ai/tasks/edit/{id}", s.aiTaskForm)
	s.mux.HandleFunc("POST /ai/tasks/save", s.aiTaskSave)
	s.mux.HandleFunc("POST /ai/tasks/delete/{id}", s.aiTaskDelete)
	s.mux.HandleFunc("POST /ai/tasks/run/{id}", s.aiTaskRunNow)
	s.mux.HandleFunc("GET /ai/tasks/runs/{id}", s.aiTaskRunView)
	s.mux.HandleFunc("POST /ai/tasks/runs/{id}/undo", s.aiTaskUndo)
	s.mux.HandleFunc("POST /ai/tasks/runs/{id}/review", s.aiTaskRunReview)
}

func (s *Server) aiTaskList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "ai_task_list.html", map[string]interface{}{"Title": "Tareas de Agentes"})
}

func (s *Server) aiTaskListData(w http.ResponseWriter, r *http.Request) {
	tasks := s.loadAITasks(s.sessionFromRequest(r))
	out := make([]map[string]interface{}, 0, len(tasks))
	for _, t := range tasks {
		var lastRun sql.NullString
		var lastStatus sql.NullString
		var needsHuman sql.NullInt64
		var reviewedBy sql.NullString
		_ = s.dbFor(r).QueryRow(`SELECT id, status, IFNULL(needs_human,0), IFNULL(reviewed_by,'')
			FROM ai_task_run WHERE task_id = ? ORDER BY id DESC LIMIT 1`, t.ID).
			Scan(&lastRun, &lastStatus, &needsHuman, &reviewedBy)
		item := map[string]interface{}{
			"id": t.ID, "name": t.Name, "agent": t.AgentName, "schedule": t.ScheduleType,
			"schedule_value": t.ScheduleValue, "enabled": t.Enabled == 1,
		}
		if lastRun.Valid {
			item["last_run_id"] = lastRun.String
			item["last_status"] = lastStatus.String
			// pending_review es «marcada y todavía sin revisar»: es lo que el
			// listado tiene que mostrar, no la marca histórica.
			item["pending_review"] = needsHuman.Int64 == 1 && reviewedBy.String == ""
		}
		out = append(out, item)
	}
	// Misma razón que en la lista de agentes: DataTables lee «data» por defecto
	// y con la matriz en la raíz la tabla revienta con «Cannot read properties of
	// undefined (reading 'length')» y se queda sin filas.
	writeJSON(w, map[string]interface{}{"data": out})
}

// aiTaskForm muestra el formulario (add/edit) con la lista de agentes.
func (s *Server) aiTaskForm(w http.ResponseWriter, r *http.Request) {
	agents := s.loadAIAgents(s.sessionFromRequest(r))
	data := map[string]interface{}{"Title": "Nueva Tarea de Agente", "Agents": agents}
	if idStr := r.PathValue("id"); idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if t, ok := s.loadAITaskByID(s.sessionFromRequest(r), id); ok {
			data["Title"] = "Editar Tarea"
			data["Task"] = t
		}
	}
	s.render(w, r, "ai_task_form.html", data)
}

// aiTaskSave valida y guarda la tarea.
func (s *Server) aiTaskSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, "Formulario inválido", 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	instructions := strings.TrimSpace(r.FormValue("instructions"))
	agentID, _ := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	scheduleType := strings.TrimSpace(r.FormValue("schedule_type"))
	scheduleValue := strings.TrimSpace(r.FormValue("schedule_value"))
	if name == "" || instructions == "" {
		jsonError(w, "Nombre e instrucciones son obligatorios", 400)
		return
	}
	if _, found := s.loadAIAgentByID(s.sessionFromRequest(r), agentID); !found {
		jsonError(w, "Agente no encontrado", 404)
		return
	}
	switch scheduleType {
	case "daily":
	case "weekly":
		if scheduleValue == "" {
			scheduleValue = "1"
		}
	case "cron5":
		scheduleType = "cron5"
		scheduleValue = ""
	case "manual":
		scheduleType = "manual"
		scheduleValue = ""
	default:
		scheduleType = "manual"
		scheduleValue = ""
	}
	enabled := boolInt(r.FormValue("enabled") != "0")
	sess := s.sessionFromRequest(r)
	creator := sessUserID(sess)

	idStr := r.FormValue("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			jsonError(w, "Identificador de tarea inválido", 400)
			return
		}
		res, err := s.dbFor(r).Exec(`UPDATE ai_task SET agent_id=?, name=?, instructions=?, schedule_type=?,
			schedule_value=?, enabled=? WHERE id=?`,
			agentID, name, instructions, scheduleType, scheduleValue, enabled, id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if n, err := res.RowsAffected(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		} else if n == 0 {
			// SQLite y MySQL pueden reportar cero cuando la fila existe pero
			// todos los valores ya eran iguales. Solo es 404 si no hay fila.
			var existe int
			if err := s.dbFor(r).QueryRow(`SELECT 1 FROM ai_task WHERE id = ?`, id).Scan(&existe); errors.Is(err, sql.ErrNoRows) {
				jsonError(w, "Tarea no encontrada", 404)
				return
			} else if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
		}
		s.aiAudit(s.sessionFromRequest(r), creator, "update", "ai_tasks", "ai_task", fmt.Sprintf("%d", id), nil, map[string]interface{}{"name": name})
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tarea actualizada", "id": id, "redirect": saveRedirect(r, "/ai/tasks")})
		return
	}
	res, err := s.dbFor(r).Exec(`INSERT INTO ai_task (agent_id, name, instructions, schedule_type, schedule_value, enabled, created_by)
		VALUES (?,?,?,?,?,?,?)`, agentID, name, instructions, scheduleType, scheduleValue, enabled, creator)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	newID, _ := res.LastInsertId()
	s.aiAudit(s.sessionFromRequest(r), creator, "create", "ai_tasks", "ai_task", fmt.Sprintf("%d", newID), nil, map[string]interface{}{"name": name, "agent_id": agentID})
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Tarea creada", "id": newID,
		"redirect": saveRedirect(r, "/ai/tasks"),
	})
}

// aiTaskDelete elimina la tarea (las corridas quedan como auditoría).
func (s *Server) aiTaskDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	res, err := s.dbFor(r).Exec(`DELETE FROM ai_task WHERE id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Tarea no encontrada", 404)
		return
	}
	sess := s.sessionFromRequest(r)
	s.aiAudit(s.sessionFromRequest(r), sessUserID(sess), "delete", "ai_tasks", "ai_task", fmt.Sprintf("%d", id), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tarea eliminada", "redirect": saveRedirect(r, "/ai/tasks")})
}

// aiTaskRunNow lanza la ejecución manual de una tarea.
func (s *Server) aiTaskRunNow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	task, found := s.loadAITaskByID(s.sessionFromRequest(r), id)
	if !found {
		jsonError(w, "Tarea no encontrada", 404)
		return
	}
	if task.Enabled != 1 {
		jsonError(w, "La tarea está deshabilitada", 400)
		return
	}
	// Disparo manual: sin ranura de período. Un humano pidió ejecutar ahora, y
	// negarlo por «ya corrió hoy» sería confundir la intención con un reintento.
	// Lo que sí se deduplica es el DOCUMENTO, en el libro de idempotencia.
	runID, err := s.aiExecuteTask(s.sessionFromRequest(r), task, "")
	if err != nil {
		if errors.Is(err, errAIRunnerBusy) {
			jsonError(w, err.Error(), 429)
			return
		}
		if errors.Is(err, errAIRunSlotTaken) {
			jsonError(w, err.Error(), 409)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tarea ejecutada", "run_id": runID})
}

// aiRunnerRunning es el cerrojo global del runner (una sola corrida a la vez
// en todo el proceso: previene tormentas de llamadas al proveedor). Se toma
// con CAS porque el ticker de programación y los handlers HTTP compiten por él
// desde goroutines distintas.
var aiRunnerRunning atomic.Int64

// errAIRunnerBusy señala que ya hay una corrida en curso.
var errAIRunnerBusy = errors.New("ya hay una tarea en ejecución; espere a que termine")

// aiTaskRunView muestra la corrida (bitácora + informe + undo).
func (s *Server) aiTaskRunView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Identificador inválido", 400)
		return
	}
	run, ok := s.aiLoadRun(s.sessionFromRequest(r), id)
	if !ok {
		http.Error(w, "Corrida no encontrada", 404)
		return
	}
	s.render(w, r, "ai_task_run.html", map[string]interface{}{
		"Title": "Ejecución #" + fmt.Sprintf("%d", run.ID), "Run": run,
	})
}

// aiLoadRun hidrata una corrida.
func (s *Server) aiLoadRun(sess *SessionData, id int64) (aiTaskRun, bool) {
	var taskID, agentID int64
	var status, started sql.NullString
	var finished sql.NullString
	var stepsRaw, report sql.NullString
	var needsHuman sql.NullInt64
	var reviewedBy, reviewedAt sql.NullString
	err := s.dbForSession(sess).QueryRow(`SELECT task_id, agent_id, status, started_at, finished_at, steps, final_report,
		IFNULL(needs_human,0), IFNULL(reviewed_by,''), reviewed_at
		FROM ai_task_run WHERE id = ?`, id).
		Scan(&taskID, &agentID, &status, &started, &finished, &stepsRaw, &report, &needsHuman, &reviewedBy, &reviewedAt)
	if err != nil {
		return aiTaskRun{}, false
	}
	task, _ := s.loadAITaskByID(sess, taskID)
	agent, _ := s.loadAIAgentByID(sess, agentID)
	run := aiTaskRun{
		ID: id, TaskID: taskID, TaskName: task.Name, AgentID: agentID, AgentName: agent.Name,
		Status: status.String, StartedAt: started.String, FinishedAt: finished.String,
		Steps: aiStepList(stepsRaw), FinalReport: report.String,
		NeedsHuman: needsHuman.Int64 == 1, ReviewedBy: reviewedBy.String, ReviewedAt: reviewedAt.String,
	}
	// También se revierte una corrida fallida: es la que deja escrituras
	// parciales, y era la única que el botón no ofrecía.
	run.CanUndo = run.Status == "done" || run.Status == "failed"
	return run, true
}

// aiTaskUndo revierte una corrida (solo admin; el journal es el guardián).
func (s *Server) aiTaskUndo(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil || (sess.UserLevel != "admin" && sess.UserLevel != "master") {
		jsonError(w, "Solo un administrador puede revertir una tarea", 403)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	var agentID int64
	if err := s.dbFor(r).QueryRow(`SELECT agent_id FROM ai_task_run WHERE id = ?`, id).Scan(&agentID); err != nil {
		jsonError(w, "Corrida no encontrada", 404)
		return
	}
	agent, found := s.loadAIAgentByID(s.sessionFromRequest(r), agentID)
	if !found {
		jsonError(w, "Agente no encontrado", 404)
		return
	}
	if err := s.aiUndoLastRun(sess, agent, id); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tarea revertida", "redirect": saveRedirect(r, "/ai/tasks")})
}

// aiExecuteTask es el bucle principal del runner. Devuelve el run_id.
//
// runSlot es la ranura de programación (ver aiRunSlot). Vacío = disparo manual.
// La ranura impide que la MISMA tarea se ejecute dos veces en el MISMO período;
// la deduplicación de los DOCUMENTOS la lleva el libro de idempotencia de las
// herramientas (ai_idempotency.go), que es otra cosa y cubre otros caminos.
func (s *Server) aiExecuteTask(sess *SessionData, task aiTask, runSlot string) (int64, error) {
	agent, found := s.loadAIAgentByID(sess, task.AgentID)
	if !found {
		return 0, fmt.Errorf("agente no encontrado")
	}
	if agent.Enabled != 1 {
		return 0, fmt.Errorf("agente deshabilitado")
	}
	// Cerrojo atómico ANTES de insertar la fila 'running': si el runner está
	// ocupado no se dejan corridas huérfanas.
	if !aiRunnerRunning.CompareAndSwap(0, 1) {
		return 0, errAIRunnerBusy
	}
	defer aiRunnerRunning.Store(0)

	db := s.dbForSession(sess)

	// RANURA DE PROGRAMACIÓN. Reclamarla es lo que corta la duplicación: antes la
	// decisión era comparar la hora del reloj, verdadera en las doce vueltas del
	// ticker dentro de esa hora, y una tarea «daily» corría hasta doce veces.
	if runSlot != "" {
		claimed, err := s.aiClaimRunSlot(db, runSlot, task.ID)
		if err != nil {
			return 0, err
		}
		if !claimed {
			return 0, errAIRunSlotTaken
		}
	}

	res, err := db.Exec(`INSERT INTO ai_task_run (task_id, agent_id, status, started_at) VALUES (?,?, 'running', ?)`,
		task.ID, agent.ID, aiAgentClock().Format("2006-01-02 15:04:05"))
	if err != nil {
		s.aiReleaseRunSlot(db, runSlot)
		return 0, err
	}
	runID, _ := res.LastInsertId()

	journal := &aiRunJournal{
		runID:   runID,
		taskID:  task.ID,
		agentID: agent.ID,
		scope:   aiRunScope(task, runSlot),
	}
	steps := []aiStep{}
	report, runErr := s.aiRunLoop(sess, agent, task, &steps, journal)

	status := "done"
	finalReport := report
	if runErr != nil {
		status = "failed"
		if finalReport == "" {
			finalReport = "Error: " + runErr.Error()
		} else {
			finalReport += "\n\n**Error:** " + runErr.Error()
		}
		steps = append(steps, aiStep{Type: "error", Output: runErr.Error(), At: aiRunnerStartedAt()})
	}
	// La ranura se libera si la corrida falla: un fallo del proveedor a las 6:00 no
	// puede dejar la tarea diaria sin reintento hasta mañana. Si termina bien, la
	// ranura queda tomada y el resto de la hora no vuelve a ejecutarla.
	if status == "failed" {
		s.aiReleaseRunSlot(db, runSlot)
	}

	// La marca de revisión humana se guarda con la corrida. Antes vivía en un campo
	// sin exportar y se perdía al serializar el journal: el agente ejecutaba un
	// pago y la marca se evaporaba.
	needsHuman := 0
	if journal.NeedsHuman {
		needsHuman = 1
	}
	journalJSON, _ := json.Marshal(journal)
	stepsJSON, _ := json.Marshal(steps)
	_, _ = db.Exec(`UPDATE ai_task_run SET status=?, finished_at=?, steps=?, final_report=?, undo_journal=?, needs_human=? WHERE id=?`,
		status, aiAgentClock().Format("2006-01-02 15:04:05"), string(stepsJSON), finalReport, string(journalJSON), needsHuman, runID)
	return runID, runErr
}

// aiRunLoop: bucle modelo-herramientas con máxSteps. Devuelve el informe.
func (s *Server) aiRunLoop(sess *SessionData, agent aiAgent, task aiTask, steps *[]aiStep, journal *aiRunJournal) (string, error) {
	p, ok := s.aiProviderForAgent(sess, agent)
	if !ok {
		return "", fmt.Errorf("no hay proveedor de IA activo con API key: actívelo en Configuración IA")
	}
	// El rol del agente se resuelve ANTES de cambiar la sesión: aquí todavía viene
	// la empresa (TenantCode), y el rol vive en la base de la empresa o en la de
	// control. Después de esta línea la sesión es la identidad mínima del agente
	// para los prompts, no para resolver permisos.
	gate := s.aiBuildRoleGate(sess, agent)
	tenant := ""
	if sess != nil {
		tenant = sess.TenantCode
	}
	// TenantCode viaja a la sesión del runner: las herramientas resuelven la base
	// por sesión (dbForSession), y perderla hacía que una tarea encolada de la
	// empresa A pudiera escribir contra la empresa activa del proceso.
	sess = &SessionData{UserID: agent.UserID, Username: aiAgentUsername(nil, agent), Fullname: agent.Name, UserLevel: "agent", TenantCode: tenant}

	roleName := agent.RoleName
	if roleName == "" {
		roleName = "operador"
	}
	// El prompt completo: reglas ocultas + comportamiento del departamento +
	// configuración personal del agente + rol + mapa técnico de SU departamento.
	//
	// El departamento es `ai_agent.department` si está puesto, y si no, el que
	// dice su nombre de rol (ver aiDeptDelAgente). Ese mismo departamento es el
	// que usa el guardián de SQL en db_query, así que el prompt y el candado
	// dicen lo mismo por construcción.
	//
	// reglas varía solo con RespuestaJSON: la regla 6 del sistema pide Markdown,
	// y para una salida JSON la regla hay que cambiarla para que no contradiga el
	// contrato de la tarea (la regla y la instrucción dicen lo mismo).
	reglas := aiReglasDelSistema(task.RespuestaJSON)
	system := s.buildAgentPrompt(sess, agent, roleName, reglas)
	// El sufijo final del prompt cambia con la misma bandera: para salida JSON el
	// «informe final» que pide la regla es el objeto JSON, no prosa. Antes se
	// añadía siempre "responde en Markdown" y contradecía el contrato de la
	// tarea → el extractor no hallaba el objeto → 502 con los tokens gastados.
	userMsg := "TAREA: " + task.Name + "\n\nINSTRUCCIONES:\n" + task.Instructions +
		"\n\nUsa las herramientas disponibles para consultar datos" + aiOperatorHint(agent) + "."
	if task.RespuestaJSON {
		userMsg += " Cuando tengas todo, responde ÚNICAMENTE con el objeto JSON solicitado en las instrucciones, sin markdown, sin vallado de código y sin texto fuera del objeto; no hagas llamadas a herramientas."
	} else {
		userMsg += " Cuando tengas todo, responde ÚNICAMENTE con el informe final en Markdown, sin llamadas a herramientas."
	}

	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": userMsg},
	}
	model := strings.TrimSpace(agent.ModelOverride)
	if model == "" {
		model = strings.TrimSpace(p.Model)
	}
	if model == "" {
		model = aiModelPorDefecto(p.Vendor)
	}

	maxSteps := agent.MaxSteps
	if maxSteps <= 0 || maxSteps > 40 {
		maxSteps = 8
	}
	// Una fila por llamada al modelo. Un agente da hasta maxSteps llamadas y cada
	// una se cobra, así que el registro va justo después de cada respuesta.
	ultimoContent := "" // acumula content no vacío: algunos modelos (ollama)
	// responden tool calls con content="" en el mensaje final; preservar el
	// content anterior permite recuperar el JSON que el modelo ya escribió.
	for step := 1; step <= maxSteps; step++ {
		content, toolCalls, usage, err := aiCallChatWithTools(p, model, messages, aiToolsJSONExcluyendo(agent.OperatorEnabled == 1, gate, task.HerramientasExcluidas))
		// Se registra aunque este paso falle: los tokens gastados hasta el
		// fallo también son consumo facturable.
		s.recordAIUsage(s.dbForSession(sess), aiUsageEvent{
			ProviderID: p.ID,
			Vendor:     p.Vendor,
			Model:      usage.Model,
			AgentID:    agent.ID,
			AgentName:  agent.Name,
			Source:     "agente",
			Prompt:     usage.Prompt,
			Completion: usage.Completion,
			OK:         err == nil,
		})
		if err != nil {
			if strings.TrimSpace(ultimoContent) != "" {
				return ultimoContent, nil
			}
			return "", err
		}
		if strings.TrimSpace(content) != "" {
			ultimoContent = content
		}
		if len(toolCalls) == 0 {
			// Informe final del modelo. Si el content está vacío pero el
			// modelo escribió content en pasos anteriores (tool calls con
			// content=""), devolver ese content acumulado.
			if strings.TrimSpace(content) == "" && strings.TrimSpace(ultimoContent) != "" {
				*steps = append(*steps, aiStep{N: step, Type: "model", Output: aiTruncateSteps(ultimoContent) + " (recuperado de paso anterior)", At: aiRunnerStartedAt()})
				return ultimoContent, nil
			}
			*steps = append(*steps, aiStep{N: step, Type: "model", Output: aiTruncateSteps(content), At: aiRunnerStartedAt()})
			return content, nil
		}
		// Registra la petición del modelo y ejecuta CADA herramienta propuesta
		// en el servidor (validada por rol en aiDispatchTool).
		messages = append(messages, map[string]string{"role": "assistant", "content": content})
		var toolResults []string
		for _, tc := range toolCalls {
			payload := aiToolPayload{Name: tc.Name, Args: aiArgumentosDeHerramienta(tc.Arguments, tc.Name)}
			out, execErr := s.aiDispatchTool(sess, agent, payload, journal, gate)
			if execErr != nil {
				out = string(mustJSON(aiToolError{execErr.Error()}))
			}
			*steps = append(*steps, aiStep{N: step, Type: "tool", Name: payload.Name, Input: aiTruncateSteps(tc.Arguments), Output: aiTruncateSteps(out), At: aiRunnerStartedAt()})
			toolResults = append(toolResults, fmt.Sprintf("Resultado de %s:\n%s", payload.Name, out))
		}
		messages = append(messages, map[string]string{"role": "user", "content": strings.Join(toolResults, "\n\n") + "\n\nContinúa la tarea o entrega el informe final."})
	}
	if strings.TrimSpace(ultimoContent) != "" {
		return ultimoContent, nil
	}
	return "", fmt.Errorf("se alcanzó el máximo de %d pasos sin informe final", maxSteps)
}

// aiArgumentosDeHerramienta lee los argumentos que Proposed el modelo en un
// `tool_call`.
//
// POR QUÉ NO ES UN SIMPLE `json.Unmarshal` SOBRE `aiToolPayload`.
//
// La API manda los argumentos como un objeto PLANO: para `db.query` es
// `{"sql":"SELECT ...","limit":50}`, no `{"name":"db.query","args":{...}}`. Al
// desempaquetarlo contra la estructura interna, `Name` y `Args` salían vacíos
// siempre, y todas las herramientas se ejecutaban sin parámetros.
//
// Medido: el agente pedía el esquema, luego llamaba ocho veces a `db.query` y
// las ocho respondían "consulta vacía". El guardián funcionaba, el mapa técnico
// llegaba al modelo, y aun así no se podía consultar NADA.
//
// Se aceptan las dos formas porque algunos proveedores envuelven los
// argumentos: si viene un `args` anidado, se usa ese.
//
// `herramienta` es el nombre con el que se registró la herramienta. Hace falta
// para no comerse un parámetro legítimo: `report.save_template` tiene un
// parámetro llamado `name`, y el código quitaba SIEMPRE la clave `name` del
// payload. Medido: el agente consultaba la base bien (su parámetro es `sql`, que
// no colisiona) y a la hora de guardar la plantilla recibía "Nombre y definition
// son obligatorios" tres veces seguidas, sin que hubiera ningún `name` que
// explicar el fallo.
func aiArgumentosDeHerramienta(texto, herramienta string) map[string]interface{} {
	vacio := map[string]interface{}{}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return vacio
	}
	var plano map[string]interface{}
	if err := json.Unmarshal([]byte(texto), &plano); err != nil {
		return vacio
	}
	if anidados, ok := plano["args"].(map[string]interface{}); ok && len(anidados) > 0 {
		return anidados
	}
	// `name` es el nombre de la herramienta SOLO si su valor coincide con el
	// nombre con el que se registró. Con cualquier otro valor es un parámetro
	// de la herramienta y se queda.
	if v, ok := plano["name"].(string); ok && aiNombreDeHerramientaCoincide(herramienta, v) {
		delete(plano, "name")
	}
	return plano
}

// aiNombreDeHerramientaCoincide compara el nombre que el modelo puso en el
// payload con el de la herramienta, tolerando el punto y el guion bajo (los dos
// formatos conviven según el proveedor; ver aiNombreDeHerramientaParaAPI).
//
// La normalización es: minúsculas, puntos y guiones bajos equivalentes, sin
// espacios. Se comparan los DOS lados con la misma regla a propósito: comparar
// solo uno contra sus dos formas hacía que `aiNombreDeHerramientaParaAPI("db_query")`
// devolviera "db_query" y todo cuadrara consigo mismo.
func aiNombreDeHerramientaCoincide(registrada, enviada string) bool {
	return normalizarNombreDeHerramienta(registrada) == normalizarNombreDeHerramienta(enviada)
}

func normalizarNombreDeHerramienta(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	n = strings.ReplaceAll(n, ".", "_")
	n = strings.ReplaceAll(n, "-", "_")
	return strings.TrimSpace(n)
}

// aiOperatorHint aclara al modelo qué fase tiene disponible.
func aiOperatorHint(agent aiAgent) string {
	if agent.OperatorEnabled == 1 {
		return " y, si las instrucciones lo piden, usa las herramientas de operador (borradores de factura, cobros, vouchers)"
	}
	return ""
}

// aiTruncateSteps recorta textos largos de la bitácora para no inflar la BD.
func aiTruncateSteps(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1200 {
		return s[:1200] + "…"
	}
	return s
}

// aiProviderForAgent devuelve el proveedor con el que trabaja un agente.
//
// Es SIEMPRE el proveedor ACTIVO de Configuración IA, nunca la fila antigua que
// pudiera apuntar ai_agent.provider_id: el administrador enciende uno de los
// tres y con eso trabaja todo el sistema, agentes incluidos. Dejar que cada
// agente conservara su propia fila era la vía silenciosa para que siguiera
// hablando con un proveedor que alguien ya apagó. La URL sale de la whitelist
// compilada igual que antes (aiVendorURL): la columna base_url se ignora.
func (s *Server) aiProviderForAgent(sess *SessionData, agent aiAgent) (aiProvider, bool) {
	p, ok := s.loadActiveProvider(sess)
	if !ok || !aiProviderUsable(p) {
		return aiProvider{}, false
	}
	return p, true
}

// aiRunScheduled fue el planificador original: ejecutaba las tareas vencidas
// una a una en el mismo goroutine, sin cola ni límites de memoria. Ahora la
// planificación vive en worker.go (enqueueDueAITasks) y la ejecución la hace
// el motor de cola, con pool fijo, backpressure y persistencia en disco.
//
// Se conserva como punto de entrada manual (disparo inmediato desde la UI),
// pero ejecuta de forma síncrona y acotada por el llamador.

// aiScheduleDue evalúa daily/weekly/cron5 (cada 5 min).
func aiScheduleDue(t aiTask, nowT time.Time) bool {
	switch t.ScheduleType {
	case "daily":
		return nowT.Hour() == parseHour(t.ScheduleValue, 6)
	case "weekly":
		day := parseHour(t.ScheduleValue, 1)
		return int(nowT.Weekday()) == day && nowT.Hour() == 6
	case "cron5":
		return true
	}
	return false
}

func parseHour(v string, def int) int {
	if n, ok := aiAtoiSafe(strings.TrimSpace(v)); ok && n >= 0 && n <= 23 {
		return n
	}
	return def
}
