package web

// ai_reports.go — Informes personalizados por plantillas JSON. Tanto usuarios
// como agentes crean plantillas; TODAS pasan por el guardián SQL al ejecutar.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

// loadReportTemplates lista las plantillas activas.
func (s *Server) loadReportTemplates(sess *SessionData) []reportTemplate {
	rows, err := s.dbForSession(sess).Query(`SELECT id, name, IFNULL(description,''), definition, IFNULL(created_by,''), status
		FROM report_template WHERE status = 1 ORDER BY name`)
	if err != nil {
		return []reportTemplate{}
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
	return nil
}

// reportResult es el resultado de ejecutar una plantilla.
type reportResult struct {
	Title    string                 `json:"title"`
	Headers  []string               `json:"headers"`
	Rows     [][]interface{}        `json:"rows"`
	Totals   map[string]interface{} `json:"totals,omitempty"`
	Warnings []string               `json:"warnings,omitempty"`
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
	// alias del SELECT. Antes se sumaba por cols[i], que es el alias, mientras
	// los encabezados se armaban con las claves declaradas: en cuanto el agente
	// nombraba una columna distinto en el SELECT que en columns, los totales
	// salían vacíos sin decir nada.
	//
	// Se calculan los dos y se suma por los dos nombres, porque la plantilla
	// puede pedir el total por cualquiera de los dos y el motor no tiene por qué
	// saber cuál escribió el agente.
	declaradas := d.ClavesDeclaradas()
	if len(declaradas) < len(cols) {
		for len(declaradas) < len(cols) {
			declaradas = append(declaradas, "")
		}
	}
	for _, c := range d.Columns {
		res.Headers = append([]string{}, cols...)
		_ = c
		break
	}
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
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
			// Se acumula por los dos nombres posibles: el alias del SELECT y la
			// clave declarada. Así la plantilla funciona se escribiera la
			// columna como se escribiera, que es justo cuando discrepan.
			//
			// Solo se suman si son DISTINTOS: cuando coinciden (lo normal) sumarlos
			// los dos duplicaba la columna y el total salía al doble, sin ningún
			// aviso. Un total que cuadra al doble es peor que uno que no sale,
			// porque parece un dato bueno.
			//
			// Y se convierte el valor antes de sumar porque MySQL devuelve los
			// DECIMAL como []byte, no como float64: si solo se sumaran los
			// float64, NINGÚN total de dinero se calcularía jamás.
			f := valorNumerico(vals[i])
			if f == nil {
				continue
			}
			sums[cols[i]] += *f
			if d := declaradas[i]; d != "" && d != cols[i] {
				sums[d] += *f
			}
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
		// Se pidió este total y no se pudo calcular. Sin aviso, el informe sale
		// con las mismas filas y sin cifras, y quien lo lee no tiene por qué
		// sospechar que le falta el dato más importante.
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"no se pudo calcular el total de la columna %q: no aparece en el resultado de la consulta", key))
	}
	return res, nil
}

// valorNumerico convierte a float lo que venga de la base.
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
	s.mux.HandleFunc("POST /reports/custom/ai-create", s.reportTemplateAICreate)
}

// reportTemplateAICreate convierte el pedido en un informe REAL: el agente
// redacta la definición y el servidor —no la IA— la pasa por el guardián SQL y
// guarda la fila en report_template. El botón anterior («Ayúdame con la IA»)
// solo mostraba un borrador y rellenaba el textarea si estaba vacío: el informe
// «creado por la IA» quedaba a mitad de camino porque nada lo guardaba.
func (s *Server) reportTemplateAICreate(w http.ResponseWriter, r *http.Request) {
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
	// El encargo de una sola pasada: el contrato es cerrado porque la respuesta
	// se va a GUARDAR, no a leer una persona. Prosa fuera del objeto entorpece;
	// la extracción lo tolera, pero el contrato la reduce.
	instruccion := req.Request + "\n\nCONTRATO DE RESPUESTA: responde ÚNICAMENTE con la definición del informe como objeto JSON con las claves \"title\" (nombre corto), \"sql\" (un único SELECT de lectura sobre las tablas del mapa), \"columns\" ([{\"key\":...,\"label\":...}...]) y \"totals\" (solo claves declaradas en columns). Sin markdown, sin vallado de código, sin texto fuera del objeto."
	// RespuestaJSON=true hace que el runner varíe la regla 6 del sistema y el
	// sufijo del prompt para pedir JSON (ver aiReglasDelSistema). Sin esto el
	// modelo recibía "entrega un informe en Markdown" y la definición nunca se
	// extraía: la corrida se gastaba en tokens y no quedaba el informe.
	//
	// HerramientasExcluidas quita report.save_template DE ESTA corrida: el
	// guardado lo hace el servidor al final, y si el agente guardara por su
	// cuenta se duplicaría la fila.
	task := aiTask{
		AgentID:             agent.ID,
		Name:                "Creación de informe",
		Instructions:        instruccion,
		RespuestaJSON:       true,
		HerramientasExcluidas: []string{"report.save_template"},
	}
	// Ámbito propio por invocación, por el mismo motivo de reportTemplateAIHelp:
	// es interactiva y de una sola pasada.
	journal := &aiRunJournal{scope: "adhoc:" + now()}
	steps := []aiStep{}
	resp, runErr := s.aiRunLoop(s.sessionFromRequest(r), agent, task, &steps, journal)
	if runErr != nil {
		jsonError(w, runErr.Error(), 502)
		return
	}
	id, nombre, crearErr := s.crearInformeDesdeRespuestaIA(r, sess, req.Request, resp)
	if crearErr != nil {
		jsonError(w, crearErr.Error(), 502)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "id": id, "name": nombre,
		"redirect": "/reports/custom/edit/" + strconv.FormatInt(id, 10)})
}

// crearInformeDesdeRespuestaIA extrae la definición del texto del agente, la
// valida con el mismo guardián que un guardado manual y guarda la fila con la
// marca ia: en created_by, para que la auditoría diga quién escribió de verdad.
func (s *Server) crearInformeDesdeRespuestaIA(r *http.Request, sess *SessionData, pedido, respuesta string) (int64, string, error) {
	def, ok := aiExtraerDefinicion(respuesta)
	if !ok {
		return 0, "", fmt.Errorf("la respuesta de la IA no contiene una definición JSON con «sql»")
	}
	var d reportDefShape
	_ = json.Unmarshal([]byte(def), &d) // aiExtraerDefinicion ya garantizó que decodifica
	nombre := strings.TrimSpace(d.Title)
	if nombre == "" {
		nombre = strings.TrimSpace(pedido)
	}
	if cortado := []rune(nombre); len(cortado) > 80 {
		nombre = string(cortado[:80])
	}
	if err := s.validateReportDefinition(def); err != nil {
		return 0, "", fmt.Errorf("la definición de la IA no pasa el guardián: %v", err)
	}
	// Antes de GUARDAR, el informe se ejecuta una vez como lo haría un técnico:
	// el guardián estático valida sintaxis, pero no que el SQL devuelva las
	// columnas declaradas, que los totales tengan sentido, o que la tabla
	// exista. Un SELECT contra una tabla que no ve el agente (departamento mal
	// elegido) aquí tira 400 y no guarda una plantilla que explota al abrirse.
	if res, preErr := s.executeReportDefinition(sess, def, "", ""); preErr != nil {
		return 0, "", fmt.Errorf("la definición de la IA no pasa la prueba de ejecución: %v", preErr)
	} else if len(res.Warnings) > 0 {
		return 0, "", fmt.Errorf("la definición de la IA no pasa la prueba de ejecución: %s. Corrígela antes de guardar",
			strings.Join(res.Warnings, "; "))
	}
	id, err := s.insertarInforme(r, sess, nombre, strings.TrimSpace(pedido), def, "ia:"+sessUserID(sess))
	return id, nombre, err
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
		RespuestaJSON:       true,
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
