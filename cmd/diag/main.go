package main

import (
    "database/sql"
    "fmt"
    _ "github.com/go-sql-driver/mysql"
)

func main() {
    // Probar conexión directa sin base
    dsn := fmt.Sprintf("orcanet24_erpgo:e1obUwX@tcp(127.0.0.1:3306)/?charset=utf8mb4&timeout=5s")
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        fmt.Println("Open error:", err)
        return
    }
    defer db.Close()
    err = db.Ping()
    if err != nil {
        fmt.Println("Ping error:", err)
        return }
    fmt.Println("Conexión OK!")
    rows, _ := db.Query("SELECT CURRENT_USER()")
    for rows.Next() {
        var u string
        rows.Scan(&u)
        fmt.Println("User:", u)
    }
}
