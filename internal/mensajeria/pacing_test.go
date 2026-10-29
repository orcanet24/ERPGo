package mensajeria

import (
	"math/rand"
	"testing"
	"time"
)

func fecha(t *testing.T, valor string) time.Time {
	t.Helper()
	f, err := time.ParseInLocation("2006-01-02 15:04", valor, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestEnVentanaRespetaHorarioYDias(t *testing.T) {
	p := PoliticaPorDefecto()
	casos := []struct {
		cuando string
		dentro bool
	}{
		{"2026-10-28 10:00", true},  // miércoles
		{"2026-10-28 07:59", false}, // antes de abrir
		{"2026-10-28 17:00", false}, // al cerrar
		{"2026-10-28 16:59", true},
		{"2026-10-31 10:00", false}, // sábado
		{"2026-11-01 10:00", false}, // domingo
	}
	for _, c := range casos {
		if got := EnVentana(fecha(t, c.cuando), p); got != c.dentro {
			t.Errorf("%s: EnVentana=%v; se esperaba %v", c.cuando, got, c.dentro)
		}
	}
}

func TestProximaVentanaSaltaAlSiguienteDiaYAlLunes(t *testing.T) {
	p := PoliticaPorDefecto()
	casos := []struct{ desde, hasta string }{
		{"2026-10-28 07:00", "2026-10-28 08:00"}, // mismo día, antes de abrir
		{"2026-10-28 18:00", "2026-10-29 08:00"}, // después de cerrar → día siguiente
		{"2026-10-30 18:00", "2026-11-02 08:00"}, // viernes → lunes
		{"2026-10-31 10:00", "2026-11-02 08:00"}, // sábado → lunes
	}
	for _, c := range casos {
		got := ProximaVentana(fecha(t, c.desde), p)
		quiero := fecha(t, c.hasta)
		if !got.Equal(quiero) {
			t.Errorf("desde %s: próxima ventana %s; se esperaba %s", c.desde, got.Format("2006-01-02 15:04"), quiero.Format("2006-01-02 15:04"))
		}
	}

	dentro := fecha(t, "2026-10-28 10:00")
	if got := ProximaVentana(dentro, p); !got.Equal(dentro) {
		t.Errorf("dentro de la ventana no debe moverse: %s", got)
	}
}

func TestPausaEntreMensajesNuncaBajaDelSuelo(t *testing.T) {
	p := PoliticaDeRitmo{PausaMinS: 1, PausaMaxS: 5}
	azar := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		if d := PausaEntreMensajes(p, azar); d < PausaMinimaSegundos*time.Second {
			t.Fatalf("pausa de %s por debajo del suelo de %d s", d, PausaMinimaSegundos)
		}
	}

	p = PoliticaPorDefecto()
	for i := 0; i < 100; i++ {
		d := PausaEntreMensajes(p, azar)
		if d < time.Duration(p.PausaMinS)*time.Second || d > time.Duration(p.PausaMaxS)*time.Second {
			t.Fatalf("pausa %s fuera del rango %d–%d", d, p.PausaMinS, p.PausaMaxS)
		}
	}
}

func TestPausaLargaAparecePocasVeces(t *testing.T) {
	azar := rand.New(rand.NewSource(7))
	cuantas := 0
	for i := 0; i < 1000; i++ {
		d, hay := PausaLarga(azar)
		if hay {
			cuantas++
			if d < 5*time.Minute || d > 15*time.Minute {
				t.Fatalf("pausa larga fuera de rango: %s", d)
			}
		}
	}
	// 5 % esperado; se admite el margen de una muestra de 1000.
	if cuantas < 20 || cuantas > 80 {
		t.Fatalf("pausas largas = %d de 1000; se esperaba alrededor del 5 %%", cuantas)
	}
}

func TestCupoDeCalentamientoSubePorDias(t *testing.T) {
	casos := map[int]int{0: 20, 1: 40, 2: 80, 3: 120, 30: 120}
	for dia, quiero := range casos {
		if got := CupoDeCalentamiento(dia); got != quiero {
			t.Errorf("día %d: cupo %d; se esperaba %d", dia, got, quiero)
		}
	}
}

func TestRetardoDeReintentoSeAgota(t *testing.T) {
	casos := []struct {
		intento int
		espera  time.Duration
		hay     bool
	}{
		{1, time.Minute, true},
		{2, 5 * time.Minute, true},
		{3, 30 * time.Minute, true},
		{4, 0, false},
	}
	for _, c := range casos {
		d, hay := RetardoDeReintento(c.intento)
		if d != c.espera || hay != c.hay {
			t.Errorf("intento %d: %s/%v; se esperaba %s/%v", c.intento, d, hay, c.espera, c.hay)
		}
	}
}
