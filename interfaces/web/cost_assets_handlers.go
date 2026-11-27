package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// cost_assets_handlers.go — Rutas HTTP de activos fijos y depreciación.
//
// El motor ya lee la depreciación del período y la pone como compDepreciation
// en el costo del producto. Faltaba el lugar donde se dan de alta los activos
// y se cierra la depreciación del mes.

func (s *Server) registerAssetsRoutes() {
	// Pantalla
	s.mux.HandleFunc("GET /cost/assets", s.costAssetsView)

	// Datos
	s.mux.HandleFunc("GET /cost/assets/data", s.costAssetsData)

	// Escrituras
	s.mux.HandleFunc("POST /cost/assets/save", s.costAssetSave)
	s.mux.HandleFunc("POST /cost/assets/recalcular", s.costAssetRecalcular)
	s.mux.HandleFunc("POST /cost/assets/cerrar-periodo", s.costAssetsCerrarPeriodo)

	// Rutas de proceso (Fase 3)
	s.mux.HandleFunc("GET /cost/routes", s.costRoutesView)
	s.mux.HandleFunc("GET /cost/routes/data", s.costRoutesData)
	s.mux.HandleFunc("POST /cost/routes/save", s.costRouteSave)
	s.mux.HandleFunc("POST /cost/routes/delete", s.costRouteDelete)
}

// costAssetsView pinta la lista de activos fijos con su estado y valor de libro.
func (s *Server) costAssetsView(w http.ResponseWriter, r *http.Request) {
	activos, err := listarActivos(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	centros, err := listarCentros(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.render(w, r, "cost_assets.html", map[string]interface{}{
		"Title":   "Activos fijos",
		"Module":  "cost",
		"Activos": activos,
		"Centros": centros,
		"Periodo": time.Now().Format("2006-01"),
		"Hoy":     time.Now().Format("2006-01-02"),
	})
}

// costAssetsData devuelve los activos en JSON para la tabla de la pantalla.
func (s *Server) costAssetsData(w http.ResponseWriter, r *http.Request) {
	activos, err := listarActivos(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if activos == nil {
		activos = []FixedAsset{}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "rows": activos})
}

// costAssetSave da de alta o actualiza un activo fijo.
func (s *Server) costAssetSave(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	precio, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("purchase_price")), 64)
	if err != nil {
		jsonError(w, "el precio de compra debe ser un número", 400)
		return
	}
	residual, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("residual_value")), 64)
	vida, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("useful_life_months")))
	centro, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("cost_center_id")))
	isGlobal := costEnteroForm(r, "is_global")

	a := FixedAsset{
		Code:            strings.TrimSpace(r.FormValue("code")),
		Name:            strings.TrimSpace(r.FormValue("name")),
		Type:            strings.TrimSpace(r.FormValue("type")),
		IsGlobal:        isGlobal,
		ProductID:       strings.TrimSpace(r.FormValue("product_id")),
		CostCenterID:    centro,
		PurchasePrice:   precio,
		ResidualValue:   residual,
		PurchaseDate:    strings.TrimSpace(r.FormValue("purchase_date")),
		UsefulLifeMonth: vida,
		Active:          costEnteroForm(r, "active"),
	}
	if err := guardarActivo(s.dbFor(r), a, now(), s.usuarioActual(r)); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Activo guardado",
		"redirect": saveRedirect(r, "/cost/assets"),
		"cuota":    calcularCuota(precio, residual, vida),
	})
}

// costAssetRecalcular cambia precio/vida/residual/fecha de un activo con
// motivo obligatorio, que queda en audit_log.
func (s *Server) costAssetRecalcular(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	motivo := strings.TrimSpace(r.FormValue("motivo"))
	if code == "" {
		jsonError(w, "falta el código del activo", 400)
		return
	}
	if motivo == "" {
		jsonError(w, "el motivo es obligatorio", 400)
		return
	}
	if err := s.recalcularDepreciacion(s.dbFor(r), r, code, motivo,
		s.usuarioActual(r), now()); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Depreciación recalculada",
		"redirect": saveRedirect(r, "/cost/assets"),
	})
}

// costAssetsCerrarPeriodo congela la depreciación de todos los activos para
// un período. Es idempotente.
func (s *Server) costAssetsCerrarPeriodo(w http.ResponseWriter, r *http.Request) {
	if !s.costGuardias(w, r) {
		return
	}
	periodo, err := periodoValidado(r.FormValue("period"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	n, err := s.cerrarDepreciacionDelPeriodo(s.dbFor(r), periodo,
		s.usuarioActual(r), now())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    fmt.Sprintf("Depreciación de %s calculada para %d activo(s)", periodo, n),
		"n":      n,
	})
}
