package web

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"erpgow/infrastructure/persistence"
	"erpgow/internal/security"
)

// TenantCredentials contiene las credenciales de BD de una empresa específica.
// La contraseña vive encriptada en el catálogo; solo se desencripta en memoria.
type TenantCredentials struct {
	Code            string
	DBUser          string
	DBPasswordEnc   []byte // AES-256-GCM encriptado
	DBPasswordPlain string // desencriptado, solo en memorio, nunca persistido
	Status          int
	MaxUsers        int
	// DBName vive aquí porque en hosting compartido el panel obliga a nombres
	// con prefijo de la cuenta (orcanet24_erpgo_...), que no derivan del código.
	// Vacío = empresa antigua: se deriva erpgo_<código> como siempre.
	DBName string
}

// generateSecurePassword genera una contraseña aleatoria segura de la longitud dada.
func generateSecurePassword(length int) string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// provisionCredentials genera credenciales únicas para una empresa y las devuelve.
// La contraseña está en texto plano para el caller (la usa para crear el usuario
// MySQL y para la siembra), y encriptada para guardar en el catálogo.
func (s *Server) provisionCredentials(code string) (dbUser, dbPassPlain string, dbPassEnc string, err error) {
	// Usuario MySQL seguro: prefijo + codigo de empresa sanitizado
	dbUser = "erpgo_" + strings.ToLower(strings.ReplaceAll(code, "-", "_"))
	if len(dbUser) > 32 {
		dbUser = dbUser[:32]
	}
	
	dbPassPlain = generateSecurePassword(24)
	
	// Encriptar contraseña para guardar en catálogo
	key := security.DeriveKey(s.config.JWTSecret)
	dbPassEnc, err = security.Encrypt(dbPassPlain, key)
	if err != nil {
		return "", "", "", fmt.Errorf("no se pudo encriptar la contraseña de la empresa: %w", err)
	}
	
	return dbUser, dbPassPlain, dbPassEnc, nil
}

// createMySQLUser crea un usuario MySQL con permisos solo en la base de la empresa.
func (s *Server) createMySQLUser(dbUser, dbPass, dbName string) error {
	cfg := persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: s.config.DBProvisionUser,
		Clave:   s.config.DBPassword,
	}
	
	// Fallback: si no hay usuario de provisión, usar el usuario principal
	if cfg.Usuario == "" {
		cfg.Usuario = s.config.DBUser
		cfg.Clave = s.config.DBPassword
	}
	
	db, err := persistence.ConectarServidorMySQL(cfg)
	if err != nil {
		return fmt.Errorf("no se pudo conectar al servidor MySQL para crear usuario: %w", err)
	}
	defer db.Close()
	
	// Crear usuario
	_, err = db.Exec(fmt.Sprintf(
		"CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'",
		dbUser, dbPass,
	))
	if err != nil {
		return fmt.Errorf("no se pudo crear el usuario MySQL: %w", err)
	}
	
	// Dar permisos SOLO en la base de la empresa
	_, err = db.Exec(fmt.Sprintf(
		"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, ALTER, INDEX, "+
			"CREATE VIEW, SHOW VIEW, TRIGGER, REFERENCES "+
			"ON `%s`.* TO '%s'@'%%'",
		dbName, dbUser,
	))
	if err != nil {
		return fmt.Errorf("no se pudo dar permisos al usuario: %w", err)
	}
	
	_, err = db.Exec("FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("no se pudieron aplicar los privilegios: %w", err)
	}
	
	return nil
}

// saveTenantCredentials guarda las credenciales encriptadas en el catálogo.
func (s *Server) saveTenantCredentials(code, dbUser, dbPassEnc string) error {
	ctl, err := s.controlStore()
	if err != nil {
		return err
	}
	
	_, err = ctl.Exec(
		"UPDATE tenants SET db_user=?, db_password_enc=?, created_at=? WHERE code=?",
		dbUser, []byte(dbPassEnc), time.Now().Format("2006-01-02 15:04:05"), code,
	)
	return err
}

// getTenantCredentials recupera las credenciales de una empresa del catálogo.
// Devuelve nil si no tiene credenciales propias (empresa antigua, usa credenciales globales).
func (s *Server) getTenantCredentials(code string) *TenantCredentials {
	ctl, err := s.controlStore()
	if err != nil {
		return nil
	}
	
	var tc TenantCredentials
	var passEnc []byte
	err = ctl.QueryRow(
		"SELECT code, db_user, db_password_enc, status, max_users, db_name FROM tenants WHERE code=?",
		code,
	).Scan(&tc.Code, &tc.DBUser, &passEnc, &tc.Status, &tc.MaxUsers, &tc.DBName)
	
	if err != nil {
		return nil
	}
	
	// Si no tiene usuario propio, es una empresa antigua sin credenciales por empresa
	if tc.DBUser == "" {
		return nil
	}
	
	tc.DBPasswordEnc = passEnc
	
	// Desencriptar contraseña
	key := security.DeriveKey(s.config.JWTSecret)
	tc.DBPasswordPlain, err = security.Decrypt(string(passEnc), key)
	if err != nil {
		return nil // no desencriptar = no conectar (fail closed)
	}
	
	return &tc
}

// ProvisionTenantExtended extiende ProvisionTenant con credenciales por empresa.
// SOLO llama a ProvisionTenant para crear la BD, luego añade credenciales propias.
func (s *Server) ProvisionTenantExtended(legalName, rif, adminUser, adminPass string) (ProvisionTenantResult, error) {
	// PASO 1: Crear la empresa con el método original (BD + migraciones + siembra)
	res, err := s.ProvisionTenant(legalName, rif, adminUser, adminPass)
	if err != nil {
		return res, err
	}
	
	// Si ya existía, no regeneramos credenciales
	if !res.Created {
		return res, nil
	}
	
	// PASO 2: Generar credenciales únicas
	dbUser, dbPassPlain, dbPassEnc, err := s.provisionCredentials(res.Code)
	if err != nil {
		return res, fmt.Errorf("empresa creada pero no se pudieron generar credenciales: %w", err)
	}
	
	// PASO 3: Crear usuario MySQL dedicado
	err = s.createMySQLUser(dbUser, dbPassPlain, res.Path)
	if err != nil {
		return res, fmt.Errorf("empresa creada pero no se pudo crear usuario MySQL: %w", err)
	}
	
	// PASO 4: Guardar credenciales encriptadas en catálogo
	err = s.saveTenantCredentials(res.Code, dbUser, dbPassEnc)
	if err != nil {
		return res, fmt.Errorf("empresa creada pero no se pudieron guardar credenciales: %w", err)
	}
	
	return res, nil
}
