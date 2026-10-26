package seniat

// consulta.go — Consulta del contribuyente (RIF o cédula) en la web del SENIAT.
//
// Es la versión Go, literal, de consultar_rif.py: abre la página BuscaRif.jsp,
// descarga el captcha EN LA MISMA SESIÓN (cookies), pasa el captcha por un OCR,
// hace el POST y parsea la respuesta HTML. Reintenta hasta 3 veces cuando el
// OCR falla (el captcha del SENIAT es caprichoso).
//
// El ÚNICO paso que no puede ir dentro del binario es el OCR del captcha: la
// lectura de esa imagen es un modelo entrenado (ddddocr en Python) y meter un
// modelo ML en el binario del ERP lo haría pesado y frágil. Por eso la función
// recibe un OcrFunc: por defecto la capa web usa un comando externo (python3 +
// ddddocr, o el que el admin configure). Si el OCR no está instalado, la
// consulta devuelve error "conocido": la pantalla solo avisa, nunca bloquea
// la venta — se sigue creando el cliente a mano.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const BaseURL = "https://contribuyente.seniat.gob.ve/BuscaRif/"

// DefaultOcrCmd es el comando de OCR por defecto: ddddocr leyendo el captcha
// desde stdin y escribiendo el código leído en stdout. Igual que hace
// consultar_rif.py. Si ddddocr no está instalado, el admin puede configurar
// el suyo (fiscal_setting `seniat_ocr_cmd`).
const DefaultOcrCmd = `python3 -c "import sys,ddddocr;o=ddddocr.DdddOcr(show_ad=False);print(o.classification(sys.stdin.buffer.read()))"`

var consultHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	"Accept-Language": "es-ES,es;q=0.9",
	"Referer":         BaseURL + "BuscaRif.jsp",
}

// MaxIntentos es el número de intentos si el captcha sale mal.
const MaxIntentos = 3

// Resultado es lo que la web del SENIAT dice del contribuyente.
type Resultado struct {
	Rif         string
	RazonSocial string
	Actividad   string
	Condicion   string
}

// OcrFunc lee un captcha JPG y devuelve el código. La capa web lo llena con un
// comando externo; así el binario no lleva un modelo dentro.
type OcrFunc func(captchaJPG []byte) (string, error)

// OCRPorComando llena un OcrFunc a partir de un comando shell: el captcha
// entra por stdin y el código sale en la primera línea de stdout.
func OCRPorComando(cmd string) OcrFunc {
	return func(jpg []byte) (string, error) {
		c := cmd
		if c == "" {
			c = DefaultOcrCmd
		}
		ocmd := exec.Command("sh", "-c", c)
		ocmd.Stdin = bytes.NewReader(jpg)
		ocmd.Stderr = io.Discard
		var out bytes.Buffer
		if err := ocmd.Run(); err != nil {
			return "", fmt.Errorf("OCR del captcha falló: %w", err)
		}
		codigo := strings.TrimSpace(out.String())
		if codigo == "" {
			return "", fmt.Errorf("el OCR no devolvió código")
		}
		return codigo, nil
	}
}

// Consultar consulta un RIF (ej. J294570968) o una cédula (ej. 14989834) en el
// SENIAT. El valor debe venir limpio (letra+dígitos, sin formatos). Si camina
// todo sale el Resultado con los datos reales del contribuyente.
func Consultar(valor string, esCedula bool, ocr OcrFunc, timeout time.Duration) (*Resultado, error) {
	if ocr == nil {
		ocr = OCRPorComando("")
	}
	campo := "p_rif"
	if esCedula {
		campo = "p_cedula"
	}

	var ultimo error
	for intento := 1; intento <= MaxIntentos; intento++ {
		res, err := consultarUnaVez(valor, campo, ocr, timeout)
		if err != nil {
			// Captcha incorrecto: reintentar. Cualquier otra falla tiene
			// sentido reportarla de una (conexión, ubicación, OCR roto).
			if strings.Contains(err.Error(), "captcha") {
				ultimo = err
				continue
			}
			return nil, err
		}
		return res, nil
	}
	if ultimo != nil {
		return nil, fmt.Errorf("no se resolvió el captcha tras %d intentos: %w", MaxIntentos, ultimo)
	}
	return nil, fmt.Errorf("no se resolvió el captcha tras %d intentos", MaxIntentos)
}

// consultarUnaVez es UNA pasada del flujo sesion→captcha→post→parseo.
func consultarUnaVez(valor, campo string, ocr OcrFunc, timeout time.Duration) (*Resultado, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: timeout,
		Jar:     jar,
	}

	// 1. Cargar la página (levanta cookies de sesión).
	req, err := http.NewRequest("GET", BaseURL+"BuscaRif.jsp", nil)
	if err != nil {
		return nil, err
	}
	for k, v := range consultHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la web del SENIAT: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("la web del SENIAT respondió %d", resp.StatusCode)
	}

	// 2. Descargar el captcha EN LA MISMA SESIÓN (cookies compartidas).
	req, err = http.NewRequest("GET", BaseURL+"Captcha.jpg", nil)
	if err != nil {
		return nil, err
	}
	for k, v := range consultHeaders {
		req.Header.Set(k, v)
	}
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo descargar el captcha: %w", err)
	}
	jpg, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || len(jpg) == 0 {
		return nil, fmt.Errorf("captcha vacío: %w", err)
	}

	// 3. OCR del captcha.
	codigo, err := ocr(jpg)
	if err != nil {
		return nil, err
	}

	// 4. POST del formulario.
	form := fmt.Sprintf("%s=%s&codigo=%s", campo, valor, codigo)
	req, err = http.NewRequest("POST", BaseURL+"BuscaRif.jsp", strings.NewReader(form))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range consultHeaders {
		req.Header.Set(k, v)
	}
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo enviar la consulta: %w", err)
	}
	defer resp.Body.Close()
	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("respuesta ilegible del SENIAT: %w", err)
	}

	// 5. Parsear la respuesta.
	res := ParsearHTML(string(html))
	if res == nil {
		return nil, fmt.Errorf("captcha incorrecto (respuesta sin datos)")
	}
	return res, nil
}

// --- Parseo del HTML, sentencia por sentencia desde consultar_rif.py ---

var (
	reCaptchaMalo = regexp.MustCompile(`(?i)c[oó]digo no coincide`)
	reFont2       = regexp.MustCompile(`(?is)<font[^>]*size="2"[^>]*>(.*?)</font>`)
	reTag         = regexp.MustCompile(`<[^>]+>`)
	reEspacios    = regexp.MustCompile(`\s+`)
	// "NOMBRE (NOMBRE)" duplicado que imprime el SENIAT.
	reDuplicado = regexp.MustCompile(`^(.+?)\s+\((.+)\)$`)
	reActividad = regexp.MustCompile(`(?is)Actividad Econ[oó]mica:\s*(.*?)(?:<br>|Condici[oó]n)`)
	reCondicion = regexp.MustCompile(`(?is)Condici[oó]n:\s*(.*?)(?:<br>|La condici[oó]n)`)
)

// limpiarHTML quita etiquetas, entes y espacios en exceso.
func limpiarHTML(s string) string {
	s = reTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return reEspacios.ReplaceAllString(strings.TrimSpace(s), " ")
}

// ParsearHTML extrae campos del HTML de respuesta del SENIAT. Devuelve nil si
// el captcha falló (el llamador reintenta).
func ParsearHTML(html string) *Resultado {
	if reCaptchaMalo.MatchString(html) {
		return nil
	}

	res := &Resultado{}

	if m := reFont2.FindStringSubmatch(html); m != nil {
		raw := limpiarHTML(m[1])
		if raw != "" {
			parts := strings.SplitN(raw, " ", 2)
			res.Rif = parts[0]
			if len(parts) > 1 {
				nombre := strings.TrimSpace(parts[1])
				if p := reDuplicado.FindStringSubmatch(nombre); p != nil && strings.EqualFold(strings.TrimSpace(p[1]), strings.TrimSpace(p[2])) {
					nombre = strings.TrimSpace(p[1])
				}
				res.RazonSocial = nombre
			}
		}
	}

	if m := reActividad.FindStringSubmatch(html); m != nil {
		res.Actividad = limpiarHTML(m[1])
	}
	if m := reCondicion.FindStringSubmatch(html); m != nil {
		res.Condicion = limpiarHTML(m[1])
	}

	if res.RazonSocial == "" {
		// Sin razón social y sin error de captcha: el contribuyente no existe
		// en la base del SENIAT (o la web no respondió con datos).
		return &Resultado{}
	}
	return res
}
