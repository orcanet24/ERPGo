# Softwara — Minutas de decisiones

**Propósito**: registrar por qué se tomó cada decisión.

---

## M-01 · La hoja de horas va en tabla propia, no en la nómina

**Decisión**: `cost_timesheet` es una tabla nueva, separada de `hr_payroll_item`.

**Motivo**: son cosas distintas y esta es la decisión que más consecuencias tiene.

| | `hr_payroll_item` | `cost_timesheet` |
|---|---|---|
| Registra | lo que se **paga** | lo que se **trabaja** |
| Se mueve con | liquidaciones, ajustes retroactivos | no |
| Si un ajuste retroactivo cambia... | la nómina | **no** el costo ya facturado |

Si estuvieran juntas, un ajuste de nómina retroactivo reescribiría el costo de proyectos
**ya cerrados y ya cobrados**. El cliente ya pagó con un margen que dejaría de ser el que se
le calculó.

**Se descartó**: añadir `hours` a `hr_payroll_item` porque "ya tiene employee_id y amount".
Existe, pero su semántica es de pago, y mezclarlas es el error de modelo que más caro sale.

---

## M-02 · `billable` es columna y no detalle de interfaz

**Decisión**: `cost_timesheet.billable` distingue horas facturables de no facturables.

**Motivo**: las no facturables son **costo real** —alguien las trabajó y se le pagó— pero no
generan ingreso. Si se meten en el mismo saco, el margen del proyecto sale inflado.

```
Proyecto:  120 h facturables  → ingreso
Reuniones:  40 h no facturables → costo SIN ingreso
```

**Se descartó**: calcular el margen solo con las horas que sí se facturaron. Eso da un margen
más alto que la realidad y es el error que hace que una consultora crea que gana más de lo que
gana.

---

## M-03 · La tarifa se guarda al calcular, no se recalcula

**Decisión**: `labor_grade.hourly_rate` es una **columna normal**; la tarifa que se usó se
guarda en `cost_component.rate_used`.

**Motivo**: el borrador original proponía `GENERATED ALWAYS AS`. Con eso, subir el salario en
abril **cambia la tasa de marzo** y re-escribe el costo de proyectos ya facturados, sin que
nadie los toque.

Con la tasa guardada: lo que se cobró en marzo se calculó con la tasa de marzo. Se puede
revisar dentro de dos años y sigue dando el mismo número.

Era el bug 0.4 del plan.

---

## M-04 · Las horas agrupan por `grade`, no solo por proyecto

**Decisión**: la hoja de horas registra el **rol** con el que se trabajó.

**Motivo**: un maestro y un ayudante no valen lo mismo. Sin `grade`, las 160 horas de un
proyecto se cobran todas igual y un proyecto con personal junior sale con el mismo costo que
uno con especialistas — que es exactamente al revés de la realidad.

---

## M-05 · Se empieza sin horas, y se dice

**Decisión**: la simulación arranca con `cost_timesheet` **vacía** y el motor emitiendo aviso.

**Motivo**: no existen horas reales. Inventarlas daría un costo de proyecto que parece medido
y es una suposición.

Preferible un costo con "no hay horas registradas para este producto" a un número que nadie
puede defender delante del cliente.

---

## M-06 · `hours_per_year` no se deja en 2080 sin pensarlo

**Decisión**: el default es 2080 (40 h × 52), pero queda marcado como dato que hay que
ajustar.

**Motivo**: una empresa con vacaciones, licencias y bajas **no trabaja 2080 horas**. Con 2080,
la tarifa sale **baja** y el costo de proyecto sale **bajo**: el margen sale inflado otra vez.

Si se deja en el default, hay que decir que el margen que salga es con el default.

---

## Pendiente de decidir

| # | Decisión pendiente | Qué hace falta |
|---|---|---|
| P-01 | Si las cargas patronales entran en la tarifa o aparte | Criterio del contador |
| P-02 | Si el costo de un proyecto se congela al cobrar o al cerrar el mes | Cuándo se considera terminado |
| P-03 | Qué pasa si un proyecto cruza dos meses | Repartir horas o cerrar por proyecto |
| P-04 | Si las horas no facturables se prorratean a todos los proyectos | Política de la empresa |
