package web

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"erpgow"
)

// assetVersion devuelve una huella corta del CSS y el JS de la aplicación,
// para el parámetro ?v= de los <link> y <script>. Se calcula una vez por
// proceso.
//
// POR QUÉ HACE FALTA. Los archivos se sirven desde el mismo nombre en todas las
// versiones. Con la caché del navegador ya poblada, el usuario seguía viendo los
// colores viejos (grises) después de actualizar el ERP, y no había forma de
// distinguir «no se aplicó» de «está cacheado».
//
// El JS entró aquí por el mismo motivo y por uno peor: una corrección en
// viewpref.js —el botón «Personalizar vista» no abría nada— seguía sin llegar al
// navegador porque el script no llevaba huella. Media corrección es peor que
// ninguna: el código está bien y el usuario jura que sigue roto.
var assetVersionOnce sync.Once
var assetVersionValue string

func (s *Server) assetVersion() string {
	assetVersionOnce.Do(func() {
		h := sha256.New()
		for _, name := range []string{
			"static/css/erpgow-tema.css",
			"static/css/erpgow-refined.css",
			"static/js/viewpref.js",
			"static/js/csrf.js",
			"static/js/appearance.js",
		} {
			if b, err := fs.ReadFile(erpgow.Static(), name); err == nil {
				h.Write(b)
			}
		}
		assetVersionValue = hex.EncodeToString(h.Sum(nil))[:10]
	})
	return assetVersionValue
}

// money formats a float with 2 decimals.
func money(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

// templateFuncs are shared by every template set.
func templateFuncs(r *http.Request) template.FuncMap {
	return template.FuncMap{
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
		"money": money,
		"divf":  func(a, b float64) float64 { return a / b },
		"query": func(key string) string {
			if r == nil {
				return ""
			}
			return r.URL.Query().Get(key)
		},
		"today": today,
		"now":   now,
		// catalogLabel traduce el identificador interno del catálogo contable
		// a texto legible. Va aquí y no en la plantilla para que la web, el log
		// de arranque y la API digan lo mismo.
		"catalogLabel": catalogLabel,
	}
}

// fileStamp resume lo que cambia en disco cuando alguien edita una plantilla:
// tamaño y fecha de modificación. Las copias embebidas tienen fecha cero, que
// es una firma estable: no cambian mientras el proceso viva.
type fileStamp struct {
	size    int64
	modTime time.Time
}

// stampOf devuelve el sello de un fichero tal como lo vería el overlay (copia
// de disco primero, embebida después). Se consulta en cada render y son dos
// os.Stat: barato, y es lo que hace que guardar una plantilla se vea en el
// navegador sin reiniciar el servidor.
func stampOf(fsys fs.FS, name string) fileStamp {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{size: info.Size(), modTime: info.ModTime()}
}

// cachedTemplateSet guarda el set parseado JUNTO con los sellos de los ficheros
// con los que se construyó. Sin los sellos, la caché era para siempre y un
// cambio en templates/*.html no se reflejaba jamás hasta reiniciar.
type cachedTemplateSet struct {
	t           *template.Template
	layoutStamp fileStamp
	pageStamp   fileStamp
}

// templateSetCache caches one template.Template per page name.
type templateSetCache struct {
	mu   sync.RWMutex
	sets map[string]cachedTemplateSet
}

var tmplCache = &templateSetCache{sets: map[string]cachedTemplateSet{}}

// buildTemplateSet parses layout.html + the given page into an isolated set,
// so every page can define "content"/"pagejs" without collisions.
// Templates come from the embedded filesystem (single binary), pero si hay una
// copia en disco (desarrollo) manda esa y se relee al cambiar.
func (s *Server) buildTemplateSet(name string) (*template.Template, error) {
	layoutStamp := stampOf(erpgow.Templates(), "templates/layout.html")
	pageStamp := stampOf(erpgow.Templates(), "templates/"+name)

	tmplCache.mu.RLock()
	c, ok := tmplCache.sets[name]
	tmplCache.mu.RUnlock()
	if ok && c.layoutStamp == layoutStamp && c.pageStamp == pageStamp {
		return c.t, nil
	}

	t := template.New(name).Funcs(templateFuncs(nil))
	// "pagejs" vacío POR DEFECTO: el layout invoca {{template "pagejs" .}} y
	// Go revienta con 500 si la página no lo define (así cayeron /cartera/list
	// y /pagar/list). Se define ANTES de parsear layout y página: el último
	// "define" de un mismo nombre manda, así que una página con su propio
	// pagejs lo usa y una página sin él hereda este vacío en vez de romper.
	if _, perr := t.Parse(`{{define "pagejs"}}{{end}}`); perr != nil {
		return nil, perr
	}
	// El layout se lee SOLO del núcleo (GlobalTemplate): es el chrome global
	// (header, footer, appearance) y no puede depender de qué módulos estén
	// desplegados en este cliente.
	layout, err := erpgow.GlobalTemplate("templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("layout.html: %w", err)
	}
	// La página sí se busca en las raíces de los módulos: una vista escrita en
	// web/<code>/templates/<name>.html se pinta aquí sin más trámite, y hereda
	// el layout global de arriba.
	page, err := fs.ReadFile(erpgow.Templates(), "templates/"+name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if t, err = t.Parse(string(layout)); err != nil {
		return nil, err
	}
	if t, err = t.Parse(string(page)); err != nil {
		return nil, err
	}
	tmplCache.mu.Lock()
	tmplCache.sets[name] = cachedTemplateSet{t: t, layoutStamp: layoutStamp, pageStamp: pageStamp}
	tmplCache.mu.Unlock()
	return t, nil
}

// outOfStockCount returns the cached count of products with zero stock.
// Cached 60s to avoid hammering the DB on every page render.
var (
	outOfStockMu   sync.RWMutex
	outOfStockVal  int
	outOfStockWhen time.Time
)

func (s *Server) outOfStockCount(r *http.Request) int {
	outOfStockMu.RLock()
	if time.Since(outOfStockWhen) < 60*time.Second {
		v := outOfStockVal
		outOfStockMu.RUnlock()
		return v
	}
	outOfStockMu.RUnlock()

	// Cuenta productos ACTIVOS sin existencias, no todos los activos: el stock
	// vive en product_lot.quantity_available (mismo criterio que
	// reorder_point.go). La columna se llama `status`, no `is_active`, y la
	// consulta vieja que contaba `is_active = 1` ni siquiera compilaba contra el
	// esquema real —fallaba en silencio y el contador del layout salía en cero.
	var v int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM product_information pi
		WHERE pi.status = 1
		  AND COALESCE((SELECT SUM(pl.quantity_available) FROM product_lot pl
			WHERE pl.product_id = pi.product_id AND pl.status = 1), 0) <= 0`).Scan(&v)
	outOfStockMu.Lock()
	outOfStockVal, outOfStockWhen = v, time.Now()
	outOfStockMu.Unlock()
	return v
}

// userImageURL normalizes the avatar path stored in users.logo. PHP-era rows
// keep values like "assets/img/icons/default.jpg"; the Go app serves the same
// files under /static/. Returns "" when empty (template falls back).
func userImageURL(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") || strings.HasPrefix(p, "/") {
		return p
	}
	if strings.HasPrefix(p, "assets/") {
		return "/static/" + strings.TrimPrefix(p, "assets/")
	}
	return "/" + p
}

// render executes a page template with the common layout context.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {
	if data == nil {
		data = map[string]interface{}{}
	}
	sess := s.sessionFromRequest(r)
	st := s.settingsFor(r)

	data["AppName"] = s.config.AppName
	data["Company"] = st.Company
	data["Setting"] = st.Web
	data["Currency"] = st.Web.Currency
	data["BaseURL"] = ""
	data["CompanyName"] = st.Company.CompanyName
	// path segments for sidebar/breadcrumb active states (PHP uri->segment equivalent)
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	data["Path1"] = segments[0]
	if len(segments) > 1 {
		data["Path2"] = segments[1]
	} else {
		data["Path2"] = ""
	}
	data["CurrentURL"] = r.URL.Path
	// Versión de los estilos para romper la caché del navegador.
	//
	// Los CSS se sirven con ETag y no-cache, pero eso no alcanza: una copia ya
	// guardada con la cabecera vieja (o un proxy) sigue sirviendo la paleta de
	// gráficas anterior, y el usuario jura que los colores no cambiaron. El
	// parámetro cambia SOLO cuando cambia el archivo, así que no invalida la
	// caché en cada arranque.
	data["AssetVer"] = s.assetVersion()
	// GOti: el instructor demo. si es false, el JS ni se descarga (coste cero en prod).
	data["GuiDemo"] = s.guiDemoFlag(r)
	if data["Module"] == nil {
		data["Module"] = ""
	}
	// Apariencia: tema y densidad del usuario, escritos como atributos
	// del <html>. Van en el HTML de origen y no los pinta el navegador,
	// que es lo que evita el destello al cambiar de tema.
	app := s.appearanceFromRequest(r)
	data["Theme"] = app.Theme
	data["Density"] = app.Density
	// out-of-stock badge count for the header bell (cached)
	data["OutOfStock"] = s.outOfStockCount(r)
	// Aviso de clave de fábrica: se comprueba en cada página, no solo al
	// iniciar sesión, porque el usuario puede cambiar la clave en cualquier
	// momento y el aviso debe desaparecer en el acto. Solo lo calcula si la
	// plantilla lo va a usar, para no pagar una consulta bcrypt en cada render.
	// La comprobación va contra authDB y no contra la base de la empresa: el aviso
	// tiene que hablar de la credencial que abre la puerta, que es la de control
	// (ver identity.go). Leída de la copia de la empresa, el aviso se quedaba
	// encendido para siempre después de cambiar la clave en "Mi Perfil" —que se
	// guarda en control— y un aviso que nunca se apaga entrena a ignorar los
	// avisos, que es justo lo que este aviso existe para evitar.
	if skip, _ := data["SkipFactoryWarning"].(bool); !skip && sess != nil && (sess.UserLevel == "admin" || sess.UserLevel == "master") {
		data["FactoryPassword"] = usesFactoryPassword(s.authDB(r), sess.Username)
	}
	if sess != nil {
		data["UserID"] = sess.UserID
		data["Username"] = sess.Username
		data["Fullname"] = sess.Fullname
		data["UserLevel"] = sess.UserLevel
		data["IsAdmin"] = sess.UserLevel == "admin" || sess.UserLevel == "master"
		data["UserImage"] = userImageURL(sess.Image)
		data["Perm"] = s.permissionMap(r, sess)
		data["CSRFToken"] = s.ensureCSRFToken(sess)
	} else {
		data["Perm"] = map[string]bool{}
	}

	// El menú lateral ya no está en layout.html: se arma desde la tabla `menu`
	// y se poda con los mismos permisos que el resto de la página.
	perm, _ := data["Perm"].(map[string]bool)
	data["Menu"] = s.menuTree(r, perm, r.URL.Path)

	t, err := s.buildTemplateSet(name)
	if err != nil {
		http.Error(w, "template "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	// re-parse with request-aware funcs (query)
	tt := t.Funcs(templateFuncs(r))

	var buf bytes.Buffer
	if err := tt.ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Never let the browser reuse a stale HTML snapshot (it would reference
	// outdated CSS/JS): the response is not stored at all, so every load gets
	// a freshly rendered page.
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	buf.WriteTo(w)
}

// renderStandalone executes a full-page template (no layout), like login.html.
func (s *Server) renderStandalone(r *http.Request, w http.ResponseWriter, name string, data map[string]interface{}) {
	if data == nil {
		data = map[string]interface{}{}
	}
	data["AppName"] = s.config.AppName
	st := s.settingsFor(r)
	data["Company"] = st.Company
	data["Setting"] = st.Web
	data["Currency"] = st.Web.Currency
	// El login y las pantallas de empresa también respetan el tema. Sin
	// sesión solo se puede leer la cookie, y eso es exactamente lo que
	// guardó la última vez que el usuario eligió.
	app := s.appearanceFromRequest(r)
	data["Theme"] = app.Theme
	data["Density"] = app.Density
	// Una página completa se salta el layout, y el layout es lo que carga
	// csrf.js. Sin esto, un <form method="post"> de aquí sale sin token y el
	// middleware lo rechaza con un 403 en JSON: un error de servidor en una
	// pantalla donde el usuario solo estaba eligiendo empresa.
	//
	// El token va en el HTML y no solo en la cookie a propósito: el campo
	// oculto funciona sin JavaScript, y el login y el selector son justo las
	// pantallas donde no se puede dar por hecho que el JS haya corrido.
	if sess := s.sessionFromRequest(r); sess != nil {
		data["CSRFToken"] = s.ensureCSRFToken(sess)
	}

	t, err := template.New(name).Funcs(templateFuncs(nil)).
		ParseFS(erpgow.Templates(), "templates/"+name)
	if err != nil {
		http.Error(w, "template "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	buf.WriteTo(w)
}

// renderPartial executes a template WITHOUT the layout wrapper (AJAX fragments).
func (s *Server) renderPartial(w http.ResponseWriter, name string, data map[string]interface{}) {
	t, err := s.buildTemplateSet(name)
	if err != nil {
		http.Error(w, "template "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "content", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	buf.WriteTo(w)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, mustJSON(v))
}

// jsonError writes {"error": msg} with a status code.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

// notFoundOrJSON devuelve 404 para páginas web y JSON para APIs.
// Así no revela que la ruta existe pero requiere más privilegios.
func notFoundOrJSON(w http.ResponseWriter, r *http.Request, msg string) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.Contains(r.Header.Get("Accept"), "application/json") {
		jsonError(w, msg, http.StatusNotFound)
		return
	}
	http.Error(w, "Not Found", http.StatusNotFound)
}

// permissionMap loads sub_module permissions for the user's role.
// Admin (user_type=1) gets everything.
func (s *Server) permissionMap(r *http.Request, sess *SessionData) map[string]bool {
	perm := map[string]bool{}
	if sess == nil {
		return perm
	}
	if sess.UserLevel == "admin" || sess.UserLevel == "master" {
		db, err := s.controlStore()
		if err != nil {
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
		perm["_all"] = true
		return perm
	}
	// `create`/`read`/`update`/`delete` son palabras reservadas de MySQL: sin
	// backticks la consulta revienta con Error 1064 y permissionMap devolvía
	// vacío en silencio —el menú de todo usuario no-admin quedaba sin nada—.
	//
	// SE MIRAN LAS DOS BASES. Los roles se editan en la de control (Roles y
	// Permisos usa authDB), pero una instalación puede tener la asignación en la
	// base de la empresa. Antes esta función sólo leía control y el middleware
	// sólo leía empresa: cada uno respondía una cosa distinta a la misma
	// pregunta. Ahora ambos usan roleDBs y basta con que una base conceda.
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

// hasPermission checks a sub_module directory permission for the request user.
func (s *Server) hasPermission(r *http.Request, directory, action string) bool {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		return false
	}
	if sess.UserLevel == "admin" || sess.UserLevel == "master" {
		return true
	}
	var ok sql.NullInt64
	col := map[string]string{"create": "rp.`create`", "read": "rp.`read`", "update": "rp.`update`", "delete": "rp.`delete`"}[action]
	if col == "" {
		return false
	}
	query := fmt.Sprintf(`
		SELECT %s FROM sec_userrole ur
		JOIN role_permission rp ON rp.role_id = ur.roleid
		JOIN sub_module sm ON sm.id = rp.fk_module_id
		WHERE ur.user_id = ? AND sm.directory = ? LIMIT 1`, col)
	// Las dos bases, igual que el middleware y el menú: un rol asignado en la
	// empresa no puede valer para pintar el botón y no para ejecutarlo.
	for _, db := range s.roleDBs(r) {
		if db == nil {
			continue
		}
		if err := db.QueryRow(query, sess.UserID, directory).Scan(&ok); err == nil && ok.Int64 == 1 {
			return true
		}
		var allowed int
		err := db.QueryRow(`SELECT arp.is_allowed FROM addon_role_permissions arp
			JOIN sec_userrole ur ON ur.roleid = arp.role_id
			JOIN addons a ON a.id = arp.addon_code AND a.is_active = 1
			WHERE ur.user_id = ? AND arp.permission_code = ? AND arp.is_allowed = 1 LIMIT 1`,
			sess.UserID, directory).Scan(&allowed)
		if err == nil && allowed == 1 {
			return true
		}
	}
	return false
}
