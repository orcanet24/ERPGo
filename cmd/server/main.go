// ERPGo — ERP Fiscal para PYMEs Venezolanas
// Distribuido por Orcanet1724 C.A.
//
// POLÍTICA DE ORDEN Y CRECIMIENTO (NO NEGOCIABLE):
// 1. Los datos existentes son sagrado — NO se tocan directamente, solo mediante migraciones idempotentes.
// 2. Migraciones: única vía de cambio — CREATE IF NOT EXISTS, ALTER ADD COLUMN IF NOT EXISTS, INSERT ON DUPLICATE KEY.
// 3. Orden en el tiempo — IDs fijos, registros históricos inmutables, obsoletos se marcan inactivos (nunca eliminan).
// 4. Crecimiento modular — capas adicionales que referencian por ID, nunca reescriben lo existente.
// 5. Regla de oro — Si un cambio rompe una BD existente o requiere intervención manual, NO se hace.
//
// Esta política rige todo desarrollo futuro. Violarla rompe la promesa de estabilidad.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"erpgo/configs"
	"erpgo/infrastructure/persistence"
	"erpgo/interfaces/web"
	"erpgo/internal/logger"
	"erpgo/internal/pos"
	"erpgo/internal/service"
	"erpgo/internal/setup"
)

// resolvePath finds a directory relative to the executable or working dir.
// It checks: executable dir, working dir, and parent dirs up to 3 levels.
func resolvePath(name string) string {
	// Try relative to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidate := filepath.Join(exeDir, name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	// Try relative to working dir
	if wd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(wd, name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		// Try parent dirs (up to 3 levels)
		dir := wd
		for i := 0; i < 3; i++ {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			candidate := filepath.Join(parent, name)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
			dir = parent
		}
	}
	// Fallback to relative (will fail at runtime if not found)
	return name
}

func main() {
	// Usage:
	//   erpgo.exe              -> start server (schema applied automatically)
	//   erpgo.exe --setup      -> run setup wizard (fresh install / maintenance)
	//   erpgo.exe --migrate    -> apply schema and exit
	//   erpgo.exe --install    -> install as Windows service (auto-start, restart on failure)
	//   erpgo.exe --uninstall  -> stop and remove Windows service
	migrate := flag.Bool("migrate", false, "Apply migrations and exit")
	install := flag.Bool("install", false, "Install ERPGo as a Windows service")
	uninstall := flag.Bool("uninstall", false, "Uninstall the ERPGo Windows service")
	setupFlag := flag.Bool("setup", false, "Run setup wizard")
	flag.Parse()

	// --setup runs the interactive installer.
	// It can work without existing .env (fresh install) or with existing config (maintenance).
	if *setupFlag {
		fmt.Println("ERPGo - Setup Wizard")
		fmt.Println("=====================")
		fmt.Println()

		wizard := setup.NewWizard()
		if err := wizard.Run(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		fmt.Println("Setup complete.")
		return
	}

	// --install y --uninstall no requieren BD ni config completa: se manejan
	// directamente con el SCM y salen.
	if *install {
		fmt.Printf("Instalando servicio Windows %q...\n", service.ServiceName)
		if err := service.InstallService("", "ERPGo Server", "Servidor web ERPGo"); err != nil {
			log.Fatalf("Error instalando servicio: %v", err)
		}
		fmt.Printf("Servicio instalado correctamente.\n")
		fmt.Printf("Para arrancar:   net start %s\n", service.ServiceName)
		fmt.Printf("Para detener:    net stop %s\n", service.ServiceName)
		fmt.Printf("Para desinstalar: erpgo.exe --uninstall\n")
		return
	}
	if *uninstall {
		fmt.Printf("Desinstalando servicio Windows %q...\n", service.ServiceName)
		if err := service.UninstallService(""); err != nil {
			log.Fatalf("Error desinstalando servicio: %v", err)
		}
		fmt.Printf("Servicio eliminado.\n")
		return
	}

	cfg, err := configs.EnsureEnv()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Logger configurable desde .env
	logger.Init(cfg.LogEnabled, cfg.LogLevel, cfg.LogFile)
	defer logger.Close()

	logger.Info("ERPGo arrancando → %s:%d (empresa: %s)", cfg.DBHost, cfg.DBPort, cfg.DBName)

	// MySQL es el backend. Hay UNA BASE POR EMPRESA (erpgo_<codigo>), y una
	// base de CONTROL (erpgo) que lleva el catálogo de empresas.
	//
	// En la primera ejecución no existe nada: se crea la base de control, se
	// aplica el esquema y se siembra la empresa por defecto. De ahí en adelante
	// solo se abre. Por eso instalar es copiar el binario y arrancarlo.

	root := cfg.DataPath()
	if err := os.MkdirAll(filepath.Join(root, "empresas"), 0o755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// EL MOTOR DE BASE DE DATOS ES DEL SERVIDOR, NO DEL ERP.
	//
	// Decisión firme: ERPGo NO instala, NO inicializa y NO arranca el motor de
	// base de datos. Antes este bloque, al no encontrar un servidor escuchando
	// en DB_PORT, ejecutaba `mysqld --initialize-insecure` con un directorio de
	// datos propio y levantaba un MySQL desde el binario del ERP, para apagarlo
	// al terminar. Eso creaba una instalación de base de datos a espaldas de
	// quien administra la máquina y fue justo lo que rompió servidores que ya
	// estaban sanos.
	//
	// El motor es infraestructura: se instala y se arranca como servicio del
	// sistema (systemd en Linux, servicio de Windows en Windows). El ERP solo se
	// conecta. Si no responde, se detiene aquí con un mensaje que dice qué
	// revisar, sin tocar absolutamente nada del sistema operativo.
	if !persistence.EsServidorMySQLArriba(cfg.DBHost, cfg.DBPort) {
		log.Fatalf(
			"No hay ningún servidor MySQL/MariaDB escuchando en %s:%d.\n"+
				"  ERPGo no instala el motor de base de datos: debe existir ANTES de arrancar.\n"+
				"  - Instale MySQL o MariaDB como servicio del sistema (Linux: paquete del\n"+
				"    sistema; Windows: instalador oficial) y arránquelo.\n"+
				"  - Compruebe que DB_HOST y DB_PORT en el .env son los del servidor\n"+
				"    (ambos motores usan 3306 por defecto).\n"+
				"  - Si la base está en otra máquina, abra el puerto y habilite el usuario remoto.",
			cfg.DBHost, cfg.DBPort)
	}

	fmt.Printf("Database: MySQL en %s:%d (base de control: %s)\n",
		cfg.DBHost, cfg.DBPort, cfg.DBName)

	// CREAR LA BASE DE CONTROL SI NO EXISTE.
	//
	// Va antes de abrir nada porque en MySQL no se puede crear una base
	// dentro de otra base: hay que hablarle al servidor sin base seleccionada.
	// Es también lo que hace que la primera ejecución no necesite que nadie
	// haya creado las tablas a mano.
	if err := persistence.EnsureDatabase(persistence.ConfigMySQL{
		Host:    cfg.DBHost,
		Port:    cfg.DBPort,
		Usuario: cfg.EffectiveRootUser(),
		Clave:   cfg.EffectiveRootPassword(),
		Nombre:  cfg.DBName,
	}); err != nil {
		log.Fatalf("No se pudo preparar la base de control: %v", err)
	}

	db, err := persistence.OpenMySQL(persistence.ConfigMySQL{
		Host:    cfg.DBHost,
		Port:    cfg.DBPort,
		Usuario: cfg.DBUser,
		Clave:   cfg.DBPassword,
		Nombre:  cfg.DBName,
	})
	if err != nil {
		log.Fatalf("Failed to open MySQL database: %v", err)
	}
	defer db.Close()

	// La FieldKey del binario se pasa a la capa web ANTES de construir el
	// servidor, porque el orquestador de escrituras arranca DENTRO de NewServer
	// y cifra su journal con ella. Sin esto el journal se creaba en clave…
	web.SetFieldKey(fieldKey())

	// El esquema NO se aplica aquí. Los DATOS de negocio viven en la base de cada
	// empresa (web.PrepareCompany… no siembra negocio en control), y cada empresa
	// tiene su base. El ESQUEMA sí es el mismo en las dos clases de base, y se
	// aplica en un solo sitio: web.NewServer → controlStore, porque en la base de
	// control viven los usuarios y sus credenciales (el login lee de ahí) y sin sus
	// tablas —user_login, users— nadie puede entrar; PrepareDefaultCompany
	// crea/migra/siembra después la base de la empresa por defecto, copiando antes
	// los datos heredados si la instalación es vieja. Así «default» es una empresa
	// más, y el catálogo deja de mezclar datos sin quedarse sin identidad.
	server := web.NewServer(cfg, db)

	// Iniciar polling de estado Cashea cada 30s como alternativa a los webhooks
	// Esta es la estrategia: consultar la API Cashea periódicamente (cada 30s)
	// en lugar de depender de webhooks, que pueden no estar configurados o tener
	// costo adicional. El polling es 'best effort' - si falla, el sistema continúa
	// con el estado local y vuelve a intentar en el siguiente ciclo.
	//
	// StartCasheaStatusPoller NO bloquea (el ciclo corre en gorutina propia):
	// antes fue un `for range ticker.C` llamado en serie y main se colgaba aquí,
	// con el ERP vivo pero sin abrir el puerto HTTP (8586).
	server.StartCasheaStatusPoller()

	// If migrate-only mode, exit after migrations
	if *migrate {
		fmt.Println("Migrations applied. Exiting.")
		_ = server.Close()
		os.Exit(0)
	}

	// Comprobación de integridad del binario. Va DESPUÉS de --migrate y ANTES de
	// levantar el servidor: si el ejecutable ha cambiado, no tiene sentido que
	// atienda peticiones de un cliente.
	//
	// Qué protege y qué NO, sin adornos: detecta corrupción y sustitución
	// accidental. NO protege de un atacante con acceso de escritura al disco,
	// porque esta comprobación vive DENTRO del binario que comprueba: quien pueda
	// parchar el ejecutable puede parchear también la comprobación. La firma de
	// código de Windows (Authenticode) sí sería una frontera, y es otra decisión
	// (S-3 en doc/SEGURIDAD.md).
	//
	// Por eso un fallo es AVISO y no parada. Sin sello —que es el caso normal en
	// desarrollo, donde se compila sin el script protegido— no se dice nada:
	// parar el ERP porque falta un archivo sería dejar al usuario sin su
	// aplicación.
	if sello := buscarSello(); sello != "" {
		if err := verificarIntegridad(sello); err != nil {
			log.Printf("AVISO: comprobación de integridad fallida: %v", err)
			log.Printf("       Si no esperabas esto, el binario cambió desde que se compiló.")
		} else {
			fmt.Println("Integridad: OK (sello SHA-256 correcto)")
		}
	}

	// Verificar firma Ed25519 si hay llave pública embebida (compilación protegida).
	if err := verificarFirmaEd25519(); err != nil {
		log.Printf("AVISO: firma Ed25519 inválida: %v", err)
	} else if pubKeyEd25519 != "" {
		fmt.Println("Firma Ed25519: OK")
	}

	// Start gRPC server for POS terminals (port 9095).
	//
	// Cada caja POS se autentica con SU API key, y la key resuelve la base de
	// SU empresa (server.DBForAPIKey): las ventas de las cajas de la empresa B
	// acaban en la base de la empresa B, no en la que estaba activa al arrancar.
	grpcServer := pos.NewGRPCServer(server.DBForAPIKey, func(action, module, table, recordID string, oldVal, newVal interface{}) {
		log.Printf("[AUDIT] %s %s %s %s", action, module, table, recordID)
	})

	// Inyectar la función fiscal central en el servidor gRPC del POS.
	// Sin esto, el POS fabrica su propio número de control ("POS-" + offlineId)
	// y múltiples facturas comparten el mismo número — inválido para SENIAT.
	// Con esto, cada venta POS consume la serie fiscal central dentro de la
	// misma transacción, garantizando correlativo único e ininterrumpido.
	grpcServer.SetFiscalDocFunc(func(tx *sql.Tx, series string) (int64, string, string, error) {
		doc, err := server.NextFiscalDoc(tx, series)
		if err != nil {
			return 0, "", "", err
		}
		return doc.Number, doc.DocNo, doc.ControlNo, nil
	})

	// El motor único de registro de ventas (interfaces/web/venta_terminal.go)
	// se inyecta también en gRPC (F-2, auditoría 2026-11-22): el proto no
	// viaja pagos ni tasa porque el camino vivo del terminal es HTTP, y aun
	// así ambas semánticas quedan idénticas —asiento, inventario,
	// idempotencia por reclamo— porque es LA MISMA función. Sin registrar
	// inyectado, gRPC se niega a facturar: dos implementaciones parciales
	// para el mismo cobro fue el origen del libro sin cajas.
	grpcServer.SetSaleRegistrar(server.RegistrarVentaTerminal)

	// Verificador de usuario y contraseña del POS (Login gRPC).
	//
	// Se inyecta la verificación del servidor web en vez de reimplementarla
	// aquí: los formatos heredados de contraseña (md5, texto plano) y su
	// reescritura a bcrypt son la misma regla de seguridad en los dos caminos,
	// y la segunda copia sería la que se quedara vieja. Sin esto, Login rechaza
	// —no inventa una sesión— porque el servidor gRPC arranca sin verificador.
	grpcServer.SetCredentialVerifier(server.VerifyPOSCredentials)
	// Toda la preparación de arriba (config, MySQL, migraciones, gRPC) es
	// común a ambos modos de arranque. Lo único que cambia es CÓMO se atiende
	// el ciclo de vida del proceso: si nos lanzó el SCM de Windows, hay que
	// hacer el protocolo de servicio (o el arranque falla con Error 1053); si
	// no, se arranca en consola como siempre (doble clic, iniciar.bat).
	app := &erpApplication{cfg: cfg, server: server, grpcSrv: grpcServer}

	if service.IsWindowsService() {
		// Servicio de Windows. El log va a un fichero: no hay consola donde
		// verlo, y sin esto un fallo de arranque sería invisible.
		redirigirLogAServicio()
		if err := service.RunService(app); err != nil {
			log.Fatalf("Servicio ERPGo: %v", err)
		}
		return
	}

	// Modo consola.
	if err := app.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
	// Se espera Ctrl+C y se apaga ordenado (antes el proceso se cortaba en
	// seco al morir la consola).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nApagando ordenadamente...")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	_ = app.Stop(ctx)
	cancel()
}

// erpApplication agrupa los servidores para poder arrancarlos y pararlos como
// una sola unidad, igual en modo consola que bajo el SCM de Windows.
type erpApplication struct {
	cfg     *configs.Config
	server  *web.Server
	grpcSrv *pos.GRPCServer
}

// Start lanza el gRPC del POS en segundo plano y se bloquea en el HTTP.
func (a *erpApplication) Start() error {
	// Servidor gRPC para las cajas POS (puerto 9095). Cada caja se autentica
	// con SU API key, que resuelve la base de SU empresa.
	go func() {
		if err := a.grpcSrv.Start(":" + a.cfg.GRPCPort); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	addr := strings.TrimSuffix(a.cfg.ServerHost+":"+a.cfg.ServerPort, ":")
	if addr == "" {
		addr = a.cfg.ServerHost + ":" + a.cfg.ServerPort
	}
	fmt.Printf("ERPGo server starting on http://%s\n", addr)
	return a.server.Start()
}

// Stop detiene todo de forma ordenada: primero el gRPC (cierra sesiones POS),
// luego drena el HTTP y por último cierra bases y orquestadores.
func (a *erpApplication) Stop(ctx context.Context) error {
	a.grpcSrv.Stop()
	var firstErr error
	if err := a.server.Shutdown(ctx); err != nil {
		firstErr = err
		log.Printf("apagado HTTP: %v", err)
	}
	if err := a.server.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// redirigirLogAServicio manda la salida de `log` a logs/erpgow-service.log,
// porque un servicio de Windows no tiene consola donde ver los errores.
func redirigirLogAServicio() {
	logDir := "logs"
	if exe, err := os.Executable(); err == nil {
		logDir = filepath.Join(filepath.Dir(exe), "logs")
	}
	_ = os.MkdirAll(logDir, 0o755)
	f, err := os.OpenFile(filepath.Join(logDir, "erpgow-service.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
}

// retireSQLiteFile moves a leftover SQLite file (from a previous SQLite
// deployment or a just-finished migration) into data/backup/, so a MySQL
// startup never silently keeps writing to the old file.
func retireSQLiteFile(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		// Not there under its own name? The migration flow may have left it
		// as path+".bak" (Windows keeps the open handle locked): archive it
		// now so the root directory is left clean.
		retireSQLiteFile(path + ".bak")
		return
	}
	os.MkdirAll("data/backup", 0755)
	stamp := time.Now().Format("2006-01-02_15-04-05")
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	ext := filepath.Ext(base)
	dst := filepath.Join("data", "backup", name+"-retired-"+stamp+ext)
	if err := os.Rename(path, dst); err != nil {
		log.Printf("Warning: could not retire SQLite file %s (%v)", path, err)
		return
	}
	fmt.Printf("Retired SQLite file: %s -> %s\n", path, dst)
	// WAL sidecar files are meaningless once the main file is gone: drop them.
	if ext == ".sqlite3" || ext == ".db" || ext == ".sqlite" {
		for _, side := range []string{ext + "-wal", ext + "-shm"} {
			if err := os.Remove(path + side); err == nil {
				fmt.Printf("Removed SQLite sidecar: %s\n", path+side)
			}
		}
	}
}
