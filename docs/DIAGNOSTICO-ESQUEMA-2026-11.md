# Diagnóstico del esquema real (Fase 0 del plan de esquema canónico)

Fecha: 2026-11-11
Medición: `go run ./tools/mediresquema` contra el MySQL de esta instalación
(`127.0.0.1:3306`), con las credenciales de `bin/.env` (el tool no las imprime).
El tool solo lee: no escribe nada en la base.

## 1. Bases medidas

| Base | Papel | Tablas | Columnas | Índices |
|---|---|---|---|---|
| `erpgow_001` | base principal (control, según `bin/.env`) | **220** | 2.272 | 522 |
| `erpgo_demo` | base de empresa (`erpgo_<código>`, `tenants.go:25`) | **221** | 2.270 | 520 |

Collation: **100 % `utf8mb4_unicode_ci`** en las 220/221 tablas de ambas bases.
La normalización de collation que corre en cada arranque
(`mysql_migrations.go:153`) dejó el esquema uniforme: el error 1267
("Illegal mix of collations") no aplica hoy.

## 2. Ledger `schema_migrations`

Ambas bases tienen **63 entradas**: los 62 archivos SQL + la pseudo-entrada
`zz_collation_utf8mb4` (checksum `v2`). Los checksums de cada archivo coinciden
entre bases, o sea que las dos migraron con la misma cadena y el mismo orden.

Cronología: la tanda inicial se aplicó el **2026-11-01** (23:22–23:23 en la
principal, 23:26 en `erpgo_demo`); luego `schema_ve_fiscal.sql` el
**2026-11-04** y `schema_zzzzz_servicio_consumo_almacenes.sql` junto con la
collation el **2026-11-06**. No hay archivos pendientes ni reintentos.

## 3. SQL declarado vs base real

| Concepto | Cifra |
|---|---|
| Tablas declaradas en `migrations/*.sql` (regex, sin comentarios) | **218** |
| `CREATE TABLE` repetidos (misma tabla en >1 archivo) | **130** |
| Declaradas y presentes en la base | **218 / 218** (ninguna ausente) |
| En la base pero NO declaradas en SQL | ver abajo |

**Ninguna tabla declarada falta en la base.** La cadena de 62 archivos
convergió exactamente al esquema declarado.

### Tablas en la base que no vienen de `migrations/*.sql` (creadas por código Go)

| Base | Tabla | Columnas | Quién la crea |
|---|---|---|---|
| `erpgow_001` | `addon_hook_out` | 11 | `interfaces/web/addon_hooks.go` |
| `erpgow_001` | `queue_tasks` | 10 | `internal/queue/queue.go` |
| `erpgo_demo` | `guia_tour` | 5 | `interfaces/web/guia_dialect.go` |
| `erpgo_demo` | `guia_paso` | 10 | `interfaces/web/guia_dialect.go` |
| `erpgo_demo` | `guia_progreso` | 6 | `interfaces/web/guia_dialect.go` |

Este es el hallazgo más importante de la medición: **las tablas creadas por
código Go no son las mismas en cada base** (2 en la principal, 3 en la empresa).
El esquema no vive solo en `migrations/` y varía según qué flujos tocaron cada
base. La Fase 4 del plan (moverlas al DDL canónico) queda justificada con
evidencia, no por intuición.

## 4. Discrepancia resuelta: "139" vs 220

El comentario de `infrastructure/persistence/testdb.go:74` ("plantilla
migrada (139 tablas)") estaba **obsoleto**: la base real tiene **220**. El
número 139 era anterior a la serie `schema_reconstruido_*` (2026-10-12).
El comentario quedó corregido con la cifra medida y su fecha.

Nota metodológica: la primera corrida del regex capturó dos "tablas" falsas
(`if`, `y`) provenientes de **comentarios** en los SQL
(`-- Idempotente: CREATE TABLE IF NOT EXISTS y ADD COLUMN...`); el tool ahora
salta las líneas de comentario y la comparación quedó limpia.

## 5. Conclusiones para el plan

1. **La cadena actual no está rota.** En esta instalación produjo exactamente
   el esquema declarado, con collation uniforme y ledger completo. El plan no
   es una reparación: es prevenir la deriva futura (los nombres `z…zzzzz` que
   solo ordenan) y eliminar la dependencia del orden de archivos.
2. **La base de empresa `erpgo_demo` existe** y debe entrar en la
   reconciliación (la Fase 2 la conecta también al alta de cada
   `erpgo_<código>`).
3. **La transición (Fase 3) tiene piso firme**: el ledger está completo y sus
   checksums coinciden entre bases; el sello `esquema_canonico_v1` puede
   apoyarse en él.
4. **El esquema canónico (Fase 1) debe nacer de las 218 tablas declaradas más
   las 5 de Go**, y las 130 definiciones repetidas se resuelven contra esta
   medición: aquí no hay pérdida, hay que elegir la definición correcta
   documentándola (ej.: `invoice` gana la de `reconstruido_03`, con columnas
   fiscales).

## 6. Lo que NO se midió

- Columna por columna (el cruce fue por nombres de tabla, no por DDL
  detallado); la Fase 1 hará ese cruce al construir el DDL canónico.
- El contenido de `backup/ERPGo.rar` (sigue sin herramienta RAR disponible).
- No se guardó un volcado completo de la base; la medición es de conteos y
  nombres, suficiente para la Fase 0.
