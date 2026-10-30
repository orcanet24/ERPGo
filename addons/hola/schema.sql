-- schema.sql del addon demo hola (fase 2 de docs/PLAN-ADDONS.md).
--
-- EL CONTRATO: la frontera SOLO aplica sentencias CREATE TABLE IF NOT EXISTS
-- de tablas declaradas en tables_own del manifiesto. Ni DROP, ni ALTER, ni
-- INSERT de siembra: el esquema ajeno no lo toca nadie. Las filas de siembra
-- las crea el addon desde su código, por su tabla propia.
--
-- El DDL es portable MySQL/SQLite: id VARCHAR como clave primaria (el id lo
-- genera el handler de negocio con randomHex; el orquestador lo respeta),
-- sin AUTO_INCREMENT/AUTOINCREMENT.
CREATE TABLE IF NOT EXISTS hola_mensajes (
    id      VARCHAR(64) NOT NULL PRIMARY KEY,
    mensaje VARCHAR(512) NOT NULL,
    usuario VARCHAR(64) NOT NULL DEFAULT '',
    empresa VARCHAR(64) NOT NULL DEFAULT '',
    creada  VARCHAR(32) NOT NULL DEFAULT ''
);
