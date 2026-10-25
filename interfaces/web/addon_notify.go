package web

// addon_notify.go — El ERP pide un aviso y el addon decide el canal.
//
// DOS PERMISOS, DOS PREGUNTAS DISTINTAS.
//
//   - ¿Puede ESTE MÓDULO pedir mensajes? Es una capability del addon
//     (`capabilities.notify.modules`), y se aprueba en la pantalla de addons.
//     Sirve para decir "cobranza sí, compras no" sin reinstalar nada.
//   - ¿Puede ESTA PERSONA enviar a mano? Es un permiso por rol
//     (`addon_role_permissions`), con el código que declare el manifiesto
//     (`wa_send`). Un aviso automático de factura no pasa por aquí: lo dispara el
//     evento y no hay persona que autorizar.
//
// POR QUÉ EL HOST NO MIRA EL CONSENTIMIENTO.
//
// El consentimiento es del canal y de la persona, no del módulo que pregunta:
// vive en la tabla del addon, junto al número y la fecha en que se dio. Si el
// núcleo lo duplicara, tendría dos verdades y una campaña podría salir con el
// permiso viejo. El addon bloquea el destinatario y devuelve el motivo; el host
// solo lo reproduce.
//
// QUÉ NO HACE ESTE FICHERO.
//
// No habla con WhatsApp. No conoce GOWA ni la Cloud API de Meta: manda el sobre
// al addon y devuelve su respuesta. Cambiar de canal no toca el núcleo.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/orcanet/ergoaddon"
)

// addonNotifyTimeout es el presupuesto del envío. Es más largo que el del guard
// porque un mensaje no está esperando en la caja: si el addon tarda dos segundos
// en aceptar el envío, nadie hace fila.
const addonNotifyTimeout = 8 * time.Second

// addonNotifyPermission es el código que el manifiesto declara para enviar a mano.
const addonNotifyPermission = "wa_send"

// activeAddonNotifier busca el addon activo que puede avisar para ese módulo.
//
// Devuelve el primero por nombre: si dos addons de mensajería declaran el mismo
// módulo, el que gana está decidido y no depende de un mapa de Go. Una
// instalación con dos addons de WhatsApp para la misma empresa es un error de
// operación, y el registro lo muestra.
func (s *Server) activeAddonNotifier(catalog *sql.DB, module string) (string, *addonProcess, *ergoaddon.Capabilities, error) {
	if catalog == nil {
		return "", nil, nil, fmt.Errorf("catálogo de addons no disponible")
	}
	rows, err := catalog.Query(`SELECT id, manifest_json FROM addons WHERE is_active = 1 ORDER BY id`)
	if err != nil {
		return "", nil, nil, err
	}
	defer rows.Close()

	type candidato struct {
		code string
		caps *ergoaddon.Capabilities
	}
	var candidatos []candidato
	for rows.Next() {
		var code, manifestJSON string
		if rows.Scan(&code, &manifestJSON) != nil {
			continue
		}
		var manifest ergoaddon.Manifest
		if json.Unmarshal([]byte(manifestJSON), &manifest) != nil {
			continue
		}
		caps := addonEffectiveCapabilitiesFor(catalog, code, &manifest)
		if caps == nil || !caps.HasNotifyModule(module) {
			continue
		}
		candidatos = append(candidatos, candidato{code: code, caps: caps})
	}
	if err := rows.Err(); err != nil {
		return "", nil, nil, err
	}
	if len(candidatos) == 0 {
		return "", nil, nil, fmt.Errorf("ningún addon de mensajería tiene aprobado el módulo %q", module)
	}

	code := candidatos[0].code
	addonMgr.mu.RLock()
	proc := addonMgr.process[code]
	addonMgr.mu.RUnlock()
	if proc == nil {
		return "", nil, nil, fmt.Errorf("el addon %s está aprobado pero no está corriendo", code)
	}
	return code, proc, candidatos[0].caps, nil
}

// addonNotify entrega el sobre al addon. `module` debe venir ya validado por el
// llamante (o por el hook HTTP); aquí solo se resuelve quién lo atiende.
func (s *Server) addonNotify(ctx context.Context, db *sql.DB, tenant, module string,
	req ergoaddon.NotifyRequest) (*ergoaddon.NotifyResponse, error) {
	if db == nil {
		return nil, fmt.Errorf("base de datos no disponible")
	}
	if strings.TrimSpace(module) == "" {
		return nil, fmt.Errorf("módulo requerido")
	}
	catalog := s.addonCatalogDB(db)
	code, proc, caps, err := s.activeAddonNotifier(catalog, module)
	if err != nil {
		return nil, err
	}

	req.Module = module
	if req.Tenant == "" {
		req.Tenant = tenant
	}
	req.HostURL = s.addonHostURL()

	grant := issueAddonGrant(code, db, *caps)
	return callAddonNotify(ctx, proc, grant, req)
}

func callAddonNotify(ctx context.Context, proc *addonProcess, grant string,
	req ergoaddon.NotifyRequest) (*ergoaddon.NotifyResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, addonNotifyTimeout)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d%s", proc.HTTPPort, ergoaddon.HookNotifyPath)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(ergoaddon.AddonTokenHeader, proc.Token)
	httpReq.Header.Set("Authorization", "Bearer "+grant)

	resp, err := (&http.Client{Timeout: addonNotifyTimeout}).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("addon %s: HTTP %d: %s", proc.Code, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out ergoaddon.NotifyResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("respuesta del addon ilegible: %w", err)
	}
	return &out, nil
}

// addonRoleHasPermission comprueba una concesión por rol SIN la regla de sufijos
// de `hasAddonPermission`: los permisos de mensajería se llaman `wa_send` y
// `wa_inbox`, no `wa_message_read`, y aplicarles el esquema de acciones los
// dejaría sin efecto para siempre.
func (s *Server) addonRoleHasPermission(db *sql.DB, userID, code string) bool {
	if db == nil || userID == "" || code == "" {
		return false
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM addon_role_permissions arp
		JOIN sec_userrole ur ON ur.roleid = arp.role_id
		JOIN addons a ON a.id = arp.addon_code AND a.is_active = 1
		WHERE ur.user_id = ? AND arp.permission_code = ? AND arp.is_allowed = 1`,
		userID, code).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// addonNotifySend es el endpoint para un envío pedido por una persona desde un
// módulo del ERP. El automático (una factura emitida) no pasa por aquí.
func (s *Server) addonNotifySend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "No autorizado", http.StatusUnauthorized)
		return
	}

	var req struct {
		Module    string                      `json:"module"`
		Purpose   string                      `json:"purpose"`
		Channel   string                      `json:"channel"`
		To        []ergoaddon.NotifyRecipient `json:"to"`
		Template  string                      `json:"template"`
		Text      string                      `json:"text"`
		Variables map[string]string           `json:"variables"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}
	req.Module = strings.TrimSpace(req.Module)
	req.Purpose = strings.TrimSpace(req.Purpose)
	if req.Module == "" || req.Purpose == "" {
		jsonError(w, "módulo y propósito son obligatorios", http.StatusBadRequest)
		return
	}
	if len(req.To) == 0 {
		jsonError(w, "sin destinatarios", http.StatusBadRequest)
		return
	}
	if req.Text == "" && req.Template == "" {
		jsonError(w, "el mensaje necesita texto o plantilla", http.StatusBadRequest)
		return
	}

	db := s.authDB(r)
	// Master y admin pueden siempre; el resto necesita la concesión por rol. Es
	// la misma regla que usan los permisos de addon del resto del ERP.
	if sess.UserLevel != "master" && sess.UserLevel != "admin" {
		if !s.addonRoleHasPermission(db, sess.UserID, addonNotifyPermission) {
			jsonError(w, "No tiene permiso para enviar mensajes", http.StatusForbidden)
			return
		}
	}

	resp, err := s.addonNotify(r.Context(), db, sess.TenantCode, req.Module, ergoaddon.NotifyRequest{
		Module:    req.Module,
		UserID:    sess.UserID,
		UserName:  sess.Username,
		Purpose:   req.Purpose,
		Channel:   req.Channel,
		To:        req.To,
		Template:  req.Template,
		Text:      req.Text,
		Variables: req.Variables,
	})
	if err != nil {
		jsonError(w, "No se pudo avisar: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":  1,
		"queued":  resp.Queued,
		"blocked": resp.Blocked,
	})
}
