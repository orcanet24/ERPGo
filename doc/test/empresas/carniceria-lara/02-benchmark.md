# Carnicería Lara — Benchmark

**Qué se mide**: que el costo por kilo de venta salga **por encima** del costo de entrada por
kilo, exactamente en la proporción del rendimiento.

**Por qué es esta la comprobación**: es la única que detecta el error de rendimiento sin mirar
el código. Si los dos números salen iguales, el rendimiento no está entrando en la cuenta y la
empresa está vendiendo hueso y merma sin cobrarlos — y no hay error, no hay aviso, no hay
excepción. El costo simplemente sale mal.

---

## Caso 1 — El rendimiento entra en la cuenta

**Entrada**: canal de 250 kg por 1.000
**Rendimiento**: 72% → 180 kg de venta

| Métrica | Esperado |
|---|---|
| Costo de entrada por kg | 1.000 / 250 = **4,0000** |
| Costo de venta por kg | 1.000 / 180 = **5,5556** |
| Diferencia | **+1,5556** por kg |

**Criterio de fallo**: si el costo de venta sale **4,0000**, el rendimiento no se aplicó.

Es un fallo silencioso: 4,00 es un número plausible y nadie lo cuestionaría mirándolo.

---

## Caso 2 — La aduana no aplica (compra local)

**Entrada**: canal de 180 kg por 900, comprada localmente

| Métrica | Esperado |
|---|---|
| Material | **5,0000** por kg (900/180) |
| Customs | **0** |
| Final | **5,0000** |

**Criterio de fallo**: `customs > 0` en una compra local. Sería un gasto de importación
asignado a una carne que no se importó.

---

## Caso 3 — Dos listas de precio

Mismo corte, dos precios:

| Lista | Precio | Margen sobre costo 5,5556 |
|---|---|---|
| `detalle` | 7,50 | +34,9% |
| `mayor` | 6,20 | +11,6% |

**Criterio de fallo**: los dos precios iguales. Significa que `product_price_list` no se está
leyendo o que la lista `mayor` no se creó.

**Criterio de negocio**: el precio al mayor **no puede quedar por debajo del costo**. Si sale
menor que 5,5556, la venta a granel está perdiendo dinero, y es el error más caro de los dos.

---

## Caso 4 — Cierre de período: el costo no cambia

| Momento | Costo del corte | Esperado |
|---|---|---|
| Calculado en el mes | 5,5556 | — |
| Después de cerrar | 5,5556 | **sin cambio** |
| Recalcado en el mes siguiente | 5,5556 | **sin cambio** |

**Criterio de fallo**: el costo de un mes cerrado cambia al recalcular.

Si cambia, el precio de venta de los meses siguientes se calculó sobre un número que ya no
existe, y el margen histórico deja de ser confiable.

---

## Caso 5 — Cierre idempotente

Cerrar el mismo período dos veces debe dar **el mismo número de filas**, no el doble.

**Criterio de fallo**: `cost_snapshot` con dos filas para el mismo producto y período.

Es el mismo error que ya apareció en los informes de IA (un total que se sumaba dos veces):
mismo origen, mismo síntoma, mismo arreglo.

---

## Cómo se comprueban

Los casos 1, 2 y 5 son **aritmética pura** y ya tienen test en
`interfaces/web/cost_engine_test.go`:

```
TestElRendimientoSubeElCostoPorUnidad
TestLaAduanaNoSeReparteSiNoSeRecibioNada
```

El caso 3 y el 4 necesitan base de datos, porque tocan `product_price_list` y
`cost_snapshot`. Son los que faltan.

---

## El número que hay que alcanzar

**Margen por kilo > 0 en todos los cortes, en ambas listas.**

Un margen negativo no es un fallo técnico: es la empresa vendiendo por debajo de lo que le
costó entrar la carne. El módulo no lo impide —no es su trabajo—, pero **lo tiene que hacer
visible**, y ese es el entregable de esta empresa.
