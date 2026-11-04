package web

// spidi_test.go — pruebas de la integración con el agente SpidiAgent.
//
// Se prueba lo que se rompe en producción: el cobro por factura contra un
// agente FALSO (que la clave del tenant viaje y que el contrato del agente
// se cumpla), las vías que NO deben abrir cobro (factura anulada, sin saldo
// o inexistente), el bloqueo sin configuración y la actualización del estado
// local cuando el agente dice otra cosa. La bitácora spidi_pago se crea con
// dialecto SQLite (la prueba abre :memory:) ANTES de los handlers:
// EnsureSpidiSchema solo crea la tabla si falta, así que el DDL de MySQL (el
// de producción) nunca se ejecuta aquí.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erpgo/configs"
)

// agenteFalso es un clon mínimo del contrato del agente (README de SpidiAgent):
// registra la clave del tenant y el cuerpo de cada solicitud, y contesta lo
// que el caso de prueba le pidió contestar.
type agenteFalso struct {
	serv         *httptest.Server
	claveVista   string
	cuerpoVisto  string
	sesiones     map[string]string // session_id -> estado que contesta al GET
	pagosCreados map[string]string // orden -> session_id que contestó el POST
}

func nuevoAgenteFalso(t *testing.T) *agenteFalso {
	t.Helper()
	a := &agenteFalso{
		sesiones:     map[string]string{},
		pagosCreados: map[string]string{},
	}
	a.serv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.claveVista = r.Header.Get("X-Tenant-Key")
		b, _ := io.ReadAll(r.Body)
		a.cuerpoVisto = string(b)
		var pedido struct {
			TenantID string  `json:"tenant_id"`
			Monto    float64 `json:"monto"`
			Moneda   string  `json:"moneda"`
			Orden    string  `json:"orden"`
		}
		_ = json.Unmarshal(b, &pedido)
		switch {
		case r.URL.Path == "/pagos" && r.Method == http.MethodPost:
			sid := fmt.Sprintf("ses_%s", strings.ReplaceAll(pedido.Orden, "FACT-", ""))
			a.pagosCreados[pedido.Orden] = sid
			a.sesiones[sid] = "pending"
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"success":true,"data":{"session_id":%q,"payment_url":%q,"status":"pending"}}`,
				sid, "https://sandbox.mispidi.com/?id="+sid)))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/pagos/"):
			sid := strings.TrimPrefix(r.URL.Path, "/pagos/")
			estado := a.sesiones[sid]
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"session_id":%q,"status":%q}}`, sid, estado)))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"ruta desconocida"}`))
		}
	}))
	t.Cleanup(a.serv.Close)
	return a
}

// configSpidiDePrueba apunta al agente falso.
func configSpidiDePrueba(url string) *configs.Config {
	return &configs.Config{
		SpidiEnabled:   true,
		SpidiAgentURL:  url,
		SpidiTenantID:  "erpgo",
		SpidiTenantKey: "clave-tenant-erpgo",
	}
}

// baseSpidi es la base de la EMPRESA de pruebas con una factura real y la
// bitácora spidi_pago (dialecto SQLite).
func baseSpidi(t *testing.T, saldo float64, anulada int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base de pruebas: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := []string{
		`CREATE TABLE invoice (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id INTEGER NOT NULL UNIQUE,
			total_amount REAL NOT NULL DEFAULT 0,
			due_amount REAL NOT NULL DEFAULT 0,
			currency_code TEXT NOT NULL DEFAULT 'USD',
			is_anulled INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE spidi_pago (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL UNIQUE,
			invoice_id INTEGER NOT NULL,
			monto NUMERIC NOT NULL DEFAULT 0,
			moneda TEXT NOT NULL DEFAULT '',
			orden TEXT NOT NULL DEFAULT '',
			estado TEXT NOT NULL DEFAULT 'pending',
			payment_url TEXT NOT NULL DEFAULT '',
			creado_at TEXT NOT NULL DEFAULT '',
			actualizado_at TEXT NOT NULL DEFAULT '')`,
		fmt.Sprintf(`INSERT INTO invoice (invoice_id, total_amount, due_amount, currency_code, is_anulled)
			VALUES (7001, 100, %v, 'USD', %d)`, saldo, anulada),
	}
	for _, d := range ddl {
		if _, err := db.Exec(d); err != nil {
			t.Fatalf("no se pudo preparar la base: %v\n%s", err, d)
		}
	}
	return db
}

// servidorSpidi arma el Server de pruebas con la integración completa.
func servidorSpidi(t *testing.T, db *sql.DB, cfg *configs.Config) *httptest.Server {
	t.Helper()
	s := &Server{db: db, config: cfg}
	s.mux = http.NewServeMux()
	s.registerSpidiRoutes()
	serv := httptest.NewServer(s.mux)
	t.Cleanup(serv.Close)
	return serv
}

func TestSpidiSinConfigurarRechaza(t *testing.T) {
	serv := servidorSpidi(t, baseSpidi(t, 100, 0), &configs.Config{})
	resp, err := http.Post(serv.URL+"/spidi/pagos/cobro-factura", "application/json", strings.NewReader(`{"invoice_id":7001}`))
	if err != nil {
		t.Fatalf("la llamada falló: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("sin configuración debe responder 400, got %d", resp.StatusCode)
	}
}

func TestSpidiCobroFacturaCreaSesionYQuedaEnBitacora(t *testing.T) {
	agente := nuevoAgenteFalso(t)
	db := baseSpidi(t, 100, 0)
	serv := servidorSpidi(t, db, configSpidiDePrueba(agente.serv.URL))

	resp, err := http.Post(serv.URL+"/spidi/pagos/cobro-factura", "application/json", strings.NewReader(`{"invoice_id":7001}`))
	if err != nil {
		t.Fatalf("la llamada falló: %v", err)
	}
	defer resp.Body.Close()
	cuerpo, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cobro debe ser 200, got %d: %s", resp.StatusCode, string(cuerpo))
	}
	var salida struct {
		SessionID  string  `json:"session_id"`
		PaymentURL string  `json:"payment_url"`
		Estado     string  `json:"estado"`
		Monto      float64 `json:"monto"`
		Moneda     string  `json:"moneda"`
	}
	if err := json.Unmarshal(cuerpo, &salida); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	if salida.SessionID == "" || salida.PaymentURL == "" {
		t.Fatalf("la sesión debe traer session_id y payment_url: %s", string(cuerpo))
	}
	if salida.Monto != 100 {
		t.Errorf("se debe cobrar el saldo deudor (100), got %v", salida.Monto)
	}
	if salida.Moneda != "USD" {
		t.Errorf("la moneda debe venir de la factura, got %q", salida.Moneda)
	}
	if agente.claveVista != "clave-tenant-erpgo" {
		t.Errorf("la solicitud debe llevar la X-Tenant-Key del ERP, got %q", agente.claveVista)
	}
	// La bitácora de la empresa debe quedar con la fila y su estado.
	var enBase int
	if err := db.QueryRow(`SELECT COUNT(*) FROM spidi_pago
		WHERE session_id = ? AND invoice_id = 7001 AND estado = 'pending'`, salida.SessionID).Scan(&enBase); err != nil {
		t.Fatalf("no se pudo verificar la bitácora: %v", err)
	}
	if enBase != 1 {
		t.Errorf("debe existir UNA fila de la sesión con estado pending, hay %d", enBase)
	}
}

func TestSpidiViasQueNoDebenCobrar(t *testing.T) {
	t.Run("factura anulada", func(t *testing.T) {
		agente := nuevoAgenteFalso(t)
		serv := servidorSpidi(t, baseSpidi(t, 100, 1), configSpidiDePrueba(agente.serv.URL))
		resp, err := http.Post(serv.URL+"/spidi/pagos/cobro-factura", "application/json", strings.NewReader(`{"invoice_id":7001}`))
		if err != nil {
			t.Fatalf("la llamada falló: %v", err)
		}
		defer resp.Body.Close()
		cuerpo, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("factura anulada debe rechazar 400, got %d", resp.StatusCode)
		}
		var salida struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(cuerpo, &salida)
		if !strings.Contains(salida.Error, "anulado") && !strings.Contains(salida.Error, "anulada") {
			t.Errorf("el texto debe decir por qué se rechaza, got %q", salida.Error)
		}
	})
	t.Run("factura sin saldo", func(t *testing.T) {
		agente := nuevoAgenteFalso(t)
		serv := servidorSpidi(t, baseSpidi(t, 0, 0), configSpidiDePrueba(agente.serv.URL))
		resp, err := http.Post(serv.URL+"/spidi/pagos/cobro-factura", "application/json", strings.NewReader(`{"invoice_id":7001}`))
		if err != nil {
			t.Fatalf("la llamada falló: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("sin saldo debe rechazar 400, got %d", resp.StatusCode)
		}
	})
	t.Run("factura inexistente", func(t *testing.T) {
		agente := nuevoAgenteFalso(t)
		serv := servidorSpidi(t, baseSpidi(t, 100, 0), configSpidiDePrueba(agente.serv.URL))
		resp, err := http.Post(serv.URL+"/spidi/pagos/cobro-factura", "application/json", strings.NewReader(`{"invoice_id":9999}`))
		if err != nil {
			t.Fatalf("la llamada falló: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("factura inexistente debe ser 404, got %d", resp.StatusCode)
		}
	})
}

func TestSpidiEstadoSeActualizaCuandoElAgenteDiceOtraCosa(t *testing.T) {
	// El ciclo completo: se abre el cobro (pending), el pago se consuma y la
	// consulta devuelve "paid"; la bitácora local debe terminar en "paid"
	// SIN tocar contabilidad (solo el estado de la sesión).
	db := baseSpidi(t, 100, 0)

	// Se siembra la fila como la habría dejado el POST real.
	ahora := time.Now().UTC().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO spidi_pago
		(session_id, invoice_id, monto, moneda, orden, estado, payment_url, creado_at, actualizado_at)
		VALUES ('ses_7001', 7001, 100, 'USD', 'FACT-7001', 'pending', 'https://x/7001', ?, ?)`, ahora, ahora); err != nil {
		t.Fatalf("no se pudo sembrar la sesión: %v", err)
	}

	agente := nuevoAgenteFalso(t)
	agente.sesiones["ses_7001"] = "paid"

	serv := servidorSpidi(t, db, configSpidiDePrueba(agente.serv.URL))
	resp, err := http.Get(serv.URL + "/spidi/pagos/status?invoice_id=7001")
	if err != nil {
		t.Fatalf("la llamada falló: %v", err)
	}
	defer resp.Body.Close()
	cuerpo, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status debe ser 200, got %d: %s", resp.StatusCode, string(cuerpo))
	}
	var salida struct {
		Pagos []struct {
			SessionID string `json:"session_id"`
			Estado    string `json:"estado"`
		} `json:"pagos"`
	}
	if err := json.Unmarshal(cuerpo, &salida); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	if len(salida.Pagos) != 1 || salida.Pagos[0].Estado != "paid" {
		t.Errorf("el estado debe reflejar el del agente (paid), got %s", string(cuerpo))
	}
	var enBase string
	if err := db.QueryRow(`SELECT estado FROM spidi_pago WHERE session_id = 'ses_7001'`).Scan(&enBase); err != nil {
		t.Fatalf("no se pudo leer la bitácora: %v", err)
	}
	if enBase != "paid" {
		t.Errorf("la bitácora local debe quedar en paid, got %q", enBase)
	}
}

func TestSpidiStatusDiagnostico(t *testing.T) {
	serv := servidorSpidi(t, baseSpidi(t, 0, 0), configSpidiDePrueba("http://agente-falso"))
	resp, err := http.Get(serv.URL + "/spidi/status")
	if err != nil {
		t.Fatalf("la llamada falló: %v", err)
	}
	defer resp.Body.Close()
	cuerpo, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	if err := json.Unmarshal(cuerpo, &res); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	if res["enabled"] != true || res["tenant_key_set"] != true {
		t.Errorf("el diagnóstico debe decir la configuración: %s", string(cuerpo))
	}
}
