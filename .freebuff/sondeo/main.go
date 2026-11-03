package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("../../bin/.env")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4", os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME"))
	db, _ := sql.Open("mysql", dsn)
	defer db.Close()

	rows, _ := db.Query("SELECT id, product_id, lot_code, quantity_received, quantity_available FROM erpgow_001.product_lot ORDER BY id")
	for rows.Next() {
		var id, pid int64
		var code string
		var rec, disp float64
		rows.Scan(&id, &pid, &code, &rec, &disp)
		fmt.Printf("lote id=%d producto=%s code=%s recibidos=%.2f disponibles=%.2f\n", id, pid, code, rec, disp)
	}
	rows.Close()

	var inv, invd int
	db.QueryRow("SELECT COUNT(*) FROM erpgow_001.invoice").Scan(&inv)
	db.QueryRow("SELECT COUNT(*) FROM erpgow_001.invoice_details").Scan(&invd)
	fmt.Println("facturas:", inv, "| renglones facturados:", invd)

	rows2, _ := db.Query("SELECT product_id, COUNT(*), SUM(quantity) FROM erpgow_001.invoice_details GROUP BY product_id ORDER BY product_id")
	for rows2.Next() {
		var p string
		var n int
		var q float64
		rows2.Scan(&p, &n, &q)
		fmt.Printf("vendido producto=%s renglones=%d cantidad=%.2f\n", p, n, q)
	}
	rows2.Close()
}
