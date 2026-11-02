package web

// ai_reports_un_paso_test.go — El asistente de informes genera la definición
// COMPLETA (SQL + columnas + totales) en UN SOLO PROMPT: reglas harcodeadas en
// el system prompt + la solicitud del usuario + el esquema entero de la base,
// cacheado por huella, documentado (descripciones, comentarios de campo,
// joins). Solo si esa definición falla la prueba se pide UNA corrección.
//
// POR QUÉ ESTA PRUEBA EXISTE: antes el flujo gastaba 3-4 llamadas al proveedor
// (fase de clasificación de módulos, paso de SQL, paso de columnas) y cada
// reintento sumaba otra; con el esquema completo entregado de una vez, esa
// coreografía es innecesaria. Lo que aquí se comprueba:
//
//  1. El caso bueno es UNA llamada, sin herramientas, con el contrato y el
//     esquema en el prompt.
//  2. Una definición que falla la prueba de ejecución tiene UNA corrección
//     llevando el fallo real en el prompt, y la segunda respuesta se acepta.
//  3. El refine recibe el SQL previo y lo ajusta en vez de regenerarlo.
//  4. El contrato del prompt es el mismo candado harcodeado para todos.
//  5. La prosa alrededor del JSON no pierde la definición.

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// llamadaChat captura el cuerpo de una petición al proveedor falso.
type llamadaChat struct {
	Cuerpo map[string]interface{}
}

// servidorDeInformeUnPaso finge el proveedor: responde según el orden de
// llamada, para poder simular «primero una definición que falla, después la
// corregida».
func servidorDeInformeUnPaso(t *testing.T, respuestas []string) (*httptest.Server, *[]llamadaChat) {
	t.Helper()
	var mu sync.Mutex
	llamadas := &[]llamadaChat{}
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var cuerpo map[string]interface{}
		_ = json.Unmarshal(raw, &cuerpo)
		mu.Lock()
		*llamadas = append(*llamadas, llamadaChat{Cuerpo: cuerpo})
		n := i
		if i < len(respuestas)-1 {
			i++
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":` +
			mustJSON(respuestas[n]) + `}}],"usage":{"prompt_tokens":10,"completion_tokens":50,"total_tokens":60}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, llamadas
}

// promptDelCuerpo devuelve el texto del ÚLTIMO mensaje del request (el del
// usuario: el primero es el system prompt del contrato).
func promptDelCuerpo(cuerpo map[string]interface{}) string {
	msgs, _ := cuerpo["messages"].([]interface{})
	if len(msgs) == 0 {
		return ""
	}
	m, _ := msgs[len(msgs)-1].(map[string]interface{})
	s, _ := m["content"].(string)
	return s
}

// sinHerramientas comprueba que la petición NO llevó «tools»: el flujo de un
// solo prompt no usa tool-calling y el modelo no debe distraerse con él.
func sinHerramientas(t *testing.T, cuerpo map[string]interface{}) {
	t.Helper()
	if _, ok := cuerpo["tools"]; ok {
		t.Error("la generación en un solo prompt no debe enviar herramientas")
	}
}

// servidorConAgenteIA levanta el andamio mínimo para iaGenerarInforme: la base
// de prueba, un agente de TI activo y su proveedor activo.
func servidorConAgenteIA(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ai_agent (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT, user_id TEXT, provider_id INTEGER, model_override TEXT,
		role_id INTEGER, operator_enabled INTEGER DEFAULT 0,
		max_steps INTEGER, max_rows INTEGER, enabled INTEGER DEFAULT 1,
		created_by TEXT, created_at TEXT,
		department TEXT, config_prompt TEXT, config_prompt_at TEXT)`); err != nil {
		t.Fatalf("crear ai_agent de prueba: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ai_agent (name, user_id, provider_id, role_id,
		max_steps, max_rows, enabled, department)
		VALUES ('Agente TI', '7', 1, 0, 8, 5000, 1, 'it')`); err != nil {
		t.Fatalf("sembrar agente TI: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ai_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT, base_url TEXT, api_key TEXT, model TEXT,
		temperature REAL, max_tokens INTEGER, prompt_template TEXT,
		is_active INTEGER, vendor TEXT, op_cache INTEGER)`); err != nil {
		t.Fatalf("crear ai_providers de prueba: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ai_providers (name, base_url, api_key, model,
		temperature, max_tokens, is_active, vendor, op_cache)
		VALUES ('Proveedor prueba', 'https://api.openai.com/v1', 'clave-del-proveedor', 'gpt-5.5',
		0.7, 400, 1, 'openai', 0)`); err != nil {
		t.Fatalf("sembrar proveedor activo: %v", err)
	}
	return s, db
}

// definicionVentasPorCliente es lo que un modelo bien portado devuelve: el
// objeto JSON del contrato, completo, y con vallado de código para demostrar
// que la limpieza no descarta nada.
func definicionVentasPorCliente() string {
	return `{"title":"Ventas por cliente","sql":"SELECT 'Cliente Demo' AS cliente","columns":[{"key":"cliente","label":"Cliente"}],"totals":[]}`
}

// TestElInformeSeGeneraEnUnSoloPrompt: el caso bueno es UNA llamada y el
// prompt lleva el contrato, la solicitud y el esquema completo, sin
// herramientas; el JSON recibido se convierte en informe con su preview.
func TestElInformeSeGeneraEnUnSoloPrompt(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	srv, llamadas := servidorDeInformeUnPaso(t, []string{
		"Aquí tienes el informe:\n```json\n" + definicionVentasPorCliente() + "\n```",
	})
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	s, _ := servidorConAgenteIA(t)
	def, name, _, preview, err := s.iaGenerarInforme(sessPrueba("admin"), "clientes con ventas de este mes", "")
	if err != nil {
		t.Fatalf("la generación en un solo prompt falló: %v", err)
	}
	if len(*llamadas) != 1 {
		t.Fatalf("el caso bueno es UNA llamada al proveedor, hubo %d", len(*llamadas))
	}
	for _, ll := range *llamadas {
		sinHerramientas(t, ll.Cuerpo)
	}
	if name != "Ventas por cliente" {
		t.Errorf("el nombre sale del title del contrato: %q", name)
	}
	if len(def.Columns) != 1 || def.Columns[0].Key != "cliente" {
		t.Errorf("las columnas del contrato no llegaron: %+v", def.Columns)
	}
	if preview == nil || len(preview.Rows) == 0 {
		t.Errorf("el preview debía ejecutar el SQL en la base de prueba: %+v", preview)
	}
	prompt := promptDelCuerpo((*llamadas)[0].Cuerpo)
	for _, esperado := range []string{"SOLICITUD DEL USUARIO", "ESQUEMA COMPLETO"} {
		if !strings.Contains(prompt, esperado) {
			t.Errorf("el prompt único debe llevar %q", esperado)
		}
	}
	msgs, _ := (*llamadas)[0].Cuerpo["messages"].([]interface{})
	if len(msgs) < 2 {
		t.Fatalf("el prompt único lleva system (contrato) + user (solicitud y esquema): %d mensajes", len(msgs))
	}
	sys, _ := msgs[0].(map[string]interface{})
	sysText, _ := sys["content"].(string)
	if !strings.Contains(sysText, "CONTRATO DE RESPUESTA") || !strings.Contains(sysText, ":desde") {
		t.Error("el contrato harcodeado va en el system prompt: con sus reglas de fechas")
	}
}

// TestLaDefinicionMalaSeCorrigeEnElSegundoPrompt: si lo que la IA entregó no
// pasa la prueba de ejecución, la ÚNICA corrección lleva el fallo real (y el
// JSON previo) en el prompt; la segunda respuesta se acepta y el flujo termina.
func TestLaDefinicionMalaSeCorrigeEnElSegundoPrompt(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	defMala := `{"title":"Ventas","sql":"SELECT inventado AS monto FROM tabla_que_no_existe","columns":[{"key":"monto","label":"Monto"}],"totals":["monto"]}`
	srv, llamadas := servidorDeInformeUnPaso(t, []string{
		defMala,
		definicionVentasPorCliente(),
	})
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	s, _ := servidorConAgenteIA(t)
	def, _, _, preview, err := s.iaGenerarInforme(sessPrueba("admin"), "ventas por cliente", "")
	if err != nil {
		t.Fatalf("la corrección debía acabarlo: %v", err)
	}
	if len(*llamadas) != 2 {
		t.Fatalf("mala + corrección son EXACTAMENTE 2 llamadas, hubo %d", len(*llamadas))
	}
	correccion := promptDelCuerpo((*llamadas)[1].Cuerpo)
	for _, esperado := range []string{"TU DEFINICIÓN ANTERIOR FALLÓ", "EL FALLO QUE DEVOLVIÓ EL MOTOR", "tabla_que_no_existe"} {
		if !strings.Contains(correccion, esperado) {
			t.Errorf("el prompt de corrección debe llevar %q (el fallo real, no uno genérico)", esperado)
		}
	}
	if len(def.Columns) != 1 || def.Columns[0].Key != "cliente" {
		t.Errorf("tras la corrección quedó la definición mala: %+v", def.Columns)
	}
	if preview == nil || len(preview.Rows) == 0 {
		t.Errorf("el preview corregido debía ejecutar: %+v", preview)
	}
}

// TestElSQLPrevioViajaEnElRefine: el ajuste recibe el SQL previo y lo modifica
// según el feedback, en vez de regenerarlo desde cero.
func TestElSQLPrevioViajaEnElRefine(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	srv, llamadas := servidorDeInformeUnPaso(t, []string{
		`{"title":"Solo ventas grandes","sql":"SELECT 'Cliente Demo' AS cliente","columns":[{"key":"cliente","label":"Cliente"}],"totals":[]}`,
	})
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	s, _ := servidorConAgenteIA(t)
	_, _, _, _, err := s.iaGenerarInforme(sessPrueba("admin"), "solo ventas grandes", "SELECT 'Cliente Demo' AS cliente")
	if err != nil {
		t.Fatalf("el refine falló: %v", err)
	}
	if len(*llamadas) != 1 {
		t.Fatalf("el refine del caso bueno es una llamada, hubo %d", len(*llamadas))
	}
	prompt := promptDelCuerpo((*llamadas)[0].Cuerpo)
	if !strings.Contains(prompt, "SQL PREVIO DEL INFORME") {
		t.Error("el refine debe decirle al modelo cuál es el SQL previo")
	}
	if !strings.Contains(prompt, "SELECT 'Cliente Demo' AS cliente") {
		t.Error("el SQL previo debe viajar completo en el prompt del refine")
	}
}

// TestElContratoEsElMismoCandadoParaTodos: el contrato harcodeado del prompt
// no puede aflojarse sin que alguien lo note: key = alias del SELECT, totales
// dentro de columns, :desde/:hasta como únicos parámetros, sin markdown.
func TestElContratoEsElMismoCandadoParaTodos(t *testing.T) {
	for _, esperado := range []string{
		"alias EXACTO", "totals", ":desde", ":hasta", "NINGÚN otro parámetro",
		"sin markdown", `{"title"`,
	} {
		if !strings.Contains(aiContratoInforme, esperado) {
			t.Errorf("el contrato harcodeado perdió la regla %q", esperado)
		}
	}
	system, user := iaPromptInforme("pedidos del mes", `{"tablas":[]}`, "")
	if !strings.Contains(system, aiContratoInforme) {
		t.Error("el contrato va en el system prompt: la IA no puede negociarlo")
	}
	if !strings.Contains(user, "pedidos del mes") || !strings.Contains(user, `{"tablas":[]}`) {
		t.Error("el user prompt debe llevar la solicitud y el esquema entero")
	}
}

// TestLaRespuestaConProsaSacaElJSON: si el modelo rodea el objeto con prosa a
// pesar del contrato, el escáner de definiciones lo salva antes de perder la
// corrida entera.
func TestLaRespuestaConProsaSacaElJSON(t *testing.T) {
	def, err := iaRespuestaADefinicion("Meta: diseñar el pedido.\n" + definicionVentasPorCliente() + "\nListo para revisar.")
	if err != nil {
		t.Fatalf("la prosa alrededor no debe perder la definición: %v", err)
	}
	if def.Title != "Ventas por cliente" {
		t.Errorf("la definición extraída no es la del contrato: %q", def.Title)
	}
	if def.SQL == "" || len(def.Columns) == 0 {
		t.Errorf("las claves mínimas del contrato no llegaron: %q / %v", def.SQL, def.Columns)
	}
	if _, err := iaRespuestaADefinicion("no pude generar el informe, tome más tokens en razonar"); err == nil {
		t.Error("una respuesta sin definición debe fallar (y disparar su corrección)")
	}
}
