-- Agregar categoría y exclusividad a addons existentes
ALTER TABLE `addons` ADD COLUMN IF NOT EXISTS `category` VARCHAR(64) NOT NULL DEFAULT 'general';
ALTER TABLE `addons` ADD COLUMN IF NOT EXISTS `exclusive_in_category` TINYINT(1) NOT NULL DEFAULT 0;

-- Tablas para facturación digital
CREATE TABLE IF NOT EXISTS `digital_invoice_log` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `invoice_id` INT NOT NULL,
  `company_id` VARCHAR(30) NOT NULL DEFAULT '1',
  `addon_code` VARCHAR(64) NOT NULL,
  `strong_id` VARCHAR(191) DEFAULT NULL,
  `nro_control` INT DEFAULT NULL,
  `nro_documento` INT NOT NULL,
  `tipo_documento` VARCHAR(8) NOT NULL,
  `serie` VARCHAR(8) DEFAULT '0',
  `status` VARCHAR(32) NOT NULL DEFAULT 'pending',
  `pdf_url` VARCHAR(512) DEFAULT NULL,
  `xml_url` VARCHAR(512) DEFAULT NULL,
  `request_json` JSON DEFAULT NULL,
  `response_json` JSON DEFAULT NULL,
  `error_message` TEXT DEFAULT NULL,
  `intentos` INT NOT NULL DEFAULT 0,
  `ultimo_intento` TIMESTAMP NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_strong_id` (`strong_id`),
  UNIQUE KEY `uk_nro_control` (`nro_control`),
  KEY `idx_invoice` (`invoice_id`),
  KEY `idx_status` (`status`),
  KEY `idx_addon` (`addon_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `digital_invoice_config` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `company_id` VARCHAR(30) NOT NULL DEFAULT '1',
  `addon_code` VARCHAR(64) NOT NULL,
  `config_json` JSON NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_company_addon` (`company_id`, `addon_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
