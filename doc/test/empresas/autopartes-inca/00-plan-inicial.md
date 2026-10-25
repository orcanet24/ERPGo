# Distribuidora Inca — Plan inicial

**Fecha**: 2026-10-04
**Tipo**: Venta de autopartes: compra producto terminado, importa, no fabrica; venta al mayor y al detalle
**Método dominante**: `IMPORT` (adquisición + aduana)
**Fabricación**: **ninguna** — por eso está aquí

## Qué hace

Compra producto terminado (repuestos), lo importa del exterior, y lo vende:

- **Al mayor**: a talleres y flotas
- **Al detalle**: al cliente particular

No fabrica nada. No confecciona, no despiece, no receta, no horas.

## Por qué esta empresa está en la lista

Es la prueba de que **el módulo no obliga a lo que un negocio no necesita**.

Tiene sentido inverso al de las otras tres: en vez de comprobar que el motor puede con un caso
complicado, comprueba que **no complica lo sencillo**. Si para costear un repuesto importado
hay que llenar receta, definir rendimiento y registrar horas, el módulo está mal diseñado
aunque todos los números salgan bien.

Se añadió como cuarta empresa después de las otras tres precisamente por esto: un negocio
compra y revende sin fabricar, que es el caso más común del comercio.

## Tablas que usa

| Tabla | Para qué | Necesaria |
|---|---|---|
| `import_cost` | flete, aduana, seguro por partida | **Sí** |
| `product_cost_config` | método `IMPORT` | **Sí** |
| `price_list` + `product_price_list` | al mayor y al detalle | **Sí** |
| `product_recipe` | — | **No** |
| `product_yield` | — | **No** |
| `cost_timesheet` | — | **No** |
| `labor_grade` | — | **No** |

**Cuatro tablas.** Todas las demás deben quedar vacías sin que eso sea un problema.

## Los tres puntos críticos

### 1. La aduana se reparte entre lo recibido

Se pidiieron 200 filtros, llegaron 170 (merma de tránsito, rechazo). El costo correcto:

```
gastos de importación / 170    ← correcto
gastos de importación / 200    ← el error
```

Con el denominator equivocado, **la diferencia entre lo pedido y lo recibido se pierde del
costo**. La empresa paga el flete por 200 y lo reparte como si hubieran llegado 200: el costo
por unidad sale demasiado barato y el margen, inflado.

### 2. Los gastos son de la partida, no del producto

`import_cost` se guarda contra `purchase_id`, no contra `product_id`. Si una partida trae
cinco referencias distintas, los gastos se reparten entre las unidades de esa partida entera.
Guardarlos contra un producto solo obliga a elegir cuál de los cinco paga la aduana.

### 3. El precio es un repuesto de dos listas

`price_list` con `detalle` (default) y `mayor`. El mismo repuesto, dos precios. Sin esta tabla,
la venta a taller y la venta al particular se hacen al mismo precio.

## Datos que faltan

- **Gastos reales de importación**: flete, aduana, seguro, otros. Vienen de la factura del
  agente aduanero y de la declaración de importación.
- **Unidades realmente recibidas**: no las pedidas. Viene del albarán de entrada.
- **Cómo se distingue un cliente mayorista**: igual que en la carnicería. Hay que mirar qué
  valores admite `customer_information.customer_type` en datos reales.

## Benchmark

Detalle en [02-benchmark.md](02-benchmark.md). En resumen: **costo unitario = adquisición +
aduana**, y los dos deben cuadrar contra la factura de importación.

## Riesgo principal

**Configurar de más.** Al ver las otras tres empresas con receta y horas, la tentación es
rellenar tablas que este negocio no usa. Eso sería un error: una receta vacía o inventada
genera advertencias y ruido, y un producto `IMPORT` con una receta colgando es un conflicto
de configuración que el motor tendría que interpretar.

Lo correcto es dejar esas tablas **vacías** y comprobar que el módulo funciona igual.
