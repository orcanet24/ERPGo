-- ============================================================================
-- La cadena de auditoría con HMAC necesita 67 caracteres por huella
-- ============================================================================
-- F-10/S-10 (auditoría atómica 2026-11-22, probado en campo contra MySQL el
-- 2026-11-23): con MasterKey configurada la huella se escribe "h2:"+hex,
-- 67 caracteres que no cabían en varchar(64) —el UPDATE fallaba con Error
-- 1406 y, desde que el registro se escribe en UNA transacción, el auditoría
-- ENTERA se deshacía: los registros de sincronización y cierre no llegaban
-- al log. El ancho nuevo (128) cubre el formato presente y cualquier
-- futuro de 64 hex con prefijo de versión.
--
-- Una sentencia por cláusula (regla D-2): un MODIFY ya aplicado no puede
-- tapar al siguiente. Los MODIFY son ampliaciones de ancho, no recortes:
-- nunca pierden dato guardado.

ALTER TABLE `audit_log` MODIFY `record_hash` varchar(128) NOT NULL DEFAULT '';
ALTER TABLE `audit_log` MODIFY `prev_hash` varchar(128) NOT NULL DEFAULT '';
ALTER TABLE `audit_chain_blocks` MODIFY `start_hash` varchar(128) NOT NULL DEFAULT '';
ALTER TABLE `audit_chain_blocks` MODIFY `end_hash` varchar(128) NOT NULL DEFAULT '';
