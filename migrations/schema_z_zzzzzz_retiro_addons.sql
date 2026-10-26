-- ---------------------------------------------------------------------------
-- RETIRO DEL MENÚ «ADDONS» (2026-10-26).
--
-- POR QUÉ EXISTE ESTA MIGRACIÓN.
-- El sistema de addons del ERP fue retirado del binario (host, rutas /mod/,
-- SDK fuera de go.mod). Pero el menú lateral no es una vista: es la tabla
-- `menu`, y el código que vivía en addon_handlers.go le insertó una fila
-- (id=280, «Addons» → /module/addons/admin, bajo Configuración) con
-- INSERT IGNORE en cada arranque. Al borrar el código, la fila quedó en la
-- base apuntando a rutas que ya no existen: un menú que promete algo que
-- desapareció, exactamente el caso que ya se resolvió para el POS web en
-- schema_z_zzzzz_menu_limpieza.sql.
--
-- SE MARCA INACTIVO, NO SE BORRA.
-- La doctrina del proyecto es que los registros históricos nunca se eliminan:
-- se marcan inactivos. `loadMenu` filtra con `status = 1`, así que con
-- status=0 la entrada desaparece del panel en el próximo arranque, y la fila
-- queda como evidencia de que existió. Idempotente: correrla N veces deja el
-- mismo estado, y no toca nada más del menú.
-- ---------------------------------------------------------------------------

UPDATE `menu` SET `status` = 0
 WHERE `id` = 280
    OR `url` LIKE '/module/addons%';
