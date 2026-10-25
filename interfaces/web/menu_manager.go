package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// ---------- Gestor de Menús (mover y reordenar) ----------

func (s *Server) registerMenuManagerRoutes() {
	s.mux.HandleFunc("GET /config/menu", s.menuManager)
	s.mux.HandleFunc("GET /config/menu/data", s.menuManagerData)
	s.mux.HandleFunc("POST /config/menu/move", s.menuMove)
	s.mux.HandleFunc("POST /config/menu/reorder", s.menuReorder)
}

// menuManager renderiza la pantalla del gestor de menús.
func (s *Server) menuManager(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "menu_manager.html", map[string]interface{}{
		"Title": "Gestor de Menús",
	})
}

// menuNodeJSON es la estructura que el frontend consume para el árbol.
type menuNodeJSON struct {
	ID        int             `json:"id"`
	ParentID  *int            `json:"parent_id"`
	Label     string          `json:"label"`
	URL       string          `json:"url"`
	Icon      string          `json:"icon"`
	Perm      string          `json:"perm"`
	SortOrder int             `json:"sort_order"`
	Status    int             `json:"status"`
	ModuleID  string          `json:"module_id"`
	IsCore    int             `json:"is_core"`
	Children  []*menuNodeJSON `json:"children"`
}

// menuManagerData devuelve todos los menús (core + addon) como árbol.
func (s *Server) menuManagerData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.authDB(r).Query(`
		SELECT id, parent_id, label, url, icon, perm, sort_order, status, module_id, is_core
		FROM menu
		WHERE status = 1
		ORDER BY COALESCE(parent_id, 0), sort_order, id`)
	if err != nil {
		jsonError(w, "Error cargando menús: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	all := []*menuNodeJSON{}
	byID := map[int]*menuNodeJSON{}
	for rows.Next() {
		n := &menuNodeJSON{}
		var parentID sql.NullInt64
		var url, icon, perm sql.NullString
		if err := rows.Scan(&n.ID, &parentID, &n.Label, &url, &icon, &perm,
			&n.SortOrder, &n.Status, &n.ModuleID, &n.IsCore); err != nil {
			continue
		}
		if parentID.Valid {
			p := int(parentID.Int64)
			n.ParentID = &p
		}
		n.URL = url.String
		n.Icon = icon.String
		n.Perm = perm.String
		all = append(all, n)
		byID[n.ID] = n
	}

	var roots []*menuNodeJSON
	for _, n := range all {
		if n.ParentID != nil {
			if p, ok := byID[*n.ParentID]; ok {
				p.Children = append(p.Children, n)
			} else {
				// Padre inexistente o inactivo: tratar como raíz
				roots = append(roots, n)
			}
			continue
		}
		roots = append(roots, n)
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"menus":  roots,
	})
}

// moveRequest para mover un menú a otro padre y/o posición.
type moveRequest struct {
	ID       int `json:"id"`
	ParentID int `json:"parent_id"` // 0 = raíz
}

// menuMove cambia el parent_id de un menú (mover entre grupos).
func (s *Server) menuMove(w http.ResponseWriter, r *http.Request) {
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validar que el menú existe
	var exists int
	err := s.authDB(r).QueryRow("SELECT COUNT(*) FROM menu WHERE id = ?", req.ID).Scan(&exists)
	if err != nil || exists == 0 {
		jsonError(w, "Menú no encontrado", http.StatusNotFound)
		return
	}

	// Validar que el padre existe (si no es raíz)
	if req.ParentID != 0 {
		var parentExists int
		err := s.authDB(r).QueryRow("SELECT COUNT(*) FROM menu WHERE id = ? AND status = 1", req.ParentID).Scan(&parentExists)
		if err != nil || parentExists == 0 {
			jsonError(w, "Menú destino no encontrado o inactivo", http.StatusBadRequest)
			return
		}
		// Evitar ciclar: un menú no puede ser su propio descendiente
		if isDescendant(s.authDB(r), req.ID, req.ParentID) {
			jsonError(w, "No se puede mover un menú dentro de sí mismo", http.StatusBadRequest)
			return
		}
	}

	// Calcular nuevo sort_order (al final del grupo destino)
	var newOrder int
	if req.ParentID == 0 {
		err = s.authDB(r).QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 10 FROM menu WHERE parent_id IS NULL").Scan(&newOrder)
	} else {
		err = s.authDB(r).QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 10 FROM menu WHERE parent_id = ?", req.ParentID).Scan(&newOrder)
	}
	if err != nil {
		newOrder = 10
	}

	_, err = s.authDB(r).Exec("UPDATE menu SET parent_id = ?, sort_order = ? WHERE id = ?",
		sql.NullInt64{Int64: int64(req.ParentID), Valid: req.ParentID != 0}, newOrder, req.ID)
	if err != nil {
		jsonError(w, "Error moviendo menú: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Menú movido correctamente",
	})
}

// reorderRequest para reordenar menús dentro de un mismo padre.
type reorderRequest struct {
	ParentID int   `json:"parent_id"` // 0 = raíz
	IDs      []int `json:"ids"`       // IDs en el nuevo orden
}

// menuReorder recalcula sort_order para los IDs dados.
func (s *Server) menuReorder(w http.ResponseWriter, r *http.Request) {
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos: "+err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := s.authDB(r).Begin()
	if err != nil {
		jsonError(w, "Error iniciando transacción: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for i, id := range req.IDs {
		sortOrder := (i + 1) * 10
		var res sql.Result
		if req.ParentID == 0 {
			res, err = tx.Exec("UPDATE menu SET parent_id = NULL, sort_order = ? WHERE id = ? AND parent_id IS NULL",
				sortOrder, id)
		} else {
			res, err = tx.Exec("UPDATE menu SET sort_order = ? WHERE id = ? AND parent_id = ?",
				sortOrder, id, req.ParentID)
		}
		if err != nil {
			jsonError(w, "Error reordenando: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ra, _ := res.RowsAffected()
		if ra == 0 {
			jsonError(w, "Menú "+strconv.Itoa(id)+" no encontrado en el grupo esperado", http.StatusBadRequest)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, "Error guardando cambios: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Orden actualizado",
	})
}

// isDescendant verifica que 'candidate' no sea descendiente de 'ancestor'.
func isDescendant(db *sql.DB, ancestor, candidate int) bool {
	if ancestor == candidate {
		return true
	}
	rows, err := db.Query("SELECT parent_id FROM menu WHERE id = ?", candidate)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var parentID sql.NullInt64
		if rows.Scan(&parentID) != nil || !parentID.Valid {
			return false
		}
		if parentID.Int64 == int64(ancestor) {
			return true
		}
		return isDescendant(db, ancestor, int(parentID.Int64))
	}
	return false
}
