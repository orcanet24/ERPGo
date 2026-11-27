package cgimprenta

// provider.go — CG Imprenta Digital según el contrato de digitalinvoice.
//
// El mapeo es explícito y en una sola dirección (ERPGo → CG). Todo lo que la
// imprenta exige y el ERP pudiera no tener se valida aquí, ANTES de la llamada:
//   - RIF sin guiones, letra + 6..10 dígitos (convenciones de CG).
//   - Tasa del día con su fecha (CG rechaza la factura sin ella).
//   - Moneda bolívar (la API de CG solo emite en Bs; enviar dólares como si
//     fueran bolívares sería un documento fiscal falso, no un reintento).
//   - Al menos un renglón, con código de producto y precio mayor que cero.
//
// Lo que el ERP no usa hoy —consulta por identificador, anulación— se responde
// con el límite REAL del API en vez de un "no implementado" mudo: quien lo
// llame desde el ERP mañana sabe qué le falta al contrato para hacerlo.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	di "erpgo/interfaces/web/digitalinvoice"
)

// Código y nombre con los que esta imprenta se identifica en el ERP. El código
// es el que se guarda en digital_invoice_config.addon_code y en
// digital_invoice_log.addon_code: cambiarlo deja huérfanas las filas viejas.
const (
	codigoProveedor = "cgimprenta"
	nombreProveedor = "CG Imprenta Digital"
)

// Proveedor es la implementación del contrato para CG.
type Proveedor struct {
	c   *cliente
	cfg di.ProviderConfig
}

// NuevoProveedor crea el proveedor con la configuración de UNA empresa.
func NuevoProveedor(cfg di.ProviderConfig) (*Proveedor, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("falta la URL base de CG Imprenta Digital")
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

// Ping comprueba credenciales y conectividad con una consulta real. Pedir el
// token ya valida el usuario y la contraseña; el catálogo de métodos de pago
// demuestra además que el token sirve para operar.
func (p *Proveedor) Ping(ctx context.Context) error {
	if _, err := p.c.get(ctx, rutaMetodosDePago); err != nil {
		return fmt.Errorf("la prueba de conexión con CG Imprenta Digital falló: %w", err)
	}
	return nil
}

// ListarMetodosPago devuelve el catálogo que publica CG.
func (p *Proveedor) ListarMetodosPago(ctx context.Context) ([]di.MetodoPago, error) {
	respuesta, err := p.c.get(ctx, rutaMetodosDePago)
	if err != nil {
		return nil, err
	}
	var datos struct {
		Metodos []struct {
			ID     string `json:"id_payment_method"`
			Nombre string `json:"payment_method"`
		} `json:"payment_methods"`
	}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return nil, fmt.Errorf("el catálogo de métodos de pago de CG no es JSON válido: %w", err)
	}
	metodos := make([]di.MetodoPago, 0, len(datos.Metodos))
	for _, m := range datos.Metodos {
		metodos = append(metodos, di.MetodoPago{Codigo: m.Nombre, Descripcion: m.Nombre})
	}
	return metodos, nil
}

// ListarSeries explica el límite: CG no publica un catálogo de series. La serie
// de numeración ("A") se configura a mano con la que el negocio tenga asignada.
func (p *Proveedor) ListarSeries(ctx context.Context) ([]di.Serie, error) {
	return nil, fmt.Errorf("CG Imprenta Digital no expone un catálogo de series: escriba en la configuración la serie de numeración asignada a su negocio (por ejemplo «A»)")
}

// Emitir traduce el documento del ERPGo al endpoint que corresponde.
func (p *Proveedor) Emitir(ctx context.Context, doc *di.DatosDocumento) (*di.ResultadoEmision, error) {
	if doc == nil {
		return nil, fmt.Errorf("no hay documento que emitir")
	}
	switch doc.Tipo {
	case di.DocFactura:
		return p.emitirFactura(ctx, doc)
	case di.DocNotaCredito:
		return p.emitirNotaCredito(ctx, doc)
	case di.DocNotaDebito:
		return p.emitirNotaDebito(ctx, doc)
	case di.DocGuiaDespacho:
		return nil, fmt.Errorf("la guía de despacho necesita datos de traslado (origen, destino, vehículo y chofer) que el contrato DatosDocumento del ERP no lleva: no se emite por esta vía")
	case di.DocRetencionIVA, di.DocRetencionISLR:
		return nil, fmt.Errorf("los comprobantes de retención no se emiten por el contrato de emisión de ventas")
	default:
		return nil, fmt.Errorf("tipo de documento no soportado por CG Imprenta Digital: %s", doc.Tipo)
	}
}

// Consultar explica el límite: CG no tiene consulta por identificador interno.
func (p *Proveedor) Consultar(ctx context.Context, strongID string) (*di.DocumentoConsulta, error) {
	return nil, fmt.Errorf("CG Imprenta Digital no expone consulta por identificador interno; consulte por número de control completo desde el panel de la imprenta")
}

// ConsultarPorNroControl explica el límite: el contrato pasa el número de
// control como entero, y en CG el número completo incluye la serie
// ("00-00000456"). Buscar por la parte numérica sola podría devolver el
// documento de otra serie, así que no se adivina.
func (p *Proveedor) ConsultarPorNroControl(ctx context.Context, nroControl int) (*di.DocumentoConsulta, error) {
	return nil, fmt.Errorf("CG Imprenta Digital consulta por número de control completo (con su serie); el número entero %d no basta para identificarlo", nroControl)
}

// Anular explica el límite: anular en CG exige el número de documento Y el
// número de control completos, y el contrato solo recibe el número de control.
func (p *Proveedor) Anular(ctx context.Context, nroControl int) error {
	return fmt.Errorf("para anular en CG Imprenta Digital hacen falta el número de documento y el número de control completos; el contrato solo recibe el control %d", nroControl)
}

// UltimoDocumento devuelve el último documento de un tipo según la lista que
// publica CG (los documentos llegan ordenados por número de control, de mayor a
// menor). Sirve para comprobar el correlativo antes de emitir.
func (p *Proveedor) UltimoDocumento(ctx context.Context, tipo di.DocumentType, serie string) (*di.DocumentoConsulta, error) {
	nombreCG, ok := tipoDocumentoCG(tipo)
	if !ok {
		return nil, fmt.Errorf("CG no publica documentos de tipo %s en su listado", tipo)
	}
	respuesta, err := p.c.get(ctx, rutaListaFacturas+"?limite=100&pagina=1")
	if err != nil {
		return nil, err
	}
	var datos struct {
		Documentos [][]struct {
			Documento     string `json:"document"`
			InvoiceNumber string `json:"invoice_number"`
			ControlNumber string `json:"control_number"`
			CustomerName  string `json:"customer_name"`
			CustomerRIF   string `json:"customer_document_identification"`
			Serial        string `json:"serial_code"`
			Status        string `json:"status"`
			Creado        string `json:"created"`
			PDF           string `json:"invoice_pdf"`
		} `json:"invoices"`
	}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return nil, fmt.Errorf("la lista de documentos de CG no es JSON válido: %w", err)
	}
	for _, grupo := range datos.Documentos {
		for _, d := range grupo {
			if !strings.EqualFold(strings.TrimSpace(d.Documento), nombreCG) {
				continue
			}
			if strings.TrimSpace(serie) != "" && d.Serial != "" && !strings.EqualFold(d.Serial, strings.TrimSpace(serie)) {
				continue
			}
			controlSerie, controlNumero := partirControl(d.ControlNumber)
			return &di.DocumentoConsulta{
				Numero:        enteroDeTexto(d.InvoiceNumber),
				NroControl:    controlNumero,
				ControlTexto:  strings.TrimSpace(d.ControlNumber),
				TipoDoc:       string(tipo),
				Serie:         controlSerie,
				ClienteRIF:    d.CustomerRIF,
				ClienteNombre: d.CustomerName,
				Moneda:        "VES",
				FechaEmision:  d.Creado,
				Status:        d.Status,
				EsAnulado:     strings.EqualFold(d.Status, "Anulada"),
				PDFURL:        strings.TrimSpace(d.PDF),
			}, nil
		}
	}
	return nil, fmt.Errorf("CG no devolvió ningún documento de tipo %s para la serie %q", nombreCG, serie)
}

// ---------- Emisión ----------

// emitirFactura emite una factura (el camino que usa hoy el ERP).
func (p *Proveedor) emitirFactura(ctx context.Context, doc *di.DatosDocumento) (*di.ResultadoEmision, error) {
	serie := p.serieDe()
	if serie == "" {
		return nil, fmt.Errorf("falta la serie de numeración: configúrela en Facturación Digital (por ejemplo «A»)")
	}
	factura, err := p.armarFactura(doc)
	if err != nil {
		return nil, err
	}

	cuerpo := map[string]interface{}{
		"numeroSerie":     serie,
		"cantidadFactura": 1,
		"facturas":        []interface{}{factura},
	}
	respuesta, err := p.c.post(ctx, rutaFacturas, cuerpo)
	if err != nil {
		return nil, err
	}

	var datos struct {
		Success       bool     `json:"success"`
		Message       string   `json:"message"`
		InvoiceErrors []string `json:"invoice_errors"`
		Emitidas      []struct {
			Numero  string `json:"invoice_number"`
			Control string `json:"control_number"`
			PDF     string `json:"invoice_pdf"`
		} `json:"invoice_list_success"`
	}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return nil, fmt.Errorf("la respuesta de CG no es JSON válido: %w", err)
	}

	numero := numeroDocumento(doc.Numero)
	for _, e := range datos.Emitidas {
		if strings.TrimSpace(e.Numero) == numero {
			if strings.TrimSpace(e.Control) == "" {
				return nil, fmt.Errorf("CG emitió la factura %s pero no devolvió su número de control", numero)
			}
			return resultado(e.Control, e.PDF, datos.Message, doc.FechaEmision), nil
		}
	}
	for _, errante := range datos.InvoiceErrors {
		if strings.TrimSpace(errante) == numero {
			return nil, fmt.Errorf("CG Imprenta Digital no emitió la factura %s: su número ya estaba registrado en esa imprenta (documento duplicado)", numero)
		}
	}
	if !datos.Success {
		return nil, fmt.Errorf("CG Imprenta Digital no emitió la factura %s: %s", numero, mensajeDe(datos.Message))
	}
	return nil, fmt.Errorf("CG Imprenta Digital respondió sin el número de control de la factura %s: %s", numero, mensajeDe(datos.Message))
}

// emitirNotaCredito emite la nota de crédito TOTAL de una factura.
func (p *Proveedor) emitirNotaCredito(ctx context.Context, doc *di.DatosDocumento) (*di.ResultadoEmision, error) {
	afectada, motivo, err := datosDeNota(doc)
	if err != nil {
		return nil, err
	}
	cuerpo := map[string]interface{}{
		"numeroFactura":     afectada,
		"numeroNotaCredito": numeroDocumento(doc.Numero),
		"descripcion":       motivo,
	}
	respuesta, err := p.c.post(ctx, rutaNotaCredito, cuerpo)
	if err != nil {
		return nil, err
	}
	var datos struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Control string `json:"control_number"`
		PDF     string `json:"credit_note_pdf"`
	}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return nil, fmt.Errorf("la respuesta de CG no es JSON válido: %w", err)
	}
	if !datos.Success || strings.TrimSpace(datos.Control) == "" {
		return nil, fmt.Errorf("CG no emitió la nota de crédito: %s", mensajeDe(datos.Message))
	}
	return resultado(datos.Control, datos.PDF, datos.Message, doc.FechaEmision), nil
}

// emitirNotaDebito emite la nota de débito de una factura.
func (p *Proveedor) emitirNotaDebito(ctx context.Context, doc *di.DatosDocumento) (*di.ResultadoEmision, error) {
	afectada, motivo, err := datosDeNota(doc)
	if err != nil {
		return nil, err
	}
	if err := validarTasaYMoneda(doc); err != nil {
		return nil, err
	}
	productos, err := productosDe(doc.Detalles, false)
	if err != nil {
		return nil, err
	}
	if len(productos) == 0 {
		return nil, fmt.Errorf("la nota de débito necesita al menos un producto o concepto nuevo")
	}

	cuerpo := map[string]interface{}{
		"numeroFactura":    afectada,
		"numeroNotaDebito": numeroDocumento(doc.Numero),
		"descripcion":      motivo,
		"tasa_del_dia":     tasaFormateada(doc.TasaCambio),
		"fecha_tasa":       fechaDeTasa(doc),
		"productos_nuevos": productos,
	}
	respuesta, err := p.c.post(ctx, rutaNotaDebito, cuerpo)
	if err != nil {
		return nil, err
	}
	var datos struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Control string `json:"control_number"`
		PDF     string `json:"debit_note_pdf"`
	}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return nil, fmt.Errorf("la respuesta de CG no es JSON válido: %w", err)
	}
	if !datos.Success || strings.TrimSpace(datos.Control) == "" {
		return nil, fmt.Errorf("CG no emitió la nota de débito: %s", mensajeDe(datos.Message))
	}
	return resultado(datos.Control, datos.PDF, datos.Message, doc.FechaEmision), nil
}

// ---------- Mapeo ERPGo → CG ----------

// serieDe es la serie de numeración con la que se emite: la configurada para la
// imprenta.
//
// NO se toma la serie interna del ERP (invoice.series: "FACT", "CONT"): esa es
// la numeración del ERPGo y la imprenta tiene la suya, asignada por el SENIAT
// ("A", "B"...). Mandar "FACT" como numeroSerie haría que CG rechazara cada
// factura con un mensaje que no explicaría el fondo del problema.
func (p *Proveedor) serieDe() string {
	return strings.TrimSpace(p.cfg.NumeroSerie)
}

// armarFactura construye el objeto `facturas[]` del API.
func (p *Proveedor) armarFactura(doc *di.DatosDocumento) (map[string]interface{}, error) {
	if doc.Numero <= 0 {
		return nil, fmt.Errorf("el documento no tiene número correlativo con el que emitirse")
	}
	if err := validarTasaYMoneda(doc); err != nil {
		return nil, err
	}
	identidad, err := identidadDe(doc.Cliente.FiscalRegistry, doc.Cliente.TipoDocFiscal)
	if err != nil {
		return nil, err
	}
	nombre := strings.TrimSpace(doc.Cliente.Nombre)
	if nombre == "" {
		return nil, fmt.Errorf("el cliente del documento no tiene nombre o razón social")
	}
	correo := strings.TrimSpace(doc.Cliente.Email)
	if correo == "" {
		return nil, fmt.Errorf("el cliente «%s» no tiene correo en su ficha: CG lo exige para emitir la factura", nombre)
	}
	direccion := strings.TrimSpace(doc.Cliente.Direccion)
	if direccion == "" {
		return nil, fmt.Errorf("el cliente «%s» no tiene dirección fiscal en su ficha: CG la exige para emitir la factura", nombre)
	}
	telefono := strings.TrimSpace(doc.Cliente.Telefono)
	if telefono == "" {
		return nil, fmt.Errorf("el cliente «%s» no tiene teléfono en su ficha: CG lo exige para emitir la factura", nombre)
	}
	productos, err := productosDe(doc.Detalles, true)
	if err != nil {
		return nil, err
	}
	if len(productos) == 0 {
		return nil, fmt.Errorf("la factura no tiene renglones que emitir")
	}

	factura := map[string]interface{}{
		"numeroFactura":             numeroDocumento(doc.Numero),
		"documentoIdentidadCliente": identidad,
		"nombreRazonSocialCliente":  recortar(nombre, 400),
		"correoCliente":             correo,
		"direccionCliente":          recortar(direccion, 300),
		"telefonoCliente":           telefono,
		"tasa_del_dia":              tasaFormateada(doc.TasaCambio),
		"fecha_tasa":                fechaDeTasa(doc),
		"productos":                 productos,
	}
	// `sucursal` es opcional y la imprenta la imprime bajo la dirección fiscal.
	// En Unidigital el mismo campo es un identificador técnico; aquí es el
	// nombre legible de la sucursal, que es lo que CG espera.
	if s := strings.TrimSpace(p.Sucursal()); s != "" {
		factura["sucursal"] = recortar(s, 300)
	}
	if d := strings.TrimSpace(doc.Descripcion); d != "" {
		factura["descripcionFactura"] = recortar(d, 500)
	}
	return factura, nil
}

// Sucursal devuelve la sucursal configurada.
func (p *Proveedor) Sucursal() string { return strings.TrimSpace(p.cfg.SucursalID) }

// validarTasaYMoneda deja pasar solo documentos que CG pueda representar.
func validarTasaYMoneda(doc *di.DatosDocumento) error {
	moneda := strings.ToUpper(strings.TrimSpace(doc.Moneda))
	if moneda != "" && moneda != "VES" {
		return fmt.Errorf("CG Imprenta Digital emite únicamente en bolívares (VES) y la factura está en %s: no se puede emitir por esta imprenta", moneda)
	}
	if doc.TasaCambio <= 0 {
		return fmt.Errorf("falta la tasa de cambio del día (Bs/USD): regístrela en Configuración → Tasa BCV antes de emitir por CG Imprenta Digital")
	}
	return nil
}

// datosDeNota valida lo común a las notas y devuelve la factura afectada y el
// motivo ya recortado al máximo de la API.
func datosDeNota(doc *di.DatosDocumento) (afectada, motivo string, err error) {
	afectada = strings.TrimSpace(doc.DocumentoAfectado)
	if afectada == "" || !soloDigitos(afectada) {
		return "", "", fmt.Errorf("la nota necesita el número de la factura que afecta (solo dígitos)")
	}
	if doc.Numero <= 0 {
		return "", "", fmt.Errorf("la nota no tiene número correlativo con el que emitirse")
	}
	motivo = strings.TrimSpace(doc.Descripcion)
	if motivo == "" {
		return "", "", fmt.Errorf("la nota necesita su motivo (descripción)")
	}
	// CG acepta hasta 50 caracteres; se recorta aquí porque el motivo es texto
	// libre del ERP y perder la nota entera por un carácter de más no tiene
	// sentido operativo.
	return afectada, recortar(motivo, 50), nil
}

// productosDe convierte los renglones del ERP. Los precios van en bolívares sin
// IVA, tal como los guarda el ERP; el IVA lo calcula CG por alícuota.
func productosDe(detalles []di.DetalleItem, conDescuento bool) ([]interface{}, error) {
	salida := make([]interface{}, 0, len(detalles))
	for i, d := range detalles {
		if d.Cantidad <= 0 {
			return nil, fmt.Errorf("el renglón %d (%s) tiene cantidad cero o negativa: no se puede emitir", i+1, textoDeRenglon(d))
		}
		if d.PrecioUnitario <= 0 {
			return nil, fmt.Errorf("el renglón %d (%s) tiene precio cero: CG Imprenta Digital no admite renglones a precio cero", i+1, textoDeRenglon(d))
		}
		nombre := strings.TrimSpace(d.Nombre)
		if nombre == "" {
			nombre = strings.TrimSpace(d.Descripcion)
		}
		descripcion := strings.TrimSpace(d.Descripcion)
		if descripcion == "" {
			descripcion = nombre
		}
		item := map[string]interface{}{
			"codigoProducto":      codigoDe(d.Codigo, i+1),
			"nombreProducto":      recortar(nombre, 400),
			"descripcionProducto": recortar(descripcion, 400),
			"tipoImpuesto":        impuestoDe(d),
			"cantidadAdquirida":   cantidad(d.Cantidad),
			"precioProducto":      monto(d.PrecioUnitario),
		}
		if conDescuento && d.DescuentoPct > 0 && d.DescuentoPct < 100 {
			item["discount"] = d.DescuentoPct
		}
		salida = append(salida, item)
	}
	return salida, nil
}

// textoDeRenglon arma el nombre legible de un renglón para los mensajes de error.
func textoDeRenglon(d di.DetalleItem) string {
	if s := strings.TrimSpace(d.Descripcion); s != "" {
		return recortar(s, 60)
	}
	if s := strings.TrimSpace(d.Nombre); s != "" {
		return recortar(s, 60)
	}
	return "sin descripción"
}

// impuestoDe traduce el porcentaje del renglón a la alícuota de CG. Los rangos
// existen porque el ERP guarda el IVA como porcentaje calculado y no como
// código: 16 % es general, 8 % reducida y 31 % adicional; cualquier otra cosa
// que no genera IVA se emite como exento.
func impuestoDe(d di.DetalleItem) string {
	if strings.TrimSpace(d.CodigoImpuesto) == "E" || d.EsExento {
		return "E"
	}
	p := d.PorcentajeIVA
	switch {
	case p >= 30:
		return "A"
	case p >= 15:
		return "G"
	case p >= 7:
		return "R"
	default:
		return "E"
	}
}

// resultado arma la respuesta del contrato con el control completo.
func resultado(control, pdf, mensaje string, fecha time.Time) *di.ResultadoEmision {
	control = strings.TrimSpace(control)
	pdf = strings.TrimSpace(pdf)
	serie, numero := partirControl(control)
	return &di.ResultadoEmision{
		StrongID:        idDeEnlace(pdf),
		NroControl:      numero,
		NroControlTexto: control,
		Serie:           serie,
		Status:          "Approved",
		Mensaje:         mensaje,
		FechaEmision:    fecha.Format(time.RFC3339),
		PDFURL:          pdf,
		EsAsignado:      true,
	}
}

// partirControl separa "00-00000056" en serie y número. Cuando el número no
// trae serie se devuelve la serie vacía y el número completo.
func partirControl(control string) (serie string, numero int) {
	control = strings.TrimSpace(control)
	if control == "" {
		return "", 0
	}
	if i := strings.LastIndex(control, "-"); i >= 0 {
		return strings.TrimSpace(control[:i]), enteroDeTexto(control[i+1:])
	}
	return "", enteroDeTexto(control)
}

// idDeEnlace saca el identificador público del documento del enlace del PDF
// (readonly/export_pdf/<uuid>). Es lo único estable que CG devuelve como
// identificador y sirve para guardar strong_id sin inventar un GUID propio.
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

// identidadDe normaliza el RIF a lo que CG exige: una letra V/E/P/J/G/C y de 6
// a 10 dígitos, sin guiones ni puntos.
func identidadDe(rif, tipoDoc string) (string, error) {
	crudo := soloAlfanumerico(rif)
	letra, digitos := "", ""
	if crudo != "" {
		if r := []rune(crudo)[0]; unicode.IsLetter(r) {
			letra = strings.ToUpper(string(r))
			digitos = string([]rune(crudo)[1:])
		} else {
			digitos = crudo
		}
	}
	if letra == "" {
		letra = strings.ToUpper(strings.TrimSpace(tipoDoc))
	}
	if !soloDigitos(digitos) || len(digitos) == 0 {
		return "", fmt.Errorf("el documento de identidad del cliente «%s» no tiene una parte numérica válida: cárguelo en Clientes (letra V/E/P/J/G/C y 6 a 10 dígitos)", strings.TrimSpace(rif))
	}
	if !strings.Contains("VEPJGC", letra) || len(letra) != 1 {
		return "", fmt.Errorf("el documento de identidad del cliente «%s» no empieza por una letra válida (V, E, P, J, G o C)", strings.TrimSpace(rif))
	}
	if len(digitos) < 6 || len(digitos) > 10 {
		return "", fmt.Errorf("el documento de identidad del cliente «%s» tiene %d dígitos: CG exige entre 6 y 10 (sin guiones ni puntos)", strings.TrimSpace(rif), len(digitos))
	}
	return letra + digitos, nil
}

// codigoDe deja un código de producto que CG acepta: máximo 13 caracteres, sin
// espacios y sin signos que su catálogo no admite. Cuando el código del ERP no
// cabe, se usa uno estable derivado del número de renglón para no cambiar de
// código entre facturas del mismo producto.
func codigoDe(codigo string, renglon int) string {
	var limpio []rune
	for _, r := range strings.TrimSpace(codigo) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			limpio = append(limpio, r)
		}
		if len(limpio) == 13 {
			break
		}
	}
	if len(limpio) == 0 {
		return fmt.Sprintf("RENGLON-%d", renglon)
	}
	return string(limpio)
}

// numeroDocumento es el número que el ERP pone al documento, solo dígitos.
func numeroDocumento(numero int) string {
	if numero <= 0 {
		return "0"
	}
	return strconv.Itoa(numero)
}

// fechaDeTasa usa el día de la tasa; si no vino, el de emisión. CG acepta
// DD-MM-YYYY.
func fechaDeTasa(doc *di.DatosDocumento) string {
	fecha := doc.FechaTasa
	if fecha.IsZero() {
		fecha = doc.FechaEmision
	}
	if fecha.IsZero() {
		fecha = time.Now()
	}
	return fecha.Format("02-01-2006")
}

// monto formatea un importe con los dos decimales que CG exige.
func monto(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// tasaFormateada conserva los cuatro decimales de la tasa de cambio. Redondear
// la tasa a dos decimales cambiaría el equivalente en divisas que imprime la
// factura (y la tasa del BCV se publica con cuatro).
func tasaFormateada(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

// cantidad formatea una cantidad sin ceros de relleno (CG acepta hasta la
// precisión que se envíe, con punto decimal).
func cantidad(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// recortar limita un texto por CARACTERES (no bytes): los acentos y la eñe del
// castellano ocuparían dos bytes y un límite en bytes partiría una palabra.
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

// soloAlfanumerico deja solo letras y dígitos: quita guiones, puntos y espacios
// del RIF tal como lo exige la convención de CG.
func soloAlfanumerico(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// enteroDeTexto saca el valor numérico de un identificador ("0000123" → 123).
func enteroDeTexto(s string) int {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			continue
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// mensajeDe normaliza el mensaje de la API para los errores propios.
func mensajeDe(m string) string {
	if s := strings.TrimSpace(m); s != "" {
		return s
	}
	return "el proveedor no explicó el motivo"
}

// tipoDocumentoCG traduce el tipo del contrato al nombre que usa CG en su
// listado de documentos.
func tipoDocumentoCG(tipo di.DocumentType) (string, bool) {
	switch tipo {
	case di.DocFactura:
		return "FACTURA", true
	case di.DocNotaCredito:
		return "NOTA DE CREDITO", true
	case di.DocNotaDebito:
		return "NOTA DE DEBITO", true
	case di.DocGuiaDespacho:
		return "GUIA DE DESPACHO", true
	default:
		return "", false
	}
}
