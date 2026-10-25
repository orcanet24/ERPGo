# Distribuidora Inca — Benchmark

**Qué se mide**: que un negocio que solo compra, importa y revende se costee con **adquisición +
aduana**, sin receta ni horas, y que las tablas que no usa queden vacías sin que eso sea un
problema.

**Por qué es esta la comprobación**: es la única de las cuatro que comprueba que el módulo **no
obliga a configurar lo que un negocio no necesita**. Si para costear un repuesto hay que llenar
receta o registrar horas, el módulo está mal diseñado aunque el número salga bien.

---

## Caso 1 — Adquisición + aduana

**Entrada**: partida de 170 filtros a 8,00 c/u = 1.360;
gastos de importación: flete 180, aduana 120, seguro 40, otros 10 → **350**.

**Cálculo**:

```
material: 1.360 / 170 =  8,0000
customs:    350 / 170 =  2,0588
                          -------
final                  10,0588
```

**Criterio de fallo**: si `customs` sale **2,0588** con `received_qty = 200`, saldría 1,75 y
el costo por filtro estaría **0,3088 por debajo** de lo real. Margen inflado.

**Criterio adicional**: `labor` y los componentes de receta deben estar **ausentes**, no en
cero con aviso — es un negocio que no fabrica, no un negocio al que le faltan datos.

---

## Caso 2 — Las tablas innecesarias quedan vacías

| Tabla | Estado esperado | Esto es |
|---|---|---|
| `import_cost` | con datos | lo que necesita |
| `price_list` | con datos | lo que necesita |
| `product_recipe` | **vacía** | bien |
| `product_yield` | **vacía** | bien |
| `cost_timesheet` | **vacía** | bien |
| `labor_grade` | **vacía** | bien |

**Criterio de fallo**: el módulo exige receta o horas para costear un repuesto.

Es el criterio que da sentido a esta empresa en el banco de pruebas: comprueba que las
**cuatro** tablas necesarias son suficientes.

---

## Caso 3 — `PP` sin aduana es un error de configuración

| Método | Costo del filtro | Aduana incluida |
|---|---|---|
| `IMPORT` | **10,0588** | sí |
| `PP` | **8,0000** | **no** ← el error |

**Criterio de fallo**: si un producto importado se deja en `PP`, el costo sale **2,0588 por
debajo** de lo real y el margen se infla exactamente en el importe de la aduana.

No es un fallo del módulo: `PP` es el default correcto para lo que se compra localmente. Es
un fallo de **configuración**, y por eso el benchmark lo comprueba como caso — para que la
pantalla de configuración haga visible que un producto importado debería estar en `IMPORT`.

---

## Caso 4 — Dos listas, dos precios

| Lista | Precio | Margen sobre 10,0588 |
|---|---|---|
| `mayor` | 13,50 | +34,2% |
| `detalle` | 17,00 | +69,0% |

**Criterio de fallo**: los dos precios iguales → `product_price_list` no se está leyendo.

**Criterio de negocio**: ninguno de los dos por debajo de 10,0588. Si el precio al mayor sale
menor, la venta a talleres está perdiendo dinero por repuesto.

---

## Caso 5 — Cierre: el costo no cambia

Igual que en las otras tres empresas: después de cerrar el período, el costo de los productos
de ese mes **no se recalcula**.

No es específico de Inca, pero se comprueba aquí también porque el cierre debe funcionar igual
con cuatro tablas llenas que con trece.

---

## El número que hay que alcanzar

**Margen > 0 en ambas listas, con el costo que incluye la aduana.**

Si sale mayor que el margen calculado con `PP`, la diferencia es exactamente lo que estaba
faltando: el flete y la aduana que no entraron en la cuenta.

---

## Por qué esta empresa es la más fácil y la más útil de las cuatro

Lo que comprueba es **negativo**: que algo **no** pasa. No obliga a llenar lo que no toca, no
mezcla métodos, no cuelga gastos donde no van.

Un fallo aquí no daría un número raro; daría un formulario con campos obligatorios que ningún
negocio de estos necesita. Por eso está en el banco de pruebas pese a ser el caso más simple.
