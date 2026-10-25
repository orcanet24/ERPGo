-- ============================================================================
-- Cierre de períodos contables.
--
-- Cerrar un mes es la afirmación de que ese mes ya no se toca. Este archivo
-- guarda esa afirmación con su responsable y su fecha, y — lo más importante —
-- conserva el rastro de toda reapertura.
--
-- Decisión: NO se borra un cierre. Un período reabierto queda con estado
-- 'reopened' en vez de desaparecer, porque la pregunta "¿alguien volvió a tocar
-- un mes que ya estaba firmado?" siempre se hace tarde (en una auditoría, en una
-- inspección, en una disputa con el SENIAT) y tiene que poder responderse.
-- ============================================================================

CREATE TABLE IF NOT EXISTS `acc_period_close` (
  `period` varchar(6) NOT NULL COMMENT 'AAAAMM',
  `status` varchar(20) NOT NULL DEFAULT 'open' COMMENT 'closed | reopened',
  `closed_at` VARCHAR(32) NOT NULL DEFAULT '',
  `closed_by` varchar(60) NOT NULL DEFAULT '',
  `reopened_at` VARCHAR(32) NOT NULL DEFAULT '',
  `reopened_by` varchar(60) NOT NULL DEFAULT '',
  `reopen_reason` VARCHAR(500) NOT NULL DEFAULT '' COMMENT 'motivo obligatorio de cada reapertura',
  PRIMARY KEY (`period`)
);

-- El índice por fecha ayuda a listar los períodos recientes sin recorrer toda la
-- tabla; en una base con años de histórico la lista del panel es la consulta más
-- frecuente sobre esta tabla.
CREATE INDEX IF NOT EXISTS `idx_acc_period_status` ON `acc_period_close`(`status`, `period`);