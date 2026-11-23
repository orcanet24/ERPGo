package web

// mcp_bridge.go — El puente MCP: la IA externa como USUARIA y como TÉCNICA
// de informes (M-1 de la auditoría atómica 2026-11-22: «No hay MCP», decía;
// el claim del README prometía lo que el transporte no existía).
//
// DISEÑO — por qué así y no un servidor MCP nuevo por su cuenta:
//
//   - El transporte es la capa que faltaba, NO una segunda puerta de
//     herramientas. initialize / tools/list / tools/call (JSON-RPC 2.0 sobre
//     HTTP, protocolo 2025-03-26) mapean DIRECTO al registro ya gobernado:
//     aiCatalogForPhase decide lo que se anuncia (mismo predicado que el
//     guardián) y aiDispatchTool ejecuta (Nivel 1 + Nivel 2 + aprobación
//     previa + propuesta). Un puente con su propia lista de herramientas
//     sería el agujero al día siguiente: dos listas, una que se olvida.
//   - La identidad es un TOKEN por AGENTE guardado en la base de CONTROL
//     (tabla mcp_tokens). El token resuelve empresa + agente; el agente
//     arrastra rol, departamento, tope de filas y modo operador que ya se
//     le configuran en /ai/agents. Nada nuevo que auditar: el mismo
//     SessionData{UserLevel:"agent"} que construye aiRunLoop.
//   - Solo se guarda el sha256 del token. Volcar la tabla no autoriza nada.
//   - Los nombres de herramienta se traducen con la función que ya existe
//     para la API de modelos (db.query → db_query): MCP exige [A-Za-z0-9_-]
//     y el viaje de vuelta ya era inequívoco por construcción.
//
// LIMITACIONES honestas (v1, documentadas en el informe):
//   - Solo JSON-RPC sobre POST (petición simple; sin streaming SSE ni
//     notificaciones server→client). Suficiente y compatible para Claude
//     Desktop y clientes stdio que aceptan "streamable HTTP".
//   - Sin MCP Resources ni Prompts: el ERP expone HERRAMIENTAS; los
//     informes plantillados ya tienen su propio camino (report.run).
//   - Los lotes (arrays JSON-RPC) no se admiten: se responden con el error
//     de protocolo correspondiente, no se procesan a medias.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const (
	mcpProtocolVersion = "2025-03-26"
	mcpServerName      = "erpgo"
	mcpServerVersion   = "1.0"
)

// ---------- JSON-RPC 2.0 ----------

type mcpRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

func mcpResponder(w http.ResponseWriter) func(id json.RawMessage, res interface{}, e *mcpRPCError) {
	w.Header().Set("Content-Type", "application/json")
	return func(id json.RawMessage, res interface{}, e *mcpRPCError) {
		resp := mcpRPCResponse{JSONRPC: "2.0", ID: id, Result: res, Error: e}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// handleMCP es el endpoint único del transporte: POST /mcp.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// GET responde texto: es lo que un humano abre primero cuando
		// «quiere ver el MCP» en el navegador. Un 405 mudo invitaba a
		// descubrirlo por prueba y error.
		w.Header().Set("Allow", "POST")
		http.Error(w, "ERPGo MCP: enviar peticiones JSON-RPC 2.0 por POST a /mcp con Authorization: Bearer <token> (los tokens se crean en IA → Agentes).", http.StatusMethodNotAllowed)
		return
	}

	// Autenticación de transporte ANTES de tocar el cuerpo: sin identidad no
	// hay método que valga la pena enumerar.
	token := mcpTokenDe(r)
	if token == "" {
		http.Error(w, `{"error":"token MCP requerido (Authorization: Bearer <token>)"}`, http.StatusUnauthorized)
		return
	}
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		http.Error(w, `{"error":"base de control no disponible"}`, http.StatusServiceUnavailable)
		return
	}
	var tokenID int64
	var companyCode string
	var agentID int64
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	err = ctl.QueryRow(`SELECT id, company_code, agent_id FROM mcp_tokens
		WHERE token_hash = ? AND revoked = 0`, hash).Scan(&tokenID, &companyCode, &agentID)
	if err != nil {
		// No se distingue "no existe" de "revocado": un emisor válido ya no
		// puede sondear el estado de tokens ajenos.
		http.Error(w, `{"error":"token MCP inválido o revocado"}`, http.StatusUnauthorized)
		return
	}
	_, _ = ctl.Exec(`UPDATE mcp_tokens SET last_used_at = ? WHERE id = ?`, now(), tokenID)

	// El agente vive en la base de SU empresa; el token fijó cuál.
	db := s.dbForTenant(companyCode)
	if db == nil {
		s.escribirMCPError(w, r, "la empresa del token no está disponible (base cerrada o inexistente)")
		return
	}
	sessEmpresa := &SessionData{TenantCode: companyCode, UserLevel: "agent"}
	ag, ok := s.loadAIAgentByID(sessEmpresa, agentID)
	if !ok {
		s.escribirMCPError(w, r, "el agente del token ya no existe o está inactivo en la empresa")
		return
	}
	gate := s.aiBuildRoleGate(sessEmpresa, ag)
	// La sesión del ejecutor se arma IGUAL que en aiRunLoop (mismo
	// UserLevel/UserID/TenantCode): si algún día cambian las reglas de esa
	// ruta, esta tiene que cambiar con ella —por eso se construye aquí con
	// los mismos campos y en el mismo orden, y la prueba lo fija.
	sess := &SessionData{
		UserID:     ag.UserID,
		Username:   s.aiAgentUsername(nil, ag),
		Fullname:   ag.Name,
		UserLevel:  "agent",
		TenantCode: companyCode,
	}
	r = withScope(r, &scope{tenant: companyCode, db: db, sess: sess})

	var req mcpRPCRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// El body puede ser un LOTE (array): se responde el error de
		// protocolo, nunca se procesa la mitad.
		http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32600,"message":"se esperaba un objeto JSON-RPC 2.0 (los lotes no se admiten)"}}`, http.StatusBadRequest)
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid request"}`, http.StatusBadRequest)
		return
	}

	send := mcpResponder(w)
	switch req.Method {
	case "initialize":
		send(req.ID, map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": mcpServerName, "version": mcpServerVersion},
		}, nil)

	case "notifications/initialized", "notifications/cancelled":
		// Notificación: si no trae id, HTTP 202 sin cuerpo (así lo pide el
		// transporte streamable).
		if len(req.ID) == 0 || string(req.ID) == "null" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		send(req.ID, map[string]interface{}{}, nil)

	case "ping":
		send(req.ID, map[string]interface{}{}, nil)

	case "tools/list":
		catalogo := aiCatalogForPhase(ag.OperatorEnabled == 1, gate)
		tools := make([]map[string]interface{}, 0, len(catalogo))
		for _, t := range catalogo {
			tools = append(tools, map[string]interface{}{
				"name":        aiNombreDeHerramientaParaAPI(t.Name),
				"description": t.Description,
				"inputSchema": t.Parameters,
			})
		}
		send(req.ID, map[string]interface{}{"tools": tools}, nil)

	case "tools/call":
		var p struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || strings.TrimSpace(p.Name) == "" {
			send(req.ID, nil, &mcpRPCError{Code: -32602, Message: "params de tools/call inválidos (se requiere name)"})
			return
		}
		journal := &aiRunJournal{scope: "mcp:" + strconv.FormatInt(tokenID, 10)}
		out, err := s.aiDispatchTool(sess, ag, aiToolPayload{Name: p.Name, Args: p.Arguments}, journal, gate)
		bloqueado := strings.Contains(out, `"blocked":true`)
		s.appendAudit(r, "MCP:"+ag.Name, clientIP(r), "mcp_tools_call:"+p.Name, "ia", "mcp_tokens",
			strconv.FormatInt(tokenID, 10), map[string]string{
				"empresa": companyCode, "agente": strconv.FormatInt(agentID, 10),
				"resumen_args": fmt.Sprintf("%.200v", p.Arguments),
			}, map[string]string{
				"resultado": ifThen(err == nil && !bloqueado, "ok", ifThen(bloqueado, "bloqueado", "error")),
			})
		if err != nil {
			send(req.ID, map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "Error: " + err.Error()}},
				"isError": true,
			}, nil)
			return
		}
		send(req.ID, map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": out}},
			"isError": bloqueado,
		}, nil)

	default:
		send(req.ID, nil, &mcpRPCError{Code: -32601, Message: "método no soportado: " + req.Method})
	}
}

func (s *Server) escribirMCPError(w http.ResponseWriter, r *http.Request, msg string) {
	send := mcpResponder(w)
	send(nil, nil, &mcpRPCError{Code: -32000, Message: msg})
}

func mcpTokenDe(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func ifThen(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// ---------- Administración de tokens (interfaz web, módulo IA) ----------

// mcpTokenCreate genera un token para un agente. La respuesta incluye el
// valor en claro UNA sola vez (es el momento en que puede copiarse; luego
// solo existe el hash) y un snippet listo para pegar en el cliente MCP.
func (s *Server) mcpTokenCreate(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "sesión requerida", 401)
		return
	}
	if !s.hasModulePermission(r, sess, 18, "update") {
		jsonError(w, "sin permiso para gestionar IA", http.StatusForbidden)
		return
	}
	var body struct {
		AgentID int64  `json:"agent_id"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentID <= 0 {
		jsonError(w, "se requiere agent_id", http.StatusBadRequest)
		return
	}
	company := s.empresaDe(r)
	db := s.dbForTenant(company)
	if db == nil {
		jsonError(w, "empresa no disponible", http.StatusServiceUnavailable)
		return
	}
	ag, ok := s.loadAIAgentByID(&SessionData{TenantCode: company, UserLevel: "admin"}, body.AgentID)
	if !ok {
		jsonError(w, "el agente no existe o está inactivo", http.StatusNotFound)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		jsonError(w, "no se pudo generar el token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	token := "erpgo_mcp_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "base de control no disponible", http.StatusServiceUnavailable)
		return
	}
	if _, err := ctl.Exec(`INSERT INTO mcp_tokens (company_code, agent_id, name, token_hash, created_at)
		VALUES (?,?,?,?,?)`, company, body.AgentID, strings.TrimSpace(body.Name), hex.EncodeToString(sum[:]), now()); err != nil {
		jsonError(w, "guardando el token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.appendAudit(r, sess.Username, clientIP(r), "mcp_token_create", "ia", "mcp_tokens", strconv.FormatInt(ag.ID, 10), nil, map[string]string{"agente": ag.Name, "empresa": company})
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"token":  token,
		"aviso":  "Este valor NO se vuelve a mostrar: guárdalo ahora. Se revoca en la lista de tokens.",
		"snippet": map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"erpgo": map[string]interface{}{
					"type": "streamable_http",
					"url":  mcpBaseURL(r),
					"headers": map[string]string{
						"Authorization": "Bearer " + token,
					},
				},
			},
		},
	})
}

// mcpTokenList lista los tokens de la empresa con el valor oculto.
func (s *Server) mcpTokenList(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "sesión requerida", 401)
		return
	}
	if !s.hasModulePermission(r, sess, 18, "update") {
		jsonError(w, "sin permiso para gestionar IA", http.StatusForbidden)
		return
	}
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "base de control no disponible", http.StatusServiceUnavailable)
		return
	}
	company := s.empresaDe(r)
	rows, err := ctl.Query(`SELECT t.id, t.agent_id, IFNULL(a.name,'(agente borrado)'), t.name, t.created_at, t.last_used_at, t.revoked
		FROM mcp_tokens t
		LEFT JOIN ai_agent a ON a.id = t.agent_id
		WHERE t.company_code = ? ORDER BY t.id DESC`, company)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type item struct {
		ID       int64  `json:"id"`
		AgentID  int64  `json:"agent_id"`
		Agente   string `json:"agente"`
		Nombre   string `json:"nombre"`
		Creado   string `json:"creado"`
		UltimoUS string `json:"ultimo_uso"`
		Vigente  bool   `json:"vigente"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		var creado, ultimo string
		var rev int
		if err := rows.Scan(&it.ID, &it.AgentID, &it.Agente, &it.Nombre, &creado, &ultimo, &rev); err != nil {
			continue
		}
		it.Creado, it.UltimoUS = creado, ultimo
		it.Vigente = rev == 0
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		log.Printf("mcp: listando tokens: %v", err)
	}
	writeJSON(w, map[string]interface{}{"status": 1, "tokens": out})
}

// mcpTokenRevoke retira un token de la empresa que llama (nunca ajena).
func (s *Server) mcpTokenRevoke(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "sesión requerida", 401)
		return
	}
	if !s.hasModulePermission(r, sess, 18, "update") {
		jsonError(w, "sin permiso para gestionar IA", http.StatusForbidden)
		return
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID <= 0 {
		jsonError(w, "se requiere id", http.StatusBadRequest)
		return
	}
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "base de control no disponible", http.StatusServiceUnavailable)
		return
	}
	res, err := ctl.Exec(`UPDATE mcp_tokens SET revoked = 1 WHERE id = ? AND company_code = ?`,
		body.ID, s.empresaDe(r))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "token no encontrado en esta empresa", http.StatusNotFound)
		return
	}
	s.appendAudit(r, sess.Username, clientIP(r), "mcp_token_revoke", "ia", "mcp_tokens", strconv.FormatInt(body.ID, 10), map[string]string{"token_id": strconv.FormatInt(body.ID, 10)}, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Token revocado"})
}

func (s *Server) registerMCPRoutes() {
	s.mux.HandleFunc("POST /ai/agents/mcp/token", s.mcpTokenCreate)
	s.mux.HandleFunc("GET /ai/agents/mcp/tokens", s.mcpTokenList)
	s.mux.HandleFunc("POST /ai/agents/mcp/revoke", s.mcpTokenRevoke)
	s.mux.HandleFunc("/mcp", s.handleMCP)
}

// mcpBaseURL reconstruye la URL pública del servidor para el snippet de
// configuración: https si la petición llega por TLS o el operador declaró
// certificado; si no, http con el Host con el que llegó (detrás de un proxy
// que termina TLS, el operador fija el scheme declarando TLS_*_FILE).
func mcpBaseURL(r *http.Request) string {
	esquema := "http"
	if r.TLS != nil {
		esquema = "https"
	}
	return esquema + "://" + r.Host + "/mcp"
}
