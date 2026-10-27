-- ---------------------------------------------------------------------------
-- Gobernanza de la IA: revisión humana persistida + idempotencia de herramientas.
--
-- POR QUÉ ESTE ARCHIVO EXISTE.
--
-- Los tres huecos del camino de la IA se cierran con DATOS, no con prompt:
--
--  1. La marca «requiere humano» se calculaba en Go y se perdía al guardar la
--     corrida, porque vivía en un campo sin exportar del journal. Aquí gana
--     columna, y gana también la fecha y el autor de la revisión: una marca que
--     nadie puede dar por vista es una marca que nadie mira.
--  2. Una tarea programada se reencolaba en cada vuelta de su hora. ai_run_slot
--     es la clave única que lo impide: una corrida por período.
--  3. Nada permitía saber si una escritura de la IA ya se había hecho. El libro
--     ai_tool_execution lo dice, con la misma idea que pos_sale_idempotency:
--     clave única reclamada de forma atómica antes de escribir.
--
-- Idempotente: CREATE TABLE IF NOT EXISTS y ADD COLUMN IF NOT EXISTS (el runner
-- quita la segunda para MySQL y tolera el error de «ya estaba»; ver
-- SinSintaxisDeMariaDB y esYaEstabaMySQL). Aplicarla dos veces no cambia nada.
--
-- ORDEN POR NOMBRE DE ARCHIVO: schema_z_ai_agents.sql (crea ai_task_run) <
-- schema_z_ai_governance.sql < schema_z_ai_operations.sql. Este archivo ALTERA
-- ai_task_run, así que no puede ordenar antes que quien la crea.
-- ---------------------------------------------------------------------------

-- 1. Revisión humana: la marca deja de evaporarse.
ALTER TABLE `ai_task_run` ADD COLUMN IF NOT EXISTS `needs_human` tinyint(1) NOT NULL DEFAULT 0;
ALTER TABLE `ai_task_run` ADD COLUMN IF NOT EXISTS `reviewed_by` varchar(50) NOT NULL DEFAULT '';
ALTER TABLE `ai_task_run` ADD COLUMN IF NOT EXISTS `reviewed_at` datetime DEFAULT NULL;

-- 2. Ranura de programación: una sola corrida por período.
--
-- Tabla aparte y no una columna con índice único: así la reclamación es un
-- INSERT IGNORE (atómico) y no depende de sintaxis de índice que cambia entre
-- MySQL y MariaDB. La fila se borra cuando la corrida FALLA, para que un error
-- del proveedor no deje la tarea diaria sin reintento hasta el día siguiente.
CREATE TABLE IF NOT EXISTS `ai_run_slot` (
  `slot_key` varchar(160) NOT NULL,
  `task_id` int NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`slot_key`),
  KEY `idx_ai_run_slot_task` (`task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. Libro de idempotencia de las herramientas de escritura.
--
-- Una fila por clave. `status`:
--   claimed -> reservada; NO se vuelve a ejecutar
--   done    -> ejecutada; si se repite se devuelve result_json, sin escribir
--   undone  -> compensada por «Revertir tarea»; la clave queda libre
--   error   -> falló la ejecución; no se repite (preferimos no duplicar)
--
-- `result_json` se guarda completo a propósito: cuando una acción se repite, el
-- modelo necesita el número de factura o de voucher para el informe final. Un
-- «ok» sin documento haría que el agente se inventara el número.
CREATE TABLE IF NOT EXISTS `ai_tool_execution` (
  `id` int NOT NULL AUTO_INCREMENT,
  `idem_key` varchar(160) NOT NULL,
  `tool` varchar(80) NOT NULL DEFAULT '',
  `agent_id` int NOT NULL DEFAULT 0,
  `run_id` int NOT NULL DEFAULT 0,
  `task_id` int NOT NULL DEFAULT 0,
  `external_ref` varchar(120) NOT NULL DEFAULT '',
  `status` varchar(12) NOT NULL DEFAULT 'claimed',
  `result_json` mediumtext,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_tool_idem` (`idem_key`),
  KEY `idx_ai_tool_run` (`run_id`),
  KEY `idx_ai_tool_ref` (`tool`, `external_ref`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
