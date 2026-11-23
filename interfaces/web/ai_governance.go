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
//
// LA APROBACIÓN VA ANTES DEL EFECTO (F5 de la auditoría). Este archivo solía
// marcar las acciones críticas con NeedsHuman DESPUÉS de ejecutarlas: el agente
// movía el dinero y luego alguien lo «revisaba», que no es una aprobación sino un
// parte de daños. Ahora una acción crítica producida por un agente se registra
// como PROPUESTA PENDIENTE y no se ejecuta: la ejecuta un humano desde la corrida
// (ver aiPropuestaPendiente y aiTaskRunReview), y el humano tiene que tener el
// permiso real de la herramienta para poder aplicarla.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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

// aiTablasProhibidasEnSQL es LA lista negra dura: tablas que ningún SQL escrito
// para la IA —db.query, informe personalizado, plantilla guardada, preview— puede
// leer ni por asomo, sea cual sea el rol, el departamento o el nivel de quien
// ejecuta.
//
// POR QUÉ UNA SOLA LISTA (A.2 de la hoja de trabajo). Antes la norma estaba
// repartida en tres criterios que no coincidían: `aiForbiddenTables` (gobernanza,
// aplicada solo a los argumentos de herramienta que parecían SQL), el mapa de 7
// tablas de `validateReportSQLwithSchema` y la whitelist del catálogo. El resultado
// medido por la auditoría: un rol con «crear informe» guardaba
// `SELECT api_key FROM ai_providers` y lo exportaba, porque el camino de informes
// no consultaba ninguna de esas listas (F2). Aquí vive una sola norma, y la.applya
// la frontera única (ai_sql_scope.go:validarSQLInformes) por dondequiera que salga
// SQL.
//
// Qué entra y por qué:
//   - Identidad y credenciales: users, user_login, sec_password, api_keys (las
//     claves del POS), web_sessions (revocación de sesiones), y las tablas de
//     RBAC, porque conocer la matriz de permisos es el mapa del ataque.
//   - Auditoría: audit_log, audit_chain_blocks. Nadie necesita un informe sobre la
//     propia cadena de prueba; y si alguien puede leerla, puede falsearla.
//   - Internos de la IA: proveedores (api_key en claro), ajustes, agentes, tareas,
//     corridas, idempotencia, propuestas, caché. Son el sistema, no el negocio.
//   - Infraestructura de datos: schema_migrations, queue_tasks.
//   - Configuración contable/fiscal: acc_predefine_account (qué cuenta es cada
//     cosa), y las fiscales que aporta aiTablasFiscalesDeInforme.
//
// Qué NO entra, a propósito: `company` (dato maestro de negocio que los informes
// citan para el nombre fiscal: su escritura ya la impide el guardián SELECT), las
// vistas de reporting y las tablas de negocio del catálogo, que se acotan por
// departamento, no por lista negra.
var aiTablasProhibidasEnSQL = func() map[string]bool {
	negras := []string{
		// Identidad, credenciales y sesiones.
		"users", "user_login", "sec_password", "api_keys", "pos_api_keys", "web_sessions",
		// RBAC: la matriz que dice quién puede qué.
		"sec_userrole", "sec_role", "role_permission", "user_department",
		// Auditoría y su cadena de prueba.
		"audit_log", "audit_chain_blocks",
		// Internos del subsistema IA.
		"ai_providers", "ai_provider", "ai_setting", "ai_agent", "ai_task", "ai_task_run",
		"ai_tool_execution", "ai_tool_proposal", "ai_run_slot", "ai_cache", "ai_operations",
		"ai_op_template", "ai_usage", "ai_report_types", "ai_department_prompt", "ai_legal_rule",
		// Infraestructura de datos y cola.
		"schema_migrations", "queue_tasks", "tenants", "tenant_users", "tenant_provision_log",
		// Configuración contable: no es dato de negocio, es el candado del plan.
		"acc_predefine_account",
	}
	out := make(map[string]bool, len(negras)+8)
	for _, t := range negras {
		out[strings.ToLower(strings.TrimSpace(t))] = true
	}
	// Las fiscales/contables salen de la lista canónica de informes, que vive en
	// ai_reports.go porque el generador de fine-tuning la lee de allí
	// (ia_dev/finetune). Se incorporan aquí: una sola norma efectiva, dos listas
	// que no se desincronizan porque una se construye de la otra.
	for _, t := range aiTablasFiscalesDeInforme() {
		out[strings.ToLower(strings.TrimSpace(t))] = true
	}
	return out
}()

// aiListaNegraOrdenada devuelve la lista negra en orden alfabético estable.
//
// Iterar el mapa directamente daría un orden distinto entre ejecuciones, y el
// nombre que sale en el mensaje de bloqueo tiene que ser siempre el mismo para el
// mismo SQL: un motivo que cambia no se puede ni probar ni explicar.
func aiListaNegraOrdenada() []string {
	out := make([]string, 0, len(aiTablasProhibidasEnSQL))
	for t := range aiTablasProhibidasEnSQL {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// aiForbiddenWriteTables nunca admiten escritura desde un agente, sin importar
// el rol ni el prompt. La contabilidad y la auditoría son intocables por IA:
// un agente puede leer un balance, pero jamás escribir en él.
var aiForbiddenWriteTables = []string{
	"acc_transaction", "acc_vaucher", "acc_coa", "acc_subcode", "acc_subtype",
	"audit_log", "audit_chain_blocks",
	"fiscal_setting", "fiscal_series", "tax_config", "retention",
	"pos_fiscal_blocks",
	"acc_predefine_account",
}

// GovernanceDecision es el veredicto del guardián para una acción.
type GovernanceDecision struct {
	Allowed bool
	Risk    RiskLevel
	Reason  string
	// Rule dice QUÉ regla decidió, para que el bloqueo sea diagnosticable y el
	// agente sepa que no debe reintentar: catálogo, tablas, permisos, operador o
	// nivel-1. Antes todos los bloqueos se reportaban como "nivel-1", así que un
	// permiso faltante y una tabla prohibida se leían igual.
	Rule string
	// NeedsHuman indica que la acción, además de permitirse, exige revisión
	// humana. Se persiste con la corrida (ver ai_task_run.needs_human).
	NeedsHuman bool
	// RequiereAprobacionPrevia es la regla que cierra F5: la acción está permitida
	// por riesgo y permisos, pero NO se ejecuta hasta que un humano la apruebe. El
	// dispatcher la convierte en propuesta pendiente (aiPropuestaPendiente) en vez
	// de llamar a Run.
	//
	// Vive en el veredicto y no en un `if riesgo == RiskCritical` suelto dentro del
	// dispatcher por la misma razón que el resto del Nivel 1: si la regla se
	// dedujera en cada llamador, bastaría con que uno nuevo lo olvidara para
	// reabrir el agujero.
	RequiereAprobacionPrevia bool
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
//
// gate son los permisos del rol del agente, resueltos una vez por corrida. Se
// pasan como valor y no se consultan aquí para que el guardián siga siendo una
// función comprobable sin base de datos.
func aiGuardAction(toolName string, args map[string]interface{}, operatorEnabled bool, gate aiRoleGate) GovernanceDecision {
	tool, known := aiToolByName[toolName]
	if !known {
		// Herramienta no declarada: se rechaza por omisión. Una whitelist es
		// segura; una blacklist obliga a perseguir cada caso nuevo.
		return GovernanceDecision{
			Risk:   RiskCritical,
			Rule:   "catalogo",
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
			Rule:   "tablas",
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
			Rule:   "operador",
			Reason: "modo operador desactivado para este agente",
		}
	}

	// PERMISOS DEL ROL. Es el hueco que quedaba abierto: el modo operador se
	// comprobaba, y el permiso concreto del rol no. Un agente con operador activo
	// podía llamar voucher.create o payment.record aunque su rol no tuviera ese
	// permiso en la interfaz, cuando los documentos del módulo prometen
	// "operator_enabled Y permisos CRUD del rol".
	//
	// Solo se exige a las herramientas que declaran Permission (las que escriben).
	// Las lecturas ya están acotadas por el departamento del rol en el guardián de
	// SQL; doblarlas aquí no añade seguridad y rompería el diseñador de informes,
	// que usa al agente por defecto para LEER.
	if !gate.allowsTool(tool) {
		return GovernanceDecision{
			Risk:   tool.Risk,
			Rule:   "permisos",
			Reason: gate.explainPermission(tool),
		}
	}

	return GovernanceDecision{
		Allowed: true,
		Risk:    tool.Risk,
		Rule:    "nivel-1",
		// NeedsHuman sigue existiendo para las corridas: es la marca que fuerza la
		// revisión de lo que el agente HIZO (borradores, asientos de execute).
		NeedsHuman: tool.Risk == RiskCritical,
		// Y aquí nace la regla nueva: lo crítico no se hace, se propone. Solo si
		// está implementado (Run != nil): una herramienta declarada y sin
		// implementar no puede quedarse «pendiente de aprobación» de algo que
		// nadie puede ejecutar.
		RequiereAprobacionPrevia: tool.Risk == RiskCritical && tool.Run != nil,
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
//
// Dos lecturas del argumento, y las dos se aplican:
//
//  1. Si el argumento es SQL (clave con «sql»), se EXTRAEN las tablas con el
//     escáner de la frontera (aiTablasEnSQL) y se comparan con nombre exacto. El
//     criterio antiguo era `strings.Contains` sobre el texto: «retention» casaba
//     con la tabla legal `vat_retention` (falso positivo) y `user_login` escrito
//     dentro de un literal era delito cuando era dato (incoherencia). Extraer y
//     comparar nombres es el mismo criterio que aplica la frontera de informes,
//     así que guardián e informes no pueden discrepar.
//  2. Si el argumento es una tabla (clave con «table»), se compara el valor tal
//     cual.
//
// La pasada por palabras sobre el texto se conserva además de la extracción: es la
// red para los casos que el escáner aún no posiciona, y no quita permisos nuevos.
func forbiddenTarget(args map[string]interface{}) string {
	prohibidas := aiTablasProhibidasEnSQL
	extra := make([]string, 0, len(aiForbiddenWriteTables))
	for _, t := range aiForbiddenWriteTables {
		if !prohibidas[strings.ToLower(t)] {
			extra = append(extra, strings.ToLower(t))
		}
	}
	for k, v := range args {
		key := strings.ToLower(k)
		if !strings.Contains(key, "table") && !strings.Contains(key, "sql") {
			continue
		}
		texto := fmt.Sprintf("%v", v)
		// La pasada por palabras corre sobre el texto SIN literales ni comentarios:
		// `WHERE nota = 'user_login'` es un DATO que menciona una tabla, no una
		// lectura de ella. El criterio antiguo (`strings.Contains` sobre el crudo)
		// bloqueaba los dos y, como castigo al falso positivo, educaba al usuario a
		// desconfiar del guardián.
		low := strings.ToLower(aiStripSQLCommentsAndLiterals(texto))
		if strings.Contains(key, "table") {
			// Un argumento «table» ES un identificador: se compara tal cual, sin
			// limpiar, porque ahí no hay SQL que analizar.
			if tab := strings.TrimSpace(strings.ToLower(texto)); prohibidas[tab] {
				return tab
			}
		}
		if strings.Contains(key, "sql") {
			for _, t := range aiTablasEnSQL(texto) {
				if prohibidas[t] {
					return t
				}
			}
			// Orden determinista para el mismo texto: el mapa no garantiza orden y el
			// motivo del bloqueo no puede cambiar entre ejecuciones.
			for _, t := range aiListaNegraOrdenada() {
				if aiContainsKeyword(low, t) {
					return t
				}
			}
			for _, t := range extra {
				if aiContainsKeyword(low, t) {
					return t
				}
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

// validateReportSQL es la función ESTRICTA de tablas para el SQL de informes.
//
// La auditoría la encontró muerta: existía, con whitelist, y nadie la llamaba
// (S-4/F2). Desde aquí es la capa de lectura de la frontera única
// (ai_sql_scope.go:validarSQLInformes), que la llama sobre la cadena YA
// NORMALIZADA —el texto que se ejecuta— y con la whitelist real del catálogo
// (aiCatalogWhitelist vía aiTablasPermitidasParaDept) y el escáner de tablas
// (aiTablasEnSQL), no con el regex `from\s+(\w+)` que solo veía la primera.
//
// Criterios, en orden de severidad:
//  1. Lista negra dura (aiTablasProhibidasEnSQL): por nombre de tabla y por
//     mención en el texto utilizable. Aplica a TODOS, incluido IT/master.
//  2. Alcance del departamento que ejecuta: solo para departamentos de negocio
//     (ver el POR QUÉ en aiValidarScopeSQL).
//
// Ya no está la heurística de «palabras clave fiscales» (impuesto, igv,
// retención) que tenía la versión anterior: bloqueaba informes venezolanos
// legítimos por el nombre de una columna, no por la tabla a la que se lee, y esa
// no es una regla de acceso. La protección fiscal real es la lista negra de
// tablas de configuración.
func validateReportSQL(sql string, dept string) *GovernanceDecision {
	if strings.TrimSpace(sql) == "" {
		return &GovernanceDecision{Risk: RiskRead, Rule: "vacio", Reason: "el SQL del informe está vacío"}
	}

	// 1. Lista negra, por las dos vías.
	stripped := aiStripSQLCommentsAndLiterals(sql)
	if t := aiMencionaTablaProhibida(stripped); t != "" {
		return &GovernanceDecision{
			Risk: RiskCritical,
			Rule: "tablas-prohibidas",
			Reason: fmt.Sprintf("la tabla «%s» está en la lista negra del sistema (credenciales, identidad, sesiones, "+
				"auditoría, internos de la IA o configuración contable): ningún informe puede leerla, sea quien sea el que lo ejecuta. "+
				"Si un informe guardado la usaba, queda no ejecutable y hay que reescribir su SQL con tablas de negocio", t),
		}
	}
	for _, t := range aiTablasEnSQL(sql) {
		if aiTablaProhibida(t) {
			return &GovernanceDecision{
				Risk:   RiskCritical,
				Rule:   "tablas-prohibidas",
				Reason: fmt.Sprintf("la tabla «%s» está en la lista negra del sistema y no puede leerse desde un informe", t),
			}
		}
	}

	// 2. Alcance del que ejecuta.
	if err := aiValidarScopeSQL(sql, dept); err != nil {
		return &GovernanceDecision{
			Risk:   RiskExecute,
			Rule:   "tabla-fuera-de-alcance",
			Reason: err.Error(),
		}
	}

	return &GovernanceDecision{
		Allowed: true,
		Risk:    RiskRead,
		Rule:    "ok",
		Reason:  "SQL de informe pasa la lista negra y el alcance del departamento que ejecuta",
	}
}

// ---------- Aprobación previa: la propuesta pendiente (F5) ----------

// aiPropuestaPendiente es una acción crítica que el agente pidió y el sistema NO
// ejecutó. Es el estado intermedio que faltaba: entre «el modelo lo pidió» y «el
// documento existe» ahora hay un registro que un humano aprueba o rechaza.
//
// Se copia el patrón que ya funcionaba en las propuestas de payroll/post_voucher
// (herramientas que devuelven applied=false): la IA propone, el humano dispone, y
// el que dispone queda en la auditoría. La diferencia es que aquí la propuesta SÍ
// es ejecutable con un clic, porque guarda los argumentos exactos con los que el
// agente la pidió.
type aiPropuestaPendiente struct {
	ID        int64
	RunID     int64
	TaskID    int64
	AgentID   int64
	Tool      string
	ArgsJSON  string
	IdemKey   string
	Status    string
	Reason    string
	Requested string
	AppliedBy string
}

// Estados de la propuesta. 'applying' es el CLAIM antes del efecto: quien pasa de
// 'pending' a 'applying' es el único que puede ejecutar, así que dos clics (o dos
// personas) sobre la misma propuesta no generan dos cobros.
//
// Son cadenas distintas de las del libro de idempotencia ('claimed'/'done'/
// 'error'/'undone' en ai_tool_execution) a propósito: son dos preguntas distintas
// —«¿ya se hizo?» y «¿quién lo autoriza?»— y compartirlas confundiría la
// auditoría.
const (
	aiEstadoPropuestaPendiente = "pending"
	aiEstadoPropuestaAplicando = "applying"
	aiEstadoPropuestaAplicada  = "applied"
	aiEstadoPropuestaRechazada = "rejected"
	aiEstadoPropuestaFallo     = "failed"
)

// aiMotivoAprobacionPrevia es el texto que recibe el MODELO. Va en el JSON de
// respuesta porque el agente tiene que entender que no falló nada: la acción se
// registró y quedó esperando a una persona, y reintentarla no la acelera.
//
// Dice el ACTO que falta («aprobación humana»), no solo el estado: un modelo al
// que se le devuelve «pendiente» suelto tiende a reintentar la llamada, y cada
// reintento es tokens y una propuesta duplicada en la pantalla del humano.
func aiMotivoAprobacionPrevia(tool string) string {
	return "la acción '" + tool + "' es de riesgo crítico y NO la ejecuta un agente: quedó registrada como " +
		"propuesta pendiente de aprobación humana en la corrida. Un humano con el permiso real de la herramienta " +
		"debe aplicarla desde Tareas → última ejecución → Revisar. No la repitas: la propuesta ya está hecha."
}

// aiRegistrarPropuesta guarda la propuesta con clave de idempotencia.
//
// FALLA CERRADO si el registro no se puede escribir: sin propuesta registrada no
// hay nada que aprobar, y ejecutar de todos modos sería volver al estado anterior
// a F5 (el agente mueve dinero y nadie lo autoriza). El agente recibe el error y
// lo reporta.
func (s *Server) aiRegistrarPropuesta(db *sql.DB, j *aiRunJournal, tool, argsJSON, idemKey, motivo, requestedBy string) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("no hay base de datos donde registrar la propuesta")
	}
	if idemKey == "" {
		return 0, fmt.Errorf("la propuesta necesita su clave de idempotencia")
	}
	var runID, taskID, agentID int64
	if j != nil {
		runID, taskID, agentID = j.runID, j.taskID, j.agentID
	}
	res, err := db.Exec(`INSERT INTO ai_tool_proposal
		(idem_key, tool, args_json, run_id, task_id, agent_id, status, reason, requested_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		idemKey, tool, argsJSON, runID, taskID, agentID, aiEstadoPropuestaPendiente, motivo, requestedBy, now(), now())
	if err != nil {
		if aiEsClaveDuplicada(err) {
			// La misma llamada ya estaba propuesta (reintento de la cola, segunda
			// vuelta del ticker): no se duplica la propuesta NI el aviso.
			var id int64
			_ = db.QueryRow(`SELECT id FROM ai_tool_proposal WHERE idem_key = ?`, idemKey).Scan(&id)
			return id, nil
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// aiReclamarPropuesta pasa una propuesta de 'pending' a 'applying' en UNA sola
// sentencia. Es el claim antes del efecto (la misma idea que ai_run_slot y que el
// libro de idempotencia): no hay ventana entre leer el estado y escribirlo.
func (s *Server) aiReclamarPropuesta(db *sql.DB, id int64) (aiPropuestaPendiente, bool, error) {
	var p aiPropuestaPendiente
	var args, reason, requested, applied sql.NullString
	if err := db.QueryRow(`SELECT id, tool, IFNULL(args_json,''), idem_key, run_id, task_id, agent_id, status,
		IFNULL(reason,''), IFNULL(requested_by,''), IFNULL(applied_by,'')
		FROM ai_tool_proposal WHERE id = ?`, id).
		Scan(&p.ID, &p.Tool, &args, &p.IdemKey, &p.RunID, &p.TaskID, &p.AgentID, &p.Status,
			&reason, &requested, &applied); err != nil {
		return p, false, err
	}
	p.ArgsJSON, p.Reason, p.Requested, p.AppliedBy = args.String, reason.String, requested.String, applied.String
	if p.Status != aiEstadoPropuestaPendiente {
		return p, false, nil
	}
	res, err := db.Exec(`UPDATE ai_tool_proposal SET status=?, updated_at=? WHERE id=? AND status=?`,
		aiEstadoPropuestaAplicando, now(), id, aiEstadoPropuestaPendiente)
	if err != nil {
		return p, false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return p, false, nil // alguien se adelantó: no se ejecuta dos veces
	}
	p.Status = aiEstadoPropuestaAplicando
	return p, true, nil
}

// aiCerrarPropuesta deja el estado final de la propuesta con su resultado.
func (s *Server) aiCerrarPropuesta(db *sql.DB, id int64, status, result, appliedBy string) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`UPDATE ai_tool_proposal SET status=?, result_json=?, applied_by=?, applied_at=?, updated_at=?
		WHERE id=?`, status, result, appliedBy, now(), now(), id)
}

// aiPropuestaPendienteDeCorrida mira UNA propuesta sin tocarla: dice la herramienta
// que propone y si sigue pendiente. Se necesita ANTES de retirar, porque el retiro
// exige comprobar el permiso del que retira y ese permiso está declarado en la
// herramienta, no en la fila.
func (s *Server) aiPropuestaPendienteDeCorrida(db *sql.DB, runID, propID int64) (herramienta string, encontrada bool) {
	if db == nil {
		return "", false
	}
	var estado string
	if err := db.QueryRow(`SELECT tool, status FROM ai_tool_proposal WHERE id = ? AND run_id = ?`,
		propID, runID).Scan(&herramienta, &estado); err != nil {
		return "", false
	}
	if estado != aiEstadoPropuestaPendiente {
		return "", false
	}
	return herramienta, true
}

// aiRechazarPropuesta cierra una propuesta SIN ejecutarla, en una sola sentencia
// condicionada al estado: si entre la comprobación del permiso y aquí alguien la
// aplicó, RowsAffected es 0 y no se pisa su decisión. Ese es el mismo claim atómico
// que usa la aprobación, en la otra dirección.
func (s *Server) aiRechazarPropuesta(db *sql.DB, propID int64, actor string) bool {
	if db == nil {
		return false
	}
	res, err := db.Exec(`UPDATE ai_tool_proposal
		SET status=?, reason=?, applied_by=?, applied_at=?, updated_at=?
		WHERE id=? AND status=?`,
		aiEstadoPropuestaRechazada, "retirada sin ejecutar por "+actor, actor, now(), now(),
		propID, aiEstadoPropuestaPendiente)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// aiPropuestasDeCorrida lista las propuestas de una corrida (la pantalla de
// revisión las muestra y el botón las aplica).
func (s *Server) aiPropuestasDeCorrida(db *sql.DB, runID int64) []aiPropuestaPendiente {
	out := []aiPropuestaPendiente{}
	if db == nil {
		return out
	}
	rows, err := db.Query(`SELECT id, tool, status, IFNULL(requested_by,''), IFNULL(applied_by,''), IFNULL(reason,'')
		FROM ai_tool_proposal WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		// Tabla ausente (migración sin aplicar): la revisión responde sin
		// propuestas en vez de un 500 — pero la corrida crítica sí quedó marcada
		// needs_human, que es lo que el humano ve en la lista.
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p aiPropuestaPendiente
		if err := rows.Scan(&p.ID, &p.Tool, &p.Status, &p.Requested, &p.AppliedBy, &p.Reason); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// aiAplicarPropuesta ejecuta la herramienta guardada con el HUMANO como actor.
//
// Tres cosas garantizan que «aprobar» no sea un agujero nuevo:
//  1. El permiso real del humano (lo comprueba el llamador con exigirPermiso sobre
//     la declaración de la herramienta; aquí se vuelve a comprobar con la misma
//     función del dispatcher, porque una propuesta aplicada desde una ruta que no
//     lo hiciera no puede acabar escribiendo).
//  2. La identidad que queda en el documento es la del humano: se le pasa su
//     sesión y un agente con su user_id, así que `sales_by`, el asiento y la
//     auditoría dicen quién lo hizo, no «el agente».
//  3. El claim 'pending'→'applying' ya ocurrió: esta función se llama con la
//     propuesta reclamada y ninguna otra puede ejecutarla.
//
// El journal que recibe la herramienta lleva el ámbito de la corrida ORIGINAL de
// la propuesta, para que la clave de idempotencia coincida con la que se calculó
// al proponer: si el agente volvió a pedir lo mismo, el humano aplica UNA vez.
func (s *Server) aiAplicarPropuesta(sess *SessionData, humano aiAgent, prop aiPropuestaPendiente, journal *aiRunJournal) (string, error) {
	tool, ok := aiToolByName[prop.Tool]
	if !ok || tool.Run == nil {
		return "", fmt.Errorf("la herramienta '%s' ya no está disponible: la propuesta no se puede aplicar", prop.Tool)
	}
	// Permiso del HUMANO con la MISMA regla y la MISMA acción que declara la
	// herramienta. No se reutiliza aiPermisoDenegado: esa función comprueba
	// siempre «create», y una propuesta de payment.record declara «update» — un
	// rol con permiso de actualización legítimo habría quedado negado por una
	// regla que no es la suya. gate.allowsTool es la declaración leída tal cual.
	gate := s.aiBuildRoleGate(sess, humano)
	if !gate.allowsTool(tool) {
		return "", fmt.Errorf("permiso denegado: %s", gate.explainPermission(tool))
	}
	args := map[string]interface{}{}
	if prop.ArgsJSON != "" {
		if err := json.Unmarshal([]byte(prop.ArgsJSON), &args); err != nil {
			return "", fmt.Errorf("los argumentos guardados de la propuesta no se pueden leer: %w", err)
		}
	}
	return tool.Run(s, sess, humano, args, journal)
}
