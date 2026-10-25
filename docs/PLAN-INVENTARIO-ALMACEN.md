# Plan: Flujo de Almacén, Inventario y Facturación — ERPGo

## Objetivo

Separar la salida física del almacén de la factura. La factura es un documento comercial/fiscal; la salida real ocurre en un punto configurable por empresa (factura, despacho o entrega).

---

## 1. Tablas a crear

### 1.1 `stock_movement` — Movimiento unificado de inventario

```sql
CREATE TABLE IF NOT EXISTS stock_movement (
  id INT NOT NULL AUTO_INCREMENT,
  tenant_id VARCHAR(30) NOT NULL DEFAULT 'default',
  movement_type ENUM('purchase','sale_return','adjustment_add','sale','purchase_return','adjustment_remove','delivery') NOT NULL,
  product_id INT NOT NULL,
  warehouse_id VARCHAR(30) DEFAULT 'WH-MAIN',
  quantity DECIMAL(18,2) NOT NULL,
  unit_cost DECIMAL(18,4) DEFAULT NULL,
  reference_type ENUM('invoice','purchase','delivery_order','adjustment','return') NOT NULL,
  reference_id INT NOT NULL,
  reference_no VARCHAR(50) DEFAULT NULL,
  date DATE NOT NULL,
  is_posted TINYINT NOT NULL DEFAULT 1,
  posted_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_sm_tenant_product_date (tenant_id, product_id, date),
  KEY idx_sm_reference (reference_type, reference_id),
  KEY idx_sm_type (movement_type)
);
```

### 1.2 `delivery_order` — Orden de salida del almacén

```sql
CREATE TABLE IF NOT EXISTS delivery_order (
  id INT NOT NULL AUTO_INCREMENT,
  tenant_id VARCHAR(30) NOT NULL DEFAULT 'default',
  invoice_id INT DEFAULT NULL COMMENT 'Factura asociada (puede ser NULL si entrega sin factura)',
  customer_id INT NOT NULL,
  warehouse_id VARCHAR(30) DEFAULT 'WH-MAIN',
  status ENUM('draft','confirmed','picking','packed','shipped','delivered','cancelled') DEFAULT 'draft',
  date DATE NOT NULL,
  delivered_at DATETIME DEFAULT NULL,
  notes TEXT,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_do_tenant_status (tenant_id, status),
  KEY idx_do_tenant_invoice (tenant_id, invoice_id)
);
```

### 1.3 `delivery_order_detail` — Líneas de la orden

```sql
CREATE TABLE IF NOT EXISTS delivery_order_detail (
  id INT NOT NULL AUTO_INCREMENT,
  tenant_id VARCHAR(30) NOT NULL DEFAULT 'default',
  delivery_order_id INT NOT NULL,
  product_id INT NOT NULL,
  quantity DECIMAL(18,2) NOT NULL,
  picked_quantity DECIMAL(18,2) DEFAULT 0,
  packed_quantity DECIMAL(18,2) DEFAULT 0,
  shipped_quantity DECIMAL(18,2) DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uq_dod (delivery_order_id, product_id)
);
```

---

## 2. Modos de descuento (configurables por empresa)

| Modo | Cuándo descuenta stock | Caso de uso |
|------|----------------------|-------------|
| `invoice` | Al facturar (`invoice_details`) | Retail/POS: cliente toma mercancía y paga en caja simultáneamente |
| `delivery` | DeliveryOrder estado `delivered` | B2B distribuidor: venta a crédito 30 días, salida física separada |
| `shipped` | DeliveryOrder estado `shipped` | E-commerce/delivery: sale del almacén, entrega al cliente después |

### Configuración

```sql
INSERT INTO fiscal_setting (key, value) VALUES
  ('inventory_deduction_mode', 'invoice');  -- Default: compatible con comportamiento actual
```

---

## 3. Flujo de estados Delivery Order

```
draft → confirmed → picking → packed → shipped → delivered
  ↓          ↓          ↓         ↓         ↓          ↓
(sin stock) (reservado) (en preparación) (salida real) (confirmado)
```

- `draft`: borrador, no afecta stock
- `confirmed`: reserva stock (no descuenta, marca como comprometido)
- `picking`: operador recoge mercancía del anaquel
- `packed`: mercancía empacada, lista para despachar
- `shipped`: **mercancía sale físicamente del almacén** → descuento stock real
- `delivered`: cliente recibe (cobranza, auditoría, tracking)

---

## 4. Función centralizada `stockForProduct()`

Toda lectura de stock pasa por una función única:

```go
func (s *Server) stockForProduct(tenantID string, productID int) float64 {
    mode := s.inventoryDeductionMode(tenantID)
    switch mode {
    case "delivery", "shipped":
        return s.stockFromMovement(tenantID, productID)
    default: // "invoice"
        return s.stockFromInvoiceDetails(tenantID, productID)
    }
}
```

### Razón

Actualmente hay **17 puntos** en 10 archivos que calculan stock como `SUM(invoice_details.quantity)`. Todos deben usar la función centralizada para que el modo de descuento funcione correctamente.

### Archivos afectados (17 puntos)

| Archivo | Líneas | Uso |
|---------|--------|-----|
| `kardex.go` | 162-180 | Salida "Venta" en kardex |
| `product.go` | 67-68 | Stock en listado de productos |
| `product.go` | ~290 | Stock en formulario de producto |
| `product.go` | ~324 | Stock check AJAX |
| `product.go` | ~381 | Stock al guardar producto |
| `invoice.go` | ~747 | Stock en factura |
| `invoice.go` | ~1014 | Stock en lista de facturas |
| `ai.go` | 639 | Stock para reportes IA |
| `dashboard.go` | 95 | Stock en dashboard |
| `report.go` | 126, 159 | Stock en reportes |
| `legal_books.go` | 285, 324 | Stock en libros legales |
| `adjustment.go` | 98 | Stock en ajustes de inventario |
| `reorder_point.go` | 221, 292 | Consumo para punto de pedido |

---

## 5. Migración del historial

Para facturas existentes que no tienen `stock_movement`:

```sql
INSERT INTO stock_movement (tenant_id, movement_type, product_id, warehouse_id, quantity,
  unit_cost, reference_type, reference_id, reference_no, date, posted_at, created_at)
SELECT 'default', 'sale', id.product_id, COALESCE(i.warehouse_id, 'WH-MAIN'), id.quantity,
  id.rate, 'invoice', i.id, i.invoice, i.date, NOW(), NOW()
FROM invoice_details id
JOIN invoice i ON i.invoice_id = id.invoice_id
WHERE i.is_draft = 'no' AND IFNULL(i.is_anulled,0) = 0;
```

---

## 6. Validación al facturar según modo

| Modo | Validación de stock al facturar |
|------|-------------------------------|
| `invoice` | Stock físico disponible (comportamiento actual) |
| `delivery` | Stock físico - stock comprometido (delivery orders en `confirmed`/`picking`). Puede bloquear o permitir con aviso de "pendiente de entrega". |
| `shipped` | Stock físico disponible. Si no hay delivery order, puede bloquear o permitir con aviso de "pendiente de despacho". |

---

## 7. Impacto en kardex

El kardex actual lee `invoice_details` como salida. Con el cambio, lee `stock_movement`:

- `movement_type = 'sale'` → salida por factura (modo invoice)
- `movement_type = 'delivery'` → salida por delivery order (modos delivery/shipped)
- `movement_type = 'purchase'` → entrada por compra
- `movement_type = 'adjustment_add'` → entrada por ajuste
- `movement_type = 'adjustment_remove'` → salida por ajuste

El kardex mantiene su naturaleza contable (CPP) pero refleja el movimiento físico real según el modo configurado.

---

## 8. Referencias de sistemas maduros

| Sistema | Flujo |
|---------|-------|
| **SAP** | Pedido → Entrega (Picking/Packing) → PGI (Post Goods Issue, descuenta stock) → Factura |
| **Odoo** | Pedido → Picking → Envío (descuenta stock) → Factura sobre cantidades entregadas |
| **ERPNext** | Material Issue → Sales Invoice → Delivery Note |

---

## 9. Fases de implementación

| Fase | Qué | Validación |
|------|-----|-----------|
| **1** | Crear tablas + migración del historial | `go vet` OK, servidor arranca, migración idempotente |
| **2** | Función `stockForProduct()` + modo `invoice` (default) | Stock actual sigue funcionando igual |
| **3** | Cambiar los 17 puntos a `stockForProduct()` | Listados, kardex, dashboard funcionan |
| **4** | Soporte modo `delivery` y `shipped` | Crear delivery order, validar facturación según modo |
| **5** | Wizard configura modo por empresa | Al instalar/dar empresa, elegir modo de descuento |

---

## 10. Lo que NO se toca en este plan

- No cambia la arquitectura de tenants (single-BD vs multi-BD)
- No modifica POSGo
- No cambia el sistema de caja/turnos
- No modifica el wizard de instalación
- No toca la comunicación gRPC POS↔ERP

---

*Fecha: 2026-10-20*
*Estado: Plan conversado, pendiente implementación*
