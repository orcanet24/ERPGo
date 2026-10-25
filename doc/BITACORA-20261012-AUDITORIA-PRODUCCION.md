# BITÁCORA — 2026-10-12 · Auditoría de producción (ERPGo / POS / SDK)

## Objetivo

Determinar, con honestidad y contra el código real, si ERPGo, el POS y el SDK están
listos para salir a operatividad. El usuario pidió expresamente no recibir una lista de
"falta esto o aquello" sin sustento, sino un veredicto verificable.

## Resultado

| Sistema | Veredicto |
|---|---|
| ERPGo web | **Casi** — faltan 3 correcciones fiscales y cerrar la brecha de migraciones |
| POS | **NO** — el código fuente no está en el disco; no es auditable |
| SDK | **NO** — Fase 1 de 5; el import del README no compila |

Documento completo: `doc/AUDITORIA-20261012-PRODUCCION.md`

## Corregido y verificado en esta sesión

| # | Fallo | Verificación |
|---|---|---|
| C-1 | `/kardex` → 404 | `curl -L /kardex` → 200 |
| C-2 | Kárdex con datos → 500 (`float64` vs `int` en template) | 200, 58 KB |
| C-3 | CPP del Kárdex mostraba la CANTIDAD como costo | Antes `0.00`, ahora `40.00` |
| C-4 | Kárdex ignoraba devoluciones/ajustes y no ordenaba | Saldo corrido correcto |
| C-5 | 4 templates inexistentes → 500 | **60/60 enlaces del sidebar → 200** |
| C-6 | `tenants` nunca se creaba | Instalación limpia registra la empresa |
| C-7 | Admin creado con usuario vacío | `username=admin`, login OK |

Pruebas de plataforma: `go build ./...` limpio, `go vet ./...` limpio,
instalación limpia 0 errores / 175 tablas.

## Pendiente (con el riesgo explícito)

| # | Hallazgo | Gravedad |
|---|---|---|
| H-1 | El POS fabrica su propio `control_no` ("POS-") sin consumir la serie fiscal. 48 facturas comparten el mismo número | **CRÍTICO (SENIAT)** |
| H-2 | Código fuente del POS inexistente (solo `.exe`) | **CRÍTICO (bloqueador raíz)** |
| H-3 | 18 migraciones aplicadas en la BD pero borradas del disco | ALTO |
| H-4 | 43 columnas multi-moneda sin migración que las cree | ALTO |
| H-5 | Cadena de auditoría rota (falta el id 284 en `audit_log`) | ALTO |
| H-6 | SDK: el import del README no resuelve | MEDIO |
| H-7 | 4 bases de datos residuales de pruebas | BAJO |

## Decisiones tomadas

1. **H-1 no se tocó.** Arreglarlo exige modificar el POS, y el POS no tiene fuente.
   Parchear el lado del ERP sin el POS dejaría el arreglo a medias y daría falsa
   sensación de cierre. Se documenta y se espera el repositorio.
2. **C-4 se resolvió unificando, no parcheando.** Las tres vistas del Kárdex tenían
   cada una su copia del cálculo del CPP y las tres divergían. Un libro fiscal no puede
   tener tres respuestas distintas: ahora las tres llaman a `kardexMovements()`.
3. **C-7 genera credencial en vez de fallar.** Quien arranca sin asistente (migración,
   siembra) necesita un admin usable; negarse a arrancar sería peor. Se imprime UNA vez.
4. **Las bases de prueba creadas durante la auditoría se borraron al terminar**, para no
   agravar la proliferación de bases que ya tiene el proyecto.

## Riesgo que el usuario debe conocer

El POS **no puede declararse listo** y es la pieza que factura. El fallo H-1
(números de control repetidos) es exactamente el tipo de incumplimiento que el usuario
mencionó como riesgo legal. Sin el código fuente del POS no hay forma de corregirlo ni
de auditarlo.

## Próximo paso sugerido

1. Obtener el repositorio del POS → desbloquea H-1.
2. Corregir H-1: que el POS llame a `nextFiscalDoc(tx, "FACT")`.
3. Reconstruir las migraciones perdidas (H-3, H-4) y validar instalación limpia.
4. Decidir sobre la cadena de auditoría (H-5): reparar o documentar el hueco.
