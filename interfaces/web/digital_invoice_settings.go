// digital_invoice_settings.go — Configuración de Facturación Digital (SENIAT).
// Gaceta Oficial 43.484 (21 oct 2026) — no obligatorio usar impresora fiscal.
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	di "erpgow/interfaces/web/digitalinvoice"
	"erpgow/internal/digitalinvoice"
)

func (s *Server) registerDigitalInvoiceRoutes() {
	s.mux.HandleFunc("GET /settings/digital-invoice", s.digitalInvoicePage)
	s.mux.HandleFunc("GET /settings/digital-invoice/data", s.digitalInvoiceData)
	s.mux.HandleFunc("POST /settings/digital-invoice/save", s.digitalInvoiceSave)
	s.mux.HandleFunc("POST /settings/digital-invoice/activate", s.digitalInvoiceActivate)
	s.mux.HandleFunc("POST /settings/digital-invoice/ping", s.digitalInvoicePing)
}

// requireDigitalInvoiceAdmin verifica que el usuario sea master o admin.
func (s *Server) requireDigitalInvoiceAdmin(w http.ResponseWriter, r *http.Request) bool {
	sess := s.sessionFromRequest(r)
	if sess == nil || (sess.UserLevel != "master" && sess.UserLevel != "admin") {
		jsonError(w, "Se requiere rol admin o master", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) digitalInvoicePage(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	cfg, _ := s.digitalInvoiceConfig(r)
	addonCode, _ := s.digitalInvoiceAddonCode(r)
	providerName := ""
	if p := digitalinvoice.ProveedorActivo(); p != nil {
		providerName = p.Nombre()
	}
	s.render(w, r, "digital_invoice.html", map[string]interface{}{
		"Title":        "Facturación Digital",
		"Config":       cfg,
		"AddonCode":    addonCode,
		"Active":       digitalinvoice.HayProveedorActivo(),
		"ProviderName": providerName,
	})
}

func (s *Server) digitalInvoiceData(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}

	var activeAddons []map[string]interface{}
	rows, _ := s.authDB(r).Query("SELECT id, name, version, is_active, status FROM addons WHERE category = 'digital_invoice'")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var code, name, version, status string
			var isActive int
			_ = rows.Scan(&code, &name, &version, &isActive, &status)
			activeAddons = append(activeAddons, map[string]interface{}{
				"code": code, "name": name, "version": version,
				"active": isActive == 1, "status": status,
			})
		}
	}

	writeJSON(w, map[string]interface{}{
		"active_addons": activeAddons,
		"provider": func() string {
			if p := digitalinvoice.ProveedorActivo(); p != nil {
				return p.Nombre()
			}
			return ""
		}(),
		"available": []map[string]interface{}{
			{"code": "unidigital", "name": "Unidigital", "built_in": true},
		},
	})
}

func (s *Server) digitalInvoiceSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}

	code := r.FormValue("addon_code")
	if code == "" {
		code = "unidigital"
	}

	cfg := di.ProviderConfig{
		Code:          code,
		Name:          r.FormValue("name"),
		Enabled:       r.FormValue("enabled") == "1",
		BaseURL:       r.FormValue("base_url"),
		BearerToken:   r.FormValue("bearer_token"),
		SerieStrongID: r.FormValue("serie_id"),
		SucursalID:    r.FormValue("sucursal_id"),
		DocumentoTipo: r.FormValue("doc_type"),
		Moneda:        r.FormValue("moneda"),
	}
	if cfg.Moneda == "" {
		cfg.Moneda = "VES"
	}

	configJSON, _ := json.Marshal(cfg)
	_, err := s.authDB(r).Exec(`INSERT INTO digital_invoice_config (company_id, addon_code, config_json, enabled, created_at)
		VALUES (?, ?, ?, ?, NOW()) ON DUPLICATE KEY UPDATE config_json=?, enabled=?, updated_at=NOW()`,
		"1", code, string(configJSON), diBoolToInt(cfg.Enabled), string(configJSON), diBoolToInt(cfg.Enabled))
	if err != nil {
		jsonError(w, "Error guardando: "+err.Error(), 500)
		return
	}

	if cfg.Enabled {
		provider, err := digitalinvoice.NuevoProveedorActivo(code, cfg)
		if err != nil {
			jsonError(w, "Error creando proveedor: "+err.Error(), 500)
			return
		}
		digitalinvoice.RegistrarProvider(provider)
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Configuración guardada y proveedor activado"})
	} else {
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Configuración guardada (deshabilitado)"})
	}
}

func (s *Server) digitalInvoiceActivate(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}

	code := r.FormValue("code")
	if code == "" {
		jsonError(w, "Código requerido", 400)
		return
	}

	// Activar addon (exclusividad la maneja addon_handlers.go)
	_, _ = s.authDB(r).Exec("UPDATE addons SET is_active=1, status='active' WHERE id=?", code)

	if code == "unidigital" {
		cfg := di.ProviderConfig{Code: code, Enabled: true}
		provider, err := digitalinvoice.NuevoProveedorActivo(code, cfg)
		if err != nil {
			jsonError(w, "Error: "+err.Error(), 500)
			return
		}
		digitalinvoice.RegistrarProvider(provider)
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Addon activado"})
}

func (s *Server) digitalInvoicePing(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}

	code := r.FormValue("code")
	if code == "" {
		code = "unidigital"
	}

	cfg := di.ProviderConfig{
		Code:          code,
		BaseURL:       r.FormValue("base_url"),
		BearerToken:   r.FormValue("bearer_token"),
		SerieStrongID: r.FormValue("serie_id"),
	}
	provider, err := digitalinvoice.NuevoProveedorActivo(code, cfg)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := provider.Ping(ctx); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "provider": provider.Nombre()})
}

// Helpers internos

func (s *Server) digitalInvoiceConfig(r *http.Request) (map[string]interface{}, error) {
	var raw string
	err := s.authDB(r).QueryRow("SELECT config_json FROM digital_invoice_config LIMIT 1").Scan(&raw)
	if err == sql.ErrNoRows {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg, nil
}

func (s *Server) digitalInvoiceAddonCode(r *http.Request) (string, error) {
	var code string
	err := s.authDB(r).QueryRow("SELECT addon_code FROM digital_invoice_config LIMIT 1").Scan(&code)
	if err == sql.ErrNoRows {
		return "unidigital", nil
	}
	return code, err
}

func diBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
