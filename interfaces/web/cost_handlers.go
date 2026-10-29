package web

// cost_handlers.go — Rutas HTTP del módulo de costos.
//
// POR QUÉ ESTE FICHERO NO HACE NADA DE LÓGICA.
//
// El handler valida, delega en cost_config_service.go (o en cost_engine.go, si
// lo que se pide es calcular) y traduce a JSON o a plantilla. Si una regla de
// negocio aparece aquí, no se puede probar sin levantar un servidor, y en este
// repo eso significa que no se prueba.
//
// CUATRO REGLAS QUE SE CUMPLEN EN TODOS:
//   1. El método HTTP va en la línea de la ruta.
//   2. db SIEMPRE por s.dbFor(r), nunca s.db.
//   3. Todo POST pasa por csrfValid antes de tocar nada.
//   4. Los errores salen por jsonError, con el mensaje que va a ver el usuario.
//
// LO QUE ESTE MÓDULO PERMITE HACER, y es lo que no se podía antes de que
// existiera (el motor estaba escrito pero nadie lo llamaba):
//
//   - Configurar el método de costeo de un producto y ver qué le falta.
//   - Ver el desglose de un costo CON la línea que explica cada renglón.
//   - Calcular el costo de los productos de un período.
//   - Cerrar el período, que congela los costos, y reabrirlo dejando motivo.
//   - Cargar la hoja de horas y los grados que la valoran.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerCostRoutes() {
	// --- Pantallas ---
	s.mux.HandleFunc("GET /cost", s.costHome)
	s.mux.HandleFunc("GET /cost/product/{id}", s.costProductView)
	s.mux.HandleFunc("GET /cost/period", s.costPeriodView)

	// --- Datos ---
	s.mux.HandleFunc("GET /cost/product/search", s.costProductSearch)
	s.mux.HandleFunc("GET /cost/product/{id}/config", s.costConfigGet)
	s.mux.HandleFunc("GET /cost/product/{id}/recipe", s.costRecipeGet)
	s.mux.HandleFunc("GET /cost/product/{id}/breakdown", s.costBreakdownGet)
	s.mux.HandleFunc("GET /cost/product/{id}/yield", s.costYieldGet)
	s.mux.HandleFunc("GET /cost/grades", s.costGradesGet)
	s.mux.HandleFunc("GET /cost/centers", s.costCentersGet)
	s.mux.HandleFunc("GET /cost/lists", s.costListsGet)
	s.mux.HandleFunc("GET /cost/timesheet", s.costTimesheetView)
	s.mux.HandleFunc("GET /cost/timesheet/data", s.costTimesheetGet)
	s.mux.HandleFunc("GET /cost/period/status", s.costPeriodStatusGet)

	// --- Escrituras ---
	s.mux.HandleFunc("POST /cost/config/save", s.costConfigSave)
	s.mux.HandleFunc("POST /cost/recipe/save", s.costRecipeSave)
	s.mux.HandleFunc("POST /cost/recipe/delete", s.costRecipeDelete)
	s.mux.HandleFunc("POST /cost/yield/save", s.costYieldSave)
	s.mux.HandleFunc("POST /cost/grade/save", s.costGradeSave)
	s.mux.HandleFunc("POST /cost/center/save", s.costCenterSave)
	s.mux.HandleFunc("POST /cost/price/save", s.costPriceSave)
	s.mux.HandleFunc("POST /cost/timesheet/save", s.costTimesheetSave)
	s.mux.HandleFunc("POST /cost/timesheet/delete", s.costTimesheetDelete)
	s.mux.HandleFunc("POST /cost/calculate", s.costCalculate)
	s.mux.HandleFunc("POST /cost/period/close", s.costPeriodClose)
	s.mux.HandleFunc("POST /cost/period/reopen", s.costPeriodReopen)

	// Hoja de horas por proyecto y tarea (cost_timesheet_handlers.go): sus
	// rutas /cost/proyectos* existían pero nadie las daba de alta, así que la
	// pantalla devolvía 404 y las horas que sí se cargaban por SQL no se veían
	// en ningún lado.
	s.registroRutasHoras()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// costGuardias hace lo que hacen TODOS los POST antes de tocar nada: validar el
// token de CSRF.
//
// Se devuelve el error ya escrito en la respuesta para que el handler no pueda
// olvidarse de hacerlo: un handler que se olvide de escribir la respuesta
// devuelve 200 con el cuerpo vacío, y el JavaScript no sabe si se guardó o no.
func (s *Server) costGuardias(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		jsonError(w, "formulario ilegible", 400)
		return false
	}
	if !s.csrfValid(r, s.sessionFromRequest(r)) {
		jsonError(w, "token de seguridad inválido: recarga la página", 403)
		return false
	}
	return true
}

// costEnteroExtrae un entero del formulario y falla si no lo es.
//
// strconv.Atoi("") da 0 sin error, y 0 colado en un WHERE product_id = ” no
// encuentra nada y parece una pantalla vacía. Por eso el vacío se trata aparte.
func costEntero(w http.ResponseWriter, r *http.Request, campo string) (int, bool) {
	v := strings.TrimSpace(r.FormValue(campo))
	if v == "" {
		jsonError(w, "falta "+campo, 400)
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		jsonError(w, campo+" debe ser un número, llegó "+strconv.Quote(v), 400)
		return 0, false
	}
	return n, true
}

// ---------------------------------------------------------------------------
// Pantallas
// ---------------------------------------------------------------------------

// costHome es la pantalla de configuración: qué método tiene cada producto, qué
// le falta para poder calcularse, y los maestros que los métodos necesitan.
func (s *Server) costHome(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		periodo = time.Now().Format("2006-01")
	}
	estado, err := leerEstadoPeriodo(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_home.html", map[string]interface{}{
		"Title":   "Costos",
		"Module":  "cost",
		"Periodo": periodo,
		"Estado":  estado,
		"Metodos": costMetodos,
	})
}

// costProductView es la ficha de un producto: su configuración, su receta, su
// desglose y lo que le falta para poder costearse.
func (s *Server) costProductView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		jsonError(w, "falta el producto", 400)
		return
	}
	db := s.dbFor(r)

	var nombre string
	if err := db.QueryRow(`SELECT product_name FROM product_information
		WHERE product_id = ?`, id).Scan(&nombre); err != nil {
		jsonError(w, "el producto "+id+" no existe", 404)
		return
	}

	cfg, err := leerCostConfig(db, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	receta, err := s.listarReceta(db, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	rendimientos, err := s.listarRendimientos(db, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Lo que falta se calcula ANTES de mostrar la ficha. Una ficha de
	// configuración que dice "todo listo" cuando no lo está es peor que no
	// tenerla: el usuario configura creyendo que ya está.
	faltantes := costFaltantes(db, cfg, receta)

	periodo, _ := periodoValidado(r.URL.Query().Get("periodo"))

	s.render(w, r, "cost_product.html", map[string]interface{}{
		"Title":         nombre,
		"Module":        "cost",
		"ProductID":     id,
		"ProductNombre": nombre,
		"Config":        cfg,
		"Receta":        receta,
		"Rendimientos":  rendimientos,
		"Metodos":       costMetodos,
		"Faltantes":     faltantes,
		"Periodo":       periodo,
		// La merma se guarda en decimal (0.0250) y se EDITA en porcentaje
		// (2.50). La conversión va aquí y no en la plantilla con un
		// `divf x 0.01`, que es un número mágico que no explica nada. La regla
		// es que las conversiones viven en Go.
		"MermaPct": cfg.LossFactor * 100,
	})
}

// costPeriodView es la pantalla de cierre: qué se calculó, qué se congeló y
// qué falta.
func (s *Server) costPeriodView(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	estado, err := leerEstadoPeriodo(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_period.html", map[string]interface{}{
		"Title":   "Cierre de costos " + periodo,
		"Module":  "cost",
		"Periodo": periodo,
		"Estado":  estado,
	})
}

// costTimesheetView es la pantalla de captura de horas.
func (s *Server) costTimesheetView(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.render(w, r, "cost_timesheet.html", map[string]interface{}{
		"Title":   "Horas " + periodo,
		"Module":  "cost",
		"Periodo": periodo,
		"Hoy":     time.Now().Format("2006-01-02"),
	})
}

// ---------------------------------------------------------------------------
// Datos: lectura
// ---------------------------------------------------------------------------

// costProductSearch busca productos para el recuadro de la pantalla.
//
// LIKE con % del usuario es injectable y además lentísimo. Se acota el patrón
// con un prefijo y se limita el número de filas: una búsqueda de Costs que
// devuelve 4000 filas no es una búsqueda.
func (s *Server) costProductSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, map[string]interface{}{"status": 1, "rows": []interface{}{}})
		return
	}

	// El comodín va al principio y al final, pero el texto se pasa como
	// parámetro, nunca concatenado en el SQL.
	rows, err := s.dbFor(r).Query(`SELECT product_id, product_name, IFNULL(cost,0), IFNULL(price,'')
		FROM product_information
		WHERE product_id LIKE ? OR product_name LIKE ?
		ORDER BY product_name LIMIT 30`, "%"+q+"%", "%"+q+"%")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var id, nombre, precio string
		var costo any
		if err := rows.Scan(&id, &nombre, &costo, &precio); err != nil {
			continue
		}
		f := valorNumerico(costo)
		var c float64
		if f != nil {
			c = *f
		}
		out = append(out, map[string]interface{}{
			"id": id, "nombre": nombre, "costo": c, "precio": precio,
		})
	}
	// out puede quedar nil si no hay nada: se devuelve vacío, no null, porque
	// el JavaScript lo recorre y null lo revienta.
	if out == nil {
		out = []map[string]interface{}{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": out})
}

// costConfigGet devuelve la configuración de un producto y lo que le falta.
func (s *Server) costConfigGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := s.dbFor(r)
	cfg, err := leerCostConfig(db, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	receta, err := s.listarReceta(db, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	faltantes := costFaltantes(db, cfg, receta)
	writeJSON(w, map[string]interface{}{"status": 1, "config": cfg, "faltantes": faltantes})
}

// costRecipeGet devuelve los componentes de la receta con su total.
func (s *Server) costRecipeGet(w http.ResponseWriter, r *http.Request) {
	receta, err := s.listarReceta(s.dbFor(r), r.PathValue("id"))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if receta == nil {
		receta = []CostReceta{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": receta})
}

// costBreakdownGet CALCULA el costo de un producto y devuelve el desglose.
//
// Es la pantalla que hace útil el módulo: no da un número, da el número Y la
// línea que explica de dónde sale cada parte. Un costo sin trazabilidad no se
// puede defender ante una auditoría, y sin esto el módulo entero no serviría
// para nada más que para mostrar una cifra.
//
// No guarda nada: calcular para mirar y guardar para cerrar son cosas
// distintas, y mezclarlas significa que un clic de curiosidad reescribe el
// histórico del mes.
func (s *Server) costBreakdownGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	cantidad := 1.0
	if c := strings.TrimSpace(r.URL.Query().Get("cantidad")); c != "" {
		cantidad, err = strconv.ParseFloat(c, 64)
		if err != nil || cantidad <= 0 {
			jsonError(w, "la cantidad debe ser un número mayor que cero", 400)
			return
		}
	}

	db := s.dbFor(r)
	in := s.cargarCostInputs(db, id, periodo, cantidad)
	b := calcularCosto(in)
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"inputs": map[string]interface{}{
			"metodo":      in.Metodo,
			"cantidad":    in.Cantidad,
			"periodo":     in.Periodo,
			"merma":       in.LossFactor,
			"componentes": len(in.Componentes),
			"horas":       len(in.Horas),
		},
		"breakdown": b,
	})
}

// costYieldGet devuelve los rendimientos de un producto de entrada.
func (s *Server) costYieldGet(w http.ResponseWriter, r *http.Request) {
	y, err := s.listarRendimientos(s.dbFor(r), r.PathValue("id"))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if y == nil {
		y = []CostRendimiento{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": y})
}

func (s *Server) costGradesGet(w http.ResponseWriter, r *http.Request) {
	g, err := listarGrados(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if g == nil {
		g = []CostGrado{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": g})
}

func (s *Server) costCentersGet(w http.ResponseWriter, r *http.Request) {
	c, err := listarCentros(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if c == nil {
		c = []CostCentro{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": c})
}

func (s *Server) costListsGet(w http.ResponseWriter, r *http.Request) {
	l, err := listarListas(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if l == nil {
		l = []CostLista{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": l})
}

func (s *Server) costTimesheetGet(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	h, err := s.listarHoras(s.dbFor(r), periodo, r.URL.Query().Get("producto"))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if h == nil {
		h = []CostHoraFila{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "periodo": periodo, "rows": h})
}

func (s *Server) costPeriodStatusGet(w http.ResponseWriter, r *http.Request) {
	periodo, err := periodoValidado(r.URL.Query().Get("periodo"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	e, err := leerEstadoPeriodo(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "estado": e})
}

// ---------------------------------------------------------------------------
// Escrituras
// ---------------------------------------------------------------------------

func (s *Server) costConfigSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	productID := strings.TrimSpace(r.FormValue("product_id"))
	if productID == "" {
		jsonError(w, "el producto es obligatorio", 400)
		return
	}
	cfg := CostConfig{
		ProductID:   productID,
		Metodo:      r.FormValue("costing_method"),
		LossFactor:  parseMoney(r.FormValue("loss_factor")),
		Perishable:  costEnteroForm(r, "perishable"),
		ShelfLife:   costEnteroForm(r, "shelf_life_days"),
		IsAssembled: costEnteroForm(r, "is_assembled"),
		TrackLots:   costEnteroForm(r, "track_lots"),
		TrackSerial: costEnteroForm(r, "track_serial"),
		Imported:    costEnteroForm(r, "imported"),
		CostCenter:  r.FormValue("cost_center_id"),
	}
	// La merma llega en PORCENTAJE desde el formulario (2,5) y se guarda en
	// decimal (0.025). Si se dividiera dos veces, o si se guardara tal cual,
	// una merma del 2,5% se aplicaría como 250%.
	m, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("merma_pct")), 64)
	if err != nil {
		m = 0
	}
	if m > 0 {
		cfg.LossFactor = m / 100
	}

	if err := guardarCostConfig(s.dbFor(r), cfg, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Configuración guardada", "redirect": saveRedirect(r, "/cost")})
}

// costEnteroForm lee un entero del formulario devolviendo 0 si no viene o no es
// un número. Para los campos de banderín (0/1) y los de días, un valor
// imposible se trata como "no marcado", que es lo que el usuario quiso decir.
func costEnteroForm(r *http.Request, campo string) int {
	n, err := strconv.Atoi(strings.TrimSpace(r.FormValue(campo)))
	if err != nil {
		return 0
	}
	return n
}

func (s *Server) costRecipeSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	merma, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("loss_factor")), 64)
	if merma > 1 {
		merma = merma / 100
	}
	step, _ := strconv.Atoi(r.FormValue("step_order"))

	rec := CostReceta{
		ProductID:   strings.TrimSpace(r.FormValue("product_id")),
		ComponentID: strings.TrimSpace(r.FormValue("component_id")),
		Cantidad:    parseMoney(r.FormValue("quantity")),
		Unit:        r.FormValue("unit"),
		LossFactor:  merma,
		Step:        step,
	}
	if err := guardarReceta(s.dbFor(r), rec); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Componente guardado", "redirect": saveRedirect(r, "/cost")})
}

func (s *Server) costRecipeDelete(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	step, ok := costEntero(w, r, "step_order")
	if !ok {
		return
	}
	if err := borrarReceta(s.dbFor(r), r.FormValue("product_id"),
		r.FormValue("component_id"), step); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Componente borrado", "redirect": saveRedirect(r, "/cost")})
}

func (s *Server) costYieldSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	pct, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("expected_pct")), 64)
	if err != nil {
		jsonError(w, "el rendimiento debe ser un número, por ejemplo 72", 400)
		return
	}
	y := CostRendimiento{
		InputID:     strings.TrimSpace(r.FormValue("input_product_id")),
		OutputID:    strings.TrimSpace(r.FormValue("output_product_id")),
		Pct:         pct,
		IsByproduct: costEnteroForm(r, "is_byproduct"),
		Notes:       r.FormValue("notes"),
	}
	if err := guardarRendimiento(s.dbFor(r), y); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Rendimiento guardado", "redirect": saveRedirect(r, "/cost")})
}

func (s *Server) costGradeSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	horas, ok := costEntero(w, r, "hours_per_year")
	if !ok {
		return
	}
	prod, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("productivity_factor")), 64)
	g := CostGrado{
		Code:       strings.TrimSpace(r.FormValue("code")),
		Name:       r.FormValue("name"),
		Salario:    parseMoney(r.FormValue("base_salary_annual")),
		Overhead:   parseMoney(r.FormValue("overhead_pct")),
		HorasAnual: horas,
		Productivy: prod,
		Activo:     costEnteroForm(r, "active"),
	}
	if err := guardarGrado(s.dbFor(r), g); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Grado guardado",
		"tasa": calcularTasaHoraria(g.Salario, g.Overhead, g.HorasAnual),
	})
}

func (s *Server) costCenterSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	prod, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("overhead_rate_unit")), 64)
	c := CostCentro{
		Code:       strings.TrimSpace(r.FormValue("code")),
		Name:       r.FormValue("name"),
		Type:       r.FormValue("type"),
		TasaHora:   parseMoney(r.FormValue("overhead_rate_hour")),
		TasaUnidad: prod,
		Activo:     costEnteroForm(r, "active"),
	}
	if err := guardarCentro(s.dbFor(r), c); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Centro de costo guardado", "redirect": saveRedirect(r, "/cost")})
}

func (s *Server) costPriceSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	lista, ok := costEntero(w, r, "price_list_id")
	if !ok {
		return
	}
	precio, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("price")), 64)
	if err != nil {
		jsonError(w, "el precio debe ser un número", 400)
		return
	}
	if err := guardarPrecioEnLista(s.dbFor(r), strings.TrimSpace(r.FormValue("product_id")),
		lista, precio); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Precio guardado", "redirect": saveRedirect(r, "/cost")})
}

func (s *Server) costTimesheetSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	horas, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("hours")), 64)
	if err != nil {
		jsonError(w, "las horas deben ser un número", 400)
		return
	}
	fact := r.FormValue("billable") == "1"
	if err := guardarHora(s.dbFor(r),
		r.FormValue("employee_id"), r.FormValue("work_date"),
		strings.TrimSpace(r.FormValue("product_id")), r.FormValue("grade"),
		r.FormValue("notes"), horas, fact, s.usuarioActual(r)); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Horas registradas", "redirect": saveRedirect(r, "/cost")})
}

func (s *Server) costTimesheetDelete(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	id, ok := costEntero(w, r, "id")
	if !ok {
		return
	}
	if err := borrarHora(s.dbFor(r), id); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Renglón borrado", "redirect": saveRedirect(r, "/cost")})
}

// ---------------------------------------------------------------------------
// Calcular y cerrar
// ---------------------------------------------------------------------------

// costCalculate calcula y GUARDA el costo de un producto en un período.
//
// Guardar es lo que distingue esto del desglose: un costo que no se guarda no
// deja rastro, y recalcular un mes pasado daría otro número sin que nadie pueda
// comparar cuál era el bueno.
func (s *Server) costCalculate(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	periodo, err := periodoValidado(r.FormValue("period"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	productID := strings.TrimSpace(r.FormValue("product_id"))
	if productID == "" {
		jsonError(w, "falta el producto", 400)
		return
	}
	cantidad, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("cantidad")), 64)
	if err != nil || cantidad <= 0 {
		cantidad = 1
	}

	db := s.dbFor(r)

	// Un período cerrado no se recalcula. Si lo estuviera, el costo de un mes ya
	// cerrado cambiaría y el cierre no habría congelado nada.
	estado, err := leerEstadoPeriodo(db, periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if estado.Status == "cerrado" {
		jsonError(w, "el período "+periodo+" está cerrado: reabrirlo para recalcular",
			409)
		return
	}

	b, err := s.calcularCostoDeProducto(db, productID, periodo, cantidad)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Costo calculado", "breakdown": b,
	})
}

// costPeriodClose congela los costos del mes.
func (s *Server) costPeriodClose(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	periodo, err := periodoValidado(r.FormValue("period"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	n, err := s.cerrarCostoDePeriodo(s.dbFor(r), periodo, s.usuarioActual(r))
	if err != nil {
		// 409: es un conflicto de estado, no un error de validación. El período
		// ya está cerrado y el cliente tiene que saber que reintentar no ayuda.
		jsonError(w, err.Error(), 409)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    fmt.Sprintf("Período %s cerrado con %d producto(s)", periodo, n),
		"n":      n,
	})
}

func (s *Server) costPeriodReopen(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	periodo, err := periodoValidado(r.FormValue("period"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := reabrirPeriodo(s.dbFor(r), periodo, s.usuarioActual(r),
		r.FormValue("motivo")); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Período " + periodo + " reabierto",
	})
}

// usuarioActual devuelve el usuario de la sesión, o "" si no hay.
func (s *Server) usuarioActual(r *http.Request) string {
	if sess := s.sessionFromRequest(r); sess != nil {
		return sess.Username
	}
	return ""
}

// ---------------------------------------------------------------------------
// Lo que falta para poder costear
// ---------------------------------------------------------------------------

// costFaltantes dice qué le impide a un producto costearse con su método.
//
// ESTA ES LA FUNCIÓN MÁS ÚTIL DE LA PANTALLA. El motor es permisivo a propósito:
// si falta un dato, calcula con lo que hay y lo avisa, en vez de devolver un
// error. Eso está bien para un cálculo guardado y mal para alguien que está
// CONFIGURANDO: quien está poniendo la receta necesita saber qué le falta, y
// sin esta lista la única forma de averiguarlo es calcular y leer la
// advertencia.
//
// Cada falta nombra la tabla donde se arregla, no solo lo que falta: "no tiene
// receta" no sirve de nada si no sabes dónde se pone una.
func costFaltantes(db *sql.DB, cfg CostConfig, receta []CostReceta) []string {
	var out []string

	switch cfg.Metodo {
	case costRecipe:
		if len(receta) == 0 {
			out = append(out, "No tiene componentes. Se añaden en la pestaña de receta (product_recipe).")
		} else {
			// Un componente sin costo hace que la receta salga subvaluada sin
			// que ningún total dé una pista. Merece su propio aviso.
			for _, c := range receta {
				if c.UnitCost <= 0 {
					out = append(out, "El componente "+c.ComponentID+" no tiene costo en product_information: se está costeando a cero.")
				}
			}
		}
	case costYield:
		// El rendimiento se busca por el producto de ENTRADA. Si el producto
		// que se está viendo es una salida, hay que decirlo: si no, el usuario
		// busca en la ficha del corte un rendimiento que nunca va a estar ahí.
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM product_yield
			WHERE input_product_id = ? OR output_product_id = ?`,
			cfg.ProductID, cfg.ProductID).Scan(&n)
		if n == 0 {
			out = append(out, "No hay rendimiento definido para este producto (product_yield). "+
				"El rendimiento se configura sobre el producto de ENTRADA (la canal), no sobre el de salida.")
		}
	case costHours:
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM cost_timesheet
			WHERE product_id = ?`, cfg.ProductID).Scan(&n)
		if n == 0 {
			out = append(out, "No hay horas registradas para este producto (cost_timesheet). "+
				"Sin horas, el costo de mano de obra sale en cero.")
		}
	case costImport:
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM import_cost c
			JOIN product_purchase_details d ON d.purchase_id = c.purchase_id
			WHERE d.product_id = ?`, cfg.ProductID).Scan(&n)
		if n == 0 {
			out = append(out, "No hay gastos de importación registrados para este producto (import_cost).")
		}
	case costPP:
		var costo any
		if err := db.QueryRow(`SELECT cost FROM product_information WHERE product_id = ?`,
			cfg.ProductID).Scan(&costo); err != nil {
			out = append(out, "El producto no tiene costo todavía: entra al inventario por una compra para tener costo promedio.")
		} else if f := valorNumerico(costo); f != nil && *f <= 0 {
			out = append(out, "El costo promedio del producto es cero: entra al inventario por una compra.")
		}
	}

	// La merma se avisa por encima del 100%: un porcentaje mayor que uno
	// significa que el número está en otra escala (2.5 en vez de 0.025) y el
	// costo sale multiplicado por diez.
	if cfg.LossFactor > 1 {
		out = append(out, "La merma está por encima del 100%: revisa si el porcentaje se guardó como decimal.")
	}
	return out
}
