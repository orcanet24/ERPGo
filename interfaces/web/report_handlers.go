package web

// report_handlers.go — Ejecución y exportación de informes por plantilla.
//
// Lo que hace posible que un usuario sin perfil técnico defina sus propios
// informes es esta capa: la plantilla se guarda una vez y a partir de ahí solo
// se rellenan los parámetros. Un informe de "ventas del mes" no se vuelve a
// escribir cada mes.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (s *Server) registerReportEngineRoutes() {
	s.mux.HandleFunc("GET /reports/engine", s.reportEngineList)
	s.mux.HandleFunc("GET /reports/engine/templates", s.reportEngineTemplates)
	s.mux.HandleFunc("GET /reports/engine/params/{id}", s.reportTemplateParams)
	s.mux.HandleFunc("POST /reports/engine/run/{id}", s.reportEngineRun)
	s.mux.HandleFunc("POST /reports/engine/export/{id}", s.reportEngineExport)
	s.mux.HandleFunc("POST /reports/engine/pdf/{id}", s.reportEnginePDF)
}

// loadReportDefinition carga y deserializa la plantilla por id.
func (s *Server) loadReportDefinition(r *http.Request, id int64) (*ReportDefinition, error) {
	var raw string
	err := s.dbFor(r).QueryRow(`SELECT definition FROM report_template WHERE id = ? AND status = 1`, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("la plantilla no existe")
		}
		return nil, fmt.Errorf("no se pudo leer la plantilla: %w", err)
	}
	var def ReportDefinition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return nil, fmt.Errorf("la plantilla está dañada: %v", err)
	}
	return &def, nil
}

// reportEngineList muestra el catálogo de plantillas con sus parámetros.
func (s *Server) reportEngineList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "report_engine.html", map[string]interface{}{
		"Title": "Mis Informes",
	})
}

// reportEngineTemplates devuelve las plantillas que cumplen la definición del
// motor tipado. Junto con /reports/custom/data permite que la pantalla incluya
// ambas fuentes, independientemente de dónde se guardó cada informe.
func (s *Server) reportEngineTemplates(w http.ResponseWriter, r *http.Request) {
	all := s.loadReportTemplates(s.sessionFromRequest(r))
	out := make([]map[string]interface{}, 0, len(all))
	for _, item := range all {
		if item.Status != 1 {
			continue
		}
		var def ReportDefinition
		if err := json.Unmarshal([]byte(item.Definition), &def); err != nil || def.Validate() != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": item.ID, "name": item.Name, "title": def.Title, "params": def.Params,
		})
	}
	writeJSON(w, out)
}

// reportTemplateParams devuelve los parámetros de una plantilla.
//
// Se pide por separado y no junto con la lista para que la pantalla de
// ejecución pueda construirse sin descargar todas las definiciones: son JSON
// que nadie necesita ver a diario.
func (s *Server) reportTemplateParams(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	def, err := s.loadReportDefinition(r, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	writeJSON(w, map[string]interface{}{
		"id": id, "title": def.Title, "params": def.Params, "columns": def.Columns,
	})
}

// reportValuesFromRequest recoge los parámetros del formulario.
//
// Se admite el prefijo "p_" porque el mismo endpoint atiende al formulario HTML
// y a las llamadas por AJAX, y en el segundo los nombres viajan ya cualificados.
func reportValuesFromRequest(r *http.Request, def *ReportDefinition) map[string]string {
	values := map[string]string{}
	for _, p := range def.Params {
		values[p.Name] = r.FormValue("p_" + p.Name)
	}
	return values
}

// reportEngineRun ejecuta la plantilla y devuelve JSON.
func (s *Server) reportEngineRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	def, err := s.loadReportDefinition(r, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	values := reportValuesFromRequest(r, def)
	run, err := s.RunReportDefinition(r, def, values)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, reportRunJSON(run))
}

// reportEnginePDF ejecuta la plantilla y devuelve PDF.
//
// Ruta aparte y no un parámetro de la de CSV: son formatos con memoriales
// distintos y la forma de pedirlo es distinta (navegador vs descarga). Meter un
// ?format=pdf obligaría a validar el parámetro en los dos caminos y a comprobar
// en cada uno que lo que se devuelve es lo que se pidió.
//
// El subtítulo se arma con el período y los parámetros de la plantilla, que es
// lo que hace falta para que un informe impreso siga siendo entendible dentro
// de seis meses: un PDF con las cifras y sin el período no dice de cuándo son.
func (s *Server) reportEnginePDF(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	def, err := s.loadReportDefinition(r, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	values := reportValuesFromRequest(r, def)
	run, err := s.RunReportDefinition(r, def, values)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	subtitulo := subtituloDeInforme(values)
	if empresa := strings.TrimSpace(s.settingsFor(r).Company.CompanyName); empresa != "" {
		if subtitulo != "" {
			subtitulo = empresa + " — " + subtitulo
		} else {
			subtitulo = empresa
		}
	}
	pdfBytes, err := ReportPDF(run, subtitulo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	nombre := reportSafeFilename(def.Title)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s.pdf\"", nombre))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)

	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), "", "export_report_pdf",
		"custom_reports", "report_template", fmt.Sprintf("%d", id), nil,
		map[string]interface{}{"titulo": def.Title})
}

// subtituloDeInforme arma la línea de contexto del PDF.
//
// Los parámetros van con su etiqueta, no solo con su valor: un "01/01/2026" sin
// decir que es la fecha desde no dice nada. Y se ordena por clave para que el
// subtítulo sea el mismo siempre, en vez de cambiar de un día a otro según el
// mapa.
func subtituloDeInforme(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	claves := make([]string, 0, len(values))
	for k := range values {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	partes := make([]string, 0, len(claves))
	for _, k := range claves {
		v := strings.TrimSpace(values[k])
		if v == "" {
			continue
		}
		partes = append(partes, k+": "+v)
	}
	if len(partes) == 0 {
		return ""
	}
	return strings.Join(partes, " · ")
}

// reportEngineExport ejecuta la plantilla y devuelve CSV.
//
// El CSV se genera en memoria y no en un archivo temporal: para los tamaños que
// maneja este motor es más rápido y evita dejar restos en disco si la petición
// se corta a mitad.
func (s *Server) reportEngineExport(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	def, err := s.loadReportDefinition(r, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	values := reportValuesFromRequest(r, def)
	run, err := s.RunReportDefinition(r, def, values)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	csvBytes, err := ReportCSV(run)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	nombre := reportSafeFilename(def.Title)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s.csv\"", nombre))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvBytes)

	// La exportación se audita: es el momento en que los datos salen del
	// sistema, y conviene saber quién los sacó y de qué período.
	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), "", "export_report",
		"reporting", "report_template", strconv.FormatInt(id, 10),
		map[string]interface{}{"params": values},
		map[string]interface{}{"rows": len(run.Rows), "truncated": run.Truncated})
}

// reportRunJSON convierte el resultado a la forma que consume la tabla.
func reportRunJSON(run *ReportRun) map[string]interface{} {
	data := make([]map[string]interface{}, 0, len(run.Rows))
	for _, row := range run.Rows {
		obj := make(map[string]interface{}, len(run.Keys))
		for i, k := range run.Keys {
			if i < len(row) {
				obj[k] = row[i]
			}
		}
		data = append(data, obj)
	}
	out := map[string]interface{}{
		"title": run.Title, "headers": run.Headers, "keys": run.Keys,
		"rows": data, "totals": run.Totals, "truncated": run.Truncated,
		"backend": run.Backend, "row_count": len(run.Rows), "max_rows": run.MaxRows,
		"warnings": run.Warnings,
	}
	if run.Chart != nil {
		out["chart_data"] = run.Chart
	}
	return out
}

// reportSafeFilename deja un nombre de archivo utilizable en cualquier sistema.
//
// Se usa con el título del informe, que es texto libre del usuario: un título
// con "/", ":" o acentos produciría un nombre que rompe la descarga o la
// confunde en el equipo del cliente.
func reportSafeFilename(titulo string) string {
	t := strings.TrimSpace(titulo)
	var b strings.Builder
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "informe"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
