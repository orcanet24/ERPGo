package web

// Pruebas de media: subida, servido autenticado y descarga de un archivo
// entrante. Lo que se certifica es que un archivo no se sirve sin permiso y que
// no puede salir de la carpeta de su empresa.

import (
	"bytes"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"erpgo/internal/security"
)

var pngDePrueba = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRdatos-de-prueba")

func baseMedia(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := baseAreas(t)
	// El directorio de datos de la prueba, para no escribir en el repo.
	s.config.DBSQLitePath = filepath.Join(t.TempDir(), "empresa.sqlite3")
	return s, db
}

func formularioMultipart(t *testing.T, campos map[string]string, nombre string, datos []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range campos {
		_ = w.WriteField(k, v)
	}
	parte, err := w.CreateFormFile("archivo", nombre)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parte.Write(datos); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestAdjuntarImagenLaGuardaYLaEncola(t *testing.T) {
	s, db := baseMedia(t)
	cuerpo, tipo := formularioMultipart(t, map[string]string{
		"device_id": "default.cxc",
		"jid":       "584141234567@s.whatsapp.net",
		"texto":     "Mire la foto",
	}, "foto.png", pngDePrueba)

	req := httptest.NewRequest(http.MethodPost, "/mensajeria/adjuntar", cuerpo)
	req.Header.Set("Content-Type", tipo)
	req = withScope(req, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("master")})
	res := httptest.NewRecorder()
	s.mensajeriaAdjuntar(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("subida: %d %s", res.Code, res.Body.String())
	}
	var mediaTipo, mediaPath string
	if err := db.QueryRow(`SELECT media_type, media_path FROM msg_outbox ORDER BY id LIMIT 1`).
		Scan(&mediaTipo, &mediaPath); err != nil {
		t.Fatal(err)
	}
	if mediaTipo != "image" || mediaPath == "" {
		t.Fatalf("la cola no guardó el archivo: %q %q", mediaTipo, mediaPath)
	}
	ruta := filepath.Join(s.msgMediaDir("default"), mediaPath)
	if _, err := os.Stat(ruta); err != nil {
		t.Fatalf("el archivo no está en la carpeta de la empresa: %v", err)
	}

	var estado string
	if err := db.QueryRow(`SELECT status FROM msg_message WHERE direction='out'`).Scan(&estado); err != nil {
		t.Fatal(err)
	}
	if estado != "en_cola" {
		t.Fatalf("el mensaje debía quedar en cola: %q", estado)
	}
}

func TestElArchivoNoSeSirveSinPermisoNiSaleDeSuCarpeta(t *testing.T) {
	s, db := baseMedia(t)
	guardado, err := s.msgGuardarMedia("default", "foto.png", pngDePrueba)
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO msg_message (device_id, provider, chat_jid, direction, message_id,
		from_msisdn, to_msisdn, body, media_type, media_path, status, is_from_me, sent_at, received_at, raw_json, created_at)
		VALUES ('default.cxc','whatsapp_gowa','584141234567@s.whatsapp.net','in','M1','584141234567','',
		'','image',?,'recibido',0,?,'','',?)`, guardado, "2026-10-28 10:00:00", "2026-10-28 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	req := reqPrueba(s, sessPrueba("master"), "/mensajeria/media/1")
	req.SetPathValue("id", itoa(int(id)))
	out := httptest.NewRecorder()
	s.mensajeriaServirMedia(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("el master no pudo abrir el archivo: %d", out.Code)
	}
	if !bytes.Equal(out.Body.Bytes(), pngDePrueba) {
		t.Fatal("el archivo servido no es el guardado")
	}

	// Un empleado sin el dispositivo asignado no lo ve.
	sess := sessPrueba("employee")
	sess.UserID = "7"
	req = reqPrueba(s, sess, "/mensajeria/media/1")
	req.SetPathValue("id", itoa(int(id)))
	out = httptest.NewRecorder()
	s.mensajeriaServirMedia(out, req)
	if out.Code != http.StatusForbidden {
		t.Fatalf("un empleado sin dispositivo obtuvo %d; debía ser 403", out.Code)
	}

	// Un nombre con .. se reduce a su base: no puede salir de la carpeta.
	if _, err := db.Exec(`UPDATE msg_message SET media_path='../../secreto.png' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	req = reqPrueba(s, sessPrueba("master"), "/mensajeria/media/1")
	req.SetPathValue("id", itoa(int(id)))
	out = httptest.NewRecorder()
	s.mensajeriaServirMedia(out, req)
	if out.Code != http.StatusNotFound {
		t.Fatalf("un nombre con .. debía quedar dentro de la carpeta y dar 404; dio %d", out.Code)
	}
}

func TestElWebhookDescargaLaImagenEntrante(t *testing.T) {
	s, db := baseMedia(t)
	// El dispositivo ya existe en la base de áreas: se le pone el secreto en
	// vez de insertar otro con el mismo id.
	enc, err := security.Encrypt("secreto", security.DeriveKey(s.config.MasterKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE msg_device SET webhook_secret_enc=? WHERE device_id='default.cxc'`, enc); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/message/M1/download" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngDePrueba)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	configurarServicio(t, s, db, srv.URL)

	cuerpo := `{"event":"message","payload":{"id":"M1","from":"584141234567@s.whatsapp.net","body":"","media_type":"image"}}`
	res := enviarWebhook(s, "default.cxc", cuerpo, firmarWebhook(cuerpo, "secreto"))
	if res.Code != http.StatusOK {
		t.Fatalf("webhook: %d %s", res.Code, res.Body.String())
	}

	var mediaPath, mediaTipo string
	if err := db.QueryRow(`SELECT media_path, media_type FROM msg_message WHERE message_id='M1'`).
		Scan(&mediaPath, &mediaTipo); err != nil {
		t.Fatal(err)
	}
	if mediaPath == "" || mediaTipo != "image" {
		t.Fatalf("la imagen entrante no se guardó: %q %q", mediaPath, mediaTipo)
	}
	if _, err := os.Stat(filepath.Join(s.msgMediaDir("default"), mediaPath)); err != nil {
		t.Fatalf("el archivo entrante no está en disco: %v", err)
	}
}
