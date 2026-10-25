# Diseño: Flujo de Almacén, Inventario y Facturación — ERPGo

**Fecha:** 2026-10-20  
**Estado:** Diseño propuesto para revisión e implementación  
**Autor:** Orcanet24 + Hermes Agent

---

## 1. Objetivo

Sistema metódico de inventario y facturación que:
- Parametrice por producto si es físico o servicio
- Múltiples formas de trabajo (pedidos, cotizaciones, facturas, notas de salida)
- Histórico inalterable de movimientos en `stock_movement`
- Precios flexibles: default + listas + descuentos configurables por la empresa
- Clasificación de clientes que determina lista de precio
- Todo configurable por el gerente, nada hardcodeado

---

## 2. Parametrización por Producto

### 2.1 Tabla `product_inventory_config`

| Campo | Tipo | Default | Descripción |
|-------|------|---------|-------------|
| `product_id` | VARCHAR(30) | — | ID del producto |
| `product_type` | ENUM('physical','service') | 'physical' | Físico pasa por almacén, servicio no |
| `requires_stock_movement` | TINYINT | 1 | 1 = mueve stock, 0 = no |

### 2.2 Reglas según tipo

| Tipo | ¿Valida stock? | ¿Mueve stock? | ¿Kardex? | ¿Contabilidad? |
|------|----------------|---------------|----------|----------------|
| `physical` | ✅ | ✅ | ✅ | ✅ (venta) |
| `service` | ❌ | ❌ | ❌ | ✅ (ingreso) |

> **Nota legal:** El kardex (Libro de Inventarios) solo refleja entradas/salidas de bienes. Los servicios no pasan por allí, pero sí generan ingreso contable y factura.

---

## 3. Precios y Listas

### 3.1 Tabla `price_list` (existente, ampliada)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | INT | PK |
| `code` | VARCHAR(20) | 'detalle', 'mayor', 'gran_mayor' |
| `name` | VARCHAR(100) | Nombre descriptivo |
| `sort_order` | INT | Orden en listas |
| `is_default` | TINYINT | 1 = lista por defecto |
| `currency_code` | VARCHAR(5) | 'VES', 'USD', etc. |
| `status` | ENUM('active','inactive') | Activa/inactiva |

**Listas base (configurables por la empresa):**

| Código | Nombre | Uso típico |
|--------|--------|------------|
| `detalle` | Venta al detal | Minorista (default) |
| `mayor` | Venta al mayor | 10+ unidades |
| `gran_mayor` | Distribuidor | Mayorista |

> La empresa puede crear más listas (promocional, empleados, etc.) sin tocar código.

### 3.2 Tabla `product_price_list` (existente)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `product_id` | INT | Producto |
| `price_list_id` | INT | Lista de precio |
| `price` | DECIMAL(16,4) | Precio en esa lista |
| `margin_pct` | DECIMAL(5,2) | Margen esperado (referencia) |
| `updated_at` | DATETIME | Última actualización |

### 3.3 Precio Default Obligatorio

En `product_information`:
- `price` → Precio default (obligatorio, usado si no hay lista asignada)
- `price_usd` → Precio en USD (opcional, para multi-moneda)
- `currency_code` → Moneda base ('VES', 'USD')

### 3.4 Tabla `product_price_discount` (nueva) — Descuentos configurables

| Campo | Tipo | Default | Descripción |
|-------|------|---------|-------------|
| `id` | INT | — | PK |
| `product_id` | INT | — | Producto |
| `customer_id` | INT | NULL | Cliente específico (NULL = aplica a todos) |
| `customer_class` | VARCHAR(20) | NULL | Clase de cliente (minorista, mayorista, distribuidor) |
| `price_list_id` | INT | NULL | Lista específica (NULL = todas) |
| `discount_pct` | DECIMAL(5,2) | 0 | Porcentaje de descuento |
| `min_qty` | DECIMAL(18,2) | 0 | Cantidad mínima para aplicar |
| `valid_from` | DATE | NULL | Vigencia inicio |
| `valid_until` | DATE | NULL | Vigencia fin |
| `reason` | VARCHAR(255) | '' | Motivo (negociación, promoción, etc.) |
| `status` | ENUM('active','inactive') | 'active' | Activo/inactivo |
| `created_at` | DATETIME | — | Fecha creación |

---

## 4. Maestro de Clientes (Ampliación)

### 4.1 Clasificación de Clientes

En `customer_information` o tabla nueva `customer_class`:

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `customer_id` | INT | PK |
| `customer_class` | ENUM('minorista','mayorista','distribuidor','vip','empleado') | Clasificación |
| `default_price_list_id` | INT | Lista de precio por defecto |
| `credit_limit` | DECIMAL(18,2) | Límite de crédito |
| `credit_days` | INT | Días de crédito |

### 4.2 Reglas de Clasificación

| Clasificación | Lista por defecto | Uso |
|---------------|-------------------|-----|
| `minorista` | `detalle` | Compra < 10 unidades |
| `mayorista` | `mayor` | Compra 10-100 unidades |
| `distribuidor` | `gran_mayor` | Compra > 100 unidades |
| `vip` | Cualquier | Cliente especial con descuentos propios |
| `empleado` | `detalle` | Precio especial interno |

---

## 5. Cálculo de Precio Final

### 5.1 Función Centralizada: `precioFinal(r, productID, customerID, priceListID)`

```go
func (s *Server) precioFinal(r *http.Request, productID int, customerID int, priceListID int) (precio float64, descuento float64, err error) {
    // 1. Obtener precio de la lista (o default si no hay lista)
    precio, err = s.precioDeLista(r, productID, priceListID)
    if err != nil {
        return 0, 0, err
    }
    
    // 2. Buscar descuento activo (más específico primero)
    descuento = s.descuentoActivo(r, productID, customerID, priceListID)
    
    // 3. Aplicar descuento
    precio = precio * (1 - descuento/100)
    
    return precio, descuento, nil
}
```

### 5.2 Prioridad de Búsqueda de Descuento

1. **Más específico:** producto + cliente + lista + vigente
2. **Por clase:** producto + customer_class + lista + vigente
3. **Por producto+lista:** producto + lista + cualquier cliente + vigente
4. **Sin descuento:** precio de lista tal cual

### 5.3 Tabla `invoice_details` (ampliar)

| Campo Nuevo | Tipo | Descripción |
|-------------|------|-------------|
| `price_list_id` | INT | Lista de precio usada |
| `discount_pct` | DECIMAL(5,2) | Descuento aplicado |
| `discount_amount` | DECIMAL(18,2) | Monto del descuento |
| `base_price` | DECIMAL(18,2) | Precio antes de descuento |

---

## 5.4 Tabla `invoice_details` (ampliar)

| Campo Nuevo | Tipo | Descripción |
|-------------|------|-------------|
| `price_list_id` | INT | Lista de precio usada |
| `discount_pct` | DECIMAL(5,2) | Descuento aplicado |
| `discount_amount` | DECIMAL(18,2) | Monto del descuento |
| `base_price` | DECIMAL(18,2) | Precio antes de descuento |

---

## 6. Formas de Trabajo

### 6.1 Pedidos (Orders)

**Propósito**: Comprometer inventario sin descuento contable ni físico.

| Estado | ¿Reserva stock? | ¿Kardex? | ¿Contabilidad? |
|--------|-----------------|----------|----------------|
| `draft` | ❌ | ❌ | ❌ |
| `confirmed` | ✅ (reserva) | ❌ | ❌ |
| `cancelled` | ❌ (libera) | ❌ | ❌ |

**Flujo**:
1. Cliente solicita productos → Se crea `order` en `draft`
2. Vendedor confirma → `confirmed` → Genera `stock_reservation` con `status='pending'`
3. Si el cliente cancela → `cancelled` → Libera reserva
4. **No afecta stock_movement** hasta que se convierte en factura o delivery order

### 6.2 Cotizaciones (Quotations) — `quotation` (existente)

**Propósito**: Propuesta comercial sin compromiso contable.

| Estado | ¿Reserva stock? | ¿Kardex? | ¿Contabilidad? |
|--------|-----------------|----------|----------------|
| `draft` | ❌ | ❌ | ❌ |
| `sent` | Configurable | ❌ | ❌ |
| `accepted` | ❌ → puede crear order/factura | ❌ | ❌ |
| `rejected` | ❌ | ❌ | ❌ |

**Configuración**: Si `enable_reservation = 1` en `inventory_flow_config`, las cotizaciones en `sent` reservan stock.

### 6.3 Facturas (Invoices)

**Propósito**: Documento fiscal que reconoce ingreso y genera cobro.

#### Modo `invoice` (default, compatible)

```
Factura confirmada:
  ├── Si producto físico → descuenta stock (stock_movement con movement_type='sale')
  ├── Si servicio → no mueve stock
  └── Asiento contable: Venta + Cuenta por Cobrar
```

#### Modo `delivery` (B2B distribuidor)

```
Factura confirmada:
  ├── Si producto físico → NO descuenta stock (solo compromete si hay order vinculado)
  ├── Si servicio → no mueve stock
  └── Asiento contable: Venta + Cuenta por Cobrar
  └── Luego: Delivery Order formaliza la salida física
```

#### Modo `shipped` (e-commerce/delivery)

Similar a `delivery`, pero el descuento ocurre al `shipped` (sale del almacén), no al `delivered`.

### 6.4 Delivery Orders (Notas de Salida) — `delivery_order`

**Propósito**: Documento formal de salida de mercancía del almacén.

| Estado | ¿Afecta stock? | Descripción |
|--------|----------------|-------------|
| `draft` | ❌ | Borrador |
| `confirmed` | ❌ (reserva) | Stock comprometido |
| `picking` | ❌ | En preparación |
| `packed` | ❌ | Listo para despachar |
| `shipped` | ✅ | **Descuenta stock** (stock_movement) |
| `delivered` | ✅ (ya descontado) | Confirmación de entrega |
| `cancelled` | ❌ (libera) | Cancelado |

**Generación de stock_movement**:
- Al transicionar a `shipped` o `delivered`: `movement_type='delivery'`, `reference_type='delivery_order'`

### 6.5 Devoluciones — `return_customer`

**Propósito**: Reversar una salida previa (cliente devuelve mercancía).

```
Devolución:
  ├── Lee el stock_movement original (venta o delivery)
  ├── Crea stock_movement con movement_type='return_customer', quantity POSITIVA (entrada)
  ├── reference_id = ID del movimiento original
  └── Anula el efecto del movimiento previo
```

### 6. Importaciones — `import`

**Propósito**: Entrada de mercancía sin compra formal (importación directa).

```
Importación:
  ├── stock_movement con movement_type='import', quantity POSITIVA
  ├── No requiere purchase order
  └── Se usa para mercancía que entra sin factura de proveedor
```

---

## 7. Tablas de Soporte (Resumen)

| Tabla | Propósito |
|-------|-----------|
| `product_inventory_config` | Tipo de producto (physical/service) |
| `inventory_flow_config` | Configuración por empresa (modo descuento, reservas) |
| `stock_reservation` | Reservas activas (pedidos, cotizaciones) |
| `stock_movement` | Movimientos unificados de inventario |
| `delivery_order` | Ordenes de salida del almacén |
| `delivery_order_detail` | Líneas de la orden |
| `price_list` | Listas de precio (detalle, mayor, etc.) |
| `product_price_list` | Precio de producto por lista |
| `product_price_discount` | Descuentos configurables |
| `customer_class` | Clasificación de clientes |

---

## 8. Cálculo de Stock Disponible

| Campo | Descripción |
|-------|-------------|
| `deduction_mode` | `invoice` / `delivery` / `shipped` |
| `enable_reservation` | 1 = pedidos/cotizaciones reservan stock |

### `stock_reservation` (reservas activas)

| Campo | Descripción |
|-------|-------------|
| `document_type` | `order` / `quotation` / `invoice` |
| `document_id` | ID del documento |
| `product_id` | Producto reservado |
| `quantity` | Cantidad reservada |
| `status` | `pending` / `committed` / `cancelled` |

---

## Cálculo de Stock Disponible

```
stock_disponible = 
  entradas (compras + importaciones + devoluciones_cliente)
  - salidas (ventas + delivery + devoluciones_proveedor)
  - reservas (stock_reservation activas si enable_reservation=1)
```

### Función centralizada: `stockForProduct(r, productID)`

```go
func (s *Server) stockForProduct(r *http.Request, productID int) float64 {
    // 1. Obtener configuración del producto
    pType := s.productType(r, productID) // physical o service
    
    // 2. Si es servicio, retorna 0 (no aplica)
    if pType == "service" {
        return 0
    }
    
    // 3. Obtener configuración de flujo de la empresa
    mode := s.inventoryDeductionMode(r)
    
    // 4. Calcular según modo
    switch mode {
    case "delivery", "shipped":
        return s.stockFromMovement(r, productID)
    default: // invoice
        return s.stockForProductWithoutMovement(r, productID)
    }
}
```

---

## Validación al Facturar

### Producto Físico

| Modo | Validación |
|------|-----------|
| `invoice` | `stock_disponible >= cantidad` |
| `delivery` | `stock_disponible - reservas >= cantidad` (puede bloquear o permitir con aviso) |
| `shipped` | `stock_disponible >= cantidad` (igual que invoice, pero el descuento real es después) |

### Producto Servicio

- No valida stock
- No genera `stock_movement`
- Solo genera asiento contable

---

## Movimientos Inmutables en `stock_movement`

Una vez generado un `stock_movement` con `is_posted=1`:
- **No se puede modificar** (regla de negocio, no trigger DB)
- Para corregir: crear un movimiento contrario (devolución, ajuste)
- El histórico queda como cadena de auditoría

### Ejemplo de cadena:

| ID | movement_type | reference_type | reference_id | quantity | is_posted |
|----|---------------|----------------|--------------|----------|-----------|
| 1 | purchase | purchase | 100 | +100 | 1 |
| 2 | sale | invoice | 200 | -30 | 1 |
| 3 | delivery | delivery_order | 50 | -20 | 1 |
| 4 | return_customer | return | 2 (ref a #2) | +5 | 1 |

Resultado: 100 - 30 - 20 + 5 = 55 unidades

---

## Integración POSGo → ERPGo

### Flujo POSGo (Retail)

```
POS Checkout:
  ├── Cliente paga → Factura fiscal (impresora)
  ├── Descuento inmediato de stock local (SQLite POSGo)
  └── Registro en cash_session (turno de caja)

Transmisión a ERPGo (sync_worker gRPC):
  ├── Ventas facturadas → invoice + invoice_details en ERPGo
  ├── Movimientos de caja → cash_session en ERPGo (auditoría)
  └── Stock descuento → stock_movement en ERPGo (movement_type='sale')
```

### Dónde se refleja en ERPGo

| Concepto POSGo | Tabla ERPGo |
|----------------|-------------|
| Factura | `invoice` + `invoice_details` |
| Descuento stock | `stock_movement` (movement_type='sale') |
| Turno/caja | `cash_session` (existente) |
| Cierre X/Z | `cash_session` + reportes de auditoría |

---

## 11. Permisos

| Permiso | Descripción |
|---------|-------------|
| `price_list` | Gestionar listas de precio |
| `product_price` | Gestionar precios de productos |
| `customer_class` | Clasificar clientes |
| `order` | Gestionar pedidos |
| `delivery_order` | Gestionar notas de salida |
| `warehouse` | Reportes de almacén |
| `stock_reservation` | Gestionar reservas |

---

## 12. Menú

### 12.1 "Almacén"
- Pedidos
- Cotizaciones
- Notas de Salida
- Devoluciones

### 12.2 "Ventas"
- Facturas (existente)
- Precios y Listas (nuevo)
- Descuentos (nuevo)
- Clasificación de Clientes (nuevo)

---

## 12. Endpoints Nuevos

### 12.1 Precios y Listas

| Método | Ruta | Acción |
|--------|------|--------|
| GET | `/price_list/list` | Listas de precio |
| POST | `/price_list/save` | Crear/editar lista |
| GET | `/price_list/activate/{id}` | Activar lista |
| GET | `/product_price/list` | Precios por producto |
| POST | `/product_price/save` | Guardar precios |
| GET | `/product_price/discounts` | Descuentos activos |
| POST | `/product_price/discount/save` | Crear descuento |
| POST | `/product_price/discount/delete/{id}` | Eliminar descuento |

### 12.2 Clientes

| Método | Ruta | Acción |
|--------|------|--------|
| GET | `/customer/class/list` | Clasificaciones |
| POST | `/customer/class/save` | Asignar clasificación |
| GET | `/customer/price_list/{id}` | Lista asignada al cliente |

### 12.3 Almacén
| GET | `/order/data` | Data para DataTables |
| GET | `/order/add` | Formulario nuevo pedido |
| GET | `/order/edit/{id}` | Editar pedido |
| POST | `/order/save` | Guardar pedido |
| POST | `/order/confirm/{id}` | Confirmar (genera reserva) |
| POST | `/order/cancel/{id}` | Cancelar (libera reserva) |
| POST | `/order/to-invoice/{id}` | Convertir pedido en factura |
| POST | `/order/to-delivery/{id}` | Convertir pedido en nota de salida |
| GET | `/delivery_order/list` | Lista de notas de salida |
| GET | `/delivery_order/add` | Nueva nota |
| POST | `/delivery_order/save` | Guardar |
| POST | `/delivery_order/update-status/{id}` | Cambiar estado |
| GET | `/inventory/config` | Parametrización de productos |
| GET | `/inventory/config/data` | Data de configuración |
| POST | `/inventory/config/save` | Guardar tipo de producto |

---

## Documentos Legales Venezolanos Relacionados

| Documento | Aplicación |
|-----------|------------|
| **RIVA** (Reglamento de la Ley de IVA) | Facturas, notas de crédito, retenciones |
| **RISLR** (Reglamento ISLR) | Retenciones sobre servicios |
| **Ley de Impuesto al Consumo** | Compras/ventas en zonas específicas |
| **PCGE** (Plan Contable General Empresarial) | Asientos contables de inventario |
| **Normas de Inventarios (Venezuela)** | Kardex, entradas/salidas, valuación |

---

*Fecha: 2026-10-20*
*Estado: Diseño propuesto para revisión e implementación*
