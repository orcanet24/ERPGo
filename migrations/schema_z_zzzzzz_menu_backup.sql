-- Copias de la tabla menu para el botón Restaurar del gestor de menús.
--
-- Cada movimiento/reordenamiento del gestor guarda el estado ANTES de aplicar
-- el cambio (source='auto', se rotan quedando los últimos 20). El admin puede
-- además crear puntos manuales (source='manual', no se rotan) y volver a
-- cualquiera desde /config/menu.
--
-- Doctrina estricta: nunca se BORRA nada de menu — la restauración solo UPDATE
-- (posiciones y estado) y recrea menús presentes en la copia y ausentes en la
-- tabla actual (borrados por error).

CREATE TABLE IF NOT EXISTS `menu_backup` (
  `id`         BIGINT NOT NULL AUTO_INCREMENT,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `source`     VARCHAR(16) NOT NULL DEFAULT 'auto',
  PRIMARY KEY (`id`),
  KEY `idx_menu_backup_source` (`source`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `menu_backup_row` (
  `backup_id`     BIGINT NOT NULL,
  `menu_id`       int NOT NULL,
  `parent_id`     int DEFAULT NULL,
  `label`         varchar(128) NOT NULL DEFAULT '',
  `url`           varchar(255) DEFAULT NULL,
  `icon`          varchar(80) DEFAULT NULL,
  `perm`          varchar(120) DEFAULT NULL,
  `active_prefix` varchar(120) DEFAULT NULL,
  `sort_order`    int NOT NULL DEFAULT 0,
  `status`        tinyint(1) NOT NULL DEFAULT 1,
  `module_id`     varchar(64) NOT NULL DEFAULT 'core',
  `is_core`       tinyint(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (`backup_id`, `menu_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
