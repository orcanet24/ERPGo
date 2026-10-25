CREATE TABLE IF NOT EXISTS `addons` (
    `id` VARCHAR(64) NOT NULL COMMENT 'code del addon, igual al nombre del directorio',
    `name` VARCHAR(128) NOT NULL COMMENT 'nombre visible',
    `version` VARCHAR(32) NOT NULL DEFAULT '1.0.0',
    `description` TEXT,
    `author_name` VARCHAR(128),
    `author_email` VARCHAR(128),
    `license_type` VARCHAR(32) DEFAULT 'propietario',
    `addon_path` VARCHAR(255) NOT NULL COMMENT 'ruta relativa a addons/',
    `entrypoint` VARCHAR(128) COMMENT 'binario a ejecutar (si tiene UI/API propia)',
    `is_active` TINYINT(1) DEFAULT 0 COMMENT '0=descubierto, 1=activo',
    `web` TINYINT(1) DEFAULT 0 COMMENT 'tiene templates web',
    `grpc` TINYINT(1) DEFAULT 0 COMMENT 'implementa servidor gRPC',
    `needs_migration` TINYINT(1) DEFAULT 0 COMMENT 'tiene migraciones pendientes',
    `manifest_json` LONGTEXT COMMENT 'manifiesto verificado del addon',
    `installed_at` TIMESTAMP NULL DEFAULT NULL,
    `status` VARCHAR(16) NOT NULL DEFAULT 'detected',
    `binary_sha256` CHAR(64) NOT NULL DEFAULT '',
    `signature` VARCHAR(255) NOT NULL DEFAULT '',
    `permissions_json` TEXT NOT NULL,
    `last_error` TEXT,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_addons_active` (`is_active`),
    KEY `idx_addons_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `addon_role_permissions` (
    `addon_code` VARCHAR(64) NOT NULL,
    `permission_code` VARCHAR(64) NOT NULL,
    `role_id` INT NOT NULL,
    `is_allowed` TINYINT(1) NOT NULL DEFAULT 0,
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`addon_code`, `permission_code`, `role_id`),
    KEY `idx_addon_role_permission_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
