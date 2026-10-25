-- Migración: Tablas de inventario y flujo de almacén
-- Idempotente: CREATE IF NOT EXISTS, sin DROP

CREATE TABLE IF NOT EXISTS stock_movement (
  id INT NOT NULL AUTO_INCREMENT,
  tenant_id VARCHAR(30) NOT NULL DEFAULT 'default',
  movement_type ENUM('purchase','sale_return','adjustment_add','sale','purchase_return','adjustment_remove','delivery') NOT NULL,
  product_id INT NOT NULL,
  warehouse_id VARCHAR(30) DEFAULT 'WH-MAIN',
  quantity DECIMAL(18,2) NOT NULL,
  unit_cost DECIMAL(18,4) DEFAULT NULL,
  reference_type ENUM('invoice','purchase','delivery_order','adjustment','return') NOT NULL,
  reference_id INT NOT NULL,
  reference_no VARCHAR(50) DEFAULT NULL,
  date DATE NOT NULL,
  is_posted TINYINT NOT NULL DEFAULT 1,
  posted_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_sm_tenant_product_date (tenant_id, product_id, date),
  KEY idx_sm_reference (reference_type, reference_id),
  KEY idx_sm_type (movement_type)
);

CREATE TABLE IF NOT EXISTS delivery_order (
  id INT NOT NULL AUTO_INCREMENT,
  tenant_id VARCHAR(30) NOT NULL DEFAULT 'default',
  invoice_id INT DEFAULT NULL,
  customer_id INT NOT NULL,
  warehouse_id VARCHAR(30) DEFAULT 'WH-MAIN',
  status ENUM('draft','confirmed','picking','packed','shipped','delivered','cancelled') DEFAULT 'draft',
  date DATE NOT NULL,
  delivered_at DATETIME DEFAULT NULL,
  notes TEXT,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_do_tenant_status (tenant_id, status),
  KEY idx_do_tenant_invoice (tenant_id, invoice_id)
);

CREATE TABLE IF NOT EXISTS delivery_order_detail (
  id INT NOT NULL AUTO_INCREMENT,
  tenant_id VARCHAR(30) NOT NULL DEFAULT 'default',
  delivery_order_id INT NOT NULL,
  product_id INT NOT NULL,
  quantity DECIMAL(18,2) NOT NULL,
  picked_quantity DECIMAL(18,2) DEFAULT 0,
  packed_quantity DECIMAL(18,2) DEFAULT 0,
  shipped_quantity DECIMAL(18,2) DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uq_dod (delivery_order_id, product_id)
);

-- Configuración por defecto: modo invoice (compatible con comportamiento actual)
INSERT INTO fiscal_setting (`key`, value) VALUES ('inventory_deduction_mode', 'invoice')
  ON DUPLICATE KEY UPDATE value = VALUES(value);
