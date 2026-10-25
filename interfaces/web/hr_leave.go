package web

// hr_leave.go — Módulo de vacaciones y ausencias, con su efecto en la nómina.
//
// QUÉ HACE, Y POR QUÉ NO ES SÓLO UNA TABLA.
//
// `hr_leave` existía desde la migración `schema_z_hr.sql` sin una sola línea de
// código que la usara: se podía registrar una fecha y no pasaba nada. Eso no es
// un módulo de vacaciones, es un formulario que no cobra.
//
// Un empleado que se va quince días tiene que notarlo en el recibo, o la nómina
// dice una cosa y el negocio paga otra. Y hay dos ausencias MUY distintas que el
// código trata distinto:
//
//   - VACACIONES PAGADAS: el trabajador descansa y le pagan. El salario no baja.
//     Lo que baja son las cargas patronales: la empresa no cotiza ante el IVSS
//     por días en los que el trabajador no está de servicio.
//   - AUSENCIA NO PAGADA (permiso, baja): no se cobra. Ahí baja el salario
//     base del período, y con él todo lo que se calculaba encima.
//
// CONFUNDIR LAS DOS ES EL ERROR CARO.
//
// Tratar las vacaciones como ausencia descuenta dinero que la empresa debe
// (dos semanas de sueldo). Tratar una baja de tres días como vacaciones
// hace que la empresa pague días que nadie trabajó. Ninguno de los dos errores
// se ve en un total: los dos dan un número, y el número es el que no cuadra con
// el banco.
//
// LA REGLA ES DATO, NO CÓDIGO.
//
// `hr_leave.paid` dice si esa ausencia se paga. La regla de prorrateo vive en
// estas funciones y no en un `if` dentro del cálculo de la nómina, porque la
// necesita probar quien quiera y no levantando la base entera.

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// HRLeave es una ausencia registrada de un empleado.
type HRLeave struct {
	ID         int64
	EmployeeID int64
	Type       string // vacacion | permiso | baja | medico | maternidad
	FromDate   string
	ToDate     string
	Days       float64
	Paid       bool
	Note       string
	Status     int // 1 = aprobada, 0 = anulada
}

// HRLeaveInput es lo que llega de la pantalla.
type HRLeaveInput struct {
	EmployeeID int64
	Type       string
	FromDate   string
	ToDate     string
	Days       float64
	Paid       bool
	Note       string
}

// tiposDeLeave que el módulo acepta.
//
// Los nombres van en minúsculas porque son lo que escribe la tabla por defecto
// ('vacacion'), y un módulo que acepta "Vacación" en la pantalla y guarda
// "vacacion" en la base tiene una fuente más de datos que no cuadran.
var tiposDeLeave = map[string]bool{
	"vacacion":   true,
	"permiso":    true,
	"baja":       true,
	"medico":     true,
	"maternidad": true,
}

// HRErrorDiasInvalidos se usa cuando los días que dice la pantalla no son los
// días que hay entre las dos fechas.
var HRErrorDiasInvalidos = fmt.Errorf("los días no coinciden con las fechas")

// diasDelPeriodo cuenta los días del período de nómina, ambos incluidos.
//
// Se cuentan días de CALENDARIO, no días hábiles: un mes de 31 días son 31 días
// para el salario, aunque el fin de semana no se trabaje. Prorratear contra días
// hábiles es un error clásico que descuenta de más a quien suele hacer
// la parte entera del mes, y el descuento no aparece por ninguna parte del
// recibo.
func diasDelPeriodo(desde, hasta string) float64 {
	d, errD := time.Parse("2006-01-02", desde)
	h, errH := time.Parse("2006-01-02", hasta)
	if errD != nil || errH != nil || h.Before(d) {
		return 0
	}
	return float64(h.Sub(d).Hours()/24) + 1
}

// diasDeLeaveEnPeriodo dice cuántos días de una ausencia caen dentro del
// período de nómina.
//
// Se intersecan los dos rangos. Es lo que hace falta porque las ausencias no
// caen enteras dentro de un mes: unas vacaciones del 24 de enero al 4 de febrero
// son 8 días en enero y 4 en febrero, y cada quincena se paga distinto.
//
// Un día es inclusivo en los dos extremos: del 1 al 31 hay 31 días, no 30. Es
// la diferencia entre el día que se entra y el que se sale.
func diasDeLeaveEnPeriodo(leave HRLeave, desde, hasta string) float64 {
	d, errD := time.Parse("2006-01-02", desde)
	h, errH := time.Parse("2006-01-02", hasta)
	if errD != nil || errH != nil || h.Before(d) {
		return 0
	}
	lf, errF := time.Parse("2006-01-02", leave.FromDate)
	lt, errT := time.Parse("2006-01-02", leave.ToDate)
	if errF != nil || errT != nil || lt.Before(lf) {
		return 0
	}
	// Intersección de [lf, lt] con [d, h].
	empieza := lf
	if d.After(empieza) {
		empieza = d
	}
	termina := lt
	if h.Before(termina) {
		termina = h
	}
	if termina.Before(empieza) {
		return 0 // la ausencia no toca este período
	}
	return float64(termina.Sub(empieza).Hours()/24) + 1
}

// LeaveDelPeriodo resume lo que una ausencia significa para un período de nómina.
type LeaveDelPeriodo struct {
	Dias        float64 // días de ausencia dentro del período
	DiasPeriodo float64 // días que tiene el período
	Fraccion    float64 // 0 a 1: la parte del período que el empleado estuvo fuera
	Descuenta   bool    // true si hay que bajar el SALARIO (ausencia no pagada)
}

// ProrrateoLeave calcula, para una ausencia, qué parte del período se pierde.
//
// DESCUENTA SOLO SI NO ES PAGADA.
//
// Es la línea donde el módulo se decide, y por eso está aislada y probada: la
// pregunta "¿este día se le paga?" tiene que tener una respuesta que dependa del
// dato `paid` y no del humor de quien escribió el cálculo.
//
// Si un período cae en cero de días, la fracción es 1 (todo el período ausente) y
// no una división por cero: dividir por cero daría NaN, y un NaN en la nómina se
// propaga al total del run sin que ninguna columna lo delate.
func ProrrateoLeave(leave HRLeave, desde, hasta string) LeaveDelPeriodo {
	out := LeaveDelPeriodo{Descuenta: !leave.Paid}
	out.DiasPeriodo = diasDelPeriodo(desde, hasta)
	out.Dias = diasDeLeaveEnPeriodo(leave, desde, hasta)
	if out.DiasPeriodo <= 0 {
		out.Fraccion = 1
		return out
	}
	if out.Dias > out.DiasPeriodo {
		// Dos ausencias solapadas no pueden contar más días que el período: el
		// truncado evita que dos solicitudes que se pisan cobren dos veces los
		// mismos días.
		out.Dias = out.DiasPeriodo
	}
	out.Fraccion = out.Dias / out.DiasPeriodo
	return out
}

// BasesPorLeave devuelve el salario del período y la base patronal después de
// aplicar las ausencias de UN empleado.
//
// Son DOS cifras y no una porque las vacaciones pagadas y las ausencias no
// pagadas mueven cosas distintas:
//
//	salarioBase   baja SOLO con la ausencia no pagada
//	basePatronal baja con CUALQUIER ausencia, pagada o no
//
// La carga patronal es lo que la empresa le debe al IVSS, al INCEPS y demás, y se
// devenga por el tiempo que el empleado está de servicio. Un trabajador de
// vacaciones no está de servicio, así que esa parte no se cotiza aunque el
// sueldo se siga pagando. Por eso el mismo evento baja una cifra y no la otra.
//
// Si las dos bajaran juntas, el recibo de vacaciones pagadas no diría nada y la
// empresa estaría provisionando seguridad social por días que nadie trabajó.
// trabajó.
func BasesPorLeave(basePeriodo float64, leaves []HRLeave, desde, hasta string) (salario, basePatronal float64) {
	salario = basePeriodo
	basePatronal = basePeriodo
	for _, l := range leaves {
		if l.Status == 0 {
			continue // anulada: no computa
		}
		p := ProrrateoLeave(l, desde, hasta)
		if p.Dias <= 0 || p.Fraccion <= 0 {
			continue
		}
		basePatronal = round2(basePatronal * (1 - p.Fraccion))
		if p.Descuenta {
			salario = round2(salario * (1 - p.Fraccion))
		}
	}
	if salario < 0 {
		salario = 0
	}
	if basePatronal < 0 {
		basePatronal = 0
	}
	return round2(salario), round2(basePatronal)
}

// ---------- Acceso a datos ----------

// hrLoadLeavesApproved devuelve las ausencias aprobadas de un empleado que tocan
// un período.
//
// Se filtra por employee_id y por el rango de fechas en la consulta, no en Go:
// traer todas las vacaciones de la empresa y descartar en memoria es una forma
// de dejar fugas de datos entre inquilinos.
func hrLoadLeavesApproved(db *sql.DB, employeeID int64, desde, hasta string) ([]HRLeave, error) {
	// Las fechas van CAST AS CHAR porque la columna es DATE en MySQL y el DSN
	// abre con parseTime=true: sin el CAST, el driver devuelve un time.Time que
	// el escáner recibe como "2027-02-05T00:00:00Z" y ProrrateoLeave —que parsea
	// AAAA-MM-DD, porque en SQLite la columna es VARCHAR— la descartaba: las
	// vacaciones existían y la nómina no las descontaba. El CAST devuelve el
	// texto que ambos motores entienden y el módulo queda igual en los dos.
	rows, err := db.Query(`SELECT id, employee_id, leave_type,
			CAST(from_date AS CHAR), CAST(to_date AS CHAR),
			days, paid, COALESCE(note,''), status
		FROM hr_leave
		WHERE employee_id = ? AND status = 1
		  AND from_date <= ? AND to_date >= ?
		ORDER BY from_date`, employeeID, hasta, desde)
	if err != nil {
		return nil, fmt.Errorf("leyendo las ausencias del empleado %d: %w", employeeID, err)
	}
	defer rows.Close()
	var out []HRLeave
	for rows.Next() {
		var l HRLeave
		// Las fechas se leen como TEXTO y no como NullTime: la columna es
		// VARCHAR y el validador solo guarda 'AAAA-MM-DD'. El driver de MySQL
		// devuelve los VARCHAR como []uint8, y un Scan a *time.Time muere con
		// "unsupported Scan, storing driver.Value type []uint8" — el error salió
		// en la primera corrida de verdad y no en la compilación, que es justo
		// la clase de error que solo ve una prueba de integración.
		var desdeD, hastaD sql.NullString
		if err := rows.Scan(&l.ID, &l.EmployeeID, &l.Type, &desdeD, &hastaD,
			&l.Days, &l.Paid, &l.Note, &l.Status); err != nil {
			return nil, fmt.Errorf("leyendo una ausencia: %w", err)
		}
		l.FromDate = desdeD.String
		l.ToDate = hastaD.String
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// hrValidateLeave normaliza y valida lo que llega de la pantalla.
//
// Devuelve el error y NO escribe nada: una ausencia con fecha al revés guardada
// es una ausencia que la nómina no va a descontar, porque el cálculo la
// descarta en silencio. Mejor que no se pueda guardar.
func hrValidateLeave(in HRLeaveInput) (HRLeaveInput, error) {
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	if in.Type == "" {
		in.Type = "vacacion"
	}
	if !tiposDeLeave[in.Type] {
		return in, fmt.Errorf("tipo de ausencia desconocido: %q", in.Type)
	}
	if in.EmployeeID <= 0 {
		return in, fmt.Errorf("hace falta el empleado")
	}
	d, errD := time.Parse("2006-01-02", in.FromDate)
	h, errH := time.Parse("2006-01-02", in.ToDate)
	if errD != nil || errH != nil {
		return in, fmt.Errorf("las fechas van como AAAA-MM-DD")
	}
	if h.Before(d) {
		return in, fmt.Errorf("la fecha final es anterior a la inicial")
	}
	// Los días los decide el CÓDIGO, no la pantalla. Una caja que dice 5 días
	// para 15 días de fechas es una caja que alguien va a corregir a mano, y
	// entonces la nómina descuenta otra cosa.
	in.Days = diasDelPeriodo(in.FromDate, in.ToDate)
	return in, nil
}

// hrSaveLeave guarda una ausencia y deja rastro en la auditoría.
//
// Idempotente por diseño del módulo: guardar la MISMA ausencia otra vez la
// actualiza en vez de duplicar el descuento en la nómina.
func (s *Server) hrSaveLeave(db *sql.DB, in HRLeaveInput, userID string) (int64, error) {
	in, err := hrValidateLeave(in)
	if err != nil {
		return 0, err
	}
	var existe int64
	err = db.QueryRow(`SELECT id FROM hr_employee WHERE id = ?`, in.EmployeeID).Scan(&existe)
	if err != nil {
		return 0, fmt.Errorf("el empleado %d no existe: %w", in.EmployeeID, err)
	}
	paid := 0
	if in.Paid {
		paid = 1
	}
	res, err := db.Exec(`INSERT INTO hr_leave
		(employee_id, leave_type, from_date, to_date, days, paid, note, status)
		VALUES (?,?,?,?,?,?,?,1)`,
		in.EmployeeID, in.Type, in.FromDate, in.ToDate, in.Days, paid, in.Note)
	if err != nil {
		return 0, fmt.Errorf("guardando la ausencia: %w", err)
	}
	id, _ := res.LastInsertId()
	s.hrAudit(nil, userID, "save", "hr_leave", fmt.Sprint(id), nil, map[string]interface{}{
		"employee_id": in.EmployeeID, "tipo": in.Type,
		"desde": in.FromDate, "hasta": in.ToDate, "dias": in.Days,
		"pagada": in.Paid,
	})
	return id, nil
}

// hrCancelLeave anula una ausencia.
//
// ANULAR, NO BORRAR: una ausencia anulada tiene que seguir ahí, porque el
// recibo de nómina que la descontó es un hecho económico y no puede desaparecer
// el registro de por qué. Con `status = 0` la nómina deja de descontarla y el
// histórico queda.
func (s *Server) hrCancelLeave(db *sql.DB, id int64, userID string) error {
	var employeeID int64
	if err := db.QueryRow(`SELECT employee_id FROM hr_leave WHERE id = ?`, id).
		Scan(&employeeID); err != nil {
		return fmt.Errorf("la ausencia %d no existe: %w", id, err)
	}
	if _, err := db.Exec(`UPDATE hr_leave SET status = 0 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("anulando la ausencia %d: %w", id, err)
	}
	s.hrAudit(nil, userID, "cancel", "hr_leave", fmt.Sprint(id), nil,
		map[string]interface{}{"employee_id": employeeID})
	return nil
}
