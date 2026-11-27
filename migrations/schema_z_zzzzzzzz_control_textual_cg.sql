-- ---------------------------------------------------------------------------
-- NÚMERO DE CONTROL TEXTUAL (2026-11-27).
--
-- POR QUÉ EXISTE. `digital_invoice_log.nro_control` es INT porque la primera
-- imprenta integrada (Unidigital) devuelve el control como entero. CG Imprenta
-- Digital lo devuelve como texto CON SU SERIE: "00-00000056". La serie no cabe
-- en un INT y perderla deja un documento que no se puede conciliar con el que
-- el cliente tiene impreso.
--
-- La columna nueva guarda el control tal como lo entrega la imprenta. La
-- columna entera se conserva para Unidigital (y para no romper el histórico).
--
-- Se agrega en una migración y NO se cambia el tipo de nro_control: una base
-- con facturas ya emitidas no puede permitirse un ALTER de tipo sobre la
-- columna que identifica documentos fiscales.
-- ---------------------------------------------------------------------------

ALTER TABLE `digital_invoice_log`
  ADD COLUMN IF NOT EXISTS `nro_control_texto` VARCHAR(32) NULL DEFAULT NULL
  COMMENT 'Número de control completo con su serie (CG: 00-00000056)';

-- Índice de búsqueda: la imprenta consulta por número de control y la relación
-- de contingencia lo muestra. No es UNIQUE a propósito: el control es único por
-- negocio e imprenta, no en toda la base de control, y varios documentos sin
-- control textual (los de Unidigital, los pendientes) comparten el NULL.
CREATE INDEX IF NOT EXISTS `idx_digital_control_texto` ON `digital_invoice_log` (`nro_control_texto`);
