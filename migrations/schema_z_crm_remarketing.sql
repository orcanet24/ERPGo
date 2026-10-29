-- ---------------------------------------------------------------------------
-- CRM Remarketing — Campañas de re-contacto segmentadas.
--
-- Extensión natural del CRM: usa contactos, clientes, oportunidades y
-- canales de aviso (notify_channel) ya existentes. No duplica datos;
-- los referencia por ID.
--
-- Tres pilares:
--   1. Segmento   — regla que selecciona contactos/clientes (ej: "no compran
--                   hace 60 días", "oportunidad perdida", "cotización enviada
--                   sin cerrar").
--   2. Campaña    — envío de un mensaje a los contactos de un segmento por
--                   un canal configurado (email, SMS, WhatsApp).
--   3. Plantilla  — mensaje reutilizable con variables (%nombre%, %empresa%).
-- ---------------------------------------------------------------------------

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE IF NOT EXISTS `crm_remarketing_segment` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) NOT NULL COMMENT 'nombre descriptivo del segmento',
  `description` varchar(255) NOT NULL DEFAULT '',
  `rule_type` varchar(40) NOT NULL DEFAULT 'no_purchase_60d'
    COMMENT 'no_purchase_Nd | opportunity_lost | quote_sent_open | no_activity_Nd | customer_all | contact_all',
  `rule_days` int NOT NULL DEFAULT '0' COMMENT 'días para reglas basadas en tiempo',
  `status` int NOT NULL DEFAULT '1' COMMENT '1=activo, 0=pausado',
  `created_at` varchar(32) NOT NULL DEFAULT '',
  `updated_at` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_seg_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crm_remarketing_template` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) NOT NULL,
  `subject` varchar(200) NOT NULL DEFAULT '' COMMENT 'asunto (email/SMS ignorado)',
  `body` text NOT NULL COMMENT 'cuerpo con variables %nombre% %empresa% %telefono%',
  `status` int NOT NULL DEFAULT '1',
  `created_at` varchar(32) NOT NULL DEFAULT '',
  `updated_at` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_tpl_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crm_remarketing_campaign` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(160) NOT NULL,
  `segment_id` int NOT NULL DEFAULT '0',
  `template_id` int NOT NULL DEFAULT '0',
  `channel_id` int NOT NULL DEFAULT '0' COMMENT 'notify_channel.id',
  `status` int NOT NULL DEFAULT '0' COMMENT 'borrador=0, enviando=1, enviada=2, cancelada=3',
  `total_recipients` int NOT NULL DEFAULT '0',
  `sent_count` int NOT NULL DEFAULT '0',
  `error_count` int NOT NULL DEFAULT '0',
  `scheduled_at` varchar(32) NOT NULL DEFAULT '' COMMENT 'vacio=inmediato',
  `created_by` int NOT NULL DEFAULT '0',
  `created_at` varchar(32) NOT NULL DEFAULT '',
  `updated_at` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_camp_status` (`status`),
  KEY `idx_crm_remarketing_camp_segment` (`segment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `crm_remarketing_recipient` (
  `id` int NOT NULL AUTO_INCREMENT,
  `campaign_id` int NOT NULL,
  `contact_id` int NOT NULL DEFAULT '0',
  `customer_id` int NOT NULL DEFAULT '0',
  `recipient` varchar(160) NOT NULL COMMENT 'destino efectivo (email o teléfono)',
  `status` varchar(20) NOT NULL DEFAULT 'pendiente' COMMENT 'pendiente|enviado|error',
  `error_msg` varchar(255) NOT NULL DEFAULT '',
  `idem_key` varchar(80) NOT NULL DEFAULT '' COMMENT 'clave de idempotencia',
  `sent_at` varchar(32) NOT NULL DEFAULT '',
  `created_at` varchar(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  KEY `idx_crm_remarketing_rec_camp` (`campaign_id`),
  KEY `idx_crm_remarketing_rec_status` (`status`),
  KEY `idx_crm_remarketing_rec_contact` (`contact_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS=1;
