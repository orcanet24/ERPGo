package web

// Pruebas del aviso de cierre de sesión con turno de caja abierto.
//
// Lo que se prueba aquí es la parte que se decide sin base de datos: el texto
// que lee la persona y el formato de la duración. Y, sobre las consultas que
// buscan los turnos, lo mismo que se comprueba con las guardas de borrado: que
// apunten a tablas y columnas que existen en las migraciones, porque una consulta
// con un nombre mal escrito devuelve error, el aviso se salta en silencio y el
// turno abierto vuelve a quedar invisible.

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"erpgow"
)

func TestDuracionLegible(t *testing.T) {
	casos := []struct {
		entrada  time.Duration
		esperado string
	}{
		{30 * time.Second, "menos de un minuto"},
		{0, "menos de un minuto"},
		// Un reloj de base desfasado puede dar negativo: no se muestra "-3 min".
		{-5 * time.Minute, "menos de un minuto"},
		{35 * time.Minute, "35 min"},
		{2 * time.Hour, "2 h"},
		{4*time.Hour + 20*time.Minute, "4 h 20 min"},
		{26 * time.Hour, "1 días 2 h"},
		{48 * time.Hour, "2 días"},
	}
	for _, c := range casos {
		if got := duracionLegible(c.entrada); got != c.esperado {
			t.Errorf("duracionLegible(%v) = %q, esperado %q", c.entrada, got, c.esperado)
		}
	}
}

func TestTextoDeTurnosAbiertos(t *testing.T) {
	if got := textoDeTurnosAbiertos(nil); got != "" {
		t.Errorf("sin turnos abiertos no hay nada que avisar; salió %q", got)
	}

	texto := textoDeTurnosAbiertos([]turnoAbierto{
		{Que: "turno de caja", Desde: "08:12", Duracion: "4 h 20 min", Terminal: "CAJA-1"},
		{Que: "turno del POS", Desde: "09:00", Duracion: "3 h 32 min"},
	})
	// El aviso tiene que decir CUÁL, DESDE CUÁNDO y CUÁNTO: sin eso no se puede
	// decidir si volver a cerrarlo.
	for _, esperado := range []string{
		"turno de caja", "08:12", "4 h 20 min", "CAJA-1",
		"turno del POS", "3 h 32 min",
	} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("el aviso %q no dice %q", texto, esperado)
		}
	}
	// Y tiene que decir que el turno NO se cierra solo: es la parte que evita
	// pensar que la sesión se llevó el turno por delante.
	if !strings.Contains(texto, "no se cierra al cerrar la sesión") {
		t.Errorf("el aviso no aclara que el turno sigue abierto: %q", texto)
	}
}

// TestLaPantallaDeTurnoAbiertoSeRenderiza monta la plantilla como lo hace el
// servidor. La pantalla es la única salida visible del aviso: si la plantilla no
// se pudiera ejecutar, el cierre de sesión con turno abierto devolvería un error
// de servidor, que es exactamente lo contrario de lo que se busca.
func TestLaPantallaDeTurnoAbiertoSeRenderiza(t *testing.T) {
	tpl, err := template.New("logout_turno_abierto.html").ParseFS(erpgow.Templates(), "templates/logout_turno_abierto.html")
	if err != nil {
		t.Fatalf("la pantalla de turno abierto no se puede cargar: %v", err)
	}
	datos := map[string]interface{}{
		"AppName": "ERPGo", "Theme": "light", "Density": "confortable",
		"Texto": textoDeTurnosAbiertos([]turnoAbierto{
			{Que: "turno de caja", Desde: "08:12", Duracion: "4 h 20 min", Terminal: "CAJA-1"},
		}),
		"Turnos": []turnoAbierto{
			{Que: "turno de caja", Desde: "08:12", Duracion: "4 h 20 min", Terminal: "CAJA-1"},
		},
	}
	var html strings.Builder
	if err := tpl.Execute(&html, datos); err != nil {
		t.Fatalf("la pantalla de turno abierto no se puede representar: %v", err)
	}
	salida := html.String()
	for _, esperado := range []string{"turno de caja", "08:12", "4 h 20 min", "CAJA-1", "forzar=1"} {
		if !strings.Contains(salida, esperado) {
			t.Errorf("la pantalla no muestra %q: el usuario no podría decidir qué hacer", esperado)
		}
	}
}

func TestLasConsultasDeTurnosApuntanAEsquemaReal(t *testing.T) {
	esquema := esquemaDeLasMigraciones(t)
	if len(consultasTurnosAbiertos) == 0 {
		t.Fatal("no hay ninguna consulta de turnos: el aviso nunca encontraría nada")
	}
	for _, c := range consultasTurnosAbiertos {
		if strings.TrimSpace(c.Que) == "" {
			t.Errorf("una consulta de turnos no dice qué cuenta: %q", c.SQL)
		}
		if n := strings.Count(c.SQL, "?"); n != 1 {
			t.Errorf("(%s): la consulta lleva %d parámetros y se pasa uno solo: %q", c.Que, n, c.SQL)
		}
		tablas, columnas := tablasYColumnasCitadas(c.SQL)
		if len(tablas) == 0 {
			t.Errorf("(%s): la consulta no cita ninguna tabla del esquema: %q", c.Que, c.SQL)
			continue
		}
		declaradas := map[string]bool{}
		for _, tabla := range tablas {
			cols, ok := esquema[tabla]
			if !ok {
				t.Errorf("(%s): la tabla %q no existe en las migraciones: %q", c.Que, tabla, c.SQL)
				continue
			}
			for col := range cols {
				declaradas[col] = true
			}
		}
		for _, col := range columnas {
			if !declaradas[col] {
				t.Errorf("(%s): la columna %q no pertenece a ninguna de las tablas citadas (%v); "+
					"la consulta fallaría y el turno abierto quedaría invisible: %q", c.Que, col, tablas, c.SQL)
			}
		}
	}
}
