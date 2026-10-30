package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// Module representa un módulo del sistema
type Module struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	IsCore      bool      `json:"is_core"`
	IsActive    bool      `json:"is_active"`
	Path        string    `json:"path"`
	Author      string    `json:"author"`
	LicenseType string    `json:"license_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	MenuCount   int       `json:"menu_count"`
}

// RegisterModuleRequest para registrar un nuevo módulo
type RegisterModuleRequest struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Description string      `json:"description"`
	Path        string      `json:"path"`
	Author      string      `json:"author"`
	LicenseType string      `json:"license_type"`
	Menus       []MenuEntry `json:"menus"`
}

type MenuEntry struct {
	Label     string `json:"label"`
	URL       string `json:"url"`
	Icon      string `json:"icon"`
	ParentID  *int   `json:"parent_id"`
	SortOrder int    `json:"sort_order"`
}

// registerModuleRoutes registra las rutas de gestión de módulos y addons
func (s *Server) registerModuleRoutes() {
	s.mux.HandleFunc("GET /module/list", s.moduleList)
	s.mux.HandleFunc("GET /module/list/data", s.moduleListData)
	s.mux.HandleFunc("POST /module/register", s.moduleRegister)
	s.mux.HandleFunc("PUT /module/{id}", s.moduleUpdate)
	s.mux.HandleFunc("POST /module/{id}/toggle", s.moduleToggle)
	s.mux.HandleFunc("DELETE /module/{id}", s.moduleDelete)
	// Panel maestro de addons externos (Configuración → Módulos → Addons).
	s.registerAddonAdminRoutes()
}
func (s *Server) moduleList(w http.ResponseWriter, r *http.Request) {	s.render(w, r, "module_list.html", map[string]interface{}{
		"Title": "Módulos del Sistema",
	})
}

// moduleListData devuelve los módulos en formato JSON
func (s *Server) moduleListData(w http.ResponseWriter, r *http.Request) {	rows, err := s.dbFor(r).Query(`
		SELECT m.id, m.name, m.version, m.description, m.is_core, m.is_active, 
		       m.path, m.author, m.license_type, m.created_at, m.updated_at,
		       COUNT(me.id) as menu_count
		FROM modules m
		LEFT JOIN menu me ON me.module_id = m.id
		GROUP BY m.id
		ORDER BY m.is_core DESC, m.name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var modules []map[string]interface{}
	for rows.Next() {
		var m Module
		var desc, path, author, licenseType sql.NullString
		var menuCount int
		err := rows.Scan(&m.ID, &m.Name, &m.Version, &desc, &m.IsCore, &m.IsActive,
			&path, &author, &licenseType, &m.CreatedAt, &m.UpdatedAt, &menuCount)
		if err != nil {
			continue
		}
		modules = append(modules, map[string]interface{}{
			"id":           m.ID,
			"name":         m.Name,
			"version":      m.Version,
			"description":  desc.String,
			"is_core":      m.IsCore,
			"is_active":    m.IsActive,
			"path":         path.String,
			"author":       author.String,
			"license_type": licenseType.String,
			"created_at":   m.CreatedAt,
			"updated_at":   m.UpdatedAt,
			"menu_count":   menuCount,
		})
	}
	writeJSON(w, map[string]interface{}{"data": modules})
}

// moduleRegister registra un nuevo módulo y sus menús
func (s *Server) moduleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterModuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", 400)
		return
	}

	if req.ID == "" || req.Name == "" {
		jsonError(w, "ID y nombre son obligatorios", 400)
		return
	}

	// Verificar que no exista
	var exists int
	err := s.dbFor(r).QueryRow("SELECT COUNT(*) FROM modules WHERE id = ?", req.ID).Scan(&exists)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if exists > 0 {
		jsonError(w, "El módulo ya existe", 409)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`INSERT INTO modules (id, name, version, description, is_core, is_active, path, author, license_type)
		VALUES (?, ?, ?, ?, 0, 1, ?, ?, ?)`,
		req.ID, req.Name, req.Version, req.Description, req.Path, req.Author, req.LicenseType)
	if err != nil {
		jsonError(w, "Error registrando módulo: "+err.Error(), 500)
		return
	}

	// Registrar menús
	for _, menu := range req.Menus {
		_, err = tx.Exec(`INSERT INTO menu (parent_id, label, url, module_id, is_core, icon, sort_order)
			VALUES (?, ?, ?, ?, 0, ?, ?)`,
			menu.ParentID, menu.Label, menu.URL, req.ID, menu.Icon, menu.SortOrder)
		if err != nil {
			jsonError(w, "Error registrando menú: "+err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Módulo registrado correctamente",
		"module": req.ID,
		"menus":  len(req.Menus),
	})
}

// moduleToggle activa/desactiva un módulo
func (s *Server) moduleToggle(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("id")

	var isCore int
	err := s.dbFor(r).QueryRow("SELECT is_core FROM modules WHERE id = ?", moduleID).Scan(&isCore)
	if err != nil {
		jsonError(w, "Módulo no encontrado", 404)
		return
	}
	if isCore == 1 {
		jsonError(w, "Los módulos core no pueden desactivarse", 403)
		return
	}

	_, err = s.dbFor(r).Exec("UPDATE modules SET is_active = NOT is_active WHERE id = ?", moduleID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// El menú se cachea 30 s. Sin invalidar aquí, el módulo seguía apareciendo
	// (o seguía oculto) después de pulsar el botón, y el usuario creía que el
	// interruptor no funcionaba.
	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Módulo actualizado"})
}

// moduleDelete elimina un módulo (solo no-core)
func (s *Server) moduleDelete(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("id")

	var isCore int
	err := s.dbFor(r).QueryRow("SELECT is_core FROM modules WHERE id = ?", moduleID).Scan(&isCore)
	if err != nil {
		jsonError(w, "Módulo no encontrado", 404)
		return
	}
	if isCore == 1 {
		jsonError(w, "Los módulos core no pueden eliminarse", 403)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Eliminar menús del módulo
	_, err = tx.Exec("DELETE FROM menu WHERE module_id = ?", moduleID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Eliminar módulo
	_, err = tx.Exec("DELETE FROM modules WHERE id = ?", moduleID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Módulo eliminado"})
}

// moduleUpdate actualiza los datos de un módulo (solo no-core)
func (s *Server) moduleUpdate(w http.ResponseWriter, r *http.Request) {
	moduleID := r.PathValue("id")

	var isCore int
	err := s.dbFor(r).QueryRow("SELECT is_core FROM modules WHERE id = ?", moduleID).Scan(&isCore)
	if err != nil {
		jsonError(w, "Módulo no encontrado", 404)
		return
	}
	if isCore == 1 {
		jsonError(w, "Los módulos core no pueden editarse", 403)
		return
	}

	var req struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Version     string      `json:"version"`
		Menus       []MenuEntry `json:"menus"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", 400)
		return
	}

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec("UPDATE modules SET name = ?, description = ?, version = ? WHERE id = ?",
		req.Name, req.Description, req.Version, moduleID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if len(req.Menus) > 0 {
		_, _ = tx.Exec("DELETE FROM menu WHERE module_id = ? AND is_core = 0", moduleID)
		for _, menu := range req.Menus {
			_, err = tx.Exec(`INSERT INTO menu (parent_id, label, url, module_id, is_core, icon, sort_order)
				VALUES (?, ?, ?, ?, 0, ?, ?)`,
				menu.ParentID, menu.Label, menu.URL, moduleID, menu.Icon, menu.SortOrder)
			if err != nil {
				jsonError(w, "Error registrando menú: "+err.Error(), 500)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Módulo actualizado"})
}

// registerModuleCore registra los módulos core si no existen
func (s *Server) registerModuleCore() {
	coreModules := []struct {
		id, name, version, description string
	}{
		{"core_ventas", "Ventas", "1.0", "Facturación, clientes, cotizaciones"},
		{"core_compras", "Compras", "1.0", "Proveedores, órdenes de compra, servicios"},
		{"core_inventario", "Inventario", "1.0", "Productos, kardex, ajustes"},
		{"core_cxc", "Cuentas por Cobrar", "1.0", "Cartera, cobros, créditos"},
		{"core_cxp", "Cuentas por Pagar", "1.0", "Pagos a proveedores, obligaciones"},
		{"core_contabilidad", "Contabilidad", "1.0", "Asientos, plan de cuentas, libros"},
		{"core_rrhh", "Recursos Humanos", "1.0", "Empleados, nómina, RRHH"},
		{"core_bancos", "Bancos", "1.0", "Cuentas bancarias, conciliación"},
		{"core_reportes", "Reportes", "1.0", "Reportes, consultas, auditoría"},
		{"core_configuracion", "Configuración", "1.0", "Sistema, usuarios, roles, ajustes"},
	}

	for _, m := range coreModules {
		ctl, err := s.controlStore()
		// Sin base de control no hay dónde registrar el módulo. Antes se llamaba
		// a ctl.Exec con ctl nil: el nil no venía de aquí, venía del arranque
		// degradado (esquema de control que no aplicó), y el resultado era un
		// panic que tumbaba el proceso entero en vez de arrancar sin catálogo.
		if err != nil || ctl == nil {
			return
		}
		_, _ = ctl.Exec(
			`INSERT IGNORE INTO modules (id, name, version, description, is_core, is_active)
			 VALUES (?, ?, ?, ?, 1, 1)`,
			m.id, m.name, m.version, m.description,
		)
	}
}
