-- ============================================================================
-- Cadena de auditoría por BLOQUES DE PERÍODO (implementa doc/bitacora-sqlite-mysql.md §7)
--
-- Problema que resuelve: la cadena era global, así que auditLogVerify recorría
-- TODA la tabla audit_log. Con millones de registros (crece con cada venta)
-- la verificación colapsaba y no era viable en el escenario de carga (25
-- terminales).
--
-- Solución: la cadena lógica sigue siendo continua (cada bloque enlaza con el
-- hash final del bloque anterior), pero se SEGMENTA en bloques mensuales. La
-- verdad criptográfica sigue living en audit_log; esta tabla es el ÍNDICE y el
-- SELLO de cada bloque, para poder auditar "el bloque de octubre" sin recorrer
-- el histórico completo.
--
-- La tabla es append-only por período: un período se sella UNA vez y su sello
-- nunca se sobrescribe (recalcularlo y obtener un valor distinto es la prueba
-- de manipulación, y auditLogVerify lo reporta).
--
-- Nota de orden: las migraciones se aplican en orden alfabético del nombre de
-- archivo. Este archivo se llama schema_z_audit_blocks.sql a propósito para
-- quedar DESPUÉS de schema_extra.sql, que es donde se crea audit_log.
-- No se altera audit_log: no se toca ni una columna del esquema de seguridad.
-- ============================================================================

CREATE TABLE IF NOT EXISTS `audit_chain_blocks` (
  `period` varchar(7) NOT NULL COMMENT 'YYYY-MM',
  `prev_block_hash` varchar(64) NOT NULL DEFAULT '' COMMENT 'block_hash del bloque anterior',
  `start_hash` varchar(64) NOT NULL DEFAULT '' COMMENT 'prev_hash del primer registro del bloque',
  `end_hash` varchar(64) NOT NULL DEFAULT '' COMMENT 'record_hash del último registro del bloque',
  `first_id` int DEFAULT NULL COMMENT 'primer id de audit_log del bloque',
  `last_id` int DEFAULT NULL COMMENT 'último id de audit_log del bloque',
  `record_count` int NOT NULL DEFAULT 0,
  `block_hash` varchar(64) NOT NULL DEFAULT '' COMMENT 'sello del bloque',
  `sealed_at` datetime DEFAULT NULL,
  PRIMARY KEY (`period`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;