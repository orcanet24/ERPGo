package gowa

// Pruebas de media: el contrato del multipart y la descarga de un mensaje.
// No tocan WhatsApp: el servidor falso comprueba el campo del archivo y las
// cabeceras, que es lo que se rompe cuando el daemon cambia.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnviarImagenArmaElMultipartYDevuelveElId(t *testing.T) {
	var campo, telefono, pie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/send/image":
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Errorf("multipart inválido: %v", err)
			}
			telefono = r.FormValue("phone")
			pie = r.FormValue("caption")
			archivo, cabecera, err := r.FormFile("image")
			if err != nil {
				t.Errorf("falta el campo image: %v", err)
			} else {
				campo = cabecera.Filename
				_, _ = io.Copy(io.Discard, archivo)
				_ = archivo.Close()
			}
			if r.Header.Get("X-Device-Id") != "acme.cxc" {
				t.Errorf("falta X-Device-Id")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"results":{"message_id":"IMG-1","status":"sent"}}`))
		case "/message/M1/download":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\ncontenido"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	archivo := filepath.Join(t.TempDir(), "foto.png")
	if err := os.WriteFile(archivo, []byte("\x89PNG\r\n\x1a\ndatos"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Nuevo(srv.URL, "erpgo", "x")

	envio, err := c.EnviarImagen(context.Background(), "acme.cxc", "584141234567", archivo, "Mire esto")
	if err != nil {
		t.Fatal(err)
	}
	if envio.MensajeID != "IMG-1" {
		t.Fatalf("id inesperado: %+v", envio)
	}
	if telefono != "584141234567" || pie != "Mire esto" || campo != "foto.png" {
		t.Fatalf("multipart inesperado: phone=%q caption=%q archivo=%q", telefono, pie, campo)
	}

	datos, tipo, err := c.DescargarMedia(context.Background(), "acme.cxc", "M1")
	if err != nil {
		t.Fatal(err)
	}
	if tipo != "image/png" || len(datos) == 0 {
		t.Fatalf("descarga inesperada: %q %d bytes", tipo, len(datos))
	}
}

func TestEnviarMediaRechazaUnArchivoQueNoExiste(t *testing.T) {
	c := Nuevo("http://127.0.0.1:1", "", "")
	if _, err := c.EnviarArchivo(context.Background(), "x", "58414", filepath.Join(t.TempDir(), "no.pdf"), ""); err == nil {
		t.Fatal("un archivo inexistente debía fallar antes de llamar al daemon")
	}
}
