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
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const BaseURL = "https://contribuyente.seniat.gob.ve/BuscaRif/"

// ocrScript es el programa Python que hace la misma llamada que
// consultar_rif.py: ddddocr leyendo el captcha por stdin y escribiendo el
// código por stdout.
const ocrScript = `import sys,ddddocr;o=ddddocr.DdddOcr(show_ad=False);print(o.classification(sys.stdin.buffer.read()))`

// interpretesOCR son los nombres del intérprete de Python en los tres
// mundos donde puede caer el binario: Linux (python3), instalaciones viejas
// o Windows (python) y Windows con el lanzador de la Microsoft Store (py).
// Es la razón típica de "en Python funciona y en Go no": el comando fijo
// `python3` no existía en el servidor y el error quedaba enterrado.
var interpretesOCR = []string{"python3", "python", "py"}

// DefaultOcrCmd es el comando OCR por defecto (el primero de la lista). Se
// muestra en la pantalla de prueba del admin; en ejecución se prueban todos.
const DefaultOcrCmd = `python3 -c "import sys,ddddocr;o=ddddocr.DdddOcr(show_ad=False);print(o.classification(sys.stdin.buffer.read()))"`

// comandosOCR devuelve los comandos a intentar, en orden. Si el admin
// configuró uno (fiscal_setting seniat_ocr_cmd), solo ese: su
// decisión manda. Si no, la lista de intérpretes con el mismo script.
func comandosOCR(cmd string) []string {
	if c := strings.TrimSpace(cmd); c != "" {
		return []string{c}
	}
	lista := make([]string, 0, len(interpretesOCR))
	for _, interp := range interpretesOCR {
		lista = append(lista, interp+` -c "`+ocrScript+`"`)
	}
	return lista
}

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

// ComandoOCR devuelve el comando OCR efectivo (el configurado o el de
// fábrica). Lo usa la pantalla de prueba para mostrar QUÉ se va a ejecutar.
func ComandoOCR(cmd string) string {
	if strings.TrimSpace(cmd) == "" {
		return DefaultOcrCmd
	}
	return strings.TrimSpace(cmd)
}

// shellPara devuelve el intérprete de comandos de cada sistema. `sh -c` NO
// existe en Windows, y ahí el OCR nunca corría — el ERP se distribuye como .bat
// para Windows, así que este detalle decidía si la consulta funcionaba o no
// según el servidor donde cayera el binario.
func shellPara() (prog string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c"}
	}
	return "sh", []string{"-c"}
}

// OCRPorComando llena un OcrFunc a partir de un comando shell: el captcha
// entra por stdin y el código sale en la primera línea de stdout.
//
// El stderr del comando SE CONSERVA (truncado) en el error: sin él, un
// `python3` ausente o un `ddddocr` no instalado producía exactamente el mismo
// error que un captcha ilegible, y era imposible saber por qué "no funciona".
func OCRPorComando(cmd string) OcrFunc {
	return func(jpg []byte) (string, error) {
		comandos := comandosOCR(cmd)
		var fallos []string
		for _, c := range comandos {
			codigo, err := correrOCR(c, jpg)
			if err == nil {
				return codigo, nil
			}
			nombre := c
			if len(nombre) > 48 {
				nombre = nombre[:48] + "…"
			}
			fallos = append(fallos, nombre+": "+err.Error())
		}
		// Un solo error con TODOS los intentos: clasificarFallo lo reconoce
		// por el prefijo y el admin ve qué intérpretes se probaron.
		return "", fmt.Errorf("OCR del captcha falló: %s", strings.Join(fallos, " | "))
	}
}

// correrOCR ejecuta UN comando OCR: captcha por stdin, código por stdout.
// El stderr SE CONSERVA (truncado) en el error: sin él, un `python3` ausente
// o un `ddddocr` no instalado producía exactamente el mismo error que un
// captcha ilegible, y era imposible saber por qué "no funciona".
func correrOCR(cmd string, jpg []byte) (string, error) {
	prog, pre := shellPara()
	ocmd := exec.Command(prog, append(pre, cmd)...)
	ocmd.Stdin = bytes.NewReader(jpg)
	var errOut bytes.Buffer
	ocmd.Stderr = &errOut
	var out bytes.Buffer
	if err := ocmd.Run(); err != nil {
		detalle := strings.TrimSpace(errOut.String())
		if len(detalle) > 300 {
			detalle = detalle[:300]
		}
		if detalle != "" {
			return "", fmt.Errorf("%v — stderr: %s", err, detalle)
		}
		return "", err
	}
	codigo := strings.TrimSpace(out.String())
	if codigo == "" {
		detalle := strings.TrimSpace(errOut.String())
		if len(detalle) > 300 {
			detalle = detalle[:300]
		}
		if detalle != "" {
			return "", fmt.Errorf("el OCR no devolvió código — stderr: %s", detalle)
		}
		return "", fmt.Errorf("el OCR no devolvió código")
	}
	return codigo, nil
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
	// Codificado como formulario (url.Values), no concatenado a mano: un
	// código de captcha con cualquier caracter fuera de [A-Za-z0-9] rompía
	// el POST y el SENIAT respondía "captcha incorrecto" sin motivo.
	form := url.Values{}
	form.Set(campo, valor)
	form.Set("codigo", codigo)
	req, err = http.NewRequest("POST", BaseURL+"BuscaRif.jsp", strings.NewReader(form.Encode()))
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
