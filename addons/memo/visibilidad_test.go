package main

// visibilidad_test.go — La regla de visibilidad del muro, corrida de verdad.
//
// POR QUÉ ESTA PRUEBA. El candado de quién ve qué es UNA sentencia SQL
// (consultaMuro) que en producción corre contra MySQL dentro del addon. Aquí
// se ejecuta la MISMA constante sobre SQLite con filas reales, porque una
// regla de permisos que no se prueba es una regla que alguien va a romper sin
// darse cuenta: el WHERE es lo único que impide que la nota privada de una
// persona aparezca en el muro de otra.
//
// LA FASE B AÑADE la rama del GRUPO, que es la que no puede probarse con
// lógica de aplicación: compartir con un grupo no copia la nota a nadie, se
// decide al leer cruzando los destinatarios con los miembros ACTUALES. Por
// eso la prueba también borra a Ana del grupo y vuelve a mirar: si alguien
// cambia la consulta para «acordarse» de los miembros del reparto, esto es lo
// que la delata.

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
		`CREATE TABLE memo_grupos (
			id VARCHAR(64) PRIMARY KEY, empresa VARCHAR(64),
			nombre VARCHAR(120), creada VARCHAR(32))`,
		`CREATE TABLE memo_grupo_miembros (
			id VARCHAR(64) PRIMARY KEY, grupo_id VARCHAR(64),
			persona_id VARCHAR(64), empresa VARCHAR(64))`,
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
		{"n7", "default", "carlos", "grupo", "activo"},   // del grupo g1 (Ana es miembro)
		{"n8", "default", "sofia", "grupo", "activo"},    // del grupo g2 (Luis es miembro)
		{"n9", "default", "carlos", "grupo", "activo"},   // de g3, que está sin miembros
	}
	for _, n := range notas {
		if _, err := db.Exec(`INSERT INTO memo_notas (id, empresa, autor, titulo, texto, visibilidad, tipo, estado, creada)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			n.id, n.empresa, n.autor, "Título de "+n.id, "Texto de "+n.id, n.vis, "nota", n.estado, "2026-10-30 09:00:00"); err != nil {
			t.Fatalf("sembrar la nota %s: %v", n.id, err)
		}
	}

	// d1: la de Luis compartida con Ana en concreto. d2 y d3: reparto por
	// GRUPO, que es lo que no se guarda expandido a personas.
	semillas := []struct{ sql, motivo string }{
		{`INSERT INTO memo_destinatarios (id, nota_id, destino_tipo, destino_id, empresa)
			VALUES ('d1','n4','persona','ana','default')`, "compartida con Ana"},
		{`INSERT INTO memo_destinatarios (id, nota_id, destino_tipo, destino_id, empresa)
			VALUES ('d2','n7','grupo','g1','default')`, "n7 al grupo g1"},
		{`INSERT INTO memo_destinatarios (id, nota_id, destino_tipo, destino_id, empresa)
			VALUES ('d3','n8','grupo','g2','default')`, "n8 al grupo g2"},
		{`INSERT INTO memo_destinatarios (id, nota_id, destino_tipo, destino_id, empresa)
			VALUES ('d4','n9','grupo','g3','default')`, "n9 al grupo vacío g3"},
		{`INSERT INTO memo_grupos (id, empresa, nombre, creada)
			VALUES ('g1','default','Logística','2026-10-30 09:00:00'),
			       ('g2','default','Ventas','2026-10-30 09:00:00'),
			       ('g3','default','Sin gente','2026-10-30 09:00:00')`, "los tres grupos"},
		{`INSERT INTO memo_grupo_miembros (id, grupo_id, persona_id, empresa)
			VALUES ('m1','g1','ana','default'), ('m2','g2','luis','default')`,
			"Ana en Logística y Luis en Ventas"},
	}
	for _, s := range semillas {
		if _, err := db.Exec(s.sql); err != nil {
			t.Fatalf("sembrar %s: %v", s.motivo, err)
		}
	}

	ver := func(usuario string) map[string]bool {
		rows, err := db.Query(consultaMuro, "default", usuario, usuario, usuario)
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

	// Ana: lo suyo, la de empresa, la que Luis le compartió y la de SU grupo…
	// y nada más.
	ana := ver("ana")
	for _, esperada := range []string{"n1", "n2", "n4", "n7"} {
		if !ana[esperada] {
			t.Errorf("a Ana debía versele la nota %s: %v", esperada, ana)
		}
	}
	for _, vedada := range []string{"n3", "n5", "n6", "n8", "n9"} {
		if ana[vedada] {
			t.Errorf("a Ana NO debe versele la nota %s (privada ajena, de otra empresa, archivada, de otro grupo o de un grupo sin gente)", vedada)
		}
	}

	// Luis: lo suyo, la de empresa, la de su grupo; NO la de Ana ni la del
	// grupo de ella.
	luis := ver("luis")
	for _, esperada := range []string{"n2", "n3", "n4", "n8"} {
		if !luis[esperada] {
			t.Errorf("a Luis debía versele la nota %s: %v", esperada, luis)
		}
	}
	for _, vedada := range []string{"n1", "n5", "n6", "n7", "n9"} {
		if luis[vedada] {
			t.Errorf("a Luis NO debe versele la nota %s", vedada)
		}
	}

	// EL CIERRE DE LA FASE B: dejar de ser miembro quita la vista, sin que
	// se reescriba la nota ni se toque el destinatario que apunta al grupo.
	if _, err := db.Exec(`DELETE FROM memo_grupo_miembros WHERE persona_id = 'ana'`); err != nil {
		t.Fatalf("no se pudo dar de baja a Ana: %v", err)
	}
	trasBaja := ver("ana")
	if trasBaja["n7"] {
		t.Error("Ana dejó el grupo y sigue viendo la nota que solo se le compartió con él")
	}
	for _, esperada := range []string{"n1", "n2", "n4"} {
		if !trasBaja[esperada] {
			t.Errorf("la baja le quitó más de lo que debía: le falta la nota %s: %v", esperada, trasBaja)
		}
	}
	// Y la nota de Luis sigue donde estaba: la baja es de Ana, no del grupo.
	if !ver("luis")["n8"] {
		t.Error("la baja de Ana no debe tocar las notas del grupo de Luis")
	}
}
