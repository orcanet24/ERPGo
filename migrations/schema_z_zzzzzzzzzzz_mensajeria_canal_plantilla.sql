-- ---------------------------------------------------------------------------
-- Un canal GOWA puede usar una plantilla en vez de asunto + cuerpo libre.
--
-- POR QUÉ. El mismo aviso de cobranza se manda cientos de veces; escribirlo en
-- cada llamada hace que cada uno diga una cosa distinta. La plantilla fija el
-- texto y solo cambian los datos. Si la plantilla usa una variable que el aviso
-- no trae, el envío falla con el motivo en vez de salir con un hueco.
--
-- Va en archivo aparte porque notify_channel ya existe. AplicarEsquema tolera la
-- columna duplicada (1060).
-- ---------------------------------------------------------------------------

ALTER TABLE `notify_channel` ADD COLUMN `msg_template_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '';
