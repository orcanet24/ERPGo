package web

// addon_memo.go — Handler de negocio del verbo «nota.crear» del módulo Memo,
// fase A (docs/PLAN-MEMO.md §5).
//
// POR QUÉ VIVE AQUÍ. La frontera valida el CONTEXTO (token, verbo declarado,
// sesión viva, empresa, permiso); el QUÉ hacer ya es negocio del módulo y se
// escribe aquí, junto al resto de handlers, bajando por el orquestador con su
// journal y su audit_log. El addon no tiene credenciales: pide, y el núcleo
// aplica. Cada fase nueva de Memo agrega su verbo con su handler y su prueba;
// la frontera no se toca.
//
// LA REGLA QUE ESTE ARCHIVO PROTEGE: la visibilidad. Una nota `privada` no
// admite destinatarios (son suyos por definición), una `empresa` no los
// necesita (la ve la empresa entera) y una `personas` sin destinatarios no se
// guarda — que es el fallo que convertiría «compartido conmigo» en «compartido
// con nadie» o, peor, en un papelito que nadie sabe para quién era.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"erpgo/internal/orchestrator"
)

// Tablas propias del módulo, declaradas en su manifiesto y creadas por su
// schema.sql. Este archivo no crea tablas: si no existen, el orquestador
// rechaza la escritura con su error, que es lo que debe pasar.
const (
	tablaMemoNotas         = "memo_notas"
	tablaMemoDestinatarios = "memo_destinatarios"
)

// Límites que la pantalla también impone. El canal de negocio se puede llamar
// sin pasar por la pantalla, así que el candado se comprueba aquí igual.
const (
	memoMaxTitulo        = 200
	memoMaxTexto         = 20000
	memoMaxDestinatarios = 100
)

// memoReUsuario forma de un id de usuario del ERP (users.user_id, varchar(15)).
// Un id con signos raros no es una persona: es un intento de meter datos donde
// después se leerán como identidad.
var memoReUsuario = regexp.MustCompile(`^[A-Za-z0-9_-]{1,15}$`)

// memoVisibilidades son los tres estados que la fase A admite.
var memoVisibilidades = map[string]bool{"privada": true, "empresa": true, "personas": true}

// addonModuloMemoVerbos conecta los verbos del módulo Memo. Se llama desde
// NewServer después de addonsIniciar: la frontera ya cargó el manifiesto y
// validó que nota.crear esté declarado en db_write con su permiso
// (memo_publicar); aquí solo se conecta el QUÉ hacer.
func (s *Server) addonModuloMemoVerbos() {
	if s == nil || s.addons == nil {
		return // sin frontera no hay verbo que conectar (arranques en pruebas)
	}
	s.addonVerboRegistrar("memo", "nota.crear", s.addonMemoNotaCrear)
}

// addonMemoNotaCrear publica una nota. Cuerpo esperado (del addon, en JSON):
//
//	{"titulo": "…", "texto": "…", "visibilidad": "privada|empresa|personas",
//	 "destinatarios": ["7", "9"]}
//
// El contexto de sesión (ses) llega YA VALIDADO de la frontera: aquí no se
// vuelve a preguntar quién llamó, solo se usa.
func (s *Server) addonMemoNotaCrear(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error) {
	titulo := memoTexto(cuerpo["titulo"])
	texto := memoTexto(cuerpo["texto"])
	visibilidad := strings.ToLower(memoTexto(cuerpo["visibilidad"]))
	if visibilidad == "" {
		visibilidad = "empresa"
	}
	if titulo == "" {
		return nil, fmt.Errorf("falta el campo titulo (el cuerpo del verbo nota.crear es {\"titulo\": …, \"texto\": …, \"visibilidad\": …, \"destinatarios\": […]})")
	}
	if texto == "" {
		return nil, fmt.Errorf("falta el campo texto: una nota vacía no se guarda")
	}
	if utf8.RuneCountInString(titulo) > memoMaxTitulo {
		return nil, fmt.Errorf("el título excede los %d caracteres que la columna admite", memoMaxTitulo)
	}
	if utf8.RuneCountInString(texto) > memoMaxTexto {
		return nil, fmt.Errorf("el texto excede los %d caracteres que la columna admite", memoMaxTexto)
	}
	if !memoVisibilidades[visibilidad] {
		return nil, fmt.Errorf("la visibilidad %q no existe: debe ser privada, empresa o personas", visibilidad)
	}

	// Los destinatarios solo significan algo en «personas»: en «privada» la
	// nota es del autor y en «empresa» la ve la empresa. Se descartan en vez
	// de guardarlos, para que la tabla diga la verdad.
	destinatarios := []string{}
	if visibilidad == "personas" {
		destinatarios = memoDestinatarios(cuerpo["destinatarios"])
		if len(destinatarios) == 0 {
			return nil, fmt.Errorf("visibilidad «personas» sin destinatarios: la nota quedaría compartida con nadie")
		}
		if len(destinatarios) > memoMaxDestinatarios {
			return nil, fmt.Errorf("una nota no se comparte con más de %d personas a la vez (%d pedidas)", memoMaxDestinatarios, len(destinatarios))
		}
		vistos := map[string]bool{}
		for _, d := range destinatarios {
			if !memoReUsuario.MatchString(d) {
				return nil, fmt.Errorf("%q no es un id de usuario válido: revisa a quién se comparte", d)
			}
			if vistos[d] {
				return nil, fmt.Errorf("la persona %q está repetida en los destinatarios", d)
			}
			vistos[d] = true
		}
	}

	id := "memo-" + randomHex(8)
	autor := ses.Usuario
	if ses.Sess != nil && strings.TrimSpace(ses.Sess.Username) != "" {
		autor = ses.Sess.Username
	}
	ctx := context.Background()

	res, err := s.Escribir(ctx, orchestrator.Entrada{
		Empresa:    ses.Empresa,
		Tabla:      tablaMemoNotas,
		Columnas:   []string{"id", "empresa", "autor", "titulo", "texto", "visibilidad", "tipo", "estado", "fijada", "creada", "actualizada"},
		Valores:    []any{id, ses.Empresa, autor, titulo, texto, visibilidad, "nota", "activo", 0, now(), now()},
		Clave:      "memo_notas:crear:" + id,
		Modulo:     "addon/memo",
		Accion:     "nota.crear",
		Usuario:    autor,
		IP:         "127.0.0.1 (canal addon)",
		RegistroID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("el orquestador rechazó la nota: %w", err)
	}

	// Los destinatarios van DESPUÉS y cada uno con su propia clave de
	// idempotencia. Si el segundo falla, queda una nota sin compartir: el
	// fallo es del autor contra su propia nota, nunca una fuga hacia gente a
	// la que no se la compartió — el orden está elegido por eso.
	compartido := 0
	for _, destino := range destinatarios {
		fila := "memo-" + randomHex(8)
		if _, err := s.Escribir(ctx, orchestrator.Entrada{
			Empresa:    ses.Empresa,
			Tabla:      tablaMemoDestinatarios,
			Columnas:   []string{"id", "nota_id", "destino_tipo", "destino_id", "empresa"},
			Valores:    []any{fila, id, "persona", destino, ses.Empresa},
			Clave:      "memo_destinatarios:crear:" + id + ":" + destino,
			Modulo:     "addon/memo",
			Accion:     "nota.crear.destinatario",
			Usuario:    autor,
			IP:         "127.0.0.1 (canal addon)",
			RegistroID: fila,
		}); err != nil {
			return nil, fmt.Errorf("la nota %s se guardó pero no se pudo compartirla con %s: %w", id, destino, err)
		}
		compartido++
	}

	return map[string]interface{}{
		"registro_id": id,
		"compartido":  compartido,
		"duplicado":   res.Duplicado,
	}, nil
}

// memoTexto convierte el campo del cuerpo en texto recortado. Un campo que no
// es texto (número, objeto) se toma como vacío: el candado de arriba se queja
// con un motivo claro en lugar de fallar de forma rara más abajo.
func memoTexto(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// memoDestinatarios normaliza la lista de destinatarios del JSON: un array
// JSON llega como []interface{}, y un addon que mande una cadena suelta
// («destinatario»: "7") también se acepta — es un error de forma, no de fondo.
func memoDestinatarios(v interface{}) []string {
	salida := []string{}
	switch lista := v.(type) {
	case []interface{}:
		for _, item := range lista {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				salida = append(salida, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, item := range lista {
			if strings.TrimSpace(item) != "" {
				salida = append(salida, strings.TrimSpace(item))
			}
		}
	case string:
		if strings.TrimSpace(lista) != "" {
			salida = append(salida, strings.TrimSpace(lista))
		}
	}
	return salida
}
