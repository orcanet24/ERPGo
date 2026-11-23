package web

import (
	"database/sql"
	"fmt"
	"log"
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
	// La empresa activa la pone el render (CompanyName): la lista ya es SU lista,
	// y sin el nombre a la vista el administrador no puede saber a qué empresa le
	// está dando de alta a la gente.
	s.render(w, r, "user_list.html", map[string]interface{}{"Title": "Usuarios"})
}

// userListData lista los usuarios de la EMPRESA ACTIVA.
//
// Antes listaba todos los usuarios de la base de control sin filtrar: cada
// empresa veía las identidades de todas las demás y un usuario dado de alta en
// una aparecía en la lista de la otra, que es el «salto» que se corrige aquí.
//
// La pertenencia se comprueba en tenant_users (la autoridad del login y del
// selector de empresa) y, como compatibilidad, en user_company_access (la que
// escribía el alta anterior). La bandera `super` marca al dueño del sistema para
// que la plantilla no le ofrezca editar ni borrar: esa identidad no se toca desde
// el panel.
func (s *Server) userListData(w http.ResponseWriter, r *http.Request) {
	empresa, err := s.empresaDeLaSesion(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
		return
	}
	ctl := s.authDB(r)
	rows, err := ctl.Query(`
		SELECT u.user_id, u.first_name, u.last_name, IFNULL(u.phone,''), ul.username,
		       IFNULL(ul.user_type, 2), u.status
		FROM users u
		JOIN user_login ul ON ul.user_id = u.user_id
		WHERE EXISTS (SELECT 1 FROM tenant_users tu
		              WHERE tu.tenant_code = ? AND tu.username = ul.username AND tu.status = 1)
		   OR EXISTS (SELECT 1 FROM user_company_access uca
		              WHERE uca.tenant_code = ? AND uca.user_id = u.user_id)
		ORDER BY u.id`, empresa, empresa)
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
			"super": usuarioTieneRolSuperAdmin(ctl, uid),
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
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
	// La empresa de la sesión es la que va a recibir al usuario: se muestra en el
	// formulario para que el administrador vea DÓNDE lo está creando antes de
	// guardar. Sin ese dato a la vista, el alta en la empresa equivocada era un
	// error invisible hasta que el usuario intentaba entrar desde otra.
	empresa, _ := s.empresaDeLaSesion(r)
	data := map[string]interface{}{
		"Title":         "Añadir Usuario",
		"Departamentos": opcionesDeDepartamentos(),
		"Roles":         s.rolesAsignablesParaUsuario(r),
	}
	if id := r.PathValue("id"); id != "" {
		ctl := s.authDB(r)
		// El super administrador no se toca desde el panel: su credencial se
		// restablece con el binario (--setup) y su rol no se reasigna. Dejarlo
		// editar aquí abriría la puerta que este archivo cierra por el otro lado.
		if usuarioTieneRolSuperAdmin(ctl, id) {
			http.Error(w, "El super administrador no se administra desde el panel: "+
				"su credencial se restablece con el binario en modo --setup.", http.StatusForbidden)
			return
		}
		// Un administrador solo edita usuarios de SU empresa. Sin esta
		// comprobación bastaba con cambiar el id de la URL para editar la
		// identidad de otra empresa.
		if empresa != "" && !usuarioEsMiembroDeEmpresa(ctl, id, empresa) {
			http.Error(w, "Ese usuario no pertenece a la empresa activa.", http.StatusForbidden)
			return
		}
		var uid, first, last, phone, username sql.NullString
		var utype, status int
		err := ctl.QueryRow(`
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
		if err := ctl.QueryRow(`SELECT role_id, department, IFNULL(config_prompt,'')
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
	id := strings.TrimSpace(r.FormValue("user_id"))
	utype, _ := strconv.Atoi(orDefault(r.FormValue("user_type"), "2"))

	if first == "" || username == "" {
		jsonError(w, "Nombre y usuario son obligatorios", 400)
		return
	}

	// LA EMPRESA SALE DE LA SESIÓN. Antes se preguntaba «cuál es el primer tenant
	// activo» y ese era el destino de todo usuario nuevo: la empresa maestra o la
	// demo, nunca la que el administrador tenía abierta. La consulta ordenada por
	// id no puede saber en qué empresa está trabajando nadie.
	empresa, err := s.empresaDeLaSesion(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
		return
	}

	ctl := s.authDB(r)

	// El rol se valida ANTES de abrir la transacción y contra el catálogo: id
	// inexistente o rol reservado del super administrador se rechazan aquí, no
	// después de haber creado la identidad.
	rolID, err := rolElegidoParaUsuario(ctl, r.FormValue("role_id"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
		return
	}

	if id != "" {
		if usuarioTieneRolSuperAdmin(ctl, id) {
			jsonError(w, "El super administrador no se edita desde el panel: "+
				"su credencial se restablece con el binario en modo --setup.", http.StatusForbidden)
			return
		}
		if !usuarioEsMiembroDeEmpresa(ctl, id, empresa) {
			jsonError(w, "Ese usuario no pertenece a la empresa activa", http.StatusForbidden)
			return
		}
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

	tx, err := ctl.Begin()
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
		// OJO: asignación, no declaración corta. Aquí había un `nuevoUID :=`
		// dentro del bloque, que dejaba en cero la variable de fuera: el rol
		// terminaba llamándose con user_id vacío y el usuario nacía sin rol
		// aunque el formulario lo trajera.
		nuevoUID = fmt.Sprintf("%d", newID)
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

		// Pertenece a la empresa donde se está creando, y solo a esa. La
		// identidad es global; la pertenencia no.
		if err := vincularUsuarioAEmpresaTx(tx, uid, username, empresa, rolID); err != nil {
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
	if err := s.asignarRolUsuarioTx(tx, uid, rolID); err != nil {
		jsonError(w, "no se pudo asignar el rol: "+err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// S-3 (auditoría 2026-11-22): si la edición cambió clave o rol, las
	// sesiones vivas del usuario quedan cerradas. Sin esto, un empleado con
	// sesión seguía trabajando con el nivel cacheado de la sesión vieja
	// hasta las 24 horas aunque el panel lo acabara de degradar.
	if cerradas := s.sessions.DeleteByUser(uid); cerradas > 0 {
		log.Printf("usuario %s: %d sesión(es) revocada(s) al guardar sus datos", uid, cerradas)
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
// roleID 0 NO borra el rol: significa «no lo toques». Borrarlo cuando el
// formulario no trae el campo convertiría a un usuario delegado en alguien sin
// permisos por el simple hecho de editarle el teléfono.
func (s *Server) asignarRolUsuarioTx(tx *sql.Tx, userID string, roleID int64) error {
	if roleID <= 0 || strings.TrimSpace(userID) == "" {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM sec_userrole WHERE user_id = ?`, userID); err != nil {
		return err
	}
	var siguiente int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(id),0)+1 FROM sec_userrole`).Scan(&siguiente); err != nil {
		siguiente = 1
	}
	_, err := tx.Exec(`INSERT INTO sec_userrole (id, user_id, roleid, createby, createdate)
		VALUES (?,?,?,?,?)`, siguiente, userID, roleID, "system", now())
	return err
}

// userDelete quita un usuario de la empresa activa.
//
// Distingue dos casos a propósito:
//
//   - el usuario pertenece a otras empresas: se le quita SOLO esta. Borrar la
//     identidad global le quitaría el acceso a las demás y eso es exactamente el
//     salto entre empresas que no debe pasar;
//   - era su última empresa: se elimina la identidad (credencial y usuario), que
//     es lo que el administrador espera cuando dice «eliminar».
//
// En los dos casos se comprueba antes que el usuario sea de esta empresa y que no
// sea el super administrador: esa identidad solo se toca desde la instalación.
func (s *Server) userDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.sessionFromRequest(r)
	if sess != nil && sess.UserID == id {
		jsonError(w, "No puede eliminar su propio usuario", 400)
		return
	}
	empresa, err := s.empresaDeLaSesion(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
		return
	}
	ctl := s.authDB(r)
	if usuarioTieneRolSuperAdmin(ctl, id) {
		jsonError(w, "El super administrador no se elimina desde el panel: "+
			"se administra con el binario en modo --setup.", http.StatusForbidden)
		return
	}
	if !usuarioEsMiembroDeEmpresa(ctl, id, empresa) {
		jsonError(w, "Ese usuario no pertenece a la empresa activa", http.StatusForbidden)
		return
	}

	var username string
	var utype int
	if err := ctl.QueryRow(`SELECT IFNULL(username,''), IFNULL(user_type, 2) FROM user_login WHERE user_id = ?`,
		id).Scan(&username, &utype); err != nil {
		jsonError(w, "Usuario no encontrado", 404)
		return
	}

	// La empresa no puede quedarse sin administrador. Se cuentan los de ESTA
	// empresa (antes se contaba el total del sistema, que no dice nada de ella).
	if utype == 1 {
		var otros int
		_ = ctl.QueryRow(`SELECT COUNT(*)
			FROM user_login ul
			JOIN tenant_users tu ON tu.username = ul.username AND tu.tenant_code = ?
			WHERE ul.user_type = 1 AND ul.status = 1 AND ul.user_id <> ?`, empresa, id).Scan(&otros)
		if otros == 0 {
			jsonError(w, "Debe existir al menos un administrador en la empresa", 400)
			return
		}
	}

	// Se leen las empresas ANTES de abrir la transacción: consultar el mismo
	// handle con una transacción abierta espera una conexión libre y, con un
	// pool de una sola (las pruebas y las instalaciones SQLite), se queda
	// esperando para siempre.
	empresas := empresasDelUsuario(ctl, id)

	tx, err := ctl.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if err := desvincularUsuarioDeEmpresaTx(tx, id, username, empresa); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if len(empresas) > 1 {
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// S-3: la sesión podía estar montada en la empresa recién quitada;
		// se cierran todas y el usuario entra de nuevo eligiendo empresa.
		s.sessions.DeleteByUser(id)
		writeJSON(w, map[string]interface{}{
			"status": 1,
			"msg":    "Se le quitó el acceso a esta empresa; sigue perteneciendo a " + strings.Join(otrasEmpresas(empresas, empresa), ", "),
		})
		return
	}

	// Última empresa: se borra la identidad completa, incluido su rol.
	if _, err := tx.Exec(`DELETE FROM sec_userrole WHERE user_id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
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
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// S-3 (auditoría 2026-11-22): la baja es efectiva en el acto. Antes la
	// identidad desaparecía del panel pero la cookie del usuario seguía
	// operativa hasta 24 horas, y en el POS hasta 12.
	s.sessions.DeleteByUser(id)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/user/list")})
}

// otrasEmpresas devuelve las empresas de la lista menos la que se acaba de
// quitar. Se usa solo para el mensaje: «sigue perteneciendo a X, Y».
func otrasEmpresas(empresas []string, quitada string) []string {
	out := make([]string, 0, len(empresas))
	for _, code := range empresas {
		if code != quitada {
			out = append(out, code)
		}
	}
	return out
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
	// S-3 (auditoría 2026-11-22): cambiar la clave revoca TODAS las sesiones
	// del usuario —incluida esta—. Es la respuesta al escenario real: el
	// dueño detecta que alguien usó su cuenta, cambia la clave y antes el
	// atacante seguía dentro hasta que expirara la cookie. El mensaje avisa
	// para que no parezca un fallo del sistema.
	cerradas := s.sessions.DeleteByUser(sess.UserID)
	msg := "Contraseña actualizada"
	if cerradas > 0 {
		msg = "Contraseña actualizada. Por seguridad se cerraron sus sesiones: vuelva a iniciar sesión."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg, "redirect": saveRedirect(r, "/user/list")})
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

	// Guarda de propiedad: /user/photo es ruta de sesión (la foto del propio
	// perfil), así que escribir la foto de OTRO usuario exige el permiso del
	// módulo de usuarios (o nivel admin/master, que canAccessModule ya
	// reconoce). Sin esto, cualquier cajero podía cambiar la foto de cualquier
	// empleado del sistema (auditoría 2026-11-22, rutas de sesión).
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "No autorizado. Inicie sesión.", http.StatusUnauthorized)
		return
	}
	if userID != sess.UserID {
		if !s.hasModulePermission(r, sess, 14, "update") {
			jsonError(w, "Solo el administrador puede cambiar la foto de otro usuario", http.StatusForbidden)
			return
		}
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

// roleListData lista el catálogo de roles y marca el reservado.
//
// El rol del super administrador se sigue mostrando —existe y hay que poder
// verlo— pero con la marca `reservado` para que la pantalla no ofrezca editarlo,
// borrarlo ni cambiarle la matriz. La lista de roles es del sistema, no de una
// empresa: vive en la base de control, la misma que usa el login.
func (s *Server) roleListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.authDB(r).Query(`
		SELECT r.id, r.type, IFNULL(r.level,''), IFNULL(r.is_system,0), COUNT(ur.id) AS users
		FROM sec_role r LEFT JOIN sec_userrole ur ON ur.roleid = r.id
		GROUP BY r.id, r.type, r.level, r.is_system`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id, users, sistema int
		var typ, level string
		_ = rows.Scan(&id, &typ, &level, &sistema, &users)
		list = append(list, map[string]interface{}{
			"id": id, "type": typ, "level": level, "users": users,
			"system": sistema == 1, "reservado": esRolReservado(level),
		})
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
	// El nivel master no se crea ni se edita desde el panel: es el del super
	// administrador y solo lo pone la instalación. Antes bastaba con elegirlo en
	// este desplegable para fabricar un segundo dueño del sistema.
	if strings.EqualFold(level, nivelSuperAdmin) {
		jsonError(w, errSuperAdminReservado.Error(), http.StatusForbidden)
		return
	}
	validLevels := map[string]bool{"admin": true, "manager": true, "employee": true}
	if !validLevels[level] {
		level = "employee"
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" {
		var nivelActual string
		if err := s.authDB(r).QueryRow(`SELECT IFNULL(level,'') FROM sec_role WHERE id = ?`, id).
			Scan(&nivelActual); err != nil {
			jsonError(w, "Rol no encontrado", 404)
			return
		}
		if esRolReservado(nivelActual) {
			jsonError(w, errSuperAdminReservado.Error(), http.StatusForbidden)
			return
		}
		if _, err := s.authDB(r).Exec(`UPDATE sec_role SET type=?, level=? WHERE id=?`, typ, level, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		if _, err := s.authDB(r).Exec(`INSERT INTO sec_role (type, level) VALUES (?, ?)`, typ, level); err != nil {
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
	// El rol del super administrador no tiene matriz que editar: su nivel master
	// ya concede todo y tocarlo solo puede quitarle algo al dueño del sistema.
	if esRolReservado(level) {
		http.Error(w, errSuperAdminReservado.Error(), http.StatusForbidden)
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
	// El rol reservado no se borra ni teniendo cero usuarios: sin él, el nivel
	// master del dueño del sistema dejaría de resolverse.
	var nivel string
	if err := s.authDB(r).QueryRow(`SELECT IFNULL(level,'') FROM sec_role WHERE id = ?`, id).Scan(&nivel); err == nil {
		if esRolReservado(nivel) {
			jsonError(w, errSuperAdminReservado.Error(), http.StatusForbidden)
			return
		}
	}
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
	if err := rechazarRolReservado(s.authDB(r), roleID); err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
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
