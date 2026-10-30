package web

// informes_ia_crea_test.go — La creación de informes por IA GUARDA un informe
// real, no un borrador en un modal.
//
// POR QUÉ ESTO SE PRUEBA: la pantalla tenía el botón «Ayúdame con la IA» que
// solo mostraba texto y, si acaso, rellenaba el textarea si estaba vacío. El
// pedido era que la IA creara informes reales en el sistema, y eso no se
// cumplía: nada guardaba la fila. Aquí se demuestra que el camino nuevo
// (crearInformeDesdeRespuestaIA) extrae la definición del texto del agente,
// la pasa por el MISMO guardián que un guardado manual, la mete en
// report_template con la marca ia: en created_by y deja su rastro en
// audit_log — y que una definición con SQL de escritura no se cuela.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// crearTablaReportTemplatePrueba arma la tabla que el esquema real trae de
// migración y que el andamio de prueba (cachas_test.go) no incluye.
func crearTablaReportTemplatePrueba(t *testing.T, db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS report_template (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT, description TEXT, definition TEXT,
		created_by TEXT, status INTEGER DEFAULT 1,
		created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatalf("crear tabla report_template de prueba: %v", err)
	}
}

// reqConCuerpo es reqPrueba con body JSON, montado con el mismo ámbito que
// attachScope — sin él, sessionFromRequest se queda sin identidad.
func reqConCuerpo(r *http.Request, cuerpo string) *http.Request {
	req := httptest.NewRequest(r.Method, r.URL.RequestURI(), strings.NewReader(cuerpo))
	return withScope(req, scopeOf(r))
}

// respuestaIAIndependiente es lo que un agente real contesta: prosa, vallado
// de código y el objeto JSON en medio.
func respuestaIAIndependiente(definicion string) string {
	return "Analizando el pedido del cliente. Qué he hecho:\n\n" +
		"```json\n" + definicion + "\n```\n\n" +
		"El informe está listo para revisar. La tabla invoice es la de facturas.\n"
}

// TestLaExtraccionSacaLaDefinicionDeLaRespuestaDelAgente: la prosa alrededor no
// contamina y un objeto de la misma respuesta que NO es la definición (por
// ejemplo un resumen sin «sql») tampoco. Un regex goloso falla los dos casos.
func TestLaExtraccionSacaLaDefinicionDeLaRespuestaDelAgente(t *testing.T) {
	definicion := `{"title":"Ventas por cliente","sql":"SELECT c.customer_name AS cliente FROM invoice i JOIN customer_information c ON c.customer_id = i.customer_id","columns":[{"key":"cliente","label":"Cliente"}],"totals":[]}`
	conResumen := `Resumen del trabajo: {"pasos": 3, "tablas_usadas": ["invoice"]}. Y aquí va la definición:\n` + definicion
	casos := []string{
		definicion,
		respuestaIAIndependiente(definicion),
		"Aquí lo tienes:\n" + conResumen,
	}
	for i, caso := range casos {
		d, ok := aiExtraerDefinicion(caso)
		if !ok {
			t.Errorf("caso %d: la extracción no encontró la definición", i)
			continue
		}
		var leido struct {
			SQL string `json:"sql"`
		}
		if err := json.Unmarshal([]byte(d), &leido); err != nil {
			t.Errorf("caso %d: lo extraído no decodifica: %v (%q)", i, err, d)
			continue
		}
		if leido.SQL == "" {
			t.Errorf("caso %d: se extrajo un objeto sin «sql»: %q", i, d)
		}
	}
	// El SQL dentro de la definición lleva comillas escapadas de JSON: el
	// escáner las respeta (no se abre de las comillas por counting bruto) y
	// las llaves que sigan dentro no desajustan el emparejado.
	definicionDura := `{"title":"Contacto","sql":"SELECT CASE WHEN c.customer_name = 'valor \"especial\"' THEN 1 ELSE 0 END AS cuenta FROM customer_information c","columns":[{"key":"cuenta","label":"Cuenta"}],"totals":[]}`
	if d, ok := aiExtraerDefinicion(respuestaIAIndependiente(definicionDura)); !ok {
		t.Errorf("la definición con llaves dentro del SQL no se extrajo")
	} else if strings.Contains(d, "tablas_usadas") {
		t.Errorf("se extrajo basura: %q", d)
	}
	// Nada parecido a un informe → nada extraído.
	if _, ok := aiExtraerDefinicion("El agente no pudo generar la definición porque no hay datos suficientes en el mapa."); ok {
		t.Errorf("un texto sin JSON no debe extraer nada")
	}
}

// TestLaIACreaElInformeEnElSistema: con la respuesta del agente en la mano, el
// informe parace fila real en report_template con created_by ia:— y con el
// nombre del «title» de la definición, no del largo pedido.
func TestLaIACreaElInformeEnElSistema(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	sess := sessPrueba("admin")
	r := reqPrueba(s, sess, "/reports/custom/ai-create")
	respuesta := respuestaIAIndependiente(`{"title":"Ventas por cliente","sql":"SELECT c.customer_name AS cliente FROM invoice i JOIN customer_information c ON c.customer_id = i.customer_id","columns":[{"key":"cliente","label":"Cliente"}],"totals":[]}`)

	id, nombre, err := s.crearInformeDesdeRespuestaIA(r, sess, "informe de ventas de este mes por cliente", respuesta)
	if err != nil {
		t.Fatalf("la IA debía crear el informe y no pudo: %v", err)
	}
	if id <= 0 {
		t.Fatalf("el informe creado no tiene id: %d", id)
	}
	if nombre != "Ventas por cliente" {
		t.Errorf("el nombre sale del title de la definición, no del pedido largo: %q", nombre)
	}
	var creador, guardada string
	if err := db.QueryRow(`SELECT IFNULL(created_by,''), definition FROM report_template WHERE id = ?`, id).
		Scan(&creador, &guardada); err != nil {
		t.Fatalf("la fila del informe creado no existe: %v", err)
	}
	if creador != "ia:7" {
		// La misma convención del guardado manual (user:7): el id del usuario,
		// con el prefijo que dice que esta fila la redactó la IA.
		t.Errorf("created_by debe llevar la marca de la IA con el id del usuario: %q", creador)
	}
	if !strings.Contains(guardada, `"sql"`) {
		t.Errorf("la definición guardada no es la del informe: %q", guardada)
	}
	var trazas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE table_name = 'report_template' AND record_id = ? AND action = 'create' AND module = 'custom_reports'`, id).
		Scan(&trazas); err != nil || trazas != 1 {
		t.Errorf("la creación por IA debe dejar su rastro en audit_log: %v trazas=%d", err, trazas)
	}
}

// TestElGuardianSeComeElSQLDeEscrituraDeLaIA: si el agente propone un INSERT,
// el informe no se guarda — y no queda fila huérfana con la mitad del camino.
func TestElGuardianSeComeElSQLDeEscrituraDeLaIA(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	r := reqPrueba(s, sessPrueba("admin"), "/reports/custom/ai-create")
	respuesta := respuestaIAIndependiente(`{"title":"Borrador tramposo","sql":"DELETE FROM invoice","columns":[{"key":"x","label":"X"}],"totals":[]}`)

	_, _, err := s.crearInformeDesdeRespuestaIA(r, sessPrueba("admin"), "limpiar facturas", respuesta)
	if err == nil {
		t.Fatal("el guardián debe rechazar el SQL de escritura de la IA")
	}
	var filas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM report_template`).Scan(&filas); err != nil || filas != 0 {
		t.Errorf("no debe quedar fila del informe rechazado: err=%v filas=%d", err, filas)
	}
}

// TestElPedidoVacioNoLLegaAlAgente: sin descripción no se gasta un paso de
// agente ni se intenta guardar nada.
func TestElPedidoVacioNoLLegaAlAgente(t *testing.T) {
	s, _ := basePrueba(t)
	w := httptest.NewRecorder()
	req := reqPrueba(s, sessPrueba("admin"), "/reports/custom/ai-create")
	s.reportTemplateAICreate(w, reqConCuerpo(req, `{"request":""}`))
	if w.Code != 400 {
		t.Errorf("un pedido vacío responde 400, llegó %d: %s", w.Code, w.Body.String())
	}
}
