-- ---------------------------------------------------------------------------
-- Tipo de media en la cola de salida.
--
-- `media_path` ya existía, pero sin el tipo el despachador no sabe si llamar a
-- /send/image, /send/audio, /send/video o /send/file. Guardarlo en la fila (y
-- no adivinarlo por la extensión al enviar) evita que un archivo renombrado
-- salga por el método equivocado.
--
-- Va en archivo aparte porque msg_outbox pudo crearse en una instalación que ya
-- arrancó. AplicarEsquema tolera la columna duplicada (1060).
-- ---------------------------------------------------------------------------

ALTER TABLE `msg_outbox` ADD COLUMN `media_type` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '';
