package web

// guia_dialect.go — DDL dialect-aware para las tablas de GOti.
//
// Por qué existe este archivo separado: ensureGuiaTables necesita saber si la
// base es MySQL (producción) o SQLite (tests) para elegir el DDL correcto.
// El resto de archivos no necesita saberlo: las consultas son portables, y el
// DDL de las otras tablas (legalSchema, controlSchema) usa siempre MySQL con
// AplicarEsquema, que en tests no se ejecuta nunca. GOti es único en que corre
// tanto en MySQL como en SQLite y necesita el DDL correcto en cada caso.

import (
	"database/sql"
)

// esMySQL detecta si la base es MySQL (producción) o SQLite (tests).
// MySQL admite SELECT VERSION(); SQLite no la tiene y devuelve error.
// La distinción importa para el DDL: AUTO_INCREMENT (MySQL) vs AUTOINCREMENT
// (SQLite) no son intercambiables.
func esMySQL(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var v string
	return db.QueryRow("SELECT VERSION()").Scan(&v) == nil
}

// guiaDDLSentencias devuelve el DDL de las tablas GOti en el dialecto correcto.
//
// Por qué no usar el conversor de migraciones (sqlite_convert.go): ese
// conversor opera sobre archivos migrations/*.sql en el arranque, no sobre DDL
// ejecutado directamente desde Go. AplicarEsquema (que sí usa este DDL) solo
// le quita la sintaxis MariaDB (IF NOT EXISTS en índices) y tolera "ya existe".
func guiaDDLSentencias(db *sql.DB) []string {
	if esMySQL(db) {
		return []string{
			`CREATE TABLE IF NOT EXISTS guia_tour (
				id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
				code VARCHAR(64) NOT NULL UNIQUE,
				label VARCHAR(160) NOT NULL,
				rol VARCHAR(30) NOT NULL DEFAULT '*',
				status TINYINT NOT NULL DEFAULT 1
			)`,
			`CREATE TABLE IF NOT EXISTS guia_paso (
				id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
				tour_id INT NOT NULL,
				orden INT NOT NULL,
				url VARCHAR(255) NOT NULL,
				selector VARCHAR(255) NOT NULL DEFAULT '',
				titulo VARCHAR(200) NOT NULL,
				mensaje TEXT NOT NULL,
				depends_on INT NULL,
				check_sql TEXT NULL,
				check_msg VARCHAR(255) NULL
			)`,
			`CREATE TABLE IF NOT EXISTS guia_progreso (
				user_id VARCHAR(64) NOT NULL,
				paso_id INT NOT NULL,
				done TINYINT NOT NULL DEFAULT 0,
				skipped TINYINT NOT NULL DEFAULT 0,
				hidden TINYINT NOT NULL DEFAULT 0,
				updated_at VARCHAR(32) NOT NULL DEFAULT '',
				PRIMARY KEY (user_id, paso_id)
			)`,
			// CREATE UNIQUE INDEX sin IF NOT EXISTS: AplicarEsquema +
			// EsErrorYaExiste toleran el "duplicate key name" de MySQL
			// al segundo arranque.
			`CREATE UNIQUE INDEX uk_tour_orden ON guia_paso (tour_id, orden)`,
		}
	}
	// SQLite: TINYINT no existe de verdad, pero SQLite lo acepta como alias.
	// INTEGER PRIMARY KEY AUTOINCREMENT es el auto-increment de SQLite.
	return []string{
		`CREATE TABLE IF NOT EXISTS guia_tour (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code VARCHAR(64) NOT NULL UNIQUE,
			label VARCHAR(160) NOT NULL,
			rol VARCHAR(30) NOT NULL DEFAULT '*',
			status TINYINT NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS guia_paso (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tour_id INT NOT NULL,
			orden INT NOT NULL,
			url VARCHAR(255) NOT NULL,
			selector VARCHAR(255) NOT NULL DEFAULT '',
			titulo VARCHAR(200) NOT NULL,
			mensaje TEXT NOT NULL,
			depends_on INT NULL,
			check_sql TEXT NULL,
			check_msg VARCHAR(255) NULL
		)`,
		`CREATE TABLE IF NOT EXISTS guia_progreso (
			user_id VARCHAR(64) NOT NULL,
			paso_id INT NOT NULL,
			done TINYINT NOT NULL DEFAULT 0,
			skipped TINYINT NOT NULL DEFAULT 0,
			hidden TINYINT NOT NULL DEFAULT 0,
			updated_at VARCHAR(32) NOT NULL DEFAULT '',
			PRIMARY KEY (user_id, paso_id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_tour_orden ON guia_paso (tour_id, orden)`,
	}
}

// guiaDDLForTest devuelve el DDL SQLite-compatible con el que baseGuia
// pre-crea las tablas antes de que guiaActive invoque ensureGuiaTables.
// No usa esMySQL porque en tests SIEMPRE es SQLite; sirve solo para que
// AplicarEsquema vea las tablas ya existentes y no intente parsear DDL MySQL.
func guiaDDLForTest() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS guia_tour (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code VARCHAR(64) NOT NULL UNIQUE,
			label VARCHAR(160) NOT NULL,
			rol VARCHAR(30) NOT NULL DEFAULT '*',
			status TINYINT NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS guia_paso (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tour_id INT NOT NULL,
			orden INT NOT NULL,
			url VARCHAR(255) NOT NULL,
			selector VARCHAR(255) NOT NULL DEFAULT '',
			titulo VARCHAR(200) NOT NULL,
			mensaje TEXT NOT NULL,
			depends_on INT NULL,
			check_sql TEXT NULL,
			check_msg VARCHAR(255) NULL
		)`,
		`CREATE TABLE IF NOT EXISTS guia_progreso (
			user_id VARCHAR(64) NOT NULL,
			paso_id INT NOT NULL,
			done TINYINT NOT NULL DEFAULT 0,
			skipped TINYINT NOT NULL DEFAULT 0,
			hidden TINYINT NOT NULL DEFAULT 0,
			updated_at VARCHAR(32) NOT NULL DEFAULT '',
			PRIMARY KEY (user_id, paso_id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_tour_orden ON guia_paso (tour_id, orden)`,
	}
}
