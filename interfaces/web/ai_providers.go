package web

// ---------- AI providers (multi-provider, OpenAI-compatible chat completions) ----------

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// aiNoProviderMsg is the error shown when nothing usable is configured.
	aiNoProviderMsg = "No hay ningún proveedor de IA activo. Ve a Configuración IA y activa o añade uno."
	// aiOpenAIBaseURL is the endpoint used by the legacy single-row ai_setting.
	aiOpenAIBaseURL = "https://api.openai.com/v1"
	// aiDefaultModel was the fallback for a row with no model; per-vendor
	// defaults now live in ai_rules.go (aiModelPorDefecto), because ORC24's
	// default is `orc24` and an OpenAI default of gpt-4o-mini would contradict
	// the curated catalog (small models are not offered).
	aiTimeout = 60 * time.Second
	aiMaxBody = 8 << 20
)

// aiProvider mirrors one row of ai_providers. La URL base SIEMPRE se resuelve
// desde la whitelist compilada (aiVendorURL, ai_rules.go) según `vendor`: una
// base_url guardada en la BD se ignora por completo (seguridad: el cliente no
// puede apuntar el ERP a otra API).
type aiProvider struct {
	ID             int64
	Name           string
	Vendor         string
	VendorOrigen   string // valor CRUDO de la columna vendor (ver aiVendorDeFila)
	VendorLabel    string // nombre legible del proveedor ("OpenAI"), derivado de Vendor
	BaseURL        string // URL base (derivada del vendor o leída de la BD para "personalizado")
	EditableURL    bool   // true si la URL es editable (vendor "personalizado")
	APIKey         string
	Model          string
	Temperature    float64
	MaxTokens      int
	PromptTemplate string
	IsActive       bool
}

// ---------- Reading ----------

// loadAIProviders lists every provider, active first.
//
// Recibe la sesión y no la petición: la lista se lee tanto desde la pantalla de
// configuración como desde la capa de IA, que no tiene petición. Resolver por
// sesión cubre los dos casos con un solo objeto.
func (s *Server) loadAIProviders(sess *SessionData) []aiProvider {
	db := s.dbForSession(sess)
	rows, err := db.Query(`SELECT id, IFNULL(name,''), IFNULL(base_url,''), IFNULL(api_key,''),
		IFNULL(model,''), IFNULL(temperature,0.7), IFNULL(max_tokens,400), IFNULL(prompt_template,''),
		IFNULL(is_active,0), IFNULL(vendor,'')
		FROM ai_providers ORDER BY is_active DESC, name ASC`)
	if err != nil {
		// Columna vendor aún no migrada: reintento sin ella.
		rows2, err2 := db.Query(`SELECT id, IFNULL(name,''), IFNULL(base_url,''), IFNULL(api_key,''),
			IFNULL(model,''), IFNULL(temperature,0.7), IFNULL(max_tokens,400), IFNULL(prompt_template,''),
			IFNULL(is_active,0)
			FROM ai_providers ORDER BY is_active DESC, name ASC`)
		if err2 != nil {
			return []aiProvider{}
		}
		list := []aiProvider{}
		for rows2.Next() {
			var p aiProvider
			var active int
			if err := rows2.Scan(&p.ID, &p.Name, &p.BaseURL, &p.APIKey, &p.Model,
				&p.Temperature, &p.MaxTokens, &p.PromptTemplate, &active); err != nil {
				continue
			}
			// Sin columna vendor la fila es de la época en que solo existía OpenAI.
			p.Vendor = "openai"
			p.VendorOrigen = "openai"
			p.VendorLabel = aiVendorLabel[p.Vendor]
			p.BaseURL = aiVendorURL(p.Vendor)
			p.Temperature = aiRoundTemperature(p.Temperature)
			p.IsActive = active == 1
			list = append(list, p)
		}
		rows2.Close()
		return list
	}
	defer rows.Close()

	list := []aiProvider{}
	for rows.Next() {
		var p aiProvider
		var active int
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.APIKey, &p.Model,
			&p.Temperature, &p.MaxTokens, &p.PromptTemplate, &active, &p.Vendor); err != nil {
			continue
		}
		p.VendorOrigen = strings.TrimSpace(p.Vendor)
		p.Vendor = aiNormalizeVendor(p.Vendor)
		p.VendorLabel = aiVendorLabel[p.Vendor]
		p.EditableURL = aiVendorEditableURL(p.Vendor)
		// Para vendor 'personalizado' la URL viene de la BD (editable).
		if !p.EditableURL {
			p.BaseURL = aiVendorURL(p.Vendor)
		}
		p.Temperature = aiRoundTemperature(p.Temperature)
		p.IsActive = active == 1
		list = append(list, p)
	}
	return list
}

// aiProveedoresOficiales devuelve EXACTAMENTE los tres proveedores del
// sistema —ORC24, OpenAI y Anthropic— en el orden de la pantalla, cada uno con
// la fila de la base que le corresponda y con la fila VACÍA si esa marca
// todavía no se configuró.
//
// Por qué no se sirve la tabla tal cual: la tabla es un almacén, no el
// catálogo. Una instalación trae filas heredadas con marcas y nombres que este
// sistema ya no admite, y leerlas directamente es exactamente lo que hacía
// parecer en Configuración IA y en el formulario de agentes que había una
// decena de proveedores entre los que elegir. El catálogo está compilado
// (aiOrdenVendors); la base solo aporta la clave y el modelo de cada marca.
func (s *Server) aiProveedoresOficiales(sess *SessionData) []aiProvider {
	porVendor := map[string]aiProvider{}
	for _, p := range s.loadAIProviders(sess) {
		v := aiVendorDeFila(p.VendorOrigen)
		if v == "" {
			continue // fila heredada de una marca fuera de la whitelist
		}
		p.Vendor = v
		p.VendorLabel = aiVendorLabel[v]
		p.BaseURL = aiVendorURL(v)
		p.EditableURL = aiVendorEditableURL(v)
		// Dos filas para la misma marca: manda la activa y, a igualdad, la
		// primera que se lea —loadAIProviders ya ordena activas primero—.
		if guardada, existe := porVendor[v]; !existe || (p.IsActive && !guardada.IsActive) {
			porVendor[v] = p
		}
	}

	salida := make([]aiProvider, 0, len(aiOrdenVendors))
	for _, v := range aiOrdenVendors {
		if p, ok := porVendor[v]; ok {
			salida = append(salida, p)
			continue
		}
		// Marca sin registrar: existe en pantalla como «sin configurar», con su
		// modelo por defecto ya puesto en el formulario.
		salida = append(salida, aiProvider{
			Vendor: v, VendorLabel: aiVendorLabel[v], BaseURL: aiVendorURL(v),
			EditableURL: aiVendorEditableURL(v),
			Model: aiModelPorDefecto(v),
		})
	}
	return salida
}

// loadActiveProviderRow returns the single row marked is_active = 1.
func (s *Server) loadActiveProviderRow(sess *SessionData) (aiProvider, bool) {
	db := s.dbForSession(sess)
	var p aiProvider
	var active int
	err := db.QueryRow(`SELECT id, IFNULL(name,''), IFNULL(api_key,''),
		IFNULL(model,''), IFNULL(temperature,0.7), IFNULL(max_tokens,400), IFNULL(prompt_template,''),
		IFNULL(is_active,0), IFNULL(vendor,'')
		FROM ai_providers WHERE is_active = 1 ORDER BY id LIMIT 1`).
		Scan(&p.ID, &p.Name, &p.APIKey, &p.Model,
			&p.Temperature, &p.MaxTokens, &p.PromptTemplate, &active, &p.Vendor)
	if err != nil {
		// Sin columna vendor (BD aún no migrada): consulta legada.
		var legacy aiProvider
		err2 := db.QueryRow(`SELECT id, IFNULL(name,''), IFNULL(api_key,''),
			IFNULL(model,''), IFNULL(temperature,0.7), IFNULL(max_tokens,400), IFNULL(prompt_template,''),
			IFNULL(is_active,0)
			FROM ai_providers WHERE is_active = 1 ORDER BY id LIMIT 1`).
			Scan(&legacy.ID, &legacy.Name, &legacy.APIKey, &legacy.Model,
				&legacy.Temperature, &legacy.MaxTokens, &legacy.PromptTemplate, &active)
		if err2 != nil {
			return aiProvider{}, false
		}
		legacy.Vendor = "openai"
		legacy.BaseURL = aiVendorURL(legacy.Vendor)
		legacy.Temperature = aiRoundTemperature(legacy.Temperature)
		legacy.IsActive = active == 1
		return legacy, true
	}
	p.Vendor = aiNormalizeVendor(p.Vendor)
	p.BaseURL = aiVendorURL(p.Vendor)
	p.Temperature = aiRoundTemperature(p.Temperature)
	p.IsActive = active == 1
	return p, true
}

// loadActiveProvider resolves the provider that serves /ai/analyze:
//
//  1. the active row of ai_providers (activate/ leaves exactly one active);
//  2. when there is NO active row and the legacy ai_setting has an api_key
//     (database migrated from the PHP ERP), that key is used through an
//     implicit OpenAI provider, so old installs keep working untouched;
//  3. an active row with no credentials (remote endpoint, no legacy key to
//     inherit) is reported as "not configured" instead of firing a request
//     that is guaranteed to fail with HTTP 401.
//
// Step 3 never applies to local endpoints (Ollama, LM Studio), which need no
// API key at all.
func (s *Server) loadActiveProvider(sess *SessionData) (aiProvider, bool) {
	legacy := s.loadAISetting(sess)
	legacyKey := strings.TrimSpace(legacy.APIKey)

	p, found := s.loadActiveProviderRow(sess)
	if !found {
		if legacyKey == "" {
			return aiProvider{}, false
		}
		if strings.TrimSpace(legacy.Model) == "" {
			legacy.Model = aiModelPorDefecto("openai")
		}
		return aiProvider{
			Name:           "OpenAI (configuración heredada)",
			Vendor:         "openai",
			BaseURL:        aiVendorURL("openai"),
			APIKey:         legacyKey,
			Model:          legacy.Model,
			Temperature:    aiRoundTemperature(legacy.Temperature),
			MaxTokens:      legacy.MaxTokens,
			PromptTemplate: legacy.PromptTemplate,
		}, true
	}

	// The legacy key is an OpenAI key: reuse it only for the OpenAI endpoint,
	// never to authenticate a different vendor.
	if strings.TrimSpace(p.APIKey) == "" && legacyKey != "" && p.Vendor == "openai" {
		p.APIKey = legacyKey
	}
	if !aiProviderUsable(p) {
		return p, false
	}
	return p, true
}

// aiProviderUsable reports whether a provider can be called as-is: con la
// whitelist compilada la condición es simplemente tener API key.
func aiProviderUsable(p aiProvider) bool {
	return strings.TrimSpace(p.APIKey) != "" && aiVendorURL(p.Vendor) != ""
}

// aiNoProviderReason explains WHY no provider can answer, so the user is not
// told "there is no active provider" when one is active but still lacks its
// API key (the most common case right after picking a new vendor).
func (s *Server) aiNoProviderReason(sess *SessionData) string {
	if p, found := s.loadActiveProviderRow(sess); found {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = fmt.Sprintf("#%d", p.ID)
		}
		return fmt.Sprintf("El proveedor activo \u00ab%s\u00bb no tiene API key configurada. "+
			"Ed\u00edtalo en Configuraci\u00f3n IA.", name)
	}
	return aiNoProviderMsg
}

// aiRoundTemperature keeps 0..2 and trims the float32 noise coming from the
// FLOAT column (0.7 would otherwise render as 0.699999988079071).
func aiRoundTemperature(v float64) float64 {
	if v < 0 || v > 2 {
		return 0.7
	}
	return math.Round(v*100) / 100
}

// ---------- URL helpers ----------

// aiCompletionsEndpoint appends /chat/completions to the base URL, keeping a
// single slash between them (and tolerating a base that already carries it).
func aiCompletionsEndpoint(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(b), "/chat/completions") {
		return b
	}
	return b + "/chat/completions"
}

// aiEndpointHost returns the lower-cased host of a base URL ("" when invalid).
func aiEndpointHost(base string) string {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// aiIsLocalEndpoint reports whether the endpoint runs on this machine.
func aiIsLocalEndpoint(base string) bool {
	switch aiEndpointHost(base) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	}
	return false
}

// aiIsOpenAIEndpoint reports whether the endpoint is api.openai.com.
func aiIsOpenAIEndpoint(base string) bool {
	return aiEndpointHost(base) == "api.openai.com"
}

// aiValidateBaseURL enforces http(s) + host on the user supplied base URL.
func aiValidateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("La URL base no es válida")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("La URL base debe empezar por http:// o https://")
	}
	if u.Host == "" {
		return fmt.Errorf("La URL base no contiene un host válido")
	}
	return nil
}

// ---------- Endpoints ----------

// aiProviderList responde GET /ai/providers: lista de proveedores para la tabla
// de Configuración. Inuye los oficiales (aunque no estén en la BD) CUALQUIER
// proveedor personalizado creado por el usuario. Así la tabla muestra todo lo
// que existe, y el frontend distingue oficiales de personalizados por el vendor.
func (s *Server) aiProviderList(w http.ResponseWriter, r *http.Request) {
	// 1. Oficiales (siempre presentes, vengan o no de la BD).
	oficiales := s.aiProveedoresOficiales(s.sessionFromRequest(r))
	porID := map[int64]map[string]interface{}{}
	for _, p := range oficiales {
		if p.ID <= 0 {
			continue
		}
		porID[p.ID] = map[string]interface{}{
			"id": p.ID, "name": p.Name, "vendor": p.Vendor,
			"vendor_label": aiVendorLabel[p.Vendor],
			"base_url":   p.BaseURL,
			"has_key":    strings.TrimSpace(p.APIKey) != "",
			"model":      p.Model, "temperature": p.Temperature,
			"max_tokens": p.MaxTokens, "is_active": p.IsActive,
			"editable_url": aiVendorEditableURL(p.Vendor),
		}
	}
	// 2. Cargar TODOS los proveedores de la BD (incluye personalizados).
	todos := s.loadAIProviders(s.sessionFromRequest(r))
	out := make([]map[string]interface{}, 0, len(todos)+len(oficiales))
	// Agregar oficiales primero.
	for _, p := range oficiales {
		out = append(out, map[string]interface{}{
			"id": p.ID, "name": p.Name, "vendor": p.Vendor,
			"vendor_label": aiVendorLabel[p.Vendor],
			"base_url":   p.BaseURL,
			"has_key":    strings.TrimSpace(p.APIKey) != "",
			"model":      p.Model, "temperature": p.Temperature,
			"max_tokens": p.MaxTokens, "is_active": p.IsActive,
			"editable_url": aiVendorEditableURL(p.Vendor),
		})
	}
	// Agregar los que NO son oficiales (personalizados y otros ya existentes).
	vendorsOficiales := map[string]bool{}
	for _, v := range aiOrdenVendors {
		vendorsOficiales[v] = true
	}
	for _, p := range todos {
		if vendorsOficiales[p.Vendor] {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":           p.ID,
			"name":         p.Name,
			"vendor":       p.Vendor,
			"vendor_label": aiVendorLabel[p.Vendor],
			"base_url":     p.BaseURL,
			"has_key":      strings.TrimSpace(p.APIKey) != "",
			"model":        p.Model,
			"temperature":  p.Temperature,
			"max_tokens":   p.MaxTokens,
			"is_active":    p.IsActive,
			"editable_url": aiVendorEditableURL(p.Vendor),
		})
	}
	writeJSON(w, out)
}

// ---------- Modelos ----------

// aiModelsList answers POST /ai/models: qué modelos puede elegir el usuario
// para un vendor.
//
// Tres reglas, en el orden en que se aplican:
//
//  1. NUNCA devuelve la URL de un proveedor. El endpoint es un dato del
//     binario —se resuelve con aiVendorURL— y la pantalla solo tiene que
//     mostrar modelos, no dónde están.
//  2. ORC24 responde del catálogo compilado y SIN salir a la red: es nuestra
//     infraestructura y su modelo único es `orc24`.
//  3. OpenAI y Anthropic se consultan (GET /models con la clave que traiga el
//     formulario, o la guardada del proveedor si se está editando) y el
//     resultado se CRUZA con el catálogo curado: solo lo que el proveedor
//     confirma vivo Y el catálogo aprueba. Si la consulta falla —sin clave,
//     sin red, proveedor caído— se devuelve el catálogo con `source=catalogo`
//     y el motivo, en vez de dejar el formulario sin modelos que ofrecer.
func (s *Server) aiModelsList(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		aiJSONError(w, "Sesión inválida", 401)
		return
	}
	if err := r.ParseForm(); err != nil {
		aiJSONError(w, "Formulario inválido", 400)
		return
	}
	vendor := aiNormalizeVendor(r.FormValue("vendor"))
	respuesta := map[string]interface{}{
		"status":       "success",
		"vendor":       vendor,
		"vendor_label": aiVendorLabel[vendor],
		"models":       aiModelsDeVendor(vendor),
		"source":       "catalogo",
	}

	if vendor == aiDefaultVendor {
		// ORC24: hardcodeado a propósito — sin consulta, sin clave, sin red.
		writeJSON(w, respuesta)
		return
	}

	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if apiKey == "" {
		// Editando un proveedor ya guardado: la clave no viaja en el formulario
		// (la BD no la devuelve por JSON), así que se lee aquí, en el servidor.
		if pid, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("provider_id")), 10, 64); err == nil && pid > 0 {
			_ = s.dbFor(r).QueryRow(`SELECT IFNULL(api_key,'') FROM ai_providers WHERE id = ?`, pid).Scan(&apiKey)
		}
	}
	if apiKey == "" {
		respuesta["nota"] = "Sin API key no se pudo consultar la lista del proveedor: se muestran los modelos del catálogo."
		writeJSON(w, respuesta)
		return
	}

	remotos, err := aiModelosRemoto(vendor, apiKey)
	if err != nil {
		respuesta["nota"] = "No se pudo consultar la lista de modelos (" + err.Error() + "): se muestran los del catálogo."
		writeJSON(w, respuesta)
		return
	}
	aprobados := aiFiltrarModelos(aiModelsDeVendor(vendor), remotos)
	if len(aprobados) == 0 {
		// El proveedor respondió, pero nada de lo que ofrece coincide con lo que
		// este sistema aprueba: el catálogo sigue siendo la oferta.
		respuesta["nota"] = "El proveedor no devuelve ninguno de los modelos aprobados: se muestran los del catálogo."
		writeJSON(w, respuesta)
		return
	}
	respuesta["models"] = aprobados
	respuesta["source"] = "remoto"
	writeJSON(w, respuesta)
}

// aiModelosRemoto consulta GET {vendor}/models y devuelve sus ids.
//
// Los errores que salen de aquí son SIEMPRE genéricos: van a una pantalla, y ni
// ahí ni en el log de la respuesta debe aparecer la URL del proveedor ni nada
// que la contenga (un error de red de Go incluye el endpoint completo).
func aiModelosRemoto(vendor, apiKey string) ([]string, error) {
	base := aiVendorURL(vendor)
	if base == "" {
		return nil, fmt.Errorf("proveedor no permitido")
	}
	endpoint := strings.TrimRight(base, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo preparar la consulta")
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client, err := aiGuardedClient(endpoint, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("no se pudo validar la conexión con el proveedor")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo contactar con el proveedor")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxBody))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta del proveedor")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 401, 403:
			return nil, fmt.Errorf("la API key fue rechazada")
		default:
			return nil, fmt.Errorf("el proveedor contestó HTTP %d", resp.StatusCode)
		}
	}
	// El sobre es el de OpenAI —y lo es también en la capa compatible de
	// Anthropic y en ORC24: {"data":[{"id":...}]}. No se distingue vendor
	// porque el formato es el mismo y una rama por proveedor sería código que
	// nadie prueba.
	var lista struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &lista); err != nil {
		return nil, fmt.Errorf("respuesta ilegible del proveedor")
	}
	ids := make([]string, 0, len(lista.Data))
	for _, m := range lista.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// aiFiltrarModelos devuelve los modelos del catálogo que además están vivos en
// la lista remota. El orden es el del CATÁLOGO: la oferta manda, la lista
// remota solo confirma que el modelo sigue existiendo.
func aiFiltrarModelos(catalogo, remotos []string) []string {
	vivos := make(map[string]bool, len(remotos))
	for _, m := range remotos {
		vivos[strings.ToLower(strings.TrimSpace(m))] = true
	}
	salida := []string{}
	for _, m := range catalogo {
		if vivos[strings.ToLower(m)] {
			salida = append(salida, m)
		}
	}
	return salida
}

// aiProviderSave upserts a provider (id = 0 creates a new one).
func (s *Server) aiProviderSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		aiJSONError(w, "Formulario inválido", 400)
		return
	}
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	name := strings.TrimSpace(r.FormValue("name"))
	// La URL base se deriva del vendor (whitelist compilada), excepto para
	// vendor "personalizado" donde el usuario escribe la URL completa.
	vendor := aiNormalizeVendor(r.FormValue("vendor"))
	baseURL := aiVendorURL(vendor)
	if aiVendorEditableURL(vendor) {
		// Personalizado: la URL viene del formulario.
		baseURL = strings.TrimSpace(r.FormValue("base_url"))
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	model := strings.TrimSpace(r.FormValue("model"))
	promptTemplate := strings.TrimSpace(r.FormValue("prompt_template"))

	if name == "" {
		aiJSONError(w, "El nombre es obligatorio", 400)
		return
	}
	if baseURL == "" {
		if aiVendorEditableURL(vendor) {
			aiJSONError(w, "La URL base es obligatoria para el proveedor personalizado", 400)
		} else {
			aiJSONError(w, "Proveedor no permitido (use orc24 u openai)", 400)
		}
		return
	}
	if model == "" {
		aiJSONError(w, "El modelo es obligatorio", 400)
		return
	}

	// UN registro por marca: la pantalla es una fila fija por proveedor. Un
	// segundo registro de OpenAI dejaría dos formas de configurar la misma cosa
	// y dos filas peleando por estar activas, que es justo el lío que hoy se ve.
	var repetido int
	if id > 0 {
		_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM ai_providers WHERE vendor = ? AND id <> ?`, vendor, id).Scan(&repetido)
	} else {
		_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM ai_providers WHERE vendor = ?`, vendor).Scan(&repetido)
	}
	if repetido > 0 {
		aiJSONError(w, "Ya existe un proveedor "+aiVendorLabel[vendor]+
			"; edítalo desde su fila en vez de crear otro registro.", 400)
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

	if id > 0 {
		var found int
		_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM ai_providers WHERE id = ?`, id).Scan(&found)
		if found == 0 {
			aiJSONError(w, "No se encontró el proveedor", 404)
			return
		}
		_, err := s.dbFor(r).Exec(`UPDATE ai_providers SET name=?, vendor=?, base_url=?, api_key=?, model=?,
			temperature=?, max_tokens=?, prompt_template=? WHERE id=?`,
			name, vendor, baseURL, apiKey, model, temperature, maxTokens, promptTemplate, id)
		if err != nil {
			aiJSONError(w, "No se pudo guardar el proveedor: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]interface{}{"status": "success", "msg": "Proveedor actualizado", "id": id})
		return
	}

	res, err := s.dbFor(r).Exec(`INSERT INTO ai_providers
		(name, vendor, base_url, api_key, model, temperature, max_tokens, prompt_template, is_active)
		VALUES (?,?,?,?,?,?,?,?,0)`,
		name, vendor, baseURL, apiKey, model, temperature, maxTokens, promptTemplate)
	if err != nil {
		aiJSONError(w, "No se pudo crear el proveedor: "+err.Error(), 500)
		return
	}
	newID, _ := res.LastInsertId()
	writeJSON(w, map[string]interface{}{"status": "success", "msg": "Proveedor creado", "id": newID})
}

// aiProviderDelete removes a provider; the active one cannot be deleted.
func (s *Server) aiProviderDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := aiProviderID(w, r)
	if !ok {
		return
	}
	var found, active int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*), IFNULL(MAX(is_active),0) FROM ai_providers WHERE id = ?`, id).
		Scan(&found, &active)
	if found == 0 {
		aiJSONError(w, "No se encontró el proveedor", 404)
		return
	}
	if active == 1 {
		aiJSONError(w, "No se puede eliminar el proveedor activo. Activa otro proveedor primero.", 400)
		return
	}
	if _, err := s.dbFor(r).Exec(`DELETE FROM ai_providers WHERE id = ?`, id); err != nil {
		aiJSONError(w, "No se pudo eliminar el proveedor: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "success", "msg": "Proveedor eliminado"})
}

// aiProviderActivate marks one provider as active and deactivates the rest.
func (s *Server) aiProviderActivate(w http.ResponseWriter, r *http.Request) {
	id, ok := aiProviderID(w, r)
	if !ok {
		return
	}
	var found int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM ai_providers WHERE id = ?`, id).Scan(&found)
	if found == 0 {
		aiJSONError(w, "No se encontró el proveedor", 404)
		return
	}
	if _, err := s.dbFor(r).Exec(`UPDATE ai_providers SET is_active = IF(id = ?, 1, 0)`, id); err != nil {
		aiJSONError(w, "No se pudo activar el proveedor: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "success", "msg": "Proveedor activado"})
}

// aiProviderID reads the {id} path segment of a provider route.
func aiProviderID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil || id <= 0 {
		aiJSONError(w, "Identificador de proveedor inválido", 400)
		return 0, false
	}
	return id, true
}

// ---------- Chat completions client ----------

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

// openAIResponse is the shape shared by OpenAI and every compatible vendor.
// Error is kept raw because vendors disagree: it can be a string, an object
// with "message" or any other payload.
//
// Usage es opcional: un vendor OpenAI-compatible puede no devolverla, y en ese
// caso el metering registra tokens 0 en vez de inventar una estimación.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
	Detail  json.RawMessage `json:"detail"`
}

// aiTokenUsage son los tokens que reportó el proveedor en una llamada.
type aiTokenUsage struct {
	Prompt     int
	Completion int
	Model      string
}

// callChatCompletion posts the prompt to <base_url>/chat/completions. The API
// key is only sent in the Authorization header (and only when the provider
// has one: local servers like Ollama need none); it is never logged back.
//
// El destino lo decide aiRespanDestino (ai_respan.go): con RESPAN_API_KEY en el
// entorno y un proveedor de terceros, la llamada sale por la pasarela de Respan
// y se autentica con esa clave; en cualquier otro caso va directa al proveedor.
//
// Devuelve además el uso reportado por el proveedor (tokens), que el caller
// pasa al metering. Es una función libre (sin *Server) porque la capa de IA la
// usa desde sitios sin petición; por eso el registro de consumo lo hace quien
// llama, que sí tiene la base de la empresa.
func callChatCompletion(p aiProvider, prompt string) (string, aiTokenUsage, error) {
	baseURL := strings.TrimSpace(p.BaseURL)
	if baseURL == "" {
		return "", aiTokenUsage{}, fmt.Errorf("El proveedor %q no tiene una URL base. Edítalo en Configuración IA.", p.Name)
	}
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return "", aiTokenUsage{}, fmt.Errorf("El proveedor %q no tiene un modelo configurado. Edítalo en Configuración IA.", p.Name)
	}
	temperature := aiRoundTemperature(p.Temperature)
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 400
	}
	systemPrompt := strings.TrimSpace(p.PromptTemplate)
	if systemPrompt == "" {
		systemPrompt = "You are a business analyst."
	}

	body, err := json.Marshal(openAIRequest{
		Model: model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", aiTokenUsage{}, fmt.Errorf("No se pudo preparar la petición a %q: %v", p.Name, err)
	}

	endpoint, apiKey, _ := aiRespanDestino(p)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", aiTokenUsage{}, fmt.Errorf("No se pudo preparar la petición a %q: %v", p.Name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client, err := aiGuardedClient(endpoint, aiTimeout)
	if err != nil {
		return "", aiTokenUsage{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", aiTokenUsage{}, fmt.Errorf("Error de red al consultar %q: %v", p.Name, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxBody))
	if err != nil {
		return "", aiTokenUsage{}, fmt.Errorf("No se pudo leer la respuesta de %q: %v", p.Name, err)
	}

	var parsed openAIResponse
	_ = json.Unmarshal(raw, &parsed)

	usage := aiTokenUsage{
		Prompt:     parsed.Usage.PromptTokens,
		Completion: parsed.Usage.CompletionTokens,
		Model:      model,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", usage, fmt.Errorf("El proveedor %q devolvió un error (HTTP %d): %s",
			p.Name, resp.StatusCode, aiErrorText(raw, parsed))
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", usage, fmt.Errorf("El proveedor %q devolvió una respuesta vacía", p.Name)
	}
	return parsed.Choices[0].Message.Content, usage, nil
}

// aiErrorText extracts the vendor's own error message so the user can tell a
// bad key from a bad URL or an exhausted quota.
func aiErrorText(raw []byte, parsed openAIResponse) string {
	if len(parsed.Error) > 0 {
		var asString string
		if json.Unmarshal(parsed.Error, &asString) == nil && strings.TrimSpace(asString) != "" {
			return aiTrimError(asString)
		}
		var asObject struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(parsed.Error, &asObject) == nil && strings.TrimSpace(asObject.Message) != "" {
			return aiTrimError(asObject.Message)
		}
	}
	if strings.TrimSpace(parsed.Message) != "" {
		return aiTrimError(parsed.Message)
	}
	if len(parsed.Detail) > 0 {
		var asString string
		if json.Unmarshal(parsed.Detail, &asString) == nil && strings.TrimSpace(asString) != "" {
			return aiTrimError(asString)
		}
		var asObject struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(parsed.Detail, &asObject) == nil && strings.TrimSpace(asObject.Message) != "" {
			return aiTrimError(asObject.Message)
		}
	}
	return aiTrimError(string(raw))
}

func aiTrimError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "(sin detalle)"
	}
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	return msg
}
