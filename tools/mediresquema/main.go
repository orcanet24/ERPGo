package main

// MEDICIÓN DEL ESQUEMA REAL (Fase 0 del plan de esquema canónico).
//
// POR QUÉ EXISTE ESTE TOOL.
//
// Las cifras del plan vinieron de leer los 62 archivos SQL con regex. Falta
// la otra mitad: cuántas tablas y columnas quedan EFECTIVAMENTE en la base
// real tras aplicarlas, y qué dice el ledger schema_migrations. Sin esa foto,
// consolidar el esquema sería opinar sobre el papel; con ella, las 129
// tablas duplicadas se resuelven contra evidencia.
//
// El tool NO escribe nada en la base: solo lee. Y lee las credenciales del
// .env del ERP (por defecto bin/.env) sin imprimirlas en la consola: el .env
// es un secreto local y no tiene por qué viajar por el historial de la
// terminal.
//
// Uso:
//
//	go run ./tools/mediresquema                 # lee bin/.env
//	go run ./tools/mediresquema -env otro.env   # .env alternativo
//	go run ./tools/mediresquema -host 127.0.0.1 -db erpgo -u root -p "..."

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var (
	patronCreate = regexp.MustCompile("(?i)CREATE TABLE (?:IF NOT EXISTS )?[`\"]?([A-Za-z0-9_]+)[`\"]?")
	patronAlter  = regexp.MustCompile("(?i)ALTER TABLE [`\"]?([A-Za-z0-9_]+)[`\"]?")
)

func main() {
	rutaEnv := flag.String("env", "bin/.env", "archivo .env del ERP con las credenciales")
	host := flag.String("host", "", "host MySQL (vacío = tomar DB_HOST del .env)")
	puerto := flag.Int("port", 0, "puerto MySQL (0 = tomar DB_PORT del .env)")
	usuario := flag.String("u", "", "usuario (vacío = tomar DB_USER del .env)")
	clave := flag.String("p", "", "clave (vacía = tomar DB_PASSWORD del .env)")
	base := flag.String("db", "", "base a medir (vacío = tomar DB_NAME del .env)")
	soloBases := flag.Bool("solo-bases", false, "solo listar las bases del servidor y salir")
	soloTablas := flag.Bool("solo-tablas", false, "solo listar las tablas de la base y salir")
	flag.Parse()

	// godotenv.Load NO sobreescribe variables ya definidas: el -p explícito
	// gana sobre el .env, que es lo que quiere un tool de medición.
	if err := godotenv.Load(*rutaEnv); err != nil {
		fmt.Printf("AVISO: no se pudo leer %s (%v); se usan variables de entorno o banderas\n", *rutaEnv, err)
	}
	cfg := struct {
		host, usuario, clave, base string
		puerto                     int
	}{
		host:    primeroNoVacio(*host, os.Getenv("DB_HOST"), "127.0.0.1"),
		usuario: primeroNoVacio(*usuario, os.Getenv("DB_USER"), "root"),
		clave:   *clave,
		base:    primeroNoVacio(*base, os.Getenv("DB_NAME"), "erpgo"),
		puerto:  *puerto,
	}
	if cfg.puerto == 0 {
		fmt.Sscanf(primeroNoVacio(os.Getenv("DB_PORT"), "3306"), "%d", &cfg.puerto)
	}
	if cfg.clave == "" {
		cfg.clave = os.Getenv("DB_PASSWORD")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true", cfg.usuario, cfg.clave, cfg.host, cfg.puerto, cfg.base)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		salir(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		salir(fmt.Errorf("no hay conexión con MySQL (%s:%d/%s): %w", cfg.host, cfg.puerto, cfg.base, err))
	}

	fmt.Printf("Base medida: %s@%s:%d\n\n", cfg.base, cfg.host, cfg.puerto)

	if *soloBases {
		filas, err := db.Query("SHOW DATABASES")
		if err != nil {
			salir(err)
		}
		defer filas.Close()
		fmt.Println("Bases del servidor:")
		for filas.Next() {
			var nombre string
			if err := filas.Scan(&nombre); err != nil {
				salir(err)
			}
			fmt.Printf("  %s\n", nombre)
		}
		return
	}

	if *soloTablas {
		filas, err := db.Query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME", cfg.base)
		if err != nil {
			salir(err)
		}
		defer filas.Close()
		fmt.Printf("Tablas de %s:\n", cfg.base)
		for filas.Next() {
			var nombre string
			if err := filas.Scan(&nombre); err != nil {
				salir(err)
			}
			fmt.Printf("  %s\n", nombre)
		}
		return
	}

	tablas, _, _ := medirBase(db, cfg.base)
	collation(db, cfg.base)
	volcarLedger(db)
	basesEmpresa(db)
	compararConArchivos(cfg.base, tablas)
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

// medirBase cuenta tablas, columnas e índices efectivos de la base.
func medirBase(db *sql.DB, base string) (tablas, columnas, indices map[string]int) {
	tablas = map[string]int{}
	filas, err := db.Query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME", base)
	if err != nil {
		salir(err)
	}
	defer filas.Close()
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			salir(err)
		}
		tablas[nombre] = 0
	}
	fmt.Printf("Tablas efectivas en la base: %d\n", len(tablas))

	filasC, err := db.Query("SELECT TABLE_NAME, COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? GROUP BY TABLE_NAME", base)
	if err != nil {
		salir(err)
	}
	defer filasC.Close()
	total := 0
	for filasC.Next() {
		var nombre string
		var n int
		if err := filasC.Scan(&nombre, &n); err != nil {
			salir(err)
		}
		tablas[nombre] = n
		total += n
	}
	fmt.Printf("Columnas efectivas: %d\n", total)

	filasI, err := db.Query("SELECT COUNT(DISTINCT TABLE_NAME, INDEX_NAME) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ?", base)
	if err != nil {
		salir(err)
	}
	defer filasI.Close()
	if filasI.Next() {
		var n int
		if err := filasI.Scan(&n); err != nil {
			salir(err)
		}
		indices = map[string]int{"_": n}
		fmt.Printf("Índices efectivos: %d\n\n", n)
	}
	return tablas, nil, indices
}

// collation mezclada es el error 1267 de MySQL: dos columnas con collation
// distinta no se pueden comparar. La normalización ya corre en cada arranque,
// pero la medición deja constancia de si la base quedó uniforme.
func collation(db *sql.DB, base string) {
	filas, err := db.Query("SELECT TABLE_COLLATION, COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? GROUP BY TABLE_COLLATION ORDER BY 2 DESC", base)
	if err != nil {
		salir(err)
	}
	defer filas.Close()
	fmt.Println("Collation por tabla:")
	for filas.Next() {
		var col string
		var n int
		if err := filas.Scan(&col, &n); err != nil {
			salir(err)
		}
		fmt.Printf("  %-28s %d tablas\n", col, n)
	}
	fmt.Println()
}

// volcarLedger muestra lo que la base recuerda haber aplicado. Es el insumo
// de la transición: una base sin filas aquí es una base anterior al ledger.
func volcarLedger(db *sql.DB) {
	filas, err := db.Query("SELECT filename, applied_at, checksum FROM schema_migrations ORDER BY filename")
	if err != nil {
		fmt.Println("Ledger schema_migrations: NO EXISTE (base anterior al ledger)")
		fmt.Println()
		return
	}
	defer filas.Close()
	fmt.Println("Ledger schema_migrations (archivo, fecha, checksum):")
	n := 0
	for filas.Next() {
		var nombre, suma string
		var fecha sql.NullString
		if err := filas.Scan(&nombre, &fecha, &suma); err != nil {
			salir(err)
		}
		corta := suma
		if len(corta) > 12 {
			corta = corta[:12]
		}
		fmt.Printf("  %-58s %-20s %s\n", nombre, fecha.String, corta)
		n++
	}
	fmt.Printf("  Total: %d entradas\n\n", n)
}

// basesEmpresa lista las bases erpgo_<código>: la reconciliación del plan
// debe correr también contra cada una de ellas, no solo contra la de control.
func basesEmpresa(db *sql.DB) {
	filas, err := db.Query("SHOW DATABASES LIKE 'erpgo\\_%'")
	if err != nil {
		fmt.Printf("No se pudieron listar las bases de empresa: %v\n\n", err)
		return
	}
	defer filas.Close()
	fmt.Println("Bases de empresa (erpgo_<código>):")
	n := 0
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			salir(err)
		}
		fmt.Printf("  %s\n", nombre)
		n++
	}
	if n == 0 {
		fmt.Println("  (ninguna: la instalación solo usa la base principal)")
	}
	fmt.Println()
}

// compararConArchivos cruza las tablas efectivas de la base con las que los
// 62 SQL declaran crear. Lo que sobra o falta en la base es exactamente la
// desviación que la Fase 1 tiene que explicar (tablas creadas por código Go,
// tablas de la versión PHP que el runner no llegó a aplicar, etc.).
func compararConArchivos(base string, reales map[string]int) {
	archivos, err := filepath.Glob(filepath.Join("migrations", "*.sql"))
	if err != nil || len(archivos) == 0 {
		fmt.Println("No se encontraron migrations/*.sql para comparar")
		return
	}
	declaradas := map[string]int{}
	alteradas := map[string]int{}
	for _, ruta := range archivos {
		contenido, err := os.ReadFile(ruta)
		if err != nil {
			continue
		}
		// Las líneas de comentario ('-- ...', '/* ... */') traen frases
		// técnicas ("CREATE TABLE IF NOT EXISTS y ADD COLUMN...") que el regex
		// capturaría como nombres de tabla. Se saltan: la medición es de DDL
		// real, no de prosa de los encabezados.
		for _, linea := range strings.Split(string(contenido), "\n") {
			limpio := strings.TrimSpace(linea)
			if limpio == "" || strings.HasPrefix(limpio, "--") ||
				strings.HasPrefix(limpio, "/*") || strings.HasPrefix(limpio, "*") {
				continue
			}
			for _, m := range patronCreate.FindAllStringSubmatch(limpio, -1) {
				declaradas[strings.ToLower(m[1])]++
			}
			for _, m := range patronAlter.FindAllStringSubmatch(limpio, -1) {
				alteradas[strings.ToLower(m[1])]++
			}
		}
	}
	fmt.Printf("Tablas declaradas en migrations/*.sql: %d (con %d CREATE repetidos)\n",
		len(declaradas), totalRepetidos(declaradas))

	faltan := []string{}
	for nombre := range declaradas {
		if _, ok := reales[strings.ToLower(nombre)]; !ok {
			faltan = append(faltan, nombre)
		}
	}
	sort.Strings(faltan)
	if len(faltan) > 0 {
		fmt.Printf("\nDeclaradas en SQL pero AUSENTES en la base (%d):\n", len(faltan))
		for _, nombre := range faltan {
			fmt.Printf("  %s (ALTERs previos: %d)\n", nombre, alteradas[nombre])
		}
	}

	sobran := []string{}
	for nombre := range reales {
		if _, ok := declaradas[nombre]; !ok {
			sobran = append(sobran, nombre)
		}
	}
	sort.Strings(sobran)
	if len(sobran) > 0 {
		fmt.Printf("\nEn la base pero NO declaradas en migrations/*.sql (%d):\n", len(sobran))
		for _, nombre := range sobran {
			fmt.Printf("  %s (%d columnas)\n", nombre, reales[nombre])
		}
	}
}

func totalRepetidos(conteos map[string]int) int {
	n := 0
	for _, c := range conteos {
		if c > 1 {
			n += c - 1
		}
	}
	return n
}
