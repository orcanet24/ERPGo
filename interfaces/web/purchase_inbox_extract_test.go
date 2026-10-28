package web

// Pruebas de la extracción de documentos de proveedor.
//
// Aquí se comprueba la única cosa que este código promete: LEER sin inventar.
// Un campo que se lee tiene que salir con su valor, un campo que no se lee
// tiene que salir en `faltan` y no convertirse en un cero silencioso, y un
// archivo que no es XML tiene que salir marcado como pendiente de OCR en vez de
// fingir que se le entendió algo.
//
// Son funciones puras: no hay base, ni servidor, ni red. La parte con base de
// datos —el candado único del archivo— está en purchase_inbox_db_test.go.

import (
	"strings"
	"testing"
)

// facturitaVenezolana es un XML del formato en que suele salir una imprenta
// venezolana: bloques Emisor/Receptor, totales aparte y renglones en un
// contenedor `Detalles`.
const facturitaVenezolana = `<?xml version="1.0" encoding="UTF-8"?>
<FacturaElectronica>
  <Emisor>
    <Rif>J-30123456-7</Rif>
    <RazonSocial>DISTRIBUIDORA EL PUENTE, C.A.</RazonSocial>
  </Emisor>
  <NumeroFactura>00-0000451</NumeroFactura>
  <NumeroControl>00-0000451-12345678</NumeroControl>
  <Fecha>01/10/2026</Fecha>
  <Moneda>VES</Moneda>
  <Detalles>
    <Item>
      <Descripcion>ARROZ GANDUL 1KG</Descripcion>
      <Cantidad>12</Cantidad>
      <PrecioUnitario>250,00</PrecioUnitario>
    </Item>
    <Item>
      <Descripcion>AZUCAR BLANCA 1KG</Descripcion>
      <Cantidad>6</Cantidad>
      <PrecioUnitario>120,00</PrecioUnitario>
    </Item>
  </Detalles>
  <Totales>
    <BaseImponible>3960,00</BaseImponible>
    <IVA>633,60</IVA>
    <Total>4593,60</Total>
  </Totales>
</FacturaElectronica>`

func TestLeeUnaFacturaVenezolanaEntera(t *testing.T) {
	ex, err := extraerXML([]byte(facturitaVenezolana))
	if err != nil {
		t.Fatalf("una factura bien formada no debe dar error: %v", err)
	}

	if ex.RIF != "J-30123456-7" {
		t.Errorf("RIF: quería J-30123456-7, salió %q", ex.RIF)
	}
	if ex.Proveedor != "DISTRIBUIDORA EL PUENTE, C.A." {
		t.Errorf("razón social: salió %q", ex.Proveedor)
	}
	if ex.Numero != "00-0000451" {
		t.Errorf("número de factura: salió %q", ex.Numero)
	}
	if ex.Control != "00-0000451-12345678" {
		t.Errorf("número de control: salió %q", ex.Control)
	}
	if ex.Fecha != "2026-10-01" {
		t.Errorf("fecha: quería 2026-10-01 (normalizada), salió %q", ex.Fecha)
	}
	if ex.Moneda != "VES" {
		t.Errorf("moneda: salió %q", ex.Moneda)
	}
	if ex.Total != 4593.60 {
		t.Errorf("total: quería 4593.60, salió %.2f", ex.Total)
	}
	if ex.IVA != 633.60 {
		t.Errorf("IVA: quería 633.60, salió %.2f", ex.IVA)
	}
	if ex.Kind != CompraFactura {
		t.Errorf("tipo: quería factura, salió %q", ex.Kind)
	}

	if len(ex.Lineas) != 2 {
		t.Fatalf("renglones: quería 2, salieron %d", len(ex.Lineas))
	}
	l := ex.Lineas[0]
	if l.Descripcion != "ARROZ GANDUL 1KG" || l.Cantidad != 12 || l.Precio != 250 {
		t.Errorf("renglón 1: salió %+v", l)
	}
	if l.Total == 0 {
		t.Errorf("el renglón sin `importe` debe totalizar cantidad × precio")
	}
	if len(ex.Faltan) != 0 {
		t.Errorf("una factura completa no debe reportar campos faltantes: %v", ex.Faltan)
	}
}

// TestElTotalNoSePrestaDeUnaBaseImponible es la trampa clásica: `BaseImponible`
// y `Total` comparten la palabra «total» en muchos esquemas, y declarar el
// primero como total dejaría la factura con el IVA sin cobrar en la propuesta.
func TestElTotalNoSePrestaDeUnaBaseImponible(t *testing.T) {
	xml := `<Factura>
		<Emisor><Rif>J-1</Rif></Emisor>
		<BaseImponible>100</BaseImponible>
		<TotalFactura>116</TotalFactura>
		<IVA>16</IVA>
	</Factura>`
	ex, _ := extraerXML([]byte(xml))
	if ex.Total != 116 {
		t.Errorf("tenía que coger TotalFactura=116, cogió %.2f", ex.Total)
	}
	if ex.IVA != 16 {
		t.Errorf("IVA: quería 16, salió %.2f", ex.IVA)
	}
}

// TestLaNotaDeEntregaNoSeDisfrazaDeFactura es el corazón de la clasificación:
// el tipo del documento viaja con la propuesta para que la compra nazca con la
// etiqueta correcta y no entre al Libro de Compras como crédito fiscal.
func TestLaNotaDeEntregaNoSeDisfrazaDeFactura(t *testing.T) {
	casos := map[string]string{
		`<Factura><TipoDocumento>Nota de Entrega</TipoDocumento></Factura>`: CompraNotaEntrega,
		`<Documento><TipoDocumento>nota entrega</TipoDocumento></Documento>`: CompraNotaEntrega,
		`<Factura><TipoDocumento>Orden de Compra</TipoDocumento></Factura>`: CompraOrden,
		`<Factura><TipoDocumento>01</TipoDocumento></Factura>`:              CompraFactura,
		`<Factura></Factura>`:                                              CompraFactura,
	}
	for entrada, esperado := range casos {
		ex, _ := extraerXML([]byte(entrada))
		if ex.Kind != esperado {
			t.Errorf("%s → quería %q, salió %q", entrada, esperado, ex.Kind)
		}
	}
}

// TestRenglonesAnidados cubre el envoltorio: `<Detalles>` también es un nombre
// de renglón, y si se tratara como uno solo se propondría UNA fila con los
// datos de la primera en vez de las dos que trae la factura.
func TestRenglonesAnidados(t *testing.T) {
	xml := `<Factura>
		<Detalles>
			<Item><Descripcion>UNO</Descripcion><Cantidad>1</Cantidad><PrecioUnitario>10</PrecioUnitario></Item>
			<Item><Descripcion>DOS</Descripcion><Cantidad>2</Cantidad><PrecioUnitario>20</PrecioUnitario></Item>
		</Detalles>
	</Factura>`
	ex, _ := extraerXML([]byte(xml))
	if len(ex.Lineas) != 2 {
		t.Fatalf("quería 2 renglones, salieron %d: %+v", len(ex.Lineas), ex.Lineas)
	}
	if ex.Lineas[1].Descripcion != "DOS" {
		t.Errorf("el segundo renglón salió como %q", ex.Lineas[1].Descripcion)
	}
}

// TestUnXmlCortadoSeLeeHastaDondeDio: un archivo a medias no se descarta, se lee
// lo que se pudo y el fallo se devuelve aparte para que la pantalla lo avise.
func TestUnXmlCortadoSeLeeHastaDondeDio(t *testing.T) {
	cortado := facturitaVenezolana[:strings.LastIndex(facturitaVenezolana, "</Totales>")]
	ex, err := extraerXML([]byte(cortado))
	if err == nil {
		t.Fatal("un XML sin cerrar tenía que devolver el error de lectura")
	}
	if ex.Numero != "00-0000451" {
		t.Errorf("lo leído antes del corte se perdió: número %q", ex.Numero)
	}
	if len(ex.Lineas) != 2 {
		t.Errorf("los renglones leídos antes del corte se perdieron: %d", len(ex.Lineas))
	}
	if ex.Total == 0 {
		t.Error("el total estaba antes del corte y no se leyó")
	}
}

// TestLoQueNoSeLeeSeReporta es la regla del humo aplicada a los faltantes:
// nada de «total 0» ni «rif vacío» sin decirlo. Lo que falta es una TAREA.
func TestLoQueNoSeLeeSeReporta(t *testing.T) {
	ex, _ := extraerXML([]byte(`<Factura><NumeroFactura>55</NumeroFactura></Factura>`))
	unido := strings.Join(ex.Faltan, " | ")
	for _, esperado := range []string{"RIF", "razón social", "control", "fecha", "total", "renglones"} {
		if !strings.Contains(unido, esperado) {
			t.Errorf("faltaba anunciar %q; los faltantes son: %v", esperado, ex.Faltan)
		}
	}
	if ex.Numero != "55" {
		t.Errorf("lo poco que sí se leyó debe conservarse: número %q", ex.Numero)
	}
}

// TestElOcrTodaviaNoAdivina: PDF e imagen llegan, se guardan, se clasifican y se
// marcan pendientes. Si alguna vez se inventara un dato de un PDF, sería peor
// que no leerlo.
func TestElOcrTodaviaNoAdivina(t *testing.T) {
	for _, formato := range []string{"pdf", "imagen"} {
		ex, err := extraerDocumento(formato, []byte("bytes"))
		if err != nil {
			t.Fatalf("%s no debe dar error: %v", formato, err)
		}
		if ex.Extraccion != "pendiente_ocr" {
			t.Errorf("%s: quería pendiente_ocr, salió %q", formato, ex.Extraccion)
		}
		if len(ex.Faltan) == 0 {
			t.Errorf("%s sin lectura tiene que avisar qué falta", formato)
		}
		if ex.Kind != CompraFactura {
			t.Errorf("%s: el tipo por defecto tiene que ser factura, salió %q", formato, ex.Kind)
		}
	}
}

func TestInboxFormato(t *testing.T) {
	casos := []struct {
		nombre, mime, esperado string
	}{
		{"factura.xml", "", "xml"},
		{"FACTURA.XML", "application/xml", "xml"},
		{"nota.pdf", "application/pdf", "pdf"},
		{"foto.jpg", "", "imagen"},
		{"foto.jpeg", "image/jpeg", "imagen"},
		{"captura.webp", "", "imagen"},
		{"resumen.txt", "text/plain", ""},
		{"peligroso.php", "text/html", ""},
		{"sinextension", "", ""},
	}
	for _, c := range casos {
		if got := inboxFormato(c.nombre, c.mime); got != c.esperado {
			t.Errorf("inboxFormato(%q, %q) = %q, quería %q", c.nombre, c.mime, got, c.esperado)
		}
	}
}

// TestInboxDinero prueba los DOS formatos que conviven en Venezuela: el punto de
// miles con coma decimal y el revés. El desempate es el último separador, que es
// lo único que no depende de la costumbre de quien escribió el archivo.
func TestInboxDinero(t *testing.T) {
	casos := []struct{ entrada string; esperado float64 }{
		{"1.234,56", 1234.56},
		{"1,234.56", 1234.56},
		{"4593.60", 4593.60},
		{"4593,60", 4593.60},
		{"Bs 120,00", 120},
		{"1234", 1234},
		{"", 0},
		{"no es número", 0},
	}
	for _, c := range casos {
		if got := inboxDinero(c.entrada); got != c.esperado {
			t.Errorf("inboxDinero(%q) = %v, quería %v", c.entrada, got, c.esperado)
		}
	}
}

func TestInboxFecha(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"01/10/2026", "2026-10-01"},
		{"2026-10-01", "2026-10-01"},
		{"2026-10-01T09:30:00", "2026-10-01"},
		{"ayer", "ayer"}, // no se adivina: se deja como vino para que la corrija el datepicker
		{"", ""},
	}
	for _, c := range casos {
		if got := inboxFecha(c.entrada); got != c.esperado {
			t.Errorf("inboxFecha(%q) = %q, quería %q", c.entrada, got, c.esperado)
		}
	}
}
