package digitalinvoice

// registry.go — Las imprentas digitales que el ERP puede montar en memoria.
//
// Hay dos registros y NO son lo mismo:
//
//   - La verdad de qué imprenta emite es la BASE (digital_invoice_config, una
//     sola fila enabled=1 por empresa). La cola de emisión lee de ahí, no de
//     aquí: así un reinicio del ERP vuelve a emitir con la imprenta correcta
//     sin que nadie toque nada.
//   - Este registro es una CACHÉ por empresa para las pantallas y las pruebas
//     de conexión. Se puebla al activar y se limpia al desactivar.
//
// La confusión entre los dos fue un fallo real: al guardar la configuración de
// una imprenta deshabilitada se borraba de memoria la que SÍ estaba activa (el
// mapa se vaciaba por empresa sin mirar el código). Ahora desactivar exige
// nombrar la imprenta que se desactiva.

import (
	"fmt"
	"sync"

	provider "erpgo/interfaces/web/digitalinvoice"
	"erpgo/interfaces/web/digitalinvoice/cgimprenta"
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

// DesactivarProveedorPorTenant retira el proveedor activo de una empresa SOLO si
// el que está en memoria es el código indicado.
//
// Sin esta comprobación, guardar (o desactivar) la configuración de la imprenta
// que NO estaba activa borraba de memoria la que sí lo estaba: la pantalla
// seguía diciendo "sin proveedor activo" hasta el siguiente reinicio, aunque la
// base dijera lo contrario.
func DesactivarProveedorPorTenant(tenant, codigo string) {
	muProveedor.Lock()
	defer muProveedor.Unlock()
	p := proveedoresPorTenant[tenant]
	if p == nil || p.Codigo() != codigo {
		return
	}
	delete(proveedoresPorTenant, tenant)
}

// ProveedorActivo retorna el proveedor actual (el último registrado, sin
// empresa). Se conserva para las herramientas internas que no tienen ámbito.
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
//
// Esta es la lista de imprentas que el ERP sabe montar. Añadir una es añadir su
// caso aquí y su catálogo en interfaces/web/digital_invoice_settings.go: el
// resto del sistema (cola, pantalla, exclusividad) no cambia.
func NuevoProveedorActivo(codigo string, cfg provider.ProviderConfig) (provider.DigitalInvoiceProvider, error) {
	switch codigo {
	case "unidigital":
		return unidigital.NuevoProveedor(cfg)
	case "cgimprenta":
		return cgimprenta.NuevoProveedor(cfg)
	default:
		return nil, fmt.Errorf("proveedor de facturación digital no soportado: %s", codigo)
	}
}
