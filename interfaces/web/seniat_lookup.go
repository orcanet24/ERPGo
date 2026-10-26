// seniat_lookup.go — Autollenado del cliente con datos reales del contribuyente.
//
// Cómo funciona: si el admin activó la consulta en Ajustes Fiscales
// (`seniat_autofill`), las ventanas de cliente (modal rápido de /invoice/new y
// Cliente Nuevo) preguntan aquí al SENIAT cuando se escribe un RIF o cédula.
// Si hay datos, rellenan el nombre con la razón social REAL; si no los hay
// (SENIAT caído, captcha no resuelto, contribuyente inexistente), devuelven
// `found:false` y la modal muestra UNA alerta simple — nunca bloquea la venta:
// el cliente se sigue creando a mano.
package web

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"erpgow/internal/seniat"
)

// msgSeniatNoEncontrado es la alerta ÚNICA que ve el operador. Sin detalle
// técnico: puede ser falla de conexión, captcha o contribuyente inexistente.
const msgSeniatNoEncontrado = "No se encontraron datos fiscales en la web del SENIAT (puede ser por fallas de conexión)"

// seniatLookup hace la consulta del contribuyente.
func (s *Server) seniatLookup(w http.ResponseWriter, r *http.Request) {
	// Solo con el ajuste del admin activo. Desactivo = la modal no pregunta
	// nada y crea el cliente a mano como siempre.
	rif := NormalizeRIF(strings.TrimSpace(r.FormValue("rif")))
	if rif == "" {
		writeJSON(w, map[string]interface{}{"status": 1, "enabled": false})
		return
	}

	letra := ""
	digitos := rif
	if len(rif) > 0 {
		letra = string(rif[0])
		digitos = rif[1:]
	} else {
		digitos = rif
	}

	// La web del SENIAT maneja dos formularios: RIF (letra + 8+1) y cédula
	// (solo el número). Decide por la letra: J/G/mayúsculas con 9 dígitos son
	// RIF; V/E con pocos dígitos sin verificador van como cédula.
	esCedula := false
	if letra == "V" || letra == "E" || letra == "P" {
		soloDigitos := true
		for i := 0; i < len(digitos); i++ {
			if digitos[i] < '0' || digitos[i] > '9' {
				soloDigitos = false
				break
			}
		}
		esCedula = soloDigitos && len(digitos) < 9
	}
	valorConsulta := rif
	if esCedula {
		valorConsulta = digitos
	}

	cmd := s.loadFiscalSettings(r).SENIATOcrCmd
	ocr := seniat.OCRPorComando(cmd)

	res, err := seniat.Consultar(valorConsulta, esCedula, ocr, 20*time.Second)
	if err != nil {
		// Conectividad/OCR/captcha: la MISMA alerta simple, con detalle solo
		// en el log del servidor para el admin.
		log.Printf("seniat: consulta %s: %v", rif, err)
		writeJSON(w, map[string]interface{}{
			"status":  1,
			"enabled": true,
			"found":   false,
			"error":   msgSeniatNoEncontrado,
		})
		return
	}

	if res == nil || res.RazonSocial == "" {
		writeJSON(w, map[string]interface{}{
			"status":  1,
			"enabled": true,
			"found":   false,
			"error":   msgSeniatNoEncontrado,
		})
		return
	}

	nombre := res.RazonSocial
	if esCedula && res.Rif == "" {
		res.Rif = fmt.Sprintf("%s%s", letra, digitos)
	}
	writeJSON(w, map[string]interface{}{
		"status":      1,
		"enabled":     true,
		"found":       true,
		"rif":         normalizeRIFBackend(res.Rif),
		"name":        nombre,
		"actividad":   res.Actividad,
		"condicion":   res.Condicion,
	})
}

// normalizeRIFBackend es NormalizeRIF pero tolerante a nil/espacios raros que
// la web del SENIAT pueda traer.
func normalizeRIFBackend(rif string) string {
	return NormalizeRIF(strings.TrimSpace(rif))
}
