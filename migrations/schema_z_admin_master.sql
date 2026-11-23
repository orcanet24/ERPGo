-- ============================================================================
-- Bootstrap de autoridad: el administrador inicial queda MASTER de su empresa
-- ============================================================================
-- (Auditoría atómica 2026-11-22; descubierto al verificar en vivo la pantalla
-- de tokens MCP el 2026-11-23.)
--
-- El problema, medido contra una instalación NUEVA: la siembra crea al admin
-- con user_type=1, pero ninguna parte sembraba sec_role ni lo ataba en
-- sec_userrole. En el login, sin rol, el nivel caía a "admin"; y las rutas de
-- sistema (/ai, /users, /tenants, /settings de empresa) exigen "master".
-- Resultado: el dueño de una instalación recién creada no podía abrir la
-- pantalla de IA, ni asignar roles, ni gestionar usuarios — justo las pantallas
-- que necesita para dejar de estar bloqueado. En los despliegues existentes se
-- había tapado a mano (UPDATE sec_role SET level='master'), que es la marca de
-- agua de este agujero.
--
-- Idempotente y portable (sin ON DUPLICATE, sin DUAL):
--  1) crea el rol maestro solo si la base no tiene NINGÚN nivel 'master';
--  2) ata a ese rol a TODO usuario administrador (user_type=1) que no tenga
--     ningún rol. Un usuario con rol asignado respeta la decisión humana: no
--     se le pisa nada.

INSERT INTO `sec_role` (`type`, `level`, `description`, `is_system`)
SELECT 'super', 'master', 'Super Administrador del sistema', 1
FROM (SELECT 1) AS t
WHERE NOT EXISTS (SELECT 1 FROM `sec_role` WHERE `level` = 'master');

INSERT INTO `sec_userrole` (`user_id`, `roleid`, `createby`, `createdate`)
SELECT u.`user_id`, MIN(r.`id`), 'bootstrap', NOW()
FROM `user_login` u
JOIN `sec_role` r ON r.`level` = 'master'
WHERE u.`user_type` = 1
  AND u.`status` = 1
  AND NOT EXISTS (SELECT 1 FROM `sec_userrole` ur WHERE ur.`user_id` = u.`user_id`)
GROUP BY u.`user_id`;
