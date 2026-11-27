-- ---------------------------------------------------------------------------
-- 11. ACTIVOS FIJOS Y DEPRECIACIÓN (Fase 2 del plan de costos).
--
-- Adopta del script de costos lo que al núcleo le falta: activos fijos con
-- depreciación mensual, valor de libro y cuota acumulada. Todo se hace desde
-- Go (sin triggers ni procedimientos), porque un addon no puede crear
-- triggers y el modelo multiempresa no lo permite.
--
-- Regla de prorrateo (decisión escrita en docs/PLAN-ADOPCION-COSTOS.md §8.2):
-- por unidades producidas. Un activo no global está asignado a un producto
-- (product_id) y su cuota mensual se divide entre las unidades del período.
-- Un activo global (is_global=1) no tiene producto; su cuota se reparte entre
-- los productos que tienen el cost_center_id del activo, y cada producto
-- recibe su parte prorrateada sobre sus propias unidades.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `fixed_asset` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(40) NOT NULL,
  `name` VARCHAR(150) NOT NULL,
  `type` VARCHAR(40) NOT NULL DEFAULT 'equipo',
  `is_global` TINYINT NOT NULL DEFAULT 0,
  `product_id` VARCHAR(100),
  `cost_center_id` INT,
  `purchase_price` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `residual_value` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `purchase_date` DATE NOT NULL,
  `useful_life_months` INT NOT NULL DEFAULT 0,
  `active` TINYINT NOT NULL DEFAULT 1,
  `created_by` VARCHAR(60) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fixed_asset_code` (`code`),
  KEY `idx_fixed_asset_producto` (`product_id`),
  KEY `idx_fixed_asset_centro` (`cost_center_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Cuota calculada por activo: cuota mensual fija, con el mes en que se calculó.
-- Una fila por activo y mes; el cierre de período no la duplica.
CREATE TABLE IF NOT EXISTS `fixed_asset_depreciation` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `asset_id` INT NOT NULL,
  `period` VARCHAR(7) NOT NULL,
  `month_amount` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `accumulated` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `book_value` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `status` VARCHAR(20) NOT NULL DEFAULT 'calculado',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fixed_asset_dep` (`asset_id`, `period`),
  KEY `idx_fixed_asset_dep_asset` (`asset_id`),
  KEY `idx_fixed_asset_dep_period` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Movimiento mensual de depreciación: traza de qué se cargó a cada período.
-- Es la fuente que lee el motor para poner compDepreciation en el costo.
CREATE TABLE IF NOT EXISTS `fixed_asset_depreciation_movement` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `asset_id` INT NOT NULL,
  `period` VARCHAR(7) NOT NULL,
  `month_amount` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `accumulated` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `book_value` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `status` VARCHAR(20) NOT NULL DEFAULT 'calculado',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fixed_asset_mov` (`asset_id`, `period`),
  KEY `idx_fixed_asset_mov_asset` (`asset_id`),
  KEY `idx_fixed_asset_mov_period` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Columnas para bases que ya tienen fixed_asset sin product_id ni cost_center_id.
-- Sin product_id no se sabe a qué producto carga la depreciación; sin
-- cost_center_id no se sabe cómo prorratear un activo global.
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'fixed_asset'
    AND COLUMN_NAME = 'product_id'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `fixed_asset` ADD COLUMN `product_id` VARCHAR(100) AFTER `is_global`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'fixed_asset'
    AND COLUMN_NAME = 'cost_center_id'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `fixed_asset` ADD COLUMN `cost_center_id` INT AFTER `product_id`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- ---------------------------------------------------------------------------
-- 12c. PERMISOS DE MENÚ: activos fijos.
-- ---------------------------------------------------------------------------

INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(210, 1, 'cost_assets', 'cost_assets', 1);
