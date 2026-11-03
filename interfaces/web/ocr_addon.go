package web

import (
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/otiai10/gosseract/v2"
)

// OCRResult es lo que la IA lee de la factura/Comprobante
type OCRResult struct {
	Text         string  `json:"text"`          // todo el texto extraído
	Confidence   float64 `json:"confidence"`    // nivel de confianza Tesseract (0-100)
	RIFFound     string  `json:"rif_found"`     // RIF detectado (si aplica)
	TotalFound   string  `json:"total_found"`   // total/grand total detectado
	DateFound    string  `json:"date_found"`    // fecha detectada
	InvoiceNo    string  `json:"invoice_no"`    // número de factura/Comprobante
	SupplierName string  `json:"supplier_name"` // nombre proveedor
	Status       string  `json:"status"`        // "ok", "low_confidence", "error"
	RawFormat    string  `json:"raw_format"`    // "xml", "pdf", "image"
}

// OCRConfig holds the activation state - setado por superusuario en settings
type OCRConfig struct {
	Enabled bool `json:"enabled"` // 1=activado, 0=desactivado (default: 0)
}

// OCRProcessor es el handler para operaciones OCR
type OCRProcessor struct {
	config *OCRConfig
}

// NewOCRProcessor crea un nuevo procesador OCR con la configuración dada
func NewOCRProcessor(config *OCRConfig) *OCRProcessor {
	return &OCRProcessor{config: config}
}

// detectFormat detecta si el archivo es XML, PDF o imagen
func detectFormat(filename string) string {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".xml") {
		return "xml"
	}
	if strings.HasSuffix(lower, ".pdf") {
		return "pdf"
	}
	if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp") ||
		strings.Contains(lower, "image/") {
		return "image"
	}
	return "unknown"
}

// extractRIF intenta encontrar un RIF en el texto OCR
func extractRIF(text string) string {
	// Patrón RIF venezolano: J-12345678-9 o V-12345678-9 o G-12345678-9
	// Buscar patrones tipo "J-12345678-9" en el texto
	for i := 0; i < len(text)-15; i++ {
		segment := text[i : i+16]
		if len(segment) >= 9 {
			first := string(segment[0])
			// RIF venezolano empieza con J, G, V, E seguidos de -
			if (first == "J" || first == "G" || first == "V" || first == "E") &&
				segment[1] == '-' {
				// Extraer candidato RIF (9 caracteres incluyendo el guión)
				candidate := segment[:9]
				// Validar formato básico: letra-guión-8-dígitos
				if len(candidate) == 9 && candidate[1] == '-' {
					// Verificar que los 8 caracteres después del guión sean dígitos
					digits := candidate[2:10]
					digitCount := 0
					for _, ch := range digits {
						if ch >= '0' && ch <= '9' {
							digitCount++
						}
					}
					if digitCount == 8 {
						return strings.TrimSpace(candidate)
					}
				}
			}
		}
	}
	return ""
}

// extractTotal intenta encontrar un total en el texto OCR
func extractTotal(text string) string {
	// Buscar patrones tipo "Total: 1234.56" o "G.Total: 1234.56"
	// Buscar la palabra "total" (case-insensitive) en el texto
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "total")
	if idx >= 0 && idx < len(text)-30 {
		// Retornar el segmento que contiene "total" y algunos caracteres después
		segment := text[idx : idx+30]
		// Buscar un número con decimal en ese segmento
		if strings.Contains(segment, ".") {
			return strings.TrimSpace(segment)
		}
	}
	return ""
}

// extractDate intenta encontrar una fecha en formato dd/mm/aaaa o similar
func extractDate(text string) string {
	// Patrones de fecha dd/mm/yyyy o dd-mm-yyyy o yyyy-mm-dd
	// Buscar patrones de fecha en el texto
	patterns := []string{
		`\d{2}[/-]\d{2}[/-]\d{4}`,    // dd/mm/yyyy o dd-mm-yyyy
		`\d{4}[/-]\d{2}[/-]\d{2}`,    // yyyy-mm-dd
	}
	for _, pat := range patterns {
		// Búsqueda simple: si el patrón aparece en el texto
		if strings.Contains(text, pat) || strings.ToLower(text) == strings.ToLower(pat) {
			// Retornar lo que parezca una fecha
			for _, line := range strings.Split(text, "\n") {
				if strings.Contains(line, pat) || strings.ToLower(line) == strings.ToLower(pat) {
					return strings.TrimSpace(line)
				}
			}
		}
	}
	return ""
}

// ProcessImageOCR procesa una imagen/subida y retorna OCRResult
// Este es el corazón del addon - usa gosseract (Tesseract Go bindings)
func (p *OCRProcessor) ProcessImageOCR(r *multipart.Form) *OCRResult {
	// Si OCR está desactivado, retorna resultado vacío pero "permitido"
	if !p.config.Enabled {
		return &OCRResult{
			Status: "disabled",
			Text:   "OCR desactivado por administrador - activate in settings",
		}
	}

	// Buscar el archivo subido en el formulario
	var fileHeader *multipart.FileHeader
	var fileData io.ReadCloser

	// Gosseract espera io.Reader, buscamos en los campos del formulario
	for key, headers := range r.File {
		if strings.HasPrefix(key, "invoice_image") || strings.HasPrefix(key, "file") {
			if len(headers) > 0 {
				fileHeader = headers[0]
				var err error
				fileData, err = headers[0].Open()
				if err != nil {
					return &OCRResult{
						Status: "error",
						Text:   "No se pudo abrir el archivo subido",
					}
				}
				break
			}
		}
	}

	if fileData == nil {
		return &OCRResult{
			Status: "error",
			Text:   "No se encontró campo de archivo en la petición",
		}
	}
	defer fileData.Close()

	// Inicializar gosseract
	serr := gosseract.New(gosseract.Options{
		Lang: "spa",           // español
		// Opcional: PSM (Page Segmentation Mode)
		// PSM 6 asume una sola página de texto
	})

	// Establecer la imagen desde reader
	if err := serr.SetImage(fileData); err != nil {
		return &OCRResult{
			Status: "error",
			Text:   fmt.Sprintf("Error configurando Tesseract: %v", err),
		}
	}

	// Extraer texto
	text, err := serr.Text()
	if err != nil {
		return &OCRResult{
			Status: "error",
			Text:   fmt.Sprintf("Error extrayendo texto OCR: %v", err),
		}
	}

	// Obtener nivel de confianza
	confidence := serr.Confidence()

	// Detectar RIF en el texto extraído
	rif := extractRIF(text)

	// Detectar total
	total := extractTotal(text)

	// Detectar fecha
	date := extractDate(text)

	// Determinar nombre del proveedor (búsqueda simple)
	supplier := ""
	if strings.Contains(strings.ToLower(text), "rfc") ||
		strings.Contains(strings.ToLower(text), "proveedor") {
		// Podría tener nombre cerca
	}

	// Formatear resultado
	result := &OCRResult{
		Text:         text,
		Confidence:   confidence,
		RIFFound:     rif,
		TotalFound:   total,
		DateFound:    date,
		SupplierName: supplier,
		Status:       "ok",
		RawFormat:    detectFormat(fileHeader.Filename),
	}

	// Ajustar status si confianza es baja
	if confidence > 0 && confidence < 30 {
		result.Status = "low_confidence"
	} else if confidence < 0 {
		result.Status = "error"
	}

	return result
}

// RegisterOCRRoutes registra las rutas HTTP para OCR
// El superusuario debe haber activado ENABLE_OCR=1 en settings
func RegisterOCRRoutes(s *Server) {
	// Ruta para procesar OCR - POST con multipart form
	s.mux.HandleFunc("/ocr/process", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Verificar que OCR esté activado (solo superusuario bypass en desarrollo)
		// En producción real, revisar token/session de superusuario
		// Por ahora, el validador fiscal y el config.Enabled protegen

		r.ParseMultipartForm(32 << 20)

		// Si está desactivado, retornar error controlado
		if !s.config.OCREnabled {
			jsonError(w, "OCR desactivado: configure ENABLE_OCR=1 en settings", 403)
			return
		}

		processor := NewOCRProcessor(&s.config.OCRConfig)
		result := processor.ProcessImageOCR(r.Form)

		writeJSON(w, map[string]interface{}{
			"success": result.Status != "error" && result.Status != "disabled",
			"result":  result,
			"message": fmt.Sprintf("OCR completado: estado=%s, confianza=%.1f%%", 
				result.Status, result.Confidence),
		})
	})

	// Ruta para verificar estado OCR
	s.mux.HandleFunc("/ocr/status", func(w http.ResponseWriter, r *http.Request) {
		enabled := false
		if s.config != nil {
			enabled = s.config.OCREnabled
		}
		writeJSON(w, map[string]interface{}{
			"enabled": enabled,
			"message": fmt.Sprintf("OCR %s", map[bool]string{"true": "activado", "false": "desactivado"}[enabled]),
		})
	})

	// Ruta de prueba con muestra integrada
	s.mux.HandleFunc("/ocr/test", func(w http.ResponseWriter, r *http.Request) {
		// Retornar un resultado de prueba basado en muestra
		// (en producción real probaría con una imagen de prueba)
		result := &OCRResult{
			Text:         "FACTURA ELECTRÓNICA\nRif: J-12345678-9\nFecha: 15/01/2025\nTotal: Bs. 150.00\nProveedor: Empresa X C.A.",
			Confidence:   85.5,
			RIFFound:     "J-12345678-9",
			TotalFound:   "Bs. 150.00",
			DateFound:    "15/01/2025",
			SupplierName: "Empresa X C.A.",
			Status:       "ok",
			RawFormat:    "test",
		}
		writeJSON(w, map[string]interface{}{
			"success": true,
			"result":  result,
			"message": "Muestra de OCR exitendo (modo prueba)",
		})
	})
}