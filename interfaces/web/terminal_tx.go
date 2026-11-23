package web

// terminal_tx.go — Piezas de transacción del registro de venta terminal.
// Viven separadas de venta_terminal.go porque son reglas compartidas: el
// mismo SQL de cliente/producto/ajustes que usaba internal/pos corría FUERA
// de toda transacción; aquí cada pieza recibe el *sql.Tx del registro para
// que el todo sea atómico o no sea (F-2).

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"erpgo/internal/pos"
)

// abrirTxReal abre con Read Committed cuando el motor lo entiende (MySQL en
// campo): D-6 (auditoría) —en REPEATABLE READ la lectura de stock puede
// ver un mundo anterior al bloqueo del contador y dos cajas venden la misma
// unidad—. SQLite (pruebas) no conoce ese nivel de aislamiento: se degrada
// al default del driver, que en SQLite es serializable por conexión y el
// problema no se da.
func abrirTxReal(db *sql.DB) (*sql.Tx, error) {
	if tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted}); err == nil {
		return tx, nil
	}
	return db.BeginTx(context.Background(), nil)
}

// findOrCreateCustomerTx: cliente por RIF, o el caminante mínimo de la caja.
// El RIF vacío se registra como V000000000 (consumidor final) porque la
// cabecera de factura lo exige no nulo: es el mismo criterio del sync
// antiguo, conservado a propósito para no romper clientes existentes.
func findOrCreateCustomerTx(tx *sql.Tx, rif, name string) (int64, error) {
	if strings.TrimSpace(rif) == "" {
		rif = "V000000000"
	}
	if strings.TrimSpace(name) == "" {
		name = "Cliente POS"
	}
	var existing int64
	if err := tx.QueryRow(`SELECT customer_id FROM customer_information WHERE rif = ? LIMIT 1`, rif).Scan(&existing); err == nil {
		return existing, nil
	}
	res, err := tx.Exec(`
		INSERT INTO customer_information (customer_name, rif, address2, status, customer_type)
		VALUES (?, ?, '', 1, 1)`, name, rif)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ensureProductTx: producto por código, o placeholder con el precio del
// ticket. Mismo criterio que antes, pero DENTRO de la transacción y con el
// error devuelto: el sync antiguo hacía `continue` sobre el error de alta y
// luego insertaba el renglón contra un producto inexistente.
func ensureProductTx(tx *sql.Tx, code, name string, price, taxRate float64) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "POS-AUTO-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	var existing string
	if err := tx.QueryRow(`SELECT product_id FROM product_information WHERE product_id = ? LIMIT 1`, code).Scan(&existing); err == nil {
		return existing, nil
	}
	if _, err := tx.Exec(`
		INSERT INTO product_information (product_id, product_name, price, tax, status, cost)
		VALUES (?, ?, ?, ?, 1, 0)`,
		code, firstNonEmpty(name, "Venta de caja"), fmt.Sprintf("%.2f", price), taxRate); err != nil {
		return "", err
	}
	return code, nil
}

// consulteroTx es lo que necesitan los lectores de configuración dentro de
// la transacción (la misma forma de consultadorUnaFila pero con Query).
type consulteroTx interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// cargarAjustesFiscalesTx lee fiscal_setting de LA EMPRESA (tx en mano).
// loadFiscalSettings(r) no sirve aquí: el registro del terminal corre sin
// el ámbito de una petición del panel.
func cargarAjustesFiscalesTx(q consulteroTx) fiscalSettings {
	fs := fiscalSettings{IGTFEnabled: true, IGTFRate: 3.0}
	rows, err := q.Query("SELECT `key`, value FROM fiscal_setting")
	if err != nil {
		return fs
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		switch k {
		case "igtf_enabled":
			fs.IGTFEnabled = v == "1"
		case "igtf_rate":
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				fs.IGTFRate = f
			}
		case "walkin_rif_required":
			fs.WalkinRIFReq = v == "1"
		case "fiscal_footnote":
			fs.FiscalFootnote = v
		case "cashhea_enabled":
			fs.CashheaEnabled = v == "1"
		}
	}
	return fs
}

// subcodeIDTx: el subcódigo contable del cliente dentro de la tx.
func subcodeIDTx(q consulteroTx, customerID int64, subTypeId int) sql.NullInt64 {
	var id int
	err := q.QueryRow(`SELECT id FROM acc_subcode WHERE referenceNo = ? AND subTypeId = ? LIMIT 1`,
		customerID, subTypeId).Scan(&id)
	if err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(id), Valid: true}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// esMetodoDivisa dice si un método de pago del terminal es de DIVISA o
// equivalente (zelle, paypal, cripto…): sobre esos recae el IGTF y con esos
// la tasa BCV importa. Es la misma lista que usa el panel; si el terminal
// manda un nombre nuevo, NO se asume divisa (asumir cobraría IGTF de más).
func esMetodoDivisa(m string) bool {
	switch strings.TrimSpace(m) {
	case "zelle", "paypal", "crypto", "cripto", "binance", "usdt", "usd", "efectivo_usd", "dolar", "dólar":
		return true
	}
	return false
}

func hayPagosDivisa(pagos []pos.PaymentMethod) bool {
	for _, p := range pagos {
		if esMetodoDivisa(strings.ToLower(strings.TrimSpace(p.Method))) {
			return true
		}
	}
	return false
}
