package web

// Pruebas de representación de las pantallas de la recepción de documentos.
//
// POR QUÉ: el JavaScript vive dentro de las plantillas, y un valor que
// html/template no sepa meter en contexto JS sale como «ZgotmplZ» —un marcador
// que NO es un error de ejecución: la página se pinta y el script se rompe en
// el navegador, donde nadie ve el motivo. Ya pasó una vez con el <style> del
// manual del programador, así que aquí se comprueba explícitamente.
//
// Se representan los DOS estados del formulario de compra: con documento
// recibido (la propuesta) y sin él (el caso normal de todos los días), porque
// la diferencia entre los dos es justo lo que no debe romperse: si la plantilla
// con datos propios fallara, la pantalla de carga de compras dejaría de abrir
// para todo el mundo.

import (
	"html/template"
	"strings"
	"testing"

	"erpgow"
)

// TestLaPantallaDeLaBandejaSeRepresenta cubre la tabla con los tres estados y
// su script.
func TestLaPantallaDeLaBandejaSeRepresenta(t *testing.T) {
	tpl, err := template.New("purchase_inbox.html").Funcs(templateFuncs(nil, nil)).
		ParseFS(erpgow.Templates(), "templates/purchase_inbox.html")
	if err != nil {
		t.Fatalf("la plantilla no se puede cargar: %v", err)
	}
	if _, err := tpl.ParseFS(erpgow.Templates(), "templates/layout.html"); err != nil {
		t.Fatalf("la página no convive con layout.html: %v", err)
	}

	filas := []inboxFila{
		{ID: 5, OrigName: "factura.xml", Formato: "xml", Extrac: "xml", Kind: CompraNotaEntrega,
			RIF: "J-30123456-7", Prov: "DISTRIBUIDORA EL PUENTE, C.A.", Numero: "00-0000451",
			Control: "00-0000451-1", Fecha: "2026-10-01", Total: 100, Estado: "leido",
			Creado: "2026-10-28 09:15"},
		{ID: 6, OrigName: "foto.png", Formato: "imagen", Extrac: "pendiente_ocr", Kind: CompraFactura,
			Notas: "OCR pendiente: cargue los datos a mano", Estado: "cargado", Linked: 77,
			Creado: "2026-10-28 10:00"},
		{ID: 7, OrigName: "duplicado.pdf", Formato: "pdf", Extrac: "pendiente_ocr", Kind: CompraOrden,
			Estado: "descartado", Creado: "2026-10-28 11:00"},
	}
	datos := map[string]interface{}{
		"Title": "Documentos recibidos", "Filas": filas, "Estado": "leido",
		"Contadores": map[string]int{"leido": 1, "cargado": 1, "descartado": 1, "todos": 3},
	}

	cuerpo := representar(t, tpl, "content", datos)
	if strings.Contains(cuerpo, "ZgotmplZ") {
		t.Error("la plantilla dejó el marcador ZgotmplZ: algo no se pudo representar en su contexto")
	}
	for _, esperado := range []string{
		"Documentos recibidos",           // título
		"/purchase/add?inbox_id=5",       // el botón que propone la compra
		"/purchase/inbox?estado=cargado", // pestañas
		"J-30123456-7", "00-0000451",     // lo que se leyó
		"Nota de entrega",            // kindLabel en la tabla
		"XML leído", "OCR pendiente", // las dos formas de lectura
		"OCR pendiente: cargue los datos a mano", // la nota del PDF
		"Compra #77",                             // el enlace a la compra ya cargada
		"100.00",                                 // el total formateado
	} {
		if !strings.Contains(cuerpo, esperado) {
			t.Errorf("la bandeja no muestra %q", esperado)
		}
	}

	js := representar(t, tpl, "pagejs", datos)
	if strings.Contains(js, "ZgotmplZ") {
		t.Error("el script de la bandeja dejó el marcador ZgotmplZ")
	}
	for _, esperado := range []string{"/purchase/inbox", "/discard", "pintarPropuesta"} {
		if !strings.Contains(js, esperado) {
			t.Errorf("el script de la bandeja no llama a %q", esperado)
		}
	}
}

// TestElFormularioDeCompraRepresentaLaPropuestaYElCasoNormal son las dos caras
// de la misma plantilla: con documento recibido y sin él.
func TestElFormularioDeCompraRepresentaLaPropuestaYElCasoNormal(t *testing.T) {
	base := func() map[string]interface{} {
		return map[string]interface{}{
			"Title": "Nueva Compra", "ControlNo": "", "DocumentKind": CompraFactura,
			"Methods": []map[string]interface{}{},
		}
	}

	conDocumento := base()
	conDocumento["InboxID"] = int64(7)
	conDocumento["InboxArchivo"] = "factura octubre.xml"
	conDocumento["InboxProveedor"] = "DISTRIBUIDORA EL PUENTE, C.A."
	conDocumento["InboxCampos"] = "RIF, número de factura"
	conDocumento["InboxFaltan"] = "número de control"
	conDocumento["InboxLineas"] = []extractoLinea{
		{Descripcion: "ARROZ GANDUL 1KG & AZÚCAR", Cantidad: 12, Precio: 250},
	}

	casos := []struct {
		nombre     string
		datos      map[string]interface{}
		esperado   []string
		ausente    []string
		esperadoJS []string
		ausenteJS  []string
	}{
		{
			nombre: "con documento recibido",
			datos:  conDocumento,
			esperado: []string{
				`name="inbox_id"`, "value=\"7\"",
				"Documento recibido", "factura octubre.xml", "DISTRIBUIDORA EL PUENTE, C.A.",
				"Se leyó del documento", "Queda por confirmar", "número de control",
			},
			ausente:    []string{"ZgotmplZ"},
			esperadoJS: []string{"var inboxLineas", "ARROZ GANDUL 1KG", "cargarPropuestaInbox"},
			ausenteJS:  []string{"ZgotmplZ"},
		},
		{
			nombre:     "carga normal, sin documento",
			datos:      base(),
			esperado:   []string{"Nueva Compra"},
			ausente:    []string{`name="inbox_id"`, "Documento recibido", "ZgotmplZ"},
			esperadoJS: []string{"var inboxLineas = []"},
			ausenteJS:  []string{"ZgotmplZ"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tpl, err := template.New("purchase_form.html").Funcs(templateFuncs(nil, nil)).
				ParseFS(erpgow.Templates(), "templates/purchase_form.html")
			if err != nil {
				t.Fatalf("la plantilla no se puede cargar: %v", err)
			}
			if _, err := tpl.ParseFS(erpgow.Templates(), "templates/layout.html"); err != nil {
				t.Fatalf("la página no convive con layout.html: %v", err)
			}

			cuerpo := representar(t, tpl, "content", c.datos)
			for _, s := range c.esperado {
				if !strings.Contains(cuerpo, s) {
					t.Errorf("el formulario no muestra %q", s)
				}
			}
			for _, s := range c.ausente {
				if strings.Contains(cuerpo, s) {
					t.Errorf("el formulario muestra %q, que no debe aparecer", s)
				}
			}

			// El script va en su propio `define`: ahí vive la propuesta de
			// renglones y ahí es donde un valor mal escapado rompería la página.
			js := representar(t, tpl, "pagejs", c.datos)
			for _, s := range c.esperadoJS {
				if !strings.Contains(js, s) {
					t.Errorf("el script del formulario no contiene %q", s)
				}
			}
			for _, s := range c.ausenteJS {
				if strings.Contains(js, s) {
					t.Errorf("el script del formulario contiene %q", s)
				}
			}
		})
	}
}
