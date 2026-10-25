-- ---------------------------------------------------------------------------
-- Agentes IA por departamento, con su prompt de configuración.
--
-- POR QUÉ ESTE FICHERO.
--
-- El agente de IA tenía un rol de seguridad (`role_id`) pero ningún
-- departamento. El departamento es lo que decide qué tablas puede LEER y sobre
-- las que puede escribir una consulta (ver ai_schema_introspect.go y
-- ai_sql_scope.go): un agente de cuentas por cobrar ve la cartera y no ve la
-- nómina.
--
-- Sin columna, el departamento se deducía del NOMBRE del rol con una tabla
-- escrita en el código. Funcionaba, pero era frágil por dos motivos:
--
--   1. El rol se llama como quiera la empresa ("Analista de cobro", "Agente
--      CxC"), no como el código. Un rol con nombre propio caía siempre en IT, que
--      es el departamento que ve TODO: un agente de cobranza terminaba con
--      acceso al plan de cuentas y a los salarios.
--   2. No había forma de guardar el comportamiento del agente. El prompt de
--      sistema lo armaba el producto entero y no había manera de decir "este
--      agente agrupa siempre por cliente y no promete fechas de cobro" sin tocar
--      el binario.
--
-- `config_prompt` es ESO: el prompt que escribe el administrador al crear el
-- agente, que se antepone al prompt del departamento y al del rol. No sustituye
-- a las reglas ocultas (esas son compiladas y van siempre primero, ver
-- ai_rules.go): es la instrucción de comportamiento, que sí es editable por
-- quien administra.
--
-- Orden: este fichero va DESPUÉS de schema_z_ai_agents.sql (que crea ai_agent).
-- El prefijo `zzzz` lo deja al final de la ordenación por nombre, que es donde
-- tiene que estar.
--
-- Idempotente: se puede aplicar sobre bases ya migradas.
-- ---------------------------------------------------------------------------

-- 1. Departamento del agente (it | fac | cxc | cxp | contb | rrhh | inv).
--
--    El default es VACÍO y no 'it' a propósito. Vacío significa "deducir del
--    nombre del rol", que es lo que corresponde a los agentes que ya existían
--    en la base: su rol dice su departamento y así siguen viendo lo que veían.
--    Si el default fuera 'it', un agente de cobranza ya creado se convertiría
--    en un agente con acceso a TODO en la primera migración, sin que nadie
--    hubiera tocado nada. Y además no se podría distinguir "soporte" de
--    "nunca configurado".
ALTER TABLE `ai_agent` ADD COLUMN IF NOT EXISTS `department` VARCHAR(10) NOT NULL DEFAULT '';

-- 2. Prompt de configuración del agente, escrito por el administrador.
ALTER TABLE `ai_agent` ADD COLUMN IF NOT EXISTS `config_prompt` MEDIUMTEXT NULL;

-- 3. Fecha de última edición del prompt, para saber quién lo tocó y cuándo.
ALTER TABLE `ai_agent` ADD COLUMN IF NOT EXISTS `config_prompt_at` VARCHAR(32) NOT NULL DEFAULT '';

-- 4. Índice: la pantalla de agentes filtra por departamento.
CREATE INDEX IF NOT EXISTS `idx_agent_department` ON `ai_agent` (`department`);

-- ---------------------------------------------------------------------------
-- Prompt por defecto de cada departamento.
--
-- Son prompts de COMPORTAMIENTO, no de sistema: no llevan las reglas de
-- seguridad (esas no son editables) sino el modo de trabajar del área. Van
-- sembrados para que al crear un agente el administrador tenga un punto de
-- partida en vez de una caja en blanco, que es como se escriben prompts malos.
--
-- Tabla propia y no `ai_setting` porque esa NO es una tabla de ajustes
-- genéricos: es la configuración del proveedor de IA (api_key, model,
-- temperature). Met departamento prompts ahí sería mezclar dos cosas que
-- no tienen nada que ver, y `ai_setting` no tiene ni `group_key` ni `value`
-- libre donde guardarlas.
--
-- `WHERE NOT EXISTS` evita el problema de `INSERT IGNORE` sobre algo que un
-- cliente ya editó: si el administrador cambió el prompt de CXC, una migración
-- posterior no debe pisárselo. Solo se siembra si la fila no existe.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `ai_department_prompt` (
  `department` VARCHAR(10) NOT NULL,
  `prompt` MEDIUMTEXT NOT NULL,
  `label` VARCHAR(150) NOT NULL DEFAULT '',
  `updated_by` VARCHAR(60) NOT NULL DEFAULT '',
  `updated_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`department`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'cxc', 'Cuentas por Cobrar',
'Tu trabajo es la cobranza. Al informar, agrupa siempre por cliente y muestra el SALDO PENDIENTE, no solo el total facturado. Cuando pidas datos, prefiere due_amount (lo que se debe) sobre total_amount (lo que se facturó). No prometas fechas de cobro ni negocies plazos: eso lo decide una persona.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='cxc') `t`);

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'cxp', 'Cuentas por Pagar',
'Tu trabajo es el pago a proveedores. Al informar, separa lo VENCIDO de lo no vencido y muestra el número de factura del proveedor. Antes de proponer un pago, ten en cuenta el vencimiento real y el saldo pendiente, no solo el monto.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='cxp') `t`);

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'fac', 'Facturación',
'Tu trabajo es la facturación. Respeta la serie y el correlativo que usa la empresa. No modifiques el IVA calculado por tu cuenta: si el total no cuadra con los renglones, dilo en el informe en lugar de corregirlo.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='fac') `t`);

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'contb', 'Contabilidad',
'Tu trabajo es la contabilidad. Cada partida que propongas debe cuadrar: el total de débitos y el total de créditos del asiento son iguales. Cita siempre la cuenta (HeadCode) y su nombre. No cierres un período ni registre un asiento sin que una persona lo pida expresamente.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='contb') `t`);

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'rrhh', 'Recursos Humanos',
'Tu trabajo es el personal. Son datos reales de personas: muéstralos solo dentro de la empresa y no los reproduzcas en informes que se puedan compartir fuera. Todos los montos a dos decimales.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='rrhh') `t`);

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'inv', 'Inventario',
'Tu trabajo es el inventario. Al informar de existencias, ten en cuenta los lotes con fecha de caducidad: lo que vence primero sale primero (FEFO). Distingue siempre entre la cantidad física contada y la disponible.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='inv') `t`);

INSERT INTO `ai_department_prompt` (`department`, `label`, `prompt`) SELECT
'it', 'Soporte y Desarrollo',
'Eres de soporte y desarrollo: ves la base entera porque diseñas informes y diagnosticas. Cuando propongas un SQL, explica qué tabla va a consultar y por qué, para que quien lo lea pueda revisarlo antes de aprobarlo.'
WHERE NOT EXISTS (SELECT 1 FROM (SELECT `department` FROM `ai_department_prompt` WHERE `department`='it') `t`);