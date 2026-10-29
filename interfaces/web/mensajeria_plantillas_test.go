package web

// Pruebas de la fase 2B: plantillas y aviso con plantilla.
//
// Lo que se certifica: que una plantilla con una variable inventada no se
// guarde, que el aviso se renderice con los datos de la ficha, y que si falta un
// dato el aviso quede en error en vez de salir con un hueco.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func basePlantillas(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := baseAvisos(t)
	ddl := []string{
		`CREATE TABLE msg_template (id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL UNIQUE,
			area TEXT DEFAULT 'general', name TEXT DEFAULT '', language TEXT DEFAULT 'es', body TEXT NOT NULL,
			variables_json TEXT DEFAULT '', is_active INTEGER DEFAULT 1, created_by TEXT DEFAULT '',
			updated_at TEXT DEFAULT '')`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar plantillas: %v\n%s", err, q)
		}
	}
	return s, db
}

func guardarPlantilla(t *testing.T, s *Server, db *sql.DB, valores map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	for k, v := range valores {
		form.Set(k, v)
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/plantillas/guardar",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaPlantillaSave(res, req)
	return res
}

func TestGuardarPlantillaValidaYRechazaVariablesInventadas(t *testing.T) {
	s, db := basePlantillas(t)

	res := guardarPlantilla(t, s, db, map[string]string{
		"code": "cxc_vencida_v1", "area": "cxc", "name": "Factura vencida",
		"body": "Hola {{cliente}}, debe {{monto}}.", "is_active": "1",
	})
	if res.Code != http.StatusOK {
		t.Fatalf("la plantilla válida no se guardó: %d %s", res.Code, res.Body.String())
	}

	res = guardarPlantilla(t, s, db, map[string]string{
		"code": "cxc_mala", "area": "cxc", "name": "Mala",
		"body": "Hola {{saldo}}.",
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("una variable inventada debía rechazarse; respondió %d", res.Code)
	}

	res = guardarPlantilla(t, s, db, map[string]string{
		"code": "cxc_vencida_v1", "area": "cxc", "name": "Otra",
		"body": "Hola {{cliente}}.",
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("un código repetido debía rechazarse; respondió %d", res.Code)
	}

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_template`).Scan(&n)
	if n != 1 {
		t.Fatalf("plantillas guardadas = %d; debe ser 1", n)
	}
}

func TestVistaPreviaRenderizaConDatosDeEjemplo(t *testing.T) {
	s, db := basePlantillas(t)
	form := url.Values{"body": {"Hola {{cliente}}, debe {{monto}}."}}
	req := httptest.NewRequest(http.MethodPost, "/settings/mensajeria/plantillas/preview",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaPlantillaPreview(res, req)

	m := jsonDePrueba(t, res)
	if m["status"] != float64(1) {
		t.Fatalf("la vista previa falló: %v", m)
	}
	if !strings.Contains(m["texto"].(string), "Cliente de ejemplo") {
		t.Fatalf("la vista previa no sustituyó: %v", m)
	}
}

func TestCanalGowaConPlantillaRenderizaElAviso(t *testing.T) {
	s, db := basePlantillas(t)
	if _, err := db.Exec(`INSERT INTO msg_template (code, area, name, body, is_active)
		VALUES ('cxc_vencida', 'cxc', 'Vencida', 'Hola {{cliente}}, debe {{monto}}.', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE notify_channel SET msg_template_code = 'cxc_vencida' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	canal, _ := s.notifyCanalPorID(db, 1)
	r := reqPrueba(s, sessPrueba("master"), "/finance/mensajes")
	alerta := map[string]interface{}{
		"party_type": "customer", "party_id": int64(1), "amount": float64(100.5),
	}
	res, err := s.financeEnviarAviso(r, db, canal, alerta, "Asunto que no debe usarse", "Cuerpo libre",
		"alerta", "1", false)
	if err != nil {
		t.Fatal(err)
	}
	if res["encolado"] != true {
		t.Fatalf("el aviso debía quedar en cola: %v", res)
	}

	var cuerpo string
	if err := db.QueryRow(`SELECT body FROM msg_outbox ORDER BY id LIMIT 1`).Scan(&cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo != "Hola Cliente, debe 100.50." {
		t.Fatalf("texto encolado inesperado: %q", cuerpo)
	}
}

func TestPlantillaSinDatoDejaElAvisoEnErrorYNoEncola(t *testing.T) {
	s, db := basePlantillas(t)
	if _, err := db.Exec(`INSERT INTO msg_template (code, area, name, body, is_active)
		VALUES ('con_enlace', 'cxc', 'Con enlace', 'Hola {{cliente}}, vea {{enlace}}.', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE notify_channel SET msg_template_code = 'con_enlace' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	canal, _ := s.notifyCanalPorID(db, 1)
	r := reqPrueba(s, sessPrueba("master"), "/finance/mensajes")
	alerta := map[string]interface{}{"party_type": "customer", "party_id": int64(1), "amount": float64(10)}

	res, err := s.financeEnviarAviso(r, db, canal, alerta, "Asunto", "Cuerpo", "alerta", "1", false)
	if err != nil {
		t.Fatal(err)
	}
	if res["enviado"] == true {
		t.Fatal("un aviso con un hueco no puede darse por bueno")
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_outbox`).Scan(&n)
	if n != 0 {
		t.Fatal("se encoló un aviso con una variable sin dato")
	}
	var estado, motivo string
	if err := db.QueryRow(`SELECT status, IFNULL(error,'') FROM finance_message ORDER BY id LIMIT 1`).Scan(&estado, &motivo); err != nil {
		t.Fatal(err)
	}
	if estado != "error" || !strings.Contains(motivo, "enlace") {
		t.Fatalf("el error debía nombrar la variable que falta: %q / %q", estado, motivo)
	}
}
