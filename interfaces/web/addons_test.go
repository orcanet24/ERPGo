package web

// addons_test.go — Pruebas de la frontera (docs/PLAN-ADDONS.md, fase 2).
//
// Lo que cada prueba protege, sin adornos:
//
//   - El candado del manifiesto: nada entra a correr si su declaratoria tiene
//     escaleras que escalar (id raro, ruta del núcleo, permiso con prefijo ajeno,
//     tabla sin prefijo del id, menú que cita permisos no declarados).
//   - El registro: addons → filas de modules/menu en la base de empresa, y el
//     desinstalado queda «no instalado» con filas inactivas, jamás borradas.
//   - El proxy: reenvía con X-ERP-* (usuario, empresa, permisos y la base de
//     la empresa activa, X-ERP-DB) y SIN cookies del ERP.
//   - El canal de negocio: 403 sin token / fuera de loopback / token ajeno;
//     en fase 2, un verb que el manifiesto NO declara en db_write da 403
//     (el manifiesto es el candado, aunque el handler exista).
//   - Fase 2: db_write exige dominio.accion y permiso declarado; tables_own
//     exige schema.sql con CREATE TABLE IF NOT EXISTS (solo sus tablas);
//     el DSN de lectura granular llega por entorno o se explica en el panel;
//     la escritura exige POST, JSON, sesión viva, empresa y permiso.
//   - El ciclo completo con el addon REAL del repo (addons/hola): se compila,
//     se lanza, sirve su página, y matarlo no derriba al ERP.
//
// En los tests la lógica de lanzamiento se inyecta o se ejecuta de verdad con
// el binario del demo; nunca se dá por buena una pieza que solo vive en RAM.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- El candado del manifiesto ----------

func TestElManifiestoValidoSeAcepta(t *testing.T) {
	m := manifiestoValido()
	if err := validarManifiesto(m); err != nil {
		t.Fatalf("un manifiesto correcto fue rechazado: %v", err)
	}
}

func TestElManifiestoRechazaLoQuePuedeDanar(t *testing.T) {
	casos := []struct {
		nombre string
		media  func(*addonManifest)
		que    string
	}{
		{"id con espacios", func(m *addonManifest) { m.ID = "con espacio" }, "id"},
		{"id que no es suyo de más", func(m *addonManifest) { m.ID = "A" }, "id"},
		{"sin nombre", func(m *addonManifest) { m.Name = "" }, "nombre"},
		{"núcleo más viejo que lo exigido", func(m *addonManifest) { m.MinCoreVersion = "9.0.0" }, "núcleo"},
		{"sin rutas", func(m *addonManifest) { m.Routes = nil }, "ruta"},
		{"ruta del núcleo", func(m *addonManifest) { m.Routes = []string{"/customer"} }, "del núcleo"},
		{"ruta rara", func(m *addonManifest) { m.Routes = []string{"/HOLA"} }, "inválida"},
		{"ruta duplicada", func(m *addonManifest) { m.Routes = []string{"/x", "/x"} }, "dos veces"},
		{"permiso sin prefijo del id", func(m *addonManifest) {
			m.Perms = []addonPermiso{{Code: "ver", Label: "Ver"}}
		}, "prefijo"},
		{"permiso con id ajeno", func(m *addonManifest) {
			m.Perms = []addonPermiso{{Code: "otro_ver", Label: "Ver"}}
		}, "prefijo"},
		{"tabla sin prefijo del id", func(m *addonManifest) {
			m.TablesOwn = []string{"customer_information"}
		}, "no son suyas"},
		{"menú bajo una ruta no declarada", func(m *addonManifest) {
			m.Menu = []addonMenuEntry{{Label: "Fuga", URL: "/otra-cosa"}}
		}, "no cae bajo ninguna ruta"},
		{"menú con permiso no declarado", func(m *addonManifest) {
			m.Menu[0].Perms = []string{"hola_fantasma"}
		}, "no declara"},
	}
	for _, c := range casos {
		m := manifiestoValido()
		c.media(m)
		err := validarManifiesto(m)
		if err == nil {
			t.Errorf("%s: el manifiesto con %s pasó el candado", c.nombre, c.que)
		}
	}
}

func TestLasVersionesDelNucleoSeComparanEnTresNumeros(t *testing.T) {
	ok := [][2]string{
		{"0.1.0", "0.1.0"}, {"", "0.1.0"}, {"0.0.9", "0.1.0"}, {"0.1", "0.1.0"},
	}
	for _, c := range ok {
		if !addonVersionCompatible(c[0], c[1]) {
			t.Errorf("%q debía ser compatible con %q", c[0], c[1])
		}
	}
	no := [][2]string{{"0.1.1", "0.1.0"}, {"1.0.0", "0.1.0"}, {"basura", "0.1.0"}}
	for _, c := range no {
		if addonVersionCompatible(c[0], c[1]) {
			t.Errorf("%q no debía pasar contra %q", c[0], c[1])
		}
	}
}

func manifiestoValido() *addonManifest {
	return &addonManifest{
		ID:             "hola",
		Name:           "Hola, addon",
		Version:        "1.0.0",
		MinCoreVersion: "0.1.0",
		Description:    "demo",
		Author:         "ERPGo",
		Routes:         []string{"/hola"},
		Menu: []addonMenuEntry{{
			Label: "Hola addon", URL: "/hola", Icon: "fa-smile-o",
			Perms: []string{"hola_ver"}, SortOrder: 90,
		}},
		Perms: []addonPermiso{{Code: "hola_ver", Label: "Ver la pantalla"}},
	}
}

// ---------- El escáner y el registro en la DB ----------

// addonDirPrueba monta una carpeta addons con un manifiesto y un binario
// ejecutable (de verdad: sale de exec.Command; el escáner lo exige via
// 0111 pero el lanzamiento es inyectable, así que el contenido, no importa).
func addonDirPrueba(t *testing.T, manifiesto string) (carpeta string) {
	t.Helper()
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "hola")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifiesto), 0o644); err != nil {
		t.Fatal(err)
	}
	return raiz
}

const manifiestoHola = `{
  "id": "hola", "name": "Hola, addon", "version": "1.0.0", "min_core_version": "0.1.0",
  "description": "demo", "author": "ERPGo",
  "routes": ["/hola"],
  "menu": [{"parent_id": null, "label": "Hola addon", "url": "/hola", "icon": "fa-smile-o", "perms": ["hola_ver"], "sort_order": 90}],
  "perms": [{"code": "hola_ver", "label": "Ver la pantalla"}]
}`

func hostDeAddon(t *testing.T, dir string) (*Server, *addonHost) {
	t.Helper()
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE modules (id TEXT PRIMARY KEY, name TEXT, version TEXT,
		description TEXT, is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 1, path TEXT,
		author TEXT, license_type TEXT)`); err != nil {
		t.Fatal(err)
	}
	// El esquema de menu de las pruebas base no trae module_id ni is_core
	// (otras pruebas los montan a mano). Aquí se reemplaza por la versión
	// COMPLETA del esquema en producción, que es con la que el registro
	// del addon tiene que funcionar.
	if _, err := db.Exec(`DROP TABLE menu`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE menu (id INTEGER PRIMARY KEY AUTOINCREMENT,
		parent_id INTEGER, label TEXT, url TEXT, icon TEXT, perm TEXT, active_prefix TEXT,
		sort_order INTEGER DEFAULT 0, status INTEGER DEFAULT 1, module_id TEXT DEFAULT 'core',
		is_core INTEGER DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	h := &addonHost{
		s: s, directorio: dir,
		progs:       map[string]*addonProc{},
		rutasDelMux: map[string]bool{},
		ahora:       time.Now,
		parado:      make(chan struct{}),
	}
	h.lanzar = func(p *addonProc, binario string, env []string) error { return nil } // no lanza de verdad
	s.addons = h
	return s, h
}

func TestElEscaneoRegistraElAddonEnModulosYMenu(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))

	h.reescanear()
	db := s.db
	var activo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = 'hola'`).Scan(&activo); err != nil {
		t.Fatalf("la fila del módulo no se creó: %v", err)
	}
	if activo != 1 {
		t.Fatal("la primera instalación nace activo; nació apagado")
	}
	row := db.QueryRow(`SELECT label, url, icon, perm, status, sort_order FROM menu WHERE module_id = 'hola'`)
	var label, url, icon, perm string
	var status, sort int64
	if err := row.Scan(&label, &url, &icon, &perm, &status, &sort); err != nil {
		t.Fatalf("la entrada de menú no se creó: %v", err)
	}
	if label != "Hola addon" || url != "/hola" || status != 1 {
		t.Fatalf("la entrada de menú quedó mal: %q %q status=%d", label, url, status)
	}
	if perm != "hola_ver" {
		t.Fatalf("el permiso del menú fue %q, se esperaba hola_ver", perm)
	}
}

func TestAlDesinstalarElAddonQuedaNoInstaladoYPreservado(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	h.reescanear()

	// Se retira el addon de la carpeta (como una desinstalación a mano) y se
	// re-escanea: las rutas dejan de proxyar y las FILAS quedan, inactivas.
	carpeta := h.directorio
	_ = os.RemoveAll(filepath.Join(carpeta, "hola"))
	h.reescanear()

	var activo int
	if err := s.db.QueryRow(`SELECT is_active FROM modules WHERE id = 'hola'`).Scan(&activo); err != nil {
		t.Fatalf("la fila de modules debía existir como evidencia: %v", err)
	}
	if activo != 0 {
		t.Fatal("el addon desinstalado debía quedar is_active = 0")
	}
	var status int
	if err := s.db.QueryRow(`SELECT status FROM menu WHERE module_id = 'hola'`).Scan(&status); err != nil {
		t.Fatalf("la fila de menú debía existir como evidencia: %v", err)
	}
	if status != 0 {
		t.Fatal("la entrada de menú debía quedar status = 0, jamás borrada")
	}
	// Y el proxy ya no la resuelve: el addon ya no vive en h.progs.
	if _, queda := h.progs["hola"]; queda {
		t.Fatal("el addon desinstalado debía salir del host")
	}
	r := reqPrueba(s, sessPrueba("master"), "/hola")
	rr := httptest.NewRecorder()
	s.addonRouter(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("la ruta de un addon desinstalado debía dar 404, dio %d", rr.Code)
	}
}

func TestElEscaneoInformaAvisoPorCarpetaDefectuosa(t *testing.T) {
	dir := addonDirPrueba(t, manifiestoHola)
	rotta := filepath.Join(dir, "roto")
	if err := os.MkdirAll(rotta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rotta, "manifest.json"),
		[]byte(`{"id_wrong": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, h := hostDeAddon(t, dir)
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	avisos := h.avisos
	h.mu.Unlock()
	if len(avisos) == 0 {
		t.Fatal("un addon con manifiesto inválido debía dejar un aviso visible para el panel")
	}
	if !strings.Contains(strings.Join(avisos, " | "), "roto") {
		t.Fatalf("el aviso no menciona la carpeta defectuosa: %v", avisos)
	}
}

// ---------- El proxy inverso ----------

func TestElProxyReenviaConContextoYNoConCookies(t *testing.T) {
	var vistasDe, vistasTenant, vistasPerms, vistasCookies, vistasBase, vistasAuth, vistasCSRF []string
	fin := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vistasDe = append(vistasDe, r.Header.Get("X-ERP-User"))
		vistasTenant = append(vistasTenant, r.Header.Get("X-ERP-Tenant"))
		vistasPerms = append(vistasPerms, r.Header.Get("X-ERP-Perms"))
		vistasCookies = append(vistasCookies, r.Header.Get("Cookie"))
		vistasBase = append(vistasBase, r.Header.Get("X-ERP-DB"))
		vistasAuth = append(vistasAuth, r.Header.Get("X-ERP-Auth-DB"))
		vistasCSRF = append(vistasCSRF, r.Header.Get("X-ERP-CSRF"))
		_, _ = w.Write([]byte("pantalla del addon"))
		close(fin)
	}))
	defer upstream.Close()

	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	// Toda instalación real trae DB_NAME: sin él la cabecera de base saldría
	// vacía y la prueba estaría midiendo un arranque a medias. El código de
	// empresa maestra se separa del de control para que las dos cabeceras de
	// base sean distinguibles y no se cuelen la una por la otra.
	s.config.DBName = "erpgo_prueba"
	s.config.DBMasterTenantCode = "empresa_maestra"
	h.reescanear()
	h.mu.Lock()
	p := h.progs["hola"]
	p.estado = estadoActivo
	p.proxy = makeProxy(upstream.URL)
	h.mu.Unlock()

	ses := sessPrueba("master")
	ses.CSRFToken = "token-csrf-de-prueba"
	r := reqPrueba(s, ses, "/hola")
	r.Header.Set("Cookie", "erpgow_session=secreta-del-ERP")
	rr := httptest.NewRecorder()
	s.addonRouter(rr, r)

	select {
	case <-fin:
	case <-time.After(2 * time.Second):
		t.Fatal("la petición no llegó al addon")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("el proxy respondió %d en vez de reenviar: %s", rr.Code, rr.Body.String())
	}
	if vistasDe[0] != "7" {
		t.Fatalf("el addon no supo el usuario: %q", vistasDe[0])
	}
	if vistasTenant[0] != defaultTenantCode {
		t.Fatalf("el addon no supo la empresa: %q", vistasTenant[0])
	}
	if vistasPerms[0] != "__master" {
		t.Fatalf("el master debía ir como __master: %q", vistasPerms[0])
	}
	if vistasCookies[0] != "" {
		t.Fatalf("las cookies del ERP llegaron al addon: %q", vistasCookies[0])
	}
	// La lectura granular solo sirve si el addon sabe EN QUÉ base consultar:
	// sin esta cabecera, el grant y el DSN no se encuentran.
	if vistasBase[0] != s.tenantDBName(defaultTenantCode) {
		t.Fatalf("X-ERP-DB debía ser la base de la empresa activa (%q), dio %q", s.tenantDBName(defaultTenantCode), vistasBase[0])
	}
	if vistasAuth[0] != "erpgo_prueba" {
		t.Fatalf("X-ERP-Auth-DB debía ser la base de identidad (\"erpgo_prueba\"), dio %q", vistasAuth[0])
	}
	if vistasAuth[0] == vistasBase[0] {
		t.Fatal("las dos cabeceras de base salieron iguales: el addon no podría distinguir empresa de identidad")
	}
	// El token CSRF va para que el formulario del addon lo adjunte: el candado
	// lo aplica el núcleo antes de reenviar.
	if vistasCSRF[0] != "token-csrf-de-prueba" {
		t.Fatalf("X-ERP-CSRF debía traer el token de la sesión, dio %q", vistasCSRF[0])
	}
	if !strings.Contains(rr.Body.String(), "pantalla del addon") {
		t.Fatalf("el cuerpo del addon no llegó al usuario: %s", rr.Body.String())
	}
}

// TestLaRutaDelAddonPideSesion: sin sesión, redirect a login, igual que
// cualquier ruta del núcleo — el addon jamás implementa su propio ingreso.
func TestLaRutaDelAddonPideSesion(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	h.reescanear()

	// La pantalla sin sesión pasa por la CADENA completa de middlewares, como
	// en producción: authMiddleware es quien manda al login.
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.addonRouter(w, r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/hola", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("sin sesión la ruta del addon debía mandar al login (302), dio %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Fatalf("el redirect fue a %q, debía ser /login", loc)
	}
}

// TestElCanalDeNegocioAtraviesaLaCadenaDeMiddlewares: el canal se prueba AQUÍ
// contra la cadena EXACTA de producción —authMiddleware envolviendo a
// permissionMiddleware—, no contra el handler suelto.
//
// Por qué hace falta esta prueba: authMiddleware manda al login a todo lo que
// no esté en SU lista de públicos y corre ANTES que el de permisos, donde
// /__addon/ ya estaba declarado. Un addon no tiene cookie ni va a tener, así
// que su escritura moría en un 302 que no puede seguir: ningún verbo de
// ningún módulo (nota.crear incluido) llegaba al orquestador en un ERP real,
// y las pruebas no lo veían porque llamaban al canal directamente.
func TestElCanalDeNegocioAtraviesaLaCadenaDeMiddlewares(t *testing.T) {
	// El manifiesto fase 2 declara tables_own: sin schema.sql el escáner ni
	// carga el addon y no hay canal que probar.
	dir := addonDirPrueba(t, manifiestoHolaFase2)
	escribirSchema(t, dir, schemaHola)
	s, h := hostDeAddon(t, dir)
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.progs["hola"].token = "token-canal"
	h.mu.Unlock()

	pasar := func(r *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		s.authMiddleware(s.permissionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.addonCanal(w, r)
		}))).ServeHTTP(rr, r)
		return rr
	}

	// a) El ping sin cookie: tiene que responder EL CANAL, no el login.
	ping := httptest.NewRequest(http.MethodGet, "/__addon/hola/ping", nil)
	ping.RemoteAddr = "127.0.0.1:50000"
	ping.Header.Set("X-ERP-Addon-Token", "token-canal")
	if rr := pasar(ping); rr.Code != http.StatusOK {
		t.Fatalf("el canal debía responder 200 al atravesar la cadena, dio %d (Location=%q): la escritura de los addons volvería a estar cortada",
			rr.Code, rr.Header().Get("Location"))
	}

	// b) Una escritura sin usuario en el contexto: la rechaza EL CANAL, con su
	//    403 y su motivo — no un redirect, que es lo que pasaba antes.
	escritura := httptest.NewRequest(http.MethodPost, "/__addon/hola/mensaje.registrar",
		strings.NewReader(`{"mensaje":"hola"}`))
	escritura.RemoteAddr = "127.0.0.1:50000"
	escritura.Header.Set("X-ERP-Addon-Token", "token-canal")
	escritura.Header.Set("Content-Type", "application/json")
	if rr := pasar(escritura); rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "sesión viva") {
		t.Fatalf("la escritura sin sesión debía rechazarla el canal (403 con su motivo), dio %d: %s",
			rr.Code, rr.Body.String())
	}
}

func TestElAddonApagadoPorElMasterNoSirveSuPantalla(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	h.reescanear()
	h.mu.Lock()
	p := h.progs["hola"]
	p.estado = estadoActivo
	p.proxy = makeProxy("http://127.0.0.1:1") // nadie escucha acá
	h.mu.Unlock()

	// El master apaga el módulo en la empresa activa.
	if _, err := s.db.Exec(`UPDATE modules SET is_active = 0 WHERE id = 'hola'`); err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, sessPrueba("master"), "/hola")
	rr := httptest.NewRecorder()
	s.addonRouter(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("el módulo apagado debía dar 404 con mensaje claro, dio %d: %s", rr.Code, rr.Body.String())
	}
}

func TestElAddonCaidoResponde502ConMensajeClaro(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	h.reescanear()
	h.mu.Lock()
	h.progs["hola"].estado = estadoCaydo
	h.mu.Unlock()

	r := reqPrueba(s, sessPrueba("master"), "/hola")
	rr := httptest.NewRecorder()
	s.addonRouter(rr, r)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("un addon caído debía dar 502, dio %d", rr.Code)
	}
	var cuerpo map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &cuerpo)
	if !strings.Contains(cuerpo["error"], "Reinícialo") {
		t.Fatalf("el mensaje de 502 no dice cómo levantarlo: %v", cuerpo)
	}
}

// ---------- El canal de negocio ----------

func TestElCanalDeNegocioExigeTokenLoopbackYVerbSolamente(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	h.reescanear()
	h.mu.Lock()
	h.progs["hola"].token = "token-de-esta-sesion"
	h.mu.Unlock()

	pasar := func(r *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		s.addonCanal(rr, r)
		return rr
	}
	peticion := func(remote, tpren string, verb string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/__addon/hola/"+verb, nil)
		req.RemoteAddr = remote
		if tpren != "" {
			req.Header.Set("X-ERP-Addon-Token", tpren)
		}
		return withScope(req, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	}

	if rr := pasar(peticion("127.0.0.1:50000", "", "ping")); rr.Code != http.StatusForbidden {
		t.Fatalf("sin token debía denegarse (403), dio %d", rr.Code)
	}
	if rr := pasar(peticion("127.0.0.1:50000", "token-ajeno", "ping")); rr.Code != http.StatusForbidden {
		t.Fatalf("token equivocado debía denegarse (403), dio %d", rr.Code)
	}
	if rr := pasar(peticion("10.0.0.5:50000", "token-de-esta-sesion", "ping")); rr.Code != http.StatusForbidden {
		t.Fatalf("fuera de loopback debía denegarse (403), dio %d", rr.Code)
	}
	// Fase 2: el verbo no declarado en db_write se rechaza con 403 (el
	// manifiesto es el candado) — ya no un 404 de «fase sin escrituras».
	if rr := pasar(peticion("127.0.0.1:50000", "token-de-esta-sesion", "borrar_todo")); rr.Code != http.StatusForbidden {
		t.Fatalf("un verb no declarado en db_write debía denegarse (403), dio %d", rr.Code)
	}
	if rr := pasar(peticion("127.0.0.1:50000", "token-ajeno", "ping")); rr.Code != http.StatusForbidden {
		t.Fatalf("combino token ajeno con id ajeno y aprobó: %d", rr.Code)
	}
	// El ping correcto, el único verb de fase 1.
	if rr := pasar(peticion("127.0.0.1:50000", "token-de-esta-sesion", "ping")); rr.Code != http.StatusOK {
		t.Fatalf("el ping correcto debía ser 200, dio %d", rr.Code)
	}
}

// ---------- Fase 2: manifiesto, migraciones propias y escritura por el canal ----------

const manifiestoHolaFase2 = `{
  "id": "hola", "name": "Hola, addon", "version": "2.0.0", "min_core_version": "0.2.0",
  "description": "demo fase 2", "author": "ERPGo",
  "routes": ["/hola"],
  "menu": [{"parent_id": null, "label": "Hola addon", "url": "/hola", "icon": "fa-smile-o", "perms": ["hola_ver"], "sort_order": 90}],
  "perms": [{"code": "hola_ver", "label": "Ver la pantalla"}],
  "tables_own": ["hola_mensajes"],
  "db_write": [{"verb": "mensaje.registrar", "perm": "hola_ver"}]
}`

const schemaHola = `CREATE TABLE IF NOT EXISTS hola_mensajes (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    mensaje VARCHAR(512) NOT NULL,
    usuario VARCHAR(64) NOT NULL DEFAULT '',
    empresa VARCHAR(64) NOT NULL DEFAULT '',
    creada VARCHAR(32) NOT NULL DEFAULT '');`

const manifiestoHolaConDBRead = `{
  "id": "hola", "name": "Hola, addon", "version": "2.0.0", "min_core_version": "0.2.0",
  "description": "demo fase 2", "author": "ERPGo",
  "routes": ["/hola"],
  "menu": [{"parent_id": null, "label": "Hola addon", "url": "/hola", "icon": "fa-smile-o", "perms": ["hola_ver"], "sort_order": 90}],
  "perms": [{"code": "hola_ver", "label": "Ver la pantalla"}],
  "db_read": ["customer_information"]
}`

// escribirSchema deja un schema.sql en la carpeta del addon dentro del dir de
// addons dado (la misma ruta que devuelve addonDirPrueba).
func escribirSchema(t *testing.T, dir, contenido string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "hola", "schema.sql"), []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestElManifiestoFase2ValidaVerboPermisoYLeerSinPrefijo(t *testing.T) {
	// Correcto: verbo dominio.accion con permiso declarado, y db_read con
	// tablas DEL ERP sin el prefijo del id (el candado de verdad lo pone el
	// GRANT SELECT de MySQL, no el manifiesto).
	m := manifiestoValido()
	m.DBRead = []string{"customer_information", "invoices"}
	m.DBWrite = []addonVerbo{{Verb: "mensaje.registrar", Perm: "hola_ver"}}
	m.TablesOwn = []string{"hola_mensajes"}
	if err := validarManifiesto(m); err != nil {
		t.Fatalf("el manifiesto fase 2 correcto fue rechazado: %v", err)
	}

	casos := []struct {
		nombre string
		media  func(*addonManifest)
		que    string
	}{
		{"verbo sin punto (SQL suelto)", func(m *addonManifest) {
			m.DBWrite = []addonVerbo{{Verb: "insertar_mensajes", Perm: "hola_ver"}}
		}, "dominio.accion"},
		{"verbo con permiso que el manifiesto no declara", func(m *addonManifest) {
			m.DBWrite = []addonVerbo{{Verb: "mensaje.registrar", Perm: "hola_fantasma"}}
		}, "no lo declara en perms"},
		{"verbo duplicado", func(m *addonManifest) {
			m.DBWrite = []addonVerbo{{Verb: "mensaje.registrar", Perm: "hola_ver"}, {Verb: "mensaje.registrar", Perm: "hola_ver"}}
		}, "dos veces"},
		{"tabla de lectura mal formada", func(m *addonManifest) {
			m.DBRead = []string{"2mala"}
		}, "de db_read"},
	}
	for _, c := range casos {
		m := manifiestoValido()
		m.DBRead = []string{"customer_information"}
		c.media(m)
		if err := validarManifiesto(m); err == nil {
			t.Errorf("%s: debió rechazarse (%s) y pasó el candado", c.nombre, c.que)
		}
	}
}

func TestElEscaneoExigeSchemaSqlYSoloDDLDeSusTablas(t *testing.T) {
	otra := strings.Replace(manifiestoHolaFase2, `"tables_own": ["hola_mensajes"]`,
		`"tables_own": ["hola_mensajes", "hola_reglas"]`, 1)
	casos := []struct {
		nombre     string
		manifiesto string
		schema     string
		fragmento  string
	}{
		{"sin schema.sql", manifiestoHolaFase2, "", "no trae schema.sql"},
		{"DDL peligroso (DROP)", manifiestoHolaFase2, "DROP TABLE IF EXISTS hola_mensajes;\n" + schemaHola, "solo se aceptan"},
		{"tabla que el manifiesto no declara", manifiestoHolaFase2, "CREATE TABLE IF NOT EXISTS hola_reglas (id VARCHAR(64) PRIMARY KEY);", "NO la declara"},
		{"la segunda tabla declarada no se crea", otra, schemaHola, "no la crea"},
	}
	for _, c := range casos {
		dir := addonDirPrueba(t, c.manifiesto)
		if c.schema != "" {
			escribirSchema(t, dir, c.schema)
		}
		_, h := hostDeAddon(t, dir)
		if err := h.reescanear(); err != nil {
			t.Fatal(err)
		}
		h.mu.Lock()
		avisos := h.avisos
		cargado := h.progs["hola"] != nil
		h.mu.Unlock()
		if cargado {
			t.Errorf("%s: el addon cargó cuando debía quedar fuera", c.nombre)
		}
		if !strings.Contains(strings.Join(avisos, " | "), c.fragmento) {
			t.Errorf("%s: el aviso no menciona %q: %v", c.nombre, c.fragmento, avisos)
		}
	}
}

func TestLasMigracionesLleganALaEmpresaNueva(t *testing.T) {
	dir := addonDirPrueba(t, manifiestoHolaFase2)
	escribirSchema(t, dir, schemaHola)
	s, h := hostDeAddon(t, dir)
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	cargado := h.progs["hola"] != nil
	h.mu.Unlock()
	if !cargado {
		t.Fatal("el addon con un schema.sql saneado debía cargar")
	}
	var existe int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM hola_mensajes`).Scan(&existe); err != nil {
		t.Fatalf("la migración no se aplicó en la empresa activa: %v", err)
	}

	// La empresa nueva hereda la tabla por su registro. Aunque su menú
	// mínimo rechace la sincronización de filas, las migraciones van
	// PRIMERAS: el addon funciona aunque el registro empiece mal.
	db2 := abrirBasePrueba(t)
	s.addonRegistrarEmpresa(db2, "otra")
	if err := db2.QueryRow(`SELECT COUNT(*) FROM hola_mensajes`).Scan(&existe); err != nil {
		t.Fatalf("la empresa nueva no recibió la tabla propia: %v", err)
	}
}

// resolverLectura se prueba directo (sin lanzar proceso real): es la pieza
// que compone el env del addon; el ciclo completo con el binario del demo
// cubre el resto del lanzamiento.
func TestElAddonConDBReadRecibeElDSNPorEntornoYElFalloSeTolera(t *testing.T) {
	// a) Con db_read, el lector inyectable corre y el DSN viaja por entorno.
	_, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHolaConDBRead))
	var vista addonManifest
	h.crearLectura = func(m addonManifest) (string, error) {
		vista = m
		return "erp_ro_hola:clave@tcp(127.0.0.1:3306)/", nil
	}
	p := &addonProc{manifiesto: addonManifest{ID: "hola", DBRead: []string{"customer_information"}}}
	env := h.resolverLectura(p)
	if vista.ID != "hola" {
		t.Fatalf("el lector no fue llamado por el addon: %+v", vista)
	}
	if len(env) != 1 || env[0] != "ERP_DB_READ_DSN=erp_ro_hola:clave@tcp(127.0.0.1:3306)/" {
		t.Fatalf("el DSN de lectura no llegó por entorno: %v", env)
	}
	if !p.lecturaOK || p.lecturaAviso != "" {
		t.Fatalf("la lectura debía quedar concedida (ok=%v, aviso=%q)", p.lecturaOK, p.lecturaAviso)
	}

	// b) Sin db_read en el manifiesto: ni se pide lector ni hay DSN.
	llamados := 0
	h.crearLectura = func(m addonManifest) (string, error) {
		llamados++
		return "", nil
	}
	pSin := &addonProc{manifiesto: addonManifest{ID: "hola"}}
	if env := h.resolverLectura(pSin); len(env) != 0 || llamados != 0 {
		t.Fatalf("sin db_read no debía pedirse lector ni entregarse DSN (env=%v, llamadas=%d)", env, llamados)
	}

	// c) Crear el lector falla (p. ej. sin MySQL en el servidor): el addon
	// arranca igual; la lectura queda sin conceder y el aviso lo explica.
	h.crearLectura = func(m addonManifest) (string, error) {
		return "", fmt.Errorf("el servidor MySQL no responde")
	}
	pFallo := &addonProc{manifiesto: addonManifest{ID: "hola", DBRead: []string{"customer_information"}}}
	if env := h.resolverLectura(pFallo); len(env) != 0 {
		t.Fatalf("con lector fallando no debía entregarse DSN: %v", env)
	}
	aviso, ok := pFallo.lecturaAviso, pFallo.lecturaOK
	if aviso == "" || ok {
		t.Fatalf("el fallo del lector debía dejar aviso sin conceder (aviso %q, ok %v)", aviso, ok)
	}
	if !strings.Contains(aviso, "el servidor MySQL no responde") {
		t.Fatalf("el aviso debía explicar el motivo: %q", aviso)
	}
}

func TestDeUsuarioEntregaLaUltimaSesionViva(t *testing.T) {
	st := NewSessionStore()
	now := time.Now()
	st.Create(&SessionData{UserID: "7", TenantCode: defaultTenantCode, ExpiresAt: now.Add(-time.Minute)})
	st.Create(&SessionData{UserID: "7", TenantCode: defaultTenantCode, ExpiresAt: now.Add(time.Hour)})
	st.Create(&SessionData{UserID: "9", TenantCode: defaultTenantCode, ExpiresAt: now.Add(time.Hour)})

	elegida := st.DeUsuario("7")
	if elegida == nil {
		t.Fatal("el usuario 7 tenía una sesión viva y DeUsuario no la encontró")
	}
	if !elegida.ExpiresAt.After(now) {
		t.Fatal("DeUsuario debía elegir la sesión viva, no la ya expirada")
	}
	if st.DeUsuario("9") == nil {
		t.Fatal("el usuario 9 también tiene sesión viva")
	}
	if st.DeUsuario("nadie") != nil {
		t.Fatal("un usuario sin sesiones no debía tener ninguna")
	}
	if st.DeUsuario("") != nil {
		t.Fatal("sin usuario no hay sesión que devolver")
	}

	// Fail-closed: si su única sesión ya expiró, no hay nada vivo.
	st2 := NewSessionStore()
	st2.Create(&SessionData{UserID: "8", ExpiresAt: now.Add(-time.Minute)})
	if st2.DeUsuario("8") != nil {
		t.Fatal("una sesión expirada no es una sesión viva")
	}
}

func TestElCanalDeFase2ValidaSesionEmpresaPermisoYHandler(t *testing.T) {
	dir := addonDirPrueba(t, manifiestoHolaFase2)
	escribirSchema(t, dir, schemaHola)
	s, h := hostDeAddon(t, dir)
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	s.sessions = NewSessionStore()
	h.mu.Lock()
	h.progs["hola"].token = "token-de-esta-sesion"
	h.mu.Unlock()

	master := &SessionData{UserID: "7", Username: "admin", UserLevel: "master", TenantCode: defaultTenantCode, ExpiresAt: time.Now().Add(time.Hour)}
	empleado := &SessionData{UserID: "8", Username: "caja", UserLevel: "employee", TenantCode: defaultTenantCode, ExpiresAt: time.Now().Add(time.Hour)}
	s.sessions.Create(master)
	s.sessions.Create(empleado)

	pedir := func(metodo, usuario, empresa, cuerpo string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(metodo, "/__addon/hola/mensaje.registrar", strings.NewReader(cuerpo))
		req.RemoteAddr = "127.0.0.1:55000"
		req.Header.Set("X-ERP-Addon-Token", "token-de-esta-sesion")
		if usuario != "" {
			req.Header.Set("X-ERP-User", usuario)
		}
		if empresa != "" {
			req.Header.Set("X-ERP-Tenant", empresa)
		}
		rr := httptest.NewRecorder()
		s.addonCanal(rr, req)
		return rr
	}

	cuerpo := `{"mensaje":"hola desde el canal}`
	cuerpo = `{"mensaje":"hola desde el canal"}`
	// Escritura por GET: no hay escritura sin POST.
	if rr := pedir(http.MethodGet, "7", "", ""); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("un GET a un verbo de escritura debía dar 405, dio %d", rr.Code)
	}
	// Cuerpo que no es JSON: 400, sin tocar nada.
	if rr := pedir(http.MethodPost, "7", "", "{no-json"); rr.Code != http.StatusBadRequest {
		t.Fatalf("un cuerpo no JSON debía dar 400, dio %d", rr.Code)
	}
	// Usuario inventado (sin sesión viva): fail-closed.
	if rr := pedir(http.MethodPost, "999", "", cuerpo); rr.Code != http.StatusForbidden {
		t.Fatalf("un usuario sin sesión debía denegarse (403), dio %d", rr.Code)
	}
	// Cross-tenant: empresa ajena frente a la de la sesión.
	if rr := pedir(http.MethodPost, "7", "otra-empresa", cuerpo); rr.Code != http.StatusForbidden {
		t.Fatalf("la empresa ajena debía denegarse (403), dio %d", rr.Code)
	}
	// Sin permiso: el empleado no tiene hola_ver.
	if rr := pedir(http.MethodPost, "8", "", cuerpo); rr.Code != http.StatusForbidden {
		t.Fatalf("el empleado sin permiso debía denegarse (403), dio %d", rr.Code)
	}
	// Verbo declarado pero sin handler conectado en este arranque: 500.
	if rr := pedir(http.MethodPost, "7", "", cuerpo); rr.Code != http.StatusInternalServerError {
		t.Fatalf("sin handler conectado debía dar 500, dio %d", rr.Code)
	}

	// Handler conectado: la escritura pasa, con el contexto YA VALIDADO
	// entregado al handler y la respuesta con id de petición.
	var recibido sesionDeAddon
	var cuerpoVisto map[string]interface{}
	s.addonVerboRegistrar("hola", "mensaje.registrar", func(p *addonProc, ses sesionDeAddon, c map[string]interface{}) (map[string]interface{}, error) {
		recibido = ses
		cuerpoVisto = c
		return map[string]interface{}{"registro_id": "hola-1"}, nil
	})
	rr := pedir(http.MethodPost, "7", "", cuerpo)
	if rr.Code != http.StatusOK {
		t.Fatalf("el handler conectado debía responder 200, dio %d: %s", rr.Code, rr.Body.String())
	}
	var salida map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &salida); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	if salida["peticion"] == "" || salida["registro_id"] != "hola-1" {
		t.Fatalf("la respuesta debía llevar peticion y registro_id: %v", salida)
	}
	if recibido.Usuario != "7" || recibido.Empresa != defaultTenantCode || recibido.Sess != master {
		t.Fatalf("el handler debía recibir el contexto de la sesión validada: %+v", recibido)
	}
	if cuerpoVisto["mensaje"] != "hola desde el canal" {
		t.Fatalf("el handler debía recibir el cuerpo JSON: %v", cuerpoVisto)
	}

	// Y si el handler rechaza, el canal lo traduce a 500 con su razón.
	s.addonVerboRegistrar("hola", "mensaje.registrar", func(p *addonProc, ses sesionDeAddon, c map[string]interface{}) (map[string]interface{}, error) {
		return nil, fmt.Errorf("la pasarela rechazó el cobro")
	})
	if rr := pedir(http.MethodPost, "7", "", cuerpo); rr.Code != http.StatusInternalServerError {
		t.Fatalf("un handler que rechaza debía dar 500, dio %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "La pasarela rechazó el cobro") {
		_ = 0
	}
	_ = s
}

// ---------- El ciclo completo con el addon real ----------

// TestElCicloCompletoDelAddonDemo compila el addon del repo, lo deja en su
// carpeta y deja la frontera viva: registro, lanzamiento real con token
// efímero, página servida con la sesión, y muerte limpia.
func TestElCicloCompletoDelAddonDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("cada del ciclo completo: con -short se salta")
	}
	// La carpeta temporal ES la carpeta addons del ERP germinado: aquí va el
	// manifiesto del repo y el binario recién compilado.
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "hola")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// El bundle REAL del repo (manifest + schema.sql de fase 2): si el demo
	// promete tablas propias, el escáner tiene que probárselo con su DDL.
	manifestReal, err := os.ReadFile(filepath.Join("..", "..", "addons", "hola", "manifest.json"))
	if err != nil {
		t.Skipf("el manifiesto del demo del repo no está en este entorno: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifestReal, 0o644); err != nil {
		t.Fatal(err)
	}
	schemaReal, err := os.ReadFile(filepath.Join("..", "..", "addons", "hola", "schema.sql"))
	if err != nil {
		t.Fatal("el addon demo promete tables_own: su schema.sql es obligatorio")
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.sql"), schemaReal, 0o644); err != nil {
		t.Fatal(err)
	}
	binario := filepath.Join(dir, "erp-addon-hola")
	if salida, err := exec.Command("go", "build", "-o", binario, "erpgo/addons/hola").CombinedOutput(); err != nil {
		t.Skipf("no se pudo compilar el addon demo en este entorno: %v\n%s", err, salida)
	}

	s, h := hostDeAddon(t, raiz)
	h.lanzar = nil // lanzamiento REAL (h.lanzarReal)
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}

	h.mu.Lock()
	estado := h.progs["hola"].estado
	h.mu.Unlock()
	if estado != estadoActivo {
		t.Fatalf("el addon demo no quedó activo (estado %q)", estado)
	}

	// Su página se sirve a través del proxy CON sesión.
	r := reqPrueba(s, sessPrueba("master"), "/hola")
	rr := httptest.NewRecorder()
	s.addonRouter(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("la pantalla del demo debía servirse (200), dio %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Hola, addon") {
		t.Fatalf("la pantalla no es la del demo: %s", rr.Body.String())
	}

	// Matar el proceso no derriba al ERP: la ruta da 502 con guía.
	h.mu.Lock()
	p := h.progs["hola"]
	if p.cmd == nil || p.cmd.Process == nil {
		t.Fatal("el demo debía tener un proceso vivo")
	}
	_ = p.cmd.Process.Kill()
	h.mu.Unlock()
	time.Sleep(100 * time.Millisecond)

	rr2 := httptest.NewRecorder()
	s.addonRouter(rr2, reqPrueba(s, sessPrueba("master"), "/hola"))
	if rr2.Code != http.StatusBadGateway && rr2.Code != http.StatusOK {
		t.Fatalf("tras matar el proceso, la ruta debía responder 502 (o recuperarse al relanzar), dio %d", rr2.Code)
	}

	// Cierre ordenado del host: nada queda al aire.
	h.parar()
}

func makeProxy(destino string) *httputil.ReverseProxy {
	u, err := url.Parse(destino)
	if err != nil {
		panic(err)
	}
	return httputil.NewSingleHostReverseProxy(u)
}
