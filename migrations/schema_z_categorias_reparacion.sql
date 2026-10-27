-- ---------------------------------------------------------------------------
-- Reparación de jerarquía corrupta en product_category.
--
-- POR QUÉ.
--
-- Dos escrituras históricas rompían las rutas materializadas (path/level):
--
--   1. La semilla insertaba categorías sin `path` (semilla_carniceria.go
--      viejo). El backfill que las arregla vive DENTRO de schema_z_categorias-
--      jerarquicas.sql, y el registro de migraciones sella cada archivo tras
--      aplicarlo una vez: las filas sembradas después quedaban con path NULL
--      para siempre.
--   2. categorySave viejo escribía la fila y su camino en DOS sentencias sin
--      transacción: un corte a mitad de camino dejaba filas con parent_id
--      real y path NULL. Y con un padre sin camino, crear un hijo heredaba
--      base vacía y guardaba rutas SIN el slash inicial ('8/9/' en vez de
--      '/8/9/'), con lo que el chequeo de ciclos por prefijo se burlaba y se
--      podía mover una categoría bajo su propia descendiente.
--
-- CÓMO (idempotente, sin procedimientos).
--
--   1. Rompe ciclos que ya existan en parent_id: autobucles y pares
--      padre↔hijo (los únicos que producía el bypass, un solo movimiento).
--      Los miembros quedan raíz.
--   2. Re-canonicaliza en pasadas: cada pasada fija cada nodo respecto del
--      camino que su padre tenga AHORA, en un solo UPDATE con self-join.
--      Repetir el mismo UPDATE hace que la corrección baje de nivel en
--      nivel hasta converger. MySQL 5.7 no tiene CTE recursivas y la sintaxis
--      de procedimientos con DELIMITER no la procesa este ejecutor de
--      migraciones, por eso van N pasadas literales y no un bucle. Diez
--      pasadas cubren profundidades absurdas para un catálogo real (y el
--      path VARCHAR(500) limita la profundidad achevable de todos modos).
--
-- Un arranque posterior no toca ni una fila: cuando nada incumple la
-- condición del WHERE, los UPDATE no cambian nada (los valores ya son los
-- canónicos), así que reaplicable sin miedo.
-- ---------------------------------------------------------------------------

-- Autobucle: una categoría que es su propio padre sube a raíz.
UPDATE product_category
SET parent_id = NULL, level = 0
WHERE parent_id = category_id;

-- Ciclo de dos eslabones: A es padre de B y B es padre de A. El self-join
-- casado simétrico vuelve a emparejar, así que AMBOS pierden el padre y
-- quedan raíz. No existe la vía de datos para ciclos más largos: el bypass
-- salía de UN movimiento (un padre bajo un descendiente directo o
-- indirecto, que siempre cierra en par o autobucle tras la corrupción de
-- camino que esta migración arregla abajo).
UPDATE product_category a
JOIN product_category b
  ON b.category_id = a.parent_id
 AND a.category_id = b.parent_id
SET a.parent_id = NULL, a.level = 0;

-- Pasada 1: canon es la ruta del padre + el propio id, o raíz si no hay
-- padre válido (NULL, colgando de un id borrado/dañado). Además de la ruta
-- se repara el level, y el padre colgante se suelta. Con path ya canónico y
-- level correcto el WHERE no casa y no se escribe nada.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 2: igual que la 1. Ya no procura nada arriba (hasta nivel 2 los
-- padres ya quedaron canónicos en la pasada anterior); baja la corrección
-- un nivel más.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 3.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 4.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 5.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 6.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 7.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 8.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 9.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);

-- Pasada 10 (última). De cubrirse ramas más profundas, con solo re-ejecutar
-- `erpgow -migrate` tras sumar pasadas se terminan de arreglar; los caminos
-- de un catálogo real nunca llegan aquí.
UPDATE product_category h
LEFT JOIN product_category p ON p.category_id = h.parent_id
SET h.parent_id = IF(p.category_id IS NOT NULL, h.parent_id, NULL),
    h.path = CONCAT(
      CASE WHEN p.category_id IS NOT NULL THEN p.path ELSE '' END,
      h.category_id, '/'),
    h.level = IF(p.category_id IS NOT NULL, p.level + 1, 0)
WHERE h.parent_id IS NULL
   OR p.category_id IS NULL
   OR h.path IS NULL
   OR h.path = ''
   OR h.path <> CONCAT(IFNULL(p.path, ''), h.category_id, '/')
   OR h.level <> IF(p.category_id IS NOT NULL, p.level + 1, 0);
