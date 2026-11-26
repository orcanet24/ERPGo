package web

// ---------- AI Business Analyst (port of dashboard AI_Setting + AI_Controller) ----------

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Multi-provider support (ai_providers table, base_url, activation and the
// chat-completions client) lives in ai_providers.go.

// aiSetting mirrors the single row of ai_setting.
type aiSetting struct {
	SettingID      int64
	APIKey         string
	Model          string
	Temperature    float64
	MaxTokens      int
	PromptTemplate string
}

// aiChartData is the Chart.js payload (arrays aligned with Labels).
type aiChartData struct {
	Labels         []string  `json:"labels"`
	Sales          []float64 `json:"sales"`
	Purchases      []float64 `json:"purchases"`
	Stock          []float64 `json:"stock"`
	SalesReturn    []float64 `json:"sales_return"`
	SupplierReturn []float64 `json:"supplier_return"`
}

// aiTableData is the report table payload ({headers, rows}).
type aiTableData struct {
	Headers []interface{}   `json:"headers"`
	Rows    [][]interface{} `json:"rows"`
}

// aiEnvelope is the JSON object the model is asked to return.
type aiEnvelope struct {
	Response  string          `json:"response"`
	ChartData json.RawMessage `json:"chart_data"`
	TableData json.RawMessage `json:"table_data"`
}

func (s *Server) registerAIRoutes() {
	s.mux.HandleFunc("GET /ai/settings", s.aiSettingsForm)
	s.mux.HandleFunc("POST /ai/settings/save", s.aiSettingsSave)
	s.mux.HandleFunc("GET /ai/chat", s.aiChatForm)
	s.mux.HandleFunc("POST /ai/analyze", s.aiAnalyze)
	s.mux.HandleFunc("GET /ai/report-types", s.aiReportTypeList)
	s.mux.HandleFunc("POST /ai/report-types/add", s.aiReportTypeAdd)
	s.mux.HandleFunc("POST /ai/report-types/delete", s.aiReportTypeDelete)
	// Providers (multi-vendor, OpenAI-compatible)
	s.mux.HandleFunc("GET /ai/providers", s.aiProviderList)
	// Lista de modelos para el formulario: catálogo curado por vendor, y para
	// OpenAI/Anthropic cruzado con la lista viva del proveedor. Nunca devuelve
	// la URL de un proveedor.
	s.mux.HandleFunc("POST /ai/models", s.aiModelsList)
	s.mux.HandleFunc("POST /ai/providers/save", s.aiProviderSave)
	s.mux.HandleFunc("POST /ai/providers/delete/{id}", s.aiProviderDelete)
	s.mux.HandleFunc("POST /ai/providers/activate/{id}", s.aiProviderActivate)
	// Metering de inferencia (ai_usage): el número con el que se factura.
	s.mux.HandleFunc("GET /ai/usage", s.aiUsageReport)
}

// aiUsageReport sirve el consumo de inferencia de la empresa como JSON.
//
// Es la fuente del panel de consumo y el número con el que el operador factura
// los paquetes de inferencia. `?days=N` acota el periodo (por defecto 30).
func (s *Server) aiUsageReport(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("days")))
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	totals := s.aiUsageTotalsFor(s.dbFor(r), days)
	writeJSON(w, map[string]interface{}{
		"status":      "success",
		"days":        days,
		"requests":    totals.Requests,
		"prompt":      totals.PromptTokens,
		"completion":  totals.CompletionToks,
		"total":       totals.TotalTokens,
		"cost_usd":    totals.CostUSD,
		"by_day":      totals.ByDay,
		"by_provider": totals.ByProvider,
	})
}

// aiJSONError writes {"status":"error","message":...} (the chat UI keys on status).
func aiJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"status":"error","message":%q}`, msg)
}

// ---------- Settings ----------

func (s *Server) loadAISetting(sess *SessionData) aiSetting {
	var a aiSetting
	err := s.dbForSession(sess).QueryRow(`SELECT setting_id, IFNULL(api_key,''), IFNULL(model,''),
		IFNULL(temperature,0.7), IFNULL(max_tokens,400), IFNULL(prompt_template,'')
		FROM ai_setting ORDER BY setting_id LIMIT 1`).
		Scan(&a.SettingID, &a.APIKey, &a.Model, &a.Temperature, &a.MaxTokens, &a.PromptTemplate)
	if err != nil {
		return aiSetting{Model: "gpt-4o-mini", Temperature: 0.7, MaxTokens: 400}
	}
	return a
}

// aiSettingsStatus is what /ai/settings shows on top of the provider list.
type aiSettingsStatus struct {
	Providers  []aiProvider
	Active     aiProvider
	HasActive  bool // an active row of ai_providers exists
	Usable     bool // the effective provider can be called right now
	LegacyOnly bool // no active row: the legacy ai_setting key is in use
}

func (s *Server) aiSettingsStatus(sess *SessionData) aiSettingsStatus {
	// Los TRES oficiales, con la fila que le corresponda a cada uno: la tabla
	// completa con sus filas heredadas era «el listado que nadie pidió».
	st := aiSettingsStatus{Providers: s.aiProveedoresOficiales(sess)}
	_, st.HasActive = s.loadActiveProviderRow(sess)
	st.Active, st.Usable = s.loadActiveProvider(sess)
	st.LegacyOnly = !st.HasActive && strings.TrimSpace(s.loadAISetting(sess).APIKey) != ""
	return st
}

func (s *Server) aiSettingsForm(w http.ResponseWriter, r *http.Request) {
	st := s.aiSettingsStatus(s.sessionFromRequest(r))
	s.render(w, r, "ai_setting.html", map[string]interface{}{
		"Title":        "Configuración IA",
		"AI":           s.loadAISetting(s.sessionFromRequest(r)),
		"Providers":    st.Providers,
		"Active":       st.Active,
		"HasActive":    st.HasActive,
		"ActiveUsable": st.Usable,
		"LegacyActive": st.LegacyOnly,
	})
}

func (s *Server) aiSettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		aiJSONError(w, "Formulario inválido", 400)
		return
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	model := strings.TrimSpace(r.FormValue("model"))
	promptTemplate := strings.TrimSpace(r.FormValue("prompt_template"))
	if apiKey == "" {
		aiJSONError(w, "La API key es obligatoria", 400)
		return
	}
	if model == "" {
		aiJSONError(w, "El modelo es obligatorio", 400)
		return
	}

	temperature := 0.7
	if v := strings.TrimSpace(r.FormValue("temperature")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			aiJSONError(w, "La temperatura debe ser un número entre 0 y 2", 400)
			return
		}
		if f < 0 || f > 2 {
			f = 0.7
		}
		temperature = f
	}

	maxTokens := 400
	if v := strings.TrimSpace(r.FormValue("max_tokens")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			aiJSONError(w, "Max tokens debe ser un número entero mayor que cero", 400)
			return
		}
		maxTokens = n
	}

	var rows int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM ai_setting`).Scan(&rows)
	if rows == 0 {
		_, err := s.dbFor(r).Exec(`INSERT INTO ai_setting (setting_id, api_key, model, temperature, max_tokens, prompt_template)
			VALUES (1,?,?,?,?,?)`, apiKey, model, temperature, maxTokens, promptTemplate)
		if err != nil {
			aiJSONError(w, "No se pudo crear la configuración: "+err.Error(), 500)
			return
		}
	} else {
		_, err := s.dbFor(r).Exec(`UPDATE ai_setting SET api_key=?, model=?, temperature=?, max_tokens=?, prompt_template=?`,
			apiKey, model, temperature, maxTokens, promptTemplate)
		if err != nil {
			aiJSONError(w, "No se pudo guardar la configuración: "+err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]interface{}{"status": "success", "msg": "Configuración guardada",
		"redirect": saveRedirect(r, "/ai/settings")})
}

// ---------- Chat page ----------

func (s *Server) aiChatForm(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	dateFrom := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	// Discreet badge with the provider/model that will answer the questions.
	badge := ""
	if p, ok := s.loadActiveProvider(s.sessionFromRequest(r)); ok {
		badge = strings.TrimSpace(p.Name)
		if m := strings.TrimSpace(p.Model); m != "" {
			badge = strings.TrimSpace(badge + " · " + m)
		}
	}
	s.render(w, r, "ai_chat_prompt.html", map[string]interface{}{
		"Title":      "AI Business Analyst",
		"DateFrom":   dateFrom,
		"DateTo":     today(),
		"AIProvider": badge,
	})
}

// ---------- Report types ----------

func (s *Server) aiReportTypeList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT id, name FROM ai_report_types ORDER BY name`)
	if err != nil {
		aiJSONError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type reportType struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	list := []reportType{}
	for rows.Next() {
		var t reportType
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			continue
		}
		list = append(list, t)
	}
	writeJSON(w, list)
}

func (s *Server) aiReportTypeAdd(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		aiJSONError(w, "El nombre es obligatorio", 400)
		return
	}
	var duplicated int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM ai_report_types WHERE name = ?`, name).Scan(&duplicated)
	if duplicated > 0 {
		aiJSONError(w, "Ese tipo de reporte ya existe", 400)
		return
	}
	if _, err := s.dbFor(r).Exec(`INSERT INTO ai_report_types (name) VALUES (?)`, name); err != nil {
		aiJSONError(w, "No se pudo guardar el tipo de reporte: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "success"})
}

func (s *Server) aiReportTypeDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		aiJSONError(w, "No se especificó el tipo de reporte", 400)
		return
	}
	res, err := s.dbFor(r).Exec(`DELETE FROM ai_report_types WHERE name = ?`, name)
	if err != nil {
		aiJSONError(w, "No se pudo eliminar el tipo de reporte: "+err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		aiJSONError(w, "No se encontró el tipo de reporte", 404)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "success"})
}

// ---------- Analysis ----------

// aiAnalyze replicates AI_Controller::bdtask_get_ai_analysis: it aggregates the
// selected report types for the date range, asks the active AI provider for a
// JSON envelope ({response, chart_data, table_data}) and answers the chat UI.
func (s *Server) aiAnalyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		aiJSONError(w, "Formulario inválido", 400)
		return
	}

	reportTypes := aiUniqueLower(append(r.Form["report_type[]"], r.Form["report_type"]...))
	dateFrom := strings.TrimSpace(r.FormValue("date_from"))
	dateTo := strings.TrimSpace(r.FormValue("date_to"))
	if dateFrom == "" {
		dateFrom = aiFirstOfMonth()
	}
	if dateTo == "" {
		dateTo = today()
	}
	if !aiValidDate(dateFrom) || !aiValidDate(dateTo) {
		aiJSONError(w, "Rango de fechas inválido (use AAAA-MM-DD)", 400)
		return
	}
	if dateFrom > dateTo {
		dateFrom, dateTo = dateTo, dateFrom
	}
	userInput := strings.TrimSpace(r.FormValue("user_input"))

	// Resolve the provider up front so a missing configuration fails fast
	// (before aggregating anything from the database).
	sess := s.sessionFromRequest(r)
	provider, hasProvider := s.loadActiveProvider(sess)
	if !hasProvider {
		aiJSONError(w, s.aiNoProviderReason(sess), 503)
		return
	}

	includeChart := aiContains(reportTypes, "chart")
	includeTable := aiContains(reportTypes, "table_summary")

	dataTypes := []string{}
	for _, t := range []string{"sales", "purchase", "stock", "sales_return", "supplier_return"} {
		if aiContains(reportTypes, t) {
			dataTypes = append(dataTypes, t)
		}
	}
	if len(dataTypes) == 0 {
		// No dataset selected: fall back to the core trio so the answer is useful.
		dataTypes = []string{"sales", "purchase", "stock"}
	}

	currency := s.settingsFor(r).Web.Currency
	if currency == "" {
		currency = "$"
	}

	chartRaw := map[string]map[string]float64{
		"sales": {}, "purchases": {}, "stock": {}, "sales_return": {}, "supplier_return": {},
	}
	labelSet := map[string]bool{}
	var summary strings.Builder

	for _, reportType := range dataTypes {
		var (
			text   string
			series map[string]float64
			err    error
		)
		switch reportType {
		case "sales":
			text, series, err = s.aiSalesSummary(s.sessionFromRequest(r), dateFrom, dateTo, currency)
		case "purchase":
			text, series, err = s.aiPurchaseSummary(s.sessionFromRequest(r), dateFrom, dateTo, currency)
		case "stock":
			text, series, err = s.aiStockSummary(s.sessionFromRequest(r), dateTo, currency)
		case "sales_return":
			text, series, err = s.aiReturnSummary(s.sessionFromRequest(r), dateFrom, dateTo, currency, 1, "Ventas", "Cliente")
		case "supplier_return":
			text, series, err = s.aiReturnSummary(s.sessionFromRequest(r), dateFrom, dateTo, currency, 2, "Compras", "Proveedor")
		}
		if err != nil {
			aiJSONError(w, err.Error(), 500)
			return
		}
		summary.WriteString(text)
		for d := range series {
			labelSet[d] = true
		}
		key := aiSeriesKey(reportType)
		for d, v := range series {
			chartRaw[key][d] += v
		}
	}

	if includeTable {
		summary.WriteString("✅ Tabla solicitada: responde además con table_data (headers y rows) cubriendo ventas, compras y devoluciones del rango.\n\n")
	}
	if len(dataTypes) == len([]string{"sales", "purchase", "stock"}) && len(reportTypes) == 0 {
		summary.WriteString("Note: no report type was selected, so a default sales/purchase/stock summary is included.\n\n")
	}

	labels := make([]string, 0, len(labelSet))
	for d := range labelSet {
		labels = append(labels, d)
	}
	sort.Strings(labels)

	var tableRows [][]interface{}
	if includeTable {
		var err error
		tableRows, err = s.aiTableSummaryRows(s.sessionFromRequest(r), dateFrom, dateTo)
		if err != nil {
			aiJSONError(w, err.Error(), 500)
			return
		}
	}

	instruction := aiInstruction(includeChart, includeTable, labels)
	finalPrompt := instruction + "\n\n" + userInput + "\n\n" + summary.String()

	response, usage, err := callChatCompletion(provider, finalPrompt)
	// El consumo se registra tanto si la llamada funcionó como si falló: una
	// llamada que consume tokens y falla también cuesta dinero.
	s.recordAIUsage(s.dbFor(r), aiUsageEvent{
		ProviderID: provider.ID,
		Vendor:     provider.Vendor,
		Model:      usage.Model,
		Source:     "informe",
		Prompt:     usage.Prompt,
		Completion: usage.Completion,
		OK:         err == nil,
	})
	if err != nil {
		aiJSONError(w, err.Error(), 502)
		return
	}

	// Server-side series are the fallback when the model does not return JSON.
	chartData := aiChartDataFromRaw(chartRaw, labels)
	tableData := &aiTableData{
		Headers: []interface{}{"Fecha", "Tipo", "Monto", "Referencia"},
		Rows:    tableRows,
	}
	resultResponse := response
	if env, ok := parseAIEnvelope(response); ok {
		if strings.TrimSpace(env.Response) != "" {
			resultResponse = env.Response
		}
		if includeChart && len(env.ChartData) > 0 {
			var cd aiChartData
			if err := json.Unmarshal(env.ChartData, &cd); err == nil && len(cd.Labels) > 0 {
				chartData = &cd
			}
		}
		if includeTable && len(env.TableData) > 0 {
			var td aiTableData
			if err := json.Unmarshal(env.TableData, &td); err == nil && len(td.Headers) > 0 {
				tableData = &td
			}
		}
	}

	var chartOut, tableOut interface{}
	if includeChart {
		chartOut = chartData
	}
	if includeTable {
		tableOut = tableData
	}
	writeJSON(w, map[string]interface{}{
		"status":     "success",
		"response":   resultResponse,
		"chart_data": chartOut,
		"table_data": tableOut,
	})
}

func aiInstruction(includeChart, includeTable bool, labels []string) string {
	var sb strings.Builder
	sb.WriteString("You are a business analyst. Analyze the following business data and answer ONLY with a single JSON object (no prose around it; a ```json code fence is allowed). The JSON must look like:\n")
	sb.WriteString(`{"response": "<markdown insights and recommendations>"`)
	if includeChart {
		sb.WriteString(`, "chart_data": {"labels": ["..."], "sales": [...], "purchases": [...], "stock": [...], "sales_return": [...], "supplier_return": [...]}`)
	}
	if includeTable {
		sb.WriteString(`, "table_data": {"headers": ["..."], "rows": [["..."]]}`)
	}
	sb.WriteString("}\n")
	if includeChart {
		sb.WriteString("Use exactly these labels for chart_data (ascending order): " + mustJSON(labels) + "\n")
		sb.WriteString("Every chart_data array must have the same length as labels; use 0 when a series has no value on that date.\n")
	}
	if includeTable {
		sb.WriteString("table_data must contain one row per record (date, kind, amount, reference).\n")
	}
	if !includeChart && !includeTable {
		sb.WriteString("No chart or table is required, only the response field with markdown insights.\n")
	}
	return sb.String()
}

// parseAIEnvelope extracts the first JSON object of a model answer (it may be
// wrapped in ```json fences or surrounded by prose).
func parseAIEnvelope(raw string) (*aiEnvelope, bool) {
	start := strings.IndexByte(raw, '{')
	end := strings.LastIndexByte(raw, '}')
	if start < 0 || end <= start {
		return nil, false
	}
	var env aiEnvelope
	if err := json.Unmarshal([]byte(raw[start:end+1]), &env); err != nil {
		return nil, false
	}
	return &env, true
}

// ---------- SQL summaries (one per report type) ----------

func (s *Server) aiSalesSummary(sess *SessionData, from, to, currency string) (string, map[string]float64, error) {
	d := s.sqlDateYMD("i.date")
	rows, err := s.dbForSession(sess).Query(`
		SELECT `+d+`, IFNULL(ci.customer_name,''), IFNULL(pi.product_name,''),
			COUNT(DISTINCT i.id), IFNULL(SUM(id.quantity),0), IFNULL(SUM(id.total_price),0)
		FROM invoice i
		JOIN invoice_details id ON i.id = id.invoice_id
		JOIN product_information pi ON pi.product_id = id.product_id
		JOIN customer_information ci ON ci.customer_id = i.customer_id
		WHERE DATE(i.date) >= ? AND DATE(i.date) <= ?
		GROUP BY `+d+`, ci.customer_name, pi.product_name
		ORDER BY `+d+` ASC
		LIMIT 250`, from, to)
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo consultar ventas: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 **Resumen de Ventas** (%s al %s):\n", from, to))
	series := map[string]float64{}
	found := 0
	for rows.Next() {
		var day, customer, product sql.NullString
		var invoices int64
		var qty, amount float64
		if err := rows.Scan(&day, &customer, &product, &invoices, &qty, &amount); err != nil {
			return "", nil, fmt.Errorf("no se pudo leer el resumen de ventas: %w", err)
		}
		fmt.Fprintf(&sb, "Fecha: %s, Facturas: %d, Cliente: %s, Producto: %s, Cantidad: %.2f, Monto: %.2f %s\n",
			day.String, invoices, customer.String, product.String, qty, amount, currency)
		series[day.String] += amount
		found++
	}
	if found == 0 {
		sb.WriteString("No sales data found.\n")
	}
	sb.WriteString("\n")
	return sb.String(), series, nil
}

func (s *Server) aiPurchaseSummary(sess *SessionData, from, to, currency string) (string, map[string]float64, error) {
	d := s.sqlDateYMD("pp.purchase_date")
	rows, err := s.dbForSession(sess).Query(`
		SELECT `+d+`, IFNULL(pi.product_name,''),
			COUNT(DISTINCT pp.purchase_id), IFNULL(SUM(ppd.quantity),0), IFNULL(SUM(ppd.total_amount),0)
		FROM product_purchase pp
		LEFT JOIN product_purchase_details ppd ON pp.purchase_id = ppd.purchase_id
		LEFT JOIN product_information pi ON pi.product_id = ppd.product_id
		WHERE DATE(pp.purchase_date) >= ? AND DATE(pp.purchase_date) <= ? AND pp.status = 1
		GROUP BY `+d+`, pi.product_name
		ORDER BY `+d+` ASC
		LIMIT 250`, from, to)
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo consultar compras: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 **Resumen de Compras** (%s al %s):\n", from, to))
	series := map[string]float64{}
	found := 0
	for rows.Next() {
		var day, product sql.NullString
		var purchases int64
		var qty, amount float64
		if err := rows.Scan(&day, &product, &purchases, &qty, &amount); err != nil {
			return "", nil, fmt.Errorf("no se pudo leer el resumen de compras: %w", err)
		}
		fmt.Fprintf(&sb, "Fecha: %s, Compras: %d, Producto: %s, Cantidad: %.2f, Costo: %.2f %s\n",
			day.String, purchases, product.String, qty, amount, currency)
		series[day.String] += amount
		found++
	}
	if found == 0 {
		sb.WriteString("No purchase data found.\n")
	}
	sb.WriteString("\n")
	return sb.String(), series, nil
}

// aiReturnSummary reads product_return (usablity 1 = sales return, 2 = supplier return).
func (s *Server) aiReturnSummary(sess *SessionData, from, to, currency string, usability int, label, party string) (string, map[string]float64, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if usability == 1 {
		rows, err = s.dbForSession(sess).Query(`
			SELECT a.date_return, IFNULL(b.customer_name,''), IFNULL(a.net_total_amount,0)
			FROM product_return a
			LEFT JOIN customer_information b ON b.customer_id = a.customer_id
			WHERE a.usablity = 1 AND a.date_return >= ? AND a.date_return <= ?
			ORDER BY a.date_return DESC LIMIT 150`, from, to)
	} else {
		rows, err = s.dbForSession(sess).Query(`
			SELECT a.date_return, IFNULL(b.supplier_name,''), IFNULL(a.net_total_amount,0)
			FROM product_return a
			LEFT JOIN supplier_information b ON b.supplier_id = a.supplier_id
			WHERE a.usablity = 2 AND a.date_return >= ? AND a.date_return <= ?
			ORDER BY a.date_return DESC LIMIT 150`, from, to)
	}
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo consultar devoluciones: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("↩️ **Resumen de Devoluciones de %s** (%s al %s):\n", label, from, to))
	series := map[string]float64{}
	found := 0
	for rows.Next() {
		var day, name sql.NullString
		var amount float64
		if err := rows.Scan(&day, &name, &amount); err != nil {
			return "", nil, fmt.Errorf("no se pudo leer el resumen de devoluciones: %w", err)
		}
		fmt.Fprintf(&sb, "Fecha: %s, %s: %s, Monto devuelto: %.2f %s\n", day.String, party, name.String, amount, currency)
		series[day.String] += amount
		found++
	}
	if found == 0 {
		sb.WriteString("No return data found.\n")
	}
	sb.WriteString("\n")
	return sb.String(), series, nil
}

func (s *Server) aiStockSummary(sess *SessionData, asOf, currency string) (string, map[string]float64, error) {
	rows, err := s.dbForSession(sess).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(a.price,'0'),
			IFNULL((SELECT AVG(d.rate) FROM product_purchase_details d WHERE d.product_id = a.product_id),0)
		FROM product_information a
		WHERE a.status = 1
		ORDER BY a.product_name
		LIMIT 300`)
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo consultar el inventario: %w", err)
	}
	defer rows.Close()

	r := s.requestForSession(sess)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 **Resumen de Inventario** (al %s):\n", asOf))
	totalStock := 0.0
	found := 0
	for rows.Next() {
		var pid, name, model, price sql.NullString
		var purchasePrice float64
		if err := rows.Scan(&pid, &name, &model, &price, &purchasePrice); err != nil {
			return "", nil, fmt.Errorf("no se pudo leer el inventario: %w", err)
		}
		pidInt := idInternoProducto(s.dbFor(r), pid.String)
		stock := s.stockForProduct(r, pidInt)
		fmt.Fprintf(&sb, "Producto: %s (%s), Existencia: %.2f, Precio compra: %.2f %s, Precio venta: %s %s\n",
			name.String, model.String, stock, purchasePrice, currency, price.String, currency)
		totalStock += stock
		found++
	}
	if found == 0 {
		sb.WriteString("No stock data found.\n")
	}
	sb.WriteString("\n")
	return sb.String(), map[string]float64{asOf: totalStock}, nil
}

// aiTableSummaryRows reproduces Report_model::get_table_summary.
func (s *Server) aiTableSummaryRows(sess *SessionData, from, to string) ([][]interface{}, error) {
	dInv := s.sqlDateYMD("`date`")
	dPur := s.sqlDateYMD("purchase_date")
	rows, err := s.dbForSession(sess).Query(`
		SELECT `+dInv+`, 'Venta', `+s.sqlCastText("total_amount")+`, `+s.sqlCastText("`invoice`")+`
		FROM invoice WHERE status = 1 AND DATE(`+"`date`"+`) >= ? AND DATE(`+"`date`"+`) <= ?
		UNION ALL
		SELECT `+dPur+`, 'Compra', `+s.sqlCastText("grand_total_amount")+`, `+s.sqlCastText("purchase_id")+`
		FROM product_purchase WHERE status = 1 AND DATE(purchase_date) >= ? AND DATE(purchase_date) <= ?
		UNION ALL
		SELECT date_return, 'Devolución Venta', `+s.sqlCastText("net_total_amount")+`, return_id
		FROM product_return WHERE usablity = 1 AND date_return >= ? AND date_return <= ?
		UNION ALL
		SELECT date_return, 'Devolución Compra', `+s.sqlCastText("net_total_amount")+`, return_id
		FROM product_return WHERE usablity = 2 AND date_return >= ? AND date_return <= ?
		ORDER BY 1 ASC
		LIMIT 300`, from, to, from, to, from, to, from, to)
	if err != nil {
		return nil, fmt.Errorf("no se pudo construir la tabla resumen: %w", err)
	}
	defer rows.Close()

	out := [][]interface{}{}
	for rows.Next() {
		var day, kind, amount, ref sql.NullString
		if err := rows.Scan(&day, &kind, &amount, &ref); err != nil {
			return nil, fmt.Errorf("no se pudo leer la tabla resumen: %w", err)
		}
		out = append(out, []interface{}{day.String, kind.String, amount.String, ref.String})
	}
	return out, nil
}

// ---------- small helpers ----------

func aiContains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func aiUniqueLower(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func aiSeriesKey(reportType string) string {
	switch reportType {
	case "sales":
		return "sales"
	case "purchase":
		return "purchases"
	case "stock":
		return "stock"
	case "sales_return":
		return "sales_return"
	case "supplier_return":
		return "supplier_return"
	}
	return reportType
}

func aiChartDataFromRaw(raw map[string]map[string]float64, labels []string) *aiChartData {
	cd := &aiChartData{Labels: labels}
	cd.Sales = make([]float64, len(labels))
	cd.Purchases = make([]float64, len(labels))
	cd.Stock = make([]float64, len(labels))
	cd.SalesReturn = make([]float64, len(labels))
	cd.SupplierReturn = make([]float64, len(labels))
	for i, label := range labels {
		cd.Sales[i] = raw["sales"][label]
		cd.Purchases[i] = raw["purchases"][label]
		cd.Stock[i] = raw["stock"][label]
		cd.SalesReturn[i] = raw["sales_return"][label]
		cd.SupplierReturn[i] = raw["supplier_return"][label]
	}
	return cd
}

func aiValidDate(value string) bool {
	if len(value) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func aiFirstOfMonth() string {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}
