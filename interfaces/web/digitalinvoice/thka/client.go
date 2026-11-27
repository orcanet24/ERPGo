// Package thka implementa el proveedor The Factory HKA (TFHKA) para ERPGo.
//
// QUÉ ES. The Factory HKA Venezuela es otra casa de facturación digital
// autorizada por el SENIAT. Su API (RIVA v2) es MUY distinta de las otras dos
// integradas: un único documento electrónico anidado que describe cabecera,
// comprador, totales e ítems, y un número de control que la imprenta asigna en
// la misma llamada.
//
// Lo que este paquete tiene de particular, y por qué no comparte cliente HTTP
// con Unidigital ni con CG:
//
//   - Autenticación: POST /api/Autenticacion con usuario y clave devuelve un
//     JWT que dura DOCE HORAS (las otras dos caducan en una o no caducan). El
//     token se cachea por host+usuario y se renueva al vencer o ante un 401.
//   - Respuesta propia: {codigo, mensaje, validaciones[], resultado}. El código
//     "200" es éxito, "201" documento duplicado, "203"/"205" rechazo por
//     validación. El error NO viene solo por el HTTP: hay que leer el cuerpo.
//   - El PDF no viene en la respuesta de emisión. Se pide aparte con
//     /api/DescargaArchivo; aquí se intenta y, si el formato de la respuesta no
//     es el esperado, se sigue: el número de control es lo fiscal, el PDF un
//     extra que el ERP archiva cuando puede.
//   - Moneda: se emite en bolívares. Una factura en divisas se rechaza con un
//     mensaje claro antes de llamar; mandar dólares como si fueran bolívares
//     sería un documento fiscal falso.
package thka

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rutas del API de THKA.
const (
	rutaAutenticacion = "/api/Autenticacion"
	rutaEmision       = "/api/Emision"
	rutaDescarga      = "/api/DescargaArchivo"
)

// vidaToken es lo que promete la referencia (12 horas); se descuenta un margen
// para no mandar un token que vence en mitad de la petición.
const (
	vidaToken      = 12 * time.Hour
	margenToken    = time.Hour
	tiempoDeEspera = 30 * time.Second
	// limiteRespuesta acota lo que se lee de la imprenta. Un PDF en base64 de
	// una factura normal no llega a 1 MB; 24 MB deja aire y evita que una
	// respuesta equivocada llene la memoria del ERP.
	limiteRespuesta = 24 << 20
)

// tokenEnMemoria es un JWT vigente de un negocio.
type tokenEnMemoria struct {
	valor  string
	expira time.Time
}

var (
	tokensMu            sync.Mutex
	tokensPorCredencial = map[string]tokenEnMemoria{}
)

// cliente es el transporte HTTP de THKA.
type cliente struct {
	baseURL string
	usuario string
	clave   string
	http    *http.Client
}

func nuevoCliente(baseURL, usuario, clave string) *cliente {
	return &cliente{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		usuario: strings.TrimSpace(usuario),
		clave:   strings.TrimSpace(clave),
		http:    &http.Client{Timeout: tiempoDeEspera},
	}
}

// respuestaTHKA es el sobre que devuelve el API en todas sus operaciones.
//
// `codigo` y `numeroControl` llegan como texto en la documentación, pero un
// servidor JSON los puede devolver como número; textoFlexible acepta los dos y
// evita que una respuesta válida se lea como error de decodificación.
type respuestaTHKA struct {
	Codigo       textoFlexible   `json:"codigo"`
	Mensaje      string          `json:"mensaje"`
	Validaciones json.RawMessage `json:"validaciones"`
	Resultado    struct {
		ImprentaDigital string        `json:"imprentaDigital"`
		Autorizado      textoFlexible `json:"autorizado"`
		Serie           string        `json:"serie"`
		TipoDocumento   textoFlexible `json:"tipoDocumento"`
		NumeroDocumento textoFlexible `json:"numeroDocumento"`
		NumeroControl   textoFlexible `json:"numeroControl"`
		FechaAsignacion string        `json:"fechaAsignacion"`
		HoraAsignacion  string        `json:"horaAsignacion"`
		TransaccionID   textoFlexible `json:"transaccionId"`
		UrlPdf          string        `json:"urlPdf"`
	} `json:"resultado"`
}

// textoFlexible es un string que también acepta números y nulos. El API de THKA
// documenta los códigos como texto, pero una versión que los mande como número
// no debe romper la emisión.
type textoFlexible string

func (t *textoFlexible) UnmarshalJSON(datos []byte) error {
	crudo := strings.TrimSpace(string(datos))
	if crudo == "" || crudo == "null" {
		*t = ""
		return nil
	}
	if crudo[0] == '"' {
		var s string
		if err := json.Unmarshal(datos, &s); err != nil {
			return err
		}
		*t = textoFlexible(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(datos, &n); err == nil {
		*t = textoFlexible(n.String())
		return nil
	}
	*t = textoFlexible(crudo)
	return nil
}

func (t textoFlexible) String() string { return string(t) }

// listaDeValidaciones lee `validaciones` venga como arreglo de textos, como un
// solo texto o como arreglo de objetos con `mensaje`.
func listaDeValidaciones(crudo json.RawMessage) []string {
	if len(crudo) == 0 {
		return nil
	}
	var textos []string
	if err := json.Unmarshal(crudo, &textos); err == nil {
		return textos
	}
	var uno string
	if err := json.Unmarshal(crudo, &uno); err == nil {
		if strings.TrimSpace(uno) != "" {
			return []string{uno}
		}
		return nil
	}
	var objetos []map[string]interface{}
	if err := json.Unmarshal(crudo, &objetos); err == nil {
		salida := make([]string, 0, len(objetos))
		for _, o := range objetos {
			for _, llave := range []string{"mensaje", "Mensaje", "descripcion", "descripción", "error"} {
				if v, ok := o[llave].(string); ok && strings.TrimSpace(v) != "" {
					salida = append(salida, v)
					break
				}
			}
		}
		return salida
	}
	return nil
}

// errorTHKA es un rechazo del proveedor con su código y sus validaciones.
type errorTHKA struct {
	Codigo       string
	Mensaje      string
	Validaciones []string
}

func (e *errorTHKA) Error() string {
	partes := []string{}
	if e.Mensaje != "" {
		partes = append(partes, e.Mensaje)
	}
	if len(e.Validaciones) > 0 {
		partes = append(partes, strings.Join(e.Validaciones, "; "))
	}
	texto := strings.Join(partes, " — ")
	if texto == "" {
		texto = "el proveedor no explicó el motivo"
	}
	if e.Codigo != "" {
		return fmt.Sprintf("The Factory HKA (código %s): %s", e.Codigo, texto)
	}
	return "The Factory HKA: " + texto
}

// token devuelve un JWT vigente, pidiendo uno nuevo si no hay o si se fuerza.
func (c *cliente) token(ctx context.Context, forzar bool) (string, error) {
	llave := c.baseURL + "|" + c.usuario

	tokensMu.Lock()
	guardado, hay := tokensPorCredencial[llave]
	tokensMu.Unlock()

	if !forzar && hay && time.Now().Before(guardado.expira) {
		return guardado.valor, nil
	}

	if c.usuario == "" || c.clave == "" {
		return "", fmt.Errorf("faltan el usuario y la clave de API de The Factory HKA: configúrelos en Facturación Digital")
	}

	cuerpo, _ := json.Marshal(map[string]string{"usuario": c.usuario, "clave": c.clave})
	status, respuesta, err := c.enviar(ctx, http.MethodPost, rutaAutenticacion, cuerpo, "")
	if err != nil {
		return "", err
	}
	if status < 200 || status > 299 {
		return "", errorDeRespuesta(status, respuesta)
	}

	token := extraerToken(respuesta)
	if token == "" {
		return "", fmt.Errorf("la autenticación de The Factory HKA respondió sin token: %.200s", strings.TrimSpace(string(respuesta)))
	}

	tokensMu.Lock()
	tokensPorCredencial[llave] = tokenEnMemoria{valor: token, expira: time.Now().Add(vidaToken - margenToken)}
	tokensMu.Unlock()
	return token, nil
}

// olvidarToken borra el token cacheado de estas credenciales (tras un 401).
func (c *cliente) olvidarToken() {
	tokensMu.Lock()
	delete(tokensPorCredencial, c.baseURL+"|"+c.usuario)
	tokensMu.Unlock()
}

// llamar ejecuta una petición autenticada y devuelve el cuerpo.
//
// Ante un 401 descarta el token, pide otro y reintenta UNA vez: es lo que
// permite que la emisión siga funcionando cuando el JWT vence sin que nadie
// tenga que reiniciar el ERP.
func (c *cliente) llamar(ctx context.Context, metodo, ruta string, cuerpo interface{}) ([]byte, error) {
	var crudo []byte
	if cuerpo != nil {
		var err error
		crudo, err = json.Marshal(cuerpo)
		if err != nil {
			return nil, fmt.Errorf("no se pudo preparar la petición a The Factory HKA: %w", err)
		}
	}

	for intento := 0; intento < 2; intento++ {
		tok, err := c.token(ctx, intento > 0)
		if err != nil {
			return nil, err
		}
		status, respuesta, err := c.enviar(ctx, metodo, ruta, crudo, tok)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized && intento == 0 {
			c.olvidarToken()
			continue
		}
		if status < 200 || status > 299 {
			return nil, errorDeRespuesta(status, respuesta)
		}
		return respuesta, nil
	}
	return nil, fmt.Errorf("The Factory HKA rechazó el token dos veces seguidas: revise el usuario y la clave de API")
}

// enviar hace una sola petición. token vacío = sin Authorization (autenticación).
func (c *cliente) enviar(ctx context.Context, metodo, ruta string, crudo []byte, token string) (int, []byte, error) {
	var lector io.Reader
	if crudo != nil {
		lector = bytes.NewReader(crudo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.baseURL+ruta, lector)
	if err != nil {
		return 0, nil, fmt.Errorf("no se pudo armar la petición a The Factory HKA: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("no hubo respuesta de The Factory HKA (%s): %w", hostDe(c.baseURL), err)
	}
	defer resp.Body.Close()

	respuesta, err := io.ReadAll(io.LimitReader(resp.Body, limiteRespuesta))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("la respuesta de The Factory HKA no se pudo leer: %w", err)
	}
	return resp.StatusCode, respuesta, nil
}

// extraerToken saca el JWT de la respuesta de autenticación.
//
// La referencia no documenta la forma exacta de esa respuesta más allá de "un
// token", así que se aceptan las variantes razonables: un JSON con la clave
// `token` (o jwt/accessToken/resultado), un JSON que sea una cadena, o el JWT
// suelto en el cuerpo. Adivinar de menos dejaría al ERP sin poder emitir por una
// diferencia de envoltorio.
func extraerToken(respuesta []byte) string {
	crudo := strings.TrimSpace(string(respuesta))
	if crudo == "" {
		return ""
	}

	// El cuerpo puede ser directamente el token (con o sin comillas).
	if strings.Count(crudo, ".") == 2 && !strings.ContainsAny(crudo, "{}\n ") {
		return crudo
	}
	var cadena string
	if err := json.Unmarshal(respuesta, &cadena); err == nil {
		return strings.TrimSpace(cadena)
	}

	var datos map[string]interface{}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return ""
	}
	for _, llave := range []string{"token", "Token", "jwt", "JWT", "accessToken", "access_token", "bearer", "Bearer"} {
		if v, ok := datos[llave].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	// Algunas variantes envuelven el token en `resultado`.
	switch v := datos["resultado"].(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		for _, llave := range []string{"token", "Token", "jwt", "accessToken"} {
			if s, ok := v[llave].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// errorDeRespuesta traduce un rechazo a un error legible.
//
// El mensaje se copia del proveedor: `mensaje` dice qué pasó y `validaciones`
// enumera los campos, que es la única pista que el administrador tendrá en la
// pantalla de la cola.
func errorDeRespuesta(status int, cuerpo []byte) error {
	var respuesta respuestaTHKA
	_ = json.Unmarshal(cuerpo, &respuesta)

	validaciones := listaDeValidaciones(respuesta.Validaciones)
	e := &errorTHKA{
		Codigo:       respuesta.Codigo.String(),
		Mensaje:      strings.TrimSpace(respuesta.Mensaje),
		Validaciones: validaciones,
	}
	if e.Codigo == "" {
		e.Codigo = strconv.Itoa(status)
	}
	if e.Mensaje == "" && len(e.Validaciones) == 0 {
		texto := strings.TrimSpace(string(cuerpo))
		if len(texto) > 300 {
			texto = texto[:300] + "…"
		}
		if texto != "" {
			e.Mensaje = texto
		}
	}
	return e
}

// hostDe deja el host de una URL para los mensajes de error.
func hostDe(base string) string {
	s := strings.TrimRight(base, "/")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "el proveedor"
	}
	return s
}
