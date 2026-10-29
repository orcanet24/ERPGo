package web

// mensajeria_outbox.go — La cola única de salida y su despachador.
//
// NADA SALE EN EL MOMENTO. Un handler escribe en `msg_outbox` y vuelve; un
// despachador propio (una goroutine, un tic de 5 s) decide cuándo se puede
// mandar de verdad. Así el usuario no espera a WhatsApp y el ritmo no depende de
// que alguien tenga la pantalla abierta.
//
// POR QUÉ NO SE USA internal/queue. Esa cola tiene dos workers fijos para IA y
// reportes. Un envío con una pausa humana de dos minutos bloquearía un worker y
// frenaría tareas que no tienen nada que ver. Aquí hay UN despachador, que es
// todo lo que hace falta: el ritmo es secuencial por dispositivo.
//
// LA COLA VIVE EN CONTROL, EL HISTORIAL EN LA EMPRESA. La cola es
// infraestructura compartida (como queue_tasks); el mensaje enviado es dato de
// negocio y se escribe en la base de su empresa. El despachador abre esa base
// por el `tenant` de la fila y NUNCA cae a la empresa activa.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"erpgo/internal/mensajeria"
	"erpgo/internal/mensajeria/gowa"
)

// Estados de una fila de la cola.
const (
	msgEnCola    = "en_cola"
	msgEnviando  = "enviando"
	msgAceptado  = "aceptado"
	msgFallido   = "fallido"
	msgBloqueado = "bloqueado"
	msgRevision  = "en_revision"
)

// msgOutbox es una fila de la cola.
type msgOutbox struct {
	ID           int64
	Tenant       string
	Provider     string
	DeviceID     string
	Priority     string
	Kind         string
	ToMSISDN     string
	Body         string
	MediaPath    string
	TemplateCode string
	RefType      string
	RefID        string
	IdemKey      string
	Status       string
	Attempts     int
	SendAfter    string
	ClaimedAt    string
	ProviderRef  string
	Error        string
	CreatedBy    string
	CreatedAt    string
	UpdatedAt    string
}

const msgOutboxSQL = `SELECT id, tenant, provider, device_id, priority, kind, to_msisdn,
	IFNULL(body,''), IFNULL(media_path,''), IFNULL(template_code,''), IFNULL(ref_type,''),
	IFNULL(ref_id,''), idem_key, status, attempts, IFNULL(send_after,''), IFNULL(claimed_at,''),
	IFNULL(provider_ref,''), IFNULL(error,''), IFNULL(created_by,''), IFNULL(created_at,''),
	IFNULL(updated_at,'')
	FROM msg_outbox`

func msgEscanearOutbox(s interface {
	Scan(dest ...interface{}) error
}) (msgOutbox, error) {
	var m msgOutbox
	err := s.Scan(&m.ID, &m.Tenant, &m.Provider, &m.DeviceID, &m.Priority, &m.Kind, &m.ToMSISDN,
		&m.Body, &m.MediaPath, &m.TemplateCode, &m.RefType, &m.RefID, &m.IdemKey, &m.Status,
		&m.Attempts, &m.SendAfter, &m.ClaimedAt, &m.ProviderRef, &m.Error, &m.CreatedBy,
		&m.CreatedAt, &m.UpdatedAt)
	return m, err
}

// msgPrioridadRank ordena la cola: un OTP va antes que un recordatorio de cobro.
// El rango se calcula en SQL para que la consulta no dependa de un ORDER BY
// alfabético, que pondría «marketing» primero.
const msgPrioridadRank = `CASE priority
	WHEN 'otp' THEN 0 WHEN 'transaccional' THEN 1 WHEN 'recordatorio' THEN 2 ELSE 3 END`

// msgClaveDeIdempotencia arma la clave si el llamador no la dio. Incluye el DÍA
// para que el mismo aviso de mañana sea una decisión nueva y legítima, y el
// mismo de hoy no salga dos veces.
func msgClaveDeIdempotencia(m msgOutbox) string {
	if strings.TrimSpace(m.IdemKey) != "" {
		return m.IdemKey
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		m.Tenant, m.Provider, m.DeviceID, m.Kind, m.ToMSISDN, m.RefType, m.RefID,
		m.TemplateCode, m.Body, time.Now().Format("2006-01-02"),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

// msgEncolar inserta un envío. Devuelve `nuevo=false` cuando la clave ya
// existía: eso NO es un error, es la idempotencia haciendo su trabajo.
func (s *Server) msgEncolar(ctl *sql.DB, m msgOutbox) (int64, bool, error) {
	if strings.TrimSpace(m.Status) == "" {
		m.Status = msgEnCola
	}
	if strings.TrimSpace(m.Priority) == "" {
		m.Priority = "transaccional"
	}
	if strings.TrimSpace(m.Provider) == "" {
		m.Provider = "whatsapp_gowa"
	}
	if strings.TrimSpace(m.SendAfter) == "" {
		m.SendAfter = now()
	}
	m.IdemKey = msgClaveDeIdempotencia(m)

	res, err := ctl.Exec(`INSERT INTO msg_outbox (tenant, provider, device_id, priority, kind, to_msisdn,
		body, media_path, template_code, ref_type, ref_id, idem_key, status, attempts, send_after,
		created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?)`,
		m.Tenant, m.Provider, m.DeviceID, m.Priority, m.Kind, m.ToMSISDN, m.Body, m.MediaPath,
		m.TemplateCode, m.RefType, m.RefID, m.IdemKey, m.Status, m.SendAfter,
		m.CreatedBy, now(), now())
	if err != nil {
		if aiEsClaveDuplicada(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	id, _ := res.LastInsertId()
	return id, true, nil
}

// msgReclamar toma hasta `limite` filas listas para enviar y las marca
// `enviando` con un UPDATE condicional. El UPDATE es el candado: si dos
// despachadores pidieran la misma fila, solo uno gana.
func (s *Server) msgReclamar(ctl *sql.DB, ahora string, limite int) ([]msgOutbox, error) {
	rows, err := ctl.Query(msgOutboxSQL+` WHERE status = ? AND (send_after = '' OR send_after <= ?)
		ORDER BY `+msgPrioridadRank+`, id LIMIT ?`, msgEnCola, ahora, limite)
	if err != nil {
		return nil, err
	}
	var candidatos []msgOutbox
	for rows.Next() {
		if m, err := msgEscanearOutbox(rows); err == nil {
			candidatos = append(candidatos, m)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var reclamados []msgOutbox
	for _, m := range candidatos {
		res, err := ctl.Exec(`UPDATE msg_outbox SET status=?, claimed_at=?, updated_at=?
			WHERE id=? AND status=?`, msgEnviando, ahora, ahora, m.ID, msgEnCola)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 1 {
			m.Status = msgEnviando
			m.ClaimedAt = ahora
			reclamados = append(reclamados, m)
		}
	}
	return reclamados, nil
}

// msgRecuperarAtascados devuelve a la cola lo que quedó «enviando» tras una
// caída del proceso. Sin esto, un mensaje a medio enviar se quedaría para
// siempre en ese estado.
func (s *Server) msgRecuperarAtascados(ctl *sql.DB) {
	limite := time.Now().Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	_, _ = ctl.Exec(`UPDATE msg_outbox SET status=?, error='recuperado tras reinicio', updated_at=?
		WHERE status=? AND claimed_at <> '' AND claimed_at < ?`,
		msgEnCola, now(), msgEnviando, limite)
}

// ---------- Despacho ----------

// startMensajeriaDispatcher arranca el único despachador. Es idempotente: dos
// llamadas no crean dos goroutines.
func (s *Server) startMensajeriaDispatcher() {
	if s.msgStop != nil {
		return
	}
	s.msgStop = make(chan struct{})
	ctl, err := s.controlStore()
	if err == nil && ctl != nil {
		s.msgRecuperarAtascados(ctl)
	}
	go func(stop chan struct{}) {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.msgDespacharUnaVez(context.Background(), time.Now())
			}
		}
	}(s.msgStop)
}

func (s *Server) pararMensajeriaDispatcher() {
	if s.msgStop != nil {
		close(s.msgStop)
		s.msgStop = nil
	}
}

// msgDespacharUnaVez procesa lo que esté listo. `ahora` se pasa como parámetro
// para que las pruebas puedan fijar el reloj sin esperar.
func (s *Server) msgDespacharUnaVez(ctx context.Context, ahora time.Time) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		return
	}
	if !msgGlobalActivo(ctl) {
		return // el servicio está apagado: la cola se conserva, no se toca
	}
	reclamados, err := s.msgReclamar(ctl, ahora.Format("2006-01-02 15:04:05"), 5)
	if err != nil {
		return
	}
	for _, m := range reclamados {
		s.msgDespacharFila(ctx, ctl, m, ahora)
	}
}

// msgGlobalActivo lee la llave de la instalación sin petición HTTP.
func msgGlobalActivo(ctl *sql.DB) bool {
	var activo int
	if err := ctl.QueryRow(`SELECT enabled FROM msg_global ORDER BY id LIMIT 1`).Scan(&activo); err != nil {
		return false
	}
	return activo == 1
}

// msgDespacharFila decide si esta fila puede salir ahora y la envía.
func (s *Server) msgDespacharFila(ctx context.Context, ctl *sql.DB, m msgOutbox, ahora time.Time) {
	dispositivo, err := s.msgDeviceDe(ctl, m.DeviceID)
	if err != nil || dispositivo.State == "retirado" {
		s.msgCerrar(ctl, m.ID, msgBloqueado, "el dispositivo no existe o está retirado")
		return
	}
	if dispositivo.State != "connected" {
		s.msgReencolar(ctl, m.ID, ahora.Add(10*time.Minute), "el dispositivo no está conectado")
		return
	}
	if pausado := msgPausadoHasta(dispositivo); pausado.After(ahora) {
		s.msgReencolar(ctl, m.ID, pausado, "el dispositivo está en pausa")
		return
	}
	if siguiente := msgFechaDe(dispositivo.NextSendAt); siguiente.After(ahora) {
		s.msgReencolar(ctl, m.ID, siguiente, "")
		return
	}

	db, err := s.tenantDB(m.Tenant)
	if err != nil || db == nil {
		s.msgReencolar(ctl, m.ID, ahora.Add(5*time.Minute), "la base de la empresa no está disponible")
		return
	}

	// Horario: fuera de la ventana no se bloquea, se reprograma para la próxima
	// apertura. Bloquearlo perdería el aviso; reprogramarlo lo envía mañana a
	// primera hora, que es lo que el cliente espera.
	politica := msgPoliticaDe(dispositivo)
	if !mensajeria.EnVentana(ahora, politica) {
		s.msgReencolar(ctl, m.ID, mensajeria.ProximaVentana(ahora, politica), "fuera del horario de envío")
		return
	}

	if motivo := msgMotivoDeBloqueo(db, dispositivo, m, ahora); motivo != "" {
		s.msgCerrar(ctl, m.ID, msgBloqueado, motivo)
		return
	}

	cliente, err := s.msgClienteSinRequest(ctl)
	if err != nil {
		s.msgReencolar(ctl, m.ID, ahora.Add(5*time.Minute), err.Error())
		return
	}

	// Presencia «escribiendo»: se manda antes y se retira después. La pausa
	// entre ambos solo corre en producción (msgTyping); en las pruebas sería
	// tiempo perdido y no cambia la decisión.
	if s.msgTyping {
		_ = cliente.Presencia(ctx, m.DeviceID, m.ToMSISDN, "start")
		time.Sleep(2*time.Second + time.Duration(rand.Intn(4))*time.Second)
		defer func() { _ = cliente.Presencia(ctx, m.DeviceID, m.ToMSISDN, "stop") }()
	}

	envio, err := cliente.EnviarTexto(ctx, m.DeviceID, m.ToMSISDN, m.Body)
	if err != nil {
		s.msgRegistrarFallo(ctl, m, err, ahora)
		return
	}

	s.msgRegistrarEnvio(db, ctl, dispositivo, m, envio, ahora)
}

// msgMotivoDeBloqueo devuelve por qué NO se puede enviar, o "" si se puede.
// El motivo se guarda en la fila: un aviso bloqueado sin explicación es un
// aviso que alguien va a reintentar a ciegas.
func msgMotivoDeBloqueo(db *sql.DB, d msgDevice, m msgOutbox, ahora time.Time) string {
	if m.ToMSISDN == "" {
		return "la ficha no tiene un teléfono normalizado"
	}
	if msgEstaEnOptOut(db, m.ToMSISDN) && m.Kind != "otp" && m.Kind != "transaccional" {
		return "el contacto pidió no recibir más mensajes (opt-out)"
	}
	if motivo := msgCupoAgotado(db, d, ahora); motivo != "" {
		return motivo
	}
	return ""
}

// msgDentroDeVentana se conserva para las pruebas y para quien necesite la
// pregunta suelta; el despachador usa ProximaVentana para reprogramar.
func msgDentroDeVentana(ahora time.Time, d msgDevice) bool {
	return mensajeria.EnVentana(ahora, msgPoliticaDe(d))
}

func msgPoliticaDe(d msgDevice) mensajeria.PoliticaDeRitmo {
	p := mensajeria.PoliticaPorDefecto()
	if d.MinDelayS > 0 {
		p.PausaMinS = d.MinDelayS
	}
	if d.MaxDelayS > 0 {
		p.PausaMaxS = d.MaxDelayS
	}
	if d.DailyCap > 0 {
		p.CupoDiario = d.DailyCap
	}
	if d.HourlyCap > 0 {
		p.CupoHorario = d.HourlyCap
	}
	return p
}

// msgCupoAgotado cuenta lo enviado hoy y en la última hora. El cupo diario baja
// mientras el número es nuevo: un número recién vinculado que manda 120
// mensajes el primer día es la señal más clara de automatización.
func msgCupoAgotado(db *sql.DB, d msgDevice, ahora time.Time) string {
	p := msgPoliticaDe(d)
	desdeDia := ahora.Format("2006-01-02") + " 00:00:00"
	desdeHora := ahora.Add(-time.Hour).Format("2006-01-02 15:04:05")

	var hoy, hora int
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_message WHERE device_id=? AND direction='out' AND created_at >= ?`,
		d.DeviceID, desdeDia).Scan(&hoy)
	_ = db.QueryRow(`SELECT COUNT(*) FROM msg_message WHERE device_id=? AND direction='out' AND created_at >= ?`,
		d.DeviceID, desdeHora).Scan(&hora)

	if hoy >= msgCupoDiarioEfectivo(d, p) {
		return "cupo diario agotado"
	}
	if hora >= p.CupoHorario {
		return "cupo por hora agotado"
	}
	return ""
}

// msgCupoDiarioEfectivo aplica el calentamiento según la antigüedad del número.
// Se calcula desde created_at y no desde un contador que alguien tenga que
// actualizar: un contador que se olvida deja el número en 20 para siempre.
func msgCupoDiarioEfectivo(d msgDevice, p mensajeria.PoliticaDeRitmo) int {
	creado := msgFechaDe(d.CreatedAt)
	if creado.IsZero() {
		return p.CupoDiario
	}
	dias := int(time.Since(creado).Hours() / 24)
	cupo := mensajeria.CupoDeCalentamiento(dias)
	if p.CupoDiario < cupo {
		return p.CupoDiario
	}
	return cupo
}

// msgRegistrarEnvio deja el mensaje en el historial de la empresa y programa la
// siguiente ventana del dispositivo.
func (s *Server) msgRegistrarEnvio(db, ctl *sql.DB, d msgDevice, m msgOutbox, envio gowa.Envio, ahora time.Time) {
	estado := msgAceptado
	if envio.Estado != "" {
		estado = envio.Estado
	}
	_, _ = db.Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		from_msisdn, to_msisdn, body, status, is_from_me, sent_at, received_at, raw_json, created_at)
		VALUES (?,?,?, 'out', ?, '', ?, ?, ?, 1, ?, '', '', ?)`,
		m.DeviceID, m.Provider, m.ToMSISDN, primeroNoVacio(envio.MensajeID, m.IdemKey),
		m.ToMSISDN, m.Body, estado, ahora.Format("2006-01-02 15:04:05"), now())

	pausa := mensajeria.PausaEntreMensajes(msgPoliticaDe(d), nil)
	if larga, hay := mensajeria.PausaLarga(nil); hay {
		pausa += larga
	}
	_, _ = ctl.Exec(`UPDATE msg_outbox SET status=?, provider_ref=?, error='', updated_at=? WHERE id=?`,
		estado, envio.MensajeID, now(), m.ID)
	_, _ = ctl.Exec(`UPDATE msg_device SET next_send_at=?, last_seen_at=? WHERE device_id=?`,
		ahora.Add(pausa).Format("2006-01-02 15:04:05"), now(), d.DeviceID)
}

// msgRegistrarFallo decide entre reintentar y cerrar. Un 401/403 no se
// reintenta: la credencial está mal y reintentar solo gasta tiempo. Un 500 o un
// corte de red sí.
func (s *Server) msgRegistrarFallo(ctl *sql.DB, m msgOutbox, err error, ahora time.Time) {
	temporal := true
	if fe, ok := err.(*gowa.Error); ok {
		temporal = fe.EsTemporal()
	}
	motivo := err.Error()
	if len(motivo) > 450 {
		motivo = motivo[:450]
	}

	if !temporal {
		s.msgCerrar(ctl, m.ID, msgFallido, motivo)
		// Credenciales malas o dispositivo inexistente: se pausa el número para
		// que no siga intentando con cada mensaje de la cola.
		if fe, ok := err.(*gowa.Error); ok && (fe.Status == 401 || fe.Status == 403 || fe.Status == 404) {
			_, _ = ctl.Exec(`UPDATE msg_device SET paused_until=? WHERE device_id=?`,
				ahora.Add(time.Hour).Format("2006-01-02 15:04:05"), m.DeviceID)
		}
		return
	}

	intento := m.Attempts + 1
	espera, hay := mensajeria.RetardoDeReintento(intento)
	if !hay {
		s.msgCerrar(ctl, m.ID, msgFallido, motivo)
		return
	}
	_, _ = ctl.Exec(`UPDATE msg_outbox SET status=?, attempts=?, send_after=?, error=?, updated_at=? WHERE id=?`,
		msgEnCola, intento, ahora.Add(espera).Format("2006-01-02 15:04:05"), motivo, now(), m.ID)
	s.msgAbrirCircuitoSiHaceFalta(ctl, m.DeviceID, ahora)
}

// msgAbrirCircuitoSiHaceFalta pausa el número una hora tras cinco fallos
// seguidos. Insistir con un número que ya está fallando es la forma más rápida
// de perderlo.
func (s *Server) msgAbrirCircuitoSiHaceFalta(ctl *sql.DB, deviceID string, ahora time.Time) {
	desde := ahora.Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	var fallos int
	// Se cuentan los fallos, no solo los definitivos: un mensaje que se
	// reintenta sigue siendo un fallo y, si van cinco seguidos, el número está
	// en problemas aunque ninguno se haya cerrado todavía.
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM msg_outbox WHERE device_id=? AND error <> '' AND updated_at >= ?`,
		deviceID, desde).Scan(&fallos)
	if fallos >= 5 {
		_, _ = ctl.Exec(`UPDATE msg_device SET paused_until=? WHERE device_id=?`,
			ahora.Add(time.Hour).Format("2006-01-02 15:04:05"), deviceID)
	}
}

func (s *Server) msgCerrar(ctl *sql.DB, id int64, estado, motivo string) {
	_, _ = ctl.Exec(`UPDATE msg_outbox SET status=?, error=?, updated_at=? WHERE id=?`,
		estado, motivo, now(), id)
}

func (s *Server) msgReencolar(ctl *sql.DB, id int64, cuando time.Time, motivo string) {
	_, _ = ctl.Exec(`UPDATE msg_outbox SET status=?, send_after=?, error=?, updated_at=? WHERE id=?`,
		msgEnCola, cuando.Format("2006-01-02 15:04:05"), motivo, now(), id)
}

// msgEncolarAviso mete un aviso financiero en la cola del dispositivo del canal.
//
// Devuelve la referencia «outbox:<id>» que se guarda en finance_message. El
// aviso NO se envía aquí: se envía cuando el despachador lo decida, con el ritmo
// y el cupo del número. Decir «enviado» en este punto sería mentir.
func (s *Server) msgEncolarAviso(r *http.Request, canal notifyChannel, destino, texto,
	refType, refID, usuario string) (string, error) {
	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		return "", fmt.Errorf("no hay base de control para la cola de mensajería")
	}
	// El teléfono se normaliza AQUÍ, antes de encolar. Enviar «0414-…» tal cual
	// es el fallo silencioso clásico: el mensaje «sale» y no llega.
	msisdn, err := s.normalizarTelefonoDeLaEmpresa(r, destino)
	if err != nil {
		return "", fmt.Errorf("el teléfono de la ficha no se pudo normalizar: %w", err)
	}
	id, nuevo, err := s.msgEncolar(ctl, msgOutbox{
		Tenant: s.tenantOf(r), Provider: "whatsapp_gowa", DeviceID: canal.MsgDeviceID,
		Priority: "transaccional", Kind: "aviso", ToMSISDN: msisdn,
		Body: texto, RefType: refType, RefID: refID,
		CreatedBy: usuario,
	})
	if err != nil {
		return "", err
	}
	if !nuevo {
		return "", fmt.Errorf("ese mismo aviso ya estaba en la cola; no se duplica")
	}
	return fmt.Sprintf("outbox:%d", id), nil
}

// msgClienteSinRequest construye el cliente desde la configuración guardada,
// para el despachador (que no tiene petición ni sesión).
func (s *Server) msgClienteSinRequest(ctl *sql.DB) (*gowa.Cliente, error) {
	var urlServicio, usuario, claveEnc string
	if err := ctl.QueryRow(`SELECT IFNULL(daemon_url,''), IFNULL(daemon_user,''), IFNULL(daemon_password_enc,'')
		FROM msg_global ORDER BY id LIMIT 1`).Scan(&urlServicio, &usuario, &claveEnc); err != nil {
		return nil, fmt.Errorf("el servicio de mensajería no está configurado")
	}
	if strings.TrimSpace(urlServicio) == "" {
		return nil, fmt.Errorf("el servicio de mensajería no tiene dirección configurada")
	}
	clave, err := s.msgDescifrar(claveEnc)
	if err != nil {
		return nil, err
	}
	return gowa.Nuevo(urlServicio, usuario, clave), nil
}

func msgPausadoHasta(d msgDevice) time.Time { return msgFechaDe(d.PausedUntil) }

// msgFechaDe tolera vacío y formatos viejos: una fecha ilegible no puede
// detener la cola para siempre.
func msgFechaDe(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	for _, formato := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(formato, v, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}
