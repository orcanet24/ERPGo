package web

// digitalinvoice_processor.go — cola de emisión de facturación digital.
//
// Para qué existe: el POS puede facturar OFFLINE (la caja ya cobró e imprimió)
// y necesita que al volver la conexión la factura llegue a la imprenta digital
// que la empresa tenga activa (Unidigital, CG Imprenta Digital o la que venga).
// También las ventas sincronizadas por gRPC y las facturas hechas desde el
// panel pasan por aquí.
//
// Cómo funciona: cada venta apunta una fila pending en digital_invoice_log.
// Un runner en segundo plano (arranca con el ERP) toma pendientes, monta el
// DatosDocumento con los datos REALES de la factura y los renglones, emite al
// proveedor y marca sent; si falla sube intentos con límite y deja
// error_message para el panel. Reintenta solo: nadie tiene que apretar nada.
//
// LA REGLA DEL MODO FISCAL. Si la empresa no tiene ninguna imprenta digital
// activa, factura por impresora fiscal y la venta NO se encola: encolarla
// crearía un documento que nunca se transmite y que, el día que se active una
// imprenta, se emitiría con números de control nuevos sobre una factura que ya
// salió impresa. La venta a impresora fiscal se queda donde pertenece.
//
// La exclusividad quedó en digital_invoice_config (enabled=1 única por
// empresa): el runner NO usa el registry en memoria para emitir, sino la
// configuración persistida — así un reinicio del ERP emite con el proveedor
// correcto sin pasos manuales—.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	di "erpgo/interfaces/web/digitalinvoice"
	"erpgo/internal/digitalinvoice"
)

// ------------- ENCOLADO -------------

// diEnqueueVenta registra una venta para su emisión digital. Idempotente por
// (company_id, invoice_id) con la fila pending SIEMPRE en la base de CONTROL:
// ahí vive digital_invoice_log, y ahí el runner la encuentra aunque la caja sea
// de una empresa distinta a la activa.
func (s *Server) diEnqueueVenta(db *sql.DB, tenant, invoiceID string) {
	if db == nil || strings.TrimSpace(tenant) == "" || strings.TrimSpace(invoiceID) == "" {
		return
	}
	ctl, err := s.controlStore()
	if err != nil {
		log.Printf("facturación digital: sin base de control para encolar %s/%s: %v", tenant, invoiceID, err)
		return
	}

	// La empresa factura por impresora fiscal mientras no haya una imprenta
	// digital activa. Se consulta ANTES de leer la factura: sin proveedor no
	// hay documento digital que producir.
	code, err := diProveedorActivoEnBase(ctl, tenant)
	if err != nil {
		log.Printf("facturación digital: no se pudo leer la imprenta activa de %s: %v", tenant, err)
		return
	}
	if code == "" {
		return
	}

	// Datos del documento desde la base de la EMPRESA. El correlativo numérico
	// va en `invoice`; `control_no` es TEXTO ("00-00000001" en línea,
	// "CONT-T01-000123" en contingencia).
	//
	// ANTES se leía `control_no` en un sql.NullInt64 y por eso NINGUNA factura
	// del POS se encolaba: el scan fallaba con "converting string to int64",
	// el error solo se logueaba y la venta quedaba cobrada sin transmitir
	// nunca. El número que el proveedor necesita como correlativo es el de la
	// factura, no el control.
	var correlativo int64
	err = db.QueryRow(`SELECT COALESCE(invoice, 0) FROM invoice WHERE invoice = ? LIMIT 1`, invoiceID).Scan(&correlativo)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("facturación digital: lectura de factura %s/%s: %v", tenant, invoiceID, err)
		}
		return
	}

	// UNA fila por venta, en cualquier estado. Dos filas de la misma factura
	// son dos emisiones y dos números de control fiscales para el mismo
	// documento: el duplicado que la ley prohíbe. Si la fila existe y no se
	// transmitió, se devuelve a la cola; si ya se transmitió, no se toca.
	var filaID int64
	var estado string
	err = ctl.QueryRow(`SELECT id, COALESCE(status,'') FROM digital_invoice_log
		WHERE company_id = ? AND invoice_id = ? ORDER BY id DESC LIMIT 1`, tenant, correlativo).
		Scan(&filaID, &estado)
	switch {
	case err == nil:
		if estado == "sent" {
			return
		}
		if _, err := ctl.Exec(`UPDATE digital_invoice_log
			SET status='pending', intentos=0, ultimo_intento=NULL, error_message=NULL,
			    addon_code=?, nro_documento=?
			WHERE id=?`, code, correlativo, filaID); err != nil {
			log.Printf("facturación digital: no se pudo devolver a la cola %s/%s: %v", tenant, invoiceID, err)
		}
		return
	case err != sql.ErrNoRows:
		log.Printf("facturación digital: consultando la cola de %s/%s: %v", tenant, invoiceID, err)
		return
	}

	_, err = ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, addon_code, strong_id, nro_documento, tipo_documento, serie, status)
		VALUES (?, ?, ?, NULL, ?, 'FA', '', 'pending')`,
		correlativo, tenant, code, correlativo)
	if err != nil && !diIsDupKey(err) {
		log.Printf("facturación digital: encolando %s/%s: %v", tenant, invoiceID, err)
	}
}

// enteroDeTexto saca el valor numérico de un identificador de factura
// ("0000123" → 123). Los dígitos se acumulan solo hasta 18 para no desbordar.
func enteroDeTexto(s string) int64 {
	var n int64
	digitos := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			continue
		}
		n = n*10 + int64(r-'0')
		digitos++
		if digitos == 18 {
			break
		}
	}
	return n
}

// diIsDupKey identifica el insert concurrente de la misma factura.
func diIsDupKey(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Duplicate entry")
}

// ------------- RUNNER -------------

// diEstadoRunner regula la ejecución: 30 s es un intervalo corto para el flujo
// diario y no se satura el proveedor.
const diIntervaloRunner = 30 * time.Second

// diMaxIntentos es el número límite de reintento por fila: si no emite en
// este rango, algo estructural falla (config rota, proveedor caído, cliente sin
// correo) y que quede 'failed' avisa al admin en vez de reintentar para
// siempre. Corregida la causa, el botón «Reintentar» del panel la devuelve a la
// cola.
const diMaxIntentos = 8

// startDigitalInvoiceRunner lanza el ciclo que emite pendientes. Un fallo aquí
// no impide arrancar el ERP: sin runner, las facturas quedan pending y no
// llegan al proveedor, pero se venden igual.
func (s *Server) startDigitalInvoiceRunner() {
	s.diStop = make(chan struct{})
	go func() {
		t := time.NewTicker(diIntervaloRunner)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.diProcesarPendientes()
			case <-s.diStop:
				return
			}
		}
	}()
	log.Printf("facturación digital: runner activo cada %v", diIntervaloRunner)
}

// diDesactivarRunner deja de emitir (llamado al cerrar el ERP).
func (s *Server) diDesactivarRunner() {
	if s.diStop != nil {
		close(s.diStop)
		s.diStop = nil
	}
}

// diProcesarPendientes es una pasada del runner: toma pendientes de la base de
// control y los emite a su proveedor correspondiente.
func (s *Server) diProcesarPendientes() {
	ctl, err := s.controlStore()
	if err != nil {
		return
	}
	rows, err := ctl.Query(`SELECT id, invoice_id, company_id, nro_documento, intentos
		FROM digital_invoice_log
		WHERE status = 'pending'
		  AND (ultimo_intento IS NULL OR ultimo_intento < NOW() - INTERVAL 1 MINUTE)
		ORDER BY id LIMIT 5`)
	if err != nil {
		return
	}
	defer rows.Close()

	type fila struct {
		id, nroDoc, intentos int64
		invoiceID, tenant    string
	}
	var pendientes []fila
	for rows.Next() {
		var f fila
		if err := rows.Scan(&f.id, &f.invoiceID, &f.tenant, &f.nroDoc, &f.intentos); err != nil {
			continue
		}
		pendientes = append(pendientes, f)
	}
	rows.Close()

	for _, f := range pendientes {
		s.diEmitirFila(ctl, f)
	}
}

// diEmitirFila emite UN documento previsto. Devuelve el error en el log.
func (s *Server) diEmitirFila(ctl *sql.DB, f struct {
	id, nroDoc, intentos int64
	invoiceID, tenant    string
}) {
	// 1) La empresa tiene proveedor habilitado en su config...
	var raw string
	err := ctl.QueryRow(`SELECT config_json FROM digital_invoice_config
		WHERE company_id = ? AND enabled = 1 ORDER BY addon_code LIMIT 1`, f.tenant).Scan(&raw)
	if err == sql.ErrNoRows {
		// La empresa quedó sin imprenta digital activa (pasó a impresora
		// fiscal) pero este documento SE VENDIÓ como digital: no se puede
		// transmitir ni se puede descartar en silencio. Se cuenta como intento
		// fallido con el motivo, para que aparezca en el panel y el
		// administrador decida (volver a activar una imprenta o reemitir).
		s.diRegistrarFallo(ctl, f.id, f.intentos,
			fmt.Errorf("la empresa no tiene ninguna imprenta digital activa: el documento no se puede transmitir hasta que active una"))
		return
	}
	if err != nil {
		log.Printf("facturación digital fila %d: config de %s: %v", f.id, f.tenant, err)
		return
	}
	var cfg di.ProviderConfig
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		log.Printf("facturación digital fila %d: config_json dañado de %s", f.id, f.tenant)
		return
	}
	provider, err := digitalinvoice.NuevoProveedorActivo(cfg.Code, cfg)
	if err != nil {
		log.Printf("facturación digital fila %d: proveedor %s: %v", f.id, cfg.Code, err)
		return
	}

	// 2) Los datos COMPLETOS de la factura en la empresa del documento.
	db := s.dbForTenant(f.tenant)
	if db == nil {
		log.Printf("facturación digital fila %d: base %s no disponible", f.id, f.tenant)
		return
	}
	doc, err := s.diCargarDocumento(db, f.invoiceID, int(f.nroDoc))
	if err != nil {
		s.diRegistrarFallo(ctl, f.id, f.intentos, err)
		return
	}

	// 3) Emitir. Presupuesto 30 s por documento: si el proveedor está caído,
	// el timeout lo corta y el runner reintenta más tarde.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := provider.Emitir(ctx, doc)
	if err != nil {
		s.diRegistrarFallo(ctl, f.id, f.intentos, err)
		return
	}

	// 4) Estado en la fila. La imprenta que emitió queda escrita (addon_code):
	// es lo que permite auditar después con qué casa salió cada documento,
	// incluso si la empresa cambió de imprenta al día siguiente.
	//
	// strong_id y nro_control se guardan como NULL cuando no vienen: la tabla
	// tiene índice único sobre los dos, y un "" o un 0 repetido haría fallar la
	// SEGUNDA factura de cualquier proveedor que no devuelva ese dato (CG no
	// devuelve número de control entero, devuelve "00-00000056").
	estado := "sent"
	consulta := `UPDATE digital_invoice_log
		SET status=?, addon_code=?, strong_id=NULLIF(?,''), nro_control=NULLIF(?,0),
		    pdf_url=NULLIF(?,''), xml_url=NULLIF(?,''), error_message=NULL`
	args := []interface{}{estado, cfg.Code, res.StrongID, res.NroControl, res.PDFURL, res.XMLURL}
	if diColumnaControlTexto(ctl) {
		consulta += `, nro_control_texto=NULLIF(?,'')`
		args = append(args, res.NroControlTexto)
	}
	consulta += ` WHERE id=?`
	args = append(args, f.id)
	if _, err := ctl.Exec(consulta, args...); err != nil {
		log.Printf("facturación digital fila %d: actualización: %v", f.id, err)
		return
	}
	log.Printf("facturación digital: factura %s/%s emitida por %s (control=%s)",
		f.tenant, f.invoiceID, cfg.Code, primerNoVacio(res.NroControlTexto, fmt.Sprintf("%d", res.NroControl)))

	// La copia propia del PDF: el enlace del proveedor caduca, el archivo del
	// contribuyente no. Va en su propio goroutine porque es I/O contra un
	// tercero y no debe frenar la cola.
	if res.PDFURL != "" || res.PDFBase64 != "" {
		go s.diArchivarPDF(db, ctl, f.tenant, enteroDeTexto(f.invoiceID), res.PDFURL, res.PDFBase64)
	}

	// LA ENTREGA AL CLIENTE. La factura ya salió con su documento: si la empresa
	// tiene activada la entrega automática, el correo sale ahora —en su propio
	// goroutine, porque un SMTP lento no debe atascar la cola de emisión—. Si
	// está apagada, diEntregarAutomatica ni siquiera consulta el canal. Y si dos
	// pasadas llegaran a cruzarse, la clave de idempotencia de finance_message
	// deja pasar una sola.
	if estado == "sent" {
		go s.diEntregarAutomatica(f.tenant, f.invoiceID)
	}
}

// diRegistrarFallo sube el contador de intentos de una fila y marca FAILED al
// llegar al límite. Mensaje corto y claro en error_message.
func (s *Server) diRegistrarFallo(ctl *sql.DB, id, intentos int64, causa error) {
	nuevo := intentos + 1
	estado := "pending"
	if nuevo >= diMaxIntentos {
		estado = "failed"
	}
	ctl.Exec(`UPDATE digital_invoice_log
		SET intentos=?, ultimo_intento=CURRENT_TIMESTAMP, status=?, error_message=?
		WHERE id=?`, nuevo, estado, causa.Error(), id)
}

// diCargarDocumento construye el DatosDocumento con los datos REALES de la
// factura y sus renglones, leídos de la base de la empresa del documento.
//
// POR QUÉ NO BASTA CON LA CABECERA. La primera versión mandaba solo totales: el
// proveedor recibía una factura sin renglones y las imprentas que validan el
// detalle (CG rechaza una factura sin productos) no emitían nada. Aquí se lee
// renglón por renglón, el producto del catálogo (código y nombre) y la ficha
// del cliente (correo, teléfono y dirección, que también son obligatorios).
func (s *Server) diCargarDocumento(db *sql.DB, invoiceID string, nroDocumento int) (*di.DatosDocumento, error) {
	var (
		fecha            string
		rifCabecera      string
		total, iva, igtf float64
		moneda           string
		tasa             float64
		customerID       int64
		idInterno        int64
	)
	err := db.QueryRow(`SELECT COALESCE(date,''), COALESCE(customer_rif,''), COALESCE(total_amount,0),
			COALESCE(total_vat_amnt,0), COALESCE(currency_code,'VES'), COALESCE(exchange_rate,0),
			COALESCE(igtf_amount,0), COALESCE(customer_id,0), COALESCE(id,0)
		FROM invoice WHERE invoice = ? LIMIT 1`, invoiceID).
		Scan(&fecha, &rifCabecera, &total, &iva, &moneda, &tasa, &igtf, &customerID, &idInterno)
	if err != nil {
		return nil, fmt.Errorf("factura inexistente: %w", err)
	}

	// La ficha del cliente: nombre, correo, teléfono y dirección. Sin correo o
	// sin dirección CG no emite, así que se traen y el proveedor dirá qué falta.
	cliente := di.DatosCliente{Nombre: "Cliente POS", FiscalRegistry: rifCabecera}
	if customerID > 0 {
		var nombre, correo, telefono, direccion, rif string
		if err := db.QueryRow(`SELECT COALESCE(customer_name,''),
				COALESCE(NULLIF(customer_email,''), email_address, ''),
				COALESCE(NULLIF(customer_mobile,''), phone, ''),
				COALESCE(NULLIF(customer_address,''), address2, ''),
				COALESCE(rif,'')
			FROM customer_information WHERE customer_id = ? LIMIT 1`, customerID).
			Scan(&nombre, &correo, &telefono, &direccion, &rif); err == nil {
			if strings.TrimSpace(nombre) != "" {
				cliente.Nombre = nombre
			}
			cliente.Email = strings.TrimSpace(correo)
			cliente.Telefono = strings.TrimSpace(telefono)
			cliente.Direccion = strings.TrimSpace(direccion)
			if strings.TrimSpace(rif) != "" {
				cliente.FiscalRegistry = rif
			}
		}
	}
	if strings.TrimSpace(cliente.FiscalRegistry) == "" {
		return nil, fmt.Errorf("la factura %s no tiene RIF de cliente: una factura fiscal no se puede emitir sin él", invoiceID)
	}
	cliente.TipoDocFiscal = diTipoDocDeRIF(cliente.FiscalRegistry)

	fechaDoc := fechaDeTexto(fecha)

	doc := &di.DatosDocumento{
		Tipo:              di.DocFactura,
		Numero:            nroDocumento,
		FechaEmision:      fechaDoc,
		FechaTasa:         fechaDoc,
		Moneda:            strings.ToUpper(strings.TrimSpace(moneda)),
		TasaCambio:        tasa,
		Total:             total,
		IVA:               iva,
		IGTF:              igtf,
		TotalFinal:        total,
		ReferenciaSistema: invoiceID,
		Cliente:           cliente,
	}
	if doc.Moneda == "" {
		doc.Moneda = "VES"
	}
	// La tasa del día: la de la factura si la tiene; si no, la vigente para su
	// fecha. CG rechaza la factura sin tasa, y mandar una inventada sería un
	// documento fiscal falso.
	if doc.TasaCambio <= 0 {
		if t, _, err := s.ObtenerTasaHoy(db, fechaDoc.Format("2006-01-02"), "USD"); err == nil {
			doc.TasaCambio = t
		}
	}

	detalles, err := diRenglones(db, invoiceID, idInterno)
	if err != nil {
		return nil, err
	}
	if len(detalles) == 0 {
		return nil, fmt.Errorf("la factura %s no tiene renglones: no hay documento fiscal que emitir", invoiceID)
	}
	doc.Detalles = detalles
	// Subtotales por alícuota: es la misma cuenta que hará la imprenta, pero el
	// ERP devuelve sus propias bases y así el documento que se archiva coincide.
	for _, d := range detalles {
		base := d.Cantidad * d.PrecioUnitario * (1 - d.DescuentoPct/100)
		doc.Subtotal += base
		switch {
		case d.PorcentajeIVA >= 30:
			doc.BaseSuntuario += base
		case d.PorcentajeIVA >= 15:
			doc.BaseImponible += base
		case d.PorcentajeIVA >= 7:
			doc.BaseReducida += base
		default:
			doc.MontoExento += base
		}
	}
	return doc, nil
}

// diRenglones lee los renglones de la factura con el producto del catálogo.
//
// EL CAMPO invoice_details.invoice_id NO ES SIEMPRE EL MISMO. El panel guarda
// ahí el `id` interno de la factura y el terminal del POS guardó el correlativo
// ("invoice"). Se consulta primero por el id interno y, si no aparece nada, por
// el correlativo: con una sola de las dos llaves, la mitad de las facturas del
// sistema saldría sin renglones y la imprenta la rechazaría.
func diRenglones(db *sql.DB, invoiceID string, idInterno int64) ([]di.DetalleItem, error) {
	consulta := `SELECT d.product_id, COALESCE(p.product_name,''), COALESCE(d.description,''),
			COALESCE(d.quantity,0), COALESCE(d.rate,0), COALESCE(d.total_price,0),
			COALESCE(d.vat_amnt,0), COALESCE(d.vat_amnt_per,''), COALESCE(d.discount_per,''),
			COALESCE(d.discount_pct,0)
		FROM invoice_details d
		LEFT JOIN product_information p ON p.product_id = d.product_id
		WHERE d.invoice_id = ? ORDER BY d.id`

	leer := func(llave interface{}) ([]di.DetalleItem, error) {
		rows, err := db.Query(consulta, llave)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var salida []di.DetalleItem
		for rows.Next() {
			var codigo, nombre, descripcion, vatPer, descPer string
			var cantidad, precio, totalRenglon, vat, descPct float64
			if rows.Scan(&codigo, &nombre, &descripcion, &cantidad, &precio, &totalRenglon,
				&vat, &vatPer, &descPer, &descPct) != nil {
				continue
			}
			porcentaje := porcentajeDeTexto(vatPer)
			if porcentaje <= 0 && vat > 0 {
				base := totalRenglon - vat
				if base > 0 {
					porcentaje = vat / base * 100
				}
			}
			descuento := porcentajeDeTexto(descPer)
			if descuento <= 0 {
				descuento = descPct
			}
			// Un renglón es exento cuando no genera IVA por alícuota, no cuando
			// el importe de IVA da cero: una base diminuta puede redondear el
			// IVA a 0,00 y eso NO convierte la venta en exenta.
			exento := porcentaje <= 0
			salida = append(salida, di.DetalleItem{
				Codigo:         strings.TrimSpace(codigo),
				Nombre:         strings.TrimSpace(nombre),
				Descripcion:    strings.TrimSpace(descripcion),
				Cantidad:       cantidad,
				PrecioUnitario: precio,
				DescuentoPct:   descuento,
				PorcentajeIVA:  porcentaje,
				EsExento:       exento,
				CodigoImpuesto: codigoImpuestoDe(porcentaje, exento),
				TipoProducto:   1,
			})
		}
		return salida, rows.Err()
	}

	if idInterno > 0 {
		if filas, err := leer(idInterno); err == nil && len(filas) > 0 {
			return filas, nil
		}
	}
	return leer(invoiceID)
}

// porcentajeDeTexto lee un porcentaje guardado como texto ("16.00", "16,00",
// "16 %"). No se usa parseMoney porque esa función interpreta la coma como
// separador de miles: "16,00" quedaría en 1600 y el renglón se emitiría con la
// alícuota de lujo (31 %) en vez de la general.
func porcentajeDeTexto(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(strings.ReplaceAll(s, ",", "."), "%f", &f)
	return f
}

// codigoImpuestoDe traduce el porcentaje de IVA del renglón al código que usan
// las imprentas (E exento, G 16 %, R 8 %, A 31 %).
func codigoImpuestoDe(porcentaje float64, exento bool) string {
	if exento && porcentaje <= 0 {
		return "E"
	}
	switch {
	case porcentaje >= 30:
		return "A"
	case porcentaje >= 15:
		return "G"
	case porcentaje >= 7:
		return "R"
	default:
		return "E"
	}
}

// fechaDeTexto interpreta la fecha de la factura, que el ERP guarda como texto
// (varchar) en formato ISO, con o sin hora.
func fechaDeTexto(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, formato := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339, "02-01-2006"} {
		if t, err := time.ParseInLocation(formato, s, time.Local); err == nil {
			return t
		}
	}
	return time.Now()
}

// diTipoDocDeRIF saca el tipo persona del RIF venezolano (J = jurídica,
// E/G/P/C según caso); por defecto V si no trae o es XI.
func diTipoDocDeRIF(rif string) string {
	rif = strings.TrimSpace(strings.ToUpper(rif))
	if rif == "" {
		return "V"
	}
	switch rif[0] {
	case 'J', 'E', 'P', 'G', 'C', 'V':
		return string(rif[0])
	}
	return "V"
}
