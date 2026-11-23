package persistence

// esquema_candados_test.go — Los candados del hallazgo D-2/D-1 (auditoría
// 2026-11-22) se prueban aquí, en SQLite, sin necesidad de un servidor MySQL:
// la división por cláusula y la tolerancia de siembras son lógica propia, no
// comportamiento del motor.

import (
	"database/sql"
	"strings"
	"testing"
)

func TestSentenciasEsquemaDivideAlterMultiClausula(t *testing.T) {
	// El caso canónico del bug: dos columnas nuevas en un ALTER, la primera
	// ya existía y MySQL abortaba las dos.
	entrada := "ALTER TABLE invoice ADD COLUMN IF NOT EXISTS currency_code VARCHAR(3), ADD COLUMN total_amount_usd DECIMAL(18,2), ADD COLUMN exchange_rate_source VARCHAR(20)"
	salida := SentenciasEsquema(entrada)
	if len(salida) != 3 {
		t.Fatalf("se querían 3 cláusulas, hubo %d: %#v", len(salida), salida)
	}
	for _, s := range salida {
		if !strings.HasPrefix(s, "ALTER TABLE invoice ADD COLUMN") {
			t.Errorf("cláusula mal reconstruida: %q", s)
		}
		if strings.Contains(s, "IF NOT EXISTS") {
			t.Errorf("sobra sintaxis de MariaDB: %q", s)
		}
		if strings.Count(s, "ADD COLUMN") != 1 {
			t.Errorf("dos cláusulas en una sentencia: %q", s)
		}
	}
	// El DECIMAL(18,2) lleva coma DENTRO de paréntesis: no debe partir ahí.
	if !strings.Contains(salida[1], "DECIMAL(18,2)") {
		t.Errorf("el tipo con coma interna se partió: %q", salida[1])
	}
}

func TestSentenciasEsquemaRespetaTablasConComillas(t *testing.T) {
	salida := SentenciasEsquema("ALTER TABLE `acc_vaucher` ADD COLUMN `a` INT, ADD COLUMN `b` INT")
	if len(salida) != 2 {
		t.Fatalf("2 cláusulas queridas, hubo %d: %#v", len(salida), salida)
	}
	if !strings.HasPrefix(salida[0], "ALTER TABLE `acc_vaucher` ADD COLUMN `a`") {
		t.Errorf("tabla con comillas mal preservada: %q", salida[0])
	}
}

func TestSentenciasEsquemaNoTocaLoQueNoEsAlter(t *testing.T) {
	for _, s := range []string{
		"CREATE TABLE IF NOT EXISTS x (id INT)",
		"INSERT INTO menu (id, label) VALUES (1, 'a')",
		"CREATE INDEX IF NOT EXISTS ix ON t (c)",
		"ALTER TABLE t ADD PRIMARY KEY (id)",
	} {
		got := SentenciasEsquema(s)
		want := strings.TrimSpace(sinMariaDBParaPrueba(s))
		if len(got) != 1 || got[0] != want {
			t.Errorf("SentenciasEsquema(%q) = %#v, se quería una sola sentencia %q", s, got, want)
		}
	}
}

func sinMariaDBParaPrueba(s string) string { return strings.TrimSpace(SinSintaxisDeMariaDB(s)) }

func TestSemillasCanonicasSoloTomaInserts(t *testing.T) {
	contenido := `
CREATE TABLE ` + "`x`" + ` (id INT);
INSERT INTO ` + "`x`" + ` (id) VALUES (1);
-- comentario
INSERT IGNORE INTO y (id) VALUES (2);
CREATE INDEX ix ON y (id);
REPLACE INTO z (id) VALUES (3);
CREATE TRIGGER trg BEFORE DELETE ON x FOR EACH ROW BEGIN INSERT INTO log VALUES (1); END;
`
	semillas := SemillasCanonicas(contenido)
	if len(semillas) != 3 {
		t.Fatalf("se querían 3 siembras (INSERT, INSERT IGNORE, REPLACE), hubo %d: %#v", len(semillas), semillas)
	}
	for _, s := range semillas {
		u := strings.ToUpper(strings.TrimSpace(s))
		if !strings.HasPrefix(u, "INSERT") && !strings.HasPrefix(u, "REPLACE") {
			t.Errorf("no es siembra: %q", s)
		}
	}
}

func TestAplicarSiembrasIdempotenteYHonesto(t *testing.T) {
	db, err := sql.Open("sqlite", t.TempDir()+"/siembra.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE catalogo (id INTEGER PRIMARY KEY, nombre TEXT)`); err != nil {
		t.Fatal(err)
	}
	semillas := []string{
		`INSERT INTO catalogo (id, nombre) VALUES (1, 'Facturas')`,
		`INSERT INTO catalogo (id, nombre) VALUES (2, 'Cobros')`,
	}
	aplicadas, toleradas, avisos := AplicarSiembras(db, semillas)
	if len(aplicadas) != 2 || toleradas != 0 || len(avisos) != 0 {
		t.Fatalf("primera pasada: aplicadas=%d toleradas=%d avisos=%v", len(aplicadas), toleradas, avisos)
	}

	// Segunda pasada (base instalada que arranca de nuevo): NADA duplicado,
	// NADA aplicado, NADA avisado. Eso es el «solo agrega, nunca pisa».
	aplicadas2, toleradas2, avisos2 := AplicarSiembras(db, semillas)
	if len(aplicadas2) != 0 || toleradas2 != 2 || len(avisos2) != 0 {
		t.Fatalf("segunda pasada: aplicadas=%d toleradas=%d avisos=%v", len(aplicadas2), toleradas2, avisos2)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM catalogo`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("filas duplicadas por la siembra: %d", n)
	}

	// Un error REAL no es "ya estaba": aviso, no silencio.
	raras := []string{`INSERT INTO tabla_que_no_existe (x) VALUES (1)`}
	_, _, avisos3 := AplicarSiembras(db, raras)
	if len(avisos3) != 1 || !strings.Contains(avisos3[0], "siembra no aplicada") {
		t.Fatalf("el fallo real debía quedar como aviso: %v", avisos3)
	}
}
