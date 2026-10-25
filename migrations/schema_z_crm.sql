-- ============================================================================
-- CRM: la relación con el cliente antes de que sea una venta.
--
-- Por qué nace como esquema propio y no sobre `customer_information`: un cliente
-- es un_patch, es decir, alguien a quien YA le facturamos. El CRM trata con el
-- contacto y la oportunidad que existen ANTES de esa primera venta, y con la
-- actividad que se tiene entre una y otra llamada. Meterlo todo en la ficha del
-- cliente obligaría a inventar campos vacíos y a no poder registrar un contacto
-- que todavía no compra nada.
--
-- Ambos se enlazan, sí: `crm_contact` puede apuntar a un cliente existente con
-- `customer_id`, que se rellena en el momento en que la oportunidad se convierte.
-- La trazabilidad (qué contacto originó qué factura) es la razón de ser del CRM.
-- ============================================================================

-- Contacto: la persona con la que habla la empresa.
CREATE TABLE IF NOT EXISTS `crm_contact` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `full_name` varchar(120) NOT NULL,
  `email` varchar(120) NOT NULL DEFAULT '',
  `phone` varchar(30) NOT NULL DEFAULT '',
  `company_name` varchar(120) NOT NULL DEFAULT '',
  `position_title` varchar(80) NOT NULL DEFAULT '',
  `customer_id` integer DEFAULT NULL COMMENT 'se llena al convertir la oportunidad',
  `supplier_id` integer DEFAULT NULL,
  `source` varchar(30) NOT NULL DEFAULT 'otro' COMMENT 'referido | web | feria | red | cold_call | otro',
  `owner_id` integer NOT NULL DEFAULT 0 COMMENT 'usuario responsable del seguimiento',
  `status` integer NOT NULL DEFAULT 1 COMMENT '1=activo, 0=descartado',
  `notes` text,
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  `updated_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
);

CREATE INDEX IF NOT EXISTS `idx_crm_contact_customer` ON `crm_contact`(`customer_id`);
CREATE INDEX IF NOT EXISTS `idx_crm_contact_owner` ON `crm_contact`(`owner_id`);

-- Embudo. Una oportunidad con su etapa y su valor esperado.
--
-- Las etapas son DATOS (crm_stage), no un enum en código: cada empresa vende de
-- forma distinta y cambiar el embudo no debería exigir recompilar. Se ordenan
-- por `position` y la última con `is_won=1` es la que cierra la venta.
CREATE TABLE IF NOT EXISTS `crm_stage` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `name` varchar(60) NOT NULL,
  `position` integer NOT NULL DEFAULT 0,
  `probability` integer NOT NULL DEFAULT 0 COMMENT 'probabilidad de cierre en % (0-100)',
  `is_won` integer NOT NULL DEFAULT 0 COMMENT '1 = etapa cerrada ganada',
  `is_lost` integer NOT NULL DEFAULT 0,
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

CREATE TABLE IF NOT EXISTS `crm_opportunity` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `title` varchar(160) NOT NULL,
  `contact_id` integer NOT NULL,
  `stage_id` integer NOT NULL DEFAULT 0,
  `amount` decimal(16,2) NOT NULL DEFAULT 0,
  `expected_close` date DEFAULT NULL,
  `owner_id` integer NOT NULL DEFAULT 0,
  `description` text,
  `lost_reason` varchar(255) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  `updated_at` VARCHAR(32) NOT NULL DEFAULT '',
  `closed_at` VARCHAR(32) NOT NULL DEFAULT '',
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

CREATE INDEX IF NOT EXISTS `idx_crm_opp_stage` ON `crm_opportunity`(`stage_id`);
CREATE INDEX IF NOT EXISTS `idx_crm_opp_contact` ON `crm_opportunity`(`contact_id`);

-- Actividad: llamadas, correos, reuniones. Es lo que convierte al CRM en un
-- historial y no en una lista de nombres.
CREATE TABLE IF NOT EXISTS `crm_activity` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `contact_id` integer NOT NULL DEFAULT 0,
  `opportunity_id` integer NOT NULL DEFAULT 0,
  `activity_type` varchar(30) NOT NULL DEFAULT 'llamada' COMMENT 'llamada | email | reunion | visita | tarea | nota',
  `subject` varchar(160) NOT NULL DEFAULT '',
  `body` text,
  `due_date` date DEFAULT NULL,
  `done` integer NOT NULL DEFAULT 0,
  `created_by` integer NOT NULL DEFAULT 0,
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  `completed_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
);

CREATE INDEX IF NOT EXISTS `idx_crm_activity_due` ON `crm_activity`(`done`, `due_date`);

-- Un contacto puede tener varias oportunidades, pero sólo una puede estar
-- abierta a la vez por el mismo motivo comercial. Este índice parcial no existe
-- en SQLite; se garantiza en la capa de servicio, que es donde se puede dar un
-- mensaje útil en vez de un error de restricción.