# Plan: Nivel ERP — Localización Venezolana

## Objetivo
Llevar ERPGo al nivel de sistemas como Profit Plus, Gala, Saint o módulos de Odoo Venezuela.

## Análisis de lo que falta vs el documento "Para que tu ERP esté al nivel"

### ✅ Ya implementado
| Área | Estado |
|------|--------|
| Libro de Compras (IVA) | ✅ `/reports/book-purchases` |
| Libro de Ventas (IVA) | ✅ `/reports/book-sales` |
| Libro de Control Inventario (Art. 177 RISLR) | ✅ Parcial (falta Kárdex completo con CPP) |
| TXT Retenciones IVA | ✅ `/reports/vat-retention/txt` |
| Retenciones IVA | ✅ Guardar + reportes |
| Retenciones ISLR | ✅ Guardar + reportes |

### 🆕 Por implementar (Prioridad Alta)

#### 1. XML para Declaración de Retenciones de ISLR
**Descripción:** Archivo XML para declarar retenciones ISLR en el portal SENIAT.

**Estructura XML (basado en normativa SENIAT):**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<RelacionRetencionesISLR AgenteRetencion="J000000001" Periodo="202609">
  <DetalleRetencion>
    <RifRetenido>J000000002</RifRetenido>
    <NumeroFactura>000123</NumeroFactura>
    <NumeroControl>00-000123</NumeroControl>
    <FechaOperacion>2026-09-15</FechaOperacion>
    <CodigoConcepto>001</CodigoConcepto>
    <BaseImponible>1000.00</BaseImponible>
    <PorcentajeRetencion>5.00</PorcentajeRetencion>
    <MontoRetenido>50.00</MontoRetenido>
  </DetalleRetencion>
</RelacionRetencionesISLR>
```

**Tablas necesarias:** Usar `islr_retention` (ya existe) + nueva tabla `islr_concept` para códigos.

**Endpoints:**
- `GET /reports/islr-retention/xml?periodo=YYYYMM` → Genera y descarga XML
- `GET /reports/islr-retention/receipt/{id}` → Comprobante PDF/print

#### 2. IGTF (Impuesto a las Grandes Transacciones Financieras - 3%)
**Descripción:** Impuesto del 3% sobre pagos en divisas o criptoactivos.

**Reglas:**
- Se aplica cuando el pago se hace en USD, EUR, USDT, BTC u otras divisas
- Monto IGTF = Monto en divisas × 3% × Tasa BCV
- Se refleja como renglón independiente en la factura

**Tablas necesarias:**
- Nueva tabla `igtf_transaction` (id, invoice_id/purchase_id, date, currency, amount_usd, rate_bcv, igtf_amount, created_by)
- Nuevo campo en `invoice`: `igtf_amount` decimal(18,2)

**Endpoints:**
- `POST /invoice/igtf/calculate` → Calcula IGTF para una factura
- `GET /reports/igtf` → Reporte consolidado de IGTF
- Integración automática en `invoiceSave` cuando `payment_type` involucra divisas

#### 3. Tasa BCV + Diferencial Cambiario
**Descripción:** Manejo automatizado de tasa oficial BCV y cálculo de diferencial.

**Tablas necesarias:**
- Nueva tabla `exchange_rate` (id, date, rate_bcv decimal(18,4), source ENUM['api','manual'], created_at)
- Nueva tabla `currency_rate` (id, currency_code, rate_bcv, date)

**Endpoints:**
- `GET /settings/exchange-rate` → Configurar tasas manualmente
- `POST /settings/exchange-rate/fetch` → Consultar API BCV
- `GET /reports/exchange-difference` → Reporte de diferencial cambiario

**Lógica de diferencial:**
- Al pagar factura en fecha posterior: compara tasa factura vs tasa pago
- Genera asiento automático: Ganancia/Pérdida por Diferencial Cambiario

### 🟡 Prioridad Media

#### 4. Pista de Auditoría e Inalterabilidad
**Tabla:** `audit_log` (id, user_id, action, module, table_name, record_id, old_value, new_value, ip_address, created_at)

**Reglas:**
- No eliminar registros, solo marcar como anulado
- Triggers en tablas críticas (invoice, purchase, acc_transaction)
- Log de impresiones (ya existe `invoice_print_log`)

### 🟢 Prioridad Baja (Fase 2)

#### 5. Parafiscales y Nómina
- IVSS, FAOV, INCES
- Prestaciones sociales
- Fase 2 posterior

---

## Orden de Implementación

### Fase 1: XML ISLR (Prioridad 🔴)
1. Crear tabla `islr_concept` con códigos oficiales SENIAT
2. Crear handler `islrRetentionTxt` con formato XML
3. Crear template de comprobante de retención (print)
4. Agregar botón de descarga en vista de retenciones

### Fase 2: IGTF (Prioridad 🔴)
1. Crear tabla `igtf_transaction`
2. Agregar campo `igtf_amount` a `invoice`
3. Integrar cálculo en `invoiceSave` cuando aplica
4. Crear reporte de IGTF
5. Actualizar template de factura para mostrar IGTF

### Fase 3: BCV + Diferencial (Prioridad 🔴)
1. Crear tablas `exchange_rate` y `currency_rate`
2. Crear módulo de configuración de tasas
3. Implementar API BCV (web scraping o API alternativa)
4. Crear lógica de cálculo de diferencial
5. Generar asientos contables automáticos

### Fase 4: Auditoría (Prioridad 🟡)
1. Crear tabla `audit_log`
2. Agregar middleware de auditoría a todas las rutas
3. Implementar triggers en BD para tablas críticas
4. Prohibir DELETE en invoice, purchase, acc_transaction

---

## Códigos de Concepto ISLR (Decreto 1808)

Los códigos oficiales para el XML de ISLR incluyen:
- 001: Honorarios profesionales
- 002: Comisiones
- 003: Intereses
- 004: Arrendamiento de bienes muebles
- 005: Arrendamiento de bienes inmuebles
- 006: Publicidad y propaganda
- 007: Servicios técnicos
- 008: Seguros y reaseguros
- ... (según tabla oficial SENIAT)

## Notas Técnicas

- El XML debe declararse con encoding UTF-8
- Los montos deben ir con 2 decimales, sin separador de miles
- El RIF debe normalizarse (letra + 9 dígitos, sin guiones)
- El período debe ser YYYYMM (6 dígitos)
- El archivo debe descargarse con Content-Type application/xml

## Estado Final Esperado

| Módulo | Estado |
|--------|--------|
| Libro de Compras | ✅ Completo |
| Libro de Ventas | ✅ Completo |
| Control Inventario (Art. 177) | ✅ Completo |
| TXT Retenciones IVA | ✅ Completo |
| XML Retenciones ISLR | 🆕 Fase 1 |
| Comprobante ISLR | 🆕 Fase 1 |
| IGTF 3% | 🆕 Fase 2 |
| Reporte IGTF | 🆕 Fase 2 |
| Tasa BCV | 🆕 Fase 3 |
| Diferencial Cambiario | 🆕 Fase 3 |
| Pista Auditoría | 🆕 Fase 4 |
| Nómina Parafiscal | ⏳ Fase 2 posterior |
