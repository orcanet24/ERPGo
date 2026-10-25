# Banco: AB-directo

- **Fecha**: 2026-10-03 23:40:33
- **Cajas registradoras**: 4
- **Facturas por caja**: 5
- **Artículos por factura**: 50
- **Objetivo por caja**: 5.0 s

## Veredicto

**CUMPLE — las 4 cajas cierran sus 5 facturas en 0.01 s (objetivo 5.0 s)**

## Rendimiento

| Medida | Valor |
|---|---|
| Tiempo total | 0.20 s |
| Caja más lenta | 0.01 s (caja #2) |
| Cajas que cumplen 5 s | 4 de 4 |
| Facturas/s | 100 |
| Líneas/s | 4999 |
| Colisiones SQLite | 0 |
| Lecturas ok / fallidas | 475 / 0 |

## Latencia por factura (ms)

| p50 | p90 | p99 | max | media |
|---|---|---|---|---|
| 7.6 | 68.1 | 116.4 | 168.3 | 24.9 |

## Lo que hay en la base (contado después, no contado de memoria)

| Medida | Valor |
|---|---|
| Facturas pedidas | 20 |
| Facturas en la base | 20 |
| Líneas pedidas | 1000 |
| Líneas en la base | 1000 |
| Movimientos de inventario | 1000 (desfase 0) |
| Rastros en audit_log | 0 |
