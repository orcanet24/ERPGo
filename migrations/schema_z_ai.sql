-- ---------------------------------------------------------------------------
-- AI Business Analyst (module IA): default report types + menu permissions.
--
-- `ai_setting` and `ai_report_types` already exist (see migrations/schema.sql).
-- Every statement below runs as INSERT IGNORE (see transformStatement) AND is
-- guarded with NOT EXISTS, so the migration is idempotent both on a fresh
-- install (empty tables) and on a database migrated from the PHP ERP that may
-- already contain these rows.
--
-- File name: fs.ReadDir("migrations") returns entries sorted by filename, so
-- this file must sort AFTER schema.sql, schema_extra.sql and
-- schema_menu_permissions.sql -> "schema_z_ai.sql".
--
-- sub_module.directory values (read by permissionMap/hasPermission and by the
-- sidebar {{if index .Perm "..."}} gates):
--   ai_settings                  -> menu "Configuracion IA"
--   ai_business_analyst_report   -> menu "AI Business Analyst"
-- ---------------------------------------------------------------------------

-- Default report types understood by AI_Controller::bdtask_get_ai_analysis
-- ("chart" enables the Chart.js series, "table_summary" the report table).
INSERT INTO `ai_report_types` (`name`)
SELECT 'sales' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'sales');

INSERT INTO `ai_report_types` (`name`)
SELECT 'purchase' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'purchase');

INSERT INTO `ai_report_types` (`name`)
SELECT 'stock' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'stock');

INSERT INTO `ai_report_types` (`name`)
SELECT 'sales_return' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'sales_return');

INSERT INTO `ai_report_types` (`name`)
SELECT 'supplier_return' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'supplier_return');

INSERT INTO `ai_report_types` (`name`)
SELECT 'chart' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'chart');

INSERT INTO `ai_report_types` (`name`)
SELECT 'table_summary' FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_report_types`) `t` WHERE `t`.`name` = 'table_summary');

-- Module that groups both AI sub-modules (module ids 1-15 are already seeded
-- by schema_menu_permissions.sql; the id is auto-generated to avoid clashes
-- with a PHP-era database that may already use id 16+).
INSERT INTO `module` (`name`, `description`, `directory`, `status`)
SELECT 'ai', 'AI Business Analyst', NULL, 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `module`) `t` WHERE `t`.`name` = 'ai');

-- Sidebar entries (ids auto-generated: sub_module.AUTO_INCREMENT is 146 on a
-- fresh install and max(id)+1 on the migrated PHP database).
INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM (SELECT `id` FROM `module` WHERE `name` = 'ai' ORDER BY `id` DESC LIMIT 1) `m`),
	'ai_settings', 'ai_settings', 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `directory` FROM `sub_module`) `s` WHERE `s`.`directory` = 'ai_settings');

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM (SELECT `id` FROM `module` WHERE `name` = 'ai' ORDER BY `id` DESC LIMIT 1) `m`),
	'ai_business_analyst_report', 'ai_business_analyst_report', 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `directory` FROM `sub_module`) `s` WHERE `s`.`directory` = 'ai_business_analyst_report');
