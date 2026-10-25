# Banco: AB-cola

- **Fecha**: 2026-10-03 23:40:38
- **Cajas registradoras**: 4
- **Facturas por caja**: 5
- **Artículos por factura**: 50
- **Objetivo por caja**: 5.0 s

## Veredicto

**CUMPLE — las 4 cajas cierran sus 5 facturas en 0.03 s (objetivo 5.0 s)**

## Rendimiento

| Medida | Valor |
|---|---|
| Tiempo total | 0.15 s |
| Caja más lenta | 0.03 s (caja #2) |
| Cajas que cumplen 5 s | 4 de 4 |
| Facturas/s | 132 |
| Líneas/s | 6588 |
| Colisiones SQLite | 0 |
| Lecturas ok / fallidas | 371 / 0 |

## Latencia por factura (ms)

| p50 | p90 | p99 | max | media |
|---|---|---|---|---|
| 22.6 | 37.3 | 40.0 | 51.4 | 28.5 |

## Lo que hay en la base (contado después, no contado de memoria)

| Medida | Valor |
|---|---|
| Facturas pedidas | 20 |
| Facturas en la base | 20 |
| Líneas pedidas | 1000 |
| Líneas en la base | 1000 |
| Movimientos de inventario | 1000 (desfase 0) |
| Rastros en audit_log | 20 |
