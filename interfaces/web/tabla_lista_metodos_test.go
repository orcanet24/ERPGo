package web

// Las tablas DataTables de las vistas piden sus datos con
// ajax: { type: 'POST' }. Si la ruta quedó registrada como «GET /xxx/data»,
// el ServeMux responde 405 a ese POST y la tabla nunca carga: el usuario ve
// la pantalla vacía sin ningún mensaje de error que lo explique.
//
// Por eso los listados de datos se registran SIN prefijo de método (igual que
// customer, invoice, product, purchase y return, que sí cargan). Estas
// pruebas recorren las cinco rutas que estaban declaradas solo con GET y
// comprueban que un POST las alcanza de verdad, responde 200 y devuelve el
// cuerpo JSON que DataTables espera; de paso verifican que el GET de la misma
// ruta sigue funcionando, porque quitar el prefijo no debe romperlo.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListadosDeTablasAceptanPost(t *testing.T) {
	s, db := basePrueba(t)
	s.mux = http.NewServeMux()
	s.registerOrderRoutes()
	s.registerDeliveryOrderRoutes()
	s.registerPriceListRoutes()

	// Esquema mínimo de cada consulta de datos: sin estas tablas el handler
	// devolvería 500 y la prueba confundiría un esquema faltante con el 405
	// que es lo que de verdad se está vigilando aquí.
	tablas := []string{
		`CREATE TABLE customer_information (customer_id INTEGER PRIMARY KEY, customer_name TEXT)`,
		`CREATE TABLE sale_order (id INTEGER PRIMARY KEY, order_no TEXT, customer_id INTEGER, status TEXT, date TEXT, notes TEXT, created_at TEXT)`,
		`CREATE TABLE delivery_order (id INTEGER PRIMARY KEY, customer_id INTEGER, status TEXT, date TEXT, warehouse_id TEXT, notes TEXT, created_at TEXT)`,
		`CREATE TABLE price_list (id INTEGER PRIMARY KEY, code TEXT, name TEXT, sort_order INTEGER, is_default INTEGER, currency_code TEXT, status TEXT)`,
		`CREATE TABLE product_information (product_id INTEGER PRIMARY KEY, product_name TEXT, price REAL, price_usd REAL, currency_code TEXT)`,
		`CREATE TABLE product_price_list (product_id INTEGER, price_list_id INTEGER, price REAL)`,
		`CREATE TABLE product_price_discount (id INTEGER PRIMARY KEY, product_id INTEGER, customer_id INTEGER, customer_class TEXT, price_list_id INTEGER, discount_pct REAL, min_qty INTEGER, valid_from TEXT, valid_until TEXT, reason TEXT, status TEXT)`,
	}
	for _, ddl := range tablas {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("crear tabla de listado: %v\n%s", err, ddl)
		}
	}

	casos := []struct {
		nombre string
		ruta   string
	}{
		{"pedidos", "/order/data"},
		{"ordenes de entrega", "/delivery_order/data"},
		{"listas de precio", "/price_list/data"},
		{"precios de productos", "/product_price/data"},
		{"descuentos por cliente", "/product_price/discount/data"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			// Lo que hace la tabla en el navegador: POST con parámetros
			// de DataTables vacíos basta para que el handler responda.
			res := httptest.NewRecorder()
			s.mux.ServeHTTP(res, montarPeticionConSesion("POST", caso.ruta, s))

			if res.Code == http.StatusMethodNotAllowed {
				t.Fatalf("POST %s devolvió 405: la ruta sigue declarada solo con GET y la tabla nunca carga", caso.ruta)
			}
			if res.Code == http.StatusNotFound {
				t.Fatalf("POST %s devolvió 404: la ruta ya no está registrada", caso.ruta)
			}
			if res.Code != http.StatusOK {
				t.Fatalf("POST %s respondió %d; debe ser 200. Cuerpo: %s", caso.ruta, res.Code, res.Body.String())
			}

			// El cuerpo debe ser el JSON de DataTables, con su clave "data".
			var cuerpo map[string]interface{}
			if err := json.Unmarshal(res.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("POST %s no devolvió JSON válido: %v. Cuerpo: %s", caso.ruta, err, res.Body.String())
			}
			if _, ok := cuerpo["data"]; !ok {
				t.Errorf("POST %s devolvió JSON sin la clave \"data\" que espera DataTables: %s", caso.ruta, res.Body.String())
			}

			// Quitar el prefijo no puede romper tampoco el GET: quien abre
			// la pantalla sigue pudiendo pedir los datos por ese verbo.
			resGet := httptest.NewRecorder()
			s.mux.ServeHTTP(resGet, montarPeticionConSesion("GET", caso.ruta, s))
			if resGet.Code != http.StatusOK {
				t.Errorf("GET %s respondió %d; la ruta debe seguir aceptando el método original. Cuerpo: %s", caso.ruta, resGet.Code, resGet.Body.String())
			}
		})
	}
}
