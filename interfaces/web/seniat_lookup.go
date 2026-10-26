// seniat_lookup.go — Autollenado del cliente con datos reales del contribuyente.
//
// Cómo funciona: si el admin activó la consulta en Ajustes Fiscales
// (`seniat_autofill`), las ventanas de cliente (modal rápida de /invoice/new y
// Cliente Nuevo) preguntan aquí al SENIAT cuando se escribe un RIF o cédula.
// Si hay datos, rellenan el nombre con la razón social REAL; si no los hay
// (SENIAT caído, captcha no resuelto, contribuyente inexistente), devuelven
// `found:false` y la modal muestra UNA alerta simple — nunca bloquea la venta:
// el cliente se sigue creando a mano.
//
// El operador solo ve la alerta genérica; el DETALLE técnico (¿faltó el OCR?,
// ¿el SENIAT devolvió 403?, ¿falló el captcha?) va al log del servidor y a la
// prueba de Ajustes Fiscales, para que "no funciona" sea diagnosticable en vez
// de adivinable.
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

// consultaContribuyente ejecuta el flujo completo contra la web del SENIAT.
//
// Devuelve además la letra/dígitos separados (el SENIAT usa dos formularios:
// RIF con p_rif, cédula con p_cedula) y un `detalle` legible con el paso donde
// falló — para el log y para la prueba del admin, no para el operador.
func consultaContribuyente(rif, ocrCmd string) (res *seniat.Resultado, letra, digitos, detalle string, err error) {
	letra = ""
	digitos = rif
	if len(rif) > 0 {
		letra = string(rif[0])
		digitos = rif[1:]
	}

	// Decide por la letra: J/G/mayúsculas con 9 dígitos son RIF; V/E con pocos
	// dígitos sin verificador van como cédula.
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

	ocr := seniat.OCRPorComando(ocrCmd)
	res, err = seniat.Consultar(valorConsulta, esCedula, ocr, 20*time.Second)
	if err != nil {
		detalle = clasificarFallo(err)
		return nil, letra, digitos, detalle, err
	}
	if res == nil || res.RazonSocial == "" {
		detalle = "el SENIAT respondió sin datos para " + valorConsulta +
			" (captcha resuelto: puede ser contribuyente inexistente o página cambiada)"
		return nil, letra, digitos, detalle, fmt.Errorf("sin datos del contribuyente")
	}
	detalle = "ok: " + res.RazonSocial
	return res, letra, digitos, detalle, nil
}

// clasificarFallo traduce el error crudo a una causa accionable. Es lo que
// distingue "instale ddddocr" de "el SENIAT me bloqueó la IP" de "el captcha
// no se resolvió".
func clasificarFallo(err error) string {
	m := err.Error()
	switch {
	case strings.Contains(m, "OCR del captcha falló"):
		return "el comando OCR no corrió en el servidor (¿python3 y ddddocr instalados? o el comando OCR de abajo está mal escrito): " + m
	case strings.Contains(m, "captcha tras"):
		return "el captcha nunca se resolvió (3 intentos): el OCR leyó mal la imagen — " + m
	case strings.Contains(m, "403"):
		return "el SENIAT bloqueó la petición desde este servidor (403): la IP del servidor está filtrada por el WAF del SENIAT"
	case strings.Contains(m, "no se pudo abrir"), strings.Contains(m, "no se pudo descargar"),
		strings.Contains(m, "no se pudo enviar"), strings.Contains(m, "context deadline"):
		return "sin conexión con la web del SENIAT desde el servidor: " + m
	default:
		return m
	}
}

// seniatLookup hace la consulta del contribuyente.
func (s *Server) seniatLookup(w http.ResponseWriter, r *http.Request) {
	fs := s.loadFiscalSettings(r)
	rif := NormalizeRIF(strings.TrimSpace(r.FormValue("rif")))
	// Solo con el ajuste del admin activo. Desactivo = la modal no pregunta
	// nada y crea el cliente a mano como siempre.
	if rif == "" || !fs.SENIATAutofill {
		writeJSON(w, map[string]interface{}{"status": 1, "enabled": false})
		return
	}

	res, letra, digitos, detalle, err := consultaContribuyente(rif, fs.SENIATOcrCmd)
	if err != nil {
		// Conectividad/OCR/captcha: la MISMA alerta simple, con detalle solo
		// en el log del servidor para el admin.
		log.Printf("seniat: consulta %s: %s", rif, detalle)
		writeJSON(w, map[string]interface{}{
			"status":  1,
			"enabled": true,
			"found":   false,
			"error":   msgSeniatNoEncontrado,
		})
		return
	}

	nombre := res.RazonSocial
	if strings.HasPrefix(rif, "V") || strings.HasPrefix(rif, "E") || strings.HasPrefix(rif, "P") {
		if res.Rif == "" {
			res.Rif = fmt.Sprintf("%s%s", letra, digitos)
		}
	}
	writeJSON(w, map[string]interface{}{
		"status":    1,
		"enabled":   true,
		"found":     true,
		"rif":       normalizeRIFBackend(res.Rif),
		"name":      nombre,
		"actividad": res.Actividad,
		"condicion": res.Condicion,
	})
}

// seniatTest es la prueba del ADMIN en Ajustes Fiscales: corre la consulta real
// y devuelve el detalle exacto del fallo. A diferencia del lookup del operador,
// aquí SÍ se muestra el motivo — es la pantalla que convierte "no funciona" en
// "falta ddddocr" / "el SENIAT me da 403".
func (s *Server) seniatTest(w http.ResponseWriter, r *http.Request) {
	fs := s.loadFiscalSettings(r)
	rif := NormalizeRIF(strings.TrimSpace(r.FormValue("rif")))
	if rif == "" {
		jsonError(w, "Escriba un RIF o cédula de prueba (ej: J294570968 o V14989834)", http.StatusBadRequest)
		return
	}
	_, _, _, detalle, err := consultaContribuyente(rif, fs.SENIATOcrCmd)
	writeJSON(w, map[string]interface{}{
		"status":  1,
		"ok":      err == nil,
		"enabled": fs.SENIATAutofill,
		"rif":     rif,
		"ocr":     seniat.ComandoOCR(fs.SENIATOcrCmd),
		"detail":  detalle,
	})
}

// normalizeRIFBackend es NormalizeRIF pero tolerante a nil/espacios raros que
// la web del SENIAT pueda traer.
func normalizeRIFBackend(rif string) string {
	return NormalizeRIF(strings.TrimSpace(rif))
}
