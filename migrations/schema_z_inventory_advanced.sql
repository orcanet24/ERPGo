-- Módulos de inventario y compras: inventario cíclico, FEFO, punto de pedido y
-- tipo de cambio.
--
-- POR QUÉ UN ARCHIVO NUEVO Y NO AMPLIAR LOS VIEJOS.
--
-- schema.sql es el volcado del PHP original y no se toca: es la referencia de
-- lo que había. Lo que se añade va en su propio archivo, con prefijo `z` para
-- que se aplique después y con `zz` si algún día depende de esto.
--
-- Lo que hay aquí NO es decoración. Cada tabla existe porque hay una operación
-- que con el ERP actual no se puede hacer:
--
--   * Un conteo cíclico tiene que decir QUÉ se contaba, quién lo contó, cuánto
--     decía el sistema y cuánto se encontró. Sin tabla, un inventario general
--     es una nota en un papel que nadie puede auditar después.
--   * FEFO necesita un lote con fecha de caducidad. El ERP actual tiene
--     `batch` (el código del lote) pero no la fecha, y sin fecha no hay "sale
--     primero el que caduca antes": hay una conjetura.
--   * El punto de pedido sale del propio punto de equilibrio, así que se guarda
--     lo que lo alimenta (demanda media, plazo del proveedor, stock de
--     seguridad) para poder recalcularlo y explicar por qué cambió.
--   * El tipo de cambio es un dato por día, pero tiene que guardar QUIÉN lo
--     puso y de dónde salió: un bolívar que cambia de origen cambia las facturas
--     nuevas, no las viejas.

-- ---------------------------------------------------------------------------
-- 1. Lotes y caducidades (soporte de FEFO)
-- ---------------------------------------------------------------------------
-- Un lote es una cantidad concreta de un producto, con entrada y caducidad. Sin
-- esto, "el producto que caduca antes" no tiene a qué referirse.
CREATE TABLE IF NOT EXISTS `product_lot` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` INT NOT NULL,
  `lot_code` VARCHAR(64) NOT NULL DEFAULT '',
  `entry_date` DATE NOT NULL,
  `expiry_date` DATE DEFAULT NULL,
  `quantity_received` DOUBLE NOT NULL DEFAULT 0,
  `quantity_available` DOUBLE NOT NULL DEFAULT 0,
  `unit_cost` DOUBLE NOT NULL DEFAULT 0,
  `supplier_id` INTEGER DEFAULT NULL,
  `purchase_id` INTEGER DEFAULT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT 'active | depleted | expired | quarantined',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_lot_product` (`product_id`),
  KEY `idx_lot_expiry` (`expiry_date`),
  KEY `idx_lot_code` (`product_id`, `lot_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Una misma pareja producto/lote no puede repetirse: el lote identifica la
-- partida y sus entradas se acumulan en `quantity_received`. Sin este índice
-- único, dos entradas del mismo lote crean dos filas y FEFO suma el doble.
CREATE UNIQUE INDEX IF NOT EXISTS `uq_lot_product_code` ON `product_lot` (`product_id`, `lot_code`);

-- ---------------------------------------------------------------------------
-- 2. Alertas de caducidad
-- ---------------------------------------------------------------------------
-- Las alertas NO son un cálculo guardado: se derivan de `product_lot`. Se
-- guardan aquí solo las que el usuario ha marcado como vistas o diferidas, para
-- que un aviso no vuelva a salir cada mañana y para poder dejar escrito por qué
-- se difiere.
CREATE TABLE IF NOT EXISTS `expiry_alert_log` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `lot_id` INT NOT NULL,
  `product_id` INT NOT NULL,
  `days_left` INT NOT NULL DEFAULT 0,
  `severity` VARCHAR(20) NOT NULL DEFAULT 'info' COMMENT 'info | warning | critical',
  `acknowledged_by` VARCHAR(64) NOT NULL DEFAULT '',
  `acknowledged_at` VARCHAR(32) NOT NULL DEFAULT '',
  `note` VARCHAR(500) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_alert_lot` (`lot_id`),
  KEY `idx_alert_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 3. Inventario cíclico
-- ---------------------------------------------------------------------------
-- El conteo en sí y sus líneas. `expected_quantity` se COPIA del sistema en el
-- momento de abrir el conteo y no se vuelve a leer: si se leyera al cerrar,
-- estarías comparando el sistema consigo mismo después de que alguien escribiera
-- en él, que es como se cuela un descuadre sin que nadie lo note.
CREATE TABLE IF NOT EXISTS `cycle_count` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(32) NOT NULL DEFAULT '',
  `warehouse_id` INTEGER DEFAULT NULL,
  `scope` VARCHAR(20) NOT NULL DEFAULT 'category' COMMENT 'category | supplier | location | abc | full',
  `scope_value` VARCHAR(191) NOT NULL DEFAULT '',
  `status` VARCHAR(20) NOT NULL DEFAULT 'open' COMMENT 'open | counting | reviewed | closed | cancelled',
  `started_by` VARCHAR(64) NOT NULL DEFAULT '',
  `started_at` VARCHAR(32) NOT NULL DEFAULT '',
  `closed_by` VARCHAR(64) NOT NULL DEFAULT '',
  `closed_at` VARCHAR(32) NOT NULL DEFAULT '',
  `note` VARCHAR(500) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cycle_code` (`code`),
  KEY `idx_cycle_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cycle_count_line` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `cycle_id` INT NOT NULL,
  `product_id` INT NOT NULL,
  `lot_id` INTEGER DEFAULT NULL,
  `expected_quantity` DOUBLE NOT NULL DEFAULT 0,
  `counted_quantity` DOUBLE DEFAULT NULL COMMENT 'NULL = aún no contado',
  `variance` DOUBLE DEFAULT NULL,
  `unit_cost` DOUBLE NOT NULL DEFAULT 0,
  `variance_value` DOUBLE NOT NULL DEFAULT 0,
  `counted_by` VARCHAR(64) NOT NULL DEFAULT '',
  `counted_at` VARCHAR(32) NOT NULL DEFAULT '',
  `adjusted` INTEGER NOT NULL DEFAULT 0,
  `note` VARCHAR(500) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_cycle_line` (`cycle_id`, `product_id`, `lot_id`),
  KEY `idx_line_cycle` (`cycle_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- El asiento que el conteo genera al ajustar. Un conteo que corrige el stock
-- tiene que dejar el asiento contable de la corrección: si no, el inventario
-- cuadra con el almacén y no cuadra con la contabilidad, y el descuadre aparece
-- meses después, en el cierre, cuando ya nadie sabe por qué.
CREATE TABLE IF NOT EXISTS `cycle_count_adjustment` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `cycle_line_id` INT NOT NULL,
  `voucher_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'asiento que corrige el stock',
  `quantity_delta` DOUBLE NOT NULL DEFAULT 0,
  `value_delta` DOUBLE NOT NULL DEFAULT 0,
  `reason` VARCHAR(191) NOT NULL DEFAULT '' COMMENT 'merma | error de entrada | robo | ajuste',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_adjust_line` (`cycle_line_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 4. Punto de pedido / reposición
-- ---------------------------------------------------------------------------
-- La política se guarda POR PRODUCTO, no global, porque el punto de equilibrio
-- de un producto de alta rotación y uno de temporada no tienen nada que ver. Se
-- guarda lo que la alimenta para poder explicar el número y recalcularlo sin
-- tener que volver a las ventas de hace seis meses.
CREATE TABLE IF NOT EXISTS `reorder_policy` (
  `product_id` INT NOT NULL,
  `lead_time_days` INT NOT NULL DEFAULT 7 COMMENT 'plazo del proveedor en días',
  `safety_days` INT NOT NULL DEFAULT 2 COMMENT 'días de consumo que se dejan como colchón',
  `min_order_qty` DOUBLE NOT NULL DEFAULT 0,
  `pack_size` DOUBLE NOT NULL DEFAULT 1 COMMENT 'múltiplo del pedido: cajas, sacos',
  `avg_daily_demand` DOUBLE NOT NULL DEFAULT 0 COMMENT 'demanda media, calculada por el sistema',
  `computed_on` VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'fecha del último cálculo',
  `active` INTEGER NOT NULL DEFAULT 1,
  `note` VARCHAR(500) NOT NULL DEFAULT '',
  `updated_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- El resultado del cálculo, uno por producto que cae por debajo del punto.
-- Es una foto, no un estado vivo: recalcular no lo modifica, lo vuelve a
-- escribir con fecha nueva. Guardar solo "lo que hay que pedir ahora" y no su
-- historia hace imposible responder "¿por qué el mes pasado pedí 300 y este
-- mes 80?".
CREATE TABLE IF NOT EXISTS `reorder_suggestion` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `product_id` INT NOT NULL,
  `suggested_date` DATE NOT NULL,
  `on_hand` DOUBLE NOT NULL DEFAULT 0,
  `reorder_point` DOUBLE NOT NULL DEFAULT 0 COMMENT 'stock de seguridad + consumo del plazo',
  `suggested_qty` DOUBLE NOT NULL DEFAULT 0,
  `pack_size` DOUBLE NOT NULL DEFAULT 1,
  `estimated_cost` DOUBLE NOT NULL DEFAULT 0,
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending | ordered | dismissed',
  `dismiss_reason` VARCHAR(255) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_suggestion_product` (`product_id`, `suggested_date`),
  KEY `idx_suggestion_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- 5. Tipo de cambio BCV / paralelo
-- ---------------------------------------------------------------------------
-- Un dato por día y por fuente, con su procedencia. La diferencia entre el BCV y
-- el paralelo no es un dato que se pueda descartar: los contratos y los precios
-- del proveedor se pactan en una tasa y las facturas se emiten en otra, y esa
-- diferencia es la que se lleva la diferencia entre lo que se pagó y lo que se
-- facturó.
CREATE TABLE IF NOT EXISTS `exchange_rate_source` (
  `code` VARCHAR(20) NOT NULL PRIMARY KEY COMMENT 'BCV | paralelo | oficial | banco',
  `name` VARCHAR(64) NOT NULL DEFAULT '',
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `active` INTEGER NOT NULL DEFAULT 1
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `exchange_rate_daily` (
  `rate_date` DATE NOT NULL,
  `currency` VARCHAR(8) NOT NULL DEFAULT 'USD',
  `source` VARCHAR(20) NOT NULL DEFAULT 'BCV',
  `buy_rate` DOUBLE NOT NULL DEFAULT 0,
  `sell_rate` DOUBLE NOT NULL DEFAULT 0 COMMENT '0 = no se registró, se usa buy_rate',
  `official_rate` DOUBLE NOT NULL DEFAULT 0 COMMENT 'tasa oficial del BCV del día',
  `is_parallel` INTEGER NOT NULL DEFAULT 0 COMMENT '1 = tasa paralela',
  `entered_by` VARCHAR(64) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`rate_date`, `currency`, `source`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `exchange_rate_source` (`code`, `name`, `description`, `active`) VALUES
  ('BCV', 'Banco Central de Venezuela', 'Tasa oficial publicada por el BCV.', 1),
  ('paralelo', 'Paralelo', 'Tasa del mercado paralelo. Es un dato de referencia, no oficial.', 1),
  ('oficial', 'Oficial', 'Tasa oficial del día comunicada por el Banco.', 1),
  ('banco', 'Banco', 'Tasa a la que compra el banco del cliente.', 1);

-- ---------------------------------------------------------------------------
-- 6. Consumos para el punto de pedido
-- ---------------------------------------------------------------------------
-- El consumo diario por producto. Se guarda en lugar de recalcularlo siempre
-- sobre `invoice_details` porque el cálculo del punto de pedido se ejecuta
-- eligiendo el menú, no en un informe: con años de líneas, recalcular en cada
-- visita es lo que hace que la pantalla tarde. Guardarlo tiene además una
-- ventaja mejor: se puede corregir a mano cuando una venta fue una corrección de sistema.
CREATE TABLE IF NOT EXISTS `product_daily_demand` (
  `product_id` INT NOT NULL,
  `demand_date` DATE NOT NULL,
  `quantity` DOUBLE NOT NULL DEFAULT 0,
  `source` VARCHAR(20) NOT NULL DEFAULT 'auto' COMMENT 'auto | manual',
  `note` VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`product_id`, `demand_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

