// Package cgimprenta implementa el proveedor CG Imprenta Digital para ERPGo.
//
// QUÉ ES. CG Imprenta Digital es una casa de facturación digital autorizada por
// el SENIAT con su propia API (documentada en su doc-site). El ERP puede emitir
// por ella igual que por Unidigital; la empresa elige cuál tiene activa y solo
// puede haber una (ver interfaces/web/digital_invoice_settings.go).
//
// QUÉ CAMBIA RESPECTO DE UNIDIGITAL, y por qué este paquete tiene su propio
// cliente HTTP:
//
//   - La credencial no es un Bearer fijo: CG entrega usuario y contraseña de
//     API y un endpoint que devuelve un JWT que VENCE A LA HORA. El token se
//     cachea por credencial y se renueva solo al vencer o cuando la API
//     responde 401. Pedir uno por cada factura sería un viaje extra por
//     documento y gastaría la cuota del negocio.
//   - Emite solo en bolívares y exige la tasa del día con su fecha en cada
//     factura. Las dos cosas se validan ANTES de llamar: un documento emitido
//     con la tasa equivocada no se corrige con un reintento.
//   - El número de control trae serie ("00-00000056"), no un entero. Por eso
//     ResultadoEmision lleva NroControlTexto además de NroControl.
package cgimprenta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Rutas del API de CG. Van todas juntas porque el contrato con el proveedor se
// lee de un vistazo así; repartirlas por el archivo esconde un cambio de
// versión del API detrás de cinco ediciones.
const (
	rutaToken         = "/api/Invoice/create_token_authenticator"
	rutaFacturas      = "/api/Invoice/add_list_invoice"
	rutaNotaCredito   = "/api/Invoice/add_credit_note"
	rutaNotaDebito    = "/api/Invoice/add_debit_note"
	rutaListaFacturas = "/api/Invoice/get_list_invoices"
	rutaMetodosDePago = "/api/Invoice/payments_methods"
)

// vidaToken es lo que CG promete: una hora. Se descuenta un margen para no
// mandar un token que vence mientras viaja la petición.
const (
	vidaToken      = time.Hour
	margenToken    = 5 * time.Minute
	tiempoDeEspera = 30 * time.Second
)

// tokenEnMemoria es un JWT vigente de un negocio. La llave es host + usuario:
// dos empresas pueden usar el mismo usuario de API en hosts distintos y no
// deben compartir token.
type tokenEnMemoria struct {
	valor  string
	expira time.Time
}

var (
	tokensMu            sync.Mutex
	tokensPorCredencial = map[string]tokenEnMemoria{}
)

// cliente es el transporte HTTP de CG. No se exporta: fuera de este paquete
// nadie debe hablar con la imprenta saltándose el mapeo de documentos.
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

// errorAPI es un rechazo del proveedor. Lleva el código HTTP y el mensaje REAL
// que contestó: el runner lo guarda en digital_invoice_log.error_message y es
// lo que el administrador ve en la pantalla cuando un documento no sale.
type errorAPI struct {
	Status  int
	Codigo  string
	Mensaje string
}

func (e *errorAPI) Error() string {
	if e.Codigo != "" {
		return fmt.Sprintf("CG Imprenta Digital (%s): %s", e.Codigo, e.Mensaje)
	}
	return fmt.Sprintf("CG Imprenta Digital (HTTP %d): %s", e.Status, e.Mensaje)
}

// token devuelve un JWT vigente, pidiendo uno nuevo si no hay o si se fuerza
// (tras un 401). El token se comparte entre instancias del proveedor: la cola
// crea un proveedor por emisión y sin esta caché cada factura pediría el suyo.
func (c *cliente) token(ctx context.Context, forzar bool) (string, error) {
	llave := c.baseURL + "|" + c.usuario

	tokensMu.Lock()
	guardado, hay := tokensPorCredencial[llave]
	tokensMu.Unlock()

	if !forzar && hay && time.Now().Before(guardado.expira) {
		return guardado.valor, nil
	}

	if c.usuario == "" || c.clave == "" {
		return "", fmt.Errorf("faltan el usuario y la contraseña de API de CG Imprenta Digital: configúrelos en Facturación Digital")
	}

	cuerpo, _ := json.Marshal(map[string]string{
		"userName":     c.usuario,
		"userPassword": c.clave,
	})
	status, respuesta, err := c.enviar(ctx, http.MethodPost, rutaToken, cuerpo, "")
	if err != nil {
		return "", err
	}
	if status < 200 || status > 299 {
		return "", errorDeRespuesta(status, respuesta)
	}

	var datos struct {
		Token   string `json:"token"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respuesta, &datos); err != nil {
		return "", fmt.Errorf("la respuesta del token de CG no es JSON válido (HTTP %d): %w", status, err)
	}
	if datos.Token == "" {
		mensaje := datos.Message
		if mensaje == "" {
			mensaje = "la API no devolvió token"
		}
		return "", &errorAPI{Status: status, Mensaje: mensaje}
	}

	tokensMu.Lock()
	tokensPorCredencial[llave] = tokenEnMemoria{valor: datos.Token, expira: time.Now().Add(vidaToken - margenToken)}
	tokensMu.Unlock()
	return datos.Token, nil
}

// olvidarToken borra el token cacheado de estas credenciales. Se usa cuando la
// API contesta 401: el token guardado ya no sirve y reintentarlo no arregla nada.
func (c *cliente) olvidarToken() {
	tokensMu.Lock()
	delete(tokensPorCredencial, c.baseURL+"|"+c.usuario)
	tokensMu.Unlock()
}

// llamar ejecuta una petición autenticada y devuelve el cuerpo JSON.
//
// Si la primera respuesta es 401 se descarta el token, se pide otro y se
// reintenta UNA vez. Más reintentos esconderían un problema de credenciales
// detrás de una espera; ninguno dejaría al ERP caído cada vez que vence la hora.
func (c *cliente) llamar(ctx context.Context, metodo, ruta string, cuerpo interface{}) ([]byte, error) {
	var crudo []byte
	if cuerpo != nil {
		var err error
		crudo, err = json.Marshal(cuerpo)
		if err != nil {
			return nil, fmt.Errorf("no se pudo preparar la petición a CG: %w", err)
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
	return nil, &errorAPI{Status: http.StatusUnauthorized, Mensaje: "la API rechazó el token dos veces seguidas: revise el usuario y la contraseña de API"}
}

// get es llamar para las consultas.
func (c *cliente) get(ctx context.Context, ruta string) ([]byte, error) {
	return c.llamar(ctx, http.MethodGet, ruta, nil)
}

// post es llamar para las emisiones.
func (c *cliente) post(ctx context.Context, ruta string, cuerpo interface{}) ([]byte, error) {
	return c.llamar(ctx, http.MethodPost, ruta, cuerpo)
}

// enviar hace una sola petición. token vacío significa "sin Authorization",
// que es solo el caso del endpoint de autenticación.
func (c *cliente) enviar(ctx context.Context, metodo, ruta string, crudo []byte, token string) (int, []byte, error) {
	var lector io.Reader
	if crudo != nil {
		lector = bytes.NewReader(crudo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.baseURL+ruta, lector)
	if err != nil {
		return 0, nil, fmt.Errorf("no se pudo armar la petición a CG: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("no hubo respuesta de CG Imprenta Digital (%s): %w", hostDe(c.baseURL), err)
	}
	defer resp.Body.Close()

	respuesta, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("la respuesta de CG no se pudo leer: %w", err)
	}
	return resp.StatusCode, respuesta, nil
}

// errorDeRespuesta traduce un rechazo de CG a un error legible.
//
// El mensaje del proveedor se copia tal cual cuando viene: es el que dice QUÉ
// campo está mal ("El campo Correo del Cliente en la factura 000123 es
// obligatorio"). Inventar un texto propio aquí le quitaría al administrador la
// única pista de por qué su factura no salió.
func errorDeRespuesta(status int, cuerpo []byte) error {
	var datos struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal(cuerpo, &datos)

	mensaje := strings.TrimSpace(datos.Message)
	if mensaje == "" {
		texto := strings.TrimSpace(string(cuerpo))
		if len(texto) > 300 {
			texto = texto[:300] + "…"
		}
		if texto == "" {
			texto = "el proveedor no explicó el motivo"
		}
		mensaje = texto
	}

	// La suscripción impaga es el único 403 con código propio; se nombra como
	// lo que es para que nadie lo confunda con una credencial mal puesta.
	if status == http.StatusForbidden && strings.EqualFold(datos.Code, "SUSCRIPCION") {
		return &errorAPI{Status: status, Codigo: "SUSCRIPCION", Mensaje: mensaje}
	}
	return &errorAPI{Status: status, Mensaje: mensaje}
}

// hostDe deja el host de una URL para los mensajes de error, sin credenciales
// ni rutas.
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
