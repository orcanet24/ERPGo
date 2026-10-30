package mensajeria

// Certificación de la parte de la migración que puede fallar en silencio: las
// siembras condicionales.
//
// El DDL es MySQL y no se ejecuta aquí; lo que sí se ejecuta —dos veces, sobre
// SQLite en memoria— son los INSERT de `module`, `sub_module`, `modules`,
// `menu` y `msg_provider`. Esos son los que, mal escritos, dejan el módulo sin
// permisos, duplican el menú o lo atan al módulo apagado. La prueba corre las
// MISMAS sentencias del archivo, no una copia.

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

const archivoMigracion = "../../migrations/schema_z_zzzzzzzzzzz_mensajeria.sql"

func baseMigracion(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := []string{
		`CREATE TABLE module (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, description TEXT,
			directory TEXT, status INTEGER DEFAULT 1)`,
		`CREATE TABLE sub_module (id INTEGER PRIMARY KEY AUTOINCREMENT, mid INTEGER, name TEXT,
			directory TEXT, status INTEGER)`,
		`CREATE TABLE modules (id TEXT PRIMARY KEY, name TEXT, version TEXT, description TEXT,
			is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 0, path TEXT, author TEXT, license_type TEXT)`,
		`CREATE TABLE menu (id INTEGER PRIMARY KEY, parent_id INTEGER, label TEXT, url TEXT, icon TEXT,
			perm TEXT, active_prefix TEXT, sort_order INTEGER, status INTEGER, module_id TEXT DEFAULT 'core',
			is_core INTEGER DEFAULT 0)`,
		`CREATE TABLE msg_provider (id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT UNIQUE,
			enabled INTEGER DEFAULT 0, config_json TEXT, updated_by TEXT, updated_at TEXT)`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("crear tabla de prueba: %v\n%s", err, q)
		}
	}
	return db
}

// sentenciasDeSiembra saca del archivo real los INSERT de la migración.
//
// Se quitan ANTES las líneas de comentario, igual que hace el aplicador real
// (`stripSQLComments`): un `;` dentro de un comentario partiría la sentencia en
// un sitio que no toca. La primera versión de esta prueba no lo hacía y
// «desaparecía» el INSERT del módulo.
func sentenciasDeSiembra(t *testing.T) []string {
	t.Helper()
	bruto, err := os.ReadFile(filepath.Clean(archivoMigracion))
	if err != nil {
		t.Fatalf("leer la migración: %v", err)
	}
	var lineas []string
	for _, ln := range strings.Split(strings.ReplaceAll(string(bruto), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "--") {
			continue
		}
		lineas = append(lineas, ln)
	}
	var out []string
	for _, trozo := range strings.Split(strings.Join(lineas, "\n"), ";") {
		s := strings.TrimSpace(trozo)
		if s == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(s), "INSERT INTO") {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatal("la migración no tiene sentencias de siembra: el archivo cambió de forma")
	}
	return out
}

func TestMigracionDeMensajeriaEsIdempotenteYDejaElModuloApagado(t *testing.T) {
	db := baseMigracion(t)
	sentencias := sentenciasDeSiembra(t)

	// Dos pasadas: una instalación nueva y una base que ya tenía la fila.
	for pasada := 1; pasada <= 2; pasada++ {
		for _, s := range sentencias {
			if _, err := db.Exec(s); err != nil {
				t.Fatalf("pasada %d: %v\n%s", pasada, err, s)
			}
		}
	}

	cuenta := func(consulta string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(consulta).Scan(&n); err != nil {
			t.Fatalf("%v\n%s", err, consulta)
		}
		return n
	}

	if n := cuenta(`SELECT COUNT(*) FROM module WHERE directory = 'mensajeria'`); n != 1 {
		t.Fatalf("módulos sembrados = %d; debe ser 1", n)
	}
	if n := cuenta(`SELECT COUNT(*) FROM sub_module WHERE directory LIKE 'mensajeria%'`); n != 3 {
		t.Fatalf("permisos sembrados = %d; deben ser 3", n)
	}
	if n := cuenta(`SELECT COUNT(*) FROM modules WHERE id = 'mensajeria'`); n != 1 {
		t.Fatalf("fila del módulo = %d; debe ser 1", n)
	}
	if n := cuenta(`SELECT COUNT(*) FROM modules WHERE id = 'mensajeria' AND is_active = 0`); n != 1 {
		t.Fatal("el módulo no nace apagado: se encendería solo al migrar")
	}
	if n := cuenta(`SELECT COUNT(*) FROM menu WHERE id IN (282,283,284)`); n != 3 {
		t.Fatalf("entradas de menú = %d; deben ser 3", n)
	}
	if n := cuenta(`SELECT COUNT(*) FROM msg_provider`); n != 4 {
		t.Fatalf("proveedores = %d; deben ser 4", n)
	}

	// La entrada de configuración pertenece al módulo: cuando está apagado no
	// se anuncia en el menú. El master llega a ella desde Configuración →
	// Módulos → Configurar, que sí es core.
	var moduloDeConfig string
	if err := db.QueryRow(`SELECT module_id FROM menu WHERE url = '/settings/mensajeria'`).Scan(&moduloDeConfig); err != nil {
		t.Fatal(err)
	}
	if moduloDeConfig != "mensajeria" {
		t.Fatalf("la entrada de configuración quedó en %q y debería pertenecer al módulo", moduloDeConfig)
	}

	// Los permisos tienen que colgar del módulo correcto, o la matriz de Roles
	// no podrá concederlos.
	var mid, midModulo int
	if err := db.QueryRow(`SELECT mid FROM sub_module WHERE directory = 'mensajeria_chat'`).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM module WHERE directory = 'mensajeria'`).Scan(&midModulo); err != nil {
		t.Fatal(err)
	}
	if mid != midModulo {
		t.Fatalf("los permisos cuelgan del módulo %d y la fila es %d", mid, midModulo)
	}
}
