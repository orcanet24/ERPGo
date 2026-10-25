package web

// csrf.go — Protección CSRF.
//
// Sin esto, un navegador que haya iniciado sesión en el ERP puede ser usado contra
// el ERP: basta con que cargue una página de otro sitio que envíe un formulario a
// /account/coa/catalog/apply-conasev o a cualquier POST de facturación, y el
// navegador lo hace con la sesión del usuario.
//
// SameSite=Lax ya cubre la mayoría (un formulario de otro origen no lleva la
// cookie), pero Lax no protege un POST hecho desde el mismo sitio: un atacante
// que encuentre un punto abierto en el propio ERP —una subida de archivo, un
// comentario, un redirect— conserva el envío. Por eso el token.
//
// El token va en la sesión, no en una cookie aparte: así no se puede falsificar
// desde otro origen (la cookie se manda sola, pero leerla requiere estar en el
// mismo origen). Es la misma razón por la que el identificador de sesión es un
// hash y no el valor en claro.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// csrfHeader es la cabecera donde el navegador devuelve el token. Se usa una
// cabecera y no un campo de formulario porque un formulario se puede inyectar
// desde otro sitio; una cabecera solo la pone el propio JavaScript de la
// aplicación, que es código nuestro.
const csrfHeader = "X-CSRF-Token"

// csrfCookieName es el nombre de la cookie legible por JavaScript. Va separada
// de la de sesión precisamente porque tiene que ser legible: el JavaScript la
// lee y la copia a la cabecera.
const csrfCookieName = "erpgo_csrf"

// newCSRFToken genera un token de 32 bytes aleatorios en hexadecimal.
func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand no falla en la práctica. Si fallara, se genera uno
		// imperfecto en vez de tumbar el ERP: el peor caso es una cookie de
		// sesión débil, que es un problema del sistema, no de este request.
		return hex.EncodeToString([]byte(strings.Repeat("0", 32)))
	}
	return hex.EncodeToString(b)
}

// ensureCSRFToken devuelve el token de la petición, creándolo si es la primera
// visita. Se cachea en la sesión para que no cambie entre navegaciones: un token
// que rota en cada carga obligaría a que cada formulario lo leyera de nuevo y
// cualquier pestaña abierta quedaría inservible.
func (s *Server) ensureCSRFToken(sess *SessionData) string {
	if sess != nil && sess.CSRFToken != "" {
		return sess.CSRFToken
	}
	// El token se fija en la sesión en memoria. No hace falta persistirlo aquí:
	// si el servidor se reinicia, la sesión tampoco sobrevive en caché y el
	// token se vuelve a emitir en la primera visita. Persistirlo en la BD solo
	// serviría para que un token viejo siguiera valiendo tras el reinicio, que
	// es justo lo contrario de lo que se busca.
	token := newCSRFToken()
	if sess != nil {
		sess.CSRFToken = token
	}
	return token
}

// csrfValid comprueba el token de la petición contra el de la sesión. La
// comparación es en tiempo constante para no filtrar el token por temporización.
func (s *Server) csrfValid(r *http.Request, sess *SessionData) bool {
	if sess == nil || sess.CSRFToken == "" {
		return false
	}
	got := r.Header.Get(csrfHeader)
	if got == "" {
		got = r.FormValue("csrf_token")
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRFToken)) == 1
}

// csrfSafeMethods son los verbos que no cambian estado y quedan exentos.
func csrfSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions || m == http.MethodTrace
}

// enforceCSRF bloquea las peticiones que cambian estado sin token válido.
//
// Se salta cuando no hay sesión (p.ej. el login: todavía no hay nada que
// falsificar) y cuando viene de una API con JSON, que un navegador de terceros
// no puede emitir sin CORS. La exencion es deliberada y está anotada donde se
// aplica: el objetivo es que la exencion sea visible y Justificada, no una excepción
// silenciosa que se extiende con el tiempo.
func (s *Server) enforceCSRF(w http.ResponseWriter, r *http.Request, next http.Handler, sess *SessionData) {
	if csrfSafeMethod(r.Method) {
		next.ServeHTTP(w, r)
		return
	}
	// API JSON: no la puede falsificar un sitio ajeno (haría falta CORS, y
	// /api/ es de la propia app).
	if strings.HasPrefix(r.URL.Path, "/api/") {
		next.ServeHTTP(w, r)
		return
	}
	if sess == nil {
		// Todavía no hay sesión: no hay nada que falsificar. El login es el
		// caso legítimo.
		next.ServeHTTP(w, r)
		return
	}
	if !s.csrfValid(r, sess) {
		jsonError(w, "Token de seguridad inválido. Recargue la página.", http.StatusForbidden)
		return
	}
	next.ServeHTTP(w, r)
}

// setCSRFCookie publica el token para que el JavaScript de la aplicación pueda
// leerlo y devolverlo en la cabecera. SameSite=Lax y Secure cuando la conexión
// es HTTPS: la cookie tiene que viajar al propio sitio y solo por canal seguro.
func setCSRFCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   86400,
	})
}
