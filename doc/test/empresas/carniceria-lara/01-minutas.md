# Carnicería Lara — Minutas de decisiones

**Propósito**: registrar por qué se tomó cada decisión. Una decisión sin motivo escrita se
reabre, y se reabre sin recordar lo que se había pensado.

---

## M-01 · El método es `YIELD`, no `RECIPE`

**Decisión**: despiece con `product_yield`, no receta con `product_recipe`.

**Motivo**: la receta describe *de qué se hace* un producto y en qué orden. El despiece no es
eso: es *de qué sale* otro producto. `product_recipe` diría "para hacer lomo hacen falta X kg
de canal", que es al revés de la realidad: la canal es lo que se compra y el lomo es lo que
sale.

Aplicar receta al despiece obligaría a escribir una receta para cada canal, que además no es
determinista: cada animal rinde distinto.

**Se descartó**: simular el despiece con recetas porque "ya existe la tabla". Existir existe,
pero el dato que guarda es el contrario.

---

## M-02 · El rendimiento se mide, no se decide

**Decisión**: el `expected_pct` sale de pesar, no de una estimación.

**Motivo**: el rendimiento es el dato que más dinero mueve de este módulo. Un 72% inventado da
una sensación de precisión que no existe: el costo sale con cinco decimales y es una
suposición con cinco decimales.

**Cómo se consigue**: peso de la canal al entrar, peso de lo que sale a venta, dividido.

**Se descartó**: poner un % redondo "para empezar" sin decir que era una estimación. Si se
hace, queda escrito en `03-ciclo-economico-5-meses.md` junto a la cifra, no solo el número.

---

## M-03 · El costo de la entrada se lee del producto de ENTRADA

**Decisión**: `materialPorRendimiento` lee `product_information.cost` de `input_product_id`
(la canal), no del producto de salida (el corte).

**Motivo**: leer el costo del producto de salida y repartirlo sobre sí mismo es circular: el
corte no tiene costo todavía, que es justo lo que se está calculando. El resultado es cero o
un número sin sentido, y **el rendimiento no entra nunca en la cuenta** — el error más caro
de este método.

Este era uno de los bugs del borrador original, está corregido y tiene test.

---

## M-04 · Dos listas de precio, con `detalle` por defecto

**Decisión**: `price_list` sembrada con `detalle` (default) y `mayor`.

**Motivo**: la venta al mayor es la mitad del negocio. Sin la tabla, un corte tiene un solo
precio y la venta a granel se hace a ojo.

El default es `detalle` porque es la venta que ocurre sin pensar: si un producto no tiene
precio configurado en ninguna lista, debe salir el de mostrador, que es el que no puede
faltar.

---

## M-05 · La carnicería NO usa receta ni horas

**Decisión**: `product_recipe`, `cost_timesheet` y `labor_grade` quedan **vacías**.

**Motivo**: son datos que este negocio no tiene. Llenarlas para "cumplir el formulario" deja
recetas inventadas que generan advertencias y ruido en los informes.

Es la prueba inversa a la de Softwara: la de servicios comprueba que el módulo puede con un
negocio sin inventario; esta comprueba que **no obliga a llenar lo que no corresponde**.

---

## M-06 · El costo no se recalcula después de cerrar el mes

**Decisión**: `cost_snapshot` congela el desglose al cerrar el período.

**Motivo**: si recalcular marzo devuelve otro número, el histórico no sirve. Y en una
carnicería el costo de marzo se usó para poner el precio de venta de abril: si cambia
retroactivamente, el margen de abril se reescribe sin que nadie lo toque.

---

## Pendiente de decidir

Estas no se deciden aquí porque requieren datos reales que aún no están:

| # | Decisión pendiente | Qué hace falta |
|---|---|---|
| P-01 | Si el rendimiento va por lote o por producto | Si hay variabilidad real entre canales |
| P-02 | Cómo se distingue un cliente mayorista | Qué valores admite `customer_type` |
| P-03 | Si la merma de despiece es un costo o una pérdida | Criterio contable del contador |
| P-04 | Redondeo del costo unitario | `decimal(18,4)` o menos, según cómo se facture |
