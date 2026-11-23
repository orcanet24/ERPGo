package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// AplicarEsquema ejecuta un conjunto de sentencias de esquema (las que viven
// en el código y no en migrations/*.sql) y tolera que ya estén.
//
// POR QUÉ EXISTE ESTA FUNCIÓN Y NO UN `CREATE INDEX IF NOT EXISTS`
//
// MySQL no admite `IF NOT EXISTS` en `CREATE INDEX`. Solo lo admite en
// `CREATE TABLE` y en `CREATE DATABASE`. En SQLite sí funciona, y el código
// venía de ahí.
//
// Sin esto, el arranque va bien la PRIMERA vez y falla la SEGUNDA: la tabla se
// crea, el índice ya existe, y MySQL contesta "duplicate key name". Un fallo
// que solo aparece al segundo arranque es de los que nadie encuentra, porque
// la instalación "funciona" y luego el servidor deja de arrancar al
// reiniciarlo.
//
// Por eso aquí no se intenta adivinar: se ejecuta, y si el error dice "ya
// existía", se da por buena. Es la misma idea que el registro de migraciones,
// pero para los esquemas que viven en el código.
//
// # LO QUE NO SE TOLERA
//
// Cualquier otro error se devuelve. Un esquema a medias que se da por bueno
// es peor que un esquema que falla: la aplicación arrancaría contra una base
// a la que le falta media estructura, y el fallo saldría más tarde, en una
// venta, en vez de aquí.
func AplicarEsquema(db *sql.DB, sentencias []string) error {
	if db == nil {
		return fmt.Errorf("sin base de datos")
	}
	for _, s := range sentencias {
		if strings.TrimSpace(s) == "" {
			continue
		}
		// Por cláusula: un ALTER multi-cláusula cuya primera columna ya
		// existía abortaba MySQL entero y las demás nunca se creaban (D-2).
		for _, st := range SentenciasEsquema(s) {
			if st == "" {
				continue
			}
			if _, err := db.Exec(st); err != nil {
				if EsErrorYaExiste(err) {
					continue
				}
				return fmt.Errorf("%w <- %.120s", err, st)
			}
		}
	}
	return nil
}

// ifNotExists suelto, con cualquier espaciado y en cualquier caso.
var reIfNotExists = regexp.MustCompile(`(?i)\bIF\s+NOT\s+EXISTS\b\s*`)

// SinSintaxisDeMariaDB devuelve la sentencia tal como la entiende MySQL.
//
// POR QUÉ HACE FALTA, Y POR QUÉ NO ES SOLO ESTÉTICA.
//
// `CREATE INDEX IF NOT EXISTS`, `CREATE UNIQUE INDEX IF NOT EXISTS` y
// `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` son sintaxis de MariaDB. MySQL no
// las admite: responde 1064 (error de sintaxis) y NO ejecuta nada. El esquema de
// este ERP —y una docena de archivos de migrations/— las usan, así que en MySQL
// esas columnas y esos índices simplemente no se creaban: el archivo se quedaba
// "NO ANOTADA" y se reintentaba (y volvía a fallar) en cada arranque. El
// resultado se veía lejos del sitio: consultas que piden una columna que nunca
// se creó.
//
// Quitarlas es SEGURO y no cambia el resultado, por una razón concreta: quien
// ejecuta estas sentencias ya tolera el error de "eso ya estaba" (ver
// EsErrorYaExiste y esYaEstabaMySQL, que cubren 1050/1060/1061). Con `IF NOT
// EXISTS` el autor pedía "créalo si falta"; sin él, el resultado es el mismo:
// si falta se crea, y si ya está el error se descarta.
//
// NO se toca `CREATE TABLE IF NOT EXISTS` ni `CREATE DATABASE IF NOT EXISTS`:
// esas sí las admite MySQL, y quitarlas convertiría una operación idempotente en
// un error en el segundo arranque.
func SinSintaxisDeMariaDB(s string) string {
	trim := strings.TrimSpace(s)
	u := strings.ToUpper(trim)
	switch {
	case strings.HasPrefix(u, "CREATE INDEX"),
		strings.HasPrefix(u, "CREATE UNIQUE INDEX"),
		strings.HasPrefix(u, "ALTER TABLE"):
		return reIfNotExists.ReplaceAllString(s, "")
	}
	return s
}

// SentenciasEsquema convierte una sentencia de esquema en N sentencias de un
// solo CLAÚSULA, ya sin sintaxis de MariaDB.
//
// POR QUÉ (hallazgo D-2, auditoría 2026-11-22). MySQL ejecuta un
// `ALTER TABLE t ADD COLUMN a ..., ADD COLUMN b ...` TODO o NADA: si la
// columna `a` ya existía (error 1060), `b` NO se crea —y el runner, que
// clasifica el 1060 como benigno, anotaba el archivo como aplicado con media
// estructura fuera. Es exactamente el fallo que dejó columnas fiscales
// inexistentes en bases reales (schema_z_zzz_recuperadas_cols.sql). Dividido
// por cláusula, cada una se juzga sola: la que ya estaba se tolera, la que
// falta se crea, y el ledger cuenta la verdad.
//
// Las comas que NO separan cláusulas son las que viven dentro de paréntesis
// (`DECIMAL(10,2)`, listas de índices) o dentro de literales de texto: el
// corta por cláusulas ignora las comas con profundidad>0 o dentro de
// ' " y ` `.
func SentenciasEsquema(s string) []string {
	limpia := strings.TrimSpace(SinSintaxisDeMariaDB(s))
	u := strings.ToUpper(limpia)
	if !strings.HasPrefix(u, "ALTER TABLE") {
		return []string{limpia}
	}
	tabla, resto := separaTablaDeCláusulas(limpia)
	if tabla == "" || resto == "" {
		return []string{limpia}
	}
	var clauses []string
	profundidad := 0
	var comilla byte
	inicio := 0
	for i := 0; i < len(resto); i++ {
		c := resto[i]
		if comilla != 0 {
			if c == comilla {
				comilla = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			comilla = c
		case '(', '[':
			profundidad++
		case ')', ']':
			if profundidad > 0 {
				profundidad--
			}
		case ',':
			if profundidad == 0 {
				clauses = append(clauses, strings.TrimSpace(resto[inicio:i]))
				inicio = i + 1
			}
		}
	}
	clauses = append(clauses, strings.TrimSpace(resto[inicio:]))
	if len(clauses) <= 1 {
		return []string{limpia}
	}
	var salida []string
	for _, cl := range clauses {
		if cl == "" {
			continue
		}
		salida = append(salida, fmt.Sprintf("ALTER TABLE %s %s", tabla, cl))
	}
	return salida
}

// separaTablaDeCláusulas devuelve la tabla (con sus comillas si las trae) y
// el resto de la sentencia tras ella, para reconstruir un ALTER por cláusula.
func separaTablaDeCláusulas(s string) (string, string) {
	resto := strings.TrimSpace(s[len("ALTER TABLE"):])
	if resto == "" {
		return "", ""
	}
	if resto[0] == '`' {
		fin := strings.Index(resto[1:], "`")
		if fin < 0 {
			return "", ""
		}
		tabla := resto[:fin+2]
		return tabla, strings.TrimSpace(resto[fin+2:])
	}
	campos := strings.SplitN(resto, " ", 2)
	tabla := campos[0]
	resto = ""
	if len(campos) > 1 {
		resto = strings.TrimSpace(campos[1])
	}
	return tabla, resto
}

// EsErrorYaExiste dice si el error es "eso ya estaba" y no un fallo real.
//
// La lista se decide por el mensaje Y por el código de error de MySQL (1061,
// 1050, 1060, 1062, 1826), porque los códigos no cambian entre versiones y
// los mensajes sí se traducen.
//
// Estos errores NO se cuentan como "aplicado": el registro de migraciones
// distingue entre "lo apliqué yo" y "ya estaba", y esa diferencia es la que
// permite que un archivo modificado se vuelva a aplicar.
func EsErrorYaExiste(err error) bool {
	if err == nil {
		return false
	}
	// El driver de MySQL expone el número de error; usarlo es más fiable que
	// buscar el texto, que cambia con el idioma del servidor.
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case 1050, // tabla ya existe
			1060, // columna duplicada
			1061, // nombre de índice duplicado
			1062, // clave duplicada
			1826: // espacio de nombres duplicado
			return true
		}
	}
	m := strings.ToLower(err.Error())
	for _, trozo := range []string{
		"already exists",
		"duplicate key name",
		"duplicate column name",
		"duplicate entry",
		"multiple primary key",
		"foreign key constraint",
		"check constraint",
	} {
		if strings.Contains(m, trozo) {
			return true
		}
	}
	return false
}
