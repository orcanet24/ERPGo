-- ---------------------------------------------------------------------------
-- El canal de aviso de WhatsApp por GOWA apunta a un DISPOSITIVO, no a una URL.
--
-- Los canales `sms`, `wapp` y `webhook` guardan en `url` la pasarela a la que se
-- entrega el mensaje. El canal `gowa` no entrega a una URL: escribe en la cola
-- del módulo de mensajería, y lo que necesita saber es QUÉ NÚMERO lo manda.
-- Guardar el identificador en `url` habría funcionado, pero deja un campo
-- llamado «url» con un valor que no es una URL, y el siguiente que lo lea
-- perderá media hora.
--
-- Va en archivo aparte porque `notify_channel` existe desde la migración de
-- alertas financieras y ya puede estar creada. AplicarEsquema tolera la columna
-- duplicada (1060).
-- ---------------------------------------------------------------------------

ALTER TABLE `notify_channel` ADD COLUMN `msg_device_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '';
