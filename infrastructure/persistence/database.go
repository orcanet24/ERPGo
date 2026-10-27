package persistence

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"erpgow"

	_ "github.com/glebarez/go-sqlite"
)

// migFile is one embedded migration file, CRLF-normalized.
type migFile struct {
	name    string
	content string
}

// OpenSQLite opens (creating on demand) the SQLite database file. SQLite is
// the zero-config default for small businesses: just run the binary.
//
// Concurrency model (multi-user safe, no hangs, no lost writes):
//   - WAL journal mode: readers NEVER block the writer and vice versa; a
//     power failure cannot corrupt committed data.
//   - _txlock=immediate: every transaction takes the write lock up front, so
//     overlapping transactions queue instead of colliding with a stale
//     read snapshot (SQLITE_BUSY_SNAPSHOT is impossible).
//   - busy_timeout(10s): any query or transaction that finds the write lock
//     taken waits in line and retries — SQLite itself is the write queue.
//   - Pool acotado: lecturas concurrentes en paralelo bajo WAL; las
//     escrituras se serializan en el único lock de escritura. Las sentencias
//     del ERP tardan microsegundos, así que un puñado de terminales de POS
//     nunca percibe la espera. El escritor se serializa siempre porque SQLite
//     tiene un solo escritor por definición; por eso el código agrupa
//     escrituras en lotes (ver internal/queue).
func OpenSQLite(path string) (*sql.DB, error) {
	// Pragmas go in the DSN so EVERY pooled connection gets them (per-connection
	// settings like busy_timeout cannot be set from just one connection).
	// `foreign_keys(1)` está aquí por una razón concreta y medida: SQLite trae
	// las claves foráneas APAGADAS por defecto, así que una FK declarada en el
	// esquema no impide nada —se comprobó insertando un cliente con su factura y
	// borrando el cliente: el borrado pasaba y la factura quedaba huérfana—.
	// Hoy el esquema SQLite no declara ninguna (el conversor las descarta), así
	// que activarlo no cambia ningún comportamiento existente; lo que hace es que
	// el día que una se declare, la base la haga cumplir en vez de decorarla.
	dsn := path + "?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}
	const sqlitePool = 25
	db.SetMaxOpenConns(sqlitePool)
	db.SetMaxIdleConns(sqlitePool)
	db.SetConnMaxLifetime(0) // SQLite has no server timeout
	return db, nil
}

// CheckpointSQLiteWAL flushes the WAL back into the main database file so a
// raw copy of just the .sqlite3 file is complete. Call before backing up or
// retiring the file.
func CheckpointSQLiteWAL(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

// CreateSQLiteFile crea el archivo de base de datos de una empresa si no existe
// y devuelve el handle abierto. Las bases se nombran por el código de la
// empresa (slug): el aislamiento es un archivo por empresa, nunca una columna.
func CreateSQLiteFile(path string) (*sql.DB, error) {
	return OpenSQLite(path)
}

// RunMigrations applies all embedded migrations/*.sql files to a SQLite
// database (see RunMigrationsFor for the semantics).
func RunMigrations(db *sql.DB) error {
	return RunMigrationsFor(db)
}

// RunMigrationsFor applies the embedded migrations/*.sql files to a SQLite
// database, and remembers which ones it already applied.
//
// Lo que se hacía antes era más simple y por eso mismo era frágil: en cada
// arranque se ejecutaban LAS 269 SENTENCIAS de todos los archivos, y la
// idempotencia dependía de que cada una se hubiera escrito de forma
// idempotente (IF NOT EXISTS por aquí, un error tolerado por allá). Funcionaba
// mientras nadie escribiera una sentencia que no lo fuera: un
// `UPDATE` sin WHERE, un `INSERT` sin OR IGNORE o un `DROP` pasarían a
// ejecutarse en cada arranque, sobre datos, sin que nadie lo pidiera.
//
// El registro (schema_migrations) convierte "funciona por suerte" en "funciona
// porque está anotado": un archivo ya aplicado con el mismo contenido no se
// vuelve a ejecutar. Lo que sí cambia el juego:
//
//   - Si el contenido del archivo cambia, su suma cambia, y se vuelve a aplicar.
//     Editar una migración sigue significando "esto hay que ejecutarlo".
//   - Si una sentencia falla, el archivo NO se anota y se reintenta en el
//     siguiente arranque. Anotar un archivo a medio aplicar dejaría el esquema
//     en un estado que nadie volvería a mirar.
//
// Las instalaciones que ya existían no tienen el registro: en su primer
// arranque con esta versión se aplican todas las sentencias una última vez
// (idempotentes) y a partir de ahí el arranque no ejecuta nada.
func RunMigrationsFor(db *sql.DB) error {
	migs, err := readMigrations()
	if err != nil {
		return err
	}

	filesApplied, filesReused, statements, warnings := applyMigrations(db, migs, true)
	fmt.Printf("Migrations (sqlite): %d files applied, %d already applied, %d statements, %d warnings\n",
		filesApplied, filesReused, statements, warnings)
	if warnings > 0 {
		fmt.Printf("  WARNING: %d statements did not apply. The app starts anyway, but check this log before trusting the data.\n", warnings)
	}

	// La protección de borrado se crea aquí, con el esquema ya aplicado: sus
	// triggers consultan tablas que tienen que existir. Ver
	// proteccion_borrado.go para por qué es un trigger y no una clave foránea.
	if err := AplicarProteccionUnaVez(db, "sqlite"); err != nil {
		fmt.Printf("  AVISO: no se pudo activar la proteccion de borrado en la base: %v\n", err)
	}
	return nil
}

// readMigrations lee los archivos de migración embebidos, ya normalizados.
//
// Normalizar los finales de línea no es cosmético: splitSQL parte por ";\n", así
// que un archivo guardado con CRLF (";\r\n") no se partiría nunca y el archivo
// entero se ejecutaría como una sola sentencia rota.
func readMigrations() ([]migFile, error) {
	entries, err := fs.ReadDir(erpgow.Migrations(), "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	var migs []migFile
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		content, err := fs.ReadFile(erpgow.Migrations(), "migrations/"+f.Name())
		if err != nil {
			fmt.Printf("  WARN: could not read %s: %v\n", f.Name(), err)
			continue
		}
		migs = append(migs, migFile{f.Name(), strings.ReplaceAll(string(content), "\r\n", "\n")})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].name < migs[j].name })

	// El esquema base (schema.sql) va SIEMPRE primero: crea las tablas que los
	// demás archivos ALTERAN o llenan. fs.ReadDir es lexicográfico y
	// "schema.sql" acaba último ("schema_cash..." < "schema.sql"), así que sin
	// esto los ALTER de cash_session, debt_collection, menu, invoice_payment_summary
	// corrían antes del CREATE y caían con Error 1146/1072; en una instalación
	// nueva eran 56 avisos y tablas nacidas sin sus columnas.
	for i, m := range migs {
		if m.name == "schema.sql" {
			migs[0], migs[i] = migs[i], migs[0]
			break
		}
	}
	return migs, nil
}

// applyMigrations applies the given files and updates the ledger.
//
// Va separado de RunMigrationsFor para poder probarlo con migraciones de
// mentira: el comportamiento interesante —no repetir lo ya aplicado, repetir
// lo que cambió, no anotar lo que falló— no se puede comprobar con los archivos
// reales, porque están bien escritos por definición.
//
// verbose=false silencia el log: los tests no necesitan que el arranque talk.
func applyMigrations(db *sql.DB, migs []migFile, verbose bool) (filesApplied, filesReused, statements, warnings int) {
	if err := ensureMigrationLedger(db); err != nil {
		fmt.Printf("  WARN: no se pudo crear el registro de migraciones: %v\n", err)
		return 0, 0, 0, 0
	}
	ledger := loadMigrationLedger(db)

	// Two-pass: primary keys are declared via later "ALTER TABLE ... ADD
	// PRIMARY KEY" statements in the MySQL dumps, but SQLite needs them in
	// the CREATE TABLE itself, so collect them all before applying.
	pkOf := collectMySQLPrimaryKeys(migs)

	for _, m := range migs {
		sum := migrationChecksum(m.content)

		// Ya aplicada y sin cambios: no se ejecuta una sola sentencia. Es el
		// caso normal de todos los arranques siguientes al primero.
		previous, known := "", false
		if v, ok := ledger[m.name]; ok {
			previous, known = v, true
		}
		if known && previous == sum {
			filesReused++
			continue
		}

		if verbose {
			if known {
				// El archivo cambió desde la última vez. Se vuelve a aplicar
				// porque las sentencias son idempotentes, y se dice en voz
				// alta: alguien editó una migración, y eso es raro.
				fmt.Printf("Applying migrations/%s (MODIFIED since last run)...\n", m.name)
			} else {
				fmt.Printf("Applying migrations/%s...\n", m.name)
			}
		}

		applied, _, failed := applyStatementsFor(db, m.content, pkOf)
		statements += applied
		warnings += failed

		if failed == 0 {
			recordMigration(db, m.name, sum, applied)
			filesApplied++
		} else if verbose {
			// Sin anotar a propósito: un archivo con sentencias sin aplicar se
			// reintentará en el próximo arranque, que es lo único que puede
			// arreglarlo. Anotarlo dejaría el esquema incompleto para siempre.
			fmt.Printf("  NOT RECORDED (will retry on next start): migrations/%s\n", m.name)
		}
	}

	return filesApplied, filesReused, statements, warnings
}

// ensureMigrationLedger crea el registro de migraciones aplicadas.
//
// Va aquí y no como un migrations/*.sql más porque es precisamente la tabla
// que decide si los demás archivos se ejecutan: si viviera en esa carpeta,
// aplicarla dependería de haber decidido si aplicarla.
func ensureMigrationLedger(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   TEXT NOT NULL PRIMARY KEY,
		checksum   TEXT NOT NULL DEFAULT '',
		statements INTEGER NOT NULL DEFAULT 0,
		applied_at TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

// loadMigrationLedger lee el registro una sola vez por arranque. Es una consulta
// pequeña (una fila por archivo) y evita una por archivo.
func loadMigrationLedger(db *sql.DB) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT filename, checksum FROM schema_migrations`)
	if err != nil {
		// Sin registro legible se aplica todo, que es el comportamiento
		// seguro: nunca se salta una migración por no haber podido leer el
		// registro.
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err == nil {
			out[name] = sum
		}
	}
	return out
}

// recordMigration anota un archivo como aplicado.
//
// ESTE FICHERO ES EL CAMINO LEGACY DE SQLite.
//
// No se puede usar `ON DUPLICATE KEY UPDATE` aquí: eso es sintaxis de MySQL.
// En SQLite el upsert es `ON CONFLICT(...) DO UPDATE`. Cuando este INSERT se
// convierte por error al dialecto equivocado, la sentencia falla, el error se
// descarta con `_ =` y el registro se queda VACÍO — que es el peor fallo
// posible: no da error, pero hace que las migraciones se reapliquen en cada
// arranque, que es justo lo que este archivo existe para evitar.
func recordMigration(db *sql.DB, name, sum string, statements int) {
	_, _ = db.Exec(`INSERT INTO schema_migrations (filename, checksum, statements, applied_at)
		VALUES (?,?,?,?)
		ON CONFLICT(filename) DO UPDATE SET
			checksum = excluded.checksum,
			statements = excluded.statements,
			applied_at = excluded.applied_at`,
		name, sum, statements, time.Now().Format("2006-01-02 15:04:05"))
}

// migrationChecksum resume el contenido de un archivo. Se calcula sobre el texto
// ya normalizado (sin CRLF) para que un cambio de fin de línea de Windows a
// Unix no cuente como una migración modificada.
func migrationChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// applyStatementsFor executes a set of SQL statements idempotently,
// translating to SQLite when needed.
func applyStatementsFor(db *sql.DB, content string, pkOf map[string]string) (applied, skipped, failed int) {
	for _, stmt := range splitSQL(content) {
		s := stripSQLComments(stmt)
		if s == "" {
			continue
		}
		for _, out := range convertForSQLite(s, pkOf) {
			a, k, f := execOne(db, out)
			applied += a
			skipped += k
			failed += f
		}
	}
	return
}

// execOne runs one statement and classifies its result.
//
// La clasificación importa más de lo que parece. Las migraciones vienen del
// PHP y están escritas en MySQL; SQLite no entiende todo. Antes, CUALQUIER error
// ("near", "syntax error", "no such column") se contaba como "skipped", que en
// el resumen se leía como "ya estaba" y que en realidad significa "esto NO se
// aplicó". Una columna que nunca se agrega es una columna que un handler
// encuentra faltante en producción, y el log no decía nada.
//
// La distinction que se aplica ahora:
//   - applied : se ejecutó
//   - skipped : ya estaba (no hay nada que hacer)
//   - tolerated: la sentencia no tiene equivalente en SQLite y su ausencia no
//     rompe nada (un índice que SQLite crea distinto, un ENGINE=InnoDB, un
//     CHARACTER SET). Se toleran POR NOMBRE, no por mensaje genérico.
//   - failed  : no se aplicó y no sabemos por qué. Se cuenta y se avisa.
func execOne(db *sql.DB, s string) (applied, skipped, failed int) {
	if _, err := db.Exec(s); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "Duplicate") || strings.Contains(msg, "already exists") ||
			strings.Contains(msg, "duplicate column name") ||
			strings.Contains(msg, "Multiple primary key"):
			// "duplicate column name" va aparte porque SQLite lo dice en
			// minúscula y el resto en mayúscula: sin esta línea, un
			// ALTER TABLE ADD COLUMN sobre una columna que ya existe se
			// contaba como fallo y ensuciaba el arranque con avisos que en
			// realidad describían el estado deseado.
			return 0, 1, 0
		case isToleratedMySQLOnly(s):
			// Ya estaba handled arriba; esto es SQL que solo tiene sentido en MySQL.
			return 0, 1, 0
		}
		// Cualquier otro error se reporta. Puede ser una incompatibilidad real
		// del esquema heredado, y prefiero que se vea a que se esconda.
		fmt.Printf("  migration warn: %v <- %.80s\n", err, s)
		return 0, 0, 1
	}
	return 1, 0, 0
}

// isToleratedMySQLOnly dice si una sentencia es SQL que solo existe en MySQL y
// que su ausencia no deja la base inservible.
//
// Se decide por la SENTENCIA, no por el mensaje de error. La razón: "near X" no
// distingue entre "esto es MySQL y SQLite no lo entiende" —tolerable— y
// "esto está mal escrito" —un fallo que hay que ver—. Con la lista explícita, la
// diferencia entre las dos cosas deja de depender del texto que SQLite imprimiría
// hoy y sigue siendo legible dentro de seis meses.
func isToleratedMySQLOnly(s string) bool {
	u := strings.ToUpper(s)
	for _, frag := range []string{
		"ENGINE=", "CHARACTER SET", "ON UPDATE CURRENT_TIMESTAMP",
		// "COLLATE" sin "=" porque el esquema escribe "COLLATE utf8_general_ci":
		// un "=" habría dejado pasar la forma más común sin querer.
		"COLLATE",
		"USING BTREE", "KEY `", "FULLTEXT KEY", "SPATIAL KEY",
		"UNSIGNED", "AUTO_INCREMENT", "ON DUPLICATE KEY",
	} {
		if strings.Contains(u, frag) {
			return true
		}
	}
	return false
}

// applyStatements executes a set of SQL statements idempotently.
func applyStatements(db *sql.DB, content string) (applied, skipped, failed int) {
	return applyStatementsFor(db, content, nil)
}

// splitSQL splits on semicolons at end of line (safe: no procedures/triggers).
func splitSQL(s string) []string {
	var parts []string
	for _, chunk := range strings.Split(s, ";\n") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		chunk = strings.TrimSuffix(chunk, ";")
		parts = append(parts, chunk)
	}
	return parts
}

// stripSQLComments removes leading full-line `--` and `/* */` comment lines so
// a statement like "-- comment\nCREATE TABLE ..." is executed, not skipped.
// (Comments AFTER the SQL on the same line are preserved: MySQL accepts them.)
func stripSQLComments(stmt string) string {
	lines := strings.Split(stmt, "\n")
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "--") || strings.HasPrefix(t, "/*") {
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// transformStatement makes a statement idempotent.
func transformStatement(s string) string {
	upper := strings.ToUpper(s)
	switch {
	case strings.HasPrefix(upper, "CREATE TABLE IF NOT EXISTS "):
		return s // already idempotent
	case strings.HasPrefix(upper, "CREATE TABLE "):
		return "CREATE TABLE IF NOT EXISTS " + s[len("CREATE TABLE "):]
	case strings.HasPrefix(upper, "INSERT INTO "):
		// `INSERT OR IGNORE` es SQLite. El `INSERT IGNORE` a secas es MySQL y
		// aquí no funciona.
		return "INSERT OR IGNORE INTO " + s[len("INSERT INTO "):]
	default:
		return s
	}
}
