package web

// appearance.go — Apariencia de cada usuario.
//
// El tema (claro / oscuro / automático) y la densidad (cómoda /
// compacta) son decisiones personales: quien llena una pantalla de
// facturas ocho horas seguidas no debería ver lo mismo que quien
// abre el sistema un minuto para consultar un saldo.
//
// Se guarda en user_view_pref con view_key = "appearance", la misma
// tabla y el mismo endpoint que ya sirven para las vistas
// personalizables del panel. No hay migración ni columna nueva: la
// clave "appearance" es una clave de pantalla válida (ValidViewKey
// solo admite minúsculas, dígitos y punto, guion y guion bajo) y el
// payload es un objeto JSON opaco, que es justo lo que esa tabla
// promete guardar.
//
// El servidor decide el atributo que viaja en el <html> de origen,
// no el navegador: por eso no hay destello al cambiar de tema, ni
// siquiera antes de que cargue una línea de JavaScript.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// Valores admitidos. Son los únicos: cualquier otra cosa se cae al
// valor por defecto en lugar de escribir un atributo que no
// corresponde con ningún bloque de CSS.
const (
	ThemeLight = "light"
	ThemeDark  = "dark"
	ThemeAuto  = "auto"

	DensityComfortable = "comfortable"
	DensityCompact     = "compact"

	// appearanceViewKey es la clave de pantalla bajo la que se guarda.
	appearanceViewKey = "appearance"
	// appearanceCookie permite que las pantallas sin sesión (login,
	// selector de empresa) respeten la preferencia. No es la fuente
	// de verdad —eso es la tabla—, pero es lo único que existe antes
	// de entrar.
	appearanceCookie = "erpgo_appearance"
	// appearanceCookieMaxAge es un año: es una preferencia, no un
	// dato sensible, y que el login salte a claro después de haber
	// elegido oscuro se lee como un fallo.
	appearanceCookieMaxAge = 365 * 24 * 3600
)

// Appearance son las dos decisiones de presentación de un usuario.
type Appearance struct {
	Theme   string `json:"theme"`
	Density string `json:"density"`
}

// DefaultAppearance es lo que se aplica cuando el usuario nunca dijo
// nada. El tema por defecto es claro, no automático: quien abre esto
// suele estar en un mostrador con luz de día, y un cambio inesperado
// al entrar genera más desconfianza que un tema neutro.
func DefaultAppearance() Appearance {
	return Appearance{Theme: ThemeLight, Density: DensityComfortable}
}

// normalizeAppearance deja la preferencia en un estado conocido. No
// falla nunca: una preferencia que no se entiende se sustituye por
// la de por defecto, porque una pantalla sin tema es peor que una
// pantalla con un tema que no es el que se pidió.
func normalizeAppearance(a Appearance) Appearance {
	d := DefaultAppearance()
	switch strings.ToLower(strings.TrimSpace(a.Theme)) {
	case ThemeLight:
		d.Theme = ThemeLight
	case ThemeDark:
		d.Theme = ThemeDark
	case ThemeAuto:
		d.Theme = ThemeAuto
	}
	switch strings.ToLower(strings.TrimSpace(a.Density)) {
	case DensityCompact:
		d.Density = DensityCompact
	default:
		d.Density = DensityComfortable
	}
	return d
}

// appearanceOf es la forma corta de construir una preferencia para
// pasarla a normalizeAppearance.
func appearanceOf(theme, density string) Appearance {
	return Appearance{Theme: theme, Density: density}
}

// parseAppearance lee el payload JSON de la tabla de vistas. Se
// acepta tanto {"theme":...} como un arreglo ["dark","compact"] para
// que una preferencia escrita a mano no se pierda por un detalle de
// forma; la forma correcta la escribe appearance.js.
func parseAppearance(payload string) Appearance {
	a := DefaultAppearance()
	p := strings.TrimSpace(payload)
	if p == "" {
		return a
	}
	var obj struct {
		Theme   string `json:"theme"`
		Density string `json:"density"`
	}
	if err := json.Unmarshal([]byte(p), &obj); err == nil {
		return normalizeAppearance(appearanceOf(obj.Theme, obj.Density))
	}
	var arr []string
	if err := json.Unmarshal([]byte(p), &arr); err == nil && len(arr) >= 2 {
		return normalizeAppearance(appearanceOf(arr[0], arr[1]))
	}
	return a
}

// appearanceFromRequest devuelve la apariencia que corresponde a esta
// petición.
//
// El orden importa: con sesión mandan las vistas guardadas, porque
// son las que el usuario eligió y siguen ahí aunque cambie de
// navegador; sin sesión, la cookie es lo único disponible y sirve
// para que el login no contradiga lo que la persona pidió ayer.
//
// dbFor(r) se usa porque la tabla vive en la base de la EMPRESA: la
// preferencia es de la persona, dentro de su empresa.
func (s *Server) appearanceFromRequest(r *http.Request) Appearance {
	if r == nil {
		return DefaultAppearance()
	}
	if sess := s.sessionFromRequest(r); sess != nil && sess.UserID != "" {
		// Se lee con sessUserID y NO con el nombre de usuario porque eso es
		// lo que usa el endpoint que la escritura. Leer con una clave y
		// escribir con otra hace que la preferencia se guarde y luego no se
		// encuentre: el interruptor no haría nada y no habría ningún aviso.
		v, found, err := LoadViewPref(s.dbFor(r), sessUserID(sess), appearanceViewKey, "")
		if err != nil || !found {
			return DefaultAppearance()
		}
		return parseAppearance(string(v.Payload))
	}
	return appearanceFromCookie(r)
}

// appearanceFromCookie lee la preferencia que dejó el navegador.
func appearanceFromCookie(r *http.Request) Appearance {
	c, err := r.Cookie(appearanceCookie)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return DefaultAppearance()
	}
	// El valor es "tema.densidad". El punto va porque NO necesita
	// codificarse: Request.Cookie() devuelve el valor tal cual, sin
	// decodificar, así que un separador que el navegador escapara
	// llegaría aquí sin entenderlo y la preferencia se perdería en
	// silencio.
	//
	// Se acepta también la barra vertical porque es lo que escribió la
	// primera versión de esta preferencia, y porque una cookie
	// escapada de esa forma es justo lo que hay que entender en lugar
	// de ignorar en silencio.
	if v, err := url.QueryUnescape(c.Value); err == nil {
		c.Value = v
	}
	parts := strings.FieldsFunc(c.Value, func(r rune) bool { return r == '.' || r == '|' })
	if len(parts) == 0 {
		return DefaultAppearance()
	}
	if len(parts) > 2 {
		parts = parts[:2]
	}
	density := DensityComfortable
	if len(parts) == 2 {
		density = parts[1]
	}
	return normalizeAppearance(appearanceOf(parts[0], density))
}

// saveAppearance guarda la preferencia del usuario y refleja el
// cambio en la cookie, para que la próxima pantalla sin sesión
// salga igual.
func (s *Server) saveAppearance(w http.ResponseWriter, r *http.Request, a Appearance) error {
	a = normalizeAppearance(a)
	if sess := s.sessionFromRequest(r); sess != nil && sess.UserID != "" {
		payload, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if err := SaveViewPref(s.dbFor(r), sessUserID(sess), appearanceViewKey, "", string(payload)); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     appearanceCookie,
		Value:    a.Theme + "." + a.Density,
		Path:     "/",
		MaxAge:   appearanceCookieMaxAge,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}
