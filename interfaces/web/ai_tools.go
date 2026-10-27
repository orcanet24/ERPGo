package web

// ai_tools.go — Catálogo de herramientas (estilo MCP interno): el modelo
// propone llamadas; el SERVIDOR valida contra el rol del agente y ejecuta.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// aiToolSchema es la definición de una herramienta para el proveedor.
type aiToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// aiParameterSchema reduce el ruido de las definiciones.
//
// `required` SIEMPRE es un array, aunque esté vacío. Un `required` ausente sale
// como `null` en el JSON, y un servidor estricto de herramientas lo rechaza
// entero: "Invalid JSON schema for field tools.function.parameters: None is not
// of type 'array'". Medido contra ORC24. El array vacío dice lo mismo que no
// decirlo, y no rompe nada.
func aiParameterSchema(props map[string]interface{}, required ...string) map[string]interface{} {
	if required == nil {
		required = []string{}
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// aiStrParam lee un parámetro string del payload de la herramienta.
//
// Acepta MÁS que cadenas, y no por generosidad sino por lo que hacen los
// modelos. Cuando el esquema dice "JSON de definición: {...}", una parte
// importante de los modelos manda un objeto JSON en vez de su texto. Con la
// versión estricta (solo `string`), `report.save_template` contestaba "Nombre y
// definition son obligatorios" con la definición perfectamente enviada, y no
// había forma de distinguir "el modelo se equivocó" de "el ERP no leyó el
// argumento".
//
// Un objeto o una lista se serializan a su texto JSON, que es exactamente lo que
// el parámetro significa. Un número o un booleano se pasan a texto también, que
// es lo que espera el resto del producto.
func aiStrParam(args map[string]interface{}, key string) string {
	switch v := args[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ""
}

// aiNumParam lee un parámetro numérico del payload.
func aiNumParam(args map[string]interface{}, key string) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case string:
		return parseMoney(v)
	}
	return 0
}

// aiToolDef es la definición ÚNICA de una herramienta: lo que el modelo ve
// (nombre, descripción, parámetros) y lo que el servidor ejecuta (Run), más su
// nivel de riesgo para el guardián del Nivel 1.
//
// Antes esto vivía en tres sitios —el catálogo que se le manda al modelo, el
// mapa de riesgos y el switch del dispatcher— y la duplicación ya costó un bug
// real: se añadió payment.record al catálogo y al dispatcher, se olvidó en el mapa
// de riesgos, y el guardián la bloqueó en producción. Un solo registro elimina
// esa clase de bug por construcción.
type aiToolDef struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Risk        RiskLevel

	// OperatorOnly marca las herramientas que no se anuncian al modelo cuando el
	// agente no tiene modo operador, y que el guardián rechaza si se invocan
	// igual. Las lecturas nunca lo llevan.
	OperatorOnly bool

	// Permission es el permiso del rol que exige la herramienta. Vive aquí, junto
	// al riesgo, por la misma razón que el riesgo: cuando la clasificación estaba
	// en un segundo sitio, una herramienta nueva quedó sin clasificar y el guardián
	// la bloqueó en producción. Una sola lista no puede desincronizarse de sí
	// misma. Las que escriben lo declaran; las lecturas y las propuestas no,
	// porque su puerta es otra (departamento del rol / revisión humana).
	//
	// Directorio Y acción: la misma tabla que limita a un empleado en la interfaz
	// (sub_module.directory → role_permission). Es el «Nivel 2» del diseño: el
	// interruptor de operador dice «este agente puede operar»; este campo dice
	// «puede operar ESTO». La acción importa —crear un comprobante y anularlo no
	// son el mismo permiso— y por eso no basta con declarar el directorio.
	Permission aiToolPermission

	// Run ejecuta la herramienta. nil significa "declarada pero todavía sin
	// implementar": el Nivel 1 sí la clasifica, pero el dispatcher responde que no
	// está disponible. Declarar antes de implementar es lo que permite que la
	// gobernanza exista para herramientas futuras sin abrirle la puerta antes.
	Run func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error)
}

// aiToolRegistry es la fuente de verdad del catálogo de herramientas. Agregar
// una herramienta aquí es el ÚNICO paso necesario: el modelo la ve, el guardián
// la conoce y el dispatcher la ejecuta.
var aiToolRegistry = []aiToolDef{
	// ---------- Lectura ----------
	{
		Name:        "db.query",
		Description: "Ejecuta una consulta SELECT de solo lectura sobre las tablas de negocio y devuelve filas JSON (máximo limit filas).",
		Risk:        RiskRead,
		Parameters: aiParameterSchema(map[string]interface{}{
			"sql":   map[string]string{"type": "string", "description": "Una sola sentencia SELECT (o WITH...SELECT). Sin punto y coma final."},
			"limit": map[string]string{"type": "integer", "description": "Máximo de filas a devolver (opcional, default 100, máximo 500)."},
		}, "sql"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolDBQuery(sess, agent, args)
		},
	},
	{
		Name:        "report.list_templates",
		Description: "Lista las plantillas de informes existentes (id, nombre, descripción).",
		Risk:        RiskRead,
		Parameters:  aiParameterSchema(map[string]interface{}{}),
		Run: func(s *Server, sess *SessionData, _ aiAgent, _ map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolListTemplates(sess)
		},
	},
	{
		Name:        "report.save_template",
		Description: "Crea una plantilla de informe reutilizable a partir de un JSON de definición. Soporta gráfico embebidos con el campo chart (type, x_key, y_key, y_label, title).",
		Risk:        RiskDraft,
		Permission:  aiToolPermission{Directory: "custom_reports", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"name":        map[string]string{"type": "string", "description": "Nombre visible del informe."},
			"description": map[string]string{"type": "string", "description": "Descripción breve para el menú."},
			"definition":  map[string]string{"type": "string", "description": "JSON de definición: {title,sql,columns:[{key,label}],totals:[keys],params:[{name,label,type}],chart:{type,line|bar|pie|doughnut),x_key,y_key,y_label,title}}. SOLO SELECT. Tablas fiscales (acc_transaction, audit_log) son solo lectura."},
		}, "name", "definition"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolSaveTemplate(sess, agent, args, journal)
		},
	},
	{
		Name:        "report.run_preview",
		Description: "Ejecuta una definición de informe en borrador y devuelve headers/filas/chart_data para validarla antes de guardarla. Útil para verificar el gráfico.",
		Risk:        RiskRead,
		Parameters: aiParameterSchema(map[string]interface{}{
			"definition": map[string]string{"type": "string", "description": "JSON de definición igual que report.save_template."},
		}, "definition"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolRunPreview(sess, agent, args)
		},
	},
	{
		Name:        "data.summary",
		Description: "Resumen agregado del negocio (ventas/compras/stock/devoluciones) para un rango de fechas, sin escribir SQL.",
		Risk:        RiskRead,
		Parameters: aiParameterSchema(map[string]interface{}{
			"date_from": map[string]string{"type": "string", "description": "Fecha inicial AAAA-MM-DD."}, "date_to": map[string]string{"type": "string", "description": "Fecha final AAAA-MM-DD."},
		}, "date_from", "date_to"),
		Run: func(s *Server, sess *SessionData, _ aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			from := aiStrParam(args, "date_from")
			to := aiStrParam(args, "date_to")
			if !aiValidDate(from) || !aiValidDate(to) {
				return string(mustJSON(aiToolError{"Fechas inválidas (use AAAA-MM-DD)."})), nil
			}
			agg, err := s.aiAggregatesForRange(sess, from, to)
			if err != nil {
				return "", err
			}
			return string(mustJSON(agg)), nil
		},
	},

	// ---------- Borrador: se borra sin dejar rastro contable ----------
	{
		Name:         "invoice.create_draft",
		Description:  "Crea una factura en BORRADOR (sin efecto contable hasta publicarse). Devuelve el número de borrador para publicar luego.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "new_invoice", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"external_ref": map[string]string{"type": "string", "description": "Identificador del documento de origen (número de listado, correo, orden de compra). Úsalo SIEMPRE que la factura nazca de un listado: es lo que impide cargarla dos veces."},
			"customer_id":  map[string]string{"type": "integer", "description": "ID del cliente."},
			"date":         map[string]string{"type": "string", "description": "Fecha AAAA-MM-DD."},
			"items":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"product_id": map[string]string{"type": "string"}, "quantity": map[string]string{"type": "number"}, "rate": map[string]string{"type": "number"}}, "required": []string{"product_id"}}}}, "customer_id", "items"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolInvoiceDraft(sess, agent, args, journal)
		},
	},
	{
		Name:         "invoice.post",
		Description:  "Publica un borrador de factura creado con invoice.create_draft (genera asientos contables).",
		Risk:         RiskExecute,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "new_invoice", Action: "update"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"draft_id":     map[string]string{"type": "integer", "description": "ID del borrador."},
			"external_ref": map[string]string{"type": "string", "description": "Identificador del documento de origen, si la publicación corresponde a uno."},
		}, "draft_id"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolInvoicePost(sess, agent, args, journal)
		},
	},
	{
		Name:         "payment.record",
		Description:  "Registra el cobro de una factura pendiente: abona paid_amount, reduce due_amount y crea el asiento contable del cobro.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "manage_invoice", Action: "update"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"external_ref": map[string]string{"type": "string", "description": "Referencia del cobro en su origen (número de transferencia, recibo, aviso del banco). Si se repite, el cobro no se duplica."},
			"invoice_id":   map[string]string{"type": "integer", "description": "Número de factura."},
			"amount":       map[string]string{"type": "number", "description": "Monto cobrado."},
			"note":         map[string]string{"type": "string", "description": "Referencia/observación del cobro."},
		}, "invoice_id", "amount"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolPaymentRecord(sess, agent, args, journal)
		},
	},
	{
		Name:         "voucher.create",
		Description:  "Crea un voucher contable (asiento de diario) con líneas débito/crédito balanceadas.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "vouchers", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"external_ref": map[string]string{"type": "string", "description": "Referencia del documento de origen (número de comprobante, acta, factura externa). Si se repite, el asiento no se duplica."},
			"vtype":        map[string]string{"type": "string", "description": "Tipo: JV (diario), CV (contra), DV (diario de banco)."},
			"ref":          map[string]string{"type": "string", "description": "Referencia (opcional)."},
			"note":         map[string]string{"type": "string", "description": "Narración del asiento."},
			"lines":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"coa_id": map[string]string{"type": "integer"}, "debit": map[string]string{"type": "number"}, "credit": map[string]string{"type": "number"}}}},
		}, "vtype", "lines"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolVoucherCreate(sess, agent, args, journal)
		},
	},

	// ---------- Herramientas MCP ----------
	//
	// Registradas aquí, en la misma tabla que el resto, para que el guardián las
	// clasifique sin que nadie tenga que acordarse de hacerlo en otro archivo.
	{
		Name:        "describe_schema",
		Description: "Devuelve el mapa relacional de las tablas de negocio, o el detalle (columnas y relaciones) de una tabla concreta.",
		Risk:        RiskRead,
		Parameters:  aiParameterSchema(map[string]interface{}{"table": map[string]string{"type": "string", "description": "Nombre de la tabla. Si se omite, devuelve el mapa completo."}}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolDescribeSchema(sess, agent, args)
		},
	},
	{
		Name:        "legal_summary",
		Description: "Devuelve la normativa vigente que la empresa ha cargado, por tema y fecha. Úsala antes de citar una cifra legal: no improvises la norma.",
		Risk:        RiskRead,
		Parameters:  aiParameterSchema(map[string]interface{}{"topic": map[string]string{"type": "string", "description": "iva | islr | retencion_iva | retencion_islr | nomina | contabilidad | libros"}, "as_of": map[string]string{"type": "string", "description": "Fecha AAAA-MM-DD. Por omisión, hoy."}}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolLegalSummary(sess, agent, args)
		},
	},
	{
		Name:        "query_report",
		Description: "Ejecuta un informe por plantilla ya definido, con sus filtros. Prefiérelo a inventar SQL: hereda las validaciones y los totales del informe.",
		Risk:        RiskRead,
		Parameters:  aiParameterSchema(map[string]interface{}{"template": map[string]string{"type": "string", "description": "Nombre del informe. Si se omite, lista los disponibles."}, "params": map[string]interface{}{"type": "object", "description": "Valores de los filtros que declara el informe."}}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolQueryReport(sess, agent, args)
		},
	},

	// ---------- Propuestas: la IA sugiere, el humano ejecuta ----------
	//
	// El patrón común es que devuelven applied=false SIEMPRE. Convertir una
	// propuesta en una acción es un acto consciente de alguien, y ese alguien
	// queda en la auditoría. Por eso van separadas de las herramientas que sí
	// escriben: que una propuesta se ejecute nunca depende de un interruptor.
	{
		Name:         "account.propose_transaction",
		Description:  "Propone un asiento contable validado (cuentas existentes y cuadre) SIN aplicarlo. Devuelve la propuesta para que un humano la revise.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Parameters: aiParameterSchema(map[string]interface{}{
			"vtype": map[string]string{"type": "string", "description": "JV (diario), CV (contra), DV (diario de banco)."},
			"ref":   map[string]string{"type": "string", "description": "Referencia."},
			"note":  map[string]string{"type": "string", "description": "Narración del asiento."},
			"lines": map[string]interface{}{"type": "array", "description": "Líneas con coa_id, debit y credit."},
		}, "lines"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolProposeTransaction(sess, agent, args)
		},
	},
	{
		Name:         "invoice.propose",
		Description:  "Propone una factura con sus datos SIN registrarla. Avisa si el período contable está cerrado.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Parameters: aiParameterSchema(map[string]interface{}{
			"customer_id": map[string]string{"type": "integer", "description": "ID del cliente."},
			"date":        map[string]string{"type": "string", "description": "Fecha AAAA-MM-DD."},
			"items":       map[string]interface{}{"type": "array", "description": "Items con product_id, quantity y rate."},
			"note":        map[string]string{"type": "string", "description": "Observación."},
		}, "customer_id"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolProposeInvoice(sess, agent, args)
		},
	},
	{
		Name:         "payroll.propose",
		Description:  "Propone una corrida de nómina SIN calcular importes: la nómina la calcula el servidor con las tasas vigentes cuando alguien la lance.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Parameters:   aiParameterSchema(map[string]interface{}{"period": map[string]string{"type": "string", "description": "Período AAAAMM o AAAA-MM."}}, "period"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolProposePayroll(sess, agent, args)
		},
	},

	// ---------- Declaradas, pendientes de implementar ----------
	//
	// Existen ya en el catálogo de riesgos aunque todavía no se puedan ejecutar
	// (Run = nil). Declararlas por adelantado es lo que permite que el Nivel 1
	// las proteja desde el día en que se implementen, sin una ventana en la que
	// queden sin clasificar. No se anuncian al modelo (ver aiCatalogForPhase).
	//
	// supplier.upsert y product.upsert YA están implementadas (ver
	// ai_tools_compras.go) y por eso se anuncian al modelo: son las que permiten
	// que un agente cargue una compra de un proveedor y de artículos que todavía
	// no están en el sistema, igual que hace el operador desde el formulario.
	{
		Name:         "invoice.anull",
		Description:  "Anula una factura mediante nota de crédito.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "manage_invoice", Action: "delete"},
		Parameters:   aiParameterSchema(map[string]interface{}{"invoice_id": map[string]string{"type": "integer", "description": "Número de factura."}}, "invoice_id"),
	},
	{
		Name:         "account.post_voucher",
		Description:  "Postea un voucher contable ya balanceado.",
		Risk:         RiskExecute,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "vouchers", Action: "update"},
		Parameters:   aiParameterSchema(map[string]interface{}{"vno": map[string]string{"type": "string", "description": "Número de voucher."}}, "vno"),
	},
	{
		Name:         "customer.upsert",
		Description:  "Crea o actualiza un cliente.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "manage_customer", Action: "update"},
		Parameters:   aiParameterSchema(map[string]interface{}{"customer_id": map[string]string{"type": "integer"}, "name": map[string]string{"type": "string"}}, "name"),
	},
	{
		Name: "supplier.upsert",
		Description: "Crea un proveedor o devuelve el que ya existe (por RIF o nombre). " +
			"Úselo antes de registrar una compra de un proveedor que no está en el sistema.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "add_supplier", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"supplier_id": map[string]string{"type": "integer", "description": "ID del proveedor a actualizar. Omitir para crear."},
			"name":        map[string]string{"type": "string", "description": "Razón social del proveedor."},
			"rif":         map[string]string{"type": "string", "description": "RIF venezolano, ej. J-12345678-9 (obligatorio para el libro de compras)."},
			"mobile":      map[string]string{"type": "string", "description": "Teléfono."},
			"email":       map[string]string{"type": "string", "description": "Correo."},
			"address":     map[string]string{"type": "string", "description": "Dirección."},
			"city":        map[string]string{"type": "string", "description": "Ciudad."},
		}, "name"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolSupplierUpsert(sess, agent, args, journal)
		},
	},
	{
		Name: "product.upsert",
		Description: "Da de alta un producto en el catálogo o devuelve el que ya existe (por código de barras o " +
			"nombre+modelo). Úselo para los artículos de una compra que todavía no están cargados.",
		Risk:         RiskDraft,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "create_product", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"product_id":    map[string]string{"type": "string", "description": "ID del producto a actualizar. Omitir para crear."},
			"name":          map[string]string{"type": "string", "description": "Nombre del producto."},
			"product_model": map[string]string{"type": "string", "description": "Modelo o presentación."},
			"barcode":       map[string]string{"type": "string", "description": "Código de barras (EAN/Code128). Es la identidad del artículo."},
			"unit":          map[string]string{"type": "string", "description": "Unidad de medida."},
			"price":         map[string]string{"type": "number", "description": "Precio de venta."},
			"product_vat":   map[string]string{"type": "number", "description": "IVA en porcentaje."},
		}, "name"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolProductUpsert(sess, agent, args, journal)
		},
	},
	{
		Name: "purchase.create",
		Description: "Registra una compra completa —cabeza, renglones con lote y vencimiento, y su asiento contable— " +
			"por el mismo camino que el formulario del sistema. Los productos que falten se pueden dar de alta " +
			"en la misma llamada. Devuelve la alerta de variación de precio frente a la última compra. " +
			"CLASIFIQUE el documento con document_kind: solo 'factura' da crédito fiscal de IVA y obliga a " +
			"retenciones. El sistema tiene un CANDADO contra duplicados: si esa misma compra ya estaba " +
			"registrada (mismo proveedor, mismos renglones y mismos números de factura y control) NO se crea " +
			"otra y la respuesta trae status 'duplicate' con el número de la compra que ya existe. Antes de " +
			"cargar un listado, use purchase.find para ver qué ya está registrado.",
		Risk:         RiskExecute,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "add_purchase", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"external_ref":  map[string]string{"type": "string", "description": "Referencia de la compra en su origen (número de listado, correo, orden de compra). Si se repite, la compra no se duplica."},
			"supplier_id":   map[string]string{"type": "integer", "description": "ID del proveedor."},
			"supplier_name": map[string]string{"type": "string", "description": "Nombre del proveedor si no se conoce el ID: se busca y, si no existe, se crea."},
			"supplier_rif":  map[string]string{"type": "string", "description": "RIF del proveedor (necesario si hay que crearlo)."},
			"date":          map[string]string{"type": "string", "description": "Fecha de la compra AAAA-MM-DD. Por omisión, hoy."},
			"chalan_no":     map[string]string{"type": "string", "description": "N° de factura del proveedor."},
			"control_no":    map[string]string{"type": "string", "description": "N° de control fiscal."},
			"document_kind": map[string]string{"type": "string", "description": "Tipo de documento: 'factura' (factura del proveedor: es la ÚNICA con crédito fiscal de IVA y retenciones), 'nota_entrega' (entra mercancía y genera deuda, sin crédito fiscal), 'orden' (orden de compra) o 'sin_documento'. Por omisión 'factura'."},
			"details":       map[string]string{"type": "string", "description": "Observaciones."},
			"discount":      map[string]string{"type": "number", "description": "Descuento global de la compra."},
			"items":         map[string]interface{}{"type": "array", "description": "Renglones: product_id o barcode o name, quantity, rate, batch_no, expiry_date, discount_per, vat_per."},
			"payments":      map[string]interface{}{"type": "array", "description": "Pagos: method_id (payment_method.id) y amount. Sin pagos, la compra queda a crédito."},
		}, "items"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
			return s.aiToolPurchaseCreate(sess, agent, args, journal)
		},
	},
	{
		Name: "purchase.find",
		Description: "Busca compras YA registradas por proveedor, número de factura, número de control o fechas, y dice para " +
			"cada una su tipo de documento, sus saldos y si tiene asiento. Úsela ANTES de cargar un listado de compras: " +
			"ver que una compra ya está es la forma de no duplicarla.",
		Risk: RiskRead,
		Parameters: aiParameterSchema(map[string]interface{}{
			"supplier_id":   map[string]string{"type": "integer", "description": "ID del proveedor (opcional)."},
			"supplier_name": map[string]string{"type": "string", "description": "Nombre o parte del nombre del proveedor (opcional)."},
			"chalan_no":     map[string]string{"type": "string", "description": "N° de factura del proveedor (opcional)."},
			"control_no":    map[string]string{"type": "string", "description": "N° de control fiscal (opcional)."},
			"date_from":     map[string]string{"type": "string", "description": "Desde AAAA-MM-DD (opcional)."},
			"date_to":       map[string]string{"type": "string", "description": "Hasta AAAA-MM-DD (opcional)."},
			"limit":         map[string]string{"type": "integer", "description": "Máximo de filas (por omisión 25, máximo 100)."},
		}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolPurchaseFind(sess, agent, args)
		},
	},
	{
		Name:         "payment.create",
		Description:  "Registra un pago a proveedor o cobro sin factura asociada.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "manage_invoice", Action: "create"},
		Parameters:   aiParameterSchema(map[string]interface{}{"amount": map[string]string{"type": "number"}, "note": map[string]string{"type": "string"}}, "amount"),
	},
	{
		Name:         "payroll.run",
		Description:  "Calcula una corrida de nómina SIN publicarla (queda en borrador).",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "hr_payroll", Action: "create"},
		Parameters:   aiParameterSchema(map[string]interface{}{"period": map[string]string{"type": "string", "description": "Período AAAA-MM."}}, "period"),
	},
	{
		Name:         "payroll.post",
		Description:  "Publica una corrida de nómina: genera los recibos y los asientos contables.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "hr_payroll_post", Action: "create"},
		Parameters:   aiParameterSchema(map[string]interface{}{"run_id": map[string]string{"type": "integer", "description": "ID de la corrida."}}, "run_id"),
	},

	// ---------- Cartera: cobranza y cuentas por pagar ----------
	//
	// Estas herramientas son el trabajo REPETITIVO de cartera: ver quién se
	// atrasó, decidir si es anormal y avisar. Las dos que escriben hacia afuera
	// llevan RiskCritical a propósito: se ejecutan, quedan auditadas y la corrida
	// se marca para revisión humana (needs_human), porque son la única salida de
	// datos del sistema hacia un tercero.
	{
		Name: "finance.receivables",
		Description: "Cuentas por cobrar: saldos, antigüedad, indicadores (vencido, días de cobro, concentración) y la " +
			"lista de ALERTAS abiertas —la misma que ve el operador en pantalla—. Úsela para saber a quién hay que " +
			"reclamarle y por qué antes de escribirle a nadie.",
		Risk: RiskRead,
		Parameters: aiParameterSchema(map[string]interface{}{
			"limit":          map[string]string{"type": "integer", "description": "Máximo de alertas a devolver (por omisión 25, máximo 200)."},
			"include_closed": map[string]string{"type": "string", "description": "'1' para incluir también las alertas ya cerradas o resueltas."},
		}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolFinanceReceivables(sess, agent, args)
		},
	},
	{
		Name: "finance.payables",
		Description: "Cuentas por pagar: documentos y servicios con saldo, antigüedad, indicadores y alertas abiertas, " +
			"incluidos los comprobantes de retención de IVA que todavía no se le enviaron al proveedor.",
		Risk: RiskRead,
		Parameters: aiParameterSchema(map[string]interface{}{
			"limit": map[string]string{"type": "integer", "description": "Máximo de alertas a devolver (por omisión 25, máximo 200)."},
		}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolFinancePayables(sess, agent, args)
		},
	},
	{
		Name: "finance.scan_alerts",
		Description: "Recalcula las alertas de cartera y de cuentas por pagar sin esperar a que alguien abra la pantalla. " +
			"Cierra las que ya no se cumplen y abre las nuevas. Es idempotente: repetirlo no duplica alertas.",
		Risk:         RiskExecute,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "customer_ledger", Action: "read"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"scope": map[string]string{"type": "string", "description": "'cxc', 'cxp' o vacío para los dos."},
		}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolFinanceScan(sess, agent, args)
		},
	},
	{
		Name: "customer.request_payment",
		Description: "Le reclama el pago a un cliente por un canal de aviso configurado (correo, SMS o webhook). " +
			"El destinatario sale SIEMPRE de la ficha del cliente: no se puede indicar una dirección. El canal debe estar " +
			"habilitado para agentes por el administrador, y hay enfriamiento e idempotencia para no repetir el aviso. " +
			"Devuelve a quién se le mandó y si salió.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "customer_ledger", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"alert_id":     map[string]string{"type": "integer", "description": "ID de la alerta de finance.receivables. Es la vía recomendada: trae el motivo redactado con los números del sistema."},
			"customer_id":  map[string]string{"type": "integer", "description": "ID del cliente, si la cuenta no genera alerta."},
			"channel_kind": map[string]string{"type": "string", "description": "'email', 'sms' o 'webhook'. Opcional: por omisión, el primer canal habilitado para agentes."},
			"channel":      map[string]string{"type": "string", "description": "Nombre exacto del canal, si hay más de uno del mismo tipo."},
			"subject":      map[string]string{"type": "string", "description": "Asunto. Si se omite, el sistema lo redacta con los datos de la deuda."},
			"message":      map[string]string{"type": "string", "description": "Texto del aviso. Si se omite, el sistema lo redacta (recomendado: no invente cifras)."},
			"external_ref": map[string]string{"type": "string", "description": "Referencia del reclamo en su origen (gestión, llamada, correo recibido). Si se repite, el aviso no se duplica."},
		}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolCustomerRequestPayment(sess, agent, args)
		},
	},
	{
		Name: "supplier.send_retention",
		Description: "Le envía a un proveedor el resumen de sus comprobantes de retención de IVA de un período, por un canal " +
			"de aviso configurado. El destinatario sale de la ficha del proveedor. Al enviarlo, la alerta de retención " +
			"pendiente se cierra sola.",
		Risk:         RiskCritical,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "supplier_payment", Action: "create"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"supplier_id":  map[string]string{"type": "integer", "description": "ID del proveedor."},
			"purchase_id":  map[string]string{"type": "integer", "description": "Compra del proveedor, si no se conoce su ID."},
			"period":       map[string]string{"type": "string", "description": "Período AAAA-MM. Si se omite, se envía el último con retenciones registradas."},
			"channel_kind": map[string]string{"type": "string", "description": "'email', 'sms' o 'webhook'. Opcional."},
			"channel":      map[string]string{"type": "string", "description": "Nombre exacto del canal, si hay varios del mismo tipo."},
			"subject":      map[string]string{"type": "string", "description": "Asunto. Opcional."},
			"message":      map[string]string{"type": "string", "description": "Texto. Si se omite, el sistema lo redacta con los comprobantes reales."},
		}),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolSupplierSendRetention(sess, agent, args)
		},
	},
	{
		Name: "notify.event",
		Description: "Manda un evento a un webhook configurado, para que otra herramienta haga algo con él (crear una tarea de " +
			"cobranza, avisar por otra vía, dejar el registro en un tablero). El evento queda registrado en Mensajes enviados.",
		Risk:         RiskExecute,
		OperatorOnly: true,
		Permission:   aiToolPermission{Directory: "setting", Action: "read"},
		Parameters: aiParameterSchema(map[string]interface{}{
			"event":        map[string]string{"type": "string", "description": "Nombre del evento, ej. 'cobranza.mora_anormal'."},
			"data":         map[string]string{"type": "string", "description": "Carga útil en JSON (objeto)."},
			"channel":      map[string]string{"type": "string", "description": "Nombre del canal webhook habilitado para agentes. Opcional si solo hay uno."},
			"external_ref": map[string]string{"type": "string", "description": "Referencia del evento en su origen, para no entregarlo dos veces."},
		}, "event"),
		Run: func(s *Server, sess *SessionData, agent aiAgent, args map[string]interface{}, _ *aiRunJournal) (string, error) {
			return s.aiToolNotifyEvent(sess, agent, args)
		},
	},
}

// aiToolByName indexa el registro por nombre para la búsqueda del guardián y del
// dispatcher. Se construye en init porque un map literal duplicaría los nombres.
var aiToolByName = func() map[string]aiToolDef {
	m := make(map[string]aiToolDef, len(aiToolRegistry))
	for _, t := range aiToolRegistry {
		m[t.Name] = t
	}
	return m
}()

// aiCatalogForPhase arma el catálogo que se le anuncia al modelo, según la fase
// del agente. Se deriva del registro: no hay una segunda lista que mantener.
//
// No se anuncian las herramientas declaradas sin implementar (Run == nil):
// ofrecerle al modelo una herramienta que solo puede devolver error gasta
// tokens, confunde al agente y entrena al usuario a esperar algo que no pasa.
func aiCatalogForPhase(operator bool, gate aiRoleGate) []aiToolSchema {
	tools := make([]aiToolSchema, 0, len(aiToolRegistry))
	for _, t := range aiToolRegistry {
		if t.Run == nil {
			continue
		}
		if t.OperatorOnly && !operator {
			continue
		}
		// No se anuncia lo que el guardián va a rechazar. El filtro es el MISMO
		// predicado que usa el guardián (gate.allowsTool), así que catálogo y
		// despacho no pueden discrepar: ofrecer una herramienta sin permiso gasta
		// pasos, tokens y la confianza del usuario en el informe.
		if !gate.allowsTool(t) {
			continue
		}
		tools = append(tools, aiToolSchema{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return tools
}

// aiToolWire es el formato que espera la API de herramientas.
//
// POR QUÉ NO SE MANDA `aiToolSchema` DIRECTO.
//
// El esquema interno de una herramienta es {name, description, parameters}, pero
// la API de OpenAI-compatible espera un objeto con `type` y una función dentro:
//
//	{"type":"function","function":{"name":...,"description":...,"parameters":{...}}}
//
// Sin el envoltorio, un servidor estricto rechaza la petición entera con
// "missing field `type`" y el agente no arranca. Medido contra ORC24: HTTP 400
// al primer paso, sin ejecutar ni una herramienta. Con api.openai.com, que es
// más permisivo, el síntoma es distinto pero igual de inútil.
//
// Este es el tipo de fallo que solo aparece al hablar con un servidor de verdad:
// el JSON es "razonable" y compila, y los tests no lo miran porque nadie los
// pasó por la API.
type aiToolWire struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

// aiNombreDeHerramientaParaAPI traduce el nombre interno al que acepta la API.
//
// El catálogo usa nombres con punto (`db.query`, `report.save_template`) porque
// se leen bien en el guardián y en los logs. La API de OpenAI-compatible no los
// admite: solo letras, dígitos, guion bajo y guion. Medido contra ORC24:
// "Validation: Function at index 0 has an invalid name: \"db.query\"", y con eso
// el agente no ejecuta NADA.
//
// Solo el primer punto se traduce, y a guion bajo: `db.query` -> `db_query`.
// Un nombre interno no puede contener guion bajo antes del punto, así que el
// camino de vuelta es inequívoco.
func aiNombreDeHerramientaParaAPI(nombre string) string {
	return strings.Replace(nombre, ".", "_", 1)
}

// aiNombreDeHerramientaDesdeAPI deshace la traducción.
//
// POR QUÉ NO ES SIMPLEMENTE CAMBIAR EL GUION BAJO POR UN PUNTO.
//
// Porque ya hay herramientas que se llaman con guion bajo (`describe_schema`), y
// al traducirlas a ciegas pasaban a llamarse `describe.schema`, que no existe en
// el catálogo: el guardián las rechazaba con "no está registrada en el catálogo
// de riesgos". Medido: el agente pedía el esquema y se lo negaban.
//
// Aquí se busca el nombre de verdad: primero tal cual llega, y si no está
// registrado, con el primer guion bajo cambiado por un punto. Un registro que se
// consulta vale más que una regla sobre el texto.
func aiNombreDeHerramientaDesdeAPI(nombre string) string {
	if aiHerramientaRegistrada(nombre) {
		return nombre
	}
	conPunto := strings.Replace(nombre, "_", ".", 1)
	if aiHerramientaRegistrada(conPunto) {
		return conPunto
	}
	return nombre
}

// aiHerramientaRegistrada dice si el nombre está en el catálogo.
func aiHerramientaRegistrada(nombre string) bool {
	for _, t := range aiToolRegistry {
		if t.Name == nombre {
			return true
		}
	}
	return false
}

// aiToolsJSON serializa el catálogo para la petición al proveedor.
func aiToolsJSON(operator bool, gate aiRoleGate) string {
	lista := aiCatalogForPhase(operator, gate)
	wire := make([]aiToolWire, 0, len(lista))
	for _, t := range lista {
		w := aiToolWire{Type: "function"}
		w.Function.Name = aiNombreDeHerramientaParaAPI(t.Name)
		w.Function.Description = t.Description
		w.Function.Parameters = t.Parameters
		wire = append(wire, w)
	}
	b, _ := json.Marshal(wire)
	return string(b)
}

// aiRunJournal es el registro de acciones revertibles de una corrida.
type aiRunJournal struct {
	Actions []aiJournalAction `json:"actions"`

	// NeedsHuman marca la corrida para revisión humana: el agente ejecutó (o
	// intentó) una acción crítica —nómina, pagos, anulaciones—.
	//
	// EXPORTADO Y CON ETIQUETA A PROPÓSITO. Antes era `needsHuman bool`: sin
	// exportar y sin tag, encoding/json lo ignoraba al persistir el journal, así
	// que la marca que el guardián sí calculaba no llegaba ni a la base ni a la
	// pantalla. Un campo que solo existe en memoria no es una marca de
	// aprobación: es una intención.
	NeedsHuman bool `json:"needs_human"`

	// Contexto de ejecución, sin exportar: no forma parte del journal persistido
	// y se rellena al abrir la corrida. Es lo que permite calcular la clave de
	// idempotencia de cada herramienta.
	runID   int64
	taskID  int64
	agentID int64
	scope   string
}

// idemScope es el ámbito de deduplicación de la corrida: la ranura de
// programación o, en un disparo manual, la tarea y el día.
//
// NO incluye el run_id, y es deliberado: si lo incluyera, el reintento de la
// cola —que abre otra corrida— volvería a escribir todo, que es exactamente el
// fallo que esto cierra.
func (j *aiRunJournal) idemScope() string {
	if j == nil {
		return ""
	}
	if j.scope != "" {
		return j.scope
	}
	return "run:" + fmt.Sprintf("%d", j.runID)
}

type aiJournalAction struct {
	Kind   string                 `json:"kind"`
	Params map[string]interface{} `json:"params"`
	At     string                 `json:"at"`
}

func (j *aiRunJournal) add(kind string, params map[string]interface{}) {
	if j == nil {
		return
	}
	j.Actions = append(j.Actions, aiJournalAction{Kind: kind, Params: params, At: now()})
}

// aiToolPayload es la petición de herramienta propuesta por el modelo.
type aiToolPayload struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// aiToolError es la respuesta de error estándar de una herramienta.
type aiToolError struct {
	Error string `json:"error"`
}

// aiToolBlocked es lo que devuelve el guardián cuando detiene una acción. Es un
// tipo aparte (y no un aiToolError) para que el agente distinga claramente
// "el sistema te lo prohibió" de "la operación falló": lo primero no se
// reintenta, lo segundo sí.
type aiToolBlocked struct {
	Error   string `json:"error"`
	Blocked bool   `json:"blocked"`
	Rule    string `json:"rule"`
	Tool    string `json:"tool"`
}

// aiDispatchTool valida y ejecuta una herramienta. agent debe estar activo y
// verificado por el llamador; operatorEnablement se pasa explícito.
func (s *Server) aiDispatchTool(sess *SessionData, agent aiAgent, p aiToolPayload, journal *aiRunJournal, gate aiRoleGate) (string, error) {
	operator := agent.OperatorEnabled == 1

	// El modelo devuelve el nombre tal y como se lo announce en el catálogo (con
	// guion bajo en vez del punto). Se vuelve al nombre interno antes de nada,
	// porque a partir de aquí el guardián y el registro trabajan con `db.query`.
	p.Name = aiNombreDeHerramientaDesdeAPI(p.Name)

	// GUARDIÁN DE NIVEL 1: se ejecuta ANTES de cualquier acción, en código, no
	// en el prompt. Es el único punto por donde pasa toda operación de un agente,
	// de modo que ninguna ruta (tarea, informe, modo operador) pueda saltárselo.
	verdict := aiGuardAction(p.Name, p.Args, operator, gate)
	if !verdict.Allowed {
		rule := verdict.Rule
		if rule == "" {
			rule = "nivel-1"
		}
		return string(mustJSON(aiToolBlocked{
			Error:   verdict.Reason,
			Blocked: true,
			Rule:    rule,
			Tool:    p.Name,
		})), nil
	}
	// Las acciones críticas (nómina, pagos, anulaciones) se marcan para que un
	// humano las revise aunque el agente tenga permisos de operador. La marca
	// viaja en el journal y se PERSISTE con la corrida (ai_task_run.needs_human).
	if verdict.NeedsHuman && journal != nil {
		journal.NeedsHuman = true
	}

	// NIVEL 2: el permiso del ROL del agente. Sin esto, "modo operador activo"
	// era la única puerta y un agente de compras podía cobrar facturas o mover
	// dinero con tal de que alguien hubiera marcado su casilla de operador.
	if motivo := s.aiPermisoDenegado(sess, agent, p.Name); motivo != "" {
		return string(mustJSON(aiToolBlocked{
			Error:   motivo,
			Blocked: true,
			Rule:    "nivel-2",
			Tool:    p.Name,
		})), nil
	}

	// La ejecución sale del MISMO registro que el guardián acaba de consultar, así
	// que no puede existir una herramienta ejecutable sin estar clasificada.
	tool, ok := aiToolByName[p.Name]
	if !ok {
		// Inaccesible en la práctica: el guardián ya habría bloqueado por
		// omisión. Se mantiene como red de seguridad si alguien llama al
		// dispatcher por otra vía en el futuro.
		return string(mustJSON(aiToolError{"Herramienta desconocida: " + p.Name})), nil
	}
	if tool.Run == nil {
		return string(mustJSON(aiToolError{"La herramienta '" + p.Name + "' está declarada pero aún no está disponible."})), nil
	}
	return tool.Run(s, sess, agent, p.Args, journal)
}

// ---------- db.query ----------

func (s *Server) aiToolDBQuery(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	sqlText := aiStrParam(args, "sql")
	limit := int(aiNumParam(args, "limit"))
	if limit <= 0 {
		limit = 100
	}
	// Guardián de escritura (SELECT puro, sin DML/DDL) MÁS guardián de lectura
	// (las tablas tienen que ser del departamento del rol del agente).
	//
	// Antes solo pasaba el primero: un SELECT podía leer cualquier tabla de las
	// 139 de la base, incluidas las que no le tocaban. El scopado del prompt es
	// cortesía; esto es el candado.
	dept := aiDeptDelAgente(agent)
	clean, err := aiValidarSQLConScope(sqlText, dept, limit)
	if err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	rows, err := s.dbForSession(sess).Query(clean)
	if err != nil {
		return string(mustJSON(aiToolError{"Error de SQL: " + err.Error()})), nil
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := [][]interface{}{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		// bytes -> string para JSON limpio
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out = append(out, vals)
		if len(out) >= limit {
			break
		}
	}
	return string(mustJSON(map[string]interface{}{"columns": cols, "rows": out, "row_count": len(out)})), nil
}

// ---------- Plantillas ----------

func (s *Server) aiToolListTemplates(sess *SessionData) (string, error) {
	rows, err := s.dbForSession(sess).Query(`SELECT id, name, IFNULL(description,'') FROM report_template WHERE status = 1 ORDER BY name`)
	if err != nil {
		return string(mustJSON(aiToolError{"No se pudieron listar plantillas: " + err.Error()})), nil
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name, desc sql.NullString
		if err := rows.Scan(&id, &name, &desc); err == nil {
			list = append(list, map[string]interface{}{"id": id, "name": name.String, "description": desc.String})
		}
	}
	return string(mustJSON(list)), nil
}

// aiToolSaveTemplate valida + guarda la plantilla con el create_by del agente.
func (s *Server) aiToolSaveTemplate(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (out string, err error) {
	db := s.dbForSession(sess)
	claim := s.aiClaimTool(db, journal, "report.save_template", aiStrParam(args, "external_ref"), args)
	if claim.Skip {
		return claim.Result, nil
	}
	defer func() { s.aiFinishTool(db, claim, out, err) }()

	name := aiStrParam(args, "name")
	definition := aiStrParam(args, "definition")
	desc := aiStrParam(args, "description")
	if name == "" || strings.TrimSpace(definition) == "" {
		return string(mustJSON(aiToolError{"Nombre y definition son obligatorios."})), nil
	}
	if err := s.validateReportDefinition(definition); err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	// Se ejecuta antes de guardar. Un informe con un total que no se puede
	// calcular es un informe que parece completo y no lo está: quien lo lea se
	// lleva el número de la última fila creyendo que es la suma. Es preferible
	// que la plantilla no se guarde y el agente reciba el motivo, porque está a
	// dos pasos de arreglarlo; si se guarda, el error aparece semanas después,
	// cuando alguien abre un informe que nunca dio cifras.
	if res, err := s.executeReportDefinition(sess, definition, "", ""); err == nil {
		if len(res.Warnings) > 0 {
			return string(mustJSON(aiToolError{
				Error: "la definición no da los totales que pide: " + strings.Join(res.Warnings, "; ") +
					". Los totales deben nombrarse con la clave que declara en columns.",
			})), nil
		}
	}
	creator := aiAgentCreator(sess, agent)
	res, err := s.dbForSession(sess).Exec(`INSERT INTO report_template (name, description, definition, created_by, status, created_at, updated_at)
		VALUES (?,?,?,?,1,?,?)`, name, desc, definition, creator, now(), now())
	if err != nil {
		return string(mustJSON(aiToolError{"No se pudo guardar: " + err.Error()})), nil
	}
	id, _ := res.LastInsertId()
	s.aiAudit(sess, aiAgentUsername(sess, agent), "create", "custom_reports", "report_template", fmt.Sprintf("%d", id), nil, map[string]interface{}{"name": name})
	return string(mustJSON(map[string]interface{}{"status": "ok", "template_id": id, "name": name})), nil
}

// aiToolRunPreview ejecuta la definición y devuelve headers/rows (sin guardar).
func (s *Server) aiToolRunPreview(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	definition := aiStrParam(args, "definition")
	if strings.TrimSpace(definition) == "" {
		return string(mustJSON(aiToolError{"definition es obligatorio."})), nil
	}
	if err := s.validateReportDefinition(definition); err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	res, err := s.executeReportDefinition(sess, definition, today(), today())
	if err != nil {
		return string(mustJSON(aiToolError{Error: err.Error()})), nil
	}
	return string(mustJSON(res)), nil
}

// ---------- Operador: borrador de factura ----------

func (s *Server) aiToolInvoiceDraft(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (out string, err error) {
	db := s.dbForSession(sess)
	claim := s.aiClaimTool(db, journal, "invoice.create_draft", aiStrParam(args, "external_ref"), args)
	if claim.Skip {
		return claim.Result, nil
	}
	defer func() { s.aiFinishTool(db, claim, out, err) }()

	customerID := int64(aiNumParam(args, "customer_id"))
	if customerID <= 0 {
		return string(mustJSON(aiToolError{"customer_id inválido."})), nil
	}
	rawItems, _ := args["items"].([]interface{})
	if len(rawItems) == 0 {
		return string(mustJSON(aiToolError{"items vacío: incluya al menos un producto."})), nil
	}
	type item struct {
		pid   string
		qty   float64
		rate  float64
		total float64
	}
	items := []item{}
	for _, ri := range rawItems {
		m, _ := ri.(map[string]interface{})
		if m == nil {
			continue
		}
		pid := aiStrParam(m, "product_id")
		if pid == "" {
			continue
		}
		qty := aiNumParam(m, "quantity")
		if qty <= 0 {
			qty = 1
		}
		rate := aiNumParam(m, "rate")
		if rate <= 0 {
			_ = s.dbForSession(sess).QueryRow(`SELECT IFNULL(price,'0') FROM product_information WHERE product_id = ?`, pid).Scan(&rate)
		}
		if rate <= 0 {
			return string(mustJSON(aiToolError{"Producto " + pid + " sin precio válido."})), nil
		}
		items = append(items, item{pid, qty, rate, round2(qty * rate)})
	}
	if len(items) == 0 {
		return string(mustJSON(aiToolError{"items vacío o inválido."})), nil
	}
	var grand float64
	for _, it := range items {
		grand += it.total
	}
	grand = round2(grand)
	date := aiStrParam(args, "date")
	if !aiValidDate(date) {
		date = today()
	}
	creator := aiAgentUsername(sess, agent)

	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var invoiceNo int64
	if err := tx.QueryRow(`SELECT IFNULL(MAX(invoice),999)+1 FROM invoice`).Scan(&invoiceNo); err != nil {
		return string(mustJSON(aiToolError{"No se pudo asignar número: " + err.Error()})), nil
	}
	res, err := tx.Exec(`INSERT INTO invoice (invoice_id, customer_id, date, total_amount, paid_amount, due_amount,
		prevous_due, shipping_cost, invoice, invoice_discount, total_discount, total_vat_amnt, total_tax,
		invoice_details, is_credit, is_fixed, is_draft, is_dynamic, status, payment_type, sales_by, series)
		VALUES (?,?,?,?,0,0,0,0,?,0,0,0,0,'Borrador creado por agente IA',0,0,'yes',0,0,5,?, 'FACT')`,
		invoiceNo, customerID, date, grand, invoiceNo, creator)
	if err != nil {
		return string(mustJSON(aiToolError{"No se pudo crear el borrador: " + err.Error()})), nil
	}
	internalID, _ := res.LastInsertId()
	for _, it := range items {
		if _, err := tx.Exec(`INSERT INTO invoice_details (invoice_details_id, invoice_id, product_id, serial_no, description,
			quantity, rate, discount, discount_per, price_list_id, discount_pct, discount_amount, base_price,
			vat_amnt, vat_amnt_per, tax, paid_amount, due_amount, total_price, status)
			VALUES (?,?,?,?, '', ?,?,0,'0',1,0,0,?,0,'0',0,0,?,0,0)`,
			generator(15), internalID, it.pid, it.qty, it.rate, it.rate,
			it.total, it.total); err != nil {
			return string(mustJSON(aiToolError{"No se pudo crear el renglón: " + err.Error()})), nil
		}
	}
	if err := tx.Commit(); err != nil {
		return string(mustJSON(aiToolError{"No se pudo confirmar el borrador: " + err.Error()})), nil
	}
	journal.add("draft_invoice", map[string]interface{}{"internal_id": internalID, "invoice_no": invoiceNo})
	s.aiAudit(sess, creator, "create", "ai_operator", "invoice", fmt.Sprintf("%d", invoiceNo), nil, map[string]interface{}{"action": "create_draft", "customer_id": customerID, "total": grand})
	return string(mustJSON(map[string]interface{}{"status": "ok", "draft_id": invoiceNo, "internal_id": internalID, "total": grand, "note": "Borrador sin efecto contable; publique con invoice.post."})), nil
}

// aiToolInvoicePost publica un borrador usando el flujo contable estándar.
func (s *Server) aiToolInvoicePost(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (out string, err error) {
	db := s.dbForSession(sess)
	claim := s.aiClaimTool(db, journal, "invoice.post", aiStrParam(args, "external_ref"), args)
	if claim.Skip {
		return claim.Result, nil
	}
	defer func() { s.aiFinishTool(db, claim, out, err) }()

	draftID := int64(aiNumParam(args, "draft_id"))
	if draftID <= 0 {
		return string(mustJSON(aiToolError{"draft_id inválido."})), nil
	}
	var isDraft string
	var customerID int64
	var total float64
	err = s.dbForSession(sess).QueryRow(`SELECT is_draft, customer_id, total_amount FROM invoice WHERE invoice = ?`, draftID).Scan(&isDraft, &customerID, &total)
	if err != nil {
		return string(mustJSON(aiToolError{"Borrador no encontrado."})), nil
	}
	if isDraft != "yes" {
		return string(mustJSON(aiToolError{"La factura ya está publicada."})), nil
	}
	// Cuenta por cobrar (crédito): débito cliente / crédito ventas. El resto
	// del ciclo (COGS/IVA) lo recalcula el flujo humano al editar; el agente
	// solo genera el asiento de la partida principal.
	pre, err := s.predefineAccount(s.requestForSession(sess))
	if err != nil {
		return string(mustJSON(aiToolError{"No se pudo cargar el plan de cuentas: " + err.Error()})), nil
	}
	creator := aiAgentUsername(sess, agent)
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE invoice SET is_draft='no', status=1, paid_amount=0, due_amount=total_amount WHERE invoice = ?`, draftID); err != nil {
		return string(mustJSON(aiToolError{"No se pudo publicar: " + err.Error()})), nil
	}
	_, _ = tx.Exec(`UPDATE invoice_details SET status=1 WHERE invoice_id = (SELECT id FROM invoice WHERE invoice = ?)`, draftID)
	vno := nextVoucherNoTx(tx, "JV")
	sub := s.subcodeID(s.requestForSession(sess), customerID, 3)
	if err := s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", draftID),
		COAID: pre.CustomerCode, RevCodde: pre.SalesCode,
		Narration: "Publicación de borrador por agente IA", Comment: "Accounts receivable posting",
		Debit: total, SubType: 3, SubCode: sub,
	}, creator); err != nil {
		return string(mustJSON(aiToolError{"No se pudo registrar el asiento: " + err.Error()})), nil
	}
	if err := tx.Commit(); err != nil {
		return string(mustJSON(aiToolError{"No se pudo confirmar la publicación: " + err.Error()})), nil
	}
	journal.add("posted_invoice", map[string]interface{}{"invoice_no": draftID, "vno": vno})
	s.aiAudit(sess, creator, "update", "ai_operator", "invoice", fmt.Sprintf("%d", draftID), map[string]interface{}{"is_draft": "yes"}, map[string]interface{}{"is_draft": "no", "voucher": vno})
	return string(mustJSON(map[string]interface{}{"status": "ok", "invoice_id": draftID, "voucher": vno, "total": total})), nil
}

// ---------- Operador: cobro ----------

func (s *Server) aiToolPaymentRecord(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (out string, err error) {
	db := s.dbForSession(sess)
	claim := s.aiClaimTool(db, journal, "payment.record", aiStrParam(args, "external_ref"), args)
	if claim.Skip {
		return claim.Result, nil
	}
	defer func() { s.aiFinishTool(db, claim, out, err) }()

	invoiceID := int64(aiNumParam(args, "invoice_id"))
	amount := round2(aiNumParam(args, "amount"))
	if invoiceID <= 0 || amount <= 0 {
		return string(mustJSON(aiToolError{"invoice_id y amount son obligatorios."})), nil
	}
	var due, paid float64
	var customerID int64
	err = s.dbForSession(sess).QueryRow(`SELECT due_amount, paid_amount, customer_id FROM invoice WHERE invoice = ? AND is_draft = 'no'`, invoiceID).Scan(&due, &paid, &customerID)
	if err != nil {
		return string(mustJSON(aiToolError{"Factura no encontrada o en borrador."})), nil
	}
	if due <= 0 {
		return string(mustJSON(aiToolError{"La factura no tiene saldo pendiente."})), nil
	}
	if amount > round2(due) {
		return string(mustJSON(aiToolError{fmt.Sprintf("El monto excede el saldo pendiente (%.2f).", due)})), nil
	}
	pre, err := s.predefineAccount(s.requestForSession(sess))
	if err != nil {
		return string(mustJSON(aiToolError{"No se pudo cargar el plan de cuentas: " + err.Error()})), nil
	}
	creator := aiAgentUsername(sess, agent)
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	newPaid := round2(paid + amount)
	newDue := round2(due - amount)
	if _, err := tx.Exec(`UPDATE invoice SET paid_amount=?, due_amount=? WHERE invoice=?`, newPaid, newDue, invoiceID); err != nil {
		return string(mustJSON(aiToolError{"No se pudo actualizar la factura: " + err.Error()})), nil
	}
	vno := nextVoucherNoTx(tx, "CV")
	sub := s.subcodeID(s.requestForSession(sess), customerID, 3)
	if err := s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
		VNo: vno, Vtype: "CV", ReferenceNo: fmt.Sprintf("%d", invoiceID),
		COAID: pre.CustomerCode, RevCodde: pre.CashCode,
		Narration: "Cobro registrado por agente IA", Comment: aiStrParam(args, "note"),
		Debit: amount, SubType: 3, SubCode: sub,
	}, creator); err != nil {
		return string(mustJSON(aiToolError{"No se pudo registrar el asiento de cobro: " + err.Error()})), nil
	}
	if err := tx.Commit(); err != nil {
		return string(mustJSON(aiToolError{"No se pudo confirmar el cobro: " + err.Error()})), nil
	}
	journal.add("payment", map[string]interface{}{"invoice_no": invoiceID, "amount": amount, "vno": vno})
	s.aiAudit(sess, creator, "create", "ai_operator", "invoice", fmt.Sprintf("%d", invoiceID), map[string]interface{}{"due": due}, map[string]interface{}{"paid": newPaid, "due": newDue, "voucher": vno})
	return string(mustJSON(map[string]interface{}{"status": "ok", "invoice_id": invoiceID, "paid": newPaid, "due": newDue, "voucher": vno})), nil
}

// ---------- Operador: voucher ----------

type aiVoucherLine struct {
	COAID  int64   `json:"coa_id"`
	Debit  float64 `json:"debit"`
	Credit float64 `json:"credit"`
}

func (s *Server) aiToolVoucherCreate(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (out string, err error) {
	db := s.dbForSession(sess)
	claim := s.aiClaimTool(db, journal, "voucher.create", aiStrParam(args, "external_ref"), args)
	if claim.Skip {
		return claim.Result, nil
	}
	defer func() { s.aiFinishTool(db, claim, out, err) }()

	vtype := strings.ToUpper(aiStrParam(args, "vtype"))
	if vtype == "" {
		vtype = "JV"
	}
	if vtype != "JV" && vtype != "CV" && vtype != "DV" {
		return string(mustJSON(aiToolError{"Tipo de voucher inválido (JV/CV/DV)."})), nil
	}
	raw, _ := args["lines"].([]interface{})
	if len(raw) < 2 {
		return string(mustJSON(aiToolError{"Un comprobante requiere al menos 2 líneas."})), nil
	}
	lines := []aiVoucherLine{}
	var sumD, sumC float64
	for _, rl := range raw {
		m, _ := rl.(map[string]interface{})
		if m == nil {
			continue
		}
		l := aiVoucherLine{COAID: int64(aiNumParam(m, "coa_id")), Debit: round2(aiNumParam(m, "debit")), Credit: round2(aiNumParam(m, "credit"))}
		if l.COAID <= 0 {
			return string(mustJSON(aiToolError{"Línea sin coa_id válido."})), nil
		}
		var exists int
		_ = s.dbForSession(sess).QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE HeadCode = ?`, l.COAID).Scan(&exists)
		if exists == 0 {
			return string(mustJSON(aiToolError{fmt.Sprintf("La cuenta %d no existe.", l.COAID)})), nil
		}
		if (l.Debit > 0) == (l.Credit > 0) && l.Debit != 0 && l.Credit != 0 {
			return string(mustJSON(aiToolError{"Cada línea debe ser débito o crédito, no ambos."})), nil
		}
		if l.Debit == 0 && l.Credit == 0 {
			continue
		}
		sumD += l.Debit
		sumC += l.Credit
		lines = append(lines, l)
	}
	if len(lines) < 2 {
		return string(mustJSON(aiToolError{"Se requieren al menos 2 líneas válidas."})), nil
	}
	if round2(sumD) != round2(sumC) {
		return string(mustJSON(aiToolError{fmt.Sprintf("No cuadra: débito %.2f vs crédito %.2f.", sumD, sumC)})), nil
	}
	creator := aiAgentUsername(sess, agent)
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	vno := nextVoucherNoTx(tx, vtype)
	ref := aiStrParam(args, "ref")
	note := aiStrParam(args, "note")
	if note == "" {
		note = "Asiento creado por agente IA"
	}
	for _, l := range lines {
		if err := s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
			VNo: vno, Vtype: vtype, ReferenceNo: ref, COAID: l.COAID,
			Narration: note, Debit: l.Debit, Credit: l.Credit,
		}, creator); err != nil {
			return string(mustJSON(aiToolError{"No se pudo insertar la línea: " + err.Error()})), nil
		}
	}
	if err := tx.Commit(); err != nil {
		return string(mustJSON(aiToolError{"No se pudo confirmar el voucher: " + err.Error()})), nil
	}
	journal.add("voucher", map[string]interface{}{"vno": vno, "vtype": vtype, "ref": ref})
	s.aiAudit(sess, creator, "create", "ai_operator", "acc_vaucher", vno, nil, map[string]interface{}{"vtype": vtype, "debit": sumD, "credit": sumC, "ref": ref, "note": note})
	return string(mustJSON(map[string]interface{}{"status": "ok", "vno": vno, "debit": sumD, "credit": sumC})), nil
}

// ---------- Reversión (undo) ----------

// aiUndoLastRun revierte en orden inverso las acciones del run y marca el run
// como 'reverted'. Toda acción deja auditoría con el usuario del agente.
func (s *Server) aiUndoLastRun(sess *SessionData, agent aiAgent, runID int64) error {
	var status string
	var raw sql.NullString
	if err := s.dbForSession(sess).QueryRow(`SELECT status, COALESCE(undo_journal,'') FROM ai_task_run WHERE id = ? AND agent_id = ?`, runID, agent.ID).Scan(&status, &raw); err != nil {
		return fmt.Errorf("corrida no encontrada")
	}
	// Se admiten 'done' Y 'failed'. Antes solo 'done', y justo la corrida que más
	// necesita compensación —la que murió a mitad de trabajo dejando escrituras
	// parciales— era la única que no se podía revertir desde la interfaz.
	if status != "done" && status != "failed" {
		return fmt.Errorf("solo se pueden revertir corridas terminadas (estado actual: %s)", status)
	}
	var j aiRunJournal
	if raw.Valid && strings.TrimSpace(raw.String) != "" {
		if err := json.Unmarshal([]byte(raw.String), &j); err != nil {
			return fmt.Errorf("journal inválido: %v", err)
		}
	}
	creator := aiAgentUsername(sess, agent)
	for i := len(j.Actions) - 1; i >= 0; i-- {
		a := j.Actions[i]
		switch a.Kind {
		case "draft_invoice":
			internal, _ := a.Params["internal_id"].(float64)
			no, _ := a.Params["invoice_no"].(float64)
			_ = s.undoDeleteDraftInvoice(sess, int64(internal), int64(no), creator)
		case "posted_invoice":
			no, _ := a.Params["invoice_no"].(float64)
			vno, _ := a.Params["vno"].(string)
			_ = s.undoUnpostInvoice(sess, int64(no), vno, creator)
		case "payment":
			invoiceNo, _ := a.Params["invoice_no"].(float64)
			amount, _ := a.Params["amount"].(float64)
			vno, _ := a.Params["vno"].(string)
			_ = s.undoReversePayment(sess, int64(invoiceNo), amount, vno, creator)
		case "voucher":
			vno, _ := a.Params["vno"].(string)
			_ = s.undoContraVoucher(sess, vno, creator)
		}
	}
	if _, err := s.dbForSession(sess).Exec(`UPDATE ai_task_run SET status='reverted', undo_journal=? WHERE id=?`, mustJSON(&aiRunJournal{}), runID); err != nil {
		return err
	}
	// Las claves de idempotencia de esta corrida quedan libres: sus documentos ya
	// no existen, así que una nueva ejecución debe poder crearlos. Sin esto,
	// revertir una corrida y volver a lanzar la tarea devolvería «ya se ejecutó»
	// y no crearía nada, que es el fallo opuesto y también caro.
	_, _ = s.dbForSession(sess).Exec(`UPDATE ai_tool_execution SET status='undone', updated_at=? WHERE run_id = ?`, now(), runID)
	s.aiAudit(sess, creator, "task.reverted", "ai_operator", "ai_task_run", fmt.Sprintf("%d", runID), nil, map[string]interface{}{"actions": len(j.Actions)})
	return nil
}

func (s *Server) undoDeleteDraftInvoice(sess *SessionData, internalID, invoiceNo int64, creator string) error {
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	tx.Exec(`DELETE FROM invoice_details WHERE invoice_id = ?`, internalID)
	if _, err := tx.Exec(`DELETE FROM invoice WHERE id = ? AND is_draft = 'yes'`, internalID); err != nil {
		return err
	}
	return tx.Commit()
}

// undoUnpostInvoice: no borra registros financieros. Devuelve la factura a
// borrador y anula el asiento con un contra-asiento espejo.
func (s *Server) undoUnpostInvoice(sess *SessionData, invoiceNo int64, vno, creator string) error {
	// Lee el estado ANTES de la tx (evita mezclar lecturas con la escritura
	// abierta en SQLite en memoria).
	var total float64
	var customerID int64
	if err := s.dbForSession(sess).QueryRow(`SELECT total_amount, customer_id FROM invoice WHERE invoice=?`, invoiceNo).Scan(&total, &customerID); err != nil {
		return err
	}
	pre, err := s.predefineAccount(s.requestForSession(sess))
	if err != nil {
		return err
	}
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE invoice SET is_draft='yes', status=0, paid_amount=0, due_amount=0 WHERE invoice=? AND is_draft='no'`, invoiceNo); err != nil {
		return err
	}
	if total > 0 {
		cvno := nextVoucherNoTx(tx, "JV")
		sub := s.subcodeID(s.requestForSession(sess), customerID, 3)
		_ = s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
			VNo: cvno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
			COAID: pre.SalesCode, RevCodde: pre.CustomerCode,
			Narration: "Reversión de publicación (agente IA)", Comment: "Contra asiento de " + vno,
			Debit: total, SubType: 3, SubCode: sub,
		}, creator)
		_ = s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
			VNo: cvno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
			COAID: pre.CustomerCode, RevCodde: pre.SalesCode,
			Narration: "Reversión de publicación (agente IA)", Comment: "Contra asiento de " + vno,
			Credit: total, SubType: 3, SubCode: sub,
		}, creator)
	}
	return tx.Commit()
}

// undoReversePayment: restaura el saldo y crea el contra-asiento del cobro.
func (s *Server) undoReversePayment(sess *SessionData, invoiceNo int64, amount float64, vno, creator string) error {
	// Lee el estado de la factura antes de abrir la tx de escritura (misma
	// razón que undoContraVoucher: no mezclar lecturas con la tx abierta).
	var paid, due float64
	var customerID int64
	if err := s.dbForSession(sess).QueryRow(`SELECT paid_amount, due_amount, customer_id FROM invoice WHERE invoice=?`, invoiceNo).Scan(&paid, &due, &customerID); err != nil {
		return err
	}
	pre, err := s.predefineAccount(s.requestForSession(sess))
	if err != nil {
		return err
	}
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	newPaid := round2(paid - amount)
	newDue := round2(due + amount)
	if _, err := tx.Exec(`UPDATE invoice SET paid_amount=?, due_amount=? WHERE invoice=?`, newPaid, newDue, invoiceNo); err != nil {
		return err
	}
	cvno := nextVoucherNoTx(tx, "JV")
	sub := s.subcodeID(s.requestForSession(sess), customerID, 3)
	_ = s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
		VNo: cvno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
		COAID: pre.CashCode, RevCodde: pre.CustomerCode,
		Narration: "Reversión de cobro (agente IA)", Comment: "Contra de " + vno,
		Debit: amount, SubType: 3, SubCode: sub,
	}, creator)
	_ = s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
		VNo: cvno, Vtype: "JV", ReferenceNo: fmt.Sprintf("%d", invoiceNo),
		COAID: pre.CustomerCode, RevCodde: pre.CashCode,
		Narration: "Reversión de cobro (agente IA)", Comment: "Contra de " + vno,
		Credit: amount, SubType: 3, SubCode: sub,
	}, creator)
	return tx.Commit()
}

// undoContraVoucher: anula el voucher con un contra-espejo (los vouchers
// nunca se borran: la contabilidad es apéndice).
func (s *Server) undoContraVoucher(sess *SessionData, vno, creator string) error {
	// Lee las filas del voucher con una conexión PROPIA antes de abrir la
	// transacción del contra-asiento (en SQLite, una tx de escritura abierta
	// bloquea las lecturas de otra conexión de la misma BD en memoria).
	type row struct {
		vtype, ref, narr, comment string
		coaid, rev                int64
		debit, credit             float64
		subType                   sql.NullInt64
		subCode                   sql.NullInt64
	}
	list := []row{}
	rows, err := s.dbForSession(sess).Query(`SELECT Vtype, referenceNo, COAID, Narration, ledgerComment, Debit, Credit, subType, subCode, RevCodde FROM acc_vaucher WHERE VNo = ?`, vno)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r row
		var vtype sql.NullString
		var ref sql.NullString
		if err := rows.Scan(&vtype, &ref, &r.coaid, &r.narr, &r.comment, &r.debit, &r.credit, &r.subType, &r.subCode, &r.rev); err == nil {
			r.vtype, r.ref = vtype.String, ref.String
			list = append(list, r)
		}
	}
	rows.Close()
	if len(list) == 0 {
		return fmt.Errorf("voucher no encontrado")
	}
	tx, err := s.dbForSession(sess).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	cvno := nextVoucherNoTx(tx, "JV")
	for _, r := range list {
		if err := s.insertVoucherRowTx(s.requestForSession(sess), tx, VoucherRow{
			VNo: cvno, Vtype: "JV", ReferenceNo: r.ref,
			COAID: r.coaid, RevCodde: r.rev,
			Narration: "Contra de " + vno + " (reversión agente IA)", Comment: r.comment,
			Debit: r.credit, Credit: r.debit, SubType: int(r.subType.Int64), SubCode: r.subCode,
		}, creator); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// aiAggregatesForRange: resumen precalculado (reutiliza los resúmenes del
// módulo AI existente) para data.summary.
func (s *Server) aiAggregatesForRange(sess *SessionData, from, to string) (map[string]interface{}, error) {
	cur := s.settingsForSession(sess).Web.Currency
	if cur == "" {
		cur = "$"
	}
	sales, salesSeries, err := s.aiSalesSummary(sess, from, to, cur)
	if err != nil {
		return nil, err
	}
	purchases, purchaseSeries, err := s.aiPurchaseSummary(sess, from, to, cur)
	if err != nil {
		return nil, err
	}
	stock, stockSeries, err := s.aiStockSummary(sess, to, cur)
	if err != nil {
		return nil, err
	}
	sum := map[string]float64{}
	for _, series := range []map[string]float64{salesSeries, purchaseSeries, stockSeries} {
		for k, v := range series {
			sum[k] += v
		}
	}
	return map[string]interface{}{
		"range":     map[string]string{"from": from, "to": to},
		"currency":  cur,
		"summaries": []string{sales, purchases, stock},
		"daily":     sum,
	}, nil
}

// aiToolTimeout es el presupuesto máximo por llamada al proveedor en el
// runner (reutiliza aiTimeout de ai_providers.go).
var _ = aiTimeout

// aiRunnerStartedAt es referenciado por el runner para timestamps.
var aiRunnerStartedAt = func() string { return time.Now().Format("2006-01-02 15:04:05") }
