package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
)

// BASE DE DATOS PARA TESTS.
//
// Los tests usaban SQLite en memoria o en fichero temporal. Con MySQL eso no
// sirve: casi todo lo que se comprueba aquí es comportamiento de MySQL —
// collation, AUTO_INCREMENT, motores, semántica de `ON DUPLICATE KEY UPDATE`,
// sql_mode— y probarlo contra SQLite sería probar otro motor y que todo
// saliera verde sin decir nada del real.
//
// POR QUÉ NO SE USA t.TEMPDir() NI :memory:
//
// MySQL no tiene base en memoria con este driver. La base tiene que existir de
// verdad en el servidor, y por eso lleva un nombre único.
//
// POR QUÉ EL TEST SE SALTA Y NO FALLA SI NO HAY MySQL
//
// Porque un `go test` en una máquina sin MySQL no debe ponerse rojo por algo
// que no está probado: debe decirse que se saltó. Fallar obligaría a instalar
// un servidor para poder ejecutar la suite, y eso convierte un problema
// concreto (no hay MySQL) en una barrera para todo el mundo.

// contador de bases de prueba, para que dos tests simultáneos no se pisen el
// nombre.
var contadorBase atomic.Int64

// NuevaBaseDePrueba crea una base MySQL nueva con el ESQUEMA COMPLETO (clonando
// la plantilla migrada) y devuelve el handle junto con la función que la borra.
//
// Para un test que va a definir su propio DDL, usa NuevaBaseVaciaDePrueba: con
// el esquema ya presente, su primer CREATE TABLE falla con Error 1050 y el test
// muere antes de probar nada.
//
// La función de limpieza se puede llamar aunque la base no llegara a
// crearse, y llamarla dos veces no hace daño: un `DROP DATABASE IF EXISTS` por
// una base que ya no está es inocuo, y en un test que ha fallado antes de
// tiempo la limpieza tiene que poder correr igual.
func NuevaBaseDePrueba(prefijo string) (*sql.DB, func(), error) {
	if err := validarEntornoPruebas(); err != nil {
		return nil, func() {}, err
	}
	nombre, limpiar, err := CrearBaseDePrueba(prefijo)
	if err != nil {
		return nil, func() {}, err
	}
	db, err := OpenMySQL(configDePrueba(nombre))
	if err != nil {
		limpiar()
		return nil, func() {}, err
	}
	return db, func() {
		db.Close()
		limpiar()
	}, nil
}

// NuevaBaseVaciaDePrueba crea una base MySQL nueva SIN ESQUEMA.
//
// Por qué existe aparte de NuevaBaseDePrueba. NuevaBaseDePrueba clona la
// plantilla migrada (139 tablas) porque es lo que quiere casi todo el mundo: los
// tests miden comportamiento sobre el esquema real. Pero hay tests que
// definen su propio DDL mínimo y ejecutan `CREATE TABLE ...` en él. Con la
// plantilla clonada, ese CREATE falla con Error 1050 "Table 'invoice' already
// exists" y el test muere antes de probar nada, con un mensaje que no señala el
// problema real.
//
// Aquí la base sale vacía de verdad: cero tablas. Es lo que esos tests piden.
//
// El nombre sigue siendo único (contador + reloj), así que dos tests
// simultáneos no se pisan.
func NuevaBaseVaciaDePrueba(prefijo string) (*sql.DB, func(), error) {
	if err := validarEntornoPruebas(); err != nil {
		return nil, func() {}, err
	}
	if !EsServidorMySQLArriba(hostDePrueba(), puertoDePrueba()) {
		return nil, func() {}, fmt.Errorf("no hay servidor MySQL en %s:%d",
			hostDePrueba(), puertoDePrueba())
	}
	nombre := nombreBaseDePrueba(prefijo)
	cfg := configDePrueba(nombre)
	if err := EnsureDatabase(cfg); err != nil {
		return nil, func() {}, err
	}
	db, err := OpenMySQL(cfg)
	if err != nil {
		borrarBase(cfg)
		return nil, func() {}, err
	}
	return db, func() {
		db.Close()
		borrarBase(cfg)
	}, nil
}

// nombreBaseDePrueba compone el nombre único de una base de prueba.
func nombreBaseDePrueba(prefijo string) string {
	n := contadorBase.Add(1)
	limpio := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			return r
		default:
			return '-'
		}
	}, prefijo)
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("TEST_DB_NAME")), "_")
	return fmt.Sprintf("%s_%s_%d_%d", base, strings.ToLower(limpio), n, time.Now().UnixNano()%100000)
}

// ReemplazarBaseDePrueba resetea SOLO la base indicada, que debe ser TEST_DB_NAME,
// clonando una plantilla explícita de pruebas. No hace DROP: elimina las tablas
// existentes y vuelve a crear su estructura/datos.
func ReemplazarBaseDePrueba(origen, destino string) error {
	if err := validarEntornoPruebas(); err != nil {
		return err
	}
	if destino != strings.TrimSpace(os.Getenv("TEST_DB_NAME")) {
		return fmt.Errorf("solo se puede reiniciar la base TEST_DB_NAME configurada")
	}
	if !esBaseDePrueba(origen) || !esBaseDePrueba(destino) {
		return fmt.Errorf("origen y destino deben ser bases de pruebas")
	}
	if _, err := nombreBaseSeguro(origen); err != nil {
		return err
	}
	if _, err := nombreBaseSeguro(destino); err != nil {
		return err
	}
	cfg := configDePrueba("")
	server, err := ConectarServidorMySQL(cfg)
	if err != nil {
		return err
	}
	defer server.Close()
	if _, err := server.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", destino)); err != nil {
		return fmt.Errorf("limpiando base descartable: %w", err)
	}
	if _, err := server.Exec(fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", destino)); err != nil {
		return fmt.Errorf("recreando base descartable: %w", err)
	}
	if err := ClonarBase(origen, destino); err != nil {
		return fmt.Errorf("restaurando plantilla de pruebas: %w", err)
	}
	return nil
}

// CrearBaseDePrueba crea una base de prueba con nombre único debajo del prefijo
// TEST_DB_NAME y devuelve su NOMBRE junto con la función que la borra.
//
// Existe separada de NuevaBaseDePrueba porque hay un caso que el handle no
// cubre: la base de CONTROL. Un test de multi-empresa necesita que su catálogo de
// empresas sea una base propia, con nombre propio, porque se configura en un
// Config y no se abre en la línea siguiente. Devolver el nombre es lo que hace
// posible eso sin que el test tenga que reimplementar la regla de nombres —que es
// justamente la clase de regla que no se debe duplicar en un test—.
func CrearBaseDePrueba(prefijo string) (string, func(), error) {
	if err := validarEntornoPruebas(); err != nil {
		return "", func() {}, err
	}
	if !EsServidorMySQLArriba(hostDePrueba(), puertoDePrueba()) {
		return "", func() {}, fmt.Errorf("no hay servidor MySQL en %s:%d",
			hostDePrueba(), puertoDePrueba())
	}
	// El nombre lleva la etiqueta del test, un contador y el reloj: con los tres,
	// dos tests simultáneos no pueden coincidir ni aunque se repitan las
	// etiquetas.
	nombre := nombreBaseDePrueba(prefijo)

	cfg := configDePrueba(nombre)
	if err := clonarBaseDePrueba(cfg); err != nil {
		return "", func() {}, err
	}
	return nombre, func() { borrarBase(cfg) }, nil
}

// AbrirBaseDePrueba abre una base de prueba POR NOMBRE, con las credenciales de
// la suite.
//
// Existe para el paso intermedio de una plantilla: hay siembras —las de negocio,
// las que hace la capa web— que solo se pueden ejecutar sobre un handle abierto, y
// el nombre lo eligió CrearBaseDePrueba. Sin esto, el helper tendría que
// reimplementar la lectura del .env y de TEST_DB_*, que es la duplicación que
// configDePrueba existe para evitar.
func AbrirBaseDePrueba(nombre string) (*sql.DB, error) {
	if err := validarEntornoPruebas(); err != nil {
		return nil, err
	}
	if !esBaseDePrueba(nombre) {
		return nil, fmt.Errorf("base %q no pertenece al espacio de nombres de pruebas", nombre)
	}
	return OpenMySQL(configDePrueba(nombre))
}

// clonarBaseDePrueba deja la base con el esquema y la semilla.
//
// POR QUÉ CLONAR Y NO MIGRAR. Las migraciones cuestan unas 10 s (17 ficheros,
// 428 sentencias) y el resultado es idéntico para todas las bases. La plantilla
// se aplica una vez —y se reutiliza entre corridas, porque su nombre lleva la
// huella de las migraciones— y clonar cuesta menos de un segundo. Medido sobre
// una corrida completa: la suite bajó de 990 s a unos 4 minutos.
func clonarBaseDePrueba(cfg ConfigMySQL) error {
	plantilla, err := PlantillaDePrueba()
	if err != nil {
		// Sin plantilla no hay atajo, pero tampoco motivo para no seguir: se
		// aplican las migraciones, que es lo que se ha hecho siempre.
		if err := EnsureDatabase(cfg); err != nil {
			return err
		}
		db, err := OpenMySQL(cfg)
		if err != nil {
			return err
		}
		defer db.Close()
		return RunMigrationsForMySQL(db, cfg)
	}
	if err := ClonarBase(plantilla, cfg.Nombre); err != nil {
		return fmt.Errorf("clonando la base de prueba desde %s: %w", plantilla, err)
	}
	return nil
}

// configDePrueba devuelve la configuración de conexión de una base de prueba.
//
// Centraliza la lectura del .env y de las variables de entorno en un sitio: si
// NuevaBaseDePrueba y BorrarBasesConPrefijo leyeran el entorno por su cuenta,
// un cambio en el nombre de una variable dejaría al primero creando bases que el
// segundo no borra, y eso solo se vería en el servidor.
var patronBaseTest = regexp.MustCompile(`^erpgo_test_[a-zA-Z0-9_-]+$`)

// validarEntornoPruebas impide que cualquier helper de este paquete lea DB_*
// o .env para crear/borrar bases. TEST_DB_NAME es un destino descartable que
// el operador eligió explícitamente; sus credenciales también deben ser TEST_DB_*.
func validarEntornoPruebas() error {
	cargarEnvDelProyecto()
	nombre := strings.TrimSpace(os.Getenv("TEST_DB_NAME"))
	if !patronBaseTest.MatchString(nombre) || len(nombre) > 48 {
		return fmt.Errorf("TEST_DB_NAME debe definir un prefijo de pruebas descartable válido (erpgo_test_<etiqueta>, máximo 48 caracteres); las pruebas no pueden operar sobre DB_NAME ni crear bases arbitrarias")
	}
	if appName := strings.TrimSpace(os.Getenv("DB_NAME")); appName != "" && nombre == appName {
		return fmt.Errorf("TEST_DB_NAME no puede coincidir con DB_NAME")
	}
	if os.Getenv("TEST_DB_USER") == "" {
		return fmt.Errorf("TEST_DB_USER debe configurarse explícitamente; no se hereda DB_USER")
	}
	if os.Getenv("TEST_DB_HOST") == "" {
		return fmt.Errorf("TEST_DB_HOST debe configurarse explícitamente; no se hereda DB_HOST")
	}
	return nil
}

func CrearEsquemaDePrueba(nombre string) error {
	if err := validarEntornoPruebas(); err != nil {
		return err
	}
	if !esBaseDePrueba(nombre) {
		return fmt.Errorf("base %q no pertenece al prefijo TEST_DB_NAME", nombre)
	}
	cfg := configDePrueba(nombre)
	if err := EnsureDatabase(cfg); err != nil {
		return err
	}
	db, err := OpenMySQL(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	return RunMigrationsForMySQL(db, cfg)
}

func configDePrueba(nombre string) ConfigMySQL {
	// El .env del proyecto, si está, para no tener que repetir la clave en cada
	// entorno de CI. TEST_DB_* gana sobre DB_*.
	//
	// godotenv.Load resuelve las rutas contra el DIRECTORIO DE TRABAJO, no
	// contra el fichero que la llama. Por eso se buscan las tres: un `go test` de
	// interfaces/web corre con el cwd ahí y necesita ../../.env, mientras que
	// `go run ./tools/limpiar` corre desde la raíz y necesita .env. Con solo
	// ../../.env la herramienta iba a MySQL sin clave y contestaba "Access
	// denied", que es un error que habla de una clave que el proyecto sí tiene.
	cargarEnvDelProyecto()
	return ConfigMySQL{
		Host:    entorno("TEST_DB_HOST", ""),
		Port:    atoi(entorno("TEST_DB_PORT", "3306")),
		Usuario: entorno("TEST_DB_USER", ""),
		Clave:   entorno("TEST_DB_PASSWORD", ""),
		Nombre:  nombre,
	}
}

func hostDePrueba() string { return entorno("TEST_DB_HOST", "") }
func puertoDePrueba() int  { return atoi(entorno("TEST_DB_PORT", entorno("DB_PORT", "3306"))) }

// cargarEnvDelProyecto busca el `.env` del proyecto y lo carga, UNA vez.
//
// POR QUÉ NO `godotenv.Load("../../.env", "../.env", ".env")` DIRECTO.
//
// Medido: con esa lista de ficheros, la PRIMERA llamada no carga nada y la
// segunda sí. godotenv recorre la lista, carga el primero que existe, y al
// llegar al segundo —que no existe— devuelve el error; el resultado es que el
// entorno queda como estaba. O sea: el fichero se cargaba a partir de la
// segunda llamada, y el primer test de cada proceso se quedaba sin
// credenciales, `EnsureDatabase` fallaba con "Access denied for user 'root'
// (using password: NO)" y el test se saltaba. Peor: se saltaba en silencio y
// la suite salía verde sin haber probado nada.
//
// Aquí se busca el fichero que existe, se carga SOLO ese, y una vez.
var cargarEnv sync.Once

func cargarEnvDelProyecto() {
	cargarEnv.Do(func() {
		for _, ruta := range []string{"../../.env", "../.env", ".env"} {
			if info, err := os.Stat(ruta); err == nil && !info.IsDir() {
				_ = godotenv.Load(ruta)
				return
			}
		}
	})
}

// PrefijoDePrueba devuelve un prefijo único y corto, para agrupar las bases que
// crea UN test.
//
// Existe por un motivo concreto: la base de una empresa se llama
// `erpgo_<código>`, y ese código lo elige el test. Sin un prefijo, todos los
// tests comparten `erpgo_default` — la base REAL de desarrollo— y se pisan los
// datos entre ellos. Ese fallo es el peor de todos: el test pasa, y lo que se
// ensucia es la base del desarrollador. Con el prefijo, cada test trabaja en
// `erpgo_t<único>_default` y se borra al terminar.
//
// Se devuelve con guion bajo delante para poder concatenarlo sin más:
// `cfg.DBTenantPrefix = persistence.PrefijoDePrueba(t, "t")`.
func PrefijoDePrueba(t interface{ Cleanup(func()) }, etiqueta string) string {
	n := contadorBase.Add(1)
	limpio := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, etiqueta)
	return fmt.Sprintf("%s%d_%d_", strings.ToLower(limpio), n, time.Now().UnixNano()%100000)
}

// BorrarBasesConPrefijo elimina todas las bases de EMPRESA creadas con ese
// prefijo.
//
// Los tests que crean bases de empresa (una por cada código de empresa que
// inventan) no pueden devolver una función de limpieza por base, porque el
// código lo elige el propio test y no el helper. Con esto, el test registra una
// sola limpieza y se lleva por delante todas las suyas.
//
// Borra por coincidencia de prefijo y no por lista, porque la lista sería
// exactamente la información que el test no tiene todavía.
//
// EL PREFIJO `erpgo_` DE ESTA BUSQUEDA NO ES OPCIONAL, Y FUE UN FALLO REAL.
//
// El nombre de la base de una empresa es `erpgo_<prefijo>_<codigo>`, no
// `<prefijo>_<codigo>`: lo antepone prefijoBaseEmpresa. Buscando el prefijo del
// test a secas no encontraba NINGUNA base, la limpieza no borraba nada, y cada
// corrida dejaba sus bases en el servidor. En dos días de pruebas eso son 248
// bases y, con ellas, el ERROR 1040 "Too many connections" que hizo fallar una
// tanda entera de tests que no tenían nada que ver con bases.
//
// Por eso la lista de bases se pide aquí, en la capa que conoce la regla de
// nombres, y no en el test.
func BorrarBasesConPrefijo(prefijo string) {
	if validarEntornoPruebas() != nil {
		return
	}
	prefijo = strings.Trim(strings.TrimSpace(prefijo), "_")
	if prefijo == "" {
		return
	}
	cfg := configDePrueba("")
	db, err := sql.Open("mysql", DSNMySQL(cfg))
	if err != nil {
		return
	}
	defer db.Close()

	// El prefijo va como parámetro, no interpolado: es la diferencia entre
	// "borra lo que empieza por esto" y "borra lo que el test haya escrito en la
	// sentencia". Aunque hoy el prefijo lo genera el propio código, esa es la
	// forma en la que esto no puede volverse un `DROP` de cualquier cosa.
	patron := strings.TrimRight(strings.TrimSpace(os.Getenv("TEST_DB_NAME")), "_") + "_" + strings.ReplaceAll(prefijo, "%", "") + "%"
	rows, err := db.Query(
		"SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE ?",
		patron)
	if err != nil {
		return
	}
	var nombres []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		nombres = append(nombres, n)
	}
	rows.Close()

	for _, n := range nombres {
		cfg.Nombre = n
		borrarBase(cfg)
	}
}

// BasesDePruebaHuerfanas devuelve los NOMBRES de las bases de prueba que
// quedaron en el servidor sin que nadie las vaya a borrar.
//
// POR QUÉ HACE FALTA, CUANDO LOS TESTS YA SE BORRAN SOLOS.
//
// Cada test registra su limpieza con t.Cleanup, así que en una corrida que
// termina bien no queda nada. El problema es la corrida que NO termina bien: si
// el binario de test se mata, si MySQL se cae a mitad, si el portapapeles se
// cierra, los t.Cleanup no llegan a ejecutarse y las bases se quedan ahí para
// siempre. No es hipotético: siete corridas incompletas llegaron a dejar 97
// bases, y el síntoma fue el ERROR 1040 "Too many connections" en una tanda de
// tests que no tenía nada que ver con bases.
//
// La limpieza al final de cada test es la primera línea; esta es la segunda,
// para lo que la primera no alcanza. Sin ella, un desarrollador que corta la corrida
// una vez tiene que acordarse de venir a limpiar a mano.
//
// SOLO TOCA BASES CUYO NOMBRE IMPOSIBLE QUE SEA DE UNA EMPRESA REAL.
//
// El patrón es `erpgo_test_...` y `erpgo_t<contador>_<sello>_...`: los dos los
// genera este fichero, con un número y una marca de tiempo dentro. Un nombre de
// empresa real no puede ser `erpgo_t107_12000__acme` porque el código de empresa
// se sanea a [a-z0-9_] y nunca lleva un número de contador ni un reloj. Por eso
// el borrado se puede automatizar sin miedo: no hay ninguna empresa de una
// persona que se llame así.
func BasesDePruebaHuerfanas() ([]string, error) {
	if err := validarEntornoPruebas(); err != nil {
		return nil, err
	}
	cfg := configDePrueba("")
	db, err := sql.Open("mysql", DSNMySQL(cfg))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(
		"SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE ?",
		strings.TrimRight(strings.TrimSpace(os.Getenv("TEST_DB_NAME")), "_")+"_%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var huerfanas []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		if esBaseDePrueba(n) {
			huerfanas = append(huerfanas, n)
		}
	}
	return huerfanas, rows.Err()
}

// esBaseDePrueba dice si un nombre es de una base que creó la suite de tests.
func esBaseDePrueba(nombre string) bool {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("TEST_DB_NAME")), "_")
	return base != "" && (nombre == base || strings.HasPrefix(nombre, base+"_"))
}

// BorrarBasesDePruebaHuerfanas elimina las bases de prueba abandonadas y
// devuelve cuántos eran y cuántos se pudieron borrar.
//
// Dice los dos números porque un borrado puede fallar sin que se note: en
// Windows una base con conexiones abiertas no se elimina, y eso se parece
// mucho a "no había nada que borrar". Informar solo del éxito daría la misma
// respuesta en los dos casos.
func BorrarBasesDePruebaHuerfanas() (encontradas, borradas int, err error) {
	if err := validarEntornoPruebas(); err != nil {
		return 0, 0, err
	}
	huerfanas, err := BasesDePruebaHuerfanas()
	if err != nil {
		return 0, 0, err
	}
	cfg := configDePrueba("")
	for _, n := range huerfanas {
		cfg.Nombre = n
		if err := borrarBaseDevolviendoError(cfg); err == nil {
			borradas++
		}
	}
	return len(huerfanas), borradas, nil
}

// borrarBaseDevolviendoError es borrarBase pero diciendo qué pasó, que es lo que
// necesita el barrido para no contar como borrada una base que se quedó.
func borrarBaseDevolviendoError(cfg ConfigMySQL) error {
	if err := validarEntornoPruebas(); err != nil {
		return err
	}
	if !esBaseDePrueba(cfg.Nombre) {
		return fmt.Errorf("rechazado DROP de base fuera de erpgo_test_*")
	}
	sinBase := cfg
	sinBase.Nombre = ""
	db, err := sql.Open("mysql", DSNMySQL(sinBase))
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.Nombre))
	return err
}

// borrarBase elimina la base de prueba, ignorando errores.
//
// Si no se puede borrar, el nombre lleva un sello de tiempo, así que la base
// abandonada no se confunde con la de otro test ni ocupa el sitio de ninguna.
func borrarBase(cfg ConfigMySQL) {
	if validarEntornoPruebas() != nil || !esBaseDePrueba(cfg.Nombre) {
		return
	}
	sinBase := cfg
	sinBase.Nombre = ""
	db, err := sql.Open("mysql", DSNMySQL(sinBase))
	if err != nil {
		return
	}
	defer db.Close()
	// El DROP puede quedarse esperando el metadata lock de una conexión del
	// propio test que aún no cerró (Windows libera los handles tarde). Sin
	// deadline, la limpieza bloquea el paquete entero en el Cleanup y el test
	// muere por "test timed out after 10m" sin apuntar al verdadero culpable.
	// Con deadline el intento se rinde a los 30s: el sello de tiempo del nombre
	// hace que la base sobreviviente no moleste a nadie (ver el comentario de
	// arriba).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.Nombre))
}

func entorno(clave, defecto string) string {
	if v, ok := os.LookupEnv(clave); ok {
		return v
	}
	return defecto
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return 3306
	}
	return n
}
