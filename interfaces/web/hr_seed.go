package web

// hr_seed.go — Datos iniciales del módulo de RRHH.
//
// IMPORTANTE sobre las tasas: aquí NO se fijan las cifras definitivas. Se siembran
// los conceptos con los valores de referencia más citados y TODOS marcados con
// verified = 0. El sistema muestra un aviso mientras haya conceptos sin validar y
// el contador de la empresa debe confirmarlos contra la legislación vigente antes
// de la primera nómina.
//
// Esto no es pereza: es la posición correcta. Las contribuciones cambian entre
// fuentes y con el tiempo, y una nómina calculada sobre una cifra que nadie
// validó es un problema para el cliente, no para el sistema. La estructura,
// la vigencia y la trazabilidad sí las garantiza el código; la cifra, el contador.

import (
	"database/sql"
	"fmt"
	"time"
)

// SeedHR crea los catálogos mínimos de RRHH. Es idempotente: se puede llamar en
// cada arranque sin duplicar nada.
func SeedHR(db *sql.DB) {
	year := time.Now().Year()
	start := fmt.Sprintf("%d-01-01", year)

	// Conceptos de nómina: asignación base, deducciones y cargas patronales.
	concepts := []struct{ code, name, kind, taxCode string }{
		{"SALARIO", "Salario base", "earning", ""},
		{"BONO", "Bono / Incentivo", "earning", ""},
		{"HORA_EXTRA", "Horas extras", "earning", ""},
		{"ISLR_RET", "Retención ISLR", "deduction", ""},
		{"IVSS_WORKER", "Aporte IVSS del trabajador", "deduction", "IVSS_WORKER"},
		{"IVSS_EMPLOYER", "Aporte patronal IVSS", "employer_charge", "IVSS_EMPLOYER"},
		{"INCEPS", "INCEPS", "employer_charge", "INCEPS"},
		{"FAOV", "FAOV", "employer_charge", "FAOV"},
		{"BPSI", "BPSI", "employer_charge", "BPSI"},
	}
	for _, c := range concepts {
		_, _ = db.Exec(`INSERT IGNORE INTO hr_payroll_concept (code, name, kind, tax_code, status)
			VALUES (?,?,?,?,1)`, c.code, c.name, c.kind, c.taxCode)
	}

	// Tasas de referencia, TODAS sin verificar (verified = 0). La cita legal
	// apunta a qué debe comprobar el contador, no a una norma concreta: se
	// completa al validar.
	rates := []struct {
		code, label string
		pct         float64
		baseKind    string
		appliesTo   string
		note        string
	}{
		{"IVSS_WORKER", "Aporte del trabajador al IVSS", 4.00, "salary", "worker",
			"Porcentaje de aporte del asalariado al IVSS. Verificar vigencia."},
		{"IVSS_EMPLOYER", "Aporte patronal al IVSS (9% + 21%)", 30.00, "salary", "employer",
			"Carga patronal total del IVSS sobre la base del aporte del trabajador. Verificar si se separa en dos conceptos."},
		{"INCEPS", "INCEPS", 1.00, "salary", "employer",
			"Instituto Nacional de Capacitación Laboral. Verificar alícuota vigente."},
		{"FAOV", "FAOV (Banco de Ahorro y Vivienda)", 1.00, "salary_min", "employer",
			"Fondo de Ahorro Obligatorio para la Vivienda. Verificar alícuota y base."},
		{"BPSI", "BPSI (Banco de Protección del Salario del Trabajador)", 0.50, "salary", "employer",
			"BPSI. Verificar alícuota vigente."},
	}
	for _, r := range rates {
		_, _ = db.Exec(`INSERT IGNORE INTO tax_rate_table
			(concept_code, label, rate_pct, base_kind, applies_to, effective_from, legal_citation, verified, status)
			VALUES (?,?,?,?,?,?,?,0,1)`,
			r.code, r.label, r.pct, r.baseKind, r.appliesTo, start, r.note)
	}

	// Departamento base para poder dar de alta empleados sin configurar todo
	// el organigrama primero.
	_, _ = db.Exec(`INSERT IGNORE INTO hr_department (name, code, status) VALUES ('General','GEN',1)`)
}
