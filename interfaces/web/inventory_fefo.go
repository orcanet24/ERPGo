package web

// inventory_fefo.go — Lotes, FEFO y alertas de caducidad.
//
// EL PROBLEMA QUE RESUELVE, Y POR QUÉ NO ES "ORDENAR POR FECHA".
//
// En un ERPY cárnico el 80 % del valor está en productos que se estropean. Un
// lote de carne que caduca en tres días vale hoy lo mismo que uno que caduca en
// tres meses, y dentro de tres días el segundo vale cero y el primero todavía
// no. Si el sistema no sabe qué lote se está consumiendo, la merma no se ve
// hasta que llega el inventario, y para entonces ya es dinero perdido.
//
// FEFO es "First Expired, First Out": sale primero lo que caduca antes. Es una
// diferencia de unos 200 bolívares por kilo en el proveedor y de cero en el ERP,
// así que el ahorro es real, no teórico.
//
// LO QUE NO HACE ESTE ARCHIVO.
//
// No toca la factura. Devuelve QUÉ lote se consume para una cantidad pedida, y
// es el flujo de venta quien lo aplica dentro de su transacción. La razón es
// que el consumo de stock y la cabecera de factura tienen que committing en la
// MISMA transacción: si se confirmaran por separado, una factura emitida sin
// stock es una promesa que el almacén no puede cumplir, y esa promesa se
// descubre en el momento de la entrega, con el cliente delante.

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Lot es una partida de producto con fecha de entrada y de caducidad.
type Lot struct {
	ID           int64
	ProductID    int64
	LotCode      string
	EntryDate    string
	ExpiryDate   string // "" si no caduca
	QtyReceived  float64
	QtyAvailable float64
	UnitCost     float64
	SupplierID   int64
	PurchaseID   int64
	Status       string
}

// DaysLeft devuelve los días que faltan para caducar. Negativo = ya caducó.
// Sin fecha de caducidad devuelve un número grande a propósito: un producto que
// no caduca nunca debe ir antes que uno que caduca mañana, y FEFO tiene que
// respectar ese orden.
func (l Lot) DaysLeft(asOf time.Time) int {
	if l.ExpiryDate == "" {
		return 999999
	}
	c, err := time.Parse("2006-01-02", l.ExpiryDate)
	if err != nil {
		return 999999
	}
	return int(c.Sub(asOf).Hours() / 24)
}

// LotAlta es un lote nuevo, tal y como lo crea el usuario o la recepción.
type LotAlta struct {
	ProductID   int64
	LotCode     string
	EntryDate   string
	ExpiryDate  string
	QtyReceived float64
	UnitCost    float64
	SupplierID  int64
	PurchaseID  int64
}

// RegistrarLote da de alta un lote, o acumula en uno existente.
//
// ACUMULAR Y NO DUPLICAR, con un motivo concreto. La misma partida puede
// llegar en dos recepciones parciales del mismo lote (el proveedor reparte la
// entrega). Si cada una creara una fila, FEFO vería dos filas del mismo lote y
// sumaría la cantidad dos veces al ordenar. Por eso hay un índice único en
// (product_id, lot_code) y esta función hace lo que dice.
//
// La caducidad NO se pisa si el lote ya existe: la primera entrada es la que
// conoce la fecha real de caducidad; una recepción posterior con la fecha en
// blanco no significa "caduca antes".
//
// LA COMPARACIÓN DE LA FECHA VA CON CAST, Y NO ES COSMÉTICO.
//
// Escribir `expiry_date = ”` en una columna DATE obliga a MySQL a convertir
// la cadena vacía en fecha, y con STRICT_TRANS_TABLES eso es Error 1292
// "Incorrect date value: ”": el INSERT entero se rechaza. Descubrir esto
// corriendo el INSERT a mano es fácil; lo que cuesta es entender por qué el
// error señala `expiry_date` cuando la fecha que se manda es correcta, cuando
// en realidad la que se está convirtiendo a fecha es la cadena vacía del
// comparador. Por eso se castea a CHAR: la comparación pasa a ser entre
// textos y MySQL no toca la columna.
func (s *Server) RegistrarLote(r *http.Request, alta LotAlta) (int64, error) {
	db := s.dbFor(r)
	if alta.ProductID == 0 {
		return 0, fmt.Errorf("el lote necesita un producto")
	}
	if alta.LotCode == "" {
		return 0, fmt.Errorf("el lote necesita un código")
	}
	entrada := alta.EntryDate
	if entrada == "" {
		entrada = time.Now().Format("2006-01-02")
	}
	if alta.ExpiryDate != "" {
		if _, err := time.Parse("2006-01-02", alta.ExpiryDate); err != nil {
			return 0, fmt.Errorf("la fecha de caducidad %q no es una fecha", alta.ExpiryDate)
		}
	}
	codigo := strings.TrimSpace(alta.LotCode)

	_, err := db.Exec(`
		INSERT INTO product_lot
			(product_id, lot_code, entry_date, expiry_date, quantity_received,
			 quantity_available, unit_cost, supplier_id, purchase_id, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,'active',?)
		ON DUPLICATE KEY UPDATE
			quantity_received = quantity_received + VALUES(quantity_received),
			quantity_available = quantity_available + VALUES(quantity_available),
			unit_cost         = IF(VALUES(unit_cost) > 0, VALUES(unit_cost), unit_cost),
			supplier_id       = IFNULL(VALUES(supplier_id), supplier_id),
			purchase_id       = IFNULL(VALUES(purchase_id), purchase_id),
		expiry_date       = IF(COALESCE(CAST(expiry_date AS CHAR), '') = '',
		                       VALUES(expiry_date), expiry_date),
			status            = 'active'`,
		alta.ProductID, codigo, entrada, nullSiVacio(alta.ExpiryDate),
		alta.QtyReceived, alta.QtyReceived, alta.UnitCost,
		nullSiCero(alta.SupplierID), nullSiCero(alta.PurchaseID), now())
	if err != nil {
		return 0, fmt.Errorf("no se pudo registrar el lote: %w", err)
	}

	var id int64
	if err := db.QueryRow(
		`SELECT id FROM product_lot WHERE product_id = ? AND lot_code = ?`,
		alta.ProductID, codigo).Scan(&id); err != nil {
		return 0, fmt.Errorf("el lote se escribió pero no se encuentra: %w", err)
	}
	return id, nil
}

func nullSiVacio(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// fechaBD convierte en "AAAA-MM-DD" lo que devuelve el servidor para una
// columna DATE.
//
// LA TRAMPA, Y POR QUÉ ESTE HELPER EXISTE.
//
// La conexión abre con `parseTime=true` (necesario: una columna DATETIME
// escaneada a `time.Time` revienta si no). La consecuencia colateral es que
// una columna DATE también vuelve como `time.Time`, y escanearla a `string`
// produce "2026-10-08T00:00:00Z" en vez de "2026-10-08".
//
// A partir de ahí, el `time.Parse("2006-01-02", x)` de la línea siguiente
// falla, el código hace `continue` y la fila DESAPARECE de la lista sin que
// salte ningún error. En la pantalla de caducidades eso se ve como "no hay
// lotes por caducar" con la cámara llena de lote viejo: el fallo más caro que
// puede tener un aviso de vencimiento, porque el sistema está diciendo que
// todo está bien.
//
// Por eso toda fecha que venga de la base pasa por aquí, y no por un Scan a
// string.
func fechaBD(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	case []byte:
		return recortarFecha(string(t))
	case string:
		return recortarFecha(t)
	}
	return ""
}

func recortarFecha(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02")
	}
	for _, layout := range []string{
		time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func nullSiCero(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// LotesActivos devuelve los lotes con existencias de un producto, ORDENADOS POR
// FEFO.
//
// El orden es: primero los que ya caducaron (que hay que rotar o Tirar, no
// vender), luego los que caducan antes, y al final los que no caducan.
//
// Es un detalle que seVe en el resultado: ordenar solo por `expiry_date` deja
// los NULL al final en MySQL, que ya es lo correcto, pero un lote caducado con
// fecha pasada queda "antes" que uno vigente y se consumiría primero. Un lote
// caducado no se vende: se separa. Por eso el primer criterio es `expiry_date <
// hoy`, no un simple ORDER BY.
func (s *Server) LotesActivos(r *http.Request, productID int64) ([]Lot, error) {
	filas, err := s.dbFor(r).Query(`
		SELECT id, product_id, lot_code, entry_date, COALESCE(expiry_date, ''),
		       quantity_received, quantity_available, unit_cost,
		       COALESCE(supplier_id, 0), COALESCE(purchase_id, 0), status
		FROM product_lot
		WHERE product_id = ? AND quantity_available > 0
		  AND status IN ('active', 'expired')
		ORDER BY expiry_date IS NULL, expiry_date ASC, id ASC`, productID)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer los lotes: %w", err)
	}
	defer filas.Close()

	asOf := time.Now()
	var lotes []Lot
	for filas.Next() {
		var l Lot
		if err := filas.Scan(&l.ID, &l.ProductID, &l.LotCode, &l.EntryDate,
			&l.ExpiryDate, &l.QtyReceived, &l.QtyAvailable, &l.UnitCost,
			&l.SupplierID, &l.PurchaseID, &l.Status); err != nil {
			continue
		}
		// Un lote caducado se marca como tal al leerlo. Es una escritura en una
		// lectura, y parece poco elegante, pero evita que la pantalla de alertas
		// tenga que volver a calcular la caducidad cada vez y que dos vistas den
		// respuestas distintas el mismo día.
		if l.DaysLeft(asOf) < 0 && l.Status != "expired" {
			l.Status = "expired"
			if _, err := s.dbFor(r).Exec(
				`UPDATE product_lot SET status = 'expired' WHERE id = ?`, l.ID); err != nil {
				log.Printf("inventario: no se pudo marcar el lote %d como vencido: %v", l.ID, err)
			}
		}
		lotes = append(lotes, l)
	}
	sort.SliceStable(lotes, func(i, j int) bool {
		return lotes[i].DaysLeft(asOf) < lotes[j].DaysLeft(asOf)
	})
	return lotes, filas.Err()
}

// AsignacionFEFO dice de qué lotes sale una cantidad pedida.
type AsignacionFEFO struct {
	LotID      int64
	LotCode    string
	Cantidad   float64
	UnitCost   float64
	Expiracion string
}

// RepartirFEFO reparte una cantidad entre lotes, empezando por el que caduca
// antes.
//
// Devuelve error si no hay existencias suficientes, y NO reparte parcialmente a
// escondidas: quien llama necesita saber si puede servir la línea entera. Un
// reparto parcial que parece completo es como se venden 300 kilos cuando solo
// hay 200.
//
// asOf es la fecha de REFERENCIA para la vigencia. No siempre es hoy: los datos
// históricos (empresa de demostración, cierre de un mes anterior) se sirven con
// la fecha de la venta que se está registrando, y un lote vencido en cálculo
// impacto contra el mes siguiente no lo está contra su venta.
func (s *Server) RepartirFEFO(r *http.Request, productID int64, cantidad float64, asOf time.Time) ([]AsignacionFEFO, float64, error) {
	if cantidad <= 0 {
		return nil, 0, nil
	}
	lotes, err := s.LotesActivos(r, productID)
	if err != nil {
		return nil, 0, err
	}

	var plan []AsignacionFEFO
	falta := cantidad
	for _, l := range lotes {
		if falta <= 0 {
			break
		}
		// Un lote caducado NO se sirve. Se salta, y lo que sigue se descuenta
		// igualmente: el cliente sigue necesitando los kilos, solo que de otro
		// lote.
		if l.DaysLeft(asOf) < 0 {
			continue
		}
		toma := l.QtyAvailable
		if toma > falta {
			toma = falta
		}
		if toma <= 0 {
			continue
		}
		plan = append(plan, AsignacionFEFO{
			LotID: l.ID, LotCode: l.LotCode, Cantidad: toma,
			UnitCost: l.UnitCost, Expiracion: l.ExpiryDate,
		})
		falta -= toma
	}
	if falta > 0.0000001 {
		return plan, falta, fmt.Errorf(
			"stock insuficiente: faltan %.2f unidades del producto %d", falta, productID)
	}
	return plan, 0, nil
}

// ConsumirLote descuenta existencias de un lote.
//
// Se llama DESPUÉS de que la factura se haya confirmado, dentro de la misma
// transacción. El `WHERE quantity_available >= ?` es lo que evita el oversell:
// dos cajas que venden las mismas 10 unidades al mismo tiempo, la segunda recibe
// 0 filas afectadas y se avisa, en vez de dejar el stock en negativo.
func (s *Server) ConsumirLote(r *http.Request, lotID int64, cantidad float64) error {
	if cantidad <= 0 {
		return nil
	}
	res, err := s.dbFor(r).Exec(
		`UPDATE product_lot
		    SET quantity_available = quantity_available - ?
		  WHERE id = ? AND quantity_available >= ?`,
		cantidad, lotID, cantidad)
	if err != nil {
		return fmt.Errorf("no se pudo descontar el lote: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("el lote %d ya no tiene %.2f unidades disponibles", lotID, cantidad)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Alertas de caducidad
// ---------------------------------------------------------------------------

// AlertaCaducidad es un aviso sobre un lote que caduca pronto o ya caducó.
type AlertaCaducidad struct {
	LotID       int64
	ProductID   int64
	ProductName string
	LotCode     string
	ExpiryDate  string
	DaysLeft    int
	Quantity    float64
	UnitCost    float64
	Valor       float64
	Severidad   string
}

// AlertasCaducidad lista los lotes que caducan dentro de `dias`.
//
// `incluirCaducados` decide si los que ya pasaron entran. Entran por defecto
// porque un lote caducado que nadie ve es un lote que se sigue intentando
// vender: esa es la forma habitual de perder la mercadería.
func (s *Server) AlertasCaducidad(r *http.Request, dias int, incluirCaducados bool) ([]AlertaCaducidad, error) {
	if dias <= 0 {
		dias = 30
	}
	asOf := time.Now().Truncate(24 * time.Hour)

	// La caducidad se filtra con `IS NOT NULL` y NO con `<> ''`. En SQLite la
	// cadena vacía era un valor cualquiera en una columna DATE; en MySQL, con
	// STRICT_TRANS_TABLES, comparar una fecha con `''` da Error 1525
	// "Incorrect DATE value: ''" y la consulta entera falla. `IS NOT NULL`
	// significa exactamente lo que se quiere: un lote sin fecha de caducidad.
	//
	// `dias` se convierte en fecha límite en Go y se manda como parámetro
	// normal. La primera versión usaba `DATE_ADD(?, INTERVAL ? DAY)`, que en el
	// cliente parece correcta y sin embargo devuelve cero filas sin dar error:
	// el parámetro dentro de INTERVAL no llega al servidor como el entero que
	// se le pasó, y la comparación queda siempre falsa. Una consulta que
	// devuelve cero sin error es el peor tipo de fallo: la pantalla dice "no
	// hay nada por caducar" y el almacén tiene la cámara llena de lote viejo.
	limite := asOf.AddDate(0, 0, dias).Format("2006-01-02")
	cond := "AND l.expiry_date IS NOT NULL AND l.expiry_date <= ?"
	args := []interface{}{limite}
	if !incluirCaducados {
		cond += " AND l.expiry_date >= ?"
		args = append(args, asOf.Format("2006-01-02"))
	}

	filas, err := s.dbFor(r).Query(`
		SELECT l.id, l.product_id, COALESCE(p.product_name, ''), l.lot_code,
		       l.expiry_date, l.quantity_available, l.unit_cost
		FROM product_lot l
		LEFT JOIN product_information p ON p.product_id = l.product_id
		WHERE l.quantity_available > 0 `+cond+`
		ORDER BY l.expiry_date ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron buscar las alertas: %w", err)
	}
	defer filas.Close()

	var alertas []AlertaCaducidad
	for filas.Next() {
		var a AlertaCaducidad
		var cruda interface{}
		if err := filas.Scan(&a.LotID, &a.ProductID, &a.ProductName, &a.LotCode,
			&cruda, &a.Quantity, &a.UnitCost); err != nil {
			continue
		}
		a.ExpiryDate = fechaBD(cruda)
		c, err := time.Parse("2006-01-02", a.ExpiryDate)
		if err != nil {
			continue
		}
		a.DaysLeft = int(c.Sub(asOf).Hours() / 24)
		a.Valor = a.Quantity * a.UnitCost
		switch {
		case a.DaysLeft < 0:
			a.Severidad = "critical"
		case a.DaysLeft <= 7:
			a.Severidad = "critical"
		case a.DaysLeft <= 30:
			a.Severidad = "warning"
		default:
			a.Severidad = "info"
		}
		alertas = append(alertas, a)
	}
	return alertas, filas.Err()
}

// ValorEnRiesgo es lo que hay en almacén que va a perder valor.
//
// Se separa del listado porque es el número que se mira: no "hay 12 alertas",
// sino "1.240,50 en mercadería que caduca este mes". El primero se ignora; el
// segundo es el que hace pedir el descuento al proveedor o dejar de comprar.
func (s *Server) ValorEnRiesgo(r *http.Request, dias int) (total float64, critico float64, n int, err error) {
	alertas, err := s.AlertasCaducidad(r, dias, true)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, a := range alertas {
		total += a.Valor
		n++
		if a.DaysLeft <= 7 {
			critico += a.Valor
		}
	}
	return total, critico, n, nil
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (s *Server) registerInventoryExpiryRoutes() {
	s.mux.HandleFunc("GET /inventory/expiry", s.inventoryExpiryPage)
	s.mux.HandleFunc("GET /inventory/expiry/data", s.inventoryExpiryData)
	s.mux.HandleFunc("POST /inventory/lots", s.inventoryLotCreate)
}

func (s *Server) inventoryExpiryPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "inventory_expiry.html", map[string]interface{}{"Title": "Caducidades"})
}

func (s *Server) inventoryExpiryData(w http.ResponseWriter, r *http.Request) {
	dias, _ := strconv.Atoi(r.FormValue("dias"))
	if dias == 0 {
		dias = 30
	}
	alertas, err := s.AlertasCaducidad(r, dias, true)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	total, critico, n, err := s.ValorEnRiesgo(r, dias)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"data":    alertas,
		"total":   total,
		"critico": critico,
		"lotes":   n,
		"dias":    dias,
	})
}

func (s *Server) inventoryLotCreate(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "create") {
		jsonError(w, "No tiene permiso para registrar lotes", 403)
		return
	}
	producto, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	cantidad, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	costo, _ := strconv.ParseFloat(r.FormValue("unit_cost"), 64)
	if producto == 0 || cantidad <= 0 {
		jsonError(w, "Falta el producto o la cantidad", 400)
		return
	}
	id, err := s.RegistrarLote(r, LotAlta{
		ProductID:   producto,
		LotCode:     r.FormValue("lot_code"),
		EntryDate:   r.FormValue("entry_date"),
		ExpiryDate:  r.FormValue("expiry_date"),
		QtyReceived: cantidad,
		UnitCost:    costo,
	})
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Lote registrado", "id": id})
}
