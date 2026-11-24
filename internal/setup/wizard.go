package setup

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"erpgo/configs"
	"erpgo/infrastructure/persistence"
	"erpgo/internal/security"
	"golang.org/x/crypto/bcrypt"
)

// masterTenantCode es el identificador estable de la empresa que usa DB_NAME.
const masterTenantCode = "default"

// Wizard maneja la instalación interactiva.
type Wizard struct {
	reader *bufio.Reader
}

// NewWizard crea un wizard.
func NewWizard() *Wizard {
	return &Wizard{reader: bufio.NewReader(os.Stdin)}
}

// Run ejecuta el wizard.
func (w *Wizard) Run() error {
	w.printBanner()

	// DISCLAIMER LEGAL. Antes de cualquier operación el instalador lee y
	// acepta las condiciones de uso: es la jornada donde Orcanet1724 C.A
	// fija, de buena fe y por escrito, qué es el sistema (gratuito), qué NO es
	// (un responsable de los datos que el cliente procesa) y qué queda
	// reservado (autoria, distribución). Si no acepta, no se toca nada.
	if !w.aceptarDisclaimer() {
		return fmt.Errorf("instalación cancelada: no se aceptaron los términos de uso")
	}

	if _, err := os.Stat(".env"); err == nil {
		cfg, loadErr := configs.Load()
		if loadErr != nil {
			return fmt.Errorf("leyendo .env: %w", loadErr)
		}
		return w.runMaintenance(cfg)
	}
	return w.runFreshInstall()
}

// aceptarDisclaimer muestra el convenio de uso y exige aceptación expresa.
// Es una condición, no un aviso: sin un «si» claro el wizard no instala nada.
func (w *Wizard) aceptarDisclaimer() bool {
	fmt.Println("┌────────────────────────────────────────────────────────────┐")
	fmt.Println("│        CONVENIO DE USO Y LIMITACIÓN DE RESPONSABILIDAD     │")
	fmt.Println("└────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  1. ERPGo, POSGo y sus herramientas asociadas SON GRATUITAS.")
	fmt.Println("     Orcanet1724 C.A no cobra licencia por el software: sus únicas")
	fmt.Println("     fuentes de ingreso son la venta de hosting, las adecuaciones")
	fmt.Println("     a medida del sistema para cada empresa y la venta de")
	fmt.Println("     inferencia de IA, servicios que se contratan aparte.")
	fmt.Println()
	fmt.Println("  2. Orcanet1724 C.A se abstiene de, y NO se responsabiliza por,")
	fmt.Println("     irregularidades, errores u omisiones que provengan del USO del")
	fmt.Println("     sistema o de las sugerencias generadas por los asistentes de IA.")
	fmt.Println("     La información fiscal (RIF, facturas, retenciones, reportes")
	fmt.Println("     SENIAT/VAT y demás obligaciones tributarias y legales) es")
	fmt.Println("     procesada, validada y presentada bajo la ÚNICA responsabilidad")
	fmt.Println("     del usuario/empresa que usa el sistema. Orcanet1724 C.A es solo")
	fmt.Println("     el FACILITADOR de un medio tecnológico gratuito y de buena fe")
	fmt.Println("     para el desarrollo del comercio venezolano, y en nada se")
	fmt.Println("     compromete con la exactitud legal, contable o tributaria de los")
	fmt.Println("     datos que cada usuario procese.")
	fmt.Println()
	fmt.Println("  3. PROPIEDAD Y CUSTODIA DE LOS DATOS: La base de datos")
	fmt.Println("     (SQLite/MySQL) y los registros generados son propiedad 100%")
	fmt.Println("     exclusiva de la empresa o comercio que opera el sistema.")
	fmt.Println("     Orcanet1724 C.A NO tiene, NO almacena ni tendrá acceso de")
	fmt.Println("     ninguna forma a los datos, montos o transacciones del usuario.")
	fmt.Println("     Por consiguiente, la sanidad, mantenimiento, prevención de")
	fmt.Println("     virus, respaldos (backups) y copias de seguridad de la base de")
	fmt.Println("     datos son responsabilidad única e irrenunciable del usuario.")
	fmt.Println()
	fmt.Println("  4. Software sin costo NO es software sin titularidad. Orcanet1724")
	fmt.Println("     C.A SOLO se reserva el derecho de AUTORÍA, CÓDIGO FUENTE y")
	fmt.Println("     DISTRIBUCIÓN ÚNICA del software ERPGo, POSGo y afines.")
	fmt.Println()
	fmt.Println("  5. PROHIBIDO VENDER, ALQUILAR O REDISTRIBUIR este software a")
	fmt.Println("     título personal o de empresa. Ser representante, integrador o")
	fmt.Println("     facilitador de soporte técnico de las aplicaciones es un estatus")
	fmt.Println("     que ORCANET1724 C.A. otorga CASO POR CASO a personas naturales")
	fmt.Println("     o jurídicas AUTORIZADAS que hayan demostrado capacidad técnica")
	fmt.Println("     y compromiso con el comercio atendido — bajo normas estrictas")
	fmt.Println("     de buen servicio y atención al comercio que deposita su fe en")
	fmt.Println("     el técnico. La premisa es que todos ganen: el comercio, el")
	fmt.Println("     técnico y el ecosistema ERPGo.")
	fmt.Println()
	fmt.Println("  Este texto no sustituye un contrato firmado; es la condición")
	fmt.Println("  técnica de uso del software como está escrito en el código mismo.")
	fmt.Println()
	return w.askYesNo("¿ACEPTA y reconoce las condiciones anteriores para continuar la instalación?")
}

func validarNombreEmpresa(codigo string) error {
	if codigo == "" || strings.TrimSpace(codigo) != codigo {
		return fmt.Errorf("el nombre de la base maestra debe ser no vacío y no llevar espacios extremos")
	}
	if len(codigo) > 53 {
		return fmt.Errorf("DB_NAME supera el máximo seguro de 53 caracteres (prefijo erpgo_ incluido: máximo MySQL 64)")
	}
	for _, r := range codigo {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return fmt.Errorf("DB_NAME solo puede contener letras ASCII, números y guion bajo")
		}
	}
	return nil
}

func validarPuertosSetup(host string, httpPort, mysqlPort, grpcPort int, mysqlYaConectado bool) error {
	puertos := []struct {
		nombre string
		valor  int
	}{{"HTTP", httpPort}, {"MySQL", mysqlPort}, {"gRPC", grpcPort}}
	vistos := make(map[int]string, len(puertos))
	for _, p := range puertos {
		if p.valor < 1 || p.valor > 65535 {
			return fmt.Errorf("puerto %s fuera del rango 1-65535: %d", p.nombre, p.valor)
		}
		if otro, ok := vistos[p.valor]; ok {
			return fmt.Errorf("el puerto %d está asignado tanto a %s como a %s", p.valor, otro, p.nombre)
		}
		vistos[p.valor] = p.nombre
		if p.nombre == "MySQL" && mysqlYaConectado {
			continue
		}
		if !IsPortOpen(host, p.valor) {
			return fmt.Errorf("el puerto %s %d ya está ocupado", p.nombre, p.valor)
		}
	}
	return nil
}

func (w *Wizard) confirmarBaseExistente(cfg persistence.ConfigMySQL) error {
	nombre := cfg.Nombre
	cfg.Nombre = ""
	server, err := persistence.ConectarServidorMySQL(cfg)
	if err != nil {
		return fmt.Errorf("verificando MySQL: %w", err)
	}
	defer server.Close()
	var exists int
	if err := server.QueryRow(`SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?`, nombre).Scan(&exists); err != nil {
		return fmt.Errorf("comprobando base maestra: %w", err)
	}
	if exists == 0 {
		return nil
	}
	fmt.Printf("\nAVISO: la base %q ya existe. No se borrará, pero las migraciones modificarán su esquema.\n", nombre)
	if !w.askYesNo("¿Confirma que esta es la base correcta y desea continuar?") {
		return fmt.Errorf("instalación cancelada: base existente no confirmada")
	}
	return nil
}

func esTenantMaestro(cfg *configs.Config, code string) bool {
	return code == configuredMasterTenantCode(cfg)
}

func configuredMasterTenantCode(cfg *configs.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.DBMasterTenantCode) == "" {
		return masterTenantCode
	}
	return strings.TrimSpace(cfg.DBMasterTenantCode)
}

func slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func (w *Wizard) printBanner() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                  ERPGo — Asistente de Instalación            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// runFreshInstall ejecuta la instalación desde cero.
func (w *Wizard) runFreshInstall() error {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  INSTALACIÓN NUEVA")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// [1/6] Conexión a un MySQL/MariaDB que YA EXISTE.
	//
	// DECISIÓN FIRME: este binario NO instala el motor de base de datos.
	//
	// Antes el asistente ofrecía «¿Desea instalar MariaDB ahora?» y, si se
	// aceptaba, ejecutaba `apt-get install mariadb-server` / `yum` / `dnf` o
	// descargaba un MSI: cambiaba paquetes, servicios, el datadir y el usuario
	// root del sistema operativo anfitrión desde dentro de una aplicación de
	// negocio. Eso rompió instalaciones que ya estaban sanas, y el motor no es
	// asunto del ERP: es infraestructura y la administra quien administra el
	// servidor. Aquí solo se comprueba que responda y se piden credenciales;
	// si no hay motor, se dice con claridad y se sale SIN TOCAR NADA.
	fmt.Println("[1/6] Base de datos MySQL/MariaDB")
	fmt.Println()
	fmt.Println("  Requisito: esta máquina (o el servidor que se indique) debe tener")
	fmt.Println("  MySQL o MariaDB instalado y arrancado ANTES de ejecutar este")
	fmt.Println("  asistente. ERPGo no instala el motor: solo se conecta al que existe.")
	fmt.Println()

	var dbHost, dbUser, dbPass, dbName string
	var dbPort int

	dbHost = w.askInput("Host MySQL/MariaDB (Enter = 127.0.0.1)", false)
	dbPort = w.askInt("Puerto MySQL/MariaDB", DefaultPorts.MySQL)
	dbName = w.askInput("Nombre de la base maestra (DB_NAME)", false)

	// El reintento se queda DENTRO de esta pregunta. Antes, un fallo de
	// credenciales volvía a ejecutar todo el asistente desde el principio
	// (recursión sobre runFreshInstall) y se perdía lo ya respondido.
	for {
		dbUser = w.askInput("Usuario MySQL/MariaDB", false)
		dbPass = w.askInput("Contraseña", true)

		fmt.Println()
		fmt.Println("Verificando conexión a MySQL/MariaDB...")
		testDB, err := persistence.ConectarServidorMySQL(persistence.ConfigMySQL{
			Host:    dbHost,
			Port:    dbPort,
			Usuario: dbUser,
			Clave:   dbPass,
		})
		if err == nil {
			err = testDB.Ping()
			_ = testDB.Close()
		}
		if err == nil {
			fmt.Println("   ✓ Conexión exitosa")
			break
		}

		fmt.Printf("   ✗ No se pudo conectar: %v\n", err)
		fmt.Println("     Revise que el servicio MySQL/MariaDB esté arrancado, que el")
		fmt.Println("     puerto sea el correcto y que el usuario tenga permiso sobre la base.")
		if !w.askYesNo("   ¿Desea reintentar con otros datos de conexión?") {
			return fmt.Errorf("conexión a MySQL/MariaDB fallida: el motor debe estar instalado y accesible antes de instalar ERPGo")
		}
		// Tras un fallo se vuelven a pedir también host, puerto y base: el error
		// más común es justamente el puerto (3307 en hostings) o un host remoto.
		dbHost = w.askInput("Host MySQL/MariaDB (Enter = 127.0.0.1)", false)
		dbPort = w.askInt("Puerto MySQL/MariaDB", DefaultPorts.MySQL)
		dbName = w.askInput("Nombre de la base maestra (DB_NAME)", false)
	}

	if err := validarNombreEmpresa(dbName); err != nil {
		return fmt.Errorf("base maestra inválida: %w", err)
	}

	// [2/6] Puertos del servidor
	fmt.Println()
	fmt.Println("[2/6] Puertos del servidor ERP")
	fmt.Println()
	serverPort := w.askInt("Puerto HTTP del ERP", DefaultPorts.ERP)
	grpcPort := w.askInt("Puerto gRPC (POS)", DefaultPorts.GRPC)

	mysqlResponds := persistence.EsServidorMySQLArriba(dbHost, dbPort)
	if !mysqlResponds {
		return fmt.Errorf("MySQL no responde en %s:%d", dbHost, dbPort)
	}
	if err := validarPuertosSetup("127.0.0.1", serverPort, dbPort, grpcPort, true); err != nil {
		return fmt.Errorf("configuración de puertos: %w", err)
	}

	// [3/6] JWT Secret
	fmt.Println()
	fmt.Println("[3/6] Seguridad")
	fmt.Println()
	jwtSecret := w.askInput("JWT Secret (deje vacío para auto-generar)", false)
	if jwtSecret == "" {
		jwtSecret = generateRandomSecret(32)
		fmt.Println("   JWT generado (se guardará de forma privada en .env)")
	}

	// Guardar .env
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  Generando configuración...")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	mainDBName := strings.TrimSpace(dbName)
	masterCode := configuredMasterTenantCode(nil)
	if err := validarNombreEmpresa(mainDBName); err != nil {
		return fmt.Errorf("base maestra inválida: %w", err)
	}
	masterCfg := persistence.ConfigMySQL{Host: dbHost, Port: dbPort, Usuario: dbUser, Clave: dbPass, Nombre: mainDBName}
	if err := w.confirmarBaseExistente(masterCfg); err != nil {
		return err
	}

	// [4/6] Preparar y migrar la base maestra configurada.
	fmt.Println()
	fmt.Println("[4/6] Preparando base maestra...")
	fmt.Println()
	if err := persistence.EnsureDatabase(masterCfg); err != nil {
		return fmt.Errorf("creando la base maestra %q: %w", mainDBName, err)
	}
	controlDB, err := persistence.OpenMySQL(masterCfg)
	if err != nil {
		return fmt.Errorf("abriendo la base maestra %q: %w", mainDBName, err)
	}
	defer controlDB.Close()
	if err := persistence.RunMigrationsForMySQL(controlDB, masterCfg); err != nil {
		return fmt.Errorf("aplicando migraciones a la base maestra: %w", err)
	}
	fmt.Println("✓ Base maestra migrada")

	// [5/6] Datos de la empresa principal
	fmt.Println()
	fmt.Println("[5/6] Datos de la empresa")
	fmt.Println()
	companyName := w.askInput("Nombre de la empresa", false)
	companyEmail := w.askInput("Email", false)
	companyPhone := w.askInput("Teléfono", false)
	companyAddress := w.askInput("Dirección", false)
	companyRIF := w.askInput("RIF", false)
	companyWebsite := w.askInput("Sitio web (opcional)", false)

	// [6/6] Crear super administrador
	fmt.Println()
	fmt.Println("[6/6] Crear Super Administrador")
	fmt.Println()
	superUser := w.askInput("Usuario (default: admin)", false)
	if superUser == "" {
		superUser = "admin"
	}
	superPass := w.askInput("Contraseña", true)

	// La inicialización de identidad y catálogos de negocio es compartida con
	// el arranque normal; no duplicar el esquema parcial dentro del wizard.
	if err := persistence.InitializeAdminUser(controlDB, "mysql", superUser, superPass); err != nil {
		return fmt.Errorf("creando super administrador y catálogos: %w", err)
	}

	// Reafirmar datos introducidos por el instalador después de la siembra.
	if _, err := controlDB.Exec(
		`INSERT INTO company_information (company_id, company_name, email, address, mobile, website, vat_no, status)
		 VALUES ('1', ?, ?, ?, ?, ?, ?, 1)
		 ON DUPLICATE KEY UPDATE company_name=VALUES(company_name), email=VALUES(email), address=VALUES(address), mobile=VALUES(mobile), website=VALUES(website), vat_no=VALUES(vat_no), status=1`,
		companyName, companyEmail, companyAddress, companyPhone, companyWebsite, companyRIF,
	); err != nil {
		return fmt.Errorf("guardando datos de la empresa maestra: %w", err)
	}

	// Registrar la empresa maestra en el catálogo de tenants. Su base es DB_NAME,
	// no una segunda erpgo_default.
	if _, err := controlDB.Exec(`INSERT INTO tenants (code, legal_name, rif, status, created_at) VALUES (?, ?, ?, 1, NOW())
		ON DUPLICATE KEY UPDATE legal_name=VALUES(legal_name), rif=VALUES(rif), status=1`,
		masterCode, companyName, companyRIF); err != nil {
		return fmt.Errorf("registrando empresa maestra: %w", err)
	}
	if _, err := controlDB.Exec(`INSERT IGNORE INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at)
		VALUES (?, ?, 1, 1, 1, NOW())`, masterCode, superUser); err != nil {
		return fmt.Errorf("registrando propietario de empresa maestra: %w", err)
	}

	// Los roles canónicos (1 master, 2 admin, 3 manager, 4 employee) los siembra
	// InitializeAdminUser EN LA MISMA transacción en la que ata al dueño al
	// master. Antes se sembraban aquí, después de la siembra, y el upsert del
	// id 3 convertía en «Manager» el rol master al que la siembra ya había
	// atado al administrador: el dueño entraba sin permisos y solo veía el menú
	// público. Un solo sembrador y un solo orden evitan esa carrera.

	if err := (&configs.Config{
		DBDriver: "mysql", DBHost: dbHost, DBPort: dbPort, DBUser: dbUser, DBPassword: dbPass,
		DBName: mainDBName, DBMasterTenantCode: masterCode,
		DBRootUser: dbUser, DBRootPassword: dbPass,
		DBProvisionUser: dbUser, DBProvisionPassword: dbPass,
		DBSQLitePath: "data/empresas/default.sqlite3",
		ServerHost:   "127.0.0.1", ServerPort: strconv.Itoa(serverPort), GRPCPort: strconv.Itoa(grpcPort),
		MasterKey: jwtSecret, JWTExpiry: 24,
		AppName: "ERPGo", AppURL: fmt.Sprintf("http://localhost:%d", serverPort),
		AppEnv: "production", UploadPath: "./uploads",
		LogEnabled: true, LogLevel: "info", LogFile: "logs/erpgo.log",
	}).WriteEnvFile(); err != nil {
		return fmt.Errorf("guardando configuración .env: %w", err)
	}

	fmt.Println("✓ Super administrador creado y .env guardado")

	// EMPRESA DEMO de práctica: pregunta explícita de instalación. Si se
	// activa, exige su propia base (la misma mecánica de la empresa real);
	// si se declina, queda fuera — sin empresa fantasma que enmascare datos.
	if w.askYesNo("¿Activar la empresa DEMO de práctica (entrenamiento)? Recomendado: sí. Necesitará su propia base de datos") {
		if err := w.createDemoTenant(&configs.Config{
			DBHost: dbHost, DBPort: dbPort, DBUser: dbUser, DBPassword: dbPass,
			DBName: mainDBName, DBMasterTenantCode: masterCode, MasterKey: jwtSecret,
		}); err != nil {
			fmt.Printf("   DEMO no activada: %v (continúe la instalación; puede activarla después con --setup, opción 3)\n", err)
		}
	}

	// Finalizar
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  INSTALACIÓN COMPLETADA")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  URL: http://localhost:%d\n", serverPort)
	fmt.Printf("  Usuario: %s\n", superUser)
	fmt.Println()
	fmt.Println("  Arrancar manualmente:")
	fmt.Println("    Windows: .\\erpgo.exe")
	fmt.Println("    Linux:   ./erpgow")
	fmt.Println()

	return nil
}

func (w *Wizard) runMaintenance(cfg *configs.Config) error {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  MANTENIMIENTO")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  1. Aplicar migraciones a todas las bases registradas")
	fmt.Println("  2. Crear una empresa adicional")
	fmt.Println("  3. Activar la empresa DEMO de práctica (requiere su propia BD)")
	fmt.Println("  4. Guardar/actualizar las credenciales de BD de una empresa")
	fmt.Println("  5. Restablecer la contraseña del super administrador")
	fmt.Println("  6. Salir")
	fmt.Println()

	switch choice := w.askInput("Opción", false); choice {
	case "1":
		return w.migrateRegisteredCompanies(cfg)
	case "2":
		return w.createCompany(cfg)
	case "3":
		return w.createDemoTenant(cfg)
	case "4":
		return w.actualizarCredencialesEmpresa(cfg)
	case "5":
		return w.restablecerSuperAdmin(cfg)
	case "6", "":
		return nil
	default:
		return fmt.Errorf("opción de mantenimiento desconocida: %q", choice)
	}
}

// restablecerSuperAdmin es la única vía para recuperar la credencial del dueño
// del sistema. El panel no puede hacerlo a propósito: si pudiera, cualquier
// administrador de empresa podría quedarse con el acceso a todas. Aquí hace
// falta estar en el servidor y ejecutar el binario, que es exactamente el nivel
// de confianza que esa llave exige.
func (w *Wizard) restablecerSuperAdmin(cfg *configs.Config) error {
	master, err := w.openMasterDB(cfg)
	if err != nil {
		return fmt.Errorf("abriendo base maestra %q: %w", cfg.DBName, err)
	}
	defer master.Close()

	actual, err := SuperAdminActual(master)
	if err != nil {
		return err
	}
	if actual != "" {
		fmt.Printf("\n  Super administrador actual: %s\n", actual)
		fmt.Println("  Se restablecerá SU contraseña (el rol es único y no se reasigna).")
	} else {
		fmt.Println("\n  ATENCIÓN: la instalación no tiene super administrador.")
		fmt.Println("  Se creará la identidad que indique a continuación.")
	}
	fmt.Println()

	usuario := ""
	if actual == "" {
		usuario = w.askInput("Usuario del super administrador (default: admin)", false)
		if usuario == "" {
			usuario = "admin"
		}
	}
	clave := w.askInput("Contraseña nueva", true)
	if strings.TrimSpace(clave) == "" {
		return fmt.Errorf("la contraseña no puede quedar vacía")
	}
	repetida := w.askInput("Repita la contraseña", true)
	if clave != repetida {
		return fmt.Errorf("las contraseñas no coinciden; no se cambió nada")
	}

	nombre, err := RestablecerCredencialSuperAdmin(master, configuredMasterTenantCode(cfg), usuario, clave)
	if err != nil {
		return err
	}
	fmt.Printf("\n  ✓ Credencial restablecida para %q.\n", nombre)
	fmt.Println("    El rol de super administrador sigue siendo uno solo.")
	fmt.Println()
	return nil
}

func (w *Wizard) openMasterDB(cfg *configs.Config) (*sql.DB, error) {
	conn := persistence.ConfigMySQL{
		Host: cfg.DBHost, Port: cfg.DBPort, Usuario: cfg.DBUser,
		Clave: cfg.DBPassword, Nombre: cfg.DBName,
	}
	return persistence.OpenMySQL(conn)
}

func (w *Wizard) migrateRegisteredCompanies(cfg *configs.Config) error {
	master, err := w.openMasterDB(cfg)
	if err != nil {
		return fmt.Errorf("abriendo base maestra %q: %w", cfg.DBName, err)
	}
	defer master.Close()
	if err := persistence.RunMigrationsForMySQL(master, persistence.ConfigMySQL{
		Host: cfg.DBHost, Port: cfg.DBPort, Usuario: cfg.DBUser,
		Clave: cfg.DBPassword, Nombre: cfg.DBName,
	}); err != nil {
		return fmt.Errorf("migrando base maestra: %w", err)
	}

	rows, err := master.Query(`SELECT code FROM tenants WHERE status = 1 ORDER BY code`)
	if err != nil {
		return fmt.Errorf("leyendo empresas del catálogo maestro: %w", err)
	}
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			return err
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, code := range codes {
		name := cfg.DBName
		if code != configuredMasterTenantCode(cfg) {
			name, err = persistence.NombreBaseEmpresa(code)
			if err != nil {
				return fmt.Errorf("nombre de base para la empresa %q: %w", code, err)
			}
		}
		db, err := persistence.OpenMySQL(persistence.ConfigMySQL{
			Host: cfg.DBHost, Port: cfg.DBPort, Usuario: cfg.DBUser,
			Clave: cfg.DBPassword, Nombre: name,
		})
		if err != nil {
			return fmt.Errorf("abriendo empresa %q (%s): %w", code, name, err)
		}
		err = persistence.RunMigrationsForMySQL(db, persistence.ConfigMySQL{
			Host: cfg.DBHost, Port: cfg.DBPort, Usuario: cfg.DBUser,
			Clave: cfg.DBPassword, Nombre: name,
		})
		_ = db.Close()
		if err != nil {
			return fmt.Errorf("migrando empresa %q: %w", code, err)
		}
		fmt.Printf("✓ %s (%s)\n", code, name)
	}
	fmt.Printf("Migraciones aplicadas a %d empresas.\n", len(codes))
	return nil
}

// createCompany registra una empresa nueva en el catálogo y prepara SU base.
//
// ERPGo NO crea bases de datos: en hosting compartido (cPanel, Plesk, etc.) el
// usuario de MySQL no tiene permiso de CREATE DATABASE y un intento de crearla
// termina con "Access denied ... Error 1044" ya en producción, que es el peor
// momento de todos. Lo que hace el wizard es PEDIR los datos de la base —que se
// crea desde el panel del servidor con su usuario exacto—, verificar la
// conexión, aplicar el esquema y registrar la empresa con las credenciales que
// el runtime usará en cada login.
func (w *Wizard) createCompany(cfg *configs.Config) error {
	master, err := w.openMasterDB(cfg)
	if err != nil {
		return fmt.Errorf("abriendo catálogo maestro: %w", err)
	}
	defer master.Close()
	name := strings.TrimSpace(w.askInput("Razón social", false))
	if name == "" {
		return fmt.Errorf("la razón social es obligatoria")
	}
	var rif string
	for {
		rif = normalizarRIF(w.askInput("RIF", false))
		if rif == "" {
			break // opcional
		}
		if verr := validarFormatoRIF(rif); verr != nil {
			fmt.Printf("    ✗ %v — intente de nuevo (Enter vacío para continuar sin RIF)\n", verr)
			continue
		}
		break
	}

	// Duplicados ANTES de pedir nada más: no tiene sentido llenar el resto del
	// formulario para descubrir al final que la empresa ya existe.
	if existenEmpresasIguales(master, name, rif) {
		return fmt.Errorf("la empresa ya está registrada en el sistema (misma razón social o RIF)\n" +
			"  Búsquela en el catálogo de empresas, o corrija el nombre/RIF")
	}

	username := strings.TrimSpace(w.askInput("Usuario administrador", false))
	if username == "" {
		return fmt.Errorf("el usuario administrador es obligatorio")
	}
	password := w.askInput("Contraseña del administrador", true)
	if len(password) < 8 {
		return fmt.Errorf("la contraseña debe tener al menos 8 caracteres")
	}

	slug := slugify(name)
	if slug == "" {
		return fmt.Errorf("la razón social no produce un código de empresa válido")
	}
	code := slug
	for suffix := 2; ; suffix++ {
		if esTenantMaestro(cfg, code) {
			code = fmt.Sprintf("%s_%d", slug, suffix)
			continue
		}
		var existsQty int
		if err := master.QueryRow(`SELECT COUNT(*) FROM tenants WHERE code=?`, code).Scan(&existsQty); err != nil {
			return fmt.Errorf("comprobando código de empresa: %w", err)
		}
		if existsQty == 0 {
			break
		}
		code = fmt.Sprintf("%s_%d", slug, suffix)
	}
	var userExists int
	if err := master.QueryRow(`SELECT COUNT(*) FROM user_login WHERE username=?`, username).Scan(&userExists); err != nil {
		return fmt.Errorf("comprobando el usuario administrador: %w", err)
	}
	if userExists != 0 {
		return fmt.Errorf("el usuario %q ya existe en el sistema; use un usuario nuevo", username)
	}

	// Base de datos: ERPGo no la crea, la SOLICITA. El nombre predeterminado
	// sigue la convención del sistema (erpgo_<código>) para que el runtime la
	// encuentre sin configuración extra; si el panel del hosting obliga a otro
	// nombre (prefijo del cuenta, etc.) se acepta y se registra: el runtime
	// usa el nombre guardado en el catálogo.
	basePorDefecto, err := persistence.NombreBaseEmpresa(code)
	if err != nil {
		return fmt.Errorf("nombre de base para %q: %w", code, err)
	}
	fmt.Println()
	fmt.Println("  ── Base de datos de la nueva empresa ──")
	fmt.Println("  ERPGo NO crea bases. Cree la base y su usuario ANTES en el panel")
	fmt.Println("  del servidor (cPanel, Plesk, ...) y colóquelas aquí, exactas.")
	dbHost := w.askInput(fmt.Sprintf("Host MySQL [Enter = %s]", cfg.DBHost), false)
	if dbHost == "" {
		dbHost = cfg.DBHost
	}
	dbPort := w.askInt("Puerto MySQL", cfg.DBPort)
	dbName := strings.TrimSpace(w.askInput(fmt.Sprintf("Nombre de la base [Enter = %s]", basePorDefecto), false))
	if dbName == "" {
		dbName = basePorDefecto
	}
	dbUser := strings.TrimSpace(w.askInput(fmt.Sprintf("Usuario de la base [Enter = %s]", cfg.DBUser), false))
	if dbUser == "" {
		dbUser = cfg.DBUser
	}
	dbPass := w.askInput("Contraseña de la base", true)
	if dbPass == "" {
		dbPass = cfg.DBPassword
	}

	dbCfg := persistence.ConfigMySQL{Host: dbHost, Port: dbPort, Usuario: dbUser, Clave: dbPass, Nombre: dbName}
	// Conectar SIN crear nada. OpenMySQL hace Ping sobre la base exacta: si no
	// existe o las credenciales están mal, el error es explícito y proviene
	// del servidor, no un fallo de permisos de CREATE disfrazado.
	db, err := persistence.OpenMySQL(dbCfg)
	if err != nil {
		return fmt.Errorf("no se pudo conectar a la base de la empresa (%s@%s:%d/%s): %v\n"+
			"  Verifique que la base existe en el panel y que el usuario tiene permisos completos sobre ella", dbName, dbUser, dbPort, dbHost, err)
	}
	defer db.Close()
	if err := persistence.RunMigrationsForMySQL(db, dbCfg); err != nil {
		return fmt.Errorf("migrando %s: %w", dbName, err)
	}
	if _, err := db.Exec(`INSERT INTO company_information (company_id, company_name, vat_no, status) VALUES ('1', ?, ?, 1)
		ON DUPLICATE KEY UPDATE company_name=VALUES(company_name), vat_no=VALUES(vat_no), status=1`, name, rif); err != nil {
		return fmt.Errorf("guardando datos de empresa: %w", err)
	}
	if err := persistence.InitializeAdminUser(db, "mysql", username, password); err != nil {
		return fmt.Errorf("creando administrador de empresa: %w", err)
	}
	if err := registerTenantAdmin(master, username, password); err != nil {
		return fmt.Errorf("registrando identidad global del administrador: %w", err)
	}

	// Credenciales por empresa: el runtime (openTenantDB) prefiere estas antes
	// que las globales, así que el login de esta empresa usa SU usuario MySQL.
	// Las claves viajan encriptadas (AES-256-GCM) con la clave derivada del
	// secreto JWT, igual que el aprovisionamiento web.
	dbPassEnc, err := security.Encrypt(dbPass, security.DeriveKey(cfg.MasterKey))
	if err != nil {
		return fmt.Errorf("encriptando credenciales de la empresa: %w", err)
	}

	tx, err := master.Begin()
	if err != nil {
		return fmt.Errorf("iniciando registro de empresa: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO tenants (code, legal_name, rif, status, created_at, db_user, db_password_enc, db_name)
		VALUES (?, ?, ?, 1, NOW(), ?, ?, ?)`, code, name, rif, dbUser, []byte(dbPassEnc), dbName); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("registrando empresa: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at) VALUES (?, ?, 2, 1, 1, NOW())`, code, username); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("registrando propietario: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO tenant_provision_log (tenant_code, action, detail, created_at) VALUES (?, 'create', ?, NOW())`,
		code, fmt.Sprintf("base=%s usuario_bd=%s host=%s:%d", dbName, dbUser, dbHost, dbPort)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("registrando bitácora de aprovisionamiento: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmando registro de empresa: %w", err)
	}
	// Archivo de conexión por empresa: config/empresas/<código>.env — la
	// segunda pata del esquema (además del catálogo). Si falla la escritura
	// no aborta: el catálogo ya guardó todo y es la fuente primaria.
	escribirArchivoEmpresa(cfg, code, configs.KindEmpresa, dbHost, dbPort, dbName, dbUser, dbPass)

	fmt.Printf("Empresa %q creada: código=%s, base=%s, admin=%s\n", name, code, dbName, username)
	return nil
}

// escribirArchivoEmpresa deja el archivo de conexión de UNA empresa en
// config/empresas/<código>.env, cifrado con el secreto JWT vigente. Es la
// alternativa ordenada por el dueño del sistema a engordar el .env por cada
// empresa: cada archivo dice QUÉ empresa es y QUÉ es (demo/empresa), y el
// runtime lo prefiere sobre el catálogo cuando existe.
// No fatal: si el disco falla (permisos, ruta), el catálogo sigue siendo la
// fuente principal y el wizard avisa.
func escribirArchivoEmpresa(cfg *configs.Config, code, kind, dbHost string, dbPort int, dbName, dbUser, dbPass string) {
	err := configs.SaveTenantConfigFile(configs.TenantConn{
		Code:   code,
		Kind:   kind,
		Host:   dbHost,
		Port:   dbPort,
		DBName: dbName,
		DBUser: dbUser,
	}, dbPass, cfg.MasterKey)
	if err != nil {
		fmt.Printf("   aviso: no se pudo escribir config/empresas/%s.env (%v) — el catálogo mantiene las credenciales\n", code, err)
		return
	}
	fmt.Printf("   archivo de conexión guardado: config/empresas/%s.env (clave cifrada)\n", code)
}

// createDemoTenant activa la empresa DEMO de práctica.
//
// La demo NO es una empresa más: es el aula del sistema (entrenamiento, guía
// de prueba, validación de una acción sin riesgo) y por eso:
//   - requiere SU PROPIA BASE de datos — servirse de la maestra pondría sus
//     datos junto a los de la empresa operativa, exactamente lo que una
//     demo NO puede hacer —,
//   - no pide datos de empresa: usa identidad de práctica y datos ejemplo,
//   - no crea usuarios: todo usuario autenticado entra por la excepción
//     is_demo del catálogo (CanAccessTenant),
//   - el runtime la marca con NOTA DEMO en cada vista y reporte.
func (w *Wizard) createDemoTenant(cfg *configs.Config) error {
	master, err := w.openMasterDB(cfg)
	if err != nil {
		return fmt.Errorf("abriendo catálogo maestro: %w", err)
	}
	defer master.Close()

	// Columnas que el catálogo moderno espera (idempotente: si ya están, el
	// ALTER duplicado se descarta).
	asegurarColumna(master, "tenants", "is_demo", "TINYINT(1) NOT NULL DEFAULT 0")
	asegurarColumna(master, "tenants", "db_name", "VARCHAR(64) NOT NULL DEFAULT ''")
	asegurarColumna(master, "tenants", "db_user", "VARCHAR(128) NOT NULL DEFAULT ''")
	asegurarColumna(master, "tenants", "db_password_enc", "VARBINARY(512) DEFAULT ''")

	var existe int
	if err := master.QueryRow(`SELECT COUNT(*) FROM tenants WHERE code='demo'`).Scan(&existe); err != nil {
		return fmt.Errorf("comprobando la demo en el catálogo: %w", err)
	}
	if existe != 0 {
		if _, err := master.Exec(`UPDATE tenants SET is_demo=1, status=1 WHERE code='demo'`); err != nil {
			return fmt.Errorf("reactivando la demo: %w", err)
		}
		fmt.Println("La empresa DEMO ya existía y fue reactivada.")
		return nil
	}

	fmt.Println()
	fmt.Println("  La empresa DEMO de práctica necesita SU PROPIA BASE de datos:")
	fmt.Println("  en el panel del hosting cree la base con su usuario y permisos,")
	fmt.Println("  y colóquelas aquí sus datos exactos. ERPGo NO crea bases.")
	dbHost := w.askInput(fmt.Sprintf("Host MySQL [Enter = %s]", cfg.DBHost), false)
	if dbHost == "" {
		dbHost = cfg.DBHost
	}
	dbPort := w.askInt("Puerto MySQL", cfg.DBPort)
	dbName := strings.TrimSpace(w.askInput(fmt.Sprintf("Nombre de la base de la DEMO [Enter = %s]", "erpgo_demo"), false))
	if dbName == "" {
		dbName = "erpgo_demo"
	}
	dbUser := strings.TrimSpace(w.askInput(fmt.Sprintf("Usuario de la base [Enter = %s]", cfg.DBUser), false))
	if dbUser == "" {
		dbUser = cfg.DBUser
	}
	dbPass := w.askInput("Contraseña de la base", true)
	if dbPass == "" {
		dbPass = cfg.DBPassword
	}

	dbCfg := persistence.ConfigMySQL{Host: dbHost, Port: dbPort, Usuario: dbUser, Clave: dbPass, Nombre: dbName}
	db, err := persistence.OpenMySQL(dbCfg)
	if err != nil {
		return fmt.Errorf("no se pudo conectar a la base de la DEMO (%s@%s:%d/%s): %v\n"+
			"  Cree la base en el panel del hosting y verifique permisos completos", dbName, dbUser, dbPort, dbHost, err)
	}
	defer db.Close()
	if err := persistence.RunMigrationsForMySQL(db, dbCfg); err != nil {
		return fmt.Errorf("migrando la DEMO: %w", err)
	}

	// Siembra de práctica: identidad de ejemplo (nunca datos de una empresa
	// real), ejercicio fiscal abierto y catálogos que las migraciones ya
	// dejaron. Sin usuarios: el acceso es por la excepción is_demo.
	if _, err := db.Exec(`INSERT INTO company_information (company_id, company_name, vat_no, status) VALUES ('1', 'Empresa DEMO (práctica)', 'D-00000000-0', 1)
		ON DUPLICATE KEY UPDATE company_name=VALUES(company_name), vat_no=VALUES(vat_no), status=1`); err != nil {
		return fmt.Errorf("sembrando identidad de la DEMO: %w", err)
	}
	anio := time.Now().Year()
	if _, err := db.Exec(`INSERT INTO financial_year (yearName, startDate, endDate, isCloseReq, status, created_by)
		VALUES (?, ?, ?, 0, 1, 1)
		ON DUPLICATE KEY UPDATE status=1`,
		fmt.Sprintf("%d", anio), fmt.Sprintf("%d-01-01", anio), fmt.Sprintf("%d-12-31", anio)); err != nil {
		logNoFatal("ejercicio fiscal de la DEMO", err)
	}

	dbPassEnc, err := security.Encrypt(dbPass, security.DeriveKey(cfg.MasterKey))
	if err != nil {
		return fmt.Errorf("encriptando credenciales de la DEMO: %w", err)
	}
	if _, err := master.Exec(`INSERT INTO tenants (code, legal_name, rif, status, created_at, is_demo, db_user, db_password_enc, db_name)
		VALUES ('demo', 'Empresa DEMO (práctica)', 'D-00000000-0', 1, NOW(), 1, ?, ?, ?)`,
		dbUser, []byte(dbPassEnc), dbName); err != nil {
		return fmt.Errorf("registrando la DEMO en el catálogo: %w", err)
	}
	if _, err := master.Exec(`INSERT INTO tenant_provision_log (tenant_code, action, detail, created_at)
		VALUES ('demo', 'demo', ?, NOW())`,
		fmt.Sprintf("base=%s usuario_bd=%s host=%s:%d", dbName, dbUser, dbHost, dbPort)); err != nil {
		logNoFatal("bitácora de aprovisionamiento de la DEMO", err)
	}
	// Archivo de conexión de la DEMO: la pata editable junto al binario.
	escribirArchivoEmpresa(cfg, "demo", configs.KindDemo, dbHost, dbPort, dbName, dbUser, dbPass)

	fmt.Printf("Empresa DEMO activada: base=%s, accesible para todos los usuarios en el selector\n", dbName)
	return nil
}

// actualizarCredencialesEmpresa repara el punto débil del Error 1044 al entrar
// a una empresa: el runtime conecta con las credenciales de la empresa
// (guardadas en el catálogo) y, si no están o no se pueden descifrar, cae al
// usuario global — que en hosting compartido no tiene acceso a la BD de esa
// empresa. Aquí el administrador las escribe DE NUEVO, se verifican contra el
// servidor REAL y quedan encriptadas con la clave JWT vigente del .env: esto
// arregla también una clave encriptada con un secreto distinto.
//
// No toca datos de negocio, ni migraciones, ni registro de la empresa: solo la
// fila del catálogo que dice dónde está su BD.
func (w *Wizard) actualizarCredencialesEmpresa(cfg *configs.Config) error {
	master, err := w.openMasterDB(cfg)
	if err != nil {
		return fmt.Errorf("abriendo catálogo maestro: %w", err)
	}
	defer master.Close()

	asegurarColumna(master, "tenants", "is_demo", "TINYINT(1) NOT NULL DEFAULT 0")
	asegurarColumna(master, "tenants", "db_name", "VARCHAR(64) NOT NULL DEFAULT ''")
	asegurarColumna(master, "tenants", "db_user", "VARCHAR(128) NOT NULL DEFAULT ''")
	asegurarColumna(master, "tenants", "db_password_enc", "VARBINARY(512) DEFAULT ''")

	// Lista de empresas para que el administrador vea el CÓDIGO exacto.
	fmt.Println()
	fmt.Println("  Empresas registradas:")
	rows, err := master.Query(`SELECT code, legal_name FROM tenants ORDER BY code`)
	if err != nil {
		return fmt.Errorf("leyendo empresas: %w", err)
	}
	for rows.Next() {
		var c, l string
		if rows.Scan(&c, &l) == nil {
			fmt.Printf("   · %s — %s\n", c, l)
		}
	}
	rows.Close()

	code := strings.TrimSpace(w.askInput("Código de la empresa", false))
	if code == "" {
		return fmt.Errorf("el código es obligatorio")
	}
	var codeExists int
	if err := master.QueryRow(`SELECT COUNT(*) FROM tenants WHERE code=?`, code).Scan(&codeExists); err != nil {
		return fmt.Errorf("comprobando la empresa: %w", err)
	}
	if codeExists == 0 {
		return fmt.Errorf("la empresa %q no está en el catálogo", code)
	}
	// La maestra usa DB_NAME directamente; sus credenciales son las del .env
	// y no se deben tocar aquí.
	if esTenantMaestro(cfg, code) {
		return fmt.Errorf("%q es la empresa maestra: usa las credenciales globales del .env y no se gestionan aquí", code)
	}

	baseDerived, err := persistence.NombreBaseEmpresa(code)
	if err != nil {
		return err
	}
	var dbNameActual string
	_ = master.QueryRow(`SELECT IFNULL(db_name,'') FROM tenants WHERE code=?`, code).Scan(&dbNameActual)
	if dbNameActual == "" {
		dbNameActual = baseDerived
	}

	dbName := strings.TrimSpace(w.askInput(fmt.Sprintf("Nombre de la base [Enter = %s]", dbNameActual), false))
	if dbName == "" {
		dbName = dbNameActual
	}
	dbUser := strings.TrimSpace(w.askInput(fmt.Sprintf("Usuario de la base [Enter = %s]", cfg.DBUser), false))
	if dbUser == "" {
		dbUser = cfg.DBUser
	}
	dbPass := w.askInput("Contraseña de la base (Enter = conservar la ya guardada)", true)

	// Si escribe contraseña nueva se puede PROBAR de verdad antes de guardar:
	// OpenMySQL pega (Ping) en la apertura, así que si conecta es porque la
	// base existe y el usuario tiene acceso; si no, el error se reporta ANTES
	// de escribir nada en el catálogo.
	if dbPass != "" {
		probe, perror := persistence.OpenMySQL(persistence.ConfigMySQL{
			Host: cfg.DBHost, Port: cfg.DBPort, Usuario: dbUser, Clave: dbPass, Nombre: dbName,
		})
		if perror != nil {
			return fmt.Errorf("la prueba de conexión falló (%s@%s:%d/%s): %v — nada se guardó", dbUser, cfg.DBHost, cfg.DBPort, dbName, perror)
		}
		_ = probe.Close()
	}

	if dbPass != "" {
		enc, err := security.Encrypt(dbPass, security.DeriveKey(cfg.MasterKey))
		if err != nil {
			return fmt.Errorf("encriptando la clave: %w", err)
		}
		if _, err := master.Exec(`UPDATE tenants SET db_user=?, db_name=?, db_password_enc=? WHERE code=?`,
			dbUser, dbName, []byte(enc), code); err != nil {
			return fmt.Errorf("guardando credenciales: %w", err)
		}
	} else {
		// Clave conservada: se actualizan usuario y nombre, la encriptada queda.
		if _, err := master.Exec(`UPDATE tenants SET db_user=?, db_name=? WHERE code=?`, dbUser, dbName, code); err != nil {
			return fmt.Errorf("guardando credenciales: %w", err)
		}
		fmt.Println("  Aviso: se conservó la clave ya guardada. Si el problema era la clave, repita con ella escrita.")
	}
	// El archivo de conexión debe quedar alineado con el catálogo tras la
	// actualización (cuando hay clave nueva va cifrada con el secreto actual).
	escribirArchivoEmpresa(cfg, code, configs.KindEmpresa, cfg.DBHost, cfg.DBPort, dbName, dbUser, dbPass)

	fmt.Printf("Credenciales de %q guardadas: base=%s, usuario=%s. Entra de nuevo a la empresa para verificar.\n", code, dbName, dbUser)
	return nil
}

// asegurarColumna ejecuta un ALTER ... ADD COLUMN y descarta el error cuando
// la columna ya existe. Es el patrón portable del esquema de control: sin
// «IF NOT EXISTS» en MySQL estándar, instalar en un catálogo nuevo y en uno
// viejo debe salir por el mismo camino.
func asegurarColumna(master *sql.DB, tabla, columna, definicion string) {
	if _, err := master.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", tabla, columna, definicion)); err != nil {
		// si falló por duplicado, la columna ya está: silencio. Cualquier otro
		// fallo quedará visible en la operación que la use.
		return
	}
}

// logNoFatal anota una siembra secundaria que no debe abortar la activación.
func logNoFatal(contexto string, err error) {
	fmt.Printf(" aviso: %s: %v (sigue sin esto)\n", contexto, err)
}

// existenEmpresasIguales busca una empresa registrada con el mismo nombre
// (insensible a mayúsculas) o el mismo RIF. Devuelve el detalle del conflicto
// para que el administrador sepa CUAL empresa pegó, sin adivinar.
func existenEmpresasIguales(master *sql.DB, name, rif string) bool {
	var dupes int
	if err := master.QueryRow(
		`SELECT COUNT(*) FROM tenants WHERE (rif<>'' AND rif=?) OR LOWER(TRIM(legal_name))=LOWER(TRIM(?))`,
		rif, name,
	).Scan(&dupes); err != nil || dupes == 0 {
		return false
	}
	rows, err := master.Query(
		`SELECT code, legal_name, rif FROM tenants WHERE (rif<>'' AND rif=?) OR LOWER(TRIM(legal_name))=LOWER(TRIM(?))`,
		rif, name,
	)
	if err == nil {
		defer rows.Close()
		fmt.Println("\n  ⚠  La empresa ya existe en el catálogo:")
		for rows.Next() {
			var c, l, r string
			if rows.Scan(&c, &l, &r) == nil {
				fmt.Printf("     · %s — %s (RIF: %s)\n", c, l, r)
			}
		}
		rows.Close()
	}
	return true
}

// normalizarRIF limpia separadores y va a mayúsculas, el mismo formato que usa
// la validación del núcleo, para que un RIF tecleado con guiones no lo tome
// como una empresa distinta.
func normalizarRIF(rif string) string {
	rif = strings.TrimSpace(rif)
	var b strings.Builder
	for _, r := range rif {
		switch r {
		case '.', '-', ' ':
			// separador fuera
		default:
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

// validarFormatoRIF aplica el algoritmo módulo 11 del SENIAT (los mismos pesos
// que la validación del núcleo en interfaces/web) para rechazar RIFs mal
// tecleados en el momento, y no cuando el primero factura.
func validarFormatoRIF(rif string) error {
	if rif == "" {
		return nil // el RIF es opcional
	}
	if len(rif) != 10 {
		return fmt.Errorf("el RIF debe tener 10 caracteres: letra + 9 dígitos (recibidos: %d)", len(rif))
	}
	letra := rif[0]
	switch letra {
	case 'J', 'G', 'V', 'E', 'P':
	default:
		return fmt.Errorf("el RIF debe iniciar con J, G, V, E o P (recibido: %c)", letra)
	}
	for i := 1; i < len(rif); i++ {
		if rif[i] < '0' || rif[i] > '9' {
			return fmt.Errorf("después de la letra el RIF debe tener solo dígitos (posición %d: %c)", i+1, rif[i])
		}
	}
	var sum int
	weights := [8]int{3, 2, 7, 6, 5, 4, 3, 2}
	for i := 0; i < 8; i++ {
		sum += int(rif[i+1]-'0') * weights[i]
	}
	dv := 11 - (sum % 11)
	if dv >= 10 {
		dv = 0
	}
	if dv != int(rif[9]-'0') {
		return fmt.Errorf("el dígito verificador del RIF no es válido")
	}
	return nil
}

func registerTenantAdmin(master *sql.DB, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("cifrando contraseña: %w", err)
	}
	tx, err := master.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var next int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(CAST(user_id AS UNSIGNED)), 0) + 1 FROM users`).Scan(&next); err != nil {
		return err
	}
	userID := fmt.Sprint(next)
	if _, err := tx.Exec(`INSERT INTO users (user_id, first_name, last_name, company_name, phone, status)
		VALUES (?, ?, '', 'ERPGo', '', 1)`, userID, username); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
		VALUES (?, ?, ?, 1, 1)`, userID, username, string(hash)); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *Wizard) askInput(label string, mask bool) string {
	fmt.Printf("  %s: ", label)
	if mask {
		// Leer del terminal sin eco si stdin es una consola. En pipes de CI o
		// scripts se usa el lector normal para mantener --setup automatizable.
		if value, err := readSecretFromTerminal(os.Stdin); err == nil {
			fmt.Println()
			return strings.TrimSpace(value)
		}
	}
	value, _ := w.reader.ReadString('\n')
	return strings.TrimSpace(value)
}

func readSecretFromTerminal(file *os.File) (string, error) {
	// Esta función evita dependencias de plataforma: sin termios disponible,
	// usa la entrada estándar normal y jamás falla la instalación.
	if file == nil {
		return "", io.ErrClosedPipe
	}
	return "", fmt.Errorf("lectura sin eco no disponible en esta compilación")
}

func (w *Wizard) askYesNo(question string) bool {
	fmt.Printf("  %s (s/n): ", question)
	answer, _ := w.reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "s" || answer == "si" || answer == "sí" || answer == "yes" || answer == "y"
}

func (w *Wizard) askInt(label string, defaultVal int) int {
	for {
		fmt.Printf("  %s [%d]: ", label, defaultVal)
		input, _ := w.reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			return defaultVal
		}
		if n, err := strconv.Atoi(input); err == nil {
			return n
		}
		fmt.Println("    Ingrese un número válido")
	}
}

func generateRandomSecret(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := make([]byte, length)
	for i := range b {
		n := make([]byte, 1)
		if _, err := rand.Read(n); err != nil {
			return ""
		}
		b[i] = charset[int(n[0])%len(charset)]
	}
	return string(b)
}
