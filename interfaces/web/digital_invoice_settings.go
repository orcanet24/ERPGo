// digital_invoice_settings.go — Configuración de Facturación Digital (SENIAT).
// Gaceta Oficial 43.484 (21 oct 2026) — no obligatorio usar impresora fiscal.
//
// hereby: este módulo YA NO depende del sistema de addons (retirado). El
// catálogo de proveedores digitales vive en código (diProveedorCatalogo) y
// cada EMPRESA elige el suyo: solo UNO puede estar activo por empresa. Si el
// admin activa otro, el anterior queda OFF automáticamente y la respuesta
// arrastrará la advertencia para que la pantalla muestre la alerta.
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	di "erpgo/interfaces/web/digitalinvoice"
	"erpgo/internal/digitalinvoice"
)

// diProveedorCatalogo es el catálogo de empresas de facturación digital que el
// ERP puede operar. Unidigital está integrado; nuevas proveedores se agregan
// aquí cuando llegue su implementación en internal/digitalinvoice.
var diProveedorCatalogo = []map[string]interface{}{
	{"code": "unidigital", "name": "Unidigital (Corporación Unidigital 1220 C.A.)", "built_in": true},
}

// diCatalogoPermitido contiene los códigos válidos para validar input.
var diCatalogoPermitido = map[string]string{
	"unidigital": "Unidigital",
}

func (s *Server) registerDigitalInvoiceRoutes() {
	s.mux.HandleFunc("GET /settings/digital-invoice", s.digitalInvoicePage)
	s.mux.HandleFunc("GET /settings/digital-invoice/data", s.digitalInvoiceData)
	s.mux.HandleFunc("POST /settings/digital-invoice/save", s.digitalInvoiceSave)
	s.mux.HandleFunc("POST /settings/digital-invoice/activate", s.digitalInvoiceActivate)
	s.mux.HandleFunc("POST /settings/digital-invoice/ping", s.digitalInvoicePing)
	// Relación de documentos emitidos en contingencia (POS sin red), anexo de
	// la notificación que exige el art. 16 de la Providencia SNAT/2024/000102.
	s.mux.HandleFunc("GET /settings/digital-invoice/contingencia.csv", s.contingenciaCSV)
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
	providerCode, _ := s.digitalInvoiceProviderCode(r)
	tenant := s.tenantCodeOrDefault()

	// Proveedor activo de ESTA empresa (cada empresa tiene la suya).
	provName := ""
	active := false
	if p := digitalinvoice.ProveedorPorTenant(tenant); p != nil {
		provName = p.Nombre()
		active = true
	}

	// Catálogo al template: el select de imprentas se arma del código, no
	// del HTML. Cada imprenta nueva en diProveedorCatalogo aparece sola y no
	// hay que tocar la pantalla.
	var catalogo []map[string]interface{}
	for _, p := range diProveedorCatalogo {
		codigo, _ := p["code"].(string)
		nombre, _ := p["name"].(string)
		catalogo = append(catalogo, map[string]interface{}{
			"code":   codigo,
			"name":   nombre,
			"active": codigo == providerCode && active,
		})
	}

	// Documentos de contingencia: pendientes primero, con su estado de
	// transmisión. Es la operación real del POS cuando se cae la red.
	contFilas, contPend, contFall := s.contingenciaResumen(r)

	// Entrega automática al cliente: el ajuste y los canales de correo REALES.
	// Sin canal activo el select sale vacío y el guardado lo dirá con claridad.
	entregaActiva, entregaCanal := s.diEntregaConfig(s.dbFor(r))
	var correos []map[string]interface{}
	for _, c := range s.notifyCanales(s.dbFor(r), true) {
		if c.Kind == "email" {
			correos = append(correos, map[string]interface{}{"id": c.ID, "name": c.Name})
		}
	}

	s.render(w, r, "digital_invoice.html", map[string]interface{}{
		"Title":        "Facturación Digital",
		"Config":       cfg,
		"ProviderCode": providerCode,
		"Active":       active,
		"ProviderName": provName,
		"Tenant":       tenant,
		"Catalogo":     catalogo,
		"Contingencia": contFilas,
		"ContPend":     contPend,
		"ContFall":     contFall,
		"EntregaActiva": entregaActiva,
		"EntregaCanal":  entregaCanal,
		"CanalesCorreo": correos,
	})
}

func (s *Server) digitalInvoiceData(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	tenant := s.tenantCodeOrDefault()
	auth := s.authDB(r)

	// Las configuraciones de ESTA empresa: una fila por proveedor conocido.
	var configs []map[string]interface{}
	rows, err := auth.Query(`SELECT addon_code, enabled, updated_at FROM digital_invoice_config WHERE company_id = ? ORDER BY addon_code`, tenant)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var code string
			var enabled int
			var updated sql.NullString
			if err := rows.Scan(&code, &enabled, &updated); err != nil {
				continue
			}
			name := diCatalogoPermitido[code]
			if name == "" {
				name = code
			}
			configs = append(configs, map[string]interface{}{
				"provider_code": code,
				"name":          name,
				"enabled":       enabled == 1,
			})
		}
	}

	provName := ""
	if p := digitalinvoice.ProveedorPorTenant(tenant); p != nil {
		provName = p.Nombre()
	}

	writeJSON(w, map[string]interface{}{
		"company":   s.currentDBPath(),
		"provider":  provName,
		"configs":   configs,
		"available": diProveedorCatalogo,
	})
}

// digitalInvoiceSave guarda la configuración de UN proveedor para la empresa
// activa. Si viene habilitado, aplica la exclusividad: los otros proveedores
// de esta empresa quedan deshabilitados, y la respuesta informa cuáles quedaron
// desactivados para que la pantalla levante la alerta.
func (s *Server) digitalInvoiceSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}

	tenant := s.tenantCodeOrDefault()
	auth := s.authDB(r)

	code := r.FormValue("provider_code")
	if code == "" {
		code = r.FormValue("addon_code") // compatibilidad con plantillas viejas
	}
	if code == "" {
		code = "unidigital"
	}
	if diCatalogoPermitido[code] == "" {
		jsonError(w, "El proveedor '"+code+"' no está en el catálogo", 400)
		return
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
	_, err := auth.Exec(`INSERT INTO digital_invoice_config (company_id, addon_code, config_json, enabled, created_at)
		VALUES (?, ?, ?, ?, NOW()) ON DUPLICATE KEY UPDATE config_json=?, enabled=?, updated_at=NOW()`,
		tenant, code, string(configJSON), diBoolToInt(cfg.Enabled), string(configJSON), diBoolToInt(cfg.Enabled))
	if err != nil {
		jsonError(w, "Error guardando: "+err.Error(), 500)
		return
	}

	resp := map[string]interface{}{"status": 1}
	if cfg.Enabled {
		resp["msg"] = "Configuración guardada y proveedor activado"
		detalle, errAct := s.diActivarProveedor(auth, tenant, code, cfg)
		if errAct != nil {
			jsonError(w, errAct.Error(), 500)
			return
		}
		resp["deactivated"] = detalle.Desactivados
		if len(detalle.Desactivados) > 0 {
			resp["msg"] = "Proveedor activado. ATENCIÓN: solo puede existir UNA empresa de facturación digital activa por empresa; quedaron desactivados: " + detalle.DesactivadosTexto
		}
	} else {
		// Deshabilitado: si era el activo, baja el activo en memoria también.
		digitalinvoice.DesactivarPorTenant(tenant)
		resp["msg"] = "Configuración guardada (proveedor deshabilitado)"
	}
	writeJSON(w, resp)
}

// digitalInvoiceActivate activa una empresa de facturación digital para la
// empresa activa. Aplica la exclusividad: solo existe una empresa activa —
// primero se desactiva el actual (si era distinto) y se activa el nuevo.
func (s *Server) digitalInvoiceActivate(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	tenant := s.tenantCodeOrDefault()
	code := r.FormValue("code")
	if code == "" {
		jsonError(w, "Código requerido", 400)
		return
	}
	if diCatalogoPermitido[code] == "" {
		jsonError(w, "El proveedor '"+code+"' no está en el catálogo", 400)
		return
	}
	auth := s.authDB(r)

	// Cargar la config guardada de ese proveedor para esta empresa, para
	// validar que esté completo antes del swap.
	cfg, err := s.diConfigPorProveedor(r, tenant, code)
	if err != nil {
		jsonError(w, "Este proveedor no tiene configuración guardada. Guárdela primero.", 400)
		return
	}
	cfg.Enabled = true

	if _, err := auth.Exec(`UPDATE digital_invoice_config SET enabled=1 WHERE company_id=? AND addon_code=?`, tenant, code); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	detalle, err := s.diActivarProveedor(auth, tenant, code, cfg)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	resp := map[string]interface{}{"status": 1, "deactivated": detalle.Desactivados}
	if len(detalle.Desactivados) > 0 {
		resp["msg"] = "Proveedor activado. ATENCIÓN: solo puede existir UNA empresa de facturación digital activa por empresa; quedó desactivado: " + detalle.DesactivadosTexto
	} else {
		resp["msg"] = "Empresa de facturación digital activada"
	}
	writeJSON(w, resp)
}

// diActivarDetalle es el resultado de aplicar la exclusividad por empresa.
type diActivarDetalle struct {
	Desactivados      []string
	DesactivadosTexto string
}

// diActivarProveedor marca la exclusividad en la base (disabled=0 el resto de
// esta empresa) y monta el proveedor en memoria para este tenant.
func (s *Server) diActivarProveedor(auth *sql.DB, tenant, code string, cfg di.ProviderConfig) (diActivarDetalle, error) {
	// La exclusividad en datos: misma empresa, otro código queda OFF.
	var desactivados []string
	rows, err := auth.Query(`SELECT addon_code FROM digital_invoice_config WHERE company_id=? AND addon_code<>? AND enabled=1`, tenant, code)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var otro string
			if rows.Scan(&otro) == nil && otro != "" {
				desactivados = append(desactivados, diCatalogoPermitido[otro])
			}
		}
		rows.Close()
		if _, err := auth.Exec(`UPDATE digital_invoice_config SET enabled=0 WHERE company_id=? AND addon_code<>?`, tenant, code); err != nil {
			return diActivarDetalle{}, err
		}
	}

	provider, err := digitalinvoice.NuevoProveedorActivo(code, cfg)
	if err != nil {
		return diActivarDetalle{}, err
	}
	digitalinvoice.RegistrarPorTenant(tenant, provider)

	texto := ""
	for i, n := range desactivados {
		if i > 0 {
			texto += ", "
		}
		texto += n
	}
	return diActivarDetalle{Desactivados: desactivados, DesactivadosTexto: texto}, nil
}

// diConfigPorProveedor trae la config guardada de un proveedor para una
// empresa. ErrNoRows significa "no configurado todavía".
func (s *Server) diConfigPorProveedor(r *http.Request, tenant, code string) (di.ProviderConfig, error) {
	var raw string
	err := s.authDB(r).QueryRow(`SELECT config_json FROM digital_invoice_config WHERE company_id=? AND addon_code=?`, tenant, code).Scan(&raw)
	if err != nil {
		return di.ProviderConfig{}, err
	}
	var cfg di.ProviderConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return di.ProviderConfig{}, err
	}
	return cfg, nil
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
	tenant := s.tenantCodeOrDefault()
	code, _ := s.digitalInvoiceProviderCode(r)
	var raw string
	err := s.authDB(r).QueryRow(`SELECT config_json FROM digital_invoice_config WHERE company_id=? AND addon_code=?`, tenant, code).Scan(&raw)
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

// digitalInvoiceProviderCode devuelve el proveedor habilitado de la empresa.
func (s *Server) digitalInvoiceProviderCode(r *http.Request) (string, error) {
	tenant := s.tenantCodeOrDefault()
	var code string
	err := s.authDB(r).QueryRow(`SELECT addon_code FROM digital_invoice_config WHERE company_id=? AND enabled=1 LIMIT 1`, tenant).Scan(&code)
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

