package pos

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Server implements the POS gRPC service
type Server struct {
	mu        sync.RWMutex
	terminals map[string]*TerminalSession
	turns     map[string]*Turn
	sales     map[string]*Sale
}

type TerminalSession struct {
	TerminalID     string
	APIKey         string
	UserID         string
	Username       string
	Role           string
	CashRegisterID string
	Token          string
	Connected      bool
	LastSeen       time.Time
	Config         *TerminalConfig
}

type Turn struct {
	ID             string
	TerminalID     string
	CashRegisterID string
	UserID         string
	OpenedAt       time.Time
	ClosedAt       time.Time
	InitialFund    float64
	DeclaredCash   float64
	DeclaredCard   float64
	Status         string // open, closed, paused
	TotalSales     float64
	TotalRefunds   float64
	SalesCount     int
}

type Sale struct {
	ID             string
	TerminalID     string
	CashRegisterID string
	TurnID         string
	Items          []SaleItem
	Payments       []PaymentMethod
	Total          float64
	Tax            float64
	Status         string // pending, completed, refunded, offline
	FiscalNumber   string
	FiscalStatus   string
	CreatedAt      time.Time
	Offline        bool
	OfflineID      string
}

type SaleItem struct {
	ProductCode string  `json:"product_code"`
	SKU         string  `json:"sku"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	DiscountPct float64 `json:"discount_pct"`
	Description string  `json:"description"`
	TaxRate     float64 `json:"tax_rate"`
}

type PaymentMethod struct {
	Method     string  `json:"method"`
	Amount     float64 `json:"amount"`
	Reference  string  `json:"reference"`
	Bank       string  `json:"bank"`
}

type TerminalConfig struct {
	TerminalID        string
	Name              string
	Location          string
	CashRegisterID    string
	PaymentMethods    []string
	FiscalEnabled     bool
	FiscalPrinterPort string
	FiscalModel       string
	MaxDiscountPct    float64
	OfflineAllowed    bool
	CurrencySymbol    string
}

// NewServer creates a new POS server
func NewServer() *Server {
	return &Server{
		terminals: make(map[string]*TerminalSession),
		turns:     make(map[string]*Turn),
		sales:     make(map[string]*Sale),
	}
}

// GlobalServer is the global POS server instance
var GlobalServer = NewServer()

// GetServer returns the global POS server
func GetServer() *Server {
	return GlobalServer
}

func (s *Server) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("error escuchando en %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(s.authInterceptor),
		grpc.StreamInterceptor(s.streamAuthInterceptor),
	)

	// Register service
	// En producción: registrar con el plugin generado por protoc
	// Por ahora, usamos el servidor gRPC directo

	log.Printf("Servidor POS gRPC iniciado en %s", addr)
	return grpcServer.Serve(lis)
}

func (s *Server) authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	token, err := extractToken(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "token requerido")
	}

	session := s.getSessionByToken(token)
	if session == nil {
		return nil, status.Error(codes.Unauthenticated, "token inválido")
	}

	return handler(ctx, req)
}

func (s *Server) streamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := ss.Context()
	token, err := extractToken(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, "token requerido")
	}

	session := s.getSessionByToken(token)
	if session == nil {
		return status.Error(codes.Unauthenticated, "token inválido")
	}

	return handler(srv, ss)
}

func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Internal, "sin metadata")
	}

	tokens := md.Get("authorization")
	if len(tokens) == 0 {
		return "", status.Error(codes.Unauthenticated, "sin token")
	}

	return tokens[0], nil
}

func (s *Server) getSessionByToken(token string) *TerminalSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, session := range s.terminals {
		if session.Token == token {
			return session
		}
	}
	return nil
}

func (s *Server) GetSession(terminalID string) *TerminalSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.terminals[terminalID]
}

func (s *Server) registerSession(session *TerminalSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminals[session.TerminalID] = session
}

func (s *Server) unregisterSession(terminalID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.terminals, terminalID)
}

func (s *Server) getOpenTurn(terminalID string) *Turn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, turn := range s.turns {
		if turn.TerminalID == terminalID && turn.Status == "open" {
			return turn
		}
	}
	return nil
}

func (s *Server) createTurn(terminalID, cashRegisterID, userID string, initialFund float64) *Turn {
	s.mu.Lock()
	defer s.mu.Unlock()

	turn := &Turn{
		ID:             fmt.Sprintf("turn_%d", time.Now().UnixNano()),
		TerminalID:     terminalID,
		CashRegisterID: cashRegisterID,
		UserID:         userID,
		OpenedAt:       time.Now(),
		InitialFund:    initialFund,
		Status:         "open",
	}
	s.turns[turn.ID] = turn
	return turn
}

func (s *Server) closeTurn(turnID string, declaredCash, declaredCard, declaredOther float64) (*Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	turn, ok := s.turns[turnID]
	if !ok {
		return nil, fmt.Errorf("turno no encontrado")
	}

	turn.DeclaredCash = declaredCash
	turn.DeclaredCard = declaredCard
	turn.ClosedAt = time.Now()
	turn.Status = "closed"

	return turn, nil
}

func (s *Server) recordSale(sale *Sale) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sales[sale.ID] = sale

	// Update turn totals
	if turn, ok := s.turns[sale.TurnID]; ok {
		turn.TotalSales += sale.Total
		turn.SalesCount++
	}
}

func (s *Server) getSale(saleID string) *Sale {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sales[saleID]
}

func (s *Server) GetTerminalSessions() []*TerminalSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*TerminalSession
	for _, session := range s.terminals {
		result = append(result, session)
	}
	return result
}

func (s *Server) GetOpenTurns() []*Turn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Turn
	for _, turn := range s.turns {
		if turn.Status == "open" {
			result = append(result, turn)
		}
	}
	return result
}

func (s *Server) getSales(terminalID string, limit int) []*Sale {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Sale
	for _, sale := range s.sales {
		if sale.TerminalID == terminalID {
			result = append(result, sale)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}
