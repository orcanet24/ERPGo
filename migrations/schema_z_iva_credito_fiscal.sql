-- ============================================================================
-- IVA DEBITO FISCAL POR PAGAR — la cuenta de pasivo que faltaba
-- (hallazgo F-4, auditoría atómica 2026-11-22)
--
-- QUÉ ARREGLA. Las ventas acreditaban en Ventas el total con IVA incluido y
-- clasificaban el impuesto como gasto (4021101 "State Income Tax" con su
-- provisión 5020101, herencia del ISLR del PHP original). Resultado: el IVA
-- no existía como deuda con el SENIAT en ningún lado del balance —estaba
-- dentro del ingreso— y el libro de ventas (base = total − IVA) no cuadraba
-- con el mayor. Las COMPRAS ya habían recibido el tratamiento correcto
-- (purchase.go:419-460: base + IVA a cuenta propia, FAIL-CLOSED si la cuenta
-- no está asignada); esta migración da a la venta la misma frontera.
--
-- ids de acc_coa 900/901 y códigos 50207/5020701 son FIJOS y no se
-- reutilizan (política de orden en el tiempo). Bajo 502 "Current
-- Liabilities", misma rama que las provisiones heredadas, para no mover el
-- árbol existente.
--
-- Idempotencia: los INSERT chocan con 1062 en la segunda ejecución, que el
-- runner tolera como "ya estaba"; el UPDATE es condición-única (solo toca
-- vat=0, nunca pisa una cuenta que el dueño haya asignado a mano).
-- ============================================================================

INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
VALUES (900, 50207, 'Impuestos por Pagar', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, '0.00', '1', NOW(), '0', NOW(), 0, 1, 0, 0);

INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
VALUES (901, 5020701, 'IVA Débito Fiscal por Pagar', 'Impuestos por Pagar', '50207', 4, 1, 0, 1, 0, 0, 'L', 0, 0, '0.00', '1', NOW(), '0', NOW(), 0, 1, 0, 0);

-- El hueco `vat` de las cuentas por defecto (históricamente 0/inútil) pasa a
-- ser la cuenta de IVA de VENTAS por pagar. Las compras siguen usando
-- purchase_vat_code, que ya estaba vivo.
UPDATE `acc_predefine_account` SET `vat` = 5020701 WHERE `vat` = 0 OR `vat` IS NULL;

-- IGTF percibido (F-12): la percepción del 3% que el cliente paga también es
-- DEUDA con el SENIAT, no ingreso, y el POS ni la calculaba. Se abre su cuenta
-- en la misma rama y una columna de asignación; el código la usa si existe y
-- se degrada a 0 (sin cuenta) con fail-closed en la venta que cobre IGTF.
ALTER TABLE `acc_predefine_account` ADD COLUMN `igtf_code` INT NOT NULL DEFAULT 0;

INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`)
VALUES (902, 5020702, 'IGTF por Pagar', 'Impuestos por Pagar', '50207', 4, 1, 0, 1, 0, 0, 'L', 0, 0, '0.00', '1', NOW(), '0', NOW(), 0, 1, 0, 0);

UPDATE `acc_predefine_account` SET `igtf_code` = 5020702 WHERE `igtf_code` = 0 OR `igtf_code` IS NULL;
