-- ---------------------------------------------------------------------------
-- 14. UNIDADES DE MEDIDA Y CONVERSIONES (Fase 4 del plan de costos).
--
-- product_information.unit es texto libre ("kg", "g", "unidad", "lt"). Para
-- que una receta en gramos sobre un costo por kilo no salga 1.000 veces mal,
-- se crea un catálogo de UoM y una tabla de conversiones por producto.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `unit_of_measure` (
  `code` VARCHAR(10) NOT NULL,
  `name` VARCHAR(60) NOT NULL,
  `allows_decimal` TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Conversión de unidades por producto: factor para pasar de from_uom a to_uom.
-- Ejemplo: producto "harina", from_uom="g", to_uom="kg", factor=0.001.
CREATE TABLE IF NOT EXISTS `product_uom_conversion` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` VARCHAR(100) NOT NULL,
  `from_uom` VARCHAR(10) NOT NULL,
  `to_uom` VARCHAR(10) NOT NULL,
  `factor` DECIMAL(18,6) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_uom_conversion` (`product_id`, `from_uom`, `to_uom`),
  KEY `idx_uom_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Semillas básicas de UoM (idempotentes: INSERT ... WHERE NOT EXISTS).
INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`)
SELECT 'kg', 'Kilogramo', 1
WHERE NOT EXISTS (SELECT 1 FROM `unit_of_measure` WHERE `code` = 'kg');

INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`)
SELECT 'g', 'Gramo', 1
WHERE NOT EXISTS (SELECT 1 FROM `unit_of_measure` WHERE `code` = 'g');

INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`)
SELECT 'un', 'Unidad', 0
WHERE NOT EXISTS (SELECT 1 FROM `unit_of_measure` WHERE `code` = 'un');

INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`)
SELECT 'lt', 'Litro', 1
WHERE NOT EXISTS (SELECT 1 FROM `unit_of_measure` WHERE `code` = 'lt');

INSERT INTO `unit_of_measure` (`code`, `name`, `allows_decimal`)
SELECT 'ml', 'Mililitro', 1
WHERE NOT EXISTS (SELECT 1 FROM `unit_of_measure` WHERE `code` = 'ml');

-- ---------------------------------------------------------------------------
-- 14b. VERSION Y RENDIMIENTO DE RECETA.
--
-- product_recipe gana version, active, batch_reference y yield_pct para
-- soportar varias versiones de receta por producto (una sola activa a la vez)
-- y el rendimiento de la receta (distinto del despiece de product_yield).
-- ---------------------------------------------------------------------------

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'uom'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `uom` VARCHAR(10) NOT NULL DEFAULT \'\' AFTER `unit`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'version'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `version` INT NOT NULL DEFAULT 1 AFTER `uom`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'active'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `active` TINYINT NOT NULL DEFAULT 1 AFTER `version`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'batch_reference'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `batch_reference` VARCHAR(40) NOT NULL DEFAULT \'\' AFTER `active`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'yield_pct'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `yield_pct` DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER `batch_reference`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- Índice para buscar la versión activa de un producto.
SET @idx := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND INDEX_NAME = 'idx_recipe_version_activa'
);
SET @sql := IF(@idx = 0,
  'ALTER TABLE `product_recipe` ADD KEY `idx_recipe_version_activa` (`product_id`, `active`, `version`)',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- ---------------------------------------------------------------------------
-- 14c. PERMISOS DE MENÚ: unidades de medida y conversiones.
-- ---------------------------------------------------------------------------

INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(212, 1, 'cost_uom', 'cost_uom', 1);
