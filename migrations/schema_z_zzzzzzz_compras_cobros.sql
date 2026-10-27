-- ============================================================================
-- ERPGo — Compras clasificadas con candado, cuentas por pagar con servicios,
-- cuentas por cobrar con cobros repartidos.
--
-- QUÉ RESUELVE CADA BLOQUE
--
--  1. product_purchase.document_kind + purchase_hash
--     La compra por FACTURA del proveedor y la compra por ORDEN o NOTA DE ENTREGA
--     ya no se guardan igual: solo la factura da crédito fiscal (IVA, retenciones).
--     Y el hash —proveedor + tipo + números + renglones— con índice único es el
--     candado: intentar cargar la misma compra dos veces choca contra la base, sea
--     un humano o un agente.
--
--     El hash admite NULL a propósito: las compras ya registradas no lo tienen y
--     tienen que seguir entrando. Para ESAS el candado compara proveedor + números
--     normalizados (ver compraLegacyDuplicada).
--
--  2. accounts_payable.concept + origin
--     El agua, la luz, el teléfono, el internet y el arriendo no pasan por el
--     almacén pero SÍ son deudas del comercio. Aquí se registran, con su concepto
--     y su origen, para que Cuentas por Pagar sea el registro de lo que se debe y
--     se vea de dónde viene cada deuda.
--
--  3. debt_collection con sus columnas de multi-moneda
--     La tabla la creaba el propio handler de cobros, y su DDL no traía
--     amount_usd/exchange_rate/currency_code —que el INSERT sí usaba—: en una
--     instalación nueva el primer cobro fallaba. Aquí queda el esquema completo.
--
--  4. Menú: Cuentas por Cobrar y Cuentas por Pagar
--     Los dos módulos existían y no había forma de llegar a ellos desde el menú.
--
-- Idempotente: el runner descarta «duplicate column name» / «duplicate key name»
-- y quita el IF NOT EXISTS de MariaDB antes de hablar con MySQL.
-- ============================================================================

ALTER TABLE `product_purchase` ADD COLUMN IF NOT EXISTS `document_kind` varchar(20) NOT NULL DEFAULT 'factura' COMMENT 'factura | nota_entrega | orden | sin_documento: solo la factura da credito fiscal',
  ADD COLUMN `purchase_hash` char(64) NULL COMMENT 'identidad de la compra (proveedor + numeros + renglones): candado contra duplicados';

ALTER TABLE `product_purchase` ADD UNIQUE KEY `uq_purchase_hash` (`purchase_hash`);

ALTER TABLE `product_purchase` ADD KEY `idx_purchase_doc` (`supplier_id`, `control_no`);

ALTER TABLE `accounts_payable` ADD COLUMN IF NOT EXISTS `concept` varchar(200) NOT NULL DEFAULT '' COMMENT 'que se debe: servicio, gasto o documento',
  ADD COLUMN `origin` varchar(20) NOT NULL DEFAULT 'compra' COMMENT 'compra | servicio | pago_compra';

CREATE TABLE IF NOT EXISTS `debt_collection` (
  `id` int NOT NULL AUTO_INCREMENT,
  `customer_id` int NOT NULL,
  `invoice_id` int DEFAULT 0,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `amount_usd` decimal(18,2) DEFAULT '0.00',
  `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES',
  `payment_method_id` int DEFAULT 0,
  `collection_date` date DEFAULT NULL,
  `reference` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text COLLATE utf8mb4_unicode_ci,
  `created_by` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_customer` (`customer_id`),
  KEY `idx_invoice` (`invoice_id`),
  KEY `idx_date` (`collection_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `debt_collection` ADD COLUMN IF NOT EXISTS `amount_usd` decimal(18,2) DEFAULT '0.00',
  ADD COLUMN `exchange_rate` decimal(18,4) DEFAULT '1.0000',
  ADD COLUMN `currency_code` varchar(5) COLLATE utf8mb4_unicode_ci DEFAULT 'VES';

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`) VALUES
  (115, 3, 'Cuentas por Cobrar', '/cartera/list', NULL, 'customer_ledger', '/cartera', 60, 1),
  (123, 4, 'Cuentas por Pagar', '/pagar/list', NULL, 'supplier_payment', '/pagar', 40, 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `parent_id` = VALUES(`parent_id`), `url` = VALUES(`url`), `icon` = VALUES(`icon`), `perm` = VALUES(`perm`), `active_prefix` = VALUES(`active_prefix`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);
