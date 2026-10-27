-- ---------------------------------------------------------------------------
-- Permiso «Pagos a proveedor» (supplier_payment).
--
-- POR QUÉ EXISTE. Registrar un pago a un proveedor es una operación que saca
-- dinero y mueve el libro (débito a la cuenta del proveedor, crédito a caja o
-- banco). Hasta ahora esa ruta no tenía permiso propio: se colgaba del módulo de
-- compras y cualquier rol con acceso al módulo podía pagar. En la matriz de Roles
-- y Permisos no había forma de decir «este rol ve proveedores y no paga».
--
-- EL NOMBRE NO ES CASUAL: el PHP original ya usaba el directorio
-- `supplier_payment`, así que una base migrada puede tener su fila (y sus
-- concesiones en role_permission) desde antes. Por eso el INSERT es
-- condicional POR DIRECTORIO y sin id explícito: si la fila ya está —con
-- cualquier id— no se toca y las concesiones existentes siguen valiendo; si no
-- está, entra con el id que le dé el motor. Un id fijo (46) chocaría con la fila
-- heredada y el INSERT IGNORE la saltaría en silencio, dejando el permiso
-- imposible de conceder.
--
-- La subconsulta va dentro de una tabla derivada a propósito: MySQL rechaza
-- (error 1093) referirse a la misma tabla que se está insertando en un FROM
-- directo, y esta sentencia tiene que correr igual en MySQL y en SQLite.
-- ---------------------------------------------------------------------------

INSERT INTO `sub_module` (`mid`, `name`, `directory`, `status`)
SELECT 4, 'supplier_payment', 'supplier_payment', 1
FROM (SELECT COUNT(*) AS n FROM `sub_module` WHERE `directory` = 'supplier_payment') AS existente
WHERE existente.n = 0;
