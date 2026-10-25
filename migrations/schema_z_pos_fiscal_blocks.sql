-- Bloques de numeración fiscal offline por caja
-- Cada caja recibe bloques exclusivos de números fiscales del servidor.
-- Esto evita duplicados cuando múltiples cajas operan offline simultáneamente.

CREATE TABLE IF NOT EXISTS pos_fiscal_blocks (
  id INT AUTO_INCREMENT PRIMARY KEY,
  terminal_id VARCHAR(64) NOT NULL,
  branch_id INT NOT NULL DEFAULT 0,
  series VARCHAR(10) NOT NULL DEFAULT 'FACT',
  block_start BIGINT NOT NULL,
  block_end BIGINT NOT NULL,
  block_size INT NOT NULL,
  used_count INT NOT NULL DEFAULT 0,
  last_used BIGINT NOT NULL DEFAULT 0,
  status ENUM('active','exhausted','revoked') NOT NULL DEFAULT 'active',
  reserved_at DATETIME NOT NULL,
  expires_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_terminal_series (terminal_id, series, block_start),
  INDEX idx_terminal_status (terminal_id, series, status),
  INDEX idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- Tabla de auditoría de uso de bloques: registra cada número fiscal consumido offline
CREATE TABLE IF NOT EXISTS pos_fiscal_block_usage (
  id INT AUTO_INCREMENT PRIMARY KEY,
  block_id INT NOT NULL,
  terminal_id VARCHAR(64) NOT NULL,
  fiscal_number BIGINT NOT NULL,
  invoice_id BIGINT NULL,
  sale_local_id INT NULL,
  used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  synced TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE KEY uk_fiscal_number (fiscal_number),
  INDEX idx_block (block_id),
  INDEX idx_terminal (terminal_id),
  CONSTRAINT fk_block FOREIGN KEY (block_id) REFERENCES pos_fiscal_blocks(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
