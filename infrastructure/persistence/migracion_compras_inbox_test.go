package persistence

// Prueba de la migración de la bandeja de documentos de compra.
//
// POR QUÉ EXISTE: hasta ahora NADA ejecutaba los archivos de migrations/ en las
// pruebas. Una sentencia mal escrita se descubría en el primer arranque real,
// donde solo produce un aviso en el log y una tabla que no existe — y el fallo
// sale después, en una pantalla, como «tabla purchase_inbox no existe».
//
// Qué se comprueba aquí: que el archivo se DESCUBRE (está en el árbol
// embebido), que se TRADUCE para SQLite con el mismo convertidor del arranque y
// que la ejecución deja la tabla con su candado único POR EMPRESA. Ese candado
// es la mitad de la garantía anti-duplicado de la recepción de facturas: sin
// él, el mismo archivo se recibiría dos veces.
//
// Se aplica SOLO este archivo y no el conjunto entero a propósito: el objeto de
// esta prueba es esta migración, y que otra migración esté rota no tiene por qué
// aparecer aquí (eso sería un fallo ajeno escondido dentro de una prueba propia).

import (
	"strings"
	"testing"
)

// sentenciaInbox es la misma forma de INSERT que usa purchaseInboxSubir: con
// lista de columnas explícita, que es la forma general en MySQL, PostgreSQL,
// MariaDB y SQLite.
const sentenciaInbox = `INSERT INTO purchase_inbox
	(tenant_id, filename, orig_name, formato, extraccion, doc_kind, sha256)
	VALUES (?,?,?,?,?,?,?)`

func TestLaMigracionDeLaBandejaSeAplicaEnSQLite(t *testing.T) {
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir SQLite en memoria: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migs, err := readMigrations()
	if err != nil {
		t.Fatalf("no se pudieron leer las migraciones embebidas: %v", err)
	}
	var mia []migFile
	for _, m := range migs {
		if m.name == "schema_z_compras_inbox.sql" {
			mia = append(mia, m)
		}
	}
	if len(mia) != 1 {
		t.Fatalf("la migración schema_z_compras_inbox.sql no está en el árbol embebido (encontradas: %d)", len(mia))
	}

	_, _, sentencias, avisos := applyMigrations(db, mia, false)
	if avisos > 0 {
		t.Fatalf("la migración de la bandeja no se aplicó completa: %d avisos", avisos)
	}
	if sentencias == 0 {
		t.Fatal("la migración no ejecutó ninguna sentencia")
	}

	// La tabla existe y acepta la primera carga.
	args := []interface{}{"EMP1", "EMP1/doc-a.xml", "factura.xml", "xml", "xml", "factura", strings.Repeat("aa", 32)}
	if _, err := db.Exec(sentenciaInbox, args...); err != nil {
		t.Fatalf("la primera carga no entró: %v", err)
	}

	// El candado: el mismo archivo, misma empresa, no vuelve a entrar.
	args[1] = "EMP1/doc-b.xml" // otro nombre, mismo contenido: da igual
	if _, err := db.Exec(sentenciaInbox, args...); err == nil {
		t.Fatal("el candado no rechazó la segunda copia del mismo archivo")
	} else if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Errorf("el rechazo no se reconoció como clave única: %v", err)
	}

	// Otra empresa con el mismo archivo sí entra: el candado es POR EMPRESA.
	args[0] = "EMP2"
	args[1] = "EMP2/doc-a.xml"
	if _, err := db.Exec(sentenciaInbox, args...); err != nil {
		t.Errorf("otra empresa debía poder recibir el mismo archivo: %v", err)
	}
}
