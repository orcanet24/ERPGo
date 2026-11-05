package main

// exportar.go — El corazón del generador: exportar las plantillas REALES de
// report_template, decidir cuáles sirven para entrenamiento (las que el
// guardián de producción aceptaría) y armar las filas de entrenamiento del
// formato messages que el pipeline TRL/Unsloth consume.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Definición de informe: espejo estructural para validar la exportación
// ---------------------------------------------------------------------------

// definicionInforme calca la ESTRUCTURA de lo que el motor consume
// (ReportDefinition en report_engine.go, reportDefShape en ai_reports.go) solo
// para validar que las plantillas exportadas entran sin romper al modelo. La
// semántica fiscal la vela el guardián de producción, no este archivo.
type definicionInforme struct {
	Title   string `json:"title"`
	SQL     string `json:"sql"`
	Columns []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	} `json:"columns"`
	Totals []string `json:"totals"`
	Chart  *struct {
		Type   string `json:"type"`
		XKey   string `json:"x_key"`
		YKey   string `json:"y_key"`
		YLabel string `json:"y_label"`
		Title  string `json:"title"`
	} `json:"chart"`
}

// validarDefinicion reproduce los invariantes del guardián de definición
// (validateReportDefinition de interfaces/web/ai_reports.go): SQL presente, al
// menos una columna, totals que referencien columnas declaradas y chart que
// lea solo columnas que el SELECT devuelve. Devuelve motivo de rechazo («» = válida).
func validarDefinicion(raw []byte) string {
	var d definicionInforme
	if err := json.Unmarshal(raw, &d); err != nil {
		return fmt.Sprintf("definition no es JSON válido: %v", err)
	}
	if strings.TrimSpace(d.SQL) == "" {
		return "definition.sql es obligatorio"
	}
	if len(d.Columns) == 0 {
		return "definition.columns debe tener al menos una columna"
	}
	declaradas := map[string]bool{}
	for _, c := range d.Columns {
		if strings.TrimSpace(c.Key) == "" {
			return "hay una columna sin key"
		}
		declaradas[c.Key] = true
	}
	for _, t := range d.Totals {
		if !declaradas[t] {
			return fmt.Sprintf("totals apunta a %q, que no está en columns: el motor no puede calcularlo", t)
		}
	}
	if d.Chart != nil {
		switch d.Chart.Type {
		case "line", "bar", "pie", "doughnut":
		default:
			return fmt.Sprintf("chart.type debe ser line, bar, pie o doughnut (recibido: %q)", d.Chart.Type)
		}
		if strings.TrimSpace(d.Chart.XKey) == "" || strings.TrimSpace(d.Chart.YKey) == "" {
			return "chart.x_key y chart.y_key son obligatorios cuando se define chart"
		}
		for campo, k := range map[string]string{"x_key": d.Chart.XKey, "y_key": d.Chart.YKey} {
			if !declaradas[k] {
				return fmt.Sprintf("chart.%s apunta a %q, que no está en columns", campo, k)
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Guardián de importación: no enseñarle al modelo lo que producción rechaza
// ---------------------------------------------------------------------------

// tablasFiscalesProhibidas es la MISMA lista de validateReportSQLwithSchema en
// interfaces/web/ai_reports.go: el reporte IA no puede tocar tablas fiscales.
// Se declara aquí con una PRUEBA (finetune_test.go) que verifica contra el
// archivo fuente que siguen alineadas: si el guardián cambia, la prueba lo
// delata antes de que el dataset enseñe algo que producción rechazará.
var tablasFiscalesProhibidas = []string{
	"acc_transaction",
	"acc_vaucher",
	"acc_coa",
	"acc_subcode",
	"acc_subtype",
	"fiscal_setting",
	"fiscal_series",
	"tax_config",
	"retention",
	"pos_fiscal_blocks",
}

// palabrasFiscalesSensibles son las que el guardián rechaza si aparecen solas
// sin contexto de cálculo (GROUP BY / SUM / COUNT): misma regla, misma fuente.
var palabrasFiscalesSensibles = []string{
	"impuesto",
	"retencion",
	"igtf",
	"base_imponible",
	"igv",
	"total_fiscal",
	"subtotal_fiscal",
}

// reTablaDeSQL saca los nombres de tabla en FROM/JOIN para SANCIÓN y para el
// cálculo del módulo. sin (?i) acá: se pasa el SQL ya en minúsculas o se baja.
var reTablaDeSQL = regexp.MustCompile(`(?:from|join)\s+([a-z_][a-z0-9_]*)`)

// reAgregacion acepta GROUP BY, SUM( o COUNT( como contexto de cálculo.
var reAgregacion = regexp.MustCompile(`\bgroup\s+by\b|\bsum\(|\bcount\(`)

// sancionarSQL devuelve «» si el SQL es entrenable o el motivo de exclusión.
// Espejo del criterio del guardián en ai_reports.go: tablas fiscales (destino
// directo o en FROM/JOIN) y palabra fiscal sin agregación. Lo que aquí no pasa
// NO entra a finetune.jsonl con error silencioso: ejemplo que producción
// rechazaría enseña exactamente lo contrario de lo que se quiere.
func sancionarSQL(sqlText string) string {
	bajo := strings.ToLower(sqlText)
	for _, t := range tablasFiscalesProhibidas {
		if strings.Contains(bajo, t) {
			return fmt.Sprintf("el SQL usa la tabla fiscal %q: el guardián de producción lo rechaza, no va al dataset", t)
		}
	}
	if !reAgregacion.MatchString(bajo) {
		for _, kw := range palabrasFiscalesSensibles {
			if strings.Contains(bajo, kw) {
				return fmt.Sprintf("la palabra fiscal %q aparece sin GROUP BY/SUM/COUNT: el guardián la rechaza, no va al dataset", kw)
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Exportación real desde la base empresa
// ---------------------------------------------------------------------------

// plantillaExportada es una fila de report_template, ya con módulo asignado y
// veredicto del candado del generador.
type plantillaExportada struct {
	ID              int64
	Nombre          string
	Descripcion     string
	Creada          string
	DefinicionRaw   string // el JSON EXACTO que la base guarda (cuando aceptada)
	MotivoExclusion string // vacío = pasa el guardián del generador
	Modulo          string
}

// exportarPlantillas lee TODAS las plantillas activas de report_template y les
// asigna módulo según las tablas que su SQL toca. Devuelve además el resumen
// de avisos de exclusión para mostrarlo en consola: una exclusión invisible es
// un dataset malo que parece bueno.
func exportarPlantillas(db *sql.DB, modulos []modulo) ([]*plantillaExportada, string, error) {
	// Solo status=1: el borrado lógico del módulo (reportTemplateDelete) apaga
	// las plantillas muertas y el dataset no debe absorberlas.
	rows, err := db.Query(`SELECT id, name, description, definition, created_at
		FROM report_template WHERE status = 1 ORDER BY id`)
	if err != nil {
		return nil, "", fmt.Errorf("leer report_template (¿falta la migración schema_z_ai_agents en esta base?): %w", err)
	}
	defer rows.Close()

	var out []*plantillaExportada
	var avisos []string
	for rows.Next() {
		var p plantillaExportada
		if err := rows.Scan(&p.ID, &p.Nombre, &p.Descripcion, &p.DefinicionRaw, &p.Creada); err != nil {
			return nil, "", err
		}
		if motivo := validarDefinicion([]byte(p.DefinicionRaw)); motivo != "" {
			p.MotivoExclusion = motivo
			avisos = append(avisos, fmt.Sprintf("plantilla %d (%s) excluida: %s", p.ID, p.Nombre, motivo))
			out = append(out, &p)
			continue
		}
		if motivo := sancionarSQL(p.DefinicionRaw); motivo != "" {
			p.MotivoExclusion = motivo
			avisos = append(avisos, fmt.Sprintf("plantilla %d (%s) excluida: %s", p.ID, p.Nombre, motivo))
			out = append(out, &p)
			continue
		}
		p.Modulo = asignarModulo(p.DefinicionRaw, modulos)
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return out, strings.Join(avisos, "\n"), nil
}

// ---------------------------------------------------------------------------
// Módulo de cada plantilla y esquema real de cada módulo
// ---------------------------------------------------------------------------

// asignarModulo determina el módulo de una plantilla contando cuántas tablas
// del catálogo aparecen en su SQL (FROM/JOIN). Empate o cero: «desconocido».
func asignarModulo(sqlText string, modulos []modulo) string {
	bajo := strings.ToLower(sqlText)
	tocadas := map[string]bool{}
	for _, m := range reTablaDeSQL.FindAllStringSubmatch(bajo, -1) {
		tocadas[strings.ToLower(m[1])] = true
	}
	mejor := ""
	nMejor := 0
	for _, m := range modulos {
		n := 0
		for _, t := range m.Tables {
			if tocadas[strings.ToLower(t)] {
				n++
			}
		}
		if n > nMejor {
			mejor, nMejor = m.Key, n
		}
	}
	if mejor == "" {
		return "desconocido"
	}
	return mejor
}

// tablaSchema es una tabla introspectada VIVA de la base empresa, lista para
// viajar dentro del user prompt.
type tablaSchema struct {
	Tabla  string       `json:"tabla"`
	Campos []campoTabla `json:"campos"`
}

// esquemaModuloJSON es el esquema de UN módulo. Es el JSON que se inyecta en
// el user prompt del entrenamiento: una versión reducida pero REAL del esquema
// del MCP del proyecto (las columnas de verdad de la base actual).
type esquemaModuloJSON struct {
	Modulo   string        `json:"modulo"`
	Etiqueta string        `json:"etiqueta"`
	Tablas   []tablaSchema `json:"tablas"`
}

// esquemaDeModulos introspecciona en vivo las tablas de cada módulo presentes
// en ESTA base. Un módulo sin tablas presentes no genera esquema: el dataset
// no debe imitar reportes de módulos que la empresa no tiene.
func esquemaDeModulos(db *sql.DB, hay listadoTablas, modulos []modulo, motor string) ([]esquemaModuloJSON, error) {
	var out []esquemaModuloJSON
	for _, m := range modulos {
		e := esquemaModuloJSON{Modulo: m.Key, Etiqueta: m.Label}
		for _, t := range m.Tables {
			if !hay[strings.ToLower(t)] {
				continue
			}
			campos, err := introspeccionTabla(db, t, motor)
			if err != nil {
				return nil, fmt.Errorf("introspección de %s: %w", t, err)
			}
			e.Tablas = append(e.Tablas, tablaSchema{Tabla: t, Campos: campos})
		}
		if len(e.Tablas) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Preguntas y filas de entrenamiento
// ---------------------------------------------------------------------------

// reFraseCorta apaga la PRIMERA frase de la descripción: variante corta sin
// inventar palabras más allá de lo ya escrito ahí.
var reFraseCorta = regexp.MustCompile(`^[^.!?]*[.]?`)

// variantesDePregunta construye hasta 3 preguntas de usuario distintas por
// plantilla, TODAS redactadas solo con palabras de la fila (nombre,
// descripción, módulo): lo que se inventa diluye el dataset. La síntesis de
// paráfrasis más libres con LLM grande es la fase posterior del plan.
func variantesDePregunta(p *plantillaExportada, semilla int64, n int) []string {
	nombre := strings.TrimSpace(p.Nombre)
	desc := strings.TrimSpace(p.Descripcion)
	bank := []string{}

	switch {
	case nombre != "" && desc != "":
		bank = append(bank, fmt.Sprintf("Quiero un informe titulado «%s»: %s", nombre, desc))
	case nombre != "":
		bank = append(bank, fmt.Sprintf("Quiero el informe «%s» en ERPGo", nombre))
	case desc != "":
		bank = append(bank, "Quiero un informe que muestre: "+desc)
	default:
		bank = append(bank, "Quiero un informe personalizado de ERPGo")
	}

	if p.Modulo != "" && p.Modulo != "desconocido" {
		bank = append(bank, fmt.Sprintf("Necesito un informe del módulo de %s: %s", p.Modulo, titleCases(nombre, desc)))
	} else {
		bank = append(bank, fmt.Sprintf("Genera la definición JSON del informe «%s».", titleCases(nombre, desc)))
	}

	corta := strings.TrimSpace(reFraseCorta.FindString(desc))
	if corta == "" {
		corta = nombre
	}
	bank = append(bank, "Necesito ver "+strings.ToLower(corta))

	// Orden determinista pero rotado por la semilla, para que las primeras
	// variantes no siempre lleven la misma redacción.
	if semilla != 0 {
		rnd := rand.New(rand.NewSource(semilla + p.ID*1000003))
		rnd.Shuffle(len(bank), func(i, j int) { bank[i], bank[j] = bank[j], bank[i] })
	}
	if n > len(bank) {
		n = len(bank)
	}
	return bank[:n]
}

// titleCases arma el título del informe PROBANDO qué fuente tiene texto.
func titleCases(nombre, desc string) string {
	if strings.TrimSpace(nombre) != "" {
		return nombre
	}
	if strings.TrimSpace(desc) != "" {
		return desc
	}
	return "informe personalizado"
}

// filaEntreno ensambla UNA fila del formato messages que TRL/Unsloth consumen:
// system = el contrato REAL extraído del código; user = la solicitud con el
// MISMO esqueleto de iaPromptInforme (solicitud + esquema + cierre); assistant
// = el JSON EXACTO guardado por la base, sin prosa ni markdown alrededor.
func filaEntreno(contrato, esquemaModulo, pregunta, definicionJSON string) map[string]interface{} {
	var user strings.Builder
	user.WriteString("SOLICITUD DEL USUARIO:\n" + pregunta + "\n\n")
	if strings.TrimSpace(esquemaModulo) != "" {
		user.WriteString("ESQUEMA DEL MÓDULO (JSON):\n" + esquemaModulo + "\n\n")
	} else {
		user.WriteString("ESQUEMA DEL MÓDULO (JSON):\n(sin esquema de módulo asignado; decide con tablas operacionales genéricas)\n\n")
	}
	user.WriteString("Entrega la definición completa del informe: solo el objeto JSON del contrato. Si el esquema no muestra algo que necesitas, decide con lo que hay: no preguntes.")
	return map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": contrato},
			{"role": "user", "content": user.String()},
			{"role": "assistant", "content": definicionJSON},
		},
	}
}

// escribirJSONL serializa una fila con el escape HTML apagado («<» y «>» van
// tal cual, no \u003c): el JSON debe leerse como texto de prompt limpio.
func escribirJSONL(f io.Writer, fila interface{}) error {
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fila); err != nil {
		return fmt.Errorf("serializar fila JSONL: %w", err)
	}
	return nil
}
