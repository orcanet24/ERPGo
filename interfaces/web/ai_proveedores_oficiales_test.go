package web

// ai_proveedores_oficiales_test.go — el catálogo de proveedores que ve la gente.
//
// POR QUÉ ESTAS COSAS SE PRUEBAN JUNTAS.
//
// Configuración IA y el formulario de agentes servían las filas de ai_providers
// tal cual, y una instalación con filas heredadas enseñaba una decena de
// «proveedores» entre los que elegir —sin que apareciera siquiera ORC24, que es
// la marca propia—. Ese lío no es cosmético: cada fila extra es una puerta para
// que un agente quede amarrado a un proveedor que nadie usa, y dos filas de la
// misma marca pelean por estar activas.
//
// Las reglas que aquí se defienden son tres y son de negocio:
//  1. El sistema tiene EXACTAMENTE tres proveedores, en un orden fijo.
//  2. Solo uno puede estar activo, y no se puede crear un segundo registro de
//     la misma marca.
//  3. Los agentes trabajan con el ACTIVO, no con la fila vieja que guardaron
//     al crearse.

import (
	"database/sql"
	"html/template"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"erpgo"

	_ "github.com/glebarez/go-sqlite"
)

// nuevaBaseDeProveedores abre una base en memoria con la tabla de proveedores.
// Una sola conexión: con varias, cada una tendría su propia :memory: y la
// prueba pasaría según el azar.
func nuevaBaseDeProveedores(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE ai_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		vendor TEXT,
		base_url TEXT NOT NULL DEFAULT '',
		api_key TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		temperature REAL NOT NULL DEFAULT 0.7,
		max_tokens INTEGER NOT NULL DEFAULT 400,
		prompt_template TEXT NOT NULL DEFAULT '',
		is_active INTEGER NOT NULL DEFAULT 0)`)
	if err != nil {
		t.Fatalf("no se pudo crear la tabla de proveedores: %v", err)
	}
	return db
}

// TestLaPantallaDeProveedoresEsLosTresYNadaMas es la regresión directa del
// problema reportado: filas heredadas y de marcas ajenas no aparecen en
// Configuración IA, y la marca propia SÍ, aunque nadie la haya registrado aún.
func TestLaPantallaDeProveedoresEsLosTresYNadaMas(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	for _, f := range []struct {
		nombre, vendor string
		activo         int
	}{
		{"Mi OpenAI", "openai", 1},
		{"Clave heredada de la migración", "", 0},
		{"Ollama del local", "ollama", 0},
		{"Anthropic laboral", "anthropic", 0},
	} {
		if _, err := db.Exec(`INSERT INTO ai_providers (name, vendor, api_key, model, is_active)
			VALUES (?,?, 'clave', 'gpt-5.5', ?)`, f.nombre, f.vendor, f.activo); err != nil {
			t.Fatalf("no se pudo sembrar la fila %q: %v", f.nombre, err)
		}
	}

	s := &Server{db: db}
	lista := s.aiProveedoresOficiales(nil)

	if len(lista) != 3 {
		t.Fatalf("la pantalla debe ofrecer exactamente 3 proveedores, ofreció %d: %+v", len(lista), lista)
	}
	orden := []string{"orc24", "openai", "anthropic"}
	for i, esperado := range orden {
		if lista[i].Vendor != esperado {
			t.Errorf("la posición %d debe ser %q y vino %q", i, esperado, lista[i].Vendor)
		}
	}
	if lista[0].ID != 0 || lista[0].IsActive {
		t.Errorf("ORC24 no está registrado: debe salir como pendiente, salió ID=%d activo=%v",
			lista[0].ID, lista[0].IsActive)
	}
	if lista[0].VendorLabel != "ORC24" {
		t.Errorf("ORC24 debe presentarse como «ORC24», salió %q", lista[0].VendorLabel)
	}
	// De OpenAI manda la fila activa, no la heredada de la migración.
	if lista[1].Name != "Mi OpenAI" || !lista[1].IsActive {
		t.Errorf("de OpenAI debe salir la fila activa «Mi OpenAI», salió %q (activo=%v)",
			lista[1].Name, lista[1].IsActive)
	}
	for _, p := range lista {
		if p.Vendor == "ollama" || strings.Contains(p.Name, "Ollama") {
			t.Errorf("se coló una fila heredada: %+v", p)
		}
	}

	// El JSON de /ai/providers es la misma oferta: si ahí volvieran las filas
	// heredadas, cualquier consumidor volvería a ofrecerlas.
	w := httptest.NewRecorder()
	r := withScope(httptest.NewRequest("GET", "/ai/providers", nil), &scope{db: db})
	s.aiProviderList(w, r)
	cuerpo := w.Body.String()
	if got := strings.Count(cuerpo, `"vendor_label"`); got != 3 {
		t.Errorf("el JSON debe devolver los 3 proveedores, devolvió %d: %s", got, cuerpo)
	}
	if strings.Contains(strings.ToLower(cuerpo), "ollama") {
		t.Errorf("el JSON filtra una fila heredada: %s", cuerpo)
	}
}

// TestNoSePuedeCrearUnSegundoRegistroDeLaMismaMarca cierra la puerta por la que
// aparecían filas de más: una marca, un registro.
func TestNoSePuedeCrearUnSegundoRegistroDeLaMismaMarca(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	s := &Server{db: db}

	guardar := func(vendor string) *httptest.ResponseRecorder {
		form := url.Values{
			"id": {"0"}, "name": {"Otro nombre"}, "vendor": {vendor},
			"api_key": {"sk-x"}, "model": {"gpt-5.5"},
		}
		r := withScope(httptest.NewRequest("POST", "/ai/providers/save", strings.NewReader(form.Encode())),
			&scope{db: db})
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.aiProviderSave(w, r)
		return w
	}

	if w := guardar("openai"); !strings.Contains(w.Body.String(), "success") {
		t.Fatalf("el primer registro de OpenAI debe crearse: %s", w.Body.String())
	}
	w := guardar("openai")
	if w.Code != 400 {
		t.Fatalf("el segundo registro de OpenAI debe rechazarse (400), salió %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "OpenAI") {
		t.Errorf("el rechazo debe decir qué marca ya existe: %s", w.Body.String())
	}

	// Una marca distinta sigue cabiendo: son dos de las tres permitidas.
	if w := guardar("anthropic"); !strings.Contains(w.Body.String(), "success") {
		t.Errorf("Anthropic es otra marca y debe poder registrarse: %s", w.Body.String())
	}
	if n := contarFilas(t, db, "ai_providers"); n != 2 {
		t.Errorf("deben quedar 2 registros (una por marca), quedaron %d", n)
	}
}

// TestElAgenteUsaElProveedorActivoYNoSuFilaVieja es la regresión de ejecución:
// la fila que guardó el agente al crearse no manda, manda el activo.
func TestElAgenteUsaElProveedorActivoYNoSuFilaVieja(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	if _, err := db.Exec(`INSERT INTO ai_providers (id, name, vendor, api_key, model, is_active)
		VALUES (1, 'Ollama del local', 'ollama', 'k-vieja', 'llama3', 0),
		       (2, 'ORC24', 'orc24', 'k-orc', 'orc24', 1)`); err != nil {
		t.Fatalf("no se pudieron sembrar las filas: %v", err)
	}
	s := &Server{db: db}

	p, ok := s.aiProviderForAgent(nil, aiAgent{ID: 7, ProviderID: 1})
	if !ok {
		t.Fatal("con un proveedor activo el agente debe poder trabajar")
	}
	if p.Vendor != "orc24" || p.APIKey != "k-orc" {
		t.Errorf("el agente debe usar el proveedor ACTIVO (orc24), usó vendor=%q clave=%q", p.Vendor, p.APIKey)
	}
	if p.BaseURL != aiVendorURL("orc24") {
		t.Errorf("la URL debe salir de la whitelist compilada: %q", p.BaseURL)
	}

	// Apagado el activo, el agente no trabaja con la fila vieja: se detiene.
	if _, err := db.Exec(`UPDATE ai_providers SET is_active = 0`); err != nil {
		t.Fatalf("no se pudo desactivar: %v", err)
	}
	if _, ok := s.aiProviderForAgent(nil, aiAgent{ID: 7, ProviderID: 1}); ok {
		t.Error("sin proveedor activo el agente no debe poder ejecutarse")
	}
}

// TestSinProveedorActivoNoSeCreaAgente comprueba el candado del formulario:
// sin proveedor activo no se da de alta un agente que no podría trabajar.
func TestSinProveedorActivoNoSeCreaAgente(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	if _, err := db.Exec(`INSERT INTO ai_providers (name, vendor, api_key, model, is_active)
		VALUES ('ORC24', 'orc24', 'k-orc', 'orc24', 0)`); err != nil {
		t.Fatalf("no se pudo sembrar la fila: %v", err)
	}
	s := &Server{db: db, sessions: NewSessionStore()}

	crear := func() *httptest.ResponseRecorder {
		form := url.Values{
			"name": {"Agente de prueba"}, "role_id": {"1"},
			"provider_id": {"9"}, // el valor del formulario se ignora
		}
		r := withScope(httptest.NewRequest("POST", "/ai/agents/save", strings.NewReader(form.Encode())),
			&scope{db: db})
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.agentSave(w, r)
		return w
	}

	w := crear()
	if w.Code != 400 || !strings.Contains(w.Body.String(), "proveedor de IA activo") {
		t.Fatalf("debe rechazarse por no haber proveedor activo (400); salió %d: %s", w.Code, w.Body.String())
	}

	// Con el proveedor activo se pasa ese candado y el siguiente que falla es
	// el rol (que esta base no tiene): la puerta de la IA ya quedó abierta.
	if _, err := db.Exec(`UPDATE ai_providers SET is_active = 1`); err != nil {
		t.Fatalf("no se pudo activar: %v", err)
	}
	w = crear()
	if w.Code != 404 || !strings.Contains(w.Body.String(), "Rol no encontrado") {
		t.Errorf("con proveedor activo el fallo debe ser el rol; salió %d: %s", w.Code, w.Body.String())
	}
}

// ---------- Representación de las dos pantallas ----------

// La clave de estas dos: el JavaScript vive dentro de la plantilla y un valor
// que html/template no sepa meter en contexto sale como «ZgotmplZ», que NO es
// un error de ejecución: la página se pinta y el script se rompe en el
// navegador, donde nadie ve el motivo.
func paginaAI(t *testing.T, archivo string) *template.Template {
	t.Helper()
	tpl, err := template.New(archivo).Funcs(templateFuncs(nil, nil)).
		ParseFS(erpgo.Templates(), "templates/"+archivo)
	if err != nil {
		t.Fatalf("la plantilla %s no se puede cargar: %v", archivo, err)
	}
	if _, err := tpl.ParseFS(erpgo.Templates(), "templates/layout.html"); err != nil {
		t.Fatalf("%s no convive con layout.html: %v", archivo, err)
	}
	return tpl
}

// TestElFormularioDeAgenteMuestraSoloElProveedorActivo cubre las dos caras del
// formulario: con proveedor activo (se enseña y se oculta el selector) y sin él
// (se avisa y no se deja crear).
func TestElFormularioDeAgenteMuestraSoloElProveedorActivo(t *testing.T) {
	tpl := paginaAI(t, "agent_form.html")

	type rol struct {
		ID   int64
		Name string
	}
	casos := []struct {
		nombre   string
		datos    map[string]interface{}
		esperado []string
		ausente  []string
	}{
		{
			nombre: "con proveedor activo",
			datos: map[string]interface{}{
				"Title": "Añadir Agente IA", "Roles": []rol{{ID: 3, Name: "Contador"}},
				"ProveedorOK": true, "ProveedorID": int64(2),
				"ProveedorEtiqueta": "ORC24 · orc24",
			},
			esperado: []string{`name="provider_id" value="2"`, "ORC24 · orc24",
				"proveedor <strong>activo</strong>"},
			ausente: []string{`id="ag_provider"`, "ZgotmplZ"},
		},
		{
			nombre: "sin proveedor activo",
			datos: map[string]interface{}{
				"Title": "Añadir Agente IA", "Roles": []rol{},
				"ProveedorOK": false, "ProveedorID": int64(0),
			},
			esperado: []string{"No hay proveedor de IA activo", "/ai/settings"},
			ausente:  []string{`id="ag_provider"`, "ZgotmplZ"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cuerpo := representar(t, tpl, "content", c.datos)
			for _, s := range c.esperado {
				if !strings.Contains(cuerpo, s) {
					t.Errorf("el formulario no muestra %q", s)
				}
			}
			for _, s := range c.ausente {
				if strings.Contains(cuerpo, s) {
					t.Errorf("el formulario muestra %q, que no debe aparecer", s)
				}
			}
			if js := representar(t, tpl, "pagejs", c.datos); strings.Contains(js, "ZgotmplZ") {
				t.Error("el script del formulario dejó el marcador ZgotmplZ")
			}
		})
	}
}

// TestLaPantallaDeConfiguracionPintaLosTres comprueba que la tabla es la lista
// canónica (con la marca pendiente pintada como tal) y que el script tiene la
// llave que impide crear un cuarto.
func TestLaPantallaDeConfiguracionPintaLosTres(t *testing.T) {
	tpl := paginaAI(t, "ai_setting.html")

	orc24 := aiProvider{ID: 0, Vendor: "orc24", VendorLabel: "ORC24", Model: "orc24"}
	openai := aiProvider{ID: 4, Name: "Mi OpenAI", Vendor: "openai", VendorLabel: "OpenAI",
		Model: "gpt-5.5", IsActive: true}
	anthropic := aiProvider{ID: 9, Name: "Anthropic laboral", Vendor: "anthropic",
		VendorLabel: "Anthropic", Model: "claude-sonnet-5-5"}
	datos := map[string]interface{}{
		"Title": "Configuración IA", "HasActive": true, "ActiveUsable": true,
		"Active": openai, "Providers": []aiProvider{orc24, openai, anthropic},
	}

	cuerpo := representar(t, tpl, "content", datos)
	paraCada := []string{
		`data-vendorlabel="ORC24"`, `data-vendorlabel="OpenAI"`, `data-vendorlabel="Anthropic"`,
		"Sin configurar",                       // la marca que falta se ve como tal
		"badge_on\">Activo",                    // solo una fila activa
		"Configurar</button>",                  // la marca pendiente se puede registrar
		"solo uno puede estar activo a la vez", // la regla, escrita en la pantalla
		"son solo 3",
	}
	for _, s := range paraCada {
		if !strings.Contains(cuerpo, s) {
			t.Errorf("Configuración IA no muestra %q", s)
		}
	}
	// La fila activa no ofrece «Eliminar»: el botón lleva a un rechazo seguro.
	if strings.Count(cuerpo, "btn_delete") != 1 {
		t.Errorf("solo la fila inactiva debe poder eliminarse; hay %d botones", strings.Count(cuerpo, "btn_delete"))
	}
	if strings.Contains(cuerpo, "Ollama") {
		t.Error("se coló una fila que no es de los tres")
	}
	if strings.Contains(cuerpo, "ZgotmplZ") {
		t.Error("la pantalla dejó el marcador ZgotmplZ")
	}

	js := representar(t, tpl, "pagejs", datos)
	for _, s := range []string{"Los tres proveedores oficiales ya están configurados", "btn_edit"} {
		if !strings.Contains(js, s) {
			t.Errorf("el script de Configuración IA no contiene %q", s)
		}
	}
	if strings.Contains(js, "ZgotmplZ") {
		t.Error("el script dejó el marcador ZgotmplZ")
	}
}
