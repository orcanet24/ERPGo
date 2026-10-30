-- ---------------------------------------------------------------------------
-- La entrada «Mensajería» de Configuración se oculta cuando el módulo está
-- apagado.
--
-- Antes quedaba siempre visible para que el master pudiera encenderla, pero eso
-- dejaba un menú que llevaba a una pantalla de un módulo inexistente: el usuario
-- veía «Mensajería» en Configuración y al entrar solo encontraba interruptores
-- de algo que no estaba activo. Ahora el camino es Configuración → Módulos →
-- Configurar, que está siempre disponible para el master.
--
-- Es un UPDATE y no un INSERT: la fila 284 ya la crea la migración del módulo en
-- las instalaciones nuevas, y en las que ya arrancaron está con module_id
-- 'core'. El WHERE por URL evita depender del id.
-- ---------------------------------------------------------------------------

UPDATE `menu` SET `module_id` = 'mensajeria' WHERE `url` = '/settings/mensajeria';
