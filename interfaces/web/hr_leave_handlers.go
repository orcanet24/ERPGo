package web

// hr_leave_handlers.go — Pantallas del módulo de vacaciones.
//
// La aritmética vive en hr_leave.go y las pruebas de nómina en
// hr_leave_nomina_test.go. Aquí solo hay pantallas: un listado con lo que la
// empresa va a descontar, un formulario de registro y el botón de anular.
//
// La pantalla existe para que el módulo se use de verdad. Antes de ella el
// único camino para registrar una ausencia era SQL a mano, y una ausencia que
// nadie registra es un descuento que la nómina no hace: el módulo del recibo
// decía una cosa y el negocio pagaba otra.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerLeaveRoutes() {
	s.mux.HandleFunc("GET /hr/leave", s.hrLeavesPage)
	s.mux.HandleFunc("GET /hr/leave/data", s.hrLeavesData)
	s.mux.HandleFunc("POST /hr/leave/save", s.hrLeaveSave)
	s.mux.HandleFunc("POST /hr/leave/cancel/{id}", s.hrLeaveCancel)
}

// hrLeavesPage pinta el listado de ausencias con su formulario de registro.
//
// El desplegable trae SOLO empleados activos: dar de alta una ausencia de un
// egresado es registrar un descuento sobre alguien que ya no cobra, y la nómina
// lo ignoraría en silencio (hrBuildPayslips solo liquida activos), dejando una
// fila que parece válida y no mueve nada.
func (s *Server) hrLeavesPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT id, code,
		TRIM(CONCAT(first_name, ' ', last_name))
		FROM hr_employee WHERE status = 1 ORDER BY code`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	empleados := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var code, name string
		if rows.Scan(&id, &code, &name) == nil {
			empleados = append(empleados, map[string]interface{}{"ID": id, "Code": code, "Name": name})
		}
	}
	if err := rows.Err(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "hr_leave_list.html", map[string]interface{}{
		"Title":     "Vacaciones y ausencias",
		"Empleados": empleados,
	})
}

// hrLeavesData alimenta el DataTable.
//
// El estado se traduce a texto aquí y no en la plantilla: el JSON es la
// interfaz, y un estado que viaja como número obliga a cada consumidor a
// repetir la misma tabla de conversión.
func (s *Server) hrLeavesData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}

	var total, filtered int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM hr_leave`).Scan(&total); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	filtered = total

	// CAST AS CHAR en las fechas por la misma razón que hrLoadLeavesApproved:
	// con parseTime=true el driver convierte las columnas DATE en time.Time y
	// el JSON tendría "2027-02-05T00:00:00Z" en vez de la fecha que la pantalla
	// (y el usuario) espera.
	rows, err := s.dbFor(r).Query(`
		SELECT l.id, l.employee_id,
		       TRIM(CONCAT(e.first_name, ' ', e.last_name)),
		       l.leave_type, CAST(l.from_date AS CHAR), CAST(l.to_date AS CHAR),
		       l.days, l.paid, COALESCE(l.note,''), l.status
		FROM hr_leave l
		JOIN hr_employee e ON e.id = l.employee_id
		ORDER BY l.id DESC
		LIMIT ? OFFSET ?`, length, start)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var (
			id, empID        int64
			emp, tipo, desde string
			hasta, nota, est string
			dias             float64
			pagada, estado   int
		)
		if err := rows.Scan(&id, &empID, &emp, &tipo, &desde, &hasta,
			&dias, &pagada, &nota, &estado); err != nil {
			continue
		}
		if estado == 0 {
			est = "Anulada"
		} else {
			est = "Aprobada"
		}
		list = append(list, map[string]interface{}{
			"id":        id,
			"employee":  emp,
			"type":      tipo,
			"from_date": desde,
			"to_date":   hasta,
			"days":      dias,
			"paid":      pagada == 1,
			"note":      nota,
			"status":    est,
		})
	}
	if err := rows.Err(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list,
	})
}

// hrLeaveSave registra una ausencia.
//
// Guardar una ausencia mueve dinero de la próxima nómina, así que pide el
// permiso de editar empleados: quien puede fichar personal es quien decide
// cuándo no trabaja. Validar primero y delegar después en hrSaveLeave, que
// es donde viven las reglas (fechas, días, auditoría).
func (s *Server) hrLeaveSave(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "hr_employee", "create") {
		jsonError(w, "No tiene permiso para registrar ausencias", 403)
		return
	}
	empID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("employee_id")), 10, 64)
	if err != nil {
		jsonError(w, "Hace falta el empleado", 400)
		return
	}
	in := HRLeaveInput{
		EmployeeID: empID,
		Type:       strings.TrimSpace(r.FormValue("leave_type")),
		FromDate:   strings.TrimSpace(r.FormValue("from_date")),
		ToDate:     strings.TrimSpace(r.FormValue("to_date")),
		Paid:       r.FormValue("paid") == "1",
		Note:       strings.TrimSpace(r.FormValue("note")),
	}
	// Normalizar AQUÍ y no solo dentro de hrSaveLeave: los días los decide el
	// código, y el mensaje de la pantalla tiene que decir los de verdad, no los
	// que vinieran (o no) en el formulario.
	in, err = hrValidateLeave(in)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	id, err := s.hrSaveLeave(s.dbFor(r), in, sessUserID(s.sessionFromRequest(r)))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "id": id,
		"msg": fmt.Sprintf("Ausencia registrada (%d días): la nómina la aplicará en su período.", int(in.Days))})
}

// hrLeaveCancel anula una ausencia.
//
// Anular no es borrar (hrCancelLeave lo documenta); aquí solo se traduce el
// botón de la pantalla a esa llamada.
func (s *Server) hrLeaveCancel(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "hr_employee", "delete") {
		jsonError(w, "No tiene permiso para anular ausencias", 403)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "Ausencia inválida", 400)
		return
	}
	if err := s.hrCancelLeave(s.dbFor(r), id, sessUserID(s.sessionFromRequest(r))); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Ausencia anulada: la nómina dejará de descontarla.", "redirect": saveRedirect(r, "/hr/leave")})
}
