package persistence

// mysqldump.go — respaldos lógicos de una base MySQL.
//
// POR QUÉ MYSQLDUMP Y NO COPIAR ARCHIVOS.
//
// El estado de una base InnoDB está repartido en varios archivos (el .ibdata1,
// un archivo por tabla, los redo logs) y una copia hecha a mitad de camino no es
// una base de datos: es un montón de archivos que no encajan entre sí. El
// respaldo correcto en MySQL es un volcado LÓGICO: sentencias SQL que se pueden
// ejecutar en cualquier MySQL, en otra máquina, dentro de cinco años.
//
// El coste de esa decisión es conocido y no se esconde: el volcado es más grande
// que el archivo físico y no permite restaurar en segundos. A cambio el respaldo
// es portable y legible. Para un ERP de una pyme, donde el respaldo se hace una
// vez al día y se restaura una vez al año, esa es la elección correcta sin
// discusión.
//
// POR QUÉ MYSQLDUMP Y NO ESCRIBIR SQL A MANO.
//
// Escribir el volcado desde Go obligaría a reimplementar SHOW CREATE TABLE, el
// escapado de literales, el formato de binarios y los procedimientos
// almacenados. mysqldump lleva veinte años haciéndolo y lo hace bien. La
// decisión que sí es nuestra es CÓMO se invoca, y por eso este archivo existe.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// VolcarBaseMySQL escribe en w el volcado lógico de la base c.Nombre.
//
// Los parámetros que se pasan a mysqldump están justificados uno por uno, porque
// un volcado "que funciona" en una demo puede estar失去 datos en producción:
//
//	--single-transaction  Foto consistente sin bloquear. Empieza una transacción
//	                      y todo el volcado sale de la misma foto. Sin esto,
//	                      mysqldump pone un LOCK TABLES sobre cada tabla y una
//	                      empresa que está facturando se para.
//	--quick               No bloquea la tabla mientras lee. Con
//	                      single-transaction es seguro y evita que una consulta
//	                      grande retenga el registro.
//	--routines            Los procedimientos y funciones también son datos. Sin
//	                      esto, un respaldo que restaura las tablas pero no las
//	                      reglas de negocio que las usan.
//	--triggers            Lo mismo con los disparadores.
//	--events              Y con los trabajos programados.
//	--add-drop-database   El DROP del propio respaldo evita "tabla ya existe" al
//	                      restaurar sobre una base que no se vació antes.
//	--hex-blob            Los datos binarios se escriben en hexadecimal. Sin esto,
//	                      un byte 0x1A corta el volcado, y una foto o un PDF
//	                      guardados en la base salen corruptos al restaurar.
//	--set-gtid-purged=OFF El servidor puede no tener GTID activados; preguntar por
//	                      ellos es un error en un servidor normal.
//	--default-character-set=utf8mb4  El esquema heredado es utf8mb3 y sin esto un
//	                      acento puede no volver. utf8mb4 es su ampliación y no
//	                      cambia lo que ya cabía.
func VolcarBaseMySQL(c ConfigMySQL, w io.Writer) error {
	if c.Nombre == "" {
		return fmt.Errorf("no se puede volcar una base sin nombre")
	}
	dump, err := LocalizarMySQLDump()
	if err != nil {
		return err
	}
	puerto := c.Port
	if puerto == 0 {
		puerto = 3306
	}
	host := c.Host
	if host == "" {
		host = MySQLHostPorDefecto
	}
	usuario := c.Usuario
	if usuario == "" {
		usuario = MySQLUsuarioDefecto
	}

	args := []string{
		"--host=" + host,
		"--port=" + strconv.Itoa(puerto),
		"--user=" + usuario,
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--add-drop-database",
		"--hex-blob",
		"--set-gtid-purged=OFF",
		"--default-character-set=utf8mb4",
	}
	if c.Clave != "" {
		// La clave va en la línea de comandos porque mysqldump no acepta de otra
		// forma. Aparece en la lista de procesos mientras dura el volcado; con un
		// servidor en localhost eso solo lo ve quien ya puede entrar a la máquina.
		// La alternativa (MYSQL_PWD) evita la línea de comandos y a cambio deja la
		// clave en el entorno de otro proceso.
		args = append(args, "--password="+c.Clave)
	}
	args = append(args, c.Nombre)

	cmd := exec.Command(dump, args...)
	cmd.Stdout = w
	// stderr se captura aparte: si el volcado falla, el error dice por qué, y va
	// en la respuesta en vez de desaparecer por la consola del servidor web.
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysqldump no pudo volcar la base %s: %w\n%s",
			c.Nombre, err, ultimasLineas(stderr.String(), 6))
	}
	return nil
}

// LocalizarMySQLDump busca el mysqldump que acompaña al mysqld.
//
// Va por el mismo camino que LocalizarMySQL y con la misma idea: el volcado
// tiene que ser de LA MISMA VERSIÓN que el servidor. Un mysqldump de 5.7
// conectado a un MySQL 8 falla al toparse con columnas que no conoce, y el
// respaldo falla justo cuando hace falta.
//
// La búsqueda es "el hermano del mysqld que ya encontramos": si hay un mysqld en
// una ruta concreta, el mysqldump está en la misma carpeta. Así no puede pasar
// que se empareje un servidor de una versión con un volcado de otra.
func LocalizarMySQLDump() (string, error) {
	nombre := "mysqldump"
	if tieneExtWindows() {
		nombre = "mysqldump.exe"
	}

	// 1) Al lado del mysqld que ya se localizó: garantiza misma versión.
	if mysqld, _, err := LocalizarMySQL(); err == nil {
		hermano := filepath.Join(filepath.Dir(mysqld), nombre)
		if info, err := os.Stat(hermano); err == nil && !info.IsDir() {
			return hermano, nil
		}
	}

	// 2) Rutas habituales propias del ERP.
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	for _, d := range dirs {
		for _, rel := range []string{
			filepath.Join("mysql", "bin", nombre),
			filepath.Join("bin", "mysql", nombre),
		} {
			candidata := filepath.Join(d, rel)
			if info, err := os.Stat(candidata); err == nil && !info.IsDir() {
				return candidata, nil
			}
		}
	}

	// 3) XAMPP y Laragon, que es donde un desarrollador de Windows lo tiene.
	rutas := []string{`C:\xampp\mysql\bin\` + nombre}
	if entradas, err := os.ReadDir(`C:\laragon\bin\mysql`); err == nil {
		versiones := []string{}
		for _, e := range entradas {
			if e.IsDir() {
				versiones = append(versiones, e.Name())
			}
		}
		for i := 0; i < len(versiones); i++ {
			for j := i + 1; j < len(versiones); j++ {
				if versiones[j] > versiones[i] {
					versiones[i], versiones[j] = versiones[j], versiones[i]
				}
			}
		}
		for _, v := range versiones {
			rutas = append(rutas, filepath.Join(`C:\laragon\bin\mysql`, v, "bin", nombre))
		}
	}
	for _, r := range rutas {
		if info, err := os.Stat(r); err == nil && !info.IsDir() {
			return r, nil
		}
	}
	// 4) El PATH, al final y no al principio: ver LocalizarMySQL.
	if p, err := exec.LookPath(nombre); err == nil {
		return p, nil
	}

	return "", fmt.Errorf(
		"no se encontró mysqldump en esta máquina, y sin él no se puede hacer un " +
			"respaldo que sirva. mysqldump viene con MySQL: instale MySQL 8 completo " +
			"(no el paquete 'server' recortado) y vuelva a intentarlo")
}

// tieneExtWindows dice si los ejecutables llevan extensión.
//
// Existe para no tener un if runtime.GOOS == "windows" repetido en cada sitio:
// el nombre del binario es lo único que cambia, y ese detalle está en un solo
// lugar.
func tieneExtWindows() bool {
	// os.PathSeparator es '\\' solo en Windows. No es la forma más elegante de
	// preguntar por el sistema operativo, pero es la que no necesita importar
	// runtime en dos archivos.
	return os.PathSeparator == '\\'
}

// POR QUÉ NO HAY UN TIMEOUT AQUÍ.
//
// El volcado es síncrono y su duración depende del volumen de la base, del disco
// y de si hay alguien facturando en ese momento. Poner un límite de cinco minutos
// "según el tamaño" no sería un dato, sería una suposición: en la base de un
// cliente con años de histórico, un límite inventado corta el respaldo por la
// mitad y deja un .sql que parece bueno y no lo es. Peor que tardar.
//
// Lo que sí hay es que la interfaz avise mientras tarda, para que el botón no
// parezca muerto.
