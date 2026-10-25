package web

import (
	"database/sql"
	"fmt"
	"strings"
)

// cost_timesheet_service.go — Horas efectivamente facturables por proyecto.
//
// LA PREGUNTA QUE ESTE FICHERO RESPONDE
//
// "De las horas que mi gente registró este mes, ¿cuántas le puedo FACTURAR al
// cliente?"
//
// No es "¿cuántas horas hubo?", que es lo que responde un `SUM(hours)`. Entre una
// y otra están los renglones marcados como facturables que no lo son, y esa
// diferencia es la que hace que una empresa de servicios facture por debajo de
// lo que cuesta sin que nadie pueda explicar por qué.
//
// POR QUÉ `billable` NO BASTA
//
// `cost_timesheet.billable` es la INTENCIÓN de quien registró la hora: "esta la
// paga el cliente". Es necesaria y no es suficiente, por cuatro razones que no
// se ven en el renglón:
//
//	1. PRECIO CERRADO. El proyecto se cobra a tanto. Las horas son el costo del
//	   trabajo, no un ingreso: facturarlas por hora sería cobrar dos veces lo
//	   mismo. El renglón puede estar marcado como facturable y aun así no
//	   facturarse — y en una empresa de servicios esto no es la excepción, es el
//	   caso más común.
//
//	2. PERÍODO CERRADO. Si el mes ya se facturó y se cerró, el trabajo de hoy no
//	   entra en esa factura. Cobrarlo exige otra factura, y si no está autorizada,
//	   no se cobra.
//
//	3. PRESUPUESTO AGOTADO. El cliente contrató N horas. La hora 121 no la
//	   contrató. Solo se factura si hay una orden de cambio firmada, y por eso
//	   existe `change_order_amount`: sin ese importe autorizando el excedente, la
//	   hora va a cuenta de la empresa.
//
//	4. TAREA NO FACTURABLE. Si la tarea es "espera de aprobación del cliente" o
//	   "administración interna", la hora no se factura aunque alguien la haya
//	   marcado como facturable. El error es de quien la registró; la tarea es la
//	   que tiene la razón.
//
// POR QUÉ NO SE CALCULA EN UNA CONSULTA SQL SÓLA
//
// Porque las cuatro reglas dependen del ACUMULADO del proyecto, no del renglón:
// el presupuesto se gasta a lo largo de todos los renglones, y el que cae
// fuera del límite es el último, no cualquiera. Una condición por fila no puede
// saber cuál es. Aquí se lee el conjunto, se ordena por fecha y se va descontando
// el presupuesto en orden cronológico, que es como ocurre en la práctica: lo que
// se hizo primero es lo que entra en el presupuesto.
//
// Y cada hora descartada lleva SU MOTIVO, porque "no facturable" no es una
// explicación: un presupuesto agotado se resuelve con una orden de cambio, un
// período cerrado con otra factura, y una tarea mal configurada corrigiendo la
// tarea. Devolver el total sin el motivo obligaría a abrir la hoja entera para
// descubrir por qué.

// ---------------------------------------------------------------------------
// Modelo
// ---------------------------------------------------------------------------

// Motivos por los que una hora marcada como facturable no llega a facturarse.
//
// Son texto estable, no traducible: van a guardarse en la columna
// `nonbillable_reason` y a aparecer en el reporte. Cambiar la cadena cambia lo
// que hay en la base y rompe los informes que leen el histórico.
const (
	// La tarea no es facturable por definición (espera, administración interna).
	noFacturableTarea = "tarea no facturable"
	// El proyecto se factura a precio cerrado: las horas son costo, no ingreso.
	noFacturablePrecioCerrado = "proyecto a precio cerrado"
	// El trabajo interno no se factura nunca.
	noFacturableInterno = "proyecto interno"
	// El proyecto no factura por horas.
	noFacturableSinTarifa = "proyecto sin tarifa por hora"
	// Se agotó el presupuesto en horas pactadas y no hay orden de cambio.
	noFacturablePresupuesto = "presupuesto de horas agotado"
	// El período ya está cerrado y no admite trabajo nuevo.
	noFacturablePeriodoCerrado = "período cerrado"
)

// Modos de facturación. Coinciden con `cost_project.billing_type`.
const (
	facturacionHoras   = "horas"
	facturacionTanto   = "tanto"
	facturacionInterno = "interno"
)

// CostProyecto es un trabajo que se le factura a un cliente.
type CostProyecto struct {
	ID          int64   `json:"id"`
	Codigo      string  `json:"codigo"`
	Nombre      string  `json:"nombre"`
	ClienteID   int64   `json:"cliente_id"`
	Facturacion string  `json:"facturacion"`
	TarifaHora  float64 `json:"tarifa_hora"`
	Presupuesto float64 `json:"presupuesto_horas"`
	Status      string  `json:"status"`
	Notas       string  `json:"notas"`
}

// CostTarea es el "en qué" del registro de horas.
type CostTarea struct {
	ID            int64  `json:"id"`
	ProyectoID    int64  `json:"proyecto_id"`
	Codigo        string `json:"codigo"`
	Nombre        string `json:"nombre"`
	FacturableDef int    `json:"facturable_def"`
	Activa        int    `json:"activa"`
}

// RenglonHora es una línea de la hoja de horas, ya resuelta.
//
// Lleva las tres cosas que hay que distinguir y que se confundían todas:
//
//	Horas        — las que registró la gente
//	Facturable   — las que el cálculo dice que se pueden cobrar
//	NoFacturable — las que NO, y POR QUÉ
type RenglonHora struct {
	ID         int64   `json:"id"`
	EmpleadoID int64   `json:"empleado_id"`
	Empleado   string  `json:"empleado"`
	Fecha      string  `json:"fecha"`
	Horas      float64 `json:"horas"`
	Facturable int     `json:"facturable"`
	TareaID    int64   `json:"tarea_id"`
	Tarea      string  `json:"tarea"`
	ProyectoID int64   `json:"proyecto_id"`
	Proyecto   string  `json:"proyecto"`
	Grade      string  `json:"grade"`
	Notas      string  `json:"notas"`
	Tarifa     float64 `json:"tarifa"`
	Importe    float64 `json:"importe"`
	// Lo que el CÁLCULO decidió, no lo que pidió quien registró:
	FacturableEfectivo bool   `json:"facturable_efectivo"`
	MotivoNoFacturable string `json:"motivo_no_facturable"`
}

// ResumenProyecto es el resultado por proyecto: el número que va a la pantalla.
type ResumenProyecto struct {
	ProyectoID  int64   `json:"proyecto_id"`
	Proyecto    string  `json:"proyecto"`
	Nombre      string  `json:"nombre"`
	Facturacion string  `json:"facturacion"`
	TarifaHora  float64 `json:"tarifa_hora"`
	Presupuesto float64 `json:"presupuesto_horas"`

	// Horas registradas: lo que se registró sin filtrar.
	HorasRegistradas float64 `json:"horas_registradas"`
	// Horas que la gente marcó como facturables (la intención).
	HorasMarcadas float64 `json:"horas_marcadas"`
	// Horas que el cálculo dice que SE COBRAN. Este es el número del título.
	HorasFacturables float64 `json:"horas_facturables"`
	// Lo que se cobra, en dinero, a la tarifa del proyecto.
	ImporteFacturable float64 `json:"importe_facturable"`
	// Horas y dinero que quedan sin poder facturar, y el motivo de cada bloque.
	HorasNoFacturables  float64 `json:"horas_no_facturables"`
	ImporteNoFacturable float64 `json:"importe_no_facturable"`

	// Desglose por motivo. Es un slice y no un mapa para que el orden sea
	// estable en el JSON: un mapa con clave elige el orden y el informe salía
	// distinto en cada ejecución, que hace imposible comparar dos meses.
	Motivos []MotivoDesglose `json:"motivos"`

	// Consumo del presupuesto y su excedente sin autorizar.
	HorasPresupuesto      float64 `json:"horas_presupuesto"`
	HorasExcedente        float64 `json:"horas_excedente"`
	OrdenCambioAutorizada float64 `json:"orden_cambio_autorizada"`
	PresupuestoAgotado    bool    `json:"presupuesto_agotado"`
}

// MotivoDesglose son las horas perdidas por un motivo concreto.
type MotivoDesglose struct {
	Motivo  string  `json:"motivo"`
	Horas   float64 `json:"horas"`
	Importe float64 `json:"importe"`
	// Explicación en una línea para el informe: qué hacer con este bloque.
	Consejo string `json:"consejo"`
}

// ---------------------------------------------------------------------------
// Lectura de proyectos y tareas
// ---------------------------------------------------------------------------

func leerProyectos(db *sql.DB, soloActivos bool) ([]CostProyecto, error) {
	q := `SELECT id, code, name, IFNULL(customer_id,0), billing_type,
		IFNULL(hourly_rate,0), IFNULL(budget_hours,0), status, notes
		FROM cost_project`
	if soloActivos {
		q += ` WHERE status = 'activo'`
	}
	q += ` ORDER BY code`
	filas, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []CostProyecto
	for filas.Next() {
		var p CostProyecto
		if err := filas.Scan(&p.ID, &p.Codigo, &p.Nombre, &p.ClienteID, &p.Facturacion,
			&p.TarifaHora, &p.Presupuesto, &p.Status, &p.Notas); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, filas.Err()
}

func leerProyecto(db *sql.DB, id int64) (CostProyecto, error) {
	var p CostProyecto
	err := db.QueryRow(`SELECT id, code, name, IFNULL(customer_id,0), billing_type,
		IFNULL(hourly_rate,0), IFNULL(budget_hours,0), status, notes
		FROM cost_project WHERE id = ?`, id).
		Scan(&p.ID, &p.Codigo, &p.Nombre, &p.ClienteID, &p.Facturacion,
			&p.TarifaHora, &p.Presupuesto, &p.Status, &p.Notas)
	return p, err
}

func leerTareas(db *sql.DB, proyectoID int64) ([]CostTarea, error) {
	filas, err := db.Query(`SELECT id, project_id, code, name, billable_default, active
		FROM cost_task WHERE project_id = ? ORDER BY code`, proyectoID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []CostTarea
	for filas.Next() {
		var t CostTarea
		if err := filas.Scan(&t.ID, &t.ProyectoID, &t.Codigo, &t.Nombre,
			&t.FacturableDef, &t.Activa); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, filas.Err()
}

// ---------------------------------------------------------------------------
// Lectura de la hoja de horas
// ---------------------------------------------------------------------------

// leerRenglonesDelPeriodo trae los renglones de un período, con la tarea y el
// proyecto ya resueltos.
//
// El LEFT JOIN es deliberado: un renglón de antes de que existiera `task_id`, o
// de un proyecto borrado, tiene que SEGUIR saliendo en la hoja con su nombre
// vacío. Con un INNER desaparece del histórico y la suma del mes deja de
// cuadrar sin que se note cuál renglón falta.
func leerRenglonesDelPeriodo(db *sql.DB, periodo string, proyectoID int64) ([]RenglonHora, error) {
	// `hr_employee` no tiene columna `name`: el nombre está partido en
	// `first_name` y `last_name`. Un `e.name` aquí no falla al compilar —el
	// SQL se arma con cadenas— sino al ejecutar, y el LEFT JOIN convierte el
	// error en un empleado sin nombre en toda la hoja.
	q := `SELECT t.id, t.employee_id,
		COALESCE(NULLIF(CONCAT(IFNULL(e.first_name,''),' ',IFNULL(e.last_name,'')),''), ''),
		DATE_FORMAT(t.work_date,'%Y-%m-%d'), t.hours, t.billable,
		IFNULL(t.task_id,0), IFNULL(ta.code,''),
		IFNULL(t.project_id,0), IFNULL(p.code,''),
		t.grade, t.notes
		FROM cost_timesheet t
		LEFT JOIN hr_employee e ON e.id = t.employee_id
		LEFT JOIN cost_task ta ON ta.id = t.task_id
		LEFT JOIN cost_project p ON p.id = t.project_id
		WHERE SUBSTRING(t.work_date,1,7) = ?`
	args := []interface{}{periodo}
	if proyectoID > 0 {
		q += ` AND t.project_id = ?`
		args = append(args, proyectoID)
	}
	q += ` ORDER BY t.work_date, t.employee_id, t.id`

	filas, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []RenglonHora
	for filas.Next() {
		var h RenglonHora
		if err := filas.Scan(&h.ID, &h.EmpleadoID, &h.Empleado, &h.Fecha, &h.Horas,
			&h.Facturable, &h.TareaID, &h.Tarea, &h.ProyectoID, &h.Proyecto,
			&h.Grade, &h.Notas); err != nil {
			return nil, err
		}
		// Un empleado borrado deja la fila con el nombre vacío. Se dice quién
		// era por su id en vez de dejar un hueco en blanco, que en un resumen
		// de horas es indistinguible de "no se registró".
		if strings.TrimSpace(h.Empleado) == "" {
			h.Empleado = fmt.Sprintf("empleado %d", h.EmpleadoID)
		}
		out = append(out, h)
	}
	return out, filas.Err()
}

// ---------------------------------------------------------------------------
// EL CÁLCULO
// ---------------------------------------------------------------------------

// ResolverProyecto aplica las cuatro reglas a los renglones de un proyecto y
// devuelve, para cada renglón, si se factura y por qué no.
//
// El ORDEN de las reglas importa y es el orden en que van en el código. Se
// comprueba primero lo que no depende de nada (el tipo de proyecto y el tipo de
// tarea) y al final lo que depende del acumulado (el presupuesto). Al revés, un
// proyecto a precio cerrado con el presupuesto agotado reportaría "presupuesto
// agotado" y el consejo que se le da al usuario sería pedir una orden de cambio
// para un proyecto que no se factura por horas. Sería un consejo imposible de
// seguir.
func ResolverProyecto(db *sql.DB, periodo string, p CostProyecto) ([]RenglonHora, ResumenProyecto, error) {
	renglones, err := leerRenglonesDelPeriodo(db, periodo, p.ID)
	if err != nil {
		return nil, ResumenProyecto{}, err
	}

	// Las tareas del proyecto se leen UNA vez y se indexan por id. Consultarlas
	// por renglón sería un N+1: con 400 renglones son 400 viajes a la base para
	// leer la misma tabla de cinco filas.
	tareas, err := leerTareas(db, p.ID)
	if err != nil {
		return nil, ResumenProyecto{}, err
	}

	// La orden de cambio de cada renglón se lee una vez por renglón y se pasa
	// como dato, no como consulta. `resolverRenglones` no sabe qué es SQL: es
	// aritmética, y por eso se puede probar sin base de datos.
	ordenCambios := map[string]float64{}
	for _, h := range renglones {
		var oc float64
		_ = db.QueryRow(`SELECT IFNULL(MAX(change_order_amount),0) FROM cost_timesheet
			WHERE project_id = ? AND work_date = ?`, p.ID, h.Fecha).Scan(&oc)
		ordenCambios[h.Fecha] = oc
	}

	resueltos, res := resolverRenglones(renglones, tareas, p, costoPeriodoCerrado(db, periodo), ordenCambios)
	return resueltos, res, nil
}

// resolverRenglones es EL CRITERIO del módulo, sin base de datos.
//
// POR QUÉ ESTÁ SEPARADO DE LA BASE, Y NO ES COSMÉTICO.
//
// Este bucle no consulta nada: lee filas, aplica cuatro reglas en orden y suma.
// Es aritmética con acumuladores. Dejarlo pegado al `db` obligaba a levantar una
// base MySQL completa —veintiuna migraciones, unos dieciocho segundos— para
// comprobar que "un proyecto a precio cerrado no factura sus horas", que es
// multiplicación pura.
//
// Con esta separación las quince pruebas del criterio corren en milisegundos y
// se pueden escribir casos que antes eran impagables: cuarenta renglones, todas
// las combinaciones de motivo, el presupuesto gastándose línea a línea. Y el
// resultado es el MISMO código para la pantalla y para las pruebas: no hay dos
// implementaciones que puedan discrepar.
//
// Los mapas de entrada son opcionales por si viene nil: un renglón sin tarea
// tampoco tiene por qué tener orden de cambio.
func resolverRenglones(renglones []RenglonHora, tareas []CostTarea, p CostProyecto,
	periodoCerrado bool, ordenCambios map[string]float64) ([]RenglonHora, ResumenProyecto) {

	facturableTarea := map[int64]int{}
	tareaNombre := map[int64]string{}
	for _, t := range tareas {
		facturableTarea[t.ID] = t.FacturableDef
		tareaNombre[t.ID] = t.Codigo + " " + t.Nombre
	}

	res := ResumenProyecto{
		ProyectoID:  p.ID,
		Proyecto:    p.Codigo,
		Nombre:      p.Nombre,
		Facturacion: p.Facturacion,
		TarifaHora:  p.TarifaHora,
		Presupuesto: p.Presupuesto,
	}

	// Motivos en orden fijo. Es un slice para que el JSON salga igual en cada
	// ejecución y dos meses se puedan comparar.
	motivos := map[string]*MotivoDesglose{}
	sumarMotivo := func(motivo string, horas, importe float64, consejo string) {
		m, ok := motivos[motivo]
		if !ok {
			m = &MotivoDesglose{Motivo: motivo, Consejo: consejo}
			motivos[motivo] = m
		}
		m.Horas += horas
		m.Importe += importe
	}

	// El presupuesto se gasta EN ORDEN CRONOLÓGICO. Las horas que lo agotan son
	// las últimas, no las de la fila que casualmente lo pasó.
	//
	// La orden de cambio se VA descontando también: si el cliente autorizó 10
	// horas extra, la undécima vuelve a ser no facturable. Autorizar "lo que
	// falte" sin techo no es una orden de cambio, es no tener presupuesto.
	presupuestoRestante := p.Presupuesto
	ordenCambioRestante := 0.0

	for i := range renglones {
		h := &renglones[i]
		// El nombre de la tarea se resuelve SI HAY TAREA, no si el nombre ya
		// viene lleno. La guarda era `if h.Tarea != ""`, que solo funciona si
		// alguien rellenó el nombre antes: en la consulta de la base sí pasaba
		// (traía `cost_task.code`), y por eso el error no se veía ahí. Pero
		// decía al revés lo que quería decir, y en cuanto los renglones llegan
		// de otro sitio —una importación, una prueba, un futuro endpoint— las
		// horas salen sin nombre de tarea y el usuario cree que nadie registró
		// la tarea.
		if h.TareaID > 0 {
			h.Tarea = tareaNombre[h.TareaID]
		}

		// La tarifa del renglón es la del proyecto. Si el proyecto no factura
		// por horas, la tarifa es 0 y el importe sale 0: es correcto, porque no
		// hay ingreso por facturar.
		h.Tarifa = p.TarifaHora
		h.Importe = h.Horas * h.Tarifa

		res.HorasRegistradas += h.Horas
		if h.Facturable == 1 {
			res.HorasMarcadas += h.Horas
		}

		// --- el renglón ni siquiera se marcó como facturable -------------
		if h.Facturable != 1 {
			h.FacturableEfectivo = false
			h.MotivoNoFacturable = "registrada como no facturable"
			sumarMotivo(h.MotivoNoFacturable, h.Horas, h.Importe,
				"Correcto si el trabajo fue interno. Si era para el cliente, márquela facturable en la hoja.")
			res.HorasNoFacturables += h.Horas
			res.ImporteNoFacturable += h.Importe
			continue
		}

		// --- regla 1: el tipo de proyecto manda sobre la marca -------------
		switch p.Facturacion {
		case facturacionInterno:
			h.FacturableEfectivo = false
			h.MotivoNoFacturable = noFacturableInterno
			sumarMotivo(noFacturableInterno, h.Horas, h.Importe,
				"El trabajo interno nunca se factura. Sus horas son costo de la empresa.")
			res.HorasNoFacturables += h.Horas
			res.ImporteNoFacturable += h.Importe
			continue
		case facturacionTanto:
			h.FacturableEfectivo = false
			h.MotivoNoFacturable = noFacturablePrecioCerrado
			sumarMotivo(noFacturablePrecioCerrado, h.Horas, h.Importe,
				"El proyecto se cobra a precio cerrado: facturar estas horas cobraría dos veces.")
			res.HorasNoFacturables += h.Horas
			res.ImporteNoFacturable += h.Importe
			continue
		}

		if p.TarifaHora <= 0 {
			h.FacturableEfectivo = false
			h.MotivoNoFacturable = noFacturableSinTarifa
			sumarMotivo(noFacturableSinTarifa, h.Horas, h.Importe,
				"El proyecto factura por horas pero no tiene tarifa. Sin tarifa no hay nada que cobrar.")
			res.HorasNoFacturables += h.Horas
			res.ImporteNoFacturable += h.Importe
			continue
		}

		// --- regla 2: la tarea no es facturable ---------------------------
		// Se comprueba el valor por defecto de la tarea. Se mira el defecto y no
		// un estado guardado en el renglón porque el defecto es lo que la tarea
		// ES: si "espera del cliente" deja de ser facturable, todas las horas
		// registradas bajo ella dejan de serlo, incluidas las de la semana
		// pasada. Es lo que evita que el error se propague por la hoja.
		if h.TareaID > 0 {
			if def, ok := facturableTarea[h.TareaID]; ok && def != 1 {
				h.FacturableEfectivo = false
				h.MotivoNoFacturable = noFacturableTarea
				sumarMotivo(noFacturableTarea, h.Horas, h.Importe,
					"La tarea no es facturable por definición. Cambia el defecto de la tarea si ahora sí se cobra.")
				res.HorasNoFacturables += h.Horas
				res.ImporteNoFacturable += h.Importe
				continue
			}
		}

		// --- regla 3: el período ya está cerrado --------------------------
		if periodoCerrado {
			h.FacturableEfectivo = false
			h.MotivoNoFacturable = noFacturablePeriodoCerrado
			sumarMotivo(noFacturablePeriodoCerrado, h.Horas, h.Importe,
				"El período está cerrado: reábrelo o emite una factura nueva para este trabajo.")
			res.HorasNoFacturables += h.Horas
			res.ImporteNoFacturable += h.Importe
			continue
		}

		// --- regla 4: el presupuesto de horas ---------------------------
		// Solo llega aquí lo que de verdad puede facturarse, así que es el
		// momento de consumir presupuesto. Se consume después de todas las
		// reglas anteriores: gastar presupuesto en horas que no se van a cobrar
		// dejaría al proyecto sin margen facturable antes de tiempo.
		//
		// UNA LÍNEA PUEDE QUEDAR PARTIDA, y por eso el renglón no es "todo
		// facturable o nada". Con 2 horas de presupuesto y una línea de 3, se
		// facturan 2 y las 3 quedan de excedente. Marcarla entera como
		// facturable cobraría de más; marcarla entera como no facturable
		// cobraría de menos y dejaría las 2 horas que sí se podían cobrar
		// tiradas. El importe tiene que ser proporcional a la parte que
		// sí se cobra, no el de la línea entera.
		cobrables := h.Horas

		// Primero el presupuesto pactado.
		if presupuestoRestante > 0 {
			delPresupuesto := cobrables
			if delPresupuesto > presupuestoRestante {
				delPresupuesto = presupuestoRestante
			}
			presupuestoRestante -= delPresupuesto
			res.HorasPresupuesto += delPresupuesto
			cobrables -= delPresupuesto
		}

		// Lo que sobra del presupuesto solo se cobra con orden de cambio, y la
		// orden también tiene tope: autorizar "lo que falte" sin techo no es una
		// orden de cambio, es no tener presupuesto.
		if cobrables > 0 {
			var ordenCambio float64
			if ordenCambios != nil {
				ordenCambio = ordenCambios[h.Fecha]
			}
			disponible := ordenCambio - ordenCambioRestante
			if disponible > 0 {
				deLaOrden := cobrables
				if deLaOrden > disponible {
					deLaOrden = disponible
				}
				ordenCambioRestante += deLaOrden
				res.OrdenCambioAutorizada += deLaOrden
				cobrables -= deLaOrden
			}
		}

		if cobrables > 0 {
			// Parte de la línea que no se puede cobrar. Es el presupuesto
			// agotado, aunque otra parte de la MISMA línea sí entre: por eso el
			// motivo va en la parte perdida y no en la línea entera.
			importePerdido := h.Importe * cobrables / h.Horas
			res.HorasExcedente += cobrables
			res.HorasNoFacturables += cobrables
			res.ImporteNoFacturable += importePerdido
			sumarMotivo(noFacturablePresupuesto, cobrables, importePerdido,
				"El cliente contrató un número de horas y ya se agotaron. Pida una orden de cambio para facturar el excedente.")
			if cobrables < h.Horas {
				// PARCIALMENTE facturable. El motivo NO es el mismo que en una
				// línea descartada entera: aquí la hora SÍ se cobra, solo que
				// menos de lo registrado. Poner el mismo texto haría que la pantalla
				// mostrara "no" sobre una línea que se está cobrando.
				h.MotivoNoFacturable = "facturación parcial: " + noFacturablePresupuesto
			} else {
				h.MotivoNoFacturable = noFacturablePresupuesto
			}
		}

		if cobrables < h.Horas {
			// Parte facturable de la línea. El importe es proporcional: con 2 de 3
			// horas cobrables, se factura dos tercios de la línea, no la línea
			// entera ni cero.
			importeCobrable := h.Importe * (h.Horas - cobrables) / h.Horas
			res.HorasFacturables += h.Horas - cobrables
			res.ImporteFacturable += importeCobrable
			// La marca efectiva es "parte sí": el renglón se factura, pero no
			// entero. Sin esto la pantalla lo mostraría como no facturable y el
			// usuario creería que no se cobra nada de él.
			h.FacturableEfectivo = true
			h.Importe = importeCobrable
		}
	}

	res.PresupuestoAgotado = p.Presupuesto > 0 && res.HorasExcedente > 0

	// Los motivos en el orden en que se declararon arriba, no en el orden
	// alfabético ni en el de aparición: el primero que se mira es el primero
	// que aparece, y el informe tiene que enseñar las reglas en ese orden.
	for _, m := range []string{
		"registrada como no facturable",
		noFacturableInterno,
		noFacturablePrecioCerrado,
		noFacturableSinTarifa,
		noFacturableTarea,
		noFacturablePeriodoCerrado,
		noFacturablePresupuesto,
	} {
		if v, ok := motivos[m]; ok {
			v.Horas = costRedondeaHoras(v.Horas)
			res.Motivos = append(res.Motivos, *v)
		}
	}

	return renglones, res
}

// ResolverPeriodo calcula el resumen de TODOS los proyectos con actividad en el
// período, ordenado de mayor a menor facturación.
//
// El orden es por facturación y no por código a propósito: el proyecto que más
// dinero genera es el que hay que mirar primero, y en una pantalla de resumen
// eso no puede depender de cómo se llamó al proyecto.
func ResolverPeriodo(db *sql.DB, periodo string) ([]ResumenProyecto, error) {
	proyectos, err := leerProyectos(db, false)
	if err != nil {
		return nil, err
	}

	var out []ResumenProyecto
	for _, p := range proyectos {
		_, res, err := ResolverProyecto(db, periodo, p)
		if err != nil {
			return nil, err
		}
		// Un proyecto sin horas no ocupa lugar: la pantalla es para decidir
		// sobre lo que pasó, no para listar el catálogo.
		if res.HorasRegistradas <= 0 {
			continue
		}
		out = append(out, res)
	}

	// Ordenación por inserción en lugar de sort.Slice: son unos pocos proyectos
	// y no vale la pena escribir un sort para ordenar diez elementos.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ImporteFacturable > out[j-1].ImporteFacturable; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// costoPeriodoCerrado dice si el período de costos está cerrado.
//
// Se consulta por separado porque se lee UNA vez para todos los renglones. Con
// cuatrocientas líneas, preguntar el estado del período en cada una son
// cuatrocientas consultas idénticas.
func costoPeriodoCerrado(db *sql.DB, periodo string) bool {
	var status string
	err := db.QueryRow(`SELECT status FROM cost_period_close WHERE period = ?`, periodo).Scan(&status)
	if err != nil {
		return false
	}
	return status == "cerrado"
}

// costRedondeaHoras redondea a dos decimales, que es la escala de
// `cost_timesheet.hours` (decimal(10,2)).
func costRedondeaHoras(v float64) float64 {
	if v < 0 {
		return float64(int64(v*100-0.5)) / 100
	}
	return float64(int64(v*100+0.5)) / 100
}

// ---------------------------------------------------------------------------
// Alta y edición
// ---------------------------------------------------------------------------

// guardarProyecto escribe un proyecto nuevo o actualiza el existente.
//
// Se valida el código ANTES de escribir, y se valida contra `uq_cost_project_code`
// con un UPSERT: si el código ya existe se actualiza en vez de reventar con un
// error de clave duplicada que el usuario no sabe interpretar.
func guardarProyecto(db *sql.DB, p CostProyecto, creadoPor string) error {
	p.Codigo = strings.TrimSpace(p.Codigo)
	p.Nombre = strings.TrimSpace(p.Nombre)
	if p.Codigo == "" {
		return fmt.Errorf("el código del proyecto es obligatorio")
	}
	if p.Nombre == "" {
		return fmt.Errorf("el nombre del proyecto es obligatorio")
	}
	if len(p.Codigo) > 20 {
		return fmt.Errorf("el código no puede pasar de 20 caracteres")
	}
	switch p.Facturacion {
	case facturacionHoras, facturacionTanto, facturacionInterno:
	case "":
		p.Facturacion = facturacionHoras
	default:
		return fmt.Errorf("la forma de facturación %q no existe: use horas, tanto o interno", p.Facturacion)
	}

	// Una tarifa negativa o un presupuesto negativo no significan nada, y un
	// cálculo posterior los multiplicaría por las horas dando importes al revés.
	if p.TarifaHora < 0 {
		return fmt.Errorf("la tarifa por hora no puede ser negativa")
	}
	if p.Presupuesto < 0 {
		return fmt.Errorf("el presupuesto de horas no puede ser negativo")
	}

	// Si el proyecto factura a precio cerrado o es interno, no tiene sentido
	// dejarle una tarifa: alguien la pondría y el resumen mostraría un importe
	// facturable que el cálculo luego descarta. Se pone en cero aquí para que el
	// dato guardado sea el que el cálculo usa.
	if p.Facturacion != facturacionHoras {
		p.TarifaHora = 0
	}

	if p.Status == "" {
		p.Status = "activo"
	}
	if p.ID > 0 {
		if _, err := db.Exec(`UPDATE cost_project SET code=?, name=?, customer_id=?,
			billing_type=?, hourly_rate=?, budget_hours=?, status=?, notes=? WHERE id=?`,
			p.Codigo, p.Nombre, p.ClienteID, p.Facturacion, p.TarifaHora,
			p.Presupuesto, p.Status, p.Notas, p.ID); err != nil {
			return fmt.Errorf("actualizar proyecto: %w", err)
		}
		return nil
	}

	if _, err := db.Exec(`INSERT INTO cost_project
		(code, name, customer_id, billing_type, hourly_rate, budget_hours, status, notes, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,NOW())`,
		p.Codigo, p.Nombre, p.ClienteID, p.Facturacion, p.TarifaHora,
		p.Presupuesto, p.Status, p.Notas, creadoPor); err != nil {
		return fmt.Errorf("guardar proyecto: %w", err)
	}
	return nil
}

// guardarTarea escribe una tarea de un proyecto.
//
// Una tarea pertenece a UN proyecto y su código es único dentro de él, no en toda
// la base: dos proyectos pueden tener ambos una tarea "T-DES" sin que estén
// hablando del mismo trabajo.
func guardarTarea(db *sql.DB, t CostTarea) error {
	t.Codigo = strings.TrimSpace(t.Codigo)
	t.Nombre = strings.TrimSpace(t.Nombre)
	if t.ProyectoID <= 0 {
		return fmt.Errorf("la tarea necesita un proyecto")
	}
	if t.Codigo == "" || t.Nombre == "" {
		return fmt.Errorf("el código y el nombre de la tarea son obligatorios")
	}
	if _, err := db.Exec(`INSERT INTO cost_task (project_id, code, name, billable_default, notes, created_at)
		VALUES (?,?,?,?,'',NOW())
		ON DUPLICATE KEY UPDATE name=VALUES(name), billable_default=VALUES(billable_default)`,
		t.ProyectoID, t.Codigo, t.Nombre, boolABit(t.FacturableDef == 1)); err != nil {
		return fmt.Errorf("guardar tarea: %w", err)
	}
	return nil
}

// guardarHoraConTarea registra horas contra un PROYECTO y una TAREA.
//
// Se separa de `guardarHora` (la del módulo de costos, por producto) en vez de
// ampliar su firma: costear un producto y cargar una hora a un proyecto son dos
// hechos distintos del mismo día de trabajo, y meterlos en una fila con diez
// columnas anulables produce una fila con cinco nulos donde nadie sabe cuál
// aplica. Aquí solo se registra lo que se factura y a quién.
func guardarHoraConTarea(db *sql.DB, empleadoID int64, fecha string,
	proyectoID, tareaID int64, grade, notas string, horas float64,
	facturable bool, ordenCambio float64, creadoPor string) error {

	if empleadoID <= 0 {
		return fmt.Errorf("el empleado es obligatorio")
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
	if ordenCambio < 0 {
		return fmt.Errorf("la orden de cambio no puede ser negativa")
	}
	if proyectoID <= 0 {
		return fmt.Errorf("el proyecto es obligatorio: una hora sin proyecto no se puede agrupar ni facturar")
	}

	// La tarea, si viene, TIENE que ser de ese proyecto. Sin esta comprobación se
	// puede colgar la tarea "Desarrollo" del proyecto A en un renglón del
	// proyecto B, y el resumen de B contaría horas de trabajo que no le pertenecen.
	if tareaID > 0 {
		var pertenece int
		if err := db.QueryRow(`SELECT COUNT(*) FROM cost_task WHERE id = ? AND project_id = ?`,
			tareaID, proyectoID).Scan(&pertenece); err != nil {
			return fmt.Errorf("verificar la tarea: %w", err)
		}
		if pertenece == 0 {
			return fmt.Errorf("la tarea indicada no pertenece a ese proyecto")
		}
	}

	var tareaAnterior int64
	_ = db.QueryRow(`SELECT id FROM cost_project WHERE id = ?`, proyectoID).Scan(&tareaAnterior)
	if tareaAnterior == 0 {
		return fmt.Errorf("el proyecto %d no existe", proyectoID)
	}

	if _, err := db.Exec(`INSERT INTO cost_timesheet
		(employee_id, work_date, hours, billable, project_id, task_id,
		 change_order_amount, nonbillable_reason, grade, notes, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,'',?,?,?,NOW())`,
		empleadoID, fecha, horas, boolABit(facturable), proyectoID, tareaID,
		ordenCambio, grade, notas, creadoPor); err != nil {
		return fmt.Errorf("guardar horas: %w", err)
	}
	return nil
}
