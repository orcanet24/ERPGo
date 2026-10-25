# Banco: COLA-4cajas-25fact

- **Fecha**: 2026-10-03 23:54:15
- **Cajas registradoras**: 4
- **Facturas por caja**: 25
- **Artículos por factura**: 150
- **Objetivo por caja**: 5.0 s

## Veredicto

**CUMPLE — las 4 cajas cierran sus 25 facturas en 0.14 s (objetivo 5.0 s)**

## Rendimiento

| Medida | Valor |
|---|---|
| Tiempo total | 2.64 s |
| Caja más lenta | 0.14 s (caja #0) |
| Cajas que cumplen 5 s | 4 de 4 |
| Facturas/s | 38 |
| Líneas/s | 5681 |
| Colisiones SQLite | 0 |
| Lecturas ok / fallidas | 2830 / 0 |

## Latencia por factura (ms)

| p50 | p90 | p99 | max | media |
|---|---|---|---|---|
| 104.0 | 132.3 | 142.5 | 145.5 | 103.7 |

## Lo que hay en la base (contado después, no contado de memoria)

| Medida | Valor |
|---|---|
| Facturas pedidas | 100 |
| Facturas en la base | 100 |
| Líneas pedidas | 15000 |
| Líneas en la base | 15000 |
| Movimientos de inventario | 15000 (desfase 0) |
| Rastros en audit_log | 100 |
