package web

// tenant_request.go — La empresa de cada petición.
//
// El problema que resuelve este archivo es el de fondo del modo multi-empresa.
// Durante mucho tiempo el servidor guardó UN puntero a la base de datos
// (s.db) y lo cambiaba cuando alguien elegía empresa. Con una persona
// trabajando, eso no se notaba. Con dos personas en empresas distintas a la
// vez, una petición podía acabar ejecutándose contra la base de la otra: no
// era un problema de permisos, era que dos empresas se veían los datos la una
// de la otra.
//
// Aquí la empresa se resuelve UNA VEZ POR PETICIÓN, en el middleware, desde la
// sesión, y se guarda en el contexto. A partir de ahí los handlers piden
// s.dbFor(r) y s.settingsFor(r), que devuelven lo de SU petición y no lo que
// alguien dejó puesto hace cinco minutos.
//
// El contexto se usa en vez de un mapa con clave por request ID porque no hay
// que registrar nada, no hay que liberar nada y no puede quedar una petición
// viva apuntando a un handle que se cerró.

import (
	"context"
	"database/sql"
	"net/http"
)

// defaultTenantCode es la empresa implícita de una instalación de una sola
// empresa. Vacío en la sesión significa "no has elegido empresa todavía", que es
// el estado entre autenticarse y pasar por el selector.
const defaultTenantCode = "default"

// scope es la empresa de una petición: su código, su base y sus ajustes.
type scope struct {
	tenant   string
	db       *sql.DB
	settings *AppSettings

	// sess es la identidad de una petición INTERNA (la capa de IA, el
	// semillero, las llamadas entre handlers). Una petición así no trae cookie
	// —no hay navegador— y por tanto sessionFromRequest no tenía de dónde
	// sacar quién actúa: los handlers internos se ejecutaban como el usuario
	// "1" y la comprobación de permisos del rol no se podía hacer.
	//
	// Vacío significa "no hay identidad interna": la petición viene del
	// navegador y la sesión se lee de su cookie, como siempre.
	sess *SessionData
}

type scopeKey struct{}

// withScope returns a copy of r carrying the request scope.
func withScope(r *http.Request, sc *scope) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), scopeKey{}, sc))
}

// scopeOf devuelve el ámbito de la petición, o nil si no lo tiene.
func scopeOf(r *http.Request) *scope {
	if r == nil {
		return nil
	}
	sc, _ := r.Context().Value(scopeKey{}).(*scope)
	return sc
}

// tenantOf devuelve el código de empresa al que pertenece esta petición.
//
// Si la petición no trae ámbito (llamadas internas, pruebas) se cae a la empresa
// activa, que es lo único sensato: quien llama fuera del ciclo HTTP no tiene
// sesión de la que fiarse.
func (s *Server) tenantOf(r *http.Request) string {
	if sc := scopeOf(r); sc != nil && sc.tenant != "" {
		return sc.tenant
	}
	return s.activeTenantCode()
}

// activeTenantCode devuelve la empresa activa del proceso.
func (s *Server) activeTenantCode() string {
	s.tenantMu.RLock()
	defer s.tenantMu.RUnlock()
	if s.activeTenant == "" {
		return defaultTenantCode
	}
	return s.activeTenant
}

// dbFor devuelve la base de datos de la empresa de esta petición.
//
// Es el sustituto de s.db. Cuando no hay ámbito (arranque, tareas en segundo
// plano, pruebas) usa la empresa activa, que es lo que se quería decir entonces.
// dbFor returns the tenant database for this request.
func (s *Server) dbFor(r *http.Request) *sql.DB {
	if sc := scopeOf(r); sc != nil && sc.db != nil {
		return sc.db
	}
	return s.activeDB()
}

// authDB SIEMPRE devuelve la base de control (erpgo), donde viven
// users, user_login, sec_role, role_permission y sub_module. El código de
// autenticación y gestión de usuarios NUNCA debe tocar la BD del tenant:
// si un empleado cambia de empresa y su login desaparece de la vista, el
// error es que se consultó la BD equivocada.
func (s *Server) authDB(r *http.Request) *sql.DB {
	if ctl, err := s.controlStore(); err == nil {
		return ctl
	}
	return s.dbFor(r)
}

// requestForSession construye una petición sintética que ya lleva el ámbito de
// una sesión.
//
// La capa de IA llama a ayudantes de datos que están escritos en términos de
// *http.Request (los de auditoría y de asientos contables, por ejemplo). Cambiar
// sus 71 puntos de llamada para que no dependan de la petición sería un refactor
// grande sin ganancia: la petición no se usa para nada del HTTP allí, solo para
// saber qué empresa es.
//
// Esta función deja que la capa de IA reuse EXACTAMENTE el mismo camino de
// resolución por empresa, con un solo objeto en vez de dos. No se envía nada,
// no tiene contexto de red y no debe usarse para leer cabeceras ni cookies:
// para eso está la sesión, que es lo único que realmente transporta.
func (s *Server) requestForSession(sess *SessionData) *http.Request {
	tenant := s.tenantOfSession(sess)
	r := withScope(&http.Request{}, &scope{
		tenant:   tenant,
		db:       s.dbForTenant(tenant),
		settings: s.settingsOfTenant(tenant),
		sess:     sess,
	})
	return r
}

// requestForTenant construye una petición sintética con el ámbito de una
// empresa concreta cuando NI siquiera hay sesión: es el caso de la entrega
// automática de facturas (diEntregarAutomatica), que corre desde el runner en
// el momento en que la factura queda emitida. Ahí no hay navegador, no hay
// cookie y tampoco hay agente: lo único que hace falta es saber a qué empresa
// pertenece la factura, para que la auditoría y los ajustes tengan de dónde
// sacar la compañía.
//
// Mismo contrato que requestForSession: no se envía nada, no hay contexto de
// red y no debe usarse para leer cabeceras ni cookies.
func (s *Server) requestForTenant(tenant string) *http.Request {
	return withScope(&http.Request{}, &scope{
		tenant:   tenant,
		db:       s.dbForTenant(tenant),
		settings: s.settingsOfTenant(tenant),
	})
}

// dbForSession devuelve la base de la empresa a la que pertenece una sesión.
//
// Es la vía de la IA: las herramientas y las tareas encoladas se ejecutan sin
// petición HTTP —a veces minutos después de que el usuario hizo clic—, así que
// no hay contexto del que sacar la empresa. Lo que sí viaja es la sesión del
// agente, que lleva su TenantCode. Sin esto, una tarea encolada de la empresa A
// acabaría ejecutándose contra la base que estuviera activa en ese momento.
func (s *Server) dbForSession(sess *SessionData) *sql.DB {
	return s.dbForTenant(s.tenantOfSession(sess))
}

// settingsForSession hace lo mismo con los ajustes de la empresa.
func (s *Server) settingsForSession(sess *SessionData) *AppSettings {
	return s.settingsOfTenant(s.tenantOfSession(sess))
}

// tenantOfSession devuelve la empresa de una sesión, o la activa si la sesión
// no trae empresa (todavía no se ha elegido en el selector).
func (s *Server) tenantOfSession(sess *SessionData) string {
	if sess != nil && sess.TenantCode != "" {
		return sess.TenantCode
	}
	return s.activeTenantCode()
}

// dbForTenant devuelve la base de una empresa concreta, o nil si esa empresa
// no está disponible.
//
// Para la empresa ACTIVA se devuelve el handle activo sin más. No es una vía
// corta: es lo que mantiene el servidor de pruebas y cualquier montaje manual
// donde se abre una base y se asigna a s.db sin pasar por el catálogo de
// empresas. Para el resto de empresas se abre (o se reutiliza) su archivo.
//
// FAIL-CLOSED (D-7/S-9, auditoría 2026-11-22): si la empresa pedida no abre,
// se devuelve nil —los llamadores responden 503—, NUNCA la base de la
// empresa activa. La versión anterior caía a activeDB(), que en una
// instalación multiempresa significaba: B pide datos, la base de B está
// caída, y la petición se ejecuta contra los datos de A leyendo Y
// ESCRIBIENDO en la empresa equivocada en silencio. Un 503 es recuperable;
// una factura en la empresa ajena no.
//
// El par (handle, código) activos se lee bajo UN solo candado (snapshot
// abajo): leerlos por separado dejaba una ventana en la que un cambio de
// empresa entre las dos lecturas devolvía el handle de una empresa bajo el
// código de otra.
func (s *Server) dbForTenant(tenant string) *sql.DB {
	if db, code := s.snapshotActivo(); db != nil && (tenant == "" || tenant == code) {
		return db
	}
	if tenant == "" {
		_, code := s.snapshotActivo()
		tenant = code
	}
	if db, err := s.tenantDB(tenant); err == nil {
		return db
	}
	// Ni activa ni abriible: nil. El llamador responde 503 y el usuario
	// reintenta; nadie escribe en una empresa que no pidió.
	return nil
}

// snapshotActivo lee el handle y el código activos bajo un único RLock:
// las dos lecturas son del MISMO instante o la comparación
// «tenant == empresa activa» puede mentir en la ventana entre ambas.
func (s *Server) snapshotActivo() (*sql.DB, string) {
	s.tenantMu.RLock()
	defer s.tenantMu.RUnlock()
	code := s.activeTenant
	if code == "" {
		code = defaultTenantCode
	}
	return s.db, code
}

// activeDB devuelve el handle de la empresa activa. Solo para lo que no está
// atado a una petición: arranque, tareas en segundo plano y pruebas. Un handler
// que lo use está leyendo la empresa equivocada en cuanto haya dos personas
// trabajando; por eso existe el test que prohíbe s.db dentro de los handlers.
func (s *Server) activeDB() *sql.DB {
	s.tenantMu.RLock()
	defer s.tenantMu.RUnlock()
	return s.db
}

// settingsFor devuelve los ajustes (razón social, moneda, logo) de la empresa de
// esta petición.
//
// También hay que separarlos por empresa, y por la misma razón: la cabecera de
// cada página se armaba con una única caché global que se recargaba al cambiar
// de empresa. Con dos empresas abiertas a la vez, todas las páginas habrían
// mostrado el nombre y la moneda de la última a la que alguien cambió.
func (s *Server) settingsFor(r *http.Request) *AppSettings {
	if sc := scopeOf(r); sc != nil && sc.settings != nil {
		return sc.settings
	}
	// Sin ámbito (pruebas, tareas en segundo plano): los ajustes de la empresa
	// activa. OJO: aquí el receptor tiene que ser getSettings, no settingsFor.
	// Un barrido automático los cambiaron los dos y dejaron una función que se
	// llamaba a sí misma hasta reventar la pila.
	return s.getSettings()
}

// attachScope resuelve la empresa de la petición y guarda base y ajustes en el
// contexto. Se llama una vez por petición, antes de cualquier handler.
func (s *Server) attachScope(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	sess := s.sessionFromRequest(r)

	// Sin sesión (login, /api/login, instalación) se trabaja sobre la empresa
	// activa: es lo que hace falta para leer la configuración y validar al
	// usuario. Con sesión, la empresa es la que eligió esa persona.
	tenant := ""
	if sess != nil {
		tenant = sess.TenantCode
	}
	if tenant == "" {
		tenant = s.activeTenantCode()
	}

	db := s.dbForTenant(tenant)
	if db == nil {
		// Sin base no hay nada que servir. Se dice claro en vez de caer con un
		// panic en el primer handler.
		http.Error(w, "No se pudo abrir la base de la empresa "+tenant, http.StatusInternalServerError)
		return r, false
	}

	return withScope(r, &scope{tenant: tenant, db: db, settings: s.settingsOfTenant(tenant)}), true
}

// settingsOfTenant devuelve los ajustes de una empresa, cargándolos la primera
// vez. La caché es por empresa y no global: ver settingsFor.
//
// D-7 (auditoría 2026-11-22): si la base de la empresa NO abre, se devuelve
// una configuración neutra y no se cachea nada. La versión anterior caía a
// tenantDBMust —que usa la empresa activa— y guardaba el resultado bajo la
// clave de la empresa pedida: en cuanto B se recuperaba, la cabecera seguía
// mostrando la razón social y la moneda de A hasta el reinicio. «Un dato
// cosmético» dicho en el comentario viejo; una cabecera con la empresa
// equivocada es precisamente el bug que motivó la caché por empresa.
func (s *Server) settingsOfTenant(tenant string) *AppSettings {
	s.settingsMu.RLock()
	if st, ok := s.settingsByTenant[tenant]; ok {
		s.settingsMu.RUnlock()
		return st
	}
	s.settingsMu.RUnlock()

	db, err := s.tenantDB(tenant)
	if err != nil {
		return &AppSettings{Web: WebSetting{Currency: "$"}}
	}
	st := s.loadSettings(db)

	s.settingsMu.Lock()
	if s.settingsByTenant == nil {
		s.settingsByTenant = map[string]*AppSettings{}
	}
	s.settingsByTenant[tenant] = st
	// La última cargada queda además como `settings`, que es lo que leen las
	// llamadas sin petición (arranque y tareas en segundo plano).
	s.settings = st
	s.settingsMu.Unlock()
	return st
}

// tenantDBMust es tenantDB para cuando no se puede fallar: si el handle no está
// abierto, usa la empresa activa. Solo se usa al leer ajustes, que es un dato
// cosmético y no debe impedir arrancar.
func (s *Server) tenantDBMust(tenant string) *sql.DB {
	if db, err := s.tenantDB(tenant); err == nil {
		return db
	}
	if db := s.activeDB(); db != nil {
		return db
	}
	return nil
}
