package web

// customer_class.go — Clasificación de clientes (minorista, mayorista, etc.)

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerCustomerClassRoutes() {
	s.mux.HandleFunc("GET /customer/class/list", s.customerClassList)
	s.mux.HandleFunc("GET /customer/class/data", s.customerClassData)
	s.mux.HandleFunc("GET /customer/class/form", s.customerClassForm)
	s.mux.HandleFunc("POST /customer/class/save", s.customerClassSave)
	s.mux.HandleFunc("POST /customer/class/delete/{id}", s.customerClassDelete)
}

func (s *Server) customerClassList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "customer_class.html", map[string]interface{}{"Title": "Clasificación de Clientes"})
}

func (s *Server) customerClassData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	search := strings.TrimSpace(r.FormValue("search[value]"))

	where := ""
	args := []interface{}{}
	if search != "" {
		where = " WHERE label LIKE ? OR description LIKE ?"
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM customer_class").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM customer_class"+where, args...).Scan(&filtered)

	rows, err := s.dbFor(r).Query(`SELECT id, label, description, sort_order, status FROM customer_class`+where+` ORDER BY sort_order LIMIT 100`, args...)
	if err != nil {
		writeJSON(w, map[string]interface{}{"data": []interface{}{}})
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var label, description, status string
		var sortOrder int
		rows.Scan(&id, &label, &description, &sortOrder, &status)
		list = append(list, map[string]interface{}{
			"id": id, "label": label, "description": description,
			"sort_order": sortOrder, "status": status,
		})
	}
	writeJSON(w, map[string]interface{}{"draw": draw, "recordsTotal": total, "recordsFiltered": filtered, "data": list})
}

func (s *Server) customerClassForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data := map[string]interface{}{"Title": "Nueva Clasificación"}
	if id != "" {
		var label, description, status string
		var sortOrder int
		err := s.dbFor(r).QueryRow("SELECT label, description, sort_order, status FROM customer_class WHERE id = ?", id).Scan(&label, &description, &sortOrder, &status)
		if err == nil {
			data["Title"] = "Editar Clasificación"
			data["EditID"] = id
			data["Label"] = label
			data["Description"] = description
			data["SortOrder"] = sortOrder
			data["Status"] = status
		}
	}
	s.render(w, r, "customer_class_form.html", data)
}

func (s *Server) customerClassSave(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "customer_class", "save") {
		jsonError(w, "Sin permisos", 403)
		return
	}

	id := r.FormValue("id")
	label := strings.TrimSpace(r.FormValue("label"))
	description := strings.TrimSpace(r.FormValue("description"))
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	status := orDefault(r.FormValue("status"), "active")

	if label == "" {
		jsonError(w, "Etiqueta requerida", 400)
		return
	}

	if id == "" {
		_, err := s.dbFor(r).Exec(`INSERT INTO customer_class (label, description, sort_order, status) VALUES (?, ?, ?, ?)`,
			label, description, sortOrder, status)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		_, err := s.dbFor(r).Exec(`UPDATE customer_class SET label=?, description=?, sort_order=?, status=? WHERE id=?`,
			label, description, sortOrder, status, id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Clasificación guardada"})
}

func (s *Server) customerClassDelete(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "customer_class", "delete") {
		jsonError(w, "Sin permisos", 403)
		return
	}

	id := r.PathValue("id")
	_, err := s.dbFor(r).Exec("DELETE FROM customer_class WHERE id = ?", id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Clasificación eliminada"})
}
