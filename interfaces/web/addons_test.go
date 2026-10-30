package web

// addons_test.go — Pruebas de la frontera fase 1 (docs/PLAN-ADDONS.md).
//
// Lo que cada prueba protege, sin adornos:
//
//   - El candado del manifiesto: nada entra a correr si su declaratoria tiene
//     escaleras que escalar (id raro, ruta del núcleo, permiso con prefijo ajeno,
//     tabla sin prefijo del id, menú que cita permisos no declarados).
//   - El registro: addons → filas de modules/menu en la base de empresa, y el
//     desinstalado queda «no instalado» con filas inactivas, jamás borradas.
//   - El proxy: reenvía con X-ERP-* y SIN cookies del ERP.
//   - El canal de negocio: 403 sin token / fuera de loopback / token ajeno,
//     404 con verb que no existe en la fase.
//   - El ciclo completo con el addon REAL del repo (addons/hola): se compila,
//     se lanza, sirve su página, y matarlo no derriba al ERP.
//
// En los tests la lógica de lanzamiento se inyecta o se ejecuta de verdad con
// el binario del demo; nunca se dá por buena una pieza que solo vive en RAM.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"net/http/httputil"
	"net/url"
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
	if rr := pasar(peticion("127.0.0.1:50000", "token-de-esta-sesion", "borrar_todo")); rr.Code != http.StatusNotFound {
		t.Fatalf("un verb fuera de fase debía dar 404, dio %d", rr.Code)
	}
	if rr := pasar(peticion("127.0.0.1:50000", "token-ajeno", "ping")); rr.Code != http.StatusForbidden {
		t.Fatalf("combino token ajeno con id ajeno y aprobó: %d", rr.Code)
	}
	// El ping correcto, el único verb de fase 1.
	if rr := pasar(peticion("127.0.0.1:50000", "token-de-esta-sesion", "ping")); rr.Code != http.StatusOK {
		t.Fatalf("el ping correcto debía ser 200, dio %d", rr.Code)
	}
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
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifiestoHola), 0o644); err != nil {
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
