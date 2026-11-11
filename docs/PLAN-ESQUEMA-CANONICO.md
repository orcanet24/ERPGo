# PLAN: Esquema canónico y cierre del legado PHP

Fecha: 2026-11-11
Estado: propuesto (sin ejecutar)
Alcance: infraestructura de esquema de base de datos + formalización de la
defunción del legado PHP. No incluye los huecos funcionales de auditoría
(parafiscales, reportes gráficos, backups automáticos), que van en otro plan.

---

## 1. Diagnóstico (por qué este plan existe)

Cifras medidas sobre el árbol actual:

| Dato | Valor | Evidencia |
|---|---|---|
| Archivos SQL en `migrations/` | 62 | listado |
| Sentencias `CREATE TABLE` | 353 para 220 tablas únicas | regex sobre los archivos |
| Tablas definidas en más de un archivo | 129 (2 definiciones; `menu` y `debt_collection` en 3) | regex |
| `schema.sql` | dump del PHP de 2013 ("Generated from SalesERP v12.13 install.sql"): 79 CREATE + 203 ALTER | cabecera del archivo |
| Serie `schema_reconstruido_01..10` | 132 tablas; existe porque "estas tablas faltaban en una instalación limpia" | cabeceras, fechado 2026-10-12 |
| Archivos nombrados con `z…` SOLO para ordenar alfabéticamente | toda la cadena `schema_z_zzzzz…` (más de 10) | cabeceras que lo explican: `schema_zz_vendor.sql:6-7`, `schema_zzz_ai_usage.sql:17-18` |
| Tablas creadas por código Go FUERA de `migrations/` | ~15: `tenants`, `tenant_users`, `web_sessions`, `guia_tour`, `guia_paso`, `guia_progreso`, `spidi_pago`, `ai_legal_rule`, `yummy_webhook_evento`, `addon_hook_out`, cola, orquestador | `tenants.go:453`, `guia_dialect.go:37`, `spidi.go:61`, `ai_legal.go:55`, `yummy.go:73`, `queue.go:162`, `orchestrator.go:277` |
| Comentario de tablas del esquema | "plantilla migrada (139 tablas)" en `testdb.go:74` vs 220 únicas en los archivos | discrepancia sin resolver contra una base real |

Defecto de raíz: **la verdad del esquema está repartida en tres generaciones
de archivos que se pisan más el código Go**. Instalar de cero y arrancar de
producción NO dieron lo mismo (eso produjo la serie reconstruido). El
registro `schema_migrations` y los runners funcionan bien (checksums,
reintento ante fallo, embebido en el binario); lo que está mal es el modelo:
una cadena de parches ordenada por nombre de archivo en vez de un esquema
declarativo que se reconcilie.

## 2. Decisiones de diseño

**D1 — Motores.** MySQL es el único motor de producción (el servidor ya
retira los `.sqlite3` heredados a `data/backup/`, `cmd/server/main.go:385`).
SQLite queda SOLO como motor de pruebas (`OpenSQLite` en toda la batería de
tests). Consecuencia: el esquema canónico se escribe una sola vez, en MySQL
(porque es el motor para el que se escribió), y el traductor a SQLite
(`sqlite_convert.go`) se mantiene para las pruebas — con un DDL limpio
(PK en línea, sin ALTERs históricos) ese traductor se puede simplificar
fuera: las dos pasadas de `collectMySQLPrimaryKeys` existen por las peculiaridades
del dump, no por MySQL.

**D2 — Fuente única de verdad.** Un DDL canónico consolidado (entregable de
la Fase 1) pasa a ser EL esquema. Las 62 migraciones se archivan y no se
vuelven a tocar. El DDL canónico incluye también las tablas que hoy crea el
código Go (Fase 4) y las semillas estructurales (menú, permisos, catálogo de
cuentas) con sus INSERT idempotentes.

**D3 — Reconciliación al arranque, no reproducción de parches.** El arranque
compara el esquema deseado (canónico) contra el esquema real
(`information_schema` en MySQL) y aplica solo el diff. Instalar de cero y
arrancar una base vieja convergen al mismo estado, siempre. El registro
`schema_migrations` NO se descarta: sobrevive una fase como mecanismo de
sello de la transición (Fase 3) y luego se jubila.

**D4 — Lo que ya vive bien en Go, se queda en Go, pero se declara.** Los
triggers de protección de borrado (`proteccion_borrado.go`), la
normalización de collation (`mysql_migrations.go:153`), el ledger y las
semillas por handlers (`semilla_carniceria.go`, `InitializeAdminUser`)
no vuelven al SQL: los triggers no parten bien por `;\n` y la collation es
mantenimiento del motor. Se declaran en el catálogo del reconciliador para
que su estado también se verifique al arranque.

**D5 — El legado PHP no se renombra.** Nombres de tablas (`acc_vaucher`,
`VNo`), convenciones de permisos (`new_invoice`, `hrm_management`), `md5compat`
y los 444 assets de `static/` se CONSERVAN tal cual. Renombrar es riesgo sin
valor fiscal. El "cierre del legado" es formal: quitar la referencia a una
carpeta PHP que ya no existe y documentar qué se conservó y por qué.

## 3. Fases de ejecución

Cada fase se cierra con: código + pruebas al lado, `go build ./...`,
`go vet ./interfaces/web`, `go test ./...` en verde, commit descriptivo y push.

### Fase 0 — Medir el estado real (día 1)

Objetivo: resolver hoy las dos incógnitas que el plan NO debe arrastrar.

1. Sobre una base MySQL real (la de la empresa instalada y una limpia de
   referencia): contar tablas y columnas efectivas tras aplicar las 62
   migraciones; comparar contra las 220 únicas declaradas.
2. Volcar el contenido real de `schema_migrations` de cada base instalada
   (es el insumo de la Fase 3).
3. Corregir el comentario de `testdb.go:74` y `plantilla.go` con la cifra real.

Entregable: `docs/DIAGNOSTICO-ESQUEMA-2026-11.md` con las cifras.
Prueba: el volcado se hace con `mysqldump --no-data` + script de conteo;
nada toca el código.

### Fase 1 — Esquema canónico consolidado (días 2-5)

Objetivo: 62 archivos → 1 DDL canónico por motor.

1. Escribir `tools/esquemacanonico/main.go`: generador que lee las 62
   migraciones y produce el DDL consolidado, con reglas de resolución
   explícitas y registradas por tabla:
   - Si una tabla tiene varias definiciones, gana la de la generación más
     reciente que la base real haya aceptado (validar contra el volcado de
     la Fase 0). Ejemplo conocido: `invoice` gana la de
     `schema_reconstruido_03_sales.sql` (trae las columnas fiscales
     `control_no`, `igtf_amount`, etc.).
   - Los `ALTER TABLE ADD COLUMN IF NOT EXISTS` de los archivos `z` se
     aplican sobre la definición ganadora y desaparecen como sentencias.
   - Los INSERT de semillas (menú, permisos, catálogo de cuentas, tasas)
     se conservan idempotentes (`ON DUPLICATE KEY UPDATE` ya usado en
     `schema_z_correlativos.sql` y la guía).
2. Revisar el resultado A MANO tabla por tabla contra el volcado de la
   Fase 0: 220 tablas es auditable en una sesión.
3. Entregables: `migrations/esquema_canonico_mysql.sql` y
   `migrations/esquema_canonico_sqlite.sql` (o uno solo + traducción en
   Go, decidir según lo que pida el traductor al simplificarlo).

Criterio de aceptación: una base limpia creada SOLO con el DDL canónico pasa
la batería completa de `go test ./...` (que hoy arranca sobre la plantilla
migrada) y el conteo de la Fase 0 coincide tablas y columnas.

### Fase 2 — Reconciliador al arranque (días 6-10)

Objetivo: sustituir "aplicar 62 archivos en orden alfabético" por
"deseado vs real, aplicar el diff".

1. `infrastructure/persistence/reconciliador.go`:
   - Lee el DDL canónico (embebido, `embed.go` ya embebe `migrations/`).
   - Introspección: `information_schema.TABLES/COLUMNS/KEYS` en MySQL;
     en SQLite, `sqlite_master` + `PRAGMA table_info` tras la traducción.
   - Aplica: `CREATE TABLE` si falta; `ADD COLUMN` si falta columna; índices
     y FK que falten. NUNCA destruye: ni `DROP` ni `MODIFY` que recorte
     datos — desviaciones incompatibles se reportan en el log como aviso
     con el detalle, siguiendo la regla del proyecto (avisar, no frenar).
   - Verifica el estado de los triggers de protección y de la collation
     (D4) y los reaplica si faltan.
2. Conectarlo en el MISMO punto donde hoy corre `RunMigrationsForMySQL`
   y `RunMigrations` (arranque, alta de empresa, asistente de instalación):
   todas las rutas por donde pasa una base (`mysql_migrations.go:94` es la
   referencia) deben pasar por el reconciliador, incluida cada base
   `erpgo_<código>` al crearse (`tenants.go`).
3. Pruebas al lado (patrón del repo): base con esquema de la producción
   volcado de la Fase 0 → debe reconciliarse sin crear NADA; base limpia →
   debe quedar idéntica a la canónica; base con una tabla a la que le falta
   una columna → la recupera. Las tres en MySQL (vía `testdb.go`) y las dos
   últimas en SQLite.

Criterio de aceptación: los tests del punto 3 en verde, y el arranque de una
base instalada NO re-aplica ni una sentencia que ya está (log limpio).

### Fase 3 — Transición de las bases instaladas (días 11-13)

Objetivo: tu empresa ya instalada converge sin pérdida y sin drama.

1. Previo obligatorio: backup de la BD (el proyecto ya lo hace a `backup/`;
   documentarlo como paso del propio arranque de transición).
2. El reconciliador de la Fase 2 ya es la transición: sobre una base vieja
   el diff con el canónico debe ser cero o casi cero (las desviaciones que
   salgan son justamente los ALTERs históricos manuales que nunca llegaron a
   `migrations/`, el caso documentado en `schema_z_zzz_recuperadas.sql`).
   Cada desviación se revisa a mano ANTES de aceptarla: o entra al canónico,
   o era basura del PHP y se declara obsoleta.
3. Sello: el reconciliador anota en `schema_migrations` una entrada
   `esquema_canonico_v1` con checksum del DDL (mismo mecanismo de siempre,
   `mysql_migrations.go:220`), para que se pueda saber en campo qué base
   está ya en el modelo nuevo.
4. Validación con la simulación: correr la simulación de
   `GUIA_SIMULACION_1_ANO_FISCAL.md` sobre una base reconciliada y una limpia
   y comparar los cuadros de mando (F.1 de la guía).

Criterio de aceptación: base de producción reconciliada + simulación cuadrada
(`Descuadre < 0.01`, `LibroVacio = false`).

### Fase 4 — El código Go deja de crear esquema por su cuenta (días 14-16)

Objetivo: una sola fuente de verdad, también para el código.

1. Mover a los DDL canónicos las tablas que hoy crean `tenants.go`,
   `guia_dialect.go`, `spidi.go`, `ai_legal.go`, `yummy.go`,
   `addon_hooks.go`, `queue.go`, `orchestrator.go`, conservando sus
   `CREATE TABLE IF NOT EXISTS` como característica de tolerancia, no como
   fuente: el reconciliador las trae, los archivos solo aguantan arrancar
   sin él.
2. Prueba: borrar una de esas tablas de la base de prueba y arrancar →
   el reconciliador la trae de vuelta.

### Fase 5 — Jubilar la cadena de migraciones (días 17-18)

1. Mover los 62 archivos a `migrations/legacy/` (el `go:embed` sigue
   funcionando con subcarpetas; el runner solo lee `.sql` del nivel raíz —
   verificar y, si hace falta, filtrar por prefijo).
2. El runner clásico queda SOLO para bases anteriores al sello de la Fase 3
   (si el ledger no tiene `esquema_canonico_v1`, se corre una última vez la
   cadena vieja y luego el reconciliador). Después de un ciclo sin incidentes,
   la cadena vieja se puede borrar del árbol.
3. Simplificación de `sqlite_convert.go` ya con DDL limpio (PK en línea):
   fuera las dos pasadas de `collectMySQLPrimaryKeys`. Con prueba.

### Fase 6 — Cierre formal del legado PHP (día 19)

1. `cmd/docgen/scan.go:132`: quitar la marca `php_original` de una ruta que
   ya no existe; documentar en el docgen qué es `static/` y que se conserva
   a propósito.
2. Documento `docs/HERENCIA-PHP.md` (corto): qué quedó del PHP (esquema
   original, `md5compat.go` para contraseñas viejas, motor contable clonado
   en `accounting.go`, nomenclatura de permisos, assets de `static/`), qué se
   retiró a propósito (POS web, `/synchronizer`, menú Addons — ya está
   documentado en `schema_z_zzzzz_menu_limpieza.sql:24-33`) y la regla de que
   nada nuevo hereda nombres del PHP.
3. El archivo `migrations/schema.sql` (el dump del PHP) queda archivado en
   `legacy/` con su lugar histórico anotado.

## 4. Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| El DDL consolidado elige mal entre definiciones en pugna | Regla de resolución validada contra el volcado real de la Fase 0; revisión a mano de las 129 tablas duplicadas |
| Desviaciones históricas en bases instaladas (ALTERs manuales que nunca entraron a `migrations/`) | El reconciliador es introspectivo: el diff se revisa a mano en la Fase 3 antes de aceptar; nada se destruye |
| Los triggers y la collation dependen de privilegios del usuario MySQL | Comportamiento ya resuelto hoy: aviso en el log y arranque sigue (`mysql_migrations.go:90-97`); el reconciliador conserva esa política |
| Los INSERT de semillas duplican catálogos si se re-aplican | El ledger ya evita re-ejecutar archivos con el mismo checksum (`database.go:204-207`); en el modelo nuevo, las semillas van con `ON DUPLICATE KEY UPDATE` y el sello |
| Las pruebas dependen de la plantilla migrada de `plantilla.go` | En la Fase 5 la plantilla se regenera desde el canónico; hasta entonces las pruebas siguen corriendo contra la cadena vieja (no se toca nada de un tirón) |
| Romper el arranque de una base vieja | Orden de fases: el reconciliador entra ANTES de jubilar la cadena; la cadena queda disponible como red de seguridad durante todo un ciclo |

## 5. Qué NO hace este plan

- No renombra tablas ni columnas de la era PHP (D5).
- No toca los assets de `static/` ni los retira.
- No implementa parafiscales, reportes gráficos ni backups automáticos:
  van en su propio plan (son huecos funcionales, no de esquema).
- No decide sobre la IA-generación de addons ni integraciones externas.

## 6. Esfuerzo relativo y orden

F0 (1 día) → F1 (4) → F2 (5) → F3 (3) → F4 (3) → F5 (2) → F6 (1).
Total estimado: ~19 días de trabajo concentrado. Las fases 2 y 3 son las
críticas: ahí se prueba con bases reales. Ninguna fase rompe lo anterior:
cada una deja el árbol en verde y commiteado.
