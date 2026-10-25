-- ---------------------------------------------------------------------------
-- LIMPIEZA DEL MENÚ LATERAL (2026-10-18).
--
-- POR QUÉ ES UNA MIGRACIÓN NUEVA Y NO UNA EDICIÓN DE schema_z_menu.sql.
-- Aquel archivo ya está anotado como aplicado en todas las instalaciones que
-- existen; editarlo obliga a re-ejecutar todo su INSERT (que se tolera, id a id)
-- y a arriesgar el orden con los addons, que también escriben en `menu`. Aquí
-- sólo se RETIRA lo que sobra y se AÑADE lo que faltaba: es idempotente por
-- construcción y sobre una base ya en uso deja exactamente el mismo estado.
--
-- 1) SE RETIRAN DOS ENTRADAS QUE NO LLEVAN A NINGUNA PARTE.
--
--    «POS» -> /invoice/pos. El POS es OTRO PROYECTO: se comunica con el ERP por
--    API y gRPC, pero el ERP no factura tipo caja registradora. La pantalla se
--    retiró de las rutas y la entrada de menú quedó apuntando a una URL que
--    responde 404: un menú que promete algo que ya no está.
--
--    «Sincronizador» -> /synchronizer. No hay ningún handler registrado para esa
--    ruta (se buscó en interfaces/web: cero coincidencias), así que también era
--    un 404 desde el menú. La sincronización del POS se sigue viendo por API.
--
-- 2) SE AÑADE «Vacaciones y ausencias» (/hr/leave).
--
--    La pantalla y sus endpoints existen (interfaces/web/hr_leave_handlers.go)
--    y no había forma de llegar a ella desde el menú: existía para quien se
--    supiera la URL. Va bajo Recursos Humanos y con el mismo permiso que el
--    resto del módulo, así que sólo la ven quienes ya gestionan personal.
-- ---------------------------------------------------------------------------

DELETE FROM `menu` WHERE `url` IN ('/invoice/pos', '/synchronizer');

-- 3) SE RETIRA EL PERMISO «POS» DE LA MATRIZ DE ROLES.
--
-- sub_module.directory = 'gui_pos' (nombre 'pos_invoice') existía sólo para la
-- pantalla del POS web. Al desaparecer la pantalla, en Roles y Permisos quedaba
-- una casilla que no autoriza nada: marcarla no enciende ninguna función y
-- desmarcarla no apaga ninguna. Un permiso que se puede conceder y no hace
-- nada es peor que ninguno, porque hace dudar del resto de la matriz.
--
-- Se borran primero las concesiones (role_permission apunta a sub_module.id) y
-- después la definición. Ambas sentencias son idempotentes: repetirlas no
-- cambia nada, que es lo que necesita una migración que puede re-ejecutarse.
DELETE rp FROM role_permission rp
  JOIN sub_module sm ON sm.id = rp.fk_module_id
 WHERE sm.directory = 'gui_pos';
DELETE FROM sub_module WHERE directory = 'gui_pos';

INSERT IGNORE INTO `menu`
  (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
VALUES
  (254, 20, 'Vacaciones y ausencias', '/hr/leave', NULL, 'hr_employee|hr_payroll', '/hr/leave', 35, 1);
