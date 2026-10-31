-- ---------------------------------------------------------------------------
-- Agentes IA: plantillas de informes + operador autónomo auditable.
--
-- Convención del proyecto (ver schema_z_ai.sql): este archivo debe ordenar
-- DESPUÉS de schema.sql / schema_extra.sql / schema_menu_permissions.sql /
-- schema_z_ai.sql PERO ANTES de schema_z_ai_providers.sql (que crea
-- `ai_providers`, referenciada por el ALTER de más abajo):
--   "schema_z_ai.sql" < "schema_z_ai_agents.sql" < "schema_z_ai_providers.sql"
-- es incorrecto; el orden correcto por ordenación de nombres es
--   schema_z_ai_agents.sql < schema_z_ai_providers.sql
-- así que aquí NO puede alterarse ai_providers. La columna `vendor` se añade
-- al final de ESTE archivo vía una tabla puente y un ALTER en
-- schema_z2_vendor.sql (que sí ordena después de schema_z_ai_providers.sql).
--
-- Seguridad: las credenciales NUNCA viven aquí. ai_agent referencia a
-- user_login (identidad con rol RBAC normal) y a ai_providers (vendor
-- whitelist compilado en el binario: openai | anthropic | orc24).
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `ai_agent` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(80) NOT NULL,
  `user_id` varchar(50) NOT NULL,
  `provider_id` int NOT NULL,
  `model_override` varchar(120) NOT NULL DEFAULT '',
  `role_id` int NOT NULL,
  `operator_enabled` tinyint(1) NOT NULL DEFAULT 0,
  `max_steps` int NOT NULL DEFAULT 8,
  `max_rows` int NOT NULL DEFAULT 500,
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `created_by` varchar(50) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `department` varchar(10) NOT NULL DEFAULT '',
  `config_prompt` mediumtext NULL,
  `config_prompt_at` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_agent_user` (`user_id`),
  KEY `idx_agent_department` (`department`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_task` (
  `id` int NOT NULL AUTO_INCREMENT,
  `agent_id` int NOT NULL,
  `name` varchar(150) NOT NULL,
  `instructions` text NOT NULL,
  `schedule_type` varchar(10) NOT NULL DEFAULT 'manual',
  `schedule_value` varchar(20) NOT NULL DEFAULT '',
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `created_by` varchar(50) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_task_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_task_run` (
  `id` int NOT NULL AUTO_INCREMENT,
  `task_id` int NOT NULL,
  `agent_id` int NOT NULL,
  `status` varchar(10) NOT NULL DEFAULT 'running',
  `started_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `finished_at` datetime DEFAULT NULL,
  `steps` mediumtext,
  `final_report` mediumtext,
  `undo_journal` text,
  PRIMARY KEY (`id`),
  KEY `idx_run_task` (`task_id`),
  KEY `idx_run_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `report_template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) NOT NULL,
  `description` varchar(250) NOT NULL DEFAULT '',
  `definition` mediumtext NOT NULL,
  `created_by` varchar(60) NOT NULL DEFAULT '',
  `status` tinyint(1) NOT NULL DEFAULT 1,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Visibilidad por informe: un reporte puede restringirse a roles/usuarios.
-- Regla: SIN filas en report_template_acl → visible para TODOS (compatibilidad
-- con los reportes pre-existentes). CON filas → el usuario ve el reporte si
-- alguna fila coincide con su role_id o user_id; si todos los valores son NULL
-- la fila significa "todo el mundo" (equivalente a no tener filas).
CREATE TABLE IF NOT EXISTS `report_template_acl` (
  `id` int NOT NULL AUTO_INCREMENT,
  `report_template_id` int NOT NULL,
  `role_id` int NULL DEFAULT NULL,
  `user_id` int NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_rt_acl_report` (`report_template_id`),
  KEY `idx_rt_acl_role` (`role_id`),
  KEY `idx_rt_acl_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;



-- Menú (mismo patrón que schema_z_ai.sql; module 'ai' ya existe allí).
INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM (SELECT `id` FROM `module` WHERE `name` = 'ai' ORDER BY `id` DESC LIMIT 1) `m`),
	'ai_agents', 'ai_agents', 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `directory` FROM `sub_module`) `s` WHERE `s`.`directory` = 'ai_agents');

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM (SELECT `id` FROM `module` WHERE `name` = 'ai' ORDER BY `id` DESC LIMIT 1) `m`),
	'ai_tasks', 'ai_tasks', 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `directory` FROM `sub_module`) `s` WHERE `s`.`directory` = 'ai_tasks');

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM (SELECT `id` FROM `module` WHERE `name` = 'ai' ORDER BY `id` DESC LIMIT 1) `m`),
	'custom_reports', 'custom_reports', 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `directory` FROM `sub_module`) `s` WHERE `s`.`directory` = 'custom_reports');
