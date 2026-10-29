package web

// Pruebas de base del tablero de cartera y de la capa de avisos.
//
// Son pruebas de SENTENCIAS REALES contra SQLite en memoria, por el mismo motivo
// que las del libro de idempotencia: lo que hay que demostrar aquí no es una
// función pura, es que el escaneo abre la alerta cuando la deuda se atrasa, que
// la CIERRA sola cuando se cobra, que una alerta resuelta a mano no se reabre y
// que el mismo aviso no sale dos veces ni dentro del enfriamiento. Ninguna de
// esas cuatro cosas se puede comprobar con un mock.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

// nuevaBaseDeCartera crea las tablas que tocan el escaneo y los avisos.
func nuevaBaseDeCartera(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := []string{
		`CREATE TABLE customer_information (customer_id INTEGER PRIMARY KEY, customer_name TEXT,
			customer_email TEXT, email_address TEXT, customer_mobile TEXT)`,
		`CREATE TABLE supplier_information (supplier_id INTEGER PRIMARY KEY, supplier_name TEXT,
			email_address TEXT, emailnumber TEXT, mobile TEXT)`,
		`CREATE TABLE invoice (id INTEGER PRIMARY KEY AUTOINCREMENT, invoice_id INTEGER, customer_id INTEGER,
			date TEXT, total_amount REAL DEFAULT 0, paid_amount REAL DEFAULT 0, due_amount REAL DEFAULT 0,
			invoice INTEGER DEFAULT 0, is_anulled INTEGER DEFAULT 0)`,
		`CREATE TABLE debt_collection (id INTEGER PRIMARY KEY AUTOINCREMENT, customer_id INTEGER,
			invoice_id INTEGER DEFAULT 0, amount REAL DEFAULT 0, collection_date TEXT)`,
		`CREATE TABLE product_purchase (id INTEGER PRIMARY KEY AUTOINCREMENT, purchase_id INTEGER,
			supplier_id INTEGER, purchase_date TEXT, chalan_no TEXT, control_no TEXT,
			grand_total_amount REAL DEFAULT 0, paid_amount REAL DEFAULT 0, due_amount REAL DEFAULT 0)`,
		`CREATE TABLE accounts_payable (id INTEGER PRIMARY KEY AUTOINCREMENT, supplier_id INTEGER,
			purchase_id INTEGER, concept TEXT, origin TEXT, amount REAL DEFAULT 0, paid_amount REAL DEFAULT 0,
			due_amount REAL DEFAULT 0, payment_method_id INTEGER DEFAULT 0, payment_date TEXT,
			reference TEXT, notes TEXT, created_by TEXT, created_at TEXT)`,
		`CREATE TABLE vat_retention (id INTEGER PRIMARY KEY AUTOINCREMENT, retention_no TEXT, period TEXT,
			date TEXT, supplier_id INTEGER, invoice_no TEXT, control_no TEXT,
			base_amount REAL DEFAULT 0, vat_amount REAL DEFAULT 0, retention_amount REAL DEFAULT 0)`,
		`CREATE TABLE company_information (company_id TEXT, company_name TEXT, status INTEGER)`,
		`CREATE TABLE finance_alert (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope TEXT NOT NULL DEFAULT 'cxc', kind TEXT NOT NULL DEFAULT '', severity TEXT NOT NULL DEFAULT 'aviso',
			party_type TEXT NOT NULL DEFAULT 'customer', party_id INTEGER NOT NULL DEFAULT 0,
			party_name TEXT NOT NULL DEFAULT '', ref_type TEXT NOT NULL DEFAULT '', ref_id TEXT NOT NULL DEFAULT '',
			amount REAL NOT NULL DEFAULT 0, days INTEGER NOT NULL DEFAULT 0, metric_json TEXT,
			fingerprint TEXT NOT NULL UNIQUE, status TEXT NOT NULL DEFAULT 'abierta',
			notified_at TEXT, notify_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE finance_message (
			id INTEGER PRIMARY KEY AUTOINCREMENT, channel_id INTEGER NOT NULL DEFAULT 0,
			channel TEXT NOT NULL DEFAULT '', channel_kind TEXT NOT NULL DEFAULT '',
			recipient TEXT NOT NULL DEFAULT '', subject TEXT NOT NULL DEFAULT '', body TEXT,
			ref_type TEXT NOT NULL DEFAULT '', ref_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'enviando', error TEXT, provider_ref TEXT,
			idem_key TEXT NOT NULL UNIQUE, created_by TEXT, created_at TEXT NOT NULL, sent_at TEXT)`,
		`CREATE TABLE finance_scan_state (scope TEXT PRIMARY KEY, last_run TEXT NOT NULL, last_result TEXT)`,
		`CREATE TABLE notify_channel (
			id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL DEFAULT 'email', name TEXT NOT NULL UNIQUE,
			is_active INTEGER NOT NULL DEFAULT 1, ai_allowed INTEGER NOT NULL DEFAULT 0,
			cooldown_days INTEGER NOT NULL DEFAULT 3, host TEXT, port INTEGER NOT NULL DEFAULT 587,
			username TEXT, password TEXT, from_address TEXT, from_name TEXT,
			security TEXT NOT NULL DEFAULT 'starttls', url TEXT, http_method TEXT NOT NULL DEFAULT 'POST',
			auth_header TEXT, auth_token TEXT, sign_secret TEXT, msg_device_id TEXT NOT NULL DEFAULT '', msg_template_code TEXT NOT NULL DEFAULT '', notes TEXT,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, s := range ddl {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("no se pudo crear la tabla: %v\n%s", err, s)
		}
	}
	return db
}

func finInsertarFactura(t *testing.T, db *sql.DB, invoiceID, customerID int64, fecha string, total, due float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO invoice (invoice_id, customer_id, date, total_amount, paid_amount, due_amount, invoice, is_anulled)
		VALUES (?,?,?,?,?,?,?,0)`, invoiceID, customerID, fecha, total, total-due, due, invoiceID); err != nil {
		t.Fatalf("no se pudo insertar la factura: %v", err)
	}
}

// TestEscanerAbreCierraYNoReabreLoResuelto cubre las tres reglas del escaneo que
// hacen que el tablero siga sirviendo dentro de un mes.
func TestEscanerAbreCierraYNoReabreLoResuelto(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	s := &Server{}

	if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name, customer_email, customer_mobile)
		VALUES (1, 'Ferretería El Tornillo', 'pagos@tornillo.test', '04140000000')`); err != nil {
		t.Fatal(err)
	}
	// Una factura de hace 120 días: pasado el umbral crítico (90).
	vieja := time.Now().AddDate(0, 0, -120).Format("2006-01-02")
	nueva := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	finInsertarFactura(t, db, 10, 1, vieja, 1000, 1000)
	finInsertarFactura(t, db, 11, 1, nueva, 200, 200)

	primero, err := s.financeEscanear(db, scopeCxC, "prueba", true)
	if err != nil {
		t.Fatalf("escaneo: %v", err)
	}
	if fmt.Sprintf("%v", primero["nuevas"]) == "0" {
		t.Fatalf("el escaneo no abrió ninguna alerta con una factura de 120 días: %v", primero)
	}

	alertas := s.finAlertas(db, scopeCxC, false, 50)
	if len(alertas) == 0 {
		t.Fatal("la alerta se dijo creada pero no aparece en la lista")
	}
	var vencida map[string]interface{}
	for _, a := range alertas {
		if fmt.Sprintf("%v", a["kind"]) == "cartera_vencida" {
			vencida = a
		}
	}
	if vencida == nil {
		t.Fatalf("no se creó la alerta de cartera vencida: %v", alertas)
	}
	if fmt.Sprintf("%v", vencida["severity"]) != "critico" {
		t.Errorf("con 120 días la alerta tiene que ser crítica, salió %v", vencida["severity"])
	}
	if !strings.Contains(fmt.Sprintf("%v", vencida["motivo"]), "120") {
		t.Errorf("el motivo tiene que decir los días de atraso; salió: %v", vencida["motivo"])
	}

	// Segundo escaneo sin cambios: no duplica nada, actualiza.
	segundo, _ := s.financeEscanear(db, scopeCxC, "prueba", true)
	if fmt.Sprintf("%v", segundo["nuevas"]) != "0" {
		t.Errorf("el segundo escaneo creó alertas nuevas: %v", segundo)
	}
	if n := contarFilas(t, db, "finance_alert"); n == 0 {
		t.Fatalf("las alertas desaparecieron del libro: %d filas", n)
	}

	// Se cobra TODO: las alertas tienen que cerrarse solas.
	if _, err := db.Exec(`UPDATE invoice SET due_amount = 0, paid_amount = total_amount`); err != nil {
		t.Fatal(err)
	}
	tercero, _ := s.financeEscanear(db, scopeCxC, "prueba", true)
	if fmt.Sprintf("%v", tercero["cerradas"]) == "0" {
		t.Errorf("al cobrarse la cartera las alertas tenían que cerrarse: %v", tercero)
	}
	if abiertas, _ := s.finContarAlertas(db, scopeCxC); abiertas != 0 {
		t.Errorf("quedaron %d alertas abiertas con la cartera cobrada", abiertas)
	}

	// Y una alerta RESUELTA a mano no se reabre, aunque el problema persista: si
	// el escaneo la levantara otra vez, nadie volvería a mirar el tablero.
	//
	// La referencia es NUEVA a propósito: así se prueba la inserción y no la
	// actualización de una alerta que el escaneo ya había creado con la factura 10.
	ref := "999"
	estado := s.finGuardarAlerta(db, scopeCxC, "cartera_vencida", "aviso", "customer", 1,
		"Ferretería El Tornillo", "invoice", ref, 1000, 120, "prueba", nil)
	if estado != "nueva" {
		t.Fatalf("se esperaba una alerta nueva, salió %q", estado)
	}
	if _, err := db.Exec(`UPDATE finance_alert SET status = 'resuelta' WHERE kind = 'cartera_vencida' AND ref_id = ?`, ref); err != nil {
		t.Fatal(err)
	}
	if estado = s.finGuardarAlerta(db, scopeCxC, "cartera_vencida", "critico", "customer", 1,
		"Ferretería El Tornillo", "invoice", ref, 1000, 120, "prueba", nil); estado == "nueva" {
		t.Fatal("el escaneo reabrió una alerta que una persona había resuelto")
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM finance_alert WHERE kind = 'cartera_vencida' AND ref_id = ?`, ref).Scan(&status)
	if status != alertaResuelta {
		t.Errorf("la alerta resuelta a mano quedó en %q", status)
	}
}

// TestAvisoNoSeRepiteYRespetaElEnfriamiento es la parte que impide que una tarea
// programada se convierta en una máquina de mandar recordatorios.
func TestAvisoNoSeRepiteYRespetaElEnfriamiento(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	s := &Server{}

	if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name, customer_email, customer_mobile)
		VALUES (1, 'Cliente Moroso', 'cliente@test.local', '04141111111')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO company_information (company_id, company_name, status) VALUES ('1','Mi Empresa',1)`); err != nil {
		t.Fatal(err)
	}

	var entregas int64
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&entregas, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer servidor.Close()

	// Canal de SMS: la entrega es HTTP (contra el servidor de prueba) pero el
	// DESTINATARIO sale del teléfono de la ficha del cliente, que es lo que hay que
	// comprobar. Con un webhook el destino sería el del propio canal y la prueba no
	// diría nada sobre el origen del destinatario.
	if _, err := db.Exec(`INSERT INTO notify_channel (kind, name, is_active, ai_allowed, cooldown_days,
		url, http_method, created_at, updated_at) VALUES ('sms','SMS de cobranza',1,1,3,?,'POST',?,?)`,
		servidor.URL, now(), now()); err != nil {
		t.Fatal(err)
	}
	canal, err := s.notifyCanalPorID(db, 1)
	if err != nil {
		t.Fatal(err)
	}

	alerta := map[string]interface{}{
		"id": int64(1), "scope": scopeCxC, "kind": "cartera_vencida",
		"party_type": "customer", "party_id": int64(1), "party": "Cliente Moroso",
		"amount": "1.000,00", "days": 120,
		"motivo": "Cliente Moroso debe 1.000 desde hace 120 días.",
		"metricas": map[string]interface{}{
			"vencido": 1000.0, "dias": 120,
		},
	}
	peticion := &http.Request{}

	res, err := s.financeEnviarAviso(peticion, db, canal, alerta, "Estado de cuenta", "Estimados:", "aviso_cobro", "1", false)
	if err != nil {
		t.Fatalf("envío: %v", err)
	}
	if enviado, _ := res["enviado"].(bool); !enviado {
		t.Fatalf("el aviso no se envió: %v", res)
	}
	if res["destino"] != "04141111111" {
		t.Errorf("el destinatario tiene que salir del teléfono de la ficha del cliente; salió %v", res["destino"])
	}
	if got := atomic.LoadInt64(&entregas); got != 1 {
		t.Fatalf("se esperaba 1 entrega, hubo %d", got)
	}

	// El mismo aviso otra vez: no sale.
	res2, err := s.financeEnviarAviso(peticion, db, canal, alerta, "Estado de cuenta", "Estimados:", "aviso_cobro", "1", false)
	if err != nil {
		t.Fatalf("segundo envío: %v", err)
	}
	if res2["enviado"] == true {
		t.Error("el mismo aviso se envió dos veces")
	}
	if got := atomic.LoadInt64(&entregas); got != 1 {
		t.Errorf("el aviso duplicado llegó al destino: %d entregas", got)
	}

	// Un año después del primer aviso, el MISMO asunto por otra referencia sigue
	// bloqueado por el enfriamiento: la clave del día ya no coincide, el
	// enfriamiento sí.
	ayer := time.Now().AddDate(0, 0, -1).Format(nowLayout)
	if _, err := db.Exec(`INSERT INTO finance_message (channel_id, channel, channel_kind, recipient, subject,
		body, ref_type, ref_id, status, idem_key, created_by, created_at, sent_at)
		VALUES (?,?,?,?,?,?, 'aviso_cobro', '2', 'enviado', 'clave-de-ayer', 'prueba', ?, ?)`,
		canal.ID, canal.Name, canal.Kind, "04141111111", "Estado de cuenta", "cuerpo", ayer, ayer); err != nil {
		t.Fatal(err)
	}
	res3, err := s.financeEnviarAviso(peticion, db, canal, alerta, "Estado de cuenta", "Estimados:", "aviso_cobro", "2", false)
	if err != nil {
		t.Fatalf("tercer envío: %v", err)
	}
	if ok, _ := res3["cooldown"].(bool); !ok {
		t.Fatalf("el enfriamiento no bloqueó el aviso: %v", res3)
	}
	if got := atomic.LoadInt64(&entregas); got != 1 {
		t.Errorf("el aviso bloqueado por enfriamiento salió igual: %d entregas", got)
	}

	// Forzado: es una decisión explícita y sale.
	res4, err := s.financeEnviarAviso(peticion, db, canal, alerta, "Estado de cuenta", "Estimados:", "aviso_cobro", "2", true)
	if err != nil {
		t.Fatalf("envío forzado: %v", err)
	}
	if enviado, _ := res4["enviado"].(bool); !enviado {
		t.Fatalf("el envío forzado no salió: %v", res4)
	}
	if got := atomic.LoadInt64(&entregas); got != 2 {
		t.Errorf("se esperaban 2 entregas tras forzar, hubo %d", got)
	}
}

// TestRetencionSinEnviarGeneraAlertaYElEnvioLaCierra comprueba la otra cara del
// módulo: la retención de IVA pendiente de comprobante.
func TestRetencionSinEnviarGeneraAlertaYElEnvioLaCierra(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	s := &Server{}

	if _, err := db.Exec(`INSERT INTO supplier_information (supplier_id, supplier_name, email_address, mobile)
		VALUES (7, 'Distribuidora Caracas', 'cuentas@dist.test', '02121234567')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO vat_retention (retention_no, period, date, supplier_id, invoice_no, control_no,
		base_amount, vat_amount, retention_amount) VALUES ('20260300000001','202603',?,7,'0012','00-00012',1000,160,160)`,
		time.Now().AddDate(0, 0, -40).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}

	res, err := s.financeEscanear(db, scopeCxP, "prueba", true)
	if err != nil {
		t.Fatalf("escaneo: %v", err)
	}
	if fmt.Sprintf("%v", res["nuevas"]) == "0" {
		t.Fatalf("no se abrió la alerta de retención pendiente: %v", res)
	}

	hayRetencion := false
	for _, a := range s.finAlertas(db, scopeCxP, false, 50) {
		if fmt.Sprintf("%v", a["kind"]) == "pagar_retencion_pendiente" {
			hayRetencion = true
			if !strings.Contains(fmt.Sprintf("%v", a["motivo"]), "marzo 2026") {
				t.Errorf("el motivo tiene que nombrar el período en palabras, salió: %v", a["motivo"])
			}
		}
	}
	if !hayRetencion {
		t.Fatal("no hay alerta de retención de IVA sin enviar")
	}

	// El comprobante se envía (se registra el mensaje como enviado) y el escaneo
	// tiene que cerrar la alerta sin que nadie la marque a mano.
	if _, err := db.Exec(`INSERT INTO finance_message (channel_id, channel, channel_kind, recipient, subject,
		body, ref_type, ref_id, status, idem_key, created_by, created_at, sent_at)
		VALUES (1,'Correo','email','cuentas@dist.test','Retención','cuerpo','retencion_iva','supplier:7:202603','enviado','k1','prueba',?,?)`,
		now(), now()); err != nil {
		t.Fatal(err)
	}
	res2, _ := s.financeEscanear(db, scopeCxP, "prueba", true)
	if fmt.Sprintf("%v", res2["cerradas"]) == "0" {
		t.Errorf("al enviarse el comprobante la alerta tenía que cerrarse: %v", res2)
	}
	if abiertas, _ := s.finContarAlertas(db, scopeCxP); abiertas != 0 {
		t.Errorf("quedaron %d alertas abiertas de cuentas por pagar", abiertas)
	}
}

// TestDiasDeAtrasoYHuella son las dos funciones puras de las que depende todo lo
// anterior: si la antigüedad se calcula mal, la alerta sale cuando no toca.
func TestDiasDeAtrasoYHuella(t *testing.T) {
	hoy := time.Now()
	casos := []struct {
		fecha  string
		quiere int
	}{
		{hoy.Format("2006-01-02"), 0},
		{hoy.AddDate(0, 0, -45).Format("2006-01-02"), 45},
		{hoy.AddDate(0, 0, -45).Format("2006-01-02 10:30:00"), 45},
		{hoy.AddDate(0, 0, -45).Format("2006/01/02"), 45},
		{"", -1},
		{"sin fecha", -1},
	}
	for _, c := range casos {
		if got := finDiasDesde(c.fecha); got != c.quiere {
			t.Errorf("finDiasDesde(%q) = %d, se esperaba %d", c.fecha, got, c.quiere)
		}
	}
	if finBucket(-1) != "sin fecha" || finBucket(10) != "0-30" || finBucket(45) != "31-60" ||
		finBucket(75) != "61-90" || finBucket(200) != "+90" {
		t.Error("los tramos de antigüedad no son los esperados")
	}
	if finHuella("a", "b") == finHuella("a", "c") {
		t.Error("dos alertas distintas no pueden compartir huella: se pisarían entre sí")
	}
	if finHuella("a", "b") != finHuella("a", "b") {
		t.Error("la misma alerta tiene que producir siempre la misma huella")
	}
	if periodoLegible("202603") != "marzo 2026" {
		t.Errorf("periodoLegible(202603) = %q", periodoLegible("202603"))
	}
}
