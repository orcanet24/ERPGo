package web

// ============================================================================
// ERPGo — Ajustes Fiscales (series, IGTF, parámetros SENIAT)
// ============================================================================

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerFiscalSettingsRoutes() {
	s.mux.HandleFunc("GET /settings/fiscal", s.fiscalSettingsPage)
	s.mux.HandleFunc("POST /settings/fiscal/save", s.fiscalSettingsSave)
	s.mux.HandleFunc("POST /settings/fiscal/series/save", s.fiscalSeriesSave)
	// Prueba real de la consulta al SENIAT: corre el flujo completo y devuelve
	// el fallo EXACTO (OCR ausente, 403 del WAF, captcha ilegible). Bajo
	// /settings → solo master, como el resto de la pantalla.
	s.mux.HandleFunc("GET /settings/fiscal/seniat-test", s.seniatTest)
}

type fiscalSeriesRow struct {
	ID          int
	Code, Name  string
	Prefix      string
	NextNo      int64
	ControlPre  string
	ControlNext int64
	Active      bool
}

func (s *Server) fiscalSettingsPage(w http.ResponseWriter, r *http.Request) {
	fs := s.loadFiscalSettings(r)
	series := []fiscalSeriesRow{}
	rows, err := s.dbFor(r).Query(`SELECT id, code, name, IFNULL(prefix,''), next_no, control_prefix, control_next, is_active
		FROM fiscal_series ORDER BY id`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var f fiscalSeriesRow
			var act int
			_ = rows.Scan(&f.ID, &f.Code, &f.Name, &f.Prefix, &f.NextNo, &f.ControlPre, &f.ControlNext, &act)
			f.Active = act == 1
			series = append(series, f)
		}
	}
	st := s.settingsFor(r)
	s.render(w, r, "fiscal_settings.html", map[string]interface{}{
		"Title":   "Ajustes Fiscales (SENIAT)",
		"Series":  series,
		"FS":      fs,
		"Company": st.Company,
	})
}

func (s *Server) fiscalSettingsSave(w http.ResponseWriter, r *http.Request) {
	igtfOn := "0"
	if r.FormValue("igtf_enabled") == "1" {
		igtfOn = "1"
	}
	rate := parseMoney(r.FormValue("igtf_rate"))
	if rate < 0 || rate > 100 {
		rate = 3.0
	}
	walkin := "0"
	if r.FormValue("walkin_rif_required") == "1" {
		walkin = "1"
	}
	foot := strings.TrimSpace(r.FormValue("fiscal_footnote"))
	_ = s.setFiscalSetting(r, "igtf_enabled", igtfOn)
	_ = s.setFiscalSetting(r, "igtf_rate", strconv.FormatFloat(rate, 'f', 2, 64))
	_ = s.setFiscalSetting(r, "walkin_rif_required", walkin)
	if foot != "" {
		_ = s.setFiscalSetting(r, "fiscal_footnote", foot)
	}
	// Consulta del contribuyente en el SENIAT (autollenado de clientes).
	seniatOn := "0"
	if r.FormValue("seniat_autofill") == "1" {
		seniatOn = "1"
	}
	_ = s.setFiscalSetting(r, "seniat_autofill", seniatOn)
	err := s.setFiscalSetting(r, "seniat_ocr_cmd", strings.TrimSpace(r.FormValue("seniat_ocr_cmd")))
	if err != nil {
		jsonError(w, "No se pudo guardar el comando OCR: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Ajustes fiscales guardados", "redirect": saveRedirect(r, "/fiscal_settings")})
}

func (s *Server) fiscalSeriesSave(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	prefix := strings.TrimSpace(r.FormValue("prefix"))
	ctrlPre := strings.TrimSpace(r.FormValue("control_prefix"))
	if id == "" {
		jsonError(w, "Serie inválida", 400)
		return
	}
	_, err := s.dbFor(r).Exec(`UPDATE fiscal_series SET prefix = ?, control_prefix = ? WHERE id = ?`, prefix, ctrlPre, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Serie actualizada", "redirect": saveRedirect(r, "/fiscal_settings")})
}
