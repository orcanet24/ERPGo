package web

// addon_demo.go — Handler de negocio del verbo “aviso.publicar” del módulo
// demo (docs/PLAN-ADDONS.md §7, fase 2).
//
// POR QUÉ VIVE EN EL NÚCLEO. La frontera (addons.go) valida el CONTEXTO de la
// escritura: token de la sesión, verbo declarado en db_write, sesión viva del
// usuario, empresa y permiso. Qué hace DESPUÉS ya es negocio —qué tabla
// recibe, qué columnas, qué idempotencia—, y eso se escribe aquí, junto a los
// handlers del resto del ERP y por la MISMA puerta: el orquestador, con su
// journal y su audit_log en la misma transacción. El módulo demo no tiene
// credenciales ni conoce un INSERT: pide, y el núcleo aplica.
//
// Este archivo es además la plantilla de lo que todo módulo nuevo necesita
// para escribir: una constante con SU tabla, una función que conecta sus
// verbos al arrancar, y un handler por verbo con sus validaciones.

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"erpgo/internal/orchestrator"
)

// addonTablaDemo es la tabla propia del módulo demo. Solo existe si el
// manifiesto la declara en tables_own y su schema.sql la crea: el handler no
// crea tablas ni toca las de nadie más.
const addonTablaDemo = "demo_avisos"

// Límites que la pantalla también impone (addons/demo/main.go): el candado se
// comprueba en el borde y se vuelve a comprobar aquí, porque el canal de
// negocio se puede llamar sin pasar por la pantalla.
const (
	addonDemoMaxTitulo  = 160
	addonDemoMaxMensaje = 4000
)

// addonModuloDemoVerbos conecta los verbos del módulo demo. Se llama desde
// NewServer, después de addonsIniciar: la frontera ya cargó el manifiesto y
// validó que aviso.publicar esté declarado en db_write con su permiso
// (demo_publicar); aquí solo se conecta el QUÉ hacer.
func (s *Server) addonModuloDemoVerbos() {
	if s == nil || s.addons == nil {
		return // sin frontera no hay verbo que conectar (arranques en pruebas)
	}
	s.addonVerboRegistrar("demo", "aviso.publicar", s.addonDemoAvisoPublicar)
}

// addonDemoAvisoPublicar publica un aviso por el orquestador. Cuerpo esperado
// (del módulo, ya en JSON): {"titulo": "…", "mensaje": "…"}.
// El contexto de sesión (ses) llega YA VALIDADO de la frontera: aquí no se
// vuelve a preguntar quién llamó, solo se usa.
func (s *Server) addonDemoAvisoPublicar(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error) {
	titulo, _ := cuerpo["titulo"].(string)
	mensaje, _ := cuerpo["mensaje"].(string)
	titulo = strings.TrimSpace(titulo)
	mensaje = strings.TrimSpace(mensaje)
	if titulo == "" {
		return nil, fmt.Errorf("falta el campo titulo (el cuerpo del verbo aviso.publicar es {\"titulo\": \"…\", \"mensaje\": \"…\"})")
	}
	if mensaje == "" {
		return nil, fmt.Errorf("falta el campo mensaje: un aviso sin texto no se guarda")
	}
	if utf8.RuneCountInString(titulo) > addonDemoMaxTitulo {
		return nil, fmt.Errorf("el título excede los %d caracteres que la columna admite", addonDemoMaxTitulo)
	}
	if utf8.RuneCountInString(mensaje) > addonDemoMaxMensaje {
		return nil, fmt.Errorf("el aviso excede los %d caracteres que la columna admite", addonDemoMaxMensaje)
	}

	id := "demo-" + randomHex(8)
	usuario := ses.Usuario
	if ses.Sess != nil && strings.TrimSpace(ses.Sess.Username) != "" {
		usuario = ses.Sess.Username
	}
	ctx := context.Background()
	res, err := s.Escribir(ctx, orchestrator.Entrada{
		Empresa:    ses.Empresa,
		Tabla:      addonTablaDemo,
		Columnas:   []string{"id", "titulo", "mensaje", "autor", "empresa", "creada"},
		Valores:    []any{id, titulo, mensaje, usuario, ses.Empresa, now()},
		Clave:      "demo_avisos:crear:" + id,
		Modulo:     "addon/demo",
		Accion:     "aviso.publicar",
		Usuario:    usuario,
		IP:         "127.0.0.1 (canal addon)",
		RegistroID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("el orquestador rechazó la escritura: %w", err)
	}
	return map[string]interface{}{
		"registro_id": id,
		"duplicado":   res.Duplicado,
		"correlativo": res.Correlativo,
	}, nil
}
