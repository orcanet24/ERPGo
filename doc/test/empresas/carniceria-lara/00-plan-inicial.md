# Carnicería Lara — Plan inicial

**Fecha**: 2026-10-04
**Tipo**: Distribuidora y venta de carne al mayor y al detalle, con carga fabril
**Método dominante**: `YIELD` (rendimiento de despiece)

## Qué hace

Compra canales de res y cerdo, las despiece, y vende:

- **Al detalle**: a través del mostrador
- **Al mayor**: a granel, con precio distinto

"Carga fabril" es el punto que define este negocio: no compra cortes terminados, compra el
animal entero y lo transforma. Todo el costo está en **cómo se reparte el precio de la canal
entre los cortes que salen**.

## Por qué necesita el módulo de costos

Con `product_information.cost` (el CPP del ERP) el negocio no funciona:

| | Sin rendimiento | Con rendimiento |
|---|---|---|
| Canal de 250 kg a 1.000 | 1.000 / 250 = **4,00**/kg | 1.000 / 180 = **5,56**/kg |
| Lo que sale | 180 kg vendibles | 180 kg vendibles |
| Lo que se cobra por los 70 kg de hueso y merma | **nada** | ya está dentro del 5,56 |

Sin rendimiento, **la empresa vende 70 kg sin cobrarlos**. Es un error que no da error: las
cuentas parecen bien hasta que se hace el inventario físico y sobran 70 kg que nadie compró.

## Tablas que usa

| Tabla | Para qué | Necesaria |
|---|---|---|
| `product_yield` | canal → cortes con % de rendimiento | **Sí** — es el corazón |
| `price_list` + `product_price_list` | al mayor y al detalle | **Sí** — dos precios por corte |
| `product_cost_config` | método `YIELD` por producto | **Sí** |
| `product_recipe` | — | No: no fabrica, despiece |
| `cost_timesheet` | — | No: no factura horas |
| `import_cost` | — | No: compra local |

La carnicería **no necesita receta ni horas**. Si el módulo las obligara a llenar, es un fallo
de diseño.

## Los tres puntos críticos

### 1. El rendimiento va sobre lo que sale, no sobre lo que entra

```
canal 250 kg → corte A 180 kg (72%) + hueso/merma 70 kg
costo por kg de venta = costo canal / 180    ← correcto
costo por kg de venta = costo canal / 250    ← el error
```

### 2. Dos precios para el mismo corte

`price_list` se siembra con `detalle` (default) y `mayor`. Sin esta tabla, un kilo de lomo
tiene un solo precio y la venta al mayor se hace a ojo.

### 3. Las mermas de porcentaje se registran por corte

No todas las canales rinden igual: la de novillo rinde distinto a la de vaca vieja. Si solo
hay un % global por producto, un lote bueno paga por uno malo. Se arranca con un % por
producto y se deja anotado que lo ideal es por lote.

## Datos que faltan (y que hay que conseguir antes de simular)

- **Costo real de la canal**: `product_information.cost` de cada canal. Sin eso, el motor no
  tiene qué repartir.
- **Rendimiento real por corte**: no es un número que se invente. Si se pone 72% es porque se
  pesó la canal y se pesó lo que salió.
- **Precio mayor**: cómo se distingue un cliente mayorista de uno de mostrador. Hoy
  `customer_information.customer_type` existe pero no está claro qué valores admite; eso hay
  que mirarlo en datos reales antes de decidir.

## Benchmark

Detalle en [02-benchmark.md](02-benchmark.md). En resumen: el costo por kilo de venta debe ser
**mayor** que el costo de entrada por kilo, exactamente en la proporción del rendimiento. Si
salen iguales, el rendimiento no está entrando en la cuenta.

## Riesgo principal

**Inventar el rendimiento.** Un % que nadie midió es tan malo como no tenerlo, porque da una
falsa sensación de precisión: el costo sale con cinco decimales y es una suposición con
cinco decimales.

El % se toma de una báscula, no de una opinión.
