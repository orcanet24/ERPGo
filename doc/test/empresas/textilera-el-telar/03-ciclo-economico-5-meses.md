# Textilera El Telar — Ciclo económico de 5 meses

**Período**: 2026-09 a 2027-01
**Estado**: **pendiente de datos reales** — este documento es el plan, no el resultado.

> **Qué es y qué no es.** Es el plan de qué se va a montar. Los montos y cantidades se llenan
> cuando existan las recetas con merma medida, los gastos reales de aduana y los precios.

---

## Ciclo que se va a montar

```
1. Compra de materia prima    → product_purchase + asiento
2. Fabricación                → product_recipe (merma por componente) + horas
3. Importación                → import_cost (flete, aduana, seguro, otros)
4. Entrada a bodega           → product_lot con unit_cost
5. Venta (detalle y mayor)    → invoice + product_price_list
6. Cobranza                   → CxC
7. Compras a proveedor        → CxP
8. Cierre de costos           → cost_snapshot congela
```

---

## Mes 1 (2026-09) — Solo lo que fabrica

Se costean las prendas propias con `RECIPE`, **sin horas todavía**.

| Paso | Qué comprueba |
|---|---|
| Receta con merma por componente | que la tela pague su 10% y el botón su 0% |
| Cierre | que `cost_snapshot` quede lleno |

**Queda marcado**: el margen de este mes es **sin mano de obra**, y así se lee. No es un
resultado final, es el material solo.

---

## Mes 2 (2026-10) — Se añade lo importado

Se registran productos con `IMPORT` y se cargan gastos de aduana en `import_cost`.

**Lo que comprueba**: que los **dos métodos convivan en el mismo catálogo** sin contaminarse.
Si un producto importado sale con receta, o una prenda propia con aduana, los dos orígenes se
están mezclando.

---

## Mes 3 (2026-11) — La aduana con llegada parcial

Se registra una partida donde **no llegaron todas las unidades** (pedido 100, recibido 80).

**Lo que comprueba**: que los gastos se repartan por **80**, no por 100. Con el denominator
equivocado, la diferencia entre lo pedido y lo recibido se pierde del costo y el margen sale
inflado.

---

## Mes 4 (2026-12) — Se empieza a registrar horas

`cost_timesheet` deja de estar vacía y la prenda gana componente de mano de obra.

**Lo que comprueba**: que el costo **suba** cuando entran las horas, y que el aviso de "sin
horas" desaparezca. Si el costo no sube, las horas no se están leyendo.

---

## Mes 5 (2027-01) — Recalculo y verificación de congelamiento

Se recalcular el mes 1 **después de cerrado**, con las horas ya registradas.

**Es la comprobación importante del ciclo**: el costo de septiembre **no debe cambiar** aunque
ahora haya horas que no había. Si cambia, `cost_snapshot` no congeló y el histórico no sirve.

---

## Datos de entrada que faltan

| # | Dato | De dónde sale | Sin esto |
|---|---|---|---|
| 1 | Recetas reales | patrón de corte con tela medida | no hay material que costear |
| 2 | Merma medida por componente | control de calidad de corte | la merma es una suposición |
| 3 | Gastos de aduana por partida | factura del agente aduanero | `customs` sale en 0 |
| 4 | Horas de confección | hoja de horas (no existe todavía) | `labor` sale en 0 con aviso |
| 5 | Precio al mayor | criterio de la empresa | solo hay una lista |

**Los puntos 1 y 2 son los críticos**: una receta con merma inventada es peor que no tener
receta, porque da un costo que parece medido.

---

## Qué NO se monta en esta empresa

- **`product_yield`** — no hay despiece. Deja de estar vacía a propósito.
- **`import_cost`** en las prendas propias — no aplica; solo en lo importado.

Y lo que **sí** se añade después, no se omite: las horas, en el mes 4. Está decidido que el
ciclo empiece sin ellas en vez de inventarlas.
