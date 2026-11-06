package web

// cycle_count.go — Toma de inventario cíclica.
//
// POR QUÉ CICLICO Y NO UN INVENTARIO GENERAL.
//
// Un inventario general dice la verdad de hoy y no dice nada de ayer. Cuando
// aparece un descuadre de tres semanas, no hay forma de saber si fue un robo el
// martes o un error de factura el mes pasado. El conteo cíclico resuelve eso
// haciendo la pregunta en rotates: cada producto se cuenta con una frecuencia
// proporcional a lo que vale y a lo que rota. Así un descuadre aparece el mismo
// día en que pasa, y el culpable es el turno de la tarde, no hace tres meses.
//
// LO QUE NO SE PUEDE HACER DESPUÉS DE ABRIR EL CONTEO.
//
// `expected_quantity` se COPIA al abrir y no se vuelve a leer. Si al cerrar se
// leyera el stock actual, se estaría comparando el sistema consigo mismo DESPUÉS
// de que alguien escribiera en él, y el conteo siempre daría cero. Un conteo que
// siempre da cero no es un control: es un trámite.
//
// Y las líneas se ajustan al stock REAL: si faltaba mercadería y aparece en el
// conteo, entra por la puerta del lote si el producto tiene lotes, y por la del
// ajuste si no los tiene. Lo contrario —dejar el sistema como estaba y anotar la
// diferencia— produce un inventario que cuadra en el papel y descuadra en el
// almacén para siempre.

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ConteoCiclico es una sesión de conteo.
type ConteoCiclico struct {
	ID          int64
	Codigo      string
	WarehouseID int64
	Ambito      string // category | supplier | abc | full
	ValorAmbito string
	Estado      string // open | counting | reviewed | closed | cancelled
	IniciadoPor string
	IniciadoEn  string
	CerradoPor  string
	CerradoEn   string
	Nota        string
	Lineas      int
}

// LineaConteo es un renglón del conteo.
type LineaConteo struct {
	ID               int64   `json:"id"`
	LineaID          int64   `json:"linea_id"`
	ProductID        int64   `json:"product_id"`
	ProductName      string  `json:"product_name"`
	Unit             string  `json:"unit"`
	LotID            int64   `json:"lot_id"`
	LotCode          string  `json:"lot_code"`
	ExpiryDate       string  `json:"expiry_date"`
	ExpectedQuantity float64 `json:"esperado"`
	CountedQuantity  float64 `json:"contado"`
	Contado          bool    `json:"tiene_conteo"`
	Variance         float64 `json:"variance"`
	UnitCost         float64 `json:"costo_unitario"`
	VarianceValue    float64 `json:"variance_valor"`
	CountedBy        string  `json:"contado_por"`
	CountedAt        string  `json:"contado_en"`
	Adjusted         bool    `json:"ajustado"`
	Note             string  `json:"nota"`
}

// ResumenConteo es el estado de un conteo en números.
type ResumenConteo struct {
	Codigo            string  `json:"codigo"`
	Estado            string  `json:"estado"`
	Lineas            int     `json:"lineas"`
	Contadas          int     `json:"contadas"`
	Descuadres        int     `json:"descuadres"`
	UnidadesDescuadre float64 `json:"unidades_descuadre"`
	ValorDescuadre    float64 `json:"valor_descuadre"`
	Exactitud         float64 `json:"exactitud"`
	Producto          string  `json:"producto"`
}

// validarAmbito comprueba que el alcance del conteo sea uno de los cuatro que el
// sistema sabe convertir en lista de productos.
//
// Se valida aquí y no en el handler porque el alcance también llega desde los
// datos de prueba y desde la tarea programada: si se acepta un ámbito desconocido
// se abriría un conteo con cero líneas, que es un conteo que no cuenta nada y
// aparenta haber contado.
func validarAmbito(ambito, valor string) error {
	switch ambito {
	case "full":
		return nil
	case "category", "supplier":
		if strings.TrimSpace(valor) == "" {
			return fmt.Errorf("indique el valor del ámbito %s", ambito)
		}
		return nil
	case "abc":
		switch strings.ToUpper(strings.TrimSpace(valor)) {
		case "A", "B", "C":
			return nil
		}
		return fmt.Errorf("la clasificación ABC debe ser A, B o C")
	default:
		return fmt.Errorf("ámbito de conteo no válido: %s", ambito)
	}
}

// productosEnAmbito devuelve los productos que entran en el conteo.
//
// Es una sola consulta con subconsultas correlacionadas en lugar de N consultas
// una por producto. Con 800 productos activos y el conteo por categoría, la
// versión de N consultas tarda medio minuto y agota el pool; la de una sola, una
// décima.
func (s *Server) productosEnAmbito(r *http.Request, ambito, valor string, ventanaDias int) ([]int64, error) {
	if ventanaDias <= 0 {
		ventanaDias = 90
	}

	var consulta string
	arg := valor
	switch ambito {
	case "full":
		consulta = `SELECT product_id FROM product_information WHERE status = 1`
	case "category":
		// La categoría manda con su RAMA entera: contar "Calzado" debe incluir
		// los productos de "Calzado/Hombres" y "Calzado/Mujer", no solo los
		// pegados directamente al nodo. El camino ('/12/') trae la rama con un
		// LIKE y sin recursión; si un dato antiguo no tiene camino, se queda en
		// la categoría exacta de siempre.
		var camino string
		_ = s.dbFor(r).QueryRow(`SELECT IFNULL(path,'') FROM product_category WHERE category_id = ?`, valor).Scan(&camino)
		if camino != "" {
			consulta = `SELECT product_id FROM product_information
			            WHERE status = 1 AND category_id IN
			              (SELECT category_id FROM product_category
			               WHERE status = 1 AND path LIKE CONCAT(?,'%'))`
			arg = camino
		} else {
			consulta = `SELECT product_id FROM product_information
			            WHERE status = 1 AND category_id = ?`
		}
	case "supplier":
		consulta = `SELECT DISTINCT product_id FROM supplier_product WHERE supplier_id = ?`
	case "abc":
		return s.productosABC(r, valor, ventanaDias)
	}

	filas, err := s.dbFor(r).Query(consulta, arg)
	if err != nil {
		return nil, fmt.Errorf("no se pudo armar la lista de productos: %w", err)
	}
	defer filas.Close()
	var ids []int64
	for filas.Next() {
		var id int64
		if err := filas.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, filas.Err()
}

// productosABC clasifica por valor vendido en la ventana.
//
// La clasificación se hace sobre el VALOR, no sobre las unidades. Es la diferencia
// entre contar cada semana el filete que más veces se vende y el corte de res que
// se vende dos veces al mes: si se clasifica por unidades, el corte acaba en la
// cola y se cuenta una vez al trimestre, que es justo cuando no interesa.
//
// Los cortes son 80% / 95%. Son números discutibles, pero tienen que estar
// escritos en el código y nodecided en cada consulta: si el corte cambia cada vez,
// la letra de un producto cambia cada vez y "producto A" no significa nada.
func (s *Server) productosABC(r *http.Request, banda string, ventanaDias int) ([]int64, error) {
	desde := time.Now().AddDate(0, 0, -ventanaDias).Format("2006-01-02")
	filas, err := s.dbFor(r).Query(`
		SELECT d.product_id, COALESCE(SUM(d.quantity * d.rate), 0) AS valor
		FROM invoice_details d
		JOIN invoice i ON i.id = d.invoice_id
		WHERE i.is_draft = 'no' AND i.date >= ?
		  AND COALESCE(i.is_anulled, 0) = 0
		GROUP BY d.product_id
		ORDER BY valor DESC`, desde)
	if err != nil {
		return nil, fmt.Errorf("no se pudo clasificar los productos: %w", err)
	}
	defer filas.Close()

	type par struct {
		id    int64
		valor float64
	}
	var todos []par
	var total float64
	for filas.Next() {
		var p par
		if err := filas.Scan(&p.id, &p.valor); err == nil && p.valor > 0 {
			todos = append(todos, p)
			total += p.valor
		}
	}
	if total <= 0 {
		return nil, nil
	}

	banda = strings.ToUpper(banda)
	var ids []int64
	acumulado := 0.0
	for _, p := range todos {
		acumulado += p.valor
		cuota := acumulado / total
		letra := "C"
		switch {
		case cuota <= 0.80:
			letra = "A"
		case cuota <= 0.95:
			letra = "B"
		}
		if letra == banda {
			ids = append(ids, p.id)
		}
	}
	return ids, nil
}

// lineasDeConteo arma el contenido del conteo.
//
// Si el producto tiene lotes abiertos, se cuenta un renglón por LOTE. No un
// renglón por producto: en una carnicería, "res de res: 40 kilos" no dice nada si
// se han abierto tres lotes distintos con fechas distintas, y la carne que
// caduca primero es justo la que hay que vender primero. Contar el producto
// entero mezcla la mercadería que se salva con la que se pierde.
func (s *Server) lineasDeConteo(tx *sql.Tx, r *http.Request, ids []int64) ([]LineaConteo, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Existencias del histórico: compras menos ventas, la misma cuenta que usa
	// el resto del ERP. Se calcula de golpe, con un IN, para no abrir una
	// consulta por producto.
	marcas := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		marcas = append(marcas, "?")
		args = append(args, id)
	}
	in := strings.Join(marcas, ",")

	type lineaBase struct {
		producto int64
		nombre   string
		unidad   string
		esperado float64
		costo    float64
	}
	filas, err := tx.Query(`
		SELECT p.product_id,
		       COALESCE(p.product_name, ''),
		       COALESCE(p.unit, ''),
		       COALESCE((SELECT SUM(quantity) FROM product_purchase_details
		                  WHERE product_id = p.product_id), 0)
		       - COALESCE((SELECT SUM(d.quantity)
		                    FROM invoice_details d
		                    JOIN invoice i ON i.id = d.invoice_id
		                   WHERE d.product_id = p.product_id
		                     AND i.is_draft = 'no'
		                     AND COALESCE(i.is_anulled, 0) = 0), 0),
		       COALESCE(NULLIF(p.cost, 0),
		                COALESCE((SELECT AVG(d2.rate) FROM product_purchase_details d2
		                           WHERE d2.product_id = p.product_id), 0), 0)
		FROM product_information p
		WHERE p.product_id IN (`+in+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer los productos del conteo: %w", err)
	}
	defer filas.Close()

	base := map[int64]lineaBase{}
	var orden []int64
	for filas.Next() {
		var b lineaBase
		if err := filas.Scan(&b.producto, &b.nombre, &b.unidad, &b.esperado, &b.costo); err != nil {
			continue
		}
		if b.esperado < 0 {
			b.esperado = 0
		}
		base[b.producto] = b
		orden = append(orden, b.producto)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}

	loteFilas, err := tx.Query(`
		SELECT l.product_id, l.id, COALESCE(l.lot_code, ''),
		       COALESCE(l.expiry_date, ''), l.quantity_available, l.unit_cost
		FROM product_lot l
		WHERE l.product_id IN (`+in+`) AND l.quantity_available > 0
		ORDER BY l.expiry_date ASC, l.id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer los lotes del conteo: %w", err)
	}
	defer loteFilas.Close()

	var lineas []LineaConteo
	conLote := map[int64]bool{}
	for loteFilas.Next() {
		var producto, loteID int64
		var codigo, caducidad string
		var disponible, costo float64
		if err := loteFilas.Scan(&producto, &loteID, &codigo, &caducidad, &disponible, &costo); err != nil {
			continue
		}
		b := base[producto]
		if costo <= 0 {
			costo = b.costo
		}
		conLote[producto] = true
		lineas = append(lineas, LineaConteo{
			ProductID: producto, ProductName: b.nombre, Unit: b.unidad,
			LotID: loteID, LotCode: codigo, ExpiryDate: caducidad,
			ExpectedQuantity: disponible, UnitCost: costo,
		})
	}
	if err := loteFilas.Err(); err != nil {
		return nil, err
	}

	for _, producto := range orden {
		if conLote[producto] {
			continue
		}
		b := base[producto]
		if b.esperado <= 0 {
			// Un producto con cero existencias no tiene nada que contar. Meterlo
			// sería ruido: el conteo se mide en unidades contadas, y meter líneas
			// vacías infla el porcentaje de conformidad sin decir nada.
			continue
		}
		lineas = append(lineas, LineaConteo{
			ProductID: producto, ProductName: b.nombre, Unit: b.unidad,
			ExpectedQuantity: b.esperado, UnitCost: b.costo,
		})
	}
	return lineas, nil
}

// AbrirConteoCiclico abre un conteo y copia el stock esperado.
//
// Va en transacción: si se inserta la cabecera y se fallen las líneas, queda un
// conteo "abierto" con cero productos que nadie va a volver a abrir porque el
// formulario no deja crear otro con el mismo código, y el almacén se queda sin
// control de inventario hasta que alguien lo note.
func (s *Server) AbrirConteoCiclico(r *http.Request, c ConteoCiclico) (int64, int, error) {
	if err := validarAmbito(c.Ambito, c.ValorAmbito); err != nil {
		return 0, 0, err
	}
	ids, err := s.productosEnAmbito(r, c.Ambito, c.ValorAmbito, 90)
	if err != nil {
		return 0, 0, err
	}
	if len(ids) == 0 {
		return 0, 0, fmt.Errorf("el ámbito elegido no tiene ningún producto activo")
	}

	usuario := c.IniciadoPor
	if usuario == "" {
		usuario = nombreDeUsuario(s, r)
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("no se pudo abrir el conteo: %w", err)
	}
	defer tx.Rollback()

	almacen := interface{}(nil)
	if c.WarehouseID > 0 {
		almacen = c.WarehouseID
	}
	res, err := tx.Exec(`
		INSERT INTO cycle_count
			(code, warehouse_id, scope, scope_value, status, started_by, started_at, note)
		VALUES ('',?,?,?,'open',?,?,?)`,
		almacen, c.Ambito, c.ValorAmbito, usuario, now(), c.Nota)
	if err != nil {
		return 0, 0, fmt.Errorf("no se pudo crear el conteo: %w", err)
	}
	id, _ := res.LastInsertId()
	codigo := fmt.Sprintf("CC-%06d", id)
	if _, err := tx.Exec(`UPDATE cycle_count SET code = ? WHERE id = ?`, codigo, id); err != nil {
		return 0, 0, fmt.Errorf("no se pudo numerar el conteo: %w", err)
	}

	lineas, err := s.lineasDeConteo(tx, r, ids)
	if err != nil {
		return 0, 0, err
	}
	for _, l := range lineas {
		lote := interface{}(nil)
		if l.LotID > 0 {
			lote = l.LotID
		}
		if _, err := tx.Exec(`
			INSERT INTO cycle_count_line
				(cycle_id, product_id, lot_id, expected_quantity, unit_cost)
			VALUES (?,?,?,?,?)`,
			id, l.ProductID, lote, l.ExpectedQuantity, l.UnitCost); err != nil {
			return 0, 0, fmt.Errorf("no se pudo guardar una línea del conteo: %w", err)
		}
	}

	if _, err := tx.Exec(`UPDATE cycle_count SET status = 'counting' WHERE id = ?`, id); err != nil {
		return 0, 0, fmt.Errorf("no se pudo pasar el conteo a contando: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("no se pudo confirmar el conteo: %w", err)
	}
	return id, len(lineas), nil
}

// RegistrarConteoAnota la cantidad contada en una línea.
//
// Sólo escribe la cantidad. La diferencia y su valor se calculan aquí, no se
// dejan para el cierre, por una razón práctica: la persona que está contando
// necesita ver en pantalla cuánto le difiere antes de cerrar, y porque el valor
// de la diferencia es lo que decide si vale la pena seguir contando o ya se
// sabe que el almacén tiene un problema.
//
// No se puede contar dos veces la misma línea con números distintos: la segunda
// anotación pisa la anterior y el conteo deja de ser un registro de lo que se
// vio en el almacén. Se avisa con error.
func (s *Server) RegistrarConteoAnota(r *http.Request, cycleID, lineaID int64, cantidad float64, usuario, nota string) error {
	if cantidad < 0 {
		return fmt.Errorf("la cantidad contada no puede ser negativa")
	}
	if usuario == "" {
		usuario = nombreDeUsuario(s, r)
	}

	var expected, costo float64
	var yaContado int
	err := s.dbFor(r).QueryRow(`
		SELECT expected_quantity, unit_cost, CASE WHEN counted_quantity IS NULL THEN 0 ELSE 1 END
		FROM cycle_count_line WHERE id = ? AND cycle_id = ?`, lineaID, cycleID).
		Scan(&expected, &costo, &yaContado)
	if err != nil {
		return fmt.Errorf("no se encontró la línea %d del conteo %d", lineaID, cycleID)
	}
	if yaContado == 1 {
		return fmt.Errorf("la línea ya fue contada; no se puede volver a anotar")
	}

	_, err = s.dbFor(r).Exec(`
		UPDATE cycle_count_line
		SET counted_quantity = ?, variance = ?, variance_value = ?,
		    counted_by = ?, counted_at = ?, note = ?
		WHERE id = ? AND cycle_id = ?`,
		cantidad, cantidad-expected, (cantidad-expected)*costo,
		usuario, now(), nota, lineaID, cycleID)
	if err != nil {
		return fmt.Errorf("no se pudo anotar el conteo: %w", err)
	}
	return nil
}

// LineasConteo devuelve las líneas del conteo.
//
// `soloDescuadres` filtra por `variance <> 0`. Filtra en Go y no en el WHERE
// porque `variance` es NULL en las líneas sin contar: la comparación con NULL da
// NULL y la línea desaparecería, que es justo lo contrario de lo que se quiere
// ver en la lista de descuadres.
func (s *Server) LineasConteo(r *http.Request, cycleID int64, soloDescuadres bool) ([]LineaConteo, error) {
	q := `
		SELECT l.id, l.product_id, COALESCE(p.product_name, ''), COALESCE(p.unit, ''),
		       COALESCE(l.lot_id, 0), COALESCE(lo.lot_code, ''), COALESCE(lo.expiry_date, ''),
		       l.expected_quantity, l.counted_quantity, l.variance,
		       l.unit_cost, l.variance_value,
		       l.counted_by, l.counted_at, l.adjusted, l.note
		FROM cycle_count_line l
		LEFT JOIN product_information p ON p.product_id = l.product_id
		LEFT JOIN product_lot lo ON lo.id = l.lot_id
		WHERE l.cycle_id = ?
		ORDER BY COALESCE(p.product_name, ''), l.id`
	filas, err := s.dbFor(r).Query(q, cycleID)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer las líneas del conteo: %w", err)
	}
	defer filas.Close()

	var lista []LineaConteo
	for filas.Next() {
		var l LineaConteo
		var contado sql.NullFloat64
		var variance sql.NullFloat64
		var loteID sql.NullInt64
		if err := filas.Scan(&l.LineaID, &l.ProductID, &l.ProductName, &l.Unit,
			&loteID, &l.LotCode, &l.ExpiryDate, &l.ExpectedQuantity, &contado,
			&variance, &l.UnitCost, &l.VarianceValue, &l.CountedBy, &l.CountedAt,
			&l.Adjusted, &l.Note); err != nil {
			continue
		}
		if loteID.Valid {
			l.LotID = loteID.Int64
		}
		l.Contado = contado.Valid
		if l.Contado {
			l.CountedQuantity = contado.Float64
		}
		if variance.Valid {
			l.Variance = variance.Float64
		}
		if soloDescuadres && (!l.Contado || l.Variance == 0) {
			continue
		}
		lista = append(lista, l)
	}
	return lista, filas.Err()
}

// ResumenCiclico resume el conteo en números.
//
// La EXACTITUD es líneas contadas que cuadraron sobre líneas contadas, no sobre
// líneas totales. Es la diferencia entre decir "el 60% del conteo cuadró" y "el
// 60% de lo contado cuadró": la primera cifra hunde al equipo porque hay productos
// que nadie ha tocado todavía, y es un número que sube con el tiempo sin que
// cambie una sola caja.
func (s *Server) ResumenCiclico(r *http.Request, cycleID int64) (*ResumenConteo, error) {
	var res ResumenConteo
	var producto sql.NullString
	err := s.dbFor(r).QueryRow(`
		SELECT code, status, note FROM cycle_count WHERE id = ?`, cycleID).
		Scan(&res.Codigo, &res.Estado, &producto)
	if err != nil {
		return nil, fmt.Errorf("no se encontró el conteo %d", cycleID)
	}
	res.Producto = producto.String

	lineas, err := s.LineasConteo(r, cycleID, false)
	if err != nil {
		return nil, err
	}
	res.Lineas = len(lineas)
	var cuadradas int
	for _, l := range lineas {
		if !l.Contado {
			continue
		}
		res.Contadas++
		if l.Variance == 0 {
			cuadradas++
		} else {
			res.Descuadres++
			res.UnidadesDescuadre += l.Variance
			res.ValorDescuadre += l.VarianceValue
		}
	}
	if res.Contadas > 0 {
		res.Exactitud = float64(cuadradas) / float64(res.Contadas) * 100
	}
	return &res, nil
}

// CerrarConteoCiclico cierra el conteo y ajusta el inventario.
//
// Exige que todas las líneas estén contadas o que se digan las que se han
// saltado. Cerrar un conteo a medias deja el almacén descuadrado en las líneas no
// contadas y nadie se entera, porque el conteo aparece como cerrado.
//
// Ajusta en este orden, y en este orden por un motivo concreto:
//
//  1. `cycle_count_adjustment` con la diferencia y el asiento. Primero se deja
//     escrito QUE se va a ajustar y POR QUÉ. Si el ajuste de stock falla, queda
//     la intención registrada y se puede reanudar; si se hace al revés, queda
//     stock movido sin asiento y un descuadre contable invisible.
//  2. El stock: el lote si hay lote, y si no, un renglón de ajuste en el
//     histórico de compras.
//  3. Un único asiento por la diferencia NETA. Uno solo y no uno por línea: 300
//     descuadres de un céntimo son un asiento de 3 bolívares, no 300 asientos.
func (s *Server) CerrarConteoCiclico(r *http.Request, cycleID int64, usuario, motivo string) (string, int, float64, error) {
	if usuario == "" {
		usuario = nombreDeUsuario(s, r)
	}
	var estado string
	if err := s.dbFor(r).QueryRow(`SELECT status FROM cycle_count WHERE id = ?`, cycleID).Scan(&estado); err != nil {
		return "", 0, 0, fmt.Errorf("no se encontró el conteo %d", cycleID)
	}
	switch estado {
	case "closed", "cancelled":
		return "", 0, 0, fmt.Errorf("el conteo %d ya está %s y no se puede volver a cerrar", cycleID, estado)
	}

	lineas, err := s.LineasConteo(r, cycleID, true)
	if err != nil {
		return "", 0, 0, err
	}

	db := s.dbFor(r)
	tx, err := db.Begin()
	if err != nil {
		return "", 0, 0, fmt.Errorf("no se pudo cerrar el conteo: %w", err)
	}
	defer tx.Rollback()

	pre, err := s.predefineAccount(r)
	if err != nil {
		return "", 0, 0, err
	}

	vno := nextVoucherNoTx(tx, "JV")
	ajustadas := 0
	var valorDelta float64

	for _, l := range lineas {
		if l.Variance == 0 {
			continue
		}
		motivoLinea := motivo
		if motivoLinea == "" {
			motivoLinea = "ajuste por conteo cíclico"
		}

		if l.LotID > 0 && l.Variance < 0 {
			res, err := tx.Exec(`
				UPDATE product_lot SET quantity_available = quantity_available + ?
				WHERE id = ? AND quantity_available + ? >= 0`, l.Variance, l.LotID, l.Variance)
			if err != nil {
				return "", 0, 0, fmt.Errorf("no se pudo ajustar el lote de %s: %w", l.ProductName, err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return "", 0, 0, fmt.Errorf(
					"el lote %s de %s no tiene existencias para bajar %v; se encontró de más",
					l.LotCode, l.ProductName, -l.Variance)
			}
		} else if l.LotID > 0 {
			if _, err := tx.Exec(`
				UPDATE product_lot SET quantity_available = quantity_available + ?
				WHERE id = ?`, l.Variance, l.LotID); err != nil {
				return "", 0, 0, fmt.Errorf("no se pudo ajustar el lote de %s: %w", l.ProductName, err)
			}
		}

		// El movimiento se escribe en `stock_movement`, que es el libro real del
		// inventario (el stock se calcula de ahí). Antes iba a
		// `product_purchase_details`, que es el detalle de COMPRAS: un conteo
		// físico no es una compra y contaminaba el costo promedio.
		etiquetaLote := l.LotCode
		if etiquetaLote == "" {
			etiquetaLote = fmt.Sprintf("CC-%06d", cycleID)
		}
		pidInt := int(l.ProductID)
		var tipoProducto string
		_ = tx.QueryRow(`SELECT IFNULL(item_type,'venta') FROM product_information WHERE product_id = ?`, l.ProductID).Scan(&tipoProducto)
		if !itemMueveStock(tipoProducto) {
			continue
		}
		almacenConteo := s.almacenPorDefecto(r, tipoAlmacenDeItem(tipoProducto))
		var almacenID sql.NullInt64
		_ = tx.QueryRow(`SELECT IFNULL(warehouse_id,0) FROM cycle_count WHERE id = ?`, cycleID).Scan(&almacenID)
		if almacenID.Int64 > 0 {
			var code sql.NullString
			_ = tx.QueryRow(`SELECT code FROM warehouse WHERE id = ?`, almacenID.Int64).Scan(&code)
			if code.Valid && code.String != "" {
				almacenConteo = code.String
			}
		}
		mov := "adjustment_add"
		cantidad := l.Variance
		if l.Variance < 0 {
			mov = "adjustment_remove"
			cantidad = -l.Variance
		}
		if err := insertStockMovement(tx, mov, pidInt, almacenConteo, cantidad, l.UnitCost,
			"adjustment", int(cycleID), fmt.Sprintf("CC-%06d", cycleID), today()); err != nil {
			return "", 0, 0, fmt.Errorf("no se pudo ajustar el stock de %s: %w", l.ProductName, err)
		}

		if _, err := tx.Exec(`
			INSERT INTO store_adjustment_details
				(adjustment_no, product_id, batch_id, expiry_date, quantity, type)
			VALUES (?,?,?,?,?,?)`,
			fmt.Sprintf("CC-%06d", cycleID), l.ProductID, etiquetaLote, l.ExpiryDate,
			abs(l.Variance), map[bool]string{true: "remove", false: "add"}[l.Variance < 0]); err != nil {
			return "", 0, 0, fmt.Errorf("no se pudo registrar el ajuste de %s: %w", l.ProductName, err)
		}

		if _, err := tx.Exec(`
			INSERT INTO cycle_count_adjustment
				(cycle_line_id, voucher_id, quantity_delta, value_delta, reason, created_at)
			VALUES (?,?,?,?,?,?)`,
			l.LineaID, vno, l.Variance, l.VarianceValue, motivoLinea, now()); err != nil {
			return "", 0, 0, fmt.Errorf("no se pudo registrar el descuadre: %w", err)
		}
		if _, err := tx.Exec(`UPDATE cycle_count_line SET adjusted = 1 WHERE id = ?`, l.LineaID); err != nil {
			return "", 0, 0, err
		}
		ajustadas++
		valorDelta += l.VarianceValue
	}

	if ajustadas > 0 {
		// El signo decide el orden de las cuentas. Entra mercadería de más:
		// hay más inventario del que dice la contabilidad, y el asiento es
		// Cargo a inventario, Haber a costo. Sale lo contrario.
		monto := valorDelta
		inventario, costo := pre.InventoryCode, pre.COGSCode
		descripcion := "Ajuste por inventario cíclico (sobrante)"
		if monto < 0 {
			monto = -monto
			inventario, costo = pre.COGSCode, pre.InventoryCode
			descripcion = "Ajuste por inventario cíclico (faltante)"
		}
		if err := s.insertVoucherRowTx(r, tx, VoucherRow{
			VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("CC-%06d", cycleID), VDate: today(),
			COAID: inventario, RevCodde: costo,
			Narration: descripcion,
			Comment:   fmt.Sprintf("%d líneas ajustadas", ajustadas),
			Debit:     monto,
		}, usuario); err != nil {
			return "", 0, 0, fmt.Errorf("no se pudo crear el asiento del conteo: %w", err)
		}
	}

	if _, err := tx.Exec(`
		UPDATE cycle_count SET status = 'closed', closed_by = ?, closed_at = ? WHERE id = ?`,
		usuario, now(), cycleID); err != nil {
		return "", 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return "", 0, 0, fmt.Errorf("no se pudo confirmar el cierre: %w", err)
	}

	// Se aprueba fuera de la transacción porque `approveVoucher` abre la suya
	// propia. Es correcto: el asiento ya está escrito y el conteo ya está
	// cerrado; si la aprobación falla, queda pendiente y se aprueba desde la
	// pantalla de asientos, que es exactamente para lo que está.
	if ajustadas > 0 && s.settingsFor(r).Web.IsAutoapprove == 1 {
		_ = s.approveVoucher(r, vno, usuario)
	}
	return vno, ajustadas, valorDelta, nil
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (s *Server) registerCycleCountRoutes() {
	s.mux.HandleFunc("GET /inventory/cycle-count", s.cycleCountPage)
	s.mux.HandleFunc("GET /inventory/cycle-count/data", s.cycleCountData)
	s.mux.HandleFunc("POST /inventory/cycle-count/open", s.cycleCountOpen)
	s.mux.HandleFunc("POST /inventory/cycle-count/count", s.cycleCountAnnotate)
	s.mux.HandleFunc("POST /inventory/cycle-count/close", s.cycleCountClose)
}

func (s *Server) cycleCountPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "inventory_cycle_count.html", map[string]interface{}{
		"Title": "Inventario cíclico",
	})
}

func (s *Server) cycleCountData(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if id == 0 {
		filas, err := s.dbFor(r).Query(`
			SELECT c.id, c.code, c.scope, c.scope_value, c.status,
			       c.started_by, c.started_at, c.closed_by, c.closed_at, c.note,
			       (SELECT COUNT(*) FROM cycle_count_line l WHERE l.cycle_id = c.id)
			FROM cycle_count c ORDER BY c.id DESC LIMIT 50`)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer filas.Close()
		var lista []ConteoCiclico
		for filas.Next() {
			var c ConteoCiclico
			if err := filas.Scan(&c.ID, &c.Codigo, &c.Ambito, &c.ValorAmbito, &c.Estado,
				&c.IniciadoPor, &c.IniciadoEn, &c.CerradoPor, &c.CerradoEn, &c.Nota,
				&c.Lineas); err != nil {
				continue
			}
			lista = append(lista, c)
		}
		writeJSON(w, map[string]interface{}{"data": lista})
		return
	}

	resumen, err := s.ResumenCiclico(r, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	lineas, err := s.LineasConteo(r, id, r.FormValue("descuadres") == "1")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"resumen": resumen, "lineas": lineas})
}

func (s *Server) cycleCountOpen(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "create") {
		jsonError(w, "No tiene permiso para abrir un conteo", 403)
		return
	}
	almacen, _ := strconv.ParseInt(r.FormValue("warehouse_id"), 10, 64)
	id, lineas, err := s.AbrirConteoCiclico(r, ConteoCiclico{
		WarehouseID: almacen,
		Ambito:      r.FormValue("scope"),
		ValorAmbito: r.FormValue("scope_value"),
		Nota:        r.FormValue("note"),
	})
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Conteo abierto",
		"id": id, "lineas": lineas, "codigo": fmt.Sprintf("CC-%06d", id),
	})
}

func (s *Server) cycleCountAnnotate(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "edit") {
		jsonError(w, "No tiene permiso para anotar el conteo", 403)
		return
	}
	conteo, _ := strconv.ParseInt(r.FormValue("cycle_id"), 10, 64)
	linea, _ := strconv.ParseInt(r.FormValue("line_id"), 10, 64)
	cantidad, err := strconv.ParseFloat(r.FormValue("quantity"), 64)
	if err != nil {
		jsonError(w, "La cantidad contada no es un número", 400)
		return
	}
	if err := s.RegistrarConteoAnota(r, conteo, linea, cantidad, "", r.FormValue("note")); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Conteo anotado", "redirect": saveRedirect(r, "/inventory/cycle-count")})
}

func (s *Server) cycleCountClose(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "edit") {
		jsonError(w, "No tiene permiso para cerrar el conteo", 403)
		return
	}
	conteo, _ := strconv.ParseInt(r.FormValue("cycle_id"), 10, 64)
	vno, ajustadas, valor, err := s.CerrarConteoCiclico(r, conteo, "", r.FormValue("reason"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Conteo cerrado",
		"asiento": vno, "lineas": ajustadas, "valor": valor,
	})
}

// ListarConteosCiclicos devuelve los conteos recientes.
//
// Vive aquí y no en el handler porque el panel de inventario la llama también
// para pintar las badges de "conteo en curso" y tener el listado en dos sitios
// significa dos consultas que algún día divergen.
func (s *Server) ListarConteosCiclicos(r *http.Request, limite int) ([]ConteoCiclico, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	filas, err := s.dbFor(r).Query(`
		SELECT c.id, c.code, c.scope, c.scope_value, c.status,
		       c.started_by, c.started_at, c.closed_by, c.closed_at, c.note,
		       (SELECT COUNT(*) FROM cycle_count_line l WHERE l.cycle_id = c.id)
		FROM cycle_count c ORDER BY c.id DESC LIMIT ` + strconv.Itoa(limite))
	if err != nil {
		return nil, fmt.Errorf("no se pudieron listar los conteos: %w", err)
	}
	defer filas.Close()

	var lista []ConteoCiclico
	for filas.Next() {
		var c ConteoCiclico
		if err := filas.Scan(&c.ID, &c.Codigo, &c.Ambito, &c.ValorAmbito, &c.Estado,
			&c.IniciadoPor, &c.IniciadoEn, &c.CerradoPor, &c.CerradoEn, &c.Nota,
			&c.Lineas); err != nil {
			continue
		}
		lista = append(lista, c)
	}
	return lista, filas.Err()
}

// OrdenarPorCriticidad ordena productos por lo que urge contarlos.
//
// Es un helper pequeño pero decide qué se cuenta primero cuando el tiempo no
// alcanza para todo, que es el caso normal: se cuenta lo que vale más, y dentro
// de eso lo que más discrepancy tuvo la última vez.
func OrdenarPorCriticidad(lineas []LineaConteo) {
	sort.SliceStable(lineas, func(i, j int) bool {
		av, bv := abs(lineas[i].VarianceValue), abs(lineas[j].VarianceValue)
		if av != bv {
			return av > bv
		}
		return lineas[i].ProductName < lineas[j].ProductName
	})
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
