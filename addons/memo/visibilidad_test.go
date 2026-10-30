package main

// visibilidad_test.go — La regla de visibilidad del muro, corrida de verdad.
//
// POR QUÉ ESTA PRUEBA. El candado de quién ve qué es UNA sentencia SQL
// (consultaMuro) que en producción corre contra MySQL dentro del addon. Aquí
// se ejecuta la MISMA constante sobre SQLite con filas reales, porque una
// regla de permisos que no se prueba es una regla que alguien va a romper sin
// darse cuenta: el WHERE es lo único que impide que la nota privada de una
// persona aparezca en el muro de otra.

import (
	"path/filepath"
	"testing"

	"erpgo/infrastructure/persistence"
)

func TestLaVisibilidadDeUnaNota(t *testing.T) {
	db, err := persistence.OpenSQLite(filepath.Join(t.TempDir(), "memo.sqlite3"))
	if err != nil {
		t.Fatalf("no se pudo abrir la base de prueba: %v", err)
	}
	defer db.Close()

	esquema := []string{
		`CREATE TABLE memo_notas (
			id VARCHAR(64) PRIMARY KEY, empresa VARCHAR(64), autor VARCHAR(64),
			titulo VARCHAR(200), texto TEXT, visibilidad VARCHAR(20),
			tipo VARCHAR(20), estado VARCHAR(20), fijada INTEGER,
			creada VARCHAR(32), actualizada VARCHAR(32))`,
		`CREATE TABLE memo_destinatarios (
			id VARCHAR(64) PRIMARY KEY, nota_id VARCHAR(64),
			destino_tipo VARCHAR(20), destino_id VARCHAR(64), empresa VARCHAR(64))`,
	}
	for _, q := range esquema {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("esquema: %v", err)
		}
	}

	notas := []struct{ id, empresa, autor, vis, estado string }{
		{"n1", "default", "ana", "privada", "activo"},    // de Ana: Ana la ve
		{"n2", "default", "ana", "empresa", "activo"},    // de empresa: los dos la ven
		{"n3", "default", "luis", "privada", "activo"},   // privada de Luis: Ana no
		{"n4", "default", "luis", "privada", "activo"},   // privada de Luis compartida con Ana
		{"n5", "otra", "ana", "empresa", "activo"},       // de otra empresa: fuera
		{"n6", "default", "ana", "empresa", "archivado"}, // archivada: fuera del muro
	}
	for _, n := range notas {
		if _, err := db.Exec(`INSERT INTO memo_notas (id, empresa, autor, titulo, texto, visibilidad, tipo, estado, creada)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			n.id, n.empresa, n.autor, "Título de "+n.id, "Texto de "+n.id, n.vis, "nota", n.estado, "2026-10-30 09:00:00"); err != nil {
			t.Fatalf("sembrar la nota %s: %v", n.id, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO memo_destinatarios (id, nota_id, destino_tipo, destino_id, empresa)
		VALUES ('d1','n4','persona','ana','default')`); err != nil {
		t.Fatalf("sembrar el destinatario: %v", err)
	}

	ver := func(usuario string) map[string]bool {
		rows, err := db.Query(consultaMuro, "default", usuario, usuario)
		if err != nil {
			t.Fatalf("la consulta del muro falló: %v", err)
		}
		defer rows.Close()
		vistas := map[string]bool{}
		for rows.Next() {
			var id, titulo, texto, autor, vis, creada string
			if err := rows.Scan(&id, &titulo, &texto, &autor, &vis, &creada); err != nil {
				t.Fatalf("no se pudo leer la fila: %v", err)
			}
			vistas[id] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("la consulta terminó mal: %v", err)
		}
		return vistas
	}

	// Ana: lo suyo, la de empresa y la que Luis le compartió… y nada más.
	ana := ver("ana")
	for _, esperada := range []string{"n1", "n2", "n4"} {
		if !ana[esperada] {
			t.Errorf("a Ana debía versele la nota %s: %v", esperada, ana)
		}
	}
	for _, vedada := range []string{"n3", "n5", "n6"} {
		if ana[vedada] {
			t.Errorf("a Ana NO debe versele la nota %s (privada ajena, de otra empresa o archivada)", vedada)
		}
	}

	// Luis: lo suyo y la de empresa; la de Ana es privada y no está compartida.
	luis := ver("luis")
	for _, esperada := range []string{"n2", "n3", "n4"} {
		if !luis[esperada] {
			t.Errorf("a Luis debía versele la nota %s: %v", esperada, luis)
		}
	}
	for _, vedada := range []string{"n1", "n5", "n6"} {
		if luis[vedada] {
			t.Errorf("a Luis NO debe versele la nota %s", vedada)
		}
	}
}
