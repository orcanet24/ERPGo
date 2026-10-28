package web

// purchase_inbox.go — La bandeja de documentos de compra.
//
// QUÉ ES ESTO, EN UNA LÍNEA: el lugar donde aterriza un archivo que un
// proveedor envió —XML fiscal, PDF o una foto de la factura— antes de que
// alguien cargue la compra. Aquí se recibe, se guarda, se lee lo que se pueda
// leer y se propone en el formulario. Es la parte de recepción de la
// automatización de compras.
//
// LA LÍNEA DEL HUMO, QUE NO SE CRUZA NUNCA.
//
//   La extracción PROPOne; el formulario CONFIRMA; el candado DECIDE.
//
// Ningún archivo de esta bandeja crea una compra solo, ni escribe un renglón, ni
// mueve stock, ni toca cuentas por pagar. El XML se propone en
// `/purchase/add?inbox_id=N`, una persona revisa, guarda, y ahí —allí mismo—
// corre el candado anti-duplicado de purchase_hash contra la compra recién
// nacida. Si el documento ya estaba cargado, el candado responde con la compra
// existente, igual que si se hubiera cargado a mano. La IA entra por ese mismo
// camino: leer una factura no es registrarla.
//
// POR QUÉ UNA TABLA PROPIA Y NO COLUMNAS EN LA COMPRA.
//
// Un mismo documento puede recibir varios archivos (el PDF del proveedor, la
// foto de la nota, la corrección del número de control), esos archivos siguen
// existiendo aunque la compra se cancele, y hay archivos que todavía no se han
// cargado. El documento tiene su propio ciclo de vida: recibido → leído →
// cargado en compra / descartado. Guardarlo en la tabla de compras haría que
// «lo que llegó» y «lo que se registró» fueran lo mismo, que es exactamente lo
// que no son.
//
// EL OCR ES LA SIGUIENTE FASE, Y AQUÍ SE DEJA EL LUGAR.
//
// Un PDF o una imagen se guardan y se marcan `pendiente_ocr`: la bandeja los
// muestra igual que un XML, con la diferencia visible de que todavía no se les
// pudo leer. Cuando llegue el OCR (Fase D), lo único que cambia es
// `extraerDocumento`; la bandeja, el formulario y el candado no se enteran.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// inboxLimite es el peso máximo de un archivo recibido. Diez megabytes cubren
// con holgura un PDF con imágenes incrustadas o una foto de alta resolución, y
// al mismo tiempo impiden que un archivo disparado llene el disco.
const inboxLimite = 10 << 20

// inboxDir es la rama, dentro de la rama `documentos` de la empresa, donde
// viven los documentos recibidos. Rama propia: una factura de proveedor no se
// mezcla con el logo ni con las fotos de producto.
const inboxDir = "recibidos"

// inboxNombreColumnas es la lista de columnas que se leen de una fila, en el
// mismo orden para la lista y para el detalle: si un día se agrega una, se
// agrega en un solo lugar.
const inboxColumnas = `id, filename, orig_name, mime, bytes, formato, extraccion, doc_kind,
	rif, proveedor, numero, control, fecha, moneda, total, iva,
	lineas, leidos, notas, estado, IFNULL(linked_purchase_id,0), CAST(created_at AS CHAR)`

// inboxFila es una fila de la bandeja, ya leída.
type inboxFila struct {
	ID       int64
	Filename string // ruta relativa al directorio base de uploads
	OrigName string // cómo lo llamó quien lo subió
	MIME     string
	Bytes    int64
	Formato  string
	Extrac   string
	Kind     string
	RIF      string
	Prov     string
	Numero   string
	Control  string
	Fecha    string
	Moneda   string
	Total    float64
	IVA      float64
	Lineas   []extractoLinea
	Campos   string
	Notas    string
	Estado   string
	Linked   int64
	Creado   string
}

// escanearInbox lee una fila con el orden de inboxColumnas. Se pasa el Scan y no
// las filas para poder usar la misma lectura en QueryRow (detalle) y en Query
// (lista), que son dos tipos distintos y no comparten interfaz.
func escanearInbox(scan func(dest ...interface{}) error) (*inboxFila, error) {
	f := &inboxFila{}
	var lineas sql.NullString
	var total, iva float64
	if err := scan(&f.ID, &f.Filename, &f.OrigName, &f.MIME, &f.Bytes, &f.Formato, &f.Extrac, &f.Kind,
		&f.RIF, &f.Prov, &f.Numero, &f.Control, &f.Fecha, &f.Moneda, &total, &iva,
		&lineas, &f.Campos, &f.Notas, &f.Estado, &f.Linked, &f.Creado); err != nil {
		return nil, err
	}
	f.Total, f.IVA = total, iva
	// lineas es JSON escrito aquí mismo; si vino NULL o vacío, la fila no tiene
	// renglones propuestos y eso también es un dato válido (PDF pendiente).
	if lineas.Valid && strings.TrimSpace(lineas.String) != "" {
		_ = json.Unmarshal([]byte(lineas.String), &f.Lineas)
	}
	return f, nil
}

func (s *Server) registerPurchaseInboxRoutes() {
	s.mux.HandleFunc("GET /purchase/inbox", s.purchaseInboxLista)
	s.mux.HandleFunc("POST /purchase/inbox", s.purchaseInboxSubir)
	s.mux.HandleFunc("GET /purchase/inbox/file/{id}", s.purchaseInboxArchivo)
	s.mux.HandleFunc("POST /purchase/inbox/{id}/discard", s.purchaseInboxDescartar)
}

// ---------------------------------------------------------------------------

// purchaseInboxSubir recibe un archivo y deja su propuesta en la bandeja.
//
// Responde con lo que se pudo leer y con un `ir` al formulario ya preparado,
// para que el recorrido de quien recibe la factura sea: subir → revisar →
// guardar. Nada de eso salta el formulario ni el candado.
func (s *Server) purchaseInboxSubir(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "Sesión inválida", http.StatusUnauthorized)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, "No se pudo leer la carga: "+err.Error(), http.StatusBadRequest)
		return
	}
	archivo, header, err := r.FormFile("documento")
	if err != nil {
		jsonError(w, "Adjunte un archivo: XML, PDF o imagen de la factura.", http.StatusBadRequest)
		return
	}
	defer archivo.Close()

	formato := inboxFormato(header.Filename, header.Header.Get("Content-Type"))
	if formato == "" {
		jsonError(w, "Solo se reciben archivos XML, PDF o imágenes (JPG, PNG, WEBP).", http.StatusBadRequest)
		return
	}

	contenido, err := io.ReadAll(io.LimitReader(archivo, inboxLimite+1))
	if err != nil {
		jsonError(w, "No se pudo leer el archivo: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(contenido) == 0 {
		jsonError(w, "El archivo vino vacío.", http.StatusBadRequest)
		return
	}
	if len(contenido) > inboxLimite {
		jsonError(w, fmt.Sprintf("El archivo pesa más de %d MB.", inboxLimite>>20), http.StatusBadRequest)
		return
	}

	db := s.dbFor(r)
	tenant := diCompanyID(db)

	// EL MISMO ARCHIVO, DOS VECES. No es el candado de la compra (eso corre al
	// guardar, y sobre la compra); es el del archivo: si la persona arrastró la
	// misma factura dos veces —o dos personas la recibieron—, ya hay una fila.
	suma := sha256.Sum256(contenido)
	sha := hex.EncodeToString(suma[:])
	if ya, ok := inboxPorSHA(db, tenant, sha); ok {
		writeJSON(w, map[string]interface{}{
			"status": "duplicate", "inbox_id": ya,
			"msg": "Ese archivo ya se recibió: está en la bandeja.",
			"ir":  fmt.Sprintf("/purchase/add?inbox_id=%d", ya),
		})
		return
	}

	// LO QUE SE PUDO LEER. Un XML incompleto no se descarta: se lee hasta donde
	// dio y el fallo se anota en `notas`, que es lo que aparece en la pantalla.
	ex, lecturaErr := extraerDocumento(formato, contenido)
	notas := strings.Join(ex.Faltan, "; ")
	if lecturaErr != nil {
		notas = strings.TrimSpace("Archivo incompleto (" + lecturaErr.Error() + "). " + notas)
	}

	// GUARDAR EL ARCHIVO ANTES QUE LA FILA: si falla la fila, se borra el
	// archivo; si falla el archivo, no hay fila que quede apuntando a la nada.
	nombreOriginal := strings.TrimSpace(header.Filename)
	if nombreOriginal == "" {
		nombreOriginal = "documento"
	}
	relativa, absoluta, err := s.inboxGuardar(db, tenant, inboxExt(header.Filename), sha, contenido)
	if err != nil {
		jsonError(w, "No se pudo guardar el archivo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fila := &inboxFila{
		Filename: relativa, OrigName: nombreOriginal, MIME: header.Header.Get("Content-Type"),
		Bytes: int64(len(contenido)), Formato: formato, Extrac: ex.Extraccion, Kind: ex.Kind,
		RIF: ex.RIF, Prov: ex.Proveedor, Numero: ex.Numero, Control: ex.Control,
		Fecha: ex.Fecha, Moneda: ex.Moneda, Total: ex.Total, IVA: ex.IVA,
		Lineas: ex.Lineas, Campos: strings.Join(ex.Campos, ", "), Notas: notas,
		Estado: "leido",
	}
	id, err := inboxInsertar(db, tenant, sessUserID(sess), sha, fila)
	if err != nil {
		// Carrera: dos cargas del mismo archivo a la vez. El índice único
		// decidió; se responde con la fila que quedó y se borra el archivo
		// sobrante, que es idéntico al que ya está guardado.
		if inboxEsDuplicado(err) {
			_ = os.Remove(absoluta)
			if ya, ok := inboxPorSHA(db, tenant, sha); ok {
				writeJSON(w, map[string]interface{}{
					"status": "duplicate", "inbox_id": ya,
					"msg": "Ese archivo ya se recibió: está en la bandeja.",
					"ir":  fmt.Sprintf("/purchase/add?inbox_id=%d", ya),
				})
				return
			}
		}
		_ = os.Remove(absoluta)
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.appendAudit(r, sessUserID(sess), clientIP(r), "create", "Compras", "purchase_inbox",
		strconv.FormatInt(id, 10), nil, map[string]interface{}{
			"origen": nombreOriginal, "formato": formato, "extraccion": ex.Extraccion,
			"rif": ex.RIF, "numero": ex.Numero, "control": ex.Control, "tipo": ex.Kind,
		})

	writeJSON(w, map[string]interface{}{
		"status": "ok", "inbox_id": id,
		"ir":      fmt.Sprintf("/purchase/add?inbox_id=%d", id),
		"formato": formato, "extraccion": ex.Extraccion, "doc_kind": ex.Kind,
		"rif": ex.RIF, "proveedor": ex.Proveedor, "numero": ex.Numero, "control": ex.Control,
		"fecha": ex.Fecha, "total": ex.Total, "lineas": len(ex.Lineas),
		"campos": ex.Campos, "faltan": ex.Faltan, "notas": notas,
	})
}

// inboxInsertar escribe la fila propuesta.
//
// Está aparte del handler y no por estética: es la sentencia que NADIE había
// ejecutado antes de que existiera la prueba, y un desfase de argumentos ahí
// solo sale en producción, en el primer archivo que alguien reciba. Con la
// lista de columnas escrita a mano —nada de `INSERT INTO ... SELECT`— la misma
// sentencia vale en MySQL, MariaDB, PostgreSQL y SQLite.
func inboxInsertar(db *sql.DB, tenant, userID, sha string, f *inboxFila) (int64, error) {
	lineasJSON, err := json.Marshal(f.Lineas)
	if err != nil || len(f.Lineas) == 0 {
		lineasJSON = []byte("[]")
	}
	res, err := db.Exec(`INSERT INTO purchase_inbox
		(tenant_id, user_id, filename, orig_name, mime, bytes, formato, extraccion, doc_kind,
		 rif, proveedor, numero, control, fecha, moneda, total, iva, lineas, leidos, notas,
		 estado, sha256)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'leido', ?)`,
		tenant, userID, f.Filename, f.OrigName, f.MIME, f.Bytes, f.Formato, f.Extrac, f.Kind,
		f.RIF, f.Prov, f.Numero, f.Control, f.Fecha, f.Moneda, f.Total, f.IVA,
		string(lineasJSON), f.Campos, f.Notas, sha)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return id, err
}

// inboxGuarda escribe el archivo en la rama de la empresa.
//
// La ruta relativa que se devuelve se guarda en `filename` con el mismo criterio
// que la Fase B (fiscales/): contra la BASE de uploads, que es lo que espera
// GetFullPath al servirlo. El nombre del archivo sale del SHA del contenido, así
// que dos cargas del mismo documento apuntan al mismo sitio y nunca se pisan
// filas distintas: esas ya se rechazaron arriba.
func (s *Server) inboxGuardar(db *sql.DB, tenant, ext, sha string, contenido []byte) (string, string, error) {
	if s.uploadMgr == nil {
		return "", "", fmt.Errorf("no hay gestor de archivos configurado")
	}
	if ext == "" {
		ext = ".bin"
	}
	empresaDir, err := s.uploadMgr.EmpresaDir(tenant, db)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(empresaDir, "documentos", inboxDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", err
	}
	ruta := filepath.Join(dir, "doc-"+sha[:24]+ext)
	if err := os.WriteFile(ruta, contenido, 0o644); err != nil {
		return "", "", err
	}

	padre := strings.ReplaceAll(filepath.Dir(empresaDir), "\\", "/")
	plana := strings.ReplaceAll(ruta, "\\", "/")
	relativa := strings.TrimPrefix(plana, padre+"/")
	if relativa == plana {
		_ = os.Remove(ruta)
		return "", "", fmt.Errorf("la ruta del archivo no queda dentro de la base de uploads")
	}
	return relativa, ruta, nil
}

// inboxPorSHA busca un archivo ya recibido por esta empresa.
func inboxPorSHA(db *sql.DB, tenant, sha string) (int64, bool) {
	var id int64
	if err := db.QueryRow(`SELECT id FROM purchase_inbox WHERE tenant_id = ? AND sha256 = ?`,
		tenant, sha).Scan(&id); err != nil {
		return 0, false
	}
	return id, true
}

// inboxEsDuplicado reconoce el rechazo del índice único en ambos motores.
func inboxEsDuplicado(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "duplicate entry") || // MySQL / MariaDB
		strings.Contains(m, "unique constraint") || // PostgreSQL / SQLite
		strings.Contains(m, "uniqueness violated")
}

// ---------------------------------------------------------------------------

// purchaseInboxLista muestra la bandeja.
func (s *Server) purchaseInboxLista(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	tenant := diCompanyID(db)

	estado := strings.TrimSpace(r.URL.Query().Get("estado"))
	// Lo que se ve al entrar son los PENDIENTES: es la pregunta de quien acaba
	// de recibir facturas. Los demás estados están en las pestañas.
	if estado == "" {
		estado = "leido"
	}
	switch estado {
	case "leido", "cargado", "descartado":
	case "todos":
		estado = ""
	default:
		estado = "leido"
	}

	// Contadores de las pestañas: se cuentan todos los estados de una sola
	// pasada para que los números nunca se contradigan entre sí.
	contadores := map[string]int{"leido": 0, "cargado": 0, "descartado": 0, "todos": 0}
	if rows, err := db.Query(`SELECT IFNULL(estado,'leido'), COUNT(*) FROM purchase_inbox
		WHERE tenant_id = ? GROUP BY estado`, tenant); err == nil {
		for rows.Next() {
			var e string
			var n int
			if rows.Scan(&e, &n) == nil {
				contadores[e] = n
				contadores["todos"] += n
			}
		}
		rows.Close()
	}

	query := `SELECT ` + inboxColumnas + ` FROM purchase_inbox WHERE tenant_id = ?`
	args := []interface{}{tenant}
	if estado != "" {
		query += ` AND estado = ?`
		args = append(args, estado)
	}
	query += ` ORDER BY id DESC LIMIT 100`

	filas := []inboxFila{}
	if rows, err := db.Query(query, args...); err == nil {
		for rows.Next() {
			f, err := escanearInbox(rows.Scan)
			if err != nil {
				continue
			}
			// La fecha se muestra hasta el minuto: los segundos no ayudan a nadie
			// a distinguir dos documentos recibidos hoy.
			if len(f.Creado) > 16 {
				f.Creado = f.Creado[:16]
			}
			filas = append(filas, *f)
		}
		rows.Close()
	}

	s.render(w, r, "purchase_inbox.html", map[string]interface{}{
		"Title":      "Documentos recibidos",
		"Filas":      filas,
		"Estado":     estado,
		"Contadores": contadores,
	})
}

// purchaseInboxArchivo sirve el archivo con sesión y del directorio de ESTA
// empresa. La ruta sale de la base, nunca del pedido: no hay manera de pedir un
// archivo de otra empresa con esta URL.
func (s *Server) purchaseInboxArchivo(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	tenant := diCompanyID(db)
	f, err := s.inboxFilaPorID(db, tenant, r.PathValue("id"))
	if err != nil {
		http.Error(w, "Documento no encontrado", http.StatusNotFound)
		return
	}
	if s.uploadMgr == nil {
		http.Error(w, "No hay gestor de archivos configurado", http.StatusInternalServerError)
		return
	}
	if err := s.uploadMgr.ServeFile(w, r, f.Filename, tenant, db); err != nil {
		http.Error(w, "El archivo ya no está disponible", http.StatusNotFound)
	}
}

// Errores de los cambios de estado. Se devuelven como valores propios y no
// como texto pegado al error de base, para que el llamante pueda contestar con
// el número de la compra y no con «fila afectada: 0», que no le dice nada a
// nadie. Se comprueban con errors.Is.
var (
	errInboxNoExiste     = errors.New("ese documento no existe en esta empresa")
	errInboxYaCargado    = errors.New("ese documento ya está cargado en una compra")
	errInboxYaDescartado = errors.New("ese documento ya estaba descartado")
)

// inboxMarcarCargado pasa el documento a «cargado» con su número de compra.
//
// El movimiento es de UN SOLO SALTO: solo sale de 'leido'. Así, un id que ya se
// vinculó no se re-vincula (la fila no cambia), uno de otra empresa no existe y
// uno descartado no se reescribe en silencio. Devuelve si la fila cambió.
func inboxMarcarCargado(db *sql.DB, tenant string, inboxID, purchaseID int64) (bool, error) {
	if inboxID <= 0 || purchaseID <= 0 {
		return false, nil
	}
	res, err := db.Exec(`UPDATE purchase_inbox
		SET estado = 'cargado', linked_purchase_id = ?, linked_at = ?
		WHERE id = ? AND tenant_id = ? AND estado = 'leido'`,
		purchaseID, time.Now().Format("2006-01-02 15:04:05"), inboxID, tenant)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// inboxMarcarDescartado pasa el documento a «descartado».
//
// NO SE BORRA: la traza de «llegó esto y se decidió no usarlo» es tan parte del
// orden del negocio como la de lo que sí se usó. Solo se puede descartar lo que
// todavía no se cargó, y lo que ya está vinculado a una compra se niega con su
// número, para que nadie se pregunte dónde fue a parar.
func inboxMarcarDescartado(db *sql.DB, tenant, id string) error {
	res, err := db.Exec(`UPDATE purchase_inbox SET estado = 'descartado'
		WHERE id = ? AND tenant_id = ? AND estado = 'leido'`, id, tenant)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	// No se movió: hay que decir POR QUÉ, con el estado real de la fila.
	var estado string
	var linked int64
	qerr := db.QueryRow(`SELECT IFNULL(estado,''), IFNULL(linked_purchase_id,0) FROM purchase_inbox
		WHERE id = ? AND tenant_id = ?`, id, tenant).Scan(&estado, &linked)
	if qerr != nil {
		return errInboxNoExiste
	}
	if linked > 0 {
		return errInboxYaCargado
	}
	return errInboxYaDescartado
}

// purchaseInboxDescartar marca un documento como descartado.
func (s *Server) purchaseInboxDescartar(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	db := s.dbFor(r)
	tenant := diCompanyID(db)
	id := strings.TrimSpace(r.PathValue("id"))

	if err := inboxMarcarDescartado(db, tenant, id); err != nil {
		switch {
		case errors.Is(err, errInboxNoExiste):
			jsonError(w, "Ese documento no existe en esta empresa.", http.StatusNotFound)
		case errors.Is(err, errInboxYaCargado):
			var linked int64
			_ = db.QueryRow(`SELECT IFNULL(linked_purchase_id,0) FROM purchase_inbox
				WHERE id = ? AND tenant_id = ?`, id, tenant).Scan(&linked)
			jsonError(w, fmt.Sprintf("Ya está cargado en la compra #%d: no se puede descartar.", linked), http.StatusConflict)
		case errors.Is(err, errInboxYaDescartado):
			jsonError(w, "Ese documento ya estaba descartado.", http.StatusConflict)
		default:
			jsonError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	s.appendAudit(r, sessUserID(sess), clientIP(r), "discard", "Compras", "purchase_inbox",
		id, nil, map[string]interface{}{"estado": "descartado"})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Documento descartado"})
}

// ---------------------------------------------------------------------------

// inboxFilaPorID lee una fila de ESTA empresa. El tenant va siempre en el WHERE
// con el id: es la misma costumbre que la subida de imágenes, donde cada
// empresa ve lo suyo y nada más.
func (s *Server) inboxFilaPorID(db *sql.DB, tenant, id string) (*inboxFila, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("id inválido")
	}
	return escanearInbox(func(dest ...interface{}) error {
		return db.QueryRow(`SELECT `+inboxColumnas+` FROM purchase_inbox
			WHERE id = ? AND tenant_id = ?`, n, tenant).Scan(dest...)
	})
}

// inboxVincular amarra la compra nacida con el documento que la propuso.
//
// Se llama desde purchaseSave DESPUÉS del commit: si falla, la compra sigue
// siendo válida —lo que se pierde es la puesta enlazada, no el movimiento—, así
// que se registra y se sigue. El WHERE exige `estado = 'leido'`: un id que ya
// se vinculó no se re-vincula, y uno de otra empresa simplemente no existe.
func (s *Server) inboxVincular(r *http.Request, inboxID, purchaseID int64) {
	db := s.dbFor(r)
	tenant := diCompanyID(db)
	cambiada, err := inboxMarcarCargado(db, tenant, inboxID, purchaseID)
	if err != nil || !cambiada {
		return
	}
	sess := s.sessionFromRequest(r)
	s.appendAudit(r, sessUserID(sess), clientIP(r), "link", "Compras", "purchase_inbox",
		strconv.FormatInt(inboxID, 10), map[string]interface{}{"estado": "leido"},
		map[string]interface{}{"estado": "cargado", "purchase_id": purchaseID})
}

// inboxParaFormulario carga la propuesta del documento en la página de compra.
//
// Solo se PROPONE: lo que sigue siendo obligatorio (número de factura con IVA,
// proveedor existente, producto identificado, candado anti-duplicado) lo sigue
// poniendo y exigiendo el guardado. Si el documento no existe o ya se descartó,
// el formulario se abre vacío y sin ruido: una pista mala no debe frenar a
// nadie que venga a cargar una compra a mano.
func (s *Server) inboxParaFormulario(r *http.Request, data map[string]interface{}) {
	q := strings.TrimSpace(r.URL.Query().Get("inbox_id"))
	if q == "" {
		return
	}
	db := s.dbFor(r)
	tenant := diCompanyID(db)
	f, err := s.inboxFilaPorID(db, tenant, q)
	if err != nil || f.Estado == "descartado" {
		return
	}

	data["InboxID"] = f.ID
	data["InboxArchivo"] = f.OrigName
	data["InboxLineas"] = f.Lineas
	data["InboxCampos"] = f.Campos
	data["InboxFaltan"] = f.Notas
	data["InboxExtraccion"] = f.Extrac
	if f.Kind != "" {
		data["DocumentKind"] = f.Kind
	}
	if f.Numero != "" {
		data["Chalan"] = f.Numero
	}
	if f.Control != "" {
		data["ControlNo"] = f.Control
	}
	if f.Fecha != "" {
		data["PDate"] = f.Fecha
	}

	// ¿El proveedor del RIF ya está dado de alta? Si sí, se elige; si no, se
	// avisa con su número para que se cree con ese mismo RIF. Adivinar otro
	// proveedor «parecido» sería pegarle la factura al equivocado.
	if f.RIF != "" {
		norm := NormalizeRIF(f.RIF)
		var sid int64
		var sname string
		qerr := db.QueryRow(`SELECT supplier_id, supplier_name FROM supplier_information
			WHERE rif = ? OR rif = ? ORDER BY supplier_id LIMIT 1`, norm, f.RIF).
			Scan(&sid, &sname)
		if qerr == nil {
			data["Supplier"] = sid
			data["InboxProveedor"] = sname
		} else {
			data["InboxProveedorNuevo"] = f.RIF
		}
	}
}
