-- Schema Reconstruido 05 Financial
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `expiry_alert_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `lot_id` int NOT NULL,
  `product_id` int NOT NULL,
  `days_left` int NOT NULL DEFAULT '0',
  `severity` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'info' COMMENT 'info | warning | critical',
  `acknowledged_by` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `acknowledged_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `note` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_alert_lot` (`lot_id`),
  KEY `idx_alert_product` (`product_id`)
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

CREATE TABLE IF NOT EXISTS `exchange_rate_current` (
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL,
  `rate` decimal(18,4) NOT NULL,
  `source` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'manual',
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`currency`),
  KEY `idx_rate_curr` (`currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `exchange_rate_daily` (
  `rate_date` date NOT NULL,
  `currency` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'USD',
  `source` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'BCV',
  `buy_rate` double NOT NULL DEFAULT '0',
  `sell_rate` double NOT NULL DEFAULT '0' COMMENT '0 = no se registró, se usa buy_rate',
  `official_rate` double NOT NULL DEFAULT '0' COMMENT 'tasa oficial del BCV del día',
  `is_parallel` int NOT NULL DEFAULT '0' COMMENT '1 = tasa paralela',
  `entered_by` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`rate_date`,`currency`,`source`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `exchange_rate_source` (
  `code` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'BCV | paralelo | oficial | banco',
  `name` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `description` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `active` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `fiscal_series` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'FACT, NC, ND, RET-IVA, RET-ISLR',
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `prefix` varchar(6) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Prefijo impreso, ej: 00-',
  `next_no` bigint NOT NULL DEFAULT '1' COMMENT 'Próximo número a asignar',
  `control_prefix` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '00' COMMENT 'Prefijo del número de control',
  `control_next` bigint NOT NULL DEFAULT '1' COMMENT 'Próximo número de control',
  `is_active` tinyint NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_series_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=371 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `fiscal_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `key` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `value` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fs_key` (`key`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `payment_method` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'cash, punto, pago_movil, zelle, transferencia, cripto, credito',
  `name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_foreign` tinyint NOT NULL DEFAULT '0' COMMENT '1=pago en divisa/cripto → aplica IGTF',
  `coa_id` bigint DEFAULT NULL COMMENT 'Cuenta contable asociada',
  `is_active` tinyint NOT NULL DEFAULT '1',
  `sort_order` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pm_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=11 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `price_list` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `sort_order` int NOT NULL DEFAULT '0',
  `is_default` tinyint NOT NULL DEFAULT '0',
  `active` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_price_list_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tax_collection` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `customer_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `relation_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tax0` text COLLATE utf8mb4_unicode_ci,
  `tax1` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `customer_id` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tax_rate_table` (
  `concept_code` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'IVSS_WORKER, IVSS_EMPLOYER, INCEPS, FAOV, BPSI...',
  `label` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `rate_pct` decimal(8,4) NOT NULL DEFAULT '0.0000' COMMENT 'porcentaje de la base imponible',
  `base_kind` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'salary' COMMENT 'salary | salary_min | n/a',
  `applies_to` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'employer' COMMENT 'worker | employer',
  `effective_from` date NOT NULL,
  `effective_to` date DEFAULT NULL,
  `legal_citation` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'ley/gaceta/fuente que fija la tasa',
  `verified` int NOT NULL DEFAULT '0' COMMENT '1 = validada por el contador de la empresa',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`concept_code`,`effective_from`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tax_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `default_value` float NOT NULL,
  `tax_name` varchar(250) COLLATE utf8mb4_unicode_ci NOT NULL,
  `nt` int NOT NULL,
  `reg_no` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_show` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `vat_tax_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `dynamic_tax` int NOT NULL DEFAULT '0',
  `fixed_tax` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `islr_bracket` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bracket_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'tipo1' COMMENT 'tipo1 = deps sin remuneración adicional',
  `from_ut` decimal(14,2) NOT NULL DEFAULT '0.00',
  `to_ut` decimal(14,2) DEFAULT NULL COMMENT 'NULL = sin tope',
  `pct` decimal(8,4) NOT NULL DEFAULT '0.0000',
  `excess_ut` decimal(14,2) NOT NULL DEFAULT '0.00' COMMENT 'exceso gravable en UT dentro del tramo',
  `effective_from` date NOT NULL,
  `effective_to` date DEFAULT NULL,
  `legal_citation` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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

CREATE TABLE IF NOT EXISTS `service_invoice` (
  `id` int NOT NULL AUTO_INCREMENT,
  `voucher_no` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` date NOT NULL,
  `employee_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `total_amount` decimal(20,2) NOT NULL DEFAULT '0.00',
  `total_discount` decimal(20,2) NOT NULL DEFAULT '0.00',
  `invoice_discount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'total invoice vat',
  `total_tax` decimal(10,2) DEFAULT '0.00',
  `paid_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `due_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `shipping_cost` decimal(10,2) NOT NULL DEFAULT '0.00',
  `previous` decimal(10,2) NOT NULL DEFAULT '0.00',
  `details` varchar(250) COLLATE utf8mb4_unicode_ci NOT NULL,
  `sales_by` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `is_credit` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `customer_id` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `service_invoice_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `service_id` int NOT NULL,
  `service_inv_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `qty` decimal(10,2) NOT NULL DEFAULT '0.00',
  `charge` decimal(10,2) NOT NULL DEFAULT '0.00',
  `discount` decimal(10,2) DEFAULT '0.00',
  `discount_amount` decimal(10,2) DEFAULT '0.00',
  `vat` decimal(10,2) DEFAULT '0.00',
  `vat_amnt` decimal(10,2) DEFAULT '0.00',
  `total` decimal(10,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `service_id` (`service_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
