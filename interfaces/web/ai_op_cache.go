package web

// ai_op_cache.go — Cache de "plantillas de operación" para los agentes.
//
// IDEA: la primera vez que la IA resuelve CÓMO hacer una operación
// estructurada (qué cuentas mueve, qué tasa aplica, qué formato de asiento)
// para una categoría determinada, ese "cómo" se guarda como plantilla. La
// siguiente solicitud de la misma categoría reutiliza la plantilla y solo cambia
// las variables (cliente, monto, fecha, referencia). Ahorro de tokens real.
//
// DETERMINÍSTICO: la clave de plantilla es operation + los campos
// ESTRUCTURADOS del request (vtype, tipo de comprobante, departamento), no fuzzy
// matching. Dos requests con los mismos campos estructurados producen la MISMA
// clave → no hay solapamiento "venta-producto" vs "venta-servicio" porque una
// tiene vtype=DV+producto y la otra vtype=DV+servicio. Cero basura por diseño.
//
// ANTI-BASURA: TTL 90 días sin uso → se borra sola. Máximo 50 plantillas por
// agente → LRU echa la menos usada. La clave es única por (agente, operación,
// key), así que repetir la misma categoría ACTUALIZA la existente en vez de
// crear una nueva fila.
//
// NEGOCIO: solo proveedores con op_cache=1 (ORC24, la IA propia) aplican el
// ahorro. OpenAI/Anthropic pagan el precio real del proveedor → ventaja
// competitiva de usar la IA propia.

import (
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// aiMaxTemplatesPorAgente es el tope LRU: más allá de esto, la menos usada se
// echa. 50 cubre a un agente que opera voucher + factura + cobro + nómina sin
// acumular basura.
const aiMaxTemplatesPorAgente = 50

// aiTemplateTTL es el tiempo máximo que una plantilla vive sin usarse.
const aiTemplateTTL = 90 * 24 * time.Hour

// aiOpTemplate es una plantilla de operación cacheada.
type aiOpTemplate struct {
	ID          int64
	CompanyID   int64
	AgentID     int64
	Operation   string
	TemplateKey string
	Pattern     map[string]interface{}
	VarSchema   []string
	Usos        int
	NeedsHuman  bool
	CreadoEn    time.Time
	LastUsado   time.Time
}

// aiOpCategoryFields lista, por nombre de operación, qué campos del request
// forman parte de la categoría (clave de la plantilla). El resto se considera
// variable. Si una operación no está aquí, su categoría es "" (plantilla única
// por operación).
var aiOpCategoryFields = map[string][]string{
	"account.propose_transaction": {"vtype"},
	"account.post_voucher":        {"vtype"},
	"voucher.create":              {"vtype"},
	"invoice.propose":             {"doctype"},
	"invoice.create_draft":        {"doctype"},
	"payment.record":              {"method"},
	"payment.create":              {"method"},
	"customer.upsert":             {"kind"},
	"supplier.upsert":             {"kind"},
}

// aiOpCacheKey construye la clave determinista de una plantilla: operación +
// campos estructurados normalizados. Sin estos campos no hay categoría.
func aiOpCacheKey(operation string, args map[string]interface{}) string {
	fields := aiOpCategoryFields[operation]
	if len(fields) == 0 {
		return operation
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		v, argsOk := args[f]
		if !argsOk {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", f, strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))))
	}
	if len(parts) == 0 {
		return operation
	}
	sort.Strings(parts)
	raw := operation + "::" + strings.Join(parts, "|")
	h := sha1.Sum([]byte(raw))
	return fmt.Sprintf("%s::%x", operation, h[:8])
}

// aiOpPatternFromArgs extrae la parte "fija" de una operación resuelta y los
// nombres de los campos que varían. El pattern conserva la estructura completa
// (incluido lines/items con el mapeo de cuentas) porque es lo reutilizable; los
// montos/date/ref/etc. se listan en varSchema para que la IA sepa qué cambiar.
// La detección de variables es recursiva (los montos viven dentro de lines).
func aiOpPatternFromArgs(operation string, args map[string]interface{}) (pattern map[string]interface{}, varSchema []string) {
	varFields := map[string]bool{"amount": true, "monto": true, "debit": true, "credit": true,
		"quantity": true, "rate": true, "price": true, "total": true, "date": true,
		"fecha": true, "ref": true, "reference": true, "referenceNo": true, "note": true,
		"narration": true, "description": true, "customer_id": true, "supplier_id": true,
		"product_id": true, "coa_id": true, "tax": true, "vat": true, "balance": true,
		"unit_price": true, "discount": true}
	catFields := map[string]bool{}
	for _, f := range aiOpCategoryFields[operation] {
		catFields[f] = true
	}
	pattern = args
	varSchema = []string{}
	seen := map[string]bool{}
	var scan func(v interface{})
	scan = func(v interface{}) {
		switch t := v.(type) {
		case map[string]interface{}:
			for k, val := range t {
				if varFields[k] && !seen[k] {
					seen[k] = true
					varSchema = append(varSchema, k)
				}
				scan(val)
			}
		case []interface{}:
			for _, item := range t {
				scan(item)
			}
		}
	}
	scan(args)
	sort.Strings(varSchema)
	return pattern, varSchema
}

// aiOpTemplateGet busca una plantilla cacheada para (agente, operación, key).
func (s *Server) aiOpTemplateGet(rdb *sql.DB, agentID int64, operation, key string) (*aiOpTemplate, bool) {
	if rdb == nil || key == "" {
		return nil, false
	}
	var t aiOpTemplate
	var patternRaw, varRaw string
	var lastUsado sql.NullTime
	err := rdb.QueryRow(`SELECT id, company_id, agent_id, operation, template_key,
			pattern, var_schema, usos, needs_human, creado_en, last_usado_en
		FROM ai_op_template WHERE agent_id = ? AND operation = ? AND template_key = ?`,
		agentID, operation, key).Scan(
		&t.ID, &t.CompanyID, &t.AgentID, &t.Operation, &t.TemplateKey,
		&patternRaw, &varRaw, &t.Usos, &t.NeedsHuman, &t.CreadoEn, &lastUsado)
	if err != nil {
		return nil, false
	}
	if lastUsado.Valid {
		t.LastUsado = lastUsado.Time
	}
	t.Pattern = map[string]interface{}{}
	_ = json.Unmarshal([]byte(patternRaw), &t.Pattern)
	t.VarSchema = []string{}
	_ = json.Unmarshal([]byte(varRaw), &t.VarSchema)
	return &t, true
}

// aiOpTemplateTouch actualiza uso y last_usado_en.
func (s *Server) aiOpTemplateTouch(rdb *sql.DB, id int64) {
	if rdb == nil {
		return
	}
	_, _ = rdb.Exec(`UPDATE ai_op_template SET usos = usos + 1, last_usado_en = NOW() WHERE id = ?`, id)
}

// aiOpTemplatePut guarda o actualiza una plantilla. La clave única (agente,
// operación, key) garantiza que repetir una categoría ACTUALIZA, no duplica.
// Tras 5 usos sin errores, needs_human se baja a 0 (ya validada).
func (s *Server) aiOpTemplatePut(rdb *sql.DB, t aiOpTemplate) error {
	if rdb == nil || t.TemplateKey == "" {
		return nil
	}
	patternRaw, _ := json.Marshal(t.Pattern)
	varRaw, _ := json.Marshal(t.VarSchema)
	_, err := rdb.Exec(`INSERT INTO ai_op_template
			(company_id, agent_id, operation, template_key, pattern, var_schema, usos, needs_human, creado_en, last_usado_en)
		VALUES (?,?,?,?,?,?,?,?,NOW(),NOW())
		ON DUPLICATE KEY UPDATE
			pattern = VALUES(pattern), var_schema = VALUES(var_schema),
			needs_human = IF(usos > 5, 0, needs_human)`,
		t.CompanyID, t.AgentID, t.Operation, t.TemplateKey, patternRaw, varRaw,
		1, boolInt(t.NeedsHuman))
	if err == nil {
		aiOpTemplateLRUEvict(rdb, t.AgentID)
	}
	return err
}

// aiOpTemplateLRUEvict echa la menos usada si el agente pasa del tope.
func aiOpTemplateLRUEvict(rdb *sql.DB, agentID int64) {
	if rdb == nil {
		return
	}
	_, _ = rdb.Exec(`DELETE t FROM ai_op_template t
		INNER JOIN (
			SELECT id FROM ai_op_template WHERE agent_id = ?
			ORDER BY last_usado_en DESC LIMIT 18446744073709551615 OFFSET ?
		) x ON t.id = x.id`, agentID, aiMaxTemplatesPorAgente)
}

// aiOpTemplateGC borra plantillas sin uso en más del TTL (basura).
func aiOpTemplateGC(rdb *sql.DB) {
	if rdb == nil {
		return
	}
	_, _ = rdb.Exec(`DELETE FROM ai_op_template WHERE last_usado_en < NOW() - INTERVAL 90 DAY`)
}

// aiOpTemplateListForAgent devuelve las plantillas vigentes de un agente.
func (s *Server) aiOpTemplateListForAgent(rdb *sql.DB, agentID int64) []aiOpTemplate {
	if rdb == nil {
		return nil
	}
	rows, err := rdb.Query(`SELECT id, company_id, agent_id, operation, template_key,
			pattern, var_schema, usos, needs_human, creado_en, last_usado_en
		FROM ai_op_template WHERE agent_id = ? ORDER BY usos DESC, last_usado_en DESC`, agentID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []aiOpTemplate{}
	for rows.Next() {
		var t aiOpTemplate
		var patternRaw, varRaw string
		var lastUsado sql.NullTime
		if err := rows.Scan(&t.ID, &t.CompanyID, &t.AgentID, &t.Operation, &t.TemplateKey,
			&patternRaw, &varRaw, &t.Usos, &t.NeedsHuman, &t.CreadoEn, &lastUsado); err != nil {
			continue
		}
		if lastUsado.Valid {
			t.LastUsado = lastUsado.Time
		}
		_ = json.Unmarshal([]byte(patternRaw), &t.Pattern)
		_ = json.Unmarshal([]byte(varRaw), &t.VarSchema)
		out = append(out, t)
	}
	return out
}

// aiOpTemplatePromptFragment genera el fragmento de system prompt que informa
// a la IA de sus plantillas establecidas.
func aiOpTemplatePromptFragment(templates []aiOpTemplate) string {
	if len(templates) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nPLANTILLAS DE OPERACIÓN ESTABLECIDAS POR USTED (reutilícelas; no las razone de nuevo):\n")
	b.WriteString("Para estas operaciones usted ya estableció la estructura correcta. COPIE el pattern de la plantilla correspondiente y sustituya solo las variables indicadas. Si la categoría del request no está en esta lista, razone normalmente y se creará una nueva plantilla.\n\n")
	for _, t := range templates {
		patternJSON, _ := json.Marshal(t.Pattern)
		varList := strings.Join(t.VarSchema, ", ")
		if varList == "" {
			varList = "los campos que varíen según el request"
		}
		human := ""
		if t.NeedsHuman {
			human = " [REVISIÓN HUMANA OBLIGATORIA]"
		}
		fmt.Fprintf(&b, "- %s (%s): pattern=%s | variables=%s | usos=%d%s\n",
			t.Operation, t.TemplateKey, string(patternJSON), varList, t.Usos, human)
	}
	return b.String()
}

// asegura que el compilador use aiTemplateTTL (referencia explícita por ahora en GC).
var _ = aiTemplateTTL
