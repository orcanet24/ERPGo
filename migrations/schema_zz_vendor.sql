-- ---------------------------------------------------------------------------
-- Columna `vendor` de ai_providers (whitelist compilada en el binario).
--
-- El nombre "schema_z2_vendor.sql" ordena DESPUÉS de
-- "schema_z_ai_providers.sql" (que crea la tabla), por lo que el ALTER
-- encuentra la tabla tanto en MySQL como en el SQLite convertido.
--
-- Valores: 'openai' | 'anthropic' | 'orc24'. La URL base NO se guarda aquí:
-- el binario la resuelve siempre desde aiVendorBaseURL() (interfaces/web/
-- ai_rules.go); una base_url ajena en la BD se ignora por completo.
-- ---------------------------------------------------------------------------

ALTER TABLE `ai_providers` ADD COLUMN IF NOT EXISTS `vendor` varchar(20) NOT NULL DEFAULT 'openai';
