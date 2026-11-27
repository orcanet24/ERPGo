package web

// Pruebas de la emisión digital: qué documento se arma con los datos de la
// factura y qué ventas entran a la cola.
//
// POR QUÉ CON SQLITE EN MEMORIA Y NO CON DOBLES. Lo que hay que demostrar no es
// una función pura: es que el ERP encuentra los renglones de una factura que
// guardó OTRO camino del mismo sistema. Ese desajuste (el panel guarda el id
// interno en invoice_details.invoice_id y el terminal guardó el correlativo) es
// exactamente el defecto que dejaba facturas sin líneas, y solo se ve con las
// sentencias reales.

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"erpgo/configs"

	_ "github.com/glebarez/go-sqlite"
)

// baseDeFacturacionDigital crea las tablas que lee la emisión.
func baseDeFacturacionDigital(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	for _, ddl := range []string{
		`CREATE TABLE invoice (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id INTEGER NOT NULL DEFAULT 0,
			invoice INTEGER NOT NULL,
			customer_id INTEGER NOT NULL DEFAULT 0,
			date TEXT, total_amount REAL DEFAULT 0, total_vat_amnt REAL DEFAULT 0,
			currency_code TEXT DEFAULT 'VES', exchange_rate REAL DEFAULT 1,
			igtf_amount REAL DEFAULT 0, customer_rif TEXT DEFAULT '', series TEXT DEFAULT 'FACT',
			is_credit INTEGER DEFAULT 0)`,
		`CREATE TABLE invoice_details (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id TEXT NOT NULL, product_id TEXT, description TEXT,
			quantity REAL DEFAULT 0, rate REAL DEFAULT 0, total_price REAL DEFAULT 0,
			vat_amnt REAL DEFAULT 0, vat_amnt_per TEXT DEFAULT '', discount_per TEXT DEFAULT '',
			discount_pct REAL DEFAULT 0)`,
		`CREATE TABLE product_information (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id TEXT NOT NULL, product_name TEXT DEFAULT '',
			item_type TEXT DEFAULT 'venta')`,
		`CREATE TABLE customer_information (
			customer_id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_name TEXT DEFAULT '', customer_email TEXT DEFAULT '',
			email_address TEXT DEFAULT '', customer_mobile TEXT DEFAULT '',
			phone TEXT DEFAULT '', customer_address TEXT DEFAULT '',
			address2 TEXT DEFAULT '', rif TEXT DEFAULT '')`,
		`CREATE TABLE exchange_rate (
			date TEXT NOT NULL, currency TEXT NOT NULL, rate_bcv REAL DEFAULT 0,
			source TEXT DEFAULT 'BCV')`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("no se pudo crear el esquema de prueba: %v\n%s", err, ddl)
		}
	}
	return db
}

// facturaDePrueba deja una factura con dos renglones, su cliente y su producto.
//
// llaveDeRenglones decide con qué valor se escribe invoice_details.invoice_id:
// el id interno (camino del panel) o el correlativo (camino del terminal).
func facturaDePrueba(t *testing.T, db *sql.DB, llaveDeRenglones string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO invoice
		(id, invoice_id, invoice, customer_id, date, total_amount, total_vat_amnt, currency_code,
		 exchange_rate, igtf_amount, customer_rif)
		VALUES (10, 900, 900, 5, '2026-11-27', 1160, 160, 'VES', 36.5, 0, 'J-12345678-9')`); err != nil {
		t.Fatalf("no se pudo insertar la factura: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO customer_information
		(customer_id, customer_name, customer_email, customer_mobile, customer_address, rif)
		VALUES (5, 'Cliente Ejemplo C.A.', 'facturacion@example.com', '04141234567',
			'Av. Principal, Caracas', 'J-12345678-9')`); err != nil {
		t.Fatalf("no se pudo insertar el cliente: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, product_name)
		VALUES ('SERV-001', 'Mantenimiento preventivo'), ('LIBRO', 'Libro escolar')`); err != nil {
		t.Fatalf("no se pudo insertar el producto: %v", err)
	}
	for _, renglon := range []struct {
		codigo, descripcion   string
		cantidad, precio      float64
		total, iva            float64
		porcentaje, descuento string
		descuentoPct          float64
	}{
		{"SERV-001", "Mantenimiento mensual", 1, 1000, 1160, 160, "16.00", "5.00", 5},
		{"LIBRO", "Libro escolar", 2, 25, 50, 0, "0.00", "0.00", 0},
	} {
		if _, err := db.Exec(`INSERT INTO invoice_details
			(invoice_id, product_id, description, quantity, rate, total_price, vat_amnt,
			 vat_amnt_per, discount_per, discount_pct)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			llaveDeRenglones, renglon.codigo, renglon.descripcion, renglon.cantidad,
			renglon.precio, renglon.total, renglon.iva, renglon.porcentaje,
			renglon.descuento, renglon.descuentoPct); err != nil {
			t.Fatalf("no se pudo insertar el renglón: %v", err)
		}
	}
}

// TestElDocumentoDigitalLlevaRenglonesYCliente: sin esto, la imprenta recibe
// una factura vacía y la rechaza (CG exige al menos un producto).
//
// Se prueban las DOS llaves con las que el sistema guardó los renglones.
func TestElDocumentoDigitalLlevaRenglonesYCliente(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		llave  string
	}{
		{"renglones con el id interno (panel)", "10"},
		{"renglones con el correlativo (terminal)", "900"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			db := baseDeFacturacionDigital(t)
			facturaDePrueba(t, db, caso.llave)
			s := &Server{}

			doc, err := s.diCargarDocumento(db, "900", 900)
			if err != nil {
				t.Fatalf("no se pudo armar el documento: %v", err)
			}
			if len(doc.Detalles) != 2 {
				t.Fatalf("se esperaban 2 renglones, vinieron %d", len(doc.Detalles))
			}
			primero := doc.Detalles[0]
			if primero.Codigo != "SERV-001" {
				t.Errorf("código del renglón = %q", primero.Codigo)
			}
			if primero.Nombre != "Mantenimiento preventivo" {
				t.Errorf("nombre del catálogo = %q", primero.Nombre)
			}
			if primero.PorcentajeIVA != 16 || primero.CodigoImpuesto != "G" || primero.EsExento {
				t.Errorf("alícuota mal leída: %+v", primero)
			}
			if primero.DescuentoPct != 5 {
				t.Errorf("descuento del renglón = %v", primero.DescuentoPct)
			}
			if doc.Detalles[1].PorcentajeIVA != 0 || doc.Detalles[1].CodigoImpuesto != "E" {
				t.Errorf("el renglón exento debe ir como E: %+v", doc.Detalles[1])
			}

			// La ficha del cliente: CG exige correo, teléfono y dirección.
			if doc.Cliente.Email != "facturacion@example.com" {
				t.Errorf("correo del cliente = %q", doc.Cliente.Email)
			}
			if doc.Cliente.Telefono != "04141234567" || doc.Cliente.Direccion == "" {
				t.Errorf("faltan datos de contacto: %+v", doc.Cliente)
			}
			if doc.Cliente.TipoDocFiscal != "J" {
				t.Errorf("tipo de documento del cliente = %q", doc.Cliente.TipoDocFiscal)
			}

			// La tasa y la moneda viajan con el documento.
			if doc.TasaCambio != 36.5 {
				t.Errorf("tasa = %v", doc.TasaCambio)
			}
			if doc.Moneda != "VES" {
				t.Errorf("moneda = %q", doc.Moneda)
			}
			if doc.Numero != 900 {
				t.Errorf("número del documento = %d", doc.Numero)
			}
			if doc.FechaEmision.Format("2006-01-02") != "2026-11-27" {
				t.Errorf("fecha de emisión = %v", doc.FechaEmision)
			}
		})
	}
}

// TestLaTasaSaleDeLaFacturaOSaleDelRegistroDelDia: CG rechaza la factura sin
// tasa. Si la factura no la trae, se busca la del día; si tampoco hay, el
// proveedor lo dirá con su mensaje y el documento queda en la cola.
func TestLaTasaSaleDeLaFacturaOSaleDelRegistroDelDia(t *testing.T) {
	db := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db, "10")
	if _, err := db.Exec(`UPDATE invoice SET exchange_rate = 0`); err != nil {
		t.Fatalf("no se pudo limpiar la tasa: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO exchange_rate (date, currency, rate_bcv, source)
		VALUES ('2026-11-26', 'USD', 342.8633, 'BCV')`); err != nil {
		t.Fatalf("no se pudo registrar la tasa: %v", err)
	}

	s := &Server{}
	doc, err := s.diCargarDocumento(db, "900", 900)
	if err != nil {
		t.Fatalf("no se pudo armar el documento: %v", err)
	}
	if doc.TasaCambio != 342.8633 {
		t.Errorf("la tasa debía venir del registro del día, vino %v", doc.TasaCambio)
	}
}

// TestUnaFacturaSinRenglonesNoSeEmite: mejor un error claro en la cola que una
// factura vacía en la imprenta.
func TestUnaFacturaSinRenglonesNoSeEmite(t *testing.T) {
	db := baseDeFacturacionDigital(t)
	if _, err := db.Exec(`INSERT INTO invoice (id, invoice_id, invoice, customer_id, date,
		total_amount, total_vat_amnt, customer_rif) VALUES (10, 900, 900, 5, '2026-11-27', 0, 0, 'J-12345678-9')`); err != nil {
		t.Fatalf("no se pudo insertar la factura: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name) VALUES (5, 'Cliente')`); err != nil {
		t.Fatalf("no se pudo insertar el cliente: %v", err)
	}
	s := &Server{}
	if _, err := s.diCargarDocumento(db, "900", 900); err == nil || !strings.Contains(err.Error(), "renglones") {
		t.Fatalf("debía rechazar la factura sin renglones, vino: %v", err)
	}
}

// TestUnaFacturaSinRIFNoSeEmite: una factura fiscal sin RIF no existe.
func TestUnaFacturaSinRIFNoSeEmite(t *testing.T) {
	db := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db, "10")
	if _, err := db.Exec(`UPDATE invoice SET customer_rif = '', customer_id = 0`); err != nil {
		t.Fatalf("no se pudo limpiar el RIF: %v", err)
	}
	s := &Server{}
	if _, err := s.diCargarDocumento(db, "900", 900); err == nil || !strings.Contains(err.Error(), "RIF") {
		t.Fatalf("debía rechazar la factura sin RIF, vino: %v", err)
	}
}

// ---------- La cola y el modo impresora fiscal ----------

// crearTablasDeCola deja en `db` las dos tablas de la facturación digital que
// viven en la base de CONTROL. En una instalación normal es otra base; en las
// pruebas de este archivo, la de control y la de la empresa pueden ser la misma.
func crearTablasDeCola(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS digital_invoice_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id INTEGER NOT NULL,
			company_id TEXT NOT NULL DEFAULT '1',
			addon_code TEXT NOT NULL DEFAULT '',
			strong_id TEXT, nro_control INTEGER, nro_control_texto TEXT,
			nro_documento INTEGER NOT NULL DEFAULT 0,
			tipo_documento TEXT NOT NULL DEFAULT 'FA', serie TEXT NOT NULL DEFAULT '0',
			status TEXT NOT NULL DEFAULT 'pending',
			pdf_url TEXT, xml_url TEXT, pdf_path TEXT, error_message TEXT,
			intentos INTEGER NOT NULL DEFAULT 0, ultimo_intento TEXT,
			created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS digital_invoice_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			company_id TEXT NOT NULL DEFAULT '1',
			addon_code TEXT NOT NULL,
			config_json TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT, updated_at TEXT,
			UNIQUE (company_id, addon_code))`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("no se pudo crear el esquema de la cola: %v\n%s", err, ddl)
		}
	}
}

// baseDeControlDigital crea la configuración y la cola, que viven en la base de
// CONTROL.
func baseDeControlDigital(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base de control: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	crearTablasDeCola(t, db)
	return db
}

// TestSinImprentaActivaNoSeEncola: la empresa que factura por impresora fiscal
// no debe dejar documentos pendientes que nunca se transmitirán.
func TestSinImprentaActivaNoSeEncola(t *testing.T) {
	ctl := baseDeControlDigital(t)
	db := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db, "10")

	s := &Server{controlDB: ctl, controlListo: true}
	s.diEnqueueVenta(db, "empresa1", "900")

	var filas int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM digital_invoice_log`).Scan(&filas); err != nil {
		t.Fatalf("no se pudo contar la cola: %v", err)
	}
	if filas != 0 {
		t.Fatalf("sin imprenta activa no debe encolarse nada, hay %d fila(s)", filas)
	}
}

// TestConImprentaActivaSeEncolaUnaSolaVez: el encolado es idempotente y la fila
// lleva la imprenta que la emitirá.
func TestConImprentaActivaSeEncolaUnaSolaVez(t *testing.T) {
	ctl := baseDeControlDigital(t)
	db := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db, "10")

	if _, err := ctl.Exec(`INSERT INTO digital_invoice_config
		(company_id, addon_code, config_json, enabled) VALUES ('empresa1', 'cgimprenta', '{}', 1)`); err != nil {
		t.Fatalf("no se pudo configurar la imprenta: %v", err)
	}

	s := &Server{controlDB: ctl, controlListo: true}
	s.diEnqueueVenta(db, "empresa1", "900")
	s.diEnqueueVenta(db, "empresa1", "900") // dos veces la misma venta

	var filas int
	var addon string
	var nroDocumento int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM digital_invoice_log`).Scan(&filas); err != nil {
		t.Fatalf("no se pudo contar la cola: %v", err)
	}
	if filas != 1 {
		t.Fatalf("la misma venta debe encolarse una sola vez, hay %d fila(s)", filas)
	}
	if err := ctl.QueryRow(`SELECT addon_code, nro_documento FROM digital_invoice_log`).Scan(&addon, &nroDocumento); err != nil {
		t.Fatalf("no se pudo leer la fila: %v", err)
	}
	if addon != "cgimprenta" {
		t.Errorf("la fila debe registrar la imprenta activa, quedó %q", addon)
	}
	if nroDocumento != 900 {
		t.Errorf("nro_documento = %d, se esperaba el correlativo 900", nroDocumento)
	}
}

// TestUnPendienteSinImprentaQuedaConSuMotivo: si el documento se vendió como
// digital y la imprenta se desactivó antes de transmitirlo, no se puede
// descartar en silencio: queda rechazado con el motivo a la vista.
func TestUnPendienteSinImprentaQuedaConSuMotivo(t *testing.T) {
	ctl := baseDeControlDigital(t)
	if _, err := ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, addon_code, nro_documento, tipo_documento, status)
		VALUES (900, 'empresa1', 'cgimprenta', 900, 'FA', 'pending')`); err != nil {
		t.Fatalf("no se pudo encolar la fila de prueba: %v", err)
	}

	s := &Server{controlDB: ctl, controlListo: true}
	fila := struct {
		id, nroDoc, intentos int64
		invoiceID, tenant    string
	}{id: 1, nroDoc: 900, intentos: 0, invoiceID: "900", tenant: "empresa1"}
	s.diEmitirFila(ctl, fila)

	var estado, mensaje string
	var intentos int
	if err := ctl.QueryRow(`SELECT status, intentos, COALESCE(error_message,'') FROM digital_invoice_log
		WHERE id = 1`).Scan(&estado, &intentos, &mensaje); err != nil {
		t.Fatalf("no se pudo leer la fila: %v", err)
	}
	if intentos != 1 {
		t.Errorf("intentos = %d, se esperaba 1", intentos)
	}
	if estado != "pending" {
		t.Errorf("estado = %q (con menos de %d intentos sigue en cola)", estado, diMaxIntentos)
	}
	if !strings.Contains(mensaje, "imprenta digital activa") {
		t.Errorf("el motivo debe explicar que no hay imprenta activa: %q", mensaje)
	}
}

// TestElControlTextualSeGuardaYSeLee: CG devuelve "00-00000056" y la pantalla
// tiene que mostrar eso, no solo el 56.
func TestElControlTextualSeGuardaYSeLee(t *testing.T) {
	ctl := baseDeControlDigital(t)
	if _, err := ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, addon_code, nro_documento, tipo_documento, status, nro_control, nro_control_texto)
		VALUES (900, 'empresa1', 'cgimprenta', 900, 'FA', 'sent', 56, '00-00000056')`); err != nil {
		t.Fatalf("no se pudo insertar el documento: %v", err)
	}

	docs := diDocsDeFacturas(ctl, "empresa1", []int64{900})
	doc, ok := docs[900]
	if !ok {
		t.Fatal("el documento no se leyó")
	}
	if doc.ControlTexto != "00-00000056" {
		t.Errorf("control textual = %q", doc.ControlTexto)
	}
	json := (&Server{}).diJSONDoc(10, doc)
	if json["control"] != "00-00000056" {
		t.Errorf("el panel debe mostrar el control completo, muestra %v", json["control"])
	}
}

// TestEmitirPorCGGuardaElControlConSuSerieDeExtremoAExtremo: proveedor falso en
// HTTP + emisión real de la cola, con el estado final en la base de control.
func TestEmitirPorCGGuardaElControlConSuSerieDeExtremoAExtremo(t *testing.T) {
	ctl := baseDeControlDigital(t)
	db := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db, "10")

	servidor := servidorCGDePrueba(t)
	defer servidor.Close()

	config := `{"code":"cgimprenta","base_url":"` + servidor.URL + `","user_name":"api@negocio.test",` +
		`"user_password":"clave","numero_serie":"A","enabled":true}`
	if _, err := ctl.Exec(`INSERT INTO digital_invoice_config
		(company_id, addon_code, config_json, enabled) VALUES ('empresa1', 'cgimprenta', ?, 1)`, config); err != nil {
		t.Fatalf("no se pudo configurar la imprenta: %v", err)
	}
	if _, err := ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, addon_code, nro_documento, tipo_documento, status)
		VALUES (900, 'empresa1', 'cgimprenta', 900, 'FA', 'pending')`); err != nil {
		t.Fatalf("no se pudo encolar la factura: %v", err)
	}

	s := &Server{controlDB: ctl, controlListo: true}
	s.tenantMu.Lock()
	s.db = db
	s.activeTenant = "empresa1"
	s.tenantMu.Unlock()

	fila := struct {
		id, nroDoc, intentos int64
		invoiceID, tenant    string
	}{id: 1, nroDoc: 900, intentos: 0, invoiceID: "900", tenant: "empresa1"}
	s.diEmitirFila(ctl, fila)

	var estado, control, addon, pdf string
	var intentos int
	if err := ctl.QueryRow(`SELECT status, COALESCE(nro_control_texto,''), addon_code,
			COALESCE(pdf_url,''), intentos FROM digital_invoice_log WHERE id = 1`).
		Scan(&estado, &control, &addon, &pdf, &intentos); err != nil {
		t.Fatalf("no se pudo leer la fila emitida: %v", err)
	}
	if estado != "sent" {
		t.Fatalf("estado = %q (intentos %d)", estado, intentos)
	}
	if control != "00-00000056" {
		t.Errorf("control textual guardado = %q", control)
	}
	if addon != "cgimprenta" {
		t.Errorf("addon_code = %q", addon)
	}
	if !strings.Contains(pdf, "readonly/export_pdf") {
		t.Errorf("pdf_url = %q", pdf)
	}
}

// TestElRenglonGravadoNoSeVuelveExentoPorRedondeo: un IVA de 0,00 por redondeo
// no convierte la venta en exenta. La alícuota manda.
func TestElRenglonGravadoNoSeVuelveExentoPorRedondeo(t *testing.T) {
	db := baseDeFacturacionDigital(t)
	if _, err := db.Exec(`INSERT INTO invoice (id, invoice_id, invoice, customer_id, date,
		total_amount, total_vat_amnt, customer_rif) VALUES (10, 900, 900, 5, '2026-11-27', 100, 0, 'J-12345678-9')`); err != nil {
		t.Fatalf("no se pudo insertar la factura: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name, rif)
		VALUES (5, 'Cliente', 'J-12345678-9')`); err != nil {
		t.Fatalf("no se pudo insertar el cliente: %v", err)
	}
	// Renglón con alícuota 16 % pero IVA calculado en 0,00.
	if _, err := db.Exec(`INSERT INTO invoice_details (invoice_id, product_id, description, quantity,
		rate, total_price, vat_amnt, vat_amnt_per, discount_per, discount_pct)
		VALUES ('10', 'X-1', 'Artículo', 1, 0.01, 0.01, 0, '16.00', '0.00', 0)`); err != nil {
		t.Fatalf("no se pudo insertar el renglón: %v", err)
	}

	s := &Server{}
	doc, err := s.diCargarDocumento(db, "900", 900)
	if err != nil {
		t.Fatalf("no se pudo armar el documento: %v", err)
	}
	if len(doc.Detalles) != 1 {
		t.Fatalf("se esperaba 1 renglón, vinieron %d", len(doc.Detalles))
	}
	if doc.Detalles[0].EsExento {
		t.Error("el renglón tiene alícuota 16 %: no es exento aunque el IVA redondee a 0")
	}
	if doc.Detalles[0].CodigoImpuesto != "G" {
		t.Errorf("código de impuesto = %q, se esperaba G", doc.Detalles[0].CodigoImpuesto)
	}
}

// TestPorcentajeDeTexto: la coma decimal no puede leerse como separador de
// miles (16,00 no es 1600).
func TestPorcentajeDeTexto(t *testing.T) {
	casos := map[string]float64{
		"16.00": 16, "16,00": 16, "16 %": 16, "8.00": 8, "": 0, "31": 31,
	}
	for texto, esperado := range casos {
		if obtenido := porcentajeDeTexto(texto); obtenido != esperado {
			t.Errorf("porcentajeDeTexto(%q) = %v, se esperaba %v", texto, obtenido, esperado)
		}
	}
}

// servidorTHKADePrueba es una imprenta The Factory HKA falsa: autentica, emite
// con su sobre Documentoelectronico y asigna un número de control.
func servidorTHKADePrueba(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Autenticacion":
			_, _ = w.Write([]byte(`{"resultado":"jwt-prueba"}`))
		case "/api/Emision":
			_, _ = w.Write([]byte(`{"codigo":"200","mensaje":"Documento procesado exitosamente",` +
				`"resultado":{"serie":"A","numeroControl":"00-00000056"}}`))
		default:
			// La descarga del PDF no está simulada: el adaptador debe seguir
			// adelante con el número de control, que es lo fiscal.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestEmitirPorTHKAGuardaElControlConSuSerie: el circuito completo del ERP con
// la tercera imprenta (configuración activa → documento → control en la cola).
func TestEmitirPorTHKAGuardaElControlConSuSerie(t *testing.T) {
	ctl := baseDeControlDigital(t)
	db := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db, "10")

	servidor := servidorTHKADePrueba(t)
	defer servidor.Close()

	config := `{"code":"thka","base_url":"` + servidor.URL + `","user_name":"usuario.api",` +
		`"user_password":"clave.api","numero_serie":"A","enabled":true}`
	if _, err := ctl.Exec(`INSERT INTO digital_invoice_config
		(company_id, addon_code, config_json, enabled) VALUES ('empresa1', 'thka', ?, 1)`, config); err != nil {
		t.Fatalf("no se pudo configurar la imprenta: %v", err)
	}
	if _, err := ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, addon_code, nro_documento, tipo_documento, status)
		VALUES (900, 'empresa1', 'thka', 900, 'FA', 'pending')`); err != nil {
		t.Fatalf("no se pudo encolar la factura: %v", err)
	}

	s := &Server{controlDB: ctl, controlListo: true}
	s.tenantMu.Lock()
	s.db = db
	s.activeTenant = "empresa1"
	s.tenantMu.Unlock()

	fila := struct {
		id, nroDoc, intentos int64
		invoiceID, tenant    string
	}{id: 1, nroDoc: 900, intentos: 0, invoiceID: "900", tenant: "empresa1"}
	s.diEmitirFila(ctl, fila)

	var estado, control, addon string
	if err := ctl.QueryRow(`SELECT status, COALESCE(nro_control_texto,''), addon_code
			FROM digital_invoice_log WHERE id = 1`).Scan(&estado, &control, &addon); err != nil {
		t.Fatalf("no se pudo leer la fila emitida: %v", err)
	}
	if estado != "sent" {
		t.Fatalf("estado = %q", estado)
	}
	if control != "00-00000056" {
		t.Errorf("control textual guardado = %q", control)
	}
	if addon != "thka" {
		t.Errorf("addon_code = %q", addon)
	}
}

// TestCadaEmpresaEmiteConSuPropiaImprenta: el modo es POR EMPRESA. Dos empresas
// con imprentas distintas, dos facturas con el mismo número, cada una sale por
// la suya y con su propio número de control.
//
// Es la regla que el dueño del sistema pidió dejar así: una empresa puede tomar
// Unidigital, otra otra distinta; no hay un proveedor único para la instalación.
func TestCadaEmpresaEmiteConSuPropiaImprenta(t *testing.T) {
	ctl := baseDeControlDigital(t)

	db1 := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db1, "10")
	db2 := baseDeFacturacionDigital(t)
	facturaDePrueba(t, db2, "10")

	var emisionesTHKA, emisionesCG int32
	servidorTHKA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Autenticacion":
			_, _ = w.Write([]byte(`{"token":"jwt-thka"}`))
		case "/api/Emision":
			atomic.AddInt32(&emisionesTHKA, 1)
			_, _ = w.Write([]byte(`{"codigo":"200","mensaje":"ok","resultado":{"serie":"A","numeroControl":"00-00000056"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer servidorTHKA.Close()

	servidorCG := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Invoice/create_token_authenticator":
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-cg"}`))
		case "/api/Invoice/add_list_invoice":
			atomic.AddInt32(&emisionesCG, 1)
			// Sin enlace de PDF a propósito: así la prueba no intenta descargar
			// un archivo de un host inventado (el archivado es best-effort).
			_, _ = w.Write([]byte(`{"success":true,"invoice_list_success":[{"invoice_number":"900",` +
				`"control_number":"00-00000101"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer servidorCG.Close()

	configTHKA := `{"code":"thka","base_url":"` + servidorTHKA.URL + `","user_name":"u","user_password":"c","numero_serie":"A","enabled":true}`
	configCG := `{"code":"cgimprenta","base_url":"` + servidorCG.URL + `","user_name":"u","user_password":"c","numero_serie":"A","enabled":true}`
	for _, fila := range []struct{ company, code, config string }{
		{"empresa1", "thka", configTHKA},
		{"empresa2", "cgimprenta", configCG},
	} {
		if _, err := ctl.Exec(`INSERT INTO digital_invoice_config
			(company_id, addon_code, config_json, enabled) VALUES (?, ?, ?, 1)`,
			fila.company, fila.code, fila.config); err != nil {
			t.Fatalf("no se pudo configurar %s: %v", fila.company, err)
		}
	}
	for _, fila := range []struct{ company, code string }{
		{"empresa1", "thka"},
		{"empresa2", "cgimprenta"},
	} {
		if _, err := ctl.Exec(`INSERT INTO digital_invoice_log
			(invoice_id, company_id, addon_code, nro_documento, tipo_documento, status)
			VALUES (900, ?, ?, 900, 'FA', 'pending')`, fila.company, fila.code); err != nil {
			t.Fatalf("no se pudo encolar %s: %v", fila.company, err)
		}
	}

	s := &Server{controlDB: ctl, controlListo: true}
	s.tenantMu.Lock()
	s.db = db1
	s.activeTenant = "empresa1"
	s.tenantDBs = map[string]*sql.DB{"empresa1": db1, "empresa2": db2}
	s.tenantMu.Unlock()

	for id := int64(1); id <= 2; id++ {
		var tenant string
		if err := ctl.QueryRow(`SELECT company_id FROM digital_invoice_log WHERE id=?`, id).Scan(&tenant); err != nil {
			t.Fatalf("no se pudo leer la fila %d: %v", id, err)
		}
		fila := struct {
			id, nroDoc, intentos int64
			invoiceID, tenant    string
		}{id: id, nroDoc: 900, intentos: 0, invoiceID: "900", tenant: tenant}
		s.diEmitirFila(ctl, fila)
	}

	if emisionesTHKA != 1 || emisionesCG != 1 {
		t.Fatalf("cada imprenta debía recibir UNA factura: THKA=%d, CG=%d", emisionesTHKA, emisionesCG)
	}

	controles := map[string]string{}
	rows, err := ctl.Query(`SELECT company_id, status, COALESCE(nro_control_texto,'') FROM digital_invoice_log`)
	if err != nil {
		t.Fatalf("no se pudo leer la cola: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var company, estado, control string
		if rows.Scan(&company, &estado, &control) != nil {
			continue
		}
		if estado != "sent" {
			t.Errorf("la factura de %s quedó %q", company, estado)
		}
		controles[company] = control
	}
	if controles["empresa1"] != "00-00000056" {
		t.Errorf("empresa1 debía emitir por THKA (control 00-00000056), quedó %q", controles["empresa1"])
	}
	if controles["empresa2"] != "00-00000101" {
		t.Errorf("empresa2 debía emitir por CG (control 00-00000101), quedó %q", controles["empresa2"])
	}
}

// TestUnaVentaDelTerminalSeEncolaParaSuEmpresa: la venta que registra el motor
// del terminal (HTTP-sync y gRPC pasan por aquí) entra a la cola de la empresa
// de ESA base, con la imprenta que tenga activa.
//
// El defecto que esta prueba impide que vuelva: el gRPC registraba la factura
// pero no la encolaba, así que las cajas que hablaban gRPC no emitían por
// facturación digital y nadie se enteraba.
func TestUnaVentaDelTerminalSeEncolaParaSuEmpresa(t *testing.T) {
	s, db := baseTerminalPrueba(t)
	// En una instalación real la cola vive en la base de CONTROL, no en la de la
	// empresa: se monta así para probar el circuito de verdad (y porque la tabla
	// que crean las migraciones de SQLite no admite el id automático).
	ctl := baseDeControlDigital(t)

	s.tenantMu.Lock()
	s.activeTenant = "empresa1"
	s.db = db
	s.tenantMu.Unlock()
	s.controlDB = ctl
	s.controlListo = true

	if _, err := ctl.Exec(`INSERT INTO digital_invoice_config (company_id, addon_code, config_json, enabled)
		VALUES ('empresa1', 'cgimprenta', '{"code":"cgimprenta"}', 1)`); err != nil {
		t.Fatalf("no se pudo configurar la imprenta: %v", err)
	}

	respuesta, err := s.registrarVentaTerminal(db, ventaTerminalBasica())
	if err != nil {
		t.Fatalf("la venta del terminal falló: %v", err)
	}
	if respuesta == nil || !respuesta.Success {
		t.Fatalf("la venta no se registró: %+v", respuesta)
	}

	var filas int
	var estado, addon, company string
	var invoiceID int64
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM digital_invoice_log`).Scan(&filas); err != nil {
		t.Fatalf("no se pudo contar la cola: %v", err)
	}
	if filas != 1 {
		t.Fatalf("la venta del terminal debía encolar UNA factura, hay %d", filas)
	}
	if err := ctl.QueryRow(`SELECT company_id, addon_code, status, invoice_id FROM digital_invoice_log`).
		Scan(&company, &addon, &estado, &invoiceID); err != nil {
		t.Fatalf("no se pudo leer la fila encolada: %v", err)
	}
	if company != "empresa1" {
		t.Errorf("la fila debe ser de la empresa de la base, quedó %q", company)
	}
	if addon != "cgimprenta" {
		t.Errorf("la fila debe llevar la imprenta activa, quedó %q", addon)
	}
	if estado != "pending" {
		t.Errorf("estado = %q", estado)
	}
	if invoiceID != enteroDeTexto(respuesta.RemoteID) {
		t.Errorf("invoice_id = %d, se esperaba %s", invoiceID, respuesta.RemoteID)
	}
}

// TestSeResuelveLaEmpresaDeLaBaseDelTerminal: el motor de venta recibe la base
// pero no el código de empresa; esta es la pieza que lo traduce y de la que
// depende que cada venta se encole en la empresa correcta.
func TestSeResuelveLaEmpresaDeLaBaseDelTerminal(t *testing.T) {
	dbActiva := baseDeFacturacionDigital(t)
	dbOtra := baseDeFacturacionDigital(t)
	dbMaestra := baseDeFacturacionDigital(t)
	dbDesconocida := baseDeFacturacionDigital(t)

	s := &Server{}
	s.tenantMu.Lock()
	s.db = dbActiva
	s.activeTenant = "empresa1"
	s.tenantDBs = map[string]*sql.DB{"empresa2": dbOtra}
	s.tenantMu.Unlock()
	s.controlDB = dbMaestra
	s.config = &configs.Config{DBMasterTenantCode: "central"}

	casos := []struct {
		nombre string
		db     *sql.DB
		espera string
	}{
		{"la empresa activa", dbActiva, "empresa1"},
		{"otra empresa abierta", dbOtra, "empresa2"},
		{"la empresa maestra (base de control)", dbMaestra, "central"},
		{"una base que no es de ninguna empresa", dbDesconocida, ""},
		{"sin base", nil, ""},
	}
	for _, caso := range casos {
		if obtenido := s.tenantCodeDeDB(caso.db); obtenido != caso.espera {
			t.Errorf("%s: tenantCodeDeDB = %q, se esperaba %q", caso.nombre, obtenido, caso.espera)
		}
	}
}

// servidorCGDePrueba es una imprenta CG falsa: pide token y emite la factura// que le manden con un control con serie. No valida nada: lo que se prueba aquí
// es el circuito del ERP (documento → proveedor → fila), no CG otra vez.
func servidorCGDePrueba(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Invoice/create_token_authenticator":
			_, _ = w.Write([]byte(`{"success":true,"message":"Token de Acceso","token":"jwt-prueba"}`))
		case "/api/Invoice/add_list_invoice":
			cuerpo, _ := io.ReadAll(r.Body)
			var pedido struct {
				Facturas []struct {
					Numero string `json:"numeroFactura"`
				} `json:"facturas"`
			}
			_ = json.Unmarshal(cuerpo, &pedido)
			numero := "0"
			if len(pedido.Facturas) == 1 {
				numero = pedido.Facturas[0].Numero
			}
			_, _ = w.Write([]byte(`{"success":true,"message":"Facturas guardadas satisfactoriamente.",` +
				`"invoice_list_success":[{"invoice_number":"` + numero + `",` +
				`"control_number":"00-00000056",` +
				`"invoice_pdf":"https://cg.example/readonly/export_pdf/abc-123"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"message":"ruta no simulada"}`))
		}
	}))
}
