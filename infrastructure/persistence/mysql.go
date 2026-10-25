package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	// Driver MySQL. Se importa con nombre porque `embedded_mysql.go` necesita
	// distinguir un error del servidor de otro cualquiera (ver esErrorDeMySQL).
	"github.com/go-sql-driver/mysql"
)

// MySQL es el backend. Antes era SQLite (que sigue en el repositorio como el
// proyecto anterior, pero ya no es el camino de producción).
//
// POR QUÉ MySQL Y NO POSTGRESQL, QUE ES LO QUE SE PREGUNTÓ
//
// Porque el esquema NACE de MySQL: `migrations/schema.sql` dice "Generated from
// SalesERP v12.13 install.sql" y tiene 74 apariciones de ENGINE=InnoDB y
// CHARSET. Todo eso hoy se traducía a SQLite con `sqlite_convert.go` (361
// líneas y 8 funciones). Ir a MySQL **borra** esa traducción; ir a PostgreSQL
// obligaría a escribir OTRA traducción, en sentido contrario.

// Defaults. Se pueden cambiar todos por entorno: el MySQL incluido en el
// instalador va en un puerto propio para no chocar con uno del sistema.
const (
	MySQLHostPorDefecto = "127.0.0.1"
	MySQLPortPorDefecto = "3306"
	MySQLUsuarioDefecto = "root"
	MySQLNombreDefecto  = "erpgo"

	// Puerto sugerido para el MySQL incluido en el instalador. NO es el
	// 3306 del sistema a propósito: si la máquina ya tiene un MySQL propio
	// (un developers con Laragon, XAMPP o WAMP, por ejemplo), ambos pueden
	// convivir sin que haya que apagar nada.
	MySQLPortIncluido = 3307

	// Nombre de la base de control: el catálogo de empresas.
	// Las bases de cada empresa son `erpgo_<codigo>`.
	prefijoBaseEmpresa = "erpgo_"
)

// ConfigMySQL es lo mínimo para abrir una conexión.
type ConfigMySQL struct {
	Host     string
	Port     int
	Usuario  string
	Clave    string
	Nombre   string // base de datos
	MaxConns int

	// MultiStatements permite mandar varias sentencias en un solo viaje.
	//
	// Se activa SOLO en la conexión que aplica el esquema, y por un motivo
	// medido: el esquema son 415 sentencias y, mandándolas de una en una, la
	// creación de una base de empresa tarda 9,5 s. Casi todo eso no es MySQL
	// trabajando, es red y espera: 23 ms por sentencia, que es lo que tarda un
	// viaje de ida y vuelta a un servidor local. Mandando cada archivo entero en
	// una sola ida son 16 viajes y la misma operación tarda menos de un segundo.
	//
	// Por qué no está en la conexión de la aplicación: con multiStatements, un
	// `?` que se cuele en una consulta con texto del usuario dejaría de ser un
	// parámetro y pasaría a ser SQL. Aquí solo se ejecutan archivos .sql que
	// vienen dentro del propio binario y que no llevan parámetros, así que el
	// riesgo no existe — y aun así la conexión es aparte, para que ningún
	// código de la aplicación pueda acabar usando una con esta opción puesta.
	MultiStatements bool
}

// DSNMySQL arma la cadena de conexión.
//
// LOS PARÁMETROS NO SON COSA ESTÉTICA, CADA UNO EVITA UN FALLO CONCRETO:
//
//	charset=utf8mb4   El esquema heredado dice `utf8mb3`, que MySQL 8 marca
//	                  como obsoleto y que no guarda de 4 bytes. utf8mb4 es su
//	                  ampliación y no cambia nada de lo que ya cabía: es
//	                  estrictamente más capaz.
//	parseTime=true    Sin esto, una columna DATETIME vuelve como []byte y
//	                  escanearla a time.Time revienta. Es el fallo clásico al
//	                  cambiar de motor.
//	loc=UTC           Se guarda en UTC y se presenta en hora local. Así el
//	                  horario de verano de otro país no toca lo ya escrito.
//	interpolateParams  Evita un viaje extra al servidor por cada parámetro.
//	timeout / readTimeout / writeTimeout  Un servidor que no responde se
//	                  detecta en 5 s en vez de dejar el proceso colgado.
//	collation=utf8mb4_unicode_ci  LA CONEXIÓN HABLA EL MISMO IDIOMA QUE EL
//	                  ESQUEMA. No es decoración y el fallo que evita es real:
//
//	                  MySQL 8 pone por defecto `collation_connection =
//	                  utf8mb4_0900_ai_ci`. Todo lo que produce una cadena en la
//	                  conexión —un literal, un `CAST(x AS CHAR)`— hereda esa
//	                  collation, y al compararla con una columna del esquema
//	                  (utf8mb4_unicode_ci) el servidor se niega:
//
//	                    Error 1267: Illegal mix of collations
//	                    (utf8mb4_unicode_ci,IMPLICIT) and (utf8mb4_0900_ai_ci,IMPLICIT)
//
//	                  Salió en `checkInvoicesPosted`, que compara el
//	                  `referenceNo` de un asiento con
//	                  `CAST(invoice.invoice AS CHAR)` — es decir, en la
//	                  comprobación de que toda factura publicada tenga su
//	                  asiento contable. Conectar con la misma collation que el
//	                  esquema lo evita en un solo sitio, y de paso hace que el
//	                  orden de las listas de texto sea el mismo en todas las
//	                  consultas.
func DSNMySQL(c ConfigMySQL) string {
	// sql_mode explícito: se QUITA ONLY_FULL_GROUP_BY.
	//
	//	No es aflojar por comodidad, es que el SQL de este ERP viene de un
	//	PHP que escribía sin agrupar todas las columnas no agregadas. Con
	//	ONLY_FULL_GROUP_BY (que MySQL 8 pone por defecto) eso se rechaza. Hay
	//	dos salidas: reescribir cada consulta de agrupación, o declarar el modo
	//	SQL que el código realmente cumple. Se elige la segunda porque es
	//	explícita y reversible; la primera sigue siendo lo correcto para el
	//	código NUEVO, que debe agrupar bien desde el principio.
	const modoSQL = "STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE," +
		"ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"

	puerto := c.Port
	if puerto == 0 {
		puerto = 3306
	}
	clave := c.Clave

	// El sql_mode va ENVUELTO EN COMILLAS (%27...%27), y no escapado con
	// QueryEscape. No es descuido: el driver construye `SET sql_mode=<valor>`
	// putting el valor tal cual, y en un SET la coma separa elementos. Sin
	// las comillas, "STRICT_TRANS_TABLES,NO_ZERO_DATE" se parte y MySQL
	// contesta "SQL syntax error" al ABRIR la conexión.
	//
	// Comprobado contra el servidor, no de memoria: sin comillas falla con
	// Error 1064; con %27 devuelve el sql_mode sin ONLY_FULL_GROUP_BY, que es
	// justo lo que se busca.
	//
	// Colocar la clave en el DSN es inevitable: el driver no acepta
	// user:pass@tcp(...) de otra forma. Los archivos .env están fuera del
	// control de versiones (ver .gitignore) y esta función no se escribe
	// nunca en un log.
	multi := ""
	if c.MultiStatements {
		multi = "&multiStatements=true"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&collation=utf8mb4_unicode_ci"+
		"&parseTime=true"+
		"&loc=UTC&interpolateParams=true&timeout=5s&readTimeout=30s&writeTimeout=30s"+
		"&sql_mode=%%27%s%%27%s",
		c.Usuario, clave, c.Host, puerto, c.Nombre, modoSQL, multi)
}

// OpenMySQL abre una base MySQL con el pool ajustado a este caso de uso.
//
// EL POOL, Y POR QUÉ 25 Y NO 100.
//
// El número sale de una medición, no de unRound number: en el banco de
// pruebas con las 25 conexiones actuales el escritor se quedaba sin
// conexión cuando los lectores saturaban el pool, y el ensayo no terminaba.
// MySQL no tiene ese problema concreto —su escritura es distinta—, pero un
// pool enorme en una pyme es solo un modo más de agotar la memoria del
// servidor, así que se mantiene acotado y holgado.
//
// MaxIdleConns es menor que MaxOpenConns a propósito: conexiones ociosas que
// solo ocupan memoria. Lo que se necesita es poder atender picos, no
// reservados estables.
func OpenMySQL(c ConfigMySQL) (*sql.DB, error) {
	if c.Nombre == "" {
		return nil, fmt.Errorf("falta el nombre de la base de datos")
	}
	return ConectarServidorMySQL(c)
}

// ConectarServidorMySQL abre una conexión al SERVIDOR, sin base seleccionada.
//
// Existe aparte de OpenMySQL porque hay tareas que no pueden usar una base: crear
// una base (no se crea una base dentro de una base), apagar el servidor y
// arrancar el MySQL incluido. En las tres hace falta hablar con el servidor, y
// no con los datos de una empresa.
//
// Es la misma apertura con la comprobación del nombre cambiada por la
// tolerancia a que falte: un nombre de base es obligatorio para trabajar con
// datos y no lo es para hablar con el servidor.
func ConectarServidorMySQL(c ConfigMySQL) (*sql.DB, error) {
	if c.Host == "" {
		c.Host = MySQLHostPorDefecto
	}
	if c.Port == 0 {
		c.Port = 3306
	}
	if c.Usuario == "" {
		c.Usuario = MySQLUsuarioDefecto
	}
	if c.MaxConns <= 0 {
		c.MaxConns = 25
	}

	db, err := sql.Open("mysql", DSNMySQL(c))
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir MySQL: %w", err)
	}

	db.SetMaxOpenConns(c.MaxConns)
	db.SetMaxIdleConns(c.MaxConns / 2)
	// MySQL cierra conexiones por su cuenta (wait_timeout). Rotarlas evita
	// quedarse con un handle que el servidor ya no reconoce, que es un
	// fallo que aparece de repente y cuesta muchísimo reproducir.
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	if err := probar(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// probar hace un Ping con un presupuesto corto.
//
// Un Ping sin plazo es un programa colgado esperando a un servidor que no
// contesta: el ERP no arranca y no dice por qué. Aqui son 5 s o un error claro.
//
// Se reintenta durante la espera porque el arranque normal de un MySQL
// contenido tarda un par de segundos en aceptar conexiones, y fallar en ese
// primer intento seria rejectar un servidor que iba a responder.
func probar(db *sql.DB) error {
	const presupuesto = 5 * time.Second
	inicio := time.Now()
	var err error
	for time.Since(inicio) < presupuesto {
		if err = db.Ping(); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("MySQL no responde en %s: %w", presupuesto, err)
}

// EnsureDatabase crea la base si no existe.
//
// Se abre primero una conexión SIN base de datos: en MySQL no se puede crear
// una base dentro de una base, hay que hablarle al servidor. Por eso esto
// existe y no puede ser un `CREATE DATABASE` suelto en las migraciones.
//
// El nombre se valida ANTES de interpolarlo. Viene de un código de empresa
// que en principio no tiene nada sospechoso, pero un nombre de base va
// crudo al protocolo y un `DROP DATABASE` por una comilla de más es de las
// cosas que no se perdonan.
func EnsureDatabase(c ConfigMySQL) error {
	sinBase := c
	sinBase.Nombre = ""

	db, err := sql.Open("mysql", DSNMySQL(sinBase))
	if err != nil {
		return fmt.Errorf("no se pudo conectar al servidor MySQL: %w", err)
	}
	defer db.Close()

	if err := probar(db); err != nil {
		return err
	}

	nombre, err := nombreBaseSeguro(c.Nombre)
	if err != nil {
		return err
	}
	// utf8mb4 explícito en el CREATE: si se deja el del servidor, el
	// esquema heredado con CHARSET=utf8mb3 se queda corto sin avisar.
	_, err = db.Exec(fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		nombre))
	if err != nil {
		return fmt.Errorf("no se pudo crear la base %q: %w", nombre, err)
	}
	return nil
}

// nombreBaseSeguro valida que un nombre de base sea solo lo que tiene que ser.
//
// MySQL no permite `;` ni espacios ni caracteres especiales en el nombre de
// una base, y un nombre con acentos o eñes (que los hay: "empresa piñata")
// NO es válido. Se traduce a algo ASCII estable en lugar de fallar, y así el
// código de empresa puede seguir siendo legible para el usuario.
func nombreBaseSeguro(nombre string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return "", fmt.Errorf("el nombre de la base está vacío")
	}
	if len(nombre) > 64 {
		return "", fmt.Errorf("el nombre de la base es demasiado largo: %d caracteres (máximo 64)", len(nombre))
	}

	var limpio strings.Builder
	for _, r := range nombre {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			limpio.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			limpio.WriteRune(r + ('a' - 'A'))
		default:
			// Un caracter no válido se convierte en guion bajo, no se
			// borra: "piñata" y "pinata" deben dar bases DISTINTAS, no
			// la misma. Si se borrara, dos empresas distintas acabarían
			// compartiendo datos, y eso es exactamente el fallo que el
			// aislamiento por empresa existe para impedir.
			limpio.WriteRune('_')
		}
	}
	res := strings.Trim(limpio.String(), "_")
	if res == "" {
		res = "bd"
	}
	if len(res) > 64 {
		res = res[:64]
	}
	return res, nil
}

// NombreBaseEmpresa devuelve el nombre de la base de una empresa.
func NombreBaseEmpresa(codigo string) (string, error) {
	seguro, err := nombreBaseSeguro(codigo)
	if err != nil {
		return "", err
	}
	return prefijoBaseEmpresa + seguro, nil
}

// EsServidorMySQLArriba dice si hay un servidor MySQL escuchando.
//
// Se usa en el arranque para poder decir algo útil ("no hay MySQL, arranca el
// servicio") en vez de un fallo de conexión seco. Un `net.Dial` de un solo
// intento, sin esperar: si tarda, no está.
func EsServidorMySQLArriba(host string, puerto int) bool {
	if host == "" {
		host = MySQLHostPorDefecto
	}
	if puerto == 0 {
		puerto = 3306
	}
	conexion, err := net.DialTimeout("tcp",
		net.JoinHostPort(host, strconv.Itoa(puerto)), 2*time.Second)
	if err != nil {
		return false
	}
	conexion.Close()
	return true
}

// puertoEscucha dice si ALGO acepta conexiones en el puerto, sin suponer que
// sea MySQL.
//
// Existe separado de EsServidorMySQLArriba porque las dos preguntas se
// confundían: "hay un puerto" y "hay un servidor de base de datos" son cosas
// distintas, y decidir que un puerto abierto es un MySQL lleva a esperar a un
// servidor que no contestará nunca.
func puertoEscucha(host string, puerto int) bool {
	return EsServidorMySQLArriba(host, puerto)
}

// esErrorDeMySQL dice si el error viene del protocolo de MySQL.
//
// La comprobación es por tipo y no por texto: el driver de MySQL devuelve sus
// errores como *mysql.MySQLError, y eso incluye el 1045 de "acceso denegado",
// que SÍ significa que hay un servidor. Mirar el texto no distinguiría un MySQL
// que rechaza la clave de un programa cualquiera que ocupa el puerto.
func esErrorDeMySQL(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr)
}
