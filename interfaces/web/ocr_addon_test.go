package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo/configs"
)

// Factura simulada del /ocr/test: debe rendir los 4 datos que el panel
// del superusuario revisa antes de procesar un archivo real.
const muestraFacturaOC = "FACTURA ELECTRÓNICA\nPROVEEDOR: Empresa X C.A.\nRIF: J-12345678-9\nFecha: 15/01/2025\nTOTAL Bs. 150,00\n"

func TestExtractRIF(t *testing.T) {
	casos := []struct {
		texto string
		queda string
	}{
		{muestraFacturaOC, "J-12345678-9"},
		{"RIF: J-311555123-2 sin más datos", "J-311555123-2"},
		// Sin guiones ni anteer el guión: el OCR suele perderlos; la letra
		// impresa y los dígitos quedan intactos para la validación humana.
		{"CIF/ J311555123 2 ", "J-311555123-2"},
		{"V-123456789", "V-12345678-9"},
		// Letra ajena al RIF venezolano: no se acepta como RIF.
		{"RIF: A-12345678-9", ""},
		{"documento sin RIF", ""},
	}
	for _, tc := range casos {
		if got := extractRIF(tc.texto); got != tc.queda {
			t.Errorf("extractRIF(%q) = %q, wanted %q", tc.texto, got, tc.queda)
		}
	}
}

func TestExtractTotal(t *testing.T) {
	casos := []struct {
		texto string
		queda string
	}{
		{muestraFacturaOC, "150,00"},
		{"Gran Total: 1234.56 Bs", "1234.56"},
		// El espacio rompe el número (limitación conocida del OCR); se
		// captura lo que es número contiguo y el usuario corrige el total.
		{"TOTAL A PAGAR $ 8 900,00", "8"},
		{"No hay total identificado", ""},
	}
	for _, tc := range casos {
		if got := extractTotal(tc.texto); got != tc.queda {
			t.Errorf("extractTotal(%q) = %q, wanted %q", tc.texto, got, tc.queda)
		}
	}
}

func TestExtractDate(t *testing.T) {
	casos := []struct {
		texto string
		queda string
	}{
		{muestraFacturaOC, "15/01/2025"},
		{"fecha de emisión: 2025-03-08", "2025-03-08"},
		{"ninguna fecha aquí", ""},
	}
	for _, tc := range casos {
		if got := extractDate(tc.texto); got != tc.queda {
			t.Errorf("extractDate(%q) = %q, wanted %q", tc.texto, got, tc.queda)
		}
	}
}

func TestExtractSupplier(t *testing.T) {
	got := extractSupplier(muestraFacturaOC)
	if got != "Empresa X C.A." {
		t.Errorf("extractSupplier(muestra) = %q, wanted Empresa X C.A.", got)
	}
	// Sin dos puntos: se devuelve la línea completa (sin la palabra
	// PROVEEDOR) para que el usuario confirme el nombre sobre el texto
	// original del documento y no sobre una interpretación.
	if got := extractSupplier("Proveedor Empresa Y"); got != "Empresa Y" {
		t.Errorf("extractSupplier(sin dos puntos) = %q, wanted Empresa Y", got)
	}
	// Pero una etiqueta no estructurada NO es proveedor: nada de adivinar.
	if got := extractSupplier("Nombre aleatorio sin contexto"); got != "" {
		t.Errorf("extractSupplier(línea sin etiqueta) = %q, wanted vacio", got)
	}
}

func TestOCRProcessImageFormatoNoSoportado(t *testing.T) {
	res := ocrProcessImage(strings.NewReader("x"), "unsupported", "tesseract", "spa")
	if res.Status == "ok" {
		t.Fatalf("un PDF o XML NO debe procesarse como imagen (status=%s)", res.Status)
	}
	// Con binario inexistente es "unavailable", no "error": el ERP funciona
	// sin Tesseract instalado y esa diferencia la consume el /ocr/status.
	res = ocrProcessImage(strings.NewReader("x"), "image", "binario-inexistente", "spa")
	if res.Status != "unavailable" {
		t.Errorf("binario ausente debe rendir 'unavailable', got %s (%q)", res.Status, res.Text)
	}
}

func TestOcrParseTSV(t *testing.T) {
	// Mínima salida tsv de Tesseract (separada por TAB, como la real): una
	// fila de página (level 1, sin 12 columnas; se descarta) y una palabra
	// fiable (level 5, conf 95) más una fila sin columna de texto.
	tsv := "1\t1\t0\t0\t0\t-1\t0\t0\t0\t0\t-1\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t50\t10\t95.000000\tRIF\n" +
		"5\t1\t1\t1\t2\t1\t10\t20\t50\t10\t-1\n"
	texto, conf := ocrParseTSV(strings.NewReader(tsv))
	if texto != "RIF" {
		t.Errorf("ocrParseTSV text = %q, wanted RIF", texto)
	}
	if conf != 95.0 {
		t.Errorf("ocrParseTSV conf = %.1f, wanted 95.0", conf)
	}
}

func TestOCRRouteDesactivadoPorDefecto(t *testing.T) {
	// El ERP nuevo arranca con OCR desactivado y la ruta 403 debe ser
	// controlada: no puede servir texto de error Go ni dejar sin respuesta.
	s := &Server{config: &configs.Config{}}
	s.mux = http.NewServeMux()
	s.registerOCRRoutes()

	serv := httptest.NewServer(s.mux)
	defer serv.Close()

	resp, err := http.Post(serv.URL+"/ocr/process", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("POST /ocr/process falló: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("OCR desactivado debe responder 403, got %d", resp.StatusCode)
	}

	resp, err = http.Get(serv.URL + "/ocr/status")
	if err != nil {
		t.Fatalf("GET /ocr/status falló: %v", err)
	}
	defer resp.Body.Close()
	var estado struct {
		Enabled bool `json:"enabled"`
	}
	body, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(body, &estado) == nil && estado.Enabled {
		t.Errorf("el estado debe decir enabled=false cuando ENABLE_OCR no está")
	}
}

func TestOCRRouteStatusTestActivado(t *testing.T) {
	// Con ENABLE_OCR=1 el health-check del extractor debe salir completo.
	s := &Server{config: &configs.Config{OCREnabled: true}}
	s.mux = http.NewServeMux()
	s.registerOCRRoutes()

	serv := httptest.NewServer(s.mux)
	defer serv.Close()

	resp, err := http.Get(serv.URL + "/ocr/test")
	if err != nil {
		t.Fatalf("GET /ocr/test falló: %v", err)
	}
	defer resp.Body.Close()
	var cuerpo struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("respuesta /ocr/test no es JSON válido: %v", err)
	}
	if !cuerpo.Success {
		t.Errorf("el /ocr/test debe salir bien con la muestra completa (message=%s)", cuerpo.Message)
	}
}

func TestOCRProcessMultipart(t *testing.T) {
	// Formulario sin archivo: 400 controlado, no un panic del servidor.
	s := &Server{config: &configs.Config{OCREnabled: true}}
	s.mux = http.NewServeMux()
	s.registerOCRRoutes()

	serv := httptest.NewServer(s.mux)
	defer serv.Close()

	var buff bytes.Buffer
	escritor := multipart.NewWriter(&buff)
	_ = escritor.WriteField("otro", "campo")
	_ = escritor.Close()

	resp, err := http.Post(serv.URL+"/ocr/process", escritor.FormDataContentType(), &buff)
	if err != nil {
		t.Fatalf("POST sin archivo falló: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("multipart sin archivo debe responder 400, got %d", resp.StatusCode)
	}
}
