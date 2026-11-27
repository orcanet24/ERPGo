package web

// cost_engine_test.go — Pruebas del motor de costos (Fase 0).
//
// POR QUÉ ESTAS PRUEBAS. El módulo tenía 13 tablas y CERO pruebas, y por eso
// vivieron tres defectos que nadie vio:
//
//  1. cost_component tenía UNIQUE(period, product_id, component), así que una
//     receta con dos materiales rompía el segundo INSERT.
//  2. cost_center_id se guardaba pero el motor nunca lo leía: el indirecto
//     salía siempre cero.
//  3. productivity_factor se guardaba y no se usaba en ninguna parte.
//
// La aritmética se prueba sin base (milisegundos) y la lectura/escritura con
// SQLite. Separadas así, un fallo dice de inmediato en cuál de las dos mitades
// está.

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// tablasCostosPrueba crea en SQLite las tablas que toca el motor. No se usa el
// esquema canónico porque es DDL de MySQL y aquí lo que se prueba es el SQL del
// motor, no el del servidor.
func tablasCostosPrueba(t *testing.T, db *sql.DB) {
	t.Helper()
	ddl := []string{
		`CREATE TABLE product_cost_config (
			product_id TEXT PRIMARY KEY,
			costing_method TEXT NOT NULL DEFAULT 'PP',
			loss_factor REAL NOT NULL DEFAULT 0,
			is_assembled INTEGER NOT NULL DEFAULT 0,
			cost_center_id INTEGER
		)`,
		`CREATE TABLE cost_center (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'production',
			overhead_rate_hour REAL NOT NULL DEFAULT 0,
			overhead_rate_unit REAL NOT NULL DEFAULT 0,
			active INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE product_information (
			product_id TEXT PRIMARY KEY,
			product_name TEXT NOT NULL DEFAULT '',
			cost REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE product_recipe (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id TEXT NOT NULL,
			component_id TEXT NOT NULL,
			quantity REAL NOT NULL DEFAULT 0,
			unit TEXT NOT NULL DEFAULT '',
			loss_factor REAL NOT NULL DEFAULT 0,
			step_order INTEGER NOT NULL DEFAULT 0,
			labor_grade TEXT NOT NULL DEFAULT '',
			hours REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE product_yield (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			input_product_id TEXT NOT NULL,
			output_product_id TEXT NOT NULL,
			expected_pct REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE cost_component (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			period TEXT NOT NULL,
			product_id TEXT NOT NULL,
			component TEXT NOT NULL,
			line_no INTEGER NOT NULL DEFAULT 1,
			amount REAL NOT NULL DEFAULT 0,
			quantity REAL NOT NULL DEFAULT 0,
			unit_cost REAL NOT NULL DEFAULT 0,
			grade TEXT NOT NULL DEFAULT '',
			hours REAL NOT NULL DEFAULT 0,
			rate_used REAL NOT NULL DEFAULT 0,
			source_ref TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			UNIQUE (period, product_id, component, line_no)
		)`,
		`CREATE TABLE cost_timesheet (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			employee_id INTEGER NOT NULL,
			work_date TEXT NOT NULL,
			hours REAL NOT NULL DEFAULT 0,
			billable INTEGER NOT NULL DEFAULT 1,
			project_id INTEGER,
			product_id TEXT,
			grade TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE labor_grade (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			base_salary_annual REAL NOT NULL DEFAULT 0,
			overhead_pct REAL NOT NULL DEFAULT 0,
			hours_per_year INTEGER NOT NULL DEFAULT 2080,
			productivity_factor REAL NOT NULL DEFAULT 1,
			active INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE import_cost (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			purchase_id INTEGER NOT NULL,
			freight REAL NOT NULL DEFAULT 0,
			customs REAL NOT NULL DEFAULT 0,
			insurance REAL NOT NULL DEFAULT 0,
			other REAL NOT NULL DEFAULT 0,
			received_qty REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE product_purchase_details (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			purchase_id INTEGER NOT NULL,
			product_id TEXT NOT NULL
		)`,
		`CREATE TABLE fixed_asset (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code VARCHAR(40) NOT NULL UNIQUE,
			name VARCHAR(150) NOT NULL,
			type VARCHAR(40) NOT NULL DEFAULT 'equipo',
			is_global INTEGER NOT NULL DEFAULT 0,
			product_id VARCHAR(100),
			cost_center_id INTEGER,
			purchase_price REAL NOT NULL DEFAULT 0,
			residual_value REAL NOT NULL DEFAULT 0,
			purchase_date TEXT NOT NULL,
			useful_life_months INTEGER NOT NULL DEFAULT 0,
			active INTEGER NOT NULL DEFAULT 1,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE fixed_asset_depreciation (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_id INTEGER NOT NULL,
			period VARCHAR(7) NOT NULL,
			month_amount REAL NOT NULL DEFAULT 0,
			accumulated REAL NOT NULL DEFAULT 0,
			book_value REAL NOT NULL DEFAULT 0,
			status VARCHAR(20) NOT NULL DEFAULT 'calculado',
			created_at TEXT NOT NULL DEFAULT '',
			UNIQUE (asset_id, period)
		)`,
		`CREATE TABLE fixed_asset_depreciation_movement (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_id INTEGER NOT NULL,
			period VARCHAR(7) NOT NULL,
			month_amount REAL NOT NULL DEFAULT 0,
			accumulated REAL NOT NULL DEFAULT 0,
			book_value REAL NOT NULL DEFAULT 0,
			status VARCHAR(20) NOT NULL DEFAULT 'calculado',
			created_at TEXT NOT NULL DEFAULT '',
			UNIQUE (asset_id, period)
		)`,
	}
	for _, d := range ddl {
		if _, err := db.Exec(d); err != nil {
			t.Fatalf("crear tabla de costos: %v\n%s", err, d)
		}
	}
}

func casiIgual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func avisoContiene(avisos []string, fragmento string) bool {
	for _, a := range avisos {
		if strings.Contains(a, fragmento) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Aritmética pura
// ---------------------------------------------------------------------------

// TestCalcularCostoConTresMaterialesProduceTresTrazas es el criterio de
// aceptación de la Fase 0: la receta de 3 componentes no pierde renglones y la
// suma del desglose coincide con el total.
func TestCalcularCostoConTresMaterialesProduceTresTrazas(t *testing.T) {
	in := costInputs{
		ProductID: "P1",
		Periodo:   "2026-11",
		Metodo:    costRecipe,
		Cantidad:  1,
		Componentes: []costComponente{
			{ComponentID: "tela", Cantidad: 2, UnitCost: 10, LossFactor: 0.05},
			{ComponentID: "boton", Cantidad: 4, UnitCost: 1, LossFactor: 0},
			{ComponentID: "hilo", Cantidad: 1, UnitCost: 2, LossFactor: 0.10},
		},
	}
	b := calcularCosto(in)

	materiales := 0
	for _, tr := range b.Traces {
		if tr.Component == compMaterial {
			materiales++
		}
	}
	if materiales != 3 {
		t.Errorf("trazas de material: %d; esperadas 3 (una por componente)", materiales)
	}
	// 2×10×1.05 + 4×1 + 1×2×1.10 = 21 + 4 + 2.2 = 27.2
	if !casiIgual(b.Material, 27.2) {
		t.Errorf("material: %v; esperado 27.2", b.Material)
	}
	if !casiIgual(b.Final, b.Material+b.Labor+b.Depreciation+b.Overhead+b.Customs) {
		t.Errorf("el final no es la suma de componentes: %+v", b)
	}
}

// TestMermaGlobalSeAplicaSoloAlMaterial fija la regla del bug 0.1: la merma no
// puede tocar la mano de obra ni la depreciación, que no se pierden en el corte.
func TestMermaGlobalSeAplicaSoloAlMaterial(t *testing.T) {
	b := calcularCosto(costInputs{
		ProductID:     "P1",
		Periodo:       "2026-11",
		Metodo:        costPP,
		Cantidad:      10,
		Salida:        100,
		LossFactor:    0.10,
		DegrPerUnidad: 2,
		OverheadPerUn: 3,
	})
	if !casiIgual(b.Material, 11) {
		t.Errorf("material con merma 10%%: %v; esperado 11", b.Material)
	}
	if !casiIgual(b.Depreciation, 2) || !casiIgual(b.Overhead, 3) || b.Labor != 0 {
		t.Errorf("la merma tocó componentes que no corresponden: %+v", b)
	}
	if !casiIgual(b.Final, 16) {
		t.Errorf("final: %v; esperado 16 (11 material + 2 depreciación + 3 indirecto)", b.Final)
	}
}

// TestCalcularCostoSumaDepreciacionEIndirecto protege el segundo defecto: los
// dos campos existían y se sumaban, pero nadie los llenaba.
func TestCalcularCostoSumaDepreciacionEIndirecto(t *testing.T) {
	b := calcularCosto(costInputs{
		ProductID:     "P1",
		Periodo:       "2026-11",
		Metodo:        costPP,
		Cantidad:      10,
		Salida:        100,
		DegrPerUnidad: 2.5,
		OverheadPerUn: 1.5,
	})
	if !casiIgual(b.Material, 10) || !casiIgual(b.Depreciation, 2.5) || !casiIgual(b.Overhead, 1.5) {
		t.Errorf("desglose inesperado: %+v", b)
	}
	if !casiIgual(b.Final, 14) {
		t.Errorf("final: %v; esperado 14", b.Final)
	}

	vistos := map[string]float64{}
	for _, tr := range b.Traces {
		vistos[tr.Component] = tr.Importe
	}
	if !casiIgual(vistos[compDepreciation], 2.5) {
		t.Errorf("traza de depreciación: %v; esperada 2.5", vistos[compDepreciation])
	}
	if !casiIgual(vistos[compOverhead], 1.5) {
		t.Errorf("traza de indirecto: %v; esperada 1.5", vistos[compOverhead])
	}
}

// TestCalcularTasaHorariaAplicaProductividadYNoDivideEntreCero cubre el tercer
// defecto: el formulario guardaba productividad y la tarifa la ignoraba.
func TestCalcularTasaHorariaAplicaProductividadYNoDivideEntreCero(t *testing.T) {
	casos := []struct {
		nombre   string
		salario  float64
		overhead float64
		prod     float64
		horas    int
		esperado float64
	}{
		{"productividad sin configurar vale 1", 20800, 0, 0, 2080, 10},
		{"productividad mayor que 1 encarece la hora", 20800, 0, 1.25, 2080, 12.5},
		{"las cargas se suman antes de dividir", 10000, 34, 1, 1000, 13.4},
		{"sin horas no hay tasa", 20800, 0, 1, 0, 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := calcularTasaHoraria(c.salario, c.overhead, c.prod, c.horas)
			if !casiIgual(got, c.esperado) {
				t.Errorf("calcularTasaHoraria(%v, %v, %v, %d) = %v; esperado %v",
					c.salario, c.overhead, c.prod, c.horas, got, c.esperado)
			}
		})
	}
}

// TestLaborPorHorasConservaGrado verifica que la traza lleva el rol. Sin ese
// dato, guardarComponentesCosto escribía grade="" y el renglón de labor no se
// podía reconstruir.
func TestLaborPorHorasConservaGrado(t *testing.T) {
	total, traces, avisos := laborPorHoras(costInputs{
		Cantidad: 2,
		Horas: []costHora{
			{Grade: "maestro", Hours: 4, HourlyRate: 10, Productivity: 1},
		},
	})
	if len(avisos) != 0 {
		t.Fatalf("avisos inesperados: %v", avisos)
	}
	if !casiIgual(total, 20) {
		t.Errorf("labor por unidad: %v; esperado 20 (4 h × 10 / 2 unidades)", total)
	}
	if len(traces) != 1 {
		t.Fatalf("trazas de labor: %d; esperada 1", len(traces))
	}
	if traces[0].Grade != "maestro" {
		t.Errorf("grade de la traza: %q; esperado maestro", traces[0].Grade)
	}
	if !casiIgual(traces[0].Cantidad, 4) {
		t.Errorf("horas de la traza: %v; esperadas 4", traces[0].Cantidad)
	}
}

// TestCostTraceSeSerializaConLosNombresDeLaPlantilla protege el contrato con
// cost_product.html. La traza no tenía etiquetas JSON, así que el template leía
// t.component / t.regla / t.source y recibía undefined: el desglose se pintaba
// vacío aunque el cálculo fuera correcto.
func TestCostTraceSeSerializaConLosNombresDeLaPlantilla(t *testing.T) {
	datos, err := json.Marshal(CostTrace{
		Component: compMaterial,
		Regla:     "receta",
		Tasa:      5,
		Cantidad:  2,
		Importe:   10,
		Fuente:    "product_recipe",
	})
	if err != nil {
		t.Fatalf("serializar traza: %v", err)
	}
	var campos map[string]interface{}
	if err := json.Unmarshal(datos, &campos); err != nil {
		t.Fatalf("deserializar traza: %v", err)
	}
	for _, campo := range []string{"component", "regla", "source", "importe"} {
		if _, ok := campos[campo]; !ok {
			t.Errorf("la traza no expone %q: %s", campo, datos)
		}
	}
}

// ---------------------------------------------------------------------------
// Lectura y escritura con base
// ---------------------------------------------------------------------------

// TestGuardarComponentesCostoNoPierdeRenglones reproduce el defecto 1 completo:
// dos materiales, una labor y un indirecto deben quedar como cuatro renglones,
// con line_no distinto para cada material.
func TestGuardarComponentesCostoNoPierdeRenglones(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	b := CostBreakdown{
		ProductID: "P1",
		Periodo:   "2026-11",
		Metodo:    costRecipe,
		Traces: []CostTrace{
			{Component: compMaterial, Regla: "tela", Tasa: 5, Cantidad: 2, Importe: 10, Fuente: "product_recipe"},
			{Component: compMaterial, Regla: "boton", Tasa: 1, Cantidad: 4, Importe: 4, Fuente: "product_recipe"},
			{Component: compLabor, Regla: "maestro", Tasa: 20, Cantidad: 3, Importe: 60, Fuente: "cost_timesheet", Grade: "maestro"},
			{Component: compOverhead, Regla: "indirecto", Tasa: 1, Cantidad: 1, Importe: 1, Fuente: "cost_center"},
			{Component: compFinal, Regla: "suma", Importe: 75},
		},
	}
	if err := s.guardarComponentesCosto(db, b, costInputs{}); err != nil {
		t.Fatalf("guardarComponentesCosto: %v", err)
	}

	type fila struct {
		component string
		linea     int
		grade     string
		horas     float64
		tasa      float64
	}
	var filas []fila
	rows, err := db.Query(`SELECT component, line_no, grade, hours, rate_used
		FROM cost_component WHERE period = ? AND product_id = ?
		ORDER BY component, line_no`, "2026-11", "P1")
	if err != nil {
		t.Fatalf("leer cost_component: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f fila
		if err := rows.Scan(&f.component, &f.linea, &f.grade, &f.horas, &f.tasa); err != nil {
			t.Fatalf("escanear cost_component: %v", err)
		}
		filas = append(filas, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("recorrer cost_component: %v", err)
	}

	if len(filas) != 4 {
		t.Fatalf("renglones guardados: %d; esperados 4 (el total no se guarda como componente)", len(filas))
	}
	// Orden: labor, material (línea 1), material (línea 2), overhead.
	if filas[0].component != compLabor || filas[0].grade != "maestro" || !casiIgual(filas[0].horas, 3) || !casiIgual(filas[0].tasa, 20) {
		t.Errorf("labor mal guardada: %+v", filas[0])
	}
	if filas[1].component != compMaterial || filas[1].linea != 1 || filas[1].grade != "" || filas[1].horas != 0 || !casiIgual(filas[1].tasa, 5) {
		t.Errorf("primer material mal guardado: %+v", filas[1])
	}
	if filas[2].component != compMaterial || filas[2].linea != 2 || !casiIgual(filas[2].tasa, 1) {
		t.Errorf("segundo material mal guardado: %+v", filas[2])
	}
	if filas[3].component != compOverhead {
		t.Errorf("indirecto mal guardado: %+v", filas[3])
	}

	// Recalcular el mismo período debe reemplazar, no duplicar.
	if err := s.guardarComponentesCosto(db, b, costInputs{}); err != nil {
		t.Fatalf("segundo guardado: %v", err)
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM cost_component WHERE period = ? AND product_id = ?`,
		"2026-11", "P1").Scan(&total); err != nil {
		t.Fatalf("contar tras recalcular: %v", err)
	}
	if total != 4 {
		t.Errorf("renglones tras recalcular: %d; esperados 4 (no se duplican)", total)
	}
}

// TestCargarCostInputsLeeIndirectoDelCentro cierra el defecto 2: el centro de
// costo configurado en el producto se lee y llega al desglose.
func TestCargarCostInputsLeeIndirectoDelCentro(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO cost_center
		(id, code, name, overhead_rate_unit, overhead_rate_hour, active)
		VALUES (1, 'PROD', 'Producción', 2.5, 0, 1)`); err != nil {
		t.Fatalf("insertar centro: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method, cost_center_id) VALUES ('P1', 'PP', 1)`); err != nil {
		t.Fatalf("insertar configuración: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('P1', 100)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}

	in := s.cargarCostInputs(db, "P1", "2026-11", 10)
	if !casiIgual(in.OverheadPerUn, 2.5) {
		t.Errorf("indirecto por unidad: %v; esperado 2.5", in.OverheadPerUn)
	}
	b := calcularCosto(in)
	if !casiIgual(b.Overhead, 2.5) {
		t.Errorf("indirecto en el desglose: %v; esperado 2.5", b.Overhead)
	}
	// product_information.cost = 100 es el costo UNITARIO: el material sale 100
	// y el indirecto se suma encima.
	if !casiIgual(b.Final, 102.5) {
		t.Errorf("final: %v; esperado 102.5 (100 material + 2.5 indirecto)", b.Final)
	}
}

// TestCargarCostInputsAvisaIndirectoNoAplicable: un centro inactivo o una
// tarifa solo por hora no pueden convertirse en un cero silencioso.
func TestCargarCostInputsAvisaIndirectoNoAplicable(t *testing.T) {
	casos := []struct {
		nombre     string
		activo     int
		tasaUnidad float64
		tasaHora   float64
		fragmento  string
	}{
		{"centro inactivo", 0, 2.5, 0, "inactivo"},
		{"solo tarifa por hora", 1, 0, 1.5, "tarifa por hora"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, db := basePrueba(t)
			tablasCostosPrueba(t, db)
			if _, err := db.Exec(`INSERT INTO cost_center
				(id, code, name, overhead_rate_unit, overhead_rate_hour, active)
				VALUES (1, 'PROD', 'Producción', ?, ?, ?)`,
				c.tasaUnidad, c.tasaHora, c.activo); err != nil {
				t.Fatalf("insertar centro: %v", err)
			}
			if _, err := db.Exec(`INSERT INTO product_cost_config
				(product_id, costing_method, cost_center_id) VALUES ('P1', 'PP', 1)`); err != nil {
				t.Fatalf("insertar configuración: %v", err)
			}
			if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('P1', 100)`); err != nil {
				t.Fatalf("insertar producto: %v", err)
			}

			in := s.cargarCostInputs(db, "P1", "2026-11", 10)
			if in.OverheadPerUn != 0 {
				t.Errorf("indirecto: %v; esperado 0", in.OverheadPerUn)
			}
			if !avisoContiene(in.Faltantes, c.fragmento) {
				t.Errorf("falta el aviso %q en %v", c.fragmento, in.Faltantes)
			}
		})
	}
}

// TestCargarCostInputsAvisaEnsambladoSinDepreciacion: mientras no existan
// activos fijos, el cero de depreciación se dice en voz alta.
func TestCargarCostInputsAvisaEnsambladoSinDepreciacion(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method, is_assembled) VALUES ('P1', 'RECIPE', 1)`); err != nil {
		t.Fatalf("insertar configuración: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('P1', 100)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}

	in := s.cargarCostInputs(db, "P1", "2026-11", 1)
	if !in.IsAssembled {
		t.Fatal("is_assembled no se leyó de la configuración")
	}
	if !avisoContiene(in.Faltantes, "depreciación") {
		t.Errorf("falta el aviso de depreciación en %v", in.Faltantes)
	}
}

// TestTasaDeGradoAplicaProductividad comprueba el camino real del formulario:
// el grado se lee de labor_grade y la productividad entra en la tarifa.
func TestTasaDeGradoAplicaProductividad(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year, productivity_factor, active)
		VALUES ('maestro', 'Maestro', 20800, 0, 2080, 1.25, 1)`); err != nil {
		t.Fatalf("insertar grado: %v", err)
	}
	if got := s.tasaDeGrado(db, "maestro"); !casiIgual(got, 12.5) {
		t.Errorf("tasa del grado: %v; esperada 12.5", got)
	}
}

// TestCargarCostInputsHorasUsaProductividadDelGrado integra la hoja de horas con
// la tarifa: 8 horas de un grado de 12.5/hora son 100 de mano de obra.
func TestCargarCostInputsHorasUsaProductividadDelGrado(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method) VALUES ('P1', 'HOURS')`); err != nil {
		t.Fatalf("insertar configuración: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year, productivity_factor, active)
		VALUES ('maestro', 'Maestro', 20800, 0, 2080, 1.25, 1)`); err != nil {
		t.Fatalf("insertar grado: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cost_timesheet
		(employee_id, work_date, hours, billable, product_id, grade)
		VALUES (1, '2026-11-05', 8, 1, 'P1', 'maestro')`); err != nil {
		t.Fatalf("insertar horas: %v", err)
	}

	in := s.cargarCostInputs(db, "P1", "2026-11", 1)
	if len(in.Horas) != 1 {
		t.Fatalf("horas leídas: %d; esperada 1", len(in.Horas))
	}
	if !casiIgual(in.Horas[0].HourlyRate, 12.5) {
		t.Errorf("tarifa aplicada: %v; esperada 12.5", in.Horas[0].HourlyRate)
	}
	b := calcularCosto(in)
	if !casiIgual(b.Labor, 100) || !casiIgual(b.Final, 100) {
		t.Errorf("costo de mano de obra: %v (final %v); esperado 100", b.Labor, b.Final)
	}
}

// ---------------------------------------------------------------------------
// Fase 1: composición, mano de obra en receta y pantallas de maestros
// ---------------------------------------------------------------------------

// TestCalcularCostoRecetaComponeMaterialYManufactura es el criterio de
// aceptación de la Fase 1: una receta puede traer material Y mano de obra, y el
// total es la suma de todo, no la parte que el método dejaba ver.
func TestCalcularCostoRecetaComponeMaterialYManufactura(t *testing.T) {
	b := calcularCosto(costInputs{
		ProductID: "PAN",
		Periodo:   "2026-11",
		Metodo:    costRecipe,
		Cantidad:  1,
		Componentes: []costComponente{
			{ComponentID: "harina", Cantidad: 1, UnitCost: 2},
			{ComponentID: "levadura", Cantidad: 0.1, UnitCost: 5},
		},
		LaborReceta: []costHora{
			{Grade: "panadero", Hours: 0.5, HourlyRate: 20, Productivity: 1},
		},
		OverheadPerUn: 1.5,
		DegrPerUnidad: 0.5,
	})

	if !casiIgual(b.Material, 2.5) {
		t.Errorf("material: %v; esperado 2.5", b.Material)
	}
	if !casiIgual(b.Labor, 10) {
		t.Errorf("mano de obra: %v; esperado 10 (0.5 h × 20)", b.Labor)
	}
	if !casiIgual(b.Overhead, 1.5) || !casiIgual(b.Depreciation, 0.5) {
		t.Errorf("indirecto/depreciación: %v/%v; esperados 1.5/0.5", b.Overhead, b.Depreciation)
	}
	if !casiIgual(b.Final, 14.5) {
		t.Errorf("final: %v; esperado 14.5 (2.5 + 10 + 1.5 + 0.5)", b.Final)
	}

	materiales, labores := 0, 0
	for _, tr := range b.Traces {
		switch tr.Component {
		case compMaterial:
			materiales++
		case compLabor:
			labores++
		}
	}
	if materiales != 2 || labores != 1 {
		t.Errorf("trazas material/labor: %d/%d; esperadas 2/1", materiales, labores)
	}
}

// TestCalcularCostoRendimientoSumaHorasDeDespiece fija la composición en el
// método de carnicería: el material se reparte por rendimiento y la mano de
// obra del despiece se suma encima. Antes, YIELD devolvía antes de leer horas.
func TestCalcularCostoRendimientoSumaHorasDeDespiece(t *testing.T) {
	b := calcularCosto(costInputs{
		ProductID: "LOMO",
		Periodo:   "2026-11",
		Metodo:    costYield,
		Cantidad:  100,
		Salida:    1000,
		YieldBase: costYieldRow{ExpectedPct: 80},
		Horas: []costHora{
			{Grade: "carnicero", Hours: 2, HourlyRate: 15, Productivity: 1},
		},
	})

	// 1000 de canal / (100 kg × 80%) = 12.5 por kg de venta.
	if !casiIgual(b.Material, 12.5) {
		t.Errorf("material por rendimiento: %v; esperado 12.5", b.Material)
	}
	// 2 h × 15 / 100 unidades = 0.3 por unidad.
	if !casiIgual(b.Labor, 0.3) {
		t.Errorf("mano de obra por unidad: %v; esperado 0.3", b.Labor)
	}
	if !casiIgual(b.Final, 12.8) {
		t.Errorf("final: %v; esperado 12.8", b.Final)
	}
}

// TestCargarCostInputsRecetaConManoDeObraNoUsaHojaDeHoras protege la regla
// contra el doble conteo: si la receta ya trae la hora estándar, la hoja de
// horas no se suma y el cálculo lo dice.
func TestCargarCostInputsRecetaConManoDeObraNoUsaHojaDeHoras(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method) VALUES ('PAN', 'RECIPE')`); err != nil {
		t.Fatalf("insertar configuración: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('PAN', 0)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('harina', 2)`); err != nil {
		t.Fatalf("insertar insumo: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year, productivity_factor, active)
		VALUES ('panadero', 'Panadero', 20800, 0, 2080, 1, 1)`); err != nil {
		t.Fatalf("insertar grado: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order, labor_grade, hours)
		VALUES ('PAN', 'harina', 1, 'kg', 0, 1, '', 0)`); err != nil {
		t.Fatalf("insertar material: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order, labor_grade, hours)
		VALUES ('PAN', 'MO:panadero', 0.5, 'h', 0, 2, 'panadero', 0.5)`); err != nil {
		t.Fatalf("insertar mano de obra: %v", err)
	}
	// Horas reales del mismo producto: no deben sumarse a la receta.
	if _, err := db.Exec(`INSERT INTO cost_timesheet
		(employee_id, work_date, hours, billable, product_id, grade)
		VALUES (1, '2026-11-05', 3, 1, 'PAN', 'panadero')`); err != nil {
		t.Fatalf("insertar horas: %v", err)
	}

	in := s.cargarCostInputs(db, "PAN", "2026-11", 1)
	if len(in.LaborReceta) != 1 {
		t.Fatalf("mano de obra de receta: %d; esperada 1", len(in.LaborReceta))
	}
	if len(in.Horas) != 0 {
		t.Errorf("horas de la hoja: %d; esperadas 0 (la receta manda)", len(in.Horas))
	}
	if !avisoContiene(in.Avisos, "dos veces") {
		t.Errorf("falta el aviso de doble conteo en %v", in.Avisos)
	}

	b := calcularCosto(in)
	// Material: 1 kg × 2. Mano de obra: 0.5 h × (20800/2080 = 10) = 5.
	if !casiIgual(b.Material, 2) || !casiIgual(b.Labor, 5) {
		t.Errorf("desglose: material %v, labor %v; esperados 2 y 5", b.Material, b.Labor)
	}
	if !casiIgual(b.Final, 7) {
		t.Errorf("final: %v; esperado 7", b.Final)
	}
}

// TestCargarCostInputsUsaHojaCuandoRecetaNoTieneLabor cierra el otro lado de la
// regla: una receta sin mano de obra propia sí toma las horas registradas.
func TestCargarCostInputsUsaHojaCuandoRecetaNoTieneLabor(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method) VALUES ('PAN', 'RECIPE')`); err != nil {
		t.Fatalf("insertar configuración: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('PAN', 0)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('harina', 2)`); err != nil {
		t.Fatalf("insertar insumo: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year, productivity_factor, active)
		VALUES ('panadero', 'Panadero', 20800, 0, 2080, 1, 1)`); err != nil {
		t.Fatalf("insertar grado: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order, labor_grade, hours)
		VALUES ('PAN', 'harina', 1, 'kg', 0, 1, '', 0)`); err != nil {
		t.Fatalf("insertar material: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cost_timesheet
		(employee_id, work_date, hours, billable, product_id, grade)
		VALUES (1, '2026-11-05', 4, 1, 'PAN', 'panadero')`); err != nil {
		t.Fatalf("insertar horas: %v", err)
	}

	in := s.cargarCostInputs(db, "PAN", "2026-11", 1)
	if len(in.LaborReceta) != 0 {
		t.Fatalf("mano de obra de receta: %d; esperada 0", len(in.LaborReceta))
	}
	if len(in.Horas) != 1 {
		t.Fatalf("horas de la hoja: %d; esperada 1", len(in.Horas))
	}
	b := calcularCosto(in)
	if !casiIgual(b.Labor, 40) {
		t.Errorf("mano de obra: %v; esperada 40 (4 h × 10)", b.Labor)
	}
}

// TestCargarCostInputsRendimientoNoPierdeHoras reproduce el return que cortaba
// el cálculo de YIELD antes de leer la hoja de horas.
func TestCargarCostInputsRendimientoNoPierdeHoras(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method) VALUES ('LOMO', 'YIELD')`); err != nil {
		t.Fatalf("insertar configuración: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('LOMO', 0)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('CANAL', 10)`); err != nil {
		t.Fatalf("insertar entrada: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_yield
		(input_product_id, output_product_id, expected_pct) VALUES ('CANAL', 'LOMO', 80)`); err != nil {
		t.Fatalf("insertar rendimiento: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year, productivity_factor, active)
		VALUES ('carnicero', 'Carnicero', 31200, 0, 2080, 1, 1)`); err != nil {
		t.Fatalf("insertar grado: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cost_timesheet
		(employee_id, work_date, hours, billable, product_id, grade)
		VALUES (1, '2026-11-05', 2, 1, 'LOMO', 'carnicero')`); err != nil {
		t.Fatalf("insertar horas: %v", err)
	}

	in := s.cargarCostInputs(db, "LOMO", "2026-11", 100)
	if len(in.Horas) != 1 {
		t.Fatalf("horas leídas: %d; esperada 1 (YIELD no debe retornar antes)", len(in.Horas))
	}
	b := calcularCosto(in)
	if !casiIgual(b.Labor, 0.3) {
		t.Errorf("mano de obra: %v; esperada 0.3 (2 h × 15 / 100)", b.Labor)
	}
	if !casiIgual(b.Material, 12.5) {
		t.Errorf("material: %v; esperado 12.5", b.Material)
	}
}

// TestGuardarRecetaManoDeObraSeLeeComoHoras prueba el camino lector: se
// insertan directamente un renglón de material y uno de mano de obra, y la
// lectura los devuelve valorados cada uno a su modo (material con merma,
// labor con tarifa del grado y horas en cantidad). El guardado por formulario
// pasa por guardarReceta, que usa ON DUPLICATE KEY (sintaxis MySQL) y no se
// puede ejecutar contra SQLite en los tests; la validación de ese camino vive
// en cost_config_service.go y se ejerce en integración real.
func TestGuardarRecetaManoDeObraSeLeeComoHoras(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO labor_grade
		(code, name, base_salary_annual, overhead_pct, hours_per_year, productivity_factor, active)
		VALUES ('panadero', 'Panadero', 20800, 0, 2080, 1, 1)`); err != nil {
		t.Fatalf("insertar grado: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information
		(product_id, product_name, cost) VALUES ('harina', 'Harina', 2)`); err != nil {
		t.Fatalf("insertar insumo: %v", err)
	}
	// Inserción directa porque ON DUPLICATE KEY no existe en SQLite.
	if _, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order, labor_grade, hours)
		VALUES ('PAN', 'MO:panadero', 0.5, 'h', 0, 1, 'panadero', 0.5)`); err != nil {
		t.Fatalf("insertar mano de obra: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order, labor_grade, hours)
		VALUES ('PAN', 'harina', 2, 'kg', 0, 2, '', 0)`); err != nil {
		t.Fatalf("insertar material: %v", err)
	}

	receta, err := s.listarReceta(db, "PAN")
	if err != nil {
		t.Fatalf("listar receta: %v", err)
	}
	if len(receta) != 2 {
		t.Fatalf("renglones: %d; esperado 2", len(receta))
	}
	// El material conserva su identidad y su merma.
	if receta[0].ComponentID != "MO:panadero" || receta[0].LaborGrade != "panadero" {
		t.Errorf("renglón 0 no es labor: %+v", receta[0])
	}
	if !casiIgual(receta[0].Hours, 0.5) || !casiIgual(receta[0].Cantidad, 0.5) || receta[0].Unit != "h" {
		t.Errorf("horas/unidad del labor: %+v", receta[0])
	}
	if !casiIgual(receta[0].UnitCost, 10) || !casiIgual(receta[0].Total, 5) {
		t.Errorf("valoración del labor: tarifa %v, total %v; esperados 10 y 5", receta[0].UnitCost, receta[0].Total)
	}
	if receta[1].ComponentID != "harina" || receta[1].LaborGrade != "" {
		t.Errorf("renglón 1 no es material: %+v", receta[1])
	}
	if !casiIgual(receta[1].Total, 4) {
		t.Errorf("total del material: %v; esperado 4 (2 kg × 2)", receta[1].Total)
	}
}

// bloqueDeTabla extrae el CREATE TABLE de una tabla concreta. Se usa para
// comprobar la definición y no una palabra suelta en cualquier parte del
// archivo (line_no aparece en otras tablas).
func bloqueDeTabla(texto, tabla string) string {
	marca := "`" + tabla + "` ("
	i := strings.Index(texto, marca)
	if i < 0 {
		return ""
	}
	resto := texto[i:]
	// Se corta en el fin de sentencia, no en cualquier punto y coma: los
	// comentarios del esquema pueden contener uno y cortarían el bloque antes
	// de la definición que se quiere mirar.
	fin := strings.Index(resto, ";\n")
	if fin < 0 {
		fin = strings.Index(resto, ";\r\n")
	}
	if fin < 0 {
		return resto
	}
	return resto[:fin]
}

// TestEsquemaCostComponentAdmiteVariosMateriales es el guardián de la migración:
// si alguien vuelve a poner la clave única antigua, esta prueba lo delata antes
// de que un recálculo de receta falle en producción.
func TestEsquemaCostComponentAdmiteVariosMateriales(t *testing.T) {
	archivos := []string{
		"../../migrations/schema_z_costos.sql",
		"../../migrations/schema_reconstruido_10_system.sql",
		"../../schema/esquema_canonico_mysql.sql",
	}
	for _, ruta := range archivos {
		contenido, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatalf("leer %s: %v", ruta, err)
		}
		bloque := bloqueDeTabla(string(contenido), "cost_component")
		if bloque == "" {
			t.Errorf("%s no define cost_component", ruta)
			continue
		}
		if !strings.Contains(bloque, "line_no") {
			t.Errorf("%s: cost_component no declara line_no", ruta)
		}
		if !strings.Contains(bloque, "uq_cost_component_linea") {
			t.Errorf("%s: cost_component no declara uq_cost_component_linea", ruta)
		}
		if strings.Contains(bloque, "UNIQUE KEY `uq_cost_component` (") {
			t.Errorf("%s: cost_component conserva la clave única antigua", ruta)
		}
	}

	// La migración del módulo debe además corregir las bases que ya existen.
	migracion, err := os.ReadFile("../../migrations/schema_z_costos.sql")
	if err != nil {
		t.Fatalf("leer schema_z_costos.sql: %v", err)
	}
	if !strings.Contains(string(migracion), "DROP INDEX `uq_cost_component`") {
		t.Error("schema_z_costos.sql no rehace la clave única en bases existentes")
	}
}

// TestEsquemaProductRecipeTieneManoDeObra es el guardián de la Fase 1: si
// alguien vuelve a dejar la receta sin labor_grade/hours, el motor no puede
// distinguir un renglón de mano de obra y la composición se rompe en silencio.
func TestEsquemaProductRecipeTieneManoDeObra(t *testing.T) {
	archivos := []string{
		"../../migrations/schema_z_costos.sql",
		"../../migrations/schema_reconstruido_02_inventory.sql",
		"../../schema/esquema_canonico_mysql.sql",
	}
	for _, ruta := range archivos {
		contenido, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatalf("leer %s: %v", ruta, err)
		}
		bloque := bloqueDeTabla(string(contenido), "product_recipe")
		if bloque == "" {
			t.Errorf("%s no define product_recipe", ruta)
			continue
		}
		if !strings.Contains(bloque, "labor_grade") {
			t.Errorf("%s: product_recipe no declara labor_grade", ruta)
		}
		if !strings.Contains(bloque, "`hours`") {
			t.Errorf("%s: product_recipe no declara hours", ruta)
		}
	}

	// La migración del módulo debe además corregir las bases que ya existen.
	migracion, err := os.ReadFile("../../migrations/schema_z_costos.sql")
	if err != nil {
		t.Fatalf("leer schema_z_costos.sql: %v", err)
	}
	for _, columna := range []string{"labor_grade", "hours"} {
		if !strings.Contains(string(migracion), "ADD COLUMN `"+columna+"`") {
			t.Errorf("schema_z_costos.sql no añade %s a las bases existentes", columna)
		}
	}
}

// ---------------------------------------------------------------------------
// Fase 2: activos fijos y depreciación
// ---------------------------------------------------------------------------

// TestCalcularCuota es pura: (precio - residual) / vida en meses.
func TestCalcularCuota(t *testing.T) {
	casos := []struct {
		nombre   string
		precio   float64
		residual float64
		vida     int
		esperado float64
	}{
		{"horno 12000, residual 0, vida 60", 12000, 0, 60, 200},
		{"con valor residual", 10000, 1000, 60, 150},
		{"vida cero no divide entre cero", 10000, 0, 0, 0},
		{"precio menor que residual: cuota cero", 1000, 2000, 60, 0},
		{"precio igual a residual: cuota cero", 5000, 5000, 60, 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := calcularCuota(c.precio, c.residual, c.vida)
			if !casiIgual(got, c.esperado) {
				t.Errorf("calcularCuota(%v, %v, %d) = %v; esperado %v", c.precio, c.residual, c.vida, got, c.esperado)
			}
		})
	}
}

// TestMesesEntre cuenta meses entre fecha de compra y período.
func TestMesesEntre(t *testing.T) {
	casos := []struct {
		nombre   string
		compra   string
		periodo  string
		esperado int
	}{
		{"misma compra y periodo", "2026-01-15", "2026-01", 0},
		{"compra enero, periodo febrero", "2026-01-15", "2026-02", 1},
		{"compra enero, periodo marzo del año siguiente", "2026-01-15", "2027-03", 14},
		{"periodo antes que compra", "2026-06-01", "2026-03", -3},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := mesesEntre(c.compra, c.periodo)
			if got != c.esperado {
				t.Errorf("mesesEntre(%q, %q) = %d; esperado %d", c.compra, c.periodo, got, c.esperado)
			}
		})
	}
}

// TestDepreciacionDelPeriodo calcula cuota, acumulado y valor de libro.
func TestDepreciacionDelPeriodo(t *testing.T) {
	_, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	// Horno: 12000, residual 0, vida 60, compra enero 2026. Cuota = 200/mes.
	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, type, is_global, product_id, cost_center_id,
		 purchase_price, residual_value, purchase_date, useful_life_months,
		 active, created_by, created_at)
		VALUES ('HORNO-01', 'Horno', 'equipo', 0, 'PAN', 1,
		 12000, 0, '2026-01-01', 60, 1, 'admin', '2026-01-01 00:00:00')`); err != nil {
		t.Fatalf("insertar activo: %v", err)
	}

	// Enero 2026: primera cuota.
	ma, ac, bv, calc, err := depreciacionDelPeriodo(db, 1, "2026-01", now())
	if err != nil {
		t.Fatalf("calcular depreciación: %v", err)
	}
	if !calc {
		t.Fatal("esperaba cálculo en enero")
	}
	if !casiIgual(ma, 200) {
		t.Errorf("cuota enero: %v; esperado 200", ma)
	}
	if !casiIgual(ac, 200) {
		t.Errorf("acumulado enero: %v; esperado 200", ac)
	}
	if !casiIgual(bv, 11800) {
		t.Errorf("libro enero: %v; esperado 11800", bv)
	}

	// Febrero 2026: segunda cuota, acumulado 400.
	ma2, ac2, bv2, calc2, err := depreciacionDelPeriodo(db, 1, "2026-02", now())
	if err != nil {
		t.Fatalf("calcular depreciación febrero: %v", err)
	}
	if !calc2 {
		t.Fatal("esperaba cálculo en febrero")
	}
	if !casiIgual(ma2, 200) || !casiIgual(ac2, 400) || !casiIgual(bv2, 11600) {
		t.Errorf("febrero: cuota %v, acumulado %v, libro %v; esperados 200, 400, 11600", ma2, ac2, bv2)
	}

	// Activo inactivo no deprecia.
	if _, err := db.Exec(`UPDATE fixed_asset SET active = 0 WHERE code = 'HORNO-01'`); err != nil {
		t.Fatalf("desactivar activo: %v", err)
	}
	_, _, _, calcInactivo, err := depreciacionDelPeriodo(db, 1, "2026-03", now())
	if err != nil {
		t.Fatalf("calcular depreciación marzo: %v", err)
	}
	if calcInactivo {
		t.Error("activo inactivo no debería depreciar, pero sí lo hizo")
	}

	// Vida vencida: después de 60 meses no hay más cuota.
	if _, err := db.Exec(`UPDATE fixed_asset SET useful_life_months = 2 WHERE code = 'HORNO-01'`); err != nil {
		t.Fatalf("actualizar vida: %v", err)
	}
	_, _, _, calcVencido, _ := depreciacionDelPeriodo(db, 1, "2026-03", now())
	if calcVencido {
		t.Error("activo con vida vencida no debería depreciar")
	}
}

// TestDepreciacionDelPeriodoRespetaResidual corta la cuota si el valor de libro
// bajaría por debajo del residual.
func TestDepreciacionDelPeriodoRespetaResidual(t *testing.T) {
	_, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	// 1000, residual 800, vida 10. Cuota = 20/mes. Después de 9 meses, acumulado 180, libro 820.
	// La décima cuota sería 20, pero libro 820 - 20 = 800 = residual, OK.
	// Si vida fuera 11, la undécima intentaría 800 - 20 = 780 < residual, se corta.
	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('A1', 'Activo', 1000, 800, '2026-01-01', 10, 1)`); err != nil {
		t.Fatalf("insertar activo: %v", err)
	}
	_, ac, bv, calc, _ := depreciacionDelPeriodo(db, 1, "2026-10", now())
	if !calc {
		t.Fatal("esperaba cálculo en el décimo mes")
	}
	if !casiIgual(ac, 1000-800) {
		t.Errorf("acumulado: %v; esperado 200", ac)
	}
	if !casiIgual(bv, 800) {
		t.Errorf("libro: %v; esperado 800 (residual)", bv)
	}
}

// TestDepreciacionProductoPeriodoSumaActivosDirectos y globales.
func TestDepreciacionProductoPeriodo(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	// Producto PAN con centro 1.
	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method, cost_center_id, is_assembled)
		VALUES ('PAN', 'RECIPE', 1, 1)`); err != nil {
		t.Fatalf("insertar config: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method, cost_center_id, is_assembled)
		VALUES ('PAN2', 'RECIPE', 1, 1)`); err != nil {
		t.Fatalf("insertar config PAN2: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('PAN', 10)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('PAN2', 10)`); err != nil {
		t.Fatalf("insertar producto PAN2: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cost_center (id, code, name, overhead_rate_unit, overhead_rate_hour, active)
		VALUES (1, 'PROD', 'Producción', 0, 0, 1)`); err != nil {
		t.Fatalf("insertar centro: %v", err)
	}

	// Activo directo a PAN: cuota 100/mes.
	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, product_id, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('DIRECTO', 'Directo PAN', 'PAN', 6000, 0, '2026-01-01', 60, 1)`); err != nil {
		t.Fatalf("insertar activo directo: %v", err)
	}
	// Activo global del centro 1: cuota 300/mes, se reparte entre PAN y PAN2 (2 productos).
	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, is_global, cost_center_id, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('GLOBAL', 'Global centro', 1, 1, 18000, 0, '2026-01-01', 60, 1)`); err != nil {
		t.Fatalf("insertar activo global: %v", err)
	}

	depr := s.depreciacionProductoPeriodo(db, "PAN", 1, "2026-01", now())
	// Directo: 100. Global: 300/2 = 150. Total: 250.
	if !casiIgual(depr, 250) {
		t.Errorf("depreciación PAN: %v; esperado 250 (100 directo + 150 global prorrateado)", depr)
	}

	// PAN2 recibe solo la parte global: 150.
	depr2 := s.depreciacionProductoPeriodo(db, "PAN2", 1, "2026-01", now())
	if !casiIgual(depr2, 150) {
		t.Errorf("depreciación PAN2: %v; esperado 150 (solo global prorrateado)", depr2)
	}
}

// TestCostoSubeConDepreciacion integra el motor: el desglose de un producto
// ensamblado incluye la depreciación del activo asignado.
func TestCostoSubeConDepreciacion(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO product_cost_config
		(product_id, costing_method, cost_center_id, is_assembled)
		VALUES ('PAN', 'RECIPE', 1, 1)`); err != nil {
		t.Fatalf("insertar config: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('PAN', 10)`); err != nil {
		t.Fatalf("insertar producto: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_information (product_id, cost) VALUES ('harina', 2)`); err != nil {
		t.Fatalf("insertar insumo: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cost_center (id, code, name, overhead_rate_unit, overhead_rate_hour, active)
		VALUES (1, 'PROD', 'Producción', 0, 0, 1)`); err != nil {
		t.Fatalf("insertar centro: %v", err)
	}
	// Activo directo: cuota 200/mes en enero 2026.
	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, product_id, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('HORNO', 'Horno', 'PAN', 12000, 0, '2026-01-01', 60, 1)`); err != nil {
		t.Fatalf("insertar activo: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO product_recipe
		(product_id, component_id, quantity, unit, loss_factor, step_order, labor_grade, hours)
		VALUES ('PAN', 'harina', 1, 'kg', 0, 1, '', 0)`); err != nil {
		t.Fatalf("insertar receta: %v", err)
	}

	in := s.cargarCostInputs(db, "PAN", "2026-01", 1)
	if !casiIgual(in.DegrPerUnidad, 200) {
		t.Errorf("depreciación por unidad: %v; esperado 200", in.DegrPerUnidad)
	}
	b := calcularCosto(in)
	if !casiIgual(b.Depreciation, 200) {
		t.Errorf("depreciación en desglose: %v; esperado 200", b.Depreciation)
	}
	if !casiIgual(b.Material, 2) {
		t.Errorf("material: %v; esperado 2", b.Material)
	}
	if !casiIgual(b.Final, 202) {
		t.Errorf("final: %v; esperado 202 (2 material + 200 depreciación)", b.Final)
	}
}

// TestCerrarDepreciacionIdempotente verifica que depreciacionDelPeriodo es
// determinista: mismo activo, mismo período, mismo resultado. El cierre real
// con ON DUPLICATE KEY se prueba contra MySQL en integración.
func TestCerrarDepreciacionIdempotente(t *testing.T) {
	_, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('A1', 'Activo', 10000, 1000, '2026-01-01', 60, 1)`); err != nil {
		t.Fatalf("insertar activo: %v", err)
	}

	ma1, ac1, bv1, c1, _ := depreciacionDelPeriodo(db, 1, "2026-06", now())
	ma2, ac2, bv2, c2, _ := depreciacionDelPeriodo(db, 1, "2026-06", now())
	if !casiIgual(ma1, ma2) || !casiIgual(ac1, ac2) || !casiIgual(bv1, bv2) || c1 != c2 {
		t.Errorf("no es idempotente: %v/%v/%v/%v vs %v/%v/%v/%v", ma1, ac1, bv1, c1, ma2, ac2, bv2, c2)
	}
	if !casiIgual(ma1, 150) {
		t.Errorf("cuota: %v; esperado 150 ((10000-1000)/60)", ma1)
	}
}

// TestRecalcularDepreciacionRechazaSinMotivo protege la regla: cambiar precio,
// vida, residual o fecha sin motivo se rechaza.
func TestRecalcularDepreciacionRechazaSinMotivo(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('A1', 'Activo', 10000, 1000, '2026-01-01', 60, 1)`); err != nil {
		t.Fatalf("insertar activo: %v", err)
	}
	r := httptest.NewRequest("POST", "/cost/assets/recalcular", nil)
	r = withScope(r, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("admin")})

	err := s.recalcularDepreciacion(db, r, "A1", "", "admin", now())
	if err == nil {
		t.Error("esperaba error por motivo vacío, pero no hubo error")
	}
}

// TestRecalcularDepreciacionConMotivoEscribeAuditoria verifica que con motivo
// el recálculo queda en audit_log.
func TestRecalcularDepreciacionConMotivoEscribeAuditoria(t *testing.T) {
	s, db := basePrueba(t)
	tablasCostosPrueba(t, db)

	if _, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, purchase_price, residual_value, purchase_date, useful_life_months, active)
		VALUES ('A1', 'Activo', 10000, 1000, '2026-01-01', 60, 1)`); err != nil {
		t.Fatalf("insertar activo: %v", err)
	}
	// Cambiar precio.
	if _, err := db.Exec(`UPDATE fixed_asset SET purchase_price = 15000 WHERE code = 'A1'`); err != nil {
		t.Fatalf("actualizar precio: %v", err)
	}

	r := httptest.NewRequest("POST", "/cost/assets/recalcular", nil)
	r = withScope(r, &scope{tenant: defaultTenantCode, db: db, sess: sessPrueba("admin")})

	if err := s.recalcularDepreciacion(db, r, "A1", "subió el precio del proveedor", "admin", now()); err != nil {
		t.Fatalf("recalcular con motivo: %v", err)
	}

	// Verificar que quedó en audit_log.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log
		WHERE action = 'recalcular_depreciacion' AND module = 'cost_assets'`).Scan(&n); err != nil {
		t.Fatalf("contar auditoría: %v", err)
	}
	if n != 1 {
		t.Errorf("registros de auditoría: %d; esperado 1", n)
	}
}
