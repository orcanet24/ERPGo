# Banco: WAL-con-lectores

- **Fecha**: 2026-10-04 00:07:53
- **Cajas registradoras**: 8
- **Facturas por caja**: 40
- **Artículos por factura**: 150
- **Objetivo por caja**: 5.0 s

## Veredicto

**CUMPLE — las 8 cajas cierran sus 40 facturas en 0.94 s (objetivo 5.0 s)**

## Rendimiento

| Medida | Valor |
|---|---|
| Tiempo total | 40.76 s |
| Caja más lenta | 0.94 s (caja #4) |
| Cajas que cumplen 5 s | 8 de 8 |
| Facturas/s | 8 |
| Líneas/s | 1178 |
| Colisiones SQLite | 0 |
| Lecturas ok / fallidas | 10426 / 0 |

## Latencia por factura (ms)

| p50 | p90 | p99 | max | media |
|---|---|---|---|---|
| 877.4 | 1698.8 | 2244.3 | 2635.8 | 1011.5 |

## Lo que hay en la base (contado después, no contado de memoria)

| Medida | Valor |
|---|---|
| Facturas pedidas | 320 |
| Facturas en la base | 320 |
| Líneas pedidas | 48000 |
| Líneas en la base | 48000 |
| Movimientos de inventario | 48000 (desfase 0) |
| Rastros en audit_log | 320 |
