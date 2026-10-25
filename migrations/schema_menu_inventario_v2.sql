-- ---------------------------------------------------------------------------
-- MENÚ INVENTARIO: reorganización a Gestion Inventario
-- Estructura deseada:
--   Gestion Inventario (raíz, id=5000)
--   ├── Stock (id=5001, enlace a /reports/stock)
--   └── Ajuste de Inventario (grupo, id=5002)
--       ├── Nuevo Ajuste (id=5003, /adjustment/new)
--       └── Historial (id=5004, /adjustment/list)
-- Además: enlace "Gestor de Menús" en Configuración (id=281)
-- ---------------------------------------------------------------------------

-- 1. Desactivar entradas antiguas de inventario (inactivas o duplicadas)
UPDATE `menu` SET `status` = 0
WHERE `id` IN (7, 14, 190, 191, 300, 304);

-- 2. Crear "Gestion Inventario" como raíz
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5000, NULL, 'Gestion Inventario', NULL, 'metismenu-icon fa fa-cubes', NULL, NULL, 70, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `icon` = VALUES(`icon`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`), `parent_id` = VALUES(`parent_id`);

-- 3. Stock como hijo directo
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5001, 5000, 'Stock', '/reports/stock', NULL, 'stock_report', '/reports/stock', 10, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

-- 4. Ajuste de Inventario como grupo
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5002, 5000, 'Ajuste de Inventario', NULL, NULL, 'adjustment', NULL, 20, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

-- 5. Nuevo Ajuste
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5003, 5002, 'Nuevo Ajuste', '/adjustment/new', NULL, NULL, NULL, 10, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

-- 6. Historial
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (5004, 5002, 'Historial', '/adjustment/list', NULL, NULL, NULL, 20, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `parent_id` = VALUES(`parent_id`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`);

-- 7. Gestor de Menús en Configuración (bajo id=21)
INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
VALUES (281, 21, 'Gestor de Menús', '/config/menu', 'fa fa-bars', 'setting', '/config/menu', 115, 1, 'core', 1)
ON DUPLICATE KEY UPDATE `label` = VALUES(`label`), `url` = VALUES(`url`), `icon` = VALUES(`icon`), `sort_order` = VALUES(`sort_order`), `status` = VALUES(`status`), `parent_id` = VALUES(`parent_id`);

-- 8. Desactivar duplicados de módulos antiguos (100, 202, 300, 400, 500, 600, 700, 900, 1000)
UPDATE `menu` SET `status` = 0 WHERE `id` IN (100, 202, 300, 400, 500, 600, 700, 900, 1000);

-- 9. Eliminar duplicados de inventario creados por migraciones anteriores
DELETE FROM `menu` WHERE `id` BETWEEN 700 AND 704;
DELETE FROM `menu` WHERE `id` BETWEEN 2000 AND 2004;

-- 10. Mover Notas de Crédito a raíz (si está como hijo)
UPDATE `menu` SET `parent_id` = NULL, `sort_order` = 100 WHERE `id` = 10 AND `parent_id` IS NOT NULL;
