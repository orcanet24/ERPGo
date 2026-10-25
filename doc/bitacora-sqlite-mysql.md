# Bitácora — Soporte Dual de Base de Datos: SQLite (pequeño negocio) + MySQL (grande)

> **⚠️ ESTADO ACTUALIZADO (2 de octubre de 2026).** Este documento describe el
> soporte dual **SQLite + MySQL**, que fue implementado y verificado entonces.
> **Ese código ya no está en el sistema.** La decisión posterior fue SQLite
> 100% (una base por empresa, sin `DB_TYPE`, sin migración de motor).
>
> Lo que se conserva de aquí:
> - La concurrencia WAL (`_txlock=immediate`, `busy_timeout`, pool de 25) — sí
>   sigue, y es lo que sostiene el multiusuario.
> - El bug de `MaxOpenConns(1)` que congelaba el POS — corregido y vigente.
> - La cadena de auditoría por bloques (§7) — implementada en `audit_chain.go`.
> - El benchmark de `tools/loadtest` — vigente y reproducible.
>
> Lo que ya no existe: `MigrateSQLiteToMySQL`, `WriteDotEnv`, la página de
> migración a MySQL (hoy `/settings/database` es respaldo de SQLite), el driver
> `go-sql-driver/mysql` y las variables `DB_HOST`/`DB_PORT`/… del `.env`.
> `migrate.go` (217 líneas) se eliminó al no tener ninguna llamada, y `go mod tidy`
> retiró el driver.
>
> **El resto de este documento se conserva como registro histórico** de lo que se
> hizo y de por qué. Para el estado vigente, ver `doc/HOJA-DE-RUTA.md`.

**Fecha:** 1 de octubre de 2026
**Estado:** HISTÓRICO — ver el aviso al principio. El soporte MySQL se retiró; SQLite quedó como único backend con una base por empresa.

---

## 1. Objetivo y decisiones

El negocio pequeño arranca con **solo ejecutar el binario** (SQLite, cero configuración:
se crea la BD y el usuario admin automáticamente). Cuando el negocio crece, se pasa a
MySQL desde **Ajustes → Base de Datos** (`/settings/database`): se configura la conexión,
se prueba, se migran TODOS los datos y el sistema escribe su propio `.env`; el próximo
arranque ya usa MySQL.

**Opinión implementada sobre el plan propuesto por el usuario:**

| Idea | Decisión |
|------|----------|
| SQLite para pequeño / MySQL para grande | ✅ Implementado (driver dual completo) |
| WAL para multiusuario | ✅ `journal_mode(WAL)` en el DSN (toda conexión) |
| "Cola en Go en memoria" | Se logra **sin código extra**: `BEGIN IMMEDIATE` + `busy_timeout(10s)` convierten a SQLite mismo en la cola FIFO de escritura; pool de 25 conexiones paralleliza lecturas. Un serializador en memoria Go duplicaría la lógica y añadiría riesgos (cola perdida al reiniciar, un solo proceso) |
| Borrar SQLite antes de cambiar a MySQL | ⚠️ Suavizado: el archivo **nunca se destruye**; se archiva como `.bak` (migración) y luego a `data/backup/erpgow.sqlite3-retired-<fecha>` (primer arranque MySQL). Es el historial contable del negocio: borrarlo sería indefendible |
| Crear su propio `.env` | ✅ `WriteDotEnv` preserva comentarios y claves existentes |

## 2. Cambios técnicos

### Concurrencia SQLite (`infrastructure/persistence/database.go`)
- DSN: `?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)`.
- Pool de 25 conexiones (igual que MySQL). Lecturas en paralelo; escrituras en cola
  (SQLite serializa; espera con retry, nunca falla por lock).
- **Bug crítico corregido:** con `MaxOpenConns(1)` (versión anterior), `tx.Begin()` tomaba
  la única conexión y el primer `s.db.Query` posterior del handler se colgaba para
  siempre → **la venta POS se congelaba**. Verificado: venta ahora responde en ~500 ms.
- `CheckpointSQLiteWAL()`: vacía el WAL antes de respaldar/archivar.

### Migraciones idempotentes para SQLite (`sqlite_convert.go`, ya existente, verificado)
- Los `migrations/*.sql` (sintaxis MySQL) se traducen al vuelo: PK de `ALTER TABLE` se
  pliega al `CREATE TABLE`, KEY→`CREATE INDEX`, AUTO_INCREMENT→`INTEGER PRIMARY KEY
  AUTOINCREMENT`, INSERT IGNORE→INSERT OR IGNORE, ENGINE/COMMENT/ON UPDATE eliminados.
- Arranque SQLite: **253 statements aplicados, 0 warnings** (los "duplicate column" se
  toleran por diseño idempotente en re-arranques).

### Fidelidad de datos al migrar (`infrastructure/persistence/migrate.go`)
- **Bug corregido:** el driver SQLite devuelve columnas DATE/DATETIME como `time.Time`
  en UTC (el texto almacenado no lleva zona) y go-sql-driver las re-escribía en hora
  local → fechas corridas un día (UTC-4). Ahora se re-formatea el wall-clock original
  (`2006-01-02 15:04:05`): fecha exacta preservada, verificado e2e.
- Copia por lotes de 500 filas; idempotente (INSERT IGNORE); solo tablas presentes en
  el esquema destino; reporte por tabla en la UI.

### Página `/settings/database` (`interfaces/web/database_settings.go` + plantilla)
- Corregido: conteo de tablas en MySQL (era `SELECT COUNT(*) FROM (SHOW TABLES)` — SQL
  inválido; ahora `information_schema.tables`), ruta del archivo SQLite visible, estado
  del motor actual, tamaño, versión de MySQL.
- Botones: **Probar conexión** (ping con timeout 5s) y **Migrar a MySQL** (flujo completo).
- URL directa integrada también como tarjeta en `/settings`.

### Arranque (`cmd/server/main.go`, `configs/config.go`)
- `DB_TYPE=sqlite|mysql` en `.env`. **Default inteligente:** sin `DB_TYPE`, una
  instalación con `.env` existente sigue en MySQL (compatibilidad); sin `.env` arranca
  en SQLite (cero-config). `DB_SQLITE_PATH` configurable (default `erpgow.sqlite3`).
- Al arrancar en MySQL con un SQLite sobrante: se archiva a `data/backup/` con fecha y
  se eliminan `-wal`/`-shm`. SQLite ejecuta migraciones + seed fiscal + admin en CADA
  arranque (idempotente); MySQL solo con `--install/--migrate/--seed` (igual que antes).

## 3. Pruebas e2e ejecutadas (reales, sobre SQLite)

| Prueba | Resultado |
|--------|-----------|
| Arranque sin `.env` ni BD | ✅ BD creada, 253 migraciones, admin admin/Admin@2026 |
| 34 páginas GET (POS, NC, caja, libros, COA, reportes, ajustes...) | ✅ todas 200 |
| Categoría / unidad / producto (barcode) / cliente (RIF) / proveedor | ✅ |
| Apertura de caja | ✅ |
| Venta POS efectivo 2×150.50+IVA → control `00-00000001` | ✅ 508 ms |
| Venta Zelle 100 → IGTF 3.09 cobrado, control `00-00000002` | ✅ |
| Reporte X, detalles de factura con fila IGTF | ✅ |
| Nota de crédito NC-00000001 (150.50, IVA 20.76) + listado | ✅ |
| Compra con N° control + RIF proveedor | ✅ |
| Migración a MySQL: **95 tablas, 248 filas** | ✅ |
| `.env` autoescrito (`DB_TYPE=mysql`, host/puerto/nombre/user/pass) | ✅ |
| Reinicio sin variables de entorno → usa MySQL automáticamente | ✅ |
| Continuidad: siguiente venta en MySQL → control `00-00000003` | ✅ |
| NC migrada visible en MySQL con fecha exacta 2026-10-01 | ✅ |
| `go build ./... && go vet ./... && go test ./...` | ✅ |

Binario de producción `erpgow.exe` reconstruido con todo esto.

## 4. Cómo usar

```bash
:: Negocio pequeño: solo ejecutar (crea erpgow.sqlite3 + admin)
erpgow.exe

:: Creció: Ajustes → Base de Datos → configurar MySQL → Probar → Migrar
:: (reinicia y el sistema arranca solo en MySQL; el archivo SQLite queda
::  archivado en data/backup/)
```

## 5. Benchmark de concurrencia (orquestador real en Go: `tools/loadtest`)

Simula N terminales/usuarios simultáneos: cada uno hace ráfagas de lecturas (POS,
listados, reportes) y una venta completa (transacción con asientos contables).
Los "usuarios" se pegan al servidor por HTTP real, no a la BD directa.

| Escenario | Resultado |
|-----------|-----------|
| **4 cajeros** (uso típico pequeño negocio): 800 lecturas + 40 ventas | 0 fallos · ventas a **47/seg** · venta promedio **12 ms** (p95 36 ms) · páginas 3 ms |
| **10 terminales** simultáneas: 500 lecturas + 100 ventas | 0 fallos · **84 ventas/seg** · venta promedio 58 ms (p95 137 ms) · páginas 4 ms |
| **25 terminales** (stress, ya es red local saturada): 300 lecturas + 100 ventas | 0 fallos · 61 ventas/seg · venta promedio 186 ms (p95 310 ms) · páginas 14 ms |
| Integridad: 240/240 ventas registradas, IDs consecutivos 1000→1239 | ✅ sin pérdidas ni duplicados |

Conclusión: el modelo WAL + BEGIN IMMEDIATE + busy_timeout ("la BD es la cola")
soporta con holgura el multiusuario de un negocio pequeño y medio; sigue fluido
hasta decenas de terminales y degrada gradual, sin errores ni pérdidas. Cuando el
negocio supere eso, se migra a MySQL desde /settings/database.

Para reproducir:

```bash
go run ./tools/loadtest   # 4 cajeros por defecto
CAJEROS=10 VENTAS=10 LECTURAS=5 go run ./tools/loadtest
```

## 6. Pendiente (fases siguientes)
1. Impresión térmica ESC/POS 80mm.
2. Multi-almacén real (transferencias, stock por bodega en POS).
3. CxC/CxP aging y conciliación.
4. Imprenta digital (Providencia SNAT/2024/000102).
5. Botón de anulación con NC en factura.
6. Retenciones IVA/ISLR auto-sugeridas en compras.

## 7. Auditoría (cadena hash SENIAT) — atención: blockchain global es un cuello de botella

### Problema identificado
El módulo de auditoría (`interfaces/web/audit_chain.go`) usa una **cadena hash SHA-256 encadenada estilo blockchain global** sobre la tabla `audit_log`. Cada fila enlaza con `prev_hash` del registro anterior y `record_hash` propio. Es tamper-evident: alterar/cualquier fila rompe la cadena.

**Demasiado bien hecho para su propio bien**: las operaciones de verificación y exportación hacen:

```sql
SELECT ... FROM audit_log ORDER BY id ASC   -- auditLogVerify: recorre TODO
SELECT ... FROM audit_log WHERE created_at BETWEEN ... ORDER BY id ASC   -- auditLogExport
```

Con **millones de registros** (típico de un negocio activo por meses/años), esto:
- **Colapsa la verificación de integridad**: auditar un mes implica recorrer toda la tabla.
- **Satura memoria**: carga todas las filas en memoria.
- **Bloquea el mutex global** (`auditChainMu`): `lastAuditHash()` hace un `ORDER BY id DESC LIMIT 1` dentro del lock en **cada auditoría** → cuello de botella en alta concurrencia (ej. retail multi-cajero).

No es un problema hoy, pero **crece de forma viral**: cuanto más usa el ERP, peor.

### Solución pragmica: bloques por período

Romper la cadena global en **bloques por mes/cierre** conserva la integridad criptográfica pero permite auditoría segmentada:

1. **Hash de bloque (`block_hash`)**: cada período (ej. mes) produce un `block_hash` = SHA256 de todos los `record_hash` del mes + `prev_block_hash`. La cadena de bloques es lineal y corta (12 bloques/año en vez de millones de filas).
2. **Verificación segmentada**: auditoría de un mes → validar **un solo bloque**.
3. **Pruning de hashes**: los `old_value/new_value` pesados se pueden archivar; los hashes de bloque son pequeños.
4. **Mutex por bloque**: en vez de `auditChainMu` global, un mutex por bloque/período. La concurrencia intra-bloque sigue serializada, pero inter-bloque es paralela.
5. **Índice parcial**: `idx_audit_period` en `(period, prev_hash)` acelera el escaneo de un bloque sin tocar el resto.

### Ventaja clave
Mantiene la **defendibilidad SENIAT** (hash anidado imposibilita alterar un asiento sin romper la cadena del mes) pero evita recorrer millones de filas para validar una operación de un día. La auditoría externa pedirá "el bloque de octubre" → se verifica en segundos, no en horas.

### Otras consideraciones virales detectadas
- **WAL growth**: con alta concurrencia, el `-wal` puede crecer. `CheckpointSQLiteWAL()` antes de backup lo vacía (ya implementado).
- **Tamaño de `audit_log`**: sin particionamiento/pruning, la tabla es el mayor crecimiento lineal. Los `old_value/new_value` JSON pueden ser grandes; se podría comprimir o externalizar a JSONB.
- **`ORDER BY id DESC LIMIT 1` en `lastAuditHash()`**: asume que `id` crece monótonamente. En MySQL con replicación async esto puede tardar; en SQLite es sólido.

### Estado
**Implementado (2 de octubre de 2026).** La cadena ya no es una sola cadena infinita: es una cadena de bloques mensuales enlazados.

| Pieza | Dónde |
|---|---|
| Tabla de sellos | `migrations/schema_z_audit_blocks.sql` (`audit_chain_blocks`) |
| Sellado y verificación | `interfaces/web/audit_chain.go` |
| Tests | `interfaces/web/audit_chain_test.go` |

Cómo quedó:

- **`audit_log` no se tocó.** Ni una columna, ni un índice, ni un tipo. La verdad criptográfica sigue siendo la cadena de filas, append-only. `audit_chain_blocks` es **derivada**: índice + sello. Si una fila de sellos se corrompe, la verificación lo detecta y lo reporta; no puede volver "bueno" a una cadena rota.
- **Bloques enlazados.** El primer registro de un mes encadena con el hash final del mes anterior, y el `block_hash` de cada mes cubre además el `block_hash` del mes anterior. Consecuencia: **borrar un mes entero también se detecta**, porque el mes siguiente ya no arranca en el hash que le corresponde.
- **Verificación acotada.** `GET /reports/audit-log/verify` valida **un bloque** (por defecto, el mes en curso) y ya no recorre la tabla entera. La verificación completa sigue existiendo, pero explícita: `?all=1`, con aviso de que es lenta.
- **Costo por escritura: O(1), igual que antes.** El barrido que sella los meses cerrados solo corre cuando cambia el mes en curso, no en cada auditoría.
- **El sello es inmutable.** Se escribe una vez por período y nunca se sobrescribe: recalcularlo y obtener otro valor *es* el hallazgo.
- La UI (`templates/audit_log.html`) ofrece selector de bloque, "Verificar Bloque" y un "Verificar Todo" aparte, con `confirm()`.

Pendiente de esta misma bitácora:

- El **mutex global `auditChainMu`** (riesgo 7 de `bitacora-auditoria-seguridad.md`) sigue siendo global: serializa el `lastAuditHash()` + INSERT de cada auditoría. Con bloques ya se puede acotar por período sin romper la cadena, pero es un cambio aparte, con sus pruebas de concurrencia.
- **Particionado/pruning de `audit_log`**: los `old_value/new_value` JSON siguen creciendo linealmente. El sellado no lo resuelve; solo lo hace verificable.

Nota de migración: `schema_z_audit_blocks.sql` va con prefijo `schema_z_` a propósito, porque las migraciones se aplican en **orden alfabético** y debe quedar después de `schema_extra.sql`, que es donde se crea `audit_log`.