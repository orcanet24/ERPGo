package web

// purchase_hash.go — Clasificación del documento de compra y candado contra
// duplicados.
//
// DOS COMPRAS DISTINTAS, DOS EFECTOS DISTINTOS.
//
// Una compra por FACTURA del proveedor es un documento fiscal: número de
// factura, número de control y RIF son los datos que el Libro de Compras (IVA) y
// las retenciones necesitan. Una compra por ORDEN o por NOTA DE ENTREGA mete
// mercancía al almacén y genera deuda, pero no da crédito fiscal: no hay IVA que
// descontar ni retención que practicar. Hasta ahora las dos se guardaban igual y
// el libro de compras las tomaba a todas, así que una nota de entrega aparecía
// como crédito fiscal inexistente.
//
// EL CANDADO.
//
// El número de documento solo no alcanza como identidad: un proveedor puede
// repetir el número de una orden, o no traer número. Por eso la identidad de la
// compra se calcula con TODO lo que la define —proveedor, RIF, tipo de
// documento, número de factura, número de control y los renglones (producto,
// cantidad y precio) ordenados— y se guarda como hash en
// `product_purchase.purchase_hash`, con un índice único. Si alguien —persona o
// agente— intenta cargar la misma compra otra vez, la base lo rechaza y el
// sistema responde con el número de la compra que YA existe.
//
// Y LO QUE NO ES DUPLICADO: el mismo proveedor con los mismos productos y OTRO
// número de factura u orden es otra compra, porque el número forma parte del
// hash. Es la regla que pidió el negocio, escrita en un solo sitio.
//
// El hash cubre también lo que ya estaba cargado antes de existir el candado:
// para esas filas (sin hash) se compara proveedor + números normalizados, que es
// la identidad del documento.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Tipos de documento de compra.
const (
	CompraFactura     = "factura"
	CompraNotaEntrega = "nota_entrega"
	CompraOrden       = "orden"
	CompraSinDoc      = "sin_documento"
)

// CompraKind normaliza el tipo recibido.
//
// Un valor vacío o desconocido cae en 'factura', que es con lo que se migran las
// compras ya registradas: el libro de compras las sigue tomando como antes en
// vez de vaciarse de golpe en el mes en que se aplica el cambio.
func CompraKind(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case CompraNotaEntrega, "nota", "entrega", "nota-de-entrega", "nota de entrega":
		return CompraNotaEntrega
	case CompraOrden, "oc", "orden_de_compra", "orden de compra":
		return CompraOrden
	case CompraSinDoc, "sin-documento", "sin documento", "sin":
		return CompraSinDoc
	default:
		return CompraFactura
	}
}

// EsCompraFiscal dice si el tipo de documento da crédito fiscal (IVA y
// retenciones). Solo la factura del proveedor lo da.
func EsCompraFiscal(kind string) bool { return CompraKind(kind) == CompraFactura }

// EtiquetaCompra es el texto que se muestra en pantalla y en los informes.
func EtiquetaCompra(kind string) string {
	switch CompraKind(kind) {
	case CompraNotaEntrega:
		return "Nota de entrega"
	case CompraOrden:
		return "Orden de compra"
	case CompraSinDoc:
		return "Sin documento"
	default:
		return "Factura"
	}
}

// compraLineaHash es un renglón reducido a lo que identifica la compra.
type compraLineaHash struct {
	ProductID string
	Qty       float64
	Rate      float64
}

// normalizaDoc deja los números comparables: sin espacios, sin guiones ni puntos
// y en mayúsculas. «00-00000123» y «0000000123» son el mismo número de control
// escrito de dos maneras, y una compra escrita de dos maneras es una sola.
func normalizaDoc(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
}

// hashDeCompra calcula la identidad de una compra: la cadena canónica de todo lo
// que la define, con sha256.
//
// Los renglones se ordenan antes de entrar (el mismo contenido cargado en otro
// orden es la misma compra) y las cantidades y precios se fijan a dos decimales,
// para que 1 y 1.00 den el mismo hash.
func hashDeCompra(supplierID int64, rif, kind, controlNo, chalanNo string, lineas []compraLineaHash, total float64) string {
	orden := make([]compraLineaHash, len(lineas))
	copy(orden, lineas)
	sort.Slice(orden, func(i, j int) bool {
		if orden[i].ProductID != orden[j].ProductID {
			return orden[i].ProductID < orden[j].ProductID
		}
		if orden[i].Qty != orden[j].Qty {
			return orden[i].Qty < orden[j].Qty
		}
		return orden[i].Rate < orden[j].Rate
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "v1|prov=%d|rif=%s|tipo=%s|control=%s|factura=%s|total=%.2f|renglones=",
		supplierID, strings.ToUpper(NormalizeRIF(rif)), CompraKind(kind),
		normalizaDoc(controlNo), normalizaDoc(chalanNo), round2(total))
	for i, l := range orden {
		if i > 0 {
			sb.WriteByte(';')
		}
		fmt.Fprintf(&sb, "%s:%.2f:%.2f", strings.TrimSpace(l.ProductID), round2(l.Qty), round2(l.Rate))
	}
	suma := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(suma[:])
}

// compraLineasDeFormulario lee los renglones del formulario tal como los necesita
// el hash: producto, cantidad y precio. Es el MISMO formulario que lee
// purchaseSave, así que el humano y el agente calculan idéntico hash.
func compraLineasDeFormulario(r *http.Request) []compraLineaHash {
	if r.Form == nil {
		_ = r.ParseMultipartForm(32 << 20)
	}
	ids := r.Form["product_id"]
	qtys := r.Form["product_quantity"]
	rates := r.Form["product_rate"]
	out := make([]compraLineaHash, 0, len(ids))
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		l := compraLineaHash{ProductID: id}
		if i < len(qtys) {
			l.Qty = parseMoney(qtys[i])
		}
		if i < len(rates) {
			l.Rate = parseMoney(rates[i])
		}
		out = append(out, l)
	}
	return out
}

// hashDeCompraDelFormulario arma el hash con lo que viene en la petición. Lo usan
// tanto la comprobación previa del formulario como el guardado, para que no haya
// dos maneras de calcular lo mismo.
func hashDeCompraDelFormulario(r *http.Request) (string, int64, string, string, string) {
	supplierID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("supplier_id")), 10, 64)
	rif := r.FormValue("supplier_rif")
	kind := r.FormValue("document_kind")
	control := r.FormValue("control_no")
	chalan := r.FormValue("chalan_no")
	total := parseMoney(r.FormValue("grand_total_price"))
	return hashDeCompra(supplierID, rif, kind, control, chalan, compraLineasDeFormulario(r), total), supplierID, control, chalan, kind
}

// compraExistente busca una compra ya registrada con la misma identidad: primero
// por el hash (las que se cargaron con el candado puesto) y, si no aparece, por
// proveedor + números (las anteriores al candado, que no tienen hash).
func (s *Server) compraExistente(r *http.Request, hash string, supplierID int64, controlNo, chalanNo string) (map[string]interface{}, bool) {
	if hash != "" {
		if m, ok := s.compraPorHash(r, hash); ok {
			return m, true
		}
	}
	return s.compraLegacyDuplicada(r, supplierID, controlNo, chalanNo)
}

// compraPorHash devuelve la compra con ese hash, si existe.
func (s *Server) compraPorHash(r *http.Request, hash string) (map[string]interface{}, bool) {
	var id int64
	var fecha, chalan, control, kind sql.NullString
	var total float64
	err := s.dbFor(r).QueryRow(`SELECT purchase_id, IFNULL(purchase_date,''), IFNULL(chalan_no,''),
		IFNULL(control_no,''), IFNULL(document_kind,''), grand_total_amount
		FROM product_purchase WHERE purchase_hash = ? LIMIT 1`, hash).
		Scan(&id, &fecha, &chalan, &control, &kind, &total)
	if err != nil {
		return nil, false
	}
	return compraHallada(id, fecha.String, chalan.String, control.String, kind.String, total), true
}

// compraLegacyDuplicada busca, entre las compras SIN hash del mismo proveedor, una
// con los mismos números de documento. Cubre lo que ya estaba cargado antes del
// candado, que es justo el material con el que se duplicaba a mano.
//
// La comparación se hace en Go con los números normalizados, no en SQL: la base
// guarda «00-00000123» y «0000000123» como textos distintos y el candado tiene
// que ver que son el mismo documento.
func (s *Server) compraLegacyDuplicada(r *http.Request, supplierID int64, controlNo, chalanNo string) (map[string]interface{}, bool) {
	control := normalizaDoc(controlNo)
	chalan := normalizaDoc(chalanNo)
	if supplierID <= 0 || (control == "" && chalan == "") {
		return nil, false
	}
	rows, err := s.dbFor(r).Query(`SELECT purchase_id, IFNULL(purchase_date,''), IFNULL(chalan_no,''),
		IFNULL(control_no,''), IFNULL(document_kind,''), grand_total_amount
		FROM product_purchase WHERE supplier_id = ? AND purchase_hash IS NULL
		ORDER BY purchase_id DESC LIMIT 300`, supplierID)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var fecha, chalanDB, controlDB, kind sql.NullString
		var total float64
		if err := rows.Scan(&id, &fecha, &chalanDB, &controlDB, &kind, &total); err != nil {
			continue
		}
		cDB, chDB := normalizaDoc(controlDB.String), normalizaDoc(chalanDB.String)
		iguales := (control != "" && control == cDB) || (chalan != "" && chalan == chDB)
		if iguales {
			return compraHallada(id, fecha.String, chalanDB.String, controlDB.String, kind.String, total), true
		}
	}
	return nil, false
}

// compraHallada resume la compra encontrada, que es lo que se devuelve a quien
// intentó cargarla de nuevo: sin el número, el aviso «ya existe» es una pared.
func compraHallada(id int64, fecha, chalan, control, kind string, total float64) map[string]interface{} {
	return map[string]interface{}{
		"purchase_id":   id,
		"date":          fecha,
		"chalan_no":     chalan,
		"control_no":    control,
		"document_kind": CompraKind(kind),
		"kind_label":    EtiquetaCompra(kind),
		"total":         money(total),
	}
}

// purchaseDupCheck responde si esa compra ya está cargada, ANTES de guardarla.
//
// El candado de verdad es el índice único de la base; esto existe para poder
// decir QUÉ compra ya existe —tipo, número, fecha y monto— en lugar de un error
// seco después de haber escrito media compra. Lo usa el formulario al enviar, y
// un agente puede usarlo igual antes de cargar: mismo cálculo, misma respuesta.
func (s *Server) purchaseDupCheck(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	hash, supplierID, control, chalan, kind := hashDeCompraDelFormulario(r)
	resp := map[string]interface{}{"status": 1, "duplicate": false, "document_kind": kind}
	if m, dup := s.compraExistente(r, hash, supplierID, control, chalan); dup {
		resp = compraDuplicadaRespuesta(m)
	}
	writeJSON(w, resp)
}

// compraEsDuplicadoDeClave dice si el error del INSERT es la clave única del
// candado. Se mira el texto porque MySQL y SQLite lo escriben distinto («Duplicate
// entry» frente a «UNIQUE constraint failed»), y lo que hay que distinguir aquí
// no es el motor sino «ya está registrada» de «falló la escritura».
func compraEsDuplicadoDeClave(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, trozo := range []string{"duplicate entry", "unique constraint", "errno 1062", "1062"} {
		if strings.Contains(m, trozo) {
			return true
		}
	}
	return false
}

// compraDuplicadaRespuesta arma la respuesta del candado, igual para la
// comprobación previa del formulario y para el guardado: el operador y el agente
// tienen que leer lo mismo.
func compraDuplicadaRespuesta(m map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"status":        1,
		"duplicate":     true,
		"purchase_id":   m["purchase_id"],
		"purchase":      m,
		"msg":           mensajeCompraDuplicada(m),
		"document_kind": m["document_kind"],
		"kind_label":    m["kind_label"],
	}
}

// mensajeCompraDuplicada explica QUÉ compra ya existe. Un «duplicado» sin el
// número obliga a buscarla a mano.
func mensajeCompraDuplicada(m map[string]interface{}) string {
	return fmt.Sprintf("Esa compra ya está registrada: %s #%v del %v por %v (%s). No se volvió a registrar.",
		m["kind_label"], m["purchase_id"], m["date"], m["total"], nombreDocumentoCompra(m))
}

// nombreDocumentoCompra nombra el documento con el número que lo identifica.
func nombreDocumentoCompra(m map[string]interface{}) string {
	control, _ := m["control_no"].(string)
	chalan, _ := m["chalan_no"].(string)
	switch {
	case chalan != "" && control != "":
		return "factura " + chalan + ", control " + control
	case chalan != "":
		return "documento " + chalan
	case control != "":
		return "control " + control
	default:
		return "sin número de documento"
	}
}
