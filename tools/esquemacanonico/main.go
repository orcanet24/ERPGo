package main

// GENERADOR DEL ESQUEMA CANÓNICO (Fase 1 del plan de esquema canónico).
//
// QUÉ HACE.
//
// Pega las 62 migraciones sobre una base scratch NUEVA con el MISMO runner que
// usa la producción (persistence.RunMigrationsForMySQL), y vuelca el resultado
// a migrations/esquema_canonico_mysql.sql: DDL de tablas + triggers + siembras.
//
// POR QUÉ REPRODUCIR LA CADENA EN VEZ DE FUSIONAR SQL A MANO.
//
// Fusionar 353 CREATE TABLE y 203 ALTER con regex es cómo se elige mal una
// definición y la base nueva queda distinta de la producción — el defecto
// exacto que la serie schema_reconstruido vino a tapar. Reproducir la cadena
// con el runner oficial da una propiedad fuerte: el canónico es, por
// construcción, lo que el runner produce hoy. Las 129 tablas definidas dos
// veces se resuelven solas: la última definición gana, que es la que la base
// real ya tiene.
//
// POR QUÉ UNA BASE SCRATCH Y NO LA REAL.
//
// El volcado se hace sobre una base recién creada, sin datos de operación: lo
// que se vuelca como INSERT es solo la siembra (menú, permisos, cuentas). La
// base real no se toca en ningún momento.
//
// VERIFICACIÓN INCLUIDA.
//
// Tras volcar, compara el scratch contra la base de referencia (la de bin/.env)
// columna por columna: si una tabla común difiere en columnas o índices, el
// tool avisa y termina con error. Las tablas que solo existen en la referencia
// (addon_hook_out, queue_tasks, guia_*) se reportan como creadas por código
// Go, no como fallo.
//
// Uso:
//
//	go run ./tools/esquemacanonico                    # construye y verifica
//	go run ./tools/esquemacanonico -sin-verificar     # solo construye
//	go run ./tools/esquemacanonico -mantener          # deja la base scratch
//	go run ./tools/esquemacanonico -referencia erpgo  # otra base de comparación

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"erpgo/infrastructure/persistence"
	"erpgo/interfaces/web"
	"erpgo/internal/queue"
)

const (
	tablaLedger = "schema_migrations" // el reconciliador del plan es su dueño
)

func main() {
	rutaEnv := flag.String("env", "bin/.env", "archivo .env del ERP con las credenciales")
	salida := flag.String("salida", "schema/esquema_canonico_mysql.sql", "archivo SQL canónico a escribir (fuera de migrations/: el runner aplica todos los .sql de esa carpeta)")
	scratch := flag.String("scratch", "erpgo_canon_build", "base temporal de reproducción")
	referencia := flag.String("referencia", "", "base de comparación (vacío = DB_NAME del .env)")
	sinVerificar := flag.Bool("sin-verificar", false, "no comparar contra la base de referencia")
	mantener := flag.Bool("mantener", false, "no borrar la base scratch al terminar")
	probar := flag.Bool("probar", false, "aplicar el canónico a una base fresca y verificarla contra la referencia")
	flag.Parse()

	if err := godotenv.Load(*rutaEnv); err != nil {
		fmt.Printf("AVISO: no se pudo leer %s (%v); se usan variables de entorno o banderas\n", *rutaEnv, err)
	}
	cfg := configuracion(*referencia)
	if cfg.Nombre == "" {
		salir(fmt.Errorf("no hay base de referencia: define DB_NAME en el .env o usa -referencia"))
	}

	// Conexión a la base de referencia (existe): sirve para crear el scratch.
	ref, err := persistence.OpenMySQL(persistence.ConfigMySQL{
		Host: cfg.Host, Port: cfg.Port, Usuario: cfg.Usuario, Clave: cfg.Clave, Nombre: cfg.Nombre,
	})
	if err != nil {
		salir(err)
	}
	defer ref.Close()
	if err := ref.Ping(); err != nil {
		salir(fmt.Errorf("no hay conexión con MySQL (%s:%d/%s): %w", cfg.Host, cfg.Port, cfg.Nombre, err))
	}

	fmt.Printf("Base de referencia: %s@%s:%d/%s\n", cfg.Usuario, cfg.Host, cfg.Port, cfg.Nombre)
	fmt.Printf("Base scratch: %s (se crea y se borra en cada corrida; -mantener la deja)\n\n", *scratch)

	if err := recrearScratch(ref, *scratch); err != nil {
		salir(err)
	}
	if !*mantener {
		defer func() {
			if _, err := ref.Exec("DROP DATABASE IF EXISTS " + *scratch); err != nil {
				fmt.Printf("AVISO: no se pudo borrar la base scratch %s: %v\n", *scratch, err)
			}
		}()
	}

	dbScratch, err := persistence.OpenMySQL(persistence.ConfigMySQL{
		Host: cfg.Host, Port: cfg.Port, Usuario: cfg.Usuario, Clave: cfg.Clave, Nombre: *scratch,
	})
	if err != nil {
		salir(err)
	}
	defer dbScratch.Close()

	// EL PASO QUE DA VALOR AL TOOL: es el runner oficial, con sus checksums,
	// su ledger, su tolerancia y sus triggers. Si el runner cambia, el
	// canónico se regenera y la verificación lo delata.
	fmt.Println("Reproduciendo la cadena de migraciones sobre la base scratch...")
	if err := persistence.RunMigrationsForMySQL(dbScratch, persistence.ConfigMySQL{
		Host: cfg.Host, Port: cfg.Port, Usuario: cfg.Usuario, Clave: cfg.Clave, Nombre: *scratch,
	}); err != nil {
		salir(err)
	}

	// Las tablas que el código crea fuera de migrations/ y que TODA
	// instalación tiene (BD de control, is_demo, cola). Sin esto el canónico
	// queda a tablas de distancia de la base real: la Fase 0 lo midió.
	if err := web.AplicarEsquemasDeCodigo(dbScratch); err != nil {
		salir(fmt.Errorf("esquemas de código (control): %w", err))
	}
	if err := queue.AplicarEsquema(dbScratch); err != nil {
		salir(fmt.Errorf("esquemas de código (cola): %w", err))
	}

	fmt.Println("\nVolcando el esquema canónico...")
	contenido, err := volcar(dbScratch, *scratch)
	if err != nil {
		salir(err)
	}
	if err := os.WriteFile(*salida, []byte(contenido), 0o644); err != nil {
		salir(err)
	}
	lineas := strings.Count(contenido, "\n")
	fmt.Printf("Escrito %s (%d líneas)\n", *salida, lineas)

	if *sinVerificar {
		return
	}
	fmt.Println("\nVerificando contra la base de referencia...")
	if err := verificar(ref, dbScratch, cfg.Nombre, *scratch); err != nil {
		// La salida con error no pasa por los defer de main (salir hace
		// os.Exit), así que la base scratch se borra aquí, en el único camino
		// que puede dejarla tirada.
		if !*mantener {
			_, _ = ref.Exec("DROP DATABASE IF EXISTS " + *scratch)
		}
		salir(err)
	}
	fmt.Println("\nVerificación: el canónico reproduce la base de referencia.")

	if *probar {
		fmt.Println("\nProbando el archivo canónico sobre una base fresca...")
		if err := probarCanonico(ref, cfg, *salida); err != nil {
			if !*mantener {
				_, _ = ref.Exec("DROP DATABASE IF EXISTS " + *scratch)
			}
			salir(err)
		}
	}
}

type creds struct {
	Host, Usuario, Clave, Nombre string
	Port                         int
}

func configuracion(referencia string) creds {
	c := creds{
		Host:    primeroNoVacio(os.Getenv("DB_HOST"), "127.0.0.1"),
		Usuario: primeroNoVacio(os.Getenv("DB_USER"), "root"),
		Clave:   os.Getenv("DB_PASSWORD"),
		Nombre:  primeroNoVacio(referencia, os.Getenv("DB_NAME")),
		Port:    3306,
	}
	fmt.Sscanf(primeroNoVacio(os.Getenv("DB_PORT"), "3306"), "%d", &c.Port)
	return c
}

func primeroNoVacio(valores ...string) string {
	for _, v := range valores {
		if v != "" {
			return v
		}
	}
	return ""
}

func salir(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}

func recrearScratch(ref *sql.DB, nombre string) error {
	if _, err := ref.Exec("DROP DATABASE IF EXISTS " + nombre); err != nil {
		return fmt.Errorf("no se pudo dropear la base scratch: %w", err)
	}
	// La collation del scratch es la misma a la que la normalización deja la
	// base real (medida en la Fase 0: 100 % utf8mb4_unicode_ci), para que el
	// volcado no la mezcle.
	if _, err := ref.Exec("CREATE DATABASE " + nombre + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return fmt.Errorf("no se pudo crear la base scratch: %w", err)
	}
	return nil
}

// volcar genera el SQL canónico: DDL de tablas, triggers y siembras.
func volcar(db *sql.DB, scratch string) (string, error) {
	var b strings.Builder
	b.WriteString("-- ESQUEMA CANÓNICO DEL ERPGo (MySQL)\n")
	b.WriteString("-- Generado por tools/esquemacanonico a partir de la cadena de migraciones.\n")
	b.WriteString("-- Vive en schema/ y NO en migrations/: el runner aplica todos los .sql de migrations/,\n")
	b.WriteString("-- y un canónico dentro de esa carpeta se aplicaría a sí mismo.\n")
	b.WriteString("-- Instalación limpia: este archivo monta tablas, triggers y siembras.\n")
	b.WriteString("-- Para bases ya existentes usa el reconciliador (Fase 2 del plan), no este archivo.\n")
	b.WriteString("-- Regenerar con: go run ./tools/esquemacanonico\n\n")
	b.WriteString("SET NAMES utf8mb4;\n")
	b.WriteString("SET FOREIGN_KEY_CHECKS=0;\n\n")

	tablas, err := listaTablas(db, scratch)
	if err != nil {
		return "", err
	}
	for _, t := range tablas {
		if t == tablaLedger {
			continue // el ledger lo lleva el reconciliador, no el canónico
		}
		ddl, err := showCreate(db, t)
		if err != nil {
			return "", err
		}
		b.WriteString(ddl + ";\n\n")
	}

	disparadores, err := volcarTriggers(db, scratch)
	if err != nil {
		return "", err
	}
	if disparadores != "" {
		b.WriteString("-- Triggers de protección de borrado (proteccion_borrado.go).\n")
		b.WriteString(disparadores + "\n")
	}

	b.WriteString("-- Siembras: catálogos y semillas que las migraciones INSERTan.\n")
	for _, t := range tablas {
		if t == tablaLedger {
			continue
		}
		inserts, err := volcarDatos(db, t)
		if err != nil {
			return "", err
		}
		if inserts != "" {
			b.WriteString(inserts + "\n")
		}
	}
	b.WriteString("SET FOREIGN_KEY_CHECKS=1;\n")
	return b.String(), nil
}

func listaTablas(db *sql.DB, scratch string) ([]string, error) {
	filas, err := db.Query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME", scratch)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var tablas []string
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			return nil, err
		}
		tablas = append(tablas, nombre)
	}
	return tablas, filas.Err()
}

func showCreate(db *sql.DB, tabla string) (string, error) {
	var nombre, ddl string
	if err := db.QueryRow("SHOW CREATE TABLE `"+tabla+"`").Scan(&nombre, &ddl); err != nil {
		return "", fmt.Errorf("SHOW CREATE TABLE %s: %w", tabla, err)
	}
	return ddl, nil
}

func volcarTriggers(db *sql.DB, scratch string) (string, error) {
	filas, err := db.Query(`SELECT TRIGGER_NAME, ACTION_TIMING, EVENT_MANIPULATION, EVENT_OBJECT_TABLE, ACTION_STATEMENT
		FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ? ORDER BY TRIGGER_NAME`, scratch)
	if err != nil {
		return "", err
	}
	defer filas.Close()
	var partes []string
	for filas.Next() {
		var nombre, momento, evento, tabla, cuerpo string
		if err := filas.Scan(&nombre, &momento, &evento, &tabla, &cuerpo); err != nil {
			return "", err
		}
		// Sin DEFINER: los triggers los crea el ERP sin definidor (verificado en
		// proteccion_borrado.go), así que son portables entre instalaciones.
		// El ";" final es obligatorio: el probador del canónico parte el archivo
		// con un splitter que entiende BEGIN...END, y necesita el cierre.
		partes = append(partes, fmt.Sprintf("DROP TRIGGER IF EXISTS `%s`;\nCREATE TRIGGER `%s` %s %s ON `%s` FOR EACH ROW\n%s;",
			nombre, nombre, momento, evento, tabla, cuerpo))
	}
	if err := filas.Err(); err != nil {
		return "", err
	}
	return strings.Join(partes, "\n\n"), nil
}

// volcarDatos genera INSERT de todas las filas de una tabla. En el scratch no
// hay datos de operación: todo lo que vuelca es siembra.
func volcarDatos(db *sql.DB, tabla string) (string, error) {
	columnas, err := columnasDe(db, tabla)
	if err != nil {
		return "", err
	}
	if len(columnas) == 0 {
		return "", nil
	}
	nombres := make([]string, len(columnas))
	for i, c := range columnas {
		nombres[i] = "`" + c + "`"
	}
	filas, err := db.Query("SELECT " + strings.Join(nombres, ", ") + " FROM `" + tabla + "`")
	if err != nil {
		return "", err
	}
	defer filas.Close()

	dest := make([]interface{}, len(columnas))
	for i := range dest {
		dest[i] = new(interface{})
	}
	var partes []string
	for filas.Next() {
		if err := filas.Scan(dest...); err != nil {
			return "", err
		}
		valores := make([]string, len(dest))
		for i, d := range dest {
			puntero, ok := d.(*interface{})
			if !ok {
				return "", fmt.Errorf("escaneo inesperado en %s: %T", tabla, d)
			}
			valores[i] = literalSQL(*puntero)
		}
		partes = append(partes, fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s);",
			tabla, strings.Join(nombres, ", "), strings.Join(valores, ", ")))
	}
	if err := filas.Err(); err != nil {
		return "", err
	}
	if len(partes) == 0 {
		return "", nil
	}
	return "-- " + tabla + " (" + fmt.Sprint(len(partes)) + " filas)\n" + strings.Join(partes, "\n"), nil
}

func columnasDe(db *sql.DB, tabla string) ([]string, error) {
	filas, err := db.Query("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION", tabla)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var columnas []string
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			return nil, err
		}
		columnas = append(columnas, nombre)
	}
	return columnas, filas.Err()
}

// literalSQL convierte un valor de MySQL a su literal SQL. El driver devuelve
// []byte para texto, int64 para enteros, float64 para decimales, time.Time para
// fechas y nil para NULL.
func literalSQL(v interface{}) string {
	switch valor := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return "'" + escapar(string(valor)) + "'"
	case string:
		return "'" + escapar(valor) + "'"
	case int64:
		return fmt.Sprint(valor)
	case float64:
		return fmt.Sprint(valor)
	case bool:
		if valor {
			return "1"
		}
		return "0"
	case time.Time:
		// Formato literal de MySQL, no String(): String() mete " +0000 UTC"
		// y el servidor lo rechaza con Error 1292.
		return "'" + valor.Format("2006-01-02 15:04:05.999999") + "'"
	default:
		return "'" + escapar(fmt.Sprint(valor)) + "'"
	}
}

func escapar(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// verificar compara el scratch (canónico) contra la base de referencia:
// columnas por tabla e índices. Las tablas que solo tiene la referencia son
// las creadas por código Go (Fase 4 del plan): se reportan, no fallan.
func verificar(ref, scratch *sql.DB, baseRef, baseScratch string) error {
	refCols := mapaColumnas(ref, baseRef)
	scrCols := mapaColumnas(scratch, baseScratch)

	var problemas []string
	var soloRef []string
	var ordenDistinto []string
	for tabla, columnas := range refCols {
		if tabla == tablaLedger {
			continue
		}
		destino, ok := scrCols[tabla]
		if !ok {
			soloRef = append(soloRef, tabla)
			continue
		}
		switch {
		case columnas == destino:
			// idénticas, incluido el orden
		case ordenColumnas(columnas) == ordenColumnas(destino):
			// Mismas columnas, distinto orden. Cosmético: no afecta a las
			// consultas (el ERP nombra columnas), pero se avisa porque un
			// volcado futuro podría ordenarlas distinto.
			ordenDistinto = append(ordenDistinto, tabla)
		default:
			problemas = append(problemas, fmt.Sprintf("%s: columnas distintas\n   referencia: %s\n   canónico:   %s",
				tabla, columnas, destino))
		}
	}
	var soloScratch []string
	for tabla := range scrCols {
		if _, ok := refCols[tabla]; !ok {
			soloScratch = append(soloScratch, tabla)
		}
	}
	sort.Strings(soloRef)
	sort.Strings(soloScratch)

	fmt.Printf("Tablas en la referencia: %d | en el canónico: %d\n", len(refCols)-1, len(scrCols))
	if len(soloRef) > 0 {
		fmt.Printf("\nSolo en la referencia (%d) — creadas por código Go, el canónico no las trae aún:\n", len(soloRef))
		for _, t := range soloRef {
			fmt.Printf("  %s\n", t)
		}
	}
	if len(soloScratch) > 0 {
		fmt.Printf("\nSolo en el canónico (%d) — revisar: no aparecen en la referencia:\n", len(soloScratch))
		for _, t := range soloScratch {
			fmt.Printf("  %s\n", t)
		}
	}
	if len(ordenDistinto) > 0 {
		fmt.Printf("\nColumnas iguales pero en distinto orden (%d) — cosmético, no frena:\n", len(ordenDistinto))
		for _, t := range ordenDistinto {
			fmt.Printf("  %s\n", t)
		}
	}
	if len(problemas) > 0 {
		return fmt.Errorf("el canónico difiere de la referencia en %d tabla(s):\n%s",
			len(problemas), strings.Join(problemas, "\n"))
	}
	fmt.Println("Columnas: idénticas en todas las tablas comunes.")
	return nil
}

// ordenColumnas ordena la lista "a,b,c" para comparar conjuntos sin importar
// el orden en que MySQL las devuelve.
func ordenColumnas(lista string) string {
	partes := strings.Split(lista, ",")
	sort.Strings(partes)
	return strings.Join(partes, ",")
}

// mapaColumnas devuelve "tabla" -> "col1|col2|..." en orden de posición.
func mapaColumnas(db *sql.DB, base string) map[string]string {
	filas, err := db.Query(`SELECT TABLE_NAME, GROUP_CONCAT(COLUMN_NAME ORDER BY ORDINAL_POSITION)
		FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? GROUP BY TABLE_NAME`, base)
	if err != nil {
		salir(err)
	}
	defer filas.Close()
	mapa := map[string]string{}
	for filas.Next() {
		var tabla, columnas string
		if err := filas.Scan(&tabla, &columnas); err != nil {
			salir(err)
		}
		mapa[tabla] = columnas
	}
	return mapa
}

// probarCanonico aplica el archivo canónico a una base NUEVA y la verifica
// contra la referencia. Es la prueba de que el ARTEFACTO (no solo la
// reproducción en memoria) monta el esquema completo: si el archivo se
// corrompe al escribirse o una sentencia queda mal, aquí salta.
//
// Se aplica sentencia a sentencia con un splitter que entiende BEGIN...END:
// los triggers llevan punto y coma dentro del cuerpo, y partir por ";\n" los
// rompería (la misma razón por la que el runner no aplica los triggers desde
// SQL).
func probarCanonico(ref *sql.DB, cfg creds, ruta string) error {
	const baseTest = "erpgo_canon_test"
	if _, err := ref.Exec("DROP DATABASE IF EXISTS " + baseTest); err != nil {
		return err
	}
	defer ref.Exec("DROP DATABASE IF EXISTS " + baseTest)
	if _, err := ref.Exec("CREATE DATABASE " + baseTest + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return err
	}

	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return err
	}
	dbTest, err := persistence.OpenMySQL(persistence.ConfigMySQL{
		Host: cfg.Host, Port: cfg.Port, Usuario: cfg.Usuario, Clave: cfg.Clave, Nombre: baseTest,
	})
	if err != nil {
		return err
	}
	defer dbTest.Close()
	for i, sentencia := range persistence.DividirSentencias(string(contenido)) {
		if _, err := dbTest.Exec(sentencia); err != nil {
			return fmt.Errorf("el canónico no se aplicó limpio sobre una base fresca (sentencia %d): %w\n%.200s", i+1, err, sentencia)
		}
	}
	fmt.Printf("El archivo canónico se aplicó limpio sobre %s.\n", baseTest)
	return verificar(ref, dbTest, cfg.Nombre, baseTest)
}

// dividirSentencias vive ahora en persistence (DividirSentencias): el tool y
// el reconciliador usan la misma implementación, no dos copias.
