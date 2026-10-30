-- ---------------------------------------------------------------------------
-- «Módulos» de vuelta en el menú de Configuración (2026-10-30).
--
-- POR QUÉ VOLVIÓ A FALTAR. La entrada nació en
-- schema_z_zzzzzzzzzzz_mensajeria_modulos_menu.sql (commit 0b58697). Esa
-- migración llevaba el nombre del módulo de mensajería, y cuando éste se
-- retiró íntegro (commit 26fd927) el archivo se fue con ella: cualquier
-- instalación nueva o base reconstruida quedaba sin la entrada de Módulos,
-- aunque la pantalla /module/list seguía existiendo. Nadie puede encender ni
-- apagar un módulo ajeno al núcleo si la puerta del interruptor no existe.
--
-- Esta migración es SUYA, no de ningún módulo en particular: el interruptor
-- de módulos es del núcleo y no muere con ninguno de ellos.
--
-- INSERT en SELECT con recuento: no duplica si la base ya traía la fila (una
-- instalación antigua que nunca perdió la entrada) ni si re-ejecuta.
--
-- `perm = '__master'` es la marca del mapa de permisos para el nivel master
-- (ver cachas.go): el administrador de empresa no ve una entrada que le daría
-- 403. El id 285 es el que ya usaba la entrada original.
-- ---------------------------------------------------------------------------

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`, `module_id`, `is_core`)
SELECT 285, 21, 'Módulos', '/module/list', 'fa fa-cubes', '__master', '/module', 68, 1, 'core', 0
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/module/list' OR `id` = 285) AS existente
WHERE existente.n = 0;
