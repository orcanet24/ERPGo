package web

// identity.go — Dónde viven los usuarios y sus credenciales. UNA vez, y en un
// solo sitio.
//
// EL DEFECTO QUE ESTE ARCHIVO CIERRA. Había dos caminos para crear una empresa y
// los dos sembraban el administrador en la base DE LA EMPRESA
// (PrepareCompanyAs → InitializeAdminUser), mientras el login lo buscaba en la
// base DE CONTROL (authDB → authenticateUser). El resultado: la empresa se
// creaba, se veía en el catálogo, se registraba la pertenencia en `tenant_users`
// y el administrador no podía entrar. El error que veía el instalador era
// "Credenciales inválidas" con la clave correcta, que es la forma más confusa de
// fallar: apunta al usuario y el problema está en dónde miró el sistema.
//
// DÓNDE VIVEN, Y POR QUÉ AHÍ. En la base de CONTROL, porque es la única que
// responde las tres preguntas que el login hace ANTES de saber en qué empresa
// está el usuario:
//
//   1. ¿Existe este usuario y esta clave?      → user_login (control)
//   2. ¿A qué empresas pertenece?              → tenant_users (control)
//   3. ¿Qué rol tiene?                         → sec_userrole/sec_role (control,
//      con el rol por empresa de la base de la empresa como primera opción)
//
// Por eso la base de control lleva el mismo esquema que una empresa (ver
// controlStore): sin las tablas, la respuesta no es "no lo sé" sino "no existe",
// y no existe es indistinguible de una clave mal escrita.
//
// LA COPIA DE LA EMPRESA. Las bases de empresa siguen teniendo sus filas de
// `users`/`user_login` porque el esquema es un espejo del PHP heredado y las
// pantallas de la empresa (Usuarios, roles, POS) leen de ahí. Lo que NO puede
// pasar es que esa copia sea la que decide quién entra: la autoridad es la de
// control, y el único código que la escribe desde una empresa es esta función.
// Mientras la pantalla de Usuarios no se mueva a control (con el filtro de
// `tenant_users` para no mezclar empresas), esa pantalla cambia la clave de una
// copia que el login no lee, y eso queda anotado como lo que es: la parte que
// falta del mismo trabajo, no un arreglo.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// errIdentidadYaExiste: el nombre de usuario ya es una identidad de la base de
// control y la contraseña que se trae no es la suya.
//
// Es un error con nombre propio porque el alta de empresa necesita distinguirlo
// de un fallo de escritura: un fallo se propaga al instalador, y esto se resuelve
// eligiendo otro nombre de usuario (o entrando con la contraseña que ya existe).
var errIdentidadYaExiste = errors.New("la identidad ya existe")

// asegurarCredencialDeEmpresa deja el acceso del administrador de una empresa en
// la base de control, que es el único sitio donde el login busca credenciales.
//
// Dos formas de llamarla, una sola implementación:
//
//   - password != "": la clave se acaba de elegir. Es el asistente de
//     instalación, donde no hay a quién preguntarle y la clave la escribe quien
//     instala.
//   - password == "": la instalación ya existía. La clave se COPIA de la base de
//     la empresa tal como está guardada —bcrypt o el MD5 heredado del PHP—, sin
//     pasar por texto en claro. Es lo que hace que una instalación anterior gane
//     el acceso sin que nadie tenga que volver a escribir su clave, y lo que
//     evita que el arranque de una instalación nueva (que genera la clave del
//     administrador y la imprime UNA vez) imprima una clave que después no sirve.
//
// NUNCA PISA UNA CREDENCIAL EXISTENTE. Si el usuario ya está en la base de
// control, se deja como está: la que abre la puerta es la que ya está guardada, y
// reescribirla desde la copia de una empresa elegiría la copia equivocada como
// autoridad. Eso importa en el caso concreto del cambio de clave de "Mi Perfil",
// que se guarda en control: un arranque posterior no puede devolverle al usuario
// la clave vieja que quedó en la base de su empresa.
//
// Idempotente: se puede llamar en cada arranque y en cada alta de empresa.
func (s *Server) asegurarCredencialDeEmpresa(tenantCode, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("sin nombre de usuario no hay credencial que registrar")
	}

	ctl, err := s.controlStore()
	if err != nil {
		return fmt.Errorf("catálogo de empresas no disponible: %w", err)
	}

	// El usuario ya está en control: es la MISMA identidad, y la identidad es
	// global (un nombre de usuario, varias empresas por `tenant_users`). Por eso
	// aquí no se escribe nada, y solo se comprueba una cosa: que quien viene con
	// una clave nueva traiga la misma que ya abre la puerta.
	//
	// POR QUÉ ESA COMPROBACIÓN NO ES UN DETALLE. Si la clave fuera distinta y se
	// dejara pasar, el administrador de la segunda empresa entraría con la clave
	// de la primera —su fila de `tenant_users` ya está puesta— y eso es acceso
	// entre empresas por un nombre de usuario repetido. Y si en cambio se pisara el
	// hash, cambiaría la clave del administrador de la OTRA empresa sin que nadie
	// lo pidiera. Las dos salidas son peores que decir que no y pedir otro nombre.
	var almacenada string
	switch err := ctl.QueryRow(`SELECT IFNULL(password,'') FROM user_login WHERE username = ? LIMIT 1`, username).Scan(&almacenada); err {
	case nil:
		if password == "" {
			return nil
		}
		if ok, _ := verificarPassword(almacenada, password); ok {
			return nil
		}
		return fmt.Errorf("el usuario %q ya existe en la base de control con otra contraseña; "+
			"use otro nombre de usuario para el administrador de esta empresa: %w", username, errIdentidadYaExiste)
	case sql.ErrNoRows:
		// Sigue: es el caso normal de una empresa recién creada.
	default:
		return fmt.Errorf("comprobando la credencial de %q en la base de control: %w", username, err)
	}

	hash := ""
	if password != "" {
		generado, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("no se pudo cifrar la contraseña del administrador: %w", err)
		}
		hash = string(generado)
	} else {
		db := s.dbForTenant(tenantCode)
		if db == nil {
			return fmt.Errorf("la base de la empresa %q no está disponible: no se puede copiar su credencial", tenantCode)
		}
		if err := db.QueryRow(`SELECT IFNULL(password,'') FROM user_login WHERE username = ? LIMIT 1`, username).Scan(&hash); err != nil {
			return fmt.Errorf("leyendo la credencial de %q en la empresa %q: %w", username, tenantCode, err)
		}
		if hash == "" {
			return fmt.Errorf("el usuario %q no tiene credencial en la base de la empresa %q", username, tenantCode)
		}
	}

	// En una transacción: el identificador se calcula y se usa dentro de ella, así
	// que dos altas de empresa simultáneas no pueden quedarse con el mismo
	// user_id (el segundo INSERT chocaría con la clave primaria en vez de
	// escribir un usuario que apunta al de la otra empresa).
	tx, err := ctl.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var siguiente int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(CAST(user_id AS UNSIGNED)), 0) + 1 FROM users`).Scan(&siguiente); err != nil {
		return fmt.Errorf("calculando el identificador del usuario %q: %w", username, err)
	}
	userID := fmt.Sprint(siguiente)

	// first_name/last_name son NOT NULL en el esquema heredado: se rellenan con
	// lo que se sabe (el nombre de usuario) en vez de con NULL, que la pantalla
	// de perfil no sabría mostrar.
	if _, err := tx.Exec(`INSERT INTO users (user_id, first_name, last_name, company_name, phone, status)
		VALUES (?, ?, '', 'ERPGo', '', 1)`, userID, username); err != nil {
		return fmt.Errorf("no se pudo crear el usuario %q en la base de control: %w", username, err)
	}
	// user_type=1 es el administrador (el mismo valor con el que lo crea la
	// siembra de la empresa). NO se le añade fila en sec_userrole: el nivel se
	// resuelve por user_type=1, y un rol del catálogo tiene su `level` en
	// 'employee' por defecto, así que mapearlo aquí degradaría al administrador a
	// empleado —el mismo fallo que documenta authenticateUser.
	if _, err := tx.Exec(`INSERT INTO user_login (user_id, username, password, user_type, status)
		VALUES (?, ?, ?, 1, 1)`, userID, username, hash); err != nil {
		return fmt.Errorf("no se pudo crear la credencial de %q en la base de control: %w", username, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("guardando la credencial de %q: %w", username, err)
	}
	return nil
}
