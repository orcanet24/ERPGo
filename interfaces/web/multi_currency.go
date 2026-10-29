package web

// multi_currency.go — Motor de conversión monetaria dual VES/USD.
//
// DELICADEZA: cada conversión se registra en auditoría.
// Un error aquí = multa SENIAT o pérdida de dinero.

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// ObtenerTasaHoy devuelve la tasa BCV vigente para la fecha dada.
// Acepta *sql.Tx para usar dentro de transacciones de factura.
func (s *Server) ObtenerTasaHoy(tx interface {
	QueryRow(string, ...interface{}) *sql.Row
}, fecha string, moneda string) (float64, string, error) {
	if moneda == "" {
		moneda = "USD"
	}
	if fecha == "" {
		fecha = time.Now().Format("2006-01-02")
	}

	var tasa float64
	var fuente string

	err := tx.QueryRow(`SELECT rate_bcv, source FROM exchange_rate WHERE date = ? AND currency = ? LIMIT 1`, fecha, moneda).Scan(&tasa, &fuente)
	if err == nil && tasa > 0 {
		return tasa, fuente, nil
	}

	err = tx.QueryRow(`SELECT official_rate, source FROM exchange_rate_daily WHERE rate_date = ? AND currency = ? AND is_parallel = 0 ORDER BY rate_date DESC LIMIT 1`, fecha, moneda).Scan(&tasa, &fuente)
	if err == nil && tasa > 0 {
		return tasa, fuente, nil
	}

	err = tx.QueryRow(`SELECT rate_bcv, source FROM exchange_rate WHERE date <= ? AND currency = ? AND rate_bcv > 0 ORDER BY date DESC LIMIT 1`, fecha, moneda).Scan(&tasa, &fuente)
	if err == nil && tasa > 0 {
		return tasa, fuente, nil
	}

	err = tx.QueryRow(`SELECT official_rate, source FROM exchange_rate_daily WHERE rate_date <= ? AND currency = ? AND is_parallel = 0 AND official_rate > 0 ORDER BY rate_date DESC LIMIT 1`, fecha, moneda).Scan(&tasa, &fuente)
	if err == nil && tasa > 0 {
		return tasa, fuente, nil
	}

	return 0, "", fmt.Errorf("no hay tasa de cambio para %s en %s", moneda, fecha)
}

// ConvertirVESaUSD convierte un monto VES a USD usando la tasa del día
func (s *Server) ConvertirVESaUSD(tx interface {
	QueryRow(string, ...interface{}) *sql.Row
}, montoVES float64, fecha string) (float64, float64, error) {
	tasa, _, err := s.ObtenerTasaHoy(tx, fecha, "USD")
	if err != nil || tasa <= 0 {
		return 0, 0, fmt.Errorf("no se puede convertir: tasa no disponible")
	}
	return montoVES / tasa, tasa, nil
}

// ConvertirUSDaVES convierte un monto USD a VES usando la tasa del día
func (s *Server) ConvertirUSDaVES(tx interface {
	QueryRow(string, ...interface{}) *sql.Row
}, montoUSD float64, fecha string) (float64, float64, error) {
	tasa, _, err := s.ObtenerTasaHoy(tx, fecha, "USD")
	if err != nil || tasa <= 0 {
		return 0, 0, fmt.Errorf("no se puede convertir: tasa no disponible")
	}
	return montoUSD * tasa, tasa, nil
}

// RegistrarAuditoriaTasa deja constancia de cada conversión realizada
func (s *Server) RegistrarAuditoriaTasa(db interface {
	Exec(string, ...interface{}) (sql.Result, error)
}, fecha, moneda string, tasa float64, fuente, usuario string, invoiceID int) {
	if _, err := db.Exec(`INSERT INTO exchange_rate_audit (rate_date, currency, rate, source, applied_by, applied_at, invoice_id) VALUES (?, ?, ?, ?, ?, NOW(), ?)`, fecha, moneda, tasa, fuente, usuario, invoiceID); err != nil {
		log.Printf("multi-moneda: no se pudo registrar la auditoría de tasa para factura %d: %v", invoiceID, err)
	}
}
