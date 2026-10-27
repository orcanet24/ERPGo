package web

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// logout.go — cerrar sesión cuando hay un turno de caja abierto.
//
// QUÉ PASA Y POR QUÉ. `handleLogout` borraba la sesión web y nada más. El turno
// de caja (`cash_session.status = 1`) y el turno del POS
// (`pos_turns.status = 'open'`) no se tocan: el tiempo del turno se calcula
// desde `opened_at` y no hay `closed_at`, así que sigue corriendo después de
// cerrar la sesión, y la caja sigue apareciendo abierta en el panel del POS. Eso
// es lo que se ve como «cierro la sesión y el tiempo sigue corriendo»: la sesión
// se cierra, el turno no.
//
// POR QUÉ NO SE CIERRA EL TURNO AUTOMÁTICAMENTE. Cerrar un turno es un ARQUEO:
// se declara el efectivo contado, se compara con el que lleva el sistema y se
// firma un hash de integridad (ver pos_api.go). Cerrarlo al salir, con el
// importe esperado, escribiría un arqueo que nadie hizo —dejaría de ser una
// prueba—. Automatizarlo aquí sería cambiar un descuadre visible por un acta
// falsa, que es mucho peor.
//
// LO QUE SÍ SE HACE: no dejar que la sesión se cierre como si no pasara nada.
// Si hay un turno abierto, se dice cuál, desde cuándo y cuánto lleva, y se deja
// elegir: volver a cerrarlo con su arqueo, o salir de todos modos. En ese último
// caso la salida queda anotada en la cadena de auditoría, para que el turno
// huérfano tenga una explicación y un responsable.

// turnoAbierto es un turno que seguiría corriendo si se cierra la sesión.
type turnoAbierto struct {
	Que      string `json:"que"`
	Desde    string `json:"desde"`
	Duracion string `json:"duracion"`
	Terminal string `json:"terminal,omitempty"`
}

// consultasTurnosAbiertos son las dos fuentes de un turno abierto.
//
// Se declaran con el mismo formato que las guardas de borrado
// (consultaReferencia) a propósito: la prueba que comprueba que una consulta
// apunta a tablas y columnas reales las revisa igual que a aquellas, así que un
// nombre mal escrito no puede quedarse en el código creyendo que consulta algo.
//
// `cash_session` es el espejo que ve el resto del ERP y `pos_turns` es la tabla
// del turno del terminal; las dos pueden tener un turno abierto del mismo
// usuario, y las dos cuentan tiempo.
var consultasTurnosAbiertos = []consultaReferencia{
	{"turno de caja",
		`SELECT warehouse_id, opened_at FROM cash_session WHERE cashier = ? AND status = 1 ORDER BY opened_at DESC LIMIT 1`},
	{"turno del POS",
		`SELECT terminal_id, opened_at FROM pos_turns WHERE user_id = ? AND status = 'open' ORDER BY opened_at DESC LIMIT 1`},
}

// turnosAbiertosDe devuelve los turnos que quedarían corriendo.
//
// NUNCA DEVUELVE ERROR, y es deliberado: si la consulta falla —una base que no
// tiene la tabla, un permiso—, el cierre de sesión sigue adelante. Dejar a
// alguien sin poder salir por una comprobación de cortesía es mucho peor que no
// avisarle.
func (s *Server) turnosAbiertosDe(sess *SessionData) []turnoAbierto {
	if sess == nil || strings.TrimSpace(sess.UserID) == "" {
		return nil
	}
	db := s.dbForSession(sess)
	if db == nil {
		return nil
	}
	ahora := time.Now()
	var turnos []turnoAbierto
	for _, c := range consultasTurnosAbiertos {
		var terminal sql.NullString
		var abierto time.Time
		if err := db.QueryRow(c.SQL, sess.UserID).Scan(&terminal, &abierto); err != nil {
			continue
		}
		turnos = append(turnos, turnoAbierto{
			Que:      c.Que,
			Desde:    abierto.Format("15:04"),
			Duracion: duracionLegible(ahora.Sub(abierto)),
			Terminal: terminal.String,
		})
	}
	return turnos
}

// duracionLegible dice cuánto lleva algo abierto, en palabras.
//
// En palabras y no en un número de minutos porque esto se lee una vez, de pie y
// con prisa: «4 h 20 min» se entiende, «260 min» hay que calcularlo.
func duracionLegible(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "menos de un minuto"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 24*time.Hour:
		horas, minutos := int(d.Hours()), int(d.Minutes())%60
		if minutos == 0 {
			return fmt.Sprintf("%d h", horas)
		}
		return fmt.Sprintf("%d h %d min", horas, minutos)
	default:
		dias, horas := int(d.Hours())/24, int(d.Hours())%24
		if horas == 0 {
			return fmt.Sprintf("%d días", dias)
		}
		return fmt.Sprintf("%d días %d h", dias, horas)
	}
}

// textoDeTurnosAbiertos es el aviso que se lee antes de salir.
func textoDeTurnosAbiertos(turnos []turnoAbierto) string {
	if len(turnos) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range turnos {
		fmt.Fprintf(&b, "Su %s sigue abierto desde las %s (%s)", t.Que, t.Desde, t.Duracion)
		if t.Terminal != "" {
			fmt.Fprintf(&b, " en %s", t.Terminal)
		}
		b.WriteString(". ")
	}
	b.WriteString("El turno no se cierra al cerrar la sesión: su tiempo seguirá corriendo y la caja no " +
		"cuadrará hasta que alguien lo cierre con su arqueo desde la terminal de caja.")
	return b.String()
}
