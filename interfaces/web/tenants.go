package web

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"

	"erpgo/configs"
	"erpgo/infrastructure/persistence"
	"erpgo/internal/security"
)

// ---------- Registro de empresas (multi-tenant sobre MySQL) ----------
//
// DB_NAME es la base de la empresa maestra y, simultáneamente, contiene el
// catálogo de tenants e identidades compartidas. La primera empresa es la
// maestra (DB_MASTER_TENANT_CODE, por compatibilidad "default"); no se crea una
// segunda base para ella. Cada empresa adicional tiene su base erpgo_<code>.
// La estructura se mantiene igual en todas las bases y los datos de negocio se
// separan físicamente por base. El tenant activo vive en la sesión.
//
// El usuario solo puede cambiar a una empresa de la que es miembro: cambiar de
// empresa mediante una URL no debe eludir el catálogo de pertenencias.

// Tenant es una empresa registrada en la BD de control.
type Tenant struct {
	Code       string // slug: es también el nombre de la base MySQL
	LegalName  string
	RIF        string
	Status     int // 1 = activa, 0 = suspendida
	CreatedAt  string
	MemberName string // nombre del admin con acceso (solo informative)
	IsMember   bool   // el usuario en sesión pertenece a esta empresa
}

// tenantCodePattern valida el slug de una empresa. El slug se usa como nombre
// de base, así que se limita a caracteres seguros: evita path traversal
// (..), separadores y caracteres inválidos en MySQL.
var tenantCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,48}[a-z0-9]$`)

// slugifyTenant convierte un nombre en un slug válido y estable.
func slugifyTenant(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	s := b.String()
	if len(s) < 3 {
		s = "empresa"
	}
	return s
}

// tenantDBName devuelve el nombre de la BASE MySQL de una empresa.
//
// Antes esto devolvía la ruta de un archivo (`empresas/<code>.sqlite3`). Ahora
// es el nombre de una base (`erpgo_<code>`). El aislamiento por empresa sigue
// siendo el mismo —una base por empresa, nunca una columna— pero el almacén
// cambió.
//
// El nombre se sanea porque MySQL no admite acentos, eñes ni espacios en el
// nombre de una base, y los códigos de empresa sí los tienen ("piñata",
// "mi empresa"). El saneo es determinista: la misma empresa da siempre la
// misma base, que es justo la propiedad de la que depende no perder sus datos.
//
// Prefijo de por medio (DB_TENANT_PREFIX) está vacío en producción. No es
// decoración: sin él, un test que abre la empresa "default" escribe en la base
// REAL del desarrollador, y dos tests que usan el mismo nombre de empresa se
// pisan los datos. Ese fallo es silencioso —el test pasa y la base del usuario
// queda con filas de prueba— y por eso el aislamiento se pone en la función que
// decide el nombre, no en cada test.
func (s *Server) tenantDBName(code string) string {
	if s != nil && s.isMasterTenant(code) {
		return s.config.DBName
	}
	prefijo := ""
	if s != nil && s.config != nil {
		prefijo = strings.Trim(s.config.DBTenantPrefix, "_")
	}
	codigo := strings.Trim(code, "_")
	if prefijo != "" {
		codigo = prefijo + "_" + codigo
	}
	nombre, err := persistence.NombreBaseEmpresa(codigo)
	if err != nil {
		// Un código que no se puede convertir es un código que no debería
		// existir; se usa un nombre fijo y visible para que el problema
		// aparezca en el log y no como una base que se crea a la ligera.
		return "erpgo_invalida"
	}
	return nombre
}

// isMasterTenant identifica el código de empresa que usa directamente DB_NAME.
// La base maestra es simultáneamente el catálogo de control y la primera empresa;
// el resto de tenants conserva su base aislada erpgo_<código>.
func (s *Server) isMasterTenant(code string) bool {
	if s == nil || s.config == nil {
		return false
	}
	masterCode := strings.TrimSpace(s.config.DBMasterTenantCode)
	if masterCode == "" {
		masterCode = defaultTenantCode
	}
	return code == masterCode
}

// currentDBPath devuelve el nombre de la BASE MySQL de la empresa activa.
//
// El nombre de la función se conserva porque la usan dos sitios que ya
// saben qué hacen (respaldar y restaurar), y renombrarla sería ruido sin
// ganancia. Lo que cambia es lo que devuelve: antes una ruta de fichero,
// ahora un nombre de base.
func (s *Server) currentDBPath() string {
	return s.tenantDBName(s.tenantCodeOrDefault())
}

// tenantCodeOrDefault devuelve el código de la empresa activa, o "default"
// cuando no hay sesión multi-empresa (PyME de una sola empresa).
func (s *Server) tenantCodeOrDefault() string {
	if s != nil && s.tenantCode() != "" {
		return s.tenantCode()
	}
	if s != nil && s.config != nil {
		return nombreTenantMaestro(s.config)
	}
	return defaultTenantCode
}

// ---------- Registro de empresas en la BD de control ----------

// controlStore abre la BD de control de forma perezosa y LA MANTIENE ABIERTA
// por vida del Server. No es un singleton de paquete a propósito: el catálogo
// pertenece a esta instancia (y a su directorio de datos), de modo que dos
// Servers sobre directorios distintos —como dos pruebas— no comparten estado,
// y el handle se puede cerrar limpiamente en Close().
func (s *Server) controlStore() (*sql.DB, error) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()

	// La base de CONTROL. El servidor la abre al arranque (cmd/server/main.go
	// ya la creó y le aplicó las migraciones), así que aquí se reutiliza ese
	// handle en vez de abrir una segunda conexión a la misma base.
	//
	// OJO con el orden: aquí NO se puede salir antes de aplicar controlSchema.
	// El arranque fija s.controlDB = db (ver NewServer), así que un "ya está
	// abierto, devuélvelo" al principio haría que el esquema de control no se
	// aplicara NUNCA: `tenants` no llegaría a existir, el arranque se vería
	// normal y el fallo aparecería al dar de alta la primera empresa —el peor
	// sitio posible para descubrirlo. Por eso primero se resuelve el handle y
	// solo después se aplica el esquema, siempre.
	if s.controlDB == nil && s.db != nil {
		s.controlDB = s.db
	}
	if s.controlDB == nil {
		db, err := persistence.OpenMySQL(persistence.ConfigMySQL{
			Host:    s.config.DBHost,
			Port:    s.config.DBPort,
			Usuario: s.config.DBUser,
			Clave:   s.config.DBPassword,
			Nombre:  s.config.DBName,
		})
		if err != nil {
			return nil, err
		}
		s.controlDB = db
	}

	db := s.controlDB

	// LA BASE DE CONTROL LLEVA EL MISMO ESQUEMA QUE UNA EMPRESA, y no es un
	// descuido: es donde viven los usuarios y sus credenciales (ver authDB y
	// identity.go), así que el login necesita sus tablas aquí. Antes, una
	// instalación nueva creaba la base de control con cuatro tablas de catálogo y
	// sembraba al administrador en la base de la empresa: el login leía una tabla
	// `user_login` que en control no existía y el administrador de una empresa
	// recién creada no podía entrar.
	//
	// El esquema se define UNA vez —migrations/— para las dos clases de base.
	// Repetir aquí las tablas de identidad sería la segunda copia de la regla: la
	// que se queda atrás sin que nadie se entere.
	//
	// Se aplica UNA vez por proceso, y solo si el handle es la base de control
	// configurada. El registro de migraciones (schema_migrations) ya evita repetir
	// sentencias, pero leer y sumar 35 archivos en cada llamada no es gratis; y una
	// base de prueba —que no es la de control— no tiene por qué recibir el esquema
	// completo por haber sido montada como catálogo.
	//
	// El esquema de control se aplica una sola vez, junto con las migraciones.
	// Repetirlo en cada consulta era trabajo inútil en MySQL y, en las pruebas
	// SQLite con esquema preparado, hacía que authDB fallara al ejecutar DDL
	// MySQL (AUTO_INCREMENT), devolviendo silenciosamente la BD de empresa: de
	// ahí salían el formulario sin roles y el listado de agentes vacío.
	//
	// controlListo se deja en true solo después de completar las sentencias
	// portables del catálogo. Si fallan, la próxima llamada puede reintentarlas y
	// el error no se oculta tras una consulta a otra base.
	if !s.controlListo {
		s.aplicarEsquemaDeControl(db)
		if err := persistence.AplicarEsquema(db, controlSchema); err != nil {
			return nil, fmt.Errorf("esquema de control: %v", err)
		}
		s.controlListo = true
	}
	return db, nil
}

// aplicarEsquemaDeControl deja el esquema completo en la base de control, que es
// la que sostiene la identidad. No se llama sobre una base que no sea la de
// control: un catálogo montado sobre otra base (pruebas, servidor mínimo con
// s.db asignado a mano) no tiene que recibir 139 tablas por haber hecho de
// catálogo.
func (s *Server) aplicarEsquemaDeControl(db *sql.DB) {
	if s.config == nil || db == nil {
		return
	}
	var nombre string
	if err := db.QueryRow(`SELECT DATABASE()`).Scan(&nombre); err != nil {
		log.Printf("no se pudo leer el nombre de la base de control: %v", err)
		return
	}
	if nombre == "" || nombre != s.config.DBName {
		return
	}
	if err := persistence.RunMigrationsForMySQL(db, persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: s.config.DBUser,
		Clave:   s.config.DBPassword,
		Nombre:  nombre,
	}); err != nil {
		log.Printf("esquema de la base de control: %v — sin sus tablas de identidad el login no encuentra a nadie", err)
	}
}

// Close cierra los handles de base de datos abiertos por el Server (los de
// las empresas cargadas y el catálogo de control).
func (s *Server) Close() error {
	var firstErr error

	// Los orquestadores primero: son escritores, y si se cerraran las bases
	// antes podrían intentar confirmar una escritura contra un handle ya
	// cerrado. Lo pendiente no se pierde (queda en los journals), pero el
	// escritor se para en orden.
	stopOrchestrators()

	// La frontera de addons después: los procesos hijos mueren antes de cerrar
	// las bases; jamás queda un addon apadrinado por un ERP que ya se fue.
	s.addons.parar()

	// El ciclo de respaldos automáticos apagan antes de cerrar las bases:
	// no debe quedar escribiendo sobre un MySQL ya desconectado.
	if s.backupStop != nil {
		close(s.backupStop)
		s.backupStop = nil
	}

	// El runner de facturación digital igual: una emisión a medio vuelo no
	// debe chocar con una base que se cierra. Lo pendiente queda en cola.
	s.diDesactivarRunner()

	s.controlMu.Lock()
	if s.controlDB != nil {
		if err := s.controlDB.Close(); err != nil {
			firstErr = err
		}
		s.controlDB = nil
	}
	s.controlMu.Unlock()

	s.tenantMu.Lock()
	for code, db := range s.tenantDBs {
		if err := db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.tenantDBs, code)
	}
	s.tenantMu.Unlock()
	return firstErr
}

// tenantDB devuelve el handle de una empresa, abriéndolo la primera vez. Es la
// vía normal: la usan el middleware (una vez por petición) y settingsOfTenant.
func (s *Server) tenantDB(code string) (*sql.DB, error) {
	s.tenantMu.RLock()
	db, ok := s.tenantDBs[code]
	s.tenantMu.RUnlock()
	if ok {
		return db, nil
	}
	return s.openTenantDB(code)
}

// openTenantDB abre (o reutiliza) el handle MySQL de una empresa. Los handles
// se cachean: cambiar de empresa ida y vuelta no reabre la base, y el pool
// de conexiones se comparte sin fugas de conexiones.
func (s *Server) openTenantDB(code string) (*sql.DB, error) {
	if s.isMasterTenant(code) {
		if s.controlDB != nil {
			return s.controlDB, nil
		}
		return nil, fmt.Errorf("la empresa maestra %q usa la base configurada DB_NAME=%q", code, s.config.DBName)
	}
	s.tenantMu.RLock()
	db, ok := s.tenantDBs[code]
	s.tenantMu.RUnlock()
	if ok {
		return db, nil
	}

	s.tenantMu.Lock()
	defer s.tenantMu.Unlock()
	if db, ok := s.tenantDBs[code]; ok {
		return db, nil
	}
	// El cache puede no estar inicializado: servidores de prueba mínimos
	// (o constructor manual) no siempre lo llenan. Sin esta línea, la primera
	// asignación sobre nil mapa revienta con panic y mata el proceso entero.
	if s.tenantDBs == nil {
		s.tenantDBs = map[string]*sql.DB{}
	}
	nombre := s.tenantDBName(code)

	// Resolver la conexión con la precedencia acordada por el dueño del
	// sistema: ARCHIVO POR EMPRESA → CATÁLOGO (credenciales encriptadas) →
	// credenciales globales del .env. Cada paso solo interviene si el
	// anterior no resolvió, y el origen queda en el log (y en el error_final)
	// para que un «Access denied» del hosting siempre se pueda atribuir.
	usuario := s.config.DBUser
	clave := s.config.DBPassword
	fuente := "credenciales globales (.env)"

	// 1) config/empresas/<código>.env: la pata que el administrador puede
	// mirar y arreglar a mano sin tocar la base maestra. La clave va cifrada;
	// si está en claro es una emergencia manual y se usa tal cual.
	if fc, ferr := configs.LoadTenantConfigFile(code); ferr != nil {
		log.Printf("tenant %q: no se pudo leer el archivo de conexión: %v — sigue el catálogo", code, ferr)
	} else if fc != nil {
		claveResuelta := ""
		switch {
		case fc.PasswordEnc != "":
			if plain, derr := security.Decrypt(fc.PasswordEnc, security.DeriveKey(s.config.MasterKey)); derr != nil {
				log.Printf("tenant %q: clave del archivo no descencripta (%v) — sigue el catálogo", code, derr)
			} else {
				claveResuelta = plain
			}
		case fc.PasswordMan != "":
			claveResuelta = fc.PasswordMan
		}
		if claveResuelta != "" || fc.DBUser != "" || fc.DBName != "" {
			if fc.Host != "" && (fc.Host != s.config.DBHost || (fc.Port != 0 && fc.Port != s.config.DBPort)) {
				log.Printf("tenant %q: su archivo declara %s:%d distinto del servidor configurado %s:%d — el runtime SIEMPRE usa el host del .env", code, fc.Host, fc.Port, s.config.DBHost, s.config.DBPort)
			}
			if claveResuelta != "" {
				clave = claveResuelta
			}
			if fc.DBUser != "" {
				usuario = fc.DBUser
			}
			if fc.DBName != "" {
				nombre = fc.DBName
			}
			fuente = "archivo por empresa"
		}
	}

	// 2) Credenciales por empresa en el catálogo (empresas creadas antes del
	// archivo, o cuyo archivo desapareció).
	if fuente == "credenciales globales (.env)" {
		if creds := s.getTenantCredentials(code); creds != nil {
			usuario = creds.DBUser
			clave = creds.DBPasswordPlain
			if creds.DBName != "" {
				nombre = creds.DBName
			}
			fuente = "catálogo (credenciales de empresa)"
		}
	}
	log.Printf("tenant %q: conexión vía %s (usuario=%q, base=%q)", code, fuente, usuario, nombre)

	// NO crear la base automáticamente. Si la empresa existe en el catálogo
	// pero su base no, es un error de infraestructura que debe resolverse
	// manualmente, no una creación silenciosa que llena el servidor de basura.
	opened, err := persistence.OpenMySQL(persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: usuario,
		Clave:   clave,
		Nombre:  nombre,
	})
	if err != nil {
		// El fallo se reporta con lo que REALMENTE se intentó (base y usuario):
		// el Error 1044 sin este detalle provocaba adivinanzas — ¿fue el usuario
		// global por falta de credenciales propias, o las propias con algo mal?
		// El log anterior de getTenantCredentials ya dice cuál fue el camino.
		return nil, fmt.Errorf("la base de la empresa %q no es accesible (se intentó %s@%s:%d sobre la base %q): %w",
			code, usuario, s.config.DBHost, s.config.DBPort, nombre, err)
	}
	// Verificar que la base realmente existe y tiene tablas
	if err := opened.Ping(); err != nil {
		opened.Close()
		return nil, fmt.Errorf("la base de la empresa %q no responde: %w", code, err)
	}

	// MIGRAR AL ABRIR, NO SÓLO AL CREAR.
	//
	// Las migraciones se aplicaban en un único sitio: la creación de la
	// empresa (ProvisionTenant). Consecuencia medida: una base creada antes de
	// que existiera una pantalla nueva se quedaba con el esquema de su fecha de
	// nacimiento para siempre —sin la tabla, sin la columna, sin la fila de
	// menú— y todo lo que se apoyaba en ella respondía 500 o 404 mientras la
	// misma versión funcionaba en una instalación recién creada. Es la clase de
	// fallo que se ve como «la vista está rota» y en realidad es «el esquema de
	// esta empresa es viejo».
	//
	// Va aquí y no en el arranque porque éste es el ÚNICO punto por el que pasa
	// toda empresa: el middleware, el cambio de empresa y los procesos en
	// segundo plano acaban en openTenantDB. Se ejecuta una vez por proceso
	// (después el handle queda en caché) y es barato cuando no hay nada nuevo:
	// el registro de migraciones se compara por hash y no se lanza una sola
	// sentencia. Un fallo NO impide abrir la empresa: se registra y se sigue,
	// porque un esquema viejo permite trabajar y un ERP que no arranca no.
	if err := persistence.RunMigrationsForMySQL(opened, persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: usuario,
		Clave:   clave,
		Nombre:  nombre,
	}); err != nil {
		log.Printf("migraciones de la empresa %q: %v — se abre con el esquema que tenga", code, err)
	}

	s.tenantDBs[code] = opened
	return opened, nil
}

// controlSchema define la BD de control. Es deliberately pequeño: solo el
// catálogo de empresas y sus miembros. Los datos de negocio de cada empresa
// viven en su propia base MySQL.
var controlSchema = []string{
	`CREATE TABLE IF NOT EXISTS tenants (
		code VARCHAR(64) PRIMARY KEY,
		legal_name VARCHAR(255) NOT NULL DEFAULT '',
		rif VARCHAR(32) NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 1,
		created_at VARCHAR(32) NOT NULL DEFAULT '',
		db_user VARCHAR(128) NOT NULL DEFAULT '',
		db_password_enc VARBINARY(512) DEFAULT '',
		max_users INTEGER NOT NULL DEFAULT 0,
		created_by VARCHAR(128) NOT NULL DEFAULT '',
		db_size_mb INTEGER NOT NULL DEFAULT 0
	)`,
	// OJO: nada de "IF NOT EXISTS" aquí. `CREATE INDEX IF NOT EXISTS` y
	// `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` son sintaxis de MariaDB: MySQL
	// responde 1064 (error de sintaxis) y aborta TODO el esquema de control, con
	// lo que el arranque se queda sin `tenants`, sin sesiones persistentes y sin
	// catálogo de empresas. AplicarEsquema ya tolera "eso ya estaba" (1050, 1060,
	// 1061), así que la forma portable es la sentencia a secas: si el índice o la
	// columna ya existen, el error se descarta; si no, se crean.
	`CREATE INDEX idx_tenants_status ON tenants (status)`,
	`CREATE INDEX idx_tenants_created_by ON tenants (created_by)`,
	// Migración: agregar columnas de seguridad si la tabla ya existe (instalaciones
	// antiguas, donde `tenants` se creó sin ellas).
	`ALTER TABLE tenants ADD COLUMN db_user VARCHAR(128) NOT NULL DEFAULT ''`,
	`ALTER TABLE tenants ADD COLUMN db_password_enc VARBINARY(512) DEFAULT ''`,
	// db_name: empresas creadas en hosting compartido, donde el panel obliga a
	// nombres que no derivan del código (prefijo de la cuenta). Vacío = derivar
	// erpgo_<código> como siempre (compatibilidad con instalaciones antiguas).
	`ALTER TABLE tenants ADD COLUMN db_name VARCHAR(64) NOT NULL DEFAULT ''`,
	`ALTER TABLE tenants ADD COLUMN max_users INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE tenants ADD COLUMN created_by VARCHAR(128) NOT NULL DEFAULT ''`,
	`ALTER TABLE tenants ADD COLUMN db_size_mb INTEGER NOT NULL DEFAULT 0`,
	`CREATE TABLE IF NOT EXISTS tenant_users (
		tenant_code VARCHAR(64) NOT NULL,
		username VARCHAR(128) NOT NULL,
		role_id INTEGER NOT NULL DEFAULT 0,
		is_owner INTEGER NOT NULL DEFAULT 0,
		status INTEGER NOT NULL DEFAULT 1,
		created_at VARCHAR(32) NOT NULL DEFAULT '',
		PRIMARY KEY (tenant_code, username)
	)`,
	`CREATE TABLE IF NOT EXISTS tenant_provision_log (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		tenant_code VARCHAR(64) NOT NULL,
		action VARCHAR(64) NOT NULL,
		detail VARCHAR(500) NOT NULL DEFAULT '',
		created_at VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	// Las sesiones viven en la BD de CONTROL y no en la de cada empresa, por dos
	// razones concretas. Primera: una sesión es la identidad de QUIÉN está
	// conectado, y esa pregunta se responde antes de saber en qué empresa está.
	// Segunda: si vivieran en la BD de la empresa, cambiar de empresa cerraría la
	// sesión, y cambiar de empresa es precisamente lo que el usuario hace
	// varias veces al día.
	//
	// El ID de sesión es un hash, no el valor en claro: la tabla no sirve para
	// robar una sesión, solo para sobrevivir a un reinicio del servidor.
	`CREATE TABLE IF NOT EXISTS web_sessions (
		id VARCHAR(191) PRIMARY KEY,
		user_id VARCHAR(64) NOT NULL DEFAULT '',
		username VARCHAR(128) NOT NULL DEFAULT '',
		fullname VARCHAR(191) NOT NULL DEFAULT '',
		user_level VARCHAR(64) NOT NULL DEFAULT '',
		image VARCHAR(255) NOT NULL DEFAULT '',
		tenant_code VARCHAR(64) NOT NULL DEFAULT '',
		is_agent INTEGER NOT NULL DEFAULT 0,
		csrf_token VARCHAR(191) NOT NULL DEFAULT '',
		expires_at VARCHAR(32) NOT NULL DEFAULT '',
		created_at VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	// El índice cubre el barrido de expiradas, que es la única consulta
	// frecuente sobre sesiones y de lo contrario sería un escaneo completo.
	`CREATE INDEX idx_web_sessions_expires ON web_sessions(expires_at)`,
}

// ---------- Aprovisionamiento ----------

// ProvisionTenantResult resume lo que pasó al crear una empresa.
type ProvisionTenantResult struct {
	Code      string
	Path      string
	Created   bool // true si la empresa (y su BD) se acaban de crear
	AdminUser string
	AdminPass string
}

// ProvisionTenant crea (o re-abre) una empresa: reserva su base MySQL,
// aplica el esquema, siembra los catálogos base y registra la empresa en el
// catálogo de control. Es idempotente: si la empresa ya existe, solo la abre.
//
// Crear una empresa es un evento de seguridad: se deja registrado en
// tenant_provision_log.
func (s *Server) ProvisionTenant(legalName, rif, adminUser, adminPass string) (ProvisionTenantResult, error) {
	res := ProvisionTenantResult{Code: slugifyTenant(legalName), AdminUser: adminUser, AdminPass: adminPass}
	if res.Code == nombreTenantMaestro(s.config) {
		res.Code += "_2"
	}

	// Código único: si ya existe en el catálogo, se agrega sufijo numérico.
	//
	// La comprobación se hace contra el CATÁLOGO, no mirando si existe una
	// base. Antes se miraba en disco; con MySQL la existencia de una
	// empresa es un dato de la base de control, y mirar el disco daría por
	// libre un código que ya está en uso.
	for i := 2; ; i++ {
		if res.Code != nombreTenantMaestro(s.config) && !s.TenantExistsInControl(res.Code) {
			res.Created = true
			break
		}
		res.Code = fmt.Sprintf("%s_%d", slugifyTenant(legalName), i)
	}
	res.Path = s.tenantDBName(res.Code)

	// PASO 1: Crear la base MySQL vacía (si no existe).
	// En MySQL, no se puede abrir una base que no existe. Hay que crearla
	// primero, luego abrir el handle, luego migrar y sembrar.
	if res.Created {
		cfg := persistence.ConfigMySQL{
			Host:    s.config.DBHost,
			Port:    s.config.DBPort,
			Usuario: s.config.DBUser,
			Clave:   s.config.DBPassword,
			Nombre:  res.Path,
		}
		if err := persistence.EnsureDatabase(cfg); err != nil {
			return res, fmt.Errorf("no se pudo crear la base de la empresa: %w", err)
		}
	}

	// PASO 2: Abrir el handle de la empresa (ya existe la base).
	db, err := s.openTenantDB(res.Code)
	if err != nil {
		return res, fmt.Errorf("no se pudo abrir la base de la empresa: %w", err)
	}

	// PASO 3: Aplicar migraciones (esquema completo).
	if err := persistence.RunMigrationsForMySQL(db, persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: s.config.DBUser,
		Clave:   s.config.DBPassword,
		Nombre:  res.Path,
	}); err != nil {
		return res, fmt.Errorf("no se pudo migrar la base de la empresa: %w", err)
	}

	// PASO 4: Catálogos base: aquí es donde una empresa deja de estar vacía y pasa a
	// ser usable. Se llama a la MISMA siembra que usa el arranque normal, para
	// que una empresa creada desde el asistente tenga exactamente lo mismo que
	// una empresa instalada: administrador con su clave, menús, permisos y plan
	// de cuentas del SENIAT.
	// adminUser/adminPass vacíos significan "usa los valores por defecto". El
	// asistente siempre los envía porque no hay nadie a quien preguntarle: en
	// ese flujo la clave la elige quien instala, no el sistema.
	// Safety net: if the wizard somehow sends no credentials, generate a strong
	// random password and display it once. The wizard ALWAYS sends credentials
	// (the form requires them), so this is a last resort, not the normal path.
	//
	// La credencial inventada se distingue de la escrita por el usuario porque las
	// dos no pueden tratarse igual cuando el nombre ya existe en la base de control
	// (ver PASO 6): una clave al azar que nadie vio no se puede "reclamar" contra
	// una identidad existente, y la que escribió una persona sí.
	credencialInventada := false
	if adminUser == "" {
		adminUser = "admin"
		const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		buf := make([]byte, 16)
		for i := range buf {
			n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
			buf[i] = chars[n.Int64()]
		}
		adminPass = "temp-" + string(buf)
		credencialInventada = true
		fmt.Printf("=== TEMPORARY ADMIN PASSWORD: %s ===\n", adminPass)
	}
	PrepareCompanyAs(db, "mysql", legalName, rif, adminUser, adminPass)

	// PASO 4½: los addons ya escaneados existen también en esta empresa nueva.
	// Sin esto, un addon instalado antes de crear la empresa desaparecería de
	// su menú y de su pantalla de módulos (y el dueño lo reportaría como un
	// addon roto, no como un registro faltante).
	s.addonRegistrarEmpresa(db)

	// PASO 5: Registrar la empresa en el catálogo de control.
	ctl, err := s.controlStore()
	if err != nil {
		return res, fmt.Errorf("no se pudo abrir el catálogo de empresas: %w", err)
	}
	nowStr := now()
	// El error SÍ se propaga: es el INSERT que hace existir a la empresa. Si
	// falla, la base de la empresa ya está creada pero nadie la conoce, el login
	// no la encuentra y el asistente vuelve a ofrecerse en cada arranque —que es
	// exactamente el bucle que documenta PrepareDefaultCompany. Antes se
	// descartaba y el flujo se daba por bueno.
	if _, err := ctl.Exec(`INSERT INTO tenants (code, legal_name, rif, status, created_at) VALUES (?,?,?,1,?)
		ON DUPLICATE KEY UPDATE legal_name=VALUES(legal_name), rif=VALUES(rif)`,
		res.Code, legalName, rif, nowStr); err != nil {
		return res, fmt.Errorf("no se pudo registrar la empresa en el catalogo: %w", err)
	}
	if adminUser != "" {
		// is_owner=1: quien crea la empresa la posee. Puede delegar el acceso
		// después, pero no puede quedarse sin acceso a lo que él mismo creó.
		_, _ = ctl.Exec(`INSERT IGNORE INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at)
			VALUES (?,?,0,1,1,?)`, res.Code, adminUser, nowStr)
	}
	action := "abrir"
	if res.Created {
		action = "crear"
	}
	_, _ = ctl.Exec(`INSERT INTO tenant_provision_log (tenant_code, action, detail, created_at) VALUES (?,?,?,?)`,
		res.Code, action, "esquema aplicado", nowStr)

	// PASO 6: la credencial del administrador, en la base de control. Es el paso
	// que faltaba: sin él la empresa existe, se ve y su administrador no puede
	// entrar, porque el login busca las credenciales en control y estaban solo en
	// la base de la empresa (ver identity.go).
	//
	// El nombre ya usado por otra empresa se trata distinto según de dónde venga la
	// clave: si la escribió una persona y no coincide con la de esa identidad, es un
	// error y se dice —dejarlo pasar sería prometer una clave que no va a abrir la
	// puerta, que es el fallo que este cambio viene a cerrar—; si la inventó el
	// sistema (contraseña temporal del respaldo), no hay nada que reclamar y la
	// empresa queda usable con la identidad que ya existía.
	if err := s.asegurarCredencialDeEmpresa(res.Code, adminUser, adminPass); err != nil {
		if errors.Is(err, errIdentidadYaExiste) && credencialInventada {
			log.Printf("AVISO: %v; el administrador de %s entra con la contraseña de esa identidad, no con la temporal", err, res.Code)
		} else {
			return res, fmt.Errorf("la empresa se creó pero su administrador no puede entrar: %w", err)
		}
	}

	// PASO 7: el administrador de la empresa es su SUPER ADMIN (master).
	//
	// Es lo que hace el asistente de instalación (internal/setup/wizard.go) y lo
	// que NO hacía este camino: una empresa creada desde el panel nacía con su
	// dueño sin fila en sec_userrole de ninguna base, así que el nivel se
	// resolvía a "employee" y el middleware le devolvía 403 "No tiene permisos
	// para este módulo" en casi todo el ERP, incluida la gestión de sus propios
	// usuarios y ajustes. El dueño de la empresa tiene que poder administrarla:
	// eso es lo que significa ser su dueño (tenant_users.is_owner=1).
	//
	// El error NO se traga y NO invalida la creación: la empresa ya existe y el
	// usuario puede entrar; lo que se pierde si esto falla es su nivel. Se avisa
	// en el log en vez de dejar creer que quedó como master.
	if err := s.grantAdminRoleToOwner(ctl, adminUser); err != nil {
		log.Printf("AVISO: la empresa %s se creó pero no se pudo confirmar el rol admin de %q: %v",
			res.Code, adminUser, err)
	}

	return res, nil
}

// rolesBaseSistema son los roles del sistema, los MISMOS que siembra el asistente
// de instalación (internal/setup/wizard.go). El id manda: `1` es Super Admin
// (master) y es el que se concede al dueño de una empresa nueva.
var rolesBaseSistema = []struct {
	ID    int
	Tipo  string
	Nivel string
	Desc  string
}{
	{1, "Super Admin", "master", "Acceso total al sistema"},
	{2, "Admin", "admin", "Administrador de empresa"},
	{3, "Manager", "manager", "Gerente"},
	{4, "Employee", "employee", "Empleado"},
}

// grantAdminRoleToOwner registra al dueño de una empresa como admin global,
// nunca como master. El rol master se reserva al superadministrador de la
// instalación; promover a master a cada dueño de tenant permitiría a cualquier
// empresa crear otras empresas. Esta fila vive en control y se usa como fallback
// si la base propia aún no tiene sec_userrole.
//
// Es idempotente: si el usuario ya tiene ese rol, no se toca. Sin esta
// comprobación, cada re-provisión de la misma empresa añadiría otra fila (la
// tabla no tiene UNIQUE sobre user_id+roleid) y el nivel se resolvería por...
// la que devolviera el LIMIT 1, que es una lotería.
func (s *Server) grantAdminRoleToOwner(ctl *sql.DB, username string) error {
	if ctl == nil || strings.TrimSpace(username) == "" {
		return nil
	}
	// Los roles del sistema primero: sin la fila id=2 no hay a qué apuntar.
	for _, r := range rolesBaseSistema {
		if _, err := ctl.Exec(`INSERT IGNORE INTO sec_role (id, type, level, description, is_system)
			VALUES (?,?,?,?,1)`, r.ID, r.Tipo, r.Nivel, r.Desc); err != nil {
			return fmt.Errorf("sembrando el rol %q: %w", r.Tipo, err)
		}
	}

	var userID string
	if err := ctl.QueryRow(`SELECT user_id FROM user_login WHERE username = ? LIMIT 1`, username).Scan(&userID); err != nil {
		return fmt.Errorf("buscando la identidad de %q en la base de control: %w", username, err)
	}
	if userID == "" {
		return fmt.Errorf("la identidad de %q está en la base de control sin user_id", username)
	}

	const ownerRoleID = 2
	var ya int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE user_id = ? AND roleid = ?`, userID, ownerRoleID).Scan(&ya); err != nil {
		return fmt.Errorf("comprobando el rol de %q: %w", username, err)
	}
	if ya > 0 {
		return nil
	}
	if _, err := ctl.Exec(`INSERT INTO sec_userrole (user_id, roleid, createby, createdate) VALUES (?, ?, 'system', ?)`,
		userID, ownerRoleID, now()); err != nil {
		return fmt.Errorf("asignando el rol admin a %q: %w", username, err)
	}
	return nil
}

// ---------- Consultas del catálogo ----------

// esUsuarioMaster pregunta si un usuario tiene el rol de nivel master en el
// catálogo de control. Es quien decide si al cambiarse de empresa ve TODAS o
// solo las suyas: el superadministrador es el dueño del sistema completo y el
// selector no debe ocultarle empresas que él mismo creó.
func (s *Server) esUsuarioMaster(username string) bool {
	ctl, err := s.controlStore()
	if err != nil {
		return false
	}
	var n int
	_ = ctl.QueryRow(`SELECT COUNT(*)
		FROM sec_userrole ur
		JOIN user_login ul ON ul.user_id = ur.user_id AND ul.status = 1
		JOIN sec_role r ON r.id = ur.roleid
		WHERE ul.username = ? AND r.level = 'master'`, username).Scan(&n)
	return n > 0
}

// ListTenantsForUser devuelve las empresas a las que pertenece un usuario,
// con la bandera IsMember ya resuelta. La sesión puede pedir el selector de
// empresa solo con esta lista.
//
// Un usuario MASTER ve todas las empresas registradas: la regla de membresía
// es para operadores. Sin esto, el superadministrador que crea una empresa
// nueva desde el asistente de consola no la ve en /switch-tenant — porque la
// membresía (tenant_users) la tuvo el admin que él asignó, no él mismo — y
// la empresa queda inaccesible desde la web: el fallo reportado con
// TECNOTEXTIL, que existía registrada pero sin puerta.
func (s *Server) ListTenantsForUser(username string) ([]Tenant, error) {
	ctl, err := s.controlStore()
	if err != nil {
		// Sin catálogo de control, se degrada a una sola empresa implícita.
		return []Tenant{{Code: "default", LegalName: "Mi Empresa", IsMember: true}}, nil
	}
	var list []Tenant
	if s.esUsuarioMaster(username) {
		rows, err := ctl.Query(`SELECT code, legal_name, rif, status, created_at
			FROM tenants
			WHERE status = 1
			ORDER BY legal_name`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var t Tenant
			var rif, created sql.NullString
			if err := rows.Scan(&t.Code, &t.LegalName, &rif, &t.Status, &created); err != nil {
				continue
			}
			t.RIF = rif.String
			t.CreatedAt = created.String
			t.IsMember = true
			list = append(list, t)
		}
		rows.Close()
	}
	rows, err := ctl.Query(`SELECT t.code, t.legal_name, t.rif, t.status, t.created_at
		FROM tenants t
		JOIN tenant_users tu ON tu.tenant_code = t.code
		WHERE tu.username = ? AND tu.status = 1
		ORDER BY t.legal_name`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	vistos := map[string]bool{}
	for _, t := range list {
		vistos[t.Code] = true
	}
	for rows.Next() {
		var t Tenant
		var rif, created sql.NullString
		if err := rows.Scan(&t.Code, &t.LegalName, &rif, &t.Status, &created); err != nil {
			continue
		}
		t.RIF = rif.String
		t.CreatedAt = created.String
		t.IsMember = true
		if vistos[t.Code] {
			continue
		}
		list = append(list, t)
	}
	rows.Close()

	// La EMPRESA DEMO de práctica se añade SIEMPRE al final si está activa:
	// es el escenario de GOti y de práctica libre, disponible para cualquier
	// usuario aunque no sea miembro. Si el admin la desactivó, no aparece.
	if s.TenantExistsInControl(tenantDemoCode) && s.tenantEsDemo(tenantDemoCode) {
		yaEsta := false
		for _, t := range list {
			if t.Code == tenantDemoCode {
				yaEsta = true
				break
			}
		}
		if !yaEsta {
			var legal sql.NullString
			_ = ctl.QueryRow(`SELECT legal_name FROM tenants WHERE code = ?`, tenantDemoCode).Scan(&legal)
			nombre := legal.String
			if nombre == "" {
				nombre = tenantDemoLabel
			}
			list = append(list, Tenant{
				Code:      tenantDemoCode,
				LegalName: nombre,
				Status:    1,
				IsMember:  true,
			})
		}
	}
	return list, nil
}

// TenantExistsInControl indica si una empresa está registrada.
func (s *Server) TenantExistsInControl(code string) bool {
	ctl, err := s.controlStore()
	if err != nil {
		return false
	}
	var n int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenants WHERE code = ? AND status = 1`, code).Scan(&n)
	return n > 0
}

// TenantCodes devuelve los códigos de todas las empresas registradas.
//
// Lo usan las tareas en segundo plano (el planificador de agentes IA), que no
// pertenecen a ninguna petición: si solo miraran la empresa activa, las tareas
// programadas de las demás empresas no se encolarían nunca.
func (s *Server) TenantCodes() []string {
	ctl, err := s.controlStore()
	if err != nil {
		return []string{s.activeTenantCode()}
	}
	rows, err := ctl.Query(`SELECT code FROM tenants WHERE status = 1 ORDER BY code`)
	if err != nil {
		return []string{s.activeTenantCode()}
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if rows.Scan(&code) == nil && code != "" {
			out = append(out, code)
		}
	}
	if len(out) == 0 {
		return []string{s.activeTenantCode()}
	}
	return out
}

// CanAccessTenant indica si un usuario es miembro de una empresa. Se usa al
// cambiar de empresa: sin esto, cambiar el tenant_code en la cookie sería
// escalamiento de privilegios entre empresas.
//
// Excepción: la EMPRESA DEMO de práctica es accesible para cualquier usuario
// autenticado (is_demo=1). Sus datos viven en una base aparte, y es el
// escenario de GOti: práctica libre sin tocar empresas reales. Un admin puede
// desactivarla (is_demo=0) y entonces vuelve a ser una empresa "de membresía
// normal" — cerrada para quien no esté inscrito.
func (s *Server) CanAccessTenant(username, code string) bool {
	// El rol master abre todas las empresas: es el dueño del sistema.
	if s.esUsuarioMaster(username) {
		return true
	}
	ctl, err := s.controlStore()
	if err != nil {
		// Sin catálogo, solo se permite la empresa implícita.
		return code == "default"
	}
	var n int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code = ? AND username = ? AND status = 1`,
		code, username).Scan(&n)
	if n > 0 {
		return true
	}
	// La demo es de acceso universal SOLO si el administrador no la desactivó.
	var demo int
	err = ctl.QueryRow(`SELECT IFNULL(is_demo,0) FROM tenants WHERE code = ? AND status = 1`, code).Scan(&demo)
	return err == nil && demo == 1 && s.tenantEsDemo(code)
}

// ---------- Contexto de tenant por request ----------

// tenantCode devuelve el código de la empresa activa.
func (s *Server) tenantCode() string {
	s.tenantMu.RLock()
	defer s.tenantMu.RUnlock()
	return s.activeTenant
}

// setTenant cambia la empresa activa (swap atómico del handle de BD) y
// recarga los catálogos en memoria (empresa, moneda, configuración) porque
// son distintos por empresa.
func (s *Server) setTenant(code string) error {
	code = slugifyTenant(code)
	db, err := s.openTenantDB(code)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la base de la empresa %s: %w", code, err)
	}

	s.tenantMu.Lock()
	s.db = db
	s.activeTenant = code
	s.tenantMu.Unlock()

	// Los catálogos (empresa, moneda, IVA) son por empresa: se recargan.
	s.reloadSettings()
	return nil
}

// tenantSetupNeeded dice si hace falta configurar la primera empresa.
//
// DEVUELVE ERROR CUANDO NO SE PUEDE SABER, y ahí está el arreglo del hallazgo
// S1: antes esta función devolvía `true` cuando el catálogo no respondía
// —"hace falta configurar"—, y como /setup/company es una ruta pública sin
// sesión, un MySQL caído o lento convertía el asistente de instalación en una
// puerta abierta: cualquiera en la red podía crear una empresa con un usuario
// master. "No hay empresas" y "no pude leerlo" tienen que ser cosas distintas.
func (s *Server) tenantSetupNeeded() (bool, error) {
	ctl, err := s.controlStore()
	if err != nil {
		return false, fmt.Errorf("catálogo de empresas no disponible: %w", err)
	}
	var n int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM tenants WHERE status = 1`).Scan(&n); err != nil {
		return false, fmt.Errorf("leyendo el catálogo de empresas: %w", err)
	}
	return n == 0, nil
}

// needsTenantSetup es la versión que solo decide un redirect (ir al asistente o
// no). Si el catálogo no se puede leer devuelve false: NO se manda al asistente
// a configurar una instalación que a lo mejor ya existe.
func (s *Server) needsTenantSetup() bool {
	necesita, err := s.tenantSetupNeeded()
	if err != nil {
		log.Printf("%v", err)
		return false
	}
	return necesita
}

// EnsureDefaultTenant registra en el catálogo la empresa con la que arrancó el
// servidor. Sin esto, una instalación nueva sembraba su base de datos pero no
// la anunciaba en ninguna parte: el login no encontraba ninguna empresa y
// mandaba al asistente de configuración, que a su vez creaba una segunda
// empresa vacía. El usuario nunca podía entrar a los datos que ya existían.
//
// Es idempotente y no pisa nada: si la empresa ya está registrada, se respeta// lo que haya (nombre, RIF) porque el usuario pudo haberlo corregido.
func (s *Server) EnsureDefaultTenant() {
	ctl, err := s.controlStore()
	if err != nil {
		log.Printf("catálogo de empresas no disponible: %v", err)
		return
	}
	code := nombreTenantMaestro(s.config)
	db := s.tenantDBMust(code)
	if db == nil {
		// Sin base no hay nada que registrar, y el paso siguiente sería
		// consultar un handle nil: un panic que tumba el proceso entero. Un
		// arranque con la base caída tiene que ser un arranque con un aviso,
		// no una caída.
		log.Printf("la base de la empresa %s no está disponible: no se registra en el catálogo", code)
		return
	}

	// El nombre y el RIF se leen de la base ya sembrada, no se inventan aquí.
	var legalName, rif string
	_ = db.QueryRow(`SELECT company_name, IFNULL(vat_no,'') FROM company_information
		WHERE status=1 ORDER BY company_id LIMIT 1`).Scan(&legalName, &rif)
	if legalName == "" {
		legalName = "Mi Empresa"
	}

	if _, err := ctl.Exec(`INSERT IGNORE INTO tenants (code, legal_name, rif, status, created_at) VALUES (?,?,?,1,?)
		`, code, legalName, rif, now()); err != nil {
		log.Printf("registro de la empresa inicial: %v", err)
		return
	}

	// El administrador de esa base es su propietario. La pertenencia se
	// registra con TODOS los administradores, no solo con "admin": una base
	// migrada desde el PHP puede tener otros, y dejarlos fuera los dejaría
	// encerrados fuera de su propia empresa.
	rows, err := db.Query(`SELECT username FROM user_login WHERE user_type = 1 AND status = 1`)
	if err != nil {
		return
	}
	defer rows.Close()
	nowStr := now()
	for rows.Next() {
		var username string
		if rows.Scan(&username) != nil || username == "" {
			continue
		}
		_, _ = ctl.Exec(`INSERT IGNORE INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at)
			VALUES (?,?,0,1,1,?)`, code, username, nowStr)

		// Y su credencial, en la base de control, que es donde el login la busca
		// (ver identity.go). Sin esto, una instalación que se sembró antes de que
		// esto existiera —o una nueva, cuyo administrador se creó con una clave al
		// azar que se imprimió UNA vez— tendría su usuario en la base de la empresa
		// y ninguna forma de entrar. Es idempotente y no pisa lo que ya esté: la
		// clave se copia tal como está, bcrypt o el MD5 heredado.
		//
		// El error se registra y NO corta el bucle: que un administrador no se
		// pueda registrar no puede dejar sin registrar a los demás.
		if err := s.asegurarCredencialDeEmpresa(code, username, ""); err != nil {
			log.Printf("credencial de %q en la base de control: %v", username, err)
		}
	}
}

// ---------- HTTP: selector y cambio de empresa ----------

func (s *Server) registerTenantRoutes() {
	s.mux.HandleFunc("GET /switch-tenant", s.switchTenantPage)
	s.mux.HandleFunc("POST /switch-tenant", s.switchTenantDo)
}

// switchTenantPage muestra el selector de empresa con las que el usuario es
// miembro. Solo se listan empresas a las que pertenece: el catálogo completo
// no se expone nunca.
func (s *Server) switchTenantPage(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	tenants, _ := s.ListTenantsForUser(sess.Username)
	names := make([]string, 0, len(tenants))
	for _, t := range tenants {
		names = append(names, t.Code)
	}
	// Página completa, no un fragmento: el selector se presenta como el login
	// (misma caja, mismo fondo) porque es un momento fuera del panel, no una
	// pantalla más del menú.
	s.renderStandalone(r, w, "switch_tenant.html", map[string]interface{}{
		"Title":   "Cambiar de empresa",
		"Tenants": tenants,
		"Current": sess.TenantCode,
		"Names":   names,
	})
}

// switchTenantDo cambia la empresa activa tras validar que el usuario es
// miembro. Cambiar de empresa rota el id de sesión (por seguridad: el
// contexto cambió) y deja el cambio registrado en la auditoría de la empresa
// DESTINO, para poder demostrar qué vio el usuario y desde cuándo.
func (s *Server) switchTenantDo(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no válida", http.StatusUnauthorized)
		return
	}
	code := slugifyTenant(r.FormValue("tenant"))

	// Validación de pertenencia: sin esto, cambiar el código en el formulario
	// sería escalamiento de privilegios entre empresas.
	if !s.CanAccessTenant(sess.Username, code) {
		jsonError(w, "No pertenece a esa empresa", http.StatusForbidden)
		return
	}
	if err := s.setTenant(code); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Rotar el id de sesión al cambiar de empresa.
	cookie, _ := r.Cookie("erpgow_session")
	if cookie != nil {
		s.sessions.Delete(cookie.Value)
	}
	// La procedencia se guarda ANTES de sobrescribir la sesión: leerla después
	// anotaba siempre el destino como origen, y una auditoría que dice que el
	// usuario venía de la misma empresa a la que llegó no sirve para demostrar
	// nada.
	origen := sess.TenantCode

	sess.TenantCode = code
	newID := s.sessions.Create(sess)
	http.SetCookie(w, &http.Cookie{
		Name:     "erpgow_session",
		Value:    newID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	// Auditoría: el cambio de empresa se registra en la empresa destino, que
	// es donde este usuario tiene permisos.
	s.audit(nil, "switch_tenant", "Seguridad", "tenant", code, nil, map[string]string{"from": origen})

	if wantsJSON(r) {
		writeJSON(w, map[string]interface{}{"status": 1, "tenant": code})
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// wantsJSON detecta si el cliente espera JSON (API/mobile) o una redirección.
func wantsJSON(r *http.Request) bool {
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		return true
	}
	ct := r.Header.Get("Content-Type")
	return strings.Contains(ct, "application/json")
}

// ---------- Aprovisionamiento desde el asistente (sin login previo) ----------

func (s *Server) registerTenantSetupRoutes() {
	s.mux.HandleFunc("GET /setup/company", s.setupCompanyPage)
	s.mux.HandleFunc("POST /setup/company", s.setupCompanyDo)
}

// companySetupAllowed decide quién puede crear una empresa. Son dos reglas y
// no una, porque el problema cambia por completo segun haya empresas o no:
//
//   - Sin empresas: cualquiera que abra /login llega aquí. Todavia no hay a quien
//     pedirle permiso: no existe ningun usuario. Es la instalación.
//   - Con empresas: solo un SUPER ADMIN (master) con sesión. Aquí crear una empresa ya
//     no es instalar, es una acción de la que hay que responder, y sin sesión
//     sería regalar empresas a cualquiera que abra la URL.
//
// La segunda es la que evita que el asistente de primer arranque quede como
// puerta abierta una vez instalado el sistema.
//
// Punto de auditoría: separar super admin (master) de admin de empresa.
// Solo master puede crear empresas. Admin de empresa gestiona solo su empresa.
// Devuelve (permitido, esInstalacion, error). El error es "no se pudo saber si
// hay empresas": quien llama responde 503, nunca deja pasar.
func (s *Server) companySetupAllowed(r *http.Request) (bool, bool, error) {
	necesita, err := s.tenantSetupNeeded()
	if err != nil {
		return false, false, err
	}
	if necesita {
		return true, true, nil
	}
	sess := s.sessionFromRequest(r)
	return sess != nil && sess.UserLevel == "master", false, nil
}

// bloquearInstalacion serializa la creación de la PRIMERA empresa.
//
// Entre "no hay ninguna" y "creo la primera" hay una ventana: dos POST
// simultáneos al asistente la cruzan a la vez y nacen dos empresas donde debía
// nacer una, cada una con su usuario master. Un bloqueo con nombre de MySQL
// (GET_LOCK) la cierra. El bloqueo es por CONEXIÓN, así que se toma una conexión
// dedicada y se devuelve la función que lo suelta.
//
// Si el motor no tiene GET_LOCK (SQLite), se sigue sin bloqueo: ahí las
// escrituras ya se serializan contra el archivo, y no instalar por no poder
// tomar un bloqueo sería peor que la ventana que se está cerrando.
func (s *Server) bloquearInstalacion() (func(), error) {
	ctl, err := s.controlStore()
	if err != nil {
		return nil, fmt.Errorf("catálogo de empresas no disponible: %w", err)
	}
	ctx := context.Background()
	conn, err := ctl.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("tomando una conexión del catálogo: %w", err)
	}

	var obtenido sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('erpgow_instalacion', 10)`).Scan(&obtenido); err != nil {
		// Motor sin GET_LOCK: se continúa sin bloqueo, pero se dice.
		log.Printf("AVISO: sin bloqueo de instalación disponible (%v); la creación de la primera empresa no queda serializada", err)
		return func() { _ = conn.Close() }, nil
	}
	if !obtenido.Valid || obtenido.Int64 != 1 {
		_ = conn.Close()
		return nil, fmt.Errorf("otra instalación está en curso")
	}

	return func() {
		_, _ = conn.ExecContext(ctx, `SELECT RELEASE_LOCK('erpgow_instalacion')`)
		_ = conn.Close()
	}, nil
}

// setupCompanyPage muestra el asistente de creación de empresas.
func (s *Server) setupCompanyPage(w http.ResponseWriter, r *http.Request) {
	primera := s.needsTenantSetup()
	if !primera {
		if sess := s.sessionFromRequest(r); sess == nil || sess.UserLevel != "master" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
	}
	titulo := "Nueva empresa"
	if primera {
		titulo = "Configurar empresa"
	}
	s.renderStandalone(r, w, "setup_company.html", map[string]interface{}{
		"Title":       titulo,
		"AdminUser":   "admin",
		"IsFirst":     primera,
		"BackURL":     "/",
		"SubmitLabel": "Crear empresa",
	})
}

// setupCompanyDo crea una empresa y la deja como activa.
func (s *Server) setupCompanyDo(w http.ResponseWriter, r *http.Request) {
	permitido, primera, err := s.companySetupAllowed(r)
	if err != nil {
		// Fail closed: sin poder comprobar el estado del catálogo no se crea
		// ninguna empresa con usuario master.
		jsonError(w, "No se pudo verificar el estado de la instalacion. Reintente en unos segundos.",
			http.StatusServiceUnavailable)
		return
	}
	if !permitido {
		jsonError(w, "Necesita ser super administrador (master) para crear una empresa", http.StatusForbidden)
		return
	}
	// Instalación: dos POST simultáneos cruzarían la ventana entre "no hay
	// ninguna" y "creo la primera" (TOCTOU). El bloqueo con nombre la cierra, y
	// DENTRO se vuelve a comprobar: el primero pudo terminar mientras
	// esperábamos y entonces ya no es una instalación, es crear la segunda.
	if primera {
		liberar, err := s.bloquearInstalacion()
		if err != nil {
			jsonError(w, "Hay otra instalacion en curso. Reintente en unos segundos.", http.StatusConflict)
			return
		}
		defer liberar()

		permitido, primera, err = s.companySetupAllowed(r)
		if err != nil || !permitido || !primera {
			jsonError(w, "La empresa ya está configurada", http.StatusConflict)
			return
		}
	}
	name := strings.TrimSpace(r.FormValue("legal_name"))
	rif := strings.TrimSpace(r.FormValue("rif"))
	adminUser := strings.TrimSpace(r.FormValue("admin_user"))
	adminPass := r.FormValue("admin_pass")

	// El asistente se entra sin credenciales (no hay nadie a quien se le
	// pregunte), así que valida la clave aquí y no en un formulario posterior.
	if len(adminPass) < 8 {
		s.renderStandalone(r, w, "setup_company.html", map[string]interface{}{
			"Title":       "Nueva empresa",
			"Error":       "La contraseña del administrador debe tener al menos 8 caracteres",
			"Name":        name,
			"RIF":         rif,
			"AdminUser":   adminUser,
			"BackURL":     "/",
			"SubmitLabel": "Crear empresa",
		})
		return
	}
	if adminUser == "" {
		adminUser = "admin"
	}

	res, err := s.ProvisionTenantExtended(name, rif, adminUser, adminPass)
	if err != nil {
		s.renderStandalone(r, w, "setup_company.html", map[string]interface{}{
			"Title":     "Nueva empresa",
			"Error":     err.Error(),
			"Name":      name,
			"RIF":       rif,
			"AdminUser": adminUser,
		})
		return
	}
	// Dejar esta empresa como la activa.
	_ = s.setTenant(res.Code)

	// Al terminar, el destino depende de quién hizo la acción. En la
	// instalación no hay sesión (el asistente se entra sin credenciales), así que
	// va al login. Creando una empresa desde el panel ya hay sesión: mandarla al
	// login sería cerrarle la sesión al propio usuario que la acaba de crear.
	if sess := s.sessionFromRequest(r); sess != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ---------- Empresa por defecto ----------

// PrepareDefaultCompany deja lista la base de la empresa «default»: la crea si
// no existe, aplica las migraciones, copia los datos heredados de la base de
// control (instalaciones antiguas) y siembra catálogos y administrador.
//
// Es lo que separa EL ESQUEMA DE CONTROL del de NEGOCIO. Antes, el arranque
// migraba y sembraba sobre la base de control y el tenant «default» resolvía a
// ese mismo handle: una sola empresa funcionaba, pero mezclando catálogo de
// empresas con facturas en la misma base, y cualquier segunda empresa quedaba
// a medias. Con esto, «default» es una empresa más, con su propio archivo de
// datos, igual que las que crea el panel.
//
// Es idempotente: en el arranque normal solo se reabre la base y no se copia
// nada (la base de la empresa ya tiene datos).
func (s *Server) PrepareDefaultCompany() {
	// La empresa maestra comparte DB_NAME con el catálogo y se deja lista al
	// arrancar; nunca debe abrirse/crearse un tenant `default` duplicado.
	if s == nil || s.config == nil {
		return
	}
	if err := PrepararEmpresaMaestra(s.config, s.controlDB); err != nil {
		log.Printf("preparando empresa maestra: %v", err)
	}
}

// empresaVacia dice si la base de la empresa todavía no tiene datos de negocio.
// Una tabla inexistente cuenta como vacía: es exactamente el caso de una base
// recién creada a la que todavía no se le aplicó nada.
func empresaVacia(db *sql.DB) bool {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM company_information`).Scan(&n); err != nil {
		return true
	}
	return n == 0
}
