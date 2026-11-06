-- ---------------------------------------------------------------------------
-- COMPRAS POR DESTINO: SERVICIO, CONSUMO INTERNO Y MERCANCÍA DE VENTA
--
-- Hasta esta migración, toda compra entraba a inventario y se asentaba contra
-- una sola cuenta (1020401 Inventario). Los servicios terminaban siendo un
-- activo con existencias y la papelería se mezclaba con la mercancía de venta.
--
-- Esta migración crea la estructura para separarlos:
--   * product_information.item_type: venta | consumo | servicio
--   * product_purchase_details: destino, almacén y cuenta por renglón
--   * warehouse.kind: almacenes de venta y de consumo
--   * cuentas nuevas: inventario de suministros, gastos de servicios,
--     gastos de suministros e IVA crédito fiscal
--   * tablas de consumo interno (la salida de suministros al gasto)
--
-- Es idempotente: se puede aplicar sobre una base vacía, sobre la base del PHP
-- original o sobre una instalación que ya la tenga.
-- ---------------------------------------------------------------------------

-- 1. Tipo de ítem en el catálogo ------------------------------------------------
ALTER TABLE `product_information` ADD COLUMN IF NOT EXISTS `item_type` VARCHAR(20) NOT NULL DEFAULT 'venta';
ALTER TABLE `product_information` ADD COLUMN IF NOT EXISTS `default_warehouse_id` VARCHAR(30) NOT NULL DEFAULT '';

-- 2. Destino por renglón de compra ---------------------------------------------
ALTER TABLE `product_purchase_details` ADD COLUMN IF NOT EXISTS `item_type` VARCHAR(20) NOT NULL DEFAULT 'venta';
ALTER TABLE `product_purchase_details` ADD COLUMN IF NOT EXISTS `coa_id` INT NULL;
ALTER TABLE `product_purchase_details` ADD COLUMN IF NOT EXISTS `warehouse_id` VARCHAR(30) NOT NULL DEFAULT '';
ALTER TABLE `product_purchase_details` ADD COLUMN IF NOT EXISTS `concept` VARCHAR(255) NOT NULL DEFAULT '';

-- 3. Tipo de almacén ------------------------------------------------------------
ALTER TABLE `warehouse` ADD COLUMN IF NOT EXISTS `kind` VARCHAR(20) NOT NULL DEFAULT 'venta';

-- 4. Cuentas de la separación ---------------------------------------------------
ALTER TABLE `acc_predefine_account` ADD COLUMN IF NOT EXISTS `supplies_inventory_code` INT NOT NULL DEFAULT 0;
ALTER TABLE `acc_predefine_account` ADD COLUMN IF NOT EXISTS `supplies_expense_code` INT NOT NULL DEFAULT 0;
ALTER TABLE `acc_predefine_account` ADD COLUMN IF NOT EXISTS `service_expense_code` INT NOT NULL DEFAULT 0;
ALTER TABLE `acc_predefine_account` ADD COLUMN IF NOT EXISTS `purchase_vat_code` INT NOT NULL DEFAULT 0;

-- 5. stock_movement acepta los tipos nuevos ------------------------------------
-- Se pasa a VARCHAR en vez de ampliar el ENUM para no volver a tocar la tabla
-- cada vez que aparezca un movimiento nuevo (consumo, transferencias).
ALTER TABLE `stock_movement` MODIFY COLUMN `movement_type` VARCHAR(30) NOT NULL;
CREATE INDEX IF NOT EXISTS `idx_sm_wh` ON `stock_movement` (`warehouse_id`, `product_id`, `date`);
CREATE INDEX IF NOT EXISTS `idx_sm_type_date` ON `stock_movement` (`movement_type`, `date`);

-- 6. Almacenes ------------------------------------------------------------------
INSERT INTO `warehouse` (`code`, `name`, `address`, `is_default`, `is_active`, `kind`, `created_at`)
  SELECT 'WH-CONSUMO', 'Consumo interno', '', 0, 1, 'consumo', NOW()
  WHERE NOT EXISTS (SELECT 1 FROM `warehouse` WHERE `code` = 'WH-CONSUMO');

-- El almacén histórico pasa a ser de venta y, si no hay ninguno marcado, queda
-- como el de venta por defecto.
UPDATE `warehouse` SET `kind` = 'venta' WHERE IFNULL(`kind`, '') = '';
UPDATE `warehouse` SET `kind` = 'consumo' WHERE `code` = 'WH-CONSUMO';
UPDATE `warehouse` SET `is_default` = 1
  WHERE `code` = 'WH-MAIN'
    AND NOT EXISTS (SELECT 1 FROM (SELECT `id` FROM `warehouse` WHERE `is_default` = 1 AND IFNULL(`kind`,'venta') = 'venta') AS x);

-- 7. Cuentas contables nuevas ---------------------------------------------------
-- Inventario de suministros y consumibles (activo, separado del de mercancía).
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
  SELECT (SELECT IFNULL(MAX(`id`), 0) + 1 FROM `acc_coa`), 1020403, 'Inventario de suministros y consumibles', 'Inventory', '10204', 4, 1, 1, 0, 0, 0, 'A', 0, 0, 0.00, '1', NOW(), '', NOW(), 0, 1, 1, 0
  WHERE NOT EXISTS (SELECT 1 FROM `acc_coa` WHERE `HeadCode` = 1020403);

-- IVA crédito fiscal de las compras (activo). Antes el IVA se sumaba al
-- inventario porque todo el documento se debitaba contra esa cuenta.
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
  SELECT (SELECT IFNULL(MAX(`id`), 0) + 1 FROM `acc_coa`), 1020901, 'IVA crédito fiscal (compras)', 'Current Asset', '102', 4, 1, 1, 0, 0, 0, 'A', 0, 0, 0.00, '1', NOW(), '', NOW(), 0, 1, 0, 0
  WHERE NOT EXISTS (SELECT 1 FROM `acc_coa` WHERE `HeadCode` = 1020901);

-- Gastos de servicios: luz, agua, teléfono, internet, asesoría.
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
  SELECT (SELECT IFNULL(MAX(`id`), 0) + 1 FROM `acc_coa`), 4021401, 'Gastos de servicios (luz, agua, internet)', 'Over Head Cost', '402', 4, 1, 1, 0, 0, 0, 'E', 0, 0, 0.00, '1', NOW(), '', NOW(), 0, 1, 0, 0
  WHERE NOT EXISTS (SELECT 1 FROM `acc_coa` WHERE `HeadCode` = 4021401);

-- Gastos de suministros y limpieza: la contrapartida del consumo interno.
INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
  SELECT (SELECT IFNULL(MAX(`id`), 0) + 1 FROM `acc_coa`), 4021402, 'Gastos de suministros y limpieza', 'Over Head Cost', '402', 4, 1, 1, 0, 0, 0, 'E', 0, 0, 0.00, '1', NOW(), '', NOW(), 0, 1, 0, 0
  WHERE NOT EXISTS (SELECT 1 FROM `acc_coa` WHERE `HeadCode` = 4021402);

-- Asignación de las cuentas nuevas a la configuración de la empresa.
UPDATE `acc_predefine_account` SET `supplies_inventory_code` = 1020403 WHERE IFNULL(`supplies_inventory_code`, 0) = 0;
UPDATE `acc_predefine_account` SET `supplies_expense_code` = 4021402 WHERE IFNULL(`supplies_expense_code`, 0) = 0;
UPDATE `acc_predefine_account` SET `service_expense_code` = 4021401 WHERE IFNULL(`service_expense_code`, 0) = 0;
UPDATE `acc_predefine_account` SET `purchase_vat_code` = 1020901 WHERE IFNULL(`purchase_vat_code`, 0) = 0;

-- 8. Consumo interno ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `internal_consumption` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `consumption_no` VARCHAR(50) NOT NULL,
  `date` DATE NOT NULL,
  `warehouse_id` VARCHAR(30) NOT NULL,
  `coa_id` INT NOT NULL,
  `concept` VARCHAR(255) NOT NULL DEFAULT '',
  `total` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `created_by` VARCHAR(50) DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_consumption_no` (`consumption_no`),
  KEY `idx_consumption_date` (`date`)
);

CREATE TABLE IF NOT EXISTS `internal_consumption_details` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `consumption_no` VARCHAR(50) NOT NULL,
  `product_id` VARCHAR(30) NOT NULL,
  `quantity` DECIMAL(18,2) NOT NULL,
  `unit_cost` DECIMAL(18,6) NOT NULL DEFAULT 0.000000,
  `total` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  PRIMARY KEY (`id`),
  KEY `idx_cons_detail_no` (`consumption_no`),
  KEY `idx_cons_detail_product` (`product_id`)
);

-- 9. Movimientos históricos -----------------------------------------------------
-- Los ajustes antiguos se guardaban en product_purchase_details y NO dejaban
-- movimiento de stock. Al pasar el stock a stock_movement, sin este relleno los
-- ajustes anteriores desaparecerían del inventario.
INSERT INTO `stock_movement` (`movement_type`, `product_id`, `warehouse_id`, `quantity`, `unit_cost`, `reference_type`, `reference_id`, `reference_no`, `date`, `is_posted`, `posted_at`, `created_at`)
  SELECT CASE WHEN d.`type` = 'add' THEN 'adjustment_add' ELSE 'adjustment_remove' END,
         CAST(d.`product_id` AS UNSIGNED), IFNULL(a.`warehouse_id`, 'WH-MAIN'), d.`quantity`, 0,
         'adjustment', a.`id`, a.`adjustment_no`, a.`date`, 1, NOW(), NOW()
  FROM `store_adjustment_details` d
  JOIN `store_adjustment` a ON a.`adjustment_no` = d.`adjustment_no`
  WHERE NOT EXISTS (
      SELECT 1 FROM `stock_movement` s
      WHERE s.`reference_type` = 'adjustment' AND s.`reference_id` = a.`id` AND s.`product_id` = CAST(d.`product_id` AS UNSIGNED)
  );

-- Compras anteriores al libro de movimientos. El detalle de compra existe
-- desde el PHP original, pero `stock_movement` es posterior: sin este relleno,
-- al pasar el stock a leerse de los movimientos, todo lo comprado antes
-- desaparecería del inventario y el sistema arrancaría en negativo.
--
-- Se excluyen los renglones de ajustes (`ADJ-%`) y de conteos (`CC-%`), que no
-- son compras: los primeros ya se rellenan arriba desde `store_adjustment`, y
-- los segundos se convierten abajo desde su propio detalle.
INSERT INTO `stock_movement` (`movement_type`, `product_id`, `warehouse_id`, `quantity`, `unit_cost`, `reference_type`, `reference_id`, `reference_no`, `date`, `is_posted`, `posted_at`, `created_at`)
  SELECT 'purchase', CAST(d.`product_id` AS UNSIGNED), IFNULL(p.`warehouse_id`, 'WH-MAIN'),
         ABS(d.`quantity`), IFNULL(d.`rate`, 0), 'purchase', d.`purchase_id`,
         CONCAT('PUR-', d.`purchase_id`), p.`purchase_date`, 1, NOW(), NOW()
  FROM `product_purchase_details` d
  JOIN `product_purchase` p ON p.`purchase_id` = d.`purchase_id`
  WHERE d.`quantity` <> 0
    AND IFNULL(d.`batch_id`, '') NOT LIKE 'ADJ-%'
    AND IFNULL(d.`batch_id`, '') NOT LIKE 'CC-%'
    AND NOT EXISTS (
        SELECT 1 FROM `stock_movement` s
        WHERE s.`reference_type` = 'purchase' AND s.`reference_id` = d.`purchase_id`
          AND s.`product_id` = CAST(d.`product_id` AS UNSIGNED)
    );

-- Conteos físicos anteriores: se guardaban en el detalle de compra, sin
-- movimiento. Se convierten en ajustes con el signo del descuadre (el conteo
-- suma si había de más y resta si faltaba), que es lo que el reporte espera.
INSERT INTO `stock_movement` (`movement_type`, `product_id`, `warehouse_id`, `quantity`, `unit_cost`, `reference_type`, `reference_id`, `reference_no`, `date`, `is_posted`, `posted_at`, `created_at`)
  SELECT CASE WHEN d.`quantity` < 0 THEN 'adjustment_remove' ELSE 'adjustment_add' END,
         CAST(d.`product_id` AS UNSIGNED), 'WH-MAIN', ABS(d.`quantity`), IFNULL(d.`rate`, 0),
         'adjustment', 0, d.`batch_id`, IFNULL(DATE(cc.`started_at`), CURDATE()), 1, NOW(), NOW()
  FROM `product_purchase_details` d
  LEFT JOIN `cycle_count` cc ON cc.`id` = d.`purchase_id`
  WHERE IFNULL(d.`batch_id`, '') LIKE 'CC-%'
    AND d.`quantity` <> 0
    AND NOT EXISTS (
        SELECT 1 FROM `stock_movement` s
        WHERE s.`reference_no` = d.`batch_id`
          AND s.`product_id` = CAST(d.`product_id` AS UNSIGNED)
    );

-- La devolución a proveedor se guardaba con cantidad NEGATIVA y ahora el signo
-- lo da el tipo de movimiento. Sin normalizar, restaría dos veces.
UPDATE `stock_movement` SET `quantity` = ABS(`quantity`)
  WHERE `movement_type` IN ('sale','delivery','adjustment_remove','return_supplier','consumo','transfer_out')
    AND `quantity` < 0;

-- 10. Menú ----------------------------------------------------------------------
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5005, 5000, 'Almacenes', '/warehouse/list', NULL, NULL, '/warehouse', 15, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5006, 5000, 'Consumo Interno', '/consumption/list', NULL, NULL, '/consumption', 25, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5007, 5006, 'Nuevo consumo', '/consumption/new', NULL, NULL, '/consumption/new', 10, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

-- 11. Permisos de los módulos nuevos --------------------------------------------
INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(46, 6, 'almacenes', 'almacenes', 1),
(47, 6, 'consumo_interno', 'consumo_interno', 1);

-- 12. Configuración de flujo de inventario y reservas -------------------------
-- El código leía estas dos tablas desde el principio, pero NINGUNA migración las
-- creaba: la configuración caía siempre al modo por defecto y las reservas no
-- existían. Se crean aquí con el comportamiento histórico (descontar al
-- facturar, sin reservas) para no cambiar nada por sorpresa.
CREATE TABLE IF NOT EXISTS `inventory_flow_config` (
  `tenant_id` VARCHAR(30) NOT NULL,
  `deduction_mode` VARCHAR(20) NOT NULL DEFAULT 'invoice',
  `enable_reservation` TINYINT NOT NULL DEFAULT 0,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`tenant_id`)
);

INSERT INTO `inventory_flow_config` (`tenant_id`, `deduction_mode`, `enable_reservation`, `updated_at`)
  SELECT 'default', 'invoice', 0, NOW()
  WHERE NOT EXISTS (SELECT 1 FROM `inventory_flow_config` WHERE `tenant_id` = 'default');

CREATE TABLE IF NOT EXISTS `stock_reservation` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` VARCHAR(30) NOT NULL,
  `warehouse_id` VARCHAR(30) NOT NULL DEFAULT 'WH-MAIN',
  `quantity` DECIMAL(18,2) NOT NULL DEFAULT 0.00,
  `reference_type` VARCHAR(30) NOT NULL DEFAULT '',
  `reference_id` INT NOT NULL DEFAULT 0,
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_res_product_status` (`product_id`, `status`),
  KEY `idx_res_warehouse` (`warehouse_id`)
);
