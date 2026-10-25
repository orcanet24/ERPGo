-- Foto de usuario.
--
-- La foto de cada usuario se guarda en uploads/EMP{8hex}/personal/{32hex}{ext}
-- (protegido, solo master/admin) y la ruta relativa se almacena en users.photo.
-- Se usa la columna con ALTER ADD COLUMN IF NOT EXISTS para que sea idempotente
-- sobre bases ya existentes (no falla si la migración se aplica dos veces).
ALTER TABLE `users` ADD COLUMN IF NOT EXISTS `photo` varchar(255) DEFAULT NULL;
