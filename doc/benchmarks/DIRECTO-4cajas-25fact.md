# Banco: DIRECTO-4cajas-25fact

- **Fecha**: 2026-10-03 23:54:06
- **Cajas registradoras**: 4
- **Facturas por caja**: 25
- **Artículos por factura**: 150
- **Objetivo por caja**: 5.0 s

## Veredicto

**CUMPLE — las 4 cajas cierran sus 25 facturas en 0.11 s (objetivo 5.0 s)**

## Rendimiento

| Medida | Valor |
|---|---|
| Tiempo total | 2.49 s |
| Caja más lenta | 0.11 s (caja #3) |
| Cajas que cumplen 5 s | 4 de 4 |
| Facturas/s | 40 |
| Líneas/s | 6026 |
| Colisiones SQLite | 0 |
| Lecturas ok / fallidas | 2488 / 0 |

## Latencia por factura (ms)

| p50 | p90 | p99 | max | media |
|---|---|---|---|---|
| 24.8 | 37.4 | 1085.8 | 1695.0 | 59.9 |

## Lo que hay en la base (contado después, no contado de memoria)

| Medida | Valor |
|---|---|
| Facturas pedidas | 100 |
| Facturas en la base | 100 |
| Líneas pedidas | 15000 |
| Líneas en la base | 15000 |
| Movimientos de inventario | 15000 (desfase 0) |
| Rastros en audit_log | 0 |
