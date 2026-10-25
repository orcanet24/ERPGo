package digitalinvoice

import (
	"fmt"
	"sync"

	provider "erpgow/interfaces/web/digitalinvoice"
	"erpgow/interfaces/web/digitalinvoice/unidigital"
)

var (
	muProveedor    sync.RWMutex
	proveedorActivo provider.DigitalInvoiceProvider
)

// RegistrarProvider establece el proveedor activo.
func RegistrarProvider(p provider.DigitalInvoiceProvider) {
	muProveedor.Lock()
	defer muProveedor.Unlock()
	proveedorActivo = p
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
