# Distribuidora Inca — Minutas de decisiones

**Propósito**: registrar por qué se tomó cada decisión.

---

## M-01 · Se añadió como CUARTA empresa, no como tercera

**Decisión**: Autopartes Inca entra después de las otras tres, no antes.

**Motivo**: las tres primeras comprueban que el módulo **puede** con un caso difícil (despiece,
doble origen de costos, negocio sin inventario). Esta comprueba que **no complica lo sencillo**.

Un negocio que compra, importa y revende es el caso más común del comercio. Si para costear un
repuesto hay que llenar receta, definir rendimiento y registrar horas, el módulo está mal
diseñado aunque todos los números salgan bien.

Elegirla la primera habría dejado los tres casos difíciles sin probar.

---

## M-02 · `product_recipe` y `cost_timesheet` quedan VACÍAS

**Decisión**: no se siembran datos en esas tablas para esta empresa.

**Motivo**: una receta vacía o inventada genera advertencias y ruido en los informes. Un
producto `IMPORT` con una receta colgando es un conflicto de configuración que el motor tendría
que interpretar, y no debería tener que interpretarlo.

**Se descartó**: rellenar las tablas "por si acaso" o porque las otras empresas las usan. Es
la tentación natural al ver los otros directorios, y es un error.

---

## M-03 · Los gastos de importación van contra la PARTIDA

**Decisión**: `import_cost.purchase_id`, nunca `product_id`.

**Motivo**: una partida de autopartes trae varias referencias distintas. Los gastos de aduana
son de la partida entera: guardarlos contra un solo producto obligaría a elegir cuál paga la
aduana, y esa elección sería arbitraria.

---

## M-04 · La aduana se reparte entre lo RECIBIDO

**Decisión**: `received_qty`, no la cantidad pedida.

**Motivo**: se pidieron 200 filtros y llegaron 170. Con el denominator equivocado, la
diferencia entre lo pedido y lo recibido **se pierde del costo**: la empresa paga el flete por
200 y lo reparte como si hubieran llegado 200, y el costo por unidad sale demasiado barato.

Es el bug 0.2 del plan. Corregido y con test.

---

## M-05 · Dos listas de precio, igual que la carnicería

**Decisión**: `detalle` (default) y `mayor`.

**Motivo**: la venta a talleres y flotas es la mitad del negocio. Sin la tabla, el mismo
repuesto tiene un solo precio y la venta a granel se hace a ojo.

---

## M-06 · El método es `IMPORT`, no `PP`

**Decisión**: aunque el default de la migración es `PP`, los productos importados se marcan
como `IMPORT`.

**Motivo**: `PP` responde "cuánto me costó comprarlo" usando el costo del kardex, y **no
incluye la aduana**. Un repuesto con `PP` saldría sin el flete ni la aduana, y el margen
saldría inflado exactamente en la proporción de esos gastos.

`PP` sigue siendo el default para los productos que se compran localmente, que es lo que hace
segura esta migración sobre instalaciones existentes.

---

## Pendiente de decidir

| # | Decisión pendiente | Qué hace falta |
|---|---|---|
| P-01 | Si la aduana se reparte por peso o por valor | Criterio fiscal |
| P-02 | Si un producto comprado localmente usa `PP` o `IMPORT` sin gastos | Conveniencia |
| P-03 | Cómo se distingue un cliente mayorista | Qué valores admite `customer_type` |
| P-04 | Si la partida con varias referencias se costea todo junto | Si se factura por referencia |
