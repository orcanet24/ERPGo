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
type gzipResponseWriter struct {
	http.ResponseWriter
	buf       *bytes.Buffer
	gw        *gzip.Writer
	wroteGzip bool
	status    int
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.status = code
	// NO pasamos el header al original aún; esperamos al flush
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if g.buf == nil {
		g.buf = new(bytes.Buffer)
	}
	return g.buf.Write(p)
}

func (g *gzipResponseWriter) WriteToOriginal(compress bool) {
	if g.wroteGzip {
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
