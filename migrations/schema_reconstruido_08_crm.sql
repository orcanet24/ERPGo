-- Schema Reconstruido 08 Crm
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `crm_activity` (
  `id` int NOT NULL AUTO_INCREMENT,
  `contact_id` int NOT NULL DEFAULT '0',
  `opportunity_id` int NOT NULL DEFAULT '0',
  `activity_type` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'llamada' COMMENT 'llamada | email | reunion | visita | tarea | nota',
  `subject` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `body` text COLLATE utf8mb4_unicode_ci,
  `due_date` date DEFAULT NULL,
  `done` int NOT NULL DEFAULT '0',
  `created_by` int NOT NULL DEFAULT '0',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `completed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_activity_due` (`done`,`due_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crm_contact` (
  `id` int NOT NULL AUTO_INCREMENT,
  `full_name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL,
  `email` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `phone` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `company_name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `position_title` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `customer_id` int DEFAULT NULL COMMENT 'se llena al convertir la oportunidad',
  `supplier_id` int DEFAULT NULL,
  `source` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'otro' COMMENT 'referido | web | feria | red | cold_call | otro',
  `owner_id` int NOT NULL DEFAULT '0' COMMENT 'usuario responsable del seguimiento',
  `status` int NOT NULL DEFAULT '1' COMMENT '1=activo, 0=descartado',
  `notes` text COLLATE utf8mb4_unicode_ci,
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_contact_customer` (`customer_id`),
  KEY `idx_crm_contact_owner` (`owner_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crm_opportunity` (
  `id` int NOT NULL AUTO_INCREMENT,
  `title` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `contact_id` int NOT NULL,
  `stage_id` int NOT NULL DEFAULT '0',
  `amount` decimal(16,2) NOT NULL DEFAULT '0.00',
  `expected_close` date DEFAULT NULL,
  `owner_id` int NOT NULL DEFAULT '0',
  `description` text COLLATE utf8mb4_unicode_ci,
  `lost_reason` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  KEY `idx_crm_opp_stage` (`stage_id`),
  KEY `idx_crm_opp_contact` (`contact_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crm_stage` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL,
  `position` int NOT NULL DEFAULT '0',
  `probability` int NOT NULL DEFAULT '0' COMMENT 'probabilidad de cierre en % (0-100)',
  `is_won` int NOT NULL DEFAULT '0' COMMENT '1 = etapa cerrada ganada',
  `is_lost` int NOT NULL DEFAULT '0',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
