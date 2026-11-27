-- Schema Reconstruido 02 Inventory
-- Reconstruido de la base de produccion erpgo_default
-- Fecha: 2026-10-12
-- Estas tablas faltaban en una instalacion limpia.

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `product_category` (
  `category_id` int NOT NULL AUTO_INCREMENT,
  `category_name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` int NOT NULL,
  PRIMARY KEY (`category_id`)
) ENGINE=InnoDB AUTO_INCREMENT=41 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_cost_config` (
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `costing_method` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'PP',
  `loss_factor` decimal(6,4) NOT NULL DEFAULT '0.0000' COMMENT 'Merma global del producto, 0.0250 = 2,5%. Solo afecta al material.',
  `perishable` tinyint NOT NULL DEFAULT '0',
  `shelf_life_days` int NOT NULL DEFAULT '0',
  `is_assembled` tinyint NOT NULL DEFAULT '0',
  `track_lots` tinyint NOT NULL DEFAULT '0',
  `track_serial` tinyint NOT NULL DEFAULT '0',
  `imported` tinyint NOT NULL DEFAULT '0',
  `cost_center_id` int DEFAULT NULL,
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`),
  KEY `idx_pcc_metodo` (`costing_method`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_daily_demand` (
  `product_id` int NOT NULL,
  `demand_date` date NOT NULL,
  `quantity` double NOT NULL DEFAULT '0',
  `source` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'auto' COMMENT 'auto | manual',
  `note` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`,`demand_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_information` (
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
  `price_usd` decimal(18,4) DEFAULT NULL COMMENT 'Precio en USD (NULL = calcular por tasa)',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES' COMMENT 'Moneda del precio base',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_product_barcode` (`barcode`),
  KEY `category_id` (`category_id`),
  KEY `product_id` (`product_id`),
  KEY `idx_product_category` (`category_id`),
  KEY `idx_product_product_id` (`product_id`)
) ENGINE=InnoDB AUTO_INCREMENT=50 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_lot` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` int NOT NULL,
  `lot_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `entry_date` date NOT NULL,
  `expiry_date` date DEFAULT NULL,
  `quantity_received` double NOT NULL DEFAULT '0',
  `quantity_available` double NOT NULL DEFAULT '0',
  `unit_cost` double NOT NULL DEFAULT '0',
  `supplier_id` int DEFAULT NULL,
  `purchase_id` int DEFAULT NULL,
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT 'active | depleted | expired | quarantined',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_lot_product_code` (`product_id`,`lot_code`),
  KEY `idx_lot_product` (`product_id`),
  KEY `idx_lot_expiry` (`expiry_date`),
  KEY `idx_lot_code` (`product_id`,`lot_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_price_list` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `price_list_id` int NOT NULL,
  `price` decimal(16,4) NOT NULL DEFAULT '0.0000',
  `margin_pct` decimal(8,4) NOT NULL DEFAULT '0.0000',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_producto_lista` (`product_id`,`price_list_id`),
  KEY `idx_ppl_lista` (`price_list_id`)
) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_purchase` (
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
  PRIMARY KEY (`id`),
  KEY `purchase_id` (`purchase_id`),
  KEY `supplier_id` (`supplier_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_purchase_details` (
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
  PRIMARY KEY (`id`),
  KEY `purchase_id` (`purchase_id`),
  KEY `product_id` (`product_id`),
  KEY `idx_ppdet_product` (`product_id`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_recipe` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `component_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quantity` decimal(14,4) NOT NULL DEFAULT '0.0000',
  `unit` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `loss_factor` decimal(6,4) NOT NULL DEFAULT '0.0000',
  `step_order` int NOT NULL DEFAULT '0',
  `is_substitute` tinyint NOT NULL DEFAULT '0',
  `labor_grade` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `hours` decimal(12,2) NOT NULL DEFAULT '0.00',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_recipe` (`product_id`,`component_id`,`step_order`),
  KEY `idx_recipe_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_return` (
  `return_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `product_id` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `invoice_id` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `purchase_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `date_purchase` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `date_return` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `byy_qty` float NOT NULL,
  `ret_qty` float NOT NULL,
  `customer_id` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `supplier_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `product_rate` decimal(10,2) NOT NULL DEFAULT '0.00',
  `deduction` float NOT NULL,
  `total_deduct` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_tax` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_ret_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `net_total_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `reason` text CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `usablity` int NOT NULL,
  KEY `product_id` (`product_id`),
  KEY `invoice_id` (`invoice_id`),
  KEY `purchase_id` (`purchase_id`),
  KEY `customer_id` (`customer_id`),
  KEY `supplier_id` (`supplier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_service` (
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

CREATE TABLE IF NOT EXISTS `product_yield` (
  `id` int NOT NULL AUTO_INCREMENT,
  `input_product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `output_product_id` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `expected_pct` decimal(6,4) NOT NULL DEFAULT '0.0000',
  `is_byproduct` tinyint NOT NULL DEFAULT '0',
  `notes` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_yield` (`input_product_id`,`output_product_id`),
  KEY `idx_yield_entrada` (`input_product_id`)
) ENGINE=InnoDB AUTO_INCREMENT=13 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cycle_count` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `warehouse_id` int DEFAULT NULL,
  `scope` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'category' COMMENT 'category | supplier | location | abc | full',
  `scope_value` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'open' COMMENT 'open | counting | reviewed | closed | cancelled',
  `started_by` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `started_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_by` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `closed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `note` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cycle_code` (`code`),
  KEY `idx_cycle_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cycle_count_adjustment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cycle_line_id` int NOT NULL,
  `voucher_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'asiento que corrige el stock',
  `quantity_delta` double NOT NULL DEFAULT '0',
  `value_delta` double NOT NULL DEFAULT '0',
  `reason` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'merma | error de entrada | robo | ajuste',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_adjust_line` (`cycle_line_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cycle_count_line` (
  `id` int NOT NULL AUTO_INCREMENT,
  `cycle_id` int NOT NULL,
  `product_id` int NOT NULL,
  `lot_id` int DEFAULT NULL,
  `expected_quantity` double NOT NULL DEFAULT '0',
  `counted_quantity` double DEFAULT NULL COMMENT 'NULL = aún no contado',
  `variance` double DEFAULT NULL,
  `unit_cost` double NOT NULL DEFAULT '0',
  `variance_value` double NOT NULL DEFAULT '0',
  `counted_by` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `counted_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `adjusted` int NOT NULL DEFAULT '0',
  `note` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cycle_line` (`cycle_id`,`product_id`,`lot_id`),
  KEY `idx_line_cycle` (`cycle_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `reorder_policy` (
  `product_id` int NOT NULL,
  `lead_time_days` int NOT NULL DEFAULT '7' COMMENT 'plazo del proveedor en días',
  `safety_days` int NOT NULL DEFAULT '2' COMMENT 'días de consumo que se dejan como colchón',
  `min_order_qty` double NOT NULL DEFAULT '0',
  `pack_size` double NOT NULL DEFAULT '1' COMMENT 'múltiplo del pedido: cajas, sacos',
  `avg_daily_demand` double NOT NULL DEFAULT '0' COMMENT 'demanda media, calculada por el sistema',
  `computed_on` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'fecha del último cálculo',
  `active` int NOT NULL DEFAULT '1',
  `note` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `reorder_suggestion` (
  `id` int NOT NULL AUTO_INCREMENT,
  `product_id` int NOT NULL,
  `suggested_date` date NOT NULL,
  `on_hand` double NOT NULL DEFAULT '0',
  `reorder_point` double NOT NULL DEFAULT '0' COMMENT 'stock de seguridad + consumo del plazo',
  `suggested_qty` double NOT NULL DEFAULT '0',
  `pack_size` double NOT NULL DEFAULT '1',
  `estimated_cost` double NOT NULL DEFAULT '0',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending' COMMENT 'pending | ordered | dismissed',
  `dismiss_reason` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_suggestion_product` (`product_id`,`suggested_date`),
  KEY `idx_suggestion_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `warehouse` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `is_default` tinyint NOT NULL DEFAULT '0',
  `is_active` tinyint NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_wh_code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
