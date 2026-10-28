package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erpgow"
)

// ---------- Gestor de Menús (mover y reordenar) ----------

func (s *Server) registerMenuManagerRoutes() {
	s.mux.HandleFunc("GET /config/menu", s.menuManager)
	s.mux.HandleFunc("GET /config/menu/data", s.menuManagerData)
	s.mux.HandleFunc("POST /config/menu/move", s.menuMove)
	s.mux.HandleFunc("POST /config/menu/reorder", s.menuReorder)
	s.mux.HandleFunc("GET /config/menu/history", s.menuBackupHistory)
	s.mux.HandleFunc("POST /config/menu/snapshot", s.menuSnapshotSave)
	s.mux.HandleFunc("POST /config/menu/restore", s.menuRestore)
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

	// Snapshot ANTES de aplicar el cambio: la red de seguridad del botón
	// Restaurar. Si el snapshot fallara por algo puntual, el movimiento sigue
	// (un fallo del snapshot no debe bloquear la edición).
	if _, err := menuSnapshotCreate(s.authDB(r), "auto"); err != nil {
		log.Printf("menús: snapshot previo a mover %d: %v", req.ID, err)
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

	invalidarCacheMenu()
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

	// Snapshot antes de mutar arranques (mismo criterio que menuMove).
	if _, err := menuSnapshotCreate(s.authDB(r), "auto"); err != nil {
		log.Printf("menús: snapshot previo a reordenar grupo %d: %v", req.ParentID, err)
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

	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Orden actualizado",
	})
}

// ---------- COPIAS Y RESTAURACIÓN DE MENÚS ----------
//
// Un drag accidental borra un mes de orden cuidadoso y no hay vuelta atrás:
// por eso CADA movimiento reordena un snapshot ANTES de tocar nada, el panel
// muestra los últimos puntos, y el botón Restaurar vuelve a cualquiera.
//
// Especial: "restaurar de fábrica" re-ejecuta el INSERT del seed embebido
// (migrations/schema_z_menu.sql, migración que ya ejecuta el binario al
// instalar), de forma que el orden de fábrica vive en UN solo lugar.
//
// NADA se borra de la tabla menu ni en la copia ni en la restauración: solo
// UPDATE (posiciones). Las filas nuevas creadas después de cada snapshot no
// se pierden.

// menuSnapshotCreate guarda el estado completo de la tabla menu de la empresa.
// Devuelve el id de la copia.
func menuSnapshotCreate(db *sql.DB, source string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO menu_backup (source) VALUES (?)`, source)
	if err != nil {
		return 0, err
	}
	backupID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`INSERT INTO menu_backup_row
		(backup_id, menu_id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id, is_core)
		SELECT ?, id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id, is_core
		FROM menu`, backupID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	// Cap de backups automáticos: quedan los últimos 20; los manuales no se
	// recortan (fueron una decisión del admin).
	if source == "auto" {
		menuSnapshotCap(db, 20)
	}
	return backupID, nil
}

// menuSnapshotCap deja nada más la última n de copias automáticas.
func menuSnapshotCap(db *sql.DB, n int) {
	var ids []interface{}
	rows, err := db.Query(`SELECT id FROM menu_backup WHERE source='auto' ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := append(ids, "auto")
	_, _ = db.Exec(`DELETE FROM menu_backup WHERE source=? AND id NOT IN (`+placeholders+")", args...)
	_, _ = db.Exec(`DELETE FROM menu_backup_row WHERE backup_id NOT IN (SELECT id FROM menu_backup)`)
}

// menuBackupHistory lista las copias (las últimas 30), más nuevas primero.
func (s *Server) menuBackupHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.authDB(r).Query(`SELECT b.id, b.source, b.created_at, COUNT(r.menu_id)
		FROM menu_backup b LEFT JOIN menu_backup_row r ON r.backup_id = b.id
		GROUP BY b.id, b.source, b.created_at
		ORDER BY b.id DESC LIMIT 30`)
	if err != nil {
		jsonError(w, "Error leyendo copias: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type copia struct {
		ID        int64  `json:"id"`
		Source    string `json:"source"`
		CreatedAt string `json:"created_at"`
		Menus     int    `json:"menus"`
	}
	var copias []copia
	for rows.Next() {
		var c copia
		var cuando time.Time
		if err := rows.Scan(&c.ID, &c.Source, &cuando, &c.Menus); err != nil {
			continue
		}
		c.CreatedAt = cuando.Format("2006-01-02 15:04:05")
		copias = append(copias, c)
	}

	writeJSON(w, map[string]interface{}{"status": 1, "backups": copias})
}

// menuSnapshotSave crea una copia MANUAL a instancias del botón de la pantalla.
func (s *Server) menuSnapshotSave(w http.ResponseWriter, r *http.Request) {
	id, err := menuSnapshotCreate(s.authDB(r), "manual")
	if err != nil {
		jsonError(w, "No se pudo guardar la copia: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "backup_id": id, "msg": "Punto de restauración guardado"})
}

// restoMenuBody para restaurar: snapshot_id o factory=true.
type restoreMenuBody struct {
	SnapshotID int64 `json:"snapshot_id"`
	Factory    bool  `json:"factory"`
}

// menuRestore vuelve a un punto anterior (snapshot) o al orden de fábrica (seed).
// SIEMPRE toma un snapshot de la situación actual antes de restaurar, para que
// incluso el error que se comete restaurando se pueda deshacer.
func (s *Server) menuRestore(w http.ResponseWriter, r *http.Request) {
	var req restoreMenuBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Red de seguridad: guardar la situación actual antes de tocar nada.
	if id, err := menuSnapshotCreate(s.authDB(r), "auto"); err != nil {
		log.Printf("menús: snapshot previo a restaurar: %v", err)
	} else {
		log.Printf("menús: restauración con respaldo previo %d", id)
	}

	if req.Factory {
		s.menuRestaurarFabrica(w, r)
		return
	}

	s.menuRestaurarSnapshot(w, r, req.SnapshotID)
}

// menuRestaurarSnapshot aplica una copia: UPDATE por cada fila copiada y
// INSERT de las menús que existan en la copia pero NO en la tabla actual (menu
// borrado por error). Lo nuevo que exista solo en la tabla no se toca.
func (s *Server) menuRestaurarSnapshot(w http.ResponseWriter, r *http.Request, backupID int64) {
	if backupID <= 0 {
		jsonError(w, "snapshot_id inválido", http.StatusBadRequest)
		return
	}
	db := s.authDB(r)

	var existe int
	if err := db.QueryRow(`SELECT COUNT(*) FROM menu_backup WHERE id = ?`, backupID).Scan(&existe); err != nil || existe == 0 {
		jsonError(w, "La copia no existe (¿ya fue depurada por la rotación automática?)", http.StatusNotFound)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		jsonError(w, "Error iniciando transacción: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// 1) Filas que están en ambas: volver a los valores del snapshot.
	res, err := tx.Exec(`UPDATE menu m
		JOIN menu_backup_row r ON r.backup_id = ? AND r.menu_id = m.id
		SET m.parent_id = r.parent_id, m.label = r.label, m.url = r.url, m.icon = r.icon,
			m.perm = r.perm, m.active_prefix = r.active_prefix, m.sort_order = r.sort_order,
			m.status = r.status, m.module_id = r.module_id, m.is_core = r.is_core`, backupID)
	if err != nil {
		jsonError(w, "Error restaurando fila: "+err.Error(), http.StatusInternalServerError)
		return
	}
	afectados, _ := res.RowsAffected()

	// 2) Menús que la copia tenía y la tabla ya no (borrados por error).
	res, err = tx.Exec(`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id, is_core)
		SELECT r.menu_id, r.parent_id, r.label, r.url, r.icon, r.perm, r.active_prefix, r.sort_order, r.status, r.module_id, r.is_core
		FROM menu_backup_row r
		WHERE r.backup_id = ? AND r.menu_id NOT IN (SELECT id FROM menu)`, backupID)
	if err != nil {
		jsonError(w, "Error restaurando menús borrados: "+err.Error(), http.StatusInternalServerError)
		return
	}
	recreados, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		jsonError(w, "Error guardando restauración: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("menús: restaurado snapshot %d (%d filas actualizadas, %d recreadas)", backupID, afectados, recreados)
	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    fmt.Sprintf("Menús restaurados al punto %d", backupID),
	})
}

// menuRestaurarFabrica re-ejecuta el seed embebido del menú (ON DUPLICATE KEY
// UPDATE), que es el ÚNICO lugar donde vive el orden de fábrica.
func (s *Server) menuRestaurarFabrica(w http.ResponseWriter, r *http.Request) {
	migs, err := fs.ReadFile(erpgow.Migrations(), "migrations/schema_z_menu.sql")
	if err != nil {
		jsonError(w, "Seed de menús no disponible en el binario: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Normalizar CRLF y partir por ';' (mismo criterio que el motor de
	// migraciones). Solo ejecutamos los INSERT INTO `menu`, nunca CREATE.
	normalizado := strings.ReplaceAll(string(migs), "\r\n", "\n")
	var ejecutadas int
	for _, stmt := range strings.Split(normalizado, ";") {
		limpio := strings.TrimSpace(stmt)
		if strings.Contains(limpio, "\n") {
			limpio = strings.TrimSpace(strings.SplitN(limpio, "\n", 2)[1])
		}
		if !strings.HasPrefix(limpio, "INSERT INTO `menu`") {
			continue
		}
		if _, err := s.authDB(r).Exec(limpio); err != nil {
			jsonError(w, "Error ejecutando seed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ejecutadas++
	}
	if ejecutadas == 0 {
		jsonError(w, "No se encontró el seed embebido del menú", http.StatusInternalServerError)
		return
	}
	log.Printf("menús: restaurado orden de fábrica (%d sentencia sembrada)", ejecutadas)
	invalidarCacheMenu()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Menús restaurados al orden de fábrica"})
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
