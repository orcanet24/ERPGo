package web

// mensajeria_dispositivos.go — Alta, QR y estado de los números.
//
// UN DISPOSITIVO ES UN NÚMERO. Su identificador es `<empresa>.<área>` y no se
// reutiliza nunca: si un número se da de baja, su fila queda como «retirado»
// para que un mensaje viejo no vuelva a caer en un número que ya no es de la
// empresa. Esa regla es la razón por la que la baja no borra la fila.
//
// EL NAVEGADOR NO HABLA CON EL DAEMON. El QR se pide al daemon, se descarga y
// se sirve por una ruta autenticada. Si el navegador pidiera la URL del daemon,
// el puerto tendría que estar abierto y la sesión quedaría a un `curl` de
// distancia.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"erpgo/internal/mensajeria"
	"erpgo/internal/security"
)

// areasDeMensajeria son los usos que puede tener un número. Se validan para que
// el identificador no se convierta en texto libre y para que las rutas por área
// de la fase 2 tengan un catálogo cerrado.
var areasDeMensajeria = map[string]string{
	"general":     "General",
	"cxc":         "Cuentas por cobrar",
	"cxp":         "Cuentas por pagar",
	"facturacion": "Facturación",
	"rrhh":        "Recursos humanos",
	"compras":     "Compras",
	"otp":         "Códigos de acceso",
	"chat":        "Atención al cliente",
}

// reDeviceID es la forma del identificador: minúsculas, sin espacios, con un
// punto que separa empresa y área.
var reDeviceID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,48}$`)

// msgDevice es una fila de `msg_device` (base de control).
type msgDevice struct {
	DeviceID         string
	Tenant           string
	Area             string
	Label            string
	Provider         string
	MSISDN           string
	WebhookSecretEnc string
	State            string
	JID              string
	DailyCap         int
	HourlyCap        int
	MinDelayS        int
	MaxDelayS        int
	CreatedBy        string
	CreatedAt        string
}

const msgDeviceSQL = `SELECT device_id, tenant, area, IFNULL(label,''), IFNULL(provider,'whatsapp_gowa'),
	IFNULL(msisdn,''), IFNULL(webhook_secret_enc,''), IFNULL(state,'nuevo'), IFNULL(jid,''),
	IFNULL(daily_cap,120), IFNULL(hourly_cap,25), IFNULL(min_delay_s,45), IFNULL(max_delay_s,180),
	IFNULL(created_by,''), IFNULL(created_at,'')
	FROM msg_device`

func msgEscanearDevice(s interface {
	Scan(dest ...interface{}) error
}) (msgDevice, error) {
	var d msgDevice
	err := s.Scan(&d.DeviceID, &d.Tenant, &d.Area, &d.Label, &d.Provider, &d.MSISDN,
		&d.WebhookSecretEnc, &d.State, &d.JID, &d.DailyCap, &d.HourlyCap, &d.MinDelayS,
		&d.MaxDelayS, &d.CreatedBy, &d.CreatedAt)
	return d, err
}

func (s *Server) msgDispositivosDe(ctl *sql.DB, tenant string) []msgDevice {
	out := []msgDevice{}
	rows, err := ctl.Query(msgDeviceSQL+` WHERE tenant = ? ORDER BY area, device_id`, tenant)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if d, err := msgEscanearDevice(rows); err == nil {
			out = append(out, d)
		}
	}
	return out
}

func (s *Server) msgDeviceDe(ctl *sql.DB, deviceID string) (msgDevice, error) {
	d, err := msgEscanearDevice(ctl.QueryRow(msgDeviceSQL+` WHERE device_id = ?`, deviceID))
	if err != nil {
		return d, fmt.Errorf("el dispositivo %q no existe", deviceID)
	}
	return d, nil
}

// msgDeviceSecreto descifra el secreto del webhook de un dispositivo.
func (s *Server) msgDeviceSecreto(d msgDevice) (string, error) {
	if d.WebhookSecretEnc == "" {
		return "", fmt.Errorf("el dispositivo no tiene secreto de webhook")
	}
	return security.Decrypt(d.WebhookSecretEnc, security.DeriveKey(s.config.MasterKey))
}

// msgWebhookURL arma la URL que se le da al daemon. Tiene que ser absoluta y
// alcanzable desde el daemon: por eso se usa APP_URL y se avisa si falta.
func (s *Server) msgWebhookURL(deviceID string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(s.config.AppURL), "/")
	if base == "" {
		return "", fmt.Errorf("falta APP_URL: el daemon necesita una dirección absoluta para entregar los eventos")
	}
	return base + "/webhooks/mensajeria/" + deviceID, nil
}

// ---------- Rutas ----------

func (s *Server) registerMensajeriaDispositivoRoutes() {
	s.mux.HandleFunc("GET /settings/mensajeria/dispositivos", s.mensajeriaDispositivosForm)
	s.mux.HandleFunc("POST /settings/mensajeria/dispositivos/crear", s.mensajeriaDeviceCreate)
	s.mux.HandleFunc("GET /settings/mensajeria/dispositivos/{id}/qr", s.mensajeriaDeviceQR)
	s.mux.HandleFunc("GET /settings/mensajeria/dispositivos/{id}/estado", s.mensajeriaDeviceEstado)
	s.mux.HandleFunc("POST /settings/mensajeria/dispositivos/{id}/reconectar", s.mensajeriaDeviceReconectar)
	s.mux.HandleFunc("POST /settings/mensajeria/dispositivos/{id}/logout", s.mensajeriaDeviceLogout)
	s.mux.HandleFunc("POST /settings/mensajeria/dispositivos/{id}/borrar", s.mensajeriaDeviceBorrar)
	// El webhook NO lleva sesión: se autentica con HMAC (ver mensajeria_webhook.go).
	s.mux.HandleFunc("POST /webhooks/mensajeria/{device}", s.mensajeriaWebhook)
}

func (s *Server) mensajeriaDispositivosForm(w http.ResponseWriter, r *http.Request) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "No hay base de control disponible.", http.StatusServiceUnavailable)
		return
	}
	tenant := s.tenantOf(r)
	s.render(w, r, "mensajeria_devices.html", map[string]interface{}{
		"Title":        "Dispositivos de mensajería",
		"Dispositivos": s.msgDispositivosDe(ctl, tenant),
		"Tenant":       tenant,
		"Areas":        areasDeMensajeria,
		"Activa":       s.mensajeriaActiva(r),
		"Nota": "Cada número es un dispositivo. El identificador se forma con la empresa y el área " +
			"(por ejemplo «" + tenant + ".cxc») y no se reutiliza nunca: al dar de baja un número, su fila " +
			"queda retirada para que un mensaje viejo no vuelva a caer en él.",
	})
}

// mensajeriaDeviceCreate da de alta el número en el daemon y lo registra aquí.
func (s *Server) mensajeriaDeviceCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "No hay base de control disponible.", http.StatusServiceUnavailable)
		return
	}
	tenant := s.tenantOf(r)
	area := strings.ToLower(strings.TrimSpace(r.FormValue("area")))
	if _, ok := areasDeMensajeria[area]; !ok {
		jsonError(w, "Elija el área del número (general, cxc, cxp, facturación, rrhh, compras, otp o chat).", http.StatusBadRequest)
		return
	}
	deviceID := tenant + "." + area
	if !reDeviceID.MatchString(deviceID) {
		jsonError(w, "El identificador «"+deviceID+"» no es válido. Use minúsculas, números, punto, guion o guion bajo.", http.StatusBadRequest)
		return
	}

	// Un identificador retirado no se recicla: la fila existe justamente para
	// impedirlo.
	if _, err := s.msgDeviceDe(ctl, deviceID); err == nil {
		jsonError(w, "El dispositivo «"+deviceID+"» ya existe (aunque esté retirado). Use otra área o pida al administrador que lo revise: un número retirado no se reutiliza.", http.StatusConflict)
		return
	}

	cliente, _, err := s.msgClienteGowa(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	webhookURL, err := s.msgWebhookURL(deviceID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	secreto := randomHex(32)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := cliente.CrearDispositivo(ctx, deviceID, webhookURL, secreto, []string{"message", "message.ack"}); err != nil {
		jsonError(w, "El servicio no pudo crear el dispositivo: "+err.Error(), http.StatusBadGateway)
		return
	}

	cifrado, err := security.Encrypt(secreto, security.DeriveKey(s.config.MasterKey))
	if err != nil {
		_ = cliente.Borrar(ctx, deviceID)
		jsonError(w, "No se pudo guardar el secreto del webhook: "+err.Error(), http.StatusInternalServerError)
		return
	}
	etiqueta := strings.TrimSpace(r.FormValue("label"))
	if etiqueta == "" {
		etiqueta = areasDeMensajeria[area]
	}
	_, err = ctl.Exec(`INSERT INTO msg_device (device_id, tenant, area, label, provider, webhook_secret_enc,
		state, daily_cap, hourly_cap, min_delay_s, max_delay_s, created_by, created_at)
		VALUES (?,?,?,?,?,?, 'nuevo', 120, 25, 45, 180, ?, ?)`,
		deviceID, tenant, area, etiqueta, "whatsapp_gowa", cifrado, s.sessionUserID(r), now())
	if err != nil {
		// Si el alta local falla, se deshace la del daemon: un dispositivo que
		// existe allá y no aquí es un número que nadie puede apagar desde el ERP.
		_ = cliente.Borrar(ctx, deviceID)
		jsonError(w, "No se pudo registrar el dispositivo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.audit(r, "crear", "mensajeria", "msg_device", deviceID, nil,
		map[string]interface{}{"area": area, "label": etiqueta})
	writeJSON(w, map[string]interface{}{"status": 1,
		"msg": "Dispositivo «" + deviceID + "» creado. Pida el QR para vincular el teléfono."})
}

// mensajeriaDeviceQR descarga el QR y lo sirve como imagen.
func (s *Server) mensajeriaDeviceQR(w http.ResponseWriter, r *http.Request) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		http.Error(w, "sin base de control", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	d, err := s.msgDeviceDe(ctl, id)
	if err != nil || d.Tenant != s.tenantOf(r) {
		http.NotFound(w, r)
		return
	}
	cliente, _, err := s.msgClienteGowa(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	qr, err := cliente.Login(ctx, id)
	if err != nil {
		http.Error(w, "el servicio no devolvió el QR: "+err.Error(), http.StatusBadGateway)
		return
	}
	imagen, tipo, err := cliente.Descargar(ctx, qr.Enlace)
	if err != nil {
		http.Error(w, "no se pudo descargar el QR: "+err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(imagen)))
	_, _ = w.Write(imagen)
}

// mensajeriaDeviceEstado consulta al daemon y actualiza la fila local.
func (s *Server) mensajeriaDeviceEstado(w http.ResponseWriter, r *http.Request) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "sin base de control", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	d, err := s.msgDeviceDe(ctl, id)
	if err != nil || d.Tenant != s.tenantOf(r) {
		jsonError(w, "el dispositivo no existe", http.StatusNotFound)
		return
	}
	cliente, _, err := s.msgClienteGowa(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	est, err := cliente.Estado(ctx, id)
	if err != nil {
		// El estado local se conserva: que el daemon no conteste no significa
		// que la sesión esté cerrada, y borrar el estado haría perder el rastro.
		writeJSON(w, map[string]interface{}{"status": 0, "error": err.Error(), "estado_local": d.State})
		return
	}
	estado := est.Estado
	if estado == "" {
		if est.Conectado {
			estado = "connected"
		} else {
			estado = "desconocido"
		}
	}
	msisdn := ""
	if strings.TrimSpace(est.MSISDN) != "" {
		if m, err := mensajeria.Normalizar(est.MSISDN, s.paisDeLaEmpresa(r)); err == nil {
			msisdn = m
		}
	}
	// El número solo se escribe si está libre: el índice único impide que dos
	// dispositivos reclamen el mismo teléfono.
	if msisdn != "" {
		if _, err := ctl.Exec(`UPDATE msg_device SET state=?, jid=?, last_seen_at=? WHERE device_id=?`,
			estado, est.JID, now(), id); err == nil {
			_, _ = ctl.Exec(`UPDATE msg_device SET msisdn=? WHERE device_id=? AND (msisdn IS NULL OR msisdn='')`, msisdn, id)
		}
	} else {
		_, _ = ctl.Exec(`UPDATE msg_device SET state=?, jid=?, last_seen_at=? WHERE device_id=?`,
			estado, est.JID, now(), id)
	}
	writeJSON(w, map[string]interface{}{"status": 1, "state": estado, "jid": est.JID,
		"msisdn": mensajeria.Enmascarar(msisdn), "connected": estado == "connected"})
}

func (s *Server) mensajeriaDeviceReconectar(w http.ResponseWriter, r *http.Request) {
	s.mensajeriaDeviceAccion(w, r, "reconectar")
}

func (s *Server) mensajeriaDeviceLogout(w http.ResponseWriter, r *http.Request) {
	s.mensajeriaDeviceAccion(w, r, "logout")
}

func (s *Server) mensajeriaDeviceBorrar(w http.ResponseWriter, r *http.Request) {
	s.mensajeriaDeviceAccion(w, r, "borrar")
}

// mensajeriaDeviceAccion ejecuta una acción del daemon y deja la fila local
// coherente. La baja NO borra la fila: la marca «retirado» y libera el número.
func (s *Server) mensajeriaDeviceAccion(w http.ResponseWriter, r *http.Request, accion string) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "sin base de control", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	d, err := s.msgDeviceDe(ctl, id)
	if err != nil || d.Tenant != s.tenantOf(r) {
		jsonError(w, "el dispositivo no existe", http.StatusNotFound)
		return
	}
	cliente, _, err := s.msgClienteGowa(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch accion {
	case "reconectar":
		err = cliente.Reconectar(ctx, id)
		if err == nil {
			_, _ = ctl.Exec(`UPDATE msg_device SET state='reconectando' WHERE device_id=?`, id)
		}
	case "logout":
		err = cliente.CerrarSesion(ctx, id)
		if err == nil {
			_, _ = ctl.Exec(`UPDATE msg_device SET state='desconectado' WHERE device_id=?`, id)
		}
	case "borrar":
		err = cliente.Borrar(ctx, id)
		if err == nil {
			// `msisdn = NULL` libera el número para otro dispositivo; el
			// identificador queda ocupado para siempre.
			_, _ = ctl.Exec(`UPDATE msg_device SET state='retirado', msisdn=NULL WHERE device_id=?`, id)
		}
	}
	if err != nil {
		jsonError(w, "El servicio no pudo completar la acción: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.audit(r, accion, "mensajeria", "msg_device", id, nil, map[string]interface{}{"accion": accion})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Acción «" + accion + "» completada."})
}
