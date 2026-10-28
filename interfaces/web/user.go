package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

// ---------- Users & Roles (replicates dashboard module user management) ----------

func (s *Server) registerUserRoutes() {
	s.mux.HandleFunc("GET /user/list", s.userList)
	s.mux.HandleFunc("GET /user/list/data", s.userListData)
	s.mux.HandleFunc("GET /user/add", s.userForm)
	s.mux.HandleFunc("GET /user/edit/{id}", s.userForm)
	s.mux.HandleFunc("POST /user/save", s.userSave)
	s.mux.HandleFunc("POST /user/delete/{id}", s.userDelete)
	s.mux.HandleFunc("GET /user/profile", s.userProfile)
	s.mux.HandleFunc("POST /user/change-password", s.userChangePassword)
	// photo
	s.mux.HandleFunc("POST /user/photo/save", s.userPhotoSave)
	// roles
	s.mux.HandleFunc("GET /role/list", s.roleList)
	s.mux.HandleFunc("GET /role/list/data", s.roleListData)
	s.mux.HandleFunc("POST /role/save", s.roleSave)
	s.mux.HandleFunc("POST /role/delete/{id}", s.roleDelete)
	s.mux.HandleFunc("POST /role/permissions", s.rolePermissionsSave)
	s.mux.HandleFunc("GET /role/permissions/{id}", s.rolePermissionsForm)
}

func (s *Server) userList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "user_list.html", map[string]interface{}{"Title": "Usuarios"})
}

func (s *Server) userListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.authDB(r).Query(`
		SELECT u.user_id, u.first_name, u.last_name, IFNULL(u.phone,''), ul.username, IFNULL(ul.user_type, 2), u.status
		FROM users u JOIN user_login ul ON ul.user_id = u.user_id
		ORDER BY u.id`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var uid string
		var first, last, phone, username sql.NullString
		var utype, status int
		_ = rows.Scan(&uid, &first, &last, &phone, &username, &utype, &status)
		level := "user"
		switch utype {
		case 1:
			level = "admin"
		case 3:
			level = "employee"
		}
		list = append(list, map[string]interface{}{
			"id": uid, "name": strings.TrimSpace(first.String + " " + last.String),
			"phone": phone.String, "username": username.String, "level": level, "status": status,
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

// rolesParaAgentes lista los roles de seguridad para el desplegable del alta de
// usuario agente.
//
// Si la tabla está vacía devuelve vacío en vez de fallar: el ERP recién instalado
// puede no tener roles, y en ese caso lo que falla es que el alta de un agente
// eche "necesita un rol", que ya es un mensaje claro. Un error de base de datos
// aquí dejaría la pantalla de usuarios entera sin abrir.
func (s *Server) rolesParaAgentes(r *http.Request) []map[string]interface{} {
	rows, err := s.authDB(r).Query(`SELECT id, type FROM sec_role ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var lista []map[string]interface{}
	for rows.Next() {
		var id int64
		var tipo string
		if err := rows.Scan(&id, &tipo); err != nil {
			continue
		}
		lista = append(lista, map[string]interface{}{"ID": id, "Type": tipo})
	}
	return lista
}

// opcionesDeDepartamento es lo que necesita el formulario: el valor que se
// guarda y el texto que ve el administrador.
//
// El orden es el de aiDepartamentos, que es el que usa el resto del sistema. Si
// se ordenara aquí distinto, la pantalla y el scopado empezarían a discrepar
// sin que nada lo dijera.
type opcionDeDepartamento struct {
	Value string
	Label string
}

func opcionesDeDepartamentos() []opcionDeDepartamento {
	out := make([]opcionDeDepartamento, 0, len(aiDepartamentos))
	for _, d := range aiDepartamentos {
		out = append(out, opcionDeDepartamento{Value: d, Label: aiDeptLabel[d]})
	}
	return out
}

func (s *Server) userForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":         "Añadir Usuario",
		"Departamentos": opcionesDeDepartamentos(),
		"Roles":         s.rolesParaAgentes(r),
	}
	if id := r.PathValue("id"); id != "" {
		var uid, first, last, phone, username sql.NullString
		var utype, status int
		err := s.authDB(r).QueryRow(`
			SELECT u.user_id, u.first_name, u.last_name, u.phone, ul.username, IFNULL(ul.user_type, 2), u.status
			FROM users u JOIN user_login ul ON ul.user_id = u.user_id WHERE u.user_id = ?`, id).
			Scan(&uid, &first, &last, &phone, &username, &utype, &status)
		if err != nil {
			http.Error(w, "Usuario no encontrado", 404)
			return
		}
		data["Title"] = "Editar Usuario"
		data["U"] = map[string]interface{}{
			"ID": uid.String, "First": first.String, "Last": last.String,
			"Phone": phone.String, "Username": username.String, "Type": utype, "Status": status,
		}
		// El rol de seguridad que tiene HOY, para que el desplegable salga marcado.
		// Sin esto, abrir un usuario y guardarlo le borraba el rol (el select
		// saldría vacío y userSave escribiría lo que hubiera).
		data["UserRoleID"] = s.rolActualDeUsuario(r, uid.String)
		// El agente que acompaña al usuario, si lo hay: sin esto, editar un
		// usuario agente lo convertiría en humano al guardar, porque el
		// formulario saldría desmarcado.
		var rolID int64
		var dept sql.NullString
		var cfg sql.NullString
		if err := s.authDB(r).QueryRow(`SELECT role_id, department, IFNULL(config_prompt,'')
			FROM ai_agent WHERE user_id = ? ORDER BY id LIMIT 1`, id).
			Scan(&rolID, &dept, &cfg); err == nil {
			data["Agente"] = map[string]interface{}{
				"RoleID": rolID, "Department": strings.ToLower(strings.TrimSpace(dept.String)),
				"ConfigPrompt": cfg.String,
			}
		}
	}
	s.render(w, r, "user_form.html", data)
}

func (s *Server) userSave(w http.ResponseWriter, r *http.Request) {
	first := strings.TrimSpace(r.FormValue("first_name"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	id := r.FormValue("user_id")
	utype, _ := strconv.Atoi(orDefault(r.FormValue("user_type"), "2"))

	if first == "" || username == "" {
		jsonError(w, "Nombre y usuario son obligatorios", 400)
		return
	}

	// Agente IA: el usuario se crea como cualquier otro, pero además puede
	// llevar el agente detrás. La contraseña NO es obligatoria en ese caso:
	// nadie la usa, porque el agente actúa por el runner del servidor y no
	// inicia sesión. Si se deja vacía se genera una aleatoria, que es
	// exactamente lo que hace agentSave.
	esAgente := r.FormValue("is_agent") == "1"
	if esAgente {
		if _, err := strconv.Atoi(r.FormValue("agent_role_id")); err != nil {
			jsonError(w, "Un usuario agente necesita un rol de seguridad", 400)
			return
		}
		if password == "" {
			secreto, err := aiRandomSecret(24)
			if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
			password = string(secreto)
		}
	}

	tx, err := s.authDB(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	nuevoUID := ""
	if id == "" {
		if password == "" {
			jsonError(w, "La contraseña es obligatoria", 400)
			return
		}
		// unique username
		var exists int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM user_login WHERE username = ?`, username).Scan(&exists)
		if exists > 0 {
			jsonError(w, "El usuario ya existe", 400)
			return
		}
		hash, err := bcryptGenerate([]byte(password))
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		res, err := tx.Exec(`INSERT INTO users (user_id, first_name, last_name, phone, status)
			VALUES (?,?,?,?,1)`, generator(6), first, r.FormValue("last_name"), r.FormValue("phone"))
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		newID, _ := res.LastInsertId()
		nuevoUID := fmt.Sprintf("%d", newID)
		uid := nuevoUID
		// users.user_id debe ser MISMO valor que user_login.user_id: el JOIN
		// `ul.user_id = u.user_id` es el que alimenta la lista de usuarios y el
		// login. Con un código aleatorio aquí, el usuario creado no aparecía en
		// la lista y no podía iniciar sesión (JOIN sin coincidencia).
		_, _ = tx.Exec(`UPDATE users SET user_id = ? WHERE id = ?`, uid, newID)

		// Un usuario agente se crea con la sesión DESACTIVADA (status=0): no
		// inicia sesión nunca, y dejarla activa abriría una puerta con una
		// contraseña que el sistema generó y nadie conoce.
		loginStatus := 1
		if esAgente {
			loginStatus = 0
		}
		if _, err := tx.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
			VALUES (?,?,?,?,?)`, uid, username, string(hash), utype, loginStatus); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}

		// Asignar usuario a la empresa activa
		var activeTenant string
		_ = s.authDB(r).QueryRow("SELECT code FROM tenants WHERE status = 1 ORDER BY id LIMIT 1").Scan(&activeTenant)
		if activeTenant == "" {
			activeTenant = "default"
		}
		if _, err := tx.Exec(`INSERT IGNORE INTO user_company_access (user_id, tenant_code, role_id) VALUES (?, ?, 2)`, uid, activeTenant); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if esAgente {
			if err := s.crearAgenteParaUsuarioTx(tx, r, uid, first, s.creatorDeRequest(r)); err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
		}
	} else {
		if _, err := tx.Exec(`UPDATE users SET first_name=?, last_name=?, phone=? WHERE user_id=?`,
			first, r.FormValue("last_name"), r.FormValue("phone"), id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if password != "" {
			hash, err := bcryptGenerate([]byte(password))
			if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
			_, _ = tx.Exec(`UPDATE user_login SET password=? WHERE user_id=?`, string(hash), id)
		}
		_, _ = tx.Exec(`UPDATE user_login SET user_type=? WHERE user_id=?`, utype, id)

		// Editar un usuario agente actualiza su departamento y su prompt. Sin
		// esto, cambiar el área de un agente obligaría a entrar por la pantalla
		// de agentes y el mismo dato estaría en dos sitios con dos reglas.
		if esAgente {
			if _, err := strconv.Atoi(r.FormValue("agent_role_id")); err != nil {
				jsonError(w, "Un usuario agente necesita un rol de seguridad", 400)
				return
			}
			if err := s.actualizarAgenteDeUsuarioTx(tx, r, id); err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
		}
	}
	// El ROL DE SEGURIDAD del usuario.
	//
	// Faltaba, y sin él la delegación no existía: la pantalla de Roles y Permisos
	// guardaba la matriz de un rol (CRUD por módulo) pero NINGUNA pantalla
	// asignaba ese rol a una persona. Un usuario creado desde aquí quedaba sin rol
	// —nivel "employee"— y recibía 403 en todo el ERP.
	//
	// Va DENTRO de la misma transacción que crea o edita al usuario: un usuario
	// creado sin su rol es un usuario que no puede trabajar, y confirmar el alta y
	// fallar después el rol deja exactamente ese estado.
	uid := id
	if uid == "" {
		uid = nuevoUID
	}
	if err := s.asignarRolUsuarioTx(tx, uid, strings.TrimSpace(r.FormValue("role_id"))); err != nil {
		jsonError(w, "no se pudo asignar el rol: "+err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"msg":      "Usuario guardado",
		"redirect": saveRedirect(r, "/user/list"),
	})
}

// rolActualDeUsuario devuelve el id del rol de seguridad asignado a un usuario.
//
// Se mira la base de la empresa y, si allí no hay fila, la de control: el rol de
// una empresa manda sobre el global, que es el mismo orden que aplica el
// middleware de permisos (si aquí fuera el contrario, la pantalla diría un rol y
// el sistema aplicaría otro).
func (s *Server) rolActualDeUsuario(r *http.Request, userID string) int64 {
	if userID == "" {
		return 0
	}
	consulta := `SELECT roleid FROM sec_userrole WHERE user_id = ? LIMIT 1`
	var id int64
	if err := s.dbFor(r).QueryRow(consulta, userID).Scan(&id); err == nil && id > 0 {
		return id
	}
	if ctl, err := s.controlStore(); err == nil && ctl != nil {
		if err := ctl.QueryRow(consulta, userID).Scan(&id); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// asignarRolUsuarioTx deja al usuario con ese rol de seguridad, y con uno solo.
//
// Es «borrar y volver a poner» porque la tabla admite varias filas por usuario
// (no tiene UNIQUE sobre user_id) y el resto del sistema —loadUserRole— resuelve
// el nivel con un LIMIT 1: si quedaran dos roles, cuál manda dependería del orden
// en que MySQL devolviera las filas. Un usuario con dos roles es un usuario con
// un nivel que cambia solo.
//
// El id se calcula a mano porque la tabla NO declara AUTO_INCREMENT en todas
// las conversiones del esquema (SQLite solo tiene UNIQUE), y un INSERT sin id
// revienta allí con NOT NULL (es el mismo motivo que documenta ai_agents.go).
// roleID vacío NO borra el rol: significa «no lo toques». Borrarlo cuando el
// formulario no trae el campo convertiría a un usuario delegado en alguien sin
// permisos por el simple hecho de editarle el teléfono.
func (s *Server) asignarRolUsuarioTx(tx *sql.Tx, userID, roleID string) error {
	if strings.TrimSpace(roleID) == "" || userID == "" {
		return nil
	}
	id, err := strconv.ParseInt(roleID, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("rol inválido: %q", roleID)
	}
	if _, err := tx.Exec(`DELETE FROM sec_userrole WHERE user_id = ?`, userID); err != nil {
		return err
	}
	var siguiente int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(id),0)+1 FROM sec_userrole`).Scan(&siguiente); err != nil {
		siguiente = 1
	}
	_, err = tx.Exec(`INSERT INTO sec_userrole (id, user_id, roleid, createby, createdate)
		VALUES (?,?,?,?,?)`, siguiente, userID, id, "system", now())
	return err
}

func (s *Server) userDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.sessionFromRequest(r)
	if sess != nil && sess.UserID == id {
		jsonError(w, "No puede eliminar su propio usuario", 400)
		return
	}
	// keep at least one admin
	var admins int
	_ = s.authDB(r).QueryRow(`SELECT COUNT(*) FROM user_login WHERE user_type=1 AND status=1`).Scan(&admins)
	var utype int
	_ = s.authDB(r).QueryRow(`SELECT user_type FROM user_login WHERE user_id=?`, id).Scan(&utype)
	if utype == 1 && admins <= 1 {
		jsonError(w, "Debe existir al menos un administrador", 400)
		return
	}
	tx, err := s.authDB(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM user_login WHERE user_id=?`, id)
	res, err := tx.Exec(`DELETE FROM users WHERE user_id=?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Usuario no encontrado", 404)
		return
	}
	_ = tx.Commit()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/user/list")})
}

func (s *Server) userProfile(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	var first, last, phone, username sql.NullString
	_ = s.authDB(r).QueryRow(`
		SELECT u.first_name, u.last_name, u.phone, ul.username
		FROM users u JOIN user_login ul ON ul.user_id = u.user_id WHERE u.user_id = ?`, sess.UserID).
		Scan(&first, &last, &phone, &username)
	s.render(w, r, "user_profile.html", map[string]interface{}{
		"Title":    "Mi Perfil",
		"First":    first.String,
		"Last":     last.String,
		"Phone":    phone.String,
		"Username": username.String,
	})
}

func (s *Server) userChangePassword(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	current := r.FormValue("current_password")
	next := r.FormValue("new_password")
	if err := validatePasswordStrength(next); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	// verify current
	if _, err := s.authenticateUser(r, sess.Username, current); err != nil {
		jsonError(w, "Contraseña actual incorrecta", 400)
		return
	}
	controlDB, err := s.controlStore()
	if err != nil {
		jsonError(w, "Error de base de datos", 500)
		return
	}
	if err := changeUserPassword(controlDB, sess.Username, current, next); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Contraseña actualizada", "redirect": saveRedirect(r, "/user/list")})
}

// userPhotoSave sube la foto de un usuario y guarda la ruta en users.photo.
// Usa la rama 'personal' del upload.Manager: el directorio está protegido
// (solo master/admin pueden acceder vía /uploads/).
func (s *Server) userPhotoSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		jsonError(w, "No se pudo procesar el formulario: "+err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.FormValue("user_id")
	if userID == "" {
		jsonError(w, "Usuario inválido", http.StatusBadRequest)
		return
	}

	file, hdr, err := r.FormFile("photo")
	if err != nil {
		if err == http.ErrMissingFile {
			jsonError(w, "No se envió ninguna foto", http.StatusBadRequest)
			return
		}
		jsonError(w, "No se pudo leer el archivo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if hdr == nil || strings.TrimSpace(hdr.Filename) == "" {
		jsonError(w, "Archivo vacío", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !allowedImageExts[ext] {
		jsonError(w, "Extensión no permitida. Formatos válidos: jpg, jpeg, png, gif, webp", http.StatusBadRequest)
		return
	}

	db := s.authDB(r)

	// company_id de la empresa activa (para upload.Manager).
	var companyID string
	_ = db.QueryRow(`SELECT IFNULL(company_id,'1') FROM company_information WHERE status=1 LIMIT 1`).Scan(&companyID)
	if companyID == "" {
		companyID = "1"
	}

	result, err := s.uploadMgr.Save(companyID, "personal", file, hdr, db)
	if err != nil {
		jsonError(w, "No se pudo guardar la foto: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := db.Exec(`UPDATE users SET photo=? WHERE user_id=?`, result.Path, userID); err != nil {
		jsonError(w, "No se pudo actualizar la foto del usuario: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Foto guardada",
		"photo":  result.Path,
	})
}

// ---------- Roles ----------

func (s *Server) roleList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "role_list.html", map[string]interface{}{"Title": "Roles"})
}

func (s *Server) roleListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.authDB(r).Query(`
		SELECT r.id, r.type, r.level, COUNT(ur.id) AS users
		FROM sec_role r LEFT JOIN sec_userrole ur ON ur.roleid = r.id
		GROUP BY r.id, r.type, r.level`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id, users int
		var typ, level string
		_ = rows.Scan(&id, &typ, &level, &users)
		list = append(list, map[string]interface{}{"id": id, "type": typ, "level": level, "users": users})
	}
	writeJSON(w, list)
}

func (s *Server) roleSave(w http.ResponseWriter, r *http.Request) {
	typ := strings.TrimSpace(r.FormValue("type"))
	if typ == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	level := strings.TrimSpace(r.FormValue("level"))
	if level == "" {
		level = "employee"
	}
	validLevels := map[string]bool{"master": true, "admin": true, "manager": true, "employee": true}
	if !validLevels[level] {
		level = "employee"
	}
	id := r.FormValue("id")
	if id == "" {
		if _, err := s.authDB(r).Exec(`INSERT INTO sec_role (type, level) VALUES (?, ?)`, typ, level); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		if _, err := s.authDB(r).Exec(`UPDATE sec_role SET type=?, level=? WHERE id=?`, typ, level, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	invalidarCachePermisos()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Rol guardado", "redirect": saveRedirect(r, "/role/list")})
}

// rolePermissionsForm: permission matrix (submodule x CRUD).
func (s *Server) rolePermissionsForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var typ, level string
	if err := s.authDB(r).QueryRow(`SELECT type, level FROM sec_role WHERE id=?`, id).Scan(&typ, &level); err != nil {
		http.Error(w, "Rol no encontrado", 404)
		return
	}

	type perm struct {
		SubID  int64
		Name   string
		Create int
		Read   int
		Update int
		Delete int
	}
	perms := []perm{}
	// Backticks obligatorios: create/read/update/delete son reservadas de MySQL
	// (Error 1064 sin ellos — y la matriz de permisos se quedaba vacía).
	rows, err := s.authDB(r).Query("SELECT sm.id, sm.name,\n"+
		"IFNULL(rp.`create`,0), IFNULL(rp.`read`,0), IFNULL(rp.`update`,0), IFNULL(rp.`delete`,0)\n"+
		"FROM sub_module sm\n"+
		"LEFT JOIN role_permission rp ON rp.fk_module_id = sm.id AND rp.role_id = ?\n"+
		"WHERE sm.status = 1\n"+
		"ORDER BY sm.mid, sm.id", id)
	if err == nil && rows != nil {
		for rows.Next() {
			var p perm
			_ = rows.Scan(&p.SubID, &p.Name, &p.Create, &p.Read, &p.Update, &p.Delete)
			// Master y Admin tienen acceso total por definición del middleware
			// (canAccessModule / permissionMap). Si el rol no tiene filas explicitas
			// en role_permission, la matriz quedaría vacía y el admin parecería
			// no tener permisos. Por eso, para master/admin, forzamos todos los
			// checkboxes a marcados: refleja la realidad del control de acceso.
			if level == "master" || level == "admin" {
				p.Create = 1
				p.Read = 1
				p.Update = 1
				p.Delete = 1
			}
			perms = append(perms, p)
		}
		rows.Close()
	}

	s.render(w, r, "role_permissions.html", map[string]interface{}{
		"Title":  "Permisos: " + typ,
		"RoleID": id,
		"Role":   typ,
		"Level":  level,
		"Perms":  perms,
	})
}

// rolePermissionsSave: updates the whole matrix for a role.
// roleDelete elimina un rol no asignado. No se puede borrar un rol que tenga
// usuarios asociados (sec_userrole) — eso dejaría a esos usuarios sin nivel.
func (s *Server) roleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var userCount int
	if err := s.authDB(r).QueryRow(`SELECT COUNT(*) FROM sec_userrole WHERE roleid = ?`, id).Scan(&userCount); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if userCount > 0 {
		jsonError(w, fmt.Sprintf("No se puede eliminar: el rol tiene %d usuarios asignados", userCount), 400)
		return
	}
	// No se puede borrar un rol del sistema (is_system=1)
	var isSystem int
	if err := s.authDB(r).QueryRow(`SELECT IFNULL(is_system,0) FROM sec_role WHERE id = ?`, id).Scan(&isSystem); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if isSystem == 1 {
		jsonError(w, "No se puede eliminar un rol del sistema", 400)
		return
	}
	if _, err := s.authDB(r).Exec(`DELETE FROM sec_role WHERE id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	invalidarCachePermisos()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Rol eliminado", "redirect": "/role/list"})
}

// rolePermissionsSave: updates the whole matrix for a role.
func (s *Server) rolePermissionsSave(w http.ResponseWriter, r *http.Request) {
	roleID := r.FormValue("role_id")
	if roleID == "" {
		jsonError(w, "Rol inválido", 400)
		return
	}
	r.ParseMultipartForm(32 << 20)
	subIDs := r.Form["sub_module_id"]

	tx, err := s.authDB(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.Exec(`DELETE FROM role_permission WHERE role_id = ?`, roleID)

	for _, subID := range subIDs {
		c := r.FormValue("create_"+subID) == "1"
		rd := r.FormValue("read_"+subID) == "1"
		u := r.FormValue("update_"+subID) == "1"
		d := r.FormValue("delete_"+subID) == "1"
		if c || rd || u || d {
			// El error NO se traga: con `_, _ =` un INSERT que revienta (p. ej.
			// columna reservada sin backticks) devolvía 200 "Permisos guardados"
			// con cero filas — el bug que dejaba role_permission vacía.
			if _, err := tx.Exec("INSERT INTO role_permission (fk_module_id, role_id, `create`, `read`, `update`, `delete`)"+
				" VALUES (?,?,?,?,?,?)", subID, roleID, boolInt(c), boolInt(rd), boolInt(u), boolInt(d)); err != nil {
				jsonError(w, "no se pudo guardar el permiso del submódulo "+subID+": "+err.Error(), 500)
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// Se acaba de reescribir la matriz de permisos de un rol: lo que sus
	// usuarios VEN (menú y botones) se recalcula en el acto en su siguiente
	// vista. La puerta de ejecución (hasPermission / canAccessModule) nunca
	// estuvo cacheada, así que no hay nada más que tocar.
	invalidarCachePermisos()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Permisos guardados", "redirect": saveRedirect(r, "/user/list")})
}

// crearAgenteParaUsuarioTx crea la fila de ai_agent que acompaña a un usuario
// marcado como agente, dentro de la MISMA transacción que lo crea.
//
// Por qué en la misma transacción y no después: un usuario sin agente es un
// usuario humano que alguien quiso que fuera agente, y no hay forma de saberlo
// después. La fila del agente es la que dice que este usuario no es una persona.
// Si la inserción del agente fallara y la del usuario se hubiera confirmado, el
// sistema tendría un empleado que no puede hacer su trabajo y nadie se entera
// hasta que se le pide un informe.
//
// Por qué NO crea el usuario: esta función solo mete la fila de ai_agent. El
// usuario, su login y su rol los crea quien llama (userSave), en este orden, y
// por eso la función recibe el uid ya generado.
//
// El departamento y el prompt salen del formulario: es la misma información que
// se elige al crear el agente desde su propia pantalla, para que un usuario
// agente se pueda dar de alta desde los dos sitios y quede igual.
func (s *Server) crearAgenteParaUsuarioTx(tx *sql.Tx, r *http.Request, userID, nombre, creator string) error {
	roleID, _ := strconv.ParseInt(r.FormValue("agent_role_id"), 10, 64)
	providerID, _ := strconv.ParseInt(orDefault(r.FormValue("agent_provider_id"), "0"), 10, 64)

	if providerID <= 0 {
		// Sin proveedor elegido se usa el primero activo. Es lo que evita que
		// dar de alta un agente falle porque el administrador no sabe todavía
		// qué proveedor hay configurado; un agente sin proveedor no hace nada,
		// pero existe y se le puede configurar después.
		_ = tx.QueryRow(`SELECT id FROM ai_providers WHERE is_active = 1 ORDER BY id LIMIT 1`).Scan(&providerID)
	}
	if providerID <= 0 {
		return fmt.Errorf("no hay ningún proveedor de IA activo: configúralo antes de crear el agente")
	}

	department := strings.ToLower(strings.TrimSpace(r.FormValue("agent_department")))
	if department != "" && !aiDeptEsValido(department) {
		return fmt.Errorf("departamento desconocido: %s", department)
	}
	configPrompt := strings.TrimSpace(r.FormValue("agent_config_prompt"))

	_, err := tx.Exec(`INSERT INTO ai_agent (name, user_id, provider_id, model_override, role_id,
		operator_enabled, max_steps, max_rows, enabled, created_by, department, config_prompt, config_prompt_at)
		VALUES (?,?,?,'',?,0,8,200,1,?,?,?,?)`,
		nombre, userID, providerID, roleID, creator,
		department, nullableTexto(configPrompt), now())
	return err
}

// actualizarAgenteDeUsuarioTx actualiza rol, departamento y prompt del agente que
// acompaña a un usuario existente.
//
// Si no hay agente detrás y se marca como agente, no lo crea: crearlo aquí
// saltaría la comprobación de proveedor activo que hace crearAgenteParaUsuarioTx, y
// quedaría un agente que no puede trabajar. Para ese caso está el alta.
func (s *Server) actualizarAgenteDeUsuarioTx(tx *sql.Tx, r *http.Request, userID string) error {
	roleID, _ := strconv.ParseInt(r.FormValue("agent_role_id"), 10, 64)
	department := strings.ToLower(strings.TrimSpace(r.FormValue("agent_department")))
	if department != "" && !aiDeptEsValido(department) {
		return fmt.Errorf("departamento desconocido: %s", department)
	}
	configPrompt := strings.TrimSpace(r.FormValue("agent_config_prompt"))
	res, err := tx.Exec(`UPDATE ai_agent SET role_id = ?, department = ?, config_prompt = ?,
		config_prompt_at = ? WHERE user_id = ?`,
		roleID, department, nullableTexto(configPrompt), now(), userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("este usuario no tiene un agente asociado: créalo desde la pantalla de agentes")
	}
	return nil
}

// creatorDeRequest devuelve el usuario que está haciendo el cambio, para
// atribuirlo en la auditoría.
func (s *Server) creatorDeRequest(r *http.Request) string {
	return sessUserID(s.sessionFromRequest(r))
}
