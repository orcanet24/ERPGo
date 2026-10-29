// Package mensajeria contiene la lógica pura del módulo de mensajería: la
// normalización de teléfonos y las capacidades de cada proveedor.
//
// Vive fuera de interfaces/web a propósito. Normalizar un número es una regla
// de negocio que se usa en fichas, importaciones, avisos, chat y OTP; si
// estuviera repartida por los handlers, cada pantalla tendría su propia idea de
// qué es un teléfono válido y los avisos «se perderían» sin dar error.
package mensajeria

import (
	"errors"
	"strings"
)

// pais describe cómo se escribe un número nacional y cuál es su código.
//
// LargoMin/LargoMax son los dígitos del número NACIONAL, sin el cero inicial y
// sin el código de país. Se usan rangos y no un valor fijo porque hay países
// con más de una longitud (Brasil, Italia, Alemania).
type pais struct {
	Codigo   string
	LargoMin int
	LargoMax int
}

// paises es la tabla de los países que el ERP puede usar hoy. No pretende ser
// el mundo entero: es la lista de los que aparecen en las fichas de clientes y
// proveedores de la instalación, y se amplía con una fila, no con un `if`.
//
// La ausencia de un país no rompe: el número internacional explícito (`+…` o
// `00…`) se acepta igual con la regla E.164 (8 a 15 dígitos). Lo que no se hace
// nunca es adivinar el país de un número nacional de un país desconocido.
var paises = map[string]pais{
	"VE": {"58", 10, 10},
	"CO": {"57", 10, 10},
	"US": {"1", 10, 10},
	"CA": {"1", 10, 10},
	"DO": {"1", 10, 10},
	"MX": {"52", 10, 10},
	"AR": {"54", 10, 10},
	"CL": {"56", 9, 9},
	"PE": {"51", 9, 9},
	"EC": {"593", 9, 9},
	"PA": {"507", 8, 8},
	"BR": {"55", 10, 11},
	"ES": {"34", 9, 9},
	"PT": {"351", 9, 9},
	"FR": {"33", 9, 9},
	"IT": {"39", 9, 10},
	"DE": {"49", 10, 11},
	"GB": {"44", 10, 10},
}

// PaisPorDefecto es el que se usa cuando la empresa no ha elegido otro.
const PaisPorDefecto = "VE"

// ErrorTelefono explica por qué un número no se pudo normalizar. Lleva el valor
// original para que la pantalla pueda mostrar exactamente lo que el usuario
// escribió, sin obligarlo a adivinar qué se rechazó.
type ErrorTelefono struct {
	Raw    string
	Pais   string
	Motivo string
}

func (e *ErrorTelefono) Error() string {
	return "teléfono «" + e.Raw + "» (" + e.Pais + "): " + e.Motivo
}

// Normalizar convierte lo que hay en una ficha al formato que esperan WhatsApp
// y Telegram: código de país + número, solo dígitos, sin `+`, sin ceros
// iniciales y sin separadores.
//
// REGLA DE ORO: si no se puede normalizar, se devuelve error y NO se adivina.
// Un número inventado es peor que un aviso bloqueado: el bloqueo se ve y se
// corrige; el mensaje «enviado» a un número que no existe, no.
func Normalizar(raw, iso string) (string, error) {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if iso == "" {
		iso = PaisPorDefecto
	}
	p, conocido := paises[iso]

	digitos, internacional, err := limpiar(raw)
	if err != nil {
		return "", &ErrorTelefono{Raw: raw, Pais: iso, Motivo: err.Error()}
	}

	if internacional {
		if conocido && strings.HasPrefix(digitos, p.Codigo) &&
			enRango(len(digitos), len(p.Codigo)+p.LargoMin, len(p.Codigo)+p.LargoMax) {
			return digitos, nil
		}
		// País explícito distinto del configurado: se respeta. Solo se exige
		// que quepa en E.164, que es lo que cualquier operador acepta.
		if enRango(len(digitos), 8, 15) {
			return digitos, nil
		}
		return "", &ErrorTelefono{Raw: raw, Pais: iso, Motivo: "no tiene una longitud internacional válida (8 a 15 dígitos)"}
	}

	if !conocido {
		return "", &ErrorTelefono{Raw: raw, Pais: iso,
			Motivo: "el país no está en la tabla y el número no trae prefijo internacional (+ o 00); no se adivina"}
	}

	// Ya viene con código de país aunque no lleve «+»: «58-414-…».
	if strings.HasPrefix(digitos, p.Codigo) &&
		enRango(len(digitos), len(p.Codigo)+p.LargoMin, len(p.Codigo)+p.LargoMax) {
		return digitos, nil
	}

	nacional := strings.TrimLeft(digitos, "0")
	if !enRango(len(nacional), p.LargoMin, p.LargoMax) {
		return "", &ErrorTelefono{Raw: raw, Pais: iso,
			Motivo: "el número nacional no tiene la longitud esperada para " + iso}
	}
	return p.Codigo + nacional, nil
}

// enRango evita repetir la comparación en cada rama.
func enRango(n, min, max int) bool { return n >= min && n <= max }

// limpiar quita separadores y decide si el número trae prefijo internacional.
func limpiar(raw string) (string, bool, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false, errors.New("está vacío")
	}
	bajo := strings.ToLower(s)
	// Una extensión no es un teléfono de WhatsApp: se rechaza en vez de
	// recortarla, porque recortar «ext 12» convertiría el número de la central
	// en el de otra persona.
	if strings.Contains(bajo, "ext") || strings.Contains(bajo, "anexo") {
		return "", false, errors.New("tiene una extensión; WhatsApp y Telegram no marcan extensiones")
	}

	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')' || r == '/' || r == '\t':
			// separadores tolerados
		case r == '+':
			// se procesa abajo
		default:
			return "", false, errors.New("contiene caracteres que no son de un teléfono")
		}
	}
	digitos := b.String()
	if digitos == "" {
		return "", false, errors.New("no tiene dígitos")
	}

	internacional := strings.HasPrefix(s, "+")
	if strings.HasPrefix(digitos, "00") {
		internacional = true
		digitos = strings.TrimLeft(digitos[2:], "0")
	}
	if internacional {
		digitos = strings.TrimLeft(digitos, "0")
		if digitos == "" {
			return "", false, errors.New("solo trae el prefijo internacional")
		}
	}
	return digitos, internacional, nil
}

// Enmascarar deja ver solo el final del número, que es lo que necesita una
// pantalla para que el usuario reconozca el teléfono sin exponerlo entero.
func Enmascarar(msisdn string) string {
	d := soloDigitos(msisdn)
	if len(d) <= 4 {
		return d
	}
	return "+" + d[:len(d)-4] + " ••• " + d[len(d)-4:]
}

func soloDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
