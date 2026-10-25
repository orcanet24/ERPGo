package web

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// financialYear replicates PHP financial_year(): active FY id covering today.
func financialYear(db *sql.DB) int {
	var id int
	err := db.QueryRow(`SELECT id FROM financial_year WHERE startDate <= ? AND endDate >= ? AND status = 1 LIMIT 1`,
		time.Now().Format("2006-01-02"), time.Now().Format("2006-01-02")).Scan(&id)
	if err != nil {
		return 1
	}
	return id
}

// generator replicates PHP generator($lenth): random alphanumeric token (35-char alphabet).
// Uses crypto/rand for unpredictability (secure for IDs/tokens).
func generator(lenth int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	var sb strings.Builder
	for i := 0; i < lenth; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		sb.WriteByte(chars[n.Int64()])
	}
	return sb.String()
}

// getMaxFieldNumber replicates Accounts_model->getMaxFieldNumber:
// returns the numeric suffix of $fild2 (VNo) from the latest row of $table
// where $where = $type, ordered by id desc.
func getMaxFieldNumber(db *sql.DB, table, where, typ, fild2 string) int64 {
	// Whitelist de tablas y columnas permitidas para evitar SQL injection
	allowedTables := map[string]bool{"acc_vaucher": true, "invoice": true, "product_purchase": true}
	allowedColumns := map[string]bool{"VNo": true, "invoice_id": true, "purchase_id": true, "Vtype": true, "type": true}
	if !allowedTables[table] || !allowedColumns[where] || !allowedColumns[fild2] {
		return 0
	}
	var vno sql.NullString
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ? ORDER BY id DESC LIMIT 1", fild2, table, where)
	_ = db.QueryRow(query, typ).Scan(&vno)
	if !vno.Valid || vno.String == "" {
		return 0
	}
	parts := strings.SplitN(vno.String, "-", 2)
	var n int64
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &n)
	} else {
		fmt.Sscanf(parts[0], "%d", &n)
	}
	return n
}

// nextVoucherNo returns the next voucher number for a Vtype ("JV-12", "DV-3"...).
func nextVoucherNo(db *sql.DB, vtype string) string {
	return fmt.Sprintf("%s-%d", vtype, getMaxFieldNumber(db, "acc_vaucher", "Vtype", vtype, "VNo")+1)
}

// randomHex returns n random bytes hex encoded.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// randInt returns a crypto random int in [0, max).
func randInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

// parseMoney parses a form value that may be empty into float64.
func parseMoney(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(strings.ReplaceAll(s, ",", ""), "%f", &f)
	return f
}

// saveRedirect devuelve la URL a la que ir después de guardar.
// "save" → vuelve al mismo formulario (Referer).
// "save_exit" → vuelve al listado.
func saveRedirect(r *http.Request, listURL string) string {
	if r.FormValue("action") == "save" {
		if ref := r.Header.Get("Referer"); ref != "" {
			return ref
		}
	}
	return listURL
}
func today() string { return time.Now().Format("2006-01-02") }

// now returns YYYY-MM-DD HH:MM:SS.
func now() string { return time.Now().Format("2006-01-02 15:04:05") }
