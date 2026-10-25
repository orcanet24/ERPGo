# Softwara — Plan inicial

**Fecha**: 2026-10-04
**Tipo**: Desarrollo de software: apps, sistemas a medida y páginas web para clientes
**Método dominante**: `HOURS` (costo por horas de equipo)
**Inventario**: **no tiene** — y eso es lo que lo hace valiosa como prueba

## Qué hace

Vende proyectos a clientes:

- Sistemas a medida (ERP, portales)
- Apps para clientes
- Páginas web

El costo es **equipo × horas × tarifa**. No hay materia prima, no hay bodega, no hay
merma, no hay aduana, no hay rendimiento.

## Por qué es la empresa más importante de las cuatro

Las otras tres son negocios con inventario, que es lo que el ERP ya sabe hacer. Esta no tiene
inventario. Si el módulo de costos solo sirve para productos que se compran y se revenden,
**esta empresa lo pone de manifiesto de inmediato**.

Y además es la que empuja el diseño hacia lo que falta de verdad: sin inventario, todo el
costo está en las horas, y **las horas no existen en el ERP**.

## El bloqueo principal: `hr_payroll_item` no tiene horas

Verificado sobre la base real:

```
hr_payroll_item: id, run_id, employee_id, concept_code, kind, amount, rate_pct, base_amount
```

**Ninguna columna de horas.** Se puede pagar a alguien y no saber cuántas horas trabajó. Para
una empresa que factura proyectos, eso significa que **no hay costo por proyecto que calcular**:
no hay de dónde sacarlo.

Por eso `cost_timesheet` es el paso 0 de todo el módulo, antes que las tablas de producto.

`cost_timesheet` no se metió en `hr_payroll_item` a propósito:

| | `hr_payroll_item` | `cost_timesheet` |
|---|---|---|
| Registra | lo que se **paga** | lo que se **trabaja** |
| Se mueve con | liquidaciones, ajustes retroactivos | no |
| Si un ajuste cambia... | la nómina | **no** el costo ya facturado |

Si estuvieran en la misma tabla, un ajuste de nómina retroactivo reescribiría el costo de
proyectos ya cerrados y cobrados.

## Tablas que usa

| Tabla | Para qué | Necesaria |
|---|---|---|
| `cost_timesheet` | horas por proyecto y rol | **Sí** — es todo |
| `labor_grade` | tarifa por rol (maestro, ayudante, supervisor) | **Sí** |
| `product_cost_config` | método `HOURS` | **Sí** |
| `cost_component` | la tarifa que se USÓ, congelada | **Sí** |
| `product_recipe` | — | **No** |
| `product_yield` | — | **No** |
| `import_cost` | — | **No** |
| `price_list` | — | **No** |

Cuatro tablas, no trece. Esa diferencia es la prueba de que el módulo no obliga a llenar lo que
un negocio no necesita.

## Los tres puntos críticos

### 1. Horas facturables y no facturables no son lo mismo

```
Proyecto:  120 h facturables  × tarifa  = ingreso
Reuniones:  40 h no facturables × tarifa = costo, pero NO ingreso
```

Las no facturables son **costo real**: alguien las trabajó y se le pagó. Si se meten en el
mismo saco que las facturables, el margen del proyecto sale inflado y nadie ve el error hasta
que la empresa trabaja más de lo que factura.

Por eso `cost_timesheet.billable` es una columna y no un detalle de la interfaz.

### 2. La tarifa se congela al calcular

`labor_grade.hourly_rate` es una **columna normal**, no una columna generada. Si fuera
generada, subir el salario en abril cambiaría la tasa de marzo y **re-escribiría el costo de
proyectos ya facturados**, sin que nadie los tocara.

La tarifa se calcula en el momento, se usa, y queda guardada en `cost_component.rate_used`.
Lo que se cobró en marzo se calculó con la tasa de marzo.

### 3. Un rol, una tarifa

Un maestro y un ayudante no valen lo mismo. Sin `grade` en la hoja de horas, las 160 horas
del proyecto se cobran todas igual, y el proyecto con más personal junior sale con el mismo
costo que uno con especialistas.

## Datos que faltan (y que hay que conseguir antes de simular)

- **La tarifa de cada rol**: no es el salario dividido por 2080. Hay que añadir cargas
  patronales y beneficios. En Venezuela las cargas son un número conocido, pero hay que
  decidir si se incluyen o no, y esa decisión queda escrita en
  [01-minutas.md](01-minutas.md).
- **Las horas reales de los proyectos**: es el dato que no existe y que hay que empezar a
  registrar. Sin él, la simulación es una proyección, no una medición.
- **Cuántas horas al año trabaja cada quien**: `hours_per_year` = 2080 es el default (40 h ×
  52), pero una empresa con vacaciones y licencias trabaja menos. Si se deja en 2080, la
  tarifa sale baja y el costo de proyecto sale bajo.

## Benchmark

Detalle en [02-benchmark.md](02-benchmark.md). En resumen: **facturado − costo de horas =
margen**, y el margen debe ser positivo en todos los proyectos. Si sale negativo, o las horas
se registraron mal o la tarifa está mal calculada.

## Riesgo principal

**Que las horas no se registren.** Todo el módulo cuelga de un dato que depende de que la
gente lo escriba. Una empresa de software que no anota horas no tiene costo por proyecto, y el
módulo le dirá exactamente eso — pero no puede inventarlas.

Es el único de los cuatro casos donde el dato no viene de un movimiento de negocio (una compra,
una venta) sino de un hábito de la gente. Eso lo hace el más frágil, y el que más cuidado
requiere en la puesta en marcha.
