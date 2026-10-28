-- schema_z_compras_inbox.sql — FASE C: recepción de facturas de proveedor.
--
-- Qué es: la BANDEJA donde aterriza un archivo (XML fiscal, PDF o foto) que un
-- proveedor envió o que alguien arrastró al sistema. Es la parte de recepción
-- de la automatización de compras: el documento se guarda, se lee lo que se
-- pueda, se propone en el formulario de compra y se deja TRAZADO de quién lo
-- cargó y cuándo.
--
-- Lo que NO es: un alta automática de compras. Aquí NO se escriben compras, ni
-- asientos, ni movimientos de stock: la línea del humo, que se proclama en la
-- cabecera de interfaces/web/purchase_inbox.go, es que la extracción propone y
-- la persona confirma. El candado anti-duplicado de `purchase_hash` corre al
-- guardar sobre la compra ya nacida, igual que si la cargó un humano a mano.
--
-- Por qué una tabla nueva y no columnas en purchase: una compra puede recibir
-- varios archivos (el PDF del proveedor, la foto de la nota, la corrección del
-- número de control) y esos archivos siguen existiendo aunque la compra se
-- cancele. El documento es a la compra lo que la factura a cobrar es al cobro:
-- un archivo con su propio ciclo de vida.

CREATE TABLE IF NOT EXISTS purchase_inbox (
    -- Id propio: dos cargas del mismo proveedor pueden ser archivos distintos
    -- y cada una queda como registro, aunque los dos digan «factura 123».
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    -- La empresa. Cada tenant solo ve lo SUYO en su directorio, igual que
    -- /uploads: no existe API que liste archivos de otras empresas.
    tenant_id           VARCHAR(64) NOT NULL,
    -- Quién lo recibió. La traza del dueño es parte del pedido de orden.
    -- VARCHAR y no BIGINT: aquí el id de usuario es textual (el de la sesión,
    -- igual que user_id de audit_log), y una columna numérica exigiría
    -- convertir algo que no tiene por qué ser un número.
    user_id             VARCHAR(64) NOT NULL DEFAULT '',
    -- El archivo tal como vino.
    filename            VARCHAR(255) NOT NULL,
    orig_name           VARCHAR(255) NOT NULL,
    mime                VARCHAR(120) NOT NULL DEFAULT '',
    bytes               BIGINT NOT NULL DEFAULT 0,
    -- xml | pdf | imagen.
    formato             VARCHAR(16) NOT NULL DEFAULT '',
    -- xml (se leyó con el traductor) | pendiente_ocr (PDF o foto, la Fase D).
    extraccion          VARCHAR(24) NOT NULL DEFAULT 'pendiente_ocr',
    -- factura | nota_entrega | orden: la clasificación pedida para el candado
    -- y para saber qué efectos fiscales carga cada documento.
    doc_kind            VARCHAR(20) NOT NULL DEFAULT 'factura',
    -- Lo propuesto a partir del archivo: es una PROPUESTA, nunca un alta.
    rif                 VARCHAR(40) NOT NULL DEFAULT '',
    proveedor           VARCHAR(200) NOT NULL DEFAULT '',
    numero              VARCHAR(40) NOT NULL DEFAULT '',
    control             VARCHAR(40) NOT NULL DEFAULT '',
    fecha               VARCHAR(32) NOT NULL DEFAULT '',
    moneda              VARCHAR(12) NOT NULL DEFAULT '',
    total               DECIMAL(18,4) NOT NULL DEFAULT 0,
    iva                 DECIMAL(18,4) NOT NULL DEFAULT 0,
    -- JSON: [{descripcion, cantidad, precio, total}, ...].
    lineas              LONGTEXT NULL,
    -- Campos que SÍ se leyeron, para pintar la lista sin reparsear.
    leidos              VARCHAR(600) NOT NULL DEFAULT '',
    -- Campo abierto: qué quedó por resolver («sin número de control»).
    notas               VARCHAR(500) NOT NULL DEFAULT '',
    -- Leído | cargado en compra | descartado. La bandeja la escribe; las
    -- compras la marcan al guardarse (purchase_inbox.go: setEstadoInbox).
    estado              VARCHAR(20) NOT NULL DEFAULT 'leido',
    -- purchase.id si ya se cargó: la traza del documento en su compra.
    linked_purchase_id  BIGINT NULL,
    linked_at           DATETIME NULL,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    -- El índice del candado: encuentra el documento de ESTA empresa en un toque.
    KEY idx_inbox_tenant (tenant_id, created_at),
    KEY idx_inbox_estado (tenant_id, estado),
    KEY idx_inbox_link (linked_purchase_id),
    -- SHA-256 del contenido: el mismo archivo, aunque lo llamen distinto.
    sha256              CHAR(64) NOT NULL,
    -- Dentro de una empresa, dos personas pueden subir el mismo archivo (el
    -- de la mañana y el de la tarde); ese duplicado se detecta aquí y se
    -- responde «ya lo recibimos» en vez de dejarlo flotando. En SQLite el
    -- motor crea el índice de la misma forma; el INSERT es nombrado (INSERT
    -- INTO ... (cols) VALUES), así que esa es su forma general en MySQL,
    -- PostgreSQL, MariaDB y SQLite, y la restricción aplica en todas.
    UNIQUE KEY uq_inbox_hash (tenant_id, sha256)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
