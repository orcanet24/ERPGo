package web

import (
	"database/sql"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Rutas de proceso e indirecto por hora (Fase 3 del plan de costos).
//
// process_route define la secuencia de operaciones de un producto:
//   - setup_time_min: tiempo de preparación del lote. NO se prorratea entre
//     unidades: se cobra completo aunque se fabrique una sola pieza.
//   - run_time_sec y machine_time_sec: tiempo por unidad. SÍ se prorratean.
//   - labor_grade: grado de mano de obra que opera la estación.
// ---------------------------------------------------------------------------

// ProcessRoute es una estación de la ruta de proceso de un producto.
type ProcessRoute struct {
	ID             int     `json:"id"`
	ProductID      string  `json:"product_id"`
	Sequence       int     `json:"sequence"`
	WorkCenterID   int     `json:"work_center_id"`
	WorkCenter     string  `json:"work_center"`
	SetupTimeMin   float64 `json:"setup_time_min"`
	RunTimeSec     float64 `json:"run_time_sec"`
	MachineTimeSec float64 `json:"machine_time_sec"`
	LaborGrade     string  `json:"labor_grade"`
	LaborGradeName string  `json:"labor_grade_name"`
	Active         int     `json:"active"`
}

// listarRutas devuelve las rutas de proceso de un producto, ordenadas por
// secuencia.
func listarRutas(db *sql.DB, productID string) ([]ProcessRoute, error) {
	rows, err := db.Query(`SELECT r.id, r.product_id, r.sequence, r.work_center_id,
		IFNULL(c.name,''), r.setup_time_min, r.run_time_sec, r.machine_time_sec,
		IFNULL(r.labor_grade,''), IFNULL(g.name,''), r.active
		FROM process_route r
		LEFT JOIN cost_center c ON c.id = r.work_center_id
		LEFT JOIN labor_grade g ON g.code = r.labor_grade
		WHERE r.product_id = ?
		ORDER BY r.sequence, r.id`, productID)
	if err != nil {
		return nil, fmt.Errorf("leer rutas de proceso de %s: %w", productID, err)
	}
	defer rows.Close()

	var out []ProcessRoute
	for rows.Next() {
		var r ProcessRoute
		if err := rows.Scan(&r.ID, &r.ProductID, &r.Sequence, &r.WorkCenterID,
			&r.WorkCenter, &r.SetupTimeMin, &r.RunTimeSec, &r.MachineTimeSec,
			&r.LaborGrade, &r.LaborGradeName, &r.Active); err != nil {
			return nil, fmt.Errorf("leer ruta: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// guardarRuta crea o actualiza una estación de la ruta de proceso.
func guardarRuta(db *sql.DB, r ProcessRoute, usuario string) error {
	if strings.TrimSpace(r.ProductID) == "" {
		return fmt.Errorf("el producto es obligatorio")
	}
	if r.Sequence < 0 {
		r.Sequence = 0
	}
	if r.SetupTimeMin < 0 {
		r.SetupTimeMin = 0
	}
	if r.RunTimeSec < 0 {
		r.RunTimeSec = 0
	}
	if r.MachineTimeSec < 0 {
		r.MachineTimeSec = 0
	}
	if r.WorkCenterID <= 0 {
		return fmt.Errorf("el centro de trabajo es obligatorio")
	}
	if _, err := db.Exec(`INSERT INTO process_route
		(product_id, sequence, work_center_id, setup_time_min, run_time_sec,
		 machine_time_sec, labor_grade, active)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE sequence=VALUES(sequence),
		 work_center_id=VALUES(work_center_id),
		 setup_time_min=VALUES(setup_time_min),
		 run_time_sec=VALUES(run_time_sec),
		 machine_time_sec=VALUES(machine_time_sec),
		 labor_grade=VALUES(labor_grade),
		 active=VALUES(active)`,
		r.ProductID, r.Sequence, r.WorkCenterID, r.SetupTimeMin, r.RunTimeSec,
		r.MachineTimeSec, r.LaborGrade, r.Active); err != nil {
		return fmt.Errorf("guardar ruta de proceso: %w", err)
	}
	return nil
}

// borrarRuta quita una estación de la ruta.
func borrarRuta(db *sql.DB, id int) error {
	if id <= 0 {
		return fmt.Errorf("el identificador de ruta no es válido")
	}
	_, err := db.Exec(`DELETE FROM process_route WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("borrar ruta de proceso: %w", err)
	}
	return nil
}

// horasRutaPorUnidad devuelve las horas de máquina y de setup por unidad,
// calculadas desde la ruta de proceso.
//
// setup_time_min NO se prorratea entre unidades: se devuelve aparte.
// run_time_sec y machine_time_sec SÍ se prorratean: se dividen por cantidad.
func horasRutaPorUnidad(db *sql.DB, productID string, cantidad float64) (setupHoras, runHoras, machineHoras float64, err error) {
	if cantidad <= 0 {
		cantidad = 1
	}
	rows, err := db.Query(`SELECT setup_time_min, run_time_sec, machine_time_sec
		FROM process_route WHERE product_id = ? AND active = 1`, productID)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var st, rt, mt float64
		if err := rows.Scan(&st, &rt, &mt); err == nil {
			setupHoras += st / 60
			// run y machine se prorratean por unidad.
			runHoras += rt / 3600 / cantidad
			machineHoras += mt / 3600 / cantidad
		}
	}
	return setupHoras, runHoras, machineHoras, rows.Err()
}

// costoManoDeObraRuta devuelve el costo de mano de obra de la ruta por unidad.
// El setup no se prorratea (va a orden de producción, Fase 6); solo run_time se
// valúa con la tarifa del grado.
func costoManoDeObraRuta(db *sql.DB, productID string, cantidad float64) (float64, error) {
	if cantidad <= 0 {
		cantidad = 1
	}
	var total float64
	rows, err := db.Query(`SELECT r.labor_grade, r.run_time_sec
		FROM process_route r
		WHERE r.product_id = ? AND r.active = 1 AND r.labor_grade <> ''`, productID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var grade string
		var runSec float64
		if err := rows.Scan(&grade, &runSec); err == nil {
			tarifa := calcularTasaHorariaDesdeGrado(db, grade)
			if tarifa > 0 {
				// run_time por unidad, valorado a la tarifa del grado.
				total += tarifa * runSec / 3600 / cantidad
			}
		}
	}
	return total, rows.Err()
}

// calcularTasaHorariaDesdeGrado devuelve la tarifa horaria de un grado.
func calcularTasaHorariaDesdeGrado(db *sql.DB, grade string) float64 {
	var salario, cargas, productividad sql.NullFloat64
	var horas int
	if err := db.QueryRow(`SELECT base_salary_annual, overhead_pct, hours_per_year,
		IFNULL(productivity_factor,1)
		FROM labor_grade WHERE code = ?`, grade).
		Scan(&salario, &cargas, &horas, &productividad); err != nil {
		return 0
	}
	return calcularTasaHoraria(salario.Float64, cargas.Float64, productividad.Float64, horas)
}

// indirectoPorRuta devuelve el indirecto por unidad calculado desde la ruta:
// overhead_rate_hour × (run_time_sec + machine_time_sec) / 3600 / cantidad.
// setup_time_min no entra: no se prorratea entre unidades.
func (s *Server) indirectoPorRuta(db *sql.DB, productID string, cantidad float64) float64 {
	if cantidad <= 0 {
		cantidad = 1
	}
	var total float64
	rows, err := db.Query(`SELECT r.run_time_sec, r.machine_time_sec,
		IFNULL(c.overhead_rate_hour,0)
		FROM process_route r
		LEFT JOIN cost_center c ON c.id = r.work_center_id
		WHERE r.product_id = ? AND r.active = 1`, productID)
	if err != nil {
		return 0
	}
	defer rows.Close()
	for rows.Next() {
		var rt, mt, tasaHora float64
		if err := rows.Scan(&rt, &mt, &tasaHora); err == nil {
			if tasaHora > 0 {
				total += tasaHora * (rt + mt) / 3600 / cantidad
			}
		}
	}
	return total
}
