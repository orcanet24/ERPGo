package web

// ai_permissions.go — El permiso del rol, comprobado DENTRO del despacho.
//
// POR QUÉ EXISTE ESTE ARCHIVO.
//
// El comentario de ai_agents.go decía «sus permisos son exactamente los de su
// rol», y era cierto para la web y falso para las herramientas: ningún ai_*.go
// llamaba a hasPermission. Un agente con operador activo podía crear un voucher
// o registrar un cobro aunque su rol no tuviera ese permiso en la interfaz. La
// única puerta era OperatorOnly y el departamento —que solo acota LECTURAS de
// SQL—, así que la escritura no pasaba por el rol en ningún momento.
//
// LA COMPROBACIÓN NO SE RESUELVE LLAMANDO A hasPermission. Esa función —y
// roleDBs— reciben un *http.Request, y el runner corre desde el ticker y desde
// la cola, sin petición y a veces minutos después del clic. Lo que sí viaja es
// la sesión del agente. De ahí esta variante, que reutiliza la MISMA consulta y
// las MISMAS dos bases (empresa y control) que usa la web: si el rol concede el
// permiso para pintar el botón, lo concede para la herramienta, y al revés.

import "database/sql"

// aiToolPermission es el permiso del rol que exige una herramienta.
//
// Directory vacío significa «sin permiso de rol». Eso cubre dos casos, y los dos
// son deliberados:
//
//   - Lecturas: ya están acotadas por el departamento del rol
//     (aiDeptDelAgente) en el guardián de SQL. Añadirles un segundo candado por
//     directorio no aporta seguridad y sí rompe flujos que hoy funcionan (el
//     diseñador de informes llama al agente por defecto, cuyo rol no tiene por
//     qué incluir `custom_reports` para LEER).
//   - Propuestas: no escriben nada, devuelven un borrador para que lo ejecute
//     un humano.
//
// La ESCRITURA sí lo exige, y esa es la regla que cierra el hueco: una
// herramienta que escribe y no declara permiso se rechaza por omisión, igual que
// una herramienta que no está en el registro.
type aiToolPermission struct {
	Directory string
	Action    string // create | read | update | delete
}

// aiRoleGate son los permisos del rol de un agente, resueltos UNA vez por
// corrida.
//
// Se resuelve una vez y no por herramienta porque el bucle del modelo puede
// llamar a ocho herramientas por corrida y cada consulta cuesta dos viajes a la
// base. Además, así todas las herramientas de una misma corrida ven el mismo
// veredicto: si un administrador cambia un rol a mitad de corrida, la corrida
// termina con la foto con la que empezó, que es lo que hace la auditoría
// legible.
type aiRoleGate struct {
	level string
	// granted[directory][action] = true. Solo se guarda lo concedido: la
	// ausencia es la denegación, igual que en role_permission.
	granted map[string]map[string]bool
}

// aiBuildRoleGate resuelve el nivel y la matriz de permisos del usuario del
// agente.
//
// Consulta las dos bases, como hasPermission: el rol se asigna en la empresa o
// en la base de control según la instalación, y basta con que UNA lo conceda.
// La web ya se tropezó con esto —el menú leía control y el middleware leía la
// empresa, y el usuario veía el módulo pero recibía 403 (permissions.go:251)—;
// repetir el error aquí sería repetir el síntoma.
func (s *Server) aiBuildRoleGate(sess *SessionData, agent aiAgent) aiRoleGate {
	gate := aiRoleGate{granted: map[string]map[string]bool{}}
	if agent.UserID == "" {
		return gate
	}
	for _, db := range s.roleDBsForSession(sess) {
		if db == nil {
			continue
		}
		if gate.level == "" {
			if level, err := loadUserRole(db, agent.UserID); err == nil {
				gate.level = level
			}
		}
		rows, err := db.Query("SELECT sm.directory, rp.`create`, rp.`read`, rp.`update`, rp.`delete`\n"+
			"FROM sec_userrole ur\n"+
			"JOIN role_permission rp ON rp.role_id = ur.roleid\n"+
			"JOIN sub_module sm ON sm.id = rp.fk_module_id\n"+
			"WHERE ur.user_id = ? AND sm.status = 1", agent.UserID)
		if err != nil {
			continue
		}
		for rows.Next() {
			var dir string
			var c, r, u, d sql.NullInt64
			if rows.Scan(&dir, &c, &r, &u, &d) != nil {
				continue
			}
			for action, v := range map[string]sql.NullInt64{"create": c, "read": r, "update": u, "delete": d} {
				if v.Int64 != 1 {
					continue
				}
				if gate.granted[dir] == nil {
					gate.granted[dir] = map[string]bool{}
				}
				gate.granted[dir][action] = true
			}
		}
		rows.Close()
	}
	return gate
}

// roleDBsForSession es roleDBs sin *http.Request.
//
// Existe por la misma razón que dbForSession: el runner no tiene petición. El
// orden —empresa primero, control después— es el que ya usa la web, para que la
// respuesta sea la misma mirando desde los dos lados.
func (s *Server) roleDBsForSession(sess *SessionData) []*sql.DB {
	out := make([]*sql.DB, 0, 2)
	if db := s.dbForSession(sess); db != nil {
		out = append(out, db)
	}
	if ctl, err := s.controlStore(); err == nil && ctl != nil {
		repetida := false
		for _, db := range out {
			if db == ctl {
				repetida = true
				break
			}
		}
		if !repetida {
			out = append(out, ctl)
		}
	}
	return out
}

// allows responde si el rol del agente concede el permiso pedido.
//
// Las reglas son las MISMAS que aplica la web en canAccessModule/hasPermission,
// y por eso el resultado coincide con lo que el usuario ve en pantalla:
//
//   - master y admin: todo. No es un privilegio nuevo, es el bypass que ya tiene
//     la interfaz.
//   - employee: solo lectura. Un agente con rol de empleado no escribe.
//   - cualquier otro nivel: exactamente lo que diga role_permission.
func (g aiRoleGate) allows(dir, action string) bool {
	if dir == "" {
		return true
	}
	if g.level == "admin" || g.level == "master" {
		return true
	}
	if action == "" {
		action = "read"
	}
	if g.level == "employee" && action != "read" {
		return false
	}
	return g.granted[dir][action]
}

// allowsTool es la regla ÚNICA: la usan el guardián y el catálogo anunciado, para
// que no se pueda ofrecer al modelo una herramienta que después el guardián
// rechace. Un catálogo que promete lo que el guardián niega gasta pasos, tokens y
// la paciencia del usuario.
func (g aiRoleGate) allowsTool(tool aiToolDef) bool {
	if tool.Permission.Directory == "" {
		return true
	}
	return g.allows(tool.Permission.Directory, tool.Permission.Action)
}

// explainPermission arma el motivo del bloqueo con lo que hace falta para
// arreglarlo: qué permiso, en qué entrada del menú y con qué nivel. Un «permiso
// denegado» a secas obliga a adivinar.
func (g aiRoleGate) explainPermission(tool aiToolDef) string {
	accion := tool.Permission.Action
	if accion == "" {
		accion = "read"
	}
	nivel := "sin rol asignado"
	if g.level != "" {
		nivel = "nivel " + g.level
	}
	return "permiso denegado: el rol del agente (" + nivel + ") no tiene '" + accion +
		"' en '" + tool.Permission.Directory + "'. Se concede en Roles y vale igual para la interfaz y para la herramienta."
}
