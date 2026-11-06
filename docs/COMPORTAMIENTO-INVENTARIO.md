# Comportamiento: Almacén, Inventario y Facturación

## Qué es cada cosa y dónde vive (desde 2026-11)

El catálogo declara el **tipo de ítem** y el sistema lo registra donde
corresponde. No hay una sola ruta "compra → inventario":

| Tipo de ítem | `item_type` | Mueve stock | Cuenta al comprar | Almacén |
|---|---|---|---|---|
| Producto de venta | `venta` | Sí | Inventario de mercancía (1020401) | Almacén de venta |
| Consumo interno | `consumo` | Sí | Inventario de suministros (1020403) | Almacén de consumo |
| Servicio / gasto | `servicio` | **No** | Cuenta de gasto del renglón (4021401…) | — |

- **Una compra tiene varios destinos.** Cada renglón elige tipo, almacén y —si es
  servicio— la cuenta de gasto. El documento sigue siendo uno solo para el Libro
  de Compras y las retenciones.
- **El IVA de las compras ya no engorda el inventario**: se asienta en la cuenta
  de IVA crédito fiscal (1020901).
- **El stock se lee de `stock_movement`**, con su almacén. Totales por empresa y
  por sucursal son dos lecturas del mismo libro; los servicios no tienen libro.
- **El consumo interno** (papelería, limpieza) sale del almacén de consumo al
  costo promedio y se lleva a gasto (`/consumption`). No pasa por costo de ventas.
- **Multi-almacén**: los almacenes se administran en `/warehouse` y se clasifican
  en venta o consumo. Los renglones de un tipo no pueden entrar al almacén del
  otro.

El resto de este documento describe los modos de descuento, que siguen vigentes.

## Flujo en ERPGo (B2B/Empresarial)

### Modo `invoice` (Default — comportamiento actual)

```
Compra → Factura → Salida Stock
   ↓         ↓          ↓
  +Q        -Q         0
```

- **Compra**: `product_purchase_details.quantity` → entrada
- **Factura**: `invoice_details.quantity` → salida (stock se descuenta aquí)
- **Kardex**: lee `stock_movement` (entradas y salidas con su almacén); excluye servicios
- **Validación**: stock físico disponible en el momento de facturar
- **Cuándo usar**: venta inmediata, cliente paga y recibe mercancía al instante

### Modo `delivery` (Nuevo — B2B distribuidor)

```
Compra → Factura → Delivery Order → Salida Stock
   ↓         ↓            ↓              ↓
  +Q        Reservado    Confirmado     -Q (al shipped/delivered)
```

- **Compra**: entrada en `stock_movement` al almacén elegido
- **Factura**: NO descuenta stock (solo reserva vía delivery order)
- **Delivery Order**:
  - `draft` → borrador, no afecta nada
  - `confirmed` → stock comprometido (reservado, no disponible)
  - `picking` → operario recoge del anaquel
  - `packed` → empacado para despacho
  - `shipped` → **salida de stock** (se descuenta)
  - `delivered` → confirmación de entrega al cliente
- **Validación al facturar**: stock físico - stock comprometido
- **Cuándo usar**: venta a crédito, despacho físico separado, logística propia

### Modo `shipped` (E-commerce/Delivery)

Similar a `delivery`, pero el descuento ocurre al `shipped` (sale del almacén). El `delivered` es confirmación de recepción por el cliente.

---

## Flujo en POSGo (Retail)

```
Compra → POS Checkout → Factura → Salida Stock
   ↓          ↓            ↓          ↓
  +Q        Paga         Factura     -Q (inmediato)
```

- **Compra**: entrada al inventario del POS
- **Checkout**: cliente paga en caja (efectivo, tarjeta, etc.)
- **Factura**: se emite el documento fiscal (impresora fiscal)
- **Salida**: **inmediata** — el cliente toma la mercancía en el momento
- **Turno de caja**: el operador abre turno, registra montos iniciales, hace arqueo
- **Cierre X**: parcial, validado por supervisor
- **Cierre Z**: final del día, cierre fiscal con impresora

---

## Comparación de Modos

| Aspecto | ERPGo invoice | ERPGo delivery | POSGo |
|---------|---------------|----------------|-------|
| **Descuento stock** | Al facturar | Al despachar | Al facturar |
| **Separación físico/documento** | No | Sí (delivery order) | No |
| **Flujo** | Simple | Complejo (4 pasos) | Simple |
| **Caja/turno** | No requiere | No requiere | Sí, obligatorio |
| **Supervisor** | No | No | Sí, en cierres |
| **Validación stock** | Físico | Físico - comprometido | Físico |

---

## Almacenamiento de Movimientos

### `stock_movement` (tabla única)

| Campo | Tipo | Uso |
|-------|------|-----|
| `movement_type` | ENUM | purchase, sale_return, adjustment_add, sale, delivery |
| `reference_type` | ENUM | invoice, delivery_order, adjustment |
| `reference_id` | INT | ID del documento origen |
| `reference_no` | VARCHAR | Número legible (INV-001, DO-005) |
| `is_posted` | TINYINT | 1 = confirmado, no modificable |

### Migración del Historial

Cuando se habilita el modo `delivery`/`shipped` en una empresa que ya facturaba, se migran las facturas existentes:

```sql
INSERT INTO stock_movement (movement_type, product_id, quantity, reference_type, reference_id, ...)
SELECT 'sale', id.product_id, id.quantity, 'invoice', i.id, ...
FROM invoice_details id JOIN invoice i ON ...
WHERE i.is_draft = 'no';
```

---

## Interfaz de Usuario

### Menú "Almacén" (nuevo)
- **Nueva Orden**: formulario para crear delivery order
- **Gestionar Órdenes**: lista con DataTables + acciones (editar, cambiar estado, eliminar)

### Lista de Órdenes
- Filtros: estado, cliente, almacén, rango de fechas
- Columnas: ID, Cliente, Almacén, Estado (badge color), Fecha, Notas
- Acciones:
  - Ver/Editar
  - Cambiar Estado (solo transiciones válidas)
  - Eliminar (solo draft)
  - Imprimir

### Formulario de Orden
- Cliente (obligatorio)
- Almacén (default WH-MAIN)
- Fecha (default hoy)
- Notas
- Líneas de producto (producto + cantidad)

### Cambios de Estado
- Botón desplegable con el siguiente estado válido
- Solo permite transiciones hacia adelante
- `cancelled` disponible desde cualquier estado excepto `delivered`
- Al confirmar `shipped` o `delivered`, genera `stock_movement` automático

---

## Permisos

| Permiso | Descripción |
|---------|-------------|
| `delivery_order` | Ver y crear órdenes de entrega |
| `warehouse` | Gestionar almacenes y ver reportes de almacén |

---

## Endpoints API

| Método | Ruta | Acción |
|--------|------|--------|
| GET | `/delivery_order/list` | Lista de órdenes |
| GET | `/delivery_order/data` | Data para DataTables |
| GET | `/delivery_order/add` | Formulario nueva orden |
| GET | `/delivery_order/edit/{id}` | Formulario editar |
| POST | `/delivery_order/save` | Guardar nueva/editar |
| POST | `/delivery_order/delete/{id}` | Eliminar (solo draft) |
| POST | `/delivery_order/update-status/{id}` | Cambiar estado |

---

## Configuración

En `fiscal_setting`:
- `inventory_deduction_mode` = `invoice` | `delivery` | `shipped`

---

*Fecha: 2026-10-20*
*Estado: Implementado y validado*
