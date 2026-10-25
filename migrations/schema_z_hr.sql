-- ============================================================================
-- RRHH: ficha de empleado, asistencia y motor de nómina con parafiscalidad.
--
-- Principio de diseño (no negociable): LAS TASAS SON DATOS, NO CÓDIGO.
-- Las cifras de contribuciones y retenciones variado entre fuentes y cambiaran
-- con la legislación. Por eso viven en `tax_rate_table` con vigencia y cita
-- legal, y el sistema AVISA y exige verificación antes de la primera nómina.
-- Un recibo de nómina guarda qué tasa aplicó ese día: si la ley cambia, los
-- recibos antiguos siguen siendo correctos y demostrables.
--
-- Se crea un esquema nuevo (hr_*) en vez de reutilizar las tablas `employee*`
-- heredadas del PHP: están muertas (cero código Go las usa), su forma es
-- deficiente (latin1, columnas ambiguas) y mezclaban asistencia con nómina.
-- ============================================================================

-- Catálogo de conceptos con su tasa vigente. applies_to: worker | employer.
CREATE TABLE IF NOT EXISTS `tax_rate_table` (
  `concept_code` varchar(30) NOT NULL COMMENT 'IVSS_WORKER, IVSS_EMPLOYER, INCEPS, FAOV, BPSI...',
  `label` varchar(120) NOT NULL DEFAULT '',
  `rate_pct` decimal(8,4) NOT NULL DEFAULT 0 COMMENT 'porcentaje de la base imponible',
  `base_kind` varchar(30) NOT NULL DEFAULT 'salary' COMMENT 'salary | salary_min | n/a',
  `applies_to` varchar(20) NOT NULL DEFAULT 'employer' COMMENT 'worker | employer',
  `effective_from` date NOT NULL,
  `effective_to` date DEFAULT NULL,
  `legal_citation` varchar(255) NOT NULL DEFAULT '' COMMENT 'ley/gaceta/fuente que fija la tasa',
  `verified` integer NOT NULL DEFAULT 0 COMMENT '1 = validada por el contador de la empresa',
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`concept_code`, `effective_from`)
);

-- Tabla de retención de ISLR por tramos (dependientes). Sustraendo y exceso se
-- calculan sobre la base gravable expresada en UT, según el tipo 1 y 2 de la
-- tabla de retenciones publicada por el SENIAT.
CREATE TABLE IF NOT EXISTS `islr_bracket` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `bracket_type` varchar(20) NOT NULL DEFAULT 'tipo1' COMMENT 'tipo1 = deps sin remuneración adicional',
  `from_ut` decimal(14,2) NOT NULL DEFAULT 0,
  `to_ut` decimal(14,2) DEFAULT NULL COMMENT 'NULL = sin tope',
  `pct` decimal(8,4) NOT NULL DEFAULT 0,
  `excess_ut` decimal(14,2) NOT NULL DEFAULT 0 COMMENT 'exceso gravable en UT dentro del tramo',
  `effective_from` date NOT NULL,
  `effective_to` date DEFAULT NULL,
  `legal_citation` varchar(255) NOT NULL DEFAULT '',
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

-- Organigrama
CREATE TABLE IF NOT EXISTS `hr_department` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `name` varchar(80) NOT NULL,
  `code` varchar(20) NOT NULL DEFAULT '',
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

CREATE TABLE IF NOT EXISTS `hr_position` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `department_id` integer NOT NULL DEFAULT 0,
  `name` varchar(80) NOT NULL,
  `base_salary` decimal(14,2) NOT NULL DEFAULT 0,
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

-- Ficha del empleado. Deliberadamente separada de `users`: un empleado no es
-- necesariamente un usuario del sistema (un cajero puede no tener login, y un
-- sistema de RRHH necesita registrar a todos los trabajadores).
CREATE TABLE IF NOT EXISTS `hr_employee` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` varchar(20) NOT NULL COMMENT 'código interno del empleado',
  `national_id` varchar(20) NOT NULL DEFAULT '' COMMENT 'cédula',
  `first_name` varchar(60) NOT NULL,
  `last_name` varchar(60) NOT NULL DEFAULT '',
  `email` varchar(120) NOT NULL DEFAULT '',
  `phone` varchar(30) NOT NULL DEFAULT '',
  `address` text,
  `birth_date` date DEFAULT NULL,
  `hire_date` date NOT NULL,
  `termination_date` date DEFAULT NULL,
  `department_id` integer NOT NULL DEFAULT 0,
  `position_id` integer NOT NULL DEFAULT 0,
  `base_salary` decimal(14,2) NOT NULL DEFAULT 0,
  `pay_frequency` varchar(20) NOT NULL DEFAULT 'quincenal' COMMENT 'quincenal | mensual',
  `bank_code` varchar(10) NOT NULL DEFAULT '',
  `bank_account` varchar(30) NOT NULL DEFAULT '',
  `status` integer NOT NULL DEFAULT 1 COMMENT '1=activo, 0=inactivo, 2=egresado',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
);

CREATE UNIQUE INDEX IF NOT EXISTS `idx_hr_employee_code` ON `hr_employee`(`code`);
CREATE UNIQUE INDEX IF NOT EXISTS `idx_hr_employee_nid` ON `hr_employee`(`national_id`);

-- Conceptos de nómina: asignaciones y deducciones del empleado.
CREATE TABLE IF NOT EXISTS `hr_payroll_concept` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `code` varchar(30) NOT NULL,
  `name` varchar(120) NOT NULL,
  `kind` varchar(20) NOT NULL DEFAULT 'earning' COMMENT 'earning | deduction | employer_charge',
  `tax_code` varchar(30) NOT NULL DEFAULT '' COMMENT 'concept_code de tax_rate_table, si aplica',
  `affects_net` integer NOT NULL DEFAULT 1,
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

-- Estructura salarial por empleado (asignaciones recurrentes).
CREATE TABLE IF NOT EXISTS `hr_salary_assignment` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `employee_id` integer NOT NULL,
  `concept_id` integer NOT NULL,
  `amount` decimal(14,2) NOT NULL DEFAULT 0,
  `is_percentage` integer NOT NULL DEFAULT 0,
  `period` varchar(20) NOT NULL DEFAULT 'quincenal' COMMENT 'quincenal | mensual',
  `status` integer NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
);

-- Asistencia y ausencias
CREATE TABLE IF NOT EXISTS `hr_attendance` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `employee_id` integer NOT NULL,
  `work_date` date NOT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'present' COMMENT 'present | absent | late | holiday',
  `hours_worked` decimal(8,2) NOT NULL DEFAULT 0,
  `note` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
);

CREATE UNIQUE INDEX IF NOT EXISTS `idx_hr_attendance` ON `hr_attendance`(`employee_id`, `work_date`);

CREATE TABLE IF NOT EXISTS `hr_leave` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `employee_id` integer NOT NULL,
  `leave_type` varchar(30) NOT NULL DEFAULT 'vacacion',
  `from_date` date NOT NULL,
  `to_date` date NOT NULL,
  `days` decimal(8,2) NOT NULL DEFAULT 0,
  `paid` integer NOT NULL DEFAULT 1,
  `note` varchar(255) NOT NULL DEFAULT '',
  `status` integer NOT NULL DEFAULT 1 COMMENT '1=aprobada, 0=anulada',
  PRIMARY KEY (`id`)
);

-- Corrida de nómina (cabecera). Una corrida es INMUTABLE una vez cerrada: sus
-- renglones y las tasas aplicadas quedan congelados para siempre.
CREATE TABLE IF NOT EXISTS `hr_payroll_run` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `period_from` date NOT NULL,
  `period_to` date NOT NULL,
  `frequency` varchar(20) NOT NULL DEFAULT 'quincenal',
  `status` varchar(20) NOT NULL DEFAULT 'draft' COMMENT 'draft | posted | cancelled',
  `employee_count` integer NOT NULL DEFAULT 0,
  `total_gross` decimal(16,2) NOT NULL DEFAULT 0,
  `total_deductions` decimal(16,2) NOT NULL DEFAULT 0,
  `total_employer` decimal(16,2) NOT NULL DEFAULT 0,
  `total_net` decimal(16,2) NOT NULL DEFAULT 0,
  `tax_rate_version` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'hash de las tasas usadas: congeladas',
  `created_by` varchar(60) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  `posted_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
);

-- Renglones del recibo: cada línea es un concepto con su monto y, si aplica,
-- la tasa congelada con la que se calculó.
CREATE TABLE IF NOT EXISTS `hr_payroll_item` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `run_id` integer NOT NULL,
  `employee_id` integer NOT NULL,
  `concept_code` varchar(30) NOT NULL,
  `kind` varchar(20) NOT NULL COMMENT 'earning | deduction | employer_charge',
  `amount` decimal(16,2) NOT NULL DEFAULT 0,
  `rate_pct` decimal(8,4) NOT NULL DEFAULT 0 COMMENT 'tasa congelada usada',
  `base_amount` decimal(16,2) NOT NULL DEFAULT 0 COMMENT 'base imponible',
  PRIMARY KEY (`id`)
);

CREATE INDEX IF NOT EXISTS `idx_hr_item_run` ON `hr_payroll_item`(`run_id`);

-- Recibos imprimibles (uno por empleado y corrida).
CREATE TABLE IF NOT EXISTS `hr_payslip` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `run_id` integer NOT NULL,
  `employee_id` integer NOT NULL,
  `net_pay` decimal(16,2) NOT NULL DEFAULT 0,
  `issued_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
);

CREATE UNIQUE INDEX IF NOT EXISTS `idx_hr_payslip` ON `hr_payslip`(`run_id`, `employee_id`);