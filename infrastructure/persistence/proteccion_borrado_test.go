package persistence

// Pruebas de la protección de borrado en la base.
//
// Tres niveles, del más barato al más caro:
//
//  1. Las SENTENCIAS son puras: cada regla está completa, los nombres no se
//     repiten y el cuerpo es el del dialecto del motor.
//  2. La TRADUCCIÓN de errores: que al navegador no le llegue el texto del
//     motor.
//  3. El BLOQUEO REAL: un esquema con las tablas que citan las reglas, la
//     protección creada encima y borrados de verdad. Si una regla no bloqueara
//     —o bloqueara de más— se ve aquí.
//
// Lo que esta prueba NO comprueba, y comprueba el test del paquete web sobre las
// mismas reglas leídas de ReglasDeBorrado(): que las tablas y las columnas que
// citan existan en el esquema de las migraciones. Se hace allí porque ese paquete
// ya sabe leerlas, y porque el motor valida las columnas al crear el trigger: una
// errata deja la protección sin crear y el arranque solo deja un aviso en el log.

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestLasReglasDeBorradoEstanCompletas(t *testing.T) {
	if len(reglasDeBorrado) == 0 {
		t.Fatal("no hay ninguna regla: la protección no protege nada")
	}
	vistos := map[string]bool{}
	for _, r := range reglasDeBorrado {
		if strings.TrimSpace(r.Tabla) == "" {
			t.Errorf("la regla %q no dice qué tabla protege", r.Nombre)
		}
		if strings.TrimSpace(r.Condicion) == "" {
			t.Errorf("la regla %q no tiene condición: bloquearía todo", r.Nombre)
		}
		if strings.TrimSpace(r.Mensaje) == "" {
			t.Errorf("la regla %q no tiene mensaje: el usuario vería el error del motor", r.Nombre)
		}
		// El mensaje va dentro de una sentencia con comillas simples: una comilla
		// dentro (un apóstrofo) rompería el CREATE TRIGGER.
		if strings.Contains(r.Mensaje, "'") {
			t.Errorf("el mensaje de %q lleva una comilla simple y rompe la sentencia", r.Nombre)
		}
		// Convención compartida con static/js/integridad.js: el aviso central
		// reconoce esta familia de mensajes por su prefijo.
		if !strings.HasPrefix(r.Mensaje, "No se puede ") {
			t.Errorf("el mensaje de %q no empieza por «No se puede»: el aviso central no lo reconocería", r.Nombre)
		}
		if vistos[r.Nombre] {
			t.Errorf("dos reglas con el mismo nombre de trigger: %q", r.Nombre)
		}
		vistos[r.Nombre] = true
	}
}

func TestSentenciasProteccionPorMotor(t *testing.T) {
	casos := []struct {
		motor     string
		cuerpo    string
		prohibido string
	}{
		{"sqlite", "RAISE(ABORT,", "SIGNAL SQLSTATE"},
		{"", "RAISE(ABORT,", "SIGNAL SQLSTATE"}, // vacío = motor por defecto = sqlite
		{"mysql", "SIGNAL SQLSTATE '45000'", "RAISE(ABORT"},
		{"MariaDB", "SIGNAL SQLSTATE '45000'", "RAISE(ABORT"},
	}
	for _, c := range casos {
		sentencias, err := SentenciasProteccion(c.motor)
		if err != nil {
			t.Fatalf("motor %q: %v", c.motor, err)
		}
		if len(sentencias) != 2*len(reglasDeBorrado) {
			t.Fatalf("motor %q: %d sentencias para %d reglas (se esperan dos por regla: DROP y CREATE)",
				c.motor, len(sentencias), len(reglasDeBorrado))
		}
		for i := 0; i < len(sentencias); i += 2 {
			regla := reglasDeBorrado[i/2]
			nombre := prefijoTrigger + regla.Nombre
			drop, crear := sentencias[i], sentencias[i+1]

			// El DROP va ANTES del CREATE: así una regla corregida sustituye a la
			// que había, en vez de quedarse la vieja para siempre.
			if !strings.HasPrefix(drop, "DROP TRIGGER IF EXISTS "+nombre) {
				t.Errorf("motor %q, regla %q: la sentencia no es el DROP esperado: %q", c.motor, regla.Nombre, drop)
			}
			if !strings.HasPrefix(crear, "CREATE TRIGGER "+nombre+" ") {
				t.Errorf("motor %q, regla %q: la sentencia no es el CREATE esperado: %q", c.motor, regla.Nombre, crear)
			}
			if !strings.Contains(crear, "BEFORE DELETE ON "+regla.Tabla) {
				t.Errorf("motor %q, regla %q: el trigger no protege la tabla %q: %q", c.motor, regla.Nombre, regla.Tabla, crear)
			}
			if !strings.Contains(crear, regla.Mensaje) {
				t.Errorf("motor %q, regla %q: el mensaje no va en la sentencia: %q", c.motor, regla.Nombre, crear)
			}
			if !strings.Contains(crear, c.cuerpo) {
				t.Errorf("motor %q, regla %q: falta el cuerpo del dialecto (%q): %q", c.motor, regla.Nombre, c.cuerpo, crear)
			}
			if strings.Contains(crear, c.prohibido) {
				t.Errorf("motor %q, regla %q: el cuerpo es del otro motor (%q): %q", c.motor, regla.Nombre, c.prohibido, crear)
			}
			// La condición se escribe en varias líneas en el código y tiene que
			// llegar aplanada: un salto de línea dentro del WHEN/IF es una
			// sentencia rota para MySQL.
			if strings.Contains(crear, "\t\t\tOR") || strings.Contains(crear, "\n\t\t\t\t") {
				t.Errorf("la condición llega con saltos de línea: %q", crear)
			}
		}
	}

	if _, err := SentenciasProteccion("oracle"); err == nil {
		t.Error("un motor desconocido debe dar error en vez de crear sentencias a ciegas")
	}
	if _, err := SentenciasQuitarProteccion("oracle"); err == nil {
		t.Error("la retirada de la protección también debe rechazar un motor desconocido")
	}
}

func TestSentenciasQuitarProteccionSoloDropea(t *testing.T) {
	sentencias, err := SentenciasQuitarProteccion("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if len(sentencias) != len(reglasDeBorrado) {
		t.Fatalf("%d sentencias para %d reglas", len(sentencias), len(reglasDeBorrado))
	}
	for _, s := range sentencias {
		if !strings.HasPrefix(s, "DROP TRIGGER IF EXISTS "+prefijoTrigger) {
			t.Errorf("la retirada incluye algo que no es un DROP de la protección: %q", s)
		}
	}
}

func TestMensajeDeIntegridad(t *testing.T) {
	casos := []struct {
		nombre   string
		err      error
		contiene string
		vacio    bool
	}{
		{
			// Así envuelve el driver SQLite el aborto de un trigger nuestro.
			nombre:   "trigger de sqlite con nuestro mensaje",
			err:      errors.New("constraint failed: No se puede eliminar el cliente: tiene facturas registradas. (1811)"),
			contiene: "No se puede eliminar el cliente: tiene facturas registradas.",
		},
		{
			nombre:   "clave foránea de sqlite, mensaje genérico",
			err:      errors.New("constraint failed: FOREIGN KEY constraint failed (1811)"),
			contiene: "otros documentos dependen",
		},
		{
			nombre:   "SIGNAL de mysql (1644)",
			err:      &mysql.MySQLError{Number: 1644, Message: "No se puede eliminar la compra: tiene asientos contables."},
			contiene: "No se puede eliminar la compra: tiene asientos contables.",
		},
		{
			nombre:   "clave foránea de mysql (1451)",
			err:      &mysql.MySQLError{Number: 1451, Message: "Cannot delete or update a parent row: a foreign key constraint fails (`e`.`f`, CONSTRAINT `x`)"},
			contiene: "otros documentos dependen",
		},
		{
			nombre:   "falta el padre (1452)",
			err:      &mysql.MySQLError{Number: 1452, Message: "Cannot add or update a child row: a foreign key constraint fails"},
			contiene: "no existe",
		},
		{
			nombre: "un error que no es de integridad no se toca",
			err:    errors.New("sql: no rows in result set"),
			vacio:  true,
		},
		{
			nombre: "sin error no hay nada que traducir",
			err:    nil,
			vacio:  true,
		},
		{
			// Un UNIQUE es una restricción, pero no es un borrado impedido: si se
			// tradujera, un alta duplicada se explicaría como un borrado.
			nombre: "unique de sqlite no se disfraza de borrado impedido",
			err:    errors.New("constraint failed: UNIQUE constraint failed: customer_information.rif (2067)"),
			vacio:  true,
		},
	}
	for _, c := range casos {
		got := MensajeDeIntegridad(c.err)
		if c.vacio {
			if got != "" {
				t.Errorf("%s: se esperaba vacío y salió %q", c.nombre, got)
			}
			continue
		}
		if !strings.Contains(got, c.contiene) {
			t.Errorf("%s: el mensaje %q no dice %q", c.nombre, got, c.contiene)
		}
		if strings.Contains(got, "1811") || strings.Contains(got, "constraint failed") {
			t.Errorf("%s: el mensaje conserva el envoltorio del driver: %q", c.nombre, got)
		}
	}
}

// esquemaMinimo son las tablas que citan las reglas, con los MISMOS nombres de
// tabla y de columna que el esquema real.
//
// SE CREAN AQUÍ Y NO MIGRANDO EL ESQUEMA COMPLETO, a propósito: el camino SQLite
// de las migraciones está incompleto —hay diez archivos
// (schema_reconstruido_*.sql) que no llegan a aplicarse en SQLite y tablas que
// solo existen en el camino MySQL, que es el que usa la aplicación—, y probar la
// protección no depende de eso. Que estos nombres coincidan con los de las
// migraciones lo comprueba el test del paquete web sobre ReglasDeBorrado().
// Si a este esquema le faltara una tabla, la creación de la protección fallaría
// aquí mismo («no such table»), así que no puede quedarse corto en silencio.
var esquemaMinimo = []string{
	`CREATE TABLE IF NOT EXISTS customer_information (customer_id INTEGER PRIMARY KEY, customer_name TEXT, status INTEGER)`,
	`CREATE TABLE IF NOT EXISTS supplier_information (supplier_id INTEGER PRIMARY KEY, supplier_name TEXT, status INTEGER)`,
	`CREATE TABLE IF NOT EXISTS product_information (product_id INTEGER PRIMARY KEY, product_name TEXT)`,
	`CREATE TABLE IF NOT EXISTS invoice (id INTEGER PRIMARY KEY, invoice_id INTEGER, customer_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS invoice_details (id INTEGER PRIMARY KEY, invoice_id INTEGER, product_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS debt_collection (id INTEGER PRIMARY KEY, customer_id INTEGER, amount REAL)`,
	`CREATE TABLE IF NOT EXISTS quotation (id INTEGER PRIMARY KEY, quotation_id TEXT, customer_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS delivery_order (id INTEGER PRIMARY KEY, customer_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS product_purchase (id INTEGER PRIMARY KEY, purchase_id INTEGER, chalan_no TEXT, supplier_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS product_purchase_details (id INTEGER PRIMARY KEY, purchase_id INTEGER, product_id INTEGER, quantity REAL)`,
	`CREATE TABLE IF NOT EXISTS accounts_payable (id INTEGER PRIMARY KEY, supplier_id TEXT, purchase_id TEXT, amount REAL, paid_amount REAL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS vat_retention (id INTEGER PRIMARY KEY, purchase_id INTEGER, supplier_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS islr_retention (id INTEGER PRIMARY KEY, supplier_id INTEGER)`,
	`CREATE TABLE IF NOT EXISTS acc_vaucher (id INTEGER PRIMARY KEY, VNo TEXT, referenceNo TEXT)`,
	`CREATE TABLE IF NOT EXISTS acc_transaction (id INTEGER PRIMARY KEY, VNo TEXT, referenceNo TEXT)`,
	`CREATE TABLE IF NOT EXISTS product_service (service_id INTEGER PRIMARY KEY, service_name TEXT)`,
	`CREATE TABLE IF NOT EXISTS quotation_service_used (id INTEGER PRIMARY KEY, quot_id TEXT, service_id INTEGER)`,
}

// TestLaProteccionBloqueaDeVerdad crea la protección sobre el esquema mínimo y
// comprueba, borrado a borrado, qué se impide y qué no.
func TestLaProteccionBloqueaDeVerdad(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "proteccion.sqlite3"))
	if err != nil {
		t.Fatalf("no se pudo abrir la base de pruebas: %v", err)
	}
	defer db.Close()

	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(sql, args...); err != nil {
			t.Fatalf("ejecutando (%s): %v", sql, err)
		}
	}
	for _, tabla := range esquemaMinimo {
		ejecutar(tabla)
	}

	// Crear la protección exige que TODAS las tablas y columnas de las reglas
	// existan: si alguna regla citara algo que no está, esto falla.
	if err := AplicarProteccionDeBorrado(db, "sqlite"); err != nil {
		t.Fatalf("no se pudo crear la protección: %v", err)
	}
	// Y es idempotente: aplicarla otra vez deja lo mismo, no falla.
	if err := AplicarProteccionDeBorrado(db, "sqlite"); err != nil {
		t.Fatalf("la protección no es idempotente: %v", err)
	}

	// borrar intenta el borrado y devuelve (mensaje traducido, bloqueado).
	borrar := func(sql string, args ...any) (string, bool) {
		t.Helper()
		_, err := db.Exec(sql, args...)
		if err == nil {
			return "", false
		}
		return MensajeDeIntegridad(err), true
	}
	cuenta := func(tabla, donde string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+tabla+` WHERE `+donde, args...).Scan(&n); err != nil {
			t.Fatalf("contando en %s: %v", tabla, err)
		}
		return n
	}

	// ---- clientes: con factura no se borra, sin documentos sí ----
	ejecutar(`INSERT INTO customer_information (customer_id, customer_name, status) VALUES (1,'ACME',1)`)
	ejecutar(`INSERT INTO customer_information (customer_id, customer_name, status) VALUES (2,'SIN MOVIMIENTO',1)`)
	ejecutar(`INSERT INTO invoice (id, invoice_id, customer_id) VALUES (1, 1, 1)`)

	msg, bloqueado := borrar(`DELETE FROM customer_information WHERE customer_id = 1`)
	if !bloqueado {
		t.Error("el cliente con una factura se borró: la protección no está funcionando")
	} else if !strings.Contains(msg, "No se puede eliminar el cliente") {
		t.Errorf("el mensaje para el cliente fue %q", msg)
	}
	if n := cuenta("customer_information", "customer_id = 1"); n != 1 {
		t.Errorf("el cliente bloqueado desapareció igualmente (quedan %d)", n)
	}
	if _, bloqueado := borrar(`DELETE FROM customer_information WHERE customer_id = 2`); bloqueado {
		t.Error("un cliente sin ningún documento tiene que poder borrarse")
	}

	// ---- proveedores: con compra no se borra ----
	ejecutar(`INSERT INTO supplier_information (supplier_id, supplier_name, status) VALUES (1,'PROVEEDOR',1)`)
	ejecutar(`INSERT INTO product_purchase (id, purchase_id, chalan_no, supplier_id) VALUES (1, 1, 'CH-1', 1)`)
	if _, bloqueado := borrar(`DELETE FROM supplier_information WHERE supplier_id = 1`); !bloqueado {
		t.Error("el proveedor con una compra se borró")
	}

	// ---- compras: con renglón no se borra, y una compra vacía sí ----
	ejecutar(`INSERT INTO product_purchase (id, purchase_id, chalan_no, supplier_id) VALUES (2, 2, 'CH-2', 1)`)
	ejecutar(`INSERT INTO product_purchase_details (id, purchase_id, product_id, quantity) VALUES (1, 2, 100, 5)`)
	msg, bloqueado = borrar(`DELETE FROM product_purchase WHERE purchase_id = 2`)
	if !bloqueado {
		t.Error("la compra con renglones de mercancía se borró")
	} else if !strings.Contains(msg, "No se puede eliminar la compra") {
		t.Errorf("el mensaje para la compra fue %q", msg)
	}
	if _, bloqueado := borrar(`DELETE FROM product_purchase WHERE purchase_id = 1`); bloqueado {
		t.Error("una compra sin renglones ni asientos sigue sin poder borrarse")
	}
	// El mismo caso por el otro lado: un asiento con ese número de documento.
	ejecutar(`INSERT INTO product_purchase (id, purchase_id, chalan_no, supplier_id) VALUES (3, 3, 'CH-3', 1)`)
	ejecutar(`INSERT INTO acc_vaucher (id, VNo, referenceNo) VALUES (1,'DV-1','3')`)
	if _, bloqueado := borrar(`DELETE FROM product_purchase WHERE purchase_id = 3`); !bloqueado {
		t.Error("la compra con asiento contable se borró")
	}

	// ---- productos: uno comprado no se borra ----
	ejecutar(`INSERT INTO product_information (product_id, product_name) VALUES (100,'CARNE')`)
	ejecutar(`INSERT INTO product_information (product_id, product_name) VALUES (101,'SIN MOVIMIENTO')`)
	if _, bloqueado := borrar(`DELETE FROM product_information WHERE product_id = 100`); !bloqueado {
		t.Error("el producto con compras registradas se borró")
	}
	if _, bloqueado := borrar(`DELETE FROM product_information WHERE product_id = 101`); bloqueado {
		t.Error("un producto sin documentos tiene que poder borrarse")
	}

	// ---- servicios: uno cotizado no se borra ----
	ejecutar(`INSERT INTO product_service (service_id, service_name) VALUES (7,'FLETE')`)
	ejecutar(`INSERT INTO product_service (service_id, service_name) VALUES (8,'SIN MOVIMIENTO')`)
	ejecutar(`INSERT INTO quotation_service_used (id, quot_id, service_id) VALUES (1,'COT-1',7)`)
	if _, bloqueado := borrar(`DELETE FROM product_service WHERE service_id = 7`); !bloqueado {
		t.Error("el servicio usado en una cotización se borró")
	}
	if _, bloqueado := borrar(`DELETE FROM product_service WHERE service_id = 8`); bloqueado {
		t.Error("un servicio sin uso tiene que poder borrarse")
	}

	// ---- cuentas por pagar: con pagos aplicados no se borran ----
	ejecutar(`INSERT INTO accounts_payable (id, supplier_id, amount, paid_amount) VALUES (9,'1',100,10)`)
	ejecutar(`INSERT INTO accounts_payable (id, supplier_id, amount, paid_amount) VALUES (10,'1',100,0)`)
	if _, bloqueado := borrar(`DELETE FROM accounts_payable WHERE id = 9`); !bloqueado {
		t.Error("una cuenta por pagar ya pagada se borró")
	}
	if _, bloqueado := borrar(`DELETE FROM accounts_payable WHERE id = 10`); bloqueado {
		t.Error("una cuenta por pagar sin pagos tiene que poder borrarse")
	}

	// ---- el cobro también ata al cliente ----
	//
	// Con un cliente PROPIO y no con el 2: al 2 se le borró antes (era el caso de
	// «sin documentos, se puede borrar») y un trigger solo salta por las filas que
	// existen —borrar un cliente que ya no está no dispara nada—, así que
	// reutilizarlo probaría el caso vacío creyendo probar el lleno.
	ejecutar(`INSERT INTO customer_information (customer_id, customer_name, status) VALUES (3,'CON COBRO',1)`)
	ejecutar(`INSERT INTO debt_collection (id, customer_id, amount) VALUES (1, 3, 50)`)
	if _, bloqueado := borrar(`DELETE FROM customer_information WHERE customer_id = 3`); !bloqueado {
		t.Error("el cliente con un cobro registrado se borró")
	}

	// ---- y todo esto desaparece cuando se quita la protección (semillar) ----
	if err := QuitarProteccionDeBorrado(db, "sqlite"); err != nil {
		t.Fatalf("no se pudo quitar la protección: %v", err)
	}
	if _, bloqueado := borrar(`DELETE FROM customer_information WHERE customer_id = 1`); bloqueado {
		t.Error("quitada la protección, el borrado tiene que pasar: si no, el reseteo de la demo no podría trabajar")
	}
}
