package mensajeria

import "testing"

// TestNormalizarTelefonos es la tabla de casos del plan, ampliada con los que
// de verdad aparecen en las fichas. Lo que se certifica aquí no es «que quite
// espacios», sino que NO se inventa un número: lo ambiguo se rechaza.
func TestNormalizarTelefonos(t *testing.T) {
	casos := []struct {
		nombre  string
		raw     string
		pais    string
		quieres string
		error   bool
	}{
		{"nacional con cero y guiones", "0414-1234567", "VE", "584141234567", false},
		{"nacional sin cero", "4141234567", "VE", "584141234567", false},
		{"internacional con más y espacios", "+58 414 123 45 67", "VE", "584141234567", false},
		{"prefijo 00", "0058 414 1234567", "VE", "584141234567", false},
		{"código de país sin más", "58-414-1234567", "VE", "584141234567", false},
		{"paréntesis y puntos", "(0414) 123.45.67", "VE", "584141234567", false},
		{"otro país explícito se respeta", "+1 305 123 4567", "VE", "13051234567", false},
		{"colombia nacional", "300 123 4567", "CO", "573001234567", false},
		{"españa nacional", "612 34 56 78", "ES", "34612345678", false},

		{"ambiguo de siete dígitos", "1234567", "VE", "", true},
		{"con extensión", "04141234567 ext 12", "VE", "", true},
		{"con anexo", "04141234567 anexo 4", "VE", "", true},
		{"letras", "no tengo", "VE", "", true},
		{"vacío", "   ", "VE", "", true},
		{"solo prefijo internacional", "+", "VE", "", true},
		{"nacional corto para el país", "041412345", "VE", "", true},
		{"país desconocido y número nacional", "4141234567", "ZZ", "", true},
		{"país desconocido pero internacional explícito", "+49 30 123456", "ZZ", "4930123456", false},
		{"país vacío usa el de por defecto", "04141234567", "", "584141234567", false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := Normalizar(c.raw, c.pais)
			if c.error {
				if err == nil {
					t.Fatalf("debía rechazarse y devolvió %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("no debía fallar: %v", err)
			}
			if got != c.quieres {
				t.Fatalf("Normalizar(%q, %q) = %q; se esperaba %q", c.raw, c.pais, got, c.quieres)
			}
		})
	}
}

func TestEnmascararNoExponeElNumeroEntero(t *testing.T) {
	if got := Enmascarar("584141234567"); got != "+58414123 ••• 4567" {
		t.Fatalf("máscara inesperada: %q", got)
	}
	if got := Enmascarar("123"); got != "123" {
		t.Fatalf("un número corto no debe romper la máscara: %q", got)
	}
}
