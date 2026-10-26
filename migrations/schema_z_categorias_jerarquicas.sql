-- ---------------------------------------------------------------------------
-- Categorías jerárquicas: familias, grupos y subcategorías.
--
-- POR QUÉ.
--
-- product_category era plana: category_id, category_name, status. Sin niveles
-- no hay forma de representar "Calzado > Hombres > Deportivos", que es como un
-- comercio realmente piensa su catálogo. La plana solo sirve para listas
-- cortas; con catálogos reales se convierte en un wall de nombres sin familia.
--
-- CÓMO (aditivo y idempotente).
--
-- Cuatro columnas nuevas con default que deja las filas existentes como RAÍZ:
--   parent_id  — el padre; NULL = raíz (todas las actuales quedan ahí).
--   path       — ruta materializada '/1/7/23/': trae una rama entera con un
--                LIKE y sin recursión en SQL (MySQL 5.7 no tiene CTE).
--   level      — profundidad, para indentar en pantalla sin calcular nada.
--   sort_order — orden entre hermanos que el administrador puede cambiar.
--
-- Ninguna fila existente cambia de significado: sus productos siguen en la
-- misma categoría, que ahora es raíz. Quien no toque nada no nota el cambio.
--
-- El backfill usa IF sobre la condición de "está vacío" para poder repetirse:
-- un segundo arranque no vuelve a tocar nada.
--
-- Orden: después de schema_reconstruido_02_inventory.sql (que crea la tabla
-- vacía en instalaciones nuevas) y sin dependencia del resto.
-- ---------------------------------------------------------------------------

-- parent_id ---------------------------------------------------------------
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_category'
    AND COLUMN_NAME = 'parent_id'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_category` ADD COLUMN `parent_id` INT DEFAULT NULL',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- path --------------------------------------------------------------------
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_category'
    AND COLUMN_NAME = 'path'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_category` ADD COLUMN `path` VARCHAR(500) NULL',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- level -------------------------------------------------------------------
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_category'
    AND COLUMN_NAME = 'level'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_category` ADD COLUMN `level` TINYINT NOT NULL DEFAULT 0',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- sort_order --------------------------------------------------------------
SET @col := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_category'
    AND COLUMN_NAME = 'sort_order'
);
SET @sql := IF(@col = 0,
  'ALTER TABLE `product_category` ADD COLUMN `sort_order` INT NOT NULL DEFAULT 0',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;

-- Backfill: lo que no tiene ruta es raíz con su propio id como ruta. Es una
-- sola sentencia condicionada a que path esté vacío, así que un arranque
-- posterior no la repite.
UPDATE `product_category`
SET `path` = CONCAT('/', category_id, '/'), `level` = 0
WHERE `path` IS NULL OR `path` = '';

-- Índice para "¿tiene hijos?" y para buscar la rama de un nodo.
SET @idx := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'product_category'
    AND INDEX_NAME = 'idx_cat_parent'
);
SET @sql := IF(@idx = 0,
  'ALTER TABLE `product_category` ADD INDEX `idx_cat_parent` (`parent_id`)',
  'DO 0');
PREPARE st FROM @sql;
EXECUTE st;
DEALLOCATE PREPARE st;