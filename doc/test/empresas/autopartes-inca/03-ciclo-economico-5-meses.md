# Distribuidora Inca — Ciclo económico de 5 meses

**Período**: 2026-09 a 2027-01
**Estado**: **pendiente de datos reales** — este documento es el plan, no el resultado.

> **Qué es y qué no es.** Es el plan de qué se va a montar. Los montos se llenan cuando
> existan las facturas de importación y las unidades realmente recibidas.

---

## Ciclo que se va a montar

Es el más corto de los cuatro: **compra, importa, revende**. Sin fabricación, sin despiece, sin
horas.

```
1. Compra + importación       → product_purchase + import_cost (flete, aduana, seguro)
2. Entrada a bodega           → product_lot con unit_cost
3. Venta (mayor y detalle)    → invoice + product_price_list
4. Cobranza                   → CxC
5. Pago a proveedor           → CxP
6. Nómina del mes             → hr_payroll_run
7. Cierre de costos           → cost_snapshot congela
```

---

## Mes 1 (2026-09) — Arranque con una sola partida

Se registra la primera importación con todos los gastos en `import_cost`.

**Lo que comprueba**: que los gastos se repartan sobre las unidades **recibidas**. Si se
reparten sobre las pedidas, el costo sale por debajo de lo real y el margen se infla en la
proporción de la aduana.

---

## Mes 2 (2026-10) — Se añade la lista al mayor

Se registra el precio `mayor` y se hacen ventas a talleres.

**Lo que comprueba**: que el mismo repuesto tenga **dos precios sobre el mismo costo**. Si sale
un solo precio, `product_price_list` no se está leyendo.

---

## Mes 3 (2026-11) — Llegada parcial

Se registra una partida donde **no llegaron todas las unidades** (pedido 200, recibido 170).

**Lo que comprueba**: el caso crítico de la aduana. Con `received_qty = 170` los gastos salen
repartidos sobre 170; con 200, el costo por filtro sale más barato y el margen inflado.

---

## Mes 4 (2026-12) — Se comprueba que `PP` no esté siendo el método

Se revisa que los productos importados estén marcados como `IMPORT` y no como `PP`.

**Lo que comprueba**: `PP` no incluye la aduana. Si un producto importado quedó en `PP`, el
costo sale **sin flete ni aduana** y el margen se infla en exactamente ese importe.

Es un fallo de configuración, no del módulo — pero es el más silencioso de esta empresa, y por
eso tiene su mes.

---

## Mes 5 (2027-01) — Recalculo y cierre del ciclo

Se recalcular el mes 1 **después de cerrado** y se revisa el acumulado:

| Métrica | Esperado |
|---|---|
| Margen > 0 en ambas listas | sí |
| Costo del mes cerrado inalterable | sí |
| Cierre idempotente (2 corridas = mismas filas) | sí |
| Diferencia contra contabilidad | **a determinar** |

---

## Datos de entrada que faltan

| # | Dato | De dónde sale | Sin esto |
|---|---|---|---|
| 1 | Unidades realmente recibidas | albarán de entrada | la aduana no se reparte bien |
| 2 | Gastos de importación | factura del agente aduanero | `customs` sale en 0 |
| 3 | Precio al mayor | criterio de la empresa | solo hay una lista |
| 4 | Distinción mayorista | `customer_type` | no se sabe a quién aplicar |
| 5 | Si un producto es `IMPORT` o `PP` | decisión al configurar | si es `PP`, falta la aduana |

**El punto 1 es el crítico**: sin saber cuánto llegó de verdad, cualquier reparto de aduana es
una suposición.

---

## Qué NO se monta en esta empresa

Las cuatro tablas que no usa quedan **vacías**, y eso es el resultado esperado, no una
pendiente:

- **`product_recipe`** — no fabrica.
- **`product_yield`** — no hay despiece.
- **`cost_timesheet`** — no factura horas.
- **`labor_grade`** — no tiene costos de mano de obra por proyecto.

Si en cualquier momento el módulo exige una de ellas, ese es el fallo que esta empresa está
puesta a detectar.
