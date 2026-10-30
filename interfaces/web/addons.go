package web

// addons.go — La frontera de los addons externos (docs/PLAN-ADDONS.md, fase 1).
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
// Qué es fase 1 y qué no. Aquí está: escáner, validación del manifiesto,
// registro en modules/menu (la pantalla Módulos que ya existe), lanzamiento con
// token efímero y puerto reservado, proxy inverso de las rutas declaradas,
// estado de salud, reinicio, y el canal de negocio /__addon/ autenticado (sin
// verbs de escritura: llegan en fase 2). NO hay: conexión de lectura (fase 2),
// creación de tablas propias (fase 2), firma del manifiesto (fase 3) ni hooks
// (fase 3). Lo declarado y no concedido se ve en el panel con esa indicación.
//
// LO QUE NO SE NEGOCIA: un addon muerto jamás derriba al ERP (solo su ruta da
// 502 con mensaje claro), las cookies del ERP jamás llegan al addon, y las
// filas de modules/menu jamás se borran (solo se marcan inactivas).

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// addonCoreVersion es lo que un manifiesto puede exigir como mínimo.
// Sube cuando la frontera gana capacidades nuevas (ahora: solo fase 1).
const addonCoreVersion = "0.1.0"

// addonFase marca la fase de la frontera que corre este binario, para que el
// panel diga la verdad («fase 1: sin escrituras ni lectura»).
const addonFase = 1

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
)

// addonDirPorDefecto es la carpeta donde viven los addons, relativa al arranque
// del proceso. Se puede sobrescribir con la variable de entorno ADDONS_DIR.
const addonDirPorDefecto = "addons"

// ---------- El manifiesto ----------

// addonMenuEntry es una entrada de menú del addon.
type addonMenuEntry struct {
	ParentID    *int     `json:"parent_id"`
	Label       string   `json:"label"`
	URL         string   `json:"url"`
	Icon        string   `json:"icon"`
	Perms       []string `json:"perms"`
	SortOrder   int      `json:"sort_order"`
	ActivePrefix string  `json:"active_prefix"`
}

// addonPermiso es un permiso declarado por el addon. El código SIEMPRE lleva
// el prefijo del id (`hola_ver`, no `ver`): es el candado del manifiesto.
type addonPermiso struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

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
	DBWrite        []string         `json:"db_write"`
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

	// Tablas: prefijo del id, para que ningún addon pueda pisar una tabla del
	// núcleo ni de otro addon (fase 2 las aplica; el candado ya va aquí).
	var tablas []string
	tablas = append(tablas, m.DBRead...)
	tablas = append(tablas, m.DBWrite...)
	tablas = append(tablas, m.TablesOwn...)
	for _, t := range tablas {
		if !addonReTabla.MatchString(t) {
			return fmt.Errorf("la tabla %q es inválida", t)
		}
		if t == m.ID || !strings.HasPrefix(t, m.ID+"_") {
			return fmt.Errorf("la tabla %q debe empezar por el prefijo del id (%s_…): las tablas del ERP no son suyas", t, m.ID)
		}
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
	estadoActivo  = "activo"
	estadoCaydo   = "caído"
	estadoSinBin  = "sin binario"
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

	// inyectables para pruebas
	lanzar func(p *addonProc, binario string, env []string) error
	ahora  func() time.Time
	parado chan struct{}
	parada sync.Once
}

// addonsIniciar arranca la frontera: escanea, registra en el mux y lanza.
// Es nil-safe y no revienta si la carpeta addons/ no existe: una instalación
// sin addons es una instalación legítima; solo se informa en el log.
func (s *Server) addonsIniciar() {
	dir := os.Getenv("ADDONS_DIR")
	if dir == "" {
		dir = addonDirPorDefecto
	}
	if _, err := os.Stat(dir); err != nil {
		log.Printf("addons: carpeta %q no encontrada; este arranque va sin addons externos", dir)
		return
	}
	h := &addonHost{
		s:           s,
		directorio:  dir,
		progs:       map[string]*addonProc{},
		rutasDelMux: map[string]bool{},
		ahora:       time.Now,
		parado:      make(chan struct{}),
	}
	h.lanzar = h.lanzarReal
	s.addons = h

	if err := h.reescanear(); err != nil {
		log.Printf("AVISO addons: no se pudo iniciar el host de addons: %v (el ERP sigue funcionando sin addons)", err)
		return
	}
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
		fila[m.ID] = &addonProc{manifiesto: m, dir: dir}
	}
	return fila, avisos
}

// reescanear aplica el estado de la carpeta addons/ al ERP: manifiestos →
// filas de modules/menu → procesos → rutas del proxy. Es el botón
// «Re-escanear addons» y el arranque. Las rutas del mux se registran UNA vez
// (al arrancar, cuando aún no hay tráfico): registrar rutas dinámicamente en
// caliente sería una carrera con el propio ServeMux; los addons nuevos que
// lleguen al vuelo aparecen con el próximo reinicio del ERP, y el panel lo
// dice así.
func (h *addonHost) reescanear() error {
	if h == nil {
		return nil
	}
	fila, avisos := h.escanear()
	idsNuevos := idsOrdenados(fila)

	h.mu.Lock()
	// Los addons que desaparecieron de la carpeta: dejar de servir, marcar
	// inactivo en la DB (las filas jamás se borran: es la trazabilidad del
	// repo) y decirlo en el log. Sus rutas siguen registradas en el mux, pero
	// el proxy ya no las resuelve: devuelve «addon no instalado».
	for id, p := range h.progs {
		if _, queda := fila[id]; !queda {
			if p.cmd != nil && p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			h.marcaInactivoEnBD(id)
			delete(h.progs, id)
		}
	}
	h.mu.Unlock()

	for _, id := range idsNuevos {
		nuevo := fila[id]
		h.mu.Lock()
		// Se conserva el proceso vivo entre rescaneos: mismo binario y mismo
		// manifiesto = mismo proceso. Solo se reinicia si el binario cambió
		// (se recompiló) o si el anterior no estaba en pie.
		if previo, estaba := h.progs[id]; estaba {
			nuevo.cmd = previo.cmd
			nuevo.token = previo.token
			nuevo.puerto = previo.puerto
			nuevo.binario = previo.binario
			nuevo.estado = previo.estado
			nuevo.fallasSalud = previo.fallasSalud
			nuevo.ultimoError = previo.ultimoError
			nuevo.proxy = previo.proxy
			nuevo.manifiesto = previo.manifiesto // el vivo manda; el nuevo pide reinicio
			nuevo.dir = previo.dir
		}
		h.progs[id] = nuevo
		rebuilt := nuevo.binario != nuevo.buscarBinario()
		reiniciar := nuevo.estado == "" || nuevo.estado == estadoCaydo ||
			nuevo.estado == estadoSinBin || rebuilt
		h.mu.Unlock()

		if err := h.registrarEnBD(nuevo.manifiesto); err != nil {
			avisos = append(avisos, fmt.Sprintf("%s: no se pudo registrar en modules/menu: %v", id, err))
		}
		if reiniciar {
			h.lanzarOReanudar(nuevo)
		}
	}
	if len(avisos) > 0 || len(idsNuevos) > 0 {
		invalidarCacheMenu()
	}
	h.mu.Lock()
	h.avisos = avisos
	h.mu.Unlock()
	for _, aviso := range avisos {
		log.Printf("addons: %s", aviso)
	}
	return nil
}

// registrarRutasAddon es la ÚNICA vez que se registran rutas de addons en el
// mux: en el arranque. Va acompañada del escaneo inicial.
func (s *Server) registrarRutasAddon() {
	if s == nil || s.addons == nil {
		return
	}
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

	// El canal de negocio SIEMPRE existe, aunque no haya addons: es el
	// receptor del núcleo, y responder «no hay addon con ese id» sin exponer
	// tokens es parte del contrato.
	s.mux.HandleFunc("/__addon/{id}/{verb}", s.addonCanal)
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

// registrarEnBD sincroniza las filas del addon con modules y menu de la base
// de empresa. Nada se borra: lo que ya no está declarado se marca inactivo.
func (h *addonHost) registrarEnBD(m addonManifest) error {
	db := h.dbDeAddon()
	if db == nil {
		return nil // sin base (arranque en pruebas): nada que grabar
	}
	var existe int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modules WHERE id = ?`, m.ID).Scan(&existe); err == nil {
		if existe == 0 {
			// Primera instalación: nace activo. El master decide después con
			// el toggle; el escaneo no vuelve a encender lo que el master apagó.
			if _, err := db.Exec(`INSERT INTO modules (id, name, version, description, is_core, is_active, path, author, license_type)
				VALUES (?, ?, ?, ?, 0, 1, ?, ?, 'MIT')`,
				m.ID, m.Name, m.Version, truncado(m.Description, 255), h.directorio, truncado(m.Author, 100)); err != nil {
				return fmt.Errorf("insertar módulo: %w", err)
			}
		} else {
			// Sin tocar is_active. Solo se refrescan los datos descriptivos.
			if _, err := db.Exec(`UPDATE modules SET name = ?, version = ?, description = ?, path = ?, author = ?
				WHERE id = ?`, m.Name, m.Version, truncado(m.Description, 255), h.directorio, truncado(m.Author, 100), m.ID); err != nil {
				return fmt.Errorf("actualizar módulo: %w", err)
			}
		}
	}
	return h.sincronizarMenu(db, m)
}

// sincronizarMenu refleja el manifiesto en el menú: las entradas declaradas se
// insertan (si no existen) o encienden/actualizan (si existen); las que ya no
// están declaradas, status = 0.
func (h *addonHost) sincronizarMenu(db *sql.DB, m addonManifest) error {
	deseadas := map[string]bool{}
	for _, e := range m.Menu {
		deseadas[e.URL] = true
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
		var existe int
		_ = db.QueryRow(`SELECT COUNT(*) FROM menu WHERE module_id = ? AND url = ?`, m.ID, e.URL).Scan(&existe)
		if existe == 0 {
			if _, err := db.Exec(`INSERT INTO menu (parent_id, label, url, module_id, is_core, icon, sort_order, status, perm)
				VALUES (?, ?, ?, ?, 0, ?, ?, 1, ?)`,
				e.ParentID, e.Label, e.URL, m.ID, e.Icon, e.SortOrder, strings.Join(e.Perms, "|")); err != nil {
				return fmt.Errorf("insertar el menú %q: %w", e.Label, err)
			}
		} else {
			if _, err := db.Exec(`UPDATE menu SET label = ?, icon = ?, sort_order = ?, status = 1, perm = ?
				WHERE module_id = ? AND url = ?`, e.Label, e.Icon, e.SortOrder, strings.Join(e.Perms, "|"), m.ID, e.URL); err != nil {
				return fmt.Errorf("actualizar el menú %q: %w", e.Label, err)
			}
		}
	}
	return nil
}

// marcaInactivoEnBD marca el addon como «no instalado» SIN borrar nada:
// modules (is_active = 0) y menú (status = 0) quedan como evidencia.
func (h *addonHost) marcaInactivoEnBD(id string) {
	db := h.dbDeAddon()
	if db == nil {
		return
	}
	_, _ = db.Exec(`UPDATE modules SET is_active = 0 WHERE id = ? AND is_core = 0`, id)
	_, _ = db.Exec(`UPDATE menu SET status = 0 WHERE module_id = ?`, id)
}

// addonRegistrarEmpresa es lo que se llama cuando se prepara una empresa
// nueva: los addons ya escaneados deben existir también en esa base.
func (s *Server) addonRegistrarEmpresa(db *sql.DB) {
	if s == nil || s.addons == nil || db == nil {
		return
	}
	s.addons.mu.Lock()
	var lista []addonManifest
	for _, p := range s.addons.progs {
		lista = append(lista, p.manifiesto)
	}
	s.addons.mu.Unlock()
	for _, m := range lista {
		if err := s.addons.registrarEnDB(db, m); err != nil {
			log.Printf("addons: no se pudo registrar %s en la empresa nueva: %v", m.ID, err)
		}
	}
}

// registrarEnDB es la variante con DB explícita de registrarEnBD (se usa al
// preparar una empresa, cuando no hay petición activa).
func (h *addonHost) registrarEnDB(db *sql.DB, m addonManifest) error {
	if db == nil {
		return nil
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

// ---------- Lanzamiento del proceso hijo ----------

// buscarBinario localiza el binario del addon en su carpeta: erp-addon-<id>
// con o sin .exe según el sistema. Sin binario compilado, el addon se ve en el
// panel como «sin binario» (verdadero, no mentira); en ese estado no hay proxy.
func (p *addonProc) buscarBinario() string {
	nombre := "erp-addon-" + p.manifiesto.ID
	var candidatos []string
	if runtime.GOOS == "windows" {
		candidatos = []string{nombre + ".exe"}
	} else {
		candidatos = []string{nombre, nombre + ".exe"}
	}
	for _, c := range candidatos {
		completo := filepath.Join(p.dir, c)
		if st, err := os.Stat(completo); err == nil && st.Mode()&0111 != 0 {
			return completo
		}
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

// lanzarOReanudar arranca el addon como proceso propio, con token efímero y
// puerto reservado. Un fallo queda explicado en el panel, no escondido.
func (h *addonHost) lanzarOReanudar(p *addonProc) {
	if h == nil || p == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lanzarBajoCandado(p)
}

func (h *addonHost) lanzarBajoCandado(p *addonProc) {
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

	iniciar := h.lanzar
	if iniciar == nil {
		iniciar = h.lanzarReal
	}
	if err := iniciar(p, binario, env); err != nil {
		p.estado = estadoCaydo
		p.ultimoError = "no se pudo lanzar el proceso: " + err.Error()
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.ultimoError)
		return
	}

	// El addon tiene un plazo fijo para estar sano; si no, queda caído y el
	// vigil lo reintenta con retroceso (ver tickSalud). Nada de esto congela
	// el arranque del ERP más allá de la vida del lanzamiento.
	if !esperarSalud(puerto, 10*time.Second) {
		p.estado = estadoCaydo
		p.ultimoError = "el addon no respondió /healthz en el plazo de gracia"
		log.Printf("AVISO addons: %s: %s", p.manifiesto.ID, p.ultimoError)
		return
	}

	p.token = token
	p.puerto = puerto
	p.estado = estadoActivo
	p.fallasSalud = 0
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

// lanzarReal ejecuta el binario como proceso hijo con su logging etiquetado.
func (h *addonHost) lanzarReal(p *addonProc, binario string, env []string) error {
	cmd := exec.Command(binario)
	cmd.Env = env
	cmd.Dir = p.dir
	logW := log.New(os.Stderr, "[addon:"+p.manifiesto.ID+"] ", 0)
	cmd.Stdout = logW.Writer()
	cmd.Stderr = logW.Writer()
	if err := cmd.Start(); err != nil {
		return err
	}
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
		}
	}
}

// tickSalud es el ciclo pequeño: 1 falla → nada (pudo ser un pico de carga);
// 2 fallas seguidas → caído; desde ahí, reintentos espaciados (5 s, 10 s, 20 s
// … tope 5 min). Un addon lento no es un addon caído.
func (h *addonHost) tickSalud(p *addonProc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p.token != "" && p.estado == estadoActivo {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", p.puerto))
		if err == nil {
			defer func() {
				if resp != nil && resp.Body != nil {
					resp.Body.Close()
				}
			}()
			if resp.StatusCode == http.StatusOK {
				p.fallasSalud = 0
				return
			}
		}
	}
	p.fallasSalud++
	if p.fallasSalud < 2 {
		return
	}
	p.estado = estadoCaydo
	retardo := time.Duration(5<<minDe(p.fallasSalud-2, 9)) * time.Second
	if retardo > 5*time.Minute {
		retardo = 5 * time.Minute
	}
	p.proximoIntento = h.ahora().Add(retardo)
}

func minDe(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- El proxy inverso ----------

// addonRouter es el handler del mux para las rutas de addons: busca el addon
// por prefijo más largo de ruta declarada.
func (s *Server) addonRouter(w http.ResponseWriter, r *http.Request) {
	h := s.addons
	if h == nil {
		jsonError(w, "No hay addons en este arranque.", http.StatusNotFound)
		return
	}
	h.mu.Lock()
	var p *addonProc
	longitud := 0
	for _, cand := range h.progs {
		for _, rt := range cand.manifiesto.Routes {
			if r.URL.Path == rt || strings.HasPrefix(r.URL.Path, rt+"/") {
				if len(rt) > longitud {
					longitud = len(rt)
					p = cand
				}
			}
		}
	}
	h.mu.Unlock()

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
	s.addonProxyRequest(w, r, sess, p)
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
// cookies del ERP: el addonde parte de las cabeceras que el núcleo ya
// autenticó, y las cookies del navegador no le sirven ni deben servirle.
func (s *Server) addonProxyRequest(w http.ResponseWriter, r *http.Request, sess *SessionData, p *addonProc) {
	req := r.Clone(r.Context())
	req.Header.Del("Cookie")
	req.Header.Del("X-Csrf-Token")
	req.Header.Set("X-ERP-User", sess.UserID)
	req.Header.Set("X-ERP-Tenant", s.tenantOf(r))
	req.Header.Set("X-ERP-Perms", s.addonPermsDelUsuario(r, sess, p.manifiesto))
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
// el token efímero y desde loopback. En fase 1 no hay verbs de escritura: el
// único implementado es ping (verificación de vida del canal). Cualquier otro
// verb responde 404 con el mensaje exacto de que la escritura es fase 2.
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

	switch verb {
	case "ping":
		writeJSON(w, map[string]interface{}{"status": 1, "core": addonCoreVersion, "fase": addonFase})
	default:
		jsonError(w, fmt.Sprintf("El verbo %q no está en la fase %d de la frontera: escrituras y lectura llegan en fase 2.", verb, addonFase), http.StatusNotFound)
	}
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
		limpio = append(limpio, map[string]interface{}{
			"id": m.ID, "name": m.Name, "version": m.Version,
			"description": m.Description, "author": m.Author,
			"routes": m.Routes, "estado": p.estado,
			"ultimo_error": p.ultimoError,
			"db_read":      m.DBRead, "db_write": m.DBWrite,
			"tables_own": m.TablesOwn, "perms": m.Perms,
			"concedido": map[string]bool{
				"db_read": false, "db_write": false, "tables": false, "hooks": false,
			},
		})
	}
	avisos := h.avisos
	if avisos == nil {
		avisos = []string{}
	}
	return map[string]interface{}{
		"fase":    addonFase,
		"core":    addonCoreVersion,
		"addons":  limpio,
		"avisos":  avisos,
		"directorio": h.directorio,
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
