# Softwara — Estructura económica

**Período**: 2026-09 a 2027-01 (5 meses)
**Estado**: **estructura diseñada**, pendiente de datos reales.

> Este documento diseña las estructuras. **No hay datos cargados todavía.** Los montos son
> plantillas, no resultados medidos.

---

## 1. Plan de cuentas

Se usa el plan existente (**1804 cuentas**). Esta empresa usa **muchas menos cuentas** que las
otras tres, porque no tiene inventario:

| Cód. | Cuenta | Para qué |
|---|---|---|
| `101` | Caja | cobros en efectivo |
| `1041` | Cuentas corrientes operativas | banco, transferencias |
| `131` | Clientes | cartera por cobrar de proyectos |
| `141` | Anticipos de clientes | proyecto cobrado por adelantado |
| `206` | Sueldos y salarios por pagar | nómina |
| `216` | Seguridad social por pagar | cargas patronales |
| `403` | Ingresos por servicios | **facturación de proyectos** |
| `511` | Gastos de operación | alquiler, licencias, herramientas |
| `512` | Gastos de personal | gasto que NO es costo de proyecto |

### Las dos cuentas que separan ingreso de costo

- `403` (ingresos por servicios) ← lo que se factura
- `512` (gastos de personal) ← lo que se paga y **no** se cobra

Lo que se factura contra lo que se cobra son las dos mitades del margen. Sin separarlas de la
nómina normal, el margen no se puede calcular.

**No hay cuentas de inventario** (`114`, `116`, `117`, `118`) porque **no hay inventario**. Ese
es el rasgo que define esta empresa.

---

## 2. Clientes

| Tipo | Cantidad | Ciclo |
|---|---|---|
| Cliente recurrente (mantenimiento) | 3-4 | mensual |
| Cliente por proyecto | 4-5 | según hito |
| **Total** | **7-9** | — |

**No usan `price_list`.** Un proyecto se cotiza por alcance, no por lista de precios. Si la
pantalla obliga a elegir lista para facturar un proyecto, es un diseño que no encaja con este
negocio.

La cotización va por `quotation` (ya existe) y la factura por `service_invoice` (ya existe).

---

## 3. Proveedores

**El más pequeño de los cuatro.**

| Tipo | Cantidad | Ciclo |
|---|---|---|
| Alojamiento / dominios | 1-2 | mensual |
| Licencias de software | 2-3 | anual |
| Hardware (puntual) | 0-1 | según necesidad |
| **Total** | **3-5** | — |

**No hay proveedor de materia prima** porque no la hay. El "proveedor" real de esta empresa es
su propio equipo, y eso entra por nómina, no por compras.

---

## 4. Nómina

`hr_payroll_run` + `hr_payroll_item` — **existe**.

| Rol | Personal | Sueldo base (Bs) | `labor_grade` |
|---|---|---|---|
| Desarrollador senior | 2 | 70.000 | `maestro` |
| Desarrollador junior | 2 | 40.000 | `ayudante` |
| Diseñador UI/UX | 1 | 50.000 | `maestro` |
| Project manager | 1 | 60.000 | `supervisor` |
| **Total** | **6** | **330.000/mes** |

Con cargas (~34%) → **≈ 442.200/mes**.

### La separación que define esta empresa

| Concepto | ¿Va a `cost_timesheet`? | ¿Va a nómina? |
|---|---|---|
| Horas de proyecto (facturables) | **Sí** | Sí |
| Reuniones y soporte (no facturables) | **Sí**, con `billable=0` | Sí |
| Vacaciones y licencias | **No** | Sí |

Las vacaciones **no** van a `cost_timesheet`: no se trabajaron contra ningún proyecto. Pero
sí son costo del negocio, y entran en `512`.

Mezclar vacaciones con horas de proyecto infla el costo de los proyectos y hace que parezcan
menos rentables de lo que son.

---

## 5. Plan de contrataciones y despidos

**5 meses**: **1 contratación, 1 despido**.

| Mes | Evento | Rol | Coste |
|---|---|---|---|
| 2026-10 | Contratación | 3er desarrollador junior | +40.000 Bs/mes |
| 2026-12 | **Despido** | desarrollador junior | liquidación |

### Por qué el despido importa aquí

Es la empresa donde el despido puede contaminar el costo de proyecto **sin que se note**:

- Si la liquidación se carga a un proyecto concreto, ese proyecto sale con un coste que no es
  suyo
- Si se reparte entre todos los proyectos, los cerrados cambian de margen

**Lo correcto**: la liquidación es gasto del período (`512`), no costo de proyecto. Y los
proyectos cerrados **no cambian** — es el mismo criterio que la congelación de `cost_snapshot`.

**Pendiente**: la mecánica de liquidación no está resuelta en el ERP (P-03 en
[01-minutas.md](01-minutas.md)).

---

## 6. Plan de compras de materia prima

**No hay.** Es la diferencia estructural con las otras tres.

Lo que sí hay que "comprar" no es materia prima sino **capacidad de trabajo**:

| Mes | Personal disponible | Horas facturables aprox. |
|---|---|---|
| 2026-09 | 5 | 600 |
| 2026-10 | 6 | 720 |
| 2026-11 | 6 | 720 |
| 2026-12 | 6 (−1 despido) | 600 |
| 2027-01 | 5 | 600 |
| **Total** | — | **3.240 h** |

**Esto es una proyección, no un dato medido.** No existe ninguna hoja de horas todavía —
`hr_payroll_item` no tiene columna de horas (verificado en la base real).

---

## 7. Inventario

**No hay inventario.** No hay `product_lot`, no hay bodega, no hay FEFO, no hay merma.

Lo que ocupa ese lugar en una empresa de servicios:

| Tabla | Qué guarda | Estado |
|---|---|---|
| `cost_timesheet` | horas trabajadas por proyecto | **creada, vacía** |
| `labor_grade` | tarifa por rol | **creada, sin sueldos** |
| `cost_component` | desglose con la tarifa usada | **creada, vacía** |
| `cost_snapshot` | costo congelado por mes | **creada, sin cerrar** |

Esas cuatro tablas hacen el trabajo que en las otras tres hacen las tablas de producto. Si el
módulo exige alguna de las de producto, es un fallo de diseño — y es justo lo que esta empresa
está puesta a detectar.

---

## 8. Resumen

| Estructura | Estado |
|---|---|
| Plan de cuentas (1804) | **existe** |
| `acc_predefine_account` | **existe** |
| `hr_payroll_run` / `hr_payroll_item` | **existe** |
| `quotation`, `service_invoice` | **existen** |
| `cost_timesheet` | **creada, vacía — es todo el módulo** |
| `labor_grade` con sueldos | **creada, sueldos sin cargar** |
| Horas reales por proyecto | **faltan — bloqueo absoluto** |
| Tarifas de rol con cargas reales | **faltan** |

**Sin horas, esta empresa no tiene costo de proyecto.** Es el único de los cuatro casos donde
el dato no viene de un movimiento de negocio (compra, venta) sino de un hábito de la gente —
y por eso el más frágil.
