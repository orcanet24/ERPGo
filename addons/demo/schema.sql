-- schema.sql del módulo demo (fase 2 de docs/PLAN-ADDONS.md).
--
-- EL CONTRATO: la frontera SOLO aplica sentencias CREATE TABLE IF NOT EXISTS
-- de tablas declaradas en tables_own del manifiesto. Ni DROP, ni ALTER, ni
-- INSERT de siembra: el esquema ajeno no lo toca nadie.
--
-- demo_avisos va además en db_read —leyendo la suya, el módulo pinta lo que
-- él mismo escribió por el canal—. Eso no le amplía el poder: el grant SELECT
-- solo cubre lo que ya es suyo.
--
-- DDL portable MySQL/SQLite: id VARCHAR como clave primaria (lo genera el
-- handler de negocio con randomHex y el orquestador lo respeta), sin
-- AUTO_INCREMENT/AUTOINCREMENT.
CREATE TABLE IF NOT EXISTS demo_avisos (
    id      VARCHAR(64) NOT NULL PRIMARY KEY,
    titulo  VARCHAR(160) NOT NULL,
    mensaje TEXT NOT NULL,
    autor   VARCHAR(64) NOT NULL DEFAULT '',
    empresa VARCHAR(64) NOT NULL DEFAULT '',
    creada  VARCHAR(32) NOT NULL DEFAULT ''
);
