package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- IGTF (Impuesto Grandes Transacciones Financieras 3%) ----------
// Se aplica cuando se paga en divisas (USD, EUR, USDT, etc.)

func (s *Server) calculateIGTF(amountUSD, rateBCV float64) float64 {
	igtfRate := 0.03
	return amountUSD * igtfRate * rateBCV
}

// ---------- Exchange Rate (Tasa BCV) ----------

func (s *Server) registerExchangeRoutes() {
	s.mux.HandleFunc("GET /settings/exchange-rate", s.exchangeRate)
	s.mux.HandleFunc("POST /settings/exchange-rate/save", s.exchangeRateSave)
	s.mux.HandleFunc("GET /settings/exchange-rate/data", s.exchangeRateData)
	s.mux.HandleFunc("GET /reports/exchange-difference", s.exchangeDifference)
}

func (s *Server) exchangeRate(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.dbFor(r).Query(`SELECT date, currency, rate_bcv, source, created_at FROM exchange_rate ORDER BY date DESC LIMIT 50`)
	type row struct {
		Date     string  `json:"date"`
		Currency string  `json:"currency"`
		Rate     float64 `json:"rate"`
		Source   string  `json:"source"`
	}
	var list []row
	if rows != nil {
		for rows.Next() {
			var d, cur, src, created sql.NullString
			var rate float64
			_ = rows.Scan(&d, &cur, &rate, &src, &created)
			list = append(list, row{d.String, cur.String, rate, src.String})
		}
		rows.Close()
	}
	s.render(w, r, "exchange_rate.html", map[string]interface{}{
		"Title":    "Tasa BCV — Configuración",
		"Rows":     list,
		"Currency": s.settingsFor(r).Web.Currency,
	})
}

func (s *Server) exchangeRateSave(w http.ResponseWriter, r *http.Request) {
	date := r.FormValue("date")
	currency := orDefault(r.FormValue("currency"), "USD")
	rateStr := r.FormValue("rate")
	rate, _ := strconv.ParseFloat(rateStr, 64)
	if rate <= 0 {
		jsonError(w, "Tasa inválida", 400)
		return
	}
	err := s.upsertExchangeRate(s.dbFor(r), date, currency, rate, "manual", time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/settings/exchange-rate", 302)
}

func (s *Server) exchangeRateData(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.dbFor(r).Query(`SELECT date, currency, rate_bcv, source FROM exchange_rate ORDER BY date DESC LIMIT 100`)
	type row struct {
		Date     string  `json:"date"`
		Currency string  `json:"currency"`
		Rate     float64 `json:"rate"`
		Source   string  `json:"source"`
	}
	var list []row
	if rows != nil {
		for rows.Next() {
			var d, cur, src sql.NullString
			var rate float64
			_ = rows.Scan(&d, &cur, &rate, &src)
			list = append(list, row{d.String, cur.String, rate, src.String})
		}
		rows.Close()
	}
	writeJSON(w, map[string]interface{}{"data": list})
}

// exchangeDifference calcula el diferencial cambiario entre factura y pago.
func (s *Server) exchangeDifference(w http.ResponseWriter, r *http.Request) {
	from := orDefault(r.FormValue("from"), time.Now().Format("2006-01")+"-01")
	to := orDefault(r.FormValue("to"), time.Now().Format("2006-01")+"-31")

	rows, err := s.dbFor(r).Query(`
		SELECT i.invoice_id, i.date, i.customer_id,
			i.total_amount AS monto_factura,
			(SELECT er.rate_bcv FROM exchange_rate er WHERE er.date <= i.date ORDER BY er.date DESC LIMIT 1) AS tasa_factura,
			(SELECT er2.rate_bcv FROM exchange_rate er2 WHERE er2.date <= i.date ORDER BY er2.date DESC LIMIT 1) AS tasa_pago
		FROM invoice i
		WHERE i.date BETWEEN ? AND ? AND i.payment_type IN (2, 5)
		ORDER BY i.date ASC`, from, to)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type diffRow struct {
		InvoiceID    string  `json:"invoice_id"`
		Date         string  `json:"date"`
		MontoFactura float64 `json:"monto_factura"`
		TasaFactura  float64 `json:"tasa_factura"`
		TasaPago     float64 `json:"tasa_pago"`
		Diferencia   float64 `json:"diferencia"`
		Ganancia     float64 `json:"ganancia_perdida"`
	}
	var list []diffRow
	var totalDiferencia float64
	for rows.Next() {
		var invID, d sql.NullString
		var monto, tasaF, tasaP float64
		_ = rows.Scan(&invID, &d, &monto, &tasaF, &tasaP)
		if tasaF > 0 && tasaP > 0 {
			diff := (monto * tasaP) - (monto * tasaF)
			list = append(list, diffRow{
				InvoiceID: invID.String, Date: d.String, MontoFactura: monto,
				TasaFactura: tasaF, TasaPago: tasaP, Diferencia: diff, Ganancia: diff,
			})
			totalDiferencia += diff
		}
	}
	s.render(w, r, "exchange_difference.html", map[string]interface{}{
		"Title":           "Diferencial Cambiario",
		"From":            from,
		"To":              to,
		"Rows":            list,
		"TotalDiferencia": totalDiferencia,
		"Currency":        s.settingsFor(r).Web.Currency,
	})
}

// Helper: Get session from context or request
func (s *Server) getSessionFromRequest(r *http.Request) string {
	// Try cookie first
	c, err := r.Cookie("erpgow_session")
	if err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// hmacSign creates a signature for integrity verification
func hmacSign(data, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// verifyIntegrity checks if data has been tampered with
func verifyIntegrity(data, signature, key string) bool {
	expected := hmacSign(data, key)
	return hmac.Equal([]byte(signature), []byte(expected))
}

// Helper to safely get float from form
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// Helper to safely get int from form
func parseInt(s string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(s))
	return i
}

// hashPassword se ha ELIMINADO.
//
// Decía "crea un hash bcrypt" y devolvía la contraseña sin transformar. Nadie
// lo llamaba, y el login de verdad ya usaba bcrypt, así que no estaba
// rompiendo nada: estaba mintiendo.
//
// Se borra en vez de arreglarse porque arreglarlo habría dejado un sitio más
// donde escribir contraseñas, que es justo lo que no se quiere. Crear un hash
// se hace en password.go (bcryptGenerate) y comprobarlo en verificarPassword:
// un solo sitio para cada cosa, y los dos con nombre que no miente.
