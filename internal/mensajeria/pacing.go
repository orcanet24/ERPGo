package mensajeria

// pacing.go — El ritmo de los envíos, como funciones puras.
//
// POR QUÉ AQUÍ Y NO EN EL DESPACHADOR. «¿Se puede mandar ahora?» y «¿cuándo
// toca?» son reglas de negocio que se prueban con una tabla de casos, sin base
// de datos ni reloj real. Si vivieran dentro del bucle que habla con el daemon,
// cada prueba necesitaría un daemon falso y esperar minutos.
//
// NADA DE ESTE ARCHIVO GARANTIZA QUE EL NÚMERO NO SEA BANEADO. Reduce la señal
// de automatización: horario, cupos, pausas variables y calentamiento. La
// decisión de usar un número no oficial sigue siendo del dueño del sistema.

import (
	"math/rand"
	"time"
)

// PoliticaDeRitmo son los límites de un dispositivo. Los valores de fábrica
// están en msg_device; el despachador los lee de ahí y los pasa aquí.
type PoliticaDeRitmo struct {
	CupoDiario     int
	CupoHorario    int
	PausaMinS      int
	PausaMaxS      int
	DiasLaborables []time.Weekday
	HoraDesde      int // minutos desde medianoche
	HoraHasta      int // minutos desde medianoche (exclusivo)
}

// PoliticaPorDefecto es el punto de partida: 08:00–17:00, lunes a viernes,
// 120 mensajes al día y 25 por hora. El mínimo duro de pausa es 20 s y se
// aplica en PausaEntreMensajes aunque la política diga menos.
func PoliticaPorDefecto() PoliticaDeRitmo {
	return PoliticaDeRitmo{
		CupoDiario: 120, CupoHorario: 25, PausaMinS: 45, PausaMaxS: 180,
		DiasLaborables: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
		HoraDesde:      8 * 60,
		HoraHasta:      17 * 60,
	}
}

// PausaMinimaSegundos es el suelo que NO se puede bajar desde la pantalla.
// Si se pudiera, el módulo sería una herramienta de spam con un checkbox.
const PausaMinimaSegundos = 20

// EnVentana dice si `ahora` cae dentro del horario permitido.
func EnVentana(ahora time.Time, p PoliticaDeRitmo) bool {
	if len(p.DiasLaborables) > 0 && !diaPermitido(ahora.Weekday(), p.DiasLaborables) {
		return false
	}
	m := ahora.Hour()*60 + ahora.Minute()
	if p.HoraDesde == p.HoraHasta {
		return true // ventana de 24 h
	}
	if p.HoraDesde < p.HoraHasta {
		return m >= p.HoraDesde && m < p.HoraHasta
	}
	// Ventana que cruza medianoche (p. ej. 22:00–06:00).
	return m >= p.HoraDesde || m < p.HoraHasta
}

// ProximaVentana devuelve el primer instante permitido a partir de `ahora`.
// Si ya está dentro, devuelve `ahora` sin tocarlo.
func ProximaVentana(ahora time.Time, p PoliticaDeRitmo) time.Time {
	if EnVentana(ahora, p) {
		return ahora
	}
	// Se avanza día a día hasta encontrar uno permitido. El tope de 8 días
	// evita un bucle infinito si la política no tiene ningún día válido.
	for i := 0; i < 8; i++ {
		dia := ahora.AddDate(0, 0, i)
		if len(p.DiasLaborables) > 0 && !diaPermitido(dia.Weekday(), p.DiasLaborables) {
			continue
		}
		candidato := time.Date(dia.Year(), dia.Month(), dia.Day(), p.HoraDesde/60, p.HoraDesde%60, 0, 0, ahora.Location())
		if candidato.After(ahora) || (i > 0 && candidato.Equal(ahora)) {
			return candidato
		}
	}
	return ahora.Add(24 * time.Hour)
}

// PausaEntreMensajes devuelve una pausa aleatoria dentro de la política, con el
// suelo duro de PausaMinimaSegundos. La aleatoriedad es deliberada: una cadencia
// perfecta es la firma de un robot.
func PausaEntreMensajes(p PoliticaDeRitmo, azar *rand.Rand) time.Duration {
	min, max := p.PausaMinS, p.PausaMaxS
	if min < PausaMinimaSegundos {
		min = PausaMinimaSegundos
	}
	if max < min {
		max = min
	}
	if azar == nil {
		azar = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	seg := min
	if max > min {
		seg = min + azar.Intn(max-min+1)
	}
	return time.Duration(seg) * time.Second
}

// PausaLarga decide si toca una pausa de varios minutos. Un 5 % de los envíos
// rompe la cadencia sin frenar el trabajo del día.
func PausaLarga(azar *rand.Rand) (time.Duration, bool) {
	if azar == nil {
		return 0, false
	}
	if azar.Intn(100) >= 5 {
		return 0, false
	}
	return time.Duration(5+azar.Intn(11)) * time.Minute, true
}

// CupoDeCalentamiento sube el tope diario de un número nuevo: 20, 40, 80 y 120
// a partir del cuarto día. Un número recién vinculado que empieza mandando 120
// mensajes es la señal más clara de automatización.
func CupoDeCalentamiento(dia int) int {
	switch {
	case dia <= 0:
		return 20
	case dia == 1:
		return 40
	case dia == 2:
		return 80
	default:
		return 120
	}
}

// RetardoDeReintento es la espera antes de volver a intentar un fallo temporal:
// 1, 5 y 30 minutos. A partir del cuarto intento ya no se reintenta.
func RetardoDeReintento(intento int) (time.Duration, bool) {
	switch intento {
	case 1:
		return time.Minute, true
	case 2:
		return 5 * time.Minute, true
	case 3:
		return 30 * time.Minute, true
	}
	return 0, false
}

func diaPermitido(d time.Weekday, dias []time.Weekday) bool {
	for _, x := range dias {
		if x == d {
			return true
		}
	}
	return false
}
