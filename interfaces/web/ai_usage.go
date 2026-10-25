package web

// ai_usage.go — Metering de inferencia: el número que hace vendible el
// producto.
//
// Por qué existe: "paquetes de inferencia" sin un contador real son una
// promesa, no un producto. Cada llamada al proveedor deja una fila en ai_usage
// con tokens y peticiones, atribuida a empresa, agente y proveedor, para poder
// facturar por periodo y desglosar por cliente.
//
// Qué NO guarda: prompts ni respuestas. Solo contadores. Es un registro de
// facturación, no un archivo de conversaciones; el contenido que el modelo ve
// sigue efímero.
//
// Qué NO hace: cobrar. Calcula y expone el consumo; la facturación es del
// operador. Decidir precios y facturar es una decisión del negocio, no del ERP.

import (
	"database/sql"
	"strconv"
	"strings"
	"time"
)

// aiUsageEvent es una llamada ya resuelta al proveedor (con éxito o con error).
type aiUsageEvent struct {
	ProviderID int64
	Vendor     string
	Model      string
	AgentID    int64
	AgentName  string
	Source     string // "informe", "agente", "chat", ...
	Prompt     int
	Completion int
	OK         bool
}

// aiUsageTotals es el resumen de consumo de un periodo.
type aiUsageTotals struct {
	Requests       int64
	PromptTokens   int64
	CompletionToks int64
	TotalTokens    int64
	CostUSD        float64
	ByDay          []aiUsageDay
	ByProvider     []aiUsageProvider
}

// aiUsageDay es una fila del desglose diario.
type aiUsageDay struct {
	Day         string  `json:"day"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// aiUsageProvider es una fila del desglose por proveedor.
type aiUsageProvider struct {
	Vendor      string  `json:"vendor"`
	Label       string  `json:"label"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// ---------- escritura ----------

// recordAIUsage guarda una llamada al proveedor. Nunca propaga un error hacia
// el llamador: el metering no puede tumbar una petición que sí funcionó. Un
// fallo de conteo se pierde, un fallo de la IA no.
//
// La tabla puede no existir aún (BD no migrada): se ignora en silencio para no
// romper instalaciones viejas.
func (s *Server) recordAIUsage(rdb *sql.DB, e aiUsageEvent) {
	if rdb == nil || e.Source == "" {
		return
	}
	now := time.Now()
	total := e.Prompt + e.Completion
	ok := 0
	if e.OK {
		ok = 1
	}
	cost := aiEstimatedCost(e.Vendor, e.Model, e.Prompt, e.Completion)

	_, err := rdb.Exec(`INSERT INTO ai_usage
		(company_id, provider_id, vendor, model, agent_id, agent_name, source,
		 prompt_tokens, completion_tokens, total_tokens, requests,
		 estimated_cost_usd, ok, created_at, created_day)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.activeCompanyID(), e.ProviderID, e.Vendor, e.Model, e.AgentID, e.AgentName, e.Source,
		e.Prompt, e.Completion, total, 1, cost, ok,
		now.Format(nowLayout), now.Format("2006-01-02"))
	if err != nil {
		// BD sin migrar o tabla momentáneamente bloqueada: se descarta el
		// conteo, nunca la respuesta de la IA.
		return
	}
}

// activeCompanyID devuelve la empresa activa para el registro de consumo.
// El metering se escribe siempre en la base de la empresa que consume.
func (s *Server) activeCompanyID() int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(s.activeTenant), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// ---------- costes ----------

// aiModelRates son tarifas de referencia en USD por 1.000 tokens.
//
// Solo OpenAI: son las tarifas de referencia para que el operador vea el
// ahorro. ORC24 se cobra en planes, no por token, así que su coste real lo
// conoce el operador y se estima aparte (0 por defecto).
var aiModelRates = map[string]struct{ In, Out float64 }{
	"gpt-4o-mini":  {0.15, 0.60},
	"gpt-4o":       {2.50, 10.00},
	"gpt-4.1-mini": {0.40, 1.60},
	"gpt-4.1":      {2.00, 8.00},
}

// aiEstimatedCost estima el coste en USD de una llamada. Si el modelo no está
// en la tabla (o es ORC24) devuelve 0: es preferible facturar de menos que
// inventar una cifra que el cliente pueda cuestionar.
func aiEstimatedCost(vendor, model string, promptTokens, completionTokens int) float64 {
	if strings.ToLower(strings.TrimSpace(vendor)) != "openai" {
		return 0
	}
	rate, ok := aiModelRates[strings.ToLower(strings.TrimSpace(model))]
	if !ok {
		return 0
	}
	in := float64(promptTokens) / 1000 * rate.In
	out := float64(completionTokens) / 1000 * rate.Out
	return in + out
}

// ---------- lectura ----------

// aiUsageTotalsFor resume el consumo de los últimos days días.
//
// Devuelve totales vacíos si la tabla aún no existe (BD no migrada): la
// pantalla de consumo se ve, pero con ceros, en vez de romper la página.
func (s *Server) aiUsageTotalsFor(rdb *sql.DB, days int) aiUsageTotals {
	t := aiUsageTotals{}
	if rdb == nil {
		return t
	}
	if days <= 0 {
		days = 30
	}
	from := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	// Totales del periodo.
	row := rdb.QueryRow(`SELECT IFNULL(SUM(requests),0), IFNULL(SUM(prompt_tokens),0),
		IFNULL(SUM(completion_tokens),0), IFNULL(SUM(total_tokens),0),
		IFNULL(SUM(estimated_cost_usd),0)
		FROM ai_usage WHERE created_day >= ?`, from)
	_ = row.Scan(&t.Requests, &t.PromptTokens, &t.CompletionToks, &t.TotalTokens, &t.CostUSD)

	// Desglose diario (solo días con consumo).
	rows, err := rdb.Query(`SELECT created_day, IFNULL(SUM(requests),0),
		IFNULL(SUM(total_tokens),0), IFNULL(SUM(estimated_cost_usd),0)
		FROM ai_usage WHERE created_day >= ?
		GROUP BY created_day ORDER BY created_day ASC`, from)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d aiUsageDay
			if rows.Scan(&d.Day, &d.Requests, &d.TotalTokens, &d.CostUSD) == nil {
				t.ByDay = append(t.ByDay, d)
			}
		}
	}

	// Desglose por proveedor.
	prows, err := rdb.Query(`SELECT vendor, IFNULL(SUM(requests),0),
		IFNULL(SUM(total_tokens),0), IFNULL(SUM(estimated_cost_usd),0)
		FROM ai_usage WHERE created_day >= ?
		GROUP BY vendor ORDER BY SUM(requests) DESC`, from)
	if err == nil {
		defer prows.Close()
		for prows.Next() {
			var p aiUsageProvider
			if prows.Scan(&p.Vendor, &p.Requests, &p.TotalTokens, &p.CostUSD) == nil {
				p.Label = aiVendorLabel[p.Vendor]
				if p.Label == "" {
					p.Label = p.Vendor
				}
				t.ByProvider = append(t.ByProvider, p)
			}
		}
	}
	return t
}
