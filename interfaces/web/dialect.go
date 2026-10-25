package web

import "database/sql"

// ---------- SQL (SQLite único) ----------
// ERPGo corre sobre SQLite en las dos ediciones (PyME y Enterprise): una
// base por empresa, con WAL. El SQL se escribe una sola vez, en dialecto
// SQLite; no hay ramificación por motor ni condiciones "if MySQL" esparcidas por
// los handlers. Por eso las consultas de este archivo son SQLite nativo y no
// ranuras entre dos dialectos.

const nowLayout = "2006-01-02 15:04:05"

// insertIgnorePrefix devuelve el prefijo INSERT idempotente de SQLite, para
// funciones que reciben un *sql.DB suelto en vez de un *Server.
func insertIgnorePrefix(dialect string) string {
	return "INSERT IGNORE INTO "
}

// sqlDateYMD formatea una columna date/datetime como YYYY-MM-DD.
func (s *Server) sqlDateYMD(col string) string {
	return "SUBSTR(" + col + ",1,10)"
}

// sqlCastText castea una expresión a texto.
func (s *Server) sqlCastText(expr string) string {
	return "CAST(" + expr + " AS CHAR)"
}

// sqlShowTables lista tablas del usuario.
//
// MySQL no tiene `sqlite_master`. information_schema con DATABASE() acota a la
// base conectada: sin ese filtro saldrían las tablas de TODAS las bases del
// servidor, y la IA vería el esquema de otras empresas.
func (s *Server) sqlShowTables() string {
	return "SELECT table_name FROM information_schema.tables " +
		"WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'"
}

// upsertFiscalSetting actualiza una fila clave/valor de fiscal_setting.
//
// Recibe la base como argumento en vez de usar s.db: quien llama ya sabe de qué
// empresa es la petición, y pasar el handle hace imposible que este helper
// escriba en la base de otra empresa (ver tenant_request.go).
func (s *Server) upsertFiscalSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO fiscal_setting (`+"`key`"+`, value) VALUES (?,?)
		ON DUPLICATE KEY UPDATE value = VALUES(value)`, key, value)
	return err
}

// upsertExchangeRate hace upsert de una tasa BCV por fecha y moneda.
func (s *Server) upsertExchangeRate(db *sql.DB, date, currency string, rate float64, source, createdAt string) error {
	_, err := db.Exec(`INSERT INTO exchange_rate (date, currency, rate_bcv, source, created_at) VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE rate_bcv = VALUES(rate_bcv), source = VALUES(source)`,
		date, currency, rate, source, createdAt)
	return err
}

// ensureFiscalSeries asegura que exista la serie fiscal (upsert no-op).
func (s *Server) ensureFiscalSeries(tx *sql.Tx, series string) error {
	_, err := tx.Exec(`INSERT IGNORE INTO fiscal_series (code, name, prefix, next_no, control_prefix, control_next, created_at)
		VALUES (?,?,?,?,?,?,?)`, series, series, "", 1, "00", 1, now())
	return err
}

// upsertPaymentSummary recalcula el resumen de pagos de una factura.
func (s *Server) upsertPaymentSummary(db *sql.DB, invoiceID int64) {
	_, _ = db.Exec(`
		INSERT INTO invoice_payment_summary (invoice_id, cash_session_id, method_id, amount)
		SELECT i.id, i.cash_session_id, pm.id, SUM(t.Debit)
		FROM invoice i
		JOIN acc_transaction t ON t.VNo LIKE 'CV-%' AND t.referenceNo = CAST(i.invoice AS CHAR)
		JOIN payment_method pm ON pm.coa_id = t.COAID
		WHERE i.id = ? AND t.Debit > 0
		GROUP BY i.id, i.cash_session_id, pm.id
		ON DUPLICATE KEY UPDATE amount = VALUES(amount)`, invoiceID)
}
