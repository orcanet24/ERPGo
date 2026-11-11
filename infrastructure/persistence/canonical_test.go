package persistence

import (
	"fmt"
	"os"
	"testing"
)

// TestParsearCanonica verifica que el DDL canónico embebido se parte bien:
// 219 tablas, tenants con sus 12 columnas y algún índice conocido.
//
// POR QUÉ ESTA PRUEBA. El reconciliador entero depende de este parser; si
// deja de reconocer una columna o un índice, el reconciliador la ignora y la
// base queda incompleta en silencio.
func TestParsearCanonica(t *testing.T) {
	contenido, err := leerCanonica()
	if err != nil {
		t.Fatalf("no se pudo leer el canónico: %v", err)
	}
	tablas, err := ParsearCanonica(contenido)
	if err != nil {
		t.Fatalf("ParsearCanonica: %v", err)
	}
	if len(tablas) != 219 {
		t.Errorf("tablas parseadas: %d, esperadas 219", len(tablas))
	}

	porNombre := map[string]TablaCanonica{}
	for _, tb := range tablas {
		porNombre[tb.Nombre] = tb
	}

	tenants, ok := porNombre["tenants"]
	if !ok {
		t.Fatal("la tabla tenants no aparece en el canónico")
	}
	if len(tenants.Columnas) != 12 {
		t.Errorf("tenants: %d columnas, esperadas 12", len(tenants.Columnas))
	}
	columnas := map[string]bool{}
	for _, c := range tenants.Columnas {
		columnas[c.Nombre] = true
	}
	for _, esperada := range []string{"code", "legal_name", "is_demo", "db_name"} {
		if !columnas[esperada] {
			t.Errorf("tenants.%s no está entre las columnas parseadas", esperada)
		}
	}

	// Los índices inline del volcado deben parsearse: tenants trae dos.
	if len(tenants.Indices) == 0 {
		t.Error("tenants no trae índices parseados")
	}
}

// TestReconciliarBaseVacia prueba el camino de instalación nueva: una base
// vacía queda completo con una sola reconciliación, y la segunda no toca
// nada (la propiedad que evita que el arranque repita trabajo).
func TestReconciliarBaseVacia(t *testing.T) {
	db, limpiar, err := NuevaBaseVaciaDePrueba("reconvacia")
	if err != nil {
		t.Skipf("no hay MySQL para esta prueba: %v", err)
	}
	defer limpiar()

	res, err := ReconciliarConCanonico(db, "mysql")
	if err != nil {
		t.Fatalf("reconciliación inicial: %v", err)
	}
	if len(res.Avisos) > 0 {
		t.Errorf("la reconciliación inicial dejó avisos: %v", res.Avisos)
	}
	if len(res.TablasCreadas) != 219 {
		t.Errorf("tablas creadas: %d, esperadas 219", len(res.TablasCreadas))
	}

	res2, err := ReconciliarConCanonico(db, "mysql")
	if err != nil {
		t.Fatalf("segunda reconciliación: %v", err)
	}
	if len(res2.TablasCreadas)+len(res2.ColumnasAgregadas)+len(res2.IndicesCreados) != 0 {
		t.Errorf("la segunda reconciliación tocó algo: %+v", res2)
	}
}

// TestReconciliarAgregaColumna prueba el diff fino: una columna que falta en
// una tabla existente se agrega, y nada más.
func TestReconciliarAgregaColumna(t *testing.T) {
	db, limpiar, err := NuevaBaseVaciaDePrueba("reconvcol")
	if err != nil {
		t.Skipf("no hay MySQL para esta prueba: %v", err)
	}
	defer limpiar()

	if _, err := ReconciliarConCanonico(db, "mysql"); err != nil {
		t.Fatalf("reconciliación inicial: %v", err)
	}
	if _, err := db.Exec("ALTER TABLE tenants DROP COLUMN is_demo"); err != nil {
		t.Fatalf("no se pudo quitar is_demo: %v", err)
	}

	res, err := ReconciliarConCanonico(db, "mysql")
	if err != nil {
		t.Fatalf("reconciliación: %v", err)
	}
	if len(res.ColumnasAgregadas) != 1 || res.ColumnasAgregadas[0] != "tenants.is_demo" {
		t.Errorf("columnas agregadas: %v, esperada [tenants.is_demo]", res.ColumnasAgregadas)
	}
}

// TestReconciliarSQLite prueba el dialecto SQLite con un canónico mínimo: el
// reconciliador no depende de MySQL para la lógica de diff, solo para las
// consultas de introspección.
func TestReconciliarSQLite(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir SQLite: %v", err)
	}
	defer db.Close()

	minima := []TablaCanonica{
		{
			Nombre: "prueba",
			Crear:  "CREATE TABLE `prueba` (`id` INTEGER PRIMARY KEY, `nombre` VARCHAR(64) NOT NULL)",
			Columnas: []ColumnaCanonica{
				{Nombre: "id", Definicion: "INTEGER PRIMARY KEY"},
				{Nombre: "nombre", Definicion: "VARCHAR(64) NOT NULL"},
				{Nombre: "extra", Definicion: "VARCHAR(32) NOT NULL DEFAULT ''"},
			},
			Indices: []IndiceCanonico{{Nombre: "idx_prueba_nombre", Columnas: "`nombre`"}},
		},
	}

	res, err := Reconciliar(db, minima, "sqlite")
	if err != nil {
		t.Fatalf("reconciliación sqlite: %v", err)
	}
	if len(res.TablasCreadas) != 1 {
		t.Errorf("tablas creadas: %v", res.TablasCreadas)
	}

	// Simula la deriva: la tabla nació sin la columna extra y sin el índice.
	if _, err := db.Exec("DROP TABLE prueba"); err != nil {
		t.Fatalf("no se pudo probar la deriva: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE `prueba` (`id` INTEGER PRIMARY KEY, `nombre` VARCHAR(64) NOT NULL)"); err != nil {
		t.Fatalf("tabla derivada: %v", err)
	}

	res2, err := Reconciliar(db, minima, "sqlite")
	if err != nil {
		t.Fatalf("reconciliación con deriva: %v", err)
	}
	if len(res2.ColumnasAgregadas) != 1 || res2.ColumnasAgregadas[0] != "prueba.extra" {
		t.Errorf("columnas agregadas: %v, esperada [prueba.extra]", res2.ColumnasAgregadas)
	}
	if len(res2.IndicesCreados) != 1 || res2.IndicesCreados[0] != "prueba.idx_prueba_nombre" {
		t.Errorf("índices creados: %v (avisos: %v)", res2.IndicesCreados, res2.Avisos)
	}
}

// TestSellarCanonico verifica que el sello queda registrado una sola vez y es
// idempotente: en campo es la forma de saber qué bases pasaron por el
// reconciliador.
func TestSellarCanonico(t *testing.T) {
	db, limpiar, err := NuevaBaseVaciaDePrueba("reconsello")
	if err != nil {
		t.Skipf("no hay MySQL para esta prueba: %v", err)
	}
	defer limpiar()

	if _, err := ReconciliarConCanonico(db, "mysql"); err != nil {
		t.Fatalf("reconciliación inicial: %v", err)
	}
	if err := SellarCanonico(db); err != nil {
		t.Fatalf("SellarCanonico: %v", err)
	}
	suma, err := SelloCanonico(db)
	if err != nil {
		t.Fatalf("SelloCanonico: %v", err)
	}
	if suma == "" {
		t.Fatal("el sello no quedó registrado")
	}

	if err := SellarCanonico(db); err != nil {
		t.Fatalf("segundo sello: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE filename = 'esquema_canonico_v1'").Scan(&n); err != nil {
		t.Fatalf("contando sellos: %v", err)
	}
	if n != 1 {
		t.Errorf("el sello se duplicó: %d filas", n)
	}
}

// TestArranqueSelladoIdempotente prueba el flujo nuevo: una base sin sello
// recorre la cadena y queda sellada; un segundo arranque sobre la base sellada
// no toca el esquema y mantiene el sello vigente.
func TestArranqueSelladoIdempotente(t *testing.T) {
	db, limpiar, err := NuevaBaseVaciaDePrueba("reconarr")
	if err != nil {
		t.Skipf("no hay MySQL para esta prueba: %v", err)
	}
	defer limpiar()

	var nombre string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&nombre); err != nil {
		t.Fatalf("no se pudo saber la base de prueba: %v", err)
	}
	cfg := ConfigMySQL{Host: os.Getenv("TEST_DB_HOST"), Port: 3306, Usuario: os.Getenv("TEST_DB_USER"), Clave: os.Getenv("TEST_DB_PASSWORD"), Nombre: nombre}
	if puerto := os.Getenv("TEST_DB_PORT"); puerto != "" {
		fmt.Sscanf(puerto, "%d", &cfg.Port)
	}

	if err := RunMigrationsForMySQL(db, cfg); err != nil {
		t.Fatalf("primer arranque: %v", err)
	}
	suma, err := SelloCanonico(db)
	if err != nil || suma == "" {
		t.Fatalf("el primer arranque no selló: %q %v", suma, err)
	}
	vigente, err := ChecksumCanonico()
	if err != nil {
		t.Fatalf("checksum del canónico: %v", err)
	}
	if suma != vigente {
		t.Errorf("sello %s != canónico vigente %s", suma, vigente)
	}

	if err := RunMigrationsForMySQL(db, cfg); err != nil {
		t.Fatalf("segundo arranque: %v", err)
	}
	suma2, _ := SelloCanonico(db)
	if suma2 != vigente {
		t.Errorf("el segundo arranque cambió el sello: %s != %s", suma2, vigente)
	}
}

// TestEsquemaEmbebido verifica que el embed de schema/ existe y trae el DDL:
// si el archivo sale del repositorio, el reconciliador se queda mudo y esta
// prueba es la que lo delata.
func TestEsquemaEmbebido(t *testing.T) {
	if _, err := leerCanonica(); err != nil {
		t.Fatalf("el DDL canónico no está embebido: %v", err)
	}
}
