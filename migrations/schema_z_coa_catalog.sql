-- Catálogo contable por empresa.
--
-- El plan de cuentas NO es del sistema: es de la empresa. Unabreaking que lleva
-- su propia codificación debe poder usarla, y una que recién empieza necesita un
-- punto de partida estándar. Por eso el catálogo es un DATO CON CONFIGURACIÓN y
-- no una constante en el código.
--
-- `catalog` admite:
--   'propio'   la empresa define sus códigos (por defecto)
--   'conasev'  se cargó el catálogo del SENIAT como estándar ofrecido
--
-- Guardarlo aquí, y no deducirlo del contenido de acc_coa, es deliberado: si se
-- dedujera, bastaría una cuenta que coincidiera para que el sistema creyera que
-- la empresa usa un catálogo cuando no. Una decisión explícita se puede mostrar y
-- corregir; una inferencia no.
CREATE TABLE IF NOT EXISTS `coa_catalog` (
  `id`          INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `catalog`     varchar(20) NOT NULL DEFAULT 'propio',
  `source`      varchar(40) NOT NULL DEFAULT '',
  `applied_at`  varchar(20) NOT NULL DEFAULT '',
  `notes`       text,
  `created_by`  varchar(50) NOT NULL DEFAULT '',
  `created_at`  varchar(20) NOT NULL DEFAULT ''
);

-- Un solo ajuste vigente por empresa: si se cargó un catálogo nuevo, el anterior
-- se reemplaza en vez de acumular filas que luego contradicen.
CREATE UNIQUE INDEX idx_coa_catalog_single ON `coa_catalog`(`id`);