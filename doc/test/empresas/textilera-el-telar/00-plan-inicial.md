# Textilera El Telar — Plan inicial

**Fecha**: 2026-10-04
**Tipo**: Fabricación de ropa propia + importación de calzado y textil; venta al detalle y al mayor
**Métodos**: `RECIPE` (fabricación) + `IMPORT` (lo que importa)

## Qué hace

Dos negocios en uno:

1. **Fabrica ropa**: compra materia prima (tela, botones, cierre) y confecciona
2. **Importa**: calzado y textil terminado, que solo revende

Y vende **al detalle y al mayor**, como la carnicería.

## Por qué necesita el módulo de costos

Tiene **dos orígenes de costo distintos**, y ese es exactamente el problema. Un solo método
no le sirve:

| Producto | Cómo llega | Qué hay que costear |
|---|---|---|
| Camisa propia | Se confecciona | Receta con merma + horas de confección |
| Zapato importado | Se compra en el exterior | Costo de adquisición + flete + aduana |

Si el módulo obliga al zapato a tener receta, le estás pidiendo que declare de qué está hecho
un producto que no fabricaste. Si le obliga a la camisa a tener aduana, lo mismo al revés.

## Tablas que usa

| Tabla | Para qué | Necesaria |
|---|---|---|
| `product_recipe` | tela, botones, cierre por prenda | **Sí** — para lo que fabrica |
| `product_cost_config` | `RECIPE` o `IMPORT` por producto | **Sí** |
| `import_cost` | flete, aduana, seguro por partida | **Sí** — para lo que importa |
| `price_list` | al mayor y al detalle | **Sí** |
| `labor_grade` + `cost_timesheet` | horas de confección | **Sí**, pero después |
| `product_yield` | — | No: no despiece |

## Los tres puntos críticos

### 1. La merma es por componente, no global

Es el caso donde se nota más. En una camisa:

| Componente | Cantidad | Merma |
|---|---|---|
| Tela | 1,8 m | **10%** — se pierde en el corte |
| Botón | 6 uds | **0%** — no se pierde |
| Cierre | 1 ud | **2%** |

Con una merma global del 5% sobre la receta entera, **el botón paga merma que no tiene**. Con
merma por componente, cada uno paga lo suyo.

### 2. La aduana se reparte entre lo recibido, no entre lo pedido

Se pidiieron 100 pares y llegaron 80 (merma de tránsito, rechazo). El costo correcto es
`gastos / 80`, no `gastos / 100`.

Con `gastos / 100`, **la diferencia entre lo pedido y lo recibido desaparece del costo**: la
empresa paga el flete por 100 pero lo reparte como si hubieran llegado 100, y el costo por
unidad sale demasiado barato.

### 3. El setup no se prorratea

`process_route.setup_time_min` es el tiempo de preparar la máquina, y se paga aunque se
fabrique una sola prenda. Si se divide entre la cantidad, **el primer lote sale absurdamente
barato** y el siguiente lo paga.

## Datos que faltan

- **Recetas reales**: tela por prenda con su merma medida, no estimada.
- **Horas de confección por prenda**: sin esto, la mano de obra sale en cero. Se puede
  empezar con `labor_grade` + una estimación de horas por prenda, pero hay que decir que es
  una estimación.
- **Componentes del costo de importación**: flete, aduana, seguro, otros. Están en la factura
  del agente aduanero.

## Benchmark

Detalle en [02-benchmark.md](02-benchmark.md). En resumen: la camisa propia debe costear
**material + mano de obra + indirectos** y el zapato **adquisición + aduana**, y los dos
métodos no deben interferirse.

## Riesgo principal

**La línea difusa entre "fabrica" y "importa".** Si un producto se marca como `IMPORT` pero
luego se le añade una receta, o al revés, el motor no sabe qué hacer y calcula lo primero que
encuentre. La configuración por producto (`product_cost_config.costing_method`) es la única
fuente de verdad, y la pantalla de configuración debe dejar ver cuál está aplicado a cada
producto.
