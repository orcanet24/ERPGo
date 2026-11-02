package web

// hr_concepto.go — Catálogo de conceptos de nómina con ATRIBUTOS JURÍDICOS.
//
// Hallazgo 1 de docs/REVISION-LEGAL-2026-11.md: el carácter salarial de un
// pago se deriva de los atributos del concepto —causa del pago, regularidad,
// moneda y soporte requerido— y NUNCA del nombre del concepto. La jurisprudencia
// de la Sala de Casación Social lo obliga:
//
//   - 021/2022 (exp. 19-327): la carga de probar que un pago NO es salarial es
//     del patrono; un concepto recurrente sin soporte se presume salarial.
//     Por eso los defectos del catálogo son los conservadores: retribución,
//     recurrente, bolívares. Atributo faltante o dudoso ≡ más salarial, no menos.
//   - 523/2025 (exp. 24-058): el complemento en efectivo o divisas NO es salario
//     salvo pacto expreso (art. 105.2 LOT).
//   - 127/2022 (exp. 20-070): vacaciones no integran el salario normal para
//     utilidades: las prestaciones tienen su propia base y NO se mezclan.
//   - 340/2026 (exp. 25-327): si no se prueba el salario en divisas, la base de
//     condena es el salario mínimo del decreto vigente. La moneda del concepto
//     se registra aquí para que la separación por moneda (siguiente paso del
//     informe) tenga de dónde leerla.
//
// El motor de nómina (payroll.go) todavía recibe líneas por concepto. Este
// catálogo no cambia su cálculo: le pone JURISPRUDENCIA a cada concepto como
// metadato. Derivar de ahí las tres bases del informe es la historia siguiente.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// ---------- Vocabularios de los atributos ----------

// Causa del pago (art. 105 LOT, y 105.2 para pagos en divisa).
const (
	CausaRetribucion     = "retribucion"      // contraprestación directa por servicios prestados: núcleo del salario.
	CausaBeneficioSocial = "beneficio_social" // prestación social (bono vacacional, utilidades): integra prestaciones, no el salario normal.
	CausaReembolso       = "reembolso"        // reintegro de gastos del trabajador (hidrocarburos, viáticos): jamás es salario.
)

// Regularidad: el TEST DE FRECUENCIA que la Sala usa para distinguir lo
// salarial de lo esporádico. Lo recurrente integra; lo eventual se examina.
const (
	RegularidadRecurrente = "recurrente" // se paga en cada período de nómina.
	RegularidadAnual      = "anual"      // una vez al año o por cierre: utilidades, bono vacacional.
	RegularidadEventual   = "eventual"   // ocasional, frecuencia no repetida.
)

// Moneda en la que se liquida el concepto. El informe exige bases separadas
// (Bs / divisa); hasta que existan, el atributo queda registrado.
const (
	MonedaVES = "VES"
	MonedaUSD = "USD"
)

// Carácter salarial DERIVADO, con el vocabulario del informe:
const (
	CaracSalarioNormal = "salario_normal" // integra el salario normal (art. 104): utilidades, vacaciones, feriados.
	CaracPrestacional  = "prestacional"   // integra las prestaciones (art. 142), NO el salario normal.
	CaracNoSalarial    = "no_salarial"    // reintegro de gastos: nunca es salario.
	CaracRiesgo        = "riesgo"         // exige soporte y no lo tiene: se PRESUME salarial (021/2022).
)

// hrConcepto es un concepto del catálogo con sus atributos jurídicos.
type hrConcepto struct {
	ID          int64
	Code        string
	Name        string
	Kind        string // earning | deduction | employer_charge
	Causa       string
	Regularidad string
	Moneda      string
	SoporteReq  bool // el patrono declara que este pago exige respaldo documental
	Status      int
}

// esValidaCausa verifica que la causa esté en el vocabulario cerrado.
func esValidaCausa(v string) bool {
	switch v {
	case CausaRetribucion, CausaBeneficioSocial, CausaReembolso:
		return true
	}
	return false
}

// esValidaRegularidad verifica la regularidad contra el vocabulario cerrado.
func esValidaRegularidad(v string) bool {
	switch v {
	case RegularidadRecurrente, RegularidadAnual, RegularidadEventual:
		return true
	}
	return false
}

// esValidaMoneda verifica la moneda contra el vocabulario cerrado.
func esValidaMoneda(v string) bool {
	switch v {
	case MonedaVES, MonedaUSD:
		return true
	}
	return false
}

// caracterSalarial deriva el carácter del PAGO de los atributos, no del nombre.
//
// El orden importa y es deliberado: primero se expulsa lo que jamás integra
// (reembolso), después lo que integra otra cosa (prestación), sólo entonces la
// retribución es salario — y la salarial con presunción si falta el soporte,
// porque demostrar lo contrario es tarea del patrono (021/2022).
//
// soporteCargado hoy se interpreta como «no hay registro de respaldo en el
// sistema». Cuando exista el archivo de soportes, este parámetro lo consulta.
func caracterSalarial(c hrConcepto, soporteCargado bool) string {
	switch c.Causa {
	case CausaReembolso:
		return CaracNoSalarial
	case CausaBeneficioSocial:
		return CaracPrestacional
	case CausaRetribucion:
		if c.SoporteReq && !soporteCargado {
			return CaracRiesgo
		}
		return CaracSalarioNormal
	}
	// Causa inválida no debería existir (el guard de hrConceptosSave rechaza
	// el valor libre). Si aparece en datos legados corruptos, el trato
	// conservador presumir la salarialidad con riesgo, no sacarla del cálculo.
	return CaracRiesgo
}

// ---------- Columnas: el ensure idempotente de la casa ----------

// Las columnas nuevas se garantizan en ARRANQUE y no por migración: MySQL no
// soporta `ADD COLUMN IF NOT EXISTS`, y el ejecutor de migraciones anota como
// fallido el archivo que no se aplicó — se reintenta y avisa en cada arranque.
// El patrón del proyecto (asegurarColumna en wizard.go, ensureDemoColumn en
// la guía) es ALTER directo y silencio si la columna ya existe.
var hrConceptoColsMu sync.Mutex
var hrConceptoCols = map[string]bool{} // puntero de base → columnas ya verificadas

func (s *Server) asegurarHrConceptoAtributos(r *http.Request) {
	db := s.dbFor(r)
	llave := fmt.Sprintf("%p", db)
	hrConceptoColsMu.Lock()
	if hrConceptoCols[llave] {
		hrConceptoColsMu.Unlock()
		return
	}
	hrConceptoColsMu.Unlock()

	// Defaults conservadores (021/2022): lo que no se sabe, se presume salarial.
	for _, col := range []string{
		"causa_pago        VARCHAR(20) NOT NULL DEFAULT 'retribucion'",
		"regularidad       VARCHAR(20) NOT NULL DEFAULT 'recurrente'",
		"moneda            VARCHAR(10) NOT NULL DEFAULT 'VES'",
		"soporte_requerido TINYINT(1)  NOT NULL DEFAULT 0",
	} {
		// Si la columna ya existe, MySQL y SQLite responden "duplicate
		// column": es el caso normal del segundo arranque y se silencia,
		// igual que asegurarColumna hace para los ALTER del wizard.
		_, _ = db.Exec(`ALTER TABLE hr_payroll_concept ADD COLUMN ` + col)
	}

	hrConceptoColsMu.Lock()
	hrConceptoCols[llave] = true
	hrConceptoColsMu.Unlock()
}

// ---------- Handlers ----------

// registerConceptoRoutes registra el catálogo en el árbol de rutas de RRHH.
func (s *Server) registerConceptoRoutes() {
	s.mux.HandleFunc("GET /hr/conceptos", s.hrConceptosPantalla)
	s.mux.HandleFunc("GET /hr/conceptos/data", s.hrConceptosData)
	s.mux.HandleFunc("POST /hr/conceptos/save", s.hrConceptosSave)
}

// hrConceptosPantalla es la pantalla del catálogo.
func (s *Server) hrConceptosPantalla(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "hr_conceptos.html", map[string]interface{}{
		"Title": "Conceptos de Nómina",
	})
}

// hrConceptosData devuelve el catálogo en el formato DataTables del proyecto,
// con el carácter salarial YA DERIVADO POR SERVIDOR. La conclusión jurídica
// sale de la base de datos; el navegador sólo la pinta.
func (s *Server) hrConceptosData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))

	s.asegurarHrConceptoAtributos(r)

	rows, err := s.dbFor(r).Query(`
		SELECT id, code, name, kind, causa_pago, regularidad, moneda,
		       soporte_requerido, status
		FROM hr_payroll_concept
		WHERE status = 1
		ORDER BY kind, code`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var (
			c   hrConcepto
			sop int
		)
		if rows.Scan(&c.ID, &c.Code, &c.Name, &c.Kind, &c.Causa,
			&c.Regularidad, &c.Moneda, &sop, &c.Status) != nil {
			continue
		}
		c.SoporteReq = sop == 1

		// El carácter sólo tiene sentido para ASIGNACIONES: una deducción o
		// una carga patronal no integra nada. Para ellas el campo queda vacío
		// y la pantalla lo muestra como «—».
		carac := ""
		if c.Kind == "earning" {
			// Sin registro de soporte cargado: si el concepto exige soporte,
			// la derivación marca el riesgo (presunción salarial, 021/2022).
			carac = caracterSalarial(c, false)
		}
		list = append(list, map[string]interface{}{
			"id": c.ID, "code": c.Code, "name": c.Name, "kind": c.Kind,
			"causa": c.Causa, "regularidad": c.Regularidad, "moneda": c.Moneda,
			"soporte_requerido": c.SoporteReq, "salarial": carac,
		})
	}
	total := len(list)
	writeJSON(w, map[string]interface{}{
		"draw": draw, "recordsTotal": total, "recordsFiltered": total, "data": list,
	})
}

// hrConceptosSave crea o actualiza un concepto del catálogo con sus atributos.
//
// Valida TODO el vocabulario en el servidor: la tabla no admite valores libres
// porque el carácter salarial se deriva de ellos — un valor mal escrito haría
// derivar un carácter distinto al que el contador creyó fijar.
func (s *Server) hrConceptosSave(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	name := strings.TrimSpace(r.FormValue("name"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	causa := strings.TrimSpace(r.FormValue("causa"))
	regularidad := strings.TrimSpace(r.FormValue("regularidad"))
	moneda := strings.ToUpper(strings.TrimSpace(r.FormValue("moneda")))
	soporte := r.FormValue("soporte_requerido") == "1"
	id := strings.TrimSpace(r.FormValue("id"))

	if code == "" || name == "" {
		jsonError(w, "El código y el nombre del concepto son obligatorios", 400)
		return
	}
	if len(code) > 30 {
		jsonError(w, "El código no puede superar 30 caracteres", 400)
		return
	}
	switch kind {
	case "earning", "deduction", "employer_charge":
	default:
		jsonError(w, "Tipo inválido: earning, deduction o employer_charge", 400)
		return
	}
	if !esValidaCausa(causa) {
		jsonError(w, "Causa inválida: retribucion, beneficio_social o reembolso", 400)
		return
	}
	if !esValidaRegularidad(regularidad) {
		jsonError(w, "Regularidad inválida: recurrente, anual o eventual", 400)
		return
	}
	if !esValidaMoneda(moneda) {
		jsonError(w, "Moneda inválida: VES o USD", 400)
		return
	}

	s.asegurarHrConceptoAtributos(r)
	db := s.dbFor(r)

	// El código es la clave con que el motor y los recibos citan el concepto:
	// repetido no vale, ni contra conceptos activos ni al editar otro distinto.
	var dup int
	if id != "" {
		if err := db.QueryRow(`SELECT COUNT(*) FROM hr_payroll_concept WHERE code = ? AND id <> ?`, code, id).Scan(&dup); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		if err := db.QueryRow(`SELECT COUNT(*) FROM hr_payroll_concept WHERE code = ?`, code).Scan(&dup); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	if dup > 0 {
		jsonError(w, "Ya existe un concepto con ese código", 400)
		return
	}

	sop := boolToInt(soporte)
	if id == "" {
		res, err := db.Exec(`
			INSERT INTO hr_payroll_concept
				(code, name, kind, causa_pago, regularidad, moneda, soporte_requerido, status)
			VALUES (?,?,?,?,?,?,?,1)`,
			code, name, kind, causa, regularidad, moneda, sop)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		nuevo, _ := res.LastInsertId()
		id = fmt.Sprintf("%d", nuevo)
	} else {
		res, err := db.Exec(`
			UPDATE hr_payroll_concept
			SET code=?, name=?, kind=?, causa_pago=?, regularidad=?, moneda=?, soporte_requerido=?
			WHERE id = ?`,
			code, name, kind, causa, regularidad, moneda, sop, id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "No existe ese concepto", 404)
			return
		}
	}

	s.hrAudit(r, sessUserID(s.sessionFromRequest(r)), "save", "hr_payroll_concept", id, nil, map[string]interface{}{
		"code": code, "kind": kind, "causa": causa,
		"regularidad": regularidad, "moneda": moneda, "soporte_requerido": soporte,
	})
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Concepto guardado", "id": id,
		"redirect": saveRedirect(r, "/hr/conceptos"),
	})
}
