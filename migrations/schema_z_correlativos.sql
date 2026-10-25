-- Correlativos con reserva bloqueada, idempotencia de ventas y unicidad fiscal
-- (hallazgos F1 y F5 del informe del 15-10).
--
-- POR QUÉ UNA TABLA DE CONTADORES Y NO `MAX(columna)+1`.
--
-- Un máximo no reserva nada: dos transacciones simultáneas leen el mismo valor
-- y las dos se quedan con el mismo número. El SELECT no bloquea y la transacción
-- por sí sola no serializa dos lecturas. Aplica igual al número de factura y al
-- número de control fiscal: dos cajas a la vez podían emitir el MISMO documento
-- fiscal, que en Venezuela lo invalida y descuadra el libro de ventas.
--
-- Aquí el número sale de UNA fila, y el UPDATE se queda con su bloqueo
-- exclusivo hasta que la transacción termina: la segunda venta espera y recibe
-- el siguiente. `value` se siembra por debajo del primer número válido y cada
-- reserva lo pone al día con el máximo ya emitido (lo hace el código, que sí
-- puede comparar), así que una base con histórico heredado no numera por debajo
-- de sus facturas.
CREATE TABLE IF NOT EXISTS pos_counters (
  name VARCHAR(64) NOT NULL,
  value BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NULL,
  PRIMARY KEY (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- La fila va DESPUÉS de su CREATE: el aplicador ejecuta el archivo en orden y un
-- INSERT delante de la tabla que crea falla con "table doesn't exist".
INSERT IGNORE INTO pos_counters (name, value) VALUES ('invoice', 999);

-- Idempotencia de ventas del POS (F5).
--
-- El terminal reintenta una venta cuando pierde el ACK: la venta ya quedó
-- confirmada en el servidor y el POS no lo sabe. Sin una clave única por
-- (terminal, venta local), el reintento emite OTRA factura y consume otro número
-- de control fiscal.
--
-- `invoice.offline_invoice_no` no sirve como clave: es BIGINT y el id que manda
-- el terminal es texto ("local-42"), así que la parte no numérica se perdería y
-- dos ventas distintas podrían compartir el mismo número.
--
-- La UNIQUE es la que decide quién gana: dos reintentos simultáneos intentan el
-- INSERT, solo uno entra y el otro lee la fila del ganador y devuelve el MISMO
-- documento.
CREATE TABLE IF NOT EXISTS pos_sale_idempotency (
  id INT NOT NULL AUTO_INCREMENT,
  terminal_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'Caja que emitió la venta',
  offline_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'Id local de la venta en el terminal',
  invoice_no BIGINT NOT NULL DEFAULT 0 COMMENT 'Número de factura asignado',
  control_no VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'Número de control fiscal asignado',
  created_at DATETIME NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_pos_sale_offline (terminal_id, offline_id),
  KEY idx_pos_sale_invoice (invoice_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Unicidad fiscal de la factura (F1).
--
-- El esquema heredado tenía `KEY` sobre `invoice` y sobre `control_no`: un
-- índice que acelera la búsqueda y no impide repetir el número. Estas dos
-- UNIQUE son la última línea de defensa: aunque una carrera de numeración
-- volviera a colarse, la base rechaza el segundo documento en vez de emitirlo.
--
-- SI FALLAN, HAY QUE MIRARLOS. Un fallo aquí significa que la base YA tiene
-- números repetidos (el defecto que se está corrigiendo), y el aplicador lo
-- informa con "migración: Error 1062 ... <- ALTER TABLE" y el resumen de
-- advertencias del arranque. No se aplican en silencio: mientras existan
-- duplicados, el índice no se crea, y eso es un dato que hay que resolver, no
-- algo que deba pasar desapercibido. Para localizarlos:
--
--   SELECT invoice, COUNT(*) c FROM invoice GROUP BY invoice HAVING c > 1;
--   SELECT series, control_no, COUNT(*) c FROM invoice
--     WHERE control_no IS NOT NULL AND control_no <> ''
--     GROUP BY series, control_no HAVING c > 1;
--
-- Los documentos con `control_no` NULL (histórico anterior a la serie fiscal)
-- no se estorban entre sí: en MySQL varias filas NULL no chocan en una UNIQUE.
ALTER TABLE invoice ADD UNIQUE KEY uq_invoice_number (invoice);
ALTER TABLE invoice ADD UNIQUE KEY uq_invoice_series_control (series, control_no);
