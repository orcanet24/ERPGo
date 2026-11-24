package web

// pais_test.go — el registro de países y la regla que protege al usuario.
//
// Lo que se fija aquí NO es estética: es que un país sin pack no se pueda
// ofrecer, que un código desconocido no se convierta en otro país por
// accidente, y que toda empresa —también las instaladas antes de esta
// migración— tenga un perfil válido con moneda, decimales y estado.

import (
	"path/filepath"
	"strings"
	"testing"

	"erpgo/infrastructure/persistence"
)

// TestPaisVenezuelaEsElOrigen: el país de origen está validado, tiene pack y
// es el respaldo de cualquier ruta sin configuración.
func TestPaisVenezuelaEsElOrigen(t *testing.T) {
	ve, ok := paisDeCodigo("VE")
	if !ok {
		t.Fatal("Venezuela debe existir en el registro")
	}
	if ve.Moneda != "VES" || ve.Decimales != 2 {
		t.Errorf("Venezuela debe usar VES con 2 decimales: %+v", ve)
	}
	if ve.Estado != paisValidado || !ve.PackDisponible {
		t.Errorf("Venezuela debe estar validada y con pack: estado=%q pack=%v", ve.Estado, ve.PackDisponible)
	}
	if p, _ := paisDeCodigo(""); p.Codigo != "VE" {
		t.Errorf("sin código debe caer a Venezuela, salió %q", p.Codigo)
	}
}

// TestPaisSinPackNoSeOfrece es la regla que evita vender humo: Colombia y
// Perú están REGISTRADOS (para el plan) pero sin pack todavía, así que no
// pueden aparecer en la lista de países instalables.
func TestPaisSinPackNoSeOfrece(t *testing.T) {
	for _, codigo := range []string{"CO", "PE"} {
		p, ok := paisDeCodigo(codigo)
		if !ok {
			t.Fatalf("%s debe estar registrado para el plan de internacionalización", codigo)
		}
		if p.PackDisponible {
			t.Errorf("%s todavía no tiene pack y no debe declararse disponible", codigo)
		}
	}
	for _, d := range paisesDisponibles() {
		if !d.PackDisponible {
			t.Errorf("paisesDisponibles() devolvió %s sin pack disponible", d.Codigo)
		}
	}
	visibles := map[string]bool{}
	for _, d := range paisesDisponibles() {
		visibles[d.Codigo] = true
	}
	if !visibles["VE"] {
		t.Error("Venezuela debe estar entre los países instalables")
	}
	if visibles["CO"] || visibles["PE"] {
		t.Error("un país sin pack no puede estar entre los instalables")
	}
}

// TestCodigoDePaisDesconocidoNoInventaPais: un código basura no puede
// seleccionar un país distinto por accidente; cae al origen y el llamador
// recibe ok=false para poder avisar.
func TestCodigoDePaisDesconocidoNoInventaPais(t *testing.T) {
	p, ok := paisDeCodigo("XX")
	if ok {
		t.Fatal("XX no debe existir en el registro")
	}
	if p.Codigo != "VE" {
		t.Errorf("un país desconocido debe caer a Venezuela, salió %q", p.Codigo)
	}
	if got := normalizarPaisCodigo(" co "); got != "CO" {
		t.Errorf("normalizar debe limpiar y poner mayúsculas: %q", got)
	}
}

// TestRegistroDePaisesEsCoherente revisa invariantes que a ojo se escapan:
// códigos ISO de dos letras, moneda de tres, nombre y estado presentes.
func TestRegistroDePaisesEsCoherente(t *testing.T) {
	estados := map[estadoPais]bool{paisBorrador: true, paisValidado: true, paisCertificado: true}
	vistos := map[string]bool{}
	for clave, p := range paises {
		if vistos[clave] {
			t.Fatalf("país duplicado: %s", clave)
		}
		vistos[clave] = true
		if p.Codigo != clave {
			t.Errorf("%s: la clave no coincide con el código (%q)", clave, p.Codigo)
		}
		if len(p.Codigo) != 2 || p.Codigo != normalizarPaisCodigo(p.Codigo) {
			t.Errorf("%s: el código debe ser ISO de dos letras en mayúsculas", clave)
		}
		if len(p.Moneda) != 3 || p.Moneda != strings.ToUpper(p.Moneda) {
			t.Errorf("%s: la moneda debe ser ISO de tres letras (%q)", clave, p.Moneda)
		}
		if p.Nombre == "" {
			t.Errorf("%s: falta el nombre", clave)
		}
		if !estados[p.Estado] {
			t.Errorf("%s: estado inválido %q", clave, p.Estado)
		}
		if p.Decimales < 0 || p.Decimales > 4 {
			t.Errorf("%s: decimales fuera de rango (%d)", clave, p.Decimales)
		}
	}
}

// TestPaisDeEmpresaPorDefectoYExplicito usa la cadena real de migraciones
// SQLite: la columna nace con default 'VE' y se puede fijar otro país.
func TestPaisDeEmpresaPorDefectoYExplicito(t *testing.T) {
	db, err := persistence.OpenSQLite(filepath.Join(t.TempDir(), "pais.sqlite3"))
	if err != nil {
		t.Fatalf("abrir base: %v", err)
	}
	defer db.Close()
	if err := persistence.RunMigrationsFor(db); err != nil {
		t.Fatalf("migraciones: %v", err)
	}

	// Sin fila de empresa: perfil de origen, nunca vacío.
	if p := paisDeEmpresa(db); p.Codigo != "VE" {
		t.Errorf("sin empresa debe devolver Venezuela, salió %q", p.Codigo)
	}

	// La columna nace con default 'VE' (instalaciones existentes intactas).
	if _, err := db.Exec(`INSERT INTO company_information (company_id, company_name, email, address, mobile, website, status)
		VALUES ('1','Mi Empresa','a@b.c','Dir','000','',1)`); err != nil {
		t.Fatalf("empresa: %v", err)
	}
	if p := paisDeEmpresa(db); p.Codigo != "VE" {
		t.Errorf("la empresa nueva debe nacer en Venezuela por default, salió %q", p.Codigo)
	}

	// País explícito: se lee el configurado.
	if _, err := db.Exec(`UPDATE company_information SET country_code = 'CO'`); err != nil {
		t.Fatalf("actualizando país: %v", err)
	}
	p := paisDeEmpresa(db)
	if p.Codigo != "CO" || p.Moneda != "COP" {
		t.Errorf("la empresa debía quedar en Colombia/COP: %+v", p)
	}
}
