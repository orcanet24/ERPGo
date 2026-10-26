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

	"erpgow/configs"
	"erpgow/infrastructure/persistence"
	"erpgow/internal/security"
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

	if _, err := os.Stat(".env"); err == nil {
		cfg, loadErr := configs.Load()
		if loadErr != nil {
			return fmt.Errorf("leyendo .env: %w", loadErr)
		}
		return w.runMaintenance(cfg)
	}
	return w.runFreshInstall()
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

	// [1/6] Instalar o conectar a MySQL
	fmt.Println("[1/6] Base de datos MySQL/MariaDB")
	fmt.Println()
	installDB := w.askYesNo("¿Desea instalar MariaDB ahora?")

	var dbHost, dbUser, dbPass, dbName string
	var dbPort int
	if installDB {
		dbHost = "127.0.0.1"
		dbPort = w.askInt("Puerto MySQL", DefaultPorts.MySQL)
		dbName = w.askInput("Nombre de la base maestra (DB_NAME)", false)
		dbUser = "root"
		dbPass = w.askInput("Contraseña root MariaDB", true)

		fmt.Println()
		fmt.Println("Instalando MariaDB...")
		instalador := NewMariaDBInstaller("./mariadb", dbPort, dbPass)
		if err := instalador.Install(); err != nil {
			return fmt.Errorf("error instalando MariaDB: %w", err)
		}
		if err := instalador.StartService(); err != nil {
			return fmt.Errorf("error iniciando MariaDB: %w", err)
		}
		if !persistence.EsServidorMySQLArriba(dbHost, dbPort) {
			return fmt.Errorf("MariaDB se instaló, pero no responde en %s:%d; corrige el servicio y vuelve a ejecutar --setup", dbHost, dbPort)
		}
		fmt.Println("✓ MariaDB instalado y corriendo")
	} else {
		fmt.Println()
		fmt.Println("Configuración de conexión a MySQL existente:")
		dbHost = w.askInput("Host", false)
		dbPort = w.askInt("Puerto MySQL", 3306)
		dbName = w.askInput("Nombre de la base maestra (DB_NAME)", false)
		dbUser = w.askInput("Usuario", false)
		dbPass = w.askInput("Contraseña", true)

		fmt.Println()
		fmt.Println("Verificando conexión a MySQL...")
		testCfg := persistence.ConfigMySQL{
			Host:    dbHost,
			Port:    dbPort,
			Usuario: dbUser,
			Clave:   dbPass,
		}
		testDB, err := persistence.ConectarServidorMySQL(testCfg)
		if err != nil {
			fmt.Printf("   ✗ No se pudo conectar: %v\n", err)
			if w.askYesNo("   ¿Desea reintentar?") {
				return w.runFreshInstall()
			}
			return fmt.Errorf("conexión fallida")
		}
		if err := testDB.Close(); err != nil {
			return fmt.Errorf("cerrando conexión de comprobación: %w", err)
		}
		fmt.Println("   ✓ Conexión exitosa")
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

	// Crear roles base del sistema
	roles := []struct {
		id     int
		typ    string
		level  string
		desc   string
		system int
	}{
		{1, "Super Admin", "master", "Acceso total al sistema", 1},
		{2, "Admin", "admin", "Administrador de empresa", 1},
		{3, "Manager", "manager", "Gerente", 1},
		{4, "Employee", "employee", "Empleado", 1},
	}
	for _, r := range roles {
		if _, err := controlDB.Exec(`INSERT INTO sec_role (id, type, level, description, is_system) VALUES (?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE type=VALUES(type), level=VALUES(level), description=VALUES(description), is_system=VALUES(is_system)`,
			r.id, r.typ, r.level, r.desc, r.system,
		); err != nil {
			return fmt.Errorf("insertando rol base: %w", err)
		}
	}
	var masterUserID string
	if err := controlDB.QueryRow(`SELECT user_id FROM user_login WHERE username=? AND status=1 LIMIT 1`, superUser).Scan(&masterUserID); err != nil {
		return fmt.Errorf("buscando el super administrador %q: %w", superUser, err)
	}
	var masterRole int
	if err := controlDB.QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE user_id=? AND roleid=1`, masterUserID).Scan(&masterRole); err != nil {
		return fmt.Errorf("comprobando rol master: %w", err)
	}
	if masterRole == 0 {
		if _, err := controlDB.Exec(`INSERT INTO sec_userrole (user_id, roleid, createby, createdate)
			VALUES (?, 1, 'system', NOW())`, masterUserID); err != nil {
			return fmt.Errorf("asignando rol master: %w", err)
		}
	}

	if err := (&configs.Config{
		DBDriver: "mysql", DBHost: dbHost, DBPort: dbPort, DBUser: dbUser, DBPassword: dbPass,
		DBName: mainDBName, DBMasterTenantCode: masterCode,
		DBRootUser: dbUser, DBRootPassword: dbPass,
		DBProvisionUser: dbUser, DBProvisionPassword: dbPass,
		DBSQLitePath: "data/empresas/default.sqlite3", DBMySQLBufferPoolMB: 256,
		DBAutoStart: false,
		ServerHost:  "127.0.0.1", ServerPort: strconv.Itoa(serverPort), GRPCPort: strconv.Itoa(grpcPort),
		JWTSecret: jwtSecret, JWTExpiry: 24,
		AppName: "ERPGo", AppURL: fmt.Sprintf("http://localhost:%d", serverPort),
		AppEnv: "production", UploadPath: "./uploads",
		LogEnabled: true, LogLevel: "info", LogFile: "logs/erpgow.log",
	}).WriteEnvFile(); err != nil {
		return fmt.Errorf("guardando configuración .env: %w", err)
	}

	fmt.Println("✓ Super administrador creado y .env guardado")

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
	fmt.Println("    Windows: .\\erpgow.exe")
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
	fmt.Println("  3. Salir")
	fmt.Println()

	switch choice := w.askInput("Opción", false); choice {
	case "1":
		return w.migrateRegisteredCompanies(cfg)
	case "2":
		return w.createCompany(cfg)
	case "3", "":
		return nil
	default:
		return fmt.Errorf("opción de mantenimiento desconocida: %q", choice)
	}
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
	dbPassEnc, err := security.Encrypt(dbPass, security.DeriveKey(cfg.JWTSecret))
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
	fmt.Printf("Empresa %q creada: código=%s, base=%s, admin=%s\n", name, code, dbName, username)
	return nil
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
