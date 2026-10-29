package web

// Pruebas de la FORMA de las respuestas que comen las tablas de IA.
//
// DataTables, con la configuración que usan estas pantallas, busca las filas en
// la propiedad «data» de la respuesta (sAjaxDataProp, que por defecto es
// «data»). Si el handler devuelve una matriz suelta en la raíz, la librería no
// encuentra «data», le llega undefined y revienta dentro de su propio callback
// de éxito con «Cannot read properties of undefined (reading 'length')»: la
// tabla queda vacía y el único aviso es un error de consola que nadie relaciona
// con el listado. El contrato, por eso, se prueba aquí y no a ojo.
//
// La excepción está también probada: /reports/custom/data NO se envuelve en el
// servidor porque report_engine.html lo recorre como matriz en la raíz; allí es
// su plantilla la que le dice a DataTables dónde están las filas.

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo"
)

// filasDeDataTables exige que la respuesta sea un objeto con «data» y que ahí
// haya una matriz de filas: exactamente lo que DataTables lee por defecto.
func filasDeDataTables(t *testing.T, cuerpo string) []json.RawMessage {
	t.Helper()
	var envoltorio map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cuerpo), &envoltorio); err != nil {
		t.Fatalf("la respuesta no es un objeto JSON con envoltura de DataTables: %v (%s)", err, cuerpo)
	}
	crudo, ok := envoltorio["data"]
	if !ok {
		t.Fatalf("falta la propiedad «data» que DataTables busca por defecto: %s", cuerpo)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(crudo)), "[") {
		t.Fatalf("«data» debe ser una matriz de filas: %s", cuerpo)
	}
	var filas []json.RawMessage
	if err := json.Unmarshal(crudo, &filas); err != nil {
		t.Fatalf("«data» no admite leerse como filas: %v (%s)", err, cuerpo)
	}
	return filas
}

// TestLasListasDeIaRespondenLaEnvolturaDeDataTables es la regresión del error
// «Cannot read properties of undefined (reading 'length')» que aparecía en la
// consola al abrir /ai/agents: la tabla se quedaba vacía y el listado no
// cargaba. Cubre también /ai/tasks, que tenía exactamente el mismo defecto.
func TestLasListasDeIaRespondenLaEnvolturaDeDataTables(t *testing.T) {
	s, _ := basePrueba(t)

	casos := []struct {
		nombre string
		ruta   string
		llamar func(http.ResponseWriter, *http.Request)
	}{
		{"agentes", "/ai/agents/data", s.agentListData},
		{"tareas", "/ai/tasks/data", s.aiTaskListData},
	}
	for _, c := range casos {
		w := httptest.NewRecorder()
		c.llamar(w, reqPrueba(s, sessPrueba("admin"), c.ruta))
		if w.Code != http.StatusOK {
			t.Errorf("%s: el listado respondió %d: %s", c.nombre, w.Code, w.Body.String())
			continue
		}
		if filas := filasDeDataTables(t, w.Body.String()); filas == nil {
			t.Errorf("%s: «data» no es una matriz de filas: %s", c.nombre, w.Body.String())
		}
	}
}

// TestLaListaDeInformesConservaLaMatrizEnLaRaiz: este endpoint es el único que
// se come desde dos sitios y report_engine.html lo recorre desde la raíz. Si un
// día se envuelve en «data», la pantalla del motor deja de listar informes en
// silencio; por eso la regla queda escrita junto a su plantilla.
func TestLaListaDeInformesConservaLaMatrizEnLaRaiz(t *testing.T) {
	s, _ := basePrueba(t)

	w := httptest.NewRecorder()
	s.reportTemplateListData(w, reqPrueba(s, sessPrueba("admin"), "/reports/custom/data"))
	if w.Code != http.StatusOK {
		t.Fatalf("la lista de informes respondió %d: %s", w.Code, w.Body.String())
	}
	var filas []json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &filas); err != nil {
		t.Fatalf("el endpoint debe seguir devolviendo la matriz en la raíz, "+
			"que es como la recorre report_engine.html: %v (%s)", err, w.Body.String())
	}

	// Y su tabla le dice a DataTables dónde están las filas, porque con la
	// configuración por defecto miraría «data» y no encontraría nada.
	plantilla, err := fs.ReadFile(erpgo.Templates(), "templates/report_template_list.html")
	if err != nil {
		t.Fatalf("no se pudo leer la plantilla de informes: %v", err)
	}
	if !strings.Contains(string(plantilla), "dataSrc: ''") {
		t.Error("report_template_list.html dejó de decirle a DataTables que las filas " +
			"están en la raíz (dataSrc: ''): la tabla revienta al cargar")
	}
}
