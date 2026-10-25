# Softwara — Benchmark

**Qué se mide**: que el margen de un proyecto sea **facturado − costo real de horas**, incluyendo
las horas que no se facturan.

**Por qué es esta la comprobación**: en un negocio sin inventario, el margen es el único
número que dice si el negocio funciona. Y es el más fácil de inflar, porque basta con contar
solo las horas que se cobran.

---

## Caso 1 — El margen real, con las horas que no se cobran

**Proyecto**: sistema a medida

| Concepto | Horas | Tarifa | Costo |
|---|---|---|---|
| Desarrollo (facturable) | 120 | 30,00 | 3.600,00 |
| Reuniones y soporte (NO facturable) | 40 | 30,00 | 1.200,00 |
| | | **total** | **4.800,00** |

Facturado: **7.500,00**

| Métrica | Esperado |
|---|---|
| Costo real | **4.800,00** |
| Margen real | **2.700,00** (36,0%) |
| Margen solo con facturables | 3.900,00 (52,0%) ← **el error** |

**Criterio de fallo**: si sale **3.900,00**, las horas no facturables no entraron en la cuenta
y el margen está inflado en un 44%.

Es un fallo silencioso: 52% es un margen plausible y nadie lo cuestionaría.

---

## Caso 2 — Dos roles, dos tarifas

| Rol | Horas | Tarifa | Costo |
|---|---|---|---|
| maestro | 60 | 38,6538 | 2.319,23 |
| ayudante | 60 | 23,0000 | 1.380,00 |
| | | **total** | **3.699,23** |

La tarifa del maestro sale de `calcularTasaHoraria`:

```
(60.000 × 1,34) / 2080 = 38,6538 por hora
```

**Criterio de fallo**: si los 120 horas se cobran todas al mismo precio, el rol no está
entrando en la cuenta y un proyecto con más personal junior sale con el mismo costo que uno
con especialistas.

Tiene test: `TestLasHorasSeDividenEntreLasUnidades` y `TestLaTasaHorariaIncluyeCargas`.

---

## Caso 3 — La tarifa no cambia al subir el salario

| Momento | Tarifa del maestro | Costo del proyecto de marzo |
|---|---|---|
| Marzo (proyecto cerrado) | 38,6538 | 3.699,23 |
| Abril (sube el salario) | 42,0000 | **3.699,23** — sin cambio |

**Criterio de fallo**: el costo de marzo cambia después de abril.

Si cambia, el margen de un proyecto ya cobrado deja de ser el que se calculó, y la empresa
pierde la capacidad de revisar lo que cobró. Es el bug 0.4, corregido.

---

## Caso 4 — Sin horas, se dice

**Estado actual**: `cost_timesheet` vacía.

| Métrica | Esperado |
|---|---|
| Costo | **0** |
| Aviso | **presente** ("no hay horas registradas...") |

**Criterio de fallo**: costo en 0 **sin** aviso.

Un costo de proyecto en cero que no avisa es un dato falso presentado como correcto. El motor
no puede inventar las horas, pero tiene que decir que faltan.

Tiene test: `TestSinHorasNoHayCostoLaboralYSeAvisa`.

---

## Caso 5 — Un negocio sin inventario usa pocas tablas

| Tabla | Estado esperado |
|---|---|
| `cost_timesheet` | se usa |
| `labor_grade` | se usa |
| `product_cost_config` | se usa |
| `product_recipe` | **vacía** |
| `product_yield` | **vacía** |
| `import_cost` | **vacía** |

**Criterio de fallo**: el módulo exige receta o rendimiento para costear un proyecto.

Es la prueba inversa a la de Inca: Inca comprueba que no se obliga a llenar lo que no corresponde
en un negocio con inventario; esta, en uno sin inventario.

---

## El número que hay que alcanzar

**Margen por proyecto > 0 en todos los proyectos, contando las horas no facturables.**

Si sale negativo, la empresa está trabajando más de lo que factura. El módulo no lo impide,
pero **lo tiene que hacer visible** — y ese es el entregable de esta empresa.

---

## Limitación de este benchmark

Los números de arriba son una **proyección**, no una medición: no hay horas reales registradas
todavía. Mientras `cost_timesheet` esté vacía, el margen real no se puede calcular, y cualquier
cifra que salga es un escenario, no un resultado.

Está dicho aquí para que no se lea como un resultado.
