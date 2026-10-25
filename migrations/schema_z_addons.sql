-- Addon catalog and role grants. The registry remains distinct from ERPGo's
-- core menu taxonomy; only the master writes menu rows for approved addons.
CREATE TABLE IF NOT EXISTS `addons` (
  `id` varchar(64) NOT NULL,
  `name` varchar(128) NOT NULL,
  `version` varchar(32) NOT NULL,
  `description` text,
  `author_name` varchar(128) DEFAULT NULL,
  `author_email` varchar(255) DEFAULT NULL,
  `license_type` varchar(64) DEFAULT NULL,
  `manifest_json` longtext NOT NULL,
  `addon_path` varchar(255) NOT NULL,
  `entrypoint` varchar(255) NOT NULL DEFAULT '',
  `is_active` tinyint(1) NOT NULL DEFAULT 0,
  `web` tinyint(1) NOT NULL DEFAULT 0,
  `grpc` tinyint(1) NOT NULL DEFAULT 1,
  `needs_migration` tinyint(1) NOT NULL DEFAULT 0,
  `status` varchar(16) NOT NULL DEFAULT 'detected',
  `binary_sha256` char(64) NOT NULL DEFAULT '',
  `signature` varchar(255) NOT NULL DEFAULT '',
  `permissions_json` text NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `installed_at` datetime DEFAULT NULL,
  `last_error` text,
  PRIMARY KEY (`id`),
  KEY `idx_addons_active` (`is_active`),
  KEY `idx_addons_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `addon_role_permissions` (
  `addon_code` varchar(64) NOT NULL,
  `permission_code` varchar(64) NOT NULL,
  `role_id` int NOT NULL,
  `is_allowed` tinyint(1) NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`addon_code`, `permission_code`, `role_id`),
  KEY `idx_addon_role_permission_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
