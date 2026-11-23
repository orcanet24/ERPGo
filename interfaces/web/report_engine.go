package web

// report_engine.go — Motor de informes por plantilla.
//
// Es la respuesta a la razón de ser del producto: que un humano, sin saber
// programar, defina un informe que antes exigía un técnico. La definición vive
// como JSON versionado en la base, se reutiliza y se puede auditar.
//
// DOS DECISIONES que separan este motor del de los agentes de IA:
//
//  1. Límite de filas distinto. La IA tiene un tope duro de 500 filas porque
//     arrastrarlas todas a memoria es un riesgo de agotamiento. Un humano
//     escribiendo un informe de ventas del año espera 20.000 filas, no 500. El
//     tope sigue existiendo, pero lo define la plantilla y es mucho mayor.
//
//  2. Parámetros tipados, no concatenación. El SQL de un informe lleva
//     parámetros con nombre (:desde). Se sustituyen por valores VALIDADOS según
//     su tipo declarado, y el SQL ya sustituido se vuelve a pasar por el
//     guardián de solo-SELECT. Un parámetro no puede convertirse en una vía de
//     inyección precisamente porque nunca se concatena texto libre: se valida,
//     se escapa y después se comprueba.
//
// Sobre DuckDB (el acelerador analítico del plan): el motor consulta a través
// de una interfaz, `reportBackend`, para que cambiar de SQLite a DuckDB sea
// implementar una interfaz y no reescribir el informe. No se activa aquí porque
// el driver de DuckDB exige CGO y este proyecto se compila con CGO desactivado
// para producir un binario único sin cadena de compilación. Añadirlo ahora
// rompería esa garantía. Cuando se compile con CGO, la implementación es una
// función.

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Límites del motor. Son topes de seguridad, no de diseño: existen para que un
// informe mal escrito no tumbe el servidor.
const (
	reportDefaultMaxRows  = 20000
	reportAbsoluteMaxRows = 200000
	reportParamMaxLen     = 200
)

// ReportChart es la configuración de gráfico embebida en la definición.
//
// Es opcional: un informe sin chart es una tabla, y eso sigue siendo válido.
// Cuando está presente, el motor calcula las etiquetas y los datos a partir de
// las columnas que devuelve el SQL, y el frontend dibuja el gráfico con Chart.js.
type ReportChart struct {
	Type       string `json:"type"`    // line | bar | pie | doughnut
	XKey       string `json:"x_key"`   // clave declarada en columns para el eje X
	YKey       string `json:"y_key"`   // clave declarada en columns para el eje Y
	YLabel     string `json:"y_label"` // etiqueta del dataset (opcional)
	Title      string `json:"title"`   // título del gráfico (opcional)
	Stacked    bool   `json:"stacked,omitempty"`
	ShowLegend bool   `json:"show_legend,omitempty"`
}

// ReportRunChart es el dataset ya procesado que consume Chart.js.
type ReportRunChart struct {
	Type     string                  `json:"type"`
	Title    string                  `json:"title,omitempty"`
	Labels   []string                `json:"labels"`
	Datasets []ReportRunChartDataset `json:"datasets"`
	Stacked  bool                    `json:"stacked,omitempty"`
}

type ReportRunChartDataset struct {
	Label string    `json:"label"`
	Data  []float64 `json:"data"`
}

// ReportDefinition es la definición completa de una plantilla de informe.
type ReportDefinition struct {
	Title      string         `json:"title"`
	SQL        string         `json:"sql"`
	Columns    []ReportColumn `json:"columns"`
	Totals     []string       `json:"totals"`
	Chart      *ReportChart   `json:"chart,omitempty"`
	Params     []ReportParam  `json:"params,omitempty"`
	MaxRows    int            `json:"max_rows,omitempty"`
	FormatHint string         `json:"format,omitempty"`
}

// ReportColumn es una columna del informe.
type ReportColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"` // text | number | money | date
}

// ClavesDeclaradas devuelve las claves que la plantilla declara, en orden.
func (d ReportDefinition) ClavesDeclaradas() []string {
	out := make([]string, 0, len(d.Columns))
	for _, c := range d.Columns {
		out = append(out, c.Key)
	}
	return out
}

// ReportParam es un parámetro que el usuario rellena al ejecutar.
type ReportParam struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // date | number | text | enum
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"` // solo para type=enum
	Required bool     `json:"required,omitempty"`
}

// Validate comprueba la definición sin ejecutarla.
func (d *ReportDefinition) Validate() error {
	if d.Chart != nil {
		switch d.Chart.Type {
		case "line", "bar", "pie", "doughnut":
		default:
			return fmt.Errorf("chart.type debe ser line, bar, pie o doughnut (recibido: %q)", d.Chart.Type)
		}
		if strings.TrimSpace(d.Chart.XKey) == "" || strings.TrimSpace(d.Chart.YKey) == "" {
			return fmt.Errorf("chart.x_key y chart.y_key son obligatorios cuando se define chart")
		}
		xFound, yFound := false, false
		for _, c := range d.Columns {
			if c.Key == d.Chart.XKey {
				xFound = true
			}
			if c.Key == d.Chart.YKey {
				yFound = true
			}
		}
		if !xFound {
			return fmt.Errorf("chart.x_key %q no coincide con ninguna key declarada en columns", d.Chart.XKey)
		}
		if !yFound {
			return fmt.Errorf("chart.y_key %q no coincide con ninguna key declarada en columns", d.Chart.YKey)
		}
	}
	if strings.TrimSpace(d.SQL) == "" {
		return fmt.Errorf("definition.sql es obligatorio")
	}
	if len(d.Columns) == 0 {
		return fmt.Errorf("definition.columns debe tener al menos una columna")
	}
	for _, c := range d.Columns {
		if strings.TrimSpace(c.Key) == "" {
			return fmt.Errorf("cada columna necesita un key")
		}
	}
	if d.MaxRows < 0 {
		return fmt.Errorf("max_rows no puede ser negativo")
	}
	if d.MaxRows > reportAbsoluteMaxRows {
		return fmt.Errorf("max_rows no puede superar %d", reportAbsoluteMaxRows)
	}
	vistos := map[string]bool{}
	for _, p := range d.Params {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("cada parámetro necesita un name")
		}
		if vistos[p.Name] {
			return fmt.Errorf("parámetro duplicado: %s", p.Name)
		}
		vistos[p.Name] = true
		switch p.Type {
		case "date", "number", "text", "enum":
		default:
			return fmt.Errorf("parámetro %s: tipo desconocido %q (use date, number, text o enum)", p.Name, p.Type)
		}
		if p.Type == "enum" && len(p.Options) == 0 {
			return fmt.Errorf("parámetro %s: un enum necesita su lista de opciones", p.Name)
		}
	}
	return nil
}

// EffectiveMaxRows es el tope real de filas de la plantilla.
func (d *ReportDefinition) EffectiveMaxRows() int {
	if d.MaxRows <= 0 {
		return reportDefaultMaxRows
	}
	if d.MaxRows > reportAbsoluteMaxRows {
		return reportAbsoluteMaxRows
	}
	return d.MaxRows
}

// reportBackend aísla cómo se ejecuta la consulta.
type reportBackend interface {
	Query(rows *sql.Rows) ([]string, [][]interface{}, error)
	Name() string
}

// sqliteReportBackend ejecuta la consulta sobre la base de la empresa.
type sqliteReportBackend struct{}

func (sqliteReportBackend) Name() string { return "sqlite" }

func (sqliteReportBackend) Query(rows *sql.Rows) ([]string, [][]interface{}, error) {
	cols, _ := rows.Columns()
	out := [][]interface{}{}
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
		}
		out = append(out, vals)
	}
	return cols, out, rows.Err()
}

// ReportRun es el resultado completo de ejecutar una plantilla.
type ReportRun struct {
	Title     string
	Headers   []string
	Keys      []string
	Rows      [][]interface{}
	Totals    map[string]interface{}
	Chart     *ReportRunChart
	Truncated bool
	Backend   string
	MaxRows   int
	Params    map[string]string

	Warnings []string
}

// RunReportDefinition ejecuta una plantilla con los valores dados.
func (s *Server) RunReportDefinition(r *http.Request, def *ReportDefinition, values map[string]string) (*ReportRun, error) {
	if err := def.Validate(); err != nil {
		return nil, err
	}

	resolved := map[string]string{}
	for _, p := range def.Params {
		raw := strings.TrimSpace(values[p.Name])
		if raw == "" {
			raw = p.Default
		}
		lit, err := reportLiteral(p, raw)
		if err != nil {
			return nil, err
		}
		resolved[p.Name] = lit
	}

	sqlText := def.SQL
	for name := range resolved {
		if !strings.Contains(sqlText, ":"+name) {
			return nil, fmt.Errorf("el parámetro '%s' está declarado pero no se usa en la consulta", name)
		}
	}
	for name, lit := range resolved {
		sqlText = replaceParam(sqlText, name, lit)
	}

	// Frontera ÚNICA (S-4, auditoría 2026-11-22; pendiente cerrado por el
	// integrador tras el cierre del subsistema IA): el motor clásico y el
	// camino de informes de IA ejecutan el mismo SQL contra la misma
	// conexión, así que la comprobación no puede ser distinta según quién
	// la llamó. Antes este punto solo exigía SELECT (verbos): un informe
	// clásico autorizable leía user_login, ai_providers o audit_log tan
	// fácil como el agujero que se cerró en el otro camino. El alcance es
	// el del EJECUTOR (master/admin = IT, paridad con la interfaz); la
	// lista negra de identidad/auditoría/series es dura para todos.
	sqlSeguro, err := validarSQLInformes(sqlText, s.aiDeptDelEjecutor(s.sessionFromRequest(r)), def.EffectiveMaxRows())
	if err != nil {
		return nil, err
	}
	clean := sqlSeguro

	rows, err := s.dbFor(r).Query(clean)
	if err != nil {
		return nil, fmt.Errorf("error de SQL: %v", err)
	}
	defer rows.Close()

	var backend reportBackend = sqliteReportBackend{}
	keys, data, err := backend.Query(rows)
	if err != nil {
		return nil, err
	}

	run := &ReportRun{
		Title:   def.Title,
		Keys:    keys,
		Rows:    data,
		Totals:  map[string]interface{}{},
		Backend: backend.Name(),
		MaxRows: def.EffectiveMaxRows(),
		Params:  resolved,
	}
	run.Truncated = len(data) >= run.MaxRows

	for _, c := range def.Columns {
		label := c.Label
		if label == "" {
			label = c.Key
		}
		run.Headers = append(run.Headers, label)
	}
	if len(run.Headers) == 0 {
		run.Headers = keys
	}

	claves := def.ClavesDeclaradas()
	if len(claves) != len(keys) {
		run.Warnings = append(run.Warnings, fmt.Sprintf(
			"la definición declara %d columnas pero la consulta devuelve %d: no se han calculado los totales",
			len(claves), len(keys)))
	}
	sums := map[string]float64{}
	for _, row := range data {
		for i, v := range row {
			f := valorNumerico(v)
			if f == nil {
				continue
			}
			if i < len(claves) && claves[i] != "" {
				sums[claves[i]] += *f
			}
			if i < len(keys) && keys[i] != "" && keys[i] != claves[i] {
				sums[keys[i]] += *f
			}
		}
	}

	// Existencia de cada nombre en el resultado: keys (aliases del SELECT) y
	// claves declaradas. Mismo tratamiento por causa que en		// executeReportDefinition (ai_reports.go): resultado vacío ⇒ total 0 sin
	// aviso; columna existente sin valores numéricos ⇒ aviso honesto; key
	// ajena al resultado ⇒ aviso de desajuste. El aviso antiguo decía "no
	// aparece" para todas y mentía en las dos primeras.
	enResultado := map[string]bool{}
	for _, k := range claves {
		if k != "" {
			enResultado[k] = true
		}
	}
	for _, k := range keys {
		if k != "" {
			enResultado[k] = true
		}
	}
	for _, key := range def.Totals {
		if v, ok := sums[key]; ok {
			run.Totals[key] = round2(v)
			continue
		}
		if enResultado[key] {
			if len(data) == 0 {
				run.Totals[key] = 0
				continue
			}
			run.Warnings = append(run.Warnings, fmt.Sprintf(
				"no se pudo calcular el total de la columna %q: existe en el resultado pero ninguno de sus valores es numérico", key))
			continue
		}
		run.Warnings = append(run.Warnings, fmt.Sprintf(
			"no se pudo calcular el total de la columna %q: no aparece en el resultado de la consulta", key))
	}

	if def.Chart != nil {
		if chart, err := buildReportChart(def.Chart, claves, keys, data); err == nil {
			run.Chart = chart
		} else {
			run.Warnings = append(run.Warnings, fmt.Sprintf("no se pudo generar el gráfico: %v", err))
		}
	}

	return run, nil
}

// buildReportChart arma el dataset para Chart.js a partir de las filas.
func buildReportChart(cfg *ReportChart, claves, keys []string, data [][]interface{}) (*ReportRunChart, error) {
	// Encontrar los índices de x_key e y_key en las claves/keys devueltos.
	xIdx, yIdx := -1, -1
	for i, k := range keys {
		if k == cfg.XKey {
			xIdx = i
		}
		if k == cfg.YKey {
			yIdx = i
		}
	}
	if xIdx == -1 {
		// Buscar por clave declarada
		for i, c := range claves {
			if c == cfg.XKey {
				xIdx = i
				break
			}
		}
	}
	if yIdx == -1 {
		for i, c := range claves {
			if c == cfg.YKey {
				yIdx = i
				break
			}
		}
	}
	if xIdx == -1 || yIdx == -1 {
		return nil, fmt.Errorf("no se encontraron las columnas del gráfico en el resultado")
	}

	labels := make([]string, 0, len(data))
	yData := make([]float64, 0, len(data))
	for _, r := range data {
		if xIdx >= len(r) || yIdx >= len(r) {
			continue
		}
		label := fmt.Sprintf("%v", r[xIdx])
		labels = append(labels, label)
		if f := valorNumerico(r[yIdx]); f != nil {
			yData = append(yData, *f)
		} else {
			yData = append(yData, 0)
		}
	}

	yLabel := cfg.YLabel
	if yLabel == "" {
		yLabel = cfg.YKey
	}

	return &ReportRunChart{
		Type:   cfg.Type,
		Title:  cfg.Title,
		Labels: labels,
		Datasets: []ReportRunChartDataset{
			{Label: yLabel, Data: yData},
		},
		Stacked: cfg.Stacked,
	}, nil
}

// reportLiteral valida un valor según el tipo del parámetro y devuelve el
// literal SQL seguro para sustituir.
func reportLiteral(p ReportParam, raw string) (string, error) {
	if len(raw) > reportParamMaxLen {
		return "", fmt.Errorf("el valor del parámetro '%s' es demasiado largo", p.Name)
	}
	if raw == "" {
		if p.Required {
			return "", fmt.Errorf("el parámetro '%s' es obligatorio", p.Name)
		}
		return "NULL", nil
	}
	switch p.Type {
	case "date":
		if !aiValidDate(raw) {
			return "", fmt.Errorf("el parámetro '%s' debe ser una fecha AAAA-MM-DD", p.Name)
		}
		return "'" + raw + "'", nil
	case "number":
		f, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64)
		if err != nil {
			return "", fmt.Errorf("el parámetro '%s' debe ser numérico", p.Name)
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case "enum":
		for _, o := range p.Options {
			if o == raw {
				return "'" + strings.ReplaceAll(o, "'", "''") + "'", nil
			}
		}
		return "", fmt.Errorf("el valor '%s' no está entre las opciones de '%s'", raw, p.Name)
	default: // text
		return "'" + strings.ReplaceAll(raw, "'", "''") + "'", nil
	}
}

// replaceParam sustituye :nombre por su literal.
func replaceParam(sqlText, name, literal string) string {
	target := ":" + name
	var b strings.Builder
	for i := 0; i < len(sqlText); {
		if sqlText[i] == ':' && strings.HasPrefix(sqlText[i:], target) {
			end := i + len(target)
			if end < len(sqlText) && isParamChar(sqlText[end]) {
				b.WriteString(sqlText[i:end])
				i = end
				continue
			}
			b.WriteString(literal)
			i = end
			continue
		}
		b.WriteByte(sqlText[i])
		i++
	}
	return b.String()
}

func isParamChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ReportCSV escribe el resultado como CSV.
func ReportCSV(run *ReportRun) ([]byte, error) {
	var b strings.Builder
	b.WriteString("\xEF\xBB\xBF") // BOM UTF-8
	w := csv.NewWriter(&b)

	encabezado := make([]string, len(run.Headers))
	copy(encabezado, run.Headers)
	if err := w.Write(encabezado); err != nil {
		return nil, err
	}

	idxTotal := map[string]int{}
	for i, k := range run.Keys {
		idxTotal[k] = i
	}

	for _, row := range run.Rows {
		out := make([]string, len(row))
		for i, v := range row {
			out[i] = formatReportValue(v)
		}
		if err := w.Write(out); err != nil {
			return nil, err
		}
	}

	if len(run.Totals) > 0 {
		total := make([]string, len(run.Headers))
		for i := range total {
			total[i] = ""
		}
		total[0] = "TOTALES"
		for k, v := range run.Totals {
			if i, ok := idxTotal[k]; ok && i < len(total) {
				total[i] = formatReportValue(v)
			}
		}
		if err := w.Write(total); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return []byte(b.String()), w.Error()
}

// formatReportValue deja el CSV legible.
func formatReportValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(t, 'f', 2, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
