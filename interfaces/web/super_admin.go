package web

// super_admin.go — El super administrador es uno solo, nace en la instalación
// y no se reparte desde el panel.
//
// POR QUÉ ESTO EXISTE.
//
// El panel de usuarios ofrecía «Super Admin» en el desplegable de rol de
// seguridad, y el de roles dejaba crear o editar un rol con nivel «master». Con
// eso, cualquier administrador de empresa con permiso de usuarios podía
// fabricarse un segundo super administrador: esa identidad abre TODAS las
// empresas (canAccessTenant y esUsuarioMaster lo resuelven por nivel master), así
// que la separación entre empresas se caía por el formulario de alta, no por un
// fallo de datos.
//
// La regla es la del dueño del sistema: un único super administrador, creado
// cuando se instala, invisible para el panel como opción asignable, y con una
// salida explícita para recuperar su credencial: el binario, en modo --setup. Sin
// esa salida, perder esa clave dejaría la instalación sin dueño; con el panel como
// salida, la llave estaría al alcance de cualquier administrador, que es lo que se
// está corrigiendo.
//
// SE IDENTIFICA POR NIVEL, NO POR ID. El rol reservado es el que tiene
// level='master', sea cual sea su id. Atarlo al id 1 rompería instalaciones
// heredadas donde ese id significa «admin»: convertirlo en master le daría acceso
// a todas las empresas a quien solo era administrador de la suya. El asistente de
// instalación crea el rol master con id 1, pero eso es una consecuencia de
// sembrarlo, no la regla.
//
// Las tres reglas, en corto:
//
//  1. el rol master no se crea, no se edita y no se asigna desde el panel;
//  2. su matriz de permisos no se toca: el nivel master ya concede todo;
//  3. si datos heredados dejaron más de un usuario con ese rol, al arrancar queda
//     el más antiguo y al resto se les retira (se avisa en el log). Si no hay
//     ninguno NO se crea aquí: eso es la instalación o --setup.

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// nivelSuperAdmin es el `level` que concede acceso a todas las empresas.
const nivelSuperAdmin = "master"

// errSuperAdminReservado es el error único que ven los handlers cuando alguien
// intenta crear, editar o asignar el rol del super administrador desde la web.
// El texto nombra la salida correcta para que el administrador no se quede
// buscando una pantalla que no existe (ni debe existir).
var errSuperAdminReservado = errors.New(
	"el rol de super administrador está reservado: se crea solo en la instalación " +
		"y su credencial se restablece con el binario en modo --setup")

// esRolReservado responde por el NIVEL del rol. No por id: ver la cabecera.
func esRolReservado(nivel string) bool {
	return strings.EqualFold(strings.TrimSpace(nivel), nivelSuperAdmin)
}

// usuarioTieneRolSuperAdmin pregunta si ese usuario lleva el rol reservado en el
// catálogo de control. Es la única fuente que decide el nivel master en el login
// (authenticateUser cae a control), así que es la que hay que consultar antes de
// dejar tocar una identidad desde el panel.
func usuarioTieneRolSuperAdmin(ctl *sql.DB, userID string) bool {
	if ctl == nil || strings.TrimSpace(userID) == "" {
		return false
	}
	var n int
	err := ctl.QueryRow(`SELECT COUNT(*)
		FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid
		WHERE ur.user_id = ? AND r.level = ?`, userID, nivelSuperAdmin).Scan(&n)
	return err == nil && n > 0
}

// rolElegidoParaUsuario valida el rol que llega del formulario y devuelve su id
// (0 significa «sin rol», que se conserva como opción legítima: un usuario sin
// rol queda en solo lectura, que es un estado válido, no un error).
//
// El rechazo del rol reservado vive aquí y no en el handler para que también lo
// usen los caminos que no pasan por el desplegable (una petición fabricada a
// mano, la pantalla heredada de permisos). Un rol que no existe se rechaza en
// vez de guardarse: una fila de sec_userrole apuntando a la nada deja al usuario
// sin nivel y sin explicación.
func rolElegidoParaUsuario(ctl *sql.DB, valor string) (int64, error) {
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(valor, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("rol inválido: %q", valor)
	}
	var nivel string
	if err := ctl.QueryRow(`SELECT IFNULL(level,'') FROM sec_role WHERE id = ?`, id).Scan(&nivel); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("el rol %d no existe en el catálogo", id)
		}
		return 0, fmt.Errorf("leyendo el rol %d: %w", id, err)
	}
	if esRolReservado(nivel) {
		return 0, errSuperAdminReservado
	}
	return id, nil
}

// rechazarRolReservado es la comprobación para los caminos que reciben un rol
// ya existente (asignarlo a una persona, editar su matriz de permisos): si su
// nivel es master, se rechaza con el mismo mensaje. Toma un *sql.DB porque la
// pantalla heredada de permisos escribe en la base de la empresa; el catálogo de
// roles está espejado en las dos.
func rechazarRolReservado(db *sql.DB, valor string) error {
	id, err := strconv.ParseInt(strings.TrimSpace(valor), 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("rol inválido: %q", valor)
	}
	if db == nil {
		return nil
	}
	var nivel string
	if err := db.QueryRow(`SELECT IFNULL(level,'') FROM sec_role WHERE id = ?`, id).Scan(&nivel); err == nil &&
		esRolReservado(nivel) {
		return errSuperAdminReservado
	}
	return nil
}

// rolesAsignablesParaUsuario es la lista del desplegable: todos los roles del
// catálogo menos el del super administrador. Filtrar en la consulta y no en la
// plantilla importa: si el rol no llega al navegador, no hay nada que un
// formulario manipulado pueda reenviar.
func (s *Server) rolesAsignablesParaUsuario(r *http.Request) []map[string]interface{} {
	rows, err := s.authDB(r).Query(`SELECT id, type FROM sec_role
		WHERE IFNULL(level,'') <> ?
		ORDER BY id`, nivelSuperAdmin)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var lista []map[string]interface{}
	for rows.Next() {
		var id int64
		var tipo string
		if err := rows.Scan(&id, &tipo); err != nil {
			continue
		}
		lista = append(lista, map[string]interface{}{"ID": id, "Type": tipo})
	}
	return lista
}

// normalizarSuperAdmin deja el catálogo en el estado que el sistema espera: el
// rol master existe y está marcado como del sistema, y hay UNA sola identidad con
// él.
//
// Se ejecuta en cada arranque porque el estado pudo quedar mal por datos
// heredados (antes el panel permitía asignarlo). Cuando hay varias identidades,
// se conserva la MÁS ANTIGUA (la primera fila de sec_userrole): es la que creó la
// instalación, y elegir otra convertiría el arreglo en un cambio de dueño.
//
// No crea ninguna identidad si no hay super administrador. Eso no es un olvido:
// crearlo aquí, con una clave que nadie eligió, volvería a repartir la llave del
// sistema; la puerta correcta es --setup, donde una persona escribe la clave.
//
// Tampoco convierte en master un rol que no lo era: si la instalación no tiene
// ningún nivel master, la fila llamada «Super Admin» se recupera (una edición
// anterior pudo degradarla) y, si tampoco existe, se avisa sin tocar nada.
func normalizarSuperAdmin(ctl *sql.DB) error {
	if ctl == nil {
		return fmt.Errorf("sin base de control no se puede comprobar el super administrador")
	}

	rolMaster, err := rolMasterExistente(ctl)
	if err != nil {
		return err
	}
	if rolMaster == 0 {
		log.Printf("AVISO: la instalación no tiene rol de super administrador; " +
			"cree la credencial con el binario en modo --setup")
		return nil
	}
	if _, err := ctl.Exec(`UPDATE sec_role SET level = ?, is_system = 1 WHERE id = ?`,
		nivelSuperAdmin, rolMaster); err != nil {
		return fmt.Errorf("marcando el rol del super administrador: %w", err)
	}

	type asignacion struct {
		fila     int64
		userID   string
		username string
	}
	// Todos los roles con nivel master, no solo el elegido: una instalación
	// heredada puede tener más de uno y todos conceden lo mismo.
	rows, err := ctl.Query(`SELECT ur.id, ur.user_id, IFNULL(ul.username,'')
		FROM sec_userrole ur
		JOIN sec_role r ON r.id = ur.roleid
		LEFT JOIN user_login ul ON ul.user_id = ur.user_id
		WHERE r.level = ?
		ORDER BY CASE WHEN LOWER(r.type) = 'super admin' THEN 0 ELSE 1 END, ur.id`, nivelSuperAdmin)
	if err != nil {
		return fmt.Errorf("leyendo quién tiene el rol del super administrador: %w", err)
	}
	var asignadas []asignacion
	for rows.Next() {
		var a asignacion
		if err := rows.Scan(&a.fila, &a.userID, &a.username); err != nil {
			rows.Close()
			return fmt.Errorf("leyendo quién tiene el rol del super administrador: %w", err)
		}
		asignadas = append(asignadas, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(asignadas) == 0 {
		log.Printf("AVISO: no hay ningún usuario con el rol de super administrador; " +
			"restablézcalo con el binario en modo --setup")
		return nil
	}

	keep := asignadas[0]
	if strings.TrimSpace(keep.userID) == "" {
		return fmt.Errorf("el super administrador tiene una fila de rol sin user_id (sec_userrole.id=%d)", keep.fila)
	}
	// La fila que se conserva se normaliza al rol master elegido: si venía de otro
	// rol master, queda en el que el sistema reconoce.
	if _, err := ctl.Exec(`UPDATE sec_userrole SET roleid = ? WHERE id = ?`, rolMaster, keep.fila); err != nil {
		return fmt.Errorf("normalizando el rol del super administrador: %w", err)
	}
	for _, extra := range asignadas[1:] {
		if _, err := ctl.Exec(`DELETE FROM sec_userrole WHERE id = ?`, extra.fila); err != nil {
			return fmt.Errorf("retirando el rol de super administrador a %q: %w", extra.username, err)
		}
		log.Printf("AVISO: %q tenía el rol de super administrador y se le retiró: "+
			"el super administrador de la instalación es uno solo (%q)", extra.username, keep.username)
	}
	return nil
}

// rolMasterExistente devuelve el id del rol con nivel master. Si no hay ninguno,
// intenta recuperar la fila llamada «Super Admin» (el nivel se pudo degradar en
// una edición) y la deja como master. Cero significa que no hay forma segura de
// saber cuál es: no se inventa uno.
func rolMasterExistente(ctl *sql.DB) (int64, error) {
	var id int64
	err := ctl.QueryRow(`SELECT id FROM sec_role WHERE level = ? ORDER BY id LIMIT 1`, nivelSuperAdmin).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("leyendo el rol del super administrador: %w", err)
	}
	err = ctl.QueryRow(`SELECT id FROM sec_role WHERE LOWER(type) = 'super admin' ORDER BY id LIMIT 1`).Scan(&id)
	if err == nil {
		if _, err := ctl.Exec(`UPDATE sec_role SET level = ?, is_system = 1 WHERE id = ?`,
			nivelSuperAdmin, id); err != nil {
			return 0, fmt.Errorf("recuperando el nivel del rol de super administrador: %w", err)
		}
		return id, nil
	}
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return 0, fmt.Errorf("leyendo el rol del super administrador: %w", err)
}

// asegurarSuperAdminUnico es la llamada de arranque. Un fallo NO impide levantar
// el ERP: se registra, porque negarse a arrancar por un catálogo de roles
// ilegible sería cambiar un aviso por una caída.
func (s *Server) asegurarSuperAdminUnico() error {
	ctl, err := s.controlStore()
	if err != nil {
		return err
	}
	return normalizarSuperAdmin(ctl)
}
