-- ---------------------------------------------------------------------------
-- Menu permissions: seed module/sub_module so every sidebar section renders.
--
-- The layout gates each sidebar section on (index .Perm "<directory>") and
-- admin permissions are read from sub_module. Without these rows, whole
-- sections (Servicios, Devoluciones, Cotizaciones, Bancos, Ajuste de
-- Inventario, Libros Legales, Auditoría) never appear in the menu.
-- INSERTs run as INSERT IGNORE (see transformStatement) => idempotent on
-- both fresh installs (empty tables) and the migrated PHP database.
-- ---------------------------------------------------------------------------

INSERT INTO `module` (`id`, `name`, `description`, `directory`, `status`) VALUES
(1, 'invoice', NULL, NULL, 1),
(2, 'customer', NULL, NULL, 1),
(3, 'product', NULL, NULL, 1),
(4, 'supplier', NULL, NULL, 1),
(5, 'purchase', NULL, NULL, 1),
(6, 'stock', NULL, NULL, 1),
(7, 'return', NULL, NULL, 1),
(8, 'report', NULL, NULL, 1),
(9, 'accounts', NULL, NULL, 1),
(10, 'bank', NULL, NULL, 1),
(11, 'tax', NULL, NULL, 1),
(12, 'hrm_management', NULL, NULL, 1),
(13, 'service', NULL, NULL, 1),
(14, 'setting', NULL, NULL, 1),
(15, 'quotation', NULL, NULL, 1);

INSERT INTO `sub_module` (`id`, `mid`, `name`, `directory`, `status`) VALUES
(1, 1, 'new_invoice', 'new_invoice', 1),
(2, 1, 'manage_invoice', 'manage_invoice', 1),
(3, 1, 'pos_invoice', 'gui_pos', 1),
(4, 2, 'add_customer', 'add_customer', 1),
(5, 2, 'manage_customer', 'manage_customer', 1),
(6, 2, 'credit_customer', 'credit_customer', 1),
(7, 2, 'paid_customer', 'paid_customer', 1),
(8, 2, 'customer_ledger', 'customer_ledger', 1),
(9, 2, 'customer_advance', 'customer_advance', 1),
(10, 3, 'category', 'category', 1),
(11, 3, 'unit', 'unit', 1),
(12, 3, 'add_product', 'create_product', 1),
(13, 3, 'manage_product', 'manage_product', 1),
(14, 4, 'add_supplier', 'add_supplier', 1),
(15, 4, 'manage_supplier', 'manage_supplier', 1),
(16, 4, 'supplier_ledger', 'supplier_ledger', 1),
(17, 5, 'add_purchase', 'add_purchase', 1),
(18, 5, 'manage_purchase', 'manage_purchase', 1),
(19, 6, 'stock_report', 'stock_report', 1),
(20, 9, 'coa', 'coa', 1),
(21, 9, 'vouchers', 'vouchers', 1),
(22, 9, 'ledger', 'ledger', 1),
(23, 8, 'sales_report', 'sales_report', 1),
(24, 8, 'purchase_report', 'purchase_report', 1),
(25, 14, 'setting', 'setting', 1),
(26, 14, 'users', 'users', 1),
(27, 14, 'roles', 'roles', 1),
(28, 14, 'backup', 'backup', 1),
-- Missing rows that hide sidebar sections (added 2026-09-27).
-- Rows 29-40 mirror `InitializeAdminUser` (infrastructure/persistence/admin.go):
-- same ids, same directory keys and same module (mid) values, so both seeders
-- can run in any order (all INSERTs are IGNOREs) without duplicates or gaps.
(29, 13, 'create_service', 'create_service', 1),
(30, 13, 'manage_service', 'manage_service', 1),
(31, 7, 'manage_return', 'manage_return', 1),
(32, 15, 'create_quotation', 'create_quotation', 1),
(33, 15, 'manage_quotation', 'manage_quotation', 1),
(34, 10, 'bank', 'bank', 1),
(35, 10, 'bank_ledger', 'bank_ledger', 1),
(36, 6, 'adjustment', 'adjustment', 1),
(37, 8, 'legal_books', 'legal_books', 1),
(38, 8, 'kardex', 'kardex', 1),
(39, 14, 'audit_log', 'audit_log', 1),
(40, 14, 'exchange_rate', 'exchange_rate', 1),
-- RRHH (mid 12 = hrm_management, ya declarado arriba pero sin submódulos, por
-- lo que la sección nunca aparecía en el menú). Se separan "ver nomina" de
-- "publicar nomina" a propósito: publicar paga dinero real y exige el permiso
-- alto, igual que en el resto del sistema con los books legales y la
-- auditoría.
(41, 12, 'hr_employee', 'hr_employee', 1),
(42, 12, 'hr_payroll', 'hr_payroll', 1),
(43, 12, 'hr_payroll_post', 'hr_payroll_post', 1),
(44, 12, 'hr_tax_rates', 'hr_tax_rates', 1),
-- CRM. Un solo permiso para todo el módulo a proposito: consultar el embudo,
-- registrar una llamada y mover una oportunidad son el mismo trabajo comercial.
-- Quien no comercializa no necesita el modulo; quien lo usa necesita el
-- completo.
(45, 12, 'crm', 'crm', 1);
