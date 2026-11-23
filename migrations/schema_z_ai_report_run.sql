-- ---------------------------------------------------------------------------
-- Corrida auditable de informes + propuestas de aprobación previa (IA/informes).
--
-- POR QUÉ ESTE ARCHIVO, Y POR QUÉ ES UNO SOLO.
--
-- El cierre de la auditoría de 2026-11-22 necesita DOS tablas nuevas y ninguna de
-- las dos existía:
--
--  1. `report_run` (F9 — informes no reproducibles). Hasta ahora ejecutar un
--     informe personalizado no dejaba rastro: ni qué definición corría, ni con qué
--     parámetros, ni cuántas filas salió, ni quién lo pidió. Sin eso es imposible
--     responder «qué cifras vio esta persona este mes» ante una diferencia, y es
--     imposible reproducir un informe después de que alguien edite la plantilla. La
--     columna `definition_hash` existe precisamente para eso: si el número no se
--     reproduce, la huella dice si la definición ya había cambiado.
--
--  2. `ai_tool_proposal` (F5 — aprobación humana POSTERIOR al efecto). Una acción
--     de riesgo crítico (cobro, asiento, aviso a un tercero) se registraba como
--     «necesita revisión» DESPUÉS de ejecutarse. Ahora se guarda aquí con sus
--     argumentos exactos y estado 'pending', y NO se ejecuta hasta que un humano la
--     aplica desde la pantalla de revisión de la corrida. La columna `idem_key` es
--     la clave única: la misma llamada del mismo ámbito produce UNA sola propuesta
--     (reintento de la cola, segunda vuelta del ticker), y el paso 'pending' →
--     'applying' es el reclamo atómico que impide que dos clics generen dos asientos.
--
-- Las dos son de la capa IA/informes, comparten el mismo cierre y el integrador
-- regenera el canónico a partir de aquí, así que van en un solo archivo nuevo.
--
-- NO TOCA ESQUEMA AJENO: no altera report_template, ni ai_task_run, ni ninguna
-- tabla del canónico. CREATE TABLE IF NOT EXISTS: aplicarla dos veces no cambia
-- nada, y una instalación existente no pierde filas.
--
-- ORDEN POR NOMBRE DE ARCHIVO: debe ir DESPUÉS de las migraciones que crean
-- `report_template` y `ai_agent`/`ai_task_run` (referenciadas por valor, sin
-- clave foránea, para que una base con datos antiguos no falle al aplicar):
--   schema_z_ai_agents.sql < schema_z_ai_governance.sql < este archivo.
-- ---------------------------------------------------------------------------

-- 1. Corrida de informe (F9).
CREATE TABLE IF NOT EXISTS `report_run` (
  `id` int NOT NULL AUTO_INCREMENT,
  `report_id` int NOT NULL DEFAULT 0,
  `definition_hash` text,
  `sql` mediumtext,
  `params` text,
  `rows_count` int NOT NULL DEFAULT 0,
  `run_by` varchar(60) NOT NULL DEFAULT '',
  `agent_id` int NULL DEFAULT NULL,
  `tenant_code` varchar(60) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL,
  `exported_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_report_run_report` (`report_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2. Propuesta de acción crítica pendiente de aprobación humana (F5).
--
-- Estados de `status`:
--   pending  -> registrada por el dispatcher; NADA se ejecutó todavía
--   applying -> reclamada por un humano (claim antes del efecto)
--   applied  -> ejecutada por ese humano; `result_json` trae el documento
--   rejected -> la retiró un administrador sin ejecutarla
--   failed   -> el efecto falló; queda el error para que se vea, no se repite solo
--
-- `args_json` es lo que el modelo pidió, literalmente: así lo que se ejecuta al
-- aprobar no es «lo que el humano recuerda que se pidió» sino el argumento que
-- pasó el guardián. Se revalida al aplicar (permiso y riesgo) — ver
-- aiAplicarPropuesta.
CREATE TABLE IF NOT EXISTS `ai_tool_proposal` (
  `id` int NOT NULL AUTO_INCREMENT,
  `idem_key` varchar(160) NOT NULL,
  `tool` varchar(80) NOT NULL DEFAULT '',
  `args_json` mediumtext,
  `run_id` int NOT NULL DEFAULT 0,
  `task_id` int NOT NULL DEFAULT 0,
  `agent_id` int NOT NULL DEFAULT 0,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `reason` text,
  `result_json` mediumtext,
  `requested_by` varchar(60) NOT NULL DEFAULT '',
  `applied_by` varchar(60) NOT NULL DEFAULT '',
  `applied_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_proposal_idem` (`idem_key`),
  KEY `idx_ai_proposal_run` (`run_id`, `status`),
  KEY `idx_ai_proposal_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. El candado no sirve si nadie lo ve: la corrida que tiene propuestas pendientes
--    necesita estar marcada, y `ai_task_run.needs_human` ya lo hace (migración
--    schema_z_ai_governance.sql). No se añade columna nueva aquí: se reutiliza la
--    que existe para que la pantalla de «pendiente de revisión» sea UNA sola.
