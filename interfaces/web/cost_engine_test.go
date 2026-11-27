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
			cost REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE product_recipe (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id TEXT NOT NULL,
			component_id TEXT NOT NULL,
			quantity REAL NOT NULL DEFAULT 0,
			loss_factor REAL NOT NULL DEFAULT 0,
			step_order INTEGER NOT NULL DEFAULT 0
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
// Guardián del esquema
// ---------------------------------------------------------------------------

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
