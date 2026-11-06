package web

// usuarios_empresa.go — De qué empresa es cada usuario, y cómo se le da de alta
// en la empresa correcta.
//
// POR QUÉ ESTO EXISTE.
//
// El alta de usuarios (`userSave`) creaba la identidad en la base de control
// —que es donde el login la busca— y después la inscribía en una empresa con
//
//	SELECT code FROM tenants WHERE status = 1 ORDER BY id LIMIT 1
//
// Es decir: el primer tenant activo, que en una instalación con la empresa DEMO
// encendida es la maestra o la demo, nunca la empresa desde la que el
// administrador estaba trabajando. El usuario nacía en el sitio equivocado y,
// como la lista de Usuarios no filtraba por empresa, aparecía en todas: daba la
// impresión de que la identidad «saltaba» de una empresa a otra.
//
// La identidad es global (así la resolvió identity.go: un nombre de usuario,
// una credencial, varias empresas), pero la PERTENENCIA es por empresa. Este
// archivo concentra las dos operaciones que el panel necesita para no confundir
// una con la otra: inscribir en la empresa activa y saber si alguien pertenece a
// ella. La empresa activa sale de la sesión, no de una consulta que ordena por
// id, para que el alta ocurra donde el administrador está mirando.

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// empresaDeLaSesion devuelve el código de la empresa en la que el administrador
// está trabajando. Es la única fuente válida para inscribir un usuario nuevo.
func (s *Server) empresaDeLaSesion(r *http.Request) (string, error) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		return "", fmt.Errorf("sesión no válida")
	}
	empresa := strings.TrimSpace(s.tenantOfSession(sess))
	if empresa == "" {
		return "", fmt.Errorf("no se pudo determinar la empresa activa")
	}
	return empresa, nil
}

// usuarioEsMiembroDeEmpresa responde si el usuario pertenece a esa empresa.
//
// Se consultan las DOS fuentes de pertenencia, en este orden:
//
//   - `tenant_users` (control): es la que usan el login, el selector de empresa y
//     canAccessTenant. Es la autoridad.
//   - `user_company_access` (control): la que escribía el alta anterior. Se mira
//     como compatibilidad para que los usuarios ya creados no desaparezcan de su
//     empresa mientras la instalación no se repara.
//
// Un usuario sin fila en ninguna de las dos no pertenece a ninguna empresa y el
// panel no debe poder editarlo ni borrarlo desde una empresa ajena.
func usuarioEsMiembroDeEmpresa(ctl *sql.DB, userID, empresa string) bool {
	if ctl == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(empresa) == "" {
		return false
	}
	var n int
	if err := ctl.QueryRow(`SELECT COUNT(*)
		FROM user_login ul
		JOIN tenant_users tu ON tu.username = ul.username
		WHERE ul.user_id = ? AND tu.tenant_code = ? AND tu.status = 1`,
		userID, empresa).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM user_company_access
		WHERE user_id = ? AND tenant_code = ?`, userID, empresa).Scan(&n); err == nil && n > 0 {
		return true
	}
	return false
}

// empresasDelUsuario lista las empresas a las que pertenece, sin repetir y en
// orden alfabético. Se usa para distinguir «quitarle el acceso a esta empresa»
// de «eliminar la identidad»: si pertenece a otras, borrar la fila global le
// quitaría el acceso a todas, que es justo el salto entre empresas que se está
// corrigiendo.
func empresasDelUsuario(ctl *sql.DB, userID string) []string {
	if ctl == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	vistas := map[string]bool{}
	agregar := func(rows *sql.Rows, err error) {
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			if rows.Scan(&code) == nil && strings.TrimSpace(code) != "" {
				vistas[code] = true
			}
		}
	}
	agregar(ctl.Query(`SELECT tu.tenant_code FROM tenant_users tu
		JOIN user_login ul ON ul.username = tu.username
		WHERE ul.user_id = ? AND tu.status = 1`, userID))
	agregar(ctl.Query(`SELECT DISTINCT tenant_code FROM user_company_access WHERE user_id = ?`, userID))

	out := make([]string, 0, len(vistas))
	for code := range vistas {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// vincularUsuarioAEmpresaTx inscribe al usuario en la empresa indicada, en las
// dos tablas de pertenencia, dentro de la transacción que crea la identidad.
//
// Va en la MISMA transacción a propósito: un usuario que existe pero no
// pertenece a ninguna empresa no puede entrar a ninguna parte, y un alta que
// confirma la identidad y falla la pertenencia deja exactamente ese estado.
//
// No borra pertenencias anteriores: inscribir no es mover. Un usuario puede
// trabajar en varias empresas (esa es la razón de que la identidad sea global) y
// el panel no debe decidir por él al darle de alta en una más.
func vincularUsuarioAEmpresaTx(tx *sql.Tx, userID, username, empresa string, roleID int64) error {
	if tx == nil {
		return fmt.Errorf("sin transacción no se puede inscribir al usuario en la empresa")
	}
	empresa = strings.TrimSpace(empresa)
	if empresa == "" {
		return fmt.Errorf("sin empresa no se puede inscribir al usuario")
	}

	var existe int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM tenant_users WHERE tenant_code = ? AND username = ?`,
		empresa, username).Scan(&existe); err != nil {
		return fmt.Errorf("comprobando la pertenencia de %q a %q: %w", username, empresa, err)
	}
	if existe == 0 {
		if _, err := tx.Exec(`INSERT INTO tenant_users (tenant_code, username, role_id, is_owner, status, created_at)
			VALUES (?,?,?,0,1,?)`, empresa, username, roleID, now()); err != nil {
			return fmt.Errorf("inscribiendo a %q en la empresa %q: %w", username, empresa, err)
		}
	}

	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_company_access WHERE user_id = ? AND tenant_code = ?`,
		userID, empresa).Scan(&existe); err != nil {
		return fmt.Errorf("comprobando el acceso de %q a %q: %w", userID, empresa, err)
	}
	if existe == 0 {
		if _, err := tx.Exec(`INSERT INTO user_company_access (user_id, tenant_code, role_id) VALUES (?,?,?)`,
			userID, empresa, roleID); err != nil {
			return fmt.Errorf("registrando el acceso de %q a la empresa %q: %w", userID, empresa, err)
		}
	}
	return nil
}

// desvincularUsuarioDeEmpresaTx quita al usuario de UNA empresa, sin tocar su
// identidad ni sus otras empresas.
func desvincularUsuarioDeEmpresaTx(tx *sql.Tx, userID, username, empresa string) error {
	if _, err := tx.Exec(`DELETE FROM tenant_users WHERE tenant_code = ? AND username = ?`,
		empresa, username); err != nil {
		return fmt.Errorf("quitando a %q de la empresa %q: %w", username, empresa, err)
	}
	if _, err := tx.Exec(`DELETE FROM user_company_access WHERE user_id = ? AND tenant_code = ?`,
		userID, empresa); err != nil {
		return fmt.Errorf("quitando el acceso de %q a la empresa %q: %w", userID, empresa, err)
	}
	return nil
}
