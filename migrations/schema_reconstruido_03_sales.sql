-- Schema Reconstruido 03 Sales
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `invoice` (
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
  KEY `customer_id` (`customer_id`),
  KEY `invoice_id` (`invoice_id`),
  KEY `idx_invoice_control_no` (`control_no`),
  KEY `idx_invoice_date_status` (`date`,`is_anulled`),
  KEY `idx_invoice_cash_session` (`cash_session_id`),
  KEY `idx_invoice_customer_id` (`customer_id`),
  KEY `idx_invoice_invoice_id` (`invoice_id`)
) ENGINE=InnoDB AUTO_INCREMENT=68 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `invoice_details` (
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
  PRIMARY KEY (`id`),
  KEY `invoice_id` (`invoice_id`),
  KEY `product_id` (`product_id`),
  KEY `idx_invdet_product` (`product_id`)
) ENGINE=InnoDB AUTO_INCREMENT=80 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `invoice_items` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `invoice_id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `product_code` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sku` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(300) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(15,3) NOT NULL,
  `unit_price` decimal(15,2) NOT NULL,
  `discount_pct` decimal(5,2) DEFAULT '0.00',
  `tax_rate` decimal(5,2) DEFAULT '16.00',
  `subtotal` decimal(15,2) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_invoice` (`invoice_id`),
  CONSTRAINT `invoice_items_ibfk_1` FOREIGN KEY (`invoice_id`) REFERENCES `invoices` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `invoice_payment_summary` (
  `id` int NOT NULL AUTO_INCREMENT,
  `invoice_id` bigint NOT NULL,
  `cash_session_id` int DEFAULT NULL,
  `method_id` int NOT NULL,
  `amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `amount_usd` decimal(18,2) DEFAULT '0.00',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_ips` (`invoice_id`,`method_id`),
  KEY `idx_ips_session` (`cash_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `invoices` (
  `id` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant_code` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_number` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `control_number` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `terminal_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `cash_register_id` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `rif` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_name` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_address` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `customer_phone` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `subtotal` decimal(15,2) NOT NULL DEFAULT '0.00',
  `tax` decimal(15,2) NOT NULL DEFAULT '0.00',
  `total` decimal(15,2) NOT NULL DEFAULT '0.00',
  `tax_rate` decimal(5,2) NOT NULL DEFAULT '16.00',
  `status` enum('draft','printing','printed','cancelled') COLLATE utf8mb4_unicode_ci DEFAULT 'draft',
  `created_at` datetime NOT NULL,
  `printed_at` datetime DEFAULT NULL,
  `fiscal_number` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_tenant` (`tenant_code`),
  KEY `idx_invoice_number` (`invoice_number`),
  KEY `idx_terminal` (`terminal_id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `credit_note` (
  `id` int NOT NULL AUTO_INCREMENT,
  `note_no` bigint NOT NULL COMMENT 'Número consecutivo de nota',
  `control_no` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Número de control fiscal de la nota',
  `series` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'NC',
  `invoice_id` bigint NOT NULL COMMENT 'Factura afectada',
  `customer_id` bigint NOT NULL,
  `customer_rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date` date NOT NULL,
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `reason` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Motivo: devolución, descuento posterior, anulación',
  `doc_type` varchar(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '03' COMMENT '01=Factura, 02=ND, 03=NC (libro ventas)',
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_note_no` (`series`,`note_no`),
  KEY `idx_cn_invoice` (`invoice_id`),
  KEY `idx_cn_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `credit_note_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `credit_note_id` int NOT NULL,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `quantity` decimal(18,2) NOT NULL DEFAULT '0.00',
  `rate` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `vat_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_price` decimal(18,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `idx_cnd_note` (`credit_note_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `debit_note` (
  `id` int NOT NULL AUTO_INCREMENT,
  `note_no` bigint NOT NULL COMMENT 'Número consecutivo de nota',
  `control_no` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Número de control fiscal de la nota',
  `series` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'ND',
  `invoice_id` bigint DEFAULT NULL COMMENT 'Factura de origen (opcional)',
  `customer_id` bigint NOT NULL,
  `customer_rif` varchar(12) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `date` date NOT NULL,
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `reason` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Motivo: gasto bancario, mora, diferencia de precio',
  `doc_type` varchar(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '02' COMMENT '01=Factura, 02=ND, 03=NC (libro ventas)',
  `created_by` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_dn_note_no` (`series`,`note_no`),
  KEY `idx_dn_invoice` (`invoice_id`),
  KEY `idx_dn_customer` (`customer_id`),
  KEY `idx_dn_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `debit_note_details` (
  `id` int NOT NULL AUTO_INCREMENT,
  `debit_note_id` int NOT NULL,
  `product_id` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `description` varchar(250) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `quantity` decimal(18,2) NOT NULL DEFAULT '0.00',
  `rate` decimal(18,4) NOT NULL DEFAULT '0.0000',
  `vat_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_price` decimal(18,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  KEY `idx_dnd_note` (`debit_note_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `quotation` (
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

CREATE TABLE IF NOT EXISTS `quotation_service_used` (
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

CREATE TABLE IF NOT EXISTS `quotation_taxinfo` (
  `id` int NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL,
  `customer_id` int NOT NULL,
  `relation_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `tax0` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `tax1` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  PRIMARY KEY (`id`),
  KEY `customer_id` (`customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `quot_products_used` (
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

CREATE TABLE IF NOT EXISTS `seles_termscondi` (
  `id` int NOT NULL AUTO_INCREMENT,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
