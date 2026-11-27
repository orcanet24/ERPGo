// Package digitalinvoice define la interfaz estándar para proveedores de facturación digital.
// Cualquier casa de facturación autorizada por SENIAT debe implementar DigitalInvoiceProvider.
//
// El ERP emite por UN proveedor a la vez (por empresa): el que esté activo en
// digital_invoice_config. Por eso aquí vive el contrato COMPARTIDO y nada más:
// cada imprenta (Unidigital, CG Imprenta Digital, la que venga) traduce este
// vocabulario al de su API en su propio paquete. Si el contrato se llenara de
// campos de una sola imprenta, la siguiente obligaría a cambiar el emisor
// entero; los campos que aquí aparecen existen porque al menos dos documentos
// fiscales venezolanos los piden (RIF sin guiones, tasa BCV del día, serie).
package digitalinvoice

import (
	"context"
	"time"
)

// DocumentType representa el tipo de documento fiscal soportado.
type DocumentType string

const (
	DocFactura       DocumentType = "FA"
	DocNotaCredito   DocumentType = "NC"
	DocNotaDebito    DocumentType = "ND"
	DocGuiaDespacho  DocumentType = "GD"
	DocRetencionIVA  DocumentType = "RI"
	DocRetencionISLR DocumentType = "RR"
)

// ProviderConfig es la configuración común a todos los proveedores.
//
// Los campos de credencial no los usa todo el mundo: Unidigital entrega un
// BearerToken ya emitido y CG entrega usuario y contraseña de API (el token lo
// pide el propio adaptador y caduca cada hora). Guardar los dos juegos en la
// misma fila es lo que permite cambiar de imprenta sin perder la configuración
// de la anterior: desactivar no es borrar.
type ProviderConfig struct {
	Code          string `json:"code"`     // Código único del proveedor (unidigital, cgimprenta, ...)
	Name          string `json:"name"`     // Nombre legible
	Enabled       bool   `json:"enabled"`  // Si está activo
	BaseURL       string `json:"base_url"` // URL base de la API
	BearerToken   string `json:"bearer_token"`
	UserName      string `json:"user_name"`     // Usuario de API (CG Imprenta Digital)
	UserPassword  string `json:"user_password"` // Contraseña de API (CG Imprenta Digital)
	SerieStrongID string `json:"serie_id"`      // Serie técnica en la imprenta (Unidigital)
	NumeroSerie   string `json:"numero_serie"`  // Serie de numeración del documento (CG: "A")
	SucursalID    string `json:"sucursal_id"`   // Sucursal (técnica en Unidigital, impresa en CG)
}

// Serie representa una serie de facturación configurada en la imprenta.
type Serie struct {
	StrongID  string `json:"strong_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// MetodoPago representa una forma de pago registrada.
type MetodoPago struct {
	Codigo      string `json:"codigo"`
	Descripcion string `json:"descripcion"`
}

// DatosCliente contiene la información fiscal del cliente para el documento.
type DatosCliente struct {
	Nombre         string `json:"nombre"`
	FiscalRegistry string `json:"rif"` // J-12345678-1 (el adaptador lo normaliza)
	Direccion      string `json:"direccion"`
	Telefono       string `json:"telefono"`
	Email          string `json:"email"`
	TipoDocFiscal  string `json:"tipo_doc"` // V, J, E, P, G, C
}

// DetalleItem representa una línea del documento fiscal.
//
// Codigo es el código del producto en el ERPGo. Existe como campo propio —y no
// se deduce de la descripción— porque las imprentas lo exigen y porque es la
// llave con la que registran el producto en su catálogo: sin él, la misma
// mercancía se duplica en la imprenta con cada factura.
type DetalleItem struct {
	Codigo          string  `json:"codigo"`
	Nombre          string  `json:"nombre"`
	Descripcion     string  `json:"descripcion"`
	Cantidad        float64 `json:"cantidad"`
	PrecioUnitario  float64 `json:"precio_unitario"` // sin IVA, en la moneda del documento
	DescuentoPct    float64 `json:"descuento_pct"`   // porcentaje del renglón (0..99)
	CodigoImpuesto  string  `json:"codigo_impuesto"` // E (exento), G (16%), R (8%), A (31%), Z (0%)
	PorcentajeIVA   float64 `json:"porcentaje_iva"`
	EsExento        bool    `json:"es_exento"`
	CodigoOperacion string  `json:"codigo_operacion"`
	TipoProducto    int     `json:"tipo_producto"` // 1=físico, 2=servicio
}

// PagoDetalle representa un pago realizado.
type PagoDetalle struct {
	MetodoPago  string  `json:"metodo_pago"` // cash, p2p, custom, etc.
	Moneda      string  `json:"moneda"`      // VES, USD, EUR
	Monto       float64 `json:"monto"`
	FechaPago   string  `json:"fecha_pago"` // YYYY-MM-DD
	TasaCambio  float64 `json:"tasa_cambio"`
	Descripcion string  `json:"descripcion"`
}

// DatosDocumento contiene toda la información para emitir un documento fiscal.
//
// FechaTasa es el día AL QUE CORRESPONDE TasaCambio, no necesariamente el de
// emisión: una factura emitida el lunes con la tasa del viernes debe imprimir la
// tasa del viernes, y varios proveedores rechazan la tasa sin su fecha.
//
// DocumentoAfectadoFecha y DocumentoAfectadoMonto existen para las NOTAS: hay
// imprentas (THKA) que imprimen la fecha y el monto de la factura afectada y
// rechazan la nota si no se los envían. La emisión de facturas los deja en cero.
type DatosDocumento struct {
	Tipo                   DocumentType  `json:"tipo"`
	Numero                 int           `json:"numero"`             // correlativo interno del documento
	DocumentoAfectado      string        `json:"documento_afectado"` // factura afectada por una NC/ND
	DocumentoAfectadoFecha time.Time     `json:"documento_afectado_fecha"`
	DocumentoAfectadoMonto float64       `json:"documento_afectado_monto"`
	Descripcion            string        `json:"descripcion"` // motivo de la nota
	FechaEmision           time.Time     `json:"fecha_emision"`
	Cliente                DatosCliente  `json:"cliente"`
	Detalles               []DetalleItem `json:"detalles"`
	Pagos                  []PagoDetalle `json:"pagos"`
	Moneda                 string        `json:"moneda"`
	TasaCambio             float64       `json:"tasa_cambio"`
	FechaTasa              time.Time     `json:"fecha_tasa"`
	Subtotal               float64       `json:"subtotal"`
	Descuento              float64       `json:"descuento"`
	MontoExento            float64       `json:"monto_exento"`
	BaseImponible          float64       `json:"base_imponible"`
	BaseReducida           float64       `json:"base_reducida"`
	BaseSuntuario          float64       `json:"base_suntuario"`
	IVA                    float64       `json:"iva"`
	IVAReducido            float64       `json:"iva_reducido"`
	IVASuntuario           float64       `json:"iva_suntuario"`
	Total                  float64       `json:"total"`
	IGTF                   float64       `json:"igtf"`
	TotalFinal             float64       `json:"total_final"`
	Propina                float64       `json:"propina"`
	TipoPago               string        `json:"tipo_pago"` // "Contado", "15 DIAS", etc.
	Notas                  []string      `json:"notas"`
	ReferenciaSistema      string        `json:"referencia_sistema"` // Referencia interna (ej: invoice_id)
}

// ResultadoEmision es la respuesta después de emitir un documento.
//
// NroControl es el número entero que usa Unidigital. NroControlTexto es el
// número COMPLETO que usa CG Imprenta Digital ("00-00000056"): el prefijo es la
// serie del rango de control y perderla al convertir a entero haría que dos
// documentos de series distintas se leyeran igual. Quien persiste el documento
// guarda las dos cosas tal como vienen.
type ResultadoEmision struct {
	StrongID        string `json:"strong_id"`         // GUID o identificador del documento en la imprenta
	NroControl      int    `json:"nro_control"`       // Número de control fiscal (imprentas que lo dan entero)
	NroControlTexto string `json:"nro_control_texto"` // Número de control con su serie ("00-00000056")
	Serie           string `json:"serie"`             // Serie del número de control
	Status          string `json:"status"`            // "Approved", "Assigned", etc.
	Mensaje         string `json:"mensaje"`           // Mensaje del servidor
	FechaEmision    string `json:"fecha_emision"`
	PDFURL          string `json:"pdf_url"`     // URL para descargar PDF fiscal
	XMLURL          string `json:"xml_url"`     // URL para descargar XML
	PDFBase64       string `json:"pdf_base64"`  // PDF en base64 (si está disponible)
	EsAsignado      bool   `json:"es_asignado"` // Si ya tiene número de control
}

// DocumentoConsulta es la información devuelta al consultar un documento.
type DocumentoConsulta struct {
	StrongID      string  `json:"strong_id"`
	Numero        int     `json:"numero"`
	NroControl    int     `json:"nro_control"`
	ControlTexto  string  `json:"control_texto"`
	TipoDoc       string  `json:"tipo_doc"`
	Serie         string  `json:"serie"`
	ClienteRIF    string  `json:"rif_cliente"`
	ClienteNombre string  `json:"nombre_cliente"`
	Moneda        string  `json:"moneda"`
	Subtotal      float64 `json:"subtotal"`
	IVA           float64 `json:"iva"`
	Total         float64 `json:"total"`
	IGTF          float64 `json:"igtf"`
	TotalFinal    float64 `json:"total_final"`
	FechaEmision  string  `json:"fecha_emision"`
	ControlNumber int     `json:"control_number"`
	Status        string  `json:"status"`
	EsAnulado     bool    `json:"es_anulado"`
	PDFURL        string  `json:"pdf_url"`
}

// DigitalInvoiceProvider es la interfaz que deben implementar todas las casas de facturación.
//
// Lo que el ERP usa hoy, de verdad, es Emitir (la cola de emisión) y Ping (la
// pantalla de configuración). El resto del contrato se implementa hasta donde la
// API de cada imprenta lo permite y, cuando no, se devuelve un error que dice
// POR QUÉ: un método que finge éxito es peor que uno que explica su límite.
type DigitalInvoiceProvider interface {
	// Código único del proveedor (ej: "unidigital")
	Codigo() string

	// Nombre legible para el usuario
	Nombre() string

	// Configuración actual
	Config() ProviderConfig

	// Actualizar configuración
	ActualizarConfig(cfg ProviderConfig)

	// Emitir un documento fiscal. Devuelve el resultado con strong_id y estado.
	Emitir(ctx context.Context, doc *DatosDocumento) (*ResultadoEmision, error)

	// Consultar un documento por su GUID
	Consultar(ctx context.Context, strongID string) (*DocumentoConsulta, error)

	// Consultar por número de control
	ConsultarPorNroControl(ctx context.Context, nroControl int) (*DocumentoConsulta, error)

	// Obtener último documento creado (para verificar correlativo)
	UltimoDocumento(ctx context.Context, tipo DocumentType, serie string) (*DocumentoConsulta, error)

	// Anular un documento por número de control
	Anular(ctx context.Context, nroControl int) error

	// Listar series configuradas en la imprenta
	ListarSeries(ctx context.Context) ([]Serie, error)

	// Listar métodos de pago disponibles
	ListarMetodosPago(ctx context.Context) ([]MetodoPago, error)

	// Verificar conexión con la API
	Ping(ctx context.Context) error
}
