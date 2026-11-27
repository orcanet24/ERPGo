// digital_invoice_settings.go — Configuración de Facturación Digital (SENIAT).
// Gaceta Oficial 43.484 (21 oct 2026) — no obligatorio usar impresora fiscal.
//
// QUÉ DECIDE ESTA PANTALLA. Cada EMPRESA elige UNA de tres formas de facturar:
//
//  1. Imprenta Unidigital        (digital_invoice_config enabled=1, código unidigital)
//  2. Imprenta CG Imprenta Digital (enabled=1, código cgimprenta)
//  3. Impresora fiscal            (ninguna imprenta digital activa)
//
// La exclusividad no es un adorno: la ley venezolana permite UNA imprenta
// digital por contribuyente, y el ERP la hace cumplir en los datos —activar una
// desactiva la otra— además de avisarlo en pantalla. Desactivar NO borra la
// configuración: la imprenta queda lista para volver a activarse con sus
// credenciales intactas, que es justo lo que hace posible cambiar de proveedor
// sin volver a teclear nada.
//
// POR QUÉ EL CATÁLOGO VIVE EN CÓDIGO Y NO EN HTML. La pantalla se dibuja a
// partir de diCatalogo: cada imprenta declara sus campos y la plantilla los
// recorre. Una imprenta nueva se agrega aquí (y en el registro de proveedores)
// y la pantalla la muestra sola; si los campos se escribieran a mano en el HTML,
// agregar una obligaría a tocar la plantilla y a acordarse de sus validaciones.
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	di "erpgo/interfaces/web/digitalinvoice"
	"erpgo/internal/digitalinvoice"
)

// diCampo es un dato de configuración de una imprenta. Se dibuja como input y
// se guarda en digital_invoice_config.config_json con el mismo nombre.
type diCampo struct {
	Nombre      string // nombre del campo (form y config_json)
	Etiqueta    string
	Tipo        string // text | password | url
	Obligatorio bool
	Secreto     bool   // no se devuelve al navegador: si está guardado, la pantalla no lo repite
	PorDefecto  string // valor sugerido cuando aún no hay configuración
	Ayuda       string
}

// diImprenta es una casa de facturación digital que el ERP sabe operar.
type diImprenta struct {
	Code          string
	Nombre        string
	Descripcion   string
	Documentacion string
	Campos        []diCampo
}

// diCatalogo es el catálogo ÚNICO de imprentas. Añadir una exige su proveedor
// en internal/digitalinvoice/registry.go; sin las dos cosas no se ofrece.
var diCatalogo = []diImprenta{
	{
		Code:          "unidigital",
		Nombre:        "Unidigital (Corporación Unidigital 1220 C.A.)",
		Descripcion:   "Imprenta digital autorizada. Emite con el token que entrega la casa y una serie técnica de su plataforma.",
		Documentacion: "https://api.unidigital.ve/v1",
		Campos: []diCampo{
			{Nombre: "base_url", Etiqueta: "URL base de la API", Tipo: "url", Obligatorio: true,
				PorDefecto: "https://api.unidigital.ve/v1"},
			{Nombre: "bearer_token", Etiqueta: "Bearer Token", Tipo: "password", Obligatorio: true, Secreto: true,
				Ayuda: "Token de autenticación que entrega Unidigital al habilitar la integración."},
			{Nombre: "serie_id", Etiqueta: "Serie Strong ID", Tipo: "text", Obligatorio: true,
				Ayuda: "Identificador de la serie en la plataforma de Unidigital."},
			{Nombre: "sucursal_id", Etiqueta: "Sucursal ID (opcional)", Tipo: "text"},
		},
	},
	{
		Code:          "cgimprenta",
		Nombre:        "CG Imprenta Digital",
		Descripcion:   "Imprenta digital autorizada con API propia: usuario y contraseña de API, token de una hora y números de control con serie.",
		Documentacion: "https://calidad.cgimprenta.digital/doc/",
		Campos: []diCampo{
			{Nombre: "base_url", Etiqueta: "URL base de la API", Tipo: "url", Obligatorio: true,
				PorDefecto: "https://calidad.cgimprenta.digital/",
				Ayuda:      "El host que le entregue CG Imprenta Digital. El de calidad es el ambiente de pruebas."},
			{Nombre: "user_name", Etiqueta: "Usuario de API", Tipo: "text", Obligatorio: true},
			{Nombre: "user_password", Etiqueta: "Contraseña de API", Tipo: "password", Obligatorio: true, Secreto: true},
			{Nombre: "numero_serie", Etiqueta: "Serie de numeración", Tipo: "text", Obligatorio: true,
				Ayuda: "La serie asignada a su negocio (por ejemplo «A»). Se valida contra CG en la primera emisión."},
			{Nombre: "sucursal_id", Etiqueta: "Sucursal (opcional)", Tipo: "text",
				Ayuda: "Si la escribe, CG la imprime debajo de la dirección fiscal de la factura."},
		},
	},
	{
		Code:          "thka",
		Nombre:        "The Factory HKA (TFHKA)",
		Descripcion:   "Imprenta digital autorizada con documento electrónico anidado: usuario y clave de API, token de doce horas y número de control asignado en la emisión. El PDF se archiva en el ERP cuando la imprenta lo entrega.",
		Documentacion: "https://demoemisionv2.thefactoryhka.com.ve/",
		Campos: []diCampo{
			{Nombre: "base_url", Etiqueta: "URL base de la API", Tipo: "url", Obligatorio: true,
				PorDefecto: "https://demoemisionv2.thefactoryhka.com.ve/",
				Ayuda:      "El host de demostración es demoemisionv2…; producción es emisionv2… Se lo entrega TFHKA con sus credenciales."},
			{Nombre: "user_name", Etiqueta: "Usuario de API", Tipo: "text", Obligatorio: true},
			{Nombre: "user_password", Etiqueta: "Clave de API", Tipo: "password", Obligatorio: true, Secreto: true},
			{Nombre: "numero_serie", Etiqueta: "Serie", Tipo: "text",
				Ayuda: "La serie del negocio (máximo 20 caracteres). Déjela vacía si no trabaja con series: THKA lo permite."},
			{Nombre: "sucursal_id", Etiqueta: "Sucursal (opcional)", Tipo: "text",
				Ayuda: "Código de sucursal de hasta 6 caracteres; THKA lo imprime en el documento si se envía."},
		},
	},
}

// diImprentaPorCodigo busca una imprenta del catálogo.
func diImprentaPorCodigo(code string) (diImprenta, bool) {
	for _, imp := range diCatalogo {
		if imp.Code == code {
			return imp, true
		}
	}
	return diImprenta{}, false
}

// diNombreImprenta es el nombre legible de un código, o el código si no está en
// el catálogo (una fila vieja de un addon retirado).
func diNombreImprenta(code string) string {
	if imp, ok := diImprentaPorCodigo(code); ok {
		return imp.Nombre
	}
	return code
}

// diImprentaVista es una imprenta con su estado para la pantalla.
type diImprentaVista struct {
	diImprenta
	Activa      bool              // es la que emite hoy
	Configurada bool              // tiene fila guardada
	Valores     map[string]string // valores actuales (los secretos van vacíos)
	Guardados   map[string]bool   // campos secretos que YA tienen valor guardado
}

func (s *Server) registerDigitalInvoiceRoutes() {
	s.mux.HandleFunc("GET /settings/digital-invoice", s.digitalInvoicePage)
	s.mux.HandleFunc("GET /settings/digital-invoice/data", s.digitalInvoiceData)
	s.mux.HandleFunc("POST /settings/digital-invoice/save", s.digitalInvoiceSave)
	s.mux.HandleFunc("POST /settings/digital-invoice/activate", s.digitalInvoiceActivate)
	s.mux.HandleFunc("POST /settings/digital-invoice/deactivate", s.digitalInvoiceDeactivate)
	s.mux.HandleFunc("POST /settings/digital-invoice/fiscal", s.digitalInvoiceFiscal)
	s.mux.HandleFunc("POST /settings/digital-invoice/ping", s.digitalInvoicePing)
	s.mux.HandleFunc("POST /settings/digital-invoice/retry", s.digitalInvoiceRetry)
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
	tenant := s.tenantOf(r)
	db := s.authDB(r)

	activo, guardadas := diEstadoImprentas(db, tenant)
	imprentas := diVistasDeCatalogo(activo, guardadas)

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
		"Title":           "Facturación Digital",
		"Tenant":          tenant,
		"Imprentas":       imprentas,
		"ProveedorActivo": diNombreImprenta(activo),
		"ModoDigital":     activo != "",
		"Contingencia":    contFilas,
		"ContPend":        contPend,
		"ContFall":        contFall,
		"EntregaActiva":   entregaActiva,
		"EntregaCanal":    entregaCanal,
		"CanalesCorreo":   correos,
	})
}

func (s *Server) digitalInvoiceData(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	tenant := s.tenantOf(r)
	activo, guardadas := diEstadoImprentas(s.authDB(r), tenant)

	configs := make([]map[string]interface{}, 0, len(diCatalogo))
	for _, imp := range diCatalogo {
		cfg, hay := guardadas[imp.Code]
		configs = append(configs, map[string]interface{}{
			"provider_code": imp.Code,
			"name":          imp.Nombre,
			"saved":         hay,
			"enabled":       activo == imp.Code,
			"base_url":      cfg.BaseURL,
			"serie":         primerNoVacio(cfg.NumeroSerie, cfg.SerieStrongID),
		})
	}
	// El catálogo se devuelve con llaves en minúscula y SIN valores de
	// credencial: este endpoint lo consume la propia pantalla (y cualquier
	// integración), y un token o una contraseña no salen de la base por aquí.
	disponibles := make([]map[string]interface{}, 0, len(diCatalogo))
	for _, imp := range diCatalogo {
		campos := make([]map[string]interface{}, 0, len(imp.Campos))
		for _, campo := range imp.Campos {
			campos = append(campos, map[string]interface{}{
				"name": campo.Nombre, "label": campo.Etiqueta, "type": campo.Tipo,
				"required": campo.Obligatorio, "secret": campo.Secreto,
			})
		}
		disponibles = append(disponibles, map[string]interface{}{
			"code": imp.Code, "name": imp.Nombre, "description": imp.Descripcion,
			"documentation": imp.Documentacion, "fields": campos,
		})
	}

	writeJSON(w, map[string]interface{}{
		"company":   s.currentDBPath(),
		"provider":  diNombreImprenta(activo),
		"mode":      diNombreModo(activo),
		"configs":   configs,
		"available": disponibles,
	})
}

// digitalInvoiceSave guarda la configuración de UNA imprenta SIN activarla.
//
// Guardar y activar son deliberadamente dos acciones: una configuración a
// medias se guarda (para no perder lo tecleado) pero no emite, y activar exige
// que la prueba de conexión pase. Antes, guardar con la casilla marcada dejaba
// la imprenta "activa" en la base aunque el proveedor no contestara.
func (s *Server) digitalInvoiceSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	code, imp, ok := s.diImprentaDelFormulario(r, w)
	if !ok {
		return
	}
	tenant := s.tenantOf(r)
	db := s.authDB(r)

	guardadas := diConfiguracionesDeEmpresa(db, tenant)
	cfg := diCombinarConfig(code, guardadas[code], camposDelFormulario(r, imp))
	if err := diValidarConfig(code, cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := diGuardarConfig(db, tenant, cfg); err != nil {
		jsonError(w, "No se pudo guardar la configuración: "+err.Error(), http.StatusInternalServerError)
		return
	}
	msg := "Configuración de " + imp.Nombre + " guardada."
	if activo, _ := diProveedorActivoEnBase(db, tenant); activo == code {
		// Si ya era la activa, la configuración nueva se aplica en memoria en el
		// acto: si no, la cola seguiría emitiendo con la vieja hasta el próximo
		// reinicio. Guardar NO revalida la conexión: eso lo hace «Probar
		// conexión» y «Activar».
		cfg.Enabled = true
		if p, err := digitalinvoice.NuevoProveedorActivo(code, cfg); err == nil {
			digitalinvoice.RegistrarPorTenant(tenant, p)
		}
		msg += " La imprenta está activa; para comprobar las credenciales nuevas use «Probar conexión»."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg})
}

// digitalInvoiceActivate activa una imprenta para la empresa: prueba la
// conexión PRIMERO, y solo si responde aplica la exclusividad (la otra queda
// desactivada) y monta el proveedor en memoria.
func (s *Server) digitalInvoiceActivate(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	code, imp, ok := s.diImprentaDelFormulario(r, w)
	if !ok {
		return
	}
	tenant := s.tenantOf(r)
	db := s.authDB(r)

	cfg, guardada := diConfiguracionesDeEmpresa(db, tenant)[code]
	if !guardada {
		jsonError(w, "La configuración de "+imp.Nombre+" no está guardada todavía: guárdela y vuelva a activar.", http.StatusBadRequest)
		return
	}
	if err := diValidarConfig(code, cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// El proveedor se monta YA como habilitado: el registro en memoria es lo que
	// leen las pantallas y las pruebas de conexión posteriores.
	cfg.Enabled = true
	provider, err := digitalinvoice.NuevoProveedorActivo(code, cfg)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := provider.Ping(ctx); err != nil {
		jsonError(w, "No se activó: "+err.Error(), http.StatusBadRequest)
		return
	}

	desactivadas, err := diActivarEnBase(db, tenant, code)
	if err != nil {
		jsonError(w, "No se pudo activar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	digitalinvoice.RegistrarPorTenant(tenant, provider)

	resp := map[string]interface{}{
		"status":      1,
		"deactivated": desactivadas,
		"msg":         "Imprenta activa: " + imp.Nombre + ". Las facturas nuevas se emitirán por ella.",
	}
	if len(desactivadas) > 0 {
		resp["msg"] = "Imprenta activa: " + imp.Nombre + ". ATENCIÓN: solo puede existir UNA imprenta digital activa por empresa; quedó desactivada " + strings.Join(desactivadas, ", ") + "."
	}
	writeJSON(w, resp)
}

// digitalInvoiceDeactivate apaga UNA imprenta. La configuración se conserva para
// poder volver a activarla; si era la que emitía, la empresa queda en el modo
// que decida después (otra imprenta o impresora fiscal).
func (s *Server) digitalInvoiceDeactivate(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	code, imp, ok := s.diImprentaDelFormulario(r, w)
	if !ok {
		return
	}
	tenant := s.tenantOf(r)
	db := s.authDB(r)
	if err := diDesactivarEnBase(db, tenant, code); err != nil {
		jsonError(w, "No se pudo desactivar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	digitalinvoice.DesactivarProveedorPorTenant(tenant, code)

	activo, _ := diProveedorActivoEnBase(db, tenant)
	msg := imp.Nombre + " desactivada. Su configuración queda guardada."
	if activo == "" {
		msg += " La empresa queda facturando por impresora fiscal hasta que active una imprenta digital."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg, "mode": diNombreModo(activo)})
}

// digitalInvoiceFiscal deja la empresa SIN imprenta digital: facturación por
// impresora fiscal. Es una acción explícita (y no el efecto de desmarcar una
// casilla) porque cambia el régimen con el que se emiten las facturas.
func (s *Server) digitalInvoiceFiscal(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	tenant := s.tenantOf(r)
	db := s.authDB(r)
	if err := diDesactivarTodasEnBase(db, tenant); err != nil {
		jsonError(w, "No se pudo cambiar el modo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, imp := range diCatalogo {
		digitalinvoice.DesactivarProveedorPorTenant(tenant, imp.Code)
	}
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"mode":   diNombreModo(""),
		"msg":    "Facturación por impresora fiscal: ninguna imprenta digital queda activa. Las configuraciones guardadas se conservan para cuando quiera volver.",
	})
}

func (s *Server) digitalInvoicePing(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	code, imp, ok := s.diImprentaDelFormulario(r, w)
	if !ok {
		return
	}
	tenant := s.tenantOf(r)
	db := s.authDB(r)

	// Se prueba lo que hay EN PANTALLA, no lo guardado: el administrador usa
	// esta prueba justo para saber si lo que acaba de teclear sirve.
	cfg, _ := diConfiguracionesDeEmpresa(db, tenant)[code]
	cfg = diCombinarConfig(code, cfg, camposDelFormulario(r, imp))
	if err := diValidarConfig(code, cfg); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider, err := digitalinvoice.NuevoProveedorActivo(code, cfg)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := provider.Ping(ctx); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "provider": provider.Nombre()})
}

// digitalInvoiceRetry devuelve a la cola los documentos que quedaron rechazados.
//
// POR QUÉ EXISTE. Un documento rechazado casi nunca se arregla solo: era el
// correo que faltaba en la ficha del cliente, la tasa que no se había
// registrado, la serie mal escrita. Cuando el administrador corrige la causa,
// sin este botón el documento se queda en 'failed' para siempre y solo se
// puede reemitir a mano en la base.
func (s *Server) digitalInvoiceRetry(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	tenant := s.tenantOf(r)
	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "La base de control no está disponible", http.StatusServiceUnavailable)
		return
	}
	res, err := ctl.Exec(`UPDATE digital_invoice_log
		SET status='pending', intentos=0, ultimo_intento=NULL, error_message=NULL
		WHERE company_id=? AND status='failed'`, tenant)
	if err != nil {
		jsonError(w, "No se pudo devolver los documentos a la cola: "+err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	msg := "No había documentos rechazados que reintentar."
	if n > 0 {
		msg = fmt.Sprintf("%d documento(s) volvieron a la cola de emisión; el sistema los reintentará en el próximo ciclo.", n)
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg, "count": n})
}

// diVistasDeCatalogo arma lo que la pantalla muestra de cada imprenta.
//
// LOS SECRETOS NO VIAJAN AL NAVEGADOR. El token y la contraseña de API se
// guardan en la base y no se devuelven nunca: la pantalla solo sabe si ya hay
// uno guardado para avisar «deje vacío para conservarlo». Un campo de
// contraseña relleno con el valor real lo pondría delante de cualquiera que
// mire la pantalla o la consola del navegador.
func diVistasDeCatalogo(activo string, guardadas map[string]di.ProviderConfig) []diImprentaVista {
	vistas := make([]diImprentaVista, 0, len(diCatalogo))
	for _, imp := range diCatalogo {
		cfg, hay := guardadas[imp.Code]
		vista := diImprentaVista{
			diImprenta:  imp,
			Activa:      activo == imp.Code,
			Configurada: hay,
			Valores:     map[string]string{},
			Guardados:   map[string]bool{},
		}
		for _, campo := range imp.Campos {
			valor := diValorDeCampo(cfg, campo.Nombre)
			if campo.Secreto {
				vista.Guardados[campo.Nombre] = valor != ""
				valor = ""
			}
			// El valor sugerido solo se muestra cuando la imprenta no tiene
			// nada guardado: pisar una URL real con la de fábrica sería un
			// cambio que nadie pidió.
			if valor == "" && !campo.Secreto && campo.PorDefecto != "" && !hay {
				valor = campo.PorDefecto
			}
			vista.Valores[campo.Nombre] = valor
		}
		vistas = append(vistas, vista)
	}
	return vistas
}

// ---------- Estado de las imprentas en la base ----------

// diProveedorActivoEnBase devuelve el código de la imprenta que emite para la
// empresa, o "" cuando factura por impresora fiscal.
//
// La exclusividad garantiza una sola fila enabled=1; si una base vieja tuviera
// dos, se toma la primera por código y la pantalla mostrará esa: no se elige al
// azar entre dos regímenes fiscales.
func diProveedorActivoEnBase(db *sql.DB, tenant string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("base no disponible")
	}
	var code string
	err := db.QueryRow(`SELECT addon_code FROM digital_invoice_config
		WHERE company_id=? AND enabled=1 ORDER BY addon_code LIMIT 1`, tenant).Scan(&code)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

// diEstadoImprentas devuelve la imprenta activa y TODAS las configuraciones
// guardadas de la empresa, indexadas por código.
func diEstadoImprentas(db *sql.DB, tenant string) (string, map[string]di.ProviderConfig) {
	guardadas := diConfiguracionesDeEmpresa(db, tenant)
	activo, err := diProveedorActivoEnBase(db, tenant)
	if err != nil {
		activo = ""
	}
	return activo, guardadas
}

// diConfiguracionesDeEmpresa lee todas las filas de configuración de la empresa.
// Una fila con JSON ilegible NO tumba la pantalla: se omite y el administrador
// vuelve a guardarla.
func diConfiguracionesDeEmpresa(db *sql.DB, tenant string) map[string]di.ProviderConfig {
	salida := map[string]di.ProviderConfig{}
	if db == nil {
		return salida
	}
	rows, err := db.Query(`SELECT addon_code, config_json FROM digital_invoice_config WHERE company_id=?`, tenant)
	if err != nil {
		return salida
	}
	defer rows.Close()
	for rows.Next() {
		var code, crudo string
		if rows.Scan(&code, &crudo) != nil {
			continue
		}
		var cfg di.ProviderConfig
		if json.Unmarshal([]byte(crudo), &cfg) != nil {
			continue
		}
		cfg.Code = code
		salida[code] = cfg
	}
	return salida
}

// ---------- Reglas de configuración (puras, probadas aparte) ----------

// diCombinarConfig junta la configuración guardada con lo que trajo el
// formulario.
//
// LA REGLA DE LOS SECRETOS: un campo secreto que llega VACÍO conserva el valor
// guardado. El navegador nunca recibe el token ni la contraseña —la pantalla no
// los repite—, así que un formulario vacío significa "no lo cambié", no "bórralo".
// Si se interpretara al revés, guardar la URL de la imprenta activa borraría su
// token y la emisión empezaría a fallar en silencio.
func diCombinarConfig(code string, base di.ProviderConfig, entrantes map[string]string) di.ProviderConfig {
	imp, ok := diImprentaPorCodigo(code)
	if !ok {
		return base
	}
	base.Code = code
	base.Name = imp.Nombre
	for _, campo := range imp.Campos {
		valor, vino := entrantes[campo.Nombre]
		valor = strings.TrimSpace(valor)
		if campo.Secreto && (!vino || valor == "") {
			continue // conservar el guardado
		}
		diPonerCampo(&base, campo.Nombre, valor)
	}
	return base
}

// diValidarConfig comprueba que la imprenta tenga TODO lo necesario para
// emitir. Se valida antes de guardar, antes de activar y antes de probar la
// conexión: es más útil decir "falta la serie" que dejar que la imprenta
// rechace la primera factura con su propio mensaje.
func diValidarConfig(code string, cfg di.ProviderConfig) error {
	imp, ok := diImprentaPorCodigo(code)
	if !ok {
		return fmt.Errorf("el proveedor «%s» no está en el catálogo", code)
	}
	faltan := []string{}
	for _, campo := range imp.Campos {
		if !campo.Obligatorio {
			continue
		}
		if strings.TrimSpace(diValorDeCampo(cfg, campo.Nombre)) == "" {
			faltan = append(faltan, campo.Etiqueta)
		}
	}
	if len(faltan) > 0 {
		return fmt.Errorf("a %s le faltan datos: %s", imp.Nombre, strings.Join(faltan, ", "))
	}
	if !diURLAceptable(cfg.BaseURL) {
		return fmt.Errorf("la URL base de %s no es válida: escriba una dirección http o https completa (por ejemplo https://calidad.cgimprenta.digital/)", imp.Nombre)
	}
	return nil
}

// diValorDeCampo lee el campo del struct de configuración.
func diValorDeCampo(cfg di.ProviderConfig, nombre string) string {
	switch nombre {
	case "base_url":
		return cfg.BaseURL
	case "bearer_token":
		return cfg.BearerToken
	case "user_name":
		return cfg.UserName
	case "user_password":
		return cfg.UserPassword
	case "serie_id":
		return cfg.SerieStrongID
	case "numero_serie":
		return cfg.NumeroSerie
	case "sucursal_id":
		return cfg.SucursalID
	}
	return ""
}

// diPonerCampo escribe el campo del struct de configuración.
func diPonerCampo(cfg *di.ProviderConfig, nombre, valor string) {
	switch nombre {
	case "base_url":
		cfg.BaseURL = valor
	case "bearer_token":
		cfg.BearerToken = valor
	case "user_name":
		cfg.UserName = valor
	case "user_password":
		cfg.UserPassword = valor
	case "serie_id":
		cfg.SerieStrongID = valor
	case "numero_serie":
		cfg.NumeroSerie = valor
	case "sucursal_id":
		cfg.SucursalID = valor
	}
}

// diURLAceptable exige http o https con host. Un "localhost:8080" sin esquema
// es un error frecuente que, sin esta comprobación, solo aparecería como
// "no hubo respuesta" en la primera factura.
func diURLAceptable(base string) bool {
	b := strings.TrimSpace(base)
	if b == "" {
		return false
	}
	for _, prefijo := range []string{"http://", "https://"} {
		if !strings.HasPrefix(strings.ToLower(b), prefijo) {
			continue
		}
		resto := b[len(prefijo):]
		if i := strings.IndexAny(resto, "/?#"); i >= 0 {
			resto = resto[:i]
		}
		return resto != ""
	}
	return false
}

// diNombreModo traduce el código activo al régimen de facturación.
func diNombreModo(code string) string {
	if strings.TrimSpace(code) == "" {
		return "Facturación por impresora fiscal"
	}
	return diNombreImprenta(code)
}

func primerNoVacio(alternativas ...string) string {
	for _, a := range alternativas {
		if s := strings.TrimSpace(a); s != "" {
			return s
		}
	}
	return ""
}

// ---------- Acciones sobre digital_invoice_config (probadas con SQLite) ----------

// diGuardarConfig hace upsert de la configuración. NO toca `enabled`: activar
// es otra acción (diActivarEnBase). Cuando la fila es nueva nace desactivada.
//
// Se pregunta primero si existe en vez de usar ON DUPLICATE KEY porque el
// driver de MySQL informa filas CAMBIADAS y no filas encontradas: guardar la
// misma configuración dos veces habría caído en el INSERT, chocado con la clave
// única y mostrado un error por un guardado que no cambió nada.
func diGuardarConfig(db *sql.DB, tenant string, cfg di.ProviderConfig) error {
	if db == nil {
		return fmt.Errorf("base no disponible")
	}
	cfg.Code = strings.TrimSpace(cfg.Code)
	if cfg.Code == "" {
		return fmt.Errorf("la configuración no dice a qué imprenta pertenece")
	}
	crudo, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	cuerpo := string(crudo)

	var existe int
	if err := db.QueryRow(`SELECT COUNT(*) FROM digital_invoice_config
		WHERE company_id=? AND addon_code=?`, tenant, cfg.Code).Scan(&existe); err != nil {
		return err
	}
	if existe > 0 {
		_, err = db.Exec(`UPDATE digital_invoice_config SET config_json=?, updated_at=CURRENT_TIMESTAMP
			WHERE company_id=? AND addon_code=?`, cuerpo, tenant, cfg.Code)
		return err
	}
	_, err = db.Exec(`INSERT INTO digital_invoice_config (company_id, addon_code, config_json, enabled, created_at)
		VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP)`, tenant, cfg.Code, cuerpo)
	if err != nil && diIsDupKey(err) {
		// Otra persona guardó la misma imprenta entre la consulta y el insert:
		// gana el último guardado, sin error en pantalla.
		_, err = db.Exec(`UPDATE digital_invoice_config SET config_json=?, updated_at=CURRENT_TIMESTAMP
			WHERE company_id=? AND addon_code=?`, cuerpo, tenant, cfg.Code)
	}
	return err
}

// diActivarEnBase deja activa UNA imprenta y desactiva las demás de la empresa.
// Devuelve los nombres de las que quedaron desactivadas, para avisarlo en
// pantalla sin que el usuario tenga que adivinarlo.
func diActivarEnBase(db *sql.DB, tenant, code string) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("base no disponible")
	}
	desactivadas, err := diCodigosActivos(db, tenant, code)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`UPDATE digital_invoice_config SET enabled=0
		WHERE company_id=? AND addon_code<>? AND enabled=1`, tenant, code); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`UPDATE digital_invoice_config SET enabled=1
		WHERE company_id=? AND addon_code=?`, tenant, code); err != nil {
		return nil, err
	}
	nombres := make([]string, 0, len(desactivadas))
	for _, c := range desactivadas {
		nombres = append(nombres, diNombreImprenta(c))
	}
	return nombres, nil
}

// diDesactivarEnBase apaga UNA imprenta sin tocar las demás.
func diDesactivarEnBase(db *sql.DB, tenant, code string) error {
	if db == nil {
		return fmt.Errorf("base no disponible")
	}
	_, err := db.Exec(`UPDATE digital_invoice_config SET enabled=0
		WHERE company_id=? AND addon_code=?`, tenant, code)
	return err
}

// diDesactivarTodasEnBase deja la empresa facturando por impresora fiscal.
func diDesactivarTodasEnBase(db *sql.DB, tenant string) error {
	if db == nil {
		return fmt.Errorf("base no disponible")
	}
	_, err := db.Exec(`UPDATE digital_invoice_config SET enabled=0 WHERE company_id=?`, tenant)
	return err
}

// diCodigosActivos lista las imprentas activas de la empresa salvo `excepto`.
func diCodigosActivos(db *sql.DB, tenant, excepto string) ([]string, error) {
	rows, err := db.Query(`SELECT addon_code FROM digital_invoice_config
		WHERE company_id=? AND enabled=1 AND addon_code<>?`, tenant, excepto)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codigos []string
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil && c != "" {
			codigos = append(codigos, c)
		}
	}
	return codigos, rows.Err()
}

// ---------- Ayudas de formulario ----------

// diImprentaDelFormulario resuelve la imprenta que trae la petición y contesta
// el error si no está en el catálogo.
func (s *Server) diImprentaDelFormulario(r *http.Request, w http.ResponseWriter) (string, diImprenta, bool) {
	code := strings.TrimSpace(r.FormValue("provider_code"))
	if code == "" {
		code = strings.TrimSpace(r.FormValue("code"))
	}
	if code == "" {
		jsonError(w, "Falta la imprenta (provider_code)", http.StatusBadRequest)
		return "", diImprenta{}, false
	}
	imp, ok := diImprentaPorCodigo(code)
	if !ok {
		jsonError(w, "La imprenta «"+code+"» no está en el catálogo", http.StatusBadRequest)
		return "", diImprenta{}, false
	}
	return code, imp, true
}

// camposDelFormulario recoge solo los campos que declara el catálogo para esa
// imprenta: así un formulario no puede escribir un campo que no existe.
func camposDelFormulario(r *http.Request, imp diImprenta) map[string]string {
	salida := make(map[string]string, len(imp.Campos))
	for _, campo := range imp.Campos {
		salida[campo.Nombre] = r.FormValue(campo.Nombre)
	}
	return salida
}
