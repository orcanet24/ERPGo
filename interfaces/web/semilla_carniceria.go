package web

// semilla_carniceria.go — Carga la empresa Carnicería Lara con cinco meses de
// operación.
//
// QUÉ ES Y POR QUÉ NO ES UN SQL.
//
// Esto NO es un `INSERT` suelto. Cada compra y cada venta pasa por los handlers
// de verdad (`purchaseSave`, `invoiceSave`), y cada renglón de venta consume
// lotes con el FEFO de verdad. La razón es una sola: un seeder que escribiera
// directo en las tablas dejaría el libro mayor vacío, el inventario sin cuadrar
// con los lotes y la cobranza sin tocar. Cargar "datos de prueba" por la puerta
// de atrás produce una base que parece llena y no sirve para probar nada.
//
// En una instalación con contabilidad esto es la diferencia entre una empresa
// real y un simulacro: el asiento contable lo genera el mismo código que lo
// genera en producción, con la misma cuenta de inventario y el mismo plan de
// 1804 cuentas.
//
// LOS NÚMEROS SON DEL PLAN, NO MEDIDOS.
//
// Vienen de `doc/test/empresas/carniceria-lara/`. Los que el plan declara como
// proyección se usan como tales y se marcan aquí con `PROYECCIÓN`. El que NO es
// del plan y seDECIDE aquí es el rendimiento del despiece (72%), que el propio
// plan marca como el bloqueo nº 1: sin pesar una canal no hay rendimiento
// verdadero. Está en `const` para que cambiarlo sea una edición de una línea.
//
// QUÉ NO HACE.
//
// No crea el plan de cuentas (el ERP ya trae 1804), ni las listas de precio (las
// crea la migración), ni toca `acc_predefine_account`. Lo que sí necesita del
// ERP es que las migraciones estén aplicadas.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Las constantes del negocio. Aquí es donde se cambian las cifras.
// ---------------------------------------------------------------------------

// rendimientoDespiece es el porcentaje de una canal que sale a venta.
//
// PROYECCIÓN, no medición: es el número que el motor usa para repartir el costo
// de la entrada. El plan de la carnicería señala esto como el bloqueo nº 1
// ("sin pesar la canal, el costo es una suposición"), y lo es: con 72% el corte
// sale a 4,00/4,00... es decir, 1,00/0,72 = 1,39 por kilo vendido. Cuando se
// pese una canal real, se cambia esta línea y se vuelve a sembrar.
const rendimientoDespiece = 72.0

// canalesPorMes son las canales que entran cada mes. Del plan:
// 40, 45, 50, 60 (temporada) y 45.
var canalesPorMes = []int{40, 45, 50, 60, 45}

// kiloPorCanal es el peso de una canal.
const kiloPorCanal = 250.0

// costoPorKilo es el costo de la carne comprada. El plan proyecta 100.000 Bs
// para 10.000 kg, o sea 10,00 Bs/kg, y mantiene ese precio los cinco meses
// (600.000 Bs por 60.000 kg). PROYECCIÓN.
const costoPorKilo = 10.00

// mesInicial es el primer mes del ciclo (AAAA-MM).
const mesInicial = "2026-09"

// nominaBase es el sueldo mensual de los cuatro empleados del plan:
// 45.000 + 25.000 + 22.000 + 20.000 = 112.000.
const nominaBase = 112000.00

// cargaPatronal son las cargas sociales. El plan usa 34% (coincide con el
// `overhead_pct` de los grados sembrados).
const cargaPatronal = 34.00

// InformeSemilla es lo que el semillero devuelve: qué cargó y qué cuadra.
type InformeSemilla struct {
	Base             string
	Meses            []string
	Empleados        int
	Proveedores      int
	Productos        int
	Clientes         int
	Compras          int
	Lotes            int
	Ventas           int
	Facturado        float64
	Cobrado          float64
	PorCobrar        float64
	Comprado         float64
	DebeLibro        float64
	HaberLibro       float64
	Descuadre        float64
	Librovacio       bool // el debe y el haber son 0: no cuadra, no hay libro
	PeriodosCerrados int
	CostosCongelados int
	Lineas           []string // resumen legible, mes a mes
}

// Resumen devuelve el informe en texto, para el final del comando.
func (i *InformeSemilla) Resumen() string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== Carnicería Lara — %s ===\n", i.Base)
	fmt.Fprintf(&b, "Meses:            %s a %s (%d)\n",
		i.Meses[0], i.Meses[len(i.Meses)-1], len(i.Meses))
	fmt.Fprintf(&b, "Empleados:        %d\n", i.Empleados)
	fmt.Fprintf(&b, "Proveedores:      %d\n", i.Proveedores)
	fmt.Fprintf(&b, "Productos:        %d\n", i.Productos)
	fmt.Fprintf(&b, "Clientes:         %d\n", i.Clientes)
	fmt.Fprintf(&b, "Compras / lotes:  %d / %d\n", i.Compras, i.Lotes)
	fmt.Fprintf(&b, "Ventas:           %d\n", i.Ventas)
	fmt.Fprintf(&b, "Comprado:         %.2f\n", i.Comprado)
	fmt.Fprintf(&b, "Facturado:        %.2f\n", i.Facturado)
	fmt.Fprintf(&b, "Cobrado:          %.2f\n", i.Cobrado)
	fmt.Fprintf(&b, "Por cobrar:       %.2f\n", i.PorCobrar)
	fmt.Fprintf(&b, "Debe / haber:     %.2f / %.2f\n", i.DebeLibro, i.HaberLibro)
	fmt.Fprintf(&b, "Descuadre:        %.2f\n", i.Descuadre)
	fmt.Fprintf(&b, "Períodos cerrados: %d (costos congelados: %d productos)\n",
		i.PeriodosCerrados, i.CostosCongelados)
	for _, l := range i.Lineas {
		fmt.Fprintf(&b, "  %s\n", l)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// El semillero
// ---------------------------------------------------------------------------

// SemillarCarniceriaLara carga la empresa completa.
//
// Recibe la base YA ABIERTA y Montada con el esquema. No crea el esquema ni las
// migraciones a propósito: quien la llama decide eso, y un seiller que aplica
// migraciones sobre la base de producción por su cuenta es unAccidente esperando.
func SemillarCarniceriaLara(db *sql.DB, nombreBase string) (*InformeSemilla, error) {
	if db == nil {
		return nil, fmt.Errorf("hace falta una base abierta")
	}

	// El servidor mínimo: una sesión y la base. Los handlers usan dbFor, que cae
	// en s.db cuando la petición no trae ámbito de empresa.
	s := &Server{
		db:           db,
		mux:          http.NewServeMux(),
		sessions:     NewSessionStore(),
		activeTenant: "default",
		tenantDBs:    map[string]*sql.DB{},
		settings:     &AppSettings{Web: WebSetting{Currency: "Bs"}},
	}
	s.settingsByTenant = map[string]*AppSettings{"default": s.settings}

	sess := &SessionData{
		UserID: "1", Username: "admin", Fullname: "Administrador",
		UserLevel: "admin", TenantCode: "default",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	sid := s.sessions.Create(sess)
	s.sessions.Get(sid).CSRFToken = newCSRFToken()

	inf := &InformeSemilla{Base: nombreBase}
	var err error

	if err = prepararEmpresaCarniceria(s, db, sid); err != nil {
		return nil, fmt.Errorf("preparar la empresa: %w", err)
	}
	if err = cargarGradosRealistas(db); err != nil {
		return nil, fmt.Errorf("grados de mano de obra: %w", err)
	}
	if inf.Empleados, err = cargarNomina(s, db, sid); err != nil {
		return nil, fmt.Errorf("nómina: %w", err)
	}
	if inf.Proveedores, err = cargarProveedoresCarniceria(db); err != nil {
		return nil, fmt.Errorf("proveedores: %w", err)
	}
	if inf.Clientes, err = cargarClientesCarniceria(db); err != nil {
		return nil, fmt.Errorf("clientes: %w", err)
	}
	if inf.Productos, err = cargarCatalogoCarniceria(s, db, sid); err != nil {
		return nil, fmt.Errorf("catálogo: %w", err)
	}
	if err = cargarRendimientosCarniceria(db); err != nil {
		return nil, fmt.Errorf("rendimientos: %w", err)
	}
	if err = cargarPreciosCarniceria(db); err != nil {
		return nil, fmt.Errorf("listas de precio: %w", err)
	}

	// --- Los cinco meses ---
	meses := generarMeses(mesInicial, len(canalesPorMes))
	inf.Meses = meses

	for idx, periodo := range meses {
		if err = sembrarMes(s, db, sid, periodo, idx, inf); err != nil {
			return nil, fmt.Errorf("mes %s: %w", periodo, err)
		}
	}

	// --- Aprobación de asientos y cierre de costos ---
	if _, err = aprobarAsientosCarniceria(s, db); err != nil {
		return nil, fmt.Errorf("aprobar asientos: %w", err)
	}
	if err = cerrarCostosCarniceria(s, db, meses); err != nil {
		return nil, fmt.Errorf("cerrar costos: %w", err)
	}
	if err = cobrarCartera(db, inf); err != nil {
		return nil, fmt.Errorf("cobrar: %w", err)
	}

	if err = medirInformeCarniceria(db, inf); err != nil {
		return nil, fmt.Errorf("medir: %w", err)
	}
	return inf, nil
}

// generarMeses devuelve "2026-09", "2026-10"...，数量n.
//
// Se usa AÑO-MES explícito y no un `AddDate` sobre un día fijo: el salto de
// diciembre a enero es el caso donde un "+1 mes" mal hecho se queda en
// diciembre, y un ciclo de cinco meses que se repite cuatro veces en diciembre
// es difícil de notar en el resultado y muy fácil de causes.
func generarMeses(inicio string, n int) []string {
	actual, err := time.Parse("2006-01", inicio)
	if err != nil {
		actual = time.Now()
	}
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, actual.Format("2006-01"))
		actual = actual.AddDate(0, 1, 0)
	}
	return out
}

// ---------------------------------------------------------------------------
// Empresa, grados y nómina
// ---------------------------------------------------------------------------

func prepararEmpresaCarniceria(s *Server, db *sql.DB, sid string) error {
	PrepareCompany(db, "mysql", "Carnes del Lara, C.A.", "J-31234567-5")

	// Departamentos con el nombre del oficio. SeedHR deja doce "General"; se
	// renombran los primeros tres porque el nombre del departamento aparece en
	// los recibos de nómina y en lala interfaz de RRHH.
	for id, nombre := range map[string]string{
		"1": "Corte y carnicería", "2": "Administración", "3": "Limpieza",
	} {
		if _, err := db.Exec(`UPDATE hr_department SET name = ?, status = 1 WHERE id = ?`,
			nombre, id); err != nil {
			return fmt.Errorf("departamento %s: %w", nombre, err)
		}
	}
	puestos := []struct {
		id, dep, nombre string
		sueldo          float64
	}{
		{"1", "1", "Carnicero maestro", 450},
		{"2", "1", "Ayudante de mostrador", 250},
		{"3", "2", "Cajero", 220},
		{"4", "3", "Auxiliar de bodega", 200},
	}
	for _, p := range puestos {
		if _, err := db.Exec(`INSERT INTO hr_position (id, department_id, name, base_salary, status)
			VALUES (?,?,?,?,1)
			ON DUPLICATE KEY UPDATE department_id=VALUES(department_id),
			                        name=VALUES(name), base_salary=VALUES(base_salary)`,
			p.id, p.dep, p.nombre, p.sueldo); err != nil {
			return fmt.Errorf("puesto %s: %w", p.nombre, err)
		}
	}
	return nil
}

// cargarGradosRealistas pone salarios en los grados que la migración sembró con
// cero, para que la hoja de horas se pueda costear.
//
// Sin esto, `tasaDeGrado` devuelve 0 y toda hora registrada cuesta 0: el módulo
// de costos no da error, simplemente dice que el trabajo es gratis.
func cargarGradosRealistas(db *sql.DB) error {
	sueldos := map[string]float64{"maestro": 45000, "ayudante": 25000, "supervisor": 38000}
	for code, anual := range sueldos {
		if _, err := db.Exec(`UPDATE labor_grade
			SET base_salary_annual = ?, overhead_pct = ?, hours_per_year = 2080,
			    productivity_factor = 1.0000, active = 1
			WHERE code = ?`, anual, cargaPatronal, code); err != nil {
			return fmt.Errorf("grado %s: %w", code, err)
		}
	}
	return nil
}

// cargarNomina da de alta a los cuatro empleados del plan y devuelve cuántos
// quedaron.
//
// Los sueldos del plan están en Bs al mes (45.000, 25.000, 22.000, 20.000), y
// el ERP guarda `base_salary` en una unidad de pago con `pay_frequency`. Aquí se
// guarda el SALARIO MENSUAL tal cual y la nómina mensual se registra en
// `hr_payroll_run`; el cálculo mensual delel motor usa ese valor. La conversión a
// anual —que es lo que necesita `labor_grade` para la tarifa horaria— se hace
// multiplicando por 12, y está en un solo sitio para que no se aplique dos veces.
func cargarNomina(s *Server, db *sql.DB, sid string) (int, error) {
	personas := []struct {
		codigo, nombres, apellidos, cedula, depto, puesto string
		sueldoMensual                                     float64
		mesesDeAntiguedad                                 int
	}{
		{"CAR-001", "José", "Ramírez", rifConFormato("V", 12345678), "1", "1", 45000, 26},
		{"CAR-002", "María", "González", rifConFormato("V", 23456789), "1", "2", 25000, 14},
		{"CAR-003", "Luis", "Bracho", rifConFormato("V", 34567890), "2", "3", 22000, 8},
		{"CAR-004", "Ana", "Pérez", rifConFormato("V", 45678901), "3", "4", 20000, 5},
	}
	for _, p := range personas {
		contrato := time.Now().AddDate(0, -p.mesesDeAntiguedad, -6).
			Format("2006-01-02")
		if _, err := db.Exec(`INSERT INTO hr_employee
			(code, national_id, first_name, last_name, email, phone, address,
			 hire_date, department_id, position_id, base_salary, pay_frequency, status, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,1,1,?)`,
			p.codigo, p.cedula, p.nombres, p.apellidos,
			strings.ToLower(p.codigo)+"@carnesdellara.ve", "04141234567",
			"Carrera 5 con calle 8, Barquisimeto",
			contrato, p.depto, p.puesto, p.sueldoMensual, now()); err != nil {
			return 0, fmt.Errorf("contratar a %s %s: %w", p.nombres, p.apellidos, err)
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hr_employee WHERE status = 1`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Proveedores y clientes
// ---------------------------------------------------------------------------

// cargarProveedoresCarniceria da de alta los proveedores del plan: dos
// ganaderos (canales), una avícola (pollo) y uno de insumos.
//
// `address2` es NOT NULL sin valor por defecto (columna heredada del volcado del
// PHP original), así que hay que mandarla SIEMPRE: omitirla da Error 1364 y el
// alta no se hace, con un campo obligatorio invisible en el formulario.
func cargarProveedoresCarniceria(db *sql.DB) (int, error) {
	provs := []struct {
		nombre, rif, contacto string
	}{
		{"Matadero Los Andes C.A.", rifConFormato("J", 40112233), "Don Eladio"},
		{"Frigorífico Lara Frío C.A.", rifConFormato("J", 40556677), "Sra. Carmen"},
		{"Avícola El Palmar C.A.", rifConFormato("J", 40889900), "Don Cheo"},
		{"Insumos La Esquina C.A.", rifConFormato("J", 40224455), "Sr. Pérez"},
	}
	for _, p := range provs {
		if _, err := db.Exec(`INSERT INTO supplier_information
			(supplier_name, address, address2, mobile, contact, phone, city, country, status, rif)
			VALUES (?,?,'',?,?,?,?,?,1,?)`,
			p.nombre, "Carretera Duarte con calle 30", "04141234567",
			p.contacto, "0251234567", "Barquisimeto", "Venezuela", p.rif); err != nil {
			return 0, fmt.Errorf("proveedor %s: %w", p.nombre, err)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM supplier_information WHERE status = 1`).Scan(&n)
	return n, nil
}

// cargarClientesCarniceria da de alta 10 de mostrador y 5 al mayor, según el
// plan (12-15 en total).
//
// `customer_type` es un `tinyint` que el ERP NO USA en ningún sitio: viene del
// volcado del PHP original y su significado es desconocido. El plan lo deja
// como pregunta abierta (P-02). Aquí se usa 1 para mostrador y 2 para mayor, y
// queda escrito: si algún día se averigua que 1 significa otra cosa, se cambia
// en UN sitio, que es `tipoClienteMayor`.
//
// LA LISTA DE PRECIO ES LO QUE DISTINGUE A UNOS DE OTROS, no el tipo: un corte
// tiene el mismo producto y dos precios, y el que decide es
// `product_price_list`.
func cargarClientesCarniceria(db *sql.DB) (int, error) {
	mostrador := []struct{ nombre, rif, tel string }{
		{"María Fernández", rifConFormato("V", 11222333), "04141111111"},
		{"José Blanco", rifConFormato("V", 12233444), "04142222222"},
		{"Carmen Delgado", rifConFormato("V", 13344555), "04143333333"},
		{"Pedro_stride", rifConFormato("V", 14455666), "04144444444"},
		{"Rosa Giménez", rifConFormato("V", 15566777), "04145555555"},
		{"Luis Marcano", rifConFormato("V", 16677888), "04146666666"},
		{"Ana Rodríguez", rifConFormato("V", 17788999), "04147777777"},
		{"Carlos Acosta", rifConFormato("V", 18889000), "04148888888"},
		{"Elena Barrios", rifConFormato("V", 19900111), "04149999999"},
		{"Frank Pérez", rifConFormato("V", 20011222), "04141010101"},
	}
	for _, c := range mostrador {
		// El nombre del cliente es único; se usa INSERT IGNORE para que volver a
		// sembrar no duplique clientes.
		if _, err := db.Exec(`INSERT IGNORE INTO customer_information
			(customer_name, customer_address, address2, customer_mobile, customer_email,
			 email_address, contact, phone, city, state, zip, country, status, create_date, create_by, rif, customer_type)
			VALUES (?,?,'',?,?,?,?,?,?,?,?,?,1,?,'admin',?,1)`,
			c.nombre, "Barquisimeto", c.tel, strings.ToLower(c.nombre[:1])+"@correo.ve",
			strings.ToLower(c.nombre[:1])+"@correo.ve", c.nombre, "0251234567",
			"Barquisimeto", "Lara", "3001", "Venezuela",
			time.Now().Format("2006-01-02"), c.rif); err != nil {
			return 0, fmt.Errorf("cliente %s: %w", c.nombre, err)
		}
	}

	mayor := []struct{ nombre, rif, tel string }{
		{"Restaurante Don Pedro", rifConFormato("J", 31000111), "04145550101"},
		{"Hotel Venetur Lara", rifConFormato("J", 31000222), "04145550202"},
		{"Food Truck El Chivo", rifConFormato("J", 31000333), "04145550303"},
		{"Pollos del Carmen C.A.", rifConFormato("J", 31000444), "04145550404"},
		{"Asadero La Esquina", rifConFormato("J", 31000555), "04145550505"},
	}
	for _, c := range mayor {
		if _, err := db.Exec(`INSERT IGNORE INTO customer_information
			(customer_name, customer_address, address2, customer_mobile, customer_email,
			 email_address, contact, phone, city, state, zip, country, status, create_date, create_by, rif, customer_type)
			VALUES (?,?,'',?,?,?,?,?,?,?,?,?,1,?,'admin',?,2)`,
			c.nombre, "Av. Universidad", c.tel, "compras@"+strings.ToLower(c.nombre[:3])+".ve",
			"compras@"+strings.ToLower(c.nombre[:3])+".ve", c.nombre, "0251234567",
			"Barquisimeto", "Lara", "3001", "Venezuela",
			time.Now().Format("2006-01-02"), c.rif); err != nil {
			return 0, fmt.Errorf("cliente mayor %s: %w", c.nombre, err)
		}
	}

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM customer_information WHERE status = 1`).Scan(&n)
	return n, nil
}

// ---------------------------------------------------------------------------
// Catálogo, rendimiento y precios
// ---------------------------------------------------------------------------

// cargarCatalogoCarniceria crea las categorías y los productos.
//
// El `product_id` es un CÓDIGO NUMÉRICO y no el de ocho letras que genera el ERP.
// Los módulos nuevos (lotes, FEFO, punto de pedido) guardan el producto como
// entero, y MySQL resuelve `product_id = 'a1b2c3d4'` contra 1 comparando 0: no
// devuelve filas y no da error. Es una trampa silenciosa y ya costó un
// diagnóstico entero.
//
// Los códigos 1007 y 1008 son las CANALES: los productos de ENTRADA. No se
// venden. Existen para que `product_yield` pueda decir "de esta canal sale este
// corte", que es la forma correcta de escribir un rendimiento.
func cargarCatalogoCarniceria(s *Server, db *sql.DB, sid string) (int, error) {
	for id, nombre := range map[string]string{
		"10": "Carnes rojas", "20": "Carnes blancas", "30": "Menudencias", "40": "Entrada (canales)",
	} {
		if _, err := db.Exec(`INSERT INTO product_category (category_id, category_name, status)
			VALUES (?,?,1) ON DUPLICATE KEY UPDATE category_name=VALUES(category_name)`,
			id, nombre); err != nil {
			return 0, fmt.Errorf("categoría %s: %w", nombre, err)
		}
	}

	// precio, costoUnitarioInicial
	cat := []struct {
		id, nombre, unidad, categoria string
		precio                        float64
		costo                         float64
	}{
		// El precio va POR ENCIMA del costo que produce el despiece: 10,00/0,72 =
		// 13,89 Bs/kg en el res y 8,00/0,72 = 11,11 en el pollo. Un precio por
		// debajo no es un "precio bajo": es vender por debajo del costo, y el
		// margen del mes sale negativo sin que nada lo avise.
		//
		// El `costo` de aquí va en cero a propósito: el costo real lo escribe
		// `despieceCanal` en el lote del corte. Ponerlo ahora sería inventar un
		// costo que después no lo confirma ningún lote.
		{"1001", "Res de res", "kg", "10", 16.90, 0},
		{"1002", "Corte de res", "kg", "10", 18.50, 0},
		{"1003", "Costilla de res", "kg", "10", 15.90, 0},
		{"1006", "Hígado de res", "kg", "30", 14.50, 0},
		{"1004", "Pollo entero", "kg", "20", 15.50, 0},
		{"1005", "Pechuga de pollo", "kg", "20", 19.90, 0},
		{"1007", "Canal de res (entrada)", "kg", "40", 0, costoPorKilo},
		{"1008", "Canal de pollo (entrada)", "kg", "40", 0, 0},
	}
	for _, p := range cat {
		campos := url.Values{}
		campos.Set("product_id", p.id)
		campos.Set("product_name", p.nombre)
		campos.Set("category_id", p.categoria)
		campos.Set("unit", p.unidad)
		campos.Set("price", strconv.FormatFloat(p.precio, 'f', 2, 64))
		campos.Set("barcode", p.id)
		campos.Set("product_details", p.nombre+" — Carnicería Lara")
		if err := llamarHandler(s, sid, "/product/save", campos, s.productSave); err != nil {
			return 0, fmt.Errorf("producto %s: %w", p.nombre, err)
		}
		if p.costo > 0 {
			if _, err := db.Exec(`UPDATE product_information SET cost = ? WHERE product_id = ?`,
				p.costo, p.id); err != nil {
				return 0, fmt.Errorf("costo de %s: %w", p.nombre, err)
			}
		}
	}

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM product_information WHERE status = 1`).Scan(&n)
	return n, nil
}

// cargarRendimientosCarniceria dice qué sale de cada canal.
//
// El reparto de los porcentajes entre los cortes es PROYECCIÓN: la suma de los
// cortes de vacuno da exactamente `rendimientoDespiece`, y el resto es hueso,
// piel y descarte. Lo que NO es proyección es el 72%, que está en la constante
// de arriba y es el número que hay que pesar.
func cargarRendimientosCarniceria(db *sql.DB) error {
	//
	// EL PORCENTAJE QUE VA AQUÍ ES EL RENDIMIENTO DE VENTA (72%), NO LA CUOTA DE
	// MASA DE CADA CORTE. La razón está en semilla_despiece.go y es la parte que
	// más caro sale si se entiende mal:
	//
	//	El motor reparte el costo de la entrada entre kg × pct.
	//	Con pct=72:  2500 Bs / (250 × 0,72) = 13,89 Bs/kg  <- correcto
	//	Con pct=18:  2500 Bs / (250 × 0,18) = 55,56 Bs/kg  <- el triple
	//
	// La cuota de masa (18%, 30%, 20%, 4%) NO va en esta tabla: `product_yield`
	// tiene un solo porcentaje y su papel es el rendimiento, no el reparto. La
	// masa vive en el plan de `repartoCanal`.
	rend := []struct {
		entrada, salida string
		notas           string
	}{
		{"1007", "1001", "Res de res: 45 kg por canal de 250 kg (18% de la masa)"},
		{"1007", "1002", "Corte de res: 75 kg por canal (30% de la masa)"},
		{"1007", "1003", "Costilla y falda: 50 kg por canal (20% de la masa)"},
		{"1007", "1006", "Hígado y menudencia roja: 10 kg por canal (4% de la masa)"},
		{"1008", "1004", "Pollo entero faenado: 155 kg por canal (62% de la masa)"},
		{"1008", "1005", "Pechuga: 25 kg por canal (10% de la masa)"},
	}
	for _, r := range rend {
		if _, err := db.Exec(`INSERT INTO product_yield
			(input_product_id, output_product_id, expected_pct, is_byproduct, notes)
			VALUES (?,?,?,0,?)
			ON DUPLICATE KEY UPDATE expected_pct=VALUES(expected_pct), notes=VALUES(notes)`,
			r.entrada, r.salida, rendimientoDespiece, r.notas); err != nil {
			return fmt.Errorf("rendimiento %s -> %s: %w", r.entrada, r.salida, err)
		}
	}

	// El método de costeo: los cortes se costean por RENDIMIENTO (no por
	// promedio ponderado), porque el costo de un corte es el de la canal
	// repartido por lo que sale. Las canales son YIELD por entrada igual, para
	// que su costo se reparta.
	for _, id := range []string{"1001", "1002", "1003", "1004", "1005", "1006", "1007", "1008"} {
		if _, err := db.Exec(`INSERT INTO product_cost_config
			(product_id, costing_method, perishable, shelf_life_days, is_assembled, track_lots, imported, updated_at)
			VALUES (?, 'YIELD', ?, ?, 1, 1, 0, NOW())
			ON DUPLICATE KEY UPDATE costing_method=VALUES(costing_method),
			                        perishable=VALUES(perishable),
			                        shelf_life_days=VALUES(shelf_life_days)`,
			id, boolABit(id != "1007" && id != "1008"), 7); err != nil {
			return fmt.Errorf("configuración de costeo de %s: %w", id, err)
		}
	}
	return nil
}

// cargarPreciosCarniceria pone el precio al mayor de cada producto.
//
// El precio de DETALLE ya está en `product_information.price`, que es lo que usa
// la venta de mostrador. El de MAYOR va en `product_price_list`, y la diferencia
// es la que hace posible que un corte tenga dos precios.
//
// 82% del precio de mostrador al mayor: un 18% de descuento, que es lo que una
// carnicería de barrio le da a un restaurante que compra todos los días.
func cargarPreciosCarniceria(db *sql.DB) error {
	var id, listaMayor string
	if err := db.QueryRow(`SELECT id FROM price_list WHERE code = 'mayor'`).Scan(&listaMayor); err != nil {
		return fmt.Errorf("no está la lista 'mayor': ¿se aplicó schema_z_costos.sql? %w", err)
	}

	filas, err := db.Query(`SELECT product_id, price FROM product_information
		WHERE price > 0`)
	if err != nil {
		return err
	}
	defer filas.Close()
	for filas.Next() {
		var pID, precioCrudo any
		if err := filas.Scan(&pID, &precioCrudo); err != nil {
			return err
		}
		f := valorNumerico(precioCrudo)
		if f == nil || *f <= 0 {
			continue
		}
		mayor := round2(*f * 0.82)
		if _, err := db.Exec(`INSERT INTO product_price_list
			(product_id, price_list_id, price, margin_pct, updated_at)
			VALUES (?,?,?,0,NOW())
			ON DUPLICATE KEY UPDATE price=VALUES(price)`,
			pID, listaMayor, mayor); err != nil {
			return fmt.Errorf("precio mayor de %v: %w", pID, err)
		}
	}
	_ = id
	return filas.Err()
}
