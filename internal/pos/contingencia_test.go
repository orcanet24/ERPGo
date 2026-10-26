package pos

import "testing"

// TestContingenciaRef fija el contrato de la referencia de contingencia: el
// número que identifica el papel que se llevó el cliente cuando la caja vendió
// sin red, y que después se cruza con el control fiscal de la imprenta digital.
func TestContingenciaRef(t *testing.T) {
	casos := []struct {
		terminal, fiscal, local, want string
	}{
		{"TERM-01", "7", "", "CONT-TERM01-7"},
		{"caja-centro-9f3a12", "000123", "", "CONT-9F3A12-000123"},
		{"", "", "SALE-42", "CONT-POS-SALE42"},
		{"TERM-01", "", "", "CONT-TERM01-0"},
	}
	for _, c := range casos {
		got, _ := contingenciaRef(c.terminal, c.fiscal, c.local)
		if got != c.want {
			t.Errorf("contingenciaRef(%q,%q,%q) = %q, esperado %q",
				c.terminal, c.fiscal, c.local, got, c.want)
		}
		// control_no es varchar(20): si esto crece, la inserción falla y la
		// venta offline se pierde al sincronizar.
		if len(got) > 20 {
			t.Errorf("referencia demasiado larga (%d): %q", len(got), got)
		}
	}

	// Idempotencia: la misma venta reintentada debe producir la misma
	// referencia, o la sincronización duplicaría facturas.
	a, _ := contingenciaRef("TERM-01", "7", "")
	b, _ := contingenciaRef("TERM-01", "7", "")
	if a != b {
		t.Fatalf("referencia no determinista: %q vs %q", a, b)
	}

	// Correlativo local de la caja: se guarda en offline_invoice_no.
	if _, n := contingenciaRef("T", "000123", ""); n != 123 {
		t.Errorf("correlativo local = %d, esperado 123", n)
	}
	if _, n := contingenciaRef("T", "", ""); n != 0 {
		t.Errorf("sin número fiscal el correlativo debe ser 0, fue %d", n)
	}
}
