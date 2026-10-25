# Carnicería Lara — Ciclo económico de 5 meses

**Período**: 2026-09 a 2027-01
**Estado**: **pendiente de datos reales** — este documento es el plan, no el resultado.

> **Qué es y qué no es.** Este es el plan de qué se va a montar. Los números concretos de
> montos, cantidades y fechas se llenan cuando existan los datos de entrada (costo real de las
> canales, rendimiento pesado, precio mayor). Mientras tanto, cualquier cifra que aparezca aquí
> es una **estructura**, no un resultado — y así se lee.

---

## Ciclo que se va a montar

Cada mes repite el mismo ciclo, y **cada paso tiene que producir movimiento contable**:

```
1. Compra de canales          → product_purchase + asiento
2. Despiece                   → product_yield reparte el costo
3. Entrada a bodega           → product_lot con unit_cost
4. Venta (mayor y detalle)    → invoice + product_price_list
5. Cobranza                   → CxC
6. Nómina del mes             → hr_payroll_run (sin horas: no aplica)
7. Cierre de costos           → cost_snapshot congela
```

---

## Mes 1 (2026-09) — Arranque

| Paso | Qué se hace | Qué comprueba |
|---|---|---|
| Compra | 10 canales, se registra el costo en `product_information.cost` | que el costo de entrada esté cargado |
| Despiece | se define `product_yield` con el % pesado | que el rendimiento entre en la cuenta |
| Venta | se usa la lista `detalle` | que salga el precio de mostrador |
| **Cierre** | se congela el período | que `cost_snapshot` quede lleno |

**Es el mes que valida el módulo**: si al cerrar el costo no coincide con lo medido, aquí se
ve.

---

## Mes 2 (2026-10) — Se añade la lista al mayor

Se registra el precio `mayor` en `product_price_list` y se hacen ventas a granel.

**Lo que comprueba**: que el mismo corte tenga **dos costos de venta distintos sobre el mismo
costo**. Si sale un solo precio, la lista `mayor` no se está leyendo.

---

## Mes 3 (2026-11) — Recalculo y verificación de congelamiento

Se recalcular el mes 1 **después de cerrado**.

**Lo que comprueba**: que el costo no cambie. Si cambia, `cost_snapshot` no está congelando y
el histórico de costos no es confiable — es decir, el precio de venta de los meses siguientes
se calculó sobre un número que ya no existe.

---

## Mes 4 (2026-12) — Temporada: variabilidad de rendimiento

Mes de más movimiento. Si hay canales de distinto tipo, se comprueba si el rendimiento varía
mucho entre ellas — que es lo que decide si el `%` va por producto (estado actual) o por lote
(ver P-01 en [01-minutas.md](01-minutas.md)).

---

## Mes 5 (2027-01) — Cierre del ciclo

Se repasa el acumulado de cinco meses:

| Métrica | Esperado |
|---|---|
| Margen > 0 en ambas listas, todos los meses | sí |
| Costo del mes cerrado inalterable | sí |
| Cierre idempotente (2 corridas = mismas filas) | sí |
| Diferencia contra contabilidad | **a determinar** |

---

## Datos de entrada que faltan

Esto es lo que impide montarlo hoy, en orden de importancia:

| # | Dato | De dónde sale | Sin esto |
|---|---|---|---|
| 1 | Costo de la canal | `product_information.cost` | no hay qué repartir |
| 2 | Rendimiento real | báscula: peso entrada / peso salida | el costo sale inventado |
| 3 | Precio mayor | criterio de la empresa | solo hay una lista |
| 4 | Distinción mayorista | `customer_type` | no se sabe a quién aplicar |

**El punto 2 es el crítico**: un rendimiento inventado da un costo con cinco decimales que es
una suposición. Preferible empezar con menos precisión declarada que con más precisión falsa.

---

## Qué NO se monta en esta empresa

Para que quede claro que son decisiones y no omisiones:

- **`product_recipe`** — no fabrica, despiece. Deja de estar vacía a propósito.
- **`cost_timesheet`** — no factura horas. Igual.
- **`import_cost`** — compra local.
