package web

// ai_providers_catalog_test.go — las reglas de negocio del catálogo de IA.
//
// POR QUÉ ESTAS TRES COSAS SE PRUEBAN JUNTAS.
//
// El sistema comercia inferencia: qué modelos se ofrecen es una decisión de
// negocio (no un detalle de implementación), la URL de cada proveedor es un
// dato del binario que ninguna pantalla debe ver, y ORC24 es infraestructura
// propia que no consulta a nadie. Si alguna de las tres se rompe, el cambio se
// ve «solo» en la UI: el formulario empieza a ofrecer modelos que no se
// facturan, o una respuesta JSON filtra dónde está el endpoint.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestElCatalogoDeModelosCumpleLasTresReglas fija la oferta por proveedor:
// ORC24 un único modelo, OpenAI sin los pequeños y Anthropic sin Haiku.
func TestElCatalogoDeModelosCumpleLasTresReglas(t *testing.T) {
	// ORC24: un modelo, el nuestro, y cae él solo cuando el vendor de la BD
	// venga vacío o con un valor heredado.
	if got := aiModelsDeVendor("orc24"); len(got) != 1 || got[0] != "orc24" {
		t.Errorf("ORC24 debe ofrecer únicamente «orc24», ofreció %v", got)
	}
	if v := aiNormalizeVendor(""); v != aiDefaultVendor {
		t.Errorf("un vendor vacío debe normalizarse a %q, salió %q", aiDefaultVendor, v)
	}

	// OpenAI: fuera los pequeños —mini, nano y la línea económica Luna—. No
	// se comprueba el nombre exacto de los grandes (esos los aporta el catálogo
	// curado); se comprueba que NADA pequeño se cuela.
	openai := strings.ToLower(strings.Join(aiModelsDeVendor("openai"), " "))
	if openai == "" {
		t.Fatal("el catálogo de OpenAI está vacío: el formulario no ofrecería nada")
	}
	for _, excluido := range []string{"mini", "nano", "luna"} {
		if strings.Contains(openai, excluido) {
			t.Errorf("el catálogo de OpenAI contiene un modelo pequeño (%q): %s", excluido, openai)
		}
	}

	// Anthropic: solo los emblemáticos. Haiku es la línea rápida y barata y
	// queda fuera por el mismo criterio que mini/nano en OpenAI.
	anthropic := strings.ToLower(strings.Join(aiModelsDeVendor("anthropic"), " "))
	if anthropic == "" {
		t.Fatal("el catálogo de Anthropic está vacío: el formulario no ofrecería nada")
	}
	if strings.Contains(anthropic, "haiku") {
		t.Errorf("el catálogo de Anthropic no debe contener Haiku: %s", anthropic)
	}
	if !strings.Contains(anthropic, "opus") || !strings.Contains(anthropic, "sonnet") {
		t.Errorf("Anthropic debe ofrecer Opus y Sonnet: %s", anthropic)
	}
}

// TestLaListaDeModelosNoExponeLaUrlDelProveedor pasa el handler real y
// comprueba que en la respuesta no aparece NI UNA URL —ni la compilada ni
// ninguna otra—. El error típico es que un mensaje incluya el endpoint que
// falló; por eso el «no http» se verifica sobre el cuerpo completo.
func TestLaListaDeModelosNoExponeLaUrlDelProveedor(t *testing.T) {
	s := &Server{sessions: NewSessionStore()}

	casos := []struct {
		vendor string
		modelo string // algún modelo que DEBE aparecer
	}{
		{"orc24", "orc24"},
		{"openai", ""},   // sin clave: catálogo con nota, sin consultar
		{"anthropic", ""}, // igual: nunca sale a la red sin clave
	}
	for _, c := range casos {
		form := url.Values{"vendor": {c.vendor}}
		r := withScope(httptest.NewRequest("POST", "/ai/models", strings.NewReader(form.Encode())),
			&scope{tenant: "test", sess: &SessionData{Username: "contador"}})
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.aiModelsList(w, r)

		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, `"status":"success"`) {
			t.Fatalf("vendor %s: la lista falló (%d): %s", c.vendor, w.Code, body)
		}
		if strings.Contains(strings.ToLower(body), "http") {
			t.Errorf("vendor %s: la respuesta filtra una URL: %s", c.vendor, body)
		}
		if c.modelo != "" && !strings.Contains(body, c.modelo) {
			t.Errorf("vendor %s: falta el modelo %q: %s", c.vendor, c.modelo, body)
		}
	}
}
