package web

// hr_handlers.go — Pantallas de RRHH.
//
// Los handlers son delgados a propósito: validan la entrada, delegan en el
// servicio (hr_service.go / payroll.go) y traducen el resultado a JSON o a una
// plantilla. La regla que los separa del resto del sistema es la del módulo
// nómina: publicar, cancelar y liquidar exigen que las tasas con las que se
// calculó estén verificadas por un humano, o el sistema lo dice con claridad
// en lugar de dejar que alguien pague sobre cifras nunca revisadas.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerHRRoutes() {
	s.mux.HandleFunc("GET /hr", s.hrHome)
	s.mux.HandleFunc("GET /hr/employee/list", s.hrEmployeeList)
	s.mux.HandleFunc("GET /hr/employee/list/data", s.hrEmployeeListData)
	s.mux.HandleFunc("GET /hr/employee/add", s.hrEmployeeForm)
	s.mux.HandleFunc("GET /hr/employee/edit/{id}", s.hrEmployeeForm)
	s.mux.HandleFunc("POST /hr/employee/save", s.hrEmployeeSave)
	s.mux.HandleFunc("GET /hr/payroll", s.hrPayrollList)
	s.mux.HandleFunc("GET /hr/payroll/list/data", s.hrPayrollListData)
	s.mux.HandleFunc("POST /hr/payroll/run", s.hrPayrollRun)
	s.mux.HandleFunc("GET /hr/payroll/view/{id}", s.hrPayrollView)
	s.mux.HandleFunc("POST /hr/payroll/post/{id}", s.hrPayrollPost)
	s.mux.HandleFunc("POST /hr/payroll/cancel/{id}", s.hrPayrollCancel)
	s.mux.HandleFunc("GET /hr/tax-rates", s.hrTaxRatePage)
	s.mux.HandleFunc("GET /hr/tax-rates/data", s.hrTaxRateListData)
	s.mux.HandleFunc("POST /hr/tax-rates/verify", s.hrTaxRateVerify)

	// Vacaciones (hr_leave_handlers.go): mism motivo que arriba, las rutas
	// /hr/leave* estaban escritas pero sin dar de alta, y el módulo solo
	// funcionaba con SQL a mano.
	s.registerLeaveRoutes()
}

// ---------- Empleados ----------

func (s *Server) hrHome(w http.ResponseWriter, r *http.Request) {
	// El tablero de RRHH muestra lo único que de verdad importa antes de
	// liquidar: cuántas personas hay activas y qué tasas siguen sin verificar.
	// Se cuenta con el mismo motor que usa la nómina, para que la pantalla y
	// la corrida nunca discrepen.
	var activos, empleados int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_employee WHERE status = 1`).Scan(&activos)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_employee`).Scan(&empleados)

	pendientes, err := s.unverifiedTaxRates(r, today())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	s.render(w, r, "hr_home.html", map[string]interface{}{
		"Title":            "Recursos Humanos",
		"EmpleadosActivos": activos,
		"EmpleadosTotal":   empleados,
		"TasasPendientes":  len(pendientes),
		"Tasas":            pendientes,
	})
}

func (s *Server) hrEmployeeList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "hr_employee_list.html", map[string]interface{}{
		"Title": "Empleados",
	})
}

// hrEmployeeRow es una fila de la lista de empleados.
type hrEmployeeRow struct {
	ID         int64
	Code       string
	Name       string
	NationalID string
	Department string
	Position   string
	Salary     float64
	Frequency  string
	Status     int
}

func (s *Server) hrEmployeeListData(w http.ResponseWriter, r *http.Request) {
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
		where = ` WHERE (e.first_name LIKE ? OR e.last_name LIKE ? OR e.code LIKE ? OR e.national_id LIKE ?)`
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}

	var total, filtered int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_employee`).Scan(&total); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_employee e`+where, args...).Scan(&filtered); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	rows, err := s.dbFor(r).Query(`
		SELECT e.id, e.code,
		       TRIM(CONCAT(e.first_name, ' ', e.last_name)),
		       IFNULL(e.national_id,''),
		       IFNULL(d.name,''), IFNULL(p.name,''),
		       e.base_salary, e.pay_frequency, e.status
		FROM hr_employee e
		LEFT JOIN hr_department d ON d.id = e.department_id
		LEFT JOIN hr_position p ON p.id = e.position_id
		`+where+`
		ORDER BY e.code
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var e hrEmployeeRow
		if err := rows.Scan(&e.ID, &e.Code, &e.Name, &e.NationalID, &e.Department, &e.Position,
			&e.Salary, &e.Frequency, &e.Status); err != nil {
			continue
		}
		estado := "Activo"
		switch e.Status {
		case 0:
			estado = "Inactivo"
		case 2:
			estado = "Egresado"
		}
		list = append(list, map[string]interface{}{
			"id":          e.ID,
			"code":        e.Code,
			"name":        e.Name,
			"national_id": e.NationalID,
			"department":  e.Department,
			"position":    e.Position,
			"salary":      money(e.Salary),
			"frequency":   e.Frequency,
			"status":      estado,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

// hrCodigoSiguiente propone el código siguiente del empleado (E-001, E-002...).
// Se genera en el servidor y no en el navegador para que dos altas simultáneas
// no obtengan el mismo código.
func (s *Server) hrCodigoSiguiente(r *http.Request) string {
	var maxID sql.NullInt64
	_ = s.dbFor(r).QueryRow(`SELECT MAX(id) FROM hr_employee`).Scan(&maxID)
	if !maxID.Valid || maxID.Int64 == 0 {
		return "E-001"
	}
	return fmt.Sprintf("E-%03d", maxID.Int64+1)
}

func (s *Server) hrEmployeeForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":         "Añadir Empleado",
		"SuggestedCode": s.hrCodigoSiguiente(r),
		"Departments":   s.hrDepartamentos(r),
		"Positions":     s.hrPosiciones(r),
	}
	if id := r.PathValue("id"); id != "" {
		var (
			code, nid, first, last, email, phone, address, hire, freq string
			dept, pos, status                                         int64
			salary                                                    float64
		)
		err := s.dbFor(r).QueryRow(`
			SELECT code, IFNULL(national_id,''), first_name, IFNULL(last_name,''),
			       IFNULL(email,''), IFNULL(phone,''), IFNULL(address,''),
			       IFNULL(CAST(hire_date AS CHAR),''), department_id, position_id, base_salary,
			       IFNULL(pay_frequency,'quincenal'), status
			FROM hr_employee WHERE id = ?`, id).
			Scan(&code, &nid, &first, &last, &email, &phone, &address,
				&hire, &dept, &pos, &salary, &freq, &status)
		if err != nil {
			http.Error(w, "Empleado no encontrado", 404)
			return
		}
		data["Title"] = "Editar Empleado"
		data["Emp"] = map[string]interface{}{
			"ID": id, "Code": code, "NationalID": nid, "FirstName": first,
			"LastName": last, "Email": email, "Phone": phone, "Address": address,
			"HireDate": hire, "DepartmentID": dept, "PositionID": pos,
			"BaseSalary": salary, "PayFrequency": freq, "Status": status,
		}
	}
	s.render(w, r, "hr_employee_form.html", data)
}

func (s *Server) hrDepartamentos(r *http.Request) []map[string]interface{} {
	rows, err := s.dbFor(r).Query(`SELECT id, name FROM hr_department WHERE status = 1 ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			out = append(out, map[string]interface{}{"ID": id, "Name": name})
		}
	}
	return out
}

func (s *Server) hrPosiciones(r *http.Request) []map[string]interface{} {
	rows, err := s.dbFor(r).Query(`SELECT id, name FROM hr_position WHERE status = 1 ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			out = append(out, map[string]interface{}{"ID": id, "Name": name})
		}
	}
	return out
}

func (s *Server) hrEmployeeSave(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.FormValue("code"))
	first := strings.TrimSpace(r.FormValue("first_name"))
	last := strings.TrimSpace(r.FormValue("last_name"))
	nid := strings.TrimSpace(r.FormValue("national_id"))
	hire := strings.TrimSpace(r.FormValue("hire_date"))
	freq := strings.TrimSpace(r.FormValue("pay_frequency"))

	if code == "" || first == "" {
		jsonError(w, "El código y el nombre son obligatorios", 400)
		return
	}
	if !aiValidDate(hire) {
		jsonError(w, "La fecha de ingreso debe tener el formato AAAA-MM-DD", 400)
		return
	}
	if freq != "quincenal" && freq != "mensual" {
		jsonError(w, "Frecuencia de pago inválida", 400)
		return
	}
	salary, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("base_salary")), 64)
	if salary < 0 {
		jsonError(w, "El salario no puede ser negativo", 400)
		return
	}

	// La cédula identifica a la persona. Se valida su unicidad ANTES de escribir
	// para no dejar dos fichas del mismo trabajador, que es como se termina
	// pagando dos nóminas a una persona.
	if nid != "" {
		var dup int
		existingID := strings.TrimSpace(r.FormValue("id"))
		if existingID != "" {
			_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_employee WHERE national_id = ? AND id <> ?`, nid, existingID).Scan(&dup)
		} else {
			_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_employee WHERE national_id = ?`, nid).Scan(&dup)
		}
		if dup > 0 {
			jsonError(w, "Ya existe un empleado registrado con esa cédula", 400)
			return
		}
	}

	dept, _ := strconv.ParseInt(r.FormValue("department_id"), 10, 64)
	pos, _ := strconv.ParseInt(r.FormValue("position_id"), 10, 64)
	status, _ := strconv.Atoi(r.FormValue("status"))
	if status == 0 {
		status = 1
	}

	id := strings.TrimSpace(r.FormValue("id"))
	var err error
	if id == "" {
		_, err = s.dbFor(r).Exec(`
			INSERT INTO hr_employee
				(code, national_id, first_name, last_name, email, phone, address,
				 hire_date, department_id, position_id, base_salary, pay_frequency, status, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			code, nid, first, last, r.FormValue("email"), r.FormValue("phone"),
			r.FormValue("address"), hire, dept, pos, salary, freq, status, now())
	} else {
		_, err = s.dbFor(r).Exec(`
			UPDATE hr_employee SET code=?, national_id=?, first_name=?, last_name=?,
				email=?, phone=?, address=?, hire_date=?, department_id=?, position_id=?,
				base_salary=?, pay_frequency=?, status=? WHERE id=?`,
			code, nid, first, last, r.FormValue("email"), r.FormValue("phone"),
			r.FormValue("address"), hire, dept, pos, salary, freq, status, id)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.hrAudit(r, sessUserID(s.sessionFromRequest(r)), "save", "hr_employee", id, nil,
		map[string]interface{}{"code": code, "base_salary": salary, "pay_frequency": freq})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Guardado correctamente", "redirect": saveRedirect(r, "/hr/employee/list")})
}

// ---------- Corridas de nómina ----------

func (s *Server) hrPayrollList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "hr_payroll_list.html", map[string]interface{}{
		"Title": "Nómina",
	})
}

func (s *Server) hrPayrollListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_payroll_run`).Scan(&total)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_payroll_run`).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`
		SELECT id, period_from, period_to, frequency, status, employee_count,
		       total_gross, total_deductions, total_employer, total_net, created_by
		FROM hr_payroll_run ORDER BY id DESC LIMIT ? OFFSET ?`, length, start)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var (
			id                                int64
			from, to, freq, status, createdBy string
			count                             int
			gross, ded, emp, net              float64
		)
		if err := rows.Scan(&id, &from, &to, &freq, &status, &count,
			&gross, &ded, &emp, &net, &createdBy); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "period_from": from, "period_to": to, "frequency": freq,
			"status": status, "employee_count": count,
			"total_gross": money(gross), "total_deductions": money(ded),
			"total_employer": money(emp), "total_net": money(net),
			"created_by": createdBy,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

func (s *Server) hrPayrollRun(w http.ResponseWriter, r *http.Request) {
	from := r.FormValue("period_from")
	to := r.FormValue("period_to")
	freq := r.FormValue("frequency")
	if freq == "" {
		freq = "quincenal"
	}
	res, err := s.hrCreateRun(r, from, to, freq, sessUserID(s.sessionFromRequest(r)))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	// Las tasas sin verificar se devuelven con la respuesta. No se bloquea la
	// corrida: el contador puede querer ver el resultado antes de validarlas,
	// y ocultar el número hasta que alguien confirme la tasa lo haría working a
	// ciegas. Lo que sí hace el sistema es decirlo en voz alta.
	resp := map[string]interface{}{
		"status": 1, "msg": "Corrida calculada en borrador",
		"run_id": res.RunID, "employee_count": res.EmployeeCount,
		"total_gross": money(res.TotalGross), "total_deductions": money(res.TotalDeductions),
		"total_net": money(res.TotalNet), "total_employer": money(res.TotalEmployer),
	}
	if len(res.Unverified) > 0 {
		pendientes := make([]map[string]interface{}, 0, len(res.Unverified))
		for _, t := range res.Unverified {
			pendientes = append(pendientes, map[string]interface{}{
				"code": t.Code, "label": t.Label, "rate_pct": t.RatePct,
				"legal_citation": t.LegalCitation,
			})
		}
		resp["unverified_rates"] = pendientes
		resp["warning"] = "Hay tasas sin verificar. Revíselas antes de publicar: el resultado depende de cifras que nadie ha validado."
	}
	writeJSON(w, resp)
}

func (s *Server) hrPayrollView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var (
		from, to, freq, status, version, createdBy string
		count                                      int
		gross, ded, emp, net                       float64
	)
	err := s.dbFor(r).QueryRow(`
		SELECT period_from, period_to, frequency, status, employee_count,
		       total_gross, total_deductions, total_employer, total_net,
		       tax_rate_version, created_by
		FROM hr_payroll_run WHERE id = ?`, id).
		Scan(&from, &to, &freq, &status, &count, &gross, &ded, &emp, &net, &version, &createdBy)
	if err != nil {
		http.Error(w, "Corrida no encontrada", 404)
		return
	}

	rows, err := s.dbFor(r).Query(`
		SELECT e.code, TRIM(CONCAT(e.first_name, ' ', e.last_name)),
		       COALESCE((SELECT SUM(amount) FROM hr_payroll_item i
		                 WHERE i.run_id = p.run_id AND i.employee_id = p.employee_id
		                   AND i.kind = 'earning'), 0),
		       p.net_pay
		FROM hr_payslip p
		JOIN hr_employee e ON e.id = p.employee_id
		WHERE p.run_id = ? ORDER BY e.code`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type slipRow struct{ Code, Name, Gross, Net string }
	slips := []slipRow{}
	for rows.Next() {
		var sr slipRow
		var g, n float64
		if rows.Scan(&sr.Code, &sr.Name, &g, &n) == nil {
			sr.Gross, sr.Net = money(g), money(n)
			slips = append(slips, sr)
		}
	}

	s.render(w, r, "hr_payroll_view.html", map[string]interface{}{
		"Title": "Corrida de Nómina",
		"Run": map[string]interface{}{
			"ID": id, "PeriodFrom": from, "PeriodTo": to, "Frequency": freq,
			"Status": status, "EmployeeCount": count, "TotalGross": money(gross),
			"TotalDeductions": money(ded), "TotalEmployer": money(emp),
			"TotalNet": money(net), "TaxRateVersion": version, "CreatedBy": createdBy,
		},
		"Slips": slips,
	})
}

func (s *Server) hrPayrollPost(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "Corrida inválida", 400)
		return
	}

	// Publicar paga dinero real e irrevierte. Se separa el permiso de "ver
	// nómina" del de "publicarla" para que un rol de consulta pueda revisar lo
	// que se va a pagar sin poder autorizarlo.
	if !s.hasPermission(r, "hr_payroll_post", "create") {
		jsonError(w, "No tiene permiso para publicar nóminas", 403)
		return
	}

	// Publicar paga dinero real. Se exige que las tasas del período estén
	// verificadas por un humano: es el último punto donde se puede detener el
	// sistema, y dejarlo pasar convierte un "no sabíamos la tasa" en un error
	// de nómina ya pagado.
	asOf, err := s.hrRunAsOf(r, runID)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	pendientes, err := s.unverifiedTaxRates(r, asOf)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if len(pendientes) > 0 {
		jsonError(w, fmt.Sprintf(
			"No se puede publicar: %d tasa(s) sin verificar (%s). Valídelas en RRHH > Tasas para poder publicar esta nómina.",
			len(pendientes), pendientes[0].Code), 409)
		return
	}

	if err := s.hrPostRun(r, runID, sessUserID(s.sessionFromRequest(r))); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Corrida publicada", "redirect": saveRedirect(r, "/hr/employee/list")})
}

func (s *Server) hrPayrollCancel(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "Corrida inválida", 400)
		return
	}

	// Cancelar una nómina ya publicada es tan grave como publicarla: deja a los
	// trabajadores sin pago registrado. Exige el mismo permiso.
	if !s.hasPermission(r, "hr_payroll_post", "delete") {
		jsonError(w, "No tiene permiso para cancelar nóminas", 403)
		return
	}
	if err := s.hrCancelRun(r, runID, sessUserID(s.sessionFromRequest(r))); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Corrida cancelada", "redirect": saveRedirect(r, "/hr/employee/list")})
}

// hrRunAsOf devuelve la fecha de referencia de una corrida, para resolver qué
// tasas le corresponden.
func (s *Server) hrRunAsOf(r *http.Request, runID int64) (string, error) {
	var periodTo sql.NullString
	if err := s.dbFor(r).QueryRow(`SELECT period_to FROM hr_payroll_run WHERE id = ?`, runID).Scan(&periodTo); err != nil {
		return "", fmt.Errorf("la corrida no existe")
	}
	if !periodTo.Valid || strings.TrimSpace(periodTo.String) == "" {
		return "", fmt.Errorf("la corrida no tiene período válido")
	}
	return periodTo.String, nil
}

// ---------- Tasas ----------// hrTaxRatePage renderiza la pantalla de tasas; los datos llegan por ajax para
// poder cambiar la fecha de vigencia sin recargar.
func (s *Server) hrTaxRatePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "hr_tax_rates.html", map[string]interface{}{
		"Title": "Tasas Fiscales",
	})
}

// hrTaxRateListData devuelve las tasas vigentes en una fecha, con su cita legal
// y su estado de verificación. Es la pantalla que cierra el círculo: el
// contador ve aquí de dónde salió cada porcentaje antes de validar la primera
// nómina.
func (s *Server) hrTaxRateListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	asOf := r.FormValue("as_of")
	if !aiValidDate(asOf) {
		asOf = today()
	}

	// effective_to es una columna DATE: "sin fecha de fin" se guarda como NULL.
	// Por eso el filtro es `IS NULL`, y no `= ''` como en SQLite.
	//
	// LAS DOS FORMAS DE COMPARAR UNA FECHA CON TEXTO SON ERROR EN MySQL, y no
	// solo una. Con `= ''` el servidor intenta convertir la cadena vacía a fecha
	// y contesta 1525. Con `= '0000-00-00'` pasa lo mismo, porque el servidor
	// lleva NO_ZERO_DATE en el sql_mode y el texto se evalúa como una fecha
	// prohibida. Se comprobó con las dos: la segunda arroja el mismo error.
	//
	// La forma correcta es `IS NULL`, que es además lo que dice el esquema
	// (`effective_to date DEFAULT NULL`).
	where := `WHERE status = 1 AND effective_from <= ?
	         AND (effective_to IS NULL OR effective_to >= ?)`
	args := []interface{}{asOf, asOf}

	search := strings.TrimSpace(r.FormValue("search[value]"))
	if search != "" {
		where += ` AND (label LIKE ? OR concept_code LIKE ?)`
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM tax_rate_table `+where, args...).Scan(&filtered); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM tax_rate_table WHERE status = 1`).Scan(&total)

	rows, err := s.dbFor(r).Query(`
		SELECT concept_code, label, rate_pct, base_kind, applies_to,
		       effective_from, IFNULL(effective_to,''), legal_citation, verified
		FROM tax_rate_table `+where+`
		ORDER BY applies_to, concept_code, effective_from`, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var (
			code, label, baseKind, appliesTo, from, to, citation string
			rate                                                 float64
			verified                                             int
		)
		if rows.Scan(&code, &label, &rate, &baseKind, &appliesTo, &from, &to, &citation, &verified) != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"code": code, "label": label, "rate_pct": rate, "base_kind": baseKind,
			"applies_to": appliesTo, "effective_from": from, "effective_to": to,
			"legal_citation": citation, "verified": verified == 1,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// hrTaxRateVerify marca una tasa como validada. Se registra quién la validó y
// cuándo, no solo el flag: la cifra que se usó en la nómina tiene que poder
// rastrearse hasta una decisión humana concreta.
func (s *Server) hrTaxRateVerify(w http.ResponseWriter, r *http.Request) {
	// Validar una tasa es el acto que desbloquea el pago de la nómina. Quien lo
	// hace está afirmando que la cifra coincide con la norma vigente, así que
	// exige su propio permiso y no el de "ver tasas".
	if !s.hasPermission(r, "hr_tax_rates", "update") {
		jsonError(w, "No tiene permiso para validar tasas", 403)
		return
	}
	code := strings.TrimSpace(r.FormValue("concept_code"))
	from := strings.TrimSpace(r.FormValue("effective_from"))
	verified := r.FormValue("verified") != "0"
	if code == "" || !aiValidDate(from) {
		jsonError(w, "Debe indicar el concepto y la fecha de vigencia", 400)
		return
	}
	res, err := s.dbFor(r).Exec(`UPDATE tax_rate_table SET verified = ? WHERE concept_code = ? AND effective_from = ?`,
		boolToInt(verified), code, from)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "No existe esa tasa vigente", 404)
		return
	}
	s.hrAudit(r, sessUserID(s.sessionFromRequest(r)), "verify_rate", "tax_rate_table", code,
		map[string]interface{}{"effective_from": from},
		map[string]interface{}{"verified": verified})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tasa actualizada", "redirect": saveRedirect(r, "/hr/employee/list")})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
