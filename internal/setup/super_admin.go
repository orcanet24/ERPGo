package setup

// super_admin.go — Recuperar la credencial del super administrador desde el
// binario, que es el único sitio desde donde se puede.
//
// POR QUÉ AQUÍ Y NO EN EL PANEL.
//
// El super administrador es el dueño del sistema: su nivel master abre todas las
// empresas. Si el panel pudiera crear otro o cambiarle la clave, la separación
// entre empresas dependería de quién tenga permiso de usuarios, y cualquier
// administrador podría fabricarse el acceso total. Por eso el panel lo muestra
// pero no lo toca (interfaces/web/super_admin.go) y la salida es esta: una
// persona con acceso al servidor ejecuta el binario en modo --setup y elige la
// clave nueva. Perder la clave deja de ser un bloqueo definitivo sin convertir
// la llave del sistema en un formulario más.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// nivelSuperAdmin concede acceso a todas las empresas. Es el nivel, no un id:
// ver la explicación en interfaces/web/super_admin.go.
const nivelSuperAdmin = "master"

// ErrSinSuperAdmin se devuelve cuando la instalación no tiene ninguna identidad
// con el rol reservado y no se indicó un usuario para crearla.
var ErrSinSuperAdmin = errors.New("la instalación no tiene super administrador: indique el usuario a crear")

// SuperAdminActual devuelve el usuario que hoy tiene el rol reservado. Cadena
// vacía significa que no hay ninguno (o que la tabla no existe todavía).
func SuperAdminActual(db *sql.DB) (string, error) {
	if db == nil {
		return "", fmt.Errorf("sin base de control no se puede buscar al super administrador")
	}
	var username string
	err := db.QueryRow(`SELECT IFNULL(ul.username,'')
		FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid
		LEFT JOIN user_login ul ON ul.user_id = ur.user_id
		WHERE r.level = ?
		ORDER BY CASE WHEN LOWER(r.type) = 'super admin' THEN 0 ELSE 1 END, ur.id LIMIT 1`, nivelSuperAdmin).Scan(&username)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("buscando al super administrador: %w", err)
	}
	return username, nil
}

// RestablecerCredencialSuperAdmin deja la credencial del super administrador con
// la clave indicada y devuelve el nombre de usuario que quedó con ese rol.
//
// Reglas:
//
//   - si ya hay un super administrador, se restablece SU clave. El usuario que
//     llegue por parámetro se ignora a propósito: aceptar otro nombre sería
//     promover a una segunda identidad, que es lo que el sistema no permite;
//   - si no hay ninguno y se indica un usuario, se crea (o se reutiliza esa
//     identidad) y se le asigna el rol reservado. Es el camino de una instalación
//     cuya fila de rol se perdió, no una vía para repartirlo: para eso hace falta
//     acceso al servidor y ejecutar el binario;
//   - si hay varios (datos heredados), se conserva el más antiguo y a los demás
//     se les retira el rol, igual que hace el arranque del servidor.
func RestablecerCredencialSuperAdmin(db *sql.DB, empresa, usuario, clave string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("sin base de control no se puede restablecer la credencial")
	}
	clave = strings.TrimSpace(clave)
	if clave == "" {
		return "", fmt.Errorf("la contraseña no puede quedar vacía")
	}
	empresa = strings.TrimSpace(empresa)

	rolMaster, err := asegurarRolSuperAdmin(db)
	if err != nil {
		return "", err
	}

	type asignacion struct {
		fila     int64
		userID   string
		username string
	}
	rows, err := db.Query(`SELECT ur.id, ur.user_id, IFNULL(ul.username,'')
		FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid
		LEFT JOIN user_login ul ON ul.user_id = ur.user_id
		WHERE r.level = ?
		ORDER BY CASE WHEN LOWER(r.type) = 'super admin' THEN 0 ELSE 1 END, ur.id`, nivelSuperAdmin)
	if err != nil {
		return "", fmt.Errorf("leyendo quién tiene el rol de super administrador: %w", err)
	}
	var asignadas []asignacion
	for rows.Next() {
		var a asignacion
		if err := rows.Scan(&a.fila, &a.userID, &a.username); err != nil {
			rows.Close()
			return "", fmt.Errorf("leyendo quién tiene el rol de super administrador: %w", err)
		}
		asignadas = append(asignadas, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	userID := ""
	username := ""
	if len(asignadas) > 0 {
		userID = asignadas[0].userID
		username = asignadas[0].username
		for _, extra := range asignadas[1:] {
			if _, err := db.Exec(`DELETE FROM sec_userrole WHERE id = ?`, extra.fila); err != nil {
				return "", fmt.Errorf("retirando el rol de super administrador a %q: %w", extra.username, err)
			}
			fmt.Printf("   AVISO: %q tenía el rol de super administrador y se le retiró (queda uno solo)\n", extra.username)
		}
		if _, err := db.Exec(`UPDATE sec_userrole SET roleid = ? WHERE id = ?`, rolMaster, asignadas[0].fila); err != nil {
			return "", fmt.Errorf("normalizando el rol del super administrador: %w", err)
		}
	}

	creando := false
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(username) == "" {
		username = strings.TrimSpace(usuario)
		if username == "" {
			return "", ErrSinSuperAdmin
		}
		creando = true
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(clave), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("cifrando la contraseña: %w", err)
	}

	if creando {
		// La identidad global: si el nombre ya existe se reutiliza, no se duplica
		// (user_login.username es único y el login resuelve por ese nombre).
		err := db.QueryRow(`SELECT user_id FROM user_login WHERE username = ? LIMIT 1`, username).Scan(&userID)
		switch {
		case err == sql.ErrNoRows:
			if err := crearIdentidadSuperAdmin(db, username, string(hash)); err != nil {
				return "", err
			}
			if err := db.QueryRow(`SELECT user_id FROM user_login WHERE username = ? LIMIT 1`, username).Scan(&userID); err != nil {
				return "", fmt.Errorf("leyendo la identidad recién creada de %q: %w", username, err)
			}
		case err != nil:
			return "", fmt.Errorf("buscando la identidad de %q: %w", username, err)
		}
		if _, err := db.Exec(`DELETE FROM sec_userrole WHERE user_id = ?`, userID); err != nil {
			return "", fmt.Errorf("limpiando el rol anterior de %q: %w", username, err)
		}
		if _, err := db.Exec(`INSERT INTO sec_userrole (user_id, roleid, createby, createdate)
			VALUES (?, ?, 'setup', ?)`, userID, rolMaster, ahora()); err != nil {
			return "", fmt.Errorf("asignando el rol de super administrador a %q: %w", username, err)
		}
	}

	if _, err := db.Exec(`UPDATE user_login SET password = ?, status = 1, user_type = 1 WHERE user_id = ?`,
		string(hash), userID); err != nil {
		return "", fmt.Errorf("guardando la contraseña de %q: %w", username, err)
	}

	// Pertenencia a la empresa maestra: sin ella, el dueño del sistema no vería
	// su propia empresa en el selector aunque su nivel master abra todas.
	if empresa != "" {
		var existe int
		if err := db.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code = ? AND username = ?`,
			empresa, username).Scan(&existe); err != nil {
			return "", fmt.Errorf("comprobando la pertenencia de %q a %q: %w", username, empresa, err)
		}
		if existe == 0 {
			if _, err := db.Exec(`INSERT INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at)
				VALUES (?,?,0,1,1,?)`, empresa, username, ahora()); err != nil {
				return "", fmt.Errorf("inscribiendo a %q como dueño de %q: %w", username, empresa, err)
			}
		}
	}
	return username, nil
}

// crearIdentidadSuperAdmin inserta users + user_login en la base de control. El
// user_id se calcula a mano por el mismo motivo que documenta identity.go: el
// identificador es texto en el esquema heredado y no siempre tiene AUTO_INCREMENT.
func crearIdentidadSuperAdmin(db *sql.DB, username, hash string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var siguiente int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(CAST(user_id AS UNSIGNED)), 0) + 1 FROM users`).Scan(&siguiente); err != nil {
		return fmt.Errorf("calculando el identificador de %q: %w", username, err)
	}
	userID := fmt.Sprint(siguiente)
	if _, err := tx.Exec(`INSERT INTO users (user_id, first_name, last_name, company_name, phone, status)
		VALUES (?, ?, '', 'ERPGo', '', 1)`, userID, username); err != nil {
		return fmt.Errorf("creando el usuario %q en la base de control: %w", username, err)
	}
	if _, err := tx.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
		VALUES (?, ?, ?, 1, 1)`, userID, username, hash); err != nil {
		return fmt.Errorf("creando la credencial de %q: %w", username, err)
	}
	return tx.Commit()
}

// asegurarRolSuperAdmin devuelve el id del rol con nivel master y lo marca como
// del sistema. Si no hay ninguno, recupera la fila llamada «Super Admin» (una
// edición anterior pudo degradarla) o crea el rol.
//
// NO usa el id 1 como regla: hay instalaciones heredadas donde ese id significa
// «admin de empresa», y convertirlo en master le daría acceso a todas las
// empresas a quien solo administraba la suya.
//
// Se resuelve con comprobaciones explícitas en vez de `ON DUPLICATE KEY`, que no
// existe en todas las bases donde corren las pruebas.
func asegurarRolSuperAdmin(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM sec_role WHERE level = ? ORDER BY id LIMIT 1`, nivelSuperAdmin).Scan(&id)
	if err == nil {
		if _, err := db.Exec(`UPDATE sec_role SET is_system = 1 WHERE id = ?`, id); err != nil {
			return 0, fmt.Errorf("marcando el rol de super administrador: %w", err)
		}
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("leyendo el rol de super administrador: %w", err)
	}

	err = db.QueryRow(`SELECT id FROM sec_role WHERE LOWER(type) = 'super admin' ORDER BY id LIMIT 1`).Scan(&id)
	switch {
	case err == nil:
		if _, err := db.Exec(`UPDATE sec_role SET level = ?, is_system = 1 WHERE id = ?`,
			nivelSuperAdmin, id); err != nil {
			return 0, fmt.Errorf("recuperando el rol de super administrador: %w", err)
		}
		return id, nil
	case err != sql.ErrNoRows:
		return 0, fmt.Errorf("buscando el rol de super administrador: %w", err)
	}

	if _, err := db.Exec(`INSERT INTO sec_role (type, level, description, is_system)
		VALUES ('Super Admin', ?, 'Acceso total al sistema', 1)`, nivelSuperAdmin); err != nil {
		return 0, fmt.Errorf("creando el rol de super administrador: %w", err)
	}
	if err := db.QueryRow(`SELECT id FROM sec_role WHERE level = ? ORDER BY id LIMIT 1`, nivelSuperAdmin).Scan(&id); err != nil {
		return 0, fmt.Errorf("leyendo el rol de super administrador recién creado: %w", err)
	}
	return id, nil
}

// ahora es el formato de fecha que usa el esquema heredado (VARCHAR, no DATETIME).
func ahora() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
