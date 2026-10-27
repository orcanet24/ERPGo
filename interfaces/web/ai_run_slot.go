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

// aiTaskRunReview marca como revisada una corrida que exigía revisión humana.
//
// Es el acto que antes no existía: el guardián calculaba «esto lo debía aprobar
// un humano» y no había forma de que un humano lo diera por visto. No ejecuta ni
// deshace nada —las acciones ya se hicieron y siguen siendo reversibles con
// «Revertir»—: deja constancia de quién lo revisó, con la misma auditoría que el
// resto del módulo, para que la marca no se quede encendida para siempre.
func (s *Server) aiTaskRunReview(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil || (sess.UserLevel != "admin" && sess.UserLevel != "master") {
		jsonError(w, "Solo un administrador puede revisar una corrida", 403)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, "Identificador inválido", 400)
		return
	}
	res, err := s.dbFor(r).Exec(`UPDATE ai_task_run SET reviewed_by=?, reviewed_at=?
		WHERE id=? AND needs_human=1 AND IFNULL(reviewed_by,'')=''`,
		sessUserID(sess), now(), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "La corrida no requiere revisión o ya fue revisada", 400)
		return
	}
	s.aiAudit(sess, sessUserID(sess), "task.reviewed", "ai_tasks", "ai_task_run", fmt.Sprintf("%d", id), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Corrida marcada como revisada"})
}
