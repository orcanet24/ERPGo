package web

import (
	"net/http"
	"strconv"
	"strings"
)

// registerUomRoutes registra las rutas HTTP de unidades de medida y conversiones.
func (s *Server) registerUomRoutes() {
	s.mux.HandleFunc("GET /cost/uom", s.costUoMView)
	s.mux.HandleFunc("GET /cost/uom/data", s.costUoMData)
	s.mux.HandleFunc("POST /cost/uom/save", s.costUoMSave)
	s.mux.HandleFunc("GET /cost/uom/{id}/conversions", s.costUoMConversionsView)
	s.mux.HandleFunc("GET /cost/uom/{id}/conversions/data", s.costUoMConversionsData)
	s.mux.HandleFunc("POST /cost/uom/{id}/conversions/save", s.costUoMConversionSave)
	s.mux.HandleFunc("POST /cost/uom/{id}/conversions/delete", s.costUoMConversionDelete)
}

// costUoMView pinta la lista de unidades de medida.
func (s *Server) costUoMView(w http.ResponseWriter, r *http.Request) {
	uoms, err := listarUoM(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_uom.html", map[string]interface{}{
		"Title":  "Unidades de medida",
		"Module": "cost",
		"UoMs":   uoms,
	})
}

// costUoMData devuelve las UoM en JSON.
func (s *Server) costUoMData(w http.ResponseWriter, r *http.Request) {
	uoms, err := listarUoM(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if uoms == nil {
		uoms = []UnitOfMeasure{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": uoms})
}

// costUoMSave guarda una unidad de medida.
func (s *Server) costUoMSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	ad, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("allows_decimal")))
	u := UnitOfMeasure{
		Code:          strings.TrimSpace(r.FormValue("code")),
		Name:          strings.TrimSpace(r.FormValue("name")),
		AllowsDecimal: ad,
	}
	if err := guardarUoM(s.dbFor(r), u); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Unidad de medida guardada",
		"redirect": saveRedirect(r, "/cost/uom"),
	})
}

// costUoMConversionsView pinta las conversiones de un producto.
func (s *Server) costUoMConversionsView(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	convs, err := listarConversionUoM(s.dbFor(r), productID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_uom_conversions.html", map[string]interface{}{
		"Title":       "Conversiones de UoM - " + productID,
		"Module":      "cost",
		"ProductID":   productID,
		"Conversions": convs,
	})
}

// costUoMConversionsData devuelve las conversiones en JSON.
func (s *Server) costUoMConversionsData(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	convs, err := listarConversionUoM(s.dbFor(r), productID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if convs == nil {
		convs = []ProductUOMConversion{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": convs})
}

// costUoMConversionSave guarda una conversión de UoM.
func (s *Server) costUoMConversionSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	productID := r.PathValue("id")
	factor, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("factor")), 64)
	if err != nil {
		jsonError(w, "el factor debe ser un número", 400)
		return
	}
	c := ProductUOMConversion{
		ProductID: productID,
		FromUOM:   strings.TrimSpace(r.FormValue("from_uom")),
		ToUOM:     strings.TrimSpace(r.FormValue("to_uom")),
		Factor:    factor,
	}
	if err := guardarConversionUoM(s.dbFor(r), c); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Conversión guardada",
		"redirect": saveRedirect(r, "/cost/uom"),
	})
}

// costUoMConversionDelete borra una conversión de UoM.
func (s *Server) costUoMConversionDelete(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	id, ok := costEntero(w, r, "id")
	if !ok {
		return
	}
	if err := borrarConversionUoM(s.dbFor(r), id); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "Conversión borrada",
		"redirect": saveRedirect(r, "/cost/uom"),
	})
}
