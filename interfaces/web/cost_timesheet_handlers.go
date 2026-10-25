package web

// cost_timesheet_handlers.go — Pantalla y API de horas por proyecto y tarea.
//
// Lo que hace es EXPONER `ResolverProyecto`, que es donde vive el criterio de qué
// hora se factura. Este fichero no decide nada: si el cálculo cambia, esta
// pantalla cambia sola. Un handler que recalcula sus propias reglas es la forma
// más corta de que la pantalla y el informe digan cosas distintas.
//
// Las rutas van en `/cost/...` porque la hoja de horas es parte del módulo de
// costos: comparte la guardia de permisos (`costGuardias`), el token CSRF y el
// estilo de las plantillas que ya existen.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// registroRutasHoras da de alta las rutas del módulo. Se llama desde
// `registerCostRoutes` para que el mapa de rutas del módulo de costos siga
// teniendo su entrada única.
func (s *Server) registroRutasHoras() {
	// Pantalla
	s.mux.HandleFunc("GET /cost/proyectos", s.costProyectosView)
	s.mux.HandleFunc("GET /cost/proyectos/{id}", s.costProyectoView)

	// Lectura
	s.mux.HandleFunc("GET /cost/proyectos/data", s.costProyectosGet)
	s.mux.HandleFunc("GET /cost/proyectos/{id}/resumen", s.costProyectoResumenGet)
	s.mux.HandleFunc("GET /cost/proyectos/{id}/tareas", s.costTareasGet)

	// Escritura
	s.mux.HandleFunc("POST /cost/proyectos/save", s.costProyectoSave)
	s.mux.HandleFunc("POST /cost/tareas/save", s.costTareaSave)
	s.mux.HandleFunc("POST /cost/horas/tarea/save", s.costHoraTareaSave)
}

// ---------------------------------------------------------------------------
// Pantallas
// ---------------------------------------------------------------------------

// costProyectosView es la pantalla resumen: una fila por proyecto con lo que se
// puede facturar y lo que no, y el motivo de lo que no.
//
// Va antes que la ruta `GET /cost/proyectos/{id}` en el mapa de rutas porque el
// patrón literal gana al de parámetro en el mux de Go 1.22, pero se registra
// primero para que quede claro en el código cuál es cuál.
func (s *Server) costProyectosView(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	proyectos, err := leerProyectos(s.dbFor(r), false)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_proyectos.html", map[string]interface{}{
		"Title":     "Horas facturables " + periodo,
		"Module":    "cost",
		"Periodo":   periodo,
		"Proyectos": proyectos,
		"Cerrado":   costoPeriodoCerrado(s.dbFor(r), periodo),
		"AceptaHoy": time.Now().Format("2006-01-02"),
	})
}

// costProyectoView es el detalle de un proyecto: sus tareas y sus renglones del
// período, cada uno con la marca de si se factura.
func (s *Server) costProyectoView(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	id, ok := costParamID(w, r, "id")
	if !ok {
		return
	}
	p, err := leerProyecto(s.dbFor(r), id)
	if err != nil {
		jsonError(w, "el proyecto no existe", 404)
		return
	}
	tareas, err := leerTareas(s.dbFor(r), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	renglones, res, err := ResolverProyecto(s.dbFor(r), periodo, p)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_proyecto.html", map[string]interface{}{
		"Title":     p.Codigo + " — " + periodo,
		"Module":    "cost",
		"Periodo":   periodo,
		"Proyecto":  p,
		"Tareas":    tareas,
		"Renglones": renglones,
		"Resumen":   res,
		"AceptaHoy": time.Now().Format("2006-01-02"),
	})
}

// ---------------------------------------------------------------------------
// Lectura
// ---------------------------------------------------------------------------

// costProyectosGet devuelve el resumen de todos los proyectos del período.
func (s *Server) costProyectosGet(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	resumenes, err := ResolverPeriodo(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Los totales van en la respuesta y no se calculan en la plantilla: la
	// pantalla tiene que mostrar lo mismo que un export, y si cada uno suma por
	// su cuenta el número cambia cuando alguien mete una fila más en el DOM.
	var registradas, facturables, noFacturables float64
	var importeFacturable, importeNoFacturable float64
	for _, r := range resumenes {
		registradas += r.HorasRegistradas
		facturables += r.HorasFacturables
		noFacturables += r.HorasNoFacturables
		importeFacturable += r.ImporteFacturable
		importeNoFacturable += r.ImporteNoFacturable
	}

	writeJSON(w, map[string]interface{}{
		"status":    1,
		"periodo":   periodo,
		"proyectos": resumenes,
		"totales": map[string]interface{}{
			"horas_registradas":     costRedondeaHoras(registradas),
			"horas_facturables":     costRedondeaHoras(facturables),
			"horas_no_facturables":  costRedondeaHoras(noFacturables),
			"importe_facturable":    importeFacturable,
			"importe_no_facturable": importeNoFacturable,
		},
	})
}

// costProyectoResumenGet devuelve el detalle de un proyecto ya resuelto.
func (s *Server) costProyectoResumenGet(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	id, ok := costParamID(w, r, "id")
	if !ok {
		return
	}
	p, err := leerProyecto(s.dbFor(r), id)
	if err != nil {
		jsonError(w, "el proyecto no existe", 404)
		return
	}
	renglones, res, err := ResolverProyecto(s.dbFor(r), periodo, p)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":    1,
		"proyecto":  p,
		"resumen":   res,
		"renglones": renglones,
	})
}

func (s *Server) costTareasGet(w http.ResponseWriter, r *http.Request) {
	id, ok := costParamID(w, r, "id")
	if !ok {
		return
	}
	tareas, err := leerTareas(s.dbFor(r), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "tareas": tareas})
}

// ---------------------------------------------------------------------------
// Escritura
// ---------------------------------------------------------------------------

func (s *Server) costProyectoSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	tarifa, err := costFormNumero(r, "hourly_rate")
	if err != nil {
		jsonError(w, "la tarifa por hora debe ser un número", 400)
		return
	}
	presupuesto, err := costFormNumero(r, "budget_hours")
	if err != nil {
		jsonError(w, "el presupuesto de horas debe ser un número", 400)
		return
	}
	cliente, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("customer_id")), 10, 64)

	p := CostProyecto{
		ID:          id,
		Codigo:      r.FormValue("code"),
		Nombre:      r.FormValue("name"),
		ClienteID:   cliente,
		Facturacion: r.FormValue("billing_type"),
		TarifaHora:  tarifa,
		Presupuesto: presupuesto,
		Status:      r.FormValue("status"),
		Notas:       r.FormValue("notes"),
	}
	if err := guardarProyecto(s.dbFor(r), p, s.usuarioActual(r)); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Proyecto guardado", "redirect": saveRedirect(r, "/cost/timesheet")})
}

func (s *Server) costTareaSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	proyectoID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("project_id")), 10, 64)
	if err != nil || proyectoID <= 0 {
		jsonError(w, "el proyecto de la tarea es obligatorio", 400)
		return
	}
	t := CostTarea{
		ProyectoID:    proyectoID,
		Codigo:        r.FormValue("code"),
		Nombre:        r.FormValue("name"),
		FacturableDef: costEsUno(r.FormValue("billable_default")),
	}
	if err := guardarTarea(s.dbFor(r), t); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tarea guardada", "redirect": saveRedirect(r, "/cost/timesheet")})
}

// costHoraTareaSave registra horas contra un proyecto y una tarea.
func (s *Server) costHoraTareaSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	empleado, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("employee_id")), 10, 64)
	if err != nil {
		jsonError(w, "el empleado es obligatorio", 400)
		return
	}
	proyectoID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("project_id")), 10, 64)
	if err != nil {
		jsonError(w, "el proyecto es obligatorio", 400)
		return
	}
	// Una tarea vacía es legal: hay horas que no son de ninguna tarea concreta.
	// ParseInt de "" da error y eso no es un fallo del usuario.
	tareaID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("task_id")), 10, 64)
	horas, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("hours")), 64)
	if err != nil {
		jsonError(w, "las horas deben ser un número", 400)
		return
	}
	ordenCambio, err := costFormNumero(r, "change_order_amount")
	if err != nil {
		jsonError(w, "la orden de cambio debe ser un número", 400)
		return
	}

	if err := guardarHoraConTarea(s.dbFor(r), empleado, r.FormValue("work_date"),
		proyectoID, tareaID, r.FormValue("grade"), strings.TrimSpace(r.FormValue("notes")),
		horas, costEsUno(r.FormValue("billable")) == 1, ordenCambio, s.usuarioActual(r)); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Horas registradas", "redirect": saveRedirect(r, "/cost/timesheet")})
}

// ---------------------------------------------------------------------------
// Utilidades
// ---------------------------------------------------------------------------

// costParamID saca un entero del PATH de la ruta.
//
// `strconv.ParseInt` de un texto vacío da error, y de un id no numérico también:
// en los dos casos se responde con el mensaje en vez de seguir con un id=0 que
// busca el proyecto número cero y dice "no existe" cuando el problema era otro.
func costParamID(w http.ResponseWriter, r *http.Request, nombre string) (int64, bool) {
	crudo := strings.TrimSpace(r.PathValue(nombre))
	id, err := strconv.ParseInt(crudo, 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, fmt.Sprintf("el %s no es válido: llegó %q", nombre, crudo), 400)
		return 0, false
	}
	return id, true
}

// costFormNumero lee un decimal del formulario. Un campo vacío es 0 y no un
// error: "presupuesto de horas" vacío significa "sin límite pactado".
func costFormNumero(r *http.Request, campo string) (float64, error) {
	v := strings.TrimSpace(r.FormValue(campo))
	if v == "" {
		return 0, nil
	}
	return strconv.ParseFloat(v, 64)
}

// costEsUno interpreta un checkbox. El valor llega como "on" desde un formulario
// y como "1" desde el fetch: se aceptan los dos porque la misma regla se aplica
// desde la pantalla y desde el código.
func costEsUno(v string) int {
	switch strings.TrimSpace(v) {
	case "1", "on", "true", "yes":
		return 1
	}
	return 0
}
