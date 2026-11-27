package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// registerProductionRoutes registra las rutas HTTP de órdenes de producción.
func (s *Server) registerProductionRoutes() {
	s.mux.HandleFunc("GET /cost/production", s.costProductionView)
	s.mux.HandleFunc("GET /cost/production/data", s.costProductionData)
	s.mux.HandleFunc("GET /cost/production/{id}/lines", s.costProductionLinesView)
	s.mux.HandleFunc("GET /cost/production/{id}/lines/data", s.costProductionLinesData)
	s.mux.HandleFunc("POST /cost/production/save", s.costProductionSave)
	s.mux.HandleFunc("POST /cost/production/{id}/consume", s.costProductionConsume)
	s.mux.HandleFunc("POST /cost/production/{id}/close", s.costProductionClose)
}

// costProductionView pinta la lista de órdenes de producción.
func (s *Server) costProductionView(w http.ResponseWriter, r *http.Request) {
	ordenes, err := listarOrdenes(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_production.html", map[string]interface{}{
		"Title":   "Órdenes de producción",
		"Module":  "cost",
		"Ordenes": ordenes,
		"Hoy":     time.Now().Format("2006-01-02"),
	})
}

// costProductionData devuelve las órdenes en JSON.
func (s *Server) costProductionData(w http.ResponseWriter, r *http.Request) {
	ordenes, err := listarOrdenes(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if ordenes == nil {
		ordenes = []ProductionOrder{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": ordenes})
}

// costProductionSave crea una orden de producción desde la receta activa.
func (s *Server) costProductionSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	quantity, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("quantity")), 64)
	o := ProductionOrder{
		Code:      strings.TrimSpace(r.FormValue("code")),
		ProductID: strings.TrimSpace(r.FormValue("product_id")),
		Quantity:  quantity,
		Periodo:   strings.TrimSpace(r.FormValue("period")),
	}
	id, err := s.crearOrdenDesdeReceta(s.dbFor(r), o, s.usuarioActual(r), now())
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Orden creada", "id": id,
		"redirect": saveRedirect(r, "/cost/production"),
	})
}

// costProductionConsume registra el consumo real de una línea.
func (s *Server) costProductionConsume(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	orderID, ok := costEntero(w, r, "order_id")
	if !ok {
		return
	}
	lineaID, ok := costEntero(w, r, "linea_id")
	if !ok {
		return
	}
	actualQty, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("actual_qty")), 64)
	if err != nil {
		jsonError(w, "la cantidad real debe ser un número", 400)
		return
	}
	if err := s.registrarConsumoReal(s.dbFor(r), orderID, lineaID, actualQty,
		s.usuarioActual(r), now()); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Consumo registrado"})
}

// costProductionClose cierra la orden y calcula el costo real.
func (s *Server) costProductionClose(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	orderID, ok := costEntero(w, r, "order_id")
	if !ok {
		return
	}
	costoUnitario, err := s.cerrarOrdenProduccion(s.dbFor(r), orderID,
		s.usuarioActual(r), now())
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":         1,
		"msg":            "Orden cerrada",
		"costo_unitario": costoUnitario,
		"redirect":       saveRedirect(r, "/cost/production"),
	})
}

// costProductionLinesView pinta las líneas de una orden.
func (s *Server) costProductionLinesView(w http.ResponseWriter, r *http.Request) {
	orderID, ok := costEntero(w, r, "id")
	if !ok {
		return
	}
	lineas, err := listarLineasOrden(s.dbFor(r), orderID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": lineas})
}

// costProductionLinesData devuelve las líneas en JSON (alias para compatibilidad).
func (s *Server) costProductionLinesData(w http.ResponseWriter, r *http.Request) {
	s.costProductionLinesView(w, r)
}
