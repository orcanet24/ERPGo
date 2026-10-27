package web

// ai_idempotency.go — Una acción de la IA no se ejecuta dos veces por el mismo motivo.
//
// EL FALLO QUE ESTO CIERRA.
//
// El camino de la IA no referenciaba ningún documento externo: no había
// external_ref ni equivalente. Una tarea que carga documentos desde un listado,
// corrida dos veces, duplicaba todo. Y había tres formas de correrla dos veces:
// el ticker de 5 minutos (que reencolaba una tarea «daily» en cada vuelta de su
// hora), el reintento de la cola (MaxAttempts: 3, cada intento con corrida y
// journal nuevos) y un segundo «Ejecutar ahora».
//
// POR QUÉ NO SE RESUELVE CON EL JOURNAL. El journal guarda compensaciones para
// deshacer; no dice «esto ya se hizo». Revertir una corrida duplicada compensa
// su parte y deja las otras, que es exactamente lo que no queremos.
//
// LA SOLUCIÓN ES LA MISMA QUE YA USA EL POS. `pos_sale_idempotency` reclama la
// venta con una clave única y, si ya estaba, devuelve el documento original
// (internal/pos/grpc_server.go). Aquí se hace igual, con una tabla propia y una
// clave derivada por el SERVIDOR: el modelo propone la llamada, no la clave.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// aiToolClaim es el resultado de intentar reclamar una ejecución.
type aiToolClaim struct {
	// Key es la clave de idempotencia calculada. Se usa para cerrar el reclamo.
	Key string
	// Skip indica que NO hay que ejecutar: la acción ya existe.
	Skip bool
	// Result es la respuesta que se devuelve al modelo cuando Skip.
	Result string
}

// aiIdemKey calcula la clave de idempotencia de una llamada.
//
// Dos formas, y el orden importa:
//
//  1. Con `external_ref`: la clave es (herramienta, referencia). Es la forma
//     FUERTE, y la que hay que usar cuando el documento viene de un listado, un
//     correo o un sistema externo: da igual cuántas veces se corra la tarea, el
//     documento se crea una sola vez, incluso en días distintos.
//  2. Sin `external_ref`: la clave es (ámbito de la corrida, herramienta,
//     argumentos). El ámbito lo pone el SERVIDOR —la ranura de programación o,
//     en un disparo manual, tarea+día—, así que el reintento de la cola y un
//     segundo «Ejecutar ahora» con los mismos datos no duplican.
//
// Los argumentos se serializan con encoding/json, que ordena las claves de un
// map: la misma llamada produce SIEMPRE el mismo texto y por tanto la misma
// clave. Sin eso, el orden de iteración de un map en Go bastaría para que dos
// llamadas idénticas no se reconocieran entre sí.
func aiIdemKey(tool, scope, externalRef string, args map[string]interface{}) string {
	if ref := strings.TrimSpace(externalRef); ref != "" {
		return aiHashKey("ref|" + tool + "|" + strings.ToLower(ref))
	}
	canon, _ := json.Marshal(args)
	return aiHashKey("run|" + scope + "|" + tool + "|" + string(canon))
}

// aiHashKey devuelve la clave en hexadecimal (64 caracteres, cabe en varchar(160)).
func aiHashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// aiClaimTool intenta reclamar la ejecución. Si la clave ya existe, NO se
// ejecuta: se devuelve el resultado original, o un aviso de que está en curso.
//
// El reclamo es un INSERT IGNORE: una sola sentencia atómica, sin ventana entre
// «mirar» y «escribir» —que es justo la ventana en la que dos corridas
// concurrentes se cuelan—. Si el efecto posterior falla, la fila queda reclamada
// a propósito: en un proceso que mueve stock y cuentas por pagar es preferible
// que una acción no se repita a que se duplique, y el error queda en la bitácora
// de la corrida para que un humano lo vea.
func (s *Server) aiClaimTool(db *sql.DB, j *aiRunJournal, tool, externalRef string, args map[string]interface{}) aiToolClaim {
	if j == nil {
		// Sin journal no hay ámbito: no se inventa una clave compartida entre
		// corridas. Se ejecuta como antes y se deja constancia en el código, en
		// vez de deduplicar contra una clave vacía que bloquearía todo.
		return aiToolClaim{}
	}
	key := aiIdemKey(tool, j.idemScope(), externalRef, args)
	claim := aiToolClaim{Key: key}
	if db == nil {
		return claim
	}

	ref := strings.TrimSpace(externalRef)
	_, err := db.Exec(`INSERT INTO ai_tool_execution
		(idem_key, tool, agent_id, run_id, task_id, external_ref, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?, 'claimed', ?, ?)`,
		key, tool, j.agentID, j.runID, j.taskID, ref, now(), now())
	if err == nil {
		return claim // reclamada: adelante
	}
	if !aiEsClaveDuplicada(err) {
		// Error distinto de «ya estaba» (tabla ausente, base caída). Se ejecuta:
		// el comportamiento anterior es mejor que bloquear el ERP por un problema
		// del libro de idempotencia, y el hueco queda visible en la bitácora.
		return claim
	}

	// La clave ya existía. Se consulta el estado para decidir qué se responde.
	var status string
	var prev sql.NullString
	if qerr := db.QueryRow(`SELECT status, COALESCE(result_json,'') FROM ai_tool_execution WHERE idem_key = ?`, key).
		Scan(&status, &prev); qerr != nil {
		// No se pudo saber el estado. Se ejecuta por la misma razón que arriba.
		return claim
	}

	claim.Skip = true
	switch status {
	case "undone":
		// La corrida anterior se revirtió y su documento ya no existe: la clave
		// vuelve a estar libre. Se reutiliza la MISMA fila para no dejar basura y
		// para conservar el rastro de que hubo una ejecución previa.
		_, _ = db.Exec(`UPDATE ai_tool_execution SET status='claimed', result_json=NULL,
			run_id=?, task_id=?, agent_id=?, updated_at=? WHERE idem_key=?`,
			j.runID, j.taskID, j.agentID, now(), key)
		claim.Skip = false
		return claim
	case "done":
		if prev.String != "" {
			claim.Result = aiDuplicateResult(prev.String)
			return claim
		}
	case "error":
		claim.Result = string(mustJSON(map[string]interface{}{
			"duplicate": true,
			"status":    "error previo",
			"note": "La ejecución anterior de esta acción falló y NO se repite sola: revísala en la corrida antes de reintentar. " +
				"Se prefiere no duplicar un documento a repetirlo.",
		}))
		return claim
	}

	claim.Result = string(mustJSON(map[string]interface{}{
		"duplicate": true,
		"status":    "en curso",
		"note": "Esta acción ya está registrada con los mismos datos y no se repite. " +
			"Si la corrida anterior quedó a medias, revísala en Tareas > última ejecución.",
	}))
	return claim
}

// aiEsClaveDuplicada dice si el error es «esa clave ya estaba».
//
// Se decide por el mensaje Y por el código de MySQL (1062), como hace el resto
// del sistema (ver infrastructure/persistence): los códigos no cambian entre
// versiones y los mensajes sí se traducen. Cubre también el mensaje de SQLite
// («UNIQUE constraint failed»), porque las pruebas corren en memoria con ese
// motor.
//
// El reclamo usa un INSERT normal y NO `INSERT IGNORE`, a propósito: así «ya
// estaba» se distingue de cualquier otro error en vez de confundirlos. Con
// INSERT IGNORE, una tabla ausente y una clave repetida devuelven el mismo
// silencio, y esa ambigüedad es la que hace que un candado parezca puesto
// cuando en realidad nadie está mirando.
func aiEsClaveDuplicada(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, trozo := range []string{
		"duplicate entry", "duplicate key", "errno 1062", "unique constraint failed",
	} {
		if strings.Contains(m, trozo) {
			return true
		}
	}
	return false
}

// aiFinishTool cierra el reclamo con el resultado real.
//
// Se guarda el resultado completo —no solo «ok»— porque es lo que se devuelve al
// modelo cuando la acción se repite: el agente necesita el número de factura o de
// voucher para el informe final, y sin él inventaría uno.
func (s *Server) aiFinishTool(db *sql.DB, claim aiToolClaim, result string, execErr error) {
	if db == nil || claim.Key == "" || claim.Skip {
		return
	}
	status := "done"
	if execErr != nil {
		status = "error"
		result = string(mustJSON(map[string]interface{}{"error": execErr.Error()}))
	}
	_, _ = db.Exec(`UPDATE ai_tool_execution SET status=?, result_json=?, updated_at=? WHERE idem_key=?`,
		status, result, now(), claim.Key)
}

// aiDuplicateResult devuelve el resultado original marcado como duplicado.
//
// Se marca porque el modelo tiene que entender que NO se creó nada nuevo: si se
// devolviera el JSON original tal cual, el agente creería que acaba de crear el
// documento y el informe final diría «registré el cobro» cuando en realidad lo
// registró la corrida anterior.
func aiDuplicateResult(prev string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(prev), &m); err != nil || m == nil {
		return string(mustJSON(map[string]interface{}{
			"duplicate":       true,
			"previous_result": prev,
			"note":            "Esta acción ya se ejecutó con los mismos datos; no se repite y no se creó nada nuevo.",
		}))
	}
	m["duplicate"] = true
	m["note"] = "Esta acción ya se ejecutó con los mismos datos; no se repite y no se creó nada nuevo."
	return string(mustJSON(m))
}
