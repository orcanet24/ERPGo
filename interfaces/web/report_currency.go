package web

// report_currency.go — Reporte de posición cambiaria.
// Muestra la exposición a devaluación: cuánto se tiene en VES vs USD,
// cuánto se debe en cada moneda, y la posición neta.

import (
	"net/http"
	"time"
)

type CurrencyPosition struct {
	Fecha       string  `json:"fecha"`
	TasaBCV     float64 `json:"tasas_bcv"`
	Total       float64 `json:"total_ves"`
	IngresosVES float64 `json:"ingresos_ves"`
	CobrosVES   float64 `json:"cobros_ves"`
	PagosVES    float64 `json:"pagos_ves"`
	IngresosUSD float64 `json:"ingresos_usd"`
	CobrosUSD   float64 `json:"cobros_usd"`
	PosicionVES float64 `json:"posicion_neta_ves"`
	PosicionUSD float64 `json:"posicion_neta_usd"`
	Exposicion  float64 `json:"exposicion_devaluacion"`
}

func (s *Server) reportCurrencyPosition(w http.ResponseWriter, r *http.Request) {
	fecha := r.FormValue("fecha")
	if fecha == "" {
		fecha = time.Now().Format("2006-01-02")
	}

	tasa, _, err := s.ObtenerTasaHoy(s.dbFor(r), fecha, "USD")
	if err != nil || tasa <= 0 {
		tasa = 1.0
	}

	pos := CurrencyPosition{
		Fecha:   fecha,
		TasaBCV: tasa,
	}

	_ = s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(total_amount),0), COALESCE(SUM(total_amount_usd),0)
		FROM invoice WHERE date = ? AND status = 1`, fecha).Scan(&pos.IngresosVES, &pos.IngresosUSD)

	_ = s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(amount),0), COALESCE(SUM(amount_usd),0)
		FROM debt_collection WHERE collection_date = ?`, fecha).Scan(&pos.CobrosVES, &pos.CobrosUSD)

	_ = s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(paid_amount),0)
		FROM product_purchase WHERE purchase_date = ? AND status = 1`, fecha).Scan(&pos.PagosVES)

	pos.Total = pos.IngresosVES
	pos.PosicionVES = pos.IngresosVES - pos.PagosVES
	pos.PosicionUSD = pos.IngresosUSD - (pos.PagosVES / tasa)
	pos.Exposicion = pos.PagosVES - pos.IngresosVES

	writeJSON(w, pos)
}

func (s *Server) reportCurrencyExposure(w http.ResponseWriter, r *http.Request) {
	var cobrarVES, cobrarUSD, pagarVES float64

	_ = s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(due_amount),0), COALESCE(SUM(total_amount_usd),0)
		FROM invoice WHERE due_amount > 0 AND status = 1`).Scan(&cobrarVES, &cobrarUSD)

	_ = s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(due_amount),0)
		FROM product_purchase WHERE due_amount > 0 AND status = 1`).Scan(&pagarVES)

	tasa, _, _ := s.ObtenerTasaHoy(s.dbFor(r), time.Now().Format("2006-01-02"), "USD")

	writeJSON(w, map[string]interface{}{
		"cuentas_por_cobrar_ves": cobrarVES,
		"cuentas_por_cobrar_usd": cobrarUSD,
		"cuentas_por_pagar_ves":  pagarVES,
		"cuentas_por_pagar_usd":  pagarVES / tasa,
		"posicion_neta_ves":      cobrarVES - pagarVES,
		"posicion_neta_usd":      cobrarUSD - (pagarVES / tasa),
		"tasa_bcv":               tasa,
	})
}
