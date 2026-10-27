package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-sql-driver/mysql"
)

// proteccion_borrado.go — la integridad de borrado vive en la BASE, no en el
// handler.
//
// Un documento con movimiento no se borra. Poner esa regla en cada handler tiene
// un problema que se ve al revisar el código: son treinta sitios, la regla se
// aplica desigual, y cualquier camino nuevo que toque la tabla —un import, una
// herramienta, un `DELETE` suelto— la salta sin que nadie lo note. Aquí la regla
// baja a la base: cualquier borrado que la viole se aborta, venga de donde venga.
//
// # POR QUÉ TRIGGERS Y NO CLAVES FORÁNEAS (medido, no razonado)
//
// La clave foránea es la herramienta natural para esto, y en este proyecto NO
// sirve. Comprobado contra el motor que se usa por defecto (SQLite, ver
// dialect.go), con el mismo driver y el mismo DSN de la aplicación
// (infrastructure/persistence/database.go):
//
//  1. Con el DSN del proyecto, `PRAGMA foreign_keys` vale 0 y una FK declarada
//     es decorativa: insertando un cliente y una factura que lo referencia, el
//     `DELETE` del cliente PASA y la factura queda huérfana. La FK nunca
//     protegió nada.
//  2. Activando el pragma (`_pragma=foreign_keys(1)`) sí bloquea, pero el
//     mensaje que llega es `FOREIGN KEY constraint failed (1811)`: no dice qué
//     documento ata ni qué hacer.
//  3. `ALTER TABLE factura ADD CONSTRAINT ... FOREIGN KEY ...` responde
//     «near "FOREIGN": syntax error». SQLite NO sabe añadir una clave foránea a
//     una tabla que ya existe: la única vía sería reconstruir la tabla entera
//     (crear, copiar, borrar, renombrar), que es exactamente lo que la política
//     del proyecto prohíbe por tocar datos existentes.
//  4. Un TRIGGER sobre una tabla ya existente sí bloquea, sin pragma, sin
//     reconstruir nada, y el mensaje llega íntegro a Go con NUESTRO texto:
//     `constraint failed: No se puede eliminar el cliente: tiene facturas
//     registradas. (1811)`. Ese texto es el que se enseña en pantalla.
//
// A eso se suma que los FK de los volcados MySQL que existen hoy están
// declarados `ON DELETE CASCADE` (cash_registers y api_keys hacia branches), es
// decir, el sentido contrario al que se busca: borrar la sucursal borra la caja.
//
// Los triggers tienen además una propiedad que ninguna guarda de handler tiene:
// el motor los comprueba. `cmd/semillar` (el reseteo de la empresa demo) borra
// todas las tablas: con esto delante, tiene que quitar la protección
// explícitamente. Eso es lo que se quiere —que saltarse la regla sea una
// decisión escrita y no un descuido—.
//
// # POR QUÉ ESTAS SENTENCIAS NO VIVEN EN migrations/*.sql
//
// Porque splitSQL parte los archivos por ";\n" (database.go) y el cuerpo de un
// trigger lleva puntos y coma dentro: llegarían troceados a la base y no se
// crearía ninguno. El esquema que vive en código ya se aplica sentencia a
// sentencia con AplicarEsquema, y ese es el camino correcto para esto. De paso,
// el dialecto exacto del cuerpo (RAISE(ABORT) en SQLite, SIGNAL en MySQL) no se
// puede unificar, y aquí se elige por motor sin ramificar ningún handler.

// ReglaDeBorrado es un documento con movimiento que no se deja borrar.
type ReglaDeBorrado struct {
	// Nombre identifica al trigger dentro de la base. Lleva prefijo para poder
	// quitarlos todos de una vez y para no pisar un trigger de otra cosa.
	Nombre string
	// Tabla es la que se protege: la que NO se puede borrar.
	Tabla string
	// Condicion es una expresión booleana sobre OLD. Si es verdadera, el
	// borrado se aborta. Se escribe una sola vez para los dos motores: SQLite la
	// pone en su WHEN y MySQL dentro de un IF ... THEN.
	Condicion string
	// Mensaje es el texto que verá la persona. Va en la base, así que evita
	// comillas simples y jerga: se enseña tal cual.
	Mensaje string
}

// reglasDeBorrado son los documentos que, una vez tienen movimiento, no se
// borran.
//
// Cada condición nombra tablas y columnas reales del esquema; si alguna
// estuviera mal escrita, el CREATE TRIGGER falla al crearse (el motor valida las
// columnas) y el test que aplica estas reglas sobre el esquema migrado lo
// detecta. Es el mismo criterio que la prueba de las guardas de handler: una
// regla que no se puede crear es una regla que no protege.
var reglasDeBorrado = []ReglaDeBorrado{
	{
		Nombre: "cliente",
		Tabla:  "customer_information",
		Condicion: `EXISTS(SELECT 1 FROM invoice WHERE customer_id = OLD.customer_id)
			OR EXISTS(SELECT 1 FROM debt_collection WHERE customer_id = OLD.customer_id)
			OR EXISTS(SELECT 1 FROM quotation WHERE customer_id = OLD.customer_id)
			OR EXISTS(SELECT 1 FROM delivery_order WHERE customer_id = OLD.customer_id)`,
		Mensaje: "No se puede eliminar el cliente: tiene facturas, cobros, cotizaciones u ordenes de entrega registrados. " +
			"Conserve el registro y corrija sus datos si estan mal.",
	},
	{
		Nombre: "proveedor",
		Tabla:  "supplier_information",
		Condicion: `EXISTS(SELECT 1 FROM product_purchase WHERE supplier_id = OLD.supplier_id)
			OR EXISTS(SELECT 1 FROM accounts_payable WHERE supplier_id = OLD.supplier_id)
			OR EXISTS(SELECT 1 FROM vat_retention WHERE supplier_id = OLD.supplier_id)
			OR EXISTS(SELECT 1 FROM islr_retention WHERE supplier_id = OLD.supplier_id)`,
		Mensaje: "No se puede eliminar el proveedor: tiene compras, cuentas por pagar o retenciones registradas. " +
			"Conserve el registro y corrija sus datos si estan mal.",
	},
	{
		// La compra es un documento fiscal: movio mercancia, publico asiento y
		// dejo el IVA en el libro de compras.
		Nombre: "compra",
		Tabla:  "product_purchase",
		Condicion: `EXISTS(SELECT 1 FROM product_purchase_details WHERE purchase_id = OLD.purchase_id)
			OR EXISTS(SELECT 1 FROM acc_vaucher WHERE referenceNo = OLD.purchase_id)
			OR EXISTS(SELECT 1 FROM acc_transaction WHERE referenceNo = OLD.purchase_id)
			OR EXISTS(SELECT 1 FROM accounts_payable WHERE purchase_id = OLD.purchase_id)`,
		Mensaje: "No se puede eliminar la compra: tiene renglones de mercancia, asientos contables o cuentas por pagar asociados. " +
			"Registre el reverso en lugar de eliminarla.",
	},
	{
		Nombre: "producto",
		Tabla:  "product_information",
		Condicion: `EXISTS(SELECT 1 FROM invoice_details WHERE product_id = OLD.product_id)
			OR EXISTS(SELECT 1 FROM product_purchase_details WHERE product_id = OLD.product_id)`,
		Mensaje: "No se puede eliminar el producto: aparece en facturas o en compras ya registradas. " +
			"Dejelo inactivo en lugar de eliminarlo.",
	},
	{
		Nombre:    "servicio",
		Tabla:     "product_service",
		Condicion: `EXISTS(SELECT 1 FROM quotation_service_used WHERE service_id = OLD.service_id)`,
		Mensaje: "No se puede eliminar el servicio: esta usado en cotizaciones registradas. " +
			"Dejelo inactivo en lugar de eliminarlo.",
	},
	{
		// Una cuenta por pagar con pagos aplicados es un movimiento bancario o
		// de caja: deshacerla borrando la fila deja el asiento sin su documento.
		Nombre:    "cuenta_por_pagar",
		Tabla:     "accounts_payable",
		Condicion: `OLD.paid_amount > 0`,
		Mensaje: "No se puede eliminar una cuenta por pagar que ya tiene pagos aplicados. " +
			"Anule el pago con su reverso en lugar de eliminar la cuenta.",
	},
}

// ReglasDeBorrado devuelve las reglas tal como están definidas.
//
// Se exponen para que la comprobación de que apuntan a tablas y columnas reales
// pueda hacerse donde ya se sabe leer el esquema de las migraciones (el test del
// paquete web), en vez de mantener dos lectores del mismo esquema. Una regla que
// nombra una columna inexistente no se crea, y el arranque solo deja un aviso en
// el log: eso hay que cazarlo en una prueba, no en producción.
func ReglasDeBorrado() []ReglaDeBorrado { return reglasDeBorrado }

// prefijoTrigger agrupa los triggers de esta protección.
const prefijoTrigger = "erpgo_protege_"

// motorDeBorrado normaliza el motor para elegir el dialecto del cuerpo.
//
// La cadena vacía se trata como SQLite porque es el motor por defecto del
// proyecto (dialect.go) y el único que no necesita servidor.
func motorDeBorrado(driver string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "sqlite", "sqlite3":
		return "sqlite", nil
	case "mysql", "mariadb":
		return "mysql", nil
	}
	return "", fmt.Errorf("motor de base desconocido para la proteccion de borrado: %q", driver)
}

// SentenciasProteccion devuelve las sentencias que crean la protección, ya en el
// dialecto del motor.
//
// Es pura a propósito: así se puede comprobar sin base de datos que cada regla
// está completa, que los nombres no se repiten y que el cuerpo lleva nuestro
// mensaje y la tabla que protege.
func SentenciasProteccion(driver string) ([]string, error) {
	motor, err := motorDeBorrado(driver)
	if err != nil {
		return nil, err
	}
	var sentencias []string
	for _, r := range reglasDeBorrado {
		nombre := prefijoTrigger + r.Nombre
		switch motor {
		case "mysql":
			sentencias = append(sentencias,
				"DROP TRIGGER IF EXISTS "+nombre,
				fmt.Sprintf("CREATE TRIGGER %s BEFORE DELETE ON %s FOR EACH ROW\nBEGIN\n\tIF %s THEN\n\t\tSIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '%s';\n\tEND IF;\nEND",
					nombre, r.Tabla, unaLinea(r.Condicion), r.Mensaje))
		default:
			sentencias = append(sentencias,
				"DROP TRIGGER IF EXISTS "+nombre,
				fmt.Sprintf("CREATE TRIGGER %s BEFORE DELETE ON %s\nWHEN %s\nBEGIN\n\tSELECT RAISE(ABORT, '%s');\nEND",
					nombre, r.Tabla, unaLinea(r.Condicion), r.Mensaje))
		}
	}
	return sentencias, nil
}

// SentenciasQuitarProteccion devuelve los DROP, para las herramientas que sí
// tienen que borrar (el reseteo de la empresa demo).
func SentenciasQuitarProteccion(driver string) ([]string, error) {
	if _, err := motorDeBorrado(driver); err != nil {
		return nil, err
	}
	var sentencias []string
	for _, r := range reglasDeBorrado {
		sentencias = append(sentencias, "DROP TRIGGER IF EXISTS "+prefijoTrigger+r.Nombre)
	}
	return sentencias, nil
}

// AplicarProteccionDeBorrado crea los triggers en una base.
//
// Es idempotente: cada regla se borra antes de crearse, así que arrancar mil
// veces deja lo mismo y una regla corregida sustituye a la vieja (que es el
// fallo clásico de un trigger: se cambia el texto en el código y la base sigue
// con el de antes).
func AplicarProteccionDeBorrado(db *sql.DB, driver string) error {
	return ejecutarProteccion(db, driver, SentenciasProteccion)
}

// QuitarProteccionDeBorrado elimina los triggers de una base.
func QuitarProteccionDeBorrado(db *sql.DB, driver string) error {
	return ejecutarProteccion(db, driver, SentenciasQuitarProteccion)
}

func ejecutarProteccion(db *sql.DB, driver string, armar func(string) ([]string, error)) error {
	if db == nil {
		return fmt.Errorf("sin base de datos")
	}
	sentencias, err := armar(driver)
	if err != nil {
		return err
	}
	for _, s := range sentencias {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("%w <- %.140s", err, s)
		}
	}
	return nil
}

// protegidasYa anota las bases a las que este proceso ya les creó la protección.
//
// POR QUÉ HACE FALTA. La aplicación de la protección va al final de la
// aplicación del esquema, y esa función se llama una vez por empresa en el
// arranque… y otra vez cada vez que se abre la base de una empresa. Repetir 12
// sentencias DDL por apertura es tirar tiempo de arranque; hacerlo una vez por
// base y proceso es exactamente lo que se necesita, porque cada proceso nuevo
// vuelve a aplicarla (y así una regla corregida sustituye a la vieja).
//
// La clave es el propio handle: un *sql.DB por empresa, vivo durante todo el
// proceso.
var protegidasYa sync.Map

// AplicarProteccionUnaVez aplica la protección una sola vez por base y proceso.
//
// Si falla, se desanota: el siguiente intento vuelve a probar en vez de darse por
// hecho algo que no se hizo.
func AplicarProteccionUnaVez(db *sql.DB, driver string) error {
	if db == nil {
		return nil
	}
	if _, ya := protegidasYa.LoadOrStore(db, true); ya {
		return nil
	}
	if err := AplicarProteccionDeBorrado(db, driver); err != nil {
		protegidasYa.Delete(db)
		return err
	}
	return nil
}

// unaLinea aplana la condición para meterla en una sentencia sin que un salto de
// línea rompa nada. Se conserva el espaciado interno.
func unaLinea(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var (
	// SQLite (driver glebarez) envuelve el error así:
	//   constraint failed: <mensaje> (1811)
	reErrorSQLite  = regexp.MustCompile(`(?s)^constraint failed:\s*(.*?)\s*\(\d+\)\s*$`)
	reSufijoSQLite = regexp.MustCompile(`\s*\(\d+\)\s*$`)
)

// MensajeDeIntegridad saca el mensaje que debe ver una persona de un error de
// restricción de la base, y devuelve "" si el error no es de esa clase.
//
// POR QUÉ TRADUCE Y NO MUESTRA EL ERROR TAL CUAL.
//
// El texto crudo llega con el dialecto del motor dentro —`constraint failed:
// ... (1811)` en SQLite, `Error 1644 (45000): ...` en MySQL— y con el mensaje
// genérico del motor cuando la restricción no es nuestra («FOREIGN KEY
// constraint failed», «Cannot delete or update a parent row»). Enseñar eso es
// lo que se quiere evitar: la persona que intenta borrar necesita leer qué la
// ata, no cómo se llama el motor.
func MensajeDeIntegridad(err error) string {
	if err == nil {
		return ""
	}

	// MySQL: el número de error es más fiable que el texto, que se traduce.
	//   1644 = SIGNAL de un trigger nuestro (lleva nuestro mensaje)
	//   1451 = borrar una fila padre con hijos (el mensaje del motor)
	//   1452 = insertar un hijo sin padre
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case 1644:
			return limpio(myErr.Message)
		case 1451:
			return "No se puede eliminar: otros documentos dependen de este registro. " +
				"Dejelo inactivo o registre el reverso en lugar de eliminarlo."
		case 1452:
			return "No se puede guardar: el registro al que apunta no existe. " +
				"Elija uno existente antes de guardar."
		}
	}

	m := err.Error()

	// SQLite: el mensaje útil va dentro del envoltorio.
	if sub := reErrorSQLite.FindStringSubmatch(strings.TrimSpace(m)); sub != nil {
		interno := strings.TrimSpace(sub[1])
		switch {
		case strings.HasPrefix(interno, "FOREIGN KEY constraint failed"):
			// El mensaje del motor no dice qué ata: se sustituye por uno que sí.
			return "No se puede eliminar: otros documentos dependen de este registro. " +
				"Dejelo inactivo o registre el reverso en lugar de eliminarlo."
		case strings.HasPrefix(interno, "UNIQUE constraint failed"),
			strings.HasPrefix(interno, "NOT NULL constraint failed"),
			strings.HasPrefix(interno, "CHECK constraint failed"):
			// Son restricciones, pero no un borrado impedido. Se dejan pasar sin
			// traducir para no disfrazar un alta duplicada de borrado bloqueado.
			return ""
		}
		return limpio(interno)
	}

	// MySQL sin número de error reconocido pero con el texto del límite.
	lower := strings.ToLower(m)
	switch {
	case strings.Contains(lower, "cannot delete or update a parent row"),
		strings.Contains(lower, "foreign key constraint fails"):
		return "No se puede eliminar: otros documentos dependen de este registro. " +
			"Dejelo inactivo o registre el reverso en lugar de eliminarlo."
	case strings.Contains(lower, "sqlstate '45000'"), strings.Contains(lower, "error 1644"):
		// El texto viene dentro del error del driver; se recorta el envoltorio.
		if i := strings.Index(m, ": "); i >= 0 {
			return limpio(reSufijoSQLite.ReplaceAllString(m[i+2:], ""))
		}
	}
	return ""
}

// limpio recorta el envoltorio del driver y el sufijo numérico del motor.
func limpio(s string) string {
	s = strings.TrimSpace(s)
	s = reSufijoSQLite.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "foreign key constraint failed") {
		return ""
	}
	return s
}
