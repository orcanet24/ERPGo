// Package gowa es el cliente HTTP del daemon GOWA (WhatsApp Web multi-dispositivo).
//
// POR QUÉ UN CLIENTE Y NO LLAMADAS SUELTAS. El daemon es un proceso aparte que
// puede estar caído, tardar o responder con un error de WhatsApp. Si cada
// pantalla armara su propio `http.Post`, cada una decidiría distinto qué es un
// fallo temporal, cuánto esperar y cómo se lee la respuesta. Aquí hay una sola
// respuesta para esas tres preguntas.
//
// NADA DE ESTE PAQUETE CONOCE LA BASE DE DATOS NI LA SESIÓN DEL USUARIO. Solo
// habla HTTP y devuelve errores tipados. Eso es lo que permite probarlo contra
// un servidor falso, sin WhatsApp y sin red.
package gowa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cliente habla con un daemon GOWA por loopback.
type Cliente struct {
	BaseURL  string
	Usuario  string
	Password string
	HTTP     *http.Client
}

// Nuevo crea el cliente. La URL debe ser la base (http://127.0.0.1:3000) y las
// credenciales las que se le pasaron al daemon en APP_BASIC_AUTH.
func Nuevo(baseURL, usuario, password string) *Cliente {
	return &Cliente{
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Usuario:  usuario,
		Password: password,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
	}
}

// Error es un fallo del daemon, ya clasificado.
//
// Temporal distingue «vuelva a intentarlo» de «esto no va a funcionar solo»: un
// 500, un 429 o un corte de red se reintentan; un 401 (credenciales malas) o un
// 404 (dispositivo inexistente) no, y reintentarlos solo gasta cupo y tiempo.
type Error struct {
	Status   int
	Codigo   int
	Mensaje  string
	Temporal bool
}

func (e *Error) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("GOWA respondió %d: %s", e.Status, e.Mensaje)
	}
	return "GOWA: " + e.Mensaje
}

// EsTemporal dice si el error admite reintento. Un error de red (Status 0)
// también es temporal: el daemon puede estar reiniciándose.
func (e *Error) EsTemporal() bool { return e.Temporal }

// sobre es la forma común de las respuestas de GOWA. Se toleran las dos
// variantes (`results` y `data`) porque el proyecto original las usa según el
// endpoint, y una integración no debería romperse por eso.
type sobre struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Results json.RawMessage `json:"results"`
	Data    json.RawMessage `json:"data"`
}

// Salud comprueba que el daemon está vivo. No exige autenticación: /health es
// público en GOWA, y así el chequeo funciona incluso si la credencial está mal.
func (c *Cliente) Salud(ctx context.Context) error {
	return c.hacer(ctx, http.MethodGet, "/health", "", nil, nil, false)
}

// Version devuelve la versión del daemon, que se compara con la que el ERP
// espera. No se impide arrancar por una diferencia: se avisa.
func (c *Cliente) Version(ctx context.Context) (string, error) {
	var m map[string]interface{}
	if err := c.hacer(ctx, http.MethodGet, "/app/info", "", nil, &m, true); err != nil {
		return "", err
	}
	return textoDe(m, "version", "app_version", "gowa_version"), nil
}

// Dispositivo es una sesión de WhatsApp dentro del daemon.
type Dispositivo struct {
	ID        string `json:"device_id"`
	Estado    string `json:"state"`
	JID       string `json:"jid"`
	MSISDN    string `json:"msisdn"`
	Nombre    string `json:"display_name"`
	Conectado bool   `json:"connected"`
}

// Dispositivos lista las sesiones del daemon.
func (c *Cliente) Dispositivos(ctx context.Context) ([]Dispositivo, error) {
	var lista []Dispositivo
	if err := c.hacer(ctx, http.MethodGet, "/devices", "", nil, &lista, true); err != nil {
		return nil, err
	}
	return lista, nil
}

// CrearDispositivo da de alta una sesión con su webhook.
func (c *Cliente) CrearDispositivo(ctx context.Context, deviceID, webhookURL, secreto string, eventos []string) error {
	cuerpo := map[string]interface{}{
		"device_id":      deviceID,
		"webhook_url":    webhookURL,
		"webhook_secret": secreto,
		"webhook_events": eventos,
	}
	return c.hacer(ctx, http.MethodPost, "/devices", "", cuerpo, nil, true)
}

// Estado devuelve el estado de una sesión.
func (c *Cliente) Estado(ctx context.Context, deviceID string) (Dispositivo, error) {
	var d Dispositivo
	err := c.hacer(ctx, http.MethodGet, "/devices/"+url.PathEscape(deviceID)+"/status", deviceID, nil, &d, true)
	return d, err
}

// QR es lo que devuelve el alta por código QR.
type QR struct {
	Enlace   string `json:"qr_link"`
	Segundos int    `json:"qr_duration"`
}

// Login pide el QR de vinculación. El enlace es del daemon: el ERP lo descarga
// y lo sirve por una ruta autenticada, nunca expone el daemon al navegador.
func (c *Cliente) Login(ctx context.Context, deviceID string) (QR, error) {
	var m map[string]interface{}
	if err := c.hacer(ctx, http.MethodGet, "/devices/"+url.PathEscape(deviceID)+"/login", deviceID, nil, &m, true); err != nil {
		return QR{}, err
	}
	q := QR{Enlace: textoDe(m, "qr_link", "qr", "link", "image")}
	if n, ok := numeroDe(m, "qr_duration", "duration", "expires_in"); ok {
		q.Segundos = n
	}
	return q, nil
}

// LoginCodigo pide vincular por código en vez de QR.
func (c *Cliente) LoginCodigo(ctx context.Context, deviceID, telefono string) (string, error) {
	ruta := "/devices/" + url.PathEscape(deviceID) + "/login/code?phone=" + url.QueryEscape(telefono)
	var m map[string]interface{}
	if err := c.hacer(ctx, http.MethodPost, ruta, deviceID, nil, &m, true); err != nil {
		return "", err
	}
	return textoDe(m, "pair_code", "code"), nil
}

func (c *Cliente) Reconectar(ctx context.Context, deviceID string) error {
	return c.hacer(ctx, http.MethodPost, "/devices/"+url.PathEscape(deviceID)+"/reconnect", deviceID, nil, nil, true)
}

func (c *Cliente) CerrarSesion(ctx context.Context, deviceID string) error {
	return c.hacer(ctx, http.MethodPost, "/devices/"+url.PathEscape(deviceID)+"/logout", deviceID, nil, nil, true)
}

func (c *Cliente) Borrar(ctx context.Context, deviceID string) error {
	return c.hacer(ctx, http.MethodDelete, "/devices/"+url.PathEscape(deviceID), deviceID, nil, nil, true)
}

// Webhook configura la entrega de eventos de una sesión.
func (c *Cliente) Webhook(ctx context.Context, deviceID, webhookURL, secreto string, eventos []string) error {
	cuerpo := map[string]interface{}{
		"webhook_url":    webhookURL,
		"webhook_secret": secreto,
		"webhook_events": eventos,
	}
	return c.hacer(ctx, http.MethodPatch, "/devices/"+url.PathEscape(deviceID)+"/webhook", deviceID, cuerpo, nil, true)
}

// Envio es el acuse inmediato del daemon. NO significa entregado: la entrega
// real llega después por webhook (message.ack). La UI tiene que decirlo así.
type Envio struct {
	MensajeID string
	Estado    string
}

func (c *Cliente) EnviarTexto(ctx context.Context, deviceID, telefono, mensaje string) (Envio, error) {
	cuerpo := map[string]interface{}{"phone": telefono, "message": mensaje}
	var m map[string]interface{}
	if err := c.hacer(ctx, http.MethodPost, "/send/message", deviceID, cuerpo, &m, true); err != nil {
		return Envio{}, err
	}
	return Envio{
		MensajeID: textoDe(m, "message_id", "id", "msg_id"),
		Estado:    textoDe(m, "status", "state"),
	}, nil
}

// Presencia envía «escribiendo» (start) o lo detiene (stop). Es lo que hace que
// el envío se parezca a una persona y no a un robot.
func (c *Cliente) Presencia(ctx context.Context, deviceID, telefono, accion string) error {
	accion = strings.ToLower(strings.TrimSpace(accion))
	if accion != "start" && accion != "stop" {
		return &Error{Mensaje: "acción de presencia inválida: " + accion}
	}
	cuerpo := map[string]interface{}{"phone": telefono, "action": accion}
	return c.hacer(ctx, http.MethodPost, "/send/chat-presence", deviceID, cuerpo, nil, true)
}

// VerificarNumero pregunta si el teléfono tiene WhatsApp. Es una consulta
// manual: cada llamada es tráfico y no se hace al guardar una ficha.
//
// La respuesta se interpreta con varias claves porque el endpoint no siempre
// devuelve la misma forma. Si no se reconoce ninguna, se devuelve false y un
// error: «no sé» no puede convertirse en «no tiene WhatsApp».
func (c *Cliente) VerificarNumero(ctx context.Context, deviceID, telefono string) (bool, error) {
	ruta := "/user/check?phone=" + url.QueryEscape(telefono)
	var m map[string]interface{}
	if err := c.hacer(ctx, http.MethodGet, ruta, deviceID, nil, &m, true); err != nil {
		return false, err
	}
	for _, clave := range []string{"is_registered", "registered", "exists", "on_whatsapp"} {
		if v, ok := m[clave]; ok {
			if b, ok := v.(bool); ok {
				return b, nil
			}
		}
	}
	return false, &Error{Mensaje: "el daemon no dijo si el número está registrado"}
}

// ---------- Media ----------

// EnviarImagen manda una imagen con un pie opcional.
func (c *Cliente) EnviarImagen(ctx context.Context, deviceID, telefono, archivo, pie string) (Envio, error) {
	return c.enviarMedia(ctx, "/send/image", "image", deviceID, telefono, archivo, pie)
}

// EnviarAudio manda una nota de voz o un audio.
func (c *Cliente) EnviarAudio(ctx context.Context, deviceID, telefono, archivo, pie string) (Envio, error) {
	return c.enviarMedia(ctx, "/send/audio", "audio", deviceID, telefono, archivo, pie)
}

// EnviarVideo manda un video.
func (c *Cliente) EnviarVideo(ctx context.Context, deviceID, telefono, archivo, pie string) (Envio, error) {
	return c.enviarMedia(ctx, "/send/video", "video", deviceID, telefono, archivo, pie)
}

// EnviarArchivo manda un documento.
func (c *Cliente) EnviarArchivo(ctx context.Context, deviceID, telefono, archivo, pie string) (Envio, error) {
	return c.enviarMedia(ctx, "/send/file", "file", deviceID, telefono, archivo, pie)
}

// enviarMedia arma el multipart y lo entrega. El campo del archivo se llama
// como el tipo (`image`, `audio`, `video`, `file`), que es el contrato de GOWA.
func (c *Cliente) enviarMedia(ctx context.Context, ruta, campo, deviceID, telefono, archivo, pie string) (Envio, error) {
	f, err := os.Open(archivo)
	if err != nil {
		return Envio{}, &Error{Mensaje: "no se pudo abrir el archivo: " + err.Error()}
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("phone", telefono)
	if strings.TrimSpace(pie) != "" {
		_ = w.WriteField("caption", pie)
	}
	parte, err := w.CreateFormFile(campo, filepath.Base(archivo))
	if err != nil {
		return Envio{}, &Error{Mensaje: "no se pudo preparar el archivo: " + err.Error()}
	}
	if _, err := io.Copy(parte, f); err != nil {
		return Envio{}, &Error{Mensaje: "no se pudo leer el archivo: " + err.Error()}
	}
	if err := w.Close(); err != nil {
		return Envio{}, &Error{Mensaje: "no se pudo cerrar el formulario: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+ruta, &buf)
	if err != nil {
		return Envio{}, &Error{Mensaje: "dirección del servicio inválida: " + err.Error()}
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ERPGo-mensajeria/1")
	if c.Usuario != "" {
		req.SetBasicAuth(c.Usuario, c.Password)
	}
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}
	cliente := c.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return Envio{}, &Error{Mensaje: "no se pudo hablar con el servicio: " + err.Error(), Temporal: true}
	}
	defer resp.Body.Close()
	bruto, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Envio{}, errorDeRespuesta(resp.StatusCode, bruto)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(bruto, &m)
	var s sobre
	if json.Unmarshal(bruto, &s) == nil && len(s.Results) > 0 {
		_ = json.Unmarshal(s.Results, &m)
	}
	return Envio{
		MensajeID: textoDe(m, "message_id", "id", "msg_id"),
		Estado:    textoDe(m, "status", "state"),
	}, nil
}

// DescargarMedia baja el archivo de un mensaje entrante. El daemon lo sirve
// desde /message/{id}/download, con la misma autenticación que el resto.
func (c *Cliente) DescargarMedia(ctx context.Context, deviceID, messageID string) ([]byte, string, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, "", &Error{Mensaje: "el mensaje no tiene id para descargar su archivo"}
	}
	ruta := "/message/" + url.PathEscape(messageID) + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+ruta, nil)
	if err != nil {
		return nil, "", &Error{Mensaje: "dirección del servicio inválida: " + err.Error()}
	}
	if c.Usuario != "" {
		req.SetBasicAuth(c.Usuario, c.Password)
	}
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}
	cliente := c.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return nil, "", &Error{Mensaje: "no se pudo descargar el archivo: " + err.Error(), Temporal: true}
	}
	defer resp.Body.Close()
	bruto, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", errorDeRespuesta(resp.StatusCode, bruto)
	}
	tipo := resp.Header.Get("Content-Type")
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	return bruto, tipo, nil
}

// Descargar trae el contenido de un enlace del daemon (el QR es el caso normal).
// Acepta `data:` en línea, una URL absoluta o una ruta relativa al
// daemon. Devuelve también el tipo de contenido para servirlo tal cual.
//
// EL NAVEGADOR NUNCA HABLA CON EL DAEMON: el ERP descarga la imagen y la sirve
// por una ruta autenticada. Así el puerto del daemon no se expone y la imagen
// no queda en la caché del navegador con una URL interna.
func (c *Cliente) Descargar(ctx context.Context, enlace string) ([]byte, string, error) {
	enlace = strings.TrimSpace(enlace)
	if enlace == "" {
		return nil, "", &Error{Mensaje: "el daemon no devolvió el enlace del QR"}
	}
	if strings.HasPrefix(enlace, "data:") {
		coma := strings.IndexByte(enlace, ',')
		if coma < 0 {
			return nil, "", &Error{Mensaje: "el QR en línea no tiene contenido"}
		}
		cabecera, datos := enlace[:coma], enlace[coma+1:]
		tipo := "image/png"
		if i := strings.Index(cabecera, ";"); i > 5 {
			tipo = cabecera[5:i]
		}
		bruto, err := base64.StdEncoding.DecodeString(datos)
		if err != nil {
			return nil, "", &Error{Mensaje: "el QR en línea no está bien codificado: " + err.Error()}
		}
		return bruto, tipo, nil
	}

	destino := enlace
	if !strings.HasPrefix(enlace, "http://") && !strings.HasPrefix(enlace, "https://") {
		destino = c.BaseURL + "/" + strings.TrimLeft(enlace, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		return nil, "", &Error{Mensaje: "enlace del QR inválido: " + err.Error()}
	}
	if c.Usuario != "" {
		req.SetBasicAuth(c.Usuario, c.Password)
	}
	cliente := c.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return nil, "", &Error{Mensaje: "no se pudo descargar el QR: " + err.Error(), Temporal: true}
	}
	defer resp.Body.Close()
	bruto, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", errorDeRespuesta(resp.StatusCode, bruto)
	}
	tipo := resp.Header.Get("Content-Type")
	if tipo == "" {
		tipo = "image/png"
	}
	return bruto, tipo, nil
}

// hacer ejecuta una llamada y traduce el resultado.
func (c *Cliente) hacer(ctx context.Context, metodo, ruta, deviceID string, cuerpo interface{}, salida interface{}, autenticar bool) error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return &Error{Mensaje: "no hay dirección del servicio de mensajería configurada"}
	}
	var lector io.Reader
	if cuerpo != nil {
		bruto, err := json.Marshal(cuerpo)
		if err != nil {
			return &Error{Mensaje: "no se pudo preparar la petición: " + err.Error()}
		}
		lector = bytes.NewReader(bruto)
	}

	req, err := http.NewRequestWithContext(ctx, metodo, c.BaseURL+ruta, lector)
	if err != nil {
		return &Error{Mensaje: "dirección del servicio inválida: " + err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ERPGo-mensajeria/1")
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if autenticar && c.Usuario != "" {
		req.SetBasicAuth(c.Usuario, c.Password)
	}
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}

	cliente := c.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return &Error{Mensaje: "no se pudo hablar con el servicio: " + err.Error(), Temporal: true}
	}
	defer resp.Body.Close()

	bruto, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorDeRespuesta(resp.StatusCode, bruto)
	}
	if salida == nil {
		return nil
	}

	var s sobre
	if err := json.Unmarshal(bruto, &s); err == nil {
		crudo := s.Results
		if len(crudo) == 0 || string(crudo) == "null" {
			crudo = s.Data
		}
		if len(crudo) > 0 && string(crudo) != "null" {
			if err := json.Unmarshal(crudo, salida); err == nil {
				return nil
			}
		}
	}
	if err := json.Unmarshal(bruto, salida); err != nil {
		return &Error{Status: resp.StatusCode, Mensaje: "respuesta inesperada del servicio: " + recortar(string(bruto), 200)}
	}
	return nil
}

// errorDeRespuesta clasifica el fallo. Un 401/403 no se reintenta: la
// credencial está mal y reintentar no la arregla.
func errorDeRespuesta(status int, bruto []byte) *Error {
	e := &Error{Status: status, Temporal: status >= 500 || status == http.StatusTooManyRequests}
	var s sobre
	if json.Unmarshal(bruto, &s) == nil {
		e.Codigo = s.Code
		e.Mensaje = strings.TrimSpace(s.Message)
	}
	if e.Mensaje == "" {
		e.Mensaje = recortar(strings.TrimSpace(string(bruto)), 300)
	}
	if e.Mensaje == "" {
		e.Mensaje = http.StatusText(status)
	}
	return e
}

// textoDe devuelve el primer valor de texto no vacío entre varias claves.
func textoDe(m map[string]interface{}, claves ...string) string {
	for _, k := range claves {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func numeroDe(m map[string]interface{}, claves ...string) (int, bool) {
	for _, k := range claves {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return int(n), true
			case int:
				return n, true
			}
		}
	}
	return 0, false
}

func recortar(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
