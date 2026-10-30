package web

// addon_hooks.go — Las manillas de la reactividad (docs/PLAN-ADDONS.md §8, fase 3).
//
// hooks_out «me avisan cuando el núcleo hace algo»: tras cada evento de
// negocio, el núcleo ENCOLA (no envía en línea) una fila en la BD de control —
// el mismo patrón del runner de facturación digital — y un runner la entrega
// al addon con POST /hooks/<evento>, el token de ESTA sesión del addon y
// X-ERP-Tenant. Un addon caído o lento jamás frena una emisión de factura:
// solo su fila espera, con reintentos espaciados hasta un tope, para que quede
// en la historia con su motivo en lugar de reintentar para siempre.
//
// hooks_in «el ERP pide que el addon haga algo»: el handler de negocio llama
// addonHookPedir con un hook DECLARADO en hooks_in y un timeout de 3 s. Si el
// addon no contesta a tiempo, la llamada devuelve error EXPLICADO y el flujo
// de negocio sigue sin él: el núcleo registra y no rompe.
//
// La cola vive en la BD de CONTROL (infraestructura, como web_sessions), con
// la empresa de cada fila dentro: un evento de la empresa A jamás se entrega
// con el contexto de la B.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// El ritmo del runner de hooks_out. 30 s como la facturación digital: un
// intervalo corto para el flujo diario sin saturar al addon.
const hookIntervaloRunner = 30 * time.Second

// hookMaxIntentos límita los reintentos por fila: si el addon no recibe el
// aviso en este rango, algo estructural falla (addon retirado, puerto podrido)
// y que quede 'failed' con su error en la historia avisa al admin en vez de
// martillar para siempre.
const hookMaxIntentos = 8

// hookTimeoutIn es el plazo de respuesta de un hook_in. Un addon que tarda más
// no participa: el flujo de negocio no puede esperar a nadie externo.
const hookTimeoutIn = 3 * time.Second

// hookSello es el formato de los timestamps de la cola: VARCHAR ordenable, el
// mismo criterio del esquema de control; se compara bien en MySQL y en SQLite.
const hookSello = "2006-01-02 15:04:05"

// topeHookPayload es el tamaño máximo del cuerpo de un aviso encolado. La
// notificación lleva lo esencial del evento, no un volcado de la factura.
const topeHookPayload = 256 * 1024

// -------- Cola persistente (BD de control) --------

// filaHook es UN avisо pendiente (o entregado) para UN addon.
type filaHook struct {
	ID       int64
	AddonID  string
	Evento   string
	Empresa  string
	Payload  []byte
	Intentos int64
}

// crearTablaHooks garantiza la cola en la BD de control. Idempotente y de
// arranque: sin cola no hay hooks_out, pero el ERP respira igual (el panel
// mostrará la historia vacía y el log llevará el aviso).
func (h *addonHost) crearTablaHooks() {
	if h == nil || h.s == nil {
		return
	}
	ctl, err := h.s.controlStore()
	if err != nil {
		log.Printf("AVISO addons: sin base de control para la cola de hooks: %v", err)
		return
	}
	_, err = ctl.Exec(`CREATE TABLE IF NOT EXISTS addon_hook_out (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		addon_id VARCHAR(64) NOT NULL,
		evento VARCHAR(100) NOT NULL,
		empresa VARCHAR(64) NOT NULL DEFAULT '',
		payload_json TEXT,
		intentos INTEGER NOT NULL DEFAULT 0,
		status VARCHAR(12) NOT NULL DEFAULT 'pending',
		ultimo_error VARCHAR(500) NOT NULL DEFAULT '',
		proximo VARCHAR(32) NOT NULL DEFAULT '',
		creado VARCHAR(32) NOT NULL DEFAULT '',
		enviado VARCHAR(32) NOT NULL DEFAULT ''
	)`)
	if err != nil {
		log.Printf("AVISO addons: cola de hooks no creada (hooks_out quedan sin entrega): %v", err)
		return
	}
	log.Printf("addons: cola de hooks_out lista en la base de control")
}

// addonHookDisparar encola UN evento de negocio para TODOS los addons que
// suscriben ese hook_out. Es lo que llaman los handlers de negocio tras el
// commit: el insert es rápido y el envío va por el runner, así que la factura
// de hoy no espera al addon de mañana.
func (s *Server) addonHookDisparar(evento, empresa string, payload any) {
	if s == nil || s.addons == nil {
		return
	}
	s.addons.hookEncolar(evento, empresa, payload)
}

// hookEncolar filtra por lo DECLARADO: un evento que el núcleo emitió y nadie
// suscribe no toca ni la cola. La fila nace 'pending' ya vencida: el próximo
// tick la toma.
func (h *addonHost) hookEncolar(evento, empresa string, payload any) {
	if h == nil || h.s == nil {
		return
	}
	ctl, err := h.s.controlStore()
	if err != nil {
		log.Printf("AVISO addons: hook %q sin cola (sin base de control): %v", evento, err)
		return
	}
	h.mu.Lock()
	var suscritos []string
	for _, p := range h.progs {
		for _, ev := range p.manifiesto.HooksOut {
			if ev == evento {
				suscritos = append(suscritos, p.manifiesto.ID)
				break
			}
		}
	}
	h.mu.Unlock()
	if len(suscritos) == 0 {
		return
	}
	cuerpo, err := json.Marshal(payload)
	if err != nil || len(cuerpo) > topeHookPayload {
		log.Printf("AVISO addons: hook %q con payload inválido o demasiado grande (%d bytes): se descarta", evento, len(cuerpo))
		return
	}
	ahora := h.sello()
	for _, id := range suscritos {
		if _, err := ctl.Exec(`INSERT INTO addon_hook_out
			(addon_id, evento, empresa, payload_json, status, creado, proximo)
			VALUES (?, ?, ?, ?, 'pending', ?, ?)`, id, evento, empresa, string(cuerpo), ahora, ahora); err != nil {
			log.Printf("AVISO addons: no se pudo encolar %q para %s: %v", evento, id, err)
		}
	}
}

// sello es el timestamp de la frontera (regulado por h.ahora en pruebas).
func (h *addonHost) sello() string {
	ahora := h.ahora
	if ahora == nil {
		ahora = time.Now
	}
	return ahora().Format(hookSello)
}

// -------- Runner de entrega --------

// vigilarHooks es el ciclo: cada 30 s toma lo pendiente y lo entrega.
func (h *addonHost) vigilarHooks() {
	t := time.NewTicker(hookIntervaloRunner)
	defer t.Stop()
	for {
		select {
		case <-h.parado:
			return
		case <-t.C:
			h.entregarPendientes()
		}
	}
}

// entregarPendientes es UNA pasada del runner: filas pending cuya hora llegó,
// en orden y acotadas. El addon destino se resuelve AHORA: si ya no está
// cargado o dejó de declarar el evento, la fila queda 'descartado' — un addon
// retirado no debe dejar avisos rodando para siempre —; si está caído, solo
// espera al próximo tick: es la cola del addon la que espera, jamás la factura
// del ERP.
func (h *addonHost) entregarPendientes() {
	if h == nil || h.s == nil {
		return
	}
	ctl, err := h.s.controlStore()
	if err != nil {
		return
	}
	rows, err := ctl.Query(`SELECT id, addon_id, evento, empresa, IFNULL(payload_json, ''), intentos
		FROM addon_hook_out
		WHERE status = 'pending' AND proximo <= ?
		ORDER BY id LIMIT 10`, h.sello())
	if err != nil {
		return
	}
	var pendientes []filaHook
	for rows.Next() {
		var f filaHook
		var payload string
		if err := rows.Scan(&f.ID, &f.AddonID, &f.Evento, &f.Empresa, &payload, &f.Intentos); err == nil {
			f.Payload = []byte(payload)
			pendientes = append(pendientes, f)
		}
	}
	rows.Close()
	for _, f := range pendientes {
		h.entregarFila(ctl, f)
	}
}

// entregarFila entrega UN aviso y deja la fila con la verdad: sent con sello,
// o un reintento más tarde con el error escrito, o 'failed' al agotar.
func (h *addonHost) entregarFila(ctl *sql.DB, f filaHook) {
	if h == nil || ctl == nil {
		return
	}
	h.mu.Lock()
	p := h.progs[f.AddonID]
	declarado := false
	if p != nil {
		for _, ev := range p.manifiesto.HooksOut {
			if ev == f.Evento {
				declarado = true
				break
			}
		}
	}
	activo := p != nil && declarado && p.estado == estadoActivo
	// El token de ESTA sesión va en la entrega: una fila que se reintenta con
	// el token de una sesión anterior recibiría 403 una y otra vez.
	token, puerto := "", 0
	if activo {
		token, puerto = p.token, p.puerto
	}
	h.mu.Unlock()

	if !activo {
		if _, err := ctl.Exec(`UPDATE addon_hook_out SET status = 'descartado', ultimo_error = ?
			WHERE id = ?`, "el addon "+f.AddonID+" ya no está cargado y activo en este arranque", f.ID); err != nil {
			log.Printf("AVISO addons: fila de hook %d sin marcar: %v", f.ID, err)
		}
		return
	}

	enviar := h.entregar
	if enviar == nil {
		enviar = h.entregarReal
	}
	err := enviar(p, f.Evento, f.Payload, f.Empresa, token, puerto)
	h.mu.Lock()
	ahora := h.sello()
	pend := h.ahoraFunc()
	h.mu.Unlock()
	if err == nil {
		if _, err := ctl.Exec(`UPDATE addon_hook_out SET status = 'sent', intentos = ?,
			enviado = ?, ultimo_error = '' WHERE id = ?`, f.Intentos+1, ahora, f.ID); err != nil {
			log.Printf("AVISO addons: hook entregado pero fila %d sin marcar: %v", f.ID, err)
		}
		return
	}
	intentos := f.Intentos + 1
	if intentos >= hookMaxIntentos {
		if _, err := ctl.Exec(`UPDATE addon_hook_out SET status = 'failed', intentos = ?,
			ultimo_error = ? WHERE id = ?`, intentos, truncado(err.Error(), 500), f.ID); err != nil {
			log.Printf("AVISO addons: fila de hook %d caída en 'failed': %v", f.ID, err)
		}
		return
	}
	// Backoff exponencial: 1m, 2m, 4m… tope 30m. El aviso sigue esperando: el
	// escalonado es para no martillar al addon que cae cinco minutos.
	retardo := time.Minute << minDe(int(intentos-1), 5)
	proximo := pend.Add(retardo).Format(hookSello)
	if _, err := ctl.Exec(`UPDATE addon_hook_out SET intentos = ?, proximo = ?,
		ultimo_error = ? WHERE id = ?`, intentos, proximo, truncado(err.Error(), 500), f.ID); err != nil {
		log.Printf("AVISO addons: fila de hook %d sin poder reprogramarse: %v", f.ID, err)
	}
}

// ahoraFunc devuelve el reloj de la frontera (para los sellos de reintento).
func (h *addonHost) ahoraFunc() time.Time {
	ahora := h.ahora
	if ahora == nil {
		ahora = time.Now
	}
	return ahora()
}

// entregarReal hace la entrega: POST /hooks/<evento> en loopback, token de la
// sesión viva del addon y la empresa del evento. 10 s de tope: si el addon
// tarda más, el aviso reintenta; jamás espera para siempre.
func (h *addonHost) entregarReal(p *addonProc, evento string, payload []byte, empresa, token string, puerto int) error {
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/hooks/%s", puerto, evento), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("construyendo el envío: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERP-Addon-Token", token)
	req.Header.Set("X-ERP-Tenant", empresa)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("el addon respondió %d al hook", resp.StatusCode)
	}
	return nil
}

// -------- Historia para el panel --------

// addonsHooksHistoria son las últimas filas de la cola (ENTREGADOS, caídos y
// descartados por igual): la pantalla Módulos muestra QUÉ llegó a cada addon,
// CUÁNDO y con qué intentos. Sin cola en este arranque, una lista vacía que
// conviene leer así: «nada se entregó porque nada hay».
func (s *Server) addonsHooksHistoria(limite int) []map[string]interface{} {
	if s == nil || s.addons == nil || limite <= 0 {
		return []map[string]interface{}{}
	}
	ctl, err := s.controlStore()
	if err != nil {
		return []map[string]interface{}{}
	}
	rows, err := ctl.Query(`SELECT id, evento, addon_id, empresa, status, intentos,
		IFNULL(ultimo_error, ''), creado, IFNULL(enviado, '')
		FROM addon_hook_out ORDER BY id DESC LIMIT ?`, limite)
	if err != nil {
		return []map[string]interface{}{}
	}
	defer rows.Close()
	historia := []map[string]interface{}{}
	for rows.Next() {
		var id, intentos int64
		var evento, addon, empresa, status, errorr, creado, enviado string
		if err := rows.Scan(&id, &evento, &addon, &empresa, &status, &intentos, &errorr, &creado, &enviado); err != nil {
			continue
		}
		historia = append(historia, map[string]interface{}{
			"id": id, "evento": evento, "addon": addon, "empresa": empresa,
			"status": status, "intentos": intentos, "error": errorr,
			"creado": creado, "enviado": enviado,
		})
	}
	return historia
}

// -------- hooks_in: el ERP pide, el addon responde --------

// addonHookPedir pide UN hook declarado en hooks_in del addon, CON la empresa
// del flujo: el candado es el manifiesto — el núcleo jamás le pide un hook que
// el addon no ofrece — y el plazo es hookTimeoutIn: si no contesta, el error
// va EXPLICADO y el handler de negocio sigue su camino de retorno.
func (s *Server) addonHookPedir(id, hook, empresa string, payload any, tope time.Duration) (json.RawMessage, error) {
	if s == nil || s.addons == nil {
		return nil, fmt.Errorf("no hay frontera de addons en este arranque")
	}
	if tope <= 0 || tope > 10*time.Second {
		tope = hookTimeoutIn
	}
	s.addons.mu.Lock()
	p := s.addons.progs[id]
	s.addons.mu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("no hay un addon %q cargado en este arranque", id)
	}
	declarado := false
	for _, h := range p.manifiesto.HooksIn {
		if h == hook {
			declarado = true
			break
		}
	}
	if !declarado {
		return nil, fmt.Errorf("el addon %q no declara el hook %q en hooks_in: el manifiesto es el candado", id, hook)
	}
	s.addons.mu.Lock()
	activo, token, puerto := p.estado == estadoActivo, p.token, p.puerto
	s.addons.mu.Unlock()
	if !activo {
		return nil, fmt.Errorf("el addon %q está %s y no responde: siga sin él", id, p.estado)
	}
	cuerpo, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload del hook %q: %w", hook, err)
	}
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/hooks/%s", puerto, hook), bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERP-Addon-Token", token)
	req.Header.Set("X-ERP-Tenant", empresa)
	resp, err := (&http.Client{Timeout: tope}).Do(req)
	if err != nil {
		// El addon no responde a tiempo: el camino de retorno es el error, con
		// el plazo visible. El log deja claro que no fue un fallo de red del ERP.
		if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
			log.Printf("AVISO addons: hook %q de %q venció el plazo de %v: el negocio sigue sin el addon", hook, id, tope)
			return nil, fmt.Errorf("el hook %q no respondió en %v", hook, tope)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("el addon %q respondió %d al hook %q", id, resp.StatusCode, hook)
	}
	var salida json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&salida); err != nil {
		return nil, fmt.Errorf("la respuesta del hook %q no es JSON: %w", hook, err)
	}
	return salida, nil
}
