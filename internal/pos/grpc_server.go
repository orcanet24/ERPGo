// Package pos implementa el servidor gRPC para comunicación con terminales POS.
package pos

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "erpgo/proto/pos"
)

// DBResolver devuelve la base de la EMPRESA a la que pertenece una API key de
// terminal POS. La inyecta el servidor central (interfaces/web): las api_keys
// viven en la base de cada empresa, nunca en la de control, y este servidor no
// puede permitirse el lujo de adivinar cuál es.
type DBResolver func(apiKey string) (*sql.DB, error)

// FiscalDocFunc consume la serie fiscal y devuelve número + control fiscal.
// Se injecta desde interfaces/web para que el POS use la MISMA numeración
// central que la venta web. Si es nil, el POS fabrica su propio número
// (comportamiento legacy, NO seguro para SENIAT).
//
// El número de control fiscal es ÚNICO para todas las cajas. El control por
// caja (qué terminal vendió qué) va por separado en el campo terminal_id,
// que no altera la serie fiscal: reportes fiscales ven el correlativo central,
// reportes internos pueden cruzar invoice_id <-> terminal_id.
type FiscalDocFunc func(tx *sql.Tx, series string) (number int64, docNo string, controlNo string, err error)

// CredentialVerifier comprueba usuario y contraseña de un empleado contra la
// base de la empresa y devuelve su id y su nivel.
//
// La inyecta interfaces/web, que es donde vive la verificación de contraseñas
// (bcrypt y filas heredadas en md5 o texto plano). Este paquete no puede
// reimplementarla: sería una segunda copia de una regla de seguridad, y la
// segunda copia es la que se queda atrás.
type CredentialVerifier func(db *sql.DB, username, password string) (userID int, level string, err error)

// Cabeceras por las que viaja la credencial del terminal.
//
// La API key identifica la CAJA (y con ella la empresa): es la credencial del
// terminal. La sesión que devuelve Login identifica al CAJERO. Antes se usaban
// indistintamente y ninguna de las dos se comprobaba de forma uniforme.
const (
	metaAPIKey   = "x-api-key"
	metaTerminal = "x-pos-terminal"
	metaAuthz    = "authorization"
	bearerPrefix = "Bearer "
)

// credencial es lo que el interceptor deja en el contexto de una llamada ya
// autenticada: la API key presentada y la base de la empresa a la que
// pertenece. Es lo ÚNICO de lo que se fía un handler para decidir en qué
// empresa escribe; el terminal_id que venga en el mensaje es un dato, no una
// credencial.
type credencial struct {
	apiKey string
	db     *sql.DB
}

type credencialClave struct{}

func conCredencial(ctx context.Context, c *credencial) context.Context {
	return context.WithValue(ctx, credencialClave{}, c)
}

func credencialDe(ctx context.Context) *credencial {
	c, _ := ctx.Value(credencialClave{}).(*credencial)
	return c
}

// dbAutenticada devuelve la base de la empresa de una llamada ya autenticada.
// nil significa "no autenticada": el handler debe rechazar, no adivinar.
func dbAutenticada(ctx context.Context) *sql.DB {
	if c := credencialDe(ctx); c != nil {
		return c.db
	}
	return nil
}

// GRPCServer implementa el servicio POS gRPC.
type GRPCServer struct {
	pb.UnimplementedPOSServiceServer

	resolve DBResolver
	mu      sync.RWMutex
	clients map[string]*ClientConn
	streams map[string]pb.POSService_ConnectServer

	auditLogger   AuditFunc
	fiscalDocFunc FiscalDocFunc
	// saleRegistrar: el motor único de registro de ventas terminal
	// (interfaces/web/venta_terminal.go). Obligatorio para facturar por
	// gRPC: sin él, processSale se niega en vez de duplicar la lógica vieja.
	saleRegistrarFunc RegistrarVentaFunc
	// verifyCredentials: ver usuario/contraseña. Sin él, Login rechaza en vez
	// de inventarse una sesión.
	verifyCredentials CredentialVerifier

	// srv es el servidor gRPC real que Start pone a escuchar. Se guarda para
	// poder pararlo ordenadamente (GracefulStop) desde el servicio de Windows,
	// sin dejar el puerto 9095 colgado.
	srv *grpc.Server
}

// SetCredentialVerifier inyecta el verificador de usuario y contraseña que
// usa Login. Debe llamarse antes de Start.
func (s *GRPCServer) SetCredentialVerifier(fn CredentialVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verifyCredentials = fn
}

func (s *GRPCServer) verifier() CredentialVerifier {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.verifyCredentials
}

func (s *GRPCServer) fiscalDoc() FiscalDocFunc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fiscalDocFunc
}

// ClientConn representa una conexión activa de un terminal.
type ClientConn struct {
	TerminalID    string
	APIKey        string
	ConnectedAt   time.Time
	LastHeartbeat time.Time
	RemoteAddr    string
}

// NewGRPCServer crea una nueva instancia del servidor gRPC.
//
// resolveDB reemplaza al handle fijo de antes: un handle único pegaba TODAS las
// ventas de TODAS las cajas a la base con la que el proceso había arrancado.
func NewGRPCServer(resolveDB DBResolver, audit AuditFunc) *GRPCServer {
	return &GRPCServer{
		resolve:     resolveDB,
		clients:     make(map[string]*ClientConn),
		streams:     make(map[string]pb.POSService_ConnectServer),
		auditLogger: audit,
	}
}

// dbFor devuelve la base de la empresa de una API key, o nil si no resuelve.
func (s *GRPCServer) dbFor(apiKey string) *sql.DB {
	if s.resolve == nil || apiKey == "" {
		return nil
	}
	db, err := s.resolve(apiKey)
	if err != nil {
		log.Printf("⚠️ API key sin empresa asignada: %v", err)
		return nil
	}
	return db
}

// Start inicia el servidor gRPC en el puerto especificado.
func (s *GRPCServer) Start(port string) error {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("escuchando puerto %s: %w", port, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)
	pb.RegisterPOSServiceServer(grpcServer, s)
	// La reflexión publica el contrato entero (métodos y mensajes) a quien
	// alcance el puerto. Va apagada por defecto y se enciende con
	// POS_GRPC_REFLECTION=1, que es lo que hace falta mientras se depura un
	// terminal nuevo.
	if reflexionHabilitada() {
		log.Printf("⚠️ gRPC: reflexión habilitada (POS_GRPC_REFLECTION)")
		reflection.Register(grpcServer)
	}

	// Se guarda antes de servir para que Stop() pueda pararlo desde otro
	// goroutine (el servicio de Windows) mientras Serve bloquea aquí.
	s.mu.Lock()
	s.srv = grpcServer
	s.mu.Unlock()

	log.Printf("🔌 Servidor gRPC POS escuchando en %s", port)
	return grpcServer.Serve(lis)
}

// Stop detiene ordenadamente el servidor gRPC (drena las llamadas en curso).
// Llamar desde otro goroutine: hará que el Serve de Start regrese.
func (s *GRPCServer) Stop() {
	s.mu.RLock()
	g := s.srv
	s.mu.RUnlock()
	if g != nil {
		g.GracefulStop()
	}
}

// reflexionHabilitada indica si se publica la reflexión gRPC. Apagada salvo
// que el operador la pida por entorno.
func reflexionHabilitada() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("POS_GRPC_REFLECTION"))) {
	case "1", "true", "t", "yes", "y", "si", "sí", "on":
		return true
	}
	return false
}

// unaryInterceptor AUTENTICA cada RPC antes de ejecutarla.
//
// Antes solo registraba el nombre del método: el comentario prometía
// autenticación y no había ninguna. La credencial es la API key del terminal,
// que viaja en la cabecera x-api-key; algunas RPC del protocolo la llevan
// además en su campo Token, y se acepta como respaldo para no romper a un
// cliente ya desplegado.
//
// El resultado de autenticar no se descarta: la empresa resuelta entra en el
// contexto de la llamada, y a partir de ahí el handler la lee de ahí.
func (s *GRPCServer) unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Printf("[gRPC] %s", info.FullMethod)

	if metodoAutenticadoPorMensaje(info.FullMethod) {
		return handler(ctx, req)
	}

	creds, err := s.autenticar(credencialDeLlamada(ctx, req))
	if err != nil {
		return nil, err
	}
	return handler(conCredencial(ctx, creds), req)
}

// streamInterceptor hace lo mismo para los streams.
//
// `Connect` queda exento porque su credencial viaja en el PRIMER mensaje del
// stream (el registro), que el interceptor no puede leer sin consumirlo; ese
// handler valida la key y la empresa por su cuenta. El resto de los streams
// exige la credencial en la cabecera, que es lo que corta el agujero de
// SyncOffline: antes resolvía la empresa con el terminal_id de otra conexión.
func (s *GRPCServer) streamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	log.Printf("[gRPC Stream] %s", info.FullMethod)

	if info.FullMethod == metodoConnect {
		return handler(srv, ss)
	}

	creds, err := s.autenticar(apiKeyDeContexto(ss.Context()))
	if err != nil {
		return err
	}
	return handler(srv, &streamConCredencial{ServerStream: ss, ctx: conCredencial(ss.Context(), creds)})
}

// streamConCredencial publica el contexto autenticado al handler del stream.
// Sin esto, el handler de un stream no vería la credencial que validó el
// interceptor (el contexto del ServerStream lo fija gRPC antes de llegar aquí).
type streamConCredencial struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *streamConCredencial) Context() context.Context { return s.ctx }

// Rutas gRPC del servicio, con el nombre completo que usa el interceptor.
const (
	metodoConnect = "/pos.POSService/Connect"
	metodoLogin   = "/pos.POSService/Login"
	metodoLatido  = "/pos.POSService/Heartbeat"
)

// metodoAutenticadoPorMensaje son las RPC que se autentican con su propio
// contenido, o que no exponen ningún dato del negocio.
func metodoAutenticadoPorMensaje(fullMethod string) bool {
	switch fullMethod {
	case metodoLogin, // la credencial del terminal viaja en el mensaje
		metodoLatido,  // solo responde la hora del servidor
		metodoConnect: // se autentica en el primer mensaje del stream
		return true
	}
	return false
}

// apiKeyDeContexto extrae la credencial del terminal de las cabeceras de la
// llamada. Solo cabeceras: un token en la URL acaba en logs, historiales y
// proxies, y aquí no hay URL donde ponerlo.
func apiKeyDeContexto(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := primerValorMD(md, metaAPIKey); v != "" {
		return v
	}
	if v := primerValorMD(md, metaAuthz); strings.HasPrefix(v, bearerPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(v, bearerPrefix))
	}
	return ""
}

// terminalDeContexto es el terminal que el cliente declara en la cabecera
// x-pos-terminal. Es un DATO para saber qué caja escribió, no una credencial:
// la credencial es la API key, y la empresa sale siempre de ella.
func terminalDeContexto(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	return primerValorMD(md, metaTerminal)
}

func primerValorMD(md metadata.MD, clave string) string {
	for _, v := range md.Get(clave) {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// credencialDeLlamada busca la credencial en la cabecera y, si no está, en el
// campo que el protocolo ya definía para llevarla.
//
// ONLY cabecera y LoginRequest.api_key y SaleRequest.token. NO se acepta el
// campo Token de OpenTurn/CloseTurn/ProcessPayment/ProcessRefund: en esas RPC el
// proto documenta ese campo como la SESIÓN del cajero, no como la API key. Al
// aceptarlo como credencial se estaba tratando la sesión como si fuera la key,
// que es exactamente la confusión que este cambio separa.
func credencialDeLlamada(ctx context.Context, req interface{}) string {
	if v := apiKeyDeContexto(ctx); v != "" {
		return v
	}
	switch m := req.(type) {
	case *pb.LoginRequest:
		return m.GetApiKey()
	case *pb.SaleRequest:
		return m.GetToken()
	}
	return ""
}

// autenticar resuelve la empresa de una credencial y devuelve la credencial
// lista para el contexto. Los motivos del rechazo no se detallan al cliente:
// decir "esa key existe en otra empresa" ya es información de más para quien
// está probando credenciales.
func (s *GRPCServer) autenticar(apiKey string) (*credencial, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, status.Error(codes.Unauthenticated,
			"falta la credencial del terminal (cabecera "+metaAPIKey+")")
	}
	if s.resolve == nil {
		return nil, status.Error(codes.Internal, "resolutor de empresas no configurado")
	}
	db, err := s.resolve(apiKey)
	if err != nil || db == nil {
		log.Printf("gRPC: credencial de terminal rechazada: %v", err)
		return nil, status.Error(codes.Unauthenticated, "credencial de terminal inválida")
	}
	return &credencial{apiKey: apiKey, db: db}, nil
}

// validarSesionPOS comprueba un token de sesión POS dentro de la base de la
// empresa de la credencial. Devuelve el id del cajero de la sesión (0 si la
// llamada no trae token, en cuyo caso la credencial válida es la API key).
//
// Lo que esto cierra: OpenTurn/CloseTurn recibían un `Token` que se comparaba
// contra `api_keys` —es decir, se trataba la sesión como si fuera la key— y no
// se comprobaba ni expiración, ni revocación, ni que fuera de esa empresa.
func (s *GRPCServer) validarSesionPOS(db *sql.DB, token string) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, nil
	}
	var userID int
	err := db.QueryRow(`SELECT user_id FROM pos_sessions
		WHERE token = ? AND is_active = 1 AND IFNULL(revoked, 0) = 0 AND expires_at > NOW()
		LIMIT 1`, token).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, status.Error(codes.Unauthenticated, "token de sesión POS inválido, revocado o expirado")
	}
	if err != nil {
		return 0, status.Errorf(codes.Internal, "comprobando la sesión POS: %v", err)
	}
	return userID, nil
}

// generateSecureToken crea un token criptográficamente seguro (256 bits).
func generateSecureToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "grpc_" + hex.EncodeToString(b)
}

// Connect implementa el stream bidireccional principal.
func (s *GRPCServer) Connect(stream pb.POSService_ConnectServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "esperando registro: %v", err)
	}

	terminalID := strings.TrimSpace(msg.GetRegister().GetTerminalId())
	apiKey := msg.GetRegister().GetApiKey()

	if terminalID == "" {
		return status.Error(codes.InvalidArgument, "el registro del terminal debe traer terminal_id")
	}

	// La empresa se resuelve con la API key del registro y se valida contra la
	// tabla api_keys de ESA empresa. Es la autenticación de todo el stream:
	// de aquí en adelante los mensajes se procesan con ESTA empresa, sin mirar
	// lo que el terminal diga de sí mismo en cada mensaje.
	db := s.dbFor(apiKey)
	if db == nil || !s.validateAPIKey(db, apiKey, terminalID) {
		return status.Error(codes.Unauthenticated, "API key invalida")
	}

	s.mu.Lock()
	s.clients[terminalID] = &ClientConn{
		TerminalID:    terminalID,
		APIKey:        apiKey,
		ConnectedAt:   time.Now(),
		LastHeartbeat: time.Now(),
	}
	s.streams[terminalID] = stream
	s.mu.Unlock()

	log.Printf("🔌 Terminal %s conectado via gRPC", terminalID)

	if err := stream.Send(&pb.ServerMessage{
		ServerId:  "erpgow",
		Timestamp: time.Now().Unix(),
	}); err != nil {
		log.Printf("Error enviando bienvenida a %s: %v", terminalID, err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			log.Printf("🔌 Terminal %s desconectado: %v", terminalID, err)
			s.removeClient(terminalID)
			return nil
		}

		if err := s.handleTerminalMessage(stream, terminalID, db, msg); err != nil {
			log.Printf("Error procesando mensaje de %s: %v", terminalID, err)
		}
	}
}

// handleTerminalMessage procesa un mensaje del terminal.
//
// Recibe el terminal_id REGISTRADO y la base de SU empresa: los datos del
// mensaje no los pueden cambiar.
func (s *GRPCServer) handleTerminalMessage(stream pb.POSService_ConnectServer, terminalID string, db *sql.DB, msg *pb.TerminalMessage) error {
	if id := strings.TrimSpace(msg.GetTerminalId()); id != "" && id != terminalID {
		return status.Errorf(codes.PermissionDenied,
			"el mensaje declara el terminal %q pero la conexion esta registrada como %q", id, terminalID)
	}

	switch payload := msg.Payload.(type) {
	case *pb.TerminalMessage_Sale:
		return s.handleSale(stream, terminalID, db, payload.Sale)
	case *pb.TerminalMessage_Payment:
		return s.handlePayment(stream, terminalID, payload.Payment)
	case *pb.TerminalMessage_Heartbeat:
		return s.handleHeartbeat(stream, terminalID, payload.Heartbeat)
	case *pb.TerminalMessage_Status:
		return s.handleStatus(stream, terminalID, payload.Status)
	default:
		return status.Errorf(codes.InvalidArgument, "tipo de mensaje desconocido")
	}
}

// handleSale procesa una venta enviada por el terminal.
//
// La empresa NO se deduce del terminal_id del mensaje: entra por parámetro,
// ya resuelta por la API key con la que se registró el stream.
func (s *GRPCServer) handleSale(stream pb.POSService_ConnectServer, terminalID string, db *sql.DB, sale *pb.SaleTransaction) error {
	if db == nil {
		return status.Errorf(codes.Unauthenticated, "terminal %s sin empresa asignada", terminalID)
	}
	res, err := s.processSale(db, terminalID, sale)
	if err != nil {
		stream.Send(&pb.ServerMessage{
			Payload: &pb.ServerMessage_SaleAck{
				SaleAck: &pb.SaleAck{
					SaleId:   sale.GetSaleId(),
					Accepted: false,
					Reason:   err.Error(),
				},
			},
			Timestamp: time.Now().Unix(),
		})
		return err
	}

	if res.Duplicada {
		// Reintento: la venta ya estaba. Se responde aceptada con la MISMA
		// factura, que es lo que el terminal necesita para no reintentar
		// para siempre ni quedarse con un local_id sin número.
		log.Printf("Venta %s ya registrada: se devuelve la factura %d (control %s)",
			sale.GetSaleId(), res.InvoiceNo, res.ControlNo)
	} else {
		log.Printf("✅ Venta %s procesada: factura %d, control %s", sale.GetSaleId(), res.InvoiceNo, res.ControlNo)
	}

	return stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_SaleAck{
			SaleAck: &pb.SaleAck{
				SaleId:   res.SaleID,
				Accepted: true,
			},
		},
		Timestamp: time.Now().Unix(),
	})
}

// handlePayment informa de que el cobro por el stream no está implementado.
//
// Antes devolvía nil: el mensaje se daba por bueno, se registraba solo en el
// log y el pago no se guardaba en ninguna parte. El error queda en el log del
// servidor y el terminal no recibe confirmación de un cobro inexistente.
func (s *GRPCServer) handlePayment(stream pb.POSService_ConnectServer, terminalID string, payment *pb.PaymentEntry) error {
	log.Printf("POS: pago recibido de la caja %s para la venta %s (no implementado)", terminalID, payment.GetSaleId())
	return status.Error(codes.Unimplemented, "el registro de pagos por el stream no esta implementado")
}

// handleHeartbeat procesa un heartbeat del terminal.
func (s *GRPCServer) handleHeartbeat(stream pb.POSService_ConnectServer, terminalID string, heartbeat *pb.HeartbeatMsg) error {
	s.mu.Lock()
	if client, ok := s.clients[terminalID]; ok {
		client.LastHeartbeat = time.Now()
	}
	s.mu.Unlock()

	return stream.Send(&pb.ServerMessage{
		Payload: &pb.ServerMessage_Pong{
			Pong: &pb.ServerPong{Ok: true},
		},
		Timestamp: time.Now().Unix(),
	})
}

// handleStatus procesa un reporte de estado.
func (s *GRPCServer) handleStatus(stream pb.POSService_ConnectServer, terminalID string, st *pb.StatusReport) error {
	log.Printf("📊 Estado de %s: online=%v, pending=%d", terminalID, st.GetOnline(), st.GetPendingSales())
	return nil
}

// SetFiscalDocFunc inyecta la numeración fiscal del servidor web. Debe llamarse
// antes de Start: con ella la venta consume la serie fiscal CENTRAL
// (SENIAT) en vez de fabricarse el número. Sin ella, la venta queda marcada
// como OFFLINE y requiere regularización.
func (s *GRPCServer) SetFiscalDocFunc(fn FiscalDocFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fiscalDocFunc = fn
}

// resultadoVenta es el resultado de una venta ya confirmada en la base.
//
// Se devuelve también cuando la venta es un REINTENTO: en ese caso Duplicada es
// true y los números son los de la factura original. Eso es justo lo que el
// terminal necesita para cerrar su cola offline — el ACK se perdió, no la venta.
type resultadoVenta struct {
	SaleID    string
	InvoiceNo int64
	DocNo     string
	ControlNo string
	Duplicada bool
}

// claveIdempotencia es el id con el que el terminal identifica la venta.
// Prefiere offline_id porque es el que el POS usa para reintentar; sale_id es el
// respaldo de las ventas que llegan por el camino en línea.
func claveIdempotencia(sale *pb.SaleTransaction) string {
	if id := strings.TrimSpace(sale.GetOfflineId()); id != "" {
		return id
	}
	return strings.TrimSpace(sale.GetSaleId())
}

// esClaveDuplicada dice si el error es una violación de clave única (MySQL
// 1062). Se usa para distinguir "otra petición se adelantó" de un fallo real.
func esClaveDuplicada(err error) bool {
	return codigoMySQL(err) == 1062
}

// esBloqueoDeCarrera reconoce el bloqueo mutuo (1213) que InnoDB puede devolver
// a una de las dos peticiones que chocan por la misma clave. Para el llamante
// significa lo mismo que un duplicado: la otra petición tiene la venta.
func esBloqueoDeCarrera(err error) bool {
	return codigoMySQL(err) == 1213
}

// codigoMySQL devuelve el número de error del servidor MySQL, o 0 si el error
// no viene de MySQL.
func codigoMySQL(err error) uint16 {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number
	}
	return 0
}

// processSale guarda la venta en la base de la EMPRESA del terminal.
//
// UNA SOLA TRANSACCIÓN: la reclamación de idempotencia, el número fiscal, la
// cabecera, los renglones y el documento quedan confirmados juntos. Si algo
// falla, no queda factura a medias ni número fiscal consumido sin venta.
func (s *GRPCServer) processSale(db *sql.DB, terminalID string, sale *pb.SaleTransaction) (resultadoVenta, error) {
	res := resultadoVenta{SaleID: sale.GetSaleId()}
	if db == nil {
		return res, fmt.Errorf("sin base de datos resuelta para el terminal")
	}
	if sale == nil {
		return res, fmt.Errorf("venta vacia")
	}

	// La caja la fija la CONEXIÓN autenticada, no el mensaje. Sin terminal
	// (venta unary sin registro previo) queda un valor fijo reconocible.
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		terminalID = "POS"
	}
	offlineID := claveIdempotencia(sale)

	// Atajo del caso normal: la venta ya se registró (el ACK se perdió y el
	// terminal reintenta). Se devuelve su documento sin tocar nada más.
	if offlineID != "" {
		previa, err := s.ventaRegistrada(db, terminalID, offlineID)
		switch {
		case err == nil:
			previa.SaleID = res.SaleID
			return previa, nil
		case err == errVentaNoRegistrada:
			// Sigue el camino normal.
		default:
			// No se puede saber si ya estaba: NO se factura a ciegas. Antes
			// que emitir una segunda factura por el mismo cobro, se rechaza y
			// el terminal reintenta.
			return res, err
		}
	}

	// Política externa (addons) ANTES de abrir la transacción: el addon veta,
	// pero no toca stock ni numeración. Si el guard tarda o se cae, el núcleo
	// sigue siendo el dueño de la factura.
	//
	// Las ventas OFFLINE no pasan por el guard: ya se cobraron cuando no había
	// red, y rechazarlas al sincronizar dejaría la cola del terminal atascada
	// con un cobro que el cliente ya pagó. Esas se reconcilian aparte.
	if !sale.GetOffline() {
		guardItems := make([]GuardItem, 0, len(sale.GetItems()))
		for _, it := range sale.GetItems() {
			guardItems = append(guardItems, GuardItem{
				ProductID: it.GetProductCode(),
				Barcode:   it.GetSku(),
				Quantity:  it.GetQuantity(),
			})
		}
		if err := evaluarGuard(context.Background(), db, terminalID, guardItems); err != nil {
			return res, err
		}
	}

	// EL REGISTRO VIVO es UN SOLO motor para los dos transportes (F-2,
	// auditoría 2026-11-22): registrarVentaTerminal, en
	// interfaces/web/venta_terminal.go, inyectado como cierre desde
	// cmd/server porque este paquete no puede importar web. El cuerpo
	// anterior duplicaba aquí una versión parcial de esa venta —sin asiento
	// contable, sin inventario, sin pagos—: dos caminos, dos semánticas
	// fiscales, y una revisión del SENIAT no distingue transportes.
	reg := s.saleRegistrar()
	if reg == nil {
		return res, fmt.Errorf("el servidor gRPC no tiene motor de ventas inyectado: así las cajas no pueden facturar (error de despliegue, no del terminal)")
	}

	req := &SyncSaleRequest{
		TerminalID:   terminalID,
		SaleLocalID:  offlineID,
		CustomerRIF:  sale.GetCustomer().GetRif(),
		CustomerName: sale.GetCustomer().GetName(),
		Subtotal:     sale.GetSubtotal(),
		Tax:          sale.GetTax(),
		Total:        sale.GetTotal(),
		FiscalNumber: sale.GetOfflineControlNo(),
	}
	// El protocolo gRPC lleva el instante de cobro como epoch (int64). Se
	// pasa al motor en RFC3339 —el formato del contrato HTTP— para que la
	// factura lleve la FECHA DEL COBRO y no la de sincronización (caja que
	// cobra a las 23:58 y sincroniza a las 00:05: la venta es del día
	// anterior, y el arqueo diario del corte tiene que decirlo).
	if ts := sale.GetCreatedAt(); ts > 0 {
		req.CreatedAt = time.Unix(ts, 0).Format(time.RFC3339)
	}
	for _, it := range sale.GetItems() {
		req.Items = append(req.Items, SaleItem{
			ProductCode: it.GetProductCode(),
			SKU:         it.GetSku(),
			Quantity:    it.GetQuantity(),
			UnitPrice:   it.GetUnitPrice(),
			DiscountPct: it.GetDiscountPct(),
			Description: it.GetDescription(),
			TaxRate:     it.GetTaxRate(),
		})
	}
	for _, p := range sale.GetPayments() {
		req.Payments = append(req.Payments, PaymentMethod{
			Method:    p.GetType(),
			Amount:    p.GetAmount(),
			Reference: p.GetReference(),
			Bank:      p.GetBank(),
		})
	}

	resp, err := reg(db, req)
	if err != nil {
		return res, err
	}
	if resp == nil {
		return res, fmt.Errorf("respuesta vacía del motor de ventas")
	}
	if resp.Status == "in_progress" {
		// El motor ya detectó la carrera; se reporta como antes para que el
		// terminal reintente sin duplicar.
		return res, fmt.Errorf("la venta %s ya se está registrando en otra petición", offlineID)
	}
	invoiceNo, err := strconv.ParseInt(resp.RemoteID, 10, 64)
	if err != nil {
		return res, fmt.Errorf("el motor de ventas devolvió un identificador ilegible: %q", resp.RemoteID)
	}

	// Tabla de ACK del gRPC (pos_sale_idempotency): se escribe DESPUÉS del
	// commit del motor. La deduplicación real vive en pos_sale_sync (reclamo
	// dentro de la transacción); esta fila existe solo para que el reintento
	// con el mismo offline_id lea su documento sin volver a pasar por el
	// motor. Se ignora el duplicado: otra petición del mismo batch la escribió.
	if offlineID != "" {
		if _, err := db.Exec(`INSERT INTO pos_sale_idempotency (terminal_id, offline_id, invoice_no, control_no, created_at)
			VALUES (?,?,?,?,NOW())
			ON DUPLICATE KEY UPDATE invoice_no = VALUES(invoice_no), control_no = VALUES(control_no)`,
			terminalID, offlineID, invoiceNo, resp.FiscalNumber); err != nil {
			// No es fatal: la venta YA está registrada; fallaría el reintento
			// rápido pero el motor idempotente lo resuelve igual. Se avisa.
			if s.auditLogger != nil {
				s.auditLogger("pos_ack_write_failed", "pos", "pos_sale_idempotency", offlineID, nil, err.Error())
			}
		}
	}

	if s.auditLogger != nil {
		s.auditLogger("sync_sale", "pos", "invoice", fmt.Sprintf("%d", invoiceNo), nil, sale.GetSaleId())
	}

	return resultadoVenta{
		SaleID:    res.SaleID,
		InvoiceNo: invoiceNo,
		DocNo:     resp.FiscalNumber,
		ControlNo: resp.FiscalNumber,
		Duplicada: resp.Status == "already_exists",
	}, nil
}

// RegistrarVentaFunc inyecta el motor único de registro de ventas terminal
// (interfaces/web/venta_terminal.go). Firma en tipos de este paquete para
// que la inyección no obligue a importar web.
type RegistrarVentaFunc func(db *sql.DB, req *SyncSaleRequest) (*SyncSaleResponse, error)

// SetSaleRegistrar inyecta el motor. Debe llamarse antes de servir (igual
// que SetFiscalDocFunc); sin él, processSale se niega en vez de reproducir
// el camino viejo sin asiento —dos semánticas fiscales para la misma caja
// es peor que una negativa ruidosa—.
func (s *GRPCServer) SetSaleRegistrar(fn RegistrarVentaFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saleRegistrarFunc = fn
}

// saleRegistrar lectura corta bajo el candado (misma RLock que fiscalDoc:
// es configuración de arranque, se lee en cada venta).
func (s *GRPCServer) saleRegistrar() RegistrarVentaFunc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saleRegistrarFunc
}

// errVentaNoRegistrada separa "no había una venta previa" de "no se pudo
// comprobar". Sin esta distinción, un fallo de lectura al comprobar la
// idempotencia se confundiría con una venta nueva y se facturaría dos veces.
var errVentaNoRegistrada = errors.New("venta no registrada antes")

// ventaRegistrada devuelve la factura que ya se emitió para una venta del
// terminal, o errVentaNoRegistrada si no hay ninguna.
//
// UNA sola lectura: es el atajo del caso normal, y en el caso normal (venta
// nueva) NO hay fila. Esperar aquí unos segundos convertiría cada venta nueva en
// cinco segundos de espera.
func (s *GRPCServer) ventaRegistrada(db *sql.DB, terminalID, offlineID string) (resultadoVenta, error) {
	var res resultadoVenta
	err := db.QueryRow(`SELECT invoice_no, control_no FROM pos_sale_idempotency
		WHERE terminal_id = ? AND offline_id = ? AND invoice_no > 0`, terminalID, offlineID).
		Scan(&res.InvoiceNo, &res.ControlNo)
	switch {
	case err == nil:
		res.Duplicada = true
		return res, nil
	case err == sql.ErrNoRows:
		return res, errVentaNoRegistrada
	default:
		return res, fmt.Errorf("comprobando si la venta ya estaba registrada: %w", err)
	}
}

// esperarVentaRegistrada lee la factura del ganador de una carrera.
//
// Aquí sí se espera: al llegar a este punto otra petición ya insertó la misma
// clave (o chocó con nosotros y deshizo la suya), y lo que falta es su
// documento. La UNIQUE hace esperar al segundo INSERT hasta que el primero
// confirma, así que la fila ya debería estar; el reintento acotado cubre el
// instante en que todavía no es visible.
func (s *GRPCServer) esperarVentaRegistrada(db *sql.DB, terminalID, offlineID string) (resultadoVenta, error) {
	limite := time.Now().Add(5 * time.Second)
	for {
		res, err := s.ventaRegistrada(db, terminalID, offlineID)
		if err == nil {
			return res, nil
		}
		if err != errVentaNoRegistrada {
			return res, err
		}
		if time.Now().After(limite) {
			return res, fmt.Errorf("la venta %s ya se esta registrando en otra peticion y aun no se puede leer su factura", offlineID)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// LA NUMERACIÓN DE VENTAS VIVE EN UN SOLO SITIO: interfaces/web/fiscal.go (nextFiscalDoc / reservarNumeroFacturaTx),// usado por registrarVentaTerminal para ambos transportes. Este paquete ya no reserva números: el gRPC delega el registro completo en ese motor (SetSaleRegistrar), y duplicar aquí el contador fue como F-3 (MAX+1 sin bloqueo) sobrevivió en un camino mientras el otro ya estaba arregrado.// (Antes existía en este archivo una copia llamada reservarNumeroFactura: eliminada con la delegación, no se conserva "por si acaso".)

// validateAPIKey verifica que la API key sea valida contra la tabla api_keys
// DE LA EMPRESA resuelta para esa key.
func (s *GRPCServer) validateAPIKey(db *sql.DB, apiKey, terminalID string) bool {
	if db == nil || apiKey == "" {
		return false
	}

	var id int
	err := db.QueryRow("SELECT id FROM api_keys WHERE api_key = ? AND is_active = 1 LIMIT 1", apiKey).Scan(&id)
	if err != nil {
		log.Printf("⚠️ API key invalida para terminal %s: %v", terminalID, err)
		return false
	}

	if _, err := db.Exec("UPDATE api_keys SET last_used_at = NOW() WHERE id = ?", id); err != nil {
		// No se rechaza la conexión por esto: la key es válida y la marca de uso
		// es informativa. Pero se dice, porque un fallo aquí puede significar una
		// base sin esa columna o abierta en solo lectura, y eso sí conviene
		// saberlo antes de que falle algo peor.
		log.Printf("POS: no se pudo marcar el uso de la api key %d: %v", id, err)
	}
	log.Printf("✅ API key valida para terminal %s", terminalID)
	return true
}

// removeClient elimina un cliente de la lista de conexiones activas.
func (s *GRPCServer) removeClient(terminalID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, terminalID)
	delete(s.streams, terminalID)
}

// GetConnectedClients retorna la lista de terminales conectados.
func (s *GRPCServer) GetConnectedClients() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	clients := make([]string, 0, len(s.clients))
	for id := range s.clients {
		clients = append(clients, id)
	}
	return clients
}

// SendToTerminal envia un mensaje a un terminal especifico.
func (s *GRPCServer) SendToTerminal(terminalID string, msg *pb.ServerMessage) error {
	s.mu.RLock()
	stream, ok := s.streams[terminalID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("terminal %s no conectado", terminalID)
	}

	return stream.Send(msg)
}

// Broadcast envia un mensaje a todos los terminales conectados.
func (s *GRPCServer) Broadcast(msg *pb.ServerMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for terminalID, stream := range s.streams {
		if err := stream.Send(msg); err != nil {
			log.Printf("Error enviando a %s: %v", terminalID, err)
		}
	}
}

// Login implementa autenticación individual con token seguro.
//
// Antes bastaba la API key: el campo username/password del proto se ignoraba y
// la sesión se creaba con el terminal_id como si fuera el usuario. Una caja
// con la key válida entraba sin cajero. Ahora la key identifica la CAJA y
// usuario/contraseña identifican al CAJERO, y sin las dos no hay sesión.
func (s *GRPCServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	db := s.dbFor(req.GetApiKey())
	if db == nil || !s.validateAPIKey(db, req.GetApiKey(), req.GetTerminalId()) {
		return nil, status.Error(codes.Unauthenticated, "API key invalida")
	}

	verificar := s.verifier()
	if verificar == nil {
		// Fail closed: sin verificador no se inventa una sesión. Una sesión
		// inventada es un cajero inexistente facturando.
		return nil, status.Error(codes.Unavailable, "verificacion de credenciales no configurada en el servidor")
	}
	userID, level, err := verificar(db, req.GetUsername(), req.GetPassword())
	if err != nil {
		log.Printf("POS: login rechazado para %q en el terminal %s: %v", req.GetUsername(), req.GetTerminalId(), err)
		return nil, status.Error(codes.Unauthenticated, "credenciales invalidas")
	}

	// La sucursal sale de la propia API key: ata la caja a una sucursal, y no
	// viene en la petición (antes se insertaba la sesión sin sucursal).
	var branchID sql.NullInt64
	if err := db.QueryRow(`SELECT branch_id FROM api_keys WHERE api_key = ? AND is_active = 1 LIMIT 1`,
		req.GetApiKey()).Scan(&branchID); err != nil && err != sql.ErrNoRows {
		return nil, status.Errorf(codes.Internal, "leyendo la sucursal de la caja: %v", err)
	}

	token := generateSecureToken()

	if _, err := db.Exec(`INSERT INTO pos_sessions (token, user_id, terminal_id, branch_id, login_time, expires_at, is_active, created_at)
		VALUES (?, ?, ?, ?, NOW(), DATE_ADD(NOW(), INTERVAL 12 HOUR), 1, NOW())`,
		token, userID, req.GetTerminalId(), branchID); err != nil {
		// Antes se descartaba el error y se devolvía un token que no existía en
		// la base: el terminal operaba con una sesión fantasma.
		return nil, status.Errorf(codes.Internal, "creando la sesion POS: %v", err)
	}

	log.Printf("POS: sesion iniciada por %q en la caja %s (nivel %s)", req.GetUsername(), req.GetTerminalId(), level)

	return &pb.LoginResponse{
		Success: true,
		Token:   token,
		Message: "Autenticado via gRPC",
		Role:    &pb.UserRole{Role: rolPOS(level), Label: level},
	}, nil
}

// rolPOS traduce el nivel del empleado al rol que viaja en el proto.
func rolPOS(level string) pb.USER_ROLE {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "admin", "admin terminal", "administrador", "root":
		return pb.USER_ROLE_ROLE_ADMIN
	case "supervisor", "manager", "gerente":
		return pb.USER_ROLE_ROLE_SUPERVISOR
	default:
		return pb.USER_ROLE_ROLE_CASHIER
	}
}

// OpenTurn implementa apertura de turno.
//
// La empresa viene de la credencial que validó el interceptor (API key) y la
// sesión del campo Token tiene que ser una sesión POS viva de esa empresa. Antes
// el Token se comparaba contra api_keys —se trataba la sesión como si fuera la
// key— y no se miraba ni expiración ni revocación.
func (s *GRPCServer) OpenTurn(ctx context.Context, req *pb.OpenTurnRequest) (*pb.TurnResponse, error) {
	db := dbAutenticada(ctx)
	if db == nil {
		return nil, status.Errorf(codes.Unauthenticated, "falta la credencial del terminal (cabecera %s)", metaAPIKey)
	}

	userID, err := s.validarSesionPOS(db, req.GetToken())
	if err != nil {
		return nil, err
	}

	cajero := req.GetTerminalId()
	if userID > 0 {
		cajero = strconv.Itoa(userID)
	}

	if _, err := db.Exec(`INSERT INTO cash_session (cashier, opening_amount, opening_amount_usd, status, opened_at, warehouse_id)
		VALUES (?, ?, ?, 1, NOW(), ?)`,
		cajero, req.GetInitialFund(), req.GetInitialFundUsd(), req.GetTerminalId()); err != nil {
		return nil, status.Errorf(codes.Internal, "abriendo el turno: %v", err)
	}

	return &pb.TurnResponse{
		Success: true,
		TurnId:  fmt.Sprintf("turn_%s_%d", req.GetTerminalId(), time.Now().Unix()),
		Message: "Turno abierto",
	}, nil
}

// CloseTurn implementa cierre de turno.
func (s *GRPCServer) CloseTurn(ctx context.Context, req *pb.CloseTurnRequest) (*pb.TurnResponse, error) {
	db := dbAutenticada(ctx)
	if db == nil {
		return nil, status.Errorf(codes.Unauthenticated, "falta la credencial del terminal (cabecera %s)", metaAPIKey)
	}

	if _, err := s.validarSesionPOS(db, req.GetToken()); err != nil {
		return nil, err
	}

	// El turno se cierra por el id de caja registradora cuando lo trae, y si no
	// por el terminal: la consulta dice exactamente QUÉ fila se cierra, en vez de
	// cerrar "la última abierta" de la caja que el mensaje diga.
	caja := req.GetTerminalId()
	if v := strings.TrimSpace(req.GetCashRegisterId()); v != "" {
		caja = v
	}

	res, err := db.Exec(`UPDATE cash_session SET status = 0, closed_at = NOW(), closing_amount = ?, closing_amount_usd = ?
		WHERE warehouse_id = ? AND status = 1 ORDER BY opened_at DESC LIMIT 1`,
		req.GetDeclaredCash(), req.GetDeclaredCashUsd(), caja)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cerrando el turno: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "comprobando el cierre del turno: %v", err)
	}
	if n == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "no hay un turno abierto para la caja %s", caja)
	}

	return &pb.TurnResponse{
		Success: true,
		TurnId:  req.GetCashRegisterId(),
		Message: "Turno cerrado",
	}, nil
}

// ProcessSale procesa una venta individual (sin stream).
//
// El path unary NO pasa por el registro en stream Connect, así que la empresa
// la resuelve el interceptor con la credencial de la petición (cabecera
// x-api-key, o el campo Token como respaldo) y aquí se lee del CONTEXTO. Antes
// se adivinaba por el terminal_id del mensaje: cualquiera que declarara el
// terminal de otra caja escribía en la empresa equivocada.
func (s *GRPCServer) ProcessSale(ctx context.Context, req *pb.SaleRequest) (*pb.SaleResponse, error) {
	db := dbAutenticada(ctx)
	if db == nil {
		return nil, status.Errorf(codes.Unauthenticated, "falta la credencial del terminal (cabecera %s)", metaAPIKey)
	}

	caja := strings.TrimSpace(req.GetTerminalId())
	if caja == "" {
		caja = terminalDeContexto(ctx)
	}

	res, err := s.processSale(db, caja, &pb.SaleTransaction{
		SaleId:         req.GetOfflineId(),
		TerminalId:     req.GetTerminalId(),
		CashRegisterId: req.GetCashRegisterId(),
		Items:          req.GetItems(),
		Customer:       req.GetCustomer(),
		Offline:        req.GetOffline(),
		OfflineId:      req.GetOfflineId(),
		Total:          req.GetTotal(),
		Tax:            req.GetTax(),
		Subtotal:       req.GetSubtotal(),
	})
	if err != nil {
		// Un veto de política no es un fallo interno: el terminal debe mostrar
		// el motivo al cajero y no reintentar como si fuera un error de red.
		var blocked *SaleBlockedError
		if errors.As(err, &blocked) {
			return nil, status.Errorf(codes.FailedPrecondition, "%s", blocked.Error())
		}
		return nil, status.Errorf(codes.Internal, "procesando la venta: %v", err)
	}

	msg := "Venta procesada"
	if res.Duplicada {
		// No es un error para el terminal: es la respuesta que le faltaba.
		msg = "Venta ya registrada: se devuelve el documento original"
	}

	return &pb.SaleResponse{
		Success:      true,
		SaleId:       fmt.Sprintf("%d", res.InvoiceNo),
		FiscalNumber: res.ControlNo,
		Message:      msg,
	}, nil
}

// ProcessPayment NO está implementado.
//
// Antes devolvía Success:true sin tocar la base: el terminal daba el cobro por
// registrado y no se registraba nada. Un "no" explícito deja al POS reintentar
// o avisar al cajero; un "sí" falso es un pago perdido y silencioso.
func (s *GRPCServer) ProcessPayment(ctx context.Context, req *pb.PaymentRequest) (*pb.PaymentResponse, error) {
	log.Printf("POS: ProcessPayment solicitado por la caja %s (no implementado)", req.GetTerminalId())
	return nil, status.Error(codes.Unimplemented, "ProcessPayment no esta implementado en este servidor")
}

// ProcessRefund NO está implementado.
//
// Mismo caso que ProcessPayment: devolvía éxito sin emitir la nota de crédito,
// así que el terminal mostraba una devolución hecha que no existía en fiscal.
func (s *GRPCServer) ProcessRefund(ctx context.Context, req *pb.RefundRequest) (*pb.RefundResponse, error) {
	log.Printf("POS: ProcessRefund solicitado por la caja %s (no implementado)", req.GetTerminalId())
	return nil, status.Error(codes.Unimplemented, "ProcessRefund no esta implementado en este servidor")
}

// GetProductByCode retorna un producto por código.
//
// El catálogo NO se sirve por gRPC: el proto no transporta identidad de
// terminal ni API key, y adivinar la empresa aquí sería el mismo bug de
// siempre. El terminal descarga el catálogo por HTTP con X-POS-Tenant.
func (s *GRPCServer) GetProductByCode(ctx context.Context, req *pb.ProductQuery) (*pb.ProductResponse, error) {
	return nil, status.Errorf(codes.Unauthenticated, "use GET /api/pos/products con X-POS-Tenant")
	/*
		var name string
		var price float64
		err := s.db.QueryRow("SELECT product_name, price FROM product_information WHERE product_id = ? AND status = 1 LIMIT 1", req.GetCode()).Scan(&name, &price)
		if err != nil {
			return &pb.ProductResponse{}, status.Errorf(codes.NotFound, "producto no encontrado")
		}
		return &pb.ProductResponse{
			Code:        req.GetCode(),
			Description: name,
			Price:       price,
		}, nil
	*/
}

// GetProducts retorna el catálogo de productos (mismo criterio que arriba).
func (s *GRPCServer) GetProducts(req *pb.ProductQuery, stream pb.POSService_GetProductsServer) error {
	return status.Errorf(codes.Unauthenticated, "use GET /api/pos/products con X-POS-Tenant")
	/*
		rows, err := s.db.Query("SELECT product_id, product_name, price FROM product_information WHERE status = 1 LIMIT 1000")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name string
			var price float64
			if err := rows.Scan(&id, &name, &price); err != nil {
				continue
			}
			if err := stream.Send(&pb.ProductResponse{
				Code:        id,
				Description: name,
				Price:       price,
			}); err != nil {
				return err
			}
		}
		return nil
	*/
}

// SyncOffline sincroniza ventas offline.
//
// La empresa viene de la CREDENCIAL del stream (API key), no del terminal_id
// del mensaje: antes se resolvía con dbOfTerminal(terminal_id), es decir con la
// API key de OTRA conexión ya registrada, así que quien declarara el terminal de
// una caja conectada quedaba trabajando con sus credenciales.
func (s *GRPCServer) SyncOffline(stream pb.POSService_SyncOfflineServer) error {
	db := dbAutenticada(stream.Context())
	if db == nil {
		return status.Errorf(codes.Unauthenticated, "falta la credencial del terminal (cabecera %s)", metaAPIKey)
	}
	caja := terminalDeContexto(stream.Context())

	var total, synced, failed int
	var resultados []*pb.SyncResult

	for {
		sale, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// Antes CUALQUIER error de lectura se trataba como fin de stream
				// y se respondía un resumen: una conexión cortada se veía como
				// "sincronización terminada".
				return status.Errorf(codes.Internal, "leyendo las ventas offline: %v", err)
			}
			return stream.SendAndClose(&pb.SyncStatus{
				Total:   int32(total),
				Synced:  int32(synced),
				Failed:  int32(failed),
				Results: resultados,
			})
		}
		total++

		id := strings.TrimSpace(sale.GetOfflineId())
		cajaVenta := caja
		if cajaVenta == "" {
			cajaVenta = sale.GetTerminalId()
		}

		res, err := s.processSale(db, cajaVenta, &pb.SaleTransaction{
			SaleId:     id,
			TerminalId: sale.GetTerminalId(),
			Items:      sale.GetSale().GetItems(),
			Customer:   sale.GetSale().GetCustomer(),
			Offline:    true,
			OfflineId:  id,
		})
		if err != nil {
			failed++
			log.Printf("Error sincronizando venta %s: %v", id, err)
			resultados = append(resultados, &pb.SyncResult{OfflineId: id, Success: false, Error: err.Error()})
			continue
		}
		synced++
		// Se devuelve el documento de CADA venta: es lo que el terminal necesita
		// para marcarlas como sincronizadas, y ya venía vacío en el proto.
		resultados = append(resultados, &pb.SyncResult{OfflineId: id, Success: true, FiscalNumber: res.ControlNo})
	}
}

// GetAIAdvice NO está implementado.
//
// Devolvía un consejo fijo ("Sistema POS operativo") ante cualquier pregunta,
// que el terminal presenta como respuesta de la IA. Un texto inventado es peor
// que un error: el cajero actúa sobre él creyendo que alguien lo calculó.
func (s *GRPCServer) GetAIAdvice(ctx context.Context, req *pb.AIRequest) (*pb.AIResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetAIAdvice no esta implementado en el servidor POS")
}

// Heartbeat procesa heartbeats.
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pb.Ping) (*pb.Pong, error) {
	return &pb.Pong{Ok: true, ServerTime: time.Now().Unix()}, nil
}
