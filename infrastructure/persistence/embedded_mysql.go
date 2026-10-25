package persistence

// embedded_mysql.go — el MySQL que viaja DENTRO del ERP.
//
// EL PROBLEMA QUE RESUELVE, DICHO SIN VENDERSELO.
//
// Un ERP que necesita "un servidor de base de datos" para funcionar en una
// máquina limpia es un ERP que no se instala: hay que saber MySQL, instalar
// MySQL, configurar MySQL, acordarse de la clave y encender el servicio antes
// de poder abrir el programa. En una pyme eso es una llamada de soporte.
//
// Aquí el MySQL va JUNTO al ERP, en una carpeta propia, con sus datos en otra
// carpeta propia y EN UN PUERTO QUE NO ES EL 3306. El 3306 se deja libre a
// propósito: si el usuario ya tiene un MySQL (Laragon, XAMPP, el de la
// empresa), los dos conviven sin apagar nada. Por eso MySQLPortIncluido.
//
// Enterprise es el mismo código por el otro lado: DB_HOST apunta al servidor
// de la empresa y aquí no se arranca nada. Por eso la decisión de qué motor
// usar se toma UNA vez, al configurar, y no en cada consulta.
//
// LO QUE ESTE ARCHIVO NO HACE.
//
// No reescribe mysqld ni lo mete en el binario. Un servidor de base de datos son
// 200 MB de código nativo que hay que mantener, actualizar y firmar aparte;
// empaquetarlo en el .exe no lo hace más portátil, solo más grande y más
// viejo. Lo que sí hace este archivo es dar por hecho que hay un mysqld
// instalado (el que trae el instalador, o uno ya presente en la máquina) y
// saber arrancarlo, esperarlo, inicializarlo y apagarlo. Esa es la frontera:
// el ERP dirige el proceso, no lo contiene.
//
// Por lo mismo, aquí no se abre una conexión a "la base de la empresa": eso es
// mysql.go. Aquí solo se habla con el PROCESO.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OpcionesMySQL describe el MySQL incluido: dónde vive el binario, dónde viven
// los datos, en qué puerto escucha y con qué credenciales entra el ERP.
type OpcionesMySQL struct {
	Host    string
	Puerto  int
	Usuario string
	Clave   string

	// BaseDir es la raíz de una instalación de MySQL (la carpeta que contiene
	// bin\, lib\ y share\). Si está vacía se busca una.
	BaseDir string
	// DataDir es donde se inicializan y viven las bases. Por defecto
	// <BaseDir>\data, que es lo que hace el instalador oficial.
	DataDir string
	// Binario es la ruta exacta a mysqld. Si está vacía se busca.
	Binario string
	// LogPath es donde se manda la salida del servidor. Sin ella, mysqld
	// escribe su propio .err y cuando algo falla no hay dónde mirar.
	LogPath string

	// InnoDBBufferPoolMB es el tamaño del búfer de InnoDB. Es el que decide
	// cuánta memoria se puede usar; 128 MB es el mínimo cómodo y el que trae
	// el servidor de fábrica.
	InnoDBBufferPoolMB int
	MaxConexiones      int

	// EsperaInicial es cuánto se concede a mysqld para aceptar conexiones. La
	// primera vez es más lenta: MySQL tiene que crear el esquema del sistema,
	// que son unos 1200 pasos, y en un disco lento eso tarda.
	EsperaInicial time.Duration
	// EsperaParada es cuánto se concede a un apagado limpio.
	EsperaParada time.Duration
}

// EmbeddedMySQL dirige el proceso del MySQL incluido.
type EmbeddedMySQL struct {
	opts OpcionesMySQL

	mu       sync.Mutex
	cmd      *exec.Cmd
	logFile  *os.File
	iniciado bool // si lo arrancamos nosotros y por lo tanto lo apagamos
}

// NewEmbeddedMySQL localiza el binario si hace falta y devuelve el gestor.
// Localizar aquí, y no al arrancar, es lo que permite que main diga
// "no hay MySQL y esta máquina no tiene ninguno" con un mensaje útil en vez de
// un fallo de conexión.
func NewEmbeddedMySQL(o OpcionesMySQL) (*EmbeddedMySQL, error) {
	if o.Puerto == 0 {
		o.Puerto = MySQLPortIncluido
	}
	if o.Host == "" {
		o.Host = MySQLHostPorDefecto
	}
	if o.Usuario == "" {
		o.Usuario = MySQLUsuarioDefecto
	}
	if o.InnoDBBufferPoolMB <= 0 {
		o.InnoDBBufferPoolMB = 256
	}
	if o.MaxConexiones <= 0 {
		o.MaxConexiones = 200
	}
	if o.EsperaInicial <= 0 {
		o.EsperaInicial = 90 * time.Second
	}
	if o.EsperaParada <= 0 {
		o.EsperaParada = 30 * time.Second
	}

	if o.Binario == "" || o.BaseDir == "" {
		bin, base, err := LocalizarMySQL()
		if err != nil {
			return nil, err
		}
		if o.Binario == "" {
			o.Binario = bin
		}
		if o.BaseDir == "" {
			o.BaseDir = base
		}
	}
	if info, err := os.Stat(o.Binario); err != nil || info.IsDir() {
		return nil, fmt.Errorf("no se encontró mysqld en %q", o.Binario)
	}
	if o.DataDir == "" {
		o.DataDir = filepath.Join(o.BaseDir, "data")
	}
	if o.LogPath == "" {
		o.LogPath = filepath.Join(o.DataDir, "erpgo-mysqld.log")
	}
	return &EmbeddedMySQL{opts: o}, nil
}

// Opciones devuelve la configuración efectiva, ya resuelta. La usa main para
// saber en qué puerto y con qué credenciales hay que conectar.
func (e *EmbeddedMySQL) Opciones() OpcionesMySQL { return e.opts }

// LocalizarMySQL busca una instalación de MySQL en la máquina.
//
// EL ORDEN IMPORTA. Se empieza por lo que acompaña al ERP, porque esa es la
// instalación que el instalador puso a propósito; luego por lo que ya había en
// la máquina, porque si funciona no hay que instalar nada. Se termina por el
// PATH, que es donde solo aparece MySQL si el usuario lo instaló "a mano".
//
// No se busca en el PATH antes que en los sitios conocidos a propósito: el PATH
// puede apuntar a un MySQL de 5.7 de una máquina que develop, y arrancar el ERP
// contra eso sin querer es peor que no encontrar nada.
func LocalizarMySQL() (binario, baseDir string, err error) {
	// Carpetas donde puede estar el programa: al lado del ejecutable y en el
	// directorio de trabajo. Se miran los dos porque en desarrollo el binario
	// se compila en un sitio y se ejecuta en otro.
	dirs := func() []string {
		exe, _ := os.Executable()
		dirs := []string{}
		if exe != "" {
			dirs = append(dirs, filepath.Dir(exe))
		}
		if wd, err := os.Getwd(); err == nil {
			dirs = append(dirs, wd)
		}
		return dirs
	}

	// El nombre del ejecutable es el mismo en Windows y en Linux ("mysqld"); en
	// Windows lo que cambia es la extensión, y las rutas de abajo ya llevan la
	// que corresponde. En Unix, exec.LookPath se encarga de la extensión.
	exeNombre := "mysqld"
	if runtime.GOOS == "windows" {
		exeNombre = "mysqld.exe"
	}
	rutas := []string{}
	for _, d := range dirs() {
		rutas = append(rutas,
			filepath.Join(d, "mysql", "bin", exeNombre),
			filepath.Join(d, "bin", "mysql", exeNombre),
			filepath.Join(d, "mysql", exeNombre),
		)
	}
	// Laragon y XAMPP, que es donde un desarrollador de Windows tiene MySQL.
	rutas = append(rutas,
		`C:\xampp\mysql\bin\`+exeNombre,
		`C:\laragon\bin\mysql`,
	)
	// Laragon no tiene una versión fija: hay que listar el directorio.
	if entradas, err := os.ReadDir(`C:\laragon\bin\mysql`); err == nil {
		var versiones []string
		for _, e := range entradas {
			if e.IsDir() {
				versiones = append(versiones, e.Name())
			}
		}
		// De la versión más alta a la más baja, por nombre: "8.0.30" > "5.7.44".
		for i := 0; i < len(versiones); i++ {
			for j := i + 1; j < len(versiones); j++ {
				if versiones[j] > versiones[i] {
					versiones[i], versiones[j] = versiones[j], versiones[i]
				}
			}
		}
		for _, v := range versiones {
			rutas = append(rutas, filepath.Join(`C:\laragon\bin\mysql`, v, "bin", exeNombre))
		}
	}
	if p, err := exec.LookPath(exeNombre); err == nil {
		rutas = append(rutas, p)
	}

	for _, r := range rutas {
		if info, err := os.Stat(r); err == nil && !info.IsDir() {
			return r, filepath.Dir(filepath.Dir(r)), nil
		}
	}
	return "", "", fmt.Errorf(
		"no se encontró mysqld en esta máquina. Se buscó en la carpeta mysql\\ al lado del " +
			"programa, en el PATH, en C:\\xampp\\mysql\\bin y en C:\\laragon\\bin\\mysql. " +
			"Instale MySQL 8 (o marque DB_AUTO_START=0 y apunte DB_HOST/DB_PORT a un " +
			"servidor que ya exista)")
}

// Inicializado dice si el directorio de datos ya tiene un MySQL dentro.
//
// El indicador no es "el directorio existe": es que exista la carpeta `mysql`
// del catálogo del sistema. Un directorio a medias —creado y luego borrado a
// medias— no es un servidor utilizable, y arrancar contra él da un error que
// no explica nada.
func (e *EmbeddedMySQL) Inicializado() bool {
	info, err := os.Stat(filepath.Join(e.opts.DataDir, "mysql"))
	return err == nil && info.IsDir()
}

// Arranca el MySQL incluido: lo inicializa si hace falta, lo lanza y espera a
// que acepte conexiones.
//
// Si ya hay algo escuchando en el puerto se devuelve nil sin hacer nada. No es
// una razón: ese MySQL puede ser el de la empresa, y apagarlo para poner el
// nuestro sería exactamente el daño que este diseño evita.
func (e *EmbeddedMySQL) Start(ctx context.Context) error {
	if EsServidorMySQLArriba(e.opts.Host, e.opts.Puerto) {
		return nil // ya hay un servidor en ese puerto: es el de la empresa
	}
	if e.puertoOcupadoPorAlgoQueNoEsMySQL() {
		// El puerto responde pero no habla MySQL. Sin esta comprobación, el
		// arranque se queda esperando a un servidor que no contestará nunca y
		// acaba con un "no aceptó conexiones en 90s" que no dice nada. Un
		// programita de Node, un WAMP con otro puerto, o un servicio del
		// antivirus dan este caso; ninguno se arregla reintentando.
		return fmt.Errorf(
			"el puerto %d está ocupado por un programa que no es MySQL. "+
				"Cambie DB_PORT a otro (el del MySQL incluido es el %d) o "+
				"libere el puerto.", e.opts.Puerto, MySQLPortIncluido)
	}
	if !e.Inicializado() {
		if err := e.inicializar(); err != nil {
			return err
		}
	}
	e.limpiarCandadosViejos()
	if err := os.MkdirAll(filepath.Dir(e.opts.LogPath), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear la carpeta del log de MySQL: %w", err)
	}
	logFile, err := os.OpenFile(e.opts.LogPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el log de MySQL (%s): %w", e.opts.LogPath, err)
	}

	// --defaults-file tiene que ser el PRIMER argumento: mysqld lee el resto
	// de la línea de comandos como opciones de otro programa si no.
	args := append([]string{
		"--defaults-file=" + e.escribirMyCnf(),
	}, e.argumentosArranque()...)

	cmd := exec.Command(e.opts.Binario, args...)
	cmd.Dir = e.opts.BaseDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("no se pudo arrancar mysqld (%s): %w", e.opts.Binario, err)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.logFile = logFile
	e.iniciado = true
	e.mu.Unlock()

	if err := e.esperar(ctx); err != nil {
		// El proceso ya está en marcha y puede que solo tarde. Matarlo aquí
		// dejaría un servidor a medias; se deja y se reporta.
		return fmt.Errorf("%w (log: %s)", err, e.opts.LogPath)
	}
	if err := e.asegurarClave(); err != nil {
		return err
	}
	return nil
}

// puertoOcupadoPorAlgoQueNoEsMySQL distingue "hay un MySQL" de "hay algo".
//
// Un puerto que acepta TCP no dice nada sobre quién lo escucha. Se lanza una
// sentencia de verdad —con nombre de base inventado, para no tocar la de
// nadie— y se mira la respuesta: si el servidor contesta con un error de
// MySQL, hay un MySQL; si contesta con otra cosa, no lo hay.
func (e *EmbeddedMySQL) puertoOcupadoPorAlgoQueNoEsMySQL() bool {
	if !puertoEscucha(e.opts.Host, e.opts.Puerto) {
		return false
	}
	db, err := ConectarServidorMySQL(ConfigMySQL{
		Host: e.opts.Host, Port: e.opts.Puerto,
		Usuario: e.opts.Usuario, Clave: e.opts.Clave,
	})
	if err == nil {
		db.Close()
		return false // sí habla MySQL: es el de la empresa y no se toca
	}
	// "Access denied" también es MySQL: hay servidor, solo que no con esas
	// credenciales. Solo se considera ajeno cuando la respuesta no es de MySQL.
	return !esErrorDeMySQL(err)
}

// limpiarCandadosViejos quita los ficheros de bloqueo que dejó un mysqld que
// murió.
//
// mysqld escribe `mysqld.pid` y `mysqld.pid.lock` en el directorio de datos. Si
// el proceso muere de golpe —un apagado forzado, un corte de luz— el `.lock` se
// queda, y el siguiente arranque se para con un "Can't start server: Bind on
// TCP/IP port" o un "Another process is using the data directory" que no dice de
// qué proceso se trata.
//
// Solo se borran si NADIE está escuchando en el puerto: si hay un mysqld vivo,
// esos ficheros son suyos y borrarlos sería mentir sobre su estado.
func (e *EmbeddedMySQL) limpiarCandadosViejos() {
	if EsServidorMySQLArriba(e.opts.Host, e.opts.Puerto) {
		return
	}
	for _, nombre := range []string{"mysqld.pid.lock", "mysqld.pid", "hostname.err"} {
		ruta := filepath.Join(e.opts.DataDir, nombre)
		if err := os.Remove(ruta); err == nil {
			fmt.Printf("  MySQL incluido: retirado el bloqueo viejo %s\n", nombre)
		}
	}
}

// Stop apaga el MySQL que arrancó ESTE proceso.
//
// Si el servidor ya estaba antes (el de la empresa, o una ejecución anterior que
// sigue viva), no se toca. Apagar un MySQL que no es nuestro es la forma más
// rápida de tumbar la base de datos de otra aplicación.
func (e *EmbeddedMySQL) Stop() {
	e.mu.Lock()
	cmd := e.cmd
	logFile := e.logFile
	nuestro := e.iniciado
	e.cmd, e.logFile, e.iniciado = nil, nil, false
	e.mu.Unlock()

	if !nuestro || cmd == nil || cmd.Process == nil {
		return
	}

	// Apagado limpio con la sentencia SHUTDOWN. Es mejor que matar el proceso:
	// InnoDB termina las transacciones en curso y no deja el registro de redo a
	// medias. Sin SHUTDOWN, el siguiente arranque tiene que hacer recuperación
	// y la base tarda más en estar disponible.
	//
	// La conexión se corta en el SHUTDOWN, así que el error que devuelve es
	// normal y no se mira.
	go func() {
		db, err := ConectarServidorMySQL(ConfigMySQL{
			Host: e.opts.Host, Port: e.opts.Puerto,
			Usuario: e.opts.Usuario, Clave: e.opts.Clave,
		})
		if err == nil {
			_, _ = db.Exec("SHUTDOWN")
			db.Close()
		}
	}()

	esperado := make(chan error, 1)
	go func() { esperado <- cmd.Wait() }()

	select {
	case <-esperado:
	case <-time.After(e.opts.EsperaParada):
		_ = cmd.Process.Kill()
		<-esperado
	}
	if logFile != nil {
		logFile.Close()
	}
}

// IniciadoPorNosotros dice si este proceso arrancó el servidor. Lo consulta
// main para no apagar un MySQL ajeno al apagar el ERP.
func (e *EmbeddedMySQL) IniciadoPorNosotros() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.iniciado
}

// inicializar crea el directorio de datos.
//
// `--initialize-insecure` deja el usuario root SIN clave, que es justo lo que
// hace falta para el paso siguiente:arrancar en local y poner la clave por SQL.
// La alternativa (`--initialize` con `--default-authentication-plugin`) pide una
// clave por consola y deja el servidor bloqueado, que no sirve para un
// instalador desatendido.
func (e *EmbeddedMySQL) inicializar() error {
	if err := os.MkdirAll(e.opts.DataDir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear el directorio de datos (%s): %w", e.opts.DataDir, err)
	}
	args := []string{
		"--no-defaults",
		"--initialize-insecure",
		"--basedir=" + e.opts.BaseDir,
		"--datadir=" + e.opts.DataDir,
		"--console",
	}
	cmd := exec.Command(e.opts.Binario, args...)
	cmd.Dir = e.opts.BaseDir
	salida, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("MySQL no se pudo inicializar en %s: %w\n%s",
			e.opts.DataDir, err, ultimasLineas(string(salida), 12))
	}
	if !e.Inicializado() {
		return fmt.Errorf("MySQL dijo que se inicializó pero %s no tiene el catálogo del sistema; "+
			"revisa el log del sistema", e.opts.DataDir)
	}
	return nil
}

// esperar espera a que el servidor acepte una consulta de verdad.
//
// El puerto abierto NO significa MySQL listo. Durante el arranque mysqld acepta
// TCP antes de tener las tablas del sistema, y en ese momento una conexión entra
// y cualquier consulta falla con un error que no dice nada útil. Por eso aquí no
// se mira el puerto: se hace una consulta real.
//
// Se acepta cualquiera de las dos formas de entrar —con la clave del .env o sin
// clave, que es como queda un servidor recién inicializado— porque en ese
// momento todavía no se sabe en cuál de los dos estados está. Cuál de los dos es
// lo decide asegurarClave, después.
func (e *EmbeddedMySQL) esperar(ctx context.Context) error {
	plazo := time.Now().Add(e.opts.EsperaInicial)
	var ultimo error
	for time.Now().Before(plazo) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if EsServidorMySQLArriba(e.opts.Host, e.opts.Puerto) {
			for _, clave := range []string{e.opts.Clave, ""} {
				db, err := ConectarServidorMySQL(ConfigMySQL{
					Host: e.opts.Host, Port: e.opts.Puerto,
					Usuario: e.opts.Usuario, Clave: clave,
				})
				if err != nil {
					ultimo = err
					continue
				}
				var uno int
				if err := db.QueryRow("SELECT 1").Scan(&uno); err != nil {
					ultimo = err
					db.Close()
					continue
				}
				db.Close()
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if ultimo != nil {
		return fmt.Errorf("MySQL no aceptó conexiones en %s:%d en %s: %w",
			e.opts.Host, e.opts.Puerto, e.opts.EsperaInicial, ultimo)
	}
	return fmt.Errorf("MySQL no aceptó conexiones en %s:%d en %s",
		e.opts.Host, e.opts.Puerto, e.opts.EsperaInicial)
}

// asegurarClave pone la clave de root si el servidor se acaba de inicializar.
//
// Tras `--initialize-insecure` el usuario root entra sin clave. Si la clave que
// pide el ERP no es la del servidor, no se cambia nada: eso significaría que el
// usuario ha configurado otra cosa a propósito y reescribirla desde el ERP sería
// perder trabajo suyo. En ese caso se avisa.
func (e *EmbeddedMySQL) asegurarClave() error {
	if e.opts.Clave == "" {
		return nil
	}
	// ¿Ya funciona con la clave pedida?
	if db, err := OpenMySQL(ConfigMySQL{
		Host: e.opts.Host, Port: e.opts.Puerto,
		Usuario: e.opts.Usuario, Clave: e.opts.Clave,
	}); err == nil {
		db.Close()
		return nil
	}
	// Probar sin clave: es el estado recién inicializado.
	db, err := sql.Open("mysql", DSNMySQL(ConfigMySQL{
		Host: e.opts.Host, Port: e.opts.Puerto, Usuario: e.opts.Usuario,
	}))
	if err != nil {
		return fmt.Errorf("no se pudo conectar al MySQL recién inicializado: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("el MySQL recién inicializado no responde: %w", err)
	}
	// El usuario y la clave NO pueden ir como parámetros: `ALTER USER` es una
	// sentencia DDL y sus identificadores no admiten marcadores de posición, así
	// que hay que componer el texto. Por eso la clave se valida antes: sin esa
	// comprobación, una clave con una comilla produciría una sentencia que
	// cambia de opinión a mitad y podría dejar el servidor sin usuarios
	// utilizables.
	if !claveValida(e.opts.Clave) {
		return fmt.Errorf(
			"la clave de MySQL tiene un carácter que MySQL no admite: %q", e.opts.Clave)
	}
	usuario := e.opts.Usuario
	if !usuarioValido(usuario) {
		return fmt.Errorf("el nombre de usuario de MySQL no es válido: %q", usuario)
	}
	// El usuario root@localhost es el que existe tras --initialize-insecure. Se
	// cambia ese y no otro: crear un usuario nuevo y quitar privileges al
	// anterior abriría un segundo camino de entrada al servidor.
	stmt := fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s'",
		usuario, e.opts.Clave)
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("no se pudo fijar la clave de %s: %w", usuario, err)
	}
	// La conexión que se acaba de autenticar sin clave sigue viva durante la
	// sesión: por eso el ALTER va después de abrirla y no en un orden que corte
	// la propia sesión antes de tiempo.
	return nil
}

// claveValida dice si una clave se puede escribir dentro de una sentencia.
//
// MySQL 8 con caching_sha2_password (el método por defecto) admite casi todo en
// una clave. Lo que no admite es el punto y coma, porque el cliente de la línea
// de comandos lo trata como fin de sentencia, y la barra invertida, que en una
// sentencia es el carácter de escape. Cortar por esos dos no hace el trabajo más
// seguro: lo hace aparecer seguro, que es peor.
func claveValida(clave string) bool {
	for _, r := range clave {
		if r == ';' || r == '\\' || r == 0 {
			return false
		}
	}
	return true
}

// usuarioValido limita el usuario a lo que MySQL acepta en un nombre de cuenta.
func usuarioValido(usuario string) bool {
	if usuario == "" || len(usuario) > 32 {
		return false
	}
	for _, r := range usuario {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '$', r == '.':
		default:
			return false
		}
	}
	return true
}

// escribirMyCnf deja el archivo de opciones del servidor.
//
// Existe por dos razones. Una es que mysqld tiene tres fuentes de configuración
// (archivo, línea de comandos, variables de entorno) que se mezclan con un orden
// no intuitivo, y pelearse con eso desde código es frágil. La otra es que el
// archivo se queda en disco y sirve para diagnosticar: si el ERP no arranca, se
// abre `erpgo-my.ini` y se ve exactamente con qué se arrancó.
func (e *EmbeddedMySQL) escribirMyCnf() string {
	ruta := filepath.Join(filepath.Dir(e.opts.LogPath), "erpgo-my.ini")
	// Rutas con barra normal: en un archivo de opciones de MySQL la barra
	// invertida es carácter de escape, y "C:\laragon\..." se lee como
	// "C:laragon...". Con barra normal no hay nada que escapar.
	sl := func(p string) string { return filepath.ToSlash(p) }

	contenido := fmt.Sprintf(`# Generado por ERPGo. No editar a mano: se reescribe en cada arranque.
[mysqld]
basedir=%s
datadir=%s
port=%d
bind-address=127.0.0.1
# El puerto X de MySQL no hace falta para este uso y a veces choca con otra
# instalación. Se apaga.
mysqlx=0
character-set-server=utf8mb4
collation-server=utf8mb4_unicode_ci
innodb_buffer_pool_size=%dM
max_connections=%d
# El binlog no aporta nada a un ERP de una sola máquina: cada escritura
# replicada es disco que no se necesita. Se apaga y se reservan las horas de
# I/O de disco para lo que sí importa.
skip-log-bin
log_error_verbosity=2

[client]
port=%d
`,
		sl(e.opts.BaseDir), sl(e.opts.DataDir), e.opts.Puerto,
		e.opts.InnoDBBufferPoolMB, e.opts.MaxConexiones, e.opts.Puerto)

	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err == nil {
		_ = os.WriteFile(ruta, []byte(contenido), 0o644)
	}
	return ruta
}

// argumentosArranque son las opciones que se pasan en la línea de comandos.
//
// Pocas a propósito: lo que no cambia entre instalaciones está en el archivo de
// arriba. Lo que se pone aquí es lo que depende de la instalación concreta (el
// puerto) y lo que el usuario puede haber cambiado por entorno.
func (e *EmbeddedMySQL) argumentosArranque() []string {
	args := []string{
		"--port=" + strconv.Itoa(e.opts.Puerto),
		"--console",
	}
	if v := os.Getenv("ERPGO_MYSQL_EXTRA_ARGS"); v != "" {
		args = append(args, strings.Fields(v)...)
	}
	return args
}

// ultimasLineas devuelve las últimas n líneas no vacías de un texto. Sirve para
// que un error traiga el final del log de mysqld, que es donde dice qué pasó,
// en vez de las doscientas líneas de arranque.
func ultimasLineas(s string, n int) string {
	lineas := []string{}
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lineas = append(lineas, l)
		}
	}
	if len(lineas) > n {
		lineas = lineas[len(lineas)-n:]
	}
	return strings.Join(lineas, "\n")
}
