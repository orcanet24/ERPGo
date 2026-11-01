-- ---------------------------------------------------------------------------
-- AI providers: multi-provider support for the AI Business Analyst module.
--
-- The legacy single-row `ai_setting` table (see migrations/schema.sql) is NOT
-- dropped: it can hold the API key migrated from the PHP ERP, and it is still
-- read as a fallback by loadActiveProvider() (interfaces/web/ai_providers.go)
-- when no row of `ai_providers` is active.
--
-- File name: fs.ReadDir("migrations") returns entries sorted by filename, so
-- this file runs AFTER schema.sql and AFTER schema_z_ai.sql:
--   "schema_z_ai.sql" < "schema_z_ai_providers.sql" ('.' = 0x2E < '_' = 0x5F)
--
-- Every statement below is rewritten to INSERT IGNORE (see
-- transformStatement) AND is additionally guarded with NOT EXISTS, so running
-- the migration twice never duplicates a seed row.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `ai_providers` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `base_url` varchar(500) NOT NULL DEFAULT '',
  `api_key` varchar(500) DEFAULT NULL,
  `model` varchar(150) NOT NULL DEFAULT '',
  `temperature` float NOT NULL DEFAULT '0.7',
  `max_tokens` int NOT NULL DEFAULT '400',
  `prompt_template` text,
  `is_active` tinyint NOT NULL DEFAULT '0',
  `vendor` varchar(20) NOT NULL DEFAULT 'openai',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -------------------------------------------------------------------------
-- Seeds: no API key is stored here (each user enters its own key from the
-- UI). Only OpenAI can be born active, and only on a FRESH install (the
-- legacy ai_setting has no api_key): a database migrated from the PHP ERP
-- keeps all seeds inactive so its legacy key keeps working through the
-- ai_setting fallback until the user activates a provider explicitly.
-- -------------------------------------------------------------------------

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'OpenAI', 'https://api.openai.com/v1', NULL, 'gpt-4o-mini', 0.7, 400, NULL,
	IF((SELECT COUNT(*) FROM `ai_setting` WHERE IFNULL(`api_key`, '') <> '') = 0, 1, 0) FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'OpenAI');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'OpenRouter', 'https://openrouter.ai/api/v1', NULL, 'openai/gpt-4o-mini', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'OpenRouter');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'Groq', 'https://api.groq.com/openai/v1', NULL, 'llama-3.3-70b-versatile', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'Groq');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'DeepSeek', 'https://api.deepseek.com/v1', NULL, 'deepseek-chat', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'DeepSeek');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'Mistral', 'https://api.mistral.ai/v1', NULL, 'mistral-small-latest', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'Mistral');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'xAI', 'https://api.x.ai/v1', NULL, 'grok-2-latest', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'xAI');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'Together AI', 'https://api.together.xyz/v1', NULL, 'meta-llama/Llama-3.3-70B-Instruct-Turbo', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'Together AI');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'Ollama', 'http://localhost:11434/v1', NULL, 'llama3.2', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'Ollama');

INSERT INTO `ai_providers` (`name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'LM Studio', 'http://localhost:1234/v1', NULL, 'local-model', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'LM Studio');

INSERT INTO `ai_providers` (`name`, `vendor`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`)
SELECT 'Personalizado', 'personalizado', 'https://api.ejemplo.com/v1', NULL, 'gpt-4o-mini', 0.7, 400, NULL, 0 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `name` FROM `ai_providers`) `t` WHERE `t`.`name` = 'Personalizado');
