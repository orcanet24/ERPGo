package web

// sse_entrega_test.go — El asistente de IA de /reports/custom/add nunca daba
// respuesta y aquí están las DOS causas, cada una con su prueba:
//
//  1. El middleware de gzip bufferizaba las respuestas text/event-stream y,
//     después del primer Flush (wroteGzip=true), WriteToOriginal nunca volvió a
//     soltar los eventos escritos después: el primer «progreso» llegaba y el
//     «listo» final se perdía para siempre.
//  2. El WriteTimeout global del servidor (120 s) se aplicaba al total de la
//     respuesta, pero el agente puede tardar mucho más (Fase 1 + hasta
//     maxSteps llamadas al proveedor de aiTimeout): net/http cortaba la conexión
//     con el stream vivo y el modal jamás recibía el resultado.
//
// POR QUÉ SE PRUEBA CON UN SERVIDOR DE PRUEBA: el defecto era de ENTREGA (qué
// le llega al navegador y cuándo muere la conexión), no de lógica de negocio:
// la única forma honesta de demostrarlo es pasar una respuesta SSE por el
// middleware real y leerla como la lee el fetch del modal.

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestElStreamSSEAtraviesaElGzipCompleto: una respuesta event-stream escrita en
// tres eventos con flush entre cada uno llega ENTERA al cliente aunque el
// request pida gzip, sin Content-Encoding y sin perder el evento final.
func TestElStreamSSEAtraviesaElGzipCompleto(t *testing.T) {
	const bodyEsperado = `event: progreso
data: Fase 1

event: progreso
data: Paso 1/8

event: listo
data: {"status":1}

`
	sseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Igual que reportTemplateAIPreviewStream: header antes de escribir.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		for _, ev := range []string{
			"event: progreso\ndata: Fase 1\n\n",
			"event: progreso\ndata: Paso 1/8\n\n",
			"event: listo\ndata: {\"status\":1}\n\n",
		} {
			_, _ = w.Write([]byte(ev))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	})

	srv := httptest.NewServer(gzipMiddleware(sseHandler))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("GET", srv.URL+"/reports/custom/ai-preview-stream", nil)
	req.Header.Set("Accept-Encoding", "gzip") // lo que manda cualquier navegador
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("el request al stream debía salir: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if enc := resp.Header.Get("Content-Encoding"); enc == "gzip" {
		t.Errorf("un event-stream NO debe comprimirse: Content-Encoding=%q rompe la entrega evento a evento", enc)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("el Content-Type debe conservarse como event-stream, llegó %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leer el stream debía completarse: %v", err)
	}
	if string(body) != bodyEsperado {
		t.Errorf("el stream debía llegar ENTERO (los 3 eventos, incluido «listo»); llegó:\n%s", string(body))
	}
}

// TestLaRespuestaJSONSigueComprimiendose: el passthrough del streaming no debe
// haber roto el camino normal de gzip — un JSON grande por el mismo middleware
// sigue saliendo comprimido.
func TestLaRespuestaJSONSigueComprimiendose(t *testing.T) {
	jsonHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"definition":"` + strings.Repeat("x", 400) + `"}`))
	})

	srv := httptest.NewServer(gzipMiddleware(jsonHandler))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("GET", srv.URL+"/reports/custom/ai-save", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("el request debía salir: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("un JSON grande debe seguir saliendo comprimido, llegó Content-Encoding=%q", resp.Header.Get("Content-Encoding"))
	}
	// Se lee el body crudo (Transport no descomprime) y se descomprime a mano:
	// así la prueba verifica el binario gzip de verdad, no su descompresión implícita.
	crudo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leer el body debía completarse: %v", err)
	}
	zr, err := gzip.NewReader(strings.NewReader(string(crudo)))
	if err != nil {
		t.Fatalf("el body debería ser gzip legible: %v", err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("descomprimir el body debía completarse: %v", err)
	}
	if !strings.Contains(string(body), `"status":1`) {
		t.Errorf("el body comprimido debe descomprimirse a lo escrito: %q", string(body))
	}
}

// escritorSSECaptura el ResponseWriter mínimo que aiSSE necesita, con el
// SetWriteDeadline real grabado: así la prueba comprueba que cada evento
// renovó el plazo de la conexión en vez de confiar en que el compilador no
// cambió el orden.
type escritorSSECaptura struct {
	http.ResponseWriter
	plazos []time.Time
}

func (e *escritorSSECaptura) Write(p []byte) (int, error) { return len(p), nil }

func (e *escritorSSECaptura) Flush() {}

func (e *escritorSSECaptura) SetWriteDeadline(t time.Time) error {
	e.plazos = append(e.plazos, t)
	return nil
}

// TestCadaEventoSSEraElPlazoDeEscritura: cada aiSSE debe renovar el plazo de
// escritura hacia el futuro por lo menos aiTimeout — el margen que cubre una
// llamada al proveedor entre dos eventos; si no, el WriteTimeout global (120 s)
// corta la conexión antes de que el agente termine y el modal nunca ve el
// evento «listo».
func TestCadaEventoSSEraElPlazoDeEscritura(t *testing.T) {
	w := &escritorSSECaptura{}
	inicio := time.Now()

	aiSSE(w, "progreso", "Fase 1: clasificando módulos...")
	aiSSE(w, "listo", `{"status":1}`)

	if len(w.plazos) != 2 {
		t.Fatalf("cada evento debe renovar el plazo una vez, hubo %d renovaciones (%v)", len(w.plazos), w.plazos)
	}
	for i, plazo := range w.plazos {
		margen := plazo.Sub(inicio)
		if margen < aiTimeout {
			t.Errorf("el plazo del evento %d concedió %v; debía ser al menos aiTimeout (%v)", i+1, margen, aiTimeout)
		}
		if plazo.Before(inicio.Add(aiSSEWriteGrace).Add(-time.Second)) {
			t.Errorf("el plazo del evento %d quedó corta: %v contra la gracia declarada %v", i+1, margen, aiSSEWriteGrace)
		}
		if plazo.Before(inicio) {
			t.Errorf("el plazo del evento %d apunta al pasado: %v", i+1, plazo)
		}
	}
}
