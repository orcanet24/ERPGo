# Textilera El Telar — Minutas de decisiones

**Propósito**: registrar por qué se tomó cada decisión. Una decisión sin motivo escrita se
reabre, y se reabre sin recordar lo que se había pensado.

---

## M-01 · Dos métodos, no uno para todo

**Decisión**: `RECIPE` para lo que fabrica, `IMPORT` para lo que importa. El método se elige
**por producto** en `product_cost_config`.

**Motivo**: la textilera tiene dos orígenes de costo distintos en el mismo catálogo. Un zapato
importado no tiene receta —fabricar nada— y una camisa propia no tiene aduana. Un solo método
para todo obligaría a inventar datos que no existen en uno de los dos lados.

**Se descartó**: un método `HYBRID` que decidiera solo según si hay receta. El problema es
que "hay receta" y "debería haber receta" no son lo mismo: un producto importado al que
alguien le añade una receta por error pasaría a costearse como si se fabricara, y el error
sería indetectable. El método es una decisión explícita del administrador.

---

## M-02 · La merma es por componente, no global

**Decisión**: `product_recipe.loss_factor` se aplica **fila a fila**.

**Motivo**: en una camisa real la tela se pierde en el corte (10%) y el botón no se pierde
(0%). Con una merma global del 5% sobre la receta entera, **el botón paga una merma que no
tiene** y la tela paga la mitad de la que tiene.

El error no da aviso: el costo sale con cinco decimales y está mal. Tiene test
(`TestLaMermaPorComponenteNoEsGlobal`), que comprueba además que el resultado **no** coincide
con el de una merma global.

---

## M-03 · La aduana se reparte entre lo RECIBIDO

**Decisión**: `import_cost.received_qty`, nunca la cantidad pedida.

**Motivo**: se pidieron 100 pares y llegaron 80. Si se reparte por 100, la diferencia entre
lo pedido y lo recibido **desaparece del costo**: la empresa paga el flete por 100 y lo reparte
como si hubieran llegado 100.

El costo por unidad sale demasiado barato y el margen, inflado. Es un error silencioso porque
el número es plausible.

Era uno de los bugs del borrador original (ver 0.2 del plan). Corregido y con test.

---

## M-04 · Los gastos son de la PARTIDA, no del producto

**Decisión**: `import_cost` se guarda contra `purchase_id`, no contra `product_id`.

**Motivo**: una partida puede traer cinco referencias distintas. Guardar los gastos contra un
solo producto obliga a elegir cuál de los cinco paga la aduana, y esa elección sería
arbitraria.

Con `purchase_id`, los gastos se reparten entre todas las unidades de la partida, que es
fiscalmente lo correcto: la aduana la paga la partida.

---

## M-05 · El tiempo de setup no se prorratea

**Decisión**: `process_route.setup_time_min` no se divide entre la cantidad.

**Motivo**: es el tiempo de preparar la máquina, y se paga aunque se fabrique una sola prenda.
Si se divide, **el primer lote sale absurdamente barato** y el siguiente paga el resto.

---

## M-06 · La mano de obra se añade después, y se dice

**Decisión**: la camisa se costea primero sin horas, con `cost_timesheet` marcado como
pendiente.

**Motivo**: las horas de confección no se registran todavía. Ponerlas de una vez sería
inventar un dato. Preferible un costo que diga "sin horas de confección" a uno que diga un
número que nadie midió.

Queda escrito en el benchmark: el margen que salga es **margen sin mano de obra**, y así se
lee.

---

## Pendiente de decidir

| # | Decisión pendiente | Qué hace falta |
|---|---|---|
| P-01 | Si la merma de corte va por prenda o por rollo | Datos de merma reales por patrón |
| P-02 | Si el setup se carga a un producto o a un centro de costo | Costo real del setup |
| P-03 | Si lo importado se costea con la tasa del día de compra o del arribo | Criterio contable |
| P-04 | Si el margen al mayor es un % o un precio fijo | Costumbre de la empresa |
