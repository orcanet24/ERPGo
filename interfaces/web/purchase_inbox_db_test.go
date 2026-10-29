package web

// Pruebas de la bandeja de documentos con base de datos.
//
// La extracción (qué se lee de un XML) es código puro y se prueba en
// purchase_inbox_extract_test.go. Aquí está lo que solo se puede comprobar con
// una base real: que el candado del ARCHIVO impide el segundo ingreso, que cada
// empresa ve lo suyo, y que el documento solo cambia de estado una vez en cada
// salto. Una prueba de funciones puras no distingue un candado puesto de un
// candado que nunca se cerró.
//
// El motor es SQLite en memoria (el driver ya está en go.mod), con el mismo
// esquema que declara migrations/schema_z_compras_inbox.sql, y la forma de
// INSERT y de DETECCIÓN de clave duplicada funciona en los dos motores.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	"erpgo/internal/upload"
)

func nuevaBaseInbox(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	// Una sola conexión: con varias, cada una tendría su propia base :memory: y
	// la prueba pasaría o fallaría según el azar del pool.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	ddl := `CREATE TABLE purchase_inbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL,
		user_id TEXT NOT NULL DEFAULT '',
		filename TEXT NOT NULL,
		orig_name TEXT NOT NULL,
		mime TEXT NOT NULL DEFAULT '',
		bytes INTEGER NOT NULL DEFAULT 0,
		formato TEXT NOT NULL DEFAULT '',
		extraccion TEXT NOT NULL DEFAULT 'pendiente_ocr',
		doc_kind TEXT NOT NULL DEFAULT 'factura',
		rif TEXT NOT NULL DEFAULT '',
		proveedor TEXT NOT NULL DEFAULT '',
		numero TEXT NOT NULL DEFAULT '',
		control TEXT NOT NULL DEFAULT '',
		fecha TEXT NOT NULL DEFAULT '',
		moneda TEXT NOT NULL DEFAULT '',
		total REAL NOT NULL DEFAULT 0,
		iva REAL NOT NULL DEFAULT 0,
		lineas TEXT,
		leidos TEXT NOT NULL DEFAULT '',
		notas TEXT NOT NULL DEFAULT '',
		estado TEXT NOT NULL DEFAULT 'leido',
		linked_purchase_id INTEGER,
		linked_at TEXT,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		sha256 TEXT NOT NULL,
		UNIQUE (tenant_id, sha256)
	)`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("no se pudo crear la tabla: %v", err)
	}
	// El directorio de la empresa se resuelve contra company_information, que es
	// como lo hace en producción: sin esta tabla la subida no tendría dónde
	// escribir el archivo y el error saldría como «no se pudo guardar».
	if _, err := db.Exec(`CREATE TABLE company_information (
		company_id TEXT NOT NULL PRIMARY KEY,
		upload_path TEXT NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 1)`); err != nil {
		t.Fatalf("no se pudo crear company_information: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO company_information (company_id, upload_path) VALUES ('test','')`); err != nil {
		t.Fatalf("no se pudo sembrar la empresa: %v", err)
	}
	return db
}

// recibir inserta una fila como lo hace purchaseInboxSubir.
func recibir(t *testing.T, db *sql.DB, tenant, sha string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO purchase_inbox
		(tenant_id, user_id, filename, orig_name, mime, bytes, formato, extraccion, doc_kind,
		 rif, proveedor, numero, control, fecha, moneda, total, iva, lineas, leidos, notas,
		 estado, sha256)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'leido', ?)`,
		tenant, "1", "EMP/documentos/recibidos/doc-"+sha[:8]+".xml", "factura.xml",
		"application/xml", 1024, "xml", "xml", "factura",
		"J-30123456-7", "DISTRIBUIDORA EL PUENTE, C.A.", "00-0000451", "00-0000451-1",
		"2026-10-01", "VES", 4593.6, 633.6,
		`[{"descripcion":"ARROZ GANDUL 1KG","cantidad":12,"precio":250,"total":3000}]`,
		"RIF, número de factura, total", "", sha)
	if err != nil {
		t.Fatalf("la primera carga tenía que entrar: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestElMismoArchivoNoSeRecibeDosVeces es el candado del archivo: el mismo PDF
// que llega por correo y otra vez por WhatsApp es UN documento, y la bandeja no
// debe llenar de filas repetidas la misma factura.
func TestElMismoArchivoNoSeRecibeDosVeces(t *testing.T) {
	db := nuevaBaseInbox(t)
	sha := strings.Repeat("ab", 32)

	primero := recibir(t, db, "EMP1", sha)
	if ya, ok := inboxPorSHA(db, "EMP1", sha); !ok || ya != primero {
		t.Fatalf("el archivo recién recibido no se encuentra por su SHA")
	}

	_, err := db.Exec(`INSERT INTO purchase_inbox
		(tenant_id, filename, orig_name, formato, extraccion, doc_kind, sha256)
		VALUES ('EMP1','otro/archivo.xml','factura.xml','xml','xml','factura',?)`, sha)
	if err == nil {
		t.Fatal("el índice único debió rechazar la segunda copia")
	}
	if !inboxEsDuplicado(err) {
		t.Errorf("el rechazo no se reconoció como duplicado: %v", err)
	}
}

// TestUnArchivoDeOtraEmpresaNoEsDuplicado: dos empresas distintas pueden recibir
// el mismo archivo (la misma imprenta manda el modelo a varios clientes). El
// candado es POR EMPRESA, igual que todo lo demás aquí.
func TestUnArchivoDeOtraEmpresaNoEsDuplicado(t *testing.T) {
	db := nuevaBaseInbox(t)
	sha := strings.Repeat("cd", 32)

	recibir(t, db, "EMP1", sha)
	if _, err := db.Exec(`INSERT INTO purchase_inbox
		(tenant_id, filename, orig_name, formato, extraccion, doc_kind, sha256)
		VALUES ('EMP2','otro/archivo.xml','factura.xml','xml','xml','factura',?)`, sha); err != nil {
		t.Errorf("otra empresa debía poder recibir el mismo archivo: %v", err)
	}
}

// TestSeLeeUnaFilaCompleta comprueba que lo que se guarda se vuelve a leer tal
// cual, incluidos los renglones propuestos: es lo que el formulario va a
// repartir en las filas de la compra.
func TestSeLeeUnaFilaCompleta(t *testing.T) {
	db := nuevaBaseInbox(t)
	id := recibir(t, db, "EMP1", strings.Repeat("ef", 32))

	f, err := (&Server{}).inboxFilaPorID(db, "EMP1", "1")
	if err != nil || f == nil {
		t.Fatalf("la fila no se pudo leer: %v", err)
	}
	if f.ID != id {
		t.Errorf("id: quería %d, salió %d", id, f.ID)
	}
	if f.RIF != "J-30123456-7" || f.Numero != "00-0000451" {
		t.Errorf("los campos propuestos salieron rotos: rif=%q numero=%q", f.RIF, f.Numero)
	}
	if f.Total != 4593.6 || f.IVA != 633.6 {
		t.Errorf("totales: %.2f / %.2f", f.Total, f.IVA)
	}
	if f.Estado != "leido" {
		t.Errorf("estado inicial: %q", f.Estado)
	}
	if len(f.Lineas) != 1 {
		t.Fatalf("los renglones propuestos no se leyeron: %+v", f.Lineas)
	}
	if f.Lineas[0].Descripcion != "ARROZ GANDUL 1KG" || f.Lineas[0].Cantidad != 12 || f.Lineas[0].Precio != 250 {
		t.Errorf("renglón: %+v", f.Lineas[0])
	}
}

// TestSoloSeVeLoDeLaEmpresa es la aislación: el id en la URL no alcanza, el
// tenant va siempre en el WHERE. Un id ajeno no existe.
func TestSoloSeVeLoDeLaEmpresa(t *testing.T) {
	db := nuevaBaseInbox(t)
	recibir(t, db, "EMP1", strings.Repeat("11", 32))

	if _, err := (&Server{}).inboxFilaPorID(db, "EMP2", "1"); err == nil {
		t.Error("una empresa leyó un documento de otra")
	}
	if _, err := (&Server{}).inboxFilaPorID(db, "EMP1", "999"); err == nil {
		t.Error("un id inexistente se leyó como si existiera")
	}
	if _, err := (&Server{}).inboxFilaPorID(db, "EMP1", "abc"); err == nil {
		t.Error("un id no numérico se aceptó")
	}
}

// TestLasRutasDeCompraYBandejaConviven comprueba algo que NINGÚN otro test
// cubre y que tumbaría el ERP al arrancar: ServeMux de Go 1.22 paniquea cuando
// dos patrones se solapan sin que uno sea más específico que el otro. Los de la
// bandeja tienen esa forma peligrosa —`/inbox/file/{id}` y
// `/inbox/{id}/discard` se cruzan en `/inbox/file/discard`— y solo conviven
// porque los cuatro llevan método. Si algún día se le quita el método a uno,
// aquí se entera en lugar de en el arranque de producción.
func TestLasRutasDeCompraYBandejaConviven(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("los patrones de ruta de compras no se pueden registrar: %v", e)
		}
	}()
	mux := http.NewServeMux()
	noop := func(http.ResponseWriter, *http.Request) {}
	// Las del módulo de compras, tal como las registra registerPurchaseRoutes.
	for _, p := range []string{
		"GET /purchase/list", "/purchase/list/data", "GET /purchase/add",
		"GET /purchase/edit/{id}", "POST /purchase/save", "POST /purchase/dup-check",
		"POST /purchase/delete/{id}", "GET /purchase/details/{id}",
	} {
		mux.HandleFunc(p, noop)
	}
	// Las de la bandeja, con método en las cuatro.
	for _, p := range []string{
		"GET /purchase/inbox", "POST /purchase/inbox",
		"GET /purchase/inbox/file/{id}", "POST /purchase/inbox/{id}/discard",
	} {
		mux.HandleFunc(p, noop)
	}
}

// TestElInsertRealDeLaBandejaEjecuta prueba la sentencia TAL CUAL la escribe el
// handler. Es la comprobación más prosaica y la más valiosa: un `?` de más o
// una columna descolocada no rompe la compilación, rompe la primera recepción
// de un archivo en producción.
func TestElInsertRealDeLaBandejaEjecuta(t *testing.T) {
	db := nuevaBaseInbox(t)
	f := &inboxFila{
		Filename: "EMP1/documentos/recibidos/doc-abc.xml", OrigName: "factura del mes.xml",
		MIME: "application/xml", Bytes: 2048, Formato: "xml", Extrac: "xml", Kind: CompraFactura,
		RIF: "J-30123456-7", Prov: "DISTRIBUIDORA EL PUENTE, C.A.",
		Numero: "00-0000451", Control: "00-0000451-1", Fecha: "2026-10-01", Moneda: "VES",
		Total: 4593.6, IVA: 633.6,
		Lineas: []extractoLinea{{Descripcion: "ARROZ GANDUL 1KG", Cantidad: 12, Precio: 250, Total: 3000}},
		Campos: "RIF, número de factura, total", Notas: "", Estado: "leido",
	}
	sha := strings.Repeat("99", 32)

	id, err := inboxInsertar(db, "EMP1", "42", sha, f)
	if err != nil {
		t.Fatalf("el INSERT del handler no se pudo ejecutar: %v", err)
	}
	if id <= 0 {
		t.Fatalf("no devolvió el id de la fila: %d", id)
	}

	leida, err := (&Server{}).inboxFilaPorID(db, "EMP1", "1")
	if err != nil {
		t.Fatalf("la fila escrita por el handler no se puede leer: %v", err)
	}
	if leida.MIME != "application/xml" || leida.Bytes != 2048 {
		t.Errorf("los datos del archivo no volvieron igual: %q / %d", leida.MIME, leida.Bytes)
	}
	if leida.Estado != "leido" {
		t.Errorf("el estado inicial tiene que ser leido, salió %q", leida.Estado)
	}
	if len(leida.Lineas) != 1 || leida.Lineas[0].Total != 3000 {
		t.Errorf("los renglones propuestos no sobrevivieron el viaje: %+v", leida.Lineas)
	}

	// Y el candado del archivo sigue cerrado con esta sentencia.
	if _, err := inboxInsertar(db, "EMP1", "42", sha, f); err == nil {
		t.Error("el mismo archivo volvió a entrar con la sentencia real")
	} else if !inboxEsDuplicado(err) {
		t.Errorf("el rechazo no se reconoció como duplicado: %v", err)
	}
}

// TestLaBandejaRecibeUnXmlYNoLoDuplica es el recorrido entero del POST
// /purchase/inbox: multipart de verdad, formato rechazado en la puerta,
// propuesta devuelta con su `ir`, archivo escrito en el directorio de la
// empresa y la segunda copia del MISMO contenido contestando «duplicate» con la
// fila que ya estaba. Es la parte que ningún test de funciones puras cubre: el
// contrato JSON que después consume el JavaScript de la pantalla.
func TestLaBandejaRecibeUnXmlYNoLoDuplica(t *testing.T) {
	db := nuevaBaseInbox(t)
	base := t.TempDir()
	s := &Server{db: db, sessions: NewSessionStore(), uploadMgr: upload.NewManager(base)}
	sess := &SessionData{UserID: "7", Username: "admin", UserLevel: "admin", TenantCode: "test"}

	subir := func(nombre string, contenido []byte) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		wri := multipart.NewWriter(&buf)
		fw, err := wri.CreateFormFile("documento", nombre)
		if err != nil {
			t.Fatalf("no se pudo armar el multipart: %v", err)
		}
		if _, err := fw.Write(contenido); err != nil {
			t.Fatalf("no se pudo escribir el archivo en el multipart: %v", err)
		}
		_ = wri.Close()

		r := withScope(httptest.NewRequest(http.MethodPost, "/purchase/inbox", &buf),
			&scope{tenant: "test", db: db, settings: &AppSettings{}, sess: sess})
		r.Header.Set("Content-Type", wri.FormDataContentType())
		w := httptest.NewRecorder()
		s.purchaseInboxSubir(w, r)
		return w
	}

	var respuesta struct {
		Status  string   `json:"status"`
		InboxID int64    `json:"inbox_id"`
		IR      string   `json:"ir"`
		Campos  []string `json:"campos"`
		Faltan  []string `json:"faltan"`
		Lineas  int      `json:"lineas"`
	}

	w := subir("factura octubre.xml", []byte(facturitaVenezolana))
	if w.Code != http.StatusOK {
		t.Fatalf("una factura válida no debe rechazarse: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &respuesta); err != nil {
		t.Fatalf("la respuesta no es JSON: %v\n%s", err, w.Body.String())
	}
	if respuesta.Status != "ok" || respuesta.InboxID <= 0 {
		t.Fatalf("respuesta inesperada: %s", w.Body.String())
	}
	esperado := fmt.Sprintf("/purchase/add?inbox_id=%d", respuesta.InboxID)
	if respuesta.IR != esperado {
		t.Errorf("el enlace de la propuesta es %q, se esperaba %q", respuesta.IR, esperado)
	}
	if respuesta.Lineas != 2 {
		t.Errorf("la factura trae 2 renglones y la respuesta dice %d", respuesta.Lineas)
	}
	if len(respuesta.Faltan) != 0 {
		t.Errorf("una factura completa no debe reportar faltantes: %v", respuesta.Faltan)
	}

	// El archivo quedó en el directorio de la empresa, con la ruta que después
	// se sirve desde /purchase/inbox/file/{id}.
	var ruta string
	if err := db.QueryRow(`SELECT filename FROM purchase_inbox WHERE id = ?`, respuesta.InboxID).Scan(&ruta); err != nil {
		t.Fatalf("la fila no se guardó: %v", err)
	}
	if _, err := os.Stat(base + "/" + ruta); err != nil {
		t.Errorf("el archivo no está donde apunta la fila (%s): %v", ruta, err)
	}

	// El mismo contenido, otro nombre: ya está recibido.
	segunda := subir("factura otra vez.xml", []byte(facturitaVenezolana))
	var dup struct {
		Status  string `json:"status"`
		InboxID int64  `json:"inbox_id"`
	}
	if err := json.Unmarshal(segunda.Body.Bytes(), &dup); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if dup.Status != "duplicate" {
		t.Errorf("la segunda copia debía contestar duplicate, contestó %q", dup.Status)
	}
	if dup.InboxID != respuesta.InboxID {
		t.Errorf("la segunda copia apuntó a otra fila: %d en vez de %d", dup.InboxID, respuesta.InboxID)
	}

	// Lo que no es documento ni se guarda ni se procesa: se rechaza en la puerta.
	nulo := subir("resumen.txt", []byte("esto no es una factura"))
	if nulo.Code != http.StatusBadRequest {
		t.Errorf("un .txt se debía rechazar con 400, llegó %d", nulo.Code)
	}
	if !strings.Contains(nulo.Body.String(), "Solo se reciben") {
		t.Errorf("el rechazo no explica qué se acepta: %s", nulo.Body.String())
	}
}

// TestElDocumentoSoloSeCargaUnaVez es el salto de estado: de pendiente a
// cargado, una sola vez, y solo el que le pertenece puede moverlo.
func TestElDocumentoSoloSeCargaUnaVez(t *testing.T) {
	db := nuevaBaseInbox(t)
	id := recibir(t, db, "EMP1", strings.Repeat("22", 32))

	cambiada, err := inboxMarcarCargado(db, "EMP1", id, 77)
	if err != nil || !cambiada {
		t.Fatalf("la primera vinculación debía funcionar: cambió=%v err=%v", cambiada, err)
	}
	if cambiada, _ := inboxMarcarCargado(db, "EMP1", id, 78); cambiada {
		t.Error("un documento cargado se re-vinculó a otra compra")
	}
	if cambiada, _ := inboxMarcarCargado(db, "EMP2", id, 79); cambiada {
		t.Error("otra empresa pudo vincular el documento")
	}
	if cambiada, _ := inboxMarcarCargado(db, "EMP1", 0, 77); cambiada {
		t.Error("una compra 0 no debe vincular nada")
	}

	f, _ := (&Server{}).inboxFilaPorID(db, "EMP1", "1")
	if f.Estado != "cargado" || f.Linked != 77 {
		t.Errorf("estado final: %q, compra %d", f.Estado, f.Linked)
	}
}

// TestDescartarSoloLoQueNoSeCargó: el descarte no borra y no toca lo que ya
// está en una compra, y el error dice el motivo concreto —no «0 filas»— para
// que la pantalla pueda responder con el número de la compra.
func TestDescartarSoloLoQueNoSeCargó(t *testing.T) {
	db := nuevaBaseInbox(t)
	recibir(t, db, "EMP1", strings.Repeat("33", 32))

	if err := inboxMarcarDescartado(db, "EMP2", "1"); !errors.Is(err, errInboxNoExiste) {
		t.Errorf("otra empresa debía recibir errInboxNoExiste, salió %v", err)
	}
	if err := inboxMarcarDescartado(db, "EMP1", "999"); !errors.Is(err, errInboxNoExiste) {
		t.Errorf("id inexistente debía recibir errInboxNoExiste, salió %v", err)
	}
	if err := inboxMarcarDescartado(db, "EMP1", "1"); err != nil {
		t.Fatalf("un documento pendiente se debía descartar: %v", err)
	}
	if err := inboxMarcarDescartado(db, "EMP1", "1"); !errors.Is(err, errInboxYaDescartado) {
		t.Errorf("segundo descarte debía recibir errInboxYaDescartado, salió %v", err)
	}

	// Y el que ya está cargado no se descarta: su traza es la de la compra.
	id2 := recibir(t, db, "EMP1", strings.Repeat("44", 32))
	if _, err := inboxMarcarCargado(db, "EMP1", id2, 88); err != nil {
		t.Fatalf("no se pudo cargar el segundo documento: %v", err)
	}
	if err := inboxMarcarDescartado(db, "EMP1", "2"); !errors.Is(err, errInboxYaCargado) {
		t.Errorf("un documento cargado debía recibir errInboxYaCargado, salió %v", err)
	}

	f, _ := (&Server{}).inboxFilaPorID(db, "EMP1", "1")
	if f.Estado != "descartado" {
		t.Errorf("el primer documento debió quedar descartado, quedó %q", f.Estado)
	}
}

