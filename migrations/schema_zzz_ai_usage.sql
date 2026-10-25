-- ---------------------------------------------------------------------------
-- 1) Retiro de `anthropic` de la whitelist de vendors.
--
-- Motivo: el cliente HTTP del proyecto es 100% OpenAI-compatible
-- (POST /chat/completions + header Authorization: Bearer). La API real de
-- Anthropic exige POST /v1/messages con headers x-api-key + anthropic-version
-- y devuelve `content[]` en lugar de `choices[]`: con el código actual ese
-- proveedor devolvía 401/404 garantizado. Se retira en vez de anunciarlo como
-- opción, porque ofrecer un vendor que no funciona es peor que no ofrecerlo.
--
-- Las filas existentes se migran a ORC24 (también OpenAI-compatible) para no
-- dejar agentes apuntando a un vendor que ya no existe. NO se activa: cada
-- cliente configura su propia clave desde la UI.
--
-- Orden: "schema_zz_vendor.sql" < "schema_zzz_ai_usage.sql" ('_' 0x5F < 'z'
-- 0x7A), así que la columna `vendor` ya existe cuando este UPDATE corre.
-- ---------------------------------------------------------------------------

UPDATE `ai_providers` SET `vendor` = 'orc24' WHERE IFNULL(`vendor`, '') = 'anthropic';

-- ---------------------------------------------------------------------------
-- 2) ai_usage — metering de inferencia.
--
-- Sin esto es imposible vender "paquetes de inferencia": el cliente necesita un
-- número real de lo que consume. Cada llamada al proveedor (informe o agente)
-- deja una fila con tokens y peticiones, atribuida a la empresa, al agente y
-- al proveedor, para poder facturar por periodo y desglosar por cliente.
--
-- NO guarda prompts ni respuestas: solo contadores. Es un dato de facturación,
-- no un archivo de conversaciones.
--
-- `estimated_cost_usd` se deja a 0 cuando el vendor no tiene tarifa conocida
-- (la calcula ORC24, no OpenAI). `company_id` sigue el multitenant: 0 = empresa
-- por defecto.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `ai_usage` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `company_id` INTEGER NOT NULL DEFAULT 0,
  `provider_id` INTEGER NOT NULL DEFAULT 0,
  `vendor` VARCHAR(191) NOT NULL DEFAULT '',
  `model` VARCHAR(191) NOT NULL DEFAULT '',
  `agent_id` INTEGER NOT NULL DEFAULT 0,
  `agent_name` VARCHAR(191) NOT NULL DEFAULT '',
  `source` VARCHAR(191) NOT NULL DEFAULT '',
  `prompt_tokens` INTEGER NOT NULL DEFAULT 0,
  `completion_tokens` INTEGER NOT NULL DEFAULT 0,
  `total_tokens` INTEGER NOT NULL DEFAULT 0,
  `requests` INTEGER NOT NULL DEFAULT 1,
  `estimated_cost_usd` REAL NOT NULL DEFAULT 0,
  `ok` INTEGER NOT NULL DEFAULT 1,
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  `created_day` VARCHAR(32) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS `idx_ai_usage_day` ON `ai_usage` (`created_day`);
CREATE INDEX IF NOT EXISTS `idx_ai_usage_company` ON `ai_usage` (`company_id`);
CREATE INDEX IF NOT EXISTS `idx_ai_usage_provider` ON `ai_usage` (`provider_id`);