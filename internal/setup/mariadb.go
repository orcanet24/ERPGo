// Instalación de MariaDB para Windows y Linux.
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MariaDBInstaller maneja la descarga e instalación de MariaDB.
type MariaDBInstaller struct {
	InstallDir string
	Port       int
	RootPass   string
}

// NewMariaDBInstaller crea un instalador.
func NewMariaDBInstaller(installDir string, port int, rootPass string) *MariaDBInstaller {
	return &MariaDBInstaller{
		InstallDir: installDir,
		Port:       port,
		RootPass:   rootPass,
	}
}

// IsMariaDBInstalled verifica si MariaDB ya está instalado.
func (mi *MariaDBInstaller) IsMariaDBInstalled() bool {
	paths := mi.commonBinPaths()
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Install descarga e instalación MariaDB.
func (mi *MariaDBInstaller) Install() error {
	if mi.IsMariaDBInstalled() {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		return mi.installWindows()
	case "linux":
		return mi.installLinux()
	default:
		return fmt.Errorf("sistema operativo no soportado: %s", runtime.GOOS)
	}
}

// installWindows instala MariaDB en Windows.
func (mi *MariaDBInstaller) installWindows() error {
	// Crear directorio de instalación
	if err := os.MkdirAll(mi.InstallDir, 0755); err != nil {
		return fmt.Errorf("creando directorio: %w", err)
	}

	// Descargar instalador MSI de MariaDB
	downloadURL := "https://downloads.mariadb.com/MariaDB/mariadb-11.4.2/winx64-packages/mariadb-11.4.2-winx64.msi"
	msiPath := filepath.Join(mi.InstallDir, "mariadb.msi")

	if err := mi.downloadFile(downloadURL, msiPath); err != nil {
		return fmt.Errorf("descargando MariaDB: %w", err)
	}

	// Ejecutar instalación silenciosa
	cmd := exec.Command("msiexec", "/i", msiPath, "/qn", "/norestart",
		fmt.Sprintf("INSTALLDIR=%s", mi.InstallDir),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("instalación fallida: %v\n%s", err, string(out))
	}

	// Inicializar datos
	initCmd := exec.Command(
		filepath.Join(mi.InstallDir, "bin", "mariadb-install-db.exe"),
		"--datadir="+filepath.Join(mi.InstallDir, "data"),
	)
	if out, err := initCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("inicializando datos: %v\n%s", err, string(out))
	}

	return nil
}

// installLinux instala MariaDB en Linux.
func (mi *MariaDBInstaller) installLinux() error {
	// Detectar distribución
	pkgManager := mi.detectPackageManager()

	var cmd *exec.Cmd
	switch pkgManager {
	case "apt":
		// Debian/Ubuntu
		cmd = exec.Command("sh", "-c",
			"apt-get update && apt-get install -y mariadb-server mariadb-client",
		)
	case "yum":
		// CentOS/RHEL 7
		cmd = exec.Command("sh", "-c", "yum install -y mariadb-server mariadb")
	case "dnf":
		// CentOS/RHEL 8+, Fedora
		cmd = exec.Command("sh", "-c", "dnf install -y mariadb-server mariadb")
	default:
		return fmt.Errorf("gestor de paquetes no soportado")
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("instalando MariaDB: %v\n%s", err, string(out))
	}

	// Iniciar servicio
	startCmd := exec.Command("systemctl", "start", "mariadb")
	startCmd.Run()

	enableCmd := exec.Command("systemctl", "enable", "mariadb")
	enableCmd.Run()

	return nil
}

// StartService inicia el servicio MariaDB.
func (mi *MariaDBInstaller) StartService() error {
	switch runtime.GOOS {
	case "windows":
		// Usar el servicio de Windows de MariaDB
		cmd := exec.Command("net", "start", "MariaDB")
		if out, err := cmd.CombinedOutput(); err != nil {
			// Si ya está corriendo, no es error
			if !strings.Contains(string(out), "already been started") {
				return fmt.Errorf("iniciando MariaDB: %v\n%s", err, string(out))
			}
		}
	case "linux":
		cmd := exec.Command("systemctl", "start", "mariadb")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("iniciando MariaDB: %v\n%s", err, string(out))
		}
	}
	return nil
}

// CreateService crea un servicio con nombre personalizado.
func (mi *MariaDBInstaller) CreateService(serviceName string) error {
	switch runtime.GOOS {
	case "windows":
		mysqldPath := filepath.Join(mi.InstallDir, "bin", "mysqld.exe")
		cmd := exec.Command("sc", "create", serviceName,
			"binPath=", fmt.Sprintf(`"%s" --defaults-file="%s" --port=%d`,
				mysqldPath, filepath.Join(mi.InstallDir, "my.ini"), mi.Port),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("creando servicio: %v\n%s", err, string(out))
		}
	case "linux":
		// Ya existe como mariadb.service
	}
	return nil
}

// SetRootPassword establece la contraseña root.
func (mi *MariaDBInstaller) SetRootPassword() error {
	cmd := exec.Command(
		mi.findMySQLBinary(),
		"-u", "root",
		fmt.Sprintf("-p%s", mi.RootPass),
		"-e", fmt.Sprintf("ALTER USER 'root'@'localhost' IDENTIFIED BY '%s'; FLUSH PRIVILEGES;", mi.RootPass),
	)
	_, err := cmd.CombinedOutput()
	return err
}

// detectPackageManager detecta el gestor de paquetes.
func (mi *MariaDBInstaller) detectPackageManager() string {
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		return "apt"
	}
	if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		return "dnf"
	}
	if _, err := os.Stat("/usr/bin/yum"); err == nil {
		return "yum"
	}
	return ""
}

// commonBinPaths devuelve las rutas comunes de mysqld.
func (mi *MariaDBInstaller) commonBinPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files\MariaDB 11.4\bin\mysqld.exe`,
			`C:\mariadb\bin\mysqld.exe`,
		}
	default:
		return []string{
			"/usr/sbin/mysqld",
			"/usr/bin/mysqld",
		}
	}
}

// findMySQLBinary busca el binario mysql client.
func (mi *MariaDBInstaller) findMySQLBinary() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(mi.InstallDir, "bin", "mysql.exe")
	default:
		return "/usr/bin/mysql"
	}
}

// downloadFile descarga una URL a un archivo local.
func (mi *MariaDBInstaller) downloadFile(url, dest string) error {
	// En producción usar http.Get + io.Copy
	// Por ahora, retornar error indicando descarga manual
	return fmt.Errorf("descarga automática no implementada aún. Descargue MariaDB manualmente de: %s", url)
}
