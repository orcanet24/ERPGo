-- schema.sql del módulo Memo, fases A y B (docs/PLAN-MEMO.md).
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
-- ── Fase B: grupos de trabajo y bandeja (docs/PLAN-MEMO.md §6) ──────────
--
-- memo_grupos + memo_grupo_miembros: el reparto por grupo. La pertenencia es
-- ESTADO ACTUAL —una fila por miembro— y no un historial: quien sale del grupo
-- se va de la tabla, y la regla de visibilidad decide en CADA lectura. Así
-- «dejar de ser miembro» quita la vista de inmediato, sin que ninguna nota
-- compartida tenga que rehacerse ni quede apuntando a nadie.
CREATE TABLE IF NOT EXISTS memo_grupos (
    id      VARCHAR(64) NOT NULL PRIMARY KEY,
    empresa VARCHAR(64) NOT NULL DEFAULT '',
    nombre  VARCHAR(120) NOT NULL,
    creada  VARCHAR(32) NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS memo_grupo_miembros (
    id         VARCHAR(64) NOT NULL PRIMARY KEY,
    grupo_id   VARCHAR(64) NOT NULL,
    persona_id VARCHAR(64) NOT NULL,
    empresa    VARCHAR(64) NOT NULL DEFAULT ''
);
-- memo_inbox: lo que se le avisa a cada destinatario al compartir (la fase F
-- la enseña con su contador de no leídas). Es DERIVADA: la verdad es la nota y
-- su reparto; esto es el aviso de que existen.
CREATE TABLE IF NOT EXISTS memo_inbox (
    id      VARCHAR(64) NOT NULL PRIMARY KEY,
    para    VARCHAR(64) NOT NULL,
    tipo    VARCHAR(30) NOT NULL DEFAULT 'compartida',
    ref_id  VARCHAR(64) NOT NULL DEFAULT '',
    texto   VARCHAR(300) NOT NULL DEFAULT '',
    leida   INTEGER NOT NULL DEFAULT 0,
    empresa VARCHAR(64) NOT NULL DEFAULT '',
    creada  VARCHAR(32) NOT NULL DEFAULT ''
);
