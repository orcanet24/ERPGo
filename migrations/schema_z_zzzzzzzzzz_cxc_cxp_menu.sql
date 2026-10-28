-- ============================================================================
-- ERPGo — Cuentas por Cobrar y Cuentas por Pagar como SECCIONES del menú.
--
-- POR QUÉ EXISTE ESTE ARCHIVO. Las dos entradas las creó
-- schema_z_zzzzzzz_compras_cobros.sql, pero COLGADAS de Clientes y de
-- Proveedores: quedaban como el ÚLTIMO renglón dentro de otro grupo, al lado de
-- «Libro Mayor». Son módulos propios, con su permiso propio, y quien los busca
-- en el primer nivel —que es donde están Devoluciones, Inventario o Contabilidad—
-- no los encuentra. Peor: en una base donde esa migración no llegó a aplicarse
-- (instalación vieja, o el archivo quedó «NO ANOTADA»), la entrada no existía en
-- absoluto y no había forma de llegar a /cartera/list ni a /pagar/list desde la
-- interfaz, sólo escribiendo la dirección a mano.
--
-- QUÉ HACE, en dos pasos, sin duplicar nada:
--
--   1. CREA la entrada si falta. El INSERT es condicional por URL y va dentro de
--      una tabla derivada a propósito: MySQL rechaza (error 1093) referirse a la
--      misma tabla que se está insertando en un FROM directo, y esta forma corre
--      igual en MySQL y en SQLite (ver schema_z_supplier_payment.sql, que usa el
--      mismo patrón por las mismas dos razones).
--
--   2. LA DEJA EN EL PRIMER NIVEL con su orden, su icono y su permiso, exista de
--      antes o no. El UPDATE va por URL y no por id: no depende del id que le
--      haya tocado a la fila en una base migrada del PHP, y es lo que corrige el
--      parent_id que había puesto la migración anterior.
--
-- LOS PERMISOS NO CAMBIAN: «customer_ledger» para la cartera y
-- «supplier_payment» para las obligaciones, que son los que el módulo ya usa en
-- Roles y Permisos. Quien no los tenga sigue sin ver la sección —y el middleware
-- le sigue respondiendo 403 en la ruta—, así que el menú y el permiso dicen lo
-- mismo. Eso importa aquí más que en otras pantallas: el menú es la primera
-- promesa que el sistema le hace al usuario.
--
-- La prueba que cubre esto —interfaces/web/menu_cxc_cxp_test.go— lee estos
-- archivos, aplica las sentencias sobre el menú en orden y comprueba que las dos
-- entradas quedan de primer nivel, visibles para quien tiene el permiso y
-- escondidas para quien no.
-- ============================================================================

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
SELECT 115, NULL, 'Cuentas por Cobrar', '/cartera/list', 'metismenu-icon fa fa-credit-card', 'customer_ledger', '/cartera', 35, 1
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/cartera/list') AS existente
WHERE existente.n = 0;

INSERT INTO `menu` (`id`, `parent_id`, `label`, `url`, `icon`, `perm`, `active_prefix`, `sort_order`, `status`)
SELECT 123, NULL, 'Cuentas por Pagar', '/pagar/list', 'metismenu-icon fa fa-money', 'supplier_payment', '/pagar', 45, 1
FROM (SELECT COUNT(*) AS n FROM `menu` WHERE `url` = '/pagar/list') AS existente
WHERE existente.n = 0;

UPDATE `menu` SET `parent_id` = NULL, `label` = 'Cuentas por Cobrar',
  `icon` = 'metismenu-icon fa fa-credit-card', `perm` = 'customer_ledger',
  `active_prefix` = '/cartera', `sort_order` = 35, `status` = 1
WHERE `url` = '/cartera/list';

UPDATE `menu` SET `parent_id` = NULL, `label` = 'Cuentas por Pagar',
  `icon` = 'metismenu-icon fa fa-money', `perm` = 'supplier_payment',
  `active_prefix` = '/pagar', `sort_order` = 45, `status` = 1
WHERE `url` = '/pagar/list';
