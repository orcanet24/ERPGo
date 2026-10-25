package web

// coa_catalog_handlers.go — Pantalla de catálogo contable.
//
// El contador necesita tres cosas y nada más: ver qué plan tiene la empresa,
// cargar el estándar del SENIAT si quiere, y volver a su propio plan. Eso es lo
// que hay aquí.
//
// Cada acción que cambia el plan pide el permiso de Plan de Cuentas: no es un
// ajuste más, es la numeración sobre la que se apoya todo lo demás que la
// empresa registre.

import (
	"net/http"
	"strconv"
	"strings"
)

// coaCatalogPage muestra el estado del plan de cuentas y las acciones
// disponibles.
func (s *Server) coaCatalogPage(w http.ResponseWriter, r *http.Request) {
	st := InspectCOACatalog(s.dbFor(r))
	s.render(w, r, "coa_catalog.html", map[string]interface{}{
		"Title":          "Plan de Cuentas",
		"Catalog":        st.Catalog,
		"Source":         st.Source,
		"AppliedAt":      st.AppliedAt,
		"Notes":          st.Notes,
		"AccountCount":   st.AccountCount,
		"CollisionCount": st.CollisionCount,
		"Collisions":     st.Collisions,
		"OrderMissing":   st.OrderAccountsMissing,
		"IsCONASEV":      st.Catalog == COACatalogCONASEV,
	})
}

// coaCatalogApply carga el catálogo estándar del SENIAT.
//
// Es una acción deliberadamente aparte del ajuste: cargar un plan contable
// significa escribir cientos de cuentas, y eso merece un clic propio y su
// confirmación, no pasar como efecto secundario de un guardado.
func (s *Server) coaCatalogApply(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "coa", "update") {
		jsonError(w, "No tiene permiso para modificar el plan de cuentas", http.StatusForbidden)
		return
	}
	if r.FormValue("confirm") != "si" {
		jsonError(w, "Debe confirmar la carga del catálogo", http.StatusBadRequest)
		return
	}

	user := sessUserID(s.sessionFromRequest(r))
	res, err := ApplyCOACatalog(s.dbFor(r), user)
	if err != nil {
		// Se devuelve 409 y no 500: no es un fallo del sistema, es una decisión
		// que falta. El contador tiene que poder seguir trabajando con su plan.
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	st := InspectCOACatalog(s.dbFor(r))
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "Catálogo CONASEV cargado",
		"inserted": res.Inserted,
		"skipped":  res.Skipped,
		"accounts": st.AccountCount,
		"catalog":  st.Catalog,
	})
}

// coaCatalogSetOwn devuelve la empresa a su propio plan sin borrar sus cuentas.
// Marcar el catálogo como "propio" es una declaración, no una modificación: las
// cuentas que hay siguen ahí, que es justo lo que se quiere decir.
func (s *Server) coaCatalogSetOwn(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "coa", "update") {
		jsonError(w, "No tiene permiso para modificar el plan de cuentas", http.StatusForbidden)
		return
	}
	user := sessUserID(s.sessionFromRequest(r))
	if err := SetCOACatalog(s.dbFor(r), COACatalogPropio, "plan propio de la empresa", "", user); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st := InspectCOACatalog(s.dbFor(r))
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "La empresa usará su propia codificación",
		"catalog":  st.Catalog,
		"accounts": st.AccountCount,
	})
}

// coaCatalogInspect devuelve el diagnóstico en JSON, para que una pantalla de
// diagnóstico pueda mostrarlo sin recargar la página.
func (s *Server) coaCatalogInspect(w http.ResponseWriter, r *http.Request) {
	st := InspectCOACatalog(s.dbFor(r))
	writeJSON(w, map[string]interface{}{
		"catalog":         st.Catalog,
		"source":          st.Source,
		"applied_at":      st.AppliedAt,
		"accounts":        st.AccountCount,
		"collisions":      st.Collisions,
		"collision_count": st.CollisionCount,
		"order_missing":   st.OrderAccountsMissing,
	})
}

// registerCOACatalogRoutes expone el catálogo contable.
func (s *Server) registerCOACatalogRoutes() {
	s.mux.HandleFunc("GET /account/coa/catalog", s.coaCatalogPage)
	s.mux.HandleFunc("GET /account/coa/catalog/inspect", s.coaCatalogInspect)
	s.mux.HandleFunc("POST /account/coa/catalog/apply-conasev", s.coaCatalogApply)
	s.mux.HandleFunc("POST /account/coa/catalog/use-own", s.coaCatalogSetOwn)
}

// catalogLabel traduce el valor interno a algo que un contador lea sin
// translate. Se usa en plantillas y evita que cada una invente su propio texto
// para lo mismo.
func catalogLabel(catalog string) string {
	switch normalizeCatalog(catalog) {
	case COACatalogCONASEV:
		return "Catálogo CONASEV (SENIAT)"
	default:
		return "Codificación propia de la empresa"
	}
}

// catalogSummary arma el texto de una línea para el resumen de la pantalla. Se
// arma aquí y no en la plantilla para que el mismo dato se lea igual en la web
// que en el log de arranque.
func catalogSummary(st COACatalogStatus) string {
	var sb strings.Builder
	sb.WriteString(catalogLabel(st.Catalog))
	sb.WriteString(" — ")
	sb.WriteString(strconv.Itoa(st.AccountCount))
	sb.WriteString(" cuentas")
	if st.CollisionCount > 0 {
		sb.WriteString("; ")
		sb.WriteString(strconv.Itoa(st.CollisionCount))
		sb.WriteString(" códigos con nombre distinto al catálogo estándar")
	}
	if len(st.OrderAccountsMissing) > 0 {
		sb.WriteString("; ")
		sb.WriteString(strconv.Itoa(len(st.OrderAccountsMissing)))
		sb.WriteString(" cuentas de orden (0x) no cargadas")
	}
	return sb.String()
}
