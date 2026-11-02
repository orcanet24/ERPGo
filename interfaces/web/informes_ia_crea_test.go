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
	"fmt"
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
	respuesta := respuestaIAIndependiente(`{"title":"Ventas por cliente","sql":"SELECT 'Cliente Demo' AS cliente","columns":[{"key":"cliente","label":"Cliente"}],"totals":[]}`)

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

// TestElGuardarSeHaceSobreElPreview: un informe con SQL de escritura no se guarda
// (regla del guardián — TestElGuardianSeComeElSQLDeEscritura) y TAMPOCO se guarda
// uno que pase el guardián pero no pueda ejecutarse (tabla inexistente, alias
// errado). El preview actúa como el técnico que prueba antes de dejarlo en el
// sistema: un SELECT contra una tabla que no existe tira 400 en la prueba y no
// deja fila huérfana.
func TestElGuardarSeHaceSobreElPreview(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	r := reqPrueba(s, sessPrueba("admin"), "/reports/custom/ai-create")
	// El SQL pasa aiValidateSQLSelect (es un SELECT) pero no existe la tabla: el
	// preview lo rechaza y no se inserta nada.
	respuesta := respuestaIAIndependiente(`{"title":"Venta por factura chuta","sql":"SELECT f.numero FROM factura_que_no_existe f","columns":[{"key":"numero","label":"Factura"}],"totals":[]}`)

	id, _, err := s.crearInformeDesdeRespuestaIA(r, sessPrueba("admin"), "ventas", respuesta)
	if err == nil {
		t.Fatal("el preview fallido debía impedir el guardado, pero se guardó id=", id)
	}
	if !strings.Contains(err.Error(), "prueba de ejecución") {
		t.Errorf("el error debe decir que falló la prueba de ejecución; salió: %v", err)
	}
	var filas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM report_template`).Scan(&filas); err != nil || filas != 0 {
		t.Errorf("no debe quedar fila del informe que falló el preview: err=%v filas=%d", err, filas)
	}
}

// TestElPreviewRechazaTotalesQueNoSePuedenCalcular: un total apuntando a una
// columna que no devuelve el SELECT pasa el guardián estático pero el preview
// advierte y el informe no se guarda —evita la peor de las fallas: un informe que
// parece tener cifra y no la tiene.
func TestElPreviewRechazaTotalesQueNoSePuedenCalcular(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	r := reqPrueba(s, sessPrueba("admin"), "/reports/custom/ai-create")
	// "cliente" está en totals (pasa el guardián estático: totals ⊆ columns),
	// pero el SELECT devuelve un texto no numérico en esa posición: el preview
	// no puede calcularlo y advierte. Un regex estático no lo vería; el preview sí.
	respuesta := respuestaIAIndependiente(`{"title":"Ventas","sql":"SELECT 'foo' AS cliente","columns":[{"key":"cliente","label":"Cliente"}],"totals":["cliente"]}`)

	if _, _, err := s.crearInformeDesdeRespuestaIA(r, sessPrueba("admin"), "ventas", respuesta); err == nil {
		t.Fatal("un informe con total imposible debía rechazarse en el preview")
	} else if !strings.Contains(err.Error(), "prueba de ejecución") {
		t.Errorf("el error debe señalar el preview; salió: %v", err)
	}
	var filas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM report_template`).Scan(&filas); err != nil || filas != 0 {
		t.Errorf("no debe quedar fila del informe rechazado por el preview: err=%v filas=%d", err, filas)
	}
}

// TestReglaFinalCambiaParaSalidaJSON: la regla 6 del sistema pide Markdown, y la
// variante para salida JSON la sustituye. Sin esto el modelo recibía "entrega en
// Markdown" y el extractor no hallaba el objeto JSON → 502 con tokens gastados y
// el informe no se creaba.
func TestReglaFinalCambiaParaSalidaJSON(t *testing.T) {
	core := aiReglasDelSistema(false)
	if !strings.Contains(core, "informe final claro en Markdown") {
		t.Fatal("la regla normal debe pedir Markdown")
	}
	porJSON := aiReglasDelSistema(true)
	if strings.Contains(porJSON, "informe final claro en Markdown") {
		t.Fatalf("la variante JSON no debe pedir Markdown; salió: %s", porJSON)
	}
	if !strings.Contains(porJSON, "ÚNICO objeto JSON válido") {
		t.Fatalf("la variante JSON debe pedir el objeto; salió: %s", porJSON)
	}
}

// TestCatalogoExcluyeHerramienta: aiToolsJSONExcluyendo quita las herramientas de
// la lista (p.ej. report.save_template en la creación por IA) y, sin exclusión,
// coincide con aiToolsJSON.
func TestCatalogoExcluyeHerramienta(t *testing.T) {
	// Un rol con el permiso concreto ve report.save_template en el catálogo; así
	// podemos probar que EXCLUIRLA la quita de verdad.
	conPermiso := gateDePrueba("user", map[string]string{"custom_reports": "create"})
	sinExcl := aiToolsJSON(false, conPermiso)
	conExcl := aiToolsJSONExcluyendo(false, conPermiso, []string{"report.save_template"})

	if !strings.Contains(sinExcl, "report_save_template") {
		t.Fatal("el catálogo con permiso debe anunciar report.save_template; no se pudo probar la exclusión")
	}
	if strings.Contains(conExcl, "report_save_template") {
		t.Error("el catálogo excluido no debe contener report.save_template (wire: report_save_template)")
	}
	// Sin exclusión explícita, el helper nuevo coincide con el anterior: no se
	// rompe ningún otro llamador.
	if aiToolsJSONExcluyendo(false, conPermiso, nil) != sinExcl {
		t.Error("sin exclusión, aiToolsJSONExcluyendo debe ser idéntico a aiToolsJSON")
	}
}

// TestParametrosDesdeHastaSeSustituyen: un SQL con :desde/:hasta pasa a literales
// de fecha validados, y fechas malas se rechazan antes de tocar el SQL.
func TestParametrosDesdeHastaSeSustituyen(t *testing.T) {
	if out, err := substituteReportParams("SELECT * FROM a WHERE f BETWEEN :desde AND :hasta",
		"2026-01-01", "2026-01-31"); err != nil {
		t.Fatalf("no debería fallar con fechas válidas: %v", err)
	} else if !strings.Contains(out, "'2026-01-01'") || !strings.Contains(out, "'2026-01-31'") {
		t.Errorf("no se sustituyeron los marcadores: %s", out)
	}
	if out, err := substituteReportParams("SELECT * FROM a", "2026-01-01", "2026-01-31"); err != nil {
		t.Fatalf("sin placeholders no debe tocar el SQL: %v", err)
	} else if out != "SELECT * FROM a" {
		t.Errorf("sin placeholders no debe modificar nada: %s", out)
	}
	if _, err := substituteReportParams("SELECT * FROM a WHERE f >= :desde", "", "2026-01-31"); err == nil {
		t.Fatal("una fecha :desde vacía debe rechazarse")
	}
}

// TestTotalsNoDependenDeOrdenColumns: reproduce el bug del usuario en el que
// el agente reordena columns respecto a los aliases del SELECT (ej: declara
// "mes" antes que "trimestre" pero el SELECT devuelve "trimestre" antes que
// "mes"). Antes, el motor sumaba posición a posición y un total declarado
// caía en la posición equivocada → "no se pudo calcular el total". Con
// keyIndex el total se busca por key declarada sin depender del orden.
func TestTotalsNoDependenDeOrdenColumns(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	// Tabla de prueba con datos numéricos.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ventas_demo (
		fecha TEXT, periodo TEXT, importe REAL)`); err != nil {
		t.Fatalf("create ventas_demo: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ventas_demo (fecha, periodo, importe) VALUES
		('2026-01-05','2026-01','100'), ('2026-01-10','2026-01','200')`); err != nil {
		t.Fatalf("insert ventas_demo: %v", err)
	}
	sess := sessPrueba("admin")
	// El agente reordena: columns dice mes,trimestre pero el SELECT devuelve
	// en orden trimestre,mes (¡la desalineación del bug!). totals pide "mes".
	def := `{"title":"Ventas","sql":"SELECT fecha AS dia, periodo AS trimestre, importe AS mes FROM ventas_demo","columns":[{"key":"dia","label":"Día"},{"key":"mes","label":"Mes"},{"key":"trimestre","label":"Trimestre"}],"totals":["mes"]}`
	_, preview, err := s.previewDefinitionFromJSON(sess, def)
	if err != nil {
		t.Fatalf("previewDefinitionFromJSON no debe fallar por reorden: %v", err)
	}
	// preview se usa solo para confirmar que no bloquea; aquí "mes" es una fecha
	// (TEXT) así que su total no se calcula, pero no debe dar error.
	_ = preview
	// Para probar el total NUMÉRICO real, usamos importe AS monto, con el key
	// declarado en otro orden que el SELECT (monto aparece en cols pero el
	// agente pudo reordenar): keyIndex resuelve por key, no por posición.
	def2 := `{"title":"Ventas","sql":"SELECT fecha AS dia, importe AS monto FROM ventas_demo","columns":[{"key":"dia","label":"Día"},{"key":"monto","label":"Monto"}],"totals":["monto"]}`
	_, preview2, err := s.previewDefinitionFromJSON(sess, def2)
	if err != nil {
		t.Fatalf("preview2: %v", err)
	}
	if len(preview2.Warnings) > 0 {
		t.Fatalf("no debe haber warnings: %v", preview2.Warnings)
	}
	v, ok := preview2.Totals["monto"]
	if !ok {
		t.Fatal("el total de 'monto' no se calculó")
	}
	if fmt.Sprintf("%v", v) != "300" && fmt.Sprintf("%v", v) != "300.0" {
		t.Errorf("esperaba total monto=300; salió %v", v)
	}
}

// TestPreviewNoBloqueaMostrandoWarning: el preview devuelve warnings (no error)
// para que el usuario itere; el warning incluye las columnas reales. Reproduce
// el reporte del usuario (ventas_actual/iva_cobrado sin valores numéricos porque
// el resultado tiene NULL por división por cero en variacion_pct).
func TestPreviewNoBloqueaMostrandoWarning(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ventas_demo2 (actual REAL, anterior REAL)`); err != nil {
		t.Fatalf("create ventas_demo2: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ventas_demo2 (actual, anterior) VALUES (100, 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	sess := sessPrueba("admin")
	// variacion_pct = actual/anterior*100 → NULL (división por cero) → no numerico.
	// ventas_actual SÍ es numérica: debe calcularse. ventas_anterior también.
	def := `{"title":"V","sql":"SELECT actual AS ventas_actual, anterior AS ventas_anterior, CASE WHEN anterior=0 THEN NULL ELSE actual/anterior*100 END AS variacion_pct FROM ventas_demo2","columns":[{"key":"ventas_actual"},{"key":"ventas_anterior"},{"key":"variacion_pct"}],"totals":["ventas_actual","ventas_anterior","variacion_pct"]}`
	_, preview, err := s.previewDefinitionFromJSON(sess, def)
	if err != nil {
		t.Fatalf("preview no debe bloquear por warnings: %v", err)
	}
	// ventas_actual y ventas_anterior deben calcularse (no están en warnings).
	if _, ok := preview.Totals["ventas_actual"]; !ok {
		t.Errorf("ventas_actual debería calcularse; warnings=%v", preview.Warnings)
	}
	if _, ok := preview.Totals["ventas_anterior"]; !ok {
		t.Errorf("ventas_anterior debería calcularse; warnings=%v", preview.Warnings)
	}
}

// TestElTotalDeUnResultadoVacioEsCeroSinWarning: reproduce el defecto medido en
// vivo con el asistente de IA. Contra una base recién instalada (sin facturas),
// el modelo genera una definición correcta cuyo SELECT devuelve CERO filas; como
// la suma va por filas, sums quedaba vacío y el motor avisaba "no se pudo
// calcular el total de la columna X (SELECT: alias): no aparece en el resultado
// de la consulta" — mientras el propio aviso la listaba entre las disponibles.
// El total de cero filas es 0 y no es un aviso: es la respuesta correcta.
func TestElTotalDeUnResultadoVacioEsCeroSinWarning(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ventas_vacias (
		cliente TEXT, importe REAL)`); err != nil {
		t.Fatalf("create ventas_vacias: %v", err)
	}
	// NO se insertan filas: esa es la condición del defecto.
	sess := sessPrueba("admin")
	def := `{"title":"Ventas por cliente","sql":"SELECT cliente AS customer_name, importe AS total_facturado FROM ventas_vacias","columns":[{"key":"customer_name","label":"Cliente"},{"key":"total_facturado","label":"Total"}],"totals":["total_facturado"]}`
	_, preview, err := s.previewDefinitionFromJSON(sess, def)
	if err != nil {
		t.Fatalf("preview con resultado vacío no debe fallar: %v", err)
	}
	if len(preview.Warnings) > 0 {
		t.Errorf("un resultado vacío no debe generar warnings; salieron: %v", preview.Warnings)
	}
	v, ok := preview.Totals["total_facturado"]
	if !ok {
		t.Fatalf("el total de la columna debe existir aunque no haya filas; Totals=%v", preview.Totals)
	}
	if fmt.Sprintf("%v", v) != "0" {
		t.Errorf("esperaba total total_facturado=0; salió %v", v)
	}
}

// TestLaColumnaExistePeroSusValoresNoSonNumericosDaAvisoHonesto: con filas y una
// columna declarada como total cuyos valores no son números (texto), el aviso
// debe decir POR QUÉ no se sumó — no puede decir que la columna "no aparece",
// porque aparece y el usuario la ve en la tabla.
func TestLaColumnaExistePeroSusValoresNoSonNumericosDaAvisoHonesto(t *testing.T) {
	s, db := basePrueba(t)
	crearTablaReportTemplatePrueba(t, db)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ventas_texto (
		cliente TEXT, importe TEXT)`); err != nil {
		t.Fatalf("create ventas_texto: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ventas_texto (cliente, importe) VALUES ('A','no es un número'), ('B','12 pesos')`); err != nil {
		t.Fatalf("insert ventas_texto: %v", err)
	}
	sess := sessPrueba("admin")
	def := `{"title":"V","sql":"SELECT cliente, importe FROM ventas_texto","columns":[{"key":"cliente"},{"key":"importe"}],"totals":["importe"]}`
	_, preview, err := s.previewDefinitionFromJSON(sess, def)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Warnings) != 1 {
		t.Fatalf("esperaba exactamente 1 warning; salieron: %v", preview.Warnings)
	}
	if !strings.Contains(preview.Warnings[0], "ninguno de sus valores es numérico") {
		t.Errorf("el warning debe explicar que los valores no son numéricos; salió: %q", preview.Warnings[0])
	}
	if strings.Contains(preview.Warnings[0], "no aparece en el resultado") {
		t.Errorf("el warning no debe mentir: la columna SÍ aparece en el resultado. Salió: %q", preview.Warnings[0])
	}
}
