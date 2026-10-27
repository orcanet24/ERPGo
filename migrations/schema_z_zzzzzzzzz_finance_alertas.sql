-- ============================================================================
-- ERPGo — Alertas financieras (CxC / CxP) y canales de aviso salientes.
--
-- QUÉ RESUELVE.
--
-- Cuentas por cobrar y cuentas por pagar sabían decir cuánto se debe, y nada
-- más: para saber QUIÉN se pasó de la raya había que mirar la lista y sacar
-- cuentas a mano, cada vez. Y aunque el humano lo viera, avisar al cliente de su
-- mora o mandarle al proveedor su retención de IVA exigía salirse del sistema
-- (correo personal, teléfono, WhatsApp), con lo que ese aviso no quedaba
-- registrado en ninguna parte.
--
-- Aquí quedan las tres piezas:
--
--  1. finance_alert
--     La alerta ES un registro, no un cálculo de pantalla. Se recalcula al abrir
--     el tablero y también desde la herramienta `finance.scan_alerts` de los
--     agentes, así que el humano y la IA miran LA MISMA lista. Su fingerprint
--     (hash de ámbito + motivo + contraparte + referencia) con índice único es
--     el candado: el escaneo corre las veces que haga falta y no acumula
--     duplicados. Cuando la deuda se paga, el escaneo CIERRA la alerta sola.
--
--  2. notify_channel
--     Los canales de salida —correo (SMTP), SMS (pasarela HTTP) y webhook— tal
--     como los configura el administrador. Las credenciales viven aquí, en la
--     base de la empresa, y NUNCA viajan al navegador: la pantalla recibe solo
--     id, nombre y tipo.
--
--     `ai_allowed` NACE EN 0 A PROPÓSITO. Un canal existe para que lo use el
--     humano en cuanto se configura; que además lo pueda usar un AGENTE es una
--     decisión del administrador, canal por canal, porque es la única salida de
--     datos del sistema hacia afuera.
--
--     `cooldown_days` es la defensa contra el propio celo: un agente que corre
--     cada cinco minutos no puede mandarle diez correos al mismo cliente por la
--     misma deuda. 0 = sin límite (decisión explícita del administrador).
--
--  3. finance_message
--     Todo aviso que sale queda escrito: canal, destinatario, asunto, cuerpo,
--     quién lo mandó, si salió o falló y por qué. Es lo que permite responder
--     «¿ya se le avisó a este cliente?» sin preguntarle a la persona que lo
--     mandó. `idem_key` único es el candado de idempotencia: la misma acción con
--     los mismos datos no sale dos veces, igual que en ai_tool_execution.
--
-- Idempotente: el runner descarta «duplicate column name» / «duplicate key name»
-- y quita el IF NOT EXISTS de MariaDB antes de hablar con MySQL.
-- ============================================================================

CREATE TABLE IF NOT EXISTS `finance_alert` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `scope` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'cxc' COMMENT 'cxc (por cobrar) | cxp (por pagar)',
  `kind` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'motivo de la alerta',
  `severity` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'aviso' COMMENT 'critico | aviso | info',
  `party_type` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'customer' COMMENT 'customer | supplier',
  `party_id` bigint NOT NULL DEFAULT 0,
  `party_name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ref_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'invoice | purchase | retention | cartera',
  `ref_id` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `days` int NOT NULL DEFAULT 0 COMMENT 'antiguedad que la disparo',
  `metric_json` text COLLATE utf8mb4_unicode_ci COMMENT 'los numeros que la sustentan + el motivo redactado',
  `fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'candado contra alertas repetidas',
  `status` varchar(15) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'abierta' COMMENT 'abierta | cerrada | resuelta | descartada',
  `notified_at` datetime DEFAULT NULL,
  `notify_count` int NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_finance_alert` (`fingerprint`),
  KEY `idx_fa_scope_status` (`scope`,`status`),
  KEY `idx_fa_party` (`party_type`,`party_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `notify_channel` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `kind` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'email' COMMENT 'email | sms | webhook',
  `name` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_active` tinyint NOT NULL DEFAULT '1',
  `ai_allowed` tinyint NOT NULL DEFAULT '0' COMMENT '0 = solo humanos; el administrador lo abre canal por canal',
  `cooldown_days` int NOT NULL DEFAULT '3' COMMENT 'dias minimos entre dos avisos por la misma alerta (0 = sin limite)',
  `host` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `port` int NOT NULL DEFAULT '587',
  `username` varchar(160) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `password` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `from_address` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `from_name` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `security` varchar(12) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'starttls' COMMENT 'tls | starttls | none',
  `url` varchar(400) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `http_method` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'POST',
  `auth_header` varchar(60) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'ej. Authorization',
  `auth_token` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `sign_secret` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'HMAC-SHA256 del cuerpo (webhook)',
  `notes` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_notify_channel_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `finance_message` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `channel_id` bigint NOT NULL DEFAULT 0,
  `channel` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `channel_kind` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `recipient` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `subject` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `body` text COLLATE utf8mb4_unicode_ci,
  `ref_type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'alerta | retencion_iva | aviso_cobro | prueba | evento',
  `ref_id` varchar(60) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(15) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'enviando' COMMENT 'enviando | enviado | error',
  `error` text COLLATE utf8mb4_unicode_ci,
  `provider_ref` varchar(160) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `idem_key` char(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'la misma accion con los mismos datos no sale dos veces',
  `created_by` varchar(60) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `sent_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_finance_message_idem` (`idem_key`),
  KEY `idx_fm_ref` (`ref_type`,`ref_id`),
  KEY `idx_fm_status` (`status`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `finance_scan_state` (
  `scope` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `last_run` datetime NOT NULL,
  `last_result` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`scope`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- El menú, con INSERT condicional por URL: si una base ya trae la entrada (por
-- haberla creado a mano) no se toca y no se duplica.
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
SELECT 275, 21, 'Canales de aviso', '/settings/notify', NULL, 'setting', '/settings/notify', 65, 1
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/settings/notify') AS existente
WHERE existente.n = 0;

-- El historial de avisos NO pide el permiso de configuración: tiene que verlo
-- quien manda los avisos (cobranza y cuentas por pagar), que no es
-- necesariamente quien configura las credenciales del canal. La pantalla de
-- canales sí queda detrás de /settings, que exige master.
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
SELECT 276, 21, 'Mensajes enviados', '/finance/mensajes', NULL, 'customer_ledger|supplier_payment', '/finance/mensajes', 66, 1
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/finance/mensajes') AS existente
WHERE existente.n = 0;
