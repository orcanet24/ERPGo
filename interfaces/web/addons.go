package web

// addons.go — La frontera de los addons externos (docs/PLAN-ADDONS.md, fases 1 y 2).
//
// POR QUÉ EXISTE. El SDK anterior (retirado en 2cfcc15) vivía en go.mod: «agregar
// un addon» era recompilar el binario. El módulo de GOWA se tejó en permisos,
// menús, migraciones y handlers a la vez: extraerlo fue cirugía. Este archivo
// implementa la frontera del plan: un addon es un programa separado, en la
// carpeta addons/ junto al binario, con un manifest.json que declara lo que
// quiere hacer; el núcleo lo lanza como proceso hijo, le entrega un token
// efímero por loopback, y navega sus rutas a través de un proxy inverso con la
// sesión del ERP. El núcleo ES este archivo y la administración de una vez;
// los addons ya no tocan el resto del código.
//
// Qué es fase 3. Fase 1 (frontera viva: escáner, manifiesto, lanzamiento,
// proxy, canal de negocio /__addon/ autenticado) y fase 2 (lectura granular
// por empresa activa con X-ERP-DB, escritura por el orquestador, tablas
// propias e identidad con CSRF para las pantallas del addon) ya respiran.
// La fase 3 añade confianza y reactividad: el manifiesto se verifica con
// ed25519 (clave pública del dueño en addons/ + manifest.sig;
// ADDONS_UNSIGNED=1 solo para desarrollo, y hasta una firma INCORRECTA se
// rechaza en cualquier modo), y los hooks: hooks_out con cola persistente en la
// BD de control y reintentos (patrón del runner de facturación digital),
// hooks_in con timeout y camino de retorno si el addon no responde. El detalle
// vive en addon_hooks.go; aquí está el candado del manifiesto y la firma.
//
// LO QUE NO SE NEGOCIA: un addon muerto jamás derriba al ERP (solo su ruta da
// 502 con mensaje claro), las cookies del ERP jamás llegan al addon, y las
// filas de modules/menu jamás se borran (solo se marcan inactivas).

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"erpgo/infrastructure/persistence"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// addonCoreVersion es lo que un manifiesto puede exigir como mínimo.
// Sube cuando la frontera gana capacidades nuevas (fase 1: la frontera viva;
// fase 2: lectura granular, escritura por el orquestador y tablas propias;
// fase 3: firma del manifiesto y hooks).
const addonCoreVersion = "0.3.0"

// addonFase marca la fase de la frontera que corre este binario, para que el
// panel diga la verdad («fase 3: firma, lectura, escritura por el canal, tablas
// propias y hooks»).
const addonFase = 3

// reservaDePrefijosCore son los prefijos de ruta que el núcleo ya usa. Un
// manifiesto que pida una de estas rutas no se instala: si le diéramos el
// prefijo al addon, el proxy taparía o sustituiría pantallas del ERP.
var reservaDePrefijosCore = []string{
	"/login", "/logout", "/health", "/api", "/static", "/uploads", "/favicon",
	"/__addon", "/webhooks", "/setup", "/module", "/data", "/home", "/assets",
	"/addon", "/guia", "/admin", "/config", "/mod", "/reports", "/accounting",
}

var (
	addonReID    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,38}$`)
	addonReRuta  = regexp.MustCompile(`^/[a-z0-9][a-z0-9_-]{0,60}(/[a-z0-9_-]{1,60})*$`)
	addonReTabla = regexp.MustCompile(`^[a-z][a-z0-9_]{0,60}$`)
	addonRePerm  = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,60}$`)

	// Los hooks se nombran igual que los verbs: dominio.accion para los
	// eventos que el núcleo emite (factura.emitida) y nombre con guiones bajos
	// para lo que el addon ofrece (send_whatsapp). Un nombre sin forma no va a
	// ningún lado y solo traería confusión al leer la historia de hooks.
	addonReHook = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

	// Los verbs de db_write son operaciones de negocio CON NOMBRE y en dos
	// partes (dominio.accion, como invoice.send): no es SQL libre, es la
	// puerta con nombre por la que el addon pide que el núcleo aplique una
	// escritura por el orquestador (docs/PLAN-ADDONS.md §7).
	addonReVerb = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$`)

	// El DDL que un addon puede pedirle al núcleo ejecutar: SOLO crear su
	// propia tabla si no existe. Ni DROP, ni ALTER, ni INSERT de siembra: el
	// schema.sql de un addon es una herramientas de creación, jamás un pincel
	// sobre el esquema ajeno.
	addonReDDL = regexp.MustCompile("(?i)^CREATE\\s+TABLE\\s+IF\\s+NOT\\s+EXISTS\\s+(?:[`\"]?)(?P<tabla>[a-z][a-z0-9_]*)(?:[`\"]?)\\s*\\(")
)

// addonDirPorDefecto es la carpeta donde viven los addons, relativa al arranque
// del proceso. Se puede sobrescribir con la variable de entorno ADDONS_DIR.
const addonDirPorDefecto = "addons"

// ---------- El manifiesto ----------

// addonMenuEntry es una entrada de menú del addon.
type addonMenuEntry struct {
	ParentID     *int     `json:"parent_id"`
	Label        string   `json:"label"`
	URL          string   `json:"url"`
	Icon         string   `json:"icon"`
	Perms        []string `json:"perms"`
	SortOrder    int      `json:"sort_order"`
	ActivePrefix string   `json:"active_prefix"`
}

// addonPermiso es un permiso declarado por el addon. El código SIEMPRE lleva
// el prefijo del id (`hola_ver`, no `ver`): es el candado del manifiesto.
type addonPermiso struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// addonVerbo es una operación de escritura declarada en db_write: un verbo
// con nombre (dominio.accion, como invoice.send) y el permiso del manifiesto
// que la sesión llamante debe tener. No es SQL libre: el verbo solo existe si
// el núcleo tiene conectado su handler de negocio, y la frontera valida que el
// usuario tenga una sesión viva, que su empresa coincida y que su permiso lo
// autorice antes de llamarlo (docs/PLAN-ADDONS.md §7).
type addonVerbo struct {
	Verb string `json:"verb"`
	Perm string `json:"perm"`
}

// eventosHooksCore es el catálogo de eventos que el NÚCLEO emite por hooks_out.
// Un addon solo puede SUSCRIBIRSE a uno de estos: si declarara un evento que el
// núcleo nunca dispara, el panel vendería una suscripción que nunca suena. El
// catálogo crece junto con los puntos de negocio (hoy: la factura que se
// registra como definitiva en invoice.go).
var eventosHooksCore = []string{"factura.emitida", "compra.registrada", "retencion.emitida"}

// addonManifest es el permiso de trabajo del addon: lo que declara, existe.
type addonManifest struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Version        string           `json:"version"`
	MinCoreVersion string           `json:"min_core_version"`
	Description    string           `json:"description"`
	Author         string           `json:"author"`
	Routes         []string         `json:"routes"`
	Menu           []addonMenuEntry `json:"menu"`
	DBRead         []string         `json:"db_read"`
	DBWrite        []addonVerbo     `json:"db_write"`
	TablesOwn      []string         `json:"tables_own"`
	Perms          []addonPermiso   `json:"perms"`
	HooksOut       []string         `json:"hooks_out"`
	HooksIn        []string         `json:"hooks_in"`
}

// validarManifiesto es EL candado: lo que no pasa aquí, no corre.
//
// Cada regla responde a un daño concreto: id raro rompe el prefijado y la
// trazabilidad; rutas reservadas pisarían pantallas del ERP; permisos sin el
// prefijo del id chocarían con los del resto del sistema; un menú que cita
// permisos no declarados sería una entrada mentirosa (visible para quien luego
// no puede entrar).
func validarManifiesto(m *addonManifest) error {
	if m == nil {
		return fmt.Errorf("el manifiesto está vacío")
	}
	if !addonReID.MatchString(m.ID) {
		return fmt.Errorf("el id %q es inválido: solo minúsculas, dígitos, guion y guion bajo (2 a 39 caracteres, empieza por letra o dígito)", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("falta el nombre del addon")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("falta la versión del addon")
	}
	if !addonVersionCompatible(m.MinCoreVersion, addonCoreVersion) {
		return fmt.Errorf("el addon exige núcleo %s o mayor y este ERP es %s: actualice el ERP o el addon", m.MinCoreVersion, addonCoreVersion)
	}
	if len(m.Routes) == 0 {
		return fmt.Errorf("el addon debe declarar al menos una ruta para su pantalla")
	}
	declaradas := map[string]bool{}
	for _, r := range m.Routes {
		if !addonReRuta.MatchString(r) {
			return fmt.Errorf("la ruta %q es inválida: una ruta es /minuscula-con-guiones, sin // ni puntos", r)
		}
		if declaradas[r] {
			return fmt.Errorf("la ruta %q está declarada dos veces", r)
		}
		declaradas[r] = true
		for _, reservada := range reservaDePrefijosCore {
			if r == reservada || strings.HasPrefix(r, reservada+"/") {
				return fmt.Errorf("la ruta %q es del núcleo (prefijo %q): elija otra", r, reservada)
			}
		}
	}
	// Permisos: código con prefijo del id y etiqueta no vacía.
	codigos := map[string]bool{}
	for _, p := range m.Perms {
		if !strings.HasPrefix(p.Code, m.ID+"_") || !addonRePerm.MatchString(p.Code) {
			return fmt.Errorf("el permiso %q debe empezar con el id del addon (ej. %q)", p.Code, m.ID+"_ver")
		}
		if strings.TrimSpace(p.Label) == "" {
			return fmt.Errorf("el permiso %q no tiene etiqueta visible", p.Code)
		}
		if codigos[p.Code] {
			return fmt.Errorf("el permiso %q está declarado dos veces", p.Code)
		}
		codigos[p.Code] = true
	}

	// Lectura granular (fase 2): db_read son tablas DEL ERP que el addon quiere
	// consultar, así que NO llevan el prefijo del id — el ejemplo del plan es
	// customer_information, una tabla del núcleo. El candado de verdad no está
	// en el manifiesto: está en MySQL, al que el núcleo SO le concede SELECT
	// sobre estas tablas en la base de cada empresa. Aquí solo exigimos que el
	// nombre sea una tabla bien formada.
	for _, t := range m.DBRead {
		if !addonReTabla.MatchString(t) {
			return fmt.Errorf("la tabla %q de db_read es inválida", t)
		}
	}

	// Escritura por el canal (fase 2): cada verbo de db_write es una puerta de
	// negocio CON NOMBRE (dominio.accion) y exige un permiso DECLARADO en este
	// mismo manifiesto: si el addon pide un permiso que él no define, el panel
	// mostraría un candado que nadie puede conceder. Un intento de SQL libre
	// (nombre sin punto) no es una operación de negocio: se rechaza.
	verbos := map[string]bool{}
	for _, v := range m.DBWrite {
		if !addonReVerb.MatchString(v.Verb) {
			return fmt.Errorf("el verbo %q de db_write es inválido: deben ser dominio.accion (ej. invoice.send)", v.Verb)
		}
		if verbos[v.Verb] {
			return fmt.Errorf("el verbo %q de db_write está declarado dos veces", v.Verb)
		}
		verbos[v.Verb] = true
		if v.Perm == "" || !codigos[v.Perm] {
			return fmt.Errorf("el verbo %q exige el permiso %q en su campo perm, y el manifiesto no lo declara en perms", v.Verb, v.Perm)
		}
	}

	// Tablas propias (fase 2): prefijo del id, para que ningún addon pueda pisar
	// una tabla del núcleo ni de otro addon. El escáner exige además que el
	// schema.sql las cree (y SOLO a ellas) con CREATE TABLE IF NOT EXISTS.
	// db_read quedó fuera de este candado a propósito: son tablas del ERP, no suyas.
	for _, t := range m.TablesOwn {
		if !addonReTabla.MatchString(t) {
			return fmt.Errorf("la tabla %q de tables_own es inválida", t)
		}
		if t == m.ID || !strings.HasPrefix(t, m.ID+"_") {
			return fmt.Errorf("la tabla %q debe empezar por el prefijo del id (%s_…): las tablas del ERP no son suyas", t, m.ID)
		}
	}

	// Hooks (fase 3): hooks_out solo suscribe eventos que el núcleo emite de
	// verdad (el catálogo eventosHooksCore); hooks_in declara lo que el addon
	// OFRECE, con nombre bien formado. En ambos casos, un hook declarado dos
	// veces es una declaración mentirosa: se recibe o se pide doble.
	vistosOut := map[string]bool{}
	for _, h := range m.HooksOut {
		if !addonReHook.MatchString(h) {
			return fmt.Errorf("el hook de salida %q es inválido: usa dominio.accion (ej. factura.emitida)", h)
		}
		if vistosOut[h] {
			return fmt.Errorf("el hook de salida %q está declarado dos veces", h)
		}
		conocido := false
		for _, ev := range eventosHooksCore {
			if ev == h {
				conocido = true
				break
			}
		}
		if !conocido {
			return fmt.Errorf("el hook de salida %q no es un evento que este núcleo emita (emite: %v)", h, eventosHooksCore)
		}
		vistosOut[h] = true
	}
	vistosIn := map[string]bool{}
	for _, h := range m.HooksIn {
		if !addonReHook.MatchString(h) {
			return fmt.Errorf("el hook de entrada %q es inválido: usa un nombre con guiones bajos (ej. send_whatsapp)", h)
		}
		if vistosIn[h] {
			return fmt.Errorf("el hook de entrada %q está declarado dos veces", h)
		}
		vistosIn[h] = true
	}

	// Menú: cada URL cae bajo una ruta declarada y cada permiso citado está
	// declarado. Un menú que lleva a lo que el permiso no contempla es la
	// entrada trampa que se prohibió.
	for i, e := range m.Menu {
		if strings.TrimSpace(e.Label) == "" {
			return fmt.Errorf("la entrada de menú %d no tiene etiqueta", i+1)
		}
		if !addonURLBajoRutas(e.URL, m.Routes) {
			return fmt.Errorf("la entrada de menú %q con URL %q no cae bajo ninguna ruta declarada (%v)", e.Label, e.URL, m.Routes)
		}
		for _, pc := range e.Perms {
			if !codigos[pc] {
				return fmt.Errorf("la entrada de menú %q usa el permiso %q que el manifiesto no declara", e.Label, pc)
			}
		}
	}
	return nil
}

// addonURLBajoRutas dice si una URL de menú cae bajo una de las rutas.
func addonURLBajoRutas(u string, rutas []string) bool {
	if u == "" || !strings.HasPrefix(u, "/") {
		return false
	}
	for _, r := range rutas {
		if u == r || strings.HasPrefix(u, r+"/") {
			return true
		}
	}
	return false
}

// addonVersionCompatible compara «1.2.3» (los segmentos que faltan valen 0).
// Devuelve true si exigida <= actual. Una versión mal escrita RECHAZA, no
// admite: una exigencia ilegible no se interpreta como «sea lo que sea».
func addonVersionCompatible(exigida, actual string) bool {
	e, okE := addonVersionNumeros(exigida)
	a, okA := addonVersionNumeros(actual)
	// Una exigencia ilegible RECHAZA: no se interpreta como «sea lo que sea».
	if !okE || !okA {
		return false
	}
	for i := 0; i < 3; i++ {
		if e[i] > a[i] {
			return false
		}
		if e[i] < a[i] {
			return true
		}
	}
	return true
}

// addonVersionNumeros parsea «1.2.3» (los segmentos que faltan valen 0).
// El segundo resultado dice si la cadena era legible de verdad.
func addonVersionNumeros(v string) ([3]int, bool) {
	var n [3]int
	if strings.TrimSpace(v) == "" {
		return n, true
	}
	segs := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3)
	parteNum := func(seg string) (int, bool) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return 0, false
		}
		por := strings.IndexAny(seg, "+-")
		if por >= 0 {
			seg = seg[:por]
		}
		for _, c := range seg {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
		var num int
		fmt.Sscanf(seg, "%d", &num)
		return num, true
	}
	for i := 0; i < len(segs) && i < 3; i++ {
		num, ok := parteNum(segs[i])
		if !ok {
			return n, false
		}
		n[i] = num
	}
	return n, true
}

// ---------- El host ----------

// Estados visibles en el panel de addons.
const (
	estadoActivo   = "activo"
	estadoCaydo    = "caído"
	estadoSinBin   = "sin binario"
	estadoDetenido = "detenido"
)

// addonProc es un addon cargado: su manifiesto, su proceso y su canal.
type addonProc struct {
	manifiesto addonManifest
	dir        string
	binario    string // "" si la carpeta no tiene binario compilado
	token      string
	puerto     int
	cmd        *exec.Cmd

	estado         string
	fallasSalud    int
	ultimoError    string
	proximoIntento time.Time

	// Fase 2: sentencias CREATE TABLE IF NOT EXISTS que el escáner validó
	// frente al manifiesto (solo tablas propias, nada de DROP/ALTER). Se
	// aplican sobre cada empresa registrada; una empresa nueva las hereda.
	migraciones []string

	// Fase 2: resultado de pedir al núcleo la lectura granular. Fallo de esa
	// solicitud NO tumba al addon — se explica en el panel con aviso_lectura.
	lecturaOK    bool
	lecturaAviso string

	// Fase 3: estado de la firma del manifiesto (verificada, sin verificar,
	// sin firma). El panel lo muestra tal cual: el dueño decide con qué
	// confianza corre su ERP.
	firma string

	proxy *httputil.ReverseProxy
}

// addonHost es la frontera entera: escáner, registro, lanzador, salud, proxy
// y canal. Vive dentro del Server; todos los métodos del Server son nil-safe.
type addonHost struct {
	s *Server

	mu          sync.Mutex
	directorio  string
	progs       map[string]*addonProc
	rutasDelMux map[string]bool // rutas ya registradas en el mux (solo al arrancar)
	avisos      []string        // avisos del último escaneo, para el panel

	// rutasVivas es el conjunto de rutas de addon resolubles AHORA, publicado
	// como snapshot inmutable. El despachador pregunta por cada petición y no
	// puede tomar h.mu: ese candado se retiene durante el lanzamiento y la
	// espera de salud (hasta 10 s), y bloquear al ERP entero detrás de un
	// addon lento sería peor que el problema que resuelve.
	rutasVivas atomic.Pointer[map[string]bool]

	// handlers de negocio de los verbs de db_write, por addon y por verbo.
	// Los conecta el núcleo en el arranque (addonVerboRegistrar); la frontera
	// NO conoce su lógica: solo valida contexto y permiso, y despacha.
	acciones map[string]map[string]verboDeAddon

	// crea el usuario MySQL de solo lectura de un addon con db_read.
	// Inyectable para pruebas; por defecto es s.crearUsuarioLectorAddon.
	crearLectura func(m addonManifest) (string, error)

	// Fase 3 (confianza): la clave pública ed25519 del dueño (addons/
	// pubkey.ed25519) y el modo de desarrollo ADDONS_UNSIGNED. Con clave
	// configurada, un addon sin manifest.sig no carga salvo modo desarrollo;
	// una firma PRESENTE e inválida no carga en ningún modo: significa
	// manifiesto TOCADO después de firmar, y el modo dev es para iterar sin
	// infraestructura de firma, no para aceptar daño activo.
	clavePub ed25519.PublicKey
	unsigned bool

	// entrega REAL de un hook_out, inyectable para pruebas (a un servidor
	// catapulta que registra la llamada); por defecto es entregarReal.
	entregar func(p *addonProc, evento string, payload []byte, empresa, token string, puerto int) error
	// inyectables para pruebas
	lanzar func(p *addonProc, binario string, env []string) error
	ahora  func() time.Time
	parado chan struct{}
	parada sync.Once
}

// verboDeAddon es la firma de un handler de negocio conectado por el núcleo:
// recibe el addon, el contexto de sesión YA VALIDADO por la frontera y el
// cuerpo JSON de la petición, y devuelve el resultado que llegará al addon.
// Errores de negocio devuelven error y el canal lo traduce a 500.
type verboDeAddon func(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error)

// sesionDeAddon es el contexto que la frontera pasa al handler de negocio:
// el usuario Y LA SESIÓN DE LA PETICIÓN (la frontera la resolvió por
// X-ERP-User: el addon nunca decide quién llama) y la empresa activa resuelta.
type sesionDeAddon struct {
	Usuario string
	Empresa string
	Sess    *SessionData
}

// addonsIniciar arranca la frontera: escanea, registra en el mux y lanza.
// Es nil-safe y también auto-suficiente: si la carpeta addons/ no existe se
// CREA aquí (0755). Razón: una instalación sin addons es legítima, pero que
// la carpeta falte no debe obligar al dueño a conocer la ruta esperada ni a
// crearla a mano: la carpeta vacía es el estado natural de «nunca instalé un
// addon». Solo si la creación misma falla (permisos, disco) se degrada:
// este arranque va sin addons y se informa.
func (s *Server) addonsIniciar() {
	dir := os.Getenv("ADDONS_DIR")
	if dir == "" {
		dir = addonDirPorDefecto
	}
	if _, err := os.Stat(dir); err != nil {
		if errC := os.MkdirAll(dir, 0o755); errC != nil {
			log.Printf("AVISO addons: la carpeta %q no existe y no se pudo crear: %v; este arranque va sin addons externos", dir, errC)
			return
		}
		log.Printf("addons: carpeta %q no existía y fue creada (vacía): los addons externos se despliegan ahí", dir)
	}
	h := &addonHost{
		s:            s,
		directorio:   dir,
		progs:        map[string]*addonProc{},
		rutasDelMux:  map[string]bool{},
		acciones:     map[string]map[string]verboDeAddon{},
		crearLectura: s.crearUsuarioLectorAddon,
		ahora:        time.Now,
		parado:       make(chan struct{}),
	}
	h.entregar = h.entregarReal
	h.lanzar = h.lanzarReal
	s.addons = h

	// Fase 3: la clave pública con la que el dueño firma, si la puso en addons/
	// pubkey.ed25519 (hex ed25519). Sin clave, todo corre «sin verificar» y el
	// panel lo dice; con clave, la firma es obligatoria.
	if pub, ok := addonClavePublica(dir); ok {
		h.clavePub = pub
	} else {
		log.Printf("AVISO addons: sin addons/pubkey.ed25519: los manifiestos NO se verifican en este arranque")
	}
	h.unsigned = os.Getenv("ADDONS_UNSIGNED") == "1"
	if h.unsigned {
		log.Printf("AVISO addons: modo ADDONS_UNSIGNED con verificación en firme; los manifiestos SIN firma cargan con marca «sin verificar»")
	}

	if err := h.reescanear(); err != nil {
		log.Printf("AVISO addons: no se pudo iniciar el host de addons: %v (el ERP sigue funcionando sin addons)", err)
		return
	}
	h.crearTablaHooks()
	go h.vigilarHooks()
	go h.vigilarSalud()
}

// escanear lee la carpeta addons/ y devuelve los manifiestos válidos junto con
// los avisos por carpeta, para que el panel diga QUÉ está mal en CADA addon,
// no solo que «algo» falló.
func (h *addonHost) escanear() (map[string]*addonProc, []string) {
	// Ya se validan colisiones de rutas entre addons aquí: es más barato
	// rechazar en la lectura que descubrirlo en un handler caído.
	fila := map[string]*addonProc{}
	var avisos []string
	entradas, err := os.ReadDir(h.directorio)
	if err != nil {
		return nil, []string{"no se pudo leer la carpeta addons/: " + err.Error()}
	}
	rutasOcupadas := map[string]string{} // ruta → id que la usó primero
	for _, e := range entradas {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(h.directorio, e.Name())
		rutaManifiesto := filepath.Join(dir, "manifest.json")
		contenido, err := os.ReadFile(rutaManifiesto)
		if err != nil {
			continue // carpeta sin manifiesto: ni se mira (no es un addon)
		}
		var m addonManifest
		dec := json.NewDecoder(bytes.NewReader(contenido))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&m); err != nil {
			avisos = append(avisos, fmt.Sprintf("%s: manifiesto inválido: %v", e.Name(), err))
			continue
		}
		if err := validarManifiesto(&m); err != nil {
			avisos = append(avisos, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		if _, yaHay := fila[m.ID]; yaHay {
			avisos = append(avisos, fmt.Sprintf("%s: el id %q ya lo usa otro addon cargado de esta carpeta", e.Name(), m.ID))
			continue
		}
		var colision bool
		for _, r := range m.Routes {
			if dueño := rutasOcupadas[r]; dueño != "" {
				avisos = append(avisos, fmt.Sprintf("%s: la ruta %q ya la sirve el addon %q", e.Name(), r, dueño))
				colision = true
				break
			}
			rutasOcupadas[r] = m.ID
		}
		if colision {
			continue
		}
		// Fase 2: si declara tablas propias, el schema.sql es obligatorio y
		// su DDL va candado: solo CREATE TABLE IF NOT EXISTS de ESAS tablas.
		// Un addon que trae DROP o ALTER, o crea una tabla que no declaró,
		// no carga: el aviso lo explica con el detalle.
		var migraciones []string
		if len(m.TablesOwn) > 0 {
			sentencias, err := addonMigracionesDeCarpeta(dir, &m)
			if err != nil {
				avisos = append(avisos, fmt.Sprintf("%s: %v", e.Name(), err))
				continue
			}
			migraciones = sentencias
		}
		// Fase 3: firma del manifiesto, ANTES de que la carpeta toque el ERP.
		estadoFirma, err := h.verificarFirmaAddon(dir, contenido)
		if err != nil {
			avisos = append(avisos, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		fila[m.ID] = &addonProc{manifiesto: m, dir: dir, migraciones: migraciones, firma: estadoFirma}
	}
	return fila, avisos
}

// addonClavePublica lee la clave pública ed25519 del dueño (hex) en
// <directorio>/pubkey.ed25519. La PRIVADA jamás vive aquí: el dueño firma con
// tools/firmaraddon y solo deposita la pública junto a los addons.
func addonClavePublica(directorio string) (ed25519.PublicKey, bool) {
	crudo, err := os.ReadFile(filepath.Join(directorio, "pubkey.ed25519"))
	if err != nil {
		return nil, false
	}
	claves, err := hex.DecodeString(strings.TrimSpace(string(crudo)))
	if err != nil || len(claves) != ed25519.PublicKeySize {
		return nil, false
	}
	return ed25519.PublicKey(claves), true
}

// verificarFirmaAddon decide el estado de la firma de UN addon y, de paso,
// rechaza lo que no entra por política:
//
//   - Sin clave pública configurada: no hay verificación posible, todo corre
//     «sin verificar» (la pantalla del panel lo dice con claridad).
//   - Con clave: la firma es obligatoria. Un addon sin manifest.sig carga SOLO
//     en modo de desarrollo (ADDONS_UNSIGNED=1), marcado «sin verificar». Una
//     firma presente e inválida NO carga en ningún modo: significa que el
//     manifiesto cambió después de firmar, y eso no es iteración, es daño.

func (h *addonHost) verificarFirmaAddon(dir string, contenido []byte) (string, error) {
	if h.clavePub == nil {
		return "sin verificar", nil
	}
	sig, err := os.ReadFile(filepath.Join(dir, "manifest.sig"))
	if err != nil {
		if os.IsNotExist(err) {
			if h.unsigned {
				return "sin verificar", nil
			}
			return "", fmt.Errorf("la clave pública del dueño está configurada y el addon NO trae manifest.sig: fírmelo con tools/firmaraddon, o pongo ADDONS_UNSIGNED=1 para desarrollo")
		}
		return "", fmt.Errorf("leyendo manifest.sig: %w", err)
	}
	firma, err := hex.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(firma) != ed25519.SignatureSize {
		return "", fmt.Errorf("manifest.sig no es una firma ed25519 en hex (%d bytes decodificados de %d esperados)", len(firma), ed25519.SignatureSize)
	}
	if !ed25519.Verify(h.clavePub, contenido, firma) {
		return "", fmt.Errorf("la firma de manifest.sig NO corresponde al manifest.json actual: el manifiesto cambió después de firmar y no se carga")
	}
	return "verificada", nil
}

// reescanear aplica el estado de la carpeta addons/ al ERP: manifiestos →
// filas de modules/menu en TODAS las empresas → procesos → rutas del proxy.
// Es el botón «Re-escanear addons» y el arranque. El mux solo gana una ruta
// estable, /mod/<id>/ (ver registrarRutasAddon): los addons nuevos que
// lleguen al vuelo se sirven por ahí sin reiniciar el ERP.
func (h *addonHost) reescanear() error {
	if h == nil {
		return nil
	}
	fila, avisos := h.escanear()
	idsNuevos := idsOrdenados(fila)

	h.mu.Lock()
	// Los addons que desaparecieron de la carpeta: dejar de servir y marcarlos
	// inactivos en todas las empresas (las filas jamás se borran: es la
	// trazabilidad del repo). Sus rutas siguen registradas en el mux, pero el
	// proxy ya no las resuelve: devuelve «addon no instalado».
	var retirados []string
	for id, p := range h.progs {
		if _, queda := fila[id]; !queda {
			if p.cmd != nil && p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			delete(h.progs, id)
			retirados = append(retirados, id)
		}
	}
	h.mu.Unlock()
	// Fuera del candado: marcar inactivo recorre el catálogo de empresas y no
	// puede bloquear al vigil de salud mientras habla con MySQL.
	for _, id := range retirados {
		h.marcaInactivoEnBD(id)
	}

	for _, id := range idsNuevos {
		nuevo := fila[id]
		h.mu.Lock()
		// Se conserva el proceso vivo entre rescaneos: mismo binario y mismo
		// manifiesto = mismo proceso. Solo se reinicia si el binario cambió
		// (se recompiló), si el manifiesto cambió (versión, rutas, menú,
		// permisos) o si el anterior no estaba en pie.
		cambioManifiesto := false
		if previo, estaba := h.progs[id]; estaba {
			cambioManifiesto = !reflect.DeepEqual(previo.manifiesto, nuevo.manifiesto)
			nuevo.cmd = previo.cmd
			nuevo.token = previo.token
			nuevo.puerto = previo.puerto
			nuevo.binario = previo.binario
			nuevo.estado = previo.estado
			nuevo.fallasSalud = previo.fallasSalud
			nuevo.ultimoError = previo.ultimoError
			nuevo.lecturaOK = previo.lecturaOK
			nuevo.lecturaAviso = previo.lecturaAviso
			nuevo.proximoIntento = previo.proximoIntento
			nuevo.proxy = previo.proxy
			if !cambioManifiesto {
				nuevo.manifiesto = previo.manifiesto // el vivo manda
			}
			nuevo.dir = previo.dir
		}
		h.progs[id] = nuevo
		cambioBinario := nuevo.binario != nuevo.buscarBinario()
		reiniciar := nuevo.estado == "" || nuevo.estado == estadoCaydo ||
			nuevo.estado == estadoSinBin || cambioBinario || cambioManifiesto
		h.mu.Unlock()

		if err := h.registrarEnBD(nuevo.manifiesto, nuevo.migraciones); err != nil {
			avisos = append(avisos, fmt.Sprintf("%s: no se pudo registrar en modules/menu: %v", id, err))
		}
		// El registro de arriba solo alcanza a la empresa activa; las demás se
		// quedarían sin fila de módulo (y su menú desaparecería) si no se
		// replican.
		h.registrarEnOtrasEmpresas(nuevo.manifiesto, nuevo.migraciones)
		if reiniciar {
			// lanzarOReanudar respeta el interruptor: un addon apagado en todas
			// las empresas no arranca, y si estaba vivo se detiene.
			h.lanzarOReanudar(nuevo)
		} else if !h.addonActivoEnBD(id) {
			h.detener(nuevo)
		}
	}
	if len(avisos) > 0 || len(idsNuevos) > 0 {
		invalidarCacheMenu()
	}
	h.mu.Lock()
	h.avisos = avisos
	h.mu.Unlock()
	h.publicarRutas()
	for _, aviso := range avisos {
		log.Printf("addons: %s", aviso)
	}
	return nil
}

// registrarRutasAddon es la ÚNICA vez que se registran rutas de addons en el
// mux: en el arranque. Va acompañada del escaneo inicial.
//
// La puerta /mod/<id>/ se registra UNA sola vez y despacha por id en caliente:
// no depende de conocer la carpeta al arrancar. Las rutas declaradas de los
// addons presentes se registran además como alias heredado (sus enlaces
// internos y los deep-links viejos siguen funcionando); un addon que llegue
// después de un re-escaneo se sirve por /mod/<id>/ sin reiniciar el ERP.
func (s *Server) registrarRutasAddon() {
	if s == nil || s.addons == nil {
		return
	}
	// El prefijo estable. Se registran las dos formas para que /mod (sin id)
	// también llegue al handler y responda con su 404 claro en vez de caer en
	// el fail-closed de rutas sin clasificar.
	s.mux.HandleFunc("/mod", func(w http.ResponseWriter, req *http.Request) { s.addonRouter(w, req) })
	s.mux.HandleFunc("/mod/", func(w http.ResponseWriter, req *http.Request) { s.addonRouter(w, req) })

	s.addons.mu.Lock()
	vistas := map[string]bool{}
	for _, p := range s.addons.progs {
		for _, r := range p.manifiesto.Routes {
			vistas[r] = true
		}
	}
	s.addons.mu.Unlock()
	for r := range vistas {
		ruta := r
		s.mux.HandleFunc(ruta, func(w http.ResponseWriter, req *http.Request) { s.addonRouter(w, req) })
		s.mux.HandleFunc(ruta+"/", func(w http.ResponseWriter, req *http.Request) { s.addonRouter(w, req) })
	}
	s.addons.mu.Lock()
	s.addons.rutasDelMux = vistas
	s.addons.mu.Unlock()
	s.addons.publicarRutas()

	// El canal de negocio SIEMPRE existe, aunque no haya addons: es el
	// receptor del núcleo, y responder «no hay addon con ese id» sin exponer
	// tokens es parte del contrato.
	s.mux.HandleFunc("/__addon/{id}/{verb}", s.addonCanal)
}

// publicarRutas deja un snapshot inmutable de las rutas de addon que el
// despachador puede resolver: las registradas en el mux al arrancar (para que
// un addon retirado siga respondiendo 404, no 403) más las declaradas por los
// manifiestos vivos (para el addon instalado en caliente). Se llama al
// escanear y al registrar el mux, nunca por petición.
func (h *addonHost) publicarRutas() {
	if h == nil {
		return
	}
	h.mu.Lock()
	vivas := make(map[string]bool, len(h.rutasDelMux))
	for r := range h.rutasDelMux {
		vivas[r] = true
	}
	for _, p := range h.progs {
		if p == nil {
			continue
		}
		for _, r := range p.manifiesto.Routes {
			vivas[r] = true
		}
	}
	h.mu.Unlock()
	h.rutasVivas.Store(&vivas)
}

// esRutaAddon dice si un path pertenece a una página declarada por el
// manifiesto de algún addon. El mapa de clasificación de permisos no puede
// conocerlas —se registran en tiempo de ejecución desde los manifiestos—, y
// esta es la pregunta que hace el fail-closed antes de negar una pantalla de
// un addon instalado hoy. La frontera real sigue siendo addonRouter: sesión
// viva, empresa y el permiso que el manifiesto declaró. nil-safe: sin carpeta
// de addons no hay rutas de addon (igual que el resto de métodos del host).
func (s *Server) esRutaAddon(path string) bool {
	if s == nil || s.addons == nil {
		return false
	}
	// La puerta estable siempre es de addons: el handler decide si el id
	// existe (404) o si el módulo está apagado para esta empresa.
	if path == "/mod" || strings.HasPrefix(path, "/mod/") {
		return true
	}
	// El snapshot incluye las rutas del mux (para que un addon retirado siga
	// dando 404, no 403) y las declaradas por los manifiestos vivos (para el
	// addon instalado en caliente). Se consulta SIN candado: esta función
	// corre en CADA petición del ERP y h.mu se retiene durante el lanzamiento
	// y la espera de salud de un addon (hasta 10 s).
	if vivas := s.addons.rutasVivas.Load(); vivas != nil {
		return algunaRutaDeAddon(path, *vivas)
	}
	// Sin snapshot (host construido a mano antes del primer escaneo) el
	// candado es la única fuente. Es el arranque, no el camino caliente.
	s.addons.mu.Lock()
	defer s.addons.mu.Unlock()
	if algunaRutaDeAddon(path, s.addons.rutasDelMux) {
		return true
	}
	for _, p := range s.addons.progs {
		for _, r := range p.manifiesto.Routes {
			if rutaDeAddonCoincide(path, r) {
				return true
			}
		}
	}
	return false
}

// algunaRutaDeAddon dice si path es una de las rutas del conjunto o cuelga de
// una de ellas.
func algunaRutaDeAddon(path string, rutas map[string]bool) bool {
	for r := range rutas {
		if rutaDeAddonCoincide(path, r) {
			return true
		}
	}
	return false
}

// rutaDeAddonCoincide compara por segmento: /mod/hola no debe aceptar
// /mod/holanda.
func rutaDeAddonCoincide(path, ruta string) bool {
	return path == ruta || strings.HasPrefix(path, ruta+"/")
}

// addonDispatchMiddleware resuelve las rutas de addons contra los manifiestos
// VIVOS antes de que el mux decida. El mux solo gana /mod/ una vez; un addon
// instalado en caliente se sirve por /mod/<id> y, además, por sus rutas
// declaradas, que es a donde apuntan los formularios y enlaces internos del
// propio addon. Sin esta capa, la pantalla cargaría en /mod/<id> pero su
// primer POST a /<ruta> caería en el mux sin patrón.
func (s *Server) addonDispatchMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.esRutaAddon(r.URL.Path) {
			s.addonRouter(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- Registro en la DB: modules y menu ----------

// dbDeAddon devuelve la base donde se registran los módulos: la de la empresa
// activa en el arranque; para el resto se llama addonsRegistrarEmpresa desde
// el preparado de empresa nueva.
func (h *addonHost) dbDeAddon() *sql.DB {
	if h.s == nil {
		return nil
	}
	return h.s.activeDB()
}

// porCadaEmpresa ejecuta fn sobre la base activa y sobre cada empresa del
// catálogo, una sola vez por handle. Es la vía para que el registro de un
// addon no se quede en la empresa que estaba activa al arrancar: las demás
// también necesitan su fila de modules/menu. Un catálogo ilegible (pruebas,
// instalación de una sola empresa) no impide la operación local.
func (h *addonHost) porCadaEmpresa(fn func(db *sql.DB, code string)) {
	if h == nil || h.s == nil || fn == nil {
		return
	}
	vistos := map[*sql.DB]bool{}
	activo := h.s.activeTenantCode()
	if db := h.s.activeDB(); db != nil {
		vistos[db] = true
		fn(db, activo)
	}
	ctl, err := h.s.controlStore()
	if err != nil {
		return
	}
	rows, err := ctl.Query(`SELECT code FROM tenants`)
	if err != nil {
		return
	}
	var codigos []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil && code != "" {
			codigos = append(codigos, code)
		}
	}
	rows.Close()
	for _, code := range codigos {
		if code == activo {
			continue
		}
		db := h.s.dbForTenant(code)
		if db == nil || vistos[db] {
			continue
		}
		vistos[db] = true
		fn(db, code)
	}
}

// addonActivoEnBD dice si el addon está encendido en ALGUNA empresa. El
// proceso es único para todo el ERP: apagarlo porque una empresa lo apagó
// dejaría sin pantalla a las demás. La visibilidad por empresa la decide
// addonEstaApagado en cada petición; esto solo decide si vale la pena tener
// el proceso vivo. Sin ninguna fila legible se responde «sí»: una base de
// prueba o una instalación recién sembrada no debe impedir el lanzamiento.
func (h *addonHost) addonActivoEnBD(id string) bool {
	if h == nil || h.s == nil {
		return true
	}
	consultado := false
	activo := false
	h.porCadaEmpresa(func(db *sql.DB, code string) {
		var a int
		if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = ?`, id).Scan(&a); err != nil {
			return
		}
		consultado = true
		if a == 1 {
			activo = true
		}
	})
	if !consultado {
		return true
	}
	return activo
}

// registrarEnBD sincroniza las filas del addon con modules y menu de la base
// de empresa: PRIMERO sus migraciones (la tabla propia debe existir aunque el
// registro de módulo falle), luego modules y menu. Nada se borra: lo que ya
// no está declarado se marca inactivo.
func (h *addonHost) registrarEnBD(m addonManifest, migraciones []string) error {
	db := h.dbDeAddon()
	if db == nil {
		return nil // sin base (arranque en pruebas): nada que grabar
	}
	return h.registrarEnDB(db, m, migraciones)
}

// addonMenuURL traduce una URL declarada por el addon a la puerta estable
// /mod/<id>. El manifiesto sigue hablando en sus rutas (/hola, /hola/config),
// pero el menú apunta al prefijo que el núcleo registra de una vez: así un
// addon instalado después del arranque también aparece y funciona sin
// reiniciar el ERP.
func addonMenuURL(m addonManifest, u string) string {
	if u == "" || len(m.Routes) == 0 {
		return u
	}
	for _, r := range m.Routes {
		if u == r {
			return "/mod/" + m.ID
		}
		if strings.HasPrefix(u, r+"/") {
			return "/mod/" + m.ID + strings.TrimPrefix(u, r)
		}
	}
	return u
}

// sincronizarMenu refleja el manifiesto en el menú: las entradas declaradas se
// insertan (si no existen) o encienden/actualizan (si existen); las que ya no
// están declaradas, status = 0. Las URLs pasan por addonMenuURL para que la
// entrada apunte a /mod/<id> y no a una ruta que solo existía en el arranque.
func (h *addonHost) sincronizarMenu(db *sql.DB, m addonManifest) error {
	if db == nil {
		return nil
	}
	deseadas := map[string]bool{}
	for _, e := range m.Menu {
		deseadas[addonMenuURL(m, e.URL)] = true
	}

	// Normalizar filas antiguas: las sembradas con la ruta declarada (/hola)
	// pasan a la puerta estable (/mod/hola) conservando su fila y su estado.
	// Se hace ANTES de leer para que el barrido de retiradas no las confunda
	// con entradas que ya no existen. Si la fila nueva ya existe, la vieja se
	// retira abajo (no está en deseadas).
	for _, e := range m.Menu {
		nueva := addonMenuURL(m, e.URL)
		if nueva == e.URL {
			continue
		}
		var existeNueva int
		_ = db.QueryRow(`SELECT COUNT(*) FROM menu WHERE module_id = ? AND url = ?`, m.ID, nueva).Scan(&existeNueva)
		if existeNueva > 0 {
			continue
		}
		if _, err := db.Exec(`UPDATE menu SET url = ? WHERE module_id = ? AND url = ?`, nueva, m.ID, e.URL); err != nil {
			return fmt.Errorf("normalizar la entrada de menú %q: %w", e.URL, err)
		}
	}

	// Filas viejas de ESTE addon que ya no están en el manifiesto → inactivas.
	rows, err := db.Query(`SELECT IFNULL(url, '') FROM menu WHERE module_id = ?`, m.ID)
	if err != nil {
		return fmt.Errorf("leer menú del addon: %w", err)
	}
	var retiradas []string
	for rows.Next() {
		var u string
		_ = rows.Scan(&u)
		if u != "" && !deseadas[u] {
			retiradas = append(retiradas, u)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("leer menú del addon: %w", err)
	}
	for _, u := range retiradas {
		if _, err := db.Exec(`UPDATE menu SET status = 0 WHERE module_id = ? AND url = ?`, m.ID, u); err != nil {
			return fmt.Errorf("retirar la entrada de menú %q: %w", u, err)
		}
	}

	for _, e := range m.Menu {
		nueva := addonMenuURL(m, e.URL)
		prefijo := addonMenuURL(m, e.ActivePrefix)
		if prefijo == "" {
			// Sin active_prefix, el subrayado del menú sigue al prefijo estable:
			// /mod/hola/config ilumina /mod/hola sin depender de la ruta interna.
			prefijo = "/mod/" + m.ID
		}
		var existe int
		_ = db.QueryRow(`SELECT COUNT(*) FROM menu WHERE module_id = ? AND url = ?`, m.ID, nueva).Scan(&existe)
		if existe == 0 {
			if _, err := db.Exec(`INSERT INTO menu (parent_id, label, url, module_id, is_core, icon, active_prefix, sort_order, status, perm)
				VALUES (?, ?, ?, ?, 0, ?, ?, ?, 1, ?)`,
				e.ParentID, e.Label, nueva, m.ID, e.Icon, prefijo, e.SortOrder, strings.Join(e.Perms, "|")); err != nil {
				return fmt.Errorf("insertar el menú %q: %w", e.Label, err)
			}
		} else {
			if _, err := db.Exec(`UPDATE menu SET label = ?, icon = ?, active_prefix = ?, sort_order = ?, status = 1, perm = ?
				WHERE module_id = ? AND url = ?`, e.Label, e.Icon, prefijo, e.SortOrder, strings.Join(e.Perms, "|"), m.ID, nueva); err != nil {
				return fmt.Errorf("actualizar el menú %q: %w", e.Label, err)
			}
		}
	}
	return nil
}

// marcaInactivoEnBD marca el addon como «no instalado» en todas las empresas
// SIN borrar nada: modules (is_active = 0) y menú (status = 0) quedan como
// evidencia.
func (h *addonHost) marcaInactivoEnBD(id string) {
	h.porCadaEmpresa(func(db *sql.DB, code string) {
		_, _ = db.Exec(`UPDATE modules SET is_active = 0 WHERE id = ? AND is_core = 0`, id)
		_, _ = db.Exec(`UPDATE menu SET status = 0 WHERE module_id = ?`, id)
	})
}

// addonRegistrarEmpresa es lo que se llama cuando se prepara una empresa
// nueva: los addons ya escaneados deben existir también en esa base.
// addonRegistrarEmpresa lleva el escaneo y el registro de los addons ya cargados
// a una empresa NUEVA: filas de modules/menu, migraciones de tablas propias y,
// si el addon tiene lectura granular en pie, los grants SELECT en la base
// nueva. Se llama al preparar empresa (PASO 4½ del provisioning). El código de
// empresa nuevo va para los grants; el resto es igual al arranque.
func (s *Server) addonRegistrarEmpresa(db *sql.DB, codigo string) {
	if s == nil || s.addons == nil || db == nil {
		return
	}
	s.addons.mu.Lock()
	var lista []*addonProc
	for _, p := range s.addons.progs {
		lista = append(lista, p)
	}
	s.addons.mu.Unlock()
	for _, p := range lista {
		if err := s.addons.registrarEnDB(db, p.manifiesto, p.migraciones); err != nil {
			log.Printf("addons: no se pudo registrar %s en la empresa nueva: %v", p.manifiesto.ID, err)
		}
		// Lectura granular: el usuario lector ya existe (fue creado al lanzar
		// el addon); solo se le agregan los grants de la base nueva. Un fallo
		// aquí NO revienta la preparación de la empresa: queda en el log y el
		// usuario lector simplemente no verá estas tablas de esta empresa.
		if len(p.manifiesto.DBRead) > 0 {
			if err := s.addonConcederLecturaEnBase(p, s.tenantDBName(codigo)); err != nil {
				log.Printf("AVISO addons: %s: sin grants de lectura para la empresa %q: %v", p.manifiesto.ID, codigo, err)
			}
		}
	}
}

// registrarEnDB es la variante con DB explícita de registrarEnBD (se usa al
// preparar una empresa, cuando no hay petición activa). Las migraciones de
// tablas propias van PRIMERAS: la tabla debe existir aunque el registro de
// módulo/popups/menu falle por la razón que sea.
func (h *addonHost) registrarEnDB(db *sql.DB, m addonManifest, migraciones []string) error {
	if db == nil {
		return nil
	}
	for _, sentencia := range migraciones {
		if _, err := db.Exec(sentencia); err != nil {
			return fmt.Errorf("migración del addon: %w (sentencia: %.80s)", err, sentencia)
		}
	}
	var existe int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modules WHERE id = ?`, m.ID).Scan(&existe); err == nil && existe == 0 {
		if _, err := db.Exec(`INSERT INTO modules (id, name, version, description, is_core, is_active, path, author, license_type)
			VALUES (?, ?, ?, ?, 0, 1, ?, ?, 'MIT')`,
			m.ID, m.Name, m.Version, truncado(m.Description, 255), h.directorio, truncado(m.Author, 100)); err != nil {
			return fmt.Errorf("insertar módulo: %w", err)
		}
	}
	return h.sincronizarMenu(db, m)
}

// registrarEnOtrasEmpresas replica el registro (migraciones, modules y menu)
// en cada empresa del catálogo que no sea la activa. El arranque solo ve la
// empresa activa; sin esto, las demás no tendrían la fila del módulo y su
// menú desaparecería, y el dueño lo reportaría como un addon roto. Los grants
// de lectura se agregan con la misma clave de la sesión (el usuario lector ya
// existe); un fallo de grants no tumba el registro: queda en el log.
func (h *addonHost) registrarEnOtrasEmpresas(m addonManifest, migraciones []string) {
	if h == nil || h.s == nil {
		return
	}
	activo := h.s.activeTenantCode()
	ctl, err := h.s.controlStore()
	if err != nil {
		return
	}
	rows, err := ctl.Query(`SELECT code FROM tenants`)
	if err != nil {
		return
	}
	var codigos []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil && code != "" {
			codigos = append(codigos, code)
		}
	}
	rows.Close()
	for _, code := range codigos {
		if code == activo {
			continue
		}
		db := h.s.dbForTenant(code)
		if db == nil {
			continue
		}
		if err := h.registrarEnDB(db, m, migraciones); err != nil {
			log.Printf("AVISO addons: %s: no se pudo registrar en la empresa %q: %v", m.ID, code, err)
			continue
		}
		if len(m.DBRead) > 0 && h.s.config != nil && h.s.config.DBHost != "" {
			if err := h.s.addonConcederLecturaEnBase(&addonProc{manifiesto: m}, h.s.tenantDBName(code)); err != nil {
				log.Printf("AVISO addons: %s: sin grants de lectura para la empresa %q: %v", m.ID, code, err)
			}
		}
	}
}

// ---------- Migraciones propias y lectura granular (fase 2) ----------

// addonMigracionesDeCarpeta lee y valida el schema.sql de un addon con
// tables_own. El contrato es de piedra:
//
//   - El archivo existe: declarar tables_own sin schema.sql no carga (el
//     addon se quedaría sin tabla y su pantalla fallaría en silencio).
//   - Cada sentencia es CREATE TABLE IF NOT EXISTS <tabla>, con <tabla>
//     declarada por el manifiesto. Ni DROP, ni ALTER, ni INSERT de siembra: el
//     esquema propio se SIEMBRA una vez con código del addon, no con DDL suelto,
//     y el esquema ajeno no lo toca nadie.
//   - Cada tabla declarada tiene su CREATE (si faltara, la tabla no
//     existiría y el addon fallaría lejos del error real).
func addonMigracionesDeCarpeta(dir string, m *addonManifest) ([]string, error) {
	contenido, err := os.ReadFile(filepath.Join(dir, "schema.sql"))
	if err != nil {
		return nil, fmt.Errorf("declara tables_own pero la carpeta no trae schema.sql: %w", err)
	}
	sentencias := addonPartirSQL(string(contenido))
	if len(sentencias) == 0 {
		return nil, fmt.Errorf("declara tables_own pero schema.sql está vacío o sin sentencias")
	}
	declaradas := map[string]bool{}
	for _, t := range m.TablesOwn {
		declaradas[t] = true
	}
	creadas := map[string]bool{}
	for _, sentencia := range sentencias {
		coincidencia := addonReDDL.FindStringSubmatch(sentencia)
		if coincidencia == nil {
			return nil, fmt.Errorf("schema.sql: solo se aceptan sentencias CREATE TABLE IF NOT EXISTS <tabla>; la sentencia %.60s… no pasa el candado", sentencia)
		}
		tabla := coincidencia[1]
		if !declaradas[tabla] {
			return nil, fmt.Errorf("schema.sql: crea la tabla %q y el manifiesto NO la declara en tables_own", tabla)
		}
		if creadas[tabla] {
			return nil, fmt.Errorf("schema.sql: la tabla %q se crea dos veces", tabla)
		}
		creadas[tabla] = true
	}
	for _, t := range m.TablesOwn {
		if !creadas[t] {
			return nil, fmt.Errorf("la tabla %q está en tables_own pero schema.sql no la crea", t)
		}
	}
	return sentencias, nil
}

// addonPartirSQL parte un .sql en sentencias “;” separadas, respetando
// comillas simples, dobles, backticks y comentarios -- de línea completa.
// No es un parser SQL: es la sierra justa para dividir un schema.sql saneado.
func addonPartirSQL(sql string) []string {
	var salida []string
	var actual strings.Builder
	enComilla := byte(0)
	comentario := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if comentario {
			if c == '\n' {
				comentario = false
				actual.WriteByte(c)
			}
			continue
		}
		if enComilla != 0 {
			if c == '\\' && enComilla != '`' {
				// Escapado dentro de comilla: copia literal los dos bytes.
				if i+1 < len(sql) {
					actual.WriteByte(c)
					actual.WriteByte(sql[i+1])
					i++
				}
				continue
			}
			if c == enComilla {
				enComilla = 0
			}
			actual.WriteByte(c)
			continue
		}
		switch {
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			// Comentario de línea: se salta hasta el fin de línea (puede
			// contener ';' o citas que confundirían la sierra).
			comentario = true
			i++
		case c == '\'' || c == '"' || c == '`':
			enComilla = c
			actual.WriteByte(c)
		case c == ';':
			tira := strings.TrimSpace(actual.String())
			if tira != "" {
				salida = append(salida, tira)
			}
			actual.Reset()
		default:
			actual.WriteByte(c)
		}
	}
	if tira := strings.TrimSpace(actual.String()); tira != "" {
		salida = append(salida, tira)
	}
	return salida
}

// crearUsuarioLectorAddon crea (o refresca) el usuario MySQL de solo lectura
// de un addon con db_read, le concede SELECT SOLO sobre las tablas declaradas
// en las bases de las empresas registradas y devuelve el DSN SIN base por
// defecto: el addon consulta la base de la empresa activa anteponiendo el
// prefijo de empresa que el provisioning fija como contrato (docs/PLAN-ADDONS
// §2). La clave es NUEVA en cada lanzamiento: cuando el addon renace, la
// sesión anterior pierde el acceso a la lectura aunque conserve la memoria.
func (s *Server) crearUsuarioLectorAddon(m addonManifest) (string, error) {
	if s == nil || s.config == nil {
		return "", fmt.Errorf("sin configuración MySQL en este arranque")
	}
	if len(m.DBRead) == 0 {
		return "", fmt.Errorf("el manifiesto no declara db_read")
	}
	db, usuario, err := s.conexionDelUsuarioLector(m.ID)
	if err != nil {
		return "", err
	}
	defer db.Close()
	clave := tokenEfimero()
	host := "127.0.0.1"
	if _, err := db.Exec(fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'", usuario, host, clave)); err != nil {
		return "", fmt.Errorf("no se pudo crear el usuario de lectura: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'", usuario, host, clave)); err != nil {
		return "", fmt.Errorf("no se pudo refrescar la clave del usuario de lectura: %w", err)
	}
	// Las bases donde el addon consulta: la del núcleo/empresa maestra y cada
	// una del catálogo. Una empresa nueva recibe sus grants en
	// addonRegistrarEmpresa (mismo usuario, misma clave, sin re-crear nada).
	bases := []string{s.config.DBName}
	if ctl, err := s.controlStore(); err == nil {
		rows, err := ctl.Query(`SELECT code FROM tenants`)
		if err == nil {
			for rows.Next() {
				var code string
				if err := rows.Scan(&code); err == nil && code != "" {
					bases = append(bases, s.tenantDBName(code))
				}
			}
			rows.Close()
		}
	}
	for _, base := range bases {
		if base == "" {
			continue
		}
		for _, tabla := range m.DBRead {
			if !addonReTabla.MatchString(tabla) {
				return "", fmt.Errorf("la tabla %q de db_read no es un nombre válido para GRANT", tabla)
			}
			if _, err := db.Exec(fmt.Sprintf("GRANT SELECT ON `%s`.`%s` TO '%s'@'%s'", base, tabla, usuario, host)); err != nil {
				return "", fmt.Errorf("no se pudo conceder SELECT en %s.%s: %w", base, tabla, err)
			}
		}
	}
	if _, err := db.Exec("FLUSH PRIVILEGES"); err != nil {
		return "", fmt.Errorf("no se pudo refrescar privilegios: %w", err)
	}
	return persistence.DSNMySQL(persistence.ConfigMySQL{Host: s.config.DBHost, Port: s.config.DBPort, Usuario: usuario, Clave: clave}), nil
}

// conexionDelUsuarioLector abre la conexión de administración MySQL y devuelve
// el nombre del usuario lector del addon (erp_ro_ + su id truncado, guiones
// por guion bajo para no pisar entre ids parecidos). Las credenciales son las
// mismas de provisioning de empresas: quien prepara empresas es quien crea
// lectores. El lector nace SIN usuario si no hay uno del provisioning: mismo
// rebusque que el preparado de empresas.
func (s *Server) conexionDelUsuarioLector(idAddon string) (*sql.DB, string, error) {
	cfgMySQL := persistence.ConfigMySQL{
		Host: s.config.DBHost, Port: s.config.DBPort,
		Usuario: s.config.DBProvisionUser, Clave: s.config.DBProvisionPassword,
	}
	if cfgMySQL.Usuario == "" {
		cfgMySQL.Usuario = s.config.DBUser
		cfgMySQL.Clave = s.config.DBPassword
	}
	db, err := persistence.ConectarServidorMySQL(cfgMySQL)
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo conectar al servidor MySQL para el usuario de lectura: %w", err)
	}
	return db, "erp_ro_" + truncado(strings.ReplaceAll(idAddon, "-", "_"), 25), nil
}

// addonConcederLecturaEnBase agrega los grants SELECT del usuario lector de
// un addon para una base NUEVA (empresa recién preparada): mismo usuario,
// misma clave de esta sesión; no se re-crea nada ni se toca la clave aquí.
func (s *Server) addonConcederLecturaEnBase(p *addonProc, base string) error {
	if s == nil || p == nil || base == "" || len(p.manifiesto.DBRead) == 0 {
		return nil
	}
	db, usuario, err := s.conexionDelUsuarioLector(p.manifiesto.ID)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, tabla := range p.manifiesto.DBRead {
		if !addonReTabla.MatchString(tabla) {
			return fmt.Errorf("la tabla %q no es un nombre válido para GRANT", tabla)
		}
		if _, err := db.Exec(fmt.Sprintf("GRANT SELECT ON `%s`.`%s` TO '%s'@'127.0.0.1'", base, tabla, usuario)); err != nil {
			return fmt.Errorf("no se pudo conceder SELECT en %s.%s: %w", base, tabla, err)
		}
	}
	if _, err := db.Exec("FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("no se pudo refrescar privilegios: %w", err)
	}
	return nil
}

// ---------- Lanzamiento del proceso hijo ----------

// buscarBinario localiza el binario del addon en su carpeta: erp-addon-<id>
// con o sin .exe según el sistema. Sin binario compilado, el addon se ve en el
// panel como «sin binario» (verdadero, no mentira); en ese estado no hay proxy.
func (p *addonProc) buscarBinario() string {
	nombre := "erp-addon-" + p.manifiesto.ID
	var candidatos []string
	if runtime.GOOS == "windows" {
		candidatos = []string{nombre + ".exe", nombre}
	} else {
		candidatos = []string{nombre, nombre + ".exe"}
	}
	for _, c := range candidatos {
		completo := filepath.Join(p.dir, c)
		st, err := os.Stat(completo)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		// El bit de ejecución (0111) solo existe en POSIX: Windows no lo
		// reporta — os.Stat devuelve 0666 para todo archivo regular — y con esa
		// prueba NINGÚN binario pasaba en Windows: siempre «sin binario». Allá
		// un archivo regular con el nombre del binario es tan lanzable como su
		// par .exe; la prueba de ejecutabilidad se queda donde significa algo.
		if runtime.GOOS != "windows" && st.Mode()&0111 == 0 {
			continue
		}
		return completo
	}
	return ""
}

// reservarPuerto pide al sistema operativo un puerto loopback libre. La
// seguridad esencial está en el 127.0.0.1: el addon solo escucha en loopback
// y solo el núcleo conoce el token.
func reservarPuerto() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// lanzarOReanudar arranca (o releva) el addon como proceso propio, con token
// efímero y puerto reservado. Si el módulo está apagado en TODAS las empresas,
// no se lanza y el proceso vivo (si lo había) se detiene. Un fallo queda
// explicado en el panel y programa el siguiente intento del vigil.
func (h *addonHost) lanzarOReanudar(p *addonProc) {
	if h == nil || p == nil {
		return
	}
	if !h.addonActivoEnBD(p.manifiesto.ID) {
		h.detener(p)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lanzarBajoCandado(p)
}

// detener apaga el proceso del addon y lo deja en estado «detenido»: el
// interruptor del módulo lo apagó. No se reintenta mientras siga apagado.
func (h *addonHost) detener(p *addonProc) {
	if h == nil || p == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.cmd = nil
	p.estado = estadoDetenido
	p.fallasSalud = 0
	p.proximoIntento = time.Time{}
	p.ultimoError = ""
	p.proxy = nil
}

// sincronizarProceso aplica el interruptor del módulo al proceso hijo: si el
// addon quedó encendido en alguna empresa, se asegura de que esté lanzado; si
// quedó apagado en todas, se detiene. El proceso es único para el ERP, así que
// el interruptor de una empresa no apaga a las demás.
func (h *addonHost) sincronizarProceso(id string) {
	if h == nil || id == "" {
		return
	}
	h.mu.Lock()
	p := h.progs[id]
	estado := ""
	if p != nil {
		estado = p.estado
	}
	h.mu.Unlock()
	if p == nil {
		return
	}
	if !h.addonActivoEnBD(id) {
		h.detener(p)
		return
	}
	switch estado {
	case estadoDetenido, estadoCaydo, estadoSinBin, "":
		h.lanzarOReanudar(p)
	}
}

// ahoraDe es el reloj del host con respaldo: las pruebas inyectan h.ahora,
// pero un host mínimo no debe entrar en pánico por no hacerlo.
func (h *addonHost) ahoraDe() time.Time {
	if h != nil && h.ahora != nil {
		return h.ahora()
	}
	return time.Now()
}

// programarReintento fija el próximo intento con retroceso exponencial
// (5 s, 10 s, 20 s… tope 5 min). Se usa cuando un lanzamiento falla para que
// el vigil vuelva a probar; un fallo de arranque no se convierte en un
// martilleo cada 30 s.
func (h *addonHost) programarReintento(p *addonProc) {
	if p == nil {
		return
	}
	p.fallasSalud++
	if p.fallasSalud < 2 {
		p.fallasSalud = 2
	}
	retardo := time.Duration(5<<minDe(p.fallasSalud-2, 9)) * time.Second
	if retardo > 5*time.Minute {
		retardo = 5 * time.Minute
	}
	p.proximoIntento = h.ahoraDe().Add(retardo)
}

func (h *addonHost) lanzarBajoCandado(p *addonProc) {
	// Un relanzamiento es un relevo: el proceso anterior muere antes de que el
	// nuevo reserve puerto. Sin esto, recompilar un binario dejaba dos procesos
	// escuchando y el proxy podía seguir hablando con el viejo.
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		p.cmd = nil
	}
	p.proximoIntento = time.Time{}

	binario := p.buscarBinario()
	p.binario = binario
	if binario == "" {
		p.estado = estadoSinBin
		p.ultimoError = fmt.Sprintf("no hay binario compilado en %q (se busca erp-addon-%s con o sin .exe)", p.dir, p.manifiesto.ID)
		log.Printf("addons: %s: %s", p.manifiesto.ID, p.ultimoError)
		return
	}

	puerto, err := reservarPuerto()
	if err != nil {
		p.estado = estadoCaydo
		p.ultimoError = "no se pudo reservar un puerto loopback: " + err.Error()
		h.programarReintento(p)
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.ultimoError)
		return
	}
	token := tokenEfimero()

	env := append(os.Environ(),
		"ERP_ADDON_TOKEN="+token,
		"ERP_ADDON_PORT="+fmt.Sprint(puerto),
	)
	if h.s != nil && h.s.config != nil {
		env = append(env, "ERP_CORE_URL=http://127.0.0.1:"+h.s.config.ServerPort)
	}

	// Lectura granular (fase 2): el lector corre ANTES de lanzar el proceso y
	// su DSN viaja en el entorno que el proceso va a recibir.
	env = append(env, h.resolverLectura(p)...)

	iniciar := h.lanzar
	if iniciar == nil {
		iniciar = h.lanzarReal
	}
	if err := iniciar(p, binario, env); err != nil {
		p.estado = estadoCaydo
		p.ultimoError = "no se pudo lanzar el proceso: " + err.Error()
		h.programarReintento(p)
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.ultimoError)
		return
	}

	// El addon tiene un plazo fijo para estar sano; si no, queda caído y el
	// vigil lo reintenta con retroceso (ver cicloSalud). Nada de esto congela
	// el arranque del ERP más allá de la vida del lanzamiento.
	if !esperarSalud(puerto, 10*time.Second) {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			p.cmd = nil
		}
		p.estado = estadoCaydo
		p.ultimoError = "el addon no respondió /healthz en el plazo de gracia"
		h.programarReintento(p)
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.ultimoError)
		return
	}

	p.token = token
	p.puerto = puerto
	p.estado = estadoActivo
	p.fallasSalud = 0
	p.proximoIntento = time.Time{}
	p.ultimoError = ""
	p.proxy = httputil.NewSingleHostReverseProxy(destinoAddon(puerto))
	p.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("addons: %s dejó de responder en el proxy: %v", p.manifiesto.ID, err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"El addon está caído. Reinícialo en Configuración → Módulos → Addons."}`))
	}
	log.Printf("addons: %s %s activo (ruta %s, 127.0.0.1:%d)", p.manifiesto.ID, p.manifiesto.Version, p.manifiesto.Routes[0], puerto)
}

// resolverLectura pide al núcleo la lectura granular de un addon con db_read
// y devuelve las entradas de entorno extra que el proceso recibirá (el DSN
// cuando se concedió; nada en otro caso). Un fallo del lector NO tumba al
// addon: su pantalla sigue sirviendo; el motivo queda en el log y en el
// panel (lecturaAviso), y el add-on arranca sin su conexión de lectura.
func (h *addonHost) resolverLectura(p *addonProc) []string {
	if len(p.manifiesto.DBRead) == 0 {
		return nil
	}
	crear := h.crearLectura
	if crear == nil && h.s != nil {
		crear = h.s.crearUsuarioLectorAddon
	}
	if crear == nil {
		p.lecturaOK = false
		p.lecturaAviso = "la lectura granular no se concedió: este arranque no trae lector"
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.lecturaAviso)
		return nil
	}
	dsn, err := crear(p.manifiesto)
	if err != nil {
		p.lecturaOK = false
		p.lecturaAviso = "la lectura granular no se concedió: " + err.Error()
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.lecturaAviso)
		return nil
	}
	p.lecturaOK = true
	p.lecturaAviso = ""
	log.Printf("addons: %s: lectura granular concedida (%d tablas), DSN por entorno", p.manifiesto.ID, len(p.manifiesto.DBRead))
	return []string{"ERP_DB_READ_DSN=" + dsn}
}

// lanzarReal ejecuta el binario como proceso hijo con su logging etiquetado.
func (h *addonHost) lanzarReal(p *addonProc, binario string, env []string) error {
	// Sin exec.Cmd{Path...} ni su Start: en Windows ese camino SIEMPRE pasa de
	// nuevo por lookExtensions (exec/exec.go, Start de cualquier Cmd, directo o
	// no) y descarta un PE sin extensión .exe, aunque CreateProcess — que es lo
	// que CreateProcessW resuelve por sí solo — sí lo corre (probado: os.StartProcess
	// de un binario sin .exe salió exit 0). buscarBinario ya verificó qué
	// archivo se lanza: se arranca directo del sistema y se envuelve en un Cmd
	// pelado solo para conservar p.cmd.Process tal como lo usan el supervisor y
	// los handlers (Kill / chequeo nil).
	//
	// Rutas absolutas, sin excepción. Descubierto en vivo sobre Windows
	// (2026-11-01): con binario y Dir RELATIVOS ("addons\\hola\\erp-addon-hola.exe"
	// + Dir "addons\\hola") el lanzamiento fallaba con "The system cannot find
	// the path specified": CreateProcess resuelve el lpApplicationName sobre
	// el directorio de trabajo del HIJO (el Dir), no el del ERP, y esa
	// combinación buscaba el addon DENTRO de su propia carpeta. Los tests no
	// lo veían porque usan t.TempDir (absoluto). Se resuelven las DOS rutas
	// contra el directorio del ERP antes de arrancar.
	lecR, lecW, err := os.Pipe()
	if err != nil {
		return err
	}
	nulo, err := os.Open(os.DevNull)
	if err != nil {
		_ = lecR.Close()
		_ = lecW.Close()
		return err
	}
	defer nulo.Close()

	// Ambas rutas se vuelven absolutas: el ejecutable y el Dir de trabajo.
	ejecutable, err := filepath.Abs(binario)
	if err != nil {
		return fmt.Errorf("no se pudo resolver %q: %w", binario, err)
	}
	dirTrabajo, err := filepath.Abs(p.dir)
	if err != nil {
		return fmt.Errorf("no se pudo resolver %q: %w", p.dir, err)
	}

	proc, err := os.StartProcess(ejecutable, []string{ejecutable}, &os.ProcAttr{
		Dir:   dirTrabajo,
		Env:   env,
		Files: []*os.File{nulo, lecW, lecW},
	})
	// El lado de escritura del registro vive en el hijo: se cierra aqui,
	// corra o no, porque el lector es quien lo seguirá.
	_ = lecW.Close()
	if err != nil {
		_ = lecR.Close()
		return err
	}

	logW := log.New(os.Stderr, "[addon:"+p.manifiesto.ID+"] ", 0)
	go func() {
		_, _ = io.Copy(logW.Writer(), lecR)
		_ = lecR.Close()
	}()

	// El contrato del host es p.cmd *exec.Cmd: se monta con el proceso ya
	// vivo y aquí Start jamás se llama, solo Process.Kill.
	cmd := &exec.Cmd{Process: proc, Path: ejecutable, Args: []string{ejecutable}}
	p.cmd = cmd
	go func(c *exec.Cmd) {
		// Cerrando el host, se mata al hijo: nunca quedan addons huérfanos
		// escuchando en loopback después de que el ERP se fue.
		<-h.parado
		if c.Process != nil {
			_ = c.Process.Kill()
		}
	}(cmd)
	return nil
}

// esperarSalud sondea /healthz con un plazo. El addon es responsable de que
// su healthz esté listo ANTES de que la frontera lo declare activo.
func esperarSalud(puerto int, tope time.Duration) bool {
	fin := time.Now().Add(tope)
	cli := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(fin) {
		resp, err := cli.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", puerto))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// tokenEfimero genera el token de vida del addon: en memoria, por sesión, sin
// copias en disco. Sin crypto/rand no hay addon: un token predecible no es token.
func tokenEfimero() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// destinoAddon construye la URL del addon para el proxy inverso.
func destinoAddon(puerto int) *url.URL {
	return &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", puerto)}
}

// ---------- Salud: el vigil de 30 s ----------

func (h *addonHost) vigilarSalud() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-h.parado:
			return
		case <-t.C:
			h.cicloSalud()
		}
	}
}

// cicloSalud es una pasada completa y testeable del vigil: comprueba la salud
// de los activos y relanza los caídos cuyo próximo intento ya venció. Se
// separa del ticker para poder probar la recuperación sin esperar 30 s.
func (h *addonHost) cicloSalud() {
	if h == nil {
		return
	}
	h.mu.Lock()
	var lista []*addonProc
	for _, p := range h.progs {
		if p != nil {
			lista = append(lista, p)
		}
	}
	h.mu.Unlock()
	for _, p := range lista {
		h.tickSalud(p)
	}
	for _, p := range lista {
		h.reintentarSiVence(p)
	}
}

// tickSalud es el ciclo pequeño: 1 falla → nada (pudo ser un pico de carga);
// 2 fallas seguidas → caído; desde ahí, reintentos espaciados (5 s, 10 s, 20 s
// … tope 5 min). Un addon lento no es un addon caído. Los estados «detenido»
// (apagado por el master) y «sin binario» no se sondean ni se reintentan: no
// hay nada que escuchar hasta que el dueño los encienda o compile.
func (h *addonHost) tickSalud(p *addonProc) {
	if p == nil {
		return
	}
	h.mu.Lock()
	if p.estado != estadoActivo {
		h.mu.Unlock()
		return
	}
	puerto, token := p.puerto, p.token
	h.mu.Unlock()

	sano := false
	if token != "" {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", puerto))
		if err == nil {
			sano = resp.StatusCode == http.StatusOK
			if resp.Body != nil {
				resp.Body.Close()
			}
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	// El estado pudo cambiar mientras se consultaba (un reinicio o un toggle):
	// no se pisa la decisión del dueño.
	if p.estado != estadoActivo {
		return
	}
	if sano {
		p.fallasSalud = 0
		return
	}
	p.fallasSalud++
	if p.fallasSalud < 2 {
		return
	}
	p.estado = estadoCaydo
	p.ultimoError = "no respondió /healthz en el vigil de 30 s"
	retardo := time.Duration(5<<minDe(p.fallasSalud-2, 9)) * time.Second
	if retardo > 5*time.Minute {
		retardo = 5 * time.Minute
	}
	p.proximoIntento = h.ahoraDe().Add(retardo)
}

// reintentarSiVence relanza un addon caído cuando su retroceso ya venció. Es
// la pieza que antes faltaba: tickSalud marcaba «caído» y programaba el
// siguiente intento, pero nadie lo consumía y el addon quedaba muerto hasta
// que el dueño pulsaba «Reiniciar».
func (h *addonHost) reintentarSiVence(p *addonProc) {
	if p == nil {
		return
	}
	h.mu.Lock()
	if p.estado != estadoCaydo || p.proximoIntento.IsZero() || h.ahoraDe().Before(p.proximoIntento) {
		h.mu.Unlock()
		return
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.cmd = nil
	p.proximoIntento = time.Time{}
	h.mu.Unlock()
	h.lanzarOReanudar(p)
}

func minDe(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- El proxy inverso ----------

// resolverAddon busca el addon que sirve un path y devuelve la ruta CANÓNICA
// que hay que reenviar al proceso. Acepta dos formas:
//
//   - /mod/<id>[/resto] → routes[0][/resto]: la puerta estable que el núcleo
//     registra de una vez y que sirve incluso a addons instalados después del
//     arranque.
//   - /<ruta declarada>[/resto] → el mismo path: alias heredado para los
//     enlaces internos y los deep-links que ya existían.
func (h *addonHost) resolverAddon(path string) (*addonProc, string) {
	if h == nil {
		return nil, ""
	}
	if path == "/mod" || strings.HasPrefix(path, "/mod/") {
		resto := strings.TrimPrefix(strings.TrimPrefix(path, "/mod"), "/")
		if resto == "" {
			return nil, ""
		}
		id := resto
		sufijo := ""
		if i := strings.IndexByte(resto, '/'); i >= 0 {
			id = resto[:i]
			sufijo = resto[i:]
		}
		h.mu.Lock()
		p := h.progs[id]
		h.mu.Unlock()
		if p == nil || len(p.manifiesto.Routes) == 0 {
			return nil, ""
		}
		return p, p.manifiesto.Routes[0] + sufijo
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var p *addonProc
	longitud := 0
	for _, cand := range h.progs {
		for _, rt := range cand.manifiesto.Routes {
			if path == rt || strings.HasPrefix(path, rt+"/") {
				if len(rt) > longitud {
					longitud = len(rt)
					p = cand
				}
			}
		}
	}
	return p, path
}

// addonRouter es el handler del mux para las rutas de addons. Primero resuelve
// la puerta estable /mod/<id> o el alias heredado; después aplica los mismos
// candados (estado, interruptor por empresa y sesión) y reenvía al proceso.
func (s *Server) addonRouter(w http.ResponseWriter, r *http.Request) {
	h := s.addons
	if h == nil {
		jsonError(w, "No hay addons en este arranque.", http.StatusNotFound)
		return
	}
	p, canonico := h.resolverAddon(r.URL.Path)
	if p == nil {
		jsonError(w, "Esa ruta no pertenece a ningún addon instalado.", http.StatusNotFound)
		return
	}

	h.mu.Lock()
	estado := p.estado
	h.mu.Unlock()

	switch estado {
	case estadoSinBin:
		jsonError(w, fmt.Sprintf("El addon %q está en estado «sin binario»: compile su binario en su carpeta.", p.manifiesto.ID), http.StatusInternalServerError)
		return
	default:
		if estado != estadoActivo {
			jsonError(w, fmt.Sprintf("El addon %q está %s y no responde. Reinícialo en Configuración → Módulos → Addons.", p.manifiesto.ID, estado), http.StatusBadGateway)
			return
		}
	}

	// El is_active de la fila del módulo en la base de la empresa activa
	// manda: si el master apagó el módulo, el menú ya desaparece por menuTree;
	// esto cubre el deep-link pegado a mano.
	if s.addonEstaApagado(r, p.manifiesto.ID) {
		jsonError(w, "El módulo está apagado para esta empresa. Enciéndalo en Configuración → Módulos.", http.StatusNotFound)
		return
	}

	// La sesión del ERP YA fue validada por la cadena de middleware
	// (authMiddleware). Este doble candado es el contrato: el addon jamás
	// implementa login, y el núcleo jamás reenvía sin identidad que entregar.
	sess := s.sessionFromRequest(r)
	if sess == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	s.addonProxyRequest(w, r, sess, p, canonico)
}

// addonEstaApagado dice si el módulo está inactivo en la empresa activa. Sin
// fila legible se asume APAGADO: mejor una pantalla que no carga que una que
// hace lo que el master no quería.
func (s *Server) addonEstaApagado(r *http.Request, id string) bool {
	db := s.dbFor(r)
	if db == nil {
		return true
	}
	var activo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = ?`, id).Scan(&activo); err != nil {
		return true
	}
	return activo != 1
}

// addonProxyRequest reenvía al addon con las cabeceras de contexto y SIN las
// cookies del ERP: el addon parte de las cabeceras que el núcleo ya autenticó,
// y las cookies del navegador no le sirven ni deben servirle. `canonico` es la
// ruta que entiende el proceso: /mod/<id>/resto ya viene traducida a
// routes[0]+/resto; el alias heredado llega tal cual.
func (s *Server) addonProxyRequest(w http.ResponseWriter, r *http.Request, sess *SessionData, p *addonProc, canonico string) {
	req := r.Clone(r.Context())
	if canonico != "" && canonico != r.URL.Path {
		req.URL.Path = canonico
		req.URL.RawPath = ""
	}
	req.Header.Del("Cookie")
	req.Header.Del("X-Csrf-Token")
	req.Header.Set("X-ERP-User", sess.UserID)
	req.Header.Set("X-ERP-Tenant", s.tenantOf(r))
	req.Header.Set("X-ERP-Perms", s.addonPermsDelUsuario(r, sess, p.manifiesto))
	// La base de la empresa activa (fase 2). Sin esta cabecera el DSN de
	// lectura granular es una llave sin puerta: el manifiesto pide tablas y el
	// usuario lector tiene los grants, pero nadie le dice en qué base están
	// las filas de ESTA sesión —el mismo addon atiende a varias empresas—.
	// El nombre de base no es secreto: sin el grant de SELECT, entrar en ella
	// no deja leer nada.
	if s.config != nil {
		req.Header.Set("X-ERP-DB", s.tenantDBName(s.tenantOf(r)))
		// La base de IDENTIDAD (control): aquí viven users y user_login, y los
		// grants del lector ya la cubren —crearUsuarioLectorAddon empieza por
		// DB_NAME—, pero sin su nombre el addon no puede usarlas. Un módulo que
		// quiera enseñar a la gente de la empresa o compartir con alguien no
		// tiene otra puerta: su propia tabla no guarda el nombre de nadie.
		req.Header.Set("X-ERP-Auth-DB", s.config.DBName)
	}
	// El token CSRF de la sesión, para que la página del addon pueda ponerlo en
	// su formulario. El candado lo aplica EL NÚCLEO antes de reenviar
	// (enforceCSRF): el addon no valida CSRF, solo recibe la petición ya
	// pasado por el filtro. Sin esta cabecera, todo POST desde una pantalla de
	// addon saldría 403 — la plantilla no tendría cómo adjuntar el token.
	if sess.CSRFToken != "" {
		req.Header.Set("X-ERP-CSRF", sess.CSRFToken)
	}
	p.proxy.ServeHTTP(w, req)
}

// addonPermsDelUsuario arma la CSV de los permisos del manifiesto que la
// sesión tiene, o "__master" si es el super administrador. Es la MISMA
// lógica que el menú usa para mostrar/ocultar la entrada: un addon en el menú
// y su proxy no pueden relatar historias distintas.
func (s *Server) addonPermsDelUsuario(r *http.Request, sess *SessionData, m addonManifest) string {
	perm := s.permissionMap(r, sess)
	if perm["__master"] || perm["_all"] {
		return "__master"
	}
	var dadas []string
	for _, pc := range m.Perms {
		if perm[pc.Code] {
			dadas = append(dadas, pc.Code)
		}
	}
	return strings.Join(dadas, ",")
}

// ---------- El canal de negocio: addon → núcleo ----------

// addonCanal es el handler de /__addon/{id}/{verb}. Todo va autenticado con
// el token efímero y desde loopback. Fase 2: además de ping (verificación de
// vida, GET permitido porque no escribe), llega la escritura: verbo declarado
// en el manifiesto, POST con cuerpo JSON, sesión viva del usuario, empresa
// coincidente y permiso concedido; la escritura se aplica por el orquestador
// vía el handler de negocio conectado (addonVerboRegistrar), NO desde aquí.
func (s *Server) addonCanal(w http.ResponseWriter, r *http.Request) {
	// Loopback sin excepción: la entrada /__addon/ es pública en los
	// middlewares (permissions.go) precisamente para que este handler sea el
	// UNICO punto que decide.
	if !esLoopback(r.RemoteAddr) {
		jsonError(w, "El canal de addons solo acepta peticiones desde loopback.", http.StatusForbidden)
		return
	}
	token := r.Header.Get("X-ERP-Addon-Token")
	if token == "" {
		jsonError(w, "Falta el token del addon (header X-ERP-Addon-Token).", http.StatusForbidden)
		return
	}
	id, verb := addonDestinoDelPath(r.URL.Path)
	if id == "" {
		jsonError(w, "La llamada al canal no trae el id del addon.", http.StatusNotFound)
		return
	}

	if s.addons == nil {
		jsonError(w, "No hay canal de addons en este arranque.", http.StatusNotFound)
		return
	}
	s.addons.mu.Lock()
	p := s.addons.progs[id]
	tokenGuardado := ""
	if p != nil {
		tokenGuardado = p.token
	}
	s.addons.mu.Unlock()

	if p == nil {
		jsonError(w, "No hay un addon con id "+id+" cargado en este arranque.", http.StatusNotFound)
		return
	}
	if token != tokenGuardado || tokenGuardado == "" {
		jsonError(w, "El token no es el de esta sesión del addon.", http.StatusForbidden)
		return
	}

	if verb == "ping" {
		writeJSON(w, map[string]interface{}{"status": 1, "core": addonCoreVersion, "fase": addonFase})
		return
	}
	s.addonCanalEscritura(w, r, p, verb)
}

// addonCanalEscritura ejecuta un verbo de db_write. El manifiesto es el
// candado y la frontera valida en ESTE orden, cada uno con su código y su
// aviso en el log:
//
//  1. POST con cuerpo JSON válido (un GET no debe poder escribir; el cuerpo
//     pasa por MaxBytesReader: un cuerpo gigante no tumba al ERP).
//  2. El verbo está declarado en db_write (lo que no está declarado no existe:
//     403 y log).
//  3. X-ERP-User trae un usuario CON sesión viva (fail-closed: sin sesión, no
//     hay ni usuario ni empresa ni permiso — 403).
//  4. La empresa de X-ERP-Tenant coincide con la de la sesión (cross-tenant:
//     403).
//  5. El usuario tiene el permiso que el manifiesto exigió para el verbo
//     (permissionMap; el master lo pasa — 403 si no).
//  6. El handler de negocio está conectado (sin él el verbo es ruido: 500).
//
// La respuesta lleva id de petición (peticion) que también dejamos en el log:
// trazabilidad triple journal → audit_log → registro del canal.
func (s *Server) addonCanalEscritura(w http.ResponseWriter, r *http.Request, p *addonProc, verb string) {
	if r.Method != http.MethodPost {
		jsonError(w, "La escritura va por POST (el verbo "+verb+" declara db_write).", http.StatusMethodNotAllowed)
		return
	}
	// El manifiesto manda ANTES de pararse a leer el cuerpo: lo que no está
	// declarado no existe, y a un intento fuera del candado ni se le lee
	// (403 y log, como exige docs/PLAN-ADDONS.md §11).
	var verbo *addonVerbo
	for i := range p.manifiesto.DBWrite {
		v := p.manifiesto.DBWrite[i]
		if v.Verb == verb {
			verbo = &v
			break
		}
	}
	if verbo == nil {
		log.Printf("AVISO addons: canal: %q pidió el verbo %q y su manifiesto NO lo declara en db_write", p.manifiesto.ID, verb)
		jsonError(w, fmt.Sprintf("El addon %q no declara el verbo %q en db_write: el manifiesto es el candado.", p.manifiesto.ID, verb), http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var cuerpo map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		jsonError(w, "El cuerpo de la escritura no es un documento JSON válido: "+err.Error(), http.StatusBadRequest)
		return
	}
	usuario := r.Header.Get("X-ERP-User")
	empresa := r.Header.Get("X-ERP-Tenant")
	sess := s.sessions.DeUsuario(usuario)
	if sess == nil {
		log.Printf("AVISO addons: canal: verbo %q de %q con usuario %q SIN sesión viva", verb, p.manifiesto.ID, usuario)
		jsonError(w, "El usuario "+usuario+" del contexto NO tiene sesión viva en este ERP: la escritura se rechaza.", http.StatusForbidden)
		return
	}
	if empresa == "" {
		empresa = sess.TenantCode
	}
	if sess.TenantCode != "" && empresa != sess.TenantCode {
		log.Printf("AVISO addons: canal: usuario %q (empresa %q) pidió escribir en %q: empresas distintas", usuario, sess.TenantCode, empresa)
		jsonError(w, fmt.Sprintf("La empresa declarada por el canal (%q) no es la de la sesión del usuario (%q): rechazado.", empresa, sess.TenantCode), http.StatusForbidden)
		return
	}
	permisos := s.permissionMap(s.requestForSession(sess), sess)
	if !permisos["__master"] && !permisos["_all"] && !permisos[verbo.Perm] {
		log.Printf("AVISO addons: canal: usuario %q sin el permiso %q que exige el verbo %q", usuario, verbo.Perm, verb)
		jsonError(w, fmt.Sprintf("El usuario no tiene el permiso %q que el manifiesto exige para %q.", verbo.Perm, verb), http.StatusForbidden)
		return
	}
	s.addons.mu.Lock()
	accion := s.addons.acciones[p.manifiesto.ID][verb]
	s.addons.mu.Unlock()
	if accion == nil {
		log.Printf("AVISO addons: canal: el verbo %q de %q está declarado pero no tiene handler conectado", verb, p.manifiesto.ID)
		jsonError(w, "El verbo "+verb+" está declarado en el manifiesto pero este arranque no conectó su handler de negocio.", http.StatusInternalServerError)
		return
	}
	idPeticion := "addon-" + randomHex(8)
	resultado, err := accion(p, sesionDeAddon{Usuario: usuario, Empresa: empresa, Sess: sess}, cuerpo)
	log.Printf("addons: canal: %s/%s usuario=%s empresa=%s petición=%s ok=%v error=%v", p.manifiesto.ID, verb, usuario, empresa, idPeticion, err == nil, err)
	if err != nil {
		jsonError(w, "El handler de negocio del addon "+p.manifiesto.ID+" rechazó la escritura: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respuesta := map[string]interface{}{"status": 1, "peticion": idPeticion}
	for k, v := range resultado {
		respuesta[k] = v
	}
	writeJSON(w, respuesta)
}

// addonVerboRegistrar conecta el handler de negocio de UN verbo declarado en
// db_write del manifiesto de un addon. Los handlers viven FUERA de la frontera
// (cada módulo registra los suyos al arrancar); la frontera solo valida
// contexto y permiso, y despacha. Un verbo sin entrada en db_write se rechaza
// antes de llamar al handler: el manifiesto manda aunque el handler exista.
func (s *Server) addonVerboRegistrar(id, verb string, fn verboDeAddon) {
	if s == nil || s.addons == nil || id == "" || verb == "" || fn == nil {
		return
	}
	h := s.addons
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.acciones == nil {
		h.acciones = map[string]map[string]verboDeAddon{}
	}
	if h.acciones[id] == nil {
		h.acciones[id] = map[string]verboDeAddon{}
	}
	h.acciones[id][verb] = fn
}

// addonDestinoDelPath separa id y verb del canal. Se parsea a mano y no con
// r.PathValue a propósito: el mismo handler se prueba fuera de un mux real
// (las pruebas llaman addonCanal directamente, sin registrar patrones) y el
// contrato no se puede poner a depender de cómo se terminó montando la request.
func addonDestinoDelPath(path string) (id, verb string) {
	resto := strings.TrimPrefix(path, "/__addon/")
	resto = strings.Trim(resto, "/")
	if resto == "" {
		return "", ""
	}
	segs := strings.Split(resto, "/")
	id = segs[0]
	if len(segs) > 1 {
		verb = segs[1]
	}
	return id, verb
}

// esLoopback juzga si la dirección remota es la propia máquina (127.0.0.1).
func esLoopback(addr string) bool {
	host := addr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = strings.Trim(host[:i], "[]")
	}
	return host == "127.0.0.1" || host == "::1"
}

// ---------- Estado para el panel ----------

// addonsEstado es el JSON del panel: manifiesto + estado del proceso + lo que
// la frontera todavía NO concede en esta fase (dicho con claridad).
func (s *Server) addonsEstado() map[string]interface{} {
	if s == nil || s.addons == nil {
		return map[string]interface{}{
			"fase": addonFase, "core": addonCoreVersion,
			"addons": []map[string]interface{}{}, "avisos": []string{},
			"directorio": addonDirPorDefecto,
		}
	}
	h := s.addons
	h.mu.Lock()
	defer h.mu.Unlock()

	limpio := []map[string]interface{}{}
	for _, id := range idsOrdenados(h.progs) {
		p := h.progs[id]
		m := p.manifiesto
		verbos := make([]string, 0, len(m.DBWrite))
		for _, v := range m.DBWrite {
			verbos = append(verbos, v.Verb)
		}
		// Concedidos reales (fase 2): db_read solo si el núcleo entregó el DSN;
		// db_write si hay verbs declarados; tables si el escáner validó y aplicó
		// sus migraciones. La mentira en el panel es la que cuesta una noche de
		// depuración: mejor «falta X» claro que «concedido» falso.
		limpio = append(limpio, map[string]interface{}{
			"id": m.ID, "name": m.Name, "version": m.Version,
			"description": m.Description, "author": m.Author,
			"routes": m.Routes, "estado": p.estado,
			"ultimo_error": p.ultimoError,
			"db_read":      m.DBRead, "db_write": verbos,
			"tables_own": m.TablesOwn, "perms": m.Perms,
			"lectura_aviso": p.lecturaAviso,
			// Fase 3: firma del manifiesto y suscripciones de hooks.
			"firma":     p.firma,
			"hooks_out": m.HooksOut, "hooks_in": m.HooksIn,
			"concedido": map[string]bool{
				"db_read":  len(m.DBRead) > 0 && p.lecturaOK,
				"db_write": len(m.DBWrite) > 0,
				"tables":   len(m.TablesOwn) > 0,
				"hooks":    len(m.HooksOut) > 0 || len(m.HooksIn) > 0,
				"firma":    p.firma == "verificada",
			},
		})
	}
	avisos := h.avisos
	if avisos == nil {
		avisos = []string{}
	}
	claveConfigurada := h.clavePub != nil
	unsigned := h.unsigned
	h.mu.Unlock()
	historia := s.addonsHooksHistoria(30)
	h.mu.Lock()
	return map[string]interface{}{
		"fase":       addonFase,
		"core":       addonCoreVersion,
		"addons":     limpio,
		"avisos":     avisos,
		"directorio": h.directorio,
		// Fase 3: confianza. Si la clave pública del dueño no está en addons/
		// pubkey.ed25519, NADA se verifica y el panel lo dice de frente.
		"firma_publica":  claveConfigurada,
		"unsigned_mode":  unsigned && claveConfigurada,
		"historia_hooks": historia,
	}
}

// ---------- Helpers y cierre ----------

func truncado(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// idsOrdenados da un orden estable al listado.
func idsOrdenados(m map[string]*addonProc) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// parar cierra la frontera al apagar el ERP: los procesos hijos mueren; jamás
// queda un addon padrone escuchando en loopback con un token de un ERP muerto.
func (h *addonHost) parar() {
	if h == nil {
		return
	}
	h.parada.Do(func() { close(h.parado) })
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.progs {
		if p.cmd != nil && p.cmd.Process == nil {
			continue
		}
		if p.estado != estadoActivo {
			if p.cmd != nil && p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			continue
		}
		// Apagado amable: el addon pudo avisar /shutdown (el demo lo entiende).
		go func(id string, puerto int, token string, c *exec.Cmd) {
			cli := &http.Client{Timeout: 2 * time.Second}
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/shutdown", puerto), nil)
			if err == nil {
				req.Header.Set("X-ERP-Addon-Token", token)
				_, _ = cli.Do(req)
			}
			time.Sleep(500 * time.Millisecond)
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}(p.manifiesto.ID, p.puerto, p.token, p.cmd)
	}
}
