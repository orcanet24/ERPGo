# Textilera El Telar — Benchmark

**Qué se mide**: que los **dos** orígenes de costo se costeen por separado y no se mezclen.

**Por qué es esta la comprobación**: la textilera es la única de las cuatro con dos métodos
viviendo en el mismo catálogo. Si un zapato importado sale con receta, o una camisa propia con
aduana, el módulo está mezclando dos casos que nada tienen que ver — y lo peor es que saldría
un número, no un error.

---

## Caso 1 — La camisa propia: receta con merma por componente

**Entrada**:

| Componente | Cant. | Costo ud | Merma |
|---|---|---|---|
| Tela | 1,8 m | 4,00 | 10% |
| Botón | 6 ud | 0,10 | 0% |
| Cierre | 1 ud | 0,80 | 2% |

**Cálculo**:

```
tela:   1,8 × 4,00 × 1,10 = 7,920
botón:  6,0 × 0,10 × 1,00 = 0,600
cierre: 1,0 × 0,80 × 1,02 = 0,816
                                -------
material                          9,336
```

**Criterio de fallo**: si sale **9,735** (merma global del 5% sobre 9,30), la merma se aplicó
como global y **el botón pagó merma que no tiene**.

Tiene test: `TestLaMermaPorComponenteNoEsGlobal`, que además comprueba explícitamente que el
resultado **no** coincide con el de la merma global.

---

## Caso 2 — El zapato importado: aduana y nada más

**Entrada**: partida de 100 pares a 12,00 c/u = 1.200; gastos de importación 400;
llegaron **80** pares.

**Cálculo**:

```
material: 1.200 / 80 = 15,0000
customs:    400 / 80 =  5,0000
                          -------
final                  20,0000
```

**Criterio de fallo**: si `customs` sale **5,0000** con `received_qty = 100`, saldría 4,0000
y el costo por par estaría **1,00 por debajo** de lo que realmente costó. Margen inflado.

**Criterio adicional**: el zapato **no debe tener componente de receta** ni de mano de obra.
Si los tiene, los dos métodos se están mezclando.

---

## Caso 3 — Los dos métodos no se contaminan

| Producto | Esperado | No debe aparecer |
|---|---|---|
| Camisa (fabricada) | `material` + `labor` | `customs` |
| Zapato (importado) | `material` + `customs` | `labor` (sin horas registradas) |

**Criterio de fallo**: `customs > 0` en un producto fabricado localmente.

Es el error más silencioso de los cuatro: un gasto de aduana colgado de un producto que no se
importó. No revienta, no avisa, y sale un costo más caro de lo que es.

---

## Caso 4 — Las horas se añaden después, y se dice

**Estado actual**: sin horas registradas → `labor = 0` con **aviso explícito**.

**Criterio de fallo**: la camisa sale con `labor = 0` y **sin** aviso. Un costo de fabricación
sin mano de obra que no lo dice es un dato falso presentado como correcto.

Es el mismo criterio que en Softwara: el motor no inventa el dato, pero **tiene que decir que
falta**.

---

## Caso 5 — Margen con y sin mano de obra

| Concepto | Valor | Nota |
|---|---|---|
| Material de la camisa | 9,336 | medido |
| Horas de confección | **pendiente** | no se registran todavía |
| Precio detalle | 25,00 | — |
| Precio mayor | 21,00 | — |

**Margen reportado** = 25,00 − 9,336 = **15,664**

**Criterio**: ese número **no es el margen real**, es el margen *sin mano de obra*. Tiene que
venir etiquetado así. Si se presenta como margen completo, la empresa está sobrevalorando lo
que gana por prenda.

---

## El número que hay que alcanzar

**Los dos métodos producen desgloses mutuamente exclusivos sobre el mismo catálogo.**

Eso es lo que comprueba que el módulo sirve para un negocio con dos orígenes de costo, que es
el caso que ninguna de las otras tres empresas plantea.
