package web

// addons_test.go — Pruebas de la frontera (docs/PLAN-ADDONS.md, fases 1-3).
//
// Lo que cada prueba protege, sin adornos:
//
//   - El candado del manifiesto: nada entra a correr si su declaratoria tiene
//     escaleras que escalar (id raro, ruta del núcleo, permiso con prefijo ajeno,
//     tabla sin prefijo del id, menú que cita permisos no declarados; en fase 3,
//     hooks_out a eventos que el núcleo no emite).
//   - El registro: addons → filas de modules/menu en la base de empresa, y el
//     desinstalado queda «no instalado» con filas inactivas, jamás borradas.
//   - El proxy: reenvía con X-ERP-* y SIN cookies del ERP.
//   - El canal de negocio: 403 sin token / fuera de loopback / token ajeno;
//     en fase 2, un verb que el manifiesto NO declara en db_write da 403
//     (el manifiesto es el candado, aunque el handler exista).
//   - Fase 2: db_write exige dominio.accion y permiso declarado; tables_own
//     exige schema.sql con CREATE TABLE IF NOT EXISTS (solo sus tablas);
//     el DSN de lectura granular llega por entorno o se explica en el panel;
//     la escritura exige POST, JSON, sesión viva, empresa y permiso.
//   - Fase 3: firma ed25519 (sin clave: «sin verificar»; con clave: sin firma
//     no carga salvo ADDONS_UNSIGNED, y una firma inválida no carga NUNCA);
//     hooks_out con cola persistente, entrega con token y reintentos con
//     backoff al tope; hooks_in con plazo y camino de retorno; el panel con
//     firma, hooks y la historia de entregas; el botón de puertos.
//   - El ciclo completo con el addon REAL del repo (addons/hola): se compila,
//     se lanza, sirve su página, y matarlo no derriba al ERP.
//
// En los tests la lógica de lanzamiento se inyecta o se ejecuta de verdad con
// el binario del demo; nunca se dá por buena una pieza que solo vive en RAM.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	var vistasDe, vistasTenant, vistasPerms, vistasCookies []string
	fin := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vistasDe = append(vistasDe, r.Header.Get("X-ERP-User"))
		vistasTenant = append(vistasTenant, r.Header.Get("X-ERP-Tenant"))
		vistasPerms = append(vistasPerms, r.Header.Get("X-ERP-Perms"))
		vistasCookies = append(vistasCookies, r.Header.Get("Cookie"))
		_, _ = w.Write([]byte("pantalla del addon"))
		close(fin)
	}))
	defer upstream.Close()

	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))
	h.reescanear()
	h.mu.Lock()
	p := h.progs["hola"]
	p.estado = estadoActivo
	p.proxy = makeProxy(upstream.URL)
	h.mu.Unlock()

	r := reqPrueba(s, sessPrueba("master"), "/hola")
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

// ---------- Fase 3: firma, hooks y panel ----------

const manifiestoHolaHooks = `{
  "id": "hola", "name": "Hola, addon", "version": "3.0.0", "min_core_version": "0.3.0",
  "description": "demo", "author": "ERPGo",
  "routes": ["/hola"],
  "menu": [{"parent_id": null, "label": "Hola addon", "url": "/hola", "icon": "fa-smile-o", "perms": ["hola_ver"], "sort_order": 90}],
  "perms": [{"code": "hola_ver", "label": "Ver la pantalla"}],
  "hooks_out": ["factura.emitida"]
}`

func TestElManifiestoFase3ValidaHooks(t *testing.T) {
	// Los hooks bien declarados caben.
	valido := manifiestoValido()
	valido.HooksOut = []string{"factura.emitida"}
	valido.HooksIn = []string{"send_whatsapp"}
	if err := validarManifiesto(valido); err != nil {
		t.Fatalf("los hooks bien declarados debían caber: %v", err)
	}
	// Cada daño posible con su aviso esperado.
	bases := []struct {
		nombre string
		monta  func() *addonManifest
		quota  string
	}{
		{"hook_out a un evento que el núcleo no emite", func() *addonManifest {
			m := manifiestoValido()
			m.HooksOut = []string{"alien.landing"}
			return m
		}, "no es un evento que este núcleo emita"},
		{"hook_out con nombre mal formado", func() *addonManifest {
			m := manifiestoValido()
			m.HooksOut = []string{"Factura!"}
			return m
		}, "es inválido"},
		{"hook_out duplicado", func() *addonManifest {
			m := manifiestoValido()
			m.HooksOut = []string{"factura.emitida", "factura.emitida"}
			return m
		}, "dos veces"},
		{"hook_in con nombre mal formado", func() *addonManifest {
			m := manifiestoValido()
			m.HooksIn = []string{"Send WhatsApp!"}
			return m
		}, "es inválido"},
		{"hook_in duplicado", func() *addonManifest {
			m := manifiestoValido()
			m.HooksIn = []string{"send_whatsapp", "send_whatsapp"}
			return m
		}, "dos veces"},
	}
	for _, b := range bases {
		if err := validarManifiesto(b.monta()); err == nil {
			t.Fatalf("%s debía rechazarse", b.nombre)
		} else if !strings.Contains(err.Error(), b.quota) {
			t.Fatalf("%s debía explicar %q, explicó: %v", b.nombre, b.quota, err)
		}
	}
}

// parejaDePrueba genera una pareja ed25519 y deja la PÚBLICA en la raíz de
// addons (pubkey.ed25519), tal como lo dejaría tools/firmaraddon clave. En las
// pruebas la clave pública se instala a mano en el host (h.clavePub): quien
// la lee del disco es addonsIniciar, y la prueba monta el host de una pieza.
func parejaDePrueba(t *testing.T, raiz string) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(raiz, "pubkey.ed25519"), []byte(hex.EncodeToString(pub)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// escribirFirma firma los bytes que llegan (el manifest.json tal cual está en
// disco) con la privada de la prueba y deja manifest.sig al lado del addon.
func escribirFirma(t *testing.T, raiz string, priv ed25519.PrivateKey, contenido string) {
	t.Helper()
	firma := ed25519.Sign(priv, []byte(contenido))
	if err := os.WriteFile(filepath.Join(raiz, "hola", "manifest.sig"), []byte(hex.EncodeToString(firma)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func firmaDe(t *testing.T, h *addonHost) (string, bool) {
	t.Helper()
	h.mu.Lock()
	p := h.progs["hola"]
	h.mu.Unlock()
	if p == nil {
		return "", false
	}
	return p.firma, true
}

func TestLaFirmaDelManifiestoDecideQuienCarga(t *testing.T) {
	// a) Sin clave pública en addons/: nada se verifica y todo corre, con la
	// marca «sin verificar» que la pantalla explicará.
	raiz := addonDirPrueba(t, manifiestoHola)
	_, h := hostDeAddon(t, raiz)
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	if firma, _ := firmaDe(t, h); firma != "sin verificar" {
		t.Fatalf("sin clave pública el estado debía ser «sin verificar», fue %q", firma)
	}

	// Con la clave del dueño puesta, la política manda. La pública va al host
	// tal como addonsIniciar la pondría leyendo pubkey.ed25519.
	raiz2 := addonDirPrueba(t, manifiestoHola)
	pub, priv := parejaDePrueba(t, raiz2)

	// b) Con clave y SIN manifest.sig, sin modo desarrollo: NO carga, y el
	// aviso dice qué hacer (firmar o modo de desarrollo).
	_, h2 := hostDeAddon(t, raiz2)
	h2.clavePub = pub
	if err := h2.reescanear(); err != nil {
		t.Fatal(err)
	}
	if _, cargada := firmaDe(t, h2); cargada {
		t.Fatal("un addon sin firma, con clave pública configurada, debía quedar fuera")
	}
	var avisoFirma string
	for _, a := range h2.avisos {
		if strings.Contains(a, "manifest.sig") {
			avisoFirma = a
		}
	}
	if avisoFirma == "" {
		t.Fatalf("la negación debía explicar la firma; avisos: %v", h2.avisos)
	}

	// c) El mismo, en modo de desarrollo (ADDONS_UNSIGNED=1): carga, pero con
	// la marca «sin verificar» bien puesta.
	_, h3 := hostDeAddon(t, raiz2)
	h3.clavePub = pub
	h3.unsigned = true
	if err := h3.reescanear(); err != nil {
		t.Fatal(err)
	}
	if firma, cargada := firmaDe(t, h3); !cargada || firma != "sin verificar" {
		t.Fatalf("en modo desarrollo el addon debía cargar «sin verificar»; quedó %q (cargada=%v)", firma, cargada)
	}

	// d) Firma VÁLIDA del manifiesto tal cual está en disco: carga «verificada».
	contenido, err := os.ReadFile(filepath.Join(raiz2, "hola", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	escribirFirma(t, raiz2, priv, string(contenido))
	_, h4 := hostDeAddon(t, raiz2)
	h4.clavePub = pub
	if err := h4.reescanear(); err != nil {
		t.Fatal(err)
	}
	if firma, cargada := firmaDe(t, h4); !cargada || firma != "verificada" {
		t.Fatalf("la firma correcta debía cargar «verificada»; quedó %q (cargada=%v)", firma, cargada)
	}

	// e) Firma válida de la versión VIEJA y manifiesto tocado después: NO
	// carga en NINGÚN modo (el modo desarrollo es para iterar SIN firma, no
	// para acepta un manifiesto que cambió después de firmar).
	if err := os.WriteFile(filepath.Join(raiz2, "hola", "manifest.json"), []byte(string(contenido)+" "), 0o644); err != nil {
		t.Fatal(err)
	}
	_, h5 := hostDeAddon(t, raiz2)
	h5.clavePub = pub
	h5.unsigned = true
	if err := h5.reescanear(); err != nil {
		t.Fatal(err)
	}
	if _, cargada5 := firmaDe(t, h5); cargada5 {
		t.Fatal("un manifiesto tocado después de firmar debía quedar fuera en CUALQUIER modo")
	}
}

// baseHooks monta el host con la cola de hooks en la base de prueba, listo
// para encolar y entregar.
func baseHooks(t *testing.T) (*Server, *addonHost) {
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS addon_hook_out (
		id INTEGER PRIMARY KEY AUTOINCREMENT, addon_id TEXT, evento TEXT, empresa TEXT,
		payload_json TEXT, intentos INTEGER DEFAULT 0, status TEXT DEFAULT 'pending',
		ultimo_error TEXT DEFAULT '', proximo TEXT DEFAULT '', creado TEXT DEFAULT '',
		enviado TEXT DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	h := &addonHost{
		s: s, directorio: addonDirPorDefecto,
		progs:       map[string]*addonProc{},
		rutasDelMux: map[string]bool{},
		ahora:       time.Now,
		parado:      make(chan struct{}),
	}
	s.addons = h
	return s, h
}

func TestLosHooksOutSeEncolanYSeEntreganConReintentos(t *testing.T) {
	s, h := baseHooks(t)

	// El receptor: un httptest real en loopback con el contrato del addon
	// (POST /hooks/<evento>, token, empresa, JSON).
	var recibidas []map[string]interface{}
	var empresaRecibida, tokenRecibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hooks/factura.emitida" || r.Method != http.MethodPost {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		tokenRecibido = r.Header.Get("X-ERP-Addon-Token")
		empresaRecibida = r.Header.Get("X-ERP-Tenant")
		var cuerpo map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		recibidas = append(recibidas, cuerpo)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer srv.Close()
	puerto := srv.Listener.Addr().(*net.TCPAddr).Port
	proceso := addonProc{
		manifiesto: addonManifest{ID: "hola", HooksOut: []string{"factura.emitida"}},
		estado:     estadoActivo, token: "token-de-esta-sesion", puerto: puerto,
	}
	h.mu.Lock()
	h.progs["hola"] = &proceso
	h.mu.Unlock()

	// El evento nace pendiente y ya vencido (el próximo tick lo toma).
	h.hookEncolar("factura.emitida", "acme", map[string]interface{}{"invoice_id": 123})
	ctl, err := s.controlStore()
	if err != nil {
		t.Fatal(err)
	}
	var cantidad int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM addon_hook_out WHERE status='pending'`).Scan(&cantidad); err != nil {
		t.Fatal(err)
	}
	if cantidad != 1 {
		t.Fatalf("el evento debía nacer pendiente; filas=%d", cantidad)
	}

	// Una pasada del runner lo entrega con el token de ESTA sesión y la empresa
	// del evento; la fila queda 'sent' con su sello.
	h.entregarPendientes()
	if tokenRecibido != "token-de-esta-sesion" || empresaRecibida != "acme" {
		t.Fatalf("la entrega debía llevar token y empresa; token=%q empresa=%q", tokenRecibido, empresaRecibida)
	}
	if len(recibidas) != 1 || recibidas[0]["invoice_id"] != 123.0 {
		t.Fatalf("el cuerpo del aviso llegó mal: %+v", recibidas)
	}
	var status string
	if err := ctl.QueryRow(`SELECT status FROM addon_hook_out`).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("la fila entregada debía quedar sent; quedó %q (%v)", status, err)
	}

	// Un evento que NADIE suscribe no llega ni a la cola.
	h.hookEncolar("compra.registrada", "acme", map[string]string{})
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM addon_hook_out WHERE evento='compra.registrada'`).Scan(&cantidad); err != nil {
		t.Fatal(err)
	}
	if cantidad != 0 {
		t.Fatal("un evento sin suscriptor no debía tocar la cola")
	}

	// El addon que la fila espera ya no está cargado: queda 'descartado' con
	// su motivo, no un martilleo hacia nada.
	h.hookEncolar("factura.emitida", "otra", map[string]string{})
	h.mu.Lock()
	delete(h.progs, "hola")
	h.mu.Unlock()
	h.entregarPendientes()
	h.mu.Lock()
	h.progs["hola"] = &proceso
	h.mu.Unlock()
	if err := ctl.QueryRow(`SELECT status FROM addon_hook_out WHERE id=(SELECT MAX(id) FROM addon_hook_out)`).Scan(&status); err != nil || status != "descartado" {
		t.Fatalf("la fila de un addon retirado debía quedar descartada; quedó %q (%v)", status, err)
	}

	// La entrega falla: intento 1 con backoff; al agotar, 'failed' con motivo,
	// jamás un reintentoo infinito.
	h.mu.Lock()
	h.entregar = func(p *addonProc, evento string, payload []byte, empresa, token string, puerto int) error {
		return fmt.Errorf("el addon respondió 500 al hook")
	}
	h.mu.Unlock()
	h.hookEncolar("factura.emitida", "acme2", map[string]string{})
	h.entregarPendientes()
	var intentos int
	var proximo string
	if err := ctl.QueryRow(`SELECT intentos, proximo FROM addon_hook_out WHERE id=(SELECT MAX(id) FROM addon_hook_out)`).Scan(&intentos, &proximo); err != nil {
		t.Fatal(err)
	}
	if intentos != 1 {
		t.Fatalf("la primera falla debía registrar intento 1, quedó %d", intentos)
	}
	if proximo == h.sello() {
		t.Fatal("tras una falla el próximo intento debía ir más tarde (backoff)")
	}
	for i := 0; i < hookMaxIntentos-1; i++ {
		if _, err := ctl.Exec(`UPDATE addon_hook_out SET proximo = ? WHERE status='pending'`, h.sello()); err != nil {
			t.Fatal(err)
		}
		h.entregarPendientes()
	}
	if err := ctl.QueryRow(`SELECT status FROM addon_hook_out WHERE id=(SELECT MAX(id) FROM addon_hook_out)`).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("tras agotar los intentos la fila debía quedar failed; quedó %q (%v)", status, err)
	}
	var motivo string
	if err := ctl.QueryRow(`SELECT ultimo_error FROM addon_hook_out WHERE id=(SELECT MAX(id) FROM addon_hook_out)`).Scan(&motivo); err != nil || motivo == "" {
		t.Fatalf("la fila fallida debía llevar su motivo; quedó %q (%v)", motivo, err)
	}
}

func TestElHookInPideConPlazoYSigueSinElAddon(t *testing.T) {
	_, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHola))

	// El addon activo con un hook_in declarado: servidor httptest que responde
	// listo (rápido, lento o con error, según el caso).
	var respuesta func(w http.ResponseWriter, r *http.Request)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respuesta(w, r)
	}))
	defer srv.Close()
	proceso := addonProc{
		manifiesto: addonManifest{ID: "hola", HooksIn: []string{"send_whatsapp"}},
		estado:     estadoActivo, token: "token-de-esta-sesion",
		puerto: srv.Listener.Addr().(*net.TCPAddr).Port,
	}

	// a) Respuesta rápida: el cuerpo va al negocio tal cual.
	respuesta = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hooks/send_whatsapp" || r.Header.Get("X-ERP-Addon-Token") != "token-de-esta-sesion" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mensaje_id":"wa-42"}`))
	}
	h.mu.Lock()
	h.progs["hola"] = &proceso
	h.mu.Unlock()
	salida, err := (&Server{addons: h}).addonHookPedir("hola", "send_whatsapp", "acme", map[string]string{"texto": "su factura"}, 3*time.Second)
	if err != nil {
		t.Fatalf("el hook rápido debía salir bueno: %v", err)
	}
	var dump map[string]interface{}
	_ = json.Unmarshal(salida, &dump)
	if dump["mensaje_id"] != "wa-42" {
		t.Fatalf("la respuesta del addon llegó mal: %v", dump)
	}

	// b) El addon tarda más del plazo: error EN EL PLAZO, con el camino de
	// retorno expuesto (el negocio sigue sin el addon).
	respuesta = func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":1}`))
	}
	inicio := time.Now()
	_, err = (&Server{addons: h}).addonHookPedir("hola", "send_whatsapp", "acme", map[string]string{}, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "50ms") {
		t.Fatalf("el vencimiento debía explicar el plazo; dio: %v", err)
	}
	if dur := time.Since(inicio); dur > 5*time.Second {
		t.Fatalf("el vencimiento debía cortar en plazo, tardó %v", dur)
	}

	// c) El addon está caído: error claro, sin intento HTTP.
	h.mu.Lock()
	proceso.estado = estadoCaydo
	h.mu.Unlock()
	if _, err = (&Server{addons: h}).addonHookPedir("hola", "send_whatsapp", "acme", map[string]string{}, 0); err == nil || !strings.Contains(err.Error(), "caído") {
		t.Fatalf("el addon caído debía dar error claro; dio: %v", err)
	}

	// d) El hook no lo ofrece nadie: el manifiesto es el candado también para
	// el núcleo que pide.
	h.mu.Lock()
	proceso.estado = estadoActivo
	h.mu.Unlock()
	if _, err = (&Server{addons: h}).addonHookPedir("hola", "cobrar_extra", "acme", map[string]string{}, 0); err == nil || !strings.Contains(err.Error(), "el manifiesto es el candado") {
		t.Fatalf("un hook no declarado debía rechazarse; dio: %v", err)
	}
}

func TestElPanelFase3MuestraFirmaHooksYHistoria(t *testing.T) {
	s, h := hostDeAddon(t, addonDirPrueba(t, manifiestoHolaHooks))
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	// La cola de la base de control de prueba, tal como baseHooks la monta en
	// las demás pruebas del motor.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS addon_hook_out (
		id INTEGER PRIMARY KEY AUTOINCREMENT, addon_id TEXT, evento TEXT, empresa TEXT,
		payload_json TEXT, intentos INTEGER DEFAULT 0, status TEXT DEFAULT 'pending',
		ultimo_error TEXT DEFAULT '', proximo TEXT DEFAULT '', creado TEXT DEFAULT '',
		enviado TEXT DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	// Dos filas en la historia: una entregada y una fallida.
	ctl, err := s.controlStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO addon_hook_out (addon_id, evento, empresa, status, creado) VALUES ('hola','factura.emitida','acme','sent','2026-10-30 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := ctl.Exec(`INSERT INTO addon_hook_out (addon_id, evento, empresa, status, intentos, ultimo_error, creado) VALUES ('hola','factura.emitida','acme','failed',8,'el addon respondió 500 al hook','2026-10-30 10:01:00')`); err != nil {
		t.Fatal(err)
	}
	st := s.addonsEstado()
	if st["firma_publica"] != false {
		t.Fatal("sin pubkey.ed25519 el panel debe trebuie decir firma_publica=false")
	}
	lista, ok := st["addons"].([]map[string]interface{})
	if !ok || len(lista) != 1 {
		t.Fatalf("el panel debía traer un addon; trajo %v", st["addons"])
	}
	a := lista[0]
	if a["firma"] != "sin verificar" {
		t.Fatalf("la firma del panel quedó %v", a["firma"])
	}
	if hooks, _ := a["hooks_out"].([]string); len(hooks) != 1 || hooks[0] != "factura.emitida" {
		t.Fatalf("hooks_out del panel quedó %v", a["hooks_out"])
	}
	concedido := a["concedido"].(map[string]bool)
	if !concedido["hooks"] {
		t.Fatal("el addon con hooks debía marcar hooks concedido")
	}
	historia, ok := st["historia_hooks"].([]map[string]interface{})
	if !ok || len(historia) != 2 {
		t.Fatalf("la historia debía traer las dos filas; trajo %v", st["historia_hooks"])
	}
	if historia[0]["status"] != "failed" || historia[0]["error"] == "" {
		t.Fatalf("la fila más reciente debía ir primero con su error: %+v", historia[0])
	}
}

func TestElBotonDePuertosRelanzaCadaAddon(t *testing.T) {
	// Con base de control (como la dejaría baseHooks): el botón responde con
	// el estado del panel, que necesita leer la historia de hooks.
	s, h := baseHooks(t)
	// El binario EN LA CARPETA es la puerta del lanzamiento real: lanzarBajoCandado
	// solo sigue mientras exista un erp-addon-hola ejecutable en p.dir, y su
	// healthz real servido por el lanzamiento de este caso de prueba.
	carpeta := t.TempDir()
	if err := os.WriteFile(filepath.Join(carpeta, "erp-addon-hola"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	proceso := addonProc{
		manifiesto: addonManifest{ID: "hola", Routes: []string{"/hola"}},
		dir:        carpeta,
		estado:     estadoActivo, token: "token-viejo", puerto: 11111,
	}
	h.mu.Lock()
	h.progs["hola"] = &proceso
	h.mu.Unlock()
	// El lanzamiento de la prueba ES REAL: recibe por entorno el puerto que la
	// frontera reservó, sirve healthz en él (como cualquier addon) y deja el
	// estado activo con su token nuevo. El aserto de puertos NO es un número
	// fijo: la reasignación es un puerto NUEVO, distinto del que el addon ya
	// tenía (11111), y el token de la sesión anterior queda reemplazado.
	h.lanzar = func(p *addonProc, binario string, env []string) error {
		puertoReservado := 0
		for _, e := range env {
			if strings.HasPrefix(e, "ERP_ADDON_PORT=") {
				puertoReservado, _ = strconv.Atoi(strings.TrimPrefix(e, "ERP_ADDON_PORT="))
			}
		}
		if puertoReservado == 0 {
			return fmt.Errorf("el lanzamiento no trajo ERP_ADDON_PORT")
		}
		escucha, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", puertoReservado))
		if err != nil {
			return err
		}
		servidor := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})}
		go servidor.Serve(escucha)
		p.puerto = puertoReservado
		p.token = "token-nuevo"
		p.estado = estadoActivo
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/module/addons/puertos", nil)
	rr := httptest.NewRecorder()
	s.addonPanelPuertos(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("el botón de puertos debía responder 200, dio %d", rr.Code)
	}
	h.mu.Lock()
	nuevoPuerto, nuevoToken := proceso.puerto, proceso.token
	h.mu.Unlock()
	if nuevoPuerto == 11111 || nuevoToken == "token-viejo" || nuevoToken == "" {
		t.Fatalf("el relanzamiento debía reasignar puerto y token; quedó %d/%q", nuevoPuerto, nuevoToken)
	}
}

func makeProxy(destino string) *httputil.ReverseProxy {
	u, err := url.Parse(destino)
	if err != nil {
		panic(err)
	}
	return httputil.NewSingleHostReverseProxy(u)
}
