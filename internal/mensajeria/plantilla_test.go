package mensajeria

import (
	"strings"
	"testing"
)

func TestVariablesDeListaSinRepetirYOrdenadas(t *testing.T) {
	cuerpo := "Hola {{cliente}}, su saldo es {{ monto }}. {{cliente}}, gracias."
	got := VariablesDe(cuerpo)
	if len(got) != 2 || got[0] != "cliente" || got[1] != "monto" {
		t.Fatalf("variables inesperadas: %v", got)
	}
}

func TestRenderSustituyeYRechazaHuecos(t *testing.T) {
	ok, err := Render("Hola {{cliente}}, debe {{monto}} desde {{fecha}}.", map[string]string{
		"cliente": "Ana", "monto": "1.234,56", "fecha": "29/10/2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok != "Hola Ana, debe 1.234,56 desde 29/10/2026." {
		t.Fatalf("render inesperado: %q", ok)
	}

	if _, err := Render("Hola {{cliente}}, debe {{monto}}.", map[string]string{"cliente": "Ana"}); err == nil {
		t.Fatal("faltando el monto debía fallar")
	}
	if _, err := Render("Hola {{cliente}}.", map[string]string{"cliente": "   "}); err == nil {
		t.Fatal("una variable vacía debía fallar: un hueco en el mensaje es peor que no enviarlo")
	}
}

func TestValidarPlantillaRechazaVariablesDesconocidasYLlavesRotas(t *testing.T) {
	casos := []struct {
		cuerpo string
		error  bool
	}{
		{"Hola {{cliente}}", false},
		{"Hola {{cliente}", true},
		{"Hola {{saldo}}", true},            // variable que no existe
		{"Hola {{ cliente }}", false},       // espacios tolerados
		{"Hola {{cliente.apellido}}", true}, // nombre no válido
		{"", true},
		{strings.Repeat("x", LargoMaximoPlantilla+1), true},
	}
	for _, c := range casos {
		err := ValidarPlantilla(c.cuerpo)
		if c.error && err == nil {
			t.Errorf("%q debía rechazarse", c.cuerpo)
		}
		if !c.error && err != nil {
			t.Errorf("%q debía aceptarse: %v", c.cuerpo, err)
		}
	}
}
