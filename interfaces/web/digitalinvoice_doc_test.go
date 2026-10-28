package web

// Pruebas de la entrega del documento fiscal digital al cliente.
//
// Son pruebas de SENTENCIAS REALES contra SQLite en memoria, por el mismo
// motivo que las del libro de avisos: lo que hay que demostrar aquí no es una
// función pura, es que la factura emitida llega al cliente UNA vez, con el
// texto correcto y al correo de la ficha — y que cuando no debe salir, no sale.
// Ninguna de esas cosas se puede comprobar con un mock.
//
// El SMTP es un doble falso que contesta lo que un cliente SMTP espera y
// guarda el mensaje: no intenta entregar a nadie. El punto de la prueba no es
// la red, es que el sistema llame a la puerta correcta una sola vez.

import (
	"bufio"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// nuevaBaseDeControl crea la tabla de la facturación digital, que vive en la
// base de CONTROL y no en la de la empresa.
func nuevaBaseDeControl(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base de control: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := `CREATE TABLE digital_invoice_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		invoice_id INTEGER NOT NULL,
		company_id TEXT NOT NULL DEFAULT '1',
		addon_code TEXT NOT NULL DEFAULT '',
		strong_id TEXT, nro_control INTEGER, nro_documento INTEGER NOT NULL DEFAULT 0,
		tipo_documento TEXT NOT NULL DEFAULT 'FA', serie TEXT NOT NULL DEFAULT '0',
		status TEXT NOT NULL DEFAULT 'pending',
		pdf_url TEXT, xml_url TEXT, error_message TEXT,
		intentos INTEGER NOT NULL DEFAULT 0, ultimo_intento TEXT,
		created_at TEXT, updated_at TEXT)`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("no se pudo crear digital_invoice_log: %v", err)
	}
	return db
}

// nuevaBaseDeControlConCopia es la misma tabla YA con la columna de la copia
// propia: es la base que deja el esquema de esta versión.
func nuevaBaseDeControlConCopia(t *testing.T) *sql.DB {
	t.Helper()
	db := nuevaBaseDeControl(t)
	if _, err := db.Exec(`ALTER TABLE digital_invoice_log ADD COLUMN pdf_path TEXT`); err != nil {
		t.Fatalf("no se pudo añadir pdf_path: %v", err)
	}
	return db
}

// completarFacturaDigital añade a la base de la empresa las columnas y las
// tablas que la entrega lee y que la ficha base de pruebas no trae. Se llama
// UNA vez por base, ANTES de insertar las facturas.
func completarFacturaDigital(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, ddl := range []string{
		`ALTER TABLE invoice ADD COLUMN control_no TEXT`,
		`ALTER TABLE invoice ADD COLUMN currency_code TEXT`,
		`ALTER TABLE invoice ADD COLUMN total_vat_amnt REAL DEFAULT 0`,
		`ALTER TABLE invoice ADD COLUMN customer_name TEXT`,
		`ALTER TABLE company_information ADD COLUMN upload_path TEXT`,
		`CREATE TABLE fiscal_setting (` + "`key`" + ` TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, action TEXT, module TEXT,
			table_name TEXT, record_id TEXT, old_value TEXT, new_value TEXT, ip_address TEXT,
			created_at TEXT, prev_hash TEXT, record_hash TEXT)`,
		`INSERT INTO company_information (company_id, company_name, status)
			VALUES ('1', 'Ferretería El Tornillo', 1)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("no se pudo preparar la base: %v\n%s", err, ddl)
		}
	}
}

// completarDocumentoFiscal pone los datos fiscales de UNA factura ya creada:
// control, moneda e IVA, que son los que el correo necesita para redactarse.
func completarDocumentoFiscal(t *testing.T, db *sql.DB, invoicePK int64) {
	t.Helper()
	if _, err := db.Exec(`UPDATE invoice SET control_no = '00-00000010', currency_code = 'VES', total_vat_amnt = 160
		WHERE invoice_id = ?`, invoicePK); err != nil {
		t.Fatalf("no se pudo completar la factura: %v", err)
	}
}

// insertarDocDigital registra el documento que la imprenta devolvió, o nada si
// pdf y xml vienen vacíos.
func insertarDocDigital(t *testing.T, ctl *sql.DB, correlativo int64, pdf, xml string) {
	t.Helper()
	_, err := ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, status, strong_id, nro_control, pdf_url, xml_url)
		VALUES (?,?, 'sent', 'abc-123', 98765432, ?, ?)`, correlativo, "empresa1", pdf, xml)
	if err != nil {
		t.Fatalf("no se pudo registrar el documento digital: %v", err)
	}
}

// ---------- El doble SMTP ----------

type smtpFalso struct {
	ln       net.Listener
	mensajes chan string
}

// nuevoSMTPFalso arranca un servidor mínimo en 127.0.0.1 y devuelve el canal
// donde cae cada mensaje aceptado.
func nuevoSMTPFalso(t *testing.T) *smtpFalso {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no se pudo abrir el doble de SMTP: %v", err)
	}
	s := &smtpFalso{ln: ln, mensajes: make(chan string, 8)}
	go s.atender()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *smtpFalso) direccion() (host string, puerto int) {
	h, p, err := net.SplitHostPort(s.ln.Addr().String())
	if err != nil {
		return "127.0.0.1", 0
	}
	_, _ = fmt.Sscanf(p, "%d", &puerto)
	return h, puerto
}

func (s *smtpFalso) atender() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			lector := bufio.NewReader(c)
			escritor := bufio.NewWriter(c)
			_, _ = escritor.WriteString("220 erpgow ESMTP\r\n")
			_ = escritor.Flush()

			var datos strings.Builder
			enDatos := false
			for {
				linea, err := lector.ReadString('\n')
				if err != nil {
					return
				}
				linea = strings.TrimRight(linea, "\r\n")
				if enDatos {
					if linea == "." {
						enDatos = false
						select {
						case s.mensajes <- datos.String():
						default:
						}
						_, _ = escritor.WriteString("250 2.0.0 ok\r\n")
						_ = escritor.Flush()
						continue
					}
					datos.WriteString(linea + "\n")
					continue
				}
				cab := strings.ToUpper(linea)
				switch {
				case strings.HasPrefix(cab, "EHLO"), strings.HasPrefix(cab, "HELO"):
					_, _ = escritor.WriteString("250 erpgow\r\n")
				case strings.HasPrefix(cab, "DATA"):
					enDatos = true
					_, _ = escritor.WriteString("354 el final es un punto solo\r\n")
				case strings.HasPrefix(cab, "QUIT"):
					_, _ = escritor.WriteString("221 2.0.0 cerrando\r\n")
					_ = escritor.Flush()
					return
				default:
					_, _ = escritor.WriteString("250 2.1.0 ok\r\n")
				}
				_ = escritor.Flush()
			}
		}(conn)
	}
}

// ---------- Las pruebas ----------

// TestLaFacturaEmitidaSeEntregaAlClienteUnaSolaVez es el circuito completo:
// factura emitida → correo al cliente de la ficha → traza en finance_message y
// en la auditoría → y una SEGUNDA llamada igual no manda nada.
func TestLaFacturaEmitidaSeEntregaAlClienteUnaSolaVez(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	ctl := nuevaBaseDeControl(t)
	smtp := nuevoSMTPFalso(t)
	s := &Server{db: db}
	completarFacturaDigital(t, db)

	if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name, customer_email, customer_mobile)
		VALUES (1, 'Ferretería El Tornillo', 'pagos@tornillo.test', '04140000000')`); err != nil {
		t.Fatal(err)
	}
	finInsertarFactura(t, db, 10, 1, time.Now().Format("2006-01-02"), 1160, 1160)
	completarDocumentoFiscal(t, db, 10)
	insertarDocDigital(t, ctl, 10, "https://docs.test/factura-10.pdf", "https://docs.test/factura-10.xml")

	if _, err := db.Exec(`INSERT INTO fiscal_setting (`+"`key`"+`, value) VALUES
		('invoice_delivery','1'), ('invoice_delivery_channel','1')`); err != nil {
		t.Fatal(err)
	}
	host, puerto := smtp.direccion()
	if _, err := db.Exec(`INSERT INTO notify_channel (kind, name, is_active, ai_allowed, cooldown_days,
		host, port, from_address, from_name, security, created_at, updated_at)
		VALUES ('email','Correo de facturación',1,0,0,?,?, 'facturas@erpgow.test', 'Ferretería', 'none', ?, ?)`,
		host, puerto, now(), now()); err != nil {
		t.Fatal(err)
	}

	peticion := &http.Request{}
	res, err := s.diEntregar(peticion, db, ctl, "empresa1", 10, false)
	if err != nil {
		t.Fatalf("la entrega falló: %v", err)
	}
	if enviado, _ := res["enviado"].(bool); !enviado {
		t.Fatalf("la factura no se entregó: %v", res)
	}
	if res["destino"] != "pagos@tornillo.test" {
		t.Errorf("el destinatario sale de la ficha del cliente; salió %v", res["destino"])
	}

	// El correo llegó, y dice lo que tiene que decir.
	var mensaje string
	select {
	case mensaje = <-smtp.mensajes:
	case <-time.After(3 * time.Second):
		t.Fatal("el correo no llegó al servidor")
	}
	for _, buscabajo := range []string{
		"pagos@tornillo.test", "https://docs.test/factura-10.pdf",
		"00-00000010", "Ferretería El Tornillo", "Subject:",
	} {
		if !strings.Contains(mensaje, buscabajo) {
			t.Errorf("el correo no contiene %q:\n%s", buscabajo, mensaje)
		}
	}

	// La traza: mensaje registrado y movimiento encadenado en auditoría.
	var refType, refID, estado, destino string
	if err := db.QueryRow(`SELECT ref_type, ref_id, status, recipient FROM finance_message ORDER BY id DESC LIMIT 1`).
		Scan(&refType, &refID, &estado, &destino); err != nil {
		t.Fatalf("no quedó registro del mensaje: %v", err)
	}
	if refType != "invoice" || refID != "10" || estado != "enviado" || destino != "pagos@tornillo.test" {
		t.Errorf("la traza del mensaje quedó mal: %s/%s %s a %s", refType, refID, estado, destino)
	}
	var acciones int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'send' AND record_id = '10'`).
		Scan(&acciones); err != nil || acciones == 0 {
		t.Errorf("el envío no quedó en la cadena de auditoría (hay %d filas, error %v)", acciones, err)
	}

	// Segunda llamada idéntica: no sale nada y no se duplica la entrega.
	res2, err := s.diEntregar(peticion, db, ctl, "empresa1", 10, false)
	if err != nil {
		t.Fatalf("la segunda entrega devolvió error: %v", err)
	}
	if enviado, _ := res2["enviado"].(bool); enviado {
		t.Errorf("el mismo correo salió dos veces: %v", res2)
	}
	select {
	case <-smtp.mensajes:
		t.Error("llegó un segundo correo al servidor")
	case <-time.After(300 * time.Millisecond):
	}
	var mensajes int
	_ = db.QueryRow(`SELECT COUNT(*) FROM finance_message WHERE ref_type = 'invoice'`).Scan(&mensajes)
	if mensajes != 1 {
		t.Errorf("se esperaba UN registro de mensaje, hay %d", mensajes)
	}
}

// TestLaEntregaSeNiegaCuandoNoHayQueEntregar cubre las tres puertas que deben
// cerrar el envío: no hay documento digital, el enlace del proveedor no es
// navegable y el cliente no tiene correo. En los tres casos el motivo tiene que
// ser legible, porque esa frase es la que va a leer el administrador.
func TestLaEntregaSeNiegaCuandoNoHayQueEntregar(t *testing.T) {
	casos := []struct {
		nombre   string
		doc      bool
		pdf      string
		xml      string
		sinCorreo bool
		esperado string
	}{
		{
			nombre:   "sin documento digital",
			esperado: "no salió por facturación digital",
		},
		{
			nombre: "enlace del proveedor no navegable",
			doc:    true, pdf: "javascript:alert(1)", xml: "",
			esperado: "aún no devuelve un enlace válido",
		},
		{
			nombre: "cliente sin correo",
			doc:    true, pdf: "https://docs.test/x.pdf", xml: "",
			sinCorreo: true,
			esperado:  "no tiene correo en su ficha",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			db := nuevaBaseDeCartera(t)
			ctl := nuevaBaseDeControl(t)
			s := &Server{db: db}
			completarFacturaDigital(t, db)

			correo := "pagos@tornillo.test"
			if c.sinCorreo {
				correo = ""
			}
			if _, err := db.Exec(`INSERT INTO customer_information (customer_id, customer_name, customer_email)
				VALUES (1, 'Ferretería El Tornillo', ?)`, correo); err != nil {
				t.Fatal(err)
			}
			finInsertarFactura(t, db, 11, 1, time.Now().Format("2006-01-02"), 100, 100)
			completarDocumentoFiscal(t, db, 11)
			if c.doc {
				insertarDocDigital(t, ctl, 11, c.pdf, c.xml)
			}
			// El ajuste y el canal hacen falta para llegar hasta la última puerta y
			// no estorban en las otras.
			if _, err := db.Exec(`INSERT INTO fiscal_setting (` + "`key`" + `, value) VALUES
				('invoice_delivery','1'), ('invoice_delivery_channel','1')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO notify_channel (kind, name, is_active, cooldown_days,
				host, port, from_address, security, created_at, updated_at)
				VALUES ('email','Correo',1,0,'localhost',25,'facturas@erpgow.test','none', ?, ?)`, now(), now()); err != nil {
				t.Fatal(err)
			}

			_, err := s.diEntregar(nil, db, ctl, "empresa1", 11, false)
			if err == nil {
				t.Fatalf("la entrega debía negarse con %q y no lo hizo", c.esperado)
			}
			if !strings.Contains(err.Error(), c.esperado) {
				t.Errorf("el motivo no explica el problema: lo pedido era %q y salió %q", c.esperado, err.Error())
			}
			// Ninguna de las tres puertas debe dejar rastro de envío.
			var mensajes int
			_ = db.QueryRow(`SELECT COUNT(*) FROM finance_message`).Scan(&mensajes)
			if mensajes != 0 {
				t.Errorf("se negó el envío pero quedaron %d registros de mensaje", mensajes)
			}
		})
	}
}

// TestElCanalElegidoMandaYSinCanalSeDice comprueba el ajuste: con canal
// nombrado se usa ese; apagado se niega en vez de caer a otro en silencio; y
// sin ningún canal de correo activo se dice claro.
func TestElCanalElegidoMandaYSinCanalSeDice(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	completarFacturaDigital(t, db)
	s := &Server{db: db}

	if _, err := db.Exec(`INSERT INTO notify_channel (kind, name, is_active, cooldown_days,
		host, port, from_address, security, created_at, updated_at)
		VALUES ('sms','SMS de cobranza',1,3,'',0,'','none', ?, ?), ('email','Correo de cobranza',1,0,'h',25,'a@b.test','none', ?, ?)`,
		now(), now(), now(), now()); err != nil {
		t.Fatal(err)
	}

	// Sin ajuste: nada de entrega.
	if activa, _ := s.diEntregaConfig(db); activa {
		t.Error("sin ajuste guardado la entrega no puede estar activa")
	}
	// Sin canal elegido, el primero de correo —nunca el SMS.
	canal, err := s.diCanalEntrega(db, 0)
	if err != nil {
		t.Fatalf("sin canal elegido debía elegir el de correo: %v", err)
	}
	if canal.Kind != "email" {
		t.Errorf("el canal elegido fue %q (%s); la factura sale por correo", canal.Kind, canal.Name)
	}

	// Canal elegido nombrado: se lee tal cual.
	if _, err := db.Exec(`INSERT INTO fiscal_setting (` + "`key`" + `, value) VALUES ('invoice_delivery_channel','2')`); err != nil {
		t.Fatal(err)
	}
	activa, elegido := s.diEntregaConfig(db)
	if activa || elegido != 2 {
		t.Errorf("la lectura del ajuste salió activa=%v canal=%d", activa, elegido)
	}
	elegidoCanal, err := s.diCanalEntrega(db, elegido)
	if err != nil || elegidoCanal.Name != "Correo de cobranza" {
		t.Errorf("el canal elegido no fue el nombrado: %v / %v", elegidoCanal.Name, err)
	}

	// Apagar el canal elegido sí se niega: no se cae a otro en silencio.
	if _, err := db.Exec(`UPDATE notify_channel SET is_active=0 WHERE id = ?`, elegido); err != nil {
		t.Fatal(err)
	}
	if _, err := s.diCanalEntrega(db, elegido); err == nil {
		t.Error("un canal apagado no puede ser el canal de entrega")
	}
	// Sin canales de correo activos, se dice claro.
	if _, err := db.Exec(`UPDATE notify_channel SET is_active=0 WHERE kind='email'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.diCanalEntrega(db, 0); err == nil {
		t.Error("sin canales de correo debía devolver error")
	}
}

// TestLosEnlacesDelProveedorSeValidanAntesDePintarse es la regla de seguridad
// del archivo: es un dato externo que viaja al navegador.
func TestLosEnlacesDelProveedorSeValidanAntesDePintarse(t *testing.T) {
	casos := map[string]string{
		"https://docs.test/factura.pdf": "https://docs.test/factura.pdf",
		"http://docs.test/factura.pdf":  "http://docs.test/factura.pdf",
		"javascript:alert(1)":           "",
		"data:text/html,<script>":       "",
		"//docs.test/factura.pdf":       "",
		"https://docs.test/a\"b":        "",
		"https://docs.test/<x>":         "",
		"":                              "",
	}
	for entrada, esperado := range casos {
		if got := diURLSegura(entrada); got != esperado {
			t.Errorf("diURLSegura(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}

// TestElCuerpoDelCorreoDiceLoQueElSistemaSabe revisa el texto: número,
// control, total, enlace y la firma de la empresa, sin que nadie lo teclee.
func TestElCuerpoDelCorreoDiceLoQueElSistemaSabe(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	s := &Server{db: db}
	completarFacturaDigital(t, db)

	asunto, cuerpo := s.diCuerpoEntrega(db, "42", "00-00000042", "2026-10-28",
		"Ferretería El Tornillo", 1160, 160, "VES", "https://docs.test/42.pdf", "")

	if !strings.Contains(asunto, "42") || !strings.Contains(asunto, "Ferretería El Tornillo") {
		t.Errorf("el asunto no nombra la factura ni la empresa: %q", asunto)
	}
	for _, buscabajo := range []string{"00-00000042", "1160.00", "160.00", "https://docs.test/42.pdf", "VES"} {
		if !strings.Contains(cuerpo, buscabajo) {
			t.Errorf("el cuerpo no contiene %q:\n%s", buscabajo, cuerpo)
		}
	}
}