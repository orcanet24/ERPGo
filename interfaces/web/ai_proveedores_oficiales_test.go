package web

// ai_proveedores_oficiales_test.go — sistema de proveedores de IA.
//
// Nuevo modelo: cada proveedor guarda su URL en BD (columna base_url).
// No hay whitelist de vendors ni URLs hardcodeadas en el binario.

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

func nuevaBaseDeProveedores(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir base: %v", err)
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
		is_active INTEGER NOT NULL DEFAULT 0,
		op_cache INTEGER NOT NULL DEFAULT 0)`)
	if err != nil {
		t.Fatalf("no se pudo crear tabla: %v", err)
	}
	return db
}

// TestProveedoresSeCarganDeBD: todos los proveedores de la BD aparecen.
func TestProveedoresSeCarganDeBD(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	for _, nombre := range []string{"Proveedor A", "Proveedor B"} {
		if _, err := db.Exec(`INSERT INTO ai_providers (name, vendor, base_url, api_key, model, is_active)
			VALUES (?, 'personalizado', 'https://api.ejemplo.com/v1', 'key', 'gpt-4o-mini', 1)`, nombre); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	s := &Server{db: db}
	lista := s.aiProveedoresOficiales(nil)
	if len(lista) != 2 {
		t.Fatalf("esperaba 2 proveedores, vinieron %d", len(lista))
	}
	// El primero es el activo.
	if !lista[0].IsActive {
		t.Errorf("el primer proveedor debe ser el activo")
	}
}

// TestBDVaciaDevuelveProveedorPorDefecto: si no hay proveedores, se ofrece
// un "Personalizado" editable por defecto.
func TestBDVaciaDevuelveProveedorPorDefecto(t *testing.T) {
	s := &Server{db: nuevaBaseDeProveedores(t)}
	lista := s.aiProveedoresOficiales(nil)
	if len(lista) != 1 || lista[0].Vendor != "personalizado" {
		t.Errorf("BD vacía debe devolver 1 proveedor personalizado: %+v", lista)
	}
}

// TestNoHayRestriccionUnRegistroPorMarca: cada proveedor es independiente.
func TestNoHayRestriccionUnRegistroPorMarca(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	s := &Server{db: db}

	guardar := func(nombre, vendor string) *httptest.ResponseRecorder {
		form := url.Values{
			"id": {"0"}, "name": {nombre}, "vendor": {vendor},
			"base_url": {"https://api.ejemplo.com/v1"}, "api_key": {"sk-x"}, "model": {"gpt-4o-mini"},
		}
		r := withScope(httptest.NewRequest("POST", "/ai/providers/save", strings.NewReader(form.Encode())), &scope{db: db})
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.aiProviderSave(w, r)
		return w
	}

	if w := guardar("Proveedor 1", "openai"); !strings.Contains(w.Body.String(), "success") {
		t.Fatalf("primer registro debe crearse: %s", w.Body.String())
	}
	// Otro con el mismo vendor también se crea (son URLs distintas).
	if w := guardar("Proveedor 2", "openai"); !strings.Contains(w.Body.String(), "success") {
		t.Fatalf("segundo registro con mismo vendor debe crearse: %s", w.Body.String())
	}
	if n := contarFilas(t, db, "ai_providers"); n != 2 {
		t.Errorf("deben quedar 2 registros, quedaron %d", n)
	}
}

// TestElAgenteUsaElProveedorActivo: los agentes trabajan con el activo.
func TestElAgenteUsaElProveedorActivo(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	if _, err := db.Exec(`INSERT INTO ai_providers (id, name, vendor, base_url, api_key, model, is_active)
		VALUES (1, 'Inactivo', 'personalizado', 'https://old.com/v1', 'k-old', 'gpt', 0),
		       (2, 'Activo', 'personalizado', 'https://new.com/v1', 'k-new', 'gpt-4o', 1)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	s := &Server{db: db}

	p, ok := s.aiProviderForAgent(nil, aiAgent{ID: 7, ProviderID: 1})
	if !ok {
		t.Fatal("con proveedor activo el agente debe trabajar")
	}
	if p.APIKey != "k-new" {
		t.Errorf("el agente debe usar el proveedor ACTIVO, usó clave=%q", p.APIKey)
	}
	// La URL viene de la BD, no del binario.
	if p.BaseURL != "https://new.com/v1" {
		t.Errorf("la URL debe venir de la BD, salió %q", p.BaseURL)
	}

	// Sin activo, el agente no trabaja.
	if _, err := db.Exec(`UPDATE ai_providers SET is_active = 0`); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := s.aiProviderForAgent(nil, aiAgent{ID: 7, ProviderID: 1}); ok {
		t.Error("sin proveedor activo el agente no debe poder ejecutarse")
	}
}

// TestSinProveedorActivoNoSeCreaAgente: sin proveedor activo no se da alta agente.
func TestSinProveedorActivoNoSeCreaAgente(t *testing.T) {
	db := nuevaBaseDeProveedores(t)
	if _, err := db.Exec(`INSERT INTO ai_providers (name, vendor, base_url, api_key, model, is_active)
		VALUES ('Proveedor', 'personalizado', 'https://api.com/v1', 'key', 'gpt', 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	s := &Server{db: db, sessions: NewSessionStore()}

	form := url.Values{"name": {"Agente"}, "role_id": {"1"}}
	r := withScope(httptest.NewRequest("POST", "/ai/agents/save", strings.NewReader(form.Encode())), &scope{db: db})
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.agentSave(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "proveedor de IA activo") {
		t.Fatalf("debe rechazarse por sin proveedor activo (400); salió %d: %s", w.Code, w.Body.String())
	}
}

// ---------- Representación de pantallas ----------
func paginaAI(t *testing.T, archivo string) *template.Template {
	t.Helper()
	tpl, err := template.New(archivo).Funcs(templateFuncs(nil, nil)).
		ParseFS(erpgo.Templates(), "templates/"+archivo)
	if err != nil {
		t.Fatalf("plantilla %s no cargable: %v", archivo, err)
	}
	if _, err := tpl.ParseFS(erpgo.Templates(), "templates/layout.html"); err != nil {
		t.Fatalf("%s no convive con layout.html: %v", archivo, err)
	}
	return tpl
}

// TestLaPantallaDeConfiguracionPintaProveedores: los proveedores de la BD se
// muestran en la tabla con sus datos (URL, modelo, estado).
func TestLaPantallaDeConfiguracionPintaProveedores(t *testing.T) {
	tpl := paginaAI(t, "ai_setting.html")

	p1 := aiProvider{ID: 1, Name: "Proveedor A", Vendor: "personalizado", VendorLabel: "Personalizado",
		BaseURL: "https://api.a.com/v1", Model: "gpt-4o-mini", IsActive: true}
	p2 := aiProvider{ID: 2, Name: "Proveedor B", Vendor: "openai", VendorLabel: "OpenAI",
		BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"}
	datos := map[string]interface{}{
		"Title": "Configuración IA", "HasActive": true, "ActiveUsable": true,
		"Active": p1, "Providers": []aiProvider{p1, p2},
	}

	cuerpo := representar(t, tpl, "content", datos)
	for _, s := range []string{
		`data-name="Proveedor A"`, `data-name="Proveedor B"`,
		`data-base-url="https://api.a.com/v1"`, `data-base-url="https://api.openai.com/v1"`,
		`data-model="gpt-4o-mini"`, `data-model="gpt-4o"`,
		"badge_on", "Editar</button>", "Eliminar</button>",
	} {
		if !strings.Contains(cuerpo, s) {
			t.Errorf("Configuración IA no muestra %q", s)
		}
	}
	if strings.Contains(cuerpo, "ZgotmplZ") {
		t.Error("la pantalla dejó el marcador ZgotmplZ")
	}

	js := representar(t, tpl, "pagejs", datos)
	if !strings.Contains(js, "btn_edit") {
		t.Errorf("el script no contiene btn_edit")
	}
	if strings.Contains(js, "ZgotmplZ") {
		t.Error("el script dejó ZgotmplZ")
	}
}
