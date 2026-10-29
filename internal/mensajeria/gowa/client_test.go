package gowa

// Pruebas del cliente contra un GOWA FALSO. Ninguna de estas pruebas toca
// WhatsApp ni la red: lo que se certifica es el contrato (autenticación,
// cabecera de dispositivo, lectura de `results`, clasificación de errores), que
// es exactamente lo que se rompe cuando el daemon cambia de versión.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func servidorFalso(t *testing.T, vistos *http.Request, cuerpos *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vistos != nil {
			*vistos = *r
		}
		if cuerpos != nil {
			b, _ := io.ReadAll(r.Body)
			*cuerpos = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok"}`))
		case r.URL.Path == "/app/info":
			_, _ = w.Write([]byte(`{"code":200,"results":{"version":"9.1.0"}}`))
		case r.URL.Path == "/devices" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"code":200,"results":[{"device_id":"acme.cxc","state":"connected","jid":"58414@s.whatsapp.net"}]}`))
		case strings.HasSuffix(r.URL.Path, "/login"):
			_, _ = w.Write([]byte(`{"code":200,"results":{"qr_link":"http://127.0.0.1/qr/abc","qr_duration":30}}`))
		case strings.HasSuffix(r.URL.Path, "/login/code"):
			_, _ = w.Write([]byte(`{"code":200,"results":{"pair_code":"123-456"}}`))
		case r.URL.Path == "/send/message":
			_, _ = w.Write([]byte(`{"code":200,"results":{"message_id":"MSG-1","status":"sent"}}`))
		case r.URL.Path == "/user/check":
			_, _ = w.Write([]byte(`{"code":200,"results":{"is_registered":true}}`))
		case strings.HasSuffix(r.URL.Path, "/status") && strings.Contains(r.URL.Path, "malo"):
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"invalid credentials"}`))
		case strings.HasSuffix(r.URL.Path, "/status") && strings.Contains(r.URL.Path, "caido"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":500,"message":"panic"}`))
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = w.Write([]byte(`{"code":200,"results":{"device_id":"acme.cxc","state":"connected","msisdn":"584141234567"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
		}
	}))
}

func TestClienteHablaConElDaemonYUsaSusCabeceras(t *testing.T) {
	var visto http.Request
	var cuerpo string
	srv := servidorFalso(t, &visto, &cuerpo)
	defer srv.Close()

	c := Nuevo(srv.URL, "erpgo", "secreto")

	if err := c.Salud(context.Background()); err != nil {
		t.Fatalf("salud: %v", err)
	}

	v, err := c.Version(context.Background())
	if err != nil || v != "9.1.0" {
		t.Fatalf("versión: %q, %v", v, err)
	}

	lista, err := c.Dispositivos(context.Background())
	if err != nil || len(lista) != 1 || lista[0].ID != "acme.cxc" {
		t.Fatalf("dispositivos: %+v, %v", lista, err)
	}

	qr, err := c.Login(context.Background(), "acme.cxc")
	if err != nil || qr.Enlace == "" || qr.Segundos != 30 {
		t.Fatalf("login: %+v, %v", qr, err)
	}
	if visto.Header.Get("X-Device-Id") != "acme.cxc" {
		t.Fatalf("falta la cabecera X-Device-Id: %v", visto.Header)
	}

	codigo, err := c.LoginCodigo(context.Background(), "acme.cxc", "584141234567")
	if err != nil || codigo != "123-456" {
		t.Fatalf("código: %q, %v", codigo, err)
	}

	envio, err := c.EnviarTexto(context.Background(), "acme.cxc", "584141234567", "Hola")
	if err != nil || envio.MensajeID != "MSG-1" {
		t.Fatalf("envío: %+v, %v", envio, err)
	}
	var pedido map[string]interface{}
	if err := json.Unmarshal([]byte(cuerpo), &pedido); err != nil {
		t.Fatalf("el cuerpo enviado no es JSON: %v", err)
	}
	if pedido["phone"] != "584141234567" || pedido["message"] != "Hola" {
		t.Fatalf("cuerpo inesperado: %s", cuerpo)
	}

	registrado, err := c.VerificarNumero(context.Background(), "acme.cxc", "584141234567")
	if err != nil || !registrado {
		t.Fatalf("verificación: %v, %v", registrado, err)
	}

	// Basic Auth: el daemon exige credenciales en cada llamada autenticada.
	esperado := "Basic " + base64.StdEncoding.EncodeToString([]byte("erpgo:secreto"))
	if visto.Header.Get("Authorization") != esperado {
		t.Fatalf("Authorization inesperada: %q", visto.Header.Get("Authorization"))
	}
}

func TestClienteClasificaLosErrores(t *testing.T) {
	srv := servidorFalso(t, nil, nil)
	defer srv.Close()
	c := Nuevo(srv.URL, "erpgo", "secreto")

	_, err := c.Estado(context.Background(), "malo")
	fe, ok := err.(*Error)
	if !ok || fe.Status != 401 || fe.EsTemporal() {
		t.Fatalf("un 401 debe ser un error NO temporal; fue: %#v", err)
	}

	_, err = c.Estado(context.Background(), "caido")
	fe, ok = err.(*Error)
	if !ok || fe.Status != 500 || !fe.EsTemporal() {
		t.Fatalf("un 500 debe ser temporal; fue: %#v", err)
	}

	// Sin URL configurada no se inventa un destino.
	c2 := Nuevo("", "", "")
	if err := c2.Salud(context.Background()); err == nil {
		t.Fatal("sin dirección del servicio la llamada debía fallar")
	}
}

func TestClienteNoConfundeUnErrorConUnaRespuestaValida(t *testing.T) {
	// El daemon devuelve 200 pero con un cuerpo que no es lo esperado: se
	// devuelve error en vez de un valor vacío que parezca bueno.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`esto no es json`))
	}))
	defer srv.Close()

	c := Nuevo(srv.URL, "", "")
	if _, err := c.Version(context.Background()); err == nil {
		t.Fatal("un cuerpo ilegible debía devolver error")
	}
}
