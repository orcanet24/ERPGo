package web

// semilla_carniceria_util.go — Utilidades del semillero.
//
// Todo lo que aquí está es una consulta o una fecha. NINGUNA función de este
// fichero escribe en la base: leer es seguro y repetible, y por eso se separa
// del fichero que sí escribe, que es donde vive el riesgo.
//
// Y una regla que atraviesa los tres ficheros del semillero: el dinero se pasa
// por `round2()` ANTES de llegar a la base. Un `float64` crudo en una columna
// decimal(18,2) acumula error de redondeo en cada suma, y un total que cuadra
// al céntimo es lo mínimo que se le puede pedir a un libro.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Peticiones
// ---------------------------------------------------------------------------

// grabaWriter es un http.ResponseWriter mínimo para las llamadas internas.
type grabaWriter struct {
	cabeceras http.Header
	codigo    int
	buf       []byte
}

func (g *grabaWriter) Header() http.Header {
	if g.cabeceras == nil {
		g.cabeceras = http.Header{}
	}
	return g.cabeceras
}
func (g *grabaWriter) Write(p []byte) (int, error) {
	g.buf = append(g.buf, p...)
	if g.codigo == 0 {
		g.codigo = 200
	}
	return len(p), nil
}
func (g *grabaWriter) WriteHeader(c int) {
	if g.codigo == 0 {
		g.codigo = c
	}
}

func cuerpoDe(g *grabaWriter) string { return string(g.buf) }

// newFormRequest arma una petición POST con cookie de sesión y token CSRF, como
// la que manda el navegador.
//
// El token va en la CABECERA `X-CSRF-Token`, que es lo que hace
// `static/js/csrf.js`. Mandarlo como `_token` en el cuerpo no vale: el servidor
// lee `csrf_token`, y la cookie se llama `erpgo_csrf`. Con el token mal puesto,
// todos loshandlers de escritura responden 403 y el semillero no carga nada
// sin decir por qué.
func newFormRequest(s *Server, sesion string, campos url.Values) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(campos.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if sesion != "" {
		r.AddCookie(&http.Cookie{Name: "erpgow_session", Value: sesion})
	}
	if sess := s.sessions.Get(sesion); sess != nil {
		r.Header.Set("X-CSRF-Token", s.ensureCSRFToken(sess))
	}
	return r
}

// ---------------------------------------------------------------------------
// Fechas
// ---------------------------------------------------------------------------

// mesDe devuelve "septiembre" a partir de "2026-09". Solo para los textos del
// parte de compra.
func mesDe(periodo string) string {
	nombres := []string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
		"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	t, err := time.Parse("2006-01", periodo)
	if err != nil || int(t.Month()) < 1 || int(t.Month()) > 12 {
		return periodo
	}
	return nombres[int(t.Month())-1]
}

// fechaVence devuelve la fecha de caducidad de un lote: el día 5 del mes que
// viene del inicio del período.
//
// Se usa el mes SIGUIENTE a propósito: una canal que entra el día 3 no caduca
// el día 3 del mes que entra, y un lote que caduca el mismo mes que se compró
// hace que el FEFO descarte mercadería válida.
func fechaVence(periodo string, dias int) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return periodo + "-05"
	}
	siguiente := t.AddDate(0, 1, dias)
	return siguiente.Format("2006-01-02")
}

// fechaEnDia da un día del mes, sin pasarse del último día.
//
// `min` y `max` acotan: en febrero, día 31 es el 28. Sin ese recorte, la fecha
// se normaliza sola a marzo y una venta del "28/02" acaba en marzo, con lo que
// el mes de febrero pierde operaciones sin que nadie lo note en el informe.
func fechaEnDia(periodo string, dia int) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return time.Now().Format("2006-01-02")
	}
	ultimo := t.AddDate(0, 1, -1).Day()
	if dia > ultimo {
		dia = ultimo
	}
	if dia < 1 {
		dia = 1
	}
	return fmt.Sprintf("%s-%02d", periodo, dia)
}

// rangoDelMes devuelve el primer y el último día del mes.
func rangoDelMes(periodo string) (string, string) {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return periodo + "-01", periodo + "-28"
	}
	ultimo := t.AddDate(0, 1, -1).Day()
	return fmt.Sprintf("%s-01", periodo), fmt.Sprintf("%s-%02d", periodo, ultimo)
}

// ---------------------------------------------------------------------------
// Consultas de lectura
// ---------------------------------------------------------------------------

func mustEntero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// idProveedor devuelve el id de un proveedor por nombre.
func idProveedor(db *sql.DB, nombre string) int64 {
	var id int64
	if err := db.QueryRow(`SELECT supplier_id FROM supplier_information
		WHERE supplier_name = ? ORDER BY supplier_id LIMIT 1`, nombre).Scan(&id); err != nil {
		return 0
	}
	return id
}

// clientesMostrador devuelve los clientes de mostrador, ordenados por id para
// que la siembra sea reproducible.
func clientesMostrador(db *sql.DB) []int64 { return clientesPorTipo(db, 1) }

// clientesMayores devuelve los clientes al mayor.
func clientesMayores(db *sql.DB) []int64 { return clientesPorTipo(db, 2) }

func clientesPorTipo(db *sql.DB, tipo int64) []int64 {
	filas, err := db.Query(`SELECT customer_id FROM customer_information
		WHERE status = 1 AND customer_type = ? ORDER BY customer_id`, tipo)
	if err != nil {
		return nil
	}
	defer filas.Close()
	var out []int64
	for filas.Next() {
		var id int64
		if err := filas.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// stockDisponible es lo que hay en los lotes, que es lo que se puede vender.
//
// Se lee de `product_lot` y NO de `product_information`, porque el ERP mantiene
// dos verdades de stock: la del histórico de compras y ventas, y la de los lotes.
// Cuando se sembró sin lotes, la segunda sale en cero. Es el mismo par que
// comprueba `compruebaEsCuadre` en el arnés de pruebas.
func stockDisponible(db *sql.DB, producto string) float64 {
	var q float64
	_ = db.QueryRow(`SELECT COALESCE(SUM(quantity_available),0) FROM product_lot
		WHERE product_id = ? AND quantity_available > 0`, mustEntero(producto)).Scan(&q)
	return q
}

// stockVigente es lo que el FEFO PODRÍA servir en una fecha dada: suma solo los
// lotes que a esa fecha no han caducado.
//
// POR QUÉ EXISTE SI YA ESTÁ stockDisponible. Son la misma pregunta si y solo si
// el stock se interroga contra «hoy». Con datos históricos se desdoblaron dos
// verdades para la misma decisión: comprobarStock contaba también lotes ya
// vencidos y dejaba llegar la venta hasta RepartirFEFO, que se negó con
// «faltan N» (mes 2026-11, pollo). Una sola verdad: lo servible es la suma de
// lotes vigentes, y el chequeo y el reparto la comparten.
func stockVigente(s *Server, producto string, fecha string) float64 {
	asOf, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return 0
	}
	lotes, err := s.LotesActivos(nil, int64(mustEntero(producto)))
	if err != nil {
		return 0
	}
	var hay float64
	for _, l := range lotes {
		if l.DaysLeft(asOf) >= 0 {
			hay += l.QtyAvailable
		}
	}
	return hay
}

// comprobarStock dice si el FEFO podría servir esa cantidad a esa fecha.
//
// Si no hay, el vendedor no entrega y la venta no se registra: es lo que pasa
// en una carnicería de verdad (hay meses sin pollo), y es preferible a vender
// mercadería que no existe. La fecha es la de la venta, no la del servidor: lo
// mismo decide el reparto que ejecuta después.
func comprobarStock(s *Server, producto, fecha string, cantidad float64) error {
	hay := stockVigente(s, producto, fecha)
	if cantidad > hay+0.009 {
		return fmt.Errorf("stock insuficiente de %s a %s: hay %.2f servibles y se piden %.2f",
			producto, fecha, hay, cantidad)
	}
	return nil
}

// precioMayor lee el precio de `product_price_list` para la lista mayor.
//
// Si no hay precio mayor, NO cae al de mostrador: devuelve 0 y el mes se queda
// sin esas ventas. Caer al de mostrador haría que la lista mayor pareciera
// funcionar cuando en realidad no está leída, que es el fallo que el mes 2 del
// ciclo económico existe para detectar.
func precioMayor(db *sql.DB, producto string) float64 {
	var v any
	err := db.QueryRow(`SELECT pl.price FROM product_price_list pl
		JOIN price_list l ON l.id = pl.price_list_id
		WHERE pl.product_id = ? AND l.code = 'mayor'`, producto).Scan(&v)
	if err != nil {
		return 0
	}
	if f := valorNumerico(v); f != nil {
		return *f
	}
	return 0
}

// precioDetalle lee el precio de mostrador de `product_information.price`.
func precioDetalle(db *sql.DB, producto string) float64 {
	var v any
	if err := db.QueryRow(`SELECT price FROM product_information WHERE product_id = ?`,
		producto).Scan(&v); err != nil {
		return 0
	}
	if f := valorNumerico(v); f != nil {
		return *f
	}
	return 0
}
