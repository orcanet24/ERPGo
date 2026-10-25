-- ============================================================================
-- ERPGo — Recuperación de COLUMNAS que faltaban en las migraciones.
--
-- Igual que las tablas de schema_z_zzz_recuperadas.sql: estas columnas existen
-- en las bases en uso (por ALTERs históricos que nunca se versionaron) y el
-- código las lee/escribe a diario, pero ninguna migración las creaba. Una
-- empresa NUEVA arrancaba sin ellas:
--
--   - Compras:    product_purchase.currency_code  -> Error 1054 en /purchase/save
--   - Facturas:   invoice.currency_code / total_amount_usd
--   - Contabilidad: acc_vaucher / acc_transaction currency + exchange_rate
--   - Multi-moneda: producto, proveedor, cobros, caja, tasa
--
-- Idempotente: en una base que ya las tiene, el runner cuenta "duplicate
-- column name" como ya-estaba y sigue. En una instalación nueva las añade.
-- ============================================================================

ALTER TABLE `invoice` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `total_amount_usd` decimal(18,2) NULL DEFAULT '0.00',
  ADD COLUMN `exchange_rate_source` varchar(20) NULL DEFAULT 'BCV';

ALTER TABLE `product_purchase` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `exchange_rate` decimal(18,4) NULL DEFAULT '1.0000',
  ADD COLUMN `total_amount_usd` decimal(18,2) NULL DEFAULT '0.00';

ALTER TABLE `acc_vaucher` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `exchange_rate` decimal(18,4) NULL DEFAULT '1.0000';

ALTER TABLE `acc_transaction` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `exchange_rate` decimal(18,4) NULL DEFAULT '1.0000';

ALTER TABLE `product_information` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES' COMMENT 'Moneda del precio base',
  ADD COLUMN `price_usd` decimal(18,4) NULL COMMENT 'Precio en USD (NULL = calcular por tasa)';

ALTER TABLE `supplier_information` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES';

ALTER TABLE `person_ledger` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `balance_usd` decimal(18,2) NULL DEFAULT '0.00';

ALTER TABLE `invoice_payment_summary` ADD COLUMN IF NOT EXISTS `currency_code` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `amount_usd` decimal(18,2) NULL DEFAULT '0.00',
  ADD COLUMN `exchange_rate` decimal(18,4) NULL DEFAULT '1.0000';

ALTER TABLE `cash_session` ADD COLUMN IF NOT EXISTS `opening_amount_usd` decimal(18,2) NULL DEFAULT '0.00',
  ADD COLUMN `closing_amount_usd` decimal(18,2) NULL,
  ADD COLUMN `expected_cash_usd` decimal(18,2) NULL,
  ADD COLUMN `difference_usd` decimal(18,2) NULL,
  ADD COLUMN `currency_` varchar(5) NULL DEFAULT 'VES';

ALTER TABLE `cost_timesheet` ADD COLUMN IF NOT EXISTS `task_id` int NULL,
  ADD COLUMN `nonbillable_reason` varchar(60) NOT NULL DEFAULT '',
  ADD COLUMN `change_order_amount` decimal(16,2) NOT NULL DEFAULT '0.00';

ALTER TABLE `menu` ADD COLUMN IF NOT EXISTS `module_id` varchar(64) NOT NULL DEFAULT 'core',
  ADD COLUMN `is_core` tinyint(1) NOT NULL DEFAULT '0';

ALTER TABLE `app_setting` ADD COLUMN IF NOT EXISTS `default_currency` varchar(5) NULL DEFAULT 'VES',
  ADD COLUMN `exchange_rate_auto` tinyint(1) NULL DEFAULT '1',
  ADD COLUMN `multi_currency_enabled` tinyint(1) NULL DEFAULT '1';

ALTER TABLE `sec_role` ADD COLUMN IF NOT EXISTS `level` enum('master','admin','manager','employee') NOT NULL DEFAULT 'employee',
  ADD COLUMN `created_at` datetime NULL DEFAULT CURRENT_TIMESTAMP,
  ADD COLUMN `description` varchar(255) NULL DEFAULT '',
  ADD COLUMN `is_system` tinyint(1) NULL DEFAULT '0';
