// Respaldo de bases de datos antes de operaciones destructivas.
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"erpgo/infrastructure/persistence"
)

// BackupManager maneja los respaldos del sistema.
type BackupManager struct {
	backupDir string
	host      string
	port      int
	user      string
	password  string
}

// NewBackupManager crea un manejador de respaldos.
func NewBackupManager(host string, port int, user, password, backupDir string) *BackupManager {
	return &BackupManager{
		host:      host,
		port:      port,
		user:      user,
		password:  password,
		backupDir: backupDir,
	}
}

// BackupAll respalda todas las bases de datos del sistema.
func (bm *BackupManager) BackupAll() (string, error) {
	// Crear directorio de respaldo
	if err := os.MkdirAll(bm.backupDir, 0755); err != nil {
		return "", fmt.Errorf("creando directorio de backup: %w", err)
	}

	// Conectar al servidor para listar bases
	cfg := persistence.ConfigMySQL{
		Host:    bm.host,
		Port:    bm.port,
		Usuario: bm.user,
		Clave:   bm.password,
	}
	db, err := persistence.ConectarServidorMySQL(cfg)
	if err != nil {
		return "", fmt.Errorf("conectando a MySQL: %w", err)
	}
	defer db.Close()

	// Listar bases que nos interesan (erpgo y erpgo_*)
	rows, err := db.Query("SHOW DATABASES LIKE 'erpgo%'")
	if err != nil {
		return "", fmt.Errorf("listando bases: %w", err)
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		databases = append(databases, name)
	}

	if len(databases) == 0 {
		return "", nil
	}

	// Nombre del archivo de backup
	stamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(bm.backupDir, fmt.Sprintf("backup_all_%s.sql", stamp))

	// Ejecutar mysqldump
	if err := bm.dumpAll(databases, backupFile); err != nil {
		return "", err
	}

	return backupFile, nil
}

// BackupDatabase respalda una sola base de datos.
func (bm *BackupManager) BackupDatabase(dbName string) (string, error) {
	if err := os.MkdirAll(bm.backupDir, 0755); err != nil {
		return "", err
	}

	stamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(bm.backupDir, fmt.Sprintf("backup_%s_%s.sql", dbName, stamp))

	if err := bm.dumpDatabase(dbName, backupFile); err != nil {
		return "", err
	}

	return backupFile, nil
}

// dumpAll ejecuta mysqldump para múltiples bases.
func (bm *BackupManager) dumpAll(databases []string, output string) error {
	args := []string{
		"--host", bm.host,
		"--port", fmt.Sprintf("%d", bm.port),
		"--user", bm.user,
		"--password=" + bm.password,
		"--single-transaction",
		"--routines",
		"--triggers",
		"--databases",
	}
	args = append(args, databases...)

	cmd := exec.Command(bm.findDumpBinary(), args...)
	
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysqldump falló: %v\n%s", err, string(out))
	}

	return os.WriteFile(output, out, 0600)
}

// dumpDatabase ejecuta mysqldump para una sola base.
func (bm *BackupManager) dumpDatabase(dbName, output string) error {
	args := []string{
		"--host", bm.host,
		"--port", fmt.Sprintf("%d", bm.port),
		"--user", bm.user,
		"--password=" + bm.password,
		"--single-transaction",
		"--routines",
		"--triggers",
		dbName,
	}

	cmd := exec.Command(bm.findDumpBinary(), args...)
	
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysqldump falló: %v\n%s", err, string(out))
	}

	return os.WriteFile(output, out, 0600)
}

// findDumpBinary busca el binario mysqldump según el sistema operativo.
func (bm *BackupManager) findDumpBinary() string {
	dumpBin := "mysqldump"
	if runtime.GOOS == "windows" {
		dumpBin = "mysqldump.exe"
	}

	// Buscar en PATH
	if path, err := exec.LookPath(dumpBin); err == nil {
		return path
	}

	// Buscar en ubicaciones comunes según el OS
	commonPaths := bm.commonDumpPaths()
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Retornar al menos el nombre base
	return dumpBin
}

// commonDumpPaths devuelve rutas comunes para mysqldump según el OS.
func (bm *BackupManager) commonDumpPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files\MySQL\MySQL Server 8.0\bin\mysqldump.exe`,
			`C:\laragon\bin\mysql\mysql-8.0.30-winx64\bin\mysqldump.exe`,
			`C:\xampp\mysql\bin\mysqldump.exe`,
		}
	default:
		return []string{
			"/usr/bin/mysqldump",
			"/usr/local/mysql/bin/mysqldump",
			"/opt/lampp/bin/mysqldump",
		}
	}
}

// ResetResult describe el resultado de un reset.
type ResetResult struct {
	BackupFile  string
	Databases   []string
	TablesReset int
}

// ResetDatabase resetea una base de datos (DROP + CREATE).
func (bm *BackupManager) ResetDatabase(dbName string) error {
	cfg := persistence.ConfigMySQL{
		Host:    bm.host,
		Port:    bm.port,
		Usuario: bm.user,
		Clave:   bm.password,
	}
	db, err := persistence.ConectarServidorMySQL(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	// Dropear y recrear
	if _, err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)); err != nil {
		return fmt.Errorf("drop %s: %w", dbName, err)
	}
	if _, err := db.Exec(fmt.Sprintf(
		"CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName,
	)); err != nil {
		return fmt.Errorf("create %s: %w", dbName, err)
	}

	return nil
}

