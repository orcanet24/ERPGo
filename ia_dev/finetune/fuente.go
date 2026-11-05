package main

// fuente.go — Todo lo que este generador toma del repo y de la base: el
// contrato del informe, el catálogo de módulos, la apertura de la base empresa
// y la introspección del esquema real.
//
// La norma que gobierna este archivo: NINGUNA copia. El contrato vive en
// ai_reports.go (aiContratoInforme) y el catálogo en ai_schema_modules.go: si
// se editan mañana, este generador cambia con ellos en la próxima corrida.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "github.com/glebarez/go-sqlite" // driver «sqlite»: glebarez, el mismo de persistence.OpenSQLite
	_ "github.com/go-sql-driver/mysql"
)

// ---------------------------------------------------------------------------
// Contrato del informe (fuente: interfaces/web/ai_reports.go)
// ---------------------------------------------------------------------------

// extraerContrato lee el literal de aiContratoInforme de ai_reports.go. Es un
// string crudo delimitado por backticks: se toma tal como está en el archivo
// fuente, así el dataset imita el system prompt que el ERP envía en
// producción (no una interpretación de él).
func extraerContrato(repoRoot string) (string, error) {
	ruta := filepath.Join(repoRoot, "interfaces", "web", "ai_reports.go")
	txt, err := os.ReadFile(ruta)
	if err != nil {
		return "", fmt.Errorf("leer el contrato de %s: %w", ruta, err)
	}
	const marca = "const aiContratoInforme = `"
	i := strings.Index(string(txt), marca)
	if i < 0 {
		return "", fmt.Errorf("%s ya no contiene «const aiContratoInforme = `»: actualizar este extractor", ruta)
	}
	inicio := i + len(marca)
	fin := strings.Index(string(txt)[inicio:], "`")
	if fin < 0 {
		return "", fmt.Errorf("el literal de aiContratoInforme no cierra con backtick")
	}
	contrato := string(txt)[inicio : inicio+fin]
	if !strings.Contains(contrato, "CONTRATO DE RESPUESTA") {
		return "", fmt.Errorf("el aiContratoInforme extraído no trae «CONTRATO DE RESPUESTA»: el extractor desencajó")
	}
	return contrato, nil
}

// ---------------------------------------------------------------------------
// Catálogo de módulos (fuente: interfaces/web/ai_schema_modules.go)
// ---------------------------------------------------------------------------

// modulo es un aiSchemaModule del catálogo, traspasado a este paquete (el del
// servidor no exporta sus tipos).
type modulo struct {
	Key         string
	Label       string
	Description string
	Tables      []string
}

// reModulo casa CADA bloque del var aiSchemaModules en su archivo: Key, Label,
// Description y el literal Tables en la misma llave. (?s) porque hay texto
// entre ambos; la captura 3 tolera las comillas escapadas de los comentarios.
var reModulo = regexp.MustCompile(`(?s)Key:\s*"([^"]*)",\s*Label:\s*"([^"]*)",\s*Description:\s*"((?:\\.|[^"\\])*)".*?Tables:\s*\[\]string\{([^}]*)\}`)

// reTabla suelta cada "tabla" del literal []string.
var reTabla = regexp.MustCompile(`"([^"]+)"`)

// leerCatalogoModulos escanea el archivo fuente y devuelve el catálogo en el
// orden del archivo (el orden importa: es el de clasificación del MCP).
func leerCatalogoModulos(repoRoot string) ([]modulo, error) {
	ruta := filepath.Join(repoRoot, "interfaces", "web", "ai_schema_modules.go")
	txt, err := os.ReadFile(ruta)
	if err != nil {
		return nil, fmt.Errorf("leer el catálogo de módulos de %s: %w", ruta, err)
	}
	hallados := reModulo.FindAllStringSubmatch(string(txt), -1)
	if len(hallados) == 0 {
		return nil, fmt.Errorf("%s no produjo ningún módulo: ¿cambió el formato del var aiSchemaModules?", ruta)
	}
	out := make([]modulo, 0, len(hallados))
	for _, m := range hallados {
		var tablas []string
		// FindAllStringSubmatch para quedarnos con el NOMBRE puro del match:
		// FindAllString traería también las comillas («"invoice"»), y esa
		// comilla fantasma impide casar el nombre contra sqlite_master.
		for _, t := range reTabla.FindAllStringSubmatch(m[4], -1) {
			tablas = append(tablas, t[1])
		}
		out = append(out, modulo{
			Key:         m[1],
			Label:       m[2],
			Description: strings.TrimSpace(desescapar(m[3])),
			Tables:      tablas,
		})
	}
	return out, nil
}

// desescapar resuelve los escapes mínimos de literales Go —comillas y barras
// invertidas escapadas— que un comentario del catálogo puede traer; los
// limpiamos para que el texto llegue limpio al dataset.
func desescapar(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// ---------------------------------------------------------------------------
// Apertura e introspección de la base empresa
// ---------------------------------------------------------------------------

// abrirDB abre la base empresa: SQLite por archivo (lo normal en instalación
// local) o MySQL por DSN. Devuelve el motor abierto ("sqlite" o "mysql"), la
// función de cierre para el defer del caller y el error.
func abrirDB(cfg configCorrida) (*sql.DB, string, func(), error) {
	if cfg.dsnMySQL != "" {
		db, err := sql.Open("mysql", cfg.dsnMySQL)
		if err != nil {
			return nil, "", nil, fmt.Errorf("abrir MySQL: %w", err)
		}
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, "", nil, fmt.Errorf("MySQL no responde («%s»): %w", cfg.dsnMySQL, err)
		}
		return db, "mysql", func() { db.Close() }, nil
	}
	if _, err := os.Stat(cfg.rutaBD); err != nil {
		return nil, "", nil, fmt.Errorf("no existe la base SQLite de la empresa en %s: %w", cfg.rutaBD, err)
	}
	db, err := sql.Open("sqlite", cfg.rutaBD)
	if err != nil {
		return nil, "", nil, fmt.Errorf("abrir SQLite %s: %w", cfg.rutaBD, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, "", nil, fmt.Errorf("SQLite %s no responde: %w", cfg.rutaBD, err)
	}
	return db, "sqlite", func() { db.Close() }, nil
}

// listadoTablas son los nombres de tabla que EXISTEN, con el caso con que la
// base los escribió (SQLite los devuelve tal cual).
type listadoTablas map[string]bool

// listarTablasBD devuelve el set de tablas existentes en ESTA empresa, para
// introspección solo de lo que de verdad existe (el catálogo de módulos es
// universal; una empresa joven puede no tener todas las tablas aún).
func listarTablasBD(db *sql.DB, motor string) (listadoTablas, error) {
	hay := listadoTablas{}
	var rows *sql.Rows
	var err error
	if motor == "mysql" {
		rows, err = db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()`)
	} else {
		rows, err = db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	}
	if err != nil {
		return nil, fmt.Errorf("listar tablas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		hay[strings.ToLower(t)] = true
	}
	return hay, rows.Err()
}

// campoTabla es una columna de una tabla, con el comentario documentado que
// ERPGo escribe en MySQL (en SQLite no hay comentario: se marca la limitación
// en la propia fila en vez de inventar documentación).
type campoTabla struct {
	Nombre     string `json:"nombre"`
	Tipo       string `json:"tipo"`
	Comentario string `json:"comentario"`
}

// introspeccionTabla devuelve los campos de UNA tabla del módulo. En MySQL
// salen desde information_schema con sus comentarios (la documentación de
// negocio que el MCP entrega al agente); en SQLite solo nombre y tipo.
func introspeccionTabla(db *sql.DB, tabla, motor string) ([]campoTabla, error) {
	if motor == "mysql" {
		rows, err := db.Query(`SELECT COLUMN_NAME, COLUMN_TYPE, COALESCE(COLUMN_COMMENT, '')
			FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ?
			ORDER BY ordinal_position`, tabla)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []campoTabla{}
		for rows.Next() {
			var c campoTabla
			if err := rows.Scan(&c.Nombre, &c.Tipo, &c.Comentario); err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, rows.Err()
	}
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, quoteIdent(tabla)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []campoTabla{}
	for rows.Next() {
		var cid int
		var nombre, tipo string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &nombre, &tipo, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		out = append(out, campoTabla{Nombre: nombre, Tipo: tipo, Comentario: ""})
	}
	return out, rows.Err()
}

// quoteIdent blinda un identificador interpolado en PRAGMA. Las tablas del
// catálogo llegan siempre en minúsculas y con guion bajo (convención del
// whitelist del proyecto), pero el candado es de piedra: si un día cambia, el
// identificador se comilla en vez de propagarse crudo en el PRAGMA.
func quoteIdent(t string) string {
	if t == "" {
		return ""
	}
	for _, c := range t {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		}
	}
	return t
}
