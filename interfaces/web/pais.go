package web

// pais.go — Dimensión PAÍS del ERP (plan de internacionalización 2026-11-24).
//
// POR QUÉ EXISTE. ERPGo nació venezolano: IGTF, retenciones IVA/ISLR,
// CONASEV, libros legales y textos al SENIAT. Las normas fiscales, por suerte,
// ya viven en datos (impuestos, plan de cuentas, cambios de cuentas por
// defecto, series); lo que faltaba era una dimensión que dijera EN QUÉ PAÍS
// está la empresa y, sobre todo, en qué ESTADO está su pack.
//
// LA REGLA QUE PROTEGE AL USUARIO (y al proyecto). Un pack a medio validar
// presentado como «cumple» no nos cuesta a nosotros: le cuesta al contador que
// firma delante del cliente. Por eso cada país declara su estado:
//
//	borrador    → normas cargadas, SIN validación contable local
//	validado    → revisado por un contador del país
//	certificado → probado en el ambiente oficial del regulador
//
// Y solo se OFRECE un país cuando su pack existe (PackDisponible). Marcar un
// país aquí no es «tenemos presencia»: es decir la verdad de en qué punto está.

import "database/sql"

// estadoPais es el nivel de confianza del pack de un país.
type estadoPais string

const (
	paisBorrador    estadoPais = "borrador"
	paisValidado    estadoPais = "validado"
	paisCertificado estadoPais = "certificado"
)

// perfilPais describe lo que el core necesita saber de un país. Todo lo demás
// (impuestos, plan de cuentas, cuentas, series, retenciones) viaja en su pack
// de datos, no en este struct: aquí solo viven los COMPORTAMIENTOS.
type perfilPais struct {
	Codigo    string // ISO 3166-1 alfa-2 (VE, CO, PE...)
	Nombre    string
	Moneda    string // ISO 4217 (VES, COP, PEN...)
	Decimales int
	// PrecioIncluyeImpuesto: en algunos países el precio de lista ya trae el
	// impuesto (p. ej. el consumidor ve el precio final) y el desglose se hace
	// hacia atrás. Cambia cómo se lee y redondea el formulario de factura.
	PrecioIncluyeImpuesto bool
	// TasaCambioFuente nombra de dónde sale el tipo de cambio (BCV, TRM,
	// SUNAT...). Vacío = el país no tiene régimen cambiario especial.
	TasaCambioFuente string
	// AddonFacturaElectronica es el addon que implementa el contrato de
	// emisión (digitalinvoice). Vacío = facturación electrónica no disponible.
	AddonFacturaElectronica string
	// PackDisponible: ¿existe ya el pack de datos de este país? Un país sin
	// pack no se puede seleccionar: elegirlo y quedarse con el plan de
	// cuentas venezolano sería un error disfrazado de configuración.
	PackDisponible bool
	Estado         estadoPais
	Notas          string
}

// paises es el registro ÚNICO de países. Agregar uno exige su pack y su
// estado; el mapa no es un catálogo de buenas intenciones.
var paises = map[string]perfilPais{
	"VE": {
		Codigo:                  "VE",
		Nombre:                  "Venezuela",
		Moneda:                  "VES",
		Decimales:               2,
		PrecioIncluyeImpuesto:   false,
		TasaCambioFuente:        "BCV",
		AddonFacturaElectronica: "digitalinvoice",
		PackDisponible:          true,
		Estado:                  paisValidado,
		Notas:                   "IGTF, retenciones IVA/ISLR, plan CONASEV y libros legales propios. País de origen del proyecto.",
	},
	"CO": {
		Codigo:                  "CO",
		Nombre:                  "Colombia",
		Moneda:                  "COP",
		Decimales:               2,
		PrecioIncluyeImpuesto:   true,
		TasaCambioFuente:        "TRM",
		AddonFacturaElectronica: "",
		PackDisponible:          false,
		Estado:                  paisBorrador,
		Notas:                   "Pack en construcción: PUC, IVA 19/5, retenciones. Falta validación de un contador colombiano y la emisión DIAN (por proveedor tecnológico).",
	},
	"PE": {
		Codigo:                  "PE",
		Nombre:                  "Perú",
		Moneda:                  "PEN",
		Decimales:               2,
		PrecioIncluyeImpuesto:   true,
		TasaCambioFuente:        "SUNAT",
		AddonFacturaElectronica: "",
		PackDisponible:          false,
		Estado:                  paisBorrador,
		Notas:                   "Registro inicial. Pack pendiente: PCGE, IGV 18 %, retenciones y CPE (SUNAT).",
	},
}

// paisVe es el perfil de respaldo: sin país configurado, la empresa es
// venezolana (compatibilidad con todas las instalaciones existentes).
var paisVe = paises["VE"]

// normalizarPaisCodigo limpia el código de país: mayúsculas y, si viene vacío
// o no es de dos letras, Venezuela. No adivina: un país desconocido NO se
// convierte en otro país, se queda en el de origen y el llamador lo reporta.
func normalizarPaisCodigo(codigo string) string {
	c := ""
	for _, r := range codigo {
		if r >= 'a' && r <= 'z' {
			r = r - 'a' + 'A'
		}
		if r >= 'A' && r <= 'Z' {
			c += string(r)
		}
		if len(c) == 2 {
			break
		}
	}
	return c
}

// paisDeCodigo devuelve el perfil del país. ok=false significa «código
// desconocido o sin pack»: el llamador decide si avisa o cae al origen; el
// perfil devuelto en ese caso es el venezolano para que NINGUNA ruta quede
// sin moneda, decimales o tasa.
func paisDeCodigo(codigo string) (perfilPais, bool) {
	c := normalizarPaisCodigo(codigo)
	if p, ok := paises[c]; ok {
		return p, true
	}
	return paisVe, false
}

// paisesDisponibles lista los países que HOY se pueden elegir (pack listo),
// ordenados por nombre. Es lo que debe usar el asistente de instalación y la
// pantalla de empresa: ofrecer un país sin pack sería vender humo.
func paisesDisponibles() []perfilPais {
	out := make([]perfilPais, 0, len(paises))
	for _, p := range paises {
		if p.PackDisponible {
			out = append(out, p)
		}
	}
	// Orden estable por nombre (sin depender del orden del mapa).
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Nombre < out[i].Nombre {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// paisDeEmpresa lee el país de la empresa de ESTA base. Sin fila o sin
// columna (instalación previa a la migración) devuelve Venezuela: el ERP
// siempre tiene un perfil válido, nunca queda a medio configurar.
func paisDeEmpresa(db *sql.DB) perfilPais {
	if db == nil {
		return paisVe
	}
	var codigo sql.NullString
	if err := db.QueryRow(`SELECT country_code FROM company_information LIMIT 1`).Scan(&codigo); err != nil {
		return paisVe
	}
	if !codigo.Valid || codigo.String == "" {
		return paisVe
	}
	p, _ := paisDeCodigo(codigo.String)
	return p
}
