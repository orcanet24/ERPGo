-- Entrada de menú: Facturación Digital (SENIAT) bajo Configuración.
--
-- La pantalla /settings/digital-invoice existía (página, save, activate,
-- ping) pero el menú nunca la referenció: quedaba huérfana y el admin no
-- podía llegar. Unidigital es el primero del catálogo; entradas nuevas de
-- imprentas digitales aparecen solas en la pantalla cuando se agregan a
-- diProveedorCatalogo en interfaces/web/digital_invoice_settings.go,
-- así que NO hace falta tocar el menú por cada imprenta nueva.
--
-- Insert idempotente y con id fijo (275): si ya existe, los campos se
-- alinean, no se duplica. NUNCA borra datos.

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
VALUES (275, 21, 'Facturación Digital', '/settings/digital-invoice', NULL, 'setting', '/settings/digital-invoice', 25, 1)
ON DUPLICATE KEY UPDATE
  `label` = VALUES(`label`),
  `parent_id` = VALUES(`parent_id`),
  `url` = VALUES(`url`),
  `icon` = VALUES(`icon`),
  `perm` = VALUES(`perm`),
  `active_prefix` = VALUES(`active_prefix`),
  `sort_order` = VALUES(`sort_order`),
  `status` = VALUES(`status`);
