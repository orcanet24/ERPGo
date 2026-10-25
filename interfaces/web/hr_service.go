package web

// hr_service.go — Orquestación de la nómina.
//
// Este archivo es el puente entre el motor de cálculo (payroll.go, puro) y la
// base de datos: carga los datos de una corrida, invoca el cálculo, y persiste
// el resultado.
//
// Decisión de diseño que atraviesa todo el módulo: UNA CORRIDA PUBLICADA ES
// INMUTABLE. Al publicar se congelan los renglones y la versión de las tasas
// (tax_rate_version). Si la ley cambia mañana, la nómina de ayer sigue
// demostrar exactamente qué se aplicó y con qué cifra; recalcularla sería
// falsear un documento ya entregado al trabajador y al SENIAT.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// hrAudit registra un movimiento de RRHH en la cadena de auditoría.
//
// No usa aiAudit porque ese helper etiqueta todo como módulo "ia": una nómina
// calculada por una persona en la pantalla de RRHH es un acto humano, y
// confundirlos volvería inútil el rastro cuando hay que responder por qué se
// pagó lo que se pagó.
//
// La nómina es, además, el registro que más se disputa: un recibo firmado exige
// poder demostrar quién lo generó y con qué tasas. Por eso entra en la cadena
// con hash (appendAudit), no como un log suelto.
func (s *Server) hrAudit(r *http.Request, userID, action, table, recordID string, oldV, newV interface{}) {
	s.appendAudit(r, userID, "", action, "rrhh", table, recordID, oldV, newV)
}

// HRPayrollRunResult es el resumen de una corrida recién calculada.
type HRPayrollRunResult struct {
	RunID           int64
	PeriodFrom      string
	PeriodTo        string
	Frequency       string
	EmployeeCount   int
	TotalGross      float64
	TotalDeductions float64
	TotalNet        float64
	TotalEmployer   float64
	Unverified      []TaxRate
}

// hrValidatePeriod comprueba que un período sea utilizable como corrida.
//
// Las fechas se validan como AAAA-MM-DD y se exige que el período no esté
// invertido. Se devuelve el período ya normalizado en ISO, que es el formato en
// que se almacena, para que el resto del código no tenga que reconvertir.
func hrValidatePeriod(from, to string) (string, string, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if !aiValidDate(from) || !aiValidDate(to) {
		return "", "", fmt.Errorf("las fechas del período deben tener el formato AAAA-MM-DD")
	}
	fromT, err := time.Parse("2006-01-02", from)
	if err != nil {
		return "", "", fmt.Errorf("fecha inicial inválida")
	}
	toT, err := time.Parse("2006-01-02", to)
	if err != nil {
		return "", "", fmt.Errorf("fecha final inválida")
	}
	if toT.Before(fromT) {
		return "", "", fmt.Errorf("la fecha final del período no puede ser anterior a la inicial")
	}
	// Un período de más de un año casi siempre es un error de captura, y tiene
	// consecuencias fiscales graves: se calcularía ISLR sobre una base que	// incluye muchísimos periodos. Se corta aquí en vez de dejar que el motor lo note.
	if toT.Sub(fromT) > 366*24*time.Hour {
		return "", "", fmt.Errorf("el período no puede superar un año")
	}
	return from, to, nil
}

// hrEmployeesForRun carga los empleados activos con su estructura salarial.
//
// Devuelve un error si no hay empleados: una corrida vacía se publicaría como
// "cero nómina", que es un documento sin sentido que después alguien imprimiría
// y archivaría. Es más honesto fallar aquí.
func (s *Server) hrEmployeesForRun(r *http.Request) ([]PayrollInput, error) {
	rows, err := s.dbFor(r).Query(`
		SELECT e.id,
		       TRIM(CONCAT(e.first_name, ' ', e.last_name)),
		       e.base_salary,
		       e.pay_frequency,
		       IFNULL(c.code,''), IFNULL(a.amount,0), IFNULL(a.is_percentage,0)
		FROM hr_employee e
		LEFT JOIN hr_salary_assignment a
		       ON a.employee_id = e.id AND a.status = 1 AND a.period = e.pay_frequency
		LEFT JOIN hr_payroll_concept c
		       ON c.id = a.concept_id AND c.status = 1
		WHERE e.status = 1
		ORDER BY e.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Se indexa por empleado porque el LEFT JOIN devuelve una fila por
	// asignación, no por empleado.
	porEmpleado := map[int64]*PayrollInput{}
	var orden []int64

	for rows.Next() {
		var (
			id            int64
			nombre        string
			baseSalary    float64
			frecuencia    string
			conceptCode   string
			amount        float64
			isPercentuale int
		)
		if err := rows.Scan(&id, &nombre, &baseSalary, &frecuencia, &conceptCode, &amount, &isPercentuale); err != nil {
			continue
		}
		in, ok := porEmpleado[id]
		if !ok {
			in = &PayrollInput{EmployeeID: id, EmployeeName: nombre, Frequency: frecuencia}
			porEmpleado[id] = in
			orden = append(orden, id)
			// El salario base es siempre una asignación, exista o no una
			// fila en hr_salary_assignment. Sin esto, un empleado recién dado de
			// alta se liquidaría en cero.
			in.Earnings = append(in.Earnings, PayrollLine{Code: "SALARIO", Kind: "earning", Amount: round2(baseSalary)})
		}
		if conceptCode == "" {
			continue
		}
		if amount == 0 {
			continue
		}
		if isPercentuale == 1 {
			// El porcentaje NO se resuelve aquí: se transporta crudo en
			// RatePct para que hrBuildPayslips lo aplique sobre el salario del
			// PERÍODO. Resolverlo contra el salario mensual y luego proratear
			// sólo el base hacía que un bono del 10% se pagara dos veces al mes
			// (100 + 100 en vez de 100), que es dinero de más para la empresa y
			// un descuadre que el trabajador detecta en la primera quincena.
			in.Earnings = append(in.Earnings, PayrollLine{Code: conceptCode, Kind: "earning", RatePct: amount})
			continue
		}
		in.Earnings = append(in.Earnings, PayrollLine{Code: conceptCode, Kind: "earning", Amount: amount})
	}

	if len(orden) == 0 {
		return nil, fmt.Errorf("no hay empleados activos para liquidar")
	}

	outs := make([]PayrollInput, 0, len(orden))
	for _, id := range orden {
		outs = append(outs, *porEmpleado[id])
	}
	return outs, nil
}

// hrRunPayrollInput es la base gravable de un empleado en el período, expresada
// en UT. La_ISLR se calcula sobre el SALARIO DEVENGADO DEL PERÍODO, no sobre el
// salario mensual: por eso se reconstruye aquí en vez de leerse de la ficha.
func (s *Server) hrISLRBaseUT(r *http.Request, periodoPct float64, baseSalary float64) (float64, float64, error) {
	ut := s.hrUTValue(r)
	if ut <= 0 {
		return 0, 0, fmt.Errorf("no hay valor de UT configurado; la retención de ISLR no puede calcularse")
	}
	// baseSalary es la remuneración mensual; el ISLR se liquida sobre el total
	// del período (quincenal = medio mes, mensual = mes completo).
	return round2(baseSalary * periodoPct / ut), ut, nil
}

// hrUTValue lee el valor de la Unidad Tributaria vigente de la fiscal_setting.
// La UT la fija el SENIAT por resolución y cambia varias veces al año, así que
// es un dato más, no una constante.
func (s *Server) hrUTValue(r *http.Request) float64 {
	var v sql.NullString
	if err := s.dbFor(r).QueryRow(`SELECT value FROM fiscal_setting WHERE ` + "`key`" + ` = 'ut_value'`).Scan(&v); err != nil {
		return 0
	}
	var f float64
	_, _ = fmt.Sscanf(strings.TrimSpace(v.String), "%f", &f)
	return f
}

// hrPeriodFraction devuelve qué fracción del mes cubre un período, para poder
// convertir la remuneración mensual a la del período (0.5 quincenal, 1 mensual).
func hrPeriodFraction(freq, from, to string) float64 {
	if freq == "mensual" {
		return 1
	}
	// Quincenal: se toma la mitad del salario, salvo que el período cubra el mes
	// completo, en cuyo caso es una mensual disfrazada y cobrar la mitad sería
	// un error de dinero.
	fromT, errF := time.Parse("2006-01-02", from)
	toT, errT := time.Parse("2006-01-02", to)
	if errF == nil && errT == nil {
		dias := toT.Sub(fromT).Hours()/24 + 1
		if dias >= 28 {
			return 1
		}
	}
	return 0.5
}

// hrBuildPayslips calcula el recibo de cada empleado con las tasas vigentes en
// la fecha del período.
//
// Se separó de la persistencia a propósito: el cálculo puro ya está probado en
// payroll_test.go, y esta función es el ensamblador que ninguna otra prueba
// necesita duplicar.
func (s *Server) hrBuildPayslips(r *http.Request, periodFrom, periodTo string) ([]Payslip, []TaxRate, map[string]TaxRate, []ISLRBracket, error) {
	asOf := runDate(periodFrom, periodTo)
	rates, err := s.loadTaxRates(r, asOf)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	brackets, err := s.loadISLRBrackets(r, "tipo1", asOf)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	employees, err := s.hrEmployeesForRun(r)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	slips := make([]Payslip, 0, len(employees))
	for _, in := range employees {
		frac := hrPeriodFraction(in.Frequency, periodFrom, periodTo)
		// Las asignaciones ya vienen resueltas a monto del período cuando son
		// porcentajes; el salario base, en cambio, hay que prorratearlo.
		var baseSalary float64
		for _, e := range in.Earnings {
			if e.Code == "SALARIO" || e.Code == "BASE" {
				baseSalary = e.Amount
			}
		}
		basePeriodo := round2(baseSalary * frac)
		escaladas := make([]PayrollLine, 0, len(in.Earnings))
		for _, e := range in.Earnings {
			if e.Code == "SALARIO" || e.Code == "BASE" {
				e.Amount = basePeriodo
			} else if e.RatePct > 0 {
				// Asignación porcentual: se resuelve contra el salario base del
				// período, no contra el acumulado de líneas (si no, un bono del
				// 10% sobre un aumento en cascada se multiplica).
				e.Amount = round2(basePeriodo * e.RatePct / 100)
			}
			escaladas = append(escaladas, e)
		}

		baseUT, utValue, err := s.hrISLRBaseUT(r, frac, baseSalary)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		slip := s.calculatePayslip(r, PayrollInput{
			EmployeeID:   in.EmployeeID,
			EmployeeName: in.EmployeeName,
			Earnings:     escaladas,
			ISLRBaseUT:   baseUT,
			UTValue:      utValue,
		}, rates, brackets)
		slips = append(slips, slip)
	}

	// Las tasas sin verificar se devuelven para que la interfaz las muestre:
	// el sistema no se niega a calcular, pero deja constancia visible de que la
	// cifra necesita revisión humana antes de pagar.
	unverified, _ := s.unverifiedTaxRates(r, asOf)
	return slips, unverified, rates, brackets, nil
}

// hrCreateRun calcula una corrida y la guarda en estado 'draft'.
//
// Borrador significa que todavía no se ha pagado a nadie: se puede recalcular o
// descartar sin dejar rastro. Publicar (hrPostRun) es el acto irreversible.
func (s *Server) hrCreateRun(r *http.Request, periodFrom, periodTo, frequency, createdBy string) (*HRPayrollRunResult, error) {
	periodFrom, periodTo, err := hrValidatePeriod(periodFrom, periodTo)
	if err != nil {
		return nil, err
	}
	if frequency != "quincenal" && frequency != "mensual" {
		return nil, fmt.Errorf("frecuencia inválida (quincenal o mensual)")
	}

	// Una corrida publicada del mismo período no se duplica: dos nóminas
	// pagadas del mismo mes es doble pago, y el daño ya no se deshace.
	var existing int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_payroll_run
		WHERE period_from = ? AND period_to = ? AND status = 'posted'`, periodFrom, periodTo).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, fmt.Errorf("ya existe una nómina PUBLICADA para el período %s a %s", periodFrom, periodTo)
	}

	slips, unverified, rates, _, err := s.hrBuildPayslips(r, periodFrom, periodTo)
	if err != nil {
		return nil, err
	}

	res := &HRPayrollRunResult{
		PeriodFrom:    periodFrom,
		PeriodTo:      periodTo,
		Frequency:     frequency,
		EmployeeCount: len(slips),
		Unverified:    unverified,
	}
	for _, sl := range slips {
		res.TotalGross += sl.Gross
		res.TotalDeductions += sl.Deductions
		res.TotalNet += sl.Net
		res.TotalEmployer += sl.EmployerCost
	}
	res.TotalGross = round2(res.TotalGross)
	res.TotalDeductions = round2(res.TotalDeductions)
	res.TotalNet = round2(res.TotalNet)
	res.TotalEmployer = round2(res.TotalEmployer)

	// Version Hash de las tasas: congela QUÉ cifras se usaron. Sin esto, si la
	// ley cambiara mañana no habría forma de demostrar qué se aplicó en su día.
	version := rateVersion(rates)

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	runRes, err := tx.Exec(`
		INSERT INTO hr_payroll_run
			(period_from, period_to, frequency, status, employee_count,
			 total_gross, total_deductions, total_employer, total_net,
			 tax_rate_version, created_by, created_at)
		VALUES (?,?,?,'draft',?,?,?,?,?,?,?,?)`,
		periodFrom, periodTo, frequency, res.EmployeeCount,
		res.TotalGross, res.TotalDeductions, res.TotalEmployer, res.TotalNet,
		version, createdBy, now())
	if err != nil {
		return nil, err
	}
	runID, err := runRes.LastInsertId()
	if err != nil {
		return nil, err
	}
	res.RunID = runID

	for _, sl := range slips {
		if _, err := tx.Exec(`
			INSERT INTO hr_payslip (run_id, employee_id, net_pay, issued_at)
			VALUES (?,?,?,?)`, runID, sl.EmployeeID, sl.Net, now()); err != nil {
			return nil, err
		}
		for _, l := range sl.Lines {
			if _, err := tx.Exec(`
				INSERT INTO hr_payroll_item (run_id, employee_id, concept_code, kind, amount, rate_pct, base_amount)
				VALUES (?,?,?,?,?,?,?)`,
				runID, sl.EmployeeID, l.Code, l.Kind, l.Amount, l.RatePct, l.BaseAmount); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.hrAudit(r, createdBy, "create", "hr_payroll_run", fmt.Sprintf("%d", runID), nil, map[string]interface{}{
		"period_from": periodFrom, "period_to": periodTo,
		"total_net": res.TotalNet, "tax_rate_version": version,
	})
	return res, nil
}

// hrPostRun publica una corrida: la marca 'posted' y sella la fecha.
//
// Publicar es el punto de no retorno, y por eso revalida dentro de la
// transacción en lugar de confiar en lo que el handler vio antes: entre la
// lectura y esta escritura otra sesión pudo cancelar o editar la corrida.
func (s *Server) hrPostRun(r *http.Request, runID int64, postedBy string) error {
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRow(`SELECT status FROM hr_payroll_run WHERE id = ?`, runID).Scan(&status); err != nil {
		return fmt.Errorf("la corrida no existe")
	}
	switch status {
	case "posted":
		return fmt.Errorf("la corrida ya fue publicada")
	case "cancelled":
		return fmt.Errorf("la corrida está cancelada y no puede publicarse")
	}

	if _, err := tx.Exec(`UPDATE hr_payroll_run SET status='posted', posted_at=? WHERE id=?`, now(), runID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.hrAudit(r, postedBy, "post", "hr_payroll_run", fmt.Sprintf("%d", runID), nil, map[string]interface{}{"estado": "publicada"})
	return nil
}

// hrCancelRun anula una corrida en borrador o publicada.
//
// Cancelar una ya publicada NO borra los renglones: los conserva y cambia el
// estado, porque el recibo de nómina es un documento que el trabajador ya tiene
// en la mano. Borrarlo sería destruir la evidencia de lo que se pagó.
func (s *Server) hrCancelRun(r *http.Request, runID int64, cancelledBy string) error {
	res, err := s.dbFor(r).Exec(`UPDATE hr_payroll_run SET status='cancelled' WHERE id = ? AND status <> 'cancelled'`, runID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("la corrida no existe o ya estaba cancelada")
	}
	s.hrAudit(r, cancelledBy, "cancel", "hr_payroll_run", fmt.Sprintf("%d", runID), nil, map[string]interface{}{"estado": "cancelada"})
	return nil
}
