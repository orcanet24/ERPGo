package web

// addons_scaffold_test.go — Pruebas del RAD mínimo de addons (fases A+B).
//
// Lo que protegen, sin adornos:
//
//   - El generador NO tiene puerta trasera: lo que escribe pasa por
//     validarManifiesto y por el candado de schema.sql, los mismos del escáner.
//   - No pisa una carpeta existente ni sale de addons/ con un id raro.
//   - El main.go generado es Go válido (se formatea y se parsea) y, cuando el
//     servidor tiene Go, compila de verdad con las dependencias del módulo.
//   - El POST del panel crea los archivos y re-escanea en caliente.

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestElGeneradorCreaUnAddonQuePasaLosCandados(t *testing.T) {
	dirBase := t.TempDir()
	sol := addonScaffoldSolicitud{
		ID:          "notas",
		Nombre:      "Notas rápidas",
		Version:     "1.0.0",
		Descripcion: "Un addon de prueba con tabla, lectura y hook.",
		Autor:       "Prueba",
		Tabla:       "notas_registros",
		DBRead:      []string{"customer_information", "products"},
		HooksOut:    []string{"factura.emitida"},
	}
	res, err := addonScaffoldCrear(dirBase, sol)
	if err != nil {
		t.Fatalf("el generador rechazó una solicitud válida: %v", err)
	}
	if res.ID != "notas" || res.Dir != filepath.Join(dirBase, "notas") {
		t.Fatalf("el resultado no apunta a la carpeta esperada: %+v", res)
	}

	// Manifiesto: se lee del disco y pasa el candado REAL del escáner.
	crudo, err := os.ReadFile(filepath.Join(res.Dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m addonManifest
	if err := json.Unmarshal(crudo, &m); err != nil {
		t.Fatalf("el manifiesto generado no es JSON válido: %v", err)
	}
	if err := validarManifiesto(&m); err != nil {
		t.Fatalf("el manifiesto generado no pasa validarManifiesto: %v", err)
	}
	if m.ID != "notas" || m.MinCoreVersion != addonCoreVersion {
		t.Fatalf("el manifiesto no quedó con el id y el mínimo del núcleo: %+v", m)
	}
	if len(m.DBWrite) != 0 {
		t.Fatalf("el generador no debe inventar verbos de escritura; quedaron %v", m.DBWrite)
	}
	if len(m.TablesOwn) != 1 || m.TablesOwn[0] != "notas_registros" {
		t.Fatalf("tables_own quedó %v", m.TablesOwn)
	}
	if len(m.HooksOut) != 1 || m.HooksOut[0] != "factura.emitida" {
		t.Fatalf("hooks_out quedó %v", m.HooksOut)
	}

	// schema.sql: el mismo candado que aplica el escáner al arrancar.
	if _, err := addonMigracionesDeCarpeta(res.Dir, &m); err != nil {
		t.Fatalf("el schema.sql generado no pasa el candado: %v", err)
	}

	// main.go: Go sintácticamente válido.
	fuente, err := os.ReadFile(filepath.Join(res.Dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", fuente, parser.AllErrors); err != nil {
		t.Fatalf("el main.go generado no es Go válido: %v", err)
	}
	if !strings.Contains(string(fuente), `idAddon     = "notas"`) {
		t.Fatalf("el main.go no quedó con el id generado:\n%s", fuente)
	}
}

func TestElGeneradorNoPisaNiEscapaDeLaCarpeta(t *testing.T) {
	dirBase := t.TempDir()
	valida := addonScaffoldSolicitud{ID: "notas", Nombre: "Notas"}

	// Primera creación: ocupa el id.
	if _, err := addonScaffoldCrear(dirBase, valida); err != nil {
		t.Fatal(err)
	}
	if _, err := addonScaffoldCrear(dirBase, valida); err == nil || !strings.Contains(err.Error(), "ya existe") {
		t.Fatalf("el generador debía negarse a pisar una carpeta existente; dio: %v", err)
	}

	casos := []struct {
		nombre string
		media  func(*addonScaffoldSolicitud)
		que    string
	}{
		{"id con traversal", func(s *addonScaffoldSolicitud) { s.ID = "../fuera" }, "id"},
		{"id con mayúsculas", func(s *addonScaffoldSolicitud) { s.ID = "Notas" }, "id"},
		{"tabla sin prefijo del id", func(s *addonScaffoldSolicitud) { s.Tabla = "otra_tabla" }, "prefijo"},
		{"ruta del núcleo", func(s *addonScaffoldSolicitud) { s.Ruta = "/module" }, "del núcleo"},
		{"permiso sin prefijo del id", func(s *addonScaffoldSolicitud) { s.Permiso = "ver" }, "prefijo"},
		{"hook que el núcleo no emite", func(s *addonScaffoldSolicitud) { s.HooksOut = []string{"evento.inventado"} }, "no es un evento"},
		{"tabla de lectura con nombre raro", func(s *addonScaffoldSolicitud) { s.DBRead = []string{"customers; DROP TABLE users"} }, "db_read"},
	}
	for _, c := range casos {
		sol := valida
		sol.ID = "otro"
		c.media(&sol)
		if _, err := addonScaffoldCrear(dirBase, sol); err == nil {
			t.Errorf("%s: el generador aceptó %s", c.nombre, c.que)
		}
	}
	if _, err := os.Stat(filepath.Join(dirBase, "..", "fuera")); err == nil {
		t.Fatal("el generador creó una carpeta fuera de addons/ con un id de traversal")
	}
}

func TestElPanelDelGeneradorCreaYReescanea(t *testing.T) {
	dirBase := t.TempDir()
	s, _ := hostDeAddon(t, dirBase)

	form := url.Values{}
	form.Set("id", "notas")
	form.Set("nombre", "Notas rápidas")
	form.Set("descripcion", "Creado desde el panel")
	form.Set("tabla", "notas_registros")
	req := httptest.NewRequest(http.MethodPost, "/module/addons/scaffold", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.addonPanelScaffold(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("el generador debía responder 200, dio %d: %s", rr.Code, rr.Body.String())
	}
	var salida struct {
		Status    int                    `json:"status"`
		Msg       string                 `json:"msg"`
		Resultado addonScaffoldResultado `json:"resultado"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &salida); err != nil {
		t.Fatalf("la respuesta no es JSON válido: %v\n%s", err, rr.Body.String())
	}
	if salida.Status != 1 || salida.Resultado.ID != "notas" {
		t.Fatalf("la respuesta no confirma la creación: %+v", salida)
	}
	if _, err := os.Stat(filepath.Join(dirBase, "notas", "manifest.json")); err != nil {
		t.Fatalf("no quedó el manifiesto en la carpeta: %v", err)
	}

	// El re-escaneo lo dejó en el estado del panel. En un TempDir no hay go.mod,
	// así que el binario no se compila: el estado debe decir «sin binario», no
	// fingir que está activo.
	estado := s.addonsEstado()
	lista, ok := estado["addons"].([]map[string]interface{})
	if !ok || len(lista) != 1 || lista[0]["id"] != "notas" {
		t.Fatalf("el addon generado no quedó en el estado del panel: %v", estado["addons"])
	}
	if lista[0]["estado"] != estadoSinBin {
		t.Fatalf("sin compilación el estado debía ser %q, quedó %v", estadoSinBin, lista[0]["estado"])
	}
}

func TestElCodigoGeneradoCompila(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("sin el comando go en PATH no se puede probar la compilación")
	}
	raiz := raizDelRepo(t)
	dirBase := filepath.Join(raiz, "addons")
	id := fmt.Sprintf("pruebagen%d", time.Now().UnixNano())
	sol := addonScaffoldSolicitud{
		ID:          id,
		Nombre:      "Prueba generada",
		Descripcion: "Compila con tabla, lectura y hook.",
		Tabla:       id + "_registros",
		DBRead:      []string{"customer_information"},
		HooksOut:    []string{"factura.emitida"},
	}
	res, err := addonScaffoldCrear(dirBase, sol)
	if err != nil {
		t.Fatalf("no se pudo generar el addon de prueba: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.Dir) })

	ok, salida := addonScaffoldCompilar(dirBase, id)
	if !ok {
		t.Fatalf("el código generado no compiló:\n%s", salida)
	}
	binario := filepath.Join(res.Dir, "erp-addon-"+id)
	if runtime.GOOS == "windows" {
		binario += ".exe"
	}
	if _, err := os.Stat(binario); err != nil {
		t.Fatalf("la compilación dijo que sí pero no quedó el binario: %v", err)
	}
}

// raizDelRepo sube desde el directorio de la prueba hasta el go.mod del ERP.
// El generador compila con la raíz del módulo como directorio de trabajo.
func raizDelRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Fatal("no se encontró go.mod subiendo desde el directorio de la prueba")
		}
		dir = padre
	}
}
