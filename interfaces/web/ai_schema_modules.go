package web

import (
	"fmt"
	"sort"
	"strings"
)

// ai_schema_modules.go — MCP inteligente: segmenta la base en módulos de negocio
// para que la IA razone sobre lo justo, no sobre las 100 tablas siempre.
//
// PROBLEMA: el mapa de esquema actual vuelca TODAS las tablas del departamento
// (~100, ~15K tokens) en cada prompt. LongCat-2.0 razona sobre todo eso en cada
// llamada → cada paso del agente es lento (30-90s) y un informe necesita 4-8 pasos.
//
// SOLUCIÓN (dos fases):
//   Fase 1 — Clasificación: la IA recibe solo el nombre y descripción corta de
//   cada módulo (~1.5K tokens) y decide cuáles necesita para el request.
//   Fase 2 — Generación: el MCP construye el esquema DETALLADO (columnas, tipos,
//   PK, FK, joins) solo de los módulos elegidos y la IA genera el SQL.
//
// Resultado: de ~15K tokens a ~4K tokens por prompt, y la IA raza menos → más
// rápido. La clasificación añade una llamada liviana (~3-5s) a cambio de que la
// generación sea 3x más barata. Neto: más rápido y más barato.

// aiSchemaModule es un grupo de tablas que pertenecen a un mismo dominio.
type aiSchemaModule struct {
	Key         string   // identificador estable: "ventas", "cxc", ...
	Label       string   // nombre legible para la IA.
	Description string   // descripción corta para la fase de clasificación.
	Tables      []string // nombres de tabla en la whitelist.
}

// aiSchemaModules es el catálogo de módulos. Cubre todas las tablas de la
// whitelist: si una tabla nueva se añade al whitelist sin asignarse a un módulo,
// seguirá apareciendo en el mapa departamental de siempre pero no en el camino
// inteligente. Mantener este lista al día es parte de añadir una tabla nueva.
var aiSchemaModules = []aiSchemaModule{
	{
		Key:         "ventas",
		Label:       "Ventas y Facturación",
		Description: "Facturas de venta, cotizaciones, facturas de servicio, notas de crédito y impuestos cobrados (IVA). Para informes de ventas, facturación y rentabilidad comercial.",
		Tables:      []string{"invoice", "invoice_details", "quotation", "service_invoice", "credit_note", "credit_note_details", "tax_collection", "invoice_payment_summary"},
	},
	{
		Key:         "cxc",
		Label:       "Cuentas por Cobrar",
		Description: "Cobros, deuda de clientes, cajas, sesiones de caja y métodos de pago. Para informes de cartera, cobranza, morosidad y flujo de caja de clientes.",
		Tables:      []string{"cash_session", "cash_session_payment", "payment_method", "closing_records"},
	},
	{
		Key:         "compras",
		Label:       "Compras y Cuentas por Pagar",
		Description: "Órdenes de compra, proveedores, devoluciones y gastos operativos. Para informes de compras, cuentas por pagar y gestión de proveedores.",
		Tables:      []string{"product_purchase", "product_purchase_details", "supplier_information", "product_return", "expense", "expense_item", "supplier_product"},
	},
	{
		Key:         "contabilidad",
		Label:       "Contabilidad General",
		Description: "Plan de cuentas (COA), asientos, vouchers, subcódigos, cierres de período y libros legales (CONASEV). Para informes contables, balances y libros.",
		Tables:      []string{"acc_coa", "acc_transaction", "acc_vaucher", "acc_subcode", "acc_subtype", "acc_monthly_balance", "acc_opening_balance", "acc_period_close", "voucher_no_setting", "coa_catalog"},
	},
	{
		Key:         "bancos_caja",
		Label:       "Bancos y Caja",
		Description: "Cuentas bancarias, movimientos bancarios, tasas de cambio BCV/paralelo. Para informes bancarios, conciliaciones y exposición cambiaria.",
		Tables:      []string{"bank_information", "bank_transaction", "exchange_rate", "exchange_rate_daily", "exchange_rate_source"},
	},
	{
		Key:         "inventario",
		Label:       "Inventario y Almacenes",
		Description: "Almacenes, ajustes de inventario, conteos cíclicos, lotes, alertas de caducidad y políticas de reposición. Para informes de stock, rotación y costo de inventario.",
		Tables:      []string{"warehouse", "store_adjustment", "store_adjustment_details", "cycle_count", "cycle_count_line", "cycle_count_adjustment", "product_lot", "expiry_alert_log", "reorder_policy", "reorder_suggestion", "product_daily_demand"},
	},
	{
		Key:         "costos",
		Label:       "Costos de Producción",
		Description: "Hojas de horas, grados de mano de obra, recetas/BOM, centros de costo, rutas de proceso, cierres de costo y gastos indirectos. Para informes de costo de producto y producción.",
		Tables:      []string{"cost_timesheet", "labor_grade", "product_recipe", "product_yield", "cost_center", "process_route", "import_cost", "price_list", "product_price_list", "product_cost_config", "cost_component", "cost_snapshot", "cost_period_close", "financial_year"},
	},
	{
		Key:         "rrhh",
		Label:       "Recursos Humanos y Nómina",
		Description: "Empleados, puestos, departamentos, asistencia, vacaciones, salarios, corridas de nómina y recibos de pago. Para informes de personal, nómina y RRHH.",
		Tables:      []string{"hr_department", "hr_position", "hr_employee", "person_information", "person_ledger", "designation", "attendance", "hr_attendance", "hr_leave", "hr_salary_assignment", "hr_payroll_run", "hr_payslip", "hr_payroll_concept", "hr_payroll_item", "employee_salary_setup", "employee_salary_payment", "employee_history", "personal_loan", "pesonal_loan_information", "salary_setup_header", "salary_sheet_generate", "salary_type", "payroll_tax_setup"},
	},
	{
		Key:         "fiscal",
		Label:       "Retenciones e Impuestos Legales",
		Description: "Retenciones de IVA, ISLR, percepciones IGTF. Para informes fiscales y libros legales de Venezuela.",
		Tables:      []string{"vat_retention", "islr_retention", "igtf_transaction"},
	},
}

// aiSchemaModuleMap indexa los módulos por clave.
var aiSchemaModuleMap map[string]aiSchemaModule

func init() {
	aiSchemaModuleMap = make(map[string]aiSchemaModule, len(aiSchemaModules))
	for _, m := range aiSchemaModules {
		aiSchemaModuleMap[m.Key] = m
	}
}

// aiSchemaModulesPromptText genera el texto compacto de la fase de clasificación:
// nombre + descripción de cada módulo, sin columnas ni tipos. ~1.5K tokens.
func aiSchemaModulesPromptText() string {
	var b strings.Builder
	b.WriteString("MODULOS DE NEGOCIO disponibles en la base de datos ERPGo (cada uno agrupa tablas afines). Para crear el informe, determina cuales modulos son necesarios:\n\n")
	for _, m := range aiSchemaModules {
		fmt.Fprintf(&b, "- %s (%s): %s\n", m.Key, m.Label, m.Description)
	}
	b.WriteString("\nResponde SOLO con las claves de los modulos necesarios, separados por comas (ej: ventas,cxc). Si no estas seguro, incluye los probables; es peor omitir uno que sobrar.")
	return b.String()
}

// aiSchemaTablesForModules devuelve la lista de tablas de un conjunto de módulos.
// Las tablas compartidas (p.ej. product_information) aparecen una vez. Las claves
// desconocidas se ignoran sin error.
func aiSchemaTablesForModules(moduleKeys []string) []string {
	seen := map[string]bool{}
	var tables []string
	// Las "tablas transversales" (productos, clientes) van siempre que haya
	// cualquier módulo de negocio, porque son el pegamento entre dominios.
	hasBusiness := false
	for _, k := range moduleKeys {
		m, ok := aiSchemaModuleMap[k]
		if !ok {
			continue
		}
		if k != "contabilidad" && k != "fiscal" {
			hasBusiness = true
		}
		for _, t := range m.Tables {
			if !seen[t] {
				seen[t] = true
				tables = append(tables, t)
			}
		}
	}
	if hasBusiness {
		shared := []string{"product_information", "product_category", "customer_information", "supplier_information"}
		for _, t := range shared {
			if !seen[t] {
				seen[t] = true
				tables = append(tables, t)
			}
		}
	}
	sort.Strings(tables)
	return tables
}

// aiSchemaModulesFromText parsea la respuesta de la IA (claves separadas por
// comas/espacios) y devuelve solo las que existen en el catálogo.
func aiSchemaModulesFromText(text string) []string {
	raw := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '\t'
	})
	keys := make([]string, 0, len(raw))
	for _, p := range raw {
		k := strings.ToLower(strings.TrimSpace(p))
		if _, ok := aiSchemaModuleMap[k]; ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// aiSchemaMapForModules construye el mapa de esquema detallado (columnas, tipos,
// PK, FK, joins) SOLO para las tablas de los módulos indicados. Es la Fase 2:
// después de que la IA clasificó, le entregamos solo lo que necesita.
func (s *Server) aiSchemaMapForModules(sess *SessionData, moduleKeys []string) string {
	tables := aiSchemaTablesForModules(moduleKeys)
	if len(tables) == 0 {
		// Fallback: si la clasificación no devolvió nada válido, mapa completo.
		return s.aiSchemaMap(sess)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Base de datos ERPGo: %d tablas relevantes para este informe (de los modulos: %s). Cada consulta debe ceñirse a estas tablas.\n\n",
		len(tables), strings.Join(moduleKeys, ", "))
	for _, t := range tables {
		if line := s.aiTableSummary(sess, t); line != "" {
			b.WriteString(line)
		}
	}
	return b.String()
}

// allModuleKeys devuelve las claves de todos los módulos del catálogo. Es el
// "todo el mapa": el fallback cuando nadie dice qué módulos usar (o cuando la
// clasificación no devolvió nada válido).
func allModuleKeys() []string {
	keys := make([]string, 0, len(aiSchemaModules))
	for _, m := range aiSchemaModules {
		keys = append(keys, m.Key)
	}
	return keys
}
