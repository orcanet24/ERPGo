# Textilera El Telar — Estructura económica

**Período**: 2026-09 a 2027-01 (5 meses)
**Estado**: **estructura diseñada**, pendiente de datos reales.

> Este documento diseña las estructuras. **No hay datos cargados todavía.** Los montos y
> nombres son plantillas de lo que se va a crear, no resultados medidos.

---

## 1. Plan de cuentas

Se usa el plan existente (**1804 cuentas** en `acc_coa`). Las que esta empresa usa de forma
específica:

| Cód. | Cuenta | Para qué |
|---|---|---|
| `101` | Caja | ventas de mostrador |
| `1041` | Cuentas corrientes operativas | banco, cobranzas |
| `114` | Materiales y suministros | tela, botones, cierres |
| `116` | Mercancía en almacén | prendas terminadas y lo importado |
| `117` | Materia prima en proceso | prendas a medio coser |
| `118` | Productos terminados | lo que ya salió de confección |
| `201` | Proveedores de mercancía | tela y proveedor importado |
| `206` | Sueldos y salarios por pagar | nómina de confección |
| `216` | Seguridad social por pagar | cargas patronales |
| `401` | Ventas de mercancía | ingreso |
| `501` | Costo de mercancía vendida | **costo calculado por el módulo** |
| `511` | Gastos de operación | alquiler, luz |
| `514` | Gastos de importación | **flete, aduana, seguro** ← clave |
| `131` | Clientes | cartera |

### La cuenta `514` es la clave de esta empresa

Es la que absorbe `import_cost` antes de repartirla al producto. Sin ella, la aduana quedaría
en ningún sitio y el costo de importación no se podría rastrear hasta el libro mayor.

**`inventoryCode` y `costs_of_good_solds`** se leen de `acc_predefine_account`, no del código.

---

## 2. Clientes

| Tipo | Cantidad | Característica |
|---|---|---|
| Tienda / detalle | 10-12 | precio `detalle` |
| Mayorista (tiendas, uniformes corporativos) | 5-6 | precio `mayor`, crédito 30 días |
| **Total** | **15-18** | — |

La distinción va en `customer_information.customer_type`. **Pendiente**: verificar los valores
admitidos (P-03 en [01-minutas.md](01-minutas.md)).

---

## 3. Proveedores

| Tipo | Cantidad | Ciclo |
|---|---|---|
| Tela / textil (nacional) | 2-3 | compra quincenal |
| Insumos (botones, cierres, hilos) | 2 | compra mensual |
| Agente aduanero / importador | 1 | por partida |
| **Total** | **5-6** | — |

**El agente aduanero es el proveedor crítico**: de ahí sale la factura con flete, aduana,
seguro y otros, que es lo que se carga en `import_cost`.

---

## 4. Nómina

`hr_payroll_run` + `hr_payroll_item` — **existe**.

| Rol | Personal | Sueldo base (Bs) |
|---|---|---|
| Diseñador / patronista | 1 | 55.000 |
| Cortador maestro | 1 | 40.000 |
| Costurera maestra | 2 | 35.000 c/u |
| Costurera ayudante | 3 | 24.000 c/u |
| Auxiliar de bodega | 1 | 20.000 |
| Administrativo | 1 | 30.000 |
| **Total** | **9** | **272.000/mes** |

Con cargas (~34%) → **≈ 364.480/mes**.

### Nómina y costo: aquí SÍ aplica `cost_timesheet`

A diferencia de la carnicería, en la textilera **la mano de obra de confección es costo de
producto**: las horas de la costurera entran en el costo de la prenda.

Pero hay que separar:

| Empleado | ¿Va al costo de producto? |
|---|---|
| Costurera, cortador | **Sí** → `cost_timesheet` con `grade` |
| Administrativo, bodega | **No** → gasto de operación (`511`) |

Mezclar los dos infla el costo de la prenda con gastos que no son suyos.

**Estado**: `cost_timesheet` está creada pero **vacía**. Se llena desde el mes 4 del ciclo.

---

## 5. Plan de contrataciones y despidos

**5 meses**: **2 contrataciones, 1 despido**.

| Mes | Evento | Rol | Coste |
|---|---|---|---|
| 2026-10 | Contratación | 2ª costurera | +24.000 Bs/mes |
| 2026-11 | Contratación | 1er ayudante de corte | +20.000 Bs/mes |
| 2027-01 | **Despido** | costurera con menor antigüedad | liquidación |

### El despido sí se diseña aquí

Es el único de los cuatro con despido en el período, y sirve para probar que el costo del
producto **no** se contamina por una liquidación:

- La liquidación es **gasto del período** (cuenta `511` o `216`), no costo de producto
- El costo de las prendas ya fabricadas **no cambia** con el despido
- Si cambia, algo se está mezclando

**Pendiente**: la mecánica de liquidación (preaviso, cesantías, utilidades) no está resuelta en
el ERP. Es una decisión contable que requiere criterio legal (P-03).

---

## 6. Plan de compras de materia prima

### Lo que se fabrica

| Mes | Tela | Botones | Cierres | Prendas |
|---|---|---|---|---|
| 2026-09 | 400 m | 700 u | 120 u | 200 |
| 2026-10 | 450 m | 800 u | 135 u | 225 |
| 2026-11 | 500 m | 900 u | 150 u | 250 |
| 2026-12 | 650 m | 1.150 u | 195 u | 325 |
| 2027-01 | 500 m | 900 u | 150 u | 250 |
| **Total** | **2.500 m** | **4.450 u** | **750 u** | **1.250** |

### Lo que se importa

| Mes | Partida | Unidades | Gastos aduana (Bs) |
|---|---|---|---|
| 2026-09 | calzado | 100 pares | 400 |
| 2026-11 | textil importado | 150 u | 600 |
| 2027-01 | calzado | 120 pares | 480 |
| **Total** | **3 partidas** | **370** | **1.480** |

**Proyección, no dato medido.** Los montos reales salen de la factura del agente aduanero.

---

## 7. Inventario

| Tabla | Qué guarda | Estado |
|---|---|---|
| `product_lot` | tela y prendas con `unit_cost` | **existe** |
| `product_recipe` | componentes de cada prenda + merma | **creada, sin datos** |
| `import_cost` | gastos por partida | **creada, sin datos** |
| `cost_snapshot` | costo congelado | **creada, sin cerrar meses** |

### Flujo de inventario de fabricación

```
tela (114) → en proceso (117) → terminado (118) → vendido (501)
```

Cada flecha es un movimiento con su asiento. La tela que no se convierte en prenda es
**merma**, y esa merma está en `product_recipe.loss_factor`.

---

## 8. Resumen

| Estructura | Estado |
|---|---|
| Plan de cuentas (1804) | **existe** |
| `acc_predefine_account` | **existe** |
| `product_lot` | **existe** |
| `hr_payroll_run` / `hr_payroll_item` | **existe** |
| `product_recipe`, `import_cost` | **tablas creadas, sin datos** |
| `cost_timesheet` | **creada, vacía hasta el mes 4** |
| Recetas con merma medida | **faltan — bloqueo** |
| Facturas de aduana reales | **faltan — bloqueo** |
| Horas de confección | **faltan** |
