package web

// period_handlers.go — Pantallas del cierre de período.
//
// El handler de cierre NO intenta ser listo: delega todo el criterio en
// CheckPeriod y, si algo bloquea, devuelve la lista completa. Un cierre que
// "a veces" deja pasar es peor que uno que no deja pasar nunca, porque el
// contador acaba de dejar de mirar.

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerPeriodRoutes() {
	s.mux.HandleFunc("GET /accounting/periods", s.periodList)
	s.mux.HandleFunc("GET /accounting/periods/data", s.periodListData)
	s.mux.HandleFunc("GET /accounting/periods/check", s.periodCheck)
	s.mux.HandleFunc("POST /accounting/periods/close", s.periodClose)
	s.mux.HandleFunc("POST /accounting/periods/reopen", s.periodReopen)
}

// currentPeriod devuelve el mes en curso en el formato AAAAMM.
func currentPeriod() string {
	n := time.Now()
	return strconv.Itoa(n.Year()) + twoDigits(int(n.Month()))
}

// twoDigits rellena a dos dígitos el mes, porque "20261" no es un período legible.
func twoDigits(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func (s *Server) periodList(w http.ResponseWriter, r *http.Request) {
	periods, _ := s.PeriodsList(r, 24)
	s.render(w, r, "accounting_periods.html", map[string]interface{}{
		"Title":         "Cierre de períodos",
		"Periods":       periods,
		"CurrentPeriod": currentPeriod(),
	})
}

func (s *Server) periodListData(w http.ResponseWriter, r *http.Request) {
	periods, err := s.PeriodsList(r, 36)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	list := []map[string]interface{}{}
	for _, p := range periods {
		estado := "Abierto"
		badge := "label-warning"
		switch p.Status {
		case "closed":
			estado, badge = "Cerrado", "label-success"
		case "reopened":
			estado, badge = "Reabierto", "label-danger"
		}
		list = append(list, map[string]interface{}{
			"period": p.Period, "status": p.Status, "estado": estado, "badge": badge,
			"closed_at": p.ClosedAt, "closed_by": p.ClosedBy,
			"reopened_at": p.ReopenedAt, "reopened_by": p.ReopenedBy,
			"vouchers":    p.Vouchers,
			"total_debit": money(p.TotalDebit), "total_credit": money(p.TotalCredit),
		})
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

// periodCheck muestra el diagnóstico completo de un período sin cerrarlo.
//
// Existe como endpoint aparte porque es la acción que el contador hace todo el
// tiempo: revisar cómo va el mes antes de decidir si lo cierra.
func (s *Server) periodCheck(w http.ResponseWriter, r *http.Request) {
	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		period = currentPeriod()
	}
	res, err := s.CheckPeriod(r, period)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	vals := []map[string]interface{}{}
	for _, v := range res.Validations {
		vals = append(vals, map[string]interface{}{
			"code": v.Code, "label": v.Label, "problem": v.Problem, "blocking": v.Blocking,
		})
	}
	writeJSON(w, map[string]interface{}{
		"period": res.Period, "status": res.Status, "can_close": res.CanClose,
		"blocking": res.Blocking, "warnings": res.Warnings,
		"validations": vals, "summary": res.Summary,
	})
}

func (s *Server) periodClose(w http.ResponseWriter, r *http.Request) {
	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		jsonError(w, "Debe indicar el período a cerrar", 400)
		return
	}

	// Se revalida aquí aunque el cliente ya haya consultado: el botón se pudo
	// pulsar hace un minuto y en ese intervalo entró un asiento descuadrado.
	res, err := s.CheckPeriod(r, period)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if !res.CanClose {
		problemas := []string{}
		for _, v := range res.Validations {
			if v.Blocking && v.Problem != "" {
				problemas = append(problemas, v.Label+": "+v.Problem)
			}
		}
		jsonError(w, "El período no se puede cerrar. "+strings.Join(problemas, " | "), 409)
		return
	}

	if err := s.ClosePeriod(r, period, sessUserID(s.sessionFromRequest(r))); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Período " + period + " cerrado"})
}

func (s *Server) periodReopen(w http.ResponseWriter, r *http.Request) {
	period := strings.TrimSpace(r.FormValue("period"))
	motivo := strings.TrimSpace(r.FormValue("reason"))
	if err := s.ReopenPeriod(r, period, sessUserID(s.sessionFromRequest(r)), motivo); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Período " + period + " reabierto"})
}
