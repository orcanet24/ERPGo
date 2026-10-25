-- ============================================================================
-- ERPGo — Migración complementaria: sesión de caja en facturas + almacén
-- ============================================================================
ALTER TABLE `invoice` ADD COLUMN IF NOT EXISTS `cash_session_id` INT DEFAULT NULL COMMENT 'Sesión de caja (turno) que cobró esta venta',
  ADD COLUMN `warehouse_id` VARCHAR(30) DEFAULT 'WH-MAIN' COMMENT 'Almacén de salida';
ALTER TABLE `invoice` ADD INDEX `idx_invoice_cash_session` (`cash_session_id`);

ALTER TABLE `product_purchase` ADD COLUMN IF NOT EXISTS `warehouse_id` VARCHAR(30) DEFAULT 'WH-MAIN' COMMENT 'Almacén de entrada';

ALTER TABLE `store_adjustment`
  MODIFY COLUMN `warehouse_id` VARCHAR(30) NOT NULL DEFAULT 'WH-MAIN';

-- Almacén principal por defecto
CREATE TABLE IF NOT EXISTS `warehouse` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(30) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `address` VARCHAR(255) DEFAULT NULL,
  `is_default` TINYINT NOT NULL DEFAULT 0,
  `is_active` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_wh_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `warehouse` (`code`, `name`, `is_default`, `created_at`)
  SELECT 'WH-MAIN', 'Almacén Principal', 1, NOW()
  WHERE NOT EXISTS (SELECT 1 FROM `warehouse` WHERE `code` = 'WH-MAIN');

-- Índice para consultas de stock (kardex/POS)
ALTER TABLE `invoice_details` ADD INDEX `idx_invdet_product` (`product_id`);
ALTER TABLE `product_purchase_details` ADD INDEX `idx_ppdet_product` (`product_id`);

-- Resumen de pagos por factura y método (alimentado por recalcPaymentSummary)
CREATE TABLE IF NOT EXISTS `invoice_payment_summary` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `invoice_id` BIGINT NOT NULL,
  `cash_session_id` INT DEFAULT NULL,
  `method_id` INT NOT NULL,
  `amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_ips` (`invoice_id`, `method_id`),
  KEY `idx_ips_session` (`cash_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
