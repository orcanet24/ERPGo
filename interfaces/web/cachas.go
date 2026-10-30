package web

// cachas.go — Las cachés de LECTURA que pagan el render de cada página.
//
// CONTEXTO Y REGLA DE ORO. Al medir el coste de una vista normal se encontró
// que el servidor gastaba, en CADA petición de página:
//
//   - unos 65 ms de CPU en comparar con bcrypt la clave de fábrica del usuario
//     (render.go llamaba a usesFactoryPassword en todo render, no solo al
//     iniciar sesión), y
//   - de 4 a 6 consultas a la base que no se cacheaban: la matriz de permisos
//     del rol y las filas del menú, leídas una y otra vez con el mismo
//     resultado.
//
// Lo que se cachea aquí es SÓLO lo que el servidor calcula para PINTAR. No se
// toca nada que decida:
//
//   - hasPermission y canAccessModule —las puertas de ejecución— siguen
//     consultando la base en cada llamada, sin caché. Es a propósito: el que
//     decide si una operación se ejecuta no puede estar 30 segundos atrasado.
//   - Nada de esto se guarda en el navegador: son cachés de proceso, en
//     memoria del servidor. El cliente sigue recibiendo la respuesta fresca de
//     cada petición.
//
// Y son cachés con caducidad corta (ttlCacheRender) más invalidación explícita
// en las pantallas que escriben (Roles y Permisos, Gestor de Menús): si alguien
// toca los permisos desde la base a mano, el display se corrige solo en cuanto
// caduque, mientras que la puerta de ejecución nunca se entera porque no está
// cacheada.
//
// La única caché SIN invalidación explícita es la de clave de fábrica, y por
// una razón: su clave es el propio HASH de la contraseña. Si la contraseña
// cambia, cambia el hash, y la entrada vieja deja de existir. No hay escenario
// posible de aviso caducado.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"erpgo"
)

// ttlCacheRender es la vida de las cachés de pintado: corta a propósito.
// Un rol o un menú recién guardados se ven reflejados en cuanto caduque, y en
// las pantallas que los escriben la invalidación es inmediata.
const ttlCacheRender = 30 * time.Second

// ---------- 1. Huella por ARCHIVO de los estáticos ----------

// assetHuella es lo que se guarda de un fichero estático: su sello (tamaño y
// fecha) y la huella ya calculada. Se cachea el sello porque hacer dos stat
// por cada uno de los 43 recursos del layout en cada render es trabajo que no
// aporta nada.
type assetHuella struct {
	sello fileStamp
	valor string
}

type assetCache struct {
	mu         sync.RWMutex
	porArchivo map[string]assetHuella
}

var assetsCache = &assetCache{porArchivo: map[string]assetHuella{}}

// assetVer devuelve la huella de UN fichero estático, calculada con SU
// contenido.
//
// SUSTITUYE A assetVersion, que hasheaba TODO el árbol de static/ en una sola
// huella. Con esa huella global, cambiar un color de un módulo cambiaba la
// huella de los 43 recursos del layout, y todos los usuarios se re-descargaban
// los 43 en su siguiente visita aunque su módulo no se hubiera tocado. Aquí la
// huella sólo cambia cuando cambia EL FICHERO que la lleva, que es la
// invalidación exacta: lo que no cambió se sigue sirviendo desde la caché del
// navegador con `max-age=31536000, immutable` (server.go).
//
// El contenido se hashea UNA VEZ por sello (tamaño + fecha), nunca en cada
// render: dos stat por recurso y, si algo cambió, una lectura de ese fichero.
// El sello no basta solo porque las copias embebidas tienen fecha cero —sólo el
// tamaño las distinguiría, y dos versiones de un CSS pueden medir lo mismo—;
// el hash del contenido cubre ese caso y es estable entre reinicios, así que la
// caché del navegador jamás queda apuntando a la versión equivocada.
//
// En desarrollo, donde los ficheros se sirven del disco, el sello detecta la
// edición al instante —mismo criterio que stampOf usa para las plantillas—, así
// que guardar un CSS se ve en el navegador sin reiniciar el servidor.
//
// Una huella vacía significa «no encuentro ese fichero»: la plantilla pide
// `?v=` vacío, que es lo que server.go trata como «sin huella → no-cache»,
// nunca como «inmutable». Un recurso que no existe no se puede cachear un año.
func assetVer(ruta string) string {
	if ruta == "" {
		return ""
	}
	// El nombre llega SIN el prefijo "static/" porque así se escribe en la
	// plantilla; el overlay lo resuelve con el prefijo, que es como lo abre
	// server.go (fs.Sub(Static(), "static")).
	nombre := ruta
	if len(nombre) < 7 || nombre[:7] != "static/" {
		nombre = "static/" + nombre
	}
	sello := stampOf(erpgo.Static(), nombre)

	assetsCache.mu.RLock()
	h, ok := assetsCache.porArchivo[ruta]
	assetsCache.mu.RUnlock()
	if ok && h.sello == sello {
		return h.valor
	}

	valor := ""
	if b, err := fs.ReadFile(erpgo.Static(), nombre); err == nil {
		suma := sha256.Sum256(b)
		valor = hex.EncodeToString(suma[:])[:10]
	}
	assetsCache.mu.Lock()
	if len(assetsCache.porArchivo) > 4000 {
		assetsCache.porArchivo = map[string]assetHuella{}
	}
	assetsCache.porArchivo[ruta] = assetHuella{sello: sello, valor: valor}
	assetsCache.mu.Unlock()
	return valor
}

// ---------- 2. Clave de fábrica, cacheada POR HASH ----------

var (
	fabricaCache  sync.Map // hash de la contraseña -> ¿sigue siendo la de fábrica?
	fabricaHits   atomic.Int64
	fabricaMisses atomic.Int64
)

// fabricaDesdeHash decide si un hash es una clave de fábrica, cacheando el
// resultado bajo EL PROPIO HASH.
//
// Por qué la clave es el hash y no el usuario: así la caché no puede quedar
// desfasada. Si alguien cambia la contraseña, cambia el hash, la clave vieja
// ya no se consulta y se recalcula. Es la única forma de ganar los ~65 ms del
// bcrypt por página sin arriesgarse a que el aviso deje de aparecer cuando
// debe aparecer — que es justo lo que un comentario del propio render exige.
//
// Los contadores existen para poder probar que la caché se usa y para poder
// mirarlos desde un diagnóstico; no son estado de negocio.
func fabricaDesdeHash(hash string) bool {
	if hash == "" {
		return false
	}
	if v, ok := fabricaCache.Load(hash); ok {
		fabricaHits.Add(1)
		return v.(bool)
	}
	fabricaMisses.Add(1)
	v := calculaFabrica(hash)
	fabricaCache.Store(hash, v)
	return v
}

// limpiarCacheFabrica vacía la caché. La necesita la prueba que comprueba que
// recalcular después de vaciar da el resultado de la base, no el de antes.
func limpiarCacheFabrica() {
	fabricaCache.Range(func(k, _ any) bool {
		fabricaCache.Delete(k)
		return true
	})
}

// ---------- 3. Mapa de permisos de la VISTA (no de la ejecución) ----------

type permisosCache struct {
	mu    sync.Mutex
	datos map[string]entradaCache[map[string]bool]
}

var permisosVista = &permisosCache{datos: map[string]entradaCache[map[string]bool]{}}

type entradaCache[T any] struct {
	valor  T
	cuando time.Time
}

// permissionMap entrega el mapa de permisos para PINTAR el menú y los botones,
// cacheándolo por empresa + usuario durante ttlCacheRender.
//
// No es la puerta de nada: ver hasPermission y canAccessModule, que siguen
// consultando la base en cada uso. Aquí sólo se decide qué se MUESTRA, y se
// muestra lo mismo que decidiría la consulta directa — se cachean las filas
// devueltas por esa misma consulta, no otra cosa.
//
// Se devuelve SIEMPRE una copia: quien recibe el mapa es el render, que lo
// mete en la plantilla; si le diera la caché compartida, un cambio hecho por
// cualquiera de los otros renders desaparecería el siguiente.
//
// La invalidación es doble: la llaman Roles y Permisos (y la asignación de
// rol) en cuanto guardan, y además caduca sola a los 30 s para todo lo que
// nadie avise —una edición directa en la base, por ejemplo—.
func (s *Server) permissionMap(r *http.Request, sess *SessionData) map[string]bool {
	if sess == nil {
		return map[string]bool{}
	}
	clave := s.tenantOf(r) + "\x00" + sess.UserID

	permisosVista.mu.Lock()
	e, ok := permisosVista.datos[clave]
	vigente := ok && time.Since(e.cuando) < ttlCacheRender
	if ok && !vigente {
		delete(permisosVista.datos, clave)
	}
	permisosVista.mu.Unlock()

	if !vigente {
		e.valor = s.permisosDelRol(r, sess)
		e.cuando = time.Now()
		permisosVista.mu.Lock()
		if len(permisosVista.datos) > 5000 {
			permisosVista.datos = map[string]entradaCache[map[string]bool]{}
		}
		permisosVista.datos[clave] = e
		permisosVista.mu.Unlock()
	}

	copia := make(map[string]bool, len(e.valor))
	for k, v := range e.valor {
		copia[k] = v
	}
	return copia
}

// permisosDelRol es la consulta original de permisos, sin caché. Es la que se
// cachea; el nombre cambió para que se vea en el código cuál es la fuente.
func (s *Server) permisosDelRol(r *http.Request, sess *SessionData) map[string]bool {
	perm := map[string]bool{}
	if sess == nil {
		return perm
	}
	if sess.UserLevel == "admin" || sess.UserLevel == "master" {
		// authDB y no controlStore directo: es la misma base en una instalación
		// sana (la de control), y cuando el catálogo no abre cae a la de la
		// empresa, que es exactamente lo que ya hace loadMenu para el menú. Un
		// controlStore en error devolvía mapa vacío y el admin veía un menú sin
		// nada en lugar del acceso total que le corresponde.
		db := s.authDB(r)
		if db == nil {
			return perm
		}
		rows, err := db.Query(`SELECT directory FROM sub_module WHERE status=1`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var d string
				_ = rows.Scan(&d)
				perm[d] = true
			}
		}
		// `__master` no es un permiso de negocio: es la marca de las entradas de
		// menú que solo debe ver el super administrador (Módulos, por ejemplo).
		// El admin de empresa ve todo lo suyo, pero no la administración del
		// sistema; sin esta distinción, la entrada aparecía y al pulsarla daba
		// 403.
		if sess.UserLevel == "master" {
			perm["__master"] = true
		}
		perm["_all"] = true
		return perm
	}
	// `create`/`read`/`update`/`delete` son palabras reservadas de MySQL: sin
	// backticks la consulta revienta con Error 1064 y el mapa quedaba vacío en
	// silencio —el menú de todo usuario no-admin sin nada—.
	//
	// SE MIRAN LAS DOS BASES: los roles se editan en control, pero una
	// instalación puede tener la asignación en la base de la empresa. Basta con
	// que una base conceda.
	for _, db := range s.roleDBs(r) {
		if db == nil {
			continue
		}
		rows, err := db.Query("SELECT sm.directory, rp.`create`, rp.`read`, rp.`update`, rp.`delete`\n"+
			"FROM sec_userrole ur\n"+
			"JOIN role_permission rp ON rp.role_id = ur.roleid\n"+
			"JOIN sub_module sm ON sm.id = rp.fk_module_id\n"+
			"WHERE ur.user_id = ? AND sm.status = 1", sess.UserID)
		if err != nil {
			continue
		}
		for rows.Next() {
			var dir string
			var c, rd, u, d sql.NullInt64
			_ = rows.Scan(&dir, &c, &rd, &u, &d)
			if c.Int64 == 1 || rd.Int64 == 1 || u.Int64 == 1 || d.Int64 == 1 {
				perm[dir] = true
			}
		}
		rows.Close()
	}
	return perm
}

// invalidarCachePermisos vacía la caché de permisos de vista. La llaman las
// pantallas que escriben permisos: Roles y Permisos, la asignación de rol a un
// usuario y la siembra de permisos.
func invalidarCachePermisos() {
	permisosVista.mu.Lock()
	permisosVista.datos = map[string]entradaCache[map[string]bool]{}
	permisosVista.mu.Unlock()
}

// ---------- 4. Filas del menú ----------

type menuCache struct {
	mu    sync.Mutex
	datos map[string]entradaCache[[]menuFila]
}

var menuFilas = &menuCache{datos: map[string]entradaCache[[]menuFila]{}}

// cargarMenuFilas lee las filas de `menu` cacheándolas por empresa.
//
// Se cachean LAS FILAS y no el árbol ya podado: el filtro de permisos se sigue
// aplicando en CADA petición con el mapa de permisos de esa petición, así que
// un usuario no ve nunca una entrada que no le toca aunque las filas vengan de
// la caché. Ese es el punto: lo que se comparte entre peticiones es lo que no
// depende de quién pregunta.
//
// Como permissionMap, se invalida en cuanto el Gestor de Menús guarda, y además
// caduca a los 30 s.
func (s *Server) cargarMenuFilas(r *http.Request) ([]menuFila, error) {
	clave := s.tenantOf(r)

	menuFilas.mu.Lock()
	e, ok := menuFilas.datos[clave]
	vigente := ok && time.Since(e.cuando) < ttlCacheRender
	if ok && !vigente {
		delete(menuFilas.datos, clave)
	}
	menuFilas.mu.Unlock()
	if vigente {
		return e.valor, nil
	}

	filas, err := s.leerMenuFilas(r)
	if err != nil {
		// Un fallo de consulta no se memoriza: el siguiente render vuelve a
		// intentarlo y el menú vacío que se pinta hoy sigue pintándose hoy.
		return nil, err
	}
	// El estado del módulo se lee de la base de la EMPRESA, que es donde lo
	// escribe el interruptor. La estructura del menú viene de control.
	activos := s.modulosActivos(r)
	for i := range filas {
		// `core` (y vacío) no es un módulo apagable: es la estructura base.
		// Cualquier otro module_id solo se pinta si su fila existe y está en 1.
		filas[i].moduleOff = filas[i].moduleID != "" && filas[i].moduleID != "core" && !activos[filas[i].moduleID]
	}
	if len(filas) == 0 {
		// Tampoco se cachea el vacío: una base a medio migrar debe recogerse en
		// la siguiente petición en vez de memorizar «sin menú» 30 segundos.
		return filas, nil
	}
	e.valor = filas
	e.cuando = time.Now()
	menuFilas.mu.Lock()
	if len(menuFilas.datos) > 1000 {
		menuFilas.datos = map[string]entradaCache[[]menuFila]{}
	}
	menuFilas.datos[clave] = e
	menuFilas.mu.Unlock()
	return filas, nil
}

// invalidarCacheMenu vacía la caché de filas de menú. La llaman el Gestor de
// Menús (mover, reordenar, restaurar) y quien instala o retira módulos.
func invalidarCacheMenu() {
	menuFilas.mu.Lock()
	menuFilas.datos = map[string]entradaCache[[]menuFila]{}
	menuFilas.mu.Unlock()
}

// modulosActivos devuelve los módulos encendidos en la EMPRESA de la petición.
//
// Es una consulta aparte del menú y no un JOIN: `menu` vive en control y
// `modules` en la base de la empresa. Un JOIN entre bases distintas no existe, y
// leer el estado de control era el fallo que hacía que el menú no apareciera al
// activar el módulo.
//
// Si la tabla no existe (base antigua), devuelve un mapa vacío: los menús de
// módulos no se pintan, que es preferible a enseñar una entrada que no lleva a
// nada. El menú core no depende de esto.
func (s *Server) modulosActivos(r *http.Request) map[string]bool {
	out := map[string]bool{}
	db := s.dbFor(r)
	if db == nil {
		return out
	}
	rows, err := db.Query(`SELECT id FROM modules WHERE is_active = 1`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

// limpiarCachesDeRender vacía las tres cachés de pintado. Sólo la usan las
// pruebas: en producción la invalidación la dispara quien escribe.
func limpiarCachesDeRender() {
	invalidarCachePermisos()
	invalidarCacheMenu()
	limpiarCacheFabrica()
	assetsCache.mu.Lock()
	assetsCache.porArchivo = map[string]assetHuella{}
	assetsCache.mu.Unlock()
}
