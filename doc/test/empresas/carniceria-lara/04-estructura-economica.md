# Carnicería Lara — Estructura económica

**Período**: 2026-09 a 2027-01 (5 meses)
**Estado**: **estructura diseñada**, pendiente de datos reales.

> Este documento diseña las estructuras. **No hay datos cargados todavía.** Los montos,
> nombres y cantidades son plantillas de lo que se va a crear, no resultados medidos.

---

## 1. Plan de cuentas

El ERP ya trae **1804 cuentas** (`acc_coa`, jerárquicas por `HeadCode` / `PHeadCode` /
`HeadLevel`). No se crea un plan nuevo: se usa el existente.

Las cuentas que esta empresa **usa de forma específica**:

| Cód. | Cuenta | Para qué |
|---|---|---|
| `101` | Caja | ventas de mostrador al detalle |
| `1041` | Cuentas corrientes operativas | banco, cobranzas al mayor |
| `115` | Mercancía en tránsito | canales en camino |
| `116` | Mercancía en almacén | canales y cortes en bodega |
| `201` | Proveedores de mercancía | compra de canales |
| `206` | Sueldos y salarios por pagar | nómina |
| `216` | Seguridad social por pagar | cargas patronales |
| `301` | Capital social | aportes del dueño |
| `401` | Ventas de mercancía | ingreso por venta |
| `501` | Costo de mercancía vendida | **el costo que calcula el módulo** |
| `511` | Gastos de operación | alquiler, luz, agua |
| `131` | Clientes | cartera por cobrar |

**La cuenta `501` es la que conecta el módulo de costos con la contabilidad.** Un costo que no
se asienta en el libro mayor es un costo que la empresa no puede demostrar.

### Se usa `acc_predefine_account`

Las cuentas de contabilización se leen de `acc_predefine_account` (`salary_code`, `cashCode`,
`inventoryCode`, `costs_of_good_solds`), **no se escriben en el código**. Un número de cuenta
escrito en el código es un número que algún día no corresponde con el plan de la empresa.

---

## 2. Clientes

Se crean dos tipos, porque la doble lista de precio exige distinguirlos:

| Tipo | Cantidad | Característica |
|---|---|---|
| Mostrador / detalle | 8-10 | pagan en el momento, precio `detalle` |
| Mayoristas (restaurantes, hoteles, food trucks) | 4-5 | crédito 15 días, precio `mayor` |
| **Total** | **12-15** | — |

**La distinción** se hace en `customer_information.customer_type`. **Pendiente**: hay que
verificar qué valores admite esa columna en datos reales antes de decidir los códigos (ver
P-02 en [01-minutas.md](01-minutas.md)).

---

## 3. Proveedores

| Tipo | Cantidad | Ciclo |
|---|---|---|
| Ganadero (canal) | 2-3 | compra semanal |
| Insumos (sal, condimentos, embalaje) | 2 | compra mensual |
| **Total** | **4-5** | — |

**Nota**: el ganadero es el proveedor crítico. De ahí sale el costo de entrada que después se
reparte con el rendimiento.

---

## 4. Nómina

`hr_payroll_run` + `hr_payroll_item` — **existe** y se puede simular.

| Rol | Personal | Sueldo base (Bs) |
|---|---|---|
| Carnicero maestro (despachador) | 1 | 45.000 |
| Ayudante de mostrador | 1 | 25.000 |
| Cajero | 1 | 22.000 |
| Auxiliar de bodega | 1 | 20.000 |
| **Total** | **4** | **112.000/mes** |

**Añadir cargas patronales** (~34%) → coste real ≈ **150.080/mes**.

### Nómina y costo: la decisión importante

**Esta empresa NO usa `cost_timesheet`.** Las horas de los 4 empleados son costo del negocio
(gastos de operación, cuenta `511`), **no costo de producto**. Si se cargaran al costo de la
carne, estarían mezclando el costo de vender con el costo de comprar.

Es la diferencia entre:
- `cost_timesheet` → costo de **fabricar** (no aplica aquí)
- nómina → gasto de **operar** (esto sí aplica)

---

## 5. Plan de contrataciones y despidos

**5 meses**: se proyecta **1 contratación** y **ningún despido**.

| Mes | Evento | Rol | Coste adicional |
|---|---|---|---|
| 2026-11 | Contratación | 2º ayudante de mostrador | +25.000 Bs/mes |

**Sin despidos** en el período. Si hubiera, hay que contemplar:
- Preaviso (según antigüedad)
- Cesantías acumuladas
- Utilidades proporcionales

**Pendiente de decisión** (ver P-03 en [01-minutas.md](01-minutas.md)): si una liquidación de
despido se carga a gastos del mes o se reparte. Hoy no está resuelto en el ERP.

---

## 6. Plan de compras de materia prima

Canales, que es la materia prima **y** el producto a la vez.

| Mes | Canales | Peso aprox. | Coste aprox. (Bs) |
|---|---|---|---|
| 2026-09 | 40 | 250 kg c/u = 10.000 kg | 100.000 |
| 2026-10 | 45 | 11.250 kg | 112.500 |
| 2026-11 | 50 | 12.500 kg | 125.000 |
| 2026-12 | 60 | 15.000 kg | 150.000 (temporada) |
| 2027-01 | 45 | 11.250 kg | 112.500 |
| **Total** | **240** | **60.000 kg** | **600.000** |

**Estos números son una proyección**, no un dato medido. El costo real por kg es **el
dato 1 de "Datos de entrada que faltan"** en el ciclo.

---

## 7. Inventario

| Tabla | Qué se guarda | Estado |
|---|---|---|
| `product_lot` | canal y cortes con `unit_cost` | ya existe, con columna de costo |
| `product_information.cost` | CPP vigente | ya existe (`decimal(18,4)`) |
| `cost_snapshot` | costo congelado al cierre | nueva |

**FEFO aplica**: los cortes son perecederos. `product_lot.expiry_date` ya existe.

### La cuenta que mueve el inventario

`inventoryCode` de `acc_predefine_account` — no se escribe el número en el código.

---

## 8. Resumen de lo que sí está listo vs. lo que falta

| Estructura | Estado |
|---|---|
| Plan de cuentas (1804) | **existe**, no hay que crearlo |
| `acc_predefine_account` | **existe** |
| `product_lot` con costo | **existe** |
| `hr_payroll_run` / `hr_payroll_item` | **existe** |
| `price_list` (detalle/mayor) | **creado en la migración** |
| `product_yield` | **tabla creada, sin datos** |
| `cost_snapshot` | **creado, sin cerrar ningún mes** |
| Datos reales de clientes/proveedores | **faltan** |
| Rendimiento medido | **falta — es el bloqueo** |
| Costo real de la canal | **falta — es el bloqueo** |
