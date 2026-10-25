package web

// view_pref.go — Vistas y plantillas por usuario.
//
// Cada persona ve el ERP distinto: unos quieren el panel de control completo,
// otros solo lo que está pendiente de cobrar. Forzar a todos la misma
// disposición es la vía rápida para que cada uno se la arme a mano, que es lo
// que termina pasando.
//
// Aquí se guarda esa disposición. La decisión de dónde vive (la base de la
// EMPRESA, no la de control) y por qué está razonada en la migración
// schema_z_user_view_pref.sql; lo que sigue es el código que la usa.
//
// Dos conceptos, una sola tabla:
//   - la vista ACTIVA (name vacío): lo que el usuario tiene puesto ahora.
//   - una plantilla guardada (con name): "solo pendientes", "resumen del día".
//
// El servidor no interpreta el contenido: `payload` es un objeto JSON que se
// guarda y se devuelve tal cual. Guardarlo opaco a propósito permite que la
// pantalla evolucione (nuevas casillas, nuevos filtros) sin migración, y
// evita que el sistema afirme entender algo que en realidad no entiende.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	// maxViewPrefPayload acota lo que se guarda. Es una disposición de
	// pantalla: si no cabe en 64 KiB no es una disposición, es un archivo, y
	// la base es para lo primero.
	maxViewPrefPayload = 64 * 1024
	// maxViewPrefName acota el nombre de la plantilla.
	maxViewPrefName = 80
	// maxViewPrefKey acota la clave de la pantalla.
	maxViewPrefKey = 80
)

// Errores de validación. Se exportan como errores propios para que los tests
// puedan afirmar qué se rechazó y por qué, en vez de comparar textos.
var (
	ErrViewKeyInvalido    = errors.New("clave de pantalla invalida")
	ErrViewNombreInvalido = errors.New("nombre de vista invalido")
	ErrViewPayloadVacio   = errors.New("la vista no trae contenido")
	ErrViewPayloadGrande  = errors.New("la vista es demasiado grande")
	ErrViewNoJSON         = errors.New("la vista debe ser un objeto JSON")
)

// ViewPref es una disposición guardada de una pantalla.
type ViewPref struct {
	Username  string          `json:"username"`
	ViewKey   string          `json:"view_key"`
	Name      string          `json:"name"`
	Payload   json.RawMessage `json:"payload"`
	UpdatedAt string          `json:"updated_at"`
}

// Etiqueta de la vista, para la lista de plantillas del selector.
func (v ViewPref) Label() string { return v.Name }

// ValidViewKey normaliza y comprueba la clave de pantalla.
//
// La clave viaja en la URL, así que se limita a un alfabeto corto en vez de
// intentar sanearla: "invoice/list" es una clave legítima y "../otro" no tiene
// ninguna interpretación que necesitemos. Rechazarla es más claro que
// arreglarla.
func ValidViewKey(key string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" || len(k) > maxViewPrefKey {
		return "", ErrViewKeyInvalido
	}
	for _, c := range k {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == '-':
		default:
			return "", ErrViewKeyInvalido
		}
	}
	return k, nil
}

// ValidViewName normaliza y comprueba el nombre de una plantilla guardada.
//
// El nombre vacío es legítimo y significa "la vista activa", no es un error.
func ValidViewName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if len(n) > maxViewPrefName {
		return "", ErrViewNombreInvalido
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return "", ErrViewNombreInvalido
		}
	}
	return n, nil
}

// ValidViewPayload comprueba que lo que se guarda es un objeto JSON de tamaño
// razonable.
//
// Que sea un objeto y no un arreglo o un escalar no es arbitrario: quien lo
// consume lo fusiona con los valores por defecto de la pantalla. Un arreglo no
// se fusiona, así que aceptarlo produciría silenciosamente una vista vacía.
func ValidViewPayload(payload string) (string, error) {
	p := strings.TrimSpace(payload)
	if p == "" {
		return "", ErrViewPayloadVacio
	}
	if len(p) > maxViewPrefPayload {
		return "", ErrViewPayloadGrande
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(p), &obj); err != nil || obj == nil {
		return "", ErrViewNoJSON
	}
	return p, nil
}

// LoadViewPref devuelve una vista concreta del usuario. found es false cuando
// el usuario nunca guardó esa vista, que es lo normal y no es un error.
func LoadViewPref(db *sql.DB, username, viewKey, name string) (v ViewPref, found bool, err error) {
	v = ViewPref{Username: username, ViewKey: viewKey, Name: name}
	var payload string
	err = db.QueryRow(`SELECT payload, updated_at FROM user_view_pref
		WHERE username = ? AND view_key = ? AND name = ?`, username, viewKey, name).
		Scan(&payload, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ViewPref{}, false, nil
	}
	if err != nil {
		return ViewPref{}, false, err
	}
	v.Payload = json.RawMessage(payload)
	return v, true, nil
}

// ListSavedViewPrefs devuelve las plantillas guardadas de una pantalla por el
// usuario, de la más reciente a la más antigua.
func ListSavedViewPrefs(db *sql.DB, username, viewKey string) ([]ViewPref, error) {
	rows, err := db.Query(`SELECT name, payload, updated_at FROM user_view_pref
		WHERE username = ? AND view_key = ? AND name <> ''
		ORDER BY updated_at DESC, name ASC`, username, viewKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ViewPref{}
	for rows.Next() {
		var (
			v       ViewPref
			payload string
		)
		if err := rows.Scan(&v.Name, &payload, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Username = username
		v.ViewKey = viewKey
		v.Payload = json.RawMessage(payload)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveViewPref guarda o reemplaza una vista del usuario.
//
// Reemplazar en vez de acumular es lo que hace que "guardar" signifique lo que
// alguien espera: si vuelve a guardar la plantilla con el mismo nombre, obtains
// la que acabas de armar, no dos plantillas peleándose por ese nombre.
func SaveViewPref(db *sql.DB, username, viewKey, name, payload string) error {
	if username == "" {
		return errors.New("usuario vacio")
	}
	p, err := ValidViewPayload(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO user_view_pref
			(username, view_key, name, payload, updated_at)
		VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE payload = VALUES(payload), updated_at = VALUES(updated_at)`,
		username, viewKey, name, p, now())
	return err
}

// DeleteViewPref borra una vista del usuario.
//
// Solo se borra lo que el usuario es dueño: la consulta lleva el username en el
// WHERE, no solo el nombre. Sin eso, un POST con action=delete y el nombre de
// otra plantilla borraría la ajena.
func DeleteViewPref(db *sql.DB, username, viewKey, name string) error {
	res, err := db.Exec(`DELETE FROM user_view_pref
		WHERE username = ? AND view_key = ? AND name = ?`, username, viewKey, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no existe la vista %q", name)
	}
	return nil
}
