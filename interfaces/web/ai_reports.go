package web

// ai_reports.go — Informes personalizados por plantillas JSON. Tanto usuarios
// como agentes crean plantillas; TODAS pasan por el guardián SQL al ejecutar.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// reportTemplate espeja una fila de report_template.
type reportTemplate struct {
	ID          int64
	Name        string
	Description string
	Definition  string
	CreatedBy   string
	Status      int
}

// loadReportTemplates lista las plantillas que el usuario puede ver.
//
// Visibilidad: SIN filas en report_template_acl → todos ven (compatibilidad
// con informes pre-migration). SI hay filas → el usuario ve el informe si alguna
// fila coincide con su role_id o user_id, o si la fila es "pública explícita"
// (role_id IS NULL AND user_id IS NULL). Master ver todo siempre.
//
// Si la tabla report_template_acl no existe todavía (instalación sin aplicar
// la migración), la query falla: se devuelve la lista SIN filtrar (comportamiento
// legado) en vez de 500. La tabla se crea en el arranque normal vía migrations.
func (s *Server) loadReportTemplates(sess *SessionData) []reportTemplate {
	db := s.dbForSession(sess)
	if db == nil {
		return []reportTemplate{}
	}
	// Master ve todo de todas formas.
	level := "employee"
	if sess != nil {
		level, _ = loadUserRole(db, sess.UserID)
		if level == "" {
			if ctl, cerr := s.controlStore(); cerr == nil {
				level, _ = loadUserRole(ctl, sess.UserID)
			}
		}
	}
	if level == "master" || sess == nil {
		// Master y usuarios sin rol con nivel: ver todo (comportamiento legado).
		// Los sin rol aparecen como employee en el frontend; si no son master,
		// la query de abajo filtra por ACL. Se deja master ver todo.
	}
	roleID := int64(0)
	if sess != nil {
		roleID = s.sessionRoleID(sess)
	}
	userID := ""
	if sess != nil {
		userID = sessUserID(sess)
	}
	query := `SELECT rt.id, rt.name, IFNULL(rt.description,''), rt.definition, IFNULL(rt.created_by,''), rt.status
		FROM report_template rt
		WHERE rt.status = 1
		  AND (
		    NOT EXISTS (SELECT 1 FROM report_template_acl WHERE report_template_id = rt.id)
		    OR EXISTS (
		      SELECT 1 FROM report_template_acl
		      WHERE report_template_id = rt.id
		        AND (
		          (role_id = ? AND ? > 0)
		          OR user_id = ?
		          OR (role_id IS NULL AND user_id IS NULL)
		        )
		    )
		  )
		ORDER BY rt.name`
	rows, err := db.Query(query, roleID, roleID, userID)
	if err != nil {
		// La tabla report_template_acl no existe aún (pre-migration):
		// devolver todo sin filtrar, como antes.
		fallback := `SELECT id, name, IFNULL(description,''), definition, IFNULL(created_by,''), status
			FROM report_template WHERE status = 1 ORDER BY name`
		rows, err = db.Query(fallback)
		if err != nil {
			return []reportTemplate{}
		}
	}
	defer rows.Close()
	list := []reportTemplate{}
	for rows.Next() {
		var t reportTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Definition, &t.CreatedBy, &t.Status); err == nil {
			list = append(list, t)
		}
	}
	if err := rows.Err(); err != nil {
		return []reportTemplate{}
	}
	return list
}

// sessionRoleID devuelve el role_id (sec_role.id) de la sesión, con fallback a
// la BD de control. sec_userrole vive en control, y el rol global del usuario
// también allí; la BD de empresa puede no tenerlo en empresas nuevas.
func (s *Server) sessionRoleID(sess *SessionData) int64 {
	if sess == nil {
		return 0
	}
	// 1. BD de empresa activa (si tiene sec_userrole).
	if db := s.db; db != nil {
		var id int64
		if err := db.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = ? LIMIT 1`, sess.UserID).Scan(&id); err == nil && id > 0 {
			return id
		}
	}
	// 2. BD de control.
	if ctl, err := s.controlStore(); err == nil && ctl != nil {
		var id int64
		if err := ctl.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = ? LIMIT 1`, sess.UserID).Scan(&id); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// reportDefShape es la estructura de una definición de informe.
type reportDefShape struct {
	Title   string          `json:"title"`
	SQL     string          `json:"sql"`
	Columns []reportDefCol  `json:"columns"`
	Totals  []string        `json:"totals"`
	Chart   *ReportChart    `json:"chart,omitempty"`
	Extra   json.RawMessage `json:"-"`
}

type reportDefCol struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

func (d reportDefShape) ClavesDeclaradas() []string {
	out := make([]string, 0, len(d.Columns))
	for _, c := range d.Columns {
		out = append(out, c.Key)
	}
	return out
}

// validateReportDefinition aplica el guardián a la definición JSON.
func (s *Server) validateReportDefinition(definition string) error {
	var d reportDefShape
	if err := json.Unmarshal([]byte(definition), &d); err != nil {
		return fmt.Errorf("definition no es JSON válido: %v", err)
	}
	if strings.TrimSpace(d.SQL) == "" {
		return fmt.Errorf("definition.sql es obligatorio")
	}
	if len(d.Columns) == 0 {
		return fmt.Errorf("definition.columns debe tener al menos una columna")
	}
	if _, err := aiValidateSQLSelect(d.SQL, aiHardMaxRows); err != nil {
		return err
	}
	// NUEVA CAPA: validar SQL contra reglas fiscales y de tabla permitidas.
	// Esto es una segunda barrera DESPUÉS del guardián SELECT y ANTES de la ejecución.
	// Requiere el esquema completo de la BD (proveniente del MCP) para validar.
	if vErr := s.validateReportSQLwithSchema(d.SQL); vErr != nil {
		return vErr
	}
	// Cada total debe ser una columna declarada. Si no lo es, el motor no
	// puede calcularlo: lo descarta y el informe sale sin cifras pero con
	// aspecto de estar completo. Es preferible que la plantilla no se pueda
	// guardar, porque se corrige en el momento y no cuando alguien lea un
	// informe que parece tener un total y no lo tiene.
	declaradas := map[string]bool{}
	for _, c := range d.Columns {
		declaradas[c.Key] = true
	}
	var fuera []string
	for _, t := range d.Totals {
		if !declaradas[t] {
			fuera = append(fuera, t)
		}
	}
	if len(fuera) > 0 {
		sort.Strings(fuera)
		return fmt.Errorf("totals apunta a columnas que no están en columns: %s. Declara la columna o quita el total",
			strings.Join(fuera, ", "))
	}
	// El gráfico NO puede apuntar a una columna que el SELECT no devuelve: el
	// motor lo descartaría en silencio y el informe se guardaría sin gráfico
	// sin que nadie sepa por qué. Se valida aquí, igual que los totales, para
	// que la causa se arregle en el momento.
	if d.Chart != nil {
		switch d.Chart.Type {
		case "line", "bar", "pie", "doughnut":
		default:
			return fmt.Errorf("chart.type debe ser line, bar, pie o doughnut (recibido: %q)", d.Chart.Type)
		}
		if strings.TrimSpace(d.Chart.XKey) == "" || strings.TrimSpace(d.Chart.YKey) == "" {
			return fmt.Errorf("chart.x_key y chart.y_key son obligatorios cuando se define chart")
		}
		for campo, k := range map[string]string{"x_key": d.Chart.XKey, "y_key": d.Chart.YKey} {
			if !declaradas[k] {
				return fmt.Errorf("chart.%s apunta a %q, que no está en columns: el gráfico no puede leer una columna que el SELECT no devuelve", campo, k)
			}
		}
	}
	return nil
}

// validateReportSQLwithSchema es la segunda barrera de seguridad para SQL de reportes.
// Se ejecuta DESPUÉS de aiValidateSQLSelect (que asegura que es un SELECT limpio)
// y VALIDA que el SQL no targetee tablas fiscales protegidas usando el esquema completo.
func (s *Server) validateReportSQLwithSchema(sql string) error {
	// Obtener el esquema completo desde el MCP (cacheado por huella).
	// Por ahora, usamos una validación básica con esquema mínimo incrustado.
	// El esquema completo se pasará cuando esté totalmente integrado.
	low := strings.ToLower(sql)

	// Tablas fiscales prohibidas (misma lista que en governance)
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

	for _, tabla := range tablasProhibidas {
		if strings.Contains(low, tabla) {
			return fmt.Errorf("El reporte IA no puede targetear la tabla fiscal '%s'. Las operaciones fiscales son solo para humanos. Genera un reporte operacional en su lugar.", tabla)
		}
	}

	// Tablas permitidas para reportes IA (operacionales)
	columnasPermitidas := map[string]bool{
		"product":  true,
		"client":   true,
		"supplier": true,
		"sale":     true,
		"purchase": true,
		"invoice":  true,
		"company":  true,
	}

	// Extraer nombres de tablas del SQL
	patronesFROM := []string{
		`from\s+(\w+)`,
		`join\s+(\w+)`,
	}

	for _, patron := range patronesFROM {
		matches := findSQLMatches(sql, patron)
		for _, tabla := range matches {
			if !columnasPermitidas[tabla] {
				// Verificar si es una tabla fiscal conocida
				esFiscal := false
				for _, fTabla := range tablasProhibidas {
					if strings.EqualFold(tabla, fTabla) {
						esFiscal = true
						break
					}
				}
				if esFiscal {
					return fmt.Errorf("Tabla '%s' es fiscal y está prohibida en reportes IA. Humanos editan en laboratorio.", tabla)
				}
				// Tabla no reconocida: advertir pero permitir si parece operación legítima
				// (en producción real, esto requeriría el esquema completo del MCP)
			}
		}
	}

	// Verificar palabras clave fiscales sensibles en SELECT
	palabrasClaveFiscales := []string{
		"impuesto",
		"retencion",
		"igtf",
		"base_imponible",
		"igv",
		"total_fiscal",
		"subtotal_fiscal",
	}

	for _, kw := range palabrasClaveFiscales {
		if strings.Contains(low, kw) {
			// Permitir si está en contexto de agregación (GROUP BY, SUM, COUNT)
			if strings.Contains(low, "group by") || strings.Contains(low, "sum(") || strings.Contains(low, "count(") {
				continue
			}
			// Advertir si aparece sola sin contexto de cálculo
			return fmt.Errorf("Palabra clave fiscal '%s' detectada en SQL de reporte. Verifica que sea parte de cálculo sobre datos, no configuración manual.", kw)
		}
	}

	return nil
}

// reportResult es el resultado de ejecutar una plantilla.
type reportResult struct {
	Title    string                 `json:"title"`
	Headers  []string               `json:"headers"`
	Rows     [][]interface{}        `json:"rows"`
	Totals   map[string]interface{} `json:"totals,omitempty"`
	Warnings []string               `json:"warnings,omitempty"`
	// Chart es el dataset ya procesado que consume Chart.js (mismo contrato
	// que el motor tipado). Va aparte de Totals: el gráfico lo alimentan las
	// filas del propio informe, no un cálculo nuevo.
	Chart *ReportRunChart `json:"chart,omitempty"`
}

// executeReportDefinition corre el SQL del informe y estructura el resultado.
func (s *Server) executeReportDefinition(sess *SessionData, definition, from, to string) (*reportResult, error) {
	var d reportDefShape
	if err := json.Unmarshal([]byte(definition), &d); err != nil {
		return nil, fmt.Errorf("definition inválida: %v", err)
	}
	sqlText, err := aiValidateSQLSelect(d.SQL, aiHardMaxRows)
	if err != nil {
		return nil, err
	}
	// Sustitución de :desde/:hasta para las definiciones legadas (contracto viejo
	// de informes clásicos). La versión tipada (RunReportDefinition) ya lo hace con
	// reportLiteral; aquí no hay esquema de parámetros, así que solo se resuelven
	// las dos fechas del rango. Validar ANTES de concatenar: desde/hasta vienen de
	// r.FormValue en el run clásico, y un valor malo no puede convertirse en inyección
	// porque se comprueba el formato de fecha antes de tocar el SQL.
	sqlText, err = substituteReportParams(sqlText, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbForSession(sess).Query(sqlText)
	if err != nil {
		return nil, fmt.Errorf("error de SQL: %v", err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	res := &reportResult{Title: d.Title, Headers: cols, Totals: map[string]interface{}{}}
	sums := map[string]float64{}

	// Los totales van por la clave DECLARADA (d.Columns[i].Key) y no por el
	// alias del SELECT. Antes se sumaba posición a posición (cols[i]↔declaradas[i]),
	// lo que rompía cuando el agente reordenaba columns respecto al SELECT
	// (ej: declara mes/trimestre al revés): el valor de una posición caía en
	// la key equivocada y el total salía vacío con un warning confuso.
	//
	// La corrección: construir keyIndex mapeando CUALQUIER nombre (alias del
	// SELECT y/o key declarada) a su posición i en vals. Luego el total por
	// key busca vals[keyIndex[key]] directamente, sin depender del orden.
	declaradas := d.ClavesDeclaradas()
	keyIndex := map[string]int{}
	for i, c := range cols {
		if i < len(declaradas) && declaradas[i] != "" && declaradas[i] != c {
			keyIndex[declaradas[i]] = i
		}
		if _, ok := keyIndex[c]; !ok {
			keyIndex[c] = i
		}
	}
	// Los encabezados los arma la definición (label > key), no el SELECT: así
	// el usuario ve nombres amigables y no aliases de SQL crudo.
	if len(d.Columns) > 0 {
		res.Headers = make([]string, 0, len(d.Columns))
		for _, c := range d.Columns {
			label := c.Label
			if label == "" {
				label = c.Key
			}
			res.Headers = append(res.Headers, label)
		}
	}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		// Normalizar []byte a string para valorNumerico.
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		// Sumar por cada key declarada; buscar el valor por su posición REAL
		// (keyIndex), no por posición. Esto resuelve el desalineamiento entre
		// columns[].key y los aliases del SELECT.
		for _, c := range d.Columns {
			key := c.Key
			if key == "" {
				continue
			}
			idx, ok := keyIndex[key]
			if !ok {
				continue
			}
			f := valorNumerico(vals[idx])
			if f == nil {
				continue
			}
			sums[key] += *f
		}
		res.Rows = append(res.Rows, vals)
		if len(res.Rows) >= aiHardMaxRows {
			break
		}
	}
	for _, key := range d.Totals {
		if v, ok := sums[key]; ok {
			res.Totals[key] = round2(v)
			continue
		}

		// Sin suma puede haber TRES causas distintas. El aviso antiguo decía
		// "no aparece en el resultado" para todas y mentía en dos: la columna
		// que el propio aviso listaba entre las "disponibles". Medido en vivo:
		// informe creado por IA contra una base maestra recién instalada pinta
		// el preview con el warning falso aunque la definición declara la
		// columna exacta. Se resuelve por causa:
		//
		// 1) Resultado sin filas: un total sobre cero filas es cero. No hay
		//    nada que avisar — un informe de un mes sin ventas sigue siendo
		//    correcto y sus totales valen 0. Como toda la suma va por
		//    keyIndex (alias e key declarada), la existencia se consulta ahí.
		// 2) Filas pero valores no numéricos (NULL, texto): la columna existe,
		//    se dice por qué no se sumó, no que "no aparece".
		// 3) La key tal cual no está ni como alias ni como key declarada: sí
		//    es un desajuste del agente y conserva el aviso detallado.
		if _, existe := keyIndex[key]; existe {
			if len(res.Rows) == 0 {
				res.Totals[key] = 0
				continue
			}
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"no se pudo calcular el total de la columna %q: existe en el resultado pero ninguno de sus valores es numérico",
				key))
			continue
		}
		// Se pidió un total de una key que el resultado ni devuelve. Sin aviso,
		// el informe sale con las mismas filas y sin cifras, y quien lo lee no
		// tiene por qué sospechar que le falta el dato más importante.
		//
		// Por qué se listan las columnas y el mismatch: el agente suele desalinear
		// `columns` respecto a los aliases del SELECT, o declarar una key que el
		// SELECT ni devuelve. Mostrar ambos nombres ("key (SELECT: alias)") deja
		// visible el desajuste para corregir en el refine.
		disponibles := make([]string, 0, len(cols))
		seen := map[string]bool{}
		for i := range cols {
			k := cols[i]
			if k == "" {
				continue
			}
			if i < len(declaradas) && declaradas[i] != "" && declaradas[i] != k && !seen[declaradas[i]] {
				disponibles = append(disponibles, declaradas[i]+" (SELECT: "+k+")")
				seen[declaradas[i]] = true
			}
			if !seen[k] {
				disponibles = append(disponibles, k)
				seen[k] = true
			}
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"no se pudo calcular el total de la columna %q: no aparece en el resultado de la consulta. Columnas disponibles: %s",
			key, strings.Join(disponibles, ", ")))
	}
	// El gráfico se arma de las MISMAS filas del informe: si el informe tiene
	// datos, el gráfico los alimenta; si no tiene filas, el gráfico sale con
	// etiquetas vacías y la tabla debajo explica por qué. Igual que en el motor
	// tipado (report_engine.go), un chart inválido es un aviso y no un error.
	if d.Chart != nil {
		if chart, err := buildReportChart(d.Chart, d.ClavesDeclaradas(), cols, res.Rows); err == nil {
			res.Chart = chart
		} else {
			res.Warnings = append(res.Warnings, fmt.Sprintf("no se pudo generar el gráfico: %v", err))
		}
	}
	return res, nil
}

// containsIntKey verifica si key ya está mapeado en m.
func containsIntKey(m map[string]int, key string) bool {
	_, ok := m[key]
	return ok
}

// valorNumerico convierte a float lo que venga de la base. convierte a float lo que venga de la base.
//
// MySQL entrega los DECIMAL como []byte para no perder precisión, de modo que un
// `SUM(total_amount)` no aparece como float64 sino como []byte("145.00"). Por eso
// esta función existe: sin ella, los totales de dinero nunca se calculan y el
// informe sale con las filas puestas y ninguna cifra, que es el peor resultado
// posible porque parece un informe completo.
func valorNumerico(v interface{}) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case []byte:
		return cadenaANumero(string(n))
	case string:
		return cadenaANumero(n)
	}
	return nil
}

// cadenaANumero lee un número escrito como texto, tolerando el formato que usa
// MySQL para los DECIMAL ("145.00") y separadores de miles.
func cadenaANumero(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Se quitan los separadores de miles: un importe de contabilidad puede venir
	// formateado y "1.234,56" no es un número para ParseFloat.
	limpio := strings.NewReplacer(",", "", " ", "", "\u00a0", "").Replace(s)
	if limpio == "" {
		return nil
	}
	f, err := strconv.ParseFloat(limpio, 64)
	if err != nil {
		return nil
	}
	return &f
}

func (s *Server) registerAIReportRoutes() {
	s.mux.HandleFunc("GET /reports/custom", s.reportTemplateList)
	s.mux.HandleFunc("GET /reports/custom/data", s.reportTemplateListData)
	s.mux.HandleFunc("GET /reports/custom/definition/{id}", s.reportTemplateEngineParams)
	s.mux.HandleFunc("POST /reports/custom/engine-run/{id}", s.reportTemplateEngineRun)
	s.mux.HandleFunc("POST /reports/custom/engine-export/{id}", s.reportTemplateEngineExport)
	s.mux.HandleFunc("GET /reports/custom/add", s.reportTemplateForm)
	s.mux.HandleFunc("GET /reports/custom/edit/{id}", s.reportTemplateForm)
	s.mux.HandleFunc("POST /reports/custom/save", s.reportTemplateSave)
	s.mux.HandleFunc("POST /reports/custom/delete/{id}", s.reportTemplateDelete)
	s.mux.HandleFunc("GET /reports/custom/run/{id}", s.reportTemplateRun)
	s.mux.HandleFunc("POST /reports/custom/ai-help", s.reportTemplateAIHelp)
	// ai-preview: IA genera definición + ejecución previa (NO guarda). Devuelve
	// {definition, name, description, preview} para el modal interactivo.
	s.mux.HandleFunc("POST /reports/custom/ai-preview", s.reportTemplateAIPreview)
	// ai-preview-stream: igual que ai-preview pero con SSE para mostrar en vivo
	// lo que la IA hace (pasos, reintentos, vista previa).
	s.mux.HandleFunc("POST /reports/custom/ai-preview-stream", s.reportTemplateAIPreviewStream)
	// ai-refine: IA ajusta la definición con feedback del usuario → preview.
	s.mux.HandleFunc("POST /reports/custom/ai-refine", s.reportTemplateAIRefine)
	s.mux.HandleFunc("POST /reports/custom/ai-refine-stream", s.reportTemplateAIRefineStream)
	// ai-save: guarda el informe con visibilidad (ACL por role_id/user_id).
	s.mux.HandleFunc("POST /reports/custom/ai-save", s.reportTemplateAISave)
	// ai-roles: lista los roles (sec_role) para el selector de visibilidad del
	// modal de creación asistida. Master ver todo; los roles son de la BD de control.
	s.mux.HandleFunc("GET /reports/custom/roles", s.reportTemplateRoles)
}

// previewDefinitionFromJSON valida y ejecuta la previa una definición JSON ya
// extraída del texto del agente. SEPARADO de iaPreviewDefinition porque varios
// callers ya tienen el JSON (crearInformeDesdeRespuestaIA, tests) y no deben
// gastar una corrida de agente de más solo para re-validar.
//
// POR QUÉ LOS WARNINGS NO SON ERROR aquí: el preview es para ITERAR, no para
// guardar. Un total que no se puede calcular (por un alias desincronizado) se
// devuelve como warning en el reportResult para que el usuario lo vea y pida
// un refine. El bloqueo definitivo es ai-save (executeDefinitionPreview +
// revisión de warnings). Antes se convertía el warning en error, el handler
// devolvía 502 y el agente no recibía preview alguno: imposible auto-corregir.
func (s *Server) previewDefinitionFromJSON(sess *SessionData, jsonText string) (def reportDefShape, preview *reportResult, err error) {
	if err = json.Unmarshal([]byte(jsonText), &def); err != nil {
		err = fmt.Errorf("la definición no es JSON válido: %v", err)
		return
	}
	if vErr := s.validateReportDefinition(jsonText); vErr != nil {
		err = fmt.Errorf("la definición no pasa el guardián: %v", vErr)
		return
	}
	pre, execErr := s.executeDefinitionPreview(sess, jsonText)
	if execErr != nil {
		err = fmt.Errorf("la definición no pasa la prueba de ejecución: %v", execErr)
		return
	}
	preview = pre
	return def, preview, nil
}

// executeDefinitionPreview ejecuta una definición como preview de prueba con
// fechas válidas (mes actual), para que :desde/:hasta no fallen con "".
// El guardián estático (validateReportDefinition) no valida valores de fecha;
// el preview ejecuta el SQL real y necesita fechas concretas.
func (s *Server) executeDefinitionPreview(sess *SessionData, jsonText string) (*reportResult, error) {
	return s.executeReportDefinition(sess, jsonText, aiFirstOfMonth(), today())
}

// ---------------------------------------------------------------------------
// Generación de informes en UN SOLO PROMPT
//
// El flujo anterior (clasificación de módulos + paso de SQL + paso de
// columnas/totales) gastaba 3-4 llamadas al proveedor para un solo informe,
// y cada reintento del guardián sumaba otra más. Como el esquema COMPLETO de
// la base queda ya cacheado por huella (ai_schema_introspect.go), la fase de
// clasificación sobró: el modelo ve toda la base documentada de una vez y
// redacta la definición completa (SQL + columnas + totales) en UNA llamada.
// Si esa definición no pasa el guardián o falla la prueba de ejecución, se
// permite UNA corrección llevando el error real en el prompt. Mejor caso:
// 1 llamada. Peor caso: 2. Nunca un bucle de 8 pasos.
// ---------------------------------------------------------------------------

// aiContratoInforme son las reglas del sistema, harcodeadas, que viajan en el
// system prompt de la generación: el contrato del JSON de respuesta, del SQL
// permitido y del motor de totales. No se negocia con el modelo y no depende
// de lo que pida el usuario: es el mismo candado para todos, como el guardián.
const aiContratoInforme = `CONTRATO DE RESPUESTA — REGLAS DEL SISTEMA (no negociables):
1. Responde ÚNICAMENTE con UN objeto JSON: sin markdown, sin vallado de código y sin texto fuera del objeto. Sus claves exactas:
   {"title":"<nombre corto>","sql":"<un único SELECT>","columns":[{"key":"...","label":"..."}],"totals":["<key>"]}
2. "sql" es UN ÚNICO SELECT de lectura, con tablas y columnas del esquema entregado. Los «join» documentados en cada tabla del esquema te dicen cómo unirlas.
3. El "key" de CADA columna de "columns" DEBE ser el alias EXACTO que usaste con "AS" en el SELECT (o el nombre de la columna, si no usaste alias). El motor del informe suma los totales por ese key.
4. Cada elemento de "totals" DEBE ser una "key" ya declarada en "columns". Un total de una key que el SELECT no devuelve deja el informe sin cifras: se rechaza.
5. Para fechas usa :desde y :hasta. NINGÚN otro parámetro con nombre: un :algo distinto rompe el SQL con error 1064.
6. Si la solicitud es un ajuste sobre un SQL previo, modifícalo conservando lo que ya estaba bien.
7. "chart" es OPCIONAL: si el informe tendrá al menos una columna categórica (fechas, nombres, periodos) y una numérica, propón un gráfico embebido con {"type":"line|bar|pie|doughnut","x_key":"<key de columns>","y_key":"<key de columns>","y_label":"<etiqueta>","title":"<título>"}. "x_key" y "y_key" DEBEN ser "key" ya declaradas en "columns": el gráfico se alimenta de las MISMAS filas del informe. Si no procede (una sola fila, todo texto), OMITE la clave "chart". El gráfico se dibuja en pantalla; no sale en el PDF.`

// iaPromptInforme arma el ÚNICO prompt de generación. En el system va el
// contrato (reglas harcodeadas); en el user van la solicitud del usuario, el
// SQL previo si es un ajuste y el esquema completo cacheado del MCP, con la
// documentación de cada tabla y campo, para que la IA decida todo de una vez
// sin iterar.
func iaPromptInforme(instruction, esquema, previousSQL string) (system, user string) {
	system = "Eres el diseñador de informes de ERPGo (agente de TI). Escribes la definición JSON completa de un informe personalizado de negocio.\n\n" + aiContratoInforme
	var b strings.Builder
	b.WriteString("SOLICITUD DEL USUARIO:\n" + instruction + "\n\n")
	if strings.TrimSpace(previousSQL) != "" {
		b.WriteString("SQL PREVIO DEL INFORME (ajústalo según lo pedido; si ya cumple la solicitud, repítelo igual):\n" + previousSQL + "\n\n")
	}
	b.WriteString("ESQUEMA COMPLETO DE LA BASE DE DATOS (JSON del MCP: estructura de cada tabla, tipo de cada campo, comentarios documentados, joins y descripciones de negocio; estúdialo entero antes de decidir):\n" + esquema + "\n\n")
	b.WriteString("Entrega la definición completa del informe: solo el objeto JSON del contrato. Si el esquema no muestra algo que necesitas, decide con lo que hay: no preguntes.")
	return system, b.String()
}

// iaRespuestaADefinicion convierte la respuesta del modelo en la definición,
// garantizando solo lo mínimo (JSON + claves del contrato). La validación
// completa (guardián + ejecución) la hace el preview. Si el modelo rodeó el
// JSON con prosa a pesar del contrato, el escáner de objetos
// (aiExtraerDefinicion) busca el primero que traiga «sql».
func iaRespuestaADefinicion(resp string) (reportDefShape, error) {
	var def reportDefShape
	raw := aiLimpiarJSON(resp)
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		embedded, ok := aiExtraerDefinicion(resp)
		if !ok {
			return def, fmt.Errorf("la respuesta no es el objeto JSON del contrato: %v", err)
		}
		if err := json.Unmarshal([]byte(embedded), &def); err != nil {
			return def, fmt.Errorf("el JSON extraído no decodifica como definición: %v", err)
		}
	}
	if strings.TrimSpace(def.SQL) == "" || len(def.Columns) == 0 {
		return def, fmt.Errorf("la definición recibida no trae «sql» o «columns»")
	}
	return def, nil
}

// aiPromptCorreccion arma el mensaje de la corrección única: el JSON que
// produjo el modelo y el fallo REAL (guardián o prueba de ejecución), para que
// corrija sobre su propio trabajo en vez de empezar de cero.
func aiPromptCorreccion(falla, defJSON string) string {
	return "TU DEFINICIÓN ANTERIOR FALLÓ.\n\nDEFINICIÓN QUE ENTREGASTE:\n" + defJSON + "\n\nEL FALLO QUE DEVOLVIÓ EL MOTOR:\n" + aiTruncar(falla, 600) + "\n\nCorrígela respetando el contrato y entrega ÚNICAMENTE el objeto JSON corregido, sin markdown ni texto fuera del objeto."
}

// aiTruncar acota un texto a n runas. Para mensajes de corrección y de log:
// el fallo completo de una consulta puede ser kilométrico y el modelo y la
// consola no necesitan la cabecera entera del stack de MySQL.
func aiTruncar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " …(truncado)…"
}

// iaGenerarInforme es el núcleo compartido de ai-preview y ai-refine: manda el
// prompt único, valida con el guardián, ejecuta la vista previa y —si la
// definición falla— hace UNA corrección con el error real. El caller decide
// qué hacer con el resultado (mostrarlo en el modal, guardar, etc.).
func (s *Server) iaGenerarInforme(sess *SessionData, instruction, previousSQL string, onProgress ...func(string)) (def reportDefShape, name, desc string, preview *reportResult, err error) {
	if sess == nil {
		err = fmt.Errorf("sesión no iniciada")
		return
	}
	var progress func(string)
	if len(onProgress) > 0 && onProgress[0] != nil {
		progress = onProgress[0]
	}
	if progress != nil {
		progress("Preparando el esquema completo de la base (cache del MCP)...")
	}
	agent, ok := s.aiDefaultITAgent(sess)
	if !ok {
		err = fmt.Errorf("no hay agentes IA activos. crea uno en Agentes IA")
		return
	}
	p, ok := s.aiProviderForAgent(sess, agent)
	if !ok {
		err = fmt.Errorf("no hay proveedor de IA activo con API key: actívalo en Configuración IA")
		return
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = aiModelPorDefecto(p.Vendor)
	}
	esquema := s.aiEsquemaGlobalCompact(sess)

	log.Printf("[AI report] === INFORME EN UN SOLO PROMPT (esquema completo: %d bytes) pedido: %.120s ===", len(esquema), instruction)
	system, user := iaPromptInforme(instruction, esquema, previousSQL)
	mensajes := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}

	if progress != nil {
		progress("Consultando a la IA (un solo prompt: reglas + solicitud + esquema completo)...")
	}
	resp, _, _, err := aiCallChatWithTools(p, model, mensajes, "")
	if err != nil {
		err = fmt.Errorf("no se pudo consultar a la IA: %v", err)
		return
	}

	def, pErr := iaRespuestaADefinicion(resp)
	falla, defJSON := "", ""
	if pErr != nil {
		falla = fmt.Sprintf("el modelo no entregó el objeto JSON del contrato: %v", pErr)
		log.Printf("[AI report] El modelo no entregó JSON: %v", pErr)
	} else {
		defJSON = mustJSON(def)
		_, preview, pErr = s.previewDefinitionFromJSON(sess, defJSON)
		switch {
		case pErr != nil:
			falla = fmt.Sprintf("la definición no pasa el guardián o la prueba de ejecución: %v", pErr)
			log.Printf("[AI report] La definición falló la prueba: %v", pErr)
			preview = nil
		case len(preview.Warnings) > 0:
			// Avisos de totales desalineados: el contrato los prohíbe y el
			// ai-save bloquearía por ellos, así que se corrigen aquí.
			falla = fmt.Sprintf("la definición ejecuta pero incumple el contrato de totales: %s", strings.Join(preview.Warnings, "; "))
			log.Printf("[AI report] Warnings del preview: %s", strings.Join(preview.Warnings, "; "))
		}
		// Sin fallos: preview validó bien y err queda nil.
	}

	if falla != "" {
		// Corrección ÚNICA, en la misma conversación: el fallo real entra en el
		// mensaje, no hay que reinyectar el esquema (el contexto ya lo tiene).
		if progress != nil {
			progress("La definición no pasó la prueba. Corrección única con el error real...")
		}
		log.Printf("[AI report] Corrección única por: %.200s", falla)
		withCorreccion := append(mensajes,
			map[string]string{"role": "assistant", "content": resp},
			map[string]string{"role": "user", "content": aiPromptCorreccion(falla, aiTruncar(defJSON, 8000))})
		resp2, _, _, err2 := aiCallChatWithTools(p, model, withCorreccion, "")
		if err2 != nil {
			err = fmt.Errorf("la corrección tampoco se pudo consultar: %v", err2)
			return
		}
		def, err = iaRespuestaADefinicion(resp2)
		if err != nil {
			err = fmt.Errorf("la corrección tampoco entregó el objeto JSON del contrato: %v", err)
			return
		}
		preview = nil
		_, preview, err = s.previewDefinitionFromJSON(sess, mustJSON(def))
		if err != nil {
			err = fmt.Errorf("el informe falló también con la corrección: %v", err)
			return
		}
		// En el preview interactivo los avisos de la corrección no bloquean:
		// se muestran en el modal y el usuario ajusta; el bloqueo definitivo
		// está en ai-save.
	}

	if progress != nil {
		progress("Vista previa lista.")
	}
	name = strings.TrimSpace(def.Title)
	desc = strings.TrimSpace(def.Title)
	log.Printf("[AI report] === INFORME OK (un solo prompt, %d llamada(s)): %s (columnas=%d, filas=%d) ===", len(mensajes)/2, name, len(def.Columns), filasDePreview(preview))
	return def, name, desc, preview, nil
}

// iaPreviewDefinition es el camino del borrador inicial: genera la definición
// desde cero (previousSQL vacío).
func (s *Server) iaPreviewDefinition(sess *SessionData, instruction string, onProgress ...func(string)) (def reportDefShape, name, desc string, preview *reportResult, err error) {
	return s.iaGenerarInforme(sess, instruction, "", onProgress...)
}

// iaRefineDefinition es el camino del ajuste: la IA recibe el SQL previo y lo
// modifica según el feedback del usuario, en vez de regenerarlo desde cero.
func (s *Server) iaRefineDefinition(sess *SessionData, instruction, previousSQL string, onProgress ...func(string)) (def reportDefShape, name, desc string, preview *reportResult, err error) {
	return s.iaGenerarInforme(sess, instruction, previousSQL, onProgress...)
}

// iaSQLPrevioExtrae devuelve el "sql" de una definición JSON de informe. Vacío
// si la definición no se puede leer: en ese caso el refine se comporta como un
// borrador (no hay previo que ajustar).
func iaSQLPrevioExtrae(raw json.RawMessage) string {
	var d struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return ""
	}
	return strings.TrimSpace(d.SQL)
}

// aiLimpiarSQL quita el vallado de código markdown y texto extra del SQL.
func aiLimpiarSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	if strings.HasPrefix(sql, "```") {
		sql = strings.TrimPrefix(sql, "```sql")
		sql = strings.TrimPrefix(sql, "```")
		sql = strings.TrimSuffix(sql, "```")
		sql = strings.TrimSpace(sql)
	}
	return sql
}

// aiLimpiarJSON quita el vallado de código markdown del JSON.
func aiLimpiarJSON(jsonStr string) string {
	jsonStr = strings.TrimSpace(jsonStr)
	if strings.HasPrefix(jsonStr, "```") {
		jsonStr = strings.TrimPrefix(jsonStr, "```json")
		jsonStr = strings.TrimPrefix(jsonStr, "```")
		jsonStr = strings.TrimSuffix(jsonStr, "```")
		jsonStr = strings.TrimSpace(jsonStr)
	}
	return jsonStr
}

func filasDePreview(p *reportResult) int {
	if p == nil || p.Rows == nil {
		return 0
	}
	return len(p.Rows)
}

// aiPreviewInstruction arma la instrucción para pedir al agente una definición.
// Usa el mapa de tablas (MCP) como contexto y cierra el contrato JSON.
//
// POR QUÉ EL CONTRATO ES ESTRICTO: el agente tiende a declarar columns con
// keys inventadas ("total_ventas") que no son los alias del SELECT, y el
// preview falla con "no se pudo calcular el total" porque el motor suma por
// la key declarado. La regla "key = alias exacto del SELECT (con AS)" evita
// que el agente se saltee validar columna a columna.
// aiSolicitudCruda devuelve la solicitud del usuario SIN contrato añadido:
// el flujo de un solo prompt lleva su contrato harcodeado en el system
// prompt (aiContratoInforme). Incrustar aquí otro contrato de JSON
// contradecía al del prompt único y el modelo respondía con JSON donde se
// le pedía otra cosa.
func aiSolicitudCruda(request string) string {
	return strings.TrimSpace(request)
}
func aiPreviewInstruction(request string) string {
	return request + "\n\nCONTRATO DE RESPUESTA (¡ESCRÍBELO BIEN LA PRIMERA VEZ!):\n" +
		"Responde ÚNICAMENTE con la definición del informe como objeto JSON con estas claves:\n" +
		`  "title" : nombre corto del informe.\n` +
		`  "sql"   : un ÚNICO SELECT de lectura sobre las tablas del mapa.\n` +
		`  "columns": [{key,label}...]. IMPORTANTE: el "key" de CADA columna DEBE ser EXACTAMENTE el alias que usaste en el SELECT con "AS". Si no usas AS, el key debe ser el nombre de la columna de la tabla tal cual aparece en el mapa.\n` +
		`  "totals": ["clave1", "clave2"...]. CADA total DEBE ser una "key" que ya declaraste en "columns". Si una columna no aparece en el resultado del SELECT, el total no se puede calcular.\n` +
		`  "chart" : OPCIONAL. Si el informe tendrá una columna categórica y una numérica, propón {"type":"line|bar|pie|doughnut","x_key":"<key de columns>","y_key":"<key de columns>","y_label":"<etiqueta>","title":"<título>"}. x_key e y_key DEBEN ser keys declaradas en columns. Si no procede, OMITE la clave.\n` +
		`Reglas: Usa :desde/:hasta para fechas. NO uses ningún otro parámetro con nombre (:algo): el motor solo resuelve :desde y :hasta; un :param distinto hace fallar el SQL con error 1064. Sin markdown, sin vallado de código, sin texto fuera del objeto. Verifica que cada "key" en columns coincide con un alias/AS del SELECT antes de devolver.`
}

// crearInformeDesdeRespuestaIA extrae la definición del texto del agente, la
// valida y ejecuta (preview) y guarda la fila con la marca ia: en created_by,
// para que la auditoría diga quién escribió de verdad.
//
// Este helper existe para los tests que ya tienen la respuesta del agente en
// mano (sin gastar una corrida). El flujo productivo interactivo usa
// iaPreviewDefinition + reportTemplateAISave, pero la lógica de validación y
// ejecución previa es idéntica (delegada a previewDefinitionFromJSON).
func (s *Server) crearInformeDesdeRespuestaIA(r *http.Request, sess *SessionData, pedido, respuesta string) (int64, string, error) {
	def, ok := aiExtraerDefinicion(respuesta)
	if !ok {
		return 0, "", fmt.Errorf("la respuesta de la IA no contiene una definición JSON con «sql»")
	}
	d, pre, err := s.previewDefinitionFromJSON(sess, def)
	if err != nil {
		return 0, "", fmt.Errorf("la definición de la IA no pasa la prueba de ejecución: %v", err)
	}
	// Este helper GUARDA directamente (flujo antiguo de un solo paso), así que
	// respeta los warnings del preview: un total que no se puede calcular deja
	// el informe vacío y es mejor bloquear aquí. El flujo INTERACTIVO moderno
	// (ai-preview → ai-refine → ai-save) muestra los warnings en pantalla y
	// los resuelve con un refine, pero el save final también los rechaza.
	if len(pre.Warnings) > 0 {
		return 0, "", fmt.Errorf("la definición no pasa la prueba de ejecución: %s. Corrígela antes de guardar",
			strings.Join(pre.Warnings, "; "))
	}
	nombre := strings.TrimSpace(d.Title)
	if nombre == "" {
		nombre = strings.TrimSpace(pedido)
	}
	if cortado := []rune(nombre); len(cortado) > 80 {
		nombre = string(cortado[:80])
	}
	id, err := s.insertarInforme(r, sess, nombre, strings.TrimSpace(pedido), def, "ia:"+sessUserID(sess))
	return id, nombre, err
}

// reportTemplateAICreate mantiene el endpoint original (ya no en routing, pero
// aún llamado por tests). Valida el request, pide la definición al agente, la
// guarda de una sola vez (visibilidad pública: sin ACL). Reemplazado en el
// frontend interactivo por ai-preview + ai-save, pero preservado para compat.
func (s *Server) reportTemplateAICreate(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	var req struct {
		Request string `json:"request"`
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		jsonError(w, "Cuerpo inválido", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Request) == "" {
		jsonError(w, "Describe el informe que necesitas", http.StatusBadRequest)
		return
	}
	agent, ok := s.aiDefaultITAgent(sess)
	if !ok {
		jsonError(w, "No hay agentes IA activos. Crea uno en Agentes IA.", http.StatusServiceUnavailable)
		return
	}
	journal := &aiRunJournal{scope: "adhoc:" + now()}
	steps := []aiStep{}
	resp, runErr := s.aiRunLoop(sess, agent, aiTask{
		AgentID:               agent.ID,
		Name:                  "Creación de informe",
		Instructions:          aiPreviewInstruction(req.Request),
		RespuestaJSON:         true,
		HerramientasExcluidas: []string{"report.save_template"},
	}, &steps, journal)
	if runErr != nil {
		jsonError(w, runErr.Error(), http.StatusBadGateway)
		return
	}
	id, nombre, crearErr := s.crearInformeDesdeRespuestaIA(r, sess, req.Request, resp)
	if crearErr != nil {
		jsonError(w, crearErr.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "id": id, "name": nombre,
		"redirect": "/reports/custom/edit/" + strconv.FormatInt(id, 10)})
}

// reportTemplateAIPreview genera una definición por IA + preview de ejecución.
// NO guarda: el usuario itera (refinar) o confirma (ai-save). Reemplaza al viejo
// ai-create que guardaba en un solo paso sin dejar pulir la definición.
func (s *Server) reportTemplateAIPreview(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	var req struct {
		Request string `json:"request"`
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		jsonError(w, "Cuerpo inválido", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Request) == "" {
		jsonError(w, "Describe el informe que necesitas", http.StatusBadRequest)
		return
	}
	def, name, desc, preview, pErr := s.iaPreviewDefinition(sess, aiSolicitudCruda(req.Request))
	if pErr != nil {
		jsonError(w, pErr.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":      1,
		"request":     req.Request,
		"definition":  mustJSON(def),
		"title":       name,
		"description": desc,
		"preview":     previewJSON(preview),
	})
}

// aiSSEWriteGrace es el margen de escritura que cada evento SSE concede a la
// conexión desde el instante de su flush. La brecha más larga entre dos eventos
// es una llamada al proveedor —acotada por aiTimeout— más una consulta a la
// base; dos veces aiTimeout la cubre con holgura y además evita que quede
// abierta para siempre una conexión muerta.
const aiSSEWriteGrace = 2 * aiTimeout

// aiSSE escribe un evento Server-Sent-Event y hace flush.
//
// Antes del flush RENUEVA el plazo de escritura de la conexión: el WriteTimeout
// global del servidor (120 s) se aplica al total de la respuesta, y el agente
// el flujo puede tardar más en diseñar un informe (hasta DOS llamadas a
// aiTimeout cada una). Sin esta renovación, net/http cortaba la
// conexión a los 120 s con el stream vivo y el modal nunca recibía el evento
// «listo»: la IA gastaba sus tokens y el usuario nunca veía la respuesta.
func aiSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(aiSSEWriteGrace))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// reportTemplateAIPreviewStream genera el informe con IA y emite el progreso en
// vivo vía SSE. Cada paso de iaPreviewDefinition llega como evento "progreso";
// el resultado final llega como "listo" (status=1) o "error" (status!=1).
func (s *Server) reportTemplateAIPreviewStream(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		http.Error(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	var req struct {
		Request string `json:"request"`
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		http.Error(w, "Cuerpo inválido", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Request) == "" {
		http.Error(w, "Describe el informe que necesitas", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	progress := func(msg string) {
		aiSSE(w, "progreso", msg)
	}

	def, name, desc, preview, pErr := s.iaPreviewDefinition(sess, aiSolicitudCruda(req.Request), progress)
	if pErr != nil {
		aiSSE(w, "error", mustJSON(map[string]interface{}{"status": 0, "error": pErr.Error()}))
		return
	}
	aiSSE(w, "listo", string(mustJSON(map[string]interface{}{
		"status":      1,
		"request":     req.Request,
		"definition":  mustJSON(def),
		"title":       name,
		"description": desc,
		"preview":     previewJSON(preview),
	})))
}

// reportTemplateAIRefine ajusta la definición según el feedback del usuario y
// devuelve una nueva vista previa. El modelo recibe el SQL previo (y el JSON
// completo como contexto) para que ajuste solo lo pedido.
func (s *Server) reportTemplateAIRefine(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	var req struct {
		Request    string          `json:"request"`    // feedback del usuario
		Definition json.RawMessage `json:"definition"` // JSON previo
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		jsonError(w, "Cuerpo inválido", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Request) == "" {
		jsonError(w, "Describe el ajuste que necesitas", http.StatusBadRequest)
		return
	}
	if len(req.Definition) == 0 {
		jsonError(w, "Debes generar primero el borrador (falta la definición previa)", http.StatusBadRequest)
		return
	}
	// La instrucción lleva el JSON previo incrustado y, además, se pasa el SQL
	// previo aparte: el modelo ajusta sobre lo que ya había en vez de empezar
	// desde cero, preservando el resto.
	instruction := fmt.Sprintf(
		"Ajusta el informe según este feedback del usuario: «%s».\n\nDEFINICIÓN PREVIA (JSON, solo contexto; el SQL exacto va aparte):\n%s",
		req.Request, string(req.Definition))
	prevSQL := iaSQLPrevioExtrae(req.Definition)
	def, name, desc, preview, pErr := s.iaRefineDefinition(sess, instruction, prevSQL)
	if pErr != nil {
		jsonError(w, pErr.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":      1,
		"definition":  mustJSON(def),
		"title":       name,
		"description": desc,
		"preview":     previewJSON(preview),
	})
}

// reportTemplateAIRefineStream: igual que ai-refine pero con SSE.
func (s *Server) reportTemplateAIRefineStream(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		http.Error(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	var req struct {
		Request    string          `json:"request"`
		Definition json.RawMessage `json:"definition"`
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		http.Error(w, "Cuerpo inválido", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Request) == "" {
		http.Error(w, "Describe el ajuste que necesitas", http.StatusBadRequest)
		return
	}
	if len(req.Definition) == 0 {
		http.Error(w, "Debes generar primero el borrador", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	progress := func(msg string) { aiSSE(w, "progreso", msg) }
	instruction := fmt.Sprintf(
		"Ajusta el informe según este feedback del usuario: «%s».\n\nDEFINICIÓN PREVIA (JSON, solo contexto; el SQL exacto va aparte):\n%s",
		req.Request, string(req.Definition))
	prevSQL := iaSQLPrevioExtrae(req.Definition)
	def, name, desc, preview, pErr := s.iaRefineDefinition(sess, instruction, prevSQL, progress)
	if pErr != nil {
		aiSSE(w, "error", mustJSON(map[string]interface{}{"status": 0, "error": pErr.Error()}))
		return
	}
	aiSSE(w, "listo", string(mustJSON(map[string]interface{}{
		"status":      1,
		"definition":  mustJSON(def),
		"title":       name,
		"description": desc,
		"preview":     previewJSON(preview),
	})))
}

// reportTemplateAISave guarda el informe (definición validada y ejecutada) con
// visibilidad por role_id/user_id. El cliente ya vio el preview; aquí se valida
// de nuevo (capa defensiva) y se persiste + la auditoría.
func (s *Server) reportTemplateAISave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Definition  string  `json:"definition"`
		RoleIDs     []int64 `json:"role_ids"` // NULL o vacío = visible para todos
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		jsonError(w, "Cuerpo inválido", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Definition = strings.TrimSpace(req.Definition)
	if req.Name == "" || req.Definition == "" {
		jsonError(w, "Nombre y definition son obligatorios", http.StatusBadRequest)
		return
	}
	if err := s.validateReportDefinition(req.Definition); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Re-ejecutar como chequeo final (el agente pudo haber ajustado algo entre
	// el preview y el save). Un warning aquí bloquea el guardado.
	preview, execErr := s.executeDefinitionPreview(sess, req.Definition)
	if execErr != nil {
		jsonError(w, "la definición falló al ejecutar: "+execErr.Error(), http.StatusBadRequest)
		return
	}
	if len(preview.Warnings) > 0 {
		jsonError(w, "la definición tiene warnings: "+strings.Join(preview.Warnings, "; "), http.StatusBadRequest)
		return
	}
	creator := "ia:" + sessUserID(sess)
	id, err := s.insertarInforme(r, sess, req.Name, req.Description, req.Definition, creator)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.insertarReporteACL(r, sess, id, req.RoleIDs); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "id": id,
		"redirect": "/reports/custom/edit/" + strconv.FormatInt(id, 10)})
}

// previewJSON serializa un reportResult para la respuesta del preview.
func previewJSON(r *reportResult) map[string]interface{} {
	if r == nil {
		return nil
	}
	out := map[string]interface{}{
		"title":    r.Title,
		"headers":  r.Headers,
		"rows":     r.Rows,
		"totals":   r.Totals,
		"warnings": r.Warnings,
	}
	// El modal dibuja el gráfico con Chart.js cuando llega chart_data; si no
	// hay chart, la clave simplemente no existe y el render no pinta nada.
	if r.Chart != nil {
		out["chart_data"] = r.Chart
	}
	return out
}

// insertarReporteACL graba la visibilidad de un informe en report_template_acl.
// Si roleIDs es vacío (nil), no se insertan filas: el informe es visible para
// TODOS (regla: SIN filas → todos ven). Esto preserva el comportamiento actual
// de los informes existentes y da al creador control explícito al pasar roles.
func (s *Server) insertarReporteACL(r *http.Request, sess *SessionData, reportID int64, roleIDs []int64) error {
	db := s.dbFor(r)
	if db == nil {
		return fmt.Errorf("base no disponible")
	}
	// Vacío: el informe es público (sin filas = todos ven). Esto permite que un
	// creador deje el informe visible a todos por defecto.
	if len(roleIDs) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM report_template_acl WHERE report_template_id = ?`, reportID); err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if _, err := tx.Exec(`INSERT INTO report_template_acl (report_template_id, role_id, user_id) VALUES (?,?,NULL)`, reportID, rid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// reportCanView verifica la visibilidad de un informe para un usuario/rol.
// Regla:
//   - SIN filas en report_template_acl para el informe → visible para TODOS.
//   - SI hay filas → visible si alguna coincide con su role_id o user_id.
//   - una fila con role_id=NULL y user_id=NULL → "todos los roles/usuarios" (público explícito).
//
// La tabla report_template_acl puede no existir todavía (pre-migration): en
// ese caso la query de COUNT falla y se asume visible para todos (fallback seguro).
func (s *Server) reportCanView(sess *SessionData, reportID int64) bool {
	if sess == nil {
		return false
	}
	db := s.dbForSession(sess)
	if db == nil {
		return false
	}
	// ¿hay filas de ACL para este informe? Si la tabla no existe, error → fallback "todxs ven".
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM report_template_acl WHERE report_template_id = ?`, reportID).Scan(&count); err != nil {
		return true
	}
	if count == 0 {
		return true // sin filas → visible para todos
	}
	roleID := s.sessionRoleID(sess)
	userID := sessUserID(sess)
	var match int
	_ = db.QueryRow(`SELECT COUNT(*) FROM report_template_acl
		WHERE report_template_id = ?
		  AND (role_id = ? OR user_id = ? OR (role_id IS NULL AND user_id IS NULL))`,
		reportID, roleID, userID).Scan(&match)
	return match > 0
}

// insertarInforme guarda la fila y escribe la auditoría. El guardado manual y
// la creación por IA comparten ESTE camino: así ninguna de las dos puede crear
// un informe por debajo del guardián o sin dejar rastro en la auditoría.
func (s *Server) insertarInforme(r *http.Request, sess *SessionData, name, desc, definition, creator string) (int64, error) {
	res, err := s.dbFor(r).Exec(`INSERT INTO report_template (name, description, definition, created_by, status, created_at, updated_at)
		VALUES (?,?,?,?,1,?,?)`, name, desc, definition, creator, now(), now())
	if err != nil {
		return 0, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	s.appendAudit(r, sessUserID(sess), "ia", "create", "custom_reports", "report_template", fmt.Sprintf("%d", newID), nil, map[string]interface{}{"name": name})
	return newID, nil
}

// aiExtraerDefinicion encuentra la definición JSON del informe dentro del texto
// del agente. Un regex goloso (como el de la pantalla del borrador) se come
// desde el primer '{' de la prosa hasta el último '}' del texto; este escaneo
// recorre los objetos con llaves emparejadas respetando comillas y escapes,
// y devuelve el primero que decodifica como plantilla con «sql».
func aiExtraerDefinicion(texto string) (string, bool) {
	for i := 0; i < len(texto); i++ {
		if texto[i] != '{' {
			continue
		}
		profundidad, enCadena, escapado := 0, false, false
	bucleObjeto:
		for j := i; j < len(texto); j++ {
			c := texto[j]
			if enCadena {
				switch {
				case escapado:
					escapado = false
				case c == '\\':
					escapado = true
				case c == '"':
					enCadena = false
				}
				continue
			}
			switch c {
			case '"':
				enCadena = true
			case '{':
				profundidad++
			case '}':
				profundidad--
				if profundidad == 0 {
					candidato := texto[i : j+1]
					var sondeo struct {
						SQL string `json:"sql"`
					}
					if json.Unmarshal([]byte(candidato), &sondeo) == nil && strings.TrimSpace(sondeo.SQL) != "" {
						return candidato, true
					}
					break bucleObjeto // objeto cerrado sin «sql»: se sigue con el próximo
				}
			}
		}
	}
	return "", false
}

func (s *Server) reportTemplateList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "report_template_list.html", map[string]interface{}{"Title": "Informes Personalizados"})
}

func (s *Server) reportTemplateListData(w http.ResponseWriter, r *http.Request) {
	list := s.loadReportTemplates(s.sessionFromRequest(r))
	out := make([]map[string]interface{}, 0, len(list))
	for _, t := range list {
		var def reportDefShape
		_ = json.Unmarshal([]byte(t.Definition), &def)
		out = append(out, map[string]interface{}{
			"id": t.ID, "name": t.Name, "description": t.Description,
			"created_by": t.CreatedBy, "title": def.Title, "status": t.Status,
		})
	}
	if err := s.dbFor(r).PingContext(r.Context()); err != nil {
		jsonError(w, "No se pudo consultar la lista de informes", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, out)
}

// reportTemplateForm: formulario + datos del MODAL DE AYUDA (mapa de tablas).
func (s *Server) reportTemplateForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "Nuevo Informe Personalizado",
		"CMap":  s.aiSchemaMap(s.sessionFromRequest(r)),
	}
	if idStr := r.PathValue("id"); idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		for _, t := range s.loadReportTemplates(s.sessionFromRequest(r)) {
			if t.ID == id {
				data["Title"] = "Editar Informe Personalizado"
				data["Template"] = t
				break
			}
		}
	}
	s.render(w, r, "report_template_form.html", data)
}

// reportTemplateRoles lista los roles para el selector de visibilidad del modal
// de creación asistida. Viene de sec_role (BD de control), que es la única
// fuente de truth para roles.
func (s *Server) reportTemplateRoles(w http.ResponseWriter, r *http.Request) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		// Sin control (tests): intentar la BD de empresa.
		ctl = s.dbFor(r)
	}
	if ctl == nil {
		writeJSON(w, []interface{}{})
		return
	}
	rows, err := ctl.Query(`SELECT id, type, level FROM sec_role ORDER BY id`)
	if err != nil {
		writeJSON(w, []interface{}{})
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var typ, level string
		_ = rows.Scan(&id, &typ, &level)
		list = append(list, map[string]interface{}{
			"id": id, "type": typ, "level": level,
		})
	}
	writeJSON(w, list)
}

// reportTemplateEngineParams devuelve los filtros para una plantilla creada
// desde la pantalla clásica. Ese formato no necesariamente satisface
// ReportDefinition.Validate (por ejemplo, usa parámetros propios); no se
// intenta reinterpretarlo aquí: responde la forma que el motor puede ejecutar.
func (s *Server) reportTemplateEngineParams(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", http.StatusBadRequest)
		return
	}
	for _, t := range s.loadReportTemplates(s.sessionFromRequest(r)) {
		if t.ID == id && t.Status == 1 {
			var def ReportDefinition
			if err := json.Unmarshal([]byte(t.Definition), &def); err != nil {
				jsonError(w, "La definición del informe está dañada", http.StatusUnprocessableEntity)
				return
			}
			writeJSON(w, map[string]interface{}{"id": t.ID, "title": def.Title, "params": def.Params, "columns": def.Columns})
			return
		}
	}
	jsonError(w, "La plantilla no existe", http.StatusNotFound)
}

// reportTemplateEngineRun ejecuta desde el motor general un informe clásico
// con la definición JSON tipada. Así el enlace mostrado en Mis Informes siempre
// tiene un camino de lectura y ejecución coherente con la pantalla.
func (s *Server) reportTemplateEngineRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", http.StatusBadRequest)
		return
	}
	t, found := s.reportTemplateByID(r, id)
	if !found {
		jsonError(w, "La plantilla no existe", http.StatusNotFound)
		return
	}
	run, err := s.runSavedReportDefinition(r, t.Definition)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, reportRunJSON(run))
}

// reportTemplateEngineExport deja la exportación del motor accesible para la
// pantalla Mis Informes sin cambiar los enlaces de la lista existente.
func (s *Server) reportTemplateEngineExport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", http.StatusBadRequest)
		return
	}
	t, found := s.reportTemplateByID(r, id)
	if !found {
		jsonError(w, "La plantilla no existe", http.StatusNotFound)
		return
	}
	run, err := s.runSavedReportDefinition(r, t.Definition)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := ReportCSV(run)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := reportSafeFilename(t.Name)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.csv\"", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), "", "export_report", "custom_reports", "report_template", strconv.FormatInt(id, 10), nil, map[string]interface{}{"rows": len(run.Rows)})
}

// runSavedReportDefinition prioriza el motor tipado, pero conserva una vía de
// ejecución para plantillas antiguas que reportTemplateSave aceptó históricamente
// (p.ej. un bloque chart legacy que no satisface la validación moderna). Esas
// definiciones se guardan en la misma tabla y tienen que seguir siendo visibles
// y ejecutables, no desaparecer o terminar en un error de JSON al seleccionarlas.
func (s *Server) runSavedReportDefinition(r *http.Request, raw string) (*ReportRun, error) {
	var def ReportDefinition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return nil, fmt.Errorf("la definición del informe está dañada: %w", err)
	}
	values := make(map[string]string, len(def.Params))
	for _, p := range def.Params {
		values[p.Name] = r.FormValue("p_" + p.Name)
	}
	if err := def.Validate(); err == nil {
		return s.RunReportDefinition(r, &def, values)
	}

	legacy, err := s.executeReportDefinition(s.sessionFromRequest(r), raw, values["desde"], values["hasta"])
	if err != nil {
		return nil, err
	}
	keys := def.ClavesDeclaradas()
	if len(keys) != len(legacy.Headers) {
		keys = append([]string(nil), legacy.Headers...)
	}
	rows := make([][]interface{}, len(legacy.Rows))
	for i := range legacy.Rows {
		rows[i] = legacy.Rows[i]
	}
	return &ReportRun{
		Title: legacy.Title, Headers: legacy.Headers, Keys: keys, Rows: rows,
		Totals: legacy.Totals, Backend: "mysql", MaxRows: aiHardMaxRows,
		Params: values, Warnings: legacy.Warnings,
	}, nil
}

func (s *Server) reportTemplateByID(r *http.Request, id int64) (reportTemplate, bool) {
	for _, t := range s.loadReportTemplates(s.sessionFromRequest(r)) {
		if t.ID == id && t.Status == 1 {
			return t, true
		}
	}
	return reportTemplate{}, false
}

// reportTemplateSave guarda la plantilla (usuario humano).
func (s *Server) reportTemplateSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, "Formulario inválido", 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	definition := strings.TrimSpace(r.FormValue("definition"))
	desc := strings.TrimSpace(r.FormValue("description"))
	if name == "" || definition == "" {
		jsonError(w, "Nombre y definition son obligatorios", 400)
		return
	}
	if err := s.validateReportDefinition(definition); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	sess := s.sessionFromRequest(r)
	creator := "user:" + sessUserID(sess)
	idStr := r.FormValue("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			jsonError(w, "Identificador inválido", http.StatusBadRequest)
			return
		}
		res, err := s.dbFor(r).Exec(`UPDATE report_template SET name=?, description=?, definition=?, updated_at=? WHERE id=? AND status=1`,
			name, desc, definition, now(), id)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		changed, err := res.RowsAffected()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if changed == 0 {
			jsonError(w, "El informe ya no existe o fue eliminado", http.StatusNotFound)
			return
		}
		s.appendAudit(r, sessUserID(sess), "ia", "update", "custom_reports", "report_template", fmt.Sprintf("%d", id), nil, map[string]interface{}{"name": name})
		redirect := "/reports/custom"
		if r.FormValue("action") == "save" {
			redirect = fmt.Sprintf("/reports/custom/edit/%d", id)
		}
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Informe actualizado", "redirect": redirect})
		return
	}
	newID, err := s.insertarInforme(r, sess, name, desc, definition, creator)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirect := "/reports/custom"
	if r.FormValue("action") == "save" {
		redirect = "/reports/custom/edit/" + strconv.FormatInt(newID, 10)
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Informe creado", "id": newID, "redirect": redirect})
}

// reportTemplateDelete desactiva (borrado lógico).
func (s *Server) reportTemplateDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	if _, err := s.dbFor(r).Exec(`UPDATE report_template SET status=0, updated_at=? WHERE id=?`, now(), id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	sess := s.sessionFromRequest(r)
	s.appendAudit(r, sessUserID(sess), "ia", "delete", "custom_reports", "report_template", fmt.Sprintf("%d", id), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Informe eliminado", "redirect": saveRedirect(r, "/reports/custom")})
}

// reportTemplateRun ejecuta la plantilla con el rango de fechas elegido.
func (s *Server) reportTemplateRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Identificador inválido", 400)
		return
	}
	var tpl *reportTemplate
	list := s.loadReportTemplates(s.sessionFromRequest(r))
	for i := range list {
		if list[i].ID == id {
			tpl = &list[i]
			break
		}
	}
	if tpl == nil {
		http.Error(w, "Informe no encontrado", 404)
		return
	}
	from := orDefault(r.FormValue("from"), aiFirstOfMonth())
	to := orDefault(r.FormValue("to"), today())
	res, err := s.executeReportDefinition(s.sessionFromRequest(r), tpl.Definition, from, to)
	if err != nil {
		s.render(w, r, "report_template_run.html", map[string]interface{}{
			"Title": tpl.Name, "Err": err.Error(), "From": from, "To": to, "TplID": tpl.ID,
		})
		return
	}
	s.render(w, r, "report_template_run.html", map[string]interface{}{
		"Title": tpl.Name, "Res": res, "From": from, "To": to, "TplID": tpl.ID,
	})
}

// reportTemplateAIHelp pide al agente de TI por defecto un borrador de
// plantilla. La petición viaja como TAREA de una sola pasada del runner.
func (s *Server) reportTemplateAIHelp(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no iniciada", 401)
		return
	}
	var req struct {
		Request string `json:"request"`
	}
	body, err := readAllLimit(r, 1<<20)
	if err != nil {
		jsonError(w, "Cuerpo inválido", 400)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Request) == "" {
		jsonError(w, "Describe el informe que necesitas", 400)
		return
	}
	agent, ok := s.aiDefaultITAgent(s.sessionFromRequest(r))
	if !ok {
		jsonError(w, "No hay agentes IA activos. Crea uno en Agentes IA.", 503)
		return
	}
	task := aiTask{AgentID: agent.ID, Name: "Ayuda de diseño de informe", Instructions: req.Request,
		// La ayuda también pide JSON: el borrador se parsea del texto para
		// rellenar el textarea, y el mismo contrato de extracción del create
		// aplica. Se excluye report.save_template: "Ayúdame" no debe guardar.
		RespuestaJSON:         true,
		HerramientasExcluidas: []string{"report.save_template"},
	}
	// Ámbito propio por invocación: esta acción es interactiva y de una sola
	// pasada, no una corrida programada. Sin él, todas estas peticiones
	// compartirían el ámbito vacío y la segunda persona que pidiera guardar la
	// misma plantilla recibiría «ya se ejecutó» en vez de su plantilla.
	journal := &aiRunJournal{scope: "adhoc:" + now()}
	steps := []aiStep{}
	report, runErr := s.aiRunLoop(s.sessionFromRequest(r), agent, task, &steps, journal)
	if runErr != nil {
		jsonError(w, runErr.Error(), 502)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "report": report})
}

// aiDefaultITAgent devuelve el agente de TI estándar (por defecto).
func (s *Server) aiDefaultITAgent(sess *SessionData) (aiAgent, bool) {
	agentes := s.loadAIAgents(sess)
	// IT (DeptIT) es el ÚNICO agente con el mapa COMPLETO de tablas (ver
	// ai_schema_introspect.go). Un agente de otro departamento solo ve su área, y
	// un informe de ventas/cuentas puede necesitar tablas de varios ámbites: si el
	// agente no conoce la tabla, el SQL queda cojo o falla al ejecutar. Preferimos
	// uno de IT; si no hay ninguno activo, caemos al primero activo —la vista ya
	// avisa de que no hay agentes activos—, antes que dejar la creación imposible.
	for _, g := range agentes {
		if g.Enabled == 1 && aiDeptDelAgente(g) == DeptIT {
			return g, true
		}
	}
	for _, g := range agentes {
		if g.Enabled == 1 {
			return g, true
		}
	}
	return aiAgent{}, false
}

// ---------- small helpers ----------

// readAllLimit lee el body con tope de tamaño.
func readAllLimit(r *http.Request, n int64) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, n))
}

// reportTemplateFirst es helper de tests (primera plantilla activa).
func (s *Server) reportTemplateFirst(sess *SessionData) (reportTemplate, bool) {
	list := s.loadReportTemplates(sess)
	if len(list) == 0 {
		return reportTemplate{}, false
	}
	return list[0], true
}

var _ = sql.NullString{}

// substituteReportParams reemplaza :desde/:hasta en el SQL de una definición
// de informe legado. El motor tipado (RunReportDefinition) valida cada parámetro
// contra su tipo declarado; aquí no hay esquema, solo el par de fechas, así que se
// sustituyen SOLO estos dos. Cualquier otro :param se deja sin tocar y el Query
// lo rechazará (comportamiento seguro: no se concatena texto no validado).
//
// Las fechas se citan como literales y se validan con aiValidDate ANTES de
// concatenar, de modo que un valor de formulario no puede convertirse en inyección
// —el guardián de solo-SELECT ya corrió, pero él no valida valores.
func substituteReportParams(sqlText, from, to string) (string, error) {
	necesita := strings.Contains(sqlText, ":desde") || strings.Contains(sqlText, ":hasta")
	if !necesita {
		return sqlText, nil
	}
	if !aiValidDate(from) {
		return "", fmt.Errorf("el informe usa :desde pero la fecha %q no es válida (use AAAA-MM-DD)", from)
	}
	if !aiValidDate(to) {
		return "", fmt.Errorf("el informe usa :hasta pero la fecha %q no es válida (use AAAA-MM-DD)", to)
	}
	sqlText = replaceParam(sqlText, "desde", "'"+from+"'")
	sqlText = replaceParam(sqlText, "hasta", "'"+to+"'")
	return sqlText, nil
}

// findSQLMatches busca coincidencias de patrón regex simple en SQL.
// Devuelve la primera palabra capturada de cada match.
func findSQLMatches(sql, pattern string) []string {
	var results []string
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(sql, -1)
	for _, m := range matches {
		if len(m) > 1 {
			results = append(results, strings.ToLower(m[1]))
		}
	}
	return results
}
