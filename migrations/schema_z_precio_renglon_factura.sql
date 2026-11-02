-- Listas de precios: soporte REAL del esquema para la facturación por listas.
--
-- POR QUÉ ESTA MIGRACIÓN EXISTE.
--
-- El motor de precios (interfaces/web/price_list.go) y el guardado de facturas
-- (interfaces/web/invoice.go) y el checkout del POS (interfaces/web/pos_api.go)
-- escriben y leen `invoice_details.price_list_id`, `discount_pct`,
-- `discount_amount` y `base_price`, y operan la tabla `product_price_discount`.
-- Ninguno de los dos crecía en ningún archivo de migraciones: la instalación
-- nueva de MySQL nacía sin ellas, y la pantalla de facturas moría al guardar con
-- «Error 1054: Unknown column 'price_list_id' in 'field list'». Descubierto en
-- vivo el 2026-11-02 facturando desde /invoice/new sobre MySQL 8.0.30.
--
-- Las columnas de `invoice_details` van como ALTER simple (sin IF NOT EXISTS):
-- los instaladores viejos ya las tienen si aplicaron este archivo una vez, y el
-- aplicador tolera el error 1060 («columna duplicada») con un aviso contado —
-- igual que hace con los índices de schema_z_correlativos.sql.
ALTER TABLE invoice_details ADD COLUMN price_list_id int NOT NULL DEFAULT 1 COMMENT 'Lista de precio aplicada al renglón';
ALTER TABLE invoice_details ADD COLUMN discount_pct decimal(5,2) DEFAULT '0.00' COMMENT 'Descuento porcentual del renglón';
ALTER TABLE invoice_details ADD COLUMN discount_amount decimal(10,2) DEFAULT '0.00' COMMENT 'Descuento en valor del renglón';
ALTER TABLE invoice_details ADD COLUMN base_price decimal(12,2) DEFAULT NULL COMMENT 'Precio base antes de descuentos';

-- La tabla de descuentos дискriminados por cliente/lista, tal como la escriben
-- los handlers: INSERT con NOW() en created_at y DELETE por id.
CREATE TABLE IF NOT EXISTS product_price_discount (
  id INT NOT NULL AUTO_INCREMENT,
  product_id VARCHAR(100) NOT NULL COMMENT 'Producto descontado',
  customer_id BIGINT NULL COMMENT 'Cliente específico (NULL = clase de cliente)',
  customer_class VARCHAR(50) NULL COMMENT 'Clase de cliente cuando no va por cliente',
  price_list_id INT NULL COMMENT 'Lista sobre la que aplica el descuento',
  discount_pct DECIMAL(5,2) NOT NULL DEFAULT 0 COMMENT 'Descuento en porcentaje',
  min_qty DECIMAL(12,3) NOT NULL DEFAULT 0 COMMENT 'Cantidad mínima para aplicar',
  valid_from VARCHAR(10) NULL COMMENT 'Vigencia desde (YYYY-MM-DD)',
  valid_until VARCHAR(10) NULL COMMENT 'Vigencia hasta (YYYY-MM-DD)',
  reason VARCHAR(255) NULL COMMENT 'Motivo documentado del descuento',
  status VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT 'active / inactive',
  created_at DATETIME NULL,
  PRIMARY KEY (id),
  KEY idx_ppd_product (product_id),
  KEY idx_ppd_customer (customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
