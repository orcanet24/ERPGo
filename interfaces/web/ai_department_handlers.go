package web

// ai_department_handlers.go — Pantallas de los prompts de departamento y del
// asiento contable del consumo de IA.
//
// Hasta aquí las dos funciones existían y estaban probadas, pero no había ni
// handler ni ruta: eran código que nada podía llamar. Una función de negocio
// sin pantalla no está hecha, y el síntoma es que el administrador busca dónde
// configurar el comportamiento de un área y no lo encuentra en ningún sitio.
//
// Estas pantallas son la tercera parte del sistema de departamentos. Las otras
// dos —el prompt que recibe el modelo y el candado del SQL— están en otro
// sitio; aquí solo se escribe lo que el ERP guarda.

import (
	"net/http"
	"strings"
)

func (s *Server) registerAIDepartmentRoutes() {
	s.mux.HandleFunc("GET /ai/departments", s.departmentPromptList)
	s.mux.HandleFunc("POST /ai/departments/save", s.departmentPromptSaveHandler)
	s.mux.HandleFunc("GET /ai/costs", s.aiCostView)
	s.mux.HandleFunc("POST /ai/costs/post", s.aiCostPostHandler)
}

// -----------------------------------------------------------------------------
// Prompts de departamento
// -----------------------------------------------------------------------------

func (s *Server) departmentPromptList(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	s.render(w, r, "ai_department_prompt.html", map[string]interface{}{
		"Title":   "Comportamiento por departamento",
		"Prompts": s.listDepartmentPrompts(sess),
	})
}

func (s *Server) departmentPromptSaveHandler(w http.ResponseWriter, r *http.Request) {
	dept := r.FormValue("department")
	if err := s.saveDepartmentPrompt(s.sessionFromRequest(r), dept,
		r.FormValue("prompt"), sessUserID(s.sessionFromRequest(r))); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Comportamiento guardado", "redirect": saveRedirect(r, "/ai_departments")})
}

// -----------------------------------------------------------------------------
// Asiento del consumo de IA
// -----------------------------------------------------------------------------

func (s *Server) aiCostView(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	periodo := strings.TrimSpace(r.URL.Query().Get("periodo"))
	if periodo == "" {
		periodo = aiCostPeriodoActual()
	}
	p, estado := s.aiCostStatus(sess, periodo)
	s.render(w, r, "ai_cost.html", map[string]interface{}{
		"Title":        "Coste de la IA",
		"Periodo":      p,
		"Estado":       estado,
		"PeriodoTexto": formatAICostPeriod(p.Periodo),
		"Moneda":       s.aiCostCurrency(sess),
	})
}

func (s *Server) aiCostPostHandler(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	periodo := strings.TrimSpace(r.FormValue("periodo"))
	if periodo == "" {
		periodo = aiCostPeriodoActual()
	}
	// El resumen se vuelve a leer aquí y no se toma de lo que envía el
	// formulario. El importe del asiento lo decide el ERP contando lo que
	// consumieron los agentes; si lo aceptáramos del cliente, un número
	// tecleado en la URL bastaría para mover una partida de gasto.
	p, estado := s.aiCostStatus(sess, periodo)
	if !p.Escriturado && p.Llamadas > 0 && strings.Contains(estado, "tasa") {
		jsonError(w, estado, 400)
		return
	}
	voucher, err := s.postAICost(s.requestForSession(sess), p, sessUserID(sess))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Asiento " + voucher + " posted"})
}
