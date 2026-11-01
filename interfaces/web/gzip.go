package web

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"
)

const gzipMinSize = 256 // No comprimir respuestas menores a 256 bytes

// gzipResponseWriter captura la respuesta y decide si comprimir al final.
// Bufferiza el body; en el flush decide si gzip conviene por tamaño y content-type.
//
// EXCEPCIÓN — streaming (text/event-stream): una respuesta SSE se escribe
// evento por evento y el browser la consume en vivo; bufferizar la mata dos
// veces. Primera: los eventos quedan retenidos hasta el fin del handler.
// Segunda (la que hizo que el asistente de IA nunca diera respuesta): el
// primer Flush ya escribía lo bufferizado y marcaba wroteGzip, y WriteToOriginal
// nunca volvió a soltar lo que llegó DESPUÉS del primer flush —incluido el
// evento «listo» final, que el modal necesita para pintar el informe—. Por eso
// los handlers SSE pasan por este wrapper EN MODO DIRECTO: cada Write va
// sin buffer y sin compresión al ResponseWriter real.
type gzipResponseWriter struct {
	http.ResponseWriter
	buf         *bytes.Buffer
	gw          *gzip.Writer
	wroteGzip   bool
	status      int
	passthrough bool // streaming: cada Write va directo al original
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.chequearPassthrough()
	g.status = code
	if !g.passthrough {
		// NO pasamos el header al original aún; esperamos al flush
		return
	}
	// Streaming: el header sale de inmediato, que es lo que el handler pidió.
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	g.chequearPassthrough()
	if g.passthrough {
		return g.ResponseWriter.Write(p)
	}
	if g.buf == nil {
		g.buf = new(bytes.Buffer)
	}
	return g.buf.Write(p)
}

// chequearPassthrough activa el modo directo cuando el handler declaró una
// respuesta de streaming: Content-Type text/event-stream (los handlers SSE lo
// fija antes de escribir el cuerpo). Se invoca tanto en WriteHeader como en
// Write porque un handler puede omitir WriteHeader (vía implícita a 200).
func (g *gzipResponseWriter) chequearPassthrough() {
	if g.passthrough {
		return
	}
	if !strings.HasPrefix(g.Header().Get("Content-Type"), "text/event-stream") {
		return
	}
	g.passthrough = true
	g.wroteGzip = true // el cierre diferido (defer WriteToOriginal) ya no debe tocar esta respuesta: se escribió sin buffer
}

func (g *gzipResponseWriter) WriteToOriginal(compress bool) {
	if g.wroteGzip || g.passthrough {
		return
	}
	g.wroteGzip = true

	var body []byte
	if g.buf != nil {
		body = g.buf.Bytes()
	}
	ct := g.Header().Get("Content-Type")

	shouldCompress := compress &&
		g.status >= 200 && g.status < 400 &&
		len(body) > gzipMinSize &&
		isTextContent(ct)

	if !shouldCompress {
		g.ResponseWriter.WriteHeader(g.status)
		if len(body) > 0 {
			g.ResponseWriter.Write(body)
		}
		return
	}

	g.Header().Set("Content-Encoding", "gzip")
	g.Header().Del("Content-Length")

	gw, err := gzip.NewWriterLevel(g.ResponseWriter, gzip.BestSpeed)
	if err != nil {
		g.ResponseWriter.WriteHeader(g.status)
		if len(body) > 0 {
			g.ResponseWriter.Write(body)
		}
		return
	}

	g.ResponseWriter.WriteHeader(g.status)
	if len(body) > 0 {
		gw.Write(body)
	}
	gw.Close()
}

func (g *gzipResponseWriter) Flush() {
	if g.passthrough {
		// Streaming: el flush va directo; nada que decidir.
		if f, ok := g.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	g.WriteToOriginal(true)
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponseWriter) Hijack() (interface{}, error) {
	// Forzar flush antes de hijack
	g.WriteToOriginal(false)
	if hj, ok := g.ResponseWriter.(interface {
		Hijack() (interface{}, error)
	}); ok {
		return hj.Hijack()
	}
	return nil, errNoHijack
}

// Unwrap expone el ResponseWriter original. http.ResponseController lo usa
// para alcanzar los plazos de la conexión REAL (SetWriteDeadline): los
// handlers SSE renuevan el plazo del WriteTimeout global del servidor en cada
// evento, y sin este método el wrapper de gzip se lo taparía.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

var errNoHijack = http.ErrHijacked

// gzipMiddleware comprime respuestas textuales si el cliente acepta gzip.
// Balance: gzip.BestSpeed (nivel 1) usa poca CPU, ahorra ancho de banda.
// No comprime: binarios (png, jpg, woff), respuestas chicas, errores.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		// No comprimir conexiones que se upgradan (WebSocket)
		if r.Header.Get("Upgrade") != "" {
			next.ServeHTTP(w, r)
			return
		}

		g := &gzipResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		defer g.WriteToOriginal(true)
		next.ServeHTTP(g, r)
	})
}

// isTextContent devuelve true para MIMEs que se comprimen bien con gzip.
func isTextContent(ct string) bool {
	ct = strings.ToLower(ct)
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/javascript", ct == "application/ecmascript":
		return true
	case strings.HasPrefix(ct, "application/javascript"), strings.HasPrefix(ct, "application/json"):
		return true
	case strings.HasPrefix(ct, "application/xml"), ct == "application/rss+xml":
		return true
	case ct == "application/atom+xml", ct == "image/svg+xml":
		return true
	case ct == "application/wasm":
		return true
	}
	return false
}
