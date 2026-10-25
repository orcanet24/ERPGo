package web

// report_pdf.go — Exportar un informe a PDF, sin dependencias.
//
// POR QUÉ UN PDF ESCRITO A MANO Y NO UNA LIBRERÍA.
//
// `go.mod` no tenía ninguna, y meter `github.com/jung-kurt/gofpdf` o similar
// significa que el instalador tiene que poder bajarla. El ERP se distribuye
// como un ejecutable con MySQL dentro: si el cliente lo instala sin red, una
// dependencia que no está en la máquina es un instalador roto. Y esto es un
// informe tabular —título, rejilla y una línea de totales—, que es un
// subconjunto tan pequeño de PDF que escribirlo son 300 líneas y una
// dependencia menos.
//
// LO QUE SOPORTA Y LO QUE NO.
//
// Soporta: páginas A4, rejilla con encabezado sombreado, texto con acentos y
// eñes (WinAnsi), totales y varias páginas con numeración.
//
// No soporta, y no se va a añadir: imágenes, gráficos, fuentes embebidas,
// enlaces, formularios. Un informe de ERP es una tabla. Si algún día hace falta
// un PDF con gráfico, es un módulo aparte con su biblioteca, no un crecimiento
// de este.
//
// LA FUENTE Y LOS ACENTOS.
//
// Helvetica con codificación WinAnsi (cp1252). Es lo que permite "Cartera de
// clientes por vencer" con eñes y acentos sin embeber una fuente. Lo que no
// cabe —emoji, cirílico, chino— se sustituye por '?' en vez de romperse el
// fichero: un informe con tres signos raritos vale más que un PDF que no abre.

import (
	"bytes"
	"fmt"
	"strings"
)

// A4 en puntos PostScript (72 por pulgada).
const (
	pdfAnchoA4 = 595.28
	pdfAltoA4  = 841.89
	pdfMargen  = 36.0

	pdfFuenteTitulo   = 14
	pdfFuenteCabecera = 8
	pdfFuenteCelda    = 8
	pdfFuenteTotal    = 9

	pdfAltoFila = 14.0
	// pdfAnchoMedio es el ancho aproximado de un carácter de Helvetica. Se usa
	// solo para recortar celdas, y a propósito es conservador: un recorte que se
	// pasa de largo deja un hueco, uno agresivo corta texto que sí cabía.
	pdfAnchoMedio = 5.0
)

// cp1252Raros son los caracteres de cp1252 fuera de Latin-1, con su byte.
var cp1252Raros = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85,
	'†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A,
	'‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92,
	'“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97,
	'˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C,
	'ž': 0x9E, 'Ÿ': 0x9F,
}

// winAnsi convierte una cadena a bytes con la codificación que entiende la
// fuente estándar.
//
// Los caracteres fuera de cp1252 se cambian por '?' en lugar de eliminarse:
// eliminados, dos celdas distintas pueden acabar con el mismo texto y el informe
// miente. Con '?' al menos se ve que ahí había algo.
func winAnsi(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0x20 && r < 0x7F:
			b.WriteRune(r)
		case r >= 0xA0 && r <= 0xFF:
			// cp1252 coincide con Latin-1 en el rango 0xA0-0xFF.
			b.WriteByte(byte(r))
		case r == '\n' || r == '\t':
			b.WriteByte(' ')
		default:
			if alt, ok := cp1252Raros[r]; ok {
				b.WriteByte(alt)
				continue
			}
			b.WriteByte('?')
		}
	}
	return b.String()
}

// pdfEscape escapa los caracteres reservados de una cadena de contenido PDF.
//
// Los paréntesis abren y cierran cadenas de contenido; sin escapar, un título
// como "Ventas (octubre)" deja el PDF sintácticamente roto y el visor lo rechaza
// con un error que no señala el título.
func pdfEscape(s string) string {
	s = winAnsi(s)
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// pdfDoc es el PDF en construcción.
type pdfDoc struct {
	paginas []string // contenido de cada página
	actual  strings.Builder
	ancho   float64
	alto    float64
}

// nuevaPaginaPDF empieza una página nueva.
func (d *pdfDoc) nuevaPagina() {
	if d.actual.Len() > 0 {
		d.paginas = append(d.paginas, d.actual.String())
		d.actual.Reset()
	}
	// La numeración va en el pie, y hay que dibujarla DESPUÉS del contenido de
	// la página, porque en PDF lo que se escribe después tapa lo anterior.
	fmt.Fprintf(&d.actual,
		"BT /F1 8 Tf 0.55 0.55 0.55 rg 1 0 0 1 %.0f %.0f Tm (%s) Tj ET\n",
		pdfMargen, pdfMargen-14, pdfEscape(fmt.Sprintf("Pagina %d", len(d.paginas)+1)))
}

// tabla dibuja la rejilla de un informe.
func (d *pdfDoc) tabla(titulo, subtitulo string, encabezados []string, filas [][]string, totales []string) {
	if len(encabezados) == 0 {
		return
	}
	anchoTotal := d.ancho - 2*pdfMargen

	// La primera columna suele ser el nombre o la etiqueta, y necesita más
	// ancho que las de cifras. El resto se reparte a partes iguales.
	anchoPrimera := anchoTotal * 0.28
	if len(encabezados) == 1 {
		anchoPrimera = anchoTotal
	}
	resto := (anchoTotal - anchoPrimera) / float64(len(encabezados)-1)
	anchos := make([]float64, len(encabezados))
	anchos[0] = anchoPrimera
	for i := 1; i < len(encabezados); i++ {
		anchos[i] = resto
	}

	y := d.alto - pdfMargen

	fmt.Fprintf(&d.actual, "BT /F2 %d Tf 0.1 0.1 0.1 rg 1 0 0 1 %.0f %.0f Tm (%s) Tj ET\n",
		pdfFuenteTitulo, pdfMargen, y, pdfEscape(titulo))
	y -= 20

	if subtitulo != "" {
		fmt.Fprintf(&d.actual, "BT /F1 9 Tf 0.35 0.35 0.35 rg 1 0 0 1 %.0f %.0f Tm (%s) Tj ET\n",
			pdfMargen, y, pdfEscape(subtitulo))
		y -= 16
	}
	y -= 6

	// Encabezado sombreado
	fmt.Fprintf(&d.actual, "0.93 0.93 0.93 rg %.0f %.0f %.0f %.0f re f\n",
		pdfMargen, y-pdfAltoFila, anchoTotal, pdfAltoFila)
	d.fila(encabezados, anchos, y, pdfFuenteCabecera, true)
	y -= pdfAltoFila

	for _, fila := range filas {
		if y < pdfMargen+30 {
			d.nuevaPagina()
			y = d.alto - pdfMargen
			// Al saltar de página hay que repetir el encabezado, o la segunda
			// página es una lista de números sin decir de qué.
			y -= pdfAltoFila
			d.fila(encabezados, anchos, y, pdfFuenteCabecera, true)
			y -= pdfAltoFila
		}
		y -= pdfAltoFila
		d.fila(fila, anchos, y, pdfFuenteCelda, false)
	}

	if len(totales) > 0 {
		if y < pdfMargen+50 {
			d.nuevaPagina()
			y = d.alto - pdfMargen
		}
		y -= pdfAltoFila + 4
		fmt.Fprintf(&d.actual, "0.85 0.85 0.85 rg %.0f %.0f %.0f %.0f re f\n",
			pdfMargen, y-pdfAltoFila, anchoTotal, pdfAltoFila)
		d.fila(totales, anchos, y, pdfFuenteTotal, true)
	}
}

// fila escribe una fila de celdas, alineando a la derecha lo que parece número.
func (d *pdfDoc) fila(celdas []string, anchos []float64, y float64, fuente int, negrita bool) {
	font := "F1"
	if negrita {
		font = "F2"
	}
	x := pdfMargen
	for i, celda := range celdas {
		if i >= len(anchos) {
			break
		}
		ancho := anchos[i]
		texto := celda
		maximo := int(ancho/(pdfAnchoMedio*(float64(fuente)/10.0))) - 1
		if maximo > 1 && len(texto) > maximo {
			texto = texto[:maximo-1] + "."
		}
		xTexto := x + 3
		if i > 0 && pdfEsNumero(celda) {
			// Alineación a la derecha: el ancho del texto se estima a partir del
			// número de caracteres y del tamaño de fuente.
			anchoTexto := float64(len(texto)) * (float64(fuente) * pdfAnchoMedio / 10.0)
			xTexto = x + ancho - anchoTexto - 3
		}
		fmt.Fprintf(&d.actual, "BT /%s %d Tf 0 0 0 rg 1 0 0 1 %.0f %.0f Tm (%s) Tj ET\n",
			font, fuente, xTexto, y+3, pdfEscape(texto))
		x += ancho
		// Separador vertical entre celdas
		fmt.Fprintf(&d.actual, "0.8 0.8 0.8 RG 0.4 w %.0f %.0f m %.0f %.0f l S\n",
			x, y-2, x, y+pdfAltoFila-4)
	}
}

// pdfEsNumero decide si una celda se alinea a la derecha.
//
// La decisión se toma por el FORMATO, no por el tipo: en este punto los valores
// ya son texto (vienen de `formatReportValue`), y un total como "1.234,56" o
// "12%" tiene que alinearse a la derecha aunque no sea parseable como número.
func pdfEsNumero(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	hayDigito := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hayDigito = true
		case r == '.' || r == ',' || r == '-' || r == '+' || r == '%' || r == ' ' || r == '$':
			// separador
		default:
			return false
		}
	}
	return hayDigito
}

// bytes ensambla el PDF completo.
//
// El xref es lo único delicado de escribir a mano: guarda la posición EN BYTES de
// cada objeto, y un byte de más o de menos hace que el lector lo rechace. Por eso
// los offsets se calculan escribiendo y contando sobre el mismo buffer, en una
// sola pasada: calcularlos aparte y concatenar después es la forma clásica de
// generar un PDF que "casi" funciona y no abre.
func (d *pdfDoc) bytes() []byte {
	if d.actual.Len() > 0 {
		d.paginas = append(d.paginas, d.actual.String())
		d.actual.Reset()
	}
	if len(d.paginas) == 0 {
		d.nuevaPagina()
		d.paginas = append(d.paginas, d.actual.String())
		d.actual.Reset()
	}

	const numPagina = 3
	const numContenido = 4
	// Cada página usa DOS números (página y contenido), así que las fuentes
	// empiezan DESPUÉS de las 2·n páginas, no después de n.
	//
	// Con `numContenido + len(paginas)` las fuentes caían EN MEDIO de los
	// objetos de página en cuanto había más de tres páginas: la fuente negrita
	// pisaba la página 4, el PDF se generaba sin error, se abría... y el
	// documento tenía menos páginas de las que tenía, con la mitad vacías.
	numF1 := numContenido + 2*len(d.paginas)
	numF2 := numF1 + 1

	kids := make([]string, len(d.paginas))
	for i := range d.paginas {
		kids[i] = fmt.Sprintf("%d 0 R", numPagina+2*i)
	}

	objetos := make(map[int]string, numF2)
	objetos[1] = "<< /Type /Catalog /Pages 2 0 R >>"
	objetos[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>",
		strings.Join(kids, " "), len(d.paginas))
	// Cada página ocupa DOS números CONSECUTIVOS: la página y su contenido. El
	// paso es 2, no 1.
	//
	// Con paso 1, el objeto de la página 2 y el del contenido de la página 1
	// salen el MISMO número, uno se pisa al escribir el mapa y el PDF tiene menos
	// páginas de las que tiene. La colisión es silenciosa: el fichero se genera
	// sin error y se abre, solo que con la mitad del contenido. Lo detectó
	// TestPDFMultipaginaSaltaPaginaConEncabezado, que es la razón de existir.
	for i, contenido := range d.paginas {
		objetos[numPagina+2*i] = fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] "+
				"/Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> >> /Contents %d 0 R >>",
			d.ancho, d.alto, numF1, numF2, numContenido+2*i)
		// El /Length es el numero de bytes ENTRE "stream\n" y "endstream".
		//
		// Se mide el flujo que se escribe, no el string de Go: anadir un salto
		// de linea delante de "endstream" y declarar el tamaño del string
		// anterior deja el /Length corto en uno, y un lector estricto rechaza
		// el fichero entero. No es un detalle cosmetico: asi se detecto.
		objetos[numContenido+2*i] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream",
			len(contenido)+1, contenido)
	}
	objetos[numF1] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"
	objetos[numF2] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>"

	total := numF2
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	offsets := make([]int, total+1)
	for i := 1; i <= total; i++ {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i, objetos[i])
	}

	inicioXref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", total+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= total; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		total+1, inicioXref)
	return buf.Bytes()
}

// ReportPDF genera el PDF de un informe.
//
// `subtitulo` es lo que va bajo el título (el período, la empresa). Se pasa
// aparte porque casi siempre es lo mismo, y calcularlo aquí ataría el PDF a un
// tipo concreto de informe.
func ReportPDF(run *ReportRun, subtitulo string) ([]byte, error) {
	if run == nil || len(run.Headers) == 0 {
		return nil, fmt.Errorf("el informe no tiene columnas")
	}
	d := &pdfDoc{ancho: pdfAnchoA4, alto: pdfAltoA4}

	filas := make([][]string, 0, len(run.Rows))
	for _, fila := range run.Rows {
		celdas := make([]string, len(fila))
		for i, v := range fila {
			celdas[i] = formatReportValue(v)
		}
		filas = append(filas, celdas)
	}

	// `Totals` es un mapa CLAVE→valor, no una fila ordenada. La posición de cada
	// total la dan `Keys`, que sí está en el mismo orden que los encabezados. Sin
	// esa indirección los totales acaban en columnas equivocadas, y en un informe
	// de dinero eso es un descuadre que no se ve hasta que alguien lo cuadra.
	var totales []string
	if len(run.Totals) > 0 {
		totales = make([]string, len(run.Headers))
		totales[0] = "TOTALES"
		idxTotal := make(map[string]int, len(run.Keys))
		for i, k := range run.Keys {
			idxTotal[k] = i
		}
		for k, v := range run.Totals {
			if i, ok := idxTotal[k]; ok && i < len(totales) {
				totales[i] = formatReportValue(v)
			}
		}
	}

	d.tabla(run.Title, subtitulo, run.Headers, filas, totales)
	return d.bytes(), nil
}
