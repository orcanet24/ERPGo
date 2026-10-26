# Facturación Digital (SENIAT) — Gaceta Oficial 43.484

## Cómo funciona

- Cada EMPRESA (tenant) elige SU imprenta digital de un catálogo. La ley permite
  solo UNA activa por empresa: al activar una nueva, las demás quedan
  deshabilitadas automáticamente y la pantalla muestra la alerta.
- La config vive en `digital_invoice_config` (base de control), una fila por
  (empresa, proveedor). enabled=1 marca la activa.
- Las facturas (incluidas las del POS emitidas offline) se encolan en
  `digital_invoice_log` (pending) y un runner cada 30 s las emite al proveedor
  activo del momento. Fallos: reintento con límite 8, luego `failed` visible.
- Pantalla: **Configuración → Facturación Digital** (`/settings/digital-invoice`,
  rol admin/master).

## Cómo agregar una nueva imprenta digital (3 pasos, un solo punto cada uno)

1. **Implementar el proveedor**: crear un paquete en
   `interfaces/web/digitalinvoice/<nombre>/provider.go` que implemente la
   interfaz `DigitalInvoiceProvider` (ejemplo: el paquete `unidigital`).
2. **Factoría**: agregar el `case "<codigo>"` en
   `internal/digitalinvoice/registry.go` → `NuevoProveedorActivo`.
3. **Catálogo**: registrar el código y nombre en
   `interfaces/web/digital_invoice_settings.go`:
   - `diProveedorCatalogo` (lista; el select de la pantalla se arma de aquí,
     no hay que tocar el template),
   - `diCatalogoPermitido` (validación de input).

Con eso el proveedor aparece en el catálogo de la pantalla y el runner lo usa
para emitir. El menú no se toca por cada imprenta: la entrada de menú apunta a
la pantalla del catálogo (`migrations/schema_z_zzzzzz_menu_digital_invoice.sql`).

## Reglas duras que no se pueden saltar

- La exclusividad es por empresa: `enabled=1` único dentro de un `company_id`.
- Cambio de imprenta = activar la nueva (la antigua queda OFF con alerta).
- El token del proveedor viaja solo en `config_json` en la base del cliente;
  no se escribe en código ni en logs.
