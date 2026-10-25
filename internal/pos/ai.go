package pos

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// AIAssistant provides intelligent guidance for POS operations
type AIAssistant struct {
	mu       sync.Mutex
	sessions map[string]*AISession
}

type AISession struct {
	ID          string
	TerminalID  string
	UserID      string
	History     []AIMessage
	LastActive  time.Time
	Context     *AIContext
}

type AIMessage struct {
	Role      string `json:"role"` // "user", "assistant", "system"
	Content   string `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type AIContext struct {
	CartSummary    string   `json:"cart_summary"`
	CurrentItems   []string `json:"current_items"`
	CustomerHistory string  `json:"customer_history"`
	Screen         string   `json:"screen"`
	Action         string   `json:"action"`
	TotalAmount    float64  `json:"total_amount"`
	CartCount      int      `json:"cart_count"`
}

type AIResponse struct {
	Advice            string   `json:"advice"`
	Action            string   `json:"action"` // "suggest", "warn", "block", "guide"
	RequireSupervisor bool     `json:"require_supervisor"`
	SuggestedProducts []string `json:"suggested_products"`
	SuggestedDiscount float64  `json:"suggested_discount"`
	Message           string   `json:"message"`
}

func NewAIAssistant() *AIAssistant {
	return &AIAssistant{
		sessions: make(map[string]*AISession),
	}
}

// GetAdvice provides AI guidance based on the current POS context
func (ai *AIAssistant) GetAdvice(terminalID, userID, query string, context *AIContext) *AIResponse {
	ai.mu.Lock()
	session := ai.getOrCreateSession(terminalID, userID)
	ai.mu.Unlock()

	// Add user message to history
	session.History = append(session.History, AIMessage{
		Role:      "user",
		Content:   query,
		Timestamp: time.Now(),
	})
	session.Context = context
	session.LastActive = time.Now()

	// Generate AI response based on context and query
	response := ai.generateResponse(session, query, context)

	// Add assistant response to history
	session.History = append(session.History, AIMessage{
		Role:      "assistant",
		Content:   response.Message,
		Timestamp: time.Now(),
	})

	return response
}

func (ai *AIAssistant) generateResponse(session *AISession, query string, context *AIContext) *AIResponse {
	query = strings.ToLower(query)

	// Product suggestions based on cart
	if strings.Contains(query, "sugerir") || strings.Contains(query, "recomendar") || strings.Contains(query, "complementar") {
		return ai.suggestProducts(session, context)
	}

	// Discount advice
	if strings.Contains(query, "descuento") || strings.Contains(query, "promocion") {
		return ai.adviseDiscount(session, context)
	}

	// Payment advice
	if strings.Contains(query, "pago") || strings.Contains(query, "cobrar") || strings.Contains(query, "dividir") {
		return ai.advisePayment(session, context)
	}

	// Refund/devolution advice
	if strings.Contains(query, "devolucion") || strings.Contains(query, "cambio") {
		return ai.adviseRefund(session, context)
	}

	// Fiscal printer issues
	if strings.Contains(query, "impresora") || strings.Contains(query, "fiscal") || strings.Contains(query, "papel") {
		return ai.adviseFiscal(session, context)
	}

	// Customer assistance
	if strings.Contains(query, "cliente") || strings.Contains(query, "ayuda") {
		return ai.adviseCustomer(session, context)
	}

	// General guidance
	return &AIResponse{
		Advice:  ai.getGeneralAdvice(context),
		Action:  "guide",
		Message: ai.getGeneralMessage(context),
	}
}

func (ai *AIAssistant) suggestProducts(session *AISession, context *AIContext) *AIResponse {
	var suggestions []string
	var message string

	// Based on current items, suggest complementary products
	if context != nil && len(context.CurrentItems) > 0 {
		// In production: query database for complementary products
		// For now, use rule-based suggestions
		for _, item := range context.CurrentItems {
			itemLower := strings.ToLower(item)
			if strings.Contains(itemLower, "pollo") || strings.Contains(itemLower, "carne") {
				suggestions = append(suggestions, "Arroz Premium 1kg", "Aceite Vegetal 1L")
				message = "Sugerencia: Los clientes que compran proteínas suelen llevar arroz y aceite. ¿Desea agregar alguno?"
			}
			if strings.Contains(itemLower, "leche") || strings.Contains(itemLower, "queso") {
				suggestions = append(suggestions, "Pan Integral", "Cereal Desayuno")
				message = "Sugerencia: Productos lácteos combinan bien con pan y cereal para desayuno."
			}
			if strings.Contains(itemLower, "pan") || strings.Contains(itemLower, "harina") {
				suggestions = append(suggestions, "Mermelada", "Mantequilla")
				message = "Sugerencia: Pan combina perfecto con mermelada o mantequilla."
			}
		}
	}

	if message == "" {
		message = "¿Puedo ayudarle con algo específico? Puedo sugerir productos, revisar descuentos, o ayudar con el pago."
	}

	return &AIResponse{
		Advice:            "suggest_products",
		Action:            "suggest",
		SuggestedProducts: suggestions,
		Message:           message,
	}
}

func (ai *AIAssistant) adviseDiscount(session *AISession, context *AIContext) *AIResponse {
	if context == nil {
		return &AIResponse{
			Advice:  "no_context",
			Action:  "guide",
			Message: "Seleccione productos antes de consultar sobre descuentos.",
		}
	}

	// High-value cart: suggest discount to close sale
	if context.TotalAmount > 1000 {
		return &AIResponse{
			Advice:            "high_value_discount",
			Action:            "suggest",
			SuggestedDiscount: 5.0,
			Message:           fmt.Sprintf("Carrito alto ($%.2f). Un descuento del 5%% puede cerrar la venta. ¿Aplicar?", context.TotalAmount),
		}
	}

	// Multiple items: suggest bundle discount
	if context.CartCount >= 5 {
		return &AIResponse{
			Advice:            "bundle_discount",
			Action:            "suggest",
			SuggestedDiscount: 3.0,
			Message:           "5+ items en carrito. Descuento por volumen del 3%% aplicable.",
		}
	}

	return &AIResponse{
		Advice:  "standard_discount",
		Action:  "guide",
		Message: "Descuento estándar disponible: hasta 10%% con autorización de supervisor.",
	}
}

func (ai *AIAssistant) advisePayment(session *AISession, context *AIContext) *AIResponse {
	if context == nil || context.TotalAmount == 0 {
		return &AIResponse{
			Advice:  "empty_cart",
			Action:  "guide",
			Message: "Carrito vacío. Agregue productos antes de procesar pago.",
		}
	}

	// Large amount: suggest splitting payment
	if context.TotalAmount > 500 {
		return &AIResponse{
			Advice:  "split_payment",
			Action:  "guide",
			Message: fmt.Sprintf("Monto $%.2f. Sugerir dividir pago: efectivo + tarjeta para mayor comodidad del cliente.", context.TotalAmount),
		}
	}

	return &AIResponse{
		Advice:  "standard_payment",
		Action:  "guide",
		Message: "Procesar pago normalmente. Aceptamos efectivo, tarjeta y transferencia.",
	}
}

func (ai *AIAssistant) adviseRefund(session *AISession, context *AIContext) *AIResponse {
	return &AIResponse{
		Advice:            "refund_policy",
		Action:            "warn",
		RequireSupervisor: true,
		Message:           "Devoluciones requieren autorización de supervisor. Verifique política de la tienda y documento original.",
	}
}

func (ai *AIAssistant) adviseFiscal(session *AISession, context *AIContext) *AIResponse {
	return &AIResponse{
		Advice:  "fiscal_printer_help",
		Action:  "guide",
		Message: "Verificaciones impresora fiscal: 1) Papel disponible, 2) Conexión USB/serie, 3) Memoria fiscal (último cierre Z). Si persiste, contacte soporte técnico.",
	}
}

func (ai *AIAssistant) adviseCustomer(session *AISession, context *AIContext) *AIResponse {
	return &AIResponse{
		Advice:  "customer_service",
		Action:  "guide",
		Message: "Para asistencia al cliente: consulte historial de compras, ofrezca productos complementarios, y si hay problema, escalar a supervisor.",
	}
}

func (ai *AIAssistant) getGeneralAdvice(context *AIContext) string {
	if context == nil {
		return "welcome"
	}
	if context.CartCount == 0 {
		return "empty_cart"
	}
	if context.TotalAmount > 1000 {
		return "high_value"
	}
	return "normal"
}

func (ai *AIAssistant) getGeneralMessage(context *AIContext) string {
	if context == nil {
		return "Bienvenido al asistente POS. ¿En qué puedo ayudarle?"
	}
	if context.CartCount == 0 {
		return "Carrito vacío. Escanee o busque productos para comenzar."
	}
	return fmt.Sprintf("Carrito: %d items, Total: $%.2f. ¿Necesita ayuda con algo?", context.CartCount, context.TotalAmount)
}

func (ai *AIAssistant) getOrCreateSession(terminalID, userID string) *AISession {
	key := terminalID + "_" + userID
	if session, ok := ai.sessions[key]; ok {
		return session
	}

	session := &AISession{
		ID:         fmt.Sprintf("ai_%d", time.Now().UnixNano()),
		TerminalID: terminalID,
		UserID:     userID,
		History:    []AIMessage{},
		LastActive: time.Now(),
	}
	ai.sessions[key] = session
	return session
}

// CleanupSessions removes inactive sessions
func (ai *AIAssistant) CleanupSessions(maxAge time.Duration) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	for key, session := range ai.sessions {
		if time.Since(session.LastActive) > maxAge {
			delete(ai.sessions, key)
		}
	}
}

// Global AI assistant instance
var AI = NewAIAssistant()

func init() {
	// Cleanup inactive sessions every 30 minutes
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		for range ticker.C {
			AI.CleanupSessions(2 * time.Hour)
		}
	}()
}
