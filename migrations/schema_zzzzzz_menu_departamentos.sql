-- ---------------------------------------------------------------------------
-- MENÚ POR DEPARTAMENTOS (2026-11-27).
--
-- QUÉ PROBLEMA RESUELVE. El menú tenía 24 entradas de primer nivel y el mismo
-- tema repartido en dos sitios: «Clientes» por un lado y «Cuentas por Cobrar»
-- a 100 entradas de distancia; «Compras» y «Proveedores» separados de «Cuentas
-- por Pagar»; «Productos», «Inventario», «Gestión Inventario», «Almacén» y
-- «Ajuste Inventario» como cinco puertas distintas al mismo almacén; «Libros
-- Legales» fuera de «Contabilidad»; «Informes Personalizados» y «Pista de
-- Auditoría» sueltos en la raíz. El resultado se ve a la primera: una lista
-- larga donde cada pantalla hay que buscarla.
--
-- CÓMO QUEDA. CATORCE departamentos, cada uno con los enlaces de su función:
--
--   Inicio · Ventas · Clientes y cobros · Compras y proveedores ·
--   Catálogo de productos · Inventario y almacén · Contabilidad ·
--   Impuestos y libros legales · Reportes · CRM · Costos · Recursos Humanos ·
--   Inteligencia artificial · Configuración
--
-- SE REUTILIZAN LAS FILAS QUE YA EXISTÍAN como raíz de su departamento
-- (Facturación pasa a ser Ventas, Compras absorbe Proveedores, Gestión
-- Inventario absorbe Productos/Stock/Almacén, etc.). No se crea un árbol
-- paralelo ni se duplica nada: cada pantalla conserva su fila —y con ella su
-- `perm`— y solo cambia de padre y de orden.
--
-- LO QUE SE ESCONDE, NO SE BORRA. Las filas que quedan como grupo vacío o como
-- enlace repetido pasan a `status = 0`. Siguen en la tabla (el Gestor de Menús
-- las muestra y se pueden reactivar) pero el sidebar no las pinta. Borrarlas
-- rompería el historial de un menú que el administrador puede haber movido.
--
-- DOS PERMISOS SE ARREGLAN AL BAJAR DE NIVEL. «Factura de Servicio» y los cuatro
-- enlaces de «Bancos» no tenían `perm` propio: los protegía el grupo del que
-- colgaban. Al desaparecer ese grupo hay que poner el permiso en cada enlace o
-- quedarían visibles para cualquiera. Se les pone el mismo que tenía el grupo.
--
-- EL ORDEN DEL SIDEBAR ES EL DE UN DÍA DE TRABAJO: primero lo que se vende
-- (Ventas, Clientes), después lo que se compra, el almacén, la contabilidad y
-- al final lo que se configura. Todo idempotente: se puede aplicar dos veces.
-- ---------------------------------------------------------------------------

-- 1) LOS CATORCE DEPARTAMENTOS ---------------------------------------------
UPDATE menu SET parent_id = NULL, label = 'Inicio', icon = 'ti-dashboard', sort_order = 10, status = 1 WHERE id = 1;
UPDATE menu SET parent_id = NULL, label = 'Ventas', icon = 'fa fa-shopping-bag', sort_order = 20, status = 1 WHERE id = 2;
UPDATE menu SET parent_id = NULL, label = 'Clientes y cobros', icon = 'fa fa-users', sort_order = 30, status = 1 WHERE id = 3;
UPDATE menu SET parent_id = NULL, label = 'Compras y proveedores', icon = 'fa fa-shopping-cart', sort_order = 40, status = 1 WHERE id = 6;
UPDATE menu SET parent_id = NULL, label = 'Productos', icon = 'fa fa-cubes', sort_order = 50, status = 1 WHERE id = 5;
UPDATE menu SET parent_id = NULL, label = 'Inventario y almacén', icon = 'fa fa-archive', sort_order = 60, status = 1 WHERE id = 5000;
UPDATE menu SET parent_id = NULL, label = 'Contabilidad', icon = 'fa fa-calculator', sort_order = 70, status = 1 WHERE id = 15;
UPDATE menu SET parent_id = NULL, label = 'Impuestos y libros', icon = 'fa fa-balance-scale', sort_order = 80, status = 1 WHERE id = 17;
UPDATE menu SET parent_id = NULL, label = 'Reportes', icon = 'fa fa-files-o', sort_order = 90, status = 1 WHERE id = 16;
UPDATE menu SET parent_id = NULL, label = 'CRM', icon = 'fa fa-briefcase', sort_order = 100, status = 1 WHERE id = 18;
UPDATE menu SET parent_id = NULL, label = 'Costos', icon = 'fa fa-percent', sort_order = 110, status = 1 WHERE id = 19;
UPDATE menu SET parent_id = NULL, label = 'Recursos Humanos', icon = 'fa fa-users', sort_order = 120, status = 1 WHERE id = 20;
UPDATE menu SET parent_id = NULL, label = 'Asistentes IA', icon = 'fa fa-magic', sort_order = 130, status = 1 WHERE id = 22;
UPDATE menu SET parent_id = NULL, label = 'Configuración', icon = 'fa fa-cog', sort_order = 140, status = 1 WHERE id = 21;

-- 2) VENTAS: facturar, cotizar, acreditar y devolver ------------------------
UPDATE menu SET parent_id = 2, sort_order = 10 WHERE url = '/invoice/new';
UPDATE menu SET parent_id = 2, sort_order = 20 WHERE url = '/invoice/list';
UPDATE menu SET parent_id = 2, sort_order = 30 WHERE url = '/quotation/new';
UPDATE menu SET parent_id = 2, sort_order = 40 WHERE url = '/quotation/list';
UPDATE menu SET parent_id = 2, sort_order = 50 WHERE url = '/creditnote/list';
UPDATE menu SET parent_id = 2, sort_order = 60 WHERE url = '/return/list';
UPDATE menu SET parent_id = 2, sort_order = 70 WHERE url = '/service/add';
UPDATE menu SET parent_id = 2, sort_order = 80 WHERE url = '/service/list';
-- El grupo «Servicios» tenía el permiso; el enlace no. Se le pone el mismo.
UPDATE menu SET parent_id = 2, sort_order = 90, perm = 'create_service|manage_service' WHERE url = '/service/invoice/new';

-- 3) CLIENTES Y COBROS: la ficha y lo que se le cobra -----------------------
UPDATE menu SET parent_id = 3, sort_order = 10 WHERE url = '/customer/add';
UPDATE menu SET parent_id = 3, sort_order = 20 WHERE url = '/customer/list';
UPDATE menu SET parent_id = 3, sort_order = 30 WHERE url = '/customer/credit';
UPDATE menu SET parent_id = 3, sort_order = 40 WHERE url = '/customer/paid';
UPDATE menu SET parent_id = 3, sort_order = 50, label = 'Libro de clientes' WHERE url = '/customer/ledger';
UPDATE menu SET parent_id = 3, sort_order = 60 WHERE url = '/cartera/list';

-- 4) COMPRAS Y PROVEEDORES: lo que se compra y lo que se debe ---------------
UPDATE menu SET parent_id = 6, sort_order = 10 WHERE url = '/purchase/add';
UPDATE menu SET parent_id = 6, sort_order = 20 WHERE url = '/purchase/list';
UPDATE menu SET parent_id = 6, sort_order = 30 WHERE url = '/supplier/add';
UPDATE menu SET parent_id = 6, sort_order = 40 WHERE url = '/supplier/list';
UPDATE menu SET parent_id = 6, sort_order = 50, label = 'Libro de proveedores' WHERE url = '/supplier/ledger';
UPDATE menu SET parent_id = 6, sort_order = 60 WHERE url = '/pagar/list';

-- 5) CATÁLOGO DE PRODUCTOS: lo que se vende --------------------------------
UPDATE menu SET parent_id = 5, sort_order = 10 WHERE url = '/product/category';
UPDATE menu SET parent_id = 5, sort_order = 20 WHERE url = '/product/unit';
UPDATE menu SET parent_id = 5, sort_order = 30 WHERE url = '/product/add';
UPDATE menu SET parent_id = 5, sort_order = 40 WHERE url = '/product/list';

-- 6) INVENTARIO Y ALMACÉN: existencias, almacenes, traslados y ajustes ------
UPDATE menu SET parent_id = 5000, sort_order = 10 WHERE url = '/reports/stock';
UPDATE menu SET parent_id = 5000, sort_order = 20 WHERE url = '/warehouse/list';
UPDATE menu SET parent_id = 5000, sort_order = 30 WHERE url = '/delivery_order/add';
UPDATE menu SET parent_id = 5000, sort_order = 40 WHERE url = '/delivery_order/list';
UPDATE menu SET parent_id = 5000, sort_order = 50, perm = 'adjustment' WHERE url = '/adjustment/new';
UPDATE menu SET parent_id = 5000, sort_order = 60, perm = 'adjustment', label = 'Historial de ajustes' WHERE url = '/adjustment/list';
UPDATE menu SET parent_id = 5000, sort_order = 70 WHERE url = '/consumption/new';
-- La pantalla de consumos existía y no estaba en el menú: se agregaba solo el
-- alta, así que el listado histórico no tenía puerta.
INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id, is_core)
VALUES (9101, 5000, 'Consumos', '/consumption/list', NULL, NULL, NULL, 80, 1, 'core', 1)
ON DUPLICATE KEY UPDATE parent_id = 5000, label = 'Consumos', url = '/consumption/list',
  sort_order = 80, status = 1, module_id = 'core', is_core = 1;

-- 7) CONTABILIDAD: plan, comprobantes, libros y bancos ----------------------
UPDATE menu SET parent_id = 15, sort_order = 10 WHERE url = '/account/coa';
UPDATE menu SET parent_id = 15, sort_order = 20 WHERE url = '/account/coa/catalog';
UPDATE menu SET parent_id = 15, sort_order = 30 WHERE url = '/account/vouchers';
UPDATE menu SET parent_id = 15, sort_order = 40 WHERE url = '/account/voucher/new-multi';
UPDATE menu SET parent_id = 15, sort_order = 50 WHERE url = '/account/ledger';
UPDATE menu SET parent_id = 15, sort_order = 60 WHERE url = '/account/trial-balance';
UPDATE menu SET parent_id = 15, sort_order = 70 WHERE url = '/account/profit-loss';
UPDATE menu SET parent_id = 15, sort_order = 80 WHERE url = '/account/balance-sheet';
-- El grupo «Bancos» tenía el permiso; sus cuatro enlaces no. Se les pone.
UPDATE menu SET parent_id = 15, sort_order = 90, perm = 'bank|bank_ledger' WHERE url = '/bank/add';
UPDATE menu SET parent_id = 15, sort_order = 100, perm = 'bank|bank_ledger' WHERE url = '/bank/list';
UPDATE menu SET parent_id = 15, sort_order = 110, perm = 'bank|bank_ledger' WHERE url = '/bank/ledger';
UPDATE menu SET parent_id = 15, sort_order = 120, perm = 'bank|bank_ledger' WHERE url = '/bank/transaction';

-- 8) IMPUESTOS Y LIBROS LEGALES: lo que se presenta al SENIAT ----------------
UPDATE menu SET parent_id = 17, sort_order = 10 WHERE url = '/reports/book-purchases';
UPDATE menu SET parent_id = 17, sort_order = 20 WHERE url = '/reports/book-sales';
UPDATE menu SET parent_id = 17, sort_order = 30 WHERE url = '/reports/inventory-control';
UPDATE menu SET parent_id = 17, sort_order = 40 WHERE url = '/reports/kardex';
UPDATE menu SET parent_id = 17, sort_order = 50 WHERE url = '/reports/vat-retention';
UPDATE menu SET parent_id = 17, sort_order = 60 WHERE url = '/reports/islr-retention';
UPDATE menu SET parent_id = 17, sort_order = 70 WHERE url = '/reports/exchange-difference';

-- 9) REPORTES: los informes de gestión (el stock vive en Inventario) --------
UPDATE menu SET parent_id = 16, sort_order = 10, label = 'Reporte de ventas' WHERE url = '/reports/sales';
UPDATE menu SET parent_id = 16, sort_order = 20, label = 'Reporte de compras' WHERE url = '/reports/purchases';
UPDATE menu SET parent_id = 16, sort_order = 30, label = 'Reporte de impuestos' WHERE url = '/reports/tax';
UPDATE menu SET parent_id = 16, sort_order = 40 WHERE url = '/reports/daily-closing';
UPDATE menu SET parent_id = 16, sort_order = 50 WHERE url = '/reports/custom';
UPDATE menu SET parent_id = 16, sort_order = 60 WHERE url = '/reports/audit-log';

-- 10) CRM: el embudo y sus campañas, sin un grupo de tercer nivel -----------
UPDATE menu SET parent_id = 18, sort_order = 10 WHERE url = '/crm';
UPDATE menu SET parent_id = 18, sort_order = 20 WHERE url = '/crm/contact/list';
UPDATE menu SET parent_id = 18, sort_order = 30 WHERE url = '/crm/pipeline';
UPDATE menu SET parent_id = 18, sort_order = 40 WHERE url = '/crm/remarketing';
UPDATE menu SET parent_id = 18, sort_order = 50 WHERE url = '/crm/remarketing/campaigns';
UPDATE menu SET parent_id = 18, sort_order = 60 WHERE url = '/crm/remarketing/segments';
UPDATE menu SET parent_id = 18, sort_order = 70 WHERE url = '/crm/remarketing/templates';

-- 11) COSTOS ----------------------------------------------------------------
UPDATE menu SET parent_id = 19, sort_order = 10, label = 'Configuración de costos' WHERE url = '/cost';
UPDATE menu SET parent_id = 19, sort_order = 20 WHERE url = '/cost/timesheet';
UPDATE menu SET parent_id = 19, sort_order = 30 WHERE url = '/cost/proyectos';
UPDATE menu SET parent_id = 19, sort_order = 40 WHERE url = '/cost/period';

-- 12) RECURSOS HUMANOS ------------------------------------------------------
UPDATE menu SET parent_id = 20, sort_order = 10 WHERE url = '/hr';
UPDATE menu SET parent_id = 20, sort_order = 20 WHERE url = '/hr/employee/list';
UPDATE menu SET parent_id = 20, sort_order = 30 WHERE url = '/hr/tax-rates';
UPDATE menu SET parent_id = 20, sort_order = 40 WHERE url = '/hr/leave';
UPDATE menu SET parent_id = 20, sort_order = 50 WHERE url = '/hr/payroll';

-- 13) INTELIGENCIA ARTIFICIAL ----------------------------------------------
UPDATE menu SET parent_id = 22, sort_order = 10 WHERE url = '/ai/settings';
UPDATE menu SET parent_id = 22, sort_order = 20 WHERE url = '/ai/chat';
UPDATE menu SET parent_id = 22, sort_order = 30 WHERE url = '/ai/agents';
UPDATE menu SET parent_id = 22, sort_order = 40 WHERE url = '/ai/tasks';
UPDATE menu SET parent_id = 22, sort_order = 50 WHERE url = '/ai/catalog';

-- 14) CONFIGURACIÓN: primero lo de la empresa, después usuarios y menú ------
UPDATE menu SET parent_id = 21, sort_order = 10 WHERE url = '/settings';
UPDATE menu SET parent_id = 21, sort_order = 20 WHERE url = '/settings/fiscal';
UPDATE menu SET parent_id = 21, sort_order = 30 WHERE url = '/settings/digital-invoice';
UPDATE menu SET parent_id = 21, sort_order = 40 WHERE url = '/settings/company';
UPDATE menu SET parent_id = 21, sort_order = 50 WHERE url = '/settings/currency';
UPDATE menu SET parent_id = 21, sort_order = 60 WHERE url = '/settings/exchange-rate';
UPDATE menu SET parent_id = 21, sort_order = 70 WHERE url = '/settings/backup';
UPDATE menu SET parent_id = 21, sort_order = 80 WHERE url = '/finance/mensajes';
UPDATE menu SET parent_id = 21, sort_order = 90 WHERE url = '/module/list';
UPDATE menu SET parent_id = 21, sort_order = 100 WHERE url = '/settings/database';
UPDATE menu SET parent_id = 21, sort_order = 110 WHERE url = '/user/list';
UPDATE menu SET parent_id = 21, sort_order = 120 WHERE url = '/role/list';
UPDATE menu SET parent_id = 21, sort_order = 130 WHERE url = '/config/menu';

-- Canales de aviso se quedaba sin puerta: su alta (finance_alertas.sql) pedía
-- el id 275, que ya usa Facturación Digital, y MySQL rechaza el INSERT por
-- clave duplicada. La pantalla existe (/settings/notify). Se siembra con su id.
INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id, is_core)
VALUES (277, 21, 'Canales de aviso', '/settings/notify', NULL, 'setting', '/settings/notify', 35, 1, 'core', 1)
ON DUPLICATE KEY UPDATE parent_id = 21, label = 'Canales de aviso', url = '/settings/notify',
  perm = 'setting', active_prefix = '/settings/notify', sort_order = 35, status = 1;

-- 15) LO QUE SE ESCONDE (grupos vacíos y pantallas repetidas) ---------------
--  4 Proveedores            → sus enlaces están en Compras y proveedores
--  7 Inventario (enlace)    → el mismo stock está como «Stock» en Inventario
--  8 Servicios              → sus enlaces están en Ventas
-- 12 Cotizaciones           → sus enlaces están en Ventas
-- 13 Bancos                 → sus enlaces están en Contabilidad
-- 14 Ajuste Inventario      → el ajuste está en Inventario (5003/5004)
-- 190/191 Ajuste (duplicado)→ se queda el par 5003/5004
-- 212 Inventario (reporte)  → la misma pantalla está como «Stock»
-- 233 Campañas              → sus tres enlaces están en CRM
-- 5002 Ajuste de Inventario → grupo vacío tras mover sus hijos
-- 5006 Consumo Interno      → grupo vacío tras mover su hijo
-- 5100 Almacén              → sus enlaces están en Inventario y almacén
UPDATE menu SET status = 0
WHERE id IN (4, 7, 8, 12, 13, 14, 190, 191, 212, 233, 5002, 5006, 5100);
