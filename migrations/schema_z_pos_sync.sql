-- Migración: sincronización POS offline-first (ventas y turnos).
--
-- POR QUÉ ESTAS TABLAS.
--
-- El POS (binario aparte, en la caja) trabaja sin red contra su SQLite. Cuando
-- vuelve la conexión sube su cola: ventas y turnos. Las ventas ya tenían
-- idempotencia por (terminal_id, offline_id) en pos_sale_idempotency, pero esa
-- tabla la usa el checkout en línea. La cola offline necesita su propio libro
-- para no depender de que exista un número fiscal (que sin impresora no existe)
-- y para poder registrar el resultado de cada venta aunque el control_no se
-- recorte.
--
-- Los turnos no tenían contraparte portable: cash_session es una tabla del
-- módulo de caja con columnas fijas y sin moneda doble completa. pos_shifts
-- guarda el turno TAL COMO lo cerró la caja (VES y USD, supervisor, diferencias)
-- y cash_session se actualiza como espejo para que el resto del ERP lo vea.
--
-- RRHH: cada apertura/cierre se refleja en pos_turn_events (open_turn /
-- close_turn), que ya existía y es la fuente para horas trabajadas.
--
-- Idempotente: CREATE IF NOT EXISTS + ADD COLUMN IF NOT EXISTS (MariaDB).

-- ─────────────────────────────────────────────────────────────────────────────
-- Libro de ventas sincronizadas desde la cola offline.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `pos_sale_sync` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `sale_local_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `invoice_id` int DEFAULT NULL,
  `invoice_no` bigint DEFAULT NULL,
  `control_no` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `fiscal_number` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'synced',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pos_sale_sync` (`terminal_id`,`sale_local_id`),
  KEY `idx_pos_sale_sync_invoice` (`invoice_id`),
  KEY `idx_pos_sale_sync_control` (`control_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────────────────────────────────────
-- Turnos de caja tal como los registra el POS (VES + USD, supervisor, cierre).
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `pos_shifts` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `local_turn_id` bigint NOT NULL,
  `branch_id` int NOT NULL DEFAULT '0',
  `user_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `user_name` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supervisor_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `supervisor_name` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opened_at` datetime DEFAULT NULL,
  `closed_at` datetime DEFAULT NULL,
  `opening_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `opening_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `declared_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `declared_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `system_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `system_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `difference_ves` decimal(18,2) NOT NULL DEFAULT '0.00',
  `difference_usd` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_sales` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_refunds` decimal(18,2) NOT NULL DEFAULT '0.00',
  `sales_count` int NOT NULL DEFAULT '0',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'open',
  `closure_type` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `till_purpose` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `notes` text COLLATE utf8mb4_unicode_ci,
  `remote_session_id` int DEFAULT NULL,
  `synced_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pos_shifts_turn` (`terminal_id`,`local_turn_id`),
  KEY `idx_pos_shifts_status` (`terminal_id`,`status`),
  KEY `idx_pos_shifts_opened` (`opened_at`),
  KEY `idx_pos_shifts_user` (`user_id`),
  KEY `idx_pos_shifts_session` (`remote_session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────────────────────────────────────
-- Idempotencia de la cola de turnos: una apertura y un cierre por turno local.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `pos_shift_sync` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `terminal_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `local_turn_id` bigint NOT NULL,
  `kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `remote_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'ok',
  `message` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pos_shift_sync` (`terminal_id`,`local_turn_id`,`kind`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────────────────────────────────────
-- Columnas que posAPIShiftClose ya escribe pero que el esquema base no tenía.
-- Sin ellas, el cierre por token fallaba con "Unknown column".
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE `cash_session`
  ADD COLUMN IF NOT EXISTS `closure_type` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS `supervisor_id` int DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS `approved_at` datetime DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS `integrity_hash` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL;
