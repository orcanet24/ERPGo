package configs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DBDriver           string
	DBHost             string
	DBPort             int
	DBUser             string
	DBPassword         string
	DBName             string
	DBMasterTenantCode string
	DBRootUser         string
	DBRootPassword     string
	DBSQLitePath       string
	DBTenantPrefix     string

	ServerPort  string
	ServerHost  string
	GRPCPort    string
	GZipEnabled bool // Compresión gzip en respuestas textuales (activada en producción)

	// Respaldo automático del ERP: activado por defecto ya que el sistema
	// fiscal no debe depender de alguien que apriete un botón.
	BackupEnabled       bool
	BackupIntervalHours int    // BACKUP_INTERVAL_HOURS (default 24)
	BackupDir           string // BACKUP_DIR (default "backup", carpeta junto al binario)
	// Credenciales de provisión: se usan SOLO al crear/eliminar empresas
	// (CREATE DATABASE, CREATE USER, DROP DATABASE). NO para operación diaria.
	// Si está vacío, se usa DBUser/DBPassword para todo.
	DBProvisionUser     string
	DBProvisionPassword string
	// TLS del servidor HTTP. Si TLSCertFile y TLSKeyFile están definidos, el
	// servidor sirve por https (obligatorio para exponer por internet); si no,
	// sirve http plano (normal en LAN/local, donde no hace falta certificado).
	TLSCertFile string
	TLSKeyFile  string
	// MasterKey es la llave maestra que cifra las contraseñas de las bases de
	// datos de cada empresa (AES-256-GCM en internal/security). Se lee de la
	// variable de entorno JWT_SECRET por compatibilidad con instalaciones
	// existentes, pero NO se usa para tokens de sesión: cambiarla inutiliza
	// las credenciales cifradas de todas las empresas y habría que volver a
	// provisionarlas. No tocar a conocer su doble función.
	MasterKey string
	// JWTExpiry se conserva por compatibilidad (antes expiraba un token que ya
	// no se emite; mantener la variable evita romper .env de clientes).
	JWTExpiry  int
	AppName    string
	AppURL     string
	AppEnv     string
	UploadPath string

	// Addon OCR (interfaces/web/ocr_addon.go): motor opcional para extraer
	// datos de facturas de proveedor escaneadas. Desactivado por defecto:
	// el sistema funciona exactamente igual con OCR apagado (o sin Tesseract
	// instalado) porque el ERP no depende de él para nada fiscal.
	OCREnabled   bool   // ENABLE_OCR=1 lo activa (default: 0)
	TesseractBin string // TESSERACT_BIN ruta al binario tesseract (default "tesseract")
	OcrLang      string // OCR_LANG modelo idiográfico de Tesseract (default "spa")

	// Pasarela de envíos YummyProxy2 (interfaces/web/yummy.go): integración
	// con la pasarela externa de Yummy como CLIENTE. La clave real de Yummy
	// vive y muere en la pasarela: el ERP solo lleva su clave del gateway y
	// el secreto con el que la pasarela firma los webhooks que entrega.
	// Desactivado por defecto: tanto si el cliente usa Yummy como si no, la
	// operación diaria del ERP funciona exactamente igual.
	YummyEnabled        bool   // YUMMY_ENABLED=1 lo activa (default: 0)
	YummyGatewayURL     string // YUMMY_GATEWAY_URL base de la pasarela, ej. http://gw:8080
	YummyGatewayKey     string // YUMMY_GATEWAY_KEY la gw_live_… del tenant ERP
	YummyCallbackSecret string // YUMMY_CALLBACK_SECRET firma HMAC de los webhooks de salida

	// Logger configurable
	LogEnabled bool
	LogLevel   string
	LogFile    string
}

type ServerConfig struct {
	Port string
	Host string
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env")

	jwtExpiry, _ := strconv.Atoi(getEnv("JWT_EXPIRY_HOURS", "24"))
	// 3306 es el puerto estándar de MySQL y MariaDB. Antes el valor por
	// defecto era 3307 porque el ERP llevaba su propio motor en ese puerto;
	// al eliminarse ese motor, el defecto vuelve a ser el del servidor real
	// que el cliente debe tener instalado.
	puerto, _ := strconv.Atoi(getEnv("DB_PORT", "3306"))
	backupInterval, _ := strconv.Atoi(getEnv("BACKUP_INTERVAL_HOURS", "24"))

	return &Config{
		DBDriver:            getEnv("DB_DRIVER", "mysql"),
		DBHost:              getEnv("DB_HOST", "127.0.0.1"),
		DBPort:              puerto,
		DBUser:              getEnv("DB_USER", "root"),
		DBPassword:          getEnv("DB_PASSWORD", ""),
		DBName:              getEnv("DB_NAME", "erpgo"),
		DBMasterTenantCode:  getEnv("DB_MASTER_TENANT_CODE", "default"),
		DBRootUser:          getEnv("DB_ROOT_USER", ""),
		DBRootPassword:      getEnv("DB_ROOT_PASSWORD", ""),
		DBTenantPrefix:      getEnv("DB_TENANT_PREFIX", ""),
		DBSQLitePath:        getEnv("DB_SQLITE_PATH", "data/empresas/default.sqlite3"),
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		GRPCPort:            getEnv("GRPC_PORT", "9095"),
		DBProvisionUser:     getEnv("DB_PROVISION_USER", ""),
		DBProvisionPassword: getEnv("DB_PROVISION_PASSWORD", ""),
		// Seguridad: por defecto solo escucha en localhost (127.0.0.1)
		// Para acceso desde red local, configurar SERVER_HOST=0.0.0.0
		ServerHost:  getEnv("SERVER_HOST", "127.0.0.1"),
		TLSCertFile: getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:  getEnv("TLS_KEY_FILE", ""),
		MasterKey:   getMasterKey(),
		JWTExpiry:   jwtExpiry,
		AppName:     getEnv("APP_NAME", "ERPGo"),
		AppURL:      getEnv("APP_URL", "http://localhost:8080"),
		AppEnv:      getEnv("APP_ENV", "development"),
		UploadPath:  getEnv("UPLOAD_PATH", "./uploads"),
		GZipEnabled: EsVerdadero(getEnv("GZIP_ENABLED", "1")), // Logger configurable desde .env
		LogEnabled:  EsVerdadero(getEnv("LOG_ENABLED", "0")),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		LogFile:     getEnv("LOG_FILE", "logs/erpgo.log"),

		// Respaldo automático: activado por defecto, cada día (24h),
		// comprimido (ZIP) en la carpeta backup/ junto al binario.
		BackupEnabled:       EsVerdadero(getEnv("BACKUP_ENABLED", "1")),
		BackupIntervalHours: backupInterval,
		BackupDir:           getEnv("BACKUP_DIR", "backup"),

		// OCR addon: apagado por defecto para que ninguna instalación existente
		// cambie de comportamiento; el superusuario lo enciende con ENABLE_OCR=1.
		OCREnabled:          EsVerdadero(getEnv("ENABLE_OCR", "0")),
		TesseractBin:        getEnv("TESSERACT_BIN", "tesseract"),
		YummyEnabled:        EsVerdadero(getEnv("YUMMY_ENABLED", "0")),
		YummyGatewayURL:     strings.TrimSpace(getEnv("YUMMY_GATEWAY_URL", "")),
		YummyGatewayKey:     strings.TrimSpace(getEnv("YUMMY_GATEWAY_KEY", "")),
		YummyCallbackSecret: strings.TrimSpace(getEnv("YUMMY_CALLBACK_SECRET", "")),
		OcrLang:             getEnv("OCR_LANG", "spa"),
	}, nil
}

func EsVerdadero(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "si", "sí", "on":
		return true
	}
	return false
}

func (c *Config) DataPath() string {
	p := c.DBSQLitePath
	if strings.Contains(p, "empresas"+string(filepath.Separator)) || strings.Contains(p, "empresas/") {
		return filepath.Dir(filepath.Dir(p))
	}
	return filepath.Dir(p)
}

func (c *Config) Server() ServerConfig {
	return ServerConfig{
		Port: c.ServerPort,
		Host: c.ServerHost,
	}
}

func (c *Config) EffectiveRootUser() string {
	if c.DBRootUser != "" {
		return c.DBRootUser
	}
	return c.DBUser
}

func (c *Config) EffectiveRootPassword() string {
	if c.DBRootPassword != "" {
		return c.DBRootPassword
	}
	return c.DBPassword
}

func getMasterKey() string {
	if v, ok := os.LookupEnv("JWT_SECRET"); ok && v != "" {
		return v
	}
	// No hay variable: generar una llave fuerte la primera vez y persistirla.
	// AVISO: esta llave cifra las contraseñas de las bases de datos de cada
	// empresa. Si se pierde o cambia, hay que volver a provisionarlas.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback: basado en PID (mejor que hardcodear)
		return fmt.Sprintf("erpgow-%d-%d", os.Getpid(), os.Getpid()*7919)
	}
	key := hex.EncodeToString(b)
	_ = appendToFile(".env", fmt.Sprintf("\nJWT_SECRET=%s\n", key))
	fmt.Printf("⚠  JWT_SECRET (llave maestra de cifrado) auto-generado. Para producción, configúralo en .env\n")
	return key
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// EnvFileExists reports whether .env exists in the working directory.
func EnvFileExists() bool {
	_, err := os.Stat(".env")
	return err == nil
}

// WriteEnvFile persists the current configuration to .env.
// Called once on first run: the binary creates its own .env from the
// environment variables (set by the installer or the shell) plus defaults.
func (c *Config) WriteEnvFile() error {
	var b strings.Builder
	b.WriteString("# ERPGo - auto-generated on first run\n")
	b.WriteString("# Generated by erpgo.exe - safe to edit\n\n")

	b.WriteString("# --- MySQL ---\n")
	b.WriteString(fmt.Sprintf("DB_DRIVER=%s\n", c.DBDriver))
	b.WriteString(fmt.Sprintf("DB_HOST=%s\n", c.DBHost))
	b.WriteString(fmt.Sprintf("DB_PORT=%d\n", c.DBPort))
	b.WriteString(fmt.Sprintf("DB_USER=%s\n", c.DBUser))
	b.WriteString(fmt.Sprintf("DB_PASSWORD=%s\n", c.DBPassword))
	b.WriteString(fmt.Sprintf("DB_NAME=%s\n", c.DBName))
	b.WriteString(fmt.Sprintf("DB_MASTER_TENANT_CODE=%s\n", c.DBMasterTenantCode))
	b.WriteString(fmt.Sprintf("DB_ROOT_USER=%s\n", c.DBRootUser))
	b.WriteString(fmt.Sprintf("DB_ROOT_PASSWORD=%s\n", c.DBRootPassword))
	b.WriteString(fmt.Sprintf("DB_TENANT_PREFIX=%s\n", c.DBTenantPrefix))
	b.WriteString(fmt.Sprintf("DB_SQLITE_PATH=%s\n", c.DBSQLitePath))

	b.WriteString("# --- Web Server ---\n")
	b.WriteString(fmt.Sprintf("SERVER_HOST=%s\n", c.ServerHost))
	b.WriteString(fmt.Sprintf("SERVER_PORT=%s\n\n", c.ServerPort))
	b.WriteString("# Puerto gRPC para terminales POS (cajas).\n")
	b.WriteString(fmt.Sprintf("GRPC_PORT=%s\n\n", c.GRPCPort))
	b.WriteString("# Credenciales de provisión (solo para crear/eliminar empresas).\n")
	b.WriteString(fmt.Sprintf("DB_PROVISION_USER=%s\n", c.DBProvisionUser))
	b.WriteString(fmt.Sprintf("DB_PROVISION_PASSWORD=%s\n\n", c.DBProvisionPassword))

	// JWT_SECRET es la llave maestra de cifrado (ver MasterKey): no renombrar
	// la variable para no romper instalaciones existentes.
	b.WriteString("# --- Llave maestra de cifrado (antes JWT_SECRET) ---\n")
	b.WriteString(fmt.Sprintf("JWT_SECRET=%s\n", c.MasterKey))
	b.WriteString(fmt.Sprintf("JWT_EXPIRY_HOURS=%d\n\n", c.JWTExpiry))

	b.WriteString("# --- App ---\n")
	b.WriteString(fmt.Sprintf("APP_NAME=%s\n", c.AppName))
	b.WriteString(fmt.Sprintf("APP_URL=%s\n", c.AppURL))
	b.WriteString(fmt.Sprintf("APP_ENV=%s\n", c.AppEnv))
	b.WriteString(fmt.Sprintf("UPLOAD_PATH=%s\n", c.UploadPath))

	b.WriteString("\n# --- Logger ---\n")
	if c.LogEnabled {
		b.WriteString("LOG_ENABLED=1\n")
	} else {
		b.WriteString("LOG_ENABLED=0\n")
	}
	b.WriteString(fmt.Sprintf("LOG_LEVEL=%s\n", c.LogLevel))
	b.WriteString(fmt.Sprintf("LOG_FILE=%s\n", c.LogFile))

	if c.TLSCertFile != "" {
		b.WriteString(fmt.Sprintf("\n# --- TLS ---\n"))
		b.WriteString(fmt.Sprintf("TLS_CERT_FILE=%s\n", c.TLSCertFile))
		b.WriteString(fmt.Sprintf("TLS_KEY_FILE=%s\n", c.TLSKeyFile))
	}

	return os.WriteFile(".env", []byte(b.String()), 0600)
}

// EnsureEnv loads .env if present, then generates it if missing.
// Returns the final config (after .env load + auto-generation).
func EnsureEnv() (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	if !EnvFileExists() {
		fmt.Println("⚙  .env no encontrado — generando automáticamente...")
		if err := cfg.WriteEnvFile(); err != nil {
			return nil, fmt.Errorf("no se pudo crear .env: %w", err)
		}
		fmt.Println("✅ .env generado. Revise el archivo si necesita ajustes.")
		// Reload to pick up the now-persisted values
		cfg, err = Load()
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
