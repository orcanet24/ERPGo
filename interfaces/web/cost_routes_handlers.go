package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// registerRoutesRutas registra las rutas HTTP de rutas de proceso.
// Se llama desde registerAssetsRoutes (junto con activos fijos) o desde
// registerCostRoutes. Aquí lo hace registerAssetsRoutes para que quede bajo
// el mismo grupo de costos.
func (s *Server) registerRoutesRutas() {
	s.mux.HandleFunc("GET /cost/routes", s.costRoutesView)
	s.mux.HandleFunc("GET /cost/routes/data", s.costRoutesData)
	s.mux.HandleFunc("POST /cost/routes/save", s.costRouteSave)
	s.mux.HandleFunc("POST /cost/routes/delete", s.costRouteDelete)
}

// costRoutesView pinta la lista de rutas de proceso de un producto.
func (s *Server) costRoutesView(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("product_id")
	if productID == "" {
		productID = r.PathValue("id")
	}
	rutas, err := listarRutas(s.dbFor(r), productID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	centros, err := listarCentros(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	grados, err := listarGrados(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	gradosMap := map[string]float64{}
	for _, g := range grados {
		gradosMap[g.Code] = g.Tasa
	}
	s.render(w, r, "cost_routes.html", map[string]interface{}{
		"Title":     "Rutas de proceso",
		"Module":    "cost",
		"ProductID": productID,
		"Rutas":     rutas,
		"Centros":   centros,
		"Grados":    grados,
		"GradosMap": gradosMap,
		"Hoy":       time.Now().Format("2006-01-02"),
	})
}

// costRoutesData devuelve las rutas en JSON.
func (s *Server) costRoutesData(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("product_id")
	rutas, err := listarRutas(s.dbFor(r), productID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if rutas == nil {
		rutas = []ProcessRoute{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": rutas})
}

// costRouteSave guarda una estación de la ruta de proceso.
func (s *Server) costRouteSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	sequence, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("sequence")))
	setupMin, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("setup_time_min")), 64)
	runSec, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("run_time_sec")), 64)
	machineSec, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("machine_time_sec")), 64)
	centroID, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("work_center_id")))

	rt := ProcessRoute{
		ProductID:      strings.TrimSpace(r.FormValue("product_id")),
		Sequence:       sequence,
		WorkCenterID:   centroID,
		SetupTimeMin:   setupMin,
		RunTimeSec:     runSec,
		MachineTimeSec: machineSec,
		LaborGrade:     strings.TrimSpace(r.FormValue("labor_grade")),
		Active:         costEnteroForm(r, "active"),
	}
	if err := guardarRuta(s.dbFor(r), rt, s.usuarioActual(r)); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Ruta guardada",
		"redirect": saveRedirect(r, "/cost/routes"),
	})
}

// costRouteDelete borra una estación de la ruta.
func (s *Server) costRouteDelete(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	id, ok := costEntero(w, r, "id")
	if !ok {
		return
	}
	if err := borrarRuta(s.dbFor(r), id); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "Estación de ruta borrada",
		"redirect": saveRedirect(r, "/cost/routes"),
	})
}
