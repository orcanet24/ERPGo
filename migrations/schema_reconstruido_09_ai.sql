-- Schema Reconstruido 09 Ai
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `ai_agent` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `provider_id` int NOT NULL,
  `model_override` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `role_id` int NOT NULL,
  `operator_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `max_steps` int NOT NULL DEFAULT '8',
  `max_rows` int NOT NULL DEFAULT '500',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `department` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `config_prompt` mediumtext COLLATE utf8mb4_unicode_ci,
  `config_prompt_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_agent_user` (`user_id`),
  KEY `idx_agent_department` (`department`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_department_prompt` (
  `department` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `prompt` mediumtext COLLATE utf8mb4_unicode_ci NOT NULL,
  `label` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`department`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_legal_rule` (
  `id` int NOT NULL AUTO_INCREMENT,
  `topic` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `title` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `citation` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `summary` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `effective_from` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `effective_to` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `verified` int NOT NULL DEFAULT '0',
  `status` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_ai_legal_topic` (`topic`,`effective_from`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;

CREATE TABLE IF NOT EXISTS `ai_providers` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `base_url` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `api_key` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `model` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `temperature` float NOT NULL DEFAULT '0.7',
  `max_tokens` int NOT NULL DEFAULT '400',
  `prompt_template` text COLLATE utf8mb4_unicode_ci,
  `is_active` tinyint NOT NULL DEFAULT '0',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `vendor` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'openai',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=21 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_task` (
  `id` int NOT NULL AUTO_INCREMENT,
  `agent_id` int NOT NULL,
  `name` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL,
  `instructions` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `schedule_type` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'manual',
  `schedule_value` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_task_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_task_run` (
  `id` int NOT NULL AUTO_INCREMENT,
  `task_id` int NOT NULL,
  `agent_id` int NOT NULL,
  `status` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'running',
  `started_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `finished_at` datetime DEFAULT NULL,
  `steps` mediumtext COLLATE utf8mb4_unicode_ci,
  `final_report` mediumtext COLLATE utf8mb4_unicode_ci,
  `undo_journal` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `idx_run_task` (`task_id`),
  KEY `idx_run_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_usage` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '0',
  `provider_id` int NOT NULL DEFAULT '0',
  `vendor` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `model` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `agent_id` int NOT NULL DEFAULT '0',
  `agent_name` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `source` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `prompt_tokens` int NOT NULL DEFAULT '0',
  `completion_tokens` int NOT NULL DEFAULT '0',
  `total_tokens` int NOT NULL DEFAULT '0',
  `requests` int NOT NULL DEFAULT '1',
  `estimated_cost_usd` double NOT NULL DEFAULT '0',
  `ok` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_day` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_ai_usage_day` (`created_day`),
  KEY `idx_ai_usage_company` (`company_id`),
  KEY `idx_ai_usage_provider` (`provider_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
