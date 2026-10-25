package web

// first_run.go — La clave de fábrica no puede pasar desapercibida.
//
// El admin arranca con admin/Admin@2026 porque sin él no hay instalación posible.
// Eso está bien para el que la instala en su máquina y es un agujero para el que
// lo expose a una red sin cambiarlo.
//
// La regla que se aplica: avisar, no bloquear. Bloquear el primer arranque sin
// base de datos es imposible (no hay dónde guardar la clave nueva), y bloquear
// solo en producción dejaría al usuario de siempre sin entrada si algo sale mal.
// Lo que no es negociable es que el aviso SEA visible en la pantalla: printed en
// la consola no lo ve nadie.

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// factoryPasswords son las claves conocidas que trae una instalación nueva.
// Se comparan en tiempo constante, igual que un hash, para no filtrar nada por
// temporización. No es un hash: es una lista corta y pública, y la comparacion con
// tiempo constante evita el caso de que alguien mida cuánto tarda.
var factoryPasswords = []string{"Admin@2026"}

// usesFactoryPassword indica si el usuario conserva una clave de fábrica.
//
// Se compara contra el HASH con bcrypt en vez de contra el texto: así el
// resultado no depende de si la clave se guardó en bcrypt o en el MD1 heredado
// del PHP.
func usesFactoryPassword(db *sql.DB, username string) bool {
	var hash string
	err := db.QueryRow(`SELECT password FROM user_login WHERE username = ? LIMIT 1`, username).Scan(&hash)
	if err != nil {
		return false
	}
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		for _, p := range factoryPasswords {
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil {
				return true
			}
		}
		return false
	}
	// Heredado del PHP: md5('gef' + clave).
	for _, p := range factoryPasswords {
		if subtle.ConstantTimeCompare([]byte(md5hex("gef"+p)), []byte(hash)) == 1 {
			return true
		}
	}
	return false
}

// factoryPasswordWarning es el texto que ve el usuario. Dice qué hacer y dónde,
// en lugar de solo advertir: un aviso sin siguiente paso se lee y se ignora.
const factoryPasswordWarning = "Su usuario todavía usa la contraseña de fábrica. " +
	"Cámbiela ahora en Editar Perfil: con la clave actual, cualquiera que " +
	"conozca la dirección del sistema puede entrar como administrador."

// validatePasswordStrength aplica el mínimo de una contraseña decente.
//
// Antes eran 6 caracteres. Seis es la longitud de un número de teléfono y de un
// PIN de cajero: con esa longitud, un ataque de fuerza bruta por diccionario
// termina en segundos. El mínimo son 8 caracteres y una letra con un número, que
// es lo que hace que "Admin@2026" no sea adivinable por forma.
//
// La regla es deliberadamente simple y explicable al usuario. Un medidor de
// "fuerza" con barras verdes es mejor UX pero aquí sobraría: el objetivo es que
// la clave no sea adivinable, y eso se explica en una frase.
func validatePasswordStrength(p string) error {
	if len(p) < 8 {
		return fmt.Errorf("La contraseña debe tener al menos 8 caracteres")
	}
	var tieneLetra, tieneNumero bool
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			tieneLetra = true
		case r >= '0' && r <= '9':
			tieneNumero = true
		}
	}
	if !tieneLetra || !tieneNumero {
		return fmt.Errorf("La contraseña debe combinar al menos una letra y un número")
	}
	return nil
}

// changeUserPassword verifica la clave actual y guarda la nueva. Vive fuera del
// handler para que el cambio de clave se pueda probar y reutilizar sin pasar por
// una petición HTTP.
func changeUserPassword(db *sql.DB, username, current, next string) error {
	var hash string
	if err := db.QueryRow(`SELECT password FROM user_login WHERE username = ?`, username).Scan(&hash); err != nil {
		return err
	}
	valida := false
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		valida = bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) == nil
	} else {
		valida = md5hex("gef"+current) == hash || md5hex(current) == hash
	}
	if !valida {
		return fmt.Errorf("la contraseña actual no es correcta")
	}
	nuevo, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE user_login SET password = ? WHERE username = ?`, string(nuevo), username)
	return err
}
