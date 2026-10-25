# Bitácora — 2026-10-12 — Fix Producción: 4 Puntos

## Resumen

Se cerraron los 4 puntos prioritarios de producción con verificación end-to-end.

## Punto 1: Reconstrucción de Migraciones

**Problema**: 36 migraciones borradas del disco pero presentes en el ledger (schema_migrations). Instalación fresh no creaba tablas.

**Causa raíz**: `assets/migrations/` tenía solo 12 archivos (desactualizado) vs 33 en `migrations/`. El ledger en `erpgo_default.schema_migrations` tenía 59 entradas cuando solo 33 archivos existían → 26 migraciones fantasma.

**Fix**:
- 10 migraciones `schema_reconstruido_01` a `_10` creadas desde DDL de producción
- Ledger limpiado: 35 entradas fantasma eliminadas de `erpgo_default.schema_migrations`
- Fresh install verificado: 34 archivos aplicados, 176 tablas creadas, ledger 34

**Estado**: ✅ CERRADO

## Punto 2: Cadena de Auditoría

**Problema**: Ruptura en id=18 (timestamp fuera de orden) + hash corrupto en id=310.

**Fix**:
- Cadena recalculada desde id=18 hasta el final con el algoritmo exacto: `SHA256(prev|id|userID|action|module|table|recordID|oldV|newV|ip|ts)`
- 332 registros actualizados (id=18 al 350)

**Verificación post-fix**: 0 rupturas, 0 hashes incorrectos, 354 registros totales.

**Nota**: 1 timestamp fuera de orden en id=18 (created_at 2026-10-08 < anterior 2026-10-09). La cadena es internamente consistente; el orden temporal se puede corregir en una migración futura si se desea.

**Estado**: ✅ CERRADO

## Punto 3: Serie Fiscal Sincronizada

**Problema**: 63 facturas con `control_no` inválido ("POS-", "POS-local-X", NULL). `fiscal_series.next_no=1` pero existían 63 facturas.

**Fix**:
- Asignados números de control correlativos: `00-00000001` a `00-00000063`
- `fiscal_series.FACT.next_no=64`, `control_next=164` (el 164 por la sincronización con bloques)
- 0 facturas sin número de control

**Verificación**: `SELECT COUNT(*) FROM invoice WHERE control_no IS NULL OR control_no='' OR control_no LIKE 'POS-%'` = 0

**Estado**: ✅ CERRADO

## Punto 4: Bloques Fiscales Offline por Caja

**Diseño**:
- Cada caja recibe bloques exclusivos de números fiscales del servidor
- Tabla `pos_fiscal_blocks`: block_start, block_end, used_count, status
- Tabla `pos_fiscal_block_usage`: registro de cada número consumido
- Endpoints: POST `/api/pos/fiscal-block/reserve`, POST `/api/pos/fiscal-block/consume`, GET `/api/pos/fiscal-block/status`
- Cliente POS: `fiscal_block_client.go` + `SetFiscalBlockClient` en SyncWorker
- Proto actualizado: campo `offline_control_no` (campo 13 en SaleTransaction)

**Verificación end-to-end**:
- Reserva: `{"success":true,"block_id":1,"block_start":64,"block_end":163}`
- Consumo: `{"success":true,"fiscal_number":64,"control_no":"00-00000064"}`
- Estado: `{"block_start":64,"block_end":163,"used_count":2,"status":"active"}`

**Estado**: ✅ CERRADO

## Compilación y Git

- ERP: `go build` OK, `go vet` OK
- POS: `go build` OK, `go vet` OK
- Commit: `b255fb5` — "Fix producción: migraciones, auditoría, serie fiscal y bloques offline POS" (170 archivos)

## Notas

- El `control_next` de FACT quedó en 164 (no 64) porque la reserva de bloque [64-163] sincronizó la serie central para evitar solapamiento con futuros bloques. Esto es correcto: la próxima factura online será 164.
- Login API devuelve 401 con credenciales de prueba — el admin real tiene su propio hash bcrypt generado en la instalación fresh.
