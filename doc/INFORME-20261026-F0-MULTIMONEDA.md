# INFORME F0 — Auditoría de multi-moneda · 2026-10-26

Alcance: verificación de los cuatro huecos listados en el plan
`PLAN-20261026-CATALOGO-PRODUCCION-MULTIMONEDA-DEPRECACION.md` (Punto 5):
valorización de kardex, listas de precio, sync POS y reportes.
Auditoría de código (no funcional contra servidor con datos).

---

## Lo que está BIEN (no se toca)

1. **Facturación ERP** (`interfaces/web/invoice.go:355-404`): guarda
   `currency_code`, `exchange_rate` de la fecha con fallback a la última tasa
   conocida (`ObtenerTasaHoy`), `total_amount_usd` y fuente de tasa
   (`exchange_rate_source`). El IGTF se calcula sobre el total extranjero y se
   recalcula el USD después. La tasa aplicada queda auditada
   (`exchange_rate_audit`).
2. **POS terminal** (`interfaces/web/pos_api.go:336-411`): toma la tasa BCV del
   día, guarda total USD y fuente en la factura del POS. Correcto.
3. **Libro de Diferencial Cambiario**: existe como libro legal propio.
4. **Convención de valoración**: costos, kardex y lotes se guardan en VES (una
   sola moneda de valoración, la contable). Es la decisión correcta; el
   problema no es la convención, es que dos caminos la incumplen (abajo).

## Huecos encontrados

### H1 — Compra en USD escribe su precio SIN convertir en el kardex (grave)

`interfaces/web/purchase.go:404-415` guarda `product_purchase_details.rate` y
`stock_movement.unit_cost` con el `rate` tal cual se escribió en la pantalla.
Si la compra está en USD, ese número son dólares.

Efecto: `kardex.go:164` suma `quantity * unit_cost` — con una compra en USD, la
suma queda en dólares mezclados con bolívares de las demás compras. Lo mismo
contamina el promedio histórico de `costoVentaDeProducto()`
(`invoice.go:817`, `AVG(rate)` de `product_purchase_details`).

Ubicación exacta del error: el cabecera SÍ convierte bien (`purchase.go:243-278`
guarda moneda y tasa), pero el renglón no usa `tasaBCV` para convertir su
`rate` antes de escribirlo.

### H2 — Solo VES y USD; cualquier otra moneda guarda tasa 1

`purchase.go:247-261` e `invoice.go:360-372` comparan contra `"VES"` y `"USD"`
literal. Con EUR (o cualquier moneda del catálogo que no sea esas dos),
`total_amount_usd` queda 0 y `exchange_rate` queda 1: los totales en USD
saldrían mal. `ObtenerTasaHoy` ya es genérico por moneda; el if no lo es.

### H3 — Las compras no crean lotes (medio)

`product_purchase` tiene `warehouse_id` y los detalles tienen `batch_id`, pero
el alta de compra nunca llama a `RegistrarLote` (`inventory_fefo.go:99`); solo
lo hace la pantalla manual de lotes y las semillas. Consecuencia: el inventario
FEFO y el costo-por-lote (la primera fuente de `costoVentaDeProducto`) solo
existen si alguien registra los lotes a mano después de cada compra.

### H4 — Reportes de compras mezclan monedas sin convertir (medio)

`interfaces/web/report.go:89-90` lista `grand_total_amount` directamente y los
totales suman sin mirar `currency_code`. Con H1 presente (compras en USD), el
reporte de compras suma bolívares y dólares en la misma cifra.

### H5 — Listas de precio sin moneda (bajo)

`product_price_list.price` no lleva moneda ni hay lista por moneda: no se puede
tener "Mayor VES" y "Mayor USD" como listas separadas. Decisión de diseño
pendiente (columna `currency` con default VES, o listas por moneda).

### H6 — Sync POS: OK

El catálogo y las ventas del POS manejan moneda y tasa en el servidor ERP
(`internal/pos/sync_server.go:147`, `pos_api.go`). No requiere cambio aquí; el
ajuste del POS cliente, si se necesita, es del repositorio POSGo.

## Cierre propuesto (pendiente de aprobación — NO aplicado aún)

| # | Corrección | Archivo | Esfuerzo |
|---|-----------|---------|----------|
| F0.1 | Convertir el `rate` del renglón a VES con la tasa ya obtenida antes de escribir detalle y kardex (usando `ConvertirUSDaVES`, que ya existe en `multi_currency.go`) | `purchase.go` | 0.5 día |
| F0.2 | Quitar el `if == "USD"`: usar la tasa genérica de la moneda elegida en compra y factura | `purchase.go`, `invoice.go` | 0.5 día |
| F0.3 | Crear el lote automáticamente desde la compra (misma transacción) cuando el producto lleve `track_lots` | `purchase.go` | 1 día |
| F0.4 | Convertir totales de reportes de compras/ventas a la moneda de reporte | `report.go` | 1 día |
| F0.5 | Moneda en listas de precio (columna + default VES) | migración + `price_list.go` | 1 día |

**Total: 4 días.** Riesgo: medio-bajo; F0.1 y F0.4 tocan zonas de costo y
reportes y deben probarse contra copia de la base de producción.

## Limitación de esta auditoría

Es auditoría de código estático. No pude ejecutar el sistema contra una base
con datos reales (el sandbox no tiene MySQL). Cada hallazgo trae su fichero y
línea para que se pueda reproducir a mano sobre su servidor.
