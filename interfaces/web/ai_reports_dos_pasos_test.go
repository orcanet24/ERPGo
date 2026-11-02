package web

// ai_reports_dos_pasos_test.go — El asistente de informes ya NO usa un agente
// con herramientas: genera la definición en DOS PASOS DIRECTOS (1: SQL, 2:
// columnas/totales) con prompts pequeños y específicos.
//
// POR QUÉ ESTA PRUEBA EXISTE: el enfoque anterior montaba un agente que recibía
// el esquema de la BD y el catálogo de herramientas; los modelos que razonan
// (LongCat-2.0) se quedaban pensando hasta agotar el presupuesto de tokens y
// nunca escribían el JSON del informe. El usuario lo vivió durante días: la IA
// se cortaba una y otra vez y el timeout la mataba antes de responder.
//
// Lo que aquí se comprueba:
//  1. Las llamadas de generación NO llevan herramientas (tools ausente).
//  2. Un SQL que el guardián rechaza se reintenta UNA vez con el error en el
//     prompt, y la segunda respuesta se acepta.
//  3. El refine recibe el SQL previo y lo ajusta en vez de regenerar desde cero.
//  4. Los vallados ```sql / ```json se limpian antes de usar la respuesta.

import (
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

// servidorDeInformeDosPasos finge el proveedor: responde según el orden de
// llamada, para poder simular «primero un SQL malo, después uno bueno».
func servidorDeInformeDosPasos(t *testing.T, respuestas []string) (*httptest.Server, *[]llamadaChat) {
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
// usuario: el primero es el system prompt fijo).
func promptDelCuerpo(cuerpo map[string]interface{}) string {
	msgs, _ := cuerpo["messages"].([]interface{})
	if len(msgs) == 0 {
		return ""
	}
	m, _ := msgs[len(msgs)-1].(map[string]interface{})
	s, _ := m["content"].(string)
	return s
}

// sinHerramientas comprueba que la petición NO llevó «tools»: el flujo directo
// no usa el agente con tool-calling que se quedaba sin tokens.
func sinHerramientas(t *testing.T, cuerpo map[string]interface{}) {
	t.Helper()
	if _, ok := cuerpo["tools"]; ok {
		t.Error("la generación directa no debe enviar herramientas: el modelo se perdía razonando sobre ellas")
	}
}

// TestLaGeneracionDirectaNoUsaHerramientasYReintentaConElErrorDelGuardian:
// reproduce el fallo del :agrupar (SQL que el guardián rechaza) y verifica que
// el reintento corrige en el acto, sin perder la corrida.
func TestLaGeneracionDirectaNoUsaHerramientasYReintentaConElErrorDelGuardian(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	srv, llamadas := servidorDeInformeDosPasos(t, []string{
		// Primer intento: el modelo inventa un parámetro no soportado.
		"SELECT CASE :agrupar WHEN 'dia' THEN fecha END AS dia FROM ventas",
		// Segundo intento: el guardián le dijo qué falló y corrige.
		"SELECT DATE_FORMAT(fecha,'%Y-%m-%d') AS dia FROM ventas",
	})
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	p := proveedorDePrueba("openai", "https://api.openai.com/v1", "clave-del-proveedor", "gpt-5.5")
	var s Server
	sql, err := s.iaGenerateSQL(p, "gpt-5.5", "ventas por día", "ventas: tabla de ventas", "")
	if err != nil {
		t.Fatalf("la generación directa falló: %v", err)
	}
	if !strings.Contains(sql, "DATE_FORMAT") {
		t.Errorf("el SQL final debía ser el corregido; llegó %q", sql)
	}
	if len(*llamadas) != 2 {
		t.Fatalf("debía haber exactamente 2 llamadas (intento + corrección), hubo %d", len(*llamadas))
	}
	for i, ll := range *llamadas {
		sinHerramientas(t, ll.Cuerpo)
		prompt := promptDelCuerpo(ll.Cuerpo)
		if !strings.Contains(prompt, "ESQUEMA DE LAS TABLAS RELEVANTES") {
			t.Errorf("llamada %d: el prompt debe incluir el esquema scopado", i+1)
		}
	}
	if !strings.Contains(promptDelCuerpo((*llamadas)[1].Cuerpo), "FALLÓ LA VALIDACIÓN") {
		t.Error("la segunda llamada debe llevar el error del guardián para que el modelo corrija")
	}
}

// TestElSQLPrevioViajaEnElRefine: el ajuste recibe el SQL previo y lo modifica
// según el feedback, en vez de regenerarlo desde cero.
func TestElSQLPrevioViajaEnElRefine(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	srv, llamadas := servidorDeInformeDosPasos(t, []string{
		"SELECT fecha AS dia FROM ventas WHERE total > 100",
	})
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	p := proveedorDePrueba("openai", "https://api.openai.com/v1", "clave-del-proveedor", "gpt-5.5")
	var s Server
	sql, err := s.iaGenerateSQL(p, "gpt-5.5", "solo ventas grandes", "ventas: tabla de ventas", "SELECT fecha AS dia FROM ventas")
	if err != nil {
		t.Fatalf("el refine falló: %v", err)
	}
	if len(*llamadas) != 1 {
		t.Fatalf("el refine debía ser una sola llamada, hubo %d", len(*llamadas))
	}
	prompt := promptDelCuerpo((*llamadas)[0].Cuerpo)
	if !strings.Contains(prompt, "SQL PREVIO") {
		t.Error("el refine debe decirle al modelo cuál es el SQL previo")
	}
	if !strings.Contains(prompt, "SELECT fecha AS dia FROM ventas") {
		t.Error("el SQL previo debe viajar completo en el prompt del refine")
	}
	if !strings.Contains(sql, "total > 100") {
		t.Errorf("el SQL ajustado no llegó: %q", sql)
	}
}

// TestLimpiaValladosMarkdown: los modelos rodean el SQL y el JSON con ``` y el
// motor no puede leer eso; la limpieza es parte del contrato.
func TestLimpiaValladosMarkdown(t *testing.T) {
	casosSQL := []struct{ in, want string }{
		{"```sql\nSELECT 1 AS uno\n```", "SELECT 1 AS uno"},
		{"```\nSELECT 2 AS dos\n```", "SELECT 2 AS dos"},
		{"  SELECT 3 AS tres  ", "SELECT 3 AS tres"},
	}
	for _, c := range casosSQL {
		if got := aiLimpiarSQL(c.in); got != c.want {
			t.Errorf("aiLimpiarSQL(%q) = %q; quería %q", c.in, got, c.want)
		}
	}
	casosJSON := []struct{ in, want string }{
		{"```json\n" + `{"title":"X"}` + "\n```", `{"title":"X"}`},
		{"```\n" + `{"a":1}` + "\n```", `{"a":1}`},
	}
	for _, c := range casosJSON {
		if got := aiLimpiarJSON(c.in); got != c.want {
			t.Errorf("aiLimpiarJSON(%q) = %q; quería %q", c.in, got, c.want)
		}
	}
}
