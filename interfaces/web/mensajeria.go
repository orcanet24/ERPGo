package web

// mensajeria.go — Activación y estado del módulo de mensajería.
//
// ESTE ARCHIVO ES EL CANDADO, no la mensajería. Aquí se decide si el módulo
// existe para esta instalación y esta empresa, se guarda el texto de riesgo que
// el dueño aceptó y se prueba que el servicio responde. Los dispositivos, la
// cola, el chat y el OTP llegan en las fases siguientes y todos preguntan a
// `mensajeriaActiva` antes de hacer nada.
//
// TRES LLAVES, NO UNA. `modules.is_active` decide si el módulo se muestra;
// `msg_global.enabled` si la instalación lo permite; `msg_setting.enabled` si
// esta empresa lo tiene encendido. Las tres son del super administrador. Si
// falta una, las rutas responden 404 —no 403—: no se anuncia lo que no existe.
//
// LOS SECRETOS NO VUELVEN AL NAVEGADOR. La contraseña del servicio se guarda
// cifrada y la pantalla solo dice si hay una puesta. Al editar, un campo vacío
// significa «no la cambies».

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"erpgo/internal/mensajeria"
	"erpgo/internal/mensajeria/gowa"
	"erpgo/internal/security"
)

// msgGlobal es la configuración de la instalación (base de control).
type msgGlobal struct {
	Enabled           bool
	DaemonURL         string
	DaemonUser        string
	DaemonPasswordEnc string
	WebhookSecretEnc  string
	DefaultCountry    string
	RiskTextVersion   int
	UpdatedBy         string
	UpdatedAt         string
}

// msgSetting es la configuración de la empresa (base de la empresa).
type msgSetting struct {
	Enabled        bool
	RiskAcceptedBy string
	RiskAcceptedAt string
	DefaultCountry string
	ChatEnabled    bool
	OtpEnabled     bool
	UpdatedBy      string
	UpdatedAt      string
}

// msgProvider es un proveedor habilitado o no en la empresa.
type msgProvider struct {
	Provider string
	Enabled  bool
	Config   string
}

// RiesgoMensajeria es el texto que el master tiene que aceptar. Va en el
// código, no en un documento: la pantalla lo muestra tal cual y la aceptación
// queda con nombre y fecha.
const RiesgoMensajeria = "Aviso obligatorio antes de encender la mensajería:\n\n" +
	"1. WhatsApp por GOWA y Telegram por cuenta personal NO son las APIs oficiales. " +
	"El número puede ser bloqueado o perdido de forma irreversible.\n" +
	"2. Use un número dedicado, nunca el principal de la empresa ni el personal.\n" +
	"3. Este módulo no envía difusiones masivas ni mensajes a desconocidos: solo avisos " +
	"a clientes, proveedores y personal con relación previa.\n" +
	"4. El OTP por estas vías no sustituye al correo como único factor de acceso.\n" +
	"5. La empresa asume el riesgo y designa un responsable, que queda registrado.\n" +
	"6. El servicio hay que mantenerlo: si WhatsApp o Telegram cambian, hay que actualizarlo."

func (s *Server) registerMensajeriaRoutes() {
	s.mux.HandleFunc("GET /settings/mensajeria", s.mensajeriaSettingsForm)
	s.mux.HandleFunc("POST /settings/mensajeria/global", s.mensajeriaGlobalSave)
	s.mux.HandleFunc("POST /settings/mensajeria/empresa", s.mensajeriaEmpresaSave)
	s.mux.HandleFunc("POST /settings/mensajeria/modulo", s.mensajeriaModuloToggle)
	s.mux.HandleFunc("POST /settings/mensajeria/probar", s.mensajeriaProbar)
	s.mux.HandleFunc("GET /settings/mensajeria/cola", s.mensajeriaColaForm)
	s.mux.HandleFunc("GET /mensajeria", s.mensajeriaHome)
}

// ---------- Lectura del estado ----------

// msgGlobalDe lee la configuración de la instalación. Una base sin la tabla (o
// sin la fila) devuelve los valores de fábrica: apagado y sin servicio.
func (s *Server) msgGlobalDe(r *http.Request) msgGlobal {
	g := msgGlobal{DefaultCountry: mensajeria.PaisPorDefecto, RiskTextVersion: 1}
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		return g
	}
	var enabled int
	var version int
	err = ctl.QueryRow(`SELECT enabled, IFNULL(daemon_url,''), IFNULL(daemon_user,''),
		IFNULL(daemon_password_enc,''), IFNULL(webhook_secret_enc,''), IFNULL(default_country,'VE'),
		IFNULL(risk_text_version,1), IFNULL(updated_by,''), IFNULL(updated_at,'')
		FROM msg_global ORDER BY id LIMIT 1`).
		Scan(&enabled, &g.DaemonURL, &g.DaemonUser, &g.DaemonPasswordEnc, &g.WebhookSecretEnc,
			&g.DefaultCountry, &version, &g.UpdatedBy, &g.UpdatedAt)
	if err != nil {
		return g
	}
	g.Enabled = enabled == 1
	g.RiskTextVersion = version
	return g
}

// msgSettingDe lee la configuración de la empresa.
func (s *Server) msgSettingDe(r *http.Request) msgSetting {
	st := msgSetting{DefaultCountry: mensajeria.PaisPorDefecto}
	db := s.dbFor(r)
	if db == nil {
		return st
	}
	var enabled, chat, otp int
	err := db.QueryRow(`SELECT enabled, IFNULL(risk_accepted_by,''), IFNULL(risk_accepted_at,''),
		IFNULL(default_country,'VE'), IFNULL(chat_enabled,0), IFNULL(otp_enabled,0),
		IFNULL(updated_by,''), IFNULL(updated_at,'')
		FROM msg_setting ORDER BY id LIMIT 1`).
		Scan(&enabled, &st.RiskAcceptedBy, &st.RiskAcceptedAt, &st.DefaultCountry, &chat, &otp,
			&st.UpdatedBy, &st.UpdatedAt)
	if err != nil {
		return st
	}
	st.Enabled = enabled == 1
	st.ChatEnabled = chat == 1
	st.OtpEnabled = otp == 1
	return st
}

// msgModuloActivo dice si la fila del módulo está encendida en esta empresa.
func (s *Server) msgModuloActivo(r *http.Request) bool {
	db := s.dbFor(r)
	if db == nil {
		return false
	}
	var activo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = 'mensajeria'`).Scan(&activo); err != nil {
		return false
	}
	return activo == 1
}

// mensajeriaActiva es la única puerta. Las tres llaves o nada.
func (s *Server) mensajeriaActiva(r *http.Request) bool {
	if !s.msgModuloActivo(r) {
		return false
	}
	if !s.msgGlobalDe(r).Enabled {
		return false
	}
	return s.msgSettingDe(r).Enabled
}

// ---------- Cliente del servicio ----------

// msgClienteGowa construye el cliente con la configuración guardada. Devuelve
// error cuando falta lo mínimo: una URL vacía no se puede «probar».
func (s *Server) msgClienteGowa(r *http.Request) (*gowa.Cliente, msgGlobal, error) {
	g := s.msgGlobalDe(r)
	if strings.TrimSpace(g.DaemonURL) == "" {
		return nil, g, fmt.Errorf("no hay dirección del servicio configurada. Vaya a Configuración de mensajería y póngala")
	}
	clave := ""
	if g.DaemonPasswordEnc != "" {
		var err error
		clave, err = security.Decrypt(g.DaemonPasswordEnc, security.DeriveKey(s.config.MasterKey))
		if err != nil {
			return nil, g, fmt.Errorf("no se pudo descifrar la credencial del servicio: %w", err)
		}
	}
	return gowa.Nuevo(g.DaemonURL, g.DaemonUser, clave), g, nil
}

// ---------- Pantallas ----------

func (s *Server) mensajeriaSettingsForm(w http.ResponseWriter, r *http.Request) {
	g := s.msgGlobalDe(r)
	st := s.msgSettingDe(r)
	s.render(w, r, "mensajeria_settings.html", map[string]interface{}{
		"Title":       "Mensajería",
		"Global":      g,
		"Empresa":     st,
		"Modulo":      s.msgModuloActivo(r),
		"Activa":      s.mensajeriaActiva(r),
		"Riesgo":      RiesgoMensajeria,
		"Proveedores": s.msgProveedores(r),
		"TieneClave":  g.DaemonPasswordEnc != "",
		"Nota": "El módulo nace apagado. Para que exista para los usuarios hacen falta las tres llaves: " +
			"el módulo visible, la instalación y la empresa. Mientras falte una, el menú no aparece y " +
			"las rutas responden 404.",
	})
}

// mensajeriaHome es la bandeja. En esta fase solo demuestra que el módulo está
// encendido; los dispositivos y el chat llegan en la fase siguiente.
func (s *Server) mensajeriaHome(w http.ResponseWriter, r *http.Request) {
	if !s.mensajeriaActiva(r) {
		// 404 a propósito: el módulo apagado no existe para los usuarios. Pero
		// el master necesita saber POR QUÉ y cómo encenderlo; un «Not Found»
		// seco fue justo lo que hizo pensar que la instalación estaba rota.
		if sess := s.sessionFromRequest(r); sess != nil && sess.UserLevel == "master" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<!doctype html><html lang="es"><head><meta charset="utf-8">
<title>Mensajería apagada</title></head>
<body style="font-family:system-ui,sans-serif;max-width:720px;margin:60px auto;line-height:1.5">
<h1>Mensajería está apagada</h1>
<p>El módulo existe, pero falta al menos una de las tres llaves: el módulo visible,
el servicio de la instalación o la empresa. Por eso esta ruta responde 404 para
el resto de usuarios.</p>
<p><a href="/settings/mensajeria">Configuración de mensajería</a> ·
<a href="/module/list">Módulos</a> ·
<a href="/settings/mensajeria/dispositivos">Dispositivos</a></p>
</body></html>`))
			return
		}
		notFoundOrJSON(w, r, "mensajería no disponible")
		return
	}
	s.render(w, r, "mensajeria_home.html", map[string]interface{}{
		"Title":       "Mensajería",
		"Global":      s.msgGlobalDe(r),
		"Empresa":     s.msgSettingDe(r),
		"Proveedores": s.msgProveedores(r),
	})
}

// mensajeriaColaForm muestra la cola y sus motivos. Es de solo lectura: la
// cola no se «arregla» a mano, se arregla el dispositivo, el horario o el
// teléfono de la ficha.
func (s *Server) mensajeriaColaForm(w http.ResponseWriter, r *http.Request) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "No hay base de control disponible.", http.StatusServiceUnavailable)
		return
	}
	conteo := map[string]int{}
	if rows, err := ctl.Query(`SELECT status, COUNT(*) FROM msg_outbox GROUP BY status`); err == nil {
		for rows.Next() {
			var estado string
			var n int
			if rows.Scan(&estado, &n) == nil {
				conteo[estado] = n
			}
		}
		_ = rows.Close()
	}
	recientes := []msgOutbox{}
	if rows, err := ctl.Query(msgOutboxSQL + ` ORDER BY id DESC LIMIT 100`); err == nil {
		for rows.Next() {
			if m, err := msgEscanearOutbox(rows); err == nil {
				recientes = append(recientes, m)
			}
		}
		_ = rows.Close()
	}
	s.render(w, r, "mensajeria_queue.html", map[string]interface{}{
		"Title":     "Cola de mensajería",
		"Conteo":    conteo,
		"Recientes": recientes,
		"Nota": "La cola no se vacía a mano. Un aviso bloqueado explica el motivo: " +
			"teléfono ambiguo, opt-out, cupo agotado o fuera de horario. Se corrige la ficha o el " +
			"dispositivo, no el mensaje.",
	})
}

// ---------- Guardado ----------

// mensajeriaGlobalSave mueve la llave de la instalación.
func (s *Server) mensajeriaGlobalSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "No hay base de control disponible.", http.StatusServiceUnavailable)
		return
	}

	antes := s.msgGlobalDe(r)
	nuevo := antes
	nuevo.Enabled = r.FormValue("enabled") == "1"
	nuevo.DaemonURL = strings.TrimSpace(r.FormValue("daemon_url"))
	nuevo.DaemonUser = strings.TrimSpace(r.FormValue("daemon_user"))
	if pais := strings.ToUpper(strings.TrimSpace(r.FormValue("default_country"))); pais != "" {
		nuevo.DefaultCountry = pais
	}
	nuevo.UpdatedBy = s.sessionUserID(r)
	nuevo.UpdatedAt = now()

	if nuevo.DaemonURL != "" {
		if err := validarURLDeServicio(nuevo.DaemonURL); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// La contraseña vacía significa «no la cambies»; solo se cifra cuando llega
	// una nueva. Nunca se devuelve al navegador.
	if clave := r.FormValue("daemon_password"); clave != "" {
		cifrada, err := security.Encrypt(clave, security.DeriveKey(s.config.MasterKey))
		if err != nil {
			jsonError(w, "No se pudo guardar la credencial: "+err.Error(), http.StatusInternalServerError)
			return
		}
		nuevo.DaemonPasswordEnc = cifrada
	}

	if nuevo.Enabled {
		if nuevo.DaemonURL == "" {
			jsonError(w, "Para encender el servicio hace falta la dirección del daemon (por ejemplo http://127.0.0.1:3000).", http.StatusBadRequest)
			return
		}
		if nuevo.DaemonUser == "" || nuevo.DaemonPasswordEnc == "" {
			jsonError(w, "Para encender el servicio hacen falta usuario y contraseña: el daemon no debe quedar abierto en la red.", http.StatusBadRequest)
			return
		}
	}

	if err := s.msgGuardarGlobal(ctl, nuevo); err != nil {
		jsonError(w, "No se pudo guardar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.audit(r, accionSegun(nuevo.Enabled, antes.Enabled), "mensajeria", "msg_global", "1",
		map[string]interface{}{"enabled": antes.Enabled, "daemon_url": antes.DaemonURL},
		map[string]interface{}{"enabled": nuevo.Enabled, "daemon_url": nuevo.DaemonURL})

	msg := "Configuración de la instalación guardada. El servicio queda apagado."
	if nuevo.Enabled {
		msg = "Servicio encendido. Todavía falta la llave de la empresa y el módulo visible para que aparezca."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg})
}

// mensajeriaEmpresaSave mueve la llave de la empresa y exige el riesgo aceptado.
func (s *Server) mensajeriaEmpresaSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	db := s.dbFor(r)
	if db == nil {
		jsonError(w, "No hay base de empresa disponible.", http.StatusServiceUnavailable)
		return
	}

	antes := s.msgSettingDe(r)
	nuevo := antes
	nuevo.Enabled = r.FormValue("enabled") == "1"
	if pais := strings.ToUpper(strings.TrimSpace(r.FormValue("default_country"))); pais != "" {
		nuevo.DefaultCountry = pais
	}
	nuevo.UpdatedBy = s.sessionUserID(r)
	nuevo.UpdatedAt = now()

	if nuevo.Enabled {
		// Sin la llave de la instalación no se enciende: el botón se explica,
		// no se esconde.
		if !s.msgGlobalDe(r).Enabled {
			jsonError(w, "La instalación tiene el servicio apagado. Encienda primero «Servicio de la instalación» en esta misma pantalla.", http.StatusBadRequest)
			return
		}
		// El riesgo se acepta UNA vez y queda registrado con nombre y fecha.
		// Volver a apagar y encender no borra esa huella.
		if nuevo.RiskAcceptedAt == "" {
			if r.FormValue("risk_accepted") != "1" {
				jsonError(w, "Antes de encender la mensajería hay que leer y aceptar el texto de riesgo. No es una casilla decorativa: sin esa aceptación el módulo no se enciende.", http.StatusBadRequest)
				return
			}
			responsable := strings.TrimSpace(r.FormValue("risk_responsable"))
			if responsable == "" {
				responsable = s.sessionUserID(r)
			}
			nuevo.RiskAcceptedBy = responsable
			nuevo.RiskAcceptedAt = now()
		}
	}

	if err := s.msgGuardarSetting(db, nuevo); err != nil {
		jsonError(w, "No se pudo guardar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.audit(r, accionSegun(nuevo.Enabled, antes.Enabled), "mensajeria", "msg_setting", "1",
		map[string]interface{}{"enabled": antes.Enabled},
		map[string]interface{}{"enabled": nuevo.Enabled, "risk_accepted_by": nuevo.RiskAcceptedBy, "risk_accepted_at": nuevo.RiskAcceptedAt})

	msg := "La empresa queda con la mensajería apagada. Los mensajes y las sesiones se conservan."
	if nuevo.Enabled {
		msg = "Empresa encendida. Falta que el módulo esté visible para que aparezca en el menú."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg})
}

// mensajeriaModuloToggle enciende o apaga la tercera llave desde la misma
// pantalla. Es la misma operación que el interruptor de /module/list, con la
// misma auditoría, para no obligar al master a buscar dos sitios.
func (s *Server) mensajeriaModuloToggle(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	if db == nil {
		jsonError(w, "No hay base de empresa disponible.", http.StatusServiceUnavailable)
		return
	}
	antes := s.msgModuloActivo(r)
	nuevo := !antes
	res, err := db.Exec(`UPDATE modules SET is_active = ? WHERE id = 'mensajeria'`, boolAInt(nuevo))
	if err != nil {
		jsonError(w, "No se pudo cambiar el módulo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "El módulo de mensajería no está instalado en esta base (falta la migración).", http.StatusConflict)
		return
	}
	invalidarCacheMenu()
	s.audit(r, accionSegun(nuevo, antes), "mensajeria", "modules", "mensajeria",
		map[string]interface{}{"is_active": antes}, map[string]interface{}{"is_active": nuevo})

	msg := "Módulo oculto: el menú desaparece y las rutas responden 404. Los datos se conservan."
	if nuevo {
		msg = "Módulo visible. Si la instalación y la empresa están encendidas, ya aparece en el menú."
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg, "is_active": nuevo})
}

// mensajeriaProbar comprueba que el daemon responde y qué versión tiene.
func (s *Server) mensajeriaProbar(w http.ResponseWriter, r *http.Request) {
	cliente, g, err := s.msgClienteGowa(r)
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": 0, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := cliente.Salud(ctx); err != nil {
		writeJSON(w, map[string]interface{}{"status": 0,
			"error": "El servicio no responde en " + g.DaemonURL + ": " + err.Error()})
		return
	}
	version, verr := cliente.Version(ctx)
	if verr != nil {
		writeJSON(w, map[string]interface{}{"status": 0,
			"error": "El servicio responde pero no dio su versión: " + verr.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1,
		"msg": "El servicio responde. Versión " + version + "."})
}

// ---------- Persistencia portable ----------

// msgGuardarGlobal hace UPDATE si ya hay fila e INSERT si no la hay.
//
// NO se usa RowsAffected para decidir: MySQL devuelve 0 cuando el UPDATE no
// CAMBIA nada (mismos valores), y entonces se insertaría una segunda fila de
// configuración. Se pregunta primero si existe, que es portable y no depende de
// esa sutileza.
func (s *Server) msgGuardarGlobal(db *sql.DB, g msgGlobal) error {
	var hay int
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_global`).Scan(&hay); err != nil {
		return err
	}
	if hay > 0 {
		_, err := db.Exec(`UPDATE msg_global SET enabled=?, daemon_url=?, daemon_user=?,
			daemon_password_enc=?, webhook_secret_enc=?, default_country=?, risk_text_version=?,
			updated_by=?, updated_at=?`,
			boolAInt(g.Enabled), g.DaemonURL, g.DaemonUser, g.DaemonPasswordEnc, g.WebhookSecretEnc,
			g.DefaultCountry, g.RiskTextVersion, g.UpdatedBy, g.UpdatedAt)
		return err
	}
	_, err := db.Exec(`INSERT INTO msg_global (enabled, daemon_url, daemon_user, daemon_password_enc,
		webhook_secret_enc, default_country, risk_text_version, updated_by, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		boolAInt(g.Enabled), g.DaemonURL, g.DaemonUser, g.DaemonPasswordEnc, g.WebhookSecretEnc,
		g.DefaultCountry, g.RiskTextVersion, g.UpdatedBy, g.UpdatedAt)
	return err
}

func (s *Server) msgGuardarSetting(db *sql.DB, st msgSetting) error {
	var hay int
	if err := db.QueryRow(`SELECT COUNT(*) FROM msg_setting`).Scan(&hay); err != nil {
		return err
	}
	if hay > 0 {
		_, err := db.Exec(`UPDATE msg_setting SET enabled=?, risk_accepted_by=?, risk_accepted_at=?,
			default_country=?, chat_enabled=?, otp_enabled=?, updated_by=?, updated_at=?`,
			boolAInt(st.Enabled), st.RiskAcceptedBy, st.RiskAcceptedAt, st.DefaultCountry,
			boolAInt(st.ChatEnabled), boolAInt(st.OtpEnabled), st.UpdatedBy, st.UpdatedAt)
		return err
	}
	_, err := db.Exec(`INSERT INTO msg_setting (enabled, risk_accepted_by, risk_accepted_at,
		default_country, chat_enabled, otp_enabled, updated_by, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		boolAInt(st.Enabled), st.RiskAcceptedBy, st.RiskAcceptedAt, st.DefaultCountry,
		boolAInt(st.ChatEnabled), boolAInt(st.OtpEnabled), st.UpdatedBy, st.UpdatedAt)
	return err
}

// msgProveedores lista los proveedores de la empresa. Si la tabla no existe
// (base sin migrar) devuelve la lista vacía: la pantalla lo dice, no falla.
func (s *Server) msgProveedores(r *http.Request) []msgProvider {
	out := []msgProvider{}
	db := s.dbFor(r)
	if db == nil {
		return out
	}
	rows, err := db.Query(`SELECT provider, enabled, IFNULL(config_json,'') FROM msg_provider ORDER BY provider`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p msgProvider
		var enabled int
		if err := rows.Scan(&p.Provider, &enabled, &p.Config); err != nil {
			continue
		}
		p.Enabled = enabled == 1
		out = append(out, p)
	}
	return out
}

// ---------- Auxiliares ----------

// validarURLDeServicio exige loopback. El daemon de WhatsApp maneja la sesión
// del número: exponerlo a la red es entregar la sesión a cualquiera que llegue
// al puerto. Si algún día se quiere otro host, se cambia aquí y se documenta.
func validarURLDeServicio(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("la dirección del servicio no es una URL válida")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("la dirección del servicio debe empezar por http:// o https://")
	}
	switch strings.ToLower(u.Hostname()) {
	case "127.0.0.1", "localhost", "::1":
		return nil
	}
	return fmt.Errorf("el servicio debe escuchar solo en este equipo (127.0.0.1 o localhost); " +
		"abrir su puerto a la red entrega la sesión de WhatsApp")
}

func boolAInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// msgDescifrar abre un secreto guardado. Un valor vacío no es un error: es «no
// hay nada guardado», y quien llama decide si eso es suficiente.
func (s *Server) msgDescifrar(v string) (string, error) {
	if strings.TrimSpace(v) == "" {
		return "", nil
	}
	return security.Decrypt(v, security.DeriveKey(s.config.MasterKey))
}

func accionSegun(nuevo, antes bool) string {
	switch {
	case nuevo && !antes:
		return "activar"
	case !nuevo && antes:
		return "desactivar"
	default:
		return "guardar"
	}
}

// paisDeLaEmpresa resuelve el país para normalizar: empresa, luego instalación,
// luego el de fábrica. Se deja aquí para que todos los llamadores usen el mismo
// orden y no haya dos ideas de «el país».
func (s *Server) paisDeLaEmpresa(r *http.Request) string {
	if p := strings.TrimSpace(s.msgSettingDe(r).DefaultCountry); p != "" {
		return p
	}
	if p := strings.TrimSpace(s.msgGlobalDe(r).DefaultCountry); p != "" {
		return p
	}
	return mensajeria.PaisPorDefecto
}

// normalizarTelefonoDeLaEmpresa aplica la tabla del país configurado.
func (s *Server) normalizarTelefonoDeLaEmpresa(r *http.Request, raw string) (string, error) {
	return mensajeria.Normalizar(raw, s.paisDeLaEmpresa(r))
}

// enteroDeForm es un atajo para los campos numéricos de la pantalla.
func enteroDeForm(v string, porDefecto int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return porDefecto
	}
	return n
}
