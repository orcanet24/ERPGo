package web

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"erpgo"
	"erpgo/configs"
	"erpgo/internal/queue"
	"erpgo/internal/upload"
)

// Server is the ERPGo web application.
type Server struct {
	config    *configs.Config
	db        *sql.DB
	mux       *http.ServeMux
	sessions  *SessionStore
	uploadMgr *upload.Manager

	// addons es la frontera de los addons externos (docs/PLAN-ADDONS.md).
	// nil = este arranque va sin addons (no existe la carpeta): todos los
	// métodos ligados a addons son nil-safe por esa razón.
	addons *addonHost

	// httpSrv es el servidor HTTP real que Start pone a escuchar. Se guarda
	// para poder pedirle un apagado ordenado (Shutdown) desde un servicio de
	// Windows o al recibir Ctrl+C, en lugar de matar el proceso.
	httpSrv *http.Server

	// App-level shared settings (currency, company, etc.).
	//
	// settingsByTenant es la caché REAL: una entrada por empresa. Antes había un
	// único `settings` global que se recargaba al cambiar de empresa, así que con
	// dos personas en empresas distintas la cabecera de todas las páginas
	// mostraba el nombre y la moneda de la última a la que alguien cambió.
	settingsByTenant map[string]*AppSettings
	// settings es la copia de la empresa activa. La leen las llamadas sin
	// petición (arranque, tareas en segundo plano); los handlers usan
	// settingsFor(r), que va a la caché por empresa.
	settingsMu sync.RWMutex
	settings   *AppSettings

	// Auditoría por bloques: último período (YYYY-MM) ya visto al escribir.
	// Solo evita re-sellar bloques ya cerrados; la verdad sigue siendo la tabla
	// audit_log, así que perder este valor tras un reinicio es inofensivo
	// (sealClosedAuditBlocks vuelve a barrer los períodos cerrados sin sello).
	// Guardado por auditChainMu.
	auditLastPeriod string

	// ---- Multi-empresa ----
	// db es el handle de la empresa ACTIVA. En PyME es siempre la misma; en
	// Enterprise cambia cuando el usuario elige otra empresa desde el panel.
	// tenantMu protege el intercambio: s.db se lee desde todos los handlers,
	// así que cambiarlo es un swap atómico bajo write lock.
	tenantMu     sync.RWMutex
	activeTenant string // código de la empresa activa; "default" en PyME

	// tenantDBs cachea los handles SQLite abiertos por empresa, para no
	// reabrir el mismo archivo al cambiar ida y vuelta (y no perder el pool).
	tenantDBs map[string]*sql.DB

	// walVigilas guarda la función que para el vigilante del WAL de cada
	// base abierta, en paralelo a tenantDBs. Sin esto, al cerrar quedaría
	// una goroutine consultando un handle ya cerrado, que es justo el tipo
	// de error que aparece en el apagado y en ningún otro sitio.
	//
	// Su propio candado y no los de tenantMu/controlMu: quien lo arranca
	// llama con esos candados puestos, y usar el mismo candado aquí sería
	// un abrazo mortal entre dos goroutines que ninguno de los dos ve.
	walMu      sync.Mutex
	walVigilas map[string]func()

	// controlDB es el catálogo de empresas (erpgo.db), abierto bajo demanda.
	controlMu sync.Mutex
	controlDB *sql.DB
	// controlListo dice si ya se intentó aplicar el esquema completo a la base de
	// control. Se intenta UNA vez por proceso: el registro de migraciones evita
	// repetir sentencias, pero leer y sumar los archivos en cada petición no es
	// gratis (y reintentar un fallo sería peor que el fallo).
	controlListo bool

	// queue es el motor de tareas en segundo plano (IA, reportes, cierres).
	// Vive sobre la BD de control con pool fijo, backpressure y spill a disco:
	// el requisito es que la app no se quede sin memoria con muchas tareas.
	queue *queue.Queue

	// backupStop apaga el ciclo de respaldos automáticos al cerrar el ERP.
	// Vale nil cuando los respaldos automáticos están deshabilitados.
	backupStop chan struct{}

	// diStop apaga el runner de facturación digital al cerrar el ERP.
	diStop chan struct{}

	// msgStop apaga el despachador de mensajería al cerrar el ERP. La cola
	// queda en la base; lo que no se envía hoy se envía cuando vuelva a
	// arrancar, no se pierde.
	msgStop chan struct{}
	// msgTyping activa la pausa de «escribiendo» antes de cada envío. Se deja
	// apagada en las pruebas: esperar segundos no cambia ninguna decisión.
	msgTyping bool
}

// AppSettings caches per-request common template data sources.
type AppSettings struct {
	Company  CompanyInfo
	Web      WebSetting
	Currency string
}

// CompanyInfo mirrors company_information row.
type CompanyInfo struct {
	CompanyName string
	Email       string
	Address     string
	Mobile      string
	Website     string
	VatNo       string
	CrNo        string
	// UploadPath es el directorio de esta empresa dentro de uploads/
	// (EMP+hex). Se cachea con el resto de los ajustes para que la guarda
	// de pertenencia de /uploads/ (S-7) no consulte la base en cada
	// petición de imágenes.
	UploadPath string
}

// WebSetting mirrors web_setting row (subset used everywhere).
type WebSetting struct {
	Logo          string
	InvoiceLogo   string
	Favicon       string
	Currency      string
	Timezone      string
	FooterText    string
	Language      string
	Rtr           string
	IsQr          int
	IsAutoapprove int
	DiscountType  int
}

// esPeticionXHR reconoce la llamada de librería (jQuery pone
// X-Requested-With en toda petición misma-origen) o un Accept de JSON. Con
// eso, la respuesta de «sesión vencida» puede ser 401 en vez del 302 que el
// fetch sigue hasta el HTML del login.
func esPeticionXHR(r *http.Request) bool {
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// safeReturnTo filtra el parámetro de retorno del login: solo rutas
// relativas del propio sitio (nada de «//evil.com» ni URLs absolutas), y nada
// de páginas que pierden el sentido al reentrar (el propio login, el logout).
func safeReturnTo(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return ""
	}
	for _, veto := range []string{"/login", "/logout", "/setup/", "/static/", "/uploads", "/favicon"} {
		if strings.HasPrefix(p, veto) {
			return ""
		}
	}
	return p
}

// mimeTypesByExt forces canonical web MIME types. On Windows the registry
// maps .js → application/x-js and .css → application/x-css; with
// X-Content-Type-Options: nosniff set by our middleware, Chrome then refuses
// to execute/apply those files — which is what makes every page look broken.
// Always keep this table in sync with the extensions used under static/.
var mimeTypesByExt = map[string]string{
	".css":   "text/css; charset=utf-8",
	".js":    "application/javascript; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".svg":   "image/svg+xml",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",
	".map":   "application/json",
	".txt":   "text/plain; charset=utf-8",
	".pdf":   "application/pdf",
	".csv":   "text/csv; charset=utf-8",
	".zip":   "application/zip",
	".sql":   "application/sql",
}

// forceContentType sets Content-Type from the extension of path before the
// file server runs (net/http only guesses when the header is still empty).
func forceContentType(w http.ResponseWriter, path string) {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return
	}
	if ct, ok := mimeTypesByExt[strings.ToLower(path[i:])]; ok {
		w.Header().Set("Content-Type", ct)
	}
}

// NewServer builds the server: templates, static files, routes.
func NewServer(cfg *configs.Config, db *sql.DB) *Server {
	s := &Server{
		config:   cfg,
		db:       db,
		mux:      http.NewServeMux(),
		sessions: NewSessionStore(),
		activeTenant: func() string {
			if cfg != nil && strings.TrimSpace(cfg.DBMasterTenantCode) != "" {
				return strings.TrimSpace(cfg.DBMasterTenantCode)
			}
			return defaultTenantCode
		}(),
		tenantDBs:  map[string]*sql.DB{},
		walVigilas: map[string]func(){},
	}
	s.sessions.StartJanitor()

	// Qué X-Forwarded-For se cree (R8, ver proxy_confianza.go). Se fija una
	// vez por proceso: el valor viene del .env y no cambia en la vida del
	// servidor.
	if cfg != nil {
		configurarProxyConfianza(cfg.ProxyTrustedIPs)
	}

	// El handle que llega del arranque es la BASE DE CONTROL: catálogo de
	// empresas, miembros y sesiones. Se fija ANTES de cualquier consulta para
	// que controlStore lo reutilice en vez de abrir una segunda conexión.
	s.controlDB = db

	// Sesiones persistentes: se conectan a la BD de control, que a su vez se
	// abre de forma perezosa. La sesión vive allí porque es la identidad de
	// QUIÉN está conectado, y esa pregunta se responde antes de saber en qué
	// empresa está (ver controlSchema en tenants.go).
	//
	// Si la BD de control no abre, el almacén degrada a memoria: se pierde la
	// persistencia pero la aplicación arranca igual. Es preferible a negarse a
	// levantar el ERP por un problema de disco.
	if control, err := s.controlStore(); err == nil {
		s.sessions.SetDB(control)
	} else {
		log.Printf("sesiones sin persistencia (BD de control no disponible): %v", err)
	}

	// La empresa maestra es la primera y comparte DB_NAME con el catálogo.
	// Las otras empresas se resuelven mediante tenants y sus bases propias.

	// Al arrancar, se asegura el registro de la maestra contra DB_NAME; no se abre
	// ni se crea otra base `erpgo_default` para el tenant inicial.
	if err := PrepararEmpresaMaestra(cfg, db); err != nil {
		log.Printf("AVISO: no se pudo preparar el tenant maestro: %v", err)
	}
	s.EnsureDefaultTenant()

	// El super administrador es UNA sola identidad y su rol no se reparte desde
	// el panel (ver super_admin.go). Se comprueba en cada arranque porque el
	// estado pudo quedar mal con datos heredados: la pantalla de roles permitía
	// asignarlo y la de usuarios ofrecía ese rol en el desplegable. Si no hay
	// ninguno, aquí NO se crea —una clave que nadie eligió volvería a repartir la
	// llave del sistema—: se avisa y se restablece con el binario en modo --setup.
	if err := s.asegurarSuperAdminUnico(); err != nil {
		log.Printf("AVISO: no se pudo comprobar el super administrador: %v", err)
	}

	// Pertenencia sin fila en tenant_users: la dejó el alta anterior, que escribía
	// solo user_company_access. Sin esto, esos usuarios no pueden entrar a ninguna
	// empresa real (el login valida con tenant_users) y parecen «de la demo».
	if ctl, err := s.controlStore(); err == nil {
		if n, err := repararPertenenciasHuerfanas(ctl); err != nil {
			log.Printf("AVISO: no se pudieron reparar las pertenencias de usuarios: %v", err)
		} else if n > 0 {
			log.Printf("pertenencias reparadas: %d usuario(s) recuperaron su empresa en tenant_users", n)
		}
	}
	// La empresa de práctica (DEMO) se auto-provisiona y queda SIEMPRE
	// disponible para cualquier usuario. El admin puede desactivarla con
	// POST /guia/demo-marcar (code=demo, demo=0).
	s.ensureDemoTenant()

	// Motor de tareas en segundo plano (cola con pool fijo y spill a disco).
	// Un fallo al abrirlo no impide arrancar la app: solo desactiva el trabajo
	// asíncrono, y el resto del ERP sigue operando.
	if err := s.startQueue(); err != nil {
		log.Printf("cola de tareas no disponible: %v", err)
	}

	// Orquestador de escrituras: la puerta única por la que deben pasar las
	// escrituras, con correlativo propio, idempotencia y rastro obligatorio en
	// audit_log. Se levanta para la empresa con la que arrancó el servidor; las
	// demás abren el suyo la primera vez que escriben.
	//
	// Un fallo aquí NO impide arrancar, por el mismo criterio que la cola: es
	// preferible operar avisando en el log a negarse a levantar el ERP. Lo que
	// no se hace es fingir que está activo.
	if _, err := s.startOrchestrator(s.activeTenant); err != nil {
		log.Printf("orquestador de escrituras no disponible: %v", err)
	}

	// Respaldo automático diario (comprimido y verificado por el binario
	// mismo): BACKUP_ENABLED/BACKUP_INTERVAL_HOURS/BACKUP_DIR en .env.
	s.startBackupScheduler()

	// Runner de facturación digital: emite las ventas sincronizadas y offline
	// al proveedor que la empresa tenga activo (digital_invoice_config).
	s.startDigitalInvoiceRunner()

	// Static files — served from the disk copy when present (live edits in
	// development) and from the embedded filesystem otherwise (single binary).
	staticRoot, err := fs.Sub(erpgo.Static(), "static")
	if err != nil {
		log.Fatalf("embedded static files: %v", err)
	}
	staticHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SECURITY: /static/ is public (no session required). /static/data/
		// holds legacy PHP-era runtime data (MySQL dumps, invoice PDFs, CSV
		// exports) that must never be served; residual .php sources neither.
		// The Go app writes backups to data/backup (root) and serves them via
		// the authenticated /data/backup/{name} route.
		if strings.HasPrefix(r.URL.Path, "/static/data/") ||
			strings.HasSuffix(strings.ToLower(r.URL.Path), ".php") {
			http.NotFound(w, r)
			return
		}
		forceContentType(w, r.URL.Path)
		// REVALIDAR SIEMPRE, en vez de cachear un año.
		//
		// Lo que decía el comentario anterior —«los archivos viven dentro del
		// binario, así que cambiar el binario = cambiar los archivos»— es cierto
		// para el DISCO y falso para el NAVEGADOR: el navegador cachea por URL, y
		// la URL de /static/css/erpgow-tema.css es la misma en la versión 1 y en la
		// 40. Con `max-age=31536000, immutable`, quien ya hubiera abierto el ERP una
		// vez se queda con el CSS y el JS viejos HASTA UN AÑO: los arreglos de
		// interfaz —colores de los gráficos, temas, estilos— se instalan en el
		// servidor y no llegan a su pantalla, y desde fuera parece que no se hizo
		// nada. Le pasó a los colores de los gráficos de este mismo ERP.
		//
		// `no-cache` (mal nombre: NO significa «no guardes», significa «no uses lo
		// guardado sin preguntar») obliga a revalidar en cada carga. En instalación
		// con la carpeta static/ al lado del binario, el ETag responde 304 y no se
		// transfiere el cuerpo; con los estáticos embebidos —donde embed.FS no da
		// fecha de modificación y por tanto no hay ETag— se reenvía el archivo, que
		// son unas decenas de KB con gzip.
		//
		// RENDIMIENTO DE NAVEGACIÓN (2026-10-26): el estático ahora se cachede
		// DUOS vías. Las URLs versionadas (?v=<hash> que pone el render) reciben
		// `immutable` de un año: el navegador NO vuelve a pedir 45 archivos en cada
		// cambio de vista, que era la lentitud que se sentía por internet. Sin `?v=`
		// (rutas escritas a mano en desarrollo) se queda el `no-cache` de siempre,
		// que nunca sirve un asset caducado.
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		// ETag aún sirve para 304 en disco (desarrollo), pero para embebidos
		// el cache-control ya cubre. Lo mantenemos por compatibilidad.
		setStaticETag(w, staticRoot, r.URL.Path)
		http.StripPrefix("/static/", http.FileServer(http.FS(staticRoot))).ServeHTTP(w, r)
	})
	s.mux.Handle("/static/", staticHandler)
	// uploads (user content) — protegido por auth + control de acceso
	s.mux.Handle("/uploads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Autenticación manual para uploads: devolver 401, no redirigir
		sess := s.sessionFromRequest(r)
		if sess == nil {
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}

		// S-7 (auditoría 2026-11-22): pertenencia de empresa. Cada empresa
		// sube a su directorio EMP+hex bajo uploads/, y hasta aquí la única
		// defensa era que el nombre era aleatorio: cualquier usuario con el
		// enlace —en un correo, en un PDF de factura— abría el documento de
		// OTRA empresa con su propia sesión. El primer segmento del path
		// debe ser el directorio de la empresa de la petición; si no lo es,
		// 404 (no 403: un 403 confirmado enseñaría qué rutas existen).
		// Master puede cruzar: gestiona todas las empresas por definición.
		rel := strings.TrimPrefix(r.URL.Path, "/uploads/")
		if primer := strings.SplitN(rel, "/", 2)[0]; primer != "" && primer != "personal" {
			st := s.settingsFor(r)
			if esperado := strings.TrimSpace(st.Company.UploadPath); esperado != "" && primer != esperado {
				nivel := ""
				if db := s.dbFor(r); db != nil {
					nivel = s.nivelDeSesion(r, db, sess)
				}
				if nivel != "master" {
					http.NotFound(w, r)
					return
				}
			}
		}

		// Bloquear directorio 'personal' para no-admins (master/admin)
		if strings.Contains(r.URL.Path, "/personal/") {
			db := s.dbFor(r)
			level := ""
			if db != nil {
				level = s.nivelDeSesion(r, db, sess)
			}
			if level != "master" && level != "admin" {
				http.Error(w, "No autorizado. Solo administradores.", http.StatusForbidden)
				return
			}
		}

		forceContentType(w, r.URL.Path)
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))).ServeHTTP(w, r)
	}))

	// Auth
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/login", s.handleAPILogin)
	s.mux.HandleFunc("/api/pos/auth/login", s.handlePOSLogin)
	s.mux.HandleFunc("/api/pos/auth/validate", s.handlePOSValidate)
	s.mux.HandleFunc("/api/pos/auth/logout", s.handlePOSLogout)
	s.mux.HandleFunc("/api/pos/auth/supervisor", s.handlePOSAutorizaSupervisor)

	// Dashboard
	s.mux.HandleFunc("/", s.handleDashboard)
	s.mux.HandleFunc("/home", s.handleDashboard)
	s.mux.HandleFunc("/dashboard/chart-data", s.handleDashboardChartData)

	// Users & roles (dashboard module)
	s.registerUserRoutes()
	// Vistas y plantillas por usuario (viven en la base de la empresa)
	s.registerViewPrefRoutes()
	// Customers
	s.registerCustomerRoutes()
	// Suppliers
	s.registerSupplierRoutes()
	// Products, categories, units
	s.registerProductRoutes()
	s.registerEntradaRapidaRoutes()
	// Purchases
	s.registerPurchaseRoutes()
	// Bandeja de recepción de documentos (XML/PDF/foto de facturas de proveedor)
	s.registerPurchaseInboxRoutes()
	// Invoices + POS
	s.registerInvoiceRoutes()
	// Accounting (COA, vouchers, ledger, reports)
	s.registerAccountRoutes()
	s.registerCOACatalogRoutes()
	s.registerConasevRoutes()
	// RRHH (empleados, asistencia, nómina)
	s.registerHRRoutes()
	// Nómina: endpoints POST de nómina (run/save, run/list, etc.)
	s.registerPayrollRoutes()
	// CRM (contactos, embudo comercial, actividades)
	s.registerCRMRoutes()
	// Remarketing (campañas segmentadas, plantillas, envío multicanal)
	s.registerRemarketingRoutes()
	// Cierre de períodos contables
	s.registerPeriodRoutes()
	// Motor de informes por plantilla (con parámetros y exportación)
	s.registerReportEngineRoutes()
	// Reports
	s.registerReportRoutes()
	// Settings
	s.registerSettingRoutes() // OCR Addon (configurable: ENABLE_OCR=0|1 para el superusuario)
	// Cuentas por papel contable: ata la separación al plan que use la empresa.
	s.registerCuentasRolRoutes()
	s.registerOCRRoutes() // YummyProxy2 (pasarela de envíos): cliente del gateway de Yummy y
	// receptor de sus webhooks (configurable con YUMMY_ENABLED=1)
	s.registerYummyRoutes()
	// SpidiAgent (pagos SPIDI): sesiones de cobro por factura y consulta
	// de estado (configurable con SPIDI_ENABLED=1)
	s.registerSpidiRoutes()
	// Multi-empresa: selector de empresa y aprovisionamiento
	s.registerTenantRoutes()
	s.registerTenantSetupRoutes()
	// Database (SQLite: respaldo e integridad)
	s.registerDatabaseSettingsRoutes()
	// Service module
	s.registerServiceRoutes()
	// Returns
	s.registerReturnRoutes()
	// Quotation
	s.registerQuotationRoutes()
	// Bank
	s.registerBankRoutes()
	// Adjustment
	s.registerAdjustmentRoutes()
	s.registerAlmacenRoutes()
	s.registerConsumoInternoRoutes()
	// Voucher multi-line (extra)
	s.registerVoucherExtraRoutes()
	// Legal Books (Venezuelan statutory books)
	s.registerLegalBooksRoutes()
	// Kardex Art. 177 RISLR
	s.registerKardexRoutes()
	// Exchange Rate (BCV)
	s.registerExchangeRoutes()
	// Costos: método de costeo por producto, receta, rendimiento, hoja de horas,
	// desglose y cierre de período.
	s.registerCostRoutes()
	// Activos fijos y depreciación (Fase 2).
	s.registerAssetsRoutes()
	// Unidades de medida y conversiones (Fase 4).
	s.registerUomRoutes()
	// Órdenes de producción (Fase 6).
	s.registerProductionRoutes()
	// Informes e integración contable (Fase 7).
	s.registerCostReportRoutes()
	// Inventario avanzado: FEFO y caducidades, punto de pedido y conteo cíclico
	s.registerInventoryExpiryRoutes()
	s.registerReorderRoutes()
	s.registerCycleCountRoutes()
	// Tipo de cambio BCV/paralelo con diferencia cambiaria
	s.registerTasaRoutes()
	// Audit Log
	s.registerAuditRoutes()
	s.registerAIRoutes()
	// Agentes IA, tareas y plantillas de informes personalizados
	registerAIAgentRoutes(s)
	registerAITaskRoutes(s)
	registerAIReportRoutes(s)
	registerAIDepartmentRoutes(s)
	// El puente MCP (M-1): transporte JSON-RPC para IAs externas sobre el
	// catálogo gobernado de herramientas + gestión de tokens por agente.
	s.registerMCPRoutes()
	// (F11) ya no se publica la global agentDB: los helpers reciben la base
	// por parámetro/sesión, y la carrera entre workers de empresas distintas
	// desapareció con la variable.
	s.aiTaskRunner()
	// FISCAL Venezuela: notas de crédito + notas de débito + ajustes fiscales
	s.registerCreditNoteRoutes()
	s.registerDebitNoteRoutes()
	s.registerFiscalSettingsRoutes()
	s.registerDigitalInvoiceRoutes()
	// Documento fiscal digital (PDF/XML de la imprenta) y su entrega automática
	s.registerDigitalDocRoutes()
	// Cartera / Cuentas por Cobrar
	s.registerCarteraRoutes()
	s.registerPagarRoutes()
	// Alertas financieras (CxC / CxP) y sus canales de salida: van junto a los
	// módulos porque son su tablero, no un módulo aparte.
	s.registerFinanceAlertRoutes()
	s.registerNotifyChannelRoutes()
	s.registerPOSAPIRoutes()
	s.registerPOSAdminRoutes()
	s.registerPOSSyncRoutes()
	s.registerDeliveryOrderRoutes()
	s.registerPriceListRoutes()
	s.registerOrderRoutes()
	s.registerCustomerClassRoutes()
	// Gestor de Menús (drag & drop para reordenar y mover)
	s.registerMenuManagerRoutes()
	// GOti: instructor de la empresa DEMO (no responde en empresas de producción)
	s.registerGuiaRoutes()
	s.registerModuleRoutes()
	s.registerModuleCore()
	s.registerPermissionRoutes()

	// Frontera de addons externos: escaneo, registro y lanzamiento.
	// El mux solo gana la puerta estable /mod/ (una vez, antes de servir);
	// addonDispatchMiddleware resuelve en caliente el id y las rutas
	// declaradas de cualquier addon, incluido uno instalado después.
	s.addonsIniciar()
	s.registrarRutasAddon()
	// Los verbos de negocio de los módulos demo (fuera de la frontera: sus
	// handlers viven en addon_hola.go y addon_demo.go, como cualquier módulo
	// de negocio del ERP; la frontera solo valida contexto y despacha).
	s.addonDemoVerboRegistrar()
	s.addonModuloDemoVerbos()
	// El verbo del módulo Memo (fase A de docs/PLAN-MEMO.md).
	s.addonModuloMemoVerbos()

	return s
}

// registerAIAgentRoutes adapta el método a función para registrar el módulo
// de agentes IA desde NewServer (los handlers viven en ai_agents.go).
func registerAIAgentRoutes(s *Server) { s.registerAIAgentRoutes() }

// registerAITaskRoutes registra las rutas de tareas (ai_tasks.go).
func registerAITaskRoutes(s *Server) { s.registerAITaskRoutes() }

// registerAIReportRoutes registra las rutas de informes personalizados
// (ai_reports.go).
func registerAIReportRoutes(s *Server) { s.registerAIReportRoutes() }

// registerAIDepartmentRoutes registra las pantallas de comportamiento por
// departamento y del asiento de coste de IA (ai_department_handlers.go).
func registerAIDepartmentRoutes(s *Server) { s.registerAIDepartmentRoutes() }

// reloadSettings loads company + web settings from DB (cached).
//
// Es la vía para la empresa activa: la usan el arranque y el cambio de empresa.
// Los handlers usan settingsFor(r), que resuelve por la empresa de la petición.
func (s *Server) reloadSettings() {
	// El handle y el código activos se leen bajo UN solo RLock
	// (snapshotActivo): leerlos por separado permitía que un cambio de
	// empresa entre las dos lecturas cachease los ajustes de A bajo la clave
	// de B (D-7, auditoría 2026-11-22).
	db, code := s.snapshotActivo()
	st := s.loadSettings(db)

	s.settingsMu.Lock()
	if s.settingsByTenant == nil {
		s.settingsByTenant = map[string]*AppSettings{}
	}
	s.settingsByTenant[code] = st
	s.settings = st
	s.settingsMu.Unlock()
}

// loadSettings lee los ajustes de una empresa. Devuelve ajustes vacíos con la
// moneda por defecto si la base todavía no está sembrada, para que una
// instalación a medio hacer siga mostrando una página legible.
func (s *Server) loadSettings(db *sql.DB) *AppSettings {
	st := &AppSettings{Web: WebSetting{Currency: "$"}}
	if db == nil {
		return st
	}
	var c CompanyInfo
	var logo, invoiceLogo, favicon, cur, tz, footer, lang, rtr sql.NullString
	var isQr, isAuto, discType sql.NullInt64
	err := db.QueryRow(`SELECT company_name, email, address, mobile, website,
		IFNULL(vat_no,''), IFNULL(cr_no,''), IFNULL(upload_path,'') FROM company_information WHERE status=1 ORDER BY company_id LIMIT 1`).
		Scan(&c.CompanyName, &c.Email, &c.Address, &c.Mobile, &c.Website, &c.VatNo, &c.CrNo, &c.UploadPath)
	if err == nil {
		st.Company = c
	}
	// Image paths are normalized + verified on disk (imageAssetPath): missing
	// or legacy PHP-era files ("assets/...") become "" so templates fall back
	// to the default logo/favicon shipped under /static/.
	err = db.QueryRow(`SELECT logo, invoice_logo, favicon, currency, timezone, footer_text, language, rtr, is_qr, is_autoapprove_v, discount_type
		FROM web_setting WHERE setting_id=1`).
		Scan(&logo, &invoiceLogo, &favicon, &cur, &tz, &footer, &lang, &rtr, &isQr, &isAuto, &discType)
	if err == nil {
		st.Web = WebSetting{
			Logo:          imageAssetPath(logo.String),
			InvoiceLogo:   imageAssetPath(invoiceLogo.String),
			Favicon:       imageAssetPath(favicon.String),
			Currency:      cur.String,
			Timezone:      tz.String,
			FooterText:    footer.String,
			Language:      lang.String,
			Rtr:           rtr.String,
			IsQr:          int(isQr.Int64),
			IsAutoapprove: int(isAuto.Int64),
			DiscountType:  int(discType.Int64),
		}
	}
	if st.Web.Currency == "" {
		st.Web.Currency = "$"
	}
	st.Currency = st.Web.Currency
	return st
}

func (s *Server) getSettings() *AppSettings {
	s.settingsMu.RLock()
	st := s.settings
	s.settingsMu.RUnlock()
	if st == nil {
		// Nunca nil: veinte llamadores hacen st.Web.X sin comprobar, y un
		// arranque parcial (o un servidor de prueba sin company_settings)
		// los reventaba. El valor por defecto es el mismo que carga
		// loadSettings cuando la tabla está vacía.
		st = &AppSettings{Web: WebSetting{Currency: "$"}}
	}
	return st
}

// Start runs the HTTP server with graceful timeouts.
//
// Si TLS_CERT_FILE y TLS_KEY_FILE están definidos, sirve por https (necesario
// para exponer el ERPGo por internet, donde las cajas POS envían su API key y
// sus ventas). Sin ellos, sirve http plano: es lo normal en una despliegue
// local/LAN, donde no hace falta certificado. El criterio de exigir TLS al
// cliente POS vive en el terminal (config/netsec.go); aquí el servidor solo
// se adapta a lo que el operador configuró.
func (s *Server) Start() error {
	s.uploadMgr = upload.NewManager("uploads")

	addr := s.config.ServerHost + ":" + s.config.ServerPort
	baseHandler := s.cacheDefaultMiddleware(s.corsMiddleware(s.authMiddleware(s.permissionMiddleware(s.auditMiddleware(s.addonDispatchMiddleware(s.mux))))))
	// gzip comprime respuestas textuales antes de salir por la red.
	// gzip.BestSpeed: nivel 1, poca CPU, ahorro significativo de ancho de banda.
	// Solo se activa si el cliente envía Accept-Encoding: gzip.
	// El navegador sigue viendo el Content-Type original; Content-Encoding: gzip viaja aparte.
	var handler http.Handler = baseHandler
	if s.config.GZipEnabled {
		handler = gzipMiddleware(baseHandler)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB - prevenir headers gigantes (DoS)
		ReadHeaderTimeout: 10 * time.Second,
	}
	// Se guarda para que Shutdown() pueda drenar las peticiones en curso en
	// lugar de cortarlas en seco al parar el proceso (servicio de Windows).
	s.httpSrv = srv

	// HTTP/2 automático con TLS. Go lo habita por defecto si el servidor
	// soporta ALPN. Forzamos el protocolo para asegurarlo.
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		log.Printf("🌐 Servidor ERPGo escuchando por HTTPS en %s (TLS habilitado, HTTP/2 activo)", addr)
		return srv.ListenAndServeTLS(s.config.TLSCertFile, s.config.TLSKeyFile)
	}
	log.Printf("🌐 Servidor ERPGo escuchando por HTTP en %s", addr)
	return srv.ListenAndServe()
}

// Shutdown detiene de forma ordenada el servidor HTTP: deja de aceptar
// conexiones nuevas y espera a que terminen las peticiones en curso hasta
// agotar ctx. Es lo que usa el servicio de Windows al recibir Stop/Shutdown.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

// authMiddleware protects everything except public paths.
//
// Es también donde se resuelve la empresa de la petición: se hace lo primero,
// antes incluso de mirar la sesión, porque hasta el login necesita leer los
// ajustes de la empresa activa para pintar su cabecera.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		r, ok = s.attachScope(w, r)
		if !ok {
			return
		}

		// Lista única de rutas públicas: publicRoutes (permissions.go). ANTES
		// authMiddleware y permissionMiddleware mantenían cada una su lista, y
		// divergieron («/uploads» sin barra en una, «/uploads/» en la otra):
		// dos puertas con distinto candado para la misma casa.
		if esRutaPublica(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		sess := s.sessionFromRequest(r)
		if sess == nil {
			// XHR y APIs de datos reciben 401 JSON, no un 302 al HTML del
			// login (U-2, auditoría 2026-11-22): el fetch seguía la
			// redirección, le llegaba la página de acceso como si fuera
			// la respuesta, y el usuario veía «Error» pelado mientras
			// perdía lo escrito. Ahora el front detecta el 401, avisa
			// «su sesión venció» y deja el formulario intacto.
			if esPeticionXHR(r) || strings.HasPrefix(r.URL.Path, "/api/") {
				jsonError(w, "Su sesión venció. Vuelva a iniciar sesión; su trabajo sigue en pantalla.", http.StatusUnauthorized)
				return
			}
			if r.URL.Path == "/uploads/" || strings.HasPrefix(r.URL.Path, "/uploads/") {
				http.Error(w, "No autorizado", http.StatusUnauthorized)
				return
			}
			// Navegación normal: se recuerda el destino para volver ahí tras
			// el login (returnTo), en vez de dejar al usuario en el panel con
			// la pantalla a medio llenar perdida.
			target := "/login"
			if r.Method == http.MethodGet && r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/home") {
				destino := r.URL.Path
				if r.URL.RawQuery != "" {
					destino += "?" + r.URL.RawQuery
				}
				if next := safeReturnTo(destino); next != "" {
					target = "/login?next=" + url.QueryEscape(next)
				}
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		// El token se renueva en cada visita para que el JavaScript de la
		// aplicación lo tenga disponible sin tener que pedirlo por separado.
		if token := s.ensureCSRFToken(sess); token != "" {
			setCSRFCookie(w, token, r.TLS != nil)
		}
		s.enforceCSRF(w, r, next, sess)
	})
}

// corsMiddleware adds security headers (and CORS for /api/).
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// HSTS solo cuando se sirve por TLS: es la cabecera que impide que
		// un navegador vuelva a conectar por http y exponga la cookie de
		// sesión (S-5, auditoría 2026-11-22). Enviarla en http plano sería
		// peor que inútil: fijaría en el navegador del usuario la orden de
		// no volver a entrar por el único acceso que funciona en la tienda.
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// cacheDefaultMiddleware is the outermost handler: it stamps a conservative
// Cache-Control on every response BEFORE any handler writes the body, so a
// handler that stays silent (JSON APIs, redirects, error pages) is never
// stored by the browser. Handlers that need a different policy overwrite the
// value with Header().Set while the body is still unwritten (static, uploads,
// HTML rendering).
// cacheDefaultMiddleware es el handler más externo: aplica Cache-Control
// conservador a TODA respuesta ANTES de que el handler escriba el body.
// Así handlers silenciosos (JSON APIs, redirects, errores) nunca se cachean
// por el navegador. Handlers que necesitan otra política la sobreescriben
// con Header().Set antes de escribir (static, uploads, HTML render).
//
// El log de cada request SOLO se imprime en desarrollo. En producción llenaría
// logs sin aportar valor (el access log del proxy/nginx ya cubre eso).
func (s *Server) cacheDefaultMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		start := time.Now()
		next.ServeHTTP(w, r)
		elapsed := time.Since(start)

		// DIAGNÓSTICO DE LENTITUD (2026-10-26): en desarrollo se loguea TODO;
		// en producción sólo las rutas que tardan más de 500 ms — con eso el
		// dueño ve en el log del servicio qué vistas son lentas (y por qué
		// cambiar la arquitectura de lenguaje no sería el remedio: esto mide
		// render+BD por request, no la velocidad de Go).
		if s.config == nil || s.config.AppEnv == "development" {
			username := "anónimo"
			if sess := s.sessionFromRequest(r); sess != nil {
				username = sess.Username
			}
			fmt.Printf("[HTTP] ← %s %s → %v (usuario: %s)\n", r.Method, r.URL.Path, elapsed, username)
			return
		}
		if elapsed > 500*time.Millisecond {
			log.Printf("LENTO: %s %s → %v", r.Method, r.URL.Path, elapsed)
		}
	})
}
