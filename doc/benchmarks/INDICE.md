# Índice de bancos de pruebas

Este fichero se REHACE en cada ensayo a partir de los `*.json` de esta
misma carpeta, así que no puede quedar ni con cabeceras duplicadas ni
con filas perdidas. Lo más reciente primero: un banco que empeora sin que
nadie lo mire es un banco que no sirve.

| Fecha | Prueba | Modo | Cajas | Facturas | Artículos | Total (s) | Facturas/s | Líneas/s | p99 (ms) | Colisiones | Veredicto |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-10-04 00:21:03 | ANTES-50cajas-500art | cola | 50 | 20 | 500 | 417.21 | 2 | 1198 | 40879.2 | 0 | SIN PERDER NADA PERO FUERA DE PLAZO — 0 de 50 cajas cumplen 5.0 s; l... |
| 2026-10-04 00:07:53 | WAL-con-lectores | cola | 8 | 40 | 150 | 40.76 | 8 | 1178 | 2244.3 | 0 | CUMPLE — las 8 cajas cierran sus 40 facturas en 0.94 s (objetivo 5.0... |
| 2026-10-03 23:54:15 | COLA-4cajas-25fact | cola | 4 | 25 | 150 | 2.64 | 38 | 5681 | 142.5 | 0 | CUMPLE — las 4 cajas cierran sus 25 facturas en 0.14 s (objetivo 5.0... |
| 2026-10-03 23:54:06 | DIRECTO-4cajas-25fact | directo | 4 | 25 | 150 | 2.49 | 40 | 6026 | 1085.8 | 0 | CUMPLE — las 4 cajas cierran sus 25 facturas en 0.11 s (objetivo 5.0... |
| 2026-10-03 23:40:52 | escala-200cajas-150art | cola | 200 | 20 | 150 | 506.97 | 8 | 1183 | 34833.5 | 0 | SIN PERDER NADA PERO FUERA DE PLAZO — 0 de 200 cajas cumplen 5.0 s; ... |
| 2026-10-03 23:40:38 | AB-cola | cola | 4 | 5 | 50 | 0.15 | 132 | 6588 | 40.0 | 0 | CUMPLE — las 4 cajas cierran sus 5 facturas en 0.03 s (objetivo 5.0 ... |
| 2026-10-03 23:40:33 | AB-directo | directo | 4 | 5 | 50 | 0.20 | 100 | 4999 | 116.4 | 0 | CUMPLE — las 4 cajas cierran sus 5 facturas en 0.01 s (objetivo 5.0 ... |
