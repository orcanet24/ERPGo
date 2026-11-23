package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MIGRACIONES EN MySQL.
//
// QUÉ CAMBIA RESPECTO A SQLite, Y POR QUÉ ES MÁS FÁCIL DE LO QUE PARECE.
//
// El camino de SQLite hacía tres cosas extra que aquí sobran:
//
//	 1. convertForSQLite(s, pkOf) — traducir el SQL de MySQL a SQLite
//	 2. collectMySQLPrimaryKeys()  — dos pasadas para sacar las PK que en
//	                                 los volcados de MySQL vienen en
//	                                 ALTER TABLE aparte
//	 3. isToleratedMySQLOnly()    — tirar a la basura ENGINE=InnoDB,
//	                                 CHARSET, ON DUPLICATE KEY...
//
// Las tres son traducción de MySQL a SQLite. En MySQL no hay nada que
// traducir: el esquema se aplica tal cual. Por eso este archivo es MÁS CORTO
// que el de SQLite, y no por[…]
// versión anterior de un motor distinto, sino porque este es el motor para el
// que se escribió el esquema.
//
// LO QUE SE CONSERVA, Y NO ES OpcIONAL: EL REGISTRO.
//
// Sin el registro, cada arranque ejecutaría las 300+ sentencias del esquema
// sobre datos reales. Con el registro, un archivo ya aplicado no se vuelve a
// ejecutar, y si una sentencia falla el archivo NO se anota para que se
// reintente. Es lo que evita que un `INSERT` sin `IF NOT EXISTS` acabe
// duplicando catálogos en cada reinicio.

// RunMigrationsForMySQL aplica los archivos de migrations/*.sql sobre una base
// MySQL y recuerda cuáles aplicó.
//
// Los archivos vienen embebidos en el binario, así que el ERP instala sin
// depender de tener la carpeta de migraciones al lado.
//
// cfg es la MISMA conexión que ya está abierta en db, y existe solo para poder
// abrir una segunda temporal con multiStatements (ver ConfigMySQL). Si esa
// segunda no se puede abrir —porque el usuario no tiene permiso, o porque el
// servidor es antiguo— el trabajo se hace igualmente, sentencia a sentencia, con
// la lentitud de antes. Perder 9 segundos en la instalación es aceptable; no
// instalar el ERP, no.
func RunMigrationsForMySQL(db *sql.DB, cfg ConfigMySQL) error {
	// ¿La base ya está en el modelo nuevo? El sello lo dice en campo sin
	// volcar el esquema, pero LA CADENA SIEMPRE CORRE: el ledger de
	// schema_migrations salta los archivos ya aplicados por su huella, así
	// que para una base al día el trabajo es una consulta rápida por archivo
	// (los 62 viejos quedan «ya aplicados») y SOLO se ejecutan los archivos
	// nuevos. Saltar la cadena cuando la base estaba sellada (decisión de la
	// Fase 3) cumplía el optimismo y rompía la promesa del README —«las
	// migraciones corren solas»—: una instalación existente nunca recibía
	// los menús, permisos ni catálogos nuevos, que es exactamente la deriva
	// que el plan de esquema canónico creía haber cerrado (hallazgo D-1,
	// auditoría 2026-11-22). El canónico aporta el DDL y las siembras
	// aditivas; la cadena aporta los cambios de VALORES sobre filas que ya
	// existen (un UPDATE de config no puede declararse como INSERT).
	sello, _ := SelloCanonico(db)
	sumaActual, sumaErr := ChecksumCanonico()
	switch {
	case sello == "":
		fmt.Println("Migrations (MySQL): base sin sellar; se aplica la cadena de migraciones.")
	case sumaErr == nil && sello == sumaActual:
		fmt.Println("Migrations (MySQL): base sellada con el DDL canónico vigente; la cadena repasa el ledger y se reconcilia.")
	default:
		fmt.Println("Migrations (MySQL): base sellada con un DDL canónico anterior; la cadena repasa el ledger y se reconcilia al vigente.")
	}

	{
		migs, err := readMigrations()
		if err != nil {
			return err
		}

		rapido, err := OpenMySQL(ConfigMySQL{
			Host: cfg.Host, Port: cfg.Port, Usuario: cfg.Usuario,
			Clave: cfg.Clave, Nombre: cfg.Nombre,
			MultiStatements: true, MaxConns: 2,
		})
		if err != nil {
			fmt.Printf("  esquema: se aplica sentencia a sentencia (multiStatements no disponible: %v)\n", err)
			rapido = nil
		} else {
			defer rapido.Close()
		}

		aplicados, reutilizados, sentencias, avisos := aplicarMigracionesMySQL(db, rapido, migs, true)
		fmt.Printf("Migrations (MySQL): %d archivos aplicados, %d ya aplicados, %d sentencias, %d avisos\n",
			aplicados, reutilizados, sentencias, avisos)
		if avisos > 0 {
			fmt.Println("  ATENCION: hubo sentencias que no se aplicaron. La aplicacion arranca,")
			fmt.Println("  pero revise este log antes de confiar en los datos.")
		}
		// La collation se normaliza DESPUÉS de aplicar el esquema, y no antes:
		// si se hiciera antes, los CREATE TABLE del propio esquema volverían a
		// dejar tablas con la collation que declaran (utf8mb3) y no se
		// arreglaría nada.
		if err := normalizarCollationMySQL(db, aplicados > 0); err != nil {
			return err
		}
	}

	// LA PROTECCIÓN DE BORRADO SE CREA AQUÍ, al final y en cada arranque.
	//
	// Este es el punto por donde pasa TODA base de empresa (arranque, alta de
	// empresa, asistente de instalación), así que ponerlo aquí evita que la
	// regla dependa de que seis sitios distintos se acuerden de aplicarla. Los
	// triggers consultan tablas del esquema, de ahí que vaya después de las
	// migraciones y no antes.
	//
	// NO ES FATAL. Un usuario de MySQL sin privilegio TRIGGER no puede crearlos, y
	// eso no puede impedir que el ERP arranque: se avisa en el log y sigue. Pero
	// el aviso va en dos líneas porque la consecuencia es real: la base deja de
	// impedir los borrados con movimiento y sólo quedan las guardas del handler.
	if err := AplicarProteccionUnaVez(db, "mysql"); err != nil {
		fmt.Printf("  AVISO: no se pudo activar la proteccion de borrado en la base: %v\n", err)
		fmt.Println("  La aplicacion arranca igual, pero la base NO esta impidiendo los borrados con movimiento.")
	}

	// Reconciliación con el esquema canónico (Fase 2 del plan de esquema
	// canónico): converged la base al DDL declarativo. En una base ya migrada
	// el diff es cero y no toca nada; en una base a medias, la deja completa.
	// Se hace DESPUÉS de la cadena y de los triggers: el canónico ya incluye
	// las tablas que el código crea, y los triggers los crea el mismo
	// AplicarProteccionUnaVez que acaba de correr.
	resultado, err := ReconciliarConCanonico(db, "mysql")
	if err != nil {
		fmt.Printf("  AVISO: no se pudo reconciliar con el esquema canónico: %v\n", err)
		return nil
	}
	cambios := len(resultado.TablasCreadas) + len(resultado.ColumnasAgregadas) + len(resultado.IndicesCreados)
	if cambios > 0 {
		fmt.Printf("  Esquema canónico: %d tablas creadas, %d columnas agregadas, %d índices creados\n",
			len(resultado.TablasCreadas), len(resultado.ColumnasAgregadas), len(resultado.IndicesCreados))
	}
	if len(resultado.SiembrasAplicadas) > 0 || resultado.SiembrasToleradas > 0 {
		fmt.Printf("  Siembras canónicas: %d filas nuevas, %d ya existían\n",
			len(resultado.SiembrasAplicadas), resultado.SiembrasToleradas)
	}
	for _, aviso := range resultado.Avisos {
		fmt.Printf("  AVISO esquema canónico: %s\n", aviso)
	}
	// Sello de la transición (Fase 3): deja constancia de que esta base ya
	// pasó por el reconciliador con el DDL canónico vigente. Se sella SOLO
	// cuando la reconciliación quedó limpia: un sello puesto sobre una base
	// con avisos diría «convergida» a una base que no convergió, y en campo
	// el sello es justo la prueba de eso. Sin sello la base simplemente
	// vuelve a intentarlo el arranque siguiente (la cadena, barata por el
	// ledger, también: ver arriba).
	if len(resultado.Avisos) > 0 {
		fmt.Println("  ATENCION: la base NO se sella porque la reconciliacion dejo avisos.")
		fmt.Println("  Se re-intentara en cada arranque hasta que quede limpia.")
	} else if err := SellarCanonico(db); err != nil {
		fmt.Printf("  AVISO: no se pudo sellar el esquema canónico en el ledger: %v\n", err)
	}
	return nil
}

// normalizarCollationMySQL deja TODAS las tablas de la base en utf8mb4_unicode_ci.
//
// POR QUÉ ESTO NO ES COSA ESTÉTICA.
//
// El esquema viene de un PHP de 2013 y sus tablas declaran lo que quieren:
// `DEFAULT CHARSET=utf8mb3`, `utf8mb4_general_ci`, y hay dos tablas en latin1. Las
// migraciones nuevas crean tablas en utf8mb4_unicode_ci. Conviven en la MISMA
// base, y MySQL se niega a comparar dos columnas con collation distinta:
//
//	Error 1267 (HY000): Illegal mix of collations
//	(utf8mb4_unicode_ci,IMPLICIT) and (utf8mb4_0900_ai_ci,IMPLICIT) for operation '='
//
// Ese error no salta al instalar: sale al usar el ERP, en la consulta que une dos
// tablas, y en el banco de pruebas se apareció en el cierre de un período
// contable —que compara el total de los asientos con el total de la factura—.
// Es decir: en contabilidad, y solo cuando ya hay datos.
//
// La conversión es `CONVERT TO CHARACTER SET`, que además de cambiar la etiqueta
// RE-CODIFICA los bytes. Eso importa: una tabla que estaba en latin1 contiene
// bytes distintos de los que dice su texto, y cambiar solo el charset produce
// basura. `CONVERT TO` lee los bytes con la collation vieja y los escribe con la
// nueva, que es lo único correcto.
//
// CUÁNDO SE HACE.
//
// Solo cuando hay algo nuevo que normalizar: la primera vez, o cuando el esquema
// cambia. En un arranque normal no se ejecuta, porque recorrer 129 tablas y
// preguntar el catálogo en cada arranque sería tirar el tiempo de arranque por la
// borda. Se registra como una entrada más de `schema_migrations`.
//
// SE MIRAN LAS COLUMNAS, NO SOLO LA TABLA.
//
// POR QUÉ LA PRIMERA VERSIÓN NO BASTABA. La collation de una tabla es solo su
// VALOR POR DEFECTO; el `CREATE TABLE` heredado del PHP le pone a muchas columnas
// su propio `CHARACTER SET ... COLLATE ...` (`migrations/schema.sql:199` declara
// `acc_vaucher.referenceNo` en utf8mb4_general_ci, y hay columnas en utf8mb3 y en
// latin1). Una tabla así puede tener el defecto ya en utf8mb4_unicode_ci —porque
// otra migración lo cambió— y sus columnas seguirlo teniendo en general_ci, con
// lo que el filtro por `table_collation` la daba por buena y NO la convertía.
//
// Consecuencia medida, no supuesta: en una base recién migrada quedaban cientos
// de columnas sin normalizar (`acc_vaucher.referenceNo` en utf8mb4_general_ci,
// `acc_transaction.*` en utf8mb3, `employee_salary_payment.*` en latin1), y el
// cierre de período moría con `Error 1267: Illegal mix of collations
// (utf8mb4_general_ci,IMPLICIT) and (utf8mb4_0900_ai_ci,IMPLICIT)` en la consulta
// que busca cada factura publicada y su asiento. El error hablaba de dos
// collations y la culpa era de una tercera que nadie miraba: el defecto era el
// filtro de esta función.
//
// La marca sube a `v2` para que las instalaciones que ya ejecutaron la `v1`
// vuelvan a revisarse: el defecto no estaba corregido allí, y un arranque normal
// no pasaba por aquí otra vez.
func normalizarCollationMySQL(db *sql.DB, esquemaNuevo bool) error {
	const marca = "zz_collation_utf8mb4"
	const marcaVersion = "v2"

	var checksum string
	err := db.QueryRow(
		`SELECT checksum FROM schema_migrations WHERE filename = ?`, marca).Scan(&checksum)
	if err == nil && checksum == marcaVersion && !esquemaNuevo {
		return nil
	}

	var pendientes []string
	filas, err := db.Query(
		`SELECT t.table_name FROM information_schema.tables t
		 WHERE t.table_schema = DATABASE() AND t.table_type = 'BASE TABLE'
		   AND (t.table_collation <> 'utf8mb4_unicode_ci'
		        OR EXISTS (SELECT 1 FROM information_schema.columns c
		                    WHERE c.table_schema = t.table_schema
		                      AND c.table_name = t.table_name
		                      AND c.collation_name IS NOT NULL
		                      AND c.collation_name <> 'utf8mb4_unicode_ci'))
		 ORDER BY t.table_name`)
	if err != nil {
		return fmt.Errorf("no se pudo listar las tablas para normalizar la collation: %w", err)
	}
	for filas.Next() {
		var t string
		if err := filas.Scan(&t); err != nil {
			continue
		}
		pendientes = append(pendientes, t)
	}
	filas.Close()

	if len(pendientes) > 0 {
		fmt.Printf("Collation: normalizando %d tablas a utf8mb4_unicode_ci\n", len(pendientes))
	}
	fallidas := 0
	// Al convertir un padre con FK (invoices) su hija (invoice_items) aún con
	// la collation vieja revienta la validación de la clave: los dos lados de
	// la FK deben coincidir MIENTRAS cada ALTER corre, que varían en el mismo
	// orden alfabético que MySQL elige. CONVERT TO re-declara las columnas por
	// tabla, así que con FOREIGN_KEY_CHECKS desactivado en la sesión la
	// conversión de un lado no es bloqueada por el otro, y al terminar ambas
	// quedan en la misma collation y la FK se revalida al restaurar el check.
	// Es la cura estándar de MySQL 8 para Error 3780 al normalizar collations.
	//
	// PERO EJECUTADO EN UNA SOLA CONEXIÓN (D-5, auditoría 2026-11-22):
	// FOREIGN_KEY_CHECKS es una variable de SESIÓN, y db.Exec sobre el pool
	// no garantiza la misma conexión: el SET podía correr en una sesión y
	// los ALTER en otra con los checks activados —el Error 3780 que el
	// comentario decía curado reaparecía sin explicación—. conn() fija un
	// hilo físico y todo el bloque corre por él.
	conn, err := db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("no se pudo tomar una conexión fija para normalizar collation: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return fmt.Errorf("no se pudo desactivar los checks de FK para normalizar collation: %w", err)
	}
	for _, t := range pendientes {
		// El nombre de la tabla no se interpola sin más: viene de
		// information_schema, así que es del propio servidor, pero el acento
		// grave lo delimita igual.
		if _, err := conn.ExecContext(context.Background(), fmt.Sprintf(
			"ALTER TABLE `%s` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", t)); err != nil {
			// Una tabla que no se puede convertir no debe impedir que las otras
			// sí se conviertan: se avisa y se sigue. El ERP funciona igual; lo que
			// queda es el aviso.
			fmt.Printf("  AVISO: %s no se pudo convertir a utf8mb4: %v\n", t, err)
			fallidas++
		}
	}
	if _, err := conn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=1"); err != nil {
		// Si el restore falla, la CONEXIÓN se descarta (Close tras un error
		// de sesión): el pool no debe devolver al ERP un hilo con los checks
		// apagados. Se reintenta explícitamente antes de cerrar.
		fmt.Printf("  AVISO: no se pudo reactivar los checks de FK: %v\n", err)
	}

	if _, err := db.Exec(`INSERT INTO schema_migrations (filename, checksum, statements, applied_at)
		VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE checksum = VALUES(checksum), applied_at = VALUES(applied_at)`,
		marca, marcaVersion, len(pendientes), time.Now().Format("2006-01-02 15:04:05")); err != nil {
		return fmt.Errorf("no se pudo registrar la normalización de collation: %w", err)
	}
	if fallidas > 0 {
		fmt.Printf("  ATENCION: %d tablas siguen sin normalizar; revise el log.\n", fallidas)
	}
	return nil
}

// aplicarMigracionesMySQL es el cuerpo, separado para poder probarlo con
// migraciones de mentira sin tocar los archivos reales.
func aplicarMigracionesMySQL(db *sql.DB, rapido *sql.DB, migs []migFile, verboso bool) (aplicados, reutilizados, sentencias, avisos int) {
	if err := asegurarRegistroMySQL(db); err != nil {
		fmt.Printf("  AVISO: no se pudo crear el registro de migraciones: %v\n", err)
		return 0, 0, 0, 0
	}
	registro := leerRegistroMySQL(db)

	for _, m := range migs {
		suma := migrationChecksum(m.content)

		// Ya aplicada y sin cambios: no se ejecuta ni una sentencia. Es el
		// caso normal de todos los arranques siguientes al primero.
		if previa, conocida := registro[m.name]; conocida && previa == suma {
			reutilizados++
			continue
		}

		modificada := false
		if previa, conocida := registro[m.name]; conocida && previa != suma {
			modificada = true
		}
		if verboso {
			switch {
			case modificada:
				fmt.Printf("Applying migrations/%s (MODIFICADA desde la última vez)...\n", m.name)
			default:
				fmt.Printf("Applying migrations/%s...\n", m.name)
			}
		}

		aplicadas, fallidas := aplicarSentenciasMySQL(db, rapido, m.content, verboso)
		sentencias += aplicadas
		avisos += fallidas

		if fallidas == 0 {
			registrarMigracionMySQL(db, m.name, suma, aplicadas)
			aplicados++
		} else if verboso {
			// Sin anotar a propósito: un archivo a medias se reintenta en el
			// siguiente arranque, que es lo único que puede arreglarlo.
			// Anotarlo dejaría el esquema incompleto para siempre.
			fmt.Printf("  NO ANOTADA (se reintentará al arrancar): migrations/%s\n", m.name)
		}
	}
	return
}

// aplicarSentenciasMySQL ejecuta un archivo de migración tal cual.
//
// A diferencia de SQLite, NO se hace translateStatement() ni
// collectMySQLPrimaryKeys(): son traducciones que solo tenían sentido al
// convertir de MySQL a SQLite.
//
// Sí se hace idempotencia básica (IF NOT EXISTS / IGNORE), porque los
// archivos heredados del PHP no son todos idempotentes y ejecutarlos sobre
// una base ya poblada debe ser inocuo.
//
// EL CAMINO RÁPIDO Y POR QUÉ HACE FALTA EL LENTO.
//
// Primero se manda el archivo ENTERO en una sola ida por la conexión con
// multiStatements. Cuando funciona —y funciona casi siempre, porque en una base
// nueva no hay nada que choque— son 16 viajes en vez de 415.
//
// Cuando falla no se da por bueno el archivo. En una base a medio migrar, la
// primera sentencia que ya existe detiene TODAS las siguientes, que es
// exactamente el caso en el que el camino rápido no puede usarse: MySQL no
// continúa después de un error dentro de un lote. Por eso entonces se repite
// archivo por archivo sentencia a sentencia, tolerando una por una las que ya
// estaban. El resultado es el mismo que antes y el error que se informa es el
// de la sentencia que falla de verdad, no el de la primera que chocó.
func aplicarSentenciasMySQL(db, rapido *sql.DB, contenido string, verboso bool) (aplicadas, fallidas int) {
	contenido = sinCambiarLaCollationDeLaConexion(contenido)
	sentencias := splitSQL(contenido)
	if rapido != nil {
		if err := ejecutarLote(rapido, contenido); err == nil {
			return len(sentencias), 0
		} else if esYaEstabaMySQL(err, "") {
			// El camino rápido falló solo por objetos ya existentes.
			// No imprimir ruido: el camino lento las tolera silenciosamente.
		} else if verboso {
			// Error real (no "ya existe"): informar para diagnóstico.
			fmt.Printf("  esquema: lote rechazado (%s), se aplica sentencia a sentencia\n",
				primeraLinea(err.Error()))
		}
	}

	for _, sentencia := range sentencias {
		// SentenciasEsquema deja los ALTER multi-cláusula como sentencias de
		// una cláusula cada una (D-2): ejecutado entero, un 1060 en la primera
		// columna abortaba las demás y el archivo quedaba anotado como
		// aplicado con la estructura a medias. Por cláusula, cada error se
		// clasifica sola.
		for _, s := range SentenciasEsquema(stripSQLComments(sentencia)) {
			if s == "" {
				continue
			}
			if _, err := db.Exec(s); err != nil {
				// Clasificación. "Ya estaba" NO es lo mismo que "no se aplicó".
				// En SQLite esa diferencia se confundía y el resumen se leía
				// como "ya estaba" cuando en realidad describía un fallo.
				if esYaEstabaMySQL(err, s) {
					continue
				}
				fmt.Printf("  migración: %v <- %.90s\n", err, s)
				fallidas++
				continue
			}
			aplicadas++
		}
	}
	return
}

// sinCambiarLaCollationDeLaConexion quita las sentencias `SET NAMES ...` de un
// archivo de migración.
//
// POR QUÉ SE QUITAN EN VEZ DE EJECUTARSE. `SET NAMES utf8mb4` —sin COLLATE, que
// es como lo escriben los diez archivos de `migrations/`— deja la collation del
// hilo en la que el SERVIDOR tenga por defecto para ese charset, y en MySQL 8
// esa es `utf8mb4_0900_ai_ci`, no la que declara el DSN
// (`collation=utf8mb4_unicode_ci`). El problema no es que cambie el charset
// —que era la intención— sino que se lleva por delante la collation de la
// conexión, y MySQL se niega a comparar una columna en `utf8mb4_unicode_ci` con
// una expresión en `utf8mb4_0900_ai_ci`:
//
//	Error 1267: Illegal mix of collations
//	(utf8mb4_unicode_ci,IMPLICIT) and (utf8mb4_0900_ai_ci,IMPLICIT) for operation '='
//
// Medido, no supuesto: en el banco de pruebas esa diferencia apareció entre la
// conexión que aplica las migraciones al arrancar y las que después consultan.
// Las migraciones corren sobre el MISMO pool que las consultas, así que las
// conexiones que aplicaron un archivo quedan con la collation cambiada y la
// consulta que después las usa —el cierre de período, por ejemplo— falla con un
// error que habla de collations y no dice que lo provocó un `SET NAMES`.
//
// Quitarlas no deja nada sin cubrir: el DSN ya pide por conexión
// `charset=utf8mb4&collation=utf8mb4_unicode_ci` (ver DSNMySQL), que es la misma
// intención con la collation correcta. Los archivos de migración se quedan como
// están —cambiar diez archivos históricos solo para esto obligaría a revisar sus
// checksums—: la neutralización es una sola vez, aquí.
func sinCambiarLaCollationDeLaConexion(contenido string) string {
	lineas := strings.Split(contenido, "\n")
	quedan := make([]string, 0, len(lineas))
	for _, l := range lineas {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(l)), "SET NAMES") {
			continue
		}
		quedan = append(quedan, l)
	}
	return strings.Join(quedan, "\n")
}

// primeraLinea devuelve solo la primera línea de un mensaje de error. Los de
// MySQL traen el SQL que lo provocó en líneas siguientes, y en el log del
// arranque eso son cientos de caracteres por archivo.
func primeraLinea(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ejecutarLote manda el archivo entero en una sola sentencia de ida y vuelta.
//
// Se quitan los comentarios antes porque el separador de sentencias del
// protocolo es el punto y coma: un `;` dentro de un comentario podría partir la
// sentencia en un sitio que no es el correcto. El servidor los toleraría, pero
// aquí no se le deja la duda.
func ejecutarLote(db *sql.DB, contenido string) error {
	// Las sentencias se normalizan antes de mandarlas: si el lote lleva una
	// `CREATE INDEX IF NOT EXISTS` sin traducir, MySQL rechaza TODO el lote por
	// sintaxis y se cae al camino lento sin necesidad.
	sentencias := splitSQL(stripSQLComments(contenido))
	var norm []string
	for _, s := range sentencias {
		// Incluye los ALTER multi-cláusula divididos por cláusula: si el lote
		// fracasa (siempre al primer "ya existe") el camino lento repite ya
		// con la granularidad correcta (D-2).
		norm = append(norm, SentenciasEsquema(s)...)
	}
	_, err := db.Exec(strings.Join(norm, ";\n"))
	return err
}

// esYaEstabaMySQL dice si el error es "eso ya existe" y no un fallo real.
//
// Recibe la SENTENCIA además del error porque hay un caso que el mensaje solo no
// distingue: "Duplicate entry" (1062).
//
//   - En un INSERT de siembra, significa "esa fila ya estaba": benigno, y los
//     archivos heredados están llenos de INSERTs con id fijo.
//   - En un ALTER/CREATE INDEX, significa "los DATOS chocan con la restricción
//     que acabo de pedir": hay que verlo. Un índice de integridad fiscal que no
//     se crea en silencio es la peor clase de fallo, porque deja creyendo que la
//     base impide números repetidos cuando no los impide.
//
// Decidir por el mensaje y no por la sentencia fue justo el error que dejaba
// pasar lo segundo como si fuera lo primero.
func esYaEstabaMySQL(err error, sentencia string) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, trozo := range []string{
		"duplicate key name",     // índice ya creado
		"already exists",         // tabla ya creada
		"duplicate column name",  // ADD COLUMN repetido
		"multiple primary key",   // PK ya definida
		"can't drop table",       // DROP IF EXISTS sobre algo que ya no está
		"foreign key constraint", // FK ya presente
		"errno 1061",             // duplicate key name (código)
	} {
		if strings.Contains(m, trozo) {
			return true
		}
	}
	// 1062 en un INSERT de siembra: la fila ya estaba.
	if strings.Contains(m, "duplicate entry") || strings.Contains(m, "errno 1062") {
		return esSentenciaDeSiembra(sentencia)
	}
	return false
}

// esSentenciaDeSiembra dice si la sentencia que falló solo INSERTA filas.
//
// Un INSERT no puede dejar a la base sin una restricción: como mucho repite un
// dato que ya estaba. Cualquier otra cosa (ALTER, CREATE INDEX, UPDATE) que
// choque con una UNIQUE sí cambia lo que la base garantiza, y se informa.
func esSentenciaDeSiembra(sentencia string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(sentencia)), "insert")
}

// asegurarRegistroMySQL crea la tabla que decide cuáles migraciones se aplican.
//
// Va aquí y no como un migrations/*.sql más porque es precisamente la tabla
// que decide si los demás archivos se ejecutan: si viviera en esa carpeta,
// aplicarla dependería de haber decidido si aplicarla.
func asegurarRegistroMySQL(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   VARCHAR(191) NOT NULL PRIMARY KEY,
		checksum   CHAR(64) NOT NULL DEFAULT '',
		statements INT NOT NULL DEFAULT 0,
		applied_at VARCHAR(32) NOT NULL DEFAULT ''
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

// leerRegistroMySQL lee el registro una sola vez por arranque.
func leerRegistroMySQL(db *sql.DB) map[string]string {
	salida := map[string]string{}
	filas, err := db.Query(`SELECT filename, checksum FROM schema_migrations`)
	if err != nil {
		// Sin registro legible se aplica todo, que es el comportamiento
		// seguro: nunca se salta una migración por no haber podido leer
		// el registro.
		return salida
	}
	defer filas.Close()
	for filas.Next() {
		var nombre, suma string
		if err := filas.Scan(&nombre, &suma); err == nil {
			salida[nombre] = suma
		}
	}
	return salida
}

// registrarMigracionMySQL anota el archivo como aplicado.
func registrarMigracionMySQL(db *sql.DB, nombre, suma string, sentencias int) {
	_, _ = db.Exec(`INSERT INTO schema_migrations (filename, checksum, statements, applied_at)
		VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE
			checksum = VALUES(checksum),
			statements = VALUES(statements),
			applied_at = VALUES(applied_at)`,
		nombre, suma, sentencias, time.Now().Format("2006-01-02 15:04:05"))
}
