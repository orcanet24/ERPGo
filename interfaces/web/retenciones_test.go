package web

// retenciones_test.go — las retenciones de IVA y ISLR, contra sentencias reales.
//
// Lo que se prueba aquí es lo que se rompió y nadie vio: que una retención se
// PUEDA guardar (el INSERT no escribía la columna `period`, que es NOT NULL, y
// la base la rechazaba), que el número de comprobante salga en el formato de la
// Providencia SNAT/2026/00093 sin que nadie lo teclee, que quede en la cadena
// de auditoría, que el libro de compras lo muestre en vez de una cadena vacía y
// que el TXT que se sube al SENIAT lleve el RIF del proveedor y no su id
// interno. Ninguna de esas seis cosas se puede comprobar con una función pura.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

// DDL fiel a migrations/ tal como lo traduce sqlite_convert para SQLite
// (sin COMMENT, sin ENGINE, id INTEGER PRIMARY KEY AUTOINCREMENT).
//
// OJO con `period NOT NULL`: es literalmente lo que declara schema_extra.sql y
// es el motivo de que el guardado fallara — la prueba vive de que el esquema de
// aquí diga lo que dice el de producción.
const ddlRetenciones = `
CREATE TABLE vat_retention (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  retention_no varchar(14) NOT NULL,
  period varchar(6) NOT NULL,
  date date NOT NULL,
  purchase_id bigint DEFAULT NULL,
  supplier_id bigint DEFAULT NULL,
  invoice_no varchar(50) DEFAULT NULL,
  control_no varchar(50) DEFAULT NULL,
  doc_type varchar(2) NOT NULL DEFAULT '01',
  total_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  base_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  vat_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  retention_percent decimal(5,2) NOT NULL DEFAULT '100.00',
  retention_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  affected_doc varchar(50) DEFAULT '0',
  exempt_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  alicuota decimal(5,2) NOT NULL DEFAULT '16.00',
  expediente varchar(20) DEFAULT '0',
  created_by varchar(30) NOT NULL,
  created_at datetime NOT NULL
);
CREATE TABLE islr_retention (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  retention_no varchar(30) NOT NULL,
  date date NOT NULL,
  supplier_id bigint DEFAULT NULL,
  concept varchar(255) DEFAULT NULL,
  base_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  retention_percent decimal(5,2) NOT NULL DEFAULT '0.00',
  retention_amount decimal(18,2) NOT NULL DEFAULT '0.00',
  created_by varchar(30) NOT NULL,
  created_at datetime NOT NULL
);
CREATE TABLE pos_counters (
  name VARCHAR(64) NOT NULL,
  value BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NULL,
  PRIMARY KEY (name)
);
CREATE TABLE product_purchase (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  purchase_id bigint NOT NULL,
  chalan_no varchar(100) NOT NULL,
  supplier_id bigint NOT NULL,
  grand_total_amount decimal(12,2) NOT NULL DEFAULT 0,
  total_vat_amnt decimal(10,2) NOT NULL DEFAULT 0,
  purchase_date varchar(50) DEFAULT NULL,
  control_no varchar(20) DEFAULT NULL,
  supplier_rif varchar(12) DEFAULT NULL,
  document_kind varchar(20) DEFAULT 'factura'
);
CREATE TABLE product_purchase_details (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  purchase_id bigint DEFAULT NULL,
  product_id varchar(30) DEFAULT NULL,
  quantity decimal(10,2) DEFAULT NULL,
  rate decimal(10,2) DEFAULT NULL,
  total_amount decimal(10,2) DEFAULT NULL,
  discount_amnt decimal(10,2) NOT NULL DEFAULT 0,
  vat_amnt_per decimal(10,2) NOT NULL DEFAULT 0,
  vat_amnt decimal(10,2) NOT NULL DEFAULT 0
);
CREATE TABLE supplier_information (
  supplier_id INTEGER PRIMARY KEY,
  supplier_name TEXT,
  rif varchar(12) DEFAULT NULL
);
CREATE TABLE company_information (
  company_id TEXT,
  company_name TEXT,
  vat_no varchar(12) DEFAULT NULL,
  status INTEGER
);
CREATE TABLE audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id varchar(30) NOT NULL,
  action varchar(20) NOT NULL,
  module varchar(50) NOT NULL,
  table_name varchar(50) DEFAULT NULL,
  record_id varchar(50) DEFAULT NULL,
  old_value text DEFAULT NULL,
  new_value text DEFAULT NULL,
  ip_address varchar(45) DEFAULT NULL,
  created_at datetime NOT NULL,
  prev_hash varchar(64) NOT NULL DEFAULT '',
  record_hash varchar(64) NOT NULL DEFAULT ''
);
CREATE TABLE audit_chain_blocks (
  period varchar(7) NOT NULL,
  prev_block_hash varchar(64) NOT NULL DEFAULT '',
  start_hash varchar(64) NOT NULL DEFAULT '',
  end_hash varchar(64) NOT NULL DEFAULT '',
  first_id int DEFAULT NULL,
  last_id int DEFAULT NULL,
  record_count int NOT NULL DEFAULT 0,
  block_hash varchar(64) NOT NULL DEFAULT '',
  sealed_at datetime DEFAULT NULL,
  PRIMARY KEY (period)
);`

func nuevaBaseRetenciones(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range strings.Split(ddlRetenciones, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("no se pudo crear el esquema: %v\n%s", err, stmt)
		}
	}
	return db
}

// nuevaPeticion monta una petición POST con la sesión y la base ya en el ámbito,
// que es el mismo camino por el que entra un guardado real.
func nuevaPeticion(t *testing.T, s *Server, db *sql.DB, ruta string, valores url.Values) *http.Request {
	t.Helper()
	r := httptest.NewRequest("POST", ruta, strings.NewReader(valores.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return withScope(r, &scope{
		tenant:   "test",
		db:       db,
		settings: &AppSettings{},
		sess:     &SessionData{Username: "contador"},
	})
}

func TestElComprobanteDeRetencionNumerayNoSeRepite(t *testing.T) {
	db := nuevaBaseRetenciones(t)

	// Una retención heredada con el formato del art. 18 ya puesta: el primer
	// número de ese período tiene que seguir por encima de la suya.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO vat_retention (retention_no, period, date, created_by, created_at)
		VALUES ('20261000000009','202610','2026-10-03','contador','2026-10-03 09:00:00')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	secuencia := []struct {
		serie, periodo, esperado string
	}{
		{SerieRetencionIVA, "202610", "20261000000010"},  // piso: no reutiliza el 9
		{SerieRetencionIVA, "202610", "20261000000011"},  // y sigue
		{SerieRetencionIVA, "202611", "20261100000001"},  // período nuevo: arranca en 1
		{SerieRetencionISLR, "202610", "20261000000001"}, // serie aparte
	}
	for _, c := range secuencia {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		got, err := comprobanteDeRetencion(tx, c.serie, c.periodo)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("comprobanteDeRetencion(%s, %s): %v", c.serie, c.periodo, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if got != c.esperado {
			t.Errorf("comprobanteDeRetencion(%s, %s) = %q, esperado %q", c.serie, c.periodo, got, c.esperado)
		}
	}

	// Y una serie que no existe se rechaza: el nombre de la tabla no se
	// concatena con lo que venga.
	if _, err := comprobanteDeRetencionTx(db, "HACER-LO-QUE-SEA", "202610"); err == nil {
		t.Error("una serie desconocida debía rechazarse, y fue aceptada")
	}
}

// comprobanteDeRetencionTx es la misma llamada sobre una transacción propia.
func comprobanteDeRetencionTx(db *sql.DB, serie, periodo string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	return comprobanteDeRetencion(tx, serie, periodo)
}

// TestGuardarRetencionEscribePeriodoComprobanteYAuditoria es la prueba del
// defecto: el INSERT de vatRetentionSave no escribía `period`, columna NOT NULL,
// y la base rechazaba la fila — el usuario veía un 500 y la retención no
// existía. Además no había número consecutivo, no había enlace con la compra y
// no quedaba rastro en la cadena de auditoría.
func TestGuardarRetencionEscribePeriodoComprobanteYAuditoria(t *testing.T) {
	db := nuevaBaseRetenciones(t)
	sembrar(db, t, `
		INSERT INTO supplier_information (supplier_id, supplier_name, rif)
		VALUES (7, 'Distribuidora Caracas', 'J-12345678-9')`)
	sembrar(db, t, `
		INSERT INTO product_purchase (purchase_id, chalan_no, supplier_id, grand_total_amount,
			total_vat_amnt, purchase_date, control_no, supplier_rif, document_kind)
		VALUES (101, 'FA-000123', 7, 1160, 160, '2026-10-28', '00-00000042', 'J123456789', 'factura')`)

	s := &Server{db: db, sessions: NewSessionStore()}
	form := url.Values{
		"date":              {"2026-10-28"},
		"purchase_id":       {"101"},
		"invoice_no":        {""}, // lo pone la compra
		"base_amount":       {"1000"},
		"vat_amount":        {"160"},
		"retention_percent": {"75"},
		"retention_amount":  {""}, // lo calcula el servidor
	}
	r := nuevaPeticion(t, s, db, "/reports/vat-retention/save", form)
	w := httptest.NewRecorder()
	s.vatRetentionSave(w, r)

	respuesta := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(respuesta, `"status":1`) {
		t.Fatalf("el guardado falló (código %d): %s", w.Code, respuesta)
	}

	var guardada struct {
		RetentionNo string `json:"retention_no"`
		Mensaje     string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &guardada); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}

	var retentionNo, periodo, date, control string
	var purchaseID int64
	var total, base, vat, percent, retained, alicuota float64
	err := db.QueryRow(`SELECT retention_no, period, date, IFNULL(purchase_id,0),
		IFNULL(control_no,''), total_amount, base_amount, vat_amount,
		retention_percent, retention_amount, alicuota
		FROM vat_retention`).Scan(&retentionNo, &periodo, &date, &purchaseID,
		&control, &total, &base, &vat, &percent, &retained, &alicuota)
	if err != nil {
		t.Fatalf("la retención no quedó en la base: %v", err)
	}

	// 1. El formato del art. 18: AAAAMM + 8 dígitos.
	if !regexp.MustCompile(`^\d{6}\d{8}$`).MatchString(retentionNo) {
		t.Errorf("el comprobante %q no tiene el formato AAAAMM + 8 dígitos", retentionNo)
	}
	// 2. El período, que era la columna que faltaba.
	if periodo != "202610" {
		t.Errorf("period = %q, esperado 202610: sin esta columna la retención no aparece en ningún informe por período", periodo)
	}
	// 3. El enlace con la compra, que era siempre 0.
	if purchaseID != 101 {
		t.Errorf("purchase_id = %d, esperado 101: sin el enlace ni el libro ni la regla de no borrado ven este comprobante", purchaseID)
	}
	// 4. El control de la factura, que no se tecleaba.
	if control != "00-00000042" {
		t.Errorf("control_no = %q, esperado 00-00000042", control)
	}
	// 5. La alícuota es la del IVA, no el % de retención.
	if alicuota != 16 {
		t.Errorf("alicuota = %.2f, esperado 16.00 (la alícuota del IVA, no el 75%% de retención)", alicuota)
	}
	if retained != 120 {
		t.Errorf("retention_amount = %.2f, esperado 120.00 (160 × 75%%)", retained)
	}
	if base != 1000 || vat != 160 {
		t.Errorf("base/vat = %.2f/%.2f, esperado 1000/160", base, vat)
	}
	if total != 1160 {
		t.Errorf("total_amount = %.2f, esperado 1160 (el total de la compra)", total)
	}
	// 6. La respuesta devuelve el número asignado: la pantalla lo muestra.
	if guardada.RetentionNo != retentionNo {
		t.Errorf("la respuesta dice %q y en la base está %q", guardada.RetentionNo, retentionNo)
	}
	if !strings.Contains(guardada.Mensaje, retentionNo) {
		t.Errorf("el mensaje %q no trae el número: sin él, ¿cómo sabe el usuario qué salió?", guardada.Mensaje)
	}

	// 7. LA TRAZA. Sin esta fila, quien reconstruya los hechos no puede saber
	// quién retuvo ni cuándo si la tabla cambia por fuera de la aplicación.
	var tabla, registro, usuario, accion string
	err = db.QueryRow(`SELECT table_name, record_id, user_id, action FROM audit_log
		WHERE table_name = 'vat_retention' ORDER BY id DESC LIMIT 1`).
		Scan(&tabla, &registro, &usuario, &accion)
	if err != nil {
		t.Fatalf("la retención no quedó en la cadena de auditoría: %v", err)
	}
	if registro != retentionNo {
		t.Errorf("la auditoría apunta al registro %q y el comprobante es %q", registro, retentionNo)
	}
	if accion != "create" || usuario != "contador" {
		t.Errorf("auditoría = %s/%s, esperado create/contador", accion, usuario)
	}

	// Y una segunda retención NO puede repetir el número.
	w2 := httptest.NewRecorder()
	s.vatRetentionSave(w2, nuevaPeticion(t, s, db, "/reports/vat-retention/save", form))
	if w2.Code != http.StatusOK {
		t.Fatalf("el segundo guardado falló: %s", w2.Body.String())
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM vat_retention`).Scan(&n)
	if n != 2 {
		t.Fatalf("se esperaban 2 retenciones, hay %d", n)
	}
	var distintos int
	_ = db.QueryRow(`SELECT COUNT(DISTINCT retention_no) FROM vat_retention`).Scan(&distintos)
	if distintos != 2 {
		t.Errorf("los dos comprobantes son %d distintos: el número se está reutilizando", distintos)
	}
}

// TestGuardarRetencionRechazaLoQueNoCierra comprueba que lo que no se puede
// declarar no llega a la base: una fecha ilegible no podía convertirse en
// período, y sin período no hay número de comprobante posible.
func TestGuardarRetencionRechazaLoQueNoCierra(t *testing.T) {
	db := nuevaBaseRetenciones(t)
	s := &Server{db: db, sessions: NewSessionStore()}

	casos := []struct {
		que    string
		form   url.Values
		espera string
	}{
		{"fecha ilegible", url.Values{"date": {"ayer"}, "base_amount": {"10"}, "vat_amount": {"1.6"}, "retention_percent": {"75"}}, "fecha"},
		{"sin proveedor ni compra", url.Values{"date": {"2026-10-28"}, "base_amount": {"10"}, "vat_amount": {"1.6"}, "retention_percent": {"75"}}, "proveedor"},
		{"% de retención fuera de rango", url.Values{"date": {"2026-10-28"}, "base_amount": {"10"}, "vat_amount": {"1.6"}, "retention_percent": {"750"}}, "% de retención"},
	}
	for _, c := range casos {
		w := httptest.NewRecorder()
		s.vatRetentionSave(w, nuevaPeticion(t, s, db, "/reports/vat-retention/save", c.form))
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), `"status":1`) {
			t.Errorf("%s: el guardado fue aceptado", c.que)
			continue
		}
		if !strings.Contains(strings.ToLower(w.Body.String()), strings.ToLower(c.espera)) {
			t.Errorf("%s: el mensaje no explica el problema: %s", c.que, w.Body.String())
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM vat_retention`).Scan(&n)
	if n != 0 {
		t.Errorf("se rechazaron las peticiones pero quedaron %d filas en la base", n)
	}
}

// TestElLibroDeComprasMuestraElComprobanteYLosExentos cubre las dos columnas
// que eran literales: `” AS retention_no` y `0 AS exempt`.
func TestElLibroDeComprasMuestraElComprobanteYLosExentos(t *testing.T) {
	db := nuevaBaseRetenciones(t)
	sembrar(db, t, `
		INSERT INTO supplier_information (supplier_id, supplier_name, rif)
		VALUES (7, 'Distribuidora Caracas', 'J123456789')`)
	// El gran total lo calcula el formulario como Σrenglones + IVA: 1000 gravados
	// + 160 exentos + 160 de IVA = 1320. Con la cuenta antigua del libro
	// (total − IVA) la base salía en 1160, metiendo los exentos dentro de la
	// base imponible.
	sembrar(db, t, `
		INSERT INTO product_purchase (purchase_id, chalan_no, supplier_id, grand_total_amount,
			total_vat_amnt, purchase_date, control_no, document_kind)
		VALUES (101, 'FA-000123', 7, 1320, 160, '2026-10-28', '00-00000042', 'factura')`)
	sembrar(db, t, `
		INSERT INTO product_purchase_details (purchase_id, product_id, quantity, rate, total_amount, discount_amnt, vat_amnt_per, vat_amnt)
		VALUES (101, 'P-1', 1, 1000, 1000, 0, 16, 160),
		       (101, 'P-2', 2, 80, 160, 0, 0, 0)`)
	// La retención quedó enlazada SOLO por proveedor + número de factura, que es
	// como quedan las anteriores al formulario que vincula la compra.
	sembrar(db, t, `
		INSERT INTO vat_retention (retention_no, period, date, purchase_id, supplier_id, invoice_no,
			control_no, total_amount, base_amount, vat_amount, retention_percent, retention_amount,
			alicuota, created_by, created_at)
		VALUES ('20261000000001','202610','2026-10-28', 0, 7, 'FA-000123',
			'00-00000042', 1160, 1000, 160, 75, 120, 16, 'contador', '2026-10-28 10:00:00')`)

	s := &Server{db: db, sessions: NewSessionStore()}
	r := withScope(httptest.NewRequest("GET", "/reports/book-purchases/data?month=10&year=2026", nil),
		&scope{tenant: "test", db: db, settings: &AppSettings{}, sess: &SessionData{Username: "contador"}})
	w := httptest.NewRecorder()
	s.bookPurchasesData(w, r)

	var respuesta struct {
		Data []struct {
			RetentionNo string  `json:"retention_no"`
			Base        float64 `json:"base_amount"`
			Vat         float64 `json:"vat_amount"`
			Exempt      float64 `json:"exempt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &respuesta); err != nil {
		t.Fatalf("respuesta ilegible: %v\n%s", err, w.Body.String())
	}
	if len(respuesta.Data) != 1 {
		t.Fatalf("se esperaba 1 renglón en el libro, hay %d: %s", len(respuesta.Data), w.Body.String())
	}
	fila := respuesta.Data[0]

	if !strings.Contains(fila.RetentionNo, "20261000000001") {
		t.Errorf("retention_no = %q: el libro tenía un literal vacío y la retención existe en su tabla", fila.RetentionNo)
	}
	if fila.Exempt != 160 {
		t.Errorf("exempt = %.2f, esperado 160.00 (el renglón sin IVA)", fila.Exempt)
	}
	if fila.Base != 1000 {
		t.Errorf("base_amount = %.2f, esperado 1000.00: la base es total − IVA − exento", fila.Base)
	}
	if fila.Vat != 160 {
		t.Errorf("vat_amount = %.2f, esperado 160.00", fila.Vat)
	}
}

// TestElTxtDeRetencionesLlevaElRifYElControl es la prueba del archivo que se
// sube al SENIAT: la columna RIF_Prov salía del id interno del proveedor y la
// alícuota llevaba el % de retención.
func TestElTxtDeRetencionesLlevaElRifYElControl(t *testing.T) {
	db := nuevaBaseRetenciones(t)
	sembrar(db, t, `
		INSERT INTO supplier_information (supplier_id, supplier_name, rif)
		VALUES (7, 'Distribuidora Caracas', 'J-12345678-9')`)
	sembrar(db, t, `INSERT INTO company_information (company_id, company_name, vat_no, status)
		VALUES ('1', 'Mi Empresa C.A.', 'J-87654321-0', 1)`)
	sembrar(db, t, `
		INSERT INTO product_purchase (purchase_id, chalan_no, supplier_id, grand_total_amount,
			total_vat_amnt, purchase_date, document_kind)
		VALUES (101, 'FA-000123', 7, 1160, 160, '2026-10-28', 'factura')`)
	sembrar(db, t, `
		INSERT INTO vat_retention (retention_no, period, date, purchase_id, supplier_id, invoice_no,
			control_no, total_amount, base_amount, vat_amount, retention_percent, retention_amount,
			alicuota, exempt_amount, created_by, created_at)
		VALUES ('20261000000001','202610','2026-10-28', 101, 7, 'FA-000123',
			'00-00000042', 1160, 1000, 160, 75, 120, 16, 0, 'contador', '2026-10-28 10:00:00')`)

	s := &Server{db: db, sessions: NewSessionStore()}
	r := withScope(httptest.NewRequest("GET", "/reports/vat-retention/txt?periodo=202610", nil),
		&scope{tenant: "test", db: db, settings: &AppSettings{}, sess: &SessionData{Username: "contador"}})
	w := httptest.NewRecorder()
	s.vatRetentionTxt(w, r)

	lineas := strings.Split(strings.TrimRight(w.Body.String(), "\r\n"), "\r\n")
	if len(lineas) != 1 {
		t.Fatalf("se esperaba 1 renglón de datos, hay %d: %q", len(lineas), w.Body.String())
	}
	cols := strings.Split(lineas[0], "\t")
	if len(cols) != 16 {
		t.Fatalf("el renglón tiene %d columnas y el formato son 16: %v", len(cols), cols)
	}

	// Col. 6: RIF del proveedor, sin guiones, con letra.
	if cols[5] != "J123456789" {
		t.Errorf("RIF_Prov = %q, esperado J123456789: antes salía el id interno del proveedor disfrazado de RIF", cols[5])
	}
	// Col. 8: número de control de la factura.
	if cols[7] != "00-00000042" {
		t.Errorf("Nro_Control = %q, esperado 00-00000042: estaba puesto a mano en vacío", cols[7])
	}
	// Col. 15: alícuota del IVA, no el % de retención.
	if cols[14] != "16.00" {
		t.Errorf("Alícuota = %q, esperado 16.00: esa columna es la del IVA, no el 75%% retenido", cols[14])
	}
	// Col. 13: número de comprobante.
	if cols[12] != "20261000000001" {
		t.Errorf("Nro_Comprobante = %q, esperado 20261000000001", cols[12])
	}
	// Col. 9: monto total, que salía en cero porque no enlazaba con la compra.
	if cols[8] != "1160.00" {
		t.Errorf("Monto_Total = %q, esperado 1160.00", cols[8])
	}
	// Col. 1: período del archivo.
	if cols[1] != "202610" {
		t.Errorf("Periodo = %q, esperado 202610", cols[1])
	}
}

// sembrar ejecuta una sentencia de prueba y falla con ella si no entra.
func sembrar(db *sql.DB, t *testing.T, sentencia string) {
	t.Helper()
	if _, err := db.Exec(sentencia); err != nil {
		t.Fatalf("sembrando datos de prueba: %v", err)
	}
}
