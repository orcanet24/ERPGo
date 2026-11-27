-- ---------------------------------------------------------------------------
-- 15. ÓRDENES DE PRODUCCIÓN (Fase 6 del plan de costos).
--
-- Una orden de producción convierte insumos en producto terminado según la
-- receta activa. Se costea siempre (calcula el costo real) y, si el módulo de
-- inventario está activo, postea el movimiento al kardex existente.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `production_order` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(40) NOT NULL,
  `product_id` VARCHAR(100) NOT NULL,
  `quantity` DECIMAL(14,4) NOT NULL DEFAULT 0,
  `period` VARCHAR(7) NOT NULL,
  `route_id` INT,
  `status` VARCHAR(20) NOT NULL DEFAULT 'abierta',
  `started_at` VARCHAR(32) NOT NULL DEFAULT '',
  `closed_at` VARCHAR(32) NOT NULL DEFAULT '',
  `created_by` VARCHAR(60) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_production_order_code` (`code`),
  KEY `idx_production_order_producto` (`product_id`),
  KEY `idx_production_order_periodo` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `production_order_line` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `order_id` INT NOT NULL,
  `component_id` VARCHAR(100) NOT NULL,
  `planned_qty` DECIMAL(14,4) NOT NULL DEFAULT 0,
  `actual_qty` DECIMAL(14,4) NOT NULL DEFAULT 0,
  `unit_cost` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `labor_grade` VARCHAR(30) NOT NULL DEFAULT '',
  `hours` DECIMAL(12,2) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_production_line_orden` (`order_id`),
  KEY `idx_production_line_componente` (`component_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 15b. PERMISOS DE MENÚ: órdenes de producción.
-- ---------------------------------------------------------------------------

INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(213, 1, 'cost_production', 'cost_production', 1);
