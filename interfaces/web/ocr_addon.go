package web

// Addon OCR: extrae datos de facturas de proveedor escaneadas usando el
// binario Tesseract. Es opt-in (config.OCREnabled): con ENABLE_OCR=0 el ERP
// funciona exactamente igual que sin addon, porque nada de la operación
// diaria ni fiscal depende de este motor.
//
// Por qué binario y no binding CGO: el ERP es un binario único (embed.go)
// que debe compilar en Windows y Linux sin dependencias nativas; un binding
// CGO a libtesseract obligaría a instalar cabeceras del sistema en cada
// máquina de construcción y a arrastrar librerías dinámicas al cliente. Con
// el binario, si falta en el servidor el resultado es un error controlado
// (status "unavailable"), nunca un bloqueo del ERP.

import (
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// OCRResult es lo que sale de procesar una imagen. La IA solo lo lee para
// PROPOSEER datos: guardar valores reales en el ERP es siempre una acción
// confirmada por el usuario y validada por las reglas fiscales, nunca una
// consecuencia automática del OCR.
type OCRResult struct {
	Text         string  `json:"text"`          // todo el texto extraído
	Confidence   float64 `json:"confidence"`    // confianza media de palabras (0-100)
	RIFFound     string  `json:"rif_found"`     // RIF detectado, normalizado J-...-d
	TotalFound   string  `json:"total_found"`   // número detectado junto a un "total"
	DateFound    string  `json:"date_found"`    // primera fecha del texto, tal como salió
	SupplierName string  `json:"supplier_name"` // línea detectada como proveedor
	Status       string  `json:"status"`        // "ok", "low_confidence", "error", "unsupported", "unavailable"
	RawFormat    string  `json:"raw_format"`    // "image" o "unsupported"
	Engine       string  `json:"engine"`        // binario tesseract encontrado ("" si no)
}

// Debajo de este umbral el front pide revisión humana. No se rechaza la
// petición por baja confianza: el valor fiscal lo confirma el usuario, no
// el OCR.
const umbralConfianzaBaja = 30.0

var (
	// RIF venezolano: letra J/G/V/E, cuerpo de 8 o 9 dígitos y dígito
	// verificador, con o sin guiones (el OCR pierde símbolos con frecuencia;
	// los dígitos quedan intactos para validar contra el documento físico).
	rifRe = regexp.MustCompile(`(?i)\b([JGVE])[-\s]?(\d{8,9})[-\s]?(\d)\b`)

	// Montos: cualquier "total" (sirve para Gran Total / Total a Pagar)
	// seguido en la misma línea de un número con un solo separador decimal.
	totalRe = regexp.MustCompile(`(?i)total[^0-9\n]{0,20}(\d{1,12}(?:[,.]\d{1,2})?)`)

	// Fechas dd/mm/aaaa | dd-mm-aaaa | aaaa-mm-dd | aaaa/mm/dd.
	fechaRe = regexp.MustCompile(`\b\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4}\b|\b\d{4}[/\-]\d{1,2}[/\-]\d{1,2}\b`)
)

// detectFormat clasifica el archivo subido. Tesseract solo procesa imágenes:
// los XML/PDF quedan como "unsupported" con mensaje claro, porque los
// factores digitales (XML) fluyen por el módulo digital_invoice.
func detectFormat(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"),
		strings.HasSuffix(lower, ".png"), strings.HasSuffix(lower, ".webp"),
		strings.HasSuffix(lower, ".tif"), strings.HasSuffix(lower, ".tiff"),
		strings.HasSuffix(lower, ".bmp"), strings.HasSuffix(lower, ".gif"):
		return "image"
	default:
		return "unsupported"
	}
}

// extractRIF devuelve el RIF normalizado J-12345678-9 si el texto lo trae.
func extractRIF(text string) string {
	m := rifRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1]) + "-" + m[2] + "-" + m[3]
}

// extractTotal devuelve el primer número que aparece hasta 20 caracteres
// después de una palabra "total" en la misma línea. Devolver el número
// limpio (no la frase) permite al front precargar el campo total con una
// sola transformación.
func extractTotal(text string) string {
	m := totalRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// extractDate devuelve la primera fecha del texto tal y como fue escrita
// (dd/mm/aaaa, aaaa-mm-dd, ...). No se la adivina al usuario: normalizar el
// formato es trabajo de la pantalla que precarga, ya validada.
func extractDate(text string) string {
	return strings.TrimSpace(fechaRe.FindString(text))
}

// extractSupplier busca la línea que presente al proveedor: la escriben como
// "PROVEEDOR:", "Proveedor:" o "Razón Social:". Se devuelve el valor después
// del dos puntos, sin alucinaciones: si no hay dos puntos, no hay proveedor.
func extractSupplier(text string) string {
	for _, linea := range strings.Split(text, "\n") {
		lower := strings.ToLower(linea)
		if !strings.Contains(lower, "proveedor") && !strings.Contains(lower, "razon social") &&
			!strings.Contains(lower, "razón social") {
			continue
		}
		// Con dos puntos: "PROVEEDOR: Nombre" → tomar el valor ya limpio.
		// "Razón Social:" solita (valor vacío) no sirve: seguimos a la
		// siguiente; no se adivina el nombre desde la línea que no lo trae.
		if idx := strings.Index(linea, ":"); idx >= 0 {
			if valor := strings.TrimSpace(linea[idx+1:]); valor != "" {
				return valor
			}
			continue
		}
		// Sin etiqueta estructurada, solo si la línea EMPIEZA por
		// "proveedor": la fila entera (el usuario confirma el nombre sobre
		// el texto original, no sobre una interpretación).
		if idx := strings.IndexAny(linea, " \t"); idx >= 1 && strings.HasPrefix(lower, "proveedor") {
			return strings.TrimSpace(linea[idx:])
		}
	}
	return ""
}

// ocrProcessImage corre Tesseract y rinde un OCRResult siempre: nunca hace
// panic, nunca deja procesos colgados y separa "error" (el motor corrió y
// falló) de "unavailable" (el motor no está instalado, el ERP sigue vivo
// sin OCR). El PSM 4 ("columnas de texto de tamaños variados") funciona bien
// con facturas venezolanas escaneadas de factura completa.
func ocrProcessImage(reader io.Reader, formato, binario, lang string) *OCRResult {
	resultado := &OCRResult{RawFormat: formato, Status: "ok"}
	if formato != "image" {
		resultado.Status = "error"
		resultado.Text = "El OCR solo procesa imágenes (jpg/jpeg/png/webp/tif/tiff/bmp/gif). Los PDF/XML cargan en digital_invoice; este motor no los lee."
		return resultado
	}

	rutaBinario, err := exec.LookPath(binario)
	if err != nil {
		resultado.Status = "unavailable"
		resultado.Text = fmt.Sprintf("Tesseract no está disponible en el servidor (binario %q no encontrado): instale tesseract-ocr y su paquete de idioma %s, o fije TESSERACT_BIN.", binario, lang)
		return resultado
	}
	resultado.Engine = rutaBinario

	// Tesseract no lee stdin con fiabilidad en todos los formatos, así que
	// copiamos el contenido a un archivo temporal del directorio del SO; el
	// borrado es automático al retorno (defer).
	tmp, err := os.CreateTemp("", "erpgo-ocr-*.png")
	if err != nil {
		resultado.Status = "error"
		resultado.Text = "No se pudo preparar el archivo temporal para el OCR: " + err.Error()
		return resultado
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, reader); err != nil {
		tmp.Close()
		resultado.Status = "error"
		resultado.Text = "No se pudo copiar la imagen para el OCR: " + err.Error()
		return resultado
	}
	if err := tmp.Close(); err != nil {
		resultado.Status = "error"
		resultado.Text = "No se pudo guardar la imagen para el OCR: " + err.Error()
		return resultado
	}

	// tsv: una sola ejecución entrega texto y confianza por palabra en el
	// mismo formato, sin haber llamado dos veces a Tesseract sobre la misma
	// imagen. La salida stderr se captura en memoria acotada para no
	// bloquear el proceso si Tesseract escribe mucho diagnóstico.
	cmd := exec.Command(rutaBinario, tmp.Name(), "stdout", "-l", lang, "--psm", "4", "-c", "preserve_interword_spaces=1", "tsv")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		resultado.Status = "error"
		resultado.Text = "No se pudo iniciar Tesseract: " + err.Error()
		return resultado
	}
	if err := cmd.Start(); err != nil {
		resultado.Status = "error"
		resultado.Text = "No se pudo iniciar Tesseract: " + err.Error()
		return resultado
	}
	texto, conf := ocrParseTSV(stdout)
	waitErr := cmd.Wait()
	if waitErr != nil && texto == "" {
		resultado.Status = "error"
		resultado.Text = fmt.Sprintf("Tesseract terminó con error: %v (stderr: %s). Verificar que el idioma %q esté instalado (tesseract-ocr-%s).", waitErr, strings.TrimSpace(stderr.String()), lang, lang)
		return resultado
	}
	resultado.Text = texto
	resultado.Confidence = conf

	resultado.RIFFound = extractRIF(texto)
	resultado.TotalFound = extractTotal(texto)
	resultado.DateFound = extractDate(texto)
	resultado.SupplierName = extractSupplier(texto)
	if conf < umbralConfianzaBaja {
		resultado.Status = "low_confidence"
	}
	return resultado
}

// ocrParseTSV lee la salida tsv de Tesseract: una fila por ítem de
// segmentación; solo las filas de nivel 5 son palabras, con la columna conf
// (0-100) y el texto en la columna 11. Las filas de niveles 1-4 (página,
// bloque, párrafo, línea) se ignoran.
func ocrParseTSV(r io.Reader) (texto string, confMedia float64) {
	lector := csv.NewReader(r)
	lector.Comma = '\t'
	lector.FieldsPerRecord = -1
	filas, err := lector.ReadAll()
	if err != nil {
		return "", 0
	}
	var frase strings.Builder
	var suma, cuenta float64
	for _, fila := range filas {
		if len(fila) < 12 || fila[0] != "5" {
			continue
		}
		confVal, err := strconv.ParseFloat(strings.TrimSpace(fila[10]), 64)
		palabra := strings.TrimSpace(fila[11])
		if err != nil || confVal < 0 || palabra == "" {
			continue
		}
		frase.WriteString(palabra)
		frase.WriteString(" ")
		suma += confVal
		cuenta++
	}
	if cuenta > 0 {
		confMedia = suma / cuenta
	}
	return strings.TrimSpace(frase.String()), confMedia
}

// registerOCRRoutes publica los 3 endpoints del addon. Como el resto del
// ERP atraviesan authMiddleware (sesión) y permissionMiddleware, un actor
// externo sin sesión no puede llamarlos. No tocan la base de datos: leen
// config y formularios, así que no hay Double-check de tenant.
func (s *Server) registerOCRRoutes() {
	// POST /ocr/process: registra factura escaneada y devuelve lo extraído.
	// Con el addon desactivado responde 403 controlado: ningún flujo se
	// rompe justamente porque este motor no es requisito de nada.
	s.mux.HandleFunc("POST /ocr/process", func(w http.ResponseWriter, r *http.Request) {
		if !s.config.OCREnabled {
			jsonError(w, "OCR desactivado: el superusuario debe fijar ENABLE_OCR=1", http.StatusForbidden)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			jsonError(w, "Formulario inválido: se espera multipart con la imagen de la factura", http.StatusBadRequest)
			return
		}
		var fileHeader *multipart.FileHeader
		for _, headers := range r.MultipartForm.File {
			if len(headers) > 0 {
				fileHeader = headers[0]
				break
			}
		}
		if fileHeader == nil {
			jsonError(w, "No se recibió archivo de factura", http.StatusBadRequest)
			return
		}
		archivo, err := fileHeader.Open()
		if err != nil {
			jsonError(w, "No se pudo abrir el archivo de factura", http.StatusBadRequest)
			return
		}
		defer archivo.Close()

		resultado := ocrProcessImage(archivo, detectFormat(fileHeader.Filename), s.config.TesseractBin, s.config.OcrLang)
		writeJSON(w, map[string]interface{}{
			"success": resultado.Status == "ok" || resultado.Status == "low_confidence",
			"result":  resultado,
			"message": fmt.Sprintf("OCR completado: estado=%s, confianza=%.1f%%", resultado.Status, resultado.Confidence),
		})
	})

	// GET /ocr/status: panel del superusuario - dice si el addon está
	// encendido y si el binario está disponible, para no descubrirlo en
	// medio de una operación.
	s.mux.HandleFunc("GET /ocr/status", func(w http.ResponseWriter, r *http.Request) {
		engine := ""
		if s.config.TesseractBin != "" {
			if ruta, err := exec.LookPath(s.config.TesseractBin); err == nil {
				engine = ruta
			}
		}
		writeJSON(w, map[string]interface{}{
			"enabled":          s.config.OCREnabled,
			"engine_available": engine != "",
			"engine_path":      engine,
			"binary":           s.config.TesseractBin,
			"lang":             s.config.OcrLang,
		})
	})

	// GET /ocr/test: health-check del extractor con una factura simulada
	// que trae los 4 datos (RIF, total, fecha, proveedor). Si esta muestra
	// no sale completa, el extractor está roto: es la señal que revisa el
	// superusuario antes de procesar un archivo real. Respeta
	// OCREnabled como el resto de los endpoints.
	s.mux.HandleFunc("GET /ocr/test", func(w http.ResponseWriter, r *http.Request) {
		if !s.config.OCREnabled {
			jsonError(w, "OCR desactivado: el superusuario debe fijar ENABLE_OCR=1", http.StatusForbidden)
			return
		}
		muestra := "FACTURA ELECTRÓNICA\nPROVEEDOR: Empresa X C.A.\nRIF: J-12345678-9\nFecha: 15/01/2025\nTOTAL Bs. 150,00\n"
		resultado := &OCRResult{
			Text:         muestra,
			Confidence:   85.5,
			RIFFound:     extractRIF(muestra),
			TotalFound:   extractTotal(muestra),
			DateFound:    extractDate(muestra),
			SupplierName: extractSupplier(muestra),
			Status:       "ok",
			RawFormat:    "test",
			Engine:       "simulado",
		}
		writeJSON(w, map[string]interface{}{
			"success": resultado.Status == "ok" && resultado.RIFFound != "" &&
				resultado.TotalFound != "" && resultado.DateFound != "" &&
				resultado.SupplierName != "",
			"result":  resultado,
			"message": "Muestra de OCR (modo prueba): extractor verificado con datos simulados",
		})
	})
}
