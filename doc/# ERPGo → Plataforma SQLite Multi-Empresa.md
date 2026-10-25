# ERPGo → Plataforma SQLite Multi-Empresa

## Decisiones cerradas (de esta conversación)

| Decisión | Valor | Motivo |
|---|---|---|
| Base de datos | **SQLite, 100%** | PyME y Enterprise alike. Se elimina el código MySQL |
| Multi-empresa | **Una BD SQLite por empresa** | Aislamiento físico total. Una fuga entre empresas es inaceptable en nómina |
| Respaldo | Copia de archivo + `CheckpointSQLiteWAL()` | Litestream solo cuando un cliente lo ponga en servidor |
| Reportes/analítica | **DuckDB** (opcional, lee el mismo `.sqlite` en solo-lectura) | No es una base alternativa: es un acelerador analítico |
| Cola | Motor propio en Go, **con límite de RAM** | El `time.Tick` actual no es una cola |
| Runtime | Un solo binario, sin PBX, sin build | Distribución gratuita |

---

## Fase 0 — Cimientos: SQLite puro y multi-empresa

### 0.1 Eliminar MySQL

Hoy conviven dos motores con ramificación dispersa. Se va el MySQL.

- Reescribir [dialect.go](interfaces/web/dialect.go): eliminar `isSQLite()` y sus 12 helpers condicionales. El SQL se escribe una vez, en dialecto SQLite.
- Borrar de [database.go](infrastructure/persistence/database.go) el camino MySQL: `InitializeDatabase`, `transformStatement`, la rama `dialect == "mysql"` de `applyStatementsFor`, y `MigrateSQLiteToMySQL`.
- Simplificar [sqlite_convert.go](infrastructure/persistence/sqlite_convert.go): sin conversor, las migraciones se aplican nativas. Se conserva y se **arregla** el fix reciente de PK inline.
- Reescribir [config.go](configs/config.go): `DB_TYPE` desaparece. Quedan `DB_SQLITE_PATH` y rutas.
- **Borrar los ~100 `ALTER TABLE ADD COLUMN` de deduplicación** de [schema_extra.sql](migrations/schema_extra.sql) y friends: SQLite con `CREATE TABLE IF NOT EXISTS` ya es idempotente. El schema se consolida en un archivo limpio y legible.

### 0.2 Registro de empresas (tenant registry)

`company_information` es hoy una tabla de una sola fila. Se convierte en el catálogo.

**Nueva BD de control** (`erpgo.db`, junto al binario): `tenants`, `tenant_users`, `tenant_provision_log`. Es la única BD que ve más de una empresa, y no contiene datos contables.

- `tenants`: `code` (slug, ej. `acme`), `legal_name`, `rif`, `status`, `created_at`. El **`code` es el nombre de archivo** de la BD.
- Login → si el usuario tiene 1 empresa, entra directo; si tiene varias, selector. Sesión lleva `tenant_code`.
- Cambio de empresa desde el panel: rota el session ID, deja rastro en la cadena de auditoría de la **empresa destino** (importante para demostrar qué vio cada usuario).
- El usuario no puede cambiar de empresa manipulando la URL: se valida contra `tenant_users`.

### 0.3 Aprovisionamiento

`/setup` crea empresas: reserva directorio, copia el schema base, corre migraciones, siembra año fiscal + menús + admin inicial. Idempotente y **auditado** (crear una empresa es un evento de seguridad).

### 0.4 Sesiones persistentes

Hoy son **en memoria** ([session.go](interfaces/web/session.go)): no sobreviven reinicios ni multi-instancia. Pasan a SQLite con `tenant_code`. Esto es requisito de la Fase 0, no opcional — sin esto, multi-empresa no funciona.

---

## Fase 1 — Cola y workers (límite de RAM)

El requisito es explícito: nada de "sistema que se come la RAM".

**`internal/queue`** — motor propio, cuatro garantías:

1. **Pool fijo de workers** (por defecto 2, configurable). Nunca "un goroutine por tarea".
2. **Cola acotada** con backpressure: si se llena, el productor espera o rechaza. Nunca crece sin límite.
3. **Spill a disco**: las tareas encoladas se persisten en SQLite, no viven solo en RAM. Al reiniciar, la cola se reconstruye.
4. **Cotas de memoria en cada tarea**: `ai_max_rows` (ya existe, 500) y `max_tokens` se respetan en la consulta, no solo en el prompt. Una tarea no puede traer 2M de filas a memoria.

Elimina el `time.Tick` de [ai_tasks.go](interfaces/web/ai_tasks.go). El scheduler se apoya en la cola: cada 5 min encola lo vencido, la cola decide ejecución.

---

## Fase 2 — RRHH (motor de nómina con parafiscalidad)

Es la fase de mayor riesgo legal, por eso las tasas son **datos versionados, nunca constantes en código**.

### 2.1 Principio: las tasas son datos

`tax_rate_table` con `concept_code`, `rate_pct`, `effective_from`, `effective_to`, `legal_citation`, `applies_to`. Las cifras vigentes (IVSS obrero/patronal, INCEPS, FAOV, BPSI, IVA retenido, anticipo ISLR) se siembran con su cita legal y vigencia. Si la ley cambia, se **agrega una fila nueva**; los recibos de nómina antiguos siguen siendo correctos porque saben qué tasa aplicaba ese día.

> Este diseño conecta directo con la cadena de auditoría por bloques: un recibo de nómina guarda el hash de sus insumos y la versión de la tabla de tasas. Si mañana el DDS cambia las cifras, se puede **demostrar** que en marzo se aplicó la vigente en marzo.

### 2.2 Alcance

- Ficha de empleado, contratos, cargos, departments, bank's de pago.
- Asistencias, permisos, vacaciones, ausencias.
- Estructura salarial: asignación fija, por hora, por comisión, bonos.
- **Liquidación quincenal/mensual**: ingresos → descuentos → neto.
- **Retenciones**: ISLR al trabajador (tabla por tramos, no un porcentaje plano), IVSS obrero, anticipation ISLR.
- **Cargas patronales**: IVSS patronal, INCEPS, FAOV, BPSI.
- Recibo de nómina imprimible con el desglose fiscal.
- **Reportes de declaración**: retención de ISLR, IVA retenido, FAE, y el consolidado de 2ª quincena, listos para exportar.
- Cada liquidación es **reproducible y auditable**, con reversión por nota de ajuste (nunca DELETE).

### 2.3 El módulo reutiliza las tablas heredadas

`employee`, `employee_salary_setup`, `employee_salary_payment`, `employee_history`, `payroll_tax_setup` ya están en el esquema. Se **revisan y completan** en vez de inventar un modelo nuevo, pero el motor de cálculo es nuevo.

---

## Fase 3 — CRM

Construcción completa desde cero (no existe nada).

- Contactos y empresas (pipeline comercial), con **enlace a `customer_information`** del ERP: el cliente que compra es el mismo que ve CRM. Sin duplicar.
- Embudo (pipeline) con etapas configurables y **probabilidad de cierre**.
- Tareas y seguimientos, con recordatorios (usa la cola de la Fase 1).
- Cotizaciones: `quotation` ya existe en el ERP; se conecta.
- Historial de interacciones y notas.
- Emails y recordatorios de vencimiento.
- Vista 360°: cliente → facturas, pagos, saldo, cotizaciones, contacto.

---

## Fase 4 — Contabilidad: robustez y cierre

No se reconstruye (Fase 0 confirmó que está sólida). Se completa:

- Cierre de período con **validaciones de blocage** (asientos descuadrados, cuentas sin movimiento, IVA retenido sin enterar).
-.libros legales**:RIB y libros de ventas ya están ([legal_books.go](interfaces/web/legal_books.go)); se valida contra lo que exige la providencia vigente.
- Reportes financieros: balance, estado de resultados, posición, flujo de caja.
- Catálogo COA con validación de cuentas y libros de mayor/menor.
- **Auditoría por bloques mensuales ya implementada** en la iteración anterior: se le añade el archivado de meses sellados a Parquet, para que el SQLite caliente no crezca sin límite.

---

## Fase 5 — Motor de reportes y plantillas

- **Reportes por plantilla** (JSON en BD, como pediste): definición de columnas, filtros, agrupación, fórmula de totales. Guardados, versionados, reutilizables.
- Un humano crea reportes sin depender de un técnico: es la razón de ser de tu nicho.
- Generación con **DuckDB** cuando el conjunto sea grande (lee el mismo `.sqlite` en solo-lectura, sin copiar). Detección automática: DuckDB si la consulta toca >500k filas, SQLite si no.
- Exportación a PDF/Excel/CSV.

---

## Fase 6 — IA como empleado supervisado (MCP + gobierno)

### 6.1 La clave: las reglas viven en el binario

Tu intuición es correcta y es lo que hace esto seguro:

- **Nivel 1 — Código Go, no negociable.** Un admin **no puede**(relajar esto, porque no está en el prompt:
  - SQL guardián: solo SELECT, blacklist de tablas sensibles (`user_login`, `ai_providers`, `users`), sin DML/DDL.
  - **Anti-fraude y anti-delito fiscal**: prohibido generar asientos que oculten ingresos, splitting para cruzar umbrales de IVA, redondeos engineered, retroactive-numbers, borrado de facturas (solo anulación con NC).
  - **Integridad referencial y de auditoría**: la IA no puede escribir en `audit_log` ni alterar un bloque sellado.
  - Toda acción de IA queda auditada y es **reversible**.
  - El mapa del esquema es de **uso exclusivo del agente**: nunca se cita al usuario (ya existe en [ai_rules.go](interfaces/web/ai_rules.go)).
- **Nivel 2 — Configurable por el administrador.** Qué agente hace, con qué límites, a qué rol pertenece. El admin puede **endurecer**, nunca aflojar el Nivel 1.
- **Nivel 3 — El MCP.** Herramientas que la IA usa de verdad:
  - `describe_schema` → el mapa relacional (qué es la entidad, cómo se relate).
  - `list_functions_by_role` → qué puede hacer cada departamento/rol.
  - `query_report` → pide un reporte de plantilla (el atajo que resuelve tu caso de uso).
  - `propose_transaction` / `propose_payroll` / `propose_invoice` → **sugiere**, nunca aplica directo.
  - `legal_summary` → **resúmenes de las leyesutdownloaded en el MCP**, para que la IA sepa hasta dónde puede llegar. (Este es el punto que pediste y es lo que hace la IA un empleado *competente* y no uno que improvisa.)

### 6.2 Niveles de riesgo (el punto de la F1)

Nada de frenar la F1, nada de dejar que rompa la empresa:

| Riesgo | Ejemplos | Quién ejecuta |
|---|---|---|
| **Lectura** | Consultas, reportes, resúmenes | La IA sola |
| **Borrador** | Reportes nuevos, análisis, propuesta de asiento | La IA sola + **alerta inmediata** al usuario |
| **Ejecución con validación** | Asiento contable, factura, cliente | La IA aplica, con validaciones duras en código y **alerta** |
| **Crítico** | Nómina, pagos, anulaciones | **Siempre aprobación humana** |

El criterio no es "que tan inteligente es la IA", es **qué tan reversible es el error**. Por eso nómina y pagos son los únicos que exigen humano: un error ahí no se deshace con un `undo`, se deshace con un expediente.

### 6.3 Tareas por plantilla

`ai_task` (ya existe) se convierte en el motor: plantillas de trabajo con prompt específico, rol, límites, horario, y las alertas. Se apoya en la cola de la Fase 1.

---

## Orden de ejecución

1. **Fase 0** — SQLite puro + multi-empresa + sesiones. Sin esto nada más tiene sentido.
2. **Fase 1** — Cola con límite de RAM. Antes de que las tareas IA se multiplicen.
3. **Fase 2** — RRHH (la más larga y la de mayor riesgo legal).
4. **Fase 6 parcial** — Gobierno de IA (nivel 1 en código). Antes de que la IA toque datos reales.
5. **Fase 3** — CRM.
6. **Fase 4** — Contabilidad.
7. **Fase 5** — Reportes + DuckDB.
8. **Fase 6 resto** — MCP completo + niveles de riesgo.

## Garantías

- Cada fase entra con `go build` + `go test ./...` en verde y **sale igual**. Baseline ya verificado.
- La cadena de auditoría por bloques mensuales se mantiene íntegra en todas las fases; ninguna toca `audit_log` a la escritura.
- Ninguna tasa fiscal se escribe en código: todas en `tax_rate_table` con vigencia y cita legal.
- Ninguna acción de IA es irreversible sin pasar por el Nivel 1 en Go.

## Nota de alcance

Es un plan de ~6 fases, no un cambio puntual. Las Fases 0-1 son la base técnica y no dependen del negocio. La Fase 2 (RRHH) es la más grande en superficie. Si quieres, se puede entregar por fases independientes en lugar de todo de una vez.