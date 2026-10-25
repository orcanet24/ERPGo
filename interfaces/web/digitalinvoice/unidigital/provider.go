// Implementación del proveedor Unidigital (Corporación Unidigital 1220 C.A.)
package unidigital

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"erpgow/interfaces/web/digitalinvoice"
)

// Estructura de respuesta genérica de Unidigital
type respuestaUnidigital struct {
	Result      interface{} `json:"result"`
	Errores     []errorUnidigital `json:"errors"`
	Exitos      []interface{} `json:"success"`
	Informacion []string      `json:"information"`
	HasErrors   bool          `json:"haserrors"`
}

type errorUnidigital struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Extra     string `json:"extra"`
}

type emisionResultadoUnidigital struct {
	StrongID string `json:"resultado"`
}

// Proveedor Unidigital
type UnidigitalProvider struct {
	client *digitalinvoice.HTTPClient
	cfg    digitalinvoice.ProviderConfig
}

// NuevoProveedor crea un proveedor Unidigital con la configuración dada.
func NuevoProveedor(cfg digitalinvoice.ProviderConfig) (*UnidigitalProvider, error) {
	client := digitalinvoice.NuevoHTTPClient(cfg.BaseURL, cfg.BearerToken)
	return &UnidigitalProvider{
		client: client,
		cfg:    cfg,
	}, nil
}

// Codigo retorna el identificador único del proveedor.
func (p *UnidigitalProvider) Codigo() string { return "unidigital" }

// Nombre retorna el nombre legible.
func (p *UnidigitalProvider) Nombre() string { return "Unidigital" }

// Config retorna la configuración actual.
func (p *UnidigitalProvider) Config() digitalinvoice.ProviderConfig { return p.cfg }

// ActualizarConfig actualiza la configuración del proveedor.
func (p *UnidigitalProvider) ActualizarConfig(cfg digitalinvoice.ProviderConfig) {
	p.cfg = cfg
	p.client = digitalinvoice.NuevoHTTPClient(cfg.BaseURL, cfg.BearerToken)
}

// Ping verifica conexión con la API.
func (p *UnidigitalProvider) Ping(ctx context.Context) error {
	_, _, err := p.client.Do(ctx, "GET", "/series/list", nil)
	return err
}

// ListarSeries lista las series configuradas en Unidigital.
func (p *UnidigitalProvider) ListarSeries(ctx context.Context) ([]digitalinvoice.Serie, error) {
	body, _, err := p.client.Do(ctx, "POST", "/series/list", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result []serieUnidigital `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	var series []digitalinvoice.Serie
	for _, s := range resp.Result {
		series = append(series, digitalinvoice.Serie{
			StrongID: s.StrongID,
			Name:     s.Name,
		})
	}
	return series, nil
}

type serieUnidigital struct {
	StrongID string `json:"strongId"`
	Name     string `json:"name"`
}

// ListarMetodosPago lista las formas de pago disponibles.
func (p *UnidigitalProvider) ListarMetodosPago(ctx context.Context) ([]digitalinvoice.MetodoPago, error) {
	// Unidigital tiene métodos estándar predefinidos
	return []digitalinvoice.MetodoPago{
		{Codigo: "cash", Descripcion: "Efectivo"},
		{Codigo: "p2p", Descripcion: "P2P / Transferencia"},
		{Codigo: "card", Descripcion: "Tarjeta"},
		{Codigo: "custom", Descripcion: "Personalizado"},
		{Codigo: "multiple", Descripcion: "Multipago"},
	}, nil
}

// Emitir crea y aprueba un documento fiscal en Unidigital.
func (p *UnidigitalProvider) Emitir(ctx context.Context, doc *digitalinvoice.DatosDocumento) (*digitalinvoice.ResultadoEmision, error) {
	// Mapeo de documento ERPGo al formato Unidigital
	payload := p.mapearDocumento(doc)

	respBody, _, err := p.client.Do(ctx, "POST", "/documents/createandapprove", payload)
	if err != nil {
		return nil, fmt.Errorf("error emitiendo documento: %w", err)
	}

	var resp struct {
		Result      string   `json:"result"`
		Information []string `json:"information"`
		HasErrors   bool     `json:"haserrors"`
		Errors      []errorUnidigital `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}

	if resp.HasErrors && len(resp.Errors) > 0 {
		return nil, fmt.Errorf("error de Unidigital: %s", resp.Errors[0].Message)
	}

	return &digitalinvoice.ResultadoEmision{
		StrongID:   resp.Result,
		Status:     "Approved",
		Mensaje:    resp.Information[0],
		FechaEmision: doc.FechaEmision.Format(time.RFC3339),
		EsAsignado: false,
	}, nil
}

// Consultar busca un documento por su GUID.
func (p *UnidigitalProvider) Consultar(ctx context.Context, strongID string) (*digitalinvoice.DocumentoConsulta, error) {
	// TODO: implementar con endpoint GET /documents?strongId=xxx
	return nil, fmt.Errorf("no implementado")
}

// ConsultarPorNroControl busca un documento por número de control.
func (p *UnidigitalProvider) ConsultarPorNroControl(ctx context.Context, nroControl int) (*digitalinvoice.DocumentoConsulta, error) {
	// TODO: implementar con endpoint GET /documents/controlnumber/xxx
	return nil, fmt.Errorf("no implementado")
}

// UltimoDocumento obtiene el último documento emitido de un tipo y serie.
func (p *UnidigitalProvider) UltimoDocumento(ctx context.Context, tipo digitalinvoice.DocumentType, serie string) (*digitalinvoice.DocumentoConsulta, error) {
	// TODO: implementar con endpoint GET /documents/last
	return nil, fmt.Errorf("no implementado")
}

// Anular anula un documento fiscal por número de control.
func (p *UnidigitalProvider) Anular(ctx context.Context, nroControl int) error {
	// TODO: implementar con endpoint POST /documents/anulled
	return fmt.Errorf("no implementado")
}

// mapearDocumento convierte un documento ERPGo al formato Unidigital.
func (p *UnidigitalProvider) mapearDocumento(doc *digitalinvoice.DatosDocumento) map[string]interface{} {
	// Mapeo de detalles
	detalles := make([]map[string]interface{}, len(doc.Detalles))
	for i, d := range doc.Detalles {
		detalles[i] = map[string]interface{}{
			"Description":     d.Descripcion,
			"Quantity":        d.Cantidad,
			"UnitPrice":       d.PrecioUnitario,
			"Amount":          d.Cantidad * d.PrecioUnitario,
			"TaxAmount":       d.PorcentajeIVA * d.Cantidad * d.PrecioUnitario / 100,
			"TaxPercent":      d.PorcentajeIVA,
			"TaxCode":         d.CodigoImpuesto,
			"IsExempt":        d.EsExento,
			"OperationCode":   d.CodigoOperacion,
			"AmountPlusDiscount": d.Cantidad * d.PrecioUnitario,
			"TotalAmount":     d.Cantidad * d.PrecioUnitario,
			"ProductType":     d.TipoProducto,
		}
	}

	// Mapeo de pagos
	pagos := make([]map[string]interface{}, len(doc.Pagos))
	for i, p := range doc.Pagos {
		pagos[i] = map[string]interface{}{
			"PaymentMethod":     p.MetodoPago,
			"Currency":          p.Moneda,
			"ConversionCurrency": "VES",
			"Amount":            p.Monto,
			"AmountVES":         p.Monto * p.TasaCambio,
			"PaymentDate":       p.FechaPago,
			"ExchangeRate":      p.TasaCambio,
			"Description":       p.Descripcion,
		}
	}

	return map[string]interface{}{
		"SerieStrongId":          p.cfg.SerieStrongID,
		"SucursalStrongId":       p.cfg.SucursalID,
		"DocumentType":           string(doc.Tipo),
		"Number":                 doc.Numero,
		"EmissionDateAndTime":    doc.FechaEmision.Format(time.RFC3339),
		"Name":                   doc.Cliente.Nombre,
		"FiscalRegistryCode":     doc.Cliente.TipoDocFiscal,
		"FiscalRegistry":         doc.Cliente.FiscalRegistry,
		"Address":                doc.Cliente.Direccion,
		"Phone":                  doc.Cliente.Telefono,
		"EmailTo":                doc.Cliente.Email,
		"PaymentType":            doc.TipoPago,
		"Currency":               doc.Moneda,
		"ExemptAmount":           doc.MontoExento,
		"TaxBase":                doc.BaseImponible,
		"TaxBaseReduced":         doc.BaseReducida,
		"TaxBaseSumptuary":       doc.BaseSuntuario,
		"Subtotal":               doc.Subtotal,
		"Discount":               doc.Descuento,
		"PreviousBalance":        0,
		"SubtotalPlusDiscount":   doc.Subtotal - doc.Descuento,
		"TaxPercent":             16,
		"TaxPercentReduced":      8,
		"TaxPercentSumptuary":    31,
		"TaxAmount":              doc.IVA,
		"TaxAmountReduced":       doc.IVAReducido,
		"TaxAmountSumptuary":     doc.IVASuntuario,
		"Taxes":                  doc.IVA + doc.IVAReducido + doc.IVASuntuario,
		"Total":                  doc.Total,
		"IGTFBaseAmount":         doc.Total,
		"IGTFAmount":             doc.IGTF,
		"IGTFPercentage":         3,
		"Tip":                    doc.Propina,
		"GrandTotal":             doc.TotalFinal + doc.Propina,
		"AmountLetters":          "",
		"ConversionCurrency":     "VES",
		"SubtotalVES":            doc.Subtotal * doc.TasaCambio,
		"DiscountVES":            doc.Descuento * doc.TasaCambio,
		"ExemptAmountVES":        doc.MontoExento * doc.TasaCambio,
		"TaxBaseVES":             doc.BaseImponible * doc.TasaCambio,
		"TaxBaseReducedVES":      doc.BaseReducida * doc.TasaCambio,
		"TaxAmountVES":           doc.IVA * doc.TasaCambio,
		"TaxAmountReducedVES":    doc.IVAReducido * doc.TasaCambio,
		"TotalVES":               doc.Total * doc.TasaCambio,
		"IGTFBaseAmountVES":      doc.Total * doc.TasaCambio,
		"IGTFAmountVES":          doc.IGTF * doc.TasaCambio,
		"GrandTotalVES":          (doc.TotalFinal + doc.Propina) * doc.TasaCambio,
		"ExchangeRate":           doc.TasaCambio,
		"SystemReference":        doc.ReferenciaSistema,
		"Note1":                  "",
		"Note2":                  "",
		"Note3":                  "",
		"Payments":               pagos,
		"Details":                detalles,
	}
}
