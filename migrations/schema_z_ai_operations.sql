-- ---------------------------------------------------------------------------
-- AI Operations + Cache
--
-- El modelo de negocio: vender operaciones de IA, no tokens.
-- - Cada operación se cuenta contra el plan de la empresa.
-- - Los cache HIT NO consumen cuota (igual que LongCat).
-- - Rate limiting por plan: si excede, la IA responde sin llamar al proveedor.
-- ---------------------------------------------------------------------------

-- Tabla de planes por empresa (1 = free, 2 = starter $25, 3 = pro $100)
ALTER TABLE `company_information` ADD COLUMN IF NOT EXISTS `ai_plan` TINYINT NOT NULL DEFAULT 1;
ALTER TABLE `company_information` ADD COLUMN IF NOT EXISTS `ai_plan_ops_monthly` INT NOT NULL DEFAULT 50;
ALTER TABLE `company_information` ADD COLUMN IF NOT EXISTS `ai_plan_ops_daily` INT NOT NULL DEFAULT 20;

-- Cache de prompts: respuestas cacheadas por tipo + hash
CREATE TABLE IF NOT EXISTS `ai_cache` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `company_id` INTEGER NOT NULL DEFAULT 0,
  `cache_key` VARCHAR(191) NOT NULL,        -- hash del prompt + params
  `operation_type` VARCHAR(64) NOT NULL,     -- 'informe', 'agente', 'chat', etc.
  `response_json` LONGTEXT NOT NULL,         -- respuesta completa (JSON)
  `hit_count` INT NOT NULL DEFAULT 0,        -- cuántas veces se reutilizó
  `expires_at` DATETIME NOT NULL,            -- TTL del cache
  `created_at` DATETIME NOT NULL,
  `last_hit_at` DATETIME NULL,
  UNIQUE KEY `idx_ai_cache_key` (`company_id`, `cache_key`),
  KEY `idx_ai_cache_expires` (`expires_at`),
  KEY `idx_ai_cache_type` (`company_id`, `operation_type`)
);

-- Contador de operaciones (facturación)
CREATE TABLE IF NOT EXISTS `ai_operations` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `company_id` INTEGER NOT NULL DEFAULT 0,
  `operation_type` VARCHAR(64) NOT NULL,
  `was_cache_hit` TINYINT NOT NULL DEFAULT 0,
  `ops_consumed` INT NOT NULL DEFAULT 1,     -- cuántas ops de plan consumió
  `prompt_tokens` INT NOT NULL DEFAULT 0,    -- para métricas
  `completion_tokens` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  KEY `idx_aiops_company_day` (`company_id`, `created_at`),
  KEY `idx_aiops_month` (`company_id`, `operation_type`, `created_at`)
);

-- Limpiar cache viejo (tarea periópica)
-- DELETE FROM ai_cache WHERE expires_at < NOW() - INTERVAL 7 DAY;
