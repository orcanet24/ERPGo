-- schema.sql del módulo Memo, fase A (docs/PLAN-MEMO.md).
--
-- EL CONTRATO: la frontera SOLO aplica sentencias CREATE TABLE IF NOT EXISTS
-- de tablas declaradas en tables_own del manifiesto. Ni DROP, ni ALTER, ni
-- INSERT de siembra. Las fases siguientes añaden sus tablas con CREATE TABLE
-- IF NOT EXISTS (idempotente), sin reescribir las que ya existen.
--
-- REGLA DE Aislamiento: cada fila lleva `empresa`. El módulo atiende a varias
-- compañías con la misma tabla en cada base, y una consulta que olvida
-- `empresa` es una filtración de una empresa en otra.
--
-- DDL portable MySQL/SQLite: id VARCHAR como clave primaria (lo genera el
-- handler de negocio con randomHex y el orquestador lo respeta).
CREATE TABLE IF NOT EXISTS memo_notas (
    id           VARCHAR(64) NOT NULL PRIMARY KEY,
    empresa      VARCHAR(64) NOT NULL DEFAULT '',
    autor        VARCHAR(64) NOT NULL DEFAULT '',
    titulo       VARCHAR(200) NOT NULL,
    texto        TEXT NOT NULL,
    visibilidad  VARCHAR(20) NOT NULL DEFAULT 'empresa',
    tipo         VARCHAR(20) NOT NULL DEFAULT 'nota',
    estado       VARCHAR(20) NOT NULL DEFAULT 'activo',
    fijada       INTEGER NOT NULL DEFAULT 0,
    creada       VARCHAR(32) NOT NULL DEFAULT '',
    actualizada  VARCHAR(32) NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS memo_destinatarios (
    id           VARCHAR(64) NOT NULL PRIMARY KEY,
    nota_id      VARCHAR(64) NOT NULL,
    destino_tipo VARCHAR(20) NOT NULL DEFAULT 'persona',
    destino_id   VARCHAR(64) NOT NULL,
    empresa      VARCHAR(64) NOT NULL DEFAULT ''
);
