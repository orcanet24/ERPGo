-- ---------------------------------------------------------------------------
-- Plantillas de operación (cache de "pautas de trabajo" de la IA).
--
-- Cuando la IA resuelve CÓMO hacer una operación estructurada (qué cuentas,
-- qué tasa, qué formato) para una categoría dada, ese "cómo" se guarda como
-- plantilla. La siguiente solicitud de la misma categoría reutiliza la plantilla
-- y solo cambia las variables (cliente, monto, fecha). Ahorro de tokens real:
-- la IA razona una vez y reutiliza muchas.
--
-- Reglas anti-basura (la condición es que NO acumule basura con el tiempo):
--   - TTL 90 días sin uso → se borra sola.
--   - Máximo 50 plantillas por agente → LRU (menos usado se echa).
--   - Los primeros usos de cada plantilla nueva pasan por revisión humana
--     (needs_human, igual que el journal ya hace con las acciones críticas).
--   - Categoría = operación + campos estructurados normalizados (vtype, etc.);
--     no hay fuzzy matching que pueda solapar dos operaciones distintas.
--
-- Negocio: ORC24 (IA propia) ahorra tokens con el cache y cuenta barato.
-- OpenAI / Anthropic cuentan inferencias normales pero NO aplican el ahorro
-- de sistema → el cliente paga el precio real del proveedor (ventaja
-- competitiva de usar la IA propia).
--
-- Convención: ordena DESPUÉS de schema_z_ai_operations.sql (que crea ai_cache y
-- ai_operations) y ANTES de cualquier archivo posterior.
-- ---------------------------------------------------------------------------

-- Flag por proveedor: ¿aplica cache de operaciones del sistema?
-- ORC24 = 1 (ahorra, cuenta barato). OpenAI / Anthropic = 0 (pagan real).
ALTER TABLE `ai_providers` ADD COLUMN IF NOT EXISTS `op_cache` TINYINT NOT NULL DEFAULT 0;

-- Plantillas de operación por agente.
-- operation:     nombre interno de la herramienta (account.post_voucher, ...).
-- template_key:  operation + campos estructurados normalizados del request
--                (vtype, causa, tipo_doc). Determinístico: dos requests con los
--                mismos campos estructurados generan la MISMA key. No hay magia.
-- pattern:       JSON con la parte FIJA que la IA decidió (cuentas, tasas,
--                formato de líneas, formato de descripción).
-- var_schema:    JSON con los nombres de los campos que VARÍAN y deben pedirse.
-- usos:          cuántas veces se ha reutilizado (para LRU y para estadística).
-- needs_human:   1 los primeros usos (revisión obligatoria), 0 cuando ya validada.
-- last_usado_en: para el TTL de expiración (90 días sin uso → basura).
CREATE TABLE IF NOT EXISTS `ai_op_template` (
  `id`            INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `company_id`    INT NOT NULL DEFAULT 0,
  `agent_id`      INT NOT NULL DEFAULT 0,
  `operation`     VARCHAR(64) NOT NULL,
  `template_key`  VARCHAR(191) NOT NULL,
  `pattern`       MEDIUMTEXT NOT NULL,              -- JSON: parte FIJA que la IA decidió
  `var_schema`    MEDIUMTEXT NOT NULL DEFAULT '[]', -- JSON: campos que VARÍAN
  `usos`          INT NOT NULL DEFAULT 0,
  `needs_human`   TINYINT NOT NULL DEFAULT 1,
  `creado_en`     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `last_usado_en` DATETIME NULL,
  UNIQUE KEY `idx_op_template_key` (`agent_id`, `operation`, `template_key`),
  KEY `idx_op_template_lru` (`agent_id`, `last_usado_en`),
  KEY `idx_op_template_expira` (`last_usado_en`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tarea periódica de limpieza (basura = plantillas sin uso en 90 días):
--   DELETE FROM ai_op_template WHERE last_usado_en < NOW() - INTERVAL 90 DAY;
--   (y, por agente, mantener ≤ 50: DELETE fuera de los 50 más recientes).
