package web

// ai_schema_introspect.go — Estructura de la base de datos LEÍDA de MySQL y
// entregada a los agentes en JSON, scopada por departamento y por rol.
//
// Por qué este archivo existe. Antes el mapa técnico de la BD se escribía a mano
// en ai_schema_catalog.go: una lista fija de tablas con su descripción y la lista
// de columnas la sacaba el propio código con una consulta por tabla. Tres
// problemas, todos reales:
//
//   - Una tabla nueva no aparece hasta que alguien la escriba a mano. El agente
//     recibe una lista de 40 tablas de una base que tiene 139, y el modelo
//     concluye que la tabla que necesita no existe. No es que se la invente:
//     es que nadie le dijo que existe.
//   - Las columnas se leían de information_schema de a una en una, en un bucle,
//     por cada llamada al modelo. Con 40 tablas son 40 viajes de red en el peor
//     camino, y el prompt resultante cambiaba de forma si una consulta fallaba
//     en silencio (aiColumnList hacía `continue` y devolvía lo que{SO)).
//   - No había noción de departamento. Un agente de cuentas por cobrar recibía el
//     mapa completo: contabilidad, RRHH, nomina. Enorme, caro, y encima le
//     enseñaba la estructura exacta de tablas que no le tocan.
//
// Lo que hace este archivo:
//
//	1. Una sola pasada de consultas a information_schema (5 consultas en total,
//	   no 40) que lee tablas, columnas, índices y claves foráneas de la base
//	   CONECTADA (la de la empresa de la sesión), nunca de todo el servidor.
//	2. Un JSON compacto y determinista con lo que el modelo necesita para
//	   escribir un SELECT correcto: nombre, tipo, nulabilidad, clave primaria,
//	   foráneas y la descripción de negocio.
//	3. Un scopado por departamento (fac, cxc, cxp, contb, rrhh, inv, it) para que
//	   un agente solo reciba la estructura de su área.
//	4. Una caché por empresa con invalidación por huella: si cambia el esquema
//	   (una migración), la huella cambia y el mapa se regenera. Sin eso habría que
//   elegir entre pagar la introspección en cada llamada o servir un mapa
//  -mentira indefinidamente.
//
// Lo que NO hace: no inventa nada. Si information_schema no devuelve una tabla,
// la tabla no está en el JSON. La descripción de negocio sí está escrita a mano
// en ai_schema_catalog.go, y solo se le añade a las tablas que además existen de
// verdad en la base.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Estructuras del JSON entregado a la IA
// ---------------------------------------------------------------------------

// ColumnaEsquema describe una columna tal y como la ve el modelo.
type ColumnaEsquema struct {
	Nombre     string `json:"nombre"`
	Tipo       string `json:"tipo"`
	Nullable   bool   `json:"nullable,omitempty"`
	Defecto    string `json:"default,omitempty"`
	Clave      string `json:"clave,omitempty"` // PK | FK | MUL | "" (índice normal)
	Autoinc    bool   `json:"autoinc,omitempty"`
	Comentario string `json:"comentario,omitempty"`
}

// TablaEsquema describe una tabla con lo mínimo que hace falta para consultarla.
type TablaEsquema struct {
	Nombre      string           `json:"nombre"`
	Descripcion string           `json:"descripcion,omitempty"`
	EsVista     bool             `json:"es_vista,omitempty"`
	Columnas    []ColumnaEsquema `json:"columnas"`
	PK          []string         `json:"pk,omitempty"`
	Foranea     []ClaveForanea   `json:"fk,omitempty"`
}

// ClaveForanea es una FK declarada. El ERP casi no declara FKs (las define en la
// aplicación, no en el DDL), así que esto suele venir vacío; cuando viene, vale.
type ClaveForanea struct {
	Columna    string `json:"columna"`
	Referencia string `json:"referencia"` // tabla.columna
}

// EsquemaEmpresa es el JSON completo que se entrega a un agente.
type EsquemaEmpresa struct {
	Base         string            `json:"base"`
	Departamento string            `json:"departamento,omitempty"`
	Generado     string            `json:"generado"`
	Tablas       []TablaEsquema    `json:"tablas"`
	Joins        map[string]string `json:"joins,omitempty"`
	// Huella identifica el esquema: si cambia, el mapa se regenera.
	Huella string `json:"huella"`
}

// ---------------------------------------------------------------------------
// Departamento: el scopado que decide qué ve el agente
// ---------------------------------------------------------------------------

// Los departamentos que existen en el ERP. Un agente pertenece a uno (o a
// varios, si su rol los cruza) y solo recibe las tablas de esos departamentos.
// 'it' es el departamento de soporte y desarrollo: es el ÚNICO que ve el mapa
// completo, porque es el que diseña informes y necesita conocer la base entera.
const (
	DeptIT     = "it"
	DeptVentas = "fac"
	DeptCXC    = "cxc"
	DeptCXP    = "cxp"
	DeptContab = "contb"
	DeptRRHH   = "rrhh"
	DeptInv    = "inv"
)

// aiDepartamentos es la lista ordenada de departamentos recognized. El orden es
// fijo a propósito: el JSON es determinista y los tests pueden compararlo.
var aiDepartamentos = []string{DeptIT, DeptVentas, DeptCXC, DeptCXP, DeptContab, DeptRRHH, DeptInv}

// aiDeptLabel es el nombre para mostrar.
var aiDeptLabel = map[string]string{
	DeptIT:     "Soporte y Desarrollo",
	DeptVentas: "Facturación",
	DeptCXC:    "Cuentas por Cobrar",
	DeptCXP:    "Cuentas por Pagar",
	DeptContab: "Contabilidad",
	DeptRRHH:   "Recursos Humanos",
	DeptInv:    "Inventario",
}

// aiDeptDeTabla asigna cada tabla de negocio a los departamentos que la usan.
// Tabla fuera de este mapa: existe en la base pero solo la ve IT.
//
// Es una decisión de negocio, no técnica: si mañana el área de compras necesita
// una tabla que hoy solo usa contabilidad, se añade aquí y el agente de compras
// la empieza a ver sin tocar código de la IA.
var aiDeptDeTabla = map[string][]string{
	// --- Facturación / ventas ---
	"invoice":         {DeptVentas, DeptCXC, DeptContab, DeptInv},
	"invoice_details": {DeptVentas, DeptCXC, DeptInv},
	"quotation":       {DeptVentas}, "service_invoice": {DeptVentas, DeptCXC}, "product_service": {DeptVentas, DeptCXC},
	"product_return":       {DeptVentas, DeptInv},
	"customer_information": {DeptVentas, DeptCXC},
	"credit_note":          {DeptVentas, DeptCXC},
	"credit_note_details":  {DeptVentas, DeptCXC},
	"tax_collection":       {DeptVentas, DeptContab},
	"vat_retention":        {DeptVentas, DeptContab},
	"islr_retention":       {DeptVentas, DeptContab},
	"igtf_transaction":     {DeptVentas, DeptContab},
	"payment_method":       {DeptVentas, DeptCXC},
	"product_information":  {DeptVentas, DeptInv},
	"product_category":     {DeptVentas, DeptInv},
	"exchange_rate":        {DeptVentas, DeptContab},
	"exchange_rate_daily":  {DeptVentas, DeptContab},
	"exchange_rate_source": {DeptVentas, DeptContab}, // --- Cuentas por cobrar ---
	//
	// `invoice_payment_summary`, `product_service` y `service_invoice` entraron
	// aquí por una revisión, no de serie. Las tres son de FAC y estaban solo
	// ahí, pero un agente de cobranza no puede informar de la cartera si no ve
	// el resumen de pagos por factura ni las facturas de servicio que se deben.
	// Son las tablas que un área necesita para responder "¿quién me debe?".
	"cash_session":         {DeptCXC},
	"cash_session_payment": {DeptCXC},
	"bank_information":     {DeptCXC, DeptCXP},
	"bank_transaction":     {DeptCXC, DeptCXP},
	"acc_subcode":          {DeptCXC, DeptCXP, DeptContab},

	// --- Cuentas por pagar ---
	"product_purchase":         {DeptCXP, DeptInv},
	"product_purchase_details": {DeptCXP, DeptInv},
	"supplier_information":     {DeptCXP},
	"expense":                  {DeptCXP, DeptContab},
	"expense_item":             {DeptCXP, DeptContab}, // --- Contabilidad ---
	"acc_coa":                  {DeptContab},
	"acc_transaction":          {DeptContab},
	"acc_vaucher":              {DeptContab},
	"acc_subtype":              {DeptContab},
	"acc_monthly_balance":      {DeptContab},
	"acc_opening_balance":      {DeptContab},
	"acc_period_close":         {DeptContab},
	"closing_records":          {DeptContab},
	"coa_catalog":              {DeptContab},
	"financial_year":           {DeptContab},
	"invoice_payment_summary":  {DeptContab, DeptCXC},
	"voucher_no_setting":       {DeptContab}, // --- Inventario ---
	"warehouse":                {DeptInv},
	"store_adjustment":         {DeptInv},
	"store_adjustment_details": {DeptInv},
	"product_lot":              {DeptInv},
	"expiry_alert_log":         {DeptInv},
	"cycle_count":              {DeptInv},
	"cycle_count_line":         {DeptInv},
	"cycle_count_adjustment":   {DeptInv},
	"reorder_policy":           {DeptInv},
	"reorder_suggestion":       {DeptInv},
	"product_daily_demand":     {DeptInv},
	"supplier_product":         {DeptInv, DeptCXP},

	// --- RRHH ---
	//
	// Los NOMBRES SON LOS REALES de migrations/schema_z_hr.sql, no los que
	// sugiere la intuición. Antes se listaba hr_salary, hr_employee_type,
	// hr_pay_scale... que no existen: ningún agente de RRHH recibía ninguna
	// tabla (medido: 0 tablas, 67 bytes de JSON). El nombre correcto es
	// hr_salary_assignment, y el de la nómina es person_information, no
	// hr_employee_type.
	"hr_department":            {DeptRRHH},
	"hr_position":              {DeptRRHH},
	"hr_employee":              {DeptRRHH},
	"person_information":       {DeptRRHH},
	"person_ledger":            {DeptRRHH},
	"designation":              {DeptRRHH},
	"attendance":               {DeptRRHH},
	"hr_attendance":            {DeptRRHH},
	"hr_leave":                 {DeptRRHH},
	"hr_salary_assignment":     {DeptRRHH},
	"hr_payroll_run":           {DeptRRHH},
	"hr_payslip":               {DeptRRHH},
	"hr_payroll_concept":       {DeptRRHH},
	"hr_payroll_item":          {DeptRRHH},
	"employee_salary_setup":    {DeptRRHH},
	"employee_salary_payment":  {DeptRRHH},
	"employee_history":         {DeptRRHH},
	"personal_loan":            {DeptRRHH},
	"pesonal_loan_information": {DeptRRHH},
	"salary_setup_header":      {DeptRRHH},
	"salary_sheet_generate":    {DeptRRHH},
	"salary_type":              {DeptRRHH},
	"payroll_tax_setup":        {DeptRRHH},

	// --- Sistema de costos ---
	//
	// Asignadas a partir de quién USA el dato, no de dónde vive la tabla. Una
	// receta es de inventario pero su costo es de contabilidad; las horas son de
	// RRHH y también del costo. Cada una va donde la necesita quien decide, no
	// donde la creó la migración.
	//
	// `price_list` y `product_price_list` las ve ventas porque es donde se
	// elige al mayor o al detalle. Si no, un agente de ventas no podría decirle
	// al dueño qué precio está aplicando.
	"cost_timesheet":      {DeptRRHH, DeptContab},
	"labor_grade":         {DeptRRHH, DeptContab},
	"product_recipe":      {DeptInv, DeptContab},
	"product_yield":       {DeptInv, DeptContab},
	"process_route":       {DeptInv, DeptContab},
	"product_cost_config": {DeptInv, DeptContab},
	"import_cost":         {DeptInv, DeptContab, DeptCXP},
	"price_list":          {DeptVentas, DeptContab},
	"product_price_list":  {DeptVentas, DeptContab},
	"cost_center":         {DeptContab},
	"cost_component":      {DeptContab},
	"cost_snapshot":       {DeptContab},
	"cost_period_close":   {DeptContab},
}

// aiDeptEsValido indica si un departamento existe.
func aiDeptEsValido(dept string) bool {
	d := strings.ToLower(strings.TrimSpace(dept))
	for _, k := range aiDepartamentos {
		if k == d {
			return true
		}
	}
	return false
}

// aiDeptNormalizar vacía → IT. Es el default: un agente sin departamento
// declarado ve el mapa entero, que es lo que corresponde a soporte y desarrollo.
// Cualquier otro valor que no exista cae en IT en vez de devolver un mapa vacío,
// porque un mapa vacío hace que el modelo concluya que la base no tiene tablas.
func aiDeptNormalizar(dept string) string {
	d := strings.ToLower(strings.TrimSpace(dept))
	if d == "" {
		return DeptIT
	}
	if aiDeptEsValido(d) {
		return d
	}
	return DeptIT
}

// aiDeptTablas devuelve el conjunto de tablas visibles para un departamento,
// en orden alfabético y determinista.
func aiDeptTablas(dept string) []string {
	d := aiDeptNormalizar(dept)
	names := make([]string, 0, len(aiDeptDeTabla))
	for tabla, depts := range aiDeptDeTabla {
		if d == DeptIT {
			// IT ve todas las tablas de negocio declaradas, más las que solo
			// existen en la base (que se añaden al leerla).
			names = append(names, tabla)
			continue
		}
		for _, x := range depts {
			if x == d {
				names = append(names, tabla)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

// aiDeptEsVisible indica si un departamento puede ver una tabla.
//
// DOS filtros, y los dos importan:
//
//	El primero es la WHITELIST (aiCatalogWhitelist). Sin él, "IT ve todo" se
//	convierte en "la IA lee user_login, users, ai_providers y ai_agent", que es
//	justo lo que la whitelist existía para impedir. Ese bug estuvo aquí: el
//	scopado de IT devolvía las 139 tablas de la base.
//
//	El segundo es la asignación por departamento.
func aiDeptEsVisible(dept, tabla string) bool {
	d := aiDeptNormalizar(dept)
	t := strings.ToLower(strings.TrimSpace(tabla))
	if !aiCatalogIsVisible(t) {
		return false
	}
	if d == DeptIT {
		return true
	}
	for _, x := range aiDeptDeTabla[t] {
		if x == d {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Introspección: una pasada de information_schema
// ---------------------------------------------------------------------------

// huellaDeEsquema resume el esquema de la base conectada en una cadena corta.
// Es lo que decide si la caché sirve o regenera.
//
// BIT_XOR(CRC32(...)) sobre todas las columnas: si se añade, quita o cambia de
// tipo una columna en cualquier tabla, la huella cambia. Cuesta una sola consulta
// y es barato comparado con las otras cuatro, así que se puede preguntar en cada
// llamada sin notar nada.
func huellaDeEsquema(db *sql.DB) (string, int, error) {
	const q = `SELECT COUNT(*), COALESCE(BIT_XOR(CRC32(CONCAT(table_name,'|',column_name,'|',column_type,'|',ordinal_position))),0)
		FROM information_schema.columns WHERE table_schema = DATABASE()`
	var n int
	var h uint64
	if err := db.QueryRow(q).Scan(&n, &h); err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%d-%016x", n, h), n, nil
}

// leerTablasBase devuelve nombre, comentario y si es vista, para TODAS las
// tablas de la base conectada.
func leerTablasBase(db *sql.DB) (map[string]TablaEsquema, error) {
	const q = `SELECT table_name, IFNULL(table_comment,''), table_type
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		ORDER BY table_name`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tablas := map[string]TablaEsquema{}
	for rows.Next() {
		var nombre, comentario, tipo string
		if err := rows.Scan(&nombre, &comentario, &tipo); err != nil {
			continue
		}
		tablas[strings.ToLower(nombre)] = TablaEsquema{
			Nombre:      nombre,
			Descripcion: "",
			EsVista:     strings.EqualFold(tipo, "VIEW"),
		}
		_ = comentario
	}
	return tablas, rows.Err()
}

// leerColumnasBase lee TODAS las columnas de TODAS las tablas en una sola
// consulta. Es el cambio de fondo: antes era una consulta por tabla dentro de un
// bucle que se ejecutaba en cada llamada al modelo.
func leerColumnasBase(db *sql.DB) (map[string][]ColumnaEsquema, map[string][]string, error) {
	const q = `SELECT table_name, column_name, column_type, is_nullable,
		IFNULL(column_default,''), column_key, IFNULL(extra,''), IFNULL(column_comment,'')
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		ORDER BY table_name, ordinal_position`
	rows, err := db.Query(q)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols := map[string][]ColumnaEsquema{}
	pks := map[string][]string{}
	for rows.Next() {
		var tabla, col, colType, nullable, defecto, key, extra, comentario string
		if err := rows.Scan(&tabla, &col, &colType, &nullable, &defecto, &key, &extra, &comentario); err != nil {
			continue
		}
		t := strings.ToLower(tabla)
		ce := ColumnaEsquema{
			Nombre:     col,
			Tipo:       strings.ToLower(colType),
			Nullable:   strings.EqualFold(nullable, "YES"),
			Clave:      key,
			Autoinc:    strings.Contains(strings.ToLower(extra), "auto_increment"),
			Comentario: comentario,
		}
		// El DEFECTO de MySQL para columnas DATE/TIMESTAMP suele venir como
		// CURRENT_TIMESTAMP: es útil, pero los literales de texto con comillas
		// sobran y engordan el prompt. Se deja solo lo que el modelo puede usar.
		if defecto != "" && !strings.EqualFold(defecto, "NULL") {
			ce.Defecto = defecto
		}
		cols[t] = append(cols[t], ce)
		if key == "PRI" {
			pks[t] = append(pks[t], col)
		}
	}
	return cols, pks, rows.Err()
}

// leerForaneasBase lee las FK declaradas. Suele devolver poco (el ERP las
// resuelve en la aplicación), pero cuando hay algo vale.
func leerForaneasBase(db *sql.DB) (map[string][]ClaveForanea, error) {
	const q = `SELECT table_name, column_name,
		IFNULL(referenced_table_name,''), IFNULL(referenced_column_name,'')
		FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE() AND referenced_table_name IS NOT NULL
		ORDER BY table_name, column_name`
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fks := map[string][]ClaveForanea{}
	for rows.Next() {
		var tabla, col, refTabla, refCol string
		if err := rows.Scan(&tabla, &col, &refTabla, &refCol); err != nil {
			continue
		}
		t := strings.ToLower(tabla)
		fks[t] = append(fks[t], ClaveForanea{
			Columna:    col,
			Referencia: strings.ToLower(refTabla) + "." + strings.ToLower(refCol),
		})
	}
	return fks, rows.Err()
}

// introspectarBase hace las 4 lecturas y devuelve el esquema completo, sin
// scopar. El scopado se hace después, sobre el resultado.
func introspectarBase(db *sql.DB) (*EsquemaEmpresa, error) {
	tablas, err := leerTablasBase(db)
	if err != nil {
		return nil, fmt.Errorf("leyendo tablas: %w", err)
	}
	cols, pks, err := leerColumnasBase(db)
	if err != nil {
		return nil, fmt.Errorf("leyendo columnas: %w", err)
	}
	fks, err := leerForaneasBase(db)
	if err != nil {
		return nil, fmt.Errorf("leyendo claves foráneas: %w", err)
	}
	huella, _, err := huellaDeEsquema(db)
	if err != nil {
		return nil, fmt.Errorf("calculando huella: %w", err)
	}

	out := &EsquemaEmpresa{
		Base:     "",
		Generado: time.Now().UTC().Format(time.RFC3339),
		Huella:   huella,
		Joins:    map[string]string{},
	}
	// El nombre de la base se toma de la conexión, no de una variable: la
	// conexión ES la base de la empresa de la sesión.
	_ = db.QueryRow("SELECT DATABASE()").Scan(&out.Base)

	nombres := make([]string, 0, len(tablas))
	for t := range tablas {
		nombres = append(nombres, t)
	}
	sort.Strings(nombres)

	for _, t := range nombres {
		// El filtro de whitelist va AQUÍ, no en el scopado. Que sea el único
		// punto por el que se lee la base evita que un camino nuevo que use el
		// introspector se quede con la lista completa por olvido. Es la
		// diferencia entre que la tabla de credenciales no salga porque nadie
		// pensó en quitarla, y que no salga porque no se puede sacar.
		if !aiCatalogIsVisible(t) {
			continue
		}
		te := tablas[t]
		te.Columnas = cols[t]
		te.PK = pks[t]
		te.Foranea = fks[t]
		if desc, ok := aiCatalogWhitelist[t]; ok {
			te.Descripcion = desc
		}
		if te.Descripcion == "" && te.EsVista {
			te.Descripcion = "Vista de la base de datos."
		}
		out.Tablas = append(out.Tablas, te)
	}
	for tabla, join := range aiCatalogRelations {
		out.Joins[tabla] = join
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Scopado por departamento
// ---------------------------------------------------------------------------// Es scopar un esquema ya leído a un departamento.
//
// Lo que sale de aquí es lo que ve la IA, así que aplica el filtro de whitelist
// además del de departamento (aiDeptEsVisible ya lo hace). El introspector leyo
// TODAS las tablas de la base a propósito —así la vista de soporte ve el
// esquema real completo— pero de aquí solo pasa lo autorizado.
func (e *EsquemaEmpresa) scopar(dept string) *EsquemaEmpresa {
	d := aiDeptNormalizar(dept)
	out := &EsquemaEmpresa{
		Base:         e.Base,
		Departamento: d,
		Generado:     e.Generado,
		Huella:       e.Huella,
		Joins:        map[string]string{},
	}
	for _, t := range e.Tablas {
		if !aiDeptEsVisible(d, t.Nombre) {
			continue
		}
		out.Tablas = append(out.Tablas, t)
		if j, ok := e.Joins[strings.ToLower(t.Nombre)]; ok {
			out.Joins[strings.ToLower(t.Nombre)] = j
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Caché por empresa, invalidada por huella
// ---------------------------------------------------------------------------

type aiEsquemaCacheEntry struct {
	esquema *EsquemaEmpresa
	huella  string
	leidoEn time.Time
}

// aiEsquemaMu protege aiEsquemaCache. Un RWMutex propio y no el de las
// sesiones: el ciclo de vida es distinto y un candado compartido con el login
// sería un deadlock en cuanto un login y una introspección coincidieran.
var aiEsquemaMu sync.RWMutex

// aiEsquemaCache indexa por CÓDIGO DE EMPRESA, no por *sql.DB: si el pool de una
// empresa se cierra y se reabre (reinicio, cambio de empresa), la caché sigue
// siendo válida porque lo que cambió fue el handle, no el esquema.
var aiEsquemaCache = map[string]*aiEsquemaCacheEntry{}

// aiEsquemaTTL es cuánto se fía de la huella sin volver a preguntar. La pregunta
// por la huella es una consulta de agregación sobre information_schema: con el
// sql_mode de este servidor es de milisegundos, pero tampoco tiene sentido
// pagarla en cada paso de un bucle de agente.
const aiEsquemaTTL = 30 * time.Second

// invalidarEsquemaCache se llama tras una migración.
func invalidarEsquemaCache() {
	aiEsquemaMu.Lock()
	aiEsquemaCache = map[string]*aiEsquemaCacheEntry{}
	aiEsquemaMu.Unlock()
}

// obtenerEsquemaPara devuelve el esquema íntegro de la base de una sesión, con
// caché por huella.
func (s *Server) obtenerEsquemaPara(sess *SessionData) *EsquemaEmpresa {
	return s.obtenerEsquemaEmpresa(s.tenantOfSession(sess))
}

// obtenerEsquemaEmpresa es obtenerEsquemaPara con la empresa explícita.
func (s *Server) obtenerEsquemaEmpresa(tenant string) *EsquemaEmpresa {
	db := s.dbForTenant(tenant)
	if db == nil {
		return &EsquemaEmpresa{Generado: time.Now().UTC().Format(time.RFC3339)}
	}

	huella, _, err := huellaDeEsquema(db)
	if err != nil {
		// Si ni la huella se puede leer, no hay nada fiable que servir. Se
		// devuelve vacío y el llamador lo sabe: no se sirve un mapa viejo
		// fingiendo que es actual.
		return &EsquemaEmpresa{Base: tenant, Generado: time.Now().UTC().Format(time.RFC3339)}
	}

	aiEsquemaMu.RLock()
	entrada := aiEsquemaCache[tenant]
	aiEsquemaMu.RUnlock()
	if entrada != nil && entrada.huella == huella && time.Since(entrada.leidoEn) < aiEsquemaTTL {
		return entrada.esquema
	}

	esquema, err := introspectarBase(db)
	if err != nil {
		if entrada != nil && entrada.huella == huella {
			// La introspección falló pero el esquema no ha cambiado: lo que
			// hay en caché sigue siendo cierto. Servirlo es mejor que dejar
			// al agente sin mapa.
			return entrada.esquema
		}
		return &EsquemaEmpresa{Base: tenant, Generado: time.Now().UTC().Format(time.RFC3339)}
	}

	aiEsquemaMu.Lock()
	aiEsquemaCache[tenant] = &aiEsquemaCacheEntry{esquema: esquema, huella: huella, leidoEn: time.Now()}
	aiEsquemaMu.Unlock()
	return esquema
}

// ---------------------------------------------------------------------------
// Render: el JSON que se entrega a la IA
// ---------------------------------------------------------------------------

// aiEsquemaJSONCompact es la forma que va al prompt del agente. NO es el JSON
// crudo de Go (que incluye nulabilidad y defecto de cada columna, y sale de
// 40 KB a 120 KB según la empresa): es una versión donde cada tabla es una
// línea de columnas, y solo se incluyen los campos que cambian cómo se escribe
// una consulta.
//
// Tres reglas que parecen arbitrarias pero no lo son:
//   - Nullable y default NO van. El modelo no necesita saber si una columna
//     acepta NULL para escribir un SELECT.
//   - autoincrement NO va. Es metadato de escritura, y este agente escribe
//     plantillas de informe, no filas.
//   - La descripción de negocio SÍ va. Es la diferencia entre que el modelo
//     adivine qué hace acc_vaucher y sepa que Vtype distingue JV de CV de DV.
func (e *EsquemaEmpresa) aiEsquemaJSONCompact() string {
	var sb strings.Builder
	sb.WriteString("{")
	sb.WriteString(fmt.Sprintf(`"base":%q,`, e.Base))
	if e.Departamento != "" {
		sb.WriteString(fmt.Sprintf(`"departamento":%q,`, e.Departamento))
	}
	sb.WriteString(`"tablas":[`)
	for i, t := range e.Tablas {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"tabla":`)
		sb.WriteString(quoteJSON(t.Nombre))
		if t.Descripcion != "" {
			sb.WriteString(`,"descripcion":`)
			sb.WriteString(quoteJSON(t.Descripcion))
		}
		if t.EsVista {
			sb.WriteString(`,"vista":true`)
		}
		sb.WriteString(`,"columnas":{`)
		for j, c := range t.Columnas {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(quoteJSON(c.Nombre))
			sb.WriteString(":")
			sb.WriteString(quoteJSON(aiTipoCorto(c.Tipo)))
		}
		sb.WriteString("}")
		if len(t.PK) > 0 {
			sb.WriteString(`,"pk":[`)
			for k, p := range t.PK {
				if k > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(quoteJSON(p))
			}
			sb.WriteString("]")
		}
		if len(t.Foranea) > 0 {
			sb.WriteString(`,"fk":{`)
			for k, f := range t.Foranea {
				if k > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(quoteJSON(f.Columna))
				sb.WriteString(":")
				sb.WriteString(quoteJSON(f.Referencia))
			}
			sb.WriteString("}")
		}
		if j, ok := e.Joins[strings.ToLower(t.Nombre)]; ok && j != "" {
			sb.WriteString(`,"join":`)
			sb.WriteString(quoteJSON(j))
		}
		sb.WriteString("}")
	}
	sb.WriteString("]}")
	return sb.String()
}

// quoteJSON escapa una cadena para JSON sin pasar por encoding/json en cada
// llamada (aquí son cientos de columnas). El juego de caracteres es utf8mb4 con
// acentos en los comentarios, y hay que escaparlos igual.
func quoteJSON(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// aiTipoCorto reduce el tipo declarado por MySQL a lo que el modelo necesita
// para escribir la consulta. "decimal(10,2) unsigned" → "decimal", "int(11)" →
// "int". El tamaño lo pone el DDL, no la consulta.
func aiTipoCorto(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "unsigned"))
	switch t {
	case "integer", "int4", "mediumint":
		return "int"
	case "bigint":
		return "bigint"
	case "tinyint":
		return "tinyint"
	case "character", "char", "varchar":
		return "text"
	case "longtext", "mediumtext", "tinytext":
		return "text"
	case "double", "double precision", "real":
		return "double"
	case "float", "decimal", "numeric":
		return "decimal"
	}
	return t
}

// aiEsquemaJSONMarshal devuelve el JSONIndent completo (para la UI y para
// descargar). No va al prompt.
func (e *EsquemaEmpresa) aiEsquemaJSONMarshal() string {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Fachadas
// ---------------------------------------------------------------------------

// aiEsquemaParaAgente devuelve el JSON scopado al departamento del agente.
// Es lo que se antepone al prompt de configuración del agente.
func (s *Server) aiEsquemaParaAgente(sess *SessionData, dept string) string {
	full := s.obtenerEsquemaPara(sess)
	return full.scopar(aiDeptNormalizar(dept)).aiEsquemaJSONCompact()
}

// aiEsquemaGlobalCompact devuelve el mapa entero sin scopar. Es lo que usan las
// herramientas de soporte y desarrollo (diseño de informes).
func (s *Server) aiEsquemaGlobalCompact(sess *SessionData) string {
	full := s.obtenerEsquemaPara(sess)
	return full.scopar(DeptIT).aiEsquemaJSONCompact()
}

// aiDepartamentosDeRol decide el scopado a partir del NOMBRE del rol del agente.
// La tabla es explícita: un rol sin entrada ve el mapa de IT, que es el más
// amplio, porque es el que más gente necesita. El nombre del rol es la clave
// natural aquí porque es lo que el administrador elige al crear el agente.
var aiDepartamentosDeRol = map[string][]string{
	"soporte":            {DeptIT},
	"it":                 {DeptIT},
	"desarrollo":         {DeptIT},
	"developer":          {DeptIT},
	"administrador":      {DeptIT},
	"admin":              {DeptIT},
	"facturacion":        {DeptVentas},
	"facturador":         {DeptVentas},
	"vendedor":           {DeptVentas},
	"cxc":                {DeptCXC},
	"cuentas por cobrar": {DeptCXC},
	"cobrador":           {DeptCXC},
	"cxp":                {DeptCXP},
	"cuentas por pagar":  {DeptCXP},
	"pagar":              {DeptCXP},
	"contabilidad":       {DeptContab},
	"contador":           {DeptContab},
	"rrhh":               {DeptRRHH},
	"recursos humanos":   {DeptRRHH},
	"inventario":         {DeptInv},
	"almacen":            {DeptInv},
}

// aiDeptDeRolNombre devuelve los departamentos de un rol por nombre.
// Rol desconocido o vacío → IT.
func aiDeptDeRolNombre(rolNombre string) string {
	r := strings.ToLower(strings.TrimSpace(rolNombre))
	if depts, ok := aiDepartamentosDeRol[r]; ok && len(depts) > 0 {
		return depts[0]
	}
	return DeptIT
}

// aiEsquemaDeRol combina: scopado por el departamento que le toca al rol.
func (s *Server) aiEsquemaDeRol(sess *SessionData, rolNombre string) string {
	return s.aiEsquemaParaAgente(sess, aiDeptDeRolNombre(rolNombre))
}

// fingerprintEsquema es la huella de un esquema como cadena corta, para
// comparar en tests y para logs.
func fingerprintEsquema(e *EsquemaEmpresa) string {
	h := sha256.New()
	h.Write([]byte(e.Huella))
	for _, t := range e.Tablas {
		h.Write([]byte(t.Nombre))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
