SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
START TRANSACTION;
SET time_zone = "+00:00";
SET NAMES utf8mb4;

-- --------------------------------------------------------
-- ERPGo — Tablas adicionales para libros legales venezolanos
-- --------------------------------------------------------

-- Tabla de retenciones de IVA (formato SENIAT: 16 columnas)
CREATE TABLE IF NOT EXISTS `vat_retention` (
  `id` int NOT NULL AUTO_INCREMENT,
  `retention_no` varchar(14) NOT NULL COMMENT 'YYYYMMXXXXXXXX (14 dígitos SENIAT)',
  `period` varchar(6) NOT NULL COMMENT 'YYYYMM período impositivo',
  `date` date NOT NULL COMMENT 'Fecha de la factura (Col C)',
  `purchase_id` bigint DEFAULT NULL COMMENT 'ID de la compra asociada',
  `supplier_id` bigint DEFAULT NULL COMMENT 'ID del proveedor retenido',
  `invoice_no` varchar(50) DEFAULT NULL COMMENT 'Número de factura (Col G)',
  `control_no` varchar(50) DEFAULT NULL COMMENT 'Número de control fiscal (Col H)',
  `doc_type` varchar(2) NOT NULL DEFAULT '01' COMMENT 'Tipo doc: 01=Factura, 02=Nota Débito, 03=Nota Crédito',
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto total documento (Col I)',
  `base_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Base imponible (Col J)',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'IVA calculado',
  `retention_percent` decimal(5,2) NOT NULL DEFAULT '100.00' COMMENT '% de retención aplicada (75%, 100%)',
  `retention_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto IVA retenido (Col K)',
  `affected_doc` varchar(50) DEFAULT '0' COMMENT 'Doc afectado para Notas Crédito/Débito (Col L)',
  `exempt_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto exento (Col N)',
  `alicuota` decimal(5,2) NOT NULL DEFAULT '16.00' COMMENT 'Alícuota IVA: 16.00, 8.00, 0.00 (Col O)',
  `expediente` varchar(20) DEFAULT '0' COMMENT 'Número de expediente (Col P)',
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de retenciones de ISLR
CREATE TABLE IF NOT EXISTS `islr_retention` (
  `id` int NOT NULL AUTO_INCREMENT,
  `retention_no` varchar(30) NOT NULL,
  `date` date NOT NULL,
  `supplier_id` bigint DEFAULT NULL,
  `concept` varchar(255) DEFAULT NULL,
  `base_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `retention_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `retention_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de ajustes de inventario
CREATE TABLE IF NOT EXISTS `store_adjustment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `adjustment_no` varchar(30) NOT NULL,
  `date` date NOT NULL,
  `warehouse_id` varchar(30) DEFAULT NULL,
  `reference` varchar(100) DEFAULT NULL,
  `note` text,
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Detalle de ajustes de inventario
CREATE TABLE IF NOT EXISTS `store_adjustment_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `adjustment_no` varchar(30) NOT NULL,
  `product_id` varchar(30) NOT NULL,
  `batch_id` varchar(30) DEFAULT NULL,
  `expiry_date` date DEFAULT NULL,
  `quantity` decimal(10,2) NOT NULL,
  `type` varchar(10) NOT NULL COMMENT 'add or remove',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de bancos
CREATE TABLE IF NOT EXISTS `bank_information` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bank_id` varchar(30) NOT NULL,
  `bank_name` varchar(255) NOT NULL,
  `ac_name` varchar(250) DEFAULT NULL,
  `ac_number` varchar(250) DEFAULT NULL,
  `branch` varchar(250) DEFAULT NULL,
  `swift_code` varchar(50) DEFAULT NULL,
  `ifsc_code` varchar(50) DEFAULT NULL,
  `opening_balance` decimal(18,2) NOT NULL DEFAULT '0.00',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `bank_id` (`bank_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Transacciones bancarias
CREATE TABLE IF NOT EXISTS `bank_transaction` (
  `id` int NOT NULL AUTO_INCREMENT,
  `bank_id` varchar(30) NOT NULL,
  `date` date NOT NULL,
  `transaction_type` varchar(20) NOT NULL COMMENT 'deposit,withdraw,transfer',
  `amount` decimal(18,2) NOT NULL,
  `reference` varchar(100) DEFAULT NULL,
  `description` text,
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Log de impresión de facturas
CREATE TABLE IF NOT EXISTS `invoice_print_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(30) NOT NULL,
  `printed_by` varchar(30) NOT NULL,
  `printed_at` datetime NOT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de códigos ISLR (Decreto 1808)
CREATE TABLE IF NOT EXISTS `islr_concept` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(10) NOT NULL COMMENT 'Código oficial SENIAT (001, 002, etc)',
  `description` varchar(255) NOT NULL,
  `default_percent` decimal(5,2) NOT NULL DEFAULT '5.00' COMMENT '% de retención por defecto',
  `status` int NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de transacciones IGTF (Impuesto Grandes Transacciones Financieras 3%)
CREATE TABLE IF NOT EXISTS `igtf_transaction` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(30) DEFAULT NULL,
  `purchase_id` varchar(30) DEFAULT NULL,
  `date` date NOT NULL,
  `currency` varchar(5) NOT NULL DEFAULT 'USD' COMMENT 'USD, EUR, USDT, BTC',
  `amount_usd` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT 'Monto original en divisa',
  `rate_bcv` decimal(18,4) NOT NULL DEFAULT '0.0000' COMMENT 'Tasa BCV al momento del pago',
  `igtf_amount` decimal(18,2) NOT NULL DEFAULT '0.00' COMMENT '3% calculado',
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de tasas de cambio BCV
CREATE TABLE IF NOT EXISTS `exchange_rate` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `currency` varchar(5) NOT NULL DEFAULT 'USD',
  `rate_bcv` decimal(18,4) NOT NULL COMMENT 'Tasa oficial BCV',
  `source` varchar(20) NOT NULL DEFAULT 'manual' COMMENT 'api, manual',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `date_currency` (`date`, `currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tabla de auditoría (logs indelebles)
CREATE TABLE IF NOT EXISTS `audit_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(30) NOT NULL,
  `action` varchar(20) NOT NULL COMMENT 'create, update, delete, print, anulate',
  `module` varchar(50) NOT NULL,
  `table_name` varchar(50) DEFAULT NULL,
  `record_id` varchar(50) DEFAULT NULL,
  `old_value` text DEFAULT NULL,
  `new_value` text DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `module` (`module`),
  KEY `record_id` (`record_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- AUTO_INCREMENT
ALTER TABLE `vat_retention` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `islr_retention` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `store_adjustment` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `store_adjustment_details` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `bank_information` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `bank_transaction` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `invoice_print_log` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `islr_concept` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `igtf_transaction` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `exchange_rate` MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `audit_log` MODIFY `id` int NOT NULL AUTO_INCREMENT;
-- Cadena de auditoría: hash previo y hash del registro (append-only)
-- (ADD COLUMN plain: el runner de migraciones ignora "Duplicate column")
ALTER TABLE `audit_log` ADD COLUMN IF NOT EXISTS `prev_hash` varchar(64) NOT NULL DEFAULT '';
ALTER TABLE `audit_log` ADD COLUMN IF NOT EXISTS `record_hash` varchar(64) NOT NULL DEFAULT '';
ALTER TABLE `audit_log` ADD INDEX `idx_audit_created` (`created_at`);
ALTER TABLE `audit_log` ADD INDEX `idx_audit_user` (`user_id`);

-- Seed: Códigos ISLR oficiales SENIAT
INSERT IGNORE INTO `islr_concept` (`code`, `description`, `default_percent`) VALUES
('001', 'Honorarios Profesionales', 5.00),
('002', 'Comisiones', 5.00),
('003', 'Intereses', 5.00),
('004', 'Arrendamiento Bienes Muebles', 5.00),
('005', 'Arrendamiento Bienes Inmuebles', 5.00),
('006', 'Publicidad y Propaganda', 5.00),
('007', 'Servicios Técnicos', 5.00),
('008', 'Seguros y Reaseguros', 5.00),
('009', 'Servicios de Mantenimiento', 5.00),
('010', 'Servicios de Vigilancia', 5.00),
('011', 'Servicios de Limpieza', 5.00),
('012', 'Honorarios de Directores', 5.00),
('013', 'Arrendamiento de Vehículos', 5.00),
('014', 'Servicios de Computación', 5.00),
('015', 'Servicios de Publicidad', 5.00);

COMMIT;
