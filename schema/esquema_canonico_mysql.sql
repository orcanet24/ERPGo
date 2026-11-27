-- ESQUEMA CANÓNICO DEL ERPGo (MySQL)
-- Generado por tools/esquemacanonico a partir de la cadena de migraciones.
-- Vive en schema/ y NO en migrations/: el runner aplica todos los .sql de migrations/,
-- y un canónico dentro de esa carpeta se aplicaría a sí mismo.
-- Instalación limpia: este archivo monta tablas, triggers y siembras.
-- Para bases ya existentes usa el reconciliador (Fase 2 del plan), no este archivo.
-- Regenerar con: go run ./tools/esquemacanonico

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE `acc_coa` (
  `id` int NOT NULL AUTO_INCREMENT,
  `HeadCode` int NOT NULL,
  `HeadName` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `PHeadName` varchar(200) COLLATE utf8mb4_unicode_ci NOT NULL,
  `PHeadCode` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `HeadLevel` int NOT NULL,
  `IsActive` tinyint(1) NOT NULL,
  `IsTransaction` tinyint(1) NOT NULL,
  `IsGL` tinyint(1) NOT NULL,
  `isCashNature` int NOT NULL DEFAULT '0',
  `isBankNature` int NOT NULL DEFAULT '0',
  `HeadType` char(1) COLLATE utf8mb4_unicode_ci NOT NULL,
  `IsBudget` tinyint(1) NOT NULL,
  `IsDepreciation` tinyint(1) NOT NULL,
  `customer_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supplier_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `bank_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `service_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `DepreciationRate` decimal(18,2) NOT NULL,
  `CreateBy` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `CreateDate` datetime NOT NULL,
  `UpdateBy` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `UpdateDate` datetime NOT NULL,
  `isSubType` int NOT NULL DEFAULT '0',
  `subType` int NOT NULL DEFAULT '1',
  `isStock` int NOT NULL DEFAULT '0',
  `isFixedAssetSch` int NOT NULL DEFAULT '0',
  `noteNo` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `assetCode` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `depCode` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `HeadCode` (`HeadCode`),
  KEY `HeadName` (`HeadName`),
  KEY `customer_id` (`customer_id`),
  KEY `supplier_id` (`supplier_id`),
  KEY `service_id` (`service_id`),
  KEY `PHeadName` (`PHeadName`),
  KEY `HeadLevel` (`HeadLevel`),
  KEY `IsTransaction` (`IsTransaction`),
  KEY `IsGL` (`IsGL`),
  KEY `IsBudget` (`IsBudget`),
  KEY `IsDepreciation` (`IsDepreciation`),
  KEY `isCashNature` (`isCashNature`),
  KEY `isBankNature` (`isBankNature`)
) ENGINE=InnoDB AUTO_INCREMENT=907 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_monthly_balance` (
  `id` int NOT NULL AUTO_INCREMENT,
  `fyear` int NOT NULL,
  `COAID` int NOT NULL,
  `balance1` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance2` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance3` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance4` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance5` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance6` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance7` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance8` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance9` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance10` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance11` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance12` decimal(18,2) NOT NULL DEFAULT '0.00',
  `totalBalance` decimal(18,2) NOT NULL DEFAULT '0.00',
  `updatedDate` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_opening_balance` (
  `id` int NOT NULL AUTO_INCREMENT,
  `fyear` int NOT NULL,
  `COAID` int NOT NULL,
  `subType` int NOT NULL DEFAULT '1',
  `subCode` int DEFAULT NULL,
  `Debit` decimal(10,0) NOT NULL,
  `Credit` decimal(10,0) NOT NULL,
  `openDate` date NOT NULL,
  `CreateBy` int NOT NULL,
  `CreateDate` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `COAID` (`COAID`),
  KEY `year` (`fyear`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_period_close` (
  `period` varchar(6) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'AAAAMM',
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'open' COMMENT 'closed | reopened',
  `closed_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopened_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopened_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopen_reason` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'motivo obligatorio de cada reapertura',
  PRIMARY KEY (`period`),
  KEY `idx_acc_period_status` (`status`,`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_predefine_account` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cashCode` int NOT NULL,
  `bankCode` int NOT NULL,
  `advance` int NOT NULL,
  `fixedAsset` int NOT NULL,
  `purchaseCode` int NOT NULL,
  `salesCode` int NOT NULL,
  `serviceCode` int NOT NULL,
  `customerCode` int NOT NULL,
  `supplierCode` int NOT NULL,
  `costs_of_good_solds` int NOT NULL,
  `vat` int NOT NULL,
  `tax` int NOT NULL,
  `inventoryCode` int NOT NULL,
  `CPLCode` int NOT NULL,
  `LPLCode` int NOT NULL,
  `salary_code` int DEFAULT NULL,
  `emp_npf_contribution` int DEFAULT NULL,
  `empr_npf_contribution` int DEFAULT NULL,
  `emp_icf_contribution` int DEFAULT NULL,
  `empr_icf_contribution` int DEFAULT NULL,
  `prov_state_tax` int NOT NULL,
  `state_tax` int NOT NULL,
  `prov_npfcode` int DEFAULT NULL,
  `igtf_code` int NOT NULL DEFAULT '0',
  `supplies_inventory_code` int NOT NULL DEFAULT '0',
  `supplies_expense_code` int NOT NULL DEFAULT '0',
  `service_expense_code` int NOT NULL DEFAULT '0',
  `purchase_vat_code` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `COAID` (`cashCode`),
  KEY `cashCode` (`cashCode`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_subcode` (
  `id` int NOT NULL AUTO_INCREMENT,
  `subTypeId` int NOT NULL,
  `name` varchar(250) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `referenceNo` int NOT NULL,
  `status` int NOT NULL DEFAULT '1',
  `created_by` int NOT NULL,
  `created_date` date NOT NULL,
  `updated_by` int NOT NULL,
  `updated_date` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `subTypeId` (`subTypeId`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_subtype` (
  `id` int NOT NULL,
  `subtypeName` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `staus` int NOT NULL DEFAULT '1',
  `created_by` int NOT NULL,
  `created_date` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_transaction` (
  `ID` int NOT NULL AUTO_INCREMENT,
  `vid` int NOT NULL,
  `fyear` int NOT NULL,
  `VNo` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `Vtype` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `referenceNo` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `VDate` date DEFAULT NULL,
  `COAID` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `Narration` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `chequeNo` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `chequeDate` date DEFAULT NULL,
  `isHonour` int DEFAULT NULL,
  `ledgerComment` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '0',
  `Debit` decimal(18,2) DEFAULT NULL,
  `Credit` decimal(18,2) DEFAULT NULL,
  `StoreID` int NOT NULL DEFAULT '0',
  `IsPosted` char(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_opening` int NOT NULL DEFAULT '0',
  `CreateBy` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `CreateDate` datetime DEFAULT NULL,
  `UpdateBy` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `UpdateDate` datetime DEFAULT NULL,
  `IsAppove` char(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `RevCodde` bigint NOT NULL,
  `subType` int NOT NULL DEFAULT '1',
  `subCode` int DEFAULT NULL,
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  UNIQUE KEY `ID` (`ID`),
  KEY `COAID` (`COAID`),
  KEY `VNo` (`VNo`),
  KEY `RevCodde` (`RevCodde`),
  KEY `subType` (`subType`),
  KEY `subCode` (`subCode`),
  KEY `referenceNo` (`referenceNo`),
  KEY `vid` (`vid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `acc_vaucher` (
  `id` int NOT NULL AUTO_INCREMENT,
  `fyear` int NOT NULL,
  `VNo` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `Vtype` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `referenceNo` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `VDate` date NOT NULL,
  `COAID` int NOT NULL,
  `Narration` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `chequeno` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `chequeDate` date DEFAULT NULL,
  `isHonour` int NOT NULL DEFAULT '0',
  `ledgerComment` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `Debit` decimal(18,2) NOT NULL DEFAULT '0.00',
  `Credit` decimal(18,2) NOT NULL DEFAULT '0.00',
  `RevCodde` int NOT NULL,
  `subType` int NOT NULL DEFAULT '1',
  `subCode` int DEFAULT NULL,
  `isApproved` int NOT NULL DEFAULT '0',
  `CreateBy` int NOT NULL,
  `CreateDate` datetime NOT NULL,
  `UpdateBy` int DEFAULT NULL,
  `UpdateDate` datetime DEFAULT NULL,
  `approvedBy` int DEFAULT NULL,
  `approvedDate` datetime DEFAULT NULL,
  `isyearClosed` tinyint(1) NOT NULL DEFAULT '0',
  `status` int NOT NULL DEFAULT '0' COMMENT '0 = non yet transfer to transation,  1 = Tranfer to transition',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  PRIMARY KEY (`id`),
  KEY `VNo` (`VNo`),
  KEY `fyear` (`fyear`),
  KEY `VDate` (`VDate`),
  KEY `COAID` (`COAID`),
  KEY `RevCodde` (`RevCodde`),
  KEY `subType` (`subType`),
  KEY `subCode` (`subCode`),
  KEY `referenceNo` (`referenceNo`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `accounts_payable` (
  `id` int NOT NULL AUTO_INCREMENT,
  `supplier_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `purchase_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `paid_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `due_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `payment_method_id` int DEFAULT NULL,
  `payment_date` date DEFAULT NULL,
  `reference` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `concept` varchar(200) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'que se debe: servicio, gasto o documento',
  `origin` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'compra' COMMENT 'compra | servicio | pago_compra',
  PRIMARY KEY (`id`),
  KEY `idx_supplier` (`supplier_id`),
  KEY `idx_purchase` (`purchase_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `activity_logs` (
  `activity_id` int NOT NULL,
  `user_id` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'for employee , it will save employee_id',
  `type` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'ticket, employee, attendnace etc.',
  `action` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'create, update, delete',
  `action_id` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `table_name` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `slug` varchar(150) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `form_data` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `create_date` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `status` tinyint(1) NOT NULL DEFAULT '0' COMMENT '1=created, 2=updated, 3=deleted	'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `addon_capability_approvals` (
  `addon_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_approved` tinyint(1) NOT NULL DEFAULT '0',
  `decided_by` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `decided_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`addon_code`,`kind`,`code`),
  KEY `idx_addon_capability_approved` (`addon_code`,`is_approved`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `addon_hook_out` (
  `id` int NOT NULL AUTO_INCREMENT,
  `addon_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `evento` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `empresa` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `payload_json` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `intentos` int NOT NULL DEFAULT '0',
  `status` varchar(12) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `ultimo_error` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `proximo` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `creado` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `enviado` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_addon_hooks_pend` (`status`,`proximo`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `addon_role_permissions` (
  `addon_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `permission_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `role_id` int NOT NULL,
  `is_allowed` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`addon_code`,`permission_code`,`role_id`),
  KEY `idx_addon_role_permission_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `addons` (
  `id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'code del addon, igual al nombre del directorio',
  `name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'nombre visible',
  `version` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '1.0.0',
  `description` text COLLATE utf8mb4_unicode_ci,
  `author_name` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `author_email` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `license_type` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT 'propietario',
  `addon_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'ruta relativa a addons/',
  `entrypoint` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'binario a ejecutar (si tiene UI/API propia)',
  `is_active` tinyint(1) DEFAULT '0' COMMENT '0=descubierto, 1=activo',
  `web` tinyint(1) DEFAULT '0' COMMENT 'tiene templates web',
  `grpc` tinyint(1) DEFAULT '0' COMMENT 'implementa servidor gRPC',
  `needs_migration` tinyint(1) DEFAULT '0' COMMENT 'tiene migraciones pendientes',
  `manifest_json` longtext COLLATE utf8mb4_unicode_ci COMMENT 'manifiesto verificado del addon',
  `installed_at` timestamp NULL DEFAULT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'detected',
  `binary_sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `signature` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `permissions_json` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `last_error` text COLLATE utf8mb4_unicode_ci,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `category` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'general',
  `exclusive_in_category` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_addons_active` (`is_active`),
  KEY `idx_addons_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_agent` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `provider_id` int NOT NULL,
  `model_override` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `role_id` int NOT NULL,
  `operator_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `max_steps` int NOT NULL DEFAULT '8',
  `max_rows` int NOT NULL DEFAULT '500',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_by` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `department` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `config_prompt` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `config_prompt_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_agent_user` (`user_id`),
  KEY `idx_agent_department` (`department`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_cache` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '0',
  `cache_key` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL,
  `operation_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `response_json` longtext COLLATE utf8mb4_unicode_ci NOT NULL,
  `hit_count` int NOT NULL DEFAULT '0',
  `expires_at` datetime NOT NULL,
  `created_at` datetime NOT NULL,
  `last_hit_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_ai_cache_key` (`company_id`,`cache_key`),
  KEY `idx_ai_cache_expires` (`expires_at`),
  KEY `idx_ai_cache_type` (`company_id`,`operation_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_department_prompt` (
  `department` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `prompt` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `label` varchar(150) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`department`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_legal_rule` (
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

CREATE TABLE `ai_op_template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '0',
  `agent_id` int NOT NULL DEFAULT '0',
  `operation` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `template_key` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL,
  `pattern` mediumtext COLLATE utf8mb4_unicode_ci NOT NULL,
  `var_schema` mediumtext COLLATE utf8mb4_unicode_ci NOT NULL,
  `usos` int NOT NULL DEFAULT '0',
  `needs_human` tinyint NOT NULL DEFAULT '1',
  `creado_en` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `last_usado_en` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_op_template_key` (`agent_id`,`operation`,`template_key`),
  KEY `idx_op_template_lru` (`agent_id`,`last_usado_en`),
  KEY `idx_op_template_expira` (`last_usado_en`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_operations` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '0',
  `operation_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `was_cache_hit` tinyint NOT NULL DEFAULT '0',
  `ops_consumed` int NOT NULL DEFAULT '1',
  `prompt_tokens` int NOT NULL DEFAULT '0',
  `completion_tokens` int NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_aiops_company_day` (`company_id`,`created_at`),
  KEY `idx_aiops_month` (`company_id`,`operation_type`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_providers` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `base_url` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `api_key` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `model` varchar(150) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `temperature` float NOT NULL DEFAULT '0.7',
  `max_tokens` int NOT NULL DEFAULT '400',
  `prompt_template` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `is_active` tinyint NOT NULL DEFAULT '0',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `vendor` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'openai',
  `op_cache` tinyint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=31 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_report_types` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=16 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_run_slot` (
  `slot_key` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `task_id` int NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`slot_key`),
  KEY `idx_ai_run_slot_task` (`task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_setting` (
  `setting_id` int NOT NULL AUTO_INCREMENT,
  `api_key` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `model` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT '1',
  `temperature` float DEFAULT '0.7',
  `max_tokens` int DEFAULT '400',
  `prompt_template` text COLLATE utf8mb4_unicode_ci,
  `language` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`setting_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_task` (
  `id` int NOT NULL AUTO_INCREMENT,
  `agent_id` int NOT NULL,
  `name` varchar(150) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `instructions` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `schedule_type` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'manual',
  `schedule_value` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `created_by` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_task_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_task_run` (
  `id` int NOT NULL AUTO_INCREMENT,
  `task_id` int NOT NULL,
  `agent_id` int NOT NULL,
  `status` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'running',
  `started_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `finished_at` datetime DEFAULT NULL,
  `steps` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `final_report` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `undo_journal` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `needs_human` tinyint(1) NOT NULL DEFAULT '0',
  `reviewed_by` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reviewed_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_run_task` (`task_id`),
  KEY `idx_run_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_tool_execution` (
  `id` int NOT NULL AUTO_INCREMENT,
  `idem_key` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tool` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `agent_id` int NOT NULL DEFAULT '0',
  `run_id` int NOT NULL DEFAULT '0',
  `task_id` int NOT NULL DEFAULT '0',
  `external_ref` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(12) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'claimed',
  `result_json` mediumtext COLLATE utf8mb4_unicode_ci,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_tool_idem` (`idem_key`),
  KEY `idx_ai_tool_run` (`run_id`),
  KEY `idx_ai_tool_ref` (`tool`,`external_ref`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_tool_proposal` (
  `id` int NOT NULL AUTO_INCREMENT,
  `idem_key` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tool` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `args_json` mediumtext COLLATE utf8mb4_unicode_ci,
  `run_id` int NOT NULL DEFAULT '0',
  `task_id` int NOT NULL DEFAULT '0',
  `agent_id` int NOT NULL DEFAULT '0',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `reason` text COLLATE utf8mb4_unicode_ci,
  `result_json` mediumtext COLLATE utf8mb4_unicode_ci,
  `requested_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `applied_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `applied_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_proposal_idem` (`idem_key`),
  KEY `idx_ai_proposal_run` (`run_id`,`status`),
  KEY `idx_ai_proposal_agent` (`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ai_usage` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '0',
  `provider_id` int NOT NULL DEFAULT '0',
  `vendor` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `model` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `agent_id` int NOT NULL DEFAULT '0',
  `agent_name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `source` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `prompt_tokens` int NOT NULL DEFAULT '0',
  `completion_tokens` int NOT NULL DEFAULT '0',
  `total_tokens` int NOT NULL DEFAULT '0',
  `requests` int NOT NULL DEFAULT '1',
  `estimated_cost_usd` double NOT NULL DEFAULT '0',
  `ok` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_day` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_ai_usage_day` (`created_day`),
  KEY `idx_ai_usage_company` (`company_id`),
  KEY `idx_ai_usage_provider` (`provider_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `api_keys` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `api_key` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
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

CREATE TABLE `app_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `localhserver` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `onlineserver` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `hotspot` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `default_currency` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `exchange_rate_auto` tinyint(1) DEFAULT '1',
  `multi_currency_enabled` tinyint(1) DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `attendance` (
  `att_id` int NOT NULL AUTO_INCREMENT,
  `employee_id` int NOT NULL,
  `date` date NOT NULL,
  `sign_in` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `sign_out` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `staytime` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`att_id`),
  KEY `employee_id` (`employee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `audit_chain_blocks` (
  `period` varchar(7) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'YYYY-MM',
  `prev_block_hash` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'block_hash del bloque anterior',
  `start_hash` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `end_hash` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `first_id` int DEFAULT NULL COMMENT 'primer id de audit_log del bloque',
  `last_id` int DEFAULT NULL COMMENT 'último id de audit_log del bloque',
  `record_count` int NOT NULL DEFAULT '0',
  `block_hash` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'sello del bloque',
  `sealed_at` datetime DEFAULT NULL,
  PRIMARY KEY (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `audit_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `action` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'create, update, delete, print, anulate',
  `module` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `table_name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `record_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `old_value` text COLLATE utf8mb4_unicode_ci,
  `new_value` text COLLATE utf8mb4_unicode_ci,
  `ip_address` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `prev_hash` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `record_hash` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `module` (`module`),
  KEY `record_id` (`record_id`),
  KEY `idx_audit_created` (`created_at`),
  KEY `idx_audit_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `bank_add` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bank_id` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `bank_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ac_name` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ac_number` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `branch` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `signature_pic` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `bank_information` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bank_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `bank_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ac_name` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ac_number` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `branch` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `swift_code` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ifsc_code` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opening_balance` decimal(18,2) NOT NULL DEFAULT '0.00',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `bank_id` (`bank_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `bank_transaction` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bank_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` date NOT NULL,
  `transaction_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'deposit,withdraw,transfer',
  `amount` decimal(18,2) NOT NULL,
  `reference` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` text COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `branches` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` int NOT NULL DEFAULT '1',
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `phone` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `email` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_active` tinyint(1) DEFAULT '1',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`),
  KEY `idx_company` (`company_id`),
  KEY `idx_active` (`is_active`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cash_registers` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `printer_port` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'COM1',
  `printer_brand` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'epson',
  `printer_model` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'TM-2000',
  `serial_port` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'COM1',
  `baud_rate` int DEFAULT '9600',
  `fiscal_enabled` tinyint(1) DEFAULT '1',
  `status` enum('active','inactive','maintenance') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'active',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_branch_code` (`branch_id`,`code`),
  KEY `idx_branch` (`branch_id`),
  KEY `idx_status` (`status`),
  CONSTRAINT `cash_registers_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cash_session` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cashier` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Usuario cajero',
  `warehouse_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opening_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Fondo inicial',
  `opening_amount_usd` decimal(18,2) DEFAULT '0.00',
  `closing_amount` decimal(18,2) DEFAULT NULL COMMENT 'Efectivo contado al cierre',
  `closing_amount_usd` decimal(18,2) DEFAULT NULL,
  `expected_cash` decimal(18,2) DEFAULT NULL COMMENT 'Efectivo esperado por sistema',
  `expected_cash_usd` decimal(18,2) DEFAULT NULL,
  `difference` decimal(18,2) DEFAULT NULL COMMENT 'Sobrante/faltante',
  `difference_usd` decimal(18,2) DEFAULT NULL,
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '1=abierta, 2=cerrada',
  `currency_` varchar(5) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `opened_at` datetime NOT NULL,
  `closed_at` datetime DEFAULT NULL,
  `open_note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `close_note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `closure_type` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supervisor_id` int DEFAULT NULL,
  `approved_at` datetime DEFAULT NULL,
  `integrity_hash` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_cs_cashier_status` (`cashier`,`status`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cash_session_payment` (
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

CREATE TABLE `cashhea_installment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cashhea_session_id` int NOT NULL COMMENT 'Referencia a cashhea_session',
  `installment_number` int NOT NULL DEFAULT '1' COMMENT 'Número de cuota (1, 2, 3...)',
  `amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto de esta cuota',
  `due_date` date NOT NULL COMMENT 'Fecha de vencimiento (14 días después anterior)',
  `paid_at` datetime DEFAULT NULL COMMENT 'Cuándo fue pagado realmente',
  `payment_method` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Método de pago de la cuota',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending' COMMENT 'pending/paid/failed',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_chi_session` (`cashhea_session_id`),
  KEY `idx_chi_number` (`installment_number`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Cuotas de pago Cashea';

CREATE TABLE `cashhea_session` (
  `id` int NOT NULL AUTO_INCREMENT,
  `session_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'UUID sesión externa Cashea',
  `pos_session_id` int NOT NULL COMMENT 'Referencia a cash_session POS',
  `client_phone` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Teléfono del cliente Cashea',
  `client_rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'RIF del cliente (opcional)',
  `sale_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto total de la venta',
  `down_payment` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Pago inicial recibido al momento',
  `installments` int NOT NULL DEFAULT '1' COMMENT 'Número de cuotas (1-14 días cada una)',
  `installment_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto de cada cuota',
  `total_fees` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Comisiones totales (venta + adelanto)',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending' COMMENT 'pending/approved/active/completed/failed',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `closed_at` datetime DEFAULT NULL COMMENT 'Cuándo se completaron todas las cuotas',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cashhea_session_id` (`session_id`),
  KEY `idx_cs_pos_session` (`pos_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Sesiones de pago Cashea (BNPL)';

CREATE TABLE `closing_records` (
  `id` int NOT NULL AUTO_INCREMENT,
  `head_code` bigint DEFAULT NULL,
  `opening_balance` decimal(10,2) NOT NULL DEFAULT '0.00',
  `amount_in` decimal(10,2) NOT NULL DEFAULT '0.00',
  `amount_out` decimal(10,2) NOT NULL DEFAULT '0.00',
  `closign_balance` decimal(10,2) NOT NULL DEFAULT '0.00',
  `datetime` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `user_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `coa_catalog` (
  `id` int NOT NULL AUTO_INCREMENT,
  `catalog` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'propio',
  `source` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `applied_at` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `notes` text COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_coa_catalog_single` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `company_information` (
  `company_id` varchar(250) COLLATE utf8mb4_unicode_ci NOT NULL,
  `company_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `email` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `mobile` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `website` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `vat_no` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cr_no` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `upload_path` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Directorio privado de archivos (nombre aleatorio)',
  `status` int NOT NULL,
  `fiscal_address` text COLLATE utf8mb4_unicode_ci COMMENT 'Domicilio fiscal declarado al SENIAT',
  `imprenta_digital` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Imprenta digital autorizada (Providencia SNAT/2024/000102)',
  `invoice_series` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'FACT' COMMENT 'Serie de factura activa',
  `ai_plan` tinyint NOT NULL DEFAULT '1',
  `ai_plan_ops_monthly` int NOT NULL DEFAULT '50',
  `ai_plan_ops_daily` int NOT NULL DEFAULT '20',
  `country_code` char(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'VE'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_center` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'production',
  `overhead_rate_hour` decimal(16,4) NOT NULL DEFAULT '0.0000',
  `overhead_rate_unit` decimal(16,4) NOT NULL DEFAULT '0.0000',
  `active` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_center_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_component` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `period` varchar(7) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `component` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `line_no` int NOT NULL DEFAULT '1',
  `amount` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `quantity` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `unit_cost` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `grade` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `hours` decimal(12,2) NOT NULL DEFAULT '0.00',
  `rate_used` decimal(18,6) NOT NULL DEFAULT '0.000000',
  `source_ref` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_component_linea` (`period`,`product_id`,`component`,`line_no`),
  KEY `idx_cc_periodo` (`period`),
  KEY `idx_cc_producto` (`product_id`,`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_period_close` (
  `period` varchar(7) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'abierto',
  `products_closed` int NOT NULL DEFAULT '0',
  `reopened_reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopened_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reopened_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_project` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `customer_id` bigint DEFAULT NULL,
  `billing_type` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'horas',
  `hourly_rate` decimal(16,4) NOT NULL DEFAULT '0.0000',
  `budget_hours` decimal(12,2) NOT NULL DEFAULT '0.00',
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'activo',
  `starts_on` date DEFAULT NULL,
  `ends_on` date DEFAULT NULL,
  `notes` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_project_code` (`code`),
  KEY `idx_proj_customer` (`customer_id`),
  KEY `idx_proj_status` (`status`)
) ENGINE=InnoDB AUTO_INCREMENT=12 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_snapshot` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `period` varchar(7) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `costing_method` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'PP',
  `final_cost` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `material` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `labor` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `depreciation` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `overhead` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `customs` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `loss_factor` decimal(6,4) NOT NULL DEFAULT '0.0000',
  `calc_method` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_snapshot` (`period`,`product_id`),
  KEY `idx_snapshot_periodo` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_task` (
  `id` int NOT NULL AUTO_INCREMENT,
  `project_id` int NOT NULL,
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `billable_default` tinyint NOT NULL DEFAULT '1',
  `active` tinyint NOT NULL DEFAULT '1',
  `notes` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_task_code` (`project_id`,`code`),
  KEY `idx_task_project` (`project_id`)
) ENGINE=InnoDB AUTO_INCREMENT=8 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cost_timesheet` (
  `id` int NOT NULL AUTO_INCREMENT,
  `employee_id` int NOT NULL,
  `work_date` date NOT NULL,
  `hours` decimal(10,2) NOT NULL DEFAULT '0.00',
  `billable` tinyint NOT NULL DEFAULT '1',
  `change_order_amount` decimal(16,2) NOT NULL DEFAULT '0.00',
  `task_id` int DEFAULT NULL,
  `nonbillable_reason` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `project_id` int DEFAULT NULL,
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `grade` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `notes` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_ts_empleado_fecha` (`employee_id`,`work_date`),
  KEY `idx_ts_proyecto` (`project_id`),
  KEY `idx_ts_fecha` (`work_date`),
  KEY `idx_ts_proyecto_tarea` (`project_id`,`task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `credit_note` (
  `id` int NOT NULL AUTO_INCREMENT,
  `note_no` bigint NOT NULL COMMENT 'Número consecutivo de nota',
  `control_no` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Número de control fiscal de la nota',
  `series` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'NC',
  `invoice_id` bigint NOT NULL COMMENT 'Factura afectada',
  `customer_id` bigint NOT NULL,
  `customer_rif` varchar(12) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date` date NOT NULL,
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Motivo: devolución, descuento posterior, anulación',
  `doc_type` varchar(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '03' COMMENT '01=Factura, 02=ND, 03=NC (libro ventas)',
  `created_by` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_note_no` (`series`,`note_no`),
  KEY `idx_cn_invoice` (`invoice_id`),
  KEY `idx_cn_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `credit_note_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `credit_note_id` int NOT NULL,
  `product_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(250) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `quantity` decimal(18,2) NOT NULL DEFAULT '0.00',
  `rate` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `vat_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_price` decimal(18,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `idx_cnd_note` (`credit_note_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_activity` (
  `id` int NOT NULL AUTO_INCREMENT,
  `contact_id` int NOT NULL DEFAULT '0',
  `opportunity_id` int NOT NULL DEFAULT '0',
  `activity_type` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'llamada' COMMENT 'llamada | email | reunion | visita | tarea | nota',
  `subject` varchar(160) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `body` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `due_date` date DEFAULT NULL,
  `done` int NOT NULL DEFAULT '0',
  `created_by` int NOT NULL DEFAULT '0',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `completed_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_activity_due` (`done`,`due_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_contact` (
  `id` int NOT NULL AUTO_INCREMENT,
  `full_name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `email` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `phone` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `company_name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `position_title` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `customer_id` int DEFAULT NULL COMMENT 'se llena al convertir la oportunidad',
  `supplier_id` int DEFAULT NULL,
  `source` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'otro' COMMENT 'referido | web | feria | red | cold_call | otro',
  `owner_id` int NOT NULL DEFAULT '0' COMMENT 'usuario responsable del seguimiento',
  `status` int NOT NULL DEFAULT '1' COMMENT '1=activo, 0=descartado',
  `notes` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_contact_customer` (`customer_id`),
  KEY `idx_crm_contact_owner` (`owner_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_opportunity` (
  `id` int NOT NULL AUTO_INCREMENT,
  `title` varchar(160) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `contact_id` int NOT NULL,
  `stage_id` int NOT NULL DEFAULT '0',
  `amount` decimal(16,2) NOT NULL DEFAULT '0.00',
  `expected_close` date DEFAULT NULL,
  `owner_id` int NOT NULL DEFAULT '0',
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `lost_reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  KEY `idx_crm_opp_stage` (`stage_id`),
  KEY `idx_crm_opp_contact` (`contact_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_remarketing_campaign` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `segment_id` int NOT NULL DEFAULT '0',
  `template_id` int NOT NULL DEFAULT '0',
  `channel_id` int NOT NULL DEFAULT '0' COMMENT 'notify_channel.id',
  `status` int NOT NULL DEFAULT '0' COMMENT 'borrador=0, enviando=1, enviada=2, cancelada=3',
  `total_recipients` int NOT NULL DEFAULT '0',
  `sent_count` int NOT NULL DEFAULT '0',
  `error_count` int NOT NULL DEFAULT '0',
  `scheduled_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'vacio=inmediato',
  `created_by` int NOT NULL DEFAULT '0',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_camp_status` (`status`),
  KEY `idx_crm_remarketing_camp_segment` (`segment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_remarketing_recipient` (
  `id` int NOT NULL AUTO_INCREMENT,
  `campaign_id` int NOT NULL,
  `contact_id` int NOT NULL DEFAULT '0',
  `customer_id` int NOT NULL DEFAULT '0',
  `recipient` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'destino efectivo (email o teléfono)',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pendiente' COMMENT 'pendiente|enviado|error',
  `error_msg` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `idem_key` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'clave de idempotencia',
  `sent_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_rec_camp` (`campaign_id`),
  KEY `idx_crm_remarketing_rec_status` (`status`),
  KEY `idx_crm_remarketing_rec_contact` (`contact_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_remarketing_segment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'nombre descriptivo del segmento',
  `description` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `rule_type` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'no_purchase_60d' COMMENT 'no_purchase_Nd | opportunity_lost | quote_sent_open | no_activity_Nd | customer_all | contact_all',
  `rule_days` int NOT NULL DEFAULT '0' COMMENT 'días para reglas basadas en tiempo',
  `status` int NOT NULL DEFAULT '1' COMMENT '1=activo, 0=pausado',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_seg_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_remarketing_template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL,
  `subject` varchar(200) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'asunto (email/SMS ignorado)',
  `body` text COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'cuerpo con variables %nombre% %empresa% %telefono%',
  `status` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_tpl_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `crm_stage` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `position` int NOT NULL DEFAULT '0',
  `probability` int NOT NULL DEFAULT '0' COMMENT 'probabilidad de cierre en % (0-100)',
  `is_won` int NOT NULL DEFAULT '0' COMMENT '1 = etapa cerrada ganada',
  `is_lost` int NOT NULL DEFAULT '0',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `currency_tbl` (
  `id` int NOT NULL AUTO_INCREMENT,
  `currency_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `icon` text COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `customer_information` (
  `customer_id` bigint NOT NULL AUTO_INCREMENT,
  `customer_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_address` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `address2` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `customer_mobile` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_email` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `email_address` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `contact` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `phone` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `fax` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `city` text COLLATE utf8mb4_unicode_ci,
  `state` text COLLATE utf8mb4_unicode_ci,
  `zip` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `country` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int NOT NULL COMMENT '1=paid,2=credit',
  `create_date` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `create_by` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'RIF: J/V/G/E + 8 dígitos + dígito verificador',
  `customer_type` tinyint NOT NULL DEFAULT '1' COMMENT '1=Contribuyente formal, 2=Consumidor final, 3=Exento',
  `vat_withholding_agent` tinyint NOT NULL DEFAULT '0' COMMENT 'Es agente de retención IVA',
  PRIMARY KEY (`customer_id`),
  KEY `customer_id` (`customer_id`),
  KEY `idx_customer_rif` (`rif`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cycle_count` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `warehouse_id` int DEFAULT NULL,
  `scope` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'category' COMMENT 'category | supplier | location | abc | full',
  `scope_value` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'open' COMMENT 'open | counting | reviewed | closed | cancelled',
  `started_by` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `started_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_by` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `note` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cycle_code` (`code`),
  KEY `idx_cycle_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cycle_count_adjustment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cycle_line_id` int NOT NULL,
  `voucher_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'asiento que corrige el stock',
  `quantity_delta` double NOT NULL DEFAULT '0',
  `value_delta` double NOT NULL DEFAULT '0',
  `reason` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'merma | error de entrada | robo | ajuste',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_adjust_line` (`cycle_line_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cycle_count_line` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cycle_id` int NOT NULL,
  `product_id` int NOT NULL,
  `lot_id` int DEFAULT NULL,
  `expected_quantity` double NOT NULL DEFAULT '0',
  `counted_quantity` double DEFAULT NULL COMMENT 'NULL = aún no contado',
  `variance` double DEFAULT NULL,
  `unit_cost` double NOT NULL DEFAULT '0',
  `variance_value` double NOT NULL DEFAULT '0',
  `counted_by` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `counted_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `adjusted` int NOT NULL DEFAULT '0',
  `note` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cycle_line` (`cycle_id`,`product_id`,`lot_id`),
  KEY `idx_line_cycle` (`cycle_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `daily_banking_add` (
  `banking_id` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` datetime DEFAULT NULL,
  `bank_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `deposit_type` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `transaction_type` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` text COLLATE utf8mb4_unicode_ci,
  `amount` int DEFAULT NULL,
  `status` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `daily_closing` (
  `closing_id` int NOT NULL AUTO_INCREMENT,
  `last_day_closing` float NOT NULL,
  `cash_in` float NOT NULL,
  `cash_out` float NOT NULL,
  `date` varchar(250) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` float NOT NULL,
  `adjustment` float DEFAULT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`closing_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `debit_note` (
  `id` int NOT NULL AUTO_INCREMENT,
  `note_no` bigint NOT NULL COMMENT 'Número consecutivo de nota',
  `control_no` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Número de control fiscal de la nota',
  `series` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'ND',
  `invoice_id` bigint DEFAULT NULL COMMENT 'Factura de origen (opcional)',
  `customer_id` bigint NOT NULL,
  `customer_rif` varchar(12) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date` date NOT NULL,
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Motivo: gasto bancario, mora, diferencia de precio',
  `doc_type` varchar(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '02' COMMENT '01=Factura, 02=ND, 03=NC (libro ventas)',
  `created_by` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_dn_note_no` (`series`,`note_no`),
  KEY `idx_dn_invoice` (`invoice_id`),
  KEY `idx_dn_customer` (`customer_id`),
  KEY `idx_dn_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `debit_note_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `debit_note_id` int NOT NULL,
  `product_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(250) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `quantity` decimal(18,2) NOT NULL DEFAULT '0.00',
  `rate` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `vat_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_price` decimal(18,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `idx_dnd_note` (`debit_note_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `debt_collection` (
  `id` int NOT NULL AUTO_INCREMENT,
  `customer_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `payment_method_id` int DEFAULT NULL,
  `collection_date` date DEFAULT NULL,
  `reference` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `amount_usd` decimal(18,2) DEFAULT '0.00',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  `currency_code` varchar(5) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  PRIMARY KEY (`id`),
  KEY `idx_customer` (`customer_id`),
  KEY `idx_invoice` (`invoice_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `delivery_order` (
  `id` int NOT NULL AUTO_INCREMENT,
  `tenant_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'default',
  `invoice_id` int DEFAULT NULL,
  `customer_id` int NOT NULL,
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT 'WH-MAIN',
  `status` enum('draft','confirmed','picking','packed','shipped','delivered','cancelled') COLLATE utf8mb4_unicode_ci DEFAULT 'draft',
  `date` date NOT NULL,
  `delivered_at` datetime DEFAULT NULL,
  `notes` text COLLATE utf8mb4_unicode_ci,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_do_tenant_status` (`tenant_id`,`status`),
  KEY `idx_do_tenant_invoice` (`tenant_id`,`invoice_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `delivery_order_detail` (
  `id` int NOT NULL AUTO_INCREMENT,
  `tenant_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'default',
  `delivery_order_id` int NOT NULL,
  `product_id` int NOT NULL,
  `quantity` decimal(18,2) NOT NULL,
  `picked_quantity` decimal(18,2) DEFAULT '0.00',
  `packed_quantity` decimal(18,2) DEFAULT '0.00',
  `shipped_quantity` decimal(18,2) DEFAULT '0.00',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_dod` (`delivery_order_id`,`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `designation` (
  `id` int NOT NULL AUTO_INCREMENT,
  `designation` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL,
  `details` text COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `digital_invoice_config` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '1',
  `addon_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_json` json NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_company_addon` (`company_id`,`addon_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `digital_invoice_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` int NOT NULL,
  `company_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '1',
  `addon_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `strong_id` varchar(191) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `nro_control` int DEFAULT NULL,
  `nro_documento` int NOT NULL,
  `tipo_documento` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL,
  `serie` varchar(8) COLLATE utf8mb4_unicode_ci DEFAULT '0',
  `status` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `pdf_url` varchar(512) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `xml_url` varchar(512) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `request_json` json DEFAULT NULL,
  `response_json` json DEFAULT NULL,
  `error_message` text COLLATE utf8mb4_unicode_ci,
  `intentos` int NOT NULL DEFAULT '0',
  `ultimo_intento` timestamp NULL DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `nro_control_texto` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Número de control completo con su serie (CG: 00-00000056)',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_strong_id` (`strong_id`),
  UNIQUE KEY `uk_nro_control` (`nro_control`),
  KEY `idx_invoice` (`invoice_id`),
  KEY `idx_status` (`status`),
  KEY `idx_addon` (`addon_code`),
  KEY `idx_digital_control_texto` (`nro_control_texto`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `email_config` (
  `id` int NOT NULL AUTO_INCREMENT,
  `protocol` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `smtp_host` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `smtp_port` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `smtp_user` varchar(35) COLLATE utf8mb4_unicode_ci NOT NULL,
  `smtp_pass` varchar(35) COLLATE utf8mb4_unicode_ci NOT NULL,
  `mailtype` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `isinvoice` tinyint NOT NULL,
  `isservice` tinyint NOT NULL,
  `isquotation` tinyint NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `employee_history` (
  `id` int NOT NULL AUTO_INCREMENT,
  `first_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `last_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `designation` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `phone` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `rate_type` int NOT NULL,
  `hrate` float NOT NULL,
  `email` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `blood_group` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address_line_1` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `address_line_2` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `image` text COLLATE utf8mb4_unicode_ci,
  `country` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `city` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `zip` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `employee_salary_payment` (
  `emp_sal_pay_id` int NOT NULL AUTO_INCREMENT,
  `generate_id` int NOT NULL,
  `employee_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `total_salary` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_working_minutes` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `working_period` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `payment_due` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `payment_date` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `salary_name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `payment_type` int DEFAULT NULL,
  `bank_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `paid_by` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `salary_month` varchar(70) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`emp_sal_pay_id`),
  KEY `employee_id` (`employee_id`),
  KEY `generate_id` (`generate_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `employee_salary_setup` (
  `e_s_s_id` int unsigned NOT NULL AUTO_INCREMENT,
  `employee_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `sal_type` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `salary_type_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `is_percentage` int DEFAULT NULL COMMENT 'all amount will add or deduct on percent if true ',
  `create_date` date DEFAULT NULL,
  `update_date` datetime(6) DEFAULT NULL,
  `update_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `gross_salary` float NOT NULL,
  PRIMARY KEY (`e_s_s_id`),
  KEY `employee_id` (`employee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `exchange_rate` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'USD',
  `rate_bcv` decimal(18,4) NOT NULL COMMENT 'Tasa oficial BCV',
  `source` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'manual' COMMENT 'api, manual',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `date_currency` (`date`,`currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `exchange_rate_audit` (
  `id` int NOT NULL AUTO_INCREMENT,
  `rate_date` date NOT NULL,
  `currency` varchar(5) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `rate` decimal(18,4) NOT NULL,
  `source` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `applied_by` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `applied_at` datetime NOT NULL,
  `invoice_id` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_audit_date` (`rate_date`,`currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `exchange_rate_current` (
  `currency` varchar(5) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `rate` decimal(18,4) NOT NULL,
  `source` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'manual',
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`currency`),
  KEY `idx_rate_curr` (`currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `exchange_rate_daily` (
  `rate_date` date NOT NULL,
  `currency` varchar(8) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'USD',
  `source` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'BCV',
  `buy_rate` double NOT NULL DEFAULT '0',
  `sell_rate` double NOT NULL DEFAULT '0' COMMENT '0 = no se registró, se usa buy_rate',
  `official_rate` double NOT NULL DEFAULT '0' COMMENT 'tasa oficial del BCV del día',
  `is_parallel` int NOT NULL DEFAULT '0' COMMENT '1 = tasa paralela',
  `entered_by` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`rate_date`,`currency`,`source`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `exchange_rate_source` (
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'BCV | paralelo | oficial | banco',
  `name` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `description` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `active` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `expense` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `type` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `voucher_no` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` float NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `expense_item` (
  `id` int NOT NULL AUTO_INCREMENT,
  `expense_item_name` varchar(200) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `expiry_alert_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `lot_id` int NOT NULL,
  `product_id` int NOT NULL,
  `days_left` int NOT NULL DEFAULT '0',
  `severity` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'info' COMMENT 'info | warning | critical',
  `acknowledged_by` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `acknowledged_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `note` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_alert_lot` (`lot_id`),
  KEY `idx_alert_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `finance_alert` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `scope` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'cxc' COMMENT 'cxc (por cobrar) | cxp (por pagar)',
  `kind` varchar(40) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'motivo de la alerta',
  `severity` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'aviso' COMMENT 'critico | aviso | info',
  `party_type` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'customer' COMMENT 'customer | supplier',
  `party_id` bigint NOT NULL DEFAULT '0',
  `party_name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ref_type` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'invoice | purchase | retention | cartera',
  `ref_id` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `days` int NOT NULL DEFAULT '0' COMMENT 'antiguedad que la disparo',
  `metric_json` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci COMMENT 'los numeros que la sustentan + el motivo redactado',
  `fingerprint` char(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'candado contra alertas repetidas',
  `status` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'abierta' COMMENT 'abierta | cerrada | resuelta | descartada',
  `notified_at` datetime DEFAULT NULL,
  `notify_count` int NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_finance_alert` (`fingerprint`),
  KEY `idx_fa_scope_status` (`scope`,`status`),
  KEY `idx_fa_party` (`party_type`,`party_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `finance_message` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `channel_id` bigint NOT NULL DEFAULT '0',
  `channel` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `channel_kind` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `recipient` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `subject` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `body` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `ref_type` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'alerta | retencion_iva | aviso_cobro | prueba | evento',
  `ref_id` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'enviando' COMMENT 'enviando | enviado | error',
  `error` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `provider_ref` varchar(160) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `idem_key` char(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'la misma accion con los mismos datos no sale dos veces',
  `created_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `sent_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_finance_message_idem` (`idem_key`),
  KEY `idx_fm_ref` (`ref_type`,`ref_id`),
  KEY `idx_fm_status` (`status`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `finance_scan_state` (
  `scope` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `last_run` datetime NOT NULL,
  `last_result` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`scope`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `financial_year` (
  `id` int NOT NULL AUTO_INCREMENT,
  `yearName` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `startDate` date NOT NULL,
  `endDate` date NOT NULL,
  `isCloseReq` tinyint(1) NOT NULL DEFAULT '0',
  `status` tinyint(1) NOT NULL DEFAULT '2' COMMENT '1=active, 0=closed, 2=pending',
  `created_by` int NOT NULL,
  `created_date` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_by` int DEFAULT NULL,
  `updated_date` datetime DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `fiscal_series` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'FACT, NC, ND, RET-IVA, RET-ISLR',
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `prefix` varchar(6) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Prefijo impreso, ej: 00-',
  `next_no` bigint NOT NULL DEFAULT '1' COMMENT 'Próximo número a asignar',
  `control_prefix` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '00' COMMENT 'Prefijo del número de control',
  `control_next` bigint NOT NULL DEFAULT '1' COMMENT 'Próximo número de control',
  `is_active` tinyint NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_series_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=371 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `fiscal_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `key` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `value` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fs_key` (`key`)
) ENGINE=InnoDB AUTO_INCREMENT=12 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `fixed_asset` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'equipo',
  `is_global` tinyint NOT NULL DEFAULT '0',
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cost_center_id` int DEFAULT NULL,
  `purchase_price` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `residual_value` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `purchase_date` date NOT NULL,
  `useful_life_months` int NOT NULL DEFAULT '0',
  `active` tinyint NOT NULL DEFAULT '1',
  `created_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fixed_asset_code` (`code`),
  KEY `idx_fixed_asset_producto` (`product_id`),
  KEY `idx_fixed_asset_centro` (`cost_center_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `fixed_asset_depreciation` (
  `id` int NOT NULL AUTO_INCREMENT,
  `asset_id` int NOT NULL,
  `period` varchar(7) COLLATE utf8mb4_unicode_ci NOT NULL,
  `month_amount` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `accumulated` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `book_value` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'calculado',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fixed_asset_dep` (`asset_id`,`period`),
  KEY `idx_fixed_asset_dep_asset` (`asset_id`),
  KEY `idx_fixed_asset_dep_period` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `fixed_asset_depreciation_movement` (
  `id` int NOT NULL AUTO_INCREMENT,
  `asset_id` int NOT NULL,
  `period` varchar(7) COLLATE utf8mb4_unicode_ci NOT NULL,
  `month_amount` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `accumulated` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `book_value` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'calculado',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fixed_asset_mov` (`asset_id`,`period`),
  KEY `idx_fixed_asset_mov_asset` (`asset_id`),
  KEY `idx_fixed_asset_mov_period` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `gmb_salary_advance` (
  `id` int NOT NULL AUTO_INCREMENT,
  `employee_id` int NOT NULL,
  `salary_month` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'for the month advance will be deducted',
  `amount` decimal(11,0) NOT NULL,
  `release_amount` decimal(11,0) DEFAULT '0',
  `paid` tinyint NOT NULL DEFAULT '0' COMMENT 'paid_to_employee',
  `CreateDate` date NOT NULL,
  `CreateBy` int NOT NULL,
  `UpdateDate` date DEFAULT NULL,
  `UpdateBy` int DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `gmb_salary_generate` (
  `id` int NOT NULL AUTO_INCREMENT,
  `sal_month_year` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `employee_id` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tin_no` int DEFAULT NULL COMMENT 'TIN No',
  `total_attendance` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'tagret_hours / days',
  `total_count` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'weorked_hours / days',
  `atten_allowance` decimal(11,2) DEFAULT NULL COMMENT 'based on taget hours / days',
  `gross` decimal(11,2) NOT NULL COMMENT 'from employee_file table',
  `basic` decimal(11,2) NOT NULL COMMENT 'from employee_file table',
  `transport` decimal(11,2) NOT NULL,
  `house_rent` decimal(11,2) DEFAULT NULL,
  `medical` decimal(11,2) DEFAULT NULL,
  `other_allowance` decimal(11,2) DEFAULT NULL,
  `gross_salary` decimal(11,2) NOT NULL COMMENT 'after adding all allowance with basic',
  `income_tax` decimal(11,2) DEFAULT NULL COMMENT 'from employee_file table it will convert to amount',
  `soc_sec_npf_tax` decimal(11,2) DEFAULT NULL COMMENT 'from employee_file table it will convert to amount',
  `employer_contribution` decimal(11,2) DEFAULT NULL COMMENT '10 % of basic if there soc.sec.tax available ',
  `icf_amount` decimal(11,0) NOT NULL COMMENT 'Id social security tax available',
  `loan_deduct` decimal(11,2) DEFAULT NULL,
  `loan_id` int DEFAULT NULL COMMENT 'From grand_loan table',
  `salary_advance` decimal(11,2) DEFAULT NULL,
  `salary_adv_id` int DEFAULT NULL COMMENT 'From gmb_salary_advance table',
  `lwp` decimal(11,2) DEFAULT NULL COMMENT 'leave without pay	',
  `pf` decimal(11,2) DEFAULT NULL COMMENT 'providend fund',
  `stamp` decimal(11,2) DEFAULT NULL COMMENT 'deduct type',
  `net_salary` decimal(11,2) DEFAULT NULL COMMENT 'after deduct net amount from gross salary',
  `createDate` date DEFAULT NULL,
  `createBy` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `updateDate` date DEFAULT NULL,
  `updateBy` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `medical_benefit` decimal(11,2) DEFAULT NULL,
  `family_benefit` decimal(11,2) DEFAULT NULL,
  `transportation_benefit` decimal(11,2) DEFAULT NULL,
  `other_benefit` decimal(11,2) DEFAULT NULL,
  `normal_working_hrs_month` decimal(20,2) DEFAULT NULL,
  `actual_working_hrs_month` decimal(20,2) DEFAULT NULL,
  `hourly_rate_basic` decimal(20,2) DEFAULT NULL,
  `hourly_rate_trasport_allowance` decimal(20,2) DEFAULT NULL,
  `basic_salary_pro_rated` decimal(20,2) DEFAULT NULL,
  `transport_allowance_pro_rated` decimal(20,2) DEFAULT NULL,
  `basic_transport_allowance` decimal(20,2) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `gmb_salary_sheet_generate` (
  `ssg_id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `gdate` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `start_date` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `end_date` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `generate_by` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'user_id',
  `approved` tinyint(1) DEFAULT '0' COMMENT '1 = approved, 0= not approved',
  `approved_by` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `approved_date` date DEFAULT NULL,
  PRIMARY KEY (`ssg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_attendance` (
  `id` int NOT NULL AUTO_INCREMENT,
  `employee_id` int NOT NULL,
  `work_date` date NOT NULL,
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'present' COMMENT 'present | absent | late | holiday',
  `hours_worked` decimal(8,2) NOT NULL DEFAULT '0.00',
  `note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_hr_attendance` (`employee_id`,`work_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_department` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=185 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_employee` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'código interno del empleado',
  `national_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'cédula',
  `first_name` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `last_name` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `email` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `phone` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `address` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `birth_date` date DEFAULT NULL,
  `hire_date` date NOT NULL,
  `termination_date` date DEFAULT NULL,
  `department_id` int NOT NULL DEFAULT '0',
  `position_id` int NOT NULL DEFAULT '0',
  `base_salary` decimal(14,2) NOT NULL DEFAULT '0.00',
  `pay_frequency` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'quincenal' COMMENT 'quincenal | mensual',
  `bank_code` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `bank_account` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1' COMMENT '1=activo, 0=inactivo, 2=egresado',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_hr_employee_code` (`code`),
  UNIQUE KEY `idx_hr_employee_nid` (`national_id`)
) ENGINE=InnoDB AUTO_INCREMENT=10 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_leave` (
  `id` int NOT NULL AUTO_INCREMENT,
  `employee_id` int NOT NULL,
  `leave_type` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'vacacion',
  `from_date` date NOT NULL,
  `to_date` date NOT NULL,
  `days` decimal(8,2) NOT NULL DEFAULT '0.00',
  `paid` int NOT NULL DEFAULT '1',
  `note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1' COMMENT '1=aprobada, 0=anulada',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_payroll_concept` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `kind` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'earning' COMMENT 'earning | deduction | employer_charge',
  `tax_code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'concept_code de tax_rate_table, si aplica',
  `affects_net` int NOT NULL DEFAULT '1',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1657 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_payroll_item` (
  `id` int NOT NULL AUTO_INCREMENT,
  `run_id` int NOT NULL,
  `employee_id` int NOT NULL,
  `concept_code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `kind` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'earning | deduction | employer_charge',
  `amount` decimal(16,2) NOT NULL DEFAULT '0.00',
  `rate_pct` decimal(8,4) NOT NULL DEFAULT '0.0000' COMMENT 'tasa congelada usada',
  `base_amount` decimal(16,2) NOT NULL DEFAULT '0.00' COMMENT 'base imponible',
  PRIMARY KEY (`id`),
  KEY `idx_hr_item_run` (`run_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_payroll_run` (
  `id` int NOT NULL AUTO_INCREMENT,
  `period_from` date NOT NULL,
  `period_to` date NOT NULL,
  `frequency` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'quincenal',
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'draft' COMMENT 'draft | posted | cancelled',
  `employee_count` int NOT NULL DEFAULT '0',
  `total_gross` decimal(16,2) NOT NULL DEFAULT '0.00',
  `total_deductions` decimal(16,2) NOT NULL DEFAULT '0.00',
  `total_employer` decimal(16,2) NOT NULL DEFAULT '0.00',
  `total_net` decimal(16,2) NOT NULL DEFAULT '0.00',
  `tax_rate_version` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'hash de las tasas usadas: congeladas',
  `created_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `posted_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_payslip` (
  `id` int NOT NULL AUTO_INCREMENT,
  `run_id` int NOT NULL,
  `employee_id` int NOT NULL,
  `net_pay` decimal(16,2) NOT NULL DEFAULT '0.00',
  `issued_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_hr_payslip` (`run_id`,`employee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_position` (
  `id` int NOT NULL AUTO_INCREMENT,
  `department_id` int NOT NULL DEFAULT '0',
  `name` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `base_salary` decimal(14,2) NOT NULL DEFAULT '0.00',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `hr_salary_assignment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `employee_id` int NOT NULL,
  `concept_id` int NOT NULL,
  `amount` decimal(14,2) NOT NULL DEFAULT '0.00',
  `is_percentage` int NOT NULL DEFAULT '0',
  `period` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'quincenal' COMMENT 'quincenal | mensual',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `igtf_transaction` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `purchase_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date` date NOT NULL,
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'USD' COMMENT 'USD, EUR, USDT, BTC',
  `amount_usd` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto original en divisa',
  `rate_bcv` decimal(18,4) NOT NULL DEFAULT '0.0000' COMMENT 'Tasa BCV al momento del pago',
  `igtf_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT '3% calculado',
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `import_cost` (
  `id` int NOT NULL AUTO_INCREMENT,
  `purchase_id` bigint NOT NULL,
  `freight` decimal(16,2) NOT NULL DEFAULT '0.00',
  `customs` decimal(16,2) NOT NULL DEFAULT '0.00',
  `insurance` decimal(16,2) NOT NULL DEFAULT '0.00',
  `other` decimal(16,2) NOT NULL DEFAULT '0.00',
  `received_qty` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `currency` varchar(5) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'USD',
  `exchange_rate` decimal(18,4) NOT NULL DEFAULT '1.0000',
  `notes` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_import_purchase` (`purchase_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `internal_consumption` (
  `id` int NOT NULL AUTO_INCREMENT,
  `consumption_no` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` date NOT NULL,
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `coa_id` int NOT NULL,
  `concept` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `total` decimal(18,2) NOT NULL DEFAULT '0.00',
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_consumption_no` (`consumption_no`),
  KEY `idx_consumption_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `internal_consumption_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `consumption_no` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(18,2) NOT NULL,
  `unit_cost` decimal(18,6) NOT NULL DEFAULT '0.000000',
  `total` decimal(18,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `idx_cons_detail_no` (`consumption_no`),
  KEY `idx_cons_detail_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `inventory_flow_config` (
  `tenant_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `deduction_mode` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'invoice',
  `enable_reservation` tinyint NOT NULL DEFAULT '0',
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`tenant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `invoice` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` bigint NOT NULL,
  `customer_id` bigint NOT NULL,
  `date` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cash_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `card_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `paid_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `due_amount` decimal(10,2) DEFAULT '0.00',
  `prevous_due` decimal(20,2) NOT NULL DEFAULT '0.00',
  `shipping_cost` decimal(10,2) NOT NULL DEFAULT '0.00',
  `invoice` bigint NOT NULL,
  `offline_invoice_no` bigint DEFAULT NULL,
  `invoice_discount` decimal(10,2) DEFAULT '0.00' COMMENT 'invoice discount',
  `total_discount` decimal(10,2) DEFAULT '0.00' COMMENT 'total invoice discount',
  `total_vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'total invoice vat',
  `total_tax` decimal(10,2) DEFAULT '0.00',
  `ret_adjust_amnt` decimal(10,2) DEFAULT NULL,
  `returnable_amount` decimal(10,2) DEFAULT NULL,
  `sales_by` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `delivery_note` text COLLATE utf8mb4_unicode_ci,
  `invoice_details` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_credit` tinyint DEFAULT NULL,
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_draft` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'no',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `status` int NOT NULL,
  `bank_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `payment_type` int NOT NULL,
  `is_online` int NOT NULL DEFAULT '1',
  `control_no` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Número de control fiscal',
  `customer_rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'RIF impreso en la factura',
  `series` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'FACT' COMMENT 'Serie del documento fiscal',
  `igtf_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'IGTF 3% percibido sobre pagos en divisa',
  `igtf_rate` decimal(5,2) NOT NULL DEFAULT '3.00' COMMENT 'Alícuota IGTF aplicada',
  `currency` varchar(5) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'VES' COMMENT 'Moneda de emisión',
  `exchange_rate` decimal(18,4) NOT NULL DEFAULT '1.0000' COMMENT 'Tasa BCV aplicada',
  `is_anulled` tinyint NOT NULL DEFAULT '0' COMMENT '0=viva, 1=anulada (nunca se borra)',
  `anulled_reason` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `anulled_at` datetime DEFAULT NULL,
  `cash_session_id` int DEFAULT NULL COMMENT 'Sesión de caja (turno) que cobró esta venta',
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT 'WH-MAIN' COMMENT 'Almacén de salida',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `total_amount_usd` decimal(18,2) DEFAULT '0.00',
  `exchange_rate_source` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'BCV',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_invoice_number` (`invoice`),
  UNIQUE KEY `uq_invoice_series_control` (`series`,`control_no`),
  KEY `customer_id` (`customer_id`),
  KEY `invoice_id` (`invoice_id`),
  KEY `idx_invoice_control_no` (`control_no`),
  KEY `idx_invoice_date_status` (`date`,`is_anulled`),
  KEY `idx_invoice_cash_session` (`cash_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `invoice_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_details_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ret_invoice_id` bigint DEFAULT NULL,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `serial_no` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `quantity` decimal(10,2) DEFAULT NULL,
  `rate` decimal(10,2) DEFAULT NULL,
  `batch_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supplier_rate` float DEFAULT NULL,
  `total_price` decimal(12,2) DEFAULT NULL,
  `discount` decimal(10,2) DEFAULT NULL,
  `discount_per` varchar(15) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `vat_amnt` decimal(10,2) DEFAULT NULL,
  `vat_amnt_per` varchar(15) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `tax` decimal(10,2) DEFAULT NULL,
  `paid_amount` decimal(12,0) DEFAULT NULL,
  `due_amount` decimal(10,2) DEFAULT NULL,
  `status` int NOT NULL,
  `price_list_id` int NOT NULL DEFAULT '1' COMMENT 'Lista de precio aplicada al renglón',
  `discount_pct` decimal(5,2) DEFAULT '0.00' COMMENT 'Descuento porcentual del renglón',
  `discount_amount` decimal(10,2) DEFAULT '0.00' COMMENT 'Descuento en valor del renglón',
  `base_price` decimal(12,2) DEFAULT NULL COMMENT 'Precio base antes de descuentos',
  PRIMARY KEY (`id`),
  KEY `invoice_id` (`invoice_id`),
  KEY `product_id` (`product_id`),
  KEY `idx_invdet_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `invoice_items` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_code` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sku` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(300) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(15,3) NOT NULL,
  `unit_price` decimal(15,2) NOT NULL,
  `discount_pct` decimal(5,2) DEFAULT '0.00',
  `tax_rate` decimal(5,2) DEFAULT '16.00',
  `subtotal` decimal(15,2) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_invoice` (`invoice_id`),
  CONSTRAINT `invoice_items_ibfk_1` FOREIGN KEY (`invoice_id`) REFERENCES `invoices` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `invoice_payment_summary` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` bigint NOT NULL,
  `cash_session_id` int DEFAULT NULL,
  `method_id` int NOT NULL,
  `amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `currency_code` varchar(5) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `amount_usd` decimal(18,2) DEFAULT '0.00',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_ips` (`invoice_id`,`method_id`),
  KEY `idx_ips_session` (`cash_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `invoice_print_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `printed_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `printed_at` datetime NOT NULL,
  `ip_address` varchar(45) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `invoices` (
  `id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant_code` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_number` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `control_number` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `terminal_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cash_register_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `rif` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_name` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_address` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_phone` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `subtotal` decimal(15,2) NOT NULL DEFAULT '0.00',
  `tax` decimal(15,2) NOT NULL DEFAULT '0.00',
  `total` decimal(15,2) NOT NULL DEFAULT '0.00',
  `tax_rate` decimal(5,2) NOT NULL DEFAULT '16.00',
  `status` enum('draft','printing','printed','cancelled') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'draft',
  `created_at` datetime NOT NULL,
  `printed_at` datetime DEFAULT NULL,
  `fiscal_number` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_tenant` (`tenant_code`),
  KEY `idx_invoice_number` (`invoice_number`),
  KEY `idx_terminal` (`terminal_id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `islr_bracket` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bracket_type` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'tipo1' COMMENT 'tipo1 = deps sin remuneración adicional',
  `from_ut` decimal(14,2) NOT NULL DEFAULT '0.00',
  `to_ut` decimal(14,2) DEFAULT NULL COMMENT 'NULL = sin tope',
  `pct` decimal(8,4) NOT NULL DEFAULT '0.0000',
  `excess_ut` decimal(14,2) NOT NULL DEFAULT '0.00' COMMENT 'exceso gravable en UT dentro del tramo',
  `effective_from` date NOT NULL,
  `effective_to` date DEFAULT NULL,
  `legal_citation` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `islr_concept` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Código oficial SENIAT (001, 002, etc)',
  `description` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `default_percent` decimal(5,2) NOT NULL DEFAULT '5.00' COMMENT '% de retención por defecto',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=16 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `islr_retention` (
  `id` int NOT NULL AUTO_INCREMENT,
  `retention_no` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` date NOT NULL,
  `supplier_id` bigint DEFAULT NULL,
  `concept` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `base_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `retention_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `retention_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `labor_grade` (
  `code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `base_salary_annual` decimal(16,2) NOT NULL DEFAULT '0.00',
  `overhead_pct` decimal(6,2) NOT NULL DEFAULT '0.00',
  `hours_per_year` int NOT NULL DEFAULT '2080',
  `productivity_factor` decimal(6,4) NOT NULL DEFAULT '1.0000',
  `active` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `language` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `phrase` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `english` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1099 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `mcp_tokens` (
  `id` int NOT NULL AUTO_INCREMENT,
  `company_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `agent_id` int NOT NULL DEFAULT '0',
  `name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `token_hash` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `last_used_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `revoked` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mcp_token_hash` (`token_hash`),
  KEY `idx_mcp_token_agent` (`company_code`,`agent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `menu` (
  `id` int NOT NULL AUTO_INCREMENT,
  `parent_id` int DEFAULT NULL,
  `label` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `url` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `icon` varchar(80) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `perm` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `active_prefix` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sort_order` int NOT NULL DEFAULT '0',
  `status` tinyint(1) NOT NULL DEFAULT '1',
  `module_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'core',
  `is_core` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_menu_parent` (`parent_id`,`sort_order`)
) ENGINE=InnoDB AUTO_INCREMENT=9102 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `menu_backup` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `source` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'auto',
  PRIMARY KEY (`id`),
  KEY `idx_menu_backup_source` (`source`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `menu_backup_row` (
  `backup_id` bigint NOT NULL,
  `menu_id` int NOT NULL,
  `parent_id` int DEFAULT NULL,
  `label` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `url` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `icon` varchar(80) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `perm` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `active_prefix` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sort_order` int NOT NULL DEFAULT '0',
  `status` tinyint(1) NOT NULL DEFAULT '1',
  `module_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'core',
  `is_core` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`backup_id`,`menu_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `module` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text COLLATE utf8mb4_unicode_ci,
  `image` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `directory` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` tinyint(1) NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=19 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `module_purchase_key` (
  `id` int NOT NULL AUTO_INCREMENT,
  `identity` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `purchase_key` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `module_registry` (
  `code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `version` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `api` int NOT NULL DEFAULT '1',
  `entry` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `binary_sha256` char(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `rif_binding` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `runtime` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'inproc',
  `signature` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'detected',
  `menu_mid` int DEFAULT NULL,
  `available_version` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `detected_at` datetime NOT NULL,
  `activated_at` datetime DEFAULT NULL,
  `deactivated_at` datetime DEFAULT NULL,
  `last_error` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`code`),
  KEY `idx_estado` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `modules` (
  `id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `version` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '1.0',
  `description` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT '',
  `is_core` tinyint(1) NOT NULL DEFAULT '0',
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `path` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `author` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT '',
  `license_type` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'MIT',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `notas_prueba` (
  `id` int NOT NULL AUTO_INCREMENT,
  `empresa` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `texto` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;

CREATE TABLE `notify_channel` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `kind` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'email' COMMENT 'email | sms | webhook',
  `name` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_active` tinyint NOT NULL DEFAULT '1',
  `ai_allowed` tinyint NOT NULL DEFAULT '0' COMMENT '0 = solo humanos; el administrador lo abre canal por canal',
  `cooldown_days` int NOT NULL DEFAULT '3' COMMENT 'dias minimos entre dos avisos por la misma alerta (0 = sin limite)',
  `host` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `port` int NOT NULL DEFAULT '587',
  `username` varchar(160) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `password` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `from_address` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `from_name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `security` varchar(12) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'starttls' COMMENT 'tls | starttls | none',
  `url` varchar(400) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `http_method` varchar(8) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'POST',
  `auth_header` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'ej. Authorization',
  `auth_token` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sign_secret` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'HMAC-SHA256 del cuerpo (webhook)',
  `notes` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_notify_channel_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `orchestrator_claves` (
  `clave` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `correlativo` int NOT NULL,
  `tabla` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `modulo` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `creada_en` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`clave`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `orchestrator_escrituras` (
  `correlativo` int NOT NULL,
  `clave` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tabla` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `registro_id` int NOT NULL DEFAULT '0',
  `modulo` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `usuario` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ip` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `accion` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `audit_id` int NOT NULL DEFAULT '0',
  `creada_en` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `aplicada_en` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`correlativo`),
  KEY `idx_orch_escrituras_aplicada` (`aplicada_en`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `payment_method` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'cash, punto, pago_movil, zelle, transferencia, cripto, credito',
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_foreign` tinyint NOT NULL DEFAULT '0' COMMENT '1=pago en divisa/cripto → aplica IGTF',
  `coa_id` bigint DEFAULT NULL COMMENT 'Cuenta contable asociada',
  `is_active` tinyint NOT NULL DEFAULT '1',
  `sort_order` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pm_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=19 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `payroll_tax_setup` (
  `tax_setup_id` int unsigned NOT NULL AUTO_INCREMENT,
  `start_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `end_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `rate` decimal(12,2) NOT NULL DEFAULT '0.00',
  `status` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`tax_setup_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `person_information` (
  `id` int NOT NULL AUTO_INCREMENT,
  `person_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_phone` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_address` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `person_ledger` (
  `id` int NOT NULL AUTO_INCREMENT,
  `transaction_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `debit` decimal(12,2) NOT NULL DEFAULT '0.00',
  `credit` decimal(12,2) NOT NULL DEFAULT '0.00',
  `details` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int NOT NULL COMMENT '1=no paid,2=paid',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `balance_usd` decimal(18,2) DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `person_id` (`person_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `personal_loan` (
  `per_loan_id` int NOT NULL AUTO_INCREMENT,
  `transaction_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `debit` decimal(12,2) DEFAULT '0.00',
  `credit` decimal(12,2) NOT NULL DEFAULT '0.00',
  `date` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `details` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int NOT NULL COMMENT '1=no paid,2=paid',
  PRIMARY KEY (`per_loan_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pesonal_loan_information` (
  `id` int NOT NULL AUTO_INCREMENT,
  `person_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_name` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_phone` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `person_address` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `person_id` (`person_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_counters` (
  `name` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `value` bigint NOT NULL DEFAULT '0',
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_fiscal_block_usage` (
  `id` int NOT NULL AUTO_INCREMENT,
  `block_id` int NOT NULL,
  `terminal_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `fiscal_number` bigint NOT NULL,
  `invoice_id` bigint DEFAULT NULL,
  `sale_local_id` int DEFAULT NULL,
  `used_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `synced` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_fiscal_number` (`fiscal_number`),
  KEY `idx_block` (`block_id`),
  KEY `idx_terminal` (`terminal_id`),
  CONSTRAINT `fk_block` FOREIGN KEY (`block_id`) REFERENCES `pos_fiscal_blocks` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_fiscal_blocks` (
  `id` int NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `branch_id` int NOT NULL DEFAULT '0',
  `series` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'FACT',
  `block_start` bigint NOT NULL,
  `block_end` bigint NOT NULL,
  `block_size` int NOT NULL,
  `used_count` int NOT NULL DEFAULT '0',
  `last_used` bigint NOT NULL DEFAULT '0',
  `status` enum('active','exhausted','revoked') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active',
  `reserved_at` datetime NOT NULL,
  `expires_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_terminal_series` (`terminal_id`,`series`,`block_start`),
  KEY `idx_terminal_status` (`terminal_id`,`series`,`status`),
  KEY `idx_expires` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_offline_queue` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `register_id` int NOT NULL,
  `terminal_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `sale_data` json NOT NULL,
  `status` enum('pending','synced','error') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'pending',
  `error_message` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
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

CREATE TABLE `pos_sale_idempotency` (
  `id` int NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Caja que emitió la venta',
  `offline_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Id local de la venta en el terminal',
  `invoice_no` bigint NOT NULL DEFAULT '0' COMMENT 'Número de factura asignado',
  `control_no` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Número de control fiscal asignado',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pos_sale_offline` (`terminal_id`,`offline_id`),
  KEY `idx_pos_sale_invoice` (`invoice_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_sale_sync` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `sale_local_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_id` int DEFAULT NULL,
  `invoice_no` bigint DEFAULT NULL,
  `control_no` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `fiscal_number` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'synced',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pos_sale_sync` (`terminal_id`,`sale_local_id`),
  KEY `idx_pos_sale_sync_invoice` (`invoice_id`),
  KEY `idx_pos_sale_sync_control` (`control_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_sessions` (
  `id` int NOT NULL AUTO_INCREMENT,
  `token` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int NOT NULL,
  `terminal_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
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

CREATE TABLE `pos_shift_sync` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `local_turn_id` bigint NOT NULL,
  `kind` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `remote_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'ok',
  `message` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pos_shift_sync` (`terminal_id`,`local_turn_id`,`kind`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_shifts` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `local_turn_id` bigint NOT NULL,
  `branch_id` int NOT NULL DEFAULT '0',
  `user_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `user_name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supervisor_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supervisor_name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opened_at` datetime DEFAULT NULL,
  `closed_at` datetime DEFAULT NULL,
  `opening_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `opening_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `declared_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `declared_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `system_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `system_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `difference_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `difference_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_sales` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_refunds` decimal(18,2) NOT NULL DEFAULT '0.00',
  `sales_count` int NOT NULL DEFAULT '0',
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'open',
  `closure_type` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `till_purpose` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `remote_session_id` int DEFAULT NULL,
  `synced_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pos_shifts_turn` (`terminal_id`,`local_turn_id`),
  KEY `idx_pos_shifts_status` (`terminal_id`,`status`),
  KEY `idx_pos_shifts_opened` (`opened_at`),
  KEY `idx_pos_shifts_user` (`user_id`),
  KEY `idx_pos_shifts_session` (`remote_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_supervisor_auth` (
  `id` int NOT NULL AUTO_INCREMENT,
  `session_id` int NOT NULL,
  `supervisor_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `auth_type` enum('devolucion','cierre_caja','anulacion') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `auth_time` datetime NOT NULL,
  `reason` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `barcode` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_pos_supervisor_session` (`session_id`),
  KEY `idx_pos_supervisor_user` (`supervisor_id`)
) ENGINE=InnoDB AUTO_INCREMENT=10 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_terminal_sessions` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `register_id` int NOT NULL,
  `api_key_id` int NOT NULL,
  `terminal_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int DEFAULT NULL,
  `user_name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `ip_address` varchar(45) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `started_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `last_activity` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `closed_at` timestamp NULL DEFAULT NULL,
  `status` enum('active','closed','timeout') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'active',
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

CREATE TABLE `pos_turn_events` (
  `id` int NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `branch_id` int DEFAULT NULL,
  `user_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `event_type` enum('login','logout','open_turn','close_turn','devolucion','cierre_caja') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `event_time` datetime NOT NULL,
  `details` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `supervisor_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_pos_events_terminal` (`terminal_id`),
  KEY `idx_pos_events_user` (`user_id`),
  KEY `idx_pos_events_type` (`event_type`),
  KEY `idx_pos_events_time` (`event_time`)
) ENGINE=InnoDB AUTO_INCREMENT=69 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_turn_payments` (
  `id` int NOT NULL AUTO_INCREMENT,
  `turn_id` int NOT NULL,
  `payment_method` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(12,2) NOT NULL,
  `reference` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_turn` (`turn_id`),
  CONSTRAINT `pos_turn_payments_ibfk_1` FOREIGN KEY (`turn_id`) REFERENCES `pos_turns` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pos_turns` (
  `id` int NOT NULL AUTO_INCREMENT,
  `branch_id` int NOT NULL,
  `register_id` int NOT NULL,
  `terminal_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int DEFAULT NULL,
  `user_name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
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
  `status` enum('open','closed','paused') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT 'open',
  `notes` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `idx_branch` (`branch_id`),
  KEY `idx_register` (`register_id`),
  KEY `idx_status` (`status`),
  KEY `idx_user` (`user_id`),
  CONSTRAINT `pos_turns_ibfk_1` FOREIGN KEY (`branch_id`) REFERENCES `branches` (`id`),
  CONSTRAINT `pos_turns_ibfk_2` FOREIGN KEY (`register_id`) REFERENCES `cash_registers` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `price_list` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `sort_order` int NOT NULL DEFAULT '0',
  `is_default` tinyint NOT NULL DEFAULT '0',
  `active` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_price_list_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `print_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `header` int NOT NULL,
  `footer` int NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `process_route` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `sequence` int NOT NULL DEFAULT '0',
  `work_center_id` int NOT NULL,
  `setup_time_min` decimal(10,2) NOT NULL DEFAULT '0.00',
  `run_time_sec` decimal(10,4) NOT NULL DEFAULT '0.0000',
  `machine_time_sec` decimal(10,4) NOT NULL DEFAULT '0.0000',
  `labor_grade` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `active` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  KEY `idx_route_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_category` (
  `category_id` int NOT NULL AUTO_INCREMENT,
  `category_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int NOT NULL,
  `parent_id` int DEFAULT NULL,
  `path` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `level` tinyint NOT NULL DEFAULT '0',
  `sort_order` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`category_id`),
  KEY `idx_cat_parent` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_cost_config` (
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `costing_method` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'PP',
  `loss_factor` decimal(6,4) NOT NULL DEFAULT '0.0000' COMMENT 'Merma global del producto, 0.0250 = 2,5%. Solo afecta al material.',
  `perishable` tinyint NOT NULL DEFAULT '0',
  `shelf_life_days` int NOT NULL DEFAULT '0',
  `is_assembled` tinyint NOT NULL DEFAULT '0',
  `track_lots` tinyint NOT NULL DEFAULT '0',
  `track_serial` tinyint NOT NULL DEFAULT '0',
  `imported` tinyint NOT NULL DEFAULT '0',
  `cost_center_id` int DEFAULT NULL,
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`),
  KEY `idx_pcc_metodo` (`costing_method`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_daily_demand` (
  `product_id` int NOT NULL,
  `demand_date` date NOT NULL,
  `quantity` double NOT NULL DEFAULT '0',
  `source` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'auto' COMMENT 'auto | manual',
  `note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`,`demand_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_information` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `category_id` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `product_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `price` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `unit` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `tax` float DEFAULT NULL COMMENT 'Tax in %',
  `serial_no` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `product_vat` float DEFAULT NULL,
  `product_model` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `product_details` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `image` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int DEFAULT NULL,
  `tax0` text COLLATE utf8mb4_unicode_ci,
  `tax1` text COLLATE utf8mb4_unicode_ci,
  `barcode` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Código de barras (EAN13/Code128/peso)',
  `cost` decimal(18,4) NOT NULL DEFAULT '0.0000' COMMENT 'Costo de reposición',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES' COMMENT 'Moneda del precio base',
  `price_usd` decimal(18,4) DEFAULT NULL COMMENT 'Precio en USD (NULL = calcular por tasa)',
  `item_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'venta',
  `default_warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_product_barcode` (`barcode`),
  KEY `category_id` (`category_id`),
  KEY `product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_lot` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` int NOT NULL,
  `lot_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `entry_date` date NOT NULL,
  `expiry_date` date DEFAULT NULL,
  `quantity_received` double NOT NULL DEFAULT '0',
  `quantity_available` double NOT NULL DEFAULT '0',
  `unit_cost` double NOT NULL DEFAULT '0',
  `supplier_id` int DEFAULT NULL,
  `purchase_id` int DEFAULT NULL,
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT 'active | depleted | expired | quarantined',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_lot_product_code` (`product_id`,`lot_code`),
  KEY `idx_lot_product` (`product_id`),
  KEY `idx_lot_expiry` (`expiry_date`),
  KEY `idx_lot_code` (`product_id`,`lot_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_price_discount` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Producto descontado',
  `customer_id` bigint DEFAULT NULL COMMENT 'Cliente específico (NULL = clase de cliente)',
  `customer_class` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Clase de cliente cuando no va por cliente',
  `price_list_id` int DEFAULT NULL COMMENT 'Lista sobre la que aplica el descuento',
  `discount_pct` decimal(5,2) NOT NULL DEFAULT '0.00' COMMENT 'Descuento en porcentaje',
  `min_qty` decimal(12,3) NOT NULL DEFAULT '0.000' COMMENT 'Cantidad mínima para aplicar',
  `valid_from` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Vigencia desde (YYYY-MM-DD)',
  `valid_until` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Vigencia hasta (YYYY-MM-DD)',
  `reason` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Motivo documentado del descuento',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT 'active / inactive',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ppd_product` (`product_id`),
  KEY `idx_ppd_customer` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_price_list` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `price_list_id` int NOT NULL,
  `price` decimal(16,4) NOT NULL DEFAULT '0.0000',
  `margin_pct` decimal(8,4) NOT NULL DEFAULT '0.0000',
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_producto_lista` (`product_id`,`price_list_id`),
  KEY `idx_ppl_lista` (`price_list_id`)
) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_purchase` (
  `id` int NOT NULL AUTO_INCREMENT,
  `purchase_id` bigint NOT NULL,
  `chalan_no` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `supplier_id` bigint NOT NULL,
  `grand_total_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `paid_amount` decimal(10,2) DEFAULT '0.00',
  `due_amount` decimal(10,2) DEFAULT '0.00',
  `total_discount` decimal(10,2) DEFAULT NULL,
  `invoice_discount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'sum of product discount',
  `total_vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'sum of product vat',
  `purchase_date` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `purchase_details` text COLLATE utf8mb4_unicode_ci,
  `status` int NOT NULL,
  `bank_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `payment_type` int NOT NULL,
  `is_credit` tinyint DEFAULT NULL,
  `control_no` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Número de control de la factura del proveedor',
  `supplier_rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'RIF del proveedor al momento de registrar',
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT 'WH-MAIN' COMMENT 'Almacén de entrada',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  `total_amount_usd` decimal(18,2) DEFAULT '0.00',
  `document_kind` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'factura' COMMENT 'factura | nota_entrega | orden | sin_documento: solo la factura da credito fiscal',
  `purchase_hash` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'identidad de la compra (proveedor + numeros + renglones): candado contra duplicados',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_purchase_hash` (`purchase_hash`),
  KEY `purchase_id` (`purchase_id`),
  KEY `supplier_id` (`supplier_id`),
  KEY `idx_purchase_doc` (`supplier_id`,`control_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_purchase_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `purchase_detail_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `purchase_id` bigint DEFAULT NULL,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `quantity` decimal(10,2) DEFAULT NULL,
  `rate` decimal(10,2) DEFAULT NULL,
  `batch_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `expiry_date` date DEFAULT NULL,
  `total_amount` decimal(10,2) DEFAULT NULL,
  `discount` float DEFAULT NULL COMMENT 'discount percentage',
  `discount_amnt` decimal(10,2) NOT NULL DEFAULT '0.00',
  `vat_amnt_per` decimal(10,2) NOT NULL DEFAULT '0.00',
  `vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00',
  `status` int DEFAULT NULL,
  `item_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'venta',
  `coa_id` int DEFAULT NULL,
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `concept` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `purchase_id` (`purchase_id`),
  KEY `product_id` (`product_id`),
  KEY `idx_ppdet_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_recipe` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `component_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `unit` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `uom` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `version` int NOT NULL DEFAULT '1',
  `loss_factor` decimal(6,4) NOT NULL DEFAULT '0.0000',
  `step_order` int NOT NULL DEFAULT '0',
  `is_substitute` tinyint NOT NULL DEFAULT '0',
  `labor_grade` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `hours` decimal(12,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_recipe` (`product_id`,`component_id`,`step_order`),
  KEY `idx_recipe_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_return` (
  `return_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `purchase_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date_purchase` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `date_return` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `byy_qty` float NOT NULL,
  `ret_qty` float NOT NULL,
  `customer_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `supplier_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_rate` decimal(10,2) NOT NULL DEFAULT '0.00',
  `deduction` float NOT NULL,
  `total_deduct` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_tax` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_ret_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `net_total_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `reason` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `usablity` int NOT NULL,
  KEY `product_id` (`product_id`),
  KEY `invoice_id` (`invoice_id`),
  KEY `purchase_id` (`purchase_id`),
  KEY `customer_id` (`customer_id`),
  KEY `supplier_id` (`supplier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_service` (
  `service_id` int NOT NULL AUTO_INCREMENT,
  `service_name` varchar(250) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `charge` decimal(10,2) NOT NULL DEFAULT '0.00',
  `service_vat` float DEFAULT NULL,
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `tax0` text COLLATE utf8mb4_unicode_ci,
  `tax1` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`service_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_uom_conversion` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `from_uom` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `to_uom` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `factor` decimal(18,6) NOT NULL DEFAULT '0.000000',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_uom_conversion` (`product_id`,`from_uom`,`to_uom`),
  KEY `idx_uom_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `product_yield` (
  `id` int NOT NULL AUTO_INCREMENT,
  `input_product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `output_product_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `expected_pct` decimal(6,4) NOT NULL DEFAULT '0.0000',
  `is_byproduct` tinyint NOT NULL DEFAULT '0',
  `notes` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_yield` (`input_product_id`,`output_product_id`),
  KEY `idx_yield_entrada` (`input_product_id`)
) ENGINE=InnoDB AUTO_INCREMENT=13 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `production_order` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `period` varchar(7) COLLATE utf8mb4_unicode_ci NOT NULL,
  `route_id` int DEFAULT NULL,
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'abierta',
  `started_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_production_order_code` (`code`),
  KEY `idx_production_order_producto` (`product_id`),
  KEY `idx_production_order_periodo` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `production_order_line` (
  `id` int NOT NULL AUTO_INCREMENT,
  `order_id` int NOT NULL,
  `component_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `planned_qty` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `actual_qty` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `unit_cost` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `labor_grade` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `hours` decimal(12,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `idx_production_line_orden` (`order_id`),
  KEY `idx_production_line_componente` (`component_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `purchase_inbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `tenant_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `filename` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `orig_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `mime` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `bytes` bigint NOT NULL DEFAULT '0',
  `formato` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `extraccion` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pendiente_ocr',
  `doc_kind` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'factura',
  `rif` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `proveedor` varchar(200) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `numero` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `control` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `fecha` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `moneda` varchar(12) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `total` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `iva` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `lineas` longtext COLLATE utf8mb4_unicode_ci,
  `leidos` varchar(600) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `notas` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `estado` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'leido',
  `linked_purchase_id` bigint DEFAULT NULL,
  `linked_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_inbox_hash` (`tenant_id`,`sha256`),
  KEY `idx_inbox_tenant` (`tenant_id`,`created_at`),
  KEY `idx_inbox_estado` (`tenant_id`,`estado`),
  KEY `idx_inbox_link` (`linked_purchase_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `queue_tasks` (
  `id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `kind` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `payload` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `attempts` int NOT NULL DEFAULT '0',
  `not_before` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `last_error` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_queue_poll` (`status`,`not_before`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `quot_products_used` (
  `id` int NOT NULL AUTO_INCREMENT,
  `quot_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `serial_no` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `batch_id` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `used_qty` decimal(10,2) DEFAULT NULL,
  `rate` decimal(10,2) DEFAULT NULL,
  `supplier_rate` float DEFAULT NULL,
  `total_price` decimal(12,2) DEFAULT NULL,
  `discount` decimal(10,2) DEFAULT NULL,
  `discount_per` varchar(15) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `vat_amnt` decimal(10,2) DEFAULT NULL,
  `vat_per` decimal(10,2) DEFAULT NULL,
  `tax` decimal(10,2) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `product_id` (`product_id`),
  KEY `quot_id` (`quot_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `quotation` (
  `id` int NOT NULL AUTO_INCREMENT,
  `quotation_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quot_no` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quot_description` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `customer_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quotdate` date NOT NULL,
  `expire_date` date DEFAULT NULL,
  `item_total_amount` decimal(12,2) NOT NULL,
  `item_total_dicount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `item_total_vat` double(10,2) NOT NULL DEFAULT '0.00',
  `item_total_tax` decimal(10,2) DEFAULT '0.00',
  `service_total_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `service_total_discount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `service_total_vat` double(10,2) NOT NULL DEFAULT '0.00',
  `service_total_tax` decimal(10,2) DEFAULT '0.00',
  `quot_dis_item` decimal(10,2) NOT NULL DEFAULT '0.00',
  `quot_dis_service` decimal(10,2) NOT NULL DEFAULT '0.00',
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `create_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `create_date` date NOT NULL,
  `update_by` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `update_date` date DEFAULT NULL,
  `status` tinyint NOT NULL,
  `cust_show` tinyint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `quot_no` (`quot_no`),
  KEY `quotation_id` (`quotation_id`),
  KEY `customer_id` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `quotation_service_used` (
  `id` int NOT NULL AUTO_INCREMENT,
  `quot_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `service_id` int NOT NULL,
  `qty` decimal(10,2) NOT NULL DEFAULT '0.00',
  `charge` decimal(10,2) NOT NULL DEFAULT '0.00',
  `discount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `discount_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `vat_per` decimal(10,2) DEFAULT '0.00',
  `vat_amnt` decimal(10,2) DEFAULT '0.00',
  `tax` decimal(10,2) DEFAULT '0.00',
  `total` decimal(10,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `quot_id` (`quot_id`),
  KEY `service_id` (`service_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `quotation_taxinfo` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `customer_id` int NOT NULL,
  `relation_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tax0` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `tax1` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `customer_id` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `reorder_policy` (
  `product_id` int NOT NULL,
  `lead_time_days` int NOT NULL DEFAULT '7' COMMENT 'plazo del proveedor en días',
  `safety_days` int NOT NULL DEFAULT '2' COMMENT 'días de consumo que se dejan como colchón',
  `min_order_qty` double NOT NULL DEFAULT '0',
  `pack_size` double NOT NULL DEFAULT '1' COMMENT 'múltiplo del pedido: cajas, sacos',
  `avg_daily_demand` double NOT NULL DEFAULT '0' COMMENT 'demanda media, calculada por el sistema',
  `computed_on` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'fecha del último cálculo',
  `active` int NOT NULL DEFAULT '1',
  `note` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `reorder_suggestion` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` int NOT NULL,
  `suggested_date` date NOT NULL,
  `on_hand` double NOT NULL DEFAULT '0',
  `reorder_point` double NOT NULL DEFAULT '0' COMMENT 'stock de seguridad + consumo del plazo',
  `suggested_qty` double NOT NULL DEFAULT '0',
  `pack_size` double NOT NULL DEFAULT '1',
  `estimated_cost` double NOT NULL DEFAULT '0',
  `status` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending' COMMENT 'pending | ordered | dismissed',
  `dismiss_reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_suggestion_product` (`product_id`,`suggested_date`),
  KEY `idx_suggestion_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `report_run` (
  `id` int NOT NULL AUTO_INCREMENT,
  `report_id` int NOT NULL DEFAULT '0',
  `definition_hash` text COLLATE utf8mb4_unicode_ci,
  `sql` mediumtext COLLATE utf8mb4_unicode_ci,
  `params` text COLLATE utf8mb4_unicode_ci,
  `rows_count` int NOT NULL DEFAULT '0',
  `run_by` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `agent_id` int DEFAULT NULL,
  `tenant_code` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL,
  `exported_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_report_run_report` (`report_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `report_template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` varchar(250) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `definition` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_by` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `report_template_acl` (
  `id` int NOT NULL AUTO_INCREMENT,
  `report_template_id` int NOT NULL,
  `role_id` int DEFAULT NULL,
  `user_id` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_rt_acl_report` (`report_template_id`),
  KEY `idx_rt_acl_role` (`role_id`),
  KEY `idx_rt_acl_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `role_permission` (
  `id` int NOT NULL AUTO_INCREMENT,
  `fk_module_id` int NOT NULL,
  `role_id` int NOT NULL,
  `create` tinyint(1) DEFAULT NULL,
  `read` tinyint(1) DEFAULT NULL,
  `update` tinyint(1) DEFAULT NULL,
  `delete` tinyint(1) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_module_id` (`fk_module_id`),
  KEY `fk_user_id` (`role_id`)
) ENGINE=InnoDB AUTO_INCREMENT=339 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `salary_setup_header` (
  `s_s_h_id` int unsigned NOT NULL,
  `employee_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `salary_payable` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `absent_deduct` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `tax_manager` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `salary_sheet_generate` (
  `ssg_id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `gdate` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `start_date` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `end_date` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `generate_by` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`ssg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `salary_type` (
  `salary_type_id` int NOT NULL AUTO_INCREMENT,
  `sal_name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `salary_type` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `emp_sal_type` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `default_amount` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`salary_type_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sec_department` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT '',
  `status` tinyint(1) DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sec_role` (
  `id` int NOT NULL AUTO_INCREMENT,
  `type` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `level` enum('master','admin','manager','employee') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'employee',
  `description` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT '',
  `is_system` tinyint(1) DEFAULT '0',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sec_userrole` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `roleid` int NOT NULL,
  `createby` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `createdate` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `ID` (`id`),
  KEY `idx_sec_userrole_user_id` (`user_id`)
) ENGINE=InnoDB AUTO_INCREMENT=3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `seles_termscondi` (
  `id` int NOT NULL AUTO_INCREMENT,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `service_invoice` (
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

CREATE TABLE `service_invoice_details` (
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

CREATE TABLE `setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `title` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `storename` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `address` text COLLATE utf8mb4_unicode_ci,
  `email` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `phone` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `logo` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `logoweb` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `favicon` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opentime` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `closetime` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `vat` int NOT NULL DEFAULT '0',
  `discount_type` int NOT NULL DEFAULT '0' COMMENT '0=amount,1=percent',
  `service_chargeType` int NOT NULL DEFAULT '0' COMMENT '0=amount,1=percent',
  `currency` int DEFAULT '0',
  `min_prepare_time` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `language` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `timezone` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL,
  `dateformat` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `site_align` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `powerbytxt` text COLLATE utf8mb4_unicode_ci,
  `footer_text` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sms_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `api_key` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `api_secret` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `from` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `isinvoice` int NOT NULL DEFAULT '0',
  `isservice` int NOT NULL DEFAULT '0',
  `isreceive` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `stock_movement` (
  `id` int NOT NULL AUTO_INCREMENT,
  `tenant_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'default',
  `movement_type` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` int NOT NULL,
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT 'WH-MAIN',
  `quantity` decimal(18,2) NOT NULL,
  `unit_cost` decimal(18,4) DEFAULT NULL,
  `reference_type` enum('invoice','purchase','delivery_order','adjustment','return') COLLATE utf8mb4_unicode_ci NOT NULL,
  `reference_id` int NOT NULL,
  `reference_no` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date` date NOT NULL,
  `is_posted` tinyint NOT NULL DEFAULT '1',
  `posted_at` datetime NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_sm_tenant_product_date` (`tenant_id`,`product_id`,`date`),
  KEY `idx_sm_reference` (`reference_type`,`reference_id`),
  KEY `idx_sm_type` (`movement_type`),
  KEY `idx_sm_wh` (`warehouse_id`,`product_id`,`date`),
  KEY `idx_sm_type_date` (`movement_type`,`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `stock_reservation` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'WH-MAIN',
  `quantity` decimal(18,2) NOT NULL DEFAULT '0.00',
  `reference_type` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `reference_id` int NOT NULL DEFAULT '0',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_res_product_status` (`product_id`,`status`),
  KEY `idx_res_warehouse` (`warehouse_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `store_adjustment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `adjustment_no` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `date` date NOT NULL,
  `warehouse_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'WH-MAIN',
  `reference` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `note` text COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `store_adjustment_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `adjustment_no` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `batch_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `expiry_date` date DEFAULT NULL,
  `quantity` decimal(10,2) NOT NULL,
  `type` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'add or remove',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sub_module` (
  `id` int NOT NULL AUTO_INCREMENT,
  `mid` int NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `image` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `directory` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` tinyint(1) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=215 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `supplier_information` (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `supplier_product` (
  `supplier_pr_id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `products_model` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supplier_id` bigint NOT NULL,
  `supplier_price` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`supplier_pr_id`),
  KEY `product_id` (`product_id`),
  KEY `supplier_id` (`supplier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `synchronizer_setting` (
  `id` int NOT NULL,
  `hostname` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `username` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `password` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `port` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `debug` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `project_root` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `tax_collection` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `customer_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `relation_id` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tax0` text COLLATE utf8mb4_unicode_ci,
  `tax1` text COLLATE utf8mb4_unicode_ci,
  PRIMARY KEY (`id`),
  KEY `customer_id` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `tax_rate_table` (
  `concept_code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'IVSS_WORKER, IVSS_EMPLOYER, INCEPS, FAOV, BPSI...',
  `label` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `rate_pct` decimal(8,4) NOT NULL DEFAULT '0.0000' COMMENT 'porcentaje de la base imponible',
  `base_kind` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'salary' COMMENT 'salary | salary_min | n/a',
  `applies_to` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'employer' COMMENT 'worker | employer',
  `effective_from` date NOT NULL,
  `effective_to` date DEFAULT NULL,
  `legal_citation` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'ley/gaceta/fuente que fija la tasa',
  `verified` int NOT NULL DEFAULT '0' COMMENT '1 = validada por el contador de la empresa',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`concept_code`,`effective_from`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `tax_settings` (
  `id` int NOT NULL AUTO_INCREMENT,
  `default_value` float NOT NULL,
  `tax_name` varchar(250) COLLATE utf8mb4_unicode_ci NOT NULL,
  `nt` int NOT NULL,
  `reg_no` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_show` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `tenant_provision_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `tenant_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `action` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `detail` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `tenant_users` (
  `tenant_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `username` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `role_id` int NOT NULL DEFAULT '0',
  `is_owner` int NOT NULL DEFAULT '0',
  `status` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`tenant_code`,`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `tenants` (
  `code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `legal_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `rif` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `db_user` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `db_password_enc` varbinary(512) DEFAULT '',
  `max_users` int NOT NULL DEFAULT '0',
  `created_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `db_size_mb` int NOT NULL DEFAULT '0',
  `db_name` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_demo` tinyint(1) NOT NULL DEFAULT '0',
  `country_code` varchar(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'VE',
  PRIMARY KEY (`code`),
  KEY `idx_tenants_status` (`status`),
  KEY `idx_tenants_created_by` (`created_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `unit_of_measure` (
  `code` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL,
  `allows_decimal` tinyint NOT NULL DEFAULT '1',
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `units` (
  `unit_id` int NOT NULL AUTO_INCREMENT,
  `unit_name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`unit_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `user_company_access` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `role_id` int NOT NULL DEFAULT '2',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_tenant` (`user_id`,`tenant_code`)
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `user_department` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `department_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_dept` (`user_id`,`department_id`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `user_login` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(15) COLLATE utf8mb4_unicode_ci NOT NULL,
  `username` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `password` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `user_type` int DEFAULT NULL,
  `security_code` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=8 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `user_view_pref` (
  `username` varchar(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `view_key` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `name` varchar(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `payload` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `updated_at` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`username`,`view_key`,`name`),
  KEY `idx_user_view_pref_screen` (`view_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `users` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(15) COLLATE utf8mb4_unicode_ci NOT NULL,
  `last_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `first_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `company_name` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `address` text COLLATE utf8mb4_unicode_ci,
  `phone` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `gender` int DEFAULT NULL,
  `date_of_birth` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `logo` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int NOT NULL,
  `photo` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=8 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `vat_retention` (
  `id` int NOT NULL AUTO_INCREMENT,
  `retention_no` varchar(14) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'YYYYMMXXXXXXXX (14 dígitos SENIAT)',
  `period` varchar(6) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'YYYYMM período impositivo',
  `date` date NOT NULL COMMENT 'Fecha de la factura (Col C)',
  `purchase_id` bigint DEFAULT NULL COMMENT 'ID de la compra asociada',
  `supplier_id` bigint DEFAULT NULL COMMENT 'ID del proveedor retenido',
  `invoice_no` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Número de factura (Col G)',
  `control_no` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Número de control fiscal (Col H)',
  `doc_type` varchar(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '01' COMMENT 'Tipo doc: 01=Factura, 02=Nota Débito, 03=Nota Crédito',
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto total documento (Col I)',
  `base_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Base imponible (Col J)',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'IVA calculado',
  `retention_percent` decimal(5,2) NOT NULL DEFAULT '100.00' COMMENT '% de retención aplicada (75%, 100%)',
  `retention_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto IVA retenido (Col K)',
  `affected_doc` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT '0' COMMENT 'Doc afectado para Notas Crédito/Débito (Col L)',
  `exempt_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto exento (Col N)',
  `alicuota` decimal(5,2) NOT NULL DEFAULT '16.00' COMMENT 'Alícuota IVA: 16.00, 8.00, 0.00 (Col O)',
  `expediente` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT '0' COMMENT 'Número de expediente (Col P)',
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `vat_tax_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `dynamic_tax` int NOT NULL DEFAULT '0',
  `fixed_tax` int NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `warehouse` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_default` tinyint NOT NULL DEFAULT '0',
  `is_active` tinyint NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL,
  `kind` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'venta',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_wh_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `web_sessions` (
  `id` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `username` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `fullname` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_level` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `image` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `tenant_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_agent` int NOT NULL DEFAULT '0',
  `csrf_token` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `expires_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_web_sessions_expires` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `web_setting` (
  `setting_id` int NOT NULL AUTO_INCREMENT,
  `logo` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `invoice_logo` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `favicon` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `currency` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `timezone` varchar(150) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_position` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `footer_text` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `language` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `rtr` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `captcha` int DEFAULT '1' COMMENT '0=active,1=inactive',
  `is_qr` int NOT NULL,
  `is_autoapprove_v` int NOT NULL DEFAULT '0',
  `site_key` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `secret_key` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `discount_type` int DEFAULT '1',
  PRIMARY KEY (`setting_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Triggers de protección de borrado (proteccion_borrado.go).
DROP TRIGGER IF EXISTS `erpgo_protege_cliente`;
CREATE TRIGGER `erpgo_protege_cliente` BEFORE DELETE ON `customer_information` FOR EACH ROW
BEGIN
	IF EXISTS(SELECT 1 FROM invoice WHERE customer_id = OLD.customer_id) OR EXISTS(SELECT 1 FROM debt_collection WHERE customer_id = OLD.customer_id) OR EXISTS(SELECT 1 FROM quotation WHERE customer_id = OLD.customer_id) OR EXISTS(SELECT 1 FROM delivery_order WHERE customer_id = OLD.customer_id) THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'No se puede eliminar el cliente: tiene facturas, cobros, cotizaciones u ordenes de entrega registrados. Conserve el registro y corrija sus datos si estan mal.';
	END IF;
END;

DROP TRIGGER IF EXISTS `erpgo_protege_compra`;
CREATE TRIGGER `erpgo_protege_compra` BEFORE DELETE ON `product_purchase` FOR EACH ROW
BEGIN
	IF EXISTS(SELECT 1 FROM product_purchase_details WHERE purchase_id = OLD.purchase_id) OR EXISTS(SELECT 1 FROM acc_vaucher WHERE referenceNo = OLD.purchase_id) OR EXISTS(SELECT 1 FROM acc_transaction WHERE referenceNo = OLD.purchase_id) OR EXISTS(SELECT 1 FROM accounts_payable WHERE purchase_id = OLD.purchase_id) THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'No se puede eliminar la compra: tiene renglones de mercancia, asientos contables o cuentas por pagar asociados. Registre el reverso en lugar de eliminarla.';
	END IF;
END;

DROP TRIGGER IF EXISTS `erpgo_protege_cuenta_por_pagar`;
CREATE TRIGGER `erpgo_protege_cuenta_por_pagar` BEFORE DELETE ON `accounts_payable` FOR EACH ROW
BEGIN
	IF OLD.paid_amount > 0 THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'No se puede eliminar una cuenta por pagar que ya tiene pagos aplicados. Anule el pago con su reverso en lugar de eliminar la cuenta.';
	END IF;
END;

DROP TRIGGER IF EXISTS `erpgo_protege_producto`;
CREATE TRIGGER `erpgo_protege_producto` BEFORE DELETE ON `product_information` FOR EACH ROW
BEGIN
	IF EXISTS(SELECT 1 FROM invoice_details WHERE product_id = OLD.product_id) OR EXISTS(SELECT 1 FROM product_purchase_details WHERE product_id = OLD.product_id) THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'No se puede eliminar el producto: aparece en facturas o en compras ya registradas. Dejelo inactivo en lugar de eliminarlo.';
	END IF;
END;

DROP TRIGGER IF EXISTS `erpgo_protege_proveedor`;
CREATE TRIGGER `erpgo_protege_proveedor` BEFORE DELETE ON `supplier_information` FOR EACH ROW
BEGIN
	IF EXISTS(SELECT 1 FROM product_purchase WHERE supplier_id = OLD.supplier_id) OR EXISTS(SELECT 1 FROM accounts_payable WHERE supplier_id = OLD.supplier_id) OR EXISTS(SELECT 1 FROM vat_retention WHERE supplier_id = OLD.supplier_id) OR EXISTS(SELECT 1 FROM islr_retention WHERE supplier_id = OLD.supplier_id) THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'No se puede eliminar el proveedor: tiene compras, cuentas por pagar o retenciones registradas. Conserve el registro y corrija sus datos si estan mal.';
	END IF;
END;

DROP TRIGGER IF EXISTS `erpgo_protege_servicio`;
CREATE TRIGGER `erpgo_protege_servicio` BEFORE DELETE ON `product_service` FOR EACH ROW
BEGIN
	IF EXISTS(SELECT 1 FROM quotation_service_used WHERE service_id = OLD.service_id) THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'No se puede eliminar el servicio: esta usado en cotizaciones registradas. Dejelo inactivo en lugar de eliminarlo.';
	END IF;
END;
-- Siembras: catálogos y semillas que las migraciones INSERTan.
-- acc_coa (89 filas)
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (8, 50202, 'Accounts Payable', 'Current Liabilities', '502', 3, 1, 0, 1, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 12:52:17', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (10, 1, 'Assets', 'COA', '0', 1, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '1970-01-01 00:00:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (13, 10201, 'Cash', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 12:07:11', '0', '2015-10-15 15:57:55', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (15, 1020101, 'Cash In Hand', 'Cash', '10201', 4, 1, 1, 1, 1, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-12 08:16:03', '0', '2016-05-23 12:05:43', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (16, 102, 'Current Asset', 'Assets', '1', 2, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '1970-01-01 00:00:00', '0', '2018-07-07 11:23:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (17, 502, 'Current Liabilities', 'Liabilities', '5', 2, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '2014-08-30 13:18:20', '0', '2015-10-15 19:49:21', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (23, 401, 'Cost of Goods Solds', 'Expenses', '4', 2, 1, 1, 1, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-02 10:28:34', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (24, 2, 'Shareholder\'s Equity', 'COA', '0', 1, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '1970-01-01 00:00:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (25, 4, 'Expenses', 'COA', '0', 1, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '2', '2019-11-24 05:45:24', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (26, 101, 'Fixed Assets', 'Assets', '1', 2, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '1970-01-01 00:00:00', '0', '2015-10-15 15:29:11', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (27, 402, 'Over Head Cost', 'Expenses', '4', 2, 1, 1, 1, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-04 10:01:58', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (29, 3, 'Income', 'COA', '0', 1, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '1970-01-01 00:00:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (30, 5, 'Liabilities', 'COA', '0', 1, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '2013-07-04 12:32:07', '0', '2015-10-15 19:46:54', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (31, 501, 'Long Term Liabilities', 'Liabilities', '5', 2, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '2014-08-30 13:18:20', '0', '2015-10-15 19:49:21', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (33, 301, 'Direct Income', 'Income', '3', 2, 1, 1, 1, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-02 10:31:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (35, 302, 'Indirect Income', 'Income', '3', 2, 1, 1, 1, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-02 10:55:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', '');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (36, 5020201, 'Supplier Payable', 'Accounts Payable', '50202', 4, 1, 0, 1, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 12:52:44', '0', '1970-01-01 00:00:00', 1, 4, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (41, 10203, 'Prepaid Expenses', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 06:45:19', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (43, 10204, 'Inventory', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 06:48:32', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (44, 50203, 'Accrued Expenses', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 06:50:20', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (46, 50101, 'Long-Term Debt', 'Long Term Liabilities', '501', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 06:51:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (47, 50102, 'Other Long-Term  Liabilities', 'Long Term Liabilities', '501', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 06:53:04', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (48, 201, 'Equity', 'Shareholder\'s Equity', '2', 2, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '2022-04-10 06:56:38', '0', '2022-04-10 06:56:38', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (49, 20101, 'Equity Capital', 'Equity', '201', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 12:31:33', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (50, 20102, 'Retained Earnings', 'Equity', '201', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 07:01:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (51, 10101, 'Property & Equipment', 'Fixed Assets', '101', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:35:53', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (52, 10102, 'Goodwills', 'Fixed Assets', '101', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-23 06:47:21', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (53, 30101, 'Construction Income', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:08:04', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (54, 30102, 'Reimbursement Income', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:09:02', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (55, 40101, 'Cost of Goods Sold', 'Cost of Goods Solds', '401', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:13:52', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (56, 40102, 'Job Expenses', 'Cost of Goods Solds', '401', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:14:18', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (57, 40201, 'Automobile', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:14:37', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (58, 40202, 'Bank Service Charges', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:15:32', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (59, 40203, 'Insurance', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:15:58', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (60, 40204, 'Interest Expenses', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:16:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (61, 40205, 'Payroll Expenses', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:17:08', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (62, 40206, 'Postage', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:17:26', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (63, 40207, 'Professional Fees', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:17:55', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (64, 40208, 'Repairs', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:18:38', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (65, 40209, 'Tools and Macchnery', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:28:02', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (66, 40210, 'Utilities', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 08:28:42', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (67, 4021001, 'Electic Bill', 'Utilities', '40210', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 09:05:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (68, 4021002, 'House Rent', 'Utilities', '40210', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 09:06:05', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (69, 10205, 'Cash at Bank', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 12:44:19', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (75, 1010201, 'Goodwill', 'Goodwills', '10102', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '15.00', '1', '2022-04-23 06:45:48', '0', '1970-01-01 00:00:00', 0, 1, 0, 1, NULL, 'GD001', NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (77, 5020401, 'property sales', 'Unearned Revenue', '50204', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:40:48', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (78, 5010101, 'Debts', 'Long-Term Debt', '50101', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:41:49', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (79, 5010201, 'Other Long-Term  Liabilities', 'Other Long-Term  Liabilities', '50102', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:42:03', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (80, 2010101, 'Capital Fund', 'Equity Capital', '20101', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:42:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (81, 2010201, 'Current year Profit & Loss', 'Retained Earnings', '20102', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:42:53', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (82, 2010202, 'Last year Profit & Loss', 'Retained Earnings', '20102', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:43:03', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (84, 50201, 'Provisions', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-10 11:46:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (85, 5020104, 'Depreciation of Goodwill', 'Depreciations', '50205', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-23 06:47:07', '0', '1970-01-01 00:00:00', 0, 1, 0, 1, NULL, NULL, 'GD001');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (86, 50204, 'Unearned Revenue', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 12:53:09', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (88, 10206, 'Advance', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-11 11:56:56', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (89, 1020601, 'Advance against Employee', 'Advance', '10206', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-11 11:57:18', '0', '1970-01-01 00:00:00', 1, 2, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (90, 1020602, 'Advance Against Customer', 'Advance', '10206', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-11 11:57:35', '0', '1970-01-01 00:00:00', 1, 3, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (91, 1020102, 'Petty Cash', 'Cash', '10201', 4, 1, 0, 0, 1, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-12 08:16:19', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (94, 40301, 'Purchase Account', 'Purchase Accounts', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '2022-04-16 10:31:43', '0', '2022-04-16 10:31:43', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (95, 4030102, 'Purchase', 'Purchase Account', '40201', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-16 10:33:59', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (96, 30103, 'Sales Accounts', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-16 10:34:37', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (97, 3010301, 'Sales Account', 'Sales Accounts', '30103', 4, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-16 10:34:57', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (99, 4020501, 'Salary Expense', 'Payroll Expenses', '40205', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-20 06:24:08', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (104, 5020102, 'Provision for npf contribution', 'Provisions', '50201', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-10 06:19:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (106, 5020101, 'Provision for State Income Tax', 'Provisions', '50201', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-26 06:44:29', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (107, 40211, 'State Income Tax', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-26 06:44:46', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (108, 4021101, 'State Income Tax', 'State Income Tax', '40211', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-04-26 06:45:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (110, 40212, 'Employeer ICF Expense', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-10 06:35:37', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (111, 4021201, 'Employeer 1% ICF Expense', 'Employeer ICF Expense', '40212', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 11:34:02', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (112, 50205, 'Depreciations', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '0', '2022-05-10 09:35:15', '0', '2022-05-10 09:35:15', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (191, 4020502, 'Employee 5 % NPF Expenses', 'Payroll Expenses', '40205', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 11:32:14', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (192, 4020503, 'Employee 10 % NPF Expenses', 'Payroll Expenses', '40205', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-05-17 11:32:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (221, 50206, 'Aoounts pay by name supplier', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-13 11:51:32', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (229, 10208, 'Accounts Receivable', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-16 07:05:42', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (230, 1020801, 'Customer Receivable', 'Accounts Receivable', '10208', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-16 07:07:17', '0', '1970-01-01 00:00:00', 1, 3, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (231, 1020802, 'Employee Receivable', 'Accounts Receivable', '10208', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-16 07:06:58', '0', '1970-01-01 00:00:00', 1, 2, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (232, 1020401, 'Inventory account', 'Inventory', '10204', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-15 07:53:16', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (233, 4010101, 'Cost of Goods Sold Account', 'Cost of Goods Sold', '40101', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-15 08:58:18', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (234, 30104, 'Service Accounts', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-15 16:23:37', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (235, 3010401, 'Service Account', 'Service Accounts', '30104', 4, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2022-06-15 16:24:11', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (241, 1020501, 'fdsfsd', 'Cash at Bank', '10205', 4, 1, 0, 0, 0, 1, 'A', 0, 0, '0', '0', '0', '0', '0.00', '1', '2025-05-12 11:07:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '0', '0', '0');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (242, 1020502, 'test', 'Cash at Bank', '10205', 4, 1, 0, 0, 0, 1, 'A', 0, 0, '0', '0', '0', '0', '0.00', '1', '2025-05-12 11:34:42', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '0', '0', '0');
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (900, 50207, 'Impuestos por Pagar', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:15', '0', '2026-11-27 22:14:15', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (901, 5020701, 'IVA Débito Fiscal por Pagar', 'Impuestos por Pagar', '50207', 4, 1, 0, 1, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:15', '0', '2026-11-27 22:14:15', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (902, 5020702, 'IGTF por Pagar', 'Impuestos por Pagar', '50207', 4, 1, 0, 1, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:15', '0', '2026-11-27 22:14:15', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (903, 1020403, 'Inventario de suministros y consumibles', 'Inventory', '10204', 4, 1, 1, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:17', '', '2026-11-27 22:14:17', 0, 1, 1, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (904, 1020901, 'IVA crédito fiscal (compras)', 'Current Asset', '102', 4, 1, 1, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:17', '', '2026-11-27 22:14:17', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (905, 4021401, 'Gastos de servicios (luz, agua, internet)', 'Over Head Cost', '402', 4, 1, 1, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:17', '', '2026-11-27 22:14:17', 0, 1, 0, 0, NULL, NULL, NULL);
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES (906, 4021402, 'Gastos de suministros y limpieza', 'Over Head Cost', '402', 4, 1, 1, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, '0.00', '1', '2026-11-27 22:14:17', '', '2026-11-27 22:14:17', 0, 1, 0, 0, NULL, NULL, NULL);
-- acc_predefine_account (1 filas)
INSERT INTO `acc_predefine_account` (`id`, `cashCode`, `bankCode`, `advance`, `fixedAsset`, `purchaseCode`, `salesCode`, `serviceCode`, `customerCode`, `supplierCode`, `costs_of_good_solds`, `vat`, `tax`, `inventoryCode`, `CPLCode`, `LPLCode`, `salary_code`, `emp_npf_contribution`, `empr_npf_contribution`, `emp_icf_contribution`, `empr_icf_contribution`, `prov_state_tax`, `state_tax`, `prov_npfcode`, `igtf_code`, `supplies_inventory_code`, `supplies_expense_code`, `service_expense_code`, `purchase_vat_code`) VALUES (2, 10201, 10205, 10206, 101, 1020401, 3010301, 3010401, 1020801, 5020201, 4010101, 5020701, 4021101, 1020401, 2010201, 2010202, 4020501, 4020502, 4020503, 4021201, 0, 5020101, 4021101, 5020102, 5020702, 1020403, 4021402, 4021401, 1020901);
-- acc_subtype (5 filas)
INSERT INTO `acc_subtype` (`id`, `subtypeName`, `staus`, `created_by`, `created_date`) VALUES (1, 'None', 1, 1, '2022-04-05 10:19:27');
INSERT INTO `acc_subtype` (`id`, `subtypeName`, `staus`, `created_by`, `created_date`) VALUES (2, 'Employee', 1, 1, '2022-04-06 08:14:48');
INSERT INTO `acc_subtype` (`id`, `subtypeName`, `staus`, `created_by`, `created_date`) VALUES (3, 'Customer', 1, 1, '2022-04-10 08:49:22');
INSERT INTO `acc_subtype` (`id`, `subtypeName`, `staus`, `created_by`, `created_date`) VALUES (4, 'Supplier', 1, 1, '2022-04-10 08:49:22');
INSERT INTO `acc_subtype` (`id`, `subtypeName`, `staus`, `created_by`, `created_date`) VALUES (6, 'Agent', 1, 1, '2022-04-10 08:50:12');
-- ai_department_prompt (7 filas)
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('contb', 'Tu trabajo es la contabilidad. Cada partida que propongas debe cuadrar: el total de débitos y el total de créditos del asiento son iguales. Cita siempre la cuenta (HeadCode) y su nombre. No cierres un período ni registre un asiento sin que una persona lo pida expresamente.', 'Contabilidad', '', '');
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('cxc', 'Tu trabajo es la cobranza. Al informar, agrupa siempre por cliente y muestra el SALDO PENDIENTE, no solo el total facturado. Cuando pidas datos, prefiere due_amount (lo que se debe) sobre total_amount (lo que se facturó). No prometas fechas de cobro ni negocies plazos: eso lo decide una persona.', 'Cuentas por Cobrar', '', '');
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('cxp', 'Tu trabajo es el pago a proveedores. Al informar, separa lo VENCIDO de lo no vencido y muestra el número de factura del proveedor. Antes de proponer un pago, ten en cuenta el vencimiento real y el saldo pendiente, no solo el monto.', 'Cuentas por Pagar', '', '');
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('fac', 'Tu trabajo es la facturación. Respeta la serie y el correlativo que usa la empresa. No modifiques el IVA calculado por tu cuenta: si el total no cuadra con los renglones, dilo en el informe en lugar de corregirlo.', 'Facturación', '', '');
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('inv', 'Tu trabajo es el inventario. Al informar de existencias, ten en cuenta los lotes con fecha de caducidad: lo que vence primero sale primero (FEFO). Distingue siempre entre la cantidad física contada y la disponible.', 'Inventario', '', '');
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('it', 'Eres de soporte y desarrollo: ves la base entera porque diseñas informes y diagnosticas. Cuando propongas un SQL, explica qué tabla va a consultar y por qué, para que quien lo lea pueda revisarlo antes de aprobarlo.', 'Soporte y Desarrollo', '', '');
INSERT INTO `ai_department_prompt` (`department`, `prompt`, `label`, `updated_by`, `updated_at`) VALUES ('rrhh', 'Tu trabajo es el personal. Son datos reales de personas: muéstralos solo dentro de la empresa y no los reproduzcas en informes que se puedan compartir fuera. Todos los montos a dos decimales.', 'Recursos Humanos', '', '');
-- ai_providers (10 filas)
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (21, 'OpenAI', 'https://api.openai.com/v1', NULL, 'gpt-4o-mini', '0.7', 400, NULL, 1, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (22, 'OpenRouter', 'https://openrouter.ai/api/v1', NULL, 'openai/gpt-4o-mini', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (23, 'Groq', 'https://api.groq.com/openai/v1', NULL, 'llama-3.3-70b-versatile', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (24, 'DeepSeek', 'https://api.deepseek.com/v1', NULL, 'deepseek-chat', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (25, 'Mistral', 'https://api.mistral.ai/v1', NULL, 'mistral-small-latest', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (26, 'xAI', 'https://api.x.ai/v1', NULL, 'grok-2-latest', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (27, 'Together AI', 'https://api.together.xyz/v1', NULL, 'meta-llama/Llama-3.3-70B-Instruct-Turbo', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (28, 'Ollama', 'http://localhost:11434/v1', NULL, 'llama3.2', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (29, 'LM Studio', 'http://localhost:1234/v1', NULL, 'local-model', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'openai', 0);
INSERT INTO `ai_providers` (`id`, `name`, `base_url`, `api_key`, `model`, `temperature`, `max_tokens`, `prompt_template`, `is_active`, `created_at`, `updated_at`, `vendor`, `op_cache`) VALUES (30, 'Personalizado', 'https://api.ejemplo.com/v1', NULL, 'gpt-4o-mini', '0.7', 400, NULL, 0, '2026-11-27 22:14:14', '2026-11-27 22:14:14', 'personalizado', 0);
-- ai_report_types (7 filas)
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (9, 'sales');
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (10, 'purchase');
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (11, 'stock');
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (12, 'sales_return');
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (13, 'supplier_return');
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (14, 'chart');
INSERT INTO `ai_report_types` (`id`, `name`) VALUES (15, 'table_summary');
-- exchange_rate_source (4 filas)
INSERT INTO `exchange_rate_source` (`code`, `name`, `description`, `active`) VALUES ('banco', 'Banco', 'Tasa a la que compra el banco del cliente.', 1);
INSERT INTO `exchange_rate_source` (`code`, `name`, `description`, `active`) VALUES ('BCV', 'Banco Central de Venezuela', 'Tasa oficial publicada por el BCV.', 1);
INSERT INTO `exchange_rate_source` (`code`, `name`, `description`, `active`) VALUES ('oficial', 'Oficial', 'Tasa oficial del día comunicada por el Banco.', 1);
INSERT INTO `exchange_rate_source` (`code`, `name`, `description`, `active`) VALUES ('paralelo', 'Paralelo', 'Tasa del mercado paralelo. Es un dato de referencia, no oficial.', 1);
-- fiscal_setting (6 filas)
INSERT INTO `fiscal_setting` (`id`, `key`, `value`) VALUES (6, 'igtf_enabled', '1');
INSERT INTO `fiscal_setting` (`id`, `key`, `value`) VALUES (7, 'igtf_rate', '3.00');
INSERT INTO `fiscal_setting` (`id`, `key`, `value`) VALUES (8, 'walkin_rif_required', '0');
INSERT INTO `fiscal_setting` (`id`, `key`, `value`) VALUES (9, 'fiscal_footnote', 'Documento fiscal conforme a Ley de IVA y Providencia SNAT/2024/000102');
INSERT INTO `fiscal_setting` (`id`, `key`, `value`) VALUES (10, 'cashhea_enabled', '0');
INSERT INTO `fiscal_setting` (`id`, `key`, `value`) VALUES (11, 'inventory_deduction_mode', 'invoice');
-- inventory_flow_config (1 filas)
INSERT INTO `inventory_flow_config` (`tenant_id`, `deduction_mode`, `enable_reservation`, `updated_at`) VALUES ('default', 'invoice', 0, '2026-11-27 22:14:17');
-- islr_concept (15 filas)
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (1, '001', 'Honorarios Profesionales', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (2, '002', 'Comisiones', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (3, '003', 'Intereses', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (4, '004', 'Arrendamiento Bienes Muebles', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (5, '005', 'Arrendamiento Bienes Inmuebles', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (6, '006', 'Publicidad y Propaganda', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (7, '007', 'Servicios Técnicos', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (8, '008', 'Seguros y Reaseguros', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (9, '009', 'Servicios de Mantenimiento', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (10, '010', 'Servicios de Vigilancia', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (11, '011', 'Servicios de Limpieza', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (12, '012', 'Honorarios de Directores', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (13, '013', 'Arrendamiento de Vehículos', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (14, '014', 'Servicios de Computación', '5.00', 1);
INSERT INTO `islr_concept` (`id`, `code`, `description`, `default_percent`, `status`) VALUES (15, '015', 'Servicios de Publicidad', '5.00', 1);
-- labor_grade (3 filas)
INSERT INTO `labor_grade` (`code`, `name`, `base_salary_annual`, `overhead_pct`, `hours_per_year`, `productivity_factor`, `active`) VALUES ('ayudante', 'Ayudante / junior', '0.00', '34.00', 2080, '1.0000', 1);
INSERT INTO `labor_grade` (`code`, `name`, `base_salary_annual`, `overhead_pct`, `hours_per_year`, `productivity_factor`, `active`) VALUES ('maestro', 'Maestro / especialista', '0.00', '34.00', 2080, '1.0000', 1);
INSERT INTO `labor_grade` (`code`, `name`, `base_salary_annual`, `overhead_pct`, `hours_per_year`, `productivity_factor`, `active`) VALUES ('supervisor', 'Supervisor', '0.00', '34.00', 2080, '1.0000', 1);
-- menu (119 filas)
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (1, NULL, 'Inicio', '/home', 'ti-dashboard', NULL, NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (2, NULL, 'Ventas', NULL, 'fa fa-shopping-bag', NULL, NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (3, NULL, 'Clientes y cobros', NULL, 'fa fa-users', NULL, NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (4, NULL, 'Proveedores', NULL, 'metismenu-icon fa fa-user-secret', NULL, NULL, 40, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5, NULL, 'Productos', NULL, 'fa fa-cubes', NULL, NULL, 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (6, NULL, 'Compras y proveedores', NULL, 'fa fa-shopping-cart', NULL, NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (7, 5000, 'Inventario', '/reports/stock', 'metismenu-icon fa fa-bar-chart', 'stock_report', '/reports/stock', 10, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (8, NULL, 'Servicios', NULL, 'metismenu-icon fa fa-wrench', 'create_service|manage_service', NULL, 80, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (9, 2, 'Devoluciones', '/return/list', 'metismenu-icon fa fa-undo', 'manage_return', NULL, 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (10, 2, 'Notas de Crédito', '/creditnote/list', 'metismenu-icon fa fa-file-text', 'manage_invoice|new_invoice', NULL, 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (12, NULL, 'Cotizaciones', NULL, 'metismenu-icon fa fa-file-text-o', NULL, NULL, 120, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (13, NULL, 'Bancos', NULL, 'metismenu-icon fa fa-university', 'bank|bank_ledger', NULL, 130, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (14, NULL, 'Ajuste Inventario', NULL, 'metismenu-icon fa fa-sliders', 'adjustment', NULL, 140, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (15, NULL, 'Contabilidad', NULL, 'fa fa-calculator', 'coa|vouchers|ledger', NULL, 70, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (16, NULL, 'Reportes', NULL, 'fa fa-files-o', 'sales_report|purchase_report|stock_report', NULL, 90, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (17, NULL, 'Impuestos y libros', NULL, 'fa fa-balance-scale', 'legal_books|kardex', NULL, 80, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (18, NULL, 'CRM', NULL, 'fa fa-briefcase', 'crm', NULL, 100, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (19, NULL, 'Costos', NULL, 'fa fa-percent', 'cost|cost_timesheet|cost_close', NULL, 110, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (20, NULL, 'Recursos Humanos', NULL, 'fa fa-users', 'hr_employee|hr_payroll|hr_tax_rates', NULL, 120, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (21, NULL, 'Configuración', NULL, 'fa fa-cog', 'setting|users|roles|backup', NULL, 140, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (22, NULL, 'Asistentes IA', NULL, 'fa fa-magic', 'ai_settings|ai_business_analyst_report', NULL, 130, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (23, 16, 'Informes Personalizados', '/reports/custom', 'metismenu-icon fa fa-bar-chart', 'custom_reports', '/reports/custom', 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (24, 16, 'Pista de Auditoría', '/reports/audit-log', 'metismenu-icon fa fa-shield', 'audit_log', '/reports/audit-log', 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (101, 2, 'Nueva Factura', '/invoice/new', NULL, 'new_invoice', NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (102, 2, 'Gestionar Facturas', '/invoice/list', NULL, 'manage_invoice', NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (110, 3, 'Añadir Cliente', '/customer/add', NULL, 'add_customer', NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (111, 3, 'Listado de Clientes', '/customer/list', NULL, 'manage_customer', NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (112, 3, 'Clientes Crédito', '/customer/credit', NULL, 'credit_customer', NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (113, 3, 'Clientes Pagados', '/customer/paid', NULL, 'paid_customer', NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (114, 3, 'Libro de clientes', '/customer/ledger', NULL, 'customer_ledger', '/customer/ledger', 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (115, 3, 'Cuentas por Cobrar', '/cartera/list', 'metismenu-icon fa fa-credit-card', 'customer_ledger', '/cartera', 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (120, 6, 'Añadir Proveedor', '/supplier/add', NULL, 'add_supplier', NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (121, 6, 'Listado de Proveedores', '/supplier/list', NULL, 'manage_supplier', NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (122, 6, 'Libro de proveedores', '/supplier/ledger', NULL, 'supplier_ledger', '/supplier/ledger', 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (123, 6, 'Cuentas por Pagar', '/pagar/list', 'metismenu-icon fa fa-money', 'supplier_payment', '/pagar', 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (130, 5, 'Categorías', '/product/category', NULL, 'category', NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (131, 5, 'Unidades', '/product/unit', NULL, 'unit', NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (132, 5, 'Añadir Producto', '/product/add', NULL, 'create_product', NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (133, 5, 'Gestionar Productos', '/product/list', NULL, 'manage_product', NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (140, 6, 'Nueva Compra', '/purchase/add', NULL, 'add_purchase', NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (141, 6, 'Gestionar Compras', '/purchase/list', NULL, 'manage_purchase', NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (150, 2, 'Añadir Servicio', '/service/add', NULL, 'create_service', NULL, 70, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (151, 2, 'Gestionar Servicios', '/service/list', NULL, 'manage_service', NULL, 80, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (152, 2, 'Factura de Servicio', '/service/invoice/new', NULL, 'create_service|manage_service', '/service/invoice', 90, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (170, 2, 'Nueva Cotización', '/quotation/new', NULL, 'create_quotation', NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (171, 2, 'Gestionar Cotizaciones', '/quotation/list', NULL, 'manage_quotation', NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (180, 15, 'Añadir Cuenta', '/bank/add', NULL, 'bank|bank_ledger', NULL, 90, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (181, 15, 'Cuentas Bancarias', '/bank/list', NULL, 'bank|bank_ledger', NULL, 100, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (182, 15, 'Libro Bancario', '/bank/ledger', NULL, 'bank|bank_ledger', '/bank/ledger', 110, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (183, 15, 'Transacción', '/bank/transaction', NULL, 'bank|bank_ledger', NULL, 120, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (190, 5000, 'Nuevo Ajuste', '/adjustment/new', NULL, 'adjustment', NULL, 50, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (191, 5000, 'Historial de ajustes', '/adjustment/list', NULL, 'adjustment', NULL, 60, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (200, 15, 'Plan de Cuentas', '/account/coa', NULL, 'coa', '/account/coa', 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (201, 15, 'Catálogo del plan', '/account/coa/catalog', NULL, 'coa', NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (202, 15, 'Comprobantes', '/account/vouchers', NULL, 'vouchers', NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (203, 15, 'Nuevo Multi-Línea', '/account/voucher/new-multi', NULL, 'vouchers', '/account/voucher', 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (204, 15, 'Libro Mayor', '/account/ledger', NULL, 'ledger', NULL, 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (205, 15, 'Balance Comprobación', '/account/trial-balance', NULL, NULL, NULL, 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (206, 15, 'Estado Resultados', '/account/profit-loss', NULL, NULL, NULL, 70, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (207, 15, 'Balance General', '/account/balance-sheet', NULL, NULL, NULL, 80, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (210, 16, 'Reporte de ventas', '/reports/sales', NULL, NULL, NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (211, 16, 'Reporte de compras', '/reports/purchases', NULL, NULL, NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (212, 5000, 'Inventario', '/reports/stock', NULL, NULL, NULL, 10, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (213, 16, 'Reporte de impuestos', '/reports/tax', NULL, NULL, NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (214, 16, 'Cierre Diario', '/reports/daily-closing', NULL, NULL, NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (220, 17, 'Libro de Compras (IVA)', '/reports/book-purchases', NULL, NULL, NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (221, 17, 'Libro de Ventas (IVA)', '/reports/book-sales', NULL, NULL, NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (222, 17, 'Control de Inventario', '/reports/inventory-control', NULL, NULL, NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (223, 17, 'Libro Entradas y Salidas', '/reports/kardex', NULL, NULL, NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (224, 17, 'Retenciones IVA', '/reports/vat-retention', NULL, NULL, NULL, 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (225, 17, 'Retenciones ISLR', '/reports/islr-retention', NULL, NULL, NULL, 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (226, 17, 'Diferencial Cambiario', '/reports/exchange-difference', NULL, NULL, NULL, 70, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (230, 18, 'Panel', '/crm', NULL, NULL, NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (231, 18, 'Contactos', '/crm/contact/list', NULL, NULL, '/crm/contact', 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (232, 18, 'Embudo', '/crm/pipeline', NULL, NULL, NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (233, 18, 'Remarketing', '/crm/remarketing', NULL, 'crm_remarketing', '/crm/remarketing', 40, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (234, 18, 'Campañas', '/crm/remarketing/campaigns', NULL, 'crm_remarketing', NULL, 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (235, 18, 'Segmentos', '/crm/remarketing/segments', NULL, 'crm_remarketing', NULL, 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (236, 18, 'Plantillas', '/crm/remarketing/templates', NULL, 'crm_remarketing', NULL, 70, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (240, 19, 'Configuración de costos', '/cost', NULL, NULL, NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (241, 19, 'Hoja de horas', '/cost/timesheet', NULL, NULL, '/cost/timesheet', 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (242, 19, 'Horas facturables', '/cost/proyectos', NULL, NULL, '/cost/proyectos', 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (243, 19, 'Cierre de período', '/cost/period', NULL, NULL, '/cost/period', 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (250, 20, 'Panel de RRHH', '/hr', NULL, NULL, NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (251, 20, 'Empleados', '/hr/employee/list', NULL, 'hr_employee', '/hr/employee', 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (252, 20, 'Tasas Fiscales', '/hr/tax-rates', NULL, 'hr_tax_rates', '/hr/tax-rates', 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (253, 20, 'Nómina', '/hr/payroll', NULL, 'hr_payroll', '/hr/payroll', 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (254, 20, 'Vacaciones y ausencias', '/hr/leave', NULL, 'hr_employee|hr_payroll', '/hr/leave', 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (260, 21, 'Software', '/settings', NULL, 'setting', '/settings', 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (261, 21, 'Fiscales (SENIAT)', '/settings/fiscal', NULL, 'setting', '/settings/fiscal', 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (262, 21, 'Empresa', '/settings/company', NULL, 'setting', '/settings/company', 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (263, 21, 'Moneda', '/settings/currency', NULL, 'setting', '/settings/currency', 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (264, 21, 'Tasa BCV', '/settings/exchange-rate', NULL, 'setting', '/settings/exchange-rate', 60, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (265, 21, 'Backup', '/settings/backup', NULL, 'backup', '/settings/backup', 70, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (266, 21, 'Base de Datos', '/settings/database', NULL, 'setting', '/settings/database', 100, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (268, 21, 'Usuarios', '/user/list', NULL, 'users', '/user', 110, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (269, 21, 'Roles y Permisos', '/role/list', NULL, 'roles', '/role', 120, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (270, 22, 'Configuración IA', '/ai/settings', NULL, 'ai_settings', NULL, 10, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (271, 22, 'AI Business Analyst', '/ai/chat', NULL, 'ai_business_analyst_report', NULL, 20, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (272, 22, 'Agentes IA', '/ai/agents', NULL, 'ai_agents', NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (273, 22, 'Tareas de Agentes', '/ai/tasks', NULL, 'ai_tasks', NULL, 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (274, 22, 'Mapa de Base de Datos', '/ai/catalog', NULL, 'ai_agents', NULL, 50, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (275, 21, 'Facturación Digital', '/settings/digital-invoice', NULL, 'setting', '/settings/digital-invoice', 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (276, 21, 'Mensajes enviados', '/finance/mensajes', NULL, 'customer_ledger|supplier_payment', '/finance/mensajes', 80, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (277, 21, 'Canales de aviso', '/settings/notify', NULL, 'setting', '/settings/notify', 35, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (281, 21, 'Gestor de Menús', '/config/menu', 'fa fa-bars', 'setting', '/config/menu', 130, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (285, 21, 'Módulos', '/module/list', 'fa fa-cubes', '__master', '/module', 90, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5000, NULL, 'Inventario y almacén', NULL, 'fa fa-archive', NULL, NULL, 60, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5001, 5000, 'Stock', '/reports/stock', NULL, 'stock_report', '/reports/stock', 10, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5002, 5000, 'Ajuste de Inventario', NULL, NULL, 'adjustment', NULL, 20, 0, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5003, 5000, 'Nuevo Ajuste', '/adjustment/new', NULL, 'adjustment', NULL, 50, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5004, 5000, 'Historial de ajustes', '/adjustment/list', NULL, 'adjustment', NULL, 60, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5005, 5000, 'Almacenes', '/warehouse/list', NULL, NULL, '/warehouse', 20, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5006, 5000, 'Consumo Interno', '/consumption/list', NULL, NULL, '/consumption', 25, 0, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5007, 5000, 'Nuevo consumo', '/consumption/new', NULL, NULL, '/consumption/new', 70, 1, 'core', 1);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5100, 5000, 'Almacén', '/delivery_order/list', 'metismenu-icon fa fa-truck', 'delivery_order|warehouse', NULL, 40, 0, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5101, 5000, 'Nueva Orden', '/delivery_order/add', NULL, NULL, NULL, 30, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (5102, 5000, 'Gestionar Órdenes', '/delivery_order/list', NULL, NULL, '/delivery_order', 40, 1, 'core', 0);
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`) VALUES (9101, 5000, 'Consumos', '/consumption/list', NULL, NULL, NULL, 80, 1, 'core', 1);
-- module (16 filas)
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (1, 'invoice', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (2, 'customer', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (3, 'product', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (4, 'supplier', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (5, 'purchase', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (6, 'stock', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (7, 'return', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (8, 'report', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (9, 'accounts', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (10, 'bank', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (11, 'tax', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (12, 'hrm_management', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (13, 'service', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (14, 'setting', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (15, 'quotation', NULL, NULL, NULL, 1);
INSERT INTO `module` (`id`, `name`, `description`, `image`, `directory`, `status`) VALUES (18, 'ai', 'AI Business Analyst', NULL, NULL, 1);
-- payment_method (8 filas)
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (11, 'cash', 'Efectivo', 0, NULL, 1, 1);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (12, 'punto', 'Punto de Venta', 0, NULL, 1, 2);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (13, 'pago_movil', 'Pago Móvil', 0, NULL, 1, 3);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (14, 'transferencia', 'Transferencia', 0, NULL, 1, 4);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (15, 'zelle', 'Zelle', 1, NULL, 1, 5);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (16, 'cripto', 'Criptoactivo', 1, NULL, 1, 6);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (17, 'credito', 'Crédito', 0, NULL, 1, 7);
INSERT INTO `payment_method` (`id`, `code`, `name`, `is_foreign`, `coa_id`, `is_active`, `sort_order`) VALUES (18, 'cashhea', 'Cashea', 0, NULL, 1, 8);
-- pos_counters (1 filas)
INSERT INTO `pos_counters` (`name`, `value`, `updated_at`) VALUES ('invoice', 999, NULL);
-- price_list (2 filas)
INSERT INTO `price_list` (`id`, `code`, `name`, `sort_order`, `is_default`, `active`) VALUES (3, 'detalle', 'Venta a detalle', 1, 1, 1);
INSERT INTO `price_list` (`id`, `code`, `name`, `sort_order`, `is_default`, `active`) VALUES (4, 'mayor', 'Venta al mayor', 2, 0, 1);
-- sec_role (1 filas)
INSERT INTO `sec_role` (`id`, `type`, `level`, `description`, `is_system`, `created_at`) VALUES (3, 'super', 'master', 'Super Administrador del sistema', 1, '2026-11-27 22:14:14');
-- sub_module (60 filas)
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (1, 1, 'new_invoice', NULL, NULL, 'new_invoice', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (2, 1, 'manage_invoice', NULL, NULL, 'manage_invoice', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (4, 2, 'add_customer', NULL, NULL, 'add_customer', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (5, 2, 'manage_customer', NULL, NULL, 'manage_customer', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (6, 2, 'credit_customer', NULL, NULL, 'credit_customer', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (7, 2, 'paid_customer', NULL, NULL, 'paid_customer', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (8, 2, 'customer_ledger', NULL, NULL, 'customer_ledger', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (9, 2, 'customer_advance', NULL, NULL, 'customer_advance', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (10, 3, 'category', NULL, NULL, 'category', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (11, 3, 'unit', NULL, NULL, 'unit', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (12, 3, 'add_product', NULL, NULL, 'create_product', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (13, 3, 'manage_product', NULL, NULL, 'manage_product', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (14, 4, 'add_supplier', NULL, NULL, 'add_supplier', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (15, 4, 'manage_supplier', NULL, NULL, 'manage_supplier', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (16, 4, 'supplier_ledger', NULL, NULL, 'supplier_ledger', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (17, 5, 'add_purchase', NULL, NULL, 'add_purchase', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (18, 5, 'manage_purchase', NULL, NULL, 'manage_purchase', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (19, 6, 'stock_report', NULL, NULL, 'stock_report', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (20, 9, 'coa', NULL, NULL, 'coa', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (21, 9, 'vouchers', NULL, NULL, 'vouchers', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (22, 9, 'ledger', NULL, NULL, 'ledger', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (23, 8, 'sales_report', NULL, NULL, 'sales_report', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (24, 8, 'purchase_report', NULL, NULL, 'purchase_report', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (25, 14, 'setting', NULL, NULL, 'setting', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (26, 14, 'users', NULL, NULL, 'users', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (27, 14, 'roles', NULL, NULL, 'roles', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (28, 14, 'backup', NULL, NULL, 'backup', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (29, 13, 'create_service', NULL, NULL, 'create_service', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (30, 13, 'manage_service', NULL, NULL, 'manage_service', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (31, 7, 'manage_return', NULL, NULL, 'manage_return', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (32, 15, 'create_quotation', NULL, NULL, 'create_quotation', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (33, 15, 'manage_quotation', NULL, NULL, 'manage_quotation', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (34, 10, 'bank', NULL, NULL, 'bank', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (35, 10, 'bank_ledger', NULL, NULL, 'bank_ledger', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (36, 6, 'adjustment', NULL, NULL, 'adjustment', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (37, 8, 'legal_books', NULL, NULL, 'legal_books', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (38, 8, 'kardex', NULL, NULL, 'kardex', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (39, 14, 'audit_log', NULL, NULL, 'audit_log', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (40, 14, 'exchange_rate', NULL, NULL, 'exchange_rate', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (41, 12, 'hr_employee', NULL, NULL, 'hr_employee', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (42, 12, 'hr_payroll', NULL, NULL, 'hr_payroll', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (43, 12, 'hr_payroll_post', NULL, NULL, 'hr_payroll_post', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (44, 12, 'hr_tax_rates', NULL, NULL, 'hr_tax_rates', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (45, 12, 'crm', NULL, NULL, 'crm', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (46, 12, 'crm_remarketing', NULL, NULL, 'crm_remarketing', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (47, 6, 'consumo_interno', NULL, NULL, 'consumo_interno', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (146, 18, 'ai_settings', NULL, NULL, 'ai_settings', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (147, 18, 'ai_business_analyst_report', NULL, NULL, 'ai_business_analyst_report', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (148, 18, 'ai_agents', NULL, NULL, 'ai_agents', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (149, 18, 'ai_tasks', NULL, NULL, 'ai_tasks', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (150, 18, 'custom_reports', NULL, NULL, 'custom_reports', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (200, 1, 'cost', NULL, NULL, 'cost', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (201, 1, 'cost_timesheet', NULL, NULL, 'cost_timesheet', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (202, 1, 'cost_close', NULL, NULL, 'cost_close', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (203, 4, 'supplier_payment', NULL, NULL, 'supplier_payment', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (210, 1, 'cost_assets', NULL, NULL, 'cost_assets', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (211, 1, 'cost_routes', NULL, NULL, 'cost_routes', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (212, 1, 'cost_uom', NULL, NULL, 'cost_uom', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (213, 1, 'cost_production', NULL, NULL, 'cost_production', 1);
INSERT INTO `sub_module` (`id`, `mid`, `name`, `description`, `image`, `directory`, `status`) VALUES (214, 4, 'supplier_payment', NULL, NULL, 'supplier_payment', 1);
-- unit_of_measure (5 filas)
INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`) VALUES ('g', 'Gramo', 1);
INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`) VALUES ('kg', 'Kilogramo', 1);
INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`) VALUES ('lt', 'Litro', 1);
INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`) VALUES ('ml', 'Mililitro', 1);
INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`) VALUES ('un', 'Unidad', 0);
-- warehouse (2 filas)
INSERT INTO `warehouse` (`id`, `code`, `name`, `address`, `is_default`, `is_active`, `created_at`, `kind`) VALUES (2, 'WH-MAIN', 'Almacén Principal', NULL, 1, 1, '2026-11-27 22:14:13', 'venta');
INSERT INTO `warehouse` (`id`, `code`, `name`, `address`, `is_default`, `is_active`, `created_at`, `kind`) VALUES (3, 'WH-CONSUMO', 'Consumo interno', '', 0, 1, '2026-11-27 22:14:17', 'consumo');
SET FOREIGN_KEY_CHECKS=1;
