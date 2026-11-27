package cgimprenta

// Pruebas del proveedor CG Imprenta Digital.
//
// Qué se demuestra aquí y por qué no basta con "compila":
//
//   - Que el documento que sale por el cable tiene lo que la API de CG exige
//     (serie, RIF sin guiones, tasa del día con su fecha y renglones con código
//     y alícuota). Un payload con un campo de menos es una factura que no se
//     emite, y el error recién aparece en producción.
//   - Que el token se reutiliza y se renueva con 401, porque la API lo vence a
//     la hora: sin caché cada factura pediría un token, y sin renovación el ERP
//     dejaría de emitir a la hora exacta de haber arrancado.
//   - Que lo que rechaza el proveedor llega con SU mensaje, que es la única
//     pista que tiene el administrador en la pantalla.

import (
	"context"
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

// olvidarTokens limpia la caché de JWT entre pruebas. La caché vive a nivel de
// paquete a propósito (la cola crea un proveedor por emisión), así que cada
// prueba arranca sin tokens para no heredar el de la anterior.
func olvidarTokens(t *testing.T) {
	t.Helper()
	tokensMu.Lock()
	tokensPorCredencial = map[string]tokenEnMemoria{}
	tokensMu.Unlock()
}

// documentoDePrueba es una factura con un renglón gravado y otro exento, que es
// la forma típica de una venta venezolana.
func documentoDePrueba() *di.DatosDocumento {
	return &di.DatosDocumento{
		Tipo:         di.DocFactura,
		Numero:       123,
		FechaEmision: time.Date(2026, 11, 27, 10, 30, 0, 0, time.UTC),
		FechaTasa:    time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		Moneda:       "VES",
		TasaCambio:   342.8633,
		Total:        1160,
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
				Cantidad: 2, PrecioUnitario: 10.5, DescuentoPct: 5, PorcentajeIVA: 16, CodigoImpuesto: "G"},
			{Codigo: "LIBRO", Nombre: "Libro escolar", Descripcion: "Libro escolar",
				Cantidad: 1, PrecioUnitario: 25, PorcentajeIVA: 0, EsExento: true, CodigoImpuesto: "E"},
		},
	}
}

func configDePrueba(baseURL string) di.ProviderConfig {
	return di.ProviderConfig{
		Code:         codigoProveedor,
		BaseURL:      baseURL,
		UserName:     "api@negocio.test",
		UserPassword: "clave-de-api",
		NumeroSerie:  "A",
	}
}

// TestEmiteFacturaConLosDatosFiscalesDelERP fija el contrato con el API: qué
// campos viajan y con qué forma.
func TestEmiteFacturaConLosDatosFiscalesDelERP(t *testing.T) {
	olvidarTokens(t)

	var (
		mu           sync.Mutex
		pedidos      []map[string]interface{}
		tokenPedidos int
	)
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/api/Invoice/create_token_authenticator":
			mu.Lock()
			tokenPedidos++
			mu.Unlock()
			var credenciales struct {
				UserName     string `json:"userName"`
				UserPassword string `json:"userPassword"`
			}
			if err := json.Unmarshal(cuerpo, &credenciales); err != nil {
				t.Errorf("el cuerpo del token no es JSON: %v", err)
			}
			if credenciales.UserName != "api@negocio.test" || credenciales.UserPassword != "clave-de-api" {
				t.Errorf("credenciales mal enviadas: %+v", credenciales)
			}
			_, _ = w.Write([]byte(`{"success":true,"message":"Token de Acceso","token":"jwt-1"}`))
		case "/api/Invoice/add_list_invoice":
			if r.Header.Get("Authorization") != "Bearer jwt-1" {
				t.Errorf("la factura no llevó el token: %q", r.Header.Get("Authorization"))
			}
			var pedido map[string]interface{}
			if err := json.Unmarshal(cuerpo, &pedido); err != nil {
				t.Errorf("el cuerpo de la factura no es JSON: %v", err)
			}
			mu.Lock()
			pedidos = append(pedidos, pedido)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true,"message":"Facturas guardadas satisfactoriamente.",
				"invoice_errors":[],
				"invoice_list_success":[{"invoice_number":"123","control_number":"00-00000056",
				"invoice_pdf":"https://cg.example/readonly/export_pdf/abc-123"}]}`))
		default:
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
	}))
	defer servidor.Close()

	proveedor, err := NuevoProveedor(configDePrueba(servidor.URL + "/"))
	if err != nil {
		t.Fatalf("no se pudo crear el proveedor: %v", err)
	}
	resultado, err := proveedor.Emitir(context.Background(), documentoDePrueba())
	if err != nil {
		t.Fatalf("la emisión falló: %v", err)
	}

	// La respuesta: control con serie, PDF y el identificador del documento.
	if resultado.NroControlTexto != "00-00000056" {
		t.Errorf("control textual = %q, se esperaba 00-00000056", resultado.NroControlTexto)
	}
	if resultado.NroControl != 56 || resultado.Serie != "00" {
		t.Errorf("control partido = (%q, %d), se esperaba (00, 56)", resultado.Serie, resultado.NroControl)
	}
	if resultado.StrongID != "abc-123" {
		t.Errorf("strong_id = %q, se esperaba el identificador del PDF (abc-123)", resultado.StrongID)
	}
	if resultado.PDFURL == "" || resultado.Status != "Approved" || !resultado.EsAsignado {
		t.Errorf("respuesta incompleta: %+v", resultado)
	}

	// El payload: lo que la API exige para emitir.
	if len(pedidos) != 1 {
		t.Fatalf("se esperaba 1 petición de factura, hubo %d", len(pedidos))
	}
	pedido := pedidos[0]
	if pedido["numeroSerie"] != "A" {
		t.Errorf("numeroSerie = %v", pedido["numeroSerie"])
	}
	if pedido["cantidadFactura"].(float64) != 1 {
		t.Errorf("cantidadFactura = %v", pedido["cantidadFactura"])
	}
	facturas, ok := pedido["facturas"].([]interface{})
	if !ok || len(facturas) != 1 {
		t.Fatalf("facturas mal formadas: %#v", pedido["facturas"])
	}
	factura := facturas[0].(map[string]interface{})

	if factura["numeroFactura"] != "123" {
		t.Errorf("numeroFactura = %v", factura["numeroFactura"])
	}
	if factura["documentoIdentidadCliente"] != "J123456789" {
		t.Errorf("el RIF debe ir sin guiones: %v", factura["documentoIdentidadCliente"])
	}
	if factura["tasa_del_dia"] != "342.8633" {
		t.Errorf("tasa_del_dia = %v", factura["tasa_del_dia"])
	}
	if factura["fecha_tasa"] != "27-11-2026" {
		t.Errorf("fecha_tasa = %v", factura["fecha_tasa"])
	}
	if factura["correoCliente"] != "facturacion@example.com" {
		t.Errorf("correoCliente = %v", factura["correoCliente"])
	}

	productos, ok := factura["productos"].([]interface{})
	if !ok || len(productos) != 2 {
		t.Fatalf("productos mal formados: %#v", factura["productos"])
	}
	primero := productos[0].(map[string]interface{})
	if primero["codigoProducto"] != "SERV-001" {
		t.Errorf("codigoProducto = %v", primero["codigoProducto"])
	}
	if primero["tipoImpuesto"] != "G" {
		t.Errorf("tipoImpuesto del renglón gravado = %v", primero["tipoImpuesto"])
	}
	if primero["cantidadAdquirida"] != "2" {
		t.Errorf("cantidadAdquirida = %v", primero["cantidadAdquirida"])
	}
	if primero["precioProducto"] != "10.50" {
		t.Errorf("precioProducto = %v", primero["precioProducto"])
	}
	if primero["discount"].(float64) != 5 {
		t.Errorf("discount = %v", primero["discount"])
	}
	segundo := productos[1].(map[string]interface{})
	if segundo["tipoImpuesto"] != "E" {
		t.Errorf("tipoImpuesto del renglón exento = %v", segundo["tipoImpuesto"])
	}
	if _, hay := segundo["discount"]; hay {
		t.Error("un renglón sin descuento no debe llevar la clave discount")
	}

	if tokenPedidos != 1 {
		t.Errorf("se pidió el token %d veces, se esperaba 1", tokenPedidos)
	}
}

// TestReutilizaElTokenEntreFacturas: la caché es la que evita un viaje extra
// por cada documento cuando la cola emite un lote.
func TestReutilizaElTokenEntreFacturas(t *testing.T) {
	olvidarTokens(t)

	var (
		mu           sync.Mutex
		tokenPedidos int
	)
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Invoice/create_token_authenticator":
			mu.Lock()
			tokenPedidos++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-estable"}`))
		case "/api/Invoice/add_list_invoice":
			// El servidor falso devuelve el control de la factura que le
			// pidieron: así la segunda emisión también encuentra su entrada.
			cuerpo, _ := io.ReadAll(r.Body)
			var pedido struct {
				Facturas []struct {
					Numero string `json:"numeroFactura"`
				} `json:"facturas"`
			}
			_ = json.Unmarshal(cuerpo, &pedido)
			numero := "0"
			if len(pedido.Facturas) == 1 {
				numero = pedido.Facturas[0].Numero
			}
			_, _ = w.Write([]byte(`{"success":true,"invoice_list_success":[{"invoice_number":"` + numero +
				`","control_number":"00-00000001","invoice_pdf":"https://cg.example/readonly/export_pdf/` + numero + `"}]}`))
		}
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	for i := 0; i < 3; i++ {
		doc := documentoDePrueba()
		doc.Numero = i + 1
		if _, err := proveedor.Emitir(context.Background(), doc); err != nil {
			t.Fatalf("emisión %d: %v", i+1, err)
		}
	}
	if tokenPedidos != 1 {
		t.Errorf("se pidió el token %d veces para 3 facturas, se esperaba 1", tokenPedidos)
	}
}

// TestRenuevaElTokenCuandoLaAPIContesta401: la API vence el token a la hora y
// responde 401; el adaptador tiene que pedir otro y reintentar una vez.
func TestRenuevaElTokenCuandoLaAPIContesta401(t *testing.T) {
	olvidarTokens(t)

	var (
		mu             sync.Mutex
		tokenPedidos   int
		facturasPedido int
	)
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Invoice/create_token_authenticator":
			mu.Lock()
			tokenPedidos++
			n := tokenPedidos
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-` + string(rune('0'+n)) + `"}`))
		case "/api/Invoice/add_list_invoice":
			mu.Lock()
			facturasPedido++
			intento := facturasPedido
			mu.Unlock()
			if intento == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"success":false,"message":"Token expirado"}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"invoice_list_success":[{"invoice_number":"123","control_number":"00-00000056","invoice_pdf":"https://cg.example/readonly/export_pdf/abc"}]}`))
		}
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	resultado, err := proveedor.Emitir(context.Background(), documentoDePrueba())
	if err != nil {
		t.Fatalf("tras el 401 debía reintentar y emitir: %v", err)
	}
	if resultado.NroControlTexto != "00-00000056" {
		t.Errorf("control = %q", resultado.NroControlTexto)
	}
	if tokenPedidos != 2 {
		t.Errorf("se pidió el token %d veces, se esperaba 2 (uno por el 401)", tokenPedidos)
	}
	if facturasPedido != 2 {
		t.Errorf("se intentó emitir %d veces, se esperaba 2", facturasPedido)
	}
}

// TestElRechazoDelProveedorLlegaConSuMensaje: el error que queda en la cola es
// el texto que el administrador puede accionar.
func TestElRechazoDelProveedorLlegaConSuMensaje(t *testing.T) {
	olvidarTokens(t)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/Invoice/create_token_authenticator" {
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-1"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"message":"El campo Correo del Cliente en la factura 123 es obligatorio."}`))
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	_, err := proveedor.Emitir(context.Background(), documentoDePrueba())
	if err == nil {
		t.Fatal("un rechazo del proveedor no puede devolver éxito")
	}
	if !strings.Contains(err.Error(), "El campo Correo del Cliente") {
		t.Errorf("el error debe traer el mensaje del proveedor, vino: %v", err)
	}
}

// TestFacturaDuplicadaSeInformaComoTal: si el número ya existía, CG no emite y
// lo lista en invoice_errors con HTTP 200.
func TestFacturaDuplicadaSeInformaComoTal(t *testing.T) {
	olvidarTokens(t)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/Invoice/create_token_authenticator" {
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"Las facturas se guardaron correctamente, pero algunas estaban duplicadas y no pudieron guardarse.","invoice_errors":["123"],"invoice_list_success":[]}`))
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	_, err := proveedor.Emitir(context.Background(), documentoDePrueba())
	if err == nil || !strings.Contains(err.Error(), "duplicado") {
		t.Fatalf("debía informar el duplicado, vino: %v", err)
	}
}

// TestNoSeEmiteLoQueCGNoPuedeRepresentar: estas validaciones ocurren ANTES de
// llamar al proveedor. Mandar dólares como si fueran bolívares sería un
// documento fiscal falso, no un reintento.
func TestNoSeEmiteLoQueCGNoPuedeRepresentar(t *testing.T) {
	casos := []struct {
		nombre   string
		tocar    func(*di.DatosDocumento)
		cfg      func(*di.ProviderConfig)
		contiene string
	}{
		{"factura en dólares", func(d *di.DatosDocumento) { d.Moneda = "USD" }, nil, "bolívares"},
		{"sin tasa", func(d *di.DatosDocumento) { d.TasaCambio = 0 }, nil, "tasa de cambio"},
		{"cliente sin correo", func(d *di.DatosDocumento) { d.Cliente.Email = "" }, nil, "correo"},
		{"cliente sin dirección", func(d *di.DatosDocumento) { d.Cliente.Direccion = "" }, nil, "dirección"},
		{"cliente sin teléfono", func(d *di.DatosDocumento) { d.Cliente.Telefono = "" }, nil, "teléfono"},
		{"RIF inválido", func(d *di.DatosDocumento) { d.Cliente.FiscalRegistry = "123" }, nil, "identidad"},
		{"renglón a precio cero", func(d *di.DatosDocumento) { d.Detalles[0].PrecioUnitario = 0 }, nil, "precio cero"},
		{"renglón con cantidad cero", func(d *di.DatosDocumento) { d.Detalles[0].Cantidad = 0 }, nil, "cantidad cero"},
		{"sin renglones", func(d *di.DatosDocumento) { d.Detalles = nil }, nil, "renglones"},
		{"sin serie configurada", nil, func(c *di.ProviderConfig) { c.NumeroSerie = "" }, "serie"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cfg := configDePrueba("http://127.0.0.1:1")
			if caso.cfg != nil {
				caso.cfg(&cfg)
			}
			proveedor, err := NuevoProveedor(cfg)
			if err != nil {
				t.Fatalf("no se pudo crear el proveedor: %v", err)
			}
			doc := documentoDePrueba()
			if caso.tocar != nil {
				caso.tocar(doc)
			}
			if _, err := proveedor.Emitir(context.Background(), doc); err == nil {
				t.Fatalf("debía rechazar la factura antes de llamar al proveedor")
			} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(caso.contiene)) {
				t.Errorf("el error debía nombrar %q, vino: %v", caso.contiene, err)
			}
		})
	}
}

// TestIdentidadDelCliente: la convención de CG es letra + 6..10 dígitos, sin
// guiones ni puntos. El RIF venezolano trae guiones y un dígito verificador que
// aquí se conserva (9 dígitos entran en el rango).
func TestIdentidadDelCliente(t *testing.T) {
	casos := []struct {
		rif, tipo, esperado string
		falla               bool
	}{
		{"J-12345678-9", "J", "J123456789", false},
		{"V-12345678", "V", "V12345678", false},
		{"12345678", "V", "V12345678", false},
		{"J123456789", "", "J123456789", false},
		{"", "J", "", true},
		{"J-123", "J", "", true},
		{"X-12345678", "X", "", true},
	}
	for _, caso := range casos {
		obtenido, err := identidadDe(caso.rif, caso.tipo)
		if caso.falla {
			if err == nil {
				t.Errorf("con RIF %q debía fallar y dio %q", caso.rif, obtenido)
			}
			continue
		}
		if err != nil {
			t.Errorf("RIF %q: error inesperado: %v", caso.rif, err)
			continue
		}
		if obtenido != caso.esperado {
			t.Errorf("RIF %q → %q, se esperaba %q", caso.rif, obtenido, caso.esperado)
		}
	}
}

// TestPartirControl: el número de control de CG trae su serie separada por guion.
func TestPartirControl(t *testing.T) {
	casos := []struct {
		control, serie string
		numero         int
	}{
		{"00-00000056", "00", 56},
		{"01-00001234", "01", 1234},
		{"00000070", "", 70},
		{"", "", 0},
	}
	for _, caso := range casos {
		serie, numero := partirControl(caso.control)
		if serie != caso.serie || numero != caso.numero {
			t.Errorf("partirControl(%q) = (%q,%d), se esperaba (%q,%d)",
				caso.control, serie, numero, caso.serie, caso.numero)
		}
	}
}

// TestCodigoDeProducto: CG exige máximo 13 caracteres y sin espacios; el código
// del ERP se adapta en vez de perder el renglón.
func TestCodigoDeProducto(t *testing.T) {
	if c := codigoDe("SERV-001", 1); c != "SERV-001" {
		t.Errorf("código simple = %q", c)
	}
	if c := codigoDe("CODIGO CON ESPACIOS MUY LARGO", 2); len([]rune(c)) > 13 || strings.Contains(c, " ") {
		t.Errorf("código sin sanear = %q", c)
	}
	if c := codigoDe("", 3); c != "RENGLON-3" {
		t.Errorf("código de relleno = %q", c)
	}
}

// TestEmitirNotaCredito: la nota total necesita la factura afectada y su motivo.
func TestEmitirNotaCredito(t *testing.T) {
	olvidarTokens(t)

	var capturado map[string]interface{}
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Invoice/create_token_authenticator":
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-1"}`))
		case "/api/Invoice/add_credit_note":
			cuerpo, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(cuerpo, &capturado)
			_, _ = w.Write([]byte(`{"success":true,"message":"Nota de crédito guardada satisfactoriamente.","control_number":"00-00000125","credit_note_pdf":"https://cg.example/readonly/export_pdf/nota-1"}`))
		}
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	doc := documentoDePrueba()
	doc.Tipo = di.DocNotaCredito
	doc.Numero = 24
	doc.DocumentoAfectado = "123"
	doc.Descripcion = "Devolución total de la mercancía"

	resultado, err := proveedor.Emitir(context.Background(), doc)
	if err != nil {
		t.Fatalf("la nota falló: %v", err)
	}
	if resultado.NroControlTexto != "00-00000125" || resultado.StrongID != "nota-1" {
		t.Errorf("resultado inesperado: %+v", resultado)
	}
	if capturado["numeroFactura"] != "123" || capturado["numeroNotaCredito"] != "24" {
		t.Errorf("la nota no afecta a la factura correcta: %#v", capturado)
	}
}

// TestLosDocumentosQueNoSeEmitenLoDicen: guías y retenciones no se emiten por
// este contrato y el mensaje explica POR QUÉ, en vez de un "no implementado".
func TestLosDocumentosQueNoSeEmitenLoDicen(t *testing.T) {
	proveedor, _ := NuevoProveedor(configDePrueba("http://127.0.0.1:1"))
	doc := documentoDePrueba()
	for _, tipo := range []di.DocumentType{di.DocGuiaDespacho, di.DocRetencionIVA} {
		doc.Tipo = tipo
		_, err := proveedor.Emitir(context.Background(), doc)
		if err == nil || !strings.Contains(err.Error(), "no se emite") {
			t.Errorf("tipo %s debía explicar su límite, vino: %v", tipo, err)
		}
	}
}

// TestPingValidaCredencialesYConectividad.
func TestPingValidaCredencialesYConectividad(t *testing.T) {
	olvidarTokens(t)

	falla := true
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/Invoice/create_token_authenticator" {
			if falla {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"success":false,"message":"No existe coincidencia de usuario en la Base de Datos."}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"token":"jwt-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"payment_methods":[{"id_payment_method":"1","payment_method":"transferencia"}]}`))
	}))
	defer servidor.Close()

	proveedor, _ := NuevoProveedor(configDePrueba(servidor.URL))
	err := proveedor.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "No existe coincidencia") {
		t.Fatalf("con credenciales inválidas el ping debía fallar con el mensaje del proveedor: %v", err)
	}

	falla = false
	olvidarTokens(t)
	if err := proveedor.Ping(context.Background()); err != nil {
		t.Fatalf("con credenciales válidas el ping debía pasar: %v", err)
	}
}
