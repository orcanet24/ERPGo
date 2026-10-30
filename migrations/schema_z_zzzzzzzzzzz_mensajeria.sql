-- ---------------------------------------------------------------------------
-- Módulo de Mensajería (WhatsApp + Telegram + correo/SMS/webhook).
--
-- NACE APAGADO. La fila de `modules` entra con is_active = 0 y no hay ninguna
-- ruta que lo encienda sin pasar por /settings/mensajeria (master) y aceptar el
-- texto de riesgo. Ese es el requisito del dueño del sistema: que el módulo
-- pueda activarse o desactivarse por el super administrador, y que mientras
-- esté apagado no exista para nadie.
--
-- DÓNDE VIVE CADA COSA. Las tablas de negocio (ajustes, proveedores, mensajes,
-- contactos, opt-out, accesos) viven en la base de la EMPRESA. Las de
-- infraestructura (servicio global, dispositivos, cola, retos OTP) viven en la
-- base de CONTROL: el webhook tiene que resolver «dispositivo → empresa» sin
-- sesión, y un número no puede pertenecer a dos empresas. Como el esquema se
-- define UNA sola vez en migrations/ y se aplica a las dos clases de base, aquí
-- están todas; la separación se respeta en el código (controlStore vs dbFor).
--
-- IDS DE MENÚ: 282-284. El máximo usado hasta hoy es 281 (Gestor de Menús).
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `msg_setting` (
  `id` int NOT NULL AUTO_INCREMENT,
  `enabled` tinyint(1) NOT NULL DEFAULT '0',
  `risk_accepted_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `risk_accepted_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `default_country` varchar(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'VE',
  `chat_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `otp_enabled` tinyint(1) NOT NULL DEFAULT '0',
  `updated_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_provider` (
  `id` int NOT NULL AUTO_INCREMENT,
  `provider` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '0',
  `config_json` text COLLATE utf8mb4_unicode_ci,
  `updated_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_provider` (`provider`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `area` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'general',
  `name` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `language` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'es',
  `body` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `variables_json` text COLLATE utf8mb4_unicode_ci,
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `created_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_template_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_message` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `device_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `chat_jid` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `direction` varchar(3) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'out',
  `message_id` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `from_msisdn` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `to_msisdn` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `body` text COLLATE utf8mb4_unicode_ci,
  `media_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `media_path` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'aceptado',
  `is_from_me` tinyint(1) NOT NULL DEFAULT '0',
  `sent_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `received_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `raw_json` text COLLATE utf8mb4_unicode_ci,
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_message` (`device_id`, `message_id`),
  KEY `idx_msg_message_chat` (`device_id`, `chat_jid`, `sent_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_contact_channel` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `msisdn` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `provider` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `address` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `verified_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `source` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'manual',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'verificado',
  `preference` int NOT NULL DEFAULT '0',
  `notes` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_contact_channel` (`msisdn`, `provider`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_optout` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `msisdn` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `source` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_optout` (`msisdn`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_user_access` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `device_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `can_read` tinyint(1) NOT NULL DEFAULT '1',
  `can_send` tinyint(1) NOT NULL DEFAULT '0',
  `created_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_user_access` (`user_id`, `device_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Infraestructura (se lee de la base de control).
CREATE TABLE IF NOT EXISTS `msg_global` (
  `id` int NOT NULL AUTO_INCREMENT,
  `enabled` tinyint(1) NOT NULL DEFAULT '0',
  `daemon_url` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `daemon_user` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `daemon_password_enc` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `webhook_secret_enc` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `default_country` varchar(2) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'VE',
  `risk_text_version` int NOT NULL DEFAULT '1',
  `updated_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_device` (
  `id` int NOT NULL AUTO_INCREMENT,
  `device_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `tenant` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `area` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'general',
  `label` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'whatsapp_gowa',
  `msisdn` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `webhook_secret_enc` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `state` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'nuevo',
  `jid` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `daily_cap` int NOT NULL DEFAULT '120',
  `hourly_cap` int NOT NULL DEFAULT '25',
  `min_delay_s` int NOT NULL DEFAULT '45',
  `max_delay_s` int NOT NULL DEFAULT '180',
  `warmup_day` int NOT NULL DEFAULT '0',
  `next_send_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `paused_until` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `last_seen_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_device` (`device_id`),
  UNIQUE KEY `uq_msg_device_msisdn` (`msisdn`),
  KEY `idx_msg_device_tenant` (`tenant`, `area`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_outbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `tenant` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `device_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `priority` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'transaccional',
  `kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'aviso',
  `to_msisdn` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `body` text COLLATE utf8mb4_unicode_ci,
  `media_path` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `template_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ref_type` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ref_id` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `idem_key` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'en_cola',
  `attempts` int NOT NULL DEFAULT '0',
  `send_after` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `claimed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider_ref` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_msg_outbox_idem` (`idem_key`),
  KEY `idx_msg_outbox_cola` (`status`, `send_after`),
  KEY `idx_msg_outbox_tenant` (`tenant`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `msg_otp_challenge` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `username` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `phone_masked` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `code_hash` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `salt` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `expires_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `attempts` int NOT NULL DEFAULT '0',
  `max_attempts` int NOT NULL DEFAULT '5',
  `consumed_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `resend_count` int NOT NULL DEFAULT '0',
  `preauth_token_hash` varchar(191) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ip` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_agent` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_msg_otp` (`username`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- Taxonomía: módulo 25 y sus permisos.
--
-- El id NO se fija a mano: si una instalación ya tiene un `module` con
-- directory 'mensajeria' (por ejemplo, una migración manual anterior), se
-- reutiliza. Fijar el 25 chocaría con la fila heredada y `INSERT IGNORE` la
-- saltaría en silencio, dejando los permisos sin dueño.
-- ---------------------------------------------------------------------------
INSERT INTO `module` (`name`, `description`, `directory`, `status`)
SELECT 'Mensajería', 'WhatsApp, Telegram, correo y SMS', 'mensajeria', 1
FROM (SELECT COUNT(*) AS n FROM `module` WHERE `directory` = 'mensajeria') AS existente
WHERE existente.n = 0;

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM `module` WHERE `directory` = 'mensajeria' LIMIT 1), 'mensajeria_chat', 'mensajeria_chat', 1
FROM (SELECT COUNT(*) AS n FROM `sub_module` WHERE `directory` = 'mensajeria_chat') AS existente
WHERE existente.n = 0;

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM `module` WHERE `directory` = 'mensajeria' LIMIT 1), 'mensajeria_send', 'mensajeria_send', 1
FROM (SELECT COUNT(*) AS n FROM `sub_module` WHERE `directory` = 'mensajeria_send') AS existente
WHERE existente.n = 0;

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT (SELECT `id` FROM `module` WHERE `directory` = 'mensajeria' LIMIT 1), 'mensajeria_admin', 'mensajeria_admin', 1
FROM (SELECT COUNT(*) AS n FROM `sub_module` WHERE `directory` = 'mensajeria_admin') AS existente
WHERE existente.n = 0;

-- El módulo, apagado de fábrica. `is_core = 0` para que /module/list lo pueda
-- apagar y encender; la llave real de la instalación es msg_global.enabled.
INSERT INTO `modules` (`id`, `name`, `version`, `description`, `is_core`, `is_active`, `path`, `author`, `license_type`)
SELECT 'mensajeria', 'Mensajería', '1.0', 'WhatsApp, Telegram, correo y SMS', 0, 0, '/mensajeria', 'ERPGo', 'MIT'
FROM (SELECT COUNT(*) AS n FROM `modules` WHERE `id` = 'mensajeria') AS existente
WHERE existente.n = 0;

-- Menú. El grupo y la bandeja pertenecen al módulo (desaparecen si se apaga);
-- la entrada de CONFIGURACIÓN pertenece a 'core' a propósito: el master tiene
-- que poder llegar a /settings/mensajeria para volver a encenderlo.
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
SELECT 282, NULL, 'Mensajería', NULL, 'fa fa-comments', 'mensajeria_chat|mensajeria_send', NULL, 215, 1, 'mensajeria', 0
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/mensajeria' OR `id` = 282) AS existente
WHERE existente.n = 0;

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
SELECT 283, 282, 'Bandeja', '/mensajeria', NULL, 'mensajeria_chat', '/mensajeria', 10, 1, 'mensajeria', 0
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/mensajeria') AS existente
WHERE existente.n = 0;

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
SELECT 284, 21, 'Mensajería', '/settings/mensajeria', NULL, 'setting', '/settings/mensajeria', 67, 1, 'mensajeria', 0
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/settings/mensajeria') AS existente
WHERE existente.n = 0;

-- Proveedores: todos apagados. Se encienden uno por uno desde la pantalla, y
-- solo después de aceptar el riesgo que corresponda.
INSERT INTO `msg_provider` (`provider`, `enabled`, `config_json`, `updated_by`, `updated_at`)
SELECT 'whatsapp_gowa', 0, '', '', ''
FROM (SELECT COUNT(*) AS n FROM `msg_provider` WHERE `provider` = 'whatsapp_gowa') AS existente
WHERE existente.n = 0;

INSERT INTO `msg_provider` (`provider`, `enabled`, `config_json`, `updated_by`, `updated_at`)
SELECT 'telegram_bot', 0, '', '', ''
FROM (SELECT COUNT(*) AS n FROM `msg_provider` WHERE `provider` = 'telegram_bot') AS existente
WHERE existente.n = 0;

INSERT INTO `msg_provider` (`provider`, `enabled`, `config_json`, `updated_by`, `updated_at`)
SELECT 'telegram_user', 0, '', '', ''
FROM (SELECT COUNT(*) AS n FROM `msg_provider` WHERE `provider` = 'telegram_user') AS existente
WHERE existente.n = 0;

INSERT INTO `msg_provider` (`provider`, `enabled`, `config_json`, `updated_by`, `updated_at`)
SELECT 'telegram_gateway', 0, '', '', ''
FROM (SELECT COUNT(*) AS n FROM `msg_provider` WHERE `provider` = 'telegram_gateway') AS existente
WHERE existente.n = 0;
