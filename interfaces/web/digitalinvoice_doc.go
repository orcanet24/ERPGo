package web

// digitalinvoice_doc.go — El documento fiscal que devuelve la imprenta digital:
// dónde vive, cómo se muestra en facturación y en cuentas por cobrar, y cómo
// llega al cliente.
//
// POR QUÉ EXISTE ESTE ARCHIVO.
//
// digital_invoice_log guarda pdf_url y xml_url desde que el proveedor contesta,
// y hasta ahora nadie los leía: el PDF del cliente existía en la base y no había
// ni un link ni un botón que lo mostrara. El circuito quedaba abierto en el
// último tramo —la imprenta emitía y el sistema no entregaba—.
//
// TRES REGLAS QUE ESTE ARCHIVO CUMPLE Y NO NUEVA:
//
//   - El destinatario sale de la FICHA del cliente, nunca de la pantalla. Es la
//     misma regla de outbound.go: si el formulario eligiera destino, un prompt
//     inyectado usaría el ERP como mensajero.
//   - Un envío sale UNA vez. La entrega reutiliza finance_message con su clave
//     de idempotencia y su enfriamiento: dos pasadas del runner cruzadas, un
//     reinicio o un reintento no convierten el correo en spam.
//   - Los enlaces del proveedor se validan antes de pintarlos. Es un dato
//     externo que viaja al navegador; un `javascript:` disfrazado de PDF sería
//     código ejecutándose en la sesión de quien abra la factura.
//
// Lo que NO hace: emitir. La emisión sigue siendo del runner
// (digitalinvoice_processor.go); aquí solo se lee y se entrega lo emitido.

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// diDocFila es lo que la imprenta digital sabe de UNA factura.
type diDocFila struct {
	InvoiceNo    int64  // correlativo de la factura (invoice.invoice)
	Estado       string // pending | sent | failed | "" (no encolado)
	StrongID     string
	NroControl   int64  // número de control entero (imprentas que lo dan así)
	ControlTexto string // número de control completo, con su serie ("00-00000056")
	PDF          string
	XML          string
	PDFPath      string // copia propia en el directorio de la empresa (vacía si no hay)
	Error        string
}

// diLoteDocs es el tamaño del lote de la consulta IN. La lista de facturas trae
// hasta 1000 filas y la cartera hasta 500: en lotes de 200 la consulta no pasa
// de un puñado de marcadores y no se come el límite de parámetros de un motor
// u otro.
const diLoteDocs = 200

// diURLSegura devuelve la URL si es un enlace navegable y seguro, y "" si no.
//
// Solo http/https, y sin comillas, ángulos ni espacios: esa misma cadena se
// pinta dentro de un atributo href construido a mano en el navegador, y una
// comilla suelta en una URL ajena cerraría el atributo y añadiría el resto.
// Mejor perder el enlace que servir un documento «del proveedor» que en realidad
// ejecuta algo.
func diURLSegura(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if strings.ContainsAny(u, "\"'<> \\") {
		return ""
	}
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return ""
	}
	if p.Scheme != "http" && p.Scheme != "https" {
		return ""
	}
	return u
}

// diDocsDeFacturas devuelve el documento digital de varias facturas, agrupado
// por correlativo.
//
// Vive en la base de CONTROL, no en la de la empresa: ahí está
// digital_invoice_log, y ahí la encuentra la caja de cualquier empresa. Por eso
// recibe las dos bases como argumentos —quien llama ya sabe de qué empresa es—
// y por eso es una función suelta y no un método: la prueba la llama con dos
// bases en memoria y no hace falta un servidor entero.
//
// Una base de control que no está disponible no inventa: devuelve el mapa
// vacío, la pantalla pinta el resto del documento y el enlace no sale. Un
// enlace fingido es peor que un enlace ausente.
func diDocsDeFacturas(ctl *sql.DB, tenant string, numeros []int64) map[int64]diDocFila {
	salida := map[int64]diDocFila{}
	if ctl == nil || strings.TrimSpace(tenant) == "" || len(numeros) == 0 {
		return salida
	}

	// Se piden solo los que faltan: una lista con 200 facturas no vuelve a
	// preguntar por las 3 que ya trae el lote anterior.
	pedidos := map[int64]bool{}
	lote := make([]int64, 0, diLoteDocs)
	for _, n := range numeros {
		if n <= 0 || pedidos[n] {
			continue
		}
		pedidos[n] = true
		lote = append(lote, n)
	}

	// Las columnas de la copia propia y del control textual solo se piden si
	// existen en ESTA base: una base anterior a esas migraciones no puede ver
	// cómo se cae la consulta entera por una columna que aún no tiene. Ver
	// diColumnaPDFPath y diColumnaControlTexto.
	colPDFPath := ""
	if diColumnaPDFPath(ctl) {
		colPDFPath = ", COALESCE(pdf_path,'')"
	}
	colControl := ""
	if diColumnaControlTexto(ctl) {
		colControl = ", COALESCE(nro_control_texto,'')"
	}

	for inicio := 0; inicio < len(lote); inicio += diLoteDocs {
		fin := inicio + diLoteDocs
		if fin > len(lote) {
			fin = len(lote)
		}
		partes := lote[inicio:fin]

		q := `SELECT invoice_id, COALESCE(status,''), COALESCE(strong_id,''), COALESCE(nro_control,0),
			COALESCE(pdf_url,''), COALESCE(xml_url,''), COALESCE(error_message,'')` + colPDFPath + colControl + `
			FROM digital_invoice_log
			WHERE company_id = ? AND invoice_id IN (?` + strings.Repeat(",?", len(partes)-1) + `)`
		args := make([]interface{}, 0, len(partes)+1)
		args = append(args, tenant)
		for _, n := range partes {
			args = append(args, n)
		}

		rows, err := ctl.Query(q, args...)
		if err != nil {
			return salida
		}
		for rows.Next() {
			var d diDocFila
			destinos := []interface{}{&d.InvoiceNo, &d.Estado, &d.StrongID, &d.NroControl, &d.PDF, &d.XML, &d.Error}
			if colPDFPath != "" {
				destinos = append(destinos, &d.PDFPath)
			}
			if colControl != "" {
				destinos = append(destinos, &d.ControlTexto)
			}
			if err := rows.Scan(destinos...); err != nil {
				continue
			}
			salida[d.InvoiceNo] = d
		}
		rows.Close()
	}
	return salida
}

// diColumnaControlTexto comprueba que la columna del número de control textual
// exista en ESTA base de control.
//
// POR QUÉ HACE FALTA. Unidigital devuelve el control como entero y CG lo
// devuelve como texto con serie ("00-00000056"). La columna nro_control es INT
// y no puede guardar la serie; la textual nació después. Una base sin la
// migración tiene que seguir funcionando (mostrando el número entero) en vez de
// reventar cada consulta del listado de facturas.
func diColumnaControlTexto(ctl *sql.DB) bool {
	if ctl == nil {
		return false
	}
	rows, err := ctl.Query(`SELECT nro_control_texto FROM digital_invoice_log LIMIT 1`)
	if err != nil {
		return false
	}
	rows.Close()
	return true
}

// diDocDeFactura devuelve el documento digital de UNA factura.
func diDocDeFactura(ctl *sql.DB, tenant string, correlativo int64) (diDocFila, bool) {
	d, ok := diDocsDeFacturas(ctl, tenant, []int64{correlativo})[correlativo]
	return d, ok
}

// diDocsDeFacturasDeLaPetición es el acceso desde un handler: resuelve la
// empresa y la base de control, y devuelve vacío si alguna de las dos no está.
func (s *Server) diDocsDeFacturasDeLaPetición(r *http.Request, numeros []int64) map[int64]diDocFila {
	ctl, err := s.controlStore()
	if err != nil {
		return map[int64]diDocFila{}
	}
	return diDocsDeFacturas(ctl, s.tenantOf(r), numeros)
}

// diEnlaceInterno es el enlace que ven las PANTALLAS: manda la copia guardada
// en el ERP cuando existe, porque no caduca, no depende de que la imprenta siga
// alojando el archivo y quien la abre tiene sesión. Mientras no haya copia,
// manda el enlace del proveedor ya validado. Es la única función que decide
// esto: cambiar el criterio es cambiarla a ella, no cinco plantillas.
func (s *Server) diEnlaceInterno(invoicePK int64, d diDocFila) string {
	if strings.TrimSpace(d.PDFPath) != "" {
		return "/invoice/fiscal/" + strconv.FormatInt(invoicePK, 10)
	}
	return diURLSegura(d.PDF)
}

// diEnlacePublico es el enlace que va dentro del CORREO al cliente: una persona
// que recibe el mensaje no tiene sesión en el ERP, así que un enlace interno no
// le abriría nada. Ahí solo sirve la URL del proveedor, validada.
func diEnlacePublico(d diDocFila) string {
	return diURLSegura(d.PDF)
}

func (s *Server) diEnlaceXML(d diDocFila) string {
	return diURLSegura(d.XML)
}

// diJSONDoc empaqueta el documento para el navegador de una lista.
//
// El número de control que se muestra es el COMPLETO cuando la imprenta lo da
// así (CG: "00-00000056"). Mostrar solo la parte numérica de un documento que
// el cliente tiene impreso con su serie no sirve para conciliar nada.
func (s *Server) diJSONDoc(invoicePK int64, d diDocFila) map[string]interface{} {
	pdf := s.diEnlaceInterno(invoicePK, d)
	xml := s.diEnlaceXML(d)
	control := strings.TrimSpace(d.ControlTexto)
	if control == "" && d.NroControl > 0 {
		control = strconv.FormatInt(d.NroControl, 10)
	}
	return map[string]interface{}{
		"tiene":   d.Estado != "" || pdf != "" || xml != "",
		"estado":  d.Estado,
		"pdf":     pdf,
		"xml":     xml,
		"control": control,
		"error":   d.Error,
	}
}

// ---------- Ajuste de entrega ----------

// Claves de la configuración en fiscal_setting: la misma tabla clave/valor que
// usan los ajustes fiscales, porque esto se decide por empresa y vive en la
// base de la empresa.
const (
	diClaveEntrega = "invoice_delivery"         // "1" = entrega activada
	diClaveCanal   = "invoice_delivery_channel" // id del canal de correo
)

// diEntregaConfig dice si la empresa entrega la factura a su cliente al
// emitirla y por qué canal. Sin ajuste guardado la respuesta es NO: mandar un
// correo a un cliente es una decisión del dueño, no un efecto secundario de
// tener una imprenta configurada.
func (s *Server) diEntregaConfig(db *sql.DB) (activa bool, canalID int64) {
	if db == nil {
		return false, 0
	}
	rows, err := db.Query("SELECT `key`, value FROM fiscal_setting WHERE `key` IN (?, ?)",
		diClaveEntrega, diClaveCanal)
	if err != nil {
		return false, 0
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) != nil {
			continue
		}
		switch k {
		case diClaveEntrega:
			activa = v == "1"
		case diClaveCanal:
			canalID, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		}
	}
	return activa, canalID
}

// diCanalEntrega resuelve el canal de correo con el que se entrega.
//
// Si el ajuste nombra un canal, se usa ese —y si desapareció o quedó apagado,
// se dice; no se cae en silencio a otro, porque quien eligió «Cobranzas» no
// esperaba que la factura saliera por «Marketing». Sin canal elegido, el
// primero de correo activo, que es la misma regla que usan las herramientas.
func (s *Server) diCanalEntrega(db *sql.DB, canalID int64) (notifyChannel, error) {
	if canalID > 0 {
		c, err := s.notifyCanalPorID(db, canalID)
		if err != nil {
			return c, err
		}
		if c.Kind != "email" {
			return c, fmt.Errorf("el canal «%s» no es de correo: la entrega de facturas sale por correo", c.Name)
		}
		if c.IsActive != 1 {
			return c, fmt.Errorf("el canal «%s» está apagado", c.Name)
		}
		return c, nil
	}
	for _, c := range s.notifyCanales(db, true) {
		if c.Kind == "email" {
			return c, nil
		}
	}
	return notifyChannel{}, fmt.Errorf("no hay ningún canal de correo activo: Configuración > Canales de aviso")
}

// ---------- Entrega ----------

// diEntregar entrega el documento fiscal de UNA factura a su cliente.
//
// Recibe la base de la empresa, la de control y —si la llamada viene del
// navegador— la petición, que es de donde sale el usuario que aparece en la
// auditoría. El runner pasa una petición sintética con el ámbito de la empresa
// del documento (ver requestForTenant) porque ese proceso no recibe peticiones
// HTTP y la factura puede no ser de la empresa activa.
//
// req == nil significa «sin peticionario»: no se audita, porque un registro
// escrito sin ámbito caería contra la empresa activa por falta de contexto,
// que es exactamente lo que no debe pasar.
func (s *Server) diEntregar(req *http.Request, db, ctl *sql.DB, tenant string, invoicePK int64, forzar bool) (map[string]interface{}, error) {
	if db == nil {
		return nil, fmt.Errorf("la base de la empresa no está disponible")
	}

	// 1) La factura, con su correlativo —es la clave con la que la imprenta
	//    guarda el documento—, su cliente y su moneda.
	var correlativo, control, fecha, moneda sql.NullString
	var customerID int64
	var total, iva float64
	err := db.QueryRow(`SELECT IFNULL(invoice,''), customer_id, IFNULL(control_no,''),
		IFNULL(CAST(date AS CHAR),''), IFNULL(currency_code,'VES'), total_amount, IFNULL(total_vat_amnt,0)
		FROM invoice WHERE invoice_id = ?`, invoicePK).
		Scan(&correlativo, &customerID, &control, &fecha, &moneda, &total, &iva)
	if err != nil {
		return nil, fmt.Errorf("la factura %d no se pudo leer: %w", invoicePK, err)
	}

	// 2) El documento emitido. Sin imprenta digital no hay nada que entregar, y
	//    sin enlace navegable el cliente recibiría un correo que no abre nada.
	if ctl == nil {
		return nil, fmt.Errorf("la base de control no está disponible")
	}
	doc, ok := diDocDeFactura(ctl, tenant, enteroDeTexto(correlativo.String))
	if !ok {
		return nil, fmt.Errorf("la factura %d no salió por facturación digital: no tiene documento fiscal que entregar", invoicePK)
	}
	// El correo lleva la URL PÚBLICA: el cliente no tiene sesión en el ERP.
	pdf := diEnlacePublico(doc)
	xml := s.diEnlaceXML(doc)
	if pdf == "" && xml == "" {
		estado := doc.Estado
		if estado == "" {
			estado = "sin estado"
		}
		return nil, fmt.Errorf("la imprenta digital aún no devuelve un enlace válido para la factura %d (estado %s)", invoicePK, estado)
	}

	// 3) El canal: el que el ajuste eligió, o el primero de correo activo.
	_, canalElegido := s.diEntregaConfig(db)
	canal, err := s.diCanalEntrega(db, canalElegido)
	if err != nil {
		return nil, err
	}

	// 4) El destinatario: de la ficha, y si no tiene correo, no se adivina.
	nombre, email, _ := s.finContactoCliente(db, customerID)
	if strings.TrimSpace(email) == "" {
		return nil, fmt.Errorf("el cliente de la factura %d no tiene correo en su ficha: cárguelo en Clientes y vuelva a intentarlo", invoicePK)
	}

	// 5) El texto. Lo redacta el sistema con lo que el sistema sabe: número,
	//    control, fecha, total y enlaces. El modelo no participa.
	asunto, cuerpo := s.diCuerpoEntrega(db, correlativo.String, control.String, fecha.String,
		nombre, total, iva, moneda.String, pdf, xml)

	alerta := map[string]interface{}{
		"scope": scopeCxC, "kind": "factura_emitida",
		"party_type": "customer", "party_id": customerID, "party": nombre,
		"motivo": "Factura " + correlativo.String + " emitida",
		"metricas": map[string]interface{}{
			"total": total, "iva": iva, "control": control.String,
			"fecha": fecha.String, "pdf": pdf,
		},
	}

	res, err := s.financeEnviarAviso(req, db, canal, alerta, asunto, cuerpo,
		"invoice", strconv.FormatInt(invoicePK, 10), forzar)
	if err != nil {
		return nil, err
	}

	// 6) La traza. El mensaje ya quedó en finance_message con su resultado; aquí
	//    se encadena además en la auditoría, como cualquier otra mutación.
	if enviado, _ := res["enviado"].(bool); enviado && req != nil {
		s.appendAudit(req, "system", clientIP(req), "send", "Facturación digital",
			"invoice", strconv.FormatInt(invoicePK, 10), nil, map[string]interface{}{
				"destino": res["destino"], "canal": canal.Name, "asunto": asunto,
				"pdf": pdf, "xml": xml,
			})
	}
	return res, nil
}

// diCuerpoEntrega redacta el correo.
//
// El texto vive aquí por la misma razón que finPlantillaAviso: lo que dice el
// sistema tiene que decirlo el sistema, no una persona ni un modelo. Si algo
// cambia (otra moneda, otro formato) cambia en un solo sitio.
func (s *Server) diCuerpoEntrega(db *sql.DB, correlativo, control, fecha, cliente string,
	total, iva float64, moneda, pdf, xml string) (asunto, cuerpo string) {

	empresa := s.finEmpresa(db)
	if empresa == "" {
		empresa = "su proveedor"
	}
	if strings.TrimSpace(cliente) == "" {
		cliente = "cliente"
	}

	asunto = "Factura N° " + strings.TrimSpace(correlativo) + " — " + empresa

	var b strings.Builder
	fmt.Fprintf(&b, "Estimado %s:\n\n", cliente)
	fmt.Fprintf(&b, "Su factura N° %s ya fue emitida por %s.\n", strings.TrimSpace(correlativo), empresa)
	if f := strings.TrimSpace(fecha); f != "" {
		fmt.Fprintf(&b, "Fecha: %s\n", f)
	}
	if c := strings.TrimSpace(control); c != "" {
		fmt.Fprintf(&b, "Número de control: %s\n", c)
	}
	fmt.Fprintf(&b, "Total: %s %s\n", moneda, money(total))
	if iva > 0 {
		fmt.Fprintf(&b, "IVA: %s %s\n", moneda, money(iva))
	}
	b.WriteString("\n")
	if pdf != "" {
		b.WriteString("Documento fiscal (PDF): " + pdf + "\n")
	}
	if xml != "" {
		b.WriteString("XML firmado: " + xml + "\n")
	}
	b.WriteString("\nEste mensaje lo envía el sistema automáticamente al emitir la factura.\n")
	if empresa != "su proveedor" {
		fmt.Fprintf(&b, "\nAtentamente,\n%s\n", empresa)
	}
	return asunto, b.String()
}

// ---------- Entrega automática ----------

// diEntregarAutomatica es lo que corre cuando una factura queda emitida.
//
// En su propio goroutine a propósito: el runner existe para emitir y no debe
// atascarse frente a un SMTP que tarda quince segundos por mensaje. Si dos
// pasadas llegaran a cruzarse, la clave de idempotencia de finance_message deja
// pasar a una sola y la otra queda registrada como duplicada.
func (s *Server) diEntregarAutomatica(tenant, correlativo string) {
	// La base se pide POR EMPRESA y no por petición: aquí no hay petición y la
	// activa puede ser otra. La petición sintética existe para que la auditoría
	// tenga de dónde sacar la empresa.
	db := s.dbForTenant(tenant)
	if db == nil {
		log.Printf("facturación digital: sin base de %s para entregar la factura %s", tenant, correlativo)
		return
	}
	if activa, _ := s.diEntregaConfig(db); !activa {
		return
	}
	req := s.requestForTenant(tenant)
	var pk int64
	if err := db.QueryRow(`SELECT invoice_id FROM invoice WHERE invoice = ? LIMIT 1`, correlativo).Scan(&pk); err != nil {
		log.Printf("facturación digital: no se encontró la factura %s de %s para entregar: %v", correlativo, tenant, err)
		return
	}
	ctl, err := s.controlStore()
	if err != nil {
		log.Printf("facturación digital: sin base de control para entregar %s/%s: %v", tenant, correlativo, err)
		return
	}
	if _, err := s.diEntregar(req, db, ctl, tenant, pk, false); err != nil {
		log.Printf("facturación digital: entrega de la factura %s de %s: %v", correlativo, tenant, err)
	}
}

// ---------- Rutas ----------

// registerDigitalDocRoutes deja las rutas de este archivo registradas junto a
// las de facturación digital.
func (s *Server) registerDigitalDocRoutes() {
	s.mux.HandleFunc("GET /invoice/fiscal/{id}", s.invoiceFiscal)
	s.mux.HandleFunc("POST /settings/digital-invoice/delivery/save", s.diDeliverySave)
}

// invoiceFiscal sirve el documento fiscal de UNA factura: la copia guardada en
// el ERP si existe, y si no, el enlace del proveedor.
//
// Pasa por la misma puerta de permisos que el resto de facturación: el PDF es
// un documento fiscal de la empresa, no un archivo público con nombre adivinable.
func (s *Server) invoiceFiscal(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "manage_invoice", "customer_ledger") {
		return
	}
	pk := parseInt64(r.PathValue("id"))
	if pk <= 0 {
		jsonError(w, "Factura inválida", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)
	if db == nil {
		jsonError(w, "Empresa no disponible", http.StatusInternalServerError)
		return
	}
	var correlativo sql.NullString
	if err := db.QueryRow(`SELECT IFNULL(invoice,'') FROM invoice WHERE invoice_id = ?`, pk).Scan(&correlativo); err != nil {
		jsonError(w, "Factura no encontrada", http.StatusNotFound)
		return
	}
	ctl, err := s.controlStore()
	if err != nil {
		jsonError(w, "La base de control no está disponible", http.StatusServiceUnavailable)
		return
	}
	doc, ok := diDocDeFactura(ctl, s.tenantOf(r), enteroDeTexto(correlativo.String))
	if !ok {
		jsonError(w, "Esta factura no tiene documento fiscal digital", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("xml") == "1" {
		if destino := s.diEnlaceXML(doc); destino != "" {
			http.Redirect(w, r, destino, http.StatusFound)
			return
		}
		jsonError(w, "La imprenta digital no ha devuelto el XML de esta factura", http.StatusNotFound)
		return
	}

	// La copia propia manda: es el archivo que guardó este sistema y el que
	// seguirá existiendo cuando la imprenta deje de alojar el documento. Sin
	// copia, se redirige a la URL del proveedor —que el navegador abrirá con la
	// sesión del que pidió el PDF, no con otra—.
	if propia := strings.TrimSpace(doc.PDFPath); propia != "" && s.uploadMgr != nil {
		if err := s.uploadMgr.ServeFile(w, r, propia, diCompanyID(db), db); err != nil {
			jsonError(w, "La copia archivada de esta factura no se encontró en el servidor: "+err.Error(), http.StatusNotFound)
		}
		return
	}
	if destino := diEnlacePublico(doc); destino != "" {
		http.Redirect(w, r, destino, http.StatusFound)
		return
	}
	jsonError(w, "La imprenta digital no ha devuelto el PDF de esta factura", http.StatusNotFound)
}

// diDeliverySave guarda el ajuste de entrega de facturas.
//
// Ruta /settings: exige admin/master como toda la pantalla de facturación
// digital. El canal se valida AQUÍ, al guardar, por lo mismo que los canales:
// un ajuste a medias se descubre cuando llega la factura, que es el peor
// momento.
func (s *Server) diDeliverySave(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	_ = r.ParseMultipartForm(32 << 20)
	db := s.dbFor(r)

	activa := r.FormValue("enabled") == "1"
	canalID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("channel_id")), 10, 64)

	if activa {
		if canalID <= 0 {
			jsonError(w, "Elija el canal de correo por el que se entregarán las facturas.", http.StatusBadRequest)
			return
		}
		if _, err := s.diCanalEntrega(db, canalID); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	valor := "0"
	if activa {
		valor = "1"
	}
	if err := s.upsertFiscalSetting(db, diClaveEntrega, valor); err != nil {
		jsonError(w, "No se pudo guardar la entrega: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if activa {
		if err := s.upsertFiscalSetting(db, diClaveCanal, strconv.FormatInt(canalID, 10)); err != nil {
			jsonError(w, "No se pudo guardar el canal: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), clientIP(r), "update", "Configuración",
		"fiscal_setting", diClaveEntrega, nil, map[string]interface{}{
			"invoice_delivery": valor, "channel_id": canalID,
		})
	msg := "La entrega automática de facturas quedó desactivada."
	if activa {
		if c, err := s.diCanalEntrega(db, canalID); err == nil {
			msg = "Las facturas emitidas se entregarán a su cliente por «" + c.Name + "»."
		}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg})
}
