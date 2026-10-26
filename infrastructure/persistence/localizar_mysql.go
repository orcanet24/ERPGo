package persistence

// localizar_mysql.go — dónde vive la instalación de MySQL de ESTA máquina.
//
// LO QUE ESTE ARCHIVO YA NO HACE.
//
// Antes, aquí vivía un gestor que, si no había un servidor escuchando en
// DB_PORT, ejecutaba `mysqld --initialize-insecure` con un directorio de datos
// propio y levantaba un MySQL desde el binario del ERP. Es decir: el ERP se
// fabricaba su propia instalación de base de datos, a espaldas de quien
// administra el servidor, y la apagaba al terminar.
//
// Eso se eliminó. El motor es infraestructura del servidor anfitrión: se instala
// y se arranca como servicio del sistema, y ERPGo solo se conecta. El ERP no
// instala MySQL ni MariaDB, no inicializa datadirs y no arranca ni apaga
// procesos de base de datos. Si el motor no responde, el arranque se detiene con
// un mensaje claro y sin tocar nada del sistema operativo.
//
// Lo único que queda es LocalizarMySQL: encontrar el binario instalado. Se usa
// para hallar el `mysqldump` de la MISMA versión y para dar un mensaje útil.
// Buscar no es instalar: esto solo lee rutas del disco.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LocalizarMySQL busca una instalación de MySQL/MariaDB en la máquina.
//
// EL ORDEN IMPORTA. Se empieza por lo que acompaña al ERP (por si el
// administrador dejó ahí una instalación portátil), luego por lo que ya había en
// la máquina y se termina por el PATH, que es donde solo aparece MySQL si el
// usuario lo instaló "a mano". No se busca en el PATH antes que en los sitios
// conocidos a propósito: el PATH puede apuntar a un MySQL de 5.7 de una máquina
// de desarrollo, y usar ese binario sin querer es peor que no encontrar nada.
func LocalizarMySQL() (binario, baseDir string, err error) {
	// Carpetas donde puede estar el programa: al lado del ejecutable y en el
	// directorio de trabajo. Se miran los dos porque en desarrollo el binario
	// se compila en un sitio y se ejecuta en otro.
	dirs := func() []string {
		exe, _ := os.Executable()
		dirs := []string{}
		if exe != "" {
			dirs = append(dirs, filepath.Dir(exe))
		}
		if wd, err := os.Getwd(); err == nil {
			dirs = append(dirs, wd)
		}
		return dirs
	}

	// El nombre del ejecutable es el mismo en Windows y en Linux ("mysqld"); en
	// Windows lo que cambia es la extensión, y las rutas de abajo ya llevan la
	// que corresponde. En Unix, exec.LookPath se encarga de la extensión.
	exeNombre := "mysqld"
	if runtime.GOOS == "windows" {
		exeNombre = "mysqld.exe"
	}
	rutas := []string{}
	for _, d := range dirs() {
		rutas = append(rutas,
			filepath.Join(d, "mysql", "bin", exeNombre),
			filepath.Join(d, "bin", "mysql", exeNombre),
			filepath.Join(d, "mysql", exeNombre),
		)
	}
	// Instalaciones típicas de Windows, que es donde un ERP de escritorio
	// encuentra el motor sin que nadie configure un PATH.
	rutas = append(rutas,
		`C:\xampp\mysql\bin\`+exeNombre,
		`C:\laragon\bin\mysql`,
	)
	// Laragon no tiene una versión fija: hay que listar el directorio.
	if entradas, err := os.ReadDir(`C:\laragon\bin\mysql`); err == nil {
		var versiones []string
		for _, e := range entradas {
			if e.IsDir() {
				versiones = append(versiones, e.Name())
			}
		}
		// De la versión más alta a la más baja, por nombre: "8.0.30" > "5.7.44".
		for i := 0; i < len(versiones); i++ {
			for j := i + 1; j < len(versiones); j++ {
				if versiones[j] > versiones[i] {
					versiones[i], versiones[j] = versiones[j], versiones[i]
				}
			}
		}
		for _, v := range versiones {
			rutas = append(rutas, filepath.Join(`C:\laragon\bin\mysql`, v, "bin", exeNombre))
		}
	}
	// MariaDB usa otro nombre de binario en Linux.
	for _, nombre := range []string{exeNombre, "mariadbd"} {
		if p, err := exec.LookPath(nombre); err == nil {
			rutas = append(rutas, p)
		}
	}

	for _, r := range rutas {
		if info, err := os.Stat(r); err == nil && !info.IsDir() {
			return r, filepath.Dir(filepath.Dir(r)), nil
		}
	}
	return "", "", fmt.Errorf(
		"no se encontró el motor MySQL/MariaDB en esta máquina (se buscó junto al " +
			"programa, en el PATH, en C:\\xampp\\mysql y en C:\\laragon\\bin\\mysql). " +
			"ERPGo no instala el motor: instálelo como servicio del sistema y apunte " +
			"DB_HOST/DB_PORT a ese servidor")
}

// ultimasLineas devuelve las últimas n líneas no vacías de un texto. Sirve para
// que un error traiga el final del log del motor, que es donde dice qué pasó, en
// vez de las doscientas líneas de arranque.
func ultimasLineas(s string, n int) string {
	lineas := []string{}
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lineas = append(lineas, l)
		}
	}
	if len(lineas) > n {
		lineas = lineas[len(lineas)-n:]
	}
	return strings.Join(lineas, "\n")
}
