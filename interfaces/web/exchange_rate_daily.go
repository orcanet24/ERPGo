package web

// exchange_rate.go — Tipo de cambio BCV / paralelo.
//
// POR QUÉ ESTO ES UN MÓDULO Y NO UN CAMPO.
//
// Es un dato por día, pero de él dependen tres cosas que se rompen de forma
// distinta si se guardan juntas:
//
//   * La TASA con la que se convierte una factura. Se elige una: la oficial o la
//     del mercado. Cambiarla después no debe tocar las facturas ya emitidas,
//     porque esas ya se facturaron en la otra tasa.
//   * La DIFERENCIA entre lo que se pagó y lo que se facturó. El proveedor cobra
//     al paralelo, la venta se factura al BCV, y ese hueco es la pérdida que se
//     lleva la diferencia entre el costo y el ingreso. Si solo se guarda una
//     tasa, esa pérdida no aparece en ningún sitio.
//   * La PROCEDENCIA. Un bolívar de fuente distinta cambia las facturas nuevas,
//     no las viejas, y hace falta saber quién lo puso.
//
// POR QUÉ LA TASA PARALELA SE GUARDA APARTE Y NO SE MEZCLA CON LA OFICIAL.
//
// Porque la oficial es la que da el fisco y la paralela es la que da el mercado,
// y mezclarlas produce un número que no es ni lo uno ni lo otro. Aquí conviven
// en la misma tabla pero con `is_parallel` que dice cuál es cuál, y el módulo se
// niega a usarlas intercambiadas: facturar al paralelo por descuido es el peor
// fallo posible de esta pantalla.

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// claveFuenteTasa guarda en `fiscal_setting` qué tasa se usa para facturar.
const claveFuenteTasa = "exchange_rate_source"

// TasaDia es un dato de tipo de cambio de un día.
type TasaDia struct {
	Fecha      string  `json:"fecha"`
	Moneda     string  `json:"moneda"`
	Fuente     string  `json:"fuente"`
	Compra     float64 `json:"compra"`
	Venta      float64 `json:"venta"`
	Oficial    float64 `json:"oficial"`
	EsParalelo bool    `json:"es_paralelo"`
	Usuario    string  `json:"usuario"`
	Registrado string  `json:"registrado"`
}

// DiferenciaTasa es una fila de la comparación BCV contra paralelo.
type DiferenciaTasa struct {
	Fecha      string  `json:"fecha"`
	Moneda     string  `json:"moneda"`
	Oficial    float64 `json:"oficial"`
	Paralelo   float64 `json:"paralelo"`
	Diferencia float64 `json:"diferencia"`
	Porcentaje float64 `json:"porcentaje"`
}

// registrarTasa escribe la tasa en la tabla nueva y refleja la oficial en la
// tabla vieja.
//
// El reflejo NO es opcional. La tabla `exchange_rate` del ERP original la leen
// los informes de IGTF y de diferencia cambiaria para saber a qué tasa se emitió
// cada factura. Si esta pantalla escribiera solo en la tabla nueva, el usuario
// vería su tasa y los informes seguirían viendo la de hace seis meses: dos
// verdades en la misma aplicación, y el que se equivoca es el fisco.
//
// Va en transacción: si el reflejo falla, la tasa nueva tampoco queda, porque una
// tasa a medio escribir es peor que ninguna.
func (s *Server) registrarTasa(r *http.Request, t TasaDia) error {
	if t.Moneda == "" {
		t.Moneda = "USD"
	}
	if t.Venta <= 0 {
		t.Venta = t.Compra
	}
	t.Fuente = strings.TrimSpace(t.Fuente)
	if t.Fuente == "" {
		t.Fuente = "BCV"
	}
	paralelo := 0
	if t.EsParalelo || strings.EqualFold(t.Fuente, "paralelo") {
		paralelo = 1
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("no se pudo iniciar la escritura de la tasa: %w", err)
	}
	defer tx.Rollback()

	// La tasa oficial del día se guarda en la fila de la fuente que se está
	// registrando solo cuando esa fuente ES la oficial. Si se está metiendo la
	// paralela, se rellena con la oficial que ya exista para ese día, para que
	// la comparación tenga las dos columnas sin tener que volver a la tabla.
	oficial := t.Oficial
	if paralelo == 0 {
		oficial = t.Compra
	} else if oficial <= 0 {
		var oficialGuardada float64
		if err := tx.QueryRow(`
			SELECT COALESCE(MAX(official_rate), 0) FROM exchange_rate_daily
			WHERE rate_date = ? AND currency = ?`, t.Fecha, t.Moneda).Scan(&oficialGuardada); err == nil {
			oficial = oficialGuardada
		}
	}
	if oficial <= 0 && paralelo == 1 {
		var bcv float64
		if err := tx.QueryRow(`
			SELECT COALESCE(MAX(buy_rate), 0) FROM exchange_rate_daily
			WHERE rate_date = ? AND currency = ? AND is_parallel = 0`, t.Fecha, t.Moneda).Scan(&bcv); err == nil {
			oficial = bcv
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO exchange_rate_daily
			(rate_date, currency, source, buy_rate, sell_rate, official_rate,
			 is_parallel, entered_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			buy_rate     = VALUES(buy_rate),
			sell_rate    = VALUES(sell_rate),
			official_rate= VALUES(official_rate),
			is_parallel  = VALUES(is_parallel),
			entered_by   = VALUES(entered_by)`,
		t.Fecha, t.Moneda, t.Fuente, t.Compra, t.Venta, oficial, paralelo,
		t.Usuario, now()); err != nil {
		return fmt.Errorf("no se pudo guardar la tasa: %w", err)
	}

	if paralelo == 0 {
		if _, err := tx.Exec(`
			INSERT INTO exchange_rate (date, currency, rate_bcv, source, created_at)
			VALUES (?,?,?,?,?)
			ON DUPLICATE KEY UPDATE rate_bcv = VALUES(rate_bcv), source = VALUES(source)`,
			t.Fecha, t.Moneda, t.Compra, t.Fuente, now()); err != nil {
			return fmt.Errorf("no se pudo actualizar la tasa BCV del ERP: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("no se pudo confirmar la tasa: %w", err)
	}
	return nil
}

// RegistrarTasaBCV y RegistrarTasaParalelo son las dos entradas de la pantalla.
//
// Se separan en vez de tener un `RegistrarTasa` con un booleano porque el
// nombre es lo que evita el error: `es_paralelo` es un entero en la base y un
// checkbox en pantalla, y los dos se pueden mandar al revés sin querer.
func (s *Server) RegistrarTasaBCV(r *http.Request, fecha, moneda string, compra float64, usuario string) error {
	if err := validarFechaTasa(fecha); err != nil {
		return err
	}
	if compra <= 0 {
		return fmt.Errorf("la tasa oficial debe ser mayor que cero")
	}
	return s.registrarTasa(r, TasaDia{
		Fecha: fecha, Moneda: moneda, Fuente: "BCV",
		Compra: compra, Oficial: compra, Usuario: usuario,
	})
}

func (s *Server) RegistrarTasaParalelo(r *http.Request, fecha, moneda string, compra float64, usuario string) error {
	if err := validarFechaTasa(fecha); err != nil {
		return err
	}
	if compra <= 0 {
		return fmt.Errorf("la tasa paralela debe ser mayor que cero")
	}
	return s.registrarTasa(r, TasaDia{
		Fecha: fecha, Moneda: moneda, Fuente: "paralelo",
		Compra: compra, EsParalelo: true, Usuario: usuario,
	})
}

// validarFechaTasa exige una fecha real y no una cadena cualquiera.
//
// La columna es DATE y MySQL en modo NO_ZERO_DATE rechaza '0000-00-00', pero
// acepta '2026-02-31' cambiándola por '2026-03-03' en silencio. Una tasa que
// cambia de día sin avisar es una tasa mal puesta con fecha buena.
func validarFechaTasa(fecha string) error {
	fecha = strings.TrimSpace(fecha)
	if fecha == "" {
		return fmt.Errorf("indique la fecha de la tasa")
	}
	if _, err := time.Parse("2006-01-02", fecha); err != nil {
		return fmt.Errorf("la fecha %q no es válida; se espera AAAA-MM-DD", fecha)
	}
	return nil
}

// TasaDelDia devuelve la tasa exacta de esa fecha y fuente.
//
// Si no hay tasa ese día, devuelve error en vez de devolver la anterior. Aquí no
// se rellena el hueco a conveniencia: una tasa del lunes usada para un sábado es
// un número inventado, y quien la usa no tiene forma de saber que se inventó.
func (s *Server) TasaDelDia(r *http.Request, fecha, moneda string, paralela bool) (float64, error) {
	if moneda == "" {
		moneda = "USD"
	}
	flag := 0
	if paralela {
		flag = 1
	}
	var tasa float64
	err := s.dbFor(r).QueryRow(`
		SELECT COALESCE(buy_rate, 0) FROM exchange_rate_daily
		WHERE rate_date = ? AND currency = ? AND is_parallel = ?`, fecha, moneda, flag).Scan(&tasa)
	if err != nil {
		return 0, fmt.Errorf("no hay tasa para %s %s (%s): %w", fecha, moneda, etiquetaFuente(flag), err)
	}
	if tasa <= 0 {
		return 0, fmt.Errorf("la tasa de %s %s (%s) está en cero", fecha, moneda, etiquetaFuente(flag))
	}
	return tasa, nil
}

// TasaVigente devuelve la tasa que se usa para convertir en esa fecha.
//
// Coge la fuente configurada en `fiscal_setting`, y si ese día no hay tasa de
// esa fuente, baja por la fecha hasta la más reciente que exista. Aquí sí se
// rellena el hueco, y por una razón concreta: facturar depende de una tasa y no
// puede quedarse sin facturar porque un lunes no se publicó el bolívar. Se deja
// constancia de la fecha real de la tasa usada para que la diferencia sea
// auditable.
func (s *Server) TasaVigente(r *http.Request, fecha, moneda string) (tasa float64, fechaReal string, fuente string, err error) {
	if moneda == "" {
		moneda = "USD"
	}
	flag := 0
	fuente = s.FuenteTasaFacturacion(r)
	if fuente == "paralelo" {
		flag = 1
	}
	err = s.dbFor(r).QueryRow(`
		SELECT rate_date, buy_rate FROM exchange_rate_daily
		WHERE rate_date <= ? AND currency = ? AND is_parallel = ? AND buy_rate > 0
		ORDER BY rate_date DESC LIMIT 1`, fecha, moneda, flag).Scan(&fechaReal, &tasa)
	if err != nil || tasa <= 0 {
		// Sin la fuente configurada se cae a la oficial, y si tampoco hay,
		// a la paralela. Se avisa de cuál se usó: facturar al paralelo sin
		// querer es exactamente el fallo que este módulo quiere evitar.
		otro := 1 - flag
		if err2 := s.dbFor(r).QueryRow(`
			SELECT rate_date, buy_rate FROM exchange_rate_daily
			WHERE rate_date <= ? AND currency = ? AND is_parallel = ? AND buy_rate > 0
			ORDER BY rate_date DESC LIMIT 1`, fecha, moneda, otro).Scan(&fechaReal, &tasa); err2 != nil || tasa <= 0 {
			return 0, "", "", fmt.Errorf("no hay ninguna tasa registrada para %s antes del %s", moneda, fecha)
		}
		fuente = etiquetaFuente(otro)
	}
	return tasa, fechaReal, fuente, nil
}

func etiquetaFuente(paralelo int) string {
	if paralelo == 1 {
		return "paralelo"
	}
	return "BCV"
}

// nombreDeUsuario devuelve quién está tocando la pantalla.
//
// Va al principio del nombre de la columna `entered_by`, que es donde va el
// nombre real y no el identificador: dentro de un año, "¿quién puso esta tasa?"
// se responde leyendo una fila, no reconstruyendo una tabla de usuarios.
func nombreDeUsuario(s *Server, r *http.Request) string {
	if sess := s.sessionFromRequest(r); sess != nil && sess.Username != "" {
		return sess.Username
	}
	return "sistema"
}

// FuenteTasaFacturacion dice qué fuente se usa para facturar.
func (s *Server) FuenteTasaFacturacion(r *http.Request) string {
	var v string
	err := s.dbFor(r).QueryRow(
		"SELECT value FROM fiscal_setting WHERE `key` = ?", claveFuenteTasa).Scan(&v)
	if err == nil {
		if v == "paralelo" {
			return "paralelo"
		}
		if v != "" {
			return "BCV"
		}
	}
	return "BCV"
}

// FijarFuenteTasaFacturacion elige con qué fuente se factura.
//
// Guarda el nombre de la fuente, no un 0/1. En la base queda escrito "BCV" o
// "paralelo", y dentro de seis meses el que lea la fila sabe qué se decidió
// sin tener que acordarse del convenio.
func (s *Server) FijarFuenteTasaFacturacion(r *http.Request, fuente string) error {
	if fuente != "BCV" && fuente != "paralelo" {
		return fmt.Errorf("fuente de tasa no válida: use BCV o paralelo")
	}
	return s.upsertFiscalSetting(s.dbFor(r), claveFuenteTasa, fuente)
}

// DiferenciaParalelo compara las dos tasas día a día.
//
// Devuelve las filas y dos promedios ponderados por días: el promedio simple de
// porcentajes miente cuando hay un día con tasa absurda, y la pérdida real del
// mes es la del período entero, no la media de los porcentajes.
func (s *Server) DiferenciaParalelo(r *http.Request, desde, hasta, moneda string) ([]DiferenciaTasa, float64, float64, error) {
	if moneda == "" {
		moneda = "USD"
	}
	if desde == "" || hasta == "" {
		return nil, 0, 0, fmt.Errorf("indique el rango de fechas")
	}
	filas, err := s.dbFor(r).Query(`
		SELECT o.rate_date,
		       COALESCE(o.buy_rate, 0),
		       COALESCE(p.buy_rate, 0)
		FROM exchange_rate_daily o
		LEFT JOIN exchange_rate_daily p
		       ON p.rate_date = o.rate_date AND p.currency = o.currency AND p.is_parallel = 1
		WHERE o.currency = ? AND o.is_parallel = 0 AND o.rate_date BETWEEN ? AND ?
		ORDER BY o.rate_date ASC`, moneda, desde, hasta)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("no se pudo comparar las tasas: %w", err)
	}
	defer filas.Close()

	var lista []DiferenciaTasa
	var sumaOficial, sumaParalelo float64
	var dias float64
	for filas.Next() {
		var f DiferenciaTasa
		var fecha time.Time
		if err := filas.Scan(&fecha, &f.Oficial, &f.Paralelo); err != nil {
			continue
		}
		f.Fecha = fecha.Format("2006-01-02")
		f.Moneda = moneda
		if f.Paralelo <= 0 {
			// Sin dato paralelo ese día no hay comparación posible. Poner la
			// oficial como paralela daría diferencia cero, que es el peor
			// caso: un hueco del 20 % se mostraría como un día perfecto.
			continue
		}
		f.Diferencia = f.Paralelo - f.Oficial
		if f.Oficial > 0 {
			f.Porcentaje = f.Diferencia / f.Oficial * 100
		}
		sumaOficial += f.Oficial
		sumaParalelo += f.Paralelo
		dias++
		lista = append(lista, f)
	}
	if err := filas.Err(); err != nil {
		return nil, 0, 0, err
	}
	promOficial, promParalelo := 0.0, 0.0
	if dias > 0 {
		promOficial = sumaOficial / dias
		promParalelo = sumaParalelo / dias
	}
	return lista, promOficial, promParalelo, nil
}

// CostoDeConversion dice cuántos bolívares cuesta hoy un monto en FOREIGN.
//
// Es el número con el que se mide el riesgo cambiario: si compras al paralelo y
// facturas al BCV, lo que aparece en esta función es lo que se está perdiendo en
// cada factura.
func (s *Server) CostoDeConversion(r *http.Request, monto float64, moneda, fecha string) (oficial, paralelo, diferencia float64, err error) {
	tasaOF, _, fuenteOF, err := s.TasaVigente(r, fecha, moneda)
	if err != nil {
		return 0, 0, 0, err
	}
	oficial = monto * tasaOF
	if fuenteOF == "paralelo" {
		// La vigente ya es la paralela: no hay oficial contra la que comparar
		// y ponerla igual daría una diferencia de cero, que es exactamente lo
		// que esta función existe para no decir.
		return oficial, oficial, 0, nil
	}
	tasaPar := 0.0
	err = s.dbFor(r).QueryRow(`
		SELECT COALESCE(buy_rate, 0) FROM exchange_rate_daily
		WHERE rate_date <= ? AND currency = ? AND is_parallel = 1 AND buy_rate > 0
		ORDER BY rate_date DESC LIMIT 1`, fecha, moneda).Scan(&tasaPar)
	if err != nil || tasaPar <= 0 {
		return oficial, 0, 0, nil
	}
	paralelo = monto * tasaPar
	return oficial, paralelo, paralelo - oficial, nil
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------
//
// `tasaData` y `tasaDiferencia` devuelven los mismos números que muestra el
// informe, calculados con la misma función. Una pantalla que calcula una cosa y
// un informe que calcula otra es una pantalla que miente cuando alguien los
// compara, y comparar los dos es lo primero que hace cualquiera que desconfíe.

// registerTasaRoutes registra las rutas del módulo de tipo de cambio BCV/paralelo.
func (s *Server) registerTasaRoutes() {
	s.mux.HandleFunc("GET /settings/tasas", s.tasaPage)
	s.mux.HandleFunc("GET /settings/tasas/data", s.tasaData)
	s.mux.HandleFunc("POST /settings/tasas/bcv", s.tasaSaveBCV)
	s.mux.HandleFunc("POST /settings/tasas/paralelo", s.tasaSaveParalelo)
	s.mux.HandleFunc("POST /settings/tasas/fuente", s.tasaSaveFuente)
	s.mux.HandleFunc("GET /settings/tasas/bcv/fetch", s.tasaFetchBCV)
	s.mux.HandleFunc("GET /reports/diferencia-cambio", s.tasaDiferencia)
}

func (s *Server) tasaPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "exchange_rate_paralelo.html", map[string]interface{}{
		"Title":    "Tipo de cambio BCV / paralelo",
		"Currency": s.settingsFor(r).Web.Currency,
		"Fuente":   s.FuenteTasaFacturacion(r),
	})
}

func (s *Server) tasaData(w http.ResponseWriter, r *http.Request) {
	dias, _ := strconv.Atoi(r.FormValue("dias"))
	if dias <= 0 {
		dias = 30
	}
	moneda := orDefault(r.FormValue("currency"), "USD")
	hasta := time.Now().Format("2006-01-02")
	desde := time.Now().AddDate(0, 0, -dias).Format("2006-01-02")

	tasas, promOF, promPar, err := s.DiferenciaParalelo(r, desde, hasta, moneda)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	oficial, fechaOF, fuenteOF, err := s.TasaVigente(r, hasta, moneda)
	if err != nil {
		oficial, fechaOF, fuenteOF = 0, "", ""
	}
	writeJSON(w, map[string]interface{}{
		"data":           tasas,
		"dias":           dias,
		"moneda":         moneda,
		"prom_oficial":   promOF,
		"prom_paralelo":  promPar,
		"vigente":        oficial,
		"vigente_fecha":  fechaOF,
		"vigente_fuente": fuenteOF,
		"facturacion":    s.FuenteTasaFacturacion(r),
	})
}

func (s *Server) tasaSaveBCV(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "settings", "edit") {
		jsonError(w, "No tiene permiso para registrar la tasa", 403)
		return
	}
	tasa, _ := strconv.ParseFloat(r.FormValue("rate"), 64)
	usuario := nombreDeUsuario(s, r)
	err := s.RegistrarTasaBCV(r, r.FormValue("date"),
		orDefault(r.FormValue("currency"), "USD"), tasa, usuario)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tasa BCV registrada", "redirect": saveRedirect(r, "/settings/tasas")})
}

func (s *Server) tasaSaveParalelo(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "settings", "edit") {
		jsonError(w, "No tiene permiso para registrar la tasa", 403)
		return
	}
	tasa, _ := strconv.ParseFloat(r.FormValue("rate"), 64)
	usuario := nombreDeUsuario(s, r)
	err := s.RegistrarTasaParalelo(r, r.FormValue("date"),
		orDefault(r.FormValue("currency"), "USD"), tasa, usuario)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Tasa paralela registrada", "redirect": saveRedirect(r, "/settings/tasas")})
}

func (s *Server) tasaSaveFuente(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "settings", "edit") {
		jsonError(w, "No tiene permiso para cambiar la fuente de tasa", 403)
		return
	}
	if err := s.FijarFuenteTasaFacturacion(r, r.FormValue("source")); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Fuente de facturación actualizada", "redirect": saveRedirect(r, "/settings/tasas")})
}

// tasaFetchBCV obtiene la tasa oficial del BCV desde https://www.bcv.org.ve/
// usando el XPath //*[@id="dolar"]/div/div/div[2]/strong y devuelve el valor
// con 3 decimales.
func (s *Server) tasaFetchBCV(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "settings", "view") {
		jsonError(w, "No tiene permiso para consultar el BCV", 403)
		return
	}
	rate, err := fetchBCVRate()
	if err != nil {
		jsonError(w, fmt.Sprintf("Error al obtener tasa del BCV: %v", err), 502)
		return
	}
	fecha := time.Now().Format("2006-01-02")
	writeJSON(w, map[string]interface{}{
		"status":    1,
		"rate":      rate,
		"date":      fecha,
		"formatted": strconv.FormatFloat(rate, 'f', 3, 64),
	})
}

// fetchBCVRate hace GET a https://www.bcv.org.ve/ y extrae el valor del
// <strong class="strong-tb"> dentro de #dolar, que contiene la tasa oficial
// del dólar publicada por el Banco Central de Venezuela.
func fetchBCVRate() (float64, error) {
	// El BCV a veces sirve un certificado no en la CA bundle del sistema.
	// Como es una fuente oficial conocida, toleramos el error de verificación.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	req, err := http.NewRequest("GET", "https://www.bcv.org.ve/", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ERPGo/1.0)")
	req.Header.Set("Accept-Language", "es-VE,es;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("BCV respondió con código %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// El BCV publica el dólar en: <div id="dolar"> ... <strong>123,456</strong>
	// Navegamos el DOM para encontrar #dolar → div → div → div[2] → strong.
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return 0, fmt.Errorf("no se pudo parsear el HTML del BCV: %w", err)
	}

	val := findBCVDolarRate(doc)
	if val == "" {
		return 0, fmt.Errorf("no se encontró el elemento #dolar/div/div/div[2]/strong en la página del BCV")
	}

	val = strings.ReplaceAll(val, ",", ".")
	rate, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return 0, fmt.Errorf("el valor del BCV '%s' no es numérico: %w", val, err)
	}

	return rate, nil
}

// findBCVDolarRate busca el <strong> dentro de #dolar/div/div/div[2]
// que contiene la tasa oficial del dólar del BCV.
// XPath equivalente: //*[@id="dolar"]/div/div/div[2]/strong
func findBCVDolarRate(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "div" {
		id := getAttr(n, "id")
		if id == "dolar" {
			// Navegar: div(id=dolar) → div[1] → div[1] → div[2] → strong
			return extractFromDolarXPath(n)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if v := findBCVDolarRate(c); v != "" {
			return v
		}
	}
	return ""
}

// extractFromDolarXPath navega la estructura #dolar/div/div/div[2]/strong.
func extractFromDolarXPath(dolarNode *html.Node) string {
	// dolarNode es <div id="dolar">
	// Paso 1: primer div hijo de #dolar (div[1] en XPath, índice 0)
	for c := dolarNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "div" {
			// Paso 2: primer div hijo de este div (div[1])
			for c2 := c.FirstChild; c2 != nil; c2 = c2.NextSibling {
				if c2.Type == html.ElementNode && c2.Data == "div" {
					// Paso 3: segundo div hijo de este div (div[2] en XPath, índice 1)
					var divs []*html.Node
					for gc := c2.FirstChild; gc != nil; gc = gc.NextSibling {
						if gc.Type == html.ElementNode && gc.Data == "div" {
							divs = append(divs, gc)
						}
					}
					if len(divs) >= 2 {
						// divs[1] es el segundo div → buscar <strong> dentro
						for sc := divs[1].FirstChild; sc != nil; sc = sc.NextSibling {
							if sc.Type == html.ElementNode && sc.Data == "strong" {
								return getNodeText(sc)
							}
						}
					}
					break
				}
			}
			break
		}
	}
	return ""
}

// findDolarStrong busca el <strong> dentro de #dolar/div/div/div[2] en el árbol HTML.
func findDolarStrong(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "div" {
		id := getAttr(n, "id")
		if id == "dolar" {
			// Navegar: div(id=dolar) → div → div → div[2] → strong
			return extractStrongFromDolar(n)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if v := findDolarStrong(c); v != "" {
			return v
		}
	}
	return ""
}

// extractStrongFromDolar navega la estructura #dolar/div/div/div[2]/strong.
func extractStrongFromDolar(dolarNode *html.Node) string {
	// dolarNode es <div id="dolar">; tomamos el primer <div> hijo
	for c := dolarNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "div" {
			// c es el primer div → ahora buscamos div dentro, luego div[2] dentro, luego strong
			return extractStrongFromPath(c, []int{0, 1, 2})
		}
	}
	return ""
}

// extractStrongFromPath navega por los índices de DIV dados y extrae el <strong>.
func extractStrongFromPath(n *html.Node, path []int) string {
	current := n
	for _, idx := range path {
		var divs []*html.Node
		for c := current.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "div" {
				divs = append(divs, c)
			}
		}
		if idx >= len(divs) {
			return ""
		}
		current = divs[idx]
	}
	// En current buscamos un <strong> directo
	for c := current.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "strong" {
			return getNodeText(c)
		}
	}
	return ""
}

// getAttr devuelve el valor de un atributo del nodo.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// getNodeText extrae el texto plano de un nodo.
func getNodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(getNodeText(c))
	}
	return sb.String()
}

func (s *Server) tasaDiferencia(w http.ResponseWriter, r *http.Request) {
	moneda := orDefault(r.FormValue("currency"), "USD")
	desde := orDefault(r.FormValue("from"), time.Now().AddDate(0, 0, -30).Format("2006-01-02"))
	hasta := orDefault(r.FormValue("to"), time.Now().Format("2006-01-02"))
	monto, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	filas, promOF, promPar, err := s.DiferenciaParalelo(r, desde, hasta, moneda)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	pago, cobro, dif, _ := s.CostoDeConversion(r, monto, moneda, hasta)
	s.render(w, r, "tasa_diferencia.html", map[string]interface{}{
		"Title":        "Diferencia de cambio",
		"Rows":         filas,
		"PromOficial":  promOF,
		"PromParalelo": promPar,
		"Moneda":       moneda,
		"Desde":        desde,
		"Hasta":        hasta,
		"Monto":        monto,
		"Paga":         pago,
		"Cobra":        cobro,
		"Diferencia":   dif,
	})
}
