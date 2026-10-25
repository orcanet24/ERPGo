-- Schema Reconstruido 07 Pos
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `pos_offline_queue` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `register_id` int NOT NULL,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `sale_data` json NOT NULL,
  `status` enum('pending','synced','error') COLLATE utf8mb4_unicode_ci DEFAULT 'pending',
  `error_message` text COLLATE utf8mb4_unicode_ci,
  `retry_count` int DEFAULT '0',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `synced_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_status` (`status`),
  KEY `idx_branch` (`branch_id`),
  KEY `register_id` (`register_id`),
  CONSTRAINT `pos_offline_queue_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`),
  CONSTRAINT `pos_offline_queue_ibfk_2` FOREIGN KEY (`register_id`) REFERENCES `cash_registers` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pos_sessions` (
  `id` int NOT NULL AUTO_INCREMENT,
  `token` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int NOT NULL,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `branch_id` int DEFAULT NULL,
  `expires_at` datetime NOT NULL,
  `created_at` datetime NOT NULL,
  `login_time` datetime DEFAULT NULL,
  `logout_time` datetime DEFAULT NULL,
  `is_active` tinyint NOT NULL DEFAULT '1',
  `revoked` tinyint(1) DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pos_session_token` (`token`),
  KEY `idx_pos_session_terminal` (`terminal_id`),
  KEY `idx_pos_session_branch` (`branch_id`),
  KEY `idx_pos_session_expires` (`expires_at`),
  KEY `idx_pos_session_user` (`user_id`),
  KEY `idx_pos_session_active` (`is_active`)
) ENGINE=InnoDB AUTO_INCREMENT=51 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pos_supervisor_auth` (
  `id` int NOT NULL AUTO_INCREMENT,
  `session_id` int NOT NULL,
  `supervisor_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `auth_type` enum('devolucion','cierre_caja','anulacion') COLLATE utf8mb4_unicode_ci NOT NULL,
  `auth_time` datetime NOT NULL,
  `reason` text COLLATE utf8mb4_unicode_ci,
  `barcode` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_pos_supervisor_session` (`session_id`),
  KEY `idx_pos_supervisor_user` (`supervisor_id`)
) ENGINE=InnoDB AUTO_INCREMENT=10 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pos_terminal_sessions` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `register_id` int NOT NULL,
  `api_key_id` int NOT NULL,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int DEFAULT NULL,
  `user_name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ip_address` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `started_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `last_activity` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `closed_at` timestamp NULL DEFAULT NULL,
  `status` enum('active','closed','timeout') COLLATE utf8mb4_unicode_ci DEFAULT 'active',
  PRIMARY KEY (`id`),
  KEY `idx_branch` (`branch_id`),
  KEY `idx_register` (`register_id`),
  KEY `idx_terminal` (`terminal_id`),
  KEY `idx_status` (`status`),
  KEY `api_key_id` (`api_key_id`),
  CONSTRAINT `pos_terminal_sessions_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`),
  CONSTRAINT `pos_terminal_sessions_ibfk_2` FOREIGN KEY (`register_id`) REFERENCES `cash_registers` (`id`),
  CONSTRAINT `pos_terminal_sessions_ibfk_3` FOREIGN KEY (`api_key_id`) REFERENCES `api_keys` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pos_turn_events` (
  `id` int NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `branch_id` int DEFAULT NULL,
  `user_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `event_type` enum('login','logout','open_turn','close_turn','devolucion','cierre_caja') COLLATE utf8mb4_unicode_ci NOT NULL,
  `event_time` datetime NOT NULL,
  `details` text COLLATE utf8mb4_unicode_ci,
  `supervisor_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_pos_events_terminal` (`terminal_id`),
  KEY `idx_pos_events_user` (`user_id`),
  KEY `idx_pos_events_type` (`event_type`),
  KEY `idx_pos_events_time` (`event_time`)
) ENGINE=InnoDB AUTO_INCREMENT=69 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pos_turns` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `register_id` int NOT NULL,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int DEFAULT NULL,
  `user_name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opened_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `closed_at` timestamp NULL DEFAULT NULL,
  `initial_fund` decimal(12,2) DEFAULT '0.00',
  `declared_cash` decimal(12,2) DEFAULT '0.00',
  `declared_card` decimal(12,2) DEFAULT '0.00',
  `system_cash` decimal(12,2) DEFAULT '0.00',
  `system_card` decimal(12,2) DEFAULT '0.00',
  `difference` decimal(12,2) DEFAULT '0.00',
  `total_sales` decimal(12,2) DEFAULT '0.00',
  `total_refunds` decimal(12,2) DEFAULT '0.00',
  `sales_count` int DEFAULT '0',
  `status` enum('open','closed','paused') COLLATE utf8mb4_unicode_ci DEFAULT 'open',
  `notes` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `idx_branch` (`branch_id`),
  KEY `idx_register` (`register_id`),
  KEY `idx_status` (`status`),
  KEY `idx_user` (`user_id`),
  CONSTRAINT `pos_turns_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`),
  CONSTRAINT `pos_turns_ibfk_2` FOREIGN KEY (`register_id`) REFERENCES `cash_registers` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `branches` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '1',
  `code` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` text COLLATE utf8mb4_unicode_ci,
  `phone` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `email` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_active` tinyint(1) DEFAULT '1',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`),
  KEY `idx_company` (`company_id`),
  KEY `idx_active` (`is_active`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cash_registers` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `code` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text COLLATE utf8mb4_unicode_ci,
  `printer_port` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'COM1',
  `printer_brand` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT 'epson',
  `printer_model` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT 'TM-2000',
  `serial_port` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'COM1',
  `baud_rate` int DEFAULT '9600',
  `fiscal_enabled` tinyint(1) DEFAULT '1',
  `status` enum('active','inactive','maintenance') COLLATE utf8mb4_unicode_ci DEFAULT 'active',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_branch_code` (`branch_id`,`code`),
  KEY `idx_branch` (`branch_id`),
  KEY `idx_status` (`status`),
  CONSTRAINT `cash_registers_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cash_session` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cashier` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Usuario cajero',
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opening_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Fondo inicial',
  `opening_amount_usd` decimal(18,2) DEFAULT '0.00',
  `closing_amount` decimal(18,2) DEFAULT NULL COMMENT 'Efectivo contado al cierre',
  `closing_amount_usd` decimal(18,2) DEFAULT NULL,
  `expected_cash` decimal(18,2) DEFAULT NULL COMMENT 'Efectivo esperado por sistema',
  `expected_cash_usd` decimal(18,2) DEFAULT NULL,
  `difference` decimal(18,2) DEFAULT NULL COMMENT 'Sobrante/faltante',
  `difference_usd` decimal(18,2) DEFAULT NULL,
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '1=abierta, 2=cerrada',
  `currency_` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `opened_at` datetime NOT NULL,
  `closed_at` datetime DEFAULT NULL,
  `open_note` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `close_note` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_cs_cashier_status` (`cashier`,`status`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cash_session_payment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cash_session_id` int NOT NULL,
  `payment_method_id` int NOT NULL,
  `tx_count` int NOT NULL DEFAULT '0',
  `amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `counted_amount` decimal(18,2) DEFAULT NULL COMMENT 'Monto contado por el cajero (efectivo)',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_csp` (`cash_session_id`,`payment_method_id`),
  KEY `idx_csp_session` (`cash_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `api_keys` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `api_key` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text COLLATE utf8mb4_unicode_ci,
  `is_active` tinyint(1) DEFAULT '1',
  `last_used_at` timestamp NULL DEFAULT NULL,
  `created_by` int DEFAULT NULL,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `api_key` (`api_key`),
  KEY `idx_branch` (`branch_id`),
  KEY `idx_active` (`is_active`),
  KEY `idx_key` (`api_key`),
  CONSTRAINT `api_keys_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `module_purchase_key` (
  `id` int NOT NULL AUTO_INCREMENT,
  `identity` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `purchase_key` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `module_registry` (
  `code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text COLLATE utf8mb4_unicode_ci,
  `version` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `api` int NOT NULL DEFAULT '1',
  `entry` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `binary_sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `rif_binding` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `runtime` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'inproc',
  `signature` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'detected',
  `menu_mid` int DEFAULT NULL,
  `available_version` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `detected_at` datetime NOT NULL,
  `activated_at` datetime DEFAULT NULL,
  `deactivated_at` datetime DEFAULT NULL,
  `last_error` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`code`),
  KEY `idx_estado` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
