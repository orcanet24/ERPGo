package web

// purchase_inbox_extract.go — Qué se puede leer de un documento recibido.
//
// LA REGLA DE ORO DE ESTE ARCHIVO.
//
// Aquí NO se adivina: lo que se lee con seguridad se propone, y lo que no se
// pudo leer queda a la vista para que lo resuelva una persona. El formulario de
// compra sigue exigiendo confirmación, el candado anti-duplicado sigue corriendo
// al guardar, y ningún campo leído aquí se escribe solo en la base. El sistema
// propone; el humano y la base deciden.
//
// POR QUÉ UN RECORRIDO TOLERANTE Y NO UN ESQUEMA POR PROVEEDOR.
//
// No existe un formato único de factura de proveedor en Venezuela: cada imprenta
// y cada sistema emite su XML con sus propios nombres de etiqueta. Un traductor
// por proveedor sería un catálogo que crece para siempre y que se rompe en
// silencio cuando llega uno nuevo. El recorrido de abajo busca CAMPOS por
// nombre («rif», «control», «fecha», «total», «iva») en cualquier profundidad,
// con la primera aparición válida ganando. Lo que encuentre se propone; lo que
// no, no se inventa.
//
// El OCR de PDF y fotos queda fuera de este archivo a propósito: es la Fase D
// que el negocio pidió ver más adelante. Un PDF o una imagen llegan aquí,
// se guardan, se clasifican y se marcan `pendiente_ocr`; la bandeja los muestra
// igual y el operador los carga a mano mientras tanto.

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// inboxFormato clasifica el archivo recibido. Devuelve "" si no es un formato
// que este sistema sepa tratar, que es la señal para rechazarlo EN LA PUERTA y
// no dejar basura en el directorio de la empresa.
func inboxFormato(nombre, mime string) string {
	nombre = strings.ToLower(strings.TrimSpace(nombre))
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasSuffix(nombre, ".xml") || strings.Contains(mime, "xml"):
		return "xml"
	case strings.HasSuffix(nombre, ".pdf") || strings.Contains(mime, "pdf"):
		return "pdf"
	case strings.HasSuffix(nombre, ".jpg"), strings.HasSuffix(nombre, ".jpeg"),
		strings.HasSuffix(nombre, ".png"), strings.HasSuffix(nombre, ".webp"),
		strings.Contains(mime, "image/"):
		return "imagen"
	}
	return ""
}

// inboxExt es la extensión con la que se guarda, la del archivo tal como vino.
func inboxExt(nombre string) string {
	nombre = strings.TrimSpace(nombre)
	if i := strings.LastIndex(nombre, "."); i > 0 && i < len(nombre)-1 {
		return strings.ToLower(nombre[i:])
	}
	return ""
}

// extractoLinea es UN renglón leído del documento.
type extractoLinea struct {
	Descripcion string  `json:"descripcion"`
	Cantidad    float64 `json:"cantidad"`
	Precio      float64 `json:"precio"`
	Total       float64 `json:"total"`
}

// extractoDocumento es todo lo que se pudo leer de un archivo.
type extractoDocumento struct {
	Formato    string          `json:"formato"`    // xml | pdf | imagen
	Extraccion string          `json:"extraccion"` // xml | pendiente_ocr
	Kind       string          `json:"doc_kind"`   // factura | nota_entrega | orden
	RIF        string          `json:"rif"`
	Proveedor  string          `json:"proveedor"`
	Numero     string          `json:"numero"`  // número de factura del proveedor
	Control    string          `json:"control"` // número de control fiscal
	Fecha      string          `json:"fecha"`   // AAAA-MM-DD si se pudo normalizar
	Moneda     string          `json:"moneda"`
	Total      float64         `json:"total"`
	IVA        float64         `json:"iva"`
	Lineas     []extractoLinea `json:"lineas"`
	Campos     []string        `json:"campos"` // los que SÍ se leyeron
	Faltan     []string        `json:"faltan"` // los que no se pudieron leer
}

// anota apunta un campo leído sin repetirlo.
func (e *extractoDocumento) anota(nombre, valor string) {
	if valor == "" {
		return
	}
	for _, c := range e.Campos {
		if c == nombre {
			return
		}
	}
	e.Campos = append(e.Campos, nombre)
}

// falta anota un campo que NO se leyó, sin repetirlo.
func (e *extractoDocumento) falta(nombre string) {
	for _, c := range e.Faltan {
		if c == nombre {
			return
		}
	}
	e.Faltan = append(e.Faltan, nombre)
}

// inboxRenglon es un renglón en curso durante el recorrido.
type inboxRenglon struct {
	nombre string
	desc   string
	cant   string
	precio string
	total  string
}

// inboxRenglonVale dice si el renglón cerró con algo que valga la pena proponer.
func (l *inboxRenglon) vale() bool {
	return l.desc != "" || inboxDinero(l.cant) > 0 || inboxDinero(l.precio) > 0 || inboxDinero(l.total) > 0
}

// normalizaXML deja un nombre de etiqueta comparable: minúsculas y sin acentos.
// «Número» y «NUMERO» son la misma etiqueta, y un proveedor puede escribir cualquiera.
func normalizaXML(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			for _, rep := range []struct{ de, a rune }{
				{'á', 'a'}, {'é', 'e'}, {'í', 'i'}, {'ó', 'o'}, {'ú', 'u'}, {'ü', 'u'}, {'ñ', 'n'},
			} {
				if r == rep.de {
					return rep.a
				}
			}
		}
		return r
	}, s)
}

// inboxDinero lee un importe escrito como lo escriban: «1.234,56», «1,234.56»,
// «1234.56», «Bs 1.234,56» o vacío.
//
// La regla es el ÚLTIMO separador: es el decimal en los dos formatos que se usan
// en Venezuela y en los que vienen de sistemas anglosajones. Lo anterior son
// miles. Un valor sin ningún separador se lee tal cual.
func inboxDinero(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	// Quitar moneda, espacios y todo lo que no sea número o separador.
	v = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\u00a0' || r == '\t' {
			return -1
		}
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' {
			return r
		}
		return -1
	}, v)
	if v == "" {
		return 0
	}
	ult := strings.LastIndexAny(v, ".,")
	if ult < 0 {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil {
			return 0
		}
		return f
	}
	entero := strings.NewReplacer(".", "", ",", "").Replace(v[:ult])
	decimales := v[ult+1:]
	f, err := strconv.ParseFloat(entero+"."+decimales, 64)
	if err != nil {
		return 0
	}
	return f
}

// inboxFecha normaliza a AAAA-MM-DD cuando se puede. Lo que no se puede leer se
// devuelve como vino: el formulario tiene datepicker y la persona corrige.
func inboxFecha(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 10 {
		if v[4] == '-' && v[7] == '-' {
			return v[:10]
		}
		// DD/MM/AAAA o DD-MM-AAAA
		if (v[2] == '/' || v[2] == '-') && (v[5] == '/' || v[5] == '-') {
			return v[6:10] + "-" + v[3:5] + "-" + v[0:2]
		}
	}
	return v
}

// inboxNombreDice dice si el nombre de una etiqueta contiene alguna de las piezas.
func inboxNombreDice(nombre string, piezas ...string) bool {
	for _, p := range piezas {
		if strings.Contains(nombre, p) {
			return true
		}
	}
	return false
}

// inboxEsRenglon dice si la etiqueta abre (o cierra) un renglón del documento.
//
// La lista incluye los envoltorios («detalles»): un XML puede envolver los
// renglones en un contenedor que también se llame así. Se apila, y solo se
// propone un renglón que traiga algo; el envoltorío vacío se descarta solo.
func inboxEsRenglon(nombre string) bool {
	switch nombre {
	case "item", "detalle", "detalles", "renglon", "renglones", "linea", "lineas",
		"producto", "productos", "concepto", "conceptos":
		return true
	}
	return false
}

// extraerXML recorre el documento y propone lo que encuentre.
//
// Tolera XML malformado (Strict=false): un archivo con un `&` suelto no puede
// dejar de leerse entero por eso. El error, si lo hay, se devuelve junto al
// extracto parcial: lo leído vale, aunque el cierre del archivo esté roto.
func extraerXML(contenido []byte) (extractoDocumento, error) {
	ex := extractoDocumento{Formato: "xml", Extraccion: "xml", Kind: CompraFactura}
	dec := xml.NewDecoder(bytes.NewReader(contenido))
	dec.Strict = false

	var pila []*inboxRenglon // renglones abiertos: envoltorio > item
	var texto strings.Builder
	var lecturaErr error // el archivo vino incompleto: se lee lo que se pudo

	// actual es el renglón en curso (el tope de la pila), o nil si estamos en
	// la cabecera del documento.
	actual := func() *inboxRenglon {
		if len(pila) == 0 {
			return nil
		}
		return pila[len(pila)-1]
	}

	// aplica una regla de campo con el texto de una etiqueta ya cerrada.
	aplicar := func(nombre, valor string) {
		valor = strings.TrimSpace(valor)
		if valor == "" {
			return
		}
		if l := actual(); l != nil {
			switch {
			case l.desc == "" && inboxNombreDice(nombre, "descripcion", "detalle", "nombre",
				"denominacion", "producto", "articulo", "concepto"):
				l.desc = valor
			case l.cant == "" && inboxNombreDice(nombre, "cantidad", "qty", "unidades"):
				l.cant = valor
			case l.precio == "" && inboxNombreDice(nombre, "preciounitario", "precio", "costo",
				"valorunitario", "precioventa"):
				l.precio = valor
			case l.total == "" && inboxNombreDice(nombre, "importe", "montototal", "total",
				"totalitem", "valortotal"):
				l.total = valor
			}
			return
		}

		// El TIPO se decide antes del resto, y no es un detalle de orden: la
		// etiqueta «tipoDocumento» comparte la palabra «documento» con la de
		// «número de factura», así que dentro del switch de abajo se comía el caso
		// y una nota de entrega se proponía como factura —que es precisamente lo
		// que la clasificación existe para evitar.
		if inboxNombreDice(nombre, "tipodocumento", "tipocomprobante") {
			if k := CompraKind(inboxTipoDocumento(valor)); k != CompraFactura {
				ex.Kind = k
				ex.anota("tipo de documento", valor)
			}
			return
		}

		switch {
		case ex.RIF == "" && inboxNombreDice(nombre, "rif", "nit", "cedula"):
			if strings.ContainsAny(valor, "0123456789") {
				ex.RIF = valor
				ex.anota("RIF", valor)
			}
		case ex.Control == "" && inboxNombreDice(nombre, "control"):
			ex.Control = valor
			ex.anota("número de control", valor)
		case ex.Numero == "" && inboxNombreDice(nombre, "numero", "nro", "num", "folio",
			"comprobante", "documento"):
			// «document_kind», «numero_documento» y parecidos traen basura al
			// lado; un número de factura corto y con dígitos sí entra.
			if len(valor) <= 40 && strings.ContainsAny(valor, "0123456789") {
				ex.Numero = valor
				ex.anota("número de factura", valor)
			}
		case ex.Fecha == "" && inboxNombreDice(nombre, "fecha"):
			ex.Fecha = inboxFecha(valor)
			ex.anota("fecha", valor)
		case ex.Moneda == "" && inboxNombreDice(nombre, "moneda", "divisa", "currency"):
			if len(valor) <= 10 {
				ex.Moneda = strings.ToUpper(valor)
				ex.anota("moneda", valor)
			}
		case ex.Total == 0 && inboxTotalClave(nombre):
			if n := inboxDinero(valor); n > 0 {
				ex.Total = n
				ex.anota("total", valor)
			}
		case ex.IVA == 0 && inboxNombreDice(nombre, "iva", "impuesto", "igtf"):
			if n := inboxDinero(valor); n > 0 {
				ex.IVA = n
				ex.anota("IVA", valor)
			}
		case ex.Proveedor == "" && inboxNombreDice(nombre, "razonsocial", "denominacion"):
			if len(valor) > 2 && len(valor) < 200 {
				ex.Proveedor = valor
				ex.anota("razón social", valor)
			}
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Archivo cortado o con sintaxis rota: se queda lo leído hasta aquí.
			// NO es un error de la carga: el extracto parcial vale como propuesta,
			// y el llamante lo anota en `notas` para que se vea en la pantalla.
			lecturaErr = err
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			nombre := normalizaXML(t.Name.Local)
			texto.Reset()
			if inboxEsRenglon(nombre) {
				pila = append(pila, &inboxRenglon{nombre: nombre})
			}
		case xml.CharData:
			texto.Write(t)
		case xml.EndElement:
			nombre := normalizaXML(t.Name.Local)
			if l := actual(); l != nil && nombre == l.nombre {
				if l.vale() {
					cant, precio, total := inboxDinero(l.cant), inboxDinero(l.precio), inboxDinero(l.total)
					if total == 0 && cant > 0 && precio > 0 {
						total = cant * precio
					}
					if cant == 0 && precio == 0 && total > 0 {
						cant = 1
					}
					ex.Lineas = append(ex.Lineas, extractoLinea{
						Descripcion: strings.TrimSpace(l.desc), Cantidad: cant, Precio: precio, Total: total,
					})
				}
				pila = pila[:len(pila)-1]
			} else {
				aplicar(nombre, strings.TrimSpace(texto.String()))
			}
		}
	}

	// El total a veces solo existe como suma de renglones.
	if ex.Total == 0 {
		for _, l := range ex.Lineas {
			ex.Total += l.Total
		}
		if ex.Total > 0 {
			ex.anota("total", "sumado de los renglones")
		}
	}
	ex.faltanPorLeer()
	return ex, lecturaErr
}

// faltanPorLeer anota lo que NO se pudo leer y devuelve siempre nil (para que
// la llamada `return ex, ex.faltanPorLeer()` de arriba lea como «hubo un error,
// pero esto es lo que sí se leyó»).
//
// Lo que falta se informa como TAREA, no como error: la pantalla lo muestra
// junto a lo leído para que la persona sepa qué tiene que completar a mano.
func (e *extractoDocumento) faltanPorLeer() error {
	if e.RIF == "" {
		e.falta("RIF del proveedor")
	}
	if e.Proveedor == "" {
		e.falta("razón social")
	}
	if e.Numero == "" {
		e.falta("número de factura")
	}
	if e.Control == "" {
		e.falta("número de control")
	}
	if e.Fecha == "" {
		e.falta("fecha")
	}
	if e.Total == 0 {
		e.falta("total")
	}
	if len(e.Lineas) == 0 {
		e.falta("renglones de productos")
	}
	return nil
}

// inboxTotalClave nombra los totales que SÍ son el total del documento, y deja
// fuera los que son de un renglón o de una base imponible («totaliva»,
// «totaldescuento», «subtotal», «baseImponible»), que arruinarían la propuesta
// si se tomaran por el total.
func inboxTotalClave(nombre string) bool {
	switch nombre {
	case "total", "totalgeneral", "grantotal", "totalapagar", "montototal",
		"importetotal", "totaldocumento", "totalfactura", "totalneto":
		return true
	}
	return false
}

// inboxTipoDocumento clasifica el texto de una etiqueta de tipo.
func inboxTipoDocumento(v string) string {
	n := normalizaXML(v)
	switch {
	case strings.Contains(n, "nota") && strings.Contains(n, "entrega"):
		return CompraNotaEntrega
	case strings.Contains(n, "orden"):
		return CompraOrden
	}
	return CompraFactura
}

// extraerDocumento es la puerta única de extracción por formato.
//
// Un PDF o una imagen NO se leen aquí: se devuelven marcados como `pendiente_ocr`
// para que la bandeja los muestre como tal en vez de fingir que tienen datos.
func extraerDocumento(formato string, contenido []byte) (extractoDocumento, error) {
	if formato == "xml" {
		return extraerXML(contenido)
	}
	ex := extractoDocumento{
		Formato: formato, Extraccion: "pendiente_ocr", Kind: CompraFactura,
	}
	ex.falta("lectura automática (OCR) — todavía no implementada: cargue los datos a mano")
	return ex, nil
}

// inboxClaveRenglon resume un renglón para mostrarlo en la pantalla.
func (l extractoLinea) inboxClaveRenglon() string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(l.Descripcion))
	if l.Cantidad > 0 {
		b.WriteString(" × " + strconv.FormatFloat(l.Cantidad, 'f', -1, 64))
	}
	return strings.TrimSpace(b.String())
}
