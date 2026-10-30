-- ---------------------------------------------------------------------------
-- «Módulos» en el menú de Configuración.
--
-- POR QUÉ FALTABA Y POR QUÉ IMPORTA. La pantalla /module/list existía desde
-- antes, pero no tenía entrada de menú: el super administrador no tenía forma de
-- llegar a ella salvo escribiendo la URL. Y sin esa pantalla no se puede
-- encender ningún módulo no-core, incluido mensajería, porque su fila nace con
-- is_active = 0.
--
-- `perm = '__master'` no es un permiso de negocio: es la marca que el mapa de
-- permisos pone solo al nivel master (ver cachas.go). Así el administrador de
-- empresa no ve una entrada que, al pulsarla, le daría 403.
--
-- El id 285 está libre: el máximo usado hasta ahora es 284 (Mensajería).
-- ---------------------------------------------------------------------------

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
SELECT 285, 21, 'Módulos', '/module/list', 'fa fa-cubes', '__master', '/module', 68, 1, 'core', 0
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/module/list' OR `id` = 285) AS existente
WHERE existente.n = 0;
