package configs

// tenantcfg.go — archivo de conexión POR EMPRESA: config/empresas/<código>.env
//
// POR QUÉ UN ARCHIVO Y NO SOLO EL CATÁLOGO DE LA BD MAESTRA
//
// Las credenciales de BD de cada empresa ya viven encriptadas en el catálogo
// (tenants.db_user / db_password_enc). El archivo por empresa es la SEGUNDA
// pata, por orden expreso del dueño del sistema:
//
//   - El administrador puede ver y corregir la conexión de una empresa sin
//     tocar la base maestra (por ejemplo, para mover una empresa a otro
//     servidor, o recuperar el acceso si el catálogo falla).
//   - Cada archivo dice QUÉ empresa es (su nombre es el código) y QUÉ es
//     (ERP_KIND: empresa / demo): nada de adivinar en un .env global crecido.
//   - El .env queda SOLO para lo global: servidor, BD maestra, JWT, backups.
//     Por cada empresa nueva NO crece una línea más del .env: su archivo
//     propio es su lugar.
//
// SEGURIDAD: la contraseña NUNCA va en claro cuando la escribe el sistema:
// se guarda cifrada (AES-256-GCM, clave derivada del secreto JWT), el mismo
// mecanismo del catálogo. Se admite también DB_PASSWORD en claro SOLO para
// una emergencia manual (el técnico escribe el archivo a mano).
//
// PRECEDENCIA en el runtime (openTenantDB): archivo por empresa → credenciales
// de la empresa en el catálogo → credenciales globales del .env.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"erpgow/internal/security"
)

// TenantConfigDir es la carpeta de archivos de conexión por empresa.
// Es var (no const) por las pruebas, que la apuntan a un directorio temporal:
// en operación nadie la escribe.
var TenantConfigDir = "config/empresas"

// Kinds permitidos en ERP_KIND.
const (
	KindEmpresa = "empresa"
	KindDemo    = "demo"
)

// TenantConn es la conexión de una empresa, como vive en disco.
type TenantConn struct {
	Code string // código del catálogo; también el nombre del archivo
	Kind string // demo | empresa

	Host        string
	Port        int
	DBName      string
	DBUser      string
	PasswordEnc string // AES-256-GCM base64 — lo normal, escrito por el sistema
	PasswordMan string // en claro SOLO si el admin escribió el archivo a mano
}

// TenantConfigFile devuelve la ruta del archivo de una empresa.
//
// El nombre se sanea con el mismo criterio de las bases (solo ascii y guion
// bajo) para que un código con caracteres raros no produzca una ruta con
// puntos ni barras: es un archivo que el sistema lee y escribe por nombre.
func TenantConfigFile(code string) (string, error) {
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		return "", fmt.Errorf("código de empresa vacío")
	}
	for _, r := range code {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return "", fmt.Errorf("código de empresa no válido para archivo: %q", code)
		}
	}
	return filepath.Join(TenantConfigDir, code+".env"), nil
}

// SaveTenantConfigFile escribe (o reescribe) el archivo de conexión de una
// empresa. Permisos 0600: solo el proceso y el admin del servidor deben leer
// esto, como el .env.
//
// La contraseña llega en claro (passwordPlain) y SE CIFRA aquí con el secreto
// (jwtSecret); o se acepta TenantConn.PasswordEnc ya cifrada si el llamador
// la tiene. PasswordMan no se escribe aquí: el en claro lo escribe el técnico
// a mano cuando hace una emergencia, no el software.
//
// jwtSecret vacío con passwordPlain != "" es un error explícito: mejor fallar
// ahora que escribir una clave cifrada con derivación vacía que el runtime
// jamás podría descifrar.
func SaveTenantConfigFile(c TenantConn, passwordPlain, jwtSecret string) error {
	path, err := TenantConfigFile(c.Code)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creando %s: %w", TenantConfigDir, err)
	}

	enc := c.PasswordEnc
	if passwordPlain != "" {
		if jwtSecret == "" {
			return fmt.Errorf("secreto vacío: no se puede cifrar la clave de la empresa %q", c.Code)
		}
		if enc, err = security.Encrypt(passwordPlain, security.DeriveKey(jwtSecret)); err != nil {
			return fmt.Errorf("cifrando clave de la empresa %q: %w", c.Code, err)
		}
	}

	var b strings.Builder
	b.WriteString("# ERPGo — conexión de la empresa " + c.Code + "\n")
	b.WriteString("# Escrito por ERPGo. La clave va CIFRADA (AES-256-GCM): editar este\n")
	b.WriteString("# archivo no sirve para cambiarla; para cambiarla use --setup opción 4,\n")
	b.WriteString("# o en una emergencia escriba DB_PASSWORD en claro (solo manual).\n")
	b.WriteString(fmt.Sprintf("ERP_KIND=%s\n", c.Kind))
	if enc != "" {
		b.WriteString(fmt.Sprintf("DB_PASSWORD_ENC=%s\n", enc))
	}
	if c.PasswordMan != "" {
		b.WriteString(fmt.Sprintf("DB_PASSWORD=%s\n", c.PasswordMan))
	}
	b.WriteString(fmt.Sprintf("DB_HOST=%s\n", c.Host))
	b.WriteString(fmt.Sprintf("DB_PORT=%d\n", c.Port))
	b.WriteString(fmt.Sprintf("DB_NAME=%s\n", c.DBName))
	b.WriteString(fmt.Sprintf("DB_USER=%s\n", c.DBUser))

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// LoadTenantConfigFile lee el archivo de conexión de una empresa.
// Devuelve (nil, nil) si NO existe — la ausencia no es error, es "esta
// empresa no tiene archivo; sigue con el catálogo".
func LoadTenantConfigFile(code string) (*TenantConn, error) {
	path, err := TenantConfigFile(code)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("consultando %s: %w", path, statErr)
	}

	vals, err := godotenv.Read(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", path, err)
	}

	conn := &TenantConn{
		Code:        code,
		Kind:        strings.TrimSpace(vals["ERP_KIND"]),
		Host:        strings.TrimSpace(vals["DB_HOST"]),
		DBName:      strings.TrimSpace(vals["DB_NAME"]),
		DBUser:      strings.TrimSpace(vals["DB_USER"]),
		PasswordEnc: strings.TrimSpace(vals["DB_PASSWORD_ENC"]),
		PasswordMan: strings.TrimSpace(vals["DB_PASSWORD"]),
	}
	if port := strings.TrimSpace(vals["DB_PORT"]); port != "" {
		if p, perr := strconv.Atoi(port); perr == nil {
			conn.Port = p
		}
	}
	if conn.Kind == "" {
		conn.Kind = KindEmpresa
	}
	return conn, nil
}
