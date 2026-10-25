# Banco: escala-200cajas-150art

- **Fecha**: 2026-10-03 23:40:52
- **Cajas registradoras**: 200
- **Facturas por caja**: 20
- **Artículos por factura**: 150
- **Objetivo por caja**: 5.0 s

## Veredicto

**SIN PERDER NADA PERO FUERA DE PLAZO — 0 de 200 cajas cumplen 5.0 s; la más lenta tardó 30.64 s**

## Rendimiento

| Medida | Valor |
|---|---|
| Tiempo total | 506.97 s |
| Caja más lenta | 30.64 s (caja #82) |
| Cajas que cumplen 5 s | 0 de 200 |
| Facturas/s | 8 |
| Líneas/s | 1183 |
| Colisiones SQLite | 0 |
| Lecturas ok / fallidas | 21472 / 0 |

## Latencia por factura (ms)

| p50 | p90 | p99 | max | media |
|---|---|---|---|---|
| 25847.2 | 30731.6 | 34833.5 | 35453.7 | 24669.5 |

## Lo que hay en la base (contado después, no contado de memoria)

| Medida | Valor |
|---|---|
| Facturas pedidas | 4000 |
| Facturas en la base | 4000 |
| Líneas pedidas | 600000 |
| Líneas en la base | 600000 |
| Movimientos de inventario | 600000 (desfase 0) |
| Rastros en audit_log | 4000 |
