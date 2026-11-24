package web

// pack_pais_test.go — la ficha del país no puede mentir.
//
// Tres reglas se prueban aquí, y ninguna es cosmética:
//  1. Todo país del registro tiene su ficha (y la ficha coincide con el código).
//  2. Un pack «borrador» declara qué le falta; «validado» quién lo firmó;
//     «certificado» en qué ambiente se probó.
//  3. Lo que se OFRECE (PackDisponible) coincide con lo que se puede sostener:
//     solo se ofrece lo que ya no está en borrador.

import (
	"io/fs"
	"strings"
	"testing"

	"erpgo"
)

func TestTodasLasFichasDePaisSonValidas(t *testing.T) {
	packs, err := cargarPacksPais()
	if err != nil {
		t.Fatalf("cargando packs: %v", err)
	}
	if len(packs) == 0 {
		t.Fatal("no se encontró ninguna ficha de país en packs/")
	}
	for codigo, p := range packs {
		if err := validarPackPais(p); err != nil {
			t.Errorf("ficha inválida: %v", err)
		}
		if _, ok := paises[codigo]; !ok {
			t.Errorf("%s tiene ficha en packs/ pero no está en el registro (pais.go)", codigo)
		}
	}
}

func TestTodoPaisDelRegistroTieneFicha(t *testing.T) {
	packs, err := cargarPacksPais()
	if err != nil {
		t.Fatalf("cargando packs: %v", err)
	}
	for codigo := range paises {
		if _, ok := packs[codigo]; !ok {
			t.Errorf("%s está en el registro pero no tiene packs/%s/pack.json", codigo, codigoLower(codigo))
		}
	}
}

// TestVenezuelaEsElPackLegado: el país de origen vive embebido en las
// migraciones históricas, está validado y eso se declara explícitamente.
func TestVenezuelaEsElPackLegado(t *testing.T) {
	packs, err := cargarPacksPais()
	if err != nil {
		t.Fatalf("cargando packs: %v", err)
	}
	ve, ok := packs["VE"]
	if !ok {
		t.Fatal("falta packs/ve/pack.json")
	}
	if !ve.LegacyEmbebido {
		t.Error("Venezuela debe declarar legacy_embebido=true: su pack está en las migraciones")
	}
	if ve.Estado != paisValidado {
		t.Errorf("Venezuela debe estar validada, está %q", ve.Estado)
	}
	if ve.ValidadoPor == "" {
		t.Error("Venezuela debe declarar quién revisó el pack")
	}
}

// TestPacksEnBorradorNoSeOfrecen es la regla que protege al cliente: el
// registro solo puede ofrecer un país cuyo pack ya dejó de ser borrador, y la
// ficha del país en borrador tiene que listar sus deudas.
func TestPacksEnBorradorNoSeOfrecen(t *testing.T) {
	packs, err := cargarPacksPais()
	if err != nil {
		t.Fatalf("cargando packs: %v", err)
	}
	for codigo, p := range packs {
		perfil, ok := paises[codigo]
		if !ok {
			continue
		}
		if p.Estado == paisBorrador {
			if perfil.PackDisponible {
				t.Errorf("%s está en borrador pero el registro lo ofrece como disponible", codigo)
			}
			if len(p.Pendientes) == 0 {
				t.Errorf("%s está en borrador y no declara pendientes", codigo)
			}
			continue
		}
		if !perfil.PackDisponible {
			t.Errorf("%s está %s pero el registro no lo ofrece", codigo, p.Estado)
		}
	}
}

// codigoLower evita depender de strings en la prueba y hace obvia la
// convención de carpeta (packs/<cc>/ con ISO en minúsculas).
func codigoLower(codigo string) string {
	salida := make([]byte, 0, len(codigo))
	for i := 0; i < len(codigo); i++ {
		c := codigo[i]
		if c >= 'A' && c <= 'Z' {
			c = c - 'A' + 'a'
		}
		salida = append(salida, c)
	}
	return string(salida)
}

// TestPackValidadoExigeSemillas: un pack no puede declararse «validado» sin
// datos aplicables. Sin semillas no hay país, solo una ficha bonita.
func TestPackValidadoExigeSemillas(t *testing.T) {
	caso := packPais{Codigo: "CO", Nombre: "Colombia", Moneda: "COP", Estado: paisValidado, ValidadoPor: "contador"}
	if err := validarPackPais(caso); err == nil {
		t.Error("un pack validado con seed_version 0 debe rechazarse")
	}
	caso.SeedVersion = 1
	if err := validarPackPais(caso); err != nil {
		t.Errorf("con semillas declaradas no debía fallar: %v", err)
	}
}

// TestPackEnBorradorNoAplicaNada: el seed de un país en borrador es una lista
// de trabajo, no código. Si alguien «activa» una sentencia por accidente, esta
// prueba lo delata antes de que se facture con datos a medias.
func TestPackEnBorradorNoAplicaNada(t *testing.T) {
	packs, err := cargarPacksPais()
	if err != nil {
		t.Fatalf("cargando packs: %v", err)
	}
	raiz := erpgo.Packs()
	for codigo, p := range packs {
		if p.LegacyEmbebido || p.Estado != paisBorrador {
			continue
		}
		dir := "packs/" + codigoLower(codigo) + "/seed.sql"
		data, err := fs.ReadFile(raiz, dir)
		if err != nil {
			continue // un borrador puede no tener seed todavía
		}
		for i, linea := range strings.Split(string(data), "\n") {
			l := strings.TrimSpace(linea)
			if l == "" || strings.HasPrefix(l, "--") {
				continue
			}
			t.Errorf("%s:%d tiene una sentencia ejecutable en un pack en borrador: %q", dir, i+1, l)
		}
	}
}
