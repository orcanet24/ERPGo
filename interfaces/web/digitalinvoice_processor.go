package web

// digitalinvoice_processor.go — cola de emisión de facturación digital.
//
// Para qué existe: el POS puede facturar OFFLINE (la caja ya cobró e imprimió)
// y necesita que al volver la conexión la factura llegue al proveedor digital
// que la empresa tenga activo (Unidigital u otro del catálogo). También las
// ventas sincronizadas por gRPC pasan por aquí.
//
// Cómo funciona: cada venta apunta una fila pending en digital_invoice_log.
// Un runner en segundo plano (arranca con el ERP) toma pendientes, monta el
// DatosDocumento con los datos REALES de la factura y los detalles del
// terminal, emite al proveedor y marca sent; si falla sube intentos con
// límite, y deja error_message para el panel. Reintenta a sí solo: nadie tiene
// que apretar nada.
//
// La exclusividad quedó en digital_invoice_config (enabled=1 única por
// empresa): el runner no usa el registry en memoria para emitir, sino la
// configuración persistida — así un reinicio del ERP emite con el proveedor
// correcto sin pasos manuales.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	di "erpgow/interfaces/web/digitalinvoice"
	"erpgow/internal/digitalinvoice"
)

// diConfigBase es la configuración guardada de UN proveedor para UNA empresa.
type diConfigBase struct {
	Code string
	Conf di.ProviderConfig
}

// ------------- ENCOLADO -------------

// diEnqueueVenta registra unaventa para su emisión digital. Idempotente por
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

	// Datos del documento desde la base de la EMPRESA. El correlativo numérico
	// va en `invoice`; `control_no` es TEXTO ("00-00000001" en línea,
	// "CONT-T01-000123" en contingencia).
	//
	// ANTES se leía `control_no` en un sql.NullInt64 y por eso NINGUNA factura
	// del POS se encolaba: el scan fallaba con "converting string to int64",
	// el error solo se logueaba y la venta quedaba cobrada sin transmitir
	// nunca. El número que el proveedor necesita como correlativo es el de la
	// factura, no el control.
	var correlativo, serie, rif string
	var total float64
	err = db.QueryRow(`SELECT COALESCE(invoice, 0), COALESCE(series,''), COALESCE(customer_rif,''), total_amount
		FROM invoice WHERE invoice = ? LIMIT 1`, invoiceID).Scan(&correlativo, &serie, &rif, &total)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("facturación digital: lectura de factura %s/%s: %v", tenant, invoiceID, err)
		}
		return
	}
	nroDoc := enteroDeTexto(correlativo)

	// Fila pending; si ya existe la misma exploración, no se duplica.
	var existing int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM digital_invoice_log
		WHERE company_id = ? AND invoice_id = ? AND status IN ('pending')`, tenant, invoiceID).Scan(&existing); err == nil && existing > 0 {
		return
	}

	_, err = ctl.Exec(`INSERT INTO digital_invoice_log
		(invoice_id, company_id, addon_code, strong_id, nro_documento, tipo_documento, serie, status)
		VALUES (?, ?, ?, NULL, ?, 'FA', ?, 'pending')
		ON DUPLICATE KEY UPDATE ultimo_intento=NULL, status='pending'`,
		invoiceID, tenant, "", nroDoc, serie)
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
// este rango, algo estructural falla (config rota, proveedor caído) y que
// quede 'failed' avisa al admin en vez de reintentar para siempre.
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
		// Sin proveedor configurado: se deja pending. Podría configurar mañana.
		ctl.Exec(`UPDATE digital_invoice_log SET ultimo_intento=NOW() WHERE id=?`, f.id)
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
	doc, err := s.diCargarDocumento(db, f.tenant, f.invoiceID, int(f.nroDoc))
	if err != nil {
		log.Printf("facturación digital fila %d: documento %s: %v", f.id, f.invoiceID, err)
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

	// 4) Estado en la fila: Approval/Assigned o sent si ya con número.
	estado := "sent"
	if strings.EqualFold(res.Status, "Approved") {
		estado = "sent"
	} else if strings.EqualFold(res.Status, "Assigned") && !res.EsAsignado {
		estado = "sent"
	}
	_, err = ctl.Exec(`UPDATE digital_invoice_log
		SET status=?, strong_id=?, nro_control=?, pdf_url=?, xml_url=?
		WHERE id=?`, estado, res.StrongID, res.NroControl, res.PDFURL, res.XMLURL, f.id)
	if err != nil {
		log.Printf("facturación digital fila %d: actualización: %v", f.id, err)
		return
	}
	log.Printf("facturación digital: factura %s/%s emitida (strong_id=%s)", f.tenant, f.invoiceID, res.StrongID)

	// LA ENTREGA AL CLIENTE. La factura ya salió con su documento: si la empresa
	// tiene activada la entrega automática, el correo sale ahora —en su propio
	// goroutine, porque un SMTP lento no debe atascar la cola de emisión. Si
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
		SET intentos=?, ultimo_intento=NOW(), status=?, error_message=?
		WHERE id=?`, nuevo, estado, causa.Error(), id)
}

// diCargarDocumento construye el DatosDocumento leyendo la factura y sus
// detalles en la base de la empresa del documento.
func (s *Server) diCargarDocumento(db *sql.DB, tenant, invoiceID string, nroControl int) (*di.DatosDocumento, error) {
	var (
		fecha      time.Time
		rif        string
		total, iva float64
		moneda     string
	)
	err := db.QueryRow(`SELECT date, COALESCE(customer_rif,''), total_vat_amnt, total_amount,
		COALESCE(currency_code,'VES')
		FROM invoice WHERE invoice = ? LIMIT 1`, invoiceID).
		Scan(&fecha, &rif, &iva, &total, &moneda)
	if err != nil {
		return nil, fmt.Errorf("factura inexistente: %w", err)
	}
	// Nombre del cliente si la fila de invoice solo trae el RIF.
	var nombreCliente string
	if rif != "" {
		_ = db.QueryRow(`SELECT customer_name FROM customer_information WHERE rif = ? LIMIT 1`, rif).Scan(&nombreCliente)
	}
	if nombreCliente == "" {
		nombreCliente = "Cliente POS"
	}

	_ = moneda // DatosDocumento lleva su campo Moneda; el default del cfg ya es VES.

	doc := &di.DatosDocumento{
		Tipo:              di.DocFactura,
		Numero:            nroControl,
		FechaEmision:      fecha,
		Moneda:            "VES",
		Total:             total,
		IVA:               iva,
		TotalFinal:        total,
		ReferenciaSistema: invoiceID,
		Cliente: di.DatosCliente{
			Nombre:         nombreCliente,
			FiscalRegistry: rif,
			TipoDocFiscal:  diTipoDocDeRIF(rif),
		},
	}

	rows, err := db.Query(`SELECT description, quantity, rate, total_price, vat_amnt
		FROM invoice_details WHERE invoice_id = ?`, invoiceID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var desc string
			var qty, price, total_item, vat float64
			if rows.Scan(&desc, &qty, &price, &total_item, &vat) != nil {
				continue
			}
			doc.Detalles = append(doc.Detalles, di.DetalleItem{
				Descripcion:    desc,
				Cantidad:       qty,
				PrecioUnitario: price,
				PorcentajeIVA:  vat / (total_item - vat),
				EsExento:       vat == 0,
				TipoProducto:   1,
			})
			doc.Subtotal += total_item - vat
			doc.BaseImponible += total_item - vat
		}
	}
	return doc, nil
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
