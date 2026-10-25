-- Lo que el addon PIDE vive en su module.json; lo que la empresa CONCEDE vive
-- aquí. Un addon no puede activarse mientras alguna capability declarada siga
-- sin decisión, y lo que no está aprobado no se sirve en caliente: el grant que
-- recibe el proceso se construye con las capabilities efectivas, no con el
-- manifiesto.
--
-- `kind` es resource | command | event | sale_guard. El guard no tiene código
-- propio (es uno por addon), así que usa `sale_guard` en ambas columnas: una
-- parte de clave vacía sería un pie para filas duplicadas.
CREATE TABLE IF NOT EXISTS `addon_capability_approvals` (
  `addon_code` varchar(64) NOT NULL,
  `kind` varchar(16) NOT NULL,
  `code` varchar(64) NOT NULL,
  `is_approved` tinyint(1) NOT NULL DEFAULT 0,
  `decided_by` varchar(64) NOT NULL DEFAULT '',
  `decided_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`addon_code`, `kind`, `code`),
  KEY `idx_addon_capability_approved` (`addon_code`, `is_approved`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
