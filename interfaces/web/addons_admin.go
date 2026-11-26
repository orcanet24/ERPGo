package web

// addons_admin.go — El panel maestro de addons (Configuración → Módulos →
// Addons externos). Vive aquí y no en addons.go porque es *administración*,
// no frontera: lo que hace es leer el estado y ordenar operaciones soportadas
// por la frontera (re-escanear, reiniciar). Las rutas caen bajo /module, que
// ya es masterOnlyRoutes: la puerta del middleware es la misma que el resto
// de la administración del sistema, sin reglas gemelas que se desertan.

import (
	"fmt"
	"net/http"
)

// registerAddonAdminRoutes registra las rutas de administración de addons.
// Se llama desde registerModuleRoutes para que el panel de módulos y el de
// addons vivan bajo el mismo prefijo.
func (s *Server) registerAddonAdminRoutes() {
	s.mux.HandleFunc("GET /module/addons", s.addonPanel)
	s.mux.HandleFunc("GET /module/addons/data", s.addonPanelData)
	s.mux.HandleFunc("POST /module/addons/rescan", s.addonPanelReescanear)
	s.mux.HandleFunc("POST /module/addons/scaffold", s.addonPanelScaffold)
	s.mux.HandleFunc("POST /module/addons/{id}/restart", s.addonPanelReiniciar)
	s.mux.HandleFunc("POST /module/addons/puertos", s.addonPanelPuertos)
}

// addonPanel muestra el estado de la frontera de addons.
func (s *Server) addonPanel(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "module_addons.html", map[string]interface{}{
		"Title": "Addons externos",
	})
}

// addonPanelData es el JSON que consume el panel.
func (s *Server) addonPanelData(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.addonsEstado())
}

// addonPanelReescanear rele el estado de la carpeta addons/ y lo aplica. Los
// addons nuevos se sirven en caliente por /mod/<id>/: el núcleo registra ese
// prefijo una sola vez y despacha por id, así que no hace falta reiniciar el
// ERP. El mensaje lo dice tal cual para que el dueño no busque un reinicio que
// no necesita.
func (s *Server) addonPanelReescanear(w http.ResponseWriter, r *http.Request) {
	if s.addons == nil {
		jsonError(w, "No hay carpeta de addons en este arranque. Créela junto al binario y reinicie el ERP.", http.StatusNotFound)
		return
	}
	if err := s.addons.reescanear(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Addons re-escaneados. Los nuevos se sirven en /mod/<id>/ sin reiniciar el ERP.", "estado": s.addonsEstado()})
}

// addonPanelReiniciar relanza el proceso hijo de un addon: la operación
// soportada cuando un addon está caído, sin tocar al resto del ERP.
func (s *Server) addonPanelReiniciar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.addons == nil {
		jsonError(w, "No hay carpeta de addons en este arranque.", http.StatusNotFound)
		return
	}
	s.addons.mu.Lock()
	p := s.addons.progs[id]
	s.addons.mu.Unlock()
	if p == nil {
		jsonError(w, "No hay un addon con id "+id+" cargado en este arranque (re-escanea para verlo).", http.StatusNotFound)
		return
	}
	// Un módulo apagado no se reinicia: sería resucitar lo que el master
	// apagó. Primero se enciende; el toggle arranca el proceso.
	if !s.addons.addonActivoEnBD(id) {
		jsonError(w, "El módulo está apagado. Enciéndalo en Configuración → Módulos antes de reiniciarlo.", http.StatusConflict)
		return
	}
	// Si el proceso estaba vivo, muere con orden: un reinicio es un relevo,
	// no dos addons escuchando el mismo puerto.
	s.addons.mu.Lock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		p.cmd = nil
	}
	s.addons.mu.Unlock()
	s.addons.lanzarOReanudar(p)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Addon relanzado.", "estado": s.addonsEstado()})
}

// addonPanelPuertos reasigna puertos: mata CADA addon vivo y lo relanza, y el
// lanzamiento reserva puerto loopback nuevo (lanzarBajoCandado). Sirve cuando
// un puerto quedó enredado (proceso huérfano, proxy atascado): el dueño no
// tiene que matar procesos a mano ni reiniciar el ERP. El addon que ya estaba
// caído recibe otro intento de lanzamiento; el apagado en todas las empresas
// no se resucita.
func (s *Server) addonPanelPuertos(w http.ResponseWriter, r *http.Request) {
	if s.addons == nil {
		jsonError(w, "No hay carpeta de addons en este arranque.", http.StatusNotFound)
		return
	}
	s.addons.mu.Lock()
	var lista []*addonProc
	for _, p := range s.addons.progs {
		lista = append(lista, p)
	}
	s.addons.mu.Unlock()
	relanzados := 0
	for _, p := range lista {
		if !s.addons.addonActivoEnBD(p.manifiesto.ID) {
			s.addons.detener(p)
			continue
		}
		s.addons.mu.Lock()
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			p.cmd = nil
		}
		s.addons.mu.Unlock()
		s.addons.lanzarOReanudar(p)
		relanzados++
	}
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    fmt.Sprintf("Puertos reasignados: %d addon(es) relanzado(s) en un puerto loopback nuevo; los apagados no se tocan.", relanzados),
		"estado": s.addonsEstado(),
	})
}
