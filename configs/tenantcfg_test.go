package configs

import (
	"strings"
	"testing"

	"erpgo/internal/security"
)

// El ciclo entero de una empresa: guardar (la clave llega en claro y queda
// CIFRADA) → cargar → los datos vuelven. Es el contrato mínimo: si esto se
// rompe, N empresas pierden su conexión sin que el catálogo lo advierta.
func TestTenantConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	old := TenantConfigDir
	TenantConfigDir = dir
	defer func() { TenantConfigDir = old }()

	const jwt = "secreto-de-prueba-suficientemente-largo"
	err := SaveTenantConfigFile(TenantConn{
		Code:   "tecnotextil",
		Kind:   KindEmpresa,
		Host:   "127.0.0.1",
		Port:   3306,
		DBName: "erpgo_tecnotextil",
		DBUser: "user_hosting",
	}, "clave-secreta", jwt)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	conn, err := LoadTenantConfigFile("tecnotextil")
	if err != nil {
		t.Fatalf("cargar: %v", err)
	}
	if conn == nil {
		t.Fatal("el archivo debería existir")
	}
	if conn.DBName != "erpgo_tecnotextil" || conn.DBUser != "user_hosting" || conn.Port != 3306 || conn.Kind != KindEmpresa {
		t.Fatalf("datos no restituidos: %+v", conn)
	}
	// La clave NUNCA debe durar en claro ni ser legible: va cifrada con el
	// secreto, y el Write no escribe PasswordMan (ese es de emergencia).
	if conn.PasswordEnc == "" {
		t.Fatal("la clave debería estar cifrada, no vacía")
	}
	if strings.Contains(conn.PasswordEnc, "clave-secreta") {
		t.Fatal("la clave en claro se filtró al archivo")
	}

	// La clave correcta se descifra; una incorrecta fracasa.
	plain, derr := security.Decrypt(conn.PasswordEnc, security.DeriveKey(jwt))
	if derr != nil || plain != "clave-secreta" {
		t.Fatalf("descifrado de la clave falló: %q %v", plain, derr)
	}
}

// Empresa sin archivo: NO es error, es "no hay archivo, sigue el catálogo".
func TestTenantConfigNoExiste(t *testing.T) {
	dir := t.TempDir()
	old := TenantConfigDir
	TenantConfigDir = dir
	defer func() { TenantConfigDir = old }()

	conn, err := LoadTenantConfigFile("empresa_inexistente")
	if err != nil {
		t.Fatalf("la ausencia no debe ser error: %v", err)
	}
	if conn != nil {
		t.Fatal("debería devolver nil sin archivo")
	}
}

