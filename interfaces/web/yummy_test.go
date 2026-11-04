package web

// yummy_test.go — pruebas de la integración con la pasarela YummyProxy2.
//
// Se prueba lo que se rompe en producción: la firma HMAC del receptor (lo que
// decide si un evento entra o no), la idempotencia (la pasarela reintenta
// hasta confirmar: el mismo event_id va a llegar dos veces), el bloqueo
// fail-closed sin configuración y el cliente contra una pasarela falsa (para
// comprobar que se envía la clave del gateway y el cuerpo en el formato de su
// API). La bitácora de eventos se crea con dialecto SQLite (la prueba abre
// :memory:) ANTES de tocar los handlers: EnsureYummySchema solo crea la
// tabla si falta, así que en pruebas queda la fila de SQLite y el DDL de
// MySQL (el que corre en producción) nunca llega a ejecutarse aquí.

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

// cuerpoEventoFirme es el body EXACTO que firma (y entrega) la prueba: el
// receptor compara el HMAC contra el cuerpo que llega, así que firmar un
// cuerpo distinto al enviado es un 401 legítimo, no un error del ERP.
const cuerpoEventoFirme = `{"order_id":"YUM-1","status":"delivered"}`

// nuevaBaseYummy deja la base de pruebas con la bitácora de eventos (dialecto
// SQLite) y devuelve el handle.
func nuevaBaseYummy(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base de pruebas: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := `CREATE TABLE yummy_webhook_evento (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		event_type TEXT NOT NULL DEFAULT '',
		attempt INTEGER NOT NULL DEFAULT 0,
		payload TEXT NOT NULL,
		estado TEXT NOT NULL DEFAULT 'recibido',
		recibido_at TEXT NOT NULL DEFAULT '')`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("no se pudo crear yummy_webhook_evento: %v", err)
	}
	return db
}

// servidorYummy arma el Server de pruebas con la integración completa.
func servidorYummy(t *testing.T, db *sql.DB, cfg *configs.Config) *httptest.Server {
	t.Helper()
	s := &Server{db: db, config: cfg}
	s.mux = http.NewServeMux()
	s.registerYummyRoutes()
	serv := httptest.NewServer(s.mux)
	t.Cleanup(serv.Close)
	return serv
}

// yummyFirma compone la cabecera exacta que la pasarela envía: t=<unix>,v1=<hex>.
func yummyFirma(secreto string, momento time.Time, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secreto))
	mac.Write([]byte(fmt.Sprintf("%d.", momento.Unix())))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", momento.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

// configYummyDePrueba es la configuración que viaja SIEMPRE con el secreto
// conocido "secreto-tenant-prueba": las firmas de las pruebas se calculan con él.
func configYummyDePrueba() *configs.Config {
	return &configs.Config{
		YummyEnabled:        true,
		YummyGatewayURL:     "http://gateway.falso",
		YummyGatewayKey:     "gw_live_prueba",
		YummyCallbackSecret: "secreto-tenant-prueba",
	}
}

// entregaEvento hace UNA entrega al receptor con la cabecera de firma indicada.
func entregaEvento(t *testing.T, serv *httptest.Server, cabecera, eventID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, serv.URL+"/webhooks/yummy", strings.NewReader(cuerpoEventoFirme))
	if err != nil {
		t.Fatalf("no se pudo armar la entrega: %v", err)
	}
	if cabecera != "" {
		req.Header.Set("X-Gateway-Signature", cabecera)
	}
	req.Header.Set("X-Gateway-Event-Id", eventID)
	req.Header.Set("X-Gateway-Event-Type", "order.items.sold")
	req.Header.Set("X-Gateway-Attempt", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("la entrega falló: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestYummyWebhookRechazaSinConfigurar(t *testing.T) {
	// Fail-closed: sin YUMMY_ENABLED el receptor responde 403 sin leer nada
	// de la base de datos.
	serv := servidorYummy(t, nuevaBaseYummy(t), &configs.Config{})
	resp := entregaEvento(t, serv, "t=1,v1=00", "evt_sin_config")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("sin configuración debe responder 403, got %d", resp.StatusCode)
	}
}

func TestYummyWebhookBienFirmadoQuedaPersistido(t *testing.T) {
	db := nuevaBaseYummy(t)
	serv := servidorYummy(t, db, configYummyDePrueba())
	firma := yummyFirma("secreto-tenant-prueba", time.Now(), []byte(cuerpoEventoFirme))
	resp := entregaEvento(t, serv, firma, "evt_001")
	if resp.StatusCode != http.StatusOK {
		cuerpo, _ := io.ReadAll(resp.Body)
		t.Fatalf("evento bien firmado debe ser 200, got %d: %s", resp.StatusCode, string(cuerpo))
	}
	var cuenta int
	if err := db.QueryRow(`SELECT COUNT(*) FROM yummy_webhook_evento WHERE event_id = 'evt_001'`).Scan(&cuenta); err != nil {
		t.Fatalf("no se pudo contar la fila: %v", err)
	}
	if cuenta != 1 {
		t.Errorf("debe existir exactamente una fila del evento, hay %d", cuenta)
	}
}

func TestYummyWebhookFirmaInvalida(t *testing.T) {
	serv := servidorYummy(t, nuevaBaseYummy(t), configYummyDePrueba())
	ahora := time.Now()

	otra := yummyFirma("otro-secreto", ahora, []byte(cuerpoEventoFirme))
	if resp := entregaEvento(t, serv, otra, "evt_002"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("firma de otro secreto debe ser 401, got %d", resp.StatusCode)
	}
	if resp := entregaEvento(t, serv, "", "evt_003"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("entrega sin firma debe ser 401, got %d", resp.StatusCode)
	}
	// Firmando con la marca de tiempo fuera de la ventana (replay viejo): 401.
	viejo := time.Now().Add(-11 * time.Minute)
	resp := entregaEvento(t, serv, yummyFirma("secreto-tenant-prueba", viejo, []byte(cuerpoEventoFirme)), "evt_004")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("evento fuera de la ventana de frescura debe ser 401, got %d", resp.StatusCode)
	}
}

func TestYummyWebhookIdempotente(t *testing.T) {
	// La pasarela reintenta hasta confirmar: el MISMO event_id llega dos
	// veces. La segunda entrega responde 200 con dedup=true y NO crea una
	// segunda fila: una bitácora duplicada es un problema contable, no
	// estético.
	db := nuevaBaseYummy(t)
	serv := servidorYummy(t, db, configYummyDePrueba())
	ahora := time.Now()
	firma := yummyFirma("secreto-tenant-prueba", ahora, []byte(cuerpoEventoFirme))

	if resp := entregaEvento(t, serv, firma, "evt_dup"); resp.StatusCode != http.StatusOK {
		cuerpo, _ := io.ReadAll(resp.Body)
		t.Fatalf("primera entrega debe ser 200, got %d: %s", resp.StatusCode, string(cuerpo))
	}
	resp2 := entregaEvento(t, serv, firma, "evt_dup")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("reentrega debe seguir 200, got %d", resp2.StatusCode)
	}
	red, _ := io.ReadAll(resp2.Body)
	var salida map[string]interface{}
	if err := json.Unmarshal(red, &salida); err != nil || salida["dedup"] != true {
		t.Errorf("la reentrega debe decir dedup=true, got %s (%v)", string(red), err)
	}
	var cuenta int
	if err := db.QueryRow(`SELECT COUNT(*) FROM yummy_webhook_evento WHERE event_id = 'evt_dup'`).Scan(&cuenta); err != nil {
		t.Fatalf("no se pudo contar: %v", err)
	}
	if cuenta != 1 {
		t.Errorf("dos entregas del mismo evento deben dejar UNA fila, hay %d", cuenta)
	}
}

func TestYummyStockSyncContraPasarelaFalsa(t *testing.T) {
	// Cliente contra una pasarela falsa: se verifica que la llamada lleve la
	// clave del gateway y el cuerpo en el formato de su API (items[].sku y
	// operation set), y que el stock sea el REAL del ERP (suma de lotes
	// activos, sin contar lotes agotados ni productos pausados).
	var claveVista, cuerpoVisto string
	falsa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claveVista = r.Header.Get("X-Gateway-Key")
		b, _ := io.ReadAll(r.Body)
		cuerpoVisto = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	t.Cleanup(falsa.Close)

	cfg := configYummyDePrueba()
	cfg.YummyGatewayURL = falsa.URL

	db := nuevaBaseYummy(t)
	preparar := []string{
		`CREATE TABLE product_information (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id TEXT NOT NULL,
			product_name TEXT, status INTEGER)`,
		`CREATE TABLE product_lot (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id INTEGER NOT NULL,
			quantity_available REAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active')`,
		`INSERT INTO product_information (product_id, product_name, status) VALUES ('ARROZ-1KG', 'Arroz', 1)`,
		`INSERT INTO product_information (product_id, product_name, status) VALUES ('ACEITE-L', 'Aceite', 1)`,
		`INSERT INTO product_information (product_id, product_name, status) VALUES ('INACTIVO', 'Pausado', 0)`,
		`INSERT INTO product_lot (product_id, quantity_available, status) VALUES (1, 25, 'active')`,
		`INSERT INTO product_lot (product_id, quantity_available, status) VALUES (1, 5, 'active')`,
		`INSERT INTO product_lot (product_id, quantity_available, status) VALUES (1, 7, 'depleted')`,
	}
	for _, ddl := range preparar {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("no se pudo preparar la base: %v\n%s", err, ddl)
		}
	}
	s := &Server{db: db, config: cfg}
	s.mux = http.NewServeMux()
	s.registerYummyRoutes()
	req := httptest.NewRequest(http.MethodPost, "/yummy/stock/sync", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stock/sync debe salir 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if claveVista != "gw_live_prueba" {
		t.Errorf("la petición debe llevar la clave del gateway, got %q", claveVista)
	}
	var subido struct {
		Items []YummyStockItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(cuerpoVisto), &subido); err != nil {
		t.Fatalf("el cuerpo enviado no es JSON válido: %v", err)
	}
	if len(subido.Items) != 2 {
		t.Fatalf("deben viajar los 2 productos activos, viajaron %d: %s", len(subido.Items), cuerpoVisto)
	}
	stocks := map[string]float64{}
	for _, it := range subido.Items {
		stocks[it.SKU] = it.Stock
		if it.Operation != "set" {
			t.Errorf("la operación de %q debe ser set, got %q", it.SKU, it.Operation)
		}
	}
	if stocks["ARROZ-1KG"] != 30 {
		t.Errorf("Arroz debe sumar 30 (25+5, sin el lote depleted), got %v", stocks["ARROZ-1KG"])
	}
	if _, hay := stocks["INACTIVO"]; hay {
		t.Errorf("el producto pausado no debe viajar a la pasarela")
	}
}

func TestYummyStatusDiceLaConfiguracion(t *testing.T) {
	db := nuevaBaseYummy(t)
	if _, err := db.Exec(`INSERT INTO yummy_webhook_evento (event_id, event_type, attempt, payload, estado, recibido_at)
		VALUES ('evt_v1', 'order.items.sold', 1, '{}', 'recibido', '2026-11-04 10:00:00')`); err != nil {
		t.Fatalf("no se pudo sembrar un evento: %v", err)
	}
	serv := servidorYummy(t, db, configYummyDePrueba())
	resp, err := http.Get(serv.URL + "/yummy/status")
	if err != nil {
		t.Fatalf("GET /yummy/status falló: %v", err)
	}
	defer resp.Body.Close()
	red, _ := io.ReadAll(resp.Body)
	var estado map[string]interface{}
	if err := json.Unmarshal(red, &estado); err != nil {
		t.Fatalf("el estado no es JSON: %s", string(red))
	}
	if estado["enabled"] != true || estado["gateway_key_set"] != true {
		t.Errorf("el estado debe decir la configuración completa: %s", string(red))
	}
	lista, _ := estado["eventos"].([]interface{})
	if len(lista) != 1 {
		t.Errorf("debe listar el evento sembrado, hay %d: %s", len(lista), string(red))
	}
}

func TestYummyVentanaDeFrescura(t *testing.T) {
	// Unidad pura de la verificación de firma: dentro de la ventana pasa; más
	// allá hacia atrás (replay viejo) o hacia adelante (reloj desincronizado)
	// es rechazo, y sin cabecera o mal compuesta también.
	ahora := time.Now()
	body := []byte(`{"x":1}`)
	if err := yummyVerificarFirma("sec", body, yummyFirma("sec", ahora.Add(-4*time.Minute), body), ahora); err != nil {
		t.Errorf("dentro de la ventana debe pasar: %v", err)
	}
	if err := yummyVerificarFirma("sec", body, yummyFirma("sec", ahora.Add(-6*time.Minute), body), ahora); err == nil {
		t.Errorf("firma de hace 6 minutos debe rechazarse (la ventana es de 5)")
	}
	if err := yummyVerificarFirma("sec", body, yummyFirma("sec", ahora.Add(30*time.Minute), body), ahora); err == nil {
		t.Errorf("firma del futuro debe rechazarse")
	}
	if err := yummyVerificarFirma("sec", body, yummyFirma("otro", ahora, body), ahora); err == nil {
		t.Errorf("firma con otro secreto debe rechazarse")
	}
	if err := yummyVerificarFirma("sec", body, "", ahora); err == nil {
		t.Errorf("sin cabecera debe rechazarse")
	}
	if err := yummyVerificarFirma("sec", body, "v1=solo-hex", ahora); err == nil {
		t.Errorf("cabecera sin t= debe rechazarse")
	}
}
