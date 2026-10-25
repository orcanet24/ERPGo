-- Schema Reconstruido 01 Accounts
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `acc_period_close` (
  `period` varchar(6) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'AAAAMM',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'open' COMMENT 'closed | reopened',
  `closed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopened_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopened_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopen_reason` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'motivo obligatorio de cada reapertura',
  PRIMARY KEY (`period`),
  KEY `idx_acc_period_status` (`status`,`period`)
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

SET FOREIGN_KEY_CHECKS=1;
