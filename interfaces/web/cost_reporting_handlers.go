package web

import (
	"fmt"
	"net/http"
	"time"
)

// registerCostReportRoutes registra las rutas HTTP de informes de costos.
func (s *Server) registerCostReportRoutes() {
	s.mux.HandleFunc("GET /cost/report/cost-margin", s.costoMarginView)
	s.mux.HandleFunc("GET /cost/report/cost-margin/data", s.costoMarginData)
	s.mux.HandleFunc("GET /cost/report/cost-ventas", s.costoVentasView)
	s.mux.HandleFunc("GET /cost/report/cost-ventas/data", s.costoVentasData)
	s.mux.HandleFunc("GET /cost/report/conciliacion", s.conciliacionView)
	s.mux.HandleFunc("GET /cost/report/conciliacion/data", s.conciliacionData)
}

// costoMarginView pinta el informe de costo vs precio vs margen.
func (s *Server) costoMarginView(w http.ResponseWriter, r *http.Request) {
	listaID := strconvAtoi(r.URL.Query().Get("lista"))
	informe, err := listarCostoMargen(s.dbFor(r), listaID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_margin.html", map[string]interface{}{
		"Title":   "Costo vs precio vs margen",
		"Module":  "cost",
		"Informe": informe,
		"ListaID": listaID,
	})
}

// costoMarginData devuelve el informe en JSON.
func (s *Server) costoMarginData(w http.ResponseWriter, r *http.Request) {
	listaID := strconvAtoi(r.URL.Query().Get("lista"))
	informe, err := listarCostoMargen(s.dbFor(r), listaID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if informe == nil {
		informe = []CostoMargen{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": informe})
}

// costoVentasView pinta el informe de costo de ventas del período.
func (s *Server) costoVentasView(w http.ResponseWriter, r *http.Request) {
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = timeNowFormat()
	}
	cv, err := costoVentasDesdeSnapshot(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_ventas.html", map[string]interface{}{
		"Title":       "Costo de ventas del período",
		"Module":      "cost",
		"CostoVentas": cv,
		"Periodo":     periodo,
	})
}

// costoVentasData devuelve el costo de ventas en JSON.
func (s *Server) costoVentasData(w http.ResponseWriter, r *http.Request) {
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = timeNowFormat()
	}
	cv, err := costoVentasDesdeSnapshot(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "row": cv})
}

// conciliacionView pinta la conciliación costo vs kardex.
func (s *Server) conciliacionView(w http.ResponseWriter, r *http.Request) {
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = timeNowFormat()
	}
	conc, err := conciliarCostoKardex(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_conciliacion.html", map[string]interface{}{
		"Title":        "Conciliación costo vs kardex",
		"Module":       "cost",
		"Conciliacion": conc,
		"Periodo":      periodo,
	})
}

// conciliacionData devuelve la conciliación en JSON.
func (s *Server) conciliacionData(w http.ResponseWriter, r *http.Request) {
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = timeNowFormat()
	}
	conc, err := conciliarCostoKardex(s.dbFor(r), periodo)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "row": conc})
}

// strconvAtoi es un helper local para evitar importar strconv en cada handler.
func strconvAtoi(s string) int {
	if s == "" {
		return 0
	}
	n, _ := strconvAtoiParse(s)
	return n
}

func strconvAtoiParse(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("no es un número")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func timeNowFormat() string {
	return time.Now().Format("2006-01")
}
