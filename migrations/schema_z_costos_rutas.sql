-- ---------------------------------------------------------------------------
-- 13. RUTAS DE PROCESO E INDIRECTO POR HORA (Fase 3 del plan de costos).
--
-- process_route ya existe desde el esquema base; este archivo asegura que
-- las columnas necesarias existan en bases que se migraron con una versión
-- anterior, y añade el permiso de menú para la pantalla de rutas.
-- ---------------------------------------------------------------------------

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'process_route'
    AND COLUMN_NAME = 'setup_time_min'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `process_route` ADD COLUMN `setup_time_min` DECIMAL(10,2) NOT NULL DEFAULT 0 AFTER `work_center_id`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'process_route'
    AND COLUMN_NAME = 'run_time_sec'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `process_route` ADD COLUMN `run_time_sec` DECIMAL(10,4) NOT NULL DEFAULT 0 AFTER `setup_time_min`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'process_route'
    AND COLUMN_NAME = 'machine_time_sec'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `process_route` ADD COLUMN `machine_time_sec` DECIMAL(10,4) NOT NULL DEFAULT 0 AFTER `run_time_sec`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'process_route'
    AND COLUMN_NAME = 'labor_grade'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `process_route` ADD COLUMN `labor_grade` VARCHAR(30) NOT NULL DEFAULT \'\' AFTER `machine_time_sec`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- ---------------------------------------------------------------------------
-- 13b. PERMISOS DE MENÚ: rutas de proceso.
-- ---------------------------------------------------------------------------

INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(211, 1, 'cost_routes', 'cost_routes', 1);
