-- Schema Reconstruido 04 Suppliers
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `supplier_information` (
  `supplier_id` bigint NOT NULL AUTO_INCREMENT,
  `supplier_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `address2` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `mobile` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `emailnumber` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `email_address` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `contact` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `phone` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `fax` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `city` text COLLATE utf8mb4_unicode_ci,
  `state` text COLLATE utf8mb4_unicode_ci,
  `zip` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `country` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `details` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int NOT NULL,
  `rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'RIF del proveedor',
  `supplier_type` tinyint NOT NULL DEFAULT '1' COMMENT '1=Contribuyente formal, 2=No contribuyente',
  `islr_concept_default` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Concepto ISLR por defecto (Decreto 1808)',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  PRIMARY KEY (`supplier_id`),
  KEY `supplier_id` (`supplier_id`),
  KEY `idx_supplier_rif` (`rif`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `supplier_product` (
  `supplier_pr_id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(30) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci NOT NULL,
  `products_model` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `supplier_id` bigint NOT NULL,
  `supplier_price` varchar(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`supplier_pr_id`),
  KEY `product_id` (`product_id`),
  KEY `supplier_id` (`supplier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `import_cost` (
  `id` int NOT NULL AUTO_INCREMENT,
  `purchase_id` bigint NOT NULL,
  `freight` decimal(16,2) NOT NULL DEFAULT '0.00',
  `customs` decimal(16,2) NOT NULL DEFAULT '0.00',
  `insurance` decimal(16,2) NOT NULL DEFAULT '0.00',
  `other` decimal(16,2) NOT NULL DEFAULT '0.00',
  `received_qty` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'USD',
  `exchange_rate` decimal(18,4) NOT NULL DEFAULT '1.0000',
  `notes` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_import_purchase` (`purchase_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
