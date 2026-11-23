package web

// ai_tools_mcp.go — Herramientas del MCP que completan el catálogo.
//
// Estas herramientas se registran en el MISMO aiToolRegistry del resto, y por
// eso el guardián de Nivel 1 las clasifica sin que nadie tenga que acordarse:
// una herramienta nueva no existe hasta que se declara, con su riesgo.
//
// El patrón común de las cuatro "propuesta" es el que define el diseño de la IA
// como empleado supervisado: PROPONEN, no aplican. Devuelven un borrador
// estructurado que un humano revisa. La frontera entre proponer y hacer está en
// el riesgo de la herramienta, no en una constante en el prompt.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------- describe_schema ----------

// aiToolDescribeSchema devuelve el mapa relacional de las tablas visibles.
//
// Devuelve las relaciones porque un modelo que sabe QUÉ tablas hay pero no CÓMO
// se relacionan escribe un JOIN equivocado. El mapa es de uso exclusivo del
// agente y nunca se cita al usuario (ver ai_rules.go): es contexto interno, no
// parte de la respuesta.
func (s *Server) aiToolDescribeSchema(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	// Scopado por el departamento del rol del agente, igual que el system
	// prompt: la herramienta no puede ser una puerta trasera al mapa entero.
	dept := aiDeptDelAgente(agent)
	mapa := s.aiEsquemaParaAgente(sess, dept)
	if tabla := strings.TrimSpace(aiStrParam(args, "table")); tabla != "" {
		if !aiCatalogIsVisible(tabla) {
			return string(mustJSON(aiToolError{Error: "esa tabla no está disponible para los agentes"})), nil
		}
		if !aiDeptEsVisible(dept, tabla) {
			return string(mustJSON(aiToolError{
				Error: "esa tabla pertenece a otro departamento y no es accesible desde tu rol",
			})), nil
		}
		// Las columnas salen del INTROSPECTOR, no de aiSQLColumns.
		//
		// aiSQLColumns devuelve el TEXTO de la consulta que lista las columnas,
		// no su resultado. La herramienta lo ponía en el campo `columns` tal
		// cual, así que cuando el agente pedía ver una tabla recibía esto:
		//
		//   "columns": "SELECT CONCAT(column_name, ' ', LOWER(column_type))
		//               FROM information_schema.columns WHERE ..."
		//
		// Medido en la corrida con IA: el agente pidió `{"table":"invoice"}` y
		// le devolvieron SQL en lugar de columnas. Peor que no devolver nada:
		// parecía información y no lo era.
		te := s.buscarEsquemaTabla(sess, tabla)
		if te == nil {
			return string(mustJSON(aiToolError{
				Error: "esa tabla no existe en la base de datos de esta empresa",
			})), nil
		}
		columnas := map[string]string{}
		for _, c := range te.Columnas {
			columnas[c.Nombre] = aiTipoCorto(c.Tipo)
		}
		salida := map[string]interface{}{
			"table":        te.Nombre,
			"description":  te.Descripcion,
			"columns":      columnas,
			"column_count": len(te.Columnas),
		}
		if len(te.PK) > 0 {
			salida["pk"] = te.PK
		}
		if len(te.Foranea) > 0 {
			salida["fk"] = te.Foranea
		}
		if rel, ok := aiCatalogRelations[strings.ToLower(tabla)]; ok {
			salida["join"] = rel
		}
		return string(mustJSON(salida)), nil
	}
	return string(mustJSON(map[string]interface{}{
		"departamento": dept,
		"schema_map":   mapa,
		"hint":         "Pide una tabla concreta con {\"table\":\"nombre\"} para ver columnas y relaciones.",
	})), nil
}

// ---------- legal_summary ----------

// aiToolLegalSummary entrega la normativa vigente cargada por la empresa.
//
// Es la herramienta que convierte a la IA en un empleado competente: no contesta
// "qué sé yo" sino "qué dice el sistema que está cargado". Si un tema no tiene
// norma, lo dice en vez de inventarla.
func (s *Server) aiToolLegalSummary(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	topic := strings.TrimSpace(aiStrParam(args, "topic"))
	if topic != "" && legalTopicName(topic) == topic && topic != "providencia" {
		// Una clave que no existe en el catálogo de temas casi siempre es un
		// error de tecleo del modelo, y devolver "no hay normas" lo haría pasar
		// por un vacío real.
		return string(mustJSON(aiToolError{
			Error: "tema desconocido: " + topic + ". Usa uno de: " + aiLegalTopicKeys(),
		})), nil
	}

	asOf := strings.TrimSpace(aiStrParam(args, "as_of"))
	if asOf == "" {
		asOf = today()
	}
	if !aiValidDate(asOf) {
		return string(mustJSON(aiToolError{Error: "as_of debe tener el formato AAAA-MM-DD"})), nil
	}

	texto, err := s.LegalSummaryFor(sess, topic, asOf)
	if err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}

	out := map[string]interface{}{"as_of": asOf, "summary": texto}
	if n := s.LegalUnverifiedCount(sess, asOf); n > 0 {
		out["unverified"] = n
		out["warning"] = fmt.Sprintf(
			"%d norma(s) cargadas sin validar por el administrador. Indícalo en la respuesta: no son respaldadas todavía.", n)
	}
	return string(mustJSON(out)), nil
}

func aiLegalTopicKeys() string {
	claves := []string{"iva", "islr", "retencion_iva", "retencion_islr", "nomina", "contabilidad", "libros"}
	return strings.Join(claves, ", ")
}

// ---------- query_report ----------

// aiToolQueryReport ejecuta un informe por plantilla.
//
// Es el atajo que resuelve el caso de uso central: la IA pide un dato que ya
// alguien definió como informe, en lugar de inventar un SQL nuevo. Un reporte de
// plantilla ya tiene sus validaciones, sus totales y su criterio, así que la IA
// hereda esa calidad en lugar de producir algo distinto cada vez.
func (s *Server) aiToolQueryReport(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	name := strings.TrimSpace(aiStrParam(args, "template"))
	if name == "" {
		list := s.loadReportTemplates(sess)
		nombres := make([]string, 0, len(list))
		for _, t := range list {
			nombres = append(nombres, t.Name)
		}
		if len(nombres) == 0 {
			return string(mustJSON(map[string]interface{}{
				"templates": []string{},
				"hint":      "No hay informes definidos. Propón uno con report.save_template.",
			})), nil
		}
		return string(mustJSON(map[string]interface{}{
			"templates": nombres,
			"hint":      "Elige uno con {\"template\":\"nombre\",\"params\":{...}}",
		})), nil
	}

	var defJSON string
	for _, t := range s.loadReportTemplates(sess) {
		if strings.EqualFold(t.Name, name) {
			defJSON = t.Definition
			break
		}
	}
	if defJSON == "" {
		return string(mustJSON(aiToolError{Error: "no existe el informe '" + name + "'"})), nil
	}

	var def ReportDefinition
	if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
		return string(mustJSON(aiToolError{Error: "la plantilla está dañada: " + err.Error()})), nil
	}

	// Los parámetros llegan como objeto: se ignoran los que la plantilla no
	// declara, en vez de inventar un filtro que no existe.
	values := map[string]string{}
	if crudos, ok := args["params"].(map[string]interface{}); ok {
		for k, v := range crudos {
			values[k] = fmt.Sprintf("%v", v)
		}
	}

	// query_report también pasa por la FRONTERA ÚNICA (F2/F4). El motor tipado
	// (report_engine.go:RunReportDefinition) solo comprueba verbos con
	// aiValidateSQLSelect, así que una plantilla guardada con
	// `SELECT api_key FROM ai_providers` se ejecutaba por aquí sin lista negra ni
	// alcance. Se valida con el departamento del AGENTE que consulta —no con el
	// del que creó el informe— y se reescribe def.SQL EN MEMORIA con la cadena
	// aprobada: lo que el motor ejecuta es exactamente lo que pasó la frontera.
	dept := aiDeptDelAgente(agent)
	aprobado, err := validarSQLInformes(def.SQL, dept, def.EffectiveMaxRows())
	if err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	def.SQL = aprobado

	run, err := s.RunReportDefinition(s.requestForSession(sess), &def, values)
	if err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	return string(mustJSON(reportRunJSON(run))), nil
}

// ---------- Propuestas ----------

// aiProposal es lo que devuelven las herramientas de propuesta.
//
// La clave del diseño está en el campo applied: SIEMPRE false. La IA propone y
// un humano ejecuta. Convertir una propuesta en una acción es un acto
// consciente de alguien, y ese alguien queda en la auditoría.
type aiProposal struct {
	Template  string                 `json:"template"`
	Applied   bool                   `json:"applied"`
	Reason    string                 `json:"reason"`
	Summary   string                 `json:"summary"`
	Payload   map[string]interface{} `json:"payload"`
	NextSteps []string               `json:"next_steps"`
}

// aiToolProposeTransaction propone un asiento contable, sin aplicarlo.
//
// Las validaciones que hace aquí (que las cuentas existan y que el asiento
// cuadre) NO son por aesthetics: una propuesta descuadrada que llega a un humano
// como "sugerencia" se acepta y se detecta meses después, cuando ya no es un
// error de la IA sino del contador que la aprobó.
func (s *Server) aiToolProposeTransaction(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	nota := strings.TrimSpace(aiStrParam(args, "note"))
	if ok, motivo := aiDetectFraudIntent(nota + " " + fmt.Sprintf("%v", args)); ok {
		return string(mustJSON(aiToolBlocked{
			Error:   "la propuesta contiene indicios de " + motivo,
			Blocked: true, Rule: "nivel-1", Tool: "account.propose_transaction",
		})), nil
	}

	vtype := strings.ToUpper(aiStrParam(args, "vtype"))
	if vtype == "" {
		vtype = "JV"
	}
	raw, _ := args["lines"].([]interface{})
	if len(raw) < 2 {
		return string(mustJSON(aiToolError{Error: "un comprobante requiere al menos 2 líneas"})), nil
	}

	type linea struct {
		COAID  int64   `json:"coa_id"`
		Debit  float64 `json:"debit"`
		Credit float64 `json:"credit"`
	}
	var lineas []linea
	var sumD, sumC float64
	for _, rl := range raw {
		m, _ := rl.(map[string]interface{})
		if m == nil {
			continue
		}
		l := linea{
			COAID:  int64(aiNumParam(m, "coa_id")),
			Debit:  round2(aiNumParam(m, "debit")),
			Credit: round2(aiNumParam(m, "credit")),
		}
		if l.COAID <= 0 {
			return string(mustJSON(aiToolError{Error: "cada línea necesita un coa_id válido"})), nil
		}
		var existe int
		_ = s.dbForSession(sess).QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE HeadCode = ?`, l.COAID).Scan(&existe)
		if existe == 0 {
			return string(mustJSON(aiToolError{
				Error: fmt.Sprintf("la cuenta %d no existe en el plan de cuentas", l.COAID),
			})), nil
		}
		if l.Debit > 0 && l.Credit > 0 {
			return string(mustJSON(aiToolError{Error: "una línea no puede ser débito y crédito a la vez"})), nil
		}
		if l.Debit == 0 && l.Credit == 0 {
			continue
		}
		sumD += l.Debit
		sumC += l.Credit
		lineas = append(lineas, l)
	}
	if len(lineas) < 2 {
		return string(mustJSON(aiToolError{Error: "se requieren al menos 2 líneas con importe"})), nil
	}
	if round2(sumD) != round2(sumC) {
		return string(mustJSON(aiToolError{
			Error: fmt.Sprintf("el asiento no cuadra: débito %.2f vs crédito %.2f", sumD, sumC),
		})), nil
	}

	prop := aiProposal{
		Template: "account.propose_transaction",
		Applied:  false,
		Reason:   "Las propuestas no se aplican: la crea un agente y la ejecuta un humano.",
		Summary: fmt.Sprintf("Asiento %s propuesto: %.2f en débitos y %.2f en créditos, %d líneas.",
			vtype, sumD, sumC, len(lineas)),
		Payload: map[string]interface{}{
			"vtype": vtype, "ref": aiStrParam(args, "ref"), "note": nota, "lines": lineas,
		},
		NextSteps: []string{
			"Revisar las cuentas y la descripción.",
			"Crear el comprobante en Contabilidad.",
			"Si el asiento nace de un documento, preferir voucher.create para que el documento y el asiento no puedan separarse.",
		},
	}
	return string(mustJSON(prop)), nil
}

// aiToolProposeInvoice propone una factura, sin registrarla.
func (s *Server) aiToolProposeInvoice(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	nota := strings.TrimSpace(aiStrParam(args, "note")) + " " + aiStrParam(args, "description")
	if ok, motivo := aiDetectFraudIntent(nota); ok {
		return string(mustJSON(aiToolBlocked{
			Error:   "la propuesta contiene indicios de " + motivo,
			Blocked: true, Rule: "nivel-1", Tool: "invoice.propose",
		})), nil
	}

	customerID := int64(aiNumParam(args, "customer_id"))
	if customerID <= 0 {
		return string(mustJSON(aiToolError{Error: "customer_id es obligatorio"})), nil
	}
	var existe int
	if err := s.dbForSession(sess).QueryRow(`SELECT COUNT(*) FROM customer_information WHERE customer_id = ?`,
		customerID).Scan(&existe); err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	if existe == 0 {
		return string(mustJSON(aiToolError{Error: "el cliente indicado no existe"})), nil
	}

	fecha := strings.TrimSpace(aiStrParam(args, "date"))
	if fecha == "" {
		fecha = today()
	}
	if !aiValidDate(fecha) {
		return string(mustJSON(aiToolError{Error: "la fecha debe tener el formato AAAA-MM-DD"})), nil
	}

	items, _ := args["items"].([]interface{})
	var total float64
	for _, it := range items {
		m, _ := it.(map[string]interface{})
		if m == nil {
			continue
		}
		total += aiNumParam(m, "quantity") * aiNumParam(m, "rate")
	}

	// Aviso de período cerrado: facturar en un mes ya firmado no es un error
	// técnico, es un problema contable que conviene decir en voz alta.
	aviso := ""
	if aiPeriodOf(fecha) != "" && s.PeriodIsClosed(s.requestForSession(sess), aiPeriodOf(fecha)) {
		aviso = "ATENCIÓN: el período " + aiPeriodOf(fecha) + " está cerrado. La factura solo debería " +
			"emitirse si existe una autorización para esa fecha."
	}

	prop := aiProposal{
		Template: "invoice.propose",
		Applied:  false,
		Reason:   "Las propuestas no se aplican: la crea un agente y la ejecuta un humano.",
		Summary: fmt.Sprintf("Factura propuesta para el cliente %d por %.2f, fechada el %s.",
			customerID, round2(total), fecha),
		Payload: map[string]interface{}{
			"customer_id": customerID, "date": fecha, "items": items, "note": nota,
		},
		NextSteps: []string{
			"Crear la factura en borrador y revisarla.",
			"Publicarla solo cuando los precios y el IVA estén confirmados.",
		},
	}
	if aviso != "" {
		prop.NextSteps = append(prop.NextSteps, aviso)
	}
	return string(mustJSON(prop)), nil
}

// aiToolProposePayroll propone una corrida de nómina, sin ejecutarla.
//
// A diferencia de las otras propuestas, aquí NO se incluyen cifras calculadas:
// la nómina se calcula en el servidor cuando alguien la lanza, con las tasas
// vigentes de esa fecha. Devolver un monto calculado por el modelo sería
// inventar una nómina, justo lo que este módulo evita.
func (s *Server) aiToolProposePayroll(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	periodo := strings.TrimSpace(aiStrParam(args, "period"))
	if periodo == "" {
		return string(mustJSON(aiToolError{Error: "period es obligatorio, con el formato AAAAMM o AAAA-MM"})), nil
	}
	normalizado, err := aiNormalizePeriod(periodo)
	if err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}

	// El estado real del período se consulta, para que la propuesta diga la
	// verdad y no lo que el modelo suponía.
	estado := "abierto"
	if s.PeriodIsClosed(s.requestForSession(sess), normalizado) {
		estado = "cerrado"
	}
	pendientes, _ := s.unverifiedTaxRates(s.requestForSession(sess), today())

	payload := map[string]interface{}{"period": normalizado, "period_status": estado}

	prop := aiProposal{
		Template: "payroll.propose",
		Applied:  false,
		Reason:   "La nómina nunca se calcula en la respuesta del agente: la calcula el servidor con las tasas vigentes.",
		Summary:  "Corrida de nómina propuesta para el período " + normalizado + " (estado: " + estado + ").",
		Payload:  payload,
		NextSteps: []string{
			"Lanzar la corrida en RRHH > Nómina y revisar el resultado antes de publicar.",
			"No se puede publicar mientras haya tasas fiscales sin verificar.",
		},
	}
	if len(pendientes) > 0 {
		prop.Payload["unverified_rates"] = len(pendientes)
		prop.NextSteps = append(prop.NextSteps,
			fmt.Sprintf("%d tasa(s) sin verificar: hay que validarlas antes de publicar.", len(pendientes)))
	}
	return string(mustJSON(prop)), nil
}

// aiPeriodOf convierte una fecha ISO en el período AAAAMM que usa el cierre.
func aiPeriodOf(fecha string) string {
	if len(fecha) < 7 {
		return ""
	}
	return strings.ReplaceAll(fecha[:7], "-", "")
}

// aiNormalizePeriod acepta AAAA-MM y AAAAMM y devuelve siempre AAAAMM.
//
// Se aceptan las dos formas porque el personal escribe de las dos maneras y un
// rechazo por un guion es una fricción absurda en un campo de uso diario.
func aiNormalizePeriod(p string) (string, error) {
	v := strings.TrimSpace(p)
	v = strings.ReplaceAll(v, "-", "")
	if len(v) != 6 {
		return "", fmt.Errorf("el período debe tener el formato AAAAMM o AAAA-MM")
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("el período debe contener solo dígitos")
		}
	}
	mes := v[4:]
	if mes < "01" || mes > "12" {
		return "", fmt.Errorf("mes inválido en el período")
	}
	return v, nil
}
