-- ---------------------------------------------------------------------------
-- El secreto del webhook vive POR DISPOSITIVO, no por instalación.
--
-- POR QUÉ. El webhook resuelve «dispositivo → empresa» sin sesión. Si el
-- secreto fuera uno solo, filtrar el de un número daría permiso para inyectar
-- mensajes en todos los demás. Con uno por dispositivo, el daño queda acotado a
-- ese número y rotarlo no obliga a reconfigurar los otros.
--
-- Va en un archivo aparte porque `msg_device` pudo crearse ya en una instalación
-- que arrancó entre las dos versiones. `AplicarEsquema` tolera la columna
-- duplicada (1060), así que en una base nueva esta sentencia no hace daño.
-- ---------------------------------------------------------------------------

ALTER TABLE `msg_device` ADD COLUMN `webhook_secret_enc` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '';
