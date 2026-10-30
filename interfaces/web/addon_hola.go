package web

// addon_hola.go — Handler de negocio del verbo “mensaje.registrar” del addon
// demo hola (docs/PLAN-ADDONS.md §7, fase 2).
//
// POR QUÉ ESTE ARCHIVO NO ES LA FRONTERA. La frontera (addons.go) valida el
// CONTEXTO de una escritura de addon: token, verbo declarado en db_write,
// sesión viva, empresa y permiso. Lo que pasa DESPUÉS ya es negocio: qué
// tabla recibe, qué columnas, qué idempotencia. Eso vive aquí, junto a los
// handlers del resto del ERP; si mañana otro addon trae su propio verbo, su
// handler será un archivo nuevo y la frontera no toca una línea.
//
// La escritura va por el ORQUESTADOR (s.Escribir), como cualquier módulo del
// ERP: el canal de addon no es una ventana para SQL suelto, es una puerta con
// nombre hacia el mismo journal + audit_log que todas las escrituras del
// núcleo. La idempotencia (Clave) se ata al id que genera este handler: si el
// addon reintenta tras una caída, el orquestador no duplica.

import (
	"context"
	"fmt"
	"strings"

	"erpgo/internal/orchestrator"
)

// addonTablaHola es la tabla propia del demo. Solo existe si el manifiesto
// del addon declara tables_own y su schema.sql la crea (fase 2 del plan): el
// handler no crea tablas ni toca las de otros; si el addon no trajo su
// migración, el orquestador descartará la escritura con error de tabla.
const addonTablaHola = "hola_mensajes"

// addonDemoVerboRegistrar conecta los handlers de negocio del addon demo
// hola. Se llama desde NewServer después de addonsIniciar: la frontera ya
// cargó el manifiesto y validó que mensaje.registrar esté declarado en
// db_write con su permiso (hola_ver); aquí solo se conecta el QUÉ hacer.
func (s *Server) addonDemoVerboRegistrar() {
	if s == nil || s.addons == nil {
		return // sin frontera no hay verbo que conectar (arranques en pruebas)
	}
	s.addonVerboRegistrar("hola", "mensaje.registrar", s.addonDemoMensajeRegistrar)
}

// addonDemoMensajeRegistrar aplica una escritura de negocio por el
// orquestador: guarda el mensaje en hola_mensajes con usuario, empresa y
// fecha. Cuerpo esperado (del addon, ya en JSON): {"mensaje": "…"}. El
// contexto de sesión (ses) lo trae la frontera YA VALIDADO: aquí no se
// vuelve a preguntar quién llamó, solo se usa.
func (s *Server) addonDemoMensajeRegistrar(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error) {
	texto, _ := cuerpo["mensaje"].(string)
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, fmt.Errorf("falta el campo mensaje (el cuerpo del verbo mensaje.registrar es {\"mensaje\": \"…\"})")
	}
	if len(texto) > 512 {
		return nil, fmt.Errorf("el mensaje excede los 512 caracteres que la columna admite")
	}
	id := "hola-" + randomHex(8)
	usuario := ses.Usuario
	if ses.Sess != nil && strings.TrimSpace(ses.Sess.Username) != "" {
		usuario = ses.Sess.Username
	}
	ctx := context.Background()
	res, err := s.Escribir(ctx, orchestrator.Entrada{
		Empresa:    ses.Empresa,
		Tabla:      addonTablaHola,
		Columnas:   []string{"id", "mensaje", "usuario", "empresa", "creada"},
		Valores:    []any{id, texto, usuario, ses.Empresa, now()},
		Clave:      "hola_mensajes:crear:" + id,
		Modulo:     "addon/hola",
		Accion:     "mensaje.registrar",
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
