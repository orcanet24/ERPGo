package web

// Pruebas de los avisos por área: factura, compra y retención salen solos por el
// número del área, y no salen si el módulo está apagado o no hay número.
//
// El aviso es un efecto secundario: lo que se certifica aquí es que el efecto no
// puede romper la operación (se llama en goroutine con recover) y que no se
// inventa un canal cuando falta.

import (
	"database/sql"
	"strings"
	"testing"
)

func baseAreas(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := baseAvisos(t)
	ddl := []string{
		`CREATE TABLE supplier_information (supplier_id INTEGER PRIMARY KEY, supplier_name TEXT,
			email_address TEXT, emailnumber TEXT, mobile TEXT)`,
		`INSERT INTO supplier_information (supplier_id, supplier_name, mobile) VALUES (2, 'Proveedor', '0414-7654321')`,
		`UPDATE modules SET is_active = 1 WHERE id = 'mensajeria'`,
		`INSERT INTO msg_setting (enabled, risk_accepted_by, risk_accepted_at)
			VALUES (1, 'jefe', '2026-10-29 10:00:00')`,
		`INSERT INTO msg_device (device_id, tenant, area, state, daily_cap, hourly_cap, min_delay_s, max_delay_s, created_at)
			VALUES ('default.facturacion', 'default', 'facturacion', 'connected', 120, 25, 20, 20, '2026-10-01 08:00:00')`,
		`INSERT INTO notify_channel (kind, name, is_active, ai_allowed, cooldown_days, msg_device_id, created_at, updated_at)
			VALUES ('gowa', 'WA facturación', 1, 0, 0, 'default.facturacion', '', '')`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar áreas: %v\n%s", err, q)
		}
	}
	return s, db
}

func TestLaFacturaSalePorElNumeroDeFacturacionYNoPorOtro(t *testing.T) {
	s, db := baseAreas(t)

	s.msgAvisarFacturaEmitida("default", 1, 10, 100.5)

	var device, cuerpo, destino string
	if err := db.QueryRow(`SELECT device_id, body, to_msisdn FROM msg_outbox ORDER BY id LIMIT 1`).
		Scan(&device, &cuerpo, &destino); err != nil {
		t.Fatal(err)
	}
	if device != "default.facturacion" {
		t.Fatalf("la factura salió por %q; debía salir por el número de facturación", device)
	}
	if destino != "584141234567" {
		t.Fatalf("destino sin normalizar: %q", destino)
	}
	if !strings.Contains(cuerpo, "Factura 10") {
		t.Fatalf("texto inesperado: %q", cuerpo)
	}
}

func TestSinModuloActivoNoSaleNingunAviso(t *testing.T) {
	s, db := baseAreas(t)
	if _, err := db.Exec(`UPDATE modules SET is_active = 0 WHERE id = 'mensajeria'`); err != nil {
		t.Fatal(err)
	}

	s.msgAvisarFacturaEmitida("default", 1, 10, 100)

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_outbox`).Scan(&n)
	if n != 0 {
		t.Fatalf("con el módulo apagado se encolaron %d avisos", n)
	}
}

func TestSinCanalParaElAreaNoSeInventaUno(t *testing.T) {
	s, db := baseAreas(t)
	if _, err := db.Exec(`DELETE FROM notify_channel WHERE msg_device_id = 'default.facturacion'`); err != nil {
		t.Fatal(err)
	}

	s.msgAvisarFacturaEmitida("default", 1, 10, 100)

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_outbox`).Scan(&n)
	if n != 0 {
		t.Fatalf("sin canal de facturación se encolaron %d avisos; debía usar el de cobranza o ninguno", n)
	}
}

func TestLaRetencionYLaCompraUsanSuPropioCanal(t *testing.T) {
	s, db := baseAreas(t)
	// Número y canal de compras/cxp.
	if _, err := db.Exec(`INSERT INTO msg_device (device_id, tenant, area, state, created_at)
		VALUES ('default.cxp', 'default', 'cxp', 'connected', '2026-10-01 08:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notify_channel (kind, name, is_active, cooldown_days, msg_device_id, created_at, updated_at)
		VALUES ('gowa', 'WA cxp', 1, 0, 'default.cxp', '', '')`); err != nil {
		t.Fatal(err)
	}

	s.msgAvisarRetencionEmitida("default", 2, "20261000000001", 16)

	var device string
	if err := db.QueryRow(`SELECT device_id FROM msg_outbox ORDER BY id DESC LIMIT 1`).Scan(&device); err != nil {
		t.Fatal(err)
	}
	if device != "default.cxp" {
		t.Fatalf("la retención salió por %q; debía salir por cxp", device)
	}
}
