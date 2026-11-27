package web

// Pruebas de la configuración de facturación digital: exclusividad entre
// imprentas, modo impresora fiscal y el tratamiento de las credenciales.
//
// POR QUÉ ESTAS REGLAS SE PRUEBAN. No son preferencias de pantalla: son las que
// deciden quién emite un documento fiscal. Dos imprentas activas a la vez
// significan dos rangos de numeración sobre el mismo contribuyente, y un token
// borrado por un guardado descuidado deja al ERP sin poder emitir hasta que
// alguien lo descubre en la primera factura del día.

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"strings"
	"testing"

	"erpgo"

	di "erpgo/interfaces/web/digitalinvoice"
	"erpgo/internal/digitalinvoice"

	_ "github.com/glebarez/go-sqlite"
)

// baseDeConfiguracionDigital crea la tabla que guarda la imprenta de cada
// empresa.
func baseDeConfiguracionDigital(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE digital_invoice_config (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		company_id TEXT NOT NULL DEFAULT '1',
		addon_code TEXT NOT NULL,
		config_json TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 0,
		created_at TEXT, updated_at TEXT,
		UNIQUE (company_id, addon_code))`); err != nil {
		t.Fatalf("no se pudo crear digital_invoice_config: %v", err)
	}
	return db
}

// configuracionDeCodigo deja una configuración guardada para una imprenta.
func configuracionDeCodigo(t *testing.T, db *sql.DB, tenant, code string, enabled bool) {
	t.Helper()
	cfg := di.ProviderConfig{Code: code, BaseURL: "https://imprenta.ejemplo/", BearerToken: "token", NumeroSerie: "A"}
	crudo, _ := json.Marshal(cfg)
	valor := 0
	if enabled {
		valor = 1
	}
	if _, err := db.Exec(`INSERT INTO digital_invoice_config (company_id, addon_code, config_json, enabled)
		VALUES (?, ?, ?, ?)`, tenant, code, string(crudo), valor); err != nil {
		t.Fatalf("no se pudo guardar %s: %v", code, err)
	}
}

// estadoDe devuelve si una imprenta quedó activa en la base.
func estadoDe(t *testing.T, db *sql.DB, tenant, code string) bool {
	t.Helper()
	var enabled int
	if err := db.QueryRow(`SELECT enabled FROM digital_invoice_config
		WHERE company_id=? AND addon_code=?`, tenant, code).Scan(&enabled); err != nil {
		t.Fatalf("no se pudo leer el estado de %s: %v", code, err)
	}
	return enabled == 1
}

// TestActivarUnaImprentaDesactivaLaOtra: la exclusividad de la ley, en datos.
func TestActivarUnaImprentaDesactivaLaOtra(t *testing.T) {
	db := baseDeConfiguracionDigital(t)
	configuracionDeCodigo(t, db, "empresa1", "unidigital", true)
	configuracionDeCodigo(t, db, "empresa1", "cgimprenta", false)

	desactivadas, err := diActivarEnBase(db, "empresa1", "cgimprenta")
	if err != nil {
		t.Fatalf("no se pudo activar: %v", err)
	}
	if !estadoDe(t, db, "empresa1", "cgimprenta") {
		t.Error("cgimprenta debía quedar activa")
	}
	if estadoDe(t, db, "empresa1", "unidigital") {
		t.Error("unidigital debía quedar desactivada al activar la otra")
	}
	if len(desactivadas) != 1 || !strings.Contains(desactivadas[0], "Unidigital") {
		t.Errorf("la respuesta debía nombrar la imprenta desactivada: %v", desactivadas)
	}

	// Y volver atrás funciona igual: la configuración de la anterior se conservó.
	if _, err := diActivarEnBase(db, "empresa1", "unidigital"); err != nil {
		t.Fatalf("no se pudo volver a unidigital: %v", err)
	}
	if !estadoDe(t, db, "empresa1", "unidigital") || estadoDe(t, db, "empresa1", "cgimprenta") {
		t.Error("al volver a unidigital, cgimprenta debía quedar desactivada")
	}
}

// TestDesactivarSoloApagaLaIndicada: apagar una imprenta no puede apagar la otra
// (el defecto que tenía el registro en memoria).
func TestDesactivarSoloApagaLaIndicada(t *testing.T) {
	db := baseDeConfiguracionDigital(t)
	configuracionDeCodigo(t, db, "empresa1", "unidigital", true)
	configuracionDeCodigo(t, db, "empresa1", "cgimprenta", false)

	if err := diDesactivarEnBase(db, "empresa1", "cgimprenta"); err != nil {
		t.Fatalf("no se pudo desactivar: %v", err)
	}
	if !estadoDe(t, db, "empresa1", "unidigital") {
		t.Error("desactivar cgimprenta no debe tocar unidigital")
	}

	activo, err := diProveedorActivoEnBase(db, "empresa1")
	if err != nil {
		t.Fatalf("no se pudo leer la imprenta activa: %v", err)
	}
	if activo != "unidigital" {
		t.Errorf("la imprenta activa debía seguir siendo unidigital, es %q", activo)
	}
}

// TestModoImpresoraFiscalDesactivaTodas: sin imprenta activa la empresa factura
// por impresora fiscal, y las configuraciones se conservan.
func TestModoImpresoraFiscalDesactivaTodas(t *testing.T) {
	db := baseDeConfiguracionDigital(t)
	configuracionDeCodigo(t, db, "empresa1", "unidigital", true)
	configuracionDeCodigo(t, db, "empresa1", "cgimprenta", false)

	if err := diDesactivarTodasEnBase(db, "empresa1"); err != nil {
		t.Fatalf("no se pudo cambiar el modo: %v", err)
	}
	activo, err := diProveedorActivoEnBase(db, "empresa1")
	if err != nil {
		t.Fatalf("no se pudo leer la imprenta activa: %v", err)
	}
	if activo != "" {
		t.Errorf("no debía quedar ninguna imprenta activa, quedó %q", activo)
	}
	if diNombreModo(activo) != "Facturación por impresora fiscal" {
		t.Errorf("el modo debía leerse como impresora fiscal: %q", diNombreModo(activo))
	}
	// Las filas siguen ahí: desactivar no es borrar.
	guardadas := diConfiguracionesDeEmpresa(db, "empresa1")
	if len(guardadas) != 2 {
		t.Errorf("debían conservarse las 2 configuraciones, quedan %d", len(guardadas))
	}
}

// TestGuardarNoCambiaElModo: guardar credenciales no activa ni desactiva nada.
// Antes, guardar con la casilla marcada dejaba la imprenta "activa" en la base
// aunque el proveedor no contestara; y guardar la otra la apagaba.
func TestGuardarNoCambiaElModo(t *testing.T) {
	db := baseDeConfiguracionDigital(t)
	configuracionDeCodigo(t, db, "empresa1", "unidigital", true)

	cfg := di.ProviderConfig{
		Code:         "cgimprenta",
		BaseURL:      "https://calidad.cgimprenta.digital/",
		UserName:     "api@negocio.test",
		UserPassword: "clave",
		NumeroSerie:  "A",
	}
	if err := diGuardarConfig(db, "empresa1", cfg); err != nil {
		t.Fatalf("no se pudo guardar: %v", err)
	}
	if estadoDe(t, db, "empresa1", "cgimprenta") {
		t.Error("guardar no debe activar la imprenta")
	}
	if !estadoDe(t, db, "empresa1", "unidigital") {
		t.Error("guardar otra imprenta no debe desactivar la que estaba activa")
	}

	// Guardar de nuevo (mismos datos) no rompe nada ni altera el modo.
	if err := diGuardarConfig(db, "empresa1", cfg); err != nil {
		t.Fatalf("el segundo guardado falló: %v", err)
	}
	if !estadoDe(t, db, "empresa1", "unidigital") {
		t.Error("el segundo guardado desactivó la imprenta activa")
	}
}

// TestGuardarUnaImprentaActivaConservaSuModo: si la configuración guardada es la
// que emite, actualizarla la deja igual de activa.
func TestGuardarUnaImprentaActivaConservaSuModo(t *testing.T) {
	db := baseDeConfiguracionDigital(t)
	configuracionDeCodigo(t, db, "empresa1", "cgimprenta", true)

	if err := diGuardarConfig(db, "empresa1", di.ProviderConfig{
		Code: "cgimprenta", BaseURL: "https://otro-host.ejemplo/", NumeroSerie: "B",
	}); err != nil {
		t.Fatalf("no se pudo actualizar: %v", err)
	}
	if !estadoDe(t, db, "empresa1", "cgimprenta") {
		t.Error("actualizar la imprenta activa no puede desactivarla")
	}
	guardadas := diConfiguracionesDeEmpresa(db, "empresa1")
	if guardadas["cgimprenta"].BaseURL != "https://otro-host.ejemplo/" {
		t.Errorf("la configuración no se actualizó: %+v", guardadas["cgimprenta"])
	}
}

// TestCombinarConfigConservaElSecretoGuardado: el navegador nunca recibe el
// token ni la contraseña, así que un campo secreto vacío significa "no lo
// cambié", no "bórralo".
func TestCombinarConfigConservaElSecretoGuardado(t *testing.T) {
	guardada := di.ProviderConfig{
		Code: "cgimprenta", BaseURL: "https://viejo.ejemplo/",
		UserName: "api", UserPassword: "clave-guardada",
	}
	entrantes := map[string]string{
		"base_url":      "https://nuevo.ejemplo/",
		"user_name":     "api-nuevo",
		"user_password": "",
		"numero_serie":  "A",
	}
	combinada := diCombinarConfig("cgimprenta", guardada, entrantes)
	if combinada.UserPassword != "clave-guardada" {
		t.Errorf("la contraseña guardada se perdió: %q", combinada.UserPassword)
	}
	if combinada.BaseURL != "https://nuevo.ejemplo/" || combinada.UserName != "api-nuevo" {
		t.Errorf("los campos no secretos debían actualizarse: %+v", combinada)
	}
	if combinada.NumeroSerie != "A" {
		t.Errorf("la serie nueva no se aplicó: %q", combinada.NumeroSerie)
	}

	// Con una contraseña nueva, manda la nueva.
	combinada = diCombinarConfig("cgimprenta", guardada, map[string]string{"user_password": "otra"})
	if combinada.UserPassword != "otra" {
		t.Errorf("la contraseña nueva debía reemplazar a la guardada: %q", combinada.UserPassword)
	}

	// Un campo no secreto vacío sí se limpia (la sucursal se puede quitar).
	combinada = diCombinarConfig("cgimprenta", di.ProviderConfig{SucursalID: "Centro"}, map[string]string{"sucursal_id": ""})
	if combinada.SucursalID != "" {
		t.Errorf("la sucursal debía limpiarse, quedó %q", combinada.SucursalID)
	}
}

// TestValidarConfigPideLasCredencialesDeCadaImprenta: cada casa pide lo suyo y
// el mensaje nombra el campo que falta.
func TestValidarConfigPideLasCredencialesDeCadaImprenta(t *testing.T) {
	casos := []struct {
		nombre   string
		code     string
		cfg      di.ProviderConfig
		contiene string
	}{
		{
			"unidigital sin token", "unidigital",
			di.ProviderConfig{Code: "unidigital", BaseURL: "https://api.unidigital.ve/v1", SerieStrongID: "s-1"},
			"Bearer Token",
		},
		{
			"unidigital sin serie", "unidigital",
			di.ProviderConfig{Code: "unidigital", BaseURL: "https://api.unidigital.ve/v1", BearerToken: "t"},
			"Serie Strong ID",
		},
		{
			"cg sin usuario", "cgimprenta",
			di.ProviderConfig{Code: "cgimprenta", BaseURL: "https://calidad.cgimprenta.digital/", UserPassword: "c", NumeroSerie: "A"},
			"Usuario de API",
		},
		{
			"cg sin contraseña", "cgimprenta",
			di.ProviderConfig{Code: "cgimprenta", BaseURL: "https://calidad.cgimprenta.digital/", UserName: "u", NumeroSerie: "A"},
			"Contraseña de API",
		},
		{
			"cg sin serie", "cgimprenta",
			di.ProviderConfig{Code: "cgimprenta", BaseURL: "https://calidad.cgimprenta.digital/", UserName: "u", UserPassword: "c"},
			"Serie de numeración",
		},
		{
			"URL sin esquema", "cgimprenta",
			di.ProviderConfig{Code: "cgimprenta", BaseURL: "calidad.cgimprenta.digital",
				UserName: "u", UserPassword: "c", NumeroSerie: "A"},
			"URL",
		},
		{
			"thka sin usuario", "thka",
			di.ProviderConfig{Code: "thka", BaseURL: "https://demoemisionv2.thefactoryhka.com.ve/", UserPassword: "c"},
			"Usuario de API",
		},
		{
			"thka sin clave", "thka",
			di.ProviderConfig{Code: "thka", BaseURL: "https://demoemisionv2.thefactoryhka.com.ve/", UserName: "u"},
			"Clave de API",
		},
		{
			"imprenta desconocida", "imprenta-inventada", di.ProviderConfig{Code: "imprenta-inventada"},
			"catálogo",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			err := diValidarConfig(caso.code, caso.cfg)
			if err == nil {
				t.Fatalf("debía rechazar la configuración")
			}
			if !strings.Contains(err.Error(), caso.contiene) {
				t.Errorf("el error debía nombrar %q, vino: %v", caso.contiene, err)
			}
		})
	}

	// Las dos configuraciones completas pasan.
	if err := diValidarConfig("unidigital", di.ProviderConfig{
		Code: "unidigital", BaseURL: "https://api.unidigital.ve/v1",
		BearerToken: "t", SerieStrongID: "s-1",
	}); err != nil {
		t.Errorf("unidigital completa no debía fallar: %v", err)
	}
	if err := diValidarConfig("cgimprenta", di.ProviderConfig{
		Code: "cgimprenta", BaseURL: "https://calidad.cgimprenta.digital/",
		UserName: "u", UserPassword: "c", NumeroSerie: "A",
	}); err != nil {
		t.Errorf("cgimprenta completa no debía fallar: %v", err)
	}
	// THKA permite no usar series: la serie es opcional en su catálogo.
	if err := diValidarConfig("thka", di.ProviderConfig{
		Code: "thka", BaseURL: "https://demoemisionv2.thefactoryhka.com.ve/",
		UserName: "u", UserPassword: "c",
	}); err != nil {
		t.Errorf("thka completa sin serie no debía fallar: %v", err)
	}
}

// TestLaPantallaDeFacturacionDigitalSeRenderiza comprueba que la vista se
// representa con las dos imprentas, que los campos salen de su catálogo y que
// las credenciales guardadas NO se pintan en el HTML.
//
// Sin esta prueba, un campo renombrado en el catálogo dejaría la pantalla
// mostrando ceros o vacía sin que nadie lo note hasta abrirla en producción.
func TestLaPantallaDeFacturacionDigitalSeRenderiza(t *testing.T) {
	guardadas := map[string]di.ProviderConfig{
		"cgimprenta": {
			Code: "cgimprenta", BaseURL: "https://calidad.cgimprenta.digital/",
			UserName: "api@negocio.test", UserPassword: "clave-que-no-debe-verse",
			NumeroSerie: "A",
		},
	}
	vistas := diVistasDeCatalogo("cgimprenta", guardadas)
	if len(vistas) != len(diCatalogo) {
		t.Fatalf("la pantalla debe ofrecer las %d imprentas del catálogo, ofrece %d", len(diCatalogo), len(vistas))
	}
	for _, vista := range vistas {
		if vista.Code == "cgimprenta" {
			if vista.Valores["user_password"] != "" {
				t.Error("la contraseña de API no puede viajar al navegador")
			}
			if !vista.Guardados["user_password"] {
				t.Error("la pantalla debe saber que la contraseña ya está guardada")
			}
			if vista.Valores["base_url"] != "https://calidad.cgimprenta.digital/" {
				t.Errorf("la URL guardada no se muestra: %q", vista.Valores["base_url"])
			}
			if !vista.Activa {
				t.Error("cgimprenta debía salir como activa")
			}
		}
		if vista.Code == "unidigital" {
			if vista.Valores["base_url"] != "https://api.unidigital.ve/v1" {
				t.Errorf("sin configurar, la URL debe sugerirse: %q", vista.Valores["base_url"])
			}
			if vista.Activa || vista.Configurada {
				t.Error("unidigital no está configurada ni activa en este caso")
			}
		}
	}

	tpl, err := template.New("digital_invoice.html").Funcs(templateFuncs(nil, nil)).
		ParseFS(erpgo.Templates(), "templates/digital_invoice.html")
	if err != nil {
		t.Fatalf("la plantilla no se puede cargar: %v", err)
	}
	if _, err := tpl.ParseFS(erpgo.Templates(), "templates/layout.html"); err != nil {
		t.Fatalf("la página no convive con layout.html: %v", err)
	}

	datos := map[string]interface{}{
		"Imprentas":       vistas,
		"ProveedorActivo": "CG Imprenta Digital",
		"ModoDigital":     true,
		"ContPend":        2,
		"ContFall":        1,
		"Contingencia": []contingenciaFila{
			{Invoice: 900, Fecha: "2026-11-27", Referencia: "CONT-01-000123", RIF: "J-12345678-9",
				Total: 1160, EstadoDI: "pending", NroControl: "00-00000056"},
		},
	}
	salida := representar(t, tpl, "content", datos)
	for _, esperado := range []string{
		"CG Imprenta Digital", "Unidigital", "Usuario de API", "Contraseña de API",
		"Serie de numeración", "ACTIVA", "Guardado — deje vacío para conservarlo",
		"Dejar la empresa solo con impresora fiscal", "00-00000056",
	} {
		if !strings.Contains(salida, esperado) {
			t.Errorf("la pantalla no muestra %q", esperado)
		}
	}
	if strings.Contains(salida, "clave-que-no-debe-verse") {
		t.Fatal("la pantalla está pintando la contraseña de API guardada")
	}

	script := representar(t, tpl, "pagejs", datos)
	for _, endpoint := range []string{
		"/settings/digital-invoice/save", "/settings/digital-invoice/activate",
		"/settings/digital-invoice/deactivate", "/settings/digital-invoice/fiscal",
		"/settings/digital-invoice/ping", "/settings/digital-invoice/retry",
	} {
		if !strings.Contains(script, endpoint) {
			t.Errorf("el script de la pantalla no llama a %q", endpoint)
		}
	}
}

// TestCadaImprentaDelCatalogoTieneProveedor: un catálogo que ofrece una imprenta
// que el sistema no sabe montar es una pantalla que miente.
func TestCadaImprentaDelCatalogoTieneProveedor(t *testing.T) {
	if len(diCatalogo) < 2 {
		t.Fatalf("el catálogo debe ofrecer al menos las dos imprentas integradas, tiene %d", len(diCatalogo))
	}
	vistos := map[string]bool{}
	for _, imp := range diCatalogo {
		if vistos[imp.Code] {
			t.Errorf("el código %q está repetido en el catálogo", imp.Code)
		}
		vistos[imp.Code] = true
		proveedor, err := digitalinvoice.NuevoProveedorActivo(imp.Code, di.ProviderConfig{
			Code: imp.Code, BaseURL: "https://imprenta.ejemplo/",
		})
		if err != nil {
			t.Errorf("la imprenta %q está en el catálogo pero el sistema no sabe montarla: %v", imp.Code, err)
			continue
		}
		if proveedor.Codigo() != imp.Code {
			t.Errorf("la imprenta %q se identifica como %q", imp.Code, proveedor.Codigo())
		}
		if len(imp.Campos) == 0 {
			t.Errorf("la imprenta %q no declara ningún campo de configuración", imp.Code)
		}
	}
}
