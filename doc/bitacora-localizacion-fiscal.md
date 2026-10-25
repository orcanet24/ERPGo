# Bitácora — Localización Fiscal Venezuela + POS Retail (ERPGo)

**Fecha:** 1 de octubre de 2026
**Estado:** IMPLEMENTADO Y VERIFICADO (build ✅ tests ✅ smoke ✅ e2e ✅)

---

## 1. Resumen de lo entregado

Se remodeló ERPGo para convertirlo en un sistema fiscalmente defendible ante el
SENIAT y operativamente útil para retail venezolano:

| Módulo | Estado |
|--------|--------|
| RIF validado (módulo 11 SENIAT) | ✅ clientes, proveedores, facturas, compras, POS |
| Número de control fiscal | ✅ asignado por serie en cada factura/NC |
| Series de documentos fiscales | ✅ FACT, NC, ND, RET-IVA, RET-ISLR (editables) |
| Notas de crédito fiscales | ✅ documento completo + asientos automáticos |
| IGTF en línea | ✅ POS calcula y percibe al cobrar en divisa/cripto |
| Métodos de pago VE | ✅ efectivo, punto, pago móvil, transferencia, Zelle, cripto, crédito |
| Caja cajera (turnos) | ✅ apertura, Reporte X, Cierre Z con arqueo, historial |
| Barcode | ✅ campo en producto + escaneo en POS (Enter) |
| Libros IVA actualizados | ✅ RIF real, control fiscal, NCs incluidas, ex-anuladas |
| Ajustes Fiscales (UI) | ✅ series, alícuota IGTF, pie fiscal, RIF obligatorio |
| Seguridad | ✅ sesión de caja resuelta en servidor; nunca confía en el cliente |

## 2. Modelo de datos nuevo

### Tablas nuevas
- `fiscal_series` — numeración única y consecutiva por tipo de documento.
  Campos: `code, name, prefix, next_no, control_prefix, control_next`.
- `credit_note` + `credit_note_details` — notas de crédito fiscales con
  referencia a la factura afectada (Col. L del libro de ventas).
- `payment_method` — catálogo de métodos VE con `is_foreign` (dispara IGTF)
  y `coa_id` (cuenta contable; auto-asignada al migrar: efectivo→Caja,
  electrónicos→Banco, crédito→Clientes).
- `cash_session` + `cash_session_payment` — turnos de caja con fondo inicial,
  efectivo esperado vs contado, diferencia, notas.
- `invoice_payment_summary` — pagos por factura y método (alimenta X/Z).
- `warehouse` — almacenes (`WH-MAIN` por defecto, prepara multi-bodega).
- `fiscal_setting` — parámetros clave/valor (IGTF on/off, tasa, RIF contado).

### Columnas nuevas
- `customer_information`: `rif`, `customer_type`, `vat_withholding_agent`
- `supplier_information`: `rif`, `supplier_type`, `islr_concept_default`
- `company_information`: `fiscal_address`, `imprenta_digital`, `invoice_series`
- `invoice`: `control_no, customer_rif, series, igtf_amount, igtf_rate,
  currency, exchange_rate, is_anulled, anulled_reason/at, cash_session_id, warehouse_id`
- `product_purchase`: `control_no, supplier_rif, warehouse_id`
- `product_information`: `barcode` (único), `cost`

## 3. Lógica fiscal implementada (interfaces/web/fiscal.go)

- **ValidateRIF**: algoritmo oficial — letra (V=4,E=8,J=12,P=16,G=20) +
  pesos por posición 3,2,7,6,5,4,3,2 → DV = 11 - (suma % 11), DV≥10→0.
  Validado con tests (`fiscal_test.go`, todos pasan).
- **nextFiscalDoc**: consume atómicamente (`SELECT ... FOR UPDATE`) número +
  control de la serie. Formato: `PREFIJO00000001` y `00-00000001`.
- **computeIGTF**: suma pagos en métodos `is_foreign`, aplica tasa (3%), redondea.
- **round2**: redondeo fiscal a 2 decimales (mitad arriba) para IVA/IGTF.
- **SeedFiscal**: crea series y cuentas COA de métodos de pago al migrar.

## 4. Flujo de venta POS (verificado end-to-end)

1. Cajero abre turno con fondo (ej: 50).
2. Escanea barcode o hace clic en producto; carrito en vivo.
3. Cliente contado o búsqueda por nombre/RIF; muestra RIF y advierte si falta.
4. Cobro:
   - **Efectivo/Punto/Pago Móvil**: un clic; si es divisa suma IGTF automáticamente.
   - **Multi-pago**: mezcla métodos; la fila en divisa suma IGTF en vivo.
   - **Crédito**: exige cliente identificado.
5. Servidor valida RIF, asigna número + control fiscal, registra:
   factura → detalles → vouchers (CAJA/BANCO por método + COGS + IVA + IGTF)
   → resumen de pagos por método con la sesión del cajero.
6. Reporte X en vivo por método; Cierre Z declara efectivo contado →
   diferencia (sobrante/faltante) queda en BD y genera asiento.

### Resultados de la prueba real
- Venta 120 por Pago Móvil → control `00-00000001`, asientos correctos.
- Venta 100 por Zelle → IGTF 3.00 (3%), cobro 103, control `00-00000002`.
- Reporte X: Pago Móvil 120 / Zelle 103 / total 223.09.
- Cierre Z contado 50 = esperado 50 → diferencia 0.
- NC por 60 sobre factura 1007 → `NC-00000001`, aparece en libro de ventas.
- Libro de ventas: RIF `V123456781`, control fiscal, NCs del período.

## 5. Seguridad aplicada

- `cash_session_id` del formulario **se ignora**: el servidor usa la sesión
  abierta del usuario autenticado (no se puede atribuir ventas a otra caja).
- RIF se valida server-side (el del navegador es solo ayuda visual).
- facturas nunca se borran: `is_anulled` + NC (cumple "no inalterabilidad").
- El dígito verificador del RIF evita clientes/proveedores inventados.
- Aritmética fiscal con redondeo determinista (`round2`), no floats crudos.

## 6. Correcciones aplicadas sobre el código heredado

- Reporte X no sumaba: `invoice_payment_summary` guardaba el número de negocio
  pero el join usaba el PK interno → ahora guarda el PK (`invoice.id`).
- Ventas POS sin `multipaytype` caían en la vía legacy (caja "Cash In Hand")
  → nueva rama que usa `payment_method.coa_id` real por método.
- Cobro real no incluía IGTF → `paidAmount` ajustado con balance defensivo.
- Sesión de caja desde el formulario (spoofeable) → resuelta en servidor.
- 9 tablas nuevas quedaron con charset del servidor (latin1) → migración a
  `utf8mb4_general_ci` incluida (evita "Incorrect string value" con acentos).

## 7. Cómo usar

```bash
erpgow.exe --migrate   # aplica tablas/columnas/series/métodos de pago
erpgow.exe             # inicia el servidor
```
- **Caja:** menú "Caja → Mi Turno" (abrir con fondo) — el POS exige turno abierto.
- **Nota de crédito:** menú "Notas de Crédito" → seleccionar factura → motivo → emitir.
- **Ajustes Fiscales:** Ajustes → Fiscales (SENIAT): series, IGTF, pie fiscal.
- **Barcode:** cargar en el producto; en el POS escanear + Enter.
- **RIF del emisor:** Ajustes → Empresa (se imprime en la factura).

## 8. Pendiente (fases siguientes del plan ya aprobado)

1. Impresión térmica ESC/POS 80mm vía agente local en la PC de caja.
2. Multi-almacén completo: transferencias, stock por bodega en POS.
3. CxC/CxP: aging, estados de cuenta, conciliación de pagos a facturas.
4. Imprenta digital (Providencia SNAT/2024/000102): integración API.
5. Anulación con nota (botón en factura usando el módulo NC ya listo).
6. Retenciones IVA/ISLR auto-sugeridas al registrar compras con RIF.
