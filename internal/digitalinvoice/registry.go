package digitalinvoice

import (
	"fmt"
	"sync"

	provider "erpgo/interfaces/web/digitalinvoice"
	"erpgo/interfaces/web/digitalinvoice/unidigital"
)

var (
	muProveedor     sync.RWMutex
	proveedorActivo provider.DigitalInvoiceProvider

	// proveedoresPorTenant mantiene UN proveedor en memoria por empresa.
	// Es clave en multi-empresa: cada empresa usa la empresa de facturación
	// digital que eligió, no la de la última que cambió de pantalla.
	proveedoresPorTenant map[string]provider.DigitalInvoiceProvider
)

// RegistrarProvider establece el proveedor activo (sin empresa asociada).
func RegistrarProvider(p provider.DigitalInvoiceProvider) {
	muProveedor.Lock()
	defer muProveedor.Unlock()
	proveedorActivo = p
}

// RegistrarPorTenant establece el proveedor activo de UNA empresa.
func RegistrarPorTenant(tenant string, p provider.DigitalInvoiceProvider) {
	muProveedor.Lock()
	defer muProveedor.Unlock()
	if proveedoresPorTenant == nil {
		proveedoresPorTenant = map[string]provider.DigitalInvoiceProvider{}
	}
	proveedoresPorTenant[tenant] = p
}

// ProveedorPorTenant retorna el proveedor activo de una empresa, si ya se
// registró. Sin registro no hay proveedor: quien lo necesite lo carga desde
// digital_invoice_config.
func ProveedorPorTenant(tenant string) provider.DigitalInvoiceProvider {
	muProveedor.RLock()
	defer muProveedor.RUnlock()
	return proveedoresPorTenant[tenant]
}

// DesactivarPorTenant retira el proveedor activo de una empresa (por ejemplo,
// cuando el admin deja su configuración en enabled=0).
func DesactivarPorTenant(tenant string) {
	muProveedor.Lock()
	defer muProveedor.Unlock()
	delete(proveedoresPorTenant, tenant)
}

// ProveedorActivo retorna el proveedor actual.
func ProveedorActivo() provider.DigitalInvoiceProvider {
	muProveedor.RLock()
	defer muProveedor.RUnlock()
	return proveedorActivo
}

// HayProveedorActivo retorna true si hay proveedor y está habilitado.
func HayProveedorActivo() bool {
	muProveedor.RLock()
	defer muProveedor.RUnlock()
	return proveedorActivo != nil && proveedorActivo.Config().Enabled
}

// NuevoProveedorActivo crea un proveedor según su código.
func NuevoProveedorActivo(codigo string, cfg provider.ProviderConfig) (provider.DigitalInvoiceProvider, error) {
	switch codigo {
	case "unidigital":
		return unidigital.NuevoProveedor(cfg)
	default:
		return nil, fmt.Errorf("proveedor de facturación digital no soportado: %s", codigo)
	}
}

