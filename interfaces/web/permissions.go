package web

// permissions.go — Sistema de control de acceso unificado.
//
// JERARQUÍA DE NIVELES (de mayor a menor):
//   master    → Super Admin, control total, crea empresas, asigna roles globales
//   admin     → Admin de empresa, gestiona solo su empresa asignada
//   manager   → Supervisor/Gerente, autoriza operaciones especiales
//   employee  → Usuario operativo, solo lectura en sus módulos asignados
//
// REGLAS DE SEGURIDAD (hardcoded, no dependen del prompt del admin):
//   1. Solo master puede crear empresas
//   2. Solo master puede crear usuarios y asignar roles
//   3. Admin de empresa gestiona solo su empresa
//   4. Ningún usuario puede modificar sus propios permisos
//   5. Los usuarios IA tienen las mismas limitaciones que los humanos
//   6. Las rutas de sistema (user, permission, setting, tenant) requieren master
//   7. El acceso a datos entre empresas está prohibido para no-master

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Módulos del sistema (department_id).
//
// LOS VALORES SON module.id (= sub_module.mid), la taxonomía canónica de la
// base: 1 invoice, 2 customer, 3 product, 4 supplier, 5 purchase, 6 stock,
// 7 return, 8 report, 9 accounts, 10 bank, 11 tax, 12 hrm, 13 service,
// 14 setting, 15 quotation, 18 ai. ANTES este mapa usaba una numeración
// propia (ventas=4, rrhh=3, compras=7…) que no coincidía con sub_module.mid:
// canAccessModule comparaba `sm.mid = department_id` contra la taxonomía
// equivocada, así que el membership de user_department y la matriz de
// role_permission señalaban módulos cruzados (p. ej. /hr pedía permisos de
// producto). Se reescribió con los ids reales de `module`.
//
// /cashsession NO está a propósito: es un menú sin permiso y todo cajero
// necesita abrir turno aunque no tenga ningún módulo asignado.
var routeDepartment = map[string]int{
	"/account": 9, "/adjustment": 6, "/ai": 18, "/ai_agents": 18,
	"/ai_department": 18, "/ai_reports": 18, "/ai_tasks": 18, "/ai_catalog": 18,
	"/appearance": 14, "/audit": 14, "/bank": 10, "/coa": 9,
	"/coa_catalog": 9, "/conasev": 8, "/cost": 1, "/creditnote": 1,
	// CRM cuelga del módulo de personal (mid=12) en la tabla sub_module, no
	// de clientes (2): el permiso «crm» que se marca en Roles pertenece a 12,
	// así que mapearlo a 2 hacía que el middleware buscara un permiso que
	// nadie tenía y devolviera 403 a quien sí lo tenía concedido.
	"/crm": 12, "/customer": 2, "/cycle_count": 6,
	"/database_settings": 14, "/exchange_rate": 14,
	"/fiscal": 11, "/hr": 12, "/inventory": 6, "/invoice": 1,
	"/kardex": 8, "/legal_books": 8, "/pagar": 5, "/payroll": 12,
	"/period": 9, "/permission": 14, "/product": 3, "/purchase": 5,
	"/quotation": 15, "/reorder_point": 6, "/report": 8, "/return": 7,
	"/service": 13, "/setting": 14, "/supplier": 4, "/tenant": 14,
	"/user": 14, "/view_pref": 14, "/voucher_extra": 9, "/cartera": 9,

	// PREFIJOS QUE FALTABAN, Y NO ERAN COSMÉTICOS.
	//
	// moduleForRoute sólo reconoce lo que está en este mapa. Una ruta que no
	// figura aquí no entra en la comprobación de módulo y queda ABIERTA a
	// cualquier usuario con sesión, sin importar su rol. Éstas estaban fuera:
	//
	//   /reports  → TODOS los reportes (ventas, compras, inventario, libros
	//               legales, cierre diario). El mapa tenía «/report» (singular),
	//               que no casa con «/reports/...»; se comprobó con un usuario
	//               empleado sin ningún permiso: abría /reports/stock con 200.
	//   /debitnote → notas de débito, que son facturación (módulo 1).
	//   /accounting → asientos contables (9).
	//   /data      → copias de seguridad (14, configuración).
	//   /module    → administración de addons (14, configuración).
	"/reports": 8, "/debitnote": 1, "/accounting": 9,
	"/data": 14, "/module": 14,

	// Mensajería tiene su propio módulo (25). No comparte id con ninguno, así
	// que el middleware puede decidir por módulo; el permiso fino de chat,
	// envío y administración se comprueba en el handler y en routeDirectory.
	"/mensajeria": 25,
}

// routeDirectory exige, ADEMÁS del módulo, el permiso concreto de la entrada de
// menú. Existe para los módulos que comparten `module.id` con otro y por tanto
// no se pueden separar con routeDepartment:
//
//   - Costos: sus permisos (cost, cost_timesheet, cost_close) viven en el
//     módulo 1 (facturación) en la tabla sub_module. Con sólo el módulo, un
//     vendedor con «crear factura» abría /cost y veía los costos internos del
//     negocio —comprobado: 200 con un rol que no tenía Costos—.
//   - CRM: comparte módulo con personal; se deja aquí por la misma razón.
//
// El valor admite lista OR separada por «|», igual que menu.perm.
var routeDirectory = map[string]string{
	"/cost": "cost|cost_timesheet|cost_close",
	"/crm":  "crm",
	// Mensajería: ver una conversación y mandar un mensaje son permisos
	// distintos, y administrar los números es un tercero. El módulo (25) solo
	// dice «esta persona trabaja en mensajería»; la entrada de menú y esta
	// comprobación deciden qué puede hacer.
	"/mensajeria": "mensajeria_chat|mensajeria_send|mensajeria_admin",
}

// Rutas que requieren master (gestión global del sistema)
var masterOnlyRoutes = map[string]bool{
	"/user": true, "/permission": true, "/setting": true, "/tenant": true,
	"/ai": true, "/ai_agents": true, "/database_settings": true,
	"/setup/company": true, "/admin/pos": true, "/pos/api": true, "/config/menu": true,
	// /role: la matriz de permisos decide qué puede hacer cada nivel; dejarla
	// abierta permitía que cualquier usuario con sesión reescribiera su propio
	// rol (escalamiento de privilegios). Antes solo estaba bajo /permission.
	"/role": true,
	// /settings: las pantallas reales de configuración viven en /settings/*
	// (no en /setting, que es el prefijo viejo). Sin esta entrada cualquier
	// usuario con sesión podía abrir /settings/fiscal y reconfigurar el RIF.
	"/settings": true,
	// /module: activar o desactivar un módulo cambia lo que ve TODA la empresa y
	// enciende o apaga funciones completas (mensajería, addons). Estaba solo en
	// routeDepartment, así que un gerente con el módulo de configuración
	// concedido podía pulsar «Activar» en /module/list. Encender mensajería es
	// una decisión del super administrador, igual que /setting y /tenant.
	"/module": true,
}

// Rutas que requieren admin o master (gestión de empresa)
var adminRoutes = map[string]bool{
	"/customer": true, "/supplier": true, "/product": true, "/purchase": true,
	"/invoice": true, "/creditnote": true, "/return": true, "/cartera": true,
	"/pagar": true, "/bank": true, "/hr": true, "/payroll": true,
}

// Rutas prohibidas para usuarios no-master (no pueden ver otras empresas)
var crossTenantRoutes = map[string]bool{
	"/tenant": true, "/setup/company": true,
}

func moduleForRoute(path string) (int, bool) {
	best := 0
	bestDept := 0
	for prefix, dept := range routeDepartment {
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			if len(prefix) > best {
				best = len(prefix)
				bestDept = dept
			}
		}
	}
	return bestDept, bestDept != 0
}

func actionFromMethod(method string) string {
	switch method {
	case http.MethodGet:
		return "read"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	}
	return "read"
}

// Carga el nivel de rol desde la BD (única fuente de verdad)
func loadUserRole(db *sql.DB, userID string) (string, error) {
	var level string
	query := `SELECT r.level
		FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid
		WHERE ur.user_id = ?
		LIMIT 1`
	err := db.QueryRow(query, userID).Scan(&level)
	if err == sql.ErrNoRows {
		// Sin rol en ESTA base NO es lo mismo que "es employee": en una
		// empresa recién creada (erpgo_textil) sec_userrole está vacía, y
		// devolver ("employee", nil) anulaba el fallback a la BD de control
		// del middleware —el if era `err != nil || level == ""`— así que el
		// dueño de la empresa recibía 403 en todas las rutas de admin. Se
		// devuelve el error para que el llamador decida (el middleware cae
		// al catálogo de control, que es donde vive el rol global).
		return "", sql.ErrNoRows
	}
	return level, err
}

// levelDeTipoDeUsuario traduce user_type a un nivel, que es la identidad que el
// login crea para el administrador de una empresa (user_type=1).
//
// Vive aquí, y no solo en handlers.go, porque el middleware de permisos necesita
// la MISMA respuesta que el menú: si el menú dice "este es administrador" y el
// middleware dice "este es empleado", el usuario ve entradas que al pulsarlas le
// devuelven 403. Devuelve "" cuando el usuario no es de tipo 1 o no existe en esa
// base, para que quien llame pueda probar la siguiente (empresa, luego control).
func levelDeTipoDeUsuario(db *sql.DB, userID string) string {
	if db == nil || userID == "" {
		return ""
	}
	var tipo sql.NullInt64
	if err := db.QueryRow(`SELECT user_type FROM user_login WHERE user_id = ? LIMIT 1`, userID).Scan(&tipo); err != nil {
		return ""
	}
	if tipo.Valid && tipo.Int64 == 1 {
		return "admin"
	}
	return ""
}

func loadUserDepartments(db *sql.DB, userID string) ([]int, error) {
	rows, err := db.Query(`SELECT department_id FROM user_department WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var depts []int
	for rows.Next() {
		var d int
		_ = rows.Scan(&d)
		depts = append(depts, d)
	}
	return depts, rows.Err()
}

func loadUserTenants(db *sql.DB, userID string) ([]string, error) {
	rows, err := db.Query(`SELECT tenant_code FROM user_company_access WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tenants []string
	for rows.Next() {
		var t string
		_ = rows.Scan(&t)
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// canAccessTenant verifica que el usuario tenga acceso a una empresa específica
func canAccessTenant(level string, userTenants []string, targetTenant string) bool {
	if level == "master" {
		return true
	}
	for _, t := range userTenants {
		if t == targetTenant {
			return true
		}
	}
	return false
}

// canAccessModule verifica permisos de módulo/acción.
//
// CONSULTA LAS DOS BASES, Y ESO ES EL ARREGLO.
//
// Los roles y su matriz viven en la base de CONTROL (es donde los escribe la
// pantalla de Roles: roleSave/rolePermissionsSave usan authDB). Pero esta
// función sólo miraba la base de la EMPRESA, donde no hay ni una fila de
// sec_userrole en una instalación normal. Resultado medido: un usuario con rol
// «Vendedor» y los permisos de facturación marcados en Roles abría el menú
// correcto (permissionMap sí consultaba control) y al pulsar «Nueva Factura»
// recibía 403; en cambio /reports/stock, sin permiso ninguno, respondía 200.
// Es decir: la interfaz prometía una cosa y el middleware decidía otra.
//
// Se comprueba empresa primero (deployment donde el rol se asigna por empresa)
// y control después (el caso normal); basta con que UNA conceda el permiso.
func canAccessModule(dbs []*sql.DB, userID, level string, departmentID int, action string) bool {
	// Master tiene acceso total
	if level == "master" {
		return true
	}
	// Admin de empresa tiene acceso total a su empresa
	if level == "admin" {
		return true
	}
	if departmentID == 0 {
		return true
	}
	// Employee solo lectura
	if level == "employee" && action != "read" {
		return false
	}

	col := map[string]string{
		"create": "rp.`create`", "read": "rp.`read`",
		"update": "rp.`update`", "delete": "rp.`delete`",
	}[action]
	if col == "" {
		col = "rp.`read`"
	}

	query := fmt.Sprintf(`SELECT %s FROM sec_userrole ur
		JOIN role_permission rp ON rp.role_id = ur.roleid
		JOIN sub_module sm ON sm.id = rp.fk_module_id
		WHERE ur.user_id = ? AND sm.mid = ?
		AND sm.status = 1
		LIMIT 1`, col)

	for _, db := range dbs {
		if db == nil {
			continue
		}
		var ok sql.NullInt64
		if err := db.QueryRow(query, userID, departmentID).Scan(&ok); err == nil && ok.Int64 == 1 {
			return true
		}
	}
	return false
}

// roleDBs devuelve las bases donde puede vivir la asignación de rol del
// usuario: la de la empresa y la de control, sin repetir si son la misma.
//
// Existe para que el middleware y el menú pregunten EXACTAMENTE lo mismo. Antes
// el menú leía control y el middleware leía la empresa: dos respuestas
// distintas para la misma pregunta, que es el origen de «veo el módulo pero me
// dice que no tengo permiso».
func (s *Server) roleDBs(r *http.Request) []*sql.DB {
	out := make([]*sql.DB, 0, 2)
	if db := s.dbFor(r); db != nil {
		out = append(out, db)
	}
	if ctl, err := s.controlStore(); err == nil && ctl != nil {
		repetida := false
		for _, db := range out {
			if db == ctl {
				repetida = true
				break
			}
		}
		if !repetida {
			out = append(out, ctl)
		}
	}
	return out
}

// isMasterRoute verifica si la ruta requiere permisos de master
func isMasterRoute(path string) bool {
	for prefix := range masterOnlyRoutes {
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			return true
		}
	}
	return false
}

// isAdminRoute verifica si la ruta requiere permisos de admin o master
func isAdminRoute(path string) bool {
	for prefix := range adminRoutes {
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			return true
		}
	}
	return false
}

// exigirPermiso aplica la MATRIZ DEL ROL a la ACCIÓN REAL de la ruta y responde
// 403 cuando no la concede. Devuelve true si el handler puede seguir.
//
// POR QUÉ HACE FALTA ALGO MÁS QUE EL MIDDLEWARE. El middleware deduce la acción
// del MÉTODO HTTP, y con eso «ver, editar, insertar o borrar» no se puede
// expresar: /supplier/save inserta o edita según venga el id, y
// /supplier/delete/{id} es un POST del que el método sólo dice «create». Las
// tres operaciones —alta, edición y borrado— quedaban bajo el mismo permiso, así
// que un rol con «editar proveedor» podía además crear y borrar. Aquí se
// comprueba la acción de verdad, en el único sitio donde se conoce: el handler.
//
// La puerta es la MISMA que usa el resto del sistema (hasPermission): master y
// admin tienen todo, «employee» es solo lectura —igual que canAccessModule— y el
// resto depende de role_permission, mirando empresa y control. Y se admiten
// varios directorios (OR) porque hay pantallas que sirven a dos módulos: la
// lista de proveedores la abre tanto quien gestiona proveedores como quien los da
// de alta, y el buscador lo usan también los formularios de compra.
func (s *Server) exigirPermiso(w http.ResponseWriter, r *http.Request, accion string, directorios ...string) bool {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "No autorizado. Inicie sesión.", http.StatusUnauthorized)
		return false
	}
	// «employee» es solo lectura: es la regla del sistema (cabecera de este
	// archivo) y canAccessModule ya la aplica al módulo. Sin repetirla aquí, un
	// rol de empleado con la casilla marcada en la matriz podría escribir por
	// esta puerta y no por la del módulo.
	if accion != "read" && sess.UserLevel == "employee" {
		jsonError(w, "Su rol es de solo lectura: no puede "+accion+" en este módulo.", http.StatusForbidden)
		return false
	}
	for _, dir := range directorios {
		if dir != "" && s.hasPermission(r, dir, accion) {
			return true
		}
	}
	jsonError(w, fmt.Sprintf("Su rol no tiene el permiso «%s» (%s) en Roles y Permisos.", strings.Join(directorios, "» o «"), accion), http.StatusForbidden)
	return false
}

// isCrossTenantRoute verifica si la ruta puede acceder datos entre empresas
func isCrossTenantRoute(path string) bool {
	for prefix := range crossTenantRoutes {
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			return true
		}
	}
	return false
}

func (s *Server) permissionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rutas públicas
		// /api/pos/ entero: registrado como público en registerPOSSyncRoutes
		// ("Las rutas bajo /api/pos/ son públicas"). Cada una lleva su propia
		// autenticación (API key o token X-POS-Token); sin esta entrada, el
		// terminal POS recibía 403 y quedaba en modo offline permanente.
		// /__addon/: canal de negocio de los addons. No lleva cookie porque un
		// addon no tiene sesión de usuario; se autentica con un grant efímero
		// emitido por el núcleo y solo acepta peticiones desde loopback.
		public := []string{"/login", "/api/login", "/api/pos/", "/__addon/", "/setup/company", "/static/", "/uploads/", "/favicon"}
		for _, p := range public {
			if strings.HasPrefix(r.URL.Path, p) {
				next.ServeHTTP(w, r)
				return
			}
		}

		sess := s.sessionFromRequest(r)
		if sess == nil {
			// Redirigir al login en vez de devolver JSON
			if strings.HasPrefix(r.URL.Path, "/api/") {
				jsonError(w, "No autorizado. Inicie sesión.", http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusFound)
			}
			return
		}

		db := s.dbFor(r)
		if db == nil {
			jsonError(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		// Cargar nivel de rol desde BD (única fuente de verdad)
		level, err := loadUserRole(db, sess.UserID)
		if err != nil || level == "" {
			// Fallback a la BD de control: en una empresa recién creada
			// (p. ej. erpgo_textil) sec_userrole está vacía y TODA la
			// UI quedaba en 403 para cualquier usuario. El rol global
			// vive en la BD de control —es la misma que usan el login y
			// el POS— y el rol por empresa, si existe, sigue teniendo
			// prioridad porque se consultó primero.
			if ctl, cerr := s.controlStore(); cerr == nil {
				level, _ = loadUserRole(ctl, sess.UserID)
			}
		}
		if level == "" {
			// Sin fila de rol en NINGUNA de las dos bases. Eso no es un empleado:
			// es el administrador de la empresa, cuya identidad se creó con
			// user_type=1 y a la que —a propósito— no se le pone fila en
			// sec_userrole (ver identity.go). El menú y el dashboard ya resolvían
			// esa identidad por user_type (handlers.go, "si level == \"\" …
			// userType == 1 → admin"); el middleware no lo hacía, así que el
			// administrador de una empresa recién creada entraba como "employee"
			// y recibía 403 "No tiene permisos para este módulo" en casi todas
			// las pantallas —incluidas las suyas propias—.
			level = levelDeTipoDeUsuario(db, sess.UserID)
			if level == "" {
				if ctl, cerr := s.controlStore(); cerr == nil {
					level = levelDeTipoDeUsuario(ctl, sess.UserID)
				}
			}
		}
		if level == "" {
			level = "employee"
		}

		// Master tiene acceso total
		if level == "master" {
			next.ServeHTTP(w, r)
			return
		}

		// Verificar acceso a empresa
		tenants, _ := loadUserTenants(db, sess.UserID)
		tenantCode := s.tenantOfSession(sess)
		if !canAccessTenant(level, tenants, tenantCode) {
			// Fallback al catálogo de membresía de la BD de control
			// (tenant_users), la misma fuente que valida el selector de
			// empresa. user_company_access en la base de la empresa está
			// vacía en las empresas nuevas, y sin este fallback el admin
			// recibía 403 "no access to this company" en toda la UI.
			if !s.CanAccessTenant(sess.Username, tenantCode) {
				jsonError(w, "No tiene acceso a esta empresa.", http.StatusForbidden)
				return
			}
		}

		// Rutas entre empresas solo para master
		if isCrossTenantRoute(r.URL.Path) {
			jsonError(w, "cross-company access denied", http.StatusForbidden)
			return
		}

		// Rutas de master solo para master
		if isMasterRoute(r.URL.Path) {
			jsonError(w, "super admin access required", http.StatusForbidden)
			return
		}

		// ANTES había aquí un bloqueo duro: `isAdminRoute(path) && level !=
		// "admin"` → 403. Era inalcanzable para casi todo el mundo porque
		// sec_role.level es enum('master','manager','employee') —el nivel
		// "admin" no se puede guardar— y master ya salía antes con acceso
		// total: ningún gerente, cajero o vendedor podía facturar, comprar ni
		// tocar RRHH aunque su rol tuviera TODOS los permisos de la matriz. La
		// puerta real es la de abajo (membership de user_department + matriz
		// role_permission por módulo), que sí distingue nivel, módulo y acción.

		// El proxy de addons tiene autorización propia, basada en los permisos
		// declarados y aprobados por rol; no se mapea como un módulo core.
		if strings.HasPrefix(r.URL.Path, "/mod/") {
			next.ServeHTTP(w, r)
			return
		}

		// ADMIN DE EMPRESA: acceso total a SU empresa, sin pasar por el membership
		// de `user_department`.
		//
		// Las reglas del sistema (cabecera de este archivo) dicen que admin
		// "gestiona solo su empresa", y canAccessModule ya devuelve true para ese
		// nivel. Pero la puerta de arriba —el membership módulo a módulo— se
		// evaluaba ANTES e igual para todos, así que un admin recién creado (sin
		// filas en user_department) recibía 403 "No tiene permisos para este
		// módulo" en todo el ERP: el formulario de alta de usuario no asignaba
		// esos departamentos, y el resultado era un administrador que no podía
		// administrar nada. Delegar funciones por rol era, en la práctica,
		// imposible: todo lo que no fuera master quedaba en 403 o en empleado.
		//
		// El acceso a la empresa ya se comprobó arriba (membership de la empresa),
		// que es lo que limita a un admin a la suya.
		if level == "admin" {
			next.ServeHTTP(w, r)
			return
		}

		// Verificar módulo. DOS FUENTES, Y ES A PROPÓSITO.
		//
		//  1. La MATRIZ DE PERMISOS DEL ROL (`role_permission`, la relación
		//     sub_module.directory ↔ módulo ↔ CRUD) que se edita en Roles y
		//     Permisos (`/role/permissions/{id}`).
		//  2. La pertenencia DIRECTA del usuario al módulo (`user_department`),
		//     herencia del PHP, que sigue valiendo para las instalaciones donde ya
		//     estaba puesta.
		//
		// ANTES SOLO CONTABA LA SEGUNDA, y la primera era decorativa: se podía
		// marcar todo el CRUD de un módulo en la pantalla de Roles, guardar, y el
		// usuario seguía recibiendo 403 «No tiene permisos para este módulo» en
		// todo el ERP. Peor: ninguna pantalla del sistema escribe
		// `user_department` (se buscó en templates/ y static/js/: cero llamadas a
		// /permission/user/save), así que esa puerta no se podía abrir desde la
		// interfaz. Delegar un módulo a un gerente —o a un cajero— era imposible:
		// lo único que funcionaba era ser master (bypass de arriba) o admin
		// (bypass de arriba).
		//
		// Ahora alcanza con UNA de las dos, y quien decide la ACCIÓN (crear, leer,
		// editar, borrar) sigue siendo canAccessModule, que ya distingue por nivel.
		deptID, hasDept := moduleForRoute(r.URL.Path)
		if hasDept && deptID > 0 {
			action := actionFromMethod(r.Method)
			// La matriz CRUD es la autorización operativa. user_department solo
			// identifica áreas de trabajo (p. ej. el POS); no puede convertir una
			// pertenencia en permiso para ver o modificar todos los datos del módulo.
			if !canAccessModule(s.roleDBs(r), sess.UserID, level, deptID, action) {
				jsonError(w, "No tiene permisos para este módulo.", http.StatusForbidden)
				return
			}
		}

		// Módulos que comparten module.id con otro: el módulo no alcanza para
		// distinguirlos, así que manda el permiso de la entrada de menú.
		action := actionFromMethod(r.Method)
		for prefix, dirs := range routeDirectory {
			if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
				continue
			}
			concedido := false
			for _, d := range strings.Split(dirs, "|") {
				if d != "" && s.hasPermission(r, d, action) {
					concedido = true
					break
				}
			}
			if !concedido {
				jsonError(w, "No tiene permisos para este módulo.", http.StatusForbidden)
				return
			}
			break
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) registerPermissionRoutes() {
	s.mux.HandleFunc("GET /permission/user/{id}", s.permissionUserGet)
	s.mux.HandleFunc("POST /permission/user/save", s.permissionUserSave)
	s.mux.HandleFunc("GET /permission/role/list", s.permissionRoleList)
	s.mux.HandleFunc("POST /permission/role/save", s.permissionRoleSave)
	s.mux.HandleFunc("POST /permission/seed", s.permissionSeed)
}

func (s *Server) permissionUserGet(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimPrefix(r.URL.Path, "/permission/user/")
	if userID == "" {
		jsonError(w, "missing user id", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)

	level, _ := loadUserRole(db, userID)
	if level == "" {
		level = "employee"
	}
	depts, _ := loadUserDepartments(db, userID)
	tenants, _ := loadUserTenants(db, userID)

	var roleID int
	_ = db.QueryRow(`SELECT roleid FROM sec_userrole WHERE user_id = ? LIMIT 1`, userID).Scan(&roleID)

	writeJSON(w, map[string]interface{}{
		"user_id":     userID,
		"role_id":     roleID,
		"level":       level,
		"departments": depts,
		"tenants":     tenants,
	})
}

func (s *Server) permissionUserSave(w http.ResponseWriter, r *http.Request) {
	userID := r.FormValue("user_id")
	roleID := r.FormValue("roleID")
	departments := r.FormValue("departments")
	tenants := r.FormValue("tenants")

	if userID == "" || roleID == "" {
		jsonError(w, "missing fields", http.StatusBadRequest)
		return
	}

	// Seguridad: un usuario no puede modificar sus propios permisos
	sess := s.sessionFromRequest(r)
	if sess != nil && sess.UserID == userID {
		jsonError(w, "cannot modify own permissions", http.StatusForbidden)
		return
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.Exec(`DELETE FROM sec_userrole WHERE user_id = ?`, userID)
	_, err = tx.Exec(`INSERT INTO sec_userrole (user_id, roleid, createby, createdate) VALUES (?, ?, ?, NOW())`,
		userID, roleID, sess.UserID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, _ = tx.Exec(`DELETE FROM user_department WHERE user_id = ?`, userID)
	if departments != "" {
		for _, d := range strings.Split(departments, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				deptID, _ := strconv.Atoi(d)
				if deptID > 0 {
					_, _ = tx.Exec(`INSERT IGNORE INTO user_department (user_id, department_id) VALUES (?, ?)`, userID, deptID)
				}
			}
		}
	}

	_, _ = tx.Exec(`DELETE FROM user_company_access WHERE user_id = ?`, userID)
	if tenants != "" {
		for _, t := range strings.Split(tenants, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				_, _ = tx.Exec(`INSERT IGNORE INTO user_company_access (user_id, tenant_code, role_id) VALUES (?, ?, ?)`, userID, t, roleID)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Se cambió la asignación de rol de este usuario: lo que SE LE MUESTRA en
	// su menú y sus botones se recalcula en el acto. Lo que NO se toca aquí es
	// la puerta de ejecución, que no está cacheada.
	invalidarCachePermisos()

	writeJSON(w, map[string]interface{}{"saved": true})
}

func (s *Server) permissionRoleList(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	rows, err := db.Query(`SELECT id, type, level FROM sec_role ORDER BY id`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var roles []map[string]interface{}
	for rows.Next() {
		var id int
		var roleType, level string
		_ = rows.Scan(&id, &roleType, &level)
		roles = append(roles, map[string]interface{}{"id": id, "type": roleType, "level": level})
	}
	writeJSON(w, roles)
}

func (s *Server) permissionRoleSave(w http.ResponseWriter, r *http.Request) {
	roleID := r.FormValue("id")
	roleType := r.FormValue("type")
	level := r.FormValue("level")

	validLevels := map[string]bool{"master": true, "admin": true, "manager": true, "employee": true}
	if !validLevels[level] {
		jsonError(w, "invalid level", http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	if roleID == "" {
		_, _ = db.Exec(`INSERT INTO sec_role (type, level) VALUES (?, ?)`, roleType, level)
	} else {
		_, _ = db.Exec(`UPDATE sec_role SET type = ?, level = ? WHERE id = ?`, roleType, level, roleID)
	}
	invalidarCachePermisos()
	writeJSON(w, map[string]interface{}{"saved": true})
}

// permissionSeed crea los roles base del sistema
func (s *Server) permissionSeed(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Roles base
	roles := []struct {
		id    int
		rtype string
		level string
	}{
		{1, "super_admin", "master"},
		{2, "admin_empresa", "admin"},
		{3, "gerente", "manager"},
		{4, "supervisor", "manager"},
		{5, "cajero", "employee"},
		{6, "facturacion", "employee"},
		{7, "compras", "employee"},
		{8, "rrhh", "employee"},
		{9, "agente_ia", "employee"},
	}

	for _, role := range roles {
		_, _ = tx.Exec(`INSERT IGNORE INTO sec_role (id, type, level) VALUES (?, ?, ?)`, role.id, role.rtype, role.level)
	}

	_ = tx.Commit()
	// Se acaban de crear roles: invalida lo que se muestra mientras se
	// reescribe la matriz.
	invalidarCachePermisos()
	writeJSON(w, map[string]interface{}{"seeded": true})
}

func (s *Server) sessionUserID(r *http.Request) string {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		return "system"
	}
	return sess.UserID
}
