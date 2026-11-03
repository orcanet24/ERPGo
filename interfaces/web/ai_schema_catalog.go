package web

// ai_schema_catalog.go — "Mapa estructural de la BD" para agentes IA.
//
// Este archivo tiene DOS capas y conviene no confundirlas:
//
//	1. La WHITELIST (aiCatalogWhitelist) y las RELACIONES (aiCatalogRelations):
//	   conocimiento de NEGOCIO escrito a mano. Dice qué significa cada tabla
//	   ("Vtype distingue JV de CV de DV") y cómo se unen dos de ellas. Eso no
//	   está en la base de datos en ninguna parte y nadie lo puede deducir.
//
//	2. La ESTRUCTURA (columnas, tipos, PK, FK): se LEE de MySQL en cada llamada
//	   por ai_schema_introspect.go. Antes se leía aquí con una consulta por tabla
//	   dentro de un bucle; ahora sale de la caché del introspector.
//
// La whitelist sigue siendo la frontera de seguridad: una tabla que no está
// aquí no se envía al modelo aunque exista en la base. Tablas sensibles
// (credenciales, claves de API, sesiones, la propia tabla de agentes) quedan
// fuera por diseño.

import (
	"fmt"
	"sort"
	"strings"
)

// aiCatalogTable describe una tabla de la whitelist.
type aiCatalogTable struct {
	Name        string
	Description string
	Columns     string // descripción compacta de columnas (rellenada por introspección)
	Relations   string // joins típicos (rellenada por introspección)
	Tipo        string // 'operacional', 'fiscal' o 'solo-lectura-fiscal'
}

// aiCatalogWhitelist: tablas de negocio visibles para la IA y para el modal
// de ayuda del diseñador de informes. Cualquier tabla fuera de esta lista
// (users, user_login, ai_providers, ai_agent, sesiones, internas) es invisible.
var aiCatalogWhitelist = map[string]string{
	"invoice":                  "Facturas de venta (cabecera): cliente, fechas, totales, pagado, por cobrar, estado e is_draft. [TIPO: operacional - IA puede consultar ventas, calcular totales]",
	"invoice_details":          "Renglones de factura: producto, cantidad, precio, descuento, IVA, total por renglón. Une con invoice por invoice_id. [TIPO: operacional]",
	"customer_information":     "Clientes: nombre, RIF, contacto y estado. PK customer_id. [TIPO: operacional - IA puede ver datos del cliente]",
	"supplier_information":     "Proveedores: nombre, contacto y estado. PK supplier_id. [TIPO: operacional - IA puede ver datos del proveedor]",
	"product_information":      "Productos: nombre, modelo, unidad, barcode, precio de venta y estado. PK product_id. [TIPO: operacional - IA puede consultar stock y precios]",
	"product_category":         "Categorías de producto. PK category_id. [TIPO: operacional]",
	"product_purchase":         "Compras (cabecera): proveedor, fecha, totales, estado. PK purchase_id. [TIPO: operacional - IA puede consultar historial de compras]",
	"product_purchase_details": "Renglones de compra: producto, cantidad, costo (rate), total. Une con product_purchase por purchase_id. [TIPO: operacional]",
	"product_return":           "Devoluciones: usablity 1=devolución de venta, 2=devolución a proveedor; net_total_amount. [TIPO: operacional]",
	"product_service":          "Servicios facturables separados de productos. [TIPO: operacional]",
	"quotation":                "Cotizaciones (cabecera) y su estado. [TIPO: operacional]",
	"service_invoice":          "Facturas de servicio: cliente, fecha, totales. [TIPO: operacional]",
	"acc_coa":                  "Plan de cuentas (COA): HeadCode, HeadName, HeadType (A=Activo, L=Pasivo, I=Ingreso, E=Gasto), jerarquía PHeadName. [TIPO: fiscal - IA SOLO LECTURA, no modificar estructura]",
	"acc_transaction":          "Movimientos contables por cuenta: VNo, VDate, Debit, Credit. Une con acc_coa por COAID. [TIPO: fiscal - IA SOLO LECTURA, NUNCA modificación]",
	"acc_vaucher":              "Vouchers/comprobantes: VNo, Vtype (JV/CV/DV/CONTRA), referencia, débito/crédito por línea. [TIPO: fiscal - IA SOLO LECTURA, nunca escribir]",
	"acc_subcode":              "Subcódigos que enlkan cuentas con clientes/proveedores: subTypeId 3=cliente, 4=proveedor, referenceNo. [TIPO: fiscal]",
	"bank_information":         "Bancos registrados y sus cuentas. [TIPO: operacional]",
	"bank_transaction":         "Movimientos bancarios: fecha, débito/crédito, referencia. [TIPO: operacional]",
	"cash_session":             "Sesiones de caja por cajero: apertura/cierre, totales. [TIPO: operacional]",
	"cash_session_payment":     "Cobros registrados en cada sesión de caja. [TIPO: operacional]",
	"payment_method":           "Métodos de pago venezolanos: código, nombre, moneda extranjera (is_foreign) y cuenta contable. [TIPO: operacional]",
	"credit_note":              "Notas de crédito: cliente, factura de origen, totales. [TIPO: operacional]",
	"credit_note_details":      "Renglones de nota de crédito. [TIPO: operacional]",
	"tax_collection":           "IVA cobrado por factura/período. [TIPO: fiscal - IA SOLO LECTURA, no configurar tasas]",
	"vat_retention":            "Retenciones de IVA (libro legal Venezuela). [TIPO: fiscal - IA SOLO LECTURA]",
	"islr_retention":           "Retenciones de ISLR (libro legal Venezuela). [TIPO: fiscal - IA SOLO LECTURA]",
	"igtf_transaction":         "Percepciones IGTF registradas. [TIPO: fiscal - IA SOLO LECTURA]",
	"exchange_rate":            "Tasas de cambio BCV por fecha. [TIPO: operacional]",
	"voucher_no_setting":       "Configuración de numeración de vouchers por tipo. [TIPO: solo-lectura-fiscal]",

	"acc_coa":                  "Plan de cuentas (COA): HeadCode, HeadName, HeadType (A=Activo, L=Pasivo, I=Ingreso, E=Gasto), jerarquía PHeadName.",
	"acc_transaction":          "Movimientos contables por cuenta: VNo, VDate, Debit, Credit. Une con acc_coa por COAID.",
	"acc_vaucher":              "Vouchers/comprobantes: VNo, Vtype (JV/CV/DV/CONTRA), referencia, débito/crédito por línea.",
	"acc_subcode":              "Subcódigos que enlzan cuentas con clientes/proveedores: subTypeId 3=cliente, 4=proveedor, referenceNo.",
	"bank_information":         "Bancos registrados y sus cuentas.",
	"bank_transaction":         "Movimientos bancarios: fecha, débito/crédito, referencia.",
	"cash_session":             "Sesiones de caja por cajero: apertura/cierre, totales.",
	"cash_session_payment":     "Cobros registrados en cada sesión de caja.",
	"payment_method":           "Métodos de pago venezolanos: código, nombre, moneda extranjera (is_foreign) y cuenta contable.",
	"credit_note":              "Notas de crédito: cliente, factura de origen, totales.",
	"credit_note_details":      "Renglones de nota de crédito.",
	"tax_collection":           "IVA cobrado por factura/período.",
	"vat_retention":            "Retenciones de IVA (libro legal Venezuela).",
	"islr_retention":           "Retenciones de ISLR (libro legal Venezuela).",
	"igtf_transaction":         "Percepciones IGTF registradas.",
	"exchange_rate":            "Tasas de cambio BCV por fecha.",
	"voucher_no_setting":       "Configuración de numeración de vouchers por tipo.",

	// --- Sistema de costos ---
	//
	// Estas 13 tablas se añaden JUNTAS a propósito: si se olvida una, la IA no la
	// ve ni siendo IT, y el síntoma es silencioso (un mapa más pequeño, un
	// informe que no encuentra la tabla). Es lo que pasó con RRHH, que vio 0
	// tablas porque sus nombres estaban inventados.
	//
	// La descripción de cada una es la que verá el agente para decidir si la usa,
	// así que se escribe en lenguaje de negocio y no de esquema.
	"cost_timesheet":           "Hoja de horas: horas trabajadas por empleado y día, contra qué proyecto/producto, si es facturable y el rol (grade) con el que se trabajó.",
	"labor_grade":              "Grados de mano de obra con la tarifa horaria: salario anual base, cargas patronales (overhead_pct), horas al año y factor de productividad. PK code.",
	"product_recipe":           "Receta/BOM: componentes de un producto fabricado, cantidad por unidad y merma (loss_factor) POR COMPONENTE. Une con product_information por component_id.",
	"product_yield":            "Rendimiento de despiece: de qué producto de entrada sale este, con qué porcentaje. Se usa en despiece de carne y materias primas.",
	"cost_center":              "Centros de costo con su indirecto por hora y por unidad. PK code.",
	"process_route":            "Ruta de proceso: secuencia de operaciones, centro de trabajo, tiempo de setup (no se prorratea) y tiempo por unidad.",
	"import_cost":              "Gastos de importación por partida: flete, aduana, seguro, otros, y las unidades realmente recibidas sobre las que se reparten.",
	"price_list":               "Listas de precio (detalle, mayor). PK code.",
	"product_price_list":       "Precio de un producto en cada lista, con su margen. Une con price_list por price_list_id.",
	"product_cost_config":      "Configuración de costeo por producto: método (PP, RECIPE, YIELD, HOURS, IMPORT), si es perecedero, si está ensamblado o importado. PK product_id.",
	"cost_component":           "DESGLOSE del costo por período: qué parte es (material, labor, depreciation, overhead, customs), cuánto valía y con qué tasa se calculó. La trazabilidad del costo.",
	"cost_snapshot":            "Costo CONGELADO por período: el desglose de un mes ya cerrado. No se recalcula una vez cerrado.",
	"cost_period_close":        "Estado del cierre de costos por período: abierto o cerrado, quién y cuándo.",
	"expense":                  "Gastos registrados: fecha, monto, responsable.",
	"expense_item":             "Renglones de gastos.",
	"financial_year":           "Años fiscales: startDate, endDate, status.",
	"warehouse":                "Almacenes.",
	"store_adjustment":         "Ajustes de inventario (cabecera).",
	"store_adjustment_details": "Renglones de ajuste de inventario.",
	"product_lot":              "Lotes de producto con fecha de caducidad y cantidad por lote. Clave única (product_id, lot_code).",
	"expiry_alert_log":         "Registro de alertas de caducidad emitidas por lote.",
	"cycle_count":              "Conteos cíclicos de inventario: alcance (full/category/supplier/abc), estado y precisión obtenida.",
	"cycle_count_line":         "Renglones del conteo: cantidad esperada, contada y diferencia por producto o lote.",
	"cycle_count_adjustment":   "Ajustes generados al cerrar un conteo cíclico.",
	"reorder_policy":           "Políticas de reposición por producto: plazo en días y stock de seguridad.",
	"reorder_suggestion":       "Sugerencias de reposición: punto de pedido, cobertura y estado (critical/warning/ok).",
	"product_daily_demand":     "Demanda media diaria por producto, calculada del histórico.",
	"exchange_rate_daily":      "Tasa de cambio diaria: fecha, valor, fuente (bcv/paralelo) y usuario que la registró.",
	"exchange_rate_source":     "Fuente de tasa configurada para la facturación.",
	"supplier_product":         "Relación proveedor-producto con precio y condiciones de compra.",
	"acc_subtype":              "Subtipos del plan de cuentas.",
	"acc_monthly_balance":      "Balance de cierre mensual por cuenta.",
	"acc_opening_balance":      "Balance de apertura del ejercicio por cuenta.",
	"acc_period_close":         "Cierres de período contable.",
	"closing_records":          "Registros de cierre de caja.",
	"coa_catalog":              "Catálogo de cuentas del CONASEV.",
	"invoice_payment_summary":  "Resumen de pagos por factura.",
	"hr_department":            "Departamentos de la empresa.",
	"hr_position":              "Puestos de trabajo.",
	"hr_employee":              "Empleados: persona, puesto, departamento y fecha de ingreso.",
	"person_information":       "Ficha de la persona: nombre, cédula, dirección, teléfono y fecha de nacimiento.",
	"person_ledger":            "Historial laboral de la persona: cambios de puesto, salario y departamento.",
	"designation":              "Cargos o funciones asignables a un empleado.",
	"attendance":               "Registro de asistencia diario del personal.",
	"hr_attendance":            "Asistencia por empleado y período.",
	"hr_leave":                 "Solicitudes y control de vacaciones y permisos.",
	"hr_salary_assignment":     "Asignación salarial del empleado: salario base, frecuencia y vigencia.",
	"hr_payroll_run":           "Corridas de nómina: período, totales y estado.",
	"hr_payslip":               "Recibo de pago generado en la nómina.",
	"hr_payroll_concept":       "Conceptos de nómina (salario, deducciones, bonuses).",
	"hr_payroll_item":          "Renglones de una corrida de nómina por empleado y concepto.",
	"employee_salary_setup":    "Configuración salarial por empleado.",
	"employee_salary_payment":  "Pagos de salario registrados.",
	"employee_history":         "Historial de cambios del empleado.",
	"personal_loan":            "Préstamos al personal.",
	"pesonal_loan_information": "Información de préstamos al personal (nombre con el error del esquema original).",
	"salary_setup_header":      "Cabecera de la configuración de nómina.",
	"salary_sheet_generate":    "Hojas de salario generadas.",
	"salary_type":              "Tipos de salario.",
	"payroll_tax_setup":        "Impuestos y retenciones de la nómina.",
}

// aiCatalogRelations documenta los joins típicos entre tablas whitelisted.
var aiCatalogRelations = map[string]string{
	"invoice":                  "JOIN customer_information c ON c.customer_id = invoice.customer_id; JOIN invoice_details d ON d.invoice_id = invoice.invoice_id (nota: invoice_details.invoice_id referencia invoice.id interno, usa invoice.id al unir con detalles).",
	"invoice_details":          "JOIN product_information p ON p.product_id = invoice_details.product_id; JOIN invoice i ON i.id = invoice_details.invoice_id.",
	"product_purchase":         "JOIN supplier_information s ON s.supplier_id = product_purchase.supplier_id.",
	"product_purchase_details": "JOIN product_information p ON p.product_id = product_purchase_details.product_id; JOIN product_purchase pp ON pp.purchase_id = product_purchase_details.purchase_id.",
	"product_return":           "usablity=1: LEFT JOIN customer_information c ON c.customer_id = product_return.customer_id; usablity=2: LEFT JOIN supplier_information s ON s.supplier_id = product_return.supplier_id.",
	"acc_transaction":          "JOIN acc_coa ON acc_coa.HeadCode = acc_transaction.COAID (COAID es texto).",
	"acc_vaucher":              "JOIN acc_coa ON acc_coa.HeadCode = acc_vaucher.COAID; referencia a factura: referenceNo = invoice.invoice_id (como texto).",
	"acc_subcode":              "subTypeId=3 + referenceNo=customer_id enlaza a customer_information; subTypeId=4 a supplier_information.",
	"credit_note":              "JOIN invoice i ON i.invoice_id = credit_note.invoice_id.",
	"cash_session_payment":     "JOIN cash_session cs ON cs.id = cash_session_payment.cash_session_id.",
	"bank_transaction":         "JOIN bank_information b ON b.bank_id = bank_transaction.bank_id.",
}

// aiCatalogIsVisible indica si una tabla puede exponerse.
func aiCatalogIsVisible(table string) bool {
	_, ok := aiCatalogWhitelist[strings.ToLower(strings.TrimSpace(table))]
	return ok
}

// aiQuoteIdent cita un identificador de tabla ya validado contra la whitelist.
//
// El delimitador es el acento grave de MySQL, no las comillas dobles. Las
// comillas dobles solo son delimitador de identificador con ANSI_QUOTES, que no
// está en el sql_mode de este servidor: ahí `"invoice"` es una cadena de texto, y
// una consulta que la usa como nombre de tabla falla. Con acentos graves
// funciona siempre, y no depende de un modo que alguien puede cambiar en el
// servidor.
func (s *Server) aiQuoteIdent(name string) string {
	name = strings.TrimSpace(name)
	if !aiCatalogIsVisible(name) {
		return ""
	}
	return "`" + name + "`"
}

// aiSQLColumns consulta las columnas de una tabla.
//
// ANTES ERA `SELECT name || ' ' || lower(type) FROM pragma_table_info('x')`, que
// es SQLite por partida doble: `pragma_table_info` no existe en MySQL y `||` en
// MySQL es el OR lógico, no la concatenación. Con la tabla en una base de
// pruebas que solo tiene cinco tablas, el mapa de esquema le decía a la IA que no
// había ninguna. Un fallo que no lanza error sino que devuelve una lista vacía es
// el peor tipo: el sistema sigue "funcionando" y solo falta la información.
//
// Ahora sale de information_schema con CONCAT, que sí concatena. El filtro
// DATABASE() es lo que acota a la base conectada: sin él, el mapa de la empresa A
// incluiría las tablas de la empresa B que viven en el mismo servidor.
func (s *Server) aiSQLColumns(table string) (string, error) {
	if s.aiQuoteIdent(table) == "" {
		return "", fmt.Errorf("tabla no visible")
	}
	// ordinal_position ordena las columnas como están en la tabla. information_schema
	// no garantiza orden y, sin él, el prompt que se manda al modelo cambia de
	// forma entre llamadas.
	return "SELECT CONCAT(column_name, ' ', LOWER(column_type)) " +
		"FROM information_schema.columns " +
		"WHERE table_schema = DATABASE() AND table_name = '" + table + "' " +
		"ORDER BY ordinal_position", nil
}

// aiSQLRelations consulta FKs declaradas (informativo; los joins canónicos
// vienen de aiCatalogRelations).
//
// MySQL guarda las claves foráneas en information_schema.key_column_usage y
// no tiene `pragma_foreign_key_list`. Se mantiene aunque casi nada del esquema
// declara FKs: el ERP las define en la aplicación y no en el DDL, así que esto
// sale vacío y el modelo recibe los joins de aiCatalogRelations, que son los que
// valen.
func (s *Server) aiSQLRelations(table string) string {
	return "SELECT 'FK' FROM information_schema.key_column_usage " +
		"WHERE table_schema = DATABASE() AND table_name = '" + table + "' " +
		"AND referenced_table_name IS NOT NULL LIMIT 1"
}

// aiTableTypeRender formatea una fila de columnas.
func aiTableTypeRender(col interface{}) string {
	if v, ok := col.(string); ok {
		return v
	}
	return fmt.Sprintf("%v", col)
}

// aiColumnList devuelve la descripción compacta de columnas de una tabla.
//
// Sale de la caché del introspector, no de una consulta por tabla. El formato
// ("nombre tipo, nombre tipo") se conserva porque lo consumen el mapa y las
// plantillas de informe.
func (s *Server) aiColumnList(sess *SessionData, table string) (string, error) {
	if !aiCatalogIsVisible(table) {
		return "", fmt.Errorf("tabla no visible")
	}
	te := s.buscarEsquemaTabla(sess, table)
	if te == nil {
		return "", fmt.Errorf("la tabla %s no existe en esta base", table)
	}
	cols := make([]string, 0, len(te.Columnas))
	for _, c := range te.Columnas {
		cols = append(cols, c.Nombre+" "+aiTipoCorto(c.Tipo))
	}
	return strings.Join(cols, ", "), nil
}

// aiHasTable existe la tabla en la BD (algunas seeds de la whitelist pueden
// no existir en instalaciones viejas).
//
// Ahora consulta la caché del introspector en vez de lanzar una consulta a
// information_schema por tabla. La respuesta es la misma: si no está
// introspectada, no está en la base de esta empresa.
func (s *Server) aiHasTable(sess *SessionData, table string) bool {
	t := strings.ToLower(strings.TrimSpace(table))
	if !aiCatalogIsVisible(t) {
		return false
	}
	return s.buscarEsquemaTabla(sess, t) != nil
}

// buscarEsquemaTabla devuelve la tabla ya introspectada, o nil.
func (s *Server) buscarEsquemaTabla(sess *SessionData, table string) *TablaEsquema {
	esquema := s.obtenerEsquemaPara(sess)
	for i := range esquema.Tablas {
		if strings.EqualFold(esquema.Tablas[i].Nombre, table) {
			return &esquema.Tablas[i]
		}
	}
	return nil
}

// sqlShowTablesLike: consulta de existencia de tabla.
func (s *Server) sqlShowTablesLike(quoted string) string {
	// MySQL, no SQLite: `sqlite_master` no existe y la comprobacion de si una
	// tabla existe daria falso SIEMPRE, que es peor que un error visible:
	// el mapa de esquema le diria a la IA que no hay tablas cuando las hay.
	//
	// Se consulta information_schema con DATABASE() para que mire la base
	// conectada y no el catalogo entero del servidor.
	//
	// La comparación va con `lower()` en los dos lados porque en Windows MySQL no
	// distingue mayúsculas en los nombres de tabla pero en Linux sí, y la misma
	// base migrada de un sistema a otro no puede empezar a perder columnas del
	// mapa de la IA.
	//
	// Solo quedan comillas simples porque en este dialecto la comilla doble es
	// delimitador de identificador.
	nombre := strings.ReplaceAll(strings.Trim(quoted, "`'"), "'", "''")
	return "SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() " +
		"AND lower(table_name) = lower('" + nombre + "') LIMIT 1"
}

// aiTableSummary describe una tabla completa (para el prompt del agente).
func (s *Server) aiTableSummary(sess *SessionData, table string) string {
	desc, ok := aiCatalogWhitelist[table]
	if !ok {
		return ""
	}
	cols, err := s.aiColumnList(sess, table)
	if err != nil || strings.TrimSpace(cols) == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("- " + table + ": " + desc + "\n")
	sb.WriteString("  Columnas: " + cols + "\n")
	if rel, ok := aiCatalogRelations[table]; ok {
		sb.WriteString("  Relaciones: " + rel + "\n")
	}
	return sb.String()
}

// aiSchemaMap construye el mapa estructural completo ordenado alfabéticamente
// (determinista para el prompt y para tests).
//
// Las columnas salen del introspector (ver ai_schema_introspect.go), así que el
// mapa refleja la base REAL: una tabla con columnas nuevas aparece con ellas
// sin tocar una línea de código. Las tablas de la whitelist que no existen en
// esta base simplemente no salen, en vez de salir con la lista de columnas fija
// que mentía sobre el esquema.
func (s *Server) aiSchemaMap(sess *SessionData) string {
	return s.aiSchemaMapDepartamento(sess, DeptIT)
}

// aiSchemaMapDepartamento es el mapa scopado a un departamento. Lo que recibe
// un agente de cuentas por cobrar es el mapa de CXC, no el de 40 tablas.
func (s *Server) aiSchemaMapDepartamento(sess *SessionData, dept string) string {
	d := aiDeptNormalizar(dept)
	names := make([]string, 0, len(aiCatalogWhitelist))
	for t := range aiCatalogWhitelist {
		if !aiDeptEsVisible(d, t) {
			continue
		}
		names = append(names, t)
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Base de datos ERPGo: %d tablas de negocio visibles para el departamento %s. Cada consulta debe ceñirse a estas tablas.\n\n",
		len(names), aiDeptLabel[d]))
	for _, t := range names {
		if line := s.aiTableSummary(sess, t); line != "" {
			sb.WriteString(line)
		}
	}
	return sb.String()
}
