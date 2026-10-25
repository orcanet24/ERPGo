package web

// password.go — Verificación de contraseñas y migración a bcrypt.
//
// ESTADO REAL, medido en las bases el 03-10 (y no supuesto):
//
//	admin     bcrypt $2a$10$…   correcto
//	revision  "revision123"      TEXTO PLANO
//
// Así que el agujero era real pero más estrecho de lo que decía la bitácora:
// el login ya usaba bcrypt (handlers.go:150) y user.go ya generaba bcrypt. Lo
// que quedaba eran filas heredadas. El código de esas filas no lo decidió el
// login: lo decidió quien las creó, y por eso hay que comprobarlo y no
// suponerlo.
//
// LA REGLA. Una contraseña solo se acepta si el formatoStored dice que es su
// formato, y SIEMPRE con comparación en tiempo constante. El problema del
// código anterior era doble: aceptaba md5 Y texto plano sin distinguir, y
// usaba == , que devuelve en cuanto encuentra una diferencia y filtra
// información por el tiempo que tarda.
//
// REHASH AL ENTRAR. Cuando alguien entra con una contraseña heredada, la
// reescribimos a bcrypt en ese mismo instante. Es el único momento en que
// tenemos la contraseña en claro Y sabemos que es correcta, así que es la única
// oportunidad de arreglarla sin molestar al usuario: él no nota nada y la fila
// queda bien al día. Un programa aparte que reescriba de noche sería un
// segundo camino, y el plan de seguridad no admite caminos.
//
// LA CONTRASEÑA EN TEXTO PLANO ES UN CASO LEGÍTIMO HEREJADO, no un descuido:
// hay instalaciones antiguas que guardaban la contraseña tal cual. Si no se
// acepta, esos usuarios no pueden entrar nunca y el sistema queda inaccesible.
// Se acepta SOLO si coincide exactamente, y se reescribe a bcrypt nada más
// entrar.

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// bcryptGenerate es el único punto donde se crea un hash de contraseña.
//
// Va aparte para que no quede ningún sitio en el código donde se pueda
// "hashear" sin hashear de verdad. El fallo que se corrigió era precisamente
// una función llamada hashPassword que devolvía la contraseña sin transformar:
// el nombre prometía una cosa y el cuerpo hacía otra.
func bcryptGenerate(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
}

// formatoPassword dice cómo está guardada una contraseña y, con ello, cómo hay
// que verificarla.
type formatoPassword int

const (
	formatoDesconocido formatoPassword = iota
	formatoBcrypt
	formatoMD5Prefijado // md5('gef' . contraseña), el esquema del PHP heredado
	formatoMD5          // md5(contraseña), variante de algunas filas
	formatoPlano        // la contraseña tal cual (instalaciones antiguas)
)

// clasificarPassword decide el formato por la FORMA del valor guardado.
//
// Se decide por la forma y no por el contenido, por una razón concreta: si se
// probara "quiza es md5" contra el texto que introduce el usuario, bastaría con
// que alguien guardara una contraseña de 32 caracteres y entraría con ella
// aunque no fuera un md5. La forma es lo único que no depende de lo que se
// escribe al entrar.
func clasificarPassword(stored string) formatoPassword {
	switch {
	case stored == "":
		return formatoDesconocido
	case strings.HasPrefix(stored, "$2a$"), strings.HasPrefix(stored, "$2b$"),
		strings.HasPrefix(stored, "$2y$"):
		return formatoBcrypt
	case len(stored) == 32 && esHex(stored):
		// Es md5 o no es nada: 32 caracteres hexadecimales. Cuál de los dos
		// esquemas se decide al verificar, probando ambos, que es la forma
		// de no adivinar.
		return formatoMD5Prefijado
	case esLegacyPlano(stored):
		return formatoPlano
	default:
		return formatoDesconocido
	}
}

// esHex dice si s son solo dígitos hexadecimales.
func esHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// esLegacyPlano filtra lo que podría ser una contraseña en texto plano.
//
// El filtro tiene que ser ESTRECHO a propósito, y no por pereza sino por
// seguridad: si se aceptara cualquier texto, entonces un md5 de 32 caracteres
// que no fuera md5 entraría, y también una contraseña que fuera en realidad el
// nombre de una tabla. Solo se aceptan cosas que de verdad pueden ser la
// contraseña de un usuario: letras, dígitos y un puñado de símbolos, sin
// espacios raros.
//
// Que el rango sea estrecho no deja fuera a nadie real: una contraseña que
// contenga un emoji o un salto de línea prácticamente no existe, y si existiera
// se puede cambiar. Un rango ancho deja entrar cualquier basura.
func esLegacyPlano(stored string) bool {
	if len(stored) < 6 || len(stored) > 64 {
		return false
	}
	for i := 0; i < len(stored); i++ {
		c := stored[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == '-', c == '@', c == '#', c == '!', c == '+':
		default:
			return false
		}
	}
	return true
}

// md5hex (el md5() de PHP del esquema heredado) vive en md5compat.go, que es
// donde ya estaba. No se duplica aquí: dos copias de la misma función
// divergen, y una que calcula el hash de una contraseña divergiendo significa
// usuarios que no pueden entrar.

// verificarPassword comprueba una contraseña contra lo guardado.
//
// Devuelve además si la fila necesita reescribirse a bcrypt. El llamador usa
// ese segundo valor para arreglar la fila en el acto, que es la única vez que
// se puede.
func verificarPassword(stored, password string) (ok bool, necesitaRehash bool) {
	switch clasificarPassword(stored) {
	case formatoBcrypt:
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil, false

	case formatoMD5Prefijado:
		// Los dos esquemas del heredado: md5('gef'.pass) y md5(pass). Se
		// prueban los dos porque la base tiene filas de las dos clases y
		// adivinar cuál es cuál sería otra suposición.
		a := md5hex("gef" + password)
		b := md5hex(password)
		if igualesEnTiempoConstante(a, stored) || igualesEnTiempoConstante(b, stored) {
			return true, true
		}
		return false, false

	case formatoPlano:
		// Solo si coincide EXACTA. Comparación en tiempo constante para no
		// filtrar cuántos caracteres van acertando.
		if igualesEnTiempoConstante(stored, password) {
			return true, true
		}
		return false, false

	default:
		// Formato desconocido: no se acepta. Preferible rechazar a alguien
		// raro que dejar entrar a quien tenga un formato que no entendemos.
		return false, false
	}
}

// igualesEnTiempoConstante compara dos cadenas sin filtrar por el tiempo.
func igualesEnTiempoConstante(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// reescribirPassword guarda el hash bcrypt de una contraseña que acabamos de
// verificar. Es lo que convierte una fila heredada en una fila correcta sin que
// el usuario tenga que hacer nada.
//
// userID llega como any porque user_login.user_id es un entero en unas tablas y
// una cadena en otras (las migradas del PHP). Se pasa tal cual lo devuelve la
// consulta en vez de forzar un tipo y fallar en la mitad de las instalaciones.
//
// Si falla, NO es un problema del login: el usuario ya ha entrado. Se avisa en el
// log y se sigue, porque devolver un error aquí dejaría a alguien fuera de su
// propia aplicación por un fallo al escribir una mejora.
func reescribirPassword(db *sql.DB, userID any, username, password string) error {
	if password == "" {
		return fmt.Errorf("no hay contraseña que reescribir")
	}
	hash, err := bcryptGenerate([]byte(password))
	if err != nil {
		return fmt.Errorf("no se pudo generar el hash: %w", err)
	}
	if _, err := db.Exec(`UPDATE user_login SET password=? WHERE user_id=?`, string(hash), userID); err != nil {
		return fmt.Errorf("no se pudo actualizar la contraseña de %s: %w", username, err)
	}
	return nil
}
