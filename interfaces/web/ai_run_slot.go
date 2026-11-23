package web

// ai_run_slot.go — Una tarea programada corre UNA vez por período.
//
// EL FALLO QUE ESTO CIERRA.
//
// aiScheduleDue decidía «daily» comparando la HORA del reloj:
//
//	nowT.Hour() == parseHour(t.ScheduleValue, 6)
//
// Eso es verdadero en las DOCE vueltas del ticker de 5 minutos dentro de esa
// hora. El único freno era «si la última corrida de los últimos 6 minutos sigue
// en running, no encones», así que una tarea que terminaba en dos minutos se
// volvía a encolar cinco minutos después: hasta doce corridas por día para una
// tarea diaria, y doce veces los mismos documentos.
//
// Un cerrojo en memoria no sirve: el proceso se reinicia y el ticker vuelve a
// empezar. La ranura es un DATO con clave única: el período se reclama antes de
// insertar la corrida, y el segundo intento del mismo período no ejecuta nada.
// El ticker puede seguir siendo de 5 minutos.

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// errAIRunSlotTaken señala que la tarea ya corrió en este período.
var errAIRunSlotTaken = errors.New("la tarea ya se ejecutó en este período; no se repite")

// aiRunSlot calcula la ranura de una tarea para el momento dado.
//
// La granularidad es la del PERÍODO de la programación, no la del tick:
//   - daily  → un día
//   - weekly → una semana ISO
//   - cron5  → un minuto (su período es cada vuelta, a propósito)
//
// Devuelve "" para las tareas manuales: no tienen período que reclamar.
func aiRunSlot(t aiTask, nowT time.Time) string {
	switch t.ScheduleType {
	case "daily":
		return fmt.Sprintf("task:%d:daily:%s", t.ID, nowT.Format("2006-01-02"))
	case "weekly":
		year, week := nowT.ISOWeek()
		return fmt.Sprintf("task:%d:weekly:%04d-W%02d", t.ID, year, week)
	case "cron5":
		return fmt.Sprintf("task:%d:cron5:%s", t.ID, nowT.Format("2006-01-02T15:04"))
	}
	return ""
}

// aiRunScope es el ámbito de deduplicación de los documentos de una corrida.
//
// Comparte la ranura cuando la hay —así el reintento de la cola y una segunda
// vuelta del ticker caen en el mismo ámbito— y, en un disparo manual, usa
// tarea+día. Con eso, lanzar la misma tarea a mano dos veces el mismo día con los
// mismos datos tampoco duplica: el ámbito coincide y la clave de la herramienta
// también.
//
// Un disparo manual NO se bloquea por la ranura (un humano pidió ejecutar
// ahora); lo que se deduplica es el documento, no la corrida. Son dos preguntas
// distintas y mezclarlas daría un «ya corrió hoy» que nadie pidió.
func aiRunScope(t aiTask, runSlot string) string {
	if runSlot != "" {
		return runSlot
	}
	return fmt.Sprintf("task:%d:day:%s", t.ID, aiAgentClock().Format("2006-01-02"))
}

// aiClaimRunSlot reclama la ranura del período. Devuelve false si ya estaba.
//
// Es una sola sentencia atómica —la clave primaria de ai_run_slot decide— así que
// no hay ventana entre «mirar» y «escribir», que es justo donde dos vueltas del
// ticker, o dos procesos, se colarían a la vez.
func (s *Server) aiClaimRunSlot(db *sql.DB, slot string, taskID int64) (bool, error) {
	if slot == "" {
		return true, nil
	}
	if db == nil {
		return true, nil
	}
	_, err := db.Exec(`INSERT INTO ai_run_slot (slot_key, task_id, created_at) VALUES (?,?,?)`,
		slot, taskID, now())
	if err == nil {
		return true, nil
	}
	if aiEsClaveDuplicada(err) {
		return false, nil // ya corrió en este período
	}
	// Error real (tabla ausente, base caída): se informa y NO se ejecuta. En un
	// proceso que mueve stock y cuentas por pagar, no ejecutar es la dirección
	// segura cuando no se puede garantizar que no se duplique; y el error queda a
	// la vista en la cola, en vez de convertirse en documentos repetidos.
	return false, err
}

// aiReleaseRunSlot libera la ranura de una corrida que falló.
func (s *Server) aiReleaseRunSlot(db *sql.DB, slot string) {
	if db == nil || slot == "" {
		return
	}
	_, _ = db.Exec(`DELETE FROM ai_run_slot WHERE slot_key = ?`, slot)
}

// aiTaskRunReview gestiona una corrida que exigía revisión humana: aplica sus
// propuestas de acción crítica (F5) y, si quien llama es administrador, firma que
// la corrida está revisada.
//
// ANTES este endpoint solo ponía un sello: «marcar como revisada» una corrida en
// la que el agente YA había movido el dinero. Con la aprobación previa (ver
// aiGuardAction:RequiereAprobacionPrevia) la corrida puede tener propuestas
// pendientes —acciones críticas que el sistema se negó a ejecutar— y este es el
// sitio donde un humano las ejecuta.
//
// Cuatro reglas hacen que «aprobar» no sea una puerta trasera:
//
//  1. El aprobador necesita el PERMISO REAL de la herramienta: exigirPermiso sobre
//     la MISMA declaración (directorio + acción) que usa el dispatcher. Se corrige
//     así el criterio antiguo, que era «administrador»: el administrador puede todo
//     en la interfaz, así que su clic no demostraba nada. Ahora un jefe de cartera
//     con `manage_invoice/update` puede aprobar el cobro propuesto, y quien no
//     lo tiene recibe 403 sin que se toque nada.
//  2. La herramienta corre con la SESIÓN DEL HUMANO: el documento, su empresa y su
//     auditoría nacen con la autoría de quien aprobó, no con la del agente.
//  3. El efecto va DETRÁS del CLAIM (`pending` → `applying` en una sola sentencia):
//     dos clics simultáneos, o dos personas sobre la misma propuesta, no generan
//     dos asientos.
//  4. Cada aprobación queda auditada con `tool.approved` (quién, qué propuesta, de
//     qué corrida). «Lo hizo la IA» deja de ser explicable: lo autorizó una persona.
//
// Firmar la corrida como revisada sigue siendo acto de administrador: es decir que
// se leyó el informe, no ejecutar nada.
func (s *Server) aiTaskRunReview(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión no iniciada", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	db := s.dbFor(r)
	esAdmin := sess.UserLevel == "admin" || sess.UserLevel == "master"

	// RETIRAR UNA PROPUESTA. `rechazar` con el id de una propuesta de esta corrida
	// la cierra sin ejecutarla. Hace falta porque lo contrario no es neutro: una
	// propuesta que nadie aprueba ni retira se queda pendiente indefinidamente y
	// cualquiera con el permiso podría aplicarla mañana. Se exige el MISMO permiso
	// que para aplicarla —quien puede hacer la acción puede cancelarla— y no se toca
	// nada más en la petición: la retirada es el único efecto.
	if idRechazo, errRechazo := strconv.ParseInt(strings.TrimSpace(r.FormValue("rechazar")), 10, 64); errRechazo == nil && idRechazo > 0 {
		herramienta, encontrada := s.aiPropuestaPendienteDeCorrida(db, id, idRechazo)
		if !encontrada {
			jsonError(w, "La propuesta no es de esta corrida, ya no está pendiente o no existe.", http.StatusNotFound)
			return
		}
		if tool, existe := aiToolByName[herramienta]; existe && tool.Permission.Directory != "" {
			if !s.exigirPermiso(w, r, tool.Permission.Action, tool.Permission.Directory) {
				return
			}
		} else if !esAdmin {
			// Sin permiso declarado no hay a quién comprobarle la autoridad: solo
			// un administrador puede retirar la propuesta.
			jsonError(w, "La herramienta de esta propuesta no declara permiso: solo un administrador puede retirarla.", http.StatusForbidden)
			return
		}
		if !s.aiRechazarPropuesta(db, idRechazo, sessUserID(sess)) {
			// Alguien la aplicó o la retiró entre la lectura y la escritura: no se
			// pisa la decisión de esa persona.
			jsonError(w, "La propuesta ya fue aplicada o retirada por otra persona.", http.StatusConflict)
			return
		}
		s.aiAudit(sess, sessUserID(sess), "tool.rejected", "ai_tasks", "ai_tool_proposal",
			fmt.Sprintf("%d", idRechazo), nil, map[string]interface{}{"tool": herramienta, "run_id": id})
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Propuesta retirada sin ejecutar", "rechazada": idRechazo})
		return
	}

	// PRE-PASE DE PERMISOS — «todo o nada». Si una corrida tiene tres propuestas y
	// la sesión no puede aprobar la tercera, aplicar las dos primeras y fallar en la
	// tercera dejaría a medias exactamente el lote que el agente pidió junto. Se
	// comprueba primero la autoridad sobre TODAS y solo después se ejecuta.
	propuestas := s.aiPropuestasDeCorrida(db, id)
	for _, p := range propuestas {
		if p.Status != aiEstadoPropuestaPendiente {
			continue
		}
		tool, ok := aiToolByName[p.Tool]
		if !ok || tool.Run == nil {
			continue // no es aprobable ahora: se reporta abajo, no se ejecuta
		}
		if tool.Permission.Directory == "" {
			jsonError(w, fmt.Sprintf(
				"la propuesta de '%s' no declara permiso en el catálogo: no se puede comprobar la autoridad de quien la aprueba, así que ninguna propuesta de esta corrida se aplica",
				tool.Name), http.StatusForbidden)
			return
		}
		if !s.hasPermission(r, tool.Permission.Directory, tool.Permission.Action) {
			jsonError(w, fmt.Sprintf(
				"la propuesta de '%s' exige el permiso «%s» sobre «%s», que su rol no tiene: no se aplica ninguna propuesta de esta corrida",
				p.Tool, tool.Permission.Action, tool.Permission.Directory), http.StatusForbidden)
			return
		}
	}

	aplicadas, pendientes := 0, 0
	fallidas := []string{}
	for _, p := range propuestas {
		if p.Status != aiEstadoPropuestaPendiente {
			continue
		}
		tool, ok := aiToolByName[p.Tool]
		if !ok || tool.Run == nil {
			// Herramienta retirada del catálogo o nunca implementada: no hay nada
			// que aprobar. Queda pendiente y se reporta, no se ejecuta a ciegas.
			pendientes++
			continue
		}
		// Segunda exigencia, con la puerta canónica del resto de la interfaz
		// (exigirPermiso). No es redundancia: es la comprobación que escribiría el
		// 403 si alguien quitara el pre-pase de arriba, y es la misma llamada que
		// hace el handler humano de la pantalla equivalente.
		if !s.exigirPermiso(w, r, tool.Permission.Action, tool.Permission.Directory) {
			return
		}
		prop, claimed, cErr := s.aiReclamarPropuesta(db, p.ID)
		if cErr != nil {
			fallidas = append(fallidas, fmt.Sprintf("%s: %v", p.Tool, cErr))
			pendientes++
			continue
		}
		if !claimed {
			continue // otra persona la está aplicando ahora mismo
		}
		out, runErr := s.aiAplicarPropuesta(sess, humanoQueAprueba(sess, prop), prop, &aiRunJournal{
			// scope: el ámbito de la PROPIA propuesta. Si se usara el de la corrida,
			// aprobar la propuesta coincidiría con la clave que el agente habría
			// calculado al pedirla y el libro de idempotencia contestaría «ya se
			// ejecutó» —cuando todavía no se ejecutó nada.
			runID:   id,
			agentID: prop.AgentID,
			scope:   fmt.Sprintf("proposal:%d", prop.ID),
		})
		if runErr != nil {
			s.aiCerrarPropuesta(db, prop.ID, aiEstadoPropuestaFallo, out, sessUserID(sess))
			fallidas = append(fallidas, fmt.Sprintf("%s: %v", p.Tool, runErr))
			continue
		}
		s.aiCerrarPropuesta(db, prop.ID, aiEstadoPropuestaAplicada, out, sessUserID(sess))
		s.aiAudit(sess, sessUserID(sess), "tool.approved", "ai_tasks", "ai_tool_proposal",
			fmt.Sprintf("%d", prop.ID), nil, map[string]interface{}{"tool": prop.Tool, "run_id": id})
		aplicadas++
	}

	if !esAdmin {
		if len(fallidas) > 0 {
			writeJSON(w, map[string]interface{}{"status": 0, "aplicadas": aplicadas,
				"fallidas": fallidas,
				"msg":      "Algunas propuestas no se pudieron aplicar: " + strings.Join(fallidas, "; ")})
			return
		}
		writeJSON(w, map[string]interface{}{"status": 1, "aplicadas": aplicadas,
			"msg": fmt.Sprintf("%d propuesta(s) aplicada(s)", aplicadas)})
		return
	}

	res, err := db.Exec(`UPDATE ai_task_run SET reviewed_by=?, reviewed_at=?
		WHERE id=? AND needs_human=1 AND IFNULL(reviewed_by,'')=''`,
		sessUserID(sess), now(), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	marco := false
	if n, _ := res.RowsAffected(); n > 0 {
		marco = true
		s.aiAudit(sess, sessUserID(sess), "task.reviewed", "ai_tasks", "ai_task_run", fmt.Sprintf("%d", id), nil, nil)
	}
	if !marco && aplicadas == 0 && pendientes == 0 && len(fallidas) == 0 {
		jsonError(w, "La corrida no requiere revisión o ya fue revisada", 400)
		return
	}
	msg := "Corrida marcada como revisada"
	if aplicadas > 0 {
		msg = fmt.Sprintf("%d propuesta(s) aplicada(s); corrida revisada", aplicadas)
	}
	salida := map[string]interface{}{"status": 1, "msg": msg,
		"aplicadas": aplicadas, "revisada": marco}
	if len(fallidas) > 0 {
		salida["fallidas"] = fallidas
	}
	if pendientes > 0 {
		salida["pendientes"] = pendientes
	}
	writeJSON(w, salida)
}

// humanoQueAprueba traduce la sesión del aprobador a la identidad con la que las
// herramientas escriben: el id del agente se conserva (es el rastro de QUÉ se
// pidió y para qué corrida), pero el usuario que firma el documento es el humano.
// Así `sales_by`, el asiento y la auditoría dicen quién ejecutó de verdad.
func humanoQueAprueba(sess *SessionData, prop aiPropuestaPendiente) aiAgent {
	return aiAgent{
		ID:       prop.AgentID,
		UserID:   sessUserID(sess),
		Name:     sess.Username,
		RoleName: sess.UserLevel,
	}
}
