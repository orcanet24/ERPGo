-- ---------------------------------------------------------------------------
-- Sistema de costos: recetas, rendimiento, horas, importación y listas de precio.
--
-- POR QUÉ ESTE FICHERO.
--
-- El ERP costea con Costo Promedio Ponderado (kardex.go), que responde "cuánto
-- me costó COMPRARLO". Cuando la empresa fabrica, esa respuesta no alcanza: no
-- dice cuánto cuesta HACERLO. El ejemplo de la panadería lo explica mejor que
-- cualquier teoría: si un maestro y un ayudante trabajan 2 horas en un pastel,
-- el costo no es 2*(X+Y) ni un promedio, sino una función del tiempo por rol,
-- la merma, la depreciación del horno y las cargas patronales.
--
-- Cuatro empresas van a usar esto, y cada una es un tipo distinto:
--
--   1. Servicios de software  -> costo por HORAS (sin inventario)
--   2. Textilera              -> RECETA con merma + IMPORTACIÓN
--   3. Carnicería             -> RENDIMIENTO de despiece + mayor/detalle
--   4. Autopartes             -> COMPRA + IMPORTACIÓN, sin fabricar
--
-- Si el módulo sirve a las cuatro, sirve; si solo sabe costear carne, es un
-- módulo de carne con otro nombre.
--
-- DINERO: decimal, nunca DOUBLE (ver 0.3 del plan). Se usa decimal(18,4) para
-- costos unitarios —que es lo que ya hace product_information.cost— y
-- decimal(18,2) para importes de cabecera.
--
-- Orden: va DESPUÉS de schema_z_hr.sql (hr_employee) y de
-- schema_z_inventory_advanced.sql (product_lot). El prefijo `z_costos` lo deja
-- al final.
--
-- Idempotente: se puede aplicar sobre bases ya migradas.
-- ---------------------------------------------------------------------------

-- ---------------------------------------------------------------------------
-- 0. HOJA DE HORAS. Es lo primero porque sin esto no existe el costo laboral.
--
-- hr_payroll_item tiene amount, rate_pct y base_amount, pero NINGUNA columna de
-- horas. Se puede pagar a alguien y no saber cuántas horas trabajó, con lo que
-- el costo por proyecto de la empresa de servicios no se puede calcular: no hay
-- de dónde sacarlo.
--
-- No se mete en hr_payroll_item a propósito: esa tabla registra renglones de
-- NÓMINA (se paga), esta registra TIEMPO TRABAJADO (se costea). Metiendo las
-- dos en una, un ajuste de nómina retroactivo cambiaría el costo de proyectos
-- ya facturados.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cost_timesheet` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `employee_id` INT NOT NULL,
  `work_date` DATE NOT NULL,
  `hours` DECIMAL(10,2) NOT NULL DEFAULT 0,
  -- 1 = se factura al cliente, 0 = es costo del negocio (reuniones, soporte,
  -- formación). Las horas no facturables son costo REAL pero no ingresan: si se
  -- mezclan, el margen sale inflado y nadie ve el error hasta el cierre.
  `billable` TINYINT NOT NULL DEFAULT 1,
  `project_id` INT DEFAULT NULL,
  `product_id` VARCHAR(100) DEFAULT NULL,
  -- El rol con el que se registra. Un maestro y un ayudante tienen tarifas
  -- distintas: sin esto, las horas de los dos se cobran igual.
  `grade` VARCHAR(30) NOT NULL DEFAULT '',
  `notes` VARCHAR(255) NOT NULL DEFAULT '',
  `created_by` VARCHAR(60) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_ts_empleado_fecha` (`employee_id`, `work_date`),
  KEY `idx_ts_proyecto` (`project_id`),
  KEY `idx_ts_fecha` (`work_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 1. RECETA / BOM. Para lo que se fabrica (textilera, panadería).
--
-- loss_factor va por COMPONENTE y no global: en la textilera la tela se pierde
-- en el corte y el botón no se pierde. Un merma única del 5% sobre toda la
-- receta carga la culpa a los componentes que no tienen.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `product_recipe` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` VARCHAR(100) NOT NULL,
  `component_id` VARCHAR(100) NOT NULL,
  `quantity` DECIMAL(14,4) NOT NULL DEFAULT 0,
  `unit` VARCHAR(20) NOT NULL DEFAULT '',
  `loss_factor` DECIMAL(6,4) NOT NULL DEFAULT 0,
  `step_order` INT NOT NULL DEFAULT 0,
  `is_substitute` TINYINT NOT NULL DEFAULT 0,
  -- Mano de obra estándar DENTRO de la receta. Un renglón con labor_grade no
  -- es material: aporta horas × tarifa del grado. Así una panadería costea
  -- harina y horas de horneado en el mismo producto, sin un método aparte.
  `labor_grade` VARCHAR(30) NOT NULL DEFAULT '',
  `hours` DECIMAL(12,2) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_recipe` (`product_id`, `component_id`, `step_order`),
  KEY `idx_recipe_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Columnas de mano de obra para las bases que ya tienen product_recipe. Sin
-- esto, una base existente sigue con la receta de solo materiales y el motor
-- no puede distinguir un renglón de labor de uno de material.
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'labor_grade'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `labor_grade` VARCHAR(30) NOT NULL DEFAULT '''' AFTER `is_substitute`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_recipe'
    AND COLUMN_NAME = 'hours'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_recipe` ADD COLUMN `hours` DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER `labor_grade`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- ---------------------------------------------------------------------------
-- 2. RENDIMIENTO DE DESPIECE. Carnicería.
--
-- De una canal de 250 kg salen 180 kg de venta y 70 kg de hueso, merma y
-- descarte. El costo de la canal NO se reparte 250/250 entre los cortes: se
-- reparte sobre los kilos que efectivamente salen a venta.
--
-- Sin esto, el precio de venta queda por debajo de lo que costó la canal y la
-- empresa pierde dinero sin que nada se lo diga. Es el caso que más fácil se
-- hace mal.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `product_yield` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `input_product_id` VARCHAR(100) NOT NULL,
  `output_product_id` VARCHAR(100) NOT NULL,
  `expected_pct` DECIMAL(6,4) NOT NULL DEFAULT 0,
  `is_byproduct` TINYINT NOT NULL DEFAULT 0,
  `notes` VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_yield` (`input_product_id`, `output_product_id`),
  KEY `idx_yield_entrada` (`input_product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 3. GRADOS DE MANO DE OBRA. Qué vale la hora de cada rol.
--
-- hourly_rate es una columna NORMAL y no GENERATED. Con una columna generada,
-- cambiar el salario re-escribe retroactivamente la tasa y el costo de proyectos
-- ya facturados cambia solo, sin que nadie lo toque: se pierde la trazabilidad
-- que este mismo documento pide en Compliance. La tasa se calcula en Go al
-- usarla y se GUARDA en el asiento de costo.
--
-- Sin sueldo calculado: no es un campo que nadie escribe de una vez y se
-- mantiene; se recalcula del mismo modo que la tasa, en el momento de usarla.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `labor_grade` (
  `code` VARCHAR(30) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `base_salary_annual` DECIMAL(16,2) NOT NULL DEFAULT 0,
  `overhead_pct` DECIMAL(6,2) NOT NULL DEFAULT 0,
  `hours_per_year` INT NOT NULL DEFAULT 2080,
  `productivity_factor` DECIMAL(6,4) NOT NULL DEFAULT 1.0000,
  `active` TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 4. CENTROS DE COSTO. Dónde se consume el costo indirecto.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cost_center` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(20) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `type` VARCHAR(20) NOT NULL DEFAULT 'production',
  `overhead_rate_hour` DECIMAL(16,4) NOT NULL DEFAULT 0,
  `overhead_rate_unit` DECIMAL(16,4) NOT NULL DEFAULT 0,
  `active` TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_center_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 5. RUTA DE PROCESO. Cuánto tarda hacer una unidad en cada estación.
--
-- setup_time_min NO se prorratea entre unidades: es tiempo de preparación que se
-- paga aunque se fabrique una sola pieza. Si se divide por la cantidad, el
-- primer lote sale absurdamente barato.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `process_route` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` VARCHAR(100) NOT NULL,
  `sequence` INT NOT NULL DEFAULT 0,
  `work_center_id` INT NOT NULL,
  `setup_time_min` DECIMAL(10,2) NOT NULL DEFAULT 0,
  `run_time_sec` DECIMAL(10,4) NOT NULL DEFAULT 0,
  `machine_time_sec` DECIMAL(10,4) NOT NULL DEFAULT 0,
  `labor_grade` VARCHAR(30) NOT NULL DEFAULT '',
  `active` TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  KEY `idx_route_producto` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 6. COSTO DE IMPORTACIÓN (landed cost). Textilera y autopartes.
--
-- El gasto es de la PARTIDA y se reparte entre las unidades que realmente
-- recibió esa partida. NO se guarda como "costo por unidad del producto": si se
-- dividiera por la cantidad pedida en lugar de la recibida, el costo del
-- producto cambiaría según para cuántas unidades se pidió, no según lo que
-- costó el mercado.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `import_cost` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `purchase_id` BIGINT NOT NULL,
  `freight` DECIMAL(16,2) NOT NULL DEFAULT 0,
  `customs` DECIMAL(16,2) NOT NULL DEFAULT 0,
  `insurance` DECIMAL(16,2) NOT NULL DEFAULT 0,
  `other` DECIMAL(16,2) NOT NULL DEFAULT 0,
  `received_qty` DECIMAL(14,4) NOT NULL DEFAULT 0,
  `currency` VARCHAR(5) NOT NULL DEFAULT 'USD',
  `exchange_rate` DECIMAL(18,4) NOT NULL DEFAULT 1,
  `notes` VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_import_purchase` (`purchase_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 7. LISTAS DE PRECIO. Al mayor y al detalle.
--
-- Sin esto, un corte de carne tiene UN precio y la carnicería no puede vender a
-- dos. Las tres empresas que venden a dos precios (textilera, carnicería,
-- autopartes) la necesitan; la de servicios no, y no debe verse obligada a
-- llenarla.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `price_list` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(30) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `sort_order` INT NOT NULL DEFAULT 0,
  `is_default` TINYINT NOT NULL DEFAULT 0,
  `active` TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_price_list_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `product_price_list` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` VARCHAR(100) NOT NULL,
  `price_list_id` INT NOT NULL,
  `price` DECIMAL(16,4) NOT NULL DEFAULT 0,
  `margin_pct` DECIMAL(8,4) NOT NULL DEFAULT 0,
  `updated_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_producto_lista` (`product_id`, `price_list_id`),
  KEY `idx_ppl_lista` (`price_list_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 8. CONFIGURACIÓN DE COSTEO POR PRODUCTO.
--
-- cost_pp (Costo Promedio Ponderado) es el default a propósito: es lo que el ERP
-- ya hace, de modo que un producto sin configuración no cambia de comportamiento
-- y ninguna instalación existente se rompe al aplicar esta migración.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `product_cost_config` (
  `product_id` VARCHAR(100) NOT NULL,
  `costing_method` VARCHAR(10) NOT NULL DEFAULT 'PP',
  -- Merma GLOBAL del producto, decimal(6,4) = 0.0250 es 2,5%.
  --
  -- ESTA COLUMNA FALTABA, Y SU AUSENCIA ROMPÍA EL MOTOR ENTERO. Es el bug 0.7.
  --
  -- `cost_engine.go` hacia `SELECT costing_method, IFNULL(loss_factor,0) FROM
  -- product_cost_config`; la columna no existía en el CREATE TABLE, así que la
  -- consulta devolvía ERROR 1054 (Unknown column), el motor se tragaba el
  -- error con `_ = err` y se quedaba con los valores por defecto. El método de
  -- costeo configurado NUNCA se leía: todo salía PP, y RECIPE, YIELD, HOURS e
  -- IMPORT eran código muerto al que no se llegaba nunca.
  --
  -- La moraleja está en el PATRON-UNICO-DE-DESARROLLO.md: un error de lectura
  -- tragado que deja el sistema en un valor por defecto razonable es el peor
  -- tipo de bug, porque no se ve. Por eso aquí, además de la columna, el motor
  -- registra el fallo en `Faltantes` en vez de ignorarlo.
  `loss_factor` DECIMAL(6,4) NOT NULL DEFAULT 0.0000 COMMENT 'Merma global del producto, 0.0250 = 2,5%. Solo afecta al material.',
  `perishable` TINYINT NOT NULL DEFAULT 0,
  `shelf_life_days` INT NOT NULL DEFAULT 0,
  `is_assembled` TINYINT NOT NULL DEFAULT 0,
  `track_lots` TINYINT NOT NULL DEFAULT 0,
  `track_serial` TINYINT NOT NULL DEFAULT 0,
  `imported` TINYINT NOT NULL DEFAULT 0,
  `cost_center_id` INT DEFAULT NULL,
  `updated_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`),
  KEY `idx_pcc_metodo` (`costing_method`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- La misma columna, para las bases que YA se migraron antes de que existiera.
--
-- MySQL no tiene "ADD COLUMN IF NOT EXISTS" hasta 8.0.29, y esta migración tiene
-- que correr también en versiones anteriores. La forma idiomática yPortable es
-- information_schema: si la columna no está, se añade. Es idempotente, que es
-- lo que permite aplicar este fichero dos veces seguidas.
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_cost_config'
    AND COLUMN_NAME = 'loss_factor'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_cost_config` ADD COLUMN IF NOT EXISTS `loss_factor` DECIMAL(6,4) NOT NULL DEFAULT 0.0000',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- ---------------------------------------------------------------------------
-- 9. COMPONENTE DE COSTO. El desglose con TRAZABILIDAD.
--
-- Es la tabla que hace defendible el módulo. Cada renglón dice qué parte del
-- costo es, cuánto valía, con qué tasa se calculó y en qué período.
--
-- Sin esto, recalcular un mes pasado devuelve otro número y no hay forma de
-- saber cuál de los dos es el correcto. Con RISLR, un costo que no se puede
-- reproducir no es un costo, es una estimación.
--
-- La TRAZABILIDAD va en la primera fase y no en "visión de futuro".
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cost_component` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `period` VARCHAR(7) NOT NULL,
  `product_id` VARCHAR(100) NOT NULL,
  `component` VARCHAR(20) NOT NULL,
  -- Número de renglón DENTRO del componente. Una receta con dos materiales
  -- produce dos trazas `material` en el mismo período y producto, y la clave
  -- única anterior (period, product_id, component) solo admitía una: el
  -- segundo INSERT rompía el cálculo. line_no conserva la unicidad sin perder
  -- el detalle de cada componente.
  `line_no` INT NOT NULL DEFAULT 1,
  `amount` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `quantity` DECIMAL(14,4) NOT NULL DEFAULT 0,
  `unit_cost` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `grade` VARCHAR(30) NOT NULL DEFAULT '',
  `hours` DECIMAL(12,2) NOT NULL DEFAULT 0,
  `rate_used` DECIMAL(18,6) NOT NULL DEFAULT 0,
  `source_ref` VARCHAR(60) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_component_linea` (`period`, `product_id`, `component`, `line_no`),
  KEY `idx_cc_periodo` (`period`),
  KEY `idx_cc_producto` (`product_id`, `period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- La columna y la clave nueva, para las bases que YA se migraron con la clave
-- antigua. Sin este bloque, una base existente conserva
-- UNIQUE(period, product_id, component) y el segundo material de una receta
-- sigue rompiendo el guardado. information_schema es la forma portable:
-- ADD COLUMN IF NOT EXISTS no existe antes de MySQL 8.0.29.
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cost_component'
    AND COLUMN_NAME = 'line_no'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `cost_component` ADD COLUMN `line_no` INT NOT NULL DEFAULT 1 AFTER `component`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @idx := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cost_component'
    AND INDEX_NAME = 'uq_cost_component'
);
SET @sql := IF(@idx > 0,
  'ALTER TABLE `cost_component` DROP INDEX `uq_cost_component`',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

SET @idx := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'cost_component'
    AND INDEX_NAME = 'uq_cost_component_linea'
);
SET @sql := IF(@idx = 0,
  'ALTER TABLE `cost_component` ADD UNIQUE KEY `uq_cost_component_linea` (`period`, `product_id`, `component`, `line_no`)',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- ---------------------------------------------------------------------------
-- 10. COSTO CONGELADO POR PERÍODO.
--
-- cost_snapshot guarda el costo de cada producto al CERRAR el período. Después
-- de cerrar, recalcula y no cambia nada: lo que ya se facturó no se re-escribe.
--
-- Es el equivalente contable de acc_period_close (que ya existe en el ERP para
-- las cuentas): lo mismo que no se abre un ejercicio fiscal ya cerrado, tampoco
-- se recalcula un costo ya congelado.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cost_snapshot` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `period` VARCHAR(7) NOT NULL,
  `product_id` VARCHAR(100) NOT NULL,
  `costing_method` VARCHAR(10) NOT NULL DEFAULT 'PP',
  `final_cost` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `material` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `labor` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `depreciation` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `overhead` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `customs` DECIMAL(18,4) NOT NULL DEFAULT 0,
  `loss_factor` DECIMAL(6,4) NOT NULL DEFAULT 0,
  `calc_method` VARCHAR(255) NOT NULL DEFAULT '',
  `closed_by` VARCHAR(60) NOT NULL DEFAULT '',
  `closed_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cost_snapshot` (`period`, `product_id`),
  KEY `idx_snapshot_periodo` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 11. SEMBRADOS.
--
-- Se usa WHERE NOT EXISTS y no INSERT IGNORE. Con INSERT IGNORE, una migración
-- posterior pisaría lo que el administrador haya escrito o haya añadido; con
-- WHERE NOT EXISTS, solo se siembra la fila si no existe.
-- ---------------------------------------------------------------------------

INSERT INTO `labor_grade` (`code`, `name`, `base_salary_annual`, `overhead_pct`, `hours_per_year`, `productivity_factor`)
SELECT 'maestro', 'Maestro / especialista', 0, 34.00, 2080, 1.0000
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `code` FROM `labor_grade` WHERE `code`='maestro') `t`);

INSERT INTO `labor_grade` (`code`, `name`, `base_salary_annual`, `overhead_pct`, `hours_per_year`, `productivity_factor`)
SELECT 'ayudante', 'Ayudante / junior', 0, 34.00, 2080, 1.0000
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `code` FROM `labor_grade` WHERE `code`='ayudante') `t`);

INSERT INTO `labor_grade` (`code`, `name`, `base_salary_annual`, `overhead_pct`, `hours_per_year`, `productivity_factor`)
SELECT 'supervisor', 'Supervisor', 0, 34.00, 2080, 1.0000
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `code` FROM `labor_grade` WHERE `code`='supervisor') `t`);

INSERT INTO `price_list` (`code`, `name`, `sort_order`, `is_default`)
SELECT 'detalle', 'Venta a detalle', 1, 1
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `code` FROM `price_list` WHERE `code`='detalle') `t`);

INSERT INTO `price_list` (`code`, `name`, `sort_order`, `is_default`)
SELECT 'mayor', 'Venta al mayor', 2, 0
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `code` FROM `price_list` WHERE `code`='mayor') `t`);

-- ---------------------------------------------------------------------------
-- 12b. PERMISOS DE MENÚ.
--
-- SIN ESTE BLOQUE EL MÓDULO EXISTE Y NO SE VE. El layout condiciona cada
-- sección a (index .Perm "<directory>"), y .Perm sale de la columna `directory`
-- de `sub_module`. Sin la fila, la sección no se pinta y el módulo se llega
-- solo por URL. Ese fue un fallo real del módulo: `grep` de `sub_module` sobre
-- 'cost' daba cero filas.
--
-- El `directory` debe COINCIDIR con la URL de la pantalla, no con el nombre de
-- la plantilla: aquí el handler pinta "cost_home.html" pero la ruta es "/cost",
-- y lo que la URL resuelve al primer segmento es "cost".
--
-- mid=1 (invoice) es un módulo padre que ya existe. INSERT IGNORE por la clave
-- primaria (id), y se eligió un id alto y fijo para no chocar con los 1..15 que
-- ya usan los módulos antiguos.
-- ---------------------------------------------------------------------------

INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(200, 1, 'cost', 'cost', 1),
(201, 1, 'cost_timesheet', 'cost_timesheet', 1),
(202, 1, 'cost_close', 'cost_close', 1);

-- ---------------------------------------------------------------------------
-- 13. CALENDARIO DE CIERRE POR PRODUCTO.
--
-- Se guarda cuándo se congeló cada fila de cost_snapshot. Sobre la misma idea de
-- cost_component: un costo sin fecha no se puede defender.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cost_period_close` (
  `period` VARCHAR(7) NOT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'abierto',
  `products_closed` INT NOT NULL DEFAULT 0,
  `reopened_reason` VARCHAR(255) NOT NULL DEFAULT '',
  `closed_by` VARCHAR(60) NOT NULL DEFAULT '',
  `closed_at` VARCHAR(32) NOT NULL DEFAULT '',
  `reopened_by` VARCHAR(60) NOT NULL DEFAULT '',
  `reopened_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
