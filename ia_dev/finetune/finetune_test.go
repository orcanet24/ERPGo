package main

// finetune_test.go — El dataset de fine-tuning no puede enseñarle al modelo lo
// que el guardián de producción rechazaría. Aquí se comprueba LO IMPORTANTE:
//
//  1. La lista de tablas fiscales que este generador usa es la MISMA que vive
//     en interfaces/web/ai_reports.go (contra el archivo real del repo).
//  2. El contrato extraído del repo real traiga el candado de fechas.
//  3. La sanción de SQL y la validación de definición reprodúzcense tal cual.
//  4. Una corrida entera contra una base empresa de prueba produce los cuatro
//     .jsonl y EXCLUYE lo fiscal.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

// repoRaizPrueba devuelve la raíz del repo desde el directorio del paquete de
// pruebas (ia_dev/finetune): dos niveles más arriba.
func repoRaizPrueba(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolver la raíz del repo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "interfaces", "web", "ai_reports.go")); err != nil {
		t.Fatalf("no se encontró interfaces/web/ai_reports.go en %s", abs)
	}
	return abs
}

// Test 1: el generador y el guardián de producción llevan la MISMA lista de
// tablas fiscales. Si alguien edita validateReportSQLwithSchema sin tocar
// aquí (o al revés), esta prueba lo delata.
func TestTablasFiscalesAlineadasConElGuardianReal(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRaizPrueba(t), "interfaces", "web", "ai_reports.go"))
	if err != nil {
		t.Fatalf("leer ai_reports.go real: %v", err)
	}
	source := string(raw)
	for _, tabla := range tablasFiscalesProhibidas {
		if !strings.Contains(source, `"`+tabla+`"`) {
			t.Errorf("la tabla fiscal %q ya NO está en validateReportSQLwithSchema del archivo real: actualizar tablasFiscalesProhibidas", tabla)
		}
	}
	// La alineación inversa: nuestra lista no puede ser MÁS CHICA que la del
	// guardián. Contamos las entradas del slice literal del guardián.
	inicio := strings.Index(source, "tablasProhibidas := []string{")
	if inicio < 0 {
		t.Fatal("no se halló el literal tablasProhibidas := []string{ en ai_reports.go")
	}
	fin := strings.Index(source[inicio:], "}")
	if fin < 0 {
		t.Fatal("el literal tablasProhibidas no cierra")
	}
	literalGuardian := source[inicio : inicio+fin]
	for _, tabla := range tablasFiscalesProhibidas {
		if !strings.Contains(literalGuardian, `"`+tabla+`"`) {
			t.Errorf("la tabla %q está en el generador pero NO en el guardián del archivo real", tabla)
		}
	}
}

// Test 2: el contrato extraído del REPO REAL es el que producción usa.
func TestContratoDelRepoReal(t *testing.T) {
	contrato, err := extraerContrato(repoRaizPrueba(t))
	if err != nil {
		t.Fatalf("extraerContrato del repo real: %v", err)
	}
	// El candado de fechas :desde/:hasta es el punto de tropiezo histórico:
	// sin él en el system prompt, el modelo rompe el SQL con parámetros de más.
	if !strings.Contains(contrato, ":desde") || !strings.Contains(contrato, ":hasta") {
		t.Error("el contrato real debe fijar el candado de fechas :desde/:hasta")
	}
}

// Test 3: validación de definición, caso por caso.
func TestValidarDefinicion(t *testing.T) {
	buena := []byte(`{"title":"Ventas","sql":"SELECT cliente, SUM(total) AS total FROM invoice GROUP BY cliente","columns":[{"key":"cliente","label":"Cliente"},{"key":"total","label":"Total"}],"totals":["total"],"chart":{"type":"bar","x_key":"cliente","y_key":"total"}}`)
	if motivo := validarDefinicion(buena); motivo != "" {
		t.Fatalf("una definición válida fue rechazada: %s", motivo)
	}
	casosMalos := map[string]string{
		"json roto":           `{"title":`,
		"sql vacío":           `{"title":"x","sql":"","columns":[{"key":"a","label":"A"}]}`,
		"sin columnas":        `{"title":"x","sql":"SELECT 1","columns":[]}`,
		"total fuera":         `{"title":"x","sql":"SELECT a","columns":[{"key":"a","label":"A"}],"totals":["b"]}`,
		"chart type malo":     `{"title":"x","sql":"SELECT a","columns":[{"key":"a","label":"A"}],"chart":{"type":"scatter3d","x_key":"a","y_key":"a"}}`,
		"chart sin y_key":     `{"title":"x","sql":"SELECT a","columns":[{"key":"a","label":"A"}],"chart":{"type":"bar","x_key":"a","y_key":""}}`,
		"chart key declarada": `{"title":"x","sql":"SELECT a","columns":[{"key":"a","label":"A"}],"chart":{"type":"bar","x_key":"a","y_key":"zzz"}}`,
	}
	for nombre, caso := range casosMalos {
		if motivo := validarDefinicion([]byte(caso)); motivo == "" {
			t.Errorf("caso %q: se aceptó y debe rechazarse", nombre)
		}
	}
}

// Test 4: la sanción de SQL rechaza lo mismo que el guardián.
func TestSancionarSQL(t *testing.T) {
	normal := "SELECT i.customer_name AS cliente, SUM(i.total_amount) AS total FROM invoice i GROUP BY cliente"
	if motivo := sancionarSQL(normal); motivo != "" {
		t.Fatalf("SQL operacional sano fue sancionado: %s", motivo)
	}
	if motivo := sancionarSQL(`SELECT * FROM acc_transaction`); motivo == "" {
		t.Error("acc_transaction (tabla fiscal) debe sanearse")
	}
	if motivo := sancionarSQL(`SELECT a FROM invoice JOIN acc_vaucher v ON v.x = a.id`); motivo == "" {
		t.Error("JOIN con acc_vaucher debe sanearse")
	}
	if motivo := sancionarSQL(`SELECT igtf FROM invoice`); motivo == "" {
		t.Error("palabra fiscal (igtf) sin agregación debe sanearse")
	}
	if motivo := sancionarSQL(`SELECT cliente, SUM(igtf) AS igtf FROM invoice GROUP BY cliente`); motivo != "" {
		t.Errorf("palabra fiscal DENTRO de agregación es entrenable y fue sancionada: %s", motivo)
	}
}

// Test 5: asignación de módulo por las tablas que el SQL toca.
func TestAsignarModulo(t *testing.T) {
	modulos := []modulo{
		{Key: "ventas", Tables: []string{"invoice", "invoice_details"}},
		{Key: "inventario", Tables: []string{"warehouse", "product_lot"}},
	}
	if m := asignarModulo("SELECT a FROM invoice_details d JOIN invoice i ON i.id = d.invoice_id", modulos); m != "ventas" {
		t.Errorf("sql de ventas asignado a %q", m)
	}
	if m := asignarModulo("SELECT a FROM tabla_que_no_existe", modulos); m != "desconocido" {
		t.Errorf("sql huérfano asignado a %q", m)
	}
}

// Test 6: las variantes son deterministas con la misma semilla y nunca
// superan el banco construido.
func TestVariantesDePregunta(t *testing.T) {
	p := &plantillaExportada{
		ID:          11,
		Nombre:      "Ventas por cliente",
		Descripcion: "Ventas del periodo por cliente con total.",
		Modulo:      "ventas",
	}
	a := variantesDePregunta(p, 42, 3)
	b := variantesDePregunta(p, 42, 3)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("variantes inconsistentes: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("la semilla 42 no es determinista en la variante %d: %q vs %q", i, a[i], b[i])
		}
	}
	if n := variantesDePregunta(p, 42, 99); len(n) > 3 {
		t.Errorf("nunca se deben pedir más variantes que el banco: %d", len(n))
	}
	// Sin pedir nada que no estaba en la fila: cada variante menciona el
	// nombre, la descripción o el módulo que la arma.
	for i, v := range a {
		if !strings.Contains(v, "Ventas por cliente") && !strings.Contains(v, "cliente") && !strings.Contains(v, "ventas") && !strings.Contains(v, "Ventas") {
			t.Errorf("variante %d menciona contenido inventado: %q", i, v)
		}
	}
}

// Test 7: la fila de entrenamiento imita el prompt REAL: contrato en system,
// solicitud + esquema en user, definición pura en assistant.
func TestFilaEntreno(t *testing.T) {
	fila := filaEntreno("CONTRATO DE RESPUESTA — reglas", `{"modulo":"ventas","tablas":[]}`, "Quiero ventas por cliente", `{"title":"x","sql":"SELECT 1","columns":[]}`)
	mensajes, ok := fila["messages"].([]map[string]string)
	if !ok || len(mensajes) != 3 {
		t.Fatalf("la fila debe tener 3 mensajes (system/user/assistant), tiene %+v", fila)
	}
	if mensajes[0]["role"] != "system" || !strings.Contains(mensajes[0]["content"], "CONTRATO") {
		t.Error("el contrato real va en el system")
	}
	if mensajes[1]["role"] != "user" || !strings.Contains(mensajes[1]["content"], "SOLICITUD DEL USUARIO") || !strings.Contains(mensajes[1]["content"], "ESQUEMA DEL MÓDULO") {
		t.Error("el user imita el esqueleto de iaPromptInforme (solicitud + esquema)")
	}
	if mensajes[2]["role"] != "assistant" || mensajes[2]["content"] != `{"title":"x","sql":"SELECT 1","columns":[]}` {
		t.Error("el assistant entrega el JSON EXACTO, sin prosa alrededor")
	}
}

// ---------------------------------------------------------------------------
// Corrida completa contra una base empresa de prueba
// ---------------------------------------------------------------------------

// baseEmpresaPrueba monta el .sqlite3 con report_template y una tabla invoice,
// con DOS plantillas: una entrenable (ventas sobre invoice) y una fiscal
// (acc_transaction) que DEBE salir excluida.
func baseEmpresaPrueba(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "empresa.sqlite3")
	db, err := sql.Open("sqlite", ruta)
	if err != nil {
		t.Fatalf("no montó la base de prueba: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE report_template (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT, description TEXT, definition TEXT,
		created_by TEXT, status INTEGER, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE invoice (
		invoice_id INTEGER, customer_name TEXT, total_amount REAL, invoice_date TEXT, status INTEGER)`); err != nil {
		t.Fatal(err)
	}

	defBuena := `{"title":"Ventas por cliente","sql":"SELECT i.customer_name AS cliente, SUM(i.total_amount) AS total FROM invoice i WHERE i.invoice_date BETWEEN :desde AND :hasta GROUP BY i.customer_name","columns":[{"key":"cliente","label":"Cliente"},{"key":"total","label":"Total"}],"totals":["total"],"chart":{"type":"bar","x_key":"cliente","y_key":"total","title":"Ventas por cliente"}}`
	defFiscal := `{"title":"Asientos","sql":"SELECT t.account, SUM(t.amount) AS monto FROM acc_transaction t GROUP BY t.account","columns":[{"key":"account","label":"Cuenta"},{"key":"monto","label":"Monto"}],"totals":["monto"]}`
	if _, err := db.Exec(`INSERT INTO report_template (name, description, definition, created_by, status, created_at)
		VALUES ('Ventas por cliente', 'Ventas del periodo por cliente con total.', ?, 'ia:x', 1, '2026-11-01'),
		       ('Asientos contables', 'Asientos del libro diario.', ?, 'ia:y', 1, '2026-11-02')`, defBuena, defFiscal); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// repoFalso monta un repo pequeño con SOLO los dos archivos fuente que el
// generador lee, para que la prueba no dependa del repo completo.
func repoFalso(t *testing.T) string {
	t.Helper()
	raiz := t.TempDir()
	dirWeb := filepath.Join(raiz, "interfaces", "web")
	if err := os.MkdirAll(dirWeb, 0o755); err != nil {
		t.Fatal(err)
	}
	reports := `package web
const aiContratoInforme = ` + "`" + `CONTRATO DE RESPUESTA — REGLAS DEL SISTEMA (no negociables):
1. Responde ÚNICAMENTE con UN objeto JSON.
5. Para fechas usa :desde y :hasta.` + "`" + `
func validateReportSQLwithSchema(sql string) error {
	tablasProhibidas := []string{
		"acc_transaction",
		"acc_vaucher",
		"acc_coa",
		"acc_subcode",
		"acc_subtype",
		"fiscal_setting",
		"fiscal_series",
		"tax_config",
		"retention",
		"pos_fiscal_blocks",
	}
	return nil
}
`
	modulos := `package web
var aiSchemaModules = []aiSchemaModule{
	{
		Key:         "ventas",
		Label:       "Ventas y Facturación",
		Description: "Facturas de venta. Para informes de ventas.",
		Tables:      []string{"invoice"},
	},
	{
		Key:         "contabilidad",
		Label:       "Contabilidad General",
		Description: "Asientos y vouchers.",
		Tables:      []string{"acc_transaction", "acc_vaucher"},
	},
}
`
	if err := os.WriteFile(filepath.Join(dirWeb, "ai_reports.go"), []byte(reports), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirWeb, "ai_schema_modules.go"), []byte(modulos), 0o644); err != nil {
		t.Fatal(err)
	}
	return raiz
}

// Test diagnóstico permanente: el catálogo se parsea DOS módulos con sus
// tablas (sin comillas fantasma — el fallo histórico de FindAllString) y la
// base de prueba expones sus tablas.
func TestCatalogoYTablasDeLaBase(t *testing.T) {
	modulos, err := leerCatalogoModulos(repoFalso(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(modulos) != 2 {
		t.Errorf("se esperaban 2 módulos del fixture, hay %d", len(modulos))
	}
	porKey := map[string]modulo{}
	for _, m := range modulos {
		porKey[m.Key] = m
	}
	ventas, ok := porKey["ventas"]
	if !ok {
		t.Fatal("el módulo ventas no está en el catálogo")
	}
	if len(ventas.Tables) != 1 || ventas.Tables[0] != "invoice" {
		t.Errorf("la tabla de ventas llegó sucia: %q", ventas.Tables)
	}
	contabilidad, ok := porKey["contabilidad"]
	if !ok || len(contabilidad.Tables) != 2 || contabilidad.Tables[0] != "acc_transaction" {
		t.Errorf("las tablas de contabilidad llegaron mal: %v", contabilidad.Tables)
	}
	db, err := sql.Open("sqlite", baseEmpresaPrueba(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hay, err := listarTablasBD(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if !hay["invoice"] || !hay["report_template"] {
		t.Errorf("la base de prueba debe exponer invoice y report_template: %v", hay)
	}
}

// Test 8: la corrida completa. Lo aceptado entra a finetune.jsonl lo que el
// guardián rechaza va EXCLUIDO a plantillas.jsonl y el resumen lo nombra.
func TestCorridaCompleta(t *testing.T) {
	dirBD := filepath.Dir(baseEmpresaPrueba(t))
	rutaBD := filepath.Join(dirBD, "empresa.sqlite3")
	cfg := configCorrida{
		rutaBD:           rutaBD,
		repoRoot:         repoFalso(t),
		directorioSalida: filepath.Join(t.TempDir(), "salida"),
		variantes:        2,
		semilla:          7,
	}
	resumen, err := ejecutar(cfg)
	if err != nil {
		t.Fatalf("ejecutar: %v", err)
	}
	// El resumen honesto: 2 plantillas totales, 1 aceptada, 1 excluida y 2
	// filas de entrenamiento (2 variantes de la única aceptada).
	if !strings.Contains(resumen, "Plantillas totales: 2") || !strings.Contains(resumen, "aceptadas: 1") ||
		!strings.Contains(resumen, "excluidas por el guardián: 1") || !strings.Contains(resumen, "filas de entrenamiento: 2") {
		t.Fatalf("resumen cumple todas las cuotas esperadas:\n%s", resumen)
	}

	// plantillas.jsonl: la fiscal sale con su motivo a la vista.
	contenido, err := os.ReadFile(filepath.Join(cfg.directorioSalida, "plantillas.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contenido), "acc_transaction") {
		t.Error("plantillas.jsonl debe nombrar la exclusión por tabla fiscal para decisión humana")
	}

	// finetune.jsonl: TODAS las filas con 3 mensajes y definición parseable.
	entreno, err := os.ReadFile(filepath.Join(cfg.directorioSalida, "finetune.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	filas := 0
	for _, linea := range strings.Split(strings.TrimSpace(string(entreno)), "\n") {
		if linea == "" {
			continue
		}
		var fila map[string]interface{}
		if err := json.Unmarshal([]byte(linea), &fila); err != nil {
			t.Fatalf("finetune.jsonl línea %d no es JSON: %v", filas+1, err)
		}
		msgs, ok := fila["messages"].([]interface{})
		if !ok || len(msgs) != 3 {
			t.Fatalf("la fila %d no trae 3 mensajes", filas+1)
		}
		filas++
	}
	if filas != 2 {
		t.Errorf("se esperaban 2 filas de entrenamiento, hay %d", filas)
	}

	// esquema.jsonl: el módulo ventas tiene su tabla invoice introspectada.
	esq, err := os.ReadFile(filepath.Join(cfg.directorioSalida, "esquema.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(esq), `"invoice"`) || !strings.Contains(string(esq), "customer_name") {
		t.Errorf("esquema.jsonl debe traer invoice con sus campos reales; lo que salió:\n%s", string(esq))
	}
}
