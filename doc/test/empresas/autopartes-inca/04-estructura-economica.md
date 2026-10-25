# Distribuidora Inca — Estructura económica

**Período**: 2026-09 a 2027-01 (5 meses)
**Estado**: **estructura diseñada**, pendiente de datos reales.

> Este documento diseña las estructuras. **No hay datos cargados todavía.** Los montos son
> plantillas, no resultados medidos.

---

## 1. Plan de cuentas

Se usa el plan existente (**1804 cuentas**). Las que esta empresa usa de forma específica:

| Cód. | Cuenta | Para qué |
|---|---|---|
| `101` | Caja | venta al detalle |
| `1041` | Cuentas corrientes operativas | banco, cobranzas al mayor |
| `115` | Mercancía en tránsito | repuestos en camino |
| `116` | Mercancía en almacén | repuestos en bodega |
| `201` | Proveedores de mercancía | importador / proveedor |
| `206` | Sueldos y salarios por pagar | nómina |
| `216` | Seguridad social por pagar | cargas patronales |
| `401` | Ventas de mercancía | ingreso |
| `501` | Costo de mercancía vendida | **costo del módulo** |
| `511` | Gastos de operación | alquiler, local |
| `514` | **Gastos de importación** | **flete, aduana, seguro** |
| `131` | Clientes | cartera |

### `514` es la cuenta que sostiene el módulo aquí

Es la que absorbe los gastos de `import_cost` **antes** de repartirlos al producto. Sin ella:

- La aduana no tiene dónde asentarse en el libro mayor
- El costo de importación no se puede rastrear hasta contabilidad
- Un costo que no se asienta no se puede demostrar

**`inventoryCode`, `costs_of_good_solds` y `purchaseCode`** se leen de
`acc_predefine_account`, no del código.

---

## 2. Clientes

| Tipo | Cantidad | Característica |
|---|---|---|
| Taller / flota (mayorista) | 6-8 | precio `mayor`, crédito 30 días |
| Cliente particular (detalle) | 10-12 | precio `detalle`, contado |
| **Total** | **16-20** | — |

**La distinción es el corazón del negocio**: el mismo filtro a dos precios. Va en
`customer_information.customer_type`.

**Pendiente** (P-03 en [01-minutas.md](01-minutas.md)): verificar qué valores admite esa
columna antes de decidir los códigos.

---

## 3. Proveedores

| Tipo | Cantidad | Ciclo |
|---|---|---|
| Importador / distribuidor exterior | 2-3 | 2-3 partidas al mes |
| Proveedor local (accesorios) | 1-2 | mensual |
| Agente aduanero | 1 | por partida |
| **Total** | **4-6** | — |

**El agente aduanero es crítico**: de su factura salen los cuatro componentes de `import_cost`
(flete, aduana, seguro, otros). Sin esa factura, `customs` sale en cero y el costo queda
incompleto.

---

## 4. Nómina

`hr_payroll_run` + `hr_payroll_item` — **existe**.

| Rol | Personal | Sueldo base (Bs) |
|---|---|---|
| Gerente / dueño | 1 | 60.000 |
| Vendedor de piso | 2 | 32.000 c/u |
| Auxiliar de bodega | 1 | 22.000 |
| Cajero | 1 | 24.000 |
| **Total** | **5** | **170.000/mes** |

Con cargas (~34%) → **≈ 227.800/mes**.

### Nómina y costo: NO va al costo de producto

Igual que la carnicería y a diferencia de la textilera: **no hay fabricación**, así que la
nómina es gasto de operación (`511`), no costo de repuesto.

| Empresa | ¿La nómina va al costo de producto? |
|---|---|
| Textilera | **Sí** (confección) |
| Carnicería | No (despiece, no fabrica) |
| Inca | No (solo revende) |
| Softwara | **Sí** (horas de proyecto) |

Esa tabla es la que demuestra que el módulo distingue los dos casos sin que nadie lo configure
por empresa.

---

## 5. Plan de contrataciones y despidos

**5 meses**: **1 contratación, ningún despido**.

| Mes | Evento | Rol | Coste |
|---|---|---|---|
| 2026-11 | Contratación | 3er vendedor de piso | +32.000 Bs/mes |

**Sin despidos.** Si lo hubiera, la liquidación es gasto del período, no costo de producto —
igual que en Softwara.

**Pendiente**: mecánica de liquidación sin resolver en el ERP (P-03).

---

## 6. Plan de compras

Es una empresa **de compra**, así que el plan de compras es su actividad principal.

| Mes | Partida | Unid. | Costo unit. (USD) | Gastos aduana (USD) |
|---|---|---|---|---|
| 2026-09 | filtros | 200 | 8,00 | 350 |
| 2026-10 | pastillas | 300 | 5,50 | 280 |
| 2026-11 | filtros | 200 | 8,00 | 350 |
| 2026-12 | amortiguadores | 150 | 22,00 | 520 |
| 2027-01 | filtros | 200 | 8,20 | 355 |
| **Total** | **5 partidas** | **1.050** | — | **1.855** |

**Nota crítica**: en el mes 3 (noviembre) se registra la **llegada parcial** (pedido 200,
recibido 170) que es el caso que prueba el reparto de aduana.

**Estos números son proyección.** Los reales salen de las facturas de importación.

---

## 7. Inventario

| Tabla | Qué guarda | Estado |
|---|---|---|
| `product_lot` | repuestos con `unit_cost` | **existe** |
| `import_cost` | gastos por partida | **creada, sin datos** |
| `product_price_list` | precio mayor y detalle | **creada, sin precios** |
| `product_cost_config` | método `IMPORT` por producto | **creada, sin configurar** |
| `cost_snapshot` | costo congelado | **creada, sin cerrar meses** |

### Tablas que quedan VACÍAS a propósito

| Tabla | Por qué |
|---|---|
| `product_recipe` | no fabrica |
| `product_yield` | no hay despiece |
| `cost_timesheet` | no factura horas |
| `labor_grade` | no tiene costos de mano de obra por producto |

Que estén vacías **es el resultado esperado**, no una deuda pendiente. Si en algún momento el
módulo exige una, ese es el fallo que esta empresa está puesta a detectar.

---

## 8. Resumen

| Estructura | Estado |
|---|---|
| Plan de cuentas (1804) | **existe** |
| `acc_predefine_account` | **existe** |
| `product_lot` | **existe** |
| `hr_payroll_run` / `hr_payroll_item` | **existe** |
| `import_cost` | **creada, sin datos** |
| `product_price_list` | **creada, sin precios** |
| `product_cost_config` | **creada, sin configurar** |
| Unidades realmente recibidas | **faltan — bloqueo** |
| Facturas de aduana reales | **faltan — bloqueo** |
| Distinción mayorista | **pendiente** (`customer_type`) |

**El punto crítico** es saber cuánto llegó de verdad. Sin eso, cualquier reparto de aduana es
una suposición — y la diferencia entre lo pedido y lo recibido es exactamente lo que se pierde
del costo si se usa el número equivocado.
