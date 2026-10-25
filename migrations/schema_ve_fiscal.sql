-- ============================================================================
-- ERPGo — Localización Fiscal Venezolana (SENIAT)
-- Migración idempotente. Añade: RIF, número de control fiscal, notas de
-- crédito, series de documentos, IGTF en línea, métodos de pago VE,
-- sesiones de caja (X/Z), barcode de productos y parámetros fiscales.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- Clientes: RIF obligatorio para libro de ventas + tipo de contribuyente
-- ---------------------------------------------------------------------------
ALTER TABLE `customer_information` ADD COLUMN IF NOT EXISTS `rif` VARCHAR(12) DEFAULT NULL COMMENT 'RIF: J/V/G/E + 8 dígitos + dígito verificador',
  ADD COLUMN `customer_type` TINYINT NOT NULL DEFAULT 1 COMMENT '1=Contribuyente formal, 2=Consumidor final, 3=Exento',
  ADD COLUMN `vat_withholding_agent` TINYINT NOT NULL DEFAULT 0 COMMENT 'Es agente de retención IVA';
ALTER TABLE `customer_information` ADD INDEX `idx_customer_rif` (`rif`);

-- ---------------------------------------------------------------------------
-- Proveedores: RIF obligatorio para libro de compras
-- ---------------------------------------------------------------------------
ALTER TABLE `supplier_information` ADD COLUMN IF NOT EXISTS `rif` VARCHAR(12) DEFAULT NULL COMMENT 'RIF del proveedor',
  ADD COLUMN `supplier_type` TINYINT NOT NULL DEFAULT 1 COMMENT '1=Contribuyente formal, 2=No contribuyente',
  ADD COLUMN `islr_concept_default` VARCHAR(10) DEFAULT NULL COMMENT 'Concepto ISLR por defecto (Decreto 1808)';
ALTER TABLE `supplier_information` ADD INDEX `idx_supplier_rif` (`rif`);

-- ---------------------------------------------------------------------------
-- Compañía: datos fiscales de emisor
-- ---------------------------------------------------------------------------
ALTER TABLE `company_information` ADD COLUMN IF NOT EXISTS `fiscal_address` TEXT DEFAULT NULL COMMENT 'Domicilio fiscal declarado al SENIAT',
  ADD COLUMN `imprenta_digital` VARCHAR(255) DEFAULT NULL COMMENT 'Imprenta digital autorizada (Providencia SNAT/2024/000102)',
  ADD COLUMN `invoice_series` VARCHAR(10) DEFAULT 'FACT' COMMENT 'Serie de factura activa';

-- ---------------------------------------------------------------------------
-- Facturas: control fiscal, RIF impreso, IGTF, multimoneda, anulación
-- ---------------------------------------------------------------------------
ALTER TABLE `invoice` ADD COLUMN IF NOT EXISTS `control_no` VARCHAR(20) DEFAULT NULL COMMENT 'Número de control fiscal',
  ADD COLUMN `customer_rif` VARCHAR(12) DEFAULT NULL COMMENT 'RIF impreso en la factura',
  ADD COLUMN `series` VARCHAR(10) NOT NULL DEFAULT 'FACT' COMMENT 'Serie del documento fiscal',
  ADD COLUMN `igtf_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00 COMMENT 'IGTF 3% percibido sobre pagos en divisa',
  ADD COLUMN `igtf_rate` DECIMAL(5,2) NOT NULL DEFAULT 3.00 COMMENT 'Alícuota IGTF aplicada',
  ADD COLUMN `currency` VARCHAR(5) NOT NULL DEFAULT 'VES' COMMENT 'Moneda de emisión',
  ADD COLUMN `exchange_rate` DECIMAL(18,4) NOT NULL DEFAULT 1.0000 COMMENT 'Tasa BCV aplicada',
  ADD COLUMN `is_anulled` TINYINT NOT NULL DEFAULT 0 COMMENT '0=viva, 1=anulada (nunca se borra)',
  ADD COLUMN `anulled_reason` VARCHAR(255) DEFAULT NULL,
  ADD COLUMN `anulled_at` DATETIME DEFAULT NULL;
ALTER TABLE `invoice` ADD INDEX `idx_invoice_control_no` (`control_no`);
ALTER TABLE `invoice` ADD INDEX `idx_invoice_date_status` (`date`, `is_anulled`);

-- ---------------------------------------------------------------------------
-- Compras: control fiscal del documento del proveedor
-- ---------------------------------------------------------------------------
ALTER TABLE `product_purchase` ADD COLUMN IF NOT EXISTS `control_no` VARCHAR(20) DEFAULT NULL COMMENT 'Número de control de la factura del proveedor',
  ADD COLUMN `supplier_rif` VARCHAR(12) DEFAULT NULL COMMENT 'RIF del proveedor al momento de registrar';

-- ---------------------------------------------------------------------------
-- Productos: barcode y costo de reposición
-- ---------------------------------------------------------------------------
ALTER TABLE `product_information` ADD COLUMN IF NOT EXISTS `barcode` VARCHAR(64) DEFAULT NULL COMMENT 'Código de barras (EAN13/Code128/peso)',
  ADD COLUMN `cost` DECIMAL(18,4) NOT NULL DEFAULT 0.0000 COMMENT 'Costo de reposición';
ALTER TABLE `product_information` ADD UNIQUE INDEX `idx_product_barcode` (`barcode`);

-- ---------------------------------------------------------------------------
-- Series de documentos fiscales (numeración única y consecutiva)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `fiscal_series` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(10) NOT NULL COMMENT 'FACT, NC, ND, RET-IVA, RET-ISLR',
  `name` VARCHAR(100) NOT NULL,
  `prefix` VARCHAR(6) NOT NULL DEFAULT '' COMMENT 'Prefijo impreso, ej: 00-',
  `next_no` BIGINT NOT NULL DEFAULT 1 COMMENT 'Próximo número a asignar',
  `control_prefix` VARCHAR(10) NOT NULL DEFAULT '00' COMMENT 'Prefijo del número de control',
  `control_next` BIGINT NOT NULL DEFAULT 1 COMMENT 'Próximo número de control',
  `is_active` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_series_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- Notas de crédito fiscales (anulan/ajustan facturas, Col. L libro de ventas)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `credit_note` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `note_no` BIGINT NOT NULL COMMENT 'Número consecutivo de nota',
  `control_no` VARCHAR(20) NOT NULL COMMENT 'Número de control fiscal de la nota',
  `series` VARCHAR(10) NOT NULL DEFAULT 'NC',
  `invoice_id` BIGINT NOT NULL COMMENT 'Factura afectada',
  `customer_id` BIGINT NOT NULL,
  `customer_rif` VARCHAR(12) DEFAULT NULL,
  `date` DATE NOT NULL,
  `total_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `vat_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `reason` VARCHAR(255) NOT NULL COMMENT 'Motivo: devolución, descuento posterior, anulación',
  `doc_type` VARCHAR(2) NOT NULL DEFAULT '03' COMMENT '01=Factura, 02=ND, 03=NC (libro ventas)',
  `created_by` VARCHAR(30) NOT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_note_no` (`series`, `note_no`),
  KEY `idx_cn_invoice` (`invoice_id`),
  KEY `idx_cn_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `credit_note_details` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `credit_note_id` INT NOT NULL,
  `product_id` VARCHAR(30) DEFAULT NULL,
  `description` VARCHAR(250) DEFAULT NULL,
  `quantity` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `rate` DECIMAL(18,4) NOT NULL DEFAULT 0.0000,
  `vat_percent` DECIMAL(5,2) NOT NULL DEFAULT 0.00,
  `vat_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `total_price` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  PRIMARY KEY (`id`),
  KEY `idx_cnd_note` (`credit_note_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- Notas de débito fiscales (cargos/adiciones a la deuda del cliente)
-- ---------------------------------------------------------------------------
-- Una nota de débito aumenta el saldo del cliente (gastos bancarios, cargos
-- por mora, diferencias de precio posteriores a la factura, etc.).
-- Genera asientos automáticos (cargo a Clientes).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `debit_note` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `note_no` BIGINT NOT NULL COMMENT 'Número consecutivo de nota',
  `control_no` VARCHAR(20) NOT NULL COMMENT 'Número de control fiscal de la nota',
  `series` VARCHAR(10) NOT NULL DEFAULT 'ND',
  `invoice_id` BIGINT DEFAULT NULL COMMENT 'Factura de origen (opcional)',
  `customer_id` BIGINT NOT NULL,
  `customer_rif` VARCHAR(12) DEFAULT NULL,
  `date` DATE NOT NULL,
  `total_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `vat_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `reason` VARCHAR(255) NOT NULL COMMENT 'Motivo: gasto bancario, mora, diferencia de precio',
  `doc_type` VARCHAR(2) NOT NULL DEFAULT '02' COMMENT '01=Factura, 02=ND, 03=NC (libro ventas)',
  `created_by` VARCHAR(30) NOT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_dn_note_no` (`series`, `note_no`),
  KEY `idx_dn_invoice` (`invoice_id`),
  KEY `idx_dn_customer` (`customer_id`),
  KEY `idx_dn_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `debit_note_details` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `debit_note_id` INT NOT NULL,
  `product_id` VARCHAR(30) DEFAULT NULL,
  `description` VARCHAR(250) DEFAULT NULL,
  `quantity` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `rate` DECIMAL(18,4) NOT NULL DEFAULT 0.0000,
  `vat_percent` DECIMAL(5,2) NOT NULL DEFAULT 0.00,
  `vat_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `total_price` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  PRIMARY KEY (`id`),
  KEY `idx_dnd_note` (`debit_note_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- Métodos de pago venezolanos (multi-pago + IGTF automático)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `payment_method` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(20) NOT NULL COMMENT 'cash, punto, pago_movil, zelle, transferencia, cripto, credito',
  `name` VARCHAR(50) NOT NULL,
  `is_foreign` TINYINT NOT NULL DEFAULT 0 COMMENT '1=pago en divisa/cripto → aplica IGTF',
  `coa_id` BIGINT DEFAULT NULL COMMENT 'Cuenta contable asociada',
  `is_active` TINYINT NOT NULL DEFAULT 1,
  `sort_order` INT NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_pm_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `payment_method` (`code`, `name`, `is_foreign`, `sort_order`) VALUES
  ('cash', 'Efectivo', 0, 1),
  ('punto', 'Punto de Venta', 0, 2),
  ('pago_movil', 'Pago Móvil', 0, 3),
  ('transferencia', 'Transferencia', 0, 4),
  ('zelle', 'Zelle', 1, 5),
  ('cripto', 'Criptoactivo', 1, 6),
  ('credito', 'Crédito', 0, 7);

-- ---------------------------------------------------------------------------
-- Sesiones de caja (turno → cierre X/Z con arqueo por método de pago)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cash_session` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `cashier` VARCHAR(30) NOT NULL COMMENT 'Usuario cajero',
  `warehouse_id` VARCHAR(30) DEFAULT NULL,
  `opening_amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00 COMMENT 'Fondo inicial',
  `opening_amount_usd` DECIMAL(18,2) NULL DEFAULT '0.00' COMMENT 'Fondo inicial en USD',
  `closing_amount` DECIMAL(18,2) DEFAULT NULL COMMENT 'Efectivo contado al cierre',
  `expected_cash` DECIMAL(18,2) DEFAULT NULL COMMENT 'Efectivo esperado por sistema',
  `difference` DECIMAL(18,2) DEFAULT NULL COMMENT 'Sobrante/faltante',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1=abierta, 2=cerrada',
  `opened_at` DATETIME NOT NULL,
  `closed_at` DATETIME DEFAULT NULL,
  `open_note` VARCHAR(255) DEFAULT NULL,
  `close_note` VARCHAR(255) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_cs_cashier_status` (`cashier`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cash_session_payment` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `cash_session_id` INT NOT NULL,
  `payment_method_id` INT NOT NULL,
  `tx_count` INT NOT NULL DEFAULT 0,
  `amount` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `counted_amount` DECIMAL(18,2) DEFAULT NULL COMMENT 'Monto contado por el cajero (efectivo)',
  PRIMARY KEY (`id`),
  KEY `idx_csp_session` (`cash_session_id`),
  UNIQUE KEY `uq_csp` (`cash_session_id`, `payment_method_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- Parámetros fiscales (leídos por fiscal.go, editables en Ajustes > Fiscal)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `fiscal_setting` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `key` VARCHAR(50) NOT NULL,
  `value` VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fs_key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `fiscal_setting` (`key`, `value`) VALUES
  ('igtf_enabled', '1'),
  ('igtf_rate', '3.00'),
  ('walkin_rif_required', '0'),
  ('fiscal_footnote', 'Documento fiscal conforme a Ley de IVA y Providencia SNAT/2024/000102');
