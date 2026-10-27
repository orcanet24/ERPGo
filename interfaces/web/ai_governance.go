package web

// ai_governance.go — NIVEL 1: reglas que NO están en el prompt.
//
// La idea que sostiene todo el módulo: un prompt es una SUGERENCIA para el
// modelo. Si la única defensa fuera el system prompt, bastaría con que un
// usuario, un dato consultado o un agente con prompt personalizado pidiera
// saltarse una regla. Por eso las reglas que no se negocian se comprueban en
// Go, inmediatamente antes de ejecutar cada acción.
//
// Tres niveles, en orden de poder decreciente:
//   Nivel 1 (este archivo, compilado): el administrador NO puede relajarlo.
//     Ni editando la BD, ni con un prompt personalizado, ni siendo
//     dueño de la empresa. Protege la contabilidad, la auditoría y
//     bloquea el fraude fiscal.
//   Nivel 2 (configurable): qué puede hacer cada agente, con qué límites.
//     El administrador lo endurece, nunca lo afloja por debajo del Nivel 1.
//   Nivel 3 (el prompt): el estilo, el tono, el formato del informe.
//
// Consecuencia práctica: la IA puede proponer y redactar sola, y puede operar
// con permisos por rol, pero ninguna combinación de prompt puede convencer al
// código de romper una regla. Eso es lo que permite soltar el "freno de F1" sin
// convertir el ERP en un riesgo para el cliente.

import (
	"fmt"
	"strings"
)

// RiskLevel clasifica qué tan reversible es una acción. Es el criterio de
// diseño central: no "¿qué tan listo está el modelo?" sino "¿qué tan
// reversible es equivocarse?".
type RiskLevel string

const (
	RiskRead     RiskLevel = "read"     // consultar: sin efecto, la IA sola
	RiskDraft    RiskLevel = "draft"    // redactar/borrador: se puede borrar
	RiskExecute  RiskLevel = "execute"  // ejecuta con validación y alerta
	RiskCritical RiskLevel = "critical" // nómina, pagos, anulaciones: humano
)

// El nivel de riesgo de cada herramienta NO vive en este archivo: vive en
// aiToolRegistry (ai_tools.go), junto a su descripción y a su función de
// ejecución, y se consulta con aiToolRiskOf.
//
// Esa indivisión de nombres es deliberada. Cuando el mapa de riesgo era un
// segundo lugar aparte, se añadió una herramienta nueva al catálogo y al
// dispatcher pero se olvidó en el mapa, y el guardián la bloqueó en producción.
// Una sola lista no puede desincronizarse de sí misma.
//
// El catálogo sigue siendo cerrado: una herramienta que no esté en el registro
// se rechaza por omisión (ver aiGuardAction). Declararla ahí la clasifica; el
// campo Run decide si además está implementada.

// aiForbiddenTables son tablas que la IA jamás puede leer por SQL. Están
// excluidas del mapa del esquema Y bloqueadas aquí: la segunda barrera importa
// porque protege si alguien amplía el mapa por error o por una tabla nueva.
var aiForbiddenTables = []string{
	"user_login", "users", "ai_providers", "ai_provider", "ai_setting",
	"sec_userrole", "sec_role", "audit_log", "audit_chain_blocks",
	"queue_tasks", "sec_password",
}

// aiForbiddenWriteTables nunca admiten escritura desde un agente, sin importar
// el rol ni el prompt. La contabilidad y la auditoría son intocables por IA:
// un agente puede leer un balance, pero jamás escribir en él.
var aiForbiddenWriteTables = []string{
	"acc_transaction", "acc_vaucher", "acc_coa", "acc_subcode", "acc_subtype",
	"audit_log", "audit_chain_blocks",
}

// GovernanceDecision es el veredicto del guardián para una acción.
type GovernanceDecision struct {
	Allowed    bool
	Risk       RiskLevel
	Reason     string
	NeedsHuman bool // si además de permitirse, exige aprobación humana
}

// aiRiskSeverity ordena los niveles de riesgo.
//
// Es un mapa, NO una comparación de strings: "read" > "execute" en orden
// alfabético, así que comparar los RiskLevel directamente haría que una
// lectura fuera tratada como más riesgosa que una ejecución. La severidad es un
// dato explícito para que el orden sea el que queremos y no el del alfabeto.
var aiRiskSeverity = map[RiskLevel]int{
	RiskRead:     1,
	RiskDraft:    2,
	RiskExecute:  3,
	RiskCritical: 4,
}

// aiAtLeast indica si un riesgo alcanza o supera un nivel mínimo. Se usa para
// razonar sobre el nivel sin escribir comparaciones sueltas en cada sitio: el
// orden vive en un solo mapa (ver TestOrdenDeRiesgoEsElIntencionado).
func aiAtLeast(r RiskLevel, min RiskLevel) bool {
	return aiRiskSeverity[r] >= aiRiskSeverity[min]
}

// aiGuardAction es el punto único por donde pasa TODA acción de la IA. Se llama
// al inicio de aiDispatchTool, antes de ejecutar nada.
func aiGuardAction(toolName string, args map[string]interface{}, operatorEnabled bool) GovernanceDecision {
	tool, known := aiToolByName[toolName]
	if !known {
		// Herramienta no declarada: se rechaza por omisión. Una whitelist es
		// segura; una blacklist obliga a perseguir cada caso nuevo.
		return GovernanceDecision{
			Risk:   RiskCritical,
			Reason: fmt.Sprintf("herramienta '%s' no está registrada en el catálogo de riesgos; el guardián la rechaza por omisión", toolName),
		}
	}

	// Tablas protegidas. La revisión ocurre tanto en lectura como en escritura:
	// consultar credenciales o la cadena de auditoría por SQL sería la misma
	// fuga de datos que escribir sobre ellas.
	if t := forbiddenTarget(args); t != "" {
		accion := "consultar"
		if isWriteIntent(toolName) {
			accion = "escribir sobre"
		}
		return GovernanceDecision{
			Risk:   RiskCritical,
			Reason: fmt.Sprintf("%s '%s' está prohibido a los agentes por regla del sistema", accion, t),
		}
	}

	// Modo operador. La condición la declara el registro (OperatorOnly), no se
	// deduce del nivel de riesgo: un borrador contable y un cobro cambian de
	// categoría a un tiempo, y si la regla se dedujera del nivel habría que
	// recordarla cada vez que cambiara una clasificación.
	if tool.OperatorOnly && !operatorEnabled {
		return GovernanceDecision{
			Risk:   tool.Risk,
			Reason: "modo operador desactivado para este agente",
		}
	}

	return GovernanceDecision{
		Allowed:    true,
		Risk:       tool.Risk,
		NeedsHuman: tool.Risk == RiskCritical,
	}
}

// isWriteIntent indica si la herramienta intenta modificar datos.
func isWriteIntent(toolName string) bool {
	switch {
	case strings.HasPrefix(toolName, "report.save"),
		strings.HasPrefix(toolName, "invoice.create"),
		strings.HasPrefix(toolName, "invoice.post"),
		strings.HasPrefix(toolName, "account.post"),
		strings.HasPrefix(toolName, "customer."),
		strings.HasPrefix(toolName, "supplier."),
		strings.HasPrefix(toolName, "product."),
		strings.HasPrefix(toolName, "payroll."),
		strings.HasPrefix(toolName, "payment."),
		strings.HasPrefix(toolName, "purchase."),
		strings.HasPrefix(toolName, "voucher."),
		strings.HasPrefix(toolName, "invoice.anull"):
		return true
	}
	return false
}

// forbiddenTarget revisa los argumentos en busca de una tabla protegida.
func forbiddenTarget(args map[string]interface{}) string {
	forbidden := append(append([]string{}, aiForbiddenWriteTables...), aiForbiddenTables...)
	for k, v := range args {
		key := strings.ToLower(k)
		if !strings.Contains(key, "table") && !strings.Contains(key, "sql") {
			continue
		}
		text := fmt.Sprintf("%v", v)
		low := strings.ToLower(text)
		for _, tbl := range forbidden {
			if strings.Contains(low, tbl) {
				return tbl
			}
		}
	}
	return ""
}

// aiDetectFraudIntent busca patrones de fraude o delito fiscal en el texto que
// la IA propone escribir. Es una barrera adicional sobre el contenido, distinta
// de la de las tablas: aquí lo que se juzga es la INTENCIÓN.
//
// No pretende detectar todo el fraude (eso es del humano y del SENIAT); pretende
// que un agente no pueda, por accidente o por prompt inyectado, redactar un
// asiento diseñado para evadir un tributo. Ante la duda se bloquea y se avisa.
func aiDetectFraudIntent(text string) (bool, string) {
	low := strings.ToLower(text)

	// Cada patrón es un CONJUNTO: se activa si aparecen TODAS sus marcas.
	// Usar conjuntos y no frases fijas es a propósito: el modelo escribe de
	// muchas formas ("dividir la factura", "partir la factura en dos"), y una
	// frase literal se escaparía con sólo cambiar dos palabras.
	patrones := []struct {
		marcas []string
		motivo string
	}{
		{[]string{"facturar", "retroactiv"}, "facturación retroactiva: la factura debe hacerse a la fecha real de la operación"},
		{[]string{"retroactiv"}, "operación retroactiva sin justificación fiscal"},
		{[]string{"disminuir", "base"}, "reducción de base imponible"},
		{[]string{"evadir"}, "evasión fiscal explícita"},
		{[]string{"evasion"}, "evasión fiscal explícita"},
		{[]string{"ocultar", "ingreso"}, "ocultamiento de ingresos"},
		{[]string{"no declarar"}, "sugerencia de no declarar"},
		{[]string{"sin factura"}, "operación sin respaldo fiscal"},
		{[]string{"sin respaldo"}, "asiento sin respaldo documental"},
		{[]string{"dividir", "factura"}, "fraccionamiento de facturas para cruzar umbrales"},
		{[]string{"partir", "factura"}, "fraccionamiento de facturas para cruzar umbrales"},
		{[]string{"fraccionar"}, "fraccionamiento de facturas para cruzar umbrales"},
		{[]string{"ajust", "impuesto"}, "ajuste manual del impuesto fuera del cálculo normal"},
		{[]string{"ajustar", "impuesto"}, "ajuste manual del impuesto fuera del cálculo normal"},
		{[]string{"redondear"}, "redondeo dirigido al resultado fiscal"},
		{[]string{"borrar", "factura"}, "borrado de factura: solo se anula con nota de crédito"},
		{[]string{"eliminar", "factura"}, "eliminación de factura: solo se anula con nota de crédito"},
		{[]string{"devolucion", "falsa"}, "devolución sin operación real"},
	}
	for _, p := range patrones {
		todas := true
		for _, m := range p.marcas {
			if !strings.Contains(low, m) {
				todas = false
				break
			}
		}
		if todas {
			return true, p.motivo
		}
	}
	return false, ""
}
