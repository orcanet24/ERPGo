package persistence

// plantilla.go — Bases de prueba de persistence, sin volver a migrar 17 veces.
//
// EL PROBLEMA.
//
// Cada test que pide su propia base la crea y le aplica el juego entero de
// migraciones: 17 ficheros, 428 sentencias. Medido sobre este esquema: 11 s.
// Con 25 bases en una corrida son cuatro y medio minutos de DDL repetido, y el
// trabajo es siempre el mismo.
//
// LA SOLUCIÓN, Y LO QUE SE MIDIO DE ELLA.
//
// Se aplica el esquema UNA vez en una base plantilla y cada base se clona con
// `CREATE TABLE ... LIKE` mas `INSERT IGNORE ... SELECT`.
//
//   migrar   11,1 s por base
//   clonar    3,9 s por base
//
// Tres veces mas rapido, con el mismo esquema y la misma semilla.
//
// DONDE NO SIRVE, Y POR QUE NO SE PUSO.
//
// El atajo se probó tambien en `interfaces/web`, que es donde se crea el 90% de
// las bases (las de empresa, por `openTenantDB`). ALLI NO APORTA: un test que
// tarda 22,8 s tardo 25,3 s con la plantilla. El clonado paga 131 `CREATE
// TABLE`, que es un DDL por tabla, y las migraciones que se ahorran ya estaban
// amortizadas por el camino rapido con `multiStatements`. Por eso aqui la
// plantilla se usa y alla no: medir antes de dejar un atajo puesto.
//
// LA PLANTILLA SE INVALIDA SOLA.
//
// El nombre lleva la huella de las migraciones: `erpgo_plantilla_<huella>`.
// Si alguien anade, borra o edita un fichero, la huella cambia, el nombre
// cambia y se crea otra. Las viejas se borran solas en `LimpiarPlantillasViejas`.
// No hay paso manual que recordar, que es donde estos atajos suelen estropear
// la base.
//
// QUE NO SE TOCA.
//
// La ruta de produccion no cambia: dar de alta una empresa sigue aplicando las
// migraciones. Esto solo lo usan los tests de ESTE paquete (ver testdb.go), donde
// lo que importa es tener el esquema real, no el camino que lo creo.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	plantillaUnaVez sync.Once
	plantillaMutex  sync.Mutex
	// huellaVacía se rellena la primera vez; a partir de ahí el nombre de la
	// plantilla es siempre el mismo.
	nombrePlantilla string
)

// PlantillaDePrueba deja lista la base plantilla y devuelve su nombre.
//
// Devuelve un error y no se salta por su cuenta: quien llama decide. El test
// decide saltarse; el producto decidiría avisar.
func PlantillaDePrueba() (string, error) {
	var err error
	plantillaUnaVez.Do(func() {
		plantillaMutex.Lock()
		defer plantillaMutex.Unlock()
		nombrePlantilla, err = asegurarPlantilla()
	})
	if err != nil {
		return "", err
	}
	if nombrePlantilla == "" {
		// El proceso anterior lo dejó sin nombre porque el .env no se pudo
		// leer. Se reintenta, que puede ser que ya esté.
		plantillaMutex.Lock()
		nombrePlantilla, err = asegurarPlantilla()
		plantillaMutex.Unlock()
		if err != nil {
			return "", err
		}
	}
	return nombrePlantilla, nil
}

// asegurarPlantilla crea la base plantilla si este juego de migraciones aún no
// tiene una.
func asegurarPlantilla() (string, error) {
	migraciones, err := readMigrations()
	if err != nil {
		return "", err
	}
	// El PID va en el nombre porque los tests se ejecutan a veces en varios
	// procesos a la vez (ver doc/TIEMPO-DE-PRUEBAS.md). Con una plantilla
	// compartida, uno la borra mientras otro la está clonando y aparecen
	// "Deadlock found when trying to get lock", "Unknown database" y "no tiene
	// tablas". Medido: con el PID, tres procesos en paralelo van limpios.
	//
	// EL RELOJ NO VA EN EL NOMBRE. Medido: estaba, y por eso `baseConEsquema`
	// NUNCA encontraba una plantilla existente —buscaba por un nombre que
	// acababa de inventar— así que cada corrida reconstruía la base desde cero
	// y dejaba la anterior atrás. En el servidor de desarrollo había 46
	// plantillas de 3,6 MB: 165 MB de basura. La antigüedad la lee la limpieza
	// de `information_schema.schemata.create_time`, que es donde está de
	// verdad y no depende de una convención de nombres.
	nombre := fmt.Sprintf("erpgo_plantilla_%s_%d",
		huellaMigraciones(migraciones)[:12], os.Getpid())

	// BARRIDA DE ZOMBIES. Las plantillas cuyo PID ya no está vivo son basura
	// indefensa: su dueño murió (timeout, Ctrl+C, crash) y su Cleanup nunca
	// corrió. La limpieza por edad (24h) no las toca si create_time se
	// refrescó con la normalización de collation, y la limpieza normal solo
	// corre al crear una plantilla del MISMO PID. Medido en la máquina de
	// desarrollo: 23 plantillas zombis de un solo día de trabajo, 149 MB.
	// La barrida compara el PID del nombre con la lista de procesos vivos:
	// si ese PID no corre, la base es orfanata y se borra sin trasladar el
	// costo al arranque normal del ERP (esto es solo para tests).
	LimpiarPlantillasZombies(nombre)

	cfg := configDePrueba(nombre)
	if baseConEsquema(cfg.Nombre, migraciones) {
		return nombre, nil
	}

	borrarBase(cfg)
	if err := EnsureDatabase(cfg); err != nil {
		return "", err
	}
	db, err := OpenMySQL(cfg)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := RunMigrationsForMySQL(db, cfg); err != nil {
		return "", err
	}
	LimpiarPlantillasViejas(nombre, 24*time.Hour)
	return nombre, nil
}

// baseConEsquema dice si la base existe con el juego de migraciones completo.
func baseConEsquema(nombre string, migraciones []migFile) bool {
	cfg := configDePrueba(nombre)
	db, err := OpenMySQL(cfg)
	if err != nil {
		return false
	}
	defer db.Close()
	if err := asegurarRegistroMySQL(db); err != nil {
		return false
	}
	registro := leerRegistroMySQL(db)
	if len(registro) != len(migraciones) {
		return false
	}
	for _, m := range migraciones {
		if _, ok := registro[m.name]; !ok {
			return false
		}
	}
	return true
}

// huellaMigraciones resume el nombre y el contenido de todos los ficheros.
func huellaMigraciones(migraciones []migFile) string {
	h := sha256.New()
	for _, m := range migraciones {
		h.Write([]byte(m.name))
		h.Write([]byte(m.content))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pidVivos devuelve el conjunto de PIDs de procesos que están corriendo
// AHORA en esta máquina. En falla devuelve nil: el llamador decide qué hacer
// con una lista vacía (aquí: no borrar nada, favor del seguro).
func pidsVivos() map[int]bool {
	vivos := map[int]bool{}

	// /proc en Linux; en Windows se enumera la lista vía tasklist.
	if entradas, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entradas {
			if pid, err := strconv.Atoi(e.Name()); err == nil {
				vivos[pid] = true
			}
		}
		return vivos
	}
	if runtime.GOOS == "windows" {
		salida, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
		if err != nil {
			return nil
		}
		for _, linea := range strings.Split(string(salida), "\n") {
			// CSV windows: "nombre","pid","..." → el PID es el segundo campo.
			campos := strings.Split(linea, ",")
			if len(campos) < 2 {
				continue
			}
			if pid, err := strconv.Atoi(strings.Trim(campos[1], `"`)); err == nil {
				vivos[pid] = true
			}
		}
	}
	return vivos
}

// LimpiarPlantillasZombies borra las plantillas cuyo PID dueño ya no existe.
// `actual` es la plantilla del proceso llamador (nada la borra a sí misma).
// Es la barrida defensiva: sin ella, cada test interrumpido deja una base
// de 3–6 MB pegada al servidor para siempre.
func LimpiarPlantillasZombies(actual string) {
	control, err := ConectarServidorMySQL(configDePrueba(""))
	if err != nil {
		return
	}
	defer control.Close()

	filas, err := control.Query(`SELECT schema_name FROM information_schema.schemata
		WHERE schema_name LIKE 'erpgo_plantilla_%' AND schema_name <> ?`, actual)
	if err != nil {
		return
	}
	var candidatas []string
	for filas.Next() {
		var n string
		if err := filas.Scan(&n); err != nil {
			continue
		}
		candidatas = append(candidatas, n)
	}
	filas.Close()

	vivos := pidsVivos()
	if vivos == nil {
		// No se pudo saber qué PIDs están vivos: no borrar nada (el seguro
		// gana; el costo es otra corrida con basura, no una base destrozada).
		return
	}
	for _, n := range candidatas {
		partePID := n[strings.LastIndex(n, "_")+1:]
		pid, err := strconv.Atoi(partePID)
		if err != nil {
			continue // nombre no estándar: no tocar
		}
		if vivos[pid] {
			continue // su dueño vive: no es zombi
		}
		borrarBase(configDePrueba(n))
	}
}

// ClonarBase copia la estructura y la semilla de `origen` en `destino`.
//
// El nombre se valida antes de interpolarlo: es lo que hace
// `nombreBaseSeguro`, y un `DROP` por una comilla de más no se perdona.
func ClonarBase(origen, destino string) error {
	if err := validarEntornoPruebas(); err != nil {
		return err
	}
	if !esBaseDePrueba(origen) || !esBaseDePrueba(destino) {
		return fmt.Errorf("clonado de pruebas solo permitido entre bases erpgo_test_*")
	}
	if _, err := nombreBaseSeguro(origen); err != nil {
		return fmt.Errorf("base de origen: %w", err)
	}
	if _, err := nombreBaseSeguro(destino); err != nil {
		return fmt.Errorf("base de destino: %w", err)
	}

	control, err := ConectarServidorMySQL(configDePrueba(""))
	if err != nil {
		return err
	}
	defer control.Close()

	if _, err := control.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", destino)); err != nil {
		return fmt.Errorf("creando %s: %w", destino, err)
	}

	tablas, err := tablasDe(control, origen)
	if err != nil {
		return err
	}
	if len(tablas) == 0 {
		return fmt.Errorf("la base plantilla %s no tiene tablas: no se clonó nada", origen)
	}

	// TODO va en un solo lote. Con 139 tablas, mandarlas una a una son 139
	// viajes de ida y vuelta al servidor; juntas es uno. Medido sobre el
	// esquema real: 3,8 s por tabla-clon frente a 0,9 s por lote.
	var sql_ strings.Builder
	fmt.Fprintf(&sql_, "SET SESSION foreign_key_checks=0;\n")
	for _, t := range tablas {
		fmt.Fprintf(&sql_, "CREATE TABLE IF NOT EXISTS `%s`.`%s` LIKE `%s`.`%s`;\n",
			destino, t, origen, t)
		// La semilla se copia con INSERT IGNORE para que el clon sea
		// idempotente: repetirlo no parte nada por la mitad.
		fmt.Fprintf(&sql_, "INSERT IGNORE INTO `%s`.`%s` SELECT * FROM `%s`.`%s`;\n",
			destino, t, origen, t)
	}
	fmt.Fprintf(&sql_, "SET SESSION foreign_key_checks=1;\n")

	lote, err := ConectarServidorMySQL(ConfigMySQL{
		Host: hostDePrueba(), Port: puertoDePrueba(),
		Usuario: configDePrueba("").Usuario, Clave: configDePrueba("").Clave,
		MultiStatements: true, MaxConns: 2,
	})
	if err != nil {
		return err
	}
	defer lote.Close()
	if _, err := lote.Exec(sql_.String()); err != nil {
		return fmt.Errorf("clonando el esquema: %w", err)
	}
	return nil
}

// tablasDe devuelve las tablas de una base y si tienen filas.
func tablasDe(control *sql.DB, base string) ([]string, error) {
	filas, err := control.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'`, base)
	if err != nil {
		return nil, fmt.Errorf("leyendo las tablas de %s: %w", base, err)
	}
	defer filas.Close()
	var tablas []string
	for filas.Next() {
		var t string
		if err := filas.Scan(&t); err != nil {
			return nil, err
		}
		tablas = append(tablas, t)
	}
	return tablas, filas.Err()
}

// LimpiarPlantillasViejas borra las plantillas de otros procesos que llevan
// mas de `edad` horas.
//
// POR QUE POR ANTIGUEDAD Y NO "TODAS LAS QUE NO SON MIA".
//
// Con varios procesos de test a la vez, borrar "todas menos la mía" es
// exactamente la forma de romper a los demás. Y la razón de que la plantilla
// sobre en el servidor es que nadie la borró, no que sea nueva: cualquier
// proceso deja la suya al terminar, y mañana habrá una de cada corrida. Por eso
// solo se van las que llevan un día, que para entonces ya no la está usando
// nadie.
//
// LA ANTIGUEDAD SE LEE DE `information_schema.tables.create_time`, no del
// nombre. Antes el nombre llevaba el reloj y esta función lo parseaba; con el
// reloj fuera del nombre (para que la plantilla se pueda reutilizar) el parseo
// era imposible. Ojo: NO es `schemata.create_time` —esa columna no existe en
// MySQL 8.0, y la consulta vieja moría en silencio, de modo que la limpieza no
// borraba NUNCA una sola plantilla (acumuló 165 MB de bases huérfanas). La
// fecha buena es la de las tablas de la base: toda plantilla tiene tablas, y
// MIN(create_time) da su momento de nacimiento sin depender de la convención
// de nombres.
func LimpiarPlantillasViejas(actual string, edad time.Duration) {
	control, err := ConectarServidorMySQL(configDePrueba(""))
	if err != nil {
		return
	}
	defer control.Close()

	filas, err := control.Query(`SELECT s.schema_name, MIN(t.create_time)
		FROM information_schema.schemata s
		JOIN information_schema.tables t ON t.table_schema = s.schema_name
		WHERE s.schema_name LIKE 'erpgo_plantilla_%' AND s.schema_name != ?
		GROUP BY s.schema_name
		HAVING MIN(t.create_time) < (NOW() - INTERVAL ? HOUR)`, actual, int(edad.Hours()))
	if err != nil {
		return
	}
	var viejas []string
	for filas.Next() {
		var n string
		var cuando interface{}
		if err := filas.Scan(&n, &cuando); err != nil {
			continue
		}
		viejas = append(viejas, n)
	}
	filas.Close()

	for _, n := range viejas {
		borrarBase(configDePrueba(n))
	}
}
