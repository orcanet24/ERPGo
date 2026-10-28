package web

// Pruebas del bloque A de rendimiento: las cachés de pintado de cachas.go.
//
// Lo que de verdad se comprueba aquí no es «que haya caché», sino que la caché
// NO puede cambiar tres respuestas que son de seguridad:
//
//  1. El aviso de clave de fábrica aparece en el acto cuando la contraseña
//     cambia, en AMBOS sentidos (de fábrica a normal y de normal a fábrica),
//     sin que nadie tenga que invalidar nada.
//  2. Lo que se cachea para pintar es SÓLO pintar: hasPermission, que es la
//     puerta de ejecución, sigue viendo la base fresca en cada llamada.
//  3. El menú cacheado sigue filtrándose por permisos EN CADA PETICIÓN: nadie
//     ve una entrada que no le toca porque las filas vengan de la caché.
//
// La cuarta cosa que se comprueba es el motivo del cambio: que la huella de un
// estático dependa de SU fichero y no de todo static/, y que cada recurso que
// pide el layout exista de verdad.

import (
	"database/sql"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"erpgow"
	"erpgow/configs"
	"erpgow/infrastructure/persistence"

	"golang.org/x/crypto/bcrypt"
)

// basePrueba crea una base SQLite con las tablas que estas pruebas necesitan y
// un Server montado SOBRE esa base, como lo monta NewServer (controlDB + db).
func basePrueba(t *testing.T) (*Server, *sql.DB) {
	s, db := &Server{config: &configs.Config{AppEnv: "test", AppName: "ERPGo"}}, abrirBasePrueba(t)
	s.db = db
	// Igual que NewServer: el handle de control YA está abierto, y el esquema
	// no se aplica porque estas tablas las crea la prueba.
	s.controlDB = db
	s.controlListo = true
	return s, db
}

// abrirBasePrueba crea una base SQLite con el mismo juego de tablas. Se abre
// una por empresa cuando una prueba necesita comprobar que dos empresas no se
// mezclan.
func abrirBasePrueba(t *testing.T) *sql.DB {
	t.Helper()
	db, err := persistence.OpenSQLite(filepath.Join(t.TempDir(), "prueba.sqlite3"))
	if err != nil {
		t.Fatalf("abrir base de prueba: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	crearTablasPrueba(t, db)
	return db
}

func crearTablasPrueba(t *testing.T, db *sql.DB) {
	t.Helper()
	tablas := []string{
		`CREATE TABLE user_login (username TEXT PRIMARY KEY, password TEXT)`,
		`CREATE TABLE sub_module (id INTEGER PRIMARY KEY, directory TEXT, status INTEGER DEFAULT 1)`,
		`CREATE TABLE sec_role (id INTEGER PRIMARY KEY, type TEXT, level TEXT)`,
		`CREATE TABLE sec_userrole (user_id TEXT, roleid INTEGER)`,
		`CREATE TABLE role_permission (role_id INTEGER, fk_module_id INTEGER, "create" INTEGER, "read" INTEGER, "update" INTEGER, "delete" INTEGER)`,
		`CREATE TABLE menu (id INTEGER PRIMARY KEY, parent_id INTEGER, label TEXT, url TEXT, icon TEXT, perm TEXT, active_prefix TEXT, sort_order INTEGER, status INTEGER DEFAULT 1)`,
		`CREATE TABLE audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, action TEXT, module TEXT, table_name TEXT, record_id TEXT, old_value TEXT, new_value TEXT, ip_address TEXT, created_at TEXT, prev_hash TEXT, record_hash TEXT)`,
		`CREATE TABLE audit_chain_blocks (period TEXT PRIMARY KEY, prev_block_hash TEXT, start_hash TEXT, end_hash TEXT, first_id INTEGER, last_id INTEGER, record_count INTEGER, block_hash TEXT, sealed_at TEXT)`,
	}
	for _, ddl := range tablas {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla: %v\n%s", err, ddl)
		}
	}
}

func sessPrueba(nivel string) *SessionData {
	return &SessionData{UserID: "7", Username: "admin", Fullname: "Admin", UserLevel: nivel}
}

// reqPrueba monta una petición CON su ámbito (empresa + identidad), igual que
// attachScope en el ciclo real. Sin el ámbito, sessionFromRequest no encuentra
// identidad y, por ejemplo, hasPermission respondería siempre false —que es
// precisamente lo que una prueba de permisos no debe confundir con un fallo.
func reqPrueba(s *Server, sess *SessionData, path string) *http.Request {
	r := httptest.NewRequest("GET", path, nil)
	return withScope(r, &scope{tenant: defaultTenantCode, db: s.db, sess: sess})
}

// ---------- Huella por archivo ----------

// TestHuellaPorArchivo: la huella de un recurso no depende de los demás.
//
// Era exactamente lo contrario con la huella global de assetVersion: tocar UN
// archivo de static/ cambiaba la huella de los 43 recursos del layout y todos
// se re-descargaban. Aquí, cambiar uno no cambia los otros.
func TestHuellaPorArchivo(t *testing.T) {
	limpiarCachesDeRender()

	css := assetVer("css/bootstrap.min.css")
	js := assetVer("js/jquery-3.4.1.min.js")
	if css == "" || js == "" {
		t.Fatalf("huella vacía: los recursos del layout no existen (css=%q js=%q)", css, js)
	}
	if css == js {
		t.Errorf("dos ficheros distintos con la misma huella: %q", css)
	}
	// Segunda llamada: sale de la caché, y sale IGUAL (si no, cada render
	// cambiaría el ?v= y la caché del navegador no serviría para nada).
	if assetVer("css/bootstrap.min.css") != css {
		t.Error("la huella no es estable entre llamadas: la caché por archivo no está sirviendo")
	}
	// Un fichero que no existe queda con huella vacía, que server.go trata como
	// no-cache (jamás como immutable).
	if assetVer("css/no-existe-en-absoluto.css") != "" {
		t.Error("un recurso inexistente debe devolver huella vacía")
	}
}

// TestLayoutPideHuellaPorArchivoYTodosLosRecursosExisten recorre el layout y
// comprueba dos cosas que a mano se olvidan: que ya no queda ni un
// `{{.AssetVer}}` (la huella global), y que TODOS los recursos a los que se les
// pone huella existen de verdad. Un recurso mal escrito saldría con `?v=` vacío
// y nadie se enteraría de que dejó de cachearse.
func TestLayoutPideHuellaPorArchivoYTodosLosRecursosExisten(t *testing.T) {
	layout, err := fs.ReadFile(erpgow.Templates(), "templates/layout.html")
	if err != nil {
		t.Fatalf("leer layout: %v", err)
	}
	contenido := string(layout)

	if strings.Contains(contenido, "{{.AssetVer}}") {
		t.Error("el layout sigue usando la huella global {{.AssetVer}}: con ella, cambiar un archivo obliga a re-descargar los 43")
	}

	re := regexp.MustCompile(`assetVer "([^"]+)"`)
	visibles := re.FindAllStringSubmatch(contenido, -1)
	if len(visibles) == 0 {
		t.Fatal("el layout no pide ninguna huella por archivo: el cambio no se aplicó")
	}
	for _, m := range visibles {
		ruta := "static/" + m[1]
		info, err := erpgow.Static().Open(ruta)
		if err != nil {
			t.Errorf("el layout pide huella de %s pero no se puede abrir: %v", ruta, err)
			continue
		}
		_ = info.Close()
		if assetVer(m[1]) == "" {
			t.Errorf("el layout pide huella de %s y sale vacía: ese recurso no se cacheará", ruta)
		}
	}
}

// ---------- Clave de fábrica cacheada por HASH ----------

// TestClaveDeFabricaSeRecalculaCuandoCambiaLaContrasena es LA prueba de
// seguridad de este bloque: la caché de ~65 ms de bcrypt está claveada por el
// hash, así que un cambio de contraseña la deja obsoleta POR CONSTRUCCIÓN.
// Se comprueba en los dos sentidos, porque los dos importan:
//
//   - de fábrica a normal  → el aviso debe APAGARSE en el acto (un aviso que
//     no se apaga se aprende a ignorar);
//   - de normal a fábrica  → el aviso debe ENCENDERSE en el acto (un aviso
//     apagado es un agujero).
func TestClaveDeFabricaSeRecalculaCuandoCambiaLaContrasena(t *testing.T) {
	limpiarCachesDeRender()

	fabrica, err := bcrypt.GenerateFromPassword([]byte("Admin@2026"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	otra, err := bcrypt.GenerateFromPassword([]byte("OtraClave1"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}

	if !fabricaDesdeHash(string(fabrica)) {
		t.Fatal("una clave de fábrica debe reconocerse como tal")
	}
	if fabricaDesdeHash(string(otra)) {
		t.Fatal("una clave normal no es de fábrica")
	}

	// Cacheado: la segunda consulta del MISMO hash no vuelve a calcular.
	antes := fabricaMisses.Load()
	fabricaDesdeHash(string(fabrica))
	fabricaDesdeHash(string(fabrica))
	if fabricaMisses.Load() != antes {
		t.Error("el mismo hash se está recalculando: la caché no se usa")
	}

	// 1) fábrica → normal: el aviso se apaga sin invalidar nada.
	if fabricaDesdeHash(string(otra)) {
		t.Error("cambió la contraseña y la caché devuelve el resultado viejo")
	}
	// 2) normal → fábrica: el aviso se enciende sin invalidar nada.
	if !fabricaDesdeHash(string(fabrica)) {
		t.Error("la contraseña pasó a ser de fábrica y el aviso no aparece")
	}
}

// TestUsaClaveDeFabricaLeeLaBase es la misma garantía pero por la vía que usa
// el render: consulta user_login y responde según el hash actual.
func TestUsaClaveDeFabricaLeeLaBase(t *testing.T) {
	limpiarCachesDeRender()
	_, db := basePrueba(t)

	fabrica, _ := bcrypt.GenerateFromPassword([]byte("Admin@2026"), bcrypt.DefaultCost)
	otra, _ := bcrypt.GenerateFromPassword([]byte("OtraClave1"), bcrypt.DefaultCost)

	if _, err := db.Exec(`INSERT INTO user_login (username, password) VALUES ('admin', ?)`, string(fabrica)); err != nil {
		t.Fatal(err)
	}
	s := &Server{config: &configs.Config{}, db: db}
	s.controlDB = db
	s.controlListo = true

	if !usaClaveDeFabrica(s.authDB(nil), "admin") {
		t.Fatal("debería avisar: la clave guardada es la de fábrica")
	}
	if _, err := db.Exec(`UPDATE user_login SET password = ? WHERE username = 'admin'`, string(otra)); err != nil {
		t.Fatal(err)
	}
	if usaClaveDeFabrica(s.authDB(nil), "admin") {
		t.Error("cambiaron la contraseña en la base y el aviso sigue encendido")
	}
}

// ---------- Permisos de la vista vs puerta de ejecución ----------

// TestPermisosDeVistaCacheadosPeroPuertaFresca es la separación de poderes:
// lo que se cachea es lo que se PINTA; hasPermission, que decide si la
// operación se ejecuta, consulta la base en CADA llamada.
func TestPermisosDeVistaCacheadosPeroPuertaFresca(t *testing.T) {
	limpiarCachesDeRender()
	s, db := basePrueba(t)

	if _, err := db.Exec(`INSERT INTO sub_module (id, directory, status) VALUES (5, 'purchase', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sec_role (id, type, level) VALUES (3, 'gerente', 'manager')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES ('7', 3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO role_permission (role_id, fk_module_id, "create", "read", "update", "delete") VALUES (3, 5, 1, 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}

	sess := sessPrueba("manager")
	r := reqPrueba(s, sess, "/purchase/add")

	// La vista SÍ ve el permiso.
	perm := s.permissionMap(r, sess)
	if !perm["purchase"] {
		t.Fatalf("la matriz del rol concedía purchase y la vista no lo muestra: %v", perm)
	}
	// La puerta de ejecución también, leyendo la base en el acto.
	if !s.hasPermission(r, "purchase", "create") {
		t.Error("hasPermission no ve un permiso concedido")
	}

	// Se REVOCA directamente en la base (sin pasar por la pantalla, que es lo
	// que la caché de 30 s cubriría).
	if _, err := db.Exec(`DELETE FROM role_permission WHERE role_id = 3 AND fk_module_id = 5`); err != nil {
		t.Fatal(err)
	}
	// La puerta se entierra INMEDIATAMENTE: no está cacheada.
	if s.hasPermission(r, "purchase", "create") {
		t.Error("SEGURIDAD: hasPermission sigue concediendo un permiso revocado en la base")
	}
	// La vista sigue con el valor cacheado… hasta que se invalide (o caduque).
	if !s.permissionMap(r, sess)["purchase"] {
		t.Error("la caché de vista no estaba activa: el cambio de 30 s no se puede probar")
	}
	invalidarCachePermisos()
	if s.permissionMap(r, sess)["purchase"] {
		t.Error("al invalidar, la vista debe recalcular con la base")
	}
}

// TestPermisosDeVistaCopiaIndependiente: el mapa que recibe el render es una
// copia. Si le entregara el de la caché, cualquier modificación de una vista
// desaparecería en la siguiente —o peor, se colaría en todas.
func TestPermisosDeVistaCopiaIndependiente(t *testing.T) {
	limpiarCachesDeRender()
	s, db := basePrueba(t)
	if _, err := db.Exec(`INSERT INTO sub_module (id, directory, status) VALUES (5, 'purchase', 1)`); err != nil {
		t.Fatal(err)
	}

	r := reqPrueba(s, sessPrueba("admin"), "/purchase/add")
	primera := s.permissionMap(r, sessPrueba("admin"))
	primera["purchase"] = false // alguien «apaga» un botón en una vista
	segunda := s.permissionMap(r, sessPrueba("admin"))
	if !segunda["purchase"] {
		t.Error("una vista cambió el mapa y se quedó grabado para las demás")
	}
}

// TestAdminSigueViendoTodo traslada el bypass de admin a la caché sin perderlo.
func TestAdminSigueViendoTodo(t *testing.T) {
	limpiarCachesDeRender()
	s, db := basePrueba(t)
	if _, err := db.Exec(`INSERT INTO sub_module (id, directory, status) VALUES (5, 'purchase', 1)`); err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, sessPrueba("admin"), "/purchase/add")
	perm := s.permissionMap(r, sessPrueba("admin"))
	if !perm["_all"] || !perm["purchase"] {
		t.Errorf("el administrador perdió su acceso total al pasar por la caché: %v", perm)
	}
	// Y la puerta de ejecución sigue resolviendo sola el nivel.
	if !s.hasPermission(r, "purchase", "delete") {
		t.Error("hasPermission dejó de conceder todo al admin")
	}
}

// ---------- Menú cacheado y podado por permisos ----------

// TestMenuCacheadoSigueFiltrandoPorPermiso es la garantía de que lo que se
// comparte entre peticiones son las FILAS y no el menú ya pintado: el filtro
// de permisos se aplica en cada petición, cache o no cache.
func TestMenuCacheadoSigueFiltrandoPorPermiso(t *testing.T) {
	limpiarCachesDeRender()
	s, db := basePrueba(t)

	insertar := `INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status)
		VALUES (?, NULL, ?, ?, '', ?, ?, ?, 1)`
	if _, err := db.Exec(insertar, 10, "Compras", "/purchase/list", "purchase", "/purchase", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insertar, 11, "Ajustes", "/settings", "", "/settings", 20); err != nil {
		t.Fatal(err)
	}

	r := reqPrueba(s, nil, "/purchase/list")

	// Con permiso de compras: ve las dos.
	conPerm := map[string]bool{"purchase": true}
	if n := len(s.menuTree(r, conPerm, "/purchase/list")); n != 2 {
		t.Fatalf("con permiso deberían verse 2 entradas, se ven %d", n)
	}
	// SIN permiso: la entrada de Compras no aparece aunque sus filas estén en
	// la caché compartida.
	sinPerm := map[string]bool{}
	arbol := s.menuTree(r, sinPerm, "/purchase/list")
	if len(arbol) != 1 {
		t.Fatalf("sin permiso sólo debe verse la entrada pública, se ven %d", len(arbol))
	}
	if arbol[0].Label != "Ajustes" {
		t.Errorf("la entrada que se ve es %q y debería ser «Ajustes»", arbol[0].Label)
	}
}

// TestMenuSeRecalculaTrasInvalidar comprueba que guardar en el Gestor de Menús
// (que llama a invalidarCacheMenu) se refleja en la siguiente vista.
func TestMenuSeRecalculaTrasInvalidar(t *testing.T) {
	limpiarCachesDeRender()
	s, db := basePrueba(t)

	if _, err := db.Exec(`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status)
		VALUES (20, NULL, 'Cuentas por Pagar', '/pagar/list', '', '', '/pagar', 30, 1)`); err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, nil, "/pagar/list")
	if n := len(s.menuTree(r, map[string]bool{}, "/pagar/list")); n != 1 {
		t.Fatalf("falta la entrada recién insertada: %d", n)
	}

	// Se añade otra fila: la caché todavía no se ha enterado…
	if _, err := db.Exec(`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status)
		VALUES (21, NULL, 'Cuentas por Cobrar', '/cartera/list', '', '', '/cartera', 40, 1)`); err != nil {
		t.Fatal(err)
	}
	if n := len(s.menuTree(r, map[string]bool{}, "/")); n != 1 {
		t.Fatalf("la caché de filas no estaba activa (se esperaba 1 hasta invalidar), hay %d", n)
	}

	// …y al invalidar —lo que hace el Gestor de Menús al guardar—, aparece.
	invalidarCacheMenu()
	if n := len(s.menuTree(r, map[string]bool{}, "/")); n != 2 {
		t.Errorf("tras invalidar deben verse 2 entradas, se ven %d", n)
	}
}

// TestPermisosCacheadosNoSeCruzanEntreEmpresas: la clave de la caché incluye
// la empresa. Sin eso, el permiso que alguien tiene en la empresa A se colaría
// en la vista de la empresa B —que es exactamente el fallo multi-empresa que ya
// tuvo este sistema por otro camino.
func TestPermisosCacheadosNoSeCruzanEntreEmpresas(t *testing.T) {
	limpiarCachesDeRender()
	s, _ := basePrueba(t)
	dbA := abrirBasePrueba(t)
	dbB := abrirBasePrueba(t)

	// Los permisos viven SÓLO en la base de la empresa A.
	if _, err := dbA.Exec(`INSERT INTO sub_module (id, directory, status) VALUES (5, 'purchase', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbA.Exec(`INSERT INTO sec_role (id, type, level) VALUES (3, 'gerente', 'manager')`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbA.Exec(`INSERT INTO sec_userrole (user_id, roleid) VALUES ('7', 3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbA.Exec(`INSERT INTO role_permission (role_id, fk_module_id, "create", "read", "update", "delete") VALUES (3, 5, 1, 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}

	sess := sessPrueba("manager")
	pet := func(tenant string, db *sql.DB) *http.Request {
		r := httptest.NewRequest("GET", "/purchase/add", nil)
		return withScope(r, &scope{tenant: tenant, db: db, sess: sess})
	}

	// Se consulta A primero, para que la caché quede llena con A antes de B.
	if !s.permissionMap(pet("empresa-a", dbA), sess)["purchase"] {
		t.Error("la empresa A sí concede purchase y su vista no lo ve")
	}
	if s.permissionMap(pet("empresa-b", dbB), sess)["purchase"] {
		t.Error("SEGURIDAD: la empresa B heredó los permisos cacheados de la empresa A")
	}
	// Y al revés: ver B primero no contamina a A.
	limpiarCachesDeRender()
	if s.permissionMap(pet("empresa-b", dbB), sess)["purchase"] {
		t.Error("la empresa B no concede nada y su vista dice que sí")
	}
	if !s.permissionMap(pet("empresa-a", dbA), sess)["purchase"] {
		t.Error("la empresa A perdió su permiso tras visitar la B")
	}
}

// BenchmarkFabricaCache mide lo que ahora cuesta el aviso de clave de fábrica
// en el camino caliente. Antes de la caché eran ~65 ms en CADA vista de un
// administrador; ahora es una búsqueda en un mapa.
func BenchmarkFabricaCache(b *testing.B) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Admin@2026"), bcrypt.DefaultCost)
	if err != nil {
		b.Fatal(err)
	}
	limpiarCacheFabrica()
	fabricaDesdeHash(string(hash)) // se llena fuera de la medición
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fabricaDesdeHash(string(hash))
	}
}

// TestMenuSinFilasNoSeMemorizaElVacio: una base a medio migrar debe volver a
// leerse en la siguiente petición en vez de «sin menú» durante 30 segundos.
func TestMenuSinFilasNoSeMemorizaElVacio(t *testing.T) {
	limpiarCachesDeRender()
	s, db := basePrueba(t)
	r := reqPrueba(s, nil, "/")

	if n := len(s.menuTree(r, map[string]bool{}, "/")); n != 0 {
		t.Fatalf("la base está vacía, se esperaban 0 entradas y hay %d", n)
	}
	// Se siembra DESPUÉS de esa primera lectura, SIN invalidar: si el vacío se
	// hubiera memorizado, la siguiente vista seguiría viendo 0 hasta caducar.
	if _, err := db.Exec(`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status)
		VALUES (30, NULL, 'Inicio', '/home', '', '', '', 5, 1)`); err != nil {
		t.Fatal(err)
	}
	if n := len(s.menuTree(r, map[string]bool{}, "/")); n != 1 {
		t.Errorf("con la base sembrada deben verse 1 entradas, se ven %d", n)
	}
}
