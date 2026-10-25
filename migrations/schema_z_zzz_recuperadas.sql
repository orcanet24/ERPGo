-- ============================================================================
-- ERPGo — Recuperación de tablas que faltaban en las migraciones.
--
-- ESTAS TABLAS SE PERDIERON DEL ESQUEMA. Existían en las bases de producción
-- (creadas por ALTERs históricos y scripts manuales que nunca llegaron a
-- migrations/*.sql) y el código las usa todos los días, pero NINGÚN archivo de
-- migración las creaba. Consecuencia: una instalación NUEVA (empresa nueva,
-- servidor nuevo) arrancaba sin ellas y fallaba en producción al primer uso:
--
--   - POS: login de caja, validación de API key, turnos  -> Unknown table
--   - Cuentas por pagar                                  -> Unknown table
--   - Permisos por departamento (user_department)        -> Unknown table
--   - Multi-moneda: tasa vigente / auditoría             -> Unknown table
--
-- El DDL se recuperó de las bases en uso (SHOW CREATE TABLE) y se normalizó a
-- CREATE TABLE IF NOT EXISTS, idempotente: en las bases que ya las tienen no
-- hace nada; en una instalación nueva las crea.
--
-- El orden respeta las claves foráneas: branches -> cash_registers -> api_keys
-- antes de las tablas POS que las referencian.
-- ============================================================================

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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

CREATE TABLE IF NOT EXISTS `pos_turn_payments` (
  `id` int NOT NULL AUTO_INCREMENT,
  `turn_id` int NOT NULL,
  `payment_method` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(12,2) NOT NULL,
  `reference` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_turn` (`turn_id`),
  CONSTRAINT `pos_turn_payments_ibfk_1` FOREIGN KEY (`turn_id`) REFERENCES `pos_turns` (`id`) ON DELETE CASCADE
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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

CREATE TABLE IF NOT EXISTS `accounts_payable` (
  `id` int NOT NULL AUTO_INCREMENT,
  `supplier_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `purchase_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `paid_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `due_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `payment_method_id` int DEFAULT NULL,
  `payment_date` date DEFAULT NULL,
  `reference` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_supplier` (`supplier_id`),
  KEY `idx_purchase` (`purchase_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `sec_department` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `status` tinyint(1) DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_department` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(15) COLLATE utf8mb4_unicode_ci NOT NULL,
  `department_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_dept` (`user_id`,`department_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_company_access` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(15) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `role_id` int NOT NULL DEFAULT '2',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_tenant` (`user_id`,`tenant_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `exchange_rate_current` (
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL,
  `rate` decimal(18,4) NOT NULL,
  `source` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'manual',
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`currency`),
  KEY `idx_rate_curr` (`currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `exchange_rate_audit` (
  `id` int NOT NULL AUTO_INCREMENT,
  `rate_date` date NOT NULL,
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL,
  `rate` decimal(18,4) NOT NULL,
  `source` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `applied_by` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `applied_at` datetime NOT NULL,
  `invoice_id` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_audit_date` (`rate_date`,`currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `modules` (
  `id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `version` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '1.0',
  `description` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `is_core` tinyint(1) NOT NULL DEFAULT '0',
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `path` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `author` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `license_type` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT 'MIT',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
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

CREATE TABLE IF NOT EXISTS `invoices` (
  `id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant_code` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_number` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `control_number` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cash_register_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `rif` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_name` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_address` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_phone` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `subtotal` decimal(15,2) NOT NULL DEFAULT '0.00',
  `tax` decimal(15,2) NOT NULL DEFAULT '0.00',
  `total` decimal(15,2) NOT NULL DEFAULT '0.00',
  `tax_rate` decimal(5,2) NOT NULL DEFAULT '16.00',
  `status` enum('draft','printing','printed','cancelled') COLLATE utf8mb4_unicode_ci DEFAULT 'draft',
  `created_at` datetime NOT NULL,
  `printed_at` datetime DEFAULT NULL,
  `fiscal_number` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_tenant` (`tenant_code`),
  KEY `idx_invoice_number` (`invoice_number`),
  KEY `idx_terminal` (`terminal_id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `invoice_items` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_code` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sku` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(300) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(15,3) NOT NULL,
  `unit_price` decimal(15,2) NOT NULL,
  `discount_pct` decimal(5,2) DEFAULT '0.00',
  `tax_rate` decimal(5,2) DEFAULT '16.00',
  `subtotal` decimal(15,2) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_invoice` (`invoice_id`),
  CONSTRAINT `invoice_items_ibfk_1` FOREIGN KEY (`invoice_id`) REFERENCES `invoices` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cost_project` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL,
  `customer_id` bigint DEFAULT NULL,
  `billing_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'horas',
  `hourly_rate` decimal(16,4) NOT NULL DEFAULT '0.0000',
  `budget_hours` decimal(12,2) NOT NULL DEFAULT '0.00',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'activo',
  `starts_on` date DEFAULT NULL,
  `ends_on` date DEFAULT NULL,
  `notes` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_project_code` (`code`),
  KEY `idx_proj_customer` (`customer_id`),
  KEY `idx_proj_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cost_task` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `code` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL,
  `billable_default` tinyint NOT NULL DEFAULT '1',
  `active` tinyint NOT NULL DEFAULT '1',
  `notes` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_task_code` (`project_id`,`code`),
  KEY `idx_task_project` (`project_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `debt_collection` (
  `id` int NOT NULL AUTO_INCREMENT,
  `customer_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `payment_method_id` int DEFAULT NULL,
  `collection_date` date DEFAULT NULL,
  `reference` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `amount_usd` decimal(18,2) DEFAULT '0.00',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  PRIMARY KEY (`id`),
  KEY `idx_customer` (`customer_id`),
  KEY `idx_invoice` (`invoice_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
