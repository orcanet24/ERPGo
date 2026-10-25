package web

// view_pref_handlers.go — Endpoints de las vistas por usuario.
//
// Tres llamadas y ninguna más:
//
//	GET  /user/view-pref/{vista}   lee la vista activa y las plantillas guardadas
//	POST /user/view-pref/{vista}   guarda la vista activa o una plantilla con nombre
//	POST /user/view-pref/{vista}   con action=delete borra una plantilla
//
// El POST lleva el token CSRF como cualquier otro del sistema (ver csrf.go), así
// que un sitio ajeno no puede guardar vistas en nombre de quien está conectado.
//
// Estas rutas cuelgan de /user/, no de /api/: la exención de CSRF de /api/
// está justificada porque un JSON con CORS no se puede falsificar desde otro
// origen. Una preferencia de pantalla no es eso, y no tiene por qué quedar
// fuera de la protección.

import (
	"net/http"
	"strings"
)

// registerViewPrefRoutes expone las vistas por usuario.
func (s *Server) registerViewPrefRoutes() {
	s.mux.HandleFunc("GET /user/view-pref/{view}", s.viewPrefLoad)
	s.mux.HandleFunc("POST /user/view-pref/{view}", s.viewPrefSave)
}

// viewPrefLoad devuelve la vista activa del usuario y sus plantillas.
//
// Con ?name= se pide una plantilla concreta en vez de la activa: así el selector
// de "vistas guardadas" puede traer el contenido de la que se elige sin
// segunda llamada ni endpoint aparte.
func (s *Server) viewPrefLoad(w http.ResponseWriter, r *http.Request) {
	viewKey, err := ValidViewKey(r.PathValue("view"))
	if err != nil {
		jsonError(w, "Pantalla desconocida", http.StatusBadRequest)
		return
	}
	name, err := ValidViewName(r.URL.Query().Get("name"))
	if err != nil {
		jsonError(w, "El nombre de la vista no es valido", http.StatusBadRequest)
		return
	}
	sess := s.sessionFromRequest(r)
	username := sessUserID(sess)

	current, found, err := LoadViewPref(s.dbFor(r), username, viewKey, name)
	if err != nil {
		jsonError(w, "No se pudo leer la vista guardada", http.StatusInternalServerError)
		return
	}
	saved, err := ListSavedViewPrefs(s.dbFor(r), username, viewKey)
	if err != nil {
		jsonError(w, "No se pudieron leer las plantillas guardadas", http.StatusInternalServerError)
		return
	}

	payload := map[string]interface{}{}
	if found {
		payload["prefs"] = current.Payload
		payload["updated_at"] = current.UpdatedAt
	}
	names := make([]string, 0, len(saved))
	for _, v := range saved {
		names = append(names, v.Name)
	}
	payload["view"] = viewKey
	payload["saved"] = names
	writeJSON(w, payload)
}

// viewPrefSave guarda o borra una vista.
//
// action decide la intención: "delete" borra la plantilla indicada por name; en
// cualquier otro caso se guarda. Se distingue por un campo explícito y no por
// la presencia de payload porque borrar no lleva payload, y porque adivinar la
// intención desde los datos es la forma habitual de que un endpoint haga algo
// distinto de lo que el usuario cree.
func (s *Server) viewPrefSave(w http.ResponseWriter, r *http.Request) {
	viewKey, err := ValidViewKey(r.PathValue("view"))
	if err != nil {
		jsonError(w, "Pantalla desconocida", http.StatusBadRequest)
		return
	}
	name, err := ValidViewName(r.FormValue("name"))
	if err != nil {
		jsonError(w, "El nombre de la vista no es valido", http.StatusBadRequest)
		return
	}
	username := sessUserID(s.sessionFromRequest(r))

	if strings.EqualFold(r.FormValue("action"), "delete") {
		// La vista activa no se borra: se sustituye por la de fábrica. Borrarla
		// dejaría al usuario sin una vista válida y la pantalla tendría que
		// inventar una; resetearla es lo que significa "volver al inicio".
		if name == "" {
			jsonError(w, "La vista activa no se borra: guarde otra para reemplazarla", http.StatusBadRequest)
			return
		}
		if err := DeleteViewPref(s.dbFor(r), username, viewKey, name); err != nil {
			jsonError(w, "La plantilla no existe", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Plantilla eliminada", "view": viewKey})
		return
	}

	// El viaje es form-encoded y no JSON: el mismo token de CSRF ya viaja en
	// la cabecera y añadir un cuerpo JSON solo daría una segunda forma de
	// llamar a lo mismo.
	payload := r.FormValue("payload")
	// La apariencia se guarda como una vista más, pero además deja una
	// cookie: el login y el selector de empresa se pintan sin sesión y no
	// tienen contra qué leer la tabla. Con la cookie, entrar al sistema no
	// cambia lo que alguien eligió ayer.
	if viewKey == appearanceViewKey {
		if err := s.saveAppearance(w, r, parseAppearance(payload)); err != nil {
			jsonError(w, "No se pudo guardar la apariencia", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Apariencia guardada", "view": viewKey})
		return
	}
	if err := SaveViewPref(s.dbFor(r), username, viewKey, name, payload); err != nil {
		switch err {
		case ErrViewPayloadVacio, ErrViewPayloadGrande, ErrViewNoJSON:
			jsonError(w, "La vista no es valida: "+err.Error(), http.StatusBadRequest)
		default:
			jsonError(w, "No se pudo guardar la vista", http.StatusInternalServerError)
		}
		return
	}

	msg := "Vista guardada"
	if name != "" {
		msg = "Plantilla guardada"
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg, "view": viewKey, "name": name})
}
