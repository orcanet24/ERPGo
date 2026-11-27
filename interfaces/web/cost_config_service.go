package web

// cost_config_service.go — Configuración y consulta del motor de costos.
//
// POR QUÉ ESTE FICHERO ESTÁ SEPARADO DE LOS HANDLERS.
//
// La misma razón que en el CRM: el handler valida y traduce, y toda la regla
// vive aquí, que es donde se prueba sin levantar un servidor HTTP.
//
// Y hay un motivo más, concreto: `cost_engine.go` ya calcula. Si la lectura de
// la configuración viviera en los handlers, el motor dependería de que el HTTP
// funcionara para poder usarse, y eso no es cierto: los scripts, los informes
// programados y el cierre de período necesitan leer la configuración sin pasar
// por un handler.
//
// LO QUE NO HACE ESTE FICHERO.
//
// No calcula ningún costo. Calcular es cosa de `cost_engine.go`, y esa frontera
// se respeta: aquí solo se lee y se escribe configuración.
//
// REGLA DEL DENOMINADOR, RECORDADA.
//
// Todo lo que se devuelve como dinero pasa por `valorNumerico()`. MySQL entrega
// los DECIMAL como []byte para no perder precisión, y saltarse eso deja los
// totales en cero sin ningún error visible — que es exactamente lo que pasó con
// el motor de informes.

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// métodos de costeo que la pantalla ofrece, con lo que cada uno necesita antes
// de poder calcular. La lista va en el servidor y no en la plantilla para que
// el HTML no sea el que decide qué métodos existen.
var costMetodos = []struct {
	Codigo    string
	Nombre    string
	Necesita  string
	ParaQuien string
}{
	{costPP, "Costo promedio", "nada: usa el costo que ya lleva el kardex",
		"Comercio simple: se compra y se revende"},
	{costRecipe, "Receta (BOM)", "product_recipe con sus componentes y merma",
		"Textilera, panadería, cualquier taller"},
	{costYield, "Rendimiento de despiece", "product_yield con el porcentaje medido",
		"Carnicería: repartir la canal sobre lo que sale a venta"},
	{costHours, "Horas de equipo", "cost_timesheet con horas y grado",
		"Servicios: el costo es tiempo"},
	{costImport, "Importación", "import_cost con gastos y unidades recibidas",
		"Autopartes y textilera: compra + aduana"},
}

// ---------------------------------------------------------------------------
// Configuración por producto
// ---------------------------------------------------------------------------

// CostConfig es la configuración de costeo de un producto, tal y como se
// guarda. Es el mismo tipo que lee y escribe el motor, para que lo que se ve en
// pantalla y lo que se calcula no puedan divergir.
type CostConfig struct {
	ProductID   string
	Metodo      string
	LossFactor  float64
	Perishable  int
	ShelfLife   int
	IsAssembled int
	TrackLots   int
	TrackSerial int
	Imported    int
	CostCenter  string
	UpdatedAt   string
}

// leerCostConfig devuelve la configuración de un producto.
//
// sql.ErrNoRows NO es un error: un producto sin configurar es un producto que
// usa PP, que es lo que hace el ERP desde antes de que esto existiera.
func leerCostConfig(db *sql.DB, productID string) (CostConfig, error) {
	var c CostConfig
	c.ProductID = productID
	c.Metodo = costPP

	var cc sql.NullString
	var loss, centro any
	err := db.QueryRow(`SELECT costing_method, IFNULL(loss_factor,0),
		perishable, shelf_life_days, is_assembled, track_lots, track_serial,
		imported, cost_center_id, updated_at
		FROM product_cost_config WHERE product_id = ?`, productID).
		Scan(&c.Metodo, &loss, &c.Perishable, &c.ShelfLife, &c.IsAssembled,
			&c.TrackLots, &c.TrackSerial, &c.Imported, &centro, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("leer configuración de costeo de %s: %w", productID, err)
	}
	if f := valorNumerico(loss); f != nil {
		c.LossFactor = *f
	}
	if cc.Valid {
		c.CostCenter = cc.String
	}
	return c, nil
}

// guardarCostConfig escribe la configuración. Es un UPSERT: guardar un producto
// que no tenía configuración no falla, y guardar uno que ya la tenía la
// reemplaza en vez de duplicar la fila.
func guardarCostConfig(db *sql.DB, c CostConfig, ahora string) error {
	if strings.TrimSpace(c.ProductID) == "" {
		return fmt.Errorf("el producto es obligatorio")
	}
	if !metodoDeCosteoValido(c.Metodo) {
		return fmt.Errorf("el método de costeo %q no existe", c.Metodo)
	}
	if c.LossFactor < 0 || c.LossFactor > 1 {
		return fmt.Errorf("la merma va entre 0 y 1 (0.025 = 2,5%%), y llegó %.4f", c.LossFactor)
	}
	if c.ShelfLife < 0 {
		return fmt.Errorf("la vida útil no puede ser negativa")
	}

	// cost_center_id es INT y admite NULL, pero NO cadena vacía.
	//
	// Este bug lo encontró TestLaMismaEscalaDeMermaEnConfigYEnPantalla, que
	// guarda una configuración sin centro de costo: MySQL en modo estricto
	// responde `Error 1366: Incorrect integer value: '' for column
	// 'cost_center_id'` y el guardado entero falla. Se traduce "" a nil aquí, en
	// el borde, y no en la consulta con un IFNULL: mandar NULL cuando no hay
	// centro ES la ausencia de centro, no un valor raro.
	var centro any
	if v := strings.TrimSpace(c.CostCenter); v != "" {
		centro = v
	}

	_, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method, loss_factor, perishable, shelf_life_days,
		 is_assembled, track_lots, track_serial, imported, cost_center_id, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 costing_method=VALUES(costing_method),
		 loss_factor=VALUES(loss_factor),
		 perishable=VALUES(perishable),
		 shelf_life_days=VALUES(shelf_life_days),
		 is_assembled=VALUES(is_assembled),
		 track_lots=VALUES(track_lots),
		 track_serial=VALUES(track_serial),
		 imported=VALUES(imported),
		 cost_center_id=VALUES(cost_center_id),
		 updated_at=VALUES(updated_at)`,
		c.ProductID, c.Metodo, c.LossFactor, c.Perishable, c.ShelfLife,
		c.IsAssembled, c.TrackLots, c.TrackSerial, c.Imported, centro, ahora)
	if err != nil {
		return fmt.Errorf("guardar configuración de %s: %w", c.ProductID, err)
	}
	return nil
}

func metodoDeCosteoValido(m string) bool {
	for _, x := range costMetodos {
		if x.Codigo == m {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Receta
// ---------------------------------------------------------------------------

// CostReceta es un renglón de receta, con el costo del componente ya resuelto.
type CostReceta struct {
	ProductID   string  `json:"product_id"`
	ComponentID string  `json:"component_id"`
	Nombre      string  `json:"nombre"`
	Cantidad    float64 `json:"cantidad"`
	Unit        string  `json:"unit"`
	UnitCost    float64 `json:"unit_cost"`
	LossFactor  float64 `json:"loss_factor"`
	Total       float64 `json:"total"`
	Step        int     `json:"step"`
}

// listarReceta devuelve los componentes de un producto con su costo.
//
// El costo del componente sale de product_information.cost, que es el mismo CPP
// que usa el kardex. No se recalcula aquí: si la receta dijera una cosa y el
// kardex otra, el costo de la receta y el del inventario no cuadrarían y no
// habría forma de saber cuál de los dos mente.
func (s *Server) listarReceta(db *sql.DB, productID string) ([]CostReceta, error) {
	rows, err := db.Query(`SELECT r.product_id, r.component_id,
		IFNULL(p.product_name,''), r.quantity, r.unit,
		IFNULL(p.cost,0), IFNULL(r.loss_factor,0), r.step_order
		FROM product_recipe r
		LEFT JOIN product_information p ON p.product_id = r.component_id
		WHERE r.product_id = ?
		ORDER BY r.step_order, r.component_id`, productID)
	if err != nil {
		return nil, fmt.Errorf("leer receta de %s: %w", productID, err)
	}
	defer rows.Close()

	var out []CostReceta
	for rows.Next() {
		var r CostReceta
		if err := rows.Scan(&r.ProductID, &r.ComponentID, &r.Nombre,
			&r.Cantidad, &r.Unit, &r.UnitCost, &r.LossFactor, &r.Step); err != nil {
			return nil, fmt.Errorf("leer renglón de receta: %w", err)
		}
		// La merma va por COMPONENTE: la tela se pierde en el corte y el botón
		// no. Una merma única sobre la receta entera carga la culpa a los que no
		// tienen (regla 3.6 del patrón).
		r.Total = r.Cantidad * r.UnitCost * (1 + r.LossFactor)
		out = append(out, r)
	}
	return out, rows.Err()
}

// guardarReceta agrega o reemplaza un componente.
//
// El UPSERT es por (product_id, component_id, step_order), la misma clave que
// declara el UNIQUE de la tabla. Recalcular una receta no puede duplicar
// renglones: un total que cuadra al doble es peor que uno que no cuadra.
func guardarReceta(db *sql.DB, r CostReceta) error {
	if strings.TrimSpace(r.ProductID) == "" || strings.TrimSpace(r.ComponentID) == "" {
		return fmt.Errorf("el producto y el componente son obligatorios")
	}
	if r.Cantidad <= 0 {
		return fmt.Errorf("la cantidad del componente debe ser mayor que cero")
	}
	if r.LossFactor < 0 || r.LossFactor > 1 {
		return fmt.Errorf("la merma del componente va entre 0 y 1, y llegó %.4f", r.LossFactor)
	}
	_, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order)
		VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE quantity=VALUES(quantity), unit=VALUES(unit),
		 loss_factor=VALUES(loss_factor)`,
		r.ProductID, r.ComponentID, r.Cantidad, r.Unit, r.LossFactor, r.Step)
	if err != nil {
		return fmt.Errorf("guardar componente de receta: %w", err)
	}
	return nil
}

// borrarReceta quita un componente por su clave única.
func borrarReceta(db *sql.DB, productID, componentID string, step int) error {
	_, err := db.Exec(`DELETE FROM product_recipe
		WHERE product_id = ? AND component_id = ? AND step_order = ?`,
		productID, componentID, step)
	if err != nil {
		return fmt.Errorf("borrar componente de receta: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Rendimiento
// ---------------------------------------------------------------------------

// CostRendimiento es la relación entre un producto de entrada y uno de salida.
type CostRendimiento struct {
	InputID     string  `json:"input_id"`
	InputName   string  `json:"input_name"`
	OutputID    string  `json:"output_id"`
	OutputName  string  `json:"output_name"`
	Pct         float64 `json:"pct"`
	IsByproduct int     `json:"is_byproduct"`
	Notes       string  `json:"notes"`
}

// listarRendimientos devuelve el rendimiento de un producto DE ENTRADA.
func (s *Server) listarRendimientos(db *sql.DB, inputID string) ([]CostRendimiento, error) {
	rows, err := db.Query(`SELECT y.input_product_id,
		IFNULL(pi.product_name,''), y.output_product_id,
		IFNULL(po.product_name,''), y.expected_pct, y.is_byproduct, y.notes
		FROM product_yield y
		LEFT JOIN product_information pi ON pi.product_id = y.input_product_id
		LEFT JOIN product_information po ON po.product_id = y.output_product_id
		WHERE y.input_product_id = ?
		ORDER BY y.expected_pct DESC`, inputID)
	if err != nil {
		return nil, fmt.Errorf("leer rendimientos de %s: %w", inputID, err)
	}
	defer rows.Close()

	var out []CostRendimiento
	for rows.Next() {
		var y CostRendimiento
		if err := rows.Scan(&y.InputID, &y.InputName, &y.OutputID, &y.OutputName,
			&y.Pct, &y.IsByproduct, &y.Notes); err != nil {
			return nil, fmt.Errorf("leer rendimiento: %w", err)
		}
		out = append(out, y)
	}
	return out, rows.Err()
}

// guardarRendimiento define qué sale de qué.
//
// El porcentaje es la llave del método: sin él el motor no puede repartir el
// costo de la entrada, y lo avisa en vez de calcular mal. Por eso el rango se
// valida aquí y no se deja pasar un 0 ni un 150.
func guardarRendimiento(db *sql.DB, y CostRendimiento) error {
	if strings.TrimSpace(y.InputID) == "" || strings.TrimSpace(y.OutputID) == "" {
		return fmt.Errorf("el producto de entrada y el de salida son obligatorios")
	}
	if y.InputID == y.OutputID {
		return fmt.Errorf("el producto de entrada y el de salida son el mismo: el rendimiento se calcularía sobre sí mismo")
	}
	if y.Pct <= 0 || y.Pct > 100 {
		return fmt.Errorf("el rendimiento va entre 0 y 100 (72 = 72%%), y llegó %.2f", y.Pct)
	}
	_, err := db.Exec(`INSERT INTO product_yield
		(input_product_id, output_product_id, expected_pct, is_byproduct, notes)
		VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE expected_pct=VALUES(expected_pct),
		 is_byproduct=VALUES(is_byproduct), notes=VALUES(notes)`,
		y.InputID, y.OutputID, y.Pct, y.IsByproduct, y.Notes)
	if err != nil {
		return fmt.Errorf("guardar rendimiento: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Grados de mano de obra y centros de costo
// ---------------------------------------------------------------------------

// CostGrado es un rol con su salario y su carga.
type CostGrado struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Salario    float64 `json:"salario"`
	Overhead   float64 `json:"overhead"`
	HorasAnual int     `json:"horas_anual"`
	Productivy float64 `json:"productividad"`
	// Tasa es la tarifa horaria YA CALCULADA. No está en la base a propósito:
	// se calcula en el momento de usarla y se guarda en el asiento de costo.
	// Si estuviera en la tabla y fueraGENERATED, subir el salario reescribiría
	// el costo de todos los proyectos ya facturados (bug 0.4).
	Tasa   float64 `json:"tasa"`
	Activo int     `json:"activo"`
}

func listarGrados(db *sql.DB) ([]CostGrado, error) {
	rows, err := db.Query(`SELECT code, name, base_salary_annual, overhead_pct,
		hours_per_year, productivity_factor, active
		FROM labor_grade ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("leer grados: %w", err)
	}
	defer rows.Close()

	var out []CostGrado
	for rows.Next() {
		var g CostGrado
		if err := rows.Scan(&g.Code, &g.Name, &g.Salario, &g.Overhead,
			&g.HorasAnual, &g.Productivy, &g.Activo); err != nil {
			return nil, fmt.Errorf("leer grado: %w", err)
		}
		// La tasa se calcula aquí, no se lee: es el mismo calcularTasaHoraria
		// que usa el motor, de modo que lo que se ve en pantalla y lo que se
		// cobra no pueden salir distintos.
		g.Tasa = calcularTasaHoraria(g.Salario, g.Overhead, g.Productivy, g.HorasAnual)
		out = append(out, g)
	}
	return out, rows.Err()
}

func guardarGrado(db *sql.DB, g CostGrado) error {
	if strings.TrimSpace(g.Code) == "" {
		return fmt.Errorf("el código del grado es obligatorio")
	}
	if g.HorasAnual <= 0 {
		return fmt.Errorf("las horas anuales deben ser mayores que cero: con cero, la tarifa sería una división entre cero")
	}
	if g.Salario < 0 || g.Overhead < 0 {
		return fmt.Errorf("el salario y la carga no pueden ser negativos")
	}
	_, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year,
		 productivity_factor, active)
		VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE name=VALUES(name),
		 base_salary_annual=VALUES(base_salary_annual),
		 overhead_pct=VALUES(overhead_pct),
		 hours_per_year=VALUES(hours_per_year),
		 productivity_factor=VALUES(productivity_factor),
		 active=VALUES(active)`,
		g.Code, g.Name, g.Salario, g.Overhead, g.HorasAnual, g.Productivy, g.Activo)
	if err != nil {
		return fmt.Errorf("guardar grado %s: %w", g.Code, err)
	}
	return nil
}

// CostCentro es un centro de costo con su tasa de indirecto.
type CostCentro struct {
	ID         int     `json:"id"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	TasaHora   float64 `json:"tasa_hora"`
	TasaUnidad float64 `json:"tasa_unidad"`
	Activo     int     `json:"activo"`
}

func listarCentros(db *sql.DB) ([]CostCentro, error) {
	rows, err := db.Query(`SELECT id, code, name, type,
		overhead_rate_hour, overhead_rate_unit, active
		FROM cost_center ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("leer centros de costo: %w", err)
	}
	defer rows.Close()

	var out []CostCentro
	for rows.Next() {
		var c CostCentro
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Type,
			&c.TasaHora, &c.TasaUnidad, &c.Activo); err != nil {
			return nil, fmt.Errorf("leer centro de costo: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func guardarCentro(db *sql.DB, c CostCentro) error {
	if strings.TrimSpace(c.Code) == "" || strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("el código y el nombre del centro son obligatorios")
	}
	_, err := db.Exec(`INSERT INTO cost_center
		(code, name, type, overhead_rate_hour, overhead_rate_unit, active)
		VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE name=VALUES(name), type=VALUES(type),
		 overhead_rate_hour=VALUES(overhead_rate_hour),
		 overhead_rate_unit=VALUES(overhead_rate_unit), active=VALUES(active)`,
		c.Code, c.Name, c.Type, c.TasaHora, c.TasaUnidad, c.Activo)
	if err != nil {
		return fmt.Errorf("guardar centro de costo %s: %w", c.Code, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Listas de precio
// ---------------------------------------------------------------------------

// CostLista es una lista de precio con su nombre y si es la de por defecto.
type CostLista struct {
	ID         int    `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	PorDefecto int    `json:"por_defecto"`
	Activo     int    `json:"activo"`
	Productos  int    `json:"productos"`
}

func listarListas(db *sql.DB) ([]CostLista, error) {
	rows, err := db.Query(`SELECT l.id, l.code, l.name, l.is_default, l.active,
		(SELECT COUNT(*) FROM product_price_list pl WHERE pl.price_list_id = l.id)
		FROM price_list l ORDER BY l.sort_order, l.code`)
	if err != nil {
		return nil, fmt.Errorf("leer listas de precio: %w", err)
	}
	defer rows.Close()

	var out []CostLista
	for rows.Next() {
		var l CostLista
		if err := rows.Scan(&l.ID, &l.Code, &l.Name, &l.PorDefecto, &l.Activo, &l.Productos); err != nil {
			return nil, fmt.Errorf("leer lista de precio: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// guardarPrecioEnLista asigna un precio a un producto en una lista.
//
// El precio NO se recalcula desde el costo aquí. Se guarda el número que el
// usuario escribió. Si se calculara, cambiar el costo cambiaría el precio de
// venta de artículos que ya se，乐侬 quoted, y eso no lo debería decidir un
// módulo de costos.
func guardarPrecioEnLista(db *sql.DB, productID string, listaID int, precio float64) error {
	if strings.TrimSpace(productID) == "" {
		return fmt.Errorf("el producto es obligatorio")
	}
	if listaID <= 0 {
		return fmt.Errorf("la lista de precio es obligatoria")
	}
	if precio < 0 {
		return fmt.Errorf("el precio no puede ser negativo")
	}
	_, err := db.Exec(`INSERT INTO product_price_list
		(product_id, price_list_id, price, updated_at)
		VALUES (?,?,?,NOW())
		ON DUPLICATE KEY UPDATE price=VALUES(price), updated_at=VALUES(updated_at)`,
		productID, listaID, precio)
	if err != nil {
		return fmt.Errorf("guardar precio en lista: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Hoja de horas
// ---------------------------------------------------------------------------

// CostHoraFila es un renglón de la hoja de horas, como se ve en pantalla.
type CostHoraFila struct {
	ID         string  `json:"id"`
	Empleado   string  `json:"empleado"`
	Nombre     string  `json:"nombre"`
	Fecha      string  `json:"fecha"`
	Horas      float64 `json:"horas"`
	Facturable int     `json:"facturable"`
	Producto   string  `json:"producto"`
	Grade      string  `json:"grade"`
	Notas      string  `json:"notas"`
	Tasa       float64 `json:"tasa"`
	Costo      float64 `json:"costo"`
}

// listarHoras lee la hoja de horas de un período, con la tarifa de cada grado
// YA APLICADA.
//
// Las horas no facturables también se listan y también cuestan dinero: no
// facturables no es lo mismo que gratis. El costo de una reunión interna es real
// y si no se ve, el margen del proyecto sale inflado.
func (s *Server) listarHoras(db *sql.DB, periodo string, productoID string) ([]CostHoraFila, error) {
	args := []any{periodo + "%"}
	q := `SELECT t.id, t.employee_id,
		IFNULL(CONCAT(e.first_name,' ',e.last_name),''), t.work_date, t.hours,
		t.billable, IFNULL(t.product_id,''), t.grade, t.notes
		FROM cost_timesheet t
		LEFT JOIN hr_employee e ON e.id = t.employee_id
		WHERE t.work_date LIKE ?`
	if strings.TrimSpace(productoID) != "" {
		q += ` AND t.product_id = ?`
		args = append(args, productoID)
	}
	q += ` ORDER BY t.work_date DESC, t.id DESC`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("leer hoja de horas: %w", err)
	}
	defer rows.Close()

	var out []CostHoraFila
	var horasFact, horasNoFact float64
	for rows.Next() {
		var h CostHoraFila
		var id int
		if err := rows.Scan(&id, &h.Empleado, &h.Nombre, &h.Fecha, &h.Horas,
			&h.Facturable, &h.Producto, &h.Grade, &h.Notas); err != nil {
			return nil, fmt.Errorf("leer renglón de horas: %w", err)
		}
		h.ID = fmt.Sprint(id)
		h.Tasa = s.tasaDeGrado(db, h.Grade)
		h.Costo = h.Horas * h.Tasa
		if h.Facturable == 1 {
			horasFact += h.Horas
		} else {
			horasNoFact += h.Horas
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Los dos totales se devuelven como filas de pseudo-datos que la pantalla
	// usa de pie de tabla. Se acumulan UNA vez cada uno, por su propia clave.
	out = append(out, CostHoraFila{ID: "total", Horas: horasFact, Notas: "horas facturables"})
	out = append(out, CostHoraFila{ID: "total-nofact", Horas: horasNoFact, Notas: "horas no facturables (costo real, no ingreso)"})
	return out, nil
}

// guardarHora registra horas de un empleado.
//
// El empleado llega como texto porque viene de un formulario: se convierte y se
// valida aquí, en el borde, y no en el handler. Convertir sin comprobar
// devuelve 0 para cualquier cosa que no sea un número, y 0 pasa sin que nadie
// se entere hasta que la hoja de horas suma horas de un empleado que no existe.
func guardarHora(db *sql.DB, empleadoID, fecha, productoID, grade, notas string, horas float64, facturable bool, creadoPor string) error {
	id, err := strconv.Atoi(strings.TrimSpace(empleadoID))
	if err != nil || id <= 0 {
		return fmt.Errorf("el empleado es obligatorio y tiene que ser un número, llegó %q", empleadoID)
	}
	if fecha == "" {
		return fmt.Errorf("la fecha es obligatoria")
	}
	if horas <= 0 {
		return fmt.Errorf("las horas deben ser mayores que cero")
	}
	if horas > 24 {
		return fmt.Errorf("no se pueden registrar más de 24 horas en un día (se inducía %.2f)", horas)
	}
	if strings.TrimSpace(grade) == "" {
		return fmt.Errorf("el grado es obligatorio: sin él las horas se costean a cero")
	}
	_, err = db.Exec(`INSERT INTO cost_timesheet
		(employee_id, work_date, hours, billable, product_id, grade, notes, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,NOW())`,
		id, fecha, horas, boolABit(facturable), productoID, grade, notas, creadoPor)
	if err != nil {
		return fmt.Errorf("guardar horas: %w", err)
	}
	return nil
}

func borrarHora(db *sql.DB, id int) error {
	if id <= 0 {
		return fmt.Errorf("el identificador no es válido")
	}
	_, err := db.Exec(`DELETE FROM cost_timesheet WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("borrar horas: %w", err)
	}
	return nil
}

func boolABit(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Estado del período
// ---------------------------------------------------------------------------

// CostEstadoPeriodo es el estado de cierre de un mes de costos.
type CostEstadoPeriodo struct {
	Periodo           string `json:"periodo"`
	Status            string `json:"status"`
	ProductosCerrados int    `json:"productos_cerrados"`
	CerradoPor        string `json:"cerrado_por"`
	CerradoEl         string `json:"cerrado_el"`
	ReabiertoPor      string `json:"reabierto_por"`
	Motivo            string `json:"motivo"`
	Calculados        int    `json:"calculados"`
	Productos         int    `json:"productos"`
}

func leerEstadoPeriodo(db *sql.DB, periodo string) (CostEstadoPeriodo, error) {
	e := CostEstadoPeriodo{Periodo: periodo, Status: "abierto"}
	var status, por, el, reabPor, reabEl, motivo sql.NullString
	var n int
	err := db.QueryRow(`SELECT status, products_closed, closed_by, closed_at,
		reopened_by, reopened_at, reopened_reason
		FROM cost_period_close WHERE period = ?`, periodo).
		Scan(&status, &n, &por, &el, &reabPor, &reabEl, &motivo)
	if err == sql.ErrNoRows {
		// Sin fila, el período está abierto: es el estado por defecto y no un
		// error. Solo se cuentan los productos ya calculados.
		_ = db.QueryRow(`SELECT COUNT(DISTINCT product_id) FROM cost_component
			WHERE period = ?`, periodo).Scan(&e.Calculados)
		_ = db.QueryRow(`SELECT COUNT(*) FROM product_information`).Scan(&e.Productos)
		return e, nil
	}
	if err != nil {
		return e, fmt.Errorf("leer estado de %s: %w", periodo, err)
	}
	if status.Valid {
		e.Status = status.String
	}
	e.ProductosCerrados = n
	e.CerradoPor = por.String
	e.CerradoEl = el.String
	e.ReabiertoPor = reabPor.String
	e.Motivo = motivo.String
	_ = db.QueryRow(`SELECT COUNT(DISTINCT product_id) FROM cost_component
		WHERE period = ?`, periodo).Scan(&e.Calculados)
	_ = db.QueryRow(`SELECT COUNT(*) FROM product_information`).Scan(&e.Productos)
	return e, nil
}

// reabrirPeriodo deja un período abierto otra vez.
//
// Lleva motivo OBLIGATORIO y queda en la tabla quién lo reabrió y por qué. Un
// cierre que se puede deshacer sin dejar rastro no es un cierre: es una
// sugerencia. Y es la excepción al congelado, así que tiene que costar: obliga a
// escribir por qué.
func reabrirPeriodo(db *sql.DB, periodo, usuario, motivo string) error {
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("explica por qué se reabre el período: es la única huella de que el histórico se tocó")
	}
	_, err := db.Exec(`INSERT INTO cost_period_close
		(period, status, reopened_reason, reopened_by, reopened_at)
		VALUES (?, 'abierto', ?, ?, NOW())
		ON DUPLICATE KEY UPDATE status='abierto', reopened_reason=VALUES(reopened_reason),
		 reopened_by=VALUES(reopened_by), reopened_at=VALUES(reopened_at)`,
		periodo, motivo, usuario)
	if err != nil {
		return fmt.Errorf("reabrir %s: %w", periodo, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Utilidades compartidas
// ---------------------------------------------------------------------------

// periodovalidado normaliza un período a YYYY-MM y lo rechaza si no lo es.
//
// Sin esto, un período mal escrito llega hasta la tabla como texto libre y el
// cierre congela un mes que no existe. Se valida aquí, una vez, en el borde.
func periodoValidado(p string) (string, error) {
	p = strings.TrimSpace(p)
	if len(p) == 7 && p[4] == '-' {
		mes := p[5:]
		if mes >= "01" && mes <= "12" {
			return p, nil
		}
	}
	if p == "" {
		return time.Now().Format("2006-01"), nil
	}
	return "", fmt.Errorf("el período debe tener el formato AAAA-MM (por ejemplo 2026-10), y llegó %q", p)
}
