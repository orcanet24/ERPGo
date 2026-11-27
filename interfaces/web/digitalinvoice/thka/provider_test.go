package thka

// Pruebas del proveedor The Factory HKA.
//
// Lo que se demuestra y por qué importa:
//
//   - Que el documento electrónico sale con la forma EXACTA que pide la
//     referencia: cabecera, comprador, renglones y totales que cuadran entre sí.
//     THKA no calcula: imprime lo que recibe y valida que las sumas coincidan,
//     así que un total mal armado es una factura rechazada.
//   - Que el token se cachea y se renueva con 401 (dura doce horas; sin caché
//     cada documento pediría uno y sin renovación el ERP dejaría de emitir).
//   - Que los rechazos del proveedor —que llegan con HTTP 200 y un `codigo`
//     distinto de 200— no se confunden con un éxito.
//   - Que lo que THKA no puede representar (dólares, nota sin datos de la
//     factura afectada) se detiene ANTES de llamar.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	di "erpgo/interfaces/web/digitalinvoice"
)

// olvidarTokens limpia la caché de JWT entre pruebas (vive a nivel de paquete).
func olvidarTokens(t *testing.T) {
	t.Helper()
	tokensMu.Lock()
	tokensPorCredencial = map[string]tokenEnMemoria{}
	tokensMu.Unlock()
}

// pdfDePrueba es un PDF mínimo en base64, con el prefijo que usa la búsqueda del
// adaptador ("JVBERi" = "%PDF").
func pdfDePrueba() string {
	contenido := "%PDF-1.4\n% ERPGo prueba de archivo fiscal\n" + strings.Repeat("% relleno\n", 20) + "%%EOF\n"
	return base64.StdEncoding.EncodeToString([]byte(contenido))
}

func configDePrueba(baseURL string) di.ProviderConfig {
	return di.ProviderConfig{
		Code:         codigoProveedor,
		BaseURL:      baseURL,
		UserName:     "usuario.api",
		UserPassword: "clave.api",
		NumeroSerie:  "A",
		SucursalID:   "0001",
	}
}

func documentoDePrueba() *di.DatosDocumento {
	return &di.DatosDocumento{
		Tipo:         di.DocFactura,
		Numero:       123,
		FechaEmision: time.Date(2026, 11, 27, 10, 30, 0, 0, time.Local),
		FechaTasa:    time.Date(2026, 11, 27, 0, 0, 0, 0, time.Local),
		Moneda:       "VES",
		TasaCambio:   36.5,
		Total:        2135,
		TipoPago:     "Inmediato",
		Cliente: di.DatosCliente{
			Nombre:         "Cliente Ejemplo C.A.",
			FiscalRegistry: "J-12345678-9",
			Direccion:      "Av. Principal, Caracas",
			Telefono:       "04141234567",
			Email:          "facturacion@example.com",
			TipoDocFiscal:  "J",
		},
		Detalles: []di.DetalleItem{
			{Codigo: "SERV-001", Nombre: "Mantenimiento", Descripcion: "Mantenimiento mensual",
				Cantidad: 1, PrecioUnitario: 1000, DescuentoPct: 5, PorcentajeIVA: 16,
				CodigoImpuesto: "G", TipoProducto: 2},
			{Codigo: "LIBRO", Nombre: "Libro escolar", Descripcion: "Libro escolar",
				Cantidad: 2, PrecioUnitario: 500, PorcentajeIVA: 0, EsExento: true,
				CodigoImpuesto: "E", TipoProducto: 1},
		},
	}
}

// servidorFake monta una imprenta THKA falsa que responde a autenticación,
// emisión y descarga del PDF.
type servidorFake struct {
	*httptest.Server
	mu              sync.Mutex
	authPedidas     int
	emisiones       []map[string]interface{}
	descargas       int
	ultimoAuth      map[string]string
	fallarPrimera   bool // responde 401 en la primera emisión
	emisionVeces    int
	respuestaHTTP   int
	respuestaCuerpo string
}

func nuevoServidorFake(t *testing.T) *servidorFake {
	t.Helper()
	f := &servidorFake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/api/Autenticacion":
			f.mu.Lock()
			f.authPedidas++
			var cred struct {
				Usuario string `json:"usuario"`
				Clave   string `json:"clave"`
			}
			_ = json.Unmarshal(cuerpo, &cred)
			f.ultimoAuth = map[string]string{"usuario": cred.Usuario, "clave": cred.Clave}
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"resultado":"token-de-prueba"}`))
		case "/api/Emision":
			f.mu.Lock()
			f.emisionVeces++
			intento := f.emisionVeces
			if f.fallarPrimera && intento == 1 {
				f.mu.Unlock()
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"codigo":"401","mensaje":"No autorizado"}`))
				return
			}
			var pedido map[string]interface{}
			_ = json.Unmarshal(cuerpo, &pedido)
			f.emisiones = append(f.emisiones, pedido)
			httpStatus := f.respuestaHTTP
			respuesta := f.respuestaCuerpo
			f.mu.Unlock()
			if httpStatus == 0 {
				httpStatus = http.StatusOK
			}
			if respuesta == "" {
				respuesta = `{"codigo":"200","mensaje":"Documento procesado exitosamente",` +
					`"validaciones":[],"resultado":{"imprentaDigital":"TFHKA","autorizado":"SI",` +
					`"serie":"A","tipoDocumento":"01","numeroDocumento":"123",` +
					`"numeroControl":"00-00000056","fechaAsignacion":"27/11/2026"}}`
			}
			w.WriteHeader(httpStatus)
			_, _ = w.Write([]byte(respuesta))
		case "/api/DescargaArchivo":
			f.mu.Lock()
			f.descargas++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"archivo":"` + pdfDePrueba() + `"}`))
		default:
			t.Errorf("ruta inesperada en la imprenta falsa: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// documentoDe devuelve la parte "Documentoelectronico" del primer pedido.
func (f *servidorFake) documentoDe(t *testing.T) map[string]interface{} {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.emisiones) == 0 {
		t.Fatal("no se envió ningún documento a la imprenta")
	}
	sobre, ok := f.emisiones[0]["Documentoelectronico"].(map[string]interface{})
	if !ok {
		t.Fatalf("el pedido no lleva Documentoelectronico: %#v", f.emisiones[0])
	}
	return sobre
}

// TestEmiteFacturaConLaEstructuraDeTHKA fija el contrato del documento.
func TestEmiteFacturaConLaEstructuraDeTHKA(t *testing.T) {
	olvidarTokens(t)
	servidor := nuevoServidorFake(t)

	proveedor, err := NuevoProveedor(configDePrueba(servidor.URL + "/"))
	if err != nil {
		t.Fatalf("no se pudo crear el proveedor: %v", err)
	}
	resultado, err := proveedor.Emitir(context.Background(), documentoDePrueba())
	if err != nil {
		t.Fatalf("la emisión falló: %v", err)
	}

	// La respuesta: control textual y PDF archivado.
	if resultado.NroControlTexto != "00-00000056" {
		t.Errorf("control = %q, se esperaba 00-00000056", resultado.NroControlTexto)
	}
	if resultado.NroControl != 56 {
		t.Errorf("parte numérica del control = %d", resultado.NroControl)
	}
	if resultado.PDFBase64 == "" {
		t.Error("el PDF que devuelve la imprenta debía quedar en base64 para archivarse")
	}
	if resultado.Status != "Approved" || !resultado.EsAsignado {
		t.Errorf("resultado inesperado: %+v", resultado)
	}

	// Las credenciales viajan como las pide el API.
	servidor.mu.Lock()
	auth := servidor.ultimoAuth
	servidor.mu.Unlock()
	if auth["usuario"] != "usuario.api" || auth["clave"] != "clave.api" {
		t.Errorf("credenciales mal enviadas: %+v", auth)
	}

	documento := servidor.documentoDe(t)
	encabezado := documento["Encabezado"].(map[string]interface{})
	identificacion := encabezado["IdentificacionDocumento"].(map[string]interface{})
	for campo, esperado := range map[string]string{
		"TipoDocumento":   "01",
		"NumeroDocumento": "123",
		"FechaEmision":    "27/11/2026",
		"Serie":           "A",
		"Sucursal":        "0001",
		"TipoDeVenta":     "Interna",
		"Moneda":          "VES",
		"TipoDePago":      "Inmediato",
		"TipoTransaccion": "01",
	} {
		if identificacion[campo] != esperado {
			t.Errorf("IdentificacionDocumento.%s = %v, se esperaba %q", campo, identificacion[campo], esperado)
		}
	}
	if !strings.HasSuffix(identificacion["HoraEmision"].(string), " am") {
		t.Errorf("HoraEmision debe ir en formato de 12 horas: %v", identificacion["HoraEmision"])
	}

	comprador := encabezado["Comprador"].(map[string]interface{})
	if comprador["TipoIdentificacion"] != "J" || comprador["NumeroIdentificacion"] != "123456789" {
		t.Errorf("el RIF debe ir separado en letra y número: %+v", comprador)
	}
	if comprador["Pais"] != "VE" {
		t.Errorf("Pais = %v", comprador["Pais"])
	}
	if correos, ok := comprador["Correo"].([]interface{}); !ok || len(correos) != 1 || correos[0] != "facturacion@example.com" {
		t.Errorf("Correo = %v", comprador["Correo"])
	}

	items := documento["DetallesItems"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 renglones, hay %d", len(items))
	}
	primero := items[0].(map[string]interface{})
	if primero["CodigoImpuesto"] != "G" || primero["TasaIVA"] != "16.00" {
		t.Errorf("alícuota del primer renglón: %+v", primero)
	}
	// 1000 con 5 % de descuento = 950; IVA 16 % = 152; total 1102.
	if primero["PrecioItem"] != "950.00" || primero["ValorIVA"] != "152.00" || primero["ValorTotalItem"] != "1102.00" {
		t.Errorf("cálculo del primer renglón: %+v", primero)
	}
	if primero["DescuentoMonto"] != "50.00" {
		t.Errorf("descuento del renglón = %v", primero["DescuentoMonto"])
	}
	if primero["IndicadorBienoServicio"] != "2" || primero["UnidadMedida"] != "ZZ" {
		t.Errorf("un servicio debe ir como servicio (2/ZZ): %+v", primero)
	}
	segundo := items[1].(map[string]interface{})
	if segundo["CodigoImpuesto"] != "E" || segundo["IndicadorBienoServicio"] != "1" || segundo["UnidadMedida"] != "NIU" {
		t.Errorf("un bien exento debe ir como bien (1/NIU/E): %+v", segundo)
	}

	totales := encabezado["Totales"].(map[string]interface{})
	// Gravado 950 + exento 1000 = 1950; IVA 152; con IVA 2102.
	for campo, esperado := range map[string]string{
		"NroItems":          "2",
		"MontoGravadoTotal": "950.00",
		"MontoExentoTotal":  "1000.00",
		"Subtotal":          "1950.00",
		"TotalIVA":          "152.00",
		"MontoTotalConIVA":  "2102.00",
		"TotalAPagar":       "2102.00",
		"TotalDescuento":    "50.00",
	} {
		if totales[campo] != esperado {
			t.Errorf("Totales.%s = %v, se esperaba %q", campo, totales[campo], esperado)
		}
	}
	impuestos := totales["ImpuestosSubtotal"].([]interface{})
	if len(impuestos) != 2 {
		t.Fatalf("se esperaban 2 alícuotas en ImpuestosSubtotal, hay %d", len(impuestos))
	}
	primera := impuestos[0].(map[string]interface{})
	if primera["CodigoTotalImp"] != "E" || primera["BaseImponibleImp"] != "1000.00" {
		t.Errorf("ImpuestosSubtotal[0] = %+v", primera)
	}

	servidor.mu.Lock()
	descargas := servidor.descargas
	servidor.mu.Unlock()
	if descargas != 1 {
		t.Errorf("se pidió el PDF %d veces, se esperaba 1", descargas)
	}
}

// TestReutilizaElTokenEntreDocumentos: el JWT dura doce horas; la caché evita
// una autenticación por factura.
func TestReutilizaElTokenEntreDocumentos(t *testing.T) {
	olvidarTokens(t)
	servidor := nuevoServidorFake(t)
	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))

	for i := 0; i < 3; i++ {
		doc := documentoDePrueba()
		doc.Numero = i + 1
		if _, err := proveedor.Emitir(context.Background(), doc); err != nil {
			t.Fatalf("emisión %d: %v", i+1, err)
		}
	}
	servidor.mu.Lock()
	auth := servidor.authPedidas
	servidor.mu.Unlock()
	if auth != 1 {
		t.Errorf("se autenticó %d veces para 3 documentos, se esperaba 1", auth)
	}
}

// TestRenuevaElTokenCon401: si el API rechaza el token, se pide otro y se
// reintenta una vez.
func TestRenuevaElTokenCon401(t *testing.T) {
	olvidarTokens(t)
	servidor := nuevoServidorFake(t)
	servidor.fallarPrimera = true

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	resultado, err := proveedor.Emitir(context.Background(), documentoDePrueba())
	if err != nil {
		t.Fatalf("tras el 401 debía reintentar y emitir: %v", err)
	}
	if resultado.NroControlTexto != "00-00000056" {
		t.Errorf("control = %q", resultado.NroControlTexto)
	}
	servidor.mu.Lock()
	auth := servidor.authPedidas
	servidor.mu.Unlock()
	if auth != 2 {
		t.Errorf("se autenticó %d veces, se esperaba 2 (una por el 401)", auth)
	}
}

// TestElCodigoDelCuerpoMandaAunqueElHTTPSea200: THKA informa duplicados y
// rechazos con HTTP 200; leer solo el HTTP daría por emitida una factura que no
// lo está.
func TestElCodigoDelCuerpoMandaAunqueElHTTPSea200(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		contiene string
	}{
		{
			"documento duplicado",
			`{"codigo":"201","mensaje":"Documento duplicado","resultado":{"numeroControl":""}}`,
			"Documento duplicado",
		},
		{
			"rechazo por validación",
			`{"codigo":"203","mensaje":"Documento no procesado",` +
				`"validaciones":["El campo Comprador.Direccion es obligatorio.","El RIF no es válido."]}`,
			"El campo Comprador.Direccion es obligatorio",
		},
		{
			"sin número de control",
			`{"codigo":"200","mensaje":"Documento procesado exitosamente","resultado":{}}`,
			"Documento procesado",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			olvidarTokens(t)
			servidor := nuevoServidorFake(t)
			servidor.respuestaCuerpo = caso.cuerpo

			proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
			_, err := proveedor.Emitir(context.Background(), documentoDePrueba())
			if err == nil {
				t.Fatal("un código distinto de 200 no puede devolver éxito")
			}
			if !strings.Contains(err.Error(), caso.contiene) {
				t.Errorf("el error debía contener %q, vino: %v", caso.contiene, err)
			}
		})
	}
}

// TestNoSeEmiteLoQueLaReferenciaNoAdmite: validaciones previas, sin llamada.
func TestNoSeEmiteLoQueLaReferenciaNoAdmite(t *testing.T) {
	casos := []struct {
		nombre   string
		tocar    func(*di.DatosDocumento)
		contiene string
	}{
		{"factura en dólares", func(d *di.DatosDocumento) { d.Moneda = "USD" }, "bolívares"},
		{"cliente sin dirección", func(d *di.DatosDocumento) { d.Cliente.Direccion = "" }, "dirección"},
		{"cliente sin RIF", func(d *di.DatosDocumento) { d.Cliente.FiscalRegistry = "" }, "RIF"},
		{"RIF con letra inválida", func(d *di.DatosDocumento) { d.Cliente.FiscalRegistry = "X-12345678" }, "V, E, P, J, G o C"},
		{"cliente sin nombre", func(d *di.DatosDocumento) { d.Cliente.Nombre = "" }, "nombre"},
		{"sin renglones", func(d *di.DatosDocumento) { d.Detalles = nil }, "renglones"},
		{"renglón a precio cero", func(d *di.DatosDocumento) { d.Detalles[0].PrecioUnitario = 0 }, "precio cero"},
		{"renglón con cantidad cero", func(d *di.DatosDocumento) { d.Detalles[0].Cantidad = 0 }, "cantidad cero"},
		{"nota sin factura afectada", func(d *di.DatosDocumento) { d.Tipo = di.DocNotaCredito }, "factura que afecta"},
		{
			"nota sin fecha de la afectada",
			func(d *di.DatosDocumento) { d.Tipo = di.DocNotaCredito; d.DocumentoAfectado = "9" },
			"fecha de la factura afectada",
		},
		{
			"nota sin monto de la afectada",
			func(d *di.DatosDocumento) {
				d.Tipo = di.DocNotaCredito
				d.DocumentoAfectado = "9"
				d.DocumentoAfectadoFecha = time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
			},
			"monto total de la factura afectada",
		},
		{
			"guía de despacho",
			func(d *di.DatosDocumento) { d.Tipo = di.DocGuiaDespacho },
			"guía de despacho",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			proveedor, err := NuevoProveedor(configDePrueba("http://127.0.0.1:1"))
			if err != nil {
				t.Fatalf("no se pudo crear el proveedor: %v", err)
			}
			doc := documentoDePrueba()
			caso.tocar(doc)
			_, err = proveedor.Emitir(context.Background(), doc)
			if err == nil {
				t.Fatal("debía rechazar el documento antes de llamar al proveedor")
			}
			if !strings.Contains(err.Error(), caso.contiene) {
				t.Errorf("el error debía nombrar %q, vino: %v", caso.contiene, err)
			}
		})
	}
}

// TestNotaDeCreditoLlevaLaFacturaAfectada: THKA imprime la fecha y el monto de
// la factura que la nota afecta.
func TestNotaDeCreditoLlevaLaFacturaAfectada(t *testing.T) {
	olvidarTokens(t)
	servidor := nuevoServidorFake(t)
	servidor.respuestaCuerpo = `{"codigo":"200","mensaje":"Documento procesado exitosamente",` +
		`"resultado":{"serie":"A","numeroControl":"00-00000125"}}`

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	doc := documentoDePrueba()
	doc.Tipo = di.DocNotaCredito
	doc.Numero = 24
	doc.DocumentoAfectado = "123"
	doc.DocumentoAfectadoFecha = time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local)
	doc.DocumentoAfectadoMonto = 2135
	doc.Descripcion = "Devolución total de la mercancía"

	resultado, err := proveedor.Emitir(context.Background(), doc)
	if err != nil {
		t.Fatalf("la nota falló: %v", err)
	}
	if resultado.NroControlTexto != "00-00000125" {
		t.Errorf("control = %q", resultado.NroControlTexto)
	}

	documento := servidor.documentoDe(t)
	identificacion := documento["Encabezado"].(map[string]interface{})["IdentificacionDocumento"].(map[string]interface{})
	for campo, esperado := range map[string]string{
		"TipoDocumento":             "02",
		"NumeroDocumento":           "24",
		"NumeroFacturaAfectada":     "123",
		"FechaFacturaAfectada":      "20/11/2026",
		"MontoFacturaAfectada":      "2135.00",
		"ComentarioFacturaAfectada": "Devolución total de la mercancía",
		"TipoTransaccion":           "02",
	} {
		if identificacion[campo] != esperado {
			t.Errorf("IdentificacionDocumento.%s = %v, se esperaba %q", campo, identificacion[campo], esperado)
		}
	}
}

// TestElIGTFEntraEnElTotalAPagar: la referencia dice que el total a pagar es
// montoTotalConIVA + IGTF; si el ERP cobró IGTF, la factura debe imprimirlo.
func TestElIGTFEntraEnElTotalAPagar(t *testing.T) {
	olvidarTokens(t)
	servidor := nuevoServidorFake(t)
	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))

	doc := documentoDePrueba()
	doc.IGTF = 30 // 3 % de 1000 en divisa
	if _, err := proveedor.Emitir(context.Background(), doc); err != nil {
		t.Fatalf("la emisión falló: %v", err)
	}
	totales := servidor.documentoDe(t)["Encabezado"].(map[string]interface{})["Totales"].(map[string]interface{})
	// 2102 (con IVA de la prueba) + 30 de IGTF.
	if totales["TotalAPagar"] != "2132.00" {
		t.Errorf("TotalAPagar = %v, se esperaba 2132.00 (con IGTF)", totales["TotalAPagar"])
	}
	encontrado := false
	for _, impuesto := range totales["ImpuestosSubtotal"].([]interface{}) {
		m := impuesto.(map[string]interface{})
		if m["CodigoTotalImp"] == "IGTF" {
			encontrado = true
			if m["ValorTotalImp"] != "30.00" || m["AlicuotaImp"] != "3.00" {
				t.Errorf("el IGTF debe ir con su alícuota y su monto: %+v", m)
			}
		}
	}
	if !encontrado {
		t.Error("el IGTF debía declararse en ImpuestosSubtotal")
	}
}

// TestPingValidaCredenciales: la prueba de conexión es la autenticación real.
func TestPingValidaCredenciales(t *testing.T) {
	olvidarTokens(t)
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"codigo":"401","mensaje":"Usuario o clave inválidos"}`))
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	err := proveedor.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Usuario o clave inválidos") {
		t.Fatalf("el ping debía fallar con el mensaje del proveedor: %v", err)
	}
}

// TestCatalogosYTraducciones fija las equivalencias con los catálogos de THKA.
func TestCatalogosYTraducciones(t *testing.T) {
	for tipo, esperado := range map[di.DocumentType]string{
		di.DocFactura: "01", di.DocNotaCredito: "02", di.DocNotaDebito: "03",
	} {
		obtenido, err := tipoDocumentoTHKA(tipo)
		if err != nil || obtenido != esperado {
			t.Errorf("tipoDocumentoTHKA(%s) = %q, %v; se esperaba %q", tipo, obtenido, err, esperado)
		}
	}
	if _, err := tipoDocumentoTHKA(di.DocGuiaDespacho); err == nil {
		t.Error("la guía de despacho debía explicar su límite")
	}

	for _, caso := range []struct {
		item     di.DetalleItem
		esperado string
	}{
		{di.DetalleItem{CodigoImpuesto: "G", PorcentajeIVA: 16}, "G"},
		{di.DetalleItem{CodigoImpuesto: "R", PorcentajeIVA: 8}, "R"},
		{di.DetalleItem{CodigoImpuesto: "A", PorcentajeIVA: 31}, "A"},
		{di.DetalleItem{CodigoImpuesto: "E", EsExento: true}, "E"},
		{di.DetalleItem{CodigoImpuesto: "P"}, "P"},
		{di.DetalleItem{PorcentajeIVA: 16}, "G"},
		{di.DetalleItem{PorcentajeIVA: 0, EsExento: true}, "E"},
	} {
		if obtenido := codigoImpuestoDe(caso.item); obtenido != caso.esperado {
			t.Errorf("codigoImpuestoDe(%+v) = %q, se esperaba %q", caso.item, obtenido, caso.esperado)
		}
	}

	for codigo, tasa := range map[string]float64{"G": 16, "R": 8, "A": 31, "E": 0, "P": 0} {
		if obtenido := tasaDeCodigo(codigo); obtenido != tasa {
			t.Errorf("tasaDeCodigo(%q) = %v, se esperaba %v", codigo, obtenido, tasa)
		}
	}

	if tipoDePago("15 DIAS") != "Crédito" || tipoDePago("contado") != "Inmediato" {
		t.Error("tipoDePago no traduce el crédito del ERP")
	}
}

// TestIdentidadSeparadaEnLetraYNumero.
func TestIdentidadSeparadaEnLetraYNumero(t *testing.T) {
	casos := []struct {
		rif, tipo, letra, digitos string
		falla                     bool
	}{
		{"J-12345678-9", "J", "J", "123456789", false},
		{"V-12345678", "V", "V", "12345678", false},
		{"12345678", "V", "V", "12345678", false},
		{"", "V", "", "", true},
		{"X-12345678", "X", "", "", true},
	}
	for _, caso := range casos {
		letra, digitos, err := identidadDe(caso.rif, caso.tipo)
		if caso.falla {
			if err == nil {
				t.Errorf("con RIF %q debía fallar", caso.rif)
			}
			continue
		}
		if err != nil || letra != caso.letra || digitos != caso.digitos {
			t.Errorf("identidadDe(%q) = (%q,%q,%v), se esperaba (%q,%q)", caso.rif, letra, digitos, err, caso.letra, caso.digitos)
		}
	}
}
