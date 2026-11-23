package web

// mcp_bridge_test.go — El puente MCP (M-1) se prueba contra MySQL de verdad:
// la tabla de tokens vive en la base de CONTROL y su esquema es DDL de MySQL
// (patrón de esquemassdecodigo_test.go); probarlo contra SQLite no diría
// nada del motor real. Sin MySQL disponible, la prueba se salta — un test
// que no puede probar lo que dice no debe pintarse de verde ni de rojo.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo/infrastructure/persistence"
)

// Cada subprueba monta su propia base MySQL (nombre único, limpieza propia):
// compartir una sola cadena sería más rápido pero acoplaría los casos entre
// sí —la prueba de revocación dejaría a las demás sin token si alguien
// reordena el archivo—. El coste (3 cadenas cuando hay MySQL; 0 cuando no,
// que es el defecto en CI) es el precio de que cada caso sea honesto solo.
func baseMCPPrueba(t *testing.T) (*Server, *sql.DB, string) {
	t.Helper()
	db, limpiar, err := persistence.NuevaBaseVaciaDePrueba("mcptest")
	if err != nil {
		t.Skipf("no hay MySQL para probar el puente MCP (esquema de control es DDL MySQL): %v", err)
	}
	t.Cleanup(limpiar)
	// Cadena REAL de MySQL (el puente habla de la base de control, que es
	// MySQL de origen; RunMigrationsFor es el camino SQLite y mintió una vez,
	// en la prueba del ledger Error 1101). Con cfg vacío el runner no abre la
	// segunda conexión rápida y aplica sentencia a sentencia: más lento,
	// mismo resultado probado.
	if err := persistence.RunMigrationsForMySQL(db, persistence.ConfigMySQL{}); err != nil {
		t.Fatalf("cadena de migraciones mysql: %v", err)
	}
	s, _ := basePrueba(t)
	s.db = db
	s.controlDB = db
	s.controlListo = false
	// La tabla de tokens nace del esquema de control, como en el arranque
	// real: se fuerza una vez aquí para poder sembrar el token de prueba.
	if _, err := s.controlStore(); err != nil {
		t.Fatalf("esquema de control: %v", err)
	}

	// Agente IT de prueba: departamento IT (el scope que la frontera de
	// informes respeta) y modo operador OFF — las herramientas de escritura
	// no deben aparecer en el catálogo que anuncia el puente.
	if _, err := db.Exec(`INSERT INTO ai_agent (name, user_id, provider_id, role_id, department, operator_enabled, max_steps, max_rows, enabled, created_by, created_at)
		VALUES ('MCP Prueba', '', 1, 0, 'IT', 0, 4, 100, 1, 'admin', '2026-11-23 00:00:00')`); err != nil {
		t.Fatalf("sembrando agente: %v", err)
	}
	token := "erpgo_mcp_tokendeprueba000000000000000000000000000000000000000000"
	sum := sha256.Sum256([]byte(token))
	if _, err := db.Exec(`INSERT INTO mcp_tokens (company_code, agent_id, name, token_hash, created_at)
		VALUES ('default', 1, 'llave de prueba', ?, '2026-11-23 00:00:00')`, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("sembrando token: %v", err)
	}
	return s, db, token
}
func mcpPost(t *testing.T, s *Server, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	s.handleMCP(w, r)
	return w
}

func TestMCPInitializeYToolsList(t *testing.T) {
	s, _, token := baseMCPPrueba(t)

	w := mcpPost(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("initialize HTTP %d: %s", w.Code, w.Body.String())
	}
	var init struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &init); err != nil {
		t.Fatalf("initialize no es JSON-RPC válido: %v — %s", err, w.Body.String())
	}
	if init.Result.ProtocolVersion != mcpProtocolVersion || init.Result.ServerInfo.Name != "erpgo" {
		t.Errorf("handshake: %+v", init.Result)
	}

	w = mcpPost(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, token)
	var lista struct {
		Result struct {
			Tools []struct {
				Name        string                 `json:"name"`
				InputSchema map[string]interface{} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &lista); err != nil {
		t.Fatalf("tools/list: %v — %s", err, w.Body.String())
	}
	nombres := map[string]bool{}
	for _, tl := range lista.Result.Tools {
		nombres[tl.Name] = true
		if tl.InputSchema == nil {
			t.Errorf("la herramienta %s se anuncia sin inputSchema (un cliente MCP estricto la rechaza)", tl.Name)
		}
	}
	if !nombres["db_query"] {
		t.Errorf("db.query debe anunciarse como db_query (guion bajo, sin punto) — llegó %v", nombres)
	}
	// El agente NO es operador: las herramientas de escritura no se anuncian
	// (mismo predicado que el guardián: catálogo y despacho no discrepan).
	if nombres["payment_record"] {
		t.Error("payment_record (operador) se anunció a un agente sin modo operador")
	}
}

func TestMCPCallConsultaYGuardianes(t *testing.T) {
	s, _, token := baseMCPPrueba(t)

	// Llamada legítima: leer una tabla de negocio.
	w := mcpPost(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"db_query","arguments":{"sql":"SELECT COUNT(*) AS n FROM invoice"}}}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("tools/call HTTP %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("respuesta no parseable: %v — %s", err, w.Body.String())
	}
	if res.Result.IsError {
		t.Errorf("la lectura simple fue devuelta como error: %s", res.Result.Content[0].Text)
	}
	if !strings.Contains(res.Result.Content[0].Text, "n") {
		t.Errorf("el resultado no trae la columna pedida: %s", res.Result.Content[0].Text)
	}

	// Nivel 1: herramienta fuera del catálogo → blocked (no error de
	// transporte: el agente debe poder distinguirlo).
	w = mcpPost(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"borrar_todo","arguments":{}}}`, token)
	if !strings.Contains(w.Body.String(), `"blocked":true`) && !strings.Contains(w.Body.String(), `\`+`"blocked`+`":true`) {
		// el texto viene doblado dentro de content[].text (JSON dentro de JSON)
		if !strings.Contains(w.Body.String(), "blocked") {
			t.Errorf("herramienta inexistente no pasó por el guardián: %s", w.Body.String())
		}
	}

	// La tabla de identidad sigue proscrita aunque el token sea IT (la lista
	// negra dura no depende del rol).
	w = mcpPost(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"db_query","arguments":{"sql":"SELECT password FROM user_login"}}}`, token)
	if !strings.Contains(w.Body.String(), "blocked") && !strings.Contains(w.Body.String(), "prohibido") &&
		!strings.Contains(w.Body.String(), "no se permite") && !strings.Contains(w.Body.String(), "Error") {
		t.Errorf("user_login por MCP: la frontera debe rechazarla; llegó %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "$2") {
		t.Error("el puente devolvió un hash de contraseña — frontera rota")
	}
}

func TestMCPAuthYTransporte(t *testing.T) {
	s, db, token := baseMCPPrueba(t)

	// Sin token ni con token malo: 401 ANTES de mirar el método.
	if w := mcpPost(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("sin token: HTTP %d, se quería 401", w.Code)
	}
	if w := mcpPost(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, "no-existe"); w.Code != http.StatusUnauthorized {
		t.Errorf("token falso: HTTP %d, se quería 401", w.Code)
	}

	// Token revocado: 401 (no se distingue de inválido, para no sondear).
	if _, err := db.Exec(`UPDATE mcp_tokens SET revoked = 1`); err != nil {
		t.Fatal(err)
	}
	// La base es compartida con las otras subpruebas: se RESTAURA el token al
	// terminar, si no el orden de los tests dentro del archivo sería una
	// dependencia oculta (mañana alguien añade un test y "misteriosamente"
	// empieza en 401).
	t.Cleanup(func() {
		if _, err := db.Exec(`UPDATE mcp_tokens SET revoked = 0`); err != nil {
			t.Errorf("restaurando el token tras la prueba de revocación: %v", err)
		}
	})
	if w := mcpPost(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, token); w.Code != http.StatusUnauthorized {
		t.Errorf("token revocado: HTTP %d, se quería 401", w.Code)
	}

	// GET responde texto útil (405), no un 404 mudo.
	r := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()
	s.handleMCP(w, r)
	if w.Code != http.StatusMethodNotAllowed || !strings.Contains(w.Body.String(), "POST") {
		t.Errorf("GET /mcp: %d %q", w.Code, w.Body.String())
	}
}

func TestMCPRutaPublicaDeTransporte(t *testing.T) {
	// /mcp es pública de TRANSPORTE (el bearer autentica dentro), y el
	// catálogo de rutas la conoce: sin esto, permissionMiddleware la mandaría
	// al 403 de «sin clasificar» y el puente no arranca ni a portazo limpio.
	if !esRutaPublica("/mcp") {
		t.Error("/mcp debe estar en publicRoutes (autentica su propio bearer)")
	}
	clase, _ := clasificarRuta("/mcp")
	if clase == "sin-clasificar" {
		t.Error("/mcp quedó sin clasificar")
	}
}
