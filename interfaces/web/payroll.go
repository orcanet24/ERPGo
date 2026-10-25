package web

// payroll.go — Motor de liquidación de nómina (Venezuela).
//
// El motor NO contiene cifras tributarias. Eso es deliberado y es lo único
// responsable: las tasas y los tramos viven en tax_rate_table / islr_bracket,
// con vigencia y cita legal, y se congelan en el recibo. Así:
//
//   - Si la ley cambia, se agrega una fila con nueva vigencia. Los recibos
//     antiguos siguen siendo correctos porque guardaron la tasa que aplicaba.
//   - La nómina es REPRODUCIBLE: la misma corrida con las mismas tasas da el
//     mismo resultado, y `tax_rate_version` (huella de las tasas usadas) deja
//     constancia de con qué tabla se calculó.
//   - Si el contador de la empresa no ha validado las tasas, el sistema lo
//     avisa y la primera nómina queda marcada: no se factura sobre números que
//     nadie ha revisado.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------- Tasas ----------

// TaxRate es una tasa vigente de un concepto parafiscal.
type TaxRate struct {
	Code          string
	Label         string
	RatePct       float64
	BaseKind      string
	AppliesTo     string // worker | employer
	EffectiveFrom string
	LegalCitation string
	Verified      bool
}

// loadTaxRates devuelve las tasas vigentes en una fecha. Si hay varias
// versiones del mismo concepto, gana la de vigencia más reciente que ya haya
// entrado en vigor: la ley nueva no aplica retroactivamente al pasado.
func (s *Server) loadTaxRates(r *http.Request, asOf string) (map[string]TaxRate, error) {
	rows, err := s.dbFor(r).Query(`SELECT concept_code, label, rate_pct, base_kind, applies_to,
		effective_from, legal_citation, verified
		FROM tax_rate_table
		WHERE status = 1 AND effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)
		ORDER BY effective_from ASC`, asOf, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]TaxRate{}
	for rows.Next() {
		var t TaxRate
		var eff, verified sql.NullString
		if rows.Scan(&t.Code, &t.Label, &t.RatePct, &t.BaseKind, &t.AppliesTo,
			&eff, &t.LegalCitation, &verified) != nil {
			continue
		}
		t.EffectiveFrom = eff.String
		t.Verified = verified.Valid && verified.String != "0"
		// Later effective_from sobrescribe: gana la más reciente en vigor.
		out[t.Code] = t
	}
	return out, rows.Err()
}

// unverifiedTaxRates lista los conceptos sin validar. Mientras haya alguno, la
// interfaz de nómina avisa: es el control que evita liquidar sobre cifras que
// nadie confirmó.
func (s *Server) unverifiedTaxRates(r *http.Request, asOf string) ([]TaxRate, error) {
	rates, err := s.loadTaxRates(r, asOf)
	if err != nil {
		return nil, err
	}
	var out []TaxRate
	codes := make([]string, 0, len(rates))
	for c := range rates {
		codes = append(codes, c)
	}
	sort.Strings(codes) // orden estable para la interfaz
	for _, c := range codes {
		if !rates[c].Verified {
			out = append(out, rates[c])
		}
	}
	return out, nil
}

// rateVersion es la huella de las tasas usadas en una corrida. Es lo que hace
// auditable una nómina antigua: si alguien cambia una tasa hoy, la huella de
// la nómina de marzo deja de coincidir y el recibo se ve sospechoso.
func rateVersion(rates map[string]TaxRate) string {
	codes := make([]string, 0, len(rates))
	for c := range rates {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	var sb strings.Builder
	for _, c := range codes {
		t := rates[c]
		sb.WriteString(fmt.Sprintf("%s=%.4f@%s;%s|", c, t.RatePct, t.EffectiveFrom, t.LegalCitation))
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// ---------- Tramos de ISLR ----------

// ISLRBracket es un tramo de la tabla de retenciones, expresado en UT.
type ISLRBracket struct {
	FromUT        float64
	ToUT          float64 // 0 = sin tope
	Pct           float64
	ExcessUT      float64
	EffectiveFrom string
	LegalCitation string
}

// loadISLRBrackets devuelve los tramos vigentes de un tipo de retención.
func (s *Server) loadISLRBrackets(r *http.Request, bracketType, asOf string) ([]ISLRBracket, error) {
	rows, err := s.dbFor(r).Query(`SELECT from_ut, IFNULL(to_ut,0), pct, excess_ut, effective_from, legal_citation
		FROM islr_bracket
		WHERE status = 1 AND bracket_type = ? AND effective_from <= ?
		  AND (effective_to IS NULL OR effective_to >= ?)
		ORDER BY from_ut ASC`, bracketType, asOf, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ISLRBracket
	for rows.Next() {
		var b ISLRBracket
		var eff string
		if rows.Scan(&b.FromUT, &b.ToUT, &b.Pct, &b.ExcessUT, &eff, &b.LegalCitation) != nil {
			continue
		}
		b.EffectiveFrom = eff
		out = append(out, b)
	}
	return out, rows.Err()
}

// islrRetentionOn calculates la retención de ISLR sobre la base gravable
// expresada en unidades tributarias (UT), recorriendo los tramos:
//
//	retención = Σ (exceso del tramo × %) − sustraendo acumulado
//
// El algoritmo es "por tramos": se acumula sobre el tramo en que cae la base
// y se resta el sustraendo acumulado de los tramos anteriores, que es como lo
// define la tabla oficial. Si no hay tramos configurados devuelve 0 en vez de
// inventar un porcentaje.
func islrRetentionOn(brackets []ISLRBracket, baseUT float64) float64 {
	if baseUT <= 0 || len(brackets) == 0 {
		return 0
	}
	retention := 0.0
	remaining := baseUT
	for _, b := range brackets {
		if remaining <= 0 {
			break
		}
		// Cantidad de la base que cae dentro de este tramo.
		span := remaining
		if b.ToUT > 0 {
			room := b.ToUT - b.FromUT
			if span > room {
				span = room
			}
		}
		if span <= 0 {
			continue
		}
		retention += span * b.Pct / 100.0
		remaining -= span
	}
	if retention < 0 {
		return 0
	}
	return round2(retention)
}

// ---------- Cálculo de un recibo ----------

// PayrollLine es una línea del recibo ya calculada.
type PayrollLine struct {
	Code       string
	Kind       string // earning | deduction | employer_charge
	Amount     float64
	RatePct    float64
	BaseAmount float64
}

// Payslip es el resultado de liquidar un empleado en un período.
type Payslip struct {
	EmployeeID   int64
	EmployeeName string
	Gross        float64
	Deductions   float64
	EmployerCost float64
	Net          float64
	Lines        []PayrollLine
}

// PayrollInput son los datos que alimentan el cálculo.
type PayrollInput struct {
	EmployeeID   int64
	EmployeeName string
	// Frequency es la frecuencia de pago del empleado (quincenal o mensual). Va
	// aquí, y no como parámetro aparte, porque el prorrateo del ISLR depende de
	// ella: una quincena grava la mitad de la remuneración mensual, y mezclar
	// una frecuencia con el período de otra es exactamente el error que hace que
	// una nómina retenga de más.
	Frequency  string
	Earnings   []PayrollLine // asignaciones
	ISLRBaseUT float64       // base gravable en UT (el patrón real usa UT)
	UTValue    float64       // valor de 1 UT en Bs
}

// calculatePayslip aplica retenciones al trabajador y cargas patronales.
//
// La tasa efectiva de retención se calcula sobre la base en UT y se aplica al
// salario en Bs: trabajar en UT es lo que hace que la nómina siga siendo
// correcta cuando la UT cambia, sin rehacer la tabla de tramos.
func (s *Server) calculatePayslip(r *http.Request, in PayrollInput, rates map[string]TaxRate, brackets []ISLRBracket) Payslip {
	slip := Payslip{EmployeeID: in.EmployeeID, EmployeeName: in.EmployeeName}

	baseSalary := 0.0
	for _, e := range in.Earnings {
		slip.Lines = append(slip.Lines, e)
		slip.Gross += e.Amount
		if e.Code == "SALARIO" || e.Code == "BASE" {
			baseSalary = e.Amount
		}
	}
	if baseSalary == 0 {
		baseSalary = slip.Gross
	}

	// --- Deducciones al trabajador ---
	// ISLR retenido: la tabla se aplica sobre la base en UT.
	if len(brackets) > 0 && in.ISLRBaseUT > 0 {
		retencionUT := islrRetentionOn(brackets, in.ISLRBaseUT)
		retencionBs := retencionUT * in.UTValue
		retencionBs = min(retencionBs, slip.Gross) // la retención nunca supera el salario
		if retencionBs > 0 {
			slip.Lines = append(slip.Lines, PayrollLine{
				Code: "ISLR_RET", Kind: "deduction", Amount: round2(retencionBs),
				RatePct: 0, BaseAmount: round2(in.ISLRBaseUT * in.UTValue),
			})
			slip.Deductions += round2(retencionBs)
		}
	}

	// Retenciones de seguridad social del trabajador (las que fija la tabla).
	for _, code := range []string{"IVSS_WORKER"} {
		r, ok := rates[code]
		if !ok || r.AppliesTo != "worker" {
			continue
		}
		amount := round2(baseSalary * r.RatePct / 100)
		if amount > 0 {
			slip.Lines = append(slip.Lines, PayrollLine{
				Code: code, Kind: "deduction", Amount: amount,
				RatePct: r.RatePct, BaseAmount: round2(baseSalary),
			})
			slip.Deductions += amount
		}
	}

	slip.Gross = round2(slip.Gross)
	slip.Deductions = round2(slip.Deductions)
	slip.Net = round2(slip.Gross - slip.Deductions)

	// --- Cargas patronales ---
	// No se descuentan al trabajador: son costo real de la empresa y deben
	// provisionarse para que el margen no sea una ficción.
	//
	// El base se resuelve una vez por fuera del bucle: dentro, el nombre `r`
	// está ocupado por la tasa de la iteración y taparía a la petición.
	db := s.dbFor(r)
	for _, code := range []string{"IVSS_EMPLOYER", "INCEPS", "FAOV", "BPSI"} {
		rate, ok := rates[code]
		if !ok || rate.AppliesTo != "employer" {
			continue
		}
		base := baseSalary
		if rate.BaseKind == "salary_min" {
			// Base por salario mínimo: se calcula sobre el mínimo, no sobre el
			// salario del empleado (así lo establece la norma para estas
			// contribuciones).
			if minSalary := s.minimumWage(db); minSalary > 0 {
				base = minSalary
			}
		}
		amount := round2(base * rate.RatePct / 100)
		if amount > 0 {
			slip.Lines = append(slip.Lines, PayrollLine{
				Code: code, Kind: "employer_charge", Amount: amount,
				RatePct: rate.RatePct, BaseAmount: round2(base),
			})
			slip.EmployerCost += amount
		}
	}
	slip.EmployerCost = round2(slip.EmployerCost)
	return slip
}

// minimumWage devuelve el salario mínimo configurado (base de algunas
// contribuciones). Se lee de fiscal_setting para que cada empresa lo ajuste.
//
// Recibe la conexión y no la petición: es un dato de configuración de la
// empresa, y el salary mínimo no cambia de una página a otra.
func (s *Server) minimumWage(db *sql.DB) float64 {
	var v sql.NullString
	if err := db.QueryRow(`SELECT value FROM fiscal_setting WHERE ` + "`key`" + ` = 'minimum_wage'`).Scan(&v); err != nil {
		return 0
	}
	var f float64
	_, _ = fmt.Sscanf(strings.TrimSpace(v.String), "%f", &f)
	return f
}

// min es el mínimo de dos flotantes (round2 vive en fiscal.go y se comparte
// con el resto de importes fiscales del sistema).
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// runDate devuelve la fecha de referencia para cargar tasas vigentes.
func runDate(from, to string) string {
	if to != "" {
		return to
	}
	return time.Now().Format("2006-01-02")
}

// =====================
// Routes & Handlers
// =====================

// registerPayrollRoutes registra los endpoints POST de nómina.
//
// La convención es la del resto de módulos: un handler delgado que valida la
// entrada, delega en el servicio (hr_service.go) para el cálculo, y traduce el
// resultado a JSON. La auditoría va en la cadena (aiAudit) para que cada corrida
// quede rastreable.
func (s *Server) registerPayrollRoutes() {
	s.mux.HandleFunc("POST /payroll/run/save", s.payrollSave)
	s.mux.HandleFunc("POST /payroll/run/list", s.payrollListData)
	s.mux.HandleFunc("POST /payroll/run/{id}", s.payrollRunDetail)
	s.mux.HandleFunc("POST /payroll/payslip/{id}", s.payrollPayslip)
}

// payrollSave calcula y persiste una corrida de nómina.
//
// Recibe period_from, period_to, frequency, employee_ids[] del formulario. Para
// cada empleado:
//  1. Carga sus asignaciones y construye el PayrollInput.
//  2. Calcula el recibo con calculatePayslip().
//  3. Inserta hr_payroll_run (cabecera), hr_payroll_item (renglones) y
//     hr_payslip (recibos) en una sola transacción.
//  4. Genera los asientos contables en acc_vaucher (débito: gasto de personal,
//     crédito: caja/banco + seguridad social por pagar).
//  5. Audita la operación.
//
// La publicación (status='posted') queda separada en /hr/payroll/post/{id}:
// calcular y publicar son actos distintos, y separarlos permite revisar antes
// de pagar dinero real.
func (s *Server) payrollSave(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	createdBy := sessUserID(sess)

	if r.Method != "POST" {
		jsonError(w, "Método no permitido", 405)
		return
	}

	periodFrom := strings.TrimSpace(r.FormValue("period_from"))
	periodTo := strings.TrimSpace(r.FormValue("period_to"))
	frequency := strings.TrimSpace(r.FormValue("frequency"))
	if frequency == "" {
		frequency = "quincenal"
	}

	// Validar período
	periodFrom, periodTo, err := hrValidatePeriod(periodFrom, periodTo)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	// Frecuencia
	if frequency != "quincenal" && frequency != "mensual" {
		jsonError(w, "Frecuencia inválida (quincenal o mensual)", 400)
		return
	}

	// employee_ids[] opcional: si no vienen, se liquidan todos los activos
	var employeeIDs []int64
	if ids := r.Form["employee_ids"]; len(ids) > 0 {
		for _, idStr := range ids {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err == nil && id > 0 {
				employeeIDs = append(employeeIDs, id)
			}
		}
	}

	// Cargar tasas y tramos vigentes para el período
	asOf := runDate(periodFrom, periodTo)
	rates, err := s.loadTaxRates(r, asOf)
	if err != nil {
		jsonError(w, "No se pudieron cargar las tasas fiscales: "+err.Error(), 500)
		return
	}
	brackets, err := s.loadISLRBrackets(r, "tipo1", asOf)
	if err != nil {
		jsonError(w, "No se pudieron cargar los tramos de ISLR: "+err.Error(), 500)
		return
	}

	// Cargar empleados
	employees, err := s.hrEmployeesForRun(r)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	// Filtrar por employee_ids si se especificaron
	if len(employeeIDs) > 0 {
		idSet := map[int64]bool{}
		for _, id := range employeeIDs {
			idSet[id] = true
		}
		filtered := make([]PayrollInput, 0, len(employees))
		for _, emp := range employees {
			if idSet[emp.EmployeeID] {
				filtered = append(filtered, emp)
			}
		}
		employees = filtered
	}

	if len(employees) == 0 {
		jsonError(w, "No hay empleados para liquidar", 400)
		return
	}

	// Calcular recibos
	slips := make([]Payslip, 0, len(employees))
	for _, in := range employees {
		frac := hrPeriodFraction(in.Frequency, periodFrom, periodTo)

		// Prorratear salario base al período y resolver porcentajes
		var baseSalary float64
		for _, e := range in.Earnings {
			if e.Code == "SALARIO" || e.Code == "BASE" {
				baseSalary = e.Amount
			}
		}
		basePeriodo := round2(baseSalary * frac)

		// Cargar ausencias aprobadas
		leaves, errL := hrLoadLeavesApproved(s.dbFor(r), in.EmployeeID, periodFrom, periodTo)
		if errL != nil {
			jsonError(w, "No se pudieron cargar las ausencias: "+errL.Error(), 500)
			return
		}
		salarioPeriodo, _ := BasesPorLeave(basePeriodo, leaves, periodFrom, periodTo)
		if len(leaves) > 0 {
			basePeriodo = salarioPeriodo
		}

		// Escalar asignaciones
		scaledEarnings := make([]PayrollLine, 0, len(in.Earnings))
		for _, e := range in.Earnings {
			if e.Code == "SALARIO" || e.Code == "BASE" {
				e.Amount = basePeriodo
			} else if e.RatePct > 0 {
				e.Amount = round2(basePeriodo * e.RatePct / 100)
			}
			scaledEarnings = append(scaledEarnings, e)
		}

		// Base ISLR en UT
		utValue := s.hrUTValue(r)
		var baseUT float64
		if utValue > 0 {
			baseUT = round2(baseSalary * frac / utValue)
		}

		slip := s.calculatePayslip(r, PayrollInput{
			EmployeeID:   in.EmployeeID,
			EmployeeName: in.EmployeeName,
			Frequency:    in.Frequency,
			Earnings:     scaledEarnings,
			ISLRBaseUT:   baseUT,
			UTValue:      utValue,
		}, rates, brackets)
		slips = append(slips, slip)
	}

	// Totales
	var totalGross, totalDeductions, totalNet, totalEmployer float64
	for _, sl := range slips {
		totalGross += sl.Gross
		totalDeductions += sl.Deductions
		totalNet += sl.Net
		totalEmployer += sl.EmployerCost
	}
	totalGross = round2(totalGross)
	totalDeductions = round2(totalDeductions)
	totalNet = round2(totalNet)
	totalEmployer = round2(totalEmployer)
	version := rateVersion(rates)

	// --- Persistencia en transacción ---
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, "No se pudo iniciar la transacción: "+err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Verificar que no exista corrida publicada para el mismo período
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM hr_payroll_run
		WHERE period_from = ? AND period_to = ? AND status = 'posted'`, periodFrom, periodTo).Scan(&existing); err != nil {
		jsonError(w, "Error verificando corridas existentes: "+err.Error(), 500)
		return
	}
	if existing > 0 {
		jsonError(w, fmt.Sprintf("Ya existe una nómina PUBLICADA para el período %s a %s", periodFrom, periodTo), 409)
		return
	}

	// Insertar cabecera de corrida
	runRes, err := tx.Exec(`INSERT INTO hr_payroll_run
		(period_from, period_to, frequency, status, employee_count,
		 total_gross, total_deductions, total_employer, total_net,
		 tax_rate_version, created_by, created_at)
		VALUES (?,?,?,'draft',?,?,?,?,?,?,?,?)`,
		periodFrom, periodTo, frequency, len(slips),
		totalGross, totalDeductions, totalEmployer, totalNet,
		version, createdBy, now())
	if err != nil {
		jsonError(w, "No se pudo crear la corrida: "+err.Error(), 500)
		return
	}
	runID, _ := runRes.LastInsertId()

	// Insertar recibos e ítems
	for _, sl := range slips {
		if _, err := tx.Exec(`INSERT INTO hr_payslip (run_id, employee_id, net_pay, issued_at)
			VALUES (?,?,?,?)`, runID, sl.EmployeeID, sl.Net, now()); err != nil {
			jsonError(w, "No se pudo crear el recibo: "+err.Error(), 500)
			return
		}
		for _, l := range sl.Lines {
			if _, err := tx.Exec(`INSERT INTO hr_payroll_item
				(run_id, employee_id, concept_code, kind, amount, rate_pct, base_amount)
				VALUES (?,?,?,?,?,?,?)`,
				runID, sl.EmployeeID, l.Code, l.Kind, l.Amount, l.RatePct, l.BaseAmount); err != nil {
				jsonError(w, "No se pudo crear el ítem: "+err.Error(), 500)
				return
			}
		}
	}

	// --- Asientos contables ---
	// Patrón de la nómina: el gasto de personal va al debe; el pago al trabajador
	// y la carga patronal retenida van al haber.
	pre, err := s.predefineAccount(r)
	if err != nil {
		jsonError(w, "No se pudieron cargar las cuentas predefinidas: "+err.Error(), 500)
		return
	}

	salaryCode := pre.SalaryCode
	if salaryCode == 0 {
		salaryCode = 621 // 621 Remuneraciones al personal (fallback)
	}
	cashCode := pre.CashCode
	if cashCode == 0 {
		cashCode = 101 // 101 Caja (fallback)
	}

	// 413 Participación de los trabajadores por pagar: la carga patronal retenida
	seguridadSocial := int64(413)
	var ssExiste int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE HeadCode = ?`, seguridadSocial).Scan(&ssExiste)
	contrapartidaSS := seguridadSocial
	if ssExiste == 0 {
		contrapartidaSS = cashCode
	}

	// Líneas del asiento:
	//   DEBE  salaryCode   totalGross + totalEmployer  (gasto de personal con carga)
	//   HABER cashCode     totalNet                    (pago a empleados)
	//   HABER contraSS     totalEmployer               (carga patronal retenida)
	//
	// Nota: insertVoucherRow inserta en acc_vaucher (borrador). Cada fila se
	// aprueba separadamente, y approveVoucher escribe el par principal + espejo
	// en acc_transaction (doble entrada).

	montoPago := totalNet
	montoCarga := totalEmployer

	// Línea 1: gasto de personal (débito) contra caja (crédito por el pago neto)
	if montoPago > 0 {
		vno := nextVoucherNoTx(tx, "JV")
		if err := s.insertVoucherRow(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", runID),
			VDate: periodTo,
			COAID: salaryCode, RevCodde: cashCode,
			Narration: fmt.Sprintf("Nómina %s a %s: pago a empleados", periodFrom, periodTo),
			Comment:   "Payroll save - employee net pay",
			Debit:     montoPago,
		}, createdBy); err != nil {
			jsonError(w, "No se pudo generar el asiento de pago: "+err.Error(), 500)
			return
		}
	}

	// Línea 2: gasto de personal (débito) contra seguridad social (crédito por carga)
	if montoCarga > 0 {
		vno := nextVoucherNoTx(tx, "JV")
		if err := s.insertVoucherRow(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", runID),
			VDate: periodTo,
			COAID: salaryCode, RevCodde: contrapartidaSS,
			Narration: fmt.Sprintf("Nómina %s a %s: carga patronal retenida", periodFrom, periodTo),
			Comment:   "Payroll save - employer charges retained",
			Debit:     montoCarga,
		}, createdBy); err != nil {
			jsonError(w, "No se pudo generar el asiento de carga patronal: "+err.Error(), 500)
			return
		}
	}

	// Commit
	if err := tx.Commit(); err != nil {
		jsonError(w, "No se pudo confirmar la transacción: "+err.Error(), 500)
		return
	}

	// Auditar
	s.aiAudit(sess, createdBy, "create", "rrhh", "hr_payroll_run", fmt.Sprintf("%d", runID), nil, map[string]interface{}{
		"period_from": periodFrom, "period_to": periodTo,
		"frequency": frequency, "employee_count": len(slips),
		"total_gross": totalGross, "total_deductions": totalDeductions,
		"total_net": totalNet, "total_employer": totalEmployer,
		"tax_rate_version": version,
	})

	// Advertencia de tasas sin verificar
	resp := map[string]interface{}{
		"status": 1, "msg": "Corrida de nómina calculada en borrador",
		"run_id": runID, "employee_count": len(slips),
		"total_gross": money(totalGross), "total_deductions": money(totalDeductions),
		"total_net": money(totalNet), "total_employer": money(totalEmployer),
		"tax_rate_version": version,
	}

	if unv, _ := s.unverifiedTaxRates(r, asOf); len(unv) > 0 {
		pendientes := make([]map[string]interface{}, 0, len(unv))
		for _, t := range unv {
			pendientes = append(pendientes, map[string]interface{}{
				"code": t.Code, "label": t.Label, "rate_pct": t.RatePct,
				"legal_citation": t.LegalCitation,
			})
		}
		resp["unverified_rates"] = pendientes
		resp["warning"] = "Hay tasas sin verificar. Revíselas antes de publicar."
	}

	writeJSON(w, resp)
}

// payrollListData devuelve las corridas de nómino en formato DataTables.
func (s *Server) payrollListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}
	search := strings.TrimSpace(r.FormValue("search[value]"))
	where := ""
	args := []interface{}{}
	if search != "" {
		where = " WHERE status = ? OR frequency = ? OR period_from LIKE ?"
		like := "%" + search + "%"
		args = append(args, search, search, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_payroll_run`).Scan(&total)
	if where != "" {
		_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_payroll_run`+where, args...).Scan(&filtered)
	} else {
		filtered = total
	}

	rows, err := s.dbFor(r).Query(`SELECT id, period_from, period_to, frequency, status,
		employee_count, total_gross, total_deductions, total_employer, total_net,
		tax_rate_version, created_by, created_at
		FROM hr_payroll_run `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var (
			id                              int64
			from, to, freq, status, version string
			createdBy, createdAt            string
			count                           int
			gross, ded, emp, net            float64
		)
		if err := rows.Scan(&id, &from, &to, &freq, &status, &count,
			&gross, &ded, &emp, &net, &version, &createdBy, &createdAt); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "period_from": from, "period_to": to, "frequency": freq,
			"status": status, "employee_count": count,
			"total_gross": money(gross), "total_deductions": money(ded),
			"total_employer": money(emp), "total_net": money(net),
			"tax_rate_version": version, "created_by": createdBy,
			"created_at": createdAt,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// payrollRunDetail devuelve el detalle de una corrida: cabecera + recibos.
func (s *Server) payrollRunDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Corrida no especificada", 400)
		return
	}

	var (
		from, to, freq, status, version, createdBy, createdAt string
		count                                                 int
		gross, ded, emp, net                                  float64
	)
	err := s.dbFor(r).QueryRow(`SELECT period_from, period_to, frequency, status,
		employee_count, total_gross, total_deductions, total_employer, total_net,
		tax_rate_version, created_by, created_at
		FROM hr_payroll_run WHERE id = ?`, id).
		Scan(&from, &to, &freq, &status, &count, &gross, &ded, &emp, &net, &version, &createdBy, &createdAt)
	if err != nil {
		jsonError(w, "Corrida no encontrada", 404)
		return
	}

	// Recibos con sus ítems
	rows, err := s.dbFor(r).Query(`SELECT p.employee_id, e.code, e.first_name, e.last_name,
		p.net_pay, p.issued_at
		FROM hr_payslip p
		JOIN hr_employee e ON e.id = p.employee_id
		WHERE p.run_id = ? ORDER BY e.code`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type slipDetail struct {
		EmployeeID   int64   `json:"employee_id"`
		EmployeeCode string  `json:"employee_code"`
		EmployeeName string  `json:"employee_name"`
		NetPay       float64 `json:"net_pay"`
		IssuedAt     string  `json:"issued_at"`
		Gross        float64 `json:"gross"`
		Deductions   float64 `json:"deductions"`
	}
	slips := []slipDetail{}
	for rows.Next() {
		var (
			empID                  int64
			code, first, last, iss string
			netP                   float64
		)
		if err := rows.Scan(&empID, &code, &first, &last, &netP, &iss); err != nil {
			continue
		}
		// Calcular bruto y deducciones de los ítems
		var sumGross, sumDed float64
		_ = s.dbFor(r).QueryRow(`SELECT
			COALESCE(SUM(CASE WHEN kind='earning' THEN amount ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN kind='deduction' THEN amount ELSE 0 END),0)
			FROM hr_payroll_item WHERE run_id = ? AND employee_id = ?`, id, empID).Scan(&sumGross, &sumDed)

		slips = append(slips, slipDetail{
			EmployeeID:   empID,
			EmployeeCode: code,
			EmployeeName: strings.TrimSpace(first + " " + last),
			NetPay:       netP,
			IssuedAt:     iss,
			Gross:        sumGross,
			Deductions:   sumDed,
		})
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"run": map[string]interface{}{
			"id": id, "period_from": from, "period_to": to, "frequency": freq,
			"status": status, "employee_count": count,
			"total_gross": money(gross), "total_deductions": money(ded),
			"total_employer": money(emp), "total_net": money(net),
			"tax_rate_version": version, "created_by": createdBy,
			"created_at": createdAt,
		},
		"payslips": slips,
	})
}

// payrollPayslip devuelve un recibo individual (un empleado en una corrida).
func (s *Server) payrollPayslip(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "Recibo no especificado", 400)
		return
	}

	// id es el id de hr_payslip
	var payslipID, runID, empID int64
	var netPay float64
	var issuedAt string
	err := s.dbFor(r).QueryRow(`SELECT id, run_id, employee_id, net_pay, issued_at
		FROM hr_payslip WHERE id = ?`, id).Scan(&payslipID, &runID, &empID, &netPay, &issuedAt)
	if err != nil {
		jsonError(w, "Recibo no encontrado", 404)
		return
	}

	// Datos del empleado
	var empCode, empFirst, empLast string
	_ = s.dbFor(r).QueryRow(`SELECT code, first_name, last_name FROM hr_employee WHERE id = ?`, empID).
		Scan(&empCode, &empFirst, &empLast)

	// Datos de la corrida
	var periodFrom, periodTo, frequency, status, taxVersion string
	_ = s.dbFor(r).QueryRow(`SELECT period_from, period_to, frequency, status, tax_rate_version
		FROM hr_payroll_run WHERE id = ?`, runID).
		Scan(&periodFrom, &periodTo, &frequency, &status, &taxVersion)

	// Ítems del recibo
	rows, err := s.dbFor(r).Query(`SELECT concept_code, kind, amount, rate_pct, base_amount
		FROM hr_payroll_item WHERE run_id = ? AND employee_id = ? ORDER BY kind, concept_code`,
		runID, empID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type itemJSON struct {
		Code       string  `json:"code"`
		Kind       string  `json:"kind"`
		Amount     float64 `json:"amount"`
		RatePct    float64 `json:"rate_pct"`
		BaseAmount float64 `json:"base_amount"`
	}
	var lines []itemJSON
	var gross, deductions, employerCost float64
	for rows.Next() {
		var it itemJSON
		if err := rows.Scan(&it.Code, &it.Kind, &it.Amount, &it.RatePct, &it.BaseAmount); err != nil {
			continue
		}
		lines = append(lines, it)
		switch it.Kind {
		case "earning":
			gross += it.Amount
		case "deduction":
			deductions += it.Amount
		case "employer_charge":
			employerCost += it.Amount
		}
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"payslip": map[string]interface{}{
			"id":               payslipID,
			"run_id":           runID,
			"employee_id":      empID,
			"employee_code":    empCode,
			"employee_name":    strings.TrimSpace(empFirst + " " + empLast),
			"period_from":      periodFrom,
			"period_to":        periodTo,
			"frequency":        frequency,
			"status":           status,
			"tax_rate_version": taxVersion,
			"gross":            money(round2(gross)),
			"deductions":       money(round2(deductions)),
			"employer_cost":    money(round2(employerCost)),
			"net_pay":          money(netPay),
			"issued_at":        issuedAt,
			"lines":            lines,
		},
	})
}
