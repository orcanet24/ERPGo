# Softwara — Ciclo económico de 5 meses

**Período**: 2026-09 a 2027-01
**Estado**: **pendiente de datos reales** — este documento es el plan, no el resultado.

> **Qué es y qué no es.** Es el plan de qué se va a montar. Las cifras de horas y tarifas se
> llenan cuando exista una hoja de horas real. Mientras `cost_timesheet` esté vacía, cualquier
> margen que salga es una **proyección**, no un resultado.

---

## Ciclo que se va a montar

Es el único de los cuatro **sin inventario**. No hay compra de materia prima, no hay bodega,
no hay despiece, no hay aduana.

```
1. Se firma un proyecto       → acuerdo con el cliente
2. Se registran horas         → cost_timesheet (facturable y no facturable)
3. Se factura el hito         → service_invoice + CxC
4. Cobranza                   → entra la plata
5. Nómina del mes             → hr_payroll_run (pago, no costo)
6. Cierre de costos           → cost_snapshot congela el costo por proyecto
```

El paso 2 es **todo el módulo** para esta empresa. Sin él, no hay nada que costear.

---

## Mes 1 (2026-09) — Se arranca la hoja de horas

Se empieza a registrar horas contra proyecto, con `grade` y `billable`.

**Estado**: `cost_timesheet` vacía → el costo sale **0 con aviso**.

Está decidido así: preferible un costo con "no hay horas registradas" a un número inventado
(ver [01-minutas.md](01-minutas.md), M-05).

---

## Mes 2 (2026-10) — Primeras horas reales

Se registran horas del proyecto en curso. Se comprueba que:

| Comprobación | Esperado |
|---|---|
| Costo por proyecto | > 0 |
| Aviso de "sin horas" | desaparece |
| Roles separados | `maestro` y `ayudante` con tarifas distintas |

---

## Mes 3 (2026-11) — Se añaden horas no facturables

Se registran reuniones y soporte con `billable = 0`.

**Lo que comprueba**: que el margen **baje** cuando entran las horas no cobradas. Si no baja,
no se están contando — y eso es el error que más infla el margen de una consultora.

```
margen con facturables solo   > margen real    ← el error
margen con todo                = margen real
```

---

## Mes 4 (2026-12) — Sube la tarifa, el mes pasado no cambia

Se sube el salario de un grado. Se comprueba que el costo de los proyectos de los meses
anteriores **no cambia**.

**Lo que comprueba**: que la tasa se guarda al calcular y no se recalcula (bug 0.4, corregido).
Si el costo de octubre cambia en diciembre, el margen de un proyecto ya cobrado deja de ser el
que se le calculó.

---

## Mes 5 (2027-01) — Cierre del ciclo

Se repasa el acumulado:

| Métrica | Esperado |
|---|---|
| Margen > 0 en todos los proyectos | sí |
| Margen contando horas no facturables | sí |
| Costo de un mes cerrado inalterable | sí |
| Diferencia contra contabilidad | **a determinar** |

---

## Datos de entrada que faltan

| # | Dato | De dónde sale | Sin esto |
|---|---|---|---|
| 1 | Horas reales por proyecto | hoja de horas, registro manual | **no hay costo que calcular** |
| 2 | Tarifa de cada rol | `labor_grade` con cargas patronales | la tarifa sale en 0 |
| 3 | Horas no facturables | se clasifican al registrar | el margen sale inflado |
| 4 | Si las cargas entran en la tarifa | criterio del contador | cambia el número |
| 5 | `hours_per_year` real | menos de 2080 si hay vacaciones | la tarifa sale baja |

**El punto 1 es el bloqueo de todo el módulo**: `hr_payroll_item` no tiene columnas de horas
(verificado en la base real). Es el único dato de las cuatro empresas que **no viene de un
movimiento de negocio** sino de un hábito de la gente — y eso lo hace el más frágil.

---

## Qué NO se monta en esta empresa

Para que quede claro que son decisiones y no omisiones:

- **`product_recipe`** — no fabrica. Vacía a propósito.
- **`product_yield`** — no hay despiece.
- **`import_cost`** — no importa.
- **`price_list`** — no vende por listas; factura proyectos.

Cuatro tablas de trece. Si el módulo exige cualquiera de las cuatro que sí usa, es un fallo de
diseño.
