package thka

// provider.go — The Factory HKA (TFHKA) según el contrato de digitalinvoice.
//
// El documento se arma con la estructura que exige la referencia técnica
// IT-21-01-01: Encabezado (identificación, comprador, totales) y DetallesItems.
// Todo lo que la imprenta exige y el ERP pudiera no tener se valida ANTES de la
// llamada, porque un documento fiscal con un dato inventado no se arregla con un
// reintento:
//
//   - Moneda bolívar (el ERP emite por esta vía en Bs; las facturas en divisas
//     se rechazan en vez de enviarlas como si fueran bolívares).
//   - RIF con su letra y su número (sin puntos ni guiones).
//   - Dirección del comprador (obligatoria en el catálogo de campos).
//   - Al menos un renglón, con cantidad y precio mayores que cero.
//   - En notas: fecha y monto de la factura afectada.
//
// Lo que el ERP no usa todavía —consulta por identificador, anulación— se
// responde con el límite REAL de la API: THKA pide para esas operaciones el
// número de documento, que el contrato de emisión no lleva.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	di "erpgo/interfaces/web/digitalinvoice"
)

// Código y nombre con los que esta imprenta se identifica en el ERP.
const (
	codigoProveedor = "thka"
	nombreProveedor = "The Factory HKA"
)

// Códigos del catálogo 1 de THKA (tipo de documento fiscal).
const (
	tipoFactura     = "01"
	tipoNotaCredito = "02"
	tipoNotaDebito  = "03"
	tipoGuia        = "04"
)

// Proveedor es la implementación del contrato para THKA.
type Proveedor struct {
	c   *cliente
	cfg di.ProviderConfig
}

// NuevoProveedor crea el proveedor con la configuración de UNA empresa.
func NuevoProveedor(cfg di.ProviderConfig) (*Proveedor, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("falta la URL base de The Factory HKA")
	}
	return &Proveedor{
		c:   nuevoCliente(cfg.BaseURL, cfg.UserName, cfg.UserPassword),
		cfg: cfg,
	}, nil
}

// Codigo retorna el identificador único del proveedor.
func (p *Proveedor) Codigo() string { return codigoProveedor }

// Nombre retorna el nombre legible.
func (p *Proveedor) Nombre() string { return nombreProveedor }

// Config retorna la configuración actual.
func (p *Proveedor) Config() di.ProviderConfig { return p.cfg }

// ActualizarConfig reemplaza la configuración y el transporte.
func (p *Proveedor) ActualizarConfig(cfg di.ProviderConfig) {
	p.cfg = cfg
	p.c = nuevoCliente(cfg.BaseURL, cfg.UserName, cfg.UserPassword)
}

// Ping valida credenciales pidiendo un token nuevo. Es la misma puerta que usa
// la emisión: si esto pasa, el usuario y la clave sirven.
func (p *Proveedor) Ping(ctx context.Context) error {
	if _, err := p.c.token(ctx, true); err != nil {
		return fmt.Errorf("la prueba de conexión con The Factory HKA falló: %w", err)
	}
	return nil
}

// ListarSeries explica el límite: THKA no publica un catálogo de series. La
// serie se escribe en la configuración (y puede quedar vacía si el negocio no
// usa series, como permite la referencia).
func (p *Proveedor) ListarSeries(ctx context.Context) ([]di.Serie, error) {
	return nil, fmt.Errorf("The Factory HKA no expone un catálogo de series: escriba la serie del negocio en la configuración o déjela vacía si no usa series")
}

// ListarMetodosPago devuelve el catálogo 11 de formas de pago de THKA.
//
// Es estático a propósito: la referencia lo fija (Depósito 01, Pago Móvil 02,
// Transferencia 03…) y el API no publica un endpoint para consultarlo.
func (p *Proveedor) ListarMetodosPago(ctx context.Context) ([]di.MetodoPago, error) {
	return []di.MetodoPago{
		{Codigo: "01", Descripcion: "Depósito en cuenta"},
		{Codigo: "02", Descripcion: "Pago Móvil"},
		{Codigo: "03", Descripcion: "Transferencia de fondos"},
		{Codigo: "04", Descripcion: "Orden de Pago"},
		{Codigo: "05", Descripcion: "Tarjeta de Débito"},
		{Codigo: "06", Descripcion: "Tarjeta de crédito"},
		{Codigo: "07", Descripcion: "Cheque no negociable"},
		{Codigo: "08", Descripcion: "Efectivo moneda de curso legal"},
		{Codigo: "09", Descripcion: "Efectivo divisas"},
		{Codigo: "18", Descripcion: "Otros medios de pago"},
	}, nil
}

// Emitir traduce el documento del ERPGo al documento electrónico de THKA.
func (p *Proveedor) Emitir(ctx context.Context, doc *di.DatosDocumento) (*di.ResultadoEmision, error) {
	if doc == nil {
		return nil, fmt.Errorf("no hay documento que emitir")
	}
	tipo, err := tipoDocumentoTHKA(doc.Tipo)
	if err != nil {
		return nil, err
	}
	envio, err := p.armarDocumento(doc, tipo)
	if err != nil {
		return nil, err
	}

	respuesta, err := p.c.llamar(ctx, "POST", rutaEmision, envio)
	if err != nil {
		return nil, err
	}

	var datos respuestaTHKA
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		// El cuerpo se muestra recortado: si la imprenta cambió la forma de su
		// respuesta, esto es lo único que permite diagnosticarlo desde la cola
		// sin volver a reproducir la emisión.
		return nil, fmt.Errorf("la respuesta de The Factory HKA no se pudo interpretar (%v): %.200s", err, strings.TrimSpace(string(respuesta)))
	}
	codigo := datos.Codigo.String()
	control := strings.TrimSpace(datos.Resultado.NumeroControl.String())

	// THKA informa el resultado en el cuerpo: "200" es éxito; "201" documento
	// duplicado y "203"/"205" rechazo por validación llegan con HTTP 200.
	if codigo != "" && codigo != "200" {
		return nil, errorDeRespuesta(200, respuesta)
	}
	if control == "" {
		return nil, fmt.Errorf("The Factory HKA respondió que procesó el documento pero no devolvió número de control: %.200s", strings.TrimSpace(string(respuesta)))
	}

	// El PDF no viene en la respuesta de emisión: se pide aparte, sin que un
	// fallo ahí impida dar por emitido el documento.
	pdfBase64, pdfURL := p.intentarPDF(ctx, tipo, numeroDocumento(doc.Numero))
	pdfURL = primerNoVacio(pdfURL, datos.Resultado.UrlPdf)

	resultado := &di.ResultadoEmision{
		StrongID:        idDeEnlace(pdfURL),
		NroControlTexto: control,
		Serie:           primerNoVacio(datos.Resultado.Serie, p.cfg.NumeroSerie),
		Status:          "Approved",
		Mensaje:         strings.TrimSpace(datos.Mensaje),
		FechaEmision:    doc.FechaEmision.Format(time.RFC3339),
		PDFURL:          pdfURL,
		PDFBase64:       pdfBase64,
		EsAsignado:      true,
	}
	resultado.NroControl = numeroDeControl(control)
	return resultado, nil
}

// Consultar explica el límite: THKA consulta por serie + tipo + número de
// documento, no por un identificador interno.
func (p *Proveedor) Consultar(ctx context.Context, strongID string) (*di.DocumentoConsulta, error) {
	return nil, fmt.Errorf("The Factory HKA no expone consulta por identificador interno; su API consulta por serie, tipo y número de documento")
}

// ConsultarPorNroControl explica el límite: el API no busca por número de
// control, sino por número de documento.
func (p *Proveedor) ConsultarPorNroControl(ctx context.Context, nroControl int) (*di.DocumentoConsulta, error) {
	return nil, fmt.Errorf("The Factory HKA no expone consulta por número de control; consulta por serie, tipo y número de documento")
}

// UltimoDocumento explica el límite: el listado de THKA está documentado como
// relación de documentos anulados/cancelados de un período, no como "el último
// documento emitido", y su respuesta no está descrita en la referencia.
func (p *Proveedor) UltimoDocumento(ctx context.Context, tipo di.DocumentType, serie string) (*di.DocumentoConsulta, error) {
	return nil, fmt.Errorf("The Factory HKA no publica una consulta de «último documento emitido»: para comprobar el correlativo use el listado de su portal")
}

// Anular explica el límite: anular en THKA exige serie, tipo y número de
// documento además del motivo; el contrato solo recibe el número de control.
func (p *Proveedor) Anular(ctx context.Context, nroControl int) error {
	return fmt.Errorf("para anular en The Factory HKA hacen falta la serie, el tipo y el número de documento; el contrato solo recibe el control %d", nroControl)
}

// ---------- Emisión ----------

// armarDocumento construye el sobre que espera /api/Emision.
func (p *Proveedor) armarDocumento(doc *di.DatosDocumento, tipo string) (map[string]interface{}, error) {
	identificacion, err := p.identificacionDe(doc, tipo)
	if err != nil {
		return nil, err
	}
	comprador, err := compradorDe(doc.Cliente)
	if err != nil {
		return nil, err
	}
	items, totales, err := itemsYTotales(doc)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"Documentoelectronico": map[string]interface{}{
			"Encabezado": map[string]interface{}{
				"IdentificacionDocumento": identificacion,
				"Comprador":               comprador,
				"Totales":                 totales,
			},
			"DetallesItems": items,
		},
	}, nil
}

// identificacionDe arma el nodo IdentificacionDocumento.
func (p *Proveedor) identificacionDe(doc *di.DatosDocumento, tipo string) (map[string]interface{}, error) {
	if doc.Numero <= 0 {
		return nil, fmt.Errorf("el documento no tiene número correlativo con el que emitirse")
	}
	moneda := strings.ToUpper(strings.TrimSpace(doc.Moneda))
	if moneda != "" && moneda != "VES" {
		return nil, fmt.Errorf("The Factory HKA emite por esta vía en bolívares (VES) y la factura está en %s: no se puede emitir por esta imprenta", moneda)
	}

	fecha := doc.FechaEmision
	if fecha.IsZero() {
		fecha = time.Now()
	}

	nodo := map[string]interface{}{
		"TipoDocumento":   tipo,
		"NumeroDocumento": numeroDocumento(doc.Numero),
		"FechaEmision":    fecha.Format("02/01/2006"),
		"HoraEmision":     fecha.Format("03:04:05 pm"),
		"Anulado":         false,
		"Serie":           strings.TrimSpace(p.cfg.NumeroSerie),
		"TipoDeVenta":     "Interna",
		"Moneda":          "VES",
		"TipoDePago":      tipoDePago(doc.TipoPago),
	}

	// La sucursal es opcional y THKA la limita a 6 caracteres.
	if s := recortar(strings.TrimSpace(p.cfg.SucursalID), 6); s != "" {
		nodo["Sucursal"] = s
	}

	// Las notas exigen los datos de la factura que afectan: la imprenta los
	// imprime y sin ellos rechaza el documento.
	if tipo != tipoFactura {
		afectada := strings.TrimSpace(doc.DocumentoAfectado)
		if afectada == "" || !soloDigitos(afectada) {
			return nil, fmt.Errorf("la nota necesita el número de la factura que afecta (solo dígitos)")
		}
		if doc.DocumentoAfectadoFecha.IsZero() {
			return nil, fmt.Errorf("la nota necesita la fecha de la factura afectada: The Factory HKA la imprime en el documento")
		}
		if doc.DocumentoAfectadoMonto <= 0 {
			return nil, fmt.Errorf("la nota necesita el monto total de la factura afectada")
		}
		nodo["TipoTransaccion"] = "02" // complemento (catálogo 3)
		nodo["NumeroFacturaAfectada"] = afectada
		nodo["FechaFacturaAfectada"] = doc.DocumentoAfectadoFecha.Format("02/01/2006")
		nodo["MontoFacturaAfectada"] = monto(doc.DocumentoAfectadoMonto)
		if s := recortar(strings.TrimSpace(p.cfg.NumeroSerie), 20); s != "" {
			nodo["SerieFacturaAfectada"] = s
		}
		if c := recortar(strings.TrimSpace(doc.Descripcion), 255); c != "" {
			nodo["ComentarioFacturaAfectada"] = c
		}
	} else {
		nodo["TipoTransaccion"] = "01" // registro (catálogo 3)
	}
	return nodo, nil
}

// compradorDe arma el nodo Comprador con el formato exacto que pide THKA.
func compradorDe(cliente di.DatosCliente) (map[string]interface{}, error) {
	letra, digitos, err := identidadDe(cliente.FiscalRegistry, cliente.TipoDocFiscal)
	if err != nil {
		return nil, err
	}
	nombre := strings.TrimSpace(cliente.Nombre)
	if nombre == "" {
		return nil, fmt.Errorf("el cliente del documento no tiene nombre o razón social")
	}
	direccion := strings.TrimSpace(cliente.Direccion)
	if direccion == "" {
		return nil, fmt.Errorf("el cliente «%s» no tiene dirección fiscal en su ficha: The Factory HKA la exige", recortar(nombre, 60))
	}

	nodo := map[string]interface{}{
		"TipoIdentificacion":   letra,
		"NumeroIdentificacion": digitos,
		"RazonSocial":          recortar(nombre, 100),
		"Direccion":            recortar(direccion, 255),
		"Pais":                 "VE",
	}
	if t := strings.TrimSpace(cliente.Telefono); t != "" {
		nodo["Telefono"] = []string{recortar(t, 20)}
	}
	if c := strings.TrimSpace(cliente.Email); c != "" {
		nodo["Correo"] = []string{recortar(c, 50)}
	}
	return nodo, nil
}

// itemsYTotales arma los renglones y los totales, que THKA contrasta entre sí
// (la imprenta no calcula: imprime lo que recibe y valida que cuadre).
func itemsYTotales(doc *di.DatosDocumento) ([]map[string]interface{}, map[string]interface{}, error) {
	if len(doc.Detalles) == 0 {
		return nil, nil, fmt.Errorf("la factura no tiene renglones que emitir")
	}

	type acumulado struct {
		base  float64
		valor float64
	}
	porImpuesto := map[string]*acumulado{}
	var gravado, exento, percibido, totalIVA, totalDescuento, antesDescuento float64

	items := make([]map[string]interface{}, 0, len(doc.Detalles))
	for i, d := range doc.Detalles {
		if d.Cantidad <= 0 {
			return nil, nil, fmt.Errorf("el renglón %d (%s) tiene cantidad cero o negativa: no se puede emitir", i+1, textoDeRenglon(d))
		}
		if d.PrecioUnitario <= 0 {
			return nil, nil, fmt.Errorf("el renglón %d (%s) tiene precio cero: The Factory HKA no admite renglones a precio cero", i+1, textoDeRenglon(d))
		}

		codigo := codigoImpuestoDe(d)
		bruto := redondear2(d.PrecioUnitario * d.Cantidad)
		descuento := redondear2(bruto * d.DescuentoPct / 100)
		precioItem := redondear2(bruto - descuento)
		tasa := tasaDeCodigo(codigo)
		valorIVA := redondear2(precioItem * tasa / 100)
		totalItem := redondear2(precioItem + valorIVA)

		item := map[string]interface{}{
			"NumeroLinea":            strconv.Itoa(i + 1),
			"IndicadorBienoServicio": indicadorBienoServicio(d),
			"Descripcion":            recortar(descripcionDe(d), 255),
			"Cantidad":               monto(d.Cantidad),
			"UnidadMedida":           unidadDeMedida(d),
			"PrecioUnitario":         monto(d.PrecioUnitario),
			"PrecioItem":             monto(precioItem),
			"CodigoImpuesto":         codigo,
			"TasaIVA":                monto(tasa),
			"ValorIVA":               monto(valorIVA),
			"ValorTotalItem":         monto(totalItem),
		}
		if c := codigoPLU(d.Codigo); c != "" {
			item["CodigoPLU"] = c
		}
		if descuento > 0 {
			item["DescuentoMonto"] = monto(descuento)
			item["PrecioAntesDescuento"] = monto(bruto)
		}
		items = append(items, item)

		switch codigo {
		case "E":
			exento += precioItem
		case "P":
			percibido += precioItem
		default:
			gravado += precioItem
		}
		totalIVA += valorIVA
		totalDescuento += descuento
		antesDescuento += bruto

		acum, hay := porImpuesto[codigo]
		if !hay {
			acum = &acumulado{}
			porImpuesto[codigo] = acum
		}
		acum.base += precioItem
		acum.valor += valorIVA
	}

	subtotal := redondear2(gravado + exento + percibido)
	totalIVA = redondear2(totalIVA)
	conIVA := redondear2(subtotal + totalIVA)

	totales := map[string]interface{}{
		"NroItems":          strconv.Itoa(len(items)),
		"MontoGravadoTotal": monto(redondear2(gravado)),
		"MontoExentoTotal":  monto(redondear2(exento)),
		"Subtotal":          monto(subtotal),
		"TotalIVA":          monto(totalIVA),
		"MontoTotalConIVA":  monto(conIVA),
	}
	if percibido > 0 {
		totales["MontoPercibidoTotal"] = monto(redondear2(percibido))
	}
	if totalDescuento > 0 {
		totales["TotalDescuento"] = monto(redondear2(totalDescuento))
		totales["SubtotalAntesDescuento"] = monto(redondear2(antesDescuento))
	}

	// El orden del catálogo (E, P, R, G, A) no importa para THKA, pero un orden
	// estable hace que dos facturas iguales salgan iguales y las pruebas sean
	// legibles.
	impuestos := make([]map[string]interface{}, 0, len(porImpuesto))
	for _, codigo := range []string{"E", "P", "R", "G", "A"} {
		acum, hay := porImpuesto[codigo]
		if !hay {
			continue
		}
		impuestos = append(impuestos, map[string]interface{}{
			"CodigoTotalImp":   codigo,
			"AlicuotaImp":      monto(tasaDeCodigo(codigo)),
			"BaseImponibleImp": monto(redondear2(acum.base)),
			"ValorTotalImp":    monto(redondear2(acum.valor)),
		})
	}

	// IGTF: se declara como impuesto subtotal y entra en el total a pagar, que
	// es la fórmula de la referencia (totalAPagar = montoTotalConIVA + IGTF).
	totalAPagar := conIVA
	if doc.IGTF > 0 {
		base := redondear2(doc.IGTF / 0.03)
		valor := redondear2(base * 0.03)
		impuestos = append(impuestos, map[string]interface{}{
			"CodigoTotalImp":   "IGTF",
			"AlicuotaImp":      "3.00",
			"BaseImponibleImp": monto(base),
			"ValorTotalImp":    monto(valor),
		})
		totalAPagar = redondear2(conIVA + valor)
	}
	totalAPagar = redondear2(totalAPagar)
	if len(impuestos) > 0 {
		totales["ImpuestosSubtotal"] = impuestos
	}
	totales["TotalAPagar"] = monto(totalAPagar)
	return items, totales, nil
}

// intentarPDF pide el PDF al endpoint de descarga.
//
// POR QUÉ ES BEST-EFFORT. La referencia describe el endpoint pero no el formato
// de su respuesta, y el número de control —que sí llega en la emisión— es lo
// fiscal. Aquí se aceptan las dos formas plausibles: el PDF binario o un JSON
// con el archivo en base64 (o un enlace). Cualquier otra cosa se ignora: la
// factura ya está emitida y el administrador ve el número de control.
func (p *Proveedor) intentarPDF(ctx context.Context, tipo, numero string) (pdfBase64, pdfURL string) {
	cuerpo := map[string]interface{}{
		"serie":           strings.TrimSpace(p.cfg.NumeroSerie),
		"tipoDocumento":   tipo,
		"numeroDocumento": numero,
	}
	respuesta, err := p.c.llamar(ctx, "POST", rutaDescarga, cuerpo)
	if err != nil || len(respuesta) == 0 {
		return "", ""
	}
	if strings.HasPrefix(strings.TrimSpace(string(respuesta)), "%PDF") {
		return base64.StdEncoding.EncodeToString(respuesta), ""
	}
	var datos interface{}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return "", ""
	}
	return buscarPDF(datos, 0)
}

// buscarPDF recorre la respuesta buscando un enlace o un archivo en base64.
// La profundidad está acotada para que una respuesta inesperada no cuelgue.
func buscarPDF(datos interface{}, profundidad int) (pdfBase64, pdfURL string) {
	if profundidad > 4 {
		return "", ""
	}
	switch v := datos.(type) {
	case map[string]interface{}:
		for llave, valor := range v {
			texto, esTexto := valor.(string)
			if !esTexto {
				if b64, url := buscarPDF(valor, profundidad+1); b64 != "" || url != "" {
					return b64, url
				}
				continue
			}
			limpio := strings.TrimSpace(texto)
			if limpio == "" {
				continue
			}
			min := strings.ToLower(llave)
			if strings.HasPrefix(limpio, "http://") || strings.HasPrefix(limpio, "https://") {
				if strings.Contains(min, "pdf") || strings.Contains(min, "url") || strings.Contains(min, "archivo") {
					return "", limpio
				}
				continue
			}
			// Un PDF en base64 empieza por "JVBERi" ("%PDF" codificado).
			if strings.HasPrefix(limpio, "JVBERi") && len(limpio) > 100 {
				return limpio, ""
			}
		}
	case []interface{}:
		for _, elemento := range v {
			if b64, url := buscarPDF(elemento, profundidad+1); b64 != "" || url != "" {
				return b64, url
			}
		}
	}
	return "", ""
}

// ---------- Traducciones ERPGo → catálogos THKA ----------

// tipoDocumentoTHKA traduce el tipo del contrato al catálogo 1 de THKA.
func tipoDocumentoTHKA(tipo di.DocumentType) (string, error) {
	switch tipo {
	case di.DocFactura:
		return tipoFactura, nil
	case di.DocNotaCredito:
		return tipoNotaCredito, nil
	case di.DocNotaDebito:
		return tipoNotaDebito, nil
	case di.DocGuiaDespacho:
		return "", fmt.Errorf("The Factory HKA emite la guía de despacho por su nodo GuiaDespacho, que el contrato de emisión del ERP no lleva (origen, destino, vehículo y conductor)")
	case di.DocRetencionIVA, di.DocRetencionISLR:
		return "", fmt.Errorf("los comprobantes de retención no se emiten por el contrato de emisión de ventas")
	default:
		return "", fmt.Errorf("tipo de documento no soportado por The Factory HKA: %s", tipo)
	}
}

// codigoImpuestoDe devuelve el código de alícuota del catálogo 10.
func codigoImpuestoDe(d di.DetalleItem) string {
	switch strings.ToUpper(strings.TrimSpace(d.CodigoImpuesto)) {
	case "G", "R", "A", "E", "P":
		return strings.ToUpper(strings.TrimSpace(d.CodigoImpuesto))
	}
	switch {
	case d.EsExento:
		return "E"
	case d.PorcentajeIVA >= 30:
		return "A"
	case d.PorcentajeIVA >= 15:
		return "G"
	case d.PorcentajeIVA >= 7:
		return "R"
	default:
		return "E"
	}
}

// tasaDeCodigo da la alícuota de un código del catálogo 10.
//
// La tasa sale DEL CÓDIGO y no del porcentaje del renglón: THKA valida que
// `TasaIVA` corresponda a la alícuota declarada, y dos fuentes distintas (16
// frente a 16,5, por ejemplo) harían que la imprenta rechazara el documento.
func tasaDeCodigo(codigo string) float64 {
	switch codigo {
	case "G":
		return 16
	case "R":
		return 8
	case "A":
		return 31
	default:
		return 0
	}
}

// indicadorBienoServicio: 1 = bien, 2 = servicio (catálogo del campo).
func indicadorBienoServicio(d di.DetalleItem) string {
	if d.TipoProducto == 2 {
		return "2"
	}
	return "1"
}

// unidadDeMedida usa el código UNECE. NIU (unidades) para bienes y ZZ ("según
// acuerdo mutuo") para servicios, que son los dos casos que maneja el ERP.
func unidadDeMedida(d di.DetalleItem) string {
	if d.TipoProducto == 2 {
		return "ZZ"
	}
	return "NIU"
}

// tipoDePago traduce el tipo de pago del ERP al texto que espera THKA.
//
// El ERP escribe "Crédito"/"Inmediato" desde la cola, pero el campo también
// admite las condiciones de venta que usan los formularios ("15 DIAS"), que son
// crédito igual: tratarlas como contado imprimiría un pago que no existe.
func tipoDePago(tipo string) string {
	t := strings.ToLower(strings.TrimSpace(tipo))
	if t == "" || strings.Contains(t, "inmediato") || strings.Contains(t, "contado") {
		return "Inmediato"
	}
	if strings.Contains(t, "cred") || strings.Contains(t, "plazo") {
		return "Crédito"
	}
	// "15 DIAS", "30 días": la condición de venta en días es crédito. Se busca
	// la palabra completa porque "inmediato" también contiene "dia".
	for _, palabra := range strings.Fields(t) {
		switch palabra {
		case "dia", "día", "dias", "días":
			return "Crédito"
		}
	}
	return "Inmediato"
}

// descripcionDe arma la descripción del renglón: la del documento y, si no hay,
// el nombre del producto.
func descripcionDe(d di.DetalleItem) string {
	if s := strings.TrimSpace(d.Descripcion); s != "" {
		return s
	}
	if s := strings.TrimSpace(d.Nombre); s != "" {
		return s
	}
	return "Ítem"
}

// textoDeRenglon arma el nombre legible de un renglón para los errores.
func textoDeRenglon(d di.DetalleItem) string {
	return recortar(descripcionDe(d), 60)
}

// codigoPLU deja el código del producto como THKA lo acepta: hasta 20
// caracteres alfanuméricos.
func codigoPLU(codigo string) string {
	var limpio []rune
	for _, r := range strings.TrimSpace(codigo) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			limpio = append(limpio, r)
		}
		if len(limpio) == 20 {
			break
		}
	}
	return string(limpio)
}

// identidadDe separa el RIF en letra y número. THKA pide la letra en
// TipoIdentificacion y el número sin puntos ni guiones.
func identidadDe(rif, tipoDoc string) (letra, digitos string, err error) {
	crudo := soloAlfanumerico(rif)
	if crudo == "" {
		return "", "", fmt.Errorf("la factura no tiene RIF de cliente: una factura fiscal no se puede emitir sin él")
	}
	r := []rune(crudo)
	if unicode.IsLetter(r[0]) {
		letra = strings.ToUpper(string(r[0]))
		digitos = string(r[1:])
	} else {
		digitos = crudo
		letra = strings.ToUpper(strings.TrimSpace(tipoDoc))
	}
	if !strings.Contains("VEPJGC", letra) || len([]rune(letra)) != 1 {
		return "", "", fmt.Errorf("el documento de identidad del cliente «%s» debe empezar por V, E, P, J, G o C", strings.TrimSpace(rif))
	}
	if !soloDigitos(digitos) || len(digitos) == 0 {
		return "", "", fmt.Errorf("el documento de identidad del cliente «%s» no tiene una parte numérica válida", strings.TrimSpace(rif))
	}
	if len(digitos) > 19 {
		return "", "", fmt.Errorf("el documento de identidad del cliente «%s» excede los 19 dígitos que admite The Factory HKA", strings.TrimSpace(rif))
	}
	return letra, digitos, nil
}

// numeroDocumento es el número que el ERP pone al documento, solo dígitos.
func numeroDocumento(numero int) string {
	if numero <= 0 {
		return "0"
	}
	return strconv.Itoa(numero)
}

// numeroDeControl saca la parte numérica del control que asigne la imprenta
// ("00-00000056" → 56). Es solo para el campo entero del registro; el que se
// muestra y se entrega al cliente es el textual.
func numeroDeControl(control string) int {
	control = strings.TrimSpace(control)
	if i := strings.LastIndex(control, "-"); i >= 0 {
		control = control[i+1:]
	}
	n, _ := strconv.Atoi(control)
	return n
}

// idDeEnlace saca el identificador del documento del enlace del PDF, si lo hay.
func idDeEnlace(enlace string) string {
	e := strings.TrimSpace(enlace)
	if e == "" {
		return ""
	}
	if i := strings.IndexAny(e, "?#"); i >= 0 {
		e = e[:i]
	}
	e = strings.TrimRight(e, "/")
	if i := strings.LastIndex(e, "/"); i >= 0 {
		e = e[i+1:]
	}
	return e
}

// redondear2 redondea a dos decimales como lo hace una factura.
func redondear2(v float64) float64 {
	return math.Round(v*100) / 100
}

// monto formatea un importe con los dos decimales que exige THKA.
func monto(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0.00"
	}
	return strconv.FormatFloat(redondear2(v), 'f', 2, 64)
}

// recortar limita un texto por CARACTERES (no bytes) para que un acento o una
// eñe no parta una palabra al llegar al límite.
func recortar(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// soloDigitos dice si el texto está formado solo por dígitos.
func soloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// soloAlfanumerico deja solo letras y dígitos.
func soloAlfanumerico(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// primerNoVacio devuelve el primer valor con contenido.
func primerNoVacio(alternativas ...string) string {
	for _, a := range alternativas {
		if s := strings.TrimSpace(a); s != "" {
			return s
		}
	}
	return ""
}
