-- ---------------------------------------------------------------------------
-- PLANTILLA: migración de un módulo nuevo.
--
-- COPIAR A: migrations/schema_z_<MODULO>.sql
--
-- CÓMO SE USA
-- 1. Sustituye <MODULO> por el nombre del módulo, en minúsculas.
-- 2. Borra las tablas de ejemplo y escribe las tuyas.
-- 3. Borra el bloque de permisos si tu módulo NO necesita entrada en el menú.
-- 4. Aplícala DOS veces contra erpgo_default. Las dos deben salir con 0.
--
-- REGLAS QUE ESTE FICHERO YA APLICA (ver doc/PATRON-UNICO-DE-DESARROLLO.md):
--   - CREATE TABLE IF NOT EXISTS, siempre. Que transformStatement lo arregle
--     no significa que el fichero se pueda leer por sí solo.
--   - Dinero en decimal: 18,2 para importes de cabecera, 18,4 para costos
--     unitarios. NUNCA DOUBLE ni FLOAT.
--   - utf8mb4_unicode_ci en tablas y en texto.
--   - Comment de columna en las tablas que la IA va a ver: es lo que el
--     introspector lee como descripción de negocio.
--   - Sin columna GENERATED para nada que se congele en un documento.
-- ---------------------------------------------------------------------------

-- ---------------------------------------------------------------------------
-- 1. TABLAS
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `<MODULO>_main` (
  `id`          INT AUTO_INCREMENT PRIMARY KEY,
  `code`        VARCHAR(50)  NOT NULL COMMENT 'Código corto y único del registro.',
  `description` VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'Descripción legible.',
  `quantity`    DECIMAL(18,4) NOT NULL DEFAULT 0.0000 COMMENT 'Cantidad. 4 decimales por unidad.',
  `amount`      DECIMAL(18,2) NOT NULL DEFAULT 0.00 COMMENT 'Importe de cabecera en dinero.',
  -- Lo que se usó para producir un documento CONGELADO va en columna NORMAL,
  -- no GENERATED: si la tasa o el precio cambia, una columna generada
  -- reescribe el histórico (bug 0.4 del módulo de costos).
  `rate_used`   DECIMAL(18,4) NOT NULL DEFAULT 0.0000 COMMENT 'Tasa realmente usada, copiada al calcular.',
  `period`      DATE         NULL COMMENT 'Período al que pertenece el registro.',
  `status`      TINYINT(1)   NOT NULL DEFAULT 1 COMMENT '1=activo, 0=inactivo.',
  `created_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uq_<MODULO>_code` (`code`),
  KEY `idx_<MODULO>_period` (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='<MODULO>: cabecera. <qué es y qué guarda.>';

CREATE TABLE IF NOT EXISTS `<MODULO>_line` (
  `id`        INT AUTO_INCREMENT PRIMARY KEY,
  `main_id`   INT          NOT NULL COMMENT 'FK a <MODULO>_main. Sin CONSTRAINT porque el ERP no las declara.',
  `item_id`   INT          NOT NULL COMMENT 'Producto o tercero.',
  `quantity`  DECIMAL(18,4) NOT NULL DEFAULT 0.0000,
  `rate`      DECIMAL(18,4) NOT NULL DEFAULT 0.0000 COMMENT 'Precio unitario pactado.',
  `amount`    DECIMAL(18,2) NOT NULL DEFAULT 0.00 COMMENT 'quantity * rate, redondeado a 2.',
  KEY `idx_<MODULO>_line_main` (`main_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='<MODULO>: renglones. Une con <MODULO>_main por main_id.';

-- ---------------------------------------------------------------------------
-- 2. PERMISOS DE MENÚ
--
-- SIN ESTE BLOQUE EL MÓDULO EXISTE Y NO SE VE. Se llega por URL y el usuario
-- no sabe que está ahí. Verificado en la base: los módulos de conteo cíclico,
-- FEFO, punto de pedido y tipo de cambio no tienen entrada en sub_module.
--
-- El `directory` DEBE COINCIDIR con el nombre de la plantilla que el handler
-- renderiza, sin la extensión:
--     handler renderiza "<MODULO>.html"  ->  directory = "<MODULO>"
--
-- `mid` debe existir ya en la tabla `module`. Usa un módulo padre existente
-- (por ejemplo 12 = hrm_management) o da de alta el suyo en el bloque de abajo.
-- ---------------------------------------------------------------------------

-- INSERT IGNORE INTO `module` (`id`, `name`, `description`, `directory`, `status`) VALUES
-- (99, '<MODULO>', NULL, NULL, 1);

-- INSERT IGNORE INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
-- (99, 12, '<MODULO>', '<MODULO>', 1);

-- ---------------------------------------------------------------------------
-- 3. DATOS MAESTRO MÍNIMOS
--
-- Solo si el módulo no arranca sin ellos. INSERT IGNORE, siempre: la migración
-- se aplica sobre bases ya sembradas.
-- ---------------------------------------------------------------------------

-- INSERT IGNORE INTO `<MODULO>_main` (`code`, `description`, `amount`)
-- VALUES ('DEFAULT', 'Configuración por defecto', 0.00);