package web

// mensajeria_media.go — Archivos de la conversación.
//
// DÓNDE VIVEN. En `data/mensajeria/<empresa>/`, fuera de `static/` y fuera de
// `uploads/`. Un archivo de una conversación no es un recurso público: se sirve
// por una ruta autenticada que comprueba empresa y dispositivo. Guardar solo el
// nombre del archivo (no la ruta absoluta) permite mover el directorio de datos
// sin romper el historial, y `filepath.Base` impide que un nombre con `..` salga
// de la carpeta de su empresa.
//
// QUÉ SE ACEPTA. Imagen, audio, video y documento, con lista blanca de
// extensiones y tipo detectado por contenido. Un ejecutable renombrado a .jpg no
// pasa: se mira el tipo real, no el nombre.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// msgMediaMaximo es el tope de subida. WhatsApp ronda los 16 MB para imagen,
// audio y video; un documento grande se manda por correo o por enlace.
const msgMediaMaximo = 16 << 20

// msgExtensionesDeMedia es la lista blanca. La clave es la extensión y el valor
// el tipo que entiende el daemon.
var msgExtensionesDeMedia = map[string]string{
	".jpg": "image", ".jpeg": "image", ".png": "image", ".webp": "image", ".gif": "image",
	".ogg": "audio", ".opus": "audio", ".mp3": "audio", ".m4a": "audio", ".wav": "audio",
	".mp4": "video", ".3gp": "video", ".webm": "video",
	".pdf": "document", ".doc": "document", ".docx": "document",
	".xls": "document", ".xlsx": "document", ".csv": "document", ".txt": "document",
}

// msgMediaDir es la carpeta privada de una empresa.
func (s *Server) msgMediaDir(tenant string) string {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}
	return filepath.Join(s.config.DataPath(), "mensajeria", filepath.Base(tenant))
}

// msgMediaRuta resuelve un nombre guardado a una ruta dentro de la carpeta de la
// empresa. Devuelve error si el nombre intenta salir de ella.
func (s *Server) msgMediaRuta(tenant, nombre string) (string, error) {
	nombre = filepath.Base(strings.TrimSpace(nombre))
	if nombre == "" || nombre == "." || nombre == ".." {
		return "", fmt.Errorf("nombre de archivo inválido")
	}
	return filepath.Join(s.msgMediaDir(tenant), nombre), nil
}

// msgTipoDeMedia valida extensión y contenido. Devuelve el tipo y la extensión
// normalizada.
func msgTipoDeMedia(nombre string, primeros []byte) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(nombre))
	tipo, ok := msgExtensionesDeMedia[ext]
	if !ok {
		return "", "", fmt.Errorf("tipo de archivo no permitido: use imagen, audio, video o documento")
	}
	detectado := http.DetectContentType(primeros)
	switch tipo {
	case "image":
		if !strings.HasPrefix(detectado, "image/") {
			return "", "", fmt.Errorf("el archivo dice ser una imagen pero su contenido no lo es")
		}
	case "audio", "video":
		if !strings.HasPrefix(detectado, "audio/") && !strings.HasPrefix(detectado, "video/") &&
			!strings.HasPrefix(detectado, "application/ogg") {
			return "", "", fmt.Errorf("el archivo dice ser %s pero su contenido no lo es", tipo)
		}
	case "document":
		// Los documentos son un cajón: PDF, Office, texto. Se acepta cualquier
		// contenido que no sea ejecutable.
		if strings.Contains(detectado, "executable") || strings.Contains(detectado, "x-msdownload") {
			return "", "", fmt.Errorf("no se aceptan ejecutables")
		}
	}
	return tipo, ext, nil
}

// msgGuardarMedia escribe el archivo y devuelve el nombre con el que se guardó.
func (s *Server) msgGuardarMedia(tenant, nombre string, datos []byte) (string, error) {
	if len(datos) == 0 {
		return "", fmt.Errorf("el archivo está vacío")
	}
	if len(datos) > msgMediaMaximo {
		return "", fmt.Errorf("el archivo pasa de %d MB", msgMediaMaximo>>20)
	}
	ext := strings.ToLower(filepath.Ext(nombre))
	guardado := randomHex(16) + ext
	dir := s.msgMediaDir(tenant)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("no se pudo crear la carpeta de la empresa: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, guardado), datos, 0o600); err != nil {
		return "", fmt.Errorf("no se pudo guardar el archivo: %w", err)
	}
	return guardado, nil
}

func (s *Server) registerMensajeriaMediaRoutes() {
	s.mux.HandleFunc("POST /mensajeria/adjuntar", s.mensajeriaAdjuntar)
	s.mux.HandleFunc("GET /mensajeria/media/{id}", s.mensajeriaServirMedia)
}

// mensajeriaAdjuntar sube un archivo y lo encola para la conversación.
func (s *Server) mensajeriaAdjuntar(w http.ResponseWriter, r *http.Request) {
	if !s.mensajeriaActiva(r) {
		notFoundOrJSON(w, r, "mensajería no disponible")
		return
	}
	if !s.exigirPermiso(w, r, "create", "mensajeria_send") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, msgMediaMaximo+1<<20)
	if err := r.ParseMultipartForm(msgMediaMaximo); err != nil {
		jsonError(w, "No se pudo leer el archivo (máximo 16 MB).", http.StatusBadRequest)
		return
	}
	device := strings.TrimSpace(r.FormValue("device_id"))
	jid := strings.TrimSpace(r.FormValue("jid"))
	pie := strings.TrimSpace(r.FormValue("texto"))
	if !msgContiene(s.msgDispositivosVisibles(r), device) {
		jsonError(w, "No tiene acceso a ese dispositivo.", http.StatusForbidden)
		return
	}
	if strings.Contains(jid, "@g.us") {
		jsonError(w, "Los grupos no están disponibles en esta fase.", http.StatusBadRequest)
		return
	}
	archivo, cabecera, err := r.FormFile("archivo")
	if err != nil {
		jsonError(w, "Falta el archivo.", http.StatusBadRequest)
		return
	}
	defer archivo.Close()
	datos, err := io.ReadAll(io.LimitReader(archivo, msgMediaMaximo+1))
	if err != nil {
		jsonError(w, "No se pudo leer el archivo.", http.StatusBadRequest)
		return
	}
	if len(datos) > msgMediaMaximo {
		jsonError(w, "El archivo pasa de 16 MB.", http.StatusBadRequest)
		return
	}
	primeros := datos
	if len(primeros) > 512 {
		primeros = primeros[:512]
	}
	tipo, _, err := msgTipoDeMedia(cabecera.Filename, primeros)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	msisdn, err := s.normalizarTelefonoDeLaEmpresa(r, msisdnDeJID(jid))
	if err != nil {
		jsonError(w, "El teléfono de la conversación no se pudo normalizar: "+err.Error(), http.StatusBadRequest)
		return
	}
	tenant := s.tenantOf(r)
	guardado, err := s.msgGuardarMedia(tenant, cabecera.Filename, datos)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		jsonError(w, "No hay base de control disponible.", http.StatusServiceUnavailable)
		return
	}
	clave := "chat|" + tenant + "|" + jid + "|" + randomHex(8)
	id, _, err := s.msgEncolar(ctl, msgOutbox{
		Tenant: tenant, Provider: "whatsapp_gowa", DeviceID: device,
		Priority: "chat", Kind: "chat", ToMSISDN: msisdn, Body: pie,
		MediaPath: guardado, MediaType: tipo,
		RefType: "chat", RefID: jid, IdemKey: clave, CreatedBy: s.sessionUserID(r),
	})
	if err != nil {
		jsonError(w, "No se pudo encolar el archivo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = s.dbFor(r).Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		from_msisdn, to_msisdn, body, media_type, media_path, status, is_from_me, sent_at, received_at, raw_json, created_at)
		VALUES (?, 'whatsapp_gowa', ?, 'out', ?, '', ?, ?, ?, ?, 'en_cola', 1, ?, '', '', ?)`,
		device, jid, clave, msisdn, pie, tipo, guardado, now(), now())

	s.audit(r, "create", "mensajeria", "msg_outbox", fmt.Sprintf("%d", id), nil,
		map[string]interface{}{"device": device, "jid": jid, "tipo": tipo})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Archivo en cola.", "outbox_id": id})
}

// mensajeriaServirMedia entrega el archivo de un mensaje con permiso.
func (s *Server) mensajeriaServirMedia(w http.ResponseWriter, r *http.Request) {
	if !s.mensajeriaActiva(r) {
		notFoundOrJSON(w, r, "mensajería no disponible")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var device, nombre, tipo string
	if err := s.dbFor(r).QueryRow(`SELECT device_id, IFNULL(media_path,''), IFNULL(media_type,'')
		FROM msg_message WHERE id = ?`, id).Scan(&device, &nombre, &tipo); err != nil {
		http.NotFound(w, r)
		return
	}
	if !msgContiene(s.msgDispositivosVisibles(r), device) {
		http.Error(w, "sin acceso", http.StatusForbidden)
		return
	}
	ruta, err := s.msgMediaRuta(s.tenantOf(r), nombre)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(ruta)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if tipo == "document" {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(nombre)+"\"")
	}
	http.ServeContent(w, r, filepath.Base(nombre), info.ModTime(), f)
}

// msgMediaEntrante descarga y guarda el archivo de un mensaje recibido. Si falla,
// el mensaje se guarda igual con su tipo: perder el archivo es malo, perder el
// mensaje es peor.
func (s *Server) msgMediaEntrante(ctx context.Context, ctl *sql.DB, tenant, device, messageID, tipo string) (string, string) {
	cliente, err := s.msgClienteSinRequest(ctl)
	if err != nil {
		return "", ""
	}
	datos, _, err := cliente.DescargarMedia(ctx, device, messageID)
	if err != nil {
		return "", ""
	}
	ext := ".bin"
	switch tipo {
	case "image":
		ext = ".jpg"
	case "audio":
		ext = ".ogg"
	case "video":
		ext = ".mp4"
	case "document":
		ext = ".pdf"
	}
	guardado, err := s.msgGuardarMedia(tenant, "entrante"+ext, datos)
	if err != nil {
		return "", ""
	}
	return guardado, tipo
}

// msgTipoEntrante normaliza el tipo que manda el daemon.
func msgTipoEntrante(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "image", "imagen", "photo":
		return "image"
	case "audio", "ptt", "voice", "nota de voz":
		return "audio"
	case "video":
		return "video"
	case "document", "file", "documento":
		return "document"
	}
	return ""
}
