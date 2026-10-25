-- ---------------------------------------------------------------------------
-- MENÚ DEL MÓDULO <Modulo> — copiar dentro de migrations/schema_z_<modulo>.sql
--
-- El sidebar ya NO se edita en templates/layout.html: se arma desde la tabla
-- `menu` (interfaces/web/menu.go). Este fichero va en el MISMO .sql que el paso 1.
--
-- Columnas:
--   id            fijo y único (usa 900+ para no chocar con la semilla actual,
--                 que va de 1 a 274).
--   parent_id     NULL = primer nivel; o el id de su grupo.
--   label         lo que se lee en pantalla (español, con acentos).
--   url           NULL = grupo desplegable; o la ruta del enlace.
--   icon          clases CSS del icono, solo en primer nivel.
--   perm          sub_module.directory; vacío = público. "a|b" = cualquiera.
--                 DEBE existir en sub_module.status=1 (si no, no lo verá nadie,
--                 ni el admin: hay un test que lo comprueba).
--   active_prefix marca el enlace activo también en sus subrutas.
--   sort_order    orden dentro de su nivel.
--   status        1 = visible; 0 = oculto sin borrar.
--
-- IMPORTANTE: un grupo con algún hijo público (perm vacío) TIENE que llevar su
-- propio perm, o aparecerá para todos los usuarios.
-- ---------------------------------------------------------------------------

INSERT INTO `menu`
  (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
VALUES
-- Grupo de primer nivel.
(900, NULL, '<Modulo>', NULL, 'metismenu-icon fa fa-cubes', '<modulo>', NULL, 300, 1),
-- Enlaces del grupo.
(910, 900, 'Ver <Modulo>', '/<modulo>', NULL, '<modulo>', NULL, 10, 1),
(911, 900, 'Configurar', '/<modulo>/settings', NULL, '<modulo>', '/<modulo>/settings', 20, 1);
